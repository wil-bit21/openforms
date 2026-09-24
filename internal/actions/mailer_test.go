package actions_test

import (
	"context"
	"net/smtp"
	"strings"
	"testing"
	"time"

	"github.com/openforms/openforms/internal/actions"
	"github.com/openforms/openforms/internal/config"
)

func TestNewMailer(t *testing.T) {
	if _, ok := actions.NewMailer(config.Config{}).(actions.LogMailer); !ok {
		t.Fatal("empty SMTPHost must give LogMailer")
	}
	m, ok := actions.NewMailer(config.Config{SMTPHost: "mail.local", SMTPPort: 1025, SMTPFrom: "of@x.test"}).(*actions.SMTPMailer)
	if !ok || m.Addr != "mail.local:1025" || m.From != "of@x.test" {
		t.Fatalf("SMTP mailer = %#v", m)
	}
	if err := (actions.LogMailer{}).Send(context.Background(), "a@b.c", "s", "b"); err != nil {
		t.Fatal(err)
	}
}

func TestBuildMessage(t *testing.T) {
	date := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	msg, err := actions.BuildMessage("of@x.test", "ada@example.com", "Hello Ada", "line1\nline2", date)
	if err != nil {
		t.Fatal(err)
	}
	s := string(msg)
	for _, want := range []string{
		"From: of@x.test\r\n",
		"To: ada@example.com\r\n",
		"Subject: Hello Ada\r\n",
		"Date: Wed, 02 Jan 2030 03:04:05 +0000\r\n",
		"MIME-Version: 1.0\r\n",
		"Content-Type: text/plain; charset=utf-8\r\n",
		"\r\n\r\nline1\r\nline2",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("message missing %q:\n%s", want, s)
		}
	}
}

func TestBuildMessage_NoHeaderInjection(t *testing.T) {
	msg, err := actions.BuildMessage("of@x.test", "ada@example.com", "Hi\r\nBcc: evil@x.test", "body", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(msg), "\r\n") {
		if strings.HasPrefix(line, "Bcc:") {
			t.Fatalf("header injected:\n%s", msg)
		}
	}
	if _, err := actions.BuildMessage("of@x.test", "ada@example.com\r\nBcc: evil@x.test", "s", "b", time.Now()); err == nil {
		t.Fatal("recipient with CRLF must be rejected")
	}
	if _, err := actions.BuildMessage("of@x.test", "not an address", "s", "b", time.Now()); err == nil {
		t.Fatal("invalid recipient must be rejected")
	}
}

func TestBuildMessage_EncodesNonASCIISubject(t *testing.T) {
	msg, err := actions.BuildMessage("of@x.test", "ada@example.com", "Grüße", "b", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(msg), "Subject: =?utf-8?q?Gr=C3=BC=C3=9Fe?=\r\n") {
		t.Fatalf("subject not encoded:\n%s", msg)
	}
}

func TestSMTPMailer_Send(t *testing.T) {
	var gotAddr, gotFrom string
	var gotTo []string
	var gotMsg []byte
	var gotAuth smtp.Auth
	m := &actions.SMTPMailer{
		Addr: "mail.local:1025", Host: "mail.local", From: "of@x.test",
		Username: "u", Password: "p",
		SendFunc: func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
			gotAddr, gotAuth, gotFrom, gotTo, gotMsg = addr, a, from, to, msg
			return nil
		},
	}
	if err := m.Send(context.Background(), "ada@example.com", "Subj", "Body"); err != nil {
		t.Fatal(err)
	}
	if gotAddr != "mail.local:1025" || gotFrom != "of@x.test" || len(gotTo) != 1 || gotTo[0] != "ada@example.com" {
		t.Fatalf("send args: %s %s %v", gotAddr, gotFrom, gotTo)
	}
	if gotAuth == nil || !strings.Contains(string(gotMsg), "Subject: Subj") {
		t.Fatalf("auth=%v msg=%s", gotAuth, gotMsg)
	}

	m.Username = ""
	m.Send(context.Background(), "ada@example.com", "s", "b")
	if gotAuth != nil {
		t.Fatal("auth must be nil when no username configured")
	}
}
