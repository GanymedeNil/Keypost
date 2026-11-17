// Package settings exposes runtime-editable configuration. Values saved from
// the admin UI (encrypted in the settings table) override the environment.
package settings

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"fulfillment/internal/config"
	"fulfillment/internal/db"
)

const (
	StripeSecretKey        = "stripe.secret_key"
	StripeWebhookSecret    = "stripe.webhook_secret"
	StripeAPIVersion       = "stripe.api_version"
	StripeManagedPayments  = "stripe.managed_payments"
	StripeOAuthClientID    = "stripe.oauth_client_id"
	StripeOAuthSecretKey   = "stripe.oauth_secret_key"
	StripeLoginMode        = "stripe.login_mode"
	StripeLoginAccounts    = "stripe.login_accounts"
	StripeLoginAutoRole    = "stripe.login_auto_role"
	StripeOAuthRevoke      = "stripe.oauth_revoke_after_login"
	SMTPHost               = "smtp.host"
	SMTPPort               = "smtp.port"
	SMTPUser               = "smtp.user"
	SMTPPassword           = "smtp.password"
	SMTPFrom               = "smtp.from"
	SMTPSecurity           = "smtp.security"
	LicenseDisableOnRefund = "license.disable_on_refund"
	DownloadLimitPerDay    = "download.limit_per_day"
	DownloadURLTTLSeconds  = "download.url_ttl_seconds"
	CheckoutSuccessURL     = "checkout.success_url"
	CheckoutCancelURL      = "checkout.cancel_url"
)

// SecretKeys never leave the server in clear text.
var SecretKeys = map[string]bool{
	StripeSecretKey: true, StripeWebhookSecret: true, StripeOAuthSecretKey: true, SMTPPassword: true,
}

type Service struct {
	db       *gorm.DB
	defaults map[string]string

	mu       sync.RWMutex
	cache    map[string]string
	loadedAt time.Time
}

func New(g *gorm.DB, c *config.Config) *Service {
	d := map[string]string{
		StripeSecretKey:        c.Stripe.SecretKey,
		StripeWebhookSecret:    c.Stripe.WebhookSecret,
		StripeAPIVersion:       c.Stripe.APIVersion,
		StripeManagedPayments:  strconv.FormatBool(c.Stripe.ManagedPayments),
		StripeOAuthClientID:    c.Stripe.OAuthClientID,
		StripeOAuthSecretKey:   c.Stripe.OAuthSecretKey,
		StripeLoginMode:        c.Stripe.LoginMode,
		StripeLoginAccounts:    strings.Join(c.Stripe.LoginAccounts, ","),
		StripeLoginAutoRole:    c.Stripe.LoginAutoRole,
		StripeOAuthRevoke:      strconv.FormatBool(c.Stripe.OAuthRevokeAfter),
		SMTPHost:               c.SMTP.Host,
		SMTPPort:               strconv.Itoa(c.SMTP.Port),
		SMTPUser:               c.SMTP.User,
		SMTPPassword:           c.SMTP.Pass,
		SMTPFrom:               c.SMTP.From,
		SMTPSecurity:           c.SMTP.Security,
		LicenseDisableOnRefund: strconv.FormatBool(c.DisableKeysRefund),
		DownloadLimitPerDay:    strconv.Itoa(c.DownloadLimitDay),
		DownloadURLTTLSeconds:  strconv.Itoa(int(c.DownloadURLTTL / time.Second)),
		CheckoutSuccessURL:     "",
		CheckoutCancelURL:      "",
		"fx.rates":             os.Getenv("FX_RATES"), // JSON, e.g. {"EUR":1.09,"GBP":1.28}: USD per 1 unit
	}
	return &Service{db: g, defaults: d, cache: map[string]string{}}
}

func (s *Service) load() {
	var rows []db.Setting
	if err := s.db.Find(&rows).Error; err != nil {
		return
	}
	m := make(map[string]string, len(rows))
	for _, r := range rows {
		m[r.Key] = string(r.Value)
	}
	s.cache = m
	s.loadedAt = time.Now()
}

func (s *Service) ensureFresh() {
	s.mu.RLock()
	fresh := time.Since(s.loadedAt) < 10*time.Second
	s.mu.RUnlock()
	if fresh {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Since(s.loadedAt) >= 10*time.Second {
		s.load()
	}
}

// Get returns the DB value when present, otherwise the environment default.
func (s *Service) Get(key string) string {
	s.ensureFresh()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.cache[key]; ok {
		return v
	}
	return s.defaults[key]
}

// Source reports where the effective value comes from: "db", "env" or "unset".
func (s *Service) Source(key string) string {
	s.ensureFresh()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.cache[key]; ok {
		return "db"
	}
	if s.defaults[key] != "" {
		return "env"
	}
	return "unset"
}

func (s *Service) Bool(key string) bool {
	v := strings.ToLower(strings.TrimSpace(s.Get(key)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func (s *Service) Int(key string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(s.Get(key))); err == nil {
		return n
	}
	return def
}

func (s *Service) List(key string) []string {
	var out []string
	for _, p := range strings.Split(s.Get(key), ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Set stores a value in the database. An empty value deletes the override so
// the environment default applies again... except for explicit clears: pass
// Clear to store an empty override.
func (s *Service) Set(key, value string) error {
	err := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&db.Setting{Key: key, Value: db.Secret(value), UpdatedAt: db.Now()}).Error
	if err != nil {
		return err
	}
	s.invalidate()
	return nil
}

// Reset removes the database override.
func (s *Service) Reset(key string) error {
	if err := s.db.Delete(&db.Setting{}, "key = ?", key).Error; err != nil {
		return err
	}
	s.invalidate()
	return nil
}

func (s *Service) invalidate() {
	s.mu.Lock()
	s.loadedAt = time.Time{}
	s.mu.Unlock()
}

// Hint renders a safe preview of a secret (first 7 + last 4 chars).
func Hint(v string) string {
	if v == "" {
		return ""
	}
	if len(v) <= 12 {
		return "…" + v[len(v)-2:]
	}
	return v[:7] + "…" + v[len(v)-4:]
}
