package service

import (
	"testing"
	"time"

	"fulfillment/internal/stripe"
)

func ptr[T any](v T) *T { return &v }

func TestMapSubscription(t *testing.T) {
	now := time.Now().UTC()
	end := now.Add(10 * 24 * time.Hour).Unix()
	item := func() struct{ Data []stripe.SubItem } {
		return struct{ Data []stripe.SubItem }{Data: []stripe.SubItem{{ID: "si", Quantity: 1, CurrentPeriodEnd: end}}}
	}
	cases := []struct {
		name      string
		in        stripe.Subscription
		status    string
		cancelled bool
		hasEnds   bool
	}{
		{"active", stripe.Subscription{Status: "active", Items: item()}, "active", false, false},
		{"trial", stripe.Subscription{Status: "trialing", TrialEnd: ptr(end), Items: item()}, "on_trial", false, false},
		{"past due", stripe.Subscription{Status: "past_due", Items: item()}, "past_due", false, false},
		{"unpaid", stripe.Subscription{Status: "unpaid", Items: item()}, "unpaid", false, false},
		{"cancel at period end", stripe.Subscription{Status: "active", CancelAtPeriodEnd: true, Items: item()}, "cancelled", true, true},
		{"cancel_at date", stripe.Subscription{Status: "active", CancelAt: ptr(end), Items: item()}, "cancelled", true, true},
		{"paused", stripe.Subscription{Status: "active", PauseCollection: &stripe.PauseCollection{Behavior: "void"}, Items: item()}, "paused", false, false},
		{"ended", stripe.Subscription{Status: "canceled", EndedAt: ptr(now.Unix() - 60)}, "expired", true, true},
		{"past due beats cancel flag", stripe.Subscription{Status: "past_due", CancelAtPeriodEnd: true, Items: item()}, "past_due", true, true},
	}
	for _, c := range cases {
		got := mapSubscription(c.in, now)
		if got.Status != c.status || got.Cancelled != c.cancelled || (got.EndsAt != nil) != c.hasEnds {
			t.Errorf("%s: status=%s cancelled=%v ends=%v", c.name, got.Status, got.Cancelled, got.EndsAt)
		}
	}
	if p := mapSubscription(stripe.Subscription{Status: "active", PauseCollection: &stripe.PauseCollection{Behavior: "void"}}, now).Pause; p == nil || p.Mode != "void" {
		t.Errorf("pause mode: %+v", p)
	}
	if p := mapSubscription(stripe.Subscription{Status: "active", PauseCollection: &stripe.PauseCollection{Behavior: "mark_uncollectible"}}, now).Pause; p == nil || p.Mode != "free" {
		t.Errorf("pause free mode: %+v", p)
	}
}

func TestAddLicenseLength(t *testing.T) {
	base := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	if got := AddLicenseLength(base, 1, "years"); got.Year() != 2027 {
		t.Errorf("years: %v", got)
	}
	if got := AddLicenseLength(base, 10, "days"); got.Day() != 10 || got.Month() != 2 {
		t.Errorf("days: %v", got)
	}
	if got := AddLicenseLength(base, 3, "months"); got.Month() != 5 {
		t.Errorf("months: %v", got)
	}
}

func TestWebhookEventCatalogue(t *testing.T) {
	for _, e := range []string{"order_created", "license_key_created", "subscription_payment_recovered", "affiliate_activated"} {
		if !IsWebhookEvent(e) {
			t.Errorf("%s should be a valid event", e)
		}
	}
	if IsWebhookEvent("order_deleted") {
		t.Error("unknown events must be rejected")
	}
}
