package importer

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"

	"fulfillment/internal/db"
	"fulfillment/internal/storage"
)

var contentTypes = map[string]string{
	"zip": "application/zip", "pdf": "application/pdf", "png": "image/png", "jpg": "image/jpeg", "jpeg": "image/jpeg",
	"gif": "image/gif", "svg": "image/svg+xml", "webp": "image/webp", "mp4": "video/mp4", "mov": "video/quicktime",
	"mp3": "audio/mpeg", "wav": "audio/wav", "txt": "text/plain", "md": "text/markdown", "csv": "text/csv",
	"json": "application/json", "xml": "application/xml", "html": "text/html", "epub": "application/epub+zip",
	"gz": "application/gzip", "tar": "application/x-tar", "7z": "application/x-7z-compressed", "rar": "application/vnd.rar",
	"dmg": "application/x-apple-diskimage", "exe": "application/vnd.microsoft.portable-executable", "msi": "application/x-msi",
	"doc": "application/msword", "docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	"xls": "application/vnd.ms-excel", "xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	"ppt": "application/vnd.ms-powerpoint", "pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	"psd": "image/vnd.adobe.photoshop", "ttf": "font/ttf", "otf": "font/otf", "woff": "font/woff", "woff2": "font/woff2",
}

// contentTypeFor guesses the media type from the file extension.
func contentTypeFor(ext, name string) string {
	ext = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ext), "."))
	if ext == "" {
		ext = strings.ToLower(strings.TrimPrefix(path.Ext(name), "."))
	}
	if ct, ok := contentTypes[ext]; ok {
		return ct
	}
	if ext != "" {
		if ct := mime.TypeByExtension("." + ext); ct != "" {
			return ct
		}
	}
	return "application/octet-stream"
}

// httpFile starts the download of a (signed, short-lived) file URL. No
// credentials are sent: the signature in the URL is the authorisation.
func httpFile(ctx context.Context, c *http.Client, rawURL string) (io.ReadCloser, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("unsupported download URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "keypost-importer/1.0")
	if c == nil {
		c = defaultHTTP
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}
	return &limitedReadCloser{limitedReader{r: resp.Body, n: maxEntryBytes}, resp.Body}, nil
}

var (
	fileBaseCols    = []string{"variant_id", "identifier", "name", "extension", "version", "sort", "status", "test_mode", "created_at", "updated_at"}
	fileStorageCols = []string{"size", "storage_driver", "storage_key", "content_type", "sha256"}
)

func (in *ingest) mapFile(o lsObject) (db.File, bool) {
	a := o.A
	ident := a.trim("identifier")
	if ident == "" {
		in.invalid("files", fmt.Sprintf("#%d has no identifier", o.ID))
		return db.File{}, false
	}
	name := orStr(a.str("name"), fmt.Sprintf("file-%d", o.ID))
	ext := strings.TrimPrefix(strings.TrimSpace(a.str("extension")), ".")
	if ext == "" {
		ext = strings.TrimPrefix(path.Ext(name), ".")
	}
	created, updated := stamps(a)
	return db.File{
		ID: o.ID, VariantID: a.i64("variant_id"), Identifier: ident, Name: name, Extension: ext, Size: a.i64("size"), Version: a.str("version"),
		Sort: a.intv("sort"), Status: orStr(strings.ToLower(a.trim("status")), "published"), ContentType: contentTypeFor(ext, name),
		TestMode: in.testMode(a), CreatedAt: created, UpdatedAt: updated,
	}, true
}

// objectExists reports whether a stored object can still be opened.
func (in *ingest) objectExists(ctx context.Context, key string) bool {
	rc, _, err := in.svc.Storage.Open(ctx, key)
	if err != nil {
		return false
	}
	_ = rc.Close()
	return true
}

// fileHandler imports files one by one: the download URL of the API is only
// valid for an hour, so the bytes are fetched as soon as the row is read.
func (in *ingest) fileHandler() *stageHandler {
	h := &stageHandler{flush: func(context.Context) error { return nil }}
	guard := in.fileGuard()
	h.add = func(ctx context.Context, o lsObject) error {
		f, ok := in.mapFile(o)
		if !ok {
			return nil
		}
		in.j.count("files", 1)
		in.j.advance(1)
		kept, err := guard([]db.File{f})
		if err != nil || len(kept) == 0 || in.cfg.dryRun {
			return err
		}
		cols := in.storeFile(ctx, &f, o.A.str("download_url"))
		spec := upsertSpec{table: "files", cols: cols}
		failedBefore := in.failed
		err = writeBatch(in, "files", []db.File{f}, func(f db.File) int64 { return f.ID }, upsertWriter[db.File](in, spec))
		if in.failed > failedBefore && f.StorageKey != "" {
			_ = in.svc.Storage.Delete(ctx, f.StorageKey) // do not leave an object nothing refers to
		}
		return err
	}
	return h
}

// storeFile makes sure the bytes of f are in storage (keeping an object that
// is already there) and returns the columns the upsert may overwrite.
func (in *ingest) storeFile(ctx context.Context, f *db.File, downloadURL string) []string {
	var existing []db.File
	in.g.Where("identifier = ?", f.Identifier).Limit(1).Find(&existing)
	if len(existing) > 0 && existing[0].StorageKey != "" && in.objectExists(ctx, existing[0].StorageKey) {
		in.j.count("files_kept", 1)
		return fileBaseCols
	}
	rc, err := in.src.openFile(ctx, fileRef{Identifier: f.Identifier, Name: f.Name, URL: downloadURL})
	switch {
	case err == nil:
		defer rc.Close()
		key := storage.NewKey(f.Name)
		size, sha, perr := in.svc.Storage.Put(ctx, key, rc, f.ContentType)
		if perr != nil {
			in.j.count("files_missing", 1)
			in.j.warn("file %s (#%d, %s): could not be stored: %v; re-run the import to retry", f.Identifier, f.ID, f.Name, perr)
			break
		}
		if f.Size != 0 && f.Size != size {
			in.j.warn("file %s (#%d, %s): stored %d bytes but Lemon Squeezy reported %d", f.Identifier, f.ID, f.Name, size, f.Size)
		}
		f.Size, f.SHA256, f.StorageKey, f.StorageDriver = size, sha, key, in.svc.Storage.Driver()
		in.j.count("files_stored", 1)
		return append(append([]string{}, fileBaseCols...), fileStorageCols...)
	case err == errNoFileData:
		in.j.count("files_missing", 1)
		in.j.warn("file %s (#%d, %s, variant #%d): no file data was imported; add the file again or re-run the import with file download enabled", f.Identifier, f.ID, f.Name, f.VariantID)
	default:
		in.j.count("files_missing", 1)
		in.j.warn("file %s (#%d, %s): download failed: %v; re-run the import to retry", f.Identifier, f.ID, f.Name, err)
	}
	if len(existing) > 0 && existing[0].StorageKey != "" {
		return fileBaseCols // keep the stale reference rather than blanking it
	}
	return append(append([]string{}, fileBaseCols...), "size")
}
