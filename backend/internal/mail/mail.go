// Package mail sends transactional email (receipts, magic links) over SMTP
// and records every attempt in email_logs.
package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"html/template"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"fulfillment/internal/crypt"
	"fulfillment/internal/db"
	"fulfillment/internal/settings"
)

type Message struct {
	To       string
	Subject  string
	HTML     string
	Text     string
	StoreID  int64
	OrderID  *int64
	Template string
	FromName string
}

type Mailer struct {
	DB       *gorm.DB
	Settings *settings.Service
	// Transport overrides SMTP delivery (tests).
	Transport func(from string, to []string, raw []byte) error
}

func New(g *gorm.DB, s *settings.Service) *Mailer { return &Mailer{DB: g, Settings: s} }

func (m *Mailer) Configured() bool {
	return m.Transport != nil || (m.Settings.Get(settings.SMTPHost) != "" && m.Settings.Get(settings.SMTPFrom) != "")
}

// Send delivers a message. When SMTP is not configured the message is logged
// as "skipped" and nil is returned so fulfilment never fails because of email.
func (m *Mailer) Send(ctx context.Context, msg Message) error {
	log := db.EmailLog{StoreID: msg.StoreID, OrderID: msg.OrderID, To: msg.To, Subject: msg.Subject, Template: msg.Template, CreatedAt: db.Now()}
	if !m.Configured() {
		log.Status, log.Error = "skipped", "SMTP is not configured"
		m.DB.Create(&log)
		return nil
	}
	err := m.send(msg)
	if err != nil {
		log.Status, log.Error = "failed", err.Error()
	} else {
		log.Status = "sent"
	}
	m.DB.Create(&log)
	return err
}

func (m *Mailer) send(msg Message) error {
	from := m.Settings.Get(settings.SMTPFrom)
	addr, err := mail.ParseAddress(from)
	if err != nil {
		return fmt.Errorf("invalid SMTP_FROM %q: %w", from, err)
	}
	if msg.FromName != "" {
		addr.Name = msg.FromName
	}
	if _, err := mail.ParseAddress(msg.To); err != nil {
		return fmt.Errorf("invalid recipient %q", msg.To)
	}
	raw := buildMIME(addr.String(), msg)
	if m.Transport != nil {
		return m.Transport(addr.Address, []string{msg.To}, raw)
	}
	host := m.Settings.Get(settings.SMTPHost)
	port := m.Settings.Int(settings.SMTPPort, 587)
	user, pass := m.Settings.Get(settings.SMTPUser), m.Settings.Get(settings.SMTPPassword)
	return smtpSend(host, port, m.Settings.Get(settings.SMTPSecurity), user, pass, addr.Address, msg.To, raw)
}

func smtpSend(host string, port int, security, user, pass, from, to string, raw []byte) error {
	target := net.JoinHostPort(host, strconv.Itoa(port))
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	var c *smtp.Client
	switch security {
	case "tls":
		conn, err := tls.DialWithDialer(dialer, "tcp", target, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err != nil {
			return err
		}
		cl, err := smtp.NewClient(conn, host)
		if err != nil {
			return err
		}
		c = cl
	default:
		conn, err := dialer.Dial("tcp", target)
		if err != nil {
			return err
		}
		cl, err := smtp.NewClient(conn, host)
		if err != nil {
			return err
		}
		c = cl
		if security != "none" {
			if ok, _ := c.Extension("STARTTLS"); !ok {
				c.Close()
				return errors.New("smtp server does not support STARTTLS (set SMTP_SECURITY=none to disable)")
			}
			if err := c.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
				c.Close()
				return err
			}
		}
	}
	defer c.Close()
	if user != "" {
		if ok, _ := c.Extension("AUTH"); ok {
			if err := c.Auth(smtp.PlainAuth("", user, pass, host)); err != nil {
				return err
			}
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(raw); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func buildMIME(from string, msg Message) []byte {
	boundary := "df-" + crypt.RandomHex(12)
	var b bytes.Buffer
	h := func(k, v string) { fmt.Fprintf(&b, "%s: %s\r\n", k, v) }
	h("From", from)
	h("To", msg.To)
	h("Subject", mimeHeader(msg.Subject))
	h("Date", time.Now().UTC().Format(time.RFC1123Z))
	h("Message-ID", fmt.Sprintf("<%s@fulfillment>", crypt.RandomHex(12)))
	h("MIME-Version", "1.0")
	h("Content-Type", fmt.Sprintf(`multipart/alternative; boundary="%s"`, boundary))
	b.WriteString("\r\n")
	part := func(ct, body string) {
		fmt.Fprintf(&b, "--%s\r\nContent-Type: %s; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n", boundary, ct, strings.ReplaceAll(body, "\n", "\r\n"))
	}
	if msg.Text != "" {
		part("text/plain", msg.Text)
	}
	part("text/html", msg.HTML)
	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	return b.Bytes()
}

func mimeHeader(s string) string {
	for _, r := range s {
		if r > 127 {
			return mime.QEncoding.Encode("utf-8", s)
		}
	}
	return s
}

// ---------------------------------------------------------------- templates

type ReceiptItem struct{ Name, Variant, Price string }
type ReceiptKey struct{ Product, Key string }
type ReceiptFile struct{ Name, Size, URL string }

type ReceiptData struct {
	StoreName    string
	Intro        string
	CustomerName string
	OrderNumber  int64
	Total        string
	Items        []ReceiptItem
	Keys         []ReceiptKey
	Files        []ReceiptFile
	ReceiptURL   string
	SupportEmail string
}

var receiptTpl = template.Must(template.New("receipt").Parse(`<!doctype html><html><body style="margin:0;background:#f5f5f7;font-family:-apple-system,Segoe UI,Helvetica,Arial,sans-serif;color:#1d1d1f">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0"><tr><td align="center" style="padding:32px 16px">
<table role="presentation" width="560" style="max-width:560px;background:#fff;border-radius:12px;padding:32px">
<tr><td>
<h2 style="margin:0 0 8px;font-size:20px">{{.StoreName}}</h2>
<p style="margin:0 0 20px;color:#6e6e73">Order #{{.OrderNumber}}{{if .CustomerName}} · {{.CustomerName}}{{end}}</p>
{{if .Intro}}<p style="line-height:1.5">{{.Intro}}</p>{{end}}
<table role="presentation" width="100%" style="border-top:1px solid #e5e5ea;border-bottom:1px solid #e5e5ea;margin:16px 0">
{{range .Items}}<tr><td style="padding:10px 0">{{.Name}}{{if .Variant}} <span style="color:#6e6e73">— {{.Variant}}</span>{{end}}</td><td align="right" style="padding:10px 0">{{.Price}}</td></tr>{{end}}
<tr><td style="padding:10px 0;font-weight:600">Total</td><td align="right" style="padding:10px 0;font-weight:600">{{.Total}}</td></tr>
</table>
{{if .Keys}}<h3 style="font-size:15px;margin:24px 0 8px">License keys</h3>
{{range .Keys}}<p style="margin:0 0 4px;color:#6e6e73;font-size:13px">{{.Product}}</p><p style="margin:0 0 12px;font-family:ui-monospace,Menlo,monospace;background:#f5f5f7;padding:10px 12px;border-radius:8px;word-break:break-all">{{.Key}}</p>{{end}}{{end}}
{{if .Files}}<h3 style="font-size:15px;margin:24px 0 8px">Downloads</h3>
{{range .Files}}<p style="margin:0 0 6px"><a href="{{.URL}}" style="color:#0071e3">{{.Name}}</a> <span style="color:#6e6e73">{{.Size}}</span></p>{{end}}{{end}}
<p style="margin:28px 0 0"><a href="{{.ReceiptURL}}" style="display:inline-block;background:#1d1d1f;color:#fff;text-decoration:none;padding:10px 18px;border-radius:8px">View your order</a></p>
{{if .SupportEmail}}<p style="color:#6e6e73;font-size:13px;margin-top:24px">Questions? Reply to {{.SupportEmail}}.</p>{{end}}
</td></tr></table></td></tr></table></body></html>`))

func RenderReceipt(d ReceiptData) (html, text string, err error) {
	var b bytes.Buffer
	if err = receiptTpl.Execute(&b, d); err != nil {
		return
	}
	html = b.String()
	var t strings.Builder
	fmt.Fprintf(&t, "%s — order #%d\n\n", d.StoreName, d.OrderNumber)
	for _, it := range d.Items {
		fmt.Fprintf(&t, "%s %s  %s\n", it.Name, it.Variant, it.Price)
	}
	fmt.Fprintf(&t, "Total: %s\n", d.Total)
	for _, k := range d.Keys {
		fmt.Fprintf(&t, "\nLicense key (%s): %s\n", k.Product, k.Key)
	}
	for _, f := range d.Files {
		fmt.Fprintf(&t, "\nDownload %s: %s\n", f.Name, f.URL)
	}
	fmt.Fprintf(&t, "\nView your order: %s\n", d.ReceiptURL)
	text = t.String()
	return
}

type MagicLinkData struct{ StoreName, Link string }

var magicTpl = template.Must(template.New("magic").Parse(`<!doctype html><html><body style="font-family:-apple-system,Segoe UI,Helvetica,Arial,sans-serif;color:#1d1d1f;padding:24px">
<h2 style="margin:0 0 12px">{{.StoreName}}</h2>
<p>Use the button below to sign in and view your orders, downloads and license keys. The link expires in 15 minutes and can be used once.</p>
<p><a href="{{.Link}}" style="display:inline-block;background:#1d1d1f;color:#fff;text-decoration:none;padding:10px 18px;border-radius:8px">Sign in</a></p>
<p style="color:#6e6e73;font-size:13px">If you did not request this, you can ignore this email.</p></body></html>`))

func RenderMagicLink(d MagicLinkData) (html, text string, err error) {
	var b bytes.Buffer
	if err = magicTpl.Execute(&b, d); err != nil {
		return
	}
	return b.String(), fmt.Sprintf("Sign in to %s: %s\n(expires in 15 minutes)\n", d.StoreName, d.Link), nil
}
