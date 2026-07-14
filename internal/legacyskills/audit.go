package legacyskills

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type AuditReport struct {
	Kind              string         `json:"kind"`
	Generated         time.Time      `json:"generated"`
	Status            string         `json:"status"`
	Path              string         `json:"path"`
	TotalSkills       int            `json:"total_skills"`
	InvalidFiles      int            `json:"invalid_files"`
	MissingDeps       int            `json:"missing_deps"`
	MutatingExternal  int            `json:"mutating_external"`
	AlreadyCovered    int            `json:"already_covered"`
	MigrateCandidates int            `json:"migrate_candidates"`
	ArchiveCandidates int            `json:"archive_candidates"`
	UserMessage       string         `json:"user_message"`
	ApprovalNote      string         `json:"approval_note"`
	Next              []string       `json:"next"`
	Summary           []SummaryRow   `json:"summary"`
	Skills            []SkillFinding `json:"skills"`
	Invalid           []InvalidFile  `json:"invalid,omitempty"`
}

type SummaryRow struct {
	Action string `json:"action"`
	Count  int    `json:"count"`
	Reason string `json:"reason"`
}

type SkillFinding struct {
	Name                string   `json:"name"`
	Description         string   `json:"description,omitempty"`
	Model               string   `json:"model,omitempty"`
	Tools               []string `json:"tools,omitempty"`
	Schedule            string   `json:"schedule,omitempty"`
	Path                string   `json:"path"`
	Risk                string   `json:"risk"`
	Dependencies        []Dep    `json:"dependencies,omitempty"`
	MissingDependencies []string `json:"missing_dependencies,omitempty"`
	RecommendedAction   string   `json:"recommended_action"`
	ReplacementTools    []string `json:"replacement_tools,omitempty"`
	Notes               []string `json:"notes,omitempty"`
}

type Dep struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type InvalidFile struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

type skillJSON struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Version      string   `json:"version"`
	Author       string   `json:"author"`
	Model        string   `json:"model"`
	SystemPrompt string   `json:"system_prompt"`
	Tools        []string `json:"tools"`
	Schedule     string   `json:"schedule"`
	ScheduleTask string   `json:"schedule_task"`
	Tags         []string `json:"tags"`
}

var coveredByMCP = map[string][]string{
	"agent-browser":      {"meshclaw_visible_browser_search", "meshclaw_browser_fetch", "meshclaw_screen_capture"},
	"apple-notes":        {"meshclaw_notes_search", "meshclaw_note_create"},
	"apple-reminders":    {"meshclaw_reminders_list", "meshclaw_reminder_create", "meshclaw_reminder_complete", "meshclaw_reminder_delete"},
	"archive":            {"meshclaw_data_archive_plan", "meshclaw_result_save"},
	"backup":             {"meshclaw_result_save"},
	"browser":            {"meshclaw_visible_browser_search", "meshclaw_browser_fetch", "meshclaw_automation_open_url"},
	"calculator":         {"meshclaw_argos_ask"},
	"calendar":           {"meshclaw_calendar_list", "meshclaw_calendar_create_event"},
	"disk-cleaner":       {"meshclaw_downloads_cleanup_plan", "meshclaw_downloads_cleanup_apply"},
	"email":              {"meshclaw_mail_accounts", "meshclaw_mail_search", "meshclaw_mail_thread", "meshclaw_mail_draft_reply", "meshclaw_mail_send"},
	"file-organizer":     {"meshclaw_downloads_cleanup_plan", "meshclaw_downloads_cleanup_apply"},
	"gh-issues":          {"meshclaw_argos_ask"},
	"git-helper":         {"meshclaw_terminal_run", "meshclaw_result_save"},
	"github":             {"meshclaw_terminal_run", "meshclaw_result_save"},
	"gpu-monitor":        {"meshclaw_server_list", "meshclaw_workflow_run"},
	"himalaya":           {"meshclaw_mail_accounts", "meshclaw_mail_search", "meshclaw_mail_thread"},
	"image-edit":         {"meshclaw_result_save"},
	"log-analyzer":       {"meshclaw_evidence_list", "meshclaw_result_save"},
	"morning-news":       {"meshclaw_news_document", "meshclaw_schedule_run_once"},
	"morning-prayer":     {"meshclaw_schedule_run_once"},
	"music":              {"meshclaw_media_play"},
	"nano-pdf":           {"meshclaw_document_export", "meshclaw_result_save"},
	"network-debug":      {"meshclaw_terminal_run", "meshclaw_result_save"},
	"news":               {"meshclaw_news_document", "meshclaw_argos_research"},
	"openai-whisper":     {"meshclaw_audio_transcribe", "meshclaw_result_save"},
	"openai-whisper-api": {"meshclaw_audio_transcribe", "meshclaw_result_save"},
	"pdf-tool":           {"meshclaw_document_export", "meshclaw_result_save"},
	"peekaboo":           {"meshclaw_screen_capture", "meshclaw_screen_record"},
	"qrcode":             {"meshclaw_result_save"},
	"reminder":           {"meshclaw_reminder_create", "meshclaw_reminders_list"},
	"reservation":        {"meshclaw_visible_browser_search", "meshclaw_maps_search", "meshclaw_maps_proof"},
	"screenshot-ocr":     {"meshclaw_screen_capture", "meshclaw_screen_record"},
	"server-monitor":     {"meshclaw_server_list", "meshclaw_workflow_run"},
	"shopping":           {"meshclaw_visible_browser_search", "meshclaw_purchase_click", "meshclaw_screen_capture"},
	"spotify-player":     {"meshclaw_media_play"},
	"summarize":          {"meshclaw_argos_research", "meshclaw_document_create"},
	"system":             {"meshclaw_doctor", "meshclaw_argos_macos_doctor"},
	"translate":          {"meshclaw_argos_ask"},
	"video-frames":       {"meshclaw_result_save"},
	"voice-news":         {"meshclaw_news_document", "meshclaw_schedule_run_once"},
	"weather":            {"meshclaw_argos_ask"},
}

var knownDeps = map[string][]string{
	"1password": {"op"}, "agent-browser": {"agent-browser"}, "apple-notes": {"memo"}, "apple-reminders": {"remindctl"},
	"archive": {"tar", "gzip", "zip", "unzip"}, "backup": {"rsync"}, "blucli": {"blu"}, "bluebubbles": {"bluebubbles"},
	"calendar": {"icalBuddy"}, "camsnap": {"ffmpeg"}, "clipboard": {"pbcopy", "pbpaste"}, "cron-manager": {"crontab"},
	"database": {"sqlite3", "psql", "mysql"}, "disk-cleaner": {"du"}, "docker-manager": {"docker"}, "downloader": {"curl"},
	"email": {"himalaya"}, "gemini": {"gemini"}, "gh-issues": {"gh"}, "gifgrep": {"gifgrep"}, "git-helper": {"git"},
	"github": {"gh"}, "gog": {"gog"}, "goplaces": {"goplaces"}, "gpu-monitor": {"nvidia-smi"}, "himalaya": {"himalaya"},
	"image-edit": {"magick"}, "imsg": {"imsg"}, "mail-monitor": {"ssh"}, "morning-news": {"edge-tts"}, "morning-prayer": {"edge-tts"},
	"notion": {"notion"}, "openhue": {"openhue"}, "ordercli": {"ordercli"}, "peekaboo": {"peekaboo"}, "reservation": {"goplaces", "agent-browser"},
	"sag": {"sag"}, "screenshot-ocr": {"screencapture", "tesseract"}, "sherpa-onnx-tts": {"sherpa-onnx-offline-tts"},
	"shopping": {"agent-browser"}, "slack": {"slack"}, "sonoscli": {"sonos"}, "spotify-player": {"spotify_player"}, "tmux": {"tmux"},
	"trello": {"trello"}, "video-frames": {"ffmpeg"}, "voice-news": {"edge-tts"}, "wacli": {"wacli"}, "xurl": {"xurl"},
}

var mutatingTerms = []string{
	"send", "delete", "remove", "clean", "backup", "archive", "book", "reserve", "purchase", "buy", "order",
	"email", "message", "write", "download", "upload", "install", "run", "execute", "control", "create", "edit",
	"manage", "set", "change", "cancel", "subscribe", "보내", "삭제", "예약", "구매", "결제", "주문", "작성", "생성", "변경", "실행",
}

func Audit(now time.Time, dir string, limit int) (AuditReport, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if limit <= 0 {
		limit = 120
	}
	if strings.TrimSpace(dir) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return AuditReport{}, err
		}
		dir = filepath.Join(home, ".meshclaw", "skills")
	}
	report := AuditReport{
		Kind:         "meshclaw_legacy_skills_audit",
		Generated:    now.UTC(),
		Status:       "ok",
		Path:         dir,
		UserMessage:  "구형 MeshClaw 스킬을 실행하지 않고 읽기 전용으로 정리했습니다.",
		ApprovalNote: "Read-only audit. This tool does not run legacy skills, install dependencies, edit files, or change active MCP behavior.",
		Next: []string{
			"Keep legacy skill.json files as reference material.",
			"Migrate useful ideas into approval-gated MCP tools instead of reviving the old skill runner.",
			"Archive or ignore duplicate skills once an MCP replacement exists.",
		},
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			report.Status = "path_missing"
			report.UserMessage = "구형 스킬 디렉터리를 찾지 못했습니다."
			return report, nil
		}
		return report, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name(), "skill.json")
		data, err := os.ReadFile(path)
		if err != nil {
			report.InvalidFiles++
			report.Invalid = append(report.Invalid, InvalidFile{Path: path, Error: err.Error()})
			continue
		}
		var skill skillJSON
		if err := json.Unmarshal(data, &skill); err != nil {
			report.InvalidFiles++
			report.Invalid = append(report.Invalid, InvalidFile{Path: path, Error: err.Error()})
			continue
		}
		if skill.Name == "" {
			skill.Name = entry.Name()
		}
		finding := classifySkill(path, skill)
		report.TotalSkills++
		if len(finding.MissingDependencies) > 0 {
			report.MissingDeps++
		}
		if finding.Risk == "mutating_or_external" {
			report.MutatingExternal++
		}
		switch finding.RecommendedAction {
		case "already_covered_by_mcp":
			report.AlreadyCovered++
		case "migrate_to_mcp":
			report.MigrateCandidates++
		case "archive_reference":
			report.ArchiveCandidates++
		}
		if len(report.Skills) < limit {
			report.Skills = append(report.Skills, finding)
		}
	}
	sort.Slice(report.Skills, func(i, j int) bool {
		if report.Skills[i].RecommendedAction == report.Skills[j].RecommendedAction {
			return report.Skills[i].Name < report.Skills[j].Name
		}
		return report.Skills[i].RecommendedAction < report.Skills[j].RecommendedAction
	})
	report.Summary = []SummaryRow{
		{Action: "already_covered_by_mcp", Count: report.AlreadyCovered, Reason: "An approval-gated MCP tool already covers the intent better than the old prompt skill."},
		{Action: "migrate_to_mcp", Count: report.MigrateCandidates, Reason: "Useful assistant capability, but should be reimplemented as a bounded MCP tool or toolset."},
		{Action: "archive_reference", Count: report.ArchiveCandidates, Reason: "Keep as reference or test data; do not expose to normal users."},
	}
	return report, nil
}

func classifySkill(path string, skill skillJSON) SkillFinding {
	text := strings.ToLower(skill.Description + "\n" + skill.SystemPrompt)
	risk := "medium_review"
	for _, term := range mutatingTerms {
		if strings.Contains(text, strings.ToLower(term)) {
			risk = "mutating_or_external"
			break
		}
	}
	if isReadMostly(skill.Name) {
		risk = "read_mostly"
	}
	finding := SkillFinding{
		Name:        skill.Name,
		Description: skill.Description,
		Model:       skill.Model,
		Tools:       append([]string(nil), skill.Tools...),
		Schedule:    skill.Schedule,
		Path:        path,
		Risk:        risk,
		Notes: []string{
			"Legacy skill.json prompt; do not run through the old skill runner by default.",
		},
	}
	for _, dep := range knownDeps[skill.Name] {
		status := "ok"
		if _, err := exec.LookPath(dep); err != nil {
			status = "missing"
			finding.MissingDependencies = append(finding.MissingDependencies, dep)
		}
		finding.Dependencies = append(finding.Dependencies, Dep{Name: dep, Status: status})
	}
	if replacements, ok := coveredByMCP[skill.Name]; ok {
		finding.RecommendedAction = "already_covered_by_mcp"
		finding.ReplacementTools = append([]string(nil), replacements...)
		finding.Notes = append(finding.Notes, "Prefer the listed MCP replacement tools.")
	} else if skill.Name == "hello" {
		finding.RecommendedAction = "archive_reference"
		finding.Notes = append(finding.Notes, "Test-only skill.")
	} else {
		finding.RecommendedAction = "migrate_to_mcp"
		finding.Notes = append(finding.Notes, "Useful only after a bounded MCP wrapper, approval policy, and evidence path are defined.")
	}
	return finding
}

func isReadMostly(name string) bool {
	switch name {
	case "calculator", "finance", "hello", "model-usage", "news", "session-logs", "summarize", "system", "translate", "weather":
		return true
	default:
		return false
	}
}
