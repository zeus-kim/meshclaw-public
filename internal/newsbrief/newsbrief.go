package newsbrief

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Feed struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	Title    string `json:"title,omitempty"`
	Category string `json:"category,omitempty"`
}

type Config struct {
	Kind      string    `json:"kind"`
	Path      string    `json:"path"`
	Feeds     []Feed    `json:"feeds"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Item struct {
	FeedID         string    `json:"feed_id"`
	FeedTitle      string    `json:"feed_title,omitempty"`
	Category       string    `json:"category,omitempty"`
	Title          string    `json:"title"`
	Link           string    `json:"link,omitempty"`
	Description    string    `json:"description,omitempty"`
	ArticleExcerpt string    `json:"article_excerpt,omitempty"`
	ArticleFetched bool      `json:"article_fetched,omitempty"`
	ArticleError   string    `json:"article_error,omitempty"`
	Published      time.Time `json:"published,omitempty"`
}

type FetchError struct {
	FeedID string `json:"feed_id"`
	URL    string `json:"url"`
	Error  string `json:"error"`
}

type BriefOptions struct {
	SinceHours          int
	Limit               int
	ArticleLimit        int
	ArticleChars        int
	DisableArticleFetch bool
}

type Brief struct {
	Kind       string       `json:"kind"`
	Generated  time.Time    `json:"generated"`
	SinceHours int          `json:"since_hours"`
	Limit      int          `json:"limit"`
	Feeds      []Feed       `json:"feeds"`
	Items      []Item       `json:"items"`
	Quality    Quality      `json:"quality"`
	Errors     []FetchError `json:"errors,omitempty"`
	Text       string       `json:"text"`
}

type Quality struct {
	FeedsTotal       int                    `json:"feeds_total"`
	FeedsOK          int                    `json:"feeds_ok"`
	FeedsFailed      int                    `json:"feeds_failed"`
	RawItems         int                    `json:"raw_items"`
	KeptItems        int                    `json:"kept_items"`
	SelectedItems    int                    `json:"selected_items"`
	DroppedUndated   int                    `json:"dropped_undated"`
	DroppedOld       int                    `json:"dropped_old"`
	DroppedDuplicate int                    `json:"dropped_duplicate"`
	ByFeed           map[string]FeedQuality `json:"by_feed,omitempty"`
}

type FeedQuality struct {
	Title            string `json:"title,omitempty"`
	Category         string `json:"category,omitempty"`
	RawItems         int    `json:"raw_items"`
	KeptItems        int    `json:"kept_items"`
	SelectedItems    int    `json:"selected_items"`
	DroppedUndated   int    `json:"dropped_undated"`
	DroppedOld       int    `json:"dropped_old"`
	DroppedDuplicate int    `json:"dropped_duplicate"`
	Error            string `json:"error,omitempty"`
}

func ConfigPath() string {
	if configured := strings.TrimSpace(os.Getenv("MESHCLAW_NEWS_FEEDS")); configured != "" {
		return configured
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".meshclaw", "news-feeds.json")
	}
	return filepath.Join(home, ".meshclaw", "news-feeds.json")
}

func DefaultConfig() Config {
	return Config{
		Kind: "meshclaw_news_feeds",
		Feeds: []Feed{
			{ID: "world-ko", Title: "세계 주요 뉴스", Category: "world", URL: "https://news.google.com/rss/search?q=%EC%84%B8%EA%B3%84%20%EC%A3%BC%EC%9A%94%EB%89%B4%EC%8A%A4&hl=ko&gl=KR&ceid=KR:ko"},
			{ID: "bbc-world", Title: "BBC World", Category: "world", URL: "https://feeds.bbci.co.uk/news/world/rss.xml"},
			{ID: "aljazeera", Title: "Al Jazeera", Category: "world", URL: "https://www.aljazeera.com/xml/rss/all.xml"},
			{ID: "guardian-world", Title: "The Guardian World", Category: "world", URL: "https://www.theguardian.com/world/rss"},
			{ID: "ap-top-news", Title: "AP Top News", Category: "world", URL: "https://apnews.com/hub/ap-top-news?output=rss"},
			{ID: "us-ko", Title: "미국 주요 뉴스", Category: "overseas/us", URL: "https://news.google.com/rss/search?q=%EB%AF%B8%EA%B5%AD%20%EC%A3%BC%EC%9A%94%EB%89%B4%EC%8A%A4&hl=ko&gl=KR&ceid=KR:ko"},
			{ID: "bbc-us-canada", Title: "BBC US & Canada", Category: "overseas/us", URL: "https://feeds.bbci.co.uk/news/world/us_and_canada/rss.xml"},
			{ID: "guardian-us", Title: "The Guardian US", Category: "overseas/us", URL: "https://www.theguardian.com/us-news/rss"},
			{ID: "npr-news", Title: "NPR News", Category: "overseas/us", URL: "https://feeds.npr.org/1001/rss.xml"},
			{ID: "japan-ko", Title: "일본 주요 뉴스", Category: "overseas/japan", URL: "https://news.google.com/rss/search?q=%EC%9D%BC%EB%B3%B8%20%EC%A3%BC%EC%9A%94%EB%89%B4%EC%8A%A4&hl=ko&gl=KR&ceid=KR:ko"},
			{ID: "japan-nhk", Title: "NHK Japan", Category: "overseas/japan", URL: "https://www3.nhk.or.jp/rss/news/cat0.xml"},
			{ID: "japan-nikkei-asia", Title: "Nikkei Asia", Category: "overseas/japan", URL: "https://asia.nikkei.com/rss/feed/nar"},
			{ID: "guardian-japan", Title: "The Guardian Japan", Category: "overseas/japan", URL: "https://www.theguardian.com/world/japan/rss"},
			{ID: "china-ko", Title: "중국 주요 뉴스", Category: "overseas/china", URL: "https://news.google.com/rss/search?q=%EC%A4%91%EA%B5%AD%20%EC%A3%BC%EC%9A%94%EB%89%B4%EC%8A%A4&hl=ko&gl=KR&ceid=KR:ko"},
			{ID: "china-daily", Title: "China Daily", Category: "overseas/china", URL: "https://www.chinadaily.com.cn/rss/china_rss.xml"},
			{ID: "guardian-china", Title: "The Guardian China", Category: "overseas/china", URL: "https://www.theguardian.com/world/china/rss"},
			{ID: "thailand-ko", Title: "태국 주요 뉴스", Category: "overseas/thailand", URL: "https://news.google.com/rss/search?q=%ED%83%9C%EA%B5%AD%20%EC%A3%BC%EC%9A%94%EB%89%B4%EC%8A%A4&hl=ko&gl=KR&ceid=KR:ko"},
			{ID: "thailand-bangkok-post", Title: "Bangkok Post Thailand", Category: "overseas/thailand", URL: "https://www.bangkokpost.com/rss/data/thailand.xml"},
			{ID: "vietnam-ko", Title: "베트남 주요 뉴스", Category: "overseas/vietnam", URL: "https://news.google.com/rss/search?q=%EB%B2%A0%ED%8A%B8%EB%82%A8%20%EC%A3%BC%EC%9A%94%EB%89%B4%EC%8A%A4&hl=ko&gl=KR&ceid=KR:ko"},
			{ID: "vietnam-vnexpress", Title: "VnExpress International", Category: "overseas/vietnam", URL: "https://e.vnexpress.net/rss/news.rss"},
			{ID: "hn", Title: "Hacker News", Category: "tech", URL: "https://news.ycombinator.com/rss"},
			{ID: "cloudflare", Title: "Cloudflare Blog", Category: "infra", URL: "https://blog.cloudflare.com/rss/"},
			{ID: "tailscale", Title: "Tailscale Blog", Category: "infra", URL: "https://tailscale.com/blog/index.xml"},
			{ID: "kubernetes", Title: "Kubernetes Blog", Category: "ops", URL: "https://kubernetes.io/feed.xml"},
		},
	}
}

func Init(force bool) (Config, error) {
	path := ConfigPath()
	if !force {
		if _, err := os.Stat(path); err == nil {
			return Load()
		}
	}
	cfg := DefaultConfig()
	cfg.Path = path
	cfg.UpdatedAt = time.Now().UTC()
	if err := writeConfig(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Load() (Config, error) {
	path := ConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	cfg.Kind = "meshclaw_news_feeds"
	cfg.Path = path
	return cfg, nil
}

func Add(feed Feed) (Config, error) {
	cfg, err := Load()
	if err != nil {
		if os.IsNotExist(err) {
			cfg = DefaultConfig()
			cfg.Feeds = nil
			cfg.Path = ConfigPath()
		} else {
			return Config{}, err
		}
	}
	feed.ID = sanitizeID(feed.ID)
	feed.URL = strings.TrimSpace(feed.URL)
	feed.Title = strings.TrimSpace(feed.Title)
	feed.Category = strings.TrimSpace(feed.Category)
	if feed.ID == "" || feed.URL == "" {
		return Config{}, fmt.Errorf("feed id and url are required")
	}
	found := false
	for i := range cfg.Feeds {
		if cfg.Feeds[i].ID == feed.ID {
			cfg.Feeds[i] = feed
			found = true
			break
		}
	}
	if !found {
		cfg.Feeds = append(cfg.Feeds, feed)
	}
	cfg.Path = ConfigPath()
	cfg.Kind = "meshclaw_news_feeds"
	cfg.UpdatedAt = time.Now().UTC()
	return cfg, writeConfig(cfg)
}

func Remove(id string) (Config, bool, error) {
	cfg, err := Load()
	if err != nil {
		return Config{}, false, err
	}
	id = sanitizeID(id)
	out := make([]Feed, 0, len(cfg.Feeds))
	removed := false
	for _, feed := range cfg.Feeds {
		if feed.ID == id {
			removed = true
			continue
		}
		out = append(out, feed)
	}
	cfg.Feeds = out
	cfg.UpdatedAt = time.Now().UTC()
	return cfg, removed, writeConfig(cfg)
}

func Build(ctx context.Context, opts BriefOptions) (Brief, error) {
	cfg, err := Load()
	if err != nil {
		if os.IsNotExist(err) {
			cfg, err = Init(false)
		}
		if err != nil {
			return Brief{}, err
		}
	}
	if opts.SinceHours <= 0 {
		opts.SinceHours = 24
	}
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	cutoff := time.Now().Add(-time.Duration(opts.SinceHours) * time.Hour)
	brief := Brief{
		Kind:       "meshclaw_news_brief",
		Generated:  time.Now().UTC(),
		SinceHours: opts.SinceHours,
		Limit:      opts.Limit,
		Feeds:      cfg.Feeds,
		Quality: Quality{
			FeedsTotal: len(cfg.Feeds),
			ByFeed:     map[string]FeedQuality{},
		},
	}
	seen := map[string]bool{}
	results := fetchFeeds(ctx, cfg.Feeds)
	for _, result := range results {
		feed := result.feed
		feedQuality := FeedQuality{Title: feed.Title, Category: feed.Category}
		if result.err != nil {
			brief.Errors = append(brief.Errors, FetchError{FeedID: feed.ID, URL: feed.URL, Error: result.err.Error()})
			brief.Quality.FeedsFailed++
			feedQuality.Error = result.err.Error()
			brief.Quality.ByFeed[feed.ID] = feedQuality
			continue
		}
		brief.Quality.FeedsOK++
		items := result.items
		brief.Quality.RawItems += len(items)
		feedQuality.RawItems = len(items)
		for _, item := range items {
			if item.Published.IsZero() {
				brief.Quality.DroppedUndated++
				feedQuality.DroppedUndated++
				continue
			}
			if item.Published.Before(cutoff) {
				brief.Quality.DroppedOld++
				feedQuality.DroppedOld++
				continue
			}
			key := newsDuplicateKey(item)
			if key == "" || seen[key] {
				brief.Quality.DroppedDuplicate++
				feedQuality.DroppedDuplicate++
				continue
			}
			seen[key] = true
			brief.Items = append(brief.Items, item)
			brief.Quality.KeptItems++
			feedQuality.KeptItems++
		}
		brief.Quality.ByFeed[feed.ID] = feedQuality
	}
	sort.SliceStable(brief.Items, func(i, j int) bool {
		si, sj := itemSourceScore(brief.Items[i]), itemSourceScore(brief.Items[j])
		if si != sj {
			return si > sj
		}
		a, b := brief.Items[i].Published, brief.Items[j].Published
		if a.IsZero() && b.IsZero() {
			return brief.Items[i].Title < brief.Items[j].Title
		}
		if a.IsZero() {
			return false
		}
		if b.IsZero() {
			return true
		}
		return a.After(b)
	})
	if !opts.DisableArticleFetch {
		articleWindow := opts.ArticleLimit
		if articleWindow < opts.Limit*3 {
			articleWindow = opts.Limit * 3
		}
		if articleWindow > 48 {
			articleWindow = 48
		}
		if articleWindow > len(brief.Items) {
			articleWindow = len(brief.Items)
		}
		if articleWindow > 0 {
			articleOpts := opts
			articleOpts.ArticleLimit = articleWindow
			enrichArticles(ctx, brief.Items[:articleWindow], articleOpts)
		}
	}
	if len(brief.Items) > opts.Limit {
		brief.Items = brief.Items[:opts.Limit]
	}
	brief.Quality.SelectedItems = len(brief.Items)
	for _, item := range brief.Items {
		feedQuality := brief.Quality.ByFeed[item.FeedID]
		feedQuality.SelectedItems++
		brief.Quality.ByFeed[item.FeedID] = feedQuality
	}
	brief.Text = formatBrief(brief)
	return brief, nil
}

func newsDuplicateKey(item Item) string {
	title := normalizedNewsTitle(item.Title)
	if title != "" {
		return title
	}
	return strings.ToLower(strings.TrimSpace(item.Link))
}

func normalizedNewsTitle(value string) string {
	value = strings.ToLower(clean(value))
	if idx := strings.LastIndex(value, " - "); idx > 0 {
		value = value[:idx]
	}
	replacer := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "", "-", "", "–", "", "—", "", "·", "", "|", "", ":", "", "：", "", "'", "", "\"", "", "‘", "", "’", "", "“", "", "”", "", "\u00a0", "")
	return strings.TrimSpace(replacer.Replace(value))
}

func itemSourceScore(item Item) int {
	id := strings.ToLower(strings.TrimSpace(item.FeedID))
	link := strings.ToLower(strings.TrimSpace(item.Link))
	switch {
	case strings.Contains(id, "bbc"), strings.Contains(id, "guardian"), strings.Contains(id, "ap-top-news"), strings.Contains(id, "npr"), strings.Contains(id, "aljazeera"):
		return 35
	case strings.Contains(id, "nhk"), strings.Contains(id, "nikkei"), strings.Contains(id, "bangkok-post"), strings.Contains(id, "vnexpress"):
		return 30
	case strings.Contains(id, "cloudflare"), strings.Contains(id, "tailscale"), strings.Contains(id, "kubernetes"):
		return 25
	case id == "hn":
		return 15
	case strings.Contains(link, "news.google.com/rss/articles") || strings.HasSuffix(id, "-ko"):
		return 5
	default:
		return 10
	}
}

func writeConfig(cfg Config) error {
	if cfg.Path == "" {
		cfg.Path = ConfigPath()
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cfg.Path, append(data, '\n'), 0600)
}

type feedResult struct {
	index int
	feed  Feed
	items []Item
	err   error
}

func fetchFeeds(ctx context.Context, feeds []Feed) []feedResult {
	results := make([]feedResult, len(feeds))
	if len(feeds) == 0 {
		return results
	}
	concurrency := feedFetchConcurrency()
	if concurrency > len(feeds) {
		concurrency = len(feeds)
	}
	sem := make(chan struct{}, concurrency)
	out := make(chan feedResult, len(feeds))
	var wg sync.WaitGroup
	for i, feed := range feeds {
		i, feed := i, feed
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				out <- feedResult{index: i, feed: feed, err: ctx.Err()}
				return
			}
			items, err := fetchFeed(ctx, feed)
			out <- feedResult{index: i, feed: feed, items: items, err: err}
		}()
	}
	wg.Wait()
	close(out)
	for result := range out {
		results[result.index] = result
	}
	return results
}

func feedFetchConcurrency() int {
	value := strings.TrimSpace(os.Getenv("MESHCLAW_NEWS_FEED_CONCURRENCY"))
	if value != "" {
		if n, err := strconv.Atoi(value); err == nil && n > 0 {
			if n > 24 {
				return 24
			}
			return n
		}
	}
	return 8
}

func fetchFeed(ctx context.Context, feed Feed) ([]Item, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feed.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "MeshClaw news brief")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("feed returned %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	items, err := parseRSS(data, feed)
	if err == nil && len(items) > 0 {
		return items, nil
	}
	return parseAtom(data, feed)
}

func parseRSS(data []byte, feed Feed) ([]Item, error) {
	var doc struct {
		Channel struct {
			Title string `xml:"title"`
			Items []struct {
				Title       string `xml:"title"`
				Link        string `xml:"link"`
				Description string `xml:"description"`
				PubDate     string `xml:"pubDate"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(doc.Channel.Items))
	for _, raw := range doc.Channel.Items {
		out = append(out, Item{
			FeedID:      feed.ID,
			FeedTitle:   firstNonEmpty(feed.Title, doc.Channel.Title, feed.ID),
			Category:    feed.Category,
			Title:       clean(raw.Title),
			Link:        strings.TrimSpace(raw.Link),
			Description: clean(raw.Description),
			Published:   parseTime(raw.PubDate),
		})
	}
	return out, nil
}

func parseAtom(data []byte, feed Feed) ([]Item, error) {
	var doc struct {
		Title   string `xml:"title"`
		Entries []struct {
			Title   string `xml:"title"`
			Summary string `xml:"summary"`
			Content string `xml:"content"`
			Updated string `xml:"updated"`
			Links   []struct {
				Href string `xml:"href,attr"`
				Rel  string `xml:"rel,attr"`
			} `xml:"link"`
		} `xml:"entry"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(doc.Entries))
	for _, raw := range doc.Entries {
		link := ""
		for _, candidate := range raw.Links {
			if candidate.Rel == "" || candidate.Rel == "alternate" {
				link = candidate.Href
				break
			}
		}
		out = append(out, Item{
			FeedID:      feed.ID,
			FeedTitle:   firstNonEmpty(feed.Title, doc.Title, feed.ID),
			Category:    feed.Category,
			Title:       clean(raw.Title),
			Link:        strings.TrimSpace(link),
			Description: clean(firstNonEmpty(raw.Summary, raw.Content)),
			Published:   parseTime(raw.Updated),
		})
	}
	return out, nil
}

func formatBrief(brief Brief) string {
	var b strings.Builder
	fmt.Fprintf(&b, "MeshClaw News Brief\n")
	fmt.Fprintf(&b, "Window: last %dh | items: %d | feeds: %d\n\n", brief.SinceHours, len(brief.Items), len(brief.Feeds))
	if brief.Quality.RawItems > 0 || brief.Quality.DroppedUndated > 0 || brief.Quality.DroppedOld > 0 || brief.Quality.DroppedDuplicate > 0 {
		fmt.Fprintf(&b, "Quality: raw=%d kept=%d selected=%d dropped_undated=%d dropped_old=%d dropped_duplicate=%d\n\n",
			brief.Quality.RawItems,
			brief.Quality.KeptItems,
			brief.Quality.SelectedItems,
			brief.Quality.DroppedUndated,
			brief.Quality.DroppedOld,
			brief.Quality.DroppedDuplicate,
		)
	}
	if len(brief.Items) == 0 {
		b.WriteString("No matching news items were found.\n")
	} else {
		grouped := map[string][]Item{}
		var categories []string
		for _, item := range brief.Items {
			category := firstNonEmpty(item.Category, "general")
			if _, ok := grouped[category]; !ok {
				categories = append(categories, category)
			}
			grouped[category] = append(grouped[category], item)
		}
		sort.Strings(categories)
		for _, category := range categories {
			fmt.Fprintf(&b, "[%s]\n", category)
			for _, item := range grouped[category] {
				fmt.Fprintf(&b, "- %s", item.Title)
				if item.FeedTitle != "" {
					fmt.Fprintf(&b, " (%s)", item.FeedTitle)
				}
				if !item.Published.IsZero() {
					fmt.Fprintf(&b, " - %s", item.Published.Format("Jan 2 15:04"))
				}
				b.WriteByte('\n')
				if item.Description != "" {
					fmt.Fprintf(&b, "  %s\n", truncate(item.Description, 180))
				}
				if item.ArticleExcerpt != "" {
					fmt.Fprintf(&b, "  article: %s\n", truncate(item.ArticleExcerpt, 220))
				}
				if item.Link != "" {
					fmt.Fprintf(&b, "  %s\n", item.Link)
				}
			}
			b.WriteByte('\n')
		}
	}
	if len(brief.Errors) > 0 {
		b.WriteString("Fetch issues:\n")
		for _, issue := range brief.Errors {
			fmt.Fprintf(&b, "- %s: %s\n", issue.FeedID, issue.Error)
		}
	}
	return strings.TrimSpace(b.String())
}

func parseTime(value string) time.Time {
	value = strings.TrimSpace(value)
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC3339, time.RFC3339Nano, "Mon, 02 Jan 2006 15:04:05 MST", "2006-01-02T15:04:05Z07:00"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t
		}
	}
	return time.Time{}
}

func clean(value string) string {
	value = html.UnescapeString(value)
	value = stripTags(value)
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\t", " ")
	for strings.Contains(value, "  ") {
		value = strings.ReplaceAll(value, "  ", " ")
	}
	return strings.TrimSpace(value)
}

func enrichArticles(ctx context.Context, items []Item, opts BriefOptions) {
	limit := opts.ArticleLimit
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	if limit > 12 {
		limit = 12
	}
	chars := opts.ArticleChars
	if chars <= 0 {
		chars = 900
	}
	type job struct {
		index int
		link  string
	}
	type result struct {
		index   int
		excerpt string
		err     error
	}
	jobs := make(chan job)
	results := make(chan result, limit)
	workers := 4
	if limit < workers {
		workers = limit
	}
	if workers <= 0 {
		return
	}
	for w := 0; w < workers; w++ {
		go func() {
			for j := range jobs {
				excerpt, err := fetchArticleExcerpt(ctx, j.link, chars)
				results <- result{index: j.index, excerpt: excerpt, err: err}
			}
		}()
	}
	submitted := 0
	for i := range items {
		if submitted >= limit {
			break
		}
		link := strings.TrimSpace(items[i].Link)
		if link == "" || !strings.HasPrefix(strings.ToLower(link), "http") {
			continue
		}
		submitted++
		jobs <- job{index: i, link: link}
	}
	close(jobs)
	for i := 0; i < submitted; i++ {
		select {
		case <-ctx.Done():
			return
		case r := <-results:
			if r.err != nil {
				items[r.index].ArticleError = r.err.Error()
				continue
			}
			if !validArticleExcerpt(r.excerpt) {
				items[r.index].ArticleError = "article excerpt was too short or looked like a wrapper page"
				continue
			}
			items[r.index].ArticleFetched = true
			items[r.index].ArticleExcerpt = r.excerpt
		}
	}
}

func EnrichArticles(ctx context.Context, items []Item, opts BriefOptions) {
	enrichArticles(ctx, items, opts)
}

func fetchArticleExcerpt(ctx context.Context, link string, chars int) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "MeshClaw news article fetch")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("article returned %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 3<<20))
	if err != nil {
		return "", err
	}
	htmlText := string(data)
	text := articleTextFromHTML(htmlText)
	if !validArticleExcerpt(text) {
		if meta := extractMetaDescription(htmlText); validArticleExcerpt(meta) {
			text = meta
		}
	}
	if text == "" {
		return "", fmt.Errorf("article text empty")
	}
	return truncate(text, chars), nil
}

func validArticleExcerpt(value string) bool {
	value = strings.TrimSpace(value)
	if len([]rune(value)) < 80 {
		return false
	}
	lower := strings.ToLower(value)
	if lower == "google news" || strings.HasPrefix(lower, "google news ") {
		return false
	}
	if strings.Count(lower, "google news") >= 2 {
		return false
	}
	if strings.Contains(lower, "comprehensive up-to-date news coverage") && strings.Contains(lower, "google news") {
		return false
	}
	if strings.Contains(lower, "the most read vietnamese newspaper") || strings.Contains(lower, "follow us on edition:") {
		return false
	}
	if strings.Contains(lower, "home news politics education environment traffic crime") {
		return false
	}
	return true
}

func extractMetaDescription(value string) string {
	lower := strings.ToLower(value)
	for {
		idx := strings.Index(lower, "<meta")
		if idx < 0 {
			return ""
		}
		endRel := strings.Index(lower[idx:], ">")
		if endRel < 0 {
			return ""
		}
		tag := value[idx : idx+endRel+1]
		tagLower := lower[idx : idx+endRel+1]
		lower = lower[idx+endRel+1:]
		value = value[idx+endRel+1:]
		if !strings.Contains(tagLower, "description") {
			continue
		}
		content := htmlAttrValue(tag, "content")
		if content == "" {
			continue
		}
		return clean(content)
	}
}

func htmlAttrValue(tag, name string) string {
	lower := strings.ToLower(tag)
	needle := strings.ToLower(name) + "="
	idx := strings.Index(lower, needle)
	if idx < 0 {
		return ""
	}
	start := idx + len(needle)
	if start >= len(tag) {
		return ""
	}
	quote := tag[start]
	if quote == '"' || quote == '\'' {
		end := strings.IndexByte(tag[start+1:], quote)
		if end < 0 {
			return ""
		}
		return html.UnescapeString(tag[start+1 : start+1+end])
	}
	end := start
	for end < len(tag) && tag[end] != ' ' && tag[end] != '>' {
		end++
	}
	return html.UnescapeString(tag[start:end])
}

func articleTextFromHTML(value string) string {
	value = removeHTMLComments(value)
	for _, tag := range []string{"script", "style", "noscript", "svg", "header", "footer", "nav", "form"} {
		value = removeHTMLBlock(value, tag)
	}
	if block := extractTagBlock(value, "article"); block != "" {
		value = block
	} else if block := extractTagBlock(value, "main"); block != "" {
		value = block
	} else if block := extractTagBlock(value, "body"); block != "" {
		value = block
	}
	value = strings.NewReplacer("</p>", "\n", "</div>", "\n", "</li>", "\n", "<br>", "\n", "<br/>", "\n", "<br />", "\n").Replace(value)
	value = clean(value)
	return normalizeArticleText(value)
}

func removeHTMLComments(value string) string {
	for {
		start := strings.Index(value, "<!--")
		if start < 0 {
			return value
		}
		end := strings.Index(value[start+4:], "-->")
		if end < 0 {
			return value[:start]
		}
		value = value[:start] + " " + value[start+4+end+3:]
	}
}

func removeHTMLBlock(value, tag string) string {
	openNeedle := "<" + tag
	closeNeedle := "</" + tag + ">"
	for {
		lower := strings.ToLower(value)
		start := strings.Index(lower, openNeedle)
		if start < 0 {
			return value
		}
		endRel := strings.Index(lower[start:], closeNeedle)
		if endRel < 0 {
			return value[:start]
		}
		end := start + endRel + len(closeNeedle)
		value = value[:start] + " " + value[end:]
	}
}

func extractTagBlock(value, tag string) string {
	lower := strings.ToLower(value)
	start := strings.Index(lower, "<"+tag)
	if start < 0 {
		return ""
	}
	openEnd := strings.Index(lower[start:], ">")
	if openEnd < 0 {
		return ""
	}
	contentStart := start + openEnd + 1
	closeStartRel := strings.Index(lower[contentStart:], "</"+tag+">")
	if closeStartRel < 0 {
		return ""
	}
	return value[contentStart : contentStart+closeStartRel]
}

func normalizeArticleText(value string) string {
	value = strings.ReplaceAll(value, "\u00a0", " ")
	for _, noise := range []string{"cookie", "cookies", "subscribe", "sign in", "log in", "advertisement", "enable javascript"} {
		lower := strings.ToLower(value)
		for {
			idx := strings.Index(lower, noise)
			if idx < 0 {
				break
			}
			lineStart := strings.LastIndex(value[:idx], ".")
			if lineStart < 0 {
				lineStart = 0
			}
			lineEndRel := strings.Index(value[idx:], ".")
			if lineEndRel < 0 || lineEndRel > 180 {
				break
			}
			value = strings.TrimSpace(value[:lineStart]) + " " + strings.TrimSpace(value[idx+lineEndRel+1:])
			lower = strings.ToLower(value)
		}
	}
	for strings.Contains(value, "  ") {
		value = strings.ReplaceAll(value, "  ", " ")
	}
	return strings.TrimSpace(value)
}

func stripTags(value string) string {
	var b strings.Builder
	inTag := false
	for _, r := range value {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
			b.WriteByte(' ')
		default:
			if !inTag {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

func truncate(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len([]rune(value)) <= limit {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:limit])) + "..."
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func sanitizeID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else if b.Len() > 0 {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
