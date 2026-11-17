// Package urls builds the public, signed URLs that appear in API payloads and
// emails (receipts, downloads, customer portal, buy links).
package urls

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	"fulfillment/internal/crypt"
)

type Builder struct {
	Base        string // e.g. https://pay.example.com (no trailing slash)
	DownloadTTL time.Duration
}

func (b Builder) API(path string) string { return b.Base + "/v1" + path }

func (b Builder) ReceiptSignature(identifier string) string {
	return crypt.MAC("receipt", identifier)
}

func (b Builder) VerifyReceipt(identifier, sig string) bool {
	return sig != "" && crypt.MACEqual(sig, b.ReceiptSignature(identifier))
}

// Receipt is the permanent signed link to the customer's order page.
func (b Builder) Receipt(identifier string) string {
	return fmt.Sprintf("%s/my-orders/%s?signature=%s", b.Base, identifier, b.ReceiptSignature(identifier))
}

func (b Builder) downloadSig(fileIdentifier string, expires int64, orderID int64) string {
	return crypt.MAC("download", fileIdentifier, strconv.FormatInt(expires, 10), strconv.FormatInt(orderID, 10))
}

// Download returns a relative or absolute URL to the signed download endpoint.
func (b Builder) Download(fileIdentifier string, orderID int64, now time.Time, absolute bool) string {
	exp := now.Add(b.DownloadTTL).Unix()
	path := fmt.Sprintf("/download/%s?expires=%d&order=%d&signature=%s", fileIdentifier, exp, orderID, b.downloadSig(fileIdentifier, exp, orderID))
	if absolute {
		return b.Base + path
	}
	return path
}

func (b Builder) VerifyDownload(fileIdentifier string, expires, orderID int64, sig string, now time.Time) bool {
	if expires < now.Unix() {
		return false
	}
	return sig != "" && crypt.MACEqual(sig, b.downloadSig(fileIdentifier, expires, orderID))
}

func (b Builder) portalSig(customerID int64, subscriptionID int64, expires int64) string {
	return crypt.MAC("portal", strconv.FormatInt(customerID, 10), strconv.FormatInt(subscriptionID, 10), strconv.FormatInt(expires, 10))
}

// Portal returns the 24h pre-signed customer portal link.
func (b Builder) Portal(customerID, subscriptionID int64, now time.Time, absolute bool) string {
	exp := now.Add(24 * time.Hour).Unix()
	q := url.Values{}
	q.Set("customer", strconv.FormatInt(customerID, 10))
	if subscriptionID > 0 {
		q.Set("subscription", strconv.FormatInt(subscriptionID, 10))
	}
	q.Set("expires", strconv.FormatInt(exp, 10))
	q.Set("signature", b.portalSig(customerID, subscriptionID, exp))
	p := "/billing?" + q.Encode()
	if absolute {
		return b.Base + p
	}
	return p
}

func (b Builder) VerifyPortal(customerID, subscriptionID, expires int64, sig string, now time.Time) bool {
	if expires < now.Unix() {
		return false
	}
	return sig != "" && crypt.MACEqual(sig, b.portalSig(customerID, subscriptionID, expires))
}

func (b Builder) Buy(variantSlug string) string { return b.Base + "/checkout/buy/" + variantSlug }

func (b Builder) MagicLink(token string) string {
	return b.Base + "/my-orders/verify?token=" + url.QueryEscape(token)
}
