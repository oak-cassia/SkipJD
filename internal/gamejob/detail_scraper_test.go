package gamejob

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

func TestParseGINoExtractsValueFromQuery(t *testing.T) {
	value, err := parseGINo("https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=278459")
	require.NoError(t, err)
	assert.Equal(t, "278459", value)
}

func TestParseGINoFailsWhenMissing(t *testing.T) {
	_, err := parseGINo("https://www.gamejob.co.kr/Recruit/GI_Read/View?other=1")
	require.Error(t, err)
}

func TestParseGINoFailsWhenNonNumeric(t *testing.T) {
	_, err := parseGINo("https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=abc")
	require.Error(t, err)
}

func TestExtractDetailTextHeavyKeepsTextAndFiltersNoiseImage(t *testing.T) {
	doc := loadFixture(t, "detail_text.html")
	content := ExtractDetail(doc)

	assert.Contains(t, content.TextContent, "스마일게이트 백엔드 엔지니어 채용")
	assert.Contains(t, content.TextContent, "주요 업무")
	assert.Contains(t, content.TextContent, "[링크] https://careers.smilegate.com/apply/12345")

	assert.Equal(t, []string{"https://careers.smilegate.com/img/header_lostark.png"}, content.ImageURLs,
		"YouTube thumbnail should be filtered by blocklist")
}

func TestExtractDetailImageHeavyKeepsShortRequiredText(t *testing.T) {
	doc := loadFixture(t, "detail_image.html")
	content := ExtractDetail(doc)

	assert.Contains(t, content.TextContent, "사전 과제")
	assert.Contains(t, content.TextContent, "[링크] https://files.dreammotion.com/assignments/client_2026.zip",
		"<a href> should be appended even when text contains a tags label")

	require.Len(t, content.ImageURLs, 1)
	assert.Equal(t, "https://imgs.gamejob.co.kr/ext/dreammotion/jobad_2026q2.png", content.ImageURLs[0])
}

func TestExtractDetailEmptyReturnsZeroValues(t *testing.T) {
	doc := loadFixture(t, "detail_empty.html")
	content := ExtractDetail(doc)

	assert.Empty(t, content.TextContent)
	assert.Empty(t, content.ImageURLs)
}

func TestIsBlockedImageMatchesHostSuffix(t *testing.T) {
	node := imageNode(t, `<img src="https://i.ytimg.com/vi/x/0.jpg">`)
	assert.True(t, isBlockedImage(node, "https://i.ytimg.com/vi/x/0.jpg"))

	node = imageNode(t, `<img src="https://scontent-icn1-1.cdninstagram.com/photo.jpg">`)
	assert.True(t, isBlockedImage(node, "https://scontent-icn1-1.cdninstagram.com/photo.jpg"))

	node = imageNode(t, `<img src="https://imgs.gamejob.co.kr/ext/photo.png">`)
	assert.False(t, isBlockedImage(node, "https://imgs.gamejob.co.kr/ext/photo.png"))
}

func TestIsBlockedImageDropsTrackingPixel(t *testing.T) {
	node := imageNode(t, `<img src="https://example.com/p.gif" width="1" height="1">`)
	assert.True(t, isBlockedImage(node, "https://example.com/p.gif"))
}

func TestDetailScraperFetchesBothIframesAndMerges(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		assert.Equal(t, "278459", r.URL.Query().Get("gno"))

		switch r.URL.Path {
		case "/Recruit/GI_Read_Comt_Ifrm":
			body, err := os.ReadFile("testdata/detail_image.html")
			require.NoError(t, err)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		case "/Recruit/GI_Read_GI_Comment_Ifrm":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html><body>[지원자격]<br>5년 이상의 서버 개발 경험</body></html>`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	scraper := NewDetailScraper(server.Client())
	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)
	scraper.baseURL = parsed

	content, err := scraper.Scrape(context.Background(),
		"https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=278459")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"/Recruit/GI_Read_Comt_Ifrm",
		"/Recruit/GI_Read_GI_Comment_Ifrm",
	}, paths)
	assert.Contains(t, content.TextContent, "사전 과제")
	assert.Contains(t, content.TextContent, "[지원자격]")
	assert.Contains(t, content.TextContent, "5년 이상의 서버 개발 경험")
	require.Len(t, content.ImageURLs, 1)
}

func TestDetailScraperGracefullyFallsBackWhenSecondaryIframeFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Recruit/GI_Read_Comt_Ifrm":
			body, err := os.ReadFile("testdata/detail_image.html")
			require.NoError(t, err)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		case "/Recruit/GI_Read_GI_Comment_Ifrm":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	scraper := NewDetailScraper(server.Client())
	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)
	scraper.baseURL = parsed

	content, err := scraper.Scrape(context.Background(),
		"https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=278459")
	require.NoError(t, err)
	assert.Contains(t, content.TextContent, "사전 과제")
}

func loadFixture(t *testing.T, name string) *html.Node {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	doc, err := html.Parse(strings.NewReader(string(raw)))
	require.NoError(t, err)
	return doc
}

func imageNode(t *testing.T, snippet string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(snippet))
	require.NoError(t, err)
	found := findFirst(doc, func(n *html.Node) bool {
		return isElement(n, "img")
	})
	require.NotNil(t, found)
	return found
}

