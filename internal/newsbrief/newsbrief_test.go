package newsbrief

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildFetchesArticleExcerpt(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/feed.xml":
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(`<?xml version="1.0"?>
<rss version="2.0"><channel><title>Test Feed</title><item>
<title>Router headline only</title>
<link>` + server.URL + `/article</link>
<description>Short RSS text</description>
<pubDate>` + time.Now().UTC().Format(time.RFC1123Z) + `</pubDate>
</item></channel></rss>`))
		case "/article":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><script>secretTracker()</script></head><body>
<nav>Subscribe and sign in</nav>
<article><h1>Actual article</h1><p>The article body says the service recovered after a planned router fix.</p><p>Operators should verify evidence before repeating the action.</p></article>
</body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "feeds.json")
	t.Setenv("MESHCLAW_NEWS_FEEDS", configPath)
	cfg := Config{
		Kind: "meshclaw_news_feeds",
		Path: configPath,
		Feeds: []Feed{{
			ID:       "test",
			Title:    "Test Feed",
			Category: "tech",
			URL:      server.URL + "/feed.xml",
		}},
		UpdatedAt: time.Now().UTC(),
	}
	data := []byte(`{"kind":"meshclaw_news_feeds","feeds":[{"id":"test","title":"Test Feed","category":"tech","url":"` + server.URL + `/feed.xml"}]}`)
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	_ = cfg

	brief, err := Build(context.Background(), BriefOptions{SinceHours: 24, Limit: 1, ArticleLimit: 1, ArticleChars: 300})
	if err != nil {
		t.Fatal(err)
	}
	if len(brief.Items) != 1 {
		t.Fatalf("items=%d, want 1", len(brief.Items))
	}
	item := brief.Items[0]
	if !item.ArticleFetched {
		t.Fatalf("article was not fetched: %#v", item)
	}
	if !strings.Contains(item.ArticleExcerpt, "service recovered after a planned router fix") {
		t.Fatalf("excerpt=%q", item.ArticleExcerpt)
	}
	if strings.Contains(item.ArticleExcerpt, "secretTracker") || strings.Contains(item.ArticleExcerpt, "Subscribe") {
		t.Fatalf("excerpt kept page chrome/script: %q", item.ArticleExcerpt)
	}
}

func TestBuildDeduplicatesSimilarHeadlines(t *testing.T) {
	now := time.Now().UTC()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<rss version="2.0"><channel><title>Test Feed</title>
<item><title>Same story - Source A</title><link>` + server.URL + `/a</link><pubDate>` + now.Format(time.RFC1123Z) + `</pubDate></item>
<item><title>Same story - Source B</title><link>` + server.URL + `/b</link><pubDate>` + now.Add(-time.Minute).Format(time.RFC1123Z) + `</pubDate></item>
</channel></rss>`))
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "feeds.json")
	t.Setenv("MESHCLAW_NEWS_FEEDS", configPath)
	data := []byte(`{"kind":"meshclaw_news_feeds","feeds":[{"id":"test","title":"Test Feed","category":"tech","url":"` + server.URL + `/feed.xml"}]}`)
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	brief, err := Build(context.Background(), BriefOptions{SinceHours: 24, Limit: 10, DisableArticleFetch: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(brief.Items) != 1 {
		t.Fatalf("items=%d, want deduped 1: %#v", len(brief.Items), brief.Items)
	}
	if brief.Quality.RawItems != 2 || brief.Quality.KeptItems != 1 || brief.Quality.DroppedDuplicate != 1 {
		t.Fatalf("quality=%#v", brief.Quality)
	}
}

func TestBuildDeduplicatesSameHeadlineAcrossCategories(t *testing.T) {
	now := time.Now().UTC()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		switch r.URL.Path {
		case "/world.xml":
			_, _ = w.Write([]byte(`<?xml version="1.0"?>
<rss version="2.0"><channel><title>World</title>
<item><title>Same global story</title><link>` + server.URL + `/world</link><pubDate>` + now.Format(time.RFC1123Z) + `</pubDate></item>
</channel></rss>`))
		default:
			_, _ = w.Write([]byte(`<?xml version="1.0"?>
<rss version="2.0"><channel><title>US</title>
<item><title>Same global story</title><link>` + server.URL + `/us</link><pubDate>` + now.Add(-time.Minute).Format(time.RFC1123Z) + `</pubDate></item>
</channel></rss>`))
		}
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "feeds.json")
	t.Setenv("MESHCLAW_NEWS_FEEDS", configPath)
	data := []byte(`{"kind":"meshclaw_news_feeds","feeds":[{"id":"world","title":"World","category":"world","url":"` + server.URL + `/world.xml"},{"id":"us","title":"US","category":"overseas/us","url":"` + server.URL + `/us.xml"}]}`)
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	brief, err := Build(context.Background(), BriefOptions{SinceHours: 24, Limit: 10, DisableArticleFetch: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(brief.Items) != 1 {
		t.Fatalf("items=%d, want cross-category deduped 1: %#v", len(brief.Items), brief.Items)
	}
}

func TestBuildDropsUndatedItems(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<rss version="2.0"><channel><title>Test Feed</title>
<item><title>Undated archive story</title><link>` + server.URL + `/old</link></item>
</channel></rss>`))
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "feeds.json")
	t.Setenv("MESHCLAW_NEWS_FEEDS", configPath)
	data := []byte(`{"kind":"meshclaw_news_feeds","feeds":[{"id":"test","title":"Test Feed","category":"tech","url":"` + server.URL + `/feed.xml"}]}`)
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	brief, err := Build(context.Background(), BriefOptions{SinceHours: 24, Limit: 10, DisableArticleFetch: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(brief.Items) != 0 {
		t.Fatalf("undated items should be dropped: %#v", brief.Items)
	}
	if brief.Quality.DroppedUndated != 1 || brief.Quality.KeptItems != 0 {
		t.Fatalf("quality=%#v", brief.Quality)
	}
}
