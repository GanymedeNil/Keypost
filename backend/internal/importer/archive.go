package importer

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"sort"
	"strings"
)

// Limits that keep a hostile or broken archive from exhausting memory or
// disk. They are variables so tests can lower them.
var (
	maxZipEntries = 200_000
	maxEntryBytes = int64(4 << 30)
)

var errEntryTooLarge = errors.New("archive entry exceeds the size limit")

// limitedReader fails (instead of silently truncating) once more than n
// bytes have been read; the size declared in a ZIP header cannot be trusted.
type limitedReader struct {
	r io.Reader
	n int64
}

func (l *limitedReader) Read(p []byte) (int, error) {
	n, err := l.r.Read(p)
	l.n -= int64(n)
	if l.n < 0 {
		return n, errEntryTooLarge
	}
	return n, err
}

type limitedReadCloser struct {
	limitedReader
	c io.Closer
}

func (l *limitedReadCloser) Close() error { return l.c.Close() }

type binEntry struct {
	rest string // path below the files/ directory, lower case
	f    *zip.File
}

// archiveSource reads a ZIP of Lemon Squeezy exports. Nothing is extracted to
// disk: entries are streamed straight from the archive, so path traversal
// in entry names cannot write anywhere.
type archiveSource struct {
	zr            *zip.ReadCloser
	j             *jobCtl
	json          map[string][]*zip.File
	ordersCSV     []*zip.File
	bins          map[string]*zip.File
	binList       []binEntry
	downloadFiles bool
	httpc         *http.Client
}

func openArchive(zipPath string, j *jobCtl, downloadFiles bool, httpc *http.Client) (*archiveSource, error) {
	zr, err := zip.OpenReader(zipPath)
	// entry names are validated one by one below, so an "insecure path"
	// report from the standard library is not fatal
	if err != nil && !errors.Is(err, zip.ErrInsecurePath) {
		return nil, fmt.Errorf("not a readable ZIP archive: %w", err)
	}
	if len(zr.File) > maxZipEntries {
		zr.Close()
		return nil, fmt.Errorf("the archive has %d entries (limit %d)", len(zr.File), maxZipEntries)
	}
	s := &archiveSource{zr: zr, j: j, json: map[string][]*zip.File{}, bins: map[string]*zip.File{}, downloadFiles: downloadFiles, httpc: httpc}
	s.scan()
	return s, nil
}

func (s *archiveSource) Close() error { return s.zr.Close() }

// entryName returns the cleaned slash-separated name, or ok=false for names
// that are absolute or climb out of the archive root.
func entryName(raw string) (string, bool) {
	name := strings.ReplaceAll(raw, "\\", "/")
	if strings.HasPrefix(name, "/") || (len(name) > 1 && name[1] == ':') {
		return "", false
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == ".." {
			return "", false
		}
	}
	return path.Clean(name), true
}

var jsonExts = map[string]bool{".json": true, ".ndjson": true, ".jsonl": true}

// resourceOf maps the base name of an export file onto a resource:
// orders.json, orders-2.json, license_keys.ndjson ...
func resourceOf(base string) (string, bool) {
	ext := strings.ToLower(path.Ext(base))
	stem := strings.TrimSuffix(strings.ToLower(base), ext)
	stem = strings.ReplaceAll(stem, "_", "-")
	names := make([]string, len(stageOrder))
	for i, r := range stageOrder {
		names[i] = strings.ReplaceAll(r, "_", "-")
	}
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	for _, n := range names {
		if stem == n {
			return strings.ReplaceAll(n, "-", "_"), true
		}
		if strings.HasPrefix(stem, n) {
			if c := stem[len(n)]; c < 'a' || c > 'z' {
				return strings.ReplaceAll(n, "-", "_"), true
			}
		}
	}
	return "", false
}

func (s *archiveSource) scan() {
	ignored := 0
	for _, f := range s.zr.File {
		if f.FileInfo().IsDir() || strings.HasSuffix(f.Name, "/") {
			continue
		}
		name, ok := entryName(f.Name)
		if !ok {
			s.j.warn("archive entry %q was ignored: unsafe path", f.Name)
			continue
		}
		if strings.HasPrefix(name, "__MACOSX/") || strings.HasPrefix(path.Base(name), "._") {
			continue
		}
		if f.UncompressedSize64 > uint64(maxEntryBytes) {
			s.j.warn("archive entry %q was ignored: %d bytes exceeds the %d byte limit", name, f.UncompressedSize64, maxEntryBytes)
			continue
		}
		parts := strings.Split(name, "/")
		base := parts[len(parts)-1]
		if idx := indexOfDir(parts[:len(parts)-1], "files"); idx >= 0 {
			s.addBinary(strings.Join(parts[idx+1:], "/"), f)
			continue
		}
		ext := strings.ToLower(path.Ext(base))
		switch {
		case jsonExts[ext]:
			if res, ok := resourceOf(base); ok {
				s.json[res] = append(s.json[res], f)
				continue
			}
		case ext == ".csv" && strings.HasPrefix(strings.ToLower(base), "orders"):
			s.ordersCSV = append(s.ordersCSV, f)
			continue
		}
		ignored++
	}
	for _, l := range s.json {
		sort.Slice(l, func(i, j int) bool { return naturalLess(l[i].Name, l[j].Name) })
	}
	sort.Slice(s.ordersCSV, func(i, j int) bool { return naturalLess(s.ordersCSV[i].Name, s.ordersCSV[j].Name) })
	sort.Slice(s.binList, func(i, j int) bool { return s.binList[i].rest < s.binList[j].rest })
	if ignored > 0 {
		s.j.info("%d archive entries were not recognised and ignored", ignored)
	}
	if len(s.json["orders"]) > 0 && len(s.ordersCSV) > 0 {
		s.j.info("orders.json is present: orders.csv is ignored")
	}
}

func indexOfDir(dirs []string, name string) int {
	for i, d := range dirs {
		if strings.EqualFold(d, name) {
			return i
		}
	}
	return -1
}

func (s *archiveSource) addBinary(rest string, f *zip.File) {
	rest = strings.ToLower(rest)
	s.binList = append(s.binList, binEntry{rest: rest, f: f})
	s.bins[rest] = f
	ident := rest
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		ident = rest[:i]
	} else if i := strings.IndexByte(rest, '.'); i >= 0 {
		ident = rest[:i]
	}
	if _, taken := s.bins[ident]; !taken {
		s.bins[ident] = f
	}
}

// naturalLess orders orders-2.json before orders-10.json.
func naturalLess(a, b string) bool {
	for a != "" && b != "" {
		da, db := digitPrefix(a), digitPrefix(b)
		if da != "" && db != "" {
			da, db = strings.TrimLeft(da, "0"), strings.TrimLeft(db, "0")
			if len(da) != len(db) {
				return len(da) < len(db)
			}
			if da != db {
				return da < db
			}
			a, b = a[len(digitPrefix(a)):], b[len(digitPrefix(b)):]
			continue
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

func digitPrefix(s string) string {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i]
}

// csvDerived reports whether a resource is rebuilt from orders.csv.
func (s *archiveSource) csvDerived(res string) bool {
	if len(s.ordersCSV) == 0 || len(s.json["orders"]) > 0 {
		return false
	}
	switch res {
	case "orders", "order_items":
		return len(s.json[res]) == 0
	case "customers":
		return len(s.json["customers"]) == 0
	}
	return false
}

func (s *archiveSource) has(res string) bool {
	return len(s.json[res]) > 0 || s.csvDerived(res)
}

func (s *archiveSource) stream(ctx context.Context, res string, h *streamHandler) error {
	if len(s.json[res]) == 0 && s.csvDerived(res) {
		return s.streamCSV(ctx, res, h)
	}
	for _, f := range s.json[res] {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.streamEntry(f, h); err != nil {
			var he *handlerError
			if errors.As(err, &he) {
				return he.err
			}
			s.j.count("invalid", 1)
			s.j.errorf("%s: %s: %v (the rest of this file was skipped)", res, f.Name, err)
		}
	}
	return nil
}

func (s *archiveSource) streamEntry(f *zip.File, h *streamHandler) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	return streamJSON(&limitedReader{r: rc, n: maxEntryBytes}, h)
}

func (s *archiveSource) findBinary(identifier string) *zip.File {
	id := strings.ToLower(strings.TrimSpace(identifier))
	if id == "" {
		return nil
	}
	if f, ok := s.bins[id]; ok {
		return f
	}
	// "<identifier>-name.zip" style entries: binary search for the prefix
	i := sort.Search(len(s.binList), func(i int) bool { return s.binList[i].rest >= id })
	if i < len(s.binList) && strings.HasPrefix(s.binList[i].rest, id) {
		return s.binList[i].f
	}
	return nil
}

func (s *archiveSource) openFile(ctx context.Context, f fileRef) (io.ReadCloser, error) {
	if e := s.findBinary(f.Identifier); e != nil {
		rc, err := e.Open()
		if err != nil {
			return nil, err
		}
		return &limitedReadCloser{limitedReader{r: rc, n: maxEntryBytes}, rc}, nil
	}
	if s.downloadFiles && f.URL != "" {
		return httpFile(ctx, s.httpc, f.URL)
	}
	return nil, errNoFileData
}
