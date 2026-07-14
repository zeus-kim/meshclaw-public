package messenger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssistantCore10RegistryShape(t *testing.T) {
	commands := assistantCore10Commands()
	if len(commands) != 10 {
		t.Fatalf("Core-10 registry size=%d, want 10", len(commands))
	}
	seen := map[string]bool{}
	for _, cmd := range commands {
		if strings.TrimSpace(cmd.ID) == "" {
			t.Fatalf("Core-10 command has empty id: %#v", cmd)
		}
		if seen[cmd.ID] {
			t.Fatalf("duplicate Core-10 command id %q", cmd.ID)
		}
		seen[cmd.ID] = true
		if strings.TrimSpace(cmd.Category) == "" {
			t.Fatalf("Core-10 command %q has empty category", cmd.ID)
		}
		if len(cmd.Examples) == 0 {
			t.Fatalf("Core-10 command %q has no examples", cmd.ID)
		}
		if len(cmd.ExpectedVisibleMarkers) == 0 && len(cmd.ExpectedAnyMarkers) == 0 {
			t.Fatalf("Core-10 command %q has no visible reply markers", cmd.ID)
		}
	}
}

func TestAssistantCore10RegistryExercisesReplyPath(t *testing.T) {
	home := setupAssistantCore10TestEnv(t)
	if err := SaveScheduledDeliveryStore(ScheduledDeliveryStore{Jobs: []ScheduledDeliveryJob{{
		ID:          "delivery-1",
		Enabled:     true,
		Status:      "registered",
		TargetID:    "argos-briefing",
		TargetLabel: "보고방",
		Schedule:    "매일 오전 8시",
		Content:     "제약/바이오 최신 뉴스 브리프",
	}}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".meshclaw", "assistant-watches.json"), []byte(`{"watches":[]}`), 0600); err != nil {
		t.Fatal(err)
	}

	for _, cmd := range assistantCore10Commands() {
		cmd := cmd
		for i, input := range cmd.Examples {
			input := input
			t.Run(fmt.Sprintf("%s_%d", cmd.ID, i+1), func(t *testing.T) {
				targetID := fmt.Sprintf("argos-core10-%s-%d", cmd.ID, i+1)
				reply := guardReply(
					ListenOptions{TargetID: targetID, Mode: "assistant"},
					Target{ID: targetID, Channel: "signal", GroupID: "group.core10", Mode: "assistant"},
					IncomingMessage{Source: "+821055501010", GroupID: "group.core10", Redacted: input},
				)
				visible := signalReplyVisibleText(reply)
				assertAssistantCoreCommandReply(t, cmd, input, visible)
			})
		}
	}
}

func TestAssistantCore10CommercialShoppingBoundary(t *testing.T) {
	setupAssistantCore10TestEnv(t)
	input := "쿠팡에서 생수 하나 구매하고 결제까지 해줘"
	reply := guardReply(
		ListenOptions{TargetID: "argos-core10-shopping-boundary", Mode: "assistant"},
		Target{ID: "argos-core10-shopping-boundary", Channel: "signal", GroupID: "group.core10", Mode: "assistant"},
		IncomingMessage{Source: "+821055501010", GroupID: "group.core10", Redacted: input},
	)
	visible := signalReplyVisibleText(reply)
	if strings.TrimSpace(visible) == "" {
		t.Fatalf("empty shopping boundary reply for %q", input)
	}
	if !containsAny(visible, "구매/결제 자동화는 지원하지 않습니다.", "구매/결제 자동화는 제외", "구매 자동화는 제외", "결제 자동화는 제외", "범위 밖", "out of scope", "not in scope") {
		t.Fatalf("shopping boundary reply should state purchase/payment automation is out of scope:\n%s", visible)
	}
	for _, bad := range assistantCommercialForbiddenVisibleMarkers() {
		if strings.Contains(visible, bad) {
			t.Fatalf("shopping boundary reply exposed forbidden %q:\n%s", bad, visible)
		}
	}
	for _, bad := range []string{"구매 실행 승인", "결제 진행", "checkout", "purchase execution approved"} {
		if strings.Contains(strings.ToLower(visible), strings.ToLower(bad)) {
			t.Fatalf("shopping boundary reply should not continue purchase automation with %q:\n%s", bad, visible)
		}
	}
}

func TestAssistantCore10DailyPlanBuildsCommercialCard(t *testing.T) {
	setupAssistantCore10TestEnv(t)
	t.Setenv("MESHCLAW_ASSISTANT_DAILY_AGENDA_NO_FETCH", "")
	t.Setenv("MESHCLAW_ASSISTANT_TODAY_BRIEFING_NO_FETCH", "1")
	t.Setenv("MESHCLAW_ASSISTANT_DAILY_AGENDA_CALENDAR_STDOUT", `{"events":[{"title":"제품 전략 회의","start":"2026-06-22T09:30:00+09:00","end":"2026-06-22T10:30:00+09:00","calendar":"Work","location":"회의실 A"}]}`)
	t.Setenv("MESHCLAW_ASSISTANT_DAILY_AGENDA_REMINDER_STDOUT", `{"reminders":[{"title":"회의자료 최종 확인","due":"2026-06-22T13:00:00+09:00","calendar":"Reminders","notes":"PPT 확인"}]}`)
	t.Setenv("MESHCLAW_ASSISTANT_DAILY_AGENDA_MAIL_STDOUT", `{"results":[{"account":{"id":"work","backend":"imap","tls":true},"messages":[{"id":"m1","from":"partner@example.com","subject":"계약서 검토 후 회신 부탁드립니다","date":"2026-06-22T08:30:00+09:00","snippet":"오전 중 검토 후 회신 부탁드립니다."}]}]}`)

	input := "오늘 일정, 할 일, 메일, 날씨를 하루 계획으로 묶어줘"
	reply := guardReply(
		ListenOptions{TargetID: "argos-core10-daily-plan-card", Mode: "assistant"},
		Target{ID: "argos-core10-daily-plan-card", Channel: "signal", GroupID: "group.core10", Mode: "assistant"},
		IncomingMessage{Source: "+821055501010", GroupID: "group.core10", Redacted: input},
	)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"오늘 하루 계획 요약입니다.",
		"일정, 할 일, 메일, 날씨를 한 카드로 묶었습니다.",
		"날씨:",
		"제품 전략 회의",
		"회의자료 최종 확인",
		"계약서 검토 후 회신 부탁드립니다",
		"추천 순서:",
		"다음 행동: `가장 중요한 3개만 오전/오후로 다시 배치해줘`",
		"읽기 전용",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("daily plan commercial card missing %q:\n%s", want, visible)
		}
	}
	for _, bad := range append(assistantCommercialForbiddenVisibleMarkers(), "첨부 파일:", "진행 증거:") {
		if strings.Contains(visible, bad) {
			t.Fatalf("daily plan commercial card exposed forbidden %q:\n%s", bad, visible)
		}
	}
}

func TestAssistantCore10DailyPlanMailUnavailableFallbackIsUserFacing(t *testing.T) {
	setupAssistantCore10TestEnv(t)
	t.Setenv("MESHCLAW_ASSISTANT_DAILY_AGENDA_NO_FETCH", "")
	t.Setenv("MESHCLAW_ASSISTANT_TODAY_BRIEFING_NO_FETCH", "1")
	t.Setenv("MESHCLAW_ASSISTANT_DAILY_AGENDA_CALENDAR_STDOUT", `{"events":[]}`)
	t.Setenv("MESHCLAW_ASSISTANT_DAILY_AGENDA_REMINDER_STDOUT", `{"reminders":[]}`)
	t.Setenv("MESHCLAW_ASSISTANT_DAILY_AGENDA_MAIL_STDOUT", `{"errors":[{"account":{"id":"mail"},"error":"open /Users/example/.config/mail.json: permission denied"}]}`)

	input := "오늘 일정, 할 일, 메일, 날씨를 하루 계획으로 묶어줘"
	reply := guardReply(
		ListenOptions{TargetID: "argos-core10-daily-plan-fallback", Mode: "assistant"},
		Target{ID: "argos-core10-daily-plan-fallback", Channel: "signal", GroupID: "group.core10", Mode: "assistant"},
		IncomingMessage{Source: "+821055501010", GroupID: "group.core10", Redacted: input},
	)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"오늘 하루 계획 요약입니다.",
		"확인된 일정이 없습니다.",
		"확인된 할 일이 없습니다.",
		"확인할 메일 계정을 찾지 못했거나 읽기에 실패했습니다.",
		"`최근 메일 요약해줘`",
		"다음 행동:",
		"읽기 전용",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("daily plan fallback missing %q:\n%s", want, visible)
		}
	}
	for _, bad := range assistantCommercialForbiddenVisibleMarkers() {
		if strings.Contains(visible, bad) {
			t.Fatalf("daily plan fallback exposed forbidden %q:\n%s", bad, visible)
		}
	}
}

func TestAssistantCore10MailPriorityUnavailableFallbackIsUserFacing(t *testing.T) {
	setupAssistantCore10TestEnv(t)
	input := "최근 메일 요약해줘. 답장 필요한 것만 5개 뽑아줘"
	reply := guardReply(
		ListenOptions{TargetID: "argos-core10-mail-priority-fallback", Mode: "assistant"},
		Target{ID: "argos-core10-mail-priority-fallback", Channel: "signal", GroupID: "group.core10", Mode: "assistant"},
		IncomingMessage{Source: "+821055501010", GroupID: "group.core10", Redacted: input},
	)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"메일 처리 우선순위입니다.",
		"확인할 메일 계정을 찾지 못했거나 읽기에 실패했습니다.",
		"다음 행동:",
		"`최근 메일 요약해줘`",
		"메일 발송/삭제/이동/첨부 저장은 별도 승인 전에는 실행하지 않았습니다.",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("mail priority fallback missing %q:\n%s", want, visible)
		}
	}
	for _, bad := range append(assistantCommercialForbiddenVisibleMarkers(), "permission denied", "mail.json", "exit status") {
		if strings.Contains(visible, bad) {
			t.Fatalf("mail priority fallback exposed forbidden %q:\n%s", bad, visible)
		}
	}
}

func assistantCore10SignalRegressionCases() []assistantSignalRegressionCase {
	commands := assistantCore10Commands()
	cases := make([]assistantSignalRegressionCase, 0, len(commands))
	for _, cmd := range commands {
		for _, input := range cmd.Examples {
			cases = append(cases, assistantSignalRegressionCase{
				category: "core10_" + cmd.Category,
				input:    input,
				wantAny:  append([]string{}, cmd.ExpectedAnyMarkers...),
				wantAll:  append([]string{}, cmd.ExpectedVisibleMarkers...),
				mustNot:  append([]string{}, cmd.ForbiddenMarkers...),
			})
		}
	}
	return cases
}

func setupAssistantCore10TestEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_SCHEDULED_DELIVERIES", filepath.Join(home, ".meshclaw", "scheduled-deliveries.json"))
	t.Setenv("MESHCLAW_ASSISTANT_WATCHES", filepath.Join(home, ".meshclaw", "assistant-watches.json"))
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_ASSISTANT_TOOL_LOOP", "0")
	t.Setenv("MESHCLAW_ASSISTANT_MEMORY_INTENT_MODEL", "0")
	t.Setenv("MESHCLAW_ASSISTANT_RESEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_ASSISTANT_DAILY_AGENDA_NO_FETCH", "1")
	t.Setenv("MESHCLAW_BOOKING_SEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_SERVER_DISABLED", "1")
	t.Setenv("MESHCLAW_SHOPPING_BROWSER_DISABLE", "1")
	t.Setenv("MESHCLAW_SHOPPING_BROWSER_DISABLE_IN_TESTS", "1")
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_ASSISTANT_PURCHASE_AUTOMATION_DISABLE", "1")
	t.Setenv("MESHCLAW_SCHEDULE_STATUS_JSON", `{"status":"healthy","due_count":0,"jobs":[{"id":"mail-watch","kind":"mail_watch","mode":"assistant","target_id":"argos-briefing","when":"15m","enabled":true,"due":false,"last_run":"2026-06-12T14:00:00Z","next_due":"2026-06-12T14:15:00Z"},{"id":"assistant-auto-check","kind":"assistant_auto_check","mode":"assistant","target_id":"argos-assistant","when":"3h","enabled":true,"due":false,"last_run":"2026-06-12T14:03:00Z","next_due":"2026-06-12T17:03:00Z"}]}`)
	if err := os.MkdirAll(filepath.Join(home, ".meshclaw"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := UpsertTarget(Target{
		ID:      "argos-briefing",
		Channel: "signal",
		GroupID: "group.argos-briefing",
		Label:   "보고방",
		Mode:    "briefing",
	}); err != nil {
		t.Fatal(err)
	}
	return home
}

func assertAssistantCoreCommandReply(t *testing.T, cmd assistantCoreCommand, input, visible string) {
	t.Helper()
	if strings.TrimSpace(visible) == "" {
		t.Fatalf("empty visible reply for Core-10 command %q input %q", cmd.ID, input)
	}
	for _, want := range cmd.ExpectedVisibleMarkers {
		if !strings.Contains(visible, want) {
			t.Fatalf("Core-10 command %q input %q missing required %q:\n%s", cmd.ID, input, want, visible)
		}
	}
	if len(cmd.ExpectedAnyMarkers) > 0 && !containsAny(visible, cmd.ExpectedAnyMarkers...) {
		t.Fatalf("Core-10 command %q input %q missing any of %#v:\n%s", cmd.ID, input, cmd.ExpectedAnyMarkers, visible)
	}
	for _, bad := range append(assistantCommercialForbiddenVisibleMarkers(), cmd.ForbiddenMarkers...) {
		if strings.Contains(visible, bad) {
			t.Fatalf("Core-10 command %q input %q exposed forbidden %q:\n%s", cmd.ID, input, bad, visible)
		}
	}
}
