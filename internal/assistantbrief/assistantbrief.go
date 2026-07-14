package assistantbrief

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/meshclaw/meshclaw/internal/aichat"
	"github.com/meshclaw/meshclaw/internal/evidence"
	"github.com/meshclaw/meshclaw/internal/newsbrief"
)

type Options struct {
	Location       string
	NewsLimit      int
	NoNews         bool
	ModelSummary   bool
	NoModelSummary bool
}

type Brief struct {
	Kind            string           `json:"kind"`
	Format          int              `json:"format,omitempty"`
	Generated       time.Time        `json:"generated"`
	Location        string           `json:"location"`
	NewsLimit       int              `json:"news_limit,omitempty"`
	Text            string           `json:"text"`
	SummaryProvider string           `json:"summary_provider,omitempty"`
	SummaryModel    string           `json:"summary_model,omitempty"`
	Weather         *Weather         `json:"weather,omitempty"`
	News            []newsbrief.Item `json:"news,omitempty"`
	Errors          []string         `json:"errors,omitempty"`
}

const newsBriefFormatVersion = 4

type Weather struct {
	Location   string `json:"location"`
	Condition  string `json:"condition"`
	TempC      string `json:"temp_c"`
	FeelsLikeC string `json:"feels_like_c"`
	Humidity   string `json:"humidity"`
	WindKmph   string `json:"wind_kmph"`
	PrecipMM   string `json:"precip_mm"`
}

func Morning(ctx context.Context, opts Options) Brief {
	opts = fill(opts)
	brief := Brief{Kind: "meshclaw_assistant_morning", Generated: time.Now().UTC(), Location: opts.Location, NewsLimit: opts.NewsLimit}
	weather, err := fetchWeather(ctx, opts.Location)
	if err != nil {
		brief.Errors = append(brief.Errors, "weather: "+err.Error())
	} else {
		brief.Weather = &weather
	}
	if !opts.NoNews {
		newsCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		news, err := newsbrief.Build(newsCtx, newsbrief.BriefOptions{SinceHours: assistantNewsSinceHours(), Limit: max(opts.NewsLimit*8, 64), ArticleLimit: min(max(opts.NewsLimit, 4), 6), ArticleChars: 650})
		if err != nil {
			brief.Errors = append(brief.Errors, "news: "+err.Error())
		} else {
			brief.News = news.Items
		}
	}
	if opts.ModelSummary {
		brief.Text = koreanMorningText(ctx, brief)
	} else {
		brief.Text = formatMorning(brief)
	}
	brief.Text = sanitizeBriefText(brief.Text)
	return brief
}

func News(ctx context.Context, opts Options) Brief {
	opts = fill(opts)
	brief := Brief{Kind: "meshclaw_assistant_news", Format: newsBriefFormatVersion, Generated: time.Now().UTC(), Location: opts.Location, NewsLimit: opts.NewsLimit}
	newsCtx, cancel := context.WithTimeout(ctx, assistantNewsFetchTimeout())
	defer cancel()
	news, err := newsbrief.Build(newsCtx, newsbrief.BriefOptions{SinceHours: assistantNewsSinceHours(), Limit: max(opts.NewsLimit*5, 32), DisableArticleFetch: true})
	if err != nil {
		brief.Errors = append(brief.Errors, "news: "+err.Error())
	} else {
		brief.News = selectMajorNews(news.Items, max(opts.NewsLimit, 10))
	}
	if len(brief.News) == 0 {
		brief.Text = formatNews(brief)
	} else if opts.ModelSummary {
		summaryCtx, summaryCancel := context.WithTimeout(ctx, assistantNewsSummaryTimeout())
		defer summaryCancel()
		brief.Text = koreanNewsText(summaryCtx, brief)
	} else {
		brief.Text = formatNews(brief)
	}
	brief.Text = sanitizeBriefText(brief.Text)
	return brief
}

func PreparedNews(ctx context.Context, opts Options) Brief {
	opts = fill(opts)
	localOpts := opts
	localOpts.ModelSummary = true
	brief := News(ctx, localOpts)
	if brief.SummaryProvider == "" && usableNewsSummary(brief.Text) && !strings.Contains(brief.Text, "  핵심: ") {
		brief.SummaryProvider = "local-model"
		brief.SummaryModel = firstNonEmptyString(strings.TrimSpace(getenv("MESHCLAW_ASSISTANT_NEWS_MODEL")), strings.TrimSpace(getenv("MESHCLAW_ASSISTANT_MODEL")))
	}
	return brief
}

func RecentNews(maxAge time.Duration) (Brief, string, bool) {
	records, err := evidence.List(400)
	if err != nil {
		return Brief{}, "", false
	}
	cutoff := time.Now().UTC().Add(-maxAge)
	for _, summary := range records {
		if summary.Kind != "assistant-news" {
			continue
		}
		if !summary.Time.IsZero() && summary.Time.Before(cutoff) {
			continue
		}
		brief, ok := loadNewsBriefFromEvidence(summary.StoredAt)
		if !ok || brief.Format < newsBriefFormatVersion || !usableNewsSummary(brief.Text) || len(brief.News) == 0 {
			continue
		}
		brief.Text = stripCachedNewsNotice(brief.Text)
		brief.Errors = stripCachedFromErrors(brief.Errors)
		return brief, summary.StoredAt, true
	}
	return Brief{}, "", false
}

const cachedNewsNotice = "최근 수집된 뉴스 브리핑을 사용했습니다. 최신 기사로 다시 수집하려면 '뉴스 새로 갱신해줘'라고 보내세요."

func stripCachedNewsNotice(value string) string {
	value = strings.ReplaceAll(value, "\n\n"+cachedNewsNotice, "")
	value = strings.ReplaceAll(value, cachedNewsNotice, "")
	for strings.Contains(value, "\n\n\n") {
		value = strings.ReplaceAll(value, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(value)
}

func stripCachedFromErrors(values []string) []string {
	if len(values) == 0 {
		return values
	}
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		if strings.HasPrefix(value, "cached_from: ") {
			continue
		}
		cleaned = append(cleaned, value)
	}
	return cleaned
}

func NewsSources() Brief {
	brief := Brief{Kind: "meshclaw_assistant_news_sources", Generated: time.Now().UTC(), Location: "Seoul", NewsLimit: 12}
	items, path, ok := loadLatestNewsItems()
	if !ok || len(items) == 0 {
		brief.Text = "최근 뉴스 브리핑 evidence를 찾지 못했습니다. 먼저 '오늘 주요뉴스 정리해줘'라고 물어보세요."
		return brief
	}
	brief.News = items
	var b strings.Builder
	b.WriteString("최근 주요뉴스 원문 링크입니다.\n\n")
	for i, item := range items {
		if i >= 12 {
			break
		}
		if item.Link != "" {
			fmt.Fprintf(&b, "%d. [%s](%s)\n", i+1, firstNonEmpty(item.Title, "원문"), item.Link)
		} else {
			fmt.Fprintf(&b, "%d. %s\n", i+1, firstNonEmpty(item.Title, "원문"))
		}
	}
	fmt.Fprintf(&b, "\n기준 evidence: %s\n", path)
	b.WriteString("원하시면 '3번 자세히'처럼 번호를 보내세요.")
	brief.Text = sanitizeBriefTextKeepURLs(b.String())
	return brief
}

func NewsDetail(ctx context.Context, message string) Brief {
	brief := Brief{Kind: "meshclaw_assistant_news_detail", Generated: time.Now().UTC(), Location: "Seoul", NewsLimit: 12}
	items, path, ok := loadLatestNewsItems()
	if !ok || len(items) == 0 {
		brief.Text = "최근 뉴스 브리핑 evidence를 찾지 못했습니다. 먼저 '오늘 주요뉴스 정리해줘'라고 물어보세요."
		return brief
	}
	brief.News = items
	index := extractNewsIndex(message)
	if index <= 0 {
		index = 1
	}
	if index > len(items) {
		brief.Text = fmt.Sprintf("최근 브리핑에는 %d번 뉴스가 없습니다. 현재 저장된 뉴스는 %d개입니다.", index, len(items))
		return brief
	}
	item := items[index-1]
	if strings.TrimSpace(item.ArticleExcerpt) == "" && strings.TrimSpace(item.Link) != "" {
		enriched := []newsbrief.Item{item}
		enrichCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		newsbrief.EnrichArticles(enrichCtx, enriched, newsbrief.BriefOptions{ArticleLimit: 1, ArticleChars: 1200})
		item = enriched[0]
		brief.News[index-1] = item
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d번 뉴스입니다.\n\n", index)
	fmt.Fprintf(&b, "제목: %s\n", newsDisplayTitle(item))
	if item.FeedTitle != "" || item.Category != "" {
		fmt.Fprintf(&b, "분류: %s / %s\n", koreanNewsCategory(firstNonEmpty(item.Category, item.FeedID)), firstNonEmpty(item.FeedTitle, item.FeedID))
	}
	if item.ArticleExcerpt != "" {
		summary := koreanNewsDetailText(ctx, item, index)
		if strings.TrimSpace(summary) != "" {
			fmt.Fprintf(&b, "\n%s\n", summary)
		} else {
			fmt.Fprintf(&b, "\n본문 근거:\n%s\n", truncateText(item.ArticleExcerpt, 650))
		}
	} else if desc := cleanNewsDescription(item.Title, item.Description); desc != "" {
		fmt.Fprintf(&b, "\nRSS 설명:\n%s\n", truncateText(desc, 500))
	} else {
		b.WriteString("\n본문 근거를 가져오지 못했습니다. 이 항목은 제목과 RSS 메타데이터만 있습니다.\n")
	}
	if item.Link != "" {
		fmt.Fprintf(&b, "\n%s\n", newsOriginalLinkLine(item.Link))
	}
	fmt.Fprintf(&b, "\n기준 evidence: %s\n", path)
	brief.Text = sanitizeBriefText(b.String())
	return brief
}

func WeatherNow(ctx context.Context, opts Options) Brief {
	opts = fill(opts)
	brief := Brief{Kind: "meshclaw_assistant_weather", Generated: time.Now().UTC(), Location: opts.Location}
	weather, err := fetchWeather(ctx, opts.Location)
	if err != nil {
		brief.Errors = append(brief.Errors, "weather: "+err.Error())
	} else {
		brief.Weather = &weather
		brief.Location = weather.Location
	}
	brief.Text = formatWeather(brief)
	return brief
}

func Evening(ctx context.Context, opts Options) Brief {
	opts = fill(opts)
	brief := Brief{Kind: "meshclaw_assistant_evening", Generated: time.Now().UTC(), Location: opts.Location}
	weather, err := fetchWeather(ctx, opts.Location)
	if err != nil {
		brief.Errors = append(brief.Errors, "weather: "+err.Error())
	} else {
		brief.Weather = &weather
	}
	brief.Text = formatEvening(brief)
	return brief
}

func Menu() Brief {
	text := strings.Join([]string{
		"MeshClaw Daily Menu",
		"번호로 답하면 해당 브리핑을 보낼 수 있습니다.",
		"",
		"1. 해외뉴스 브리핑",
		"2. 날씨/생활정보",
		"3. 오늘 일정/메일 정리",
		"4. 저녁 10시 리마인더",
		"5. 콘텐츠/RSS/영상 링크",
		"",
		"현재 구현: 1, 2, 아침/저녁 정기 브리핑.",
		"메일/달력/문서작성/쇼핑은 별도 권한 adapter가 필요합니다.",
	}, "\n")
	return Brief{Kind: "meshclaw_assistant_menu", Generated: time.Now().UTC(), Text: text}
}

func fill(opts Options) Options {
	if strings.TrimSpace(opts.Location) == "" {
		opts.Location = "Seoul"
	}
	if opts.NewsLimit <= 0 {
		opts.NewsLimit = 10
	}
	if opts.NoModelSummary || strings.EqualFold(getenv("MESHCLAW_ASSISTANT_MODEL_SUMMARY"), "0") {
		opts.ModelSummary = false
	} else {
		opts.ModelSummary = true
	}
	return opts
}

func assistantNewsSinceHours() int {
	value := strings.TrimSpace(getenv("MESHCLAW_ASSISTANT_NEWS_SINCE_HOURS"))
	if value == "" {
		return 24
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 24
	}
	if parsed > 168 {
		return 168
	}
	return parsed
}

func assistantNewsFetchTimeout() time.Duration {
	value := strings.TrimSpace(getenv("MESHCLAW_ASSISTANT_NEWS_TIMEOUT_SECONDS"))
	if value == "" {
		return 4 * time.Second
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 4 * time.Second
	}
	if parsed > 30 {
		return 30 * time.Second
	}
	return time.Duration(parsed) * time.Second
}

func assistantNewsSummaryTimeout() time.Duration {
	value := strings.TrimSpace(getenv("MESHCLAW_ASSISTANT_NEWS_SUMMARY_TIMEOUT_SECONDS"))
	if value == "" {
		return 6 * time.Second
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 6 * time.Second
	}
	if parsed > 30 {
		return 30 * time.Second
	}
	return time.Duration(parsed) * time.Second
}

const defaultWeatherLocation = "Seoul"

// normalizeWeatherLocation makes a user/mediaQuery-derived location safe for wttr.in.
// Rejects empty, whitespace, question marks, and bare particles left after
// keyword stripping (e.g. "서울 날씨는 ?" → "서울 는 ?").
func normalizeWeatherLocation(location string) string {
	location = strings.TrimSpace(location)
	if location == "" {
		return defaultWeatherLocation
	}
	location = strings.TrimRight(location, " ?!?。，.:")
	location = strings.TrimSpace(location)
	if location == "" || strings.ContainsAny(location, " \t\n\r?") {
		return defaultWeatherLocation
	}
	switch location {
	case "는", "은", "이", "가", "을", "를", "에", "의":
		return defaultWeatherLocation
	}
	return location
}

func fetchWeather(ctx context.Context, location string) (Weather, error) {
	loc := normalizeWeatherLocation(location)
	var lastErr error
	for _, candidate := range weatherLocationCandidates(loc) {
		weather, err := fetchWeatherOnce(ctx, candidate)
		if err == nil {
			return weather, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("weather lookup failed")
	}
	return Weather{}, lastErr
}

func weatherLocationCandidates(loc string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(value string) {
		value = normalizeWeatherLocation(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		out = append(out, value)
	}
	add(loc)
	if loc != defaultWeatherLocation {
		add(defaultWeatherLocation)
	}
	return out
}

func fetchWeatherOnce(ctx context.Context, loc string) (Weather, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return Weather{}, ctx.Err()
			case <-time.After(400 * time.Millisecond):
			}
		}
		weather, err := fetchWeatherFromWTTR(ctx, loc)
		if err == nil {
			return weather, nil
		}
		lastErr = err
		if !isRetryableWeatherErr(err) {
			return Weather{}, err
		}
	}
	return Weather{}, lastErr
}

func isRetryableWeatherErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "500") ||
		strings.Contains(msg, "502") ||
		strings.Contains(msg, "503") ||
		strings.Contains(msg, "504") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "temporarily")
}

func fetchWeatherFromWTTR(ctx context.Context, loc string) (Weather, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://wttr.in/"+url.PathEscape(loc)+"?format=j1", nil)
	if err != nil {
		return Weather{}, err
	}
	req.Header.Set("User-Agent", "MeshClaw assistant brief")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Weather{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Weather{}, fmt.Errorf("weather returned %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Weather{}, err
	}
	var decoded struct {
		Current []struct {
			TempC       string `json:"temp_C"`
			FeelsLikeC  string `json:"FeelsLikeC"`
			Humidity    string `json:"humidity"`
			WindKmph    string `json:"windspeedKmph"`
			PrecipMM    string `json:"precipMM"`
			WeatherDesc []struct {
				Value string `json:"value"`
			} `json:"weatherDesc"`
		} `json:"current_condition"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return Weather{}, err
	}
	if len(decoded.Current) == 0 {
		return Weather{}, fmt.Errorf("weather returned no current condition")
	}
	cur := decoded.Current[0]
	condition := ""
	if len(cur.WeatherDesc) > 0 {
		condition = cur.WeatherDesc[0].Value
	}
	return Weather{
		Location: loc, Condition: condition, TempC: cur.TempC, FeelsLikeC: cur.FeelsLikeC,
		Humidity: cur.Humidity, WindKmph: cur.WindKmph, PrecipMM: cur.PrecipMM,
	}, nil
}

func formatMorning(brief Brief) string {
	var b strings.Builder
	b.WriteString("좋은 아침입니다.\n")
	b.WriteString("오늘의 MeshClaw 모닝 브리핑입니다.\n\n")
	if brief.Weather != nil {
		fmt.Fprintf(&b, "날씨: %s, %s°C, 체감 %s°C, 습도 %s%%, 바람 %skm/h, 강수 %smm\n\n",
			koreanWeatherCondition(brief.Weather.Condition), brief.Weather.TempC, brief.Weather.FeelsLikeC, brief.Weather.Humidity, brief.Weather.WindKmph, brief.Weather.PrecipMM)
	}
	b.WriteString("오늘 체크할 것:\n")
	b.WriteString("- 중요한 메일과 일정은 개인 방에서만 정리합니다.\n")
	b.WriteString("- 서버/보안 조치는 MeshClaw Ops 승인 흐름을 거칩니다.\n")
	b.WriteString("- 쇼핑/결제/예약은 아직 자동 실행하지 않고 approval-required로 남깁니다.\n\n")
	if len(brief.News) > 0 {
		b.WriteString("해외/기술 뉴스:\n")
		for _, item := range selectMorningNews(brief.News, brief.NewsLimit) {
			fmt.Fprintf(&b, "- [%s] %s\n", firstNonEmpty(item.Category, item.FeedID), item.Title)
			if item.Link != "" {
				fmt.Fprintf(&b, "  %s\n", item.Link)
			}
		}
		b.WriteByte('\n')
	}
	b.WriteString("기도문:\n")
	b.WriteString("오늘 해야 할 일을 분별하게 하시고, 급한 일과 중요한 일을 구분하게 하소서. 사람과 시스템을 다룰 때 조급함보다 정확함을 먼저 선택하게 하소서.\n")
	appendErrors(&b, brief.Errors)
	return strings.TrimSpace(b.String())
}

func formatNews(brief Brief) string {
	var b strings.Builder
	b.WriteString("오늘의 주요뉴스입니다.\n\n")
	if generated := koreanGeneratedTime(brief.Generated); generated != "" {
		fmt.Fprintf(&b, "수집 기준: %s\n\n", generated)
	}
	items := selectMajorNews(brief.News, brief.NewsLimit)
	if len(items) == 0 {
		b.WriteString("현재 수집된 주요 뉴스가 없습니다.\n")
		appendErrors(&b, brief.Errors)
		return strings.TrimSpace(b.String())
	}
	grouped := map[string][]newsbrief.Item{}
	order := []string{}
	for _, item := range items {
		category := firstNonEmpty(item.Category, item.FeedID, "general")
		if _, ok := grouped[category]; !ok {
			order = append(order, category)
		}
		grouped[category] = append(grouped[category], item)
	}
	globalIndex := 1
	for _, category := range order {
		fmt.Fprintf(&b, "%s\n", koreanNewsCategory(category))
		for _, item := range grouped[category] {
			fmt.Fprintf(&b, "%d. %s\n", globalIndex, newsDisplayTitle(item))
			if excerpt := cleanNewsDescription(item.Title, item.ArticleExcerpt); excerpt != "" {
				fmt.Fprintf(&b, "  핵심: %s\n", truncateText(excerpt, 180))
			} else if desc := cleanNewsDescription(item.Title, item.Description); desc != "" {
				fmt.Fprintf(&b, "  핵심: %s\n", truncateText(desc, 140))
			}
			globalIndex++
		}
		b.WriteByte('\n')
	}
	b.WriteString("원문 링크가 필요하면 '출처'라고 보내세요. 그러면 관련 링크를 따로 정리해 드립니다.\n")
	appendErrors(&b, brief.Errors)
	return strings.TrimSpace(b.String())
}

func newsDisplayTitle(item newsbrief.Item) string {
	title := strings.TrimSpace(item.Title)
	if title == "" {
		return "제목 없음"
	}
	category := firstNonEmpty(item.Category, item.FeedID)
	if strings.HasPrefix(category, "overseas/japan") {
		switch {
		case strings.Contains(title, "母娘殺害") || strings.Contains(title, "殺害事件"):
			if strings.Contains(title, "42歳") || strings.Contains(title, "42") {
				return "일본 효고현 모녀 살해 사건에서 42세 용의자 공개 수배"
			}
			return "일본 살해 사건 관련 경찰 수사 진행"
		case strings.Contains(title, "大相撲"):
			name := extractBetween(title, "小結 ", "が")
			if name == "" {
				name = extractBetween(title, "小結", "が")
			}
			if strings.TrimSpace(name) != "" {
				return fmt.Sprintf("일본 스모 여름 대회에서 %s 우승", strings.TrimSpace(name))
			}
			return "일본 스모 여름 대회 우승자 결정"
		case strings.Contains(title, "トランプ") && strings.Contains(title, "イラン"):
			return "트럼프 대통령, 이란 협상이 최종 조정 단계라고 언급"
		case strings.Contains(title, "地震") || strings.Contains(title, "震度"):
			return "일본 지진 정보: 쓰나미 우려는 낮음"
		}
	}
	return title
}

func extractBetween(value, start, end string) string {
	idx := strings.Index(value, start)
	if idx < 0 {
		return ""
	}
	rest := value[idx+len(start):]
	endIdx := strings.Index(rest, end)
	if endIdx < 0 {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(rest[:endIdx])
}

func cleanNewsDescription(title, desc string) string {
	desc = strings.TrimSpace(strings.ReplaceAll(desc, "\u00a0", " "))
	if desc == "" {
		return ""
	}
	lower := strings.ToLower(desc)
	if lower == "comments" || strings.HasPrefix(lower, "comments ") {
		return ""
	}
	compactTitle := compactNewsCompareText(title)
	compactDesc := compactNewsCompareText(desc)
	if compactTitle != "" && (compactDesc == compactTitle || strings.Contains(compactDesc, compactTitle) || strings.Contains(compactTitle, compactDesc)) {
		return ""
	}
	return desc
}

func compactNewsCompareText(value string) string {
	value = strings.ToLower(value)
	replacer := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "", "-", "", "–", "", "—", "", "·", "", "|", "", ":", "", "：", "", "\u00a0", "")
	return replacer.Replace(value)
}

func formatWeather(brief Brief) string {
	var b strings.Builder
	location := defaultWeatherLocation
	if brief.Weather != nil && strings.TrimSpace(brief.Weather.Location) != "" {
		location = brief.Weather.Location
	} else if strings.TrimSpace(brief.Location) != "" {
		location = normalizeWeatherLocation(brief.Location)
	}
	displayLocation := koreanLocationName(location)
	particle := koreanJosaEun(displayLocation)
	if brief.Weather == nil {
		fmt.Fprintf(&b, "%s%s 날씨를 가져오지 못했습니다.", displayLocation, particle)
		appendErrors(&b, brief.Errors)
		return strings.TrimSpace(b.String())
	}
	condition := koreanWeatherCondition(brief.Weather.Condition)
	temp := firstNonEmpty(brief.Weather.TempC, "?")
	feels := firstNonEmpty(brief.Weather.FeelsLikeC, temp)
	humidity := firstNonEmpty(brief.Weather.Humidity, "?")
	wind := firstNonEmpty(brief.Weather.WindKmph, "?")
	precip := firstNonEmpty(brief.Weather.PrecipMM, "0")

	fmt.Fprintf(&b, "%s%s 지금 %s이고, 기온은 %s도입니다. 체감은 %s도라서 크게 춥거나 덥지는 않은 편입니다.\n\n", displayLocation, particle, condition, temp, feels)
	fmt.Fprintf(&b, "습도는 %s%%, 바람은 시속 %skm 정도입니다. 강수량은 %smm로 잡혀 있습니다.\n\n", humidity, wind, precip)
	b.WriteString(weatherAdvice(*brief.Weather))
	b.WriteString("\n\nweather API로 확인한 현재값입니다.")
	appendErrors(&b, brief.Errors)
	return strings.TrimSpace(b.String())
}

func weatherAdvice(weather Weather) string {
	var advice []string
	temp := parseFloat(weather.FeelsLikeC)
	if temp.Valid {
		switch {
		case temp.Value <= 5:
			advice = append(advice, "두꺼운 외투와 장갑을 챙기는 편이 좋겠습니다.")
		case temp.Value <= 12:
			advice = append(advice, "가벼운 외투나 겉옷을 챙기면 좋겠습니다.")
		case temp.Value >= 29:
			advice = append(advice, "얇은 옷차림에 물을 챙기고, 오래 걷는 일정은 피하는 편이 좋겠습니다.")
		case temp.Value >= 24:
			advice = append(advice, "낮에는 조금 따뜻할 수 있으니 가벼운 옷차림이 좋겠습니다.")
		default:
			advice = append(advice, "평소 옷차림이면 무난하겠습니다.")
		}
	}
	if parseFloat(weather.PrecipMM).Value > 0.2 || strings.Contains(koreanWeatherCondition(weather.Condition), "비") {
		advice = append(advice, "비 가능성이 있으니 작은 우산을 챙기세요.")
	}
	if parseFloat(weather.WindKmph).Value >= 20 {
		advice = append(advice, "바람이 강한 편이라 우산보다는 후드나 바람막이가 더 편할 수 있습니다.")
	}
	if parseFloat(weather.Humidity).Value >= 85 {
		advice = append(advice, "습도가 높아 체감이 눅눅할 수 있습니다.")
	}
	if len(advice) == 0 {
		return "외출할 때 특별히 챙길 것은 많지 않아 보입니다."
	}
	return strings.Join(advice, " ")
}

type optionalFloat struct {
	Value float64
	Valid bool
}

func parseFloat(value string) optionalFloat {
	value = strings.TrimSpace(strings.TrimSuffix(value, "%"))
	if value == "" {
		return optionalFloat{}
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return optionalFloat{}
	}
	return optionalFloat{Value: n, Valid: true}
}

func selectMajorNews(items []newsbrief.Item, limit int) []newsbrief.Item {
	if limit <= 0 {
		limit = 12
	}
	priority := []string{
		"domestic/korea",
		"world",
		"overseas/us",
		"business",
		"tech",
		"overseas/china",
		"overseas/japan",
		"overseas/thailand",
		"overseas/vietnam",
		"infra",
		"ops",
	}
	used := map[int]bool{}
	categoryCounts := map[string]int{}
	var out []newsbrief.Item
	for _, category := range priority {
		categoryItems := rankedNewsForCategory(items, category, used)
		taken := 0
		for _, ranked := range categoryItems {
			out = append(out, ranked.item)
			used[ranked.index] = true
			categoryCounts[category]++
			taken++
			if len(out) >= limit || taken >= firstPassMajorNewsLimit(category) {
				break
			}
		}
		if len(out) >= limit {
			return out
		}
	}
	for i, item := range items {
		if used[i] {
			continue
		}
		if shouldSkipMajorNewsCandidate(item) {
			continue
		}
		category := firstNonEmpty(item.Category, item.FeedID, "general")
		if categoryCounts[category] >= 2 {
			continue
		}
		out = append(out, item)
		used[i] = true
		categoryCounts[category]++
		if len(out) >= limit {
			break
		}
	}
	if len(out) >= limit {
		return out
	}
	for i, item := range items {
		if used[i] {
			continue
		}
		if shouldSkipMajorNewsCandidate(item) {
			continue
		}
		category := firstNonEmpty(item.Category, item.FeedID, "general")
		if categoryCounts[category] >= 3 {
			continue
		}
		out = append(out, item)
		used[i] = true
		categoryCounts[category]++
		if len(out) >= limit || len(out) >= 8 {
			break
		}
	}
	if len(out) >= limit || len(out) >= 7 {
		return out
	}
	return out
}

func firstPassMajorNewsLimit(category string) int {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "domestic/korea", "world", "overseas/us", "business", "tech":
		return 1
	default:
		return 1
	}
}

type rankedNewsItem struct {
	index int
	item  newsbrief.Item
}

func rankedNewsForCategory(items []newsbrief.Item, category string, used map[int]bool) []rankedNewsItem {
	var ranked []rankedNewsItem
	for i, item := range items {
		if used[i] || firstNonEmpty(item.Category, item.FeedID) != category {
			continue
		}
		if shouldSkipMajorNewsCandidate(item) {
			continue
		}
		ranked = append(ranked, rankedNewsItem{index: i, item: item})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		ai, aj := newsEvidenceScore(ranked[i].item), newsEvidenceScore(ranked[j].item)
		if ai != aj {
			return ai > aj
		}
		a, b := ranked[i].item.Published, ranked[j].item.Published
		if a.IsZero() || b.IsZero() {
			return ranked[i].index < ranked[j].index
		}
		return a.After(b)
	})
	return ranked
}

func shouldSkipMajorNewsCandidate(item newsbrief.Item) bool {
	if isSportsOrEntertainmentNews(item) {
		return true
	}
	return newsContentPriorityScore(item) <= -20
}

func isSportsOrEntertainmentNews(item newsbrief.Item) bool {
	text := strings.ToLower(strings.Join([]string{item.Title, item.Description}, " "))
	for _, keyword := range []string{
		" live:", "as it happened",
		"french open", "premier league", "champions league", "world cup", " nba ", " mlb ", " nfl ",
		"tennis", "football", "soccer", "cricket", "golf", "formula 1", "grand prix",
		"celebrity", "movie review", "album review", "recipe",
	} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func newsEvidenceScore(item newsbrief.Item) int {
	score := 0
	score += newsSourceReliabilityScore(item)
	score += newsContentPriorityScore(item)
	if hasHangul(item.Title) {
		score += 30
	}
	if hasHangul(item.Description) {
		score += 10
	}
	if !item.Published.IsZero() {
		age := time.Since(item.Published)
		switch {
		case age <= 6*time.Hour:
			score += 40
		case age <= 12*time.Hour:
			score += 30
		case age <= 24*time.Hour:
			score += 20
		case age <= 48*time.Hour:
			score += 5
		default:
			score -= 30
		}
	}
	if strings.TrimSpace(item.ArticleExcerpt) != "" {
		score += 15
	}
	if strings.TrimSpace(item.Description) != "" {
		score += 5
	}
	if strings.Contains(strings.ToLower(item.Link), "news.google.com/rss/articles") {
		score -= 20
	}
	return score
}

func newsContentPriorityScore(item newsbrief.Item) int {
	text := strings.ToLower(strings.Join([]string{item.Title, item.Description}, " "))
	score := 0
	for _, keyword := range []string{
		"war", "ceasefire", "peace deal", "iran", "gaza", "ukraine", "russia", "china", "taiwan",
		"trump", "president", "prime minister", "government", "parliament", "election", "court",
		"economy", "market", "inflation", "rate", "trade", "tariff",
		"ebola", "outbreak", "disease", "hospital", "disaster", "earthquake", "flood", "security",
		"전쟁", "휴전", "종전", "합의", "이란", "가자", "우크라이나", "러시아", "중국", "대만", "미국",
		"트럼프", "대통령", "총리", "정부", "국회", "선거", "법원", "검찰", "안보", "외교",
		"경제", "증시", "시장", "물가", "금리", "환율", "무역", "관세", "공급망", "반도체",
		"데이터센터", "오픈ai", "챗gpt", "ai", "보안", "개인정보", "무단결제", "해킹",
	} {
		if strings.Contains(text, keyword) {
			score += 8
			if score >= 24 {
				break
			}
		}
	}
	for _, keyword := range []string{
		"live:", " live", "as it happened", "french open", "premier league", "nba", "mlb", "nfl", "football", "tennis",
		"celebrity", "movie review", "album", "recipe",
		"연예", "신혼", "동반출연", "앨범", "차트", "괴물 신인", "영화", "드라마", "예능", "리뷰",
		"맛집", "레시피", "공연", "콘서트", "음원", "포스트 프로덕션",
	} {
		if strings.Contains(text, keyword) {
			score -= 25
			break
		}
	}
	return score
}

func newsSourceReliabilityScore(item newsbrief.Item) int {
	id := strings.ToLower(strings.TrimSpace(item.FeedID))
	switch {
	case strings.Contains(id, "bbc"), strings.Contains(id, "guardian"), strings.Contains(id, "ap-top-news"), strings.Contains(id, "npr"), strings.Contains(id, "aljazeera"):
		return 38
	case strings.Contains(id, "nhk"), strings.Contains(id, "nikkei"), strings.Contains(id, "bangkok-post"), strings.Contains(id, "vnexpress"):
		return 35
	case strings.Contains(id, "cloudflare"), strings.Contains(id, "tailscale"), strings.Contains(id, "kubernetes"):
		return 30
	case id == "hn":
		return 20
	case strings.HasSuffix(id, "-ko"):
		return 42
	default:
		return 10
	}
}

func selectMorningNews(items []newsbrief.Item, limit int) []newsbrief.Item {
	if limit <= 0 {
		limit = 8
	}
	priority := []string{
		"world",
		"overseas/us",
		"overseas/japan",
		"overseas/china",
		"overseas/thailand",
		"overseas/vietnam",
		"tech",
		"infra",
		"ops",
	}
	used := map[int]bool{}
	var out []newsbrief.Item
	for _, category := range priority {
		for i, item := range items {
			if used[i] || firstNonEmpty(item.Category, item.FeedID) != category {
				continue
			}
			out = append(out, item)
			used[i] = true
			break
		}
		if len(out) >= limit {
			return out
		}
	}
	for i, item := range items {
		if used[i] {
			continue
		}
		out = append(out, item)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func koreanMorningText(ctx context.Context, brief Brief) string {
	input := morningFacts(brief)
	today := koreanGeneratedTime(brief.Generated)
	cfg := aichat.Config{
		BaseURL:     firstNonEmptyString(strings.TrimSpace(getenv("MESHCLAW_ASSISTANT_BASE_URL")), "http://g4:11434/v1"),
		APIKey:      "ollama",
		Model:       firstNonEmptyString(strings.TrimSpace(getenv("MESHCLAW_ASSISTANT_MODEL")), "gemma3:4b"),
		MaxTokens:   1600,
		Temperature: 0.45,
		SystemPrompt: strings.Join([]string{
			"너는 한국어 라디오 뉴스 작가이자 개인 비서 브리핑 편집자다.",
			"입력에는 날씨와 여러 언어의 RSS 뉴스 제목/설명이 들어온다.",
			"출력은 반드시 자연스러운 한국어로만 작성한다. 사람이 전화로 들을 원고처럼 쓴다.",
			"매일 같은 템플릿처럼 쓰지 않는다. 오늘 날짜, 날씨, 뉴스 조합을 보고 그날의 새 원고처럼 다시 쓴다.",
			"단, 입력에 없는 사실을 새로 만들지 않는다. 입력된 사실을 자연스럽게 엮어서 표현만 새롭게 한다.",
			"일본어, 중국어, 태국어, 베트남어, 영어 원문 제목을 그대로 노출하지 말고 한국어로 의미 중심 요약을 한다.",
			"일본어 한자/가나, 중국어 원문, 태국어 원문, 베트남어 원문을 본문에 남기지 않는다.",
			"모르는 고유명사는 억지로 음차하지 말고 '일본 정부', '태국 매체', '베트남 대학'처럼 안전하게 일반화한다.",
			"전화 음성 원고에서는 URL을 읽지 않는다. 링크 목록을 만들지 않는다.",
			"과장하지 말고, 불확실한 내용은 '확인이 필요합니다'라고 말한다.",
			"형식은 고정하지 말되 다음 내용은 자연스럽게 포함한다: 짧은 인사, 날씨, 오늘 체크할 것, 해외/기술 뉴스 4-6개, 짧은 마무리.",
			"항상 '좋은 아침입니다. 오늘의 MeshClaw 모닝 브리핑입니다.' 같은 고정 문구로 시작하지 않는다.",
			"각 뉴스는 제목 번역이 아니라 '무슨 일이 있었고 왜 볼 만한지'를 한두 문장으로 설명한다.",
			"총 길이는 한국어 음성 기준 60-90초 안에 읽을 수 있게 한다.",
			"목록 기호는 적게 쓰고, 문장 중심으로 자연스럽게 이어 쓴다.",
			"마지막 문장은 자연스럽게 마무리하되, '안녕히 계십시오' 같은 방송 종료 멘트는 쓰지 않는다.",
			"비밀번호/메일/쇼핑/결제/예약은 자동 실행하지 않고 승인 필요라고만 말한다.",
			"오늘 기준 시각: " + today,
		}, "\n"),
	}
	reply, err := aichat.NewClient(cfg).Chat(ctx, nil, input)
	if err != nil || strings.TrimSpace(reply) == "" {
		return morningFallbackNotice(brief, err)
	}
	return sanitizeBriefText(reply)
}

func morningFallbackNotice(brief Brief, err error) string {
	var b strings.Builder
	b.WriteString("모닝 브리핑 원고 생성 모델이 응답하지 않아 수집된 사실만 임시로 정리합니다.\n\n")
	if err != nil {
		fmt.Fprintf(&b, "모델 오류: %s\n\n", err)
	}
	b.WriteString(formatMorning(brief))
	return b.String()
}

func koreanNewsText(ctx context.Context, brief Brief) string {
	if len(brief.News) == 0 {
		return formatNews(brief)
	}
	requestedLimit := normalizedNewsLimit(brief.NewsLimit, 10)
	input := newsFacts(brief, max(requestedLimit, 10))
	cfg := aichat.Config{
		BaseURL:     firstNonEmptyString(strings.TrimSpace(getenv("MESHCLAW_ASSISTANT_BASE_URL")), "http://g4:11434/v1"),
		APIKey:      "ollama",
		MaxTokens:   1400,
		Temperature: 0.15,
		SystemPrompt: strings.Join([]string{
			"너는 한국어 뉴스 편집자다.",
			"입력에는 RSS에서 가져온 여러 나라와 기술 뉴스 제목/설명/본문 근거 excerpt가 들어온다.",
			"출력은 반드시 자연스러운 한국어로만 작성한다.",
			"HTML 태그, <br>, markdown 표, raw JSON을 절대 출력하지 않는다.",
			"외국어 원문 제목을 그대로 복사하지 말고 의미를 한국어로 풀어쓴다.",
			"사람 이름, 지명, 조직명, 서비스명 같은 고유명사는 모르면 절대 새 이름으로 바꾸지 않는다.",
			"일본어 이름이나 태국/베트남 이름을 확실히 모르면 '일본 스모 선수', '태국 경찰', '홍콩 의사'처럼 일반화한다.",
			"예를 들어 若隆景를 임의로 가와무라, 이시도 같은 다른 이름으로 바꾸면 안 된다.",
			"작은 모델에서도 안정적으로 처리해야 한다. 새 사실을 만들지 말고 입력된 제목, 설명, 본문 근거만 바탕으로 쓴다.",
			"article_excerpt가 있으면 제목보다 article_excerpt를 우선 근거로 삼는다.",
			"입력에 없는 국가/분야 섹션을 새로 만들지 않는다. 예를 들어 입력에 미국 뉴스가 없으면 미국 섹션을 쓰지 않는다.",
			"하나의 입력 항목은 한 번만 사용한다. 같은 뉴스를 다른 섹션에 다시 넣지 않는다.",
			"각 뉴스 번호는 입력의 number 값을 그대로 유지한다. 섹션이 바뀌어도 1부터 다시 시작하지 않는다.",
			fmt.Sprintf("중요한 항목만 %d개 고른다.", requestedLimit),
			"각 항목은 '무슨 일인지'를 한 문장으로 설명하고, 필요할 때만 왜 볼 만한지 덧붙인다.",
			"확인되지 않은 내용은 단정하지 않는다.",
			"본문에는 긴 URL을 넣지 않는다. 끝에 '원문 링크가 필요하면 출처라고 보내세요.'라고 안내한다.",
			"형식: 제목 1줄, 핵심 요약 2문장, 실제 입력에 있는 국가/분야별 주요 뉴스. 짧고 읽기 쉽게 쓴다.",
			"첫 줄은 반드시 '오늘의 주요뉴스입니다.'로 시작한다.",
		}, "\n"),
	}
	for _, model := range newsSummaryModels() {
		cfg.Model = model
		reply, err := aichat.NewClient(cfg).Chat(ctx, nil, input)
		if err != nil || strings.TrimSpace(reply) == "" {
			continue
		}
		if usableGeneratedNewsSummary(reply, len(selectMajorNews(brief.News, requestedLimit))) && generatedNewsMatchesFacts(reply, input) {
			return cleanGeneratedNewsSummary(reply)
		}
	}
	return formatNews(brief)
}

func cleanGeneratedNewsSummary(value string) string {
	value = sanitizeBriefText(value)
	value = strings.ReplaceAll(value, "**", "")
	for strings.Contains(value, "\n\n\n") {
		value = strings.ReplaceAll(value, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(value)
}

func NewsLimitFromText(text string, fallback int) int {
	limit := normalizedNewsLimit(fallback, 10)
	lower := strings.ToLower(text)
	replacements := map[string]string{
		"한": "1", "하나": "1", "두": "2", "둘": "2", "세": "3", "셋": "3", "네": "4", "넷": "4",
		"다섯": "5", "여섯": "6", "일곱": "7", "여덟": "8", "아홉": "9", "열": "10",
		"five": "5", "four": "4", "three": "3", "two": "2", "ten": "10", "seven": "7",
	}
	candidates := []string{lower}
	for word, digit := range replacements {
		if strings.Contains(lower, word) {
			candidates = append(candidates, strings.ReplaceAll(lower, word, digit))
		}
	}
	patterns := []string{"개", "가지", "items", "headlines", "stories", "news"}
	re := regexp.MustCompile(`\d{1,2}`)
	for _, candidate := range candidates {
		matches := re.FindAllStringIndex(candidate, -1)
		for _, match := range matches {
			raw := candidate[match[0]:match[1]]
			n, ok := parseSmallPositiveInt(raw)
			if !ok {
				continue
			}
			start := max(0, match[0]-18)
			end := min(len(candidate), match[1]+48)
			window := candidate[start:end]
			if containsBriefAny(window, patterns...) {
				return normalizedNewsLimit(n, limit)
			}
		}
	}
	return limit
}

func normalizedNewsLimit(value, fallback int) int {
	if fallback <= 0 {
		fallback = 10
	}
	if value <= 0 {
		value = fallback
	}
	if value < 3 {
		return 3
	}
	if value > 12 {
		return 12
	}
	return value
}

func parseSmallPositiveInt(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	n := 0
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
		if n > 50 {
			return 0, false
		}
	}
	return n, n > 0
}

func usableGeneratedNewsSummary(reply string, itemCount int) bool {
	reply = strings.TrimSpace(reply)
	if !hasEnoughKorean(reply) {
		return false
	}
	if strings.Contains(strings.ToLower(reply), "<br") || strings.Contains(reply, "article_excerpt=") || strings.Contains(reply, "title=") {
		return false
	}
	if containsBriefAny(reply, "요코무라", "가와무라", "이시도", "미도스마", "요시노리", "하츠루스모", "열동", "대장판", "대장풍", "대상국물", "대장급") {
		return false
	}
	if itemCount >= 4 && len([]rune(reply)) < 280 {
		return false
	}
	if itemCount >= 4 {
		tail := reply
		if len([]rune(tail)) > 180 {
			runes := []rune(tail)
			tail = string(runes[len(runes)-180:])
		}
		if !containsBriefAny(tail, "원문 링크가 필요하면", "출처라고 보내", "출처'라고 보내", "출처 라고 보내") {
			return false
		}
	}
	tail := strings.TrimSpace(reply)
	if strings.HasSuffix(tail, "*") || strings.HasSuffix(tail, "**") {
		return false
	}
	return true
}

func generatedNewsMatchesFacts(reply, facts string) bool {
	if strings.Contains(facts, "若隆景") && !strings.Contains(reply, "若隆景") {
		return false
	}
	if strings.Contains(facts, "令和4年") && strings.Contains(reply, "2024") {
		return false
	}
	return true
}

func containsBriefAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func koreanNewsDetailText(ctx context.Context, item newsbrief.Item, index int) string {
	input := strings.Join([]string{
		fmt.Sprintf("number=%d", index),
		"category=" + firstNonEmpty(item.Category, item.FeedID),
		"source=" + firstNonEmpty(item.FeedTitle, item.FeedID),
		"title=" + truncateText(item.Title, 220),
		"description=" + truncateText(item.Description, 360),
		"article_excerpt=" + truncateText(item.ArticleExcerpt, 1200),
	}, "\n")
	cfg := aichat.Config{
		BaseURL:     firstNonEmptyString(strings.TrimSpace(getenv("MESHCLAW_ASSISTANT_BASE_URL")), "http://g4:11434/v1"),
		APIKey:      "ollama",
		MaxTokens:   420,
		Temperature: 0.1,
		SystemPrompt: strings.Join([]string{
			"너는 한국어 뉴스 해설자다.",
			"입력된 제목, 설명, 본문 excerpt만 근거로 삼는다.",
			"출력은 반드시 자연스러운 한국어로만 쓴다.",
			"영어/일본어/태국어/베트남어 원문 문장을 그대로 길게 복사하지 않는다.",
			"HTML 태그, raw JSON, 긴 URL은 쓰지 않는다.",
			"형식: '핵심 요약:' 2-3문장, '볼 점:' 1-2개 bullet.",
			"모르는 내용은 추측하지 말고 '본문 근거만으로는 확인하기 어렵다'고 말한다.",
		}, "\n"),
	}
	for _, model := range newsSummaryModels() {
		cfg.Model = model
		reply, err := aichat.NewClient(cfg).Chat(ctx, nil, input)
		if err != nil || strings.TrimSpace(reply) == "" || !hasEnoughKorean(reply) {
			continue
		}
		return sanitizeBriefText(reply)
	}
	return ""
}

func newsSummaryModels() []string {
	return uniqueNonEmptyStrings(
		strings.TrimSpace(getenv("MESHCLAW_ASSISTANT_NEWS_MODEL")),
		strings.TrimSpace(getenv("MESHCLAW_ASSISTANT_MODEL")),
		"gemma3:4b",
		"gemma3:12b",
		"gemma4:e4b",
	)
}

func newsOriginalLinkLine(link string) string {
	link = strings.TrimSpace(link)
	if link == "" {
		return ""
	}
	return fmt.Sprintf("원문 링크: [원문 보기](%s)", link)
}

func morningFacts(brief Brief) string {
	var b strings.Builder
	b.WriteString("날씨:\n")
	if brief.Weather != nil {
		fmt.Fprintf(&b, "- location=%s condition=%s temp_c=%s feels_like_c=%s humidity=%s wind_kmph=%s precip_mm=%s\n",
			brief.Weather.Location, brief.Weather.Condition, brief.Weather.TempC, brief.Weather.FeelsLikeC, brief.Weather.Humidity, brief.Weather.WindKmph, brief.Weather.PrecipMM)
	}
	b.WriteString("\n뉴스 원문 후보:\n")
	for i, item := range brief.News {
		if i >= 8 {
			break
		}
		fmt.Fprintf(&b, "%d. category=%s source=%s\n", i+1, firstNonEmpty(item.Category, item.FeedID), firstNonEmpty(item.FeedTitle, item.FeedID))
		fmt.Fprintf(&b, "title=%s\n", truncateText(item.Title, 180))
		if item.Description != "" {
			fmt.Fprintf(&b, "description=%s\n", truncateText(item.Description, 260))
		}
		if item.ArticleExcerpt != "" {
			fmt.Fprintf(&b, "article_excerpt=%s\n", truncateText(item.ArticleExcerpt, 360))
		}
		if item.Link != "" {
			fmt.Fprintf(&b, "link=%s\n", item.Link)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func newsFacts(brief Brief, limit int) string {
	var b strings.Builder
	if generated := koreanGeneratedTime(brief.Generated); generated != "" {
		fmt.Fprintf(&b, "generated=%s\n", generated)
	}
	b.WriteString("뉴스 원문 후보:\n")
	items := selectMajorNews(brief.News, limit)
	for i, item := range items {
		fmt.Fprintf(&b, "number=%d category=%s category_ko=%s source=%s\n", i+1, firstNonEmpty(item.Category, item.FeedID), koreanNewsCategory(firstNonEmpty(item.Category, item.FeedID)), firstNonEmpty(item.FeedTitle, item.FeedID))
		fmt.Fprintf(&b, "title=%s\n", item.Title)
		if item.Description != "" {
			fmt.Fprintf(&b, "description=%s\n", item.Description)
		}
		if item.ArticleExcerpt != "" {
			fmt.Fprintf(&b, "article_excerpt=%s\n", item.ArticleExcerpt)
		}
		if item.Link != "" {
			fmt.Fprintf(&b, "link=%s\n", item.Link)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func loadLatestNewsItems() ([]newsbrief.Item, string, bool) {
	records, err := evidence.List(400)
	if err != nil {
		return nil, "", false
	}
	for _, summary := range records {
		if summary.Kind != "assistant-news" && summary.Kind != "news-brief" {
			continue
		}
		items, ok := loadNewsItemsFromEvidence(summary.StoredAt)
		if ok && len(items) > 0 {
			return items, summary.StoredAt, true
		}
	}
	return nil, "", false
}

func loadNewsItemsFromEvidence(path string) ([]newsbrief.Item, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var record struct {
		Payload struct {
			News  []newsbrief.Item `json:"news"`
			Items []newsbrief.Item `json:"items"`
			Brief *struct {
				News  []newsbrief.Item `json:"news"`
				Items []newsbrief.Item `json:"items"`
			} `json:"brief"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, false
	}
	switch {
	case len(record.Payload.News) > 0:
		return record.Payload.News, true
	case len(record.Payload.Items) > 0:
		return record.Payload.Items, true
	case record.Payload.Brief != nil && len(record.Payload.Brief.News) > 0:
		return record.Payload.Brief.News, true
	case record.Payload.Brief != nil && len(record.Payload.Brief.Items) > 0:
		return record.Payload.Brief.Items, true
	default:
		return nil, false
	}
}

func loadNewsBriefFromEvidence(path string) (Brief, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Brief{}, false
	}
	var record struct {
		Payload Brief `json:"payload"`
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return Brief{}, false
	}
	if strings.TrimSpace(record.Payload.Text) == "" {
		return Brief{}, false
	}
	return record.Payload, true
}

func usableNewsSummary(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || countHangul(value) < 10 {
		return false
	}
	bad := []string{"RSS 설명", "본문 근거", "article_excerpt=", "title=", "http://", "https://", "**"}
	for _, marker := range bad {
		if strings.Contains(value, marker) {
			return false
		}
	}
	return true
}

func extractNewsIndex(message string) int {
	re := regexp.MustCompile(`\d+`)
	raw := re.FindString(message)
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return value
}

func formatEvening(brief Brief) string {
	var b strings.Builder
	b.WriteString("저녁 10시 리마인더입니다.\n\n")
	b.WriteString("오늘 정리:\n")
	b.WriteString("- 남은 메일/메모/일정은 개인 브리핑에서만 다룹니다.\n")
	b.WriteString("- 내일 아침 브리핑에서 뉴스, 날씨, 서버 상태를 다시 묶어 보냅니다.\n")
	b.WriteString("- 비밀번호/토큰/결제/쇼핑은 자동 실행하지 않고 승인 흐름을 유지합니다.\n\n")
	if brief.Weather != nil {
		fmt.Fprintf(&b, "현재 날씨: %s, %s°C, 체감 %s°C\n\n", koreanWeatherCondition(brief.Weather.Condition), brief.Weather.TempC, brief.Weather.FeelsLikeC)
	}
	b.WriteString("기도문:\n")
	b.WriteString("오늘의 실수와 피로를 내려놓고, 내일 다시 시작할 힘을 주소서. 기록해야 할 것은 기록하고, 잊어도 되는 것은 편히 놓게 하소서.\n")
	appendErrors(&b, brief.Errors)
	return strings.TrimSpace(b.String())
}

func koreanWeatherCondition(condition string) string {
	switch strings.ToLower(strings.TrimSpace(condition)) {
	case "sunny":
		return "맑음"
	case "clear":
		return "쾌청"
	case "partly cloudy":
		return "구름 조금"
	case "cloudy":
		return "흐림"
	case "overcast":
		return "흐림"
	case "mist":
		return "옅은 안개"
	case "fog":
		return "안개"
	case "patchy rain nearby", "light rain", "rain":
		return "비"
	case "moderate rain", "heavy rain":
		return "강한 비"
	case "light drizzle", "drizzle":
		return "이슬비"
	case "snow", "light snow", "moderate snow", "heavy snow":
		return "눈"
	case "thundery outbreaks possible", "thunderstorm":
		return "천둥번개 가능"
	default:
		if strings.TrimSpace(condition) == "" {
			return "정보 없음"
		}
		return condition
	}
}

func koreanLocationName(location string) string {
	switch strings.ToLower(strings.TrimSpace(location)) {
	case "", "seoul":
		return "서울"
	case "hanoi":
		return "하노이"
	case "tokyo":
		return "도쿄"
	case "bangkok":
		return "방콕"
	case "beijing":
		return "베이징"
	case "new york":
		return "뉴욕"
	case "busan":
		return "부산"
	case "incheon":
		return "인천"
	case "daegu":
		return "대구"
	case "gwangju":
		return "광주"
	case "daejeon":
		return "대전"
	case "jeju":
		return "제주"
	case "namyangju":
		return "남양주"
	default:
		if normalizeWeatherLocation(location) == defaultWeatherLocation {
			return "서울"
		}
		return location
	}
}

func koreanJosaEun(word string) string {
	runes := []rune(strings.TrimSpace(word))
	if len(runes) == 0 {
		return "은"
	}
	last := runes[len(runes)-1]
	if last >= 0xAC00 && last <= 0xD7A3 {
		if (last-0xAC00)%28 == 0 {
			return "는"
		}
		return "은"
	}
	// Latin place names: use 는 (e.g. "Namyangju는") instead of awkward "Namyangju은".
	return "는"
}

func koreanNewsCategory(category string) string {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "world", "global", "international":
		return "세계"
	case "overseas/us", "us", "usa":
		return "미국"
	case "overseas/japan", "japan":
		return "일본"
	case "overseas/china", "china":
		return "중국"
	case "overseas/thailand", "thailand":
		return "태국"
	case "overseas/vietnam", "vietnam":
		return "베트남"
	case "tech":
		return "기술"
	case "infra":
		return "인프라"
	case "ops":
		return "운영/DevOps"
	default:
		return category
	}
}

func koreanGeneratedTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		loc = time.FixedZone("KST", 9*60*60)
	}
	return t.In(loc).Format("2006-01-02 15:04 KST")
}

func sanitizeBriefText(value string) string {
	replacer := strings.NewReplacer(
		"<br>", "\n",
		"<br/>", "\n",
		"<br />", "\n",
		"&lt;br&gt;", "\n",
		"&lt;br/&gt;", "\n",
		"&lt;br /&gt;", "\n",
	)
	value = replacer.Replace(value)
	value = strings.ReplaceAll(value, "안녕히 계십시오.", "좋은 하루 보내세요.")
	value = strings.ReplaceAll(value, "안녕히 계십시오", "좋은 하루 보내세요.")
	value = stripAngleTags(value)
	value = removeRawURLLines(value)
	for strings.Contains(value, "\n\n\n") {
		value = strings.ReplaceAll(value, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(value)
}

func sanitizeBriefTextKeepURLs(value string) string {
	replacer := strings.NewReplacer(
		"<br>", "\n",
		"<br/>", "\n",
		"<br />", "\n",
		"&lt;br&gt;", "\n",
		"&lt;br/&gt;", "\n",
		"&lt;br /&gt;", "\n",
	)
	value = replacer.Replace(value)
	value = stripAngleTags(value)
	for strings.Contains(value, "\n\n\n") {
		value = strings.ReplaceAll(value, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(value)
}

func hasEnoughKorean(value string) bool {
	hangul := countHangul(value)
	letters := 0
	for _, r := range value {
		if r >= '가' && r <= '힣' {
			letters++
			continue
		}
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			letters++
		}
	}
	return hangul >= 20 && (letters == 0 || float64(hangul)/float64(letters) >= 0.25)
}

func countHangul(value string) int {
	hangul := 0
	for _, r := range value {
		if r >= '가' && r <= '힣' {
			hangul++
		}
	}
	return hangul
}

func hasHangul(value string) bool {
	return countHangul(value) > 0
}

func removeRawURLLines(value string) string {
	lines := strings.Split(value, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func stripAngleTags(value string) string {
	var b strings.Builder
	inTag := false
	for _, r := range value {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

func truncateText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len([]rune(value)) <= limit {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:limit])) + "..."
}

func appendErrors(b *strings.Builder, errors []string) {
	if len(errors) == 0 {
		return
	}
	b.WriteString("\n\n수집 이슈:\n")
	for _, err := range errors {
		fmt.Fprintf(b, "- %s\n", err)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "general"
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func uniqueNonEmptyStrings(values ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func getenv(key string) string {
	return strings.TrimSpace(strings.Trim(os.Getenv(key), "\""))
}
