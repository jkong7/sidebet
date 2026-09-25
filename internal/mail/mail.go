package mail

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"time"
)

type Sender interface {
	Send(ctx context.Context, to, subject, body string) error
}

type Log struct{ Logger *slog.Logger }

func (l Log) Send(_ context.Context, to, subject, body string) error {
	l.Logger.Info("email (not sent, no SMTP configured)", "to", to, "subject", subject, "body", body)
	return nil
}

type SMTP struct {
	Host string
	Port string
	User string
	Pass string
	From string
}

func (s SMTP) Send(ctx context.Context, to, subject, body string) error {
	if strings.ContainsAny(to+subject, "\r\n") {
		return fmt.Errorf("mail: header injection")
	}
	msg := strings.Join([]string{
		"From: " + s.From,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"Date: " + time.Now().UTC().Format(time.RFC1123Z),
		"",
		body,
	}, "\r\n")
	done := make(chan error, 1)
	go func() {
		auth := smtp.PlainAuth("", s.User, s.Pass, s.Host)
		done <- smtp.SendMail(net.JoinHostPort(s.Host, s.Port), auth, s.From, []string{to}, []byte(msg))
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func CodeEmail(campus, code string) (string, string) {
	subject := fmt.Sprintf("%s is your sidebet code", code)
	body := fmt.Sprintf("Your code to join %s on sidebet is %s.\n\nIt expires in 10 minutes. If you didn't ask for this, ignore this email.\n", campus, code)
	return subject, body
}
