package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"fulfillment/internal/settings"
	"fulfillment/internal/stripe"
)

// WebhookEvents lists every event name merchants can subscribe to.
var WebhookEvents = []string{
	"order_created", "order_refunded",
	"customer_updated",
	"subscription_created", "subscription_updated", "subscription_cancelled", "subscription_resumed",
	"subscription_expired", "subscription_paused", "subscription_unpaused",
	"subscription_payment_success", "subscription_payment_failed", "subscription_payment_recovered", "subscription_payment_refunded",
	"license_key_created", "license_key_updated",
	"affiliate_activated",
}

func IsWebhookEvent(name string) bool {
	for _, e := range WebhookEvents {
		if e == name {
			return true
		}
	}
	return false
}

// RequiredStripeEvents are the Stripe events this service consumes.
var RequiredStripeEvents = []string{
	"checkout.session.completed",
	"checkout.session.async_payment_succeeded",
	"invoice.paid",
	"invoice.payment_failed",
	"customer.subscription.updated",
	"customer.subscription.deleted",
	"customer.subscription.paused",
	"customer.subscription.resumed",
	"charge.refunded",
	"customer.updated",
}

// Stripe returns a client bound to the current settings.
func (s *Svc) Stripe() (*stripe.Client, error) {
	key := s.Settings.Get(settings.StripeSecretKey)
	if key == "" {
		return nil, stripe.ErrNotConfigured
	}
	return stripe.NewClient(key, s.Settings.Get(settings.StripeAPIVersion), s.Cfg.Stripe.APIBase), nil
}

// StripeTestMode reports whether the configured key is a test-mode key.
func (s *Svc) StripeTestMode() bool {
	k := s.Settings.Get(settings.StripeSecretKey)
	return strings.HasPrefix(k, "sk_test_") || strings.HasPrefix(k, "rk_test_")
}

func stripeErr(err error) error {
	if err == nil {
		return nil
	}
	if e, ok := err.(*stripe.Error); ok {
		return Upstream(e.Message)
	}
	if err == stripe.ErrNotConfigured {
		return Upstream(err.Error())
	}
	return Upstream(err.Error())
}

// ---------------------------------------------------------------- background work

var bgWG sync.WaitGroup

// Go runs fn in the background (receipt emails etc.).
func (s *Svc) Go(fn func()) {
	bgWG.Add(1)
	go func() {
		defer bgWG.Done()
		defer func() {
			if r := recover(); r != nil && s.Log != nil {
				s.Log.Error("background task panic", "err", r)
			}
		}()
		fn()
	}()
}

// WaitBackground blocks until background tasks finish (tests, shutdown).
func WaitBackground() { bgWG.Wait() }

// ---------------------------------------------------------------- currency

// defaultUSDPerUnit approximates USD per one major unit; used only for the
// *_usd reporting columns and overridable through the fx.rates setting.
var defaultUSDPerUnit = map[string]float64{
	"USD": 1, "EUR": 1.08, "GBP": 1.27, "CAD": 0.73, "AUD": 0.65, "NZD": 0.60, "CHF": 1.12, "SEK": 0.095,
	"NOK": 0.093, "DKK": 0.145, "JPY": 0.0067, "CNY": 0.14, "HKD": 0.128, "SGD": 0.74, "INR": 0.012,
	"KRW": 0.00072, "BRL": 0.19, "MXN": 0.055, "PLN": 0.25, "CZK": 0.043, "TWD": 0.031,
}

const settingFXRates = "fx.rates"

func (s *Svc) usdPerMinorUnit(cur string) float64 {
	cur = strings.ToUpper(cur)
	if cur == "USD" || cur == "" {
		return 1
	}
	rate, ok := defaultUSDPerUnit[cur]
	if raw := s.Settings.Get(settingFXRates); raw != "" {
		var m map[string]float64
		if json.Unmarshal([]byte(raw), &m) == nil {
			if r, found := m[cur]; found && r > 0 {
				rate, ok = r, true
			}
		}
	}
	if !ok {
		rate = 1
	}
	// zero-decimal currencies: 1 minor unit == 1 major unit
	if zeroDecimalCurrency(cur) {
		return rate
	}
	return rate // major→USD factor applies equally to minor units (both /100)
}

func zeroDecimalCurrency(cur string) bool {
	switch strings.ToUpper(cur) {
	case "JPY", "KRW", "VND", "CLP", "UGX":
		return true
	}
	return false
}

// toUSD converts minor units of cur into USD cents.
func (s *Svc) toUSD(amount int64, cur string) int64 {
	r := s.usdPerMinorUnit(cur)
	if r == 1 {
		return amount
	}
	if zeroDecimalCurrency(cur) {
		return int64(float64(amount)*r*100 + 0.5)
	}
	return int64(float64(amount)*r + 0.5)
}

// currencyRate formats the LS "currency_rate" (units of cur per 1 USD).
func (s *Svc) currencyRate(cur string) string {
	r := s.usdPerMinorUnit(cur)
	if r == 0 || r == 1 {
		return "1.0000"
	}
	return fmt.Sprintf("%.4f", 1/r)
}

var _ = context.Background
