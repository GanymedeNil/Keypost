package resource

import (
	"sort"

	"gorm.io/gorm"

	"fulfillment/internal/db"
)

// ---------------------------------------------------------------- helpers

func fetchMap[M any](g *gorm.DB, ids []int64, idOf func(M) int64) map[int64]M {
	out := map[int64]M{}
	ids = uniq(ids)
	if len(ids) == 0 {
		return out
	}
	var rows []M
	if err := g.Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return out
	}
	for _, r := range rows {
		out[idOf(r)] = r
	}
	return out
}

func uniq(ids []int64) []int64 {
	seen := map[int64]bool{}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id != 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// ---------------------------------------------------------------- store

type StoreAttrs struct {
	Name            string  `json:"name"`
	Slug            string  `json:"slug"`
	Domain          string  `json:"domain"`
	URL             string  `json:"url"`
	AvatarURL       string  `json:"avatar_url"`
	Plan            string  `json:"plan"`
	Country         string  `json:"country"`
	CountryNicename string  `json:"country_nicename"`
	Currency        string  `json:"currency"`
	TotalSales      int64   `json:"total_sales"`
	TotalRevenue    int64   `json:"total_revenue"`
	ThirtyDaySales  int64   `json:"thirty_day_sales"`
	ThirtyDayRev    int64   `json:"thirty_day_revenue"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
	SupportEmail    *string `json:"support_email,omitempty"`
}

type StoreStats struct{ Sales, Revenue, Sales30, Revenue30 int64 }

func (b *Builder) StoreStats(storeID int64, testMode bool) StoreStats {
	var st StoreStats
	type row struct{ N, Sum int64 }
	var r row
	b.DB.Model(&db.Order{}).Select("COUNT(*) AS n, COALESCE(SUM(total_usd - refunded_amount_usd),0) AS sum").
		Where("store_id = ? AND test_mode = ? AND status IN ?", storeID, testMode, []string{"paid", "partial_refund", "refunded"}).Scan(&r)
	st.Sales, st.Revenue = r.N, r.Sum
	b.DB.Model(&db.Order{}).Select("COUNT(*) AS n, COALESCE(SUM(total_usd - refunded_amount_usd),0) AS sum").
		Where("store_id = ? AND test_mode = ? AND status IN ? AND created_at >= ?", storeID, testMode, []string{"paid", "partial_refund", "refunded"}, b.now().AddDate(0, 0, -30)).Scan(&r)
	st.Sales30, st.Revenue30 = r.N, r.Sum
	return st
}

var countryNames = map[string]string{
	"US": "United States", "GB": "United Kingdom", "DE": "Germany", "FR": "France", "CA": "Canada", "AU": "Australia",
	"CN": "China", "JP": "Japan", "IN": "India", "BR": "Brazil", "NL": "Netherlands", "ES": "Spain", "IT": "Italy",
	"SE": "Sweden", "NO": "Norway", "DK": "Denmark", "FI": "Finland", "PL": "Poland", "CH": "Switzerland",
	"SG": "Singapore", "HK": "Hong Kong", "KR": "South Korea", "MX": "Mexico", "IE": "Ireland", "AT": "Austria",
	"BE": "Belgium", "PT": "Portugal", "NZ": "New Zealand", "TW": "Taiwan", "RU": "Russia", "UA": "Ukraine",
}

func CountryName(code string) string {
	if n, ok := countryNames[code]; ok {
		return n
	}
	return code
}

func (b *Builder) Store(s db.Store, testMode bool) Resource {
	st := b.StoreStats(s.ID, testMode)
	return b.res("stores", s.ID, StoreAttrs{
		Name: s.Name, Slug: s.Slug, Domain: s.Domain, URL: s.URL, AvatarURL: s.AvatarURL, Plan: s.Plan,
		Country: s.Country, CountryNicename: CountryName(s.Country), Currency: s.Currency,
		TotalSales: st.Sales, TotalRevenue: st.Revenue, ThirtyDaySales: st.Sales30, ThirtyDayRev: st.Revenue30,
		CreatedAt: TS(s.CreatedAt), UpdatedAt: TS(s.UpdatedAt),
	}, "products", "orders", "subscriptions", "discounts", "license-keys", "webhooks")
}

// ---------------------------------------------------------------- user

type UserAttrs struct {
	Name            string `json:"name"`
	Email           string `json:"email"`
	Color           string `json:"color"`
	AvatarURL       string `json:"avatar_url"`
	HasCustomAvatar bool   `json:"has_custom_avatar"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
}

func (b *Builder) User(u db.User) Resource {
	return Resource{Type: "users", ID: idStr(u.ID), Attributes: UserAttrs{
		Name: u.Name, Email: u.Email, Color: "#7047EB", AvatarURL: "", CreatedAt: TS(u.CreatedAt), UpdatedAt: TS(u.UpdatedAt),
	}, Links: b.self("users", u.ID)}
}

// ---------------------------------------------------------------- customers

type CustomerAttrs struct {
	StoreID                       int64        `json:"store_id"`
	Name                          string       `json:"name"`
	Email                         string       `json:"email"`
	Status                        string       `json:"status"`
	City                          *string      `json:"city"`
	Region                        *string      `json:"region"`
	Country                       *string      `json:"country"`
	TotalRevenueCurrency          int64        `json:"total_revenue_currency"`
	MRR                           int64        `json:"mrr"`
	StatusFormatted               string       `json:"status_formatted"`
	CountryFormatted              *string      `json:"country_formatted"`
	TotalRevenueCurrencyFormatted string       `json:"total_revenue_currency_formatted"`
	MRRFormatted                  string       `json:"mrr_formatted"`
	URLs                          CustomerURLs `json:"urls"`
	CreatedAt                     string       `json:"created_at"`
	UpdatedAt                     string       `json:"updated_at"`
	TestMode                      bool         `json:"test_mode"`
}

type CustomerURLs struct {
	CustomerPortal *string `json:"customer_portal"`
}

type CustomerStats struct {
	Revenue, MRR int64
	HasSubs      bool
}

// monthlyAmount normalises a recurring amount to a monthly value.
func monthlyAmount(amount int64, unit string, qty int) int64 {
	if qty <= 0 {
		qty = 1
	}
	per := float64(amount)
	switch unit {
	case "day":
		per = per * 30 / float64(qty)
	case "week":
		per = per * 52 / 12 / float64(qty)
	case "year":
		per = per / 12 / float64(qty)
	default:
		per = per / float64(qty)
	}
	return int64(per + 0.5)
}

func (b *Builder) CustomerStats(ids []int64) map[int64]CustomerStats {
	out := map[int64]CustomerStats{}
	ids = uniq(ids)
	if len(ids) == 0 {
		return out
	}
	type rev struct {
		CustomerID int64
		Sum        int64
	}
	var rs []rev
	b.DB.Model(&db.Order{}).Select("customer_id, COALESCE(SUM(total_usd - refunded_amount_usd),0) AS sum").
		Where("customer_id IN ? AND status IN ?", ids, []string{"paid", "partial_refund", "refunded"}).Group("customer_id").Scan(&rs)
	for _, r := range rs {
		s := out[r.CustomerID]
		s.Revenue += r.Sum
		out[r.CustomerID] = s
	}
	var rs2 []rev
	b.DB.Model(&db.SubscriptionInvoice{}).Select("customer_id, COALESCE(SUM(total_usd - refunded_amount_usd),0) AS sum").
		Where("customer_id IN ? AND billing_reason <> ? AND status IN ?", ids, "initial", []string{"paid", "partial_refund", "refunded"}).Group("customer_id").Scan(&rs2)
	for _, r := range rs2 {
		s := out[r.CustomerID]
		s.Revenue += r.Sum
		out[r.CustomerID] = s
	}
	var subs []db.Subscription
	b.DB.Where("customer_id IN ?", ids).Find(&subs)
	priceIDs := []int64{}
	for _, s := range subs {
		if s.PriceID != nil {
			priceIDs = append(priceIDs, *s.PriceID)
		}
	}
	prices := fetchMap(b.DB.Model(&db.Price{}), priceIDs, func(p db.Price) int64 { return p.ID })
	for _, s := range subs {
		st := out[s.CustomerID]
		st.HasSubs = true
		if (s.Status == "active" || s.Status == "on_trial" || s.Status == "past_due") && s.PriceID != nil {
			if p, ok := prices[*s.PriceID]; ok && p.RenewalIntervalUnit != nil {
				q := 1
				if p.RenewalIntervalQuantity != nil {
					q = *p.RenewalIntervalQuantity
				}
				st.MRR += monthlyAmount(p.UnitPrice*int64(maxInt(s.Quantity, 1)), *p.RenewalIntervalUnit, q)
			}
		}
		out[s.CustomerID] = st
	}
	return out
}

// MonthlyAmount normalises a recurring amount to a monthly value (MRR).
func MonthlyAmount(amount int64, unit string, qty int) int64 { return monthlyAmount(amount, unit, qty) }

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (b *Builder) Customers(cs []db.Customer) []Resource {
	ids := make([]int64, len(cs))
	for i, c := range cs {
		ids[i] = c.ID
	}
	stats := b.CustomerStats(ids)
	out := make([]Resource, len(cs))
	for i, c := range cs {
		st := stats[c.ID]
		var portal *string
		if st.HasSubs {
			p := b.URLs.Portal(c.ID, 0, b.now(), true)
			portal = &p
		}
		var country *string
		if c.Country != "" {
			n := CountryName(c.Country)
			country = &n
		}
		out[i] = b.res("customers", c.ID, CustomerAttrs{
			StoreID: c.StoreID, Name: c.Name, Email: c.Email, Status: c.Status,
			City: strOrNil(c.City), Region: strOrNil(c.Region), Country: strOrNil(c.Country),
			TotalRevenueCurrency: st.Revenue, MRR: st.MRR, StatusFormatted: FormatStatus(c.Status), CountryFormatted: country,
			TotalRevenueCurrencyFormatted: FormatMoney(st.Revenue, "USD"), MRRFormatted: FormatMoney(st.MRR, "USD"),
			URLs: CustomerURLs{CustomerPortal: portal}, CreatedAt: TS(c.CreatedAt), UpdatedAt: TS(c.UpdatedAt), TestMode: c.TestMode,
		}, "store", "orders", "subscriptions", "license-keys")
	}
	return out
}

// ---------------------------------------------------------------- products / variants / prices

type ProductAttrs struct {
	StoreID            int64   `json:"store_id"`
	Name               string  `json:"name"`
	Slug               string  `json:"slug"`
	Description        string  `json:"description"`
	Status             string  `json:"status"`
	StatusFormatted    string  `json:"status_formatted"`
	ThumbURL           *string `json:"thumb_url"`
	LargeThumbURL      *string `json:"large_thumb_url"`
	Price              int64   `json:"price"`
	PriceFormatted     string  `json:"price_formatted"`
	FromPrice          *int64  `json:"from_price"`
	ToPrice            *int64  `json:"to_price"`
	PayWhatYouWant     bool    `json:"pay_what_you_want"`
	BuyNowURL          string  `json:"buy_now_url"`
	FromPriceFormatted *string `json:"from_price_formatted"`
	ToPriceFormatted   *string `json:"to_price_formatted"`
	CreatedAt          string  `json:"created_at"`
	UpdatedAt          string  `json:"updated_at"`
	TestMode           bool    `json:"test_mode"`
}

// firstPrices returns the lowest-id price for every variant id.
func (b *Builder) firstPrices(variantIDs []int64) map[int64]db.Price {
	out := map[int64]db.Price{}
	variantIDs = uniq(variantIDs)
	if len(variantIDs) == 0 {
		return out
	}
	var ps []db.Price
	b.DB.Where("variant_id IN ?", variantIDs).Order("id").Find(&ps)
	for _, p := range ps {
		if _, ok := out[p.VariantID]; !ok {
			out[p.VariantID] = p
		}
	}
	return out
}

func (b *Builder) Products(ps []db.Product) []Resource {
	ids := make([]int64, len(ps))
	for i, p := range ps {
		ids[i] = p.ID
	}
	var vs []db.Variant
	if len(ids) > 0 {
		b.DB.Where("product_id IN ?", ids).Order("sort, id").Find(&vs)
	}
	vids := make([]int64, len(vs))
	for i, v := range vs {
		vids[i] = v.ID
	}
	prices := b.firstPrices(vids)
	byProduct := map[int64][]db.Variant{}
	for _, v := range vs {
		byProduct[v.ProductID] = append(byProduct[v.ProductID], v)
	}
	out := make([]Resource, len(ps))
	for i, p := range ps {
		cur := "USD"
		var amounts []int64
		pwyw := false
		buy := ""
		for _, v := range byProduct[p.ID] {
			if v.Status == "draft" {
				continue
			}
			if buy == "" {
				buy = b.URLs.Buy(v.Slug)
			}
			if pr, ok := prices[v.ID]; ok {
				amounts = append(amounts, pr.UnitPrice)
				cur = pr.Currency
				pwyw = pwyw || pr.Category == "pwyw"
			}
		}
		sort.Slice(amounts, func(a, c int) bool { return amounts[a] < amounts[c] })
		a := ProductAttrs{
			StoreID: p.StoreID, Name: p.Name, Slug: p.Slug, Description: p.Description, Status: p.Status,
			StatusFormatted: FormatStatus(p.Status), ThumbURL: strOrNil(p.ThumbURL), LargeThumbURL: strOrNil(p.LargeThumbURL),
			PayWhatYouWant: pwyw, BuyNowURL: buy, CreatedAt: TS(p.CreatedAt), UpdatedAt: TS(p.UpdatedAt), TestMode: p.TestMode,
		}
		if len(amounts) > 0 {
			a.Price = amounts[0]
		}
		a.PriceFormatted = FormatMoney(a.Price, cur)
		if len(amounts) > 1 && amounts[0] != amounts[len(amounts)-1] {
			lo, hi := amounts[0], amounts[len(amounts)-1]
			a.FromPrice, a.ToPrice = &lo, &hi
			f, t := FormatMoney(lo, cur), FormatMoney(hi, cur)
			a.FromPriceFormatted, a.ToPriceFormatted = &f, &t
		}
		out[i] = b.res("products", p.ID, a, "store", "variants")
	}
	return out
}

type VariantAttrs struct {
	ProductID                int64            `json:"product_id"`
	Name                     string           `json:"name"`
	Slug                     string           `json:"slug"`
	Description              string           `json:"description"`
	Links                    []db.VariantLink `json:"links"`
	Price                    int64            `json:"price"`
	IsSubscription           bool             `json:"is_subscription"`
	Interval                 *string          `json:"interval"`
	IntervalCount            *int             `json:"interval_count"`
	HasFreeTrial             bool             `json:"has_free_trial"`
	TrialInterval            string           `json:"trial_interval"`
	TrialIntervalCount       int              `json:"trial_interval_count"`
	PayWhatYouWant           bool             `json:"pay_what_you_want"`
	MinPrice                 int64            `json:"min_price"`
	SuggestedPrice           int64            `json:"suggested_price"`
	HasLicenseKeys           bool             `json:"has_license_keys"`
	LicenseActivationLimit   int              `json:"license_activation_limit"`
	IsLicenseLimitUnlimited  bool             `json:"is_license_limit_unlimited"`
	LicenseLengthValue       int              `json:"license_length_value"`
	LicenseLengthUnit        string           `json:"license_length_unit"`
	IsLicenseLengthUnlimited bool             `json:"is_license_length_unlimited"`
	Sort                     int              `json:"sort"`
	Status                   string           `json:"status"`
	StatusFormatted          string           `json:"status_formatted"`
	CreatedAt                string           `json:"created_at"`
	UpdatedAt                string           `json:"updated_at"`
	TestMode                 bool             `json:"test_mode"`
}

func (b *Builder) Variants(vs []db.Variant) []Resource {
	ids := make([]int64, len(vs))
	for i, v := range vs {
		ids[i] = v.ID
	}
	prices := b.firstPrices(ids)
	out := make([]Resource, len(vs))
	for i, v := range vs {
		a := VariantAttrs{
			ProductID: v.ProductID, Name: v.Name, Slug: v.Slug, Description: v.Description, Links: v.Links.V,
			HasLicenseKeys: v.HasLicenseKeys, LicenseActivationLimit: v.LicenseActivationLimit,
			IsLicenseLimitUnlimited: v.IsLicenseLimitUnlimited, LicenseLengthValue: v.LicenseLengthValue,
			LicenseLengthUnit: v.LicenseLengthUnit, IsLicenseLengthUnlimited: v.IsLicenseLengthUnlimited,
			Sort: v.Sort, Status: v.Status, StatusFormatted: FormatStatus(v.Status),
			CreatedAt: TS(v.CreatedAt), UpdatedAt: TS(v.UpdatedAt), TestMode: v.TestMode,
			TrialInterval: "day", TrialIntervalCount: 30,
		}
		if a.Links == nil {
			a.Links = []db.VariantLink{}
		}
		if p, ok := prices[v.ID]; ok {
			a.Price = p.UnitPrice
			a.IsSubscription = p.Category == "subscription"
			a.Interval = p.RenewalIntervalUnit
			a.IntervalCount = p.RenewalIntervalQuantity
			a.HasFreeTrial = p.TrialIntervalUnit != nil
			if p.TrialIntervalUnit != nil {
				a.TrialInterval = *p.TrialIntervalUnit
			}
			if p.TrialIntervalQuantity != nil {
				a.TrialIntervalCount = *p.TrialIntervalQuantity
			}
			a.PayWhatYouWant = p.Category == "pwyw"
			if p.MinPrice != nil {
				a.MinPrice = *p.MinPrice
			}
			if p.SuggestedPrice != nil {
				a.SuggestedPrice = *p.SuggestedPrice
			}
		}
		out[i] = b.res("variants", v.ID, a, "product", "files", "price-model")
	}
	return out
}

type PriceAttrs struct {
	VariantID               int64          `json:"variant_id"`
	Category                string         `json:"category"`
	Scheme                  string         `json:"scheme"`
	UsageAggregation        *string        `json:"usage_aggregation"`
	UnitPrice               int64          `json:"unit_price"`
	UnitPriceDecimal        *string        `json:"unit_price_decimal"`
	SetupFeeEnabled         bool           `json:"setup_fee_enabled"`
	SetupFee                *int64         `json:"setup_fee"`
	PackageSize             int            `json:"package_size"`
	Tiers                   []db.PriceTier `json:"tiers"`
	RenewalIntervalUnit     *string        `json:"renewal_interval_unit"`
	RenewalIntervalQuantity *int           `json:"renewal_interval_quantity"`
	TrialIntervalUnit       *string        `json:"trial_interval_unit"`
	TrialIntervalQuantity   *int           `json:"trial_interval_quantity"`
	MinPrice                *int64         `json:"min_price"`
	SuggestedPrice          *int64         `json:"suggested_price"`
	TaxCode                 string         `json:"tax_code"`
	CreatedAt               string         `json:"created_at"`
	UpdatedAt               string         `json:"updated_at"`
}

func (b *Builder) Prices(ps []db.Price) []Resource {
	out := make([]Resource, len(ps))
	for i, p := range ps {
		tiers := p.Tiers.V
		if tiers == nil {
			tiers = []db.PriceTier{}
		}
		pkg := p.PackageSize
		if pkg == 0 {
			pkg = 1
		}
		out[i] = b.res("prices", p.ID, PriceAttrs{
			VariantID: p.VariantID, Category: p.Category, Scheme: p.Scheme, UsageAggregation: p.UsageAggregation,
			UnitPrice: p.UnitPrice, UnitPriceDecimal: p.UnitPriceDecimal, SetupFeeEnabled: p.SetupFeeEnabled, SetupFee: p.SetupFee,
			PackageSize: pkg, Tiers: tiers, RenewalIntervalUnit: p.RenewalIntervalUnit, RenewalIntervalQuantity: p.RenewalIntervalQuantity,
			TrialIntervalUnit: p.TrialIntervalUnit, TrialIntervalQuantity: p.TrialIntervalQuantity,
			MinPrice: p.MinPrice, SuggestedPrice: p.SuggestedPrice, TaxCode: p.TaxCode,
			CreatedAt: TS(p.CreatedAt), UpdatedAt: TS(p.UpdatedAt),
		}, "variant")
	}
	return out
}

// ---------------------------------------------------------------- files

type FileAttrs struct {
	VariantID     int64  `json:"variant_id"`
	Identifier    string `json:"identifier"`
	Name          string `json:"name"`
	Extension     string `json:"extension"`
	DownloadURL   string `json:"download_url"`
	Size          int64  `json:"size"`
	SizeFormatted string `json:"size_formatted"`
	Version       string `json:"version"`
	Sort          int    `json:"sort"`
	Status        string `json:"status"`
	CreatedAtCam  string `json:"createdAt"`
	UpdatedAtCam  string `json:"updatedAt"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
	TestMode      bool   `json:"test_mode"`
}

func (b *Builder) Files(fs []db.File) []Resource {
	out := make([]Resource, len(fs))
	for i, f := range fs {
		out[i] = b.res("files", f.ID, FileAttrs{
			VariantID: f.VariantID, Identifier: f.Identifier, Name: f.Name, Extension: f.Extension,
			DownloadURL: b.URLs.Download(f.Identifier, 0, b.now(), true), Size: f.Size, SizeFormatted: HumanSize(f.Size),
			Version: f.Version, Sort: f.Sort, Status: f.Status,
			CreatedAtCam: TS(f.CreatedAt), UpdatedAtCam: TS(f.UpdatedAt), CreatedAt: TS(f.CreatedAt), UpdatedAt: TS(f.UpdatedAt), TestMode: f.TestMode,
		}, "variant")
	}
	return out
}

// ---------------------------------------------------------------- orders

type OrderItemBrief struct {
	ID          int64   `json:"id"`
	OrderID     int64   `json:"order_id"`
	ProductID   int64   `json:"product_id"`
	VariantID   int64   `json:"variant_id"`
	ProductName string  `json:"product_name"`
	VariantName string  `json:"variant_name"`
	Price       int64   `json:"price"`
	Quantity    int     `json:"quantity"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
	DeletedAt   *string `json:"deleted_at"`
	TestMode    bool    `json:"test_mode"`
}

type OrderAttrs struct {
	StoreID                 int64           `json:"store_id"`
	CustomerID              int64           `json:"customer_id"`
	Identifier              string          `json:"identifier"`
	OrderNumber             int64           `json:"order_number"`
	UserName                string          `json:"user_name"`
	UserEmail               string          `json:"user_email"`
	Currency                string          `json:"currency"`
	CurrencyRate            string          `json:"currency_rate"`
	Subtotal                int64           `json:"subtotal"`
	SetupFee                int64           `json:"setup_fee"`
	DiscountTotal           int64           `json:"discount_total"`
	Tax                     int64           `json:"tax"`
	Total                   int64           `json:"total"`
	SubtotalUSD             int64           `json:"subtotal_usd"`
	SetupFeeUSD             int64           `json:"setup_fee_usd"`
	DiscountTotalUSD        int64           `json:"discount_total_usd"`
	TaxUSD                  int64           `json:"tax_usd"`
	TotalUSD                int64           `json:"total_usd"`
	TaxName                 *string         `json:"tax_name"`
	TaxRate                 string          `json:"tax_rate"`
	TaxInclusive            bool            `json:"tax_inclusive"`
	Status                  string          `json:"status"`
	StatusFormatted         string          `json:"status_formatted"`
	Refunded                bool            `json:"refunded"`
	RefundedAt              *string         `json:"refunded_at"`
	RefundedAmount          int64           `json:"refunded_amount"`
	RefundedAmountUSD       int64           `json:"refunded_amount_usd"`
	SubtotalFormatted       string          `json:"subtotal_formatted"`
	SetupFeeFormatted       string          `json:"setup_fee_formatted"`
	DiscountTotalFormatted  string          `json:"discount_total_formatted"`
	TaxFormatted            string          `json:"tax_formatted"`
	TotalFormatted          string          `json:"total_formatted"`
	RefundedAmountFormatted string          `json:"refunded_amount_formatted"`
	AffiliateID             *int64          `json:"affiliate_id"`
	ReferralAmount          *int64          `json:"referral_amount"`
	FirstOrderItem          *OrderItemBrief `json:"first_order_item"`
	URLs                    struct {
		Receipt string `json:"receipt"`
	} `json:"urls"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	TestMode  bool   `json:"test_mode"`
}

func (b *Builder) firstItems(orderIDs []int64) map[int64]db.OrderItem {
	out := map[int64]db.OrderItem{}
	orderIDs = uniq(orderIDs)
	if len(orderIDs) == 0 {
		return out
	}
	var items []db.OrderItem
	b.DB.Where("order_id IN ?", orderIDs).Order("id").Find(&items)
	for _, it := range items {
		if _, ok := out[it.OrderID]; !ok {
			out[it.OrderID] = it
		}
	}
	return out
}

func (b *Builder) Orders(os []db.Order) []Resource {
	ids := make([]int64, len(os))
	for i, o := range os {
		ids[i] = o.ID
	}
	first := b.firstItems(ids)
	out := make([]Resource, len(os))
	for i, o := range os {
		a := OrderAttrs{
			StoreID: o.StoreID, CustomerID: o.CustomerID, Identifier: o.Identifier, OrderNumber: o.OrderNumber,
			UserName: o.UserName, UserEmail: o.UserEmail, Currency: o.Currency, CurrencyRate: o.CurrencyRate,
			Subtotal: o.Subtotal, SetupFee: o.SetupFee, DiscountTotal: o.DiscountTotal, Tax: o.Tax, Total: o.Total,
			SubtotalUSD: o.SubtotalUSD, SetupFeeUSD: o.SetupFeeUSD, DiscountTotalUSD: o.DiscountTotalUSD, TaxUSD: o.TaxUSD, TotalUSD: o.TotalUSD,
			TaxName: o.TaxName, TaxRate: o.TaxRate, TaxInclusive: o.TaxInclusive, Status: o.Status, StatusFormatted: FormatStatus(o.Status),
			Refunded: o.Refunded, RefundedAt: TSP(o.RefundedAt), RefundedAmount: o.RefundedAmount, RefundedAmountUSD: o.RefundedAmountUSD,
			SubtotalFormatted: FormatMoney(o.Subtotal, o.Currency), SetupFeeFormatted: FormatMoney(o.SetupFee, o.Currency),
			DiscountTotalFormatted: FormatMoney(o.DiscountTotal, o.Currency), TaxFormatted: FormatMoney(o.Tax, o.Currency),
			TotalFormatted: FormatMoney(o.Total, o.Currency), RefundedAmountFormatted: FormatMoney(o.RefundedAmount, o.Currency),
			AffiliateID: o.AffiliateID, ReferralAmount: o.ReferralAmount,
			CreatedAt: TS(o.CreatedAt), UpdatedAt: TS(o.UpdatedAt), TestMode: o.TestMode,
		}
		a.URLs.Receipt = b.URLs.Receipt(o.Identifier)
		if it, ok := first[o.ID]; ok {
			a.FirstOrderItem = &OrderItemBrief{
				ID: it.ID, OrderID: it.OrderID, ProductID: it.ProductID, VariantID: it.VariantID, ProductName: it.ProductName,
				VariantName: it.VariantName, Price: it.Price, Quantity: it.Quantity, CreatedAt: TS(it.CreatedAt), UpdatedAt: TS(it.UpdatedAt), TestMode: it.TestMode,
			}
		}
		out[i] = b.res("orders", o.ID, a, "store", "customer", "order-items", "subscriptions", "license-keys", "discount-redemptions")
	}
	return out
}

type OrderItemAttrs struct {
	OrderID     int64  `json:"order_id"`
	ProductID   int64  `json:"product_id"`
	VariantID   int64  `json:"variant_id"`
	PriceID     *int64 `json:"price_id"`
	ProductName string `json:"product_name"`
	VariantName string `json:"variant_name"`
	Price       int64  `json:"price"`
	Quantity    int    `json:"quantity"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	TestMode    bool   `json:"test_mode"`
}

func (b *Builder) OrderItems(is []db.OrderItem) []Resource {
	out := make([]Resource, len(is))
	for i, it := range is {
		out[i] = b.res("order-items", it.ID, OrderItemAttrs{
			OrderID: it.OrderID, ProductID: it.ProductID, VariantID: it.VariantID, PriceID: it.PriceID,
			ProductName: it.ProductName, VariantName: it.VariantName, Price: it.Price, Quantity: it.Quantity,
			CreatedAt: TS(it.CreatedAt), UpdatedAt: TS(it.UpdatedAt), TestMode: it.TestMode,
		}, "order", "product", "variant")
	}
	return out
}

// ---------------------------------------------------------------- subscriptions

type SubItemBrief struct {
	ID             int64  `json:"id"`
	SubscriptionID int64  `json:"subscription_id"`
	PriceID        int64  `json:"price_id"`
	Quantity       int    `json:"quantity"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type SubscriptionAttrs struct {
	StoreID          int64                 `json:"store_id"`
	CustomerID       int64                 `json:"customer_id"`
	OrderID          int64                 `json:"order_id"`
	OrderItemID      int64                 `json:"order_item_id"`
	ProductID        int64                 `json:"product_id"`
	VariantID        int64                 `json:"variant_id"`
	ProductName      string                `json:"product_name"`
	VariantName      string                `json:"variant_name"`
	UserName         string                `json:"user_name"`
	UserEmail        string                `json:"user_email"`
	Status           string                `json:"status"`
	StatusFormatted  string                `json:"status_formatted"`
	CardBrand        string                `json:"card_brand"`
	CardLastFour     string                `json:"card_last_four"`
	PaymentProcessor string                `json:"payment_processor"`
	Pause            *db.SubscriptionPause `json:"pause"`
	Cancelled        bool                  `json:"cancelled"`
	TrialEndsAt      *string               `json:"trial_ends_at"`
	BillingAnchor    int                   `json:"billing_anchor"`
	FirstSubItem     *SubItemBrief         `json:"first_subscription_item"`
	URLs             struct {
		UpdatePaymentMethod  string  `json:"update_payment_method"`
		CustomerPortal       string  `json:"customer_portal"`
		CustomerPortalUpdate *string `json:"customer_portal_update_subscription"`
	} `json:"urls"`
	RenewsAt  *string `json:"renews_at"`
	EndsAt    *string `json:"ends_at"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
	TestMode  bool    `json:"test_mode"`
}

func (b *Builder) Subscriptions(ss []db.Subscription) []Resource {
	ids := make([]int64, len(ss))
	for i, s := range ss {
		ids[i] = s.ID
	}
	items := map[int64]db.SubscriptionItem{}
	if len(ids) > 0 {
		var its []db.SubscriptionItem
		b.DB.Where("subscription_id IN ?", ids).Order("id").Find(&its)
		for _, it := range its {
			if _, ok := items[it.SubscriptionID]; !ok {
				items[it.SubscriptionID] = it
			}
		}
	}
	out := make([]Resource, len(ss))
	for i, s := range ss {
		a := SubscriptionAttrs{
			StoreID: s.StoreID, CustomerID: s.CustomerID, OrderID: s.OrderID, OrderItemID: s.OrderItemID, ProductID: s.ProductID,
			VariantID: s.VariantID, ProductName: s.ProductName, VariantName: s.VariantName, UserName: s.UserName, UserEmail: s.UserEmail,
			Status: s.Status, StatusFormatted: FormatStatus(s.Status), CardBrand: s.CardBrand, CardLastFour: s.CardLastFour,
			PaymentProcessor: s.PaymentProcessor, Pause: s.Pause.V, Cancelled: s.Cancelled, TrialEndsAt: TSP(s.TrialEndsAt),
			BillingAnchor: s.BillingAnchor, RenewsAt: TSP(s.RenewsAt), EndsAt: TSP(s.EndsAt),
			CreatedAt: TS(s.CreatedAt), UpdatedAt: TS(s.UpdatedAt), TestMode: s.TestMode,
		}
		if it, ok := items[s.ID]; ok {
			a.FirstSubItem = &SubItemBrief{ID: it.ID, SubscriptionID: it.SubscriptionID, PriceID: it.PriceID, Quantity: it.Quantity, CreatedAt: TS(it.CreatedAt), UpdatedAt: TS(it.UpdatedAt)}
		}
		a.URLs.UpdatePaymentMethod = b.URLs.Portal(s.CustomerID, s.ID, b.now(), true)
		a.URLs.CustomerPortal = b.URLs.Portal(s.CustomerID, 0, b.now(), true)
		out[i] = b.res("subscriptions", s.ID, a, "store", "customer", "order", "order-item", "product", "variant", "subscription-items", "subscription-invoices")
	}
	return out
}

type SubscriptionItemAttrs struct {
	SubscriptionID int64  `json:"subscription_id"`
	PriceID        int64  `json:"price_id"`
	Quantity       int    `json:"quantity"`
	IsUsageBased   bool   `json:"is_usage_based"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

func (b *Builder) SubscriptionItems(is []db.SubscriptionItem) []Resource {
	out := make([]Resource, len(is))
	for i, it := range is {
		out[i] = b.res("subscription-items", it.ID, SubscriptionItemAttrs{
			SubscriptionID: it.SubscriptionID, PriceID: it.PriceID, Quantity: it.Quantity, IsUsageBased: it.IsUsageBased,
			CreatedAt: TS(it.CreatedAt), UpdatedAt: TS(it.UpdatedAt),
		}, "subscription", "price", "usage-records")
	}
	return out
}

type SubscriptionInvoiceAttrs struct {
	StoreID                 int64   `json:"store_id"`
	SubscriptionID          int64   `json:"subscription_id"`
	CustomerID              int64   `json:"customer_id"`
	UserName                string  `json:"user_name"`
	UserEmail               string  `json:"user_email"`
	BillingReason           string  `json:"billing_reason"`
	CardBrand               string  `json:"card_brand"`
	CardLastFour            string  `json:"card_last_four"`
	Currency                string  `json:"currency"`
	CurrencyRate            string  `json:"currency_rate"`
	Status                  string  `json:"status"`
	StatusFormatted         string  `json:"status_formatted"`
	Refunded                bool    `json:"refunded"`
	RefundedAt              *string `json:"refunded_at"`
	Subtotal                int64   `json:"subtotal"`
	DiscountTotal           int64   `json:"discount_total"`
	Tax                     int64   `json:"tax"`
	TaxInclusive            bool    `json:"tax_inclusive"`
	Total                   int64   `json:"total"`
	RefundedAmount          int64   `json:"refunded_amount"`
	SubtotalUSD             int64   `json:"subtotal_usd"`
	DiscountTotalUSD        int64   `json:"discount_total_usd"`
	TaxUSD                  int64   `json:"tax_usd"`
	TotalUSD                int64   `json:"total_usd"`
	RefundedAmountUSD       int64   `json:"refunded_amount_usd"`
	SubtotalFormatted       string  `json:"subtotal_formatted"`
	DiscountTotalFormatted  string  `json:"discount_total_formatted"`
	TaxFormatted            string  `json:"tax_formatted"`
	TotalFormatted          string  `json:"total_formatted"`
	RefundedAmountFormatted string  `json:"refunded_amount_formatted"`
	URLs                    struct {
		InvoiceURL *string `json:"invoice_url"`
	} `json:"urls"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	TestMode  bool   `json:"test_mode"`
}

func (b *Builder) SubscriptionInvoices(is []db.SubscriptionInvoice) []Resource {
	out := make([]Resource, len(is))
	for i, v := range is {
		a := SubscriptionInvoiceAttrs{
			StoreID: v.StoreID, SubscriptionID: v.SubscriptionID, CustomerID: v.CustomerID, UserName: v.UserName, UserEmail: v.UserEmail,
			BillingReason: v.BillingReason, CardBrand: v.CardBrand, CardLastFour: v.CardLastFour, Currency: v.Currency, CurrencyRate: v.CurrencyRate,
			Status: v.Status, StatusFormatted: FormatStatus(v.Status), Refunded: v.Refunded, RefundedAt: TSP(v.RefundedAt),
			Subtotal: v.Subtotal, DiscountTotal: v.DiscountTotal, Tax: v.Tax, TaxInclusive: v.TaxInclusive, Total: v.Total, RefundedAmount: v.RefundedAmount,
			SubtotalUSD: v.SubtotalUSD, DiscountTotalUSD: v.DiscountTotalUSD, TaxUSD: v.TaxUSD, TotalUSD: v.TotalUSD, RefundedAmountUSD: v.RefundedAmountUSD,
			SubtotalFormatted: FormatMoney(v.Subtotal, v.Currency), DiscountTotalFormatted: FormatMoney(v.DiscountTotal, v.Currency),
			TaxFormatted: FormatMoney(v.Tax, v.Currency), TotalFormatted: FormatMoney(v.Total, v.Currency),
			RefundedAmountFormatted: FormatMoney(v.RefundedAmount, v.Currency),
			CreatedAt:               TS(v.CreatedAt), UpdatedAt: TS(v.UpdatedAt), TestMode: v.TestMode,
		}
		a.URLs.InvoiceURL = strOrNil(v.InvoiceURL)
		out[i] = b.res("subscription-invoices", v.ID, a, "store", "subscription")
	}
	return out
}

// ---------------------------------------------------------------- licensing

type LicenseKeyAttrs struct {
	StoreID         int64   `json:"store_id"`
	CustomerID      int64   `json:"customer_id"`
	OrderID         int64   `json:"order_id"`
	OrderItemID     int64   `json:"order_item_id"`
	ProductID       int64   `json:"product_id"`
	UserName        string  `json:"user_name"`
	UserEmail       string  `json:"user_email"`
	Key             string  `json:"key"`
	KeyShort        string  `json:"key_short"`
	ActivationLimit *int    `json:"activation_limit"`
	InstancesCount  int     `json:"instances_count"`
	Disabled        int     `json:"disabled"`
	Status          string  `json:"status"`
	StatusFormatted string  `json:"status_formatted"`
	ExpiresAt       *string `json:"expires_at"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

func KeyShort(key string) string {
	if len(key) > 12 {
		return "XXXX-" + key[len(key)-12:]
	}
	return "XXXX-" + key
}

func (b *Builder) LicenseKeys(ks []db.LicenseKey) []Resource {
	now := b.now()
	out := make([]Resource, len(ks))
	for i := range ks {
		k := ks[i]
		st := LicenseStatus(&k, now)
		dis := 0
		if k.Disabled {
			dis = 1
		}
		out[i] = b.res("license-keys", k.ID, LicenseKeyAttrs{
			StoreID: k.StoreID, CustomerID: k.CustomerID, OrderID: k.OrderID, OrderItemID: k.OrderItemID, ProductID: k.ProductID,
			UserName: k.UserName, UserEmail: k.UserEmail, Key: k.Key, KeyShort: KeyShort(k.Key), ActivationLimit: k.ActivationLimit,
			InstancesCount: k.InstancesCount, Disabled: dis, Status: st, StatusFormatted: FormatStatus(st),
			ExpiresAt: TSP(k.ExpiresAt), CreatedAt: TS(k.CreatedAt), UpdatedAt: TS(k.UpdatedAt),
		}, "store", "customer", "order", "order-item", "product", "license-key-instances")
	}
	return out
}

type LicenseKeyInstanceAttrs struct {
	LicenseKeyID int64  `json:"license_key_id"`
	Identifier   string `json:"identifier"`
	Name         string `json:"name"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

func (b *Builder) LicenseKeyInstances(is []db.LicenseKeyInstance) []Resource {
	out := make([]Resource, len(is))
	for i, it := range is {
		out[i] = b.res("license-key-instances", it.ID, LicenseKeyInstanceAttrs{
			LicenseKeyID: it.LicenseKeyID, Identifier: it.Identifier, Name: it.Name, CreatedAt: TS(it.CreatedAt), UpdatedAt: TS(it.UpdatedAt),
		}, "license-key")
	}
	return out
}

// ---------------------------------------------------------------- webhooks

type WebhookAttrs struct {
	StoreID    int64    `json:"store_id"`
	URL        string   `json:"url"`
	Events     []string `json:"events"`
	LastSentAt *string  `json:"last_sent_at"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
	TestMode   bool     `json:"test_mode"`
}

func (b *Builder) Webhooks(ws []db.Webhook) []Resource {
	out := make([]Resource, len(ws))
	for i, w := range ws {
		ev := w.Events.V
		if ev == nil {
			ev = []string{}
		}
		out[i] = b.res("webhooks", w.ID, WebhookAttrs{
			StoreID: w.StoreID, URL: w.URL, Events: ev, LastSentAt: TSP(w.LastSentAt),
			CreatedAt: TS(w.CreatedAt), UpdatedAt: TS(w.UpdatedAt), TestMode: w.TestMode,
		}, "store")
	}
	return out
}
