package messenger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meshclaw/meshclaw/internal/browserauto"
	"github.com/meshclaw/meshclaw/internal/publish"
)

func TestAssistantCalendarTravelPackageCreatesMobileArtifacts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_ASSISTANT_RESEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_BASE_URL", "https://argos.example.test/argos")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_SERVER_DISABLED", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-travel-calendar", Mode: "assistant"}, "일주일 동안 내가 가장 시간이 많이 나는 시기는 언제인지 확인하고 제주 여행계획을 잡아줘. 비행기표와 호텔 후보를 표와 PPT와 음성으로 준비해줘.")
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"제주 가용시간 기반 여행계획 패키지",
		"캘린더 가용시간",
		"항공/호텔 검색 쿼리:",
		"캘린더 확인 상태:",
		"항공/호텔 검색 상태: 검색 후보 0개 / 원문 읽음 0개 / 공식 후보 0개 / 읽기 실패 0개",
		"항공/호텔 검색 실행 상태: 실제 웹 검색 비활성화",
		"예약 전 결정표:",
		"항공 후보:",
		"호텔 후보:",
		"항공 후보 링크:",
		"호텔 후보 링크:",
		"탑승자/투숙자 실명",
		"해외 여권/영문명",
		"수하물·추가요금",
		"취소·환불 조건",
		"여행계획 미리보기",
		"동행자 공유 초안:",
		"제주 여행 후보를 캘린더 빈 시간 기준으로 잡아봤어요.",
		"최종 예약/결제는 다시 승인받고 진행할게요.",
		"예약/결제는 최종 승인 전 중지",
		"SVG 여행 타임라인",
		"edge-tts MP3 음성파일",
		"모바일에서 바로 열기:",
		"DOCX 보고서: https://argos.example.test/argos/",
		"MP3 음성 브리핑: https://argos.example.test/argos/",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("visible travel reply missing %q:\n%s", want, visible)
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
			t.Fatalf("travel package missing %s attachment: %#v", ext, attachments)
		}
	}
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), ".html") {
			t.Fatalf("HTML should not be attached for mobile Signal package: %#v", attachments)
		}
	}
	timeline := firstAttachmentExt(attachments, ".svg")
	data, err := os.ReadFile(timeline)
	if err != nil {
		t.Fatalf("read travel timeline SVG: %v", err)
	}
	for _, want := range []string{"<svg", "제주 가용시간 기반 여행계획 패키지", "예약 전 확인", "실명/해외 여권명", "수하물·좌석요금"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("travel timeline SVG missing %q:\n%s", want, string(data))
		}
	}
	markdown := firstAttachmentExt(attachments, ".md")
	data, err = os.ReadFile(markdown)
	if err != nil {
		t.Fatalf("read travel Markdown: %v", err)
	}
	for _, want := range []string{"탑승자/투숙자 실명", "해외 여권/영문명", "수하물/좌석/세금/리조트피", "취소·환불 마감/조건", "## 동행자 공유 초안", "최종 예약/결제는 다시 승인받고 진행할게요."} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("travel Markdown missing concrete booking check %q:\n%s", want, string(data))
		}
	}
}

func TestAssistantCalendarTravelPackageRouting(t *testing.T) {
	if !isAssistantCalendarTravelPackageRequest("일주일 동안 내가 가장 시간이 많이 나는 시기는 언제인지 확인하고 여행계획을 잡아줘. 비행기표와 호텔 후보도 같이 봐줘.") {
		t.Fatal("expected availability-based travel request to route to calendar travel package")
	}
	if !isAssistantCalendarTravelPackageRequest("prepare a travel plan for Osaka next week with flight and hotel candidates, itinerary table, PPT, and voice briefing") {
		t.Fatal("expected English availability-based travel request to route to calendar travel package")
	}
	if isAssistantCalendarTravelPackageRequest("후쿠오카 2박3일 여행 준비물, 예산, 동선 PPT로 만들어줘") {
		t.Fatal("plain prep bundle without calendar availability should stay on department workflow")
	}
}

func TestAssistantCalendarTravelPackageEnglishCanSendToBriefing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_ASSISTANT_RESEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, "targets.json"))
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_BASE_URL", "https://argos.example.test/argos")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_SERVER_DISABLED", "1")
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatalf("upsert briefing target: %v", err)
	}

	reply := assistantReply(
		ListenOptions{TargetID: "argos-assistant-travel-send-en", Mode: "assistant"},
		"prepare a travel plan for Osaka next week with flight and hotel candidates, itinerary table, PPT, and voice briefing. send to briefing room. do not book or pay.",
	)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Prepared the travel plan package for Signal delivery.",
		"Target: Briefing room",
		"The briefing room is one-way/no-reply",
		"Osaka calendar-based travel planning package",
		"Flight/hotel search query: Seoul Osaka flights hotels",
		"Calendar status:",
		"Flight/hotel search status: 0 search candidates / 0 pages read / 0 official candidates / 0 read failures",
		"Flight/hotel research execution: live web search is disabled",
		"Pre-booking decision table:",
		"Flight candidates:",
		"Hotel candidates:",
		"Flight candidate links:",
		"Hotel candidate links:",
		"traveler/guest legal names",
		"passport/English name when international",
		"baggage/extra fees",
		"cancellation/refund terms",
		"Companion share draft:",
		"I drafted Osaka travel candidates around the open calendar windows.",
		"final booking/payment will wait for explicit approval",
		"Mobile-openable SVG travel timeline",
		"edge-tts MP3 audio reading the travel plan",
		"Stop before final booking/payment unless explicit approval is present",
		"Useful follow-up commands:",
		"Open on mobile:",
		"DOCX report: https://argos.example.test/argos/",
		"MP3 voice brief: https://argos.example.test/argos/",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("English travel send preview missing %q:\n%s", want, visible)
		}
	}
	if containsHangul(visible) {
		t.Fatalf("English travel send preview should not expose Korean text:\n%s", visible)
	}
	for _, unwanted := range []string{"meshclaw-attachment:", "/.meshclaw/evidence/", "/Documents/Argos Vault/"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("visible reply should not expose raw path %q:\n%s", unwanted, visible)
		}
	}

	attachments := signalReplyAttachments(reply)
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg", ".mp3"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("English travel send preview missing %s attachment: %#v", ext, attachments)
		}
	}
	companionDraftSeen := false
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
			t.Fatalf("read English travel artifact %s: %v", attachment, err)
		}
		if containsHangul(string(data)) {
			t.Fatalf("English travel artifact should not expose Korean text in %s:\n%s", attachment, string(data))
		}
		if strings.HasSuffix(lower, ".md") {
			if strings.Contains(string(data), "## Companion Share Draft") && strings.Contains(string(data), "final booking/payment will wait for explicit approval") {
				companionDraftSeen = true
			}
			for _, want := range []string{"traveler/guest legal names", "passport/English name when international", "baggage/extra fees", "cancellation/refund deadline"} {
				if !strings.Contains(string(data), want) {
					t.Fatalf("English travel Markdown missing concrete booking check %q:\n%s", want, string(data))
				}
			}
		}
	}
	if !companionDraftSeen {
		t.Fatalf("English travel Markdown attachments should include companion share draft: %#v", attachments)
	}
}

func TestAssistantCalendarTravelPackageExecuteSendsMobileLinks(t *testing.T) {
	home := t.TempDir()
	signalArgs := filepath.Join(home, "signal-args.txt")
	fakeSignal := filepath.Join(home, "signal-cli")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" > %q\nprintf 'travel-signal-654\\n'\n", signalArgs)
	if err := os.WriteFile(fakeSignal, []byte(script), 0700); err != nil {
		t.Fatalf("write fake signal-cli: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_ASSISTANT_RESEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))
	t.Setenv("MESHCLAW_SIGNAL_CLI", fakeSignal)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, "targets.json"))
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_BASE_URL", "https://argos.example.test/argos")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_HOST", "argos.example.test")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_SERVER_DISABLED", "1")
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatalf("upsert briefing target: %v", err)
	}

	reply := assistantReply(
		ListenOptions{TargetID: "argos-assistant-travel-execute-en", Mode: "assistant", Execute: true},
		"prepare a travel plan for Osaka next week with flight and hotel candidates, itinerary table, PPT, and voice briefing. send to briefing room. do not book or pay.",
	)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Sent the travel plan package to Signal.",
		"Target: Briefing room",
		"Signal ID: travel-signal-654",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("execute travel reply missing %q:\n%s", want, visible)
		}
	}
	if containsHangul(visible) {
		t.Fatalf("English execute travel reply should not expose Korean:\n%s", visible)
	}

	argsData, err := os.ReadFile(signalArgs)
	if err != nil {
		t.Fatalf("read fake signal args: %v", err)
	}
	args := string(argsData)
	for _, want := range []string{
		"Open on mobile:",
		"DOCX report: https://argos.example.test/argos/",
		"PPTX deck: https://argos.example.test/argos/",
		"XLSX table: https://argos.example.test/argos/",
		"SVG chart: https://argos.example.test/argos/",
		"MP3 voice brief: https://argos.example.test/argos/",
		"--attachment",
		".docx",
		".pptx",
		".xlsx",
		".svg",
		".mp3",
	} {
		if !strings.Contains(args, want) {
			t.Fatalf("fake signal args missing %q:\n%s", want, args)
		}
	}
	if strings.Contains(args, ".html") {
		t.Fatalf("mobile Signal package should not attach HTML:\n%s", args)
	}
}

func TestAssistantTravelAvailabilityUsesCalendarBusyTime(t *testing.T) {
	loc := time.FixedZone("KST", 9*60*60)
	now := time.Date(2026, 6, 15, 7, 0, 0, 0, loc)
	calendar := strings.Join([]string{
		"일정을 확인했습니다.",
		"일정 2개:",
		"1. 월간회의 | 2026-06-15 09:00 ~ 12:00",
		"2. 외부 미팅 | 2026-06-16 08:00 ~ 18:00 | 강남",
	}, "\n")
	lines := assistantTravelAvailabilityCandidateLinesAt(calendar, now, 3)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "실제 캘린더 기준 빈 시간 후보") {
		t.Fatalf("availability title missing:\n%s", joined)
	}
	if strings.Contains(joined, "6월 16일") {
		t.Fatalf("heavily booked day should not be a top travel window:\n%s", joined)
	}
	if !strings.Contains(joined, "약 14시간 비어 있음") {
		t.Fatalf("expected full open-day duration:\n%s", joined)
	}
	events := parseAssistantTravelCalendarVisibleEvents(calendar)
	if len(events) != 2 || events[1].Title != "외부 미팅" {
		t.Fatalf("events=%#v", events)
	}
}

func TestAssistantTravelCandidateSourceLinesSplitFlightAndHotel(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	report := publish.ResearchReport{Search: browserauto.SearchResult{Results: []browserauto.Link{
		{Text: "Booking.com | Official site | The best hotels, flights, car rentals", URL: "https://www.booking.com/"},
		{Text: "Search Flights, Hotels & Rental Cars | KAYAK", URL: "https://www.kayak.com/"},
		{Text: "Agoda Official Site | Free Cancellation & Booking Deals", URL: "https://www.agoda.com/"},
		{Text: "항공권 예약 및 여행 정보 | 대한항공", URL: "https://www.koreanair.com/"},
		{Text: "제주 호텔 특가", URL: "https://example.com/jeju-hotel"},
	}}}
	flights := strings.Join(assistantTravelCandidateSourceLines(report, "flight", 5), "\n")
	hotels := strings.Join(assistantTravelCandidateSourceLines(report, "hotel", 5), "\n")
	if !strings.Contains(flights, "KAYAK") || !strings.Contains(flights, "대한항공") {
		t.Fatalf("flight links not split correctly:\n%s", flights)
	}
	if !strings.Contains(hotels, "Agoda") || !strings.Contains(hotels, "제주 호텔") {
		t.Fatalf("hotel links not split correctly:\n%s", hotels)
	}
	if strings.Contains(flights, "Booking.com") {
		t.Fatalf("flight links should not include hotel-primary source:\n%s", flights)
	}
	if strings.Contains(hotels, "대한항공") {
		t.Fatalf("hotel links should not include airline-only source:\n%s", hotels)
	}
}
