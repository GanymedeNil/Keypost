package admin

import (
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"fulfillment/internal/db"
	"fulfillment/internal/resource"
	"fulfillment/internal/storage"
	"fulfillment/internal/storefront"
)

// ---------------------------------------------------------------- DTOs

func (h *Handler) storeDTO(s db.Store, testMode bool) map[string]any {
	st := h.S.Res.StoreStats(s.ID, testMode)
	return map[string]any{
		"id": s.ID, "name": s.Name, "slug": s.Slug, "domain": s.Domain, "url": s.URL, "avatar_url": s.AvatarURL, "plan": s.Plan,
		"country": s.Country, "currency": s.Currency, "support_email": s.SupportEmail, "email_from_name": s.EmailFromName,
		"receipt_subject": s.ReceiptSubject, "receipt_intro": s.ReceiptIntro, "stripe_account_id": s.StripeAccountID,
		"total_sales": st.Sales, "total_revenue": st.Revenue, "thirty_day_sales": st.Sales30, "thirty_day_revenue": st.Revenue30,
		"created_at": ts(s.CreatedAt), "updated_at": ts(s.UpdatedAt),
	}
}

func priceDTO(p db.Price) map[string]any {
	tiers := p.Tiers.V
	if tiers == nil {
		tiers = []db.PriceTier{}
	}
	return map[string]any{
		"id": p.ID, "variant_id": p.VariantID, "category": p.Category, "scheme": p.Scheme, "unit_price": p.UnitPrice,
		"unit_price_decimal": p.UnitPriceDecimal, "setup_fee_enabled": p.SetupFeeEnabled, "setup_fee": p.SetupFee, "package_size": p.PackageSize,
		"tiers": tiers, "renewal_interval_unit": p.RenewalIntervalUnit, "renewal_interval_quantity": p.RenewalIntervalQuantity,
		"trial_interval_unit": p.TrialIntervalUnit, "trial_interval_quantity": p.TrialIntervalQuantity, "min_price": p.MinPrice,
		"suggested_price": p.SuggestedPrice, "tax_code": p.TaxCode, "currency": p.Currency, "stripe_price_id": p.StripePriceID,
		"created_at": ts(p.CreatedAt), "updated_at": ts(p.UpdatedAt),
	}
}

func (h *Handler) variantDTOs(vs []db.Variant, withFiles bool) []map[string]any {
	ids := make([]int64, len(vs))
	pids := make([]int64, len(vs))
	for i, v := range vs {
		ids[i], pids[i] = v.ID, v.ProductID
	}
	prices := map[int64][]db.Price{}
	filesCount := map[int64]int{}
	filesBy := map[int64][]db.File{}
	prods := map[int64]db.Product{}
	if len(ids) > 0 {
		var ps []db.Price
		h.S.DB.Where("variant_id IN ?", ids).Order("id").Find(&ps)
		for _, p := range ps {
			prices[p.VariantID] = append(prices[p.VariantID], p)
		}
		var fs []db.File
		h.S.DB.Where("variant_id IN ?", ids).Order("sort, id").Find(&fs)
		for _, f := range fs {
			filesCount[f.VariantID]++
			filesBy[f.VariantID] = append(filesBy[f.VariantID], f)
		}
		var prs []db.Product
		h.S.DB.Where("id IN ?", pids).Find(&prs)
		for _, p := range prs {
			prods[p.ID] = p
		}
	}
	out := make([]map[string]any, len(vs))
	for i, v := range vs {
		pr := []map[string]any{}
		for _, p := range prices[v.ID] {
			pr = append(pr, priceDTO(p))
		}
		links := v.Links.V
		if links == nil {
			links = []db.VariantLink{}
		}
		m := map[string]any{
			"id": v.ID, "product_id": v.ProductID, "product_name": prods[v.ProductID].Name, "name": v.Name, "slug": v.Slug,
			"description": v.Description, "links": links, "sort": v.Sort, "status": v.Status, "has_license_keys": v.HasLicenseKeys,
			"license_activation_limit": v.LicenseActivationLimit, "is_license_limit_unlimited": v.IsLicenseLimitUnlimited,
			"license_length_value": v.LicenseLengthValue, "license_length_unit": v.LicenseLengthUnit,
			"is_license_length_unlimited": v.IsLicenseLengthUnlimited, "stripe_product_id": v.StripeProductID,
			"buy_now_url": h.S.URLs.Buy(v.Slug), "prices": pr, "files_count": filesCount[v.ID], "test_mode": v.TestMode,
			"created_at": ts(v.CreatedAt), "updated_at": ts(v.UpdatedAt),
		}
		if withFiles {
			fl := []map[string]any{}
			for _, f := range filesBy[v.ID] {
				fl = append(fl, fileDTO(f, v.Name, prods[v.ProductID].Name))
			}
			m["files"] = fl
		}
		out[i] = m
	}
	return out
}

func fileDTO(f db.File, variantName, productName string) map[string]any {
	return map[string]any{
		"id": f.ID, "variant_id": f.VariantID, "variant_name": variantName, "product_name": productName, "identifier": f.Identifier,
		"name": f.Name, "extension": f.Extension, "size": f.Size, "size_formatted": resource.HumanSize(f.Size), "version": f.Version,
		"sort": f.Sort, "status": f.Status, "storage_driver": f.StorageDriver, "content_type": f.ContentType, "sha256": f.SHA256,
		"downloads_count": f.DownloadsCount, "test_mode": f.TestMode, "created_at": ts(f.CreatedAt), "updated_at": ts(f.UpdatedAt),
	}
}

func (h *Handler) fileDTOs(fs []db.File) []map[string]any {
	vids := make([]int64, len(fs))
	for i, f := range fs {
		vids[i] = f.VariantID
	}
	vn, pn := h.variantProductNames(vids)
	out := make([]map[string]any, len(fs))
	for i, f := range fs {
		out[i] = fileDTO(f, vn[f.VariantID], pn[f.VariantID])
	}
	return out
}

// variantProductNames maps variant id → variant name and product name.
func (h *Handler) variantProductNames(variantIDs []int64) (vn, pn map[int64]string) {
	vn, pn = map[int64]string{}, map[int64]string{}
	if len(variantIDs) == 0 {
		return
	}
	var vs []db.Variant
	h.S.DB.Where("id IN ?", variantIDs).Find(&vs)
	var pids []int64
	for _, v := range vs {
		pids = append(pids, v.ProductID)
	}
	prods := map[int64]string{}
	var ps []db.Product
	h.S.DB.Where("id IN ?", pids).Find(&ps)
	for _, p := range ps {
		prods[p.ID] = p.Name
	}
	for _, v := range vs {
		vn[v.ID], pn[v.ID] = v.Name, prods[v.ProductID]
	}
	return
}

func (h *Handler) productDTOs(ps []db.Product) []map[string]any {
	ids := make([]int64, len(ps))
	for i, p := range ps {
		ids[i] = p.ID
	}
	var vs []db.Variant
	if len(ids) > 0 {
		h.S.DB.Where("product_id IN ?", ids).Order("sort, id").Find(&vs)
	}
	vids := make([]int64, len(vs))
	for i, v := range vs {
		vids[i] = v.ID
	}
	first := map[int64]db.Price{}
	if len(vids) > 0 {
		var prs []db.Price
		h.S.DB.Where("variant_id IN ?", vids).Order("id").Find(&prs)
		for _, p := range prs {
			if _, ok := first[p.VariantID]; !ok {
				first[p.VariantID] = p
			}
		}
	}
	by := map[int64][]db.Variant{}
	for _, v := range vs {
		by[v.ProductID] = append(by[v.ProductID], v)
	}
	out := make([]map[string]any, len(ps))
	for i, p := range ps {
		var lo, hi *int64
		buy := ""
		for _, v := range by[p.ID] {
			if buy == "" && v.Status != "draft" {
				buy = h.S.URLs.Buy(v.Slug)
			}
			if pr, ok := first[v.ID]; ok {
				a := pr.UnitPrice
				if lo == nil || a < *lo {
					lo = &a
				}
				if hi == nil || a > *hi {
					hi = &a
				}
			}
		}
		out[i] = map[string]any{
			"id": p.ID, "store_id": p.StoreID, "name": p.Name, "slug": p.Slug, "description": p.Description, "status": p.Status,
			"thumb_url": p.ThumbURL, "large_thumb_url": p.LargeThumbURL, "stripe_product_id": p.StripeProductID, "test_mode": p.TestMode,
			"buy_now_url": buy, "variants_count": len(by[p.ID]), "from_price": lo, "to_price": hi,
			"created_at": ts(p.CreatedAt), "updated_at": ts(p.UpdatedAt),
		}
	}
	return out
}

// ---------------------------------------------------------------- stores

func (h *Handler) listStores(w http.ResponseWriter, r *http.Request, u *db.User) {
	p := parsePage(r)
	var rows []db.Store
	meta, err := paginate(applySort(h.S.DB.Model(&db.Store{}), p.Sort, map[string]string{"name": "name", "created_at": "created_at"}, "id"), p, &rows)
	if err != nil {
		h.failErr(w, err)
		return
	}
	out := make([]map[string]any, len(rows))
	for i, s := range rows {
		out[i] = h.storeDTO(s, p.TestMode)
	}
	h.respondList(w, out, meta)
}

func (h *Handler) getStore(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	var s db.Store
	if err := h.S.DB.First(&s, id).Error; err != nil {
		h.failErr(w, err)
		return
	}
	h.ok(w, h.storeDTO(s, parsePage(r).TestMode))
}

func applyStoreFields(s *db.Store, b body) {
	set := func(k string, dst *string) {
		if v, ok := b.str(k); ok {
			*dst = v
		}
	}
	set("name", &s.Name)
	set("domain", &s.Domain)
	set("url", &s.URL)
	set("avatar_url", &s.AvatarURL)
	set("support_email", &s.SupportEmail)
	set("email_from_name", &s.EmailFromName)
	set("receipt_subject", &s.ReceiptSubject)
	set("stripe_account_id", &s.StripeAccountID)
	if v, ok := b["receipt_intro"].(string); ok {
		s.ReceiptIntro = v
	}
	if v, ok := b.str("country"); ok {
		s.Country = strings.ToUpper(v)
	}
	if v, ok := b.str("currency"); ok {
		s.Currency = strings.ToUpper(v)
	}
	if v, ok := b.str("slug"); ok {
		s.Slug = slugify(v)
	}
}

func (h *Handler) createStore(w http.ResponseWriter, r *http.Request, u *db.User) {
	b, ok := h.readBody(w, r)
	if !ok {
		return
	}
	s := db.Store{Plan: "self-hosted", Country: "US", Currency: "USD"}
	applyStoreFields(&s, b)
	if strings.TrimSpace(s.Name) == "" {
		h.invalid(w, "name", "The name field is required.")
		return
	}
	if s.Slug == "" {
		s.Slug = slugify(s.Name)
	}
	s.CreatedAt, s.UpdatedAt = db.Now(), db.Now()
	if err := h.S.DB.Create(&s).Error; err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "duplicate") {
			h.invalid(w, "slug", "That slug is already taken.")
			return
		}
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "store.create", "store", s.ID, nil)
	h.created(w, h.storeDTO(s, false))
}

func (h *Handler) updateStore(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	var s db.Store
	if err := h.S.DB.First(&s, id).Error; err != nil {
		h.failErr(w, err)
		return
	}
	b, ok := h.readBody(w, r)
	if !ok {
		return
	}
	applyStoreFields(&s, b)
	s.UpdatedAt = db.Now()
	if err := h.S.DB.Save(&s).Error; err != nil {
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "store.update", "store", s.ID, nil)
	h.ok(w, h.storeDTO(s, false))
}

// ---------------------------------------------------------------- products

func (h *Handler) listProducts(w http.ResponseWriter, r *http.Request, u *db.User) {
	p := parsePage(r)
	q := h.S.DB.Model(&db.Product{}).Where("test_mode = ?", p.TestMode)
	if p.StoreID > 0 {
		q = q.Where("store_id = ?", p.StoreID)
	}
	if v := r.URL.Query().Get("status"); v != "" {
		q = q.Where("status = ?", v)
	}
	if p.Q != "" {
		q = q.Where("LOWER(name) LIKE ? ESCAPE '\\'", like(p.Q))
	}
	q = applySort(q, p.Sort, map[string]string{"name": "name", "created_at": "created_at", "id": "id", "status": "status"}, "id DESC")
	var rows []db.Product
	meta, err := paginate(q, p, &rows)
	if err != nil {
		h.failErr(w, err)
		return
	}
	h.respondList(w, h.productDTOs(rows), meta)
}

func (h *Handler) getProduct(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	var p db.Product
	if err := h.S.DB.First(&p, id).Error; err != nil {
		h.failErr(w, err)
		return
	}
	dto := h.productDTOs([]db.Product{p})[0]
	var vs []db.Variant
	h.S.DB.Where("product_id = ?", p.ID).Order("sort, id").Find(&vs)
	dto["variants"] = h.variantDTOs(vs, true)
	h.ok(w, dto)
}

func applyProductFields(p *db.Product, b body) *string {
	set := func(k string, dst *string) {
		if v, ok := b.str(k); ok {
			*dst = v
		}
	}
	set("name", &p.Name)
	set("thumb_url", &p.ThumbURL)
	set("large_thumb_url", &p.LargeThumbURL)
	set("stripe_product_id", &p.StripeProductID)
	if v, ok := b["description"].(string); ok {
		p.Description = v
	}
	if v, ok := b.str("slug"); ok {
		p.Slug = slugify(v)
	}
	if v, ok := b.str("status"); ok {
		if v != "draft" && v != "published" {
			e := "The status must be draft or published."
			return &e
		}
		p.Status = v
	}
	return nil
}

func (h *Handler) createProduct(w http.ResponseWriter, r *http.Request, u *db.User) {
	b, ok := h.readBody(w, r)
	if !ok {
		return
	}
	p := db.Product{Status: "draft"}
	if e := applyProductFields(&p, b); e != nil {
		h.invalid(w, "status", *e)
		return
	}
	sid, ok := b.int("store_id")
	if !ok {
		var st db.Store
		if h.S.DB.Order("id").First(&st).Error == nil {
			sid = st.ID
		}
	}
	var st db.Store
	if err := h.S.DB.First(&st, sid).Error; err != nil {
		h.invalid(w, "store_id", "The selected store is invalid.")
		return
	}
	p.StoreID = st.ID
	if p.Name == "" {
		h.invalid(w, "name", "The name field is required.")
		return
	}
	if p.Slug == "" {
		p.Slug = slugify(p.Name)
	}
	if v, ok := b.boolean("test_mode"); ok {
		p.TestMode = v
	}
	p.CreatedAt, p.UpdatedAt = db.Now(), db.Now()
	if err := h.S.DB.Create(&p).Error; err != nil {
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "product.create", "product", p.ID, nil)
	h.created(w, h.productDTOs([]db.Product{p})[0])
}

func (h *Handler) updateProduct(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	var p db.Product
	if err := h.S.DB.First(&p, id).Error; err != nil {
		h.failErr(w, err)
		return
	}
	b, ok := h.readBody(w, r)
	if !ok {
		return
	}
	if e := applyProductFields(&p, b); e != nil {
		h.invalid(w, "status", *e)
		return
	}
	p.UpdatedAt = db.Now()
	if err := h.S.DB.Save(&p).Error; err != nil {
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "product.update", "product", p.ID, nil)
	h.ok(w, h.productDTOs([]db.Product{p})[0])
}

func (h *Handler) deleteProduct(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	var p db.Product
	if err := h.S.DB.First(&p, id).Error; err != nil {
		h.failErr(w, err)
		return
	}
	var refs int64
	h.S.DB.Model(&db.OrderItem{}).Where("product_id = ?", id).Count(&refs)
	if refs > 0 {
		h.fail(w, http.StatusConflict, "conflict", fmt.Sprintf("This product has %d order item(s) and cannot be deleted. Set it to draft instead.", refs), nil)
		return
	}
	var vs []db.Variant
	h.S.DB.Where("product_id = ?", id).Find(&vs)
	for _, v := range vs {
		h.removeVariant(r, v)
	}
	h.S.DB.Delete(&db.Product{}, id)
	h.audit(r, u, "product.delete", "product", id, map[string]any{"name": p.Name})
	noContent(w)
}

// removeVariant deletes a variant with its prices and files (including stored objects).
func (h *Handler) removeVariant(r *http.Request, v db.Variant) {
	var fs []db.File
	h.S.DB.Where("variant_id = ?", v.ID).Find(&fs)
	for _, f := range fs {
		_ = h.S.Storage.Delete(r.Context(), f.StorageKey)
	}
	h.S.DB.Where("variant_id = ?", v.ID).Delete(&db.File{})
	h.S.DB.Where("variant_id = ?", v.ID).Delete(&db.Price{})
	h.S.DB.Delete(&db.Variant{}, v.ID)
}

// ---------------------------------------------------------------- variants

func (h *Handler) listVariants(w http.ResponseWriter, r *http.Request, u *db.User) {
	p := parsePage(r)
	q := h.S.DB.Model(&db.Variant{}).Where("test_mode = ?", p.TestMode)
	if pid := qInt(r, "product_id"); pid > 0 {
		q = q.Where("product_id = ?", pid)
	}
	if p.Q != "" {
		q = q.Where("LOWER(name) LIKE ? ESCAPE '\\'", like(p.Q))
	}
	q = applySort(q, p.Sort, map[string]string{"name": "name", "sort": "sort", "created_at": "created_at"}, "sort, id")
	var rows []db.Variant
	meta, err := paginate(q, p, &rows)
	if err != nil {
		h.failErr(w, err)
		return
	}
	h.respondList(w, h.variantDTOs(rows, false), meta)
}

func (h *Handler) getVariant(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	var v db.Variant
	if err := h.S.DB.First(&v, id).Error; err != nil {
		h.failErr(w, err)
		return
	}
	h.ok(w, h.variantDTOs([]db.Variant{v}, true)[0])
}

func applyVariantFields(v *db.Variant, b body) (field, msg string) {
	if s, ok := b.str("name"); ok {
		v.Name = s
	}
	if s, ok := b["description"].(string); ok {
		v.Description = s
	}
	if n, ok := b.int("sort"); ok {
		v.Sort = int(n)
	}
	if s, ok := b.str("status"); ok {
		if s != "pending" && s != "draft" && s != "published" {
			return "status", "The status must be pending, draft or published."
		}
		v.Status = s
	}
	if x, ok := b.boolean("has_license_keys"); ok {
		v.HasLicenseKeys = x
	}
	if n, ok := b.int("license_activation_limit"); ok {
		if n < 0 {
			return "license_activation_limit", "Must be zero or greater."
		}
		v.LicenseActivationLimit = int(n)
	}
	if x, ok := b.boolean("is_license_limit_unlimited"); ok {
		v.IsLicenseLimitUnlimited = x
	}
	if n, ok := b.int("license_length_value"); ok {
		if n < 0 {
			return "license_length_value", "Must be zero or greater."
		}
		v.LicenseLengthValue = int(n)
	}
	if s, ok := b.str("license_length_unit"); ok {
		if s != "days" && s != "months" && s != "years" {
			return "license_length_unit", "The unit must be days, months or years."
		}
		v.LicenseLengthUnit = s
	}
	if x, ok := b.boolean("is_license_length_unlimited"); ok {
		v.IsLicenseLengthUnlimited = x
	}
	if s, ok := b.str("stripe_product_id"); ok {
		v.StripeProductID = s
	}
	if raw, ok := b["links"].([]any); ok {
		links := make([]db.VariantLink, 0, len(raw))
		for _, e := range raw {
			if m, ok := e.(map[string]any); ok {
				t, _ := m["title"].(string)
				u, _ := m["url"].(string)
				if strings.TrimSpace(u) == "" {
					continue
				}
				links = append(links, db.VariantLink{Title: strings.TrimSpace(t), URL: strings.TrimSpace(u)})
			}
		}
		v.Links = db.J(links)
	}
	return "", ""
}

func (h *Handler) createVariant(w http.ResponseWriter, r *http.Request, u *db.User) {
	b, ok := h.readBody(w, r)
	if !ok {
		return
	}
	pid, _ := b.int("product_id")
	var prod db.Product
	if err := h.S.DB.First(&prod, pid).Error; err != nil {
		h.invalid(w, "product_id", "The selected product is invalid.")
		return
	}
	v := db.Variant{ProductID: prod.ID, Slug: uuid.NewString(), Status: "published", LicenseActivationLimit: 1, LicenseLengthValue: 1, LicenseLengthUnit: "years", IsLicenseLengthUnlimited: true, TestMode: prod.TestMode, Links: db.J([]db.VariantLink{})}
	if f, m := applyVariantFields(&v, b); f != "" {
		h.invalid(w, f, m)
		return
	}
	if v.Name == "" {
		h.invalid(w, "name", "The name field is required.")
		return
	}
	v.CreatedAt, v.UpdatedAt = db.Now(), db.Now()
	err := h.S.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&v).Error; err != nil {
			return err
		}
		if pb, ok := b.obj("price"); ok {
			pr := db.Price{VariantID: v.ID, Category: "one_time", Scheme: "standard", PackageSize: 1, TaxCode: "eservice", Currency: "USD", Tiers: db.J([]db.PriceTier{})}
			if st := (db.Store{}); tx.First(&st, prod.StoreID).Error == nil && st.Currency != "" {
				pr.Currency = st.Currency
			}
			if f, m := applyPriceFields(&pr, pb); f != "" {
				return fieldErr{f, m}
			}
			pr.CreatedAt, pr.UpdatedAt = db.Now(), db.Now()
			return tx.Create(&pr).Error
		}
		return nil
	})
	if fe, ok := err.(fieldErr); ok {
		h.invalid(w, "price."+fe.field, fe.msg)
		return
	}
	if err != nil {
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "variant.create", "variant", v.ID, nil)
	h.created(w, h.variantDTOs([]db.Variant{v}, true)[0])
}

type fieldErr struct{ field, msg string }

func (e fieldErr) Error() string { return e.field + ": " + e.msg }

func (h *Handler) updateVariant(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	var v db.Variant
	if err := h.S.DB.First(&v, id).Error; err != nil {
		h.failErr(w, err)
		return
	}
	b, ok := h.readBody(w, r)
	if !ok {
		return
	}
	if f, m := applyVariantFields(&v, b); f != "" {
		h.invalid(w, f, m)
		return
	}
	v.UpdatedAt = db.Now()
	if err := h.S.DB.Save(&v).Error; err != nil {
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "variant.update", "variant", v.ID, nil)
	h.ok(w, h.variantDTOs([]db.Variant{v}, true)[0])
}

func (h *Handler) deleteVariant(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	var v db.Variant
	if err := h.S.DB.First(&v, id).Error; err != nil {
		h.failErr(w, err)
		return
	}
	var refs int64
	h.S.DB.Model(&db.OrderItem{}).Where("variant_id = ?", id).Count(&refs)
	if refs > 0 {
		h.fail(w, http.StatusConflict, "conflict", fmt.Sprintf("This variant has %d order item(s) and cannot be deleted. Set it to draft instead.", refs), nil)
		return
	}
	h.removeVariant(r, v)
	h.audit(r, u, "variant.delete", "variant", id, map[string]any{"name": v.Name})
	noContent(w)
}

// ---------------------------------------------------------------- prices

func applyPriceFields(p *db.Price, b body) (field, msg string) {
	if s, ok := b.str("category"); ok {
		switch s {
		case "one_time", "subscription", "lead_magnet", "pwyw":
			p.Category = s
		default:
			return "category", "The category must be one_time, subscription, lead_magnet or pwyw."
		}
	}
	if s, ok := b.str("scheme"); ok {
		switch s {
		case "standard", "package", "graduated", "volume":
			p.Scheme = s
		default:
			return "scheme", "Invalid pricing scheme."
		}
	}
	if n, ok := b.int("unit_price"); ok {
		if n < 0 {
			return "unit_price", "Must be zero or greater."
		}
		p.UnitPrice = n
	}
	optStr := func(k string, dst **string, allowed ...string) string {
		if b.isNull(k) {
			*dst = nil
			return ""
		}
		if s, ok := b.str(k); ok {
			if len(allowed) > 0 {
				found := false
				for _, a := range allowed {
					found = found || a == s
				}
				if !found {
					return "Invalid value for " + k
				}
			}
			*dst = &s
		}
		return ""
	}
	optInt := func(k string, dst **int64) {
		if b.isNull(k) {
			*dst = nil
		} else if n, ok := b.int(k); ok {
			*dst = &n
		}
	}
	optInt32 := func(k string, dst **int) {
		if b.isNull(k) {
			*dst = nil
		} else if n, ok := b.int(k); ok {
			v := int(n)
			*dst = &v
		}
	}
	if m := optStr("renewal_interval_unit", &p.RenewalIntervalUnit, "day", "week", "month", "year"); m != "" {
		return "renewal_interval_unit", m
	}
	if m := optStr("trial_interval_unit", &p.TrialIntervalUnit, "day", "week", "month", "year"); m != "" {
		return "trial_interval_unit", m
	}
	optStr("unit_price_decimal", &p.UnitPriceDecimal)
	optStr("usage_aggregation", &p.UsageAggregation)
	optInt32("renewal_interval_quantity", &p.RenewalIntervalQuantity)
	optInt32("trial_interval_quantity", &p.TrialIntervalQuantity)
	optInt("setup_fee", &p.SetupFee)
	optInt("min_price", &p.MinPrice)
	optInt("suggested_price", &p.SuggestedPrice)
	if x, ok := b.boolean("setup_fee_enabled"); ok {
		p.SetupFeeEnabled = x
	}
	if n, ok := b.int("package_size"); ok && n > 0 {
		p.PackageSize = int(n)
	}
	if s, ok := b.str("tax_code"); ok {
		p.TaxCode = s
	}
	if s, ok := b.str("currency"); ok {
		p.Currency = strings.ToUpper(s)
	}
	if s, ok := b.str("stripe_price_id"); ok {
		p.StripePriceID = s
	}
	if raw, ok := b["tiers"].([]any); ok {
		tiers := make([]db.PriceTier, 0, len(raw))
		for _, e := range raw {
			m, _ := e.(map[string]any)
			t := db.PriceTier{LastUnit: m["last_unit"]}
			if n, ok := m["unit_price"].(float64); ok {
				v := int64(n)
				t.UnitPrice = &v
			}
			if s, ok := m["unit_price_decimal"].(string); ok {
				t.UnitPriceDecimal = &s
			}
			if n, ok := m["fixed_fee"].(float64); ok {
				v := int64(n)
				t.FixedFee = &v
			}
			tiers = append(tiers, t)
		}
		p.Tiers = db.J(tiers)
	}
	if p.Category == "subscription" && (p.RenewalIntervalUnit == nil) {
		return "renewal_interval_unit", "Subscriptions need a renewal interval."
	}
	return "", ""
}

func (h *Handler) listPrices(w http.ResponseWriter, r *http.Request, u *db.User) {
	p := parsePage(r)
	q := h.S.DB.Model(&db.Price{})
	if vid := qInt(r, "variant_id"); vid > 0 {
		q = q.Where("variant_id = ?", vid)
	}
	q = applySort(q, p.Sort, map[string]string{"id": "id", "created_at": "created_at"}, "id")
	var rows []db.Price
	meta, err := paginate(q, p, &rows)
	if err != nil {
		h.failErr(w, err)
		return
	}
	out := make([]map[string]any, len(rows))
	for i, x := range rows {
		out[i] = priceDTO(x)
	}
	h.respondList(w, out, meta)
}

func (h *Handler) getPrice(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	var p db.Price
	if err := h.S.DB.First(&p, id).Error; err != nil {
		h.failErr(w, err)
		return
	}
	h.ok(w, priceDTO(p))
}

func (h *Handler) createPrice(w http.ResponseWriter, r *http.Request, u *db.User) {
	b, ok := h.readBody(w, r)
	if !ok {
		return
	}
	vid, _ := b.int("variant_id")
	var v db.Variant
	if err := h.S.DB.First(&v, vid).Error; err != nil {
		h.invalid(w, "variant_id", "The selected variant is invalid.")
		return
	}
	pr := db.Price{VariantID: v.ID, Category: "one_time", Scheme: "standard", PackageSize: 1, TaxCode: "eservice", Currency: "USD", Tiers: db.J([]db.PriceTier{})}
	var prod db.Product
	var st db.Store
	if h.S.DB.First(&prod, v.ProductID).Error == nil && h.S.DB.First(&st, prod.StoreID).Error == nil && st.Currency != "" {
		pr.Currency = st.Currency
	}
	if f, m := applyPriceFields(&pr, b); f != "" {
		h.invalid(w, f, m)
		return
	}
	pr.CreatedAt, pr.UpdatedAt = db.Now(), db.Now()
	if err := h.S.DB.Create(&pr).Error; err != nil {
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "price.create", "price", pr.ID, nil)
	h.created(w, priceDTO(pr))
}

func (h *Handler) updatePrice(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	var pr db.Price
	if err := h.S.DB.First(&pr, id).Error; err != nil {
		h.failErr(w, err)
		return
	}
	b, ok := h.readBody(w, r)
	if !ok {
		return
	}
	if f, m := applyPriceFields(&pr, b); f != "" {
		h.invalid(w, f, m)
		return
	}
	pr.UpdatedAt = db.Now()
	if err := h.S.DB.Save(&pr).Error; err != nil {
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "price.update", "price", pr.ID, nil)
	h.ok(w, priceDTO(pr))
}

func (h *Handler) deletePrice(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	var refs int64
	h.S.DB.Model(&db.OrderItem{}).Where("price_id = ?", id).Count(&refs)
	var subRefs int64
	h.S.DB.Model(&db.Subscription{}).Where("price_id = ?", id).Count(&subRefs)
	if refs+subRefs > 0 {
		h.fail(w, http.StatusConflict, "conflict", "This price is used by existing orders or subscriptions.", nil)
		return
	}
	h.S.DB.Delete(&db.Price{}, id)
	h.audit(r, u, "price.delete", "price", id, nil)
	noContent(w)
}

// ---------------------------------------------------------------- files

func (h *Handler) listFiles(w http.ResponseWriter, r *http.Request, u *db.User) {
	p := parsePage(r)
	q := h.S.DB.Model(&db.File{}).Where("test_mode = ?", p.TestMode)
	if vid := qInt(r, "variant_id"); vid > 0 {
		q = q.Where("variant_id = ?", vid)
	}
	if s := r.URL.Query().Get("status"); s != "" {
		q = q.Where("status = ?", s)
	}
	if p.Q != "" {
		q = q.Where("LOWER(name) LIKE ? ESCAPE '\\'", like(p.Q))
	}
	q = applySort(q, p.Sort, map[string]string{"name": "name", "size": "size", "created_at": "created_at", "sort": "sort"}, "variant_id, sort, id")
	var rows []db.File
	meta, err := paginate(q, p, &rows)
	if err != nil {
		h.failErr(w, err)
		return
	}
	h.respondList(w, h.fileDTOs(rows), meta)
}

func (h *Handler) getFile(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	var f db.File
	if err := h.S.DB.First(&f, id).Error; err != nil {
		h.failErr(w, err)
		return
	}
	h.ok(w, h.fileDTOs([]db.File{f})[0])
}

func (h *Handler) updateFile(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	var f db.File
	if err := h.S.DB.First(&f, id).Error; err != nil {
		h.failErr(w, err)
		return
	}
	b, ok := h.readBody(w, r)
	if !ok {
		return
	}
	if s, ok := b.str("name"); ok && s != "" {
		f.Name = s
		f.Extension = strings.TrimPrefix(strings.ToLower(filepath.Ext(s)), ".")
	}
	if s, ok := b["version"].(string); ok {
		f.Version = strings.TrimSpace(s)
	}
	if n, ok := b.int("sort"); ok {
		f.Sort = int(n)
	}
	if s, ok := b.str("status"); ok {
		if s != "draft" && s != "published" {
			h.invalid(w, "status", "The status must be draft or published.")
			return
		}
		f.Status = s
	}
	f.UpdatedAt = db.Now()
	if err := h.S.DB.Save(&f).Error; err != nil {
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "file.update", "file", f.ID, nil)
	h.ok(w, h.fileDTOs([]db.File{f})[0])
}

func (h *Handler) deleteFile(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	var f db.File
	if err := h.S.DB.First(&f, id).Error; err != nil {
		h.failErr(w, err)
		return
	}
	_ = h.S.Storage.Delete(r.Context(), f.StorageKey)
	h.S.DB.Delete(&db.File{}, id)
	h.audit(r, u, "file.delete", "file", id, map[string]any{"name": f.Name})
	noContent(w)
}

func maxUploadBytes() int64 {
	mb := int64(1024)
	if v, err := strconv.ParseInt(os.Getenv("MAX_UPLOAD_MB"), 10, 64); err == nil && v > 0 {
		mb = v
	}
	return mb << 20
}

func (h *Handler) uploadFile(w http.ResponseWriter, r *http.Request, u *db.User) {
	vid, _ := pathID(r)
	var v db.Variant
	if err := h.S.DB.First(&v, vid).Error; err != nil {
		h.failErr(w, err)
		return
	}
	limit := maxUploadBytes()
	r.Body = http.MaxBytesReader(w, r.Body, limit+(1<<20))
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		h.fail(w, http.StatusRequestEntityTooLarge, "validation_failed", fmt.Sprintf("Upload failed or exceeds the %d MB limit.", limit>>20), nil)
		return
	}
	defer r.MultipartForm.RemoveAll()
	fhs := r.MultipartForm.File["file"]
	if len(fhs) == 0 {
		h.invalid(w, "file", "The file field is required.")
		return
	}
	fh := fhs[0]
	src, err := fh.Open()
	if err != nil {
		h.failErr(w, err)
		return
	}
	defer src.Close()
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		name = storage.SafeName(fh.Filename)
	} else if filepath.Ext(name) == "" {
		// the UI sends the display name without extension: keep the uploaded file's extension
		name += filepath.Ext(storage.SafeName(fh.Filename))
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	ct := mime.TypeByExtension("." + ext)
	if ct == "" {
		ct = fh.Header.Get("Content-Type")
	}
	status := r.FormValue("status")
	if status != "draft" {
		status = "published"
	}
	key := storage.NewKey(name)
	size, sha, err := h.S.Storage.Put(r.Context(), key, src, ct)
	if err != nil {
		h.failErr(w, err)
		return
	}
	var maxSort int
	h.S.DB.Model(&db.File{}).Where("variant_id = ?", vid).Select("COALESCE(MAX(sort),0)").Scan(&maxSort)
	now := db.Now()
	f := db.File{VariantID: vid, Identifier: uuid.NewString(), Name: name, Extension: ext, Size: size, Version: strings.TrimSpace(r.FormValue("version")),
		Sort: maxSort + 1, Status: status, StorageDriver: h.S.Storage.Driver(), StorageKey: key, ContentType: ct, SHA256: sha, TestMode: v.TestMode, CreatedAt: now, UpdatedAt: now}
	if err := h.S.DB.Create(&f).Error; err != nil {
		_ = h.S.Storage.Delete(r.Context(), key)
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "file.upload", "file", f.ID, map[string]any{"name": name, "size": size})
	h.created(w, h.fileDTOs([]db.File{f})[0])
}

func (h *Handler) downloadFile(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	var f db.File
	if err := h.S.DB.First(&f, id).Error; err != nil {
		h.failErr(w, err)
		return
	}
	storefront.Serve(r.Context(), h.S, w, r, f)
}
