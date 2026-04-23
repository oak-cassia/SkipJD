package gamejob

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseListPageExtractsRowsAndPagination(t *testing.T) {
	scraper := newTestScraper()
	htmlText := readFixture(t, "client_page1.html")

	page, err := scraper.parseListPage(htmlText, 1)
	require.NoError(t, err)
	require.Len(t, page.rows, 2)

	assert.True(t, page.hasNext)
	assert.Equal(t, "드림모션", page.rows[0].company)
	assert.Equal(t, "[PC 신작 프로젝트] 프로그래머 인재 채용 (신입/경력)", page.rows[0].title)
	assert.Equal(t, "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=278544", page.rows[0].url)
	assert.Equal(t, "신입", page.rows[0].expText)
	assert.Equal(t, "채용시", page.rows[0].closingDate)
	assert.Equal(t, "3시간 전 수정", page.rows[0].modifyText)
	assert.Equal(t, "크니브스튜디오", page.rows[1].company)
}

func TestParseObservedDateSupportsRelativeAndAbsoluteFormats(t *testing.T) {
	scraper := newTestScraper()
	scraper.now = func() time.Time {
		return time.Date(2026, 4, 7, 0, 10, 0, 0, scraper.loc)
	}
	todayDate := time.Date(2026, 4, 7, 0, 0, 0, 0, scraper.loc)

	testCases := []struct {
		name       string
		modifyText string
		expected   string
	}{
		{name: "hours", modifyText: "2시간 전 수정", expected: "2026-04-06"},
		{name: "days", modifyText: "14일 전 수정", expected: "2026-03-24"},
		{name: "absolute", modifyText: "03/27(금) 등록", expected: "2026-03-27"},
		{name: "yesterday", modifyText: "어제 수정", expected: "2026-04-06"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			observedDate, err := scraper.parseObservedDate(todayDate, tc.modifyText)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, observedDate.Format("2006-01-02"))
		})
	}
}

func TestParseMinExperienceYears(t *testing.T) {
	assert.Equal(t, 0, parseMinExperienceYears("신입"))
	assert.Equal(t, 0, parseMinExperienceYears("경력무관"))
	assert.Equal(t, 3, parseMinExperienceYears("경력3년↑"))
	assert.Equal(t, 1, parseMinExperienceYears("경력 1-5년차"))
}

func TestNormalizeCompanyName(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		// 법인격 접두사
		{"㈜넵튠", "넵튠"},
		{"(주)웹젠", "웹젠"},
		{"주식회사 컴투스", "컴투스"},
		{"주식회사인포바인", "인포바인"},
		{"㈔한국인공지능게임협", "한국인공지능게임협"},
		// 법인격 접미사
		{"팀스파르타㈜", "팀스파르타"},
		{"라인게임즈㈜", "라인게임즈"},
		{"111퍼센트㈜", "111퍼센트"},
		// 후미 괄호 부가정보 + 접미사
		{"옴니크래프트랩스㈜(크래프톤 계열회사)", "옴니크래프트랩스"},
		// 후미 괄호 부가정보만
		{"EA코리아 (Electronic Arts Korea)", "EA코리아"},
		// 변환 없음
		{"스마일게이트", "스마일게이트"},
		{"NC", "NC"},
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.expected, NormalizeCompanyName(tc.input))
		})
	}
}

func TestNormalizeDutyCodesDeduplicatesAndOrdersCodes(t *testing.T) {
	assert.Equal(t, []int{1, 3, 16}, NormalizeDutyCodes([]int{16, 1, 3, 1}))
	assert.Equal(t, []int{3}, NormalizeDutyCodes([]int{3, 3}))
}

func TestScrapeStopsAtCutoffCallback(t *testing.T) {
	var requestedTargets []string
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			require.Equal(t, http.MethodPost, r.Method)
			require.Equal(t, "/Recruit/_GI_Job_List/", r.URL.Path)
			require.Equal(t, "XMLHttpRequest", r.Header.Get("X-Requested-With"))
			require.Equal(t, "Mozilla/5.0", r.Header.Get("User-Agent"))
			require.NoError(t, r.ParseForm())

			dutyCode := r.FormValue("condition[duty]")
			assert.Equal(t, "duty", r.FormValue("condition[menucode]"))
			assert.Equal(t, "4", r.FormValue("order"))
			assert.Equal(t, "40", r.FormValue("pagesize"))
			assert.Equal(t, "1", r.FormValue("tabcode"))
			assert.Equal(t, []string{dutyCode}, r.Form["condition[dutyArr][]"])
			assert.Equal(t, "https://www.gamejob.co.kr/Recruit/joblist?duty="+dutyCode+"&menucode=duty", r.Header.Get("Referer"))

			page := r.FormValue("page")
			requestedTargets = append(requestedTargets, dutyCode+":"+page)

			switch dutyCode + ":" + page {
			case "1:1", "3:1", "16:1":
				return htmlResponse(r, buildListPageHTML(1, []testListRow{
					{
						company:     "크니브스튜디오",
						title:       "Posting " + dutyCode + "-1",
						url:         "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=100" + dutyCode,
						expText:     "신입",
						closingDate: "채용시",
						modifyText:  "2시간 전 수정",
					},
					{
						company:     "에피드게임즈",
						title:       "Posting " + dutyCode + "-2",
						url:         "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868" + dutyCode,
						expText:     "경력3년↑",
						closingDate: "상시",
						modifyText:  "1일 전 수정",
					},
				}, true)), nil
			case "1:2", "3:2", "16:2":
				return htmlResponse(r, buildListPageHTML(2, []testListRow{
					{
						company:     "에피드게임즈",
						title:       "Posting " + dutyCode + "-3",
						url:         "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275869" + dutyCode,
						expText:     "경력 3-5년",
						closingDate: "상시",
						modifyText:  "3일 전 수정",
					},
					{
						company:     "에피드게임즈",
						title:       "Old posting " + dutyCode,
						url:         "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275870" + dutyCode,
						expText:     "경력무관",
						closingDate: "채용시",
						modifyText:  "03/30(월) 등록",
					},
				}, true)), nil
			default:
				t.Fatalf("unexpected page request: %s", page)
			}
			return nil, nil
		}),
	}

	scraper := NewClientScraper(client)
	scraper.baseURL = mustParseURL(t, "https://www.gamejob.co.kr")
	scraper.now = func() time.Time {
		return time.Date(2026, 4, 7, 9, 0, 0, 0, scraper.loc)
	}

	postings, err := scraper.Scrape(context.Background(), ScrapeOptions{
		TodayDate: time.Date(2026, 4, 7, 0, 0, 0, 0, scraper.loc),
		Stop: func(posting ScrapedPosting) bool {
			cutoffDate := time.Date(2026, 4, 4, 0, 0, 0, 0, scraper.loc)
			return posting.ObservedDate.Before(cutoffDate)
		},
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"1:1", "1:2", "3:1", "3:2", "16:1", "16:2"}, requestedTargets)
	require.Len(t, postings, 9)
	assert.True(t, slices.Equal([]string{
		"Posting 1-1",
		"Posting 1-2",
		"Posting 1-3",
		"Posting 3-1",
		"Posting 3-2",
		"Posting 3-3",
		"Posting 16-1",
		"Posting 16-2",
		"Posting 16-3",
	}, []string{
		postings[0].Title,
		postings[1].Title,
		postings[2].Title,
		postings[3].Title,
		postings[4].Title,
		postings[5].Title,
		postings[6].Title,
		postings[7].Title,
		postings[8].Title,
	}))
	assert.Equal(t, "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=1001", postings[0].SourceKey)
	assert.Equal(t, 1, postings[0].DutyCode)
	assert.Equal(t, "2026-04-07", postings[0].ObservedDate.Format("2006-01-02"))
	assert.Equal(t, "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=2758681", postings[1].SourceKey)
	assert.Equal(t, 1, postings[1].DutyCode)
	assert.Equal(t, "상시", postings[1].ClosingDate)
	assert.Equal(t, 3, postings[1].MinExperienceYears)
	assert.Equal(t, "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=2758681", postings[1].URL)
	assert.Equal(t, "2026-04-04", postings[2].ObservedDate.Format("2006-01-02"))
	assert.Equal(t, 1, postings[2].DutyCode)
	assert.Equal(t, 3, postings[3].DutyCode)
	assert.Equal(t, 16, postings[6].DutyCode)
}

func TestScrapeStopsAtDefaultMaxPages(t *testing.T) {
	var requestedTargets []string
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			require.NoError(t, r.ParseForm())
			dutyCode := r.FormValue("condition[duty]")
			page := r.FormValue("page")
			requestedTargets = append(requestedTargets, dutyCode+":"+page)

			currentPage, err := strconv.Atoi(page)
			require.NoError(t, err)

			return htmlResponse(r, buildListPageHTML(currentPage, []testListRow{{
				company:     "에피드게임즈",
				title:       fmt.Sprintf("Posting %s-%d", dutyCode, currentPage),
				url:         fmt.Sprintf("https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=%s%d", dutyCode, 300000+currentPage),
				expText:     "신입",
				closingDate: "채용시",
				modifyText:  "1시간 전 수정",
			}}, true)), nil
		}),
	}

	scraper := NewClientScraper(client)
	scraper.now = func() time.Time {
		return time.Date(2026, 4, 7, 9, 0, 0, 0, scraper.loc)
	}

	postings, err := scraper.Scrape(context.Background(), ScrapeOptions{
		TodayDate: time.Date(2026, 4, 7, 0, 0, 0, 0, scraper.loc),
	})
	require.NoError(t, err)

	require.Len(t, requestedTargets, len(defaultDutyCodes)*DefaultMaxPages)
	assert.Equal(t, "1:1", requestedTargets[0])
	assert.Equal(t, "16:10", requestedTargets[len(requestedTargets)-1])
	assert.Len(t, postings, len(defaultDutyCodes)*DefaultMaxPages)
	assert.Equal(t, 1, postings[0].DutyCode)
	assert.Equal(t, 3, postings[10].DutyCode)
	assert.Equal(t, 16, postings[20].DutyCode)
}

func TestScrapeStopsOnEmptyPage(t *testing.T) {
	var requestedTargets []string
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			require.NoError(t, r.ParseForm())
			requestedTargets = append(requestedTargets, r.FormValue("condition[duty]")+":"+r.FormValue("page"))
			return htmlResponse(r, buildListPageHTML(1, nil, false)), nil
		}),
	}

	scraper := NewClientScraper(client)
	postings, err := scraper.Scrape(context.Background(), ScrapeOptions{
		TodayDate: time.Date(2026, 4, 7, 0, 0, 0, 0, scraper.loc),
		MaxPages:  3,
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"1:1", "3:1", "16:1"}, requestedTargets)
	assert.Empty(t, postings)
}

func TestScrapeStopsWhenPaginationEnds(t *testing.T) {
	var requestedTargets []string
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			require.NoError(t, r.ParseForm())
			dutyCode := r.FormValue("condition[duty]")
			requestedTargets = append(requestedTargets, dutyCode+":"+r.FormValue("page"))
			return htmlResponse(r, buildListPageHTML(1, []testListRow{{
				company:     "에피드게임즈",
				title:       "Only page " + dutyCode,
				url:         "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=40000" + dutyCode,
				expText:     "경력무관",
				closingDate: "채용시",
				modifyText:  "오늘 수정",
			}}, false)), nil
		}),
	}

	scraper := NewClientScraper(client)
	postings, err := scraper.Scrape(context.Background(), ScrapeOptions{
		TodayDate: time.Date(2026, 4, 7, 0, 0, 0, 0, scraper.loc),
		MaxPages:  3,
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"1:1", "3:1", "16:1"}, requestedTargets)
	require.Len(t, postings, 3)
	assert.Equal(t, "Only page 1", postings[0].Title)
	assert.Equal(t, "Only page 3", postings[1].Title)
	assert.Equal(t, "Only page 16", postings[2].Title)
	assert.Equal(t, 1, postings[0].DutyCode)
	assert.Equal(t, 3, postings[1].DutyCode)
	assert.Equal(t, 16, postings[2].DutyCode)
}

func TestScrapeWithoutStopContinuesPastOlderRows(t *testing.T) {
	var requestedTargets []string
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			require.NoError(t, r.ParseForm())
			dutyCode := r.FormValue("condition[duty]")
			page := r.FormValue("page")
			requestedTargets = append(requestedTargets, dutyCode+":"+page)

			switch dutyCode + ":" + page {
			case "1:1", "3:1", "16:1":
				return htmlResponse(r, buildListPageHTML(1, []testListRow{{
					company:     "에피드게임즈",
					title:       "Old but still collected " + dutyCode,
					url:         "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=500001" + dutyCode,
					expText:     "경력무관",
					closingDate: "채용시",
					modifyText:  "03/30(월) 등록",
				}}, true)), nil
			case "1:2", "3:2", "16:2":
				return htmlResponse(r, buildListPageHTML(2, []testListRow{{
					company:     "에피드게임즈",
					title:       "Second page " + dutyCode,
					url:         "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=500002" + dutyCode,
					expText:     "신입",
					closingDate: "채용시",
					modifyText:  "오늘 수정",
				}}, false)), nil
			default:
				t.Fatalf("unexpected page request: %s", page)
			}
			return nil, nil
		}),
	}

	scraper := NewClientScraper(client)
	scraper.now = func() time.Time {
		return time.Date(2026, 4, 7, 9, 0, 0, 0, scraper.loc)
	}

	postings, err := scraper.Scrape(context.Background(), ScrapeOptions{
		TodayDate: time.Date(2026, 4, 7, 0, 0, 0, 0, scraper.loc),
		MaxPages:  3,
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"1:1", "1:2", "3:1", "3:2", "16:1", "16:2"}, requestedTargets)
	require.Len(t, postings, 6)
	assert.Equal(t, "Old but still collected 1", postings[0].Title)
	assert.Equal(t, "Second page 1", postings[1].Title)
	assert.Equal(t, "Old but still collected 3", postings[2].Title)
	assert.Equal(t, "Second page 3", postings[3].Title)
	assert.Equal(t, "Old but still collected 16", postings[4].Title)
	assert.Equal(t, "Second page 16", postings[5].Title)
	assert.Equal(t, 1, postings[0].DutyCode)
	assert.Equal(t, 3, postings[2].DutyCode)
	assert.Equal(t, 16, postings[4].DutyCode)
}

func TestParseObservedDateReturnsErrorForUnsupportedModifyText(t *testing.T) {
	scraper := newTestScraper()
	_, err := scraper.parseObservedDate(time.Date(2026, 4, 7, 0, 0, 0, 0, scraper.loc), "방금 전 수정")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported modify date format")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func newTestScraper() *ClientScraper {
	scraper := NewClientScraper(nil)
	scraper.now = func() time.Time {
		return time.Date(2026, 4, 7, 9, 0, 0, 0, scraper.loc)
	}
	return scraper
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func mustParseURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()
	parsedURL, err := url.Parse(rawURL)
	require.NoError(t, err)
	return parsedURL
}

type testListRow struct {
	company     string
	title       string
	url         string
	expText     string
	closingDate string
	modifyText  string
}

func buildListPageHTML(currentPage int, rows []testListRow, hasNext bool) string {
	var builder strings.Builder
	builder.WriteString("<html><body><table class=\"tblList\"><tbody>")
	for _, row := range rows {
		fmt.Fprintf(
			&builder,
			"<tr><td><strong>%s</strong></td><td><div class=\"tit\"><a href=\"%s\">%s</a></div><p class=\"info\"><span>%s</span></p></td><td><span class=\"date\">%s</span><span class=\"modifyDate\">%s</span></td></tr>",
			row.company,
			row.url,
			row.title,
			row.expText,
			row.closingDate,
			row.modifyText,
		)
	}
	builder.WriteString("</tbody></table>")
	if hasNext {
		nextPage := currentPage + 1
		fmt.Fprintf(&builder, "<div class=\"pagination\"><a data-page=\"%d\">%d</a></div>", nextPage, nextPage)
	}
	builder.WriteString("</body></html>")
	return builder.String()
}

func htmlResponse(request *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header: http.Header{
			"Content-Type": []string{"text/html; charset=utf-8"},
		},
		Body:    io.NopCloser(strings.NewReader(body)),
		Request: request,
	}
}
