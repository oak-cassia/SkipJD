package gamejob

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"

	"skipjd/internal/model"
)

const (
	defaultBaseURL   = "https://www.gamejob.co.kr"
	jobListPath      = "/Recruit/joblist"
	jobListQuery     = "menucode=duty&duty=1"
	jobListAjaxPath  = "/Recruit/_GI_Job_List/"
	defaultUserAgent = "Mozilla/5.0"
	clientDutyCode   = "1"
	DefaultMaxPages  = 10
	SourceName       = "browser_agent"
)

var (
	hoursAgoPattern   = regexp.MustCompile(`(\d+)\s*시간\s*전`)
	minutesAgoPattern = regexp.MustCompile(`(\d+)\s*분\s*전`)
	daysAgoPattern    = regexp.MustCompile(`(\d+)\s*일\s*전`)
	monthDayPattern   = regexp.MustCompile(`(\d{2})/(\d{2})`)
	yearRangePattern  = regexp.MustCompile(`(\d+)\s*[-~]\s*\d+\s*년`)
	yearsPattern      = regexp.MustCompile(`(\d+)\s*년`)
)

type OutputPosting struct {
	Source             string    `json:"source"`
	SourceKey          string    `json:"source_key"`
	Title              string    `json:"title"`
	Company            string    `json:"company"`
	ClosingDate        string    `json:"closing_date"`
	URL                string    `json:"url"`
	MinExperienceYears *int      `json:"min_experience_years"`
	FirstSeenAt        time.Time `json:"first_seen_at"`
	LastSeenAt         time.Time `json:"last_seen_at"`
}

type Output struct {
	Postings []OutputPosting `json:"postings"`
}

type CollectOptions struct {
	PreferredCompanies []string
	LastUpdated        time.Time
	TodayDate          time.Time
	MaxPages           int
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

func (s *ClientScraper) Collect(ctx context.Context, opts CollectOptions) ([]model.JobPosting, error) {
	if len(opts.PreferredCompanies) == 0 {
		return nil, fmt.Errorf("preferred companies are required")
	}
	if opts.LastUpdated.IsZero() {
		return nil, fmt.Errorf("last updated date is required")
	}
	if opts.TodayDate.IsZero() {
		return nil, fmt.Errorf("today date is required")
	}

	cutoffDate := s.normalizeDate(opts.LastUpdated)
	todayDate := s.normalizeDate(opts.TodayDate)
	companySet := normalizeCompanies(opts.PreferredCompanies)
	if len(companySet) == 0 {
		return nil, fmt.Errorf("preferred companies are required")
	}
	maxPages := opts.MaxPages
	if maxPages <= 0 {
		maxPages = DefaultMaxPages
	}

	collectedAt := s.now().In(s.loc)
	postings := make([]model.JobPosting, 0)
	seenURLs := make(map[string]struct{})

	for page := 1; page <= maxPages; page++ {
		htmlText, err := s.fetchPage(ctx, page)
		if err != nil {
			return nil, err
		}

		parsedPage, err := s.parseListPage(htmlText, page)
		if err != nil {
			return nil, err
		}
		if len(parsedPage.rows) == 0 {
			break
		}

		stop := false
		for _, row := range parsedPage.rows {
			observedDate, err := s.parseObservedDate(todayDate, row.modifyText)
			if err != nil {
				return nil, err
			}
			if observedDate.Before(cutoffDate) {
				stop = true
				break
			}

			if _, ok := companySet[canonicalCompanyName(row.company)]; !ok {
				continue
			}
			if _, exists := seenURLs[row.url]; exists {
				continue
			}

			minExperienceYears := intPtr(parseMinExperienceYears(row.expText))
			sourceKey, err := buildSourceKey(row.url)
			if err != nil {
				return nil, err
			}

			postings = append(postings, model.JobPosting{
				Source:             SourceName,
				SourceKey:          sourceKey,
				Title:              row.title,
				Company:            row.company,
				ClosingDate:        row.closingDate,
				URL:                row.url,
				MinExperienceYears: minExperienceYears,
				FirstSeenAt:        collectedAt,
				LastSeenAt:         collectedAt,
			})
			seenURLs[row.url] = struct{}{}
		}

		if stop || !parsedPage.hasNext {
			break
		}
	}

	return postings, nil
}

func NewOutput(postings []model.JobPosting) Output {
	items := make([]OutputPosting, 0, len(postings))
	for _, posting := range postings {
		items = append(items, OutputPosting{
			Source:             posting.Source,
			SourceKey:          posting.SourceKey,
			Title:              posting.Title,
			Company:            posting.Company,
			ClosingDate:        posting.ClosingDate,
			URL:                posting.URL,
			MinExperienceYears: cloneIntPointer(posting.MinExperienceYears),
			FirstSeenAt:        posting.FirstSeenAt,
			LastSeenAt:         posting.LastSeenAt,
		})
	}
	return Output{Postings: items}
}

func (s *ClientScraper) fetchPage(ctx context.Context, page int) (string, error) {
	form := url.Values{}
	form.Set("condition[dutyCtgr]", "0")
	form.Set("condition[duty]", clientDutyCode)
	form.Set("condition[reg_dt]", "0")
	form.Set("condition[menucode]", "duty")
	form.Set("condition[searchtype]", "B")
	form.Add("condition[dutyArr][]", clientDutyCode)
	form.Add("condition[dutyCtgrSelect][]", clientDutyCode)
	form.Add("condition[dutySelect][]", clientDutyCode)
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
	req.Header.Set("Referer", s.refererURL())

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch page %d: %w", page, err)
	}
	defer resp.Body.Close()

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
		company := normalizeSpace(textContent(companyNode))
		if company == "" {
			company = normalizeSpace(textContent(tds[0]))
		}

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

func intPtr(value int) *int {
	copied := value
	return &copied
}

func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	return intPtr(*value)
}

func normalizeCompanies(companies []string) map[string]struct{} {
	normalized := make(map[string]struct{}, len(companies))
	for _, company := range companies {
		trimmed := canonicalCompanyName(company)
		if trimmed == "" {
			continue
		}
		normalized[trimmed] = struct{}{}
	}
	return normalized
}

func canonicalCompanyName(value string) string {
	normalized := normalizeSpace(value)
	if normalized == "" {
		return ""
	}

	prefixes := []string{"㈜", "(주)", "（주）", "주식회사"}
	suffixes := []string{"㈜", "(주)", "（주）", "주식회사"}

	for {
		changed := false
		for _, prefix := range prefixes {
			if strings.HasPrefix(normalized, prefix) {
				normalized = normalizeSpace(strings.TrimPrefix(normalized, prefix))
				changed = true
			}
		}
		for _, suffix := range suffixes {
			if strings.HasSuffix(normalized, suffix) {
				normalized = normalizeSpace(strings.TrimSuffix(normalized, suffix))
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	return normalized
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

func normalizeSpace(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
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

func (s *ClientScraper) refererURL() string {
	return s.baseURL.ResolveReference(&url.URL{
		Path:     jobListPath,
		RawQuery: jobListQuery,
	}).String()
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
