package tools

import (
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

type PageLink struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

type ReadPageResult struct {
	PageTitle   string     `json:"page_title"`
	FinalURL    string     `json:"final_url"`
	VisibleText string     `jsoxn:"visible_text"`
	Links       []PageLink `json:"links"`
}

func ReadPage(browser *rod.Browser, targetURL string) (*ReadPageResult, error) {
	page := browser.MustPage(targetURL)
	defer page.MustClose()

	page.MustWaitLoad()
	time.Sleep(2 * time.Second) // 초기에는 단순 대기, 나중에 명시적 wait로 교체 추천

	html, err := page.HTML()
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	pageTitle := strings.TrimSpace(doc.Find("title").First().Text())
	finalURL := page.MustInfo().URL
	visibleText := extractVisibleText(doc)
	links := extractLinks(doc, finalURL)

	return &ReadPageResult{
		PageTitle:   pageTitle,
		FinalURL:    finalURL,
		VisibleText: visibleText,
		Links:       links,
	}, nil
}

func extractLinks(doc *goquery.Document, baseURL string) []PageLink {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}

	rootHost := normalizeHost(base.Host)

	seen := make(map[string]bool)
	links := make([]PageLink, 0, 32)

	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		href, ok := s.Attr("href")
		if !ok {
			return
		}
		href = strings.TrimSpace(href)
		if href == "" {
			return
		}

		text := normalizeSpace(s.Text())
		if text == "" {
			return
		}

		parsed, err := url.Parse(href)
		if err != nil {
			return
		}

		absURL := base.ResolveReference(parsed)
		if absURL == nil {
			return
		}

		if normalizeHost(absURL.Host) != rootHost {
			return
		}

		// 너무 쓸모없는 링크 필터
		key := absURL.String()
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "javascript:") ||
			strings.HasPrefix(lower, "mailto:") ||
			strings.HasPrefix(lower, "tel:") {
			return
		}

		if seen[key] {
			return
		}
		seen[key] = true

		links = append(links, PageLink{
			Text: text,
			URL:  key,
		})
	})

	return links
}

func extractVisibleText(doc *goquery.Document) string {
	clone := cloneDocument(doc)

	// 읽기 방해 요소 제거
	clone.Find("script, style, noscript, svg, footer, header").Remove()

	candidates := []string{
		"main",
		"article",
		"[role='main']",
		".content",
		".container",
		".job-description",
		".job-detail",
		".posting",
		"body",
	}

	best := ""
	for _, sel := range candidates {
		text := normalizeSpace(clone.Find(sel).First().Text())
		if len(text) > len(best) {
			best = text
		}
	}

	return best
}

func normalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.TrimPrefix(host, "www.")
	return host
}

func cloneDocument(doc *goquery.Document) *goquery.Document {
	html, _ := doc.Html()
	cloned, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	return cloned
}

func normalizeSpace(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func NewBrowser() *rod.Browser {
	u := launcher.New().
		Headless(true).
		MustLaunch()

	return rod.New().ControlURL(u).MustConnect()
}
