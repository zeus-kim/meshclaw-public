package messenger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meshclaw/meshclaw/internal/browserauto"
)

func TestAssistantCoupangShoppingPackageCreatesMobileArtifacts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_BASE_URL", "https://argos.example.test/argos")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_SERVER_DISABLED", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-shopping-package", Mode: "assistant"}, "쿠팡에서 생수 500ml 20개 로켓배송 가격 리뷰 좋은 후보 3개 비교해서 표와 PPT와 음성으로 준비해줘")
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"쿠팡 상품 후보 비교 패키지를 만들었습니다.",
		"검색어: 생수 500ml 20개",
		"쿠팡 후보 확인 상태: 실제 검색 후보 0개 / 대체 후보 3개 / 구매 완료 0건",
		"브라우저 준비 상태:",
		"구매 경계 상태: 최종 주문/결제는 `구매 실행 승인` 전에는 실행하지 않습니다.",
		"검색 결과 후보:",
		"바로 확인할 항목:",
		"최종 화면에서 읽을 값:",
		"내일 실제 테스트 전 준비:",
		"Mac mini 브라우저가 쿠팡에 로그인",
		"구매 전 안전 체크 SVG",
		"edge-tts MP3 음성파일",
		"구매 실행 승인",
		"화면값이 모두 맞으면 구매 실행 승인",
		"모바일에서 바로 열기:",
		"DOCX 보고서: https://argos.example.test/argos/",
		"MP3 음성 브리핑: https://argos.example.test/argos/",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("visible shopping package reply missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"meshclaw-attachment:", "/.meshclaw/evidence/", "/Documents/Argos Vault/"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("visible reply should not expose raw path %q:\n%s", unwanted, visible)
		}
	}
	if strings.Contains(visible, "검색어: 생수 500ml 20개 와") {
		t.Fatalf("shopping query should not leave trailing Korean particles:\n%s", visible)
	}
	if got := assistantShoppingPackageQuery("쿠팡에서 생수 500ml 20개 로켓배송 가격 리뷰 좋은 후보 3개 비교해서 표와 PPT와 음성으로 정리해서 비서방에 보내줘. 내일 실제 테스트 전 준비도 같이 보여줘."); got != "생수 500ml 20개" {
		t.Fatalf("shopping package query should remove Signal target and live-test instructions, got %q", got)
	}

	attachments := signalReplyAttachments(reply)
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg", ".mp3"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("shopping package missing %s attachment: %#v", ext, attachments)
		}
	}
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), ".html") {
			t.Fatalf("HTML should not be attached for mobile Signal package: %#v", attachments)
		}
	}

	markdown := firstAttachmentExt(attachments, ".md")
	data, err := os.ReadFile(markdown)
	if err != nil {
		t.Fatalf("read markdown attachment: %v", err)
	}
	content := string(data)
	for _, want := range []string{"# 쿠팡 상품 후보 비교 보고서", "쿠팡 후보 확인 상태: 실제 검색 후보 0개 / 대체 후보 3개 / 구매 완료 0건", "구매 경계 상태: 최종 주문/결제는 `구매 실행 승인` 전에는 실행하지 않습니다.", "상품명, 옵션, 수량, 총액, 도착 예정일, 배송지 표시 여부", "## 후보", "## 구매 전 확인 기준", "## 최종 화면에서 읽을 값:", "## 내일 실제 테스트 전 준비:", "화면값이 모두 맞다는 요약을 받은 뒤에만 `구매 실행 승인`", "생수 500ml 20개"} {
		if !strings.Contains(content, want) {
			t.Fatalf("markdown content missing %q:\n%s", want, content)
		}
	}
	safety := firstAttachmentExt(attachments, ".svg")
	safetyData, err := os.ReadFile(safety)
	if err != nil {
		t.Fatalf("read safety SVG attachment: %v", err)
	}
	for _, want := range []string{"<svg", "쿠팡 상품 후보 비교 패키지", "최종 화면에서 반드시 읽을 값", "구매 실행 승인"} {
		if !strings.Contains(string(safetyData), want) {
			t.Fatalf("safety SVG missing %q:\n%s", want, string(safetyData))
		}
	}
}

func TestAssistantCoupangShoppingPackageCanSendToBriefingWithVoice(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, "targets.json"))
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_BASE_URL", "https://argos.example.test/argos")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_SERVER_DISABLED", "1")
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatalf("upsert briefing target: %v", err)
	}
	if got := assistantShoppingPackageQuery("쿠팡에서 생수 500ml 20개 로켓배송 가격 리뷰 좋은 후보 3개 비교표와 PPT로 만들어서 보고방에 보내줘. 음성으로도 준비해줘."); got != "생수 500ml 20개" {
		t.Fatalf("shopping package query should keep only product conditions, got %q", got)
	}

	reply := assistantReply(
		ListenOptions{TargetID: "argos-assistant-shopping-send", Mode: "assistant"},
		"쿠팡에서 생수 500ml 20개 로켓배송 가격 리뷰 좋은 후보 3개 비교표와 PPT로 만들어서 보고방에 보내줘. 음성으로도 준비해줘.",
	)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"쿠팡 상품 후보 비교 패키지를 Signal로 보낼 준비를 했습니다.",
		"대상: 보고방",
		"보고방은 one-way/no-reply",
		"쿠팡 상품 후보 비교 패키지를 만들었습니다.",
		"검색어: 생수 500ml 20개",
		"쿠팡 후보 확인 상태: 실제 검색 후보 0개 / 대체 후보 3개 / 구매 완료 0건",
		"구매 경계 상태: 최종 주문/결제는 `구매 실행 승인` 전에는 실행하지 않습니다.",
		"구매 전 안전 체크 SVG",
		"edge-tts MP3 음성파일",
		"내일 실제 테스트 전 준비:",
		"Mac mini 브라우저가 쿠팡에 로그인",
		"최종 주문 클릭",
		"화면값이 모두 맞으면 구매 실행 승인",
		"모바일에서 바로 열기:",
		"DOCX 보고서: https://argos.example.test/argos/",
		"MP3 음성 브리핑: https://argos.example.test/argos/",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("visible send preview missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"meshclaw-attachment:", "/.meshclaw/evidence/", "/Documents/Argos Vault/"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("visible reply should not expose raw path %q:\n%s", unwanted, visible)
		}
	}

	attachments := signalReplyAttachments(reply)
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg", ".mp3"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("shopping package send preview missing %s attachment: %#v", ext, attachments)
		}
	}
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), ".html") {
			t.Fatalf("HTML should not be attached for mobile Signal package: %#v", attachments)
		}
	}
}

func TestAssistantCoupangShoppingPackageEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, "targets.json"))
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_BASE_URL", "https://argos.example.test/argos")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_SERVER_DISABLED", "1")
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatalf("upsert briefing target: %v", err)
	}
	if got := assistantShoppingPackageQuery("Coupang bottled water 500ml 20 pack Rocket delivery price reviews comparison table deck voice send to briefing room"); got != "bottled water 500ml 20 pack" {
		t.Fatalf("English shopping package query should keep only product conditions, got %q", got)
	}
	if got := assistantShoppingPackageQuery("Coupang bottled water 500ml 20 pack Rocket delivery price reviews table deck voice send to assistant room. Also show tomorrow live test prep."); got != "bottled water 500ml 20 pack" {
		t.Fatalf("English shopping package query should remove target and live-test instructions, got %q", got)
	}

	reply := assistantReply(
		ListenOptions{TargetID: "argos-assistant-shopping-send-en", Mode: "assistant"},
		"Coupang bottled water 500ml 20 pack Rocket delivery price reviews comparison table deck voice send to briefing room",
	)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Prepared the Coupang product-candidate package for Signal delivery.",
		"Target: Briefing room",
		"The briefing room is one-way/no-reply",
		"Created a Coupang product-candidate comparison package.",
		"Query: bottled water 500ml 20 pack",
		"Coupang candidate status: 0 live search candidates / 3 fallback candidates / 0 purchases completed",
		"Browser prep status:",
		"Purchase boundary status: final order/payment will not run before `purchase execution approved`.",
		"Search-result candidates:",
		"Check next:",
		"Fields to read on the final screen:",
		"Before tomorrow's live test:",
		"Confirm the Mac mini browser is logged in to Coupang",
		"Mobile-openable pre-purchase safety SVG",
		"edge-tts MP3 audio file",
		"purchase execution approved",
		"send `purchase execution approved` again as a separate message",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("English visible shopping package missing %q:\n%s", want, visible)
		}
	}
	if containsHangul(visible) {
		t.Fatalf("English visible shopping package should not expose Korean text:\n%s", visible)
	}
	for _, unwanted := range []string{"meshclaw-attachment:", "/.meshclaw/evidence/", "/Documents/Argos Vault/", "final order completed"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("visible reply should not expose or claim %q:\n%s", unwanted, visible)
		}
	}

	attachments := signalReplyAttachments(reply)
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg", ".mp3"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("English shopping package missing %s attachment: %#v", ext, attachments)
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
			t.Fatalf("read English shopping artifact %s: %v", attachment, err)
		}
		text := string(data)
		if containsHangul(text) {
			t.Fatalf("English shopping package artifact should not expose Korean text in %s:\n%s", attachment, text)
		}
	}

	reportMarkdown := ""
	for _, attachment := range attachments {
		if !strings.HasSuffix(strings.ToLower(attachment), ".md") {
			continue
		}
		data, err := os.ReadFile(attachment)
		if err != nil {
			t.Fatalf("read English markdown candidate: %v", err)
		}
		if strings.Contains(string(data), "# Coupang product-candidate comparison report") {
			reportMarkdown = string(data)
			break
		}
	}
	if reportMarkdown == "" {
		t.Fatalf("English shopping package should include localized report markdown: %#v", attachments)
	}
	for _, want := range []string{"# Coupang product-candidate comparison report", "Coupang candidate status: 0 live search candidates / 3 fallback candidates / 0 purchases completed", "Purchase boundary status: final order/payment will not run before `purchase execution approved`.", "product, option, quantity, total, arrival estimate, address visibility", "## Candidates", "## Pre-purchase Check Criteria", "## Fields to read on the final screen:", "## Before tomorrow's live test:", "Only after the screen-value summary is correct, send `purchase execution approved` again as a separate message.", "Do not click the final order button before `purchase execution approved`."} {
		if !strings.Contains(reportMarkdown, want) {
			t.Fatalf("English shopping package markdown missing %q:\n%s", want, reportMarkdown)
		}
	}
}

func TestAssistantCoupangShoppingPackageArtifactBoundaryMentionsArrivalEstimate(t *testing.T) {
	tests := []struct {
		name      string
		locale    string
		query     string
		request   string
		report    string
		deck      string
		voice     string
		searchURL string
	}{
		{
			name:      "ko",
			locale:    "ko",
			query:     "생수 500ml 20개",
			request:   "쿠팡에서 생수 500ml 20개 비교표와 PPT와 음성으로 준비해줘",
			report:    "상품명, 옵션, 수량, 총액, 도착 예정일, 배송지 표시 여부",
			deck:      "총액, 도착 예정일, 배송지 표시 여부",
			voice:     "총액, 도착 예정일, 배송지 표시 여부",
			searchURL: "https://www.coupang.com/np/search?q=%EC%83%9D%EC%88%98",
		},
		{
			name:      "en",
			locale:    "en",
			query:     "bottled water 500ml 20 pack",
			request:   "Coupang bottled water 500ml 20 pack comparison deck voice",
			report:    "product, option, quantity, total, arrival estimate, address visibility",
			deck:      "total, arrival estimate, address visibility",
			voice:     "total, arrival estimate, address visibility",
			searchURL: "https://www.coupang.com/np/search?q=bottled+water",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("MESHCLAW_LANG", tt.locale)
			candidates := assistantShoppingPackageFallbackCandidates(tt.query, tt.searchURL)
			sourceStatus := assistantShoppingPackageSourceStatusLine(0, len(candidates))
			browserStatus := assistantShoppingPackageBrowserStatusLine("")
			boundaryStatus := assistantShoppingPackageBoundaryStatusLine()
			report := assistantShoppingPackageReportBody(tt.request, tt.query, tt.searchURL, candidates, true, sourceStatus, browserStatus, boundaryStatus)
			deck := assistantShoppingPackageDeckBody(tt.query, tt.searchURL, candidates, true, sourceStatus, browserStatus, boundaryStatus)
			voice := assistantShoppingPackageVoiceScript(tt.query, candidates, true, 0, len(candidates), "")
			voiceApproval := "화면값 요약이 모두 맞으면"
			if tt.locale == "en" {
				voiceApproval = "screen-value summary is all correct"
			}
			for label, body := range map[string]string{
				"report": report,
				"deck":   deck,
				"voice":  voice,
			} {
				want := tt.report
				if label == "deck" {
					want = tt.deck
				}
				if label == "voice" {
					want = tt.voice
				}
				if !strings.Contains(body, want) {
					t.Fatalf("%s boundary missing %q:\n%s", label, want, body)
				}
				if label == "voice" && !strings.Contains(body, voiceApproval) {
					t.Fatalf("%s approval handoff missing %q:\n%s", label, voiceApproval, body)
				}
			}
		})
	}
}

func TestAssistantCoupangShoppingPackageRouting(t *testing.T) {
	if !isAssistantCoupangShoppingPackageRequest("쿠팡에서 생수 500ml 20개 로켓배송 가격 리뷰 좋은 후보 3개 비교해서 표와 PPT로 준비해줘") {
		t.Fatal("expected Coupang artifact package request to route to shopping package")
	}
	for _, input := range []string{
		"쿠팡에서 아이패드 키보드 후보 5개 비교표와 구매 체크리스트 만들어줘",
		"첫 번째 상품 상세 열고 총액 확인해줘",
		"1번 상품을 장바구니 직전 화면까지 준비해. 결제는 하지 마",
		"최종 화면의 상품명, 옵션, 총액, 배송지, 결제수단을 읽어줘",
		"구매 실행 승인",
	} {
		if isAssistantCoupangShoppingPackageRequest(input) {
			t.Fatalf("shopping package should not steal follow-up request: %s", input)
		}
	}
}

func TestAssistantCoupangCheckoutPrepPackageCanSendToBriefing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, "targets.json"))
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_BASE_URL", "https://argos.example.test/argos")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_SERVER_DISABLED", "1")
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatalf("upsert briefing target: %v", err)
	}
	if got := assistantCoupangCheckoutPrepPackageQuery("쿠팡에서 생수 500ml 20개를 1번 후보 기준으로 장바구니 직전까지 준비해서 보고방에 보내줘. 결제는 하지 마. 모바일 SVG 안전 체크와 음성으로도 준비해줘."); got != "생수 500ml 20개" {
		t.Fatalf("checkout prep query should keep only product conditions, got %q", got)
	}

	reply := assistantReply(
		ListenOptions{TargetID: "argos-assistant-checkout-prep", Mode: "assistant"},
		"쿠팡에서 생수 500ml 20개를 1번 후보 기준으로 장바구니 직전까지 준비해서 보고방에 보내줘. 결제는 하지 마. 음성으로도 준비해줘.",
	)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"쿠팡 구매 전 준비 카드를 Signal로 보낼 준비를 했습니다.",
		"대상: 보고방",
		"보고방은 one-way/no-reply",
		"쿠팡 구매 전 준비 카드를 만들었습니다.",
		"검색어: 생수 500ml 20개",
		"기준 후보: 1번",
		"구매 전 준비 상태: 실제 검색 후보 0개 / 대체 후보 3개 / 주문·결제 실행 0건",
		"브라우저 준비 상태:",
		"구매 경계 상태: 최종 주문/결제는 `구매 실행 승인` 전에는 실행하지 않습니다.",
		"실전 테스트 카드:",
		"준비됨: 검색 후보, 선택 기준",
		"Signal에 보낼 값: 상품명, 옵션, 수량, 총액, 도착 예정일, 배송지 표시: 맞음/아님/2개 보임, 결제수단 표시: 맞음/아님/2개 보임, 주문 버튼 위치.",
		"2개 이상 보이면 `쿠팡 배송지 2개 확인해`로 멈춤",
		"2개 이상 보이면 `쿠팡 결제수단 확인해`로 멈춤",
		"멈춤선: `구매 실행 승인` 전에는",
		"장바구니 직전 체크리스트:",
		"최종 화면에서 읽을 값:",
		"Signal로 보내면 바로 판정하는 값 템플릿:",
		"상품명: 화면 그대로",
		"총액: 최종 결제 금액",
		"최종 승인 메시지 초안:",
		"`구매 실행 승인`: 후보 1번, 총액 [화면 총액], 도착 [화면 도착 예정일], 배송지 표시 맞음, 결제수단 표시 맞음.",
		"2개 이상 보이면 승인 대신 확인 명령으로 멈춤.",
		"내일 실제 테스트 전 준비:",
		"배송지/결제수단 선택 게이트:",
		"배송지가 2개라면 구매 승인 전에 `쿠팡 배송지 2개 확인해`로 의도한 배송지 하나만 확정합니다.",
		"결제수단이 2개 이상이면 `쿠팡 결제수단 확인해`로 의도한 결제수단 하나만 확정합니다.",
		"주소/카드 원문은 Signal에 쓰지 않고 맞음/아님/2개 보임만 보냅니다.",
		"실구매 테스트 준비상태:",
		"로그인만으로 끝이 아니라",
		"비서는 검색, 후보 비교",
		"Mac mini 브라우저가 쿠팡에 로그인",
		"본인인증",
		"먼저 `쿠팡 배송지 2개 확인해`를 보내 배송지를 하나로 확정하세요.",
		"먼저 `쿠팡 결제수단 확인해`를 보내 결제수단을 하나로 확정하세요.",
		"구매 전 안전 체크 SVG",
		"edge-tts MP3 구매 전 브리핑",
		"구매 실행 승인",
		"모바일에서 바로 열기:",
		"DOCX 보고서: https://argos.example.test/argos/",
		"MP3 음성 브리핑: https://argos.example.test/argos/",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("visible checkout prep package missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{
		"Signal에 보낼 값: 상품명, 옵션, 수량, 총액, 배송지, 결제수단, 주문 버튼 위치.",
		"상품명, 옵션, 수량, 총액, 배송지, 결제수단, 주문 버튼 위치",
		"상품명과 옵션",
	} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("visible checkout prep package should use visibility labels, found %q:\n%s", unwanted, visible)
		}
	}
	for _, unwanted := range []string{"meshclaw-attachment:", "/.meshclaw/evidence/", "/Documents/Argos Vault/", "구매 완료", "주문 완료"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("visible reply should not expose or claim %q:\n%s", unwanted, visible)
		}
	}

	attachments := signalReplyAttachments(reply)
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg", ".mp3"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("checkout prep package missing %s attachment: %#v", ext, attachments)
		}
	}
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), ".html") {
			t.Fatalf("HTML should not be attached for mobile Signal package: %#v", attachments)
		}
	}

	markdown := firstAttachmentExt(attachments, ".md")
	data, err := os.ReadFile(markdown)
	if err != nil {
		t.Fatalf("read markdown attachment: %v", err)
	}
	content := string(data)
	for _, want := range []string{"# 쿠팡 구매 전 준비 카드", "구매 전 준비 상태: 실제 검색 후보 0개 / 대체 후보 3개 / 주문·결제 실행 0건", "구매 경계 상태: 최종 주문/결제는 `구매 실행 승인` 전에는 실행하지 않습니다.", "장바구니 직전 체크리스트", "최종 화면에서 읽을 값", "도착 예정일: 화면 그대로 복사", "Signal로 보내면 바로 판정하는 값 템플릿:", "상품명: 화면 그대로", "도착 예정일: 화면 그대로", "배송지 표시: 맞음/아님/2개 보임", "결제수단 표시: 맞음/아님/2개 보임", "주문 버튼: 보임/안 보임", "## 배송지/결제수단 선택 게이트:", "배송지가 2개라면 구매 승인 전에 `쿠팡 배송지 2개 확인해`로 의도한 배송지 하나만 확정합니다.", "결제수단이 2개 이상이면 `쿠팡 결제수단 확인해`로 의도한 결제수단 하나만 확정합니다.", "주소/카드 원문은 Signal에 쓰지 않고 맞음/아님/2개 보임만 보냅니다.", "## 최종 승인 메시지 초안", "`구매 실행 승인`: 후보 1번, 총액 [화면 총액], 도착 [화면 도착 예정일], 배송지 표시 맞음, 결제수단 표시 맞음.", "주소/카드 원문은 쓰지 않고", "내일 실제 테스트 전 준비:", "실구매 테스트 준비상태:", "로그인만으로 끝이 아니라", "Mac mini 브라우저가 쿠팡에 로그인", "먼저 `쿠팡 배송지 2개 확인해`를 보내 배송지를 하나로 확정하세요.", "먼저 `쿠팡 결제수단 확인해`를 보내 결제수단을 하나로 확정하세요.", "flowchart TD", "구매 실행 승인"} {
		if !strings.Contains(content, want) {
			t.Fatalf("markdown content missing %q:\n%s", want, content)
		}
	}
	csv := firstAttachmentExt(attachments, ".csv")
	csvData, err := os.ReadFile(csv)
	if err != nil {
		t.Fatalf("read csv attachment: %v", err)
	}
	if !strings.Contains(string(csvData), "상품명, 옵션, 수량, 총액, 도착 예정일, 배송지 표시 여부") {
		t.Fatalf("checkout prep csv should include arrival estimate in final-screen values:\n%s", string(csvData))
	}
	if !strings.Contains(string(csvData), "배송지/결제수단 선택 게이트") || !strings.Contains(string(csvData), "2개 이상 보이면 확인 명령으로 멈춤") {
		t.Fatalf("checkout prep csv should include address/payment choice gate:\n%s", string(csvData))
	}
	safety := firstAttachmentExt(attachments, ".svg")
	safetyData, err := os.ReadFile(safety)
	if err != nil {
		t.Fatalf("read safety SVG attachment: %v", err)
	}
	for _, want := range []string{"<svg", "쿠팡 구매 전 준비 카드", "브라우저 로그인", "상품명", "옵션", "수량", "도착 예정일", "배송지/결제수단 2개", "보임이면 중지", "최종 화면에서 반드시 읽을 값", "화면 값", "버튼 위치", "구매 실행 승인"} {
		if !strings.Contains(string(safetyData), want) {
			t.Fatalf("safety SVG missing %q:\n%s", want, string(safetyData))
		}
	}
}

func TestAssistantCoupangCheckoutPrepPackageEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, "targets.json"))
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_BASE_URL", "https://argos.example.test/argos")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_SERVER_DISABLED", "1")
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatalf("upsert briefing target: %v", err)
	}

	reply := assistantReply(
		ListenOptions{TargetID: "argos-assistant-checkout-prep-en", Mode: "assistant"},
		"Coupang checkout prep for bottled water 500ml 20 pack. Send it to the briefing room with a pre-cart checklist, mobile SVG safety card, PPT, CSV, and voice briefing. Do not pay.",
	)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Prepared the Coupang pre-purchase prep card for Signal delivery.",
		"Target: Briefing room",
		"Created a Coupang pre-purchase prep card.",
		"Query:",
		"Reference candidate: #1",
		"Pre-purchase prep status: 0 live search candidates / 3 fallback candidates / 0 order/payment actions executed",
		"Browser prep status:",
		"Purchase boundary status: final order/payment will not run before `purchase execution approved`.",
		"Live-test card:",
		"Ready: search candidates, selection criteria",
		"Send back in Signal: product, option, quantity, total, arrival estimate, Address visible: yes/no/two shipping addresses, Payment method visible: yes/no/two cards, and order-button location.",
		"If multiple addresses are visible, stop with `check Coupang two shipping addresses`",
		"If multiple methods are visible, stop with `check Coupang payment method`",
		"Stop line: add-to-cart, payment, and final order click do not run before `purchase execution approved`.",
		"Pre-cart checklist:",
		"Fields to read on the final screen:",
		"Signal value template for an immediate decision:",
		"Product: copy exactly as shown",
		"Total: final payment amount",
		"Final approval message draft:",
		"`purchase execution approved`: candidate #1, total [screen total], arrival [screen arrival estimate], address visible yes, payment method visible yes.",
		"if multiple choices are visible, stop with the verification command instead of approving.",
		"Before tomorrow's live test:",
		"Address/payment choice gate:",
		"If two shipping addresses are visible, confirm exactly one intended address with `check Coupang two shipping addresses` before purchase approval.",
		"If multiple payment methods are visible, confirm exactly one intended payment method with `check Coupang payment method`.",
		"Do not write raw address or card details into Signal; send only yes/no/two choices visible.",
		"Live-purchase test readiness:",
		"Login alone is not enough",
		"The assistant can search, compare candidates",
		"Confirm the Mac mini browser is logged in to Coupang",
		"identity check",
		"First send `check Coupang two shipping addresses` to confirm exactly one address.",
		"First send `check Coupang payment method` to confirm exactly one payment method.",
		"purchase execution approved",
		"Mobile-openable pre-purchase safety SVG",
		"edge-tts MP3 pre-purchase briefing",
		"Open on mobile:",
		"DOCX report: https://argos.example.test/argos/",
		"MP3 voice brief: https://argos.example.test/argos/",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("English visible checkout prep package missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{
		"Send back in Signal: product, option, quantity, total, shipping address, payment method, and order-button location.",
		"Product, option, quantity, total, address, payment method, and order button location",
		"Product name and options",
	} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("English visible checkout prep package should use visibility labels, found %q:\n%s", unwanted, visible)
		}
	}
	if containsHangul(visible) {
		t.Fatalf("English visible checkout prep package should not expose Korean text:\n%s", visible)
	}

	attachments := signalReplyAttachments(reply)
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg", ".mp3"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("English checkout prep package missing %s attachment: %#v", ext, attachments)
		}
	}
	for _, attachment := range attachments {
		lower := strings.ToLower(attachment)
		if !(strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".csv") || strings.HasSuffix(lower, ".svg")) {
			continue
		}
		data, err := os.ReadFile(attachment)
		if err != nil {
			t.Fatalf("read English checkout artifact %s: %v", attachment, err)
		}
		text := string(data)
		if containsHangul(text) {
			t.Fatalf("English checkout prep artifact should not expose Korean text in %s:\n%s", attachment, text)
		}
	}
	reportMarkdown := ""
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), ".md") {
			data, err := os.ReadFile(attachment)
			if err != nil {
				t.Fatalf("read English markdown candidate: %v", err)
			}
			if strings.Contains(string(data), "## Fields to read on the final screen:") || strings.Contains(string(data), "Purchase execution approved?") {
				reportMarkdown = string(data)
				break
			}
		}
	}
	if reportMarkdown == "" {
		t.Fatalf("English checkout prep should include localized report markdown: %#v", attachments)
	}
	for _, want := range []string{"Coupang pre-purchase prep card", "Pre-purchase prep status: 0 live search candidates / 3 fallback candidates / 0 order/payment actions executed", "Purchase boundary status: final order/payment will not run before `purchase execution approved`.", "Arrival estimate: copy exactly as shown on screen", "Signal value template for an immediate decision:", "Arrival estimate: copy exactly as shown", "Address visible: yes/no/two shipping addresses", "Payment method visible: yes/no/two cards", "Order button: visible/not visible", "## Address/payment choice gate:", "If two shipping addresses are visible, confirm exactly one intended address with `check Coupang two shipping addresses` before purchase approval.", "If multiple payment methods are visible, confirm exactly one intended payment method with `check Coupang payment method`.", "Do not write raw address or card details into Signal; send only yes/no/two choices visible.", "## Final Approval Message Draft", "`purchase execution approved`: candidate #1, total [screen total], arrival [screen arrival estimate], address visible yes, payment method visible yes.", "Do not include raw address/card details", "Live-purchase test readiness:", "Login alone is not enough", "First send `check Coupang two shipping addresses` to confirm exactly one address.", "First send `check Coupang payment method` to confirm exactly one payment method.", "flowchart TD", "Purchase execution approved?", "purchase execution approved", "Add-to-cart, buy-now, payment, and final order were not run automatically"} {
		if !strings.Contains(reportMarkdown, want) {
			t.Fatalf("English checkout prep markdown missing %q:\n%s", want, reportMarkdown)
		}
	}
	englishCSV := firstAttachmentExt(attachments, ".csv")
	englishCSVData, err := os.ReadFile(englishCSV)
	if err != nil {
		t.Fatalf("read English checkout csv: %v", err)
	}
	if !strings.Contains(string(englishCSVData), "Product, option, quantity, total, arrival estimate, address visibility") {
		t.Fatalf("English checkout prep csv should include arrival estimate in final-screen values:\n%s", string(englishCSVData))
	}
	if !strings.Contains(string(englishCSVData), "Address/payment choice gate") || !strings.Contains(string(englishCSVData), "If multiple choices are visible, stop with the verification command") {
		t.Fatalf("English checkout prep csv should include address/payment choice gate:\n%s", string(englishCSVData))
	}
	englishSafety := firstAttachmentExt(attachments, ".svg")
	englishSafetyData, err := os.ReadFile(englishSafety)
	if err != nil {
		t.Fatalf("read English checkout safety svg: %v", err)
	}
	for _, want := range []string{"<svg", "Product name", "Option", "Quantity", "Arrival estimate", "Values to read on the final screen", "screen values", "button location", "purchase execution approved"} {
		if !strings.Contains(string(englishSafetyData), want) {
			t.Fatalf("English checkout prep safety SVG missing %q:\n%s", want, string(englishSafetyData))
		}
	}
}

func TestAssistantCoupangCheckoutPrepVoiceChoiceGateUsesLanguagePack(t *testing.T) {
	tests := []struct {
		locale string
		want   []string
		bad    []string
	}{
		{
			locale: "ko",
			want: []string{
				"배송지가 두 개라면 의도한 배송지 하나를 먼저 확정",
				"결제수단이 여러 개라면 의도한 결제수단 하나를 먼저 확정",
				"주소와 카드 원문은 시그널에 쓰지 않습니다",
			},
			bad: []string{"two shipping addresses", "raw address"},
		},
		{
			locale: "en",
			want: []string{
				"If two shipping addresses are visible, confirm exactly one intended address first.",
				"If multiple payment methods are visible, confirm exactly one intended payment method first.",
				"Do not put raw address or card details into Signal.",
			},
			bad: []string{"배송지", "결제수단"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.locale, func(t *testing.T) {
			t.Setenv("MESHCLAW_LANG", tt.locale)
			voice := assistantCoupangCheckoutPrepVoiceScript("bottled water 500ml 20 pack", browserauto.Link{}, 0, true, 0, 3, "")
			for _, want := range tt.want {
				if !strings.Contains(voice, want) {
					t.Fatalf("checkout prep voice missing %q:\n%s", want, voice)
				}
			}
			for _, bad := range tt.bad {
				if strings.Contains(voice, bad) {
					t.Fatalf("checkout prep voice should not expose other locale %q:\n%s", bad, voice)
				}
			}
		})
	}
}

func legacyTestAssistantCoupangCheckoutPrepPackageWinsOverFinalScreenReview(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, "targets.json"))
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatalf("upsert briefing target: %v", err)
	}
	input := "내일 쿠팡에서 생수 500ml 20개 실제 구매 테스트를 할 거니까 장바구니 직전 체크리스트, 최종 화면에서 읽을 값, 모바일 SVG 안전 카드, PPT, CSV, edge tts 음성 브리핑으로 준비해서 보고방에 보내줘. 결제는 하지 마."
	if got := assistantCoupangCheckoutPrepPackageQuery(input); got != "생수 500ml 20개" {
		t.Fatalf("checkout prep query should remove final-screen and voice instructions, got %q", got)
	}

	reply := assistantReply(
		ListenOptions{TargetID: "argos-assistant-checkout-prep-final-fields", Mode: "assistant"},
		input,
	)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"쿠팡 구매 전 준비 카드를 Signal로 보낼 준비를 했습니다.",
		"실전 테스트 카드:",
		"최종 화면에서 읽을 값:",
		"구매 전 안전 체크 SVG",
		"edge-tts MP3 구매 전 브리핑",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("checkout prep package should win over final-screen review, missing %q:\n%s", want, visible)
		}
	}
	if strings.Contains(visible, "쿠팡 최종 화면 확인 단계") || strings.Contains(visible, "장바구니/결제 전 확인 화면을 열었습니다") {
		t.Fatalf("checkout prep package request was hijacked by final-screen review:\n%s", visible)
	}
}

func TestAssistantCoupangCheckoutPrepPackageExecuteSendsToBriefingWithArtifacts(t *testing.T) {
	home := t.TempDir()
	signalArgs := filepath.Join(home, "signal-args.txt")
	fakeSignal := filepath.Join(home, "signal-cli")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" > %q\nprintf 'checkout-signal-789\\n'\n", signalArgs)
	if err := os.WriteFile(fakeSignal, []byte(script), 0700); err != nil {
		t.Fatalf("write fake signal-cli: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))
	t.Setenv("MESHCLAW_SIGNAL_CLI", fakeSignal)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, "targets.json"))
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_BASE_URL", "https://argos.example.test/argos")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_SERVER_DISABLED", "1")
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatalf("upsert briefing target: %v", err)
	}

	reply := assistantReply(
		ListenOptions{TargetID: "argos-assistant-checkout-prep-execute-en", Mode: "assistant", Execute: true},
		"Coupang live purchase rehearsal for bottled water 500ml 20 pack. Send a checklist, PPT, CSV, mobile SVG safety card, and voice briefing to the briefing room. Do not pay.",
	)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Sent the Coupang pre-purchase prep card to Signal.",
		"Target: Briefing room",
		"Signal ID: checkout-signal-789",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("execute checkout prep reply missing %q:\n%s", want, visible)
		}
	}
	if containsHangul(visible) {
		t.Fatalf("English execute checkout prep reply should not expose Korean:\n%s", visible)
	}
	for _, unwanted := range []string{"meshclaw-attachment:", "/Documents/Argos Vault/", "/.meshclaw/evidence/", "order completed", "purchase completed"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("execute checkout prep reply should not expose or claim %q:\n%s", unwanted, visible)
		}
	}
	argsData, err := os.ReadFile(signalArgs)
	if err != nil {
		t.Fatalf("read fake signal args: %v", err)
	}
	args := string(argsData)
	for _, want := range []string{"send", "-g", "group-briefing", "--attachment", ".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg", ".mp3", "Open on mobile:", "DOCX report: https://argos.example.test/argos/", "PPTX deck: https://argos.example.test/argos/", "XLSX table: https://argos.example.test/argos/", "SVG chart: https://argos.example.test/argos/", "MP3 voice brief: https://argos.example.test/argos/"} {
		if !strings.Contains(args, want) {
			t.Fatalf("fake signal args missing %q:\n%s", want, args)
		}
	}
	for _, unwanted := range []string{".html", "purchase completed", "order completed"} {
		if strings.Contains(args, unwanted) {
			t.Fatalf("fake signal args should not expose or claim %q:\n%s", unwanted, args)
		}
	}
}

func TestAssistantCoupangCheckoutPrepPackageRouting(t *testing.T) {
	if !isAssistantCoupangCheckoutPrepPackageRequest("쿠팡에서 생수 500ml 20개를 1번 후보 기준으로 장바구니 직전까지 준비해서 보고방에 보내줘. 결제는 하지 마. 음성으로도 준비해줘.") {
		t.Fatal("expected checkout prep package request to route")
	}
	if !isAssistantCoupangCheckoutPrepPackageRequest("내일 쿠팡에서 생수 500ml 20개 진짜 구매 테스트를 할 거니까 보고방에 체크리스트와 PPT와 음성으로 준비해줘. 결제는 하지 마.") {
		t.Fatal("expected natural live-purchase rehearsal request to route to checkout prep")
	}
	if got := assistantCoupangCheckoutPrepPackageQuery("내일 쿠팡에서 생수 500ml 20개 진짜 구매 테스트를 할 거니까 보고방에 체크리스트와 PPT와 음성으로 준비해줘. 결제는 하지 마."); got != "생수 500ml 20개" {
		t.Fatalf("live-purchase rehearsal query should keep only product conditions, got %q", got)
	}
	if got := assistantCoupangCheckoutPrepPackageQuery("내일 쿠팡에서 생수 500ml 20개 실제 구매 테스트를 할 거니까 비서방에 장바구니 직전 체크리스트와 PPT, CSV, 모바일 SVG 안전 카드, 음성으로 준비해줘. 결제는 하지 마."); got != "생수 500ml 20개" {
		t.Fatalf("live-purchase mobile safety-card query should keep only product conditions, got %q", got)
	}
	if !isAssistantCoupangCheckoutPrepPackageRequest("Coupang live purchase rehearsal for bottled water 500ml 20 pack. Send a checklist, PPT, and voice briefing to the briefing room. Do not pay.") {
		t.Fatal("expected English live purchase rehearsal request to route to checkout prep")
	}
	if got := assistantCoupangCheckoutPrepPackageQuery("Coupang live purchase rehearsal for bottled water 500ml 20 pack. Send a checklist, PPT, and voice briefing to the briefing room. Do not pay."); got != "bottled water 500ml 20 pack" {
		t.Fatalf("English live-purchase rehearsal query should keep only product conditions, got %q", got)
	}
	if isAssistantCoupangCheckoutPrepPackageRequest("오늘 제품/마케팅 회의 메모입니다. 결정: 내일 쿠팡 실구매 테스트는 결제 전 확인 단계까지 진행합니다. 할 일: 김팀장은 예산안을 작성합니다. 이걸 회의록, 액션아이템 표, PPT, Mermaid 흐름도, SVG 액션보드, edge tts 음성 브리핑으로 만들어서 보고방에 보내줘.") {
		t.Fatal("checkout prep package must not steal meeting-minutes delivery requests that mention Coupang as one action item")
	}
	for _, input := range []string{
		"쿠팡에서 생수 500ml 20개 로켓배송 가격 리뷰 좋은 후보 3개 비교해서 표와 PPT로 준비해줘",
		"첫 번째 상품 상세 열고 총액 확인해줘",
		"1번 상품을 장바구니 직전 화면까지 준비해. 결제는 하지 마",
		"최종 화면의 상품명, 옵션, 총액, 배송지, 결제수단을 읽어줘",
		"구매 실행 승인",
	} {
		if isAssistantCoupangCheckoutPrepPackageRequest(input) {
			t.Fatalf("checkout prep package should not steal regular follow-up request: %s", input)
		}
	}
}
