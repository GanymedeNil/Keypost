// Package webhooks delivers Lemon Squeezy compatible outgoing webhooks:
// same event names, JSON:API payloads and X-Signature (HMAC-SHA256 hex).
package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"syscall"
	"time"

	"gorm.io/gorm"

	"fulfillment/internal/crypt"
	"fulfillment/internal/db"
	"fulfillment/internal/resource"
	"fulfillment/internal/service"
)

type Dispatcher struct {
	DB          *gorm.DB
	Res         *resource.Builder
	Log         *slog.Logger
	MaxAttempts int
	Client      *http.Client

	wake chan struct{}
	sem  chan struct{}
	wg   sync.WaitGroup
}

func New(g *gorm.DB, res *resource.Builder, log *slog.Logger, maxAttempts int) *Dispatcher {
	if maxAttempts < 1 {
		maxAttempts = 4
	}
	d := &Dispatcher{DB: g, Res: res, Log: log, MaxAttempts: maxAttempts, wake: make(chan struct{}, 1), sem: make(chan struct{}, 8)}
	d.Client = &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{Timeout: 5 * time.Second, Control: blockMetadata}).DialContext,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return d
}

// blockMetadata refuses link-local targets (cloud metadata services).
func blockMetadata(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()) {
		return errors.New("webhook target is a link-local address")
	}
	return nil
}

var _ service.Emitter = (*Dispatcher)(nil)

// Wait blocks until in-flight deliveries finish.
func (d *Dispatcher) Wait() { d.wg.Wait() }

// ---------------------------------------------------------------- emit

func (d *Dispatcher) build(typ string, id int64) (resource.Resource, error) {
	switch typ {
	case "orders":
		var m db.Order
		if err := d.DB.First(&m, id).Error; err != nil {
			return resource.Resource{}, err
		}
		return d.Res.Orders([]db.Order{m})[0], nil
	case "subscriptions":
		var m db.Subscription
		if err := d.DB.First(&m, id).Error; err != nil {
			return resource.Resource{}, err
		}
		return d.Res.Subscriptions([]db.Subscription{m})[0], nil
	case "subscription-invoices":
		var m db.SubscriptionInvoice
		if err := d.DB.First(&m, id).Error; err != nil {
			return resource.Resource{}, err
		}
		return d.Res.SubscriptionInvoices([]db.SubscriptionInvoice{m})[0], nil
	case "license-keys":
		var m db.LicenseKey
		if err := d.DB.First(&m, id).Error; err != nil {
			return resource.Resource{}, err
		}
		return d.Res.LicenseKeys([]db.LicenseKey{m})[0], nil
	case "customers":
		var m db.Customer
		if err := d.DB.First(&m, id).Error; err != nil {
			return resource.Resource{}, err
		}
		return d.Res.Customers([]db.Customer{m})[0], nil
	}
	return resource.Resource{}, fmt.Errorf("unsupported resource type %q", typ)
}

type payload struct {
	Meta map[string]any    `json:"meta"`
	Data resource.Resource `json:"data"`
}

// Payload renders the exact JSON body sent for an event.
func (d *Dispatcher) Payload(ev service.Event) ([]byte, error) {
	res, err := d.build(ev.ResourceType, ev.ResourceID)
	if err != nil {
		return nil, err
	}
	meta := map[string]any{"event_name": ev.Name}
	if len(ev.CustomData) > 0 && (ev.ResourceType == "orders" || ev.ResourceType == "subscriptions" || ev.ResourceType == "license-keys" || ev.ResourceType == "subscription-invoices") {
		meta["custom_data"] = ev.CustomData
	}
	return json.Marshal(payload{Meta: meta, Data: res})
}

// Emit queues one delivery per matching, enabled webhook endpoint.
func (d *Dispatcher) Emit(ctx context.Context, ev service.Event) {
	var hooks []db.Webhook
	if err := d.DB.WithContext(ctx).Where("store_id = ? AND test_mode = ? AND disabled = ?", ev.StoreID, ev.TestMode, false).Find(&hooks).Error; err != nil {
		d.Log.Error("webhook lookup failed", "err", err)
		return
	}
	var body []byte
	n := 0
	for _, h := range hooks {
		if !contains(h.Events.V, ev.Name) {
			continue
		}
		if body == nil {
			b, err := d.Payload(ev)
			if err != nil {
				d.Log.Error("webhook payload failed", "event", ev.Name, "err", err)
				return
			}
			body = b
		}
		now := db.Now()
		rec := db.WebhookDelivery{
			WebhookID: h.ID, StoreID: ev.StoreID, EventName: ev.Name, ResourceType: ev.ResourceType, ResourceID: fmt.Sprint(ev.ResourceID),
			Payload: string(body), Status: "pending", NextAttemptAt: now, CreatedAt: now,
		}
		if err := d.DB.WithContext(ctx).Create(&rec).Error; err != nil {
			d.Log.Error("queue webhook delivery", "err", err)
			continue
		}
		n++
	}
	if n > 0 {
		select {
		case d.wake <- struct{}{}:
		default:
		}
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- worker

// Run processes the delivery queue until ctx is cancelled.
func (d *Dispatcher) Run(ctx context.Context) {
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for {
		d.drain(ctx)
		select {
		case <-ctx.Done():
			d.Wait()
			return
		case <-d.wake:
		case <-t.C:
		}
	}
}

func (d *Dispatcher) drain(ctx context.Context) {
	for {
		var due []db.WebhookDelivery
		if err := d.DB.WithContext(ctx).Where("status = ? AND next_attempt_at <= ?", "pending", db.Now()).Order("id").Limit(25).Find(&due).Error; err != nil || len(due) == 0 {
			return
		}
		claimed := 0
		for _, rec := range due {
			// lease the row so concurrent instances do not double-send
			res := d.DB.Model(&db.WebhookDelivery{}).Where("id = ? AND status = ? AND next_attempt_at <= ?", rec.ID, "pending", db.Now()).
				Update("next_attempt_at", db.Now().Add(60*time.Second))
			if res.Error != nil || res.RowsAffected == 0 {
				continue
			}
			claimed++
			d.sem <- struct{}{}
			d.wg.Add(1)
			go func(id int64) {
				defer func() { <-d.sem; d.wg.Done() }()
				d.deliver(context.Background(), id)
			}(rec.ID)
		}
		if claimed == 0 || ctx.Err() != nil {
			return
		}
	}
}

func backoff(attempt int) time.Duration {
	// 5s, 25s, 125s, 625s …
	d := 5 * time.Second
	for i := 1; i < attempt; i++ {
		d *= 5
	}
	return d
}

// deliver sends one queued delivery and records the outcome.
func (d *Dispatcher) deliver(ctx context.Context, id int64) *db.WebhookDelivery {
	var rec db.WebhookDelivery
	if err := d.DB.First(&rec, id).Error; err != nil {
		return nil
	}
	var h db.Webhook
	if err := d.DB.First(&h, rec.WebhookID).Error; err != nil {
		d.DB.Model(&rec).Updates(map[string]any{"status": "failed", "last_error": "webhook endpoint was deleted"})
		return &rec
	}
	code, resp, dur, err := d.send(ctx, h, rec.EventName, []byte(rec.Payload))
	rec.Attempts++
	rec.LastStatusCode, rec.LastResponse, rec.DurationMS = code, resp, int(dur.Milliseconds())
	now := db.Now()
	upd := map[string]any{"attempts": rec.Attempts, "last_status_code": code, "last_response": resp, "duration_ms": rec.DurationMS}
	switch {
	case err == nil && code >= 200 && code < 300:
		rec.Status, rec.LastError, rec.DeliveredAt = "success", "", &now
		upd["status"], upd["last_error"], upd["delivered_at"] = "success", "", now
		d.DB.Model(&db.Webhook{}).Where("id = ?", h.ID).Update("last_sent_at", now)
	default:
		msg := fmt.Sprintf("HTTP %d", code)
		if err != nil {
			msg = err.Error()
		}
		rec.LastError = msg
		upd["last_error"] = msg
		if rec.Attempts >= d.MaxAttempts {
			rec.Status = "failed"
		} else {
			rec.Status = "pending"
			rec.NextAttemptAt = now.Add(backoff(rec.Attempts))
			upd["next_attempt_at"] = rec.NextAttemptAt
		}
		upd["status"] = rec.Status
	}
	d.DB.Model(&db.WebhookDelivery{}).Where("id = ?", rec.ID).Updates(upd)
	return &rec
}

func (d *Dispatcher) send(ctx context.Context, h db.Webhook, event string, body []byte) (int, string, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL, bytes.NewReader(body))
	if err != nil {
		return 0, "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Event-Name", event)
	req.Header.Set("X-Signature", crypt.HMACHex(string(h.Secret), body))
	req.Header.Set("User-Agent", "DigitalFulfillment-Webhooks/1.0")
	start := time.Now()
	resp, err := d.Client.Do(req)
	dur := time.Since(start)
	if err != nil {
		return 0, "", dur, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	return resp.StatusCode, string(b), dur, nil
}

// ---------------------------------------------------------------- admin helpers

// Redeliver resets a delivery and sends it immediately.
func (d *Dispatcher) Redeliver(ctx context.Context, id int64) (*db.WebhookDelivery, error) {
	var rec db.WebhookDelivery
	if err := d.DB.First(&rec, id).Error; err != nil {
		return nil, err
	}
	d.DB.Model(&rec).Updates(map[string]any{"status": "pending", "attempts": 0, "next_attempt_at": db.Now().Add(time.Hour), "delivered_at": nil})
	out := d.deliver(ctx, id)
	if out == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return out, nil
}

// SendTest posts a sample order_created payload to one endpoint and returns the stored delivery.
func (d *Dispatcher) SendTest(ctx context.Context, h db.Webhook) (*db.WebhookDelivery, error) {
	var o db.Order
	err := d.DB.Where("store_id = ? AND test_mode = ?", h.StoreID, h.TestMode).Order("id DESC").First(&o).Error
	var body []byte
	if err == nil {
		body, err = d.Payload(service.Event{Name: "order_created", ResourceType: "orders", ResourceID: o.ID, CustomData: map[string]any{"test": true}})
	}
	if err != nil {
		body = []byte(`{"meta":{"event_name":"order_created","custom_data":{"test":true}},"data":{"type":"orders","id":"0","attributes":{"identifier":"00000000-0000-0000-0000-000000000000","order_number":0,"status":"paid","total":0,"currency":"USD","user_email":"test@example.com","test_mode":true}}}`)
	}
	now := db.Now()
	rec := db.WebhookDelivery{WebhookID: h.ID, StoreID: h.StoreID, EventName: "order_created", ResourceType: "orders", ResourceID: "test", Payload: string(body), Status: "pending", NextAttemptAt: now.Add(time.Hour), CreatedAt: now}
	if err := d.DB.Create(&rec).Error; err != nil {
		return nil, err
	}
	// a test is a single attempt, never retried
	out := d.deliver(ctx, rec.ID)
	if out != nil && out.Status == "pending" {
		d.DB.Model(&db.WebhookDelivery{}).Where("id = ?", rec.ID).Update("status", "failed")
		out.Status = "failed"
	}
	return out, nil
}
