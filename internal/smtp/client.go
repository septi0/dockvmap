package smtp

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"time"
)

const sendTimeout = 15 * time.Second

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

	boundary := fmt.Sprintf("dockvmap-%d", time.Now().UnixNano())

	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", boundary)

	fmt.Fprintf(&b, "--%s\r\n", boundary)
	b.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n\r\n")
	b.WriteString(textBody)
	b.WriteString("\r\n\r\n")

	fmt.Fprintf(&b, "--%s\r\n", boundary)
	b.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n\r\n")
	b.WriteString(htmlBody)
	b.WriteString("\r\n\r\n")

	fmt.Fprintf(&b, "--%s--\r\n", boundary)

	return []byte(b.String())
}
