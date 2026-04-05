package mailing

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"skipjd/internal/model"
)

type Mailer interface {
	SendDigest(ctx context.Context, runAt time.Time, postings []model.JobPosting) error
}

type SMTPConfig struct {
	Host string
	Port int
	User string
	Pass string
	From string
	To   string
}

const (
	smtpDialTimeout = 10 * time.Second
	smtpIOTimeout   = 15 * time.Second
)

type sendMailFunc func(ctx context.Context, config SMTPConfig, msg []byte) error

type SMTPMailer struct {
	config   SMTPConfig
	sendMail sendMailFunc
}

func NewSMTPMailer(config SMTPConfig) *SMTPMailer {
	return &SMTPMailer{
		config:   config,
		sendMail: sendSMTPMail,
	}
}

func (m *SMTPMailer) SendDigest(ctx context.Context, runAt time.Time, postings []model.JobPosting) error {
	if len(postings) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	subject := fmt.Sprintf("SkipJD Digest %s (%d new postings)", runAt.Format(time.RFC3339), len(postings))
	body := buildDigestBody(postings)

	message := strings.Join([]string{
		fmt.Sprintf("From: %s", m.config.From),
		fmt.Sprintf("To: %s", m.config.To),
		fmt.Sprintf("Subject: %s", subject),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")

	return m.sendMail(ctx, m.config, []byte(message))
}

func buildDigestBody(postings []model.JobPosting) string {
	var b strings.Builder
	for i, posting := range postings {
		_, _ = fmt.Fprintf(&b, "[%d]\n", i+1)
		_, _ = fmt.Fprintf(&b, "제목: %s\n", posting.Title)
		_, _ = fmt.Fprintf(&b, "회사: %s\n", posting.Company)
		_, _ = fmt.Fprintf(&b, "마감일: %s\n", posting.ClosingDate)
		_, _ = fmt.Fprintf(&b, "링크: %s\n", posting.URL)

		if i < len(postings)-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

func sendSMTPMail(ctx context.Context, config SMTPConfig, msg []byte) error {
	addr := net.JoinHostPort(config.Host, strconv.Itoa(config.Port))
	dialer := net.Dialer{Timeout: smtpDialTimeout}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("dial smtp server %s: %w", addr, err)
	}
	defer conn.Close()

	stop := closeConnOnContextDone(ctx, conn)
	defer stop()

	if err := setSMTPDeadline(conn, ctx, smtpIOTimeout); err != nil {
		return fmt.Errorf("set smtp deadline: %w", err)
	}

	client, err := smtp.NewClient(conn, config.Host)
	if err != nil {
		return fmt.Errorf("create smtp client: %w", err)
	}
	defer client.Close()

	ok, _ := client.Extension("STARTTLS")
	if !ok {
		return fmt.Errorf("smtp server does not support STARTTLS")
	}
	if err := client.StartTLS(&tls.Config{ServerName: config.Host, MinVersion: tls.VersionTLS12}); err != nil {
		return fmt.Errorf("starttls: %w", err)
	}

	auth := smtp.PlainAuth("", config.User, config.Pass, config.Host)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("authenticate smtp user: %w", err)
	}

	if err := client.Mail(config.From); err != nil {
		return fmt.Errorf("set sender: %w", err)
	}
	if err := client.Rcpt(config.To); err != nil {
		return fmt.Errorf("set recipient: %w", err)
	}

	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("open smtp data writer: %w", err)
	}
	if _, err := wc.Write(msg); err != nil {
		_ = wc.Close()
		return fmt.Errorf("write smtp message: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("close smtp data writer: %w", err)
	}

	if err := client.Quit(); err != nil {
		return fmt.Errorf("close smtp session: %w", err)
	}

	return nil
}

func setSMTPDeadline(conn net.Conn, ctx context.Context, fallback time.Duration) error {
	deadline := time.Now().Add(fallback)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	return conn.SetDeadline(deadline)
}

func closeConnOnContextDone(ctx context.Context, conn net.Conn) func() {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	return func() { close(done) }
}
