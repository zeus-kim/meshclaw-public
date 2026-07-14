package publish

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meshclaw/meshclaw/internal/assistantbrief"
	"github.com/meshclaw/meshclaw/internal/newsbrief"
)

func TestRenderNewsDocumentMarkdown(t *testing.T) {
	brief := assistantbrief.Brief{
		Generated: time.Date(2026, 5, 31, 12, 34, 0, 0, time.UTC),
		News: []newsbrief.Item{
			{
				Title:          "첫 뉴스",
				FeedTitle:      "테스트 피드",
				ArticleExcerpt: "첫 뉴스 요약입니다. - 언론사",
				Link:           "https://example.com/a",
			},
		},
	}
	got := RenderNewsDocumentMarkdown(brief, 10)
	for _, want := range []string{
		"# 오늘 주요뉴스",
		"## 1. 첫 뉴스",
		"출처: 테스트 피드",
		"첫 뉴스 요약입니다.",
		"원문: https://example.com/a",
		"MeshClaw Argos가 RSS 헤드라인을 정리했습니다.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered markdown missing %q:\n%s", want, got)
		}
	}
}

func TestSaveNewsDocument(t *testing.T) {
	dir := t.TempDir()
	doc, err := SaveNewsDocument(
		assistantbrief.Brief{
			Generated: time.Date(2026, 5, 31, 12, 34, 0, 0, time.UTC),
			News:      []newsbrief.Item{{Title: "뉴스", Link: "https://example.com"}},
		},
		NewsDocumentOptions{
			Limit: 1,
			Now:   time.Date(2026, 5, 31, 12, 35, 0, 0, time.Local),
			Dir:   dir,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(dir, "argos-news-20260531-123500.md")
	if doc.Path != wantPath {
		t.Fatalf("path = %q, want %q", doc.Path, wantPath)
	}
	if doc.Items != 1 || doc.Limit != 1 {
		t.Fatalf("doc summary = %#v, want items=1 limit=1", doc)
	}
	data, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "## 1. 뉴스") {
		t.Fatalf("saved markdown missing item:\n%s", string(data))
	}
}
