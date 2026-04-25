package gamejob

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	defaultBaseURL   = "https://www.gamejob.co.kr"
	jobListPath      = "/Recruit/joblist"
	jobListAjaxPath  = "/Recruit/_GI_Job_List/"
	defaultUserAgent = "Mozilla/5.0"
	DefaultMaxPages  = 10
	SourceName       = "browser_agent"
)

var defaultDutyCodes = []int{1, 3, 16}

var (
	hoursAgoPattern           = regexp.MustCompile(`(\d+)\s*시간\s*전`)
	minutesAgoPattern         = regexp.MustCompile(`(\d+)\s*분\s*전`)
	daysAgoPattern            = regexp.MustCompile(`(\d+)\s*일\s*전`)
	monthDayPattern           = regexp.MustCompile(`(\d{2})/(\d{2})`)
	yearRangePattern          = regexp.MustCompile(`(\d+)\s*[-~]\s*\d+\s*년`)
	yearsPattern              = regexp.MustCompile(`(\d+)\s*년`)
	companyParenSuffixPattern = regexp.MustCompile(`\s*\([^)]*\)\s*$`)

	legalEntitySuffixes = []string{"㈜", "(주)", "㈔"}
	legalEntityPrefixes = []string{"주식회사 ", "주식회사", "(주)", "㈜", "㈔"}
)

type ScrapedPosting struct {
	SourceKey          string    `json:"source_key"`
	Title              string    `json:"title"`
	Company            string    `json:"company"`
	DutyCode           int       `json:"duty_code,omitempty"`
	URL                string    `json:"url"`
	ClosingDate        string    `json:"closing_date"`
	MinExperienceYears int       `json:"min_experience_years"`
	ObservedDate       time.Time `json:"observed_date"`
}

type ScrapeOptions struct {
	TodayDate time.Time
	MaxPages  int
	Stop      func(ScrapedPosting) bool
}

type ClientScraper struct {
	client  *http.Client
	baseURL *url.URL
	now     func() time.Time
	loc     *time.Location
}

type listPage struct {
	rows    []listRow
	hasNext bool
}

type listRow struct {
	company     string
	title       string
	url         string
	expText     string
	closingDate string
	modifyText  string
}

func NewClientScraper(client *http.Client) *ClientScraper {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}

	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		loc = time.FixedZone("KST", 9*60*60)
	}

	baseURL, err := url.Parse(defaultBaseURL)
	if err != nil {
		panic(err)
	}

	return &ClientScraper{
		client:  client,
		baseURL: baseURL,
		now:     time.Now,
		loc:     loc,
	}
}

func (s *ClientScraper) Scrape(ctx context.Context, opts ScrapeOptions) ([]ScrapedPosting, error) {
	if opts.TodayDate.IsZero() {
		return nil, fmt.Errorf("today date is required")
	}

	todayDate := s.normalizeDate(opts.TodayDate)
	maxPages := opts.MaxPages
	if maxPages <= 0 {
		maxPages = DefaultMaxPages
	}

	postings := make([]ScrapedPosting, 0)

	for _, dutyCode := range defaultDutyCodes {
		for page := 1; page <= maxPages; page++ {
			pagePostings, stop, hasNext, err := s.scrapePage(ctx, dutyCode, page, todayDate, opts)
			if err != nil {
				return nil, err
			}
			postings = append(postings, pagePostings...)

			if stop || !hasNext {
				break
			}
		}
	}

	return postings, nil
}

func (s *ClientScraper) scrapePage(ctx context.Context, dutyCode int, page int, todayDate time.Time, opts ScrapeOptions) ([]ScrapedPosting, bool, bool, error) {
	htmlText, err := s.fetchPage(ctx, dutyCode, page)
	if err != nil {
		return nil, false, false, err
	}

	parsedPage, err := s.parseListPage(htmlText, page)
	if err != nil {
		return nil, false, false, err
	}
	if len(parsedPage.rows) == 0 {
		return nil, false, false, nil
	}

	var postings []ScrapedPosting
	stop := false

	for _, row := range parsedPage.rows {
		observedDate, err := s.parseObservedDate(todayDate, row.modifyText)
		if err != nil {
			return nil, false, false, err
		}
		sourceKey, err := buildSourceKey(row.url)
		if err != nil {
			return nil, false, false, err
		}

		scrapedPosting := ScrapedPosting{
			SourceKey:          sourceKey,
			Title:              row.title,
			Company:            row.company,
			DutyCode:           dutyCode,
			ClosingDate:        row.closingDate,
			URL:                row.url,
			MinExperienceYears: parseMinExperienceYears(row.expText),
			ObservedDate:       observedDate,
		}
		if opts.Stop != nil && opts.Stop(scrapedPosting) {
			stop = true
			break
		}

		postings = append(postings, scrapedPosting)
	}

	return postings, stop, parsedPage.hasNext, nil
}

func (s *ClientScraper) fetchPage(ctx context.Context, dutyCode int, page int) (string, error) {
	dutyCodeValue := strconv.Itoa(dutyCode)
	form := url.Values{}
	form.Set("condition[dutyCtgr]", "0")
	form.Set("condition[duty]", dutyCodeValue)
	form.Set("condition[reg_dt]", "0")
	form.Set("condition[menucode]", "duty")
	form.Set("condition[searchtype]", "B")
	form.Add("condition[dutyArr][]", dutyCodeValue)
	form.Add("condition[dutyCtgrSelect][]", dutyCodeValue)
	form.Add("condition[dutySelect][]", dutyCodeValue)
	form.Set("page", strconv.Itoa(page))
	form.Set("direct", "0")
	form.Set("order", "4")
	form.Set("pagesize", "40")
	form.Set("tabcode", "1")

	requestURL := s.baseURL.ResolveReference(&url.URL{Path: jobListAjaxPath})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("build request for page %d: %w", page, err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Origin", s.origin())
	req.Header.Set("Referer", s.refererURL(dutyCode))

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch page %d: %w", page, err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch page %d: unexpected status %s", page, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read page %d: %w", page, err)
	}

	return string(body), nil
}

func (s *ClientScraper) parseListPage(htmlText string, currentPage int) (listPage, error) {
	doc, err := html.Parse(strings.NewReader(htmlText))
	if err != nil {
		return listPage{}, fmt.Errorf("parse page %d html: %w", currentPage, err)
	}

	table := findFirst(doc, func(node *html.Node) bool {
		return isElement(node, "table") && hasClass(node, "tblList")
	})
	if table == nil {
		return listPage{}, fmt.Errorf("parse page %d html: listing table not found", currentPage)
	}

	tbody := findFirst(table, func(node *html.Node) bool {
		return isElement(node, "tbody")
	})
	if tbody == nil {
		return listPage{}, fmt.Errorf("parse page %d html: listing body not found", currentPage)
	}

	rows := make([]listRow, 0)
	for _, tr := range childElementsByTag(tbody, "tr") {
		tds := childElementsByTag(tr, "td")
		if len(tds) < 3 {
			continue
		}

		titleLink := findFirst(tds[1], func(node *html.Node) bool {
			return isElement(node, "a") && hasAncestorWithClass(node, "tit")
		})
		if titleLink == nil {
			continue
		}

		href := strings.TrimSpace(attr(titleLink, "href"))
		postingURL, err := s.resolveURL(href)
		if err != nil {
			return listPage{}, fmt.Errorf("parse page %d row url: %w", currentPage, err)
		}

		companyNode := findFirst(tds[0], func(node *html.Node) bool {
			return isElement(node, "strong")
		})
		rawCompany := normalizeSpace(textContent(companyNode))
		if rawCompany == "" {
			rawCompany = normalizeSpace(textContent(tds[0]))
		}
		company := NormalizeCompanyName(rawCompany)

		expNode := findFirst(tds[1], func(node *html.Node) bool {
			return isElement(node, "span") && hasAncestor(node, func(ancestor *html.Node) bool {
				return isElement(ancestor, "p") && hasClass(ancestor, "info")
			})
		})
		closingNode := findFirst(tds[2], func(node *html.Node) bool {
			return isElement(node, "span") && hasClass(node, "date")
		})
		modifyNode := findFirst(tds[2], func(node *html.Node) bool {
			return isElement(node, "span") && hasClass(node, "modifyDate")
		})

		rows = append(rows, listRow{
			company:     company,
			title:       normalizeSpace(textContent(titleLink)),
			url:         postingURL,
			expText:     normalizeSpace(textContent(expNode)),
			closingDate: normalizeSpace(textContent(closingNode)),
			modifyText:  normalizeSpace(textContent(modifyNode)),
		})
	}

	pagination := findFirst(doc, func(node *html.Node) bool {
		return isElement(node, "div") && hasClass(node, "pagination")
	})

	nextPage := strconv.Itoa(currentPage + 1)
	hasNext := pagination != nil && findFirst(pagination, func(node *html.Node) bool {
		return isElement(node, "a") && attr(node, "data-page") == nextPage
	}) != nil

	return listPage{
		rows:    rows,
		hasNext: hasNext,
	}, nil
}

func (s *ClientScraper) parseObservedDate(todayDate time.Time, modifyText string) (time.Time, error) {
	normalized := normalizeSpace(modifyText)
	if normalized == "" {
		return time.Time{}, fmt.Errorf("modify text is empty")
	}

	referenceTime := s.referenceTime(todayDate)

	if matches := minutesAgoPattern.FindStringSubmatch(normalized); len(matches) == 2 {
		minutes, _ := strconv.Atoi(matches[1])
		return s.normalizeDate(referenceTime.Add(-time.Duration(minutes) * time.Minute)), nil
	}
	if matches := hoursAgoPattern.FindStringSubmatch(normalized); len(matches) == 2 {
		hours, _ := strconv.Atoi(matches[1])
		return s.normalizeDate(referenceTime.Add(-time.Duration(hours) * time.Hour)), nil
	}
	if matches := daysAgoPattern.FindStringSubmatch(normalized); len(matches) == 2 {
		days, _ := strconv.Atoi(matches[1])
		return todayDate.AddDate(0, 0, -days), nil
	}
	if strings.Contains(normalized, "오늘") {
		return todayDate, nil
	}
	if strings.Contains(normalized, "어제") {
		return todayDate.AddDate(0, 0, -1), nil
	}
	if matches := monthDayPattern.FindStringSubmatch(normalized); len(matches) == 3 {
		month, _ := strconv.Atoi(matches[1])
		day, _ := strconv.Atoi(matches[2])
		candidate := time.Date(todayDate.Year(), time.Month(month), day, 0, 0, 0, 0, s.loc)
		if candidate.After(todayDate) {
			candidate = candidate.AddDate(-1, 0, 0)
		}
		return candidate, nil
	}

	return time.Time{}, fmt.Errorf("unsupported modify date format: %q", modifyText)
}

func parseMinExperienceYears(expText string) int {
	normalized := normalizeSpace(expText)
	if normalized == "" {
		return 0
	}
	if strings.Contains(normalized, "신입") || strings.Contains(normalized, "경력무관") || strings.Contains(normalized, "무관") {
		return 0
	}
	if matches := yearRangePattern.FindStringSubmatch(normalized); len(matches) == 2 {
		years, err := strconv.Atoi(matches[1])
		if err == nil {
			return years
		}
	}
	if matches := yearsPattern.FindStringSubmatch(normalized); len(matches) == 2 {
		years, err := strconv.Atoi(matches[1])
		if err == nil {
			return years
		}
	}

	return 0
}

func buildSourceKey(rawURL string) (string, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", fmt.Errorf("source key requires a non-empty url")
	}

	parsedURL, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("parse source url: %w", err)
	}
	parsedURL.Fragment = ""
	return parsedURL.String(), nil
}

func NormalizeDutyCodes(codes []int) []int {
	seen := make(map[int]struct{}, len(codes))
	normalized := make([]int, 0, len(codes))

	for _, code := range codes {
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		normalized = append(normalized, code)
	}

	sort.SliceStable(normalized, func(i, j int) bool {
		leftPriority := defaultDutyCodePriority(normalized[i])
		rightPriority := defaultDutyCodePriority(normalized[j])
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		return normalized[i] < normalized[j]
	})

	return normalized
}

func normalizeSpace(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

// NormalizeCompanyName strips legal entity markers and trailing parenthetical
// annotations from a raw company name (e.g. "㈜넵튠" → "넵튠",
// "옴니크래프트랩스㈜(크래프톤 계열회사)" → "옴니크래프트랩스").
func NormalizeCompanyName(name string) string {
	s := strings.TrimSpace(name)
	if s == "" {
		return s
	}

	// 1. 후미 괄호 부가정보 제거: "옴니크래프트랩스㈜(크래프톤 계열회사)" → "옴니크래프트랩스㈜"
	//                            "EA코리아 (Electronic Arts Korea)"     → "EA코리아"
	s = companyParenSuffixPattern.ReplaceAllString(s, "")

	// 2. 법인격 접미사 제거: "팀스파르타㈜" → "팀스파르타", "라인게임즈㈜" → "라인게임즈"
	for _, suf := range legalEntitySuffixes {
		if strings.HasSuffix(s, suf) {
			s = strings.TrimSpace(s[:len(s)-len(suf)])
			break
		}
	}

	// 3. 법인격 접두사 제거: "㈜넵튠" → "넵튠", "주식회사 컴투스" → "컴투스"
	//    "주식회사 " (공백 포함)을 먼저 검사해야 "주식회사인포바인" 오작동 방지
	for _, pre := range legalEntityPrefixes {
		if strings.HasPrefix(s, pre) {
			s = strings.TrimSpace(s[len(pre):])
			break
		}
	}

	return s
}

func (s *ClientScraper) normalizeDate(value time.Time) time.Time {
	value = value.In(s.loc)
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, s.loc)
}

func (s *ClientScraper) referenceTime(todayDate time.Time) time.Time {
	now := s.now().In(s.loc)
	if sameDate(now, todayDate) {
		return now
	}
	return time.Date(todayDate.Year(), todayDate.Month(), todayDate.Day(), 12, 0, 0, 0, s.loc)
}

func (s *ClientScraper) resolveURL(href string) (string, error) {
	if strings.TrimSpace(href) == "" {
		return "", fmt.Errorf("url is empty")
	}

	parsed, err := url.Parse(href)
	if err != nil {
		return "", err
	}
	return s.baseURL.ResolveReference(parsed).String(), nil
}

func (s *ClientScraper) origin() string {
	return s.baseURL.Scheme + "://" + s.baseURL.Host
}

func (s *ClientScraper) refererURL(dutyCode int) string {
	return s.baseURL.ResolveReference(&url.URL{
		Path: jobListPath,
		RawQuery: url.Values{
			"menucode": {"duty"},
			"duty":     {strconv.Itoa(dutyCode)},
		}.Encode(),
	}).String()
}

func defaultDutyCodePriority(code int) int {
	for index, candidate := range defaultDutyCodes {
		if candidate == code {
			return index
		}
	}
	return len(defaultDutyCodes)
}

func sameDate(left, right time.Time) bool {
	leftYear, leftMonth, leftDay := left.Date()
	rightYear, rightMonth, rightDay := right.Date()
	return leftYear == rightYear && leftMonth == rightMonth && leftDay == rightDay
}

func findFirst(node *html.Node, match func(*html.Node) bool) *html.Node {
	if node == nil {
		return nil
	}
	if match(node) {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findFirst(child, match); found != nil {
			return found
		}
	}
	return nil
}

func childElementsByTag(node *html.Node, tag string) []*html.Node {
	children := make([]*html.Node, 0)
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if isElement(child, tag) {
			children = append(children, child)
		}
	}
	return children
}

func hasAncestor(node *html.Node, match func(*html.Node) bool) bool {
	for ancestor := node.Parent; ancestor != nil; ancestor = ancestor.Parent {
		if match(ancestor) {
			return true
		}
	}
	return false
}

func hasAncestorWithClass(node *html.Node, className string) bool {
	return hasAncestor(node, func(ancestor *html.Node) bool {
		return hasClass(ancestor, className)
	})
}

func hasClass(node *html.Node, className string) bool {
	for _, candidate := range strings.Fields(attr(node, "class")) {
		if candidate == className {
			return true
		}
	}
	return false
}

func attr(node *html.Node, name string) string {
	if node == nil {
		return ""
	}
	for _, attribute := range node.Attr {
		if attribute.Key == name {
			return attribute.Val
		}
	}
	return ""
}

func isElement(node *html.Node, tag string) bool {
	return node != nil && node.Type == html.ElementNode && node.Data == tag
}

func textContent(node *html.Node) string {
	if node == nil {
		return ""
	}

	var builder strings.Builder
	var visit func(*html.Node)
	visit = func(current *html.Node) {
		if current == nil {
			return
		}
		if current.Type == html.TextNode {
			builder.WriteString(current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(node)
	return builder.String()
}
