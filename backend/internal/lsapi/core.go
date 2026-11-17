// Package lsapi implements the Lemon Squeezy compatible HTTP API:
// the public License API (/v1/licenses/*) and the JSON:API resource API.
package lsapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"fulfillment/internal/crypt"
	"fulfillment/internal/db"
	"fulfillment/internal/httpx"
	"fulfillment/internal/ratelimit"
	"fulfillment/internal/resource"
	"fulfillment/internal/service"
)

const contentType = "application/vnd.api+json"

type API struct {
	S       *service.Svc
	limiter *ratelimit.Limiter
}

func New(s *service.Svc) *API {
	return &API{S: s, limiter: ratelimit.New(time.Minute)}
}

// Auth is the authenticated API key context.
type Auth struct {
	Key      db.APIKey
	User     db.User
	TestMode bool
}

type authedHandler func(w http.ResponseWriter, r *http.Request, au *Auth)

// ---------------------------------------------------------------- output

type jsonapiVersion struct {
	Version string `json:"version"`
}

var v10 = jsonapiVersion{Version: "1.0"}

type singleDoc struct {
	JSONAPI  jsonapiVersion      `json:"jsonapi"`
	Links    *resource.SelfLink  `json:"links,omitempty"`
	Data     resource.Resource   `json:"data"`
	Included []resource.Resource `json:"included,omitempty"`
}

type pageMeta struct {
	CurrentPage int `json:"currentPage"`
	From        int `json:"from"`
	LastPage    int `json:"lastPage"`
	PerPage     int `json:"perPage"`
	To          int `json:"to"`
	Total       int `json:"total"`
}

type listDoc struct {
	Meta     map[string]any      `json:"meta"`
	JSONAPI  jsonapiVersion      `json:"jsonapi"`
	Links    map[string]string   `json:"links"`
	Data     []resource.Resource `json:"data"`
	Included []resource.Resource `json:"included,omitempty"`
}

func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type errObj struct {
	Detail string         `json:"detail"`
	Source map[string]any `json:"source,omitempty"`
	Status string         `json:"status"`
	Title  string         `json:"title"`
}

func fail(w http.ResponseWriter, status int, title, detail, pointer string) {
	e := errObj{Detail: detail, Status: strconv.Itoa(status), Title: title}
	if pointer != "" {
		e.Source = map[string]any{"pointer": pointer}
	}
	write(w, status, map[string]any{"jsonapi": v10, "errors": []errObj{e}})
}

// failErr maps service errors to JSON:API errors.
func (a *API) failErr(w http.ResponseWriter, err error) {
	if e, ok := service.AsError(err); ok {
		fail(w, e.Status, e.Title, e.Detail, e.Pointer)
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		fail(w, http.StatusNotFound, "Not Found", "The requested resource could not be found.", "")
		return
	}
	a.S.Log.Error("lsapi error", "err", err)
	fail(w, http.StatusInternalServerError, "Internal Server Error", "An unexpected error occurred.", "")
}

// ---------------------------------------------------------------- auth

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// Authenticate resolves an API key token to its owner. Exported for reuse.
func (a *API) authenticate(ctx context.Context, token string) (*Auth, bool) {
	if token == "" {
		return nil, false
	}
	var k db.APIKey
	err := a.S.DB.WithContext(ctx).Where("key_hash = ? AND revoked_at IS NULL", crypt.SHA256Hex(token)).First(&k).Error
	if err != nil {
		return nil, false
	}
	if k.ExpiresAt != nil && k.ExpiresAt.Before(time.Now()) {
		return nil, false
	}
	var u db.User
	if err := a.S.DB.WithContext(ctx).First(&u, k.UserID).Error; err != nil || u.Disabled {
		return nil, false
	}
	if k.LastUsedAt == nil || time.Since(*k.LastUsedAt) > time.Minute {
		now := db.Now()
		a.S.DB.Model(&db.APIKey{}).Where("id = ?", k.ID).Update("last_used_at", now)
	}
	return &Auth{Key: k, User: u, TestMode: k.TestMode}, true
}

func (a *API) auth(next authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		au, ok := a.authenticate(r.Context(), bearer(r))
		if !ok {
			w.Header().Set("WWW-Authenticate", "Bearer")
			fail(w, http.StatusUnauthorized, "Unauthorized", "Unauthenticated.", "")
			return
		}
		limit := a.S.Cfg.APIRateLimit
		allowed, remaining, reset := a.limiter.Allow("api:"+strconv.FormatInt(au.Key.ID, 10), limit)
		w.Header().Set("X-Ratelimit-Limit", strconv.Itoa(limit))
		w.Header().Set("X-Ratelimit-Remaining", strconv.Itoa(remaining))
		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(reset.Seconds())+1))
			fail(w, http.StatusTooManyRequests, "Too Many Requests", "Too Many Attempts.", "")
			return
		}
		next(w, r, au)
	}
}

// ---------------------------------------------------------------- request parsing

func pageParams(r *http.Request) (number, size int) {
	q := r.URL.Query()
	number, size = 1, 10
	if n, err := strconv.Atoi(q.Get("page[number]")); err == nil && n > 0 {
		number = n
	}
	if n, err := strconv.Atoi(q.Get("page[size]")); err == nil && n > 0 {
		size = n
	}
	if size > 100 {
		size = 100
	}
	return
}

var sortColumns = map[string]string{
	"id": "id", "createdAt": "created_at", "created_at": "created_at", "updatedAt": "updated_at", "updated_at": "updated_at",
	"name": "name", "email": "email", "status": "status", "sort": "sort", "order_number": "order_number", "orderNumber": "order_number",
}

func applySort(q *gorm.DB, param string) (*gorm.DB, string) {
	if param == "" {
		return q.Order("id DESC"), "-id"
	}
	applied := 0
	for _, part := range strings.Split(param, ",") {
		part = strings.TrimSpace(part)
		dir := "ASC"
		if strings.HasPrefix(part, "-") {
			dir, part = "DESC", part[1:]
		}
		if col, ok := sortColumns[part]; ok {
			q = q.Order(col + " " + dir)
			applied++
		}
	}
	if applied == 0 {
		return q.Order("id DESC"), "-id"
	}
	return q.Order("id DESC"), param
}

func pageLink(r *http.Request, n, size int, sort string) string {
	q := url.Values{}
	for k, vs := range r.URL.Query() {
		if strings.HasPrefix(k, "page[") || k == "sort" {
			continue
		}
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	q.Set("page[number]", strconv.Itoa(n))
	q.Set("page[size]", strconv.Itoa(size))
	q.Set("sort", sort)
	return "" + q.Encode()
}

func filterParams(r *http.Request) map[string]string {
	out := map[string]string{}
	for k, vs := range r.URL.Query() {
		if strings.HasPrefix(k, "filter[") && strings.HasSuffix(k, "]") && len(vs) > 0 {
			out[k[7:len(k)-1]] = vs[0]
		}
	}
	return out
}

func includeParam(r *http.Request) []string {
	var out []string
	for _, p := range strings.Split(r.URL.Query().Get("include"), ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

// ---------------------------------------------------------------- generic specs

type spec[M any] struct {
	typ      string
	scope    func(q *gorm.DB, au *Auth) *gorm.DB
	filters  map[string]func(q *gorm.DB, v string) *gorm.DB
	build    func(a *API, au *Auth, ms []M) []resource.Resource
	includes map[string]func(a *API, au *Auth, ms []M) []resource.Resource
	idOf     func(M) int64
}

func eqFilter(col string) func(q *gorm.DB, v string) *gorm.DB {
	return func(q *gorm.DB, v string) *gorm.DB { return q.Where(col+" = ?", v) }
}

func boolFilter(col string) func(q *gorm.DB, v string) *gorm.DB {
	return func(q *gorm.DB, v string) *gorm.DB {
		b := v == "true" || v == "1"
		return q.Where(col+" = ?", b)
	}
}

func testModeScope(q *gorm.DB, au *Auth) *gorm.DB { return q.Where("test_mode = ?", au.TestMode) }

func dedupe(in []resource.Resource) []resource.Resource {
	seen := map[string]bool{}
	var out []resource.Resource
	for _, r := range in {
		k := r.Type + ":" + r.ID
		if !seen[k] {
			seen[k] = true
			out = append(out, r)
		}
	}
	return out
}

func listOf[M any](a *API, sp spec[M], extra func(q *gorm.DB, r *http.Request) *gorm.DB) authedHandler {
	return func(w http.ResponseWriter, r *http.Request, au *Auth) {
		q := a.S.DB.WithContext(r.Context()).Model(new(M))
		if sp.scope != nil {
			q = sp.scope(q, au)
		}
		if extra != nil {
			q = extra(q, r)
		}
		for k, v := range filterParams(r) {
			if f, ok := sp.filters[k]; ok {
				q = f(q, v)
			}
		}
		var total int64
		if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			a.failErr(w, err)
			return
		}
		number, size := pageParams(r)
		q, sortStr := applySort(q, r.URL.Query().Get("sort"))
		var rows []M
		if err := q.Offset((number - 1) * size).Limit(size).Find(&rows).Error; err != nil {
			a.failErr(w, err)
			return
		}
		last := int((total + int64(size) - 1) / int64(size))
		if last < 1 {
			last = 1
		}
		from, to := 0, 0
		if len(rows) > 0 {
			from = (number-1)*size + 1
			to = from + len(rows) - 1
		}
		base := a.S.URLs.Base + r.URL.Path + "?"
		links := map[string]string{
			"first": base + pageLink(r, 1, size, sortStr),
			"last":  base + pageLink(r, last, size, sortStr),
		}
		if number < last {
			links["next"] = base + pageLink(r, number+1, size, sortStr)
		}
		if number > 1 {
			links["prev"] = base + pageLink(r, number-1, size, sortStr)
		}
		data := sp.build(a, au, rows)
		if data == nil {
			data = []resource.Resource{}
		}
		doc := listDoc{
			Meta:    map[string]any{"page": pageMeta{CurrentPage: number, From: from, LastPage: last, PerPage: size, To: to, Total: int(total)}},
			JSONAPI: v10, Links: links, Data: data,
		}
		var inc []resource.Resource
		for _, name := range includeParam(r) {
			if f, ok := sp.includes[name]; ok {
				inc = append(inc, f(a, au, rows)...)
			}
		}
		doc.Included = dedupe(inc)
		write(w, http.StatusOK, doc)
	}
}

// loadOne fetches a single scoped model by path id.
func loadOne[M any](a *API, sp spec[M], r *http.Request, au *Auth, id int64) (*M, error) {
	var m M
	q := a.S.DB.WithContext(r.Context()).Model(new(M))
	if sp.scope != nil {
		q = sp.scope(q, au)
	}
	if err := q.Where("id = ?", id).First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func respondOne[M any](a *API, au *Auth, sp spec[M], w http.ResponseWriter, r *http.Request, m M, status int) {
	built := sp.build(a, au, []M{m})
	doc := singleDoc{JSONAPI: v10, Links: built[0].Links, Data: built[0]}
	var inc []resource.Resource
	for _, name := range includeParam(r) {
		if f, ok := sp.includes[name]; ok {
			inc = append(inc, f(a, au, []M{m})...)
		}
	}
	doc.Included = dedupe(inc)
	write(w, status, doc)
}

func getOf[M any](a *API, sp spec[M]) authedHandler {
	return func(w http.ResponseWriter, r *http.Request, au *Auth) {
		id, ok := pathID(r)
		if !ok {
			fail(w, http.StatusNotFound, "Not Found", "The requested resource could not be found.", "")
			return
		}
		m, err := loadOne(a, sp, r, au, id)
		if err != nil {
			a.failErr(w, err)
			return
		}
		respondOne(a, au, sp, w, r, *m, http.StatusOK)
	}
}

// ---------------------------------------------------------------- request bodies (JSON:API documents)

type docIn struct {
	Type          string
	ID            string
	Attrs         attrMap
	Relationships map[string]relRef
}

type relRef struct{ Type, ID string }

type attrMap map[string]any

func (m attrMap) has(k string) bool { _, ok := m[k]; return ok }

func (m attrMap) str(k string) (string, bool) {
	v, ok := m[k]
	if !ok || v == nil {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func (m attrMap) num(k string) (float64, bool) {
	v, ok := m[k]
	if !ok || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	}
	return 0, false
}

func (m attrMap) boolean(k string) (bool, bool) {
	v, ok := m[k]
	if !ok || v == nil {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

func readDoc(r *http.Request) (*docIn, error) {
	var raw struct {
		Data struct {
			Type          string         `json:"type"`
			ID            any            `json:"id"`
			Attributes    map[string]any `json:"attributes"`
			Relationships map[string]struct {
				Data *struct {
					Type string `json:"type"`
					ID   any    `json:"id"`
				} `json:"data"`
			} `json:"relationships"`
		} `json:"data"`
	}
	if err := httpx.ReadJSON(r, &raw, 1<<20); err != nil {
		return nil, service.Bad("Invalid JSON:API document: " + err.Error())
	}
	d := &docIn{Type: raw.Data.Type, ID: fmt.Sprint(valOr(raw.Data.ID)), Attrs: raw.Data.Attributes, Relationships: map[string]relRef{}}
	if d.Attrs == nil {
		d.Attrs = attrMap{}
	}
	for k, v := range raw.Data.Relationships {
		if v.Data != nil {
			d.Relationships[k] = relRef{Type: v.Data.Type, ID: fmt.Sprint(valOr(v.Data.ID))}
		}
	}
	return d, nil
}

func valOr(v any) any {
	if v == nil {
		return ""
	}
	return v
}

func (d *docIn) relID(name string) (int64, bool) {
	r, ok := d.Relationships[name]
	if !ok {
		return 0, false
	}
	id, err := strconv.ParseInt(r.ID, 10, 64)
	return id, err == nil
}

func parseTime(s string) (*time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000000Z", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			t = t.UTC()
			return &t, nil
		}
	}
	return nil, fmt.Errorf("invalid date-time %q", s)
}
