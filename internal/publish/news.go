package publish

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/meshclaw/meshclaw/internal/assistantbrief"
)

type NewsDocumentOptions struct {
	Limit int
	Now   time.Time
	Dir   string
}

type NewsDocument struct {
	Path  string `json:"path"`
	Limit int    `json:"limit"`
	Items int    `json:"items"`
}

func SaveNewsDocument(brief assistantbrief.Brief, opts NewsDocumentOptions) (NewsDocument, error) {
	limit := opts.Limit
	if limit <= 0 || limit > 10 {
		limit = 10
	}
	dir := opts.Dir
	if strings.TrimSpace(dir) == "" {
		defaultDir, err := DefaultWorkReportsDir()
		if err != nil {
			return NewsDocument{}, err
		}
		dir = defaultDir
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return NewsDocument{}, err
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	path := filepath.Join(dir, "argos-news-"+now.Format("20060102-150405")+".md")
	if err := os.WriteFile(path, []byte(RenderNewsDocumentMarkdown(brief, limit)), 0600); err != nil {
		return NewsDocument{}, err
	}
	return NewsDocument{Path: path, Limit: limit, Items: len(brief.News)}, nil
}

func DefaultWorkReportsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Documents", "Argos Vault", "Work Reports"), nil
}

func RenderNewsDocumentMarkdown(brief assistantbrief.Brief, limit int) string {
	if limit <= 0 || limit > 10 {
		limit = 10
	}
	var b strings.Builder
	b.WriteString("# 오늘 주요뉴스\n\n")
	if !brief.Generated.IsZero() {
		fmt.Fprintf(&b, "기준: %s\n\n", brief.Generated.In(time.Local).Format("2006-01-02 15:04"))
	}
	items := brief.News
	if len(items) == 0 {
		b.WriteString("현재 수집된 주요 뉴스가 없습니다.\n")
	} else {
		for i, item := range items {
			if i >= limit {
				break
			}
			title := strings.TrimSpace(item.Title)
			if title == "" {
				title = "제목 없음"
			}
			fmt.Fprintf(&b, "## %d. %s\n\n", i+1, title)
			source := strings.TrimSpace(firstNonEmpty(item.FeedTitle, item.FeedID))
			if source != "" {
				fmt.Fprintf(&b, "출처: %s\n\n", source)
			}
			if summary := cleanNewsSummary(item.Title, firstNonEmpty(item.ArticleExcerpt, item.Description)); summary != "" {
				fmt.Fprintf(&b, "%s\n\n", summary)
			}
			if link := strings.TrimSpace(item.Link); link != "" {
				fmt.Fprintf(&b, "원문: %s\n\n", link)
			}
		}
	}
	b.WriteString("\n---\n\nMeshClaw Argos가 RSS 헤드라인을 정리했습니다.\n")
	return b.String()
}

func cleanNewsSummary(title, value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	title = strings.Join(strings.Fields(strings.TrimSpace(title)), " ")
	if value == "" || strings.EqualFold(value, title) {
		return ""
	}
	for _, sep := range []string{" - ", " | ", " — "} {
		if idx := strings.LastIndex(value, sep); idx > 40 {
			value = strings.TrimSpace(value[:idx])
		}
	}
	if len([]rune(value)) > 260 {
		runes := []rune(value)
		value = strings.TrimSpace(string(runes[:260])) + "..."
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
