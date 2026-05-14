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

	"skipjd/internal/matcher"
	"skipjd/internal/model"
)

// ScoredPosting bundles a candidate JobPosting with its match score so the
// mailer can render both the ranking and the per-category match reasoning.
type ScoredPosting struct {
	Posting model.JobPosting
	Score   matcher.Score
}

// Section is one labeled group of postings inside a match digest body.
// Used to separate "필수 (회사/직무/경력 매칭)" from "추천 (점수순)".
// Sections with empty Items are skipped during rendering.
type Section struct {
	Title string
	Items []ScoredPosting
}

// TotalItems reports how many ScoredPostings the sections contain in total.
// Used by callers to decide whether the digest is worth sending at all.
func TotalItems(sections []Section) int {
	n := 0
	for _, s := range sections {
		n += len(s.Items)
	}
	return n
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

type sendMailFunc func(ctx context.Context, config SMTPConfig, to string, msg []byte) error

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

	return m.sendMail(ctx, m.config, m.config.To, []byte(message))
}

// SendMatchDigest sends a personalized digest containing one or more
// labeled sections (typically "[필수]" and "[추천]"). Empty / all-empty
// sections are a no-op. Within a section, items are rendered in the order
// the caller supplied — sorting is the caller's responsibility.
func (m *SMTPMailer) SendMatchDigest(ctx context.Context, to string, runAt time.Time, sections []Section) error {
	total := TotalItems(sections)
	if total == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	subject := fmt.Sprintf("SkipJD 추천 공고 %s (%d건)", runAt.Format("2006-01-02"), total)
	body := BuildMatchDigestBody(sections)

	message := strings.Join([]string{
		fmt.Sprintf("From: %s", m.config.From),
		fmt.Sprintf("To: %s", to),
		fmt.Sprintf("Subject: %s", subject),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")

	return m.sendMail(ctx, m.config, to, []byte(message))
}

// BuildMatchDigestBody renders the plain-text body for a SendMatchDigest
// payload. Exposed so callers can preview the output via --dry-run before
// committing to sending.
//
// Sections with a non-empty Title are rendered with a header separator;
// sections with an empty Title render their items directly. Empty sections
// are skipped.
func BuildMatchDigestBody(sections []Section) string {
	var b strings.Builder
	first := true
	for _, sec := range sections {
		if len(sec.Items) == 0 {
			continue
		}
		if !first {
			b.WriteString("\n")
		}
		first = false
		if sec.Title != "" {
			fmt.Fprintf(&b, "── %s ──\n\n", sec.Title)
		}
		renderItems(&b, sec.Items)
	}
	return b.String()
}

func renderItems(b *strings.Builder, items []ScoredPosting) {
	for i, item := range items {
		s := item.Score
		fmt.Fprintf(b, "[%d] 점수 %d (경험:%d / 역량:%d / 성향:%d)\n",
			i+1, s.Total, s.Experience.Hits, s.Competency.Hits, s.Trait.Hits)
		fmt.Fprintf(b, "회사: %s\n", item.Posting.Company)
		fmt.Fprintf(b, "제목: %s\n", item.Posting.Title)
		fmt.Fprintf(b, "마감: %s\n", item.Posting.ClosingDate)
		fmt.Fprintf(b, "URL: %s\n", item.Posting.URL)
		if s.Total > 0 {
			b.WriteString("매칭:\n")
			writeCategory(b, "경험", s.Experience.Matched)
			writeCategory(b, "역량", s.Competency.Matched)
			writeCategory(b, "성향", s.Trait.Matched)
		}
		if i < len(items)-1 {
			b.WriteString("\n")
		}
	}
}

func writeCategory(b *strings.Builder, label string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "  · %s: %s\n", label, strings.Join(items, ", "))
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

func sendSMTPMail(ctx context.Context, config SMTPConfig, to string, msg []byte) error {
	addr := net.JoinHostPort(config.Host, strconv.Itoa(config.Port))
	dialer := net.Dialer{Timeout: smtpDialTimeout}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("dial smtp server %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()

	stop := closeConnOnContextDone(ctx, conn)
	defer stop()

	if err := setSMTPDeadline(conn, ctx, smtpIOTimeout); err != nil {
		return fmt.Errorf("set smtp deadline: %w", err)
	}

	client, err := smtp.NewClient(conn, config.Host)
	if err != nil {
		return fmt.Errorf("create smtp client: %w", err)
	}
	defer func() { _ = client.Close() }()

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
	if err := client.Rcpt(to); err != nil {
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
