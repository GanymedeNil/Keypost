package stripe

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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

// Client talks to the Stripe REST API with a secret key.
type Client struct {
	Key        string
	APIVersion string
	Base       string
	HTTP       *http.Client
	// ConnectedAccount, when set, sends the Stripe-Account header.
	ConnectedAccount string
}

func NewClient(key, version, base string) *Client {
	if base == "" {
		base = "https://api.stripe.com"
	}
	return &Client{Key: key, APIVersion: version, Base: strings.TrimRight(base, "/"), HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Error is a Stripe API error.
type Error struct {
	Status  int
	Type    string
	Code    string
	Message string
	Param   string
}

func (e *Error) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("stripe: %s (%s, HTTP %d)", e.Message, e.Code, e.Status)
	}
	return fmt.Sprintf("stripe: %s (HTTP %d)", e.Message, e.Status)
}

var ErrNotConfigured = errors.New("stripe is not configured: set a secret key in Settings → Stripe")

// Params builds Stripe's form-encoded request bodies.
type Params struct{ v url.Values }

func P() *Params { return &Params{v: url.Values{}} }

func (p *Params) Set(k, v string) *Params       { p.v.Set(k, v); return p }
func (p *Params) Add(k, v string) *Params       { p.v.Add(k, v); return p }
func (p *Params) Int(k string, n int64) *Params { p.v.Set(k, strconv.FormatInt(n, 10)); return p }
func (p *Params) Bool(k string, b bool) *Params { p.v.Set(k, strconv.FormatBool(b)); return p }
func (p *Params) Expand(fields ...string) *Params {
	for i, f := range fields {
		p.v.Set(fmt.Sprintf("expand[%d]", i), f)
	}
	return p
}
func (p *Params) Meta(m map[string]string) *Params {
	for k, v := range m {
		p.v.Set("metadata["+k+"]", v)
	}
	return p
}
func (p *Params) values() url.Values {
	if p == nil {
		return url.Values{}
	}
	return p.v
}

func (c *Client) do(ctx context.Context, method, path string, p *Params, idem string, out any) error {
	if c.Key == "" {
		return ErrNotConfigured
	}
	var body io.Reader
	u := c.Base + path
	vals := p.values()
	if method == http.MethodGet || method == http.MethodDelete {
		if len(vals) > 0 {
			u += "?" + vals.Encode()
		}
	} else {
		body = strings.NewReader(vals.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.Key, "")
	if c.APIVersion != "" {
		req.Header.Set("Stripe-Version", c.APIVersion)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if c.ConnectedAccount != "" {
		req.Header.Set("Stripe-Account", c.ConnectedAccount)
	}
	if idem != "" {
		req.Header.Set("Idempotency-Key", idem)
	}
	req.Header.Set("User-Agent", "digital-fulfillment/1.0")
	var resp *http.Response
	for attempt := 0; ; attempt++ {
		resp, err = c.HTTP.Do(req)
		if err == nil && resp.StatusCode < 500 && resp.StatusCode != 429 {
			break
		}
		if attempt >= 2 || (method != http.MethodGet && idem == "") {
			if err != nil {
				return err
			}
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 400 * time.Millisecond):
		}
		if body != nil {
			req.Body, _ = req.GetBody()
		}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		var e struct {
			Error struct {
				Type    string `json:"type"`
				Code    string `json:"code"`
				Message string `json:"message"`
				Param   string `json:"param"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		msg := e.Error.Message
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
			if len(msg) > 200 {
				msg = msg[:200]
			}
		}
		return &Error{Status: resp.StatusCode, Type: e.Error.Type, Code: e.Error.Code, Message: msg, Param: e.Error.Param}
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("stripe: decode response: %w", err)
		}
	}
	return nil
}

// ---------------------------------------------------------------- objects

func (c *Client) Account(ctx context.Context) (*Account, error) {
	var a Account
	return &a, c.do(ctx, "GET", "/v1/account", nil, "", &a)
}

func (c *Client) RetrieveSession(ctx context.Context, id string) (*Session, error) {
	var s Session
	p := P().Expand("line_items.data.price.product", "customer", "payment_intent.latest_charge", "payment_intent.payment_method", "subscription.default_payment_method", "invoice")
	return &s, c.do(ctx, "GET", "/v1/checkout/sessions/"+id, p, "", &s)
}

func (c *Client) CreateSession(ctx context.Context, p *Params, idem string) (*Session, error) {
	var s Session
	return &s, c.do(ctx, "POST", "/v1/checkout/sessions", p, idem, &s)
}

func (c *Client) ExpireSession(ctx context.Context, id string) error {
	return c.do(ctx, "POST", "/v1/checkout/sessions/"+id+"/expire", nil, "", nil)
}

func (c *Client) RetrieveSubscription(ctx context.Context, id string) (*Subscription, error) {
	var s Subscription
	return &s, c.do(ctx, "GET", "/v1/subscriptions/"+id, P().Expand("default_payment_method"), "", &s)
}

func (c *Client) UpdateSubscription(ctx context.Context, id string, p *Params) (*Subscription, error) {
	var s Subscription
	return &s, c.do(ctx, "POST", "/v1/subscriptions/"+id, p, "", &s)
}

func (c *Client) CancelSubscriptionNow(ctx context.Context, id string) (*Subscription, error) {
	var s Subscription
	return &s, c.do(ctx, "DELETE", "/v1/subscriptions/"+id, nil, "", &s)
}

func (c *Client) RetrieveInvoice(ctx context.Context, id string) (*Invoice, error) {
	var i Invoice
	return &i, c.do(ctx, "GET", "/v1/invoices/"+id, P().Expand("payments"), "", &i)
}

func (c *Client) RetrievePaymentIntent(ctx context.Context, id string) (*PaymentIntent, error) {
	var pi PaymentIntent
	return &pi, c.do(ctx, "GET", "/v1/payment_intents/"+id, P().Expand("latest_charge", "payment_method"), "", &pi)
}

func (c *Client) RetrieveCharge(ctx context.Context, id string) (*Charge, error) {
	var ch Charge
	return &ch, c.do(ctx, "GET", "/v1/charges/"+id, nil, "", &ch)
}

func (c *Client) RetrieveCustomer(ctx context.Context, id string) (*Customer, error) {
	var cu Customer
	return &cu, c.do(ctx, "GET", "/v1/customers/"+id, nil, "", &cu)
}

func (c *Client) CreateCustomer(ctx context.Context, p *Params) (*Customer, error) {
	var cu Customer
	return &cu, c.do(ctx, "POST", "/v1/customers", p, "", &cu)
}

func (c *Client) ListCustomersByEmail(ctx context.Context, email string) ([]Customer, error) {
	var l list[Customer]
	err := c.do(ctx, "GET", "/v1/customers", P().Set("email", email).Int("limit", 100), "", &l)
	return l.Data, err
}

func (c *Client) CreateRefund(ctx context.Context, p *Params, idem string) (*Refund, error) {
	var r Refund
	return &r, c.do(ctx, "POST", "/v1/refunds", p, idem, &r)
}

func (c *Client) CreatePortalSession(ctx context.Context, customerID, returnURL string) (*BillingPortalSession, error) {
	var s BillingPortalSession
	p := P().Set("customer", customerID)
	if returnURL != "" {
		p.Set("return_url", returnURL)
	}
	return &s, c.do(ctx, "POST", "/v1/billing_portal/sessions", p, "", &s)
}

// ListProducts returns every product (auto-paginated, capped at 1000).
func (c *Client) ListProducts(ctx context.Context, activeOnly bool) ([]Product, error) {
	var all []Product
	after := ""
	for len(all) < 1000 {
		p := P().Int("limit", 100)
		if activeOnly {
			p.Bool("active", true)
		}
		if after != "" {
			p.Set("starting_after", after)
		}
		var l list[Product]
		if err := c.do(ctx, "GET", "/v1/products", p, "", &l); err != nil {
			return all, err
		}
		all = append(all, l.Data...)
		if !l.HasMore || len(l.Data) == 0 {
			break
		}
		after = l.Data[len(l.Data)-1].ID
	}
	return all, nil
}

func (c *Client) ListPrices(ctx context.Context, activeOnly bool) ([]Price, error) {
	var all []Price
	after := ""
	for len(all) < 3000 {
		p := P().Int("limit", 100)
		if activeOnly {
			p.Bool("active", true)
		}
		if after != "" {
			p.Set("starting_after", after)
		}
		var l list[Price]
		if err := c.do(ctx, "GET", "/v1/prices", p, "", &l); err != nil {
			return all, err
		}
		all = append(all, l.Data...)
		if !l.HasMore || len(l.Data) == 0 {
			break
		}
		after = l.Data[len(l.Data)-1].ID
	}
	return all, nil
}

func (c *Client) ListWebhookEndpoints(ctx context.Context) ([]WebhookEndpoint, error) {
	var l list[WebhookEndpoint]
	err := c.do(ctx, "GET", "/v1/webhook_endpoints", P().Int("limit", 100), "", &l)
	return l.Data, err
}

func (c *Client) CreateWebhookEndpoint(ctx context.Context, endpointURL string, events []string) (*WebhookEndpoint, error) {
	p := P().Set("url", endpointURL).Set("description", "Digital Fulfillment")
	for i, e := range events {
		p.Set(fmt.Sprintf("enabled_events[%d]", i), e)
	}
	var w WebhookEndpoint
	return &w, c.do(ctx, "POST", "/v1/webhook_endpoints", p, "", &w)
}

func (c *Client) UpdateWebhookEndpoint(ctx context.Context, id string, events []string) (*WebhookEndpoint, error) {
	p := P()
	for i, e := range events {
		p.Set(fmt.Sprintf("enabled_events[%d]", i), e)
	}
	var w WebhookEndpoint
	return &w, c.do(ctx, "POST", "/v1/webhook_endpoints/"+id, p, "", &w)
}

func (c *Client) DeleteWebhookEndpoint(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/v1/webhook_endpoints/"+id, nil, "", nil)
}

// ---------------------------------------------------------------- OAuth (Connect)

type OAuthToken struct {
	StripeUserID string `json:"stripe_user_id"`
	Scope        string `json:"scope"`
	Livemode     bool   `json:"livemode"`
	TokenType    string `json:"token_type"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

// OAuthExchange swaps an authorization code for the connected account id.
func OAuthExchange(ctx context.Context, httpc *http.Client, base, secretKey, code string) (*OAuthToken, error) {
	if base == "" {
		base = "https://connect.stripe.com"
	}
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}}
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(base, "/")+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(secretKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if httpc == nil {
		httpc = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var t OAuthToken
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, fmt.Errorf("stripe oauth: bad response (HTTP %d)", resp.StatusCode)
	}
	if t.Error != "" || t.StripeUserID == "" {
		return nil, fmt.Errorf("stripe oauth: %s %s", t.Error, t.ErrorDesc)
	}
	return &t, nil
}

// OAuthDeauthorize revokes the platform's access to the connected account.
func OAuthDeauthorize(ctx context.Context, httpc *http.Client, base, secretKey, clientID, accountID string) error {
	if base == "" {
		base = "https://connect.stripe.com"
	}
	form := url.Values{"client_id": {clientID}, "stripe_user_id": {accountID}}
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(base, "/")+"/oauth/deauthorize", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(secretKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if httpc == nil {
		httpc = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("stripe deauthorize: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

// ---------------------------------------------------------------- webhook signatures

// VerifySignature validates a Stripe-Signature header against any of secrets.
func VerifySignature(payload []byte, header string, secrets []string, tolerance time.Duration, now time.Time) error {
	if len(secrets) == 0 {
		return errors.New("no webhook signing secret configured")
	}
	var ts int64
	var sigs []string
	for _, part := range strings.Split(header, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			ts, _ = strconv.ParseInt(kv[1], 10, 64)
		case "v1":
			sigs = append(sigs, kv[1])
		}
	}
	if ts == 0 || len(sigs) == 0 {
		return errors.New("unable to extract timestamp and signatures from header")
	}
	if tolerance > 0 {
		d := now.Sub(time.Unix(ts, 0))
		if d < 0 {
			d = -d
		}
		if d > tolerance {
			return errors.New("timestamp outside the tolerance zone")
		}
	}
	signed := strconv.FormatInt(ts, 10) + "." + string(payload)
	for _, secret := range secrets {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(signed))
		want := hex.EncodeToString(mac.Sum(nil))
		for _, got := range sigs {
			if hmac.Equal([]byte(want), []byte(got)) {
				return nil
			}
		}
	}
	return errors.New("no signatures found matching the expected signature for payload")
}

// SignPayload produces a Stripe-Signature header (used by tests and the mock).
func SignPayload(payload []byte, secret string, ts time.Time) string {
	signed := strconv.FormatInt(ts.Unix(), 10) + "." + string(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signed))
	return fmt.Sprintf("t=%d,v1=%s", ts.Unix(), hex.EncodeToString(mac.Sum(nil)))
}
