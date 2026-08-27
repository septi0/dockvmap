package smtp

import (
	"context"
	"crypto/tls"
	_ "embed"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"time"
)

const sendTimeout = 15 * time.Second

const logoContentID = "dockvmap-logo"

//go:embed assets/dockvmap-logo.png
var logoPNG []byte

type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	TLS      bool
}

type Client struct {
	cfg Config
}

func NewClient(cfg Config) *Client {
	return &Client{cfg: cfg}
}

func (c *Client) Send(ctx context.Context, to []string, subject, textBody, htmlBody string) error {
	if len(to) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, sendTimeout)

	defer cancel()

	addr := fmt.Sprintf("%s:%d", c.cfg.Host, c.cfg.Port)

	dialer := &net.Dialer{}

	conn, err := dialer.DialContext(ctx, "tcp", addr)

	if err != nil {
		return fmt.Errorf("dialing smtp server %s: %w", addr, err)
	}

	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}

	client, err := smtp.NewClient(conn, c.cfg.Host)

	if err != nil {
		return fmt.Errorf("initializing smtp client: %w", err)
	}

	defer client.Close()

	if c.cfg.TLS {
		if err := client.StartTLS(&tls.Config{ServerName: c.cfg.Host}); err != nil {
			return fmt.Errorf("starting tls: %w", err)
		}
	}

	if c.cfg.Username != "" {
		auth := smtp.PlainAuth("", c.cfg.Username, c.cfg.Password, c.cfg.Host)

		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("authenticating: %w", err)
		}
	}

	if err := client.Mail(c.cfg.From); err != nil {
		return fmt.Errorf("setting sender: %w", err)
	}

	accepted := 0

	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			slog.Warn("smtp recipient rejected, skipping", "recipient", recipient, "error", err)
			continue
		}

		accepted++
	}

	if accepted == 0 {
		return fmt.Errorf("all %d recipient(s) rejected", len(to))
	}

	wc, err := client.Data()

	if err != nil {
		return fmt.Errorf("opening message writer: %w", err)
	}

	if _, err := wc.Write(buildMessage(c.cfg.From, subject, textBody, htmlBody)); err != nil {
		wc.Close()

		return fmt.Errorf("writing message: %w", err)
	}

	if err := wc.Close(); err != nil {
		return fmt.Errorf("closing message writer: %w", err)
	}

	return client.Quit()
}

func buildMessage(from, subject, textBody, htmlBody string) []byte {
	var b strings.Builder

	fmt.Fprintf(&b, "From: %s\r\n", from)
	b.WriteString("To: undisclosed-recipients:;\r\n")
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")

	if htmlBody == "" {
		b.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n\r\n")
		b.WriteString(textBody)

		return []byte(b.String())
	}

	now := time.Now().UnixNano()
	related := fmt.Sprintf("dockvmap-rel-%d", now)
	alt := fmt.Sprintf("dockvmap-alt-%d", now)

	fmt.Fprintf(&b, "Content-Type: multipart/related; boundary=\"%s\"; type=\"multipart/alternative\"\r\n\r\n", related)

	fmt.Fprintf(&b, "--%s\r\n", related)
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", alt)

	fmt.Fprintf(&b, "--%s\r\n", alt)
	b.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n\r\n")
	b.WriteString(textBody)
	b.WriteString("\r\n\r\n")

	fmt.Fprintf(&b, "--%s\r\n", alt)
	b.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n\r\n")
	b.WriteString(htmlBody)
	b.WriteString("\r\n\r\n")

	fmt.Fprintf(&b, "--%s--\r\n\r\n", alt)

	if len(logoPNG) > 0 {
		fmt.Fprintf(&b, "--%s\r\n", related)
		b.WriteString("Content-Type: image/png\r\n")
		b.WriteString("Content-Transfer-Encoding: base64\r\n")
		fmt.Fprintf(&b, "Content-ID: <%s>\r\n", logoContentID)
		b.WriteString("Content-Disposition: inline; filename=\"dockvmap-logo.png\"\r\n\r\n")
		b.WriteString(base64Lines(logoPNG))
		b.WriteString("\r\n")
	}

	fmt.Fprintf(&b, "--%s--\r\n", related)

	return []byte(b.String())
}

func base64Lines(data []byte) string {
	const lineLen = 76

	encoded := base64.StdEncoding.EncodeToString(data)

	var b strings.Builder

	for len(encoded) > lineLen {
		b.WriteString(encoded[:lineLen])
		b.WriteString("\r\n")
		encoded = encoded[lineLen:]
	}

	b.WriteString(encoded)

	return b.String()
}
