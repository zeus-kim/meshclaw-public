package assistantbrief

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/meshclaw/meshclaw/internal/newsbrief"
)

func TestSanitizeBriefTextRemovesHTMLBreaks(t *testing.T) {
	got := sanitizeBriefText("첫 줄<br>둘째 줄<br />셋째 줄")
	if strings.Contains(got, "<br") {
		t.Fatalf("unsanitized text: %q", got)
	}
	if !strings.Contains(got, "첫 줄\n둘째 줄\n셋째 줄") {
		t.Fatalf("got=%q", got)
	}
}

func TestSanitizeBriefTextRewritesStiffBroadcastClosing(t *testing.T) {
	got := sanitizeBriefText("오늘 브리핑입니다.\n\n안녕히 계십시오.")
	if strings.Contains(got, "안녕히 계십시오") {
		t.Fatalf("stiff closing leaked: %q", got)
	}
	if !strings.Contains(got, "좋은 하루 보내세요.") {
		t.Fatalf("closing was not rewritten: %q", got)
	}
}

func TestKoreanNewsCategory(t *testing.T) {
	if got := koreanNewsCategory("overseas/vietnam"); got != "베트남" {
		t.Fatalf("got=%q", got)
	}
	if got := koreanNewsCategory("world"); got != "세계" {
		t.Fatalf("got=%q", got)
	}
}

func TestFormatWeatherUsesCleanLocationLabel(t *testing.T) {
	brief := Brief{
		Location: "는",
		Weather:  &Weather{Location: "Seoul", Condition: "Sunny", TempC: "24", FeelsLikeC: "24", Humidity: "45", WindKmph: "10", PrecipMM: "0.0"},
	}
	got := formatWeather(brief)
	if strings.Contains(got, "는은") {
		t.Fatalf("particle artifact in reply: %q", got)
	}
	if !strings.HasPrefix(got, "서울") {
		t.Fatalf("got=%q", got)
	}
}

func TestKoreanJosaEun(t *testing.T) {
	if got := koreanJosaEun("서울"); got != "은" {
		t.Fatalf("got=%q", got)
	}
	if got := koreanJosaEun("부산"); got != "은" {
		t.Fatalf("got=%q", got)
	}
	if got := koreanJosaEun("제주"); got != "는" {
		t.Fatalf("got=%q", got)
	}
	if got := koreanJosaEun("Namyangju"); got != "는" {
		t.Fatalf("Latin place name got=%q want 는", got)
	}
	if got := koreanLocationName("Namyangju"); got != "남양주" {
		t.Fatalf("got=%q", got)
	}
}

func TestFormatWeatherDoesNotUseMorningGreeting(t *testing.T) {
	brief := Brief{
		Location: "Seoul",
		Weather:  &Weather{Location: "Seoul", Condition: "Sunny", TempC: "24", FeelsLikeC: "24", Humidity: "45", WindKmph: "10", PrecipMM: "0.0"},
	}
	got := formatWeather(brief)
	if strings.Contains(got, "좋은 아침") {
		t.Fatalf("weather reply used morning greeting: %q", got)
	}
	if !strings.Contains(got, "서울") || !strings.Contains(got, "맑음") {
		t.Fatalf("got=%q", got)
	}
	if !strings.Contains(got, "외출") && !strings.Contains(got, "옷차림") {
		t.Fatalf("got=%q", got)
	}
}

func TestKoreanNewsTextDoesNotInventWhenNoItems(t *testing.T) {
	brief := Brief{
		Kind:      "meshclaw_assistant_news",
		Generated: parseTestTime(t, "2026-05-24T00:00:00Z"),
		Errors:    []string{"news: feed file missing"},
	}
	got := koreanNewsText(context.Background(), brief)
	if strings.Contains(got, "iPhone") || strings.Contains(got, "도요타") || strings.Contains(got, "BYD") {
		t.Fatalf("news text invented stories: %q", got)
	}
	if !strings.Contains(got, "현재 수집된 주요 뉴스가 없습니다") {
		t.Fatalf("got=%q", got)
	}
}

func TestCleanNewsDescriptionDropsDuplicatesAndComments(t *testing.T) {
	if got := cleanNewsDescription("The day my ping took countermeasures", "Comments"); got != "" {
		t.Fatalf("comments description = %q, want empty", got)
	}
	if got := cleanNewsDescription("A title", "A title   News Source"); got != "" {
		t.Fatalf("duplicate description = %q, want empty", got)
	}
	if got := cleanNewsDescription("A title", "A short useful description"); got == "" {
		t.Fatalf("useful description was removed")
	}
}

func TestFormatNewsUsesArticleExcerpt(t *testing.T) {
	brief := Brief{
		Kind:      "meshclaw_assistant_news",
		Generated: parseTestTime(t, "2026-05-24T00:00:00Z"),
		NewsLimit: 1,
		News: []newsbrief.Item{{
			FeedID:         "tech",
			Category:       "tech",
			Title:          "Headline only",
			Description:    "RSS title-like text",
			ArticleExcerpt: "The fetched article body explains the actual outage fix and the evidence that operators verified.",
		}},
	}
	got := formatNews(brief)
	if strings.Contains(got, "본문 근거") || strings.Contains(got, "RSS 설명") {
		t.Fatalf("leaked internal evidence label: %q", got)
	}
	if !strings.Contains(got, "핵심") {
		t.Fatalf("missing readable summary label: %q", got)
	}
	if !strings.Contains(got, "actual outage fix") {
		t.Fatalf("missing article excerpt: %q", got)
	}
}

func TestNewsLimitFromText(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"Argos, give me the five most important news items I should read this morning.", 5},
		{"오늘 아침에 꼭 알아야 할 뉴스 5개만 짧게 정리해줘.", 5},
		{"주요뉴스 다섯 개만 알려줘", 5},
		{"뉴스 정리해줘", 10},
	}
	for _, tc := range cases {
		if got := NewsLimitFromText(tc.input, 10); got != tc.want {
			t.Fatalf("NewsLimitFromText(%q)=%d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestFormatNewsUsesGlobalNumbersAcrossSections(t *testing.T) {
	brief := Brief{
		Kind:      "meshclaw_assistant_news",
		Generated: parseTestTime(t, "2026-05-24T00:00:00Z"),
		NewsLimit: 3,
		News: []newsbrief.Item{
			{FeedID: "japan-nhk", Category: "overseas/japan", Title: "Japan item"},
			{FeedID: "thailand-bangkok-post", Category: "overseas/thailand", Title: "Thailand item"},
			{FeedID: "hn", Category: "tech", Title: "Tech item"},
		},
	}
	got := formatNews(brief)
	for _, want := range []string{"1. Tech item", "2. Japan item", "3. Thailand item"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Count(got, "1.") != 1 {
		t.Fatalf("numbering restarted across sections:\n%s", got)
	}
}

func TestSelectMajorNewsKeepsCategoryBalance(t *testing.T) {
	now := time.Now().UTC()
	items := []newsbrief.Item{
		{FeedID: "japan-nhk", Category: "overseas/japan", Title: "Japan 1", Published: now},
		{FeedID: "japan-nhk", Category: "overseas/japan", Title: "Japan 2", Published: now.Add(-time.Minute)},
		{FeedID: "japan-nhk", Category: "overseas/japan", Title: "Japan 3", Published: now.Add(-2 * time.Minute)},
		{FeedID: "thailand-bangkok-post", Category: "overseas/thailand", Title: "Thailand 1", Published: now},
		{FeedID: "thailand-bangkok-post", Category: "overseas/thailand", Title: "Thailand 2", Published: now.Add(-time.Minute)},
		{FeedID: "hn", Category: "tech", Title: "Tech 1", Published: now},
	}
	got := selectMajorNews(items, 5)
	counts := map[string]int{}
	for _, item := range got {
		counts[item.Category]++
	}
	if counts["overseas/japan"] > 2 {
		t.Fatalf("selected too many Japan items: %#v", got)
	}
	if len(got) != 5 {
		t.Fatalf("selected %d items, want 5: %#v", len(got), got)
	}
}

func TestRankedNewsPrefersHardNewsOverLiveSports(t *testing.T) {
	now := time.Now().UTC()
	items := []newsbrief.Item{
		{FeedID: "guardian-us", Category: "overseas/us", Title: "French Open 2026: Raducanu crashes out as it happened", Published: now},
		{FeedID: "npr-news", Category: "overseas/us", Title: "DR Congo Ebola cases rise amid distrust and armed conflict zone", Published: now.Add(-time.Hour)},
	}
	got := selectMajorNews(items, 1)
	if len(got) != 1 || !strings.Contains(got[0].Title, "Ebola") {
		t.Fatalf("selected %#v, want hard-news item", got)
	}
}

func TestUsableGeneratedNewsSummaryRejectsTruncatedModelOutput(t *testing.T) {
	got := usableGeneratedNewsSummary("오늘의 주요뉴스입니다.\n\n**일본**\n* **트", 6)
	if got {
		t.Fatalf("truncated summary was accepted")
	}
}

func TestUsableGeneratedNewsSummaryRejectsInventedJapaneseNames(t *testing.T) {
	reply := "오늘의 주요뉴스입니다.\n\n2. 대장판 여름 장사, 요코무라가 미토카미를 이기고 우승했습니다.\n\n원문 링크가 필요하면 출처라고 보내세요."
	if usableGeneratedNewsSummary(reply, 10) {
		t.Fatalf("invented Japanese proper-name summary was accepted")
	}
	reply = "오늘의 주요뉴스입니다.\n\n2. 대장급 열동, 2년 만에 우승했습니다.\n\n원문 링크가 필요하면 출처라고 보내세요."
	if usableGeneratedNewsSummary(reply, 10) {
		t.Fatalf("invented Japanese sumo summary was accepted")
	}
	reply = "오늘의 주요뉴스입니다.\n\n4. 하츠루스모에서 요시노리가 우승했습니다.\n\n원문 링크가 필요하면 출처라고 보내세요."
	if usableGeneratedNewsSummary(reply, 10) {
		t.Fatalf("invented Japanese romanized sumo name was accepted")
	}
}

func TestUsableNewsSummaryRejectsOldModelMarkdownCache(t *testing.T) {
	reply := "오늘의 주요뉴스입니다.\n\n1. **호주, 독립 세력 신당 구성 가능성**: 정치적 변동성이 커지고 있습니다.\n\n원문 링크가 필요하면 출처라고 보내세요."
	if usableNewsSummary(reply) {
		t.Fatalf("old markdown model summary cache was accepted")
	}
}

func TestGeneratedNewsMatchesFactsRequiresKnownProperName(t *testing.T) {
	facts := "title=大相撲夏場所 小結 若隆景が優勝 令和4年春場所以来2回目"
	if generatedNewsMatchesFacts("일본 스모 선수 요시노리가 우승했습니다.", facts) {
		t.Fatalf("summary dropped the source proper name")
	}
	if generatedNewsMatchesFacts("일본 스모 선수 若隆景가 2024년 봄 장사 이후 2번째 우승했습니다.", facts) {
		t.Fatalf("summary mistranslated Reiwa year")
	}
	if !generatedNewsMatchesFacts("일본 스모 여름 대회에서 若隆景가 우승했습니다.", facts) {
		t.Fatalf("summary preserved the source proper name but was rejected")
	}
}

func TestNewsDisplayTitleKeepsJapaneseProperName(t *testing.T) {
	item := newsbrief.Item{
		FeedID:   "japan-nhk",
		Category: "overseas/japan",
		Title:    "大相撲夏場所 小結 若隆景が優勝 令和4年春場所以来2回目",
	}
	got := newsDisplayTitle(item)
	if !strings.Contains(got, "若隆景") {
		t.Fatalf("proper name was not preserved: %q", got)
	}
	if strings.Contains(got, "요코무라") || strings.Contains(got, "가와무라") {
		t.Fatalf("invented Korean name appeared: %q", got)
	}
}

func TestRankedNewsPrefersRecentDirectSourceOverOlderGoogleWrapper(t *testing.T) {
	now := time.Now().UTC()
	items := []newsbrief.Item{
		{
			FeedID:      "japan-ko",
			Category:    "overseas/japan",
			Title:       "Older Google wrapper",
			Link:        "https://news.google.com/rss/articles/abc",
			Description: "RSS text",
			Published:   now.Add(-20 * time.Hour),
		},
		{
			FeedID:    "japan-nhk",
			Category:  "overseas/japan",
			Title:     "Fresh direct source",
			Link:      "https://www3.nhk.or.jp/news/html/example.html",
			Published: now.Add(-2 * time.Hour),
		},
	}
	ranked := rankedNewsForCategory(items, "overseas/japan", map[int]bool{})
	if len(ranked) != 2 || ranked[0].item.FeedID != "japan-nhk" {
		t.Fatalf("unexpected ranking: %#v", ranked)
	}
}

func TestRankedNewsPrefersKoreanFeedForUserFacingNews(t *testing.T) {
	now := time.Now().UTC()
	items := []newsbrief.Item{
		{
			FeedID:      "bbc-world",
			Category:    "world",
			Title:       "English world headline about trade talks",
			Description: "A useful English description.",
			Published:   now,
		},
		{
			FeedID:      "world-ko",
			Category:    "world",
			Title:       "세계 무역 협상 관련 주요 뉴스",
			Description: "한국어로 된 핵심 설명입니다.",
			Link:        "https://news.google.com/rss/articles/example",
			Published:   now.Add(-15 * time.Minute),
		},
	}
	ranked := rankedNewsForCategory(items, "world", map[int]bool{})
	if len(ranked) != 2 || ranked[0].item.FeedID != "world-ko" {
		t.Fatalf("unexpected ranking: %#v", ranked)
	}
}

func TestSelectMajorNewsPrioritizesBroadcastMixOverEntertainment(t *testing.T) {
	now := time.Now().UTC()
	items := []newsbrief.Item{
		{
			FeedID:      "japan-ko",
			Category:    "overseas/japan",
			Title:       "앤더블, 일본 주요 앨범 차트 상위권 석권...'괴물 신인' 입증",
			Description: "앨범 차트와 신인 기록을 다룬 연예 소식입니다.",
			Published:   now,
		},
		{
			FeedID:      "korea-top",
			Category:    "domestic/korea",
			Title:       "국회, 데이터센터 전력망 지원 법안 논의",
			Description: "정부와 국회가 AI 데이터센터 전력 수요와 지역 전력망 대책을 논의했습니다.",
			Published:   now.Add(-10 * time.Minute),
		},
		{
			FeedID:      "world-ko",
			Category:    "world",
			Title:       "미국과 이란, 종전 합의 문안 조율",
			Description: "미국과 이란이 전쟁 종료를 위한 합의 문안을 조율하고 있습니다.",
			Published:   now.Add(-20 * time.Minute),
		},
		{
			FeedID:      "biz-ko",
			Category:    "business",
			Title:       "환율 하락에 국내 증시 상승 출발",
			Description: "금리 전망과 환율 안정이 시장 투자 심리에 영향을 줬습니다.",
			Published:   now.Add(-30 * time.Minute),
		},
		{
			FeedID:      "tech-ko",
			Category:    "tech",
			Title:       "오픈AI, 중국계 계정의 데이터센터 반대 여론 조성 적발",
			Description: "AI를 이용한 여론 조작과 보안 문제가 드러났습니다.",
			Published:   now.Add(-40 * time.Minute),
		},
	}
	got := selectMajorNews(items, 4)
	if len(got) != 4 {
		t.Fatalf("selected %d items, want 4: %#v", len(got), got)
	}
	for _, item := range got {
		if strings.Contains(item.Title, "앨범 차트") {
			t.Fatalf("selected entertainment item for broadcast mix: %#v", got)
		}
	}
	wantCategories := []string{"domestic/korea", "world", "business", "tech"}
	for _, category := range wantCategories {
		found := false
		for _, item := range got {
			if item.Category == category {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing category %s from broadcast mix: %#v", category, got)
		}
	}
}

func TestNewsDetailUsesReadableMarkdownLink(t *testing.T) {
	got := newsOriginalLinkLine("https://news.google.com/rss/articles/very-long-id?oc=5")
	if !strings.Contains(got, "[원문 보기](") {
		t.Fatalf("missing markdown link: %q", got)
	}
	if strings.Contains(got, "원문: https://") {
		t.Fatalf("raw URL label leaked: %q", got)
	}
}

func TestNewsSummaryModelsPrefersConfiguredThenFastFallbacks(t *testing.T) {
	t.Setenv("MESHCLAW_ASSISTANT_NEWS_MODEL", "custom-news")
	t.Setenv("MESHCLAW_ASSISTANT_MODEL", "custom-chat")
	got := newsSummaryModels()
	want := []string{"custom-news", "custom-chat", "gemma3:4b", "gemma3:12b", "gemma4:e4b"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("models = %#v, want %#v", got, want)
	}
}

func parseTestTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
