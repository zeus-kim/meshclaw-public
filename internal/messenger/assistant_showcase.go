package messenger

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/meshclaw/meshclaw/internal/assistantcatalog"
	"github.com/meshclaw/meshclaw/internal/evidence"
	"github.com/meshclaw/meshclaw/internal/lang"
	"github.com/meshclaw/meshclaw/internal/publish"
)

type AssistantShowcaseOptions struct {
	TargetID       string `json:"target_id,omitempty"`
	Mode           string `json:"mode,omitempty"`
	RunSamples     bool   `json:"run_samples,omitempty"`
	IncludePlanned bool   `json:"include_planned,omitempty"`
	WriteMarkdown  bool   `json:"write_markdown,omitempty"`
	Now            time.Time
}

type AssistantShowcaseReport struct {
	Kind         string                    `json:"kind"`
	Generated    time.Time                 `json:"generated"`
	Text         string                    `json:"text"`
	MarkdownPath string                    `json:"markdown_path,omitempty"`
	Attachments  []string                  `json:"attachments,omitempty"`
	Counts       map[string]int            `json:"counts"`
	Samples      []AssistantShowcaseSample `json:"samples,omitempty"`
}

type AssistantShowcaseSample struct {
	Title       string   `json:"title"`
	Prompt      string   `json:"prompt"`
	Status      string   `json:"status"`
	Summary     string   `json:"summary"`
	Attachments []string `json:"attachments,omitempty"`
}

type assistantShowcaseConcreteScenario struct {
	Persona   string
	Situation string
	Request   string
	Result    string
	Signal    string
	FollowUp  string
}

func BuildAssistantShowcaseReport(opts AssistantShowcaseOptions) (AssistantShowcaseReport, error) {
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	catalog := assistantShowcaseCatalog(opts.IncludePlanned)
	counts := assistantShowcaseCounts(catalog)
	samples := []AssistantShowcaseSample(nil)
	if opts.RunSamples {
		samples = runAssistantShowcaseSamples(ListenOptions{
			TargetID: firstNonEmpty(opts.TargetID, "argos-assistant"),
			Mode:     firstNonEmpty(opts.Mode, "assistant"),
		})
	}
	text := assistantShowcaseSignalText(now, catalog, counts, samples, opts.IncludePlanned)
	report := AssistantShowcaseReport{
		Kind:        "meshclaw_assistant_showcase",
		Generated:   now,
		Text:        text,
		Counts:      counts,
		Samples:     samples,
		Attachments: assistantShowcaseSampleAttachments(samples),
	}
	if opts.WriteMarkdown {
		path, err := writeAssistantShowcaseMarkdown(now, catalog, counts, samples, opts.IncludePlanned)
		if err != nil {
			return report, err
		}
		report.MarkdownPath = path
		report.Attachments = uniqueShowcaseAttachments(append([]string{path}, report.Attachments...))
		report.Text = text + "\n\n" + lang.T("assistant.showcase.catalog_attachment")
	}
	return report, nil
}

func AssistantShowcaseText(targetID, mode string) string {
	report, err := BuildAssistantShowcaseReport(AssistantShowcaseOptions{
		TargetID:       targetID,
		Mode:           mode,
		IncludePlanned: true,
	})
	if err != nil {
		return lang.T("assistant.showcase.error", err.Error())
	}
	return report.Text
}

func assistantShowcaseReply(opts ListenOptions, request string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(request))
	if !isAssistantShowcaseRequest(lower) {
		return "", false
	}
	report, err := BuildAssistantShowcaseReport(AssistantShowcaseOptions{
		TargetID:       firstNonEmpty(opts.TargetID, "argos-assistant"),
		Mode:           firstNonEmpty(opts.Mode, "assistant"),
		RunSamples:     assistantShowcaseRunSamplesRequest(lower),
		IncludePlanned: true,
		WriteMarkdown:  true,
		Now:            time.Now().UTC(),
	})
	if err != nil {
		return lang.T("assistant.showcase.error", err.Error()), true
	}
	if assistantShowcaseWantsBriefing(lower) && opts.Execute {
		mobileLinkLines := assistantWorkflowMobileLinkLines(report.Attachments, 6)
		sendLines := appendAssistantWorkflowMobileLinkLines(strings.Split(report.Text, "\n"), mobileLinkLines)
		sendText := strings.Join(compactBlankLines(sendLines), "\n")
		result, sendErr := Send(SendOptions{
			TargetID:       "argos-briefing",
			Kind:           "text",
			Text:           sendText,
			Attachments:    report.Attachments,
			Execute:        true,
			TimeoutSeconds: 90,
		})
		sendRecord, sendStoreErr := evidence.Store("assistant-showcase-send", firstNonEmpty(opts.TargetID, "assistant"), "argos-briefing", map[string]interface{}{
			"kind":             "assistant_showcase_send",
			"attachment_count": len(report.Attachments),
			"markdown_path":    report.MarkdownPath,
			"mobile_links":     mobileLinkLines,
			"send":             result,
			"send_error":       errorString(sendErr),
			"created_at":       time.Now().UTC(),
		})
		if sendErr != nil {
			lines := []string{
				lang.T("assistant.showcase.briefing_failed"),
				lang.T("assistant.showcase.show_here_first"),
				"",
				report.Text,
			}
			lines = appendAssistantWorkflowMobileLinkLines(lines, mobileLinkLines)
			lines = append(lines, assistantShowcaseAttachmentMarkers(report.Attachments)...)
			return strings.Join(appendAssistantEvidenceNote(lines, sendRecord, sendStoreErr), "\n"), true
		}
		lines := []string{lang.T("assistant.showcase.sent_to_briefing")}
		if id := strings.TrimSpace(result.Stdout); id != "" {
			lines = append(lines, lang.T("assistant.showcase.signal_id", id))
		}
		lines = append(lines,
			lang.T("assistant.showcase.sent_samples", len(report.Samples)),
			"",
			lang.T("assistant.showcase.briefing_contains"),
			assistantShowcaseBriefSummary(report),
		)
		return strings.Join(appendAssistantEvidenceNote(lines, sendRecord, sendStoreErr), "\n"), true
	}
	lines := []string{report.Text}
	if assistantShowcaseWantsBriefing(lower) {
		lines = append([]string{
			lang.T("assistant.showcase.briefing_preview"),
			lang.T("assistant.showcase.listener_will_send"),
			"",
		}, lines...)
	}
	lines = appendAssistantWorkflowMobileLinkLines(lines, assistantWorkflowMobileLinkLines(report.Attachments, 6))
	lines = append(lines, assistantShowcaseAttachmentMarkers(report.Attachments)...)
	return strings.Join(lines, "\n"), true
}

func isAssistantShowcaseRequest(lower string) bool {
	lower = strings.ToLower(strings.TrimSpace(lower))
	if lower == "" {
		return false
	}
	compact := strings.NewReplacer(" ", "", "\t", "", "\n", "", ".", "", "!", "", "?", "", "-", "").Replace(lower)
	if containsAny(lower,
		"assistant showcase", "argos showcase", "showcase", "scenario catalog", "scenario catalogue",
		"기능 쇼케이스", "쇼케이스", "시나리오 카탈로그", "시나리오 목록", "구체 시나리오", "300가지", "300개",
		"할 수 있는 모든 일", "할수 있는 모든 일", "할수있는 모든 일", "할 수 있는 일 전부", "할수있는일전부",
		"비서 기능 예제", "기능 예제", "예제 카탈로그", "사용 예제",
	) {
		return true
	}
	return containsAny(compact,
		"할수있는모든일", "할수있는일전부", "비서기능예제", "기능예제", "예제카탈로그",
		"시나리오카탈로그", "구체시나리오", "300가지", "300개",
	)
}

func assistantShowcaseRunSamplesRequest(lower string) bool {
	lower = strings.ToLower(strings.TrimSpace(lower))
	if lower == "" {
		return false
	}
	if forced := strings.ToLower(strings.TrimSpace(os.Getenv("MESHCLAW_ASSISTANT_SHOWCASE_SAMPLES"))); forced != "" {
		return forced == "1" || forced == "true" || forced == "on" || forced == "yes"
	}
	return containsAny(lower,
		"실제", "작동", "작동시켜", "실행", "샘플", "결과물", "파일", "첨부", "시그널로", "signal로",
		"보고방", "보고방에", "회의록", "시장조사", "음성", "여행계획",
	)
}

func assistantShowcaseWantsBriefing(lower string) bool {
	lower = strings.ToLower(strings.TrimSpace(lower))
	return containsAny(lower, "보고방", "브리핑방", "argos-briefing", "보고로", "보고하게", "보고해", "send to briefing", "to briefing", "briefing room")
}

func assistantShowcaseAttachmentMarkers(paths []string) []string {
	lines := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path != "" {
			lines = append(lines, "meshclaw-attachment: "+path)
		}
	}
	return lines
}

func assistantShowcaseBriefSummary(report AssistantShowcaseReport) string {
	lines := []string{
		lang.T("assistant.showcase.summary_counts", report.Counts["total"], report.Counts["implemented"], report.Counts["partial"]),
		lang.T("assistant.showcase.summary_catalog"),
	}
	if len(report.Samples) > 0 {
		lines = append(lines, lang.T("assistant.showcase.summary_samples"))
		for _, sample := range report.Samples {
			lines = append(lines, fmt.Sprintf("  %s: %s", sample.Title, sample.Summary))
			if len(lines) >= 8 {
				break
			}
		}
	}
	if report.MarkdownPath != "" {
		lines = append(lines, lang.T("assistant.showcase.summary_mobile_md"))
	}
	return strings.Join(lines, "\n")
}

func assistantShowcaseCatalog(includePlanned bool) []assistantcatalog.Scenario {
	all := assistantcatalog.Catalog()
	out := make([]assistantcatalog.Scenario, 0, len(all))
	for _, scenario := range all {
		if strings.TrimSpace(scenario.Example) == "" {
			continue
		}
		if !includePlanned && scenario.Status == "planned" {
			continue
		}
		out = append(out, scenario)
	}
	return out
}

func assistantShowcaseCounts(catalog []assistantcatalog.Scenario) map[string]int {
	counts := map[string]int{"total": len(catalog)}
	for _, scenario := range catalog {
		status := strings.TrimSpace(scenario.Status)
		if status == "" {
			status = "unknown"
		}
		counts[status]++
		counts["category:"+scenario.Category]++
	}
	return counts
}

func assistantShowcaseSignalText(now time.Time, catalog []assistantcatalog.Scenario, counts map[string]int, samples []AssistantShowcaseSample, includePlanned bool) string {
	lines := []string{
		lang.T("assistant.showcase.title"),
		lang.T("assistant.showcase.subtitle"),
		lang.T("assistant.showcase.counts", len(catalog), counts["implemented"], counts["partial"]),
		lang.T("assistant.showcase.catalog", 300),
		"",
		lang.T("assistant.showcase.visible_work"),
		"1. " + lang.T("assistant.showcase.work.1"),
		"2. " + lang.T("assistant.showcase.work.2"),
		"3. " + lang.T("assistant.showcase.work.3"),
		"4. " + lang.T("assistant.showcase.work.4"),
		"5. " + lang.T("assistant.showcase.work.5"),
	}
	lines = append(lines, "", lang.T("assistant.showcase.result_examples"))
	for _, example := range assistantShowcaseVisibleResultExamples() {
		lines = append(lines, "- "+example)
	}
	if len(samples) > 0 {
		lines = append(lines, "", lang.T("assistant.showcase.samples"))
		for _, sample := range samples {
			lines = append(lines, fmt.Sprintf("- %s: %s - %s", sample.Title, sample.Status, sample.Summary))
		}
		if attachments := assistantShowcaseSampleAttachments(samples); len(attachments) > 0 {
			lines = append(lines, lang.T("assistant.showcase.generated_files", len(attachments)))
		}
	}
	lines = append(lines, "", lang.T("assistant.showcase.commands"))
	for _, example := range assistantShowcaseRepresentativeExamples(catalog, 8) {
		lines = append(lines, "- "+example)
	}
	if includePlanned {
		lines = append(lines, "", lang.T("assistant.showcase.planned_note"))
	}
	lines = append(lines, lang.T("assistant.showcase.generated_at", now.In(time.FixedZone("KST", 9*60*60)).Format("2006-01-02 15:04 KST")))
	return strings.Join(lines, "\n")
}

func assistantShowcaseVisibleResultExamples() []string {
	examples := make([]string, 0, 13)
	for i := 1; i <= 13; i++ {
		key := fmt.Sprintf("assistant.showcase.example.%d", i)
		if text := lang.T(key); text != key {
			examples = append(examples, text)
		}
	}
	return examples
}

func assistantShowcaseRepresentativeExamples(catalog []assistantcatalog.Scenario, limit int) []string {
	preferred := []string{
		"doc_meeting_minutes",
		"market_outlook",
		"voice_news_brief",
		"mail_summary",
		"mail_draft_reply",
		"calendar_list",
		"scheduled_delivery",
		"ops_voice_report",
	}
	byID := map[string]assistantcatalog.Scenario{}
	for _, scenario := range catalog {
		byID[scenario.ID] = scenario
	}
	examples := []string{}
	for _, id := range preferred {
		if scenario, ok := byID[id]; ok && scenario.Example != "" {
			examples = append(examples, scenario.Example)
		}
	}
	for _, scenario := range catalog {
		if len(examples) >= limit {
			break
		}
		if scenario.Example == "" || containsString(examples, scenario.Example) {
			continue
		}
		examples = append(examples, scenario.Example)
	}
	if len(examples) > limit {
		return examples[:limit]
	}
	return examples
}

func runAssistantShowcaseSamples(opts ListenOptions) []AssistantShowcaseSample {
	cases := []struct {
		title  string
		prompt string
		name   string
		args   string
	}{
		{title: "뉴스", prompt: "오늘 주요뉴스 알려줘", name: "get_news_headlines", args: `{"limit":3}`},
		{title: "날씨", prompt: "오늘 서울 날씨 알려줘", name: "get_weather", args: `{"location":"Seoul"}`},
		{title: "메일", prompt: "연결된 이메일 뭐 있어?", name: "list_mail_accounts", args: `{}`},
		{title: "회의록 문서", prompt: "회의록 문서로 만들어줘", name: "create_document", args: `{"title":"Argos 쇼케이스 회의록","body":"회의 주제: Argos 비서 기능 쇼케이스\n결정: 사용자에게 내부 점검이 아니라 실제 업무 결과물을 보여준다.\n할 일: 메일, 뉴스, 회의록, 시장조사, 음성 브리핑 예제를 Signal에 보고한다."}`},
		{title: "회의자료 패키지", prompt: "내일 회의자료 패키지 만들어줘", name: "prepare_meeting_materials", args: `{"title":"Argos 비서 제품화 회의","audience":"제품/개발 팀","slide_count":5,"body":"목표: OpenClaw/Hermes 수준의 개인 비서 경험을 Signal에서 체감하게 만든다.\n안건: 1) 메일/뉴스/회의록/시장조사 핵심 흐름 2) Signal 결과물 첨부 3) 매일 자동 보고 4) 사용자가 실제로 맡길 수 있는 스킬 확대\n결정 요청: 다음 스프린트는 런타임 관리보다 사용자-facing 비서 기능을 우선한다."}`},
		{title: "예산표", prompt: "예산표 엑셀 만들어줘", name: "create_spreadsheet", args: `{"title":"Argos 쇼케이스 예산표","body":"| 항목 | 금액 | 비고 |\n| --- | ---: | --- |\n| 뉴스/시장조사 | 0 | 읽기 전용 |\n| 문서/회의록 | 0 | 로컬 파일 생성 |\n| 음성 브리핑 | 0 | edge-tts 기본 |"}`},
		{title: "예약 후보", prompt: "내일 저녁 7시에 강남 파스타 식당 2명 예약 후보 찾아줘", name: "find_booking", args: `{"query":"내일 저녁 7시 강남 파스타 식당 2명 예약 가능"}`},
		{title: "음성", prompt: "Argos 소개를 edge tts 음성파일로 만들어줘", name: "send_tts_voice", args: `{"topic":"Argos 기능 소개","content":"아르고스는 메일, 뉴스, 회의록, 시장조사, 일정, 문서, 음성 브리핑을 Signal에서 처리하는 개인 비서입니다.","engine":"edge-tts","execute":true}`},
	}
	out := make([]AssistantShowcaseSample, 0, len(cases))
	for _, item := range cases {
		reply := executeAssistantToolCall(opts, item.prompt, item.name, item.args)
		status, summary := assistantShowcaseSampleSummary(reply)
		out = append(out, AssistantShowcaseSample{
			Title:       item.title,
			Prompt:      item.prompt,
			Status:      status,
			Summary:     summary,
			Attachments: signalReplyAttachments(reply),
		})
	}
	out = append(out, runAssistantShowcaseChemicalNewsSample(opts))
	out = append(out, runAssistantShowcaseTravelPlanSample(opts))
	return out
}

func runAssistantShowcaseChemicalNewsSample(opts ListenOptions) AssistantShowcaseSample {
	const title = "화학회사 민감 뉴스 분석"
	const prompt = "최근 화학회사들이 민감하게 반응할만한 뉴스를 찾아서 분석 보고서를 만들어줘"
	query := "chemical industry news PFAS regulation petrochemical supply chain battery materials sanctions tariffs safety 2026"
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	report, _ := publish.Research(ctx, publish.ResearchOptions{Query: query, Limit: 6, Timeout: 15})
	body := assistantChemicalNewsReportBody(query, report)
	args := assistantShowcaseToolArgs(map[string]interface{}{"title": "화학회사 민감 뉴스 분석 보고서", "body": body})
	reply := executeAssistantToolCall(opts, prompt, "create_document", args)
	status, summary := assistantShowcaseSampleSummary(reply)
	attachments := uniqueShowcaseAttachments(append(signalReplyAttachments(reply), assistantPublishResearchAttachments(report)...))
	return AssistantShowcaseSample{
		Title:       title,
		Prompt:      prompt,
		Status:      status,
		Summary:     summary,
		Attachments: attachments,
	}
}

func runAssistantShowcaseTravelPlanSample(opts ListenOptions) AssistantShowcaseSample {
	const title = "일주일 가용시간 기반 여행계획"
	const prompt = "일주일 동안 내가 가장 시간이 많이 나는 시기를 확인하고 여행계획을 잡아줘"
	calendarReply := executeAssistantToolCall(opts, "다음 7일 일정 뭐 있어?", "run_mac_action", `{"prompt":"다음 7일 일정 뭐 있어?"}`)
	query := "Seoul Jeju flight hotel 2 nights next week fare hotel booking"
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	report, _ := publish.Research(ctx, publish.ResearchOptions{Query: query, Limit: 6, Timeout: 15})
	body := assistantTravelPlanReportBody(query, calendarReply, report)
	args := assistantShowcaseToolArgs(map[string]interface{}{"title": "일주일 가용시간 기반 제주 여행계획", "body": body})
	reply := executeAssistantToolCall(opts, prompt, "create_document", args)
	status, summary := assistantShowcaseSampleSummary(reply)
	attachments := uniqueShowcaseAttachments(append(signalReplyAttachments(reply), assistantPublishResearchAttachments(report)...))
	return AssistantShowcaseSample{
		Title:       title,
		Prompt:      prompt,
		Status:      status,
		Summary:     summary,
		Attachments: attachments,
	}
}

func assistantShowcaseToolArgs(values map[string]interface{}) string {
	data, err := json.Marshal(values)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func assistantChemicalNewsReportBody(query string, report publish.ResearchReport) string {
	lines := []string{
		"# 화학회사 민감 뉴스 분석 보고서",
		"",
		"## 내일 회의 핵심 토픽",
		"1. PFAS/영구화학물질 규제와 소송 리스크: 제품 포트폴리오, 고객사 요구, 공시 문구까지 영향을 줄 수 있습니다.",
		"2. 석유화학 스프레드와 원료 가격: 나프타, 천연가스, 운임, 재고 사이클이 마진 회복 시점을 좌우합니다.",
		"3. 배터리 소재와 전기차 수요 둔화: 양극재/분리막/전해액 업체는 고객사 생산계획 변화에 민감합니다.",
		"4. 중국 공급과 무역 규제: 반덤핑, 관세, 수출통제, 중국 증설은 가격과 공급망 협상력을 흔듭니다.",
		"5. 안전/환경 사고와 공장 가동률: 사고, 정기보수, 배출 규제는 단기 공급과 평판에 바로 반영됩니다.",
		"",
		"## 회의에서 던질 질문",
		"- 우리 제품 중 PFAS/REACH/TSCA 이슈에 노출된 매출 비중은 얼마인가?",
		"- 고객사가 요구하는 대체 소재 인증 일정은 언제까지인가?",
		"- 중국 저가 공급 확대에 대해 가격 방어가 가능한 제품군은 무엇인가?",
		"- 원료 가격 10% 변동 시 분기 EBITDA 민감도는 얼마인가?",
		"- 이번 주 확인해야 할 사고/규제/소송 뉴스가 고객사 계약에 미치는 영향은 무엇인가?",
		"",
		"## 검색 쿼리",
		"- " + query,
		"",
		"## 출처 후보",
	}
	lines = append(lines, assistantResearchSourceLines(report)...)
	lines = append(lines,
		"",
		"## 내일 회의용 결론",
		"화학회사는 단일 뉴스보다 규제, 원료, 수요, 공급망, 사고가 동시에 움직일 때 민감하게 반응합니다. 회의에서는 '어떤 뉴스가 주가에 영향을 주었는가'보다 '우리 제품/고객/계약/마진에 직접 연결되는 노출이 무엇인가'를 기준으로 토픽을 정해야 합니다.",
	)
	return strings.Join(lines, "\n")
}

func assistantTravelPlanReportBody(query, calendarReply string, report publish.ResearchReport) string {
	now := time.Now().In(time.FixedZone("KST", 9*60*60))
	lines := []string{
		"# 일주일 가용시간 기반 제주 여행계획",
		"",
		"## 기준",
		"- 생성 시각: " + now.Format("2006-01-02 15:04 KST"),
		"- 여행지 샘플: 제주 2박 3일",
		"- 목적: 다음 7일 중 가장 긴 빈 시간을 찾아 항공/호텔 후보와 일정표까지 한 번에 준비",
		"",
		"## 캘린더 확인 결과",
	}
	for _, line := range strings.Split(signalReplyVisibleText(calendarReply), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.Contains(line, "/Users/") {
			lines = append(lines, "- "+line)
		}
		if len(lines) > 18 {
			break
		}
	}
	lines = append(lines,
		"",
		"## 추천 일정안",
		"- 1안: 금요일 저녁 출발, 일요일 저녁 복귀. 업무일 손실을 최소화하고 숙박은 2박으로 제한합니다.",
		"- 2안: 토요일 오전 출발, 월요일 오전 복귀. 주말 항공권이 비싸면 월요일 오전 복귀로 가격을 낮춥니다.",
		"- 3안: 평일 하루 휴가를 붙여 목요일 저녁 출발, 일요일 복귀. 가장 여유로운 일정입니다.",
		"",
		"## 항공/호텔 확인 쿼리",
		"- "+query,
		"",
		"## 항공/호텔 후보 링크",
	)
	lines = append(lines, assistantResearchSourceLines(report)...)
	lines = append(lines,
		"",
		"## 예약 진행 체크리스트",
		"- 항공: 출발/복귀 시간, 위탁수하물, 취소수수료, 총액 확인",
		"- 호텔: 위치, 체크인 시간, 조식, 무료취소 기한, 총액 확인",
		"- 동선: 공항 이동, 렌터카/택시, 첫날 저녁 식사 후보",
		"- 예약 확정 전 마지막 확인: 이름, 날짜, 결제 금액, 환불 조건",
	)
	return strings.Join(lines, "\n")
}

func assistantResearchSourceLines(report publish.ResearchReport) []string {
	lines := []string{}
	for i, item := range report.Search.Results {
		if i >= 8 {
			break
		}
		title := strings.TrimSpace(item.Text)
		if title == "" {
			title = lang.T("assistant.research_package.source.untitled")
		}
		if strings.TrimSpace(item.URL) != "" {
			lines = append(lines, fmt.Sprintf("%d. %s - %s", i+1, title, strings.TrimSpace(item.URL)))
		} else {
			lines = append(lines, fmt.Sprintf("%d. %s", i+1, title))
		}
	}
	if len(lines) == 0 {
		return []string{"- " + lang.T("assistant.research_package.source.none")}
	}
	return lines
}

func assistantPublishResearchAttachments(report publish.ResearchReport) []string {
	return uniqueShowcaseAttachments([]string{report.Path, report.PreviewImage, report.PreviewPath})
}

func assistantShowcaseSampleAttachments(samples []AssistantShowcaseSample) []string {
	attachments := []string{}
	for _, sample := range samples {
		attachments = append(attachments, sample.Attachments...)
	}
	return uniqueShowcaseAttachments(attachments)
}

func uniqueShowcaseAttachments(paths []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" || seen[path] {
			continue
		}
		if !assistantShowcaseAttachmentAllowed(path) {
			continue
		}
		if st, err := os.Stat(path); err == nil && !st.IsDir() && st.Size() > 0 {
			seen[path] = true
			out = append(out, path)
		}
	}
	return out
}

func assistantShowcaseAttachmentAllowed(path string) bool {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(path)))
	if ext == ".html" || ext == ".htm" {
		return false
	}
	return true
}

func assistantShowcaseSampleSummary(reply string) (string, string) {
	lower := strings.ToLower(reply)
	status := "OK"
	if containsAny(lower, "실패", "오류", "failed", "error", "문제:") {
		status = "주의"
	}
	hasAttachment := strings.Contains(reply, "meshclaw-attachment:")
	clean := []string{}
	for _, line := range strings.Split(reply, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, "meshclaw-attachment:") || strings.Contains(line, "/Users/") || strings.Contains(line, ".meshclaw") || strings.Contains(line, "증거:") || strings.Contains(line, "상세 기록") || strings.Contains(line, "자세한 근거") {
			continue
		}
		clean = append(clean, line)
		if len(clean) >= 2 {
			break
		}
	}
	summary := "실행 결과를 만들었습니다."
	if len(clean) > 0 {
		summary = strings.Join(clean, " ")
	}
	if hasAttachment {
		summary += " 파일도 생성됨."
	}
	return status, trimForContext(summary, 130)
}

func writeAssistantShowcaseMarkdown(now time.Time, catalog []assistantcatalog.Scenario, counts map[string]int, samples []AssistantShowcaseSample, includePlanned bool) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		home = os.TempDir()
	}
	dir := filepath.Join(home, ".meshclaw", "assistant-showcases")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "argos-showcase-"+now.UTC().Format("20060102-150405")+".md")
	content := assistantShowcaseMarkdown(now, catalog, counts, samples, includePlanned)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return "", err
	}
	return path, nil
}

func assistantShowcaseMarkdown(now time.Time, catalog []assistantcatalog.Scenario, counts map[string]int, samples []AssistantShowcaseSample, includePlanned bool) string {
	var b strings.Builder
	b.WriteString("# " + lang.T("assistant.showcase.markdown.title") + "\n\n")
	b.WriteString("- " + lang.T("assistant.showcase.markdown.generated") + ": " + now.In(time.FixedZone("KST", 9*60*60)).Format("2006-01-02 15:04 KST") + "\n")
	b.WriteString(fmt.Sprintf("- %s: %d\n", lang.T("assistant.showcase.markdown.total"), len(catalog)))
	b.WriteString(fmt.Sprintf("- %s: %d\n", lang.T("assistant.showcase.markdown.implemented"), counts["implemented"]))
	b.WriteString(fmt.Sprintf("- %s: %d\n", lang.T("assistant.showcase.markdown.partial"), counts["partial"]))
	if includePlanned {
		b.WriteString(fmt.Sprintf("- %s: %d\n", lang.T("assistant.showcase.markdown.planned"), counts["planned"]))
	}
	if len(samples) > 0 {
		b.WriteString("\n## " + lang.T("assistant.showcase.markdown.samples") + "\n\n")
		for _, sample := range samples {
			b.WriteString("- **" + sample.Title + "** (" + sample.Status + "): " + sample.Summary + "\n")
			b.WriteString("  - " + lang.T("assistant.showcase.markdown.prompt") + ": `" + sample.Prompt + "`\n")
			if len(sample.Attachments) > 0 {
				b.WriteString(fmt.Sprintf("  - %s: %d\n", lang.T("assistant.showcase.markdown.generated_files"), len(sample.Attachments)))
			}
		}
	}
	b.WriteString("\n## " + lang.T("assistant.showcase.markdown.usage") + "\n\n")
	b.WriteString("- " + lang.T("assistant.showcase.markdown.usage.1") + "\n")
	b.WriteString("- " + lang.T("assistant.showcase.markdown.usage.2") + "\n")
	b.WriteString("- " + lang.T("assistant.showcase.markdown.usage.3") + "\n")
	b.WriteString("- " + lang.T("assistant.showcase.markdown.usage.4") + "\n")
	scenarios := assistantShowcaseConcreteScenarios(catalog, 300)
	b.WriteString("\n## " + lang.T("assistant.showcase.markdown.scenarios", len(scenarios)) + "\n\n")
	for i, scenario := range scenarios {
		b.WriteString(fmt.Sprintf("### %03d. %s - %s\n\n", i+1, scenario.Persona, scenario.Situation))
		b.WriteString("- 요청: `" + scenario.Request + "`\n")
		b.WriteString("- Argos 결과: " + scenario.Result + "\n")
		b.WriteString("- Signal에서 보이는 형태: " + scenario.Signal + "\n")
		b.WriteString("- 이어서 할 말: `" + scenario.FollowUp + "`\n\n")
	}
	grouped := map[string][]assistantcatalog.Scenario{}
	categories := []string{}
	for _, scenario := range catalog {
		grouped[scenario.Category] = append(grouped[scenario.Category], scenario)
	}
	for category := range grouped {
		categories = append(categories, category)
	}
	sort.SliceStable(categories, func(i, j int) bool {
		return assistantShowcaseCategoryRank(categories[i]) < assistantShowcaseCategoryRank(categories[j])
	})
	for _, category := range categories {
		label := assistantShowcaseCategoryLabel(category)
		b.WriteString("\n## " + label + "\n\n")
		for _, scenario := range grouped[category] {
			approval := ""
			if scenario.RequiresApproval {
				approval = " / 승인 필요"
			}
			b.WriteString(fmt.Sprintf("- `%s` - %s (%s%s)\n", scenario.Example, scenario.Title, scenario.Status, approval))
		}
	}
	return b.String()
}

func assistantShowcaseConcreteScenarios(catalog []assistantcatalog.Scenario, limit int) []assistantShowcaseConcreteScenario {
	if limit <= 0 {
		return nil
	}
	personas := []string{
		"1인 창업자 김현수", "마케팅 리드 박서연", "개발팀장 이준호", "프리랜서 디자이너 최민지",
		"영업 담당 정우진", "투자 검토자 한지윤", "학부모 오세영", "개인 투자자 문태호",
		"소규모 쇼핑몰 운영자 강나래", "프로덕트 매니저 유다은", "콘텐츠 크리에이터 백도윤", "법무 담당 신혜린",
		"행사 기획자 장민석", "대학원생 임수아", "병원 행정 담당 고은별", "부동산 중개사 차도현",
	}
	situations := map[string][]string{
		"mail":     {"아침 출근 전 받은 메일을 정리해야 하는 상황", "답장해야 할 메일을 놓치면 안 되는 상황", "영수증과 구독 메일을 찾아야 하는 상황"},
		"research": {"회의 전에 최신 이슈를 빠르게 파악해야 하는 상황", "경쟁사 움직임을 주간 보고로 만들어야 하는 상황", "시장 가격과 뉴스 흐름을 확인해야 하는 상황"},
		"files":    {"회의 직후 바로 공유할 문서가 필요한 상황", "보고서 초안을 모바일에서 확인해야 하는 상황", "발표자료와 요약본을 같이 만들어야 하는 상황"},
		"voice":    {"운전 중 읽을 수 없어 음성으로 들어야 하는 상황", "보고 내용을 음성파일로 공유해야 하는 상황", "짧은 안내문을 자연스럽게 읽어야 하는 상황"},
		"time":     {"일정이 겹치는지 확인해야 하는 상황", "오늘 할 일을 놓치지 않아야 하는 상황", "새 약속을 리마인더와 함께 정리해야 하는 상황"},
		"maps":     {"약속 장소까지 가는 길을 바로 열어야 하는 상황", "근처 장소 후보를 고르고 공유해야 하는 상황", "주차 가능 여부를 확인해야 하는 상황"},
		"contacts": {"연락처를 찾아 바로 연락 문구를 만들어야 하는 상황", "늦는다고 보낼 메시지를 깔끔하게 써야 하는 상황", "전화하기 전 말할 내용을 정리해야 하는 상황"},
		"booking":  {"저녁 예약 후보를 빠르게 좁혀야 하는 상황", "출장 항공/호텔 후보를 비교해야 하는 상황", "결제 전 단계까지만 준비해야 하는 상황"},
		"shopping": {"상품 리뷰와 가격을 비교해야 하는 상황", "배송 빠른 옵션을 골라야 하는 상황", "구독/영수증/반품 정보를 확인해야 하는 상황"},
		"media":    {"이동 중 들을 콘텐츠를 고르고 틀어야 하는 상황", "오늘 볼만한 영상이나 OTT 후보가 필요한 상황", "집중용 배경음을 켜야 하는 상황"},
		"mac":      {"Mac mini에서 앱이나 파일을 바로 열어야 하는 상황", "화면 캡처나 녹화로 작업 결과를 보여야 하는 상황", "클립보드나 단축어로 반복 작업을 줄여야 하는 상황"},
		"ops":      {"서버 상태를 사람이 읽는 보고로 받아야 하는 상황", "보안/로그 이상 징후를 빠르게 확인해야 하는 상황", "운영 보고를 음성으로 듣고 싶어 하는 상황"},
		"daily":    {"하루 시작 전에 날씨와 할 일을 알고 싶은 상황", "저녁에 오늘 처리한 일을 정리해야 하는 상황", "가족/개인 일정 흐름을 정리해야 하는 상황"},
		"approval": {"삭제/결제/발송 같은 마지막 클릭을 막아야 하는 상황", "승인 대기 작업을 한눈에 봐야 하는 상황", "자동화 허용 범위를 좁게 정해야 하는 상황"},
		"identity": {"처음 쓰는 사용자가 무엇을 맡길 수 있는지 묻는 상황", "비서가 어떤 규칙으로 행동하는지 확인하는 상황", "동료에게 사용 예시를 보여줘야 하는 상황"},
	}
	active := []assistantcatalog.Scenario{}
	for _, scenario := range catalog {
		if scenario.Status != "implemented" && scenario.Status != "partial" {
			continue
		}
		if strings.TrimSpace(scenario.Example) != "" {
			active = append(active, scenario)
		}
	}
	if len(active) == 0 {
		return nil
	}
	cards := make([]assistantShowcaseConcreteScenario, 0, limit)
	for i := 0; len(cards) < limit; i++ {
		scenario := active[i%len(active)]
		categorySituations := situations[scenario.Category]
		if len(categorySituations) == 0 {
			categorySituations = []string{"바로 결과가 필요한 상황"}
		}
		card := assistantShowcaseConcreteScenario{
			Persona:   personas[i%len(personas)],
			Situation: categorySituations[(i/len(active)+i)%len(categorySituations)],
			Request:   assistantShowcaseScenarioRequest(scenario, i/len(active)),
			Result:    assistantShowcaseScenarioResult(scenario),
			Signal:    assistantShowcaseScenarioSignal(scenario),
			FollowUp:  assistantShowcaseScenarioFollowUp(scenario),
		}
		cards = append(cards, card)
	}
	return cards
}

func assistantShowcaseScenarioRequest(scenario assistantcatalog.Scenario, variant int) string {
	base := strings.TrimSpace(scenario.Example)
	if base == "" {
		base = strings.TrimSpace(scenario.Title)
	}
	if variant <= 0 {
		return base
	}
	switch scenario.Category {
	case "mail":
		return base + " 그리고 답장해야 할 일과 오늘 처리할 순서를 같이 정리해줘"
	case "research":
		return base + " 내일 회의에서 바로 쓸 3가지 토픽과 질문까지 뽑아줘"
	case "files":
		return base + " 모바일에서 열 수 있는 Markdown, DOCX, PPTX 첨부로 같이 만들어줘"
	case "voice":
		return base + " 뉴스방송 원고처럼 자연스럽게 만들어서 음성파일로 보내줘"
	case "time":
		return base + " 이번 주 빈 시간과 겹치는 약속까지 같이 봐줘"
	case "maps":
		return base + " 후보 3개와 이동시간, 주차 여부를 같이 정리해줘"
	case "contacts":
		return base + " 바로 보낼 수 있는 문장과 전화할 때 말할 스크립트도 써줘"
	case "booking":
		return base + " 가격, 취소조건, 일정표, 예약 확인 문구까지 준비해줘"
	case "shopping":
		return base + " 총액, 배송, 반품 조건, 대체 후보까지 비교해줘"
	case "media":
		return base + " 지금 바로 틀 수 있는 링크와 왜 고른 건지 짧게 설명해줘"
	case "mac":
		return base + " 실행 결과를 캡처나 짧은 요약으로 Signal에 보여줘"
	case "ops":
		return base + " 위험도와 다음 조치까지 사람이 읽는 보고서로 보내줘"
	case "daily":
		return base + " 날씨, 일정, 메일, 이동까지 하루 계획으로 묶어줘"
	case "approval":
		return base + " 승인하면 어떤 일이 실행되는지 한 문장으로 먼저 보여줘"
	default:
		return base + " 결과를 보고방에서 바로 읽을 수 있게 정리해줘"
	}
}

func assistantShowcaseScenarioResult(scenario assistantcatalog.Scenario) string {
	switch scenario.Category {
	case "mail":
		return "최근 메일을 읽고 보낸 사람, 제목, 핵심 내용, 처리할 일, 답장 초안을 한 화면에 정리합니다."
	case "research":
		return "뉴스/검색 결과를 읽고 핵심 3줄, 중요한 변화, 사용자에게 의미 있는 다음 행동으로 요약합니다."
	case "files":
		return "Markdown 원본, DOCX/PDF, PowerPoint에서 여는 PPTX, XLSX 편집 파일까지 만들어 Signal 첨부로 보냅니다."
	case "voice":
		return "edge-tts 기본 음성으로 MP3 또는 보이스노트를 만들어 Signal에서 바로 재생할 수 있게 보냅니다."
	case "time":
		return "캘린더와 리마인더를 조회하거나 변경 후보를 만들고, 날짜/시간/누락 정보를 함께 정리합니다."
	case "maps":
		return "지도 검색/길찾기 링크와 장소 후보를 만들어 iPhone에서 바로 열 수 있게 보냅니다."
	case "contacts":
		return "연락처 확인, 연락 문구, 전화 스크립트처럼 바로 복사해 쓸 내용을 작성합니다."
	case "booking":
		return "예약 후보와 조건을 정리하고, 예약/결제 확정 전에는 사용자가 승인할 수 있는 단계에서 멈춥니다."
	case "shopping":
		return "가격, 리뷰, 배송, 쿠폰, 구독/영수증 단서를 비교하고 구매 직전 확인 포인트를 보여줍니다."
	case "media":
		return "라디오, YouTube, OTT, 팟캐스트 후보를 고르고 열 수 있는 링크나 재생 준비 결과를 보냅니다."
	case "mac":
		return "Mac mini에서 앱 열기, URL 열기, 화면 캡처/녹화, 단축어 실행 결과를 Signal로 확인시킵니다."
	case "ops":
		return "서버, 보안, 로그, 배포 상태를 사람이 읽는 운영 보고와 음성 보고 형태로 정리합니다."
	case "daily":
		return "날씨, 뉴스, 일정, 할 일을 하루 흐름에 맞춰 짧은 브리핑으로 정리합니다."
	case "approval":
		return "실행 전 승인 카드와 변경 범위를 만들어 사용자가 한 번 더 확인하게 합니다."
	default:
		return "사용자 요청을 실행 가능한 작업으로 바꾸고 Signal에서 바로 확인할 결과를 만듭니다."
	}
}

func assistantShowcaseScenarioSignal(scenario assistantcatalog.Scenario) string {
	switch scenario.Category {
	case "files":
		return "요약 메시지와 함께 Markdown/DOCX/PPTX/XLSX 같은 실제 파일 첨부. PPTX는 iPhone Signal에서 탭해 PowerPoint/Keynote/Files로 열 수 있음"
	case "voice":
		return "짧은 설명 메시지와 MP3/보이스노트 첨부"
	case "mail":
		return "메일 목록, 요약, 답장 초안이 번호 없이 읽히는 메시지"
	case "research", "daily", "ops":
		return "결론 먼저, 세부 항목, 다음 행동 순서의 보고 메시지"
	case "maps", "booking", "shopping", "media":
		return "후보/링크/주의점이 포함된 모바일용 메시지"
	default:
		return "사용자가 다음 행동을 바로 고를 수 있는 짧은 Signal 답장"
	}
}

func assistantShowcaseScenarioFollowUp(scenario assistantcatalog.Scenario) string {
	switch scenario.Category {
	case "mail":
		return "첫 번째 메일에 답장 초안 써줘"
	case "research":
		return "이걸 한 페이지 시장조사 보고서로 만들어줘"
	case "files":
		return "이 문서를 더 짧게 줄이고 PDF로도 보내줘"
	case "voice":
		return "같은 내용을 더 차분한 목소리로 다시 보내줘"
	case "time":
		return "이 일정 기준으로 오늘 할 일 5개만 정리해줘"
	case "maps":
		return "이 후보 중 주차 되는 곳만 골라줘"
	case "ops":
		return "이 상태를 1분 음성 보고로 보내줘"
	default:
		return "이걸 보고방에 정리해서 보내줘"
	}
}

func assistantShowcaseCategoryRank(category string) int {
	order := []string{"identity", "daily", "mail", "research", "files", "voice", "time", "maps", "contacts", "booking", "shopping", "media", "mac", "ops", "approval"}
	for i, existing := range order {
		if category == existing {
			return i
		}
	}
	return 1000
}

func assistantShowcaseCategoryLabel(category string) string {
	key := "assistant.showcase.category." + strings.TrimSpace(category)
	if label := lang.T(key); label != key {
		return label
	}
	return category
}

func containsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}
