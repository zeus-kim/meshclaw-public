package main

import (
	"strings"
	"testing"
	"time"

	"github.com/meshclaw/meshclaw/internal/osauto"
)

func TestFormatArgosDoctorDisplay(t *testing.T) {
	out := map[string]interface{}{
		"argos": osauto.ArgosMacDoctorReport{
			OK: true,
			UIRunner: osauto.Result{
				OK:     true,
				Stdout: `{"accessibility_trusted":true}`,
			},
			UIRunnerInstall: osauto.UIRunnerInstallReport{
				OK:                 true,
				RunningAppPath:     "/Users/argos/Applications/Argos UI Runner.app",
				StableInstallInUse: true,
			},
			ReminderShortcut: osauto.ReminderShortcutReport{OK: true},
			Calendar:         osauto.CalendarAutomationReport{OK: true},
			Contacts:         osauto.ContactsAutomationReport{OK: true},
			Shortcuts:        osauto.Result{OK: true},
			Frontends: osauto.FrontendsReport{
				OK: true,
				Frontends: []osauto.FrontendStatus{
					{Provider: "claude", Available: true},
					{Provider: "chatgpt", Available: true},
				},
			},
			ScreenRecording: osauto.Result{OK: true, Stdout: "not checked; run meshclaw argos doctor --screen-recording --json to test screen recording"},
			Grants:          []osauto.ArgosPermissionGrant{{ID: "grant-1"}},
			CreatedAt:       time.Now().UTC(),
		},
		"signal_call": map[string]interface{}{
			"ok":               true,
			"signal_running":   true,
			"accessibility_ok": true,
			"current_input":    "BlackHole 2ch",
			"current_output":   "Mac mini 스피커",
		},
		"signal_images": map[string]interface{}{
			"ok":                      false,
			"ocr":                     "macos_vision",
			"ocr_script_ready":        true,
			"attachment_dir_writable": true,
			"next_actions":            []string{"set model"},
		},
	}

	got := formatArgosDoctorDisplay(out)
	for _, want := range []string{
		"Argos macOS assistant 상태",
		"전체: 정상",
		"Runner: 정상 | accessibility=yes",
		"Runner path: /Users/argos/Applications/Argos UI Runner.app",
		"- Reminders: 정상",
		"- Calendar: 정상",
		"- Contacts: 정상",
		"- Frontends: 정상 | claude, chatgpt",
		"- Screen Recording: 미검사",
		"- Signal call: 정상 | Signal running, accessibility ok",
		"- Signal image/OCR: 선택 기능 확인 필요 | OCR=macos_vision",
		"- Saved grants: 1개",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("doctor display missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "{") || strings.Contains(got, "code_signature") {
		t.Fatalf("doctor display should not dump JSON:\n%s", got)
	}
}

func TestFormatArgosSetupDisplay(t *testing.T) {
	report := osauto.ArgosMacSetupReport{
		OK:          true,
		UIRunnerURL: "http://127.0.0.1:48292",
		AccessibilityRequest: osauto.Result{
			Kind:   "argos_ui_runner_accessibility",
			Action: "request_accessibility",
			OK:     true,
		},
		RemindersRequest: osauto.Result{
			Kind:   "argos_ui_runner_reminders",
			Action: "request_reminders",
			OK:     true,
		},
		Doctor: osauto.ArgosMacDoctorReport{
			OK: true,
			UIRunner: osauto.Result{
				OK:     true,
				Stdout: `{"accessibility_trusted":true}`,
			},
			UIRunnerInstall:  osauto.UIRunnerInstallReport{OK: true, RunningAppPath: "/Users/argos/Applications/Argos UI Runner.app"},
			ReminderShortcut: osauto.ReminderShortcutReport{OK: true},
			Calendar:         osauto.CalendarAutomationReport{OK: true},
			Contacts:         osauto.ContactsAutomationReport{OK: true},
			Shortcuts:        osauto.Result{OK: true},
			Frontends:        osauto.FrontendsReport{OK: true},
			ScreenRecording:  osauto.Result{OK: true, Stdout: "not checked; run meshclaw argos doctor --screen-recording --json to test screen recording"},
		},
	}

	got := formatArgosSetupDisplay(report)
	for _, want := range []string{
		"Argos macOS setup",
		"상태: 완료",
		"Runner URL: http://127.0.0.1:48292",
		"실행한 단계:",
		"- Accessibility 요청: 정상",
		"- Reminders 권한 요청: 정상",
		"Doctor 요약:",
		"Runner: 정상 | accessibility=yes",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("setup display missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "{") || strings.Contains(got, "ui_runner_install") {
		t.Fatalf("setup display should not dump JSON:\n%s", got)
	}
}

func TestFormatArgosPermissionsDisplay(t *testing.T) {
	grants := []osauto.ArgosPermissionGrant{
		{ID: "b", Action: "reminder_create", Scope: "reminders", Label: "Reminders 할 일 생성", Source: "signal-auto", CreatedAt: time.Date(2026, 5, 25, 20, 31, 0, 0, time.UTC)},
		{ID: "a", Action: "open_app", Scope: "safari", Label: "Safari", Source: "signal:argos-assistant", CreatedAt: time.Date(2026, 5, 24, 1, 2, 0, 0, time.UTC)},
		{ID: "c", Action: "calendar_event_create", Scope: "calendar", Label: "Calendar 일정 생성", Source: "signal-auto", CreatedAt: time.Date(2026, 5, 25, 12, 15, 0, 0, time.UTC)},
		{ID: "d", Action: "macbook_command", Scope: "macbook_executor", Label: "MacBook 앱 실행기", Source: "signal-auto", CreatedAt: time.Date(2026, 5, 26, 6, 7, 0, 0, time.UTC)},
	}

	got := formatArgosPermissionsDisplay(grants)
	for _, want := range []string{
		"Argos 실행 권한",
		"총 4개",
		"Calendar:",
		"- c: Calendar 일정 생성",
		"Device runner:",
		"- d: MacBook 앱 실행기 [legacy]",
		"Mac/Web:",
		"- a: Safari",
		"Reminders:",
		"- b: Reminders 할 일 생성",
		"권한 제거: `meshclaw argos revoke <id>`",
		"legacy 권한은 기존 호환용입니다.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("permissions display missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "{") || strings.Contains(got, "created_at") {
		t.Fatalf("permissions display should not dump JSON:\n%s", got)
	}
}

func TestFormatMacRunnersDisplay(t *testing.T) {
	store := osauto.MacRunnerStore{
		Path: "/tmp/mac-runners.json",
		Runners: []osauto.MacRunner{{
			ID:           "imac",
			SSHTarget:    "operator@imac",
			Project:      "/Users/example/Documents/New project",
			Capabilities: []string{"calendar", "pages"},
			Enabled:      true,
			Selected:     true,
		}},
	}
	got := formatMacRunnersDisplay(store)
	for _, want := range []string{
		"Argos Mac 실행기",
		"Registry: /tmp/mac-runners.json",
		"총 1개, enabled 1개, selected imac",
		"- imac [selected]: enabled",
		"ssh: operator@imac",
		"project: /Users/example/Documents/New project",
		"기능: calendar, pages",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("mac runners display missing %q:\n%s", want, got)
		}
	}
}

func TestFormatMacRunnersDisplayExplainsEmptyRegistry(t *testing.T) {
	got := formatMacRunnersDisplay(osauto.MacRunnerStore{Path: "/tmp/mac-runners.json"})
	for _, want := range []string{
		"등록된 Mac 실행기가 없습니다.",
		"stable Argos UI Runner",
		"Mac 실행기는 다른 Mac을 추가 워커로 붙일 때만 필요합니다.",
		"meshclaw argos runners add",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("empty mac runners display missing %q:\n%s", want, got)
		}
	}
}

func TestFormatMacRunnerDoctorDisplay(t *testing.T) {
	report := osauto.MacRunnerDoctorReport{
		OK: true,
		Runner: osauto.MacRunner{
			ID:        "imac",
			SSHTarget: "operator@imac",
			Project:   "/Users/example/Documents/New project",
		},
		Checks: []osauto.Result{
			{Action: "ssh_probe", OK: true},
			{Action: "project_dir", OK: true},
		},
	}
	got := formatMacRunnerDoctorDisplay(report)
	for _, want := range []string{
		"Argos Mac 실행기 진단",
		"대상: imac",
		"상태: 정상",
		"ssh: operator@imac",
		"- ssh_probe: 정상",
		"- project_dir: 정상",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("mac runner doctor display missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "{") {
		t.Fatalf("mac runner doctor display should not dump JSON:\n%s", got)
	}
}
