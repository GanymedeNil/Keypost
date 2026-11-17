package admin

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"fulfillment/internal/auth"
	"fulfillment/internal/db"
	"fulfillment/internal/mail"
	"fulfillment/internal/service"
)

// ---------------------------------------------------------------- outbound webhooks

func (h *Handler) webhookDTOs(ws []db.Webhook) []map[string]any {
	since := db.Now().Add(-24 * time.Hour)
	out := make([]map[string]any, len(ws))
	for i, wh := range ws {
		var ok24, bad24 int64
		h.S.DB.Model(&db.WebhookDelivery{}).Where("webhook_id = ? AND created_at > ? AND status = ?", wh.ID, since, "success").Count(&ok24)
		h.S.DB.Model(&db.WebhookDelivery{}).Where("webhook_id = ? AND created_at > ? AND status = ?", wh.ID, since, "failed").Count(&bad24)
		ev := wh.Events.V
		if ev == nil {
			ev = []string{}
		}
		out[i] = map[string]any{
			"id": wh.ID, "store_id": wh.StoreID, "url": wh.URL, "events": ev, "disabled": wh.Disabled, "secret_set": wh.Secret != "",
			"last_sent_at": tsp(wh.LastSentAt), "test_mode": wh.TestMode, "stats": map[string]any{"success_24h": ok24, "failed_24h": bad24},
			"created_at": ts(wh.CreatedAt), "updated_at": ts(wh.UpdatedAt),
		}
	}
	return out
}

func (h *Handler) webhookEvents(w http.ResponseWriter, r *http.Request, u *db.User) {
	h.ok(w, service.WebhookEvents)
}

func (h *Handler) listWebhooks(w http.ResponseWriter, r *http.Request, u *db.User) {
	p := parsePage(r)
	q := h.S.DB.Model(&db.Webhook{}).Where("test_mode = ?", p.TestMode)
	if p.StoreID > 0 {
		q = q.Where("store_id = ?", p.StoreID)
	}
	if p.Q != "" {
		q = q.Where("LOWER(url) LIKE ? ESCAPE '\\'", like(p.Q))
	}
	q = applySort(q, p.Sort, map[string]string{"created_at": "created_at", "id": "id"}, "id DESC")
	var rows []db.Webhook
	meta, err := paginate(q, p, &rows)
	if err != nil {
		h.failErr(w, err)
		return
	}
	h.respondList(w, h.webhookDTOs(rows), meta)
}

func (h *Handler) getWebhook(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	var wh db.Webhook
	if err := h.S.DB.First(&wh, id).Error; err != nil {
		h.failErr(w, err)
		return
	}
	h.ok(w, h.webhookDTOs([]db.Webhook{wh})[0])
}

func (h *Handler) applyWebhook(w http.ResponseWriter, wh *db.Webhook, b body, creating bool) bool {
	if s, ok := b.str("url"); ok {
		pu, err := url.Parse(s)
		if err != nil || (pu.Scheme != "http" && pu.Scheme != "https") || pu.Host == "" {
			h.invalid(w, "url", "Enter a valid http(s) URL.")
			return false
		}
		wh.URL = s
	} else if creating {
		h.invalid(w, "url", "The url field is required.")
		return false
	}
	if ev, ok := b.strList("events"); ok {
		for _, e := range ev {
			if !service.IsWebhookEvent(e) {
				h.invalid(w, "events", "Unknown event: "+e)
				return false
			}
		}
		if len(ev) == 0 {
			h.invalid(w, "events", "Select at least one event.")
			return false
		}
		wh.Events = db.J(ev)
	} else if creating {
		h.invalid(w, "events", "Select at least one event.")
		return false
	}
	if s, ok := b["secret"].(string); ok && s != "" {
		if len(s) < 6 || len(s) > 40 {
			h.invalid(w, "secret", "The secret must be between 6 and 40 characters.")
			return false
		}
		wh.Secret = db.Secret(s)
	} else if creating {
		h.invalid(w, "secret", "The secret field is required (6–40 characters).")
		return false
	}
	if d, ok := b.boolean("disabled"); ok {
		wh.Disabled = d
	}
	return true
}

func (h *Handler) createWebhook(w http.ResponseWriter, r *http.Request, u *db.User) {
	b, ok := h.readBody(w, r)
	if !ok {
		return
	}
	sid, _ := b.int("store_id")
	if sid == 0 {
		var st db.Store
		if h.S.DB.Order("id").First(&st).Error == nil {
			sid = st.ID
		}
	}
	var st db.Store
	if err := h.S.DB.First(&st, sid).Error; err != nil {
		h.invalid(w, "store_id", "The selected store is invalid.")
		return
	}
	wh := db.Webhook{StoreID: st.ID, CreatedAt: db.Now(), UpdatedAt: db.Now()}
	if tm, ok := b.boolean("test_mode"); ok {
		wh.TestMode = tm
	}
	if !h.applyWebhook(w, &wh, b, true) {
		return
	}
	if err := h.S.DB.Create(&wh).Error; err != nil {
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "webhook.create", "webhook", wh.ID, map[string]any{"url": wh.URL})
	h.created(w, h.webhookDTOs([]db.Webhook{wh})[0])
}

func (h *Handler) updateWebhook(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	var wh db.Webhook
	if err := h.S.DB.First(&wh, id).Error; err != nil {
		h.failErr(w, err)
		return
	}
	b, ok := h.readBody(w, r)
	if !ok {
		return
	}
	if !h.applyWebhook(w, &wh, b, false) {
		return
	}
	wh.UpdatedAt = db.Now()
	if err := h.S.DB.Save(&wh).Error; err != nil {
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "webhook.update", "webhook", wh.ID, nil)
	h.ok(w, h.webhookDTOs([]db.Webhook{wh})[0])
}

func (h *Handler) deleteWebhook(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	h.S.DB.Where("webhook_id = ? AND status = ?", id, "pending").Delete(&db.WebhookDelivery{})
	if err := h.S.DB.Delete(&db.Webhook{}, id).Error; err != nil {
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "webhook.delete", "webhook", id, nil)
	noContent(w)
}

func (h *Handler) testWebhook(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	var wh db.Webhook
	if err := h.S.DB.First(&wh, id).Error; err != nil {
		h.failErr(w, err)
		return
	}
	d, err := h.D.SendTest(r.Context(), wh)
	if err != nil || d == nil {
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "webhook.test", "webhook", id, nil)
	h.ok(w, h.deliveryDTOs([]db.WebhookDelivery{*d}, true)[0])
}

func (h *Handler) deliveryDTOs(ds []db.WebhookDelivery, payload bool) []map[string]any {
	ids := map[int64]bool{}
	for _, d := range ds {
		ids[d.WebhookID] = true
	}
	urls := map[int64]string{}
	if len(ids) > 0 {
		var keys []int64
		for k := range ids {
			keys = append(keys, k)
		}
		var whs []db.Webhook
		h.S.DB.Where("id IN ?", keys).Find(&whs)
		for _, wh := range whs {
			urls[wh.ID] = wh.URL
		}
	}
	out := make([]map[string]any, len(ds))
	for i, d := range ds {
		m := map[string]any{
			"id": d.ID, "webhook_id": d.WebhookID, "webhook_url": urls[d.WebhookID], "event_name": d.EventName, "resource_type": d.ResourceType,
			"resource_id": d.ResourceID, "status": d.Status, "attempts": d.Attempts, "next_attempt_at": nextAttempt(d),
			"last_status_code": d.LastStatusCode, "last_response": d.LastResponse, "last_error": d.LastError, "duration_ms": d.DurationMS,
			"created_at": ts(d.CreatedAt), "delivered_at": tsp(d.DeliveredAt),
		}
		if payload {
			m["payload"] = d.Payload
		}
		out[i] = m
	}
	return out
}

func nextAttempt(d db.WebhookDelivery) any {
	if d.Status != "pending" {
		return nil
	}
	return ts(d.NextAttemptAt)
}

func (h *Handler) listDeliveries(w http.ResponseWriter, r *http.Request, u *db.User) {
	p := parsePage(r)
	q := h.S.DB.Model(&db.WebhookDelivery{})
	if wid := qInt(r, "webhook_id"); wid > 0 {
		q = q.Where("webhook_id = ?", wid)
	}
	if s := r.URL.Query().Get("status"); s != "" {
		q = q.Where("status = ?", s)
	}
	if e := r.URL.Query().Get("event_name"); e != "" {
		q = q.Where("event_name = ?", e)
	}
	q = applySort(q, p.Sort, map[string]string{"created_at": "created_at", "id": "id"}, "id DESC")
	var rows []db.WebhookDelivery
	meta, err := paginate(q, p, &rows)
	if err != nil {
		h.failErr(w, err)
		return
	}
	h.respondList(w, h.deliveryDTOs(rows, false), meta)
}

func (h *Handler) getDelivery(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	var d db.WebhookDelivery
	if err := h.S.DB.First(&d, id).Error; err != nil {
		h.failErr(w, err)
		return
	}
	h.ok(w, h.deliveryDTOs([]db.WebhookDelivery{d}, true)[0])
}

func (h *Handler) redeliver(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	d, err := h.D.Redeliver(r.Context(), id)
	if err != nil {
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "webhook.redeliver", "webhook_delivery", id, nil)
	h.ok(w, h.deliveryDTOs([]db.WebhookDelivery{*d}, true)[0])
}

// ---------------------------------------------------------------- stripe events

func stripeEventDTO(e db.StripeEvent, payload bool) map[string]any {
	m := map[string]any{
		"id": e.ID, "type": e.Type, "account": e.Account, "livemode": e.Livemode, "status": e.Status, "attempts": e.Attempts,
		"error": e.Error, "received_at": ts(e.ReceivedAt), "processed_at": tsp(e.ProcessedAt),
	}
	if payload {
		m["payload"] = e.Payload
	}
	return m
}

func (h *Handler) listStripeEvents(w http.ResponseWriter, r *http.Request, u *db.User) {
	p := parsePage(r)
	q := h.S.DB.Model(&db.StripeEvent{})
	if s := r.URL.Query().Get("status"); s != "" {
		q = q.Where("status = ?", s)
	}
	if t := r.URL.Query().Get("type"); t != "" {
		q = q.Where("type = ?", t)
	}
	if p.Q != "" {
		q = q.Where("(LOWER(id) LIKE ? ESCAPE '\\' OR LOWER(type) LIKE ? ESCAPE '\\' OR LOWER(error) LIKE ? ESCAPE '\\')", like(p.Q), like(p.Q), like(p.Q))
	}
	q = applySort(q, p.Sort, map[string]string{"received_at": "received_at", "type": "type"}, "received_at DESC")
	var rows []db.StripeEvent
	meta, err := paginate(q, p, &rows)
	if err != nil {
		h.failErr(w, err)
		return
	}
	out := make([]map[string]any, len(rows))
	for i, e := range rows {
		out[i] = stripeEventDTO(e, false)
	}
	h.respondList(w, out, meta)
}

func (h *Handler) getStripeEvent(w http.ResponseWriter, r *http.Request, u *db.User) {
	var e db.StripeEvent
	if err := h.S.DB.First(&e, "id = ?", r.PathValue("id")).Error; err != nil {
		h.failErr(w, err)
		return
	}
	h.ok(w, stripeEventDTO(e, true))
}

func (h *Handler) replayStripeEvent(w http.ResponseWriter, r *http.Request, u *db.User) {
	rec, err := h.S.ReplayStripeEvent(r.Context(), r.PathValue("id"))
	if err != nil {
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "stripe_event.replay", "stripe_event", rec.ID, map[string]any{"status": rec.Status})
	h.ok(w, stripeEventDTO(*rec, true))
}

// ---------------------------------------------------------------- API keys

func (h *Handler) apiKeyDTO(k db.APIKey, owner string) map[string]any {
	return map[string]any{
		"id": k.ID, "name": k.Name, "prefix": k.Prefix, "test_mode": k.TestMode, "owner_email": owner,
		"last_used_at": tsp(k.LastUsedAt), "expires_at": tsp(k.ExpiresAt), "revoked_at": tsp(k.RevokedAt), "created_at": ts(k.CreatedAt),
	}
}

func (h *Handler) listAPIKeys(w http.ResponseWriter, r *http.Request, u *db.User) {
	p := parsePage(r)
	q := applySort(h.S.DB.Model(&db.APIKey{}), p.Sort, map[string]string{"created_at": "created_at", "last_used_at": "last_used_at"}, "id DESC")
	var rows []db.APIKey
	meta, err := paginate(q, p, &rows)
	if err != nil {
		h.failErr(w, err)
		return
	}
	owners := map[int64]string{}
	var us []db.User
	h.S.DB.Find(&us)
	for _, x := range us {
		owners[x.ID] = x.Email
	}
	out := make([]map[string]any, len(rows))
	for i, k := range rows {
		out[i] = h.apiKeyDTO(k, owners[k.UserID])
	}
	h.respondList(w, out, meta)
}

func (h *Handler) createAPIKey(w http.ResponseWriter, r *http.Request, u *db.User) {
	b, ok := h.readBody(w, r)
	if !ok {
		return
	}
	name, _ := b.str("name")
	if name == "" {
		h.invalid(w, "name", "Give the key a name.")
		return
	}
	tm, _ := b.boolean("test_mode")
	var exp *time.Time
	if s, ok := b.str("expires_at"); ok && s != "" {
		t, err := parseRFC3339(s)
		if err != nil {
			h.invalid(w, "expires_at", "Invalid date-time.")
			return
		}
		exp = t
	}
	k, token, err := auth.NewAPIKey(h.S.DB, u.ID, name, tm, exp)
	if err != nil {
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "api_key.create", "api_key", k.ID, map[string]any{"name": name})
	dto := h.apiKeyDTO(*k, u.Email)
	dto["token"] = token
	h.created(w, dto)
}

func (h *Handler) revokeAPIKey(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	res := h.S.DB.Model(&db.APIKey{}).Where("id = ? AND revoked_at IS NULL", id).Update("revoked_at", db.Now())
	if res.Error != nil {
		h.failErr(w, res.Error)
		return
	}
	if res.RowsAffected == 0 {
		h.fail(w, http.StatusNotFound, "not_found", "API key not found or already revoked.", nil)
		return
	}
	h.audit(r, u, "api_key.revoke", "api_key", id, nil)
	noContent(w)
}

// ---------------------------------------------------------------- users

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request, u *db.User) {
	p := parsePage(r)
	q := h.S.DB.Model(&db.User{})
	if p.Q != "" {
		q = q.Where("(LOWER(email) LIKE ? ESCAPE '\\' OR LOWER(name) LIKE ? ESCAPE '\\')", like(p.Q), like(p.Q))
	}
	q = applySort(q, p.Sort, map[string]string{"created_at": "created_at", "email": "email"}, "id")
	var rows []db.User
	meta, err := paginate(q, p, &rows)
	if err != nil {
		h.failErr(w, err)
		return
	}
	out := make([]map[string]any, len(rows))
	for i, x := range rows {
		out[i] = userDTO(x)
	}
	h.respondList(w, out, meta)
}

func (h *Handler) countOwners(excluding int64) int64 {
	var n int64
	h.S.DB.Model(&db.User{}).Where("role = ? AND disabled = ? AND id <> ?", auth.RoleOwner, false, excluding).Count(&n)
	return n
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request, u *db.User) {
	b, ok := h.readBody(w, r)
	if !ok {
		return
	}
	email, _ := b.str("email")
	if !strings.Contains(email, "@") {
		h.invalid(w, "email", "Enter a valid email address.")
		return
	}
	role, _ := b.str("role")
	if !auth.ValidRole(role) {
		h.invalid(w, "role", "The role must be owner, admin or viewer.")
		return
	}
	nu := db.User{Email: email, Role: role, CreatedAt: db.Now(), UpdatedAt: db.Now()}
	nu.Name, _ = b.str("name")
	if pw, ok := b["password"].(string); ok && pw != "" {
		if len(pw) < 10 {
			h.invalid(w, "password", "Use at least 10 characters.")
			return
		}
		hash, err := auth.HashPassword(pw)
		if err != nil {
			h.failErr(w, err)
			return
		}
		nu.PasswordHash = hash
	}
	if s, ok := b.str("stripe_account_id"); ok && s != "" {
		nu.StripeAccountID = &s
	}
	if nu.PasswordHash == "" && nu.StripeAccountID == nil {
		h.invalid(w, "password", "Set a password or a Stripe account id so the user can sign in.")
		return
	}
	if err := h.S.DB.Create(&nu).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			h.invalid(w, "email", "That email (or Stripe account) is already used.")
			return
		}
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "user.create", "user", nu.ID, map[string]any{"email": nu.Email, "role": nu.Role})
	h.created(w, userDTO(nu))
}

func (h *Handler) updateUser(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	var t db.User
	if err := h.S.DB.First(&t, id).Error; err != nil {
		h.failErr(w, err)
		return
	}
	b, ok := h.readBody(w, r)
	if !ok {
		return
	}
	if s, ok := b.str("name"); ok {
		t.Name = s
	}
	if s, ok := b.str("role"); ok {
		if !auth.ValidRole(s) {
			h.invalid(w, "role", "The role must be owner, admin or viewer.")
			return
		}
		if t.Role == auth.RoleOwner && s != auth.RoleOwner && h.countOwners(t.ID) == 0 {
			h.fail(w, http.StatusConflict, "conflict", "There must be at least one owner.", nil)
			return
		}
		t.Role = s
	}
	if d, ok := b.boolean("disabled"); ok {
		if d && t.Role == auth.RoleOwner && h.countOwners(t.ID) == 0 {
			h.fail(w, http.StatusConflict, "conflict", "You cannot disable the last owner.", nil)
			return
		}
		t.Disabled = d
	}
	if b.has("stripe_account_id") {
		if s, ok := b.str("stripe_account_id"); ok && s != "" {
			t.StripeAccountID = &s
		} else {
			t.StripeAccountID = nil
			t.StripeAccountLabel = ""
		}
	}
	if pw, ok := b["password"].(string); ok && pw != "" {
		if len(pw) < 10 {
			h.invalid(w, "password", "Use at least 10 characters.")
			return
		}
		hash, err := auth.HashPassword(pw)
		if err != nil {
			h.failErr(w, err)
			return
		}
		t.PasswordHash = hash
		auth.DestroyUserSessions(h.S.DB, t.ID)
	}
	t.UpdatedAt = db.Now()
	if err := h.S.DB.Save(&t).Error; err != nil {
		h.failErr(w, err)
		return
	}
	if t.Disabled {
		auth.DestroyUserSessions(h.S.DB, t.ID)
	}
	h.audit(r, u, "user.update", "user", t.ID, nil)
	h.ok(w, userDTO(t))
}

func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	if id == u.ID {
		h.fail(w, http.StatusConflict, "conflict", "You cannot delete your own account.", nil)
		return
	}
	var t db.User
	if err := h.S.DB.First(&t, id).Error; err != nil {
		h.failErr(w, err)
		return
	}
	if t.Role == auth.RoleOwner && h.countOwners(t.ID) == 0 {
		h.fail(w, http.StatusConflict, "conflict", "You cannot delete the last owner.", nil)
		return
	}
	err := h.S.DB.Transaction(func(tx *gorm.DB) error {
		tx.Where("user_id = ?", id).Delete(&db.AdminSession{})
		tx.Model(&db.APIKey{}).Where("user_id = ? AND revoked_at IS NULL", id).Update("revoked_at", db.Now())
		return tx.Delete(&db.User{}, id).Error
	})
	if err != nil {
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "user.delete", "user", id, map[string]any{"email": t.Email})
	noContent(w)
}

// ---------------------------------------------------------------- audit + email logs

func (h *Handler) listAudit(w http.ResponseWriter, r *http.Request, u *db.User) {
	p := parsePage(r)
	q := h.S.DB.Model(&db.AuditLog{})
	for col, param := range map[string]string{"action": "action", "user_email": "user_email", "resource_type": "resource_type"} {
		if v := r.URL.Query().Get(param); v != "" {
			q = q.Where(col+" = ?", v)
		}
	}
	if p.Q != "" {
		q = q.Where("(LOWER(action) LIKE ? ESCAPE '\\' OR LOWER(user_email) LIKE ? ESCAPE '\\' OR resource_id = ?)", like(p.Q), like(p.Q), p.Q)
	}
	q = q.Order("id DESC")
	var rows []db.AuditLog
	meta, err := paginate(q, p, &rows)
	if err != nil {
		h.failErr(w, err)
		return
	}
	out := make([]map[string]any, len(rows))
	for i, a := range rows {
		out[i] = map[string]any{"id": a.ID, "user_email": a.UserEmail, "action": a.Action, "resource_type": a.ResourceType, "resource_id": a.ResourceID, "details": a.Details.V, "ip": a.IP, "created_at": ts(a.CreatedAt)}
	}
	h.respondList(w, out, meta)
}

func emailDTO(e db.EmailLog) map[string]any {
	return map[string]any{"id": e.ID, "order_id": e.OrderID, "to": e.To, "subject": e.Subject, "template": e.Template, "status": e.Status, "error": e.Error, "created_at": ts(e.CreatedAt)}
}

func (h *Handler) listEmailLogs(w http.ResponseWriter, r *http.Request, u *db.User) {
	p := parsePage(r)
	q := h.S.DB.Model(&db.EmailLog{})
	if s := r.URL.Query().Get("status"); s != "" {
		q = q.Where("status = ?", s)
	}
	if p.Q != "" {
		q = q.Where("(LOWER(to_addr) LIKE ? ESCAPE '\\' OR LOWER(subject) LIKE ? ESCAPE '\\')", like(p.Q), like(p.Q))
	}
	q = q.Order("id DESC")
	var rows []db.EmailLog
	meta, err := paginate(q, p, &rows)
	if err != nil {
		h.failErr(w, err)
		return
	}
	out := make([]map[string]any, len(rows))
	for i, e := range rows {
		out[i] = emailDTO(e)
	}
	h.respondList(w, out, meta)
}

func (h *Handler) testEmail(w http.ResponseWriter, r *http.Request, u *db.User) {
	var in struct {
		To string `json:"to"`
	}
	if !h.decode(w, r, &in) {
		return
	}
	in.To = strings.TrimSpace(in.To)
	if !strings.Contains(in.To, "@") {
		h.invalid(w, "to", "Enter a valid email address.")
		return
	}
	if !h.S.Mail.Configured() {
		h.fail(w, http.StatusUnprocessableEntity, "validation_failed", "SMTP is not configured (Settings → Email).", nil)
		return
	}
	html, text, _ := mail.RenderMagicLink(mail.MagicLinkData{StoreName: "Digital Fulfillment", Link: h.S.URLs.Base + "/admin"})
	if err := h.S.Mail.Send(r.Context(), mail.Message{To: in.To, Subject: "Test email from Digital Fulfillment", HTML: html, Text: text, Template: "test"}); err != nil {
		h.fail(w, http.StatusBadGateway, "internal", "SMTP error: "+err.Error(), nil)
		return
	}
	h.audit(r, u, "email.test", "email", in.To, nil)
	noContent(w)
}

var _ = strconv.Itoa
