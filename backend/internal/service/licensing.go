package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"fulfillment/internal/db"
	"fulfillment/internal/resource"
)

// LicenseMeta mirrors the "meta" object of the Lemon Squeezy License API.
type LicenseMeta struct {
	StoreID       int64  `json:"store_id"`
	OrderID       int64  `json:"order_id"`
	OrderItemID   int64  `json:"order_item_id"`
	ProductID     int64  `json:"product_id"`
	ProductName   string `json:"product_name"`
	VariantID     int64  `json:"variant_id"`
	VariantName   string `json:"variant_name"`
	CustomerID    int64  `json:"customer_id"`
	CustomerName  string `json:"customer_name"`
	CustomerEmail string `json:"customer_email"`
}

type LicenseOutcome struct {
	OK       bool
	Err      string
	HTTP     int
	Key      *db.LicenseKey
	Instance *db.LicenseKeyInstance
	Meta     *LicenseMeta
}

const (
	errKeyNotFound      = "license_key not found."
	errInstanceNotFound = "license_key_instance not found."
	errLimitReached     = "This license key has reached the activation limit."
	errDisabled         = "This license key is disabled."
	errExpired          = "This license key has expired."
)

func NewLicenseKeyString() string { return uuid.NewString() }

// AddLicenseLength computes the expiry for a fixed-length license.
func AddLicenseLength(base time.Time, value int, unit string) time.Time {
	switch unit {
	case "days":
		return base.AddDate(0, 0, value)
	case "months":
		return base.AddDate(0, value, 0)
	default:
		return base.AddDate(value, 0, 0)
	}
}

func (s *Svc) licenseMeta(k db.LicenseKey) *LicenseMeta {
	m := &LicenseMeta{
		StoreID: k.StoreID, OrderID: k.OrderID, OrderItemID: k.OrderItemID, ProductID: k.ProductID, VariantID: k.VariantID,
		CustomerID: k.CustomerID, CustomerName: k.UserName, CustomerEmail: k.UserEmail,
	}
	var p db.Product
	if k.ProductID != 0 && s.DB.First(&p, k.ProductID).Error == nil {
		m.ProductName = p.Name
	}
	var v db.Variant
	if k.VariantID != 0 && s.DB.First(&v, k.VariantID).Error == nil {
		m.VariantName = v.Name
	}
	if k.OrderItemID != 0 && (m.ProductName == "" || m.VariantName == "") {
		var it db.OrderItem
		if s.DB.First(&it, k.OrderItemID).Error == nil {
			if m.ProductName == "" {
				m.ProductName = it.ProductName
			}
			if m.VariantName == "" {
				m.VariantName = it.VariantName
			}
		}
	}
	var c db.Customer
	if k.CustomerID != 0 && s.DB.First(&c, k.CustomerID).Error == nil {
		if c.Name != "" {
			m.CustomerName = c.Name
		}
		if c.Email != "" {
			m.CustomerEmail = c.Email
		}
	}
	return m
}

func (s *Svc) findKey(tx *gorm.DB, key string) (*db.LicenseKey, error) {
	var k db.LicenseKey
	err := lockForUpdate(tx).Where(clause.Eq{Column: clause.Column{Name: "key"}, Value: strings.TrimSpace(key)}).First(&k).Error
	if err != nil {
		return nil, err
	}
	return &k, nil
}

func notFoundOutcome(msg string) LicenseOutcome {
	return LicenseOutcome{HTTP: http.StatusNotFound, Err: msg}
}

// ActivateLicense creates a new instance for the key (License API: activate).
func (s *Svc) ActivateLicense(ctx context.Context, key, instanceName string) LicenseOutcome {
	var out LicenseOutcome
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		k, err := s.findKey(tx, key)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			out = notFoundOutcome(errKeyNotFound)
			return nil
		}
		if err != nil {
			return err
		}
		now := db.Now()
		out.Key = k
		switch st := resource.LicenseStatus(k, now); {
		case st == "disabled":
			out.HTTP, out.Err = http.StatusBadRequest, errDisabled
			return nil
		case st == "expired":
			out.HTTP, out.Err = http.StatusBadRequest, errExpired
			return nil
		}
		if k.ActivationLimit != nil && k.InstancesCount >= *k.ActivationLimit {
			out.HTTP, out.Err = http.StatusBadRequest, errLimitReached
			return nil
		}
		inst := db.LicenseKeyInstance{LicenseKeyID: k.ID, Identifier: uuid.NewString(), Name: instanceName, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&inst).Error; err != nil {
			return err
		}
		k.InstancesCount++
		k.UpdatedAt = now
		if err := tx.Model(k).Updates(map[string]any{"instances_count": k.InstancesCount, "updated_at": now}).Error; err != nil {
			return err
		}
		out.OK, out.HTTP, out.Instance = true, http.StatusOK, &inst
		return nil
	})
	if err != nil {
		return LicenseOutcome{HTTP: http.StatusInternalServerError, Err: "internal error"}
	}
	if out.Key != nil {
		out.Meta = s.licenseMeta(*out.Key)
	}
	return out
}

// ValidateLicense checks a key (and optionally one of its instances).
func (s *Svc) ValidateLicense(ctx context.Context, key, instanceID string) LicenseOutcome {
	var k db.LicenseKey
	err := s.DB.WithContext(ctx).Where(clause.Eq{Column: clause.Column{Name: "key"}, Value: strings.TrimSpace(key)}).First(&k).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return notFoundOutcome(errKeyNotFound)
	}
	if err != nil {
		return LicenseOutcome{HTTP: http.StatusInternalServerError, Err: "internal error"}
	}
	out := LicenseOutcome{Key: &k, Meta: s.licenseMeta(k), HTTP: http.StatusOK}
	if instanceID != "" {
		var inst db.LicenseKeyInstance
		err := s.DB.WithContext(ctx).Where("license_key_id = ? AND identifier = ?", k.ID, instanceID).First(&inst).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			out.HTTP, out.Err = http.StatusNotFound, errInstanceNotFound
			return out
		}
		if err != nil {
			return LicenseOutcome{HTTP: http.StatusInternalServerError, Err: "internal error"}
		}
		out.Instance = &inst
	}
	switch resource.LicenseStatus(&k, db.Now()) {
	case "disabled":
		out.HTTP, out.Err = http.StatusBadRequest, errDisabled
	case "expired":
		out.HTTP, out.Err = http.StatusBadRequest, errExpired
	default:
		out.OK = true
	}
	return out
}

// DeactivateLicense removes an instance and frees one activation.
func (s *Svc) DeactivateLicense(ctx context.Context, key, instanceID string) LicenseOutcome {
	var out LicenseOutcome
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		k, err := s.findKey(tx, key)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			out = notFoundOutcome(errKeyNotFound)
			return nil
		}
		if err != nil {
			return err
		}
		out.Key = k
		res := tx.Where("license_key_id = ? AND identifier = ?", k.ID, instanceID).Delete(&db.LicenseKeyInstance{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			out.HTTP, out.Err = http.StatusNotFound, errInstanceNotFound
			return nil
		}
		var n int64
		tx.Model(&db.LicenseKeyInstance{}).Where("license_key_id = ?", k.ID).Count(&n)
		k.InstancesCount = int(n)
		if err := tx.Model(k).Updates(map[string]any{"instances_count": k.InstancesCount, "updated_at": db.Now()}).Error; err != nil {
			return err
		}
		out.OK, out.HTTP = true, http.StatusOK
		return nil
	})
	if err != nil {
		return LicenseOutcome{HTTP: http.StatusInternalServerError, Err: "internal error"}
	}
	if out.Key != nil {
		out.Meta = s.licenseMeta(*out.Key)
	}
	return out
}

// LicensePatch is a partial update of a license key (tri-state fields).
type LicensePatch struct {
	ActivationLimitSet bool
	ActivationLimit    *int
	ExpiresAtSet       bool
	ExpiresAt          *time.Time
	Disabled           *bool
}

func (s *Svc) PatchLicenseKey(ctx context.Context, id int64, p LicensePatch) (*db.LicenseKey, error) {
	var k db.LicenseKey
	buf := &eventBuf{}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&k, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return NotFound("License key")
			}
			return err
		}
		upd := map[string]any{"updated_at": db.Now()}
		if p.ActivationLimitSet {
			if p.ActivationLimit != nil && *p.ActivationLimit < 0 {
				return Invalid("/data/attributes/activation_limit", "The activation limit must be at least 0.")
			}
			upd["activation_limit"] = p.ActivationLimit
		}
		if p.ExpiresAtSet {
			upd["expires_at"] = p.ExpiresAt
		}
		if p.Disabled != nil {
			upd["disabled"] = *p.Disabled
		}
		if err := tx.Model(&k).Updates(upd).Error; err != nil {
			return err
		}
		if err := tx.First(&k, id).Error; err != nil {
			return err
		}
		buf.add(Event{Name: "license_key_updated", StoreID: k.StoreID, TestMode: k.TestMode, ResourceType: "license-keys", ResourceID: k.ID})
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.flush(ctx, buf)
	return &k, nil
}

// LicenseSpec describes how new keys are derived from a variant.
func licenseSpec(v db.Variant, now time.Time, subscriptionLinked bool) (limit *int, expires *time.Time) {
	if !v.IsLicenseLimitUnlimited {
		l := v.LicenseActivationLimit
		limit = &l
	}
	if !subscriptionLinked && !v.IsLicenseLengthUnlimited && v.LicenseLengthValue > 0 {
		e := AddLicenseLength(now, v.LicenseLengthValue, v.LicenseLengthUnit)
		expires = &e
	}
	return
}

// IssueLicenseKey creates one key for an order item inside tx.
func (s *Svc) IssueLicenseKey(tx *gorm.DB, store db.Store, c db.Customer, o db.Order, it db.OrderItem, v db.Variant, subscriptionID *int64) (*db.LicenseKey, error) {
	now := db.Now()
	limit, expires := licenseSpec(v, now, subscriptionID != nil)
	k := db.LicenseKey{
		StoreID: store.ID, CustomerID: c.ID, OrderID: o.ID, OrderItemID: it.ID, ProductID: it.ProductID, VariantID: v.ID,
		SubscriptionID: subscriptionID, UserName: o.UserName, UserEmail: o.UserEmail, Key: NewLicenseKeyString(),
		ActivationLimit: limit, ExpiresAt: expires, TestMode: o.TestMode, CreatedAt: now, UpdatedAt: now,
	}
	if err := tx.Create(&k).Error; err != nil {
		return nil, err
	}
	return &k, nil
}

// SyncSubscriptionLicenses aligns the keys tied to a subscription with its
// lifecycle: cancelled keys expire at ends_at, resumed/active keys are
// perpetual again, expired subscriptions expire their keys immediately.
func (s *Svc) SyncSubscriptionLicenses(tx *gorm.DB, sub db.Subscription) ([]db.LicenseKey, error) {
	var keys []db.LicenseKey
	if err := tx.Where("subscription_id = ?", sub.ID).Find(&keys).Error; err != nil {
		return nil, err
	}
	var changed []db.LicenseKey
	for i := range keys {
		k := &keys[i]
		var want *time.Time
		switch sub.Status {
		case "expired":
			t := db.Now()
			if sub.EndsAt != nil {
				t = *sub.EndsAt
			}
			want = &t
		case "cancelled":
			want = sub.EndsAt
		default:
			want = nil
		}
		if equalTimePtr(k.ExpiresAt, want) {
			continue
		}
		k.ExpiresAt = want
		k.UpdatedAt = db.Now()
		if err := tx.Model(k).Updates(map[string]any{"expires_at": want, "updated_at": k.UpdatedAt}).Error; err != nil {
			return nil, err
		}
		changed = append(changed, *k)
	}
	return changed, nil
}

func equalTimePtr(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}
