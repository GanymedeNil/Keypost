package urls

import (
	"net/url"
	"strconv"
	"testing"
	"time"

	"fulfillment/internal/crypt"
)

func TestSignedURLs(t *testing.T) {
	crypt.SetSecret("urls-test-secret-0123456789")
	b := Builder{Base: "https://pay.example.com", DownloadTTL: time.Hour}
	now := time.Now()

	if !b.VerifyReceipt("abc", b.ReceiptSignature("abc")) || b.VerifyReceipt("abd", b.ReceiptSignature("abc")) || b.VerifyReceipt("abc", "") {
		t.Fatal("receipt signatures")
	}

	raw := b.Download("file-1", 7, now, true)
	u, _ := url.Parse(raw)
	exp, _ := strconv.ParseInt(u.Query().Get("expires"), 10, 64)
	sig := u.Query().Get("signature")
	if !b.VerifyDownload("file-1", exp, 7, sig, now) {
		t.Fatal("fresh download link rejected")
	}
	if b.VerifyDownload("file-2", exp, 7, sig, now) || b.VerifyDownload("file-1", exp, 8, sig, now) || b.VerifyDownload("file-1", exp+1, 7, sig, now) {
		t.Fatal("download link must bind file, order and expiry")
	}
	if b.VerifyDownload("file-1", exp, 7, sig, now.Add(2*time.Hour)) {
		t.Fatal("expired download link accepted")
	}

	p, _ := url.Parse(b.Portal(5, 9, now, true))
	pe, _ := strconv.ParseInt(p.Query().Get("expires"), 10, 64)
	if !b.VerifyPortal(5, 9, pe, p.Query().Get("signature"), now) || b.VerifyPortal(6, 9, pe, p.Query().Get("signature"), now) {
		t.Fatal("portal signature")
	}
}
