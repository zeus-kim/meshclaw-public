package browserauto

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type FetchOptions struct {
	URL     string `json:"url"`
	MaxBody int    `json:"max_body,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
}

type SearchOptions struct {
	Query   string `json:"query"`
	Limit   int    `json:"limit,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
}

type Page struct {
	Kind       string    `json:"kind"`
	URL        string    `json:"url"`
	FinalURL   string    `json:"final_url"`
	Title      string    `json:"title,omitempty"`
	Text       string    `json:"text,omitempty"`
	Links      []Link    `json:"links,omitempty"`
	StatusCode int       `json:"status_code"`
	FetchedAt  time.Time `json:"fetched_at"`
	Error      string    `json:"error,omitempty"`
}

type Link struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

type SearchResult struct {
	Kind      string    `json:"kind"`
	Query     string    `json:"query"`
	Results   []Link    `json:"results"`
	FetchedAt time.Time `json:"fetched_at"`
	Error     string    `json:"error,omitempty"`
}

func Fetch(ctx context.Context, opts FetchOptions) (Page, error) {
	if opts.MaxBody <= 0 {
		opts.MaxBody = 12000
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 20
	}
	rawURL, err := normalizeURL(opts.URL)
	if err != nil {
		return Page{}, err
	}
	if readme, ok := fetchGitHubReadme(ctx, rawURL, opts); ok {
		return readme, nil
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(opts.Timeout)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Page{}, err
	}
	req.Header.Set("User-Agent", "MeshClaw browser adapter")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Page{Kind: "meshclaw_browser_fetch", URL: rawURL, FetchedAt: time.Now().UTC(), Error: err.Error()}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return Page{}, err
	}
	body := string(data)
	page := Page{
		Kind:       "meshclaw_browser_fetch",
		URL:        rawURL,
		FinalURL:   resp.Request.URL.String(),
		Title:      extractTitle(body),
		Text:       truncate(cleanHTML(body), opts.MaxBody),
		Links:      extractLinks(body, resp.Request.URL, 24),
		StatusCode: resp.StatusCode,
		FetchedAt:  time.Now().UTC(),
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		page.Error = resp.Status
	}
	return page, nil
}

func Search(ctx context.Context, opts SearchOptions) (SearchResult, error) {
	if strings.TrimSpace(opts.Query) == "" {
		return SearchResult{}, fmt.Errorf("query is required")
	}
	if opts.Limit <= 0 {
		opts.Limit = 8
	}
	searchURL := "https://duckduckgo.com/html/?q=" + url.QueryEscape(opts.Query)
	page, err := Fetch(ctx, FetchOptions{URL: searchURL, Timeout: opts.Timeout, MaxBody: 20000})
	result := SearchResult{Kind: "meshclaw_browser_search", Query: opts.Query, FetchedAt: time.Now().UTC()}
	if err != nil {
		result.Error = err.Error()
		return result, err
	}
	for _, link := range page.Links {
		link.URL = normalizeSearchResultURL(link.URL)
		if isSearchSelfLink(link.URL) || link.Text == "" {
			continue
		}
		result.Results = append(result.Results, link)
		if len(result.Results) >= opts.Limit {
			break
		}
	}
	if len(result.Results) == 0 {
		result.Results = page.Links
		if len(result.Results) > opts.Limit {
			result.Results = result.Results[:opts.Limit]
		}
	}
	return result, nil
}

func fetchGitHubReadme(ctx context.Context, rawURL string, opts FetchOptions) (Page, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil || strings.ToLower(parsed.Host) != "github.com" {
		return Page{}, false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Page{}, false
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 20
	}
	maxBody := opts.MaxBody
	if maxBody <= 0 {
		maxBody = 12000
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()
	for _, branch := range []string{"main", "master"} {
		readmeURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/README.md", url.PathEscape(parts[0]), url.PathEscape(parts[1]), branch)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, readmeURL, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "MeshClaw browser adapter")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		_ = resp.Body.Close()
		if readErr != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
			continue
		}
		text := cleanMarkdown(string(data))
		if strings.TrimSpace(text) == "" {
			continue
		}
		return Page{
			Kind:       "meshclaw_browser_fetch",
			URL:        rawURL,
			FinalURL:   rawURL,
			Title:      fmt.Sprintf("%s/%s README", parts[0], parts[1]),
			Text:       truncate(text, maxBody),
			StatusCode: resp.StatusCode,
			FetchedAt:  time.Now().UTC(),
		}, true
	}
	return Page{}, false
}

func cleanMarkdown(value string) string {
	value = regexp.MustCompile(`(?is)<!--.*?-->`).ReplaceAllString(value, " ")
	value = regexp.MustCompile("(?s)```.*?```").ReplaceAllString(value, " ")
	for _, tag := range []string{"script", "style", "svg"} {
		value = regexp.MustCompile(`(?is)<`+tag+`\b[^>]*>.*?</`+tag+`>`).ReplaceAllString(value, " ")
	}
	value = regexp.MustCompile(`(?m)^\s*!\[[^\]]*\]\([^)]+\)\s*$`).ReplaceAllString(value, " ")
	value = regexp.MustCompile(`(?is)<img\b[^>]*>`).ReplaceAllString(value, " ")
	value = regexp.MustCompile(`(?is)</?(p|div|h1|h2|h3|h4|h5|h6|br|center|em|strong|b|i)\b[^>]*>`).ReplaceAllString(value, "\n")
	value = regexp.MustCompile(`(?is)</?a\b[^>]*>`).ReplaceAllString(value, " ")
	value = regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(value, " ")
	value = regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`).ReplaceAllString(value, "$1")
	value = regexp.MustCompile(`(?m)^\s*\[[^\]]+\]:\s+\S+.*$`).ReplaceAllString(value, " ")
	value = regexp.MustCompile(`(?m)^\s{0,3}#{1,6}\s*`).ReplaceAllString(value, "")
	value = strings.ReplaceAll(value, "`", "")
	value = strings.ReplaceAll(value, "|", " ")
	return cleanText(value)
}

func normalizeSearchResultURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return raw
	}
	host := strings.ToLower(parsed.Host)
	if strings.HasSuffix(host, "duckduckgo.com") {
		if target := parsed.Query().Get("uddg"); target != "" {
			if decoded, err := url.QueryUnescape(target); err == nil && decoded != "" {
				return decoded
			}
			return target
		}
	}
	return parsed.String()
}

func isSearchSelfLink(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Host)
	if host == "duckduckgo.com" || host == "html.duckduckgo.com" || strings.HasSuffix(host, ".duckduckgo.com") {
		return true
	}
	return false
}

func normalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("url is required")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported url scheme: %s", parsed.Scheme)
	}
	return parsed.String(), nil
}

func extractTitle(body string) string {
	re := regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	match := re.FindStringSubmatch(body)
	if len(match) < 2 {
		return ""
	}
	return cleanText(match[1])
}

func extractLinks(body string, base *url.URL, limit int) []Link {
	re := regexp.MustCompile(`(?is)<a\s+[^>]*href=["']([^"']+)["'][^>]*>(.*?)</a>`)
	matches := re.FindAllStringSubmatch(body, -1)
	seen := map[string]bool{}
	var out []Link
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		href := strings.TrimSpace(html.UnescapeString(match[1]))
		if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(strings.ToLower(href), "javascript:") {
			continue
		}
		parsed, err := url.Parse(href)
		if err != nil {
			continue
		}
		absolute := base.ResolveReference(parsed).String()
		if seen[absolute] {
			continue
		}
		text := cleanText(match[2])
		if text == "" {
			text = absolute
		}
		seen[absolute] = true
		out = append(out, Link{Text: truncate(text, 160), URL: absolute})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func cleanHTML(body string) string {
	body = preferredContentHTML(body)
	body = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`).ReplaceAllString(body, " ")
	body = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`).ReplaceAllString(body, " ")
	body = regexp.MustCompile(`(?is)<noscript[^>]*>.*?</noscript>`).ReplaceAllString(body, " ")
	body = regexp.MustCompile(`(?is)<svg[^>]*>.*?</svg>`).ReplaceAllString(body, " ")
	for _, tag := range []string{"nav", "header", "footer", "aside", "dialog", "template"} {
		body = regexp.MustCompile(`(?is)<`+tag+`\b[^>]*>.*?</`+tag+`>`).ReplaceAllString(body, " ")
	}
	body = regexp.MustCompile(`(?is)<div[^>]+role=["']navigation["'][^>]*>.*?</div>`).ReplaceAllString(body, " ")
	body = regexp.MustCompile(`(?is)<br\s*/?>`).ReplaceAllString(body, "\n")
	body = regexp.MustCompile(`(?is)</(p|div|li|h1|h2|h3|section|article)>`).ReplaceAllString(body, "\n")
	body = regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(body, " ")
	return cleanText(body)
}

func preferredContentHTML(body string) string {
	candidates := []string{}
	for _, pattern := range []string{
		`(?is)<article\b[^>]*class=["'][^"']*(markdown-body|entry-content|post|article|content)[^"']*["'][^>]*>.*?</article>`,
		`(?is)<article\b[^>]*>.*?</article>`,
		`(?is)<main\b[^>]*>.*?</main>`,
		`(?is)<section\b[^>]*class=["'][^"']*(markdown-body|entry-content|post|article|content|readme)[^"']*["'][^>]*>.*?</section>`,
		`(?is)<div\b[^>]*(id|class)=["'][^"']*(readme|markdown-body|entry-content|article-body|post-content|main-content)[^"']*["'][^>]*>.*?</div>`,
	} {
		matches := regexp.MustCompile(pattern).FindAllString(body, -1)
		candidates = append(candidates, matches...)
	}
	best := ""
	bestScore := 0
	for _, candidate := range candidates {
		score := len([]rune(cleanText(regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(candidate, " "))))
		if score > bestScore {
			best = candidate
			bestScore = score
		}
	}
	if bestScore >= 200 {
		return best
	}
	return body
}

func cleanText(value string) string {
	value = html.UnescapeString(value)
	value = strings.ReplaceAll(value, "\r", "\n")
	value = strings.ReplaceAll(value, "\t", " ")
	lines := strings.Split(value, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func truncate(value string, limit int) string {
	if limit <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return strings.TrimSpace(string(runes[:limit])) + "..."
}

func JSON(v interface{}) string {
	data, _ := json.MarshalIndent(v, "", "  ")
	return string(data)
}
