package importer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultLSBase   = "https://api.lemonsqueezy.com"
	lsPageSize      = 100
	lsMaxPageBytes  = 64 << 20
	lsMaxPages      = 1_000_000
	lsMaxRetries5xx = 4
	lsMaxRetries429 = 8
	lsMaxRetryAfter = 5 * time.Minute
)

// lsError is an HTTP error answer from the Lemon Squeezy API.
type lsError struct {
	Status int
	Detail string
}

func (e *lsError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("Lemon Squeezy API returned HTTP %d", e.Status)
	}
	return fmt.Sprintf("Lemon Squeezy API returned HTTP %d: %s", e.Status, e.Detail)
}

// lsClient is a small JSON:API client for the Lemon Squeezy REST API. It
// stays below the documented limit of 300 requests per minute, honours
// Retry-After on 429 and retries transient failures.
type lsClient struct {
	origin string // scheme://host
	prefix string // optional path prefix of the base URL
	key    string
	httpc  *http.Client
	sleep  func(context.Context, time.Duration) error
	limit  int
	window time.Duration
	times  []time.Time
	note   func(format string, args ...any)
}

func newLSClient(r *Runner, base, key string, note func(string, ...any)) (*lsClient, error) {
	if strings.TrimSpace(base) == "" {
		base = defaultLSBase
	}
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(base), "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("invalid Lemon Squeezy base URL %q", base)
	}
	c := &lsClient{
		origin: u.Scheme + "://" + u.Host, prefix: strings.TrimRight(u.EscapedPath(), "/"), key: key,
		httpc: r.client(), sleep: r.sleep, limit: r.rateLimit, window: r.rateWindow, note: note,
	}
	if c.sleep == nil {
		c.sleep = sleepCtx
	}
	return c, nil
}

// throttle blocks until another request fits into the rate window.
func (c *lsClient) throttle(ctx context.Context) error {
	if c.limit <= 0 || c.window <= 0 {
		return ctx.Err()
	}
	for {
		now := time.Now()
		cut := now.Add(-c.window)
		i := 0
		for i < len(c.times) && !c.times[i].After(cut) {
			i++
		}
		c.times = c.times[i:]
		if len(c.times) < c.limit {
			c.times = append(c.times, now)
			return ctx.Err()
		}
		wait := c.times[0].Add(c.window).Sub(now) + time.Millisecond
		if err := c.sleep(ctx, wait); err != nil {
			return err
		}
	}
}

func retryAfter(h http.Header, attempt int) time.Duration {
	if v := strings.TrimSpace(h.Get("Retry-After")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return clampWait(time.Duration(n) * time.Second)
		}
		if t, err := http.ParseTime(v); err == nil {
			return clampWait(time.Until(t))
		}
	}
	d := time.Second << min(attempt, 6)
	return clampWait(d)
}

func clampWait(d time.Duration) time.Duration {
	if d < 0 {
		return 0
	}
	if d > lsMaxRetryAfter {
		return lsMaxRetryAfter
	}
	return d
}

func apiErrorDetail(body []byte) string {
	var e struct {
		Errors []struct {
			Detail string `json:"detail"`
			Title  string `json:"title"`
		} `json:"errors"`
	}
	if json.Unmarshal(body, &e) == nil && len(e.Errors) > 0 {
		return firstNonEmpty(e.Errors[0].Detail, e.Errors[0].Title)
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// get requests path (with query) and returns the body of a 2xx answer.
func (c *lsClient) get(ctx context.Context, rel string) ([]byte, error) {
	full := c.origin + rel
	var n5xx, n429 int
	for {
		if err := c.throttle(ctx); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.api+json")
		req.Header.Set("Content-Type", "application/vnd.api+json")
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("User-Agent", "keypost-importer/1.0")
		resp, err := c.httpc.Do(req)
		var body []byte
		if err == nil {
			body, err = io.ReadAll(io.LimitReader(resp.Body, lsMaxPageBytes+1))
			resp.Body.Close()
			if err == nil && len(body) > lsMaxPageBytes {
				return nil, fmt.Errorf("response of %s is too large", rel)
			}
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if n5xx++; n5xx > lsMaxRetries5xx {
				return nil, fmt.Errorf("request %s failed: %w", rel, err)
			}
			if err := c.sleep(ctx, time.Duration(n5xx)*500*time.Millisecond); err != nil {
				return nil, err
			}
			continue
		}
		switch {
		case resp.StatusCode == http.StatusTooManyRequests:
			if n429++; n429 > lsMaxRetries429 {
				return nil, &lsError{Status: resp.StatusCode, Detail: "rate limit still exceeded after several retries"}
			}
			wait := retryAfter(resp.Header, n429)
			c.note("Lemon Squeezy rate limit reached (HTTP 429); waiting %s", wait.Round(time.Millisecond))
			if err := c.sleep(ctx, wait); err != nil {
				return nil, err
			}
		case resp.StatusCode >= 500:
			if n5xx++; n5xx > lsMaxRetries5xx {
				return nil, &lsError{Status: resp.StatusCode, Detail: apiErrorDetail(body)}
			}
			if err := c.sleep(ctx, time.Duration(n5xx)*500*time.Millisecond); err != nil {
				return nil, err
			}
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			return body, nil
		default:
			return nil, &lsError{Status: resp.StatusCode, Detail: apiErrorDetail(body)}
		}
	}
}

// ---------------------------------------------------------------- API source

// apiSource streams resources from the Lemon Squeezy REST API.
type apiSource struct {
	c             *lsClient
	storeIDs      []int64
	downloadFiles bool
	httpc         *http.Client
}

func (s *apiSource) has(string) bool { return true }

func (s *apiSource) openFile(ctx context.Context, f fileRef) (io.ReadCloser, error) {
	if !s.downloadFiles || f.URL == "" {
		return nil, errNoFileData
	}
	return httpFile(ctx, s.httpc, f.URL)
}

func (s *apiSource) stream(ctx context.Context, res string, h *streamHandler) error {
	ids := []int64{0}
	if storeFiltered[res] && len(s.storeIDs) > 0 {
		ids = s.storeIDs
	}
	total := 0
	for _, sid := range ids {
		rel := s.c.prefix + apiPath(res) + "?page%5Bnumber%5D=1&page%5Bsize%5D=" + strconv.Itoa(lsPageSize)
		if sid != 0 {
			rel += "&filter%5Bstore_id%5D=" + strconv.FormatInt(sid, 10)
		}
		if err := s.paginate(ctx, rel, h, &total); err != nil {
			return err
		}
	}
	return nil
}

type lsPage struct {
	data    []json.RawMessage
	next    string
	current int
	last    int
	total   int
	hasMeta bool
}

func decodePage(body []byte) (*lsPage, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("invalid JSON from the API: %w", err)
	}
	p := &lsPage{}
	if raw, ok := doc["data"]; ok && !isNull(raw) {
		if err := json.Unmarshal(raw, &p.data); err != nil {
			var one json.RawMessage
			if json.Unmarshal(raw, &one) != nil {
				return nil, fmt.Errorf("unexpected \"data\" in API response")
			}
			p.data = []json.RawMessage{one}
		}
	}
	if raw, ok := doc["links"]; ok {
		if v, err := decodeAny(raw); err == nil {
			if m, ok := v.(map[string]any); ok {
				if s, ok := m["next"].(string); ok {
					p.next = s
				}
			}
		}
	}
	if raw, ok := doc["meta"]; ok {
		if v, err := decodeAny(raw); err == nil {
			if m, ok := v.(map[string]any); ok {
				if pg := attrs(m).obj("page"); pg != nil {
					p.hasMeta = true
					p.current, p.last, p.total = pg.intv("currentPage"), pg.intv("lastPage"), pg.intv("total")
				}
			}
		}
	}
	return p, nil
}

// nextRel computes the path of the following page. The API's links.next is
// re-based onto the configured origin so a response can never redirect the
// API key to another host.
func (s *apiSource) nextRel(cur string, p *lsPage) string {
	if p.next != "" {
		if u, err := url.Parse(p.next); err == nil && u.Path != "" {
			path := u.EscapedPath()
			if s.c.prefix != "" && !strings.HasPrefix(path, s.c.prefix+"/") {
				path = s.c.prefix + path
			}
			if u.RawQuery != "" {
				path += "?" + u.RawQuery
			}
			return path
		}
	}
	if p.hasMeta && p.current > 0 && p.current < p.last {
		if u, err := url.Parse(cur); err == nil {
			q := u.Query()
			q.Set("page[number]", strconv.Itoa(p.current+1))
			return u.EscapedPath() + "?" + q.Encode()
		}
	}
	return ""
}

func (s *apiSource) paginate(ctx context.Context, rel string, h *streamHandler, total *int) error {
	seen := map[string]bool{}
	first := true
	for pages := 0; rel != "" && pages < lsMaxPages; pages++ {
		if seen[rel] {
			return nil
		}
		seen[rel] = true
		if err := ctx.Err(); err != nil {
			return err
		}
		body, err := s.c.get(ctx, rel)
		if err != nil {
			return err
		}
		p, err := decodePage(body)
		if err != nil {
			return err
		}
		if first {
			first = false
			if p.hasMeta {
				*total += p.total
				h.setTotal(*total)
			}
		}
		for _, raw := range p.data {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := h.emit(raw); err != nil {
				return err
			}
		}
		rel = s.nextRel(rel, p)
	}
	return nil
}

func isCtxErr(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
