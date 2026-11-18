package admin

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"fulfillment/internal/db"
	"fulfillment/internal/importer"
)

func importDTO(j db.ImportJob) map[string]any {
	opts := j.Options.V
	if opts == nil {
		opts = map[string]any{}
	}
	prog := j.Progress.V
	if prog == nil {
		prog = map[string]any{}
	}
	return map[string]any{
		"id": j.ID, "kind": j.Kind, "status": j.Status, "options": opts, "progress": prog, "error": j.Error, "created_by": j.CreatedBy,
		"started_at": tsp(j.StartedAt), "finished_at": tsp(j.FinishedAt), "created_at": ts(j.CreatedAt),
	}
}

func (h *Handler) listImports(w http.ResponseWriter, r *http.Request, u *db.User) {
	p := parsePage(r)
	q := h.S.DB.Model(&db.ImportJob{})
	if s := r.URL.Query().Get("status"); s != "" {
		q = q.Where("status = ?", s)
	}
	q = q.Order("id DESC")
	var rows []db.ImportJob
	meta, err := paginate(q, p, &rows)
	if err != nil {
		h.failErr(w, err)
		return
	}
	out := make([]map[string]any, len(rows))
	for i, j := range rows {
		out[i] = importDTO(j)
	}
	h.respondList(w, out, meta)
}

func (h *Handler) getImport(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	var j db.ImportJob
	if err := h.S.DB.First(&j, id).Error; err != nil {
		h.failErr(w, err)
		return
	}
	h.ok(w, importDTO(j))
}

func (h *Handler) importLogs(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	after := qInt(r, "after")
	var logs []db.ImportLog
	h.S.DB.Where("job_id = ? AND id > ?", id, after).Order("id").Limit(500).Find(&logs)
	out := make([]map[string]any, len(logs))
	for i, l := range logs {
		out[i] = map[string]any{"id": l.ID, "level": l.Level, "message": l.Message, "created_at": ts(l.CreatedAt)}
	}
	h.ok(w, out)
}

func (h *Handler) cancelImport(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	if !h.Imp.Cancel(id) {
		h.fail(w, http.StatusConflict, "conflict", "This import is not running.", nil)
		return
	}
	h.audit(r, u, "import.cancel", "import", id, nil)
	var j db.ImportJob
	h.S.DB.First(&j, id)
	h.ok(w, importDTO(j))
}

func (h *Handler) importLS(w http.ResponseWriter, r *http.Request, u *db.User) {
	b, ok := h.readBody(w, r)
	if !ok {
		return
	}
	key, _ := b.str("api_key")
	if key == "" {
		h.invalid(w, "api_key", "Paste a Lemon Squeezy API key.")
		return
	}
	opts := importer.LSOptions{APIKey: key}
	opts.TestMode, _ = b.boolean("test_mode")
	opts.DownloadFiles, _ = b.boolean("download_files")
	opts.DryRun, _ = b.boolean("dry_run")
	opts.OnConflict, _ = b.str("on_conflict")
	if raw, ok := b["store_ids"].([]any); ok {
		for _, e := range raw {
			if f, ok := e.(float64); ok {
				opts.StoreIDs = append(opts.StoreIDs, int64(f))
			}
		}
	}
	if res, ok := b.strList("resources"); ok {
		opts.Resources = res
	}
	job, err := h.Imp.StartLemonSqueezyAPI(u.Email, opts)
	if err != nil {
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "import.start", "import", job.ID, map[string]any{"kind": "lemonsqueezy_api", "dry_run": opts.DryRun, "on_conflict": opts.OnConflict})
	h.created(w, importDTO(*job))
}

// saveUpload stores the uploaded multipart field "file" in a temp file the importer will own.
func (h *Handler) saveUpload(w http.ResponseWriter, r *http.Request, maxBytes int64) (path string, ok bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		h.fail(w, http.StatusRequestEntityTooLarge, "validation_failed", "Upload failed or is too large.", nil)
		return "", false
	}
	fhs := r.MultipartForm.File["file"]
	if len(fhs) == 0 {
		h.invalid(w, "file", "The file field is required.")
		return "", false
	}
	src, err := fhs[0].Open()
	if err != nil {
		h.failErr(w, err)
		return "", false
	}
	defer src.Close()
	dir := filepath.Join(h.S.Cfg.DataDir, "tmp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		h.failErr(w, err)
		return "", false
	}
	f, err := os.CreateTemp(dir, "import-*")
	if err != nil {
		h.failErr(w, err)
		return "", false
	}
	if _, err := io.Copy(f, src); err != nil {
		f.Close()
		os.Remove(f.Name())
		h.failErr(w, err)
		return "", false
	}
	f.Close()
	return f.Name(), true
}

func (h *Handler) importLSArchive(w http.ResponseWriter, r *http.Request, u *db.User) {
	path, ok := h.saveUpload(w, r, 8<<30)
	if !ok {
		return
	}
	defer r.MultipartForm.RemoveAll()
	opts := importer.ArchiveOptions{
		DownloadFiles: r.FormValue("download_files") == "true",
		DryRun:        r.FormValue("dry_run") == "true",
		TestMode:      r.FormValue("test_mode") == "true",
		OnConflict:    r.FormValue("on_conflict"),
	}
	job, err := h.Imp.StartLemonSqueezyArchive(u.Email, path, opts)
	if err != nil {
		os.Remove(path)
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "import.start", "import", job.ID, map[string]any{"kind": "lemonsqueezy_files", "dry_run": opts.DryRun, "on_conflict": opts.OnConflict})
	h.created(w, importDTO(*job))
}

func (h *Handler) importMapping(w http.ResponseWriter, r *http.Request, u *db.User) {
	path, ok := h.saveUpload(w, r, 64<<20)
	if !ok {
		return
	}
	defer r.MultipartForm.RemoveAll()
	kind := strings.TrimSpace(r.FormValue("kind"))
	if kind == "" {
		kind = "customers"
	}
	if kind != "customers" && kind != "products" && kind != "subscriptions" {
		os.Remove(path)
		h.invalid(w, "kind", "The kind must be customers, products or subscriptions.")
		return
	}
	job, err := h.Imp.StartMappingCSV(u.Email, kind, path)
	if err != nil {
		os.Remove(path)
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "import.start", "import", job.ID, map[string]any{"kind": "stripe_mapping", "mapping": kind})
	h.created(w, importDTO(*job))
}

func (h *Handler) importAutoMatch(w http.ResponseWriter, r *http.Request, u *db.User) {
	b, ok := h.readBody(w, r)
	if !ok {
		return
	}
	o := importer.MatchOptions{}
	o.Customers, _ = b.boolean("customers")
	o.Products, _ = b.boolean("products")
	o.Subscriptions, _ = b.boolean("subscriptions")
	if !o.Customers && !o.Products && !o.Subscriptions {
		o = importer.MatchOptions{Customers: true, Products: true, Subscriptions: true}
	}
	job, err := h.Imp.StartAutoMatch(u.Email, o)
	if err != nil {
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "import.start", "import", job.ID, map[string]any{"kind": "stripe_auto_match"})
	h.created(w, importDTO(*job))
}

func (h *Handler) mappingReport(w http.ResponseWriter, r *http.Request, u *db.User) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="stripe-mapping-report.csv"`)
	if err := h.Imp.MappingReport(w); err != nil {
		h.S.Log.Error("mapping report", "err", err)
	}
}

var _ = strconv.Itoa
