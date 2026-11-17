// Package storage abstracts where uploaded product files live: the local
// disk (default) or any S3 compatible object store.
package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"fulfillment/internal/config"
)

var ErrNotFound = errors.New("storage: object not found")

type Storage interface {
	Driver() string
	// Put stores r under key and returns the stored size and sha256 hex.
	Put(ctx context.Context, key string, r io.Reader, contentType string) (size int64, sha string, err error)
	Open(ctx context.Context, key string) (io.ReadCloser, int64, error)
	Delete(ctx context.Context, key string) error
	// PresignGet returns a time limited direct URL, or "" when the driver
	// cannot presign (the caller then streams through the API).
	PresignGet(ctx context.Context, key, filename string, ttl time.Duration) (string, error)
}

// NewKey builds a collision-free, path-safe object key for an upload.
func NewKey(filename string) string {
	name := SafeName(filename)
	now := time.Now().UTC()
	return fmt.Sprintf("%04d/%02d/%s/%s", now.Year(), int(now.Month()), uuid.NewString(), name)
}

func SafeName(filename string) string {
	name := path.Base(strings.ReplaceAll(filename, "\\", "/"))
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.TrimLeft(b.String(), ".")
	if out == "" {
		out = "file"
	}
	return out
}

func New(c *config.Config) (Storage, error) {
	switch c.StorageDriver {
	case "", "local":
		return NewLocal(filepath.Join(c.DataDir, "files"))
	case "s3":
		return NewS3(c.S3)
	}
	return nil, fmt.Errorf("unknown STORAGE_DRIVER %q", c.StorageDriver)
}

// ---------------------------------------------------------------- local

type Local struct{ root string }

func NewLocal(root string) (*Local, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	return &Local{root: abs}, nil
}

func (l *Local) Driver() string { return "local" }

func (l *Local) resolve(key string) (string, error) {
	p := filepath.Join(l.root, filepath.FromSlash(key))
	if !strings.HasPrefix(p, l.root+string(filepath.Separator)) {
		return "", errors.New("storage: invalid key")
	}
	return p, nil
}

func (l *Local) Put(_ context.Context, key string, r io.Reader, _ string) (int64, string, error) {
	p, err := l.resolve(key)
	if err != nil {
		return 0, "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return 0, "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".upload-*")
	if err != nil {
		return 0, "", err
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), r)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return 0, "", err
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

func (l *Local) Open(_ context.Context, key string) (io.ReadCloser, int64, error) {
	p, err := l.resolve(key)
	if err != nil {
		return nil, 0, err
	}
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, ErrNotFound
		}
		return nil, 0, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, st.Size(), nil
}

func (l *Local) Delete(_ context.Context, key string) error {
	p, err := l.resolve(key)
	if err != nil {
		return err
	}
	err = os.Remove(p)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	// best effort: remove the now empty uuid directory
	_ = os.Remove(filepath.Dir(p))
	return nil
}

func (l *Local) PresignGet(context.Context, string, string, time.Duration) (string, error) {
	return "", nil
}

// ---------------------------------------------------------------- s3

type S3 struct {
	cli    *minio.Client
	bucket string
	prefix string
}

func NewS3(c config.S3Config) (*S3, error) {
	if c.Bucket == "" || c.Endpoint == "" {
		return nil, errors.New("S3_ENDPOINT and S3_BUCKET are required for STORAGE_DRIVER=s3")
	}
	endpoint := strings.TrimPrefix(strings.TrimPrefix(c.Endpoint, "https://"), "http://")
	lookup := minio.BucketLookupAuto
	if c.PathStyle {
		lookup = minio.BucketLookupPath
	}
	cli, err := minio.New(endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(c.AccessKey, c.SecretKey, ""),
		Secure:       c.UseSSL,
		Region:       c.Region,
		BucketLookup: lookup,
	})
	if err != nil {
		return nil, err
	}
	return &S3{cli: cli, bucket: c.Bucket, prefix: c.Prefix}, nil
}

func (s *S3) Driver() string { return "s3" }

func (s *S3) obj(key string) string {
	if s.prefix == "" {
		return key
	}
	return s.prefix + "/" + key
}

type hashReader struct {
	r io.Reader
	h interface{ Write([]byte) (int, error) }
	n int64
}

func (h *hashReader) Read(p []byte) (int, error) {
	n, err := h.r.Read(p)
	if n > 0 {
		h.h.Write(p[:n])
		h.n += int64(n)
	}
	return n, err
}

func (s *S3) Put(ctx context.Context, key string, r io.Reader, contentType string) (int64, string, error) {
	hs := sha256.New()
	hr := &hashReader{r: r, h: hs}
	_, err := s.cli.PutObject(ctx, s.bucket, s.obj(key), hr, -1, minio.PutObjectOptions{ContentType: contentType, PartSize: 16 << 20})
	if err != nil {
		return 0, "", err
	}
	return hr.n, hex.EncodeToString(hs.Sum(nil)), nil
}

func (s *S3) Open(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	o, err := s.cli.GetObject(ctx, s.bucket, s.obj(key), minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, err
	}
	st, err := o.Stat()
	if err != nil {
		o.Close()
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, 0, ErrNotFound
		}
		return nil, 0, err
	}
	return o, st.Size, nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	return s.cli.RemoveObject(ctx, s.bucket, s.obj(key), minio.RemoveObjectOptions{})
}

func (s *S3) PresignGet(ctx context.Context, key, filename string, ttl time.Duration) (string, error) {
	q := url.Values{}
	q.Set("response-content-disposition", fmt.Sprintf(`attachment; filename="%s"`, strings.ReplaceAll(filename, `"`, "")))
	u, err := s.cli.PresignedGetObject(ctx, s.bucket, s.obj(key), ttl, q)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}
