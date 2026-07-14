package publish

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meshclaw/meshclaw/internal/browserauto"
)

func TestSaveResearchDocument(t *testing.T) {
	dir := t.TempDir()
	report, err := SaveResearchDocument(
		browserauto.SearchResult{
			Query: "meshclaw",
			Results: []browserauto.Link{
				{Text: "MeshClaw Docs", URL: "https://example.com/docs"},
				{Text: "MeshClaw Runtime", URL: "https://example.com/runtime"},
			},
		},
		ResearchDocumentOptions{
			Limit: 1,
			Now:   time.Date(2026, 5, 31, 13, 0, 0, 0, time.Local),
			Dir:   dir,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(dir, "argos-search-20260531-130000.md")
	if report.Path != wantPath {
		t.Fatalf("path = %q, want %q", report.Path, wantPath)
	}
	if report.PreviewPath != filepath.Join(dir, "argos-search-20260531-130000.html") {
		t.Fatalf("preview = %q", report.PreviewPath)
	}
	data, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "# 검색 기반 리서치 노트") ||
		!strings.Contains(text, "## 출처 후보") ||
		!strings.Contains(text, "중요한 사실은 최종 사용 전에 원문 출처를 다시 확인해야 합니다") ||
		!strings.Contains(text, "MeshClaw Docs") {
		t.Fatalf("saved markdown missing expected content:\n%s", text)
	}
	if strings.Contains(text, "MeshClaw Runtime") {
		t.Fatalf("limit was not applied:\n%s", text)
	}
	if _, err := os.Stat(report.PreviewPath); err != nil {
		t.Fatal(err)
	}
}

func TestRenderResearchMarkdownWithSourcesAddsGroundedSummary(t *testing.T) {
	longText := strings.Repeat("세종대왕은 조선의 네 번째 왕으로 훈민정음 창제와 과학 기술 진흥에 중요한 역할을 했습니다. ", 8)
	text := RenderResearchMarkdownWithSources(
		"세종대왕",
		browserauto.SearchResult{Query: "세종대왕", Results: []browserauto.Link{{Text: "세종대왕 자료", URL: "https://example.com/sejong"}}},
		[]browserauto.Page{
			{URL: "https://example.com/1", FinalURL: "https://example.com/1", Title: "세종대왕 공식 자료", Text: longText, StatusCode: 200},
			{URL: "https://example.com/2", FinalURL: "https://example.com/2", Title: "훈민정음 자료", Text: longText, StatusCode: 200},
		},
		5,
		time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC),
	)
	if !strings.Contains(text, "## 핵심 요약 (원문 발췌 기반)") ||
		!strings.Contains(text, "[S1]") ||
		!strings.Contains(text, "## 출처별 근거") ||
		!strings.Contains(text, "[S1] 세종대왕 공식 자료") {
		t.Fatalf("missing source-grounded summary:\n%s", text)
	}
	if strings.Contains(text, "자동으로 읽은 원문 본문이 충분하지 않아") {
		t.Fatalf("should not show insufficient-source warning:\n%s", text)
	}
}

func TestRenderResearchMarkdownWithSourcesStaysConservativeWhenSourcesAreThin(t *testing.T) {
	text := RenderResearchMarkdownWithSources(
		"세종대왕",
		browserauto.SearchResult{Query: "세종대왕", Results: []browserauto.Link{{Text: "세종대왕 자료", URL: "https://example.com/sejong"}}},
		[]browserauto.Page{{URL: "https://example.com/1", FinalURL: "https://example.com/1", Title: "짧은 자료", Text: "짧은 본문", StatusCode: 200}},
		5,
		time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC),
	)
	if !strings.Contains(text, "## 원문 확인 상태") ||
		!strings.Contains(text, "충분하지 않아 긴 요약으로 확장하지 않았습니다") {
		t.Fatalf("missing conservative warning:\n%s", text)
	}
	if strings.Contains(text, "## 핵심 요약 (원문 발췌 기반)") {
		t.Fatalf("should not add source-grounded summary for thin source:\n%s", text)
	}
}

func TestSourceExcerptSkipsMarkupNoise(t *testing.T) {
	text := `</span>"}'> 워드 프로세서에 대해서는 훈민정음 문서를 참고하십시오. {"그림":{"wt":"Hunminjeongum.jpg"}}
language=ko&pagename=%ED%9B%88%EB%AF%BC%EC%A0%95%EC%9D%8C&params=37_35_37 관련 글꼴 이 설치되지 않은 경우, 일부 문자가 깨진 글자 로 표시될 수 있습니다.
기록으로 만나는 대한민국 상단메뉴 바로가기 메뉴 바로가기 내용 바로가기 하단정보 바로가기 이전주제 바로가기 국가기록원 바로가기 검색하기 검색어 입력.
카드회전버튼 세계가 인정하는 문자, 훈민정음 ＜세종대왕 동상(서울 종로구)＞ 그 소식 들었나.
훈민정음은 한글의 옛 이름으로 세종대왕이 창제한 문자의 명칭이자 창제 원리와 사용법을 해설한 책의 제목이기도 하다.
세종 25년에 창제된 뒤 세종 28년에 반포되었고, 백성이 쉽게 읽고 쓸 수 있도록 만든 문자로 설명된다.`
	got := sourceExcerpt(text, 220)
	if strings.Contains(got, "</span>") || strings.Contains(got, "{\"") || strings.Contains(got, "language=ko") || strings.Contains(got, "관련 글꼴") || strings.Contains(got, "바로가기") || strings.Contains(got, "카드회전버튼") {
		t.Fatalf("excerpt kept markup noise: %q", got)
	}
	if !strings.Contains(got, "훈민정음은 한글의 옛 이름") {
		t.Fatalf("excerpt missed useful sentence: %q", got)
	}
}

func TestSourceExcerptSkipsNavigationMetadata(t *testing.T) {
	text := `훈민정음 - 나무위키 최근 변경 최근 토론 특수 기능 훈민정음 최근 수정 시각: 2026-05-25 08:26:59 92 편집 토론 역사 분류 훈민정음 조선의 도서 1440년대 도서 한글 세종(조선) 조선 국왕 관련 문서 [ 펼치기 · 접기 ] 태조 생애 | 건원릉 | 위화도 회군 | 왕씨 몰살 | 이성계 여진족설 | 조.
10 만세 운동 | 유릉 | 대중매체 이 문자의 현재 이름에 대한 내용은 한글 문서를, 삼성전자의 워드프로세서에 대한 내용은 훈민정음(오피스) 문서를, 술 게임에 대한 내용은 술 게임/두뇌 게임 문서의 훈민정음 부분을 참고하십시오.
훈민정음은 한글의 옛 이름으로, 세종대왕이 창제한 문자의 명칭이자 창제 원리와 사용법을 설명한 책의 제목이기도 하다.
세종 25년에 창제되고 세종 28년에 반포되었다는 설명은 여러 사료 확인의 출발점이 된다.`
	got := sourceExcerpt(text, 260)
	for _, bad := range []string{"최근 변경", "최근 수정 시각", "편집 토론 역사", "펼치기", "위화도 회군", "대중매체", "참고하십시오"} {
		if strings.Contains(got, bad) {
			t.Fatalf("excerpt kept navigation metadata %q: %q", bad, got)
		}
	}
	if !strings.Contains(got, "훈민정음은 한글의 옛 이름") {
		t.Fatalf("excerpt missed useful sentence: %q", got)
	}
}

func TestUsableSourcePagesPrefersEncyclopedicSources(t *testing.T) {
	longText := strings.Repeat("훈민정음은 세종대왕이 백성을 위해 만든 문자와 해설서로 설명되며, 창제와 반포 시기 및 제작 취지를 확인하려면 신뢰할 수 있는 원문 출처를 대조해야 한다. ", 8)
	pages := usableSourcePages([]browserauto.Page{
		{URL: "https://namu.wiki/w/훈민정음", FinalURL: "https://namu.wiki/w/훈민정음", Title: "훈민정음 - 나무위키", Text: longText, StatusCode: 200},
		{URL: "https://ko.wikipedia.org/wiki/훈민정음", FinalURL: "https://ko.wikipedia.org/wiki/훈민정음", Title: "훈민정음 - 위키백과", Text: longText, StatusCode: 200},
		{URL: "https://encykorea.aks.ac.kr/Article/E0061508", FinalURL: "https://encykorea.aks.ac.kr/Article/E0061508", Title: "한글 - 한국민족문화대백과사전", Text: longText, StatusCode: 200},
	})
	if len(pages) != 2 {
		t.Fatalf("expected 2 preferred usable pages, got %d", len(pages))
	}
	if !strings.Contains(pages[0].FinalURL, "encykorea.aks.ac.kr") {
		t.Fatalf("expected encyclopedic source first: %#v", pages)
	}
	for _, page := range pages {
		if strings.Contains(page.FinalURL, "namu.wiki") {
			t.Fatalf("expected noisy negative source to be dropped when enough preferred sources exist: %#v", pages)
		}
	}
}

func TestUsableSourcePagesDropsYouTubeWhenPreferredSourcesExist(t *testing.T) {
	longText := strings.Repeat("Migration Assistant helps transfer contacts, calendars, email accounts, and files from another computer to a Mac. ", 8)
	pages := usableSourcePages([]browserauto.Page{
		{URL: "https://www.youtube.com/watch?v=eOFaYzLz6Fo", FinalURL: "https://www.youtube.com/watch?v=eOFaYzLz6Fo", Title: "How to transfer your data using Migration Assistant | Apple Support - YouTube", Text: longText, StatusCode: 200},
		{URL: "https://support.apple.com/en-us/102565", FinalURL: "https://support.apple.com/en-us/102565", Title: "Transfer from PC to Mac with Migration Assistant - Apple Support", Text: longText, StatusCode: 200},
		{URL: "https://support.apple.com/en-us/102613", FinalURL: "https://support.apple.com/en-us/102613", Title: "Move content to a new Mac - Apple Support", Text: longText, StatusCode: 200},
	})
	if len(pages) != 2 {
		t.Fatalf("expected 2 preferred support pages, got %d", len(pages))
	}
	for _, page := range pages {
		if strings.Contains(page.FinalURL, "youtube.com") {
			t.Fatalf("expected YouTube source to be dropped when enough preferred sources exist: %#v", pages)
		}
	}
}

func TestCleanResearchSentenceRemovesWikiFootnotes(t *testing.T) {
	got := cleanResearchSentence("역사 [ 편집 ] 세종은 1443년 (세종 25년) 훈민정음을 만들어서 [ 1 ] 1446년 훈민정음을 반포했다.")
	if strings.Contains(got, "[ 편집 ]") || strings.Contains(got, "[ 1 ]") {
		t.Fatalf("wiki footnotes should be removed: %q", got)
	}
	if !strings.Contains(got, "세종은 1443년") || !strings.Contains(got, "1446년 훈민정음을 반포했다") {
		t.Fatalf("useful content should remain: %q", got)
	}
}

func TestCleanResearchSentenceRemovesRepeatedAppleSupportTitle(t *testing.T) {
	got := cleanResearchSentence("\ufeff Transfer from PC to Mac with Migration Assistant - Apple Support Transfer from PC to Mac with Migration Assistant Migration Assistant copies contacts, calendars, email accounts, and files.")
	if strings.Contains(got, "Apple Support Transfer from PC to Mac") {
		t.Fatalf("title prefix should be removed: %q", got)
	}
	if !strings.HasPrefix(got, "Transfer from PC to Mac with Migration Assistant Migration Assistant copies") {
		t.Fatalf("useful Apple Support content should remain: %q", got)
	}
}

func TestSourceExcerptSkipsYouTubeFooterNoise(t *testing.T) {
	text := `How to transfer your data from a Windows PC to a Mac using Migration Assistant | Apple Support - YouTube 정보 보도자료 저작권 문의하기 크리에이터 광고 개발자 약관 개인정보처리방침 정책 및 안전 YouTube 작동의 원리 새로운 기능 테스트.
Migration Assistant can copy files, contacts, calendars, email accounts, and settings from a Windows PC to a Mac.
Before transferring, update Windows and connect both computers to power and the same network.`
	got := sourceExcerpt(text, 260)
	for _, bad := range []string{"YouTube 정보", "저작권 문의하기", "크리에이터 광고 개발자", "개인정보처리방침", "YouTube 작동의 원리"} {
		if strings.Contains(got, bad) {
			t.Fatalf("excerpt kept YouTube footer noise %q: %q", bad, got)
		}
	}
	if !strings.Contains(got, "Migration Assistant can copy files") {
		t.Fatalf("excerpt missed useful sentence: %q", got)
	}
}

func TestSourceCitedFindingsAddsSourceLabels(t *testing.T) {
	longText := strings.Repeat("훈민정음은 세종대왕이 백성을 위해 만든 문자와 해설서로 설명된다. 창제와 반포 시기, 제작 취지 확인에는 원문 출처가 필요하다. ", 5)
	got := sourceCitedFindings([]browserauto.Page{
		{URL: "https://example.com/1", Title: "세종 자료", Text: longText, StatusCode: 200},
	}, 2)
	if len(got) == 0 {
		t.Fatal("expected cited findings")
	}
	if !strings.Contains(got[0], "[S1]") || strings.Contains(got[0], "세종 자료:") {
		t.Fatalf("unexpected cited finding: %#v", got)
	}
}
