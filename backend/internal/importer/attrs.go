package importer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// attrs is a loosely typed attribute bag decoded from a resource object. Lemon
// Squeezy payloads (and hand-made exports) are not fully consistent: numbers
// may arrive as strings, booleans as 0/1 and any field may be null or missing.
// The accessors therefore never fail; they return zero values instead.
type attrs map[string]any

// lsObject is one Lemon Squeezy resource (JSON:API object, flat object or a
// row derived from orders.csv).
type lsObject struct {
	Type string
	ID   int64
	A    attrs
	csv  bool // derived from orders.csv: ID is allocated by the importer
}

func decodeAny(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

func parseObject(raw []byte) (lsObject, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return lsObject{}, fmt.Errorf("not a JSON object: %w", err)
	}
	if fields == nil {
		return lsObject{}, fmt.Errorf("null is not a JSON object")
	}
	return objectFromFields(fields)
}

func isNull(raw json.RawMessage) bool {
	s := bytes.TrimSpace(raw)
	return len(s) == 0 || string(s) == "null"
}

func objectFromFields(f map[string]json.RawMessage) (lsObject, error) {
	var o lsObject
	if v, ok := f["type"]; ok {
		_ = json.Unmarshal(v, &o.Type)
	}
	if v, ok := f["id"]; ok {
		if x, err := decodeAny(v); err == nil {
			o.ID, _ = toInt(x)
		}
	}
	if v, ok := f["attributes"]; ok && !isNull(v) {
		x, err := decodeAny(v)
		if err != nil {
			return o, fmt.Errorf("attributes: %w", err)
		}
		m, ok := x.(map[string]any)
		if !ok {
			return o, fmt.Errorf("attributes is not an object")
		}
		o.A = attrs(m)
		return o, nil
	}
	o.A = attrs{}
	for k, v := range f {
		switch k {
		case "id", "type", "relationships", "meta", "attributes":
			continue
		case "links":
			if !bytes.HasPrefix(bytes.TrimSpace(v), []byte("[")) {
				continue
			}
		}
		if x, err := decodeAny(v); err == nil {
			o.A[k] = x
		}
	}
	return o, nil
}

func toInt(v any) (int64, bool) {
	switch x := v.(type) {
	case json.Number:
		if n, err := x.Int64(); err == nil {
			return n, true
		}
		if f, err := x.Float64(); err == nil {
			return int64(math.Round(f)), true
		}
	case float64:
		return int64(math.Round(x)), true
	case int64:
		return x, true
	case int:
		return int64(x), true
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return 0, false
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n, true
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
			return int64(math.Round(f)), true
		}
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

func (a attrs) str(k string) string {
	switch v := a[k].(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	}
	return ""
}

// trim is str with surrounding whitespace removed (ids, emails, slugs).
func (a attrs) trim(k string) string { return strings.TrimSpace(a.str(k)) }

// strp returns nil for null / missing / empty values.
func (a attrs) strp(k string) *string {
	s := a.str(k)
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

func (a attrs) i64(k string) int64 { n, _ := toInt(a[k]); return n }

func (a attrs) i64p(k string) *int64 {
	n, ok := toInt(a[k])
	if !ok {
		return nil
	}
	return &n
}

func (a attrs) intv(k string) int { return int(a.i64(k)) }

func (a attrs) intp(k string) *int {
	n, ok := toInt(a[k])
	if !ok {
		return nil
	}
	v := int(n)
	return &v
}

func (a attrs) boolean(k string) bool {
	switch v := a[k].(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "y", "on":
			return true
		}
		return false
	case nil:
		return false
	}
	n, _ := toInt(a[k])
	return n != 0
}

// present reports whether the key exists with a non-null value.
func (a attrs) present(k string) bool { v, ok := a[k]; return ok && v != nil }

func (a attrs) obj(k string) attrs {
	if m, ok := a[k].(map[string]any); ok {
		return attrs(m)
	}
	return nil
}

func (a attrs) list(k string) []any {
	l, _ := a[k].([]any)
	return l
}

func (a attrs) tm(k string) *time.Time {
	s := strings.TrimSpace(a.str(k))
	if s == "" {
		return nil
	}
	if t, ok := parseTime(s); ok {
		return &t
	}
	return nil
}

var timeLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04:05.999999999 -0700",
	"2006-01-02 15:04:05.999999999 MST",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04",
	"2006-01-02T15:04Z07:00",
	"2006-01-02",
	"01/02/2006 15:04:05",
	"01/02/2006",
}

func parseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, l := range timeLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC(), true
		}
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 1e9 && n < 1e12 {
		return time.Unix(n, 0).UTC(), true
	}
	return time.Time{}, false
}

// when returns the timestamp or the fallback when absent.
func (a attrs) when(k string, fallback time.Time) time.Time {
	if t := a.tm(k); t != nil {
		return *t
	}
	return fallback
}

// dec returns a decimal as the string Lemon Squeezy would show (nil when null).
func (a attrs) dec(k string) *string {
	switch v := a[k].(type) {
	case nil:
		return nil
	case json.Number:
		s := v.String()
		return &s
	case float64:
		s := strconv.FormatFloat(v, 'f', -1, 64)
		return &s
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return &v
	}
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
