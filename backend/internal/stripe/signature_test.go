package stripe

import (
	"encoding/json"
	"testing"
	"time"
)

func TestVerifySignature(t *testing.T) {
	now := time.Now()
	payload := []byte(`{"id":"evt_1"}`)
	good := SignPayload(payload, "whsec_a", now)
	if err := VerifySignature(payload, good, []string{"whsec_a"}, 5*time.Minute, now); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if err := VerifySignature(payload, good, []string{"whsec_old", "whsec_a"}, 5*time.Minute, now); err != nil {
		t.Fatalf("secret rotation (any secret) rejected: %v", err)
	}
	if VerifySignature(payload, good, []string{"whsec_b"}, 5*time.Minute, now) == nil {
		t.Fatal("wrong secret accepted")
	}
	if VerifySignature([]byte(`{"id":"evt_2"}`), good, []string{"whsec_a"}, 5*time.Minute, now) == nil {
		t.Fatal("tampered payload accepted")
	}
	if VerifySignature(payload, SignPayload(payload, "whsec_a", now.Add(-time.Hour)), []string{"whsec_a"}, 5*time.Minute, now) == nil {
		t.Fatal("stale timestamp accepted")
	}
	for _, h := range []string{"", "garbage", "t=abc,v1=zz", "v1=abcd"} {
		if VerifySignature(payload, h, []string{"whsec_a"}, 5*time.Minute, now) == nil {
			t.Fatalf("header %q accepted", h)
		}
	}
	if VerifySignature(payload, good, nil, 5*time.Minute, now) == nil {
		t.Fatal("no secrets must fail closed")
	}
}

func TestTolerantShapes(t *testing.T) {
	// pre-basil: period on the subscription; basil+: period on the item
	var old, neu Subscription
	_ = json.Unmarshal([]byte(`{"id":"sub_1","current_period_end":1700000000,"items":{"data":[{"id":"si_1"}]}}`), &old)
	_ = json.Unmarshal([]byte(`{"id":"sub_1","items":{"data":[{"id":"si_1","current_period_end":1700000500},{"id":"si_2","current_period_end":1700000900}]}}`), &neu)
	if old.PeriodEnd() != 1700000000 || neu.PeriodEnd() != 1700000900 {
		t.Fatalf("PeriodEnd old=%d new=%d", old.PeriodEnd(), neu.PeriodEnd())
	}
	var legacy, parent Invoice
	_ = json.Unmarshal([]byte(`{"id":"in_1","subscription":"sub_1","payment_intent":"pi_1","charge":"ch_1","tax":42}`), &legacy)
	_ = json.Unmarshal([]byte(`{"id":"in_2","parent":{"subscription_details":{"subscription":"sub_2"}},"payments":{"data":[{"payment":{"payment_intent":"pi_2","charge":"ch_2"}}]},"total_taxes":[{"amount":10},{"amount":5}]}`), &parent)
	if legacy.SubscriptionID() != "sub_1" || legacy.PaymentIntentID() != "pi_1" || legacy.ChargeID() != "ch_1" || legacy.TaxAmount() != 42 {
		t.Fatalf("legacy invoice: %+v", legacy)
	}
	if parent.SubscriptionID() != "sub_2" || parent.PaymentIntentID() != "pi_2" || parent.ChargeID() != "ch_2" || parent.TaxAmount() != 15 {
		t.Fatalf("basil invoice: %+v", parent)
	}
	var s Session
	_ = json.Unmarshal([]byte(`{"id":"cs_1","customer":{"id":"cus_1","email":"a@b.co"},"payment_intent":"pi_9","subscription":null}`), &s)
	var c Customer
	if !s.Customer.Decode(&c) || c.Email != "a@b.co" || s.PaymentIntent.ID != "pi_9" || s.PaymentIntent.Expanded() || s.Subscription.ID != "" {
		t.Fatalf("expandable handling: %+v", s)
	}
}
