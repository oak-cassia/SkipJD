package gamejob

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	detailIframePath = "/Recruit/GI_Read_Comt_Ifrm"
	detailIframeQuery = "v1"
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
	client  *http.Client
	baseURL *url.URL
}

func NewDetailScraper(client *http.Client) *DetailScraper {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}

	baseURL, err := url.Parse(defaultBaseURL)
	if err != nil {
		panic(err)
	}

	return &DetailScraper{client: client, baseURL: baseURL}
}

// Scrape fetches the iframe body for a posting URL and extracts text + image URLs.
// The posting URL must contain a GI_No query parameter.
func (s *DetailScraper) Scrape(ctx context.Context, postingURL string) (DetailContent, error) {
	giNo, err := parseGINo(postingURL)
	if err != nil {
		return DetailContent{}, err
	}

	iframeURL := s.baseURL.ResolveReference(&url.URL{
		Path:     detailIframePath,
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

func (s *DetailScraper) fetchIframe(ctx context.Context, iframeURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, iframeURL, nil)
	if err != nil {
		return "", fmt.Errorf("build iframe request: %w", err)
	}
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch iframe: %w", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch iframe: unexpected status %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read iframe body: %w", err)
	}
	return string(body), nil
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
