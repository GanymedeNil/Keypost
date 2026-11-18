package admin

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"fulfillment/internal/auth"
	"fulfillment/internal/brand"
	"fulfillment/internal/crypt"
	"fulfillment/internal/db"
)

var errNoRecovery = errors.New("no such recovery code")

// lockRow takes a row lock on PostgreSQL (SQLite serialises writers anyway).
func lockRow(tx *gorm.DB) *gorm.DB {
	if db.IsPostgres(tx) {
		return tx.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	return tx
}

// Two-step verification (TOTP) for every member; see docs/ADMIN_API.md §1.1.

const (
	mfaMaxFailures = 5                // wrong codes in a row before the account is locked
	mfaLockFor     = 15 * time.Minute // how long
	mfaRecoveryN   = 10
)

// mfaInfo is the "mfa" block of the user object; verified is about the current session.
func (h *Handler) mfaInfo(u *db.User, verified bool) map[string]any {
	return map[string]any{
		"required": h.S.Cfg.MFARequired, "enabled": u.MFAEnabledAt != nil, "verified": verified,
		"recovery_codes_left": len(u.MFARecoveryCodes.V),
	}
}

func (h *Handler) meDTO(u db.User, verified bool) map[string]any {
	d := userDTO(u)
	d["mfa"] = h.mfaInfo(&u, verified)
	return d
}

func (h *Handler) mfaIssuer() string {
	host := h.S.Cfg.PublicURL
	if p, err := url.Parse(host); err == nil && p.Host != "" {
		host = p.Host
	}
	return brand.Name + " (" + host + ")"
}

func (h *Handler) sessionToken(r *http.Request) string {
	c, _ := r.Cookie(sessionCookie)
	if c == nil {
		return ""
	}
	return c.Value
}

// mfaLocked reports whether too many wrong codes locked the account, and for how long still.
func mfaLocked(u *db.User) (time.Duration, bool) {
	if u.MFALockedUntil == nil {
		return 0, false
	}
	left := time.Until(*u.MFALockedUntil)
	return left, left > 0
}

func (h *Handler) failLocked(w http.ResponseWriter, left time.Duration) {
	mins := int(left.Minutes()) + 1
	h.fail(w, http.StatusTooManyRequests, "rate_limited", fmt.Sprintf("Too many incorrect codes. Try again in %d minute(s).", mins), nil)
}

// mfaWrong counts a wrong code and answers: 422, or 429 once the account gets locked.
func (h *Handler) mfaWrong(w http.ResponseWriter, r *http.Request, u *db.User, field string) {
	h.S.DB.Model(&db.User{}).Where("id = ?", u.ID).UpdateColumn("mfa_failed", gorm.Expr("mfa_failed + 1"))
	var cur db.User
	h.S.DB.Select("id", "mfa_failed").First(&cur, u.ID)
	h.audit(r, u, "auth.mfa_failed", "user", u.ID, map[string]any{"attempt": cur.MFAFailed})
	if cur.MFAFailed >= mfaMaxFailures {
		until := db.Now().Add(mfaLockFor)
		h.S.DB.Model(&db.User{}).Where("id = ?", u.ID).Updates(map[string]any{"mfa_failed": 0, "mfa_locked_until": until})
		h.failLocked(w, mfaLockFor)
		return
	}
	h.fail(w, http.StatusUnprocessableEntity, "validation_failed", "The code is incorrect or has already been used.", map[string]string{field: "The code is incorrect or has already been used."})
}

// consumeTOTP checks a code against the member's secret. A code works once: the
// accepted step is stored with a compare-and-set so two parallel requests cannot both use it.
func (h *Handler) consumeTOTP(u *db.User, code string) bool {
	secret, err := crypt.Decrypt(u.MFASecret)
	if err != nil || secret == "" {
		return false
	}
	step, ok := auth.CheckTOTP(secret, code, time.Now(), u.MFALastStep)
	if !ok {
		return false
	}
	res := h.S.DB.Model(&db.User{}).Where("id = ? AND mfa_last_step < ?", u.ID, step).
		Updates(map[string]any{"mfa_last_step": step, "mfa_failed": 0, "mfa_locked_until": nil})
	return res.Error == nil && res.RowsAffected == 1
}

func (h *Handler) mfaSetup(w http.ResponseWriter, r *http.Request, u *db.User) {
	if u.MFAEnabledAt != nil {
		h.fail(w, http.StatusConflict, "conflict", "Two-factor authentication is already enabled. Ask an owner to reset it if you lost your device.", nil)
		return
	}
	secret := auth.NewTOTPSecret()
	enc, err := crypt.Encrypt(secret)
	if err != nil {
		h.failErr(w, err)
		return
	}
	if err := h.S.DB.Model(&db.User{}).Where("id = ?", u.ID).Updates(map[string]any{"mfa_pending_secret": enc, "updated_at": db.Now()}).Error; err != nil {
		h.failErr(w, err)
		return
	}
	h.ok(w, map[string]any{"secret": secret, "otpauth_uri": auth.TOTPURI(h.mfaIssuer(), u.Email, secret)})
}

func (h *Handler) mfaEnable(w http.ResponseWriter, r *http.Request, u *db.User) {
	var in struct {
		Code string `json:"code"`
	}
	if !h.decode(w, r, &in) {
		return
	}
	if u.MFAEnabledAt != nil {
		h.fail(w, http.StatusConflict, "conflict", "Two-factor authentication is already enabled.", nil)
		return
	}
	if u.MFAPendingSecret == "" {
		h.fail(w, http.StatusConflict, "conflict", "Start the setup first.", nil)
		return
	}
	if ok, _, _ := h.limiter.Allow("mfa-enable:"+fmt.Sprint(u.ID), 10); !ok {
		h.fail(w, http.StatusTooManyRequests, "rate_limited", "Too many attempts. Try again in a minute.", nil)
		return
	}
	secret, err := crypt.Decrypt(u.MFAPendingSecret)
	step, ok := auth.CheckTOTP(secret, in.Code, time.Now(), 0)
	if err != nil || !ok {
		h.invalid(w, "code", "The code is incorrect. Check the time on your phone and try the next code.")
		return
	}
	codes, hashes := auth.NewRecoveryCodes(mfaRecoveryN)
	now := db.Now()
	err = h.S.DB.Model(&db.User{}).Where("id = ? AND mfa_enabled_at IS NULL", u.ID).Updates(map[string]any{
		"mfa_secret": u.MFAPendingSecret, "mfa_pending_secret": "", "mfa_enabled_at": now, "mfa_last_step": step,
		"mfa_failed": 0, "mfa_locked_until": nil, "mfa_recovery_codes": db.J(hashes), "updated_at": now,
	}).Error
	if err != nil {
		h.failErr(w, err)
		return
	}
	if err := auth.VerifySession(h.S.DB, h.sessionToken(r)); err != nil {
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "auth.mfa_enabled", "user", u.ID, nil)
	h.ok(w, map[string]any{"recovery_codes": codes})
}

func (h *Handler) mfaVerify(w http.ResponseWriter, r *http.Request, u *db.User) {
	var in struct {
		Code         string `json:"code"`
		RecoveryCode string `json:"recovery_code"`
	}
	if !h.decode(w, r, &in) {
		return
	}
	if u.MFAEnabledAt == nil {
		h.fail(w, http.StatusConflict, "mfa_setup_required", "Two-factor authentication is not set up yet.", nil)
		return
	}
	if ok, _, _ := h.limiter.Allow("mfa-ip:"+h.ip(r), 30); !ok {
		h.fail(w, http.StatusTooManyRequests, "rate_limited", "Too many attempts. Try again in a minute.", nil)
		return
	}
	if left, locked := mfaLocked(u); locked {
		h.failLocked(w, left)
		return
	}
	switch {
	case strings.TrimSpace(in.Code) != "":
		if !h.consumeTOTP(u, in.Code) {
			h.mfaWrong(w, r, u, "code")
			return
		}
		h.audit(r, u, "auth.mfa_verified", "user", u.ID, nil)
	case strings.TrimSpace(in.RecoveryCode) != "":
		left, ok := h.consumeRecoveryCode(u, in.RecoveryCode)
		if !ok {
			h.mfaWrong(w, r, u, "recovery_code")
			return
		}
		h.audit(r, u, "auth.mfa_recovery_used", "user", u.ID, map[string]any{"codes_left": left})
	default:
		h.invalid(w, "code", "Enter the code from your authenticator app.")
		return
	}
	if err := auth.VerifySession(h.S.DB, h.sessionToken(r)); err != nil {
		h.failErr(w, err)
		return
	}
	var fresh db.User
	h.S.DB.First(&fresh, u.ID)
	h.ok(w, h.meDTO(fresh, true))
}

// consumeRecoveryCode spends a recovery code (they are single use); it returns how many are left.
func (h *Handler) consumeRecoveryCode(u *db.User, code string) (left int, ok bool) {
	want := auth.HashRecoveryCode(code)
	err := h.S.DB.Transaction(func(tx *gorm.DB) error {
		var cur db.User
		if err := lockRow(tx).First(&cur, u.ID).Error; err != nil {
			return err
		}
		var rest []string
		found := false
		for _, hsh := range cur.MFARecoveryCodes.V {
			if !found && crypt.MACEqual(hsh, want) {
				found = true
				continue
			}
			rest = append(rest, hsh)
		}
		if !found {
			return errNoRecovery
		}
		left = len(rest)
		return tx.Model(&db.User{}).Where("id = ?", u.ID).Updates(map[string]any{
			"mfa_recovery_codes": db.J(rest), "mfa_failed": 0, "mfa_locked_until": nil, "updated_at": db.Now(),
		}).Error
	})
	return left, err == nil
}

func (h *Handler) mfaRecoveryCodes(w http.ResponseWriter, r *http.Request, u *db.User) {
	var in struct {
		Code string `json:"code"`
	}
	if !h.decode(w, r, &in) {
		return
	}
	if u.MFAEnabledAt == nil {
		h.fail(w, http.StatusConflict, "conflict", "Two-factor authentication is not enabled.", nil)
		return
	}
	if left, locked := mfaLocked(u); locked {
		h.failLocked(w, left)
		return
	}
	if !h.consumeTOTP(u, in.Code) {
		h.mfaWrong(w, r, u, "code")
		return
	}
	codes, hashes := auth.NewRecoveryCodes(mfaRecoveryN)
	if err := h.S.DB.Model(&db.User{}).Where("id = ?", u.ID).Updates(map[string]any{"mfa_recovery_codes": db.J(hashes), "updated_at": db.Now()}).Error; err != nil {
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "auth.mfa_recovery_regenerated", "user", u.ID, nil)
	h.ok(w, map[string]any{"recovery_codes": codes})
}

// mfaReset (owner) removes a member's authenticator: they sign in again and set up a new one.
func (h *Handler) mfaReset(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	if id == u.ID {
		h.fail(w, http.StatusConflict, "conflict", "You cannot reset your own two-factor authentication. Another owner can do it, or run dfctl on the server.", nil)
		return
	}
	var t db.User
	if err := h.S.DB.First(&t, id).Error; err != nil {
		h.failErr(w, err)
		return
	}
	err := h.S.DB.Model(&db.User{}).Where("id = ?", id).Updates(map[string]any{
		"mfa_secret": "", "mfa_pending_secret": "", "mfa_enabled_at": nil, "mfa_last_step": 0, "mfa_failed": 0,
		"mfa_locked_until": nil, "mfa_recovery_codes": db.J([]string(nil)), "updated_at": db.Now(),
	}).Error
	if err != nil {
		h.failErr(w, err)
		return
	}
	auth.DestroyUserSessions(h.S.DB, id)
	h.audit(r, u, "user.mfa_reset", "user", id, map[string]any{"email": t.Email})
	var fresh db.User // not t: scanning into a loaded struct would keep its old pointer fields
	h.S.DB.First(&fresh, id)
	h.ok(w, userDTO(fresh))
}
