package messenger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/meshclaw/meshclaw/internal/lang"
	"github.com/meshclaw/meshclaw/internal/publish"
)

type assistantIndustryNewsProfile struct {
	ID           string
	LabelKey     string
	QueryTerms   []string
	DARTKeywords []string
	SECCompanies []assistantSECCompany
}

type assistantSECCompany struct {
	Name string
	CIK  string
}

func assistantIndustryLatestNewsReply(opts ListenOptions, text string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(text))
	if !isAssistantIndustryLatestNewsRequest(lower, text) {
		return "", false
	}
	profile, _ := assistantIndustryLatestNewsProfileForText(text)
	query := assistantIndustryLatestNewsQuery(text, profile)
	researchDisabled := strings.TrimSpace(os.Getenv("MESHCLAW_ASSISTANT_RESEARCH_DISABLE")) != ""

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	var report publish.ResearchReport
	var researchErr error
	if !researchDisabled {
		report, researchErr = publish.Research(ctx, publish.ResearchOptions{Query: query, Limit: 6, Timeout: 15})
	}
	officialLines := assistantIndustryOfficialDisclosureLines(ctx, profile, researchDisabled)
	_ = opts
	return strings.Join(assistantIndustryLatestNewsLines(text, query, profile, report, officialLines, researchErr, researchDisabled), "\n"), true
}

func assistantIndustryNewsMeetingMaterialsReply(opts ListenOptions, text string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(text))
	if !isAssistantIndustryNewsMeetingMaterialsRequest(lower, text) {
		return "", false
	}
	profile, _ := assistantIndustryLatestNewsProfileForText(text)
	query := assistantIndustryLatestNewsQuery(text, profile)
	researchDisabled := strings.TrimSpace(os.Getenv("MESHCLAW_ASSISTANT_RESEARCH_DISABLE")) != ""
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var report publish.ResearchReport
	var researchErr error
	if !researchDisabled {
		report, researchErr = publish.Research(ctx, publish.ResearchOptions{Query: query, Limit: 6, Timeout: 15})
	}
	officialLines := assistantIndustryOfficialDisclosureLines(ctx, profile, researchDisabled)
	body := strings.Join(assistantIndustryLatestNewsLines(text, query, profile, report, officialLines, researchErr, researchDisabled), "\n")
	args := map[string]interface{}{
		"title":       lang.T(profile.LabelKey) + " 최신 뉴스 회의자료",
		"body":        body,
		"audience":    "회의 참석자",
		"slide_count": inferSlideCount(text, 6),
	}
	if target := inferAssistantSignalTargetRef(text); target != "" {
		args["target"] = target
	}
	argsJSON, _ := json.Marshal(args)
	reply := executeAssistantToolCall(opts, text, "prepare_meeting_materials", string(argsJSON))
	if strings.TrimSpace(reply) != "" {
		reply = strings.TrimRight(reply, "\n") + "\n" + lang.T("assistant.industry_news.materials_includes")
	}
	return reply, true
}

func assistantIndustrySkillDraftNameSummary(text, name, summary string) (string, string, bool) {
	lower := strings.ToLower(strings.TrimSpace(text))
	if !containsAny(lower, "스킬", "skill") || !containsAny(lower, "뉴스", "브리프", "latest", "news", "brief") {
		return name, summary, false
	}
	profile, matched := assistantIndustryLatestNewsProfileForText(text)
	if !matched {
		return name, summary, false
	}
	keyID := profile.ID
	switch keyID {
	case "pharma", "semiconductor":
	default:
		keyID = "generic"
	}
	return lang.T("assistant.industry_news.skill." + keyID + ".name"),
		lang.T("assistant.industry_news.skill." + keyID + ".summary"),
		true
}

func isAssistantIndustryNewsMeetingMaterialsRequest(lower, raw string) bool {
	if lower == "" {
		return false
	}
	if containsAny(lower, "구매", "쇼핑", "쿠팡", "장바구니", "결제", "buy", "purchase", "shopping", "checkout", "cart") {
		return false
	}
	if _, matchedProfile := assistantIndustryLatestNewsProfileForText(raw); !matchedProfile {
		return false
	}
	newsSignal := containsAny(lower,
		"뉴스", "최신", "최근", "이슈", "업계", "산업", "시장조사", "시장 조사", "시장분석", "시장 분석", "리서치", "분석",
		"latest", "recent", "news", "industry", "sector", "market research", "market analysis",
	)
	materialSignal := containsAny(lower,
		"회의자료", "회의 자료", "회의용", "미팅 자료", "자료로", "자료 만들어", "문서로", "문서 만들어", "보고서", "패키지",
		"docx", "pptx", "ppt", "슬라이드", "발표자료", "meeting material", "meeting materials", "deck", "slides", "document", "report",
	)
	return newsSignal && materialSignal
}

func isAssistantIndustryLatestNewsRequest(lower, raw string) bool {
	if lower == "" {
		return false
	}
	if containsAny(lower, "구매", "쇼핑", "쿠팡", "장바구니", "결제", "buy", "purchase", "shopping", "checkout", "cart") {
		return false
	}
	if inferAssistantSignalTargetRef(raw) != "" ||
		containsAny(lower, "ppt", "pptx", "docx", "슬라이드", "발표자료", "패키지", "문서로", "자료로 만들어", "보고방") {
		return false
	}
	if isAssistantMarketResearchMeetingTopicsRequest(lower) {
		return false
	}
	_, matchedProfile := assistantIndustryLatestNewsProfileForText(raw)
	if !matchedProfile {
		return false
	}
	return containsAny(lower,
		"뉴스", "최신", "최근", "이슈", "업계", "산업", "시장조사", "시장 조사", "시장분석", "시장 분석", "리서치", "분석", "정리",
		"latest", "recent", "news", "industry", "sector", "market research", "market analysis", "brief", "summarize",
	)
}

func assistantIndustryLatestNewsProfileForText(text string) (assistantIndustryNewsProfile, bool) {
	lower := strings.ToLower(strings.TrimSpace(text))
	profiles := []assistantIndustryNewsProfile{
		{
			ID:         "pharma",
			LabelKey:   "assistant.industry_news.profile.pharma",
			QueryTerms: []string{"제약", "바이오", "임상", "FDA", "EMA", "MFDS", "약가", "급여", "기술수출", "바이오시밀러", "pharma", "biotech"},
			DARTKeywords: []string{
				"바이오", "제약", "약품", "셀트리온", "삼성바이오", "유한양행", "한미약품", "종근당", "대웅", "녹십자", "SK바이오", "에스케이바이오",
			},
			SECCompanies: []assistantSECCompany{
				{Name: "Pfizer", CIK: "0000078003"},
				{Name: "Merck", CIK: "0000310158"},
				{Name: "Johnson & Johnson", CIK: "0000200406"},
				{Name: "Eli Lilly", CIK: "0000059478"},
				{Name: "AbbVie", CIK: "0001551152"},
				{Name: "Bristol Myers Squibb", CIK: "0000014272"},
				{Name: "Amgen", CIK: "0000318154"},
				{Name: "Gilead", CIK: "0000882095"},
				{Name: "Moderna", CIK: "0001682852"},
			},
		},
		{
			ID:         "semiconductor",
			LabelKey:   "assistant.industry_news.profile.semiconductor",
			QueryTerms: []string{"반도체", "HBM", "메모리", "파운드리", "장비", "수출통제", "semiconductor", "chip", "foundry"},
			DARTKeywords: []string{
				"반도체", "전자", "하이닉스", "삼성전자", "원익", "주성", "한미반도체", "솔브레인", "동진쎄미켐", "리노공업",
			},
			SECCompanies: []assistantSECCompany{
				{Name: "NVIDIA", CIK: "0001045810"},
				{Name: "AMD", CIK: "0000002488"},
				{Name: "Intel", CIK: "0000050863"},
				{Name: "Micron", CIK: "0000723125"},
				{Name: "Qualcomm", CIK: "0000804328"},
				{Name: "Broadcom", CIK: "0001730168"},
				{Name: "TSMC", CIK: "0001046179"},
			},
		},
		{
			ID:           "auto",
			LabelKey:     "assistant.industry_news.profile.auto",
			QueryTerms:   []string{"자동차", "전기차", "배터리", "충전", "자율주행", "EV", "auto", "automotive"},
			DARTKeywords: []string{"자동차", "모비스", "현대차", "기아", "만도", "HL만도", "배터리", "엘앤에프", "에코프로"},
			SECCompanies: []assistantSECCompany{
				{Name: "Tesla", CIK: "0001318605"},
				{Name: "Ford", CIK: "0000037996"},
				{Name: "General Motors", CIK: "0001467858"},
			},
		},
		{
			ID:           "finance",
			LabelKey:     "assistant.industry_news.profile.finance",
			QueryTerms:   []string{"금융", "은행", "증권", "보험", "금리", "대출", "finance", "bank", "securities"},
			DARTKeywords: []string{"금융", "은행", "증권", "보험", "카드", "지주"},
			SECCompanies: []assistantSECCompany{
				{Name: "JPMorgan Chase", CIK: "0000019617"},
				{Name: "Bank of America", CIK: "0000070858"},
				{Name: "Goldman Sachs", CIK: "0000886982"},
				{Name: "Morgan Stanley", CIK: "0000895421"},
				{Name: "Citigroup", CIK: "0000831001"},
			},
		},
		{
			ID:           "energy",
			LabelKey:     "assistant.industry_news.profile.energy",
			QueryTerms:   []string{"에너지", "전력", "원전", "태양광", "석유", "가스", "energy", "power", "oil", "gas"},
			DARTKeywords: []string{"에너지", "전력", "가스", "석유", "정유", "원전", "태양광", "풍력"},
			SECCompanies: []assistantSECCompany{
				{Name: "Exxon Mobil", CIK: "0000034088"},
				{Name: "Chevron", CIK: "0000093410"},
				{Name: "SLB", CIK: "0000087347"},
			},
		},
		{
			ID:           "chemical",
			LabelKey:     "assistant.industry_news.profile.chemical",
			QueryTerms:   []string{"화학", "소재", "정밀화학", "배터리소재", "chemical", "materials"},
			DARTKeywords: []string{"화학", "케미칼", "소재", "정밀화학", "첨단소재", "배터리"},
			SECCompanies: []assistantSECCompany{},
		},
		{
			ID:           "ai",
			LabelKey:     "assistant.industry_news.profile.ai",
			QueryTerms:   []string{"AI", "인공지능", "LLM", "소프트웨어", "클라우드", "SaaS", "software"},
			DARTKeywords: []string{"AI", "인공지능", "소프트웨어", "클라우드", "데이터", "보안"},
			SECCompanies: []assistantSECCompany{
				{Name: "Microsoft", CIK: "0000789019"},
				{Name: "Alphabet", CIK: "0001652044"},
				{Name: "Meta", CIK: "0001326801"},
				{Name: "Amazon", CIK: "0001018724"},
				{Name: "NVIDIA", CIK: "0001045810"},
			},
		},
		{
			ID:           "retail",
			LabelKey:     "assistant.industry_news.profile.retail",
			QueryTerms:   []string{"유통", "커머스", "이커머스", "소매", "retail", "commerce", "e-commerce"},
			DARTKeywords: []string{"유통", "커머스", "쇼핑", "리테일", "마트", "백화점", "편의점"},
			SECCompanies: []assistantSECCompany{
				{Name: "Amazon", CIK: "0001018724"},
				{Name: "Walmart", CIK: "0000104169"},
				{Name: "Costco", CIK: "0000909832"},
			},
		},
	}
	for _, profile := range profiles {
		if assistantIndustryTextMatchesProfile(lower, profile.ID, profile.QueryTerms) {
			return profile, true
		}
	}
	if containsAny(lower, "업종별", "업종", "sector", "industry-specific") {
		return assistantIndustryNewsProfile{
			ID:           "generic",
			LabelKey:     "assistant.market_topics.domain.default",
			QueryTerms:   []string{"업종", "산업", "시장", "규제", "수요", "공급", "경쟁"},
			DARTKeywords: []string{},
			SECCompanies: []assistantSECCompany{},
		}, true
	}
	return assistantIndustryNewsProfile{}, false
}

func assistantIndustryTextMatchesProfile(lower, id string, terms []string) bool {
	if id == "ai" {
		return containsAny(lower, " ai ", "ai 뉴스", "ai latest", "인공지능", "llm", "소프트웨어", "software", "saas", "클라우드")
	}
	if id == "auto" {
		return containsAny(lower, "자동차", "전기차", "배터리", "충전", "자율주행", " ev ", "ev 뉴스", "automotive", "vehicle", "car maker")
	}
	for _, term := range terms {
		if strings.Contains(lower, strings.ToLower(term)) {
			return true
		}
	}
	return false
}

func assistantIndustryLatestNewsQuery(text string, profile assistantIndustryNewsProfile) string {
	request := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	label := lang.T(profile.LabelKey)
	terms := strings.Join(profile.QueryTerms, " ")
	if request == "" {
		request = label
	}
	return strings.TrimSpace(fmt.Sprintf("%s %s DART SEC %s", request, lang.T("assistant.industry_news.query_terms"), terms))
}

func assistantIndustryLatestNewsLines(request, query string, profile assistantIndustryNewsProfile, report publish.ResearchReport, officialLines []string, researchErr error, researchDisabled bool) []string {
	label := lang.T(profile.LabelKey)
	lines := []string{
		lang.T("assistant.industry_news.title", label),
		lang.T("assistant.industry_news.scope", label),
		"",
		lang.T("assistant.industry_news.conclusion_title"),
		"- " + lang.T("assistant.industry_news.conclusion", label),
		"",
		lang.T("assistant.industry_news.status", len(report.Search.Results), len(report.SourcePages)),
	}
	if researchDisabled {
		lines = append(lines, lang.T("assistant.industry_news.disabled"))
	} else if researchErr != nil {
		lines = append(lines, lang.T("assistant.industry_news.error", researchErr.Error()))
	}
	lines = append(lines,
		"",
		lang.T("assistant.industry_news.issues_title"),
	)
	lines = append(lines, assistantIndustryTopicLines(profile)...)
	lines = append(lines,
		"",
		lang.T("assistant.industry_news.metrics_title"),
	)
	lines = append(lines, assistantIndustryMetricLines(profile)...)
	lines = append(lines,
		"",
		lang.T("assistant.industry_news.official_title"),
	)
	if len(officialLines) == 0 {
		lines = append(lines, lang.T("assistant.industry_news.official_none"))
	} else {
		lines = append(lines, officialLines...)
	}
	lines = append(lines,
		"",
		lang.T("assistant.industry_news.sources_title"),
	)
	sourceLines := assistantMarketResearchVisibleSourceLines(report, 4)
	if len(sourceLines) == 0 {
		lines = append(lines, lang.T("assistant.industry_news.no_sources"))
	} else {
		lines = append(lines, sourceLines...)
	}
	lines = append(lines,
		"",
		lang.T("assistant.industry_news.notes_title"),
	)
	noteLines := assistantIndustrySourceNoteLines(report, 3)
	if len(noteLines) == 0 {
		lines = append(lines, lang.T("assistant.industry_news.no_notes"))
	} else {
		lines = append(lines, noteLines...)
	}
	lines = append(lines,
		"",
		lang.T("assistant.industry_news.questions_title"),
	)
	lines = append(lines, assistantIndustryQuestionLines(profile)...)
	lines = append(lines,
		"",
		lang.T("assistant.industry_news.next_title"),
		"- "+lang.T("assistant.industry_news.next.brief"),
		"- "+lang.T("assistant.industry_news.next.package"),
		"- "+lang.T("assistant.industry_news.next.schedule", label),
		"- "+lang.T("assistant.industry_news.next.schedule_approval"),
		"- "+lang.T("assistant.industry_news.next.skill", label),
		"- "+lang.T("assistant.industry_news.next.reuse", label),
		"",
		lang.T("assistant.industry_news.boundary"),
		lang.T("assistant.industry_news.query", query),
	)
	_ = request
	return compactBlankLines(lines)
}

func assistantIndustryTopicLines(profile assistantIndustryNewsProfile) []string {
	keyPrefix := "assistant.industry_news.generic.topic."
	if profile.ID == "pharma" || profile.ID == "semiconductor" {
		keyPrefix = "assistant.industry_news." + profile.ID + ".topic."
	}
	lines := []string{}
	for i := 1; i <= 5; i++ {
		lines = append(lines, fmt.Sprintf("%d. %s", i, lang.T(fmt.Sprintf("%s%d", keyPrefix, i))))
	}
	return lines
}

func assistantIndustryQuestionLines(profile assistantIndustryNewsProfile) []string {
	keyPrefix := "assistant.industry_news.generic.question."
	if profile.ID == "pharma" || profile.ID == "semiconductor" {
		keyPrefix = "assistant.industry_news." + profile.ID + ".question."
	}
	lines := []string{}
	for i := 1; i <= 3; i++ {
		lines = append(lines, fmt.Sprintf("%d. %s", i, lang.T(fmt.Sprintf("%s%d", keyPrefix, i))))
	}
	return lines
}

func assistantIndustryMetricLines(profile assistantIndustryNewsProfile) []string {
	keyPrefix := "assistant.industry_news.generic.metric."
	if profile.ID == "pharma" || profile.ID == "semiconductor" {
		keyPrefix = "assistant.industry_news." + profile.ID + ".metric."
	}
	lines := []string{}
	for i := 1; i <= 4; i++ {
		lines = append(lines, fmt.Sprintf("%d. %s", i, lang.T(fmt.Sprintf("%s%d", keyPrefix, i))))
	}
	return lines
}

func assistantIndustrySourceNoteLines(report publish.ResearchReport, limit int) []string {
	if limit <= 0 {
		limit = 3
	}
	lines := []string{}
	for _, page := range report.SourcePages {
		if len(lines) >= limit {
			break
		}
		if strings.TrimSpace(page.Error) != "" {
			continue
		}
		excerpt := publish.SourceExcerpt(page.Text, 170)
		if excerpt == "" {
			continue
		}
		title := trimSignalListLine(firstNonEmpty(page.Title, page.FinalURL, page.URL), 48)
		lines = append(lines, "- "+title+": "+trimSignalListLine(excerpt, 190))
	}
	return lines
}

func assistantIndustryOfficialDisclosureLines(ctx context.Context, profile assistantIndustryNewsProfile, disabled bool) []string {
	lines := []string{}
	dartKey := firstNonEmpty(os.Getenv("MESHCLAW_DART_API_KEY"), os.Getenv("OPENDART_API_KEY"), os.Getenv("DART_API_KEY"))
	secUserAgent := firstNonEmpty(os.Getenv("MESHCLAW_SEC_USER_AGENT"), os.Getenv("SEC_USER_AGENT"))
	if disabled {
		lines = append(lines, lang.T("assistant.industry_news.official_disabled"))
		lines = append(lines, assistantIndustryPublicDisclosureLines(profile, true, true)...)
		return lines
	}
	if dartKey == "" {
		lines = append(lines, assistantIndustryPublicDisclosureLine("DART", assistantIndustryDARTPublicURL(), assistantIndustryDARTPublicQuery(profile)))
	} else {
		items, err := assistantFetchDARTDisclosures(ctx, dartKey, profile, 5)
		if err != nil {
			lines = append(lines, lang.T("assistant.industry_news.official_error", "DART", err.Error()))
			lines = append(lines, assistantIndustryPublicDisclosureLine("DART", assistantIndustryDARTPublicURL(), assistantIndustryDARTPublicQuery(profile)))
		} else if len(items) == 0 {
			lines = append(lines, assistantIndustryPublicDisclosureLine("DART", assistantIndustryDARTPublicURL(), assistantIndustryDARTPublicQuery(profile)))
		} else {
			lines = append(lines, items...)
		}
	}
	if secUserAgent == "" {
		lines = append(lines, assistantIndustryPublicDisclosureLine("SEC EDGAR", assistantIndustrySECPublicURL(profile), assistantIndustrySECPublicQuery(profile)))
	} else {
		items, err := assistantFetchSECDisclosures(ctx, secUserAgent, profile, 5)
		if err != nil {
			lines = append(lines, lang.T("assistant.industry_news.official_error", "SEC", err.Error()))
			lines = append(lines, assistantIndustryPublicDisclosureLine("SEC EDGAR", assistantIndustrySECPublicURL(profile), assistantIndustrySECPublicQuery(profile)))
		} else if len(items) == 0 {
			lines = append(lines, assistantIndustryPublicDisclosureLine("SEC EDGAR", assistantIndustrySECPublicURL(profile), assistantIndustrySECPublicQuery(profile)))
		} else {
			lines = append(lines, items...)
		}
	}
	if len(lines) == 0 {
		return []string{lang.T("assistant.industry_news.official_none")}
	}
	return lines
}

func assistantIndustryPublicDisclosureLines(profile assistantIndustryNewsProfile, includeDART, includeSEC bool) []string {
	lines := []string{lang.T("assistant.industry_news.official_public_note")}
	if includeDART {
		lines = append(lines, assistantIndustryPublicDisclosureLine("DART", assistantIndustryDARTPublicURL(), assistantIndustryDARTPublicQuery(profile)))
	}
	if includeSEC {
		lines = append(lines, assistantIndustryPublicDisclosureLine("SEC EDGAR", assistantIndustrySECPublicURL(profile), assistantIndustrySECPublicQuery(profile)))
	}
	return lines
}

func assistantIndustryPublicDisclosureLine(source, link, query string) string {
	return lang.T("assistant.industry_news.official_public_line", source, link, trimSignalListLine(query, 80))
}

func assistantIndustryDARTPublicURL() string {
	return "https://dart.fss.or.kr/dsab007/main.do"
}

func assistantIndustryDARTPublicQuery(profile assistantIndustryNewsProfile) string {
	return assistantIndustryPublicQuery(profile, profile.DARTKeywords)
}

func assistantIndustrySECPublicURL(profile assistantIndustryNewsProfile) string {
	query := url.QueryEscape(assistantIndustrySECPublicQuery(profile))
	if query == "" {
		return "https://www.sec.gov/edgar/search/"
	}
	return "https://www.sec.gov/edgar/search/#/q=" + query + "&dateRange=30d"
}

func assistantIndustrySECPublicQuery(profile assistantIndustryNewsProfile) string {
	parts := []string{}
	for _, company := range profile.SECCompanies {
		if strings.TrimSpace(company.Name) == "" {
			continue
		}
		parts = append(parts, company.Name)
		if len(parts) >= 4 {
			break
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, " OR ")
	}
	return assistantIndustryPublicQuery(profile, profile.QueryTerms)
}

func assistantIndustryPublicQuery(profile assistantIndustryNewsProfile, terms []string) string {
	parts := []string{}
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		parts = append(parts, term)
		if len(parts) >= 5 {
			break
		}
	}
	if len(parts) == 0 {
		label := strings.TrimSpace(lang.T(profile.LabelKey))
		if label != "" {
			parts = append(parts, label)
		}
	}
	return strings.Join(parts, " ")
}

type assistantDARTListResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	List    []struct {
		CorpName string `json:"corp_name"`
		ReportNM string `json:"report_nm"`
		RceptNo  string `json:"rcept_no"`
		RceptDT  string `json:"rcept_dt"`
	} `json:"list"`
}

func assistantFetchDARTDisclosures(ctx context.Context, apiKey string, profile assistantIndustryNewsProfile, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 5
	}
	end := time.Now().Format("20060102")
	start := time.Now().AddDate(0, -1, 0).Format("20060102")
	endpoint, _ := url.Parse("https://opendart.fss.or.kr/api/list.json")
	q := endpoint.Query()
	q.Set("crtfc_key", apiKey)
	q.Set("bgn_de", start)
	q.Set("end_de", end)
	q.Set("last_reprt_at", "Y")
	q.Set("page_count", "100")
	q.Set("page_no", "1")
	endpoint.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "MeshClaw Argos industry disclosure brief")
	resp, err := (&http.Client{Timeout: 12 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	var parsed assistantDARTListResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	if parsed.Status != "" && parsed.Status != "000" && parsed.Status != "013" {
		return nil, fmt.Errorf(firstNonEmpty(parsed.Message, parsed.Status))
	}
	lines := []string{}
	for _, item := range parsed.List {
		if len(lines) >= limit {
			break
		}
		title := strings.TrimSpace(strings.TrimSpace(item.CorpName) + " " + strings.TrimSpace(item.ReportNM))
		if title == "" || !assistantDisclosureMatchesIndustry(title, profile.DARTKeywords) {
			continue
		}
		date := assistantFormatDisclosureDate(item.RceptDT)
		link := "https://dart.fss.or.kr/dsaf001/main.do?rcpNo=" + url.QueryEscape(strings.TrimSpace(item.RceptNo))
		lines = append(lines, lang.T("assistant.industry_news.official_line", "DART", date, trimSignalListLine(title, 90), link))
	}
	return lines, nil
}

func assistantDisclosureMatchesIndustry(text string, keywords []string) bool {
	if len(keywords) == 0 {
		return true
	}
	lower := strings.ToLower(text)
	for _, keyword := range keywords {
		if strings.Contains(lower, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

type assistantSECSubmissionsResponse struct {
	Name    string `json:"name"`
	Filings struct {
		Recent struct {
			AccessionNumber []string `json:"accessionNumber"`
			FilingDate      []string `json:"filingDate"`
			Form            []string `json:"form"`
			PrimaryDocument []string `json:"primaryDocument"`
		} `json:"recent"`
	} `json:"filings"`
}

func assistantFetchSECDisclosures(ctx context.Context, userAgent string, profile assistantIndustryNewsProfile, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 5
	}
	if len(profile.SECCompanies) == 0 {
		return nil, nil
	}
	client := &http.Client{Timeout: 12 * time.Second}
	lines := []string{}
	for _, company := range profile.SECCompanies {
		if len(lines) >= limit {
			break
		}
		cik := assistantNormalizeSECcik(company.CIK)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://data.sec.gov/submissions/CIK"+cik+".json", nil)
		if err != nil {
			return lines, err
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return lines, err
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		resp.Body.Close()
		if readErr != nil {
			return lines, readErr
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return lines, fmt.Errorf("SEC %s http %d", company.Name, resp.StatusCode)
		}
		var parsed assistantSECSubmissionsResponse
		if err := json.Unmarshal(data, &parsed); err != nil {
			return lines, err
		}
		name := firstNonEmpty(parsed.Name, company.Name)
		recent := parsed.Filings.Recent
		for i := range recent.Form {
			if len(lines) >= limit {
				break
			}
			if i >= len(recent.AccessionNumber) || i >= len(recent.FilingDate) || i >= len(recent.PrimaryDocument) {
				continue
			}
			form := strings.TrimSpace(recent.Form[i])
			if !assistantSECFormIsUseful(form) {
				continue
			}
			accession := strings.TrimSpace(recent.AccessionNumber[i])
			document := strings.TrimSpace(recent.PrimaryDocument[i])
			date := strings.TrimSpace(recent.FilingDate[i])
			link := assistantSECFilingURL(cik, accession, document)
			title := fmt.Sprintf("%s %s %s", name, form, document)
			lines = append(lines, lang.T("assistant.industry_news.official_line", "SEC", date, trimSignalListLine(title, 90), link))
			break
		}
	}
	return lines, nil
}

func assistantNormalizeSECcik(cik string) string {
	cik = strings.TrimSpace(cik)
	if len(cik) >= 10 {
		return cik
	}
	return strings.Repeat("0", 10-len(cik)) + cik
}

func assistantSECFormIsUseful(form string) bool {
	form = strings.ToUpper(strings.TrimSpace(form))
	switch form {
	case "10-K", "10-Q", "8-K", "20-F", "6-K", "S-1", "F-1":
		return true
	default:
		return strings.HasPrefix(form, "SC 13") || strings.HasPrefix(form, "424B")
	}
}

func assistantSECFilingURL(cik, accession, document string) string {
	cikPath := strings.TrimLeft(assistantNormalizeSECcik(cik), "0")
	if cikPath == "" {
		cikPath = "0"
	}
	accessionPath := strings.ReplaceAll(strings.TrimSpace(accession), "-", "")
	document = strings.TrimSpace(document)
	if accessionPath == "" || document == "" {
		return "https://www.sec.gov/edgar/search/"
	}
	return "https://www.sec.gov/Archives/edgar/data/" + cikPath + "/" + accessionPath + "/" + url.PathEscape(document)
}

func assistantFormatDisclosureDate(value string) string {
	value = strings.TrimSpace(value)
	if len(value) == 8 {
		return value[:4] + "-" + value[4:6] + "-" + value[6:8]
	}
	return value
}
