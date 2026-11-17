// Package stripe is a small, dependency-free Stripe REST client. It pins the
// API version explicitly and parses objects tolerantly so that both the
// pre- and post-"basil" response shapes (subscription periods on the item,
// invoice.parent.subscription_details, …) are understood.
package stripe

import (
	"bytes"
	"encoding/json"
)

// Expandable is either an object id (string) or an expanded object.
type Expandable struct {
	ID  string
	Raw json.RawMessage // set when the object was expanded
}

func (e *Expandable) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*e = Expandable{}
		return nil
	}
	if b[0] == '"' {
		e.Raw = nil
		return json.Unmarshal(b, &e.ID)
	}
	var probe struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(b, &probe); err != nil {
		return err
	}
	e.ID = probe.ID
	e.Raw = append(e.Raw[:0], b...)
	return nil
}

func (e Expandable) MarshalJSON() ([]byte, error) {
	if e.Raw != nil {
		return e.Raw, nil
	}
	return json.Marshal(e.ID)
}

func (e Expandable) Expanded() bool { return len(e.Raw) > 0 }

// Decode decodes an expanded object into v; it reports false when only the id is present.
func (e Expandable) Decode(v any) bool {
	if len(e.Raw) == 0 {
		return false
	}
	return json.Unmarshal(e.Raw, v) == nil
}

type Address struct {
	City       string `json:"city"`
	Country    string `json:"country"`
	Line1      string `json:"line1"`
	PostalCode string `json:"postal_code"`
	State      string `json:"state"`
}

type Customer struct {
	ID       string            `json:"id"`
	Email    string            `json:"email"`
	Name     string            `json:"name"`
	Address  *Address          `json:"address"`
	Metadata map[string]string `json:"metadata"`
	Livemode bool              `json:"livemode"`
	Deleted  bool              `json:"deleted"`
	Created  int64             `json:"created"`
}

type Product struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Active       bool              `json:"active"`
	Metadata     map[string]string `json:"metadata"`
	DefaultPrice Expandable        `json:"default_price"`
	Images       []string          `json:"images"`
	Created      int64             `json:"created"`
}

type Recurring struct {
	Interval        string `json:"interval"`
	IntervalCount   int    `json:"interval_count"`
	TrialPeriodDays *int   `json:"trial_period_days"`
}

type Price struct {
	ID               string            `json:"id"`
	Product          Expandable        `json:"product"`
	Active           bool              `json:"active"`
	Currency         string            `json:"currency"`
	UnitAmount       *int64            `json:"unit_amount"`
	Type             string            `json:"type"`
	Recurring        *Recurring        `json:"recurring"`
	Metadata         map[string]string `json:"metadata"`
	Nickname         string            `json:"nickname"`
	CustomUnitAmount *struct {
		Minimum *int64 `json:"minimum"`
		Preset  *int64 `json:"preset"`
	} `json:"custom_unit_amount"`
}

type LineItem struct {
	ID             string `json:"id"`
	Description    string `json:"description"`
	Quantity       int    `json:"quantity"`
	AmountSubtotal int64  `json:"amount_subtotal"`
	AmountTotal    int64  `json:"amount_total"`
	AmountDiscount int64  `json:"amount_discount"`
	AmountTax      int64  `json:"amount_tax"`
	Currency       string `json:"currency"`
	Price          *Price `json:"price"`
}

type TaxID struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type CustomerDetails struct {
	Email   string   `json:"email"`
	Name    string   `json:"name"`
	Address *Address `json:"address"`
	TaxIDs  []TaxID  `json:"tax_ids"`
}

type TotalDetails struct {
	AmountDiscount int64 `json:"amount_discount"`
	AmountTax      int64 `json:"amount_tax"`
	AmountShipping int64 `json:"amount_shipping"`
}

type TotalTax struct {
	Amount    int64 `json:"amount"`
	Inclusive bool  `json:"tax_behavior_inclusive"`
}

type Session struct {
	ID                string            `json:"id"`
	Mode              string            `json:"mode"` // payment | subscription | setup
	PaymentStatus     string            `json:"payment_status"`
	Status            string            `json:"status"`
	Currency          string            `json:"currency"`
	AmountSubtotal    int64             `json:"amount_subtotal"`
	AmountTotal       int64             `json:"amount_total"`
	TotalDetails      *TotalDetails     `json:"total_details"`
	Customer          Expandable        `json:"customer"`
	CustomerEmail     string            `json:"customer_email"`
	CustomerDetails   *CustomerDetails  `json:"customer_details"`
	ClientReferenceID string            `json:"client_reference_id"`
	Metadata          map[string]string `json:"metadata"`
	PaymentIntent     Expandable        `json:"payment_intent"`
	Subscription      Expandable        `json:"subscription"`
	Invoice           Expandable        `json:"invoice"`
	LineItems         *struct {
		Data []LineItem `json:"data"`
	} `json:"line_items"`
	Livemode  bool   `json:"livemode"`
	Created   int64  `json:"created"`
	ExpiresAt int64  `json:"expires_at"`
	URL       string `json:"url"`
}

type SubItem struct {
	ID                 string `json:"id"`
	Quantity           int    `json:"quantity"`
	Price              *Price `json:"price"`
	CurrentPeriodStart int64  `json:"current_period_start"`
	CurrentPeriodEnd   int64  `json:"current_period_end"`
}

type PauseCollection struct {
	Behavior  string `json:"behavior"` // keep_as_draft | mark_uncollectible | void
	ResumesAt *int64 `json:"resumes_at"`
}

type Subscription struct {
	ID                   string                   `json:"id"`
	Status               string                   `json:"status"` // trialing active past_due unpaid canceled incomplete incomplete_expired paused
	Customer             Expandable               `json:"customer"`
	CancelAtPeriodEnd    bool                     `json:"cancel_at_period_end"`
	CancelAt             *int64                   `json:"cancel_at"`
	CanceledAt           *int64                   `json:"canceled_at"`
	EndedAt              *int64                   `json:"ended_at"`
	CurrentPeriodEnd     int64                    `json:"current_period_end"` // legacy (pre-basil)
	CurrentPeriodStart   int64                    `json:"current_period_start"`
	TrialEnd             *int64                   `json:"trial_end"`
	BillingCycleAnchor   int64                    `json:"billing_cycle_anchor"`
	Items                struct{ Data []SubItem } `json:"items"`
	DefaultPaymentMethod Expandable               `json:"default_payment_method"`
	LatestInvoice        Expandable               `json:"latest_invoice"`
	PauseCollection      *PauseCollection         `json:"pause_collection"`
	Metadata             map[string]string        `json:"metadata"`
	Livemode             bool                     `json:"livemode"`
	Created              int64                    `json:"created"`
}

// PeriodEnd returns the end of the current billing period for both API shapes.
func (s Subscription) PeriodEnd() int64 {
	if s.CurrentPeriodEnd > 0 {
		return s.CurrentPeriodEnd
	}
	var end int64
	for _, it := range s.Items.Data {
		if it.CurrentPeriodEnd > end {
			end = it.CurrentPeriodEnd
		}
	}
	return end
}

type PaymentMethod struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Card *struct {
		Brand string `json:"brand"`
		Last4 string `json:"last4"`
	} `json:"card"`
}

type Charge struct {
	ID             string     `json:"id"`
	PaymentIntent  Expandable `json:"payment_intent"`
	Invoice        Expandable `json:"invoice"`
	Customer       Expandable `json:"customer"`
	Amount         int64      `json:"amount"`
	AmountRefunded int64      `json:"amount_refunded"`
	Refunded       bool       `json:"refunded"`
	Currency       string     `json:"currency"`
	ReceiptURL     string     `json:"receipt_url"`
	Created        int64      `json:"created"`
	Livemode       bool       `json:"livemode"`
	BillingDetails *struct {
		Name    string   `json:"name"`
		Email   string   `json:"email"`
		Address *Address `json:"address"`
	} `json:"billing_details"`
	PaymentMethodDetails *struct {
		Type string `json:"type"`
		Card *struct {
			Brand string `json:"brand"`
			Last4 string `json:"last4"`
		} `json:"card"`
	} `json:"payment_method_details"`
}

type PaymentIntent struct {
	ID            string            `json:"id"`
	Amount        int64             `json:"amount"`
	Currency      string            `json:"currency"`
	LatestCharge  Expandable        `json:"latest_charge"`
	PaymentMethod Expandable        `json:"payment_method"`
	Customer      Expandable        `json:"customer"`
	Metadata      map[string]string `json:"metadata"`
}

type Invoice struct {
	ID            string     `json:"id"`
	Status        string     `json:"status"`
	Customer      Expandable `json:"customer"`
	CustomerEmail string     `json:"customer_email"`
	CustomerName  string     `json:"customer_name"`
	Subscription  Expandable `json:"subscription"` // legacy
	Parent        *struct {
		SubscriptionDetails *struct {
			Subscription Expandable        `json:"subscription"`
			Metadata     map[string]string `json:"metadata"`
		} `json:"subscription_details"`
	} `json:"parent"`
	BillingReason string `json:"billing_reason"`
	Currency      string `json:"currency"`
	Subtotal      int64  `json:"subtotal"`
	Total         int64  `json:"total"`
	AmountPaid    int64  `json:"amount_paid"`
	Tax           *int64 `json:"tax"` // legacy
	TotalTaxes    []struct {
		Amount int64 `json:"amount"`
	} `json:"total_taxes"`
	TotalDiscountAmounts []struct {
		Amount int64 `json:"amount"`
	} `json:"total_discount_amounts"`
	HostedInvoiceURL string     `json:"hosted_invoice_url"`
	InvoicePDF       string     `json:"invoice_pdf"`
	PaymentIntent    Expandable `json:"payment_intent"` // legacy
	Charge           Expandable `json:"charge"`         // legacy
	Payments         *struct {
		Data []struct {
			Payment struct {
				PaymentIntent Expandable `json:"payment_intent"`
				Charge        Expandable `json:"charge"`
			} `json:"payment"`
		} `json:"data"`
	} `json:"payments"`
	Created           int64 `json:"created"`
	StatusTransitions *struct {
		PaidAt *int64 `json:"paid_at"`
	} `json:"status_transitions"`
	Livemode bool `json:"livemode"`
}

// SubscriptionID resolves the subscription for both API shapes.
func (i Invoice) SubscriptionID() string {
	if i.Subscription.ID != "" {
		return i.Subscription.ID
	}
	if i.Parent != nil && i.Parent.SubscriptionDetails != nil {
		return i.Parent.SubscriptionDetails.Subscription.ID
	}
	return ""
}

func (i Invoice) TaxAmount() int64 {
	if i.Tax != nil {
		return *i.Tax
	}
	var t int64
	for _, x := range i.TotalTaxes {
		t += x.Amount
	}
	return t
}

func (i Invoice) DiscountAmount() int64 {
	var t int64
	for _, x := range i.TotalDiscountAmounts {
		t += x.Amount
	}
	return t
}

func (i Invoice) PaymentIntentID() string {
	if i.PaymentIntent.ID != "" {
		return i.PaymentIntent.ID
	}
	if i.Payments != nil {
		for _, p := range i.Payments.Data {
			if p.Payment.PaymentIntent.ID != "" {
				return p.Payment.PaymentIntent.ID
			}
		}
	}
	return ""
}

func (i Invoice) ChargeID() string {
	if i.Charge.ID != "" {
		return i.Charge.ID
	}
	if i.Payments != nil {
		for _, p := range i.Payments.Data {
			if p.Payment.Charge.ID != "" {
				return p.Payment.Charge.ID
			}
		}
	}
	return ""
}

type Refund struct {
	ID            string     `json:"id"`
	Status        string     `json:"status"`
	Amount        int64      `json:"amount"`
	Currency      string     `json:"currency"`
	Charge        Expandable `json:"charge"`
	PaymentIntent Expandable `json:"payment_intent"`
}

type Account struct {
	ID              string `json:"id"`
	Email           string `json:"email"`
	Country         string `json:"country"`
	DefaultCurrency string `json:"default_currency"`
	BusinessProfile *struct {
		Name string `json:"name"`
	} `json:"business_profile"`
	Settings *struct {
		Dashboard struct {
			DisplayName string `json:"display_name"`
		} `json:"dashboard"`
	} `json:"settings"`
}

func (a Account) DisplayName() string {
	if a.Settings != nil && a.Settings.Dashboard.DisplayName != "" {
		return a.Settings.Dashboard.DisplayName
	}
	if a.BusinessProfile != nil && a.BusinessProfile.Name != "" {
		return a.BusinessProfile.Name
	}
	return a.Email
}

type WebhookEndpoint struct {
	ID            string   `json:"id"`
	URL           string   `json:"url"`
	Status        string   `json:"status"`
	EnabledEvents []string `json:"enabled_events"`
	Secret        string   `json:"secret"`
	Livemode      bool     `json:"livemode"`
}

type BillingPortalSession struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

type Event struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Account  string `json:"account"`
	Livemode bool   `json:"livemode"`
	Created  int64  `json:"created"`
	Data     struct {
		Object             json.RawMessage `json:"object"`
		PreviousAttributes json.RawMessage `json:"previous_attributes"`
	} `json:"data"`
}

type list[T any] struct {
	Data    []T  `json:"data"`
	HasMore bool `json:"has_more"`
}
