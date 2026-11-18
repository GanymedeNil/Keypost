// Package auth contains password hashing, admin sessions and API key helpers.
package auth

import (
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"fulfillment/internal/crypt"
	"fulfillment/internal/db"
)

const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleViewer = "viewer"
)

func ValidRole(r string) bool { return r == RoleOwner || r == RoleAdmin || r == RoleViewer }

func HashPassword(p string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(p), 12)
	return string(b), err
}

func CheckPassword(hash, p string) bool {
	if hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(p)) == nil
}

// dummyHash lets failed logins for unknown emails cost the same time.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password"), 12)

func BurnPassword(p string) { _ = bcrypt.CompareHashAndPassword(dummyHash, []byte(p)) }

// ---------------------------------------------------------------- sessions

// NewSession stores a session and returns the cookie token. verified says
// whether the session already counts as having passed two-step verification.
func NewSession(g *gorm.DB, userID int64, ip, ua string, ttl time.Duration, verified bool) (token string, exp time.Time, err error) {
	token = crypt.RandomToken(32)
	now := db.Now()
	exp = now.Add(ttl)
	s := db.AdminSession{ID: crypt.SHA256Hex(token), UserID: userID, IP: ip, UserAgent: trunc(ua, 255), ExpiresAt: exp, CreatedAt: now, LastSeenAt: now, MFAVerified: verified}
	return token, exp, g.Create(&s).Error
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// SessionInfo resolves a cookie token to its (enabled) user and the session row.
func SessionInfo(g *gorm.DB, token string) (*db.User, *db.AdminSession, error) {
	if token == "" {
		return nil, nil, errors.New("no session")
	}
	var s db.AdminSession
	if err := g.Where("id = ? AND expires_at > ?", crypt.SHA256Hex(token), db.Now()).First(&s).Error; err != nil {
		return nil, nil, err
	}
	var u db.User
	if err := g.First(&u, s.UserID).Error; err != nil {
		return nil, nil, err
	}
	if u.Disabled {
		return nil, nil, errors.New("user disabled")
	}
	if time.Since(s.LastSeenAt) > 5*time.Minute {
		g.Model(&s).Update("last_seen_at", db.Now())
	}
	return &u, &s, nil
}

// SessionUser resolves a cookie token to its (enabled) user.
func SessionUser(g *gorm.DB, token string) (*db.User, error) {
	u, _, err := SessionInfo(g, token)
	return u, err
}

// VerifySession marks the session behind a cookie token as having passed the second step.
func VerifySession(g *gorm.DB, token string) error {
	return g.Model(&db.AdminSession{}).Where("id = ?", crypt.SHA256Hex(token)).Update("mfa_verified", true).Error
}

func DestroySession(g *gorm.DB, token string) {
	g.Delete(&db.AdminSession{}, "id = ?", crypt.SHA256Hex(token))
}

func DestroyUserSessions(g *gorm.DB, userID int64) {
	g.Delete(&db.AdminSession{}, "user_id = ?", userID)
}

// ---------------------------------------------------------------- API keys

// NewAPIKey creates a key and returns the plaintext token (shown once).
func NewAPIKey(g *gorm.DB, userID int64, name string, testMode bool, expires *time.Time) (*db.APIKey, string, error) {
	mode := "live"
	if testMode {
		mode = "test"
	}
	token := "dfk_" + mode + "_" + crypt.RandomToken(36)
	k := db.APIKey{UserID: userID, Name: strings.TrimSpace(name), Prefix: token[:12], KeyHash: crypt.SHA256Hex(token), TestMode: testMode, ExpiresAt: expires, CreatedAt: db.Now()}
	if err := g.Create(&k).Error; err != nil {
		return nil, "", err
	}
	return &k, token, nil
}
