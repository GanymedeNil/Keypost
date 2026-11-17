package admin

import (
	"net/http"
	"sort"
	"strconv"
	"time"

	"fulfillment/internal/db"
	"fulfillment/internal/resource"
	"fulfillment/internal/settings"
)

type seriesPoint struct {
	Date    string `json:"date"`
	Revenue int64  `json:"revenue"`
	Orders  int64  `json:"orders"`
	Refunds int64  `json:"refunds"`
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request, u *db.User) {
	p := parsePage(r)
	days := 30
	if n, err := strconv.Atoi(r.URL.Query().Get("days")); err == nil && n >= 1 && n <= 365 {
		days = n
	}
	now := db.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(days - 1))
	prevStart := start.AddDate(0, 0, -days)

	paidStatuses := []string{"paid", "partial_refund", "refunded"}
	type orderRow struct {
		CreatedAt         time.Time
		TotalUSD          int64
		RefundedAmountUSD int64
	}
	loadOrders := func(from, to time.Time) []orderRow {
		var rows []orderRow
		q := h.S.DB.Model(&db.Order{}).Select("created_at, total_usd, refunded_amount_usd").
			Where("test_mode = ? AND status IN ? AND created_at >= ? AND created_at < ?", p.TestMode, paidStatuses, from, to)
		if p.StoreID > 0 {
			q = q.Where("store_id = ?", p.StoreID)
		}
		q.Limit(200000).Scan(&rows)
		return rows
	}
	loadRenewals := func(from, to time.Time) []orderRow {
		var rows []orderRow
		q := h.S.DB.Model(&db.SubscriptionInvoice{}).Select("created_at, total_usd, refunded_amount_usd").
			Where("test_mode = ? AND billing_reason <> ? AND status IN ? AND created_at >= ? AND created_at < ?", p.TestMode, "initial", paidStatuses, from, to)
		if p.StoreID > 0 {
			q = q.Where("store_id = ?", p.StoreID)
		}
		q.Limit(200000).Scan(&rows)
		return rows
	}
	end := now.Add(time.Second)
	orders, renewals := loadOrders(start, end), loadRenewals(start, end)
	prevOrders, prevRenewals := loadOrders(prevStart, start), loadRenewals(prevStart, start)

	byDay := map[string]*seriesPoint{}
	for d := 0; d < days; d++ {
		key := start.AddDate(0, 0, d).Format("2006-01-02")
		byDay[key] = &seriesPoint{Date: key}
	}
	var revenue, refunded int64
	for _, o := range orders {
		sp := byDay[o.CreatedAt.UTC().Format("2006-01-02")]
		if sp == nil {
			continue
		}
		sp.Revenue += o.TotalUSD - o.RefundedAmountUSD
		sp.Refunds += o.RefundedAmountUSD
		sp.Orders++
		revenue += o.TotalUSD - o.RefundedAmountUSD
		refunded += o.RefundedAmountUSD
	}
	for _, o := range renewals {
		if sp := byDay[o.CreatedAt.UTC().Format("2006-01-02")]; sp != nil {
			sp.Revenue += o.TotalUSD - o.RefundedAmountUSD
			sp.Refunds += o.RefundedAmountUSD
		}
		revenue += o.TotalUSD - o.RefundedAmountUSD
		refunded += o.RefundedAmountUSD
	}
	var prevRevenue int64
	for _, o := range prevOrders {
		prevRevenue += o.TotalUSD - o.RefundedAmountUSD
	}
	for _, o := range prevRenewals {
		prevRevenue += o.TotalUSD - o.RefundedAmountUSD
	}
	series := make([]seriesPoint, 0, len(byDay))
	for _, sp := range byDay {
		series = append(series, *sp)
	}
	sort.Slice(series, func(i, j int) bool { return series[i].Date < series[j].Date })

	scoped := func(model any) int64 {
		var n int64
		q := h.S.DB.Model(model).Where("test_mode = ?", p.TestMode)
		if p.StoreID > 0 {
			q = q.Where("store_id = ?", p.StoreID)
		}
		q.Count(&n)
		return n
	}
	var activeSubs int64
	{
		q := h.S.DB.Model(&db.Subscription{}).Where("test_mode = ? AND status IN ?", p.TestMode, []string{"active", "on_trial", "past_due"})
		if p.StoreID > 0 {
			q = q.Where("store_id = ?", p.StoreID)
		}
		q.Count(&activeSubs)
	}
	var mrr int64
	{
		var subs []db.Subscription
		q := h.S.DB.Where("test_mode = ? AND status IN ?", p.TestMode, []string{"active", "on_trial", "past_due"})
		if p.StoreID > 0 {
			q = q.Where("store_id = ?", p.StoreID)
		}
		q.Limit(100000).Find(&subs)
		pids := make([]int64, 0, len(subs))
		for _, s := range subs {
			if s.PriceID != nil {
				pids = append(pids, *s.PriceID)
			}
		}
		prices := map[int64]db.Price{}
		if len(pids) > 0 {
			var ps []db.Price
			h.S.DB.Where("id IN ?", pids).Find(&ps)
			for _, pr := range ps {
				prices[pr.ID] = pr
			}
		}
		for _, s := range subs {
			if s.PriceID == nil {
				continue
			}
			pr, ok := prices[*s.PriceID]
			if !ok || pr.RenewalIntervalUnit == nil {
				continue
			}
			q := 1
			if pr.RenewalIntervalQuantity != nil {
				q = *pr.RenewalIntervalQuantity
			}
			qty := s.Quantity
			if qty < 1 {
				qty = 1
			}
			mrr += h.S.ToUSD(resource.MonthlyAmount(pr.UnitPrice*int64(qty), *pr.RenewalIntervalUnit, q), pr.Currency)
		}
	}
	var activeKeys int64
	{
		q := h.S.DB.Model(&db.LicenseKey{}).Where("test_mode = ? AND disabled = ? AND (expires_at IS NULL OR expires_at > ?)", p.TestMode, false, now)
		if p.StoreID > 0 {
			q = q.Where("store_id = ?", p.StoreID)
		}
		q.Count(&activeKeys)
	}
	var downloads int64
	h.S.DB.Model(&db.File{}).Where("test_mode = ?", p.TestMode).Select("COALESCE(SUM(downloads_count),0)").Scan(&downloads)

	var recent []db.Order
	{
		q := h.S.DB.Where("test_mode = ?", p.TestMode)
		if p.StoreID > 0 {
			q = q.Where("store_id = ?", p.StoreID)
		}
		q.Order("id DESC").Limit(8).Find(&recent)
	}

	// recent events feed
	type ev struct {
		kind, title, status string
		at                  time.Time
	}
	var feed []ev
	var ses []db.StripeEvent
	h.S.DB.Order("received_at DESC").Limit(6).Find(&ses)
	for _, e := range ses {
		feed = append(feed, ev{"stripe_event", e.Type, e.Status, e.ReceivedAt})
	}
	var wds []db.WebhookDelivery
	h.S.DB.Order("id DESC").Limit(6).Find(&wds)
	for _, d := range wds {
		feed = append(feed, ev{"webhook_delivery", d.EventName, d.Status, d.CreatedAt})
	}
	var jobs []db.ImportJob
	h.S.DB.Order("id DESC").Limit(3).Find(&jobs)
	for _, j := range jobs {
		feed = append(feed, ev{"import", j.Kind, j.Status, j.CreatedAt})
	}
	sort.Slice(feed, func(i, j int) bool { return feed[i].at.After(feed[j].at) })
	if len(feed) > 10 {
		feed = feed[:10]
	}
	events := make([]map[string]any, len(feed))
	for i, e := range feed {
		events[i] = map[string]any{"kind": e.kind, "title": e.title, "status": e.status, "at": ts(e.at)}
	}

	var failedEv, failedDel, pendingDel int64
	h.S.DB.Model(&db.StripeEvent{}).Where("status = ?", "failed").Count(&failedEv)
	h.S.DB.Model(&db.WebhookDelivery{}).Where("status = ? AND created_at > ?", "failed", now.AddDate(0, 0, -7)).Count(&failedDel)
	h.S.DB.Model(&db.WebhookDelivery{}).Where("status = ?", "pending").Count(&pendingDel)

	currency := "USD"
	var st db.Store
	if p.StoreID > 0 {
		h.S.DB.First(&st, p.StoreID)
	} else {
		h.S.DB.Order("id").First(&st)
	}
	_ = currency
	h.ok(w, map[string]any{
		"currency": "USD",
		"totals": map[string]any{
			"revenue": revenue, "orders": int64(len(orders)), "refunded": refunded, "customers": scoped(&db.Customer{}),
			"active_subscriptions": activeSubs, "mrr": mrr, "license_keys": scoped(&db.LicenseKey{}), "active_license_keys": activeKeys, "downloads": downloads,
		},
		"previous":      map[string]any{"revenue": prevRevenue, "orders": int64(len(prevOrders))},
		"series":        series,
		"recent_orders": h.orderDTOs(recent),
		"recent_events": events,
		"health": map[string]any{
			"stripe_configured": h.S.Settings.Get(settings.StripeSecretKey) != "", "stripe_webhook_secret_set": h.S.Settings.Get(settings.StripeWebhookSecret) != "",
			"smtp_configured": h.S.Mail.Configured(), "failed_stripe_events": failedEv, "failed_webhook_deliveries": failedDel,
			"pending_webhook_deliveries": pendingDel, "storage_driver": h.S.Storage.Driver(),
		},
	})
}
