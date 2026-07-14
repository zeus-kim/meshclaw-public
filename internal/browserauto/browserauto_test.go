package browserauto

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchExtractsTitleTextAndLinks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><head><title>Example Page</title><style>.x{}</style></head><body><h1>Hello</h1><p>본문입니다.</p><a href="/next">Next</a></body></html>`))
	}))
	defer server.Close()

	page, err := Fetch(context.Background(), FetchOptions{URL: server.URL, MaxBody: 200})
	if err != nil {
		t.Fatal(err)
	}
	if page.Title != "Example Page" {
		t.Fatalf("title=%q", page.Title)
	}
	if !strings.Contains(page.Text, "본문입니다.") {
		t.Fatalf("text=%q", page.Text)
	}
	if len(page.Links) != 1 || page.Links[0].URL != server.URL+"/next" {
		t.Fatalf("links=%#v", page.Links)
	}
}

func TestNormalizeURLDefaultsToHTTPS(t *testing.T) {
	got, err := normalizeURL("example.com/path")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://example.com/path" {
		t.Fatalf("got=%q", got)
	}
}

func TestCleanHTMLPrefersArticleContent(t *testing.T) {
	body := `<html><body><header>Sign in Pricing Navigation</header><main><article class="markdown-body entry-content"><h1>MeshClaw</h1><p>Bring AI to the Mesh.</p><p>Works without Internet.</p></article></main><footer>Terms Privacy</footer></body></html>`
	text := cleanHTML(body)
	if strings.Contains(text, "Sign in") || strings.Contains(text, "Pricing") || strings.Contains(text, "Terms") {
		t.Fatalf("navigation leaked into text: %q", text)
	}
	if !strings.Contains(text, "MeshClaw") || !strings.Contains(text, "Bring AI to the Mesh") {
		t.Fatalf("missing article content: %q", text)
	}
}

func TestCleanMarkdownRemovesCodeAndImages(t *testing.T) {
	got := cleanMarkdown("# Title\n![demo](demo.png)\nSee [docs](https://example.com/docs).\n```sh\nsecret\n```\nDone")
	if strings.Contains(got, "demo.png") || strings.Contains(got, "secret") {
		t.Fatalf("markdown noise leaked: %q", got)
	}
	if !strings.Contains(got, "Title") || !strings.Contains(got, "https://example.com/docs") {
		t.Fatalf("markdown content missing: %q", got)
	}
}

func TestNormalizeSearchResultURLDecodesDuckDuckGoRedirect(t *testing.T) {
	got := normalizeSearchResultURL("https://duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fdocs%3Fa%3D1&rut=abc")
	if got != "https://example.com/docs?a=1" {
		t.Fatalf("got=%q", got)
	}
}

func TestIsSearchSelfLink(t *testing.T) {
	if !isSearchSelfLink("https://html.duckduckgo.com/html/") {
		t.Fatal("expected duckduckgo html page to be treated as self link")
	}
	if isSearchSelfLink("https://example.com/") {
		t.Fatal("did not expect normal result to be treated as self link")
	}
}
