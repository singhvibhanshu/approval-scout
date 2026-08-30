package main

import (
	"crypto/tls"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// sendEmail delivers a multipart/alternative (plain text + HTML) message via
// SMTP. Port 465 uses implicit TLS; any other port (e.g. the usual 587) uses
// STARTTLS when the server advertises it. This works with Gmail app passwords
// and essentially any standard SMTP provider.
func sendEmail(cfg *Config, subject, textBody, htmlBody string) error {
	msg := buildMIME(cfg.EmailFrom, cfg.EmailTo, subject, textBody, htmlBody)
	addr := net.JoinHostPort(cfg.SMTPHost, fmt.Sprintf("%d", cfg.SMTPPort))

	client, err := dialSMTP(addr, cfg.SMTPHost, cfg.SMTPPort)
	if err != nil {
		return err
	}
	defer client.Close()

	if cfg.SMTPUsername != "" {
		if ok, _ := client.Extension("AUTH"); ok {
			auth := smtp.PlainAuth("", cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPHost)
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("smtp auth: %w", err)
			}
		}
	}

	if err := client.Mail(cfg.EmailFrom); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}
	for _, to := range cfg.EmailTo {
		if err := client.Rcpt(to); err != nil {
			return fmt.Errorf("smtp RCPT %s: %w", to, err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("writing message: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("closing message: %w", err)
	}
	return client.Quit()
}

func dialSMTP(addr, host string, port int) (*smtp.Client, error) {
	if port == 465 {
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 20 * time.Second}, "tcp", addr, &tls.Config{ServerName: host})
		if err != nil {
			return nil, fmt.Errorf("tls dial %s: %w", addr, err)
		}
		client, err := smtp.NewClient(conn, host)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("smtp client: %w", err)
		}
		return client, nil
	}

	conn, err := net.DialTimeout("tcp", addr, 20*time.Second)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("smtp client: %w", err)
	}
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: host}); err != nil {
			client.Close()
			return nil, fmt.Errorf("starttls: %w", err)
		}
	}
	return client, nil
}

func buildMIME(from string, to []string, subject, textBody, htmlBody string) string {
	boundary := fmt.Sprintf("alt-%d", time.Now().UnixNano())
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(to, ", "))
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subject))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary)

	fmt.Fprintf(&b, "--%s\r\n", boundary)
	b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(textBody)
	b.WriteString("\r\n")

	fmt.Fprintf(&b, "--%s\r\n", boundary)
	b.WriteString("Content-Type: text/html; charset=\"utf-8\"\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(htmlBody)
	b.WriteString("\r\n")

	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	return b.String()
}
