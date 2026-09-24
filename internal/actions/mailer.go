package actions

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/openforms/openforms/internal/config"
)

type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}

// NewMailer returns an SMTP mailer when SMTPHost is configured, otherwise a
// mailer that only logs (useful for local development).
func NewMailer(cfg config.Config) Mailer {
	if cfg.SMTPHost == "" {
		return LogMailer{}
	}
	return &SMTPMailer{
		Addr:     net.JoinHostPort(cfg.SMTPHost, strconv.Itoa(cfg.SMTPPort)),
		Host:     cfg.SMTPHost,
		Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword,
		From:     cfg.SMTPFrom,
	}
}

type LogMailer struct{}

func (LogMailer) Send(ctx context.Context, to, subject, body string) error {
	slog.InfoContext(ctx, "email (log mailer)", "to", to, "subject", subject, "body", body)
	return nil
}

type SMTPMailer struct {
	Addr, Host, Username, Password, From string
	// SendFunc defaults to smtp.SendMail; tests replace it.
	SendFunc func(addr string, a smtp.Auth, from string, to []string, msg []byte) error
}

func (m *SMTPMailer) Send(ctx context.Context, to, subject, body string) error {
	msg, err := BuildMessage(m.From, to, subject, body, time.Now())
	if err != nil {
		return err
	}
	var auth smtp.Auth
	if m.Username != "" {
		auth = smtp.PlainAuth("", m.Username, m.Password, m.Host)
	}
	send := m.SendFunc
	if send == nil {
		send = smtp.SendMail
	}
	if err := send(m.Addr, auth, m.From, []string{to}, msg); err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}
	return nil
}

// BuildMessage renders a plain-text RFC 5322 message. It rejects addresses
// containing CR/LF and strips CR/LF from the subject to prevent header injection.
func BuildMessage(from, to, subject, body string, date time.Time) ([]byte, error) {
	for _, a := range []string{from, to} {
		if strings.ContainsAny(a, "\r\n") {
			return nil, errors.New("email address contains a line break")
		}
	}
	if _, err := mail.ParseAddress(to); err != nil {
		return nil, fmt.Errorf("invalid recipient %q: %w", to, err)
	}
	subject = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(subject)
	body = strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", "\r\n")

	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subject))
	fmt.Fprintf(&b, "Date: %s\r\n", date.Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return b.Bytes(), nil
}
