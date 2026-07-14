package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/meshclaw/meshclaw/internal/assistantbrief"
	"github.com/meshclaw/meshclaw/internal/assistantprofile"
	"github.com/meshclaw/meshclaw/internal/assistantwatch"
	"github.com/meshclaw/meshclaw/internal/datadoctor"
	"github.com/meshclaw/meshclaw/internal/evidence"
	"github.com/meshclaw/meshclaw/internal/mailadapter"
	"github.com/meshclaw/meshclaw/internal/messenger"
	"github.com/meshclaw/meshclaw/internal/monitor"
	"github.com/meshclaw/meshclaw/internal/openwebui"
	"github.com/meshclaw/meshclaw/internal/osauto"
)

type Job struct {
	ID            string   `json:"id"`
	Mode          string   `json:"mode"`
	Kind          string   `json:"kind"`
	Description   string   `json:"description"`
	Interval      string   `json:"interval,omitempty"`
	DailyAt       string   `json:"daily_at,omitempty"`
	HourlyAt      string   `json:"hourly_at,omitempty"`
	RecommendedOn []string `json:"recommended_on,omitempty"`
	TargetID      string   `json:"target_id,omitempty"`
	Enabled       bool     `json:"enabled"`
}

type Plan struct {
	Kind       string   `json:"kind"`
	Generated  string   `json:"generated"`
	Controller string   `json:"controller"`
	Jobs       []Job    `json:"jobs"`
	Notes      []string `json:"notes"`
}

type RunOptions struct {
	JobID   string
	Target  string
	Execute bool
	Now     time.Time
}

type RunResult struct {
	Kind       string                `json:"kind"`
	Job        Job                   `json:"job"`
	StartedAt  time.Time             `json:"started_at"`
	FinishedAt time.Time             `json:"finished_at"`
	Success    bool                  `json:"success"`
	Status     string                `json:"status"`
	Summary    string                `json:"summary"`
	Message    string                `json:"message,omitempty"`
	Payload    interface{}           `json:"payload,omitempty"`
	Evidence   evidence.Record       `json:"evidence"`
	StoreError string                `json:"store_error,omitempty"`
	Send       *messenger.SendResult `json:"send,omitempty"`
	SendError  string                `json:"send_error,omitempty"`
}

type State struct {
	Kind    string               `json:"kind"`
	Path    string               `json:"path"`
	LastRun map[string]time.Time `json:"last_run"`
}

type ServerOpsPayload struct {
	States      map[string]*monitor.NodeState `json:"states"`
	Alerts      []monitor.Alert               `json:"alerts"`
	OpenWebUI   *openwebui.ScanReport         `json:"openwebui,omitempty"`
	Recommended []string                      `json:"recommended,omitempty"`
}

type ExpertAuditPayload struct {
	Scope             string   `json:"scope"`
	RecommendedAI     []string `json:"recommended_ai"`
	Prompt            string   `json:"prompt"`
	Inputs            []string `json:"inputs"`
	Next              []string `json:"next"`
	ServerOnline      int      `json:"server_online"`
	ServerTotal       int      `json:"server_total"`
	Alerts            int      `json:"alerts"`
	OpenWebUIFailures int      `json:"openwebui_failures"`
	RecentEvidence    int      `json:"recent_evidence"`
	Problems          []string `json:"problems,omitempty"`
}

type MailWatchPayload struct {
	Results     []mailadapter.WatchResult `json:"results"`
	Errors      []string                  `json:"errors,omitempty"`
	SeenErrors  []string                  `json:"seen_errors,omitempty"`
	NewMessages int                       `json:"new_messages"`
	SeenSkipped int                       `json:"seen_skipped"`
}

type MailWatchState struct {
	Kind      string              `json:"kind"`
	Path      string              `json:"path"`
	Seen      map[string][]string `json:"seen"`
	Errors    map[string]string   `json:"errors,omitempty"`
	UpdatedAt time.Time           `json:"updated_at"`
}

type AssistantWatchPayload struct {
	Result assistantwatch.CheckResult `json:"result"`
}

type ArgosHealthPayload struct {
	Doctor    osauto.ArgosMacDoctorReport `json:"doctor"`
	Dashboard DashboardRefresh            `json:"dashboard,omitempty"`
}

type DashboardRefresh struct {
	Attempted bool     `json:"attempted"`
	OK        bool     `json:"ok"`
	Path      string   `json:"path,omitempty"`
	Command   []string `json:"command,omitempty"`
	Error     string   `json:"error,omitempty"`
}

type AssistantAutoCheckPayload struct {
	ProfileStatus string   `json:"profile_status"`
	SkillCount    int      `json:"skill_count"`
	MemoryStatus  string   `json:"memory_status"`
	MemoryLayers  []string `json:"memory_layers"`
	SelfTestLine  string   `json:"self_test_line"`
}

type LocalHygienePayload struct {
	Home          string                `json:"home"`
	RemovedFiles  []LocalHygieneRemoved `json:"removed_files,omitempty"`
	RemovedBytes  int64                 `json:"removed_bytes"`
	PublicFiles   int                   `json:"public_files"`
	EvidenceFiles int                   `json:"evidence_files"`
	LogFiles      int                   `json:"log_files"`
	LogBytes      int64                 `json:"log_bytes"`
	Warnings      []string              `json:"warnings,omitempty"`
	Skipped       []string              `json:"skipped,omitempty"`
	Retention     map[string]string     `json:"retention"`
}

type LocalAIBriefingPayload struct {
	Command  []string `json:"command"`
	Execute  bool     `json:"execute"`
	TargetID string   `json:"target_id,omitempty"`
	Stdout   string   `json:"stdout,omitempty"`
	Stderr   string   `json:"stderr,omitempty"`
}

type LocalHygieneRemoved struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
	Age   string `json:"age"`
}

func DefaultPlan() Plan {
	return Plan{
		Kind:       "meshclaw_schedule_plan",
		Generated:  time.Now().UTC().Format(time.RFC3339),
		Controller: controllerLane(),
		Jobs: []Job{
			{
				ID:          "serverops-quickcheck",
				Mode:        "ops",
				Kind:        "serverops_quickcheck",
				Description: "Read-only fleet health, alert, and Open WebUI router check for local models.",
				Interval:    "3h",
				TargetID:    "argos-ops",
				Enabled:     true,
			},
			{
				ID:          "local-ai-briefing",
				Mode:        "briefing",
				Kind:        "local_ai_briefing",
				Description: "Rich local-AI report with balanced news, server status, and mail metadata for the approved Signal report room.",
				HourlyAt:    "07",
				TargetID:    "argos-briefing",
				Enabled:     true,
			},
			{
				ID:          "morning-briefing",
				Mode:        "briefing",
				Kind:        "assistant_morning",
				Description: "Fallback daily weather/news morning briefing; disabled while local-ai-briefing is the preferred report path.",
				DailyAt:     "08:00",
				TargetID:    "argos-briefing",
				Enabled:     false,
			},
			{
				ID:          "mail-watch",
				Mode:        "assistant",
				Kind:        "mail_watch",
				Description: "Check configured mail accounts for recent messages and produce a redacted notification payload.",
				Interval:    "15m",
				TargetID:    "argos-briefing",
				Enabled:     true,
			},
			{
				ID:          "assistant-watch",
				Mode:        "assistant",
				Kind:        "assistant_watch",
				Description: "Check user-created assistant watches such as price alerts and topic reminders.",
				Interval:    "1h",
				TargetID:    "argos-briefing",
				Enabled:     true,
			},
			{
				ID:          "argos-health",
				Mode:        "assistant",
				Kind:        "argos_health",
				Description: "Check Argos macOS assistant readiness and stable UI Runner permission health.",
				Interval:    "1h",
				TargetID:    "argos-briefing",
				Enabled:     true,
			},
			{
				ID:          "assistant-auto-check",
				Mode:        "assistant",
				Kind:        "assistant_auto_check",
				Description: "Read-only Argos assistant profile, memory, and reply contract self-check for the interactive assistant room, eight times per day.",
				Interval:    "3h",
				TargetID:    "argos-assistant",
				Enabled:     true,
			},
			{
				ID:          "local-hygiene",
				Mode:        "ops",
				Kind:        "local_hygiene",
				Description: "Bounded local data hygiene for public links, temporary recordings, logs, and evidence growth warnings.",
				Interval:    "12h",
				TargetID:    "argos-ops",
				Enabled:     true,
			},
			{
				ID:          "data-doctor",
				Mode:        "ops",
				Kind:        "data_doctor",
				Description: "Read-only MeshClaw data growth, retention, log, and evidence health check.",
				Interval:    "6h",
				Enabled:     true,
			},
			{
				ID:            "daily-expert-audit",
				Mode:          "ops",
				Kind:          "expert_audit_handoff",
				Description:   "Create a Codex/Claude handoff prompt for a deeper once-daily MCP audit.",
				DailyAt:       "09:00",
				TargetID:      "argos-ops",
				Enabled:       true,
				RecommendedOn: []string{"codex", "claude"},
			},
		},
		Notes: []string{
			"Local models and Open WebUI run frequent low-cost checks.",
			"Codex and Claude remain expert operators for deeper daily review or escalation.",
			"Signal/Argos is a delivery and approval surface; it is not the source of authority.",
			"All scheduled work is dry-run unless the caller passes --execute.",
		},
	}
}

func RunOnce(ctx context.Context, opts RunOptions) (RunResult, error) {
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	job, err := findJob(opts.JobID)
	if err != nil {
		return RunResult{}, err
	}
	targetRequested := opts.Target != "" || opts.Execute
	if opts.Target != "" {
		job.TargetID = opts.Target
	}
	result := RunResult{Kind: "meshclaw_schedule_run", Job: job, StartedAt: opts.Now}
	switch job.Kind {
	case "serverops_quickcheck":
		result.Payload, result.Summary, err = runServerOpsQuickcheck()
	case "assistant_morning":
		result.Payload, result.Summary, err = runMorningBrief(ctx)
	case "local_ai_briefing":
		result.Payload, result.Summary, err = runLocalAIBriefing(ctx, opts.Execute, job.TargetID)
	case "mail_watch":
		result.Payload, result.Summary, err = runMailWatch()
	case "assistant_watch":
		result.Payload, result.Summary, err = runAssistantWatch(ctx, opts.Now)
	case "argos_health":
		result.Payload, result.Summary, err = runArgosHealth(ctx)
	case "assistant_auto_check":
		result.Payload, result.Summary, err = runAssistantAutoCheck(opts.Now)
	case "local_hygiene":
		result.Payload, result.Summary, err = runLocalHygiene(opts.Now)
	case "data_doctor":
		result.Payload, result.Summary, err = runDataDoctor(opts.Now)
	case "expert_audit_handoff":
		result.Payload, result.Summary, err = runExpertAuditHandoff()
	default:
		err = fmt.Errorf("unsupported scheduled job kind %q", job.Kind)
	}
	result.FinishedAt = time.Now().UTC()
	warningOnly := isDataDoctorWarning(result, err)
	result.Success = err == nil || warningOnly
	if warningOnly {
		result.Status = "warning"
	} else if err != nil {
		result.Status = "failed"
		result.Summary = firstNonEmpty(result.Summary, err.Error())
	} else {
		result.Status = "ok"
	}
	record, storeErr := evidence.Store("schedule-"+job.Kind, job.Mode, result.Summary, result)
	result.Evidence = record
	if storeErr != nil {
		result.StoreError = storeErr.Error()
	}
	result.Message = formatScheduleMessage(result)
	if targetRequested && job.TargetID != "" && shouldSendScheduleResult(result) {
		send, sendErr := messenger.Send(messenger.SendOptions{
			TargetID: job.TargetID,
			Kind:     "text",
			Text:     result.Message,
			Execute:  opts.Execute,
		})
		result.Send = &send
		if sendErr != nil {
			result.SendError = sendErr.Error()
		}
	}
	return result, err
}

func shouldSendScheduleResult(result RunResult) bool {
	switch result.Job.Kind {
	case "mail_watch":
		if !result.Success {
			return true
		}
		payload, ok := result.Payload.(MailWatchPayload)
		if !ok {
			return true
		}
		if len(payload.Errors) > 0 {
			return true
		}
		for _, item := range payload.Results {
			if len(item.Messages) > 0 {
				return true
			}
		}
		return false
	case "assistant_watch":
		payload, ok := result.Payload.(AssistantWatchPayload)
		if !ok {
			return true
		}
		return len(payload.Result.Matched) > 0
	case "argos_health":
		if !result.Success {
			return true
		}
		payload, ok := result.Payload.(ArgosHealthPayload)
		if !ok {
			return true
		}
		return !payload.Doctor.OK || len(payload.Doctor.Problems) > 0
	case "assistant_auto_check":
		return shouldSendAssistantAutoCheck(result)
	case "local_hygiene":
		if !result.Success {
			return true
		}
		payload, ok := result.Payload.(LocalHygienePayload)
		if !ok {
			return true
		}
		return len(payload.Warnings) > 0 || len(payload.Skipped) > 0 || payload.RemovedBytes >= 50*1024*1024
	case "data_doctor":
		// Data-doctor output is internal retention/accounting state. Keep it in
		// evidence for audit, but do not push raw counters to Signal report rooms.
		// User-facing Signal reports should be briefing/server-status style.
		return false
	case "local_ai_briefing":
		// The local-AI briefing pipeline owns its rich multi-part Signal delivery
		// and HTML attachment. The scheduler tracks evidence/state only.
		return false
	default:
		return true
	}
}

func shouldSendAssistantAutoCheck(result RunResult) bool {
	if !result.Success {
		return true
	}
	payload, ok := result.Payload.(AssistantAutoCheckPayload)
	if !ok {
		return true
	}
	if strings.TrimSpace(payload.ProfileStatus) != "ok" {
		return true
	}
	switch strings.TrimSpace(payload.MemoryStatus) {
	case "", "ok", "empty":
	default:
		return true
	}
	if payload.SkillCount <= 0 {
		return true
	}
	return assistantAutoCheckSelfTestHasWarning(payload.SelfTestLine)
}

func assistantAutoCheckSelfTestHasWarning(line string) bool {
	line = strings.ToLower(strings.TrimSpace(line))
	if line == "" {
		return false
	}
	if strings.Contains(line, "주의 0개") || strings.Contains(line, "warnings=0") || strings.Contains(line, "warn=0") {
		return false
	}
	return strings.Contains(line, "주의") || strings.Contains(line, "warning") || strings.Contains(line, "warn")
}

func RunDue(ctx context.Context, opts RunOptions) ([]RunResult, error) {
	plan := DefaultPlan()
	state, _ := LoadState()
	var results []RunResult
	var firstErr error
	for _, job := range plan.Jobs {
		if !job.Enabled {
			continue
		}
		if !Due(job, state, opts.Now) {
			continue
		}
		runOpts := opts
		runOpts.JobID = job.ID
		result, err := RunOnce(ctx, runOpts)
		results = append(results, result)
		state.LastRun[job.ID] = result.FinishedAt
		if err != nil && firstErr == nil && !isDataDoctorWarning(result, err) {
			firstErr = err
		}
	}
	if err := SaveState(state); err != nil && firstErr == nil {
		firstErr = err
	}
	return results, firstErr
}

func isDataDoctorWarning(result RunResult, err error) bool {
	if err == nil || result.Job.Kind != "data_doctor" {
		return false
	}
	report, ok := result.Payload.(datadoctor.Report)
	return ok && len(report.Warnings) > 0
}

func Due(job Job, state State, now time.Time) bool {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	last := state.LastRun[job.ID]
	if job.Interval != "" {
		if last.IsZero() {
			return true
		}
		interval, err := time.ParseDuration(job.Interval)
		if err != nil || interval <= 0 {
			return true
		}
		return now.Sub(last) >= interval
	}
	if job.DailyAt != "" {
		return dailyDue(job.DailyAt, last, now)
	}
	if job.HourlyAt != "" {
		return hourlyDue(job.HourlyAt, last, now)
	}
	return true
}

func NextDue(job Job, state State, now time.Time) time.Time {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	last := state.LastRun[job.ID]
	if job.Interval != "" {
		if last.IsZero() {
			return now
		}
		interval, err := time.ParseDuration(job.Interval)
		if err != nil || interval <= 0 {
			return now
		}
		next := last.Add(interval)
		if next.Before(now) {
			return now
		}
		return next
	}
	if job.DailyAt != "" {
		next, ok := nextDailyAtAfterLastRun(job.DailyAt, last, now)
		if !ok {
			return now
		}
		return next
	}
	if job.HourlyAt != "" {
		next, ok := nextHourlyAtAfterLastRun(job.HourlyAt, last, now)
		if !ok {
			return now
		}
		return next
	}
	return now
}

func LoadState() (State, error) {
	path := StatePath()
	state := State{Kind: "meshclaw_schedule_state", Path: path, LastRun: map[string]time.Time{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, err
	}
	state.Kind = "meshclaw_schedule_state"
	state.Path = path
	if state.LastRun == nil {
		state.LastRun = map[string]time.Time{}
	}
	return state, nil
}

func SaveState(state State) error {
	if state.Path == "" {
		state.Path = StatePath()
	}
	state.Kind = "meshclaw_schedule_state"
	if state.LastRun == nil {
		state.LastRun = map[string]time.Time{}
	}
	if err := os.MkdirAll(filepath.Dir(state.Path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(state.Path, append(data, '\n'), 0600)
}

func LoadMailWatchState() (MailWatchState, error) {
	path := MailWatchStatePath()
	state := MailWatchState{Kind: "meshclaw_mail_watch_state", Path: path, Seen: map[string][]string{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, err
	}
	state.Kind = "meshclaw_mail_watch_state"
	state.Path = path
	if state.Seen == nil {
		state.Seen = map[string][]string{}
	}
	if state.Errors == nil {
		state.Errors = map[string]string{}
	}
	return state, nil
}

func SaveMailWatchState(state MailWatchState) error {
	if state.Path == "" {
		state.Path = MailWatchStatePath()
	}
	state.Kind = "meshclaw_mail_watch_state"
	state.UpdatedAt = time.Now().UTC()
	if state.Seen == nil {
		state.Seen = map[string][]string{}
	}
	if state.Errors == nil {
		state.Errors = map[string]string{}
	}
	if err := os.MkdirAll(filepath.Dir(state.Path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(state.Path, append(data, '\n'), 0600)
}

func runServerOpsQuickcheck() (interface{}, string, error) {
	m, err := monitor.New(monitor.DefaultConfig())
	if err != nil {
		return nil, "", err
	}
	states := m.CheckAll()
	alerts := m.DetectAlerts()
	report, scanErr := openwebui.Scan([]string{"g4"}, 1)
	payload := ServerOpsPayload{
		States:    states,
		Alerts:    alerts,
		OpenWebUI: &report,
		Recommended: []string{
			"Use local models/Open WebUI for repeated read-only checks.",
			"Escalate to Codex or Claude when alerts persist, policy blocks execution, or evidence needs expert review.",
		},
	}
	online := 0
	for _, state := range states {
		if state != nil && state.Online {
			online++
		}
	}
	summary := fmt.Sprintf("serverops quickcheck: online=%d/%d alerts=%d openwebui_failures=%d", online, len(states), len(alerts), report.Failures)
	if scanErr != nil {
		summary += " openwebui_error=" + scanErr.Error()
	}
	return payload, summary, scanErr
}

func runMorningBrief(ctx context.Context) (interface{}, string, error) {
	brief := assistantbrief.Morning(ctx, assistantbrief.Options{NewsLimit: 8})
	return brief, fmt.Sprintf("morning briefing generated: news=%d errors=%d", len(brief.News), len(brief.Errors)), nil
}

func runLocalAIBriefing(ctx context.Context, execute bool, targetID string) (interface{}, string, error) {
	bin := strings.TrimSpace(os.Getenv("MESHCLAW_BIN"))
	if bin == "" {
		if exe, err := os.Executable(); err == nil {
			bin = exe
		}
	}
	if bin == "" {
		bin = "meshclaw"
	}
	args := []string{"briefing", "local-ai"}
	if !execute {
		args = append(args, "--no-send")
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "MESHCLAW_BIN="+bin)
	if strings.TrimSpace(os.Getenv("OLLAMA_URL")) == "" {
		cmd.Env = append(cmd.Env, "OLLAMA_URL=http://localhost:11434")
	}
	if strings.TrimSpace(os.Getenv("BRIEF_MODEL")) == "" {
		cmd.Env = append(cmd.Env, "BRIEF_MODEL=gpt-oss:20b")
	}
	if strings.TrimSpace(os.Getenv("BRIEF_SIGNAL_PART_MAX")) == "" {
		cmd.Env = append(cmd.Env, "BRIEF_SIGNAL_PART_MAX=2000")
	}
	if strings.TrimSpace(os.Getenv("BRIEF_ATTACH_MARKDOWN")) == "" {
		cmd.Env = append(cmd.Env, "BRIEF_ATTACH_MARKDOWN=1")
	}
	if strings.TrimSpace(os.Getenv("BRIEF_ATTACH_HTML")) == "" {
		cmd.Env = append(cmd.Env, "BRIEF_ATTACH_HTML=0")
	}
	if strings.TrimSpace(os.Getenv("BRIEF_PRINT_REPORT")) == "" {
		cmd.Env = append(cmd.Env, "BRIEF_PRINT_REPORT=0")
	}
	if strings.TrimSpace(os.Getenv("BRIEF_SIGNAL_SHOW_PATHS")) == "" {
		cmd.Env = append(cmd.Env, "BRIEF_SIGNAL_SHOW_PATHS=0")
	}
	if strings.TrimSpace(targetID) != "" {
		cmd.Env = append(cmd.Env, "BRIEF_TARGET="+strings.TrimSpace(targetID))
	}
	if !execute {
		cmd.Env = append(cmd.Env, "BRIEF_SEND=0")
	}
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	payload := LocalAIBriefingPayload{
		Command:  append([]string{bin}, args...),
		Execute:  execute,
		TargetID: strings.TrimSpace(targetID),
		Stdout:   output,
	}
	if err != nil {
		payload.Stderr = output
		return payload, "local-ai briefing failed", err
	}
	mode := "dry-run"
	if execute {
		mode = "sent"
	}
	return payload, "local-ai briefing " + mode, nil
}

func runMailWatch() (interface{}, string, error) {
	store, err := mailadapter.ListAccounts()
	if err != nil {
		return MailWatchPayload{}, "", err
	}
	payload := MailWatchPayload{Results: []mailadapter.WatchResult{}}
	state, _ := LoadMailWatchState()
	if state.Seen == nil {
		state.Seen = map[string][]string{}
	}
	if state.Errors == nil {
		state.Errors = map[string]string{}
	}
	totalRaw := 0
	for _, account := range store.Accounts {
		result, watchErr := mailadapter.WatchOnce(mailadapter.WatchOptions{Account: account.ID, Since: 15 * time.Minute, Limit: 10})
		if watchErr != nil {
			errText := account.ID + ": " + watchErr.Error()
			if state.Errors[account.ID] == errText {
				payload.SeenErrors = append(payload.SeenErrors, errText)
			} else {
				payload.Errors = append(payload.Errors, errText)
				state.Errors[account.ID] = errText
			}
			continue
		}
		delete(state.Errors, account.ID)
		totalRaw += len(result.Messages)
		filtered, skipped := filterNewMailMessages(state, account.ID, result.Messages)
		payload.NewMessages += len(filtered)
		payload.SeenSkipped += skipped
		result.Messages = filtered
		payload.Results = append(payload.Results, result)
	}
	if saveErr := SaveMailWatchState(state); saveErr != nil {
		payload.Errors = append(payload.Errors, "mail-watch-state: "+saveErr.Error())
	}
	return payload, fmt.Sprintf("mail watch: accounts=%d raw=%d new=%d seen=%d errors=%d seen_errors=%d", len(store.Accounts), totalRaw, payload.NewMessages, payload.SeenSkipped, len(payload.Errors), len(payload.SeenErrors)), nil
}

func filterNewMailMessages(state MailWatchState, accountID string, messages []mailadapter.MessageSummary) ([]mailadapter.MessageSummary, int) {
	seenList := state.Seen[accountID]
	seen := map[string]bool{}
	for _, id := range seenList {
		seen[id] = true
	}
	filtered := make([]mailadapter.MessageSummary, 0, len(messages))
	skipped := 0
	for _, message := range messages {
		id := strings.TrimSpace(message.ID)
		if id == "" {
			filtered = append(filtered, message)
			continue
		}
		if seen[id] {
			skipped++
			continue
		}
		seen[id] = true
		seenList = append(seenList, id)
		filtered = append(filtered, message)
	}
	if len(seenList) > 200 {
		seenList = seenList[len(seenList)-200:]
	}
	state.Seen[accountID] = seenList
	return filtered, skipped
}

func runAssistantWatch(ctx context.Context, now time.Time) (interface{}, string, error) {
	result, err := assistantwatch.CheckDue(ctx, now)
	payload := AssistantWatchPayload{Result: result}
	return payload, fmt.Sprintf("assistant watch: total=%d due=%d matched=%d links=%d", result.Total, len(result.Due), len(result.Matched), len(result.Links)), err
}

func runArgosHealth(ctx context.Context) (interface{}, string, error) {
	doctor := osauto.ArgosMacDoctor(ctx, false)
	dashboard := refreshArgosDashboard(ctx)
	payload := ArgosHealthPayload{Doctor: doctor, Dashboard: dashboard}
	summary := fmt.Sprintf("argos health: ok=%t problems=%d runner_ok=%t calendar=%t reminders=%t contacts=%t", doctor.OK, len(doctor.Problems), doctor.UIRunner.OK, doctor.Calendar.OK, doctor.ReminderShortcut.OK, doctor.Contacts.OK)
	if dashboard.Attempted {
		summary += fmt.Sprintf(" dashboard=%t", dashboard.OK)
	}
	if !doctor.OK {
		return payload, summary, fmt.Errorf(summary)
	}
	return payload, summary, nil
}

func refreshArgosDashboard(ctx context.Context) DashboardRefresh {
	script := strings.TrimSpace(os.Getenv("MESHCLAW_ARGOS_DASHBOARD_SCRIPT"))
	if script == "" {
		for _, candidate := range []string{
			filepath.Join(os.Getenv("HOME"), "bin", "argos-dashboard.sh"),
			"/Users/argos/bin/argos-dashboard.sh",
			"/Users/example/bin/argos-dashboard.sh",
		} {
			if fileExecutable(candidate) {
				script = candidate
				break
			}
		}
	}
	if script == "" {
		return DashboardRefresh{}
	}
	result := DashboardRefresh{Attempted: true, Command: []string{script}}
	cmd := exec.CommandContext(ctx, script)
	cmd.Env = os.Environ()
	if bin := dashboardMeshClawBin(); bin != "" {
		cmd.Env = append(cmd.Env, "MESHCLAW_BIN="+bin)
	}
	out, err := cmd.CombinedOutput()
	result.Path = strings.TrimSpace(string(out))
	if err != nil {
		result.Error = strings.TrimSpace(err.Error() + ": " + string(out))
		return result
	}
	result.OK = result.Path != ""
	return result
}

func dashboardMeshClawBin() string {
	if bin := strings.TrimSpace(os.Getenv("MESHCLAW_BIN")); bin != "" {
		return bin
	}
	for _, candidate := range []string{
		"/Users/argos/bin/meshclaw",
		"/Users/example/bin/meshclaw",
	} {
		if fileExecutable(candidate) {
			return candidate
		}
	}
	return "meshclaw"
}

func fileExecutable(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0111 != 0
}

func runAssistantAutoCheck(now time.Time) (interface{}, string, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	profile, profileErr := assistantprofile.Inspect(now, "")
	snapshot, snapshotErr := assistantprofile.BuildMemorySnapshot(now, "", "비서 자동 점검")
	selfText, _, selfErr := messenger.AssistantSelfTestText("argos-assistant", "assistant")
	payload := AssistantAutoCheckPayload{
		ProfileStatus: profile.Status,
		SkillCount:    profile.Skills.Count,
		MemoryStatus:  snapshot.Status,
		MemoryLayers:  append([]string{}, snapshot.Use...),
		SelfTestLine:  extractSelfTestSummaryLine(selfText),
	}
	errs := []string{}
	for _, err := range []error{profileErr, snapshotErr, selfErr} {
		if err != nil {
			errs = append(errs, err.Error())
		}
	}
	summary := fmt.Sprintf("assistant auto-check: profile=%s memory=%s skills=%d layers=%d", firstNonEmpty(payload.ProfileStatus, "unknown"), firstNonEmpty(payload.MemoryStatus, "unknown"), payload.SkillCount, len(payload.MemoryLayers))
	if len(errs) > 0 {
		return payload, summary + " errors=" + strings.Join(errs, "; "), fmt.Errorf(strings.Join(errs, "; "))
	}
	return payload, summary, nil
}

func extractSelfTestSummaryLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "요약:") {
			return line
		}
	}
	return ""
}

func runLocalHygiene(now time.Time) (interface{}, string, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	payload := LocalHygienePayload{
		Retention: map[string]string{
			"public_argos":      "delete files older than 48h except index.html; keep newest 100 files",
			"doctor_recordings": "delete files older than 7d under ~/.meshclaw/doctor; keep newest 20 files",
			"evidence":          "count and warn only; never auto-delete",
			"logs":              "count and warn only; never auto-delete",
		},
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		payload.Warnings = append(payload.Warnings, "home directory not available")
		return payload, "local hygiene: home unavailable", err
	}
	payload.Home = home
	removeOldFiles(&payload, filepath.Join(home, ".meshclaw", "public", "argos"), now, 48*time.Hour, func(path string, info fs.FileInfo) bool {
		return info.Name() != "index.html"
	})
	removeOldFiles(&payload, filepath.Join(home, ".meshclaw", "doctor"), now, 7*24*time.Hour, func(path string, info fs.FileInfo) bool {
		return true
	})
	trimOldestFiles(&payload, filepath.Join(home, ".meshclaw", "public", "argos"), 100, func(path string, info fs.FileInfo) bool {
		return info.Name() == "index.html"
	})
	trimOldestFiles(&payload, filepath.Join(home, ".meshclaw", "doctor"), 20, nil)
	payload.PublicFiles, _ = countFiles(filepath.Join(home, ".meshclaw", "public", "argos"))
	payload.EvidenceFiles, _ = countFiles(filepath.Join(home, ".meshclaw", "evidence"))
	payload.LogFiles, payload.LogBytes = countFilesAndBytes(filepath.Join(home, ".meshclaw", "logs"))
	if payload.EvidenceFiles > 5000 {
		payload.Warnings = append(payload.Warnings, fmt.Sprintf("evidence files are growing: %d files; keep for audit, but consider archival policy", payload.EvidenceFiles))
	}
	if payload.LogBytes > 1024*1024*1024 {
		payload.Warnings = append(payload.Warnings, fmt.Sprintf("log directory is large: %s; review log rotation", formatBytes(payload.LogBytes)))
	}
	summary := fmt.Sprintf("local hygiene: removed=%d bytes=%s public=%d evidence=%d logs=%s warnings=%d", len(payload.RemovedFiles), formatBytes(payload.RemovedBytes), payload.PublicFiles, payload.EvidenceFiles, formatBytes(payload.LogBytes), len(payload.Warnings))
	return payload, summary, nil
}

func runDataDoctor(now time.Time) (interface{}, string, error) {
	report, err := datadoctor.Check(now)
	if err != nil {
		return report, "data doctor: unavailable", err
	}
	summary := fmt.Sprintf("data doctor: ok=%t warnings=%d public=%d evidence=%d logs=%s state=%d invalid=%d", report.OK, len(report.Warnings), locationFileCount(report.Locations, "public_argos"), report.Evidence.Files, formatBytes(locationBytes(report.Locations, "logs")), len(report.StateFiles), invalidStateFileCount(report.StateFiles))
	if !report.OK {
		return report, summary, fmt.Errorf(summary)
	}
	return report, summary, nil
}

func invalidStateFileCount(files []datadoctor.StateFile) int {
	count := 0
	for _, file := range files {
		if file.Exists && !file.OK {
			count++
		}
	}
	return count
}

func locationFileCount(locations []datadoctor.Location, id string) int {
	for _, loc := range locations {
		if loc.ID == id {
			return loc.Files
		}
	}
	return 0
}

func locationBytes(locations []datadoctor.Location, id string) int64 {
	for _, loc := range locations {
		if loc.ID == id {
			return loc.Bytes
		}
	}
	return 0
}

func removeOldFiles(payload *LocalHygienePayload, dir string, now time.Time, maxAge time.Duration, keep func(string, fs.FileInfo) bool) {
	info, err := os.Stat(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			payload.Skipped = append(payload.Skipped, dir+": "+err.Error())
		}
		return
	}
	if !info.IsDir() {
		payload.Skipped = append(payload.Skipped, dir+": not a directory")
		return
	}
	_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			payload.Skipped = append(payload.Skipped, path+": "+err.Error())
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			payload.Skipped = append(payload.Skipped, path+": "+err.Error())
			return nil
		}
		if keep != nil && !keep(path, info) {
			return nil
		}
		age := now.Sub(info.ModTime())
		if age < maxAge {
			return nil
		}
		removeHygieneFile(payload, path, info, age)
		return nil
	})
}

func trimOldestFiles(payload *LocalHygienePayload, dir string, maxFiles int, keep func(string, fs.FileInfo) bool) {
	if maxFiles <= 0 {
		return
	}
	info, err := os.Stat(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			payload.Skipped = append(payload.Skipped, dir+": "+err.Error())
		}
		return
	}
	if !info.IsDir() {
		payload.Skipped = append(payload.Skipped, dir+": not a directory")
		return
	}
	type candidate struct {
		path string
		info fs.FileInfo
	}
	var files []candidate
	_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			payload.Skipped = append(payload.Skipped, path+": "+err.Error())
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			payload.Skipped = append(payload.Skipped, path+": "+err.Error())
			return nil
		}
		if keep != nil && keep(path, info) {
			return nil
		}
		files = append(files, candidate{path: path, info: info})
		return nil
	})
	if len(files) <= maxFiles {
		return
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].info.ModTime().Before(files[j].info.ModTime())
	})
	for _, file := range files[:len(files)-maxFiles] {
		removeHygieneFile(payload, file.path, file.info, time.Since(file.info.ModTime()))
	}
}

func removeHygieneFile(payload *LocalHygienePayload, path string, info fs.FileInfo, age time.Duration) {
	size := info.Size()
	if err := os.Remove(path); err != nil {
		payload.Skipped = append(payload.Skipped, path+": "+err.Error())
		return
	}
	payload.RemovedFiles = append(payload.RemovedFiles, LocalHygieneRemoved{Path: path, Bytes: size, Age: age.Round(time.Second).String()})
	payload.RemovedBytes += size
}

func countFiles(dir string) (int, error) {
	count, _ := countFilesAndBytes(dir)
	return count, nil
}

func countFilesAndBytes(dir string) (int, int64) {
	count := 0
	var bytes int64
	_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		count++
		bytes += info.Size()
		return nil
	})
	return count, bytes
}

func formatBytes(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%dB", bytes)
	}
	if bytes < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	}
	if bytes < 1024*1024*1024 {
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
	}
	return fmt.Sprintf("%.1fGB", float64(bytes)/(1024*1024*1024))
}

func runExpertAuditHandoff() (interface{}, string, error) {
	payload := ExpertAuditPayload{
		Scope:         "daily fleet, Open WebUI, policy, evidence, and approval review",
		RecommendedAI: []string{"Codex", "Claude"},
		Inputs: []string{
			"meshclaw monitor-check --json",
			"meshclaw openwebui-scan --hosts g4 --json",
			"meshclaw ops-control --json",
			"meshclaw evidence-list 20 --json",
		},
		Prompt: strings.Join([]string{
			"Run the daily MeshClaw expert audit through MCP.",
			"Review fleet health, Open WebUI router health, recent evidence, policy gates, and approval blockers.",
			"Do not execute destructive actions. Create a concise report and store evidence.",
			"Escalate only items that local models cannot safely interpret.",
		}, " "),
		Next: []string{
			"Open Codex or Claude with MeshClaw MCP enabled.",
			"Run the listed read-only commands/tools.",
			"Write a redacted report to evidence and optionally send it to the ops target.",
		},
	}
	if m, err := monitor.New(monitor.DefaultConfig()); err == nil {
		states := m.CheckAll()
		alerts := m.DetectAlerts()
		payload.ServerTotal = len(states)
		payload.Alerts = len(alerts)
		for _, state := range states {
			if state != nil && state.Online {
				payload.ServerOnline++
			}
		}
	} else {
		payload.Problems = append(payload.Problems, "server monitor: "+err.Error())
	}
	if report, err := openwebui.Scan([]string{"g4"}, 1); err == nil {
		payload.OpenWebUIFailures = report.Failures
	} else {
		payload.Problems = append(payload.Problems, "Open WebUI scan: "+err.Error())
	}
	if recent, err := evidence.List(8); err == nil {
		payload.RecentEvidence = len(recent)
	} else {
		payload.Problems = append(payload.Problems, "recent evidence: "+err.Error())
	}
	return payload, fmt.Sprintf("daily expert audit: servers=%d/%d alerts=%d openwebui_failures=%d recent=%d problems=%d", payload.ServerOnline, payload.ServerTotal, payload.Alerts, payload.OpenWebUIFailures, payload.RecentEvidence, len(payload.Problems)), nil
}

func findJob(id string) (Job, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		id = "serverops-quickcheck"
	}
	for _, job := range DefaultPlan().Jobs {
		if job.ID == id || job.Kind == id {
			return job, nil
		}
	}
	return Job{}, fmt.Errorf("unknown scheduled job %q", id)
}

func formatServerOpsMessage(payload ServerOpsPayload, result RunResult) string {
	var b strings.Builder
	loc := time.Now()
	online := 0
	total := len(payload.States)
	for _, st := range payload.States {
		if st != nil && st.Online {
			online++
		}
	}

	fmt.Fprintf(&b, "🖥️ 서버 상태 보고 — %s\n", loc.Format("01/02 15:04"))
	fmt.Fprintf(&b, "전체적으로 %d대 중 %d대가 정상 가동 중입니다.\n", total, online)

	var crit, warn []monitor.Alert
	for _, a := range payload.Alerts {
		switch a.Level {
		case "critical":
			crit = append(crit, a)
		case "warning":
			warn = append(warn, a)
		}
	}
	sort.Slice(crit, func(i, j int) bool { return crit[i].Node < crit[j].Node })
	sort.Slice(warn, func(i, j int) bool { return warn[i].Node < warn[j].Node })
	offline := offlineNodeNames(payload.States)
	diskAttention := diskAttentionNodes(payload.States)
	if len(crit) == 0 && len(warn) == 0 {
		b.WriteString("지금은 긴급 조치가 필요한 서버 경고가 없습니다.")
	} else {
		b.WriteString("현재 확인이 필요한 부분은 ")
		parts := []string{}
		if len(offline) > 0 {
			parts = append(parts, fmt.Sprintf("%s 오프라인", strings.Join(offline, ", ")))
		}
		if len(diskAttention) > 0 {
			parts = append(parts, fmt.Sprintf("%s 디스크 사용량", strings.Join(diskAttention, ", ")))
		}
		if len(parts) == 0 {
			parts = append(parts, fmt.Sprintf("경고 %d건", len(crit)+len(warn)))
		}
		fmt.Fprintf(&b, "%s입니다.", strings.Join(parts, ", "))
	}
	if payload.OpenWebUI != nil {
		if payload.OpenWebUI.Failures == 0 {
			b.WriteString(" 로컬 AI 연결 상태도 정상입니다.")
		} else {
			fmt.Fprintf(&b, " 로컬 AI 연결 경로에 실패 %d건이 있어 확인이 필요합니다.", payload.OpenWebUI.Failures)
		}
	}
	b.WriteString("\n")

	if previous, ok := previousServerOpsPayload(result.Evidence.StoredAt); ok {
		fmt.Fprintf(&b, "\n%s\n", serverOpsComparisonLine(payload, previous, online, total))
	} else {
		b.WriteString("\n이전 보고 기준은 아직 찾지 못했습니다. 다음 실행부터는 온라인 수, 오프라인 노드, 디스크 경고의 변화를 함께 비교합니다.\n")
	}

	if len(crit) > 0 {
		b.WriteString("\n🔴 긴급 확인\n")
		for _, a := range crit {
			fmt.Fprintf(&b, "• %s %s\n", koreanNodeTopic(a.Node), koreanMonitorAlertMessage(a))
		}
	}
	if len(warn) > 0 {
		b.WriteString("\n🟡 주의할 점\n")
		for _, a := range warn {
			fmt.Fprintf(&b, "• %s %s\n", koreanNodeTopic(a.Node), koreanMonitorAlertMessage(a))
		}
	}
	if len(crit) == 0 && len(warn) == 0 {
		b.WriteString("\n🟢 모든 노드가 정상이며 별도 경고는 없습니다.\n")
	}
	b.WriteString("\n다음에 할 일\n")
	if len(offline) > 0 {
		fmt.Fprintf(&b, "• %s는 계속 오프라인이면 전원, 네트워크, Tailscale, SSH 접속 상태를 확인하세요.\n", strings.Join(offline, ", "))
	}
	if len(diskAttention) > 0 {
		fmt.Fprintf(&b, "• %s는 디스크 사용량이 높습니다. 바로 삭제하지 말고 먼저 큰 파일과 보관 가능한 데이터를 확인한 뒤 정리 계획을 세우세요.\n", strings.Join(diskAttention, ", "))
	}
	if payload.OpenWebUI != nil && payload.OpenWebUI.Failures > 0 {
		b.WriteString("• 로컬 AI 연결이 실패했습니다. Open WebUI 라우터와 g4의 서비스 상태를 먼저 확인하세요.\n")
	}
	if len(offline) == 0 && len(diskAttention) == 0 && (payload.OpenWebUI == nil || payload.OpenWebUI.Failures == 0) {
		b.WriteString("• 지금 바로 할 일은 없습니다. 다음 정기 보고에서 변화가 있는지만 보면 됩니다.\n")
	}
	b.WriteString("• 자동 복구, 삭제, 재시작은 이 보고만으로 실행하지 않았습니다. 조치가 필요하면 보고방이 아니라 비서방에서 요청하고 승인하세요.\n")

	names := make([]string, 0, len(payload.States))
	for name := range payload.States {
		names = append(names, name)
	}
	sort.Strings(names)
	b.WriteString("\n📊 노드별 상세\n")
	for _, name := range names {
		st := payload.States[name]
		if st == nil {
			continue
		}
		if !st.Online {
			fmt.Fprintf(&b, "• %s: ⚫ 오프라인\n", name)
			continue
		}
		line := fmt.Sprintf("• %s CPU %.0f%%, 메모리 %.0f%%, 디스크 %.0f%%를 사용 중입니다", koreanNodeTopic(name), st.CPU, st.Memory, st.Disk)
		if st.GPUMemory > 0 || st.GPUUsage > 0 {
			line = strings.TrimSuffix(line, "입니다") + fmt.Sprintf("이고, GPU 사용률은 %.0f%%입니다", st.GPUUsage)
		}
		fmt.Fprint(&b, line+".\n")
	}

	if payload.OpenWebUI != nil {
		if payload.OpenWebUI.Failures == 0 {
			b.WriteString("\n🟢 로컬 AI 연결 상태는 정상입니다. Open WebUI 라우터 실패는 0건입니다.\n")
		} else {
			fmt.Fprintf(&b, "\n🔴 로컬 AI 연결 경로에서 실패가 %d건 있습니다. Open WebUI 라우터를 확인해야 합니다.\n", payload.OpenWebUI.Failures)
		}
	}

	return strings.TrimSpace(b.String())
}

func previousServerOpsPayload(currentEvidencePath string) (ServerOpsPayload, bool) {
	summaries, err := evidence.List(200)
	if err != nil {
		return ServerOpsPayload{}, false
	}
	currentEvidencePath = strings.TrimSpace(currentEvidencePath)
	for _, summary := range summaries {
		if summary.Kind != "schedule-serverops_quickcheck" {
			continue
		}
		if currentEvidencePath != "" && summary.StoredAt == currentEvidencePath {
			continue
		}
		payload, ok := loadServerOpsPayloadFromEvidence(summary.StoredAt)
		if ok {
			return payload, true
		}
	}
	return ServerOpsPayload{}, false
}

func loadServerOpsPayloadFromEvidence(path string) (ServerOpsPayload, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ServerOpsPayload{}, false
	}
	var record struct {
		Payload struct {
			Payload ServerOpsPayload `json:"payload"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return ServerOpsPayload{}, false
	}
	if len(record.Payload.Payload.States) == 0 && len(record.Payload.Payload.Alerts) == 0 {
		return ServerOpsPayload{}, false
	}
	return record.Payload.Payload, true
}

func serverOpsComparisonLine(current, previous ServerOpsPayload, currentOnline, currentTotal int) string {
	prevOnline := 0
	for _, st := range previous.States {
		if st != nil && st.Online {
			prevOnline++
		}
	}
	parts := []string{}
	delta := currentOnline - prevOnline
	switch {
	case delta > 0:
		parts = append(parts, fmt.Sprintf("정상 가동 노드는 이전보다 %d대 늘었습니다", delta))
	case delta < 0:
		parts = append(parts, fmt.Sprintf("정상 가동 노드는 이전보다 %d대 줄었습니다", -delta))
	default:
		parts = append(parts, fmt.Sprintf("정상 가동 노드는 이전과 같은 %d/%d대입니다", currentOnline, currentTotal))
	}

	currOffline := offlineNodeNames(current.States)
	prevOffline := offlineNodeNames(previous.States)
	newOffline := differenceStrings(currOffline, prevOffline)
	recovered := differenceStrings(prevOffline, currOffline)
	if len(newOffline) > 0 {
		parts = append(parts, fmt.Sprintf("새로 오프라인으로 보이는 노드는 %s입니다", strings.Join(newOffline, ", ")))
	}
	if len(recovered) > 0 {
		parts = append(parts, fmt.Sprintf("복구된 노드는 %s입니다", strings.Join(recovered, ", ")))
	}
	if len(newOffline) == 0 && len(recovered) == 0 && len(currOffline) > 0 {
		parts = append(parts, fmt.Sprintf("오프라인 노드는 이전과 동일하게 %s입니다", strings.Join(currOffline, ", ")))
	}

	if diskLine := diskComparisonLine(current.States, previous.States); diskLine != "" {
		parts = append(parts, diskLine)
	}
	return "이전 보고와 비교하면 " + strings.Join(parts, ". ") + "."
}

func offlineNodeNames(states map[string]*monitor.NodeState) []string {
	names := []string{}
	for name, st := range states {
		if st != nil && !st.Online {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func diskAttentionNodes(states map[string]*monitor.NodeState) []string {
	names := []string{}
	for name, st := range states {
		if st != nil && st.Online && st.Disk >= 80 {
			names = append(names, fmt.Sprintf("%s %.0f%%", name, st.Disk))
		}
	}
	sort.Strings(names)
	return names
}

func diskComparisonLine(current, previous map[string]*monitor.NodeState) string {
	notes := []string{}
	names := make([]string, 0, len(current))
	for name, st := range current {
		if st != nil && st.Online && st.Disk >= 80 {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		st := current[name]
		prev := previous[name]
		if prev == nil || !prev.Online {
			notes = append(notes, fmt.Sprintf("%s 디스크는 현재 %.1f%%입니다", name, st.Disk))
			continue
		}
		diff := st.Disk - prev.Disk
		switch {
		case diff >= 1:
			notes = append(notes, fmt.Sprintf("%s 디스크는 %.1f%%로 이전보다 %.1f%%p 늘었습니다", name, st.Disk, diff))
		case diff <= -1:
			notes = append(notes, fmt.Sprintf("%s 디스크는 %.1f%%로 이전보다 %.1f%%p 줄었습니다", name, st.Disk, -diff))
		default:
			notes = append(notes, fmt.Sprintf("%s 디스크는 %.1f%%로 이전과 거의 같습니다", name, st.Disk))
		}
	}
	return strings.Join(notes, ". ")
}

func differenceStrings(a, b []string) []string {
	seen := map[string]bool{}
	for _, value := range b {
		seen[value] = true
	}
	var out []string
	for _, value := range a {
		if !seen[value] {
			out = append(out, value)
		}
	}
	return out
}

func koreanMonitorAlertMessage(a monitor.Alert) string {
	msg := strings.TrimSpace(a.Message)
	lower := strings.ToLower(msg)
	percent := alertMessageSuffix(msg)
	switch strings.TrimSpace(a.Type) {
	case "offline":
		return "노드가 오프라인입니다."
	case "disk":
		if strings.Contains(lower, "critical") {
			return "디스크 사용량이 위험 수준입니다" + percent + "."
		}
		return "디스크 사용량이 높습니다" + percent + "."
	case "memory":
		return "메모리 사용량이 높습니다" + percent + "."
	}
	if strings.HasPrefix(lower, "node ") && strings.Contains(lower, " is offline") {
		return "노드가 오프라인입니다."
	}
	if strings.HasPrefix(lower, "disk usage critical") {
		return "디스크 사용량이 위험 수준입니다" + percent + "."
	}
	if strings.HasPrefix(lower, "disk usage high") {
		return "디스크 사용량이 높습니다" + percent + "."
	}
	if strings.HasPrefix(lower, "memory usage high") {
		return "메모리 사용량이 높습니다" + percent + "."
	}
	return msg
}

func compactLine(value string, limit int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if limit <= 0 || len([]rune(value)) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit]) + "..."
}

func koreanMailWatchError(errText string) string {
	errText = strings.TrimSpace(errText)
	if errText == "" {
		return "메일 계정 확인 중 오류가 있었습니다."
	}
	account := errText
	if idx := strings.Index(errText, ":"); idx >= 0 {
		account = strings.TrimSpace(errText[:idx])
	}
	lower := strings.ToLower(errText)
	switch {
	case strings.Contains(lower, "secret not found"), strings.Contains(lower, "keychain"), strings.Contains(lower, "access denied"):
		return fmt.Sprintf("%s 계정의 저장된 암호 또는 Keychain 접근 권한을 확인해야 합니다.", account)
	case strings.Contains(lower, "login"), strings.Contains(lower, "auth"):
		return fmt.Sprintf("%s 계정의 로그인 상태를 확인해야 합니다.", account)
	default:
		return fmt.Sprintf("%s 계정 확인 중 오류가 있었습니다.", account)
	}
}

func alertMessageSuffix(message string) string {
	idx := strings.LastIndex(message, ":")
	if idx < 0 {
		return ""
	}
	value := strings.TrimSpace(message[idx+1:])
	if value == "" {
		return ""
	}
	return ": " + value
}

func koreanNodeTopic(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "해당 노드는"
	}
	last := []rune(name)[len([]rune(name))-1]
	if last >= '0' && last <= '9' {
		switch last {
		case '0', '1', '3', '6', '7', '8':
			return name + "은"
		default:
			return name + "는"
		}
	}
	return name + "는"
}

func scheduleDashboardURL() string {
	if url := strings.TrimSpace(os.Getenv("MESHCLAW_ARGOS_DASHBOARD_URL")); url != "" {
		return url
	}
	return "https://argos.zeus.kim/argos/dashboard.html"
}

type localAIBriefingOutcome struct {
	Markdown     string `json:"markdown"`
	ObsidianNote string `json:"obsidian_note"`
	SignalParts  int    `json:"signal_parts"`
	Counts       struct {
		News          int            `json:"news"`
		NewsByRegion  map[string]int `json:"news_by_region"`
		Mail          int            `json:"mail"`
		ServersOnline int            `json:"servers_online"`
		ServersTotal  int            `json:"servers_total"`
	} `json:"counts"`
}

func localAIBriefingOutcomeLines(stdout string) ([]string, bool) {
	outcome, ok := parseLocalAIBriefingOutcome(stdout)
	if !ok {
		return nil, false
	}
	lines := []string{}
	if outcome.Counts.News > 0 || outcome.Counts.Mail > 0 || outcome.Counts.ServersTotal > 0 {
		serverText := "서버 상태를 확인했습니다"
		if outcome.Counts.ServersTotal > 0 {
			serverText = fmt.Sprintf("서버 %d/%d대 온라인", outcome.Counts.ServersOnline, outcome.Counts.ServersTotal)
		}
		lines = append(lines, fmt.Sprintf("뉴스 %d건, 메일 %d건, %s을 확인했습니다.", outcome.Counts.News, outcome.Counts.Mail, serverText))
	}
	if len(outcome.Counts.NewsByRegion) > 0 {
		parts := []string{}
		for _, region := range []string{"한국", "미국", "글로벌", "아시아"} {
			if count := outcome.Counts.NewsByRegion[region]; count > 0 {
				parts = append(parts, fmt.Sprintf("%s %d건", region, count))
			}
		}
		if len(parts) > 0 {
			lines = append(lines, "뉴스 구성: "+strings.Join(parts, ", "))
		}
	}
	if outcome.SignalParts > 0 {
		lines = append(lines, fmt.Sprintf("Signal 보고방 본문 %d개로 전송했습니다.", outcome.SignalParts))
	}
	if strings.TrimSpace(outcome.Markdown) != "" {
		lines = append(lines, "Markdown 보고서를 첨부했습니다.")
	}
	if strings.TrimSpace(outcome.ObsidianNote) != "" {
		lines = append(lines, "Obsidian 문서도 저장했습니다.")
	}
	return lines, len(lines) > 0
}

func parseLocalAIBriefingOutcome(stdout string) (localAIBriefingOutcome, bool) {
	lines := strings.Split(stdout, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var outcome localAIBriefingOutcome
		if err := json.Unmarshal([]byte(line), &outcome); err == nil {
			return outcome, true
		}
	}
	return localAIBriefingOutcome{}, false
}

func appendScheduleDetailLink(b *strings.Builder) {
	if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
		b.WriteString("\n")
	}
	fmt.Fprintf(b, "최근 보고와 상태는 대시보드에서 볼 수 있습니다: %s", scheduleDashboardURL())
}

func formatScheduleMessage(result RunResult) string {
	if result.Job.Kind == "serverops_quickcheck" {
		if payload, ok := result.Payload.(ServerOpsPayload); ok {
			return formatServerOpsMessage(payload, result)
		}
	}
	if result.Job.Kind == "assistant_morning" {
		if payload, ok := result.Payload.(assistantbrief.Brief); ok {
			var msg strings.Builder
			msg.WriteString("아침 브리핑을 준비했습니다.\n")
			if strings.TrimSpace(payload.Text) != "" {
				msg.WriteString(strings.TrimSpace(payload.Text))
				msg.WriteString("\n")
			} else {
				fmt.Fprintf(&msg, "뉴스: %d개 / 오류: %d개\n", len(payload.News), len(payload.Errors))
			}
			if len(payload.Errors) > 0 {
				fmt.Fprintf(&msg, "확인 필요: %d개\n", len(payload.Errors))
			}
			if result.Evidence.StoredAt != "" {
				appendScheduleDetailLink(&msg)
			}
			return strings.TrimSpace(msg.String())
		}
	}
	if result.Job.Kind == "local_ai_briefing" {
		if payload, ok := result.Payload.(LocalAIBriefingPayload); ok {
			var msg strings.Builder
			if payload.Execute {
				msg.WriteString("로컬 AI 브리핑을 만들고 보고방 전송까지 마쳤습니다.\n")
			} else {
				msg.WriteString("로컬 AI 브리핑을 시험 생성했습니다. 이번 실행에서는 Signal 전송은 하지 않았습니다.\n")
			}
			if result.Success {
				msg.WriteString("뉴스, 서버 상태, 메일 정보를 모아 요약하는 과정은 정상적으로 끝났습니다.\n")
			} else {
				msg.WriteString("브리핑 생성 과정에서 확인이 필요한 문제가 있었습니다.\n")
			}
			msg.WriteString("다음에 할 일: 중요한 항목이 있으면 보고방에서 답하지 말고 비서방에서 이어서 질문하세요. 결제, 삭제, 재시작 같은 작업은 이 브리핑만으로 실행하지 않았고, 필요하면 비서방에서 별도로 요청하고 승인해야 합니다.\n")
			if payload.TargetID != "" {
				fmt.Fprintf(&msg, "대상 보고방은 %s입니다.\n", payload.TargetID)
			}
			if lines, ok := localAIBriefingOutcomeLines(payload.Stdout); ok {
				msg.WriteString("실행 결과:\n")
				for _, line := range lines {
					fmt.Fprintf(&msg, "• %s\n", line)
				}
			} else if strings.TrimSpace(payload.Stdout) != "" {
				msg.WriteString("실행 결과: 브리핑 산출물은 생성됐지만 세부 집계는 읽지 못했습니다.\n")
			}
			if result.Evidence.StoredAt != "" {
				appendScheduleDetailLink(&msg)
			}
			return strings.TrimSpace(msg.String())
		}
	}
	if result.Job.Kind == "expert_audit_handoff" {
		var msg strings.Builder
		msg.WriteString("오늘 심층 점검 결과입니다.\n")
		if payload, ok := result.Payload.(ExpertAuditPayload); ok {
			fmt.Fprintf(&msg, "서버 상태: %d/%d대가 온라인이고, 경고는 %d개입니다.\n", payload.ServerOnline, payload.ServerTotal, payload.Alerts)
			fmt.Fprintf(&msg, "Open WebUI 경로: 실패 %d개로 확인했습니다.\n", payload.OpenWebUIFailures)
			fmt.Fprintf(&msg, "최근 작업 기록: 최근 %d개 기록을 읽어 점검 범위에 포함했습니다.\n", payload.RecentEvidence)
			if len(payload.Problems) > 0 {
				for _, problem := range payload.Problems {
					fmt.Fprintf(&msg, "확인 필요: %s\n", koreanScheduleSummary(problem))
				}
			}
		} else {
			msg.WriteString("서버, 로컬 AI 연결 경로, 최근 작업 기록, 승인 대기 항목을 읽기 전용 범위로 확인했습니다.\n")
		}
		msg.WriteString("자동 실행 결과: 삭제, 결제, 메일/Signal 발송, 서버 재시작, 배포는 0건입니다.\n")
		msg.WriteString("다음에 할 일: 경고가 있으면 비서방에서 '오늘 심층 점검 자세히'라고 요청하세요. 위험한 변경은 이 단계에서 실행하지 않았고, 조치가 필요하면 별도 승인 후 진행합니다.\n")
		if result.Evidence.StoredAt != "" {
			appendScheduleDetailLink(&msg)
		}
		return strings.TrimSpace(msg.String())
	}
	var b strings.Builder
	fmt.Fprintf(&b, "MeshClaw 자동 작업 결과: %s\n", koreanScheduleJobName(result.Job))
	fmt.Fprintf(&b, "상태: %s\n", koreanScheduleStatus(result.Status, result.Success))
	if strings.TrimSpace(result.Summary) != "" {
		fmt.Fprintf(&b, "요약: %s\n", koreanScheduleSummary(result.Summary))
	}
	if result.Evidence.StoredAt != "" {
		appendScheduleDetailLink(&b)
		b.WriteString("\n")
	}
	if result.Job.Kind == "mail_watch" {
		if payload, ok := result.Payload.(MailWatchPayload); ok {
			var msg strings.Builder
			total := 0
			for _, watch := range payload.Results {
				total += len(watch.Messages)
			}
			if total == 0 && len(payload.Errors) == 0 {
				msg.WriteString("새로 알려드릴 메일은 없습니다.\n")
			} else if total == 1 {
				msg.WriteString("새 메일이 1개 들어왔습니다.\n")
			} else {
				fmt.Fprintf(&msg, "새 메일이 %d개 들어왔습니다.\n", total)
			}
			if len(payload.Errors) > 0 {
				fmt.Fprintf(&msg, "일부 메일 계정은 확인이 필요합니다. 오류 %d건이 있었습니다.\n", len(payload.Errors))
			}
			written := 0
			for _, watch := range payload.Results {
				if len(watch.Messages) == 0 {
					continue
				}
				account := firstNonEmpty(watch.Account.Email, watch.Account.ID, "메일 계정")
				if total > 1 {
					fmt.Fprintf(&msg, "\n%s 계정에서 확인된 메일입니다.\n", account)
				}
				for _, mail := range watch.Messages {
					if written >= 5 {
						fmt.Fprintf(&msg, "그 외 %d개 메일은 비서방에서 이어서 확인할 수 있습니다.\n", total-written)
						break
					}
					fmt.Fprintf(&msg, "%d. %s\n", written+1, firstNonEmpty(mail.Subject, "제목 없음"))
					fmt.Fprintf(&msg, "   보낸 사람: %s\n", firstNonEmpty(mail.From, "보낸 사람 정보 없음"))
					if total == 1 {
						fmt.Fprintf(&msg, "   계정: %s\n", account)
					}
					if snippet := messenger.CleanMailSnippetForSignal(mail.Snippet, 140); snippet != "" {
						fmt.Fprintf(&msg, "   미리보기: %s\n", snippet)
					}
					if mail.HasAttach {
						msg.WriteString("   첨부 파일이 있습니다.\n")
					}
					written++
				}
			}
			for _, errText := range payload.Errors {
				fmt.Fprintf(&msg, "확인 필요: %s\n", koreanMailWatchError(errText))
			}
			msg.WriteString("다음에 할 일: 읽어야 할 메일이면 비서방에서 '이 메일 요약해줘' 또는 '답장 초안 만들어줘'라고 요청하세요. 이 보고는 새 메일을 알려주는 것뿐이며 답장, 삭제, 이동은 하지 않았습니다.\n")
			if result.Evidence.StoredAt != "" {
				appendScheduleDetailLink(&msg)
			}
			return strings.TrimSpace(msg.String())
		}
	}
	if result.Job.Kind == "argos_health" {
		if payload, ok := result.Payload.(ArgosHealthPayload); ok {
			if payload.Doctor.OK {
				return strings.TrimSpace("Argos macOS 비서 상태를 확인했습니다. 현재는 Calendar, Contacts, Reminders 같은 Mac 작업을 처리할 준비가 되어 있습니다.\n다음에 할 일: 지금은 별도 조치가 필요 없습니다. Mac 작업이 실패하면 이 보고를 기준으로 권한이나 Runner 상태를 다시 확인하면 됩니다.")
			}
			var alert strings.Builder
			alert.WriteString("Argos macOS 비서 상태를 확인했는데, 일부 기능은 바로 쓰기 전에 손봐야 합니다.\n")
			for _, problem := range payload.Doctor.Problems {
				fmt.Fprintf(&alert, "• 문제: %s\n", problem)
			}
			for _, action := range payload.Doctor.NextActions {
				fmt.Fprintf(&alert, "• 다음 조치: %s\n", action)
			}
			alert.WriteString("다음에 할 일: 권한 승인이 필요한 항목은 사용자가 Mac에서 직접 허용해야 합니다. Runner 재시작이나 설정 열기는 보고방이 아니라 비서방에서 요청하고 승인하세요.\n")
			if result.Evidence.StoredAt != "" {
				appendScheduleDetailLink(&alert)
			}
			return strings.TrimSpace(alert.String())
		}
	}
	if result.Job.Kind == "assistant_auto_check" {
		if payload, ok := result.Payload.(AssistantAutoCheckPayload); ok {
			status := "정상"
			if !result.Success || payload.ProfileStatus != "ok" || payload.MemoryStatus != "ok" {
				status = "확인 필요"
			}
			var msg strings.Builder
			fmt.Fprintf(&msg, "Argos 비서 자동 점검입니다. 현재 상태는 %s입니다.\n", status)
			fmt.Fprintf(&msg, "비서 설정: 정체성, 말투, 안전 규칙을 읽을 수 있는지 확인했습니다. 결과는 %s입니다.\n", firstNonEmpty(payload.ProfileStatus, "unknown"))
			fmt.Fprintf(&msg, "사용 가능한 기능 묶음: 뉴스, 날씨, 일정, 메일, 지도, Mac 작업 같은 비서 기능 %d개를 확인했습니다.\n", payload.SkillCount)
			if len(payload.MemoryLayers) > 0 {
				fmt.Fprintf(&msg, "기억 상태: 이번 답변에 참고할 장기/단기 기억 묶음은 %s입니다.\n", strings.Join(payload.MemoryLayers, ", "))
			} else {
				msg.WriteString("기억 상태: 이번 점검에서 답변에 따로 참고할 장기/단기 기억은 없었습니다.\n")
			}
			if payload.SelfTestLine != "" {
				fmt.Fprintf(&msg, "기능 점검: 비서방 답변, 뉴스/날씨, Mac 권한, 기능 목록 같은 대표 기능을 시험했습니다. 결과는 %s입니다.\n", strings.TrimPrefix(payload.SelfTestLine, "요약: "))
			}
			msg.WriteString("읽기 전용이라는 뜻: 상태 확인과 테스트 답변 생성만 했고, 파일 삭제, 결제, 메일/Signal 발송, 서버 재시작이나 배포는 하지 않았습니다.\n")
			if result.Evidence.StoredAt != "" {
				appendScheduleDetailLink(&msg)
			}
			return strings.TrimSpace(msg.String())
		}
	}
	if result.Job.Kind == "local_hygiene" {
		if payload, ok := result.Payload.(LocalHygienePayload); ok {
			var msg strings.Builder
			if len(payload.RemovedFiles) > 0 {
				fmt.Fprintf(&msg, "MeshClaw 로컬 정리를 실행해서 임시 파일 %d개, 총 %s를 정리했습니다.\n", len(payload.RemovedFiles), formatBytes(payload.RemovedBytes))
			} else {
				msg.WriteString("MeshClaw 로컬 정리 상태를 확인했습니다. 이번에는 지울 임시 파일이 없었습니다.\n")
			}
			fmt.Fprintf(&msg, "현재 공개 산출물은 %d개, 자동 작업 보관 파일은 %d개이고 로그 용량은 %s입니다.\n", payload.PublicFiles, payload.EvidenceFiles, formatBytes(payload.LogBytes))
			for _, warning := range payload.Warnings {
				fmt.Fprintf(&msg, "확인할 점: %s\n", koreanScheduleWarning(warning))
			}
			msg.WriteString("다음에 할 일: 경고가 없으면 그대로 두면 됩니다. 자동 작업 보관 파일이나 로그가 계속 늘어나면 삭제하지 말고, 비서방에서 보관 계획을 먼저 요청한 뒤 승인해서 정리하세요.\n")
			if result.Evidence.StoredAt != "" {
				appendScheduleDetailLink(&msg)
			}
			return strings.TrimSpace(msg.String())
		}
	}
	if result.Job.Kind == "data_doctor" {
		if payload, ok := result.Payload.(datadoctor.Report); ok {
			var msg strings.Builder
			if payload.OK {
				msg.WriteString("MeshClaw 데이터 보관 상태를 확인했습니다. 현재 상태 파일은 정상으로 보입니다.\n")
			} else {
				fmt.Fprintf(&msg, "MeshClaw 데이터 보관 상태를 확인했습니다. 확인할 점이 %d개 있습니다.\n", len(payload.Warnings))
			}
			for _, loc := range payload.Locations {
				fmt.Fprintf(&msg, "• %s에는 %d개 파일, %s가 있습니다.\n", koreanDataDoctorLocation(loc.ID), loc.Files, formatBytes(loc.Bytes))
			}
			fmt.Fprintf(&msg, "자동 작업 보관 파일은 %d개, 총 %s입니다.\n", payload.Evidence.Files, formatBytes(payload.Evidence.Bytes))
			if len(payload.StateFiles) > 0 {
				invalid := []string{}
				for _, file := range payload.StateFiles {
					if file.Exists && !file.OK {
						invalid = append(invalid, file.ID)
					}
				}
				fmt.Fprintf(&msg, "상태 JSON은 %d개를 검사했고, 깨진 파일은 %d개입니다.\n", len(payload.StateFiles), len(invalid))
				if len(invalid) > 0 {
					fmt.Fprintf(&msg, "깨진 상태 파일은 %s입니다.\n", strings.Join(invalid, ", "))
				}
			}
			for _, warning := range payload.Warnings {
				fmt.Fprintf(&msg, "확인할 점: %s\n", koreanScheduleWarning(warning))
			}
			msg.WriteString("다음에 할 일: 상태 JSON이 깨졌으면 원인을 확인해야 합니다. 자동 작업 보관 파일이 많다는 경고는 바로 삭제하라는 뜻이 아니며, 비서방에서 보관 계획 후보를 먼저 확인하고 승인 후에만 정리하세요.\n")
			if result.Evidence.StoredAt != "" {
				appendScheduleDetailLink(&msg)
			}
			return strings.TrimSpace(msg.String())
		}
	}
	if result.Job.Kind == "assistant_watch" {
		if payload, ok := result.Payload.(AssistantWatchPayload); ok {
			if len(payload.Result.Matched) > 0 {
				var alert strings.Builder
				alert.WriteString("가격 알림 조건에 맞는 항목을 찾았습니다.\n")
				for i, observation := range payload.Result.Matched {
					fmt.Fprintf(&alert, "%d. %s\n", i+1, observation.Query)
					fmt.Fprintf(&alert, "   확인 가격: %s", formatWon(observation.FoundAmount))
					if observation.FoundText != "" {
						fmt.Fprintf(&alert, " (%s)", observation.FoundText)
					}
					if observation.ThresholdAmount > 0 {
						fmt.Fprintf(&alert, " / 기준: %s 이하", formatWon(observation.ThresholdAmount))
					}
					fmt.Fprintf(&alert, "\n   링크: %s\n", visibleScheduleWatchLink(observation.URL))
				}
				if result.Evidence.StoredAt != "" {
					appendScheduleDetailLink(&alert)
				}
				alert.WriteString("\n다음에 할 일: 링크를 열어 실제 조건이 맞는지 확인하세요. 구매, 예약, 결제는 이 알림만으로 실행하지 않았습니다. 진행하려면 보고방이 아니라 비서방에서 다시 요청하고 승인해야 합니다.")
				return strings.TrimSpace(alert.String())
			}
			var msg strings.Builder
			fmt.Fprintf(&msg, "등록해 둔 사용자 알림 %d개를 확인했습니다.\n", payload.Result.Total)
			if len(payload.Result.Due) > 0 {
				fmt.Fprintf(&msg, "이번 실행에서 실제로 확인한 항목은 %d개입니다.\n", len(payload.Result.Due))
			}
			if len(payload.Result.Links) > 0 {
				msg.WriteString("확인할 수 있는 링크는 아래와 같습니다.\n")
				for i, link := range payload.Result.Links {
					fmt.Fprintf(&msg, "%d. %s\n", i+1, visibleScheduleWatchLink(link))
				}
			} else {
				msg.WriteString("조건에 맞은 새 항목은 없어서 따로 보낼 알림은 없습니다.\n")
			}
			msg.WriteString("다음에 할 일: 지금은 할 일이 없습니다. 조건이 너무 느슨하거나 자주 울리면 비서방에서 알림 조건을 수정하라고 요청하세요.\n")
			if result.Evidence.StoredAt != "" {
				appendScheduleDetailLink(&msg)
			}
			return strings.TrimSpace(msg.String())
		}
	}
	return strings.TrimSpace(b.String())
}

func koreanScheduleJobName(job Job) string {
	switch job.Kind {
	case "assistant_morning":
		return "아침 브리핑"
	case "local_ai_briefing":
		return "로컬 AI 브리핑"
	case "mail_watch":
		return "메일 감시"
	case "assistant_watch":
		return "사용자 알림 감시"
	case "argos_health":
		return "Argos 상태 확인"
	case "assistant_auto_check":
		return "Argos 비서 자동 점검"
	case "serverops_quickcheck":
		return "서버 상태 빠른 점검"
	case "local_hygiene":
		return "로컬 정리 점검"
	case "data_doctor":
		return "데이터 상태 점검"
	case "expert_audit_handoff":
		return "전문가 점검 인계"
	default:
		if strings.TrimSpace(job.ID) != "" {
			return job.ID
		}
		return job.Kind
	}
}

func koreanScheduleStatus(status string, success bool) string {
	status = strings.TrimSpace(strings.ToLower(status))
	if status == "" {
		if success {
			return "정상"
		}
		return "확인 필요"
	}
	switch status {
	case "ok", "success", "succeeded":
		return "정상"
	case "error", "failed", "fail":
		return "실패"
	case "skipped":
		return "건너뜀"
	case "warning", "warn":
		return "주의"
	default:
		if success {
			return "정상"
		}
		return status
	}
}

func koreanScheduleSummary(summary string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}
	switch {
	case strings.HasPrefix(summary, "morning briefing generated:"):
		return strings.NewReplacer("morning briefing generated:", "아침 브리핑 생성:", "news=", "뉴스 ", "errors=", "오류 ").Replace(summary)
	case strings.HasPrefix(summary, "local-ai briefing"):
		return strings.NewReplacer("local-ai briefing dry-run", "로컬 AI 브리핑 점검 완료", "local-ai briefing sent", "로컬 AI 브리핑 전송 완료", "local-ai briefing failed", "로컬 AI 브리핑 실패").Replace(summary)
	case strings.HasPrefix(summary, "assistant watch:"):
		return strings.NewReplacer("assistant watch:", "사용자 알림 감시:", "total=", "전체 ", "due=", "확인대상 ", "matched=", "조건일치 ", "links=", "링크 ").Replace(summary)
	case strings.HasPrefix(summary, "daily expert audit handoff"):
		return "전문가 점검 인계 자료 생성 완료"
	case strings.HasPrefix(summary, "local hygiene:"):
		return strings.NewReplacer("local hygiene:", "로컬 정리:", "removed=", "정리 ", "bytes=", "용량 ", "public=", "공개파일 ", "evidence=", "보관파일 ", "logs=", "로그 ", "warnings=", "주의 ").Replace(summary)
	case strings.HasPrefix(summary, "mail watch:"):
		return strings.NewReplacer("mail watch:", "메일 감시:", "accounts=", "계정 ", "raw=", "확인 ", "new=", "새 메일 ", "seen=", "이미 확인 ", "errors=", "오류 ", "seen_errors=", "반복 오류 ").Replace(summary)
	case strings.HasPrefix(summary, "argos health:"):
		return strings.NewReplacer("argos health:", "Argos 상태:", "ok=", "정상 ", "problems=", "문제 ", "runner_ok=", "Runner ", "calendar=", "캘린더 ", "reminders=", "리마인더 ", "contacts=", "연락처 ").Replace(summary)
	default:
		return summary
	}
}

func koreanDataDoctorLocation(id string) string {
	switch strings.TrimSpace(id) {
	case "public_argos":
		return "공개 산출물"
	case "doctor":
		return "진단 산출물"
	case "logs":
		return "로그"
	case "state":
		return "상태 파일"
	default:
		if strings.TrimSpace(id) == "" {
			return "기타"
		}
		return id
	}
}

func koreanScheduleWarning(warning string) string {
	warning = strings.TrimSpace(warning)
	if warning == "" {
		return ""
	}
	if strings.HasPrefix(warning, "evidence files are growing:") {
		count := strings.TrimSpace(strings.TrimPrefix(warning, "evidence files are growing:"))
		if idx := strings.Index(count, ";"); idx >= 0 {
			count = strings.TrimSpace(count[:idx])
		}
		count = strings.TrimSuffix(count, " files") + "개"
		return "자동 작업 보관 파일이 늘어나고 있습니다: " + count + ". 감사용 기록은 유지하되, 정리가 필요하면 비서방에서 보관 계획을 요청하세요."
	}
	if strings.HasPrefix(warning, "log directory is large:") {
		size := strings.TrimSpace(strings.TrimPrefix(warning, "log directory is large:"))
		if idx := strings.Index(size, ";"); idx >= 0 {
			size = strings.TrimSpace(size[:idx])
		}
		return "로그 폴더가 큽니다: " + size + ". 로그 회전을 검토하세요."
	}
	if strings.HasPrefix(warning, "evidence files exceed 5000") {
		return "자동 작업 보관 파일이 기준치 5000개를 넘었습니다. 감사용 기록은 유지하되, 정리가 필요하면 비서방에서 보관 정책 적용을 요청하세요."
	}
	if strings.HasPrefix(warning, "state file is not valid JSON:") {
		name := strings.TrimSpace(strings.TrimPrefix(warning, "state file is not valid JSON:"))
		if name == "" {
			return "상태 파일 중 JSON 형식이 깨진 항목이 있습니다."
		}
		return "상태 파일 JSON 형식을 확인해야 합니다: " + name
	}
	switch warning {
	case "home directory not available":
		return "홈 디렉터리를 확인할 수 없습니다."
	default:
		return warning
	}
}

func visibleScheduleWatchLink(link string) string {
	link = strings.TrimSpace(link)
	if link == "" {
		return "없음"
	}
	if isLoopbackScheduleURL(link) {
		return "로컬 테스트 링크라 iPhone에서 열 수 없습니다. 실제 상품 URL이나 검색어로 다시 등록하세요."
	}
	return link
}

func isLoopbackScheduleURL(value string) bool {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.Trim(u.Hostname(), "[]"))
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

func formatWon(amount int) string {
	if amount <= 0 {
		return "확인 안 됨"
	}
	value := strconv.Itoa(amount)
	var parts []string
	for len(value) > 3 {
		parts = append([]string{value[len(value)-3:]}, parts...)
		value = value[:len(value)-3]
	}
	parts = append([]string{value}, parts...)
	return strings.Join(parts, ",") + "원"
}

func controllerLane() string {
	host, _ := os.Hostname()
	if strings.Contains(strings.ToLower(host), "mini") {
		return "macmini-worker"
	}
	if wd, err := os.Getwd(); err == nil && strings.Contains(wd, "meshclaw") {
		return "interactive-workspace"
	}
	return "local"
}

func sameLocalDate(a, b time.Time) bool {
	aa := a.Local()
	bb := b.Local()
	ay, am, ad := aa.Date()
	by, bm, bd := bb.Date()
	return ay == by && am == bm && ad == bd
}

func dailyDue(value string, last, now time.Time) bool {
	scheduled, ok := dailyAtOnLocalDate(value, now)
	if !ok {
		return true
	}
	if now.Local().Before(scheduled) {
		return false
	}
	if last.IsZero() {
		return true
	}
	return !sameLocalDate(last, now)
}

func nextDailyAtAfterLastRun(value string, last, now time.Time) (time.Time, bool) {
	scheduled, ok := dailyAtOnLocalDate(value, now)
	if !ok {
		return time.Time{}, false
	}
	if !last.IsZero() && sameLocalDate(last, now) {
		return scheduled.AddDate(0, 0, 1), true
	}
	if now.Local().Before(scheduled) {
		return scheduled, true
	}
	return now, true
}

func dailyAtOnLocalDate(value string, now time.Time) (time.Time, bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return time.Time{}, false
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return time.Time{}, false
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return time.Time{}, false
	}
	local := now.Local()
	year, month, day := local.Date()
	return time.Date(year, month, day, hour, minute, 0, 0, local.Location()), true
}

func hourlyDue(value string, last, now time.Time) bool {
	scheduled, ok := hourlyAtOnLocalHour(value, now)
	if !ok {
		return true
	}
	if now.Local().Before(scheduled) {
		return false
	}
	if last.IsZero() {
		return true
	}
	return last.Local().Before(scheduled)
}

func nextHourlyAtAfterLastRun(value string, last, now time.Time) (time.Time, bool) {
	scheduled, ok := hourlyAtOnLocalHour(value, now)
	if !ok {
		return time.Time{}, false
	}
	if !last.IsZero() && !last.Local().Before(scheduled) {
		return scheduled.Add(time.Hour), true
	}
	if now.Local().Before(scheduled) {
		return scheduled, true
	}
	return now, true
}

func hourlyAtOnLocalHour(value string, now time.Time) (time.Time, bool) {
	minute, err := strconv.Atoi(strings.Trim(strings.TrimSpace(value), ":"))
	if err != nil || minute < 0 || minute > 59 {
		return time.Time{}, false
	}
	local := now.Local()
	year, month, day := local.Date()
	return time.Date(year, month, day, local.Hour(), minute, 0, 0, local.Location()), true
}

func nextDailyAt(value string, now time.Time) (time.Time, bool) {
	next, ok := dailyAtOnLocalDate(value, now)
	if !ok {
		return time.Time{}, false
	}
	local := now.Local()
	if !next.After(local) {
		next = next.AddDate(0, 0, 1)
	}
	return next, true
}

func StatePath() string {
	if path := strings.TrimSpace(os.Getenv("MESHCLAW_SCHEDULE_STATE")); path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".meshclaw", "schedule-state.json")
	}
	return filepath.Join(home, ".meshclaw", "schedule-state.json")
}

func MailWatchStatePath() string {
	if path := strings.TrimSpace(os.Getenv("MESHCLAW_MAIL_WATCH_STATE")); path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".meshclaw", "schedule-mail-watch-state.json")
	}
	return filepath.Join(home, ".meshclaw", "schedule-mail-watch-state.json")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
