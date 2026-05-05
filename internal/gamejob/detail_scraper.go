package gamejob

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"

	"skipjd/internal/retry"
)

const (
	// gamejob 상세 페이지는 본문을 두 iframe으로 나눠서 제공한다.
	// Comt: 회사/조직 소개 + 담당업무. GI_Comment: 자격요건/우대사항/전형/근무지.
	// 두 iframe 모두 받아 합쳐야 본문이 완전해진다.
	detailComtIframePath      = "/Recruit/GI_Read_Comt_Ifrm"
	detailGICommentIframePath = "/Recruit/GI_Read_GI_Comment_Ifrm"
	detailIframeQuery         = "v1"
)

// imageHostBlocklist drops images whose hostname suffix-matches any entry.
// Add hosts here as noise patterns are observed in production.
var imageHostBlocklist = []string{
	"img.youtube.com",
	"i.ytimg.com",
	"facebook.com",
	"fbcdn.net",
	"instagram.com",
	"cdninstagram.com",
}

type DetailContent struct {
	TextContent string
	ImageURLs   []string
}

type DetailScraper struct {
	client         *http.Client
	baseURL        *url.URL
	logf           func(string, ...any)
	attemptTimeout time.Duration
}

func NewDetailScraper(client *http.Client) (*DetailScraper, error) {
	if client == nil {
		client = &http.Client{Timeout: defaultClientTimeout}
	}

	baseURL, err := url.Parse(defaultBaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse default base url: %w", err)
	}

	return &DetailScraper{
		client:         client,
		baseURL:        baseURL,
		logf:           log.Printf,
		attemptTimeout: defaultAttemptTimeout,
	}, nil
}

// SetAttemptTimeout overrides the per-HTTP-attempt timeout (default 15s).
// Zero or negative values restore the default.
func (s *DetailScraper) SetAttemptTimeout(d time.Duration) {
	if d <= 0 {
		s.attemptTimeout = defaultAttemptTimeout
		return
	}
	s.attemptTimeout = d
}

// Scrape fetches both iframe bodies for a posting URL and returns the merged
// text + image URLs. The posting URL must contain a GI_No query parameter.
//
// The Comt iframe (회사 소개/담당업무) is the primary source — if it fails the
// whole scrape fails. The GI_Comment iframe (자격요건/우대사항/전형/근무지) is
// treated as best-effort: if it errors we still return the primary content,
// because some older or withdrawn postings only ship the Comt iframe.
func (s *DetailScraper) Scrape(ctx context.Context, postingURL string) (DetailContent, error) {
	giNo, err := parseGINo(postingURL)
	if err != nil {
		return DetailContent{}, err
	}

	primary, err := s.fetchAndExtract(ctx, giNo, detailComtIframePath)
	if err != nil {
		return DetailContent{}, err
	}

	secondary, err := s.fetchAndExtract(ctx, giNo, detailGICommentIframePath)
	if err != nil {
		s.logf("detail secondary iframe failed gi_no=%s err=%v (returning primary only)", giNo, err)
		return primary, nil
	}

	return mergeDetails(primary, secondary), nil
}

func (s *DetailScraper) fetchAndExtract(ctx context.Context, giNo, path string) (DetailContent, error) {
	iframeURL := s.baseURL.ResolveReference(&url.URL{
		Path:     path,
		RawQuery: "gno=" + giNo + "&" + detailIframeQuery,
	}).String()

	htmlText, err := s.fetchIframe(ctx, iframeURL)
	if err != nil {
		return DetailContent{}, err
	}

	doc, err := html.Parse(strings.NewReader(htmlText))
	if err != nil {
		return DetailContent{}, fmt.Errorf("parse iframe html: %w", err)
	}

	return ExtractDetail(doc), nil
}

func mergeDetails(primary, secondary DetailContent) DetailContent {
	var text strings.Builder
	text.WriteString(primary.TextContent)
	if primary.TextContent != "" && secondary.TextContent != "" {
		text.WriteString("\n")
	}
	text.WriteString(secondary.TextContent)

	images := make([]string, 0, len(primary.ImageURLs)+len(secondary.ImageURLs))
	images = append(images, primary.ImageURLs...)
	images = append(images, secondary.ImageURLs...)

	return DetailContent{
		TextContent: text.String(),
		ImageURLs:   images,
	}
}

func (s *DetailScraper) fetchIframe(ctx context.Context, iframeURL string) (string, error) {
	var bodyString string
	err := retry.Do(ctx, httpRetryAttempts, httpRetryBaseDelay,
		func(attemptCtx context.Context) error {
			attemptCtx, cancel := context.WithTimeout(attemptCtx, s.attemptTimeout)
			defer cancel()

			req, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, iframeURL, nil)
			if err != nil {
				return permanentErr{fmt.Errorf("build iframe request: %w", err)}
			}
			req.Header.Set("User-Agent", defaultUserAgent)

			resp, err := s.client.Do(req)
			if err != nil {
				return fmt.Errorf("fetch iframe: %w", err)
			}
			defer func(Body io.ReadCloser) {
				_ = Body.Close()
			}(resp.Body)

			if resp.StatusCode != http.StatusOK {
				return statusErr{status: resp.StatusCode, msg: fmt.Sprintf("fetch iframe: unexpected status %s", resp.Status)}
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("read iframe body: %w", err)
			}
			bodyString = string(body)
			return nil
		},
		isRetryableHTTP,
	)
	if err != nil {
		return "", err
	}
	return bodyString, nil
}

// ExtractDetail walks an iframe document and pulls text + non-blocklisted image URLs.
// Exposed for testing with fixture HTML.
func ExtractDetail(doc *html.Node) DetailContent {
	textBuilder := strings.Builder{}
	images := make([]string, 0)
	links := make([]string, 0)
	seenLink := make(map[string]struct{})

	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node == nil {
			return
		}

		switch {
		case node.Type == html.TextNode:
			textBuilder.WriteString(node.Data)
		case isElement(node, "img"):
			if src := absoluteImageSrc(node); src != "" && !isBlockedImage(node, src) {
				images = append(images, src)
			}
			return
		case isElement(node, "a"):
			href := strings.TrimSpace(attr(node, "href"))
			if isExternalLink(href) {
				if _, exists := seenLink[href]; !exists {
					seenLink[href] = struct{}{}
					links = append(links, href)
				}
			}
		case isElement(node, "script"), isElement(node, "style"):
			return
		case isElement(node, "br"), isElement(node, "p"), isElement(node, "div"), isElement(node, "tr"):
			textBuilder.WriteString("\n")
		}

		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(doc)

	text := normalizeMultilineText(textBuilder.String())
	if len(links) > 0 {
		var sb strings.Builder
		sb.WriteString(text)
		for _, link := range links {
			sb.WriteString("\n[링크] ")
			sb.WriteString(link)
		}
		text = sb.String()
	}

	return DetailContent{
		TextContent: text,
		ImageURLs:   images,
	}
}

func parseGINo(postingURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(postingURL))
	if err != nil {
		return "", fmt.Errorf("parse posting url: %w", err)
	}
	value := parsed.Query().Get("GI_No")
	if value == "" {
		return "", fmt.Errorf("posting url missing GI_No: %s", postingURL)
	}
	if _, err := strconv.Atoi(value); err != nil {
		return "", fmt.Errorf("posting url has non-numeric GI_No %q", value)
	}
	return value, nil
}

func absoluteImageSrc(node *html.Node) string {
	src := strings.TrimSpace(attr(node, "src"))
	if src == "" || strings.HasPrefix(src, "data:") {
		return ""
	}
	parsed, err := url.Parse(src)
	if err != nil || !parsed.IsAbs() {
		return ""
	}
	return parsed.String()
}

func isBlockedImage(node *html.Node, src string) bool {
	if w := parseDimension(attr(node, "width")); w > 0 && w <= 1 {
		return true
	}
	if h := parseDimension(attr(node, "height")); h > 0 && h <= 1 {
		return true
	}

	parsed, err := url.Parse(src)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	for _, blocked := range imageHostBlocklist {
		if host == blocked || strings.HasSuffix(host, "."+blocked) {
			return true
		}
	}
	return false
}

func parseDimension(raw string) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0
	}
	return v
}

func isExternalLink(href string) bool {
	return strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://")
}

func normalizeMultilineText(raw string) string {
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.Join(strings.Fields(line), " ")
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return strings.Join(out, "\n")
}
