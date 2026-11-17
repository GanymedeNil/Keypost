package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"fulfillment/internal/auth"
	"fulfillment/internal/crypt"
	"fulfillment/internal/db"
	"fulfillment/internal/settings"
	"fulfillment/internal/stripe"
)

func userDTO(u db.User) map[string]any {
	return map[string]any{
		"id": u.ID, "email": u.Email, "name": u.Name, "role": u.Role, "stripe_account_id": u.StripeAccountID,
		"stripe_account_label": u.StripeAccountLabel, "disabled": u.Disabled, "has_password": u.PasswordHash != "",
		"last_login_at": tsp(u.LastLoginAt), "created_at": ts(u.CreatedAt),
	}
}

func (h *Handler) stripeLoginEnabled() bool {
	return h.S.Settings.Get(settings.StripeOAuthClientID) != "" && h.oauthSecret() != ""
}

func (h *Handler) oauthSecret() string {
	if k := h.S.Settings.Get(settings.StripeOAuthSecretKey); k != "" {
		return k
	}
	return h.S.Settings.Get(settings.StripeSecretKey)
}

func (h *Handler) setupRequired() bool {
	var n int64
	h.S.DB.Model(&db.User{}).Count(&n)
	return n == 0
}

func (h *Handler) authConfig(w http.ResponseWriter, r *http.Request) {
	h.ok(w, map[string]any{"password_login": true, "stripe_login": h.stripeLoginEnabled(), "setup_required": h.setupRequired()})
}

func (h *Handler) startSession(w http.ResponseWriter, r *http.Request, u *db.User) error {
	token, exp, err := auth.NewSession(h.S.DB, u.ID, h.ip(r), r.UserAgent(), h.S.Cfg.SessionTTL)
	if err != nil {
		return err
	}
	now := db.Now()
	h.S.DB.Model(u).Update("last_login_at", now)
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: h.secureCookie(), SameSite: http.SameSiteLaxMode, Expires: exp})
	return nil
}

func (h *Handler) setup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SetupToken string `json:"setup_token"`
		Email      string `json:"email"`
		Name       string `json:"name"`
		Password   string `json:"password"`
	}
	if !h.decode(w, r, &in) {
		return
	}
	if !h.setupRequired() {
		h.fail(w, http.StatusConflict, "conflict", "Setup has already been completed.", nil)
		return
	}
	if ok, _, _ := h.limiter.Allow("setup:"+h.ip(r), 10); !ok {
		h.fail(w, http.StatusTooManyRequests, "rate_limited", "Too many attempts.", nil)
		return
	}
	want := h.S.Settings.Get("setup.token")
	if want == "" || !crypt.MACEqual(crypt.SHA256Hex(strings.TrimSpace(in.SetupToken)), crypt.SHA256Hex(want)) {
		h.invalid(w, "setup_token", "The setup token is invalid. It is printed in the server log on first start.")
		return
	}
	in.Email = strings.TrimSpace(in.Email)
	if !strings.Contains(in.Email, "@") {
		h.invalid(w, "email", "Enter a valid email address.")
		return
	}
	if len(in.Password) < 10 {
		h.invalid(w, "password", "Use at least 10 characters.")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		h.failErr(w, err)
		return
	}
	now := db.Now()
	u := db.User{Email: in.Email, Name: firstNonEmpty(in.Name, "Owner"), PasswordHash: hash, Role: auth.RoleOwner, CreatedAt: now, UpdatedAt: now}
	if err := h.S.DB.Create(&u).Error; err != nil {
		h.failErr(w, err)
		return
	}
	_ = h.S.Settings.Reset("setup.token")
	if err := h.startSession(w, r, &u); err != nil {
		h.failErr(w, err)
		return
	}
	h.audit(r, &u, "auth.setup", "user", u.ID, nil)
	h.created(w, map[string]any{"user": userDTO(u)})
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Password string }
	if !h.decode(w, r, &in) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if ok, _, _ := h.limiter.Allow("login-ip:"+h.ip(r), 20); !ok {
		h.fail(w, http.StatusTooManyRequests, "rate_limited", "Too many sign-in attempts. Try again in a minute.", nil)
		return
	}
	if ok, _, _ := h.limiter.Allow("login-user:"+email, 8); !ok {
		h.fail(w, http.StatusTooManyRequests, "rate_limited", "Too many sign-in attempts for this account. Try again in a minute.", nil)
		return
	}
	var u db.User
	err := h.S.DB.Where("LOWER(email) = ?", email).First(&u).Error
	if err != nil || !auth.CheckPassword(u.PasswordHash, in.Password) {
		if err != nil {
			auth.BurnPassword(in.Password)
		}
		h.fail(w, http.StatusUnauthorized, "unauthenticated", "Invalid email or password.", nil)
		return
	}
	if u.Disabled {
		h.fail(w, http.StatusForbidden, "forbidden", "This account is disabled.", nil)
		return
	}
	if err := h.startSession(w, r, &u); err != nil {
		h.failErr(w, err)
		return
	}
	h.audit(r, &u, "auth.login", "user", u.ID, nil)
	h.ok(w, map[string]any{"user": userDTO(u)})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		auth.DestroySession(h.S.DB, c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	noContent(w)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request, u *db.User) { h.ok(w, userDTO(*u)) }

func (h *Handler) updateMe(w http.ResponseWriter, r *http.Request, u *db.User) {
	var in struct {
		Name *string `json:"name"`
	}
	if !h.decode(w, r, &in) {
		return
	}
	if in.Name != nil {
		u.Name = strings.TrimSpace(*in.Name)
		h.S.DB.Model(u).Updates(map[string]any{"name": u.Name, "updated_at": db.Now()})
	}
	h.ok(w, userDTO(*u))
}

func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request, u *db.User) {
	var in struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if !h.decode(w, r, &in) {
		return
	}
	if u.PasswordHash != "" && !auth.CheckPassword(u.PasswordHash, in.Current) {
		h.invalid(w, "current_password", "The current password is incorrect.")
		return
	}
	if len(in.New) < 10 {
		h.invalid(w, "new_password", "Use at least 10 characters.")
		return
	}
	hash, err := auth.HashPassword(in.New)
	if err != nil {
		h.failErr(w, err)
		return
	}
	h.S.DB.Model(u).Updates(map[string]any{"password_hash": hash, "updated_at": db.Now()})
	// sign out every other session
	if c, err := r.Cookie(sessionCookie); err == nil {
		h.S.DB.Where("user_id = ? AND id <> ?", u.ID, crypt.SHA256Hex(c.Value)).Delete(&db.AdminSession{})
	}
	h.audit(r, u, "auth.password_changed", "user", u.ID, nil)
	noContent(w)
}

func (h *Handler) unlinkStripe(w http.ResponseWriter, r *http.Request, u *db.User) {
	if u.PasswordHash == "" {
		h.fail(w, http.StatusConflict, "conflict", "Set a password first, otherwise you could lock yourself out.", nil)
		return
	}
	h.S.DB.Model(u).Updates(map[string]any{"stripe_account_id": nil, "stripe_account_label": "", "updated_at": db.Now()})
	h.audit(r, u, "auth.stripe_unlinked", "user", u.ID, nil)
	noContent(w)
}

// ---------------------------------------------------------------- Sign in with Stripe (Connect OAuth)

const oauthCookie = "df_oauth"

func (h *Handler) redirectURI() string { return h.S.Cfg.PublicURL + "/admin-api/auth/stripe/callback" }

func (h *Handler) oauthFail(w http.ResponseWriter, r *http.Request, code string, mode string) {
	dest := "/admin/login?error=" + url.QueryEscape(code)
	if mode == "link" {
		dest = "/admin/settings/profile?error=" + url.QueryEscape(code)
	}
	http.Redirect(w, r, h.S.Cfg.PublicURL+dest, http.StatusFound)
}

func (h *Handler) stripeStart(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	if mode != "link" {
		mode = "login"
	}
	var uid int64
	if mode == "link" {
		u, ok := h.currentUser(r)
		if !ok {
			h.oauthFail(w, r, "unauthenticated", "login")
			return
		}
		uid = u.ID
	}
	if !h.stripeLoginEnabled() {
		h.oauthFail(w, r, "stripe_unconfigured", mode)
		return
	}
	nonce := crypt.RandomToken(18)
	exp := time.Now().Add(10 * time.Minute).Unix()
	val := fmt.Sprintf("%s|%d|%s|%d|%s", mode, uid, nonce, exp, crypt.MAC("oauth", mode, strconv.FormatInt(uid, 10), nonce, strconv.FormatInt(exp, 10)))
	http.SetCookie(w, &http.Cookie{Name: oauthCookie, Value: val, Path: "/admin-api/auth/stripe", HttpOnly: true, Secure: h.secureCookie(), SameSite: http.SameSiteLaxMode, MaxAge: 600})
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", h.S.Settings.Get(settings.StripeOAuthClientID))
	q.Set("scope", "read_write")
	q.Set("state", nonce)
	q.Set("redirect_uri", h.redirectURI())
	base := strings.TrimRight(h.S.Cfg.Stripe.OAuthAuthBase, "/")
	http.Redirect(w, r, base+"/oauth/authorize?"+q.Encode(), http.StatusFound)
}

type oauthState struct {
	Mode string
	UID  int64
}

func (h *Handler) readOAuthState(r *http.Request, state string) (*oauthState, bool) {
	c, err := r.Cookie(oauthCookie)
	if err != nil {
		return nil, false
	}
	parts := strings.Split(c.Value, "|")
	if len(parts) != 5 {
		return nil, false
	}
	exp, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil || exp < time.Now().Unix() || parts[2] != state || state == "" {
		return nil, false
	}
	if !crypt.MACEqual(parts[4], crypt.MAC("oauth", parts[0], parts[1], parts[2], parts[3])) {
		return nil, false
	}
	uid, _ := strconv.ParseInt(parts[1], 10, 64)
	return &oauthState{Mode: parts[0], UID: uid}, true
}

func (h *Handler) stripeCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	http.SetCookie(w, &http.Cookie{Name: oauthCookie, Value: "", Path: "/admin-api/auth/stripe", MaxAge: -1, HttpOnly: true})
	st, ok := h.readOAuthState(r, q.Get("state"))
	if !ok {
		h.oauthFail(w, r, "stripe_state", "login")
		return
	}
	if q.Get("error") != "" {
		h.oauthFail(w, r, "stripe_denied", st.Mode)
		return
	}
	if ok, _, _ := h.limiter.Allow("oauth:"+h.ip(r), 20); !ok {
		h.oauthFail(w, r, "stripe_exchange", st.Mode)
		return
	}
	secret := h.oauthSecret()
	tok, err := stripe.OAuthExchange(r.Context(), nil, h.S.Cfg.Stripe.OAuthAuthBase, secret, q.Get("code"))
	if err != nil {
		h.S.Log.Warn("stripe oauth exchange failed", "err", err)
		h.oauthFail(w, r, "stripe_exchange", st.Mode)
		return
	}
	acct := tok.StripeUserID
	label, email := h.describeAccount(r.Context(), secret, acct)
	// We only need the identity: drop the platform's access to the account again.
	if h.S.Settings.Bool(settings.StripeOAuthRevoke) {
		clientID := h.S.Settings.Get(settings.StripeOAuthClientID)
		h.S.Go(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if err := stripe.OAuthDeauthorize(ctx, nil, h.S.Cfg.Stripe.OAuthAuthBase, secret, clientID, acct); err != nil {
				h.S.Log.Warn("stripe oauth deauthorize failed", "account", acct, "err", err)
			}
		})
	}

	if st.Mode == "link" {
		var u db.User
		if err := h.S.DB.First(&u, st.UID).Error; err != nil {
			h.oauthFail(w, r, "unauthenticated", "login")
			return
		}
		var other int64
		h.S.DB.Model(&db.User{}).Where("stripe_account_id = ? AND id <> ?", acct, u.ID).Count(&other)
		if other > 0 {
			h.oauthFail(w, r, "stripe_not_allowed", "link")
			return
		}
		h.S.DB.Model(&u).Updates(map[string]any{"stripe_account_id": acct, "stripe_account_label": label, "updated_at": db.Now()})
		h.audit(r, &u, "auth.stripe_linked", "user", u.ID, map[string]any{"account": acct})
		http.Redirect(w, r, h.S.Cfg.PublicURL+"/admin/settings/profile?linked=1", http.StatusFound)
		return
	}

	u, code := h.resolveStripeUser(acct, label, email)
	if u == nil {
		h.S.Log.Warn("stripe login rejected", "account", acct, "reason", code)
		h.oauthFail(w, r, code, "login")
		return
	}
	if err := h.startSession(w, r, u); err != nil {
		h.oauthFail(w, r, "stripe_exchange", "login")
		return
	}
	h.audit(r, u, "auth.login_stripe", "user", u.ID, map[string]any{"account": acct})
	http.Redirect(w, r, h.S.Cfg.PublicURL+"/admin", http.StatusFound)
}

// describeAccount best-effort fetches a display label and email for the connected account.
func (h *Handler) describeAccount(ctx context.Context, secret, acct string) (label, email string) {
	cl := stripe.NewClient(secret, h.S.Settings.Get(settings.StripeAPIVersion), h.S.Cfg.Stripe.APIBase)
	cl.ConnectedAccount = acct
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if a, err := cl.Account(ctx); err == nil {
		return a.DisplayName(), a.Email
	}
	return acct, ""
}

// resolveStripeUser applies the login policy. It returns the user, or nil and
// an error code understood by the login page.
func (h *Handler) resolveStripeUser(acct, label, email string) (*db.User, string) {
	var u db.User
	if err := h.S.DB.Where("stripe_account_id = ?", acct).First(&u).Error; err == nil {
		if u.Disabled {
			return nil, "disabled"
		}
		if label != "" && label != u.StripeAccountLabel {
			h.S.DB.Model(&u).Update("stripe_account_label", label)
		}
		return &u, ""
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, "stripe_exchange"
	}
	allowed := false
	role := auth.RoleAdmin
	for _, a := range h.S.Settings.List(settings.StripeLoginAccounts) {
		if a == acct {
			allowed = true
		}
	}
	if !allowed && h.S.Settings.Get(settings.StripeLoginMode) == "auto" {
		var n int64
		h.S.DB.Model(&db.Store{}).Where("stripe_account_id = ?", acct).Count(&n)
		if n > 0 {
			allowed = true
			if r := h.S.Settings.Get(settings.StripeLoginAutoRole); auth.ValidRole(r) {
				role = r
			}
		}
	}
	if !allowed {
		return nil, "stripe_not_allowed"
	}
	var owners int64
	h.S.DB.Model(&db.User{}).Where("role = ?", auth.RoleOwner).Count(&owners)
	if owners == 0 {
		role = auth.RoleOwner
	}
	mail := email
	var clash int64
	h.S.DB.Model(&db.User{}).Where("LOWER(email) = ?", strings.ToLower(mail)).Count(&clash)
	if mail == "" || clash > 0 {
		mail = strings.ToLower(acct) + "@stripe.local"
	}
	now := db.Now()
	nu := db.User{Email: mail, Name: firstNonEmpty(label, acct), Role: role, StripeAccountID: &acct, StripeAccountLabel: label, CreatedAt: now, UpdatedAt: now}
	if err := h.S.DB.Create(&nu).Error; err != nil {
		return nil, "stripe_exchange"
	}
	return &nu, ""
}
