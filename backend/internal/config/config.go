// Package config loads process-level configuration from environment variables.
// Runtime-editable settings (Stripe keys, SMTP, ...) live in the settings
// package and fall back to the values defined here.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env        string // development | production
	ListenAddr string
	PublicURL  string // externally visible base URL, no trailing slash
	AppSecret  string // master secret: encryption of stored secrets + URL/cookie signing

	DatabaseURL string // postgres://... or sqlite:path/to/file.db
	DataDir     string

	StorageDriver string // local | s3
	S3            S3Config

	Stripe StripeConfig
	SMTP   SMTPConfig

	AdminEmail    string
	AdminPassword string
	AdminName     string

	LicenseRateLimit   int // requests / minute / IP
	APIRateLimit       int // requests / minute / key
	DownloadLimitDay   int // downloads / day / (file, IP)
	DownloadURLTTL     time.Duration
	SessionTTL         time.Duration
	TrustProxyHeaders  bool
	CORSAllowedOrigins []string
	DisableKeysRefund  bool // disable license keys on full refund
	WebhookMaxAttempts int
}

type S3Config struct {
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	UseSSL    bool
	PathStyle bool
	Prefix    string
}

type StripeConfig struct {
	SecretKey        string
	WebhookSecret    string // may hold several comma separated secrets
	APIVersion       string
	APIBase          string // override for tests
	ManagedPayments  bool
	OAuthClientID    string
	OAuthSecretKey   string
	OAuthAuthBase    string // override for tests, default https://connect.stripe.com
	LoginMode        string // allowlist | auto
	LoginAccounts    []string
	LoginAutoRole    string
	OAuthRevokeAfter bool
}

type SMTPConfig struct {
	Host     string
	Port     int
	User     string
	Pass     string
	From     string
	Security string // starttls | tls | none
}

func Load() (*Config, error) {
	c := &Config{
		Env:        env("APP_ENV", "development"),
		ListenAddr: env("LISTEN_ADDR", ":8080"),
		PublicURL:  strings.TrimRight(env("PUBLIC_URL", "http://localhost:3000"), "/"),
		AppSecret:  os.Getenv("APP_SECRET"),

		DatabaseURL:   env("DATABASE_URL", "sqlite:./data/fulfillment.db"),
		DataDir:       env("DATA_DIR", "./data"),
		StorageDriver: env("STORAGE_DRIVER", "local"),
		S3: S3Config{
			Endpoint:  os.Getenv("S3_ENDPOINT"),
			Region:    env("S3_REGION", "us-east-1"),
			Bucket:    os.Getenv("S3_BUCKET"),
			AccessKey: os.Getenv("S3_ACCESS_KEY"),
			SecretKey: os.Getenv("S3_SECRET_KEY"),
			UseSSL:    envBool("S3_USE_SSL", true),
			PathStyle: envBool("S3_PATH_STYLE", false),
			Prefix:    strings.Trim(env("S3_PREFIX", "files"), "/"),
		},
		Stripe: StripeConfig{
			SecretKey:        os.Getenv("STRIPE_SECRET_KEY"),
			WebhookSecret:    os.Getenv("STRIPE_WEBHOOK_SECRET"),
			APIVersion:       env("STRIPE_API_VERSION", "2025-09-30.clover"),
			APIBase:          env("STRIPE_API_BASE", "https://api.stripe.com"),
			ManagedPayments:  envBool("STRIPE_MANAGED_PAYMENTS", false),
			OAuthClientID:    os.Getenv("STRIPE_OAUTH_CLIENT_ID"),
			OAuthSecretKey:   os.Getenv("STRIPE_OAUTH_SECRET_KEY"),
			OAuthAuthBase:    env("STRIPE_OAUTH_BASE", "https://connect.stripe.com"),
			LoginMode:        env("STRIPE_LOGIN_MODE", "allowlist"),
			LoginAccounts:    splitList(os.Getenv("ADMIN_STRIPE_ACCOUNTS")),
			LoginAutoRole:    env("STRIPE_LOGIN_AUTO_ROLE", "viewer"),
			OAuthRevokeAfter: envBool("STRIPE_OAUTH_REVOKE_AFTER_LOGIN", true),
		},
		SMTP: SMTPConfig{
			Host:     os.Getenv("SMTP_HOST"),
			Port:     envInt("SMTP_PORT", 587),
			User:     os.Getenv("SMTP_USER"),
			Pass:     os.Getenv("SMTP_PASS"),
			From:     os.Getenv("SMTP_FROM"),
			Security: env("SMTP_SECURITY", "starttls"),
		},
		AdminEmail:         os.Getenv("ADMIN_EMAIL"),
		AdminPassword:      os.Getenv("ADMIN_PASSWORD"),
		AdminName:          env("ADMIN_NAME", "Admin"),
		LicenseRateLimit:   envInt("LICENSE_RATE_LIMIT", 60),
		APIRateLimit:       envInt("API_RATE_LIMIT", 300),
		DownloadLimitDay:   envInt("DOWNLOAD_LIMIT_PER_DAY", 10),
		DownloadURLTTL:     time.Duration(envInt("DOWNLOAD_URL_TTL_SECONDS", 3600)) * time.Second,
		SessionTTL:         time.Duration(envInt("SESSION_TTL_HOURS", 24*14)) * time.Hour,
		TrustProxyHeaders:  envBool("TRUST_PROXY_HEADERS", false),
		CORSAllowedOrigins: splitList(os.Getenv("CORS_ALLOWED_ORIGINS")),
		DisableKeysRefund:  envBool("DISABLE_LICENSE_ON_REFUND", true),
		WebhookMaxAttempts: envInt("WEBHOOK_MAX_ATTEMPTS", 4),
	}
	if c.AppSecret == "" {
		if c.Env == "production" {
			return nil, fmt.Errorf("APP_SECRET is required in production (use at least 32 random bytes)")
		}
		c.AppSecret = "dev-only-insecure-secret-change-me-0123456789"
	}
	if len(c.AppSecret) < 16 {
		return nil, fmt.Errorf("APP_SECRET must be at least 16 characters")
	}
	return c, nil
}

func (c *Config) IsProd() bool { return c.Env == "production" }

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envBool(k string, def bool) bool {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		switch strings.ToLower(v) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return def
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
