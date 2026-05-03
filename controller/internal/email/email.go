// controller/internal/email/email.go — SMTP email alerts
//
// Supports three connection modes selected automatically by port:
//   Port 465          → implicit TLS  (most external providers support this)
//   Port 587 / other  → STARTTLS if the server advertises it, plain otherwise
//   No username set   → skip AUTH entirely (Mailpit, local Postfix relay, etc.)

package email

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/ding/ding/internal/diff"
)

// Config holds one user's SMTP settings.
type Config struct {
	Host     string // e.g. "smtp.gmail.com" or "localhost"
	Port     int    // 587 (STARTTLS), 465 (implicit TLS), 1025 (Mailpit)
	Username string // empty = no authentication
	Password string
	From     string // From address, e.g. "Ding <ding@home.local>"
	To       string // alert recipient
}

// Send formats changes as a plain-text email and delivers it via SMTP.
func Send(cfg Config, changes []diff.Change) error {
	if len(changes) == 0 {
		return nil
	}
	if cfg.Host == "" || cfg.To == "" {
		return fmt.Errorf("email: host and To address are required")
	}

	subject, body := buildContent(changes)
	msg := buildMessage(cfg.From, cfg.To, subject, body)
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))

	if cfg.Port == 465 {
		return sendImplicitTLS(cfg, addr, msg)
	}
	return sendSTARTTLS(cfg, addr, msg)
}

// SendTest delivers a single test message to verify the SMTP config works.
func SendTest(cfg Config) error {
	if cfg.Host == "" || cfg.To == "" {
		return fmt.Errorf("email: host and To address are required")
	}
	subject := "Ding — test message"
	body := "Your email alert configuration is working correctly.\n\n— Ding"
	msg := buildMessage(cfg.From, cfg.To, subject, body)
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	if cfg.Port == 465 {
		return sendImplicitTLS(cfg, addr, msg)
	}
	return sendSTARTTLS(cfg, addr, msg)
}

// sendSTARTTLS dials plain TCP, upgrades to TLS via STARTTLS if available,
// then authenticates and delivers the message.
// When no username is set (e.g. Mailpit), the AUTH step is skipped entirely.
func sendSTARTTLS(cfg Config, addr, msg string) error {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("smtp dial %s: %w", addr, err)
	}
	c, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp client %s: %w", addr, err)
	}
	defer c.Close()

	if ok, _ := c.Extension("STARTTLS"); ok {
		tlsCfg := &tls.Config{ServerName: cfg.Host}
		if err = c.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("smtp STARTTLS: %w", err)
		}
	}

	if cfg.Username != "" {
		auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
		if err = c.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	return deliver(c, cfg.From, cfg.To, msg)
}

// sendImplicitTLS connects directly over TLS (port 465).
func sendImplicitTLS(cfg Config, addr, msg string) error {
	tlsCfg := &tls.Config{ServerName: cfg.Host}
	dialer := &tls.Dialer{Config: tlsCfg, NetDialer: &net.Dialer{Timeout: 10 * time.Second}}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("smtp tls dial %s: %w", addr, err)
	}

	c, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer c.Close()

	if cfg.Username != "" {
		auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
		if err = c.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	return deliver(c, cfg.From, cfg.To, msg)
}

// deliver sends MAIL FROM / RCPT TO / DATA to an already-connected client.
func deliver(c *smtp.Client, from, to, msg string) error {
	if from == "" {
		from = "ding@localhost"
	}
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("smtp RCPT TO: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	if _, err = fmt.Fprint(w, msg); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err = w.Close(); err != nil {
		return fmt.Errorf("smtp close data: %w", err)
	}
	return c.Quit()
}

// buildContent returns the subject line and plain-text body for a change alert.
func buildContent(changes []diff.Change) (subject, body string) {
	n := len(changes)
	if n == 1 {
		subject = fmt.Sprintf("Ding — 1 network change")
	} else {
		subject = fmt.Sprintf("Ding — %d network changes", n)
	}

	var sb strings.Builder
	sb.WriteString("Network changes detected on your LAN:\n\n")
	for _, c := range changes {
		sb.WriteString("  • ")
		sb.WriteString(c.String())
		sb.WriteByte('\n')
	}
	sb.WriteString(fmt.Sprintf("\n— Ding at %s UTC", time.Now().UTC().Format("2006-01-02 15:04")))
	return subject, sb.String()
}

// buildMessage assembles a minimal RFC 2822 email message.
func buildMessage(from, to, subject, body string) string {
	// Ensure From has a display name so it doesn't land in spam
	if from == "" {
		from = "Ding <ding@localhost>"
	}
	var sb strings.Builder
	sb.WriteString("From: " + from + "\r\n")
	sb.WriteString("To: " + to + "\r\n")
	sb.WriteString("Subject: " + subject + "\r\n")
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(strings.ReplaceAll(body, "\n", "\r\n"))
	return sb.String()
}
