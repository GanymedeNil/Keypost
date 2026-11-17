package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"fulfillment/internal/auth"
	"fulfillment/internal/db"
	"fulfillment/internal/settings"
)

func newSlug() string { return uuid.NewString() }

func (h *Handler) secretField(key string) map[string]any {
	v := h.S.Settings.Get(key)
	m := map[string]any{"set": v != "", "source": h.S.Settings.Source(key)}
	if v != "" {
		m["hint"] = settings.Hint(v)
	}
	return m
}

func (h *Handler) settingsView() map[string]any {
	s := h.S.Settings
	accounts := s.List(settings.StripeLoginAccounts)
	if accounts == nil {
		accounts = []string{}
	}
	return map[string]any{
		"urls": map[string]any{
			"public_url": h.S.Cfg.PublicURL, "stripe_webhook_url": h.webhookURL(),
			"stripe_oauth_redirect_uri": h.redirectURI(), "api_base_url": h.S.Cfg.PublicURL + "/v1",
		},
		"stripe": map[string]any{
			"secret_key": h.secretField(settings.StripeSecretKey), "webhook_secret": h.secretField(settings.StripeWebhookSecret),
			"api_version": s.Get(settings.StripeAPIVersion), "managed_payments": s.Bool(settings.StripeManagedPayments),
			"oauth_client_id": s.Get(settings.StripeOAuthClientID), "oauth_secret_key": h.secretField(settings.StripeOAuthSecretKey),
			"login_mode": s.Get(settings.StripeLoginMode), "login_accounts": accounts, "login_auto_role": s.Get(settings.StripeLoginAutoRole),
			"oauth_revoke_after_login": s.Bool(settings.StripeOAuthRevoke),
		},
		"smtp": map[string]any{
			"host": s.Get(settings.SMTPHost), "port": s.Int(settings.SMTPPort, 587), "user": s.Get(settings.SMTPUser),
			"password": h.secretField(settings.SMTPPassword), "from": s.Get(settings.SMTPFrom), "security": s.Get(settings.SMTPSecurity),
		},
		"license":  map[string]any{"disable_on_refund": s.Bool(settings.LicenseDisableOnRefund)},
		"download": map[string]any{"limit_per_day": s.Int(settings.DownloadLimitPerDay, 10), "url_ttl_seconds": s.Int(settings.DownloadURLTTLSeconds, 3600)},
		"checkout": map[string]any{"success_url": s.Get(settings.CheckoutSuccessURL), "cancel_url": s.Get(settings.CheckoutCancelURL)},
	}
}

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request, u *db.User) {
	h.ok(w, h.settingsView())
}

// settingSpec maps a (section, field) of the settings JSON to a stored key.
type settingSpec struct {
	section, field, key string
	kind                string // string | secret | bool | int | list | enum
	enum                []string
}

var settingSpecs = []settingSpec{
	{"stripe", "secret_key", settings.StripeSecretKey, "secret", nil},
	{"stripe", "webhook_secret", settings.StripeWebhookSecret, "secret", nil},
	{"stripe", "api_version", settings.StripeAPIVersion, "string", nil},
	{"stripe", "managed_payments", settings.StripeManagedPayments, "bool", nil},
	{"stripe", "oauth_client_id", settings.StripeOAuthClientID, "string", nil},
	{"stripe", "oauth_secret_key", settings.StripeOAuthSecretKey, "secret", nil},
	{"stripe", "login_mode", settings.StripeLoginMode, "enum", []string{"allowlist", "auto"}},
	{"stripe", "login_accounts", settings.StripeLoginAccounts, "list", nil},
	{"stripe", "login_auto_role", settings.StripeLoginAutoRole, "enum", []string{auth.RoleViewer, auth.RoleAdmin}},
	{"stripe", "oauth_revoke_after_login", settings.StripeOAuthRevoke, "bool", nil},
	{"smtp", "host", settings.SMTPHost, "string", nil},
	{"smtp", "port", settings.SMTPPort, "int", nil},
	{"smtp", "user", settings.SMTPUser, "string", nil},
	{"smtp", "password", settings.SMTPPassword, "secret", nil},
	{"smtp", "from", settings.SMTPFrom, "string", nil},
	{"smtp", "security", settings.SMTPSecurity, "enum", []string{"starttls", "tls", "none"}},
	{"license", "disable_on_refund", settings.LicenseDisableOnRefund, "bool", nil},
	{"download", "limit_per_day", settings.DownloadLimitPerDay, "int", nil},
	{"download", "url_ttl_seconds", settings.DownloadURLTTLSeconds, "int", nil},
	{"checkout", "success_url", settings.CheckoutSuccessURL, "string", nil},
	{"checkout", "cancel_url", settings.CheckoutCancelURL, "string", nil},
}

func (h *Handler) putSettings(w http.ResponseWriter, r *http.Request, u *db.User) {
	b, ok := h.readBody(w, r)
	if !ok {
		return
	}
	type change struct {
		key, val string
		reset    bool
	}
	var changes []change
	for _, sp := range settingSpecs {
		sec, ok := b.obj(sp.section)
		if !ok || !sec.has(sp.field) {
			continue
		}
		raw := sec[sp.field]
		path := sp.section + "." + sp.field
		switch sp.kind {
		case "secret":
			s, isStr := raw.(string)
			if raw == nil || !isStr {
				continue // null/absent keeps the stored value
			}
			s = strings.TrimSpace(s)
			if s == "" {
				changes = append(changes, change{key: sp.key, reset: true})
			} else {
				changes = append(changes, change{key: sp.key, val: s})
			}
		case "string":
			s, _ := raw.(string)
			changes = append(changes, change{key: sp.key, val: strings.TrimSpace(s)})
		case "enum":
			s, _ := raw.(string)
			okEnum := false
			for _, e := range sp.enum {
				okEnum = okEnum || e == s
			}
			if !okEnum {
				h.invalid(w, path, "Invalid value for "+path+".")
				return
			}
			changes = append(changes, change{key: sp.key, val: s})
		case "bool":
			v, isB := raw.(bool)
			if !isB {
				h.invalid(w, path, "Expected true or false.")
				return
			}
			changes = append(changes, change{key: sp.key, val: strconv.FormatBool(v)})
		case "int":
			f, isNum := raw.(float64)
			if !isNum || f < 0 {
				h.invalid(w, path, "Expected a non-negative number.")
				return
			}
			changes = append(changes, change{key: sp.key, val: strconv.Itoa(int(f))})
		case "list":
			arr, isArr := raw.([]any)
			if !isArr {
				h.invalid(w, path, "Expected a list.")
				return
			}
			var parts []string
			for _, e := range arr {
				if s, ok := e.(string); ok && strings.TrimSpace(s) != "" {
					parts = append(parts, strings.TrimSpace(s))
				}
			}
			changes = append(changes, change{key: sp.key, val: strings.Join(parts, ",")})
		}
	}
	for _, c := range changes {
		var err error
		if c.reset {
			err = h.S.Settings.Reset(c.key)
		} else {
			err = h.S.Settings.Set(c.key, c.val)
		}
		if err != nil {
			h.failErr(w, err)
			return
		}
	}
	keys := make([]string, 0, len(changes))
	for _, c := range changes {
		keys = append(keys, c.key)
	}
	h.audit(r, u, "settings.update", "settings", "", map[string]any{"keys": keys})
	h.ok(w, h.settingsView())
}
