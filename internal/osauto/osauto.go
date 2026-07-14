package osauto

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/meshclaw/meshclaw/internal/aichat"
	"github.com/meshclaw/meshclaw/internal/argosreport"
	"github.com/meshclaw/meshclaw/internal/browserauto"
	"github.com/meshclaw/meshclaw/internal/lang"
	"github.com/meshclaw/meshclaw/internal/publish"
)

type Result struct {
	Kind      string    `json:"kind"`
	Action    string    `json:"action"`
	Command   []string  `json:"command,omitempty"`
	Provider  string    `json:"provider,omitempty"`
	Prompt    string    `json:"prompt,omitempty"`
	URL       string    `json:"url,omitempty"`
	App       string    `json:"app,omitempty"`
	Preview   string    `json:"preview,omitempty"`
	PDF       string    `json:"pdf,omitempty"`
	DOCX      string    `json:"docx,omitempty"`
	PPTX      string    `json:"pptx,omitempty"`
	XLSX      string    `json:"xlsx,omitempty"`
	CSV       string    `json:"csv,omitempty"`
	Markdown  string    `json:"markdown,omitempty"`
	OK        bool      `json:"ok"`
	Stdout    string    `json:"stdout,omitempty"`
	Stderr    string    `json:"stderr,omitempty"`
	Error     string    `json:"error,omitempty"`
	SizeBytes int64     `json:"size_bytes,omitempty"`
	SHA256    string    `json:"sha256,omitempty"`
	DeleteAt  string    `json:"delete_at,omitempty"`
	Retention string    `json:"retention,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type workDemoDocumentFile struct {
	Path        string
	PreviewPath string
	Body        string
	App         string
}

type ArgosRequest struct {
	Text          string `json:"text"`
	Execute       bool   `json:"execute"`
	RecordSeconds int    `json:"record_seconds,omitempty"`
	RecordPath    string `json:"record_path,omitempty"`
}

type ArgosAction struct {
	Kind             string                    `json:"kind"`
	Text             string                    `json:"text"`
	Intent           string                    `json:"intent"`
	Action           string                    `json:"action"`
	Query            string                    `json:"query,omitempty"`
	URL              string                    `json:"url,omitempty"`
	App              string                    `json:"app,omitempty"`
	Shortcut         string                    `json:"shortcut,omitempty"`
	Input            string                    `json:"input,omitempty"`
	Provider         string                    `json:"provider,omitempty"`
	Prompt           string                    `json:"prompt,omitempty"`
	NoteTitle        string                    `json:"note_title,omitempty"`
	NoteBody         string                    `json:"note_body,omitempty"`
	ReminderTitle    string                    `json:"reminder_title,omitempty"`
	ReminderNotes    string                    `json:"reminder_notes,omitempty"`
	ReminderDue      string                    `json:"reminder_due,omitempty"`
	ReminderQuery    string                    `json:"reminder_query,omitempty"`
	ReminderStart    string                    `json:"reminder_start,omitempty"`
	ReminderEnd      string                    `json:"reminder_end,omitempty"`
	ReminderID       string                    `json:"reminder_id,omitempty"`
	CalendarTitle    string                    `json:"calendar_title,omitempty"`
	CalendarNotes    string                    `json:"calendar_notes,omitempty"`
	CalendarStart    string                    `json:"calendar_start,omitempty"`
	CalendarEnd      string                    `json:"calendar_end,omitempty"`
	CalendarQuery    string                    `json:"calendar_query,omitempty"`
	CalendarID       string                    `json:"calendar_id,omitempty"`
	ContactQuery     string                    `json:"contact_query,omitempty"`
	OutputPath       string                    `json:"output_path,omitempty"`
	RequiresApproval bool                      `json:"requires_approval"`
	Executed         bool                      `json:"executed"`
	Result           *Result                   `json:"result,omitempty"`
	Search           *browserauto.SearchResult `json:"search,omitempty"`
	Page             *browserauto.Page         `json:"page,omitempty"`
	Recording        *Result                   `json:"recording,omitempty"`
	RecordingPath    string                    `json:"recording_path,omitempty"`
	Error            string                    `json:"error,omitempty"`
	NextActions      []string                  `json:"next_actions,omitempty"`
	CreatedAt        time.Time                 `json:"created_at"`
}

type AIHandoffOptions struct {
	Provider string
	Prompt   string
}

type FrontendStatus struct {
	Provider    string   `json:"provider"`
	App         string   `json:"app,omitempty"`
	URL         string   `json:"url,omitempty"`
	Available   bool     `json:"available"`
	Mode        string   `json:"mode"`
	Error       string   `json:"error,omitempty"`
	NextActions []string `json:"next_actions,omitempty"`
}

type FrontendsReport struct {
	Kind      string           `json:"kind"`
	OK        bool             `json:"ok"`
	Frontends []FrontendStatus `json:"frontends"`
	CreatedAt time.Time        `json:"created_at"`
}

type ArgosMacDoctorReport struct {
	Kind             string                   `json:"kind"`
	OK               bool                     `json:"ok"`
	OS               string                   `json:"os"`
	User             string                   `json:"user,omitempty"`
	PermissionsPath  string                   `json:"permissions_path,omitempty"`
	UIRunnerURL      string                   `json:"ui_runner_url,omitempty"`
	UIRunner         Result                   `json:"ui_runner,omitempty"`
	UIRunnerInstall  UIRunnerInstallReport    `json:"ui_runner_install"`
	Grants           []ArgosPermissionGrant   `json:"grants,omitempty"`
	Frontends        FrontendsReport          `json:"frontends"`
	Shortcuts        Result                   `json:"shortcuts"`
	ReminderShortcut ReminderShortcutReport   `json:"reminder_shortcut"`
	Calendar         CalendarAutomationReport `json:"calendar"`
	Contacts         ContactsAutomationReport `json:"contacts"`
	ScreenRecording  Result                   `json:"screen_recording"`
	Problems         []string                 `json:"problems,omitempty"`
	NextActions      []string                 `json:"next_actions,omitempty"`
	CreatedAt        time.Time                `json:"created_at"`
}

type ReminderShortcutReport struct {
	Kind          string    `json:"kind"`
	OK            bool      `json:"ok"`
	Installed     bool      `json:"installed"`
	ShortcutName  string    `json:"shortcut_name,omitempty"`
	ExpectedNames []string  `json:"expected_names"`
	InputSchema   string    `json:"input_schema"`
	ExampleInput  string    `json:"example_input"`
	TestCommand   string    `json:"test_command"`
	SetupSteps    []string  `json:"setup_steps,omitempty"`
	Error         string    `json:"error,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type CalendarAutomationReport struct {
	Kind         string    `json:"kind"`
	OK           bool      `json:"ok"`
	Installed    bool      `json:"installed"`
	Provider     string    `json:"provider,omitempty"`
	InputSchema  string    `json:"input_schema"`
	ExampleInput string    `json:"example_input"`
	TestCommand  string    `json:"test_command"`
	SetupSteps   []string  `json:"setup_steps,omitempty"`
	Error        string    `json:"error,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type ContactsAutomationReport struct {
	Kind         string    `json:"kind"`
	OK           bool      `json:"ok"`
	Installed    bool      `json:"installed"`
	Provider     string    `json:"provider,omitempty"`
	InputSchema  string    `json:"input_schema"`
	ExampleInput string    `json:"example_input"`
	TestCommand  string    `json:"test_command"`
	SetupSteps   []string  `json:"setup_steps,omitempty"`
	Error        string    `json:"error,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type UIRunnerInstallReport struct {
	Kind               string    `json:"kind"`
	OK                 bool      `json:"ok"`
	ExpectedBundleID   string    `json:"expected_bundle_id"`
	RecommendedPath    string    `json:"recommended_path"`
	InstalledPath      string    `json:"installed_path,omitempty"`
	Installed          bool      `json:"installed"`
	BundleID           string    `json:"bundle_id,omitempty"`
	Executable         string    `json:"executable,omitempty"`
	CodeSigned         bool      `json:"code_signed"`
	CodeSignature      string    `json:"code_signature,omitempty"`
	BinarySHA256       string    `json:"binary_sha256,omitempty"`
	AppSHA256          string    `json:"app_sha256,omitempty"`
	RunningAppPath     string    `json:"running_app_path,omitempty"`
	RunningExecutable  string    `json:"running_executable,omitempty"`
	RunningBundleID    string    `json:"running_bundle_id,omitempty"`
	RunningCodeSigned  bool      `json:"running_code_signed,omitempty"`
	StableInstallInUse bool      `json:"stable_install_in_use"`
	StagedPath         string    `json:"staged_path,omitempty"`
	StagedBundleID     string    `json:"staged_bundle_id,omitempty"`
	StagedCodeSigned   bool      `json:"staged_code_signed,omitempty"`
	StagedBinarySHA256 string    `json:"staged_binary_sha256,omitempty"`
	StagedAppSHA256    string    `json:"staged_app_sha256,omitempty"`
	StagedNeedsApply   bool      `json:"staged_needs_apply,omitempty"`
	Problems           []string  `json:"problems,omitempty"`
	NextActions        []string  `json:"next_actions,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}

type ArgosMacSetupReport struct {
	Kind                 string               `json:"kind"`
	OK                   bool                 `json:"ok"`
	UIRunnerURL          string               `json:"ui_runner_url"`
	OpenRunner           Result               `json:"open_runner,omitempty"`
	OpenAccessibility    Result               `json:"open_accessibility_settings,omitempty"`
	OpenScreenRecording  Result               `json:"open_screen_recording_settings,omitempty"`
	AccessibilityRequest Result               `json:"accessibility_request,omitempty"`
	RemindersRequest     Result               `json:"reminders_request,omitempty"`
	CalendarRequest      Result               `json:"calendar_request,omitempty"`
	ContactsRequest      Result               `json:"contacts_request,omitempty"`
	Doctor               ArgosMacDoctorReport `json:"doctor"`
	Problems             []string             `json:"problems,omitempty"`
	NextActions          []string             `json:"next_actions,omitempty"`
	CreatedAt            time.Time            `json:"created_at"`
}

func ShortcutsList(ctx context.Context) Result {
	return run(ctx, "meshclaw_automation_shortcuts_list", "shortcuts", "list")
}

func ArgosMacDoctor(ctx context.Context, checkScreenRecording bool) ArgosMacDoctorReport {
	runnerURL := defaultUIRunnerURL()
	runnerHealth := UIRunnerHealth(ctx, runnerURL)
	report := ArgosMacDoctorReport{
		Kind:             "meshclaw_argos_macos_doctor",
		OK:               true,
		OS:               runtime.GOOS,
		PermissionsPath:  argosPermissionPath(),
		UIRunnerURL:      runnerURL,
		UIRunner:         runnerHealth,
		UIRunnerInstall:  ArgosUIRunnerInstallDoctor(ctx, &runnerHealth),
		Frontends:        FrontendsDoctor(ctx),
		Shortcuts:        ShortcutsList(ctx),
		ReminderShortcut: ReminderShortcutDoctor(ctx),
		Calendar:         CalendarAutomationDoctor(ctx),
		Contacts:         ContactsAutomationDoctor(ctx),
		ScreenRecording: Result{
			Kind:      "meshclaw_automation_screen_record",
			Action:    "screen_record",
			OK:        true,
			Stdout:    "not checked; run meshclaw argos doctor --screen-recording --json to test screen recording",
			CreatedAt: time.Now().UTC(),
		},
		CreatedAt: time.Now().UTC(),
	}
	if user := strings.TrimSpace(os.Getenv("USER")); user != "" {
		report.User = user
	}
	grants, err := ListArgosPermissions()
	if err != nil {
		report.Problems = append(report.Problems, "Argos 실행 권한 저장소를 읽을 수 없습니다: "+err.Error())
		report.NextActions = append(report.NextActions, report.PermissionsPath+"의 소유자와 파일 권한을 확인하세요. 로컬 사용자만 읽을 수 있어야 합니다.")
		report.OK = false
	} else {
		report.Grants = grants
	}
	if !report.UIRunner.OK {
		report.OK = false
		report.Problems = append(report.Problems, "Argos UI Runner가 준비되지 않았습니다: "+firstNonEmpty(report.UIRunner.Error, report.UIRunner.Stderr, "unknown error"))
		report.NextActions = append(report.NextActions,
			"Argos UI Runner.app을 열고, 시스템 설정 > 개인정보 보호 및 보안 > 손쉬운 사용에서 Argos UI Runner를 허용하세요.",
			"이후 `meshclaw argos setup --json` 또는 Signal에서 `Argos setup`을 다시 실행하세요.",
		)
	}
	if !report.UIRunnerInstall.OK {
		report.OK = false
		report.Problems = append(report.Problems, report.UIRunnerInstall.Problems...)
		report.NextActions = append(report.NextActions, report.UIRunnerInstall.NextActions...)
	}
	if !report.Frontends.OK {
		report.OK = false
		report.Problems = append(report.Problems, "이 Mac에서 로그인된 AI 프론트엔드 앱 일부를 사용할 수 없습니다.")
	}
	if !report.Shortcuts.OK {
		report.OK = false
		report.Problems = append(report.Problems, "Shortcuts CLI가 준비되지 않았습니다: "+firstNonEmpty(report.Shortcuts.Error, report.Shortcuts.Stderr, "unknown error"))
		report.NextActions = append(report.NextActions, "Shortcuts 앱을 한 번 열고 Argos용 단축어가 설치되어 있는지 확인하세요.")
	}
	if !report.ReminderShortcut.OK {
		report.OK = false
		report.Problems = append(report.Problems, "Argos 리마인더 단축어가 아직 없습니다. 리마인더 생성은 Reminders 직접 제어로 fallback되며 macOS Automation 권한에서 막힐 수 있습니다.")
		report.NextActions = append(report.NextActions, report.ReminderShortcut.SetupSteps...)
	}
	if !report.Calendar.OK {
		report.OK = false
		report.Problems = append(report.Problems, "Argos Calendar helper가 아직 준비되지 않았습니다. 캘린더 일정 생성은 Argos UI Runner의 Calendar 권한이 필요합니다.")
		report.NextActions = append(report.NextActions, report.Calendar.SetupSteps...)
	}
	if !report.Contacts.OK {
		report.OK = false
		report.Problems = append(report.Problems, "Argos Contacts helper가 아직 준비되지 않았습니다. 연락처 조회는 Argos UI Runner의 Contacts 권한이 필요합니다.")
		report.NextActions = append(report.NextActions, report.Contacts.SetupSteps...)
	}
	if checkScreenRecording {
		output := filepath.Join(defaultMeshClawDir(), "doctor", fmt.Sprintf("screen-recording-%s.mov", time.Now().UTC().Format("20060102T150405Z")))
		report.ScreenRecording = ScreenRecord(ctx, 1, output)
		if !report.ScreenRecording.OK {
			report.OK = false
			report.Problems = append(report.Problems, "화면 기록 테스트 실패: "+firstNonEmpty(report.ScreenRecording.Error, report.ScreenRecording.Stderr, "unknown error"))
			report.NextActions = append(report.NextActions,
				"signal-dispatcher가 도는 Mac에서 시스템 설정 > 개인정보 보호 및 보안 > 화면 및 시스템 오디오 기록을 여세요.",
				"Argos UI Runner.app에 화면 기록 권한을 허용하세요. 화면 녹화는 MeshClaw 바이너리가 아니라 stable Runner가 수행합니다.",
				"권한 허용 후 `meshclaw daemon restart signal-dispatcher --json`으로 재시작하고 `meshclaw argos doctor --json`으로 확인하세요.",
			)
		}
	}
	if runtime.GOOS != "darwin" {
		report.OK = false
		report.Problems = append(report.Problems, "Argos macOS 자동화는 darwin이 필요합니다. 현재 OS: "+runtime.GOOS)
	}
	return report
}

func ArgosMacSetup(ctx context.Context) ArgosMacSetupReport {
	runnerURL := defaultUIRunnerURL()
	report := ArgosMacSetupReport{
		Kind:        "meshclaw_argos_macos_setup",
		UIRunnerURL: runnerURL,
		CreatedAt:   time.Now().UTC(),
	}
	health := UIRunnerHealth(ctx, runnerURL)
	if !health.OK && strings.TrimSpace(health.Stdout) == "" {
		report.OpenRunner = OpenUIRunnerApp(ctx)
		time.Sleep(1200 * time.Millisecond)
		health = UIRunnerHealth(ctx, runnerURL)
	}
	if health.OK || strings.TrimSpace(health.Stdout) != "" {
		report.AccessibilityRequest = UIRunnerRequestAccessibility(ctx, runnerURL)
		if !report.AccessibilityRequest.OK {
			report.OpenAccessibility = OpenMacPrivacyPane(ctx, "accessibility")
		}
		report.RemindersRequest = UIRunnerRequestReminders(ctx, runnerURL)
		report.CalendarRequest = UIRunnerRequestCalendar(ctx, runnerURL)
		report.ContactsRequest = UIRunnerRequestContacts(ctx, runnerURL)
	} else {
		report.Problems = append(report.Problems, "Argos UI Runner에 연결할 수 없습니다: "+firstNonEmpty(health.Error, health.Stderr, "unknown error"))
		report.NextActions = append(report.NextActions, "Argos UI Runner.app이 설치되어 있는지 확인한 뒤 직접 한 번 실행하세요.")
	}
	report.Doctor = ArgosMacDoctor(ctx, true)
	if report.Doctor.ScreenRecording.Command != nil && !report.Doctor.ScreenRecording.OK {
		report.OpenScreenRecording = OpenMacPrivacyPane(ctx, "screen_recording")
	}
	report.Problems = append(report.Problems, report.Doctor.Problems...)
	report.NextActions = append(report.NextActions, report.Doctor.NextActions...)
	if len(report.Problems) > 0 {
		report.NextActions = append(report.NextActions,
			"원격으로 처리하려면 iPhone/MacBook에서 Screen Sharing, VNC, Jump Desktop, 또는 Screens로 이 Mac에 접속한 뒤 열린 설정 화면에서 Argos UI Runner.app 토글만 켜세요.",
			"제품 기본 구조는 항상 켜진 Mac 한 대입니다. 여러 Mac은 나중에 각 Mac별 Runner와 Screen Sharing 복구 채널로 확장합니다.",
		)
	}
	report.OK = len(report.Problems) == 0
	return report
}

func OpenMacPrivacyPane(ctx context.Context, pane string) Result {
	if runtime.GOOS != "darwin" {
		return failed("meshclaw_automation_open_privacy_settings", "macOS privacy settings are only implemented for darwin")
	}
	var target string
	switch strings.ToLower(strings.TrimSpace(pane)) {
	case "accessibility", "손쉬운 사용":
		target = "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"
	case "screen_recording", "screen-recording", "screen", "화면 기록":
		target = "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture"
	default:
		target = "x-apple.systempreferences:com.apple.preference.security"
	}
	result := OpenURL(ctx, target)
	result.Kind = "meshclaw_automation_open_privacy_settings"
	result.Action = "open_privacy_settings"
	result.URL = target
	if !result.OK {
		fallback := OpenApp(ctx, "System Settings")
		result.Command = append(result.Command, fallback.Command...)
		result.Stderr = strings.TrimSpace(strings.Join(nonEmpty(result.Stderr, fallback.Stderr), "\n"))
		result.Error = firstNonEmpty(fallback.Error, result.Error)
		result.OK = fallback.OK
	}
	return result
}

func UIRunnerHealth(ctx context.Context, runnerURL string) Result {
	runnerURL = strings.TrimRight(strings.TrimSpace(runnerURL), "/")
	if runnerURL == "" {
		runnerURL = defaultUIRunnerURL()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, runnerURL+"/health", nil)
	if err != nil {
		return failed("meshclaw_automation_ui_runner_health", err.Error())
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return failed("meshclaw_automation_ui_runner_health", err.Error())
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	result := Result{
		Kind:      "meshclaw_automation_ui_runner_health",
		Action:    "ui_runner_health",
		Command:   []string{"GET", runnerURL + "/health"},
		OK:        resp.StatusCode >= 200 && resp.StatusCode < 300,
		Stdout:    strings.TrimSpace(buf.String()),
		CreatedAt: time.Now().UTC(),
	}
	if !result.OK {
		result.Error = "ui runner health returned " + resp.Status
		return result
	}
	if strings.Contains(result.Stdout, `"ok":false`) {
		result.OK = false
		result.Error = "Argos UI Runner returned ok=false"
	}
	return result
}

func UIRunnerRequestAccessibility(ctx context.Context, runnerURL string) Result {
	runnerURL = strings.TrimRight(strings.TrimSpace(runnerURL), "/")
	if runnerURL == "" {
		runnerURL = defaultUIRunnerURL()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, runnerURL+"/request-accessibility", nil)
	if err != nil {
		return failed("meshclaw_automation_ui_runner_request_accessibility", err.Error())
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return failed("meshclaw_automation_ui_runner_request_accessibility", err.Error())
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	result := Result{
		Kind:      "meshclaw_automation_ui_runner_request_accessibility",
		Action:    "ui_runner_request_accessibility",
		Command:   []string{"POST", runnerURL + "/request-accessibility"},
		OK:        resp.StatusCode >= 200 && resp.StatusCode < 300,
		Stdout:    strings.TrimSpace(buf.String()),
		CreatedAt: time.Now().UTC(),
	}
	if !result.OK {
		result.Error = "ui runner accessibility request returned " + resp.Status
		return result
	}
	if strings.Contains(result.Stdout, `"ok":false`) {
		result.OK = false
		result.Error = "Accessibility permission is still not granted"
	}
	return result
}

func UIRunnerRequestReminders(ctx context.Context, runnerURL string) Result {
	if runnerURL == "" {
		runnerURL = defaultUIRunnerURL()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, runnerURL+"/request-reminders", nil)
	if err != nil {
		return failed("meshclaw_automation_ui_runner_request_reminders", err.Error())
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return failed("meshclaw_automation_ui_runner_request_reminders", err.Error())
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	result := Result{
		Kind:      "meshclaw_automation_ui_runner_request_reminders",
		Action:    "ui_runner_request_reminders",
		Command:   []string{"POST", runnerURL + "/request-reminders"},
		OK:        resp.StatusCode >= 200 && resp.StatusCode < 300,
		Stdout:    strings.TrimSpace(buf.String()),
		CreatedAt: time.Now().UTC(),
	}
	if !result.OK || strings.Contains(result.Stdout, `"ok":false`) {
		result.OK = false
		result.Error = firstNonEmpty(uiRunnerError(result.Stdout), "Reminders permission is still not granted")
	}
	return result
}

func UIRunnerRequestCalendar(ctx context.Context, runnerURL string) Result {
	if runnerURL == "" {
		runnerURL = defaultUIRunnerURL()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, runnerURL+"/request-calendar", nil)
	if err != nil {
		return failed("meshclaw_automation_ui_runner_request_calendar", err.Error())
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return failed("meshclaw_automation_ui_runner_request_calendar", err.Error())
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	result := Result{
		Kind:      "meshclaw_automation_ui_runner_request_calendar",
		Action:    "ui_runner_request_calendar",
		Command:   []string{"POST", runnerURL + "/request-calendar"},
		OK:        resp.StatusCode >= 200 && resp.StatusCode < 300,
		Stdout:    strings.TrimSpace(buf.String()),
		CreatedAt: time.Now().UTC(),
	}
	if !result.OK || strings.Contains(result.Stdout, `"ok":false`) {
		result.OK = false
		result.Error = firstNonEmpty(uiRunnerError(result.Stdout), "Calendar permission is still not granted")
	}
	return result
}

func UIRunnerRequestContacts(ctx context.Context, runnerURL string) Result {
	if runnerURL == "" {
		runnerURL = defaultUIRunnerURL()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, runnerURL+"/request-contacts", nil)
	if err != nil {
		return failed("meshclaw_automation_ui_runner_request_contacts", err.Error())
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return failed("meshclaw_automation_ui_runner_request_contacts", err.Error())
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	result := Result{
		Kind:      "meshclaw_automation_ui_runner_request_contacts",
		Action:    "ui_runner_request_contacts",
		Command:   []string{"POST", runnerURL + "/request-contacts"},
		OK:        resp.StatusCode >= 200 && resp.StatusCode < 300,
		Stdout:    strings.TrimSpace(buf.String()),
		CreatedAt: time.Now().UTC(),
	}
	if !result.OK || strings.Contains(result.Stdout, `"ok":false`) {
		result.OK = false
		result.Error = firstNonEmpty(uiRunnerError(result.Stdout), "Contacts permission is still not granted")
	}
	return result
}

func ShortcutRun(ctx context.Context, name, input string) Result {
	name = strings.TrimSpace(name)
	if name == "" {
		return failed("meshclaw_automation_shortcut_run", "shortcut name is required")
	}
	args := []string{"run", name}
	if strings.TrimSpace(input) != "" {
		args = append(args, "-i", input)
	}
	return run(ctx, "meshclaw_automation_shortcut_run", "shortcuts", args...)
}

func OpenURL(ctx context.Context, rawURL string) Result {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return failed("meshclaw_automation_open_url", "url is required")
	}
	switch runtime.GOOS {
	case "darwin":
		return run(ctx, "meshclaw_automation_open_url", "open", rawURL)
	case "linux":
		return run(ctx, "meshclaw_automation_open_url", "xdg-open", rawURL)
	default:
		return failed("meshclaw_automation_open_url", "open-url is only implemented for macOS and Linux")
	}
}

func CloseBrowserTabsForHosts(ctx context.Context, hosts []string) Result {
	hosts = normalizeBrowserTabCleanupHosts(hosts)
	if len(hosts) == 0 {
		return failed("meshclaw_automation_browser_tab_cleanup", "at least one host is required")
	}
	if runtime.GOOS != "darwin" {
		return failed("meshclaw_automation_browser_tab_cleanup", "browser tab cleanup is only implemented for macOS")
	}
	return run(ctx, "meshclaw_automation_browser_tab_cleanup", "osascript", "-e", browserTabCleanupAppleScript(hosts))
}

func normalizeBrowserTabCleanupHosts(hosts []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, host := range hosts {
		host = strings.ToLower(strings.TrimSpace(host))
		if parsed, err := url.Parse(host); err == nil && parsed.Hostname() != "" {
			host = parsed.Hostname()
		}
		if cut := strings.IndexAny(host, "/?#"); cut >= 0 {
			host = host[:cut]
		}
		host = strings.TrimPrefix(strings.TrimPrefix(host, "*."), ".")
		if host == "" || strings.ContainsAny(host, " \t\r\n\"'") || seen[host] {
			continue
		}
		seen[host] = true
		out = append(out, host)
	}
	sort.Strings(out)
	return out
}

func browserTabCleanupAppleScript(hosts []string) string {
	quotedHosts := make([]string, 0, len(hosts))
	for _, host := range hosts {
		quotedHosts = append(quotedHosts, appleScriptString(host))
	}
	return fmt.Sprintf(`global meshclawTargetHosts, meshclawClosedTabs
set meshclawTargetHosts to {%s}
set meshclawClosedTabs to 0

on meshclawURLMatches(theURL)
	global meshclawTargetHosts
	set u to theURL as text
	if u is "" then return false
	set currentHost to my meshclawHostFromURL(u)
	if currentHost is "" then return false
	repeat with hostItem in meshclawTargetHosts
		set h to hostItem as text
		if currentHost is h then return true
		if currentHost ends with "." & h then return true
	end repeat
	return false
end meshclawURLMatches

on meshclawHostFromURL(theURL)
	set oldDelimiters to AppleScript's text item delimiters
	try
		set AppleScript's text item delimiters to "://"
		set parts to text items of theURL
		if (count of parts) < 2 then
			set AppleScript's text item delimiters to oldDelimiters
			return ""
		end if
		set remainderText to item 2 of parts
		set AppleScript's text item delimiters to {"/", "?", "#", ":"}
		set hostText to item 1 of text items of remainderText
		set AppleScript's text item delimiters to oldDelimiters
		return hostText
	on error
		set AppleScript's text item delimiters to oldDelimiters
		return ""
	end try
end meshclawHostFromURL

if application "Safari" is running then
	tell application "Safari"
		repeat with w in windows
			try
				set tabList to tabs of w
				repeat with t in tabList
					try
						if my meshclawURLMatches(URL of t) then
							close t
							set meshclawClosedTabs to meshclawClosedTabs + 1
						end if
					end try
				end repeat
			end try
		end repeat
	end tell
end if

if application "Google Chrome" is running then
	tell application "Google Chrome"
		repeat with w in windows
			try
				set tabList to tabs of w
				repeat with t in tabList
					try
						if my meshclawURLMatches(URL of t) then
							close t
							set meshclawClosedTabs to meshclawClosedTabs + 1
						end if
					end try
				end repeat
			end try
		end repeat
	end tell
end if

return "closed_tabs=" & meshclawClosedTabs
`, strings.Join(quotedHosts, ", "))
}

func SetClipboard(ctx context.Context, text string) Result {
	if strings.TrimSpace(text) == "" {
		return failed("meshclaw_automation_clipboard_set", "clipboard text is required")
	}
	switch runtime.GOOS {
	case "darwin":
		return runWithInput(ctx, "meshclaw_automation_clipboard_set", text, "pbcopy")
	case "linux":
		if _, err := exec.LookPath("wl-copy"); err == nil {
			return runWithInput(ctx, "meshclaw_automation_clipboard_set", text, "wl-copy")
		}
		if _, err := exec.LookPath("xclip"); err == nil {
			return runWithInput(ctx, "meshclaw_automation_clipboard_set", text, "xclip", "-selection", "clipboard")
		}
		return failed("meshclaw_automation_clipboard_set", "no clipboard command found: install wl-copy or xclip")
	default:
		return failed("meshclaw_automation_clipboard_set", "clipboard is only implemented for macOS and Linux")
	}
}

func UIRunnerKey(ctx context.Context, key string, modifiers ...string) Result {
	payload := map[string]interface{}{"key": key, "modifiers": modifiers}
	return postUIRunnerCommand(ctx, "/key", "meshclaw_automation_ui_key", payload)
}

func UIRunnerTypeText(ctx context.Context, text string) Result {
	payload := map[string]interface{}{"text": text}
	return postUIRunnerCommand(ctx, "/type-text", "meshclaw_automation_ui_type_text", payload)
}

func postUIRunnerCommand(ctx context.Context, path, kind string, payload map[string]interface{}) Result {
	runnerURL := strings.TrimRight(defaultUIRunnerURL(), "/")
	data, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, runnerURL+path, bytes.NewReader(data))
	if err != nil {
		return failed(kind, err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return failed(kind, err.Error())
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	result := Result{
		Kind:      kind,
		Action:    strings.TrimPrefix(strings.TrimPrefix(kind, "meshclaw_automation_"), "ui_"),
		Command:   []string{"POST", runnerURL + path},
		OK:        resp.StatusCode >= 200 && resp.StatusCode < 300,
		Stdout:    strings.TrimSpace(buf.String()),
		CreatedAt: time.Now().UTC(),
	}
	if !result.OK || strings.Contains(result.Stdout, `"ok":false`) {
		result.OK = false
		statusError := ""
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			statusError = fmt.Sprintf("ui runner command failed: %s", resp.Status)
		}
		result.Error = firstNonEmpty(uiRunnerError(result.Stdout), statusError, "ui runner command failed")
	}
	return result
}

func uiRunnerError(stdout string) string {
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		return ""
	}
	return payload.Error
}

func OpenApp(ctx context.Context, name string) Result {
	name = strings.TrimSpace(name)
	if name == "" {
		return failed("meshclaw_automation_open_app", "app name is required")
	}
	switch runtime.GOOS {
	case "darwin":
		return run(ctx, "meshclaw_automation_open_app", "open", "-a", name)
	case "linux":
		return failed("meshclaw_automation_open_app", "open-app is only implemented for macOS")
	default:
		return failed("meshclaw_automation_open_app", "open-app is only implemented for macOS")
	}
}

func UIRunnerCreateReminder(ctx context.Context, title, notes, dueISO string) Result {
	payload := map[string]interface{}{
		"title": strings.TrimSpace(title),
		"notes": strings.TrimSpace(notes),
		"due":   strings.TrimSpace(dueISO),
	}
	result := postUIRunnerCommand(ctx, "/reminder/create", "meshclaw_automation_reminder_create", payload)
	result.Action = "reminder_create"
	return result
}

func UIRunnerListReminders(ctx context.Context, startISO, endISO, query string) Result {
	payload := map[string]interface{}{
		"start": strings.TrimSpace(startISO),
		"end":   strings.TrimSpace(endISO),
		"query": strings.TrimSpace(query),
	}
	result := postUIRunnerCommand(ctx, "/reminder/list", "meshclaw_automation_reminders_list", payload)
	result.Action = "reminders_list"
	return result
}

func UIRunnerMutateReminder(ctx context.Context, action, id, query string) Result {
	path := "/reminder/" + action
	kind := "meshclaw_automation_reminder_" + action
	payload := map[string]interface{}{
		"id":    strings.TrimSpace(id),
		"query": strings.TrimSpace(query),
		"title": strings.TrimSpace(query),
	}
	result := postUIRunnerCommand(ctx, path, kind, payload)
	result.Action = "reminder_" + action
	return result
}

func UIRunnerCreateCalendarEvent(ctx context.Context, title, notes, startISO, endISO string) Result {
	payload := map[string]interface{}{
		"title": strings.TrimSpace(title),
		"notes": strings.TrimSpace(notes),
		"start": strings.TrimSpace(startISO),
		"end":   strings.TrimSpace(endISO),
	}
	result := postUIRunnerCommand(ctx, "/calendar/create", "meshclaw_automation_calendar_event_create", payload)
	result.Action = "calendar_event_create"
	return result
}

func UIRunnerListCalendarEvents(ctx context.Context, startISO, endISO, query string) Result {
	payload := map[string]interface{}{
		"start": strings.TrimSpace(startISO),
		"end":   strings.TrimSpace(endISO),
		"query": strings.TrimSpace(query),
	}
	result := postUIRunnerCommand(ctx, "/calendar/list", "meshclaw_automation_calendar_events_list", payload)
	result.Action = "calendar_events_list"
	return result
}

func UIRunnerDeleteCalendarEvent(ctx context.Context, id, query, startISO, endISO string) Result {
	payload := map[string]interface{}{
		"id":    strings.TrimSpace(id),
		"query": strings.TrimSpace(query),
		"title": strings.TrimSpace(query),
		"start": strings.TrimSpace(startISO),
		"end":   strings.TrimSpace(endISO),
	}
	result := postUIRunnerCommand(ctx, "/calendar/delete", "meshclaw_automation_calendar_event_delete", payload)
	result.Action = "calendar_event_delete"
	return result
}

func UIRunnerSearchContacts(ctx context.Context, query string) Result {
	payload := map[string]interface{}{
		"query": strings.TrimSpace(query),
	}
	result := postUIRunnerCommand(ctx, "/contacts/search", "meshclaw_automation_contacts_search", payload)
	result.Action = "contacts_search"
	return result
}

func CreateNote(ctx context.Context, title, body string) Result {
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	if title == "" {
		title = "Argos Note"
	}
	if body == "" {
		return failed("meshclaw_automation_note_create", "note body is required")
	}
	if runtime.GOOS != "darwin" {
		return failed("meshclaw_automation_note_create", "note creation is only implemented for macOS")
	}
	script := fmt.Sprintf(`tell application "Notes"
	activate
	make new note at default account with properties {name:%s, body:%s}
end tell`, appleScriptString(title), appleScriptString(notesHTMLBody(body)))
	return run(ctx, "meshclaw_automation_note_create", "osascript", "-e", script)
}

func notesHTMLBody(body string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\r\n", "<br>",
		"\n", "<br>",
		"\r", "<br>",
	)
	return replacer.Replace(body)
}

func SearchNotes(ctx context.Context, query string, limit int) Result {
	query = strings.TrimSpace(query)
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	if runtime.GOOS != "darwin" {
		return failed("meshclaw_automation_notes_search", "notes search is only implemented for macOS")
	}
	script := fmt.Sprintf(`set q to %s
set maxItems to %d
set outText to ""
set foundCount to 0
tell application "Notes"
	set allNotes to notes
	repeat with n in allNotes
		if foundCount is greater than or equal to maxItems then exit repeat
		set noteName to name of n as text
		set noteBody to body of n as text
		set haystack to noteName & " " & noteBody
		if q is "" or haystack contains q then
			set foundCount to foundCount + 1
			if length of noteBody is greater than 220 then
				set noteBody to text 1 thru 220 of noteBody
			end if
			set noteBody to my normalizeLine(noteBody)
			set outText to outText & foundCount & ". " & noteName & " ||| " & noteBody & linefeed
		end if
	end repeat
end tell
return outText

on normalizeLine(t)
	set AppleScript's text item delimiters to {linefeed, return, tab}
	set parts to text items of t
	set AppleScript's text item delimiters to " "
	set joined to parts as text
	set AppleScript's text item delimiters to ""
	return joined
end normalizeLine`, appleScriptString(query), limit)
	result := run(ctx, "meshclaw_automation_notes_search", "osascript", "-e", script)
	result.Action = "notes_search"
	return result
}

func CreateCalendarEvent(ctx context.Context, title, notes, startISO, endISO string) Result {
	title = strings.TrimSpace(title)
	notes = strings.TrimSpace(notes)
	startISO = strings.TrimSpace(startISO)
	endISO = strings.TrimSpace(endISO)
	if title == "" {
		return failed("meshclaw_automation_calendar_event_create", "calendar event title is required")
	}
	if startISO == "" {
		return failed("meshclaw_automation_calendar_event_create", "calendar event start time is required")
	}
	if runtime.GOOS != "darwin" {
		return failed("meshclaw_automation_calendar_event_create", "calendar event creation is only implemented for macOS")
	}
	native := UIRunnerCreateCalendarEvent(ctx, title, notes, startISO, endISO)
	if native.OK || !uiRunnerCalendarUnavailable(native) {
		return native
	}
	return failed("meshclaw_automation_calendar_event_create", "Argos Calendar helper is not available")
}

func ListCalendarEvents(ctx context.Context, startISO, endISO, query string) Result {
	if strings.TrimSpace(startISO) == "" || strings.TrimSpace(endISO) == "" {
		return failed("meshclaw_automation_calendar_events_list", "calendar list start and end time are required")
	}
	if runtime.GOOS != "darwin" {
		return failed("meshclaw_automation_calendar_events_list", "calendar listing is only implemented for macOS")
	}
	result := UIRunnerListCalendarEvents(ctx, startISO, endISO, query)
	if result.OK || !uiRunnerCalendarUnavailable(result) {
		return result
	}
	return failed("meshclaw_automation_calendar_events_list", "Argos Calendar helper is not available")
}

func DeleteCalendarEvent(ctx context.Context, id, query, startISO, endISO string) Result {
	if strings.TrimSpace(id) == "" && strings.TrimSpace(query) == "" {
		return failed("meshclaw_automation_calendar_event_delete", "calendar event id or query is required")
	}
	if strings.TrimSpace(startISO) == "" || strings.TrimSpace(endISO) == "" {
		return failed("meshclaw_automation_calendar_event_delete", "calendar event delete start and end time are required")
	}
	if runtime.GOOS != "darwin" {
		return failed("meshclaw_automation_calendar_event_delete", "calendar deletion is only implemented for macOS")
	}
	result := UIRunnerDeleteCalendarEvent(ctx, id, query, startISO, endISO)
	if result.OK || !uiRunnerCalendarUnavailable(result) {
		return result
	}
	return failed("meshclaw_automation_calendar_event_delete", "Argos Calendar helper is not available")
}

func SearchContacts(ctx context.Context, query string) Result {
	if strings.TrimSpace(query) == "" {
		return failed("meshclaw_automation_contacts_search", "contacts search query is required")
	}
	if runtime.GOOS != "darwin" {
		return failed("meshclaw_automation_contacts_search", "contacts search is only implemented for macOS")
	}
	result := UIRunnerSearchContacts(ctx, query)
	if result.OK || !uiRunnerContactsUnavailable(result) {
		return result
	}
	return failed("meshclaw_automation_contacts_search", "Argos Contacts helper is not available")
}

func CreateReminder(ctx context.Context, title, notes, dueISO string) Result {
	title = strings.TrimSpace(title)
	notes = strings.TrimSpace(notes)
	dueISO = strings.TrimSpace(dueISO)
	if title == "" {
		return failed("meshclaw_automation_reminder_create", "reminder title is required")
	}
	if runtime.GOOS != "darwin" {
		return failed("meshclaw_automation_reminder_create", "reminder creation is only implemented for macOS")
	}
	native := UIRunnerCreateReminder(ctx, title, notes, dueISO)
	if native.OK || !uiRunnerReminderUnavailable(native) {
		return native
	}
	if shortcut := reminderShortcutName(ctx); shortcut != "" {
		input := reminderShortcutInput(title, notes, dueISO)
		result := ShortcutRun(ctx, shortcut, input)
		result.Kind = "meshclaw_automation_reminder_create"
		result.Action = "reminder_create"
		return result
	}
	props := "{name:" + appleScriptString(title)
	if notes != "" {
		props += ", body:" + appleScriptString(notes)
	}
	dueScript := ""
	if due, err := time.Parse(time.RFC3339, dueISO); err == nil {
		due = due.Local()
		dueScript = strings.Join([]string{
			"set dueDate to current date",
			fmt.Sprintf("set year of dueDate to %d", due.Year()),
			fmt.Sprintf("set month of dueDate to %s", appleScriptMonthName(due.Month())),
			fmt.Sprintf("set day of dueDate to %d", due.Day()),
			fmt.Sprintf("set time of dueDate to %d", due.Hour()*3600+due.Minute()*60+due.Second()),
		}, "\n") + "\n"
		props += ", remind me date:dueDate"
	}
	props += "}"
	script := dueScript + fmt.Sprintf(`tell application "Reminders"
	activate
	tell default list
		make new reminder with properties %s
	end tell
end tell`, props)
	runCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	result := run(runCtx, "meshclaw_automation_reminder_create", "osascript", "-e", script)
	if runCtx.Err() != nil && result.Error != "" {
		result.Error = "Reminders automation timed out. macOS may be waiting for Automation permission for Terminal/meshclaw to control Reminders."
	}
	return result
}

func ListReminders(ctx context.Context, startISO, endISO, query string) Result {
	if strings.TrimSpace(startISO) == "" || strings.TrimSpace(endISO) == "" {
		return failed("meshclaw_automation_reminders_list", "reminder list start and end time are required")
	}
	if runtime.GOOS != "darwin" {
		return failed("meshclaw_automation_reminders_list", "reminder listing is only implemented for macOS")
	}
	result := UIRunnerListReminders(ctx, startISO, endISO, query)
	if result.OK || !uiRunnerReminderUnavailable(result) {
		return result
	}
	return failed("meshclaw_automation_reminders_list", "Argos Reminders helper is not available")
}

func MutateReminder(ctx context.Context, action, id, query string) Result {
	action = strings.TrimSpace(action)
	if action != "complete" && action != "delete" {
		return failed("meshclaw_automation_reminder_"+action, "unsupported reminder mutation")
	}
	if strings.TrimSpace(id) == "" && strings.TrimSpace(query) == "" {
		return failed("meshclaw_automation_reminder_"+action, "reminder id or query is required")
	}
	if runtime.GOOS != "darwin" {
		return failed("meshclaw_automation_reminder_"+action, "reminder mutation is only implemented for macOS")
	}
	result := UIRunnerMutateReminder(ctx, action, id, query)
	if result.OK || !uiRunnerReminderUnavailable(result) {
		return result
	}
	return failed("meshclaw_automation_reminder_"+action, "Argos Reminders helper is not available")
}

func uiRunnerCalendarUnavailable(result Result) bool {
	text := strings.ToLower(strings.TrimSpace(result.Error + "\n" + result.Stderr + "\n" + result.Stdout))
	if text == "" {
		return true
	}
	return strings.Contains(text, "connection refused") ||
		strings.Contains(text, "no such host") ||
		strings.Contains(text, "operation timed out") ||
		strings.Contains(text, "404") ||
		strings.Contains(text, "not found")
}

func uiRunnerContactsUnavailable(result Result) bool {
	text := strings.ToLower(strings.TrimSpace(result.Error + "\n" + result.Stderr + "\n" + result.Stdout))
	if text == "" {
		return true
	}
	return strings.Contains(text, "connection refused") ||
		strings.Contains(text, "no such host") ||
		strings.Contains(text, "operation timed out") ||
		strings.Contains(text, "404") ||
		strings.Contains(text, "not found")
}

func uiRunnerReminderUnavailable(result Result) bool {
	text := strings.ToLower(strings.TrimSpace(result.Error + "\n" + result.Stderr + "\n" + result.Stdout))
	if text == "" {
		return true
	}
	return strings.Contains(text, "connection refused") ||
		strings.Contains(text, "no such host") ||
		strings.Contains(text, "operation timed out") ||
		strings.Contains(text, "404") ||
		strings.Contains(text, "not found")
}

func reminderShortcutName(ctx context.Context) string {
	candidates := reminderShortcutCandidates()
	list := ShortcutsList(ctx)
	if !list.OK {
		return ""
	}
	available := map[string]string{}
	for _, line := range strings.Split(list.Stdout, "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		available[strings.ToLower(name)] = name
	}
	for _, candidate := range candidates {
		if name := available[strings.ToLower(strings.TrimSpace(candidate))]; name != "" {
			return name
		}
	}
	return ""
}

func reminderShortcutCandidates() []string {
	configured := strings.TrimSpace(os.Getenv("MESHCLAW_ARGOS_REMINDER_SHORTCUT"))
	candidates := []string{}
	if configured != "" {
		candidates = append(candidates, configured)
	}
	return append(candidates, "Argos Add Reminder", "Argos Reminder", "MeshClaw Add Reminder", "리마인더 추가")
}

func ReminderShortcutDoctor(ctx context.Context) ReminderShortcutReport {
	exampleInput := reminderShortcutInput("우유 사기", "Signal 요청", "2026-05-25T09:00:00+09:00")
	report := ReminderShortcutReport{
		Kind:          "meshclaw_argos_reminder_shortcut",
		OK:            false,
		ExpectedNames: reminderShortcutCandidates(),
		InputSchema:   `{"title":"string","notes":"string","due":"RFC3339 optional"}`,
		ExampleInput:  exampleInput,
		TestCommand:   "meshclaw automation shortcut-run 'Argos Add Reminder' --input '" + exampleInput + "' --json",
		SetupSteps: []string{
			"`INSTALL=1 ARGOS_RUNNER_REPLACE=1 scripts/build-argos-ui-runner-app.sh`로 Argos UI Runner를 설치/갱신하세요.",
			"Argos UI Runner.app을 한 번 열고 Reminders 접근 요청이 나오면 허용하세요.",
			"그 다음 Signal에서 같은 리마인더 명령을 다시 보내세요.",
		},
		CreatedAt: time.Now().UTC(),
	}
	health := UIRunnerHealth(ctx, defaultUIRunnerURL())
	if strings.Contains(health.Stdout, `"reminder_create":true`) {
		report.OK = true
		report.Installed = true
		report.ShortcutName = "Argos UI Runner reminder helper"
		report.SetupSteps = nil
		report.TestCommand = "meshclaw argos ask '내일 오전 9시에 우유 사기 리마인더 추가해줘' --execute --json"
		return report
	}
	list := ShortcutsList(ctx)
	if !list.OK {
		report.Error = firstNonEmpty(health.Error, list.Error, list.Stderr, "Argos reminder helper is not installed")
		return report
	}
	if name := reminderShortcutName(ctx); name != "" {
		report.OK = true
		report.Installed = true
		report.ShortcutName = name
		report.SetupSteps = nil
		report.TestCommand = "meshclaw automation shortcut-run '" + name + "' --input '" + exampleInput + "' --json"
		return report
	}
	return report
}

func CalendarAutomationDoctor(ctx context.Context) CalendarAutomationReport {
	exampleInput := `{"title":"Argos 테스트 회의","start":"2026-05-25T15:00:00+09:00","end":"2026-05-25T16:00:00+09:00","notes":"Signal 요청"}`
	report := CalendarAutomationReport{
		Kind:         "meshclaw_argos_calendar_automation",
		OK:           false,
		InputSchema:  `{"title":"string","notes":"string","start":"RFC3339","end":"RFC3339 optional"}`,
		ExampleInput: exampleInput,
		TestCommand:  "meshclaw argos ask '내일 오후 3시에 Argos 테스트 회의 일정 추가해줘' --execute --json",
		SetupSteps: []string{
			"`INSTALL=1 ARGOS_RUNNER_REPLACE=1 scripts/build-argos-ui-runner-app.sh`로 Argos UI Runner를 설치/갱신하세요.",
			"Argos UI Runner.app을 한 번 열고 Calendar 접근 요청이 나오면 허용하세요.",
			"그 다음 Signal에서 같은 캘린더 명령을 다시 보내세요.",
		},
		CreatedAt: time.Now().UTC(),
	}
	health := UIRunnerHealth(ctx, defaultUIRunnerURL())
	if strings.Contains(health.Stdout, `"calendar_create":true`) {
		report.OK = true
		report.Installed = true
		report.Provider = "Argos UI Runner calendar helper"
		report.SetupSteps = nil
		return report
	}
	report.Error = firstNonEmpty(health.Error, "Argos calendar helper is not installed")
	return report
}

func ContactsAutomationDoctor(ctx context.Context) ContactsAutomationReport {
	report := ContactsAutomationReport{
		Kind:         "meshclaw_argos_contacts_automation",
		OK:           false,
		InputSchema:  `{"query":"string"}`,
		ExampleInput: `{"query":"홍길동"}`,
		TestCommand:  "meshclaw argos ask '연락처에서 홍길동 전화번호 찾아줘' --execute --json",
		SetupSteps: []string{
			"`INSTALL=1 ARGOS_RUNNER_REPLACE=1 scripts/build-argos-ui-runner-app.sh`로 Argos UI Runner를 설치/갱신하세요.",
			"Argos UI Runner.app을 한 번 열고 Contacts 접근 요청이 나오면 허용하세요.",
			"그 다음 Signal에서 같은 연락처 명령을 다시 보내세요.",
		},
		CreatedAt: time.Now().UTC(),
	}
	health := UIRunnerHealth(ctx, defaultUIRunnerURL())
	if strings.Contains(health.Stdout, `"contacts_search":true`) {
		report.OK = true
		report.Installed = true
		report.Provider = "Argos UI Runner contacts helper"
		report.SetupSteps = nil
		return report
	}
	if staged := findStagedUIRunner(); staged != "" {
		helper := filepath.Join(staged, "Contents", "MacOS", "argos-contacts-helper")
		if st, err := os.Stat(helper); err == nil && !st.IsDir() {
			report.Provider = "staged Argos UI Runner contacts helper"
			report.Error = "Argos contacts helper is staged but not active; apply the staged Runner while the user is present to approve macOS Contacts permission."
			report.SetupSteps = []string{
				"Apply " + staged + " only while the user is present.",
				"Open Argos UI Runner.app and approve Contacts access when macOS asks.",
				"Then run `meshclaw argos contacts-setup --json` again.",
			}
			return report
		}
	}
	report.Error = firstNonEmpty(health.Error, "Argos contacts helper is not installed")
	return report
}

func reminderShortcutInput(title, notes, dueISO string) string {
	payload := map[string]string{
		"title": strings.TrimSpace(title),
		"notes": strings.TrimSpace(notes),
		"due":   strings.TrimSpace(dueISO),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return strings.TrimSpace(title)
	}
	return string(data)
}

func appleScriptMonthName(month time.Month) string {
	switch month {
	case time.January:
		return "January"
	case time.February:
		return "February"
	case time.March:
		return "March"
	case time.April:
		return "April"
	case time.May:
		return "May"
	case time.June:
		return "June"
	case time.July:
		return "July"
	case time.August:
		return "August"
	case time.September:
		return "September"
	case time.October:
		return "October"
	case time.November:
		return "November"
	case time.December:
		return "December"
	default:
		return "January"
	}
}

func ArgosDo(ctx context.Context, req ArgosRequest) ArgosAction {
	action := ClassifyArgosAction(req.Text)
	action.Executed = false
	if !req.Execute {
		action.NextActions = append(action.NextActions, "실행하려면 --execute를 붙이거나 Signal에서 명확히 실행 요청을 보내세요.")
		return action
	}
	action.Executed = true
	var recording *exec.Cmd
	if req.RecordSeconds > 0 {
		recordPath := req.RecordPath
		if strings.TrimSpace(recordPath) == "" {
			recordPath = defaultRecordingPath()
		}
		action.RecordingPath = recordPath
		recording = startScreenRecording(ctx, req.RecordSeconds, recordPath, &action)
		if recording != nil {
			time.Sleep(recordingPrerollDelay())
		}
	}
	switch action.Action {
	case "browser_search":
		result, err := browserauto.Search(ctx, browserauto.SearchOptions{Query: action.Query, Limit: 5, Timeout: 20})
		action.Search = &result
		if err != nil {
			action.Error = err.Error()
		}
		if action.Error == "" {
			if doc, err := saveBrowserSearchDocument(ctx, action.Query, result); err == nil {
				action.OutputPath = doc.Path
			}
		}
	case "visible_browser_search":
		result := OpenBrowserSearch(ctx, action.Query)
		action.Result = &result
		action.Error = result.Error
		search, err := browserauto.Search(ctx, browserauto.SearchOptions{Query: action.Query, Limit: 5, Timeout: 20})
		action.Search = &search
		if err != nil && action.Error == "" {
			action.Error = err.Error()
		}
		if action.Error == "" {
			if doc, err := saveBrowserSearchDocument(ctx, action.Query, search); err == nil {
				action.OutputPath = doc.Path
			}
		}
	case "work_demo":
		result := WorkDemo(ctx, action.Query)
		action.Result = &result
		action.OutputPath = result.URL
		action.Error = result.Error
	case "browser_fetch":
		page, err := browserauto.Fetch(ctx, browserauto.FetchOptions{URL: action.URL, MaxBody: 6000, Timeout: 20})
		action.Page = &page
		if err != nil {
			action.Error = err.Error()
		}
		if action.Error == "" {
			if doc, err := saveBrowserFetchDocument(ctx, page); err == nil {
				action.OutputPath = doc.Path
			}
		}
	case "document_create":
		result := CreateArgosDocument(ctx, action.NoteTitle, action.NoteBody)
		action.Result = &result
		action.OutputPath = result.URL
		action.Error = result.Error
	case "mac_runner_command":
		result := RunMacRunnerCommand(ctx, action.Input)
		action.Result = &result
		action.OutputPath = firstNonEmpty(result.URL, result.Preview)
		action.Error = result.Error
	case "mac_runner_doctor":
		report := DoctorMacRunner(ctx, action.Input)
		payload, _ := json.Marshal(report)
		result := Result{
			Kind:      report.Kind,
			Action:    "mac_runner_doctor",
			OK:        report.OK,
			Stdout:    string(payload),
			CreatedAt: report.CreatedAt,
		}
		if !report.OK {
			result.Error = strings.Join(report.Problems, "; ")
		}
		action.Result = &result
		action.Error = result.Error
		action.NextActions = append(action.NextActions, report.NextActions...)
	case "open_url":
		result := runMacRunnerCommandOrLocal(ctx, action, func() Result { return OpenURL(ctx, action.URL) })
		action.Result = &result
		action.Error = result.Error
	case "open_app":
		result := runMacRunnerCommandOrLocal(ctx, action, func() Result { return OpenApp(ctx, action.App) })
		action.Result = &result
		action.Error = result.Error
	case "shortcut_run":
		result := runMacRunnerCommandOrLocal(ctx, action, func() Result { return ShortcutRun(ctx, action.Shortcut, action.Input) })
		action.Result = &result
		action.Error = result.Error
	case "note_create":
		result := runMacRunnerCommandOrLocal(ctx, action, func() Result { return CreateNote(ctx, action.NoteTitle, action.NoteBody) })
		action.Result = &result
		action.Error = result.Error
	case "reminder_create":
		result := runMacRunnerCommandOrLocal(ctx, action, func() Result {
			return CreateReminder(ctx, action.ReminderTitle, action.ReminderNotes, action.ReminderDue)
		})
		action.Result = &result
		action.Error = result.Error
	case "reminders_list":
		result := runMacRunnerCommandOrLocal(ctx, action, func() Result {
			return ListReminders(ctx, action.ReminderStart, action.ReminderEnd, action.ReminderQuery)
		})
		action.Result = &result
		action.Error = result.Error
	case "reminder_complete":
		result := runMacRunnerCommandOrLocal(ctx, action, func() Result {
			return MutateReminder(ctx, "complete", action.ReminderID, action.ReminderQuery)
		})
		action.Result = &result
		action.Error = result.Error
	case "reminder_delete":
		result := runMacRunnerCommandOrLocal(ctx, action, func() Result {
			return MutateReminder(ctx, "delete", action.ReminderID, action.ReminderQuery)
		})
		action.Result = &result
		action.Error = result.Error
	case "calendar_event_create":
		result := runMacRunnerCommandOrLocal(ctx, action, func() Result {
			return CreateCalendarEvent(ctx, action.CalendarTitle, action.CalendarNotes, action.CalendarStart, action.CalendarEnd)
		})
		action.Result = &result
		action.Error = result.Error
	case "calendar_events_list":
		result := runMacRunnerCommandOrLocal(ctx, action, func() Result {
			return ListCalendarEvents(ctx, action.CalendarStart, action.CalendarEnd, action.CalendarQuery)
		})
		action.Result = &result
		action.Error = result.Error
	case "calendar_event_delete":
		result := runMacRunnerCommandOrLocal(ctx, action, func() Result {
			return DeleteCalendarEvent(ctx, action.CalendarID, action.CalendarQuery, action.CalendarStart, action.CalendarEnd)
		})
		action.Result = &result
		action.Error = result.Error
	case "contacts_search":
		result := runMacRunnerCommandOrLocal(ctx, action, func() Result { return SearchContacts(ctx, action.ContactQuery) })
		action.Result = &result
		action.Error = result.Error
	case "clipboard_set":
		result := SetClipboard(ctx, action.Input)
		action.Result = &result
		action.Error = result.Error
	case "ai_handoff":
		result := AIHandoff(ctx, AIHandoffOptions{Provider: action.Provider, Prompt: action.Prompt})
		action.Result = &result
		action.Error = result.Error
	case "help":
		result := Result{
			Kind:      "meshclaw_automation_help",
			Action:    "help",
			OK:        true,
			Stdout:    strings.Join(action.NextActions, "\n"),
			CreatedAt: time.Now().UTC(),
		}
		action.Result = &result
	default:
		action.Error = "unsupported Argos macOS action"
	}
	if recording != nil {
		action.Recording = finishScreenRecording(recording, req.RecordSeconds, action.RecordingPath)
	}
	return action
}

func isArgosHelpRequest(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return false
	}
	if containsAny(lower, []string{
		"뭘 할 수", "뭐 할 수", "무엇을 할 수", "할 수 있는 일", "할수있는 일", "가능한 일",
		"기능 알려", "기능 설명", "사용법", "도움말", "help", "what can you do", "what can i do",
	}) {
		return true
	}
	return (strings.Contains(lower, "할 수") || strings.Contains(lower, "할수")) &&
		containsAny(lower, []string{"있어", "있나", "알려", "말해", "목록", "list"})
}

func ScreenRecord(ctx context.Context, seconds int, output string) Result {
	if seconds <= 0 {
		return failed("meshclaw_automation_screen_record", "record seconds must be positive")
	}
	if strings.TrimSpace(output) == "" {
		output = defaultRecordingPath()
	}
	if runtime.GOOS != "darwin" {
		return failed("meshclaw_automation_screen_record", "screen recording is only implemented for macOS")
	}
	if err := os.MkdirAll(filepath.Dir(output), 0700); err != nil {
		return failed("meshclaw_automation_screen_record", err.Error())
	}
	result := UIRunnerScreenRecord(ctx, defaultUIRunnerURL(), seconds, output)
	if result.OK || result.Stdout != "" || !allowDirectScreenRecordFallback() {
		return result
	}
	return directScreenRecord(ctx, seconds, output)
}

func ScreenCapture(ctx context.Context, output string) Result {
	if strings.TrimSpace(output) == "" {
		output = defaultScreenCapturePath()
	}
	if runtime.GOOS != "darwin" {
		return failed("meshclaw_automation_screen_capture", "screen capture is only implemented for macOS")
	}
	if err := os.MkdirAll(filepath.Dir(output), 0700); err != nil {
		return failed("meshclaw_automation_screen_capture", err.Error())
	}
	result := run(ctx, "meshclaw_automation_screen_capture", "screencapture", "-x", output)
	result.Action = "screen_capture"
	if st, statErr := os.Stat(output); statErr == nil && !st.IsDir() && st.Size() > 0 {
		result.URL = output
		result.OK = true
		annotateRecordingResult(&result, output, st)
	} else if allowUIRunnerScreenCaptureFallback() {
		fallback := uiRunnerScreenCaptureThumbnail(ctx, output)
		if fallback.OK {
			if strings.TrimSpace(result.Stderr) != "" {
				fallback.Stderr = strings.TrimSpace(result.Stderr)
			}
			if strings.TrimSpace(result.Error) != "" {
				fallback.Preview = "direct screencapture failed: " + strings.TrimSpace(result.Error)
			}
			return fallback
		}
		if result.Error == "" {
			result.Error = firstNonEmpty(fallback.Error, "screen capture failed")
		}
	} else if result.OK {
		result.OK = false
		result.Error = "screen capture finished but no output file was created"
	}
	return result
}

func allowUIRunnerScreenCaptureFallback() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("MESHCLAW_ARGOS_UI_RUNNER_CAPTURE_FALLBACK")))
	return value != "0" && value != "false" && value != "no" && value != "off"
}

func uiRunnerScreenCaptureThumbnail(ctx context.Context, output string) Result {
	if strings.TrimSpace(output) == "" {
		output = defaultScreenCapturePath()
	}
	if err := os.MkdirAll(filepath.Dir(output), 0700); err != nil {
		return failed("meshclaw_automation_screen_capture", err.Error())
	}
	base := strings.TrimSuffix(output, filepath.Ext(output))
	video := base + ".fallback.mp4"
	record := UIRunnerScreenRecord(ctx, "", 1, video)
	if !record.OK {
		errText := firstNonEmpty(record.Error, record.Stdout, "ui runner screen record failed")
		return failed("meshclaw_automation_screen_capture", errText)
	}
	thumbDir, err := os.MkdirTemp(filepath.Dir(output), "meshclaw-capture-thumb-*")
	if err != nil {
		return failed("meshclaw_automation_screen_capture", err.Error())
	}
	defer os.RemoveAll(thumbDir)
	cmd := exec.CommandContext(ctx, "qlmanage", "-t", "-s", "1440", "-o", thumbDir, video)
	qlOut, qlErr := cmd.CombinedOutput()
	if qlErr != nil {
		return failed("meshclaw_automation_screen_capture", strings.TrimSpace(string(qlOut)+"\n"+qlErr.Error()))
	}
	thumb, err := newestPNGInDir(thumbDir)
	if err != nil {
		return failed("meshclaw_automation_screen_capture", err.Error())
	}
	if err := moveOrCopyFile(thumb, output); err != nil {
		return failed("meshclaw_automation_screen_capture", err.Error())
	}
	_ = os.Chmod(output, 0600)
	result := Result{
		Kind:      "meshclaw_automation_screen_capture",
		Action:    "screen_capture",
		Command:   []string{"POST", defaultUIRunnerURL() + "/screen-record", "qlmanage", "-t", "-s", "1440", "-o", thumbDir, video},
		OK:        true,
		URL:       output,
		Stdout:    strings.TrimSpace("ui_runner_screen_record_thumbnail\n" + record.Stdout),
		CreatedAt: time.Now().UTC(),
	}
	if st, statErr := os.Stat(output); statErr == nil && !st.IsDir() && st.Size() > 0 {
		annotateRecordingResult(&result, output, st)
	}
	return result
}

func newestPNGInDir(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	type candidate struct {
		path string
		mod  time.Time
		size int64
	}
	var best candidate
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".png") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		current := candidate{path: filepath.Join(dir, entry.Name()), mod: info.ModTime(), size: info.Size()}
		if best.path == "" || current.mod.After(best.mod) || (current.mod.Equal(best.mod) && current.size > best.size) {
			best = current
		}
	}
	if best.path == "" {
		return "", fmt.Errorf("qlmanage did not create a PNG thumbnail")
	}
	return best.path, nil
}

func moveOrCopyFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func startScreenRecording(ctx context.Context, seconds int, output string, action *ArgosAction) *exec.Cmd {
	if runtime.GOOS != "darwin" {
		action.Recording = ptrResult(failed("meshclaw_automation_screen_record", "screen recording is only implemented for macOS"))
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(output), 0700); err != nil {
		action.Recording = ptrResult(failed("meshclaw_automation_screen_record", err.Error()))
		return nil
	}
	payload := fmt.Sprintf(`{"seconds":%d,"output":%s}`, seconds, jsonString(output))
	cmd := exec.CommandContext(ctx, "curl", "-sS", "-X", "POST", "-H", "Content-Type: application/json", "--data", payload, defaultUIRunnerURL()+"/screen-record")
	if err := cmd.Start(); err != nil {
		action.Recording = ptrResult(failed("meshclaw_automation_screen_record", err.Error()))
		return nil
	}
	return cmd
}

func finishScreenRecording(cmd *exec.Cmd, seconds int, output string) *Result {
	err := cmd.Wait()
	result := Result{
		Kind:      "meshclaw_automation_screen_record",
		Action:    "screen_record",
		Command:   append([]string(nil), cmd.Args...),
		OK:        false,
		CreatedAt: time.Now().UTC(),
	}
	if err != nil {
		result.Error = err.Error()
	}
	if st, statErr := os.Stat(output); statErr == nil && !st.IsDir() && st.Size() > 0 {
		result.URL = output
		result.OK = true
		annotateRecordingResult(&result, output, st)
	} else if result.Error == "" {
		result.Error = "screen recording finished but no output file was created"
	}
	return &result
}

func UIRunnerScreenRecord(ctx context.Context, runnerURL string, seconds int, output string) Result {
	runnerURL = strings.TrimRight(strings.TrimSpace(runnerURL), "/")
	if runnerURL == "" {
		runnerURL = defaultUIRunnerURL()
	}
	payload := map[string]interface{}{"seconds": seconds, "output": output}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, runnerURL+"/screen-record", bytes.NewReader(data))
	if err != nil {
		return failed("meshclaw_automation_screen_record", err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return failed("meshclaw_automation_screen_record", err.Error())
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	result := Result{
		Kind:      "meshclaw_automation_screen_record",
		Action:    "screen_record",
		Command:   []string{"POST", runnerURL + "/screen-record"},
		OK:        resp.StatusCode >= 200 && resp.StatusCode < 300,
		Stdout:    strings.TrimSpace(buf.String()),
		CreatedAt: time.Now().UTC(),
	}
	if !result.OK {
		result.Error = "ui runner screen record returned " + resp.Status
		return result
	}
	if strings.Contains(result.Stdout, `"ok":false`) {
		result.OK = false
		result.Error = "Screen Recording permission is still not granted to Argos UI Runner"
	}
	if st, statErr := os.Stat(output); statErr == nil && !st.IsDir() && st.Size() > 0 {
		result.URL = output
		result.OK = true
		annotateRecordingResult(&result, output, st)
	}
	return result
}

func annotateRecordingResult(result *Result, path string, st os.FileInfo) {
	if result == nil || st == nil {
		return
	}
	result.SizeBytes = st.Size()
	result.SHA256 = recordingFileSHA256(path)
	result.Retention = "ephemeral_after_signal_delivery"
	result.DeleteAt = time.Now().UTC().Add(recordingRetention()).Format(time.RFC3339)
}

func recordingFileSHA256(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return ""
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func recordingRetention() time.Duration {
	value := strings.TrimSpace(os.Getenv("MESHCLAW_ARGOS_RECORDING_RETENTION"))
	if value == "" {
		return 15 * time.Minute
	}
	if d, err := time.ParseDuration(value); err == nil && d > 0 {
		return d
	}
	return 15 * time.Minute
}

func recordingPrerollDelay() time.Duration {
	return durationFromEnv("MESHCLAW_ARGOS_RECORDING_PREROLL", 200*time.Millisecond)
}

func directScreenRecord(ctx context.Context, seconds int, output string) Result {
	return run(ctx, "meshclaw_automation_screen_record", "screencapture", "-x", "-v", "-V", fmt.Sprintf("%d", seconds), output)
}

func allowDirectScreenRecordFallback() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("MESHCLAW_ARGOS_DIRECT_SCREEN_RECORD")))
	return value == "1" || value == "true" || value == "yes"
}

func defaultRecordingPath() string {
	return filepath.Join(defaultMeshClawDir(), "recordings", fmt.Sprintf("argos-%s.mov", time.Now().UTC().Format("20060102T150405Z")))
}

func defaultScreenCapturePath() string {
	return filepath.Join(defaultMeshClawDir(), "recordings", fmt.Sprintf("argos-%s.png", time.Now().UTC().Format("20060102T150405Z")))
}

func defaultMeshClawDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "meshclaw")
	}
	return filepath.Join(home, ".meshclaw")
}

func defaultUIRunnerURL() string {
	if value := strings.TrimRight(strings.TrimSpace(os.Getenv("MESHCLAW_ARGOS_UI_RUNNER_URL")), "/"); value != "" {
		return value
	}
	if value := strings.TrimRight(strings.TrimSpace(os.Getenv("MESHCLAW_ARGOS_UI_RUNNER")), "/"); value != "" {
		return value
	}
	return "http://127.0.0.1:48292"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func containsAnyLocal(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func ptrResult(result Result) *Result {
	return &result
}

func jsonString(value string) string {
	data, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(data)
}

func ClassifyArgosAction(text string) ArgosAction {
	trimmed := strings.TrimSpace(text)
	action := ArgosAction{
		Kind:      "meshclaw_argos_action",
		Text:      trimmed,
		Intent:    "macos_assistant",
		Action:    "none",
		CreatedAt: time.Now().UTC(),
	}
	lower := strings.ToLower(trimmed)
	if trimmed == "" {
		action.Error = "text is required"
		return action
	}
	if isArgosHelpRequest(trimmed) {
		action.Action = "help"
		action.NextActions = append(action.NextActions,
			"가능한 작업: 일정/리마인더 조회, 메모/문서 초안, 웹 검색, 앱 열기, 단축어 실행, 연락처 검색, Mac 실행기 상태 점검.",
			"개인 데이터 변경은 실행 전 확인하거나 --execute가 필요합니다.",
		)
		return action
	}
	if provider, prompt, ok := parseAIHandoff(trimmed); ok {
		action.Action = "ai_handoff"
		action.Provider = provider
		action.Prompt = prompt
		return action
	}
	if name, input, ok := parseShortcut(trimmed); ok {
		action.Action = "shortcut_run"
		action.Shortcut = name
		action.Input = input
		return action
	}
	if query, ok := parseWorkDemo(trimmed); ok {
		action.Action = "work_demo"
		action.Query = query
		return action
	}
	if title, body, ok := parseDocumentCreate(trimmed); ok {
		action.Action = "document_create"
		action.NoteTitle = title
		action.NoteBody = body
		return action
	}
	if id, ok := parseMacRunnerDoctor(trimmed); ok {
		action.Action = "mac_runner_doctor"
		action.Input = id
		return action
	}
	if parseMacRunnerCommand(trimmed) {
		action.Action = "mac_runner_command"
		action.Input = trimmed
		return action
	}
	if query, start, end, ok := parseCalendarList(trimmed, time.Now()); ok && !isExplicitNoteRequest(trimmed) {
		action.Action = "calendar_events_list"
		action.CalendarQuery = query
		action.CalendarStart = start
		action.CalendarEnd = end
		return action
	}
	if title, body, ok := parseNote(trimmed); ok {
		action.Action = "note_create"
		action.NoteTitle = title
		action.NoteBody = body
		return action
	}
	if query, ok := parseContactsSearch(trimmed); ok {
		action.Action = "contacts_search"
		action.ContactQuery = query
		return action
	}
	if isVagueContactsRequest(trimmed) {
		action.Error = "contact search needs a specific name, company, phone, or email query"
		action.NextActions = append(action.NextActions, "연락처를 찾으려면 이름, 회사, 전화번호, 이메일 중 하나를 구체적으로 알려주세요.")
		return action
	}
	if query, start, end, ok := parseCalendarList(trimmed, time.Now()); ok {
		action.Action = "calendar_events_list"
		action.CalendarQuery = query
		action.CalendarStart = start
		action.CalendarEnd = end
		return action
	}
	if query, start, end, ok := parseCalendarDelete(trimmed, time.Now()); ok {
		action.Action = "calendar_event_delete"
		action.CalendarQuery = query
		action.CalendarStart = start
		action.CalendarEnd = end
		return action
	}
	if title, notes, start, end, ok := parseCalendarEvent(trimmed, time.Now()); ok {
		action.Action = "calendar_event_create"
		action.CalendarTitle = title
		action.CalendarNotes = notes
		action.CalendarStart = start
		action.CalendarEnd = end
		return action
	}
	if query, start, end, ok := parseReminderList(trimmed, time.Now()); ok {
		action.Action = "reminders_list"
		action.ReminderQuery = query
		action.ReminderStart = start
		action.ReminderEnd = end
		return action
	}
	if mutation, query, ok := parseReminderMutation(trimmed); ok {
		action.Action = "reminder_" + mutation
		action.ReminderQuery = query
		return action
	}
	if title, notes, due, ok := parseReminder(trimmed, time.Now()); ok {
		action.Action = "reminder_create"
		action.ReminderTitle = title
		action.ReminderNotes = notes
		action.ReminderDue = due
		return action
	}
	if urlValue := firstURL(trimmed); urlValue != "" {
		if strings.Contains(lower, "읽") || strings.Contains(lower, "요약") || strings.Contains(lower, "확인") || strings.Contains(lower, "check") || strings.Contains(lower, "summar") {
			action.Action = "browser_fetch"
		} else {
			action.Action = "open_url"
		}
		action.URL = urlValue
		return action
	}
	if app := parseOpenApp(trimmed); app != "" {
		action.Action = "open_app"
		action.App = app
		return action
	}
	if query, ok := parseSearch(trimmed); ok {
		if wantsVisibleBrowser(lower) {
			action.Action = "visible_browser_search"
		} else {
			action.Action = "browser_search"
		}
		action.Query = query
		return action
	}
	if strings.Contains(lower, "클립보드") || strings.Contains(lower, "복사") || strings.Contains(lower, "clipboard") {
		action.Action = "clipboard_set"
		action.Input = stripKnownPrefixes(trimmed, []string{"클립보드에", "복사해", "복사", "clipboard"})
		if action.Input == "" {
			action.Error = "clipboard text is required"
		}
		return action
	}
	action.NextActions = append(action.NextActions, "지원되는 예: 브라우저로 검색해줘, Safari 열어줘, Notes에 메모해줘, 내일 오전 9시에 리마인더 추가해줘, 단축어 실행해줘, Claude로 넘겨줘.")
	return action
}

func OpenBrowserSearch(ctx context.Context, query string) Result {
	query = strings.TrimSpace(query)
	if query == "" {
		return failed("meshclaw_automation_visible_browser_search", "search query is required")
	}
	searchURL := "https://duckduckgo.com/?q=" + url.QueryEscape(query)
	result := OpenURL(ctx, searchURL)
	result.Kind = "meshclaw_automation_visible_browser_search"
	result.Action = "visible_browser_search"
	result.URL = searchURL
	return result
}

func WorkDemo(ctx context.Context, query string) Result {
	query = strings.TrimSpace(query)
	if query == "" {
		query = defaultArgosDemoQuery()
	}
	result := Result{
		Kind:      "meshclaw_automation_work_demo",
		Action:    "work_demo",
		Prompt:    query,
		CreatedAt: time.Now().UTC(),
	}
	opened := OpenBrowserSearch(ctx, query)
	result.Command = opened.Command
	result.URL = opened.URL
	if !opened.OK {
		result.OK = false
		result.Error = opened.Error
		return result
	}
	time.Sleep(demoStepDelay())
	search, searchErr := browserauto.Search(ctx, browserauto.SearchOptions{Query: query, Limit: 4, Timeout: 12})
	doc, err := saveWorkDemoDocument(query, search, searchErr)
	if err != nil {
		result.OK = false
		result.Error = err.Error()
		return result
	}
	if clip := SetClipboard(ctx, doc.Body); !clip.OK {
		result.OK = false
		result.Error = clip.Error
		return result
	}
	_ = UIRunnerKey(ctx, "w", "command")
	time.Sleep(demoStepDelay())
	if opened := OpenWorkDocument(ctx, doc.Path, doc.App); !opened.OK {
		result.OK = false
		result.Error = opened.Error
		return result
	}
	time.Sleep(demoStepDelay())
	if doc.App == "TextEdit" {
		_ = UIRunnerKey(ctx, "a", "command")
		time.Sleep(120 * time.Millisecond)
		if pasted := UIRunnerKey(ctx, "v", "command"); !pasted.OK {
			result.OK = false
			result.Error = pasted.Error
			return result
		}
	}
	time.Sleep(demoRevealDelay())
	_ = run(ctx, "meshclaw_automation_reveal_file", "open", "-R", doc.Path)
	result.OK = true
	result.URL = doc.Path
	result.App = doc.App
	result.Stdout = fmt.Sprintf("saved %s\npreview %s", doc.Path, doc.PreviewPath)
	return result
}

func workDemoDocument(query string, search browserauto.SearchResult, searchErr error) string {
	var lines []string
	lines = append(lines, "Argos Work Demo")
	lines = append(lines, "")
	lines = append(lines, "Request: "+query)
	lines = append(lines, "Steps:")
	lines = append(lines, "1. Opened browser search.")
	lines = append(lines, "2. Collected visible web results.")
	lines = append(lines, "3. Pasted a short working note.")
	lines = append(lines, "4. Saved this file for review.")
	lines = append(lines, "")
	lines = append(lines, "Summary:")
	if searchErr != nil {
		lines = append(lines, "- Search summary unavailable: "+searchErr.Error())
	} else if len(search.Results) == 0 {
		lines = append(lines, "- No search results were returned.")
	} else {
		for i, item := range search.Results {
			if i >= 4 {
				break
			}
			lines = append(lines, fmt.Sprintf("- %s", strings.TrimSpace(item.Text)))
			if strings.TrimSpace(item.URL) != "" {
				lines = append(lines, "  "+strings.TrimSpace(item.URL))
			}
		}
	}
	lines = append(lines, "")
	lines = append(lines, "Saved by MeshClaw Argos at "+time.Now().Format(time.RFC3339))
	return strings.Join(lines, "\n")
}

func saveWorkDemoDocument(query string, search browserauto.SearchResult, searchErr error) (workDemoDocumentFile, error) {
	body := workDemoDocument(query, search, searchErr)
	home, err := os.UserHomeDir()
	if err != nil {
		return workDemoDocumentFile{}, err
	}
	if appAvailableLocal("Obsidian") {
		dir := filepath.Join(home, "Documents", "Argos Vault", "Work Reports")
		if err := os.MkdirAll(dir, 0700); err != nil {
			return workDemoDocumentFile{}, err
		}
		path := filepath.Join(dir, "argos-work-demo-"+time.Now().Format("20060102-150405")+".md")
		if err := os.WriteFile(path, []byte(markdownWorkDemo(body)), 0600); err != nil {
			return workDemoDocumentFile{}, err
		}
		preview, err := saveWorkDemoPreviewHTML(path, query, body)
		if err != nil {
			return workDemoDocumentFile{}, err
		}
		return workDemoDocumentFile{Path: path, PreviewPath: preview, Body: body, App: "Obsidian"}, nil
	}
	dir := filepath.Join(home, "Documents", "Argos")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return workDemoDocumentFile{}, err
	}
	name := "argos-work-demo-" + time.Now().Format("20060102-150405") + ".txt"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		return workDemoDocumentFile{}, err
	}
	preview, err := saveWorkDemoPreviewHTML(path, query, body)
	if err != nil {
		return workDemoDocumentFile{}, err
	}
	return workDemoDocumentFile{Path: path, PreviewPath: preview, Body: body, App: "TextEdit"}, nil
}

func saveBrowserSearchDocument(ctx context.Context, query string, search browserauto.SearchResult) (workDemoDocumentFile, error) {
	body := browserSearchDocument(query, search)
	if appAvailableLocal("Obsidian") {
		sourcePages := publish.FetchResearchSources(ctx, search, 5, 20)
		report, err := publish.SaveResearchDocument(search, publish.ResearchDocumentOptions{Query: query, Limit: 5, SourcePages: sourcePages})
		if err != nil {
			return workDemoDocumentFile{}, err
		}
		return workDemoDocumentFile{Path: report.Path, PreviewPath: report.PreviewPath, Body: body, App: "Obsidian"}, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return workDemoDocumentFile{}, err
	}
	dir := filepath.Join(home, "Documents", "Argos")
	app := "TextEdit"
	if err := os.MkdirAll(dir, 0700); err != nil {
		return workDemoDocumentFile{}, err
	}
	ext := ".txt"
	content := body
	path := filepath.Join(dir, "argos-search-"+time.Now().Format("20060102-150405")+ext)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return workDemoDocumentFile{}, err
	}
	preview, err := saveBrowserSearchPreviewHTML(path, query, search)
	if err != nil {
		return workDemoDocumentFile{}, err
	}
	return workDemoDocumentFile{Path: path, PreviewPath: preview, Body: body, App: app}, nil
}

func CreateArgosDocument(ctx context.Context, title, body string) Result {
	_ = ctx
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Argos 문서"
	}
	body = strings.TrimSpace(body)
	if body == "" {
		body = "Signal 요청으로 생성한 문서입니다."
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return failed("meshclaw_automation_document_create", err.Error())
	}
	dir := filepath.Join(home, "Documents", "Argos Vault", "Inbox")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return failed("meshclaw_automation_document_create", err.Error())
	}
	base := safeArgosDocumentFilename(title)
	stamp := time.Now().Format("20060102-150405")
	mdPath := filepath.Join(dir, base+"-"+stamp+".md")
	path := filepath.Join(dir, base+"-"+stamp+".html")
	docxPath := filepath.Join(dir, base+"-"+stamp+".docx")
	md := markdownArgosDocument(title, body)
	doc := argosreport.RenderMobileHTML(argosreport.MobileReport{
		Eyebrow:  "Argos Document",
		Title:    title,
		Subtitle: "맥북 포커스를 건드리지 않고 맥미니에서 생성했습니다.",
		Findings: []string{body},
		Footer:   "Markdown은 Obsidian 편집용, HTML은 iPhone 보기용입니다.",
	})
	if err := os.WriteFile(mdPath, []byte(md), 0600); err != nil {
		return failed("meshclaw_automation_document_create", err.Error())
	}
	if err := os.WriteFile(path, []byte(doc), 0600); err != nil {
		return failed("meshclaw_automation_document_create", err.Error())
	}
	if err := writeSimpleDOCX(docxPath, title, body); err != nil {
		return failed("meshclaw_automation_document_create", err.Error())
	}
	preview := saveWorkDemoPreviewImage(path)
	return Result{
		Kind:      "meshclaw_automation_document_create",
		Action:    "document_create",
		URL:       path,
		Preview:   preview,
		DOCX:      docxPath,
		Markdown:  mdPath,
		OK:        true,
		Stdout:    "created " + mdPath,
		CreatedAt: time.Now().UTC(),
	}
}

func CreateSpreadsheet(ctx context.Context, title, body string) Result {
	_ = ctx
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Argos 표"
	}
	rows := spreadsheetRowsFromBody(title, body)
	home, err := os.UserHomeDir()
	if err != nil {
		return failed("meshclaw_automation_spreadsheet_create", err.Error())
	}
	dir := filepath.Join(home, "Documents", "Argos Vault", "Sheets")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return failed("meshclaw_automation_spreadsheet_create", err.Error())
	}
	base := safeArgosDocumentFilename(title)
	stamp := time.Now().Format("20060102-150405")
	xlsxPath := filepath.Join(dir, base+"-"+stamp+".xlsx")
	csvPath := filepath.Join(dir, base+"-"+stamp+".csv")
	htmlPath := filepath.Join(dir, base+"-"+stamp+".html")
	if err := writeSimpleXLSX(xlsxPath, rows); err != nil {
		return failed("meshclaw_automation_spreadsheet_create", err.Error())
	}
	if err := writeCSV(csvPath, rows); err != nil {
		return failed("meshclaw_automation_spreadsheet_create", err.Error())
	}
	if err := os.WriteFile(htmlPath, []byte(spreadsheetPreviewHTML(title, rows)), 0600); err != nil {
		return failed("meshclaw_automation_spreadsheet_create", err.Error())
	}
	return Result{
		Kind:      "meshclaw_automation_spreadsheet_create",
		Action:    "spreadsheet_create",
		URL:       htmlPath,
		XLSX:      xlsxPath,
		CSV:       csvPath,
		OK:        true,
		Stdout:    fmt.Sprintf("created %s with %d rows", xlsxPath, len(rows)),
		CreatedAt: time.Now().UTC(),
	}
}

func OpenFile(ctx context.Context, path, app string) Result {
	path = strings.TrimSpace(path)
	if path == "" {
		return failed("meshclaw_automation_open_file", "file path is required")
	}
	result := OpenWorkDocument(ctx, path, app)
	result.Kind = "meshclaw_automation_open_file"
	result.Action = "open_file"
	result.URL = path
	return result
}

func ShowNotification(ctx context.Context, title, body string) Result {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Argos"
	}
	body = strings.TrimSpace(body)
	if body == "" {
		body = "MeshClaw notification"
	}
	if runtime.GOOS == "darwin" {
		return run(ctx, "meshclaw_automation_notification_show", "osascript", "-e", fmt.Sprintf("display notification %s with title %s", appleScriptString(body), appleScriptString(title)))
	}
	if runtime.GOOS == "linux" {
		return run(ctx, "meshclaw_automation_notification_show", "notify-send", title, body)
	}
	return failed("meshclaw_automation_notification_show", "notifications are only implemented for macOS and Linux")
}

func ExportMarkdown(ctx context.Context, input, format, output string) Result {
	input = strings.TrimSpace(input)
	if input == "" {
		return failed("meshclaw_automation_document_export", "input markdown path is required")
	}
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "docx"
	}
	if format != "docx" && format != "pdf" {
		return failed("meshclaw_automation_document_export", "format must be docx or pdf")
	}
	data, err := os.ReadFile(input)
	if err != nil {
		return failed("meshclaw_automation_document_export", err.Error())
	}
	if strings.TrimSpace(output) == "" {
		output = strings.TrimSuffix(input, filepath.Ext(input)) + "." + format
	}
	if err := os.MkdirAll(filepath.Dir(output), 0700); err != nil {
		return failed("meshclaw_automation_document_export", err.Error())
	}
	if _, err := exec.LookPath("pandoc"); err == nil {
		result := run(ctx, "meshclaw_automation_document_export", "pandoc", input, "-o", output)
		result.Action = "document_export"
		result.URL = output
		if format == "docx" {
			result.DOCX = output
		} else {
			result.PDF = output
		}
		return result
	}
	if format == "pdf" {
		result := failed("meshclaw_automation_document_export", "pandoc is required for pdf export")
		result.Action = "document_export"
		result.Markdown = input
		result.URL = input
		return result
	}
	title := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
	if err := writeSimpleDOCX(output, title, string(data)); err != nil {
		return failed("meshclaw_automation_document_export", err.Error())
	}
	info, _ := os.Stat(output)
	result := Result{
		Kind:      "meshclaw_automation_document_export",
		Action:    "document_export",
		URL:       output,
		DOCX:      output,
		OK:        true,
		Stdout:    "created " + output,
		CreatedAt: time.Now().UTC(),
	}
	if info != nil {
		result.SizeBytes = info.Size()
	}
	return result
}

type PresentationSlide struct {
	Title   string   `json:"title"`
	Bullets []string `json:"bullets,omitempty"`
}

func CreatePresentation(ctx context.Context, title, body, audience string, slideCount int, output string) Result {
	_ = ctx
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Argos Presentation"
	}
	body = strings.TrimSpace(body)
	if body == "" {
		body = "사용자 요청에 맞춘 발표자료 초안입니다."
	}
	if slideCount <= 0 {
		slideCount = 6
	}
	if slideCount > 20 {
		slideCount = 20
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return failed("meshclaw_automation_presentation_create", err.Error())
	}
	stamp := time.Now().Format("20060102-150405")
	dir := filepath.Join(home, "Documents", "Argos Vault", "Presentations")
	if strings.TrimSpace(output) != "" {
		dir = filepath.Dir(output)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return failed("meshclaw_automation_presentation_create", err.Error())
	}
	base := safeArgosDocumentFilename(title) + "-" + stamp
	pptxPath := output
	if strings.TrimSpace(pptxPath) == "" {
		pptxPath = filepath.Join(dir, base+".pptx")
	}
	mdPath := filepath.Join(dir, strings.TrimSuffix(filepath.Base(pptxPath), filepath.Ext(pptxPath))+"-outline.md")
	htmlPath := filepath.Join(dir, strings.TrimSuffix(filepath.Base(pptxPath), filepath.Ext(pptxPath))+"-preview.html")
	slides := buildPresentationSlides(title, body, audience, slideCount)
	if err := writeSimplePPTX(pptxPath, slides); err != nil {
		return failed("meshclaw_automation_presentation_create", err.Error())
	}
	outline := presentationOutlineMarkdown(title, audience, slides)
	if err := os.WriteFile(mdPath, []byte(outline), 0600); err != nil {
		return failed("meshclaw_automation_presentation_create", err.Error())
	}
	if err := os.WriteFile(htmlPath, []byte(presentationPreviewHTML(title, audience, slides)), 0600); err != nil {
		return failed("meshclaw_automation_presentation_create", err.Error())
	}
	if err := verifySimplePPTX(pptxPath, len(slides)); err != nil {
		return failed("meshclaw_automation_presentation_create", err.Error())
	}
	preview := saveWorkDemoPreviewImage(htmlPath)
	info, _ := os.Stat(pptxPath)
	result := Result{
		Kind:      "meshclaw_automation_presentation_create",
		Action:    "presentation_create",
		URL:       htmlPath,
		Preview:   preview,
		PPTX:      pptxPath,
		Markdown:  mdPath,
		OK:        true,
		Stdout:    fmt.Sprintf("created %s with %d slides", pptxPath, len(slides)),
		CreatedAt: time.Now().UTC(),
	}
	if info != nil {
		result.SizeBytes = info.Size()
	}
	return result
}

func VerifyPresentation(ctx context.Context, path string) Result {
	_ = ctx
	path = strings.TrimSpace(path)
	if path == "" {
		return failed("meshclaw_automation_presentation_verify", "pptx path is required")
	}
	reader, err := zip.OpenReader(path)
	if err != nil {
		return failed("meshclaw_automation_presentation_verify", err.Error())
	}
	defer reader.Close()
	slides := 0
	hasPresentation := false
	hasRels := false
	for _, file := range reader.File {
		if strings.HasPrefix(file.Name, "ppt/slides/slide") && strings.HasSuffix(file.Name, ".xml") {
			slides++
		}
		if file.Name == "ppt/presentation.xml" {
			hasPresentation = true
		}
		if file.Name == "ppt/_rels/presentation.xml.rels" {
			hasRels = true
		}
	}
	if slides == 0 {
		return failed("meshclaw_automation_presentation_verify", "pptx has no slides")
	}
	if !hasPresentation || !hasRels {
		return failed("meshclaw_automation_presentation_verify", "pptx is missing presentation metadata")
	}
	info, _ := os.Stat(path)
	result := Result{
		Kind:      "meshclaw_automation_presentation_verify",
		Action:    "presentation_verify",
		URL:       path,
		PPTX:      path,
		OK:        true,
		Stdout:    fmt.Sprintf("verified %s with %d slides", path, slides),
		CreatedAt: time.Now().UTC(),
	}
	if info != nil {
		result.SizeBytes = info.Size()
	}
	return result
}

func ExportPresentation(ctx context.Context, input, format, output string) Result {
	input = strings.TrimSpace(input)
	if input == "" {
		return failed("meshclaw_automation_presentation_export", "input pptx path is required")
	}
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "pdf"
	}
	if format != "pdf" {
		return failed("meshclaw_automation_presentation_export", "format must be pdf")
	}
	verify := VerifyPresentation(ctx, input)
	if !verify.OK {
		verify.Kind = "meshclaw_automation_presentation_export"
		verify.Action = "presentation_export"
		return verify
	}
	soffice := firstAvailableCommand("soffice", "libreoffice")
	if soffice == "" {
		result := failed("meshclaw_automation_presentation_export", "LibreOffice/soffice is required for PPTX to PDF export")
		result.Action = "presentation_export"
		result.PPTX = input
		result.URL = input
		return result
	}
	if strings.TrimSpace(output) == "" {
		output = strings.TrimSuffix(input, filepath.Ext(input)) + ".pdf"
	}
	outDir := filepath.Dir(output)
	if err := os.MkdirAll(outDir, 0700); err != nil {
		return failed("meshclaw_automation_presentation_export", err.Error())
	}
	result := run(ctx, "meshclaw_automation_presentation_export", soffice, "--headless", "--convert-to", "pdf", "--outdir", outDir, input)
	result.Action = "presentation_export"
	result.PPTX = input
	converted := filepath.Join(outDir, strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))+".pdf")
	if converted != output {
		if err := os.Rename(converted, output); err != nil && !os.IsNotExist(err) {
			result.OK = false
			result.Error = err.Error()
		}
	}
	if _, err := os.Stat(output); err != nil {
		result.OK = false
		if result.Error == "" {
			result.Error = err.Error()
		}
	}
	if result.OK {
		result.URL = output
		result.PDF = output
	}
	info, _ := os.Stat(output)
	if info != nil {
		result.SizeBytes = info.Size()
	}
	return result
}

func MapsSearch(ctx context.Context, query, provider string, execute bool) Result {
	query = strings.TrimSpace(query)
	if query == "" {
		return failed("meshclaw_automation_maps_search", "query is required")
	}
	provider = normalizeMapsProvider(provider)
	target := mapsSearchURL(query, provider)
	result := Result{
		Kind:      "meshclaw_automation_maps_search",
		Action:    "maps_search",
		Provider:  provider,
		URL:       target,
		OK:        true,
		Stdout:    target,
		CreatedAt: time.Now().UTC(),
	}
	if execute {
		opened := OpenURL(ctx, target)
		result.OK = opened.OK
		result.Command = opened.Command
		result.Error = opened.Error
		result.Stderr = opened.Stderr
	}
	return result
}

func MapsDirections(ctx context.Context, origin, destination, mode, provider string, execute bool) Result {
	destination = strings.TrimSpace(destination)
	if destination == "" {
		return failed("meshclaw_automation_maps_directions", "destination is required")
	}
	origin = strings.TrimSpace(origin)
	mode = normalizeDirectionsMode(mode)
	provider = normalizeMapsProvider(provider)
	target := mapsDirectionsURL(origin, destination, mode, provider)
	result := Result{
		Kind:      "meshclaw_automation_maps_directions",
		Action:    "maps_directions",
		Provider:  provider,
		URL:       target,
		OK:        true,
		Stdout:    target,
		CreatedAt: time.Now().UTC(),
	}
	if execute {
		opened := OpenURL(ctx, target)
		result.OK = opened.OK
		result.Command = opened.Command
		result.Error = opened.Error
		result.Stderr = opened.Stderr
	}
	return result
}

func EditPresentation(ctx context.Context, input, title, body, output string, backup bool) Result {
	_ = ctx
	input = strings.TrimSpace(input)
	if input == "" {
		return failed("meshclaw_automation_presentation_edit", "input pptx path is required")
	}
	verify := VerifyPresentation(context.Background(), input)
	if !verify.OK {
		verify.Kind = "meshclaw_automation_presentation_edit"
		verify.Action = "presentation_edit"
		return verify
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = "추가 슬라이드"
	}
	body = strings.TrimSpace(body)
	if body == "" {
		body = "추가할 내용이 비어 있습니다."
	}
	if strings.TrimSpace(output) == "" {
		ext := filepath.Ext(input)
		output = strings.TrimSuffix(input, ext) + "-edited-" + time.Now().Format("20060102-150405") + ext
	}
	if err := os.MkdirAll(filepath.Dir(output), 0700); err != nil {
		return failed("meshclaw_automation_presentation_edit", err.Error())
	}
	backupPath := ""
	if backup {
		backupPath = input + ".bak-" + time.Now().Format("20060102T150405Z")
		if err := copyFile(input, backupPath); err != nil {
			return failed("meshclaw_automation_presentation_edit", err.Error())
		}
	}
	if err := appendSimplePPTXSlides(input, output, markdownSections("# "+title+"\n"+body)); err != nil {
		return failed("meshclaw_automation_presentation_edit", err.Error())
	}
	edited := VerifyPresentation(ctx, output)
	edited.Kind = "meshclaw_automation_presentation_edit"
	edited.Action = "presentation_edit"
	edited.PPTX = output
	edited.URL = output
	if edited.OK {
		edited.Stdout = strings.TrimSpace(edited.Stdout + " edited from " + input)
		if backupPath != "" {
			edited.Stderr = "backup: " + backupPath
		}
	}
	return edited
}

func buildPresentationSlides(title, body, audience string, slideCount int) []PresentationSlide {
	sections := markdownSections(body)
	slides := []PresentationSlide{{Title: title, Bullets: []string{firstNonEmptyLocal(audience, lang.T("osauto.presentation.default_audience")), lang.T("osauto.presentation.generated_draft")}}}
	for _, section := range sections {
		if len(slides) >= slideCount {
			break
		}
		slides = append(slides, section)
	}
	for len(slides) < slideCount {
		idx := len(slides)
		slides = append(slides, PresentationSlide{
			Title: lang.T("osauto.presentation.fallback_title", idx),
			Bullets: []string{
				lang.T("osauto.presentation.fallback_bullet.1"),
				lang.T("osauto.presentation.fallback_bullet.2"),
			},
		})
	}
	return slides
}

func markdownSections(body string) []PresentationSlide {
	lines := strings.Split(body, "\n")
	var sections []PresentationSlide
	current := PresentationSlide{}
	flush := func() {
		if strings.TrimSpace(current.Title) == "" && len(current.Bullets) == 0 {
			return
		}
		if strings.TrimSpace(current.Title) == "" {
			current.Title = lang.T("osauto.presentation.section_title", len(sections)+1)
		}
		if len(current.Bullets) == 0 {
			current.Bullets = []string{lang.T("osauto.presentation.section_body")}
		}
		sections = append(sections, current)
		current = PresentationSlide{}
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			flush()
			current.Title = strings.TrimSpace(strings.TrimLeft(line, "#"))
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "-"), "•"))
		if current.Title == "" && len([]rune(line)) <= 48 {
			current.Title = line
			continue
		}
		current.Bullets = append(current.Bullets, line)
		if len(current.Bullets) >= 5 {
			flush()
		}
	}
	flush()
	if len(sections) == 0 {
		words := strings.Fields(body)
		for i := 0; i < len(words); i += 24 {
			end := i + 24
			if end > len(words) {
				end = len(words)
			}
			sections = append(sections, PresentationSlide{Title: lang.T("osauto.presentation.section_title", len(sections)+1), Bullets: []string{strings.Join(words[i:end], " ")}})
		}
	}
	return sections
}

func presentationOutlineMarkdown(title, audience string, slides []PresentationSlide) string {
	lines := []string{
		"---",
		"source: MeshClaw presentation_create",
		"created: " + time.Now().Format(time.RFC3339),
		"audience: " + strings.TrimSpace(audience),
		"---",
		"",
		"# " + strings.TrimSpace(title),
		"",
	}
	for i, slide := range slides {
		lines = append(lines, fmt.Sprintf("## %d. %s", i+1, slide.Title), "")
		for _, bullet := range slide.Bullets {
			lines = append(lines, "- "+bullet)
		}
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func presentationPreviewHTML(title, audience string, slides []PresentationSlide) string {
	findings := []string{}
	for i, slide := range slides {
		if i >= 8 {
			break
		}
		line := fmt.Sprintf("%d. %s", i+1, slide.Title)
		if len(slide.Bullets) > 0 {
			line += " — " + strings.Join(limitStrings(slide.Bullets, 2), " / ")
		}
		findings = append(findings, line)
	}
	if len(findings) == 0 {
		findings = append(findings, lang.T("osauto.presentation.no_slides"))
	}
	return argosreport.RenderMobileHTML(argosreport.MobileReport{
		Eyebrow:  "Argos Presentation",
		Title:    title,
		Subtitle: firstNonEmptyLocal(audience, fmt.Sprintf("%d slides", len(slides))),
		Flow: []string{
			lang.T("osauto.presentation.preview.flow.interpret"),
			lang.T("osauto.presentation.preview.flow.slides"),
			lang.T("osauto.presentation.preview.flow.create"),
			lang.T("osauto.presentation.preview.flow.verify"),
		},
		Findings: findings,
		Footer:   lang.T("osauto.presentation.preview.footer"),
	})
}

func limitStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return values[:limit]
}

func SaveAutomationResult(ctx context.Context, title, body, source string) Result {
	_ = ctx
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Argos 작업 결과"
	}
	body = strings.TrimSpace(body)
	if body == "" {
		body = "작업 결과 본문이 비어 있습니다."
	}
	source = strings.TrimSpace(source)
	if source == "" {
		source = "meshclaw"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return failed("meshclaw_automation_result_save", err.Error())
	}
	stamp := time.Now()
	dir := filepath.Join(home, ".meshclaw", "automation-results", stamp.Format("2006-01-02"))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return failed("meshclaw_automation_result_save", err.Error())
	}
	base := stamp.Format("150405") + "-" + safeArgosDocumentFilename(title)
	mdPath := filepath.Join(dir, base+".md")
	htmlPath := filepath.Join(dir, base+".html")
	md := automationResultMarkdown(title, body, source, stamp)
	doc := argosreport.RenderMobileHTML(argosreport.MobileReport{
		Eyebrow:  "Argos Result",
		Title:    title,
		Subtitle: source,
		Findings: []string{body},
		Footer:   "Saved by MeshClaw macOS assistant runtime.",
	})
	if err := os.WriteFile(mdPath, []byte(md), 0600); err != nil {
		return failed("meshclaw_automation_result_save", err.Error())
	}
	if err := os.WriteFile(htmlPath, []byte(doc), 0600); err != nil {
		return failed("meshclaw_automation_result_save", err.Error())
	}
	preview := saveWorkDemoPreviewImage(htmlPath)
	info, _ := os.Stat(mdPath)
	result := Result{
		Kind:      "meshclaw_automation_result_save",
		Action:    "result_save",
		URL:       htmlPath,
		Preview:   preview,
		Markdown:  mdPath,
		OK:        true,
		Stdout:    "saved " + mdPath,
		CreatedAt: stamp.UTC(),
	}
	if info != nil {
		result.SizeBytes = info.Size()
	}
	return result
}

func RunTerminalTask(ctx context.Context, shell, command, title string, saveArtifact bool) Result {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "터미널 작업"
	}
	result := ScriptRun(ctx, shell, command)
	result.Kind = "meshclaw_automation_terminal_run"
	result.Action = "terminal_run"
	if saveArtifact {
		artifact := SaveAutomationResult(ctx, title, terminalResultBody(command, result), "terminal")
		if result.OK && !artifact.OK {
			result.OK = false
			result.Error = artifact.Error
		}
		result.URL = artifact.URL
		result.Preview = artifact.Preview
		result.Markdown = artifact.Markdown
	}
	return result
}

func RunShortcutTextTask(ctx context.Context, name, input, title string, saveArtifact bool) Result {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Shortcuts 작업"
	}
	result := ShortcutRun(ctx, name, input)
	result.Kind = "meshclaw_automation_shortcut_text_run"
	result.Action = "shortcut_text_run"
	if saveArtifact {
		artifact := SaveAutomationResult(ctx, title, shortcutResultBody(name, input, result), "shortcuts")
		if result.OK && !artifact.OK {
			result.OK = false
			result.Error = artifact.Error
		}
		result.URL = artifact.URL
		result.Preview = artifact.Preview
		result.Markdown = artifact.Markdown
	}
	return result
}

func automationResultMarkdown(title, body, source string, stamp time.Time) string {
	return strings.Join([]string{
		"---",
		"source: " + source,
		"created: " + stamp.Format(time.RFC3339),
		"---",
		"",
		"# " + title,
		"",
		body,
		"",
	}, "\n")
}

func terminalResultBody(command string, result Result) string {
	lines := []string{
		"## Command",
		"",
		"```sh",
		strings.TrimSpace(command),
		"```",
		"",
		"## Status",
		"",
		fmt.Sprintf("- ok: %v", result.OK),
	}
	if strings.TrimSpace(result.Error) != "" {
		lines = append(lines, "- error: "+result.Error)
	}
	if strings.TrimSpace(result.Stdout) != "" {
		lines = append(lines, "", "## Stdout", "", "```text", truncateAutomationOutput(result.Stdout), "```")
	}
	if strings.TrimSpace(result.Stderr) != "" {
		lines = append(lines, "", "## Stderr", "", "```text", truncateAutomationOutput(result.Stderr), "```")
	}
	return strings.Join(lines, "\n")
}

func shortcutResultBody(name, input string, result Result) string {
	lines := []string{
		"## Shortcut",
		"",
		"- name: " + strings.TrimSpace(name),
		"- ok: " + strconv.FormatBool(result.OK),
	}
	if strings.TrimSpace(input) != "" {
		lines = append(lines, "", "## Input", "", "```text", truncateAutomationOutput(input), "```")
	}
	if strings.TrimSpace(result.Stdout) != "" {
		lines = append(lines, "", "## Output", "", "```text", truncateAutomationOutput(result.Stdout), "```")
	}
	if strings.TrimSpace(result.Stderr) != "" {
		lines = append(lines, "", "## Error Output", "", "```text", truncateAutomationOutput(result.Stderr), "```")
	}
	if strings.TrimSpace(result.Error) != "" {
		lines = append(lines, "", "## Error", "", result.Error)
	}
	return strings.Join(lines, "\n")
}

func truncateAutomationOutput(text string) string {
	text = strings.TrimSpace(text)
	const max = 12000
	if len(text) <= max {
		return text
	}
	return text[:max] + "\n...[truncated]"
}

func markdownArgosDocument(title, body string) string {
	lines := []string{
		"---",
		"source: Argos Signal",
		"created: " + time.Now().Format(time.RFC3339),
		"---",
		"",
		"# " + strings.TrimSpace(title),
		"",
		strings.TrimSpace(body),
		"",
	}
	return strings.Join(lines, "\n")
}

func spreadsheetRowsFromBody(title, body string) [][]string {
	if rows := markdownTableRows(body); len(rows) > 0 {
		return rows
	}
	lower := strings.ToLower(strings.TrimSpace(title + " " + body))
	switch {
	case strings.Contains(lower, "invoice") || strings.Contains(lower, "청구") || strings.Contains(lower, "인보이스"):
		return [][]string{
			{"항목", "수량", "단가", "금액", "메모"},
			{"서비스/제품", "1", "0", "0", "수정 필요"},
			{"합계", "", "", "0", ""},
		}
	case strings.Contains(lower, "budget") || strings.Contains(lower, "예산") || strings.Contains(lower, "비용"):
		return [][]string{
			{"구분", "예산", "실사용", "차이", "메모"},
			{"인프라", "0", "0", "0", "서버/클라우드 비용"},
			{"도구", "0", "0", "0", "앱/구독 비용"},
			{"합계", "0", "0", "0", ""},
		}
	case strings.Contains(lower, "tracker") || strings.Contains(lower, "트래커") || strings.Contains(lower, "체크리스트"):
		return [][]string{
			{"항목", "담당", "상태", "마감", "다음 액션"},
			{"첫 번째 작업", "", "대기", "", "내용 입력"},
			{"두 번째 작업", "", "대기", "", "내용 입력"},
		}
	default:
		lines := nonEmptyLines(body)
		rows := [][]string{{"항목", "내용", "상태", "메모"}}
		if len(lines) == 0 {
			lines = []string{strings.TrimSpace(title)}
		}
		for _, line := range lines {
			line = strings.Trim(strings.TrimSpace(line), "-*# ")
			if line == "" {
				continue
			}
			rows = append(rows, []string{line, "", "대기", ""})
			if len(rows) >= 12 {
				break
			}
		}
		if len(rows) == 1 {
			rows = append(rows, []string{"첫 번째 항목", "", "대기", ""})
		}
		return rows
	}
}

func markdownTableRows(body string) [][]string {
	rows := [][]string{}
	for _, line := range strings.Split(body, "\n") {
		row, ok := parseMarkdownTableRow(line)
		if !ok {
			continue
		}
		rows = append(rows, row)
	}
	return rows
}

func nonEmptyLines(body string) []string {
	out := []string{}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func writeCSV(path string, rows [][]string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	w := csv.NewWriter(file)
	if err := w.WriteAll(rows); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

func spreadsheetPreviewHTML(title string, rows [][]string) string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>`)
	b.WriteString(html.EscapeString(title))
	b.WriteString(`</title><style>body{font-family:-apple-system,BlinkMacSystemFont,sans-serif;margin:24px;line-height:1.45}table{border-collapse:collapse;width:100%;font-size:14px}th,td{border:1px solid #d0d7de;padding:8px;text-align:left;vertical-align:top}th{background:#f6f8fa}caption{font-size:20px;font-weight:700;text-align:left;margin-bottom:12px}</style></head><body><table><caption>`)
	b.WriteString(html.EscapeString(title))
	b.WriteString(`</caption>`)
	for i, row := range rows {
		if i == 0 {
			b.WriteString("<thead><tr>")
			for _, cell := range row {
				b.WriteString("<th>")
				b.WriteString(html.EscapeString(cell))
				b.WriteString("</th>")
			}
			b.WriteString("</tr></thead><tbody>")
			continue
		}
		b.WriteString("<tr>")
		for _, cell := range row {
			b.WriteString("<td>")
			b.WriteString(html.EscapeString(cell))
			b.WriteString("</td>")
		}
		b.WriteString("</tr>")
	}
	b.WriteString("</tbody></table></body></html>")
	return b.String()
}

func writeSimpleXLSX(path string, rows [][]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	zw := zip.NewWriter(file)
	write := func(name, content string) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = io.WriteString(w, content)
		return err
	}
	if err := write("[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
</Types>`); err != nil {
		return err
	}
	if err := write("_rels/.rels", `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`); err != nil {
		return err
	}
	if err := write("xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
</Relationships>`); err != nil {
		return err
	}
	if err := write("xl/workbook.xml", `<?xml version="1.0" encoding="UTF-8"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Argos" sheetId="1" r:id="rId1"/></sheets></workbook>`); err != nil {
		return err
	}
	if err := write("xl/worksheets/sheet1.xml", xlsxWorksheetXML(rows)); err != nil {
		return err
	}
	return zw.Close()
}

func xlsxWorksheetXML(rows [][]string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for r, row := range rows {
		b.WriteString(fmt.Sprintf(`<row r="%d">`, r+1))
		for c, cell := range row {
			ref := xlsxCellRef(c, r)
			b.WriteString(`<c r="`)
			b.WriteString(ref)
			b.WriteString(`" t="inlineStr"><is><t>`)
			b.WriteString(html.EscapeString(strings.TrimSpace(cell)))
			b.WriteString(`</t></is></c>`)
		}
		b.WriteString(`</row>`)
	}
	b.WriteString(`</sheetData></worksheet>`)
	return b.String()
}

func xlsxCellRef(col, row int) string {
	name := ""
	col++
	for col > 0 {
		col--
		name = string(rune('A'+(col%26))) + name
		col /= 26
	}
	return fmt.Sprintf("%s%d", name, row+1)
}

func writeSimpleDOCX(path, title, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	zw := zip.NewWriter(file)
	write := func(name, content string) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = io.WriteString(w, content)
		return err
	}
	if err := write("[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`); err != nil {
		return err
	}
	if err := write("_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`); err != nil {
		return err
	}
	document := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		docxParagraph(title, true) +
		docxParagraph("", false)
	lines := strings.Split(body, "\n")
	for i := 0; i < len(lines); i++ {
		if row, ok := parseMarkdownTableRow(lines[i]); ok {
			rows := [][]string{row}
			j := i + 1
			if j < len(lines) && isMarkdownTableSeparator(lines[j]) {
				j++
			}
			for ; j < len(lines); j++ {
				next, ok := parseMarkdownTableRow(lines[j])
				if !ok {
					break
				}
				rows = append(rows, next)
			}
			document += docxTable(rows)
			i = j - 1
			continue
		}
		document += docxParagraph(lines[i], false)
	}
	document += `<w:sectPr><w:pgSz w:w="12240" w:h="15840"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440"/></w:sectPr></w:body></w:document>`
	if err := write("word/document.xml", document); err != nil {
		return err
	}
	return zw.Close()
}

func writeSimplePPTX(path string, slides []PresentationSlide) error {
	if len(slides) == 0 {
		return fmt.Errorf("at least one slide is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	zw := zip.NewWriter(file)
	write := func(name, content string) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = io.WriteString(w, content)
		return err
	}
	if err := write("[Content_Types].xml", pptxContentTypes(len(slides))); err != nil {
		return err
	}
	if err := write("_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="ppt/presentation.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>
<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>
</Relationships>`); err != nil {
		return err
	}
	if err := write("docProps/app.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes"><Application>MeshClaw</Application><PresentationFormat>On-screen Show (16:9)</PresentationFormat><Slides>`+strconv.Itoa(len(slides))+`</Slides></Properties>`); err != nil {
		return err
	}
	if err := write("docProps/core.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"><dc:creator>MeshClaw</dc:creator><dc:title>`+html.EscapeString(slides[0].Title)+`</dc:title><dcterms:created xsi:type="dcterms:W3CDTF">`+time.Now().UTC().Format(time.RFC3339)+`</dcterms:created></cp:coreProperties>`); err != nil {
		return err
	}
	if err := write("ppt/presentation.xml", pptxPresentationXML(len(slides))); err != nil {
		return err
	}
	if err := write("ppt/_rels/presentation.xml.rels", pptxPresentationRels(len(slides))); err != nil {
		return err
	}
	if err := writePPTXCommonParts(write); err != nil {
		return err
	}
	for i, slide := range slides {
		if err := write(fmt.Sprintf("ppt/slides/slide%d.xml", i+1), pptxSlideXML(slide, i+1)); err != nil {
			return err
		}
		if err := write(fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", i+1), pptxSlideRels()); err != nil {
			return err
		}
	}
	return zw.Close()
}

func appendSimplePPTXSlides(input, output string, extraSlides []PresentationSlide) error {
	if len(extraSlides) == 0 {
		return fmt.Errorf("at least one extra slide is required")
	}
	reader, err := zip.OpenReader(input)
	if err != nil {
		return err
	}
	defer reader.Close()
	existingSlides := 0
	for _, file := range reader.File {
		if strings.HasPrefix(file.Name, "ppt/slides/slide") && strings.HasSuffix(file.Name, ".xml") {
			existingSlides++
		}
	}
	if existingSlides == 0 {
		return fmt.Errorf("pptx has no existing slides")
	}
	out, err := os.Create(output)
	if err != nil {
		return err
	}
	defer out.Close()
	zw := zip.NewWriter(out)
	skip := map[string]bool{
		"[Content_Types].xml": true,
	}
	for _, file := range reader.File {
		if skip[file.Name] || shouldRegeneratePPTXPart(file.Name) {
			continue
		}
		if err := copyZipFile(zw, file); err != nil {
			_ = zw.Close()
			return err
		}
	}
	total := existingSlides + len(extraSlides)
	write := func(name, content string) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = io.WriteString(w, content)
		return err
	}
	if err := write("[Content_Types].xml", pptxContentTypes(total)); err != nil {
		_ = zw.Close()
		return err
	}
	if err := write("ppt/presentation.xml", pptxPresentationXML(total)); err != nil {
		_ = zw.Close()
		return err
	}
	if err := write("ppt/_rels/presentation.xml.rels", pptxPresentationRels(total)); err != nil {
		_ = zw.Close()
		return err
	}
	if err := writePPTXCommonParts(write); err != nil {
		_ = zw.Close()
		return err
	}
	for idx := 1; idx <= existingSlides; idx++ {
		if err := write(fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", idx), pptxSlideRels()); err != nil {
			_ = zw.Close()
			return err
		}
	}
	for i, slide := range extraSlides {
		idx := existingSlides + i + 1
		if err := write(fmt.Sprintf("ppt/slides/slide%d.xml", idx), pptxSlideXML(slide, idx)); err != nil {
			_ = zw.Close()
			return err
		}
		if err := write(fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", idx), pptxSlideRels()); err != nil {
			_ = zw.Close()
			return err
		}
	}
	return zw.Close()
}

func shouldRegeneratePPTXPart(name string) bool {
	if name == "ppt/presentation.xml" || name == "ppt/_rels/presentation.xml.rels" {
		return true
	}
	if name == "ppt/slideMasters/slideMaster1.xml" ||
		name == "ppt/slideMasters/_rels/slideMaster1.xml.rels" ||
		name == "ppt/slideLayouts/slideLayout1.xml" ||
		name == "ppt/slideLayouts/_rels/slideLayout1.xml.rels" ||
		name == "ppt/theme/theme1.xml" {
		return true
	}
	if strings.HasPrefix(name, "ppt/slides/_rels/slide") && strings.HasSuffix(name, ".xml.rels") {
		return true
	}
	return false
}

func copyZipFile(zw *zip.Writer, file *zip.File) error {
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	header := file.FileHeader
	w, err := zw.CreateHeader(&header)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, src)
	return err
}

func verifySimplePPTX(path string, slideCount int) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer reader.Close()
	required := map[string]bool{
		"[Content_Types].xml":                          false,
		"_rels/.rels":                                  false,
		"ppt/presentation.xml":                         false,
		"ppt/_rels/presentation.xml.rels":              false,
		"ppt/slideMasters/slideMaster1.xml":            false,
		"ppt/slideMasters/_rels/slideMaster1.xml.rels": false,
		"ppt/slideLayouts/slideLayout1.xml":            false,
		"ppt/slideLayouts/_rels/slideLayout1.xml.rels": false,
		"ppt/theme/theme1.xml":                         false,
	}
	for i := 1; i <= slideCount; i++ {
		required[fmt.Sprintf("ppt/slides/slide%d.xml", i)] = false
		required[fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", i)] = false
	}
	for _, file := range reader.File {
		if _, ok := required[file.Name]; ok {
			required[file.Name] = true
		}
	}
	for name, ok := range required {
		if !ok {
			return fmt.Errorf("pptx missing %s", name)
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func pptxContentTypes(slides int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>
<Override PartName="/ppt/slideMasters/slideMaster1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideMaster+xml"/>
<Override PartName="/ppt/slideLayouts/slideLayout1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml"/>
<Override PartName="/ppt/theme/theme1.xml" ContentType="application/vnd.openxmlformats-officedocument.theme+xml"/>
<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>
<Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>`)
	for i := 1; i <= slides; i++ {
		b.WriteString(fmt.Sprintf(`<Override PartName="/ppt/slides/slide%d.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>`, i))
	}
	b.WriteString(`</Types>`)
	return b.String()
}

func pptxPresentationXML(slides int) string {
	var ids strings.Builder
	for i := 1; i <= slides; i++ {
		ids.WriteString(fmt.Sprintf(`<p:sldId id="%d" r:id="rId%d"/>`, 255+i, i))
	}
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:presentation xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId` + strconv.Itoa(slides+1) + `"/></p:sldMasterIdLst><p:sldIdLst>` + ids.String() + `</p:sldIdLst><p:sldSz cx="12192000" cy="6858000" type="screen16x9"/><p:notesSz cx="6858000" cy="9144000"/><p:defaultTextStyle><a:defPPr><a:defRPr lang="ko-KR"/></a:defPPr></p:defaultTextStyle></p:presentation>`
}

func pptxPresentationRels(slides int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for i := 1; i <= slides; i++ {
		b.WriteString(fmt.Sprintf(`<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide%d.xml"/>`, i, i))
	}
	b.WriteString(fmt.Sprintf(`<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideMaster" Target="slideMasters/slideMaster1.xml"/>`, slides+1))
	b.WriteString(`</Relationships>`)
	return b.String()
}

func writePPTXCommonParts(write func(string, string) error) error {
	parts := map[string]string{
		"ppt/slideMasters/slideMaster1.xml":            pptxSlideMasterXML(),
		"ppt/slideMasters/_rels/slideMaster1.xml.rels": pptxSlideMasterRels(),
		"ppt/slideLayouts/slideLayout1.xml":            pptxSlideLayoutXML(),
		"ppt/slideLayouts/_rels/slideLayout1.xml.rels": pptxSlideLayoutRels(),
		"ppt/theme/theme1.xml":                         pptxThemeXML(),
	}
	keys := make([]string, 0, len(parts))
	for key := range parts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := write(key, parts[key]); err != nil {
			return err
		}
	}
	return nil
}

func pptxSlideRels() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/></Relationships>`
}

func pptxSlideMasterXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sldMaster xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:cSld><p:bg><p:bgPr><a:solidFill><a:srgbClr val="FFFFFF"/></a:solidFill></p:bgPr></p:bg><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr></p:spTree></p:cSld><p:clrMap accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" bg1="lt1" bg2="lt2" folHlink="folHlink" hlink="hlink" tx1="dk1" tx2="dk2"/><p:sldLayoutIdLst><p:sldLayoutId id="1" r:id="rId1"/></p:sldLayoutIdLst><p:txStyles><p:titleStyle/><p:bodyStyle/><p:otherStyle/></p:txStyles></p:sldMaster>`
}

func pptxSlideMasterRels() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/theme" Target="../theme/theme1.xml"/></Relationships>`
}

func pptxSlideLayoutXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sldLayout xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" type="blank" preserve="1"><p:cSld name="Blank"><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr></p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sldLayout>`
}

func pptxSlideLayoutRels() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideMaster" Target="../slideMasters/slideMaster1.xml"/></Relationships>`
}

func pptxThemeXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" name="MeshClaw"><a:themeElements><a:clrScheme name="MeshClaw"><a:dk1><a:srgbClr val="111111"/></a:dk1><a:lt1><a:srgbClr val="FFFFFF"/></a:lt1><a:dk2><a:srgbClr val="1F2937"/></a:dk2><a:lt2><a:srgbClr val="F3F4F6"/></a:lt2><a:accent1><a:srgbClr val="2563EB"/></a:accent1><a:accent2><a:srgbClr val="059669"/></a:accent2><a:accent3><a:srgbClr val="DC2626"/></a:accent3><a:accent4><a:srgbClr val="7C3AED"/></a:accent4><a:accent5><a:srgbClr val="EA580C"/></a:accent5><a:accent6><a:srgbClr val="0891B2"/></a:accent6><a:hlink><a:srgbClr val="2563EB"/></a:hlink><a:folHlink><a:srgbClr val="7C3AED"/></a:folHlink></a:clrScheme><a:fontScheme name="MeshClaw"><a:majorFont><a:latin typeface="Arial"/><a:ea typeface="Apple SD Gothic Neo"/><a:cs typeface="Arial"/></a:majorFont><a:minorFont><a:latin typeface="Arial"/><a:ea typeface="Apple SD Gothic Neo"/><a:cs typeface="Arial"/></a:minorFont></a:fontScheme><a:fmtScheme name="MeshClaw"><a:fillStyleLst><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:gradFill rotWithShape="1"><a:gsLst><a:gs pos="0"><a:schemeClr val="phClr"/></a:gs><a:gs pos="100000"><a:schemeClr val="phClr"/></a:gs></a:gsLst><a:lin ang="5400000" scaled="0"/></a:gradFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:fillStyleLst><a:lnStyleLst><a:ln w="63500" cap="flat" cmpd="sng" algn="ctr"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln><a:ln w="127000" cap="flat" cmpd="sng" algn="ctr"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln><a:ln w="190500" cap="flat" cmpd="sng" algn="ctr"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln></a:lnStyleLst><a:effectStyleLst><a:effectStyle><a:effectLst/></a:effectStyle><a:effectStyle><a:effectLst/></a:effectStyle><a:effectStyle><a:effectLst/></a:effectStyle></a:effectStyleLst><a:bgFillStyleLst><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:bgFillStyleLst></a:fmtScheme></a:themeElements><a:objectDefaults/><a:extraClrSchemeLst/></a:theme>`
}

func pptxSlideXML(slide PresentationSlide, idx int) string {
	title := html.EscapeString(strings.TrimSpace(slide.Title))
	if title == "" {
		title = fmt.Sprintf("Slide %d", idx)
	}
	bullets := slide.Bullets
	if len(bullets) == 0 {
		bullets = []string{" "}
	}
	var bulletXML strings.Builder
	for _, bullet := range bullets {
		bullet = html.EscapeString(strings.TrimSpace(bullet))
		if bullet == "" {
			continue
		}
		bulletXML.WriteString(`<a:p><a:pPr marL="342900" indent="-171450"><a:buChar char="•"/></a:pPr><a:r><a:rPr lang="ko-KR" sz="2400"/><a:t>` + bullet + `</a:t></a:r></a:p>`)
	}
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>` +
		pptxTextBox(2, "Title", 685800, 457200, 10820400, 914400, title, true) +
		pptxTextBoxRaw(3, "Body", 914400, 1600200, 10363200, 4114800, bulletXML.String()) +
		`</p:spTree></p:cSld></p:sld>`
}

func pptxTextBox(id int, name string, x, y, cx, cy int, text string, title bool) string {
	size := "2800"
	bold := "0"
	if title {
		size = "4000"
		bold = "1"
	}
	body := `<a:p><a:r><a:rPr lang="ko-KR" sz="` + size + `" b="` + bold + `"/><a:t>` + text + `</a:t></a:r></a:p>`
	return pptxTextBoxRaw(id, name, x, y, cx, cy, body)
}

func pptxTextBoxRaw(id int, name string, x, y, cx, cy int, body string) string {
	return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="%s"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:noFill/></p:spPr><p:txBody><a:bodyPr wrap="square"/><a:lstStyle/>%s</p:txBody></p:sp>`, id, html.EscapeString(name), x, y, cx, cy, body)
}

func docxParagraph(text string, bold bool) string {
	text = html.EscapeString(strings.TrimSpace(text))
	if bold {
		return `<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>` + text + `</w:t></w:r></w:p>`
	}
	return `<w:p><w:r><w:t>` + text + `</w:t></w:r></w:p>`
}

func parseMarkdownTableRow(line string) ([]string, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") || isMarkdownTableSeparator(line) {
		return nil, false
	}
	raw := strings.Split(strings.Trim(line, "|"), "|")
	if len(raw) < 2 {
		return nil, false
	}
	cells := make([]string, 0, len(raw))
	for _, cell := range raw {
		cells = append(cells, strings.TrimSpace(cell))
	}
	return cells, true
}

func isMarkdownTableSeparator(line string) bool {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
		return false
	}
	for _, r := range strings.Trim(line, "| ") {
		if r != '-' && r != ':' && r != '|' && r != ' ' {
			return false
		}
	}
	return strings.Contains(line, "-")
}

func docxTable(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<w:tbl><w:tblPr><w:tblBorders><w:top w:val="single" w:sz="4"/><w:left w:val="single" w:sz="4"/><w:bottom w:val="single" w:sz="4"/><w:right w:val="single" w:sz="4"/><w:insideH w:val="single" w:sz="4"/><w:insideV w:val="single" w:sz="4"/></w:tblBorders></w:tblPr>`)
	for rowIndex, row := range rows {
		b.WriteString(`<w:tr>`)
		for _, cell := range row {
			b.WriteString(`<w:tc><w:p><w:r>`)
			if rowIndex == 0 {
				b.WriteString(`<w:rPr><w:b/></w:rPr>`)
			}
			b.WriteString(`<w:t>`)
			b.WriteString(html.EscapeString(strings.TrimSpace(cell)))
			b.WriteString(`</w:t></w:r></w:p></w:tc>`)
		}
		b.WriteString(`</w:tr>`)
	}
	b.WriteString(`</w:tbl>`)
	return b.String()
}

func safeArgosDocumentFilename(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "argos-document"
	}
	runes := []rune(out)
	if len(runes) > 60 {
		out = strings.Trim(string(runes[:60]), "-")
	}
	return out
}

func browserSearchDocument(query string, search browserauto.SearchResult) string {
	lines := []string{"Argos Browser Search", "", "Query: " + query, "", "Results:"}
	if search.Error != "" {
		lines = append(lines, "- Search error: "+search.Error)
	} else if len(search.Results) == 0 {
		lines = append(lines, "- No search results were returned.")
	} else {
		for i, item := range search.Results {
			if i >= 5 {
				break
			}
			lines = append(lines, fmt.Sprintf("%d. %s", i+1, strings.TrimSpace(item.Text)))
			if strings.TrimSpace(item.URL) != "" {
				lines = append(lines, "   "+strings.TrimSpace(item.URL))
			}
		}
	}
	lines = append(lines, "", "Saved by MeshClaw Argos at "+time.Now().Format(time.RFC3339))
	return strings.Join(lines, "\n")
}

func markdownBrowserSearch(body string) string {
	lines := strings.Split(body, "\n")
	var out []string
	for i, line := range lines {
		switch {
		case i == 0:
			out = append(out, "# "+strings.TrimSpace(line))
		case strings.HasSuffix(line, ":") && strings.TrimSpace(line) != "":
			out = append(out, "## "+strings.TrimSuffix(strings.TrimSpace(line), ":"))
		default:
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func saveBrowserSearchPreviewHTML(documentPath, query string, search browserauto.SearchResult) (string, error) {
	base := strings.TrimSuffix(documentPath, filepath.Ext(documentPath))
	path := base + ".html"
	findings := []string{}
	if search.Error != "" {
		findings = append(findings, "검색 오류: "+search.Error)
	} else {
		for i, item := range search.Results {
			if i >= 3 {
				break
			}
			if text := strings.TrimSpace(item.Text); text != "" {
				findings = append(findings, text)
			}
		}
	}
	htmlDoc := argosreport.RenderMobileHTML(argosreport.MobileReport{
		Eyebrow:  "Argos Browser Report",
		Title:    "검색 결과 정리",
		Subtitle: query,
		Flow:     []string{"검색 실행", "결과 수집", "중요 항목 정리", "링크 저장"},
		Findings: findings,
		Footer:   "MeshClaw Argos가 브라우저 검색 결과를 정리했습니다.",
	})
	if err := os.WriteFile(path, []byte(htmlDoc), 0600); err != nil {
		return "", err
	}
	_ = saveWorkDemoPreviewImage(path)
	return path, nil
}

func saveBrowserFetchDocument(ctx context.Context, page browserauto.Page) (workDemoDocumentFile, error) {
	findings := modelPageSummaryBullets(ctx, page)
	body := browserFetchDocument(page, findings)
	home, err := os.UserHomeDir()
	if err != nil {
		return workDemoDocumentFile{}, err
	}
	dir := filepath.Join(home, "Documents", "Argos Vault", "Work Reports")
	app := "Obsidian"
	if !appAvailableLocal("Obsidian") {
		dir = filepath.Join(home, "Documents", "Argos")
		app = "TextEdit"
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return workDemoDocumentFile{}, err
	}
	ext := ".md"
	content := markdownBrowserFetch(body)
	if app == "TextEdit" {
		ext = ".txt"
		content = body
	}
	path := filepath.Join(dir, "argos-page-"+time.Now().Format("20060102-150405")+ext)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return workDemoDocumentFile{}, err
	}
	preview, err := saveBrowserFetchPreviewHTML(path, page, findings)
	if err != nil {
		return workDemoDocumentFile{}, err
	}
	return workDemoDocumentFile{Path: path, PreviewPath: preview, Body: body, App: app}, nil
}

func browserFetchDocument(page browserauto.Page, findings []string) string {
	title := firstNonEmptyLocal(page.Title, page.FinalURL, page.URL)
	lines := []string{"Argos Page Report", "", "Page: " + title}
	if page.FinalURL != "" {
		lines = append(lines, "URL: "+page.FinalURL)
	} else if page.URL != "" {
		lines = append(lines, "URL: "+page.URL)
	}
	lines = append(lines, "", "Summary:")
	for _, bullet := range firstNonEmptySlice(findings, pageTextBullets(page.Text, 5)) {
		lines = append(lines, "- "+bullet)
	}
	if page.Text != "" {
		lines = append(lines, "", "Excerpt:", trimTextForReport(page.Text, 1600))
	}
	lines = append(lines, "", "Saved by MeshClaw Argos at "+time.Now().Format(time.RFC3339))
	return strings.Join(lines, "\n")
}

func markdownBrowserFetch(body string) string {
	lines := strings.Split(body, "\n")
	var out []string
	for i, line := range lines {
		switch {
		case i == 0:
			out = append(out, "# "+strings.TrimSpace(line))
		case strings.HasSuffix(line, ":") && strings.TrimSpace(line) != "":
			out = append(out, "## "+strings.TrimSuffix(strings.TrimSpace(line), ":"))
		default:
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func saveBrowserFetchPreviewHTML(documentPath string, page browserauto.Page, findings []string) (string, error) {
	base := strings.TrimSuffix(documentPath, filepath.Ext(documentPath))
	path := base + ".html"
	title := firstNonEmptyLocal(page.Title, "페이지 읽기 완료")
	subtitle := firstNonEmptyLocal(page.FinalURL, page.URL)
	findings = firstNonEmptySlice(findings, pageTextBullets(page.Text, 3))
	if len(findings) == 0 && page.Error != "" {
		findings = []string{"페이지 오류: " + page.Error}
	}
	htmlDoc := argosreport.RenderMobileHTML(argosreport.MobileReport{
		Eyebrow:  "Argos Page Report",
		Title:    "페이지 읽기 완료",
		Subtitle: firstNonEmptyLocal(title, subtitle),
		Flow:     []string{"페이지 열기", "본문 추출", "요약 정리", "링크 저장"},
		Findings: findings,
		Footer:   "MeshClaw Argos가 페이지 내용을 읽고 정리했습니다.",
	})
	if err := os.WriteFile(path, []byte(htmlDoc), 0600); err != nil {
		return "", err
	}
	_ = saveWorkDemoPreviewImage(path)
	return path, nil
}

func modelPageSummaryBullets(ctx context.Context, page browserauto.Page) []string {
	if !argosReportModelSummaryEnabled() || strings.TrimSpace(page.Text) == "" {
		return nil
	}
	cfg := aichat.DefaultConfig()
	cfg.BaseURL = firstNonEmptyLocal(strings.TrimSpace(os.Getenv("MESHCLAW_ARGOS_REPORT_BASE_URL")), strings.TrimSpace(os.Getenv("MESHCLAW_ASSISTANT_BASE_URL")), "http://g4:11434/v1")
	cfg.Model = firstNonEmptyLocal(strings.TrimSpace(os.Getenv("MESHCLAW_ARGOS_REPORT_MODEL")), strings.TrimSpace(os.Getenv("MESHCLAW_ASSISTANT_MODEL")), "gemma3:4b")
	cfg.APIKey = firstNonEmptyLocal(strings.TrimSpace(os.Getenv("MESHCLAW_ARGOS_REPORT_API_KEY")), "ollama")
	cfg.MaxTokens = 512
	cfg.Temperature = 0.1
	cfg.SystemPrompt = "너는 Argos iPhone 작업 보고서의 핵심 결과를 만드는 요약기다. 한국어로 짧고 구체적으로 쓴다. 과장, 이모지, 설명문 없이 결과만 출력한다."
	timeout := argosReportModelTimeout()
	summaryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	input := strings.Join([]string{
		"아래 페이지 내용을 iPhone 카드에 넣을 핵심 결과 3개로 요약해.",
		"규칙:",
		"- 각 줄은 45자 이내",
		"- 줄마다 '- '로 시작",
		"- 일반 설명 말고 페이지의 실제 내용만",
		"- 한국어로 출력",
		"",
		"제목: " + firstNonEmptyLocal(page.Title, page.FinalURL, page.URL),
		"본문:",
		trimTextForReport(page.Text, 4000),
	}, "\n")
	reply, err := aichat.NewClient(cfg).Chat(summaryCtx, nil, input)
	if err != nil {
		return nil
	}
	return parseSummaryBullets(reply, 3)
}

func argosReportModelSummaryEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("MESHCLAW_ARGOS_REPORT_MODEL_SUMMARY")))
	return value != "0" && value != "false" && value != "off" && value != "no"
}

func argosReportModelTimeout() time.Duration {
	value := strings.TrimSpace(os.Getenv("MESHCLAW_ARGOS_REPORT_MODEL_TIMEOUT"))
	if value == "" {
		return 8 * time.Second
	}
	if d, err := time.ParseDuration(value); err == nil && d > 0 {
		return d
	}
	if seconds, err := strconv.ParseFloat(value, 64); err == nil && seconds > 0 {
		return time.Duration(seconds * float64(time.Second))
	}
	return 8 * time.Second
}

func parseSummaryBullets(text string, limit int) []string {
	out := []string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "-")
		line = strings.TrimPrefix(line, "•")
		line = strings.TrimSpace(line)
		line = strings.Trim(line, "\"'`")
		if line == "" {
			continue
		}
		out = append(out, trimTextForReport(line, 90))
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func firstNonEmptySlice(primary, fallback []string) []string {
	if len(primary) > 0 {
		return primary
	}
	return fallback
}

func pageTextBullets(text string, limit int) []string {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return nil
	}
	parts := regexp.MustCompile(`[.!?。！？]\s+`).Split(text, -1)
	out := []string{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, trimTextForReport(part, 120))
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	if len(out) == 0 {
		out = append(out, trimTextForReport(text, 120))
	}
	return out
}

func trimTextForReport(text string, max int) string {
	text = strings.TrimSpace(text)
	if max <= 0 || len([]rune(text)) <= max {
		return text
	}
	runes := []rune(text)
	return strings.TrimSpace(string(runes[:max])) + "..."
}

func firstNonEmptyLocal(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstAvailableCommand(names ...string) string {
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

func normalizeMapsProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "google", "googlemaps", "google_maps":
		return "google"
	default:
		return "apple"
	}
}

func normalizeDirectionsMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "walk", "walking", "도보":
		return "walking"
	case "transit", "public", "public_transit", "대중교통":
		return "transit"
	default:
		return "driving"
	}
}

func mapsSearchURL(query, provider string) string {
	escaped := url.QueryEscape(query)
	if provider == "google" {
		return "https://www.google.com/maps/search/?api=1&query=" + escaped
	}
	return "https://maps.apple.com/?q=" + escaped
}

func mapsDirectionsURL(origin, destination, mode, provider string) string {
	if provider == "google" {
		params := url.Values{}
		params.Set("api", "1")
		if origin != "" {
			params.Set("origin", origin)
		}
		params.Set("destination", destination)
		params.Set("travelmode", mode)
		return "https://www.google.com/maps/dir/?" + params.Encode()
	}
	flag := "d"
	switch mode {
	case "walking":
		flag = "w"
	case "transit":
		flag = "r"
	}
	params := url.Values{}
	if origin != "" {
		params.Set("saddr", origin)
	}
	params.Set("daddr", destination)
	params.Set("dirflg", flag)
	return "https://maps.apple.com/?" + params.Encode()
}

func saveWorkDemoPreviewHTML(documentPath, query, body string) (string, error) {
	base := strings.TrimSuffix(documentPath, filepath.Ext(documentPath))
	path := base + ".html"
	htmlDoc := mobileWorkDemoHTML(query, body)
	if err := os.WriteFile(path, []byte(htmlDoc), 0600); err != nil {
		return "", err
	}
	_ = saveWorkDemoPreviewImage(path)
	return path, nil
}

func mobileWorkDemoHTML(query, body string) string {
	return argosreport.RenderMobileHTML(argosreport.MobileReport{
		Eyebrow:  "Argos Work Report",
		Title:    "Mac에서 작업 완료",
		Subtitle: query,
		Flow:     []string{"브라우저 검색", "결과 확인", "내용 정리", "문서 저장"},
		Findings: workDemoFindings(body, 3),
		Footer:   "MeshClaw Argos가 실행하고 Signal로 보고했습니다.",
	})
}

func workDemoFindings(body string, limit int) []string {
	lines := strings.Split(body, "\n")
	out := []string{}
	inSummary := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.EqualFold(strings.TrimSuffix(line, ":"), "Summary") {
			inSummary = true
			continue
		}
		if !inSummary || !strings.HasPrefix(line, "-") {
			continue
		}
		item := strings.TrimSpace(strings.TrimPrefix(line, "-"))
		if item == "" {
			continue
		}
		out = append(out, item)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func saveWorkDemoPreviewImage(htmlPath string) string {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("MESHCLAW_SKIP_PREVIEW_IMAGE")), "1") ||
		strings.EqualFold(strings.TrimSpace(os.Getenv("MESHCLAW_SKIP_PREVIEW_IMAGE")), "true") {
		return ""
	}
	if runtime.GOOS != "darwin" || strings.TrimSpace(htmlPath) == "" {
		return ""
	}
	out := htmlPath + ".png"
	if st, err := os.Stat(out); err == nil && !st.IsDir() && st.Size() > 0 {
		return out
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "qlmanage", "-t", "-s", "1200", "-o", filepath.Dir(htmlPath), htmlPath)
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	if err := cmd.Run(); err != nil {
		return ""
	}
	if st, err := os.Stat(out); err == nil && !st.IsDir() && st.Size() > 0 {
		return out
	}
	return ""
}

func markdownWorkDemo(body string) string {
	lines := strings.Split(body, "\n")
	var out []string
	for i, line := range lines {
		switch {
		case i == 0:
			out = append(out, "# "+strings.TrimSpace(line))
		case strings.HasSuffix(line, ":") && strings.TrimSpace(line) != "":
			out = append(out, "## "+strings.TrimSuffix(strings.TrimSpace(line), ":"))
		default:
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func OpenWorkDocument(ctx context.Context, path, app string) Result {
	if strings.TrimSpace(path) == "" {
		return failed("meshclaw_automation_open_work_document", "document path is required")
	}
	if strings.TrimSpace(app) == "" {
		app = "TextEdit"
	}
	if runtime.GOOS == "darwin" {
		if app == "Obsidian" {
			_ = OpenApp(ctx, "Obsidian")
			time.Sleep(demoStepDelay())
			return run(ctx, "meshclaw_automation_open_work_document", "open", "-a", "Obsidian", path)
		}
		return run(ctx, "meshclaw_automation_open_work_document", "open", "-a", app, path)
	}
	return failed("meshclaw_automation_open_work_document", "open work document is only implemented for macOS")
}

func appAvailableLocal(app string) bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	res := run(context.Background(), "meshclaw_automation_app_available", "osascript", "-e", fmt.Sprintf("id of app %s", appleScriptString(app)))
	return res.OK
}

func demoStepDelay() time.Duration {
	return durationFromEnv("MESHCLAW_ARGOS_DEMO_STEP_DELAY", 450*time.Millisecond)
}

func demoRevealDelay() time.Duration {
	return durationFromEnv("MESHCLAW_ARGOS_DEMO_REVEAL_DELAY", 700*time.Millisecond)
}

func wantsVisibleBrowser(lower string) bool {
	return containsAnyLocal(lower, "브라우저", "사파리", "safari", "화면", "열어서", "띄워", "보이게", "visible", "open browser", "in browser")
}

func AIHandoff(ctx context.Context, opts AIHandoffOptions) Result {
	provider := normalizeProvider(opts.Provider)
	prompt := strings.TrimSpace(opts.Prompt)
	if prompt == "" {
		return failed("meshclaw_automation_ai_handoff", "handoff prompt is required")
	}
	clip := SetClipboard(ctx, prompt)
	result := Result{
		Kind:      "meshclaw_automation_ai_handoff",
		Action:    "ai_handoff",
		Provider:  provider,
		Prompt:    prompt,
		OK:        clip.OK,
		Stdout:    clip.Stdout,
		Stderr:    clip.Stderr,
		Error:     clip.Error,
		CreatedAt: time.Now().UTC(),
	}
	if !clip.OK {
		return result
	}
	target := providerTarget(provider)
	result.URL = target.URL
	result.App = target.App
	switch {
	case target.App != "":
		opened := OpenApp(ctx, target.App)
		result.Command = opened.Command
		result.OK = opened.OK
		result.Stdout = strings.TrimSpace(strings.Join(nonEmpty(clip.Stdout, opened.Stdout), "\n"))
		result.Stderr = strings.TrimSpace(strings.Join(nonEmpty(clip.Stderr, opened.Stderr), "\n"))
		result.Error = opened.Error
	case target.URL != "":
		opened := OpenURL(ctx, target.URL)
		result.Command = opened.Command
		result.OK = opened.OK
		result.Stdout = strings.TrimSpace(strings.Join(nonEmpty(clip.Stdout, opened.Stdout), "\n"))
		result.Stderr = strings.TrimSpace(strings.Join(nonEmpty(clip.Stderr, opened.Stderr), "\n"))
		result.Error = opened.Error
		if result.OK && shouldAutoSubmitAI(provider) {
			time.Sleep(aiWebLoadDelay())
			paste := UIRunnerKey(ctx, "v", "command")
			result.Stdout = strings.TrimSpace(strings.Join(nonEmpty(result.Stdout, paste.Stdout), "\n"))
			if !paste.OK {
				result.OK = false
				result.Error = paste.Error
				return result
			}
			time.Sleep(aiSubmitDelay())
			enter := UIRunnerKey(ctx, "return")
			result.Stdout = strings.TrimSpace(strings.Join(nonEmpty(result.Stdout, enter.Stdout), "\n"))
			if !enter.OK {
				result.OK = false
				result.Error = enter.Error
			}
		}
	default:
		result.OK = false
		result.Error = "unknown AI handoff provider"
	}
	return result
}

func shouldAutoSubmitAI(provider string) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("MESHCLAW_ARGOS_AI_AUTOSUBMIT")))
	if value == "0" || value == "false" || value == "no" {
		return false
	}
	return provider == "chatgpt"
}

func aiWebLoadDelay() time.Duration {
	return durationFromEnv("MESHCLAW_ARGOS_AI_LOAD_DELAY", 1200*time.Millisecond)
}

func aiSubmitDelay() time.Duration {
	return durationFromEnv("MESHCLAW_ARGOS_AI_SUBMIT_DELAY", 100*time.Millisecond)
}

func durationFromEnv(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	if d, err := time.ParseDuration(value); err == nil && d >= 0 {
		return d
	}
	return fallback
}

func FrontendsDoctor(ctx context.Context) FrontendsReport {
	providers := []string{"codex", "claude", "chatgpt"}
	report := FrontendsReport{
		Kind:      "meshclaw_subscription_frontends",
		OK:        true,
		CreatedAt: time.Now().UTC(),
	}
	for _, provider := range providers {
		target := providerTarget(provider)
		status := FrontendStatus{
			Provider: provider,
			App:      target.App,
			URL:      target.URL,
			Mode:     "subscription_frontend",
		}
		switch {
		case target.App != "":
			status.Available, status.Error = appAvailable(ctx, target.App)
			if !status.Available && target.URL != "" {
				status.NextActions = append(status.NextActions, "Install or open the "+target.App+" app, or use the browser URL fallback.")
			} else if !status.Available {
				status.NextActions = append(status.NextActions, "Install and log into "+target.App+" on this Mac.")
			}
		case target.URL != "":
			status.Available = runtime.GOOS == "darwin" || runtime.GOOS == "linux"
			if !status.Available {
				status.Error = "open-url is not implemented for this OS"
			}
		default:
			status.Error = "no target configured"
		}
		if !status.Available {
			report.OK = false
		}
		report.Frontends = append(report.Frontends, status)
	}
	return report
}

type providerTargetInfo struct {
	App string
	URL string
}

func normalizeProvider(provider string) string {
	p := strings.ToLower(strings.TrimSpace(provider))
	p = strings.ReplaceAll(p, " ", "")
	p = strings.ReplaceAll(p, "-", "")
	switch p {
	case "codex", "openai", "코덱스":
		return "codex"
	case "claude", "anthropic", "클로드":
		return "claude"
	case "chatgpt", "gpt", "openaiweb", "챗지피티", "지피티":
		return "chatgpt"
	case "1code", "onecode":
		return "1code"
	default:
		if p == "" {
			return "chatgpt"
		}
		return p
	}
}

func providerTarget(provider string) providerTargetInfo {
	switch provider {
	case "codex":
		return providerTargetInfo{App: "Codex"}
	case "claude":
		return providerTargetInfo{App: "Claude", URL: "https://claude.ai/new"}
	case "chatgpt":
		return providerTargetInfo{URL: "https://chatgpt.com/"}
	case "1code":
		return providerTargetInfo{App: "1Code"}
	default:
		return providerTargetInfo{URL: "https://chatgpt.com/"}
	}
}

func parseAIHandoff(text string) (string, string, bool) {
	lower := strings.ToLower(text)
	providers := []string{"codex", "코덱스", "claude", "클로드", "chatgpt", "지피티", "챗지피티"}
	for _, marker := range providers {
		idx := strings.Index(lower, strings.ToLower(marker))
		if idx < 0 {
			continue
		}
		if !containsAny(lower, []string{"넘겨", "물어", "질문", "프롬프트", "handoff", "ask"}) {
			continue
		}
		provider := normalizeProvider(marker)
		prompt := strings.TrimSpace(text[idx+len(marker):])
		prompt = strings.TrimLeft(prompt, " :：에게로한테")
		for _, sep := range []string{":", "："} {
			if parts := strings.SplitN(prompt, sep, 2); len(parts) == 2 {
				prompt = strings.TrimSpace(parts[1])
				break
			}
		}
		prompt = strings.TrimSpace(strings.TrimPrefix(prompt, "로"))
		prompt = strings.TrimSpace(strings.TrimPrefix(prompt, "에게"))
		prompt = strings.TrimSpace(strings.TrimPrefix(prompt, "물어봐"))
		prompt = strings.TrimSpace(strings.TrimPrefix(prompt, "넘겨줘"))
		if prompt == "" {
			prompt = strings.TrimSpace(text)
		}
		return provider, prompt, true
	}
	return "", "", false
}

func parseWorkDemo(text string) (string, bool) {
	lower := strings.ToLower(text)
	if !containsAny(lower, []string{"데모", "시연", "여러가지", "여러 가지", "work demo", "demo"}) {
		return "", false
	}
	if !strings.Contains(lower, "작업 데모") && !containsAny(lower, []string{"검색", "브라우저", "파일", "저장", "문서", "edit", "save", "browser", "search"}) {
		return "", false
	}
	query := defaultArgosDemoQuery()
	for _, marker := range []string{"주제는", "검색어는", "query:", "topic:", "about:"} {
		if idx := strings.Index(lower, strings.ToLower(marker)); idx >= 0 {
			query = strings.TrimSpace(text[idx+len(marker):])
			break
		}
	}
	query = strings.Trim(query, " \t\r\n:：.-")
	if query == "" {
		query = defaultArgosDemoQuery()
	}
	return query, true
}

func defaultArgosDemoQuery() string {
	if value := strings.TrimSpace(os.Getenv("MESHCLAW_ARGOS_DEMO_QUERY")); value != "" {
		return value
	}
	return "가나 경제 뉴스"
}

func parseShortcut(text string) (string, string, bool) {
	lower := strings.ToLower(text)
	if !strings.Contains(lower, "단축어") && !strings.Contains(lower, "shortcut") {
		return "", "", false
	}
	name := text
	input := ""
	if idx := strings.Index(lower, "단축어"); idx >= 0 {
		before := strings.TrimSpace(text[:idx])
		after := strings.TrimSpace(text[idx+len("단축어"):])
		if before != "" {
			name = before
		} else {
			name = after
		}
	}
	for _, suffix := range []string{"실행해줘", "실행해", "실행", "run", "켜줘", "해줘"} {
		name = strings.TrimSpace(strings.TrimSuffix(name, suffix))
	}
	if parts := strings.SplitN(name, " 입력 ", 2); len(parts) == 2 {
		name = strings.TrimSpace(parts[0])
		input = strings.TrimSpace(parts[1])
	}
	name = strings.Trim(name, " \"'`")
	return name, input, name != ""
}

func parseNote(text string) (string, string, bool) {
	lower := strings.ToLower(text)
	if !containsAny(lower, []string{"메모", "노트", "notes", "note"}) {
		return "", "", false
	}
	if !containsAny(lower, []string{"메모", "적어", "써", "기록", "저장", "만들", "작성", "write", "save", "create"}) {
		return "", "", false
	}
	body := text
	for _, marker := range []string{"내용은", "내용:", "본문은", "본문:", "메모해줘", "메모해", "적어줘", "적어", "기록해줘", "기록해"} {
		if idx := strings.Index(strings.ToLower(body), strings.ToLower(marker)); idx >= 0 {
			body = strings.TrimSpace(body[idx+len(marker):])
			break
		}
	}
	title := inferNoteTitle(text)
	if value, ok := extractField(text, []string{"제목은", "제목:", "title:"}, []string{"내용은", "본문은", "내용:", "본문:", "body:"}); ok {
		title = value
	}
	body = strings.Trim(body, " \t\r\n:：.-")
	return title, body, body != ""
}

func inferNoteTitle(text string) string {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	for _, marker := range []string{"메모", "노트", "notes", "note"} {
		if idx := strings.Index(strings.ToLower(normalized), strings.ToLower(marker)); idx > 0 {
			candidate := strings.TrimSpace(normalized[:idx])
			candidate = strings.Trim(candidate, " \t\r\n:：.-\"'`“”‘’")
			if candidate != "" && !containsAny(strings.ToLower(candidate), []string{"내용은", "본문은", "write", "save", "create"}) {
				return candidate
			}
		}
	}
	return "Argos Note"
}

func parseReminder(text string, now time.Time) (string, string, string, bool) {
	lower := strings.ToLower(text)
	if !containsAny(lower, []string{"리마인더", "알림", "알려줘", "알려 줘", "reminder", "remind me", "todo", "할 일", "할일"}) {
		return "", "", "", false
	}
	if !containsAny(lower, []string{"추가", "등록", "만들", "알려", "기억", "remind", "add", "create"}) {
		return "", "", "", false
	}
	title := text
	for _, marker := range []string{"제목은", "제목:", "내용은", "내용:", "할 일은", "할일은", "리마인더", "알림", "reminder", "remind me to", "remind me", "todo"} {
		if idx := strings.Index(strings.ToLower(title), strings.ToLower(marker)); idx >= 0 {
			before := strings.TrimSpace(title[:idx])
			after := strings.TrimSpace(title[idx+len(marker):])
			if before != "" && (marker == "리마인더" || marker == "알림" || marker == "reminder" || marker == "todo") {
				title = before
			} else {
				title = after
			}
			break
		}
	}
	title = stripReminderCommandWords(title)
	due, dueText := parseReminderDue(now, text)
	if dueText != "" {
		title = strings.TrimSpace(strings.ReplaceAll(title, dueText, " "))
	}
	title = stripReminderCommandWords(title)
	title = strings.TrimSpace(strings.TrimPrefix(title, "에 "))
	title = strings.TrimSpace(strings.TrimPrefix(title, "에"))
	if title == "" {
		if dueText == "" {
			return "", "", "", false
		}
		title = "알림"
	}
	dueISO := ""
	if !due.IsZero() {
		dueISO = due.Format(time.RFC3339)
	}
	notes := ""
	if dueText != "" {
		notes = "Signal 요청: " + strings.TrimSpace(text)
	}
	return title, notes, dueISO, true
}

func parseReminderList(text string, now time.Time) (string, string, string, bool) {
	lower := strings.ToLower(text)
	if !containsAny(lower, []string{"리마인더", "알림", "할 일", "할일", "todo", "reminder", "reminders"}) {
		return "", "", "", false
	}
	if containsAny(lower, []string{"추가", "등록", "만들", "기억", "remind me", "add", "create"}) {
		return "", "", "", false
	}
	if !containsAny(lower, []string{"뭐", "보여", "알려", "확인", "조회", "목록", "있어", "list", "show", "check", "what"}) {
		return "", "", "", false
	}
	start := beginningOfDay(now)
	end := start.Add(24 * time.Hour)
	if containsAny(lower, []string{"내일", "tomorrow"}) {
		start = beginningOfDay(now).Add(24 * time.Hour)
		end = start.Add(24 * time.Hour)
	} else if containsAny(lower, []string{"모레"}) {
		start = beginningOfDay(now).Add(48 * time.Hour)
		end = start.Add(24 * time.Hour)
	} else if containsAny(lower, []string{"이번주", "이번 주", "week"}) {
		start = beginningOfDay(now)
		end = start.Add(7 * 24 * time.Hour)
	}
	query := ""
	if value, ok := extractField(text, []string{"검색어는", "검색:", "query:"}, nil); ok {
		query = value
	}
	return query, start.Format(time.RFC3339), end.Format(time.RFC3339), true
}

func parseReminderMutation(text string) (string, string, bool) {
	lower := strings.ToLower(text)
	if !containsAny(lower, []string{"리마인더", "알림", "할 일", "할일", "todo", "reminder", "reminders"}) {
		return "", "", false
	}
	mutation := ""
	switch {
	case containsAny(lower, []string{"완료", "끝냈", "끝내", "complete", "done"}):
		mutation = "complete"
	case containsAny(lower, []string{"삭제", "지워", "delete", "remove"}):
		mutation = "delete"
	default:
		return "", "", false
	}
	query := text
	for _, marker := range []string{"리마인더", "알림", "할 일", "할일", "todo", "reminder", "reminders"} {
		query = strings.ReplaceAll(query, marker, " ")
	}
	for _, marker := range []string{"완료해줘", "완료해", "완료", "끝냈어", "끝내줘", "삭제해줘", "삭제해", "삭제", "지워줘", "지워", "complete", "done", "delete", "remove"} {
		query = strings.ReplaceAll(strings.ToLower(query), strings.ToLower(marker), " ")
	}
	query = strings.Trim(query, " \t\r\n:：.-")
	query = strings.Join(strings.Fields(query), " ")
	return mutation, query, query != ""
}

func parseContactsSearch(text string) (string, bool) {
	lower := strings.ToLower(text)
	if !containsAny(lower, []string{"연락처", "주소록", "contact", "contacts"}) {
		return "", false
	}
	if !containsAny(lower, []string{"찾아", "검색", "조회", "알려", "전화", "이메일", "email", "phone", "search", "find", "lookup"}) {
		return "", false
	}
	query := text
	for _, marker := range []string{"연락처에서", "주소록에서", "연락처", "주소록", "contacts", "contact"} {
		query = strings.ReplaceAll(query, marker, " ")
	}
	for _, marker := range []string{"찾아줘", "찾아", "검색해줘", "검색", "조회해줘", "조회", "알려줘", "알려", "전화번호", "전화", "이메일", "email", "phone", "search", "find", "lookup"} {
		query = strings.ReplaceAll(strings.ToLower(query), strings.ToLower(marker), " ")
	}
	for _, marker := range []string{"아무나", "아무 사람", "아무 연락처", "한 명", "한명", "누구든", "random", "anyone", "someone"} {
		query = strings.ReplaceAll(strings.ToLower(query), strings.ToLower(marker), " ")
	}
	query = strings.Trim(query, " \t\r\n:：.-")
	query = strings.Join(strings.Fields(query), " ")
	return query, query != ""
}

func isVagueContactsRequest(text string) bool {
	lower := strings.ToLower(text)
	if !containsAny(lower, []string{"연락처", "주소록", "contact", "contacts"}) {
		return false
	}
	if !containsAny(lower, []string{"찾아", "검색", "조회", "알려", "전화", "이메일", "email", "phone", "search", "find", "lookup"}) {
		return false
	}
	_, ok := parseContactsSearch(text)
	return !ok
}

func isExplicitNoteRequest(text string) bool {
	lower := strings.ToLower(text)
	return containsAny(lower, []string{"notes", "note", "메모", "노트", "내용은", "내용:", "제목은", "제목:"})
}

func parseCalendarList(text string, now time.Time) (string, string, string, bool) {
	lower := strings.ToLower(text)
	if !containsAny(lower, []string{"캘린더", "달력", "일정", "calendar", "schedule", "agenda"}) {
		return "", "", "", false
	}
	if !containsAny(lower, []string{"뭐", "보여", "알려", "확인", "조회", "있어", "list", "show", "check", "what"}) {
		return "", "", "", false
	}
	start := beginningOfDay(now)
	end := start.Add(24 * time.Hour)
	if containsAny(lower, []string{"내일", "tomorrow"}) {
		start = beginningOfDay(now).Add(24 * time.Hour)
		end = start.Add(24 * time.Hour)
	} else if containsAny(lower, []string{"모레"}) {
		start = beginningOfDay(now).Add(48 * time.Hour)
		end = start.Add(24 * time.Hour)
	} else if containsAny(lower, []string{"이번주", "이번 주", "week"}) {
		start = beginningOfDay(now)
		end = start.Add(7 * 24 * time.Hour)
	}
	query := ""
	if value, ok := extractField(text, []string{"검색어는", "검색:", "query:"}, nil); ok {
		query = value
	}
	return query, start.Format(time.RFC3339), end.Format(time.RFC3339), true
}

func parseCalendarDelete(text string, now time.Time) (string, string, string, bool) {
	lower := strings.ToLower(text)
	if !containsAny(lower, []string{"캘린더", "달력", "일정", "calendar", "event", "meeting", "회의", "약속"}) {
		return "", "", "", false
	}
	if !containsAny(lower, []string{"삭제", "지워", "취소", "delete", "remove", "cancel"}) {
		return "", "", "", false
	}
	start, matched := parseReminderDue(now, text)
	if start.IsZero() {
		start = beginningOfDay(now)
	}
	end := start.Add(24 * time.Hour)
	query := text
	if matched != "" {
		query = strings.ReplaceAll(query, matched, " ")
	}
	for _, marker := range []string{"캘린더", "달력", "일정", "calendar", "event", "meeting", "회의", "약속"} {
		query = strings.ReplaceAll(query, marker, " ")
	}
	for _, marker := range []string{"삭제해줘", "삭제해", "삭제", "지워줘", "지워", "취소해줘", "취소해", "취소", "delete", "remove", "cancel"} {
		query = strings.ReplaceAll(strings.ToLower(query), strings.ToLower(marker), " ")
	}
	query = strings.Trim(query, " \t\r\n:：.-")
	query = strings.Join(strings.Fields(query), " ")
	return query, start.Format(time.RFC3339), end.Format(time.RFC3339), query != ""
}

func parseCalendarEvent(text string, now time.Time) (string, string, string, string, bool) {
	lower := strings.ToLower(text)
	if !containsAny(lower, []string{"캘린더", "달력", "일정", "calendar", "event", "meeting", "회의", "약속"}) {
		return "", "", "", "", false
	}
	if !containsAny(lower, []string{"추가", "등록", "만들", "잡아", "예약", "add", "create", "schedule"}) {
		return "", "", "", "", false
	}
	title := text
	quotedTitle := firstQuotedCalendarTitle(text)
	if quotedTitle != "" {
		title = quotedTitle
	} else {
		for _, marker := range []string{"제목은", "제목:", "내용은", "내용:", "일정", "캘린더", "달력", "calendar", "event", "meeting"} {
			if idx := strings.Index(strings.ToLower(title), strings.ToLower(marker)); idx >= 0 {
				before := strings.TrimSpace(title[:idx])
				after := strings.TrimSpace(title[idx+len(marker):])
				if before != "" && (marker == "일정" || marker == "calendar" || marker == "event" || marker == "meeting") {
					title = before
				} else {
					title = after
				}
				break
			}
		}
	}
	start, matched := parseReminderDue(now, text)
	if start.IsZero() {
		return "", "", "", "", false
	}
	if matched != "" {
		title = strings.TrimSpace(strings.ReplaceAll(title, matched, " "))
	}
	title = stripCalendarCommandWords(title)
	title = strings.TrimSpace(strings.TrimPrefix(title, "에 "))
	title = strings.TrimSpace(strings.TrimPrefix(title, "에"))
	if title == "" {
		title = "Argos 일정"
	}
	end := start.Add(time.Hour)
	notes := "Signal 요청: " + strings.TrimSpace(text)
	return title, notes, start.Format(time.RFC3339), end.Format(time.RFC3339), true
}

func firstQuotedCalendarTitle(text string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`'([^']{1,80})'`),
		regexp.MustCompile(`"([^"]{1,80})"`),
		regexp.MustCompile(`“([^”]{1,80})”`),
		regexp.MustCompile(`‘([^’]{1,80})’`),
		regexp.MustCompile(`「([^」]{1,80})」`),
		regexp.MustCompile(`『([^』]{1,80})』`),
	}
	for _, pattern := range patterns {
		match := pattern.FindStringSubmatch(text)
		if len(match) == 2 {
			title := strings.TrimSpace(match[1])
			if title != "" {
				return title
			}
		}
	}
	return ""
}

func stripCalendarCommandWords(text string) string {
	out := strings.TrimSpace(text)
	replacements := []string{
		"캘린더에", "달력에", "일정으로",
		"일정 추가해줘", "일정 추가", "일정 등록해줘", "일정 등록", "일정 만들어줘",
		"add calendar event", "create calendar event", "calendar event",
		"add", "create", "schedule",
	}
	for _, marker := range replacements {
		out = strings.ReplaceAll(out, marker, " ")
	}
	for _, suffix := range []string{"해줘", "해", "추가", "등록", "만들어", "잡아줘", "잡아", "예약"} {
		out = strings.TrimSpace(strings.TrimSuffix(out, suffix))
	}
	out = strings.Join(strings.Fields(out), " ")
	return strings.Trim(out, " \t\r\n:：.-")
}

func stripReminderCommandWords(text string) string {
	out := strings.TrimSpace(text)
	replacements := []string{
		"리마인더 추가해줘", "리마인더 추가", "리마인더 등록해줘", "리마인더 등록", "리마인더 만들어줘", "리마인더 만들",
		"알림 추가해줘", "알림 추가", "알림 등록해줘", "알림 등록", "알려줘", "기억해줘",
		"reminder", "remind me to", "remind me", "add", "create", "todo",
	}
	for _, marker := range replacements {
		out = strings.ReplaceAll(out, marker, " ")
	}
	for _, suffix := range []string{"해줘", "해", "추가", "등록", "만들어", "만들기"} {
		out = strings.TrimSpace(strings.TrimSuffix(out, suffix))
	}
	out = strings.TrimSpace(strings.TrimSuffix(out, "이면 돼"))
	out = strings.TrimSpace(strings.TrimSuffix(out, "이면 됩니다"))
	out = strings.TrimSpace(strings.TrimSuffix(out, "면 돼"))
	out = strings.TrimSpace(strings.TrimSuffix(out, "면 됩니다"))
	out = strings.TrimSpace(strings.TrimSuffix(out, "이면 되"))
	out = strings.TrimSpace(strings.TrimSuffix(out, "면 되"))
	return strings.Trim(out, " \t\r\n:：.-")
}

func parseReminderDue(now time.Time, text string) (time.Time, string) {
	lower := strings.ToLower(text)
	if due, matched := parseRelativeReminderDue(now, lower); matched != "" {
		return due, matched
	}
	base := time.Time{}
	matchedDate := ""
	switch {
	case strings.Contains(lower, "내일"):
		base = now.AddDate(0, 0, 1)
		matchedDate = "내일"
	case strings.Contains(lower, "모레"):
		base = now.AddDate(0, 0, 2)
		matchedDate = "모레"
	case strings.Contains(lower, "오늘"):
		base = now
		matchedDate = "오늘"
	}
	if base.IsZero() {
		return time.Time{}, ""
	}
	hour, minute, matchedTime := reminderHourMinute(lower)
	if matchedTime == "" {
		hour, minute = 9, 0
	}
	due := time.Date(base.Year(), base.Month(), base.Day(), hour, minute, 0, 0, now.Location())
	matched := strings.TrimSpace(strings.Join([]string{matchedDate, matchedTime}, " "))
	return due, matched
}

func parseRelativeReminderDue(now time.Time, lower string) (time.Time, string) {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`([0-9]{1,3})\s*(분|시간)\s*(?:뒤|후|있다가)`),
		regexp.MustCompile(`in\s+([0-9]{1,3})\s*(minutes|minute|min|hours|hour|hrs|hr)`),
	}
	for _, pattern := range patterns {
		match := pattern.FindStringSubmatch(lower)
		if len(match) < 3 {
			continue
		}
		amount, err := strconv.Atoi(match[1])
		if err != nil || amount <= 0 {
			continue
		}
		unit := strings.ToLower(match[2])
		var duration time.Duration
		switch unit {
		case "분", "minute", "minutes", "min":
			duration = time.Duration(amount) * time.Minute
		case "시간", "hour", "hours", "hr", "hrs":
			duration = time.Duration(amount) * time.Hour
		default:
			continue
		}
		return now.Add(duration), strings.TrimSpace(match[0])
	}
	return time.Time{}, ""
}

func beginningOfDay(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
}

func reminderHourMinute(lower string) (int, int, string) {
	re := regexp.MustCompile(`(오전|오후|am|pm)?\s*([0-9]{1,2})\s*시(?:\s*([0-9]{1,2})\s*분)?`)
	match := re.FindStringSubmatch(lower)
	if len(match) == 0 {
		return 0, 0, ""
	}
	hour, _ := strconv.Atoi(match[2])
	minute := 0
	if match[3] != "" {
		minute, _ = strconv.Atoi(match[3])
	}
	ampm := strings.TrimSpace(match[1])
	if (ampm == "오후" || ampm == "pm") && hour < 12 {
		hour += 12
	}
	if (ampm == "오전" || ampm == "am") && hour == 12 {
		hour = 0
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, ""
	}
	return hour, minute, strings.TrimSpace(match[0])
}

func parseOpenApp(text string) string {
	lower := strings.ToLower(text)
	if !containsAny(lower, []string{"열어", "켜", "실행", "open", "launch"}) {
		return ""
	}
	apps := map[string]string{
		"safari": "Safari", "사파리": "Safari",
		"chrome": "Google Chrome", "크롬": "Google Chrome",
		"notes": "Notes", "note": "Notes", "메모": "Notes", "노트": "Notes",
		"textedit": "TextEdit", "텍스트": "TextEdit", "메모장": "TextEdit",
		"finder": "Finder", "파인더": "Finder",
		"mail": "Mail", "메일": "Mail",
		"calendar": "Calendar", "캘린더": "Calendar",
		"terminal": "Terminal", "터미널": "Terminal",
		"calculator": "Calculator", "calc": "Calculator", "계산기": "Calculator",
		"preview": "Preview", "미리보기": "Preview",
		"photos": "Photos", "사진": "Photos",
		"music": "Music", "음악": "Music",
		"messages": "Messages", "메시지": "Messages", "문자": "Messages",
		"facetime": "FaceTime", "페이스타임": "FaceTime",
		"system settings": "System Settings", "시스템 설정": "System Settings", "설정": "System Settings",
		"signal": "Signal", "시그널": "Signal",
		"claude": "Claude", "클로드": "Claude",
		"codex": "Codex", "코덱스": "Codex",
		"ollama": "Ollama", "올라마": "Ollama",
	}
	for key, app := range apps {
		if strings.Contains(lower, key) {
			return app
		}
	}
	candidate := cleanOpenAppCandidate(text)
	if candidate == "" {
		return ""
	}
	return candidate
}

func cleanOpenAppCandidate(text string) string {
	out := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	for _, marker := range []string{"열어줘", "열어", "켜줘", "켜", "실행해줘", "실행해", "실행", "open", "launch"} {
		out = strings.ReplaceAll(strings.ToLower(out), strings.ToLower(marker), " ")
	}
	for _, marker := range []string{"앱", "어플", "application", "app"} {
		out = strings.ReplaceAll(strings.ToLower(out), strings.ToLower(marker), " ")
	}
	out = strings.Join(strings.Fields(strings.Trim(out, " \t\r\n:：.-\"'`“”‘’")), " ")
	if out == "" || containsAny(out, []string{"브라우저", "검색", "연락처", "메모", "리마인더", "캘린더", "일정"}) {
		return ""
	}
	return out
}

func parseMacRunnerCommand(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return false
	}
	return containsAny(lower, []string{
		"맥에서", "맥으로", "등록된 맥", "등록된 mac", "맥북에서", "맥북으로", "아이맥에서", "아이맥으로",
		"mac runner", "mac-runner", "device runner", "device-runner", "macbook", "imac",
	}) &&
		containsAny(lower, []string{"실행", "처리", "작성", "열어", "등록", "추가", "조작", "run", "execute"})
}

func parseMacRunnerDoctor(text string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return "", false
	}
	if !containsAny(lower, []string{"mac runner", "mac-runner", "device runner", "device-runner", "맥 러너", "맥러너", "맥 실행기", "실행기", "등록된 맥"}) {
		return "", false
	}
	if !containsAny(lower, []string{"doctor", "check", "status", "ready", "진단", "점검", "상태", "체크", "준비"}) {
		return "", false
	}
	words := strings.Fields(lower)
	for i, word := range words {
		if (word == "runner" || word == "실행기" || word == "맥러너" || word == "mac-runner" || word == "device-runner") && i+1 < len(words) {
			next := strings.Trim(words[i+1], ".,:;!?()[]{}\"'")
			if next != "" && !containsAny(next, []string{"상태", "진단", "점검", "체크", "준비", "status", "doctor", "check", "ready"}) {
				return next, true
			}
		}
	}
	return "", true
}

func parseDocumentCreate(text string) (string, string, bool) {
	lower := strings.ToLower(strings.TrimSpace(text))
	if !containsAny(lower, []string{"pages", "페이지", "문서", "보고서", "회의록", "초안"}) ||
		!containsAny(lower, []string{"작성", "만들", "저장", "초안", "create", "make", "save"}) {
		return "", "", false
	}
	if containsAny(lower, []string{"메모", "노트", "note"}) {
		return "", "", false
	}
	if containsAny(lower, []string{"맥에서", "맥으로", "등록된 맥", "등록된 mac", "맥북에서", "맥북으로", "아이맥에서", "아이맥으로", "mac runner", "mac-runner", "device runner", "device-runner", "macbook", "imac"}) {
		return "", "", false
	}
	title := extractAfterMarkers(text, []string{"제목은", "제목:", "문서명은", "문서명:", "title:"})
	body := extractAfterMarkers(text, []string{"내용은", "본문은", "내용:", "본문:", "body:"})
	if title == "" {
		title = "Argos 문서"
	}
	if body == "" {
		body = strings.TrimSpace(text)
	}
	return cleanDocumentField(title), cleanDocumentField(body), true
}

func extractAfterMarkers(text string, markers []string) string {
	for _, marker := range markers {
		idx := strings.Index(text, marker)
		if idx < 0 {
			continue
		}
		out := strings.TrimSpace(text[idx+len(marker):])
		for _, stop := range []string{"내용은", "본문은", "내용:", "본문:", "제목은", "제목:", "문서명은", "문서명:"} {
			if stop == marker {
				continue
			}
			if stopIdx := strings.Index(out, stop); stopIdx > 0 {
				out = strings.TrimSpace(out[:stopIdx])
			}
		}
		return out
	}
	return ""
}

func cleanDocumentField(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, ".。 \n\t")
	return value
}

func parseSearch(text string) (string, bool) {
	lower := strings.ToLower(text)
	if !containsAny(lower, []string{"검색", "찾아봐", "찾아줘", "조사", "웹에서", "브라우저로", "브라우저에서", "search"}) {
		return "", false
	}
	query := stripKnownPrefixes(text, []string{"브라우저로", "브라우저에서", "웹에서", "인터넷에서", "구글에서", "검색해줘", "검색해", "찾아봐", "찾아줘", "조사해줘", "조사해", "search web for", "search for", "search"})
	query = stripSearchCommandWords(query)
	for _, suffix := range []string{"검색해줘", "검색해", "검색", "찾아봐", "찾아줘", "찾아", "조사해줘", "조사해", "조사", "해줘"} {
		query = strings.TrimSpace(strings.TrimSuffix(query, suffix))
	}
	query = collapseRepeatedQuery(strings.Trim(query, " \"'`"))
	return query, query != ""
}

func stripSearchCommandWords(text string) string {
	out := strings.TrimSpace(text)
	replacements := []string{"브라우저로", "브라우저에서", "웹에서", "인터넷에서", "구글에서", "검색해줘", "검색해", "검색", "찾아봐", "찾아줘", "찾아", "조사해줘", "조사해", "조사", "search web for", "search for", "web for", "search"}
	for _, marker := range replacements {
		out = strings.ReplaceAll(out, marker, " ")
	}
	return strings.Join(strings.Fields(out), " ")
}

func collapseRepeatedQuery(text string) string {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields)%2 != 0 || len(fields) == 0 {
		return strings.Join(fields, " ")
	}
	half := len(fields) / 2
	for i := 0; i < half; i++ {
		if !strings.EqualFold(fields[i], fields[i+half]) {
			return strings.Join(fields, " ")
		}
	}
	return strings.Join(fields[:half], " ")
}

func firstURL(text string) string {
	re := regexp.MustCompile(`https?://[^\s]+|[A-Za-z0-9.-]+\.[A-Za-z]{2,}[^\s]*`)
	value := strings.TrimRight(re.FindString(text), ".,)")
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Host != "" {
		return parsed.String()
	}
	return ""
}

func stripKnownPrefixes(text string, prefixes []string) string {
	out := strings.TrimSpace(text)
	for _, prefix := range prefixes {
		out = strings.TrimSpace(strings.TrimPrefix(out, prefix))
	}
	return out
}

func extractField(text string, starts []string, stops []string) (string, bool) {
	startIndex := -1
	startLen := 0
	lower := strings.ToLower(text)
	for _, marker := range starts {
		if idx := strings.Index(lower, strings.ToLower(marker)); idx >= 0 && (startIndex == -1 || idx < startIndex) {
			startIndex = idx
			startLen = len(marker)
		}
	}
	if startIndex < 0 {
		return "", false
	}
	valueStart := startIndex + startLen
	valueEnd := len(text)
	lowerTail := strings.ToLower(text[valueStart:])
	for _, marker := range stops {
		if idx := strings.Index(lowerTail, strings.ToLower(marker)); idx >= 0 && valueStart+idx < valueEnd {
			valueEnd = valueStart + idx
		}
	}
	value := strings.Trim(text[valueStart:valueEnd], " \t\r\n:：.-")
	return value, value != ""
}

func containsAny(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func appleScriptString(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\n", "<br>")
	return "\"" + value + "\""
}

func appAvailable(ctx context.Context, app string) (bool, string) {
	if runtime.GOOS != "darwin" {
		return false, "app detection is only implemented for macOS"
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "open", "-Ra", app)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return false, msg
	}
	return true, ""
}

func nonEmpty(values ...string) []string {
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func MouseClick(ctx context.Context, runnerURL string, x, y float64) Result {
	runnerURL = strings.TrimRight(strings.TrimSpace(runnerURL), "/")
	if runnerURL == "" {
		runnerURL = "http://127.0.0.1:48292"
	}
	payload, _ := json.Marshal(map[string]float64{"x": x, "y": y})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, runnerURL+"/click", bytes.NewReader(payload))
	if err != nil {
		return failed("meshclaw_automation_mouse_click", err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return failed("meshclaw_automation_mouse_click", err.Error())
	}
	defer resp.Body.Close()
	var decoded map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&decoded)
	result := Result{Kind: "meshclaw_automation_mouse_click", Action: "mouse_click", OK: resp.StatusCode >= 200 && resp.StatusCode < 300, CreatedAt: time.Now().UTC()}
	if !result.OK {
		result.Error = fmt.Sprintf("ui runner returned %s", resp.Status)
	}
	if len(decoded) > 0 {
		data, _ := json.Marshal(decoded)
		result.Stdout = string(data)
	}
	return result
}

func ScriptRun(ctx context.Context, shell, script string) Result {
	shell = strings.TrimSpace(shell)
	if shell == "" {
		shell = "/bin/sh"
	}
	if strings.TrimSpace(script) == "" {
		return failed("meshclaw_automation_script_run", "script is required")
	}
	return run(ctx, "meshclaw_automation_script_run", shell, "-lc", script)
}

func run(ctx context.Context, kind, name string, args ...string) Result {
	return runWithInput(ctx, kind, "", name, args...)
}

func runWithInput(ctx context.Context, kind, input, name string, args ...string) Result {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if blockTestGUICommand(name) {
		return Result{
			Kind:      kind,
			Action:    strings.TrimPrefix(kind, "meshclaw_automation_"),
			Command:   append([]string{name}, args...),
			OK:        false,
			Error:     "external GUI command blocked during go test; set MESHCLAW_ALLOW_GUI_IN_TESTS=1 for integration tests",
			CreatedAt: time.Now().UTC(),
		}
	}
	cmd := exec.CommandContext(ctx, name, args...)
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := runCommandWithContext(ctx, cmd)
	result := Result{
		Kind:      kind,
		Action:    strings.TrimPrefix(kind, "meshclaw_automation_"),
		Command:   append([]string{name}, args...),
		OK:        err == nil,
		Stdout:    strings.TrimSpace(stdout.String()),
		Stderr:    strings.TrimSpace(stderr.String()),
		CreatedAt: time.Now().UTC(),
	}
	if err != nil {
		result.Error = err.Error()
		if ctx.Err() != nil {
			result.Error = ctx.Err().Error()
		}
	}
	return result
}

func blockTestGUICommand(name string) bool {
	if os.Getenv("MESHCLAW_ALLOW_GUI_IN_TESTS") == "1" || !runningUnderGoTest() {
		return false
	}
	switch filepath.Base(strings.TrimSpace(name)) {
	case "open", "xdg-open", "osascript", "shortcuts":
		return true
	default:
		return false
	}
}

func runningUnderGoTest() bool {
	return strings.HasSuffix(filepath.Base(os.Args[0]), ".test")
}

func runMacRunnerCommandOrLocal(ctx context.Context, action ArgosAction, local func() Result) Result {
	if macRunnerRemoteEnabledFor(action.Action) {
		return RunMacRunnerCommand(ctx, action.Text)
	}
	return local()
}

func macRunnerRemoteEnabledFor(action string) bool {
	if macRunnerSSHTarget() == "" {
		return false
	}
	switch action {
	case "calendar_event_create", "calendar_events_list", "calendar_event_delete",
		"contacts_search", "mac_runner_command", "note_create", "open_app", "open_url",
		"reminder_complete", "reminder_create", "reminder_delete", "reminders_list",
		"shortcut_run", "visible_browser_search":
		return true
	default:
		return false
	}
}

func RunMacRunnerCommand(ctx context.Context, text string) Result {
	target := macRunnerSSHTarget()
	if target == "" {
		return failed("meshclaw_automation_mac_runner_command", "mac runner ssh target is not configured")
	}
	project := firstNonEmpty(
		macRunnerProject(),
	)
	if project == "" {
		project = "/Users/example/Documents/New project"
	}
	python := firstNonEmpty(
		macRunnerPython(),
	)
	if python == "" {
		python = "python3"
	}
	script := "cd " + shellQuote(project) + " && " + shellQuote(python) + " scripts/signal_bridge.py local-command --execute " + shellQuote(text)
	cmd := exec.CommandContext(ctx, "ssh", target, script)
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := runCommandWithContext(ctx, cmd)
	result := Result{
		Kind:      "meshclaw_automation_mac_runner_command",
		Action:    "mac_runner_command",
		Command:   []string{"ssh", target, script},
		OK:        err == nil,
		Stdout:    strings.TrimSpace(stdout.String()),
		Stderr:    strings.TrimSpace(stderr.String()),
		CreatedAt: time.Now().UTC(),
	}
	if err != nil {
		result.Error = err.Error()
		if ctx.Err() != nil {
			result.Error = ctx.Err().Error()
		}
		return result
	}
	if strings.TrimSpace(result.Stdout) != "" {
		var payload struct {
			Success bool   `json:"success"`
			Reply   string `json:"reply"`
		}
		if json.Unmarshal([]byte(result.Stdout), &payload) == nil {
			result.OK = payload.Success
			if !payload.Success {
				result.Error = firstNonEmpty(strings.TrimSpace(payload.Reply), "mac runner command failed")
			}
		}
	}
	if result.OK {
		remotePath, remotePDFPath := macRunnerCommandSavedArtifacts(result.Stdout)
		if remotePath != "" {
			localPath, previewPath, artifactErr := fetchMacRunnerCommandArtifact(ctx, target, remotePath)
			if localPath != "" {
				result.URL = localPath
			}
			if previewPath != "" {
				result.Preview = previewPath
			}
			if artifactErr != nil {
				result.Stderr = strings.TrimSpace(strings.Join(nonEmpty(result.Stderr, "artifact copy warning: "+artifactErr.Error()), "\n"))
			}
		}
		if remotePDFPath != "" {
			localPDFPath, _, artifactErr := fetchMacRunnerCommandArtifact(ctx, target, remotePDFPath)
			if localPDFPath != "" {
				result.PDF = localPDFPath
			}
			if artifactErr != nil {
				result.Stderr = strings.TrimSpace(strings.Join(nonEmpty(result.Stderr, "pdf copy warning: "+artifactErr.Error()), "\n"))
			}
		}
	}
	return result
}

func macRunnerCommandSavedPath(stdout string) string {
	path, _ := macRunnerCommandSavedArtifacts(stdout)
	return path
}

func macRunnerCommandSavedArtifacts(stdout string) (string, string) {
	stdout = strings.TrimSpace(stdout)
	if stdout == "" {
		return "", ""
	}
	var payload struct {
		CommandResult struct {
			Result struct {
				SavedTo    string `json:"saved_to"`
				PDFSavedTo string `json:"pdf_saved_to"`
			} `json:"result"`
		} `json:"command_result"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err == nil {
		if path := strings.TrimSpace(payload.CommandResult.Result.SavedTo); path != "" {
			return path, strings.TrimSpace(payload.CommandResult.Result.PDFSavedTo)
		}
	}
	var direct struct {
		Result struct {
			SavedTo    string `json:"saved_to"`
			PDFSavedTo string `json:"pdf_saved_to"`
		} `json:"result"`
		SavedTo    string `json:"saved_to"`
		PDFSavedTo string `json:"pdf_saved_to"`
	}
	if err := json.Unmarshal([]byte(stdout), &direct); err == nil {
		return firstNonEmpty(strings.TrimSpace(direct.Result.SavedTo), strings.TrimSpace(direct.SavedTo)),
			firstNonEmpty(strings.TrimSpace(direct.Result.PDFSavedTo), strings.TrimSpace(direct.PDFSavedTo))
	}
	return "", ""
}

func fetchMacRunnerCommandArtifact(ctx context.Context, target, remotePath string) (string, string, error) {
	remotePath = strings.TrimSpace(remotePath)
	if target == "" || remotePath == "" {
		return "", "", nil
	}
	dir := filepath.Join(defaultMeshClawDir(), "signal-attachments", "mac-runner")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", "", err
	}
	stamp := time.Now().UTC().Format("20060102T150405Z")
	ext := strings.ToLower(filepath.Ext(remotePath))
	if ext == "" {
		ext = ".dat"
	}
	localPath := filepath.Join(dir, fmt.Sprintf("argos-document-%s%s", stamp, ext))
	out, err := os.Create(localPath)
	if err != nil {
		return "", "", err
	}
	cmd := exec.CommandContext(ctx, "ssh", target, "cat "+shellQuote(remotePath))
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	var stderr bytes.Buffer
	cmd.Stdout = out
	cmd.Stderr = &stderr
	err = runCommandWithContext(ctx, cmd)
	closeErr := out.Close()
	if err != nil {
		_ = os.Remove(localPath)
		return "", "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if closeErr != nil {
		_ = os.Remove(localPath)
		return "", "", closeErr
	}
	if st, err := os.Stat(localPath); err != nil || st.IsDir() || st.Size() == 0 {
		_ = os.Remove(localPath)
		return "", "", fmt.Errorf("copied artifact is empty")
	}
	preview, previewErr := macRunnerArtifactPreview(ctx, localPath)
	if previewErr != nil {
		return localPath, "", previewErr
	}
	return localPath, preview, nil
}

func macRunnerArtifactPreview(ctx context.Context, path string) (string, error) {
	if runtime.GOOS != "darwin" || strings.TrimSpace(path) == "" {
		return "", nil
	}
	dir := filepath.Dir(path)
	started := time.Now().Add(-2 * time.Second)
	cmd := exec.CommandContext(ctx, "qlmanage", "-t", "-s", "1200", "-o", dir, path)
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := runCommandWithContext(ctx, cmd); err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	candidates, _ := filepath.Glob(filepath.Join(dir, filepath.Base(path)+"*.png"))
	for _, candidate := range candidates {
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() && st.Size() > 0 {
			return signalFriendlyJPEGPreview(ctx, candidate, started)
		}
	}
	candidates, _ = filepath.Glob(filepath.Join(dir, "*.png"))
	var newest string
	var newestTime time.Time
	for _, candidate := range candidates {
		st, err := os.Stat(candidate)
		if err != nil || st.IsDir() || st.Size() == 0 || st.ModTime().Before(started) {
			continue
		}
		if newest == "" || st.ModTime().After(newestTime) {
			newest = candidate
			newestTime = st.ModTime()
		}
	}
	if newest != "" {
		return signalFriendlyJPEGPreview(ctx, newest, started)
	}
	return "", nil
}

func signalFriendlyJPEGPreview(ctx context.Context, pngPath string, started time.Time) (string, error) {
	if strings.TrimSpace(pngPath) == "" {
		return "", nil
	}
	dir := filepath.Dir(pngPath)
	out := filepath.Join(dir, fmt.Sprintf("argos-proof-%s.jpg", time.Now().UTC().Format("20060102T150405Z")))
	cmd := exec.CommandContext(ctx, "sips", "-s", "format", "jpeg", pngPath, "--out", out)
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := runCommandWithContext(ctx, cmd); err != nil {
		return pngPath, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if st, err := os.Stat(out); err == nil && !st.IsDir() && st.Size() > 0 && !st.ModTime().Before(started) {
		_ = os.Remove(pngPath)
		return out, nil
	}
	return pngPath, nil
}

func macRunnerSSHTarget() string {
	if target := firstNonEmpty(
		strings.TrimSpace(os.Getenv("MESHCLAW_ARGOS_MAC_RUNNER_SSH_TARGET")),
		strings.TrimSpace(os.Getenv("MESHCLAW_MAC_RUNNER_SSH_TARGET")),
	); target != "" {
		return target
	}
	if runner, ok := SelectedMacRunner(); ok {
		return runner.SSHTarget
	}
	return ""
}

func macRunnerProject() string {
	if project := firstNonEmpty(
		strings.TrimSpace(os.Getenv("MESHCLAW_ARGOS_MAC_RUNNER_PROJECT")),
		strings.TrimSpace(os.Getenv("MESHCLAW_MAC_RUNNER_PROJECT")),
	); project != "" {
		return project
	}
	if runner, ok := SelectedMacRunner(); ok {
		return runner.Project
	}
	return ""
}

func macRunnerPython() string {
	if python := firstNonEmpty(
		strings.TrimSpace(os.Getenv("MESHCLAW_ARGOS_MAC_RUNNER_PYTHON")),
		strings.TrimSpace(os.Getenv("MESHCLAW_MAC_RUNNER_PYTHON")),
	); python != "" {
		return python
	}
	if runner, ok := SelectedMacRunner(); ok {
		return runner.Python
	}
	return ""
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func runCommandWithContext(ctx context.Context, cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		killCommandProcess(cmd)
		<-done
		return ctx.Err()
	}
}

func killCommandProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if runtime.GOOS != "windows" {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	_ = cmd.Process.Kill()
}

func failed(kind, message string) Result {
	return Result{Kind: kind, Action: strings.TrimPrefix(kind, "meshclaw_automation_"), OK: false, Error: message, CreatedAt: time.Now().UTC()}
}
