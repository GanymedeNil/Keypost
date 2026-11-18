package db

import "time"

// Conventions
//   - Integer primary keys. Rows imported from Lemon Squeezy keep their
//     original ids so references held by customers' apps stay valid.
//   - Money is stored as integer minor units (cents).
//   - Resource tables carry TestMode; API keys in test mode only see test data.

// ---------------------------------------------------------------- identity

type User struct {
	ID                 int64   `gorm:"primaryKey"`
	Email              string  `gorm:"size:255;uniqueIndex;not null"`
	Name               string  `gorm:"size:255"`
	PasswordHash       string  `gorm:"size:255"`
	Role               string  `gorm:"size:20;not null;default:viewer"` // owner | admin | viewer
	StripeAccountID    *string `gorm:"size:64;uniqueIndex"`
	StripeAccountLabel string  `gorm:"size:255"`
	Disabled           bool
	LastLoginAt        *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time

	// Two-step verification (TOTP). Secrets are encrypted with APP_SECRET, recovery
	// codes are stored as keyed hashes.
	MFASecret        string              `gorm:"column:mfa_secret;size:512"`
	MFAPendingSecret string              `gorm:"column:mfa_pending_secret;size:512"` // enrolment started, not confirmed yet
	MFAEnabledAt     *time.Time          `gorm:"column:mfa_enabled_at"`
	MFALastStep      int64               `gorm:"column:mfa_last_step"` // last accepted 30 s step: a code works once
	MFAFailed        int                 `gorm:"column:mfa_failed"`
	MFALockedUntil   *time.Time          `gorm:"column:mfa_locked_until"`
	MFARecoveryCodes JSONField[[]string] `gorm:"column:mfa_recovery_codes"`
}

type AdminSession struct {
	ID         string `gorm:"primaryKey;size:64"` // sha256 of the cookie token
	UserID     int64  `gorm:"index;not null"`
	IP         string `gorm:"size:64"`
	UserAgent  string `gorm:"size:255"`
	ExpiresAt  time.Time
	CreatedAt  time.Time
	LastSeenAt time.Time
	// MFAVerified: the second step was passed in this session (see docs/ADMIN_API.md §1.1).
	MFAVerified bool `gorm:"column:mfa_verified"`
}

type APIKey struct {
	ID         int64  `gorm:"primaryKey"`
	UserID     int64  `gorm:"index;not null"`
	Name       string `gorm:"size:255"`
	Prefix     string `gorm:"size:24"` // first characters of the token, shown in the UI
	KeyHash    string `gorm:"size:64;uniqueIndex;not null"`
	TestMode   bool
	LastUsedAt *time.Time
	ExpiresAt  *time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

type AuditLog struct {
	ID           int64 `gorm:"primaryKey"`
	UserID       *int64
	UserEmail    string `gorm:"size:255"`
	Action       string `gorm:"size:100;index"`
	ResourceType string `gorm:"size:50;index"`
	ResourceID   string `gorm:"size:100"`
	Details      JSONField[map[string]any]
	IP           string    `gorm:"size:64"`
	CreatedAt    time.Time `gorm:"index"`
}

type Setting struct {
	Key       string `gorm:"primaryKey;size:100"`
	Value     Secret // always encrypted at rest
	UpdatedAt time.Time
}

// ---------------------------------------------------------------- catalogue

type Store struct {
	ID              int64  `gorm:"primaryKey"`
	Name            string `gorm:"size:255;not null"`
	Slug            string `gorm:"size:100;uniqueIndex;not null"`
	Domain          string `gorm:"size:255"`
	URL             string `gorm:"size:512"`
	AvatarURL       string `gorm:"size:512"`
	Plan            string `gorm:"size:50;default:self-hosted"`
	Country         string `gorm:"size:2;default:US"`
	Currency        string `gorm:"size:3;default:USD"`
	SupportEmail    string `gorm:"size:255"`
	EmailFromName   string `gorm:"size:255"`
	ReceiptSubject  string `gorm:"size:255"`
	ReceiptIntro    string `gorm:"type:text"`
	StripeAccountID string `gorm:"size:64;index"` // acct_… that owns this store (optional)
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type Product struct {
	ID              int64  `gorm:"primaryKey"`
	StoreID         int64  `gorm:"index;not null"`
	Name            string `gorm:"size:255;not null"`
	Slug            string `gorm:"size:255;index"`
	Description     string `gorm:"type:text"`
	Status          string `gorm:"size:20;default:draft"` // draft | published
	ThumbURL        string `gorm:"size:1024"`
	LargeThumbURL   string `gorm:"size:1024"`
	StripeProductID string `gorm:"size:64;index"`
	TestMode        bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type VariantLink struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type Variant struct {
	ID                       int64  `gorm:"primaryKey"`
	ProductID                int64  `gorm:"index;not null"`
	Name                     string `gorm:"size:255;not null"`
	Slug                     string `gorm:"size:64;uniqueIndex;not null"`
	Description              string `gorm:"type:text"`
	Links                    JSONField[[]VariantLink]
	Sort                     int
	Status                   string `gorm:"size:20;default:published"` // pending | draft | published
	HasLicenseKeys           bool
	LicenseActivationLimit   int
	IsLicenseLimitUnlimited  bool
	LicenseLengthValue       int
	LicenseLengthUnit        string `gorm:"size:10;default:years"` // days | months | years
	IsLicenseLengthUnlimited bool
	StripeProductID          string `gorm:"size:64;index"`
	TestMode                 bool
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

type PriceTier struct {
	LastUnit         any     `json:"last_unit"`
	UnitPrice        *int64  `json:"unit_price"`
	UnitPriceDecimal *string `json:"unit_price_decimal"`
	FixedFee         *int64  `json:"fixed_fee"`
}

type Price struct {
	ID                      int64   `gorm:"primaryKey"`
	VariantID               int64   `gorm:"index;not null"`
	Category                string  `gorm:"size:20;default:one_time"` // one_time | subscription | lead_magnet | pwyw
	Scheme                  string  `gorm:"size:20;default:standard"`
	UsageAggregation        *string `gorm:"size:30"`
	UnitPrice               int64
	UnitPriceDecimal        *string `gorm:"size:50"`
	SetupFeeEnabled         bool
	SetupFee                *int64
	PackageSize             int `gorm:"default:1"`
	Tiers                   JSONField[[]PriceTier]
	RenewalIntervalUnit     *string `gorm:"size:10"` // day | week | month | year
	RenewalIntervalQuantity *int
	TrialIntervalUnit       *string `gorm:"size:10"`
	TrialIntervalQuantity   *int
	MinPrice                *int64
	SuggestedPrice          *int64
	TaxCode                 string `gorm:"size:20;default:eservice"`
	Currency                string `gorm:"size:3;default:USD"`
	StripePriceID           string `gorm:"size:64;index"`
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

type File struct {
	ID             int64  `gorm:"primaryKey"`
	VariantID      int64  `gorm:"index;not null"`
	Identifier     string `gorm:"size:64;uniqueIndex;not null"` // uuid
	Name           string `gorm:"size:512;not null"`
	Extension      string `gorm:"size:20"`
	Size           int64
	Version        string `gorm:"size:50"`
	Sort           int
	Status         string `gorm:"size:20;default:published"` // draft | published
	StorageDriver  string `gorm:"size:10"`                   // local | s3
	StorageKey     string `gorm:"size:1024"`
	ContentType    string `gorm:"size:255"`
	SHA256         string `gorm:"size:64"`
	DownloadsCount int64
	TestMode       bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type DownloadLog struct {
	ID        int64     `gorm:"primaryKey"`
	FileID    int64     `gorm:"index:idx_dl_file_ip,priority:1;not null"`
	OrderID   *int64    `gorm:"index"`
	IP        string    `gorm:"size:64;index:idx_dl_file_ip,priority:2"`
	UserAgent string    `gorm:"size:255"`
	CreatedAt time.Time `gorm:"index:idx_dl_file_ip,priority:3"`
}

// ---------------------------------------------------------------- customers & orders

type Customer struct {
	ID               int64  `gorm:"primaryKey"`
	StoreID          int64  `gorm:"uniqueIndex:idx_customer_email,priority:1;not null"`
	TestMode         bool   `gorm:"uniqueIndex:idx_customer_email,priority:2"`
	Email            string `gorm:"size:255;uniqueIndex:idx_customer_email,priority:3;not null"`
	Name             string `gorm:"size:255"`
	Status           string `gorm:"size:30;default:subscribed"`
	City             string `gorm:"size:255"`
	Region           string `gorm:"size:255"`
	Country          string `gorm:"size:2"`
	StripeCustomerID string `gorm:"size:64;index"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type Order struct {
	ID                    int64  `gorm:"primaryKey"`
	StoreID               int64  `gorm:"uniqueIndex:idx_order_number,priority:1;not null"`
	CustomerID            int64  `gorm:"index;not null"`
	Identifier            string `gorm:"size:64;uniqueIndex;not null"`
	OrderNumber           int64  `gorm:"uniqueIndex:idx_order_number,priority:2"`
	UserName              string `gorm:"size:255"`
	UserEmail             string `gorm:"size:255;index"`
	Currency              string `gorm:"size:3;default:USD"`
	CurrencyRate          string `gorm:"size:20;default:1.0000"`
	Subtotal              int64
	SetupFee              int64
	DiscountTotal         int64
	Tax                   int64
	Total                 int64
	RefundedAmount        int64
	SubtotalUSD           int64
	SetupFeeUSD           int64
	DiscountTotalUSD      int64
	TaxUSD                int64
	TotalUSD              int64
	RefundedAmountUSD     int64
	TaxName               *string `gorm:"size:50"`
	TaxRate               string  `gorm:"size:20;default:0.00"`
	TaxInclusive          bool
	Status                string `gorm:"size:20;default:paid;index"` // pending | failed | paid | refunded | partial_refund | fraudulent
	Refunded              bool
	RefundedAt            *time.Time
	AffiliateID           *int64
	ReferralAmount        *int64
	City                  string `gorm:"size:255"`
	Country               string `gorm:"size:2"`
	PostalCode            string `gorm:"size:32"`
	TaxNumber             string `gorm:"size:64"`
	CardBrand             string `gorm:"size:20"`
	CardLastFour          string `gorm:"size:4"`
	CustomData            JSONField[map[string]any]
	StripeCheckoutSession *string `gorm:"size:100;uniqueIndex:idx_order_session"`
	StripePaymentIntent   string  `gorm:"size:100;index"`
	StripeInvoiceID       string  `gorm:"size:100;index"`
	StripeChargeID        string  `gorm:"size:100;index"`
	StripeReceiptURL      string  `gorm:"size:1024"`
	StripeCustomerID      string  `gorm:"size:64"`
	Source                string  `gorm:"size:20;default:stripe"` // stripe | lemonsqueezy | manual
	TestMode              bool
	CreatedAt             time.Time `gorm:"index"`
	UpdatedAt             time.Time
}

type OrderItem struct {
	ID          int64 `gorm:"primaryKey"`
	OrderID     int64 `gorm:"index;not null"`
	ProductID   int64 `gorm:"index"`
	VariantID   int64 `gorm:"index"`
	PriceID     *int64
	ProductName string `gorm:"size:255"`
	VariantName string `gorm:"size:255"`
	Price       int64
	Quantity    int `gorm:"default:1"`
	TestMode    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ---------------------------------------------------------------- subscriptions

type Subscription struct {
	ID                   int64 `gorm:"primaryKey"`
	StoreID              int64 `gorm:"index;not null"`
	CustomerID           int64 `gorm:"index;not null"`
	OrderID              int64 `gorm:"index"`
	OrderItemID          int64
	ProductID            int64 `gorm:"index"`
	VariantID            int64 `gorm:"index"`
	PriceID              *int64
	ProductName          string `gorm:"size:255"`
	VariantName          string `gorm:"size:255"`
	UserName             string `gorm:"size:255"`
	UserEmail            string `gorm:"size:255;index"`
	Status               string `gorm:"size:20;default:active;index"` // on_trial | active | paused | past_due | unpaid | cancelled | expired
	CardBrand            string `gorm:"size:20"`
	CardLastFour         string `gorm:"size:4"`
	PaymentProcessor     string `gorm:"size:20;default:stripe"`
	Pause                JSONField[*SubscriptionPause]
	Cancelled            bool
	TrialEndsAt          *time.Time
	BillingAnchor        int
	Quantity             int `gorm:"default:1"`
	RenewsAt             *time.Time
	EndsAt               *time.Time
	StripeSubscriptionID *string `gorm:"size:64;uniqueIndex"`
	StripeCustomerID     string  `gorm:"size:64;index"`
	CustomData           JSONField[map[string]any]
	TestMode             bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type SubscriptionPause struct {
	Mode      string     `json:"mode"` // void | free
	ResumesAt *time.Time `json:"resumes_at"`
}

type SubscriptionItem struct {
	ID                     int64 `gorm:"primaryKey"`
	SubscriptionID         int64 `gorm:"index;not null"`
	PriceID                int64
	Quantity               int
	IsUsageBased           bool
	StripeSubscriptionItem string `gorm:"size:64;index"`
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type SubscriptionInvoice struct {
	ID                  int64  `gorm:"primaryKey"`
	StoreID             int64  `gorm:"index;not null"`
	SubscriptionID      int64  `gorm:"index;not null"`
	CustomerID          int64  `gorm:"index"`
	UserName            string `gorm:"size:255"`
	UserEmail           string `gorm:"size:255"`
	BillingReason       string `gorm:"size:20"` // initial | renewal | updated
	CardBrand           string `gorm:"size:20"`
	CardLastFour        string `gorm:"size:4"`
	Currency            string `gorm:"size:3;default:USD"`
	CurrencyRate        string `gorm:"size:20;default:1.0000"`
	Status              string `gorm:"size:20;default:paid"` // pending | paid | void | refunded | partial_refund
	Refunded            bool
	RefundedAt          *time.Time
	Subtotal            int64
	DiscountTotal       int64
	Tax                 int64
	TaxInclusive        bool
	Total               int64
	RefundedAmount      int64
	SubtotalUSD         int64
	DiscountTotalUSD    int64
	TaxUSD              int64
	TotalUSD            int64
	RefundedAmountUSD   int64
	InvoiceURL          string  `gorm:"size:1024"`
	StripeInvoiceID     *string `gorm:"size:100;uniqueIndex"`
	StripePaymentIntent string  `gorm:"size:100;index"`
	StripeChargeID      string  `gorm:"size:100;index"`
	TestMode            bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// ---------------------------------------------------------------- licensing

type LicenseKey struct {
	ID              int64  `gorm:"primaryKey"`
	StoreID         int64  `gorm:"index;not null"`
	CustomerID      int64  `gorm:"index"`
	OrderID         int64  `gorm:"index"`
	OrderItemID     int64  `gorm:"index"`
	ProductID       int64  `gorm:"index"`
	VariantID       int64  `gorm:"index"`
	SubscriptionID  *int64 `gorm:"index"`
	UserName        string `gorm:"size:255"`
	UserEmail       string `gorm:"size:255;index"`
	Key             string `gorm:"size:255;uniqueIndex;not null"`
	ActivationLimit *int   // nil = unlimited
	InstancesCount  int
	Disabled        bool
	ExpiresAt       *time.Time
	TestMode        bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type LicenseKeyInstance struct {
	ID           int64  `gorm:"primaryKey"`
	LicenseKeyID int64  `gorm:"index;not null"`
	Identifier   string `gorm:"size:64;uniqueIndex;not null"` // uuid == instance_id
	Name         string `gorm:"size:255"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Checkout mirrors a Lemon Squeezy checkout object; it wraps a Stripe Checkout Session.
type Checkout struct {
	ID              string `gorm:"primaryKey;size:64"` // uuid
	StoreID         int64  `gorm:"index;not null"`
	VariantID       int64  `gorm:"index"`
	StripeSessionID string `gorm:"size:100;index"`
	URL             string `gorm:"type:text"`
	RedirectURL     string `gorm:"type:text"`
	Attributes      JSONField[map[string]any]
	ExpiresAt       *time.Time
	TestMode        bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ---------------------------------------------------------------- outbound webhooks

type Webhook struct {
	ID         int64  `gorm:"primaryKey"`
	StoreID    int64  `gorm:"index;not null"`
	URL        string `gorm:"size:1024;not null"`
	Events     JSONField[[]string]
	Secret     Secret
	Disabled   bool
	LastSentAt *time.Time
	TestMode   bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type WebhookDelivery struct {
	ID             int64  `gorm:"primaryKey"`
	WebhookID      int64  `gorm:"index;not null"`
	StoreID        int64  `gorm:"index"`
	EventName      string `gorm:"size:64;index"`
	ResourceType   string `gorm:"size:50"`
	ResourceID     string `gorm:"size:50"`
	Payload        string `gorm:"type:text"`
	Status         string `gorm:"size:20;index;default:pending"` // pending | success | failed
	Attempts       int
	NextAttemptAt  time.Time `gorm:"index"`
	LastStatusCode int
	LastResponse   string `gorm:"type:text"`
	LastError      string `gorm:"type:text"`
	DurationMS     int
	CreatedAt      time.Time `gorm:"index"`
	DeliveredAt    *time.Time
}

// ---------------------------------------------------------------- stripe

type StripeEvent struct {
	ID          string `gorm:"primaryKey;size:100"` // evt_…
	Type        string `gorm:"size:100;index"`
	Account     string `gorm:"size:64"`
	Livemode    bool
	Status      string `gorm:"size:20;index"` // received | processed | ignored | failed
	Attempts    int
	Error       string    `gorm:"type:text"`
	Payload     string    `gorm:"type:text"`
	ReceivedAt  time.Time `gorm:"index"`
	ProcessedAt *time.Time
}

// ---------------------------------------------------------------- customer auth, email

type CustomerLoginToken struct {
	ID        int64  `gorm:"primaryKey"`
	TokenHash string `gorm:"size:64;uniqueIndex;not null"`
	Email     string `gorm:"size:255;index"`
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

type EmailLog struct {
	ID        int64 `gorm:"primaryKey"`
	StoreID   int64
	OrderID   *int64    `gorm:"index"`
	To        string    `gorm:"column:to_addr;size:255;index"`
	Subject   string    `gorm:"size:512"`
	Template  string    `gorm:"size:50"`
	Status    string    `gorm:"size:20"` // sent | failed | skipped
	Error     string    `gorm:"type:text"`
	CreatedAt time.Time `gorm:"index"`
}

// ---------------------------------------------------------------- imports

type ImportJob struct {
	ID         int64  `gorm:"primaryKey"`
	Kind       string `gorm:"size:50;index"`                 // lemonsqueezy_api | lemonsqueezy_files | stripe_mapping | stripe_auto_match | stripe_catalog
	Status     string `gorm:"size:20;index;default:pending"` // pending | running | completed | failed | cancelled
	Options    JSONField[map[string]any]
	Progress   JSONField[map[string]any]
	Error      string `gorm:"type:text"`
	CreatedBy  string `gorm:"size:255"`
	StartedAt  *time.Time
	FinishedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type ImportLog struct {
	ID        int64  `gorm:"primaryKey"`
	JobID     int64  `gorm:"index;not null"`
	Level     string `gorm:"size:10"` // info | warn | error
	Message   string `gorm:"type:text"`
	CreatedAt time.Time
}

// AllModels lists every table for AutoMigrate.
func AllModels() []any {
	return []any{
		&User{}, &AdminSession{}, &APIKey{}, &AuditLog{}, &Setting{},
		&Store{}, &Product{}, &Variant{}, &Price{}, &File{}, &DownloadLog{},
		&Customer{}, &Order{}, &OrderItem{},
		&Subscription{}, &SubscriptionItem{}, &SubscriptionInvoice{},
		&LicenseKey{}, &LicenseKeyInstance{},
		&Checkout{}, &Webhook{}, &WebhookDelivery{}, &StripeEvent{},
		&CustomerLoginToken{}, &EmailLog{}, &ImportJob{}, &ImportLog{},
	}
}
