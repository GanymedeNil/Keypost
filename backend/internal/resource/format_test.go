package resource

import (
	"testing"
	"time"

	"fulfillment/internal/db"
)

func TestFormatMoney(t *testing.T) {
	cases := []struct {
		cents int64
		cur   string
		want  string
	}{
		{999, "USD", "$9.99"}, {0, "USD", "$0.00"}, {5, "usd", "$0.05"}, {123456789, "USD", "$1,234,567.89"},
		{1999, "EUR", "€19.99"}, {500, "JPY", "¥500"}, {2500, "CHF", "CHF 25.00"}, {-250, "USD", "-$2.50"}, {100, "XYZ", "XYZ 1.00"},
	}
	for _, c := range cases {
		if got := FormatMoney(c.cents, c.cur); got != c.want {
			t.Errorf("FormatMoney(%d,%s) = %q, want %q", c.cents, c.cur, got, c.want)
		}
	}
}

func TestHumanSizeAndStatus(t *testing.T) {
	if got := HumanSize(874694); got != "854 KB" {
		t.Errorf("HumanSize = %q", got)
	}
	if got := HumanSize(512); got != "512 B" {
		t.Errorf("HumanSize = %q", got)
	}
	if got := HumanSize(5_767_168); got != "5.5 MB" {
		t.Errorf("HumanSize = %q", got)
	}
	if FormatStatus("partial_refund") != "Partial refund" || FormatStatus("on_trial") != "On trial" {
		t.Error("FormatStatus")
	}
}

func TestLicenseStatusPrecedence(t *testing.T) {
	now := time.Now()
	past, future := now.Add(-time.Hour), now.Add(time.Hour)
	cases := []struct {
		k    db.LicenseKey
		want string
	}{
		{db.LicenseKey{}, "inactive"},
		{db.LicenseKey{InstancesCount: 2}, "active"},
		{db.LicenseKey{InstancesCount: 2, ExpiresAt: &future}, "active"},
		{db.LicenseKey{InstancesCount: 2, ExpiresAt: &past}, "expired"},
		{db.LicenseKey{Disabled: true, ExpiresAt: &past, InstancesCount: 1}, "disabled"},
	}
	for i, c := range cases {
		if got := LicenseStatus(&c.k, now); got != c.want {
			t.Errorf("case %d: got %s want %s", i, got, c.want)
		}
	}
}

func TestKeyShortAndMonthly(t *testing.T) {
	if got := KeyShort("80e15db5-c796-436b-850c-8f9c98a48abe"); got != "XXXX-8f9c98a48abe" {
		t.Errorf("KeyShort = %s", got)
	}
	if MonthlyAmount(1200, "year", 1) != 100 || MonthlyAmount(900, "month", 3) != 300 || MonthlyAmount(100, "week", 1) != 433 {
		t.Error("MonthlyAmount")
	}
}
