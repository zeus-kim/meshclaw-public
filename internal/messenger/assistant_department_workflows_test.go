package messenger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/meshclaw/meshclaw/internal/lang"
)

func TestAssistantDepartmentWorkflowAdvertisingCreatesArtifacts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_BASE_URL", "https://argos.example.test/argos")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_SERVER_DISABLED", "1")

	reply, handled := assistantDepartmentWorkflowReply(ListenOptions{TargetID: "argos-assistant-workflow", Mode: "assistant"}, "광고기획팀 회의용으로 캠페인 콘셉트 5개와 A/B 테스트표를 만들어줘")
	if !handled {
		t.Fatal("advertising workflow request was not handled")
	}
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{"광고기획팀 캠페인 실행 패키지", "오늘 바로 쓸 결과:", "첫 판단:", "명시 승인 전에는 하지 않았습니다", "캠페인 콘셉트", "A/B 테스트", "XLSX/CSV", "이어서 바로 시킬 수 있는 명령", "모바일에서 바로 열기:", "DOCX 보고서: https://argos.example.test/argos/", "PPTX 발표자료: https://argos.example.test/argos/"} {
		if !strings.Contains(visible, want) {
			t.Fatalf("visible reply missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"meshclaw-attachment:", "/Documents/Argos Vault/", "/.meshclaw/evidence/"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("visible reply should not expose raw path %q:\n%s", unwanted, visible)
		}
	}
	attachments := signalReplyAttachments(reply)
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("missing %s attachment in %#v", ext, attachments)
		}
	}
	markdown := firstAttachmentExt(attachments, ".md")
	data, err := os.ReadFile(markdown)
	if err != nil {
		t.Fatalf("read markdown attachment: %v", err)
	}
	for _, want := range []string{"채널 믹스", "캠페인 콘셉트 5개", "A/B 테스트"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("markdown missing %q:\n%s", want, string(data))
		}
	}
}

func TestAssistantDepartmentWorkflowCanSendMarketingPackageToBriefingWithVoice(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", home+"/targets.json")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_BASE_URL", "https://argos.example.test/argos")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_SERVER_DISABLED", "1")
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-workflow-send", Mode: "assistant"}, "광고기획팀 회의용으로 캠페인 콘셉트 5개와 A/B 테스트표를 만들어서 보고방에 보내줘. 음성으로도 준비해줘.")
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"부서 업무 패키지를 Signal로 보낼 준비를 했습니다.",
		"대상: 보고방",
		"보고방은 one-way/no-reply",
		"광고기획팀 캠페인 실행 패키지",
		"오늘 바로 쓸 결과:",
		"바로 이어서 시킬 일:",
		"캠페인 콘셉트",
		"edge-tts MP3 부서 업무 브리핑",
		"모바일에서 바로 열기:",
		"DOCX 보고서: https://argos.example.test/argos/",
		"MP3 음성 브리핑: https://argos.example.test/argos/",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("department workflow send preview missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"meshclaw-attachment:", "/Documents/Argos Vault/", "/.meshclaw/evidence/"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("visible reply should not expose raw path %q:\n%s", unwanted, visible)
		}
	}
	attachments := signalReplyAttachments(reply)
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".mp3"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("department workflow send preview missing %s attachment: %#v", ext, attachments)
		}
	}
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), ".html") {
			t.Fatalf("HTML should not be attached for mobile Signal package: %#v", attachments)
		}
	}
}

func TestAssistantDepartmentWorkflowRoutesConcreteHRAndPublicAgency(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	hrReply := assistantReply(ListenOptions{TargetID: "argos-assistant-hr", Mode: "assistant"}, "인사팀 채용공고, 후보자 스크리닝 표, 면접 질문지를 한 번에 만들어줘")
	hrVisible := signalReplyVisibleText(hrReply)
	hrAttachments := signalReplyAttachments(hrReply)
	if !strings.Contains(hrVisible, "인사팀 채용 실행 패키지") ||
		!strings.Contains(hrVisible, "후보자 스크리닝 표") ||
		!strings.Contains(hrVisible, "Mermaid 그래프 Markdown") ||
		!strings.Contains(hrVisible, "SVG 차트 스냅샷") ||
		!hasAttachmentExt(hrAttachments, ".xlsx") ||
		!hasAttachmentExt(hrAttachments, ".svg") {
		t.Fatalf("HR workflow did not produce concrete package:\n%s\nattachments=%#v", hrVisible, hrAttachments)
	}
	hrGraphMarkdown := ""
	hrGraphSVG := ""
	for _, attachment := range hrAttachments {
		if strings.HasSuffix(strings.ToLower(attachment), "-graphs.md") {
			hrGraphMarkdown = attachment
		}
		if strings.HasSuffix(strings.ToLower(attachment), "-chart.svg") {
			hrGraphSVG = attachment
		}
	}
	if hrGraphMarkdown == "" || hrGraphSVG == "" {
		t.Fatalf("HR workflow should attach graph markdown and SVG chart: %#v", hrAttachments)
	}
	hrGraphData, err := os.ReadFile(hrGraphMarkdown)
	if err != nil {
		t.Fatalf("read HR graph markdown: %v", err)
	}
	for _, want := range []string{"```mermaid", "채용 파이프라인 흐름", "지원 312명", "면접 우선순위"} {
		if !strings.Contains(string(hrGraphData), want) {
			t.Fatalf("HR graph markdown missing %q:\n%s", want, string(hrGraphData))
		}
	}
	hrSVGData, err := os.ReadFile(hrGraphSVG)
	if err != nil {
		t.Fatalf("read HR SVG chart: %v", err)
	}
	for _, want := range []string{"<svg", "인사팀 채용 실행 패키지", "지원", "312명", "입사"} {
		if !strings.Contains(string(hrSVGData), want) {
			t.Fatalf("HR SVG chart missing %q:\n%s", want, string(hrSVGData))
		}
	}

	govReply := assistantReply(ListenOptions{TargetID: "argos-assistant-gov", Mode: "assistant"}, "구청 민원 데이터를 요약해서 부서별 처리 현황과 시민 안내문 초안을 만들어줘")
	govVisible := signalReplyVisibleText(govReply)
	govAttachments := signalReplyAttachments(govReply)
	if !strings.Contains(govVisible, "공공기관 민원·정책 업무 패키지") ||
		!strings.Contains(govVisible, "시민 안내문") ||
		!strings.Contains(govVisible, "Mermaid 그래프 Markdown") ||
		!strings.Contains(govVisible, "SVG 차트 스냅샷") ||
		!hasAttachmentExt(govAttachments, ".xlsx") ||
		!hasAttachmentExt(govAttachments, ".svg") {
		t.Fatalf("public agency workflow did not produce concrete package:\n%s\nattachments=%#v", govVisible, govAttachments)
	}
	govGraphMarkdown := ""
	govGraphSVG := ""
	for _, attachment := range govAttachments {
		if strings.HasSuffix(strings.ToLower(attachment), "-graphs.md") {
			govGraphMarkdown = attachment
		}
		if strings.HasSuffix(strings.ToLower(attachment), "-chart.svg") {
			govGraphSVG = attachment
		}
	}
	if govGraphMarkdown == "" || govGraphSVG == "" {
		t.Fatalf("public agency workflow should attach graph markdown and SVG chart: %#v", govAttachments)
	}
	govGraphData, err := os.ReadFile(govGraphMarkdown)
	if err != nil {
		t.Fatalf("read public agency graph markdown: %v", err)
	}
	for _, want := range []string{"```mermaid", "민원 처리 흐름", "접수 1,039건", "미해결 민원 162건"} {
		if !strings.Contains(string(govGraphData), want) {
			t.Fatalf("public agency graph markdown missing %q:\n%s", want, string(govGraphData))
		}
	}
	govSVGData, err := os.ReadFile(govGraphSVG)
	if err != nil {
		t.Fatalf("read public agency SVG chart: %v", err)
	}
	for _, want := range []string{"<svg", "공공기관 민원·정책 업무 패키지", "교통", "428건", "생활"} {
		if !strings.Contains(string(govSVGData), want) {
			t.Fatalf("public agency SVG chart missing %q:\n%s", want, string(govSVGData))
		}
	}
}

func legacyTestAssistantDepartmentWorkflowChemicalMarginCreatesConcretePackage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-chemical", Mode: "assistant"}, "화학회사 원자재 가격 변동이 마진에 미치는 영향을 규제 리스크까지 분석해서 보고서와 PPT로 만들어줘")
	visible := signalReplyVisibleText(reply)
	attachments := signalReplyAttachments(reply)
	for _, want := range []string{
		"화학회사 원자재·규제 대응 패키지",
		"나프타 가격",
		"PFAS/REACH",
		"가격 전가",
		"Mermaid 그래프 Markdown",
		"SVG 차트 스냅샷",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("chemical workflow visible reply missing %q:\n%s", want, visible)
		}
	}
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("chemical workflow missing %s attachment: %#v", ext, attachments)
		}
	}
	graphMarkdown := ""
	graphSVG := ""
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), "-graphs.md") {
			graphMarkdown = attachment
		}
		if strings.HasSuffix(strings.ToLower(attachment), "-chart.svg") {
			graphSVG = attachment
		}
	}
	if graphMarkdown == "" || graphSVG == "" {
		t.Fatalf("chemical workflow should attach graph markdown and SVG chart: %#v", attachments)
	}
	graphData, err := os.ReadFile(graphMarkdown)
	if err != nil {
		t.Fatalf("read chemical graph markdown: %v", err)
	}
	for _, want := range []string{"```mermaid", "마진 워룸 흐름", "나프타·벤젠·프로필렌 가격", "가격 전가 / 감산 / 제품 믹스 조정"} {
		if !strings.Contains(string(graphData), want) {
			t.Fatalf("chemical graph markdown missing %q:\n%s", want, string(graphData))
		}
	}
	svgData, err := os.ReadFile(graphSVG)
	if err != nil {
		t.Fatalf("read chemical SVG chart: %v", err)
	}
	for _, want := range []string{"<svg", "화학회사 원자재·규제 대응 패키지", "나프타", "87점", "PFAS"} {
		if !strings.Contains(string(svgData), want) {
			t.Fatalf("chemical SVG chart missing %q:\n%s", want, string(svgData))
		}
	}
}

func TestAssistantDepartmentWorkflowManufacturingQualityPackage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-manufacturing-quality", Mode: "assistant"}, "제조 품질팀에서 불량률과 고객 클레임을 8D 보고서, CAPA 액션표, 파레토 그래프로 PPT 만들어줘")
	visible := signalReplyVisibleText(reply)
	attachments := signalReplyAttachments(reply)
	for _, want := range []string{
		"제조 품질·클레임 대응 패키지",
		"8D",
		"CAPA",
		"고객 공지",
		"Mermaid 그래프 Markdown",
		"SVG 차트 스냅샷",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("manufacturing quality visible reply missing %q:\n%s", want, visible)
		}
	}
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("manufacturing quality package missing %s attachment: %#v", ext, attachments)
		}
	}
	graphMarkdown := ""
	graphSVG := ""
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), "-graphs.md") {
			graphMarkdown = attachment
		}
		if strings.HasSuffix(strings.ToLower(attachment), "-chart.svg") {
			graphSVG = attachment
		}
	}
	if graphMarkdown == "" || graphSVG == "" {
		t.Fatalf("manufacturing quality should attach graph markdown and SVG chart: %#v", attachments)
	}
	graphData, err := os.ReadFile(graphMarkdown)
	if err != nil {
		t.Fatalf("read manufacturing quality graph markdown: %v", err)
	}
	for _, want := range []string{"```mermaid", "품질 클레임 대응 흐름", "CAPA 시정조치", "출하 보류·고객 발송 전 승인"} {
		if !strings.Contains(string(graphData), want) {
			t.Fatalf("manufacturing quality graph markdown missing %q:\n%s", want, string(graphData))
		}
	}
	svgData, err := os.ReadFile(graphSVG)
	if err != nil {
		t.Fatalf("read manufacturing quality SVG: %v", err)
	}
	for _, want := range []string{"<svg", "제조 품질·클레임 대응 패키지", "불량", "91건", "고객"} {
		if !strings.Contains(string(svgData), want) {
			t.Fatalf("manufacturing quality SVG missing %q:\n%s", want, string(svgData))
		}
	}
}

func TestAssistantDepartmentWorkflowManufacturingQualityUsesLanguagePack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply, handled := assistantDepartmentWorkflowReply(ListenOptions{TargetID: "argos-assistant-manufacturing-quality-en", Mode: "assistant"}, "create a manufacturing quality defect and customer claim package with 8D, CAPA, Pareto chart, report, and PPT")
	if !handled {
		t.Fatal("manufacturing quality workflow should handle English quality package requests")
	}
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Manufacturing Quality and Customer Claim Response Package",
		"Defect view",
		"CAPA table",
		"No shipment hold",
		"Useful follow-up commands",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("manufacturing quality English reply missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"제조 품질", "고객 공지", "출하 보류"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("manufacturing quality English reply should not expose Korean text %q:\n%s", unwanted, visible)
		}
	}
}

func TestAssistantDepartmentWorkflowHealthcareClinicOpsPackage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-healthcare-clinic", Mode: "assistant"}, "병원 원무팀에서 진료 예약표, 대기시간, 검사 안내, 보험청구 체크리스트, 환자 공지를 PPT와 그래프로 만들어줘")
	visible := signalReplyVisibleText(reply)
	attachments := signalReplyAttachments(reply)
	for _, want := range []string{
		"병원·클리닉 원무 운영 패키지",
		"진료 예약표",
		"보험청구",
		"의료 판단",
		"Mermaid 그래프 Markdown",
		"SVG 차트 스냅샷",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("healthcare clinic visible reply missing %q:\n%s", want, visible)
		}
	}
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("healthcare clinic package missing %s attachment: %#v", ext, attachments)
		}
	}
	graphMarkdown := ""
	graphSVG := ""
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), "-graphs.md") {
			graphMarkdown = attachment
		}
		if strings.HasSuffix(strings.ToLower(attachment), "-chart.svg") {
			graphSVG = attachment
		}
	}
	if graphMarkdown == "" || graphSVG == "" {
		t.Fatalf("healthcare clinic should attach graph markdown and SVG chart: %#v", attachments)
	}
	graphData, err := os.ReadFile(graphMarkdown)
	if err != nil {
		t.Fatalf("read healthcare clinic graph markdown: %v", err)
	}
	for _, want := range []string{"```mermaid", "원무 운영 흐름", "보험청구 서류 체크", "청구 제출·환자 발송 전 승인"} {
		if !strings.Contains(string(graphData), want) {
			t.Fatalf("healthcare clinic graph markdown missing %q:\n%s", want, string(graphData))
		}
	}
	svgData, err := os.ReadFile(graphSVG)
	if err != nil {
		t.Fatalf("read healthcare clinic SVG: %v", err)
	}
	for _, want := range []string{"<svg", "병원·클리닉 원무 운영 패키지", "예약", "142건", "공지"} {
		if !strings.Contains(string(svgData), want) {
			t.Fatalf("healthcare clinic SVG missing %q:\n%s", want, string(svgData))
		}
	}
}

func TestAssistantDepartmentWorkflowHealthcareClinicUsesLanguagePack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply, handled := assistantDepartmentWorkflowReply(ListenOptions{TargetID: "argos-assistant-healthcare-clinic-en", Mode: "assistant"}, "create a clinic operations package with appointment schedule, wait time, insurance claim checklist, patient notice, graph, and PPT")
	if !handled {
		t.Fatal("healthcare clinic workflow should handle English clinic operations requests")
	}
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Hospital and Clinic Front-Office Operations Package",
		"Appointment operations",
		"Insurance claims",
		"No diagnosis",
		"Useful follow-up commands",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("healthcare clinic English reply missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"병원", "보험청구", "의료 판단"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("healthcare clinic English reply should not expose Korean text %q:\n%s", unwanted, visible)
		}
	}
}

func TestAssistantDepartmentWorkflowLogisticsDeliveryOpsPackage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-logistics-delivery", Mode: "assistant"}, "물류 운영팀에서 배송 지연, 피킹 오류, 재고 부족, 배송 클레임, SLA 회복 플랜을 PPT와 그래프로 만들어줘")
	visible := signalReplyVisibleText(reply)
	attachments := signalReplyAttachments(reply)
	for _, want := range []string{
		"물류·배송 운영 대응 패키지",
		"배송 지연",
		"피킹 오류",
		"SLA",
		"Mermaid 그래프 Markdown",
		"SVG 차트 스냅샷",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("logistics delivery visible reply missing %q:\n%s", want, visible)
		}
	}
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("logistics delivery package missing %s attachment: %#v", ext, attachments)
		}
	}
	graphMarkdown := ""
	graphSVG := ""
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), "-graphs.md") {
			graphMarkdown = attachment
		}
		if strings.HasSuffix(strings.ToLower(attachment), "-chart.svg") {
			graphSVG = attachment
		}
	}
	if graphMarkdown == "" || graphSVG == "" {
		t.Fatalf("logistics delivery should attach graph markdown and SVG chart: %#v", attachments)
	}
	graphData, err := os.ReadFile(graphMarkdown)
	if err != nil {
		t.Fatalf("read logistics delivery graph markdown: %v", err)
	}
	for _, want := range []string{"```mermaid", "물류 운영 대응 흐름", "고객 발송·환불·재배송 전 승인", "SLA 회복 플랜"} {
		if !strings.Contains(string(graphData), want) {
			t.Fatalf("logistics delivery graph markdown missing %q:\n%s", want, string(graphData))
		}
	}
	svgData, err := os.ReadFile(graphSVG)
	if err != nil {
		t.Fatalf("read logistics delivery SVG: %v", err)
	}
	for _, want := range []string{"<svg", "물류·배송 운영 대응 패키지", "지연", "128건", "SLA"} {
		if !strings.Contains(string(svgData), want) {
			t.Fatalf("logistics delivery SVG missing %q:\n%s", want, string(svgData))
		}
	}
}

func TestAssistantDepartmentWorkflowLogisticsDeliveryUsesLanguagePack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply, handled := assistantDepartmentWorkflowReply(ListenOptions{TargetID: "argos-assistant-logistics-delivery-en", Mode: "assistant"}, "create a logistics ops package with delivery delay, picking error, stockout, shipment SLA, dashboard, and PPT")
	if !handled {
		t.Fatal("logistics delivery workflow should handle English logistics package requests")
	}
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Logistics and Delivery Operations Response Package",
		"Delivery delays",
		"Warehouse errors",
		"No shipment hold",
		"Useful follow-up commands",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("logistics delivery English reply missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"물류", "배송 지연", "출고 보류"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("logistics delivery English reply should not expose Korean text %q:\n%s", unwanted, visible)
		}
	}
}

func legacyTestAssistantDepartmentWorkflowRetailFranchiseOpsPackage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-retail-franchise", Mode: "assistant"}, "프랜차이즈 유통 본부에서 매장별 매출, 품절 SKU, 리뷰 이슈, 로컬 광고, 다음 주 발주와 점주 공지를 PPT와 그래프로 만들어줘")
	visible := signalReplyVisibleText(reply)
	attachments := signalReplyAttachments(reply)
	for _, want := range []string{
		"리테일·프랜차이즈 본부 운영 패키지",
		"매장별 매출",
		"품절 SKU",
		"로컬 광고",
		"점주 공지",
		"Mermaid 그래프 Markdown",
		"SVG 차트 스냅샷",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("retail franchise visible reply missing %q:\n%s", want, visible)
		}
	}
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("retail franchise package missing %s attachment: %#v", ext, attachments)
		}
	}
	graphMarkdown := ""
	graphSVG := ""
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), "-graphs.md") {
			graphMarkdown = attachment
		}
		if strings.HasSuffix(strings.ToLower(attachment), "-chart.svg") {
			graphSVG = attachment
		}
	}
	if graphMarkdown == "" || graphSVG == "" {
		t.Fatalf("retail franchise should attach graph markdown and SVG chart: %#v", attachments)
	}
	graphData, err := os.ReadFile(graphMarkdown)
	if err != nil {
		t.Fatalf("read retail franchise graph markdown: %v", err)
	}
	for _, want := range []string{"```mermaid", "프랜차이즈 본부 운영 흐름", "발주 확정·가격 변경 전 승인", "매장 조치 우선순위"} {
		if !strings.Contains(string(graphData), want) {
			t.Fatalf("retail franchise graph markdown missing %q:\n%s", want, string(graphData))
		}
	}
	svgData, err := os.ReadFile(graphSVG)
	if err != nil {
		t.Fatalf("read retail franchise SVG: %v", err)
	}
	for _, want := range []string{"<svg", "리테일·프랜차이즈 본부 운영 패키지", "매출", "118점", "발주"} {
		if !strings.Contains(string(svgData), want) {
			t.Fatalf("retail franchise SVG missing %q:\n%s", want, string(svgData))
		}
	}
}

func TestAssistantDepartmentWorkflowRetailFranchiseUsesLanguagePack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply, handled := assistantDepartmentWorkflowReply(ListenOptions{TargetID: "argos-assistant-retail-franchise-en", Mode: "assistant"}, "create a retail franchise ops package with store sales, stockout SKUs, review issues, local ads, replenishment, franchisee notice, graph, and PPT")
	if !handled {
		t.Fatal("retail franchise workflow should handle English retail operations requests")
	}
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Retail and Franchise HQ Operations Package",
		"store-level sales",
		"Inventory",
		"No price change",
		"Useful follow-up commands",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("retail franchise English reply missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"리테일", "매장별 매출", "점주 공지"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("retail franchise English reply should not expose Korean text %q:\n%s", unwanted, visible)
		}
	}
}

func TestAssistantDepartmentWorkflowCustomerSuccessRetentionPackage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-customer-success", Mode: "assistant"}, "SaaS 고객성공팀에서 해지 위험 고객, 사용량 감소, 지원 티켓, 재계약 일정, 업셀 후보를 PPT와 그래프로 만들고 음성 브리핑도 준비해줘")
	visible := signalReplyVisibleText(reply)
	attachments := signalReplyAttachments(reply)
	for _, want := range []string{
		"고객성공 이탈 방지·재계약 패키지",
		"해지 위험 고객",
		"사용량 감소",
		"재계약",
		"업셀",
		"Mermaid 그래프 Markdown",
		"SVG 차트 스냅샷",
		"edge-tts MP3",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("customer success visible reply missing %q:\n%s", want, visible)
		}
	}
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg", ".mp3"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("customer success package missing %s attachment: %#v", ext, attachments)
		}
	}
	graphMarkdown := ""
	graphSVG := ""
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), "-graphs.md") {
			graphMarkdown = attachment
		}
		if strings.HasSuffix(strings.ToLower(attachment), "-chart.svg") {
			graphSVG = attachment
		}
	}
	if graphMarkdown == "" || graphSVG == "" {
		t.Fatalf("customer success should attach graph markdown and SVG chart: %#v", attachments)
	}
	graphData, err := os.ReadFile(graphMarkdown)
	if err != nil {
		t.Fatalf("read customer success graph markdown: %v", err)
	}
	for _, want := range []string{"```mermaid", "고객성공 이탈 방지 흐름", "고객 발송·계약 변경 전 승인", "계정 조치 우선순위"} {
		if !strings.Contains(string(graphData), want) {
			t.Fatalf("customer success graph markdown missing %q:\n%s", want, string(graphData))
		}
	}
	svgData, err := os.ReadFile(graphSVG)
	if err != nil {
		t.Fatalf("read customer success SVG: %v", err)
	}
	for _, want := range []string{"<svg", "고객성공 이탈 방지·재계약 패키지", "위험", "38개", "재계약"} {
		if !strings.Contains(string(svgData), want) {
			t.Fatalf("customer success SVG missing %q:\n%s", want, string(svgData))
		}
	}
}

func TestAssistantDepartmentWorkflowCustomerSuccessUsesLanguagePack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply, handled := assistantDepartmentWorkflowReply(ListenOptions{TargetID: "argos-assistant-customer-success-en", Mode: "assistant"}, "create a customer success churn and renewal package with at-risk accounts, usage drop, support tickets, renewal plan, expansion pipeline, graph, and PPT")
	if !handled {
		t.Fatal("customer success workflow should handle English retention requests")
	}
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Customer Success Churn Prevention and Renewal Package",
		"Churn risk",
		"Renewals",
		"No customer send",
		"Useful follow-up commands",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("customer success English reply missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"고객성공", "해지 위험", "재계약"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("customer success English reply should not expose Korean text %q:\n%s", unwanted, visible)
		}
	}
}

func TestAssistantDepartmentWorkflowFinanceARCollectionPackage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-finance-ar", Mode: "assistant"}, "재무팀에서 미수금, 매출채권 aging, 연체 인보이스, 현금흐름 예측, 수금 우선순위를 PPT와 그래프와 음성 브리핑으로 만들어줘")
	visible := signalReplyVisibleText(reply)
	attachments := signalReplyAttachments(reply)
	for _, want := range []string{
		"미수금·현금흐름 수금 패키지",
		"연체 인보이스",
		"매출채권 aging",
		"현금흐름 예측",
		"수금 우선순위",
		"Mermaid 그래프 Markdown",
		"SVG 차트 스냅샷",
		"edge-tts MP3",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("finance AR visible reply missing %q:\n%s", want, visible)
		}
	}
	if strings.Contains(visible, "법인카드·영수증 정산 패키지") {
		t.Fatalf("finance AR request should not route to expense reconciliation:\n%s", visible)
	}
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg", ".mp3"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("finance AR package missing %s attachment: %#v", ext, attachments)
		}
	}
	graphMarkdown := ""
	graphSVG := ""
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), "-graphs.md") {
			graphMarkdown = attachment
		}
		if strings.HasSuffix(strings.ToLower(attachment), "-chart.svg") {
			graphSVG = attachment
		}
	}
	if graphMarkdown == "" || graphSVG == "" {
		t.Fatalf("finance AR should attach graph markdown and SVG chart: %#v", attachments)
	}
	graphData, err := os.ReadFile(graphMarkdown)
	if err != nil {
		t.Fatalf("read finance AR graph markdown: %v", err)
	}
	for _, want := range []string{"```mermaid", "미수금 수금 흐름", "고객 발송·신용보류·ERP 수정 전 승인", "수금 조치 우선순위"} {
		if !strings.Contains(string(graphData), want) {
			t.Fatalf("finance AR graph markdown missing %q:\n%s", want, string(graphData))
		}
	}
	svgData, err := os.ReadFile(graphSVG)
	if err != nil {
		t.Fatalf("read finance AR SVG: %v", err)
	}
	for _, want := range []string{"<svg", "미수금·현금흐름 수금 패키지", "연체", "184M", "현금"} {
		if !strings.Contains(string(svgData), want) {
			t.Fatalf("finance AR SVG missing %q:\n%s", want, string(svgData))
		}
	}
}

func TestAssistantDepartmentWorkflowFinanceARUsesLanguagePack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply, handled := assistantDepartmentWorkflowReply(ListenOptions{TargetID: "argos-assistant-finance-ar-en", Mode: "assistant"}, "create an accounts receivable cash collection package with AR aging, overdue invoices, cash forecast, collection priority, customer email drafts, graph, and PPT")
	if !handled {
		t.Fatal("finance AR workflow should handle English collection requests")
	}
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Accounts Receivable and Cash Collection Package",
		"Overdue",
		"Cash forecast",
		"No customer send",
		"Useful follow-up commands",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("finance AR English reply missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"미수금", "현금흐름", "수금"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("finance AR English reply should not expose Korean text %q:\n%s", unwanted, visible)
		}
	}
}

func TestAssistantDepartmentWorkflowProcurementVendorPackage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-procurement", Mode: "assistant"}, "구매팀에서 벤더 비교, 견적 비교, TCO, 납기 리스크, 발주 품의서를 PPT와 그래프와 음성 브리핑으로 만들어줘")
	visible := signalReplyVisibleText(reply)
	attachments := signalReplyAttachments(reply)
	for _, want := range []string{
		"구매팀 벤더 비교·발주 품의 패키지",
		"벤더 견적 비교",
		"총소유비용",
		"납기 리스크",
		"발주 품의서",
		"Mermaid 그래프 Markdown",
		"SVG 차트 스냅샷",
		"edge-tts MP3",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("procurement visible reply missing %q:\n%s", want, visible)
		}
	}
	if strings.Contains(visible, "RFP 제안서 응답") || strings.Contains(visible, "쿠팡 상품 비교") {
		t.Fatalf("procurement request should not route to RFP or shopping packages:\n%s", visible)
	}
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg", ".mp3"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("procurement package missing %s attachment: %#v", ext, attachments)
		}
	}
	graphMarkdown := ""
	graphSVG := ""
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), "-graphs.md") {
			graphMarkdown = attachment
		}
		if strings.HasSuffix(strings.ToLower(attachment), "-chart.svg") {
			graphSVG = attachment
		}
	}
	if graphMarkdown == "" || graphSVG == "" {
		t.Fatalf("procurement should attach graph markdown and SVG chart: %#v", attachments)
	}
	graphData, err := os.ReadFile(graphMarkdown)
	if err != nil {
		t.Fatalf("read procurement graph markdown: %v", err)
	}
	for _, want := range []string{"```mermaid", "구매 품의 흐름", "발주·결제·계약·ERP 입력 전 승인", "벤더 선택 우선순위"} {
		if !strings.Contains(string(graphData), want) {
			t.Fatalf("procurement graph markdown missing %q:\n%s", want, string(graphData))
		}
	}
	svgData, err := os.ReadFile(graphSVG)
	if err != nil {
		t.Fatalf("read procurement SVG: %v", err)
	}
	for _, want := range []string{"<svg", "구매팀 벤더 비교·발주 품의 패키지", "견적", "82점", "승인"} {
		if !strings.Contains(string(svgData), want) {
			t.Fatalf("procurement SVG missing %q:\n%s", want, string(svgData))
		}
	}
}

func TestAssistantDepartmentWorkflowProcurementUsesLanguagePack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply, handled := assistantDepartmentWorkflowReply(ListenOptions{TargetID: "argos-assistant-procurement-en", Mode: "assistant"}, "create a procurement vendor selection package with vendor comparison, quote comparison, TCO, delivery risk, purchase approval memo, graph, and PPT")
	if !handled {
		t.Fatal("procurement workflow should handle English vendor selection requests")
	}
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Procurement Vendor Comparison and Purchase Approval Package",
		"Quote comparison",
		"Delivery risk",
		"No purchase order",
		"Useful follow-up commands",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("procurement English reply missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"구매팀", "발주 품의", "납기 리스크"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("procurement English reply should not expose Korean text %q:\n%s", unwanted, visible)
		}
	}
}

func TestAssistantDepartmentWorkflowCommunicationsCrisisPackage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-comms-crisis", Mode: "assistant"}, "홍보팀에서 위기 대응 보도자료, 입장문, FAQ, 대표 브리핑 Q&A, SNS 확산 리스크를 PPT와 그래프와 음성 브리핑으로 만들어줘")
	visible := signalReplyVisibleText(reply)
	attachments := signalReplyAttachments(reply)
	for _, want := range []string{
		"홍보팀 위기대응 커뮤니케이션 패키지",
		"이슈 타임라인",
		"보도자료·입장문 초안",
		"임원 Q&A",
		"채널별 대응표",
		"Mermaid 그래프 Markdown",
		"SVG 차트 스냅샷",
		"edge-tts MP3",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("communications crisis visible reply missing %q:\n%s", want, visible)
		}
	}
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg", ".mp3"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("communications crisis package missing %s attachment: %#v", ext, attachments)
		}
	}
	graphMarkdown := ""
	graphSVG := ""
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), "-graphs.md") {
			graphMarkdown = attachment
		}
		if strings.HasSuffix(strings.ToLower(attachment), "-chart.svg") {
			graphSVG = attachment
		}
	}
	if graphMarkdown == "" || graphSVG == "" {
		t.Fatalf("communications crisis should attach graph markdown and SVG chart: %#v", attachments)
	}
	graphData, err := os.ReadFile(graphMarkdown)
	if err != nil {
		t.Fatalf("read communications crisis graph markdown: %v", err)
	}
	for _, want := range []string{"```mermaid", "위기 커뮤니케이션 흐름", "외부 발송·SNS 게시 전 승인", "채널 대응 우선순위"} {
		if !strings.Contains(string(graphData), want) {
			t.Fatalf("communications crisis graph markdown missing %q:\n%s", want, string(graphData))
		}
	}
	svgData, err := os.ReadFile(graphSVG)
	if err != nil {
		t.Fatalf("read communications crisis SVG: %v", err)
	}
	for _, want := range []string{"<svg", "홍보팀 위기대응 커뮤니케이션 패키지", "언론", "73점", "승인"} {
		if !strings.Contains(string(svgData), want) {
			t.Fatalf("communications crisis SVG missing %q:\n%s", want, string(svgData))
		}
	}
}

func TestAssistantDepartmentWorkflowCommunicationsCrisisUsesLanguagePack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply, handled := assistantDepartmentWorkflowReply(ListenOptions{TargetID: "argos-assistant-comms-crisis-en", Mode: "assistant"}, "create a PR crisis communications package with press statement, FAQ, executive Q&A, social spread risk, media response, graph, and PPT")
	if !handled {
		t.Fatal("communications crisis workflow should handle English PR requests")
	}
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"PR Crisis Communications Response Package",
		"Timeline",
		"Core message",
		"No external send",
		"Useful follow-up commands",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("communications crisis English reply missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"홍보팀", "보도자료", "위기대응"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("communications crisis English reply should not expose Korean text %q:\n%s", unwanted, visible)
		}
	}
}

func TestAssistantDepartmentWorkflowITOffboardingPackage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-it-offboarding", Mode: "assistant"}, "IT팀에서 퇴사자 계정 회수, 접근권한 점검, SaaS 라이선스 정리, 기기 반납, 데이터 이관 체크리스트를 PPT와 그래프와 음성 브리핑으로 만들어줘")
	visible := signalReplyVisibleText(reply)
	attachments := signalReplyAttachments(reply)
	for _, want := range []string{
		"IT 퇴사자 계정 회수·라이선스 정리 패키지",
		"퇴사자 계정",
		"SaaS 라이선스",
		"기기 반납",
		"데이터 이관",
		"Mermaid 그래프 Markdown",
		"SVG 차트 스냅샷",
		"edge-tts MP3",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("IT offboarding visible reply missing %q:\n%s", want, visible)
		}
	}
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg", ".mp3"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("IT offboarding package missing %s attachment: %#v", ext, attachments)
		}
	}
	graphMarkdown := ""
	graphSVG := ""
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), "-graphs.md") {
			graphMarkdown = attachment
		}
		if strings.HasSuffix(strings.ToLower(attachment), "-chart.svg") {
			graphSVG = attachment
		}
	}
	if graphMarkdown == "" || graphSVG == "" {
		t.Fatalf("IT offboarding should attach graph markdown and SVG chart: %#v", attachments)
	}
	graphData, err := os.ReadFile(graphMarkdown)
	if err != nil {
		t.Fatalf("read IT offboarding graph markdown: %v", err)
	}
	for _, want := range []string{"```mermaid", "IT 오프보딩 흐름", "계정 비활성화·권한 삭제 전 승인", "권한 회수 우선순위"} {
		if !strings.Contains(string(graphData), want) {
			t.Fatalf("IT offboarding graph markdown missing %q:\n%s", want, string(graphData))
		}
	}
	svgData, err := os.ReadFile(graphSVG)
	if err != nil {
		t.Fatalf("read IT offboarding SVG: %v", err)
	}
	for _, want := range []string{"<svg", "IT 퇴사자 계정 회수·라이선스 정리 패키지", "계정", "27개", "승인"} {
		if !strings.Contains(string(svgData), want) {
			t.Fatalf("IT offboarding SVG missing %q:\n%s", want, string(svgData))
		}
	}
}

func TestAssistantDepartmentWorkflowITOffboardingUsesLanguagePack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply, handled := assistantDepartmentWorkflowReply(ListenOptions{TargetID: "argos-assistant-it-offboarding-en", Mode: "assistant"}, "create an IT offboarding package with account access removal, SaaS license cleanup, device return, data handoff checklist, graph, and PPT")
	if !handled {
		t.Fatal("IT offboarding workflow should handle English access removal requests")
	}
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"IT Access Offboarding and SaaS License Cleanup Package",
		"Accounts",
		"Licenses",
		"No account deactivation",
		"Useful follow-up commands",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("IT offboarding English reply missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"퇴사자", "계정 회수", "라이선스 정리"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("IT offboarding English reply should not expose Korean text %q:\n%s", unwanted, visible)
		}
	}
}

func TestAssistantDepartmentWorkflowInsuranceClaimsPackage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-insurance-claims", Mode: "assistant"}, "보험사 보상팀에서 보험금 청구 접수, 누락 서류 보완, 이상 징후, 지급 대기, 고객 안내문을 PPT와 그래프와 음성 브리핑으로 만들어줘")
	visible := signalReplyVisibleText(reply)
	attachments := signalReplyAttachments(reply)
	for _, want := range []string{
		"보험금 청구 심사·서류 보완 패키지",
		"보험금 청구",
		"누락 서류",
		"이상 징후",
		"지급 대기",
		"Mermaid 그래프 Markdown",
		"SVG 차트 스냅샷",
		"edge-tts MP3",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("insurance claims visible reply missing %q:\n%s", want, visible)
		}
	}
	if strings.Contains(visible, "병원·클리닉 원무 운영") || strings.Contains(visible, "고객지원 티켓") {
		t.Fatalf("insurance claims request should not route to healthcare or support packages:\n%s", visible)
	}
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg", ".mp3"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("insurance claims package missing %s attachment: %#v", ext, attachments)
		}
	}
	graphMarkdown := ""
	graphSVG := ""
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), "-graphs.md") {
			graphMarkdown = attachment
		}
		if strings.HasSuffix(strings.ToLower(attachment), "-chart.svg") {
			graphSVG = attachment
		}
	}
	if graphMarkdown == "" || graphSVG == "" {
		t.Fatalf("insurance claims should attach graph markdown and SVG chart: %#v", attachments)
	}
	graphData, err := os.ReadFile(graphMarkdown)
	if err != nil {
		t.Fatalf("read insurance claims graph markdown: %v", err)
	}
	for _, want := range []string{"```mermaid", "보험금 청구 처리 흐름", "지급·거절·고객 발송 전 승인", "청구 심사 우선순위"} {
		if !strings.Contains(string(graphData), want) {
			t.Fatalf("insurance claims graph markdown missing %q:\n%s", want, string(graphData))
		}
	}
	svgData, err := os.ReadFile(graphSVG)
	if err != nil {
		t.Fatalf("read insurance claims SVG: %v", err)
	}
	for _, want := range []string{"<svg", "보험금 청구 심사·서류 보완 패키지", "접수", "128건", "고객"} {
		if !strings.Contains(string(svgData), want) {
			t.Fatalf("insurance claims SVG missing %q:\n%s", want, string(svgData))
		}
	}
}

func TestAssistantDepartmentWorkflowInsuranceClaimsUsesLanguagePack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply, handled := assistantDepartmentWorkflowReply(ListenOptions{TargetID: "argos-assistant-insurance-claims-en", Mode: "assistant"}, "create an insurance claims triage package with claim intake, missing documents, risk signals, payout queue, customer notices, graph, and PPT")
	if !handled {
		t.Fatal("insurance claims workflow should handle English claims operations requests")
	}
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Insurance Claims Triage and Missing-Document Package",
		"Intake",
		"Missing",
		"No payout approval",
		"Useful follow-up commands",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("insurance claims English reply missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"보험금", "누락 서류", "지급 대기"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("insurance claims English reply should not expose Korean text %q:\n%s", unwanted, visible)
		}
	}
}

func TestAssistantDepartmentWorkflowConstructionSitePackage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-construction-site", Mode: "assistant"}, "건설사 현장소장용으로 공정 지연, 안전 점검, 자재 반입, 협력사 이슈, 발주처 보고를 PPT와 그래프와 음성 브리핑으로 만들어줘")
	visible := signalReplyVisibleText(reply)
	attachments := signalReplyAttachments(reply)
	for _, want := range []string{
		"건설 현장 공정·안전·자재 리스크 패키지",
		"공정 지연",
		"안전 위험",
		"자재 반입",
		"협력사",
		"Mermaid 그래프 Markdown",
		"SVG 차트 스냅샷",
		"edge-tts MP3",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("construction site visible reply missing %q:\n%s", want, visible)
		}
	}
	if strings.Contains(visible, "제조 품질") || strings.Contains(visible, "물류·배송") {
		t.Fatalf("construction site request should not route to manufacturing or logistics packages:\n%s", visible)
	}
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg", ".mp3"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("construction site package missing %s attachment: %#v", ext, attachments)
		}
	}
	graphMarkdown := ""
	graphSVG := ""
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), "-graphs.md") {
			graphMarkdown = attachment
		}
		if strings.HasSuffix(strings.ToLower(attachment), "-chart.svg") {
			graphSVG = attachment
		}
	}
	if graphMarkdown == "" || graphSVG == "" {
		t.Fatalf("construction site should attach graph markdown and SVG chart: %#v", attachments)
	}
	graphData, err := os.ReadFile(graphMarkdown)
	if err != nil {
		t.Fatalf("read construction site graph markdown: %v", err)
	}
	for _, want := range []string{"```mermaid", "건설 현장 운영 흐름", "작업중지·공정 변경·외부 발송 전 승인", "현장 리스크 우선순위"} {
		if !strings.Contains(string(graphData), want) {
			t.Fatalf("construction site graph markdown missing %q:\n%s", want, string(graphData))
		}
	}
	svgData, err := os.ReadFile(graphSVG)
	if err != nil {
		t.Fatalf("read construction site SVG: %v", err)
	}
	for _, want := range []string{"<svg", "건설 현장 공정·안전·자재 리스크 패키지", "공정", "74건", "승인"} {
		if !strings.Contains(string(svgData), want) {
			t.Fatalf("construction site SVG missing %q:\n%s", want, string(svgData))
		}
	}
}

func TestAssistantDepartmentWorkflowConstructionSiteUsesLanguagePack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply, handled := assistantDepartmentWorkflowReply(ListenOptions{TargetID: "argos-assistant-construction-site-en", Mode: "assistant"}, "create a construction site package with schedule delay, site safety, material delivery, subcontractor risks, owner report, graph, and PPT")
	if !handled {
		t.Fatal("construction site workflow should handle English site operations requests")
	}
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Construction Site Schedule, Safety, and Materials Risk Package",
		"Schedule",
		"Safety",
		"No stop-work order",
		"Useful follow-up commands",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("construction site English reply missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"건설 현장", "공정 지연", "자재 반입"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("construction site English reply should not expose Korean text %q:\n%s", unwanted, visible)
		}
	}
}

func TestAssistantDepartmentWorkflowLegalDisputePackage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-legal-dispute", Mode: "assistant"}, "법무팀에서 소송 분쟁 대응, 쟁점, 증거 타임라인, 내용증명, 기일, 합의 옵션을 PPT와 그래프와 음성 브리핑으로 만들어줘")
	visible := signalReplyVisibleText(reply)
	attachments := signalReplyAttachments(reply)
	for _, want := range []string{
		"법무팀 소송·분쟁 대응 패키지",
		"분쟁 쟁점",
		"증거 타임라인",
		"내용증명",
		"합의 옵션",
		"Mermaid 그래프 Markdown",
		"SVG 차트 스냅샷",
		"edge-tts MP3",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("legal dispute visible reply missing %q:\n%s", want, visible)
		}
	}
	if strings.Contains(visible, "계약서·벤더 온보딩") {
		t.Fatalf("legal dispute request should not route to contract review package:\n%s", visible)
	}
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg", ".mp3"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("legal dispute package missing %s attachment: %#v", ext, attachments)
		}
	}
	graphMarkdown := ""
	graphSVG := ""
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), "-graphs.md") {
			graphMarkdown = attachment
		}
		if strings.HasSuffix(strings.ToLower(attachment), "-chart.svg") {
			graphSVG = attachment
		}
	}
	if graphMarkdown == "" || graphSVG == "" {
		t.Fatalf("legal dispute should attach graph markdown and SVG chart: %#v", attachments)
	}
	graphData, err := os.ReadFile(graphMarkdown)
	if err != nil {
		t.Fatalf("read legal dispute graph markdown: %v", err)
	}
	for _, want := range []string{"```mermaid", "분쟁 대응 흐름", "내용증명 발송·소송 제출·상대방 연락 전 승인", "분쟁 대응 우선순위"} {
		if !strings.Contains(string(graphData), want) {
			t.Fatalf("legal dispute graph markdown missing %q:\n%s", want, string(graphData))
		}
	}
	svgData, err := os.ReadFile(graphSVG)
	if err != nil {
		t.Fatalf("read legal dispute SVG: %v", err)
	}
	for _, want := range []string{"<svg", "법무팀 소송·분쟁 대응 패키지", "쟁점", "9건", "승인"} {
		if !strings.Contains(string(svgData), want) {
			t.Fatalf("legal dispute SVG missing %q:\n%s", want, string(svgData))
		}
	}
}

func TestAssistantDepartmentWorkflowLegalDisputeUsesLanguagePack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply, handled := assistantDepartmentWorkflowReply(ListenOptions{TargetID: "argos-assistant-legal-dispute-en", Mode: "assistant"}, "create a litigation and legal dispute response package with issues, evidence timeline, demand letter, deadlines, settlement options, graph, and PPT")
	if !handled {
		t.Fatal("legal dispute workflow should handle English litigation requests")
	}
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Legal Litigation and Dispute Response Package",
		"Issues",
		"Evidence",
		"No demand-letter send",
		"Useful follow-up commands",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("legal dispute English reply missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"법무팀", "내용증명", "소송"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("legal dispute English reply should not expose Korean text %q:\n%s", unwanted, visible)
		}
	}
}

func TestAssistantDepartmentWorkflowEducationAcademyOpsPackage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-education-academy", Mode: "assistant"}, "학원 운영팀에서 상담 리드, 체험수업, 출결, 수강료 미납, 재등록, 학부모 공지를 PPT와 그래프로 만들어줘")
	visible := signalReplyVisibleText(reply)
	attachments := signalReplyAttachments(reply)
	for _, want := range []string{
		"학원·교육기관 운영 패키지",
		"상담 리드",
		"체험수업",
		"수강료 미납",
		"Mermaid 그래프 Markdown",
		"SVG 차트 스냅샷",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("education academy visible reply missing %q:\n%s", want, visible)
		}
	}
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("education academy package missing %s attachment: %#v", ext, attachments)
		}
	}
	graphMarkdown := ""
	graphSVG := ""
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), "-graphs.md") {
			graphMarkdown = attachment
		}
		if strings.HasSuffix(strings.ToLower(attachment), "-chart.svg") {
			graphSVG = attachment
		}
	}
	if graphMarkdown == "" || graphSVG == "" {
		t.Fatalf("education academy should attach graph markdown and SVG chart: %#v", attachments)
	}
	graphData, err := os.ReadFile(graphMarkdown)
	if err != nil {
		t.Fatalf("read education academy graph markdown: %v", err)
	}
	for _, want := range []string{"```mermaid", "학원 운영 대응 흐름", "등록 확정·학부모 발송 전 승인", "재등록 캠페인"} {
		if !strings.Contains(string(graphData), want) {
			t.Fatalf("education academy graph markdown missing %q:\n%s", want, string(graphData))
		}
	}
	svgData, err := os.ReadFile(graphSVG)
	if err != nil {
		t.Fatalf("read education academy SVG: %v", err)
	}
	for _, want := range []string{"<svg", "학원·교육기관 운영 패키지", "상담", "184건", "미납"} {
		if !strings.Contains(string(svgData), want) {
			t.Fatalf("education academy SVG missing %q:\n%s", want, string(svgData))
		}
	}
}

func TestAssistantDepartmentWorkflowEducationAcademyUsesLanguagePack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply, handled := assistantDepartmentWorkflowReply(ListenOptions{TargetID: "argos-assistant-education-academy-en", Mode: "assistant"}, "create an academy ops package with enrollment funnel, trial class, attendance, unpaid tuition, parent notice, curriculum schedule, dashboard, and PPT")
	if !handled {
		t.Fatal("education academy workflow should handle English academy operations requests")
	}
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Academy and Education Operations Package",
		"Enrollment funnel",
		"Operations issues",
		"No enrollment confirmation",
		"Useful follow-up commands",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("education academy English reply missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"학원", "체험수업", "수강료"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("education academy English reply should not expose Korean text %q:\n%s", unwanted, visible)
		}
	}
}

func TestAssistantDepartmentWorkflowBookingCandidatePackage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-booking-package", Mode: "assistant"}, "내일 저녁 7시에 강남 파스타 식당 2명 예약 후보를 표와 PPT와 전화문구로 만들어줘")
	visible := signalReplyVisibleText(reply)
	attachments := signalReplyAttachments(reply)
	for _, want := range []string{
		"예약 후보 비교·문의 패키지",
		"후보 3개",
		"전화/메시지 초안",
		"최종 예약 확정",
		"Mermaid 그래프 Markdown",
		"SVG 차트 스냅샷",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("booking workflow visible reply missing %q:\n%s", want, visible)
		}
	}
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("booking workflow missing %s attachment: %#v", ext, attachments)
		}
	}
	graphMarkdown := ""
	graphSVG := ""
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), "-graphs.md") {
			graphMarkdown = attachment
		}
		if strings.HasSuffix(strings.ToLower(attachment), "-chart.svg") {
			graphSVG = attachment
		}
	}
	if graphMarkdown == "" || graphSVG == "" {
		t.Fatalf("booking workflow should attach graph markdown and SVG chart: %#v", attachments)
	}
	graphData, err := os.ReadFile(graphMarkdown)
	if err != nil {
		t.Fatalf("read booking graph markdown: %v", err)
	}
	for _, want := range []string{"```mermaid", "예약 후보 처리 흐름", "최종 예약 확정 전 멈춤", "사용자 최종 승인 후 진행"} {
		if !strings.Contains(string(graphData), want) {
			t.Fatalf("booking graph markdown missing %q:\n%s", want, string(graphData))
		}
	}
	svgData, err := os.ReadFile(graphSVG)
	if err != nil {
		t.Fatalf("read booking SVG chart: %v", err)
	}
	for _, want := range []string{"<svg", "예약 후보 비교·문의 패키지", "위치", "88점", "취소"} {
		if !strings.Contains(string(svgData), want) {
			t.Fatalf("booking SVG chart missing %q:\n%s", want, string(svgData))
		}
	}
}

func TestAssistantDepartmentWorkflowBookingPackageDoesNotStealPlainBooking(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	if _, handled := assistantDepartmentWorkflowReply(ListenOptions{TargetID: "argos-assistant-booking-search", Mode: "assistant"}, "내일 저녁 7시에 강남 파스타 식당 2명 예약 후보 찾아줘"); handled {
		t.Fatal("plain booking candidate search should remain on the booking/browser route, not department artifact workflow")
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-hospital-booking-package", Mode: "assistant"}, "병원 예약 후보 3개를 표와 PPT와 전화 문구로 만들어줘")
	visible := signalReplyVisibleText(reply)
	if !strings.Contains(visible, "예약 후보 비교·문의 패키지") || strings.Contains(visible, "병원·클리닉 원무 운영 패키지") {
		t.Fatalf("hospital booking candidate request should stay on booking package route:\n%s", visible)
	}
}

func TestAssistantDepartmentWorkflowFinanceExpenseReconciliationPackage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-finance-expense", Mode: "assistant"}, "법인카드 영수증 정산표와 구매품의 결재 요청 초안을 보고서와 PPT로 만들어줘")
	visible := signalReplyVisibleText(reply)
	attachments := signalReplyAttachments(reply)
	for _, want := range []string{
		"법인카드·영수증 정산 패키지",
		"누락증빙",
		"결재 요청 초안",
		"실제 결제",
		"Mermaid 그래프 Markdown",
		"SVG 차트 스냅샷",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("finance expense workflow visible reply missing %q:\n%s", want, visible)
		}
	}
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("finance expense workflow missing %s attachment: %#v", ext, attachments)
		}
	}
	graphMarkdown := ""
	graphSVG := ""
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), "-graphs.md") {
			graphMarkdown = attachment
		}
		if strings.HasSuffix(strings.ToLower(attachment), "-chart.svg") {
			graphSVG = attachment
		}
	}
	if graphMarkdown == "" || graphSVG == "" {
		t.Fatalf("finance expense workflow should attach graph markdown and SVG chart: %#v", attachments)
	}
	graphData, err := os.ReadFile(graphMarkdown)
	if err != nil {
		t.Fatalf("read finance expense graph markdown: %v", err)
	}
	for _, want := range []string{"```mermaid", "법인카드 정산 흐름", "누락증빙 요청", "실제 결제·송금·승인 전 멈춤"} {
		if !strings.Contains(string(graphData), want) {
			t.Fatalf("finance expense graph markdown missing %q:\n%s", want, string(graphData))
		}
	}
	svgData, err := os.ReadFile(graphSVG)
	if err != nil {
		t.Fatalf("read finance expense SVG chart: %v", err)
	}
	for _, want := range []string{"<svg", "법인카드·영수증 정산 패키지", "교통", "148만원", "누락"} {
		if !strings.Contains(string(svgData), want) {
			t.Fatalf("finance expense SVG chart missing %q:\n%s", want, string(svgData))
		}
	}
}

func TestAssistantDepartmentWorkflowSalesRFPResponsePackage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-sales-rfp", Mode: "assistant"}, "고객 RFP 제안요청서를 요구사항 매트릭스와 영업 제안서 PPT, 후속 메일 초안으로 만들어줘")
	visible := signalReplyVisibleText(reply)
	attachments := signalReplyAttachments(reply)
	for _, want := range []string{
		"RFP·영업 제안서 응답 패키지",
		"요구사항 분해",
		"가격 선택지",
		"후속 메일 초안",
		"견적 발송",
		"Mermaid 그래프 Markdown",
		"SVG 차트 스냅샷",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("sales RFP workflow visible reply missing %q:\n%s", want, visible)
		}
	}
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("sales RFP workflow missing %s attachment: %#v", ext, attachments)
		}
	}
	graphMarkdown := ""
	graphSVG := ""
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), "-graphs.md") {
			graphMarkdown = attachment
		}
		if strings.HasSuffix(strings.ToLower(attachment), "-chart.svg") {
			graphSVG = attachment
		}
	}
	if graphMarkdown == "" || graphSVG == "" {
		t.Fatalf("sales RFP workflow should attach graph markdown and SVG chart: %#v", attachments)
	}
	graphData, err := os.ReadFile(graphMarkdown)
	if err != nil {
		t.Fatalf("read sales RFP graph markdown: %v", err)
	}
	for _, want := range []string{"```mermaid", "RFP 응답 흐름", "견적 발송 전 승인", "계약/서명"} {
		if !strings.Contains(string(graphData), want) {
			t.Fatalf("sales RFP graph markdown missing %q:\n%s", want, string(graphData))
		}
	}
	svgData, err := os.ReadFile(graphSVG)
	if err != nil {
		t.Fatalf("read sales RFP SVG chart: %v", err)
	}
	for _, want := range []string{"<svg", "RFP·영업 제안서 응답 패키지", "기능", "92점", "가격"} {
		if !strings.Contains(string(svgData), want) {
			t.Fatalf("sales RFP SVG chart missing %q:\n%s", want, string(svgData))
		}
	}
}

func TestAssistantDepartmentWorkflowSalesRFPUsesLanguagePack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply, handled := assistantDepartmentWorkflowReply(ListenOptions{TargetID: "argos-assistant-sales-rfp-en", Mode: "assistant"}, "create an RFP proposal response package with requirements matrix, pricing options, and follow-up email")
	if !handled {
		t.Fatal("sales RFP workflow should handle English RFP package requests")
	}
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"RFP and Sales Proposal Response Package",
		"Requirement breakdown",
		"Pricing options",
		"follow-up email",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("sales RFP English language pack reply missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"RFP·영업 제안서 응답 패키지", "요구사항 분해"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("sales RFP English reply should not expose Korean text %q:\n%s", unwanted, visible)
		}
	}
}

func TestAssistantDepartmentWorkflowSupportTicketResponsePackage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-support-ticket", Mode: "assistant"}, "고객지원 CS 티켓을 긴급 장애, 환불 문의, 기능 문의로 분류해서 답변 초안과 에스컬레이션 표, PPT로 만들어줘")
	visible := signalReplyVisibleText(reply)
	attachments := signalReplyAttachments(reply)
	for _, want := range []string{
		"고객지원 티켓·장애 대응 패키지",
		"티켓 분류",
		"답변 초안",
		"에스컬레이션",
		"환불 승인",
		"Mermaid 그래프 Markdown",
		"SVG 차트 스냅샷",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("support ticket workflow visible reply missing %q:\n%s", want, visible)
		}
	}
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("support ticket workflow missing %s attachment: %#v", ext, attachments)
		}
	}
	graphMarkdown := ""
	graphSVG := ""
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), "-graphs.md") {
			graphMarkdown = attachment
		}
		if strings.HasSuffix(strings.ToLower(attachment), "-chart.svg") {
			graphSVG = attachment
		}
	}
	if graphMarkdown == "" || graphSVG == "" {
		t.Fatalf("support ticket workflow should attach graph markdown and SVG chart: %#v", attachments)
	}
	graphData, err := os.ReadFile(graphMarkdown)
	if err != nil {
		t.Fatalf("read support ticket graph markdown: %v", err)
	}
	for _, want := range []string{"```mermaid", "고객지원 처리 흐름", "고객 발송 전 승인", "환불"} {
		if !strings.Contains(string(graphData), want) {
			t.Fatalf("support ticket graph markdown missing %q:\n%s", want, string(graphData))
		}
	}
	svgData, err := os.ReadFile(graphSVG)
	if err != nil {
		t.Fatalf("read support ticket SVG chart: %v", err)
	}
	for _, want := range []string{"<svg", "고객지원 티켓·장애 대응 패키지", "긴급", "6건", "피드백"} {
		if !strings.Contains(string(svgData), want) {
			t.Fatalf("support ticket SVG chart missing %q:\n%s", want, string(svgData))
		}
	}
}

func TestAssistantDepartmentWorkflowSupportTicketUsesLanguagePack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply, handled := assistantDepartmentWorkflowReply(ListenOptions{TargetID: "argos-assistant-support-ticket-en", Mode: "assistant"}, "create a customer support ticket triage package with refund requests, incident response, reply drafts, and escalation table")
	if !handled {
		t.Fatal("support ticket workflow should handle English support package requests")
	}
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Customer Support Ticket and Incident Response Package",
		"Ticket triage",
		"Reply drafts",
		"Escalation",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("support ticket English language pack reply missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"고객지원 티켓·장애 대응 패키지", "티켓 분류"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("support ticket English reply should not expose Korean text %q:\n%s", unwanted, visible)
		}
	}
}

func TestAssistantDepartmentWorkflowProductPRDReleasePackage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-product-prd", Mode: "assistant"}, "제품팀 신기능 PRD와 로드맵, 백로그 표, 릴리즈 노트 PPT를 만들어줘")
	visible := signalReplyVisibleText(reply)
	attachments := signalReplyAttachments(reply)
	for _, want := range []string{
		"제품팀 PRD·로드맵·릴리즈 준비 패키지",
		"문제 정의",
		"요구사항",
		"릴리즈 준비",
		"실제 배포",
		"Mermaid 그래프 Markdown",
		"SVG 차트 스냅샷",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("product PRD workflow visible reply missing %q:\n%s", want, visible)
		}
	}
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("product PRD workflow missing %s attachment: %#v", ext, attachments)
		}
	}
	graphMarkdown := ""
	graphSVG := ""
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), "-graphs.md") {
			graphMarkdown = attachment
		}
		if strings.HasSuffix(strings.ToLower(attachment), "-chart.svg") {
			graphSVG = attachment
		}
	}
	if graphMarkdown == "" || graphSVG == "" {
		t.Fatalf("product PRD workflow should attach graph markdown and SVG chart: %#v", attachments)
	}
	graphData, err := os.ReadFile(graphMarkdown)
	if err != nil {
		t.Fatalf("read product PRD graph markdown: %v", err)
	}
	for _, want := range []string{"```mermaid", "제품 개발 흐름", "배포 전 승인", "릴리즈 전 확인표"} {
		if !strings.Contains(string(graphData), want) {
			t.Fatalf("product PRD graph markdown missing %q:\n%s", want, string(graphData))
		}
	}
	svgData, err := os.ReadFile(graphSVG)
	if err != nil {
		t.Fatalf("read product PRD SVG chart: %v", err)
	}
	for _, want := range []string{"<svg", "제품팀 PRD·로드맵·릴리즈 준비 패키지", "문제", "91점", "출시"} {
		if !strings.Contains(string(svgData), want) {
			t.Fatalf("product PRD SVG chart missing %q:\n%s", want, string(svgData))
		}
	}
}

func TestAssistantDepartmentWorkflowProductPRDUsesLanguagePack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply, handled := assistantDepartmentWorkflowReply(ListenOptions{TargetID: "argos-assistant-product-prd-en", Mode: "assistant"}, "create a product PRD roadmap release notes package with backlog table and metrics")
	if !handled {
		t.Fatal("product PRD workflow should handle English product package requests")
	}
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Product PRD, Roadmap, and Release Readiness Package",
		"Problem definition",
		"Requirements",
		"Release readiness",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("product PRD English language pack reply missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"제품팀 PRD·로드맵·릴리즈 준비 패키지", "문제 정의"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("product PRD English reply should not expose Korean text %q:\n%s", unwanted, visible)
		}
	}
}

func TestAssistantDepartmentWorkflowProductPRDDoesNotStealMarketingProductGrowth(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-product-growth", Mode: "assistant"}, "제품군 A 성장 원인만 임원 보고용으로 한 페이지 요약해줘")
	visible := signalReplyVisibleText(reply)
	if !strings.Contains(visible, "제품군 A 성장 원인 임원 1페이지 보고") {
		t.Fatalf("product growth request should stay on marketing follow-up route:\n%s", visible)
	}
	if strings.Contains(visible, "제품팀 PRD·로드맵·릴리즈 준비 패키지") {
		t.Fatalf("product PRD route should not steal product group growth reports:\n%s", visible)
	}
}

func TestAssistantDepartmentWorkflowLegalContractReviewPackage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-legal-contract", Mode: "assistant"}, "법무팀 SaaS 계약서와 벤더 온보딩 리스크를 조항별 표와 협상 메일, PPT로 만들어줘")
	visible := signalReplyVisibleText(reply)
	attachments := signalReplyAttachments(reply)
	for _, want := range []string{
		"계약서·벤더 온보딩 검토 패키지",
		"조항별 리스크",
		"협상 질문",
		"승인 체크리스트",
		"서명, 발송, 결제",
		"Mermaid 그래프 Markdown",
		"SVG 차트 스냅샷",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("legal contract workflow visible reply missing %q:\n%s", want, visible)
		}
	}
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("legal contract workflow missing %s attachment: %#v", ext, attachments)
		}
	}
	graphMarkdown := ""
	graphSVG := ""
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), "-graphs.md") {
			graphMarkdown = attachment
		}
		if strings.HasSuffix(strings.ToLower(attachment), "-chart.svg") {
			graphSVG = attachment
		}
	}
	if graphMarkdown == "" || graphSVG == "" {
		t.Fatalf("legal contract workflow should attach graph markdown and SVG chart: %#v", attachments)
	}
	graphData, err := os.ReadFile(graphMarkdown)
	if err != nil {
		t.Fatalf("read legal contract graph markdown: %v", err)
	}
	for _, want := range []string{"```mermaid", "계약 검토 흐름", "서명·발송·결제 전 멈춤", "법무 검토"} {
		if !strings.Contains(string(graphData), want) {
			t.Fatalf("legal contract graph markdown missing %q:\n%s", want, string(graphData))
		}
	}
	svgData, err := os.ReadFile(graphSVG)
	if err != nil {
		t.Fatalf("read legal contract SVG chart: %v", err)
	}
	for _, want := range []string{"<svg", "계약서·벤더 온보딩 검토 패키지", "책임", "88점", "개인정보"} {
		if !strings.Contains(string(svgData), want) {
			t.Fatalf("legal contract SVG chart missing %q:\n%s", want, string(svgData))
		}
	}
}

func TestAssistantDepartmentWorkflowLegalContractUsesLanguagePack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply, handled := assistantDepartmentWorkflowReply(ListenOptions{TargetID: "argos-assistant-legal-contract-en", Mode: "assistant"}, "create a vendor contract review package with clause risk matrix, negotiation email, DPA, SLA, and approval checklist")
	if !handled {
		t.Fatal("legal contract workflow should handle English contract package requests")
	}
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Contract and Vendor Onboarding Review Package",
		"Clause risk",
		"Vendor onboarding",
		"Approval boundary",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("legal contract English language pack reply missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"계약서·벤더 온보딩 검토 패키지", "조항별 리스크"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("legal contract English reply should not expose Korean text %q:\n%s", unwanted, visible)
		}
	}
}

func TestAssistantDepartmentWorkflowOnboardingTrainingPackage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-onboarding", Mode: "assistant"}, "인사팀 신입사원 온보딩 교육자료와 30/60/90일 일정표, 퀴즈, 안내 메일을 PPT로 만들어줘")
	visible := signalReplyVisibleText(reply)
	attachments := signalReplyAttachments(reply)
	for _, want := range []string{
		"온보딩·교육 운영 패키지",
		"첫날 준비",
		"30/60/90일 계획",
		"교육 퀴즈",
		"계정 생성",
		"Mermaid 그래프 Markdown",
		"SVG 차트 스냅샷",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("onboarding workflow visible reply missing %q:\n%s", want, visible)
		}
	}
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("onboarding workflow missing %s attachment: %#v", ext, attachments)
		}
	}
	graphMarkdown := ""
	graphSVG := ""
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), "-graphs.md") {
			graphMarkdown = attachment
		}
		if strings.HasSuffix(strings.ToLower(attachment), "-chart.svg") {
			graphSVG = attachment
		}
	}
	if graphMarkdown == "" || graphSVG == "" {
		t.Fatalf("onboarding workflow should attach graph markdown and SVG chart: %#v", attachments)
	}
	graphData, err := os.ReadFile(graphMarkdown)
	if err != nil {
		t.Fatalf("read onboarding graph markdown: %v", err)
	}
	for _, want := range []string{"```mermaid", "온보딩 운영 흐름", "계정·권한·메일 실행 전 승인", "90일 리뷰"} {
		if !strings.Contains(string(graphData), want) {
			t.Fatalf("onboarding graph markdown missing %q:\n%s", want, string(graphData))
		}
	}
	svgData, err := os.ReadFile(graphSVG)
	if err != nil {
		t.Fatalf("read onboarding SVG chart: %v", err)
	}
	for _, want := range []string{"<svg", "온보딩·교육 운영 패키지", "첫날", "92점", "90일"} {
		if !strings.Contains(string(svgData), want) {
			t.Fatalf("onboarding SVG chart missing %q:\n%s", want, string(svgData))
		}
	}
}

func TestAssistantDepartmentWorkflowOnboardingUsesLanguagePack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply, handled := assistantDepartmentWorkflowReply(ListenOptions{TargetID: "argos-assistant-onboarding-en", Mode: "assistant"}, "create a new hire onboarding training package with 30/60/90 schedule, quiz, checklist, and welcome email")
	if !handled {
		t.Fatal("onboarding workflow should handle English onboarding package requests")
	}
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Onboarding and Training Operations Package",
		"Day-one setup",
		"Week-one training",
		"30/60/90 plan",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("onboarding English language pack reply missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"온보딩·교육 운영 패키지", "첫날 준비"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("onboarding English reply should not expose Korean text %q:\n%s", unwanted, visible)
		}
	}
}

func TestAssistantDepartmentWorkflowOnboardingDoesNotStealRecruitingPackage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-hr-recruiting", Mode: "assistant"}, "인사팀 채용공고, 후보자 스크리닝 표, 면접 질문지를 한 번에 만들어줘")
	visible := signalReplyVisibleText(reply)
	if !strings.Contains(visible, "인사팀 채용 실행 패키지") {
		t.Fatalf("recruiting request should stay on HR workflow:\n%s", visible)
	}
	if strings.Contains(visible, "온보딩·교육 운영 패키지") {
		t.Fatalf("onboarding route should not steal recruiting packages:\n%s", visible)
	}
}

func TestAssistantDepartmentWorkflowAdvertisingFollowUpsAreSpecific(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	copyReply := assistantReply(ListenOptions{TargetID: "argos-assistant-ad-copy", Mode: "assistant"}, "검색광고 카피만 더 날카롭게 20개 다시 써줘")
	copyVisible := signalReplyVisibleText(copyReply)
	if !strings.Contains(copyVisible, "검색광고 카피 20개 개선안") ||
		!strings.Contains(copyVisible, "A/B 테스트 표") ||
		strings.Contains(copyVisible, "광고기획팀 캠페인 실행 패키지") {
		t.Fatalf("copy follow-up should produce specific copy package:\n%s", copyVisible)
	}
	copyAttachments := signalReplyAttachments(copyReply)
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv"} {
		if !hasAttachmentExt(copyAttachments, ext) {
			t.Fatalf("copy follow-up missing %s attachment: %#v", ext, copyAttachments)
		}
	}
	copyMarkdown := firstAttachmentExt(copyAttachments, ".md")
	copyData, err := os.ReadFile(copyMarkdown)
	if err != nil {
		t.Fatalf("read copy markdown: %v", err)
	}
	if !strings.Contains(string(copyData), "## 카피 20개") ||
		strings.Count(string(copyData), ". ") < 20 {
		t.Fatalf("copy markdown should include 20 copy lines:\n%s", string(copyData))
	}

	scheduleReply := assistantReply(ListenOptions{TargetID: "argos-assistant-ad-schedule", Mode: "assistant"}, "이 KPI 표를 다음 주 실험 일정표로 바꿔줘")
	scheduleVisible := signalReplyVisibleText(scheduleReply)
	if !strings.Contains(scheduleVisible, "다음 주 광고 실험 일정표") ||
		!strings.Contains(scheduleVisible, "화~목은 A/B 테스트") ||
		strings.Contains(scheduleVisible, "광고기획팀 캠페인 실행 패키지") {
		t.Fatalf("schedule follow-up should produce experiment schedule:\n%s", scheduleVisible)
	}
	if !hasAttachmentExt(signalReplyAttachments(scheduleReply), ".xlsx") {
		t.Fatalf("schedule follow-up should attach xlsx: %#v", signalReplyAttachments(scheduleReply))
	}
}

func TestAssistantDepartmentWorkflowHRFollowUpsAreSpecific(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	jdReply := assistantReply(ListenOptions{TargetID: "argos-assistant-hr-jd", Mode: "assistant"}, "이 채용공고를 시니어 백엔드 엔지니어용으로 바꿔줘")
	jdVisible := signalReplyVisibleText(jdReply)
	if !strings.Contains(jdVisible, "시니어 백엔드 엔지니어 채용공고 패키지") ||
		!strings.Contains(jdVisible, "운영 안정성") ||
		strings.Contains(jdVisible, "인사팀 채용 실행 패키지 작업물") {
		t.Fatalf("senior backend JD follow-up should produce specific package:\n%s", jdVisible)
	}
	if !hasAttachmentExt(signalReplyAttachments(jdReply), ".xlsx") {
		t.Fatalf("senior backend JD should attach xlsx: %#v", signalReplyAttachments(jdReply))
	}

	compareReply := assistantReply(ListenOptions{TargetID: "argos-assistant-hr-compare", Mode: "assistant"}, "후보자 5명 이력서를 붙일테니 이 표 기준으로 비교해줘")
	compareVisible := signalReplyVisibleText(compareReply)
	if !strings.Contains(compareVisible, "후보자 5명 스크리닝 비교표") ||
		!strings.Contains(compareVisible, "면접 우선순위") ||
		strings.Contains(compareVisible, "인사팀 채용 실행 패키지 작업물") {
		t.Fatalf("candidate comparison follow-up should produce specific package:\n%s", compareVisible)
	}
	if !hasAttachmentExt(signalReplyAttachments(compareReply), ".xlsx") {
		t.Fatalf("candidate comparison should attach xlsx: %#v", signalReplyAttachments(compareReply))
	}

	interviewReply := assistantReply(ListenOptions{TargetID: "argos-assistant-hr-interview", Mode: "assistant"}, "면접 질문을 더 압박 없는 말투로 바꿔줘")
	interviewVisible := signalReplyVisibleText(interviewReply)
	if !strings.Contains(interviewVisible, "압박 없는 면접 질문지") ||
		!strings.Contains(interviewVisible, "부드러운 말투") ||
		strings.Contains(interviewVisible, "인사팀 채용 실행 패키지 작업물") {
		t.Fatalf("gentle interview follow-up should produce specific package:\n%s", interviewVisible)
	}
	if !hasAttachmentExt(signalReplyAttachments(interviewReply), ".docx") {
		t.Fatalf("gentle interview should attach docx: %#v", signalReplyAttachments(interviewReply))
	}
}

func TestAssistantDepartmentWorkflowPublicAgencyFollowUpsAreSpecific(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	execReply := assistantReply(ListenOptions{TargetID: "argos-assistant-gov-exec", Mode: "assistant"}, "이 민원 보고서를 구청장 보고용 한 페이지로 줄여줘")
	execVisible := signalReplyVisibleText(execReply)
	if !strings.Contains(execVisible, "구청장 보고용 민원 1페이지 브리프") ||
		!strings.Contains(execVisible, "접수 1,039건") ||
		strings.Contains(execVisible, "공공기관 민원·정책 업무 패키지 작업물") {
		t.Fatalf("executive brief follow-up should produce specific package:\n%s", execVisible)
	}
	if !hasAttachmentExt(signalReplyAttachments(execReply), ".pptx") {
		t.Fatalf("executive brief should attach pptx: %#v", signalReplyAttachments(execReply))
	}

	noticeReply := assistantReply(ListenOptions{TargetID: "argos-assistant-gov-notice", Mode: "assistant"}, "복지 문의만 따로 시민 안내문으로 다시 써줘")
	noticeVisible := signalReplyVisibleText(noticeReply)
	if !strings.Contains(noticeVisible, "복지 문의 시민 안내문") ||
		!strings.Contains(noticeVisible, "제출서류 체크표") ||
		strings.Contains(noticeVisible, "공공기관 민원·정책 업무 패키지 작업물") {
		t.Fatalf("welfare notice follow-up should produce specific package:\n%s", noticeVisible)
	}
	if !hasAttachmentExt(signalReplyAttachments(noticeReply), ".docx") {
		t.Fatalf("welfare notice should attach docx: %#v", signalReplyAttachments(noticeReply))
	}

	budgetReply := assistantReply(ListenOptions{TargetID: "argos-assistant-gov-budget", Mode: "assistant"}, "예산 집행표를 분기별 그래프로 바꿔줘")
	budgetVisible := signalReplyVisibleText(budgetReply)
	if !strings.Contains(budgetVisible, "분기별 예산 집행 그래프 패키지") ||
		!strings.Contains(budgetVisible, "Q2 지연") ||
		strings.Contains(budgetVisible, "공공기관 민원·정책 업무 패키지 작업물") {
		t.Fatalf("budget graph follow-up should produce specific package:\n%s", budgetVisible)
	}
	if !hasAttachmentExt(signalReplyAttachments(budgetReply), ".xlsx") {
		t.Fatalf("budget graph should attach xlsx: %#v", signalReplyAttachments(budgetReply))
	}
}

func TestAssistantDepartmentWorkflowMarketingWarRoomPackage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-marketing-war-room", Mode: "assistant"}, "CMO 보고용 마케팅 워룸으로 시장조사, 매출, ROAS, 경쟁사, 광고 캠페인 조언을 그래프와 PPT로 만들어줘")
	visible := signalReplyVisibleText(reply)
	attachments := signalReplyAttachments(reply)
	for _, want := range []string{
		"CMO 마케팅 워룸 실행 패키지",
		"시장 신호",
		"광고 조언",
		"회의 결정",
		"Mermaid 그래프 Markdown",
		"SVG 차트 스냅샷",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("marketing war-room visible reply missing %q:\n%s", want, visible)
		}
	}
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("marketing war-room package missing %s attachment: %#v", ext, attachments)
		}
	}
	graphMarkdown := ""
	graphSVG := ""
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), "-graphs.md") {
			graphMarkdown = attachment
		}
		if strings.HasSuffix(strings.ToLower(attachment), "-chart.svg") {
			graphSVG = attachment
		}
	}
	if graphMarkdown == "" || graphSVG == "" {
		t.Fatalf("marketing war-room should attach graph markdown and SVG chart: %#v", attachments)
	}
	graphData, err := os.ReadFile(graphMarkdown)
	if err != nil {
		t.Fatalf("read marketing war-room graph markdown: %v", err)
	}
	for _, want := range []string{"```mermaid", "CMO 워룸 판단 흐름", "경쟁사 메시지", "광고 집행·예산 변경 전 승인"} {
		if !strings.Contains(string(graphData), want) {
			t.Fatalf("marketing war-room graph markdown missing %q:\n%s", want, string(graphData))
		}
	}
	svgData, err := os.ReadFile(graphSVG)
	if err != nil {
		t.Fatalf("read marketing war-room SVG: %v", err)
	}
	for _, want := range []string{"<svg", "CMO 마케팅 워룸 실행 패키지", "시장", "92점", "실험"} {
		if !strings.Contains(string(svgData), want) {
			t.Fatalf("marketing war-room SVG missing %q:\n%s", want, string(svgData))
		}
	}
}

func TestAssistantDepartmentWorkflowMarketingWarRoomUsesLanguagePack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply, handled := assistantDepartmentWorkflowReply(ListenOptions{TargetID: "argos-assistant-marketing-war-room-en", Mode: "assistant"}, "create a CMO marketing war room package with market signals, revenue, ROAS, competitor messaging, funnel graph, and PPT")
	if !handled {
		t.Fatal("marketing war-room workflow should handle English CMO package requests")
	}
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"CMO Marketing War Room Execution Package",
		"Market signal",
		"Ad advice",
		"No ad launch",
		"Useful follow-up commands",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("marketing war-room English reply missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"시장 신호", "광고 조언", "마케팅 워룸"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("marketing war-room English reply should not expose Korean text %q:\n%s", unwanted, visible)
		}
	}
}

func TestAssistantDepartmentWorkflowMobileLinksUseLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_HOST", "argos.example.test")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_SERVER_DISABLED", "1")
	files := []string{
		filepath.Join(home, "report.docx"),
		filepath.Join(home, "deck.pptx"),
		filepath.Join(home, "table.xlsx"),
		filepath.Join(home, "chart.svg"),
		filepath.Join(home, "voice.mp3"),
		filepath.Join(home, "source.md"),
		filepath.Join(home, "data.csv"),
	}
	for _, file := range files {
		if err := os.WriteFile(file, []byte("artifact"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	lines := assistantWorkflowMobileLinkLines(files, 6)
	got := strings.Join(appendAssistantWorkflowMobileLinkLines([]string{"Summary"}, lines), "\n")
	for _, want := range []string{"Open on mobile:", "DOCX report", "PPTX deck", "XLSX table", "SVG chart", "MP3 voice brief", "http://argos.example.test:48303/argos/"} {
		if !strings.Contains(got, want) {
			t.Fatalf("mobile links missing %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{home, "모바일", "보고서", "발표자료"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("mobile links should not expose %q:\n%s", unwanted, got)
		}
	}
}

func TestAssistantDepartmentWorkflowMarketingSalesFollowUpsAreSpecific(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	baseReply := assistantReply(ListenOptions{TargetID: "argos-assistant-marketing-sales", Mode: "assistant"}, "회사 마케팅 부서에서 지난 6개월 매출과 ROAS를 분석해서 그래프화하고 회의자료로 만들어줘")
	baseVisible := signalReplyVisibleText(baseReply)
	if !strings.Contains(baseVisible, "마케팅 매출 분석 실행 패키지") ||
		!strings.Contains(baseVisible, "Mermaid 그래프 Markdown") ||
		!strings.Contains(baseVisible, "SVG 차트 스냅샷") ||
		!strings.Contains(baseVisible, "제품군 A가 6개월 성장") {
		t.Fatalf("marketing sales package should include graph artifact in visible summary:\n%s", baseVisible)
	}
	baseAttachments := signalReplyAttachments(baseReply)
	if !hasAttachmentExt(baseAttachments, ".pptx") || !hasAttachmentExt(baseAttachments, ".xlsx") || !hasAttachmentExt(baseAttachments, ".svg") {
		t.Fatalf("marketing sales package should attach pptx/xlsx/svg: %#v", baseAttachments)
	}
	graphMarkdown := ""
	graphSVG := ""
	for _, attachment := range baseAttachments {
		if strings.HasSuffix(strings.ToLower(attachment), "-graphs.md") {
			graphMarkdown = attachment
		}
		if strings.HasSuffix(strings.ToLower(attachment), "-chart.svg") {
			graphSVG = attachment
		}
	}
	if graphMarkdown == "" {
		t.Fatalf("marketing sales package should attach Mermaid graph markdown: %#v", baseAttachments)
	}
	if graphSVG == "" {
		t.Fatalf("marketing sales package should attach SVG chart snapshot: %#v", baseAttachments)
	}
	graphData, err := os.ReadFile(graphMarkdown)
	if err != nil {
		t.Fatalf("read graph markdown: %v", err)
	}
	graphContent := string(graphData)
	for _, want := range []string{"```mermaid", "매출 성장 흐름", "ROAS 기준 예산 조정 방향", "영업 퍼널 병목"} {
		if !strings.Contains(graphContent, want) {
			t.Fatalf("graph markdown missing %q:\n%s", want, graphContent)
		}
	}
	svgData, err := os.ReadFile(graphSVG)
	if err != nil {
		t.Fatalf("read graph svg: %v", err)
	}
	svgContent := string(svgData)
	for _, want := range []string{"<svg", "마케팅 매출 분석 실행 패키지", "1월", "188M"} {
		if !strings.Contains(svgContent, want) {
			t.Fatalf("graph svg missing %q:\n%s", want, svgContent)
		}
	}

	growthReply := assistantReply(ListenOptions{TargetID: "argos-assistant-marketing-growth", Mode: "assistant"}, "제품군 A 성장 원인만 임원 보고용으로 한 페이지 요약해줘")
	growthVisible := signalReplyVisibleText(growthReply)
	if !strings.Contains(growthVisible, "제품군 A 성장 원인 임원 1페이지 보고") ||
		!strings.Contains(growthVisible, "검색 수요") ||
		strings.Contains(growthVisible, "마케팅 매출 분석 실행 패키지 작업물") {
		t.Fatalf("product growth follow-up should produce specific package:\n%s", growthVisible)
	}
	if !hasAttachmentExt(signalReplyAttachments(growthReply), ".pptx") {
		t.Fatalf("product growth brief should attach pptx: %#v", signalReplyAttachments(growthReply))
	}

	roasReply := assistantReply(ListenOptions{TargetID: "argos-assistant-marketing-roas", Mode: "assistant"}, "ROAS 낮은 채널을 줄이는 예산 재배분안을 만들어줘")
	roasVisible := signalReplyVisibleText(roasReply)
	if !strings.Contains(roasVisible, "ROAS 기반 광고 예산 재배분안") ||
		!strings.Contains(roasVisible, "검색광고·리테일미디어") ||
		strings.Contains(roasVisible, "마케팅 매출 분석 실행 패키지 작업물") {
		t.Fatalf("ROAS follow-up should produce specific package:\n%s", roasVisible)
	}
	if !hasAttachmentExt(signalReplyAttachments(roasReply), ".xlsx") {
		t.Fatalf("ROAS plan should attach xlsx: %#v", signalReplyAttachments(roasReply))
	}

	leadReply := assistantReply(ListenOptions{TargetID: "argos-assistant-marketing-leads", Mode: "assistant"}, "상위 리드 20개 후속 콜 SLA와 영업 액션표를 만들어줘")
	leadVisible := signalReplyVisibleText(leadReply)
	if !strings.Contains(leadVisible, "상위 리드 20개 영업 후속 액션표") ||
		!strings.Contains(leadVisible, "전화 스크립트") ||
		strings.Contains(leadVisible, "마케팅 매출 분석 실행 패키지 작업물") {
		t.Fatalf("lead action follow-up should produce specific package:\n%s", leadVisible)
	}
	if !hasAttachmentExt(signalReplyAttachments(leadReply), ".docx") {
		t.Fatalf("lead action plan should attach docx: %#v", signalReplyAttachments(leadReply))
	}

	battlecardReply := assistantReply(ListenOptions{TargetID: "argos-assistant-marketing-battlecard", Mode: "assistant"}, "경쟁사 3곳 비교해서 세일즈 배틀카드 만들어줘")
	battlecardVisible := signalReplyVisibleText(battlecardReply)
	if !strings.Contains(battlecardVisible, "경쟁사 3곳 세일즈 배틀카드") ||
		!strings.Contains(battlecardVisible, "고객 질문별 답변") ||
		strings.Contains(battlecardVisible, "마케팅 매출 분석 실행 패키지 작업물") {
		t.Fatalf("competitor battlecard should produce specific package:\n%s", battlecardVisible)
	}
	if !hasAttachmentExt(signalReplyAttachments(battlecardReply), ".xlsx") {
		t.Fatalf("competitor battlecard should attach xlsx: %#v", signalReplyAttachments(battlecardReply))
	}

	meetingReply := assistantReply(ListenOptions{TargetID: "argos-assistant-marketing-meeting", Mode: "assistant"}, "다음 주 매출 회의 아젠다와 질문, 회의자료를 준비해줘")
	meetingVisible := signalReplyVisibleText(meetingReply)
	if !strings.Contains(meetingVisible, "다음 주 매출 회의 아젠다와 질문지") ||
		!strings.Contains(meetingVisible, "결정 질문") ||
		strings.Contains(meetingVisible, "마케팅 매출 분석 실행 패키지 작업물") {
		t.Fatalf("revenue meeting follow-up should produce specific package:\n%s", meetingVisible)
	}
	if !hasAttachmentExt(signalReplyAttachments(meetingReply), ".pptx") {
		t.Fatalf("revenue meeting pack should attach pptx: %#v", signalReplyAttachments(meetingReply))
	}
}

func TestAssistantDepartmentWorkflowTravelAndShoppingArtifacts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	travelReply := assistantReply(ListenOptions{TargetID: "argos-assistant-travel-pack", Mode: "assistant"}, "후쿠오카 2박3일 여행 준비물, 예산, 동선 PPT로 만들어줘")
	travelVisible := signalReplyVisibleText(travelReply)
	if !strings.Contains(travelVisible, "후쿠오카 2박 3일 여행 준비 패키지") ||
		!strings.Contains(travelVisible, "준비물, 예산, 동선") ||
		!strings.Contains(travelVisible, "예약 확정 전 확인") {
		t.Fatalf("travel prep bundle should produce concrete package:\n%s", travelVisible)
	}
	travelAttachments := signalReplyAttachments(travelReply)
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv"} {
		if !hasAttachmentExt(travelAttachments, ext) {
			t.Fatalf("travel prep bundle missing %s attachment: %#v", ext, travelAttachments)
		}
	}

	shoppingReply := assistantReply(ListenOptions{TargetID: "argos-assistant-shopping-workbook", Mode: "assistant"}, "쿠팡에서 아이패드 키보드 후보 5개 비교표와 구매 체크리스트 만들어줘")
	shoppingVisible := signalReplyVisibleText(shoppingReply)
	if !strings.Contains(shoppingVisible, "쿠팡 상품 비교와 구매 리허설 패키지") ||
		!strings.Contains(shoppingVisible, "구매 전 확인") ||
		!strings.Contains(shoppingVisible, "구매 실행 승인") ||
		!strings.Contains(shoppingVisible, "Mermaid 그래프 Markdown") ||
		!strings.Contains(shoppingVisible, "SVG 차트 스냅샷") {
		t.Fatalf("shopping decision workbook should produce concrete package:\n%s", shoppingVisible)
	}
	shoppingAttachments := signalReplyAttachments(shoppingReply)
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg"} {
		if !hasAttachmentExt(shoppingAttachments, ext) {
			t.Fatalf("shopping decision workbook missing %s attachment: %#v", ext, shoppingAttachments)
		}
	}
	shoppingGraphMarkdown := ""
	shoppingGraphSVG := ""
	for _, attachment := range shoppingAttachments {
		if strings.HasSuffix(strings.ToLower(attachment), "-graphs.md") {
			shoppingGraphMarkdown = attachment
		}
		if strings.HasSuffix(strings.ToLower(attachment), "-chart.svg") {
			shoppingGraphSVG = attachment
		}
	}
	if shoppingGraphMarkdown == "" || shoppingGraphSVG == "" {
		t.Fatalf("shopping decision workbook should attach graph markdown and SVG chart: %#v", shoppingAttachments)
	}
	shoppingGraphData, err := os.ReadFile(shoppingGraphMarkdown)
	if err != nil {
		t.Fatalf("read shopping graph markdown: %v", err)
	}
	for _, want := range []string{"```mermaid", "쿠팡 구매 리허설 흐름", "구매 실행 승인 전 멈춤", "최종 클릭 전 확인표"} {
		if !strings.Contains(string(shoppingGraphData), want) {
			t.Fatalf("shopping graph markdown missing %q:\n%s", want, string(shoppingGraphData))
		}
	}
	shoppingSVGData, err := os.ReadFile(shoppingGraphSVG)
	if err != nil {
		t.Fatalf("read shopping SVG chart: %v", err)
	}
	for _, want := range []string{"<svg", "쿠팡 상품 비교와 구매 리허설 패키지", "가격", "82점", "승인"} {
		if !strings.Contains(string(shoppingSVGData), want) {
			t.Fatalf("shopping SVG chart missing %q:\n%s", want, string(shoppingSVGData))
		}
	}
	rehearsalReply := assistantReply(ListenOptions{TargetID: "argos-assistant-shopping-rehearsal", Mode: "assistant"}, "쿠팡에서 아이패드 키보드 후보 5개 비교표와 구매 체크리스트, 브라우저 리허설 흐름도를 PPT와 그래프로 만들어줘")
	rehearsalVisible := signalReplyVisibleText(rehearsalReply)
	if !strings.Contains(rehearsalVisible, "쿠팡 상품 비교와 구매 리허설 패키지") ||
		!strings.Contains(rehearsalVisible, "SVG 차트 스냅샷") ||
		strings.Contains(rehearsalVisible, "쿠팡 상품 후보 비교 패키지") {
		t.Fatalf("shopping rehearsal should route to graph workbook, not real-search package:\n%s", rehearsalVisible)
	}

	if _, handled := assistantDepartmentWorkflowReply(ListenOptions{TargetID: "argos-assistant-shopping-search", Mode: "assistant"}, "쿠팡에서 생수 500ml 20개 로켓배송으로 가격 리뷰 좋은 후보 3개 비교해줘"); handled {
		t.Fatal("plain Coupang search/compare request should remain on the real shopping browser flow, not department artifact workflow")
	}
}

func TestAssistantDepartmentWorkflowShoppingDecisionUsesLanguagePack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply, handled := assistantDepartmentWorkflowReply(ListenOptions{TargetID: "argos-assistant-shopping-workbook-en", Mode: "assistant"}, "create a Coupang product comparison workbook with purchase checklist, browser rehearsal, spreadsheet, and PPT")
	if !handled {
		t.Fatal("shopping decision workflow should handle English Coupang workbook requests")
	}
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Coupang Product Comparison and Purchase Rehearsal Package",
		"Candidate comparison",
		"Browser rehearsal",
		"purchase execution approval",
		"Useful follow-up commands",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("shopping decision English reply missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"쿠팡 상품", "구매 실행 승인", "최종 구매"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("shopping decision English reply should not expose Korean text %q:\n%s", unwanted, visible)
		}
	}
}

func TestAssistantDepartmentWorkflowMeetingMinutesPack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-meeting-minutes", Mode: "assistant"}, "이 회의 메모를 결정사항, 할 일, 리스크가 보이는 회의록으로 만들어줘")
	visible := signalReplyVisibleText(reply)
	if !strings.Contains(visible, "회의 메모 기반 실행 회의록 패키지") ||
		!strings.Contains(visible, "결정사항, 할 일, 담당자, 마감, 리스크") ||
		!strings.Contains(visible, "액션아이템") {
		t.Fatalf("meeting minutes pack should produce concrete visible package:\n%s", visible)
	}
	attachments := signalReplyAttachments(reply)
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("meeting minutes pack missing %s attachment: %#v", ext, attachments)
		}
	}
	markdown := firstAttachmentExt(attachments, ".md")
	data, err := os.ReadFile(markdown)
	if err != nil {
		t.Fatalf("read meeting markdown: %v", err)
	}
	for _, want := range []string{"## 결정사항", "## 할 일", "## 리스크", "## 다음 회의 안건"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("meeting markdown missing %q:\n%s", want, string(data))
		}
	}
}

func TestAssistantDepartmentWorkflowExecutiveDecisionUsesStructuredRenderer(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))

	reply, handled := assistantDepartmentWorkflowReply(ListenOptions{TargetID: "argos-assistant-executive-decision", Mode: "assistant"}, "경영회의에서 가격 정책, 채널 예산, 제품 출시, 인력 배치 의사결정 안건을 보고서와 PPT와 그래프와 음성 브리핑으로 만들어줘")
	if !handled {
		t.Fatal("executive decision workflow should handle a concrete leadership decision request")
	}
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"경영회의 의사결정·액션 패키지",
		"구조화 렌더러",
		"가격 정책",
		"채널 예산",
		"Mermaid 그래프 Markdown",
		"SVG 차트 스냅샷",
		"edge-tts MP3",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("executive decision visible reply missing %q:\n%s", want, visible)
		}
	}
	attachments := signalReplyAttachments(reply)
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg", ".mp3"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("executive decision package missing %s attachment: %#v", ext, attachments)
		}
	}
	graphMarkdown := ""
	graphSVG := ""
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), "-graphs.md") {
			graphMarkdown = attachment
		}
		if strings.HasSuffix(strings.ToLower(attachment), "-chart.svg") {
			graphSVG = attachment
		}
	}
	if graphMarkdown == "" || graphSVG == "" {
		t.Fatalf("executive decision package should attach graph markdown and SVG chart: %#v", attachments)
	}
	graphData, err := os.ReadFile(graphMarkdown)
	if err != nil {
		t.Fatalf("read executive decision graph markdown: %v", err)
	}
	for _, want := range []string{"```mermaid", "경영회의 의사결정 흐름", "예산 변경·외부 발송·시스템 변경 전 승인", "의사결정 우선순위"} {
		if !strings.Contains(string(graphData), want) {
			t.Fatalf("executive decision graph markdown missing %q:\n%s", want, string(graphData))
		}
	}
	svgData, err := os.ReadFile(graphSVG)
	if err != nil {
		t.Fatalf("read executive decision SVG: %v", err)
	}
	for _, want := range []string{"<svg", "경영회의 의사결정·액션 패키지", "결정", "4건", "승인"} {
		if !strings.Contains(string(svgData), want) {
			t.Fatalf("executive decision SVG missing %q:\n%s", want, string(svgData))
		}
	}
	for _, key := range []string{
		"assistant.workflow.executive_decision.doc",
		"assistant.workflow.executive_decision.deck",
		"assistant.workflow.executive_decision.sheet",
		"assistant.workflow.executive_decision.graph",
	} {
		if got := lang.T(key); got != key {
			t.Fatalf("executive decision should not add long body key %s; got %q", key, got)
		}
	}
}

func TestAssistantDepartmentWorkflowExecutiveDecisionUsesLanguagePack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))

	reply, handled := assistantDepartmentWorkflowReply(ListenOptions{TargetID: "argos-assistant-executive-decision-en", Mode: "assistant"}, "create an executive decision package with pricing policy, channel budget, product launch, staffing options, risk, action owners, graph, PPT, and voice briefing")
	if !handled {
		t.Fatal("executive decision workflow should handle English leadership decision requests")
	}
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Executive Decision and Action Package",
		"structured renderer",
		"pricing policy",
		"channel budget",
		"No budget change",
		"Useful follow-up commands",
		"Voice script preview:",
		"This is the Argos department workflow briefing.",
		"edge-tts MP3 department workflow brief",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("executive decision English reply missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"경영회의", "가격 정책", "채널 예산", "그래프", "음성 원고", "아르고스 부서", "중요 포인트", "첨부한 문서"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("executive decision English reply should not expose Korean text %q:\n%s", unwanted, visible)
		}
	}
	attachments := signalReplyAttachments(reply)
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg", ".mp3"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("executive decision English package missing %s attachment: %#v", ext, attachments)
		}
	}
	graphMarkdown := ""
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), "-graphs.md") {
			graphMarkdown = attachment
			break
		}
	}
	if graphMarkdown == "" {
		t.Fatalf("executive decision English package should attach graph markdown: %#v", attachments)
	}
	graphData, err := os.ReadFile(graphMarkdown)
	if err != nil {
		t.Fatalf("read executive decision English graph markdown: %v", err)
	}
	graph := string(graphData)
	for _, want := range []string{"Executive Decision and Action Package graphs", "Mermaid graphs and chart source data", "Executive Decision Flow", "Decision Priorities"} {
		if !strings.Contains(graph, want) {
			t.Fatalf("executive decision English graph markdown missing %q:\n%s", want, graph)
		}
	}
	for _, unwanted := range []string{"그래프", "모바일", "의사결정 우선순위", "예산 변경"} {
		if strings.Contains(graph, unwanted) {
			t.Fatalf("executive decision English graph markdown should not expose Korean text %q:\n%s", unwanted, graph)
		}
	}
	graphSVG := firstAttachmentExt(attachments, ".svg")
	svgData, err := os.ReadFile(graphSVG)
	if err != nil {
		t.Fatalf("read executive decision English SVG: %v", err)
	}
	svg := string(svgData)
	for _, want := range []string{"Executive Decision and Action Package", "Decision", "4items", "Approval"} {
		if !strings.Contains(svg, want) {
			t.Fatalf("executive decision English SVG missing %q:\n%s", want, svg)
		}
	}
	if containsHangul(svg) {
		t.Fatalf("executive decision English SVG should not expose Korean text:\n%s", svg)
	}
}

func TestAssistantDepartmentChartSeriesUsesLanguagePack(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "en")
	for _, kind := range []string{
		"marketing_sales_analysis",
		"public_agency_budget_graph",
		"public_agency_civil_service",
		"hr_recruiting",
		"chemical_margin_risk",
		"booking_candidate_package",
		"finance_expense_reconciliation",
		"sales_rfp_response",
		"support_ticket_response",
		"product_prd_release",
		"legal_contract_review",
		"onboarding_training",
	} {
		points, unit, note := assistantDepartmentChartSeries(assistantDepartmentWorkflowSpec{
			Kind:       kind,
			Title:      "Language Neutral Chart",
			Highlights: []string{"First point"},
		})
		if len(points) == 0 {
			t.Fatalf("%s chart should have points", kind)
		}
		parts := []string{unit, note}
		for _, point := range points {
			parts = append(parts, point.Label)
		}
		if text := strings.Join(parts, "\n"); containsHangul(text) {
			t.Fatalf("%s chart labels should come from English language pack:\n%s", kind, text)
		}
	}
}

func TestAssistantDepartmentWorkflowSendUsesLocalizedTargetLabel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_BASE_URL", "https://argos.example.test/argos")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_SERVER_DISABLED", "1")
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatalf("upsert briefing target: %v", err)
	}

	reply, handled := assistantDepartmentWorkflowReply(ListenOptions{TargetID: "argos-assistant-executive-send-en", Mode: "assistant"}, "create an executive decision package with pricing policy, channel budget, product launch, staffing options, risk, action owners, graph, PPT, and voice briefing. send to argos-briefing")
	if !handled {
		t.Fatal("executive decision workflow send should handle English request")
	}
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Prepared the department workflow package for Signal delivery.",
		"Target: Briefing room",
		"The briefing room is one-way/no-reply",
		"Executive Decision and Action Package",
		"Ready-to-use result:",
		"Useful next command:",
		"No external send, budget change, purchase, booking, or deletion happens without explicit approval.",
		"Open on mobile:",
		"DOCX report: https://argos.example.test/argos/",
		"MP3 voice brief: https://argos.example.test/argos/",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("English send preview missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"Target: 보고방", "대상:", "보고방은", "보낼 내용"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("English send preview should not expose Korean text %q:\n%s", unwanted, visible)
		}
	}
	if containsHangul(visible) {
		t.Fatalf("English send preview should not expose Korean text:\n%s", visible)
	}
	attachments := signalReplyAttachments(reply)
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg", ".mp3"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("English send preview missing %s attachment: %#v", ext, attachments)
		}
	}
	for _, attachment := range attachments {
		lower := strings.ToLower(attachment)
		if strings.HasSuffix(lower, ".html") {
			t.Fatalf("HTML should not be attached for mobile Signal package: %#v", attachments)
		}
		if !(strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".csv") || strings.HasSuffix(lower, ".svg")) {
			continue
		}
		data, err := os.ReadFile(attachment)
		if err != nil {
			t.Fatalf("read English workflow artifact %s: %v", attachment, err)
		}
		if containsHangul(string(data)) {
			t.Fatalf("English workflow artifact should not expose Korean in %s:\n%s", attachment, string(data))
		}
	}
}

func containsHangul(value string) bool {
	for _, r := range value {
		if unicode.In(r, unicode.Hangul) {
			return true
		}
	}
	return false
}
