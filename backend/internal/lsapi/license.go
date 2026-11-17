package lsapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

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
		ActivationUsage: k.InstancesCount, CreatedAt: resource.TS(k.CreatedAt), ExpiresAt: resource.TSP(k.ExpiresAt),
	}
}

func instOut(o service.LicenseOutcome) any {
	if o.Instance == nil {
		return nil
	}
	return instanceOut{ID: o.Instance.Identifier, Name: o.Instance.Name, CreatedAt: resource.TS(o.Instance.CreatedAt)}
}

func required(w http.ResponseWriter, okKey, field string) {
	httpx.JSON(w, http.StatusUnprocessableEntity, map[string]any{
		okKey: false, "error": "The " + field + " field is required.",
		"license_key": nil, "instance": nil, "meta": nil,
	})
}

func (a *API) licenseActivate(w http.ResponseWriter, r *http.Request) {
	p, ok := a.licenseParams(w, r, "activated")
	if !ok {
		return
	}
	key, name := p["license_key"], p["instance_name"]
	if key == "" {
		required(w, "activated", "license_key")
		return
	}
	if name == "" {
		required(w, "activated", "instance_name")
		return
	}
	o := a.S.ActivateLicense(r.Context(), key, name)
	body := map[string]any{"activated": o.OK, "error": nilIfEmpty(o.Err), "license_key": a.keyOut(o), "meta": o.Meta}
	if o.Key == nil {
		body["instance"] = nil
	} else if o.OK {
		body["instance"] = instOut(o)
	}
	httpx.JSON(w, o.HTTP, body)
}

func (a *API) licenseValidate(w http.ResponseWriter, r *http.Request) {
	p, ok := a.licenseParams(w, r, "valid")
	if !ok {
		return
	}
	key, instanceID := p["license_key"], p["instance_id"]
	if key == "" {
		required(w, "valid", "license_key")
		return
	}
	o := a.S.ValidateLicense(r.Context(), key, instanceID)
	httpx.JSON(w, o.HTTP, map[string]any{
		"valid": o.OK, "error": nilIfEmpty(o.Err), "license_key": a.keyOut(o), "instance": instOut(o), "meta": o.Meta,
	})
}

func (a *API) licenseDeactivate(w http.ResponseWriter, r *http.Request) {
	p, ok := a.licenseParams(w, r, "deactivated")
	if !ok {
		return
	}
	key, instanceID := p["license_key"], p["instance_id"]
	if key == "" {
		required(w, "deactivated", "license_key")
		return
	}
	if instanceID == "" {
		required(w, "deactivated", "instance_id")
		return
	}
	o := a.S.DeactivateLicense(r.Context(), key, instanceID)
	httpx.JSON(w, o.HTTP, map[string]any{
		"deactivated": o.OK, "error": nilIfEmpty(o.Err), "license_key": a.keyOut(o), "meta": o.Meta,
	})
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
