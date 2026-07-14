package messenger

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meshclaw/meshclaw/internal/mailadapter"
	"github.com/meshclaw/meshclaw/internal/osauto"
)

func TestAssistantApprovalQueueShowsPendingMacActionReadOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-mac-action-queue"
	resetPendingArgosActionForTest(t, targetID)
	now := time.Now().UTC()

	rememberPendingArgosAction(argosPendingKey(targetID), pendingArgosAction{
		Action: osauto.ArgosAction{
			Action:        "calendar_event_create",
			CalendarTitle: "Argos 테스트 회의",
			CalendarStart: "2026-06-16T10:00:00+09:00",
			CalendarEnd:   "2026-06-16T10:30:00+09:00",
		},
		Decision: osauto.ArgosPermissionDecision{
			Grantable: true,
			Allowed:   false,
			Action:    "calendar_event_create",
			Scope:     "calendar:create",
			Label:     "Calendar 생성",
		},
		CreatedAt: now,
		ExpiresAt: now.Add(10 * time.Minute),
	})

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "승인 대기열 보여줘")
	for _, want := range []string{
		"Argos 승인 대기열",
		"바로 보낼 문장:",
		"일정/리마인더/Mac 작업: `실행`",
		"`일정 승인`",
		"`리마인더 승인`",
		"같은 범위 반복 허용: `항상 허용`",
		"최근 Mac 작업 실행 대기 후보",
		"캘린더 일정 생성",
		"Argos 테스트 회의",
		"범위=Calendar 생성",
		"`실행`",
		"`항상 허용`",
		"읽기 전용",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("approval queue missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"일정을 만들었습니다", "예약 완료", "실행했습니다"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("approval queue should not claim execution %q:\n%s", bad, reply)
		}
	}
	if _, ok := peekPendingArgosAction(argosPendingKey(targetID)); !ok {
		t.Fatalf("approval queue consumed the pending Mac action")
	}
}

func TestAssistantApprovalQueuePendingMacActionEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_LANG", "")
	targetID := "argos-assistant-mac-action-queue-en"
	resetPendingArgosActionForTest(t, targetID)
	now := time.Now().UTC()

	rememberPendingArgosAction(argosPendingKey(targetID), pendingArgosAction{
		Action: osauto.ArgosAction{
			Action:        "reminder_create",
			ReminderTitle: "Take medicine",
			ReminderDue:   "2026-06-16T08:00:00+09:00",
		},
		Decision: osauto.ArgosPermissionDecision{
			Grantable: false,
			Allowed:   false,
			Action:    "reminder_create",
			Scope:     "reminder:create",
			Label:     "Reminder create",
		},
		CreatedAt: now,
		ExpiresAt: now.Add(10 * time.Minute),
	})

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "approval queue")
	for _, want := range []string{
		"Argos approval queue.",
		"Send one of these exact phrases:",
		"Calendar/reminder/Mac action: `execute`",
		"`calendar approved`",
		"`reminder approved`",
		"Recent pending Mac action candidate",
		"Create reminder",
		"Take medicine",
		"scope=Reminder create",
		"`execute`",
		"No repeat allow",
		"This queue card is read-only",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English approval queue missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English approval queue should not expose Korean:\n%s", reply)
	}
	if _, ok := peekPendingArgosAction(argosPendingKey(targetID)); !ok {
		t.Fatalf("approval queue consumed the pending Mac action")
	}
}

func TestArgosApprovalTextAcceptsCalendarReminderAliases(t *testing.T) {
	for _, input := range []string{
		"리마인더 승인",
		"리마인더 실행해",
		"일정 승인",
		"캘린더 실행해줘",
		"reminder approved",
		"approve reminder",
		"calendar approved",
		"approve calendar",
		"event approved",
	} {
		if !isArgosExecuteOnceText(input) {
			t.Fatalf("expected %q to execute pending calendar/reminder action", input)
		}
	}
	for _, input := range []string{
		"메일 승인",
		"구매 승인",
		"리마인더 목록",
		"calendar approval queue",
	} {
		if isArgosExecuteOnceText(input) {
			t.Fatalf("did not expect %q to execute pending calendar/reminder action", input)
		}
	}
}

func TestAssistantFinalApprovalShowsPendingMacActionExecutionCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	targetID := "argos-assistant-final-approval-mac-action"
	resetPendingArgosActionForTest(t, targetID)
	now := time.Now().UTC()

	rememberPendingArgosAction(argosPendingKey(targetID), pendingArgosAction{
		Action: osauto.ArgosAction{
			Action:        "reminder_create",
			ReminderTitle: "약 먹기",
			ReminderDue:   "2026-06-18T08:00:00+09:00",
		},
		Decision: osauto.ArgosPermissionDecision{
			Grantable: false,
			Allowed:   false,
			Action:    "reminder_create",
			Scope:     "reminder:create",
			Label:     "리마인더 생성",
		},
		CreatedAt: now,
		ExpiresAt: now.Add(10 * time.Minute),
	})

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "최종 승인 카드 보여줘")
	for _, want := range []string{
		"범용 최종 승인 게이트입니다.",
		"최근 일정/리마인더 후보:",
		"- 작업: 리마인더 생성",
		"- 대상: 약 먹기",
		"- 범위: 리마인더 생성",
		"- 실행 문장: `실행`",
		"`리마인더 승인`",
		"- kind: reminder_create",
		"- evidence_source: pending_argos_action",
		"- execute=false",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("final approval card missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "리마인더를 만들었습니다") || strings.Contains(reply, "실행했습니다") {
		t.Fatalf("final approval card should not execute the Mac action:\n%s", reply)
	}
	if _, ok := peekPendingArgosAction(argosPendingKey(targetID)); !ok {
		t.Fatalf("final approval card consumed the pending Mac action")
	}
}

func TestAssistantFinalApprovalShowsPendingMailExecutionCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	resetPendingMailSendForTest(t)
	targetID := "argos-assistant-final-approval-mail"
	now := time.Now().UTC()
	rememberPendingMailSend(mailPendingKey(targetID), pendingMailSend{
		Draft: mailadapter.Draft{
			ID:        "draft-final-approval",
			Account:   "personal",
			To:        []string{"sender@example.com"},
			Subject:   "Re: 회의 자료",
			Body:      "확인했습니다. 감사합니다.",
			Status:    "draft",
			CreatedAt: now,
		},
		CreatedAt: now,
		ExpiresAt: now.Add(10 * time.Minute),
	})

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "발송 최종 승인 카드 보여줘")
	for _, want := range []string{
		"범용 최종 승인 게이트입니다.",
		"최근 메일 후보:",
		"- 수신: sender@example.com",
		"- 제목: Re: 회의 자료",
		"- draft: draft-final-approval",
		"- 실행 문장: `승인`",
		"- kind: mail_send",
		"- evidence_source: pending_mail_draft",
		"- execute=false",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("final approval mail card missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "메일을 발송했습니다") {
		t.Fatalf("final approval card should not send mail:\n%s", reply)
	}
	if _, ok := peekPendingMailSend(mailPendingKey(targetID)); !ok {
		t.Fatalf("final approval card consumed the pending mail draft")
	}
}

func TestAssistantShortApprovalDisambiguatesPendingMailAndMacAction(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	resetPendingMailSendForTest(t)
	targetID := "argos-assistant-ambiguous-short-approval"
	resetPendingArgosActionForTest(t, targetID)
	now := time.Now().UTC()
	rememberPendingMailSend(mailPendingKey(targetID), pendingMailSend{
		Draft: mailadapter.Draft{
			ID:        "draft-ambiguous-short",
			Account:   "personal",
			To:        []string{"sender@example.com"},
			Subject:   "Re: 회의",
			Body:      "확인했습니다.",
			Status:    "draft",
			CreatedAt: now,
		},
		CreatedAt: now,
		ExpiresAt: now.Add(10 * time.Minute),
	})
	rememberPendingArgosAction(argosPendingKey(targetID), pendingArgosAction{
		Action: osauto.ArgosAction{
			Action:        "reminder_create",
			ReminderTitle: "회의 자료 확인",
			ReminderDue:   "2026-06-18T08:00:00+09:00",
		},
		Decision: osauto.ArgosPermissionDecision{
			Grantable: false,
			Allowed:   false,
			Action:    "reminder_create",
			Scope:     "reminder:create",
			Label:     "리마인더 생성",
		},
		CreatedAt: now,
		ExpiresAt: now.Add(10 * time.Minute),
	})

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "ㅇㅇ")
	for _, want := range []string{
		"대기 중인 실행 후보가 둘 이상입니다.",
		"- 메일 발송: `승인`",
		"- 일정/리마인더/Mac 작업: `실행`",
		"아무 작업도 실행하지 않았습니다.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("ambiguous approval reply missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "메일을 보냈습니다") || strings.Contains(reply, "리마인더를 만들었습니다") || strings.Contains(reply, "실행했습니다") {
		t.Fatalf("ambiguous approval should not execute anything:\n%s", reply)
	}
	if _, ok := peekPendingMailSend(mailPendingKey(targetID)); !ok {
		t.Fatalf("ambiguous approval consumed the pending mail draft")
	}
	if _, ok := peekPendingArgosAction(argosPendingKey(targetID)); !ok {
		t.Fatalf("ambiguous approval consumed the pending Mac action")
	}
}

func TestMailApprovalTextAcceptsNaturalAliases(t *testing.T) {
	for _, input := range []string{
		"메일 승인",
		"메일 발송 승인",
		"메일 보내줘",
		"이메일 승인",
		"이메일 발송해",
		"mail approved",
		"approve mail",
		"email approved",
		"send approved",
		"send mail",
	} {
		if !isMailApprovalText(input) {
			t.Fatalf("expected %q to approve a pending mail draft", input)
		}
	}
	for _, input := range []string{
		"일정 승인",
		"리마인더 승인",
		"구매 승인",
		"메일 검색",
		"mail search",
	} {
		if isMailApprovalText(input) {
			t.Fatalf("did not expect %q to approve a pending mail draft", input)
		}
	}
}

func TestAssistantExactMailApprovalWinsWhenMailAndMacActionPending(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	resetPendingMailSendForTest(t)
	targetID := "argos-assistant-exact-mail-approval"
	resetPendingArgosActionForTest(t, targetID)
	now := time.Now().UTC()
	rememberPendingMailSend(mailPendingKey(targetID), pendingMailSend{
		Draft: mailadapter.Draft{
			ID:        "draft-exact-mail",
			Account:   "personal",
			To:        []string{"sender@example.com"},
			Subject:   "Re: 회의",
			Body:      "확인했습니다.",
			Status:    "draft",
			CreatedAt: now,
		},
		CreatedAt: now,
		ExpiresAt: now.Add(10 * time.Minute),
	})
	rememberPendingArgosAction(argosPendingKey(targetID), pendingArgosAction{
		Action: osauto.ArgosAction{
			Action:        "reminder_create",
			ReminderTitle: "회의 자료 확인",
			ReminderDue:   "2026-06-18T08:00:00+09:00",
		},
		Decision: osauto.ArgosPermissionDecision{
			Grantable: false,
			Allowed:   false,
			Action:    "reminder_create",
			Scope:     "reminder:create",
			Label:     "리마인더 생성",
		},
		CreatedAt: now,
		ExpiresAt: now.Add(10 * time.Minute),
	})

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "승인")
	if !strings.Contains(reply, "메일 발송에 실패했습니다") || !strings.Contains(reply, "draft-exact-mail") {
		t.Fatalf("exact mail approval should route to the pending mail draft:\n%s", reply)
	}
	if _, ok := peekPendingMailSend(mailPendingKey(targetID)); ok {
		t.Fatalf("exact mail approval should consume the pending mail draft")
	}
	if _, ok := peekPendingArgosAction(argosPendingKey(targetID)); !ok {
		t.Fatalf("exact mail approval should leave the pending Mac action untouched")
	}
}

func TestAssistantNaturalMailApprovalWinsWhenMailAndMacActionPending(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	resetPendingMailSendForTest(t)
	targetID := "argos-assistant-natural-mail-approval"
	resetPendingArgosActionForTest(t, targetID)
	now := time.Now().UTC()
	rememberPendingMailSend(mailPendingKey(targetID), pendingMailSend{
		Draft: mailadapter.Draft{
			ID:        "draft-natural-mail",
			Account:   "personal",
			To:        []string{"sender@example.com"},
			Subject:   "Re: 회의",
			Body:      "확인했습니다.",
			Status:    "draft",
			CreatedAt: now,
		},
		CreatedAt: now,
		ExpiresAt: now.Add(10 * time.Minute),
	})
	rememberPendingArgosAction(argosPendingKey(targetID), pendingArgosAction{
		Action: osauto.ArgosAction{
			Action:        "reminder_create",
			ReminderTitle: "회의 자료 확인",
			ReminderDue:   "2026-06-18T08:00:00+09:00",
		},
		Decision: osauto.ArgosPermissionDecision{
			Grantable: false,
			Allowed:   false,
			Action:    "reminder_create",
			Scope:     "reminder:create",
			Label:     "리마인더 생성",
		},
		CreatedAt: now,
		ExpiresAt: now.Add(10 * time.Minute),
	})

	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "메일 발송 승인")
	if !strings.Contains(reply, "메일 발송에 실패했습니다") || !strings.Contains(reply, "draft-natural-mail") {
		t.Fatalf("natural mail approval should route to the pending mail draft:\n%s", reply)
	}
	if _, ok := peekPendingMailSend(mailPendingKey(targetID)); ok {
		t.Fatalf("natural mail approval should consume the pending mail draft")
	}
	if _, ok := peekPendingArgosAction(argosPendingKey(targetID)); !ok {
		t.Fatalf("natural mail approval should leave the pending Mac action untouched")
	}
}

func resetPendingArgosActionForTest(t *testing.T, targetID string) {
	t.Helper()
	key := argosPendingKey(targetID)
	argosActionPending.Lock()
	delete(argosActionPending.items, key)
	argosActionPending.Unlock()
	t.Cleanup(func() {
		argosActionPending.Lock()
		delete(argosActionPending.items, key)
		argosActionPending.Unlock()
	})
}
