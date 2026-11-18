package lsapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"fulfillment/internal/httpx"
	"fulfillment/internal/resource"
	"fulfillment/internal/service"
)

// License API: public endpoints, form-encoded requests, plain JSON responses
// (not JSON:API), exactly as documented by Lemon Squeezy.

type licenseKeyOut struct {
	ID              int64   `json:"id"`
	Status          string  `json:"status"`
	Key             string  `json:"key"`
	ActivationLimit *int    `json:"activation_limit"`
	ActivationUsage int     `json:"activation_usage"`
	CreatedAt       string  `json:"created_at"`
	ExpiresAt       *string `json:"expires_at"`
	TestMode        bool    `json:"test_mode"`
}

type instanceOut struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

func (a *API) licenseParams(w http.ResponseWriter, r *http.Request, okKey string) (vals map[string]string, ok bool) {
	ip := httpx.ClientIP(r, a.S.Cfg.TrustProxyHeaders)
	limit := a.S.Cfg.LicenseRateLimit
	allowed, remaining, reset := a.limiter.Allow("lic:"+ip, limit)
	w.Header().Set("X-Ratelimit-Limit", strconv.Itoa(limit))
	w.Header().Set("X-Ratelimit-Remaining", strconv.Itoa(remaining))
	if !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(reset.Seconds())+1))
		httpx.JSON(w, http.StatusTooManyRequests, map[string]any{okKey: false, "error": "Too many requests. Please slow down."})
		return nil, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	vals = map[string]string{}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		for k, v := range body {
			if s, isStr := v.(string); isStr {
				vals[k] = s
			}
		}
	} else if err := r.ParseForm(); err == nil {
		for k := range r.Form {
			vals[k] = r.Form.Get(k)
		}
	}
	for k, v := range vals {
		vals[k] = strings.TrimSpace(v)
	}
	return vals, true
}

func (a *API) keyOut(o service.LicenseOutcome) any {
	if o.Key == nil {
		return nil
	}
	k := o.Key
	return licenseKeyOut{
		ID: k.ID, Status: resource.LicenseStatus(k, a.S.Res.Clock()), Key: k.Key, ActivationLimit: k.ActivationLimit,
		ActivationUsage: k.InstancesCount, CreatedAt: resource.TS(k.CreatedAt), ExpiresAt: resource.TSP(k.ExpiresAt), TestMode: k.TestMode,
	}
}

func instOut(o service.LicenseOutcome) any {
	if o.Instance == nil {
		return nil
	}
	return instanceOut{ID: o.Instance.Identifier, Name: o.Instance.Name, CreatedAt: resource.TS(o.Instance.CreatedAt)}
}

// fieldRule is one validation rule of a License API parameter. Lemon Squeezy
// is built on Laravel, so failures come back as 422 {"message", "errors"} with
// Laravel's wording ("The instance name field is required.").
type fieldRule struct {
	field    string
	required bool
	min, max int // in characters, 0 = no limit
}

func (f fieldRule) check(v string) string {
	label := strings.ReplaceAll(f.field, "_", " ")
	n := utf8.RuneCountInString(v)
	switch {
	case v == "":
		if f.required {
			return "The " + label + " field is required."
		}
	case f.min > 0 && n < f.min:
		return "The " + label + " field must be at least " + strconv.Itoa(f.min) + " characters."
	case f.max > 0 && n > f.max:
		return "The " + label + " field must not be greater than " + strconv.Itoa(f.max) + " characters."
	}
	return ""
}

// validateParams answers 422 and reports false when a parameter is invalid.
func validateParams(w http.ResponseWriter, vals map[string]string, rules ...fieldRule) bool {
	errs := map[string][]string{}
	var first string
	count := 0
	for _, r := range rules {
		if msg := r.check(vals[r.field]); msg != "" {
			errs[r.field] = []string{msg}
			if count == 0 {
				first = msg
			}
			count++
		}
	}
	if count == 0 {
		return true
	}
	message := first
	switch {
	case count == 2:
		message += " (and 1 more error)"
	case count > 2:
		message += " (and " + strconv.Itoa(count-1) + " more errors)"
	}
	httpx.JSON(w, http.StatusUnprocessableEntity, map[string]any{"message": message, "errors": errs})
	return false
}

func (a *API) licenseActivate(w http.ResponseWriter, r *http.Request) {
	p, ok := a.licenseParams(w, r, "activated")
	if !ok || !validateParams(w, p, fieldRule{field: "license_key", required: true}, fieldRule{field: "instance_name", required: true, min: 3, max: 255}) {
		return
	}
	o := a.S.ActivateLicense(r.Context(), p["license_key"], p["instance_name"])
	body := map[string]any{"activated": o.OK, "error": nilIfEmpty(o.Err)}
	if o.Key != nil {
		body["license_key"], body["meta"] = a.keyOut(o), o.Meta
		if o.OK {
			body["instance"] = instOut(o)
		}
	}
	httpx.JSON(w, o.HTTP, body)
}

func (a *API) licenseValidate(w http.ResponseWriter, r *http.Request) {
	p, ok := a.licenseParams(w, r, "valid")
	if !ok || !validateParams(w, p, fieldRule{field: "license_key", required: true}) {
		return
	}
	o := a.S.ValidateLicense(r.Context(), p["license_key"], p["instance_id"])
	body := map[string]any{"valid": o.OK, "error": nilIfEmpty(o.Err)}
	if o.Key != nil {
		body["license_key"], body["instance"], body["meta"] = a.keyOut(o), instOut(o), o.Meta
	}
	httpx.JSON(w, o.HTTP, body)
}

func (a *API) licenseDeactivate(w http.ResponseWriter, r *http.Request) {
	p, ok := a.licenseParams(w, r, "deactivated")
	if !ok || !validateParams(w, p, fieldRule{field: "license_key", required: true}, fieldRule{field: "instance_id", required: true}) {
		return
	}
	o := a.S.DeactivateLicense(r.Context(), p["license_key"], p["instance_id"])
	body := map[string]any{"deactivated": o.OK, "error": nilIfEmpty(o.Err)}
	if o.Key != nil {
		body["license_key"], body["meta"] = a.keyOut(o), o.Meta
	}
	httpx.JSON(w, o.HTTP, body)
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
