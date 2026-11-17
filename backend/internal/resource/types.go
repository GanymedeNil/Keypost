// Package resource turns database rows into Lemon Squeezy shaped JSON:API
// resource objects. The same builders feed the REST API and outgoing webhooks.
package resource

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"fulfillment/internal/db"
	"fulfillment/internal/urls"
)

// Builder renders models. Create one per process.
type Builder struct {
	DB   *gorm.DB
	URLs urls.Builder
	Now  func() time.Time
}

// Clock returns the builder clock (UTC now unless overridden in tests).
func (b *Builder) Clock() time.Time { return b.now() }

func (b *Builder) now() time.Time {
	if b.Now != nil {
		return b.Now()
	}
	return time.Now().UTC()
}

type Resource struct {
	Type          string                  `json:"type"`
	ID            string                  `json:"id"`
	Attributes    any                     `json:"attributes"`
	Relationships map[string]Relationship `json:"relationships,omitempty"`
	Links         *SelfLink               `json:"links,omitempty"`
}

type SelfLink struct {
	Self string `json:"self"`
}

type Relationship struct {
	Links RelLinks `json:"links"`
}

type RelLinks struct {
	Related string `json:"related"`
	Self    string `json:"self"`
}

// TS formats a time the way Lemon Squeezy does (microseconds, Z suffix).
func TS(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000000Z") }

func TSP(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := TS(*t)
	return &s
}

func strOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func idStr(id int64) string { return fmt.Sprintf("%d", id) }

func (b *Builder) base() string { return b.URLs.Base + "/v1" }

func (b *Builder) rels(typ string, id int64, names ...string) map[string]Relationship {
	m := make(map[string]Relationship, len(names))
	for _, n := range names {
		m[n] = Relationship{Links: RelLinks{
			Related: fmt.Sprintf("%s/%s/%d/%s", b.base(), typ, id, n),
			Self:    fmt.Sprintf("%s/%s/%d/relationships/%s", b.base(), typ, id, n),
		}}
	}
	return m
}

func (b *Builder) self(typ string, id int64) *SelfLink {
	return &SelfLink{Self: fmt.Sprintf("%s/%s/%d", b.base(), typ, id)}
}

func (b *Builder) res(typ string, id int64, attrs any, rel ...string) Resource {
	return Resource{Type: typ, ID: idStr(id), Attributes: attrs, Relationships: b.rels(typ, id, rel...), Links: b.self(typ, id)}
}

// ---------------------------------------------------------------- money

var symbols = map[string]string{
	"USD": "$", "EUR": "€", "GBP": "£", "JPY": "¥", "CNY": "CN¥", "AUD": "A$", "CAD": "CA$",
	"INR": "₹", "KRW": "₩", "BRL": "R$", "CHF": "CHF ", "SEK": "SEK ", "NOK": "NOK ", "DKK": "DKK ",
}

var zeroDecimal = map[string]bool{"JPY": true, "KRW": true, "VND": true, "CLP": true, "UGX": true}

// FormatMoney renders minor units, e.g. 999,"USD" → "$9.99".
func FormatMoney(cents int64, currency string) string {
	cur := strings.ToUpper(currency)
	if cur == "" {
		cur = "USD"
	}
	sym, ok := symbols[cur]
	if !ok {
		sym = cur + " "
	}
	neg := ""
	if cents < 0 {
		neg = "-"
		cents = -cents
	}
	if zeroDecimal[cur] {
		return fmt.Sprintf("%s%s%s", neg, sym, commas(cents))
	}
	return fmt.Sprintf("%s%s%s.%02d", neg, sym, commas(cents/100), cents%100)
}

func commas(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var out []byte
	pre := len(s) % 3
	if pre > 0 {
		out = append(out, s[:pre]...)
	}
	for i := pre; i < len(s); i += 3 {
		if len(out) > 0 {
			out = append(out, ',')
		}
		out = append(out, s[i:i+3]...)
	}
	return string(out)
}

// FormatStatus: "partial_refund" → "Partial refund".
func FormatStatus(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "_", " ")
	return strings.ToUpper(s[:1]) + s[1:]
}

func HumanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	val := float64(n) / float64(div)
	suffix := []string{"KB", "MB", "GB", "TB"}[exp]
	if val >= 100 || val == float64(int64(val)) {
		return fmt.Sprintf("%.0f %s", val, suffix)
	}
	return fmt.Sprintf("%.1f %s", val, suffix)
}

// LicenseStatus computes the externally visible status of a license key.
func LicenseStatus(k *db.LicenseKey, now time.Time) string {
	switch {
	case k.Disabled:
		return "disabled"
	case k.ExpiresAt != nil && !k.ExpiresAt.After(now):
		return "expired"
	case k.InstancesCount > 0:
		return "active"
	default:
		return "inactive"
	}
}
