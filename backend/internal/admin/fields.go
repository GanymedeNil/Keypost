package admin

import (
	"net/http"
	"strconv"
	"strings"
	"unicode"
)

// body is a decoded JSON object that distinguishes absent keys from null.
type body map[string]any

func (h *Handler) readBody(w http.ResponseWriter, r *http.Request) (body, bool) {
	var b body
	if !h.decode(w, r, &b) {
		return nil, false
	}
	if b == nil {
		b = body{}
	}
	return b, true
}

func (b body) has(k string) bool { _, ok := b[k]; return ok }

func (b body) isNull(k string) bool { v, ok := b[k]; return ok && v == nil }

func (b body) str(k string) (string, bool) {
	v, ok := b[k]
	if !ok || v == nil {
		return "", false
	}
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x), true
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64), true
	}
	return "", false
}

func (b body) int(k string) (int64, bool) {
	v, ok := b[k]
	if !ok || v == nil {
		return 0, false
	}
	switch x := v.(type) {
	case float64:
		return int64(x), true
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
		return n, err == nil
	}
	return 0, false
}

func (b body) boolean(k string) (bool, bool) {
	v, ok := b[k]
	if !ok || v == nil {
		return false, false
	}
	switch x := v.(type) {
	case bool:
		return x, true
	case string:
		return x == "true" || x == "1", true
	}
	return false, false
}

func (b body) strList(k string) ([]string, bool) {
	raw, ok := b[k].([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(raw))
	for _, e := range raw {
		if s, ok := e.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out, true
}

func (b body) obj(k string) (body, bool) {
	m, ok := b[k].(map[string]any)
	return body(m), ok
}

func slugify(s string) string {
	var out []rune
	dash := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out = append(out, r)
			dash = false
		} else if !dash && len(out) > 0 {
			out = append(out, '-')
			dash = true
		}
	}
	res := strings.Trim(string(out), "-")
	if res == "" {
		res = "item"
	}
	return res
}
