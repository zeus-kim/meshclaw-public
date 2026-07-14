package messenger

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/meshclaw/meshclaw/internal/assistantbrief"
	"github.com/meshclaw/meshclaw/internal/assistantprofile"
	"github.com/meshclaw/meshclaw/internal/browserauto"
	"github.com/meshclaw/meshclaw/internal/evidence"
	"github.com/meshclaw/meshclaw/internal/mailadapter"
	"github.com/meshclaw/meshclaw/internal/newsbrief"
	"github.com/meshclaw/meshclaw/internal/osauto"
)

func TestAssistantScenarioMapLinkIsFastAndClickable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := mapsScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "광화문 교보문고 지도 보여줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "https://www.google.com/maps/search/") ||
		!strings.Contains(reply, "query=") ||
		!strings.Contains(reply, "지도 화면 캡처") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestSignalCapabilityReplyAssistant(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	reply, handled := signalCapabilityReply("assistant", "어떤 일을 할 수 있지?")
	if !handled {
		t.Fatal("capability question was not handled")
	}
	for _, want := range []string{"저는 Argos", "Mac mini", "MeshClaw", "제약회사 최신 뉴스", "DART/SEC", "최근 메일 요약", "오늘 일정", "장기기억", "작업 재사용", "스킬", "DOCX/PPTX", "승인 필요"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "其中") || strings.Contains(reply, "用户") {
		t.Fatalf("reply should not be model-generated mixed language:\n%s", reply)
	}
}

func TestSignalCapabilityReplyAssistantEnglishUsesLanguagePack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "en")
	reply, handled := signalCapabilityReply("assistant", "what can you do?")
	if !handled {
		t.Fatal("capability question was not handled")
	}
	for _, want := range []string{
		"I am Argos",
		"Mac mini",
		"MeshClaw",
		"pharma/biotech",
		"DART/SEC",
		"recent mail",
		"daily plan",
		"long-term memory",
		"Work reuse",
		"reusable skill",
		"DOCX/PPTX",
		"Approval required",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English capability reply missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English capability reply should not expose Korean:\n%s", reply)
	}
}

func TestAssistantCapabilitiesReplyShowsApprovedPersonalization(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	if _, err := assistantprofile.AddMemory(time.Now(), "", "user", "사용자는 결론부터 짧게 답하는 것을 선호한다", true); err != nil {
		t.Fatal(err)
	}
	if _, err := assistantprofile.InstallSkill(time.Now(), "", "Short Brief", "Summarize first and keep irreversible actions gated.", true); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-capability-personalized", Mode: "assistant"}, "뭘 할 수 있어?")
	for _, want := range []string{
		"저는 Argos",
		"현재 개인화:",
		"기억: 사용자는 결론부터 짧게 답하는 것을 선호한다",
		"활성 스킬: Short Brief",
		"개인화는 답변 맥락일 뿐 권한이 아닙니다.",
		"승인 필요",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("personalized capability reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "발송했습니다", "예약 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("capability reply should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantCapabilitiesReplyEnglishShowsApprovedPersonalization(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	if _, err := assistantprofile.AddMemory(time.Now(), "", "user", "User prefers concise answers with the conclusion first.", true); err != nil {
		t.Fatal(err)
	}
	if _, err := assistantprofile.InstallSkill(time.Now(), "", "Short Brief", "Summarize first and keep irreversible actions gated.", true); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-capability-personalized-en", Mode: "assistant"}, "what can you do?")
	for _, want := range []string{
		"I am Argos",
		"Current personalization:",
		"Memory: User prefers concise answers with the conclusion first.",
		"Active skill: Short Brief",
		"Personalization is context, not permission.",
		"Approval required",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English personalized capability reply missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English personalized capability reply should not expose Korean:\n%s", reply)
	}
}

func TestAssistantConversationalReplyShowsApprovedPersonalization(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	if _, err := assistantprofile.AddMemory(time.Now(), "", "user", "사용자는 결론부터 짧게 답하는 것을 선호한다", true); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-conversation-personalized", Mode: "assistant"}, "고마워")
	for _, want := range []string{
		"네. 이어서 바로 시키면 됩니다.",
		"개인화 반영: 사용자는 결론부터 짧게 답하는 것을 선호한다",
		"개인화는 답변 맥락일 뿐 권한이 아닙니다.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("personalized conversation reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "발송했습니다", "예약 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("conversation reply should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantConversationalReplyEnglishUsesLanguagePackAndMemory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	if _, err := assistantprofile.AddMemory(time.Now(), "", "user", "User prefers concise answers with the conclusion first.", true); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-conversation-personalized-en", Mode: "assistant"}, "thanks")
	for _, want := range []string{
		"Yes. Send the next task directly.",
		"Personalization applied: User prefers concise answers with the conclusion first.",
		"Personalization is context, not permission.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English personalized conversation reply missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English personalized conversation reply should not expose Korean:\n%s", reply)
	}
}

func TestSignalIdentityQuestionReplyAssistant(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	reply, handled := signalCapabilityReply("assistant", "넌 누구지? 무엇을 할 수 있지?")
	if !handled {
		t.Fatal("identity question was not handled")
	}
	for _, want := range []string{"저는 Argos", "Mac mini", "Signal", "MeshClaw", "승인"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("identity reply missing %q:\n%s", want, reply)
		}
	}
}

func TestSignalIdentityQuestionReplyNaturalAssistantWording(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	reply, handled := signalCapabilityReply("assistant", "넌 누구고 뭘 할 수 있어?")
	if !handled {
		t.Fatal("identity question was not handled")
	}
	for _, want := range []string{"저는 Argos", "Mac mini", "Signal", "바로 해볼 예시", "승인"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("identity reply missing %q:\n%s", want, reply)
		}
	}
}

func TestSignalCapabilityReplyAssistantQuickExampleTriggers(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, input := range []string{"뭘 할 수 있어", "기능 보여줘", "빠른 예시", "바로 해볼 명령"} {
		reply, handled := signalCapabilityReply("assistant", input)
		if !handled {
			t.Fatalf("%q was not handled", input)
		}
		lines := strings.Split(reply, "\n")
		if len(lines) < 8 || len(lines) > 12 {
			t.Fatalf("reply should stay mobile-sized, got %d lines:\n%s", len(lines), reply)
		}
		for _, want := range []string{"제약회사 최신 뉴스", "DART/SEC", "매일 오전 8시", "보고방", "최근 메일 요약해줘", "오늘 일정", "장기기억", "작업 재사용", "스킬", "DOCX/PPTX", "메일 발송, 예약 등록, 외부 공유, 삭제"} {
			if !strings.Contains(reply, want) {
				t.Fatalf("reply for %q missing %q:\n%s", input, want, reply)
			}
		}
	}
}

func TestAssistantStarterRecommendationReplyIsResultFocused(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	for _, input := range []string{"뭐부터 해볼까?", "다음 뭐 하면 좋아?", "추천해줘"} {
		reply := assistantReply(ListenOptions{TargetID: "argos-assistant-starter", Mode: "assistant"}, input)
		for _, want := range []string{"그대로 보내세요", "최근 메일 요약", "방송 원고", "회의자료", "업종별", "마케팅/광고/인사/관공서", "DOCX/PPTX/XLSX/SVG", "음성 브리핑", "화학회사", "제주 2박 3일", "하루 계획", "보안 위험", "Markdown/DOCX", "XLSX", "음성파일"} {
			if !strings.Contains(reply, want) {
				t.Fatalf("starter reply for %q missing %q:\n%s", input, want, reply)
			}
		}
		for _, bad := range []string{"원하는 번호", "1.", "2.", "3.", "4."} {
			if strings.Contains(reply, bad) {
				t.Fatalf("starter reply should not ask for number selection %q:\n%s", bad, reply)
			}
		}
	}
}

func TestAssistantStarterRecommendationReplyEnglishUsesLanguagePack(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "en")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-starter-en", Mode: "assistant"}, "what should I try first")
	for _, want := range []string{
		"send one of these sentences as-is",
		"Summarize recent mail",
		"broadcast script",
		"Markdown and DOCX",
		"marketing/advertising/HR/public-agency",
		"virtual company workflow scenario package",
		"DOCX, PPTX, XLSX, SVG charts, and a voice briefing",
		"chemical companies",
		"Jeju two-night three-day",
		"daily plan",
		"security risks",
		"Signal message, Markdown/DOCX, XLSX, and a voice file",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English starter reply missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English starter reply should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"원하는 번호", "1.", "2.", "3.", "4."} {
		if strings.Contains(reply, bad) {
			t.Fatalf("English starter reply should not ask for number selection %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantStarterRecommendationReplyShowsApprovedMemoryHint(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	if _, err := assistantprofile.AddMemory(time.Now(), "", "user", "사용자는 결론부터 짧게 답하는 것을 선호한다", true); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-starter-memory", Mode: "assistant"}, "뭐부터 해볼까?")
	for _, want := range []string{
		"그대로 보내세요",
		"개인화 반영: 사용자는 결론부터 짧게 답하는 것을 선호한다",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("starter reply with memory missing %q:\n%s", want, reply)
		}
	}
}

func TestAssistantStarterRecommendationReplyEnglishSkipsKoreanMemoryHint(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	if _, err := assistantprofile.AddMemory(time.Now(), "", "user", "사용자는 결론부터 짧게 답하는 것을 선호한다", true); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-starter-memory-en", Mode: "assistant"}, "what should I try first")
	if strings.Contains(reply, "Personalization applied:") {
		t.Fatalf("English starter should not expose Korean memory hint:\n%s", reply)
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English starter reply should not expose Korean:\n%s", reply)
	}
}

func TestAssistantHomeReplyRoutesOpenClawHermesAssistantRequest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	if _, err := assistantprofile.AddMemory(time.Now(), "", "user", "사용자는 결론부터 짧게 답하는 것을 선호한다", true); err != nil {
		t.Fatal(err)
	}
	input := "오픈클로 헤르메스 같은 비서 만들어"
	if !isAssistantHomeRequest(strings.ToLower(input)) {
		t.Fatal("assistant home request was not detected")
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-home", Mode: "assistant"}, input)
	for _, want := range []string{
		"Argos 비서 홈입니다.",
		"OpenClaw/Hermes처럼",
		"구매/결제 자동화는 제외",
		"작업 팔레트:",
		"메일:",
		"하루 계획:",
		"회의:",
		"리서치:",
		"업무 패키지:",
		"음성:",
		"메모리:",
		"스킬:",
		"현재 개인화:",
		"바로 보낼 문장:",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("assistant home reply missing %q:\n%s", want, reply)
		}
	}
	for _, unwanted := range []string{"휴지", "쿠팡", "결제 클릭", "구매 실행"} {
		if strings.Contains(reply, unwanted) {
			t.Fatalf("assistant home should not advertise purchase automation %q:\n%s", unwanted, reply)
		}
	}
}

func TestAssistantHomeReplyEnglishUsesLanguagePack(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "en")
	input := "make an openclaw-like assistant"
	if !isAssistantHomeRequest(strings.ToLower(input)) {
		t.Fatal("English assistant home request was not detected")
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-home-en", Mode: "assistant"}, input)
	for _, want := range []string{
		"Argos assistant home.",
		"OpenClaw/Hermes-style personal assistant mode",
		"Purchasing/payment automation is excluded",
		"Work palette:",
		"Mail:",
		"Daily plan:",
		"Meetings:",
		"Research:",
		"Work packages:",
		"Voice:",
		"Memory:",
		"Skills:",
		"Send one now:",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English assistant home reply missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English assistant home reply should not expose Korean:\n%s", reply)
	}
}

func TestAssistantHomeDoesNotStealOpenClawHermesBrowserSearch(t *testing.T) {
	input := "브라우저에서 OpenClaw Hermes 비교 검색해줘"
	if isAssistantHomeRequest(strings.ToLower(input)) {
		t.Fatal("browser search should not be treated as assistant home")
	}
}

func TestAssistantHomeReplyWinsForShortAssistantMode(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-home-short", Mode: "assistant"}, "비서 모드")
	for _, want := range []string{"Argos 비서 홈입니다.", "작업 팔레트:", "바로 보낼 문장:"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("short assistant mode reply missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "비서 모드 활성화했습니다") {
		t.Fatalf("short assistant mode should not use legacy short activation reply:\n%s", reply)
	}
}

func TestAssistantAutomationStatusReplyIsReadOnlyAndMobileSized(t *testing.T) {
	home := t.TempDir()
	schedulePath := filepath.Join(home, ".meshclaw", "schedule-state.json")
	if err := os.MkdirAll(filepath.Dir(schedulePath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(schedulePath, []byte(`{
  "kind": "meshclaw_schedule_state",
  "last_run": {
    "mail-watch": "2026-06-12T14:00:00Z",
    "assistant-auto-check": "2026-06-12T14:03:00Z"
  }
}`), 0600); err != nil {
		t.Fatal(err)
	}
	devWorkerPath := filepath.Join(home, "dev-worker-latest.json")
	if err := os.WriteFile(devWorkerPath, []byte(`{
  "ok": true,
  "stamp_utc": "20260612T140344Z",
  "raw_profileless_mcp": {"post_count": 0, "raw_profileless_mcp_count": 0},
  "local_lite_smoke": {"tool_count": 25}
}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MESHCLAW_SCHEDULE_STATE", schedulePath)
	t.Setenv("MESHCLAW_DEV_WORKER_LATEST_JSON", devWorkerPath)

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "자동화 돌아가?")
	for _, want := range []string{"Argos 자동화 상태", "Mac mini schedule", "due=", "비서 기능 점검 (assistant-auto-check)", "next=새 메일 확인 (mail-watch)", "in now", "Codex dev-worker", "PASS", "raw MCP=0", "local-lite tools=25", "읽기 전용"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("automation status reply missing %q:\n%s", want, reply)
		}
	}
	for _, blocked := range []string{"재시작했습니다", "배포했습니다", "삭제했습니다"} {
		if strings.Contains(reply, blocked) {
			t.Fatalf("automation status reply should be read-only, found %q:\n%s", blocked, reply)
		}
	}
	if lines := strings.Count(reply, "\n") + 1; lines > 7 {
		t.Fatalf("automation status reply too long (%d lines):\n%s", lines, reply)
	}
}

func TestAssistantStatusPhrasesUseAutomationStatusReply(t *testing.T) {
	home := t.TempDir()
	schedulePath := filepath.Join(home, ".meshclaw", "schedule-state.json")
	if err := os.MkdirAll(filepath.Dir(schedulePath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(schedulePath, []byte(`{"last_run":{"assistant-auto-check":"2026-06-12T14:03:00Z"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MESHCLAW_SCHEDULE_STATE", schedulePath)
	t.Setenv("MESHCLAW_DEV_WORKER_LATEST_JSON", filepath.Join(home, "missing-dev-worker.json"))

	for _, input := range []string{"Signal 상태 봐줘", "자동화 상태 알려줘"} {
		reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, input)
		for _, want := range []string{"Argos 자동화 상태", "Mac mini schedule", "Signal dispatcher", "읽기 전용"} {
			if !strings.Contains(reply, want) {
				t.Fatalf("reply for %q missing %q:\n%s", input, want, reply)
			}
		}
		if strings.Contains(reply, "실행하지 않습니다.") {
			t.Fatalf("status phrase should not be treated as unsafe action:\n%s", reply)
		}
	}
}

func legacyTestAssistantControlCardReplySummarizesAssistantRuntime(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	t.Setenv("MESHCLAW_SCHEDULE_STATE", filepath.Join(home, ".meshclaw", "schedule-state.json"))
	t.Setenv("MESHCLAW_SCHEDULE_STATUS_JSON", `{"status":"healthy","due_count":0,"next_due_job":"mail-watch","next_due":"2099-06-13T06:21:32Z","jobs":[{"id":"mail-watch","kind":"mail_watch","mode":"assistant","target_id":"argos-briefing","when":"15m","enabled":true,"due":false,"last_run":"2099-06-13T06:06:32Z","next_due":"2099-06-13T06:21:32Z"}]}`)
	t.Setenv("MESHCLAW_ASSISTANT_WATCHES", filepath.Join(home, ".meshclaw", "assistant-watches.json"))
	t.Setenv("MESHCLAW_SCHEDULED_DELIVERIES", filepath.Join(home, ".meshclaw", "scheduled-deliveries.json"))
	t.Setenv("MESHCLAW_ARGOS_DASHBOARD_URL", "https://argos.zeus.kim/argos/dashboard.html")
	if _, _, err := UpsertTarget(Target{ID: "argos-assistant", Channel: "signal", GroupID: "group.assistant", Label: "비서방", Mode: "assistant"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group.briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".meshclaw"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".meshclaw", "schedule-state.json"), []byte(`{"last_run":{"mail-watch":"2026-06-12T14:00:00Z","assistant-auto-check":"2026-06-12T14:03:00Z"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".meshclaw", "assistant-watches.json"), []byte(`{"watches":[{"id":"watch-1","kind":"price_alert","query":"test","enabled":true,"status":"watching"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SaveScheduledDeliveryStore(ScheduledDeliveryStore{Jobs: []ScheduledDeliveryJob{{ID: "job-1", Enabled: true, Status: "registered"}}}); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{"비서 상태 확인", "설정 보여줘", "Hermes 상태"} {
		reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, input)
		for _, want := range []string{"Argos 컨트롤 카드", "정체성", "스킬", "메모리", "채널", "자동화", "다음 실행", "새 메일 확인 (mail-watch)", "관제센터", "https://argos.zeus.kim/argos/dashboard.html", "최근 승인 묶음", "승인", "Health", "Approvals", "Evidence", "Recent Reports", "Dashboard=관찰", "Signal=승인", "Webhook=외부 이벤트 수신", "비서방/채팅방은 대화형", "보고방/보고방 Ops는 one-way/no-reply", "자동화 목록", "승인 대기열", "웹훅 상태", "최근 작업", "읽기 전용"} {
			if !strings.Contains(reply, want) {
				t.Fatalf("control card for %q missing %q:\n%s", input, want, reply)
			}
		}
		if strings.Contains(reply, "Argos 자동화 상태입니다.") {
			t.Fatalf("control card should not fall back to automation-only status:\n%s", reply)
		}
	}
}

func TestAssistantAutomationListReplyShowsSchedulesWatchesAndDeliveries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_SCHEDULE_STATE", filepath.Join(home, ".meshclaw", "schedule-state.json"))
	t.Setenv("MESHCLAW_SCHEDULE_STATUS_JSON", `{"status":"backlog","due_count":1,"next_due_job":"assistant-auto-check","next_due":"2026-06-12T17:03:00Z","jobs":[{"id":"mail-watch","kind":"mail_watch","mode":"assistant","target_id":"argos-briefing","when":"15m","enabled":true,"due":true,"last_run":"2026-06-12T14:00:00Z","next_due":"2026-06-12T14:15:00Z"},{"id":"assistant-auto-check","kind":"assistant_auto_check","mode":"assistant","target_id":"argos-assistant","when":"3h","enabled":true,"due":false,"last_run":"2026-06-12T14:03:00Z","next_due":"2026-06-12T17:03:00Z"},{"id":"data-doctor","kind":"data_doctor","mode":"ops","target_id":"","when":"6h","enabled":true,"due":false,"last_run":"2026-06-12T13:36:00Z","next_due":"2026-06-12T19:36:00Z"}]}`)
	t.Setenv("MESHCLAW_ASSISTANT_WATCHES", filepath.Join(home, ".meshclaw", "assistant-watches.json"))
	t.Setenv("MESHCLAW_SCHEDULED_DELIVERIES", filepath.Join(home, ".meshclaw", "scheduled-deliveries.json"))
	if err := os.MkdirAll(filepath.Join(home, ".meshclaw"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".meshclaw", "schedule-state.json"), []byte(`{"last_run":{"mail-watch":"2026-06-12T14:00:00Z","assistant-auto-check":"2026-06-12T14:03:00Z"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := evidence.Store("schedule-mail_watch", "assistant", "mail watch: accounts=3 raw=2 new=0 seen=2 errors=0 seen_errors=1", map[string]interface{}{"new_messages": 0}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".meshclaw", "assistant-watches.json"), []byte(`{"watches":[{"id":"watch-1","kind":"price_alert","query":"나이키 러닝화 3만원 아래","enabled":true,"status":"watching"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SaveScheduledDeliveryStore(ScheduledDeliveryStore{Jobs: []ScheduledDeliveryJob{{
		ID:          "job-1",
		Enabled:     true,
		Status:      "registered",
		TargetID:    "argos-briefing",
		TargetLabel: "보고방",
		Schedule:    "매일 오전 8시",
		Content:     "데브옵스 보안 브리핑",
	}}}); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{"자동화 목록 보여줘", "list automations", "show automations"} {
		if !isAssistantAutomationListRequest(input) {
			t.Fatalf("automation list request was not detected: %q", input)
		}
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "자동화 목록 보여줘")
	for _, want := range []string{"Argos 자동화 목록", "스케줄", "새 메일 확인 (mail-watch)", "상태=due", "목적=새 메일을 읽고 보고방에 요약", "주기=15m", "방=보고방", "최근 결과=메일 확인", "새 메일 0", "비서 기능 점검 (assistant-auto-check)", "상태=ok", "정체성, 스킬, 메모리", "3h / 하루 8번", "방=비서방", "data-doctor", "data_doctor", "최근=", "다음=", "감시/watch: 1개 활성", "나이키 러닝화", "예약 발송: 1개 활성", "매일 오전 8시", "관제센터", "중지 승인 요청", "읽기 전용"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("automation list missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "Argos 자동화 상태입니다.") {
		t.Fatalf("automation list should not fall back to status reply:\n%s", reply)
	}
}

func TestAssistantAutomationListConciseReplyShowsOnlyUsefulStopSummary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SCHEDULE_STATUS_JSON", `{"status":"backlog","due_count":1,"jobs":[{"id":"mail-watch","kind":"mail_watch","mode":"assistant","target_id":"argos-briefing","when":"15m","enabled":true,"due":true,"last_run":"2026-06-12T14:00:00Z","next_due":"2026-06-12T14:15:00Z"},{"id":"assistant-auto-check","kind":"assistant_auto_check","mode":"assistant","target_id":"argos-assistant","when":"3h","enabled":true,"due":false,"last_run":"2026-06-12T14:03:00Z","next_due":"2026-06-12T17:03:00Z"},{"id":"serverops-quickcheck","kind":"ops","mode":"ops","target_id":"argos-ops","when":"3h","enabled":false,"due":false,"last_run":"2026-06-12T13:36:00Z","next_due":"2026-06-12T19:36:00Z"}]}`)
	t.Setenv("MESHCLAW_ASSISTANT_WATCHES", filepath.Join(home, ".meshclaw", "assistant-watches.json"))
	t.Setenv("MESHCLAW_SCHEDULED_DELIVERIES", filepath.Join(home, ".meshclaw", "scheduled-deliveries.json"))
	if err := os.MkdirAll(filepath.Join(home, ".meshclaw"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".meshclaw", "assistant-watches.json"), []byte(`{"watches":[{"id":"watch-1","kind":"price_alert","query":"나이키 러닝화 3만원 아래","enabled":true,"status":"watching"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SaveScheduledDeliveryStore(ScheduledDeliveryStore{Jobs: []ScheduledDeliveryJob{{
		ID:          "delivery-1",
		Enabled:     true,
		Status:      "registered",
		TargetID:    "argos-briefing",
		TargetLabel: "보고방",
		Schedule:    "매일 오전 8시",
		Content:     "데브옵스 보안 브리핑",
	}}}); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "자동화 목록 보여줘. 지금 돌아가는 예약 발송과 중지할 수 있는 것만 간단히 보여줘.")
	for _, want := range []string{"Argos 자동화 요약", "지금 돌아가는 스케줄: 2개", "실행 대기 1개", "새 메일 확인 (mail-watch)", "비서 기능 점검 (assistant-auto-check)", "예약 발송: 1개 활성", "중지 가능", "`mail-watch 중지 승인 요청 만들어줘`", "관제센터", "조회만 했습니다"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("concise automation list missing %q:\n%s", want, reply)
		}
	}
	for _, blocked := range []string{"목적=", "최근 결과=", "상태=due", "data_doctor"} {
		if strings.Contains(reply, blocked) {
			t.Fatalf("concise automation list should not include verbose detail %q:\n%s", blocked, reply)
		}
	}
}

func TestAssistantAutomationListConciseReplyUsesLanguagePack(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SCHEDULE_STATUS_JSON", `{"status":"healthy","due_count":0,"jobs":[{"id":"mail-watch","kind":"mail_watch","mode":"assistant","target_id":"argos-briefing","when":"15m","enabled":true,"due":false,"last_run":"2026-06-12T14:00:00Z","next_due":"2026-06-12T14:15:00Z"}]}`)
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "show automations briefly")
	for _, want := range []string{"Argos automation summary.", "Running schedules: 1, due now 0", "Can request stop approval:", "Operations center:"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("concise automation list did not use language pack %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "Argos 자동화 요약") || strings.Contains(reply, "지금 돌아가는 스케줄") {
		t.Fatalf("concise automation list leaked Korean strings in en locale:\n%s", reply)
	}
}

func TestAssistantReportRoomNoiseControlReplyShowsSchedulesAndPersonalization(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SCHEDULE_STATUS_JSON", `{"status":"healthy","due_count":0,"jobs":[{"id":"local-ai-briefing","kind":"local_ai_briefing","mode":"briefing","target_id":"argos-briefing","when":"hourly@07","enabled":true,"due":false,"last_run":"2026-06-18T05:07:00Z","next_due":"2026-06-18T06:07:00Z"},{"id":"mail-watch","kind":"mail_watch","mode":"assistant","target_id":"argos-briefing","when":"15m","enabled":true,"due":false,"last_run":"2026-06-18T05:00:00Z","next_due":"2026-06-18T05:15:00Z"},{"id":"assistant-watch","kind":"assistant_watch","mode":"briefing","target_id":"argos-briefing","when":"1h","enabled":true,"due":false,"last_run":"2026-06-18T05:00:00Z","next_due":"2026-06-18T06:00:00Z"},{"id":"argos-health","kind":"argos_health","mode":"briefing","target_id":"argos-briefing","when":"1h","enabled":true,"due":false,"last_run":"2026-06-18T05:00:00Z","next_due":"2026-06-18T06:00:00Z"}]}`)
	if _, err := assistantprofile.AddMemory(time.Now(), "", "user", "사용자는 보고방 알림을 줄이고 경로 노출 없는 짧은 답변을 선호한다", true); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "보고방 소음 줄여줘")
	for _, want := range []string{
		"보고방 소음 제어 카드입니다.",
		"정기 AI/뉴스 브리핑 (local-ai-briefing)",
		"새 메일 확인 (mail-watch)",
		"사용자 요청 감시 (assistant-watch)",
		"Argos Mac 상태 점검 (argos-health)",
		"`local-ai-briefing 중지 승인 요청 만들어줘`",
		"`mail-watch 중지 승인 요청 만들어줘`",
		"경로 노출 없는 짧은 요약",
		"개인화 반영:",
		"읽기 전용",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("report-room noise control missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"중지했습니다", "꺼졌습니다", "/.meshclaw/evidence", "작업 기록"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("report-room noise control should not claim mutation or expose paths %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantAutomationStopReplyRequiresExplicitTargetAndApproval(t *testing.T) {
	t.Setenv("MESHCLAW_SCHEDULE_STATUS_JSON", `{"status":"healthy","due_count":0,"jobs":[{"id":"mail-watch","kind":"mail_watch","mode":"assistant","target_id":"argos-briefing","when":"15m","enabled":true,"due":false,"last_run":"2026-06-13T06:06:32Z","next_due":"2026-06-13T06:21:32Z"},{"id":"local-ai-briefing","kind":"local_ai_briefing","mode":"briefing","target_id":"argos-briefing","when":"hourly@07","enabled":true,"due":true,"last_run":"2026-06-13T05:14:12Z","next_due":"2026-06-13T06:07:00Z"}]}`)
	for _, input := range []string{"자동화 중지", "mail-watch 멈춰", "local-ai-briefing 꺼줘", "dev loop 멈춰"} {
		if !isAssistantAutomationStopRequest(input) {
			t.Fatalf("automation stop request was not detected: %q", input)
		}
		reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, input)
		for _, want := range []string{"자동화 중지는 바로 실행하지 않습니다", "읽기 전용", "아무 스케줄도 끄지 않았습니다", "자동화 목록", "승인 대기열", "관제센터"} {
			if !strings.Contains(reply, want) {
				t.Fatalf("automation stop reply for %q missing %q:\n%s", input, want, reply)
			}
		}
		for _, blocked := range []string{"중지했습니다", "꺼졌습니다", "삭제했습니다"} {
			if strings.Contains(reply, blocked) {
				t.Fatalf("automation stop reply should not claim mutation, found %q:\n%s", blocked, reply)
			}
		}
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "mail-watch 멈춰")
	if !strings.Contains(reply, "요청 대상: mail-watch") || !strings.Contains(reply, "승인 대기열") {
		t.Fatalf("specific target should be summarized for approval:\n%s", reply)
	}
	for _, want := range []string{"현재 상태:", "새 메일 확인", "상태=ok", "주기=15m"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("specific target should include live status %q:\n%s", want, reply)
		}
	}
	if !strings.Contains(reply, "`mail-watch 중지 승인 요청 만들어줘`") {
		t.Fatalf("specific target should include next approval draft command:\n%s", reply)
	}
	if isAssistantAutomationStopRequest("가격 알림 중지") {
		t.Fatal("price alert disable should stay on the shopping/watch flow")
	}
}

func TestAssistantAutomationStopReplyEnglishUsesLanguagePack(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SCHEDULE_STATUS_JSON", `{"status":"healthy","due_count":0,"jobs":[{"id":"mail-watch","kind":"mail_watch","mode":"assistant","target_id":"argos-briefing","when":"15m","enabled":true,"due":false,"last_run":"2026-06-13T06:06:32Z","next_due":"2026-06-13T06:21:32Z"}]}`)
	for _, input := range []string{"stop automation", "stop mail-watch"} {
		if !isAssistantAutomationStopRequest(input) {
			t.Fatalf("English automation stop request was not detected: %q", input)
		}
		reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, input)
		for _, want := range []string{
			"Automation stop is not executed immediately.",
			"This reply is read-only. No schedule was disabled.",
			"approval queue",
			"automation list",
			"Operations center:",
		} {
			if !strings.Contains(reply, want) {
				t.Fatalf("English automation stop reply for %q missing %q:\n%s", input, want, reply)
			}
		}
		if input == "stop automation" {
			if !strings.Contains(reply, "Specify which automation to stop first.") {
				t.Fatalf("English generic stop should request a target:\n%s", reply)
			}
		} else {
			for _, want := range []string{
				"Requested target: mail-watch",
				"Change: prepare an approval draft to stop the `mail-watch` automation.",
				"This changes schedule or delivery state",
				"Next approval draft: `create approval request to stop mail-watch`",
			} {
				if !strings.Contains(reply, want) {
					t.Fatalf("English targeted automation stop missing %q:\n%s", want, reply)
				}
			}
		}
		if assistantCheckoutPrepContainsHangul(reply) {
			t.Fatalf("English automation stop reply should not expose Korean:\n%s", reply)
		}
		for _, blocked := range []string{"disabled successfully", "deleted", "중지했습니다", "꺼졌습니다"} {
			if strings.Contains(strings.ToLower(reply), strings.ToLower(blocked)) {
				t.Fatalf("English automation stop reply should not claim mutation %q:\n%s", blocked, reply)
			}
		}
	}
}

func TestAssistantScheduledDeliveryStatusReplyShowsRunnerBoundary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_SCHEDULED_DELIVERIES", filepath.Join(home, ".meshclaw", "scheduled-deliveries.json"))
	lastRun := time.Date(2026, 6, 12, 20, 30, 0, 0, time.UTC)
	if err := SaveScheduledDeliveryStore(ScheduledDeliveryStore{Jobs: []ScheduledDeliveryJob{
		{
			ID:          "job-1",
			Enabled:     true,
			Status:      "registered",
			TargetID:    "argos-briefing",
			TargetLabel: "보고방",
			Schedule:    "매일 오전 8시",
			Content:     "데브옵스 보안 브리핑",
			Delivery:    "signal",
			LastRunAt:   lastRun,
		},
		{ID: "job-2", Enabled: false, Status: "disabled", TargetID: "argos-chat", Schedule: "매주 월요일", Content: "팀 노트"},
	}}); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{"예약 발송 상태", "예약발송 러너", "scheduled delivery runner"} {
		if !isAssistantScheduledDeliveryStatusRequest(input) {
			t.Fatalf("scheduled delivery status request was not detected: %q", input)
		}
		reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, input)
		for _, want := range []string{"Argos 예약 발송 상태", "등록 2개", "활성 1개", "비활성 1개", "다음 발송 후보", "job-1", "매일 오전 8시", "보고방", "데브옵스 보안 브리핑", "상태=등록됨", "방식=Signal", "최근=", "다음=", "(", "작동 방식", "예약 실행기", "변경/중지", "<ID> 예약 발송 중지 승인 요청", "읽기 전용"} {
			if !strings.Contains(reply, want) {
				t.Fatalf("scheduled delivery status for %q missing %q:\n%s", input, want, reply)
			}
		}
		for _, blocked := range []string{"발송했습니다", "등록했습니다", "중지했습니다", "status=registered", "delivery=signal", "Runner 경계", "별도 runner 단계"} {
			if strings.Contains(reply, blocked) {
				t.Fatalf("scheduled delivery status must be read-only, found %q:\n%s", blocked, reply)
			}
		}
	}
}

func TestAssistantScheduledDeliveryStatusReplyExplainsEmptyState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_SCHEDULED_DELIVERIES", filepath.Join(home, ".meshclaw", "scheduled-deliveries.json"))
	if err := SaveScheduledDeliveryStore(ScheduledDeliveryStore{Jobs: []ScheduledDeliveryJob{}}); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "예약 발송 상태")
	for _, want := range []string{"등록 0개", "활성 예약 발송이 없습니다", "새 예약", "계획/미리보기", "승인 전에는 등록하거나 발송하지 않습니다", "읽기 전용"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("empty scheduled delivery status missing %q:\n%s", want, reply)
		}
	}
	for _, blocked := range []string{"등록했습니다", "발송했습니다"} {
		if strings.Contains(reply, blocked) {
			t.Fatalf("empty scheduled delivery status must be read-only, found %q:\n%s", blocked, reply)
		}
	}
}

func TestAssistantScheduledDeliveryNextRunParsesDailyKoreanTime(t *testing.T) {
	now := time.Date(2026, 6, 12, 7, 30, 0, 0, time.Local)
	next, ok := assistantScheduledDeliveryNextRun(ScheduledDeliveryJob{Schedule: "매일 오전 8시"}, now)
	if !ok || next.Hour() != 8 || next.Day() != now.Day() {
		t.Fatalf("next=%s ok=%t", next, ok)
	}
	next, ok = assistantScheduledDeliveryNextRun(ScheduledDeliveryJob{Schedule: "매일 오후 3시"}, now)
	if !ok || next.Hour() != 15 || next.Day() != now.Day() {
		t.Fatalf("pm next=%s ok=%t", next, ok)
	}
	next, ok = assistantScheduledDeliveryNextRun(ScheduledDeliveryJob{Schedule: "매일 오전 7시"}, now)
	if !ok || next.Hour() != 7 || next.Day() != now.Add(24*time.Hour).Day() {
		t.Fatalf("tomorrow next=%s ok=%t", next, ok)
	}
	next, ok = assistantScheduledDeliveryNextRun(ScheduledDeliveryJob{Schedule: "매주 월요일 9시"}, now)
	if !ok || next.Weekday() != time.Monday || next.Hour() != 9 || next.Day() != 15 {
		t.Fatalf("weekly Monday next=%s ok=%t", next, ok)
	}
	next, ok = assistantScheduledDeliveryNextRun(ScheduledDeliveryJob{Schedule: "매주 금욜 오후 2시"}, now)
	if !ok || next.Weekday() != time.Friday || next.Hour() != 14 || next.Day() != 12 {
		t.Fatalf("weekly colloquial Friday next=%s ok=%t", next, ok)
	}
	text := assistantScheduledDeliveryNextText(ScheduledDeliveryJob{Schedule: "매주 월요일 9시"}, now)
	if !strings.Contains(text, "06-15 09:00") || !strings.Contains(text, "in ") {
		t.Fatalf("weekly next text should include exact next run and remaining time: %q", text)
	}
}

func TestAssistantApprovalQueueReplyExplainsApprovalBoundary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_SCHEDULE_STATUS_JSON", `{"status":"healthy","due_count":0,"jobs":[{"id":"mail-watch","kind":"mail_watch","mode":"assistant","target_id":"argos-briefing","when":"15m","enabled":true,"due":false,"last_run":"2026-06-13T06:06:32Z","next_due":"2026-06-13T06:21:32Z"},{"id":"assistant-auto-check","kind":"assistant_auto_check","mode":"assistant","target_id":"argos-assistant","when":"3h","enabled":true,"due":false,"last_run":"2026-06-13T05:49:57Z","next_due":"2026-06-13T08:49:57Z"}]}`)
	t.Setenv("MESHCLAW_SCHEDULED_DELIVERIES", filepath.Join(home, ".meshclaw", "scheduled-deliveries.json"))
	if err := SaveScheduledDeliveryStore(ScheduledDeliveryStore{Jobs: []ScheduledDeliveryJob{{
		ID:          "scheduled-news",
		Enabled:     true,
		Status:      "registered",
		TargetID:    "argos-briefing",
		TargetLabel: "보고방",
		Schedule:    "매주 월요일 9시",
		Content:     "주간 뉴스 브리핑",
		Delivery:    "signal",
	}}}); err != nil {
		t.Fatal(err)
	}
	if live, ok := readAssistantScheduleLiveStatus(); !ok {
		t.Fatal("schedule live status fixture was not readable")
	} else if _, found := assistantScheduleLiveJobByID(live, "mail-watch"); !found {
		t.Fatalf("mail-watch live fixture was not found: %#v", live.Jobs)
	}
	for _, input := range []string{"승인 대기열 보여줘", "승인 목록"} {
		if !isAssistantApprovalQueueRequest(input) {
			t.Fatalf("approval queue request was not detected: %q", input)
		}
		reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, input)
		for _, want := range []string{"Argos 승인 대기열", "관제센터", "승인 후보", "승인 기록", "지금 만들 수 있는 승인 초안", "mail-watch 중지 승인 요청", "현재 새 메일 확인", "상태=ok", "주기=15m", "assistant-auto-check 중지 승인 요청", "현재 비서 기능 점검", "webhook receiver 열기 승인 요청", "예약 발송 변경", "scheduled-news 예약 발송 중지 승인 요청", "메일/결제/삭제", "승인 요청 예시", "자동 실행", "정책을 다시 확인", "읽기 전용"} {
			if !strings.Contains(reply, want) {
				t.Fatalf("approval queue reply for %q missing %q:\n%s", input, want, reply)
			}
		}
		for _, blocked := range []string{"workflow bundle", "workflow resume", "evidence 기록"} {
			if strings.Contains(reply, blocked) {
				t.Fatalf("approval queue reply should avoid internal term %q:\n%s", blocked, reply)
			}
		}
		if strings.Contains(reply, "승인 기록 완료") {
			t.Fatalf("approval queue must not record an approval:\n%s", reply)
		}
	}
}

func TestAssistantApprovalQueueEnglishInputWithoutEnvUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-approval-en-no-env", Mode: "assistant"}, "approval queue")
	for _, want := range []string{
		"Argos approval queue.",
		"Operations center:",
		"Approval drafts you can create now:",
		"`create approval request to stop mail-watch`",
		"Server actions:",
		"Mail/payment/delete",
		"Important: approval is not automatic execution.",
		"This was read-only.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English no-env approval queue missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English no-env approval queue should not expose Korean:\n%s", reply)
	}
}

func TestAssistantApprovalQueueReplyShowsRecentPurchaseFinalClickCandidate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	targetID := "argos-assistant-approval-purchase"
	ready := formatPurchaseFinalExecutionTemplateReply(purchaseExecutionFields{
		Merchant:     "쿠팡",
		Item:         "풀무원샘물 무라벨 생수 500ml 20개",
		Total:        "10,990원",
		Delivery:     "내일 도착",
		Shipping:     "맞음",
		Payment:      "맞음",
		URL:          "https://www.coupang.com/checkout/order",
		X:            456,
		Y:            789,
		Proof:        "버튼 위치 확인됨",
		Confirmation: "구매 실행 승인",
	})
	if err := appendSignalHistory(targetID, "최종 실행 템플릿", ready); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "승인 대기열 보여줘")
	for _, want := range []string{
		"Argos 승인 대기열",
		"바로 보낼 문장:",
		"구매 최종 클릭: `구매 승인` 또는 `구매 실행 승인`",
		"최근 구매 최종 클릭 대기 후보:",
		"구매 최종 클릭: 풀무원샘물 무라벨 생수 500ml 20개",
		"총액=10,990원",
		"`meshclaw_purchase_click`",
		"최종 버튼 위치를 다시 확인",
		"정책을 재확인",
		"`구매 승인` 또는 `구매 실행 승인`을 보내세요",
		"`구매 플로우 중지`",
		"여기서 즉시 클릭하지 않습니다",
		"읽기 전용",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("approval queue purchase candidate missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{`"execute": true`, `"approve": true`, "구매 완료", "주문 완료", "최종 주문 버튼을 눌렀습니다"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("approval queue purchase candidate should remain read-only and not expose %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantApprovalQueuePurchaseCandidateEnglishActionsUseLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	targetID := "argos-assistant-approval-purchase-en"
	ready := formatPurchaseFinalExecutionTemplateReply(purchaseExecutionFields{
		Merchant:     "Coupang",
		Item:         "Pulmuone bottled water 500ml 20-pack",
		Total:        "10,990 KRW",
		Delivery:     "tomorrow",
		Shipping:     "yes",
		Payment:      "yes",
		URL:          "https://www.coupang.com/checkout/order",
		X:            456,
		Y:            789,
		Proof:        "button location confirmed",
		Confirmation: "purchase execution approved",
	})
	if err := appendSignalHistory(targetID, "final execution template", ready); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "approval queue")
	for _, want := range []string{
		"Argos approval queue.",
		"Send one of these exact phrases:",
		"Purchase final click: `purchase approved` or `purchase execution approved`",
		"Operations center:",
		"Approval drafts you can create now:",
		"`create approval request to stop mail-watch`",
		"Mail/payment/delete",
		"Important: approval is not automatic execution.",
		"Recent purchase final-click candidate:",
		"Purchase final click: Pulmuone bottled water 500ml 20-pack",
		"total=10,990 KRW",
		"Continue: if the final screen still matches",
		"`purchase approved` or `purchase execution approved`",
		"Stop: send `stop the purchase flow`",
		"This queue card is read-only and does not click immediately.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English approval queue purchase candidate missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English approval queue purchase candidate should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{`"execute": true`, `"approve": true`, "purchase completed", "order completed", "clicked the final"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English approval queue purchase candidate should remain read-only and not expose %q:\n%s", bad, reply)
		}
	}
}

func legacyTestAssistantApprovalQueueShowsPendingDirectPurchaseResume(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	targetID := "argos-assistant-approval-direct-purchase"
	rememberPendingShoppingDirectPurchase(ListenOptions{TargetID: targetID, Mode: "assistant"}, "휴지 구매해", "휴지")

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "승인 대기열 보여줘")
	for _, want := range []string{
		"바로 보낼 문장:",
		"구매 자동 진행 재개: `구매 승인`, `ㅇㅇ`, 또는 `현재 화면에서 다시 해봐`",
		"최근 구매 자동 진행 대기 후보:",
		"구매 자동 진행: 휴지",
		"승인 상태: 이 구매 건은 이미 한 번 승인된 흐름입니다.",
		"`구매 승인`이나 `ㅇㅇ`은 새 승인 요청이 아니라 같은 구매를 이어 진행합니다.",
		"진행 증거 확인: `최근 작업 보여줘` 또는 관제센터 Evidence에서 최근 캡처/클릭 기록을 확인하세요.",
		"계속: 화면 처리나 상품 확인을 끝냈으면 `구매 승인`, `ㅇㅇ`, `ㄱㄱ`, `다시 해봐`, `계속해`",
		"`현재 화면에서 다시 해봐`",
		"`구매 플로우 중지`",
		"여기서 즉시 클릭하지 않습니다",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("approval queue pending direct purchase missing %q:\n%s", want, reply)
		}
	}
	if _, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false); !ok {
		t.Fatal("approval queue should not consume pending direct purchase")
	}
}

func legacyTestAssistantApprovalQueueShowsPendingDirectPurchaseSelectedLink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	targetID := "argos-assistant-approval-direct-purchase-selected-link"
	selectedURL := "https://www.coupang.com/vp/products/456"
	rememberPendingShoppingDirectPurchaseWithURLAndBlocker(ListenOptions{TargetID: targetID, Mode: "assistant"}, "생수 구매해\n2번으로 할게", "아이시스 500ml 20개", selectedURL, "")

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "승인 대기열 보여줘")
	for _, want := range []string{
		"최근 구매 자동 진행 대기 후보:",
		"구매 자동 진행: 아이시스 500ml 20개",
		"선택 후보 링크: " + selectedURL,
		"승인 상태: 이 구매 건은 이미 한 번 승인된 흐름입니다.",
		"진행 증거 확인: `최근 작업 보여줘` 또는 관제센터 Evidence에서 최근 캡처/클릭 기록을 확인하세요.",
		"`현재 화면에서 다시 해봐`",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("approval queue selected direct purchase link missing %q:\n%s", want, reply)
		}
	}
	if _, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false); !ok {
		t.Fatal("approval queue should not consume pending direct purchase with selected link")
	}
}

func legacyTestAssistantApprovalQueueShowsPendingDirectPurchaseBlocker(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	targetID := "argos-assistant-approval-direct-purchase-blocked"
	rememberPendingShoppingDirectPurchaseWithBlocker(ListenOptions{TargetID: targetID, Mode: "assistant"}, "휴지 구매해", "휴지", directPurchaseBlockerMacOSPermission)

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "승인 대기열 보여줘")
	for _, want := range []string{
		"최근 구매 자동 진행 대기 후보:",
		"구매 자동 진행: 휴지",
		"승인 상태: 이 구매 건은 이미 한 번 승인된 흐름입니다.",
		"진행 증거 확인: `최근 작업 보여줘` 또는 관제센터 Evidence에서 최근 캡처/클릭 기록을 확인하세요.",
		"마지막 중단 이유: macOS 권한 확인 팝업",
		"사용자 처리: macOS 권한 팝업에서 허용을 선택한 뒤 `ㅇㅇ`을 보내세요.",
		"계속: 화면 처리나 상품 확인을 끝냈으면 `구매 승인`, `ㅇㅇ`, `ㄱㄱ`, `다시 해봐`, `계속해`",
		"`현재 화면에서 다시 해봐`",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("approval queue pending direct purchase blocker missing %q:\n%s", want, reply)
		}
	}
}

func legacyTestAssistantApprovalQueueShowsPendingDirectPurchaseBlockerEnglishHint(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_ARGOS_DASHBOARD_URL", "https://argos.example/argos/dashboard.html")
	targetID := "argos-assistant-approval-direct-purchase-blocked-en"
	evidenceID := "20260618T072124Z-222333444"
	rememberPendingShoppingDirectPurchaseWithURLBlockerEvidence(ListenOptions{TargetID: targetID, Mode: "assistant"}, "buy toilet paper", "toilet paper", "", directPurchaseBlockerPaymentAuth, evidenceID)

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "approval queue")
	for _, want := range []string{
		"Recent pending automatic purchase:",
		"Automatic purchase: toilet paper",
		"Approval state: this purchase flow already has the one approval.",
		"Evidence check: send `recent reports " + evidenceID + "` or just the short ID `" + evidenceID + "`.",
		"Evidence tab: " + assistantDashboardEvidenceURL(evidenceID),
		"Last blocker: payment authentication",
		"User action: Enter only the card CVC/payment password prompt, then send `yes`.",
		"Continue: after handling the screen or confirming the item, send `purchase approved`, `yes`, `go`, `try again`, or `continue` to resume.",
		"`try again from current screen`",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English approval queue pending direct purchase blocker missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English approval queue blocker hint should not expose Korean:\n%s", reply)
	}
}

func legacyTestAssistantApprovalQueueShowsPendingDirectPurchaseResumeEnglish(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	targetID := "argos-assistant-approval-direct-purchase-en"
	rememberPendingShoppingDirectPurchase(ListenOptions{TargetID: targetID, Mode: "assistant"}, "buy toilet paper", "toilet paper")

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "approval queue")
	for _, want := range []string{
		"Send one of these exact phrases:",
		"Resume automatic purchase: `purchase approved`, `yes`, or `try again from current screen`",
		"Recent pending automatic purchase:",
		"Automatic purchase: toilet paper",
		"Approval state: this purchase flow already has the one approval.",
		"`purchase approved` or `yes` resumes the same purchase instead of asking again.",
		"Evidence check: send `recent reports` or open the Operations Center Evidence tab for the latest captures and click records.",
		"Continue: after handling the screen or confirming the item, send `purchase approved`, `yes`, `go`, `try again`, or `continue` to resume.",
		"`try again from current screen`",
		"Stop: send `stop the purchase flow`",
		"This queue card is read-only and does not click immediately.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English approval queue pending direct purchase missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English pending direct purchase queue should not expose Korean:\n%s", reply)
	}
	if _, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false); !ok {
		t.Fatal("English approval queue should not consume pending direct purchase")
	}
}

func legacyTestAssistantDirectPurchaseRetryHelpShowsPendingWithoutConsuming(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	targetID := "argos-assistant-direct-purchase-retry-help"
	rememberPendingShoppingDirectPurchaseWithBlocker(ListenOptions{TargetID: targetID, Mode: "assistant"}, "휴지 구매해", "휴지", directPurchaseBlockerMacOSPermission)

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "안 사잖아?")
	for _, want := range []string{
		"구매 자동 진행 재시도 안내입니다.",
		"대기 중인 상품: 휴지",
		"마지막 중단 이유: macOS 권한 확인 팝업",
		"`구매 승인`, `ㅇㅇ`, `ㄱㄱ`, `다시 해봐`, `계속해`",
		"`현재 화면에서 다시 해봐`",
		"대기 상태를 소비하지 않습니다",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("Korean retry help missing %q:\n%s", want, reply)
		}
	}
	if _, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false); !ok {
		t.Fatal("retry help should not consume pending direct purchase")
	}
}

func legacyTestAssistantDirectPurchaseRetryHelpShowsSelectedLink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	targetID := "argos-assistant-direct-purchase-retry-help-selected-link"
	selectedURL := "https://www.coupang.com/vp/products/456"
	rememberPendingShoppingDirectPurchaseWithURLAndBlocker(ListenOptions{TargetID: targetID, Mode: "assistant"}, "생수 구매해\n2번으로 할게", "아이시스 500ml 20개", selectedURL, directPurchaseBlockerNotReady)

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "안 사잖아?")
	for _, want := range []string{
		"구매 자동 진행 재시도 안내입니다.",
		"대기 중인 상품: 아이시스 500ml 20개",
		"선택 후보 링크: " + selectedURL,
		"마지막 중단 이유: 상품/총액/배송/결제 화면이 아직 최종 주문 가능 상태로 읽히지 않음",
		"대기 상태를 소비하지 않습니다",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("retry help selected link missing %q:\n%s", want, reply)
		}
	}
	if _, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false); !ok {
		t.Fatal("retry help should not consume pending direct purchase with selected link")
	}
}

func legacyTestAssistantDirectPurchaseRetryHelpEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	targetID := "argos-assistant-direct-purchase-retry-help-en"
	selectedURL := "https://www.coupang.com/vp/products/456"
	rememberPendingShoppingDirectPurchaseWithURLAndBlocker(ListenOptions{TargetID: targetID, Mode: "assistant"}, "buy toilet paper", "toilet paper", selectedURL, directPurchaseBlockerNotReady)

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "purchase stuck")
	for _, want := range []string{
		"Automatic purchase retry help.",
		"Pending item: toilet paper",
		"Selected candidate link: " + selectedURL,
		"Last blocker: product, total, delivery, and payment screen are not yet readable as final-order ready",
		"`try again from current screen`",
		"This help message does not consume the pending purchase state.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English retry help missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English retry help should not expose Korean:\n%s", reply)
	}
	if _, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false); !ok {
		t.Fatal("English retry help should not consume pending direct purchase")
	}
}

func legacyTestAssistantDirectPurchaseRetryHelpEnglishPaymentAuthBlocker(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	targetID := "argos-assistant-direct-purchase-retry-help-en-payment-auth"
	rememberPendingShoppingDirectPurchaseWithURLAndBlocker(ListenOptions{TargetID: targetID, Mode: "assistant"}, "buy toilet paper", "toilet paper", "", directPurchaseBlockerPaymentAuth)

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "purchase stuck")
	for _, want := range []string{
		"Automatic purchase retry help.",
		"Pending item: toilet paper",
		"Last blocker: payment authentication",
		"User action: Enter only the card CVC/payment password prompt, then send `yes`.",
		"`purchase approved`, `yes`, `go`, `try again`, or `continue`",
		"This help message does not consume the pending purchase state.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English retry help payment-auth blocker missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English payment-auth retry help should not expose Korean:\n%s", reply)
	}
}

func legacyTestAssistantPendingDirectPurchaseCandidateSearchUsesPendingQuery(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
	targetID := "argos-assistant-direct-purchase-pending-candidates"
	rememberPendingShoppingDirectPurchase(ListenOptions{TargetID: targetID, Mode: "assistant"}, "휴지 구매해", "휴지")

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "제품 추천해서 장바구니까지 준비해야 하는데")
	for _, want := range []string{
		"쿠팡 후보 비교를 시작했습니다.",
		"검색어: 휴지",
		"https://www.coupang.com/np/search?q=%ED%9C%B4%EC%A7%80",
		"검색 결과 후보:",
		"`상위 3개 후보 비교표 만들어줘`",
		"`장바구니까지 준비해. 결제는 하지 마`",
		"구매 대기는 유지됩니다.",
		"`현재 화면에서 다시 해봐`",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("pending direct purchase candidate search missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"최근 쇼핑 후보를 찾지 못했습니다", "구매 완료", "주문 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("pending direct purchase candidate search should not contain %q:\n%s", bad, reply)
		}
	}
	pending, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false)
	if !ok || pending.Kind != "shopping_direct_purchase" || pending.Intent.Query != "휴지" {
		t.Fatalf("candidate search should keep pending direct purchase: %#v ok=%v", pending, ok)
	}
}

func legacyTestAssistantPendingDirectPurchaseCandidateSearchEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
	targetID := "argos-assistant-direct-purchase-pending-candidates-en"
	rememberPendingShoppingDirectPurchase(ListenOptions{TargetID: targetID, Mode: "assistant"}, "buy toilet paper", "toilet paper")

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "recommend candidates and prepare cart")
	for _, want := range []string{
		"Started Coupang candidate comparison.",
		"Query: toilet paper",
		"https://www.coupang.com/np/search?q=toilet+paper",
		"Search-result candidates:",
		"`make a comparison table for the top 3 candidates`",
		"`prepare the cart, but do not pay`",
		"The pending purchase remains active.",
		"`try again from current screen`",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English pending direct purchase candidate search missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English pending direct purchase candidate search should not expose Korean:\n%s", reply)
	}
	pending, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false)
	if !ok || pending.Kind != "shopping_direct_purchase" || pending.Intent.Query != "toilet paper" {
		t.Fatalf("English candidate search should keep pending direct purchase: %#v ok=%v", pending, ok)
	}
}

func TestShoppingPurchaseStopClearsPendingDirectPurchase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	targetID := "argos-assistant-direct-purchase-stop-clears-pending"
	rememberPendingShoppingDirectPurchaseWithBlocker(ListenOptions{TargetID: targetID, Mode: "assistant"}, "휴지 구매해", "휴지", directPurchaseBlockerMacOSPermission)

	stopReply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "구매 플로우 중지")
	if !strings.Contains(stopReply, "구매 플로우를 중지했습니다.") {
		t.Fatalf("purchase stop reply did not handle pending direct purchase:\n%s", stopReply)
	}
	if _, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false); ok {
		t.Fatal("purchase stop should clear pending direct purchase")
	}
	queue := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "승인 대기열 보여줘")
	for _, bad := range []string{
		"최근 구매 자동 진행 대기 후보:",
		"구매 자동 진행: 휴지",
		"마지막 중단 이유: macOS 권한 확인 팝업",
		"`ㅇㅇ` 또는 `ㄱㄱ`",
	} {
		if strings.Contains(queue, bad) {
			t.Fatalf("approval queue should hide stopped pending direct purchase %q:\n%s", bad, queue)
		}
	}
}

func legacyTestShoppingPurchaseStopClearsPendingDirectPurchaseEnglish(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	targetID := "argos-assistant-direct-purchase-stop-clears-pending-en"
	rememberPendingShoppingDirectPurchase(ListenOptions{TargetID: targetID, Mode: "assistant"}, "buy toilet paper", "toilet paper")

	stopReply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "stop the purchase flow")
	if !strings.Contains(stopReply, "Purchase flow stopped.") {
		t.Fatalf("English purchase stop reply did not handle pending direct purchase:\n%s", stopReply)
	}
	if _, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false); ok {
		t.Fatal("English purchase stop should clear pending direct purchase")
	}
	queue := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "approval queue")
	for _, bad := range []string{
		"Recent pending automatic purchase:",
		"Automatic purchase: toilet paper",
		"send `yes` or `go` to resume",
	} {
		if strings.Contains(queue, bad) {
			t.Fatalf("English approval queue should hide stopped pending direct purchase %q:\n%s", bad, queue)
		}
	}
	if assistantCheckoutPrepContainsHangul(queue) {
		t.Fatalf("English approval queue should not expose Korean after direct purchase stop:\n%s", queue)
	}
}

func legacyTestDirectPurchaseBlockedExecutionStoresQueueBlocker(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
	targetID := "argos-assistant-direct-purchase-blocker-store"

	oldCapture := shoppingCheckoutScreenCapture
	oldOCR := shoppingCheckoutScreenOCR
	oldOpenURL := shoppingDirectPurchaseOpenURL
	defer func() {
		shoppingCheckoutScreenCapture = oldCapture
		shoppingCheckoutScreenOCR = oldOCR
		shoppingDirectPurchaseOpenURL = oldOpenURL
	}()
	shoppingDirectPurchaseOpenURL = func(ctx context.Context, rawURL string) osauto.Result {
		return osauto.Result{Kind: "meshclaw_automation_open_url", Action: "open_url", URL: rawURL, OK: true}
	}
	shoppingCheckoutScreenCapture = func(ctx context.Context, output string) osauto.Result {
		if err := os.WriteFile(output, []byte("fake blocked direct purchase screen"), 0600); err != nil {
			t.Fatal(err)
		}
		return osauto.Result{Kind: "meshclaw_automation_screen_capture", Action: "screen_capture", OK: true}
	}
	shoppingCheckoutScreenOCR = func(path string) (string, string) {
		return strings.Join([]string{
			"쿠팡",
			"휴지",
			"\"meshclaw\"에서 \"Finder\"을(를) 제어하려고 합니다.",
			"허용 안 함",
			"허용",
		}, "\n"), ""
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant", Execute: true}, "휴지사")
	if !strings.Contains(reply, "macOS 권한 확인 팝업") {
		t.Fatalf("blocked direct purchase reply missing localized blocker:\n%s", reply)
	}
	pending, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false)
	if !ok || pending.Kind != "shopping_direct_purchase" {
		t.Fatalf("blocked direct purchase should store pending purchase, got %#v ok=%v", pending, ok)
	}
	if blocker := pendingShoppingDirectPurchaseBlocker(pending); blocker != directPurchaseBlockerMacOSPermission {
		t.Fatalf("pending blocker=%q, want %q", blocker, directPurchaseBlockerMacOSPermission)
	}
	queue := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "승인 대기열 보여줘")
	if !strings.Contains(queue, "마지막 중단 이유: macOS 권한 확인 팝업") ||
		!strings.Contains(queue, "계속: 화면 처리나 상품 확인을 끝냈으면 `구매 승인`, `ㅇㅇ`, `ㄱㄱ`, `다시 해봐`, `계속해`") {
		t.Fatalf("approval queue should show blocked direct purchase resume:\n%s", queue)
	}
}

func TestAssistantApprovalQueuePurchaseCandidateEnglishWithoutEnvUsesCandidateLanguage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	targetID := "argos-assistant-approval-purchase-en-no-env"
	ready := formatPurchaseFinalExecutionTemplateReplyFor("en", purchaseExecutionFields{
		Merchant:     "Coupang",
		Item:         "Pulmuone bottled water 500ml 20-pack",
		Total:        "10,990 KRW",
		Delivery:     "tomorrow",
		Shipping:     "yes",
		Payment:      "yes",
		URL:          "https://www.coupang.com/checkout/order",
		X:            456,
		Y:            789,
		Proof:        "button location confirmed",
		Confirmation: "purchase execution approved",
	})
	if err := appendSignalHistory(targetID, "final execution template", ready); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "approval queue")
	for _, want := range []string{
		"Argos approval queue.",
		"Recent purchase final-click candidate:",
		"Purchase final click: Pulmuone bottled water 500ml 20-pack",
		"total=10,990 KRW",
		"Continue: if the final screen still matches",
		"`purchase execution approved`",
		"Stop: send `stop the purchase flow`",
		"This queue card is read-only and does not click immediately.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English no-env approval queue purchase candidate missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English no-env approval queue purchase candidate should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{`"execute": true`, `"approve": true`, "구매 실행 승인", "purchase completed", "order completed", "clicked the final"} {
		if strings.Contains(strings.ToLower(reply), strings.ToLower(bad)) {
			t.Fatalf("English no-env approval queue purchase candidate should remain read-only and not expose %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantApprovalQueueHidesPurchaseCandidateAfterStop(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	targetID := "argos-assistant-approval-purchase-stopped"
	ready := formatPurchaseFinalExecutionTemplateReply(purchaseExecutionFields{
		Merchant:     "쿠팡",
		Item:         "풀무원샘물 무라벨 생수 500ml 20개",
		Total:        "10,990원",
		Delivery:     "내일 도착",
		Shipping:     "맞음",
		Payment:      "맞음",
		URL:          "https://www.coupang.com/checkout/order",
		X:            456,
		Y:            789,
		Proof:        "버튼 위치 확인됨",
		Confirmation: "구매 실행 승인",
	})
	if err := appendSignalHistory(targetID, "최종 실행 템플릿", ready); err != nil {
		t.Fatal(err)
	}
	stopReply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "구매 플로우 중지")
	if !strings.Contains(stopReply, "구매 플로우를 중지했습니다.") {
		t.Fatalf("purchase stop reply did not handle stop request:\n%s", stopReply)
	}
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "승인 대기열 보여줘")
	for _, bad := range []string{
		"최근 구매 최종 클릭 대기 후보:",
		"구매 최종 클릭: 풀무원샘물 무라벨 생수 500ml 20개",
		"`구매 실행 승인`을 다시 보내세요",
		"`meshclaw_purchase_click`",
		`"execute": true`,
		`"approve": true`,
	} {
		if strings.Contains(reply, bad) {
			t.Fatalf("approval queue should hide stopped purchase candidate %q:\n%s", bad, reply)
		}
	}
	for _, want := range []string{"Argos 승인 대기열", "지금 만들 수 있는 승인 초안", "읽기 전용"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("approval queue after purchase stop missing %q:\n%s", want, reply)
		}
	}
}

func TestAssistantApprovalQueueHidesPurchaseCandidateAfterStopEnglish(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	targetID := "argos-assistant-approval-purchase-stopped-en"
	ready := formatPurchaseFinalExecutionTemplateReply(purchaseExecutionFields{
		Merchant:     "Coupang",
		Item:         "Pulmuone bottled water 500ml 20-pack",
		Total:        "10,990 KRW",
		Delivery:     "tomorrow",
		Shipping:     "yes",
		Payment:      "yes",
		URL:          "https://www.coupang.com/checkout/order",
		X:            456,
		Y:            789,
		Proof:        "button location confirmed",
		Confirmation: "purchase execution approved",
	})
	if err := appendSignalHistory(targetID, "final execution template", ready); err != nil {
		t.Fatal(err)
	}
	stopReply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "stop the purchase flow")
	if !strings.Contains(stopReply, "Purchase flow stopped.") {
		t.Fatalf("English purchase stop reply did not handle stop request:\n%s", stopReply)
	}
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "approval queue")
	for _, bad := range []string{
		"Recent purchase final-click candidate:",
		"Purchase final click: Pulmuone bottled water 500ml 20-pack",
		"`purchase execution approved`",
		"`meshclaw_purchase_click`",
		`"execute": true`,
		`"approve": true`,
	} {
		if strings.Contains(reply, bad) {
			t.Fatalf("English approval queue should hide stopped purchase candidate %q:\n%s", bad, reply)
		}
	}
	for _, want := range []string{"Argos approval queue.", "Approval drafts you can create now:", "This was read-only."} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English approval queue after purchase stop missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English approval queue after purchase stop should not expose Korean:\n%s", reply)
	}
}

func TestAssistantApprovalRequestDraftReplyIsExplicitAndReadOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	t.Setenv("MESHCLAW_SCHEDULE_STATUS_JSON", `{"status":"healthy","due_count":0,"jobs":[{"id":"mail-watch","kind":"mail_watch","mode":"assistant","target_id":"argos-briefing","when":"15m","enabled":true,"due":false,"last_run":"2026-06-13T06:06:32Z","next_due":"2026-06-13T06:21:32Z"}]}`)
	t.Setenv("MESHCLAW_SCHEDULED_DELIVERIES", filepath.Join(home, ".meshclaw", "scheduled-deliveries.json"))
	if _, _, err := UpsertTarget(Target{ID: "argos-assistant", Channel: "signal", GroupID: "group-assistant", Label: "비서방", Mode: "assistant"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}
	if err := SaveScheduledDeliveryStore(ScheduledDeliveryStore{Jobs: []ScheduledDeliveryJob{{
		ID:          "scheduled-news",
		Enabled:     true,
		Status:      "registered",
		TargetID:    "argos-briefing",
		TargetLabel: "보고방",
		Schedule:    "매주 월요일 9시",
		Content:     "주간 뉴스 브리핑",
		Delivery:    "signal",
		LastRunAt:   time.Date(2026, 6, 12, 6, 0, 0, 0, time.UTC),
	}}}); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{"승인 요청 만들어줘", "mail-watch 중지 승인 요청 만들어줘", "g4 서버 재시작 승인 요청 초안", "approval request draft for deploy", "webhook receiver 열기 승인 요청 만들어줘", "scheduled-news 예약 발송 중지 승인 요청 만들어줘"} {
		if !isAssistantApprovalRequestDraftRequest(input) {
			t.Fatalf("approval request draft was not detected: %q", input)
		}
		reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, input)
		for _, want := range []string{"승인 요청 초안", "필요한 확인", "무엇을 바꾸는가", "되돌리는 방법", "실행 후 확인할 결과", "초안만", "명시 승인"} {
			if !strings.Contains(reply, want) {
				t.Fatalf("approval request draft for %q missing %q:\n%s", input, want, reply)
			}
		}
		if strings.Contains(reply, "확인할 evidence") {
			t.Fatalf("approval request draft should use user-facing result wording:\n%s", reply)
		}
		for _, blocked := range []string{"승인 기록 완료", "실행했습니다", "배포했습니다", "발송했습니다"} {
			if strings.Contains(reply, blocked) {
				t.Fatalf("approval request draft should not mutate, found %q:\n%s", blocked, reply)
			}
		}
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "mail-watch 중지 승인 요청 만들어줘")
	if !strings.Contains(reply, "대상: mail-watch") {
		t.Fatalf("specific approval subject should be summarized:\n%s", reply)
	}
	for _, want := range []string{"현재 상태:", "새 메일 확인", "상태=ok", "주기=15m", "방=보고방", "변경 내용:", "정기 실행과 자동 보고를 멈춥니다", "되돌리는 방법:", "자동화 상태", "확인할 결과:", "상태=disabled"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("schedule approval draft missing %q:\n%s", want, reply)
		}
	}
	reply = assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "scheduled-news 예약 발송 중지 승인 요청 만들어줘")
	for _, want := range []string{"대상: scheduled-news", "현재 예약:", "매주 월요일 9시", "보고방", "방식=Signal", "주간 뉴스 브리핑", "변경 내용:", "예약 발송을 비활성화", "되돌리는 방법:", "예약 발송 상태", "확인할 결과:", "상태=꺼짐"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("scheduled delivery approval draft missing %q:\n%s", want, reply)
		}
	}
	reply = assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "webhook receiver 열기 승인 요청 만들어줘")
	for _, want := range []string{"대상: webhook receiver", "현재 상태:", "Signal targets 2개", "Webhook 공개 수신 endpoint는 닫힘", "변경 내용:", "수신 기록 저장과 승인 초안 생성", "execute=false", "되돌리는 방법:", "웹훅 상태", "확인할 결과:", "서명 없는 요청은 거부", "메일 발송, 삭제, 결제", "웹훅 개방 조건", "/argos/webhook/events", "HMAC", "rate limit"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("webhook approval draft missing %q:\n%s", want, reply)
		}
	}
}

func TestAssistantApprovalRequestDraftEnglishUsesLanguagePack(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "en")
	for _, input := range []string{"approval request draft for deploy", "create approval request to open webhook receiver"} {
		if !isAssistantApprovalRequestDraftRequest(input) {
			t.Fatalf("English approval request draft was not detected: %q", input)
		}
		reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, input)
		for _, want := range []string{
			"Approval request draft.",
			"Target:",
			"Change: prepare an approval draft",
			"Required checks:",
			"- what changes",
			"- rollback path",
			"- result to verify after execution",
			"This reply only prepares the draft.",
			"To proceed, put it in the approval queue",
		} {
			if !strings.Contains(reply, want) {
				t.Fatalf("English approval request draft for %q missing %q:\n%s", input, want, reply)
			}
		}
		if strings.Contains(input, "webhook") {
			for _, want := range []string{"Webhook opening conditions:", "/argos/webhook/events", "HMAC signature", "execute=false"} {
				if !strings.Contains(reply, want) {
					t.Fatalf("English webhook approval draft missing %q:\n%s", want, reply)
				}
			}
		}
		if assistantCheckoutPrepContainsHangul(reply) {
			t.Fatalf("English approval request draft should not expose Korean for %q:\n%s", input, reply)
		}
		for _, blocked := range []string{"approval recorded", "executed", "sent", "deployed", "구매 완료", "주문 완료"} {
			if strings.Contains(strings.ToLower(reply), strings.ToLower(blocked)) && !strings.Contains(reply, "did not record approval, execute, send, or deploy") {
				t.Fatalf("English approval draft should stay read-only, found %q:\n%s", blocked, reply)
			}
		}
	}
}

func TestAssistantDashboardReplyLinksOperationsCenter(t *testing.T) {
	t.Setenv("MESHCLAW_ARGOS_DASHBOARD_URL", "https://argos.zeus.kim/argos/dashboard.html")
	for _, input := range []string{"대시보드 보여줘", "관제센터", "NOC 화면"} {
		if !isAssistantDashboardRequest(input) {
			t.Fatalf("dashboard request was not detected: %q", input)
		}
		reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, input)
		for _, want := range []string{"Argos 관제 센터", "https://argos.zeus.kim/argos/dashboard.html", "Health", "Approvals", "Evidence", "Dashboard = Observe", "Signal = Approve"} {
			if !strings.Contains(reply, want) {
				t.Fatalf("dashboard reply for %q missing %q:\n%s", input, want, reply)
			}
		}
		for _, blocked := range []string{"배포했습니다", "승인했습니다", "발송했습니다"} {
			if strings.Contains(reply, blocked) {
				t.Fatalf("dashboard reply should be read-only, found %q:\n%s", blocked, reply)
			}
		}
	}
}

func TestAssistantRecentReportsReplyShowsEvidenceWithoutRawURLs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_DASHBOARD_URL", "대시보드")
	openRecord, err := evidence.Store("open_url", "assistant", "https://www.google.com/search?tbm=shop&q=%EA%B8%B4%EC%A3%BC%EC%86%8C", map[string]string{"source": "test"})
	if err != nil {
		t.Fatal(err)
	}
	newsRecord, err := evidence.Store("assistant-news", "assistant", "뉴스 브리핑 완료", map[string]string{"source": "test"})
	if err != nil {
		t.Fatal(err)
	}
	purchaseRecord, err := evidence.Store("assistant-direct-purchase-approved-start", "argos-assistant", "휴지", map[string]string{"source": "test"})
	if err != nil {
		t.Fatal(err)
	}
	if !isAssistantRecentReportsRequest("최근 작업 보여줘") {
		t.Fatal("recent reports request was not detected")
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "최근 작업 보여줘")
	for _, want := range []string{"Argos 최근 보고", "요약: 최근 3개", "구매 자동 진행", "증거 ID: " + assistantEvidenceDisplayID(purchaseRecord.ID), "뉴스 브리핑", "증거 ID: " + assistantEvidenceDisplayID(newsRecord.ID), "결과: 뉴스 브리핑 완료", "URL 열기 요청", "증거 ID: " + assistantEvidenceDisplayID(openRecord.ID), "결과: URL 열기 요청", "google.com", "읽기 전용"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("recent reports reply missing %q:\n%s", want, reply)
		}
	}
	for _, raw := range []string{openRecord.StoredAt, newsRecord.StoredAt, purchaseRecord.StoredAt, "assistant-news", "open_url", "assistant-direct-purchase-approved-start", "https://www.google.com/search", "%EA%B8%B4"} {
		if raw != "" && strings.Contains(reply, raw) {
			t.Fatalf("recent reports should hide raw evidence/source value %q:\n%s", raw, reply)
		}
	}
	if strings.Contains(reply, "/.meshclaw/evidence/") {
		t.Fatalf("recent reports should hide raw long URLs:\n%s", reply)
	}
	for _, blocked := range []string{"실행했습니다", "발송했습니다", "삭제했습니다", "배포했습니다"} {
		if strings.Contains(reply, blocked) {
			t.Fatalf("recent reports reply should be read-only, found %q:\n%s", blocked, reply)
		}
	}
}

func TestAssistantRecentReportsReplyFiltersByShortEvidenceID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_DASHBOARD_URL", "https://argos.example/argos/dashboard.html")
	openRecord, err := evidence.Store("open_url", "assistant", "https://www.google.com/search?tbm=shop&q=%EA%B8%B4%EC%A3%BC%EC%86%8C", map[string]string{"source": "test"})
	if err != nil {
		t.Fatal(err)
	}
	newsRecord, err := evidence.Store("assistant-news", "assistant", "뉴스 브리핑 완료", map[string]string{"source": "test"})
	if err != nil {
		t.Fatal(err)
	}
	purchaseRecord, err := evidence.Store("assistant-direct-purchase-approved-start", "argos-assistant", "휴지", map[string]string{"source": "test"})
	if err != nil {
		t.Fatal(err)
	}
	shortID := assistantEvidenceDisplayID(purchaseRecord.ID)
	if !isAssistantRecentReportsRequest(shortID) {
		t.Fatalf("bare short evidence ID should be treated as a recent reports request: %s", shortID)
	}
	if !isAssistantRecentReportsRequest("증거 ID " + shortID) {
		t.Fatalf("Korean evidence ID lookup should be treated as a recent reports request: %s", shortID)
	}
	if !isAssistantRecentReportsRequest("evidence ID " + strings.ToLower(shortID)) {
		t.Fatalf("English lowercase evidence ID lookup should be treated as a recent reports request: %s", shortID)
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "최근 작업 보여줘 "+shortID)
	for _, want := range []string{
		"Argos 최근 보고",
		"필터: 증거 ID " + shortID,
		"요약: 최근 1개",
		"구매 자동 진행",
		"증거 ID: " + shortID,
		"결과: 휴지",
		"Evidence 탭: " + assistantDashboardEvidenceURL(shortID),
		"읽기 전용",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("filtered recent reports reply missing %q:\n%s", want, reply)
		}
	}
	for _, unwanted := range []string{
		assistantEvidenceDisplayID(openRecord.ID),
		assistantEvidenceDisplayID(newsRecord.ID),
		"뉴스 브리핑",
		"URL 열기 요청",
		openRecord.StoredAt,
		newsRecord.StoredAt,
		purchaseRecord.StoredAt,
		"/.meshclaw/evidence/",
	} {
		if unwanted != "" && strings.Contains(reply, unwanted) {
			t.Fatalf("filtered recent reports reply should not expose %q:\n%s", unwanted, reply)
		}
	}
	bare := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, shortID)
	if !strings.Contains(bare, "필터: 증거 ID "+shortID) || !strings.Contains(bare, "구매 자동 진행") {
		t.Fatalf("bare short evidence ID should filter recent reports:\n%s", bare)
	}
	english := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "evidence ID "+strings.ToLower(shortID))
	for _, want := range []string{"Argos recent reports.", "Filter: evidence ID " + shortID, "Automatic purchase", "Evidence ID: " + shortID, "Evidence tab: " + assistantDashboardEvidenceURL(shortID), "This was read-only"} {
		if !strings.Contains(english, want) {
			t.Fatalf("English evidence ID lookup missing %q:\n%s", want, english)
		}
	}
}

func TestSignalEvidenceRefUsesShortDisplayID(t *testing.T) {
	record := evidence.Record{
		ID:       "20260618T065123Z-1234567890-assistant-direct-purchase-approved-start",
		StoredAt: "/Users/example/.meshclaw/evidence/2026-06-18/20260618T065123Z-1234567890-assistant-direct-purchase-approved-start.json",
	}
	got := signalEvidenceRef(record)
	if got != "20260618T065123Z-1234567890" {
		t.Fatalf("signal evidence ref = %q", got)
	}
	for _, raw := range []string{record.ID, record.StoredAt, "/Users/", ".json"} {
		if strings.Contains(got, raw) {
			t.Fatalf("signal evidence ref should not expose raw value %q: %q", raw, got)
		}
	}
}

func TestFormatSignalFrontendsReportUsesEvidenceDashboardLink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_DASHBOARD_URL", "https://argos.example/argos/dashboard.html")
	record, err := evidence.Store("assistant-frontends", "argos-assistant", "frontends", map[string]string{"source": "test"})
	if err != nil {
		t.Fatal(err)
	}
	shortID := assistantEvidenceDisplayID(record.ID)
	reply := formatSignalFrontendsReport(osauto.FrontendsReport{
		Frontends: []osauto.FrontendStatus{{
			Provider:  "claude",
			App:       "Claude",
			Available: true,
		}},
	}, record, nil)
	for _, want := range []string{
		"월정액/로그인 프론트엔드 상태입니다.",
		"- claude: 사용 가능 (Claude)",
		"증거 ID: " + shortID,
		"Evidence 탭: " + assistantDashboardEvidenceURL(shortID),
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("frontends report missing %q:\n%s", want, reply)
		}
	}
	for _, raw := range []string{record.ID, record.StoredAt, home, "/.meshclaw/evidence/", ".json"} {
		if raw != "" && strings.Contains(reply, raw) {
			t.Fatalf("frontends report should hide raw evidence value %q:\n%s", raw, reply)
		}
	}
}

func TestFormatSignalAIHandoffUsesEvidenceDashboardLink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_DASHBOARD_URL", "https://argos.example/argos/dashboard.html")
	record, err := evidence.Store("assistant-ai-handoff", "argos-assistant", "claude handoff", map[string]string{"source": "test"})
	if err != nil {
		t.Fatal(err)
	}
	shortID := assistantEvidenceDisplayID(record.ID)
	reply := formatSignalAIHandoff(osauto.Result{
		OK:       true,
		Provider: "claude",
		App:      "Claude",
		URL:      "https://claude.ai",
	}, record, nil)
	for _, want := range []string{
		"CLAUDE로 작업을 넘길 준비를 했습니다.",
		"앱: Claude",
		"증거 ID: " + shortID,
		"Evidence 탭: " + assistantDashboardEvidenceURL(shortID),
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("AI handoff report missing %q:\n%s", want, reply)
		}
	}
	for _, raw := range []string{record.ID, record.StoredAt, home, "/.meshclaw/evidence/", ".json"} {
		if raw != "" && strings.Contains(reply, raw) {
			t.Fatalf("AI handoff report should hide raw evidence value %q:\n%s", raw, reply)
		}
	}
}

func TestAssistantRecentReportsReplyEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_ARGOS_DASHBOARD_URL", "dashboard")
	openRecord, err := evidence.Store("open_url", "assistant", "https://www.google.com/search?tbm=shop&q=toilet+paper", map[string]string{"source": "test"})
	if err != nil {
		t.Fatal(err)
	}
	newsRecord, err := evidence.Store("assistant-news", "assistant", "News briefing complete", map[string]string{"source": "test"})
	if err != nil {
		t.Fatal(err)
	}
	purchaseRecord, err := evidence.Store("assistant-direct-purchase-approved-start", "argos-assistant", "toilet paper", map[string]string{"source": "test"})
	if err != nil {
		t.Fatal(err)
	}
	if !isAssistantRecentReportsRequest("recent reports") {
		t.Fatal("English recent reports request was not detected")
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "recent reports")
	for _, want := range []string{
		"Argos recent reports.",
		"Summary: latest 3 records",
		"Automatic purchase",
		"Evidence ID: " + assistantEvidenceDisplayID(purchaseRecord.ID),
		"News briefing",
		"Evidence ID: " + assistantEvidenceDisplayID(newsRecord.ID),
		"Result: News briefing complete",
		"URL open request",
		"Evidence ID: " + assistantEvidenceDisplayID(openRecord.ID),
		"Result: URL open request",
		"www.google.com",
		"Dashboard: dashboard",
		"This was read-only",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English recent reports reply missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English recent reports reply should not expose Korean:\n%s", reply)
	}
	for _, raw := range []string{openRecord.StoredAt, newsRecord.StoredAt, purchaseRecord.StoredAt, "assistant-news", "open_url", "assistant-direct-purchase-approved-start", "https://www.google.com/search", "toilet+paper"} {
		if raw != "" && strings.Contains(reply, raw) {
			t.Fatalf("English recent reports should hide raw evidence/source value %q:\n%s", raw, reply)
		}
	}
	if strings.Contains(reply, "/.meshclaw/evidence/") {
		t.Fatalf("English recent reports should hide raw long URLs:\n%s", reply)
	}
	for _, blocked := range []string{"executed", "sent", "deleted", "deployed", "purchase complete", "order complete"} {
		if strings.Contains(strings.ToLower(reply), blocked) && !strings.Contains(reply, "Nothing was executed, sent, deleted, or deployed") {
			t.Fatalf("English recent reports reply should be read-only, found %q:\n%s", blocked, reply)
		}
	}
}

func TestAssistantChannelStatusReplySeparatesSignalAndWebhookBoundary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	if _, _, err := UpsertTarget(Target{ID: "argos-assistant", Channel: "signal", GroupID: "group-assistant", Label: "비서방", Mode: "assistant"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := UpsertTarget(Target{ID: "argos-chat", Channel: "signal", GroupID: "group-chat", Label: "채팅방", Mode: "chat"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}
	if !isAssistantChannelStatusRequest("웹훅 상태") || !isAssistantChannelStatusRequest("채널 현황") {
		t.Fatal("channel status request was not detected")
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "웹훅 상태 알려줘")
	for _, want := range []string{"Argos 채널 상태", "Signal targets", "interactive", "one-way/no-reply", "대화형", "보고 전용", "Gateway", "Signal은 열림", "Webhook 상태: 닫힘", "공개 수신 endpoint", "열기 전 조건", "/argos/webhook/events", "HMAC 서명", "rate limit", "payload 최대 크기", "기본 dry-run", "예시 이벤트", "메일 초안", "일정 후보", "서버 조치 요청", "승인 초안", "수신 흐름", "외부 도구", "Signal 승인", "별도 실행", "raw token/password 금지", "Guard/vault handle", "수신만으로 메일 발송", "Dashboard는 관찰", "Signal은 승인", "webhook receiver 열기 승인 요청", "관제센터", "읽기 전용"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("channel status reply missing %q:\n%s", want, reply)
		}
	}
	if lines := strings.Count(reply, "\n") + 1; lines > 12 {
		t.Fatalf("channel status reply should stay mobile-sized, got %d lines:\n%s", lines, reply)
	}
	for _, blocked := range []string{"채널을 만들었습니다", "웹훅을 열었습니다", "수신 서버를 시작했습니다", "intent/evidence/approval", "execute 단계"} {
		if strings.Contains(reply, blocked) {
			t.Fatalf("channel status must be read-only, found %q:\n%s", blocked, reply)
		}
	}
}

func TestAssistantNextScheduleDueIncludesLocalAIBriefing(t *testing.T) {
	now := time.Date(2026, 6, 12, 19, 4, 0, 0, time.UTC)
	nextJob, nextDue, ok := assistantNextScheduleDue(map[string]time.Time{
		"mail-watch":           now.Add(-1 * time.Minute),
		"local-ai-briefing":    time.Date(2026, 6, 12, 18, 7, 30, 0, time.UTC),
		"assistant-watch":      now.Add(-10 * time.Minute),
		"argos-health":         now.Add(-10 * time.Minute),
		"assistant-auto-check": now.Add(-10 * time.Minute),
		"serverops-quickcheck": now.Add(-10 * time.Minute),
	}, now)
	if !ok || nextJob != "local-ai-briefing" {
		t.Fatalf("next=%q due=%s ok=%t", nextJob, nextDue, ok)
	}
	if got, want := nextDue.Sub(now), 3*time.Minute; got != want {
		t.Fatalf("next due in %s, want %s", got, want)
	}
}

func TestAssistantScheduleDueCount(t *testing.T) {
	now := time.Date(2026, 6, 12, 19, 30, 0, 0, time.UTC)
	got := assistantScheduleDueCount(map[string]time.Time{
		"mail-watch":           now.Add(-16 * time.Minute),
		"local-ai-briefing":    now.Add(-59 * time.Minute),
		"assistant-watch":      now.Add(-61 * time.Minute),
		"argos-health":         now.Add(-10 * time.Minute),
		"assistant-auto-check": now.Add(-10 * time.Minute),
		"serverops-quickcheck": now.Add(-10 * time.Minute),
	}, now)
	if got != 2 {
		t.Fatalf("due count=%d, want 2", got)
	}
}

func TestAssistantAutomationStatusReplyExplainsMissingDevWorkerArtifact(t *testing.T) {
	home := t.TempDir()
	schedulePath := filepath.Join(home, ".meshclaw", "schedule-state.json")
	if err := os.MkdirAll(filepath.Dir(schedulePath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(schedulePath, []byte(`{"last_run":{"assistant-auto-check":"2026-06-12T14:03:00Z"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MESHCLAW_SCHEDULE_STATE", schedulePath)
	t.Setenv("MESHCLAW_DEV_WORKER_LATEST_JSON", filepath.Join(home, "missing-dev-worker.json"))

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "자동화 상태 알려줘")
	for _, want := range []string{"Codex dev-worker", "MacBook 개발 루프", "Mac mini는 Signal 런타임", "정상"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("missing dev-worker artifact reply missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "아직 생성 전") {
		t.Fatalf("reply should not imply the automation has not run:\n%s", reply)
	}
}

func TestSignalCapabilityReplyChat(t *testing.T) {
	reply, handled := signalCapabilityReply("chat", "뭘 할 수 있어")
	if !handled {
		t.Fatal("capability question was not handled")
	}
	if !strings.Contains(reply, "일반 질문") || !strings.Contains(reply, "Assistant 방") {
		t.Fatalf("unexpected chat help:\n%s", reply)
	}
}

func TestReportRoomsDoNotAutoReply(t *testing.T) {
	event := IncomingMessage{Source: "+821012345678", Redacted: "뭘 할 수 있어?"}
	if reply := guardReply(
		ListenOptions{TargetID: "argos-briefing", Mode: "briefing"},
		Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Mode: "briefing"},
		event,
	); reply != "" {
		t.Fatalf("briefing report room should be push-only, got %q", reply)
	}
	if reply := guardReply(
		ListenOptions{TargetID: "report-room", Mode: "ops"},
		Target{ID: "report-room", Channel: "signal", GroupID: "group-ops", Mode: "ops"},
		event,
	); reply != "" {
		t.Fatalf("ops report room should be push-only, got %q", reply)
	}
	if reply := guardReply(
		ListenOptions{TargetID: "argos-ops", Mode: "assistant"},
		Target{ID: "argos-ops", Channel: "signal", GroupID: "group-ops", Mode: "ops"},
		event,
	); reply != "" {
		t.Fatalf("argos ops report room must ignore global assistant mode override, got %q", reply)
	}
	if !isOneWayReportTarget(
		ListenOptions{TargetID: "custom-report", Mode: "assistant"},
		Target{ID: "custom-report", Channel: "signal", GroupID: "group-report", Mode: "briefing"},
		"assistant",
	) {
		t.Fatal("target briefing mode should remain one-way even when listen opts request assistant mode")
	}
}

func TestAssistantConversationalGreeting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	reply, handled := assistantInteractiveReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "안녕")
	if !handled {
		t.Fatal("greeting should be handled without model")
	}
	for _, want := range []string{"여기 있습니다", "뉴스", "무엇부터"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
}

func TestAssistantConversationalVagueMailSend(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	reply, handled := assistantInteractiveReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "메일 보내줘")
	if !handled {
		t.Fatal("vague mail send should be handled")
	}
	for _, want := range []string{"보낼 계정", "받는 사람", "제목", "본문", "승인"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
}

func TestAssistantConversationalEnglishVagueMailSend(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	for _, input := range []string{"send mail", "mail please"} {
		reply, handled := assistantInteractiveReply(ListenOptions{TargetID: "argos-assistant-" + strings.ReplaceAll(input, " ", "-"), Mode: "assistant"}, input)
		if !handled {
			t.Fatalf("vague English mail send should be handled for %q", input)
		}
		for _, want := range []string{"보낼 계정", "받는 사람", "제목", "본문"} {
			if !strings.Contains(reply, want) {
				t.Fatalf("reply for %q missing %q:\n%s", input, want, reply)
			}
		}
	}
}

func TestAssistantPendingMailAccountNumberUsesConfiguredAccount(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MAIL_CONFIG", filepath.Join(home, "mail.json"))
	t.Setenv("FIRST_MAIL_PASSWORD", "one")
	t.Setenv("SECOND_MAIL_PASSWORD", "two")

	if _, _, err := mailadapter.UpsertAccount(mailadapter.Account{
		ID:          "first",
		Backend:     "imap",
		Email:       "first@example.com",
		Host:        "imap.example.com",
		Port:        993,
		SMTPHost:    "smtp.example.com",
		SMTPPort:    587,
		Username:    "first@example.com",
		PasswordEnv: "FIRST_MAIL_PASSWORD",
		TLS:         true,
		SMTPTLS:     true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := mailadapter.UpsertAccount(mailadapter.Account{
		ID:          "second",
		Backend:     "imap",
		Email:       "second@example.com",
		Host:        "imap.example.com",
		Port:        993,
		SMTPHost:    "smtp.example.com",
		SMTPPort:    587,
		Username:    "second@example.com",
		PasswordEnv: "SECOND_MAIL_PASSWORD",
		TLS:         true,
		SMTPTLS:     true,
	}); err != nil {
		t.Fatal(err)
	}

	target := "mail-account-number"
	reply, handled := assistantInteractiveReply(ListenOptions{TargetID: target, Mode: "assistant"}, "send mail")
	if !handled || !strings.Contains(reply, "메일을 보내려면") {
		t.Fatalf("initial reply=%q handled=%t", reply, handled)
	}
	reply, handled = assistantInteractiveReply(ListenOptions{TargetID: target, Mode: "assistant"}, "to nobody@example.com subject: assistant test body: this is a signal test")
	if !handled || !strings.Contains(reply, "남은 항목: 보낼 계정") {
		t.Fatalf("slot reply=%q handled=%t", reply, handled)
	}
	reply, handled = assistantInteractiveReply(ListenOptions{TargetID: target, Mode: "assistant"}, "1")
	if !handled {
		t.Fatal("account choice was not handled")
	}
	if !strings.Contains(reply, "메일 초안을 만들었습니다") ||
		!strings.Contains(reply, "to: nobody@example.com") ||
		!strings.Contains(reply, "subject: assistant test") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantScenarioNaturalKoreanMailDraftBeatsURLDetection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MAIL_CONFIG", filepath.Join(home, "mail.json"))
	if _, _, err := mailadapter.UpsertAccount(mailadapter.Account{
		ID:          "101",
		Backend:     "imap",
		Email:       "101@101.band",
		Host:        "imap.example.com",
		Port:        993,
		SMTPHost:    "smtp.example.com",
		SMTPPort:    587,
		Username:    "101@101.band",
		PasswordEnv: "MAIL_PASSWORD",
		TLS:         true,
		SMTPTLS:     true,
	}); err != nil {
		t.Fatal(err)
	}

	reply, handled := assistantInteractiveReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "101 계정으로 nobody@example.com한테 테스트 메일 초안 하나 만들어줘. 제목은 아르고스 자연어 테스트, 내용은 시그널에서 말로 시켜서 초안을 만든다는 내용이야. 보내지는 말고 초안만 만들어줘.")
	if !handled {
		t.Fatal("natural mail draft was not handled")
	}
	if strings.Contains(reply, "open_url") || strings.Contains(reply, "주소를 열었습니다") {
		t.Fatalf("mail draft became URL open:\n%s", reply)
	}
	for _, want := range []string{"메일 초안을 만들었습니다", "to: nobody@example.com", "subject: 아르고스 자연어 테스트", "시그널에서 말로 시켜서 초안을 만든다는 내용이야"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
}

func TestAssistantConversationalNextSuggestion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	reply, handled := assistantInteractiveReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "다음은?")
	if !handled {
		t.Fatal("next question should be handled")
	}
	for _, want := range []string{"그대로 보내세요", "최근 메일 요약", "회의자료", "시장조사", "여행 후보"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("next suggestion missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "원하는 번호") {
		t.Fatalf("unexpected reply:\n%s", reply)
	}
}

func TestAssistantSelfTestRequest(t *testing.T) {
	for _, input := range []string{"기능 테스트", "비서 테스트", "뉴스 테스트", "assistant self test"} {
		if !isAssistantSelfTestRequest(input) {
			t.Fatalf("expected self-test request: %q", input)
		}
	}
	if isAssistantSelfTestRequest("오늘 주요뉴스 3개만 알려줘") {
		t.Fatal("normal news request should not become self-test")
	}
	if isAssistantSelfTestRequest("to nobody@example.com subject: assistant test body: this is a signal test") {
		t.Fatal("mail slot text should not become self-test")
	}
}

func TestFormatAssistantSelfTestIsMobileCompact(t *testing.T) {
	got := formatAssistantSelfTest(signalAssistantSelfTest{
		Items: []signalSelfTestItem{
			{Name: "뉴스", Status: "ok", Detail: "2개 수집", DurationMS: 120},
			{Name: "Mac 권한", Status: "warn", Detail: "항상 허용된 작업 없음"},
			{Name: "기능 매트릭스", Status: "ok", Detail: "9개 대표 라우팅 OK"},
			{Name: "Signal 답변 계약", Status: "ok", Detail: "본문/첨부 계약 OK"},
		},
	})
	if !strings.Contains(got, "Argos Assistant 기능 테스트") ||
		!strings.Contains(got, "뉴스: OK") ||
		!strings.Contains(got, "Mac 권한: 주의") ||
		!strings.Contains(got, "기능 매트릭스: OK") ||
		!strings.Contains(got, "Signal 답변 계약: OK") ||
		!strings.Contains(got, "요약: OK 3개, 주의 1개") {
		t.Fatalf("unexpected self-test format:\n%s", got)
	}
	if strings.Contains(got, "evidence:") {
		t.Fatalf("formatter should not include evidence by itself:\n%s", got)
	}
}

func TestAssistantSelfTestIncludesCapabilityAndReplyContract(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	report := runAssistantSelfTest(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "")
	byName := map[string]signalSelfTestItem{}
	for _, item := range report.Items {
		byName[item.Name] = item
	}
	for _, name := range []string{"기능 매트릭스", "Signal 답변 계약"} {
		item, ok := byName[name]
		if !ok {
			t.Fatalf("self-test missing %s: %#v", name, report.Items)
		}
		if item.Status != "ok" {
			t.Fatalf("%s status=%s detail=%s", name, item.Status, item.Detail)
		}
	}
	got := formatAssistantSelfTest(report)
	if !strings.Contains(got, "기능 매트릭스: OK") ||
		!strings.Contains(got, "Signal 답변 계약: OK") {
		t.Fatalf("formatted self-test missing contract rows:\n%s", got)
	}
}

func TestAssistantScenarioMapEnglishQueryDropsCommandWords(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := mapsScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "show map for Gwanghwamun Kyobo Bookstore")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if strings.Contains(reply, "query=show+") || strings.Contains(reply, "+for+") ||
		!strings.Contains(reply, "query=Gwanghwamun+Kyobo+Bookstore") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantScenarioDirectionsLink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := mapsScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "강남역에서 성수역까지 길찾기")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "https://www.google.com/maps/dir/") ||
		!strings.Contains(reply, "origin=") ||
		!strings.Contains(reply, "destination=") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantScenarioDirectionsCapturePlanUsesMeshClawProof(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := mapsScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "강남역에서 서울역 길안내 링크를 주고 지도 캡쳐는 실제 실행하지 말고 캡쳐 계획만 보여줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "https://www.google.com/maps/dir/") ||
		!strings.Contains(reply, "MeshClaw") ||
		!strings.Contains(reply, "maps_proof") ||
		!strings.Contains(reply, "승인") ||
		strings.Contains(reply, "볼륨") ||
		strings.Contains(reply, "전원") {
		t.Fatalf("reply should prefer MeshClaw proof plan, got=%q", reply)
	}
}

func TestAssistantScenarioTravelTimeLink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := mapsScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "강남역에서 서울역까지 지금 얼마나 걸려?")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "이동시간 확인 링크") ||
		!strings.Contains(reply, "https://www.google.com/maps/dir/") ||
		!strings.Contains(reply, "travelmode=transit") ||
		strings.Contains(reply, "%EC%96%BC%EB%A7%88%EB%82%98") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantScenarioTravelTimeWithoutKkajiLink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := mapsScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "강남역에서 서울역 이동시간 알려줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "이동시간 확인 링크") ||
		!strings.Contains(reply, "origin=") ||
		!strings.Contains(reply, "destination=") ||
		strings.Contains(reply, "reminder_create") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantScenarioTravelTimeNeedsSharedLocation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := mapsScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "여기서 서울역까지 얼마나 걸려?")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "위치 공유") || !strings.Contains(reply, "출발지") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantScenarioCommuteDirectionsLink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := commuteScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "분당에서 광화문까지 출근길 알려줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "https://www.google.com/maps/dir/") ||
		!strings.Contains(reply, "travelmode=transit") ||
		!strings.Contains(reply, "출퇴근 길찾기") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantScenarioCommuteAsksForRoute(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := commuteScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "출근길 상황 알려줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "출발지와 목적지") || !strings.Contains(reply, "분당에서 광화문까지") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantScenarioNearbyOpenNowNeedsLocation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := mapsScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "근처 문 연 약국 찾아줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "기준 위치가 필요") || !strings.Contains(reply, "강남역 근처 문 연 약국") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantScenarioNearbyOpenNowWithLocation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := mapsScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "강남역 근처 문 연 약국 찾아줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "https://www.google.com/maps/search/") ||
		!strings.Contains(reply, "open+now") ||
		!strings.Contains(reply, "장소 검색 링크") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantScenarioParkingNeedsPlace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := mapsScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "그 식당 주차 되는지 봐줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "장소명이 필요") || !strings.Contains(reply, "주차") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantScenarioParkingLink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := mapsScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "성수동 난포 주차 되는지 봐줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "주차 정보 확인 링크") ||
		!strings.Contains(reply, "https://www.google.com/maps/search/") ||
		!strings.Contains(reply, "parking") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantScenarioRestaurantBookingPreparesButDoesNotSubmit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := bookingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "내일 저녁 7시에 강남 파스타 식당 2명 예약해줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "예약 후보") ||
		!strings.Contains(reply, "https://www.google.com/search") ||
		!strings.Contains(reply, "마지막 단계에서는 멈춘 뒤") {
		t.Fatalf("reply=%q", reply)
	}
}

func legacyTestAssistantScenarioCalendarMentionBeatsBookingKeyword(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_ASSISTANT_TOOL_LOOP", "0")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "아르고스, 내일 오후에 병원 예약 관련해서 전화해야 해. 일단 내일 오후 3시에 '병원 전화하기'로 캘린더에 넣어줘.")
	handled := strings.TrimSpace(reply) != ""
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "calendar_event_create") ||
		!strings.Contains(reply, "병원 전화하기") ||
		strings.Contains(reply, "예약 후보") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantScenarioMediaChoiceStaysDeterministic(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := mediaScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "음악 틀어줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "어디에서 음악을 틀까요?") ||
		!strings.Contains(reply, "YouTube") ||
		!strings.Contains(reply, "iPhone") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantScenarioShoppingStopsBeforePayment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "이 상품 바로 결제해서 사줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "구매 승인 한 번만 받겠습니다.") ||
		!strings.Contains(reply, "상품: 현재 화면 상품") ||
		!strings.Contains(reply, "`구매 승인`, `ㅇㅇ`, `ㄱㄱ`, 또는 `ㅇㅋ`") ||
		!strings.Contains(reply, "최종 주문 클릭") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantReplyShortKoreanDirectPurchaseAsksOneApproval(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
	targetID := "argos-assistant-direct-purchase-short"

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "휴지사")
	for _, want := range []string{
		"구매 승인 한 번만 받겠습니다.",
		"상품: 휴지",
		"`구매 승인`, `ㅇㅇ`, `ㄱㄱ`, 또는 `ㅇㅋ`",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("direct purchase reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"https://www.google.com/search", "Google Shopping", "한 번 더 명시적으로 승인"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("direct purchase reply should not contain %q:\n%s", bad, reply)
		}
	}
	pending, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false)
	if !ok || pending.Kind != "shopping_direct_purchase" || pending.Intent.Query != "휴지" {
		t.Fatalf("pending direct purchase not stored correctly: %#v ok=%v", pending, ok)
	}
}

func TestAssistantReplyBareNaturalApprovalContinuesPendingDirectPurchase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
	targetID := "argos-assistant-direct-purchase-bare-approval"

	first := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "휴지 구매해")
	if !strings.Contains(first, "상품: 휴지") {
		t.Fatalf("initial direct purchase reply did not store item:\n%s", first)
	}
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "사줘")
	for _, want := range []string{
		"`휴지` 구매 승인을 받았습니다.",
		"현재 실행 모드가 아니어서 클릭은 하지 않았습니다.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("bare natural approval reply missing %q:\n%s", want, reply)
		}
	}
	if _, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false); ok {
		t.Fatalf("pending direct purchase should be consumed after bare approval")
	}
}

func TestAssistantReplyEnglishBareNaturalApprovalContinuesPendingDirectPurchase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
	targetID := "argos-assistant-direct-purchase-en-bare-approval"

	first := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "Order me toilet paper on Coupang")
	if !strings.Contains(first, "Item: toilet paper") {
		t.Fatalf("initial English direct purchase reply did not store item:\n%s", first)
	}
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "buy it")
	for _, want := range []string{
		"Purchase approval received for `toilet paper`.",
		"Execution mode is off, so no click was made.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English bare natural approval reply missing %q:\n%s", want, reply)
		}
	}
	if _, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false); ok {
		t.Fatalf("English pending direct purchase should be consumed after bare approval")
	}
}

func TestAssistantReplyPendingDirectPurchaseAcceptsRetryPhrase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
	targetID := "argos-assistant-direct-purchase-retry-phrase"

	first := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "휴지 구매해")
	if !strings.Contains(first, "상품: 휴지") {
		t.Fatalf("initial direct purchase reply did not store item:\n%s", first)
	}
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "다시 해봐")
	for _, want := range []string{
		"`휴지` 구매 승인을 받았습니다.",
		"현재 실행 모드가 아니어서 클릭은 하지 않았습니다.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("retry phrase reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"이어서 할 일을 한 문장으로 보내세요", "구매 승인 한 번만 받겠습니다.", "google.com/search"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("retry phrase should continue pending purchase, not fallback/restart %q:\n%s", bad, reply)
		}
	}
	if _, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false); ok {
		t.Fatalf("pending direct purchase should be consumed after retry phrase")
	}
}

func TestAssistantReplyPendingDirectPurchaseAcceptsEnglishRetryPhrase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
	targetID := "argos-assistant-direct-purchase-en-retry-phrase"

	first := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "Order me toilet paper on Coupang")
	if !strings.Contains(first, "Item: toilet paper") {
		t.Fatalf("initial English direct purchase reply did not store item:\n%s", first)
	}
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "try again")
	for _, want := range []string{
		"Purchase approval received for `toilet paper`.",
		"Execution mode is off, so no click was made.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English retry phrase reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"Send the next thing to do in one sentence", "Purchase approval will be requested once", "google.com/search"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("English retry phrase should continue pending purchase, not fallback/restart %q:\n%s", bad, reply)
		}
	}
	if _, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false); ok {
		t.Fatalf("English pending direct purchase should be consumed after retry phrase")
	}
}

func TestAssistantReplyKoreanDirectPurchaseVerbVariantsAskOneApproval(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")

	cases := []struct {
		name  string
		input string
		query string
	}{
		{name: "compact purchase", input: "휴지구입해줘", query: "휴지"},
		{name: "bring buy", input: "생수 사다줘", query: "생수"},
		{name: "order spaced", input: "고양이 모래 주문해줘", query: "고양이 모래"},
		{name: "polite order", input: "휴지 주문해주세요", query: "휴지"},
		{name: "polite buy", input: "생수 500ml 20개 사줘요", query: "생수 500ml 20개"},
		{name: "put order", input: "쿠팡에서 생수 주문 넣어줘", query: "생수"},
		{name: "make buy", input: "휴지 사지게 해", query: "휴지"},
		{name: "make order", input: "생수 주문되게 해줘", query: "생수"},
		{name: "make purchase compact", input: "햇반구매되게해줘", query: "햇반"},
		{name: "softener filler", input: "휴지 좀 사줘", query: "휴지"},
		{name: "attached softener filler", input: "쿠팡에서 휴지좀 사줘", query: "휴지"},
		{name: "urgency fillers", input: "지금 바로 휴지 빨리 주문해줘", query: "휴지"},
		{name: "object particle", input: "쿠팡에서 휴지를 사줘", query: "휴지"},
		{name: "quantity object particle", input: "햇반 10개를 주문해줘", query: "햇반 10개"},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			targetID := fmt.Sprintf("argos-assistant-direct-purchase-variant-%d", i)
			reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, tc.input)
			for _, want := range []string{
				"구매 승인 한 번만 받겠습니다.",
				"상품: " + tc.query,
				"`구매 승인`, `ㅇㅇ`, `ㄱㄱ`, 또는 `ㅇㅋ`",
			} {
				if !strings.Contains(reply, want) {
					t.Fatalf("direct purchase variant reply missing %q:\n%s", want, reply)
				}
			}
			for _, bad := range []string{"https://www.google.com/search", "Google Shopping", "검색 후에는 리뷰/배송비/총액을 비교"} {
				if strings.Contains(reply, bad) {
					t.Fatalf("direct purchase variant should not fall back to generic search %q:\n%s", bad, reply)
				}
			}
			pending, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false)
			if !ok || pending.Kind != "shopping_direct_purchase" || pending.Intent.Query != tc.query {
				t.Fatalf("pending direct purchase variant not stored correctly: %#v ok=%v", pending, ok)
			}
		})
	}
}

func TestAssistantReplyEnglishDirectPurchaseVerbVariantsAskOneApproval(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")

	cases := []struct {
		name  string
		input string
		query string
	}{
		{name: "order me", input: "Order me toilet paper on Coupang", query: "toilet paper"},
		{name: "please purchase", input: "Please purchase toilet paper from Coupang", query: "toilet paper"},
		{name: "can buy me", input: "Can you buy me toilet paper on Coupang?", query: "toilet paper"},
		{name: "urgency fillers", input: "Please buy me toilet paper right now on Coupang", query: "toilet paper"},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			targetID := fmt.Sprintf("argos-assistant-direct-purchase-en-variant-%d", i)
			reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, tc.input)
			for _, want := range []string{
				"I will ask for purchase approval once.",
				"Item: " + tc.query,
				"`purchase approved`, `yes`, `go`, or `ok`",
			} {
				if !strings.Contains(reply, want) {
					t.Fatalf("English direct purchase variant reply missing %q:\n%s", want, reply)
				}
			}
			for _, bad := range []string{"Live purchase request workflow.", "Started Coupang candidate comparison.", "https://www.google.com/search"} {
				if strings.Contains(reply, bad) {
					t.Fatalf("English direct purchase variant should not fall back to workflow/search %q:\n%s", bad, reply)
				}
			}
			pending, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false)
			if !ok || pending.Kind != "shopping_direct_purchase" || pending.Intent.Query != tc.query {
				t.Fatalf("English pending direct purchase variant not stored correctly: %#v ok=%v", pending, ok)
			}
		})
	}
}

func TestAssistantReplyCurrentVisibleProductDirectPurchaseAskOneApproval(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")

	targetID := "argos-assistant-current-visible-product"
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "Buy this item on Coupang")
	for _, want := range []string{
		"I will ask for purchase approval once.",
		"Item: current visible item",
		"`purchase approved`, `yes`, `go`, or `ok`",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("current visible product reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"coupang.com/np/search", "Started Coupang candidate comparison.", "https://www.google.com/search"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("current visible product should not open/search %q:\n%s", bad, reply)
		}
	}
	pending, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false)
	if !ok || pending.Kind != "shopping_direct_purchase" || !isDirectPurchaseCurrentVisibleProductQuery(pending.Intent.Query) {
		t.Fatalf("current visible product pending not stored correctly: %#v ok=%v", pending, ok)
	}
}

func TestAssistantReplyCurrentVisibleProductDirectPurchaseExecuteSkipsSearch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	targetID := "argos-assistant-current-visible-product-execute"

	oldCapture := shoppingCheckoutScreenCapture
	oldOCR := shoppingCheckoutScreenOCR
	oldOpenURL := shoppingDirectPurchaseOpenURL
	defer func() {
		shoppingCheckoutScreenCapture = oldCapture
		shoppingCheckoutScreenOCR = oldOCR
		shoppingDirectPurchaseOpenURL = oldOpenURL
	}()
	opened := false
	shoppingDirectPurchaseOpenURL = func(ctx context.Context, rawURL string) osauto.Result {
		opened = true
		return osauto.Result{Kind: "meshclaw_automation_open_url", Action: "open_url", URL: rawURL, OK: true}
	}
	captureCalls := 0
	shoppingCheckoutScreenCapture = func(ctx context.Context, output string) osauto.Result {
		captureCalls++
		if err := os.WriteFile(output, []byte("fake current product proof"), 0600); err != nil {
			t.Fatal(err)
		}
		return osauto.Result{Kind: "meshclaw_automation_screen_capture", Action: "screen_capture", OK: true}
	}
	clicks := 0
	shoppingCheckoutScreenOCR = func(path string) (string, string) {
		switch {
		case clicks == 0:
			return strings.Join([]string{
				"쿠팡 상품 상세",
				"휴지 30롤",
				"로켓배송",
				"내일 도착",
				"바로구매",
			}, "\n"), ""
		case clicks == 1:
			return strings.Join([]string{
				"상품명: 휴지 30롤",
				"옵션/수량: 1개",
				"총 결제 금액: 12,900원",
				"도착 예정일: 내일 도착",
				"배송지 표시: 맞음",
				"결제수단 표시: 맞음",
				"최종 주문 버튼 위치: x=1100, y=780",
				"결제하기",
			}, "\n"), ""
		default:
			return strings.Join([]string{
				"주문이 완료되었습니다",
				"주문번호: 123456789012",
				"내일 도착",
			}, "\n"), ""
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/click" {
			t.Fatalf("unexpected runner path: %s", r.URL.Path)
		}
		clicks++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	t.Setenv("MESHCLAW_ARGOS_UI_RUNNER_URL", server.URL)

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant", Execute: true}, "이 상품 사줘")
	if opened {
		t.Fatalf("current visible product direct purchase should not open a new search:\n%s", reply)
	}
	if clicks < 2 {
		t.Fatalf("expected product buy and final-order clicks, got %d:\n%s", clicks, reply)
	}
	if captureCalls < 5 {
		t.Fatalf("expected current screen reads through proof, got %d:\n%s", captureCalls, reply)
	}
	for _, want := range []string{
		"최종 구매 클릭을 실행했습니다.",
		"상품: 휴지 30롤 / 1개",
		"총액: 12,900원",
		"주문번호: ********9012",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("current visible product execute reply missing %q:\n%s", want, reply)
		}
	}
}

func TestAssistantReplyCurrentVisibleProductPendingApprovalExecutesWithoutSearch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
	targetID := "argos-assistant-current-visible-product-pending"

	first := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "이거 주문해줘")
	for _, want := range []string{
		"구매 승인 한 번만 받겠습니다.",
		"상품: 현재 화면 상품",
		"`구매 승인`, `ㅇㅇ`, `ㄱㄱ`, 또는 `ㅇㅋ`",
	} {
		if !strings.Contains(first, want) {
			t.Fatalf("current visible product pending reply missing %q:\n%s", want, first)
		}
	}
	pending, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false)
	if !ok || pending.Kind != "shopping_direct_purchase" || !isDirectPurchaseCurrentVisibleProductQuery(pending.Intent.Query) {
		t.Fatalf("current visible product pending not stored correctly: %#v ok=%v", pending, ok)
	}

	oldCapture := shoppingCheckoutScreenCapture
	oldOCR := shoppingCheckoutScreenOCR
	oldOpenURL := shoppingDirectPurchaseOpenURL
	defer func() {
		shoppingCheckoutScreenCapture = oldCapture
		shoppingCheckoutScreenOCR = oldOCR
		shoppingDirectPurchaseOpenURL = oldOpenURL
	}()
	opened := false
	shoppingDirectPurchaseOpenURL = func(ctx context.Context, rawURL string) osauto.Result {
		opened = true
		return osauto.Result{Kind: "meshclaw_automation_open_url", Action: "open_url", URL: rawURL, OK: true}
	}
	shoppingCheckoutScreenCapture = func(ctx context.Context, output string) osauto.Result {
		if err := os.WriteFile(output, []byte("fake pending current product proof"), 0600); err != nil {
			t.Fatal(err)
		}
		return osauto.Result{Kind: "meshclaw_automation_screen_capture", Action: "screen_capture", OK: true}
	}
	clicks := 0
	shoppingCheckoutScreenOCR = func(path string) (string, string) {
		if clicks == 0 {
			return "쿠팡 상품 상세\n휴지 30롤\n로켓배송\n내일 도착\n바로구매", ""
		}
		if clicks == 1 {
			return "상품명: 휴지 30롤\n옵션/수량: 1개\n총 결제 금액: 12,900원\n도착 예정일: 내일 도착\n배송지 표시: 맞음\n결제수단 표시: 맞음\n최종 주문 버튼 위치: x=1100, y=780\n결제하기", ""
		}
		return "주문이 완료되었습니다\n주문번호: 123456789012\n내일 도착", ""
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clicks++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	t.Setenv("MESHCLAW_ARGOS_UI_RUNNER_URL", server.URL)

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant", Execute: true}, "ㄱㄱ")
	if opened {
		t.Fatalf("current visible product pending approval should not open search:\n%s", reply)
	}
	if clicks < 2 {
		t.Fatalf("expected current product buy and final-order clicks, got %d:\n%s", clicks, reply)
	}
	for _, want := range []string{"최종 구매 클릭을 실행했습니다.", "상품: 휴지 30롤 / 1개", "총액: 12,900원", "주문번호: ********9012"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("current visible product pending approval reply missing %q:\n%s", want, reply)
		}
	}
	if _, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false); ok {
		t.Fatalf("current visible product pending should be consumed after approval")
	}
}

func TestAssistantReplyPendingDirectPurchaseCanRetryFromCurrentScreen(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
	targetID := "argos-assistant-direct-purchase-current-screen-retry"

	first := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "휴지 구매해")
	if !strings.Contains(first, "상품: 휴지") {
		t.Fatalf("initial direct purchase did not store concrete item:\n%s", first)
	}
	pending, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false)
	if !ok || pending.Kind != "shopping_direct_purchase" || pending.Intent.Query != "휴지" {
		t.Fatalf("pending direct purchase not stored before current-screen retry: %#v ok=%v", pending, ok)
	}

	oldCapture := shoppingCheckoutScreenCapture
	oldOCR := shoppingCheckoutScreenOCR
	oldOpenURL := shoppingDirectPurchaseOpenURL
	defer func() {
		shoppingCheckoutScreenCapture = oldCapture
		shoppingCheckoutScreenOCR = oldOCR
		shoppingDirectPurchaseOpenURL = oldOpenURL
	}()
	opened := false
	shoppingDirectPurchaseOpenURL = func(ctx context.Context, rawURL string) osauto.Result {
		opened = true
		return osauto.Result{Kind: "meshclaw_automation_open_url", Action: "open_url", URL: rawURL, OK: true}
	}
	shoppingCheckoutScreenCapture = func(ctx context.Context, output string) osauto.Result {
		if err := os.WriteFile(output, []byte("fake current-screen retry proof"), 0600); err != nil {
			t.Fatal(err)
		}
		return osauto.Result{Kind: "meshclaw_automation_screen_capture", Action: "screen_capture", OK: true}
	}
	ocrCalls := 0
	shoppingCheckoutScreenOCR = func(path string) (string, string) {
		ocrCalls++
		if ocrCalls == 1 {
			return strings.Join([]string{
				"상품명: 휴지 30롤",
				"옵션/수량: 1개",
				"총 결제 금액: 12,900원",
				"도착 예정일: 내일 도착",
				"배송지 표시: 맞음",
				"결제수단 표시: 맞음",
				"최종 주문 버튼 위치: x=1100, y=780",
				"결제하기",
			}, "\n"), ""
		}
		return "주문이 완료되었습니다\n주문번호: 123456789012\n내일 도착\n로켓배송", ""
	}
	clicked := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/click" {
			t.Fatalf("unexpected runner path: %s", r.URL.Path)
		}
		clicked = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	t.Setenv("MESHCLAW_ARGOS_UI_RUNNER_URL", server.URL)

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant", Execute: true}, "현재 화면에서 다시 해봐")
	if opened {
		t.Fatalf("current-screen retry should not open a new search/product URL:\n%s", reply)
	}
	if !clicked {
		t.Fatalf("current-screen retry did not click final order from readable screen:\n%s", reply)
	}
	for _, want := range []string{"최종 구매 클릭을 실행했습니다.", "상품: 휴지 30롤 / 1개", "총액: 12,900원", "주문번호: ********9012"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("current-screen retry reply missing %q:\n%s", want, reply)
		}
	}
	if _, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false); ok {
		t.Fatalf("pending direct purchase should be consumed after current-screen retry")
	}
}

func TestShoppingDirectPurchaseCurrentScreenFollowUpRejectsStopPhrases(t *testing.T) {
	for _, input := range []string{
		"이거 진행하지 마",
		"현재 화면에서 하지마",
		"do not continue from this screen",
		"cancel this screen",
	} {
		if shoppingDirectPurchaseFollowUpUsesCurrentScreen(input) {
			t.Fatalf("current-screen retry should reject stop phrase %q", input)
		}
	}
}

func TestDirectPurchaseAdvanceIgnoresHeaderLoginOnActionableScreens(t *testing.T) {
	cases := []struct {
		name string
		ocr  string
		kind string
	}{
		{
			name: "search result with login header",
			ocr: strings.Join([]string{
				"쿠팡",
				"로그인",
				"검색 결과",
				"휴지 로켓배송",
				"관련도순",
			}, "\n"),
			kind: "search_result",
		},
		{
			name: "product detail with login header",
			ocr: strings.Join([]string{
				"쿠팡",
				"로그인",
				"휴지 30롤",
				"로켓배송",
				"내일 도착",
				"바로구매",
			}, "\n"),
			kind: "product_buy",
		},
		{
			name: "coupang product list without explicit search result label",
			ocr: strings.Join([]string{
				"Safari",
				"coupang.com",
				"전체 휴지 휴지 화장지 두루마리휴지 키친타올",
				"대왕롤앤롤 라벤더 3겹 고급롤화장지, 30m, 30개입, 2개",
				"44% 22,800원",
				"판매자로켓",
				"내일 내일(화) 도착",
				"무료배송",
				"최대 1,140원 적립",
				"광고",
				"깨끗한나라 순수 시그니처 천연펄프 3겹 롤화장지",
			}, "\n"),
			kind: "search_result",
		},
		{
			name: "coupang search result with cart header",
			ocr: strings.Join([]string{
				"쿠팡",
				"장바구니",
				"휴지",
				"탐사 클래식 천연펄프 3겹 롤화장지 30m, 30롤",
				"13,490원",
				"로켓배송",
				"내일 도착 보장",
				"무료배송",
				"별점 4.5",
				"리뷰 12,345",
			}, "\n"),
			kind: "search_result",
		},
		{
			name: "product detail with cart add CTA is not mistaken for search result",
			ocr: strings.Join([]string{
				"쿠팡",
				"장바구니",
				"탐사 클래식 천연펄프 3겹 롤화장지 30m, 30롤",
				"13,490원",
				"로켓배송 내일 도착",
				"장바구니 담기",
			}, "\n"),
			kind: "product_cart_add",
		},
		{
			name: "cart continue checkout CTA",
			ocr: strings.Join([]string{
				"장바구니",
				"탐사 클래식 천연펄프 3겹 롤화장지 30m, 30롤",
				"수량 1",
				"총 상품금액 12,900원",
				"구매 계속하기",
			}, "\n"),
			kind: "cart_checkout",
		},
		{
			name: "product buy continue CTA",
			ocr: strings.Join([]string{
				"쿠팡",
				"탐사 클래식 천연펄프 3겹 롤화장지 30m, 30롤",
				"로켓배송 내일 도착",
				"12,900원",
				"구매 계속하기",
			}, "\n"),
			kind: "product_buy",
		},
		{
			name: "cart added short confirmation",
			ocr: strings.Join([]string{
				"쿠팡",
				"탐사 클래식 천연펄프 3겹 롤화장지 30m, 30롤",
				"장바구니 담김",
				"장바구니 보기",
			}, "\n"),
			kind: "cart_added_open_cart",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			action, ok := inferDirectPurchaseScreenAction(tc.ocr, "")
			if !ok {
				t.Fatalf("expected actionable direct-purchase screen for OCR:\n%s", tc.ocr)
			}
			if action.Kind != tc.kind || action.Blocker != "" {
				t.Fatalf("expected kind=%q without blocker, got %#v", tc.kind, action)
			}
		})
	}
}

func TestDirectPurchaseAdvanceBlocksActualLoginForm(t *testing.T) {
	action, ok := inferDirectPurchaseScreenAction(strings.Join([]string{
		"쿠팡 로그인",
		"이메일 또는 휴대폰 번호",
		"비밀번호",
		"로그인",
	}, "\n"), "")
	if !ok {
		t.Fatal("expected login form to be recognized as a blocker")
	}
	if action.Kind != "blocked" || action.Blocker != directPurchaseBlockerLogin {
		t.Fatalf("expected login/password blocker, got %#v", action)
	}
	if got := directPurchaseBlockerDisplayFor("ko", action.Blocker); got != "로그인/비밀번호" {
		t.Fatalf("expected Korean login blocker display, got %q", got)
	}
	if got := directPurchaseBlockerDisplayFor("en", action.Blocker); got != "login/password required" {
		t.Fatalf("expected English login blocker display, got %q", got)
	}
}

func TestDirectPurchaseAdvanceBlocksMacOSPermissionPrompt(t *testing.T) {
	action, ok := inferDirectPurchaseScreenAction(strings.Join([]string{
		"쿠팡",
		"휴지",
		"탐사 클래식 천연펄프 3겹 롤화장지",
		"장바구니 담기",
		"\"meshclaw\"에서 \"Finder\"을(를) 제어하려고 합니다.",
		"허용 안 함",
		"허용",
	}, "\n"), "")
	if !ok {
		t.Fatal("expected macOS permission prompt to be recognized as a blocker")
	}
	if action.Kind != "blocked" || action.Blocker != directPurchaseBlockerMacOSPermission {
		t.Fatalf("expected macOS permission blocker, got %#v", action)
	}
	if got := directPurchaseBlockerDisplayFor("ko", action.Blocker); got != "macOS 권한 확인 팝업" {
		t.Fatalf("expected Korean blocker display, got %q", got)
	}
	if got := directPurchaseBlockerDisplayFor("en", action.Blocker); got != "macOS permission prompt" {
		t.Fatalf("expected English blocker display, got %q", got)
	}
}

func TestDirectPurchaseHumanActionBlockersUseLanguagePack(t *testing.T) {
	cases := []struct {
		name string
		ocr  string
		code string
		ko   string
		en   string
	}{
		{
			name: "captcha",
			ocr:  "쿠팡\n보안문자\n로봇이 아닙니다",
			code: directPurchaseBlockerCaptcha,
			ko:   "CAPTCHA/보안문자",
			en:   "CAPTCHA/security-code challenge",
		},
		{
			name: "identity",
			ocr:  "본인인증\n휴대폰 인증\n인증번호 입력",
			code: directPurchaseBlockerIdentity,
			ko:   "본인인증",
			en:   "identity verification",
		},
		{
			name: "payment",
			ocr:  "결제 비밀번호\n보안코드\nCVC 입력",
			code: directPurchaseBlockerPaymentAuth,
			ko:   "결제 인증",
			en:   "payment authentication",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			action, ok := inferDirectPurchaseScreenAction(tc.ocr, "")
			if !ok {
				t.Fatalf("expected blocker for OCR:\n%s", tc.ocr)
			}
			if action.Kind != "blocked" || action.Blocker != tc.code {
				t.Fatalf("expected blocker=%q, got %#v", tc.code, action)
			}
			if got := directPurchaseBlockerDisplayFor("ko", action.Blocker); got != tc.ko {
				t.Fatalf("Korean blocker display=%q, want %q", got, tc.ko)
			}
			if got := directPurchaseBlockerDisplayFor("en", action.Blocker); got != tc.en {
				t.Fatalf("English blocker display=%q, want %q", got, tc.en)
			}
			if assistantCheckoutPrepContainsHangul(directPurchaseBlockerDisplayFor("en", action.Blocker)) {
				t.Fatalf("English blocker display leaked Korean: %q", directPurchaseBlockerDisplayFor("en", action.Blocker))
			}
		})
	}
}

func TestDirectPurchaseBlockerHintsUseLanguagePack(t *testing.T) {
	cases := []struct {
		name       string
		blocker    string
		koContains string
		enContains string
	}{
		{
			name:       "macos_permission",
			blocker:    directPurchaseBlockerMacOSPermission,
			koContains: "macOS 권한 팝업에서 허용",
			enContains: "Allow the macOS permission prompt",
		},
		{
			name:       "not_ready",
			blocker:    directPurchaseBlockerNotReady,
			koContains: "현재 화면의 상품/총액/배송/결제수단",
			enContains: "Confirm the visible item, total, delivery, and payment method",
		},
		{
			name:       "captcha",
			blocker:    directPurchaseBlockerCaptcha,
			koContains: "CAPTCHA/보안문자만 직접 통과",
			enContains: "Complete only the CAPTCHA/security-code challenge",
		},
		{
			name:       "identity",
			blocker:    directPurchaseBlockerIdentity,
			koContains: "휴대폰/본인 인증만 직접 완료",
			enContains: "Complete only the phone/identity verification",
		},
		{
			name:       "login",
			blocker:    directPurchaseBlockerLogin,
			koContains: "쿠팡 로그인만 직접 완료",
			enContains: "Complete the Coupang login only",
		},
		{
			name:       "payment_auth",
			blocker:    directPurchaseBlockerPaymentAuth,
			koContains: "카드 CVC/결제 비밀번호",
			enContains: "Enter only the card CVC/payment password prompt",
		},
		{
			name:       "legacy_payment_auth",
			blocker:    "결제 인증",
			koContains: "카드 CVC/결제 비밀번호",
			enContains: "Enter only the card CVC/payment password prompt",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ko := directPurchaseBlockerUserHintFor("ko", tc.blocker)
			if !strings.Contains(ko, tc.koContains) {
				t.Fatalf("Korean blocker hint=%q, want contains %q", ko, tc.koContains)
			}
			en := directPurchaseBlockerUserHintFor("en", tc.blocker)
			if !strings.Contains(en, tc.enContains) {
				t.Fatalf("English blocker hint=%q, want contains %q", en, tc.enContains)
			}
			if assistantCheckoutPrepContainsHangul(en) {
				t.Fatalf("English blocker hint leaked Korean: %q", en)
			}
		})
	}
}

func TestDirectPurchaseScreenActionLabelUsesLanguagePack(t *testing.T) {
	cases := []struct {
		kind string
		ko   string
		en   string
	}{
		{kind: "search_result", ko: "첫 번째 상품", en: "first product"},
		{kind: "reorder_buy_again", ko: "재구매/다시 구매 버튼", en: "buy-again/reorder button"},
		{kind: "cart_added_open_cart", ko: "장바구니 페이지", en: "cart page"},
		{kind: "product_cart_add", ko: "장바구니 담기", en: "add-to-cart"},
		{kind: "cart_checkout", ko: "주문/결제 진행 버튼", en: "checkout button"},
		{kind: "product_buy", ko: "바로구매/구매 버튼", en: "buy-now/purchase button"},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			action := directPurchaseScreenAction{Kind: tc.kind, Label: "내부 한국어 라벨"}
			if got := directPurchaseScreenActionLabelFor("ko", action); got != tc.ko {
				t.Fatalf("Korean label=%q, want %q", got, tc.ko)
			}
			gotEN := directPurchaseScreenActionLabelFor("en", action)
			if gotEN != tc.en {
				t.Fatalf("English label=%q, want %q", gotEN, tc.en)
			}
			if assistantCheckoutPrepContainsHangul(gotEN) {
				t.Fatalf("English action label leaked Hangul: %q", gotEN)
			}
		})
	}
}

func TestDirectPurchaseBlockedReplyDetectionSurvivesLocaleMismatch(t *testing.T) {
	koEvidenceID := "20260618T070124Z-123456789"
	koReply := formatShoppingDirectPurchaseBlockedReplyFor("ko", "휴지", []string{
		"자동 진행 blocker: macOS 권한 확인 팝업",
		"진행 증거 ID: " + koEvidenceID,
	}, directPurchaseBlockerMacOSPermission)
	if !isShoppingDirectPurchaseBlockedReply("en", "toilet paper", koReply) {
		t.Fatalf("expected Korean blocked reply to be detected despite locale/query mismatch:\n%s", koReply)
	}
	if !strings.Contains(koReply, "다음 재시도: 새 검색을 열지 않고 현재 화면이나 선택한 상품 화면을 먼저 읽고") {
		t.Fatalf("Korean blocked reply should expose concrete retry plan:\n%s", koReply)
	}
	if !strings.Contains(koReply, "진행 증거 확인: `최근 작업 보여줘 "+koEvidenceID+"` 또는 관제센터 Evidence에서 ID `"+koEvidenceID+"`를 검색하세요.") {
		t.Fatalf("Korean blocked reply should expose evidence check command:\n%s", koReply)
	}
	enEvidenceID := "20260618T070125Z-987654321"
	enReply := formatShoppingDirectPurchaseBlockedReplyFor("en", "toilet paper", []string{
		"Automatic execution blocker: macOS permission prompt",
		"Execution evidence ID: " + enEvidenceID,
	}, directPurchaseBlockerMacOSPermission)
	if !isShoppingDirectPurchaseBlockedReply("ko", "휴지", enReply) {
		t.Fatalf("expected English blocked reply to be detected despite locale/query mismatch:\n%s", enReply)
	}
	if !strings.Contains(enReply, "Next retry: I read the current screen or selected product screen first") ||
		assistantCheckoutPrepContainsHangul(enReply) {
		t.Fatalf("English blocked reply should expose localized concrete retry plan:\n%s", enReply)
	}
	if !strings.Contains(enReply, "Evidence check: send `recent reports "+enEvidenceID+"` or search ID `"+enEvidenceID+"` in the Operations Center Evidence tab.") {
		t.Fatalf("English blocked reply should expose localized evidence check command:\n%s", enReply)
	}
	if isShoppingDirectPurchaseBlockedReply("ko", "휴지", "좋습니다.\n이어서 할 일을 한 문장으로 보내세요.") {
		t.Fatal("generic assistant reply should not be treated as a blocked direct-purchase reply")
	}
}

func TestCheckoutScreenPointSizeDoesNotUseFinderBoundsByDefault(t *testing.T) {
	t.Setenv("MESHCLAW_CHECKOUT_FINDER_BOUNDS", "")
	if allowFinderDesktopBoundsForCheckout() {
		t.Fatal("Finder desktop bounds fallback should be disabled by default")
	}
	width, height := checkoutScreenPointSize("")
	if width != 1440 || height != 900 {
		t.Fatalf("blank capture should use safe default size without Finder fallback, got %dx%d", width, height)
	}

	t.Setenv("MESHCLAW_CHECKOUT_FINDER_BOUNDS", "1")
	if !allowFinderDesktopBoundsForCheckout() {
		t.Fatal("explicit Finder bounds opt-in should be honored")
	}
}

func TestAssistantReplyShortApprovalExecutesPendingDirectPurchase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
	targetID := "argos-assistant-direct-purchase-execute"

	oldCapture := shoppingCheckoutScreenCapture
	oldOCR := shoppingCheckoutScreenOCR
	oldOpenURL := shoppingDirectPurchaseOpenURL
	defer func() {
		shoppingCheckoutScreenCapture = oldCapture
		shoppingCheckoutScreenOCR = oldOCR
		shoppingDirectPurchaseOpenURL = oldOpenURL
	}()
	openedURL := ""
	shoppingDirectPurchaseOpenURL = func(ctx context.Context, rawURL string) osauto.Result {
		openedURL = rawURL
		return osauto.Result{Kind: "meshclaw_automation_open_url", Action: "open_url", URL: rawURL, OK: true}
	}
	captureCalls := 0
	shoppingCheckoutScreenCapture = func(ctx context.Context, output string) osauto.Result {
		captureCalls++
		if err := os.WriteFile(output, []byte("fake screen"), 0600); err != nil {
			t.Fatal(err)
		}
		return osauto.Result{Kind: "meshclaw_automation_screen_capture", Action: "screen_capture", OK: true}
	}
	clicks := 0
	ocrCalls := 0
	shoppingCheckoutScreenOCR = func(path string) (string, string) {
		ocrCalls++
		switch {
		case clicks == 0:
			return strings.Join([]string{
				"쿠팡 검색 결과",
				"휴지 로켓배송",
				"관련도순",
			}, "\n"), ""
		case clicks == 1:
			return strings.Join([]string{
				"휴지 30롤",
				"로켓배송",
				"내일 도착",
				"바로구매",
			}, "\n"), ""
		case clicks == 2:
			return strings.Join([]string{
				"상품명: 휴지 30롤",
				"옵션/수량: 1개",
				"총 결제 금액: 12,900원",
				"도착 예정일: 내일 도착",
				"배송지 표시: 맞음",
				"결제수단 표시: 맞음",
				"최종 주문 버튼 위치: x=1100, y=780",
				"결제하기",
			}, "\n"), ""
		default:
			return strings.Join([]string{
				"주문이 완료되었습니다",
				"주문번호: 123456789012",
				"내일 도착",
				"쿠팡 로켓배송",
			}, "\n"), ""
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/click" {
			t.Fatalf("unexpected runner path: %s", r.URL.Path)
		}
		clicks++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	t.Setenv("MESHCLAW_ARGOS_UI_RUNNER_URL", server.URL)

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant", Execute: true}, "휴지사")
	if !strings.Contains(openedURL, "coupang.com/np/search") || !strings.Contains(openedURL, "%ED%9C%B4%EC%A7%80") {
		t.Fatalf("pending approval did not open Coupang search for tissue, opened=%q", openedURL)
	}
	if clicks < 3 {
		t.Fatalf("expected search/product/final clicks, got %d:\n%s", clicks, reply)
	}
	if captureCalls < 7 {
		t.Fatalf("expected screen reads through post-click proof, got %d:\n%s", captureCalls, reply)
	}
	for _, want := range []string{
		"최종 구매 클릭을 실행했습니다.",
		"상품: 휴지 30롤 / 1개",
		"총액: 12,900원",
		"주문 결과 화면에서 읽은 내용:",
		"주문번호: ********9012",
		"실행 증거:",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("pending direct purchase execute missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"/.meshclaw/evidence/", "/Users/", ".json"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("pending direct purchase execute should not expose raw evidence path %q:\n%s", bad, reply)
		}
	}
	if _, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false); ok {
		t.Fatalf("pending direct purchase should be consumed after approval")
	}
}

func TestAssistantReplyShortApprovalDirectPurchasePrefersCartAdd(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
	targetID := "argos-assistant-direct-purchase-cart-add"

	oldCapture := shoppingCheckoutScreenCapture
	oldOCR := shoppingCheckoutScreenOCR
	oldOpenURL := shoppingDirectPurchaseOpenURL
	defer func() {
		shoppingCheckoutScreenCapture = oldCapture
		shoppingCheckoutScreenOCR = oldOCR
		shoppingDirectPurchaseOpenURL = oldOpenURL
	}()
	openedURLs := []string{}
	cartOpened := false
	shoppingDirectPurchaseOpenURL = func(ctx context.Context, rawURL string) osauto.Result {
		openedURLs = append(openedURLs, rawURL)
		if strings.Contains(rawURL, "cart.coupang.com") {
			cartOpened = true
		}
		return osauto.Result{Kind: "meshclaw_automation_open_url", Action: "open_url", URL: rawURL, OK: true}
	}
	captureCalls := 0
	shoppingCheckoutScreenCapture = func(ctx context.Context, output string) osauto.Result {
		captureCalls++
		if err := os.WriteFile(output, []byte("fake direct purchase cart-add screen"), 0600); err != nil {
			t.Fatal(err)
		}
		return osauto.Result{Kind: "meshclaw_automation_screen_capture", Action: "screen_capture", OK: true}
	}
	clicks := 0
	shoppingCheckoutScreenOCR = func(path string) (string, string) {
		switch {
		case clicks == 0:
			return "쿠팡 검색 결과\n휴지 로켓배송\n관련도순", ""
		case clicks == 1:
			return strings.Join([]string{
				"쿠팡 상품 상세",
				"휴지 30롤",
				"로켓배송",
				"내일 도착",
				"장바구니 담기",
				"바로구매",
			}, "\n"), ""
		case clicks == 2 && !cartOpened:
			return "장바구니에 상품이 담겼습니다\n장바구니 보기", ""
		case clicks == 2 && cartOpened:
			return "쿠팡 장바구니\n휴지 30롤\n구매하기", ""
		case clicks == 3:
			return strings.Join([]string{
				"상품명: 휴지 30롤",
				"옵션/수량: 1개",
				"총 결제 금액: 12,900원",
				"도착 예정일: 내일 도착",
				"배송지 표시: 맞음",
				"결제수단 표시: 맞음",
				"최종 주문 버튼 위치: x=1100, y=780",
				"결제하기",
			}, "\n"), ""
		default:
			return "주문이 완료되었습니다\n주문번호: 123456789012\n내일 도착\n쿠팡 로켓배송", ""
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/click" {
			t.Fatalf("unexpected runner path: %s", r.URL.Path)
		}
		clicks++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	t.Setenv("MESHCLAW_ARGOS_UI_RUNNER_URL", server.URL)

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant", Execute: true}, "휴지사")
	if len(openedURLs) < 2 || !strings.Contains(openedURLs[0], "coupang.com/np/search") || !strings.Contains(openedURLs[len(openedURLs)-1], "cart.coupang.com/cartView.pang") {
		t.Fatalf("expected direct purchase to open search then cart, opened=%#v reply=\n%s", openedURLs, reply)
	}
	if clicks < 4 {
		t.Fatalf("expected search result, cart add, cart checkout, and final clicks, got %d:\n%s", clicks, reply)
	}
	if captureCalls < 9 {
		t.Fatalf("expected screen reads through cart-add path and proof, got %d:\n%s", captureCalls, reply)
	}
	for _, want := range []string{
		"장바구니 담기 버튼을 클릭했습니다.",
		"장바구니 페이지를 열었습니다: https://cart.coupang.com/cartView.pang",
		"주문/결제 진행 버튼 버튼을 클릭했습니다.",
		"최종 구매 클릭을 실행했습니다.",
		"주문번호: ********9012",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("direct purchase cart-add reply missing %q:\n%s", want, reply)
		}
	}
}

func TestAssistantReplyShortApprovalDirectPurchaseOpensProductCandidate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-direct-purchase-product-candidate"
	productURL := "https://www.coupang.com/vp/products/1234567890?itemId=111&vendorItemId=222"

	oldCapture := shoppingCheckoutScreenCapture
	oldOCR := shoppingCheckoutScreenOCR
	oldOpenURL := shoppingDirectPurchaseOpenURL
	oldCandidates := shoppingDirectPurchaseProductCandidates
	defer func() {
		shoppingCheckoutScreenCapture = oldCapture
		shoppingCheckoutScreenOCR = oldOCR
		shoppingDirectPurchaseOpenURL = oldOpenURL
		shoppingDirectPurchaseProductCandidates = oldCandidates
	}()
	candidateQueries := []string{}
	shoppingDirectPurchaseProductCandidates = func(ctx context.Context, query string, limit int) []browserauto.Link {
		candidateQueries = append(candidateQueries, query)
		return []browserauto.Link{{Text: "휴지 30롤 로켓배송", URL: productURL}}
	}
	openedURLs := []string{}
	cartOpened := false
	shoppingDirectPurchaseOpenURL = func(ctx context.Context, rawURL string) osauto.Result {
		openedURLs = append(openedURLs, rawURL)
		if strings.Contains(rawURL, "cart.coupang.com") {
			cartOpened = true
		}
		return osauto.Result{Kind: "meshclaw_automation_open_url", Action: "open_url", URL: rawURL, OK: true}
	}
	captureCalls := 0
	shoppingCheckoutScreenCapture = func(ctx context.Context, output string) osauto.Result {
		captureCalls++
		if err := os.WriteFile(output, []byte("fake direct product candidate screen"), 0600); err != nil {
			t.Fatal(err)
		}
		return osauto.Result{Kind: "meshclaw_automation_screen_capture", Action: "screen_capture", OK: true}
	}
	clicks := 0
	shoppingCheckoutScreenOCR = func(path string) (string, string) {
		switch {
		case clicks == 0:
			return strings.Join([]string{
				"쿠팡 상품 상세",
				"휴지 30롤 로켓배송",
				"내일 도착",
				"장바구니 담기",
				"바로구매",
			}, "\n"), ""
		case clicks == 1 && !cartOpened:
			return "장바구니에 상품이 담겼습니다\n장바구니 보기", ""
		case clicks == 1 && cartOpened:
			return "쿠팡 장바구니\n휴지 30롤 로켓배송\n구매하기", ""
		case clicks == 2:
			return strings.Join([]string{
				"상품명: 휴지 30롤 로켓배송",
				"옵션/수량: 1개",
				"총 결제 금액: 12,900원",
				"도착 예정일: 내일 도착",
				"배송지 표시: 맞음",
				"결제수단 표시: 맞음",
				"최종 주문 버튼 위치: x=1100, y=780",
				"결제하기",
			}, "\n"), ""
		default:
			return "주문이 완료되었습니다\n주문번호: 123456789012\n내일 도착\n쿠팡 로켓배송", ""
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/click" {
			t.Fatalf("unexpected runner path: %s", r.URL.Path)
		}
		clicks++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	t.Setenv("MESHCLAW_ARGOS_UI_RUNNER_URL", server.URL)

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant", Execute: true}, "휴지사")
	if len(candidateQueries) != 1 || candidateQueries[0] != "휴지" {
		t.Fatalf("expected product candidate search for tissue, got %#v", candidateQueries)
	}
	if len(openedURLs) < 2 || openedURLs[0] != productURL || strings.Contains(openedURLs[0], "/np/search") || !strings.Contains(openedURLs[len(openedURLs)-1], "cart.coupang.com/cartView.pang") {
		t.Fatalf("expected direct purchase to open product candidate then cart, opened=%#v reply=\n%s", openedURLs, reply)
	}
	if clicks < 3 {
		t.Fatalf("expected cart add, cart checkout, and final clicks without search-result click, got %d:\n%s", clicks, reply)
	}
	if captureCalls < 7 {
		t.Fatalf("expected screen reads through product candidate path and proof, got %d:\n%s", captureCalls, reply)
	}
	for _, want := range []string{
		"쿠팡 상품 상세를 열었습니다: " + productURL,
		"장바구니 담기 버튼을 클릭했습니다.",
		"장바구니 페이지를 열었습니다: https://cart.coupang.com/cartView.pang",
		"최종 구매 클릭을 실행했습니다.",
		"주문번호: ********9012",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("direct purchase product candidate reply missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "첫 번째 상품 버튼을 클릭했습니다.") {
		t.Fatalf("product candidate path should not click generic first search result:\n%s", reply)
	}
}

func TestAssistantReplyShortApprovalDirectPurchaseUsesRecentProductCandidate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-direct-purchase-recent-product"
	productURL := "https://www.coupang.com/vp/products/987654321?itemId=333&vendorItemId=444"
	previous := coupangShoppingPrepReply("실행 상태 테스트", "휴지", browserauto.Link{
		Text: "휴지 30롤 로켓배송",
		URL:  productURL,
	})
	if err := appendSignalHistory(targetID, "쿠팡에서 휴지 후보 찾아줘", previous); err != nil {
		t.Fatal(err)
	}

	oldCapture := shoppingCheckoutScreenCapture
	oldOCR := shoppingCheckoutScreenOCR
	oldOpenURL := shoppingDirectPurchaseOpenURL
	oldCandidates := shoppingDirectPurchaseProductCandidates
	defer func() {
		shoppingCheckoutScreenCapture = oldCapture
		shoppingCheckoutScreenOCR = oldOCR
		shoppingDirectPurchaseOpenURL = oldOpenURL
		shoppingDirectPurchaseProductCandidates = oldCandidates
	}()
	liveSearchCalls := 0
	shoppingDirectPurchaseProductCandidates = func(ctx context.Context, query string, limit int) []browserauto.Link {
		liveSearchCalls++
		return []browserauto.Link{{Text: "다른 휴지", URL: "https://www.coupang.com/vp/products/111?itemId=1&vendorItemId=2"}}
	}
	openedURLs := []string{}
	cartOpened := false
	shoppingDirectPurchaseOpenURL = func(ctx context.Context, rawURL string) osauto.Result {
		openedURLs = append(openedURLs, rawURL)
		if strings.Contains(rawURL, "cart.coupang.com") {
			cartOpened = true
		}
		return osauto.Result{Kind: "meshclaw_automation_open_url", Action: "open_url", URL: rawURL, OK: true}
	}
	shoppingCheckoutScreenCapture = func(ctx context.Context, output string) osauto.Result {
		if err := os.WriteFile(output, []byte("fake recent product candidate screen"), 0600); err != nil {
			t.Fatal(err)
		}
		return osauto.Result{Kind: "meshclaw_automation_screen_capture", Action: "screen_capture", OK: true}
	}
	clicks := 0
	shoppingCheckoutScreenOCR = func(path string) (string, string) {
		switch {
		case clicks == 0:
			return "쿠팡 상품 상세\n휴지 30롤 로켓배송\n내일 도착\n장바구니 담기\n바로구매", ""
		case clicks == 1 && !cartOpened:
			return "장바구니에 상품이 담겼습니다\n장바구니 보기", ""
		case clicks == 1 && cartOpened:
			return "쿠팡 장바구니\n휴지 30롤 로켓배송\n구매하기", ""
		case clicks == 2:
			return strings.Join([]string{
				"상품명: 휴지 30롤 로켓배송",
				"옵션/수량: 1개",
				"총 결제 금액: 12,900원",
				"도착 예정일: 내일 도착",
				"배송지 표시: 맞음",
				"결제수단 표시: 맞음",
				"최종 주문 버튼 위치: x=1100, y=780",
				"결제하기",
			}, "\n"), ""
		default:
			return "주문이 완료되었습니다\n주문번호: 123456789012\n내일 도착\n쿠팡 로켓배송", ""
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/click" {
			t.Fatalf("unexpected runner path: %s", r.URL.Path)
		}
		clicks++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	t.Setenv("MESHCLAW_ARGOS_UI_RUNNER_URL", server.URL)

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant", Execute: true}, "휴지사")
	if liveSearchCalls != 0 {
		t.Fatalf("expected recent Signal candidate to avoid live product search, calls=%d reply=\n%s", liveSearchCalls, reply)
	}
	if len(openedURLs) < 2 || openedURLs[0] != productURL || !strings.Contains(openedURLs[len(openedURLs)-1], "cart.coupang.com/cartView.pang") {
		t.Fatalf("expected direct purchase to open recent product candidate then cart, opened=%#v reply=\n%s", openedURLs, reply)
	}
	for _, want := range []string{
		"쿠팡 상품 상세를 열었습니다: " + productURL,
		"장바구니 담기 버튼을 클릭했습니다.",
		"최종 구매 클릭을 실행했습니다.",
		"주문번호: ********9012",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("recent product candidate direct purchase missing %q:\n%s", want, reply)
		}
	}
}

func TestAssistantShoppingProductCandidateLinesSkipsNonProductCoupangLinks(t *testing.T) {
	candidates := assistantShoppingProductCandidateLines([]browserauto.Link{
		{Text: "쿠팡 홈", URL: "https://www.coupang.com/"},
		{Text: "로켓배송 캠페인", URL: "https://www.coupang.com/np/campaigns/82"},
		{Text: "휴지 30롤", URL: "https://www.coupang.com/vp/products/1234567890?itemId=111&vendorItemId=222"},
		{Text: "검색 결과", URL: "https://www.coupang.com/np/search?q=%ED%9C%B4%EC%A7%80"},
	}, 3)
	if len(candidates) != 1 {
		t.Fatalf("expected one product candidate, got %#v", candidates)
	}
	if candidates[0].Text == "" || !strings.Contains(candidates[0].URL, "/vp/products/") {
		t.Fatalf("unexpected product candidate: %#v", candidates[0])
	}
}

func TestAssistantReplyCoupangTabCleanupCommandClosesOnlyCoupangTabs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_DASHBOARD_URL", "https://argos.example/argos/dashboard.html")
	targetID := "argos-assistant-coupang-tab-cleanup"

	oldCleanup := shoppingBrowserTabCleanup
	defer func() {
		shoppingBrowserTabCleanup = oldCleanup
	}()
	var gotHosts []string
	shoppingBrowserTabCleanup = func(ctx context.Context, hosts []string) osauto.Result {
		gotHosts = append([]string{}, hosts...)
		return osauto.Result{Kind: "meshclaw_automation_browser_tab_cleanup", Action: "browser_tab_cleanup", OK: true, Stdout: "closed_tabs=7"}
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant", Execute: true}, "쿠팡 탭 정리해")
	for _, want := range []string{
		"쿠팡 브라우저 탭 정리입니다.",
		"닫은 쿠팡 탭: 7개",
		"로그인 쿠키/비밀번호/저장 세션은 지우지 않습니다.",
		"정리 기록:",
		"Evidence 탭: https://argos.example/argos/dashboard.html?evidence=",
		"#evidence",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("cleanup reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{home, "/.meshclaw/evidence/", ".json"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("cleanup reply should not expose raw evidence path %q:\n%s", bad, reply)
		}
	}
	if strings.Join(gotHosts, ",") != "coupang.com" {
		t.Fatalf("cleanup hosts=%#v", gotHosts)
	}
}

func TestAssistantReplyDirectPurchaseCleansOldCoupangTabsBeforeOpeningSearch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
	targetID := "argos-assistant-direct-purchase-tab-cleanup"

	oldCapture := shoppingCheckoutScreenCapture
	oldOpenURL := shoppingDirectPurchaseOpenURL
	oldCleanup := shoppingBrowserTabCleanup
	defer func() {
		shoppingCheckoutScreenCapture = oldCapture
		shoppingDirectPurchaseOpenURL = oldOpenURL
		shoppingBrowserTabCleanup = oldCleanup
	}()
	cleanupCalled := false
	shoppingBrowserTabCleanup = func(ctx context.Context, hosts []string) osauto.Result {
		cleanupCalled = strings.Join(hosts, ",") == "coupang.com"
		return osauto.Result{Kind: "meshclaw_automation_browser_tab_cleanup", Action: "browser_tab_cleanup", OK: true, Stdout: "closed_tabs=3"}
	}
	openedURL := ""
	shoppingDirectPurchaseOpenURL = func(ctx context.Context, rawURL string) osauto.Result {
		openedURL = rawURL
		return osauto.Result{Kind: "meshclaw_automation_open_url", Action: "open_url", URL: rawURL, OK: true}
	}
	shoppingCheckoutScreenCapture = func(ctx context.Context, output string) osauto.Result {
		return osauto.Result{Kind: "meshclaw_automation_screen_capture", Action: "screen_capture", OK: false, Error: "screen unavailable"}
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant", Execute: true}, "휴지 구매해")
	if !cleanupCalled {
		t.Fatalf("direct purchase did not cleanup old Coupang tabs before opening search:\n%s", reply)
	}
	if !strings.Contains(openedURL, "coupang.com/np/search") {
		t.Fatalf("direct purchase did not open Coupang search, opened=%q reply=\n%s", openedURL, reply)
	}
}

func TestAssistantReplyDirectPurchaseBlockedKeepsPendingForRetry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
	targetID := "argos-assistant-direct-purchase-retry-after-blocker"

	oldCapture := shoppingCheckoutScreenCapture
	oldOpenURL := shoppingDirectPurchaseOpenURL
	defer func() {
		shoppingCheckoutScreenCapture = oldCapture
		shoppingDirectPurchaseOpenURL = oldOpenURL
	}()
	shoppingDirectPurchaseOpenURL = func(ctx context.Context, rawURL string) osauto.Result {
		return osauto.Result{Kind: "meshclaw_automation_open_url", Action: "open_url", URL: rawURL, OK: true}
	}
	shoppingCheckoutScreenCapture = func(ctx context.Context, output string) osauto.Result {
		return osauto.Result{Kind: "meshclaw_automation_screen_capture", Action: "screen_capture", OK: false, Error: "screen capture failed"}
	}

	start := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "휴지 구매해")
	if !strings.Contains(start, "상품: 휴지") {
		t.Fatalf("initial direct purchase reply did not store item:\n%s", start)
	}
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant", Execute: true}, "ㅇㅇ")
	for _, want := range []string{
		"`휴지` 구매 자동 진행이 멈췄습니다.",
		"화면 캡처 실패: screen capture failed",
		"`ㅇㅇ`, `다시 해봐`, `계속해`",
		"`현재 화면에서 다시 해봐`",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("blocked direct purchase reply missing %q:\n%s", want, reply)
		}
	}
	pending, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false)
	if !ok || pending.Kind != "shopping_direct_purchase" || pending.Intent.Query != "휴지" {
		t.Fatalf("blocked direct purchase should keep pending retry: %#v ok=%v", pending, ok)
	}
}

func TestAssistantScenarioCoupangRealPurchasePrepGivesConcreteTomorrowScript(t *testing.T) {
	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "내일은 쿠팡에서 진짜로 물건을 검색해서 구매를 할거야")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	for _, want := range []string{"쿠팡 실제 구매 테스트 준비", "쿠팡 열어줘", "로켓배송, 가격, 리뷰", "구매 직전 화면까지", "결제는 하지 마", "상품명, 옵션, 총액, 도착 예정일, 배송지 표시, 결제수단 표시", "Signal에는 표시 여부만 보냅니다", "구매 실행 승인", "비밀번호, 카드 CVC, 본인인증"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("prep reply missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "구매 완료") || strings.Contains(reply, "주문 완료") {
		t.Fatalf("prep reply must not claim purchase:\n%s", reply)
	}
}

func TestAssistantReplyCoupangRealPurchasePrepUsesScenarioRoute(t *testing.T) {
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "내일은 쿠팡에서 진짜로 물건을 검색해서 구매를 할거야")
	for _, want := range []string{"쿠팡 실제 구매 테스트 준비", "쿠팡 열어줘", "구매 직전 화면까지", "구매 실행 승인"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("assistant reply missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "어떤 물건들을 검색") {
		t.Fatalf("assistant reply fell through to model chat:\n%s", reply)
	}
}

func TestAssistantReplyShoppingLivePurchaseWorkflowGuide(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "실제 구매를 하라고 비서방에서 이야기 하면 제품을 찾아서 제안하고 승인받고 사게 되는건가?")
	for _, want := range []string{
		"실제 구매 요청 처리 흐름입니다.",
		"아직 장바구니/결제/최종 주문 버튼은 누르지 않았습니다.",
		"제품 검색과 후보 제안은 진행",
		"장바구니/결제 직전까지만 자동 준비",
		"한 번 더 명시적으로 승인",
		"후보 3개를 Signal에 제안",
		"최종 화면의 상품명, 옵션, 총액, 도착 예정일, 배송지 표시, 결제수단 표시",
		"CVC, 결제 비밀번호, 본인인증",
		"`구매 실행 승인`",
		"옵션/배송지/결제수단이 여러 개 보일 때:",
		"`쿠팡 상품 옵션 확인해`",
		"`쿠팡 배송지 2개 확인해`",
		"`쿠팡 결제수단 확인해`",
		"Signal에는 주소 원문/수령인/전화번호/상세주소, 카드번호/CVC/계좌번호/유효기간/명의자를 보내지 않고",
		"쿠팡에서 <상품명> 검색해서",
		"구매 완료/주문 완료를 주장하지 않고",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("live purchase workflow guide missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료했습니다", "주문 완료했습니다", "최종 주문 버튼을 눌렀습니다", "CVC: 123", "카드번호:"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("live purchase workflow guide should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingLivePurchaseWorkflowGuideEnglishUsesLanguagePack(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "en")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "If I ask you to buy for real, do you find products, suggest them, get approval, and buy?")
	for _, want := range []string{
		"Live purchase request workflow.",
		"No cart, payment, or final order button has been clicked yet.",
		"product search and candidate suggestions can proceed",
		"requires one more explicit Signal approval",
		"Suggest three candidates in Signal",
		"final screen product, option, total, arrival estimate, address visibility, and payment-method visibility",
		"CVC, payment password, and identity verification",
		"`purchase execution approved`",
		"When multiple options, addresses, or payment methods are visible:",
		"`check Coupang product options`",
		"`check Coupang two shipping addresses`",
		"`check Coupang payment method`",
		"Do not send raw address text, recipient names, phone numbers, detailed addresses, card numbers, CVC, account numbers, expiration dates, or cardholder names to Signal",
		"search Coupang for <item>",
		"do not click the final order button",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English live purchase workflow guide missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English live purchase workflow guide should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"purchase completed", "order completed", "clicked the final order button", "cvc: 123", "card number:"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English live purchase workflow guide should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingLivePurchaseWorkflowGuideFollowsEnglishInputWithoutEnvLanguage(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "If I ask you to actually buy something in the assistant room, will you find products, suggest options, get approval, and purchase it?")
	for _, want := range []string{
		"Live purchase request workflow.",
		"No cart, payment, or final order button has been clicked yet.",
		"product search and candidate suggestions can proceed",
		"requires one more explicit Signal approval",
		"Suggest three candidates in Signal",
		"`purchase execution approved`",
		"`check Coupang two shipping addresses`",
		"read the final screen product, option, total, arrival estimate, address visibility, and payment-method visibility",
		"Start in the assistant Signal room with:",
		"Before approval, I do not claim purchase/order completion and do not click the final order button.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English input live purchase workflow guide missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English input live purchase workflow guide should not expose Korean when env language is unset:\n%s", reply)
	}
	for _, bad := range []string{"구매", "주문", "결제", "purchase completed", "order completed", "clicked the final order button", "cvc: 123", "card number:"} {
		if strings.Contains(strings.ToLower(reply), strings.ToLower(bad)) {
			t.Fatalf("English input live purchase workflow guide should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingPurchaseStopGuide(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "쿠팡 구매 중지해. 결제 취소하고 멈춰")
	for _, want := range []string{
		"구매 플로우를 중지했습니다.",
		"장바구니/결제/최종 주문 버튼은 누르지 않았습니다.",
		"중지/취소 요청",
		"후보 제안, 장바구니 준비, 결제 화면 이동, 최종 클릭을 새로 실행하지 않습니다.",
		"브라우저 화면은 그대로",
		"비밀번호, CVC, 주소, 결제수단 원문",
		"`쿠팡에서 <상품명> 검색해서 후보 3개 비교해줘`",
		"`구매 실행 승인`을 보내도 바로 최종 클릭",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("purchase stop guide missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "주문 완료", "최종 주문 버튼을 눌렀습니다", "CVC: 123", "카드번호:"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("purchase stop guide should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingPurchaseStopGuideEnglishUsesLanguagePack(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "en")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "Stop the Coupang purchase flow and cancel checkout.")
	for _, want := range []string{
		"Purchase flow stopped.",
		"No cart, payment, or final order button has been clicked.",
		"stop/cancel request",
		"Do not newly run candidate selection",
		"Leave any already-open browser screen as-is",
		"Do not send passwords, CVC, raw addresses",
		"`search Coupang for <item> and compare three candidates`",
		"`purchase execution approved` does not jump straight to the final click",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English purchase stop guide missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English purchase stop guide should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"purchase completed", "order completed", "clicked the final order button", "cvc: 123", "card number:"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English purchase stop guide should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingPurchaseResumeGuide(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "쿠팡 구매 다시 시작해. 중지한 플로우 재개해")
	for _, want := range []string{
		"구매 플로우 재개는 가능합니다.",
		"최종 클릭으로 바로 이어지지 않습니다.",
		"아직 장바구니/결제/최종 주문 버튼은 누르지 않았습니다.",
		"중지 후 재개 요청",
		"이전 화면 상태를 믿지 않고",
		"후보 3개를 다시 비교",
		"상품명, 옵션, 총액, 도착 예정일, 배송지 표시, 결제수단 표시",
		"새 `구매 실행 승인`",
		"`쿠팡에서 <상품명> 검색해서 로켓배송, 가격, 리뷰 좋은 후보 3개 비교해줘`",
		"구매 완료/주문 완료를 주장하지 않고 최종 주문 버튼을 누르지 않습니다.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("purchase resume guide missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료했습니다", "주문 완료했습니다", "최종 주문 버튼을 눌렀습니다", "CVC: 123", "카드번호:"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("purchase resume guide should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingPurchaseResumeGuideEnglishUsesLanguagePack(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "en")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "Resume the Coupang purchase flow and continue shopping.")
	for _, want := range []string{
		"Purchase flow can resume",
		"does not jump directly to the final click",
		"No cart, payment, or final order button has been clicked yet.",
		"resume-after-stop request",
		"Do not trust the old screen state",
		"Compare three candidates again",
		"product, option, total, arrival estimate, address visibility, and payment-method visibility",
		"new `purchase execution approved`",
		"`search Coupang for <item> and compare three good Rocket-delivery candidates by price and reviews`",
		"does not click the final order button",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English purchase resume guide missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English purchase resume guide should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"purchase completed", "order completed", "clicked the final order button", "cvc: 123", "card number:"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English purchase resume guide should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingPurchaseControlGuidesFollowEnglishInputWithoutEnvLanguage(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "")
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "stop",
			input: "Stop the purchase flow.",
			want: []string{
				"Purchase flow stopped.",
				"stop/cancel request",
				"Do not newly run candidate selection",
				"`purchase execution approved` does not jump straight to the final click",
			},
		},
		{
			name:  "resume",
			input: "Resume the Coupang purchase flow.",
			want: []string{
				"Purchase flow can resume",
				"does not jump directly to the final click",
				"resume-after-stop request",
				"new `purchase execution approved`",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, tt.input)
			for _, want := range tt.want {
				if !strings.Contains(reply, want) {
					t.Fatalf("%s purchase control guide missing %q:\n%s", tt.name, want, reply)
				}
			}
			if assistantCheckoutPrepContainsHangul(reply) {
				t.Fatalf("%s purchase control guide should follow English input when env language is unset:\n%s", tt.name, reply)
			}
			for _, bad := range []string{"구매", "주문", "결제", "purchase completed", "order completed", "cvc: 123", "card number:"} {
				if strings.Contains(strings.ToLower(reply), strings.ToLower(bad)) {
					t.Fatalf("%s purchase control guide should not expose/claim %q:\n%s", tt.name, bad, reply)
				}
			}
		})
	}
}

func TestAssistantReplyShoppingPurchaseMissingDetailsGuide(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "쿠팡에서 사줘")
	for _, want := range []string{
		"구매할 상품 조건이 아직 부족합니다.",
		"아직 검색/장바구니/결제/최종 주문 버튼은 누르지 않았습니다.",
		"상품명 없이 실제 구매를 시작하지 않습니다.",
		"상품명 또는 상품 링크",
		"예산 또는 최대 금액",
		"필수 옵션과 수량",
		"배송 조건",
		"복사해서 보내기:",
		"상품: ",
		"예산: ",
		"옵션/수량: ",
		"후보 수: 3개",
		"조건을 받기 전에는 후보 제안, 장바구니 준비, 구매 실행 승인을 진행하지 않습니다.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("missing-details guide missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "주문 완료", "최종 주문 버튼을 눌렀습니다", "CVC: 123", "카드번호:"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("missing-details guide should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingPurchaseMissingDetailsGuideEnglishUsesLanguagePack(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "en")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "buy something for me")
	for _, want := range []string{
		"Purchase details are still missing.",
		"No search, cart, payment, or final order button has been clicked yet.",
		"I do not start a live purchase without an item name.",
		"Product name or product link",
		"Budget or maximum amount",
		"Required options and quantity",
		"Delivery constraints",
		"Copy and send:",
		"Product: ",
		"Budget: ",
		"Options/quantity: ",
		"Candidates: 3",
		"Before receiving these constraints",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English missing-details guide missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English missing-details guide should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"purchase completed", "order completed", "clicked the final order button", "cvc: 123", "card number:"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English missing-details guide should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingPurchaseMissingDetailsFollowsEnglishInputWithoutEnvLanguage(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "")
	for _, input := range []string{
		"Buy it on Coupang.",
		"Coupang buy something for me.",
	} {
		reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, input)
		for _, want := range []string{
			"Purchase details are still missing.",
			"No search, cart, payment, or final order button has been clicked yet.",
			"I do not start a live purchase without an item name.",
			"Product name or product link",
			"Budget or maximum amount",
			"Copy and send:",
			"Product: ",
			"Candidates: 3",
			"Before receiving these constraints",
		} {
			if !strings.Contains(reply, want) {
				t.Fatalf("English input missing-details guide for %q missing %q:\n%s", input, want, reply)
			}
		}
		if assistantCheckoutPrepContainsHangul(reply) {
			t.Fatalf("English input missing-details guide should not expose Korean for %q:\n%s", input, reply)
		}
		for _, bad := range []string{"쿠팡 후보 비교", "검색 결과 후보", "opened link", "https://www.coupang.com/np/search", "purchase completed", "order completed", "cvc: 123"} {
			if strings.Contains(strings.ToLower(reply), strings.ToLower(bad)) {
				t.Fatalf("English input missing-details guide should not search/expose/claim %q for %q:\n%s", bad, input, reply)
			}
		}
	}
}

func TestAssistantReplyCoupangPurchaseReadinessPreflight(t *testing.T) {
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "쿠팡 구매 테스트 준비상태 점검해줘")
	for _, want := range []string{"쿠팡 실제 구매 테스트 준비상태", "Signal 비서방 응답: 정상", "쿠팡 검색 라우팅: 정상", "브라우저 로그인: 직접 확인 필요", "실전 화면 판정 규칙:", "로그인 화면으로 이동하면", "주소지 2개인 경우", "`배송지 표시: 맞음/아님/2개 보임`", "`쿠팡 배송지 2개 확인해`", "`결제수단 표시: 맞음/아님/2개 보임`", "`쿠팡 결제수단 확인해`", "권한 질문", "비밀번호, CVC, 본인인증", "구매 실행 승인", "내일 첫 테스트 문장"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("readiness reply missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "구매 완료") || strings.Contains(reply, "주문 완료") || strings.Contains(reply, "내일 Signal에서 그대로 보낼 문장:") {
		t.Fatalf("readiness reply should not claim completion or fall back to prep script:\n%s", reply)
	}
}

func TestAssistantReplyCoupangPurchaseReadinessEnglishUsesLanguagePack(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "en")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "check Coupang live purchase readiness")
	for _, want := range []string{
		"Coupang live purchase test readiness.",
		"Signal assistant-room response: ok",
		"Coupang search routing is ready.",
		"Live screen decision rules:",
		"redirects to the login screen",
		"When two shipping addresses exist",
		"`Address visible: yes/no/two shipping addresses`",
		"`check Coupang two shipping addresses`",
		"`Payment method visible: yes/no/two cards`",
		"`check Coupang payment method`",
		"purchase execution approved",
		"First test message for tomorrow:",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English readiness reply missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English readiness reply should not expose Korean:\n%s", reply)
	}
}

func TestAssistantReplyCoupangTwoAddressChoiceGuide(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "맥미니 쿠팡에 로그인되어있어. 주소지가 두개야. 확인해")
	for _, want := range []string{
		"쿠팡 배송지 2개 확인 단계입니다.",
		"아직 장바구니/결제/최종 주문 버튼은 누르지 않았습니다.",
		"기본 배송지 또는 의도한 배송지 하나만",
		"먼저 열 화면:",
		"마이쿠팡 > 배송지 관리/주소록 관리",
		"로그인/본인확인 화면이면 사용자가 직접 처리",
		"주소 원문/수령인/전화번호/상세주소는 읽거나 Signal에 보내지 않습니다.",
		"화면에 하나만 남거나 선택 표시가 분명할 때만",
		"주소 원문, 수령인, 전화번호, 상세주소는 Signal에 보내지 말고",
		"주소 후보를 원문 없이 구분하는 방식:",
		"`배송지 후보 A: 별칭/시·구/기본여부만`",
		"`배송지 후보 B: 별칭/시·구/기본여부만`",
		"`선택 필요: A/B/기본배송지`처럼 보내고, 도로명·동호수·전화번호는 보내지 않습니다.",
		"`배송지 표시: 맞음`",
		"`배송지 표시: 2개 보임`이면 구매 가능 판정은 계속 중지됩니다.",
		"`배송지 표시: 아님`",
		"민감값 없는 회신 템플릿:",
		"`배송지 선택 확인: A/B/기본배송지`",
		"`배송지 표시: 맞음/아님/2개 보임`",
		"`주소 원문 공유: 안 함`",
		"최종 화면의 상품명, 옵션, 총액, 도착 예정일, 배송지 표시, 결제수단 표시를 읽어줘",
		"배송지 2개가 보이는 동안에는 최종 주문 클릭을 실행하지 않습니다.",
		"실행 상태:",
		"쿠팡 브라우저 테스트 화면을 열려면 Mac 작업 실행 권한이 필요합니다.",
		"작업: open_url",
		"열린 주소: https://www.coupang.com/np/mycoupang",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("two-address guide missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "주문 완료", "최종 주문 버튼을 눌렀습니다", "서울시", "상세주소:"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("two-address guide should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyCoupangTwoAddressChoiceFollowUpUsesRecentContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-coupang-address-followup"
	if err := appendSignalHistory(targetID, "쿠팡 최종 화면 확인해", "쿠팡 최종 구매 화면 확인 단계입니다.\n배송지 표시: 2개 보임\n아직 구매 버튼은 누르지 않았습니다."); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "주소지가 두개야. 확인해")
	for _, want := range []string{
		"쿠팡 배송지 2개 확인 단계입니다.",
		"기본 배송지 또는 의도한 배송지 하나만",
		"`배송지 표시: 맞음`",
		"`배송지 표시: 2개 보임`이면 구매 가능 판정은 계속 중지됩니다.",
		"주소 원문, 수령인, 전화번호, 상세주소는 Signal에 보내지 말고",
		"민감값 없는 회신 템플릿:",
		"`배송지 선택 확인: A/B/기본배송지`",
		"`주소 원문 공유: 안 함`",
		"`배송지 후보 A: 별칭/시·구/기본여부만`",
		"`선택 필요: A/B/기본배송지`처럼 보내고, 도로명·동호수·전화번호는 보내지 않습니다.",
		"배송지 2개가 보이는 동안에는 최종 주문 클릭을 실행하지 않습니다.",
		"실행 상태:",
		"열린 주소: https://www.coupang.com/np/mycoupang",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("two-address follow-up guide missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "주문 완료", "서울시", "상세주소:"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("two-address follow-up guide should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyCoupangTwoAddressChoiceGuideEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "Coupang is logged in and there are two shipping addresses. Check it.")
	for _, want := range []string{
		"Coupang two-address verification step.",
		"No cart, payment, or final order button has been clicked yet.",
		"verify exactly one default or intended shipping address",
		"Open first:",
		"My Coupang > shipping-address/address-book management",
		"If login or identity verification appears",
		"raw address, recipient name, phone number, or detailed address",
		"exactly one address remains visible",
		"Do not send the raw address, recipient name, phone number, or detailed address to Signal",
		"How to distinguish address candidates without raw address text:",
		"`Address candidate A: label/city-district/default-only`",
		"`Address candidate B: label/city-district/default-only`",
		"`Choice needed: A/B/default address`; do not send street, unit, or phone details.",
		"`Address visible: yes`",
		"`Address visible: two shipping addresses` keeps the purchase-readiness decision stopped.",
		"`Address visible: no`",
		"Sensitive-value-free reply template:",
		"`Address choice confirmed: A/B/default address`",
		"`Address visible: yes/no/two shipping addresses`",
		"`Raw address shared: no`",
		"read the final screen product, option, total, arrival estimate, address visibility, and payment-method visibility",
		"While two addresses are visible, do not execute the final order click.",
		"Execution status:",
		"Coupang browser-test screen needs Mac action permission.",
		"Action: open_url",
		"Opened URL: https://www.coupang.com/np/mycoupang",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English two-address guide missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English two-address guide should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"purchase completed", "order completed", "clicked the final order button"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English two-address guide should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyCoupangTwoAddressChoiceEnglishFollowUpUsesRecentContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-coupang-address-followup-en"
	if err := appendSignalHistory(targetID, "read the Coupang final screen", "Coupang final purchase-screen review step.\nAddress visible: two shipping addresses\nThe purchase button has not been clicked yet."); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "two shipping addresses, check it")
	for _, want := range []string{
		"Coupang two-address verification step.",
		"verify exactly one default or intended shipping address",
		"`Address visible: yes`",
		"`Address visible: two shipping addresses` keeps the purchase-readiness decision stopped.",
		"Do not send the raw address, recipient name, phone number, or detailed address to Signal",
		"Sensitive-value-free reply template:",
		"`Address choice confirmed: A/B/default address`",
		"`Raw address shared: no`",
		"`Address candidate A: label/city-district/default-only`",
		"`Choice needed: A/B/default address`; do not send street, unit, or phone details.",
		"While two addresses are visible, do not execute the final order click.",
		"Execution status:",
		"Opened URL: https://www.coupang.com/np/mycoupang",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English two-address follow-up guide missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English two-address follow-up guide should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"purchase completed", "order completed", "raw address:"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English two-address follow-up guide should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestCoupangAddressOpenURLActionUsesAddressSpecificCard(t *testing.T) {
	rawURL := coupangAddressBookURL()
	koAction := strings.Join(formatSignalCoupangOpenURLActionFor("ko", rawURL), "\n")
	koReply := localizeCoupangAddressOpenURLActionReply("ko", koAction, rawURL)
	for _, want := range []string{
		"쿠팡 배송지 확인 화면 열기를 요청했습니다.",
		"열린 주소: https://www.coupang.com/np/mycoupang",
		"열린 화면에서 확인할 것:",
		"배송지 목록에서 기본 배송지 또는 이번 주문에 쓸 배송지만 선택/확인합니다.",
		"`배송지 후보 A: 별칭/시·구/기본여부만`",
		"`선택 필요: A/B/기본배송지`처럼 보내고, 도로명·동호수·전화번호는 보내지 않습니다.",
		"배송지 확인 외에는 장바구니/결제/최종 주문 버튼을 누르지 않습니다.",
	} {
		if !strings.Contains(koReply, want) {
			t.Fatalf("Korean address-specific open card missing %q:\n%s", want, koReply)
		}
	}
	for _, bad := range []string{"검색어와 후보 상품명", "상품가, 배송비, 쿠폰", "상위 3개 후보 비교표"} {
		if strings.Contains(koReply, bad) {
			t.Fatalf("Korean address-specific open card should not show generic product-search copy %q:\n%s", bad, koReply)
		}
	}

	enAction := strings.Join(formatSignalCoupangOpenURLActionFor("en", rawURL), "\n")
	enReply := localizeCoupangAddressOpenURLActionReply("en", enAction, rawURL)
	for _, want := range []string{
		"Requested opening the Coupang address-verification screen.",
		"Opened URL: https://www.coupang.com/np/mycoupang",
		"Check on the opened screen:",
		"In the address list, select/confirm only the default address or the address for this order.",
		"`Address candidate A: label/city-district/default-only`",
		"`Choice needed: A/B/default address`; do not send street, unit, or phone details.",
		"Do not click add-to-cart, payment, or final order while verifying the address.",
	} {
		if !strings.Contains(enReply, want) {
			t.Fatalf("English address-specific open card missing %q:\n%s", want, enReply)
		}
	}
	if assistantCheckoutPrepContainsHangul(enReply) {
		t.Fatalf("English address-specific open card should not expose Korean:\n%s", enReply)
	}
	for _, bad := range []string{"Search query and candidate product names", "Expected total after item price", "top 3 candidates"} {
		if strings.Contains(enReply, bad) {
			t.Fatalf("English address-specific open card should not show generic product-search copy %q:\n%s", bad, enReply)
		}
	}
}

func TestAssistantScenarioCoupangAddressChangeStopsBeforeSave(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "쿠팡 배송지 변경해줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	for _, want := range []string{
		"쿠팡 배송지 변경 준비 단계입니다.",
		"아직 배송지 변경 저장, 주문 변경, 주문 취소, 고객센터 전송 버튼은 누르지 않았습니다.",
		"주문 또는 배송지 변경 가능 여부만 확인하고 저장/제출 전에서 멈춰야 합니다.",
		"쿠팡 주문목록을 열고 배송지 변경 대상 주문을 찾습니다.",
		"상품명, 주문일, 수량, 배송 상태가 사용자가 변경하려는 주문과 맞는지 확인합니다.",
		"배송지 변경 가능 버튼, 변경 전/후 주소 선택 상태, 추가 배송비나 도착일 변화",
		"주소 원문, 수령인, 전화번호, 상세주소, 공동현관 비밀번호는 Signal에 보내지 말고",
		"`배송지 변경 가능: 맞음`",
		"`배송지 변경 제한 보임`이면 보이는 안내만 요약하고 버튼은 누르지 않음",
		"명시 승인 전에는 외부 계정 상태를 바꾸는 배송지 저장/적용/주문변경/고객센터 제출을 실행하지 않습니다.",
		"열린 주소: https://www.coupang.com/np/mycoupang/order/list",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("address-change guard missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"쿠팡 배송지 2개 확인 단계", "배송지를 변경했습니다", "주소를 저장했습니다", "서울시", "상세주소:"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("address-change guard should not misroute/expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantScenarioCoupangAddressChangeEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "change the shipping address for my Coupang order")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	for _, want := range []string{
		"Coupang shipping-address change prep step.",
		"No shipping-address save, order change, order cancel, or customer-service submit button has been clicked yet.",
		"check only whether the order or shipping address can be changed, then stop before save or submission.",
		"open the Coupang order list and find the order for shipping-address change.",
		"product name, order date, quantity, and delivery status match the order",
		"whether an address-change button is available",
		"shipping fee or arrival date changes are visible",
		"Do not send raw address, recipient name, phone number, detailed address, or building-entry password",
		"`Address change available: yes`",
		"`Address change limitation visible` means summarize only visible guidance and do not click buttons",
		"Before explicit approval, do not save/apply shipping-address changes, change orders, or submit customer-service actions",
		"Opened URL: https://www.coupang.com/np/mycoupang/order/list",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English address-change guard missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English address-change guard should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"two-address verification step", "address changed", "address saved", "purchase flow has been stopped"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English address-change guard should not misroute/expose/claim %q:\n%s", bad, reply)
		}
	}
}

func legacyTestAssistantReplyCoupangPaymentChoiceGuide(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "쿠팡 결제수단이 두개야. 어떤 카드인지 확인해")
	for _, want := range []string{
		"쿠팡 결제수단 확인 단계입니다.",
		"아직 결제/최종 주문 버튼은 누르지 않았습니다.",
		"이번 주문에 쓸 결제수단 하나만",
		"먼저 열 화면:",
		"결제 직전 화면 또는 결제수단 관리 화면",
		"결제 비밀번호/CVC/본인확인 화면이면 사용자가 직접 처리",
		"카드/계좌 민감값은 읽거나 Signal에 보내지 않습니다.",
		"하나가 선택된 표시가 분명할 때만",
		"카드번호, CVC, 계좌번호, 유효기간, 명의자, 전화번호 원문은 Signal에 보내지 말고",
		"결제수단 후보를 민감값 없이 구분하는 방식:",
		"`결제수단 후보 A: 카드/계좌 종류·발급사/은행·끝 4자리만`",
		"`결제수단 후보 B: 카드/계좌 종류·발급사/은행·끝 4자리만`",
		"`선택 필요: A/B/기본결제수단`처럼 보내고, 전체 카드번호·계좌번호·CVC·유효기간은 보내지 않습니다.",
		"`결제수단 표시: 맞음`",
		"`결제수단 표시: 2개 보임`이면 구매 가능 판정은 계속 중지됩니다.",
		"`결제수단 표시: 아님`",
		"민감값 없는 회신 템플릿:",
		"`결제수단 선택 확인: A/B/기본결제수단`",
		"`결제수단 표시: 맞음/아님/2개 보임`",
		"`카드/계좌 원문 공유: 안 함`",
		"최종 화면의 상품명, 옵션, 총액, 도착 예정일, 배송지 표시, 결제수단 표시를 읽어줘",
		"결제수단을 고르는 동안에는 최종 주문 클릭을 실행하지 않습니다.",
		"실행 상태:",
		"쿠팡 브라우저 테스트 화면을 열려면 Mac 작업 실행 권한이 필요합니다.",
		"작업: open_url",
		"열린 주소: https://cart.coupang.com/cartView.pang",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("payment-choice guide missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "주문 완료", "최종 주문 버튼을 눌렀습니다", "카드번호: 1234", "CVC: 123"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("payment-choice guide should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyCoupangPaymentChoiceFollowUpUsesRecentContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-coupang-payment-followup"
	if err := appendSignalHistory(targetID, "쿠팡 최종 화면 확인해", "쿠팡 최종 구매 화면 확인 단계입니다.\n결제수단 표시: 2개 보임\n아직 구매 버튼은 누르지 않았습니다."); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "카드가 두개야. 확인해")
	for _, want := range []string{
		"쿠팡 결제수단 확인 단계입니다.",
		"이번 주문에 쓸 결제수단 하나만",
		"`결제수단 표시: 맞음`",
		"`결제수단 표시: 2개 보임`이면 구매 가능 판정은 계속 중지됩니다.",
		"카드번호, CVC, 계좌번호, 유효기간, 명의자, 전화번호 원문은 Signal에 보내지 말고",
		"민감값 없는 회신 템플릿:",
		"`결제수단 선택 확인: A/B/기본결제수단`",
		"`카드/계좌 원문 공유: 안 함`",
		"`결제수단 후보 A: 카드/계좌 종류·발급사/은행·끝 4자리만`",
		"`선택 필요: A/B/기본결제수단`처럼 보내고, 전체 카드번호·계좌번호·CVC·유효기간은 보내지 않습니다.",
		"결제수단을 고르는 동안에는 최종 주문 클릭을 실행하지 않습니다.",
		"실행 상태:",
		"열린 주소: https://cart.coupang.com/cartView.pang",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("payment-choice follow-up guide missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "주문 완료", "카드번호: 1234", "CVC: 123", "최종 주문 버튼을 눌렀습니다"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("payment-choice follow-up guide should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyCoupangPaymentChoiceGuideEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "Coupang has two payment methods. Check which card to use.")
	for _, want := range []string{
		"Coupang payment-method verification step.",
		"No payment or final order button has been clicked yet.",
		"verify exactly one payment method",
		"Open first:",
		"pre-payment screen or payment-method management",
		"If payment password, CVC, or identity verification appears",
		"sensitive card/account values",
		"exactly one payment method is clearly selected",
		"Do not send raw card numbers, CVC, account numbers, expiration dates, cardholder names, or phone numbers to Signal",
		"How to distinguish payment candidates without sensitive values:",
		"`Payment candidate A: card/account type, issuer/bank, last 4 only`",
		"`Payment candidate B: card/account type, issuer/bank, last 4 only`",
		"`Choice needed: A/B/default payment method`; do not send full card number, account number, CVC, or expiration date.",
		"`Payment method visible: yes`",
		"`Payment method visible: two cards` keeps the purchase-readiness decision stopped.",
		"`Payment method visible: no`",
		"Sensitive-value-free reply template:",
		"`Payment choice confirmed: A/B/default payment method`",
		"`Payment method visible: yes/no/two cards`",
		"`Raw card/account values shared: no`",
		"read the final screen product, option, total, arrival estimate, address visibility, and payment-method visibility",
		"While choosing a payment method, do not execute the final order click.",
		"Execution status:",
		"Coupang browser-test screen needs Mac action permission.",
		"Action: open_url",
		"Opened URL: https://cart.coupang.com/cartView.pang",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English payment-choice guide missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English payment-choice guide should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"purchase completed", "order completed", "clicked the final order button", "cvc: 123"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English payment-choice guide should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyCoupangPaymentChoiceEnglishFollowUpUsesRecentContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-coupang-payment-followup-en"
	if err := appendSignalHistory(targetID, "read the Coupang final screen", "Coupang final purchase-screen review step.\nPayment method visible: two cards\nThe purchase button has not been clicked yet."); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "two cards, check it")
	for _, want := range []string{
		"Coupang payment-method verification step.",
		"verify exactly one payment method",
		"`Payment method visible: yes`",
		"`Payment method visible: two cards` keeps the purchase-readiness decision stopped.",
		"Do not send raw card numbers, CVC, account numbers, expiration dates, cardholder names, or phone numbers to Signal",
		"`Payment candidate A: card/account type, issuer/bank, last 4 only`",
		"`Choice needed: A/B/default payment method`; do not send full card number, account number, CVC, or expiration date.",
		"While choosing a payment method, do not execute the final order click.",
		"Execution status:",
		"Opened URL: https://cart.coupang.com/cartView.pang",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English payment-choice follow-up guide missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English payment-choice follow-up guide should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"purchase completed", "order completed", "card number:", "cvc:"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English payment-choice follow-up guide should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestCoupangPaymentOpenURLActionUsesPaymentSpecificCard(t *testing.T) {
	rawURL := coupangPaymentReviewURL()
	koAction := strings.Join(formatSignalCoupangOpenURLActionFor("ko", rawURL), "\n")
	koReply := localizeCoupangPaymentOpenURLActionReply("ko", koAction, rawURL)
	for _, want := range []string{
		"쿠팡 결제수단 확인 화면 열기를 요청했습니다.",
		"열린 주소: https://cart.coupang.com/cartView.pang",
		"열린 화면에서 확인할 것:",
		"결제수단 목록에서 이번 주문에 쓸 카드/계좌만 선택/확인합니다.",
		"`결제수단 후보 A: 카드/계좌 종류·발급사/은행·끝 4자리만`",
		"`선택 필요: A/B/기본결제수단`처럼 보내고, 전체 카드번호·계좌번호·CVC·유효기간은 보내지 않습니다.",
		"결제수단 확인 외에는 결제/최종 주문 버튼을 누르지 않습니다.",
	} {
		if !strings.Contains(koReply, want) {
			t.Fatalf("Korean payment-specific open card missing %q:\n%s", want, koReply)
		}
	}
	for _, bad := range []string{"검색어와 후보 상품명", "상품가, 배송비, 쿠폰", "상위 3개 후보 비교표"} {
		if strings.Contains(koReply, bad) {
			t.Fatalf("Korean payment-specific open card should not show generic product-search copy %q:\n%s", bad, koReply)
		}
	}

	enAction := strings.Join(formatSignalCoupangOpenURLActionFor("en", rawURL), "\n")
	enReply := localizeCoupangPaymentOpenURLActionReply("en", enAction, rawURL)
	for _, want := range []string{
		"Requested opening the Coupang payment-method verification screen.",
		"Opened URL: https://cart.coupang.com/cartView.pang",
		"Check on the opened screen:",
		"In the payment-method list, select/confirm only the card or account for this order.",
		"`Payment candidate A: card/account type, issuer/bank, last 4 only`",
		"`Choice needed: A/B/default payment method`; do not send full card number, account number, CVC, or expiration date.",
		"Do not click payment or final order while verifying the payment method.",
	} {
		if !strings.Contains(enReply, want) {
			t.Fatalf("English payment-specific open card missing %q:\n%s", want, enReply)
		}
	}
	if assistantCheckoutPrepContainsHangul(enReply) {
		t.Fatalf("English payment-specific open card should not expose Korean:\n%s", enReply)
	}
	for _, bad := range []string{"Search query and candidate product names", "Expected total after item price", "top 3 candidates"} {
		if strings.Contains(enReply, bad) {
			t.Fatalf("English payment-specific open card should not show generic product-search copy %q:\n%s", bad, enReply)
		}
	}
}

func TestAssistantReplyCoupangPaymentAuthGuide(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "쿠팡 결제 비밀번호랑 CVC가 떠. 어떻게 해?")
	for _, want := range []string{
		"쿠팡 결제 인증 직접 처리 단계입니다.",
		"아직 결제/최종 주문 버튼은 누르지 않았습니다.",
		"CVC, 결제 비밀번호, 본인인증은 사용자가 화면에서 직접 처리해야 합니다.",
		"민감값 원문은 Signal에 보내지 않습니다.",
		"`결제수단 표시: 맞음`",
		"`결제수단 표시: 아님`",
		"최종 화면의 상품명, 옵션, 총액, 도착 예정일, 배송지 표시, 결제수단 표시를 읽어줘",
		"인증값을 처리하는 동안에는 최종 주문 클릭을 실행하지 않습니다.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("payment-auth guide missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "주문 완료", "최종 주문 버튼을 눌렀습니다", "CVC: 123", "비밀번호:"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("payment-auth guide should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyCoupangPaymentAuthGuideEnglishUsesLanguagePack(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "en")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "Coupang asks for CVC and payment password. What should I do?")
	for _, want := range []string{
		"Coupang payment-authentication handoff.",
		"No payment or final order button has been clicked yet.",
		"CVC, payment password, and identity verification must be handled directly by the user",
		"Do not send raw sensitive values to Signal.",
		"`Payment method visible: yes`",
		"`Payment method visible: no`",
		"read the final screen product, option, total, arrival estimate, address visibility, and payment-method visibility",
		"While authentication values are being handled, do not execute the final order click.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English payment-auth guide missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English payment-auth guide should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"purchase completed", "order completed", "clicked the final order button", "cvc: 123", "password:"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English payment-auth guide should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyCoupangPaymentAuthFollowUpUsesRecentContext(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	target := "argos-assistant-coupang-auth-followup-ko"
	if err := appendSignalHistory(target, "쿠팡 최종 화면 확인해", "쿠팡 최종 구매 화면 확인 단계입니다.\n본인인증 화면이 나왔습니다.\n아직 구매 버튼은 누르지 않았습니다."); err != nil {
		t.Fatalf("append history: %v", err)
	}

	reply := assistantReply(ListenOptions{TargetID: target, Mode: "assistant"}, "본인인증 나왔어. 확인해")
	for _, want := range []string{
		"쿠팡 결제 인증 직접 처리 단계입니다.",
		"아직 결제/최종 주문 버튼은 누르지 않았습니다.",
		"CVC, 결제 비밀번호, 본인인증은 사용자가 화면에서 직접 처리해야 합니다.",
		"민감값 원문은 Signal에 보내지 않습니다.",
		"`결제수단 표시: 맞음`",
		"인증값을 처리하는 동안에는 최종 주문 클릭을 실행하지 않습니다.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("payment-auth follow-up guide missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "주문 완료", "최종 주문 버튼을 눌렀습니다", "CVC: 123", "비밀번호:"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("payment-auth follow-up guide should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyCoupangPaymentAuthEnglishFollowUpUsesRecentContext(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "en")
	target := "argos-assistant-coupang-auth-followup-en"
	if err := appendSignalHistory(target, "read the Coupang final screen", "Coupang final purchase-screen review step.\nIdentity verification appeared.\nThe purchase button has not been clicked yet."); err != nil {
		t.Fatalf("append history: %v", err)
	}

	reply := assistantReply(ListenOptions{TargetID: target, Mode: "assistant"}, "identity verification appeared, check it")
	for _, want := range []string{
		"Coupang payment-authentication handoff.",
		"No payment or final order button has been clicked yet.",
		"CVC, payment password, and identity verification must be handled directly by the user",
		"Do not send raw sensitive values to Signal.",
		"`Payment method visible: yes`",
		"While authentication values are being handled, do not execute the final order click.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English payment-auth follow-up guide missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English payment-auth follow-up guide should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"purchase completed", "order completed", "clicked the final order button", "cvc: 123", "password:"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English payment-auth follow-up guide should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyCoupangProductOptionChoiceGuide(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "쿠팡 상품 옵션이 여러 개야. 색상 사이즈 수량 어떤 걸 선택할지 확인해")
	for _, want := range []string{
		"쿠팡 상품 옵션 확인 단계입니다.",
		"아직 장바구니/결제/최종 주문 버튼은 누르지 않았습니다.",
		"색상, 사이즈, 수량, 용량 같은 구매 옵션",
		"색상/사이즈/용량/묶음 옵션만 선택",
		"자동 정기배송/구독 옵션",
		"선택된 옵션명, 수량, 배송 유형이 화면에서 분명할 때만",
		"임의로 고르지 말고",
		"상품 옵션 후보를 구매값만으로 구분하는 방식:",
		"`옵션 후보 A: 옵션명·색상/사이즈·수량·재고/배송·예상 총액만`",
		"`옵션 후보 B: 옵션명·색상/사이즈·수량·재고/배송·예상 총액만`",
		"`선택 필요: A/B/수량 변경`처럼 보내고, 주소·결제 민감값은 보내지 않습니다.",
		"`옵션 표시: 맞음`",
		"옵션이 여러 개이거나 헷갈리면 `옵션 표시: 아님`",
		"`옵션 표시: 아님`",
		"장바구니 또는 구매 직전 화면까지 준비해줘",
		"옵션/수량이 확정되기 전에는 장바구니, 결제, 최종 주문 클릭을 실행하지 않습니다.",
		"실행 상태:",
		"쿠팡 브라우저 테스트 화면을 열려면 Mac 작업 실행 권한이 필요합니다.",
		"작업: open_url",
		"열린 주소: https://www.coupang.com/",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("product-option guide missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "주문 완료", "최종 주문 버튼을 눌렀습니다", "CVC: 123", "카드번호:"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("product-option guide should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyCoupangProductOptionChoiceFollowUpUsesRecentContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-coupang-option-followup"
	if err := appendSignalHistory(targetID, "쿠팡 1번 상품 상세 열어줘", "쿠팡 상품 상세 확인 단계입니다.\n옵션/수량이 여러 개 보입니다.\n아직 장바구니 버튼은 누르지 않았습니다."); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "옵션이 여러 개야. 확인해")
	for _, want := range []string{
		"쿠팡 상품 옵션 확인 단계입니다.",
		"색상, 사이즈, 수량, 용량 같은 구매 옵션",
		"상품 옵션 후보를 구매값만으로 구분하는 방식:",
		"`선택 필요: A/B/수량 변경`처럼 보내고, 주소·결제 민감값은 보내지 않습니다.",
		"`옵션 표시: 맞음`",
		"옵션이 여러 개이거나 헷갈리면 `옵션 표시: 아님`",
		"임의로 고르지 말고",
		"옵션/수량이 확정되기 전에는 장바구니, 결제, 최종 주문 클릭을 실행하지 않습니다.",
		"실행 상태:",
		"열린 주소: https://www.coupang.com/",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("product-option follow-up guide missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "주문 완료", "장바구니에 담았습니다", "최종 주문 버튼을 눌렀습니다"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("product-option follow-up guide should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyCoupangProductOptionChoiceGuideEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "Coupang has multiple product options for color size and quantity. Which one should we choose?")
	for _, want := range []string{
		"Coupang product-option verification step.",
		"No cart, payment, or final order button has been clicked yet.",
		"color, size, quantity, volume, or bundle options",
		"Select only the color, size, volume, or bundle option",
		"verify subscription or recurring delivery is not enabled",
		"selected option name, quantity, and delivery type are clear",
		"do not guess",
		"How to distinguish product-option candidates using purchase values only:",
		"`Option candidate A: option name, color/size, quantity, stock/delivery, estimated total only`",
		"`Option candidate B: option name, color/size, quantity, stock/delivery, estimated total only`",
		"`Choice needed: A/B/quantity change`; do not send address or payment-sensitive values.",
		"`Option visible: yes`",
		"If multiple options are visible or unclear, send `Option visible: no`",
		"`Option visible: no`",
		"prepare candidate 1 up to the cart or pre-purchase screen",
		"Before option and quantity are confirmed, do not execute cart, payment, or final order clicks.",
		"Execution status:",
		"Coupang browser-test screen needs Mac action permission.",
		"Action: open_url",
		"Opened URL: https://www.coupang.com/",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English product-option guide missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English product-option guide should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"purchase completed", "order completed", "clicked the final order button", "cvc: 123", "card number:"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English product-option guide should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyCoupangProductOptionChoiceEnglishFollowUpUsesRecentContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-coupang-option-followup-en"
	if err := appendSignalHistory(targetID, "open the first Coupang product detail", "Coupang product detail review step.\nMultiple option/quantity choices are visible.\nNo add-to-cart button has been clicked yet."); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "multiple options, check it")
	for _, want := range []string{
		"Coupang product-option verification step.",
		"color, size, quantity, volume, or bundle options",
		"How to distinguish product-option candidates using purchase values only:",
		"`Choice needed: A/B/quantity change`; do not send address or payment-sensitive values.",
		"`Option visible: yes`",
		"If multiple options are visible or unclear, send `Option visible: no`",
		"do not guess",
		"Before option and quantity are confirmed, do not execute cart, payment, or final order clicks.",
		"Execution status:",
		"Opened URL: https://www.coupang.com/",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English product-option follow-up guide missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English product-option follow-up guide should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"purchase completed", "order completed", "added to cart", "clicked the final order button"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English product-option follow-up guide should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestCoupangProductOptionOpenURLActionUsesOptionSpecificCard(t *testing.T) {
	rawURL := "https://www.coupang.com/vp/products/123456789"
	koAction := strings.Join(formatSignalCoupangOpenURLActionFor("ko", rawURL), "\n")
	koReply := localizeCoupangProductOptionOpenURLActionReply("ko", koAction, rawURL)
	for _, want := range []string{
		"쿠팡 상품 옵션 확인 화면 열기를 요청했습니다.",
		"열린 주소: https://www.coupang.com/vp/products/123456789",
		"열린 화면에서 확인할 것:",
		"이번 주문에 맞는 색상/사이즈/용량/묶음 옵션만 선택합니다.",
		"상품 옵션 후보를 구매값만으로 구분하는 방식:",
		"`선택 필요: A/B/수량 변경`처럼 보내고, 주소·결제 민감값은 보내지 않습니다.",
		"옵션/수량 확인 외에는 장바구니/결제/최종 주문 버튼을 누르지 않습니다.",
	} {
		if !strings.Contains(koReply, want) {
			t.Fatalf("Korean option-specific open card missing %q:\n%s", want, koReply)
		}
	}
	for _, bad := range []string{"검색어와 후보 상품명", "상품가, 배송비, 쿠폰", "상위 3개 후보 비교표"} {
		if strings.Contains(koReply, bad) {
			t.Fatalf("Korean option-specific open card should not show generic product-search copy %q:\n%s", bad, koReply)
		}
	}

	enAction := strings.Join(formatSignalCoupangOpenURLActionFor("en", rawURL), "\n")
	enReply := localizeCoupangProductOptionOpenURLActionReply("en", enAction, rawURL)
	for _, want := range []string{
		"Requested opening the Coupang product-option verification screen.",
		"Opened URL: https://www.coupang.com/vp/products/123456789",
		"Check on the opened screen:",
		"Select only the color, size, volume, or bundle option intended for this order.",
		"How to distinguish product-option candidates using purchase values only:",
		"`Choice needed: A/B/quantity change`; do not send address or payment-sensitive values.",
		"Do not click cart, payment, or final order while verifying option and quantity.",
	} {
		if !strings.Contains(enReply, want) {
			t.Fatalf("English option-specific open card missing %q:\n%s", want, enReply)
		}
	}
	if assistantCheckoutPrepContainsHangul(enReply) {
		t.Fatalf("English option-specific open card should not expose Korean:\n%s", enReply)
	}
	for _, bad := range []string{"Search query and candidate product names", "Expected total after item price", "top 3 candidates"} {
		if strings.Contains(enReply, bad) {
			t.Fatalf("English option-specific open card should not show generic product-search copy %q:\n%s", bad, enReply)
		}
	}
}

func TestAssistantReplyCoupangChoiceGuidesFollowEnglishInputWithoutEnvLanguage(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "")
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "address",
			input: "Coupang is logged in and there are two shipping addresses. Check it.",
			want: []string{
				"Coupang two-address verification step.",
				"`Address visible: yes`",
				"While two addresses are visible, do not execute the final order click.",
			},
		},
		{
			name:  "payment-method",
			input: "Coupang has two payment methods. Check which card to use.",
			want: []string{
				"Coupang payment-method verification step.",
				"`Payment method visible: yes`",
				"While choosing a payment method, do not execute the final order click.",
			},
		},
		{
			name:  "payment-auth",
			input: "Coupang asks for CVC and payment password. What should I do?",
			want: []string{
				"Coupang payment-authentication handoff.",
				"`Payment method visible: yes`",
				"While authentication values are being handled, do not execute the final order click.",
			},
		},
		{
			name:  "product-option",
			input: "Coupang has multiple product options for color size and quantity. Which one should we choose?",
			want: []string{
				"Coupang product-option verification step.",
				"`Option visible: yes`",
				"Before option and quantity are confirmed, do not execute cart, payment, or final order clicks.",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, tt.input)
			for _, want := range tt.want {
				if !strings.Contains(reply, want) {
					t.Fatalf("%s guide missing %q:\n%s", tt.name, want, reply)
				}
			}
			if assistantCheckoutPrepContainsHangul(reply) {
				t.Fatalf("%s guide should follow English input when env language is unset:\n%s", tt.name, reply)
			}
			for _, bad := range []string{"purchase completed", "order completed", "clicked the final order button", "cvc: 123", "password: 123"} {
				if strings.Contains(strings.ToLower(reply), bad) {
					t.Fatalf("%s guide should not expose/claim %q:\n%s", tt.name, bad, reply)
				}
			}
		})
	}
}

func TestAssistantReplyPurchaseFinalApprovalNeedsContext(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "구매 실행 승인")
	for _, want := range []string{"구매 실행 승인은 받았지만", "아직 최종 클릭은 하지 않았습니다", "상품명과 옵션", "총액", "도착 예정일", "배송지", "결제수단", "최종 주문 버튼 위치", "먼저 헷갈리는 화면값을 정리하세요:", "`쿠팡 상품 옵션 확인해`", "`쿠팡 배송지 2개 확인해`", "`쿠팡 결제수단 확인해`", "최종 화면의 상품명"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("purchase final approval guard missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "주문 완료", "Argos가 Mac 작업을 처리했습니다"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("purchase final approval guard should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyPurchaseFinalApprovalAcceptsNaturalAliases(t *testing.T) {
	for _, input := range []string{"구매 승인", "구입 승인", "주문 승인", "결제 승인"} {
		t.Run(input, func(t *testing.T) {
			t.Setenv("MESHCLAW_LANG", "ko")
			reply := assistantReply(ListenOptions{TargetID: "argos-assistant-" + strings.NewReplacer(" ", "-", "\t", "-").Replace(input), Mode: "assistant"}, input)
			for _, want := range []string{"구매 실행 승인은 받았지만", "아직 최종 클릭은 하지 않았습니다", "상품명과 옵션", "총액", "배송지", "결제수단"} {
				if !strings.Contains(reply, want) {
					t.Fatalf("purchase alias %q guard missing %q:\n%s", input, want, reply)
				}
			}
			for _, bad := range []string{"구매 완료", "주문 완료", "최종 주문 버튼을 눌렀습니다"} {
				if strings.Contains(reply, bad) {
					t.Fatalf("purchase alias %q should not claim execution %q:\n%s", input, bad, reply)
				}
			}
		})
	}
}

func TestAssistantReplyPurchaseFinalApprovalAcceptsEnglishAliases(t *testing.T) {
	for _, input := range []string{"purchase approved", "order approved", "checkout approved", "payment approved"} {
		t.Run(input, func(t *testing.T) {
			t.Setenv("MESHCLAW_LANG", "en")
			reply := assistantReply(ListenOptions{TargetID: "argos-assistant-" + strings.NewReplacer(" ", "-", "\t", "-").Replace(input), Mode: "assistant"}, input)
			for _, want := range []string{"Purchase execution approval was received", "I have not clicked the final order button yet", "Product name and options", "Total", "Payment-method visibility"} {
				if !strings.Contains(reply, want) {
					t.Fatalf("English purchase alias %q guard missing %q:\n%s", input, want, reply)
				}
			}
			for _, bad := range []string{"purchase completed", "order completed", "I clicked the final order button", "clicked the final order button."} {
				if strings.Contains(strings.ToLower(reply), bad) {
					t.Fatalf("English purchase alias %q should not claim execution %q:\n%s", input, bad, reply)
				}
			}
		})
	}
}

func TestAssistantReplyPurchaseNaturalGoAheadNeedsContext(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "이대로 사줘. 결제 진행해")
	for _, want := range []string{
		"최종 구매 진행 요청을 받았지만",
		"아직 최종 클릭은 하지 않았습니다",
		"최종 주문 버튼을 누르려면 먼저 아래 정보가 확정되어야 합니다.",
		"상품명과 옵션",
		"총액",
		"도착 예정일",
		"배송지",
		"결제수단",
		"최종 주문 버튼 위치",
		"`쿠팡 배송지 2개 확인해`",
		"`쿠팡 결제수단 확인해`",
		"`최종 화면의 상품명, 옵션, 총액, 도착 예정일, 배송지 표시, 결제수단 표시를 읽어줘`",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("natural purchase go-ahead guard missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "주문 완료", "최종 주문 버튼을 눌렀습니다", "실제 구매 요청 처리 흐름"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("natural purchase go-ahead guard should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyPurchaseFinalApprovalEnglishUsesLanguagePack(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "en")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "purchase execution approved")
	for _, want := range []string{
		"Purchase execution approval was received",
		"I have not clicked the final order button yet",
		"Product name and options",
		"Total",
		"Arrival estimate",
		"Address visibility",
		"Payment-method visibility",
		"Final order button location",
		"Resolve unclear screen values first:",
		"`check Coupang product options`",
		"`check Coupang two shipping addresses`",
		"`check Coupang payment method`",
		"read the final screen product, option, total, arrival estimate, address visibility, and payment-method visibility",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English purchase final approval guard missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English purchase final approval guard should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"purchase completed", "order completed", "argos handled a mac action"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("purchase final approval guard should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyPurchaseFinalApprovalFollowsEnglishInputWithoutEnvLanguage(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "purchase execution approved")
	for _, want := range []string{
		"Purchase execution approval was received",
		"I have not clicked the final order button yet",
		"Before the final order button can be clicked",
		"Product name and options",
		"Arrival estimate",
		"Payment-method visibility",
		"`check Coupang two shipping addresses`",
		"`check Coupang payment method`",
		"read the final screen product, option, total, arrival estimate, address visibility, and payment-method visibility",
		"`purchase execution approved`",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English input purchase final approval guard missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English input purchase final approval guard should not expose Korean when env language is unset:\n%s", reply)
	}
	for _, bad := range []string{"구매", "주문", "결제", "purchase completed", "order completed", "argos handled a mac action"} {
		if strings.Contains(strings.ToLower(reply), strings.ToLower(bad)) {
			t.Fatalf("English input purchase final approval guard should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyPurchaseNaturalGoAheadFollowsEnglishInputWithoutEnvLanguage(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "Go ahead and complete checkout.")
	for _, want := range []string{
		"Final purchase go-ahead was received",
		"I have not clicked the final order button yet",
		"Before the final order button can be clicked",
		"Product name and options",
		"Arrival estimate",
		"Payment-method visibility",
		"`check Coupang two shipping addresses`",
		"`check Coupang payment method`",
		"read the final screen product, option, total, arrival estimate, address visibility, and payment-method visibility",
		"`purchase execution approved`",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English input natural purchase go-ahead guard missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English input natural purchase go-ahead guard should not expose Korean when env language is unset:\n%s", reply)
	}
	for _, bad := range []string{"구매", "주문", "결제", "purchase completed", "order completed", "actual click execution tool"} {
		if strings.Contains(strings.ToLower(reply), strings.ToLower(bad)) {
			t.Fatalf("English input natural purchase go-ahead guard should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyPurchaseNaturalGoAheadWithReadyDecisionEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	targetID := "argos-assistant-purchase-natural-goahead-en"
	readyDecision := formatShoppingCheckoutDecisionReply(browserauto.Link{Text: "Pulmuone bottled water 500ml 20-pack"}, checkoutScreenFields{
		Product:  "Pulmuone bottled water",
		Option:   "500ml 20-pack",
		Quantity: "1",
		Total:    "10,990 KRW",
		Delivery: "tomorrow",
		Address:  "yes",
		Payment:  "yes",
		Button:   "blue button at bottom",
	})
	if err := appendSignalHistory(targetID, "purchase readiness decision", readyDecision); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "Go ahead and buy it now.")
	for _, want := range []string{
		"Final purchase go-ahead is confirmed.",
		"pre-final-click execution step",
		"Recent purchase readiness decision: ready",
		"The order button has not been clicked yet.",
		"`meshclaw_purchase_click`",
		"Checkout/order screen URL",
		"Final order button coordinates x/y",
		"Final execution template to send next in Signal:",
		"Confirmation text: purchase execution approved",
		"Only when these values and internal approve=true are present",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English natural purchase go-ahead handoff missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English natural purchase go-ahead handoff should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"purchase completed", "order completed", "clicked the final order button", "clicked the final button"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English natural purchase go-ahead handoff should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyPurchaseNaturalShortKoreanGoAheadWithReadyDecisionUsesRecentContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	targetID := "argos-assistant-purchase-natural-short-goahead-ko"
	readyDecision := formatShoppingCheckoutDecisionReply(browserauto.Link{Text: "풀무원샘물 무라벨 생수, 500ml, 20개"}, checkoutScreenFields{
		Product:  "풀무원샘물 무라벨 생수",
		Option:   "500ml 20개",
		Quantity: "1",
		Total:    "10,990원",
		Delivery: "내일 도착",
		Address:  "맞음",
		Payment:  "맞음",
		Button:   "화면 하단 파란 버튼",
	})
	if err := appendSignalHistory(targetID, "구매 가능 판정해줘", readyDecision); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "그래 주문해")
	for _, want := range []string{
		"최종 구매 진행 요청을 확인했습니다.",
		"최종 클릭 실행 직전 단계",
		"최근 구매 가능 판정: 준비됨",
		"아직 주문 버튼은 누르지 않았습니다.",
		"`meshclaw_purchase_click`",
		"체크아웃/주문 화면 URL",
		"최종 주문 버튼 좌표 x/y",
		"Signal에 이어 보낼 최종 실행 템플릿:",
		"확인 문구: 구매 실행 승인",
		"이 값과 내부 approve=true가 모두 있을 때만 최종 버튼을 한 번 클릭합니다.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("Korean short natural purchase go-ahead handoff missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "주문 완료", "최종 버튼을 눌렀습니다", "최종 주문 버튼을 눌렀습니다"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("Korean short natural purchase go-ahead handoff should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyPurchaseShortApprovalBuildsExecutionCardFromReadyDecisionCoordinates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	targetID := "argos-assistant-purchase-short-approval-execution-card"
	readyDecision := formatShoppingCheckoutDecisionReply(browserauto.Link{Text: "풀무원샘물 무라벨 생수, 500ml, 20개"}, checkoutScreenFields{
		Product:  "풀무원샘물 무라벨 생수",
		Option:   "500ml 20개",
		Quantity: "1",
		Total:    "10,990원",
		Delivery: "내일 도착",
		Address:  "맞음",
		Payment:  "맞음",
		Button:   "x=456, y=789",
	})
	if err := appendSignalHistory(targetID, "구매 가능 판정해줘", readyDecision); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "ㅇㅇ")
	for _, want := range []string{
		"최종 구매 클릭 실행값이 준비되었습니다.",
		"판정: 실행 도구 입력값 완성",
		"상품명: 풀무원샘물 무라벨 생수 / 500ml 20개",
		"총액: 10,990원",
		"도착 예정일: 내일 도착",
		"배송지 표시: 확인됨",
		"결제수단 표시: 확인됨",
		"최종 주문 버튼 좌표: x=456, y=789",
		"`meshclaw_purchase_click`",
		`"execute": true`,
		`"approve": true`,
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("short approval execution card missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "주문 완료", "최종 주문 버튼을 눌렀습니다"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("non-execute short approval should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyPurchaseShortApprovalExecuteClicksRunner(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_DASHBOARD_URL", "https://argos.example/argos/dashboard.html")
	clicked := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/click" {
			t.Fatalf("unexpected runner path: %s", r.URL.Path)
		}
		clicked = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	t.Setenv("MESHCLAW_ARGOS_UI_RUNNER_URL", server.URL)
	targetID := "argos-assistant-purchase-short-approval-execute"
	rememberPendingShoppingDirectPurchaseWithURLAndBlocker(
		ListenOptions{TargetID: targetID, Mode: "assistant"},
		"생수 구매해",
		"풀무원샘물 무라벨 생수, 500ml, 20개",
		"https://www.coupang.com/vp/products/8288410923",
		"",
	)
	readyDecision := formatShoppingCheckoutDecisionReply(browserauto.Link{Text: "풀무원샘물 무라벨 생수, 500ml, 20개"}, checkoutScreenFields{
		Product:  "풀무원샘물 무라벨 생수",
		Option:   "500ml 20개",
		Quantity: "1",
		Total:    "10,990원",
		Delivery: "내일 도착",
		Address:  "맞음",
		Payment:  "맞음",
		Button:   "x=456, y=789",
	})
	if err := appendSignalHistory(targetID, "구매 가능 판정해줘", readyDecision); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant", Execute: true}, "ㅇㅇ")
	if !clicked {
		t.Fatalf("short approval did not click the runner:\n%s", reply)
	}
	for _, want := range []string{
		"최종 구매 클릭을 실행했습니다.",
		"확인된 최종 주문 버튼 좌표를 한 번 클릭했습니다.",
		"상품: 풀무원샘물 무라벨 생수 / 500ml 20개",
		"총액: 10,990원",
		"다음 확인: 주문 완료 화면 또는 쿠팡 주문목록",
		"실행 증거:",
		"Evidence 탭: https://argos.example/argos/dashboard.html?evidence=",
		"#evidence",
		"이 구매의 대기 상태를 정리했습니다.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("short approval execute reply missing %q:\n%s", want, reply)
		}
	}
	if pending, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false); ok && pending.Kind == "shopping_direct_purchase" {
		t.Fatalf("successful final click should clear pending direct purchase: %#v", pending)
	}
}

func TestPurchaseFinalClickReplyReadsPostClickOrderResult(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_DASHBOARD_URL", "https://argos.example/argos/dashboard.html")
	oldCapture := shoppingCheckoutScreenCapture
	oldOCR := shoppingCheckoutScreenOCR
	defer func() {
		shoppingCheckoutScreenCapture = oldCapture
		shoppingCheckoutScreenOCR = oldOCR
	}()
	shoppingCheckoutScreenCapture = func(ctx context.Context, output string) osauto.Result {
		if err := os.WriteFile(output, []byte("fake purchase proof"), 0600); err != nil {
			t.Fatal(err)
		}
		return osauto.Result{Kind: "meshclaw_automation_screen_capture", Action: "screen_capture", OK: true}
	}
	shoppingCheckoutScreenOCR = func(path string) (string, string) {
		return strings.Join([]string{
			"주문이 완료되었습니다",
			"주문번호: 123456789012",
			"내일 도착",
			"배송 상태: 배송중 운송장번호: 987654321000",
			"택배사: 쿠팡 로켓배송 운송장번호: 987654321000",
		}, "\n"), ""
	}
	clicked := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/click" {
			t.Fatalf("unexpected runner path: %s", r.URL.Path)
		}
		clicked = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	t.Setenv("MESHCLAW_ARGOS_UI_RUNNER_URL", server.URL)

	reply := executePurchaseFinalClickReplyFor("ko", purchaseExecutionFields{
		Merchant:     "쿠팡",
		Item:         "풀무원샘물 무라벨 생수 / 500ml 20개",
		Total:        "10,990원",
		Delivery:     "내일 도착",
		Shipping:     "맞음",
		Payment:      "맞음",
		URL:          "https://cart.coupang.com/cartView.pang",
		X:            456,
		Y:            789,
		Proof:        "버튼 위치 확인됨",
		Confirmation: "ㅇㅇ",
	})
	if !clicked {
		t.Fatalf("purchase click did not call runner:\n%s", reply)
	}
	for _, want := range []string{
		"최종 구매 클릭을 실행했습니다.",
		"주문 결과 화면에서 읽은 내용:",
		"주문 상태: 주문 완료 화면 확인",
		"주문번호: ********9012",
		"도착 예정: 내일 도착",
		"배송/택배 표시: 쿠팡 로켓배송",
		"운송장/조회번호: ********1000",
		"다음 Signal 액션: `배송 조회해줘` 또는 `방금 주문 확인해줘`",
		"Evidence 탭: https://argos.example/argos/dashboard.html?evidence=",
		"#evidence",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("post-click order result missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"주문번호: 123456789012", "운송장/조회번호: 987654321000", "987654321000"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("post-click order result should mask raw identifier %q:\n%s", bad, reply)
		}
	}
}

func TestParsePurchasePostClickOCRScrubsIdentifiersFromStatusAndCarrier(t *testing.T) {
	result := parsePurchasePostClickOCR(strings.Join([]string{
		"배송 상태: 배송중 운송장번호: 987654321000",
		"택배사: CJ대한통운 운송장번호: 987654321000",
	}, "\n"), "")
	if result.Status != "배송중" {
		t.Fatalf("status should strip tracking identifiers, got %#v", result)
	}
	if result.Carrier != "CJ대한통운" {
		t.Fatalf("carrier should strip tracking identifiers, got %#v", result)
	}
	if result.TrackingNumber != "987654321000" {
		t.Fatalf("tracking number should still be extracted separately, got %#v", result)
	}

	english := parsePurchasePostClickOCR(strings.Join([]string{
		"Delivery status: in transit Tracking number: 1Z9999999999",
		"Carrier: Coupang Rocket delivery Tracking number: 1Z9999999999",
	}, "\n"), "")
	if english.Status != "in transit" {
		t.Fatalf("English status should strip tracking identifiers, got %#v", english)
	}
	if english.Carrier != "Coupang Rocket delivery" {
		t.Fatalf("English carrier should strip tracking identifiers, got %#v", english)
	}
	if english.TrackingNumber != "1Z9999999999" {
		t.Fatalf("English tracking number should still be extracted separately, got %#v", english)
	}

	preparing := parsePurchasePostClickOCR("주문 상태: 상품준비 운송장번호: 123456789012", "")
	if preparing.Status != "상품 준비 중" {
		t.Fatalf("Korean compact preparing status should be canonicalized, got %#v", preparing)
	}
	delivered := parsePurchasePostClickOCR("Shipment status: delivered Tracking number: 1Z9999999999", "")
	if delivered.Status != "delivered" {
		t.Fatalf("English delivered status should be canonicalized, got %#v", delivered)
	}
}

func TestAssistantReplyPurchaseFinalApprovalWithReadyDecisionShowsClickHandoff(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	targetID := "argos-assistant-purchase-final-handoff"
	readyDecision := formatShoppingCheckoutDecisionReply(browserauto.Link{Text: "풀무원샘물 무라벨 생수, 500ml, 20개"}, checkoutScreenFields{
		Product:  "풀무원샘물 무라벨 생수",
		Option:   "500ml 20개",
		Quantity: "1",
		Total:    "10,990원",
		Delivery: "내일 도착",
		Address:  "맞음",
		Payment:  "맞음",
		Button:   "화면 하단 파란 버튼",
	})
	if err := appendSignalHistory(targetID, "구매 가능 판정해줘", readyDecision); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "구매 실행 승인")
	for _, want := range []string{
		"구매 실행 승인을 확인했습니다",
		"최종 클릭 실행 직전 단계",
		"최근 구매 가능 판정: 준비됨",
		"`meshclaw_purchase_click`",
		"체크아웃/주문 화면 URL",
		"최종 주문 버튼 좌표 x/y",
		"Signal에 이어 보낼 최종 실행 템플릿:",
		"배송지 표시: 맞음",
		"결제수단 표시: 맞음",
		"화면 확인: 버튼 위치 확인됨",
		"확인 문구: 구매 실행 승인",
		"최종 버튼을 한 번 클릭",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("purchase final handoff missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "주문 완료", "최종 버튼을 눌렀습니다"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("purchase final handoff should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyPurchaseFinalNaturalAliasWithReadyDecisionShowsClickHandoff(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	targetID := "argos-assistant-purchase-final-handoff-natural-alias"
	readyDecision := formatShoppingCheckoutDecisionReply(browserauto.Link{Text: "풀무원샘물 무라벨 생수, 500ml, 20개"}, checkoutScreenFields{
		Product:  "풀무원샘물 무라벨 생수",
		Option:   "500ml 20개",
		Quantity: "1",
		Total:    "10,990원",
		Delivery: "내일 도착",
		Address:  "맞음",
		Payment:  "맞음",
		Button:   "화면 하단 파란 버튼",
	})
	if err := appendSignalHistory(targetID, "구매 가능 판정해줘", readyDecision); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "구매 승인")
	for _, want := range []string{
		"구매 실행 승인을 확인했습니다",
		"최종 클릭 실행 직전 단계",
		"최근 구매 가능 판정: 준비됨",
		"`meshclaw_purchase_click`",
		"Signal에 이어 보낼 최종 실행 템플릿:",
		"확인 문구: 구매 승인",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("purchase final natural alias handoff missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "주문 완료", "최종 버튼을 눌렀습니다"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("purchase final natural alias handoff should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyPurchaseFinalApprovalWithReadyDecisionEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	targetID := "argos-assistant-purchase-final-handoff-en"
	readyDecision := formatShoppingCheckoutDecisionReply(browserauto.Link{Text: "Pulmuone bottled water 500ml 20-pack"}, checkoutScreenFields{
		Product:  "Pulmuone bottled water",
		Option:   "500ml 20-pack",
		Quantity: "1",
		Total:    "10,990 KRW",
		Delivery: "tomorrow",
		Address:  "yes",
		Payment:  "yes",
		Button:   "blue button at bottom",
	})
	if err := appendSignalHistory(targetID, "purchase readiness decision", readyDecision); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "purchase execution approved")
	for _, want := range []string{
		"Purchase execution approval is confirmed",
		"pre-final-click execution step",
		"`meshclaw_purchase_click`",
		"Checkout/order screen URL",
		"Final order button coordinates x/y",
		"Final execution template to send next in Signal:",
		"Address visible: yes",
		"Payment method visible: yes",
		"Screen proof: button location confirmed",
		"Confirmation text: purchase execution approved",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English purchase final handoff missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English purchase final handoff should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"purchase completed", "order completed", "clicked the final button"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English purchase final handoff should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyPurchaseFinalApprovalWithReadyDecisionEnglishWithoutEnvLanguage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	targetID := "argos-assistant-purchase-final-handoff-en-no-env"
	readyDecision := formatShoppingCheckoutDecisionReplyFor("en", browserauto.Link{Text: "Pulmuone bottled water 500ml 20-pack"}, checkoutScreenFields{
		Product:  "Pulmuone bottled water",
		Option:   "500ml 20-pack",
		Quantity: "1",
		Total:    "10,990 KRW",
		Delivery: "tomorrow",
		Address:  "yes",
		Payment:  "yes",
		Button:   "blue button at bottom",
	})
	if err := appendSignalHistory(targetID, "purchase readiness decision", readyDecision); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "purchase execution approved")
	for _, want := range []string{
		"Purchase execution approval is confirmed",
		"pre-final-click execution step",
		"Recent purchase readiness decision: ready",
		"The order button has not been clicked yet.",
		"`meshclaw_purchase_click`",
		"Checkout/order screen URL",
		"Final order button coordinates x/y",
		"Final execution template to send next in Signal:",
		"Merchant: Coupang",
		"Address visible: yes",
		"Payment method visible: yes",
		"Screen proof: button location confirmed",
		"Confirmation text: purchase execution approved",
		"Only when these values and internal approve=true are present",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English no-env purchase final handoff missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English no-env purchase final handoff should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"구매 실행 승인", "최종 클릭", "purchase completed", "order completed", "clicked the final button"} {
		if strings.Contains(strings.ToLower(reply), strings.ToLower(bad)) {
			t.Fatalf("English no-env purchase final handoff should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyPurchaseFinalExecutionTemplateReady(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, strings.Join([]string{
		"Signal에 이어 보낼 최종 실행 템플릿:",
		"판매처: 쿠팡",
		"상품명: 풀무원샘물 무라벨 생수 500ml 20개",
		"총액: 10,990원",
		"도착 예정일: 내일 도착",
		"배송지 표시: 맞음",
		"결제수단 표시: 맞음",
		"체크아웃 URL: https://www.coupang.com/checkout/order",
		"최종 주문 버튼 좌표: x=456, y=789",
		"화면 확인: 버튼 위치 확인됨",
		"확인 문구: 구매 실행 승인",
	}, "\n"))
	for _, want := range []string{
		"최종 구매 클릭 실행값이 준비되었습니다.",
		"판정: 실행 도구 입력값 완성",
		"아직 최종 주문 버튼은 누르지 않았습니다.",
		"승인 대기 상태:",
		"`승인 대기열 보여줘`",
		"`구매 플로우 중지`",
		"이 최종 클릭 후보는 대기열에서 숨겨집니다.",
		"판매처: 쿠팡",
		"상품명: 풀무원샘물 무라벨 생수 500ml 20개",
		"총액: 10,990원",
		"도착 예정일: 내일 도착",
		"체크아웃 URL: https://www.coupang.com/checkout/order",
		"최종 주문 버튼 좌표: x=456, y=789",
		"화면 확인: 버튼 위치 확인됨",
		"`meshclaw_purchase_click`",
		"실제 도구 호출 인자:",
		"```json",
		`"merchant": "쿠팡"`,
		`"item": "풀무원샘물 무라벨 생수 500ml 20개"`,
		`"delivery": "내일 도착"`,
		`"url": "https://www.coupang.com/checkout/order"`,
		`"x": 456`,
		`"y": 789`,
		`"proof": "버튼 위치 확인됨"`,
		"execute=true",
		`"execute": true`,
		"approve=true",
		`"approve": true`,
		"이 답장만으로는 클릭하지 않습니다.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("purchase final execution template missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "주문 완료", "최종 주문 버튼을 눌렀습니다"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("purchase final execution template should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyPurchaseFinalExecutionTemplateMissingValues(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, strings.Join([]string{
		"Signal에 이어 보낼 최종 실행 템플릿:",
		"판매처: 쿠팡",
		"상품명: 화면 그대로",
		"총액: 화면 그대로",
		"도착 예정일: 화면 그대로",
		"배송지 표시: 맞음",
		"결제수단 표시: 맞음",
		"체크아웃 URL: 화면 주소",
		"최종 주문 버튼 좌표: x=..., y=...",
		"확인 문구: 구매 실행 승인",
	}, "\n"))
	for _, want := range []string{
		"최종 구매 클릭 실행값이 아직 부족합니다.",
		"아직 최종 주문 버튼은 누르지 않았습니다.",
		"다시 채울 값:",
		"상품명",
		"총액",
		"도착 예정일",
		"체크아웃/주문 화면 URL",
		"최종 주문 버튼 좌표 x/y",
		"화면 확인 증거",
		"`화면 확인: 버튼 위치 확인됨`",
		"같은 템플릿으로 다시 보내주세요.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("purchase final execution missing-values reply missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "판정: 실행 도구 입력값 완성") || strings.Contains(reply, "구매 완료") || strings.Contains(reply, "주문 완료") {
		t.Fatalf("missing-values reply should not claim ready or completed:\n%s", reply)
	}
}

func TestAssistantReplyPurchaseFinalExecutionTemplateRejectsUnconfirmedProof(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, strings.Join([]string{
		"Signal에 이어 보낼 최종 실행 템플릿:",
		"판매처: 쿠팡",
		"상품명: 풀무원샘물 무라벨 생수 500ml 20개",
		"총액: 10,990원",
		"도착 예정일: 내일 도착",
		"배송지 표시: 맞음",
		"결제수단 표시: 맞음",
		"체크아웃 URL: https://www.coupang.com/checkout/order",
		"최종 주문 버튼 좌표: x=456, y=789",
		"화면 확인: 버튼이 안 보임",
		"확인 문구: 구매 실행 승인",
	}, "\n"))
	for _, want := range []string{
		"최종 구매 클릭 실행값이 아직 부족합니다.",
		"아직 최종 주문 버튼은 누르지 않았습니다.",
		"다시 채울 값:",
		"화면 확인 증거",
		"`화면 확인: 버튼 위치 확인됨`",
		"같은 템플릿으로 다시 보내주세요.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("unconfirmed proof reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"판정: 실행 도구 입력값 완성", "실제 도구 호출 인자:", "```json", `"execute": true`, "구매 완료", "주문 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("unconfirmed proof reply should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyPurchaseFinalExecutionTemplateRejectsRawAddressPayment(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, strings.Join([]string{
		"Signal에 이어 보낼 최종 실행 템플릿:",
		"판매처: 쿠팡",
		"상품명: 풀무원샘물 무라벨 생수 500ml 20개",
		"총액: 10,990원",
		"도착 예정일: 내일 도착",
		"배송지: 서울시 테스트구 테스트동",
		"결제수단: 쿠페이 머니",
		"체크아웃 URL: https://www.coupang.com/checkout/order",
		"최종 주문 버튼 좌표: x=456, y=789",
		"화면 확인: 버튼 위치 확인됨",
		"확인 문구: 구매 실행 승인",
	}, "\n"))
	for _, want := range []string{
		"최종 구매 클릭 실행값이 아직 부족합니다.",
		"아직 최종 주문 버튼은 누르지 않았습니다.",
		"다시 채울 값:",
		"배송지 표시",
		"결제수단 표시",
		"배송지/결제수단은 원문을 쓰지 말고",
		"`배송지 표시: 맞음`",
		"`결제수단 표시: 맞음`",
		"같은 템플릿으로 다시 보내주세요.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("raw address/payment final execution reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"판정: 실행 도구 입력값 완성", "실제 도구 호출 인자:", "```json", `"execute": true`, "서울시 테스트구", "쿠페이 머니", "구매 완료", "주문 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("raw address/payment final execution reply should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyPurchaseFinalExecutionTemplateRejectsMultipleAddresses(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, strings.Join([]string{
		"Signal에 이어 보낼 최종 실행 템플릿:",
		"판매처: 쿠팡",
		"상품명: 풀무원샘물 무라벨 생수 500ml 20개",
		"총액: 10,990원",
		"도착 예정일: 내일 도착",
		"배송지 표시: 2개 보임",
		"결제수단 표시: 맞음",
		"체크아웃 URL: https://www.coupang.com/checkout/order",
		"최종 주문 버튼 좌표: x=456, y=789",
		"화면 확인: 버튼 위치 확인됨",
		"확인 문구: 구매 실행 승인",
	}, "\n"))
	for _, want := range []string{
		"최종 구매 클릭 실행값이 아직 부족합니다.",
		"아직 최종 주문 버튼은 누르지 않았습니다.",
		"다시 채울 값:",
		"배송지 표시",
		"배송지가 2개/여러 개 보이면 최종 클릭 실행값으로 사용할 수 없습니다.",
		"`배송지 표시: 맞음`",
		"같은 템플릿으로 다시 보내주세요.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("multiple-address final execution reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"판정: 실행 도구 입력값 완성", "실제 도구 호출 인자:", "```json", `"execute": true`, "구매 완료", "주문 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("multiple-address final execution reply should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyPurchaseFinalExecutionTemplateRejectsMultiplePaymentMethods(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, strings.Join([]string{
		"Signal에 이어 보낼 최종 실행 템플릿:",
		"판매처: 쿠팡",
		"상품명: 풀무원샘물 무라벨 생수 500ml 20개",
		"총액: 10,990원",
		"도착 예정일: 내일 도착",
		"배송지 표시: 맞음",
		"결제수단 표시: 2개 보임",
		"체크아웃 URL: https://www.coupang.com/checkout/order",
		"최종 주문 버튼 좌표: x=456, y=789",
		"화면 확인: 버튼 위치 확인됨",
		"확인 문구: 구매 실행 승인",
	}, "\n"))
	for _, want := range []string{
		"최종 구매 클릭 실행값이 아직 부족합니다.",
		"아직 최종 주문 버튼은 누르지 않았습니다.",
		"다시 채울 값:",
		"결제수단 표시",
		"결제수단이 2개/여러 개 보이면 최종 클릭 실행값으로 사용할 수 없습니다.",
		"`결제수단 표시: 맞음`",
		"같은 템플릿으로 다시 보내주세요.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("multiple-payment final execution reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"판정: 실행 도구 입력값 완성", "실제 도구 호출 인자:", "```json", `"execute": true`, "구매 완료", "주문 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("multiple-payment final execution reply should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyPurchaseFinalExecutionTemplateRejectsUnclearDelivery(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, strings.Join([]string{
		"Signal에 이어 보낼 최종 실행 템플릿:",
		"판매처: 쿠팡",
		"상품명: 풀무원샘물 무라벨 생수 500ml 20개",
		"총액: 10,990원",
		"도착 예정일: 배송 옵션 여러 개 보임",
		"배송지 표시: 맞음",
		"결제수단 표시: 맞음",
		"체크아웃 URL: https://www.coupang.com/checkout/order",
		"최종 주문 버튼 좌표: x=456, y=789",
		"화면 확인: 버튼 위치 확인됨",
		"확인 문구: 구매 실행 승인",
	}, "\n"))
	for _, want := range []string{
		"최종 구매 클릭 실행값이 아직 부족합니다.",
		"아직 최종 주문 버튼은 누르지 않았습니다.",
		"다시 채울 값:",
		"도착 예정일",
		"도착 예정일이 여러 개/변경/지연/불확실하면 최종 클릭 실행값으로 사용할 수 없습니다.",
		"`도착 예정일: 화면 그대로`",
		"같은 템플릿으로 다시 보내주세요.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("unclear-delivery final execution reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"판정: 실행 도구 입력값 완성", "실제 도구 호출 인자:", "```json", `"execute": true`, "구매 완료", "주문 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("unclear-delivery final execution reply should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyPurchaseFinalExecutionTemplateRejectsCardAlias(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, strings.Join([]string{
		"Signal에 이어 보낼 최종 실행 템플릿:",
		"판매처: 쿠팡",
		"상품명: 풀무원샘물 무라벨 생수 500ml 20개",
		"총액: 10,990원",
		"도착 예정일: 내일 도착",
		"배송지 표시: 맞음",
		"카드 표시: 2개 보임",
		"체크아웃 URL: https://www.coupang.com/checkout/order",
		"최종 주문 버튼 좌표: x=456, y=789",
		"화면 확인: 버튼 위치 확인됨",
		"확인 문구: 구매 실행 승인",
	}, "\n"))
	for _, want := range []string{
		"최종 구매 클릭 실행값이 아직 부족합니다.",
		"결제수단 표시",
		"결제수단이 2개/여러 개 보이면 최종 클릭 실행값으로 사용할 수 없습니다.",
		"`결제수단 표시: 맞음`",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("card-alias final execution reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"판정: 실행 도구 입력값 완성", "실제 도구 호출 인자:", "```json", `"execute": true`, "구매 완료", "주문 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("card-alias final execution reply should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyPurchaseFinalExecutionTemplateAcceptsButtonLocationAlias(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, strings.Join([]string{
		"Signal에 이어 보낼 최종 실행 템플릿:",
		"판매처: 쿠팡",
		"상품명: 풀무원샘물 무라벨 생수 500ml 20개",
		"총액: 10,990원",
		"도착 예정일: 내일 도착",
		"배송지 표시: 맞음",
		"결제수단 표시: 맞음",
		"체크아웃 URL: https://www.coupang.com/checkout/order",
		"최종 주문 버튼 위치: x=456, y=789",
		"화면 확인: 버튼 위치 확인됨",
		"확인 문구: 구매 실행 승인",
	}, "\n"))
	for _, want := range []string{
		"최종 구매 클릭 실행값이 준비되었습니다.",
		"최종 주문 버튼 좌표: x=456, y=789",
		"`meshclaw_purchase_click`",
		`"x": 456`,
		`"y": 789`,
		`"execute": true`,
		`"approve": true`,
		"이 답장만으로는 클릭하지 않습니다.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("button-location alias final execution reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "주문 완료", "최종 주문 버튼을 눌렀습니다"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("button-location alias final execution reply should not claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyPurchaseFinalExecutionTemplateEnglishUsesLanguagePack(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "en")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, strings.Join([]string{
		"Final execution template:",
		"Merchant: Coupang",
		"Item: Pulmuone bottled water 500ml 20-pack",
		"Total: 10,990 KRW",
		"Arrival estimate: tomorrow",
		"Address visible: yes",
		"Payment method visible: yes",
		"Checkout URL: https://www.coupang.com/checkout/order",
		"Final order button coordinates: x=456, y=789",
		"Screen proof: button location confirmed",
		"Confirmation text: purchase execution approved",
	}, "\n"))
	for _, want := range []string{
		"Final purchase-click execution values are ready.",
		"Decision: execution-tool inputs are complete.",
		"The final order button has not been clicked yet.",
		"Approval-queue status:",
		"`approval queue`",
		"`stop the purchase flow`",
		"final-click candidate will be hidden from the queue",
		"Merchant: Coupang",
		"Item: Pulmuone bottled water 500ml 20-pack",
		"Arrival estimate: tomorrow",
		"Checkout URL: https://www.coupang.com/checkout/order",
		"Final order button coordinates: x=456, y=789",
		"Screen proof: button location confirmed",
		"`meshclaw_purchase_click`",
		"Actual tool-call arguments:",
		"```json",
		`"merchant": "Coupang"`,
		`"item": "Pulmuone bottled water 500ml 20-pack"`,
		`"delivery": "tomorrow"`,
		`"url": "https://www.coupang.com/checkout/order"`,
		`"x": 456`,
		`"y": 789`,
		`"proof": "button location confirmed"`,
		"execute=true",
		`"execute": true`,
		"approve=true",
		`"approve": true`,
		"This reply alone does not click anything.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English purchase final execution template missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English purchase final execution template should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"purchase completed", "order completed", "clicked the final order button"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English purchase final execution template should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyPurchaseFinalExecutionTemplateFollowsEnglishInputWithoutEnvLanguage(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, strings.Join([]string{
		"Final execution template:",
		"Merchant: Coupang",
		"Item: Pulmuone bottled water 500ml 20-pack",
		"Total: 10,990 KRW",
		"Arrival estimate: tomorrow",
		"Address visible: yes",
		"Payment method visible: yes",
		"Checkout URL: https://www.coupang.com/checkout/order",
		"Final order button location: x=456, y=789",
		"Screen proof: button location confirmed",
		"Confirmation text: purchase execution approved",
	}, "\n"))
	for _, want := range []string{
		"Final purchase-click execution values are ready.",
		"Final order button coordinates: x=456, y=789",
		"`meshclaw_purchase_click`",
		`"merchant": "Coupang"`,
		`"delivery": "tomorrow"`,
		`"x": 456`,
		`"y": 789`,
		`"execute": true`,
		`"approve": true`,
		"This reply alone does not click anything.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English input final execution reply missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English input final execution reply should not expose Korean when env language is unset:\n%s", reply)
	}
	for _, bad := range []string{"구매", "주문", "클릭했습니다", "purchase completed", "order completed", "clicked the final order button"} {
		if strings.Contains(strings.ToLower(reply), strings.ToLower(bad)) {
			t.Fatalf("English input final execution reply should not claim or expose %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyPurchaseFinalExecutionTemplateRejectsMultipleAddressesEnglishUsesLanguagePack(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "en")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, strings.Join([]string{
		"Final execution template:",
		"Merchant: Coupang",
		"Item: Pulmuone bottled water 500ml 20-pack",
		"Total: 10,990 KRW",
		"Arrival estimate: tomorrow",
		"Address visible: two shipping addresses",
		"Payment method visible: yes",
		"Checkout URL: https://www.coupang.com/checkout/order",
		"Final order button coordinates: x=456, y=789",
		"Screen proof: button location confirmed",
		"Confirmation text: purchase execution approved",
	}, "\n"))
	for _, want := range []string{
		"Final purchase-click execution values are still incomplete.",
		"The final order button has not been clicked yet.",
		"Fill these values again:",
		"Address visibility",
		"multiple shipping addresses are visible",
		"`Address visible: yes`",
		"resend the same template",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English multiple-address final execution reply missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English multiple-address final execution reply should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"execution-tool inputs are complete", "actual tool-call arguments:", "```json", `"execute": true`, "purchase completed", "order completed"} {
		if strings.Contains(strings.ToLower(reply), strings.ToLower(bad)) {
			t.Fatalf("English multiple-address final execution reply should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyPurchaseFinalExecutionTemplateRejectsMultiplePaymentMethodsEnglishUsesLanguagePack(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "en")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, strings.Join([]string{
		"Final execution template:",
		"Merchant: Coupang",
		"Item: Pulmuone bottled water 500ml 20-pack",
		"Total: 10,990 KRW",
		"Arrival estimate: tomorrow",
		"Address visible: yes",
		"Payment method visible: two cards",
		"Checkout URL: https://www.coupang.com/checkout/order",
		"Final order button coordinates: x=456, y=789",
		"Screen proof: button location confirmed",
		"Confirmation text: purchase execution approved",
	}, "\n"))
	for _, want := range []string{
		"Final purchase-click execution values are still incomplete.",
		"The final order button has not been clicked yet.",
		"Fill these values again:",
		"Payment-method visibility",
		"multiple payment methods are visible",
		"`Payment method visible: yes`",
		"resend the same template",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English multiple-payment final execution reply missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English multiple-payment final execution reply should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"execution-tool inputs are complete", "actual tool-call arguments:", "```json", `"execute": true`, "purchase completed", "order completed"} {
		if strings.Contains(strings.ToLower(reply), strings.ToLower(bad)) {
			t.Fatalf("English multiple-payment final execution reply should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyPurchaseFinalExecutionTemplateRejectsUnclearDeliveryEnglishUsesLanguagePack(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "en")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, strings.Join([]string{
		"Final execution template:",
		"Merchant: Coupang",
		"Item: Pulmuone bottled water 500ml 20-pack",
		"Total: 10,990 KRW",
		"Arrival estimate: multiple options visible",
		"Address visible: yes",
		"Payment method visible: yes",
		"Checkout URL: https://www.coupang.com/checkout/order",
		"Final order button coordinates: x=456, y=789",
		"Screen proof: button location confirmed",
		"Confirmation text: purchase execution approved",
	}, "\n"))
	for _, want := range []string{
		"Final purchase-click execution values are still incomplete.",
		"The final order button has not been clicked yet.",
		"Fill these values again:",
		"Arrival estimate",
		"When arrival estimates are multiple, changed, delayed, or uncertain",
		"`Arrival estimate: copy exactly as shown`",
		"resend the same template",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English unclear-delivery final execution reply missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English unclear-delivery final execution reply should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"execution-tool inputs are complete", "actual tool-call arguments:", "```json", `"execute": true`, "purchase completed", "order completed"} {
		if strings.Contains(strings.ToLower(reply), strings.ToLower(bad)) {
			t.Fatalf("English unclear-delivery final execution reply should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyPurchaseFinalExecutionTemplateRejectsUnconfirmedProofEnglishUsesLanguagePack(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "en")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, strings.Join([]string{
		"Final execution template:",
		"Merchant: Coupang",
		"Item: Pulmuone bottled water 500ml 20-pack",
		"Total: 10,990 KRW",
		"Arrival estimate: tomorrow",
		"Address visible: yes",
		"Payment method visible: yes",
		"Checkout URL: https://www.coupang.com/checkout/order",
		"Final order button coordinates: x=456, y=789",
		"Screen proof: button not visible",
		"Confirmation text: purchase execution approved",
	}, "\n"))
	for _, want := range []string{
		"Final purchase-click execution values are still incomplete.",
		"The final order button has not been clicked yet.",
		"Fill these values again:",
		"screen proof",
		"`Screen proof: button location confirmed`",
		"resend the same template",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English unconfirmed proof reply missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English unconfirmed proof reply should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"execution-tool inputs are complete", "actual tool-call arguments:", "```json", `"execute": true`, "purchase completed", "order completed"} {
		if strings.Contains(strings.ToLower(reply), strings.ToLower(bad)) {
			t.Fatalf("English unconfirmed proof reply should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyPurchaseFinalExecutionTemplateButtonLocationAliasEnglishUsesLanguagePack(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "en")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, strings.Join([]string{
		"Final execution template:",
		"Merchant: Coupang",
		"Item: Pulmuone bottled water 500ml 20-pack",
		"Total: 10,990 KRW",
		"Arrival estimate: tomorrow",
		"Address visible: yes",
		"Payment method visible: yes",
		"Checkout URL: https://www.coupang.com/checkout/order",
		"Final order button location: x=456, y=789",
		"Screen proof: button location confirmed",
		"Confirmation text: purchase execution approved",
	}, "\n"))
	for _, want := range []string{
		"Final purchase-click execution values are ready.",
		"Final order button coordinates: x=456, y=789",
		"`meshclaw_purchase_click`",
		"Arrival estimate: tomorrow",
		`"x": 456`,
		`"y": 789`,
		`"execute": true`,
		`"approve": true`,
		"This reply alone does not click anything.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English button-location alias final execution reply missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English button-location alias final execution reply should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"purchase completed", "order completed", "clicked the final order button"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English button-location alias final execution reply should not claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyPurchaseFinalExecutionTemplateRejectsRawAddressPaymentEnglishUsesLanguagePack(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "en")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, strings.Join([]string{
		"Final execution template:",
		"Merchant: Coupang",
		"Item: Pulmuone bottled water 500ml 20-pack",
		"Total: 10,990 KRW",
		"Arrival estimate: tomorrow",
		"Address: 123 Test Street, Seoul",
		"Payment method: Coupay Money",
		"Checkout URL: https://www.coupang.com/checkout/order",
		"Final order button coordinates: x=456, y=789",
		"Screen proof: button location confirmed",
		"Confirmation text: purchase execution approved",
	}, "\n"))
	for _, want := range []string{
		"Final purchase-click execution values are still incomplete.",
		"The final order button has not been clicked yet.",
		"Fill these values again:",
		"Address visibility",
		"Payment-method visibility",
		"do not send raw values",
		"`Address visible: yes`",
		"`Payment method visible: yes`",
		"resend the same template",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English raw address/payment final execution reply missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English raw address/payment final execution reply should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"execution-tool inputs are complete", "actual tool-call arguments:", "```json", `"execute": true`, "123 Test Street", "Coupay Money", "purchase completed", "order completed"} {
		if strings.Contains(strings.ToLower(reply), strings.ToLower(bad)) {
			t.Fatalf("English raw address/payment final execution reply should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyCoupangCheckoutReviewRequestStopsBeforeClick(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "최종 화면의 상품명, 옵션, 총액, 배송지, 결제수단을 읽어줘")
	for _, want := range []string{"쿠팡 최종 구매 화면 확인 단계", "아직 구매 버튼은 누르지 않았습니다", "상품명과 옵션", "수량", "총액", "배송지", "결제수단", "도착 예정일", "최종 주문 버튼 위치", "구매 실행 승인", "복사해서 채울 구매 판정 템플릿", "상품명: 화면 그대로", "배송지 표시: 맞음/아님/2개 보임", "결제수단 표시: 맞음/아님/2개 보임", "최종 주문 버튼 위치: 보임/안 보임"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("checkout review reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"상품 구매는 결제 직전까지만", "google.com/search", "구매 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("checkout review should stay in review route, found %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyCoupangCheckoutReviewEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "read the final screen product, option, total, address, and payment method")
	for _, want := range []string{
		"Coupang final purchase-screen review step.",
		"The purchase button has not been clicked yet.",
		"Product name and options",
		"Quantity",
		"Total",
		"Address visibility",
		"Payment-method visibility",
		"Arrival estimate",
		"Final order button location",
		"purchase execution approved",
		"Copy and fill this purchase-decision template:",
		"Product: copy exactly as shown",
		"Address visible: yes/no/two shipping addresses",
		"Payment method visible: yes/no/two cards",
		"Final order button location: visible/not visible",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English checkout review reply missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English checkout review reply should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"purchase completed", "order completed", "argos handled a mac action"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("checkout review reply should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyCoupangCheckoutReviewEnglishDetectedWithoutEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-checkout-review-en", Mode: "assistant"}, "read the final product, option, total, address visibility, and payment-method visibility")
	for _, want := range []string{
		"Coupang final purchase-screen review step.",
		"The purchase button has not been clicked yet.",
		"Product name and options",
		"Address visibility",
		"Payment-method visibility",
		"purchase execution approved",
		"Copy and fill this purchase-decision template:",
		"Product: copy exactly as shown",
		"Address visible: yes/no/two shipping addresses",
		"Payment method visible: yes/no/two cards",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English checkout review reply missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English checkout review reply should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"쿠팡 최종", "구매 실행 승인", "복사해서 채울", "purchase completed", "order completed"} {
		if strings.Contains(strings.ToLower(reply), strings.ToLower(bad)) {
			t.Fatalf("English checkout review reply should not contain %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyCoupangDeliveryChoiceFollowUpUsesRecentContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-coupang-delivery-followup"
	if err := appendSignalHistory(targetID, "쿠팡 최종 화면 확인해", "쿠팡 최종 구매 화면 확인 단계입니다.\n도착 예정일: 배송 옵션이 여러 개 보임\n아직 구매 버튼은 누르지 않았습니다."); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "배송 옵션이 여러 개야. 확인해")
	for _, want := range []string{
		"쿠팡 배송 옵션 확인 단계입니다.",
		"아직 장바구니/결제/최종 주문 버튼은 누르지 않았습니다.",
		"이번 주문에 쓸 배송 방식, 도착 예정일, 배송비",
		"쿠팡 장바구니 또는 결제 직전 화면의 배송 옵션 영역",
		"로켓배송, 무료배송, 새벽배송, 정기배송",
		"도착 예정일, 배송비, 총액 변화",
		"배송 옵션 후보를 구매값만으로 구분하는 방식:",
		"`배송 후보 A: 배송 방식·도착 예정일·배송비·총액 변화만`",
		"`배송 후보 B: 배송 방식·도착 예정일·배송비·총액 변화만`",
		"`선택 필요: A/B/기본배송 유지`처럼 보내고, 주소 원문이나 결제 민감값은 보내지 않습니다.",
		"`배송 표시: 맞음`",
		"`배송 표시: 여러 개 보임`이면 구매 가능 판정은 계속 중지됩니다.",
		"배송 옵션이 확정되기 전에는 장바구니, 결제, 최종 주문 클릭을 실행하지 않습니다.",
		"실행 상태:",
		"열린 주소: https://cart.coupang.com/cartView.pang",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("delivery follow-up guide missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"쿠팡 배송지 2개 확인 단계", "구매 완료", "주문 완료", "장바구니에 담았습니다", "최종 주문 버튼을 눌렀습니다"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("delivery follow-up should not misroute or claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyCoupangDeliveryChoiceEnglishFollowUpDoesNotMisrouteAsAddress(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-coupang-delivery-followup-en"
	if err := appendSignalHistory(targetID, "read the Coupang final screen", "Coupang final purchase-screen review step.\nArrival estimate: multiple shipping options visible\nThe purchase button has not been clicked yet."); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "multiple shipping options, check it")
	for _, want := range []string{
		"Coupang delivery-option verification step.",
		"No cart, payment, or final order button has been clicked yet.",
		"verify the delivery method, arrival estimate, and shipping fee",
		"delivery-option area on the Coupang cart or pre-payment screen",
		"Rocket delivery, free shipping, dawn delivery, or recurring delivery",
		"arrival estimate, shipping fee, total change",
		"How to distinguish delivery-option candidates using purchase values only:",
		"`Delivery candidate A: delivery method, arrival estimate, shipping fee, total change only`",
		"`Delivery candidate B: delivery method, arrival estimate, shipping fee, total change only`",
		"`Choice needed: A/B/keep default delivery`; do not send raw address or payment-sensitive values.",
		"`Delivery visible: yes`",
		"`Delivery visible: multiple options` keeps the purchase-readiness decision stopped.",
		"Before delivery option is confirmed, do not execute cart, payment, or final order clicks.",
		"Execution status:",
		"Opened URL: https://cart.coupang.com/cartView.pang",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English delivery follow-up guide missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English delivery follow-up should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"Coupang two-address verification step", "purchase completed", "order completed", "added to cart"} {
		if strings.Contains(strings.ToLower(reply), strings.ToLower(bad)) {
			t.Fatalf("English delivery follow-up should not misroute or claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantScenarioCoupangDeliveryTrackingStopsBeforeAccountChange(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "쿠팡 배송 조회해줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	for _, want := range []string{
		"쿠팡 배송 조회 준비 단계입니다.",
		"아직 주문 취소, 반품, 교환, 재배송 요청, 고객센터 전송 버튼은 누르지 않았습니다.",
		"주문목록에서 배송 상태만 확인하고 계정 상태를 바꾸는 작업은 멈춰야 합니다.",
		"쿠팡 주문목록을 열고 배송 조회 대상 주문을 찾습니다.",
		"상품명, 주문일, 수량, 배송 상태가 사용자가 찾는 주문과 맞는지 확인합니다.",
		"도착 예정일, 배송 단계, 택배사/운송장 표시 여부",
		"전체 운송장번호, 주문번호 원문은 Signal에 보내지 말고",
		"`배송 조회: 맞음`",
		"`배송 문제 보임`이면 보이는 문구만 요약하고 조치 버튼은 누르지 않음",
		"명시 승인 전에는 외부 계정 상태를 바꾸는 주문취소/반품/교환/재배송/고객센터 제출을 실행하지 않습니다.",
		"열린 주소: https://www.coupang.com/np/mycoupang/order/list",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("delivery-tracking guard missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"쿠팡 배송 옵션 확인 단계", "구매 플로우를 중지했습니다", "주문 취소했습니다", "재배송 요청을 제출했습니다"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("delivery-tracking guard should not misroute or claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantScenarioCoupangDeliveryTrackingReusesRecentPurchaseResult(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	targetID := "argos-assistant-delivery-recent-result"
	recent := strings.Join([]string{
		"최종 구매 클릭을 실행했습니다.",
		"주문 결과 화면에서 읽은 내용:",
		"- 주문 상태: 배송중",
		"- 주문번호: ********9012",
		"- 도착 예정: 내일 도착 예정",
		"- 배송/택배 표시: 쿠팡 로켓배송",
		"- 운송장/조회번호: ********1000",
	}, "\n")
	if err := appendSignalHistory(targetID, "휴지 구매해", recent); err != nil {
		t.Fatal(err)
	}

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "배송 조회해줘")
	if !handled {
		t.Fatal("recent delivery tracking follow-up was not handled")
	}
	for _, want := range []string{
		"최근 주문 결과를 다시 확인했습니다.",
		"출처: 직전 구매 완료/배송 조회 결과 카드",
		"주문목록 화면에서 읽은 내용:",
		"주문 상태: 배송중",
		"주문번호: ********9012",
		"도착 예정: 내일 도착 예정",
		"배송/택배 표시: 쿠팡 로켓배송",
		"운송장/조회번호: ********1000",
		"최신 상태를 주문목록에서 다시 읽으려면 `쿠팡 주문목록 새로 열어 배송 조회해줘`",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("recent purchase result follow-up missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"쿠팡 배송 조회 준비 단계입니다.", "열린 주소:", "123456789012", "987654321000"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("recent purchase result follow-up should not open browser or leak raw ids %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantScenarioCoupangDeliveryTrackingRecentPurchaseResultEnglish(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	targetID := "argos-assistant-delivery-recent-result-en"
	recent := strings.Join([]string{
		"Final purchase click executed.",
		"Read from the order-result screen:",
		"- Order status: in transit",
		"- Order number: ********7890",
		"- Arrival estimate: Arrives tomorrow",
		"- Delivery/carrier text: Coupang Rocket delivery",
		"- Tracking number: ********1000",
	}, "\n")
	if err := appendSignalHistory(targetID, "Order me toilet paper on Coupang", recent); err != nil {
		t.Fatal(err)
	}

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "track delivery")
	if !handled {
		t.Fatal("English recent delivery tracking follow-up was not handled")
	}
	for _, want := range []string{
		"Recalled the recent order result.",
		"Source: recent purchase-completion or delivery-tracking result card.",
		"Read from the order-list screen:",
		"Order status: in transit",
		"Order number: ********7890",
		"Arrival estimate: Arrives tomorrow",
		"Delivery/carrier text: Coupang Rocket delivery",
		"Tracking number: ********1000",
		"open the Coupang order list and track delivery",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English recent purchase result follow-up missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English recent purchase result follow-up leaked Korean:\n%s", reply)
	}
	for _, bad := range []string{"Coupang delivery-tracking screen", "Opened URL:", "CP1234567890", "987654321000"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("English recent purchase result follow-up should not open browser or leak raw ids %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantScenarioCoupangDeliveryTrackingExecuteReadsOrderListOCR(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_DASHBOARD_URL", "https://argos.example/argos/dashboard.html")
	oldCapture := shoppingCheckoutScreenCapture
	oldOCR := shoppingCheckoutScreenOCR
	defer func() {
		shoppingCheckoutScreenCapture = oldCapture
		shoppingCheckoutScreenOCR = oldOCR
	}()
	shoppingCheckoutScreenCapture = func(ctx context.Context, output string) osauto.Result {
		if err := os.WriteFile(output, []byte("fake coupang order-list proof"), 0600); err != nil {
			t.Fatal(err)
		}
		return osauto.Result{Kind: "meshclaw_automation_screen_capture", Action: "screen_capture", OK: true}
	}
	shoppingCheckoutScreenOCR = func(path string) (string, string) {
		return strings.Join([]string{
			"쿠팡 주문목록",
			"풀무원샘물 무라벨 생수 500ml 20개",
			"배송중",
			"오늘 도착 예정",
			"쿠팡 로켓배송",
			"운송장 987654321000",
			"주문번호: 123456789012",
		}, "\n"), ""
	}
	orderURL := coupangReturnRefundReviewURL()
	actionReply := strings.Join(formatCoupangDeliveryTrackingOpenURLActionFor("ko", orderURL), "\n")
	reply, ok := coupangDeliveryTrackingReadScreenReply("ko", ListenOptions{TargetID: "argos-assistant-delivery-read"}, "배송 어디야", orderURL, actionReply)
	if !ok {
		t.Fatal("delivery tracking screen read was not handled")
	}
	for _, want := range []string{
		"쿠팡 주문/배송 상태를 확인했습니다.",
		"쿠팡 배송 조회 화면 열기를 요청했습니다.",
		"열린 주소: https://www.coupang.com/np/mycoupang/order/list",
		"주문목록 화면 캡처:",
		"주문목록 화면에서 읽은 내용:",
		"주문 상태: 배송중",
		"주문번호: ********9012",
		"도착 예정: 오늘 도착 예정",
		"배송/택배 표시: 쿠팡 로켓배송",
		"운송장/조회번호: ********1000",
		"조회 증거:",
		"Evidence 탭: https://argos.example/argos/dashboard.html?evidence=",
		"#evidence",
		"다음 Signal 액션: `영수증 확인해줘`, `반품 준비해줘`, `주문 취소 준비해줘`",
		"배송조회 외에 주문취소/반품/교환/재배송/고객센터 제출 버튼은 누르지 않습니다.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("delivery tracking OCR reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"주문번호: 123456789012", "운송장/조회번호: 987654321000", home, "/.meshclaw/evidence/", ".json"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("delivery tracking reply should mask raw identifier %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantScenarioShortDeliveryTrackingExecuteOpensAndReadsOrderList(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	oldOpenURL := shoppingDeliveryTrackingOpenURL
	oldCapture := shoppingCheckoutScreenCapture
	oldOCR := shoppingCheckoutScreenOCR
	defer func() {
		shoppingDeliveryTrackingOpenURL = oldOpenURL
		shoppingCheckoutScreenCapture = oldCapture
		shoppingCheckoutScreenOCR = oldOCR
	}()
	opened := ""
	shoppingDeliveryTrackingOpenURL = func(ctx context.Context, rawURL string) osauto.Result {
		opened = rawURL
		return osauto.Result{Kind: "meshclaw_automation_open_url", Action: "open_url", OK: true, URL: rawURL}
	}
	shoppingCheckoutScreenCapture = func(ctx context.Context, output string) osauto.Result {
		if err := os.WriteFile(output, []byte("fake coupang order-list proof"), 0600); err != nil {
			t.Fatal(err)
		}
		return osauto.Result{Kind: "meshclaw_automation_screen_capture", Action: "screen_capture", OK: true}
	}
	shoppingCheckoutScreenOCR = func(path string) (string, string) {
		return strings.Join([]string{
			"쿠팡 주문목록",
			"휴지 30롤",
			"배송중",
			"내일 도착 예정",
			"쿠팡 로켓배송",
			"운송장 987654321000",
			"주문번호: 123456789012",
		}, "\n"), ""
	}

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant-delivery-short-execute", Mode: "assistant", Execute: true}, "배송 추적해")
	if !handled {
		t.Fatal("short delivery tracking request was not handled")
	}
	if opened != coupangReturnRefundReviewURL() {
		t.Fatalf("order list URL was not opened directly: %q", opened)
	}
	for _, want := range []string{
		"쿠팡 주문/배송 상태를 확인했습니다.",
		"열린 주소: https://www.coupang.com/np/mycoupang/order/list",
		"주문 상태: 배송중",
		"도착 예정: 내일 도착 예정",
		"배송/택배 표시: 쿠팡 로켓배송",
		"운송장/조회번호: ********1000",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("short delivery tracking execute reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"Mac 작업 실행 권한", "작업: open_url", "쿠팡 배송 조회 준비 단계입니다."} {
		if strings.Contains(reply, bad) {
			t.Fatalf("short delivery tracking execute should not fall back to prep/permission card %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantScenarioCoupangOrderConfirmationExecutesOrderListRead(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	oldOpenURL := shoppingDeliveryTrackingOpenURL
	oldCapture := shoppingCheckoutScreenCapture
	oldOCR := shoppingCheckoutScreenOCR
	defer func() {
		shoppingDeliveryTrackingOpenURL = oldOpenURL
		shoppingCheckoutScreenCapture = oldCapture
		shoppingCheckoutScreenOCR = oldOCR
	}()
	opened := ""
	shoppingDeliveryTrackingOpenURL = func(ctx context.Context, rawURL string) osauto.Result {
		opened = rawURL
		return osauto.Result{Kind: "meshclaw_automation_open_url", Action: "open_url", OK: true, URL: rawURL}
	}
	shoppingCheckoutScreenCapture = func(ctx context.Context, output string) osauto.Result {
		if err := os.WriteFile(output, []byte("fake coupang order-list proof"), 0600); err != nil {
			t.Fatal(err)
		}
		return osauto.Result{Kind: "meshclaw_automation_screen_capture", Action: "screen_capture", OK: true}
	}
	shoppingCheckoutScreenOCR = func(path string) (string, string) {
		return strings.Join([]string{
			"쿠팡 주문목록",
			"휴지 30롤",
			"주문 접수",
			"내일 도착 예정",
			"쿠팡 로켓배송",
			"주문번호: 123456789012",
		}, "\n"), ""
	}

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant-order-confirm-execute", Mode: "assistant", Execute: true}, "쿠팡 주문 확인해")
	if !handled {
		t.Fatal("Coupang order confirmation request was not handled")
	}
	if opened != coupangReturnRefundReviewURL() {
		t.Fatalf("order list URL was not opened directly: %q", opened)
	}
	for _, want := range []string{
		"쿠팡 주문/배송 상태를 확인했습니다.",
		"열린 주소: https://www.coupang.com/np/mycoupang/order/list",
		"주문 상태: 주문 접수",
		"주문번호: ********9012",
		"도착 예정: 내일 도착 예정",
		"배송/택배 표시: 쿠팡 로켓배송",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("order confirmation execute reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"Mac 작업 실행 권한", "작업: open_url", "쿠팡 후보 비교", "검색어:"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("order confirmation should not fall back to permission/search %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantScenarioShortOrderConfirmationWithRecentCoupangContextExecutesRead(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	targetID := "argos-assistant-order-confirm-short-context"
	if err := appendSignalHistory(targetID, "휴지 구매해", "쿠팡 구매 완료\n주문 완료\n구매: 휴지"); err != nil {
		t.Fatal(err)
	}
	oldOpenURL := shoppingDeliveryTrackingOpenURL
	oldCapture := shoppingCheckoutScreenCapture
	oldOCR := shoppingCheckoutScreenOCR
	defer func() {
		shoppingDeliveryTrackingOpenURL = oldOpenURL
		shoppingCheckoutScreenCapture = oldCapture
		shoppingCheckoutScreenOCR = oldOCR
	}()
	opened := ""
	shoppingDeliveryTrackingOpenURL = func(ctx context.Context, rawURL string) osauto.Result {
		opened = rawURL
		return osauto.Result{Kind: "meshclaw_automation_open_url", Action: "open_url", OK: true, URL: rawURL}
	}
	shoppingCheckoutScreenCapture = func(ctx context.Context, output string) osauto.Result {
		if err := os.WriteFile(output, []byte("fake coupang order-list proof"), 0600); err != nil {
			t.Fatal(err)
		}
		return osauto.Result{Kind: "meshclaw_automation_screen_capture", Action: "screen_capture", OK: true}
	}
	shoppingCheckoutScreenOCR = func(path string) (string, string) {
		return strings.Join([]string{
			"쿠팡 주문목록",
			"휴지 30롤",
			"배송중",
			"내일 도착 예정",
			"쿠팡 로켓배송",
			"주문번호: 123456789012",
		}, "\n"), ""
	}

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: targetID, Mode: "assistant", Execute: true}, "주문 확인해")
	if !handled {
		t.Fatal("short order confirmation follow-up was not handled")
	}
	if opened != coupangReturnRefundReviewURL() {
		t.Fatalf("order list URL was not opened directly: %q", opened)
	}
	for _, want := range []string{
		"쿠팡 주문/배송 상태를 확인했습니다.",
		"주문 상태: 배송중",
		"도착 예정: 내일 도착 예정",
		"배송/택배 표시: 쿠팡 로켓배송",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("short order confirmation execute reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"Mac 작업 실행 권한", "쿠팡 후보 비교", "검색어:"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("short order confirmation should not fall back to permission/search %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantScenarioCoupangDeliveryTrackingEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "track this Coupang order delivery")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	for _, want := range []string{
		"Coupang delivery-tracking prep step.",
		"No order cancel, return, exchange, reshipment request, or customer-service submit button has been clicked yet.",
		"check only delivery status in the order list and stop before any account-changing action.",
		"open the Coupang order list and find the order for delivery tracking.",
		"product name, order date, quantity, and delivery status match the order",
		"arrival estimate, delivery stage, carrier/tracking-number visibility",
		"Do not send raw detailed address, phone number, full tracking number, or raw order number",
		"`Delivery tracking: yes`",
		"`Delivery issue visible` means summarize only visible text and do not click action buttons",
		"Before explicit approval, do not submit any order-cancel, return, exchange, reshipment, or customer-service action",
		"Opened URL: https://www.coupang.com/np/mycoupang/order/list",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English delivery-tracking guard missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English delivery-tracking guard should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"delivery-option verification step", "purchase flow has been stopped", "order canceled", "reshipment requested"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English delivery-tracking guard should not misroute or claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantScenarioCoupangReceiptInvoiceStopsBeforeSensitiveAction(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "쿠팡 영수증 받아줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	for _, want := range []string{
		"쿠팡 영수증/증빙 확인 준비 단계입니다.",
		"아직 영수증 발급/다운로드/공유, 현금영수증/세금계산서 신청, 고객센터 전송 버튼은 누르지 않았습니다.",
		"증빙 가능 여부와 유형만 확인하고 민감정보 처리나 발급 실행은 멈춰야 합니다.",
		"쿠팡 주문목록을 열고 영수증/증빙 대상 주문을 찾습니다.",
		"상품명, 주문일, 결제 금액, 배송 상태가 사용자가 찾는 주문과 맞는지 확인합니다.",
		"영수증, 거래명세서, 카드매출전표, 현금영수증, 세금계산서",
		"사업자번호, 카드번호, 승인번호, 주문번호 원문은 Signal에 보내지 말고",
		"`영수증 확인: 맞음`",
		"`증빙 유형 여러 개`이면 가능한 유형만 요약하고 버튼은 누르지 않음",
		"명시 승인 전에는 영수증 발급/다운로드/공유, 현금영수증/세금계산서 신청, 고객센터 제출을 실행하지 않습니다.",
		"열린 주소: https://www.coupang.com/np/mycoupang/order/list",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("receipt/invoice guard missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"쿠팡 배송 조회 준비 단계", "쿠팡 배송 옵션 확인 단계", "구매 플로우를 중지했습니다", "영수증을 다운로드했습니다", "세금계산서를 신청했습니다"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("receipt/invoice guard should not misroute or claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantScenarioCoupangReceiptInvoiceExecuteReadsOrderListOCR(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_DASHBOARD_URL", "https://argos.example/argos/dashboard.html")
	oldOpenURL := shoppingReceiptInvoiceOpenURL
	oldCapture := shoppingCheckoutScreenCapture
	oldOCR := shoppingCheckoutScreenOCR
	defer func() {
		shoppingReceiptInvoiceOpenURL = oldOpenURL
		shoppingCheckoutScreenCapture = oldCapture
		shoppingCheckoutScreenOCR = oldOCR
	}()
	opened := ""
	shoppingReceiptInvoiceOpenURL = func(ctx context.Context, rawURL string) osauto.Result {
		opened = rawURL
		return osauto.Result{Kind: "meshclaw_automation_open_url", Action: "open_url", OK: true, URL: rawURL}
	}
	shoppingCheckoutScreenCapture = func(ctx context.Context, output string) osauto.Result {
		if err := os.WriteFile(output, []byte("fake coupang receipt proof"), 0600); err != nil {
			t.Fatal(err)
		}
		return osauto.Result{Kind: "meshclaw_automation_screen_capture", Action: "screen_capture", OK: true}
	}
	shoppingCheckoutScreenOCR = func(path string) (string, string) {
		return strings.Join([]string{
			"쿠팡 주문목록",
			"휴지 30롤",
			"배송완료",
			"주문일 2026.06.15",
			"총 결제 금액 12,900원",
			"영수증",
			"카드매출전표",
			"현금영수증",
			"세금계산서",
			"주문번호: 123456789012",
		}, "\n"), ""
	}

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant-receipt-execute", Mode: "assistant", Execute: true}, "쿠팡 영수증 보여줘")
	if !handled {
		t.Fatal("receipt/invoice execute request was not handled")
	}
	if opened != coupangReturnRefundReviewURL() {
		t.Fatalf("order list URL was not opened directly: %q", opened)
	}
	for _, want := range []string{
		"쿠팡 영수증/증빙 가능 여부를 확인했습니다.",
		"열린 주소: https://www.coupang.com/np/mycoupang/order/list",
		"주문 상태: 배송완료",
		"주문번호: ********9012",
		"표시 날짜: 주문일 2026.06.15",
		"표시 금액: 12,900원",
		"가능한 증빙 유형:",
		"영수증",
		"카드매출전표",
		"현금영수증",
		"세금계산서",
		"확인 증거:",
		"Evidence 탭: https://argos.example/argos/dashboard.html?evidence=",
		"#evidence",
		"다음 Signal 액션: `배송 조회해줘`, `반품 준비해줘`, `주문 취소 준비해줘`",
		"발급/다운로드/공유/현금영수증/세금계산서 신청 버튼은 누르지 않습니다.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("receipt/invoice execute reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"Mac 작업 실행 권한", "작업: open_url", "쿠팡 영수증/증빙 확인 준비 단계입니다.", "영수증을 다운로드했습니다", "세금계산서를 신청했습니다", "주문번호: 123456789012", home, "/.meshclaw/evidence/", ".json"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("receipt/invoice execute should not fall back or claim sensitive action %q:\n%s", bad, reply)
		}
	}
}

func TestShoppingCheckoutAutoDecisionEvidenceUsesDashboardLink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_DASHBOARD_URL", "https://argos.example/argos/dashboard.html")
	oldCapture := shoppingCheckoutScreenCapture
	oldOCR := shoppingCheckoutScreenOCR
	defer func() {
		shoppingCheckoutScreenCapture = oldCapture
		shoppingCheckoutScreenOCR = oldOCR
	}()
	shoppingCheckoutScreenCapture = func(ctx context.Context, output string) osauto.Result {
		if err := os.WriteFile(output, []byte("fake checkout proof"), 0600); err != nil {
			t.Fatal(err)
		}
		return osauto.Result{Kind: "meshclaw_automation_screen_capture", Action: "screen_capture", OK: true}
	}
	shoppingCheckoutScreenOCR = func(path string) (string, string) {
		return strings.Join([]string{
			"상품명: 풀무원샘물 무라벨 생수",
			"옵션/수량: 500ml 20개 / 1개",
			"총액: 10,990원",
			"도착 예정일: 내일 도착",
			"배송지 표시: 맞음",
			"결제수단 표시: 맞음",
			"주문 버튼: 보임 x=456, y=789",
		}, "\n"), ""
	}

	reply, attachments, ok := shoppingCheckoutAutoDecisionReplyFor("ko", ListenOptions{TargetID: "argos-assistant-checkout-auto-evidence"}, "최종 화면 자동 판독", browserauto.Link{Text: "풀무원샘물 무라벨 생수", URL: "https://cart.coupang.com/cartView.pang"})
	if !ok {
		t.Fatalf("checkout auto decision was not handled, attachments=%#v reply=\n%s", attachments, reply)
	}
	for _, want := range []string{
		"체크아웃 화면을 자동으로 읽었습니다.",
		"OCR 결과를 구매 가능 판정에 반영했습니다.",
		"자동 준비 증거:",
		"Evidence 탭: https://argos.example/argos/dashboard.html?evidence=",
		"#evidence",
		"구매 가능 판정",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("checkout auto reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{home, "/.meshclaw/evidence/", ".json"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("checkout auto reply should not expose raw evidence path %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantScenarioShortReceiptInvoiceWithRecentCoupangContextExecutesReadOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	targetID := "argos-assistant-short-receipt-context"
	if err := appendSignalHistory(targetID, "휴지 구매해", "쿠팡 구매 완료\n주문 완료\n구매: 휴지"); err != nil {
		t.Fatal(err)
	}
	oldOpenURL := shoppingReceiptInvoiceOpenURL
	oldCapture := shoppingCheckoutScreenCapture
	oldOCR := shoppingCheckoutScreenOCR
	defer func() {
		shoppingReceiptInvoiceOpenURL = oldOpenURL
		shoppingCheckoutScreenCapture = oldCapture
		shoppingCheckoutScreenOCR = oldOCR
	}()
	opened := ""
	shoppingReceiptInvoiceOpenURL = func(ctx context.Context, rawURL string) osauto.Result {
		opened = rawURL
		return osauto.Result{Kind: "meshclaw_automation_open_url", Action: "open_url", OK: true, URL: rawURL}
	}
	shoppingCheckoutScreenCapture = func(ctx context.Context, output string) osauto.Result {
		if err := os.WriteFile(output, []byte("fake coupang receipt proof"), 0600); err != nil {
			t.Fatal(err)
		}
		return osauto.Result{Kind: "meshclaw_automation_screen_capture", Action: "screen_capture", OK: true}
	}
	shoppingCheckoutScreenOCR = func(path string) (string, string) {
		return strings.Join([]string{
			"쿠팡 주문목록",
			"휴지 30롤",
			"배송완료",
			"총 결제 금액 12,900원",
			"영수증",
			"주문번호: 123456789012",
		}, "\n"), ""
	}

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: targetID, Mode: "assistant", Execute: true}, "영수증 보여줘")
	if !handled {
		t.Fatal("short receipt/invoice follow-up was not handled")
	}
	if opened != coupangReturnRefundReviewURL() {
		t.Fatalf("order list URL was not opened directly: %q", opened)
	}
	for _, want := range []string{
		"쿠팡 영수증/증빙 가능 여부를 확인했습니다.",
		"주문 상태: 배송완료",
		"표시 금액: 12,900원",
		"가능한 증빙 유형: 영수증",
		"다음 Signal 액션: `배송 조회해줘`, `반품 준비해줘`, `주문 취소 준비해줘`",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("short receipt/invoice execute reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"Mac 작업 실행 권한", "쿠팡 영수증/증빙 확인 준비 단계입니다.", "다운로드했습니다", "주문번호: 123456789012"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("short receipt/invoice execute should not fall back or claim sensitive action %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantScenarioShortReturnPrepWithRecentCoupangContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-short-return-context"
	if err := appendSignalHistory(targetID, "휴지 구매해", "쿠팡 구매 완료\n주문 완료\n구매: 휴지"); err != nil {
		t.Fatal(err)
	}

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "반품 준비해줘")
	if !handled {
		t.Fatal("short return prep follow-up was not handled")
	}
	for _, want := range []string{
		"쿠팡 반품/환불/교환 준비 단계입니다.",
		"아직 반품 신청, 환불 요청, 교환 요청, 주문 취소 제출 버튼은 누르지 않았습니다.",
		"쿠팡 주문목록을 열고, 반품/환불/교환 대상 주문을 찾습니다.",
		"`반품/환불/교환 준비: 맞음`",
		"이어 할 수 있는 말: `배송 조회해줘`, `영수증 확인해줘`, `취소 준비해줘`",
		"명시 승인 전에는 외부 계정 상태를 바꾸는 반품/환불/교환/주문취소 제출을 실행하지 않습니다.",
		"열린 주소: https://www.coupang.com/np/mycoupang/order/list",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("short return prep follow-up missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"반품 신청을 제출했습니다", "환불 요청을 제출했습니다", "주문 취소했습니다", "가격 알림"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("short return prep follow-up should not misroute or claim submission %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantScenarioShortOrderCancelPrepWithRecentCoupangContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-short-order-cancel-context"
	if err := appendSignalHistory(targetID, "휴지 구매해", "쿠팡 구매 완료\n주문 완료\n구매: 휴지"); err != nil {
		t.Fatal(err)
	}

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "취소 준비해줘")
	if !handled {
		t.Fatal("short order-cancel prep follow-up was not handled")
	}
	for _, want := range []string{
		"쿠팡 주문 취소 준비 단계입니다.",
		"아직 주문 취소 신청/제출, 고객센터 전송, 환불/반품/교환 버튼은 누르지 않았습니다.",
		"쿠팡 주문목록을 열고 취소 대상 주문을 찾습니다.",
		"`주문 취소 준비: 맞음`",
		"이어 할 수 있는 말: `배송 조회해줘`, `영수증 확인해줘`, `반품 준비해줘`",
		"명시 승인 전에는 외부 계정 상태를 바꾸는 주문 취소 신청/제출을 실행하지 않습니다.",
		"열린 주소: https://www.coupang.com/np/mycoupang/order/list",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("short order-cancel prep follow-up missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 플로우를 중지했습니다", "쿠팡 구매 중지", "주문 취소했습니다", "반품/환불/교환 준비 단계"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("short order-cancel prep follow-up should not misroute or claim submission %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantScenarioCoupangReceiptInvoiceEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "get the Coupang receipt or tax invoice")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	for _, want := range []string{
		"Coupang receipt/proof-of-purchase prep step.",
		"No receipt issue, download, share, cash-receipt/tax-invoice request, or customer-service submit button has been clicked yet.",
		"check only whether proof documents are available in the order list and stop before handling sensitive data or issuing documents.",
		"open the Coupang order list and find the order for receipt/proof-of-purchase review.",
		"product name, order date, payment amount, and delivery status match the order",
		"receipt, transaction statement, card sales slip, cash receipt, or tax invoice",
		"business-registration number, card number, approval number, or raw order number",
		"`Receipt check: yes`",
		"`Multiple proof types visible` means summarize only available types and do not click buttons",
		"Before explicit approval, do not issue, download, or share receipts, request cash receipts/tax invoices, or submit customer-service actions.",
		"Opened URL: https://www.coupang.com/np/mycoupang/order/list",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English receipt/invoice guard missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English receipt/invoice guard should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"delivery-tracking prep step", "delivery-option verification step", "purchase flow has been stopped", "receipt downloaded", "tax invoice requested"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English receipt/invoice guard should not misroute or claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantScenarioCoupangCustomerServiceContactStopsBeforeSend(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "쿠팡 고객센터에 문의해줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	for _, want := range []string{
		"쿠팡 고객센터/판매자 문의 준비 단계입니다.",
		"아직 고객센터 전송, 판매자 문의 전송, 채팅 시작, 주문 변경 버튼은 누르지 않았습니다.",
		"주문과 문의 채널만 확인하고 실제 메시지 전송 전에서 멈춰야 합니다.",
		"쿠팡 주문목록을 열고 문의 대상 주문을 찾습니다.",
		"상품명, 주문일, 수량, 배송 상태가 문의하려는 주문과 맞는지 확인합니다.",
		"고객센터, 판매자 문의, 배송 문의, 교환/반품 문의",
		"문의 내용은 Signal에 초안으로만 정리하고, 화면 입력칸에 붙여넣거나 전송하지 않습니다.",
		"`문의 준비: 맞음`",
		"명시 승인 전에는 외부 계정 상태를 바꾸거나 외부로 전송되는 고객센터/판매자 문의/채팅/주문 변경을 실행하지 않습니다.",
		"열린 주소: https://www.coupang.com/np/mycoupang/order/list",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("customer-service contact guard missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"쿠팡 배송 조회 준비 단계", "쿠팡 반품/환불/교환 준비 단계", "문의를 전송했습니다", "채팅을 시작했습니다"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("customer-service contact guard should not misroute or claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantScenarioCoupangCustomerServiceContactEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "contact Coupang customer service about this order")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	for _, want := range []string{
		"Coupang customer-service/seller-contact prep step.",
		"No customer-service send, seller-contact send, chat start, or order-change button has been clicked yet.",
		"verify only the order and contact channel, then stop before any actual message send.",
		"open the Coupang order list and find the order for customer-service or seller contact.",
		"product name, order date, quantity, and delivery status match the order",
		"customer service, seller contact, delivery inquiry, or return/exchange inquiry",
		"Prepare the message only as a Signal draft",
		"`Contact prep: yes`",
		"Before explicit approval, do not send customer-service or seller-contact messages",
		"Opened URL: https://www.coupang.com/np/mycoupang/order/list",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English customer-service contact guard missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English customer-service contact guard should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"delivery-tracking prep step", "return/refund/exchange prep step", "message sent", "chat started"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English customer-service contact guard should not misroute or claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantScenarioCoupangProductReviewPrepStopsBeforePost(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "쿠팡 상품평 작성해줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	for _, want := range []string{
		"쿠팡 상품평/리뷰 작성 준비 단계입니다.",
		"아직 별점 저장, 상품평 등록, 사진/영상 업로드 확정, 리뷰 게시 버튼은 누르지 않았습니다.",
		"주문과 작성 초안만 확인하고 실제 게시/등록 전에서 멈춰야 합니다.",
		"쿠팡 주문목록을 열고 상품평을 작성할 주문을 찾습니다.",
		"상품명, 주문일, 옵션, 수량이 상품평을 남기려는 주문과 맞는지 확인합니다.",
		"별점 후보, 장점/단점, 사용 기간, 재구매 의사",
		"사진/영상 첨부가 필요하면 필요한 종류만 메모",
		"`상품평 준비: 맞음`",
		"`게시 직전 화면`이 보이면 멈추고 사용자 승인 요청",
		"명시 승인 전에는 외부에 공개되는 상품평/리뷰 등록, 별점 저장, 사진/영상 업로드 확정을 실행하지 않습니다.",
		"열린 주소: https://www.coupang.com/np/mycoupang/order/list",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("product-review guard missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"상위 후보 비교표", "리뷰/가격/배송 조건을 비교", "상품평을 등록했습니다", "리뷰를 게시했습니다"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("product-review guard should not misroute or claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantScenarioCoupangProductReviewPrepEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "write a Coupang review for this order")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	for _, want := range []string{
		"Coupang product-review prep step.",
		"No rating save, product-review submit, photo/video upload confirmation, or review post button has been clicked yet.",
		"verify only the order and review draft, then stop before any actual post or submission.",
		"open the Coupang order list and find the order to review.",
		"product name, order date, option, and quantity match the order",
		"rating candidate, pros/cons, usage period, and repurchase intent",
		"note only the attachment type; do not upload files",
		"`Product review prep: yes`",
		"If the pre-post screen is visible, stop and ask the user for approval",
		"Before explicit approval, do not publish product reviews",
		"Opened URL: https://www.coupang.com/np/mycoupang/order/list",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English product-review guard missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English product-review guard should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"candidate comparison", "review posted", "review submitted", "product review posted"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English product-review guard should not misroute or claim %q:\n%s", bad, reply)
		}
	}
}

func TestCoupangProductReviewOpenURLActionUsesReviewSpecificCard(t *testing.T) {
	rawURL := coupangReturnRefundReviewURL()
	koAction := strings.Join(formatSignalCoupangOpenURLActionFor("ko", rawURL), "\n")
	koReply := localizeCoupangProductReviewOpenURLActionReply("ko", koAction, rawURL)
	for _, want := range []string{
		"쿠팡 상품평/리뷰 작성 준비 화면 열기를 요청했습니다.",
		"열린 주소: https://www.coupang.com/np/mycoupang/order/list",
		"열린 화면에서 확인할 것:",
		"별점 후보, 장점/단점, 사용 기간, 재구매 의사",
		"상품평 작성 가능 여부 확인 외에는 별점 저장, 상품평 등록",
		"명시 승인 전에는 외부에 공개되는 상품평/리뷰 등록",
	} {
		if !strings.Contains(koReply, want) {
			t.Fatalf("Korean product-review-specific open card missing %q:\n%s", want, koReply)
		}
	}
	for _, bad := range []string{"상위 3개 후보 비교표", "반품/환불/교환 준비 화면"} {
		if strings.Contains(koReply, bad) {
			t.Fatalf("Korean product-review-specific open card should not show generic copy %q:\n%s", bad, koReply)
		}
	}

	enAction := strings.Join(formatSignalCoupangOpenURLActionFor("en", rawURL), "\n")
	enReply := localizeCoupangProductReviewOpenURLActionReply("en", enAction, rawURL)
	for _, want := range []string{
		"Requested opening the Coupang product-review prep screen.",
		"Opened URL: https://www.coupang.com/np/mycoupang/order/list",
		"Check on the opened screen:",
		"rating candidate, pros/cons, usage period, and repurchase intent",
		"Other than checking review availability, do not click rating save",
		"Before explicit approval, do not publish product reviews",
	} {
		if !strings.Contains(enReply, want) {
			t.Fatalf("English product-review-specific open card missing %q:\n%s", want, enReply)
		}
	}
	if assistantCheckoutPrepContainsHangul(enReply) {
		t.Fatalf("English product-review-specific open card should not expose Korean:\n%s", enReply)
	}
	for _, bad := range []string{"Top 3 candidate comparison", "return/refund/exchange prep screen"} {
		if strings.Contains(enReply, bad) {
			t.Fatalf("English product-review-specific open card should not show generic copy %q:\n%s", bad, enReply)
		}
	}
}

func TestAssistantScenarioCoupangReorderPrepStopsBeforeCartOrPayment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "쿠팡에서 전에 산 생수 다시 주문해줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	for _, want := range []string{
		"쿠팡 재구매/다시 주문 준비 단계입니다.",
		"아직 장바구니 추가, 바로구매, 결제, 최종 주문 버튼은 누르지 않았습니다.",
		"이전 주문과 현재 구매 조건만 확인하고 장바구니/결제 전에서 멈춰야 합니다.",
		"쿠팡 주문목록을 열고 재구매하려는 이전 주문을 찾습니다.",
		"상품명, 주문일, 옵션, 수량이 재구매하려는 이전 주문과 맞는지 확인합니다.",
		"현재 판매 여부, 현재 가격, 배송 예정일, 로켓배송/판매자 변경, 품절/단종 여부",
		"옵션/수량/정기배송/추가 구성품이 이전 주문과 달라졌는지 확인",
		"`재구매 준비: 맞음`",
		"`장바구니/바로구매 화면`이 보이면 멈추고 사용자 승인 요청",
		"명시 승인 전에는 장바구니 추가, 바로구매, 결제, 최종 주문, 정기배송 신청",
		"열린 주소: https://www.coupang.com/np/mycoupang/order/list",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reorder guard missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"장바구니에 추가했습니다", "구매 완료", "최종 주문을 실행했습니다", "상위 후보 비교표"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("reorder guard should not misroute or claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantScenarioCoupangReorderExecuteClicksBuyAgainAndFinalOrder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	oldOpenURL := shoppingReorderOpenURL
	oldCapture := shoppingCheckoutScreenCapture
	oldOCR := shoppingCheckoutScreenOCR
	defer func() {
		shoppingReorderOpenURL = oldOpenURL
		shoppingCheckoutScreenCapture = oldCapture
		shoppingCheckoutScreenOCR = oldOCR
	}()
	opened := ""
	shoppingReorderOpenURL = func(ctx context.Context, rawURL string) osauto.Result {
		opened = rawURL
		return osauto.Result{Kind: "meshclaw_automation_open_url", Action: "open_url", OK: true, URL: rawURL}
	}
	ocrCalls := 0
	shoppingCheckoutScreenCapture = func(ctx context.Context, output string) osauto.Result {
		if err := os.WriteFile(output, []byte("fake coupang reorder proof"), 0600); err != nil {
			t.Fatal(err)
		}
		return osauto.Result{Kind: "meshclaw_automation_screen_capture", Action: "screen_capture", OK: true}
	}
	shoppingCheckoutScreenOCR = func(path string) (string, string) {
		ocrCalls++
		switch ocrCalls {
		case 1, 2:
			return strings.Join([]string{
				"쿠팡 주문목록",
				"휴지 30롤",
				"이전 주문",
				"재구매",
			}, "\n"), ""
		case 3:
			return strings.Join([]string{
				"상품명: 휴지",
				"옵션/수량: 30롤 1개",
				"총 결제 금액 12,900원",
				"내일 도착",
				"배송지",
				"결제수단 카드",
				"결제하기",
			}, "\n"), ""
		default:
			return strings.Join([]string{
				"주문 완료",
				"주문번호: 123456789012",
				"내일 도착 예정",
				"쿠팡 로켓배송",
			}, "\n"), ""
		}
	}
	clicks := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/click" {
			t.Fatalf("unexpected runner path: %s", r.URL.Path)
		}
		clicks++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	t.Setenv("MESHCLAW_ARGOS_UI_RUNNER_URL", server.URL)

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant-reorder-execute", Mode: "assistant", Execute: true}, "재구매해")
	if !handled {
		t.Fatal("reorder execute request was not handled")
	}
	if opened != coupangReturnRefundReviewURL() {
		t.Fatalf("order list URL was not opened directly: %q", opened)
	}
	if clicks < 2 {
		t.Fatalf("expected buy-again and final-order clicks, got %d:\n%s", clicks, reply)
	}
	for _, want := range []string{
		"최종 구매 클릭을 실행했습니다.",
		"상품: 휴지 / 30롤 1개",
		"총액: 12,900원",
		"도착 예정: 내일 도착",
		"주문 결과 화면에서 읽은 내용:",
		"주문번호: ********9012",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reorder execute reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"쿠팡 재구매/다시 주문 준비 단계입니다.", "Mac 작업 실행 권한", "작업: open_url", "검색어:"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("reorder execute should not fall back to prep/permission/search %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantScenarioCoupangReorderExecuteBlockedUsesEvidenceID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	oldOpenURL := shoppingReorderOpenURL
	oldCapture := shoppingCheckoutScreenCapture
	oldOCR := shoppingCheckoutScreenOCR
	defer func() {
		shoppingReorderOpenURL = oldOpenURL
		shoppingCheckoutScreenCapture = oldCapture
		shoppingCheckoutScreenOCR = oldOCR
	}()
	shoppingReorderOpenURL = func(ctx context.Context, rawURL string) osauto.Result {
		return osauto.Result{Kind: "meshclaw_automation_open_url", Action: "open_url", OK: true, URL: rawURL}
	}
	shoppingCheckoutScreenCapture = func(ctx context.Context, output string) osauto.Result {
		if err := os.WriteFile(output, []byte("fake coupang reorder auth proof"), 0600); err != nil {
			t.Fatal(err)
		}
		return osauto.Result{Kind: "meshclaw_automation_screen_capture", Action: "screen_capture", OK: true}
	}
	shoppingCheckoutScreenOCR = func(path string) (string, string) {
		return strings.Join([]string{
			"쿠팡 결제 인증",
			"결제 비밀번호",
			"CVC 입력",
			"보안코드",
		}, "\n"), ""
	}

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant-reorder-execute-blocked", Mode: "assistant", Execute: true}, "재구매해")
	if !handled {
		t.Fatal("reorder execute request was not handled")
	}
	for _, want := range []string{
		"`재구매/다시 주문` 구매 자동 진행이 멈췄습니다.",
		"이유: 결제 인증",
		"재구매 자동 진행을 시작했습니다.",
		"진행 증거 ID:",
		"진행 증거 확인: `최근 작업 보여줘 ",
		"사용자 처리: 카드 CVC/결제 비밀번호 같은 결제 인증만 직접 입력한 뒤 `ㅇㅇ`을 보내세요.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("blocked reorder reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"/.meshclaw/evidence/", "/Users/", ".json"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("blocked reorder reply should not expose raw evidence path %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantScenarioShortReorderWithRecentCoupangContextExecutes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	targetID := "argos-assistant-reorder-short-context"
	if err := appendSignalHistory(targetID, "휴지 구매해", "쿠팡 구매 완료\n주문 완료\n구매: 휴지"); err != nil {
		t.Fatal(err)
	}
	oldOpenURL := shoppingReorderOpenURL
	oldCapture := shoppingCheckoutScreenCapture
	oldOCR := shoppingCheckoutScreenOCR
	defer func() {
		shoppingReorderOpenURL = oldOpenURL
		shoppingCheckoutScreenCapture = oldCapture
		shoppingCheckoutScreenOCR = oldOCR
	}()
	opened := ""
	shoppingReorderOpenURL = func(ctx context.Context, rawURL string) osauto.Result {
		opened = rawURL
		return osauto.Result{Kind: "meshclaw_automation_open_url", Action: "open_url", OK: true, URL: rawURL}
	}
	ocrCalls := 0
	shoppingCheckoutScreenCapture = func(ctx context.Context, output string) osauto.Result {
		if err := os.WriteFile(output, []byte("fake coupang reorder proof"), 0600); err != nil {
			t.Fatal(err)
		}
		return osauto.Result{Kind: "meshclaw_automation_screen_capture", Action: "screen_capture", OK: true}
	}
	shoppingCheckoutScreenOCR = func(path string) (string, string) {
		ocrCalls++
		if ocrCalls <= 2 {
			return "쿠팡 주문목록\n휴지 30롤\n재구매", ""
		}
		if ocrCalls == 3 {
			return "상품명: 휴지\n옵션/수량: 30롤 1개\n총 결제 금액 12,900원\n내일 도착\n배송지\n결제수단 카드\n결제하기", ""
		}
		return "주문 완료\n주문번호: 123456789012\n내일 도착 예정", ""
	}
	clicks := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clicks++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	t.Setenv("MESHCLAW_ARGOS_UI_RUNNER_URL", server.URL)

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: targetID, Mode: "assistant", Execute: true}, "또 사줘")
	if !handled {
		t.Fatal("short reorder follow-up was not handled")
	}
	if opened != coupangReturnRefundReviewURL() {
		t.Fatalf("order list URL was not opened directly: %q", opened)
	}
	if clicks < 2 {
		t.Fatalf("expected buy-again and final-order clicks, got %d:\n%s", clicks, reply)
	}
	for _, want := range []string{"최종 구매 클릭을 실행했습니다.", "상품: 휴지 / 30롤 1개", "총액: 12,900원", "주문번호: ********9012"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("short reorder execute reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"검색어: 또", "쿠팡 후보 비교", "Mac 작업 실행 권한"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("short reorder should not fall back to search/permission %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantScenarioCoupangReorderPrepEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "reorder the same Coupang item from my previous order")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	for _, want := range []string{
		"Coupang reorder/buy-again prep step.",
		"No add-to-cart, buy-now, payment, or final-order button has been clicked yet.",
		"verify only the previous order and current purchase terms, then stop before cart or payment.",
		"open the Coupang order list and find the previous order to buy again.",
		"product name, order date, option, and quantity match the previous order",
		"current availability, current price, arrival estimate, Rocket/seller change",
		"options, quantity, recurring delivery, or add-on bundles differ",
		"`Reorder prep: yes`",
		"If the cart or buy-now screen is visible, stop and ask the user for approval",
		"Before explicit approval, do not add to cart, buy now, pay, place the final order",
		"Opened URL: https://www.coupang.com/np/mycoupang/order/list",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English reorder guard missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English reorder guard should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"added to cart", "purchase completed", "final order placed", "candidate comparison"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English reorder guard should not misroute or claim %q:\n%s", bad, reply)
		}
	}
}

func TestCoupangReorderOpenURLActionUsesReorderSpecificCard(t *testing.T) {
	rawURL := coupangReturnRefundReviewURL()
	koAction := strings.Join(formatSignalCoupangOpenURLActionFor("ko", rawURL), "\n")
	koReply := localizeCoupangReorderOpenURLActionReply("ko", koAction, rawURL)
	for _, want := range []string{
		"쿠팡 재구매/다시 주문 준비 화면 열기를 요청했습니다.",
		"열린 주소: https://www.coupang.com/np/mycoupang/order/list",
		"열린 화면에서 확인할 것:",
		"현재 판매 여부, 현재 가격, 배송 예정일, 로켓배송/판매자 변경, 품절/단종 여부",
		"재구매 가능 여부 확인 외에는 장바구니 추가, 바로구매, 결제",
		"명시 승인 전에는 장바구니 추가, 바로구매, 결제, 최종 주문",
	} {
		if !strings.Contains(koReply, want) {
			t.Fatalf("Korean reorder-specific open card missing %q:\n%s", want, koReply)
		}
	}
	for _, bad := range []string{"상위 3개 후보 비교표", "상품평/리뷰 작성 준비 화면"} {
		if strings.Contains(koReply, bad) {
			t.Fatalf("Korean reorder-specific open card should not show generic copy %q:\n%s", bad, koReply)
		}
	}

	enAction := strings.Join(formatSignalCoupangOpenURLActionFor("en", rawURL), "\n")
	enReply := localizeCoupangReorderOpenURLActionReply("en", enAction, rawURL)
	for _, want := range []string{
		"Requested opening the Coupang reorder/buy-again prep screen.",
		"Opened URL: https://www.coupang.com/np/mycoupang/order/list",
		"Check on the opened screen:",
		"current availability, current price, arrival estimate, Rocket/seller change",
		"Other than checking reorder availability, do not click add-to-cart",
		"Before explicit approval, do not add to cart, buy now, pay, place the final order",
	} {
		if !strings.Contains(enReply, want) {
			t.Fatalf("English reorder-specific open card missing %q:\n%s", want, enReply)
		}
	}
	if assistantCheckoutPrepContainsHangul(enReply) {
		t.Fatalf("English reorder-specific open card should not expose Korean:\n%s", enReply)
	}
	for _, bad := range []string{"Top 3 candidate comparison", "product-review prep screen"} {
		if strings.Contains(enReply, bad) {
			t.Fatalf("English reorder-specific open card should not show generic copy %q:\n%s", bad, enReply)
		}
	}
}

func TestAssistantScenarioCoupangSubscriptionPrepStopsBeforeChange(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "쿠팡 정기배송 해지해줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	for _, want := range []string{
		"쿠팡 정기배송/구독 관리 준비 단계입니다.",
		"아직 정기배송 해지, 주기 변경, 수량 변경, 주소/결제 변경 저장, 다시 시작 버튼은 누르지 않았습니다.",
		"정기배송 상태와 변경 가능 여부만 확인하고 저장/해지 전에서 멈춰야 합니다.",
		"쿠팡 주문목록 또는 정기배송 관리 화면을 열고 관리 대상 상품을 찾습니다.",
		"다음 배송일, 배송 주기, 현재 가격, 배송비, 적용 쿠폰/와우 조건",
		"해지 가능, 주기/수량 변경 가능, 주소/결제 변경 가능 여부만 확인",
		"`정기배송 확인: 맞음`",
		"`저장/해지 직전 화면`이 보이면 멈추고 사용자 승인 요청",
		"명시 승인 전에는 정기배송 해지, 주기/수량/주소/결제 변경 저장, 다시 시작, 결제 또는 최종 주문을 실행하지 않습니다.",
		"열린 주소: https://www.coupang.com/np/mycoupang/order/list",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("subscription guard missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"주문 취소 준비 단계", "구매 완료", "해지를 실행했습니다", "변경 저장했습니다"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("subscription guard should not misroute or claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantScenarioCoupangSubscriptionPrepEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "cancel my Coupang recurring delivery subscription")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	for _, want := range []string{
		"Coupang recurring-delivery/subscription management prep step.",
		"No recurring-delivery cancel, cycle change, quantity change, address/payment change save, or restart button has been clicked yet.",
		"verify only subscription status and change availability, then stop before save or cancellation.",
		"open the Coupang order list or recurring-delivery management screen and find the item to manage.",
		"next delivery date, delivery cycle, current price, shipping fee",
		"cancel, cycle/quantity change, or address/payment change is available",
		"`Recurring delivery check: yes`",
		"If the pre-save or pre-cancel screen is visible, stop and ask the user for approval",
		"Before explicit approval, do not cancel recurring delivery, save cycle/quantity/address/payment changes, restart, pay, or place a final order.",
		"Opened URL: https://www.coupang.com/np/mycoupang/order/list",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English subscription guard missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English subscription guard should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"order-cancel prep step", "purchase completed", "subscription canceled", "changes saved"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English subscription guard should not misroute or claim %q:\n%s", bad, reply)
		}
	}
}

func TestCoupangSubscriptionOpenURLActionUsesSubscriptionSpecificCard(t *testing.T) {
	rawURL := coupangReturnRefundReviewURL()
	koAction := strings.Join(formatSignalCoupangOpenURLActionFor("ko", rawURL), "\n")
	koReply := localizeCoupangSubscriptionOpenURLActionReply("ko", koAction, rawURL)
	for _, want := range []string{
		"쿠팡 정기배송/구독 관리 준비 화면 열기를 요청했습니다.",
		"열린 주소: https://www.coupang.com/np/mycoupang/order/list",
		"열린 화면에서 확인할 것:",
		"다음 배송일, 배송 주기, 현재 가격, 배송비, 적용 쿠폰/와우 조건",
		"정기배송 상태 확인 외에는 해지, 주기 변경 저장",
		"명시 승인 전에는 정기배송 해지, 주기/수량/주소/결제 변경 저장",
	} {
		if !strings.Contains(koReply, want) {
			t.Fatalf("Korean subscription-specific open card missing %q:\n%s", want, koReply)
		}
	}
	for _, bad := range []string{"상위 3개 후보 비교표", "주문 취소 준비 화면", "재구매/다시 주문 준비 화면"} {
		if strings.Contains(koReply, bad) {
			t.Fatalf("Korean subscription-specific open card should not show generic copy %q:\n%s", bad, koReply)
		}
	}

	enAction := strings.Join(formatSignalCoupangOpenURLActionFor("en", rawURL), "\n")
	enReply := localizeCoupangSubscriptionOpenURLActionReply("en", enAction, rawURL)
	for _, want := range []string{
		"Requested opening the Coupang recurring-delivery/subscription management prep screen.",
		"Opened URL: https://www.coupang.com/np/mycoupang/order/list",
		"Check on the opened screen:",
		"next delivery date, delivery cycle, current price, shipping fee",
		"Other than checking recurring-delivery status, do not click cancel",
		"Before explicit approval, do not cancel recurring delivery, save cycle/quantity/address/payment changes",
	} {
		if !strings.Contains(enReply, want) {
			t.Fatalf("English subscription-specific open card missing %q:\n%s", want, enReply)
		}
	}
	if assistantCheckoutPrepContainsHangul(enReply) {
		t.Fatalf("English subscription-specific open card should not expose Korean:\n%s", enReply)
	}
	for _, bad := range []string{"Top 3 candidate comparison", "order-cancel prep screen", "reorder/buy-again prep screen"} {
		if strings.Contains(enReply, bad) {
			t.Fatalf("English subscription-specific open card should not show generic copy %q:\n%s", bad, enReply)
		}
	}
}

func TestCoupangDeliveryOpenURLActionUsesDeliverySpecificCard(t *testing.T) {
	rawURL := coupangDeliveryReviewURL()
	koAction := strings.Join(formatSignalCoupangOpenURLActionFor("ko", rawURL), "\n")
	koReply := localizeCoupangDeliveryOpenURLActionReply("ko", koAction, rawURL)
	for _, want := range []string{
		"쿠팡 배송 옵션 확인 화면 열기를 요청했습니다.",
		"열린 주소: https://cart.coupang.com/cartView.pang",
		"열린 화면에서 확인할 것:",
		"로켓배송, 무료배송, 새벽배송, 정기배송 같은 배송 방식 중 이번 주문 조건에 맞는 하나만 확인합니다.",
		"배송 옵션 후보를 구매값만으로 구분하는 방식:",
		"`선택 필요: A/B/기본배송 유지`처럼 보내고, 주소 원문이나 결제 민감값은 보내지 않습니다.",
		"배송 옵션 확인 외에는 장바구니/결제/최종 주문 버튼을 누르지 않습니다.",
	} {
		if !strings.Contains(koReply, want) {
			t.Fatalf("Korean delivery-specific open card missing %q:\n%s", want, koReply)
		}
	}
	for _, bad := range []string{"검색어와 후보 상품명", "상위 3개 후보 비교표"} {
		if strings.Contains(koReply, bad) {
			t.Fatalf("Korean delivery-specific open card should not show generic product-search copy %q:\n%s", bad, koReply)
		}
	}

	enAction := strings.Join(formatSignalCoupangOpenURLActionFor("en", rawURL), "\n")
	enReply := localizeCoupangDeliveryOpenURLActionReply("en", enAction, rawURL)
	for _, want := range []string{
		"Requested opening the Coupang delivery-option verification screen.",
		"Opened URL: https://cart.coupang.com/cartView.pang",
		"Check on the opened screen:",
		"Verify only one delivery method that matches this order, such as Rocket delivery, free shipping, dawn delivery, or recurring delivery.",
		"How to distinguish delivery-option candidates using purchase values only:",
		"`Choice needed: A/B/keep default delivery`; do not send raw address or payment-sensitive values.",
		"Do not click cart, payment, or final order while verifying delivery options.",
	} {
		if !strings.Contains(enReply, want) {
			t.Fatalf("English delivery-specific open card missing %q:\n%s", want, enReply)
		}
	}
	if assistantCheckoutPrepContainsHangul(enReply) {
		t.Fatalf("English delivery-specific open card should not expose Korean:\n%s", enReply)
	}
	for _, bad := range []string{"Search query and candidate product names", "top 3 candidates"} {
		if strings.Contains(enReply, bad) {
			t.Fatalf("English delivery-specific open card should not show generic product-search copy %q:\n%s", bad, enReply)
		}
	}
}

func TestAssistantReplyCoupangDiscountChoiceFollowUpUsesRecentContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-coupang-discount-followup"
	if err := appendSignalHistory(targetID, "쿠팡 최종 화면 확인해", "쿠팡 최종 구매 화면 확인 단계입니다.\n총액: 쿠폰/와우할인 선택지가 여러 개 보임\n아직 구매 버튼은 누르지 않았습니다."); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "쿠폰이 여러 개야. 확인해")
	for _, want := range []string{
		"쿠팡 쿠폰/할인 확인 단계입니다.",
		"아직 결제/최종 주문 버튼은 누르지 않았습니다.",
		"이번 주문에 적용할 쿠폰/할인 또는 자동 적용 상태",
		"쿠팡 장바구니 또는 결제 직전 화면의 쿠폰/할인 영역",
		"자동 적용 쿠폰, 와우할인, 즉시할인",
		"할인 전/후 총액, 배송비, 정기배송/와우 조건",
		"쿠폰/할인 후보를 코드 원문 없이 구분하는 방식:",
		"`쿠폰 후보 A: 할인 유형·예상 할인액·적용 후 총액·조건만`",
		"`쿠폰 후보 B: 할인 유형·예상 할인액·적용 후 총액·조건만`",
		"`선택 필요: A/B/자동적용`처럼 보내고, 쿠폰 코드 원문이나 결제 민감값은 보내지 않습니다.",
		"`쿠폰 표시: 맞음`",
		"`쿠폰 표시: 여러 개 보임`이면 구매 가능 판정은 계속 중지됩니다.",
		"쿠폰/할인이 확정되기 전에는 장바구니, 결제, 최종 주문 클릭을 실행하지 않습니다.",
		"실행 상태:",
		"열린 주소: https://cart.coupang.com/cartView.pang",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("discount follow-up guide missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"브라우저에서 coupon discount code 검색", "google.com/search", "구매 완료", "주문 완료", "장바구니에 담았습니다", "최종 주문 버튼을 눌렀습니다"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("discount follow-up should not search or claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyCoupangDiscountChoiceEnglishFollowUpUsesRecentContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-coupang-discount-followup-en"
	if err := appendSignalHistory(targetID, "read the Coupang final screen", "Coupang final purchase-screen review step.\nTotal: multiple coupon or Wow discount choices visible\nThe purchase button has not been clicked yet."); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "multiple coupons, check it")
	for _, want := range []string{
		"Coupang coupon/discount verification step.",
		"No payment or final order button has been clicked yet.",
		"verify the coupon, discount, or automatically applied discount",
		"coupon/discount area on the Coupang cart or pre-payment screen",
		"auto coupons, Wow discounts, and instant discounts",
		"before/after-discount total, shipping fee, recurring-delivery, and Wow conditions",
		"How to distinguish coupon/discount candidates without raw codes:",
		"`Coupon candidate A: discount type, estimated discount, post-discount total, conditions only`",
		"`Coupon candidate B: discount type, estimated discount, post-discount total, conditions only`",
		"`Choice needed: A/B/auto-applied`; do not send raw coupon codes or payment-sensitive values.",
		"`Coupon visible: yes`",
		"`Coupon visible: multiple coupons` keeps the purchase-readiness decision stopped.",
		"Before coupon/discount is confirmed, do not execute cart, payment, or final order clicks.",
		"Execution status:",
		"Opened URL: https://cart.coupang.com/cartView.pang",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English discount follow-up guide missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English discount follow-up should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"coupon discount code", "google.com/search", "purchase completed", "order completed", "added to cart"} {
		if strings.Contains(strings.ToLower(reply), strings.ToLower(bad)) {
			t.Fatalf("English discount follow-up should not search or claim %q:\n%s", bad, reply)
		}
	}
}

func TestCoupangDiscountOpenURLActionUsesDiscountSpecificCard(t *testing.T) {
	rawURL := coupangDiscountReviewURL()
	koAction := strings.Join(formatSignalCoupangOpenURLActionFor("ko", rawURL), "\n")
	koReply := localizeCoupangDiscountOpenURLActionReply("ko", koAction, rawURL)
	for _, want := range []string{
		"쿠팡 쿠폰/할인 확인 화면 열기를 요청했습니다.",
		"열린 주소: https://cart.coupang.com/cartView.pang",
		"열린 화면에서 확인할 것:",
		"자동 적용 쿠폰, 와우할인, 즉시할인 중 이번 주문 총액이 가장 낮아지는 적용 상태만 확인합니다.",
		"`쿠폰 후보 A: 할인 유형·예상 할인액·적용 후 총액·조건만`",
		"`선택 필요: A/B/자동적용`처럼 보내고, 쿠폰 코드 원문이나 결제 민감값은 보내지 않습니다.",
		"쿠폰/할인 확인 외에는 장바구니/결제/최종 주문 버튼을 누르지 않습니다.",
	} {
		if !strings.Contains(koReply, want) {
			t.Fatalf("Korean discount-specific open card missing %q:\n%s", want, koReply)
		}
	}
	for _, bad := range []string{"검색어와 후보 상품명", "상위 3개 후보 비교표"} {
		if strings.Contains(koReply, bad) {
			t.Fatalf("Korean discount-specific open card should not show generic product-search copy %q:\n%s", bad, koReply)
		}
	}

	enAction := strings.Join(formatSignalCoupangOpenURLActionFor("en", rawURL), "\n")
	enReply := localizeCoupangDiscountOpenURLActionReply("en", enAction, rawURL)
	for _, want := range []string{
		"Requested opening the Coupang coupon/discount verification screen.",
		"Opened URL: https://cart.coupang.com/cartView.pang",
		"Check on the opened screen:",
		"Verify only the applied state that gives the lowest order total among auto coupons, Wow discounts, and instant discounts.",
		"`Coupon candidate A: discount type, estimated discount, post-discount total, conditions only`",
		"`Choice needed: A/B/auto-applied`; do not send raw coupon codes or payment-sensitive values.",
		"Do not click cart, payment, or final order while verifying coupons and discounts.",
	} {
		if !strings.Contains(enReply, want) {
			t.Fatalf("English discount-specific open card missing %q:\n%s", want, enReply)
		}
	}
	if assistantCheckoutPrepContainsHangul(enReply) {
		t.Fatalf("English discount-specific open card should not expose Korean:\n%s", enReply)
	}
	for _, bad := range []string{"Search query and candidate product names", "top 3 candidates"} {
		if strings.Contains(enReply, bad) {
			t.Fatalf("English discount-specific open card should not show generic product-search copy %q:\n%s", bad, enReply)
		}
	}
}

func TestAssistantReplyCoupangCartItemChoiceFollowUpUsesRecentContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-coupang-cart-items-followup"
	if err := appendSignalHistory(targetID, "쿠팡 장바구니 확인해", "쿠팡 최종 구매 화면 확인 단계입니다.\n장바구니 품목: 상품이 2개 보임\n아직 구매 버튼은 누르지 않았습니다."); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "장바구니에 상품이 두 개야. 확인해")
	for _, want := range []string{
		"쿠팡 장바구니 상품 확인 단계입니다.",
		"아직 결제/최종 주문 버튼은 누르지 않았습니다.",
		"이번 주문 대상 상품 하나만 남아 있는지",
		"쿠팡 장바구니를 열고, 이번 주문 대상 상품과 추가 상품 여부",
		"요청한 상품명/옵션/판매자만 장바구니에 남아 있는지",
		"다른 상품, 추천 추가 상품, 자동 추가 구성품",
		"장바구니 상품 후보를 구매값만으로 구분하는 방식:",
		"`장바구니 후보 A: 상품명·옵션·수량·판매자·예상 총액만`",
		"`장바구니 후보 B: 상품명·옵션·수량·판매자·예상 총액만`",
		"`선택 필요: A/B/삭제 필요`처럼 보내고, 주소·결제 민감값은 보내지 않습니다.",
		"`장바구니 상품 표시: 맞음`",
		"`장바구니 상품 표시: 여러 개 보임`이면 구매 가능 판정은 계속 중지됩니다.",
		"장바구니 상품이 확정되기 전에는 결제, 최종 주문 클릭을 실행하지 않습니다.",
		"실행 상태:",
		"열린 주소: https://cart.coupang.com/cartView.pang",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("cart item follow-up guide missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"장바구니 확인 단계입니다.", "구매 완료", "주문 완료", "장바구니에 담았습니다"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("cart item follow-up should not confirm cart or claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyCoupangCartItemChoiceEnglishFollowUpUsesRecentContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-coupang-cart-items-followup-en"
	if err := appendSignalHistory(targetID, "read the Coupang cart", "Coupang final purchase-screen review step.\nCart items: two products visible\nThe purchase button has not been clicked yet."); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "two items in cart, check it")
	for _, want := range []string{
		"Coupang cart-item verification step.",
		"No payment or final order button has been clicked yet.",
		"only the intended product for this order remains in the cart",
		"open the Coupang cart and verify the intended product",
		"only the requested product name, option, and seller remain in the cart",
		"another product, recommended add-on, or automatically added bundle",
		"How to distinguish cart candidates using purchase values only:",
		"`Cart candidate A: product name, option, quantity, seller, estimated total only`",
		"`Cart candidate B: product name, option, quantity, seller, estimated total only`",
		"`Choice needed: A/B/remove extra item`; do not send address or payment-sensitive values.",
		"`Cart item visible: yes`",
		"`Cart item visible: multiple items` keeps the purchase-readiness decision stopped.",
		"Before the cart item is confirmed, do not execute payment or final order clicks.",
		"Execution status:",
		"Opened URL: https://cart.coupang.com/cartView.pang",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English cart item follow-up guide missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English cart item follow-up should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"Cart confirmation step.", "purchase completed", "order completed", "added to cart"} {
		if strings.Contains(strings.ToLower(reply), strings.ToLower(bad)) {
			t.Fatalf("English cart item follow-up should not confirm cart or claim %q:\n%s", bad, reply)
		}
	}
}

func TestCoupangCartItemOpenURLActionUsesCartItemSpecificCard(t *testing.T) {
	rawURL := coupangCartItemReviewURL()
	koAction := strings.Join(formatSignalCoupangOpenURLActionFor("ko", rawURL), "\n")
	koReply := localizeCoupangCartItemOpenURLActionReply("ko", koAction, rawURL)
	for _, want := range []string{
		"쿠팡 장바구니 상품 확인 화면 열기를 요청했습니다.",
		"열린 주소: https://cart.coupang.com/cartView.pang",
		"열린 화면에서 확인할 것:",
		"요청한 상품명/옵션/판매자만 장바구니에 남아 있는지 확인합니다.",
		"`장바구니 후보 A: 상품명·옵션·수량·판매자·예상 총액만`",
		"`선택 필요: A/B/삭제 필요`처럼 보내고, 주소·결제 민감값은 보내지 않습니다.",
		"장바구니 상품 확인 외에는 결제/최종 주문 버튼을 누르지 않습니다.",
	} {
		if !strings.Contains(koReply, want) {
			t.Fatalf("Korean cart-item-specific open card missing %q:\n%s", want, koReply)
		}
	}
	for _, bad := range []string{"검색어와 후보 상품명", "상위 3개 후보 비교표"} {
		if strings.Contains(koReply, bad) {
			t.Fatalf("Korean cart-item-specific open card should not show generic product-search copy %q:\n%s", bad, koReply)
		}
	}

	enAction := strings.Join(formatSignalCoupangOpenURLActionFor("en", rawURL), "\n")
	enReply := localizeCoupangCartItemOpenURLActionReply("en", enAction, rawURL)
	for _, want := range []string{
		"Requested opening the Coupang cart-item verification screen.",
		"Opened URL: https://cart.coupang.com/cartView.pang",
		"Check on the opened screen:",
		"Verify that only the requested product name, option, and seller remain in the cart.",
		"`Cart candidate A: product name, option, quantity, seller, estimated total only`",
		"`Choice needed: A/B/remove extra item`; do not send address or payment-sensitive values.",
		"Do not click payment or final order while verifying cart items.",
	} {
		if !strings.Contains(enReply, want) {
			t.Fatalf("English cart-item-specific open card missing %q:\n%s", want, enReply)
		}
	}
	if assistantCheckoutPrepContainsHangul(enReply) {
		t.Fatalf("English cart-item-specific open card should not expose Korean:\n%s", enReply)
	}
	for _, bad := range []string{"Search query and candidate product names", "top 3 candidates"} {
		if strings.Contains(enReply, bad) {
			t.Fatalf("English cart-item-specific open card should not show generic product-search copy %q:\n%s", bad, enReply)
		}
	}
}

func TestAssistantShoppingFallbackLabelsEnglishUseLanguagePack(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "en")
	candidate := browserauto.Link{}
	replies := []string{
		formatShoppingCandidateCartPrepReply(candidate, ""),
		formatShoppingCandidateCartAddReply(candidate, ""),
		formatShoppingCandidateCheckoutReviewReply(candidate, coupangCartURL(), ""),
		formatShoppingCandidateCartConfirmReply(candidate, coupangCartURL(), ""),
		formatShoppingCandidateCompareReply("water", []browserauto.Link{{}}),
		formatShoppingCheckoutDecisionReply(candidate, checkoutScreenFields{}),
		renderShoppingCheckoutDecisionSVG(candidate, checkoutScreenFields{}),
	}
	joined := strings.Join(replies, "\n")
	for _, want := range []string{"Product candidate", "Recent shopping item"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("English shopping fallback should include %q:\n%s", want, joined)
		}
	}
	if assistantCheckoutPrepContainsHangul(joined) {
		t.Fatalf("English shopping fallback labels should not expose Korean:\n%s", joined)
	}
}

func TestAssistantScenarioReturnRequestStopsBeforeSubmit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "이 상품 반품 신청해줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	for _, want := range []string{
		"쿠팡 반품/환불/교환 준비 단계입니다.",
		"아직 반품 신청, 환불 요청, 교환 요청, 주문 취소 제출 버튼은 누르지 않았습니다.",
		"주문과 사유를 화면에서 확인하고 최종 제출 직전에서 멈춰야 합니다.",
		"쿠팡 주문목록을 열고, 반품/환불/교환 대상 주문을 찾습니다.",
		"대상 상품명, 주문일, 수량, 배송 상태",
		"반품/환불/교환 사유, 회수/재배송 방식, 배송비/수수료, 환불 예상 금액이나 교환 조건",
		"`반품/환불/교환 준비: 맞음`",
		"`제출 전 화면`이 보이면 멈추고 사용자 승인 요청",
		"이어 할 수 있는 말: `배송 조회해줘`, `영수증 확인해줘`, `취소 준비해줘`",
		"명시 승인 전에는 외부 계정 상태를 바꾸는 반품/환불/교환/주문취소 제출을 실행하지 않습니다.",
		"실행 상태:",
		"열린 주소: https://www.coupang.com/np/mycoupang/order/list",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("return/refund guard missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"반품 신청을 제출했습니다", "환불 요청을 제출했습니다", "주문 취소했습니다"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("return/refund guard should not claim submission %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantScenarioOrderCancelPrepStopsBeforeSubmit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "쿠팡 주문 취소해줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	for _, want := range []string{
		"쿠팡 주문 취소 준비 단계입니다.",
		"아직 주문 취소 신청/제출, 고객센터 전송, 환불/반품/교환 버튼은 누르지 않았습니다.",
		"취소 가능 여부와 환불/배송 영향만 확인하고 최종 취소 제출 전에서 멈춰야 합니다.",
		"쿠팡 주문목록을 열고 취소 대상 주문을 찾습니다.",
		"상품명, 주문일, 수량, 배송 상태가 취소하려는 주문과 맞는지 확인합니다.",
		"즉시 취소 가능, 판매자 승인 필요, 배송 시작됨, 고객센터 문의 필요",
		"환불 예상 금액, 취소 수수료/위약금, 배송/회수 영향, 취소 가능 기한",
		"`주문 취소 준비: 맞음`",
		"`최종 취소 제출 화면`이 보이면 멈추고 사용자 승인 요청",
		"이어 할 수 있는 말: `배송 조회해줘`, `영수증 확인해줘`, `반품 준비해줘`",
		"명시 승인 전에는 외부 계정 상태를 바꾸는 주문 취소 신청/제출을 실행하지 않습니다.",
		"열린 주소: https://www.coupang.com/np/mycoupang/order/list",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("order-cancel prep guard missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"쿠팡 반품/환불/교환 준비 단계", "구매 플로우를 중지했습니다", "쿠팡 구매 중지", "주문 취소했습니다"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("order-cancel prep guard should not use generic stop/submission copy %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantScenarioExchangePrepStopsBeforeSubmit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "쿠팡 교환 신청해줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	for _, want := range []string{
		"쿠팡 반품/환불/교환 준비 단계입니다.",
		"아직 반품 신청, 환불 요청, 교환 요청, 주문 취소 제출 버튼은 누르지 않았습니다.",
		"쿠팡 주문목록을 열고, 반품/환불/교환 대상 주문을 찾습니다.",
		"반품/환불/교환 사유, 회수/재배송 방식, 배송비/수수료, 환불 예상 금액이나 교환 조건",
		"주문 확인 외에는 반품 신청/환불 요청/교환 요청/주문 취소 제출 버튼을 누르지 않습니다.",
		"이어 할 수 있는 말: `배송 조회해줘`, `영수증 확인해줘`, `취소 준비해줘`",
		"명시 승인 전에는 외부 계정 상태를 바꾸는 반품/환불/교환/주문취소 제출을 실행하지 않습니다.",
		"열린 주소: https://www.coupang.com/np/mycoupang/order/list",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("exchange prep guard missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 플로우를 중지했습니다", "교환 요청을 제출했습니다", "교환 신청을 제출했습니다"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("exchange prep guard should not use generic stop/submission copy %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantScenarioReturnRequestEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "Return this Coupang item and request a refund")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	for _, want := range []string{
		"Coupang return/refund/exchange prep step.",
		"No return request, refund request, exchange request, or order-cancel submission button has been clicked yet.",
		"verify the order and reason on screen, then stop before final submission.",
		"open the Coupang order list and find the order for return/refund/exchange.",
		"product name, order date, quantity, and delivery status",
		"return/refund/exchange reason, pickup/reshipment method, shipping fee/penalty, expected refund amount, or exchange terms",
		"`Return/refund/exchange prep: yes`",
		"If the pre-submit screen is visible, stop and ask the user for approval",
		"You can continue with: `track delivery`, `check receipt`, or `prepare order cancellation`.",
		"Before explicit approval, do not submit any return, refund, exchange, or order-cancel action",
		"Execution status:",
		"Opened URL: https://www.coupang.com/np/mycoupang/order/list",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English return/refund guard missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English return/refund guard should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"return request submitted", "refund request submitted", "order canceled"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English return/refund guard should not claim submission %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantScenarioOrderCancelEnglishUsesReturnPrepCard(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "cancel this Coupang order")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	for _, want := range []string{
		"Coupang order-cancel prep step.",
		"No order-cancel request/submission, customer-service send, return, refund, or exchange button has been clicked yet.",
		"verify only cancellation availability plus refund/delivery impact, then stop before final cancellation submission.",
		"open the Coupang order list and find the order to cancel.",
		"product name, order date, quantity, and delivery status match the order",
		"instant cancel, seller approval required, shipment already started, or customer-service handoff",
		"refund estimate, cancellation fee/penalty, delivery/pickup impact, and cancellation deadline",
		"`Order cancel prep: yes`",
		"If the final cancellation-submit screen is visible, stop and ask the user for approval",
		"You can continue with: `track delivery`, `check receipt`, or `prepare return`.",
		"Before explicit approval, do not submit or request an order cancellation",
		"Opened URL: https://www.coupang.com/np/mycoupang/order/list",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English order-cancel prep guard missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English order-cancel prep guard should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"return/refund/exchange prep step", "purchase flow has been stopped", "order canceled"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English order-cancel prep guard should not use generic stop/submission copy %q:\n%s", bad, reply)
		}
	}
}

func TestCoupangOrderCancelOpenURLActionUsesCancelSpecificCard(t *testing.T) {
	rawURL := coupangReturnRefundReviewURL()
	koAction := strings.Join(formatSignalCoupangOpenURLActionFor("ko", rawURL), "\n")
	koReply := localizeCoupangOrderCancelOpenURLActionReply("ko", koAction, rawURL)
	for _, want := range []string{
		"쿠팡 주문 취소 준비 화면 열기를 요청했습니다.",
		"열린 주소: https://www.coupang.com/np/mycoupang/order/list",
		"열린 화면에서 확인할 것:",
		"즉시 취소 가능, 판매자 승인 필요, 배송 시작됨, 고객센터 문의 필요",
		"주문 취소 가능 여부 확인 외에는 주문 취소 신청/제출",
		"명시 승인 전에는 외부 계정 상태를 바꾸는 주문 취소 신청/제출을 실행하지 않습니다.",
	} {
		if !strings.Contains(koReply, want) {
			t.Fatalf("Korean order-cancel-specific open card missing %q:\n%s", want, koReply)
		}
	}
	for _, bad := range []string{"반품/환불/교환 준비 화면", "상위 3개 후보 비교표"} {
		if strings.Contains(koReply, bad) {
			t.Fatalf("Korean order-cancel-specific open card should not show generic copy %q:\n%s", bad, koReply)
		}
	}

	enAction := strings.Join(formatSignalCoupangOpenURLActionFor("en", rawURL), "\n")
	enReply := localizeCoupangOrderCancelOpenURLActionReply("en", enAction, rawURL)
	for _, want := range []string{
		"Requested opening the Coupang order-cancel prep screen.",
		"Opened URL: https://www.coupang.com/np/mycoupang/order/list",
		"Check on the opened screen:",
		"instant cancel, seller approval required, shipment already started, or customer-service handoff",
		"Other than checking cancellation availability, do not click order-cancel request/submission",
		"Before explicit approval, do not submit or request an order cancellation",
	} {
		if !strings.Contains(enReply, want) {
			t.Fatalf("English order-cancel-specific open card missing %q:\n%s", want, enReply)
		}
	}
	if assistantCheckoutPrepContainsHangul(enReply) {
		t.Fatalf("English order-cancel-specific open card should not expose Korean:\n%s", enReply)
	}
	for _, bad := range []string{"return/refund/exchange prep screen", "Top 3 candidate comparison"} {
		if strings.Contains(enReply, bad) {
			t.Fatalf("English order-cancel-specific open card should not show generic copy %q:\n%s", bad, enReply)
		}
	}
}

func TestAssistantScenarioExchangeEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "Exchange this Coupang item")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	for _, want := range []string{
		"Coupang return/refund/exchange prep step.",
		"No return request, refund request, exchange request, or order-cancel submission button has been clicked yet.",
		"open the Coupang order list and find the order for return/refund/exchange.",
		"return/refund/exchange reason, pickup/reshipment method, shipping fee/penalty, expected refund amount, or exchange terms",
		"Do not click return, refund, exchange, order-cancel, or final submission buttons while verifying the order.",
		"Before explicit approval, do not submit any return, refund, exchange, or order-cancel action",
		"Opened URL: https://www.coupang.com/np/mycoupang/order/list",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English exchange prep guard missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English exchange prep guard should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"purchase flow has been stopped", "exchange request submitted", "exchange submitted"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English exchange prep guard should not use generic stop/submission copy %q:\n%s", bad, reply)
		}
	}
}

func TestCoupangReturnRefundOpenURLActionUsesReturnSpecificCard(t *testing.T) {
	rawURL := coupangReturnRefundReviewURL()
	koAction := strings.Join(formatSignalCoupangOpenURLActionFor("ko", rawURL), "\n")
	koReply := localizeCoupangReturnRefundOpenURLActionReply("ko", koAction, rawURL)
	for _, want := range []string{
		"쿠팡 반품/환불/교환 준비 화면 열기를 요청했습니다.",
		"열린 주소: https://www.coupang.com/np/mycoupang/order/list",
		"열린 화면에서 확인할 것:",
		"대상 상품명, 주문일, 수량, 배송 상태가 사용자가 의도한 주문과 맞는지 확인합니다.",
		"주문 확인 외에는 반품 신청/환불 요청/교환 요청/주문 취소 제출 버튼을 누르지 않습니다.",
	} {
		if !strings.Contains(koReply, want) {
			t.Fatalf("Korean return-specific open card missing %q:\n%s", want, koReply)
		}
	}
	for _, bad := range []string{"검색어와 후보 상품명", "상위 3개 후보 비교표"} {
		if strings.Contains(koReply, bad) {
			t.Fatalf("Korean return-specific open card should not show generic product-search copy %q:\n%s", bad, koReply)
		}
	}

	enAction := strings.Join(formatSignalCoupangOpenURLActionFor("en", rawURL), "\n")
	enReply := localizeCoupangReturnRefundOpenURLActionReply("en", enAction, rawURL)
	for _, want := range []string{
		"Requested opening the Coupang return/refund/exchange prep screen.",
		"Opened URL: https://www.coupang.com/np/mycoupang/order/list",
		"Check on the opened screen:",
		"Verify that product name, order date, quantity, and delivery status match the user's intended order.",
		"Do not click return, refund, exchange, order-cancel, or final submission buttons while verifying the order.",
	} {
		if !strings.Contains(enReply, want) {
			t.Fatalf("English return-specific open card missing %q:\n%s", want, enReply)
		}
	}
	if assistantCheckoutPrepContainsHangul(enReply) {
		t.Fatalf("English return-specific open card should not expose Korean:\n%s", enReply)
	}
	for _, bad := range []string{"Search query and candidate product names", "top 3 candidates"} {
		if strings.Contains(enReply, bad) {
			t.Fatalf("English return-specific open card should not show generic product-search copy %q:\n%s", bad, enReply)
		}
	}
}

func TestAssistantScenarioPriceAlertCollectsSlots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_ASSISTANT_WATCHES", filepath.Join(home, "assistant-watches.json"))

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "러닝 벨트 3만원 아래로 가격 내려가면 알려줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "가격 알림을 등록") ||
		!strings.Contains(reply, "watch id:") ||
		!strings.Contains(reply, "scheduler") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantScenarioPriceAlertListAndDisable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_ASSISTANT_WATCHES", filepath.Join(home, "assistant-watches.json"))

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "러닝 벨트 3만원 아래로 가격 내려가면 알려줘")
	if !handled || !strings.Contains(reply, "가격 알림을 등록") {
		t.Fatalf("reply=%q handled=%t", reply, handled)
	}
	reply, handled = shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "가격 알림 목록")
	if !handled || !strings.Contains(reply, "등록된 가격 알림") || !strings.Contains(reply, "러닝 벨트") {
		t.Fatalf("reply=%q handled=%t", reply, handled)
	}
	reply, handled = shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "러닝 가격 알림 중지")
	if !handled || !strings.Contains(reply, "가격 알림을 중지") {
		t.Fatalf("reply=%q handled=%t", reply, handled)
	}
}

func TestAssistantScenarioCoupangProductSearchOpensCoupangSearch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "쿠팡에서 러닝 벨트 3만원 이하 찾아줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "open_url") || !strings.Contains(reply, "coupang.com/np/search") || strings.Contains(reply, "google.com/search") {
		t.Fatalf("coupang search should open Coupang search, got:\n%s", reply)
	}
	for _, want := range []string{"쿠팡 후보 비교를 시작", "검색어: 러닝 벨트 3만원 이하", "바로 확인할 항목", "상위 3개 후보 비교표", "구매 실행 승인"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("coupang search reply missing %q:\n%s", want, reply)
		}
	}
}

func TestAssistantScenarioCoupangLivePrepEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")

	readiness := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "Coupang purchase readiness check")
	for _, want := range []string{
		"Coupang live purchase test readiness.",
		"Signal assistant-room response: ok",
		"Browser login still needs direct verification.",
		"purchase execution approved",
	} {
		if !strings.Contains(readiness, want) {
			t.Fatalf("English readiness reply missing %q:\n%s", want, readiness)
		}
	}
	if assistantCheckoutPrepContainsHangul(readiness) {
		t.Fatalf("English readiness reply should not expose Korean:\n%s", readiness)
	}

	prep := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "Tomorrow Coupang real purchase test prep")
	for _, want := range []string{
		"Coupang live purchase test prep.",
		"Send these messages in Signal tomorrow:",
		"`open Coupang`",
		"Argos does not enter passwords",
		"purchase execution approved",
	} {
		if !strings.Contains(prep, want) {
			t.Fatalf("English prep reply missing %q:\n%s", want, prep)
		}
	}
	if assistantCheckoutPrepContainsHangul(prep) {
		t.Fatalf("English prep reply should not expose Korean:\n%s", prep)
	}

	login := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "Open the Coupang login page")
	for _, want := range []string{
		"Opening the Coupang login page in the browser requires execution approval.",
		"https://www.coupang.com/",
		"The user must enter login credentials directly.",
	} {
		if !strings.Contains(login, want) {
			t.Fatalf("English login reply missing %q:\n%s", want, login)
		}
	}
	if assistantCheckoutPrepContainsHangul(login) {
		t.Fatalf("English login reply should not expose Korean approval text:\n%s", login)
	}
}

func TestCoupangShoppingPrepReplyIncludesCandidateTable(t *testing.T) {
	reply := coupangShoppingPrepReply("실행 상태 테스트", "생수 500ml 20", browserauto.Link{
		Text: "탐사수 500ml 20개",
		URL:  "https://www.coupang.com/vp/products/123",
	}, browserauto.Link{
		Text: "아이시스 500ml 20개",
		URL:  "https://www.coupang.com/vp/products/456",
	})
	for _, want := range []string{"검색 결과 후보", "1. 탐사수 500ml 20개", "2. 아이시스 500ml 20개", "실제 가격, 로켓배송, 리뷰", "상위 3개 후보 비교표"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
}

func TestAssistantReplyShoppingCandidateCompareFollowUpUsesRecentCandidates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	targetID := "argos-assistant-shopping-followup"
	previous := coupangShoppingPrepReply("실행 상태 테스트", "생수 500ml 20", browserauto.Link{
		Text: "풀무원샘물 무라벨 생수, 500ml, 20개 - 국산생수 | 쿠팡",
		URL:  "https://www.coupang.com/vp/products/8288410923",
	}, browserauto.Link{
		Text: "아이시스 500ml 20개",
		URL:  "https://www.coupang.com/vp/products/456",
	})
	if err := appendSignalHistory(targetID, "쿠팡에서 생수 500ml 20개 로켓배송으로 가격, 리뷰 좋은 후보 3개 비교해줘", previous); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "상위 3개 후보 비교표 만들어줘")
	for _, want := range []string{"상위 후보 비교표", "| 후보 | 상품/링크 | 가격 | 배송 | 리뷰 | 다음 행동 |", "풀무원샘물 무라벨 생수", "상품 페이지에서 확인", "첫 번째 상품 상세 열고 총액 확인"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "국산생수 | 쿠팡") {
		t.Fatalf("table cell should escape pipe characters:\n%s", reply)
	}
}

func TestAssistantReplyShoppingCandidateCompareFollowUpEnglishHistoryUsesLanguagePackWithoutEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	targetID := "argos-assistant-shopping-followup-en-history"
	previous := coupangShoppingPrepReplyFor("en", "browser action test", "water 500ml 20 pack", browserauto.Link{
		Text: "Pulmuone bottled water 500ml 20 pack",
		URL:  "https://www.coupang.com/vp/products/111",
	}, browserauto.Link{
		Text: "Icis bottled water 500ml 20 pack",
		URL:  "https://www.coupang.com/vp/products/456",
	})
	if err := appendSignalHistory(targetID, "Search Coupang for water 500ml 20 pack and compare three candidates", previous); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "make a top-three candidate comparison table")
	for _, want := range []string{
		"Top candidate comparison table.",
		"Search query: water 500ml 20 pack",
		"| Candidate | Product/link | Price | Shipping | Reviews | Next action |",
		"Pulmuone bottled water 500ml 20 pack",
		"check on product page",
		"open detail page and verify final total",
		"No cart, checkout, payment, or purchase has been made.",
		"purchase execution approved",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English compare follow-up missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English compare follow-up should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"상위 후보 비교표", "기준 검색어", "상품 페이지에서 확인", "구매 실행 승인", "결제"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("English compare follow-up should not contain %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingCandidateDetailFollowUpUsesRecentCandidates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-shopping-detail-followup"
	previous := coupangShoppingPrepReply("실행 상태 테스트", "생수 500ml 20", browserauto.Link{
		Text: "풀무원샘물 무라벨 생수, 500ml, 20개 - 국산생수 | 쿠팡",
		URL:  "https://www.coupang.com/vp/products/8288410923",
	}, browserauto.Link{
		Text: "아이시스 500ml 20개",
		URL:  "https://www.coupang.com/vp/products/456",
	})
	if err := appendSignalHistory(targetID, "쿠팡에서 생수 500ml 20개 로켓배송으로 가격, 리뷰 좋은 후보 3개 비교해줘", previous); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "첫 번째 상품 상세 열고 총액 확인해줘")
	for _, want := range []string{"상품 상세 확인을 시작", "선택 후보: 1번", "풀무원샘물 무라벨 생수", "https://www.coupang.com/vp/products/8288410923", "상품가, 배송비, 쿠폰 적용 후 최종 총액", "구매 실행 승인"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "1번 뉴스") {
		t.Fatalf("shopping detail follow-up was misrouted as news:\n%s", reply)
	}
}

func TestAssistantReplyShoppingCandidateDetailFollowUpEnglishHistoryUsesLanguagePackWithoutEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SIGNAL_LOCAL_AUTO_GRANT", "0")
	targetID := "argos-assistant-shopping-detail-followup-en-history"
	previous := coupangShoppingPrepReplyFor("en", "browser action test", "water 500ml 20 pack", browserauto.Link{
		Text: "Pulmuone bottled water 500ml 20 pack",
		URL:  "https://www.coupang.com/vp/products/111",
	}, browserauto.Link{
		Text: "Icis bottled water 500ml 20 pack",
		URL:  "https://www.coupang.com/vp/products/456",
	})
	if err := appendSignalHistory(targetID, "Search Coupang for water 500ml 20 pack and compare three candidates", previous); err != nil {
		t.Fatal(err)
	}
	if !isShoppingCandidateDetailFollowUp("open candidate 1 and check the final total") {
		t.Fatal("English detail follow-up phrase was not detected")
	}
	query, candidates, recentLocale, ok := latestShoppingCandidatesFromHistoryWithLocale(targetID)
	if !ok || recentLocale != "en" || query != "water 500ml 20 pack" || len(candidates) != 2 {
		t.Fatalf("recent candidates not parsed: ok=%t locale=%q query=%q candidates=%#v\nprevious:\n%s", ok, recentLocale, query, candidates, previous)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "open candidate 1 and check the final total")
	for _, want := range []string{
		"Started product detail check.",
		"Selected candidate: #1",
		"Product: Pulmuone bottled water 500ml 20 pack",
		"Opened link: https://www.coupang.com/vp/products/111",
		"Check on the detail page:",
		"Final total after item price, shipping, and coupons",
		"Next: `prepare this product in the cart, but do not pay`",
		"No cart, checkout, payment, or purchase has been made.",
		"Execution status:",
		"Coupang browser-test screen needs Mac action permission.",
		"Action: open_url",
		"Reply `run` to execute it once.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English detail follow-up missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(signalReplyVisibleText(reply)) {
		t.Fatalf("English detail follow-up should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"상품 상세 확인", "선택 후보", "상세 화면에서 확인", "구매 실행 승인", "Mac 작업 실행 권한", "작업: open_url", "purchase completed", "order completed"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("English detail follow-up should not contain %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingCandidateDetailFollowUpSelectsSecondCandidate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-shopping-detail-second"
	previous := coupangShoppingPrepReply("실행 상태 테스트", "생수 500ml 20", browserauto.Link{
		Text: "풀무원샘물 무라벨 생수",
		URL:  "https://www.coupang.com/vp/products/111",
	}, browserauto.Link{
		Text: "아이시스 500ml 20개",
		URL:  "https://www.coupang.com/vp/products/456",
	})
	if err := appendSignalHistory(targetID, "쿠팡에서 생수 500ml 20개 로켓배송으로 가격, 리뷰 좋은 후보 3개 비교해줘", previous); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "2번 상품도 열어줘")
	for _, want := range []string{"상품 상세 확인을 시작", "선택 후보: 2번", "아이시스 500ml 20개", "https://www.coupang.com/vp/products/456"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "1번 뉴스") {
		t.Fatalf("shopping detail follow-up was misrouted as news:\n%s", reply)
	}
}

func TestAssistantReplyShoppingCandidateSelectionRecordsChoice(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	targetID := "argos-assistant-shopping-selection"
	previous := coupangShoppingPrepReply("실행 상태 테스트", "생수 500ml 20", browserauto.Link{
		Text: "풀무원샘물 무라벨 생수",
		URL:  "https://www.coupang.com/vp/products/111",
	}, browserauto.Link{
		Text: "아이시스 500ml 20개",
		URL:  "https://www.coupang.com/vp/products/456",
	})
	if err := appendSignalHistory(targetID, "쿠팡에서 생수 500ml 20개 로켓배송으로 가격, 리뷰 좋은 후보 3개 비교해줘", previous); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "1번으로 할게")
	for _, want := range []string{
		"후보 선택을 기록했습니다.",
		"선택 후보: 1번",
		"선택 상품: 풀무원샘물 무라벨 생수",
		"상품 링크: https://www.coupang.com/vp/products/111",
		"검색어: 생수 500ml 20",
		"`1번 상품 상세 열고 총액 확인해줘`",
		"`1번 상품을 장바구니 또는 구매 직전 화면까지 준비해줘. 결제는 하지 마`",
		"후보 선택만으로 장바구니/결제/최종 주문 버튼을 누르지 않습니다.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("selection reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"장바구니 담기 실행", "구매 완료", "주문 완료", "최종 주문 버튼을 눌렀습니다", "CVC: 123"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("selection reply should not contain %q:\n%s", bad, reply)
		}
	}
}

func legacyTestAssistantShoppingCandidateSelectionAlignsPendingDirectPurchase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	targetID := "argos-assistant-shopping-selection-pending-direct"
	rememberPendingShoppingDirectPurchase(ListenOptions{TargetID: targetID, Mode: "assistant"}, "생수 구매해", "생수")
	previous := coupangShoppingPrepReply("실행 상태 테스트", "생수 500ml 20", browserauto.Link{
		Text: "풀무원샘물 무라벨 생수",
		URL:  "https://www.coupang.com/vp/products/111",
	}, browserauto.Link{
		Text: "아이시스 500ml 20개",
		URL:  "https://www.coupang.com/vp/products/456",
	})
	if err := appendSignalHistory(targetID, "쿠팡에서 생수 후보 비교해줘", previous); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "2번으로 할게")
	for _, want := range []string{
		"후보 선택을 기록했습니다.",
		"선택 상품: 아이시스 500ml 20개",
		"구매 대기 상품도 이 후보로 맞췄습니다.",
		"`현재 화면에서 다시 해봐`",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("selection pending alignment missing %q:\n%s", want, reply)
		}
	}
	pending, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false)
	if !ok || pending.Kind != "shopping_direct_purchase" || pending.Intent.Query != "아이시스 500ml 20개" || pending.Intent.URL != "https://www.coupang.com/vp/products/456" {
		t.Fatalf("pending direct purchase was not aligned to selected candidate: %#v ok=%v", pending, ok)
	}
}

func legacyTestAssistantShoppingCandidateSelectionClearsStalePendingURLForNonProduct(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	targetID := "argos-assistant-shopping-selection-pending-direct-non-product"
	rememberPendingShoppingDirectPurchaseWithURLAndBlocker(ListenOptions{TargetID: targetID, Mode: "assistant"}, "생수 구매해", "아이시스 500ml 20개", "https://www.coupang.com/vp/products/456", "")
	previous := coupangShoppingPrepReply("실행 상태 테스트", "생수", browserauto.Link{
		Text: "쿠팡 검색 홈",
		URL:  "https://www.coupang.com/",
	})
	if err := appendSignalHistory(targetID, "쿠팡에서 생수 후보 비교해줘", previous); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "1번으로 할게")
	if !strings.Contains(reply, "구매 대기 상품도 이 후보로 맞췄습니다.") {
		t.Fatalf("non-product selection should still align pending query:\n%s", reply)
	}
	pending, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false)
	if !ok || pending.Kind != "shopping_direct_purchase" || pending.Intent.Query != "쿠팡 검색 홈" || pending.Intent.URL != "" {
		t.Fatalf("non-product candidate should clear stale direct purchase URL: %#v ok=%v", pending, ok)
	}
}

func TestIsCoupangProductURLRequiresHTTPProductURL(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{raw: "https://www.coupang.com/vp/products/456", want: true},
		{raw: "http://www.coupang.com/vp/products/456", want: true},
		{raw: "https://www.coupang.com/np/search?q=%ED%9C%B4%EC%A7%80", want: false},
		{raw: "javascript://www.coupang.com/vp/products/456", want: false},
		{raw: "ftp://www.coupang.com/vp/products/456", want: false},
	}
	for _, tc := range cases {
		if got := isCoupangProductURL(tc.raw); got != tc.want {
			t.Fatalf("isCoupangProductURL(%q)=%v want %v", tc.raw, got, tc.want)
		}
	}
}

func TestAssistantReplyShoppingCandidateSelectionEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	targetID := "argos-assistant-shopping-selection-en"
	previous := coupangShoppingPrepReply("browser action test", "water 500ml 20 pack", browserauto.Link{
		Text: "Pulmuone bottled water",
		URL:  "https://www.coupang.com/vp/products/111",
	}, browserauto.Link{
		Text: "Icis 500ml 20 pack",
		URL:  "https://www.coupang.com/vp/products/456",
	})
	if err := appendSignalHistory(targetID, "search Coupang for water 500ml 20 pack and compare three candidates", previous); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "Choose candidate 2.")
	for _, want := range []string{
		"Candidate selection recorded.",
		"Selected candidate: #2",
		"Selected product: Icis 500ml 20 pack",
		"Product link: https://www.coupang.com/vp/products/456",
		"Search query: water 500ml 20 pack",
		"`open candidate 2 and check the final total`",
		"`prepare candidate 2 up to the cart or pre-purchase screen, but do not pay`",
		"Selecting a candidate alone does not click cart, payment, or final order buttons.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English selection reply missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English selection reply should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"purchase completed", "order completed", "clicked the final order button", "cvc: 123", "card number:"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English selection reply should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func legacyTestAssistantShoppingCandidateSelectionAlignsPendingDirectPurchaseEnglish(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	targetID := "argos-assistant-shopping-selection-pending-direct-en"
	rememberPendingShoppingDirectPurchase(ListenOptions{TargetID: targetID, Mode: "assistant"}, "buy water", "water")
	previous := coupangShoppingPrepReplyFor("en", "browser action test", "water 500ml 20 pack", browserauto.Link{
		Text: "Pulmuone bottled water",
		URL:  "https://www.coupang.com/vp/products/111",
	}, browserauto.Link{
		Text: "Icis 500ml 20 pack",
		URL:  "https://www.coupang.com/vp/products/456",
	})
	if err := appendSignalHistory(targetID, "search Coupang for water 500ml 20 pack and compare three candidates", previous); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "Choose candidate 2.")
	for _, want := range []string{
		"Candidate selection recorded.",
		"Selected product: Icis 500ml 20 pack",
		"The pending purchase item is now aligned to this candidate.",
		"`try again from current screen`",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English selection pending alignment missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English selection pending alignment should not expose Korean:\n%s", reply)
	}
	pending, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false)
	if !ok || pending.Kind != "shopping_direct_purchase" || pending.Intent.Query != "Icis 500ml 20 pack" || pending.Intent.URL != "https://www.coupang.com/vp/products/456" {
		t.Fatalf("English pending direct purchase was not aligned to selected candidate: %#v ok=%v", pending, ok)
	}
}

func TestAssistantReplyShoppingCandidateSelectionNeedsNumber(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	targetID := "argos-assistant-shopping-selection-needs-number"
	previous := coupangShoppingPrepReply("실행 상태 테스트", "생수 500ml 20", browserauto.Link{
		Text: "풀무원샘물 무라벨 생수",
		URL:  "https://www.coupang.com/vp/products/111",
	}, browserauto.Link{
		Text: "아이시스 500ml 20개",
		URL:  "https://www.coupang.com/vp/products/456",
	})
	if err := appendSignalHistory(targetID, "쿠팡에서 생수 500ml 20개 로켓배송으로 가격, 리뷰 좋은 후보 3개 비교해줘", previous); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "이걸로 할게")
	for _, want := range []string{
		"어떤 후보인지 번호가 필요합니다.",
		"후보 번호가 없어서 장바구니/결제/최종 주문 버튼은 누르지 않았습니다.",
		"검색어: 생수 500ml 20",
		"최근 후보:",
		"1. 풀무원샘물 무라벨 생수",
		"2. 아이시스 500ml 20개",
		"`1번으로 할게`",
		"`2번 상품 상세 열고 총액 확인해줘`",
		"번호 없이는 선택, 장바구니 준비, 결제, 최종 주문을 진행하지 않습니다.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("selection number-required reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"선택 후보:", "장바구니 담기 실행", "구매 완료", "주문 완료", "최종 주문 버튼을 눌렀습니다", "CVC: 123"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("selection number-required reply should not contain %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingCandidateSelectionNeedsNumberEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	targetID := "argos-assistant-shopping-selection-needs-number-en"
	previous := coupangShoppingPrepReply("browser action test", "water 500ml 20 pack", browserauto.Link{
		Text: "Pulmuone bottled water",
		URL:  "https://www.coupang.com/vp/products/111",
	}, browserauto.Link{
		Text: "Icis 500ml 20 pack",
		URL:  "https://www.coupang.com/vp/products/456",
	})
	if err := appendSignalHistory(targetID, "search Coupang for water 500ml 20 pack and compare three candidates", previous); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "Use this one.")
	for _, want := range []string{
		"I need the candidate number.",
		"Because no candidate number was provided",
		"Search query: water 500ml 20 pack",
		"Recent candidates:",
		"1. Pulmuone bottled water",
		"2. Icis 500ml 20 pack",
		"`choose candidate 1`",
		"`open candidate 2 and check the final total`",
		"Without a number, I do not select, prepare the cart, pay, or place the final order.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English selection number-required reply missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English selection number-required reply should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"selected candidate:", "purchase completed", "order completed", "clicked the final order button", "cvc: 123", "card number:"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English selection number-required reply should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingCandidateSelectionOutOfRangeShowsRecentCandidates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	targetID := "argos-assistant-shopping-selection-out-of-range"
	previous := coupangShoppingPrepReply("실행 상태 테스트", "생수 500ml 20", browserauto.Link{
		Text: "풀무원샘물 무라벨 생수",
		URL:  "https://www.coupang.com/vp/products/111",
	}, browserauto.Link{
		Text: "아이시스 500ml 20개",
		URL:  "https://www.coupang.com/vp/products/456",
	})
	if err := appendSignalHistory(targetID, "쿠팡에서 생수 500ml 20개 로켓배송으로 가격, 리뷰 좋은 후보 3개 비교해줘", previous); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "4번으로 할게")
	for _, want := range []string{
		"요청한 후보 번호가 최근 목록 범위를 벗어났습니다.",
		"4번 후보는 없고 최근 후보는 2개입니다.",
		"장바구니/결제/최종 주문 버튼은 누르지 않았습니다.",
		"검색어: 생수 500ml 20",
		"최근 후보:",
		"1. 풀무원샘물 무라벨 생수",
		"2. 아이시스 500ml 20개",
		"`1번으로 할게`처럼 1번부터 2번 사이의 후보 번호를 다시 보내세요.",
		"범위를 벗어난 번호로는 선택, 장바구니 준비, 결제, 최종 주문을 진행하지 않습니다.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("selection out-of-range reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"선택 후보:", "장바구니 담기 실행", "구매 완료", "주문 완료", "최종 주문 버튼을 눌렀습니다", "CVC: 123"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("selection out-of-range reply should not contain %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingCandidateSelectionOutOfRangeEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	targetID := "argos-assistant-shopping-selection-out-of-range-en"
	previous := coupangShoppingPrepReply("browser action test", "water 500ml 20 pack", browserauto.Link{
		Text: "Pulmuone bottled water",
		URL:  "https://www.coupang.com/vp/products/111",
	}, browserauto.Link{
		Text: "Icis 500ml 20 pack",
		URL:  "https://www.coupang.com/vp/products/456",
	})
	if err := appendSignalHistory(targetID, "search Coupang for water 500ml 20 pack and compare three candidates", previous); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "Choose candidate 4.")
	for _, want := range []string{
		"The requested candidate number is outside the recent list.",
		"Candidate #4 does not exist; the recent list has 2 candidate(s).",
		"I did not click cart, payment, or final order buttons.",
		"Search query: water 500ml 20 pack",
		"Recent candidates:",
		"1. Pulmuone bottled water",
		"2. Icis 500ml 20 pack",
		"Send a valid number from 1 to 2",
		"With an out-of-range number, I do not select, prepare the cart, pay, or place the final order.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English selection out-of-range reply missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English selection out-of-range reply should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"selected candidate:", "purchase completed", "order completed", "clicked the final order button", "cvc: 123", "card number:"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English selection out-of-range reply should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingCandidateCriterionNeedsVerification(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	targetID := "argos-assistant-shopping-selection-criterion"
	previous := coupangShoppingPrepReply("실행 상태 테스트", "생수 500ml 20", browserauto.Link{
		Text: "풀무원샘물 무라벨 생수",
		URL:  "https://www.coupang.com/vp/products/111",
	}, browserauto.Link{
		Text: "아이시스 500ml 20개",
		URL:  "https://www.coupang.com/vp/products/456",
	})
	if err := appendSignalHistory(targetID, "쿠팡에서 생수 500ml 20개 로켓배송으로 가격, 리뷰 좋은 후보 3개 비교해줘", previous); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "제일 싼 걸로 할게")
	for _, want := range []string{
		"기준만으로는 후보를 확정하지 않습니다.",
		"가격/배송/리뷰를 화면에서 다시 확인해야 하므로",
		"기준: 최저가/가성비",
		"검색어: 생수 500ml 20",
		"확인할 최근 후보:",
		"1. 풀무원샘물 무라벨 생수",
		"2. 아이시스 500ml 20개",
		"`상위 3개 후보 비교표 만들어줘`",
		"`1번 상품 상세 열고 총액 확인해줘`",
		"확인 뒤 `1번으로 할게`",
		"기준만 말한 상태에서는 후보 선택, 장바구니 준비, 결제, 최종 주문을 진행하지 않습니다.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("criterion verification reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"선택 후보:", "장바구니 담기 실행", "구매 완료", "주문 완료", "최종 주문 버튼을 눌렀습니다", "CVC: 123"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("criterion verification reply should not contain %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingCandidateCriterionEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	targetID := "argos-assistant-shopping-selection-criterion-en"
	previous := coupangShoppingPrepReply("browser action test", "water 500ml 20 pack", browserauto.Link{
		Text: "Pulmuone bottled water",
		URL:  "https://www.coupang.com/vp/products/111",
	}, browserauto.Link{
		Text: "Icis 500ml 20 pack",
		URL:  "https://www.coupang.com/vp/products/456",
	})
	if err := appendSignalHistory(targetID, "search Coupang for water 500ml 20 pack and compare three candidates", previous); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "Choose the cheapest one.")
	for _, want := range []string{
		"A criterion alone is not enough to choose a candidate.",
		"Because price, delivery, and reviews must be rechecked on screen",
		"Criterion: lowest price / best value",
		"Search query: water 500ml 20 pack",
		"Recent candidates to verify:",
		"1. Pulmuone bottled water",
		"2. Icis 500ml 20 pack",
		"`make a top-three candidate comparison table`",
		"`open candidate 1 and check the final total`",
		"Then choose by number, such as `choose candidate 1`.",
		"With only a criterion, I do not select a candidate, prepare the cart, pay, or place the final order.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English criterion verification reply missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English criterion verification reply should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"selected candidate:", "purchase completed", "order completed", "clicked the final order button", "cvc: 123", "card number:"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English criterion verification reply should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingCandidateCartPrepUsesLatestSelection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-shopping-cart-selection"
	previous := coupangShoppingPrepReply("실행 상태 테스트", "생수 500ml 20", browserauto.Link{
		Text: "풀무원샘물 무라벨 생수",
		URL:  "https://www.coupang.com/vp/products/111",
	}, browserauto.Link{
		Text: "아이시스 500ml 20개",
		URL:  "https://www.coupang.com/vp/products/456",
	})
	if err := appendSignalHistory(targetID, "쿠팡에서 생수 500ml 20개 로켓배송으로 가격, 리뷰 좋은 후보 3개 비교해줘", previous); err != nil {
		t.Fatal(err)
	}
	selection := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "2번으로 할게")
	if !strings.Contains(selection, "선택 후보: 2번") {
		t.Fatalf("selection did not select second candidate:\n%s", selection)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "이 상품으로 장바구니까지 준비해. 결제는 하지 마")
	for _, want := range []string{"장바구니 준비 단계", "대상 상품: 아이시스 500ml 20개", "https://www.coupang.com/vp/products/456", "장바구니에 담기 전에 확인할 것", "최종 주문 클릭은 `구매 실행 승인`"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("cart prep after selection missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "풀무원샘물") {
		t.Fatalf("cart prep should use latest selected candidate, not first candidate:\n%s", reply)
	}
}

func TestAssistantReplyShoppingCandidateCartPrepUsesLatestDetail(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-shopping-cart-detail"
	candidate := browserauto.Link{
		Text: "풀무원샘물 무라벨 생수, 500ml, 20개",
		URL:  "https://www.coupang.com/vp/products/8288410923",
	}
	previous := coupangShoppingPrepReply("실행 상태 테스트", "생수 500ml 20", candidate)
	if err := appendSignalHistory(targetID, "쿠팡에서 생수 500ml 20개 로켓배송으로 가격, 리뷰 좋은 후보 3개 비교해줘", previous); err != nil {
		t.Fatal(err)
	}
	detail := formatShoppingCandidateDetailReply(0, candidate, "실행 상태 테스트")
	if err := appendSignalHistory(targetID, "첫 번째 상품 상세 열고 총액 확인해줘", detail); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "이 상품으로 장바구니까지 준비해. 결제는 하지 마")
	for _, want := range []string{"장바구니 준비 단계", "대상 상품: 풀무원샘물 무라벨 생수", "https://www.coupang.com/vp/products/8288410923", "장바구니에 담기 전에 확인할 것", "상품가, 배송비, 쿠폰", "기본 배송지", "최종 주문 클릭은 `구매 실행 승인`"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"상품 링크나 정확한 상품명을 보내주세요", "1번 뉴스", "구매 완료", "주문 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("cart prep reply should not contain %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingCandidateCartPrepEnglishDetailUsesLanguagePackWithoutEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SIGNAL_LOCAL_AUTO_GRANT", "0")
	targetID := "argos-assistant-shopping-cart-detail-en"
	candidate := browserauto.Link{
		Text: "Pulmuone bottled water 500ml 20 pack",
		URL:  "https://www.coupang.com/vp/products/111",
	}
	detail := formatShoppingCandidateDetailReplyFor("en", 0, candidate, "browser action test")
	if err := appendSignalHistory(targetID, "open candidate 1 and check the final total", detail); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "prepare this product in the cart, but do not pay")
	for _, want := range []string{
		"Continuing to cart-prep.",
		"Target product: Pulmuone bottled water 500ml 20 pack",
		"Reopened link: https://www.coupang.com/vp/products/111",
		"Check before adding to cart:",
		"Expected total after item price, shipping, coupons, and Wow discounts",
		"Stop before payment-method visibility confirmation",
		"No add-to-cart, payment, or purchase has been made yet.",
		"Execution status:",
		"Coupang browser-test screen needs Mac action permission.",
		"Action: open_url",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English cart prep missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(signalReplyVisibleText(reply)) {
		t.Fatalf("English cart prep should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"장바구니 준비", "대상 상품", "장바구니에 담기 전에", "구매 실행 승인", "Mac 작업 실행 권한", "작업: open_url", "purchase completed", "order completed"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("English cart prep should not contain %q:\n%s", bad, reply)
		}
	}
}

func legacyTestAssistantReplyShoppingCandidateCartPrepCanSelectNumberedCandidate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-shopping-cart-numbered"
	rememberPendingShoppingDirectPurchase(ListenOptions{TargetID: targetID, Mode: "assistant"}, "생수 구매해", "생수")
	previous := coupangShoppingPrepReply("실행 상태 테스트", "생수 500ml 20", browserauto.Link{
		Text: "풀무원샘물 무라벨 생수",
		URL:  "https://www.coupang.com/vp/products/111",
	}, browserauto.Link{
		Text: "아이시스 500ml 20개",
		URL:  "https://www.coupang.com/vp/products/456",
	})
	if err := appendSignalHistory(targetID, "쿠팡에서 생수 500ml 20개 로켓배송으로 가격, 리뷰 좋은 후보 3개 비교해줘", previous); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "2번 상품을 장바구니까지 준비해. 결제는 하지 마")
	for _, want := range []string{"장바구니 준비 단계", "대상 상품: 아이시스 500ml 20개", "https://www.coupang.com/vp/products/456", "구매 대기 상품도 이 후보로 맞췄습니다."} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "풀무원샘물") {
		t.Fatalf("numbered cart prep should select the second candidate:\n%s", reply)
	}
	pending, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false)
	if !ok || pending.Kind != "shopping_direct_purchase" || pending.Intent.Query != "아이시스 500ml 20개" || pending.Intent.URL != "https://www.coupang.com/vp/products/456" {
		t.Fatalf("numbered cart prep should align pending purchase to second candidate: %#v ok=%v", pending, ok)
	}
}

func legacyTestAssistantReplyShoppingCandidateCartAddUsesLatestDetail(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-shopping-cart-add"
	rememberPendingShoppingDirectPurchase(ListenOptions{TargetID: targetID, Mode: "assistant"}, "생수 구매해", "생수")
	candidate := browserauto.Link{
		Text: "풀무원샘물 무라벨 생수, 500ml, 20개",
		URL:  "https://www.coupang.com/vp/products/8288410923",
	}
	previous := coupangShoppingPrepReply("실행 상태 테스트", "생수 500ml 20", candidate)
	if err := appendSignalHistory(targetID, "쿠팡에서 생수 500ml 20개 로켓배송으로 가격, 리뷰 좋은 후보 3개 비교해줘", previous); err != nil {
		t.Fatal(err)
	}
	detail := formatShoppingCandidateDetailReply(0, candidate, "실행 상태 테스트")
	if err := appendSignalHistory(targetID, "첫 번째 상품 상세 열고 총액 확인해줘", detail); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "장바구니 담기 실행")
	for _, want := range []string{"장바구니 담기 실행 단계", "대상 상품: 풀무원샘물 무라벨 생수", "https://www.coupang.com/vp/products/8288410923", "상품 페이지를 다시 열고", "장바구니에 담겼다는 알림", "최종 화면의 상품명", "구매 실행 승인", "구매 대기 상품도 이 후보로 맞췄습니다."} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"최근 쇼핑 상품을 찾지 못했습니다", "상품 링크나 정확한 상품명을 보내주세요", "1번 뉴스", "구매 완료", "주문 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("cart add reply should not contain %q:\n%s", bad, reply)
		}
	}
	pending, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false)
	if !ok || pending.Kind != "shopping_direct_purchase" || pending.Intent.Query != "풀무원샘물 무라벨 생수, 500ml, 20개" || pending.Intent.URL != "https://www.coupang.com/vp/products/8288410923" {
		t.Fatalf("cart add should align pending purchase to recent detail: %#v ok=%v", pending, ok)
	}
}

func TestAssistantReplyShoppingCandidateCartAddExecuteClicksCTA(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_DASHBOARD_URL", "https://argos.example/argos/dashboard.html")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-shopping-cart-add-execute"
	candidate := browserauto.Link{
		Text: "풀무원샘물 무라벨 생수, 500ml, 20개",
		URL:  "https://www.coupang.com/vp/products/8288410923",
	}
	previous := coupangShoppingPrepReply("실행 상태 테스트", "생수 500ml 20", candidate)
	if err := appendSignalHistory(targetID, "쿠팡에서 생수 500ml 20개 로켓배송으로 가격, 리뷰 좋은 후보 3개 비교해줘", previous); err != nil {
		t.Fatal(err)
	}
	detail := formatShoppingCandidateDetailReply(0, candidate, "실행 상태 테스트")
	if err := appendSignalHistory(targetID, "첫 번째 상품 상세 열고 총액 확인해줘", detail); err != nil {
		t.Fatal(err)
	}

	oldOpenURL := shoppingCartAddOpenURL
	oldCapture := shoppingCheckoutScreenCapture
	oldOCR := shoppingCheckoutScreenOCR
	oldClick := shoppingMouseClick
	defer func() {
		shoppingCartAddOpenURL = oldOpenURL
		shoppingCheckoutScreenCapture = oldCapture
		shoppingCheckoutScreenOCR = oldOCR
		shoppingMouseClick = oldClick
	}()
	openedURL := ""
	shoppingCartAddOpenURL = func(ctx context.Context, rawURL string) osauto.Result {
		openedURL = rawURL
		return osauto.Result{Kind: "meshclaw_automation_open_url", Action: "open_url", URL: rawURL, OK: true}
	}
	captureCalls := 0
	shoppingCheckoutScreenCapture = func(ctx context.Context, output string) osauto.Result {
		captureCalls++
		if err := os.WriteFile(output, []byte("fake cart add screen"), 0600); err != nil {
			t.Fatal(err)
		}
		return osauto.Result{Kind: "meshclaw_automation_screen_capture", Action: "screen_capture", OK: true}
	}
	ocrCalls := 0
	shoppingCheckoutScreenOCR = func(path string) (string, string) {
		ocrCalls++
		if ocrCalls == 1 {
			return strings.Join([]string{
				"쿠팡 상품 상세",
				"풀무원샘물 무라벨 생수, 500ml, 20개",
				"로켓배송",
				"내일 도착",
				"장바구니 담기",
				"바로구매",
			}, "\n"), ""
		}
		return strings.Join([]string{
			"장바구니에 상품이 담겼습니다",
			"장바구니 보기",
		}, "\n"), ""
	}
	clicks := 0
	var clickedX, clickedY float64
	shoppingMouseClick = func(ctx context.Context, runnerURL string, x, y float64) osauto.Result {
		clicks++
		clickedX, clickedY = x, y
		return osauto.Result{Kind: "meshclaw_automation_mouse_click", Action: "mouse_click", OK: true}
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant", Execute: true}, "장바구니 담기 실행")
	if openedURL != candidate.URL {
		t.Fatalf("cart add did not open product URL, got=%q reply=\n%s", openedURL, reply)
	}
	if clicks != 1 || clickedX <= 0 || clickedY <= 0 {
		t.Fatalf("expected one add-to-cart click with coordinates, clicks=%d x=%v y=%v reply=\n%s", clicks, clickedX, clickedY, reply)
	}
	if captureCalls < 2 || ocrCalls < 2 {
		t.Fatalf("expected capture and proof reads, captures=%d ocr=%d reply=\n%s", captureCalls, ocrCalls, reply)
	}
	for _, want := range []string{
		"장바구니 담기 실행 단계",
		"자동화 실행: `장바구니 담기` 버튼을 클릭했습니다.",
		"장바구니 담김 확인: 화면에서 장바구니 추가 결과를 읽었습니다.",
		"장바구니 실행 증거:",
		"Evidence 탭: https://argos.example/argos/dashboard.html?evidence=",
		"#evidence",
		"상품 페이지를 열었습니다: https://www.coupang.com/vp/products/8288410923",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("cart add execute reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"최종 구매 클릭을 실행했습니다", "구매 완료", "주문 완료", "결제하기 버튼을 클릭", home, "/.meshclaw/evidence/", ".json"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("cart add execute should not claim final purchase %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingCandidateCartAddEnglishDetailUsesLanguagePackWithoutEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SIGNAL_LOCAL_AUTO_GRANT", "0")
	targetID := "argos-assistant-shopping-cart-add-en"
	candidate := browserauto.Link{
		Text: "Pulmuone bottled water 500ml 20 pack",
		URL:  "https://www.coupang.com/vp/products/111",
	}
	detail := formatShoppingCandidateDetailReplyFor("en", 0, candidate, "browser action test")
	if err := appendSignalHistory(targetID, "open candidate 1 and check the final total", detail); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "add to cart")
	for _, want := range []string{
		"Cart add execution step.",
		"Target product: Pulmuone bottled water 500ml 20 pack",
		"Product link opened: https://www.coupang.com/vp/products/111",
		"Automation completed now: reopened the product page and moved to the add-to-cart confirmation step.",
		"Check on screen:",
		"If quantity/options are correct, the `Add to cart` button is ready to press.",
		"Payment/purchase buttons were not clicked.",
		"Execution status:",
		"Coupang browser-test screen needs Mac action permission.",
		"Action: open_url",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English cart add missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(signalReplyVisibleText(reply)) {
		t.Fatalf("English cart add should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"장바구니 담기", "대상 상품", "상품 페이지를 다시 열고", "구매 실행 승인", "Mac 작업 실행 권한", "작업: open_url", "purchase completed", "order completed"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("English cart add should not contain %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingCandidateCartAddNeedsRecentProduct(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-shopping-cart-add-empty", Mode: "assistant"}, "장바구니 담기 실행")
	if !strings.Contains(reply, "장바구니에 담을 최근 상품을 찾지 못했습니다") {
		t.Fatalf("reply should ask for recent product context:\n%s", reply)
	}
	if strings.Contains(reply, "1번 뉴스") {
		t.Fatalf("cart add request should not route to news:\n%s", reply)
	}
}

func TestAssistantReplyShoppingCandidateCartAddNeedsRecentProductEnglishWithoutEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-shopping-cart-add-empty-en", Mode: "assistant"}, "add to cart")
	if !strings.Contains(reply, "I could not find a recent product to add to cart.") {
		t.Fatalf("English add-to-cart no-context reply should ask for recent product context:\n%s", reply)
	}
	if assistantCheckoutPrepContainsHangul(signalReplyVisibleText(reply)) {
		t.Fatalf("English add-to-cart no-context reply should not expose Korean:\n%s", reply)
	}
}

func legacyTestAssistantReplyShoppingCandidateCartConfirmOpensCart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-shopping-cart-confirm"
	rememberPendingShoppingDirectPurchase(ListenOptions{TargetID: targetID, Mode: "assistant"}, "생수 구매해", "생수")
	candidate := browserauto.Link{
		Text: "풀무원샘물 무라벨 생수, 500ml, 20개",
		URL:  "https://www.coupang.com/vp/products/8288410923",
	}
	previous := coupangShoppingPrepReply("실행 상태 테스트", "생수 500ml 20", candidate)
	if err := appendSignalHistory(targetID, "쿠팡에서 생수 500ml 20개 로켓배송으로 가격, 리뷰 좋은 후보 3개 비교해줘", previous); err != nil {
		t.Fatal(err)
	}
	detail := formatShoppingCandidateDetailReply(0, candidate, "실행 상태 테스트")
	if err := appendSignalHistory(targetID, "첫 번째 상품 상세 열고 총액 확인해줘", detail); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "장바구니 확인 완료")
	for _, want := range []string{"장바구니 확인 단계", "최근 상품: 풀무원샘물 무라벨 생수", "https://cart.coupang.com/cartView.pang", "쿠팡 장바구니 페이지를 열었습니다", "상품명과 수량/옵션", "상품가, 배송비, 쿠폰", "최종 화면의 상품명", "구매 실행 승인", "구매 대기 상품도 이 후보로 맞췄습니다."} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"1번 뉴스", "구매 완료", "주문 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("cart confirm reply should not contain %q:\n%s", bad, reply)
		}
	}
	pending, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false)
	if !ok || pending.Kind != "shopping_direct_purchase" || pending.Intent.Query != "풀무원샘물 무라벨 생수, 500ml, 20개" || pending.Intent.URL != "https://www.coupang.com/vp/products/8288410923" {
		t.Fatalf("cart confirm should align pending purchase to recent detail: %#v ok=%v", pending, ok)
	}
}

func TestAssistantReplyShoppingCandidateCartConfirmEnglishDetailUsesLanguagePackWithoutEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SIGNAL_LOCAL_AUTO_GRANT", "0")
	targetID := "argos-assistant-shopping-cart-confirm-en"
	candidate := browserauto.Link{
		Text: "Pulmuone bottled water 500ml 20 pack",
		URL:  "https://www.coupang.com/vp/products/111",
	}
	detail := formatShoppingCandidateDetailReplyFor("en", 0, candidate, "browser action test")
	if err := appendSignalHistory(targetID, "open candidate 1 and check the final total", detail); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "cart confirmation complete")
	for _, want := range []string{
		"Cart confirmation step.",
		"Recent product: Pulmuone bottled water 500ml 20 pack",
		"Cart link: https://cart.coupang.com/cartView.pang",
		"Automation completed now: opened the Coupang cart page.",
		"Check in the cart:",
		"Product name, quantity, and options match the request",
		"Payment/purchase buttons were not clicked.",
		"Execution status:",
		"Coupang browser-test screen needs Mac action permission.",
		"Action: open_url",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English cart confirm missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(signalReplyVisibleText(reply)) {
		t.Fatalf("English cart confirm should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"장바구니 확인", "최근 상품", "상품명과 수량", "구매 실행 승인", "Mac 작업 실행 권한", "작업: open_url", "purchase completed", "order completed"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("English cart confirm should not contain %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingCandidateCartConfirmNeedsRecentProductEnglishWithoutEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-shopping-cart-confirm-empty-en", Mode: "assistant"}, "cart confirmation complete")
	if !strings.Contains(reply, "I could not find a recent cart product.") {
		t.Fatalf("English cart-confirm no-context reply should ask for recent product context:\n%s", reply)
	}
	if assistantCheckoutPrepContainsHangul(signalReplyVisibleText(reply)) {
		t.Fatalf("English cart-confirm no-context reply should not expose Korean:\n%s", reply)
	}
}

func TestAssistantReplyShoppingCandidateCartConfirmNeedsRecentProduct(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-shopping-cart-confirm-empty", Mode: "assistant"}, "장바구니 확인 완료")
	if !strings.Contains(reply, "최근 장바구니 상품을 찾지 못했습니다") {
		t.Fatalf("reply should ask for cart context:\n%s", reply)
	}
	if strings.Contains(reply, "1번 뉴스") {
		t.Fatalf("cart confirm request should not route to news:\n%s", reply)
	}
}

func legacyTestAssistantReplyShoppingCheckoutReviewUsesRecentCartProduct(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-shopping-checkout-review"
	rememberPendingShoppingDirectPurchase(ListenOptions{TargetID: targetID, Mode: "assistant"}, "생수 구매해", "생수")
	candidate := browserauto.Link{
		Text: "풀무원샘물 무라벨 생수, 500ml, 20개",
		URL:  "https://www.coupang.com/vp/products/8288410923",
	}
	previous := coupangShoppingPrepReply("실행 상태 테스트", "생수 500ml 20", candidate)
	if err := appendSignalHistory(targetID, "쿠팡에서 생수 500ml 20개 로켓배송으로 가격, 리뷰 좋은 후보 3개 비교해줘", previous); err != nil {
		t.Fatal(err)
	}
	detail := formatShoppingCandidateDetailReply(0, candidate, "실행 상태 테스트")
	if err := appendSignalHistory(targetID, "첫 번째 상품 상세 열고 총액 확인해줘", detail); err != nil {
		t.Fatal(err)
	}
	confirm := formatShoppingCandidateCartConfirmReply(candidate, coupangCartURL(), "실행 상태 테스트")
	if err := appendSignalHistory(targetID, "장바구니 확인 완료", confirm); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "최종 화면의 상품명, 옵션, 총액, 배송지, 결제수단을 읽어줘")
	for _, want := range []string{"쿠팡 최종 화면 확인 단계", "확인 대상 상품: 풀무원샘물 무라벨 생수", "https://cart.coupang.com/cartView.pang", "장바구니/결제 전 확인 화면을 열었습니다", "상품명과 옵션", "수량", "총액", "배송지", "결제수단", "최종 주문 버튼 위치", "구매 실행 승인", "복사해서 채울 구매 판정 템플릿", "상품명: 화면 그대로", "배송지 표시: 맞음/아님/2개 보임", "결제수단 표시: 맞음/아님/2개 보임", "구매 대기 상품도 이 후보로 맞췄습니다."} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "주문 완료", "1번 뉴스"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("checkout review reply should not contain %q:\n%s", bad, reply)
		}
	}
	pending, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false)
	if !ok || pending.Kind != "shopping_direct_purchase" || pending.Intent.Query != "풀무원샘물 무라벨 생수, 500ml, 20개" || pending.Intent.URL != "https://www.coupang.com/vp/products/8288410923" {
		t.Fatalf("checkout review should align pending purchase to recent cart product: %#v ok=%v", pending, ok)
	}
}

func TestAssistantReplyShoppingCheckoutReviewEnglishDetailUsesLanguagePackWithoutEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SIGNAL_LOCAL_AUTO_GRANT", "0")
	targetID := "argos-assistant-shopping-checkout-review-en"
	candidate := browserauto.Link{
		Text: "Pulmuone bottled water 500ml 20 pack",
		URL:  "https://www.coupang.com/vp/products/111",
	}
	detail := formatShoppingCandidateDetailReplyFor("en", 0, candidate, "browser action test")
	if err := appendSignalHistory(targetID, "open candidate 1 and check the final total", detail); err != nil {
		t.Fatal(err)
	}
	confirm := formatShoppingCandidateCartConfirmReplyFor("en", candidate, coupangCartURL(), "browser action test")
	if err := appendSignalHistory(targetID, "cart confirmation complete", confirm); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "read the final product, option, total, address visibility, and payment-method visibility")
	for _, want := range []string{
		"Coupang final-screen review step.",
		"Product to review: Pulmuone bottled water 500ml 20 pack",
		"Review screen: https://cart.coupang.com/cartView.pang",
		"Automation completed now: opened the cart / pre-checkout review screen.",
		"Read and verify on screen:",
		"Product name and options",
		"Payment-method visibility",
		"Final order button location",
		"The purchase button was not clicked.",
		"Copy and fill this purchase-decision template:",
		"Execution status:",
		"Coupang browser-test screen needs Mac action permission.",
		"Action: open_url",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English checkout review follow-up missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(signalReplyVisibleText(reply)) {
		t.Fatalf("English checkout review follow-up should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"쿠팡 최종", "확인 대상 상품", "장바구니/결제 전", "구매 실행 승인", "Mac 작업 실행 권한", "작업: open_url", "purchase completed", "order completed"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("English checkout review follow-up should not contain %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingCheckoutDecisionReady(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-shopping-checkout-decision"
	candidate := browserauto.Link{
		Text: "풀무원샘물 무라벨 생수, 500ml, 20개",
		URL:  "https://www.coupang.com/vp/products/8288410923",
	}
	detail := formatShoppingCandidateDetailReply(0, candidate, "실행 상태 테스트")
	if err := appendSignalHistory(targetID, "첫 번째 상품 상세 열고 총액 확인해줘", detail); err != nil {
		t.Fatal(err)
	}
	input := strings.Join([]string{
		"최종 화면 값 구매 가능 판정해줘",
		"상품명: 풀무원샘물 무라벨 생수",
		"옵션: 500ml 20개",
		"수량: 1",
		"총액: 10,990원",
		"배송지 표시: 맞음",
		"결제수단 표시: 맞음",
		"도착 예정일: 내일 도착",
		"최종 주문 버튼 위치: 화면 하단 파란 버튼",
	}, "\n")

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, input)
	for _, want := range []string{"구매 가능 판정", "기준 상품: 풀무원샘물 무라벨 생수", "판정: 구매 실행 승인 전 단계까지 준비됨", "다음 행동: 화면값이 맞으면 `구매 실행 승인`을 보내세요", "아직 최종 주문 버튼은 누르지 않았습니다", "상품/옵션: 풀무원샘물 무라벨 생수 / 500ml 20개", "수량: 1", "총액: 10,990원", "배송지 표시: 확인됨", "결제수단 표시: 확인됨", "도착 예정일: 내일 도착", "최종 주문 버튼 위치: 화면 하단 파란 버튼", "다음 Signal 액션:", "최종 진행 문장: `구매 실행 승인`", "이 문장을 보내기 전에는 최종 주문 버튼을 누르지 않음"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	visible := signalReplyVisibleText(reply)
	if strings.Contains(visible, "meshclaw-attachment:") || strings.Contains(visible, "/Documents/Argos Vault/") {
		t.Fatalf("visible decision reply should not expose raw attachment paths:\n%s", visible)
	}
	attachments := signalReplyAttachments(reply)
	if !hasAttachmentExt(attachments, ".svg") {
		t.Fatalf("ready decision should attach mobile SVG card: %#v", attachments)
	}
	card := firstAttachmentExt(attachments, ".svg")
	cardData, err := os.ReadFile(card)
	if err != nil {
		t.Fatalf("read decision SVG: %v", err)
	}
	for _, want := range []string{"<svg", "구매 가능 판정", "판정: 구매 실행 승인 전 단계까지 준비됨", "총액", "10,990원", "최종 주문 버튼 위치", "구매 실행 승인"} {
		if !strings.Contains(string(cardData), want) {
			t.Fatalf("decision SVG missing %q:\n%s", want, string(cardData))
		}
	}
	for _, bad := range []string{"누락:", "중지 사유", "구매 완료", "주문 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("ready decision should not contain %q:\n%s", bad, reply)
		}
	}

	templateInput := strings.Join([]string{
		"구매 가능 판정해줘",
		"상품명: 풀무원샘물 무라벨 생수",
		"옵션/수량: 500ml 20개 / 1",
		"총액: 10,990원",
		"도착 예정일: 내일 도착",
		"배송지 표시: 맞음",
		"결제수단 표시: 맞음",
		"주문 버튼: 보임",
	}, "\n")
	templateReply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, templateInput)
	for _, want := range []string{"판정: 구매 실행 승인 전 단계까지 준비됨", "상품/옵션: 풀무원샘물 무라벨 생수 / 500ml 20개", "수량: 1", "총액: 10,990원", "배송지 표시: 확인됨", "결제수단 표시: 확인됨", "구매 실행 승인"} {
		if !strings.Contains(templateReply, want) {
			t.Fatalf("template-style decision reply missing %q:\n%s", want, templateReply)
		}
	}
}

func TestInferCheckoutScreenFieldsFromOCRPreparesReadyCoordinates(t *testing.T) {
	fields := inferCheckoutScreenFieldsFromOCRTextWithSize(strings.Join([]string{
		"주문서",
		"풀무원샘물 무라벨 생수, 500ml, 20개",
		"수량 1개",
		"총 결제금액 10,990원",
		"배송지",
		"결제수단 신용카드",
		"내일 도착",
		"결제하기",
	}, "\n"), browserauto.Link{Text: "풀무원샘물 무라벨 생수, 500ml, 20개"}, 1440, 900)
	if missing := checkoutDecisionMissingFields(fields); len(missing) != 0 {
		t.Fatalf("OCR inference should fill checkout fields, missing=%v fields=%#v", missing, fields)
	}
	if problems := checkoutDecisionProblems(fields); len(problems) != 0 {
		t.Fatalf("OCR inference should not create blockers: %#v fields=%#v", problems, fields)
	}
	if !strings.Contains(fields.Button, "x=1180, y=792") {
		t.Fatalf("OCR inference should estimate final button coordinates, got %q", fields.Button)
	}
	reply := formatShoppingCheckoutDecisionReply(browserauto.Link{Text: "풀무원샘물 무라벨 생수, 500ml, 20개"}, fields)
	for _, want := range []string{
		"판정: 구매 실행 승인 전 단계까지 준비됨",
		"최종 주문 버튼 위치: OCR final order button visible; x=1180, y=792",
		"최종 진행 문장: `구매 실행 승인`",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("OCR checkout decision missing %q:\n%s", want, reply)
		}
	}
}

func TestInferCheckoutScreenFieldsFromCoupangNativeOCR(t *testing.T) {
	fields := inferCheckoutScreenFieldsFromOCRTextWithSize(strings.Join([]string{
		"주문/결제",
		"주문상품",
		"탐사 클래식 천연펄프 3겹 롤화장지 30m, 30롤, 1팩",
		"1개",
		"받는사람",
		"배송지",
		"배송 요청사항",
		"로켓배송 내일 도착 보장",
		"결제수단",
		"쿠페이 머니",
		"총 주문금액 12,900원",
		"12,900원 결제하기",
	}, "\n"), browserauto.Link{}, 1440, 900)
	if missing := checkoutDecisionMissingFields(fields); len(missing) != 0 {
		t.Fatalf("native Coupang OCR inference should fill checkout fields, missing=%v fields=%#v", missing, fields)
	}
	if problems := checkoutDecisionProblems(fields); len(problems) != 0 {
		t.Fatalf("native Coupang OCR inference should not create blockers: %#v fields=%#v", problems, fields)
	}
	if !strings.Contains(fields.Product, "탐사 클래식") {
		t.Fatalf("expected product line, got %#v", fields)
	}
	if fields.Quantity != "1" {
		t.Fatalf("expected quantity 1, got %#v", fields)
	}
	if fields.Total != "12,900원" {
		t.Fatalf("expected total 12,900원, got %#v", fields)
	}
	if !strings.Contains(fields.Delivery, "내일 도착") {
		t.Fatalf("expected delivery line, got %#v", fields)
	}
	if fields.Address != "yes" || fields.Payment != "yes" {
		t.Fatalf("expected address/payment visibility only, got %#v", fields)
	}
	if !strings.Contains(fields.Button, "x=1180, y=792") {
		t.Fatalf("expected estimated final button coordinates, got %q", fields.Button)
	}
}

func TestCoupangPurchaseOCRFixturesCoverHappyAndBlockerPaths(t *testing.T) {
	tests := []struct {
		name        string
		fixture     string
		wantAction  string
		wantBlock   string
		wantDisplay string
		wantReady   bool
	}{
		{name: "checkout-ready", fixture: "internal/messenger/testdata/coupang_purchase/checkout_ready_ko.txt", wantReady: true},
		{name: "search-results", fixture: "internal/messenger/testdata/coupang_purchase/search_results_ko.txt", wantAction: "search_result"},
		{name: "product-detail", fixture: "internal/messenger/testdata/coupang_purchase/product_detail_ko.txt", wantAction: "product_cart_add"},
		{name: "cart-checkout", fixture: "internal/messenger/testdata/coupang_purchase/cart_checkout_ko.txt", wantAction: "cart_checkout"},
		{name: "identity-blocker", fixture: "internal/messenger/testdata/coupang_purchase/identity_blocker_ko.txt", wantBlock: directPurchaseBlockerIdentity, wantDisplay: "본인인증"},
		{name: "captcha-blocker", fixture: "internal/messenger/testdata/coupang_purchase/captcha_blocker_ko.txt", wantBlock: directPurchaseBlockerCaptcha, wantDisplay: "CAPTCHA/보안문자"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(repoRootForTest(t), tt.fixture))
			if err != nil {
				t.Fatal(err)
			}
			text := string(data)
			if tt.wantReady {
				fields := inferCheckoutScreenFieldsFromOCRTextWithSize(text, browserauto.Link{}, 1440, 900)
				if missing := checkoutDecisionMissingFields(fields); len(missing) != 0 {
					t.Fatalf("fixture should infer ready checkout fields, missing=%v fields=%#v", missing, fields)
				}
				if problems := checkoutDecisionProblems(fields); len(problems) != 0 {
					t.Fatalf("fixture should not infer checkout blockers: %#v fields=%#v", problems, fields)
				}
				if !strings.Contains(fields.Button, "x=1180, y=792") {
					t.Fatalf("fixture should infer final button coordinate, got %#v", fields)
				}
				return
			}
			action, ok := inferDirectPurchaseScreenAction(text, "")
			if !ok {
				t.Fatalf("fixture did not infer direct purchase action")
			}
			if tt.wantAction != "" && action.Kind != tt.wantAction {
				t.Fatalf("action kind=%q want=%q action=%#v", action.Kind, tt.wantAction, action)
			}
			if tt.wantBlock != "" && action.Blocker != tt.wantBlock {
				t.Fatalf("blocker=%q want=%q action=%#v", action.Blocker, tt.wantBlock, action)
			}
			if tt.wantDisplay != "" && directPurchaseBlockerDisplayFor("ko", action.Blocker) != tt.wantDisplay {
				t.Fatalf("display blocker=%q want=%q action=%#v", directPurchaseBlockerDisplayFor("ko", action.Blocker), tt.wantDisplay, action)
			}
		})
	}
}

func repoRootForTest(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find repo root from %s", dir)
		}
		dir = parent
	}
}

func TestPurchaseGoAheadExecuteAutoReadsCheckoutScreenAndClicks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	oldCapture := shoppingCheckoutScreenCapture
	oldOCR := shoppingCheckoutScreenOCR
	defer func() {
		shoppingCheckoutScreenCapture = oldCapture
		shoppingCheckoutScreenOCR = oldOCR
	}()
	shoppingCheckoutScreenCapture = func(ctx context.Context, output string) osauto.Result {
		if err := os.WriteFile(output, []byte("not an actual screenshot"), 0600); err != nil {
			t.Fatal(err)
		}
		return osauto.Result{Kind: "meshclaw_automation_screen_capture", Action: "screen_capture", OK: true}
	}
	shoppingCheckoutScreenOCR = func(path string) (string, string) {
		return strings.Join([]string{
			"주문서",
			"풀무원샘물 무라벨 생수, 500ml, 20개",
			"수량 1개",
			"총 결제금액 10,990원",
			"배송지",
			"결제수단 신용카드",
			"내일 도착",
			"결제하기",
		}, "\n"), ""
	}
	clicked := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/click" {
			t.Fatalf("unexpected runner path: %s", r.URL.Path)
		}
		clicked = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	t.Setenv("MESHCLAW_ARGOS_UI_RUNNER_URL", server.URL)

	reply := purchaseFinalGoAheadReplyForRequest(ListenOptions{TargetID: "argos-assistant-auto-final-click", Mode: "assistant", Execute: true}, "ko", true, "")
	if !clicked {
		t.Fatalf("auto-read go-ahead did not click runner:\n%s", reply)
	}
	for _, want := range []string{
		"최종 구매 클릭을 실행했습니다.",
		"확인된 최종 주문 버튼 좌표를 한 번 클릭했습니다.",
		"상품: 풀무원샘물 무라벨 생수, 500ml, 20개",
		"총액: 10,990원",
		"도착 예정: 내일 도착",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("auto-read go-ahead reply missing %q:\n%s", want, reply)
		}
	}
}

func TestAssistantReplyShoppingCheckoutDecisionReadyEnglishDetailUsesLanguagePackWithoutEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-shopping-checkout-decision-ready-en"
	candidate := browserauto.Link{
		Text: "Pulmuone bottled water 500ml 20 pack",
		URL:  "https://www.coupang.com/vp/products/111",
	}
	detail := formatShoppingCandidateDetailReplyFor("en", 0, candidate, "browser action test")
	if err := appendSignalHistory(targetID, "open candidate 1 and check the final total", detail); err != nil {
		t.Fatal(err)
	}
	input := strings.Join([]string{
		"purchase readiness decision",
		"Product: Pulmuone bottled water 500ml 20 pack",
		"Option: 500ml 20 pack",
		"Quantity: 1",
		"Total: KRW 10,990",
		"Address visible: yes",
		"Payment method visible: yes",
		"Arrival estimate: tomorrow",
		"Final order button location: blue button at bottom",
	}, "\n")

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, input)
	for _, want := range []string{
		"Purchase readiness decision.",
		"Reference product: Pulmuone bottled water 500ml 20 pack",
		"Decision: ready for the pre-final approval step",
		"Next action: if the screen values are correct, send `purchase execution approved`.",
		"Product/options: Pulmuone bottled water 500ml 20 pack / 500ml 20 pack",
		"Quantity: 1",
		"Total: KRW 10,990",
		"Address visibility: provided",
		"Payment-method visibility: provided",
		"Arrival estimate: tomorrow",
		"Final order button location: blue button at bottom",
		"Next Signal action:",
		"Final go-ahead phrase: `purchase execution approved`",
		"The purchase button has not been clicked yet.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English ready decision missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(signalReplyVisibleText(reply)) {
		t.Fatalf("English ready decision should not expose Korean:\n%s", reply)
	}
	attachments := signalReplyAttachments(reply)
	if !hasAttachmentExt(attachments, ".svg") {
		t.Fatalf("English ready decision should attach mobile SVG card: %#v", attachments)
	}
	card := firstAttachmentExt(attachments, ".svg")
	cardData, err := os.ReadFile(card)
	if err != nil {
		t.Fatalf("read English decision SVG: %v", err)
	}
	for _, want := range []string{"<svg", "Purchase readiness decision", "Decision: ready for the pre-final approval step", "Total", "KRW 10,990", "Final order button location", "purchase execution approved"} {
		if !strings.Contains(string(cardData), want) {
			t.Fatalf("English decision SVG missing %q:\n%s", want, string(cardData))
		}
	}
	if assistantCheckoutPrepContainsHangul(string(cardData)) {
		t.Fatalf("English decision SVG should not expose Korean:\n%s", string(cardData))
	}
	for _, bad := range []string{"구매 가능 판정", "구매 실행 승인", "최종 주문", "purchase completed", "order completed"} {
		if strings.Contains(strings.ToLower(reply), strings.ToLower(bad)) || strings.Contains(strings.ToLower(string(cardData)), strings.ToLower(bad)) {
			t.Fatalf("English ready decision should not contain %q:\nreply=%s\nsvg=%s", bad, reply, string(cardData))
		}
	}
}

func legacyTestAssistantReplyShoppingCheckoutDecisionCanSendToAssistantRoom(t *testing.T) {
	home := t.TempDir()
	signalArgs := filepath.Join(home, "signal-args.txt")
	fakeSignal := filepath.Join(home, "signal-cli")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + strconv.Quote(signalArgs) + "\nprintf 'decision-signal-456\\n'\n"
	if err := os.WriteFile(fakeSignal, []byte(script), 0700); err != nil {
		t.Fatalf("write fake signal-cli: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SIGNAL_CLI", fakeSignal)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, "targets.json"))
	if _, _, err := UpsertTarget(Target{ID: "argos-assistant", Channel: "signal", GroupID: "group-assistant", Label: "비서방", Mode: "assistant"}); err != nil {
		t.Fatalf("upsert assistant target: %v", err)
	}
	targetID := "argos-assistant-shopping-checkout-decision-send"
	candidate := browserauto.Link{
		Text: "풀무원샘물 무라벨 생수, 500ml, 20개",
		URL:  "https://www.coupang.com/vp/products/8288410923",
	}
	detail := formatShoppingCandidateDetailReply(0, candidate, "실행 상태 테스트")
	if err := appendSignalHistory(targetID, "첫 번째 상품 상세 열고 총액 확인해줘", detail); err != nil {
		t.Fatal(err)
	}
	input := strings.Join([]string{
		"최종 화면 값 구매 가능 판정해서 비서방에 보내줘",
		"상품명: 풀무원샘물 무라벨 생수",
		"옵션: 500ml 20개",
		"수량: 1",
		"총액: 10,990원",
		"배송지 표시: 맞음",
		"결제수단 표시: 맞음",
		"도착 예정일: 내일 도착",
		"최종 주문 버튼 위치: 화면 하단 파란 버튼",
	}, "\n")

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant", Execute: true}, input)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{"구매 가능 판정 결과를 Signal로 보냈습니다.", "대상: 비서방", "Signal ID: decision-signal-456"} {
		if !strings.Contains(visible, want) {
			t.Fatalf("send decision reply missing %q:\n%s", want, visible)
		}
	}
	if strings.Contains(visible, "meshclaw-attachment:") || strings.Contains(visible, "/Documents/Argos Vault/") || strings.Contains(visible, "구매 완료") {
		t.Fatalf("send decision visible reply should stay clean:\n%s", visible)
	}
	argsData, err := os.ReadFile(signalArgs)
	if err != nil {
		t.Fatalf("read fake signal args: %v", err)
	}
	args := string(argsData)
	for _, want := range []string{"send", "-g", "group-assistant", "--attachment", ".svg", "다음 Signal 액션:", "최종 진행 문장: `구매 실행 승인`"} {
		if !strings.Contains(args, want) {
			t.Fatalf("fake signal args missing %q:\n%s", want, args)
		}
	}
	for _, unwanted := range []string{"구매 완료", "주문 완료", ".html"} {
		if strings.Contains(args, unwanted) {
			t.Fatalf("fake signal args should not claim or attach %q:\n%s", unwanted, args)
		}
	}
}

func TestAssistantReplyShoppingCheckoutDecisionStopsOnMissingFields(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	input := strings.Join([]string{
		"구매 가능 판정해줘",
		"상품명: 다른 상품",
		"총액: 10,990원",
		"배송지 표시: 맞음",
	}, "\n")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-shopping-checkout-decision-missing", Mode: "assistant"}, input)
	for _, want := range []string{"구매 가능 판정", "판정: 중지", "다음 행동: 누락값을 채워 다시 보내세요", "구매/결제는 계속 중지합니다", "누락:", "수량", "결제수단 표시", "최종 주문 버튼 위치", "다시 판정", "다음 Signal 액션:", "수량: 화면 값 그대로", "결제수단 표시: 화면 값 그대로", "최종 주문 버튼 위치: 화면 값 그대로", "아직 구매 버튼은 누르지 않았습니다"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	attachments := signalReplyAttachments(reply)
	if !hasAttachmentExt(attachments, ".svg") {
		t.Fatalf("hold decision should attach mobile SVG card: %#v", attachments)
	}
	card := firstAttachmentExt(attachments, ".svg")
	cardData, err := os.ReadFile(card)
	if err != nil {
		t.Fatalf("read hold decision SVG: %v", err)
	}
	for _, want := range []string{"<svg", "구매 가능 판정", "판정: 중지", "MISS", "누락", "총액", "10,990원"} {
		if !strings.Contains(string(cardData), want) {
			t.Fatalf("hold decision SVG missing %q:\n%s", want, string(cardData))
		}
	}
	if strings.Contains(reply, "구매 실행 승인 전 단계까지 준비됨") || strings.Contains(reply, "구매 완료") {
		t.Fatalf("missing fields should not be ready:\n%s", reply)
	}

	placeholderInput := strings.Join([]string{
		"구매 가능 판정해줘",
		"상품명: 화면 그대로",
		"옵션/수량: 화면 그대로",
		"총액: 최종 결제 금액",
		"도착 예정일: 화면 그대로",
		"배송지 표시: 맞음/아님/2개 보임",
		"결제수단 표시: 맞음/아님/2개 보임",
		"주문 버튼: 보임/안 보임",
	}, "\n")
	placeholderReply := assistantReply(ListenOptions{TargetID: "argos-assistant-shopping-checkout-decision-placeholder", Mode: "assistant"}, placeholderInput)
	for _, want := range []string{"구매 가능 판정", "판정: 중지", "누락:", "상품명과 옵션", "수량", "총액", "배송지", "결제수단", "최종 주문 버튼 위치", "다음 Signal 액션:", "상품명과 옵션: 화면 값 그대로"} {
		if !strings.Contains(placeholderReply, want) {
			t.Fatalf("placeholder decision reply missing %q:\n%s", want, placeholderReply)
		}
	}
	if strings.Contains(placeholderReply, "구매 실행 승인 전 단계까지 준비됨") || strings.Contains(placeholderReply, "상품/옵션: 화면 그대로") || strings.Contains(placeholderReply, "총액: 최종 결제 금액") {
		t.Fatalf("placeholder values should not be treated as real values:\n%s", placeholderReply)
	}
}

func TestAssistantReplyShoppingCheckoutDecisionStopsOnNegativeScreenValues(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	input := strings.Join([]string{
		"구매 가능 판정해줘",
		"상품명: 풀무원샘물 무라벨 생수",
		"옵션: 500ml 20개",
		"수량: 1",
		"총액: 10,990원",
		"배송지 표시: 아님",
		"결제수단 표시: 아님",
		"도착 예정일: 내일 도착",
		"최종 주문 버튼 위치: 안 보임",
	}, "\n")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-shopping-checkout-decision-negative", Mode: "assistant"}, input)
	for _, want := range []string{
		"구매 가능 판정",
		"판정: 중지",
		"다음 행동: 아님/안 보임 항목을 수정한 뒤 다시 보내세요.",
		"배송지 표시: 아님",
		"결제수단 표시: 아님",
		"최종 주문 버튼 위치: 안 보임",
		"중지 사유: 배송지 표시가 맞음으로 확인되지 않았습니다.",
		"중지 사유: 결제수단 표시가 맞음으로 확인되지 않았습니다.",
		"중지 사유: 최종 주문 버튼이 보임으로 확인되지 않았습니다.",
		"아님/안 보임 항목을 화면에서 수정해 다시 보내면",
		"다음 Signal 액션:",
		"배송지 표시: 화면 값 그대로",
		"결제수단 표시: 화면 값 그대로",
		"최종 주문 버튼 위치: 화면 값 그대로",
		"아직 구매 버튼은 누르지 않았습니다.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("negative screen-value decision missing %q:\n%s", want, reply)
		}
	}
	attachments := signalReplyAttachments(reply)
	if !hasAttachmentExt(attachments, ".svg") {
		t.Fatalf("negative decision should attach mobile SVG card: %#v", attachments)
	}
	card := firstAttachmentExt(attachments, ".svg")
	cardData, err := os.ReadFile(card)
	if err != nil {
		t.Fatalf("read negative decision SVG: %v", err)
	}
	for _, want := range []string{"<svg", "구매 가능 판정", "판정: 중지", "MISS", "배송지 표시가 맞음으로 확인되지 않았습니다", "최종 주문 버튼"} {
		if !strings.Contains(string(cardData), want) {
			t.Fatalf("negative decision SVG missing %q:\n%s", want, string(cardData))
		}
	}
	for _, bad := range []string{"판정: 구매 실행 승인 전 단계까지 준비됨", "최종 진행 문장: `구매 실행 승인`", "구매 완료", "주문 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("negative values should not be ready or claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingCheckoutDecisionStopsOnNegativeScreenValuesEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	input := strings.Join([]string{
		"ready to buy?",
		"Product: Pulmuone bottled water 500ml 20-pack",
		"Option: 500ml 20-pack",
		"Quantity: 1",
		"Total: KRW 10,990",
		"Address visible: no",
		"Payment method visible: no",
		"Arrival estimate: tomorrow",
		"Final order button location: not visible",
	}, "\n")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-shopping-checkout-decision-negative-en", Mode: "assistant"}, input)
	for _, want := range []string{
		"Purchase readiness decision.",
		"Decision: stop.",
		"Next action: fix the no/not-visible items",
		"Address visibility: no",
		"Payment-method visibility: no",
		"Final order button location: not visible",
		"Stop reason: Address visibility was not confirmed as yes.",
		"Stop reason: Payment-method visibility was not confirmed as yes.",
		"Stop reason: Final order button was not confirmed as visible.",
		"Fix the no/not-visible items on screen",
		"Next Signal action:",
		"Address visibility: screen value exactly as shown",
		"Payment-method visibility: screen value exactly as shown",
		"Final order button location: screen value exactly as shown",
		"The purchase button has not been clicked yet.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English negative screen-value decision missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English negative screen-value decision should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"ready for the pre-final approval step", "final go-ahead phrase", "purchase completed", "order completed"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English negative values should not be ready or claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingCheckoutDecisionStopsOnUnclearDelivery(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	input := strings.Join([]string{
		"구매 가능 판정해줘",
		"상품명: 풀무원샘물 무라벨 생수",
		"옵션: 500ml 20개",
		"수량: 1",
		"총액: 10,990원",
		"배송지 표시: 맞음",
		"결제수단 표시: 맞음",
		"도착 예정일: 배송 옵션 여러 개 보임",
		"최종 주문 버튼 위치: 화면 하단 파란 버튼",
	}, "\n")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-shopping-checkout-decision-delivery", Mode: "assistant"}, input)
	for _, want := range []string{
		"구매 가능 판정",
		"판정: 중지",
		"도착 예정일: 배송 옵션 여러 개 보임",
		"중지 사유: 도착 예정일이 여러 개/변경/지연/불확실로 보이면 이번 주문에 쓸 배송 옵션과 도착 예정일을 하나로 확정해야 합니다.",
		"다음 Signal 액션:",
		"먼저 `쿠팡 배송 옵션 확인해`를 보내 이번 주문의 배송 옵션과 도착 예정일을 하나로 확정하세요.",
		"아직 구매 버튼은 누르지 않았습니다.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("unclear-delivery decision missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"도착 예정일: 화면 값 그대로", "구매 실행 승인 전 단계까지 준비됨", "최종 진행 문장: `구매 실행 승인`", "구매 완료", "주문 완료", "최종 주문 버튼을 눌렀습니다"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("unclear-delivery decision should not claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingCheckoutDecisionStopsOnUnclearDeliveryEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	input := strings.Join([]string{
		"purchase readiness decision",
		"Product: Pulmuone bottled water",
		"Option: 500ml 20-pack",
		"Quantity: 1",
		"Total: 10,990 KRW",
		"Address visible: yes",
		"Payment method visible: yes",
		"Arrival estimate: multiple options visible",
		"Final order button location: blue button at bottom",
	}, "\n")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-shopping-checkout-decision-delivery-en", Mode: "assistant"}, input)
	for _, want := range []string{
		"Purchase readiness decision",
		"Decision: stop",
		"Arrival estimate: multiple options visible",
		"Stop reason: When arrival estimates are multiple, changed, delayed, or uncertain",
		"Next Signal action:",
		"First send `check Coupang delivery options` to confirm exactly one delivery option and arrival estimate for this order.",
		"The purchase button has not been clicked yet.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English unclear-delivery decision missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English unclear-delivery decision should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"arrival estimate: screen value exactly as shown", "ready for the pre-final approval step", "final go-ahead phrase", "purchase completed", "order completed"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English unclear-delivery decision should not claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingCheckoutDecisionStopsOnMultipleAddresses(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	input := strings.Join([]string{
		"구매 가능 판정해줘",
		"상품명: 풀무원샘물 무라벨 생수",
		"옵션: 500ml 20개",
		"수량: 1",
		"총액: 10,990원",
		"배송지 표시: 2개 보임",
		"결제수단 표시: 맞음",
		"도착 예정일: 내일 도착",
		"최종 주문 버튼 위치: 화면 하단 파란 버튼",
	}, "\n")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-shopping-checkout-decision-addresses", Mode: "assistant"}, input)
	for _, want := range []string{
		"구매 가능 판정",
		"판정: 중지",
		"배송지 표시: 2개/여러 개라 하나로 확정 필요",
		"중지 사유: 배송지가 2개/여러 개 보이면 기본 배송지 또는 이번 주문에 쓸 배송지 하나를 먼저 확정해야 합니다.",
		"다음 Signal 액션:",
		"먼저 `쿠팡 배송지 2개 확인해`를 보내 배송지를 하나로 확정하세요.",
		"아직 구매 버튼은 누르지 않았습니다.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("multiple-address decision missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"배송지 표시: 화면 값 그대로", "구매 실행 승인 전 단계까지 준비됨", "구매 완료", "주문 완료", "최종 주문 버튼을 눌렀습니다", "서울시", "상세주소"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("multiple-address decision should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingCheckoutDecisionStopsOnMultipleAddressesEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	input := strings.Join([]string{
		"purchase readiness decision",
		"Product: Pulmuone bottled water",
		"Option: 500ml 20-pack",
		"Quantity: 1",
		"Total: 10,990 KRW",
		"Address visible: two shipping addresses",
		"Payment method visible: yes",
		"Arrival estimate: tomorrow",
		"Final order button location: blue button at bottom",
	}, "\n")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-shopping-checkout-decision-addresses-en", Mode: "assistant"}, input)
	for _, want := range []string{
		"Purchase readiness decision",
		"Decision: stop",
		"Address visibility: multiple addresses shown, choose exactly one first",
		"Stop reason: When multiple shipping addresses are visible, confirm exactly one default or intended address before continuing.",
		"Next Signal action:",
		"First send `check Coupang two shipping addresses` to confirm exactly one address.",
		"The purchase button has not been clicked yet.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English multiple-address decision missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English multiple-address decision should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"address visibility: screen value exactly as shown", "ready up to purchase-execution approval", "purchase completed", "order completed", "clicked the final"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English multiple-address decision should not claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingCheckoutDecisionStopsOnMultiplePaymentMethods(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	input := strings.Join([]string{
		"구매 가능 판정해줘",
		"상품명: 풀무원샘물 무라벨 생수",
		"옵션: 500ml 20개",
		"수량: 1",
		"총액: 10,990원",
		"배송지 표시: 맞음",
		"결제수단 표시: 2개 보임",
		"도착 예정일: 내일 도착",
		"최종 주문 버튼 위치: 화면 하단 파란 버튼",
	}, "\n")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-shopping-checkout-decision-payments", Mode: "assistant"}, input)
	for _, want := range []string{
		"구매 가능 판정",
		"판정: 중지",
		"결제수단 표시: 2개/여러 개라 하나로 확정 필요",
		"중지 사유: 결제수단이 2개/여러 개 보이면 이번 주문에 쓸 결제수단 하나를 먼저 확정해야 합니다.",
		"다음 Signal 액션:",
		"먼저 `쿠팡 결제수단 확인해`를 보내 결제수단을 하나로 확정하세요.",
		"아직 구매 버튼은 누르지 않았습니다.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("multiple-payment decision missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"결제수단 표시: 화면 값 그대로", "구매 실행 승인 전 단계까지 준비됨", "구매 완료", "주문 완료", "최종 주문 버튼을 눌렀습니다", "카드번호", "CVC"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("multiple-payment decision should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingCheckoutDecisionStopsOnMultiplePaymentMethodsEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	input := strings.Join([]string{
		"purchase readiness decision",
		"Product: Pulmuone bottled water",
		"Option: 500ml 20-pack",
		"Quantity: 1",
		"Total: 10,990 KRW",
		"Address visible: yes",
		"Payment method visible: two cards",
		"Arrival estimate: tomorrow",
		"Final order button location: blue button at bottom",
	}, "\n")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-shopping-checkout-decision-payments-en", Mode: "assistant"}, input)
	for _, want := range []string{
		"Purchase readiness decision",
		"Decision: stop",
		"Payment-method visibility: multiple methods shown, choose exactly one first",
		"Stop reason: When multiple payment methods are visible, confirm exactly one intended payment method before continuing.",
		"Next Signal action:",
		"First send `check Coupang payment method` to confirm exactly one payment method.",
		"The purchase button has not been clicked yet.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English multiple-payment decision missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English multiple-payment decision should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"payment-method visibility: screen value exactly as shown", "ready up to purchase-execution approval", "purchase completed", "order completed", "clicked the final", "card number", "cvc"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English multiple-payment decision should not claim/expose %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingCheckoutDecisionStopsOnCardAlias(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	input := strings.Join([]string{
		"구매 가능 판정해줘",
		"상품명: 풀무원샘물 무라벨 생수",
		"옵션: 500ml 20개",
		"수량: 1",
		"총액: 10,990원",
		"배송지 표시: 맞음",
		"카드 표시: 2개 보임",
		"도착 예정일: 내일 도착",
		"최종 주문 버튼 위치: 화면 하단 파란 버튼",
	}, "\n")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-shopping-checkout-decision-card-alias", Mode: "assistant"}, input)
	for _, want := range []string{
		"구매 가능 판정",
		"판정: 중지",
		"결제수단 표시: 2개/여러 개라 하나로 확정 필요",
		"먼저 `쿠팡 결제수단 확인해`를 보내 결제수단을 하나로 확정하세요.",
		"아직 구매 버튼은 누르지 않았습니다.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("card-alias checkout decision missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"결제수단 표시: 화면 값 그대로", "구매 실행 승인 전 단계까지 준비됨", "구매 완료", "주문 완료", "카드번호", "CVC"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("card-alias checkout decision should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingCheckoutDecisionStopsOnCardAliasEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-shopping-checkout-decision-card-alias-en"
	previous := coupangShoppingPrepReply("browser action test", "생수 500ml 20", browserauto.Link{
		Text: "생수 - 국산생수, 수입생수 | 쿠팡",
		URL:  "https://www.coupang.com/vp/products/123",
	})
	if err := appendSignalHistory(targetID, "쿠팡에서 생수 500ml 20개 찾아줘", previous); err != nil {
		t.Fatal(err)
	}
	input := strings.Join([]string{
		"purchase readiness decision",
		"Product: Pulmuone bottled water",
		"Option: 500ml 20-pack",
		"Quantity: 1",
		"Total: 10,990 KRW",
		"Address visible: yes",
		"Card visible: two cards",
		"Arrival estimate: tomorrow",
		"Final order button location: blue button at bottom",
	}, "\n")

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, input)
	for _, want := range []string{
		"Purchase readiness decision",
		"Reference product: Pulmuone bottled water",
		"Decision: stop",
		"Payment-method visibility: multiple methods shown, choose exactly one first",
		"First send `check Coupang payment method` to confirm exactly one payment method.",
		"The purchase button has not been clicked yet.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English card-alias checkout decision missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English card-alias checkout decision should not expose Korean:\n%s", reply)
	}
	for _, bad := range []string{"payment-method visibility: screen value exactly as shown", "ready up to purchase-execution approval", "purchase completed", "order completed", "card number", "cvc"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English card-alias checkout decision should not expose/claim %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingCheckoutDecisionStopsOnRawAddressPaymentValues(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	input := strings.Join([]string{
		"구매 가능 판정해줘",
		"상품명: 풀무원샘물 무라벨 생수",
		"옵션: 500ml 20개",
		"수량: 1",
		"총액: 10,990원",
		"배송지: 서울시 테스트구 테스트동",
		"결제수단: 쿠페이 머니",
		"도착 예정일: 내일 도착",
		"최종 주문 버튼 위치: 화면 하단 파란 버튼",
	}, "\n")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-shopping-checkout-decision-raw-visibility", Mode: "assistant"}, input)
	for _, want := range []string{
		"구매 가능 판정",
		"판정: 중지",
		"배송지 표시: 원문 대신 맞음/아님/2개 보임 필요",
		"결제수단 표시: 원문 대신 맞음/아님/2개 보임 필요",
		"배송지 원문은 Signal에 보내지 말고 `배송지 표시: 맞음/아님/2개 보임`으로 다시 보내야 합니다.",
		"결제수단 원문은 Signal에 보내지 말고 `결제수단 표시: 맞음/아님/2개 보임`으로 다시 보내야 합니다.",
		"배송지 표시: 화면 값 그대로",
		"결제수단 표시: 화면 값 그대로",
		"아직 구매 버튼은 누르지 않았습니다.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("raw visibility decision missing %q:\n%s", want, reply)
		}
	}
	visible := signalReplyVisibleText(reply)
	for _, raw := range []string{"서울시 테스트구", "쿠페이 머니"} {
		if strings.Contains(visible, raw) {
			t.Fatalf("raw address/payment value should not be echoed %q:\n%s", raw, visible)
		}
	}
	for _, bad := range []string{"판정: 구매 실행 승인 전 단계까지 준비됨", "최종 진행 문장: `구매 실행 승인`", "구매 완료", "주문 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("raw visibility values should not be ready or claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyShoppingCheckoutDecisionStopsOnRawAddressPaymentValuesEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	input := strings.Join([]string{
		"ready to buy?",
		"Product: Pulmuone bottled water 500ml 20-pack",
		"Option: 500ml 20-pack",
		"Quantity: 1",
		"Total: KRW 10,990",
		"Address: 123 Test Street, Seoul",
		"Payment method: Coupay Money",
		"Arrival estimate: tomorrow",
		"Final order button location: blue button at bottom",
	}, "\n")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-shopping-checkout-decision-raw-visibility-en", Mode: "assistant"}, input)
	for _, want := range []string{
		"Purchase readiness decision.",
		"Decision: stop.",
		"Address visibility: send yes/no/two shipping addresses instead of the raw address",
		"Payment-method visibility: send yes/no/two cards instead of the raw payment method",
		"Do not send the raw address to Signal; resend only `Address visible: yes/no/two shipping addresses`.",
		"Do not send the raw payment method to Signal; resend only `Payment method visible: yes/no/two cards`.",
		"Address visibility: screen value exactly as shown",
		"Payment-method visibility: screen value exactly as shown",
		"The purchase button has not been clicked yet.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English raw visibility decision missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English raw visibility decision should not expose Korean:\n%s", reply)
	}
	visible := signalReplyVisibleText(reply)
	for _, raw := range []string{"123 Test Street", "Coupay Money"} {
		if strings.Contains(visible, raw) {
			t.Fatalf("raw address/payment value should not be echoed %q:\n%s", raw, visible)
		}
	}
	for _, bad := range []string{"ready for the pre-final approval step", "final go-ahead phrase", "purchase completed", "order completed"} {
		if strings.Contains(strings.ToLower(reply), bad) {
			t.Fatalf("English raw visibility values should not be ready or claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantScenarioCoupangSearchCleansInstructionWords(t *testing.T) {
	query := coupangShoppingQuery("쿠팡에서 생수 500ml 20개 로켓배송으로 가격, 리뷰 좋은 후보 3개 비교해줘")
	if query != "생수 500ml 20" {
		t.Fatalf("query = %q", query)
	}
	url := coupangSearchURL(query)
	for _, bad := range []string{"%EC%A2%8B%EC%9D%80", "%ED%9B%84%EB%B3%B4", "%EB%B9%84%EA%B5%90"} {
		if strings.Contains(url, bad) {
			t.Fatalf("coupang URL still contains instruction word %q: %s", bad, url)
		}
	}

	query = coupangShoppingQuery("Search Coupang for bottled water 500ml 20 pack and compare three good Rocket-delivery candidates by price and reviews.")
	if query != "bottled water 500ml 20 pack" {
		t.Fatalf("English query = %q", query)
	}
	url = coupangSearchURL(query)
	for _, bad := range []string{"Search", "Coupang", "compare", "candidate", "review", "by+and+s"} {
		if strings.Contains(url, bad) {
			t.Fatalf("English Coupang URL still contains instruction word %q: %s", bad, url)
		}
	}

	query = coupangShoppingQuery("휴지 구매해")
	if query != "휴지" {
		t.Fatalf("direct purchase query = %q", query)
	}

	query = coupangShoppingQuery("햇반 10개를 주문해줘")
	if query != "햇반 10개" {
		t.Fatalf("direct purchase particle query = %q", query)
	}
	query = coupangShoppingQuery("휴지 사지게 해")
	if query != "휴지" {
		t.Fatalf("make-it-buy direct purchase query = %q", query)
	}
	url = coupangSearchURL(query)
	if strings.Contains(url, "%EB%A5%BC") {
		t.Fatalf("direct purchase URL should not retain object particle: %s", url)
	}

	query = coupangShoppingQuery("Buy this item on Coupang")
	if !isDirectPurchaseCurrentVisibleProductQuery(query) {
		t.Fatalf("current visible product query = %q", query)
	}
}

func TestAssistantReplyCoupangProductSearchOpensCoupangSearch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "쿠팡에서 러닝 벨트 3만원 이하 찾아줘")
	if !strings.Contains(reply, "open_url") || !strings.Contains(reply, "coupang.com/np/search") || strings.Contains(reply, "google.com/search") {
		t.Fatalf("assistant reply should open Coupang search, got:\n%s", reply)
	}
	for _, want := range []string{"쿠팡 후보 비교를 시작", "상위 3개 후보 비교표", "구매 실행 승인"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("assistant reply missing %q:\n%s", want, reply)
		}
	}
}

func TestAssistantReplyCoupangProductSearchSkipsBrowserWhenDisabled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SHOPPING_BROWSER_DISABLE", "1")
	t.Setenv("MESHCLAW_SHOPPING_BROWSER_DISABLE_IN_TESTS", "1")
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-shopping-browser-disabled", Mode: "assistant"}, "쿠팡에서 러닝 벨트 3만원 이하 찾아줘")
	for _, want := range []string{
		"쿠팡 후보 비교를 시작",
		"검색 링크: https://www.coupang.com/np/search",
		"쇼핑 브라우저 실행을 건너뛰었습니다.",
		"새 브라우저 탭을 열지 않았고",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("disabled shopping browser reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"Mac 작업 실행 권한", "작업: open_url", "쿠팡 브라우저 테스트 화면을 열었습니다.", "열린 링크:"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("disabled shopping browser reply should not contain %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyPurchaseAutomationDisabled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_ASSISTANT_PURCHASE_AUTOMATION_DISABLE", "1")
	t.Setenv("MESHCLAW_SHOPPING_BROWSER_DISABLE", "1")
	t.Setenv("MESHCLAW_SHOPPING_BROWSER_DISABLE_IN_TESTS", "1")
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-purchase-disabled", Mode: "assistant"}, "휴지 구매해")
	for _, want := range []string{
		"구매/결제 자동화는 지원하지 않습니다.",
		"쿠팡/쇼핑몰 열기",
		"장바구니, 체크아웃, 결제, 자동 주문을 진행하지 않습니다.",
		"새 브라우저 탭, 장바구니, 결제, 주문 작업은 실행하지 않았습니다.",
		"제품 조건·후기·가격 비교 기준 정리",
		"나중에 직접 확인할 리마인더",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("disabled purchase reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 승인 한 번만 받겠습니다", "최종 주문 클릭", "구매 실행 승인", "open_url", "coupang.com/np/search"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("disabled purchase reply should not expose automation %q:\n%s", bad, reply)
		}
	}
	if _, ok := takePendingAssistantConversation(assistantPendingKey("argos-assistant-purchase-disabled"), false); ok {
		t.Fatal("disabled purchase request should not leave pending purchase state")
	}
}

func TestAssistantToolSearchShoppingDisabledDoesNotRouteAutomation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_ASSISTANT_PURCHASE_AUTOMATION_DISABLE", "1")
	t.Setenv("MESHCLAW_SHOPPING_BROWSER_DISABLE", "1")
	t.Setenv("MESHCLAW_SHOPPING_BROWSER_DISABLE_IN_TESTS", "1")
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")

	reply := executeAssistantToolCall(ListenOptions{TargetID: "argos-assistant-tool-shopping-disabled", Mode: "assistant"}, "휴지 구매해", "search_shopping", `{"query":"휴지"}`)
	for _, want := range []string{
		"구매/결제 자동화는 지원하지 않습니다.",
		"새 브라우저 탭, 장바구니, 결제, 주문 작업은 실행하지 않았습니다.",
		"후보 장단점 요약",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("disabled tool-routed shopping reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 승인", "최종 주문 클릭", "google.com/search", "coupang.com/np/search", "작업: open_url"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("disabled tool-routed shopping reply should not expose %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantApprovalQueueHidesPurchaseAutomationWhenDisabled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ASSISTANT_PURCHASE_AUTOMATION_DISABLE", "1")
	targetID := "argos-assistant-approval-purchase-disabled"
	rememberPendingShoppingDirectPurchase(ListenOptions{TargetID: targetID, Mode: "assistant"}, "휴지 구매해", "휴지")
	ready := formatPurchaseFinalExecutionTemplateReply(purchaseExecutionFields{
		Merchant:     "쿠팡",
		Item:         "풀무원샘물 무라벨 생수 500ml 20개",
		Total:        "10,990원",
		URL:          "https://www.coupang.com/checkout/order",
		X:            456,
		Y:            789,
		Confirmation: "구매 실행 승인",
	})
	if err := appendSignalHistory(targetID, "최종 실행 템플릿", ready); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "승인 대기열 보여줘")
	for _, bad := range []string{
		"구매 자동 진행 재개",
		"최근 구매 자동 진행 대기 후보",
		"구매 최종 클릭",
		"`meshclaw_purchase_click`",
		"`구매 승인`",
		"`구매 실행 승인`",
	} {
		if strings.Contains(reply, bad) {
			t.Fatalf("disabled approval queue should hide purchase automation %q:\n%s", bad, reply)
		}
	}
	if !strings.Contains(reply, "Argos 승인 대기열") || !strings.Contains(reply, "읽기 전용") {
		t.Fatalf("approval queue should still render non-purchase card:\n%s", reply)
	}
}

func TestAssistantReplyDirectPurchaseStartsCoupangFlowNotGoogle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_SIGNAL_LOCAL_AUTO_GRANT", "0")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-direct-purchase", Mode: "assistant"}, "휴지 구매해")
	for _, want := range []string{
		"구매 승인 한 번만 받겠습니다.",
		"상품: 휴지",
		"`구매 승인`, `ㅇㅇ`, `ㄱㄱ`, 또는 `ㅇㅋ`",
		"최종 주문 클릭",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("direct purchase reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"google.com/search", "https://www.google.com/search?tbm=shop", "Mac 작업 실행 권한", "작업: open_url", "검색 후에는 리뷰/배송비/총액을 비교"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("direct purchase should not use generic Google fallback %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantToolSearchShoppingPurchaseIntentStartsDirectPurchaseNotGoogle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_SIGNAL_LOCAL_AUTO_GRANT", "0")
	targetID := "argos-assistant-tool-direct-purchase"
	opts := ListenOptions{TargetID: targetID, Mode: "assistant"}

	reply := executeAssistantToolCall(opts, "휴지 구매해", "search_shopping", `{"query":"휴지"}`)
	for _, want := range []string{
		"구매 승인 한 번만 받겠습니다.",
		"상품: 휴지",
		"`구매 승인`, `ㅇㅇ`, `ㄱㄱ`, 또는 `ㅇㅋ`",
		"최종 주문 클릭",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("tool-routed direct purchase reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"google.com/search", "https://www.google.com/search?tbm=shop", "Mac 작업 실행 권한", "작업: open_url"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("tool-routed direct purchase should not use generic Google fallback %q:\n%s", bad, reply)
		}
	}
	pending, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false)
	if !ok || pending.Kind != "shopping_direct_purchase" || pending.Intent.Query != "휴지" {
		t.Fatalf("tool-routed direct purchase did not store pending purchase: %#v ok=%v", pending, ok)
	}
}

func TestAssistantReplyDirectPurchasePendingAcceptsNaturalApproval(t *testing.T) {
	for _, approval := range []string{"지금 사줘", "구매해", "구매 승인", "사지게 해", "주문되게 해", "사"} {
		t.Run(approval, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("MESHCLAW_LANG", "ko")
			t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
			t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
			t.Setenv("MESHCLAW_SIGNAL_LOCAL_AUTO_GRANT", "0")
			opts := ListenOptions{TargetID: "argos-assistant-direct-purchase-" + strings.NewReplacer(" ", "-", "\t", "-").Replace(approval), Mode: "assistant"}

			start := assistantReply(opts, "휴지 구매해")
			if !strings.Contains(start, "구매 승인 한 번만 받겠습니다.") {
				t.Fatalf("direct purchase did not start pending flow:\n%s", start)
			}
			reply := assistantReply(opts, approval)
			for _, want := range []string{
				"`휴지` 구매 승인을 받았습니다.",
				"현재 실행 모드가 아니어서 클릭은 하지 않았습니다.",
				"Signal 실행 런타임에서는 이 승인으로 바로 진행합니다.",
			} {
				if !strings.Contains(reply, want) {
					t.Fatalf("approval %q reply missing %q:\n%s", approval, want, reply)
				}
			}
			for _, bad := range []string{"구매 승인 한 번만 받겠습니다.", "google.com/search", "https://www.google.com/search?tbm=shop"} {
				if strings.Contains(reply, bad) {
					t.Fatalf("approval %q should continue pending purchase, not restart/fallback %q:\n%s", approval, bad, reply)
				}
			}
		})
	}
}

func TestAssistantReplyPurchaseNaturalMakeItBuyWithReadyDecisionUsesRecentContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	targetID := "argos-assistant-purchase-natural-make-it-buy-ko"
	readyDecision := formatShoppingCheckoutDecisionReply(browserauto.Link{Text: "풀무원샘물 무라벨 생수, 500ml, 20개"}, checkoutScreenFields{
		Product:  "풀무원샘물 무라벨 생수",
		Option:   "500ml 20개",
		Quantity: "1",
		Total:    "10,990원",
		Delivery: "내일 도착",
		Address:  "맞음",
		Payment:  "맞음",
		Button:   "화면 하단 파란 버튼",
	})
	if err := appendSignalHistory(targetID, "구매 가능 판정해줘", readyDecision); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "사지게 해")
	for _, want := range []string{
		"최종 구매 진행 요청을 확인했습니다.",
		"최종 클릭 실행 직전 단계",
		"최근 구매 가능 판정: 준비됨",
		"`meshclaw_purchase_click`",
		"Signal에 이어 보낼 최종 실행 템플릿:",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("make-it-buy go-ahead missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "주문 완료", "최종 버튼을 눌렀습니다"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("make-it-buy go-ahead should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func legacyTestAssistantReplyDirectPurchaseExecuteBlockedPersistsForShortRetry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_PENDING_ASSISTANT_CONVERSATIONS", filepath.Join(home, "pending-assistant-conversations.json"))
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_COUPANG_TAB_CLEANUP", "0")
	targetID := "argos-assistant-direct-purchase-execute-blocked-persist"

	oldCapture := shoppingCheckoutScreenCapture
	oldOpenURL := shoppingDirectPurchaseOpenURL
	defer func() {
		shoppingCheckoutScreenCapture = oldCapture
		shoppingDirectPurchaseOpenURL = oldOpenURL
		assistantConversationPending.Lock()
		delete(assistantConversationPending.items, assistantPendingKey(targetID))
		assistantConversationPending.Unlock()
	}()
	shoppingCheckoutScreenCapture = func(ctx context.Context, output string) osauto.Result {
		return osauto.Result{Kind: "meshclaw_automation_screen_capture", Action: "screen_capture", OK: false, Error: "screen unavailable"}
	}
	opened := 0
	shoppingDirectPurchaseOpenURL = func(ctx context.Context, rawURL string) osauto.Result {
		opened++
		return osauto.Result{Kind: "meshclaw_automation_open_url", Action: "open_url", URL: rawURL, OK: true}
	}

	opts := ListenOptions{TargetID: targetID, Mode: "assistant", Execute: true}
	first := assistantReply(opts, "휴지 구매해")
	if !strings.Contains(first, "`휴지` 구매 자동 진행이 멈췄습니다.") {
		t.Fatalf("first direct purchase should block and keep pending:\n%s", first)
	}
	if !strings.Contains(first, "승인 상태: 이 구매 건은 이미 한 번 승인되었습니다.") ||
		!strings.Contains(first, "`ㅇㅇ`/`다시 해봐`는 승인 질문을 반복하지 않고 같은 흐름을 이어갑니다.") {
		t.Fatalf("blocked direct purchase should explain one-approval resume state:\n%s", first)
	}
	if !strings.Contains(first, "다음 재시도: 새 검색을 열지 않고 현재 화면이나 선택한 상품 화면을 먼저 읽고") ||
		!strings.Contains(first, "필요하면 다음 구매 단계 버튼을 한 번만 누른 뒤 최종 화면을 다시 판정합니다.") {
		t.Fatalf("blocked direct purchase should explain concrete retry plan:\n%s", first)
	}
	if !strings.Contains(first, "진행 증거 ID:") {
		t.Fatalf("blocked direct purchase should show the direct evidence ID:\n%s", first)
	}
	firstEvidenceID := assistantRecentReportsRequestedEvidenceID(first)
	if firstEvidenceID == "" {
		t.Fatalf("blocked direct purchase should expose a short evidence ID:\n%s", first)
	}
	if !strings.Contains(first, "진행 증거 확인: `최근 작업 보여줘 "+firstEvidenceID+"` 또는 관제센터 Evidence에서 ID `"+firstEvidenceID+"`를 검색하세요.") {
		t.Fatalf("blocked direct purchase should show evidence check command:\n%s", first)
	}
	recent := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "최근 작업 보여줘 "+firstEvidenceID)
	for _, want := range []string{"필터: 증거 ID " + firstEvidenceID, "구매 자동 진행", "증거 ID: " + firstEvidenceID, "읽기 전용"} {
		if !strings.Contains(recent, want) {
			t.Fatalf("short evidence ID recent report missing %q:\n%s", want, recent)
		}
	}
	queue := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "승인 대기열 보여줘")
	if !strings.Contains(queue, "진행 증거 확인: `최근 작업 보여줘 "+firstEvidenceID+"` 또는 짧은 ID `"+firstEvidenceID+"`만 보내세요.") {
		t.Fatalf("approval queue should keep the last direct-purchase evidence ID:\n%s", queue)
	}
	if pending, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false); !ok || pending.Kind != "shopping_direct_purchase" || pending.Intent.Query != "휴지" {
		t.Fatalf("blocked direct purchase did not keep pending: %#v ok=%t", pending, ok)
	}

	assistantConversationPending.Lock()
	assistantConversationPending.items = map[string]pendingAssistantConversation{}
	assistantConversationPending.Unlock()

	retry := assistantReply(opts, "ㅇㅇ")
	if strings.Contains(retry, "이어서 할 일을 한 문장으로 보내세요") {
		t.Fatalf("short approval fell through to generic conversational reply:\n%s", retry)
	}
	if !strings.Contains(retry, "`휴지` 구매 자동 진행이 멈췄습니다.") {
		t.Fatalf("short approval did not restore persisted purchase pending:\n%s", retry)
	}
	if !strings.Contains(retry, "승인 상태: 이 구매 건은 이미 한 번 승인되었습니다.") {
		t.Fatalf("short retry should keep one-approval state visible:\n%s", retry)
	}
	if !strings.Contains(retry, "다음 재시도: 새 검색을 열지 않고 현재 화면이나 선택한 상품 화면을 먼저 읽고") {
		t.Fatalf("short retry should keep concrete retry plan visible:\n%s", retry)
	}
	retryEvidenceID := assistantRecentReportsRequestedEvidenceID(retry)
	if retryEvidenceID == "" || !strings.Contains(retry, "진행 증거 확인: `최근 작업 보여줘 "+retryEvidenceID+"`") {
		t.Fatalf("short retry should keep evidence check visible:\n%s", retry)
	}
	if opened != 1 {
		t.Fatalf("expected retry to reuse current screen without reopening Coupang search, opened=%d", opened)
	}
	if !strings.Contains(retry, "새 검색을 열지 않고 현재 화면에서 구매 진행을 이어갑니다.") {
		t.Fatalf("short retry should continue from current screen:\n%s", retry)
	}
}

func TestAssistantReplyPendingDirectPurchaseUsesSelectedCandidateURLOnRetry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_PENDING_ASSISTANT_CONVERSATIONS", filepath.Join(home, "pending-assistant-conversations.json"))
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_COUPANG_TAB_CLEANUP", "0")
	targetID := "argos-assistant-direct-purchase-selected-url-retry"
	selectedURL := "https://www.coupang.com/vp/products/456"

	oldCapture := shoppingCheckoutScreenCapture
	oldOpenURL := shoppingDirectPurchaseOpenURL
	oldCandidates := shoppingDirectPurchaseProductCandidates
	defer func() {
		shoppingCheckoutScreenCapture = oldCapture
		shoppingDirectPurchaseOpenURL = oldOpenURL
		shoppingDirectPurchaseProductCandidates = oldCandidates
		assistantConversationPending.Lock()
		delete(assistantConversationPending.items, assistantPendingKey(targetID))
		assistantConversationPending.Unlock()
	}()
	shoppingCheckoutScreenCapture = func(ctx context.Context, output string) osauto.Result {
		return osauto.Result{Kind: "meshclaw_automation_screen_capture", Action: "screen_capture", OK: false, Error: "screen unavailable"}
	}
	openedURL := ""
	shoppingDirectPurchaseOpenURL = func(ctx context.Context, rawURL string) osauto.Result {
		openedURL = rawURL
		return osauto.Result{Kind: "meshclaw_automation_open_url", Action: "open_url", URL: rawURL, OK: true}
	}
	shoppingDirectPurchaseProductCandidates = func(ctx context.Context, query string, limit int) []browserauto.Link {
		t.Fatalf("selected candidate URL should be used before live product search, query=%q limit=%d", query, limit)
		return nil
	}

	opts := ListenOptions{TargetID: targetID, Mode: "assistant", Execute: true}
	rememberPendingShoppingDirectPurchaseWithURLAndBlocker(opts, "생수 구매해\n2번으로 할게", "아이시스 500ml 20개", selectedURL, "")
	reply := assistantReply(opts, "ㅇㅇ")
	if openedURL != selectedURL {
		t.Fatalf("retry should open selected candidate URL, opened=%q want=%q\n%s", openedURL, selectedURL, reply)
	}
	if strings.Contains(openedURL, "/np/search") {
		t.Fatalf("retry should not fall back to search URL when selected URL exists, opened=%q", openedURL)
	}
	if !strings.Contains(reply, "`아이시스 500ml 20개` 구매 자동 진행이 멈췄습니다.") {
		t.Fatalf("retry should continue selected pending purchase:\n%s", reply)
	}
	if !strings.Contains(reply, "선택한 후보 상품 상세를 열었습니다: "+selectedURL) {
		t.Fatalf("retry should show that it opened the selected candidate URL:\n%s", reply)
	}
	pending, ok := takePendingAssistantConversation(assistantPendingKey(targetID), false)
	if !ok || pending.Kind != "shopping_direct_purchase" || pending.Intent.Query != "아이시스 500ml 20개" || pending.Intent.URL != selectedURL {
		t.Fatalf("blocked retry should preserve selected candidate URL: %#v ok=%t", pending, ok)
	}
}

func TestAssistantReplyDirectPurchaseExecuteClicksReadableCheckoutScreen(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	oldCapture := shoppingCheckoutScreenCapture
	oldOCR := shoppingCheckoutScreenOCR
	defer func() {
		shoppingCheckoutScreenCapture = oldCapture
		shoppingCheckoutScreenOCR = oldOCR
	}()
	shoppingCheckoutScreenCapture = func(ctx context.Context, output string) osauto.Result {
		if err := os.WriteFile(output, []byte("fake checkout proof"), 0600); err != nil {
			t.Fatal(err)
		}
		return osauto.Result{Kind: "meshclaw_automation_screen_capture", Action: "screen_capture", OK: true}
	}
	shoppingCheckoutScreenOCR = func(path string) (string, string) {
		return strings.Join([]string{
			"상품명: 휴지",
			"옵션/수량: 30롤 1개",
			"총 결제 금액 12,900원",
			"내일 도착",
			"배송지",
			"결제수단 카드",
			"결제하기",
			"주문번호: 123456789012",
		}, "\n"), ""
	}
	clicked := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/click" {
			t.Fatalf("unexpected runner path: %s", r.URL.Path)
		}
		clicked = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	t.Setenv("MESHCLAW_ARGOS_UI_RUNNER_URL", server.URL)

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-direct-purchase-execute", Mode: "assistant", Execute: true}, "휴지 구매해")
	if !clicked {
		t.Fatalf("direct purchase checkout screen did not click runner:\n%s", reply)
	}
	for _, want := range []string{
		"최종 구매 클릭을 실행했습니다.",
		"상품: 휴지 / 30롤 1개",
		"총액: 12,900원",
		"도착 예정: 내일 도착",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("direct purchase execute reply missing %q:\n%s", want, reply)
		}
	}
}

func TestAssistantReplyCoupangProductSearchEnglishInputUsesLanguagePackWithoutEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_SIGNAL_LOCAL_AUTO_GRANT", "0")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-coupang-en-search", Mode: "assistant"}, "Search Coupang for bottled water 500ml 20 pack and compare three good Rocket-delivery candidates by price and reviews.")
	for _, want := range []string{
		"Started Coupang candidate comparison.",
		"Query: bottled water 500ml 20 pack",
		"https://www.coupang.com/np/search?q=bottled+water+500ml+20+pack",
		"Search-result candidates:",
		"Candidate titles are not readable yet.",
		"Check next:",
		"You can continue with:",
		"The final order click is allowed only after",
		"Execution status:",
		"Coupang browser-test screen needs Mac action permission.",
		"Action: open_url",
		"Reply `run` to execute it once.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English Coupang search reply missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(signalReplyVisibleText(reply)) {
		t.Fatalf("English Coupang search reply should not expose Korean UI labels:\n%s", reply)
	}
	for _, bad := range []string{"쿠팡 후보", "검색어:", "열린 링크:", "바로 확인할 항목", "Mac 작업 실행 권한", "작업: open_url", "by and s", "purchase completed", "order completed"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("English Coupang search reply should not contain %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantReplyCoupangCandidateSearchDoesNotMisrouteAsFollowUp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-coupang-new-search", Mode: "assistant"}, "쿠팡에서 생수 500ml 20개 로켓배송으로 가격, 리뷰 좋은 후보 3개 비교해줘")
	for _, want := range []string{"쿠팡 후보 비교를 시작", "검색어: 생수 500ml 20", "coupang.com/np/search"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("assistant reply missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "최근 쇼핑 후보를 찾지 못했습니다") {
		t.Fatalf("new Coupang search was misrouted as follow-up:\n%s", reply)
	}
}

func TestAssistantScenarioCouponSearchGoesThroughBrowserPermission(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "나이키 러닝화 쿠폰 찾아줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "Mac 작업 실행 권한") ||
		(!strings.Contains(reply, "visible_browser_search") && !strings.Contains(reply, "browser_search")) {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantScenarioProductSearchRoutesWithoutShoppingKeyword(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "검정색 M 사이즈 러닝 벨트 3만원 이하 찾아줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "Mac 작업 실행 권한") ||
		(!strings.Contains(reply, "open_url") && !strings.Contains(reply, "browser_search")) {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantScenarioCoupangLoginOpensBrowserInsteadOfSearchReport(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	for _, input := range []string{
		"쿠팡 로그인 페이지 열어줘",
		"쿠팡 열어줘",
		"open coupang",
	} {
		reply, handled := shoppingScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, input)
		if !handled {
			t.Fatalf("scenario was not handled for input: %s", input)
		}
		if !strings.Contains(reply, "open_url") ||
			!strings.Contains(reply, "coupang.com") ||
			!strings.Contains(reply, "실전 화면 판정 규칙:") ||
			!strings.Contains(reply, "주소지 2개인 경우") ||
			strings.Contains(reply, "검색해서 리포트") {
			t.Fatalf("reply=%q", reply)
		}
	}
}

func TestAssistantReplyCoupangLoginOpensBrowserInsteadOfSearchReport(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	for _, input := range []string{
		"쿠팡 로그인 페이지 열어줘",
		"쿠팡 열어줘",
		"open coupang",
	} {
		reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, input)
		if !strings.Contains(reply, "open_url") ||
			!strings.Contains(reply, "coupang.com") ||
			!strings.Contains(reply, "실전 화면 판정 규칙:") ||
			!strings.Contains(reply, "주소지 2개인 경우") ||
			strings.Contains(reply, "검색해서 리포트") {
			t.Fatalf("reply=%q", reply)
		}
	}
}

func TestAssistantBroadScenarioFallbackIsOffByDefault(t *testing.T) {
	t.Setenv("MESHCLAW_ASSISTANT_TOOL_LOOP", "0")
	for _, input := range []string{
		"강남역에서 서울역까지 지금 얼마나 걸려?",
		"나이키 러닝화 쿠폰 찾아줘",
		"내일 저녁 7시에 강남 파스타 식당 2명 예약해줘",
		"유튜브에서 재즈 틀어줘",
		"성수동에서 오늘 저녁 뭐 먹지?",
	} {
		if reply, handled := assistantInteractiveReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, input); handled {
			t.Fatalf("%q should not be handled by broad deterministic fallback:\n%s", input, reply)
		}
	}
}

func TestAssistantLegacyScenarioFallbackCanBeEnabled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_ASSISTANT_LEGACY_SCENARIOS", "1")
	reply, handled := mapsScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "강남역에서 서울역까지 지금 얼마나 걸려?")
	if !handled || !strings.Contains(reply, "이동시간 확인 링크") {
		t.Fatalf("reply=%q handled=%t", reply, handled)
	}
}

func TestAssistantScenarioOTTSubscriptionOpensAccountGate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := mediaScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "넷플릭스 구독 관리 열어줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "Mac 작업 실행 권한") ||
		!strings.Contains(reply, "open_url") ||
		!strings.Contains(reply, "netflix.com/account") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantScenarioMealSuggestionReturnsMapLink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := lifePlanningScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "성수동에서 오늘 저녁 뭐 먹지?")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "식사 후보 지도 링크") ||
		!strings.Contains(reply, "https://www.google.com/maps/search/") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantScenarioPackingListDraft(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := lifePlanningScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "출장 짐 목록 만들어줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "짐 목록 초안") ||
		!strings.Contains(reply, "노트북") ||
		!strings.Contains(reply, "작업 기록도 저장했습니다.") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantScenarioPackingListCanSaveToNotes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := lifePlanningScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "출장 짐 목록 메모로 저장해줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "Mac 작업 실행 권한") ||
		!strings.Contains(reply, "note_create") ||
		!strings.Contains(reply, "출장 짐 목록") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantScenarioGroceryListCanSaveReminder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := lifePlanningScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "장보기 목록 리마인더로 저장해줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "Mac 작업 실행 권한") ||
		!strings.Contains(reply, "reminder_create") ||
		!strings.Contains(reply, "장보기") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantScenarioDailyPlanStartsWithCalendarPermission(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply, handled := lifePlanningScenarioReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "오늘 일정 보고 동선 짜줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "calendar_events_list") ||
		!strings.Contains(reply, "하루계획 1단계") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantScenarioHabitCheckGoesThroughReminderMutation(t *testing.T) {
	t.Skip("reminder_complete habit phrasing routes through model chat until osauto classifier is extended")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "오늘 운동 완료해줘")
	handled := strings.TrimSpace(reply) != ""
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "Mac 작업 실행 권한") ||
		!strings.Contains(reply, "reminder_complete") ||
		!strings.Contains(reply, "운동") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantScenarioReminderCreateGoesThroughArgosPermission(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "내일 오전 9시에 우유 사기 리마인더 추가해줘")
	handled := strings.TrimSpace(reply) != ""
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "Mac 작업 실행 권한") ||
		!strings.Contains(reply, "reminder_create") ||
		!strings.Contains(reply, "항상 허용") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantScenarioScheduledDeliveryDoesNotBecomeReminderOrCalendar(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	if _, _, err := UpsertTarget(Target{
		ID:        "yun",
		Channel:   "signal",
		Recipient: "+821012345678",
		Label:     "윤",
		Mode:      "assistant",
	}); err != nil {
		t.Fatal(err)
	}

	for _, text := range []string{
		"매주 월요일 9시에 윤에게 업무 브리핑 보내줘",
		"매주 월욜 오전 10시에 팀 회의 일정 보내줘",
	} {
		reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, text)
		visible := signalReplyVisibleText(reply)
		if strings.TrimSpace(visible) == "" {
			t.Fatalf("reply was empty for %q", text)
		}
		if !strings.Contains(visible, "예약 발송 계획을 만들었습니다.") {
			t.Fatalf("expected scheduled delivery plan reply for %q: %q", text, visible)
		}
		if strings.Contains(visible, "reminder_create") || strings.Contains(visible, "calendar_create_event") || strings.Contains(visible, "calendar_create") {
			t.Fatalf("expected scheduled-delivery path, got possible legacy scheduling path for %q: %q", text, visible)
		}
		if strings.Contains(visible, "윤") && !strings.Contains(visible, "대상: 윤") {
			t.Fatalf("expected resolved target preview for %q: %q", text, visible)
		}
	}
}

func TestAssistantScenarioNewsDoesNotBecomeReminder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	reply, handled := assistantInteractiveReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "오늘 주요뉴스 3개만 알려줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if strings.Contains(reply, "reminder_create") || strings.Contains(reply, "Mac 작업 실행 권한") {
		t.Fatalf("news request was routed as reminder: %q", reply)
	}
}

func TestAssistantScenarioChemicalMarketResearchBeatsFastNews(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_ASSISTANT_RESEARCH_DISABLE", "1")

	reply, handled := assistantInteractiveReply(
		ListenOptions{TargetID: "argos-assistant-chemical-market-news", Mode: "assistant"},
		"최근 화학회사들이 민감하게 볼 뉴스를 시장조사로 분석해서 회의 토픽으로 정리해줘",
	)
	if !handled {
		t.Fatal("scenario was not handled")
	}
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{"화학회사 민감 뉴스 분석 보고서", "내일 회의 핵심 토픽", "PFAS", "회의에서 바로 던질 질문"} {
		if !strings.Contains(visible, want) {
			t.Fatalf("chemical market research reply missing %q:\n%s", want, visible)
		}
	}
	for _, bad := range []string{"오늘 주요뉴스입니다.", "코스피", "구글뉴스 한국 톱"} {
		if strings.Contains(visible, bad) {
			t.Fatalf("chemical market research was routed to fast news (%q):\n%s", bad, visible)
		}
	}
}

func TestAssistantScenarioMarketResearchMeetingTopicsBeatsFastNews(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_ASSISTANT_RESEARCH_DISABLE", "1")

	reply, handled := assistantInteractiveReply(
		ListenOptions{TargetID: "argos-assistant-market-topics", Mode: "assistant"},
		"최근 반도체 장비 뉴스들을 시장조사로 분석해서 회의 토픽으로 정리해줘",
	)
	if !handled {
		t.Fatal("scenario was not handled")
	}
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{"시장조사 회의 토픽 브리프", "반도체/장비 시장", "회의 토픽", "회의에서 바로 던질 질문", "Markdown/DOCX/PPTX"} {
		if !strings.Contains(visible, want) {
			t.Fatalf("market research meeting topics reply missing %q:\n%s", want, visible)
		}
	}
	for _, bad := range []string{"오늘 주요뉴스입니다.", "코스피", "구글뉴스 한국 톱", "/.meshclaw/evidence"} {
		if strings.Contains(visible, bad) {
			t.Fatalf("market research meeting topics was routed poorly (%q):\n%s", bad, visible)
		}
	}
}

func legacyTestAssistantReportRoomQuietModeApprovalAndHomeCard(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	policyPath := filepath.Join(home, "report-room-policy.json")
	t.Setenv("MESHCLAW_REPORT_ROOM_POLICY", policyPath)

	card := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "보고방 소음 줄여줘")
	if !strings.Contains(card, "보고방 조용 모드 승인") ||
		!strings.Contains(card, "뉴스/AI 브리핑은 하루 1-2회") {
		t.Fatalf("noise card missing quiet approval path:\n%s", card)
	}
	if _, err := os.Stat(policyPath); !os.IsNotExist(err) {
		t.Fatalf("noise card should not write policy before approval, stat err=%v", err)
	}

	applied := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "보고방 조용 모드 승인")
	if !strings.Contains(applied, "보고방 조용 모드를 저장했습니다.") ||
		!strings.Contains(applied, "경로와 증거 파일 원문은 기본 답변에서 숨김") {
		t.Fatalf("quiet approval reply missing saved policy:\n%s", applied)
	}
	policy, ok := readAssistantReportRoomQuietPolicy()
	if !ok || !policy.QuietMode || !policy.MailPriorityOnly || policy.NewsMaxPerDay != 2 {
		t.Fatalf("unexpected quiet policy: ok=%v policy=%#v", ok, policy)
	}

	homeCard := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "오늘 홈카드 보여줘")
	for _, want := range []string{"오늘 비서 홈카드입니다.", "메일:", "보고방: 조용 모드 적용 중", "시장조사"} {
		if !strings.Contains(homeCard, want) {
			t.Fatalf("home card missing %q:\n%s", want, homeCard)
		}
	}
	for _, bad := range []string{policyPath, "/.meshclaw/evidence"} {
		if strings.Contains(homeCard, bad) {
			t.Fatalf("home card exposed path %q:\n%s", bad, homeCard)
		}
	}
}

func TestAssistantScenarioPlainMailCheckRoutesToMailPriority(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MAIL_ACCOUNTS", filepath.Join(home, "missing-mail-accounts.json"))

	for _, input := range []string{
		"메일 확인해줘",
		"최근 메일 요약해줘 처리할 일 5개만 뽑아줘",
	} {
		reply, handled := assistantInteractiveReply(
			ListenOptions{TargetID: "argos-assistant-mail-check", Mode: "assistant"},
			input,
		)
		if !handled {
			t.Fatalf("plain mail check was not handled for %q", input)
		}
		visible := signalReplyVisibleText(reply)
		for _, want := range []string{"확인할 메일 계정을 찾지 못했거나 읽기에 실패했습니다"} {
			if !strings.Contains(visible, want) {
				t.Fatalf("plain mail check should route to mail priority for %q, missing %q:\n%s", input, want, visible)
			}
		}
		for _, bad := range []string{"Argos가 Mac 작업을 처리했습니다", "오늘 주요뉴스", "바로 도와드릴 일", "현재 승인된 Argos memory", "User Memory Approved"} {
			if strings.Contains(visible, bad) {
				t.Fatalf("plain mail check was misrouted for %q (%q):\n%s", input, bad, visible)
			}
		}
	}
}

func TestFormatSignalFastNewsIsCompactAndSourceBased(t *testing.T) {
	got := formatSignalFastNews(assistantbrief.Brief{
		Generated: time.Date(2026, 5, 25, 9, 30, 0, 0, time.Local),
		NewsLimit: 2,
		News: []newsbrief.Item{
			{
				FeedTitle:   "BBC World",
				Title:       "Major diplomatic talks continue",
				Description: "Officials said the talks would continue after a short recess and no agreement has been signed yet.",
			},
			{
				FeedTitle:   "NPR News",
				Title:       "Chemical tank issue prompts evacuation",
				Description: "Local authorities ordered evacuations while engineers inspect a damaged chemical tank.",
			},
		},
	}, 2)
	if !strings.Contains(got, "1. Major diplomatic talks continue (BBC World)") ||
		!strings.Contains(got, "2. Chemical tank issue prompts evacuation (NPR News)") ||
		!strings.Contains(got, "원문은 `출처`") {
		t.Fatalf("unexpected compact news:\n%s", got)
	}
	if strings.Contains(got, "원문 링크가 필요하면") {
		t.Fatalf("expected mobile compact footer, got:\n%s", got)
	}
	if strings.Contains(got, "\n...") {
		t.Fatalf("expected summaries to stay on one mobile line, got:\n%s", got)
	}
}

func TestFormatSignalFastNewsAppliesApprovedStyleMemory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	if _, err := assistantprofile.AddMemory(time.Now(), "", "user", "사용자는 결론부터 짧게 답하는 것을 선호한다", true); err != nil {
		t.Fatal(err)
	}

	got := formatSignalFastNews(assistantbrief.Brief{
		Generated: time.Date(2026, 5, 25, 9, 30, 0, 0, time.Local),
		NewsLimit: 1,
		News: []newsbrief.Item{
			{
				FeedTitle:   "BBC World",
				Title:       "Major diplomatic talks continue",
				Description: "Officials said the talks would continue after a short recess and no agreement has been signed yet.",
			},
		},
	}, 1)
	for _, want := range []string{
		"저장된 선호에 맞춰 결론부터 짧게 배치했습니다.",
		"개인화 반영: 사용자는 결론부터 짧게 답하는 것을 선호한다",
		"1. Major diplomatic talks continue (BBC World)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("personalized compact news missing %q:\n%s", want, got)
		}
	}
}

func TestFormatSignalNewsVoiceBriefingSoundsLikeBroadcast(t *testing.T) {
	got := formatSignalNewsVoiceBriefing(assistantbrief.Brief{
		Generated: time.Date(2026, 5, 25, 9, 30, 0, 0, time.Local),
		NewsLimit: 3,
		News: []newsbrief.Item{
			{
				FeedTitle:      "BBC World",
				Title:          "외교 협상 재개",
				Description:    "짧은 RSS 요약",
				ArticleExcerpt: "관계자들은 짧은 휴식 뒤 협상이 계속될 예정이며 아직 합의문에는 서명하지 않았다고 밝혔습니다.",
			},
			{
				FeedTitle:   "세계 주요뉴스(한국어)",
				Title:       "화학 탱크 손상으로 대피령",
				Description: "지역 당국은 기술자들이 손상된 화학 탱크를 점검하는 동안 주민 대피를 명령했습니다.",
			},
			{
				FeedTitle:   "NPR News",
				Title:       "전력망 보강 법안 논의",
				Description: "의회와 정부가 데이터센터 전력 수요 증가에 맞춰 지역 전력망 보강 방안을 논의했습니다.",
			},
		},
	}, 3)
	for _, want := range []string{
		"Argos 뉴스 브리핑입니다.",
		"09시 30분 기준으로 확인된 주요 흐름을 전해드립니다.",
		"오늘은 3가지 소식을 짧은 앵커 원고로 정리했습니다.",
		"먼저, 외교 협상 재개.",
		"아직 합의문에는 서명하지 않았다고 밝혔습니다.",
		"이어서, 화학 탱크 손상으로 대피령.",
		"마지막으로, 전력망 보강 법안 논의.",
		"BBC World 보도 기준입니다.",
		"NPR News 보도 기준입니다.",
		"지금까지 들어온 주요 뉴스 흐름은 여기까지입니다.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("voice briefing missing %q:\n%s", want, got)
		}
	}
	for _, bad := range []string{"`출처`", "1.", "2.", "- ", "세계 주요뉴스", "출처는"} {
		if strings.Contains(got, bad) {
			t.Fatalf("voice briefing should not sound like markdown/list text (%q):\n%s", bad, got)
		}
	}
}

func TestCleanVoiceNewsSourceDropsInternalFeedNames(t *testing.T) {
	for _, source := range []string{"세계 주요뉴스(한국어)", "기술 뉴스(한국어)", "일본 주요뉴스(한국어)"} {
		if got := cleanVoiceNewsSource(source); got != "" {
			t.Fatalf("internal feed source should be dropped: %q -> %q", source, got)
		}
	}
	if got := cleanVoiceNewsSource("BBC World"); got != "BBC World" {
		t.Fatalf("real source should be kept: %q", got)
	}
}

func TestGeneratedNewsVoiceScriptValidationRejectsFeedNames(t *testing.T) {
	bad := strings.Repeat("자연스러운 뉴스 문장입니다. ", 12) + "출처는 세계 주요뉴스한국어입니다."
	if usableSignalNewsVoiceScript(bad) {
		t.Fatal("script with internal feed name should be rejected")
	}
	listy := "Argos 뉴스 브리핑입니다.\n1. 첫 번째 뉴스입니다. " + strings.Repeat("자연스러운 뉴스 문장입니다. ", 12)
	if usableSignalNewsVoiceScript(listy) {
		t.Fatal("numbered/list-like script should be rejected")
	}
	good := strings.Repeat("오늘 주요 흐름은 인공지능과 국제 정세 이슈가 함께 움직이고 있다는 점입니다. ", 4)
	if !usableSignalNewsVoiceScript(good) {
		t.Fatal("natural script should be accepted")
	}
}

func TestSignalNewsVoiceFactsPreferFetchedArticleExcerpt(t *testing.T) {
	got := signalNewsVoiceFacts(assistantbrief.Brief{
		Generated: time.Date(2026, 5, 25, 9, 30, 0, 0, time.Local),
		News: []newsbrief.Item{
			{
				FeedTitle:      "BBC World",
				Title:          "Short RSS headline",
				Description:    "Short RSS summary",
				ArticleExcerpt: "Fetched article body explains the policy change, the affected data centers, and why local officials are concerned about power demand.",
			},
		},
	}, 1)
	if !strings.Contains(got, "article_excerpt=Fetched article body explains the policy change") {
		t.Fatalf("facts should include fetched article excerpt:\n%s", got)
	}
	if strings.Contains(got, "rss_summary=") {
		t.Fatalf("rss summary should not be used when article excerpt exists:\n%s", got)
	}
}

func TestTrimSignalNewsLineKeepsOneLine(t *testing.T) {
	got := trimSignalNewsLine("The deal under discussion would involve a 60-day ceasefire extension during which the Strait of Hormuz would be reopened, according to US media.", 80)
	if strings.Contains(got, "\n") {
		t.Fatalf("expected one line, got %q", got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected ellipsis, got %q", got)
	}
	if strings.Contains(got, "accordin...") {
		t.Fatalf("expected word-boundary trim, got %q", got)
	}
}

func TestAssistantScenarioCalendarListGoesThroughArgosPermission(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "내일 일정 뭐 있어?")
	handled := strings.TrimSpace(reply) != ""
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "Mac 작업 실행 권한") ||
		!strings.Contains(reply, "calendar_events_list") ||
		!strings.Contains(reply, "Calendar 일정 조회") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantScenarioNaturalCalendarListDoesNotBecomePendingCreate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "내일 내가 잡아둔 일정 뭐 있어? 방금 넣은 병원 전화도 보이는지 봐줘.")
	handled := strings.TrimSpace(reply) != ""
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if strings.Contains(reply, "일정에 필요한 정보") ||
		!strings.Contains(reply, "calendar_events_list") {
		t.Fatalf("reply=%q", reply)
	}
	if strings.Contains(reply, "get_calendar_events") || strings.Contains(reply, "지원하지 않는 도구") {
		t.Fatalf("calendar list should not leak legacy tool fallback: %q", reply)
	}
}

func TestAssistantConversationalVagueCalendarEvent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	target := "calendar-pending"
	reply, handled := assistantInteractiveReply(ListenOptions{TargetID: target, Mode: "assistant"}, "내일 저녁 약속 잡아줘")
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "일정에 필요한 정보") ||
		!strings.Contains(reply, "남은 항목: 시간") ||
		!strings.Contains(reply, "일정: 약속") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantConversationalCalendarFollowupCreatesEvent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	target := "calendar-followup"
	reply, handled := assistantInteractiveReply(ListenOptions{TargetID: target, Mode: "assistant"}, "내일 저녁 약속 잡아줘")
	if !handled || !strings.Contains(reply, "남은 항목: 시간") {
		t.Fatalf("initial reply=%q handled=%t", reply, handled)
	}
	reply, handled = assistantInteractiveReply(ListenOptions{TargetID: target, Mode: "assistant"}, "6시에 강남역 장소는 나중에")
	if !handled {
		t.Fatal("followup was not handled")
	}
	if !strings.Contains(reply, "calendar_event_create") ||
		!strings.Contains(reply, "강남역") ||
		!strings.Contains(reply, "18:00") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantScenarioContactsSearchGoesThroughArgosPermission(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "연락처에서 홍길동 전화번호 찾아줘")
	handled := strings.TrimSpace(reply) != ""
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "Mac 작업 실행 권한") ||
		!strings.Contains(reply, "contacts_search") ||
		!strings.Contains(reply, "Contacts 연락처 조회") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantScenarioVisibleBrowserSearchGoesThroughArgosPermission(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_ASSISTANT_TOOL_LOOP", "0")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "브라우저에서 가나 경제 뉴스 검색해줘")
	handled := strings.TrimSpace(reply) != ""
	if !handled {
		t.Fatal("scenario was not handled")
	}
	if !strings.Contains(reply, "실행") ||
		!(strings.Contains(reply, "브라우저") || strings.Contains(reply, "웹")) ||
		!(strings.Contains(reply, "Mac 작업 실행 권한") || strings.Contains(reply, "검색을 진행")) {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantScenarioDoesNotRunInOpsMode(t *testing.T) {
	reply, handled := assistantInteractiveReply(ListenOptions{TargetID: "argos-ops", Mode: "ops"}, "광화문 교보문고 지도 보여줘")
	if handled || reply != "" {
		t.Fatalf("reply=%q handled=%t", reply, handled)
	}
}

func TestAssistantCapabilitiesReplyListsRealToolPaths(t *testing.T) {
	if !isAssistantCapabilitiesRequest("어떤 도구들이 있어?") {
		t.Fatal("capabilities request was not detected")
	}
	reply := assistantCapabilitiesReply()
	for _, want := range []string{"제약회사 최신 뉴스", "DART/SEC", "매일 오전 8시", "최근 메일 요약해줘", "오늘 일정", "장기기억", "스킬", "승인 필요"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("missing %q in reply=%q", want, reply)
		}
	}
}

func TestAssistantSkillStatusReplySummarizesImplementedSkills(t *testing.T) {
	if !isAssistantSkillStatusRequest("스킬 현황 알려줘") {
		t.Fatal("skill status request was not detected")
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "구현된 기능 몇 개야?")
	for _, want := range []string{"Argos 스킬 현황", "전체", "구현", "보강 중", "준비 중", "Signal에서 바로 맡길 수 있는 묶음", "뉴스/검색", "메일", "확인 후 실행"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("missing %q in reply:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "바로 해볼 예시입니다.") {
		t.Fatalf("skill status should not fall back to quick examples:\n%s", reply)
	}
}

func TestAssistantSkillMarketReplyShowsCommandPalette(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	if _, err := assistantprofile.InstallSkill(time.Now().UTC(), "", "Research Report", "Collect current sources and return a cited report.", true); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-skill-market", Mode: "assistant"}, "스킬 마켓 보여줘")
	for _, want := range []string{
		"Argos 스킬 마켓입니다.",
		"상태별 다음 행동:",
		"활성: Research Report -> `스킬 테스트 research-report` 또는 바로 요청",
		"명령 팔레트:",
		"`research-report`: `스킬 설치 research-report` -> `스킬 저장해` -> `스킬 테스트 research-report`",
		"등록 스킬 바로가기:",
		"`research-report`: `스킬 상세 research-report` / `스킬 테스트 research-report` / `스킬 검토 research-report`",
		"수정: `스킬 수정 research-report: 새 동작 설명`",
		"비활성화: `스킬 비활성화 research-report` -> `스킬 비활성화 승인`",
		"마켓 카드는 읽기 전용입니다.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("skill market reply missing %q:\n%s", want, reply)
		}
	}
}

func TestAssistantSkillInstallDraftParsesColonSeparatedSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	reply := assistantReply(
		ListenOptions{TargetID: "argos-assistant-skill-install", Mode: "assistant"},
		"스킬 만들어 시장조사: 출처 있는 보고서를 만들고 발송 전 멈추기",
	)
	for _, want := range []string{
		"스킬 설치 초안입니다.",
		"이름: 시장조사",
		"내용: 출처 있는 보고서를 만들고 발송 전 멈추기",
		"지금은 파일을 쓰지 않았습니다.",
		"`스킬 저장해`",
		"저장 후 바로 확인:",
		"테스트 카드: `스킬 테스트",
		"결과 검토: `스킬 테스트 결과",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("skill install draft missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "이름: 시장조사:") {
		t.Fatalf("skill name should not include colon-separated summary:\n%s", reply)
	}
}

func TestAssistantSkillInstallDraftEnglishParsesColonSeparatedSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	reply := assistantReply(
		ListenOptions{TargetID: "argos-assistant-skill-install-en", Mode: "assistant"},
		"create skill Market Watch: collect cited competitor notes and stop before sending",
	)
	for _, want := range []string{
		"Skill install draft.",
		"Name: Market Watch",
		"Content: collect cited competitor notes and stop before sending",
		"No files were written yet.",
		"`save skill`",
		"After saving:",
		"Test card: `test skill market-watch`",
		"Result review: `skill test result market-watch:",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English skill install draft missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "Name: Market Watch:") {
		t.Fatalf("English skill name should not include colon-separated summary:\n%s", reply)
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English skill install draft should not expose Korean:\n%s", reply)
	}
}

func TestAssistantTodayBriefingReplyIsReadableAndReadOnly(t *testing.T) {
	t.Setenv("MESHCLAW_ASSISTANT_TODAY_BRIEFING_NO_FETCH", "1")
	for _, input := range []string{"오늘 뭐 챙겨야 해?", "오늘 브리핑 해줘", "아침 브리핑", "하루 시작 브리핑"} {
		if !isAssistantTodayBriefingRequest(input) {
			t.Fatalf("today briefing request was not detected: %q", input)
		}
		reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, input)
		for _, want := range []string{"오늘 챙길 것 요약", "날씨", "일정", "할 일", "메일", "뉴스", "읽기 전용"} {
			if !strings.Contains(reply, want) {
				t.Fatalf("missing %q in reply for %q:\n%s", want, input, reply)
			}
		}
		if strings.Contains(reply, "원하는 번호") || strings.Contains(reply, "작업을 처리했습니다") {
			t.Fatalf("today briefing should be direct and read-only for %q:\n%s", input, reply)
		}
	}
}

func TestAssistantTodayPlanStarterRoutesToDailyBriefing(t *testing.T) {
	t.Setenv("MESHCLAW_ASSISTANT_TODAY_BRIEFING_NO_FETCH", "1")
	input := "오늘 일정, 할 일, 메일, 날씨를 하루 계획으로 묶어줘"
	if !isAssistantTodayBriefingRequest(input) {
		t.Fatal("today plan starter was not detected")
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-today-plan", Mode: "assistant"}, input)
	for _, want := range []string{"오늘 하루 계획 요약", "일정, 할 일, 메일, 날씨", "날씨", "일정", "할 일", "메일", "뉴스", "읽기 전용"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("today plan reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"접근할 수 없습니다", "직접 접근할 수 없습니다", "말씀해주시겠어요", "죄송하지만"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("today plan should not fall back to model refusal %q:\n%s", bad, reply)
		}
	}
}

func TestDailyWeatherLocation(t *testing.T) {
	tests := map[string]string{
		"오늘 서울 날씨 알려줘":                "Seoul",
		"부산 비 와?":                     "Busan",
		"weather in Tokyo":            "Tokyo",
		"오늘 뭐 입고 나가":                  "Seoul",
		"what should I wear in Paris": "Paris",
	}
	for input, want := range tests {
		if got := dailyWeatherLocation(input); got != want {
			t.Fatalf("dailyWeatherLocation(%q)=%q want %q", input, got, want)
		}
	}
}

func TestDailyWeatherRequestIgnoresMetaComplaints(t *testing.T) {
	if !isDailyWeatherRequest("오늘 날씨 알려줘") {
		t.Fatal("direct weather request was not detected")
	}
	if isDailyWeatherRequest("오늘날씨는 모르네") {
		t.Fatal("weather meta complaint should not trigger weather route")
	}
}
