package mail

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
	"unicode"
	"unicode/utf8"
)

// E-mail templates. E-mail clients are stuck in 2005, so the markup is nested
// tables with inline styles; the few <style> rules (dark mode, narrow screens)
// are an enhancement that clients which ignore them can do without.
// Every message ships as multipart/alternative with a hand-written plain-text part.

const accent = "#4f46e5"

// ReceiptItem, ReceiptKey and ReceiptFile are the lines of a receipt.
type ReceiptItem struct{ Name, Variant, Price string }
type ReceiptKey struct{ Product, Key string }
type ReceiptFile struct{ Name, Size, URL string }

type ReceiptData struct {
	StoreName    string
	AvatarURL    string // optional, absolute http(s) URL of the store's logo
	Intro        string
	CustomerName string
	OrderNumber  int64
	OrderDate    string // already formatted, e.g. "Nov 18, 2026"
	Subtotal     string // Subtotal, Discount and Tax are shown only when Discount or Tax is set
	Discount     string
	Tax          string
	Total        string
	PaidWith     string // e.g. "Visa ending in 4242"
	Items        []ReceiptItem
	Keys         []ReceiptKey
	Files        []ReceiptFile
	ReceiptURL   string
	SupportEmail string
}

type MagicLinkData struct{ StoreName, AvatarURL, Link string }

type TestData struct{ StoreName, AvatarURL, Product string }

// layout is what the shared frame needs; each message adds its own body.
type layout struct {
	Title, Preheader, StoreName, AvatarURL, Initial, Footer string
}

type view[T any] struct {
	L layout
	D T
}

func newLayout(title, preheader, store, avatar, footer string) layout {
	l := layout{Title: title, Preheader: preheader, StoreName: store, Footer: footer}
	if strings.HasPrefix(avatar, "https://") || strings.HasPrefix(avatar, "http://") {
		l.AvatarURL = avatar
	}
	r, _ := utf8.DecodeRuneInString(strings.TrimSpace(store))
	if r == utf8.RuneError || r == ' ' {
		r = '•'
	}
	l.Initial = string(unicode.ToUpper(r))
	return l
}

const frame = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light dark">
<meta name="supported-color-schemes" content="light dark">
<title>{{.L.Title}}</title>
<style>
@media (prefers-color-scheme: dark) {
  .bg { background: #0e1014 !important; }
  .card { background: #171a21 !important; border-color: #262b36 !important; }
  .tx { color: #f2f4f8 !important; }
  .mu { color: #98a1b2 !important; }
  .box { background: #11141a !important; border-color: #2b313d !important; }
  .hr { border-color: #262b36 !important; }
  .pill { background: #1f2330 !important; color: #c7cbff !important; }
  .lnk { color: #a5b4fc !important; }
}
@media only screen and (max-width: 620px) {
  .wrap { width: 100% !important; }
  .pad { padding: 26px 20px !important; }
  .h1 { font-size: 22px !important; }
  .key { font-size: 14px !important; }
}
</style>
</head>
<body class="bg" style="margin:0;padding:0;background:#f4f5f9;-webkit-text-size-adjust:100%;">
<div style="display:none;max-height:0;overflow:hidden;opacity:0;color:transparent;font-size:1px;line-height:1px;">{{.L.Preheader}}&nbsp;&zwnj;&nbsp;&zwnj;&nbsp;&zwnj;&nbsp;&zwnj;&nbsp;&zwnj;&nbsp;&zwnj;&nbsp;&zwnj;&nbsp;&zwnj;</div>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" class="bg" style="background:#f4f5f9;">
<tr><td align="center" style="padding:32px 12px 40px;">
<table role="presentation" class="wrap" width="600" cellpadding="0" cellspacing="0" border="0" style="width:600px;max-width:100%;">
  <tr><td style="padding:0 6px 18px;">
    <table role="presentation" cellpadding="0" cellspacing="0" border="0"><tr>
      <td style="padding-right:10px;vertical-align:middle;">{{if .L.AvatarURL}}<img src="{{.L.AvatarURL}}" width="36" height="36" alt="" style="display:block;width:36px;height:36px;border-radius:9px;border:0;">{{else}}<table role="presentation" cellpadding="0" cellspacing="0" border="0"><tr><td width="36" height="36" align="center" valign="middle" bgcolor="` + accent + `" style="width:36px;height:36px;border-radius:9px;background:` + accent + `;color:#ffffff;font:700 17px/36px -apple-system,'Segoe UI',Helvetica,Arial,sans-serif;">{{.L.Initial}}</td></tr></table>{{end}}</td>
      <td class="tx" style="vertical-align:middle;font:600 16px/1.3 -apple-system,'Segoe UI',Helvetica,Arial,sans-serif;color:#14161c;">{{.L.StoreName}}</td>
    </tr></table>
  </td></tr>
  <tr><td class="card" style="background:#ffffff;border:1px solid #e6e8ef;border-radius:16px;">
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0"><tr><td class="pad tx" style="padding:38px 40px 36px;font:400 15px/1.6 -apple-system,'Segoe UI',Helvetica,Arial,sans-serif;color:#14161c;">
{{block "body" .}}{{end}}
    </td></tr></table>
  </td></tr>
  <tr><td class="mu" align="center" style="padding:22px 12px 0;font:400 12px/1.6 -apple-system,'Segoe UI',Helvetica,Arial,sans-serif;color:#8a92a3;">{{.L.Footer}}</td></tr>
</table>
</td></tr>
</table>
</body>
</html>`

const button = `<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin:6px 0 0;"><tr><td align="center" bgcolor="` + accent + `" style="border-radius:10px;background:` + accent + `;"><a href="@HREF@" style="display:inline-block;padding:13px 28px;font:600 15px/1 -apple-system,'Segoe UI',Helvetica,Arial,sans-serif;color:#ffffff;text-decoration:none;border-radius:10px;">@LABEL@</a></td></tr></table>`

// btn is the call-to-action button; href may be a template action.
func btn(href, label string) string {
	return strings.NewReplacer("@HREF@", href, "@LABEL@", label).Replace(button)
}

func page(body string) *template.Template {
	t := template.Must(template.New("frame").Parse(frame))
	return template.Must(t.Parse(`{{define "body"}}` + body + `{{end}}`))
}

func exec[T any](t *template.Template, v view[T]) (string, error) {
	var b bytes.Buffer
	if err := t.Execute(&b, v); err != nil {
		return "", err
	}
	return b.String(), nil
}

// ---------------------------------------------------------------- receipt

var receiptTpl = page(`
<h1 class="h1 tx" style="margin:0 0 6px;font:700 26px/1.25 -apple-system,'Segoe UI',Helvetica,Arial,sans-serif;color:#14161c;letter-spacing:-0.01em;">Thanks for your order{{if .D.CustomerName}}, {{.D.CustomerName}}{{end}}</h1>
<p class="mu" style="margin:0 0 22px;font-size:14px;color:#6b7385;">Order #{{.D.OrderNumber}}{{if .D.OrderDate}} · {{.D.OrderDate}}{{end}}</p>
{{if .D.Intro}}<p style="margin:0 0 24px;">{{.D.Intro}}</p>{{end}}

<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" class="box" style="background:#f8f9fc;border:1px solid #e6e8ef;border-radius:12px;">
<tr><td style="padding:6px 20px;">
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
  {{range .D.Items}}<tr>
    <td style="padding:13px 12px 13px 0;border-bottom:1px solid #e6e8ef;" class="hr"><div class="tx" style="font-weight:600;color:#14161c;">{{.Name}}</div>{{if .Variant}}<div class="mu" style="font-size:13px;color:#6b7385;">{{.Variant}}</div>{{end}}</td>
    <td align="right" valign="top" class="tx hr" style="padding:13px 0;border-bottom:1px solid #e6e8ef;white-space:nowrap;color:#14161c;">{{.Price}}</td>
  </tr>{{end}}
  {{if or .D.Discount .D.Tax}}
    <tr><td class="mu" style="padding:10px 0 0;font-size:14px;color:#6b7385;">Subtotal</td><td align="right" class="mu" style="padding:10px 0 0;font-size:14px;color:#6b7385;">{{.D.Subtotal}}</td></tr>
    {{if .D.Discount}}<tr><td class="mu" style="padding:4px 0 0;font-size:14px;color:#6b7385;">Discount</td><td align="right" class="mu" style="padding:4px 0 0;font-size:14px;color:#6b7385;">−{{.D.Discount}}</td></tr>{{end}}
    {{if .D.Tax}}<tr><td class="mu" style="padding:4px 0 0;font-size:14px;color:#6b7385;">Tax</td><td align="right" class="mu" style="padding:4px 0 0;font-size:14px;color:#6b7385;">{{.D.Tax}}</td></tr>{{end}}
  {{end}}
    <tr><td class="tx" style="padding:14px 0 12px;font-weight:700;font-size:16px;color:#14161c;">Total</td><td align="right" class="tx" style="padding:14px 0 12px;font-weight:700;font-size:16px;color:#14161c;white-space:nowrap;">{{.D.Total}}</td></tr>
  </table>
</td></tr></table>
{{if .D.PaidWith}}<p class="mu" style="margin:10px 2px 0;font-size:13px;color:#6b7385;">Paid with {{.D.PaidWith}}</p>{{end}}

{{if .D.Keys}}
<h2 class="tx" style="margin:34px 0 4px;font:700 17px/1.3 -apple-system,'Segoe UI',Helvetica,Arial,sans-serif;color:#14161c;">{{if gt (len .D.Keys) 1}}Your license keys{{else}}Your license key{{end}}</h2>
<p class="mu" style="margin:0 0 14px;font-size:13px;color:#6b7385;">Keep {{if gt (len .D.Keys) 1}}them{{else}}it{{end}} private. You can always find {{if gt (len .D.Keys) 1}}them{{else}}it{{end}} again on your order page.</p>
{{range .D.Keys}}
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="margin:0 0 12px;"><tr><td class="box" style="background:#f8f9fc;border:1px solid #e6e8ef;border-radius:12px;padding:14px 18px;">
  {{if .Product}}<div class="mu" style="font-size:12px;letter-spacing:.04em;text-transform:uppercase;color:#6b7385;margin-bottom:6px;">{{.Product}}</div>{{end}}
  <div class="key tx" style="font:600 16px/1.5 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;color:#14161c;word-break:break-all;">{{.Key}}</div>
</td></tr></table>
{{end}}
{{end}}

{{if .D.Files}}
<h2 class="tx" style="margin:30px 0 12px;font:700 17px/1.3 -apple-system,'Segoe UI',Helvetica,Arial,sans-serif;color:#14161c;">Downloads</h2>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" class="box" style="background:#f8f9fc;border:1px solid #e6e8ef;border-radius:12px;"><tr><td style="padding:2px 18px;">
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
  {{range $i, $f := .D.Files}}<tr>
    <td class="hr" style="padding:12px 10px 12px 0;{{if $i}}border-top:1px solid #e6e8ef;{{end}}"><div class="tx" style="font-weight:600;color:#14161c;word-break:break-word;">{{$f.Name}}</div>{{if $f.Size}}<div class="mu" style="font-size:13px;color:#6b7385;">{{$f.Size}}</div>{{end}}</td>
    <td align="right" valign="middle" class="hr" style="padding:12px 0;{{if $i}}border-top:1px solid #e6e8ef;{{end}}white-space:nowrap;"><a class="lnk" href="{{$f.URL}}" style="color:` + accent + `;font-weight:600;text-decoration:none;">Download &rarr;</a></td>
  </tr>{{end}}
  </table>
</td></tr></table>
{{end}}

<div style="margin:32px 0 0;">` + btn("{{.D.ReceiptURL}}", "View your order") + `</div>
<p class="mu" style="margin:18px 0 0;font-size:12px;line-height:1.6;color:#8a92a3;word-break:break-all;">Button not working? Copy this link into your browser:<br>{{.D.ReceiptURL}}</p>
{{if .D.SupportEmail}}<hr class="hr" style="border:0;border-top:1px solid #e6e8ef;margin:28px 0 18px;">
<p class="mu" style="margin:0;font-size:13px;color:#6b7385;">Questions about your order? Write to <a class="lnk" href="mailto:{{.D.SupportEmail}}" style="color:` + accent + `;text-decoration:none;">{{.D.SupportEmail}}</a>.</p>{{end}}
`)

func RenderReceipt(d ReceiptData) (html, text string, err error) {
	v := view[ReceiptData]{
		L: newLayout("Your order #"+fmt.Sprint(d.OrderNumber), receiptPreheader(d), d.StoreName, d.AvatarURL, "You received this e-mail because of your order from "+d.StoreName+"."),
		D: d,
	}
	if html, err = exec(receiptTpl, v); err != nil {
		return "", "", err
	}
	return html, receiptText(d), nil
}

func receiptPreheader(d ReceiptData) string {
	switch {
	case len(d.Keys) > 0:
		return "Your license key and order details for order #" + fmt.Sprint(d.OrderNumber)
	case len(d.Files) > 0:
		return "Your downloads and order details for order #" + fmt.Sprint(d.OrderNumber)
	}
	return "Order #" + fmt.Sprint(d.OrderNumber) + " · " + d.Total
}

func receiptText(d ReceiptData) string {
	var t strings.Builder
	rule := func(title string) {
		fmt.Fprintf(&t, "\n%s\n%s\n", strings.ToUpper(title), strings.Repeat("-", len(title)))
	}
	money := func(label, value string) { fmt.Fprintf(&t, "  %-34s %12s\n", label, value) }
	fmt.Fprintf(&t, "%s\n", d.StoreName)
	fmt.Fprintf(&t, "Thanks for your order%s\n", map[bool]string{true: ", " + d.CustomerName, false: ""}[d.CustomerName != ""])
	fmt.Fprintf(&t, "Order #%d", d.OrderNumber)
	if d.OrderDate != "" {
		fmt.Fprintf(&t, " · %s", d.OrderDate)
	}
	t.WriteString("\n")
	if d.Intro != "" {
		fmt.Fprintf(&t, "\n%s\n", d.Intro)
	}
	rule("Order summary")
	for _, it := range d.Items {
		name := it.Name
		if it.Variant != "" {
			name += " — " + it.Variant
		}
		money(name, it.Price)
	}
	if d.Discount != "" || d.Tax != "" {
		money("Subtotal", d.Subtotal)
		if d.Discount != "" {
			money("Discount", "-"+d.Discount)
		}
		if d.Tax != "" {
			money("Tax", d.Tax)
		}
	}
	money("Total", d.Total)
	if d.PaidWith != "" {
		fmt.Fprintf(&t, "  Paid with %s\n", d.PaidWith)
	}
	if len(d.Keys) > 0 {
		rule("License keys")
		for _, k := range d.Keys {
			if k.Product != "" {
				fmt.Fprintf(&t, "  %s\n", k.Product)
			}
			fmt.Fprintf(&t, "  %s\n\n", k.Key)
		}
	}
	if len(d.Files) > 0 {
		rule("Downloads")
		for _, f := range d.Files {
			fmt.Fprintf(&t, "  %s", f.Name)
			if f.Size != "" {
				fmt.Fprintf(&t, " (%s)", f.Size)
			}
			t.WriteString("\n")
		}
		fmt.Fprintf(&t, "  Download them from your order page.\n")
	}
	fmt.Fprintf(&t, "\nView your order:\n%s\n", d.ReceiptURL)
	if d.SupportEmail != "" {
		fmt.Fprintf(&t, "\nQuestions? Write to %s.\n", d.SupportEmail)
	}
	return t.String()
}

// ---------------------------------------------------------------- magic link

var magicTpl = page(`
<h1 class="h1 tx" style="margin:0 0 10px;font:700 26px/1.25 -apple-system,'Segoe UI',Helvetica,Arial,sans-serif;color:#14161c;letter-spacing:-0.01em;">Sign in to your orders</h1>
<p style="margin:0 0 26px;">Use the button below to sign in to {{.D.StoreName}} and see your orders, downloads and license keys. The link works once and expires in 15 minutes.</p>
` + btn("{{.D.Link}}", "Sign in") + `
<p class="mu" style="margin:22px 0 0;font-size:12px;line-height:1.6;color:#8a92a3;word-break:break-all;">Button not working? Copy this link into your browser:<br>{{.D.Link}}</p>
<hr class="hr" style="border:0;border-top:1px solid #e6e8ef;margin:28px 0 18px;">
<p class="mu" style="margin:0;font-size:13px;color:#6b7385;">If you did not ask for this e-mail, you can safely ignore it. Nobody can sign in without the link.</p>
`)

func RenderMagicLink(d MagicLinkData) (html, text string, err error) {
	v := view[MagicLinkData]{L: newLayout("Sign in to "+d.StoreName, "Your sign-in link for "+d.StoreName+" (valid for 15 minutes)", d.StoreName, d.AvatarURL, "Sent by "+d.StoreName+"."), D: d}
	if html, err = exec(magicTpl, v); err != nil {
		return "", "", err
	}
	text = fmt.Sprintf("%s\n\nSign in to see your orders, downloads and license keys:\n%s\n\nThe link works once and expires in 15 minutes.\nIf you did not ask for it, you can ignore this e-mail.\n", d.StoreName, d.Link)
	return html, text, nil
}

// ---------------------------------------------------------------- test message

var testTpl = page(`
<h1 class="h1 tx" style="margin:0 0 10px;font:700 26px/1.25 -apple-system,'Segoe UI',Helvetica,Arial,sans-serif;color:#14161c;letter-spacing:-0.01em;">Your e-mail settings work</h1>
<p style="margin:0 0 18px;">This is a test message from {{.D.Product}}. If you can read it, your SMTP server accepted the message and your customers will get their receipts and sign-in links.</p>
<table role="presentation" cellpadding="0" cellspacing="0" border="0"><tr><td class="pill" style="background:#eef0ff;color:#3730a3;border-radius:999px;padding:6px 14px;font-size:13px;font-weight:600;">&#10003; Delivered</td></tr></table>
`)

func RenderTest(d TestData) (html, text string, err error) {
	v := view[TestData]{L: newLayout("Test e-mail", "Your e-mail settings work", d.StoreName, d.AvatarURL, "Sent from "+d.Product+"."), D: d}
	if html, err = exec(testTpl, v); err != nil {
		return "", "", err
	}
	return html, fmt.Sprintf("%s\n\nYour e-mail settings work.\nThis is a test message from %s: your SMTP server accepted it, so receipts and sign-in links will be delivered.\n", d.StoreName, d.Product), nil
}
