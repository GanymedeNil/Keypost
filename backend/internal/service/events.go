package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"fulfillment/internal/db"
	"fulfillment/internal/stripe"
)

// IngestStripeEvent records and processes one verified Stripe event. A nil
// error means the event is settled (processed, ignored or a duplicate); a
// non-nil error asks Stripe to retry delivery.
func (s *Svc) IngestStripeEvent(ctx context.Context, payload []byte) error {
	var ev stripe.Event
	if err := json.Unmarshal(payload, &ev); err != nil || ev.ID == "" {
		return fmt.Errorf("malformed event payload")
	}
	rec := db.StripeEvent{ID: ev.ID, Type: ev.Type, Account: ev.Account, Livemode: ev.Livemode, Status: "received", Payload: string(payload), ReceivedAt: db.Now()}
	res := s.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&rec)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		var cur db.StripeEvent
		if err := s.DB.First(&cur, "id = ?", ev.ID).Error; err != nil {
			return err
		}
		if cur.Status == "processed" || cur.Status == "ignored" {
			return nil // duplicate delivery
		}
	}
	return s.runEvent(ctx, ev)
}

// ReplayStripeEvent reprocesses a stored event regardless of its status.
func (s *Svc) ReplayStripeEvent(ctx context.Context, id string) (*db.StripeEvent, error) {
	var rec db.StripeEvent
	if err := s.DB.First(&rec, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, NotFound("Stripe event")
		}
		return nil, err
	}
	var ev stripe.Event
	if err := json.Unmarshal([]byte(rec.Payload), &ev); err != nil {
		return nil, Bad("stored payload is not valid JSON")
	}
	err := s.runEvent(ctx, ev)
	s.DB.First(&rec, "id = ?", id)
	if err != nil {
		return &rec, nil // status/error are persisted on the record
	}
	return &rec, nil
}

func (s *Svc) runEvent(ctx context.Context, ev stripe.Event) error {
	res, err := s.processEvent(ctx, ev)
	now := db.Now()
	upd := map[string]any{"attempts": gorm.Expr("attempts + 1"), "processed_at": now}
	if err != nil {
		upd["status"], upd["error"] = "failed", err.Error()
		s.DB.Model(&db.StripeEvent{}).Where("id = ?", ev.ID).Updates(upd)
		if s.Log != nil {
			s.Log.Error("stripe event failed", "id", ev.ID, "type", ev.Type, "err", err)
		}
		return err
	}
	upd["status"], upd["error"] = res.Status, res.Note
	s.DB.Model(&db.StripeEvent{}).Where("id = ?", ev.ID).Updates(upd)
	return nil
}

func objectID(raw json.RawMessage) string {
	var x struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &x)
	return x.ID
}

func (s *Svc) processEvent(ctx context.Context, ev stripe.Event) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	obj := ev.Data.Object
	switch ev.Type {
	case "checkout.session.completed", "checkout.session.async_payment_succeeded":
		return s.FulfillCheckoutSession(ctx, objectID(obj), ev.Account)
	case "invoice.paid", "invoice.payment_succeeded":
		return s.handleInvoicePaid(ctx, objectID(obj))
	case "invoice.payment_failed":
		return s.handleInvoiceFailed(ctx, objectID(obj))
	case "customer.subscription.updated", "customer.subscription.deleted", "customer.subscription.paused", "customer.subscription.resumed":
		return s.handleSubscriptionEvent(ctx, objectID(obj))
	case "charge.refunded":
		return s.handleChargeRefunded(ctx, objectID(obj))
	case "customer.updated":
		var c stripe.Customer
		if err := json.Unmarshal(obj, &c); err != nil {
			return Result{}, err
		}
		return s.handleCustomerUpdated(ctx, c)
	}
	return ignored("event type " + ev.Type + " is not handled"), nil
}

// RetryFailedStripeEvents re-runs failed events (background janitor).
func (s *Svc) RetryFailedStripeEvents(ctx context.Context) {
	var recs []db.StripeEvent
	cutoff := db.Now().Add(-2 * time.Minute)
	s.DB.Where("status = ? AND attempts < ? AND processed_at < ?", "failed", 8, cutoff).Order("received_at").Limit(20).Find(&recs)
	for _, r := range recs {
		var ev stripe.Event
		if json.Unmarshal([]byte(r.Payload), &ev) == nil {
			_ = s.runEvent(ctx, ev)
		}
	}
}

func (s *Svc) PruneOldRecords(ctx context.Context) {
	cut := db.Now().AddDate(0, 0, -90)
	s.DB.WithContext(ctx).Where("received_at < ? AND status IN ?", cut, []string{"processed", "ignored"}).Delete(&db.StripeEvent{})
	s.DB.WithContext(ctx).Where("created_at < ?", db.Now().AddDate(0, 0, -30)).Delete(&db.CustomerLoginToken{})
	s.DB.WithContext(ctx).Where("expires_at < ?", db.Now()).Delete(&db.AdminSession{})
	_ = strings.TrimSpace
}
