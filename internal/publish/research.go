package publish

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/meshclaw/meshclaw/internal/argosreport"
	"github.com/meshclaw/meshclaw/internal/browserauto"
)

type ResearchOptions struct {
	Query   string
	Limit   int
	Timeout int
	Now     time.Time
	Dir     string
}

type ResearchReport struct {
	Query        string                   `json:"query"`
	Path         string                   `json:"path"`
	PreviewPath  string                   `json:"preview_path,omitempty"`
	PreviewImage string                   `json:"preview_image,omitempty"`
	Links        []string                 `json:"links,omitempty"`
	Search       browserauto.SearchResult `json:"search"`
	SourcePages  []browserauto.Page       `json:"source_pages,omitempty"`
}

func Research(ctx context.Context, opts ResearchOptions) (ResearchReport, error) {
	query := strings.TrimSpace(opts.Query)
	if query == "" {
		return ResearchReport{}, fmt.Errorf("query is required")
	}
	limit := opts.Limit
	if limit <= 0 || limit > 10 {
		limit = 8
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 20
	}
	search, searchErr := browserauto.Search(ctx, browserauto.SearchOptions{Query: query, Limit: limit, Timeout: timeout})
	sourcePages := FetchResearchSources(ctx, search, limit, timeout)
	doc, err := SaveResearchDocument(search, ResearchDocumentOptions{Query: query, Limit: limit, Now: opts.Now, Dir: opts.Dir, SourcePages: sourcePages})
	if err != nil {
		if searchErr != nil {
			return doc, fmt.Errorf("search failed: %v; save failed: %w", searchErr, err)
		}
		return doc, err
	}
	if searchErr != nil {
		return doc, searchErr
	}
	return doc, nil
}

type ResearchDocumentOptions struct {
	Query       string
	Limit       int
	Now         time.Time
	Dir         string
	SourcePages []browserauto.Page
}

func SaveResearchDocument(search browserauto.SearchResult, opts ResearchDocumentOptions) (ResearchReport, error) {
	query := firstNonEmpty(opts.Query, search.Query)
	if query == "" {
		query = "research"
	}
	limit := opts.Limit
	if limit <= 0 || limit > 10 {
		limit = 8
	}
	dir := opts.Dir
	if strings.TrimSpace(dir) == "" {
		defaultDir, err := DefaultWorkReportsDir()
		if err != nil {
			return ResearchReport{}, err
		}
		dir = defaultDir
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return ResearchReport{}, err
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	path := filepath.Join(dir, "argos-search-"+now.Format("20060102-150405")+".md")
	if err := os.WriteFile(path, []byte(RenderResearchMarkdownWithSources(query, search, opts.SourcePages, limit, now)), 0600); err != nil {
		return ResearchReport{}, err
	}
	preview, err := saveResearchPreviewHTML(path, query, search)
	if err != nil {
		return ResearchReport{}, err
	}
	previewImage := savePreviewImage(preview)
	links := DocumentLinks(path)
	return ResearchReport{Query: query, Path: path, PreviewPath: preview, PreviewImage: previewImage, Links: links, Search: search, SourcePages: opts.SourcePages}, nil
}

func RenderResearchMarkdown(query string, search browserauto.SearchResult, limit int, now time.Time) string {
	return RenderResearchMarkdownWithSources(query, search, nil, limit, now)
}

func RenderResearchMarkdownWithSources(query string, search browserauto.SearchResult, sourcePages []browserauto.Page, limit int, now time.Time) string {
	if limit <= 0 || limit > 10 {
		limit = 8
	}
	if now.IsZero() {
		now = time.Now()
	}
	query = strings.TrimSpace(query)
	lines := []string{
		"# 검색 기반 리서치 노트",
		"",
		"- 요청: " + query,
		"- 생성: " + now.Format(time.RFC3339),
		"- 기준: 검색 결과와 접근 가능한 원문 일부를 정리한 초안입니다. 중요한 사실은 최종 사용 전에 원문 출처를 다시 확인해야 합니다.",
		"",
		"## 출처 후보",
	}
	if search.Error != "" {
		lines = append(lines, "- 검색 오류: "+search.Error)
	} else if len(search.Results) == 0 {
		lines = append(lines, "- 검색 결과가 없습니다.")
	} else {
		for i, item := range search.Results {
			if i >= limit {
				break
			}
			title := strings.TrimSpace(item.Text)
			if title == "" {
				title = "제목 없음"
			}
			lines = append(lines, fmt.Sprintf("%d. %s", i+1, title))
			if strings.TrimSpace(item.URL) != "" {
				lines = append(lines, "   "+strings.TrimSpace(item.URL))
			}
		}
	}
	if enoughSourceText(sourcePages) {
		lines = append(lines,
			"",
			"## 핵심 요약 (원문 발췌 기반)",
		)
		for _, item := range sourceCitedFindings(sourcePages, 5) {
			lines = append(lines, "- "+item)
		}
		lines = append(lines,
			"",
			"## 출처별 근거",
		)
		for i, page := range usableSourcePages(sourcePages) {
			title := firstNonEmpty(strings.TrimSpace(page.Title), strings.TrimSpace(page.FinalURL), strings.TrimSpace(page.URL))
			lines = append(lines, fmt.Sprintf("- [S%d] %s", i+1, title))
			if strings.TrimSpace(page.FinalURL) != "" && strings.TrimSpace(page.FinalURL) != strings.TrimSpace(title) {
				lines = append(lines, "  "+strings.TrimSpace(page.FinalURL))
			} else if strings.TrimSpace(page.URL) != "" && strings.TrimSpace(page.URL) != strings.TrimSpace(title) {
				lines = append(lines, "  "+strings.TrimSpace(page.URL))
			}
		}
	} else {
		lines = append(lines,
			"",
			"## 원문 확인 상태",
			"- 검색 결과는 확보했지만, 자동으로 읽은 원문 본문이 충분하지 않아 긴 요약으로 확장하지 않았습니다.",
			"- 아래 초안은 출처 후보를 바탕으로 한 리서치 출발점입니다.",
		)
	}
	lines = append(lines,
		"",
		"## 초안 요약",
		"- 이 문서는 검색 결과를 바탕으로 작성한 리서치 출발점입니다.",
		"- 위 출처 후보를 열어 핵심 사실, 날짜, 수치, 인용문을 확인한 뒤 최종 보고서로 다듬어야 합니다.",
		"- 출처 간 내용이 다르면 더 공신력 있는 1차/공식 자료를 우선해야 합니다.",
		"",
		"## 다음에 확인할 질문",
		"- 이 주제에서 공식/1차 출처는 무엇인가?",
		"- 여러 출처가 공통으로 말하는 핵심 사실은 무엇인가?",
		"- 날짜, 인물, 수치, 사건 순서에 충돌은 없는가?",
		"- 최종 문서가 필요한 형식은 브리프, 보고서, 발표자료 중 무엇인가?",
	)
	return strings.Join(lines, "\n")
}

func FetchResearchSources(ctx context.Context, search browserauto.SearchResult, limit, timeout int) []browserauto.Page {
	if limit <= 0 || limit > 10 {
		limit = 8
	}
	if timeout <= 0 {
		timeout = 20
	}
	maxPages := 3
	if limit < maxPages {
		maxPages = limit
	}
	seen := map[string]bool{}
	pages := []browserauto.Page{}
	for _, link := range search.Results {
		if len(pages) >= maxPages {
			break
		}
		url := strings.TrimSpace(link.URL)
		if url == "" || seen[url] || !strings.HasPrefix(strings.ToLower(url), "http") {
			continue
		}
		seen[url] = true
		page, err := browserauto.Fetch(ctx, browserauto.FetchOptions{URL: url, MaxBody: 6000, Timeout: minInt(timeout, 10)})
		if err != nil && strings.TrimSpace(page.Error) == "" {
			page.Error = err.Error()
		}
		pages = append(pages, page)
	}
	return pages
}

func enoughSourceText(pages []browserauto.Page) bool {
	usable := usableSourcePages(pages)
	if len(usable) >= 2 {
		return true
	}
	return len(usable) == 1 && len([]rune(strings.TrimSpace(usable[0].Text))) >= 900
}

func usableSourcePages(pages []browserauto.Page) []browserauto.Page {
	usable := []browserauto.Page{}
	for _, page := range pages {
		text := strings.TrimSpace(page.Text)
		if page.Error != "" || page.StatusCode < 200 || page.StatusCode >= 300 || len([]rune(text)) < 300 {
			continue
		}
		usable = append(usable, page)
	}
	sort.SliceStable(usable, func(i, j int) bool {
		return researchSourceScore(usable[i]) > researchSourceScore(usable[j])
	})
	preferred := []browserauto.Page{}
	for _, page := range usable {
		if researchSourceScore(page) >= 0 {
			preferred = append(preferred, page)
		}
	}
	if len(preferred) >= 2 {
		return preferred
	}
	return usable
}

func researchSourceScore(page browserauto.Page) int {
	host := researchSourceHost(firstNonEmpty(page.FinalURL, page.URL))
	title := strings.ToLower(strings.TrimSpace(page.Title))
	score := 0
	switch {
	case strings.Contains(host, "encykorea.aks.ac.kr"):
		score += 45
	case strings.HasSuffix(host, ".go.kr") || strings.Contains(host, ".go.kr"):
		score += 40
	case strings.Contains(host, ".ac.kr") || strings.Contains(host, "wikipedia.org"):
		score += 30
	case strings.Contains(host, "support.") || strings.Contains(title, "docs") || strings.Contains(title, "documentation"):
		score += 20
	case strings.Contains(host, "github.com"):
		score += 10
	}
	if strings.Contains(host, "namu.wiki") {
		score -= 25
	}
	if strings.Contains(host, "youtube.com") || strings.Contains(host, "youtu.be") {
		score -= 25
	}
	if strings.Contains(host, "blog.") || strings.Contains(host, "tistory.com") || strings.Contains(host, "naver.com") {
		score -= 15
	}
	return score
}

func researchSourceHost(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(parsed.Host), "www.")
}

func sourceFindings(pages []browserauto.Page, limit int) []string {
	if limit <= 0 {
		limit = 4
	}
	findings := []string{}
	for i, page := range usableSourcePages(pages) {
		if len(findings) >= limit {
			break
		}
		excerpt := sourceExcerpt(page.Text, 220)
		if excerpt == "" {
			continue
		}
		title := firstNonEmpty(strings.TrimSpace(page.Title), fmt.Sprintf("출처 %d", i+1))
		findings = append(findings, fmt.Sprintf("%s: %s", title, excerpt))
	}
	return findings
}

func sourceCitedFindings(pages []browserauto.Page, limit int) []string {
	if limit <= 0 {
		limit = 5
	}
	findings := []string{}
	for i, page := range usableSourcePages(pages) {
		if len(findings) >= limit {
			break
		}
		excerpts := sourceExcerptSentences(page.Text, 2, 180)
		for _, excerpt := range excerpts {
			if len(findings) >= limit {
				break
			}
			findings = append(findings, fmt.Sprintf("%s [S%d]", excerpt, i+1))
		}
	}
	if len(findings) == 0 {
		return sourceFindings(pages, limit)
	}
	return findings
}

func sourceExcerpt(text string, maxRunes int) string {
	return strings.Join(sourceExcerptSentences(text, 3, maxRunes), " ")
}

func SourceExcerpt(text string, maxRunes int) string {
	return sourceExcerpt(text, maxRunes)
}

func sourceExcerptSentences(text string, maxSentences, maxRunes int) []string {
	text = strings.TrimSpace(strings.Join(strings.Fields(text), " "))
	if text == "" {
		return nil
	}
	candidates := splitSentences(text)
	parts := []string{}
	count := 0
	for _, sentence := range candidates {
		sentence = strings.TrimSpace(sentence)
		sentence = cleanResearchSentence(sentence)
		if len([]rune(sentence)) < 35 {
			continue
		}
		if !usefulResearchSentence(sentence) {
			continue
		}
		next := count + len([]rune(sentence))
		if next > maxRunes && len(parts) > 0 {
			break
		}
		parts = append(parts, sentence)
		count = next
		if count >= maxRunes || len(parts) >= maxSentences {
			break
		}
	}
	if len(parts) == 0 {
		fallback := truncateRunes(text, maxRunes)
		if fallback == "" {
			return nil
		}
		return []string{fallback}
	}
	return splitSentences(truncateRunes(strings.Join(parts, " "), maxRunes))
}

var wikiFootnotePattern = regexp.MustCompile(`\[\s*\d+\s*\]`)

func cleanResearchSentence(sentence string) string {
	sentence = wikiFootnotePattern.ReplaceAllString(sentence, "")
	for _, marker := range []string{"[ 편집 ]", "[편집]", "[ 펼치기 ]", "[ 접기 ]"} {
		sentence = strings.ReplaceAll(sentence, marker, " ")
	}
	sentence = strings.TrimLeft(sentence, "\ufeff\u200b\u200c\u200d ")
	sentence = trimRepeatedTitlePrefix(sentence)
	return strings.TrimSpace(strings.Join(strings.Fields(sentence), " "))
}

func trimRepeatedTitlePrefix(sentence string) string {
	sentence = strings.TrimSpace(sentence)
	separators := []string{" - Apple Support ", " | Apple Support ", " - YouTube ", " | YouTube "}
	for _, sep := range separators {
		idx := strings.Index(sentence, sep)
		if idx <= 0 {
			continue
		}
		prefix := strings.TrimSpace(sentence[:idx])
		rest := strings.TrimSpace(sentence[idx+len(sep):])
		if prefix == "" || rest == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(rest), strings.ToLower(prefix)) {
			return strings.TrimSpace(rest)
		}
	}
	return sentence
}

func usefulResearchSentence(sentence string) bool {
	lower := strings.ToLower(sentence)
	if strings.Contains(lower, "<span") || strings.Contains(lower, "</span") ||
		strings.Contains(lower, "class=") || strings.Contains(lower, "mw-parser") ||
		strings.Contains(lower, "language=") || strings.Contains(lower, "params=") ||
		strings.Contains(lower, "&amp;") || strings.Contains(lower, "%ed%") ||
		strings.Contains(sentence, "{\"") || strings.Contains(sentence, "\"}") ||
		strings.Contains(sentence, "\\u003c") || strings.Contains(sentence, "좌표") ||
		strings.Contains(sentence, "관련 글꼴") || strings.Contains(sentence, "문자가 깨진 글자") ||
		strings.Contains(sentence, "상단메뉴 바로가기") || strings.Contains(sentence, "내용 바로가기") ||
		strings.Contains(sentence, "하단정보 바로가기") || strings.Contains(sentence, "이전주제 바로가기") ||
		strings.Contains(sentence, "검색어 입력") || strings.Contains(sentence, "카드회전버튼") ||
		strings.Contains(sentence, "최근 변경") || strings.Contains(sentence, "최근 수정 시각") ||
		strings.Contains(sentence, "최근 토론") || strings.Contains(sentence, "특수 기능") ||
		strings.Contains(sentence, "편집 토론 역사") || strings.Contains(sentence, "펼치기 · 접기") ||
		strings.Contains(sentence, "관련 문서 [") || strings.Contains(sentence, "나무위키 최근") ||
		strings.Contains(sentence, "참고하십시오") || strings.Contains(sentence, "대중매체") ||
		strings.Contains(sentence, "문서를,") || strings.Contains(sentence, "문서의") ||
		strings.Contains(sentence, "YouTube 정보") || strings.Contains(sentence, "저작권 문의하기") ||
		strings.Contains(sentence, "크리에이터 광고 개발자") || strings.Contains(sentence, "개인정보처리방침") ||
		strings.Contains(sentence, "정책 및 안전") || strings.Contains(sentence, "YouTube 작동의 원리") ||
		strings.Contains(sentence, "＜") || strings.Contains(sentence, "＞") {
		return false
	}
	if strings.Count(sentence, " | ") >= 4 {
		return false
	}
	if strings.Count(sentence, "바로가기") >= 2 {
		return false
	}
	symbols := 0
	letters := 0
	for _, r := range sentence {
		switch {
		case r == '<' || r == '>' || r == '{' || r == '}' || r == '\\':
			symbols++
		case r > ' ':
			letters++
		}
	}
	return letters > 0 && symbols*12 <= letters
}

func splitSentences(text string) []string {
	replacer := strings.NewReplacer("?", ". ", "!", ". ", "。", ". ", "\n", ". ")
	text = replacer.Replace(text)
	raw := strings.Split(text, ".")
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item+".")
	}
	return out
}

func truncateRunes(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return strings.TrimSpace(string(runes[:maxRunes])) + "..."
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func saveResearchPreviewHTML(documentPath, query string, search browserauto.SearchResult) (string, error) {
	path := strings.TrimSuffix(documentPath, filepath.Ext(documentPath)) + ".html"
	findings := []string{}
	if search.Error != "" {
		findings = append(findings, "검색 오류: "+search.Error)
	} else {
		for i, item := range search.Results {
			if i >= 3 {
				break
			}
			if text := strings.TrimSpace(item.Text); text != "" {
				findings = append(findings, text)
			}
		}
	}
	htmlDoc := argosreport.RenderMobileHTML(argosreport.MobileReport{
		Eyebrow:  "Argos Browser Report",
		Title:    "검색 결과 정리",
		Subtitle: query,
		Flow:     []string{"검색 실행", "결과 수집", "중요 항목 정리", "링크 저장"},
		Findings: findings,
		Footer:   "MeshClaw Argos가 브라우저 검색 결과를 정리했습니다.",
	})
	if err := os.WriteFile(path, []byte(htmlDoc), 0600); err != nil {
		return "", err
	}
	return path, nil
}

func savePreviewImage(htmlPath string) string {
	if runtime.GOOS != "darwin" || strings.TrimSpace(htmlPath) == "" {
		return ""
	}
	out := htmlPath + ".png"
	if fileNonEmpty(out) {
		return out
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "qlmanage", "-t", "-s", "1200", "-o", filepath.Dir(htmlPath), htmlPath)
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	if err := cmd.Run(); err != nil {
		return ""
	}
	if fileNonEmpty(out) {
		return out
	}
	return ""
}
