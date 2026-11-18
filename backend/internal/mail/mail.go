// Package mail sends transactional email (receipts, magic links) over SMTP
// and records every attempt in email_logs.
package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
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
