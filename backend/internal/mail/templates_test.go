package mail

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sampleReceipt() ReceiptData {
	return ReceiptData{
		StoreName: "Somuno", Intro: "Thank you for supporting us.", CustomerName: "Jane", OrderNumber: 5149903, OrderDate: "Nov 18, 2026",
		Subtotal: "$20.00", Discount: "$5.00", Tax: "$1.20", Total: "$16.20", PaidWith: "Visa ending in 4242",
		Items:        []ReceiptItem{{Name: "Pro App", Variant: "Lifetime", Price: "$20.00"}},
		Keys:         []ReceiptKey{{Product: "Pro App", Key: "7b75887d-96e4-42fa-94f3-c4f3ff73cdb5"}},
		Files:        []ReceiptFile{{Name: "app.zip", Size: "12.4 MB", URL: "https://pay.example.com/my-orders/x?signature=s"}},
		ReceiptURL:   "https://pay.example.com/my-orders/x?signature=s",
		SupportEmail: "help@example.com",
	}
}

func TestReceiptHTMLAndText(t *testing.T) {
	html, text, err := RenderReceipt(sampleReceipt())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Thanks for your order, Jane", "Order #5149903", "Nov 18, 2026", "Thank you for supporting us.", "Pro App", "Lifetime", "$20.00",
		"Subtotal", "Discount", "−$5.00", "Tax", "$1.20", "$16.20", "Paid with Visa ending in 4242",
		"Your license key", "7b75887d-96e4-42fa-94f3-c4f3ff73cdb5", "Downloads", "app.zip", "12.4 MB", "Download &rarr;",
		`href="https://pay.example.com/my-orders/x?signature=s"`, "View your order", "mailto:help@example.com",
		`name="color-scheme"`, "prefers-color-scheme: dark", "max-width: 620px",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("html lacks %q", want)
		}
	}
	for _, want := range []string{"Somuno", "Order #5149903", "ORDER SUMMARY", "Discount", "-$5.00", "Total", "$16.20", "LICENSE KEYS", "7b75887d-96e4-42fa-94f3-c4f3ff73cdb5", "DOWNLOADS", "app.zip (12.4 MB)", "View your order:", "help@example.com"} {
		if !strings.Contains(text, want) {
			t.Errorf("text lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "<") {
		t.Errorf("the plain-text part must not contain markup:\n%s", text)
	}
}

func TestReceiptOptionalSections(t *testing.T) {
	d := sampleReceipt()
	d.Keys, d.Files, d.Discount, d.Tax, d.Subtotal, d.PaidWith, d.SupportEmail, d.Intro, d.CustomerName = nil, nil, "", "", "", "", "", "", ""
	html, text, err := RenderReceipt(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"license key", "Downloads", "Subtotal", "Paid with", "mailto:", "Thank you for supporting"} {
		if strings.Contains(html, gone) || strings.Contains(text, gone) {
			t.Errorf("%q must not appear without its data", gone)
		}
	}
	if !strings.Contains(html, "Thanks for your order</h1>") {
		t.Error("no customer name: no trailing comma")
	}
	d.Keys = []ReceiptKey{{Key: "a"}, {Key: "b"}}
	html, _, _ = RenderReceipt(d)
	if !strings.Contains(html, "Your license keys") || !strings.Contains(html, "Keep them private") {
		t.Error("several keys use the plural")
	}
}

func TestTemplatesEscapeEverythingTheyPrint(t *testing.T) {
	evil := `<script>alert(1)</script>`
	d := sampleReceipt()
	d.StoreName, d.Intro, d.CustomerName = evil, evil, evil
	d.Items[0].Name, d.Keys[0].Key, d.Files[0].Name = evil, evil, evil
	d.AvatarURL = `javascript:alert(1)`
	html, _, err := RenderReceipt(d)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "<script>") || strings.Contains(html, "javascript:") {
		t.Errorf("unescaped input reached the HTML")
	}
	ml, _, _ := RenderMagicLink(MagicLinkData{StoreName: evil, Link: `javascript:alert(1)`})
	if strings.Contains(ml, "<script>") || strings.Contains(ml, `href="javascript:`) {
		t.Errorf("unescaped input reached the sign-in e-mail")
	}
}

func TestLayoutBadgeAndAvatar(t *testing.T) {
	for _, c := range []struct{ store, want string }{{"somuno", "S"}, {"  acme", "A"}, {"雪花", "雪"}, {"", "•"}} {
		if got := newLayout("t", "p", c.store, "", "f").Initial; got != c.want {
			t.Errorf("initial of %q = %q, want %q", c.store, got, c.want)
		}
	}
	if newLayout("t", "p", "S", "https://cdn.example.com/a.png", "f").AvatarURL == "" {
		t.Error("an https logo is used")
	}
	for _, bad := range []string{"/relative.png", "javascript:alert(1)", "data:image/png;base64,AAAA", "ftp://x/y.png"} {
		if newLayout("t", "p", "S", bad, "f").AvatarURL != "" {
			t.Errorf("%q must not be used as a logo", bad)
		}
	}
	html, _, _ := RenderMagicLink(MagicLinkData{StoreName: "Somuno", AvatarURL: "https://cdn.example.com/a.png", Link: "https://x.test/l"})
	if !strings.Contains(html, `<img src="https://cdn.example.com/a.png"`) {
		t.Error("the store logo should replace the monogram")
	}
}

func TestMagicLinkAndTestMessage(t *testing.T) {
	html, text, err := RenderMagicLink(MagicLinkData{StoreName: "Somuno", Link: "https://pay.example.com/my-orders/verify?token=abc"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(html, "https://pay.example.com/my-orders/verify?token=abc") != 2 { // the button and the "copy this link" fallback
		t.Errorf("the link must be in the button and in the fallback text")
	}
	for _, want := range []string{"Sign in to your orders", "expires in 15 minutes", "ignore it"} {
		if !strings.Contains(html, want) {
			t.Errorf("sign-in html lacks %q", want)
		}
	}
	if !strings.Contains(text, "https://pay.example.com/my-orders/verify?token=abc") || !strings.Contains(text, "15 minutes") {
		t.Errorf("sign-in text: %s", text)
	}
	html, text, err = RenderTest(TestData{StoreName: "Somuno", Product: "Keypost"})
	if err != nil || !strings.Contains(html, "Your e-mail settings work") || !strings.Contains(html, "Keypost") || !strings.Contains(text, "SMTP") {
		t.Errorf("test message: %v\n%s", err, text)
	}
}

// MAIL_SAMPLES_DIR=/some/dir go test ./internal/mail -run Samples writes the messages to files for a visual check.
func TestWriteSamples(t *testing.T) {
	dir := os.Getenv("MAIL_SAMPLES_DIR")
	if dir == "" {
		t.Skip("set MAIL_SAMPLES_DIR to write sample e-mails")
	}
	write := func(name, html, text string) {
		if err := os.WriteFile(filepath.Join(dir, name+".html"), []byte(html), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".txt"), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h, x, _ := RenderReceipt(sampleReceipt())
	write("receipt", h, x)
	two := sampleReceipt()
	two.Discount, two.Tax, two.Subtotal, two.CustomerName, two.Intro = "", "", "", "", ""
	two.Items = append(two.Items, ReceiptItem{Name: "Icon pack", Variant: "", Price: "$9.00"})
	two.Keys = append(two.Keys, ReceiptKey{Product: "Icon pack", Key: "XXXX-YYYY-ZZZZ-1234"})
	two.Files = append(two.Files, ReceiptFile{Name: "icons-2026-final-v2.zip", Size: "85 MB", URL: two.ReceiptURL})
	two.Total = "$29.00"
	h, x, _ = RenderReceipt(two)
	write("receipt2", h, x)
	h, x, _ = RenderMagicLink(MagicLinkData{StoreName: "Somuno", Link: "https://pay.example.com/my-orders/verify?token=VZVQr-IG-YBo9CWZUYUgHf0meKdO0L_zTeQHWvB6YUw"})
	write("signin", h, x)
	h, x, _ = RenderTest(TestData{StoreName: "Somuno", Product: "Keypost"})
	write("test", h, x)
}
