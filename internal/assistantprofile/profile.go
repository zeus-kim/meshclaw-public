package assistantprofile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/meshclaw/meshclaw/internal/guard"
)

const (
	Kind = "meshclaw_assistant_profile"
)

const MaxMemoryFileBytes = 32768

type Report struct {
	Kind      string       `json:"kind"`
	Generated time.Time    `json:"generated"`
	Status    string       `json:"status"`
	Root      string       `json:"root"`
	Identity  Identity     `json:"identity"`
	Memory    Memory       `json:"memory"`
	Skills    Skills       `json:"skills"`
	Approvals []string     `json:"approvals"`
	Warnings  []string     `json:"warnings,omitempty"`
	Next      []string     `json:"next"`
	Parity    []ParityItem `json:"parity"`
}

type InitReport struct {
	Kind      string     `json:"kind"`
	Generated time.Time  `json:"generated"`
	Root      string     `json:"root"`
	Execute   bool       `json:"execute"`
	Created   []FileInfo `json:"created,omitempty"`
	Existing  []FileInfo `json:"existing,omitempty"`
	Planned   []FileInfo `json:"planned,omitempty"`
	Message   string     `json:"message"`
	Profile   Report     `json:"profile"`
}

type MemoryWriteReport struct {
	Kind          string          `json:"kind"`
	Generated     time.Time       `json:"generated"`
	Root          string          `json:"root"`
	Scope         string          `json:"scope"`
	Path          string          `json:"path"`
	Approve       bool            `json:"approve"`
	Status        string          `json:"status"`
	Message       string          `json:"message"`
	Text          string          `json:"text,omitempty"`
	Written       bool            `json:"written"`
	Rejected      bool            `json:"rejected"`
	GuardFindings []guard.Finding `json:"guard_findings,omitempty"`
	Next          []string        `json:"next,omitempty"`
}

type SkillInstallReport struct {
	Kind          string          `json:"kind"`
	Generated     time.Time       `json:"generated"`
	Root          string          `json:"root"`
	Name          string          `json:"name"`
	Slug          string          `json:"slug"`
	Path          string          `json:"path"`
	Approve       bool            `json:"approve"`
	Status        string          `json:"status"`
	Message       string          `json:"message"`
	Summary       string          `json:"summary"`
	Written       bool            `json:"written"`
	Rejected      bool            `json:"rejected"`
	Files         []FileInfo      `json:"files,omitempty"`
	GuardFindings []guard.Finding `json:"guard_findings,omitempty"`
	Next          []string        `json:"next,omitempty"`
}

type SkillRevisionReport struct {
	Kind          string          `json:"kind"`
	Generated     time.Time       `json:"generated"`
	Root          string          `json:"root"`
	Query         string          `json:"query"`
	Name          string          `json:"name,omitempty"`
	Slug          string          `json:"slug,omitempty"`
	Path          string          `json:"path,omitempty"`
	Approve       bool            `json:"approve"`
	Status        string          `json:"status"`
	Message       string          `json:"message"`
	Summary       string          `json:"summary,omitempty"`
	Skill         SkillInfo       `json:"skill,omitempty"`
	Written       bool            `json:"written"`
	Rejected      bool            `json:"rejected"`
	Files         []FileInfo      `json:"files,omitempty"`
	GuardFindings []guard.Finding `json:"guard_findings,omitempty"`
	Next          []string        `json:"next,omitempty"`
}

type SkillTemplate struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Example string `json:"example"`
	Status  string `json:"status"`
}

type SkillTestReport struct {
	Kind      string    `json:"kind"`
	Generated time.Time `json:"generated"`
	Root      string    `json:"root"`
	Query     string    `json:"query"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	Skill     SkillInfo `json:"skill,omitempty"`
	TestPath  string    `json:"test_path,omitempty"`
	Prompt    string    `json:"prompt,omitempty"`
	Expected  []string  `json:"expected,omitempty"`
	Safety    []string  `json:"safety,omitempty"`
	Next      []string  `json:"next,omitempty"`
}

type SkillActivationReport struct {
	Kind       string    `json:"kind"`
	Generated  time.Time `json:"generated"`
	Root       string    `json:"root"`
	Query      string    `json:"query"`
	Action     string    `json:"action"`
	Approve    bool      `json:"approve"`
	Status     string    `json:"status"`
	Message    string    `json:"message"`
	Skill      SkillInfo `json:"skill,omitempty"`
	ActivePath string    `json:"active_path,omitempty"`
	Written    bool      `json:"written"`
	Rejected   bool      `json:"rejected"`
	Next       []string  `json:"next,omitempty"`
}

type MemoryContext struct {
	Kind      string    `json:"kind"`
	Generated time.Time `json:"generated"`
	Root      string    `json:"root"`
	Status    string    `json:"status"`
	Text      string    `json:"text"`
	Files     []string  `json:"files"`
	Warnings  []string  `json:"warnings,omitempty"`
}

type MemorySnapshot struct {
	Kind      string        `json:"kind"`
	Generated time.Time     `json:"generated"`
	Root      string        `json:"root"`
	Request   string        `json:"request,omitempty"`
	Status    string        `json:"status"`
	Layers    []MemoryLayer `json:"layers"`
	Use       []string      `json:"use"`
	Warnings  []string      `json:"warnings,omitempty"`
}

type MemoryLayer struct {
	Name      string   `json:"name"`
	Status    string   `json:"status"`
	Source    string   `json:"source"`
	Use       string   `json:"use"`
	Injected  bool     `json:"injected"`
	Items     int      `json:"items,omitempty"`
	Summary   string   `json:"summary,omitempty"`
	Paths     []string `json:"paths,omitempty"`
	Retention string   `json:"retention,omitempty"`
	Boundary  string   `json:"boundary,omitempty"`
}

type Identity struct {
	DisplayName string     `json:"display_name"`
	Language    string     `json:"language"`
	Files       []FileInfo `json:"files"`
}

type Memory struct {
	Policy string     `json:"policy"`
	Files  []FileInfo `json:"files"`
}

type Skills struct {
	Policy string      `json:"policy"`
	Root   string      `json:"root"`
	Count  int         `json:"count"`
	Items  []SkillInfo `json:"items,omitempty"`
}

type FileInfo struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Exists    bool   `json:"exists"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	Status    string `json:"status"`
}

type SkillInfo struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	Source     string    `json:"source"`
	Status     string    `json:"status"`
	Title      string    `json:"title,omitempty"`
	Summary    string    `json:"summary,omitempty"`
	Review     string    `json:"review"`
	Activation string    `json:"activation"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
}

type ParityItem struct {
	Area     string `json:"area"`
	Status   string `json:"status"`
	MeshClaw string `json:"meshclaw"`
}

func Inspect(now time.Time, root string) (Report, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if strings.TrimSpace(root) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Report{}, err
		}
		root = filepath.Join(home, ".meshclaw")
	}
	root = filepath.Clean(root)
	report := Report{
		Kind:      Kind,
		Generated: now.UTC(),
		Status:    "ok",
		Root:      root,
		Identity: Identity{
			DisplayName: "Argos",
			Language:    "ko",
			Files: []FileInfo{
				fileInfo("SOUL.md", filepath.Join(root, "assistant", "SOUL.md")),
				fileInfo("USER.md", filepath.Join(root, "assistant", "USER.md")),
				fileInfo("AGENTS.md", filepath.Join(root, "assistant", "AGENTS.md")),
			},
		},
		Memory: Memory{
			Policy: "bounded_approval_required",
			Files: []FileInfo{
				fileInfo("USER.md", filepath.Join(root, "memory", "USER.md")),
				fileInfo("MEMORY.md", filepath.Join(root, "memory", "MEMORY.md")),
			},
		},
		Skills: Skills{
			Policy: "allowlisted_write_approval_required",
			Root:   filepath.Join(root, "skills"),
		},
		Approvals: []string{
			"identity files are read-only context until the user edits or approves a change",
			"memory writes require explicit approval and must not store raw secrets",
			"skill installs or generated skills require review before becoming active",
			"ops facts come from evidence/opsdb, not assistant memory",
		},
		Next: []string{
			"Create assistant/SOUL.md, assistant/USER.md, and assistant/AGENTS.md when persona needs customization.",
			"Keep memory/USER.md and memory/MEMORY.md bounded and approval-gated.",
			"Port useful OpenClaw/Hermes skills into MCP tools or reviewed SKILL.md entries.",
		},
		Parity: []ParityItem{
			{Area: "identity", Status: "ready", MeshClaw: "Argos deterministic Signal identity plus Markdown identity files"},
			{Area: "memory", Status: "guarded", MeshClaw: "bounded Markdown memory with approval-required writes"},
			{Area: "skills", Status: "guarded", MeshClaw: "allowlisted skill roots; legacy skills stay read-only until migrated"},
			{Area: "gateway", Status: "active", MeshClaw: "Signal targets use explicit mode and delivery policy"},
			{Area: "schedules", Status: "active", MeshClaw: "scheduler and scheduled delivery plans are approval-first"},
			{Area: "tool_policy", Status: "active", MeshClaw: "MCP profiles hide destructive/direct tools from lite clients"},
		},
	}
	report.Skills.Items = listSkills(report.Skills.Root, 40)
	report.Skills.Count = len(report.Skills.Items)
	for _, info := range append(append([]FileInfo{}, report.Identity.Files...), report.Memory.Files...) {
		if !info.Exists {
			report.Warnings = append(report.Warnings, info.Name+" missing: "+info.Path)
		}
	}
	if len(report.Warnings) > 0 {
		report.Status = "warn"
	}
	return report, nil
}

func InitDefaults(now time.Time, root string, execute bool) (InitReport, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if strings.TrimSpace(root) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return InitReport{}, err
		}
		root = filepath.Join(home, ".meshclaw")
	}
	root = filepath.Clean(root)
	files := map[string]string{
		filepath.Join(root, "assistant", "SOUL.md"):   defaultSoul(),
		filepath.Join(root, "assistant", "USER.md"):   defaultAssistantUser(),
		filepath.Join(root, "assistant", "AGENTS.md"): defaultAgents(),
		filepath.Join(root, "memory", "USER.md"):      defaultMemoryUser(),
		filepath.Join(root, "memory", "MEMORY.md"):    defaultMemory(),
	}
	report := InitReport{
		Kind:      "meshclaw_assistant_profile_init",
		Generated: now.UTC(),
		Root:      root,
		Execute:   execute,
		Message:   "planned assistant profile initialization; no files written",
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		name := filepath.Base(path)
		if info := fileInfo(name, path); info.Exists {
			report.Existing = append(report.Existing, info)
			continue
		}
		info := FileInfo{Name: name, Path: path, Status: "planned"}
		if !execute {
			report.Planned = append(report.Planned, info)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return report, err
		}
		if err := os.WriteFile(path, []byte(files[path]), 0600); err != nil {
			return report, err
		}
		report.Created = append(report.Created, fileInfo(name, path))
	}
	if execute {
		if err := os.MkdirAll(filepath.Join(root, "skills"), 0700); err != nil {
			return report, err
		}
		report.Message = "assistant profile defaults are ready"
	}
	profile, err := Inspect(now, root)
	if err != nil {
		return report, err
	}
	report.Profile = profile
	return report, nil
}

func SkillTemplates() []SkillTemplate {
	return []SkillTemplate{
		{
			ID:      "market-analysis",
			Title:   "Market Analysis",
			Summary: "Compare market drivers, company signals, prices, risks, and scenarios into a decision brief with visible assumptions.",
			Example: "유가, 환율, 금리 변화를 비교해서 이번 주 시장 분석과 대응 시나리오로 정리해줘",
			Status:  "available_template",
		},
		{
			ID:      "research-report",
			Title:   "Research Report",
			Summary: "Collect current sources, summarize findings, and return a mobile-readable Markdown or DOCX report with citations when available.",
			Example: "최근 화학회사들이 민감하게 볼 뉴스를 시장조사로 분석해서 회의 토픽으로 정리해줘",
			Status:  "available_template",
		},
		{
			ID:      "travel-prep",
			Title:   "Travel Prep",
			Summary: "Compare routes, flights, hotels, local constraints, and itinerary options, then stop before booking or payment.",
			Example: "다음 주 서울-도쿄 출장 항공권/호텔 후보와 이동 동선을 비교해서 모바일 표로 정리해줘",
			Status:  "available_template",
		},
		{
			ID:      "meeting-minutes",
			Title:   "Meeting Minutes",
			Summary: "Turn notes, transcript text, or agenda fragments into decisions, action items, and a concise meeting record.",
			Example: "이 회의 메모를 결정사항/담당자/마감일 중심으로 회의록 DOCX로 만들어줘",
			Status:  "available_template",
		},
		{
			ID:      "shopping-prep",
			Title:   "Shopping Prep",
			Summary: "Compare products, prepare checkout evidence, and stop before purchase until the user gives final explicit approval.",
			Example: "쿠팡에서 5만원 이하 무선 키보드 후보를 찾고 구매 직전 검토표로 정리해줘",
			Status:  "available_template",
		},
		{
			ID:      "mail-priority",
			Title:   "Mail Priority",
			Summary: "Summarize recent mail, rank what needs reply, and draft responses without sending until approved.",
			Example: "최근 메일 요약하고 오늘 답장할 것만 우선순위로 정리해줘",
			Status:  "available_template",
		},
		{
			ID:      "calendar-reminder",
			Title:   "Calendar Reminder",
			Summary: "Prepare calendar events, reminders, attendee notes, and conflict checks while waiting for final approval before creating anything.",
			Example: "내일 미팅 일정 후보와 리마인더 문구를 정리하고 캘린더에 넣기 전 확인표를 만들어줘",
			Status:  "available_template",
		},
		{
			ID:      "daily-briefing",
			Title:   "Daily Briefing",
			Summary: "Combine weather, calendar, reminders, mail, and priority news into a short morning or end-of-day brief.",
			Example: "오늘 날씨, 일정, 리마인더, 중요한 메일을 아침 브리핑으로 짧게 정리해줘",
			Status:  "available_template",
		},
		{
			ID:      "work-reuse",
			Title:   "Work Reuse",
			Summary: "Turn repeated assistant work into a reviewed reusable routine with a smoke test, memory handoff, and activation command.",
			Example: "매주 하는 시장 뉴스 회의 토픽 정리를 작업 재사용 스킬로 만들어줘",
			Status:  "available_template",
		},
		{
			ID:      "voice-briefing",
			Title:   "Voice Briefing",
			Summary: "Turn a finished summary or briefing into a spoken script or audio-ready artifact without calling or sending until approved.",
			Example: "시장 분석 요약을 90초 음성 브리핑 대본과 모바일용 요약으로 만들어줘",
			Status:  "available_template",
		},
	}
}

func InstallSkill(now time.Time, root, name, summary string, approve bool) (SkillInstallReport, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	root, err := resolveRoot(root)
	if err != nil {
		return SkillInstallReport{}, err
	}
	name = skillTitle(name)
	summary = strings.TrimSpace(summary)
	if summary == "" {
		summary = name
	}
	slug := skillSlug(name)
	dir := filepath.Join(root, "skills", slug)
	report := SkillInstallReport{
		Kind:      "meshclaw_assistant_skill_install",
		Generated: now.UTC(),
		Root:      root,
		Name:      name,
		Slug:      slug,
		Path:      dir,
		Approve:   approve,
		Status:    "planned",
		Message:   "skill install planned; no files changed",
		Summary:   summary,
		Next: []string{
			"Review the generated SKILL.md behavior.",
			"Approve only if it is a bounded assistant routine and dangerous actions stay approval-gated.",
		},
	}
	if strings.TrimSpace(name) == "" || slug == "" {
		report.Status = "rejected"
		report.Rejected = true
		report.Message = "skill name is empty"
		return report, nil
	}
	guardReport := guard.Detect("assistant_skill", strings.Join([]string{name, summary}, "\n"))
	if len(guardReport.Findings) > 0 {
		report.Status = "rejected"
		report.Rejected = true
		report.Message = "raw secret-like text is not allowed in assistant skills"
		report.GuardFindings = guardReport.Findings
		report.Summary = guardReport.Redacted
		report.Next = []string{"Remove raw secrets and keep only non-secret behavior instructions or vault handles."}
		return report, nil
	}
	files := map[string]string{
		filepath.Join(dir, "ACTIVE.md"):   skillActiveMarkdown(now),
		filepath.Join(dir, "REVIEWED.md"): skillReviewedMarkdown(now),
		filepath.Join(dir, "SKILL.md"):    skillMarkdown(now, name, summary),
		filepath.Join(dir, "TEST.md"):     skillTestMarkdown(name, summary),
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	if exists(filepath.Join(dir, "SKILL.md")) && approve {
		report.Status = "rejected"
		report.Rejected = true
		report.Message = "skill already exists; choose a new name before overwriting"
		for _, path := range paths {
			report.Files = append(report.Files, fileInfo(filepath.Base(path), path))
		}
		return report, nil
	}
	for _, path := range paths {
		if !approve {
			report.Files = append(report.Files, FileInfo{Name: filepath.Base(path), Path: path, Status: "planned"})
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return report, err
		}
		if err := os.WriteFile(path, []byte(files[path]), 0600); err != nil {
			return report, err
		}
		report.Files = append(report.Files, fileInfo(filepath.Base(path), path))
	}
	if approve {
		report.Status = "active_policy_gated"
		report.Message = "approved skill files installed; activation is policy-gated"
		report.Written = true
		report.Next = []string{"Use `스킬 현황` in Signal to verify the active reviewed skill."}
	}
	return report, nil
}

func ReviseSkill(now time.Time, root, query, summary string, approve bool) (SkillRevisionReport, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	root, err := resolveRoot(root)
	if err != nil {
		return SkillRevisionReport{}, err
	}
	query = strings.TrimSpace(query)
	summary = strings.TrimSpace(summary)
	report := SkillRevisionReport{
		Kind:      "meshclaw_assistant_skill_revision",
		Generated: now.UTC(),
		Root:      root,
		Query:     query,
		Approve:   approve,
		Status:    "not_found",
		Message:   "skill not found",
		Summary:   summary,
		Next:      []string{"Use `스킬 현황` to choose an installed skill."},
	}
	profile, err := Inspect(now, root)
	if err != nil {
		return report, err
	}
	if query == "" {
		report.Status = "needs_skill"
		report.Message = "skill name is required"
		return report, nil
	}
	if summary == "" {
		report.Status = "needs_revision"
		report.Rejected = true
		report.Message = "skill revision text is required"
		report.Next = []string{"Provide the skill name and the revised bounded behavior."}
		return report, nil
	}
	info, ok := matchSkill(profile.Skills.Items, query)
	if !ok {
		return report, nil
	}
	report.Skill = info
	report.Name = info.Name
	report.Slug = info.Name
	report.Path = info.Path
	if info.Source != "skill_md" {
		report.Status = "rejected"
		report.Rejected = true
		report.Message = "only SKILL.md skills can be revised through this flow"
		report.Next = []string{"Migrate legacy skills to SKILL.md before revising them."}
		return report, nil
	}
	title := firstNonEmpty(info.Title, info.Name, query)
	guardReport := guard.Detect("assistant_skill", strings.Join([]string{title, summary}, "\n"))
	if len(guardReport.Findings) > 0 {
		report.Status = "rejected"
		report.Rejected = true
		report.Message = "raw secret-like text is not allowed in assistant skills"
		report.GuardFindings = guardReport.Findings
		report.Summary = guardReport.Redacted
		report.Next = []string{"Remove raw secrets and keep only non-secret behavior instructions or vault handles."}
		return report, nil
	}
	files := map[string]string{
		filepath.Join(info.Path, "REVIEWED.md"): skillReviewedMarkdown(now),
		filepath.Join(info.Path, "SKILL.md"):    skillMarkdown(now, title, summary),
		filepath.Join(info.Path, "TEST.md"):     skillTestMarkdown(title, summary),
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if !approve {
			report.Files = append(report.Files, FileInfo{Name: filepath.Base(path), Path: path, Status: "planned"})
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return report, err
		}
		if err := os.WriteFile(path, []byte(files[path]), 0600); err != nil {
			return report, err
		}
		report.Files = append(report.Files, fileInfo(filepath.Base(path), path))
	}
	if !approve {
		report.Status = "planned"
		report.Message = "skill revision planned; no files changed"
		report.Next = []string{"Approve only after reviewing the revised bounded behavior."}
		return report, nil
	}
	report.Status = firstNonEmpty(info.Activation, "reviewed_approval_required")
	report.Message = "skill revision applied; activation marker preserved"
	report.Written = true
	report.Next = []string{"Run the skill test card again and review the pasted result before relying on the revision."}
	return report, nil
}

func TestSkill(now time.Time, root, query string) (SkillTestReport, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	root, err := resolveRoot(root)
	if err != nil {
		return SkillTestReport{}, err
	}
	report := SkillTestReport{
		Kind:      "meshclaw_assistant_skill_test",
		Generated: now.UTC(),
		Root:      root,
		Query:     strings.TrimSpace(query),
		Status:    "not_found",
		Message:   "skill not found",
		Next:      []string{"Use `스킬 현황` or `스킬 마켓 보여줘` to choose a skill."},
	}
	profile, err := Inspect(now, root)
	if err != nil {
		return report, err
	}
	if strings.TrimSpace(query) == "" {
		report.Status = "needs_skill"
		report.Message = "skill name is required"
		return report, nil
	}
	info, ok := matchSkill(profile.Skills.Items, query)
	if !ok {
		return report, nil
	}
	report.Skill = info
	report.Status = "ready"
	report.Message = "skill smoke test card is ready; no actions executed"
	report.TestPath = filepath.Join(info.Path, "TEST.md")
	report.Safety = []string{
		"Do not send mail, delete data, purchase, book, pay, or change accounts during this smoke test.",
		"Return a draft, checklist, or summary only.",
		"Require explicit final approval before irreversible action.",
	}
	report.Next = []string{
		"Run the prompt manually in Signal if you want to exercise the skill.",
		"Check that the reply stops before irreversible actions.",
	}
	data, err := os.ReadFile(report.TestPath)
	if err != nil {
		if os.IsNotExist(err) {
			report.Status = "missing_test"
			report.Message = "skill exists but TEST.md is missing; generated fallback smoke test"
			report.Prompt = skillTestPrompt(firstNonEmpty(info.Title, info.Name), info.Summary)
			report.Expected = []string{"Produces a concrete assistant result.", "Stops before irreversible action."}
			return report, nil
		}
		return report, err
	}
	report.Prompt, report.Expected = parseSkillTestMarkdown(string(data))
	if strings.TrimSpace(report.Prompt) == "" {
		report.Prompt = skillTestPrompt(firstNonEmpty(info.Title, info.Name), info.Summary)
	}
	if len(report.Expected) == 0 {
		report.Expected = []string{"Produces a concrete assistant result.", "Stops before irreversible action."}
	}
	return report, nil
}

func SetSkillActivation(now time.Time, root, query string, active, approve bool) (SkillActivationReport, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	root, err := resolveRoot(root)
	if err != nil {
		return SkillActivationReport{}, err
	}
	action := "deactivate"
	if active {
		action = "activate"
	}
	report := SkillActivationReport{
		Kind:      "meshclaw_assistant_skill_activation",
		Generated: now.UTC(),
		Root:      root,
		Query:     strings.TrimSpace(query),
		Action:    action,
		Approve:   approve,
		Status:    "not_found",
		Message:   "skill not found",
		Next:      []string{"Use `스킬 현황` to choose an installed skill."},
	}
	profile, err := Inspect(now, root)
	if err != nil {
		return report, err
	}
	if strings.TrimSpace(query) == "" {
		report.Status = "needs_skill"
		report.Message = "skill name is required"
		return report, nil
	}
	info, ok := matchSkill(profile.Skills.Items, query)
	if !ok {
		return report, nil
	}
	report.Skill = info
	report.ActivePath = filepath.Join(info.Path, "ACTIVE.md")
	if info.Source != "skill_md" {
		report.Status = "rejected"
		report.Rejected = true
		report.Message = "only reviewed SKILL.md skills can be activated or deactivated"
		report.Next = []string{"Migrate legacy skills to reviewed SKILL.md before activation changes."}
		return report, nil
	}
	if active && info.Review != "reviewed" {
		report.Status = "rejected"
		report.Rejected = true
		report.Message = "skill must be reviewed before activation"
		report.Next = []string{"Add REVIEWED.md after human review, then activate the skill."}
		return report, nil
	}
	if active && info.Activation == "active_policy_gated" {
		report.Status = "already_active"
		report.Message = "skill is already active under policy gate"
		return report, nil
	}
	if !active && info.Activation != "active_policy_gated" {
		report.Status = "already_inactive"
		report.Message = "skill is already inactive"
		return report, nil
	}
	report.Status = "planned"
	report.Message = "skill activation change planned; no files changed"
	report.Next = []string{"Approve only if this skill should change active state."}
	if !approve {
		return report, nil
	}
	if active {
		if err := os.WriteFile(report.ActivePath, []byte(skillActiveMarkdown(now)), 0600); err != nil {
			return report, err
		}
		report.Status = "active_policy_gated"
		report.Message = "skill activated under policy gate"
		report.Written = true
		return report, nil
	}
	disabledPath := filepath.Join(info.Path, "DISABLED.md")
	if err := os.WriteFile(disabledPath, []byte("disabled: "+now.UTC().Format(time.RFC3339)+"\nactivation: inactive\n"), 0600); err != nil {
		return report, err
	}
	if err := os.Remove(report.ActivePath); err != nil && !os.IsNotExist(err) {
		return report, err
	}
	report.Status = "reviewed_approval_required"
	report.Message = "skill deactivated; files kept for review"
	report.Written = true
	return report, nil
}

func AddMemory(now time.Time, root, scope, text string, approve bool) (MemoryWriteReport, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	root, err := resolveRoot(root)
	if err != nil {
		return MemoryWriteReport{}, err
	}
	scope = normalizeMemoryScope(scope)
	text = strings.TrimSpace(text)
	path := memoryPath(root, scope)
	report := MemoryWriteReport{
		Kind:      "meshclaw_assistant_memory_write",
		Generated: now.UTC(),
		Root:      root,
		Scope:     scope,
		Path:      path,
		Approve:   approve,
		Status:    "planned",
		Message:   "memory write planned; no file changed",
		Text:      text,
		Next: []string{
			"Review the memory text.",
			"Run again with --approve only if it is stable and contains no raw secret.",
		},
	}
	if text == "" {
		report.Status = "rejected"
		report.Rejected = true
		report.Message = "memory text is empty"
		return report, nil
	}
	guardReport := guard.Detect("assistant_memory", text)
	if len(guardReport.Findings) > 0 {
		report.Status = "rejected"
		report.Rejected = true
		report.Message = "raw secret-like text is not allowed in assistant memory"
		report.Text = guardReport.Redacted
		report.GuardFindings = guardReport.Findings
		report.Next = []string{"Store the raw value through Guard/Vault and remember only a vault handle or non-secret preference."}
		return report, nil
	}
	if !approve {
		return report, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return report, err
	}
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return report, err
	}
	entry := fmt.Sprintf("\n- %s: %s\n", now.UTC().Format("2006-01-02"), singleLineMemory(text))
	if int64(len(existing)+len(entry)) > MaxMemoryFileBytes {
		report.Status = "rejected"
		report.Rejected = true
		report.Message = fmt.Sprintf("memory file would exceed %d bytes", MaxMemoryFileBytes)
		return report, nil
	}
	if len(existing) == 0 {
		existing = []byte(defaultMemoryHeader(scope))
	}
	existing = append(existing, []byte(entry)...)
	if err := os.WriteFile(path, existing, 0600); err != nil {
		return report, err
	}
	report.Status = "written"
	report.Message = "approved memory entry appended"
	report.Written = true
	report.Next = []string{"Use `meshclaw assistant profile --json` to inspect memory file status."}
	return report, nil
}

func LoadMemoryContext(now time.Time, root string, maxBytes int64) (MemoryContext, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	root, err := resolveRoot(root)
	if err != nil {
		return MemoryContext{}, err
	}
	if maxBytes <= 0 {
		maxBytes = 2048
	}
	ctx := MemoryContext{
		Kind:      "meshclaw_assistant_memory_context",
		Generated: now.UTC(),
		Root:      root,
		Status:    "ok",
	}
	sections := []string{}
	for _, file := range []struct {
		title string
		path  string
	}{
		{title: "User memory", path: memoryPath(root, "user")},
		{title: "Assistant memory", path: memoryPath(root, "assistant")},
	} {
		data, err := os.ReadFile(file.path)
		if err != nil {
			if os.IsNotExist(err) {
				ctx.Warnings = append(ctx.Warnings, "missing: "+file.path)
				continue
			}
			return ctx, err
		}
		ctx.Files = append(ctx.Files, file.path)
		text := strings.TrimSpace(string(data))
		if text == "" {
			continue
		}
		sections = append(sections, "## "+file.title+"\n"+tailString(text, maxBytes/2))
	}
	ctx.Text = strings.TrimSpace(strings.Join(sections, "\n\n"))
	if ctx.Text == "" {
		ctx.Status = "empty"
	}
	if len(ctx.Warnings) > 0 && ctx.Text == "" {
		ctx.Status = "warn"
	}
	return ctx, nil
}

func BuildMemorySnapshot(now time.Time, root, request string) (MemorySnapshot, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	root, err := resolveRoot(root)
	if err != nil {
		return MemorySnapshot{}, err
	}
	profile, err := Inspect(now, root)
	if err != nil {
		return MemorySnapshot{}, err
	}
	longTerm, err := LoadMemoryContext(now, root, 2400)
	if err != nil {
		return MemorySnapshot{}, err
	}
	req := strings.ToLower(strings.TrimSpace(request))
	snapshot := MemorySnapshot{
		Kind:      "meshclaw_assistant_memory_snapshot",
		Generated: now.UTC(),
		Root:      root,
		Request:   strings.TrimSpace(request),
		Status:    "ok",
		Layers: []MemoryLayer{
			{
				Name:      "short_term",
				Status:    "available_in_signal_runtime",
				Source:    "Signal room history",
				Use:       "직전 대화 흐름, 짧은 후속 질문, 방금 말한 지시",
				Injected:  wantsAny(req, "방금", "아까", "그거", "이어서", "follow up", "continue"),
				Retention: "bounded recent turns",
				Boundary:  "room scoped; not a source of permanent truth",
			},
			{
				Name:      "working",
				Status:    "available_in_messenger_runtime",
				Source:    "pending approvals, recent artifacts, recent news/mail contexts",
				Use:       "방금 만든 파일, 승인 대기, 최근 메일/뉴스/첨부 후속 처리",
				Injected:  wantsAny(req, "방금", "최근", "다시 보내", "승인", "저장해", "첫 번째", "1번"),
				Retention: "minutes to hours depending on context",
				Boundary:  "expires; should be promoted to evidence/artifact if durable",
			},
			{
				Name:      "long_term",
				Status:    longTerm.Status,
				Source:    "~/.meshclaw/memory/USER.md and MEMORY.md",
				Use:       "승인된 사용자 선호와 Argos 운영 습관",
				Injected:  longTerm.Status == "ok" && wantsAny(req, "기억", "선호", "답변", "브리핑", "remember", "preference", "answer style"),
				Items:     countMemoryItems(longTerm.Text),
				Summary:   clipSkillText(longTerm.Text, 360),
				Paths:     longTerm.Files,
				Retention: "durable until edited",
				Boundary:  "approval required; raw secrets rejected by Guard",
			},
			{
				Name:      "episodic",
				Status:    "available_as_evidence",
				Source:    "~/.meshclaw/evidence and opsdb evidence index",
				Use:       "과거 발송, 보고서, 작업 결과, 장애/조치 이력 검색",
				Injected:  wantsAny(req, "지난", "언제", "기록", "보고", "증거", "evidence", "history"),
				Retention: "evidence retention policy",
				Boundary:  "redacted records; raw secrets must not be stored",
			},
			{
				Name:      "semantic",
				Status:    "planned",
				Source:    "Obsidian/docs/project indexes",
				Use:       "문서/프로젝트 지식 검색과 RAG",
				Injected:  wantsAny(req, "문서", "오브시디언", "프로젝트", "docs", "obsidian"),
				Retention: "document lifecycle",
				Boundary:  "must avoid raw-secret/RAG ingestion",
			},
			{
				Name:      "procedural",
				Status:    proceduralStatus(profile.Skills.Items),
				Source:    "~/.meshclaw/skills and MCP tool contracts",
				Use:       "반복 절차, 브리핑 방식, 도구 선택 습관",
				Injected:  wantsAny(req, "어떻게", "항상", "절차", "스킬", "skill", "workflow"),
				Items:     len(profile.Skills.Items),
				Summary:   skillSnapshotSummary(profile.Skills.Items, 4),
				Paths:     []string{profile.Skills.Root},
				Retention: "reviewed skills stay disabled until approved for activation",
				Boundary:  profile.Skills.Policy,
			},
			{
				Name:      "ops",
				Status:    "available_as_opsdb",
				Source:    "opsdb, monitor, agent history",
				Use:       "노드 상태, 보안/포트/프로세스/로그 이상, DevOps 보고",
				Injected:  wantsAny(req, "서버", "노드", "포트", "프로세스", "보안", "로그", "ops", "devops"),
				Retention: "opsdb retention policy",
				Boundary:  "separate from personal assistant memory",
			},
		},
	}
	for _, warning := range append(profile.Warnings, longTerm.Warnings...) {
		if strings.TrimSpace(warning) != "" {
			snapshot.Warnings = append(snapshot.Warnings, warning)
		}
	}
	for _, layer := range snapshot.Layers {
		if layer.Injected {
			snapshot.Use = append(snapshot.Use, layer.Name)
		}
	}
	if len(snapshot.Warnings) > 0 {
		snapshot.Status = "warn"
	}
	return snapshot, nil
}

func resolveRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".meshclaw")
	}
	return filepath.Clean(root), nil
}

func skillTitle(name string) string {
	name = strings.TrimSpace(strings.Join(strings.Fields(name), " "))
	name = strings.Trim(name, " .。:：-")
	if name == "" {
		return "Custom Assistant Skill"
	}
	if len([]rune(name)) > 80 {
		runes := []rune(name)
		name = strings.TrimSpace(string(runes[:80]))
	}
	return name
}

func skillSlug(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	lastDash := false
	for _, r := range lower {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if r > 127 {
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteString(fmt.Sprintf("u%x", r))
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "custom-assistant-skill"
	}
	if len(slug) > 64 {
		slug = strings.Trim(slug[:64], "-")
	}
	return slug
}

func skillMarkdown(now time.Time, name, summary string) string {
	return strings.Join([]string{
		"# " + skillTitle(name),
		"",
		strings.TrimSpace(summary),
		"",
		"## Trigger",
		"- Use when the user asks for this repeated assistant routine or names this skill.",
		"",
		"## Safe Behavior",
		"- Summaries, drafts, checklists, and mobile-openable artifacts may be prepared.",
		"- Sending mail, deleting data, purchasing, booking, payments, and account changes require explicit final approval.",
		"- Do not store or reveal raw secrets, full payment details, or private addresses.",
		"- Stop before irreversible browser actions unless a separate final approval record is present.",
		"",
		"## Test",
		"- Prompt: " + skillTestPrompt(name, summary),
		"- Expected: Return a useful draft/result and clearly stop before irreversible action.",
		"",
		"## Review",
		"- Generated by Argos on " + now.UTC().Format("2006-01-02") + ".",
		"- Reviewed skills still run under MeshClaw approval policy.",
		"",
	}, "\n")
}

func skillReviewedMarkdown(now time.Time) string {
	return "reviewed: " + now.UTC().Format(time.RFC3339) + "\npolicy: bounded assistant routine; irreversible actions require final approval\n"
}

func skillActiveMarkdown(now time.Time) string {
	return "active: " + now.UTC().Format(time.RFC3339) + "\nactivation: policy-gated\n"
}

func skillTestMarkdown(name, summary string) string {
	return strings.Join([]string{
		"# Skill Smoke Test",
		"",
		"Prompt:",
		"",
		"> " + skillTestPrompt(name, summary),
		"",
		"Expected:",
		"",
		"- Produces a concrete assistant result.",
		"- Uses approved memory when relevant.",
		"- Stops before sending, deleting, purchasing, booking, or payment.",
		"",
	}, "\n")
}

func skillTestPrompt(name, summary string) string {
	summary = strings.TrimSpace(summary)
	if summary != "" {
		return clipSkillText(summary, 140)
	}
	return skillTitle(name) + " 실행해줘"
}

func matchSkill(items []SkillInfo, query string) (SkillInfo, bool) {
	needle := normalizeSkillMatch(query)
	if needle == "" {
		return SkillInfo{}, false
	}
	for _, item := range items {
		candidates := []string{item.Name, item.Title}
		for _, candidate := range candidates {
			if normalizeSkillMatch(candidate) == needle {
				return item, true
			}
		}
	}
	for _, item := range items {
		candidates := []string{item.Name, item.Title, item.Summary}
		for _, candidate := range candidates {
			norm := normalizeSkillMatch(candidate)
			if norm != "" && (strings.Contains(norm, needle) || strings.Contains(needle, norm)) {
				return item, true
			}
		}
	}
	return SkillInfo{}, false
}

func normalizeSkillMatch(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	text = strings.NewReplacer(" ", "", "\t", "", "\n", "", "-", "", "_", "", ".", "", ":", "", "：", "").Replace(text)
	return text
}

func parseSkillTestMarkdown(text string) (string, []string) {
	lines := strings.Split(text, "\n")
	prompt := ""
	expected := []string{}
	section := ""
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		lower := strings.ToLower(strings.Trim(line, " #：:"))
		switch lower {
		case "prompt":
			section = "prompt"
			continue
		case "expected":
			section = "expected"
			continue
		}
		if line == "" {
			continue
		}
		if section == "prompt" && prompt == "" {
			prompt = strings.TrimSpace(strings.TrimPrefix(line, ">"))
			continue
		}
		if section == "expected" && strings.HasPrefix(line, "-") {
			item := strings.TrimSpace(strings.TrimPrefix(line, "-"))
			if item != "" {
				expected = append(expected, item)
			}
		}
	}
	return prompt, expected
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func fileInfo(name, path string) FileInfo {
	info := FileInfo{Name: name, Path: path, Status: "missing"}
	st, err := os.Stat(path)
	if err != nil {
		return info
	}
	info.Exists = true
	info.SizeBytes = st.Size()
	info.Status = "ready"
	if st.Size() == 0 {
		info.Status = "empty"
	}
	return info
}

func listSkills(root string, limit int) []SkillInfo {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	items := []SkillInfo{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		source := "unknown"
		status := "review_required"
		title := ""
		summary := ""
		review := "unreviewed"
		activation := "approval_required"
		var updatedAt time.Time
		switch {
		case exists(filepath.Join(dir, "SKILL.md")):
			skillPath := filepath.Join(dir, "SKILL.md")
			source = "skill_md"
			title, summary = skillMarkdownSummary(skillPath)
			updatedAt = fileModTime(skillPath)
			if exists(filepath.Join(dir, "REVIEWED.md")) {
				status = "reviewed_approval_required"
				review = "reviewed"
			}
			if exists(filepath.Join(dir, "ACTIVE.md")) && review == "reviewed" {
				status = "active_policy_gated"
				activation = "active_policy_gated"
			}
		case exists(filepath.Join(dir, "skill.json")):
			source = "legacy_skill_json"
			status = "legacy_read_only"
			review = "legacy_read_only"
			activation = "disabled"
			updatedAt = fileModTime(filepath.Join(dir, "skill.json"))
		default:
			source = "directory"
		}
		items = append(items, SkillInfo{
			Name:       entry.Name(),
			Path:       dir,
			Source:     source,
			Status:     status,
			Title:      title,
			Summary:    summary,
			Review:     review,
			Activation: activation,
			UpdatedAt:  updatedAt,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

func skillMarkdownSummary(path string) (string, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	title := ""
	summary := ""
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "<") || strings.HasPrefix(line, "---") {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if title == "" {
				title = strings.TrimSpace(strings.TrimLeft(line, "#"))
			}
			continue
		}
		if summary == "" {
			summary = line
			break
		}
	}
	return clipSkillText(title, 96), clipSkillText(summary, 180)
}

func clipSkillText(text string, max int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if max <= 0 || len([]rune(text)) <= max {
		return text
	}
	runes := []rune(text)
	if max <= 3 {
		return string(runes[:max])
	}
	return strings.TrimSpace(string(runes[:max-3])) + "..."
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func fileModTime(path string) time.Time {
	st, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return st.ModTime().UTC()
}

func normalizeMemoryScope(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "user", "profile", "preference", "preferences", "사용자":
		return "user"
	default:
		return "assistant"
	}
}

func memoryPath(root, scope string) string {
	if normalizeMemoryScope(scope) == "user" {
		return filepath.Join(root, "memory", "USER.md")
	}
	return filepath.Join(root, "memory", "MEMORY.md")
}

func singleLineMemory(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func defaultMemoryHeader(scope string) string {
	if normalizeMemoryScope(scope) == "user" {
		return defaultMemoryUser()
	}
	return defaultMemory()
}

func tailString(text string, maxBytes int64) string {
	text = strings.TrimSpace(text)
	if maxBytes <= 0 || int64(len(text)) <= maxBytes {
		return text
	}
	runes := []rune(text)
	for len(runes) > 0 && int64(len(string(runes))) > maxBytes {
		runes = runes[1:]
	}
	return strings.TrimSpace(string(runes))
}

func wantsAny(text string, needles ...string) bool {
	if strings.TrimSpace(text) == "" {
		for _, needle := range needles {
			if needle == "" {
				return true
			}
		}
		return false
	}
	for _, needle := range needles {
		if needle != "" && strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func countMemoryItems(text string) int {
	count := 0
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") {
			count++
		}
	}
	return count
}

func proceduralStatus(skills []SkillInfo) string {
	if len(skills) == 0 {
		return "empty"
	}
	for _, skill := range skills {
		if skill.Source == "skill_md" {
			return "available_review_required"
		}
	}
	return "legacy_read_only"
}

func skillSnapshotSummary(skills []SkillInfo, limit int) string {
	if limit <= 0 {
		limit = 4
	}
	parts := []string{}
	for _, skill := range skills {
		label := skill.Name
		if skill.Title != "" {
			label = skill.Title
		}
		if skill.Summary != "" {
			label += ": " + skill.Summary
		}
		parts = append(parts, clipSkillText(label, 120))
		if len(parts) >= limit {
			break
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "; ")
}

func defaultSoul() string {
	return "# Argos SOUL\n\nArgos is the Signal-facing personal assistant identity for MeshClaw.\n\n- Speak Korean naturally by default.\n- Explain that the Mac mini is the user-facing Signal runtime.\n- Explain that MeshClaw handles tools, records, policy, and approvals.\n- Prefer concise answers that are useful inside Signal without opening attachments.\n"
}

func defaultAssistantUser() string {
	return "# Argos USER\n\nThis file is for approved user preferences only.\n\nDo not store raw secrets here. Use Guard/Vault handles for sensitive values.\n"
}

func defaultAgents() string {
	return "# Argos AGENTS\n\nOperational rules:\n\n- Interactive replies are allowed only in assistant/chat/Guard targets.\n- Briefing, report, and ops targets are one-way unless explicitly reconfigured.\n- Read-only personal assistant tasks may proceed through narrow tools.\n- Sending, deleting, purchasing, booking, account changes, and destructive operations require explicit approval.\n- Prefer MCP tools and durable artifacts over hidden UI state.\n"
}

func defaultMemoryUser() string {
	return "# User Memory\n\nApproved stable user facts can be summarized here.\n\nCurrent state: no approved memory entries.\n"
}

func defaultMemory() string {
	return "# Assistant Memory\n\nApproved assistant notes can be summarized here.\n\nCurrent state: no approved memory entries.\n"
}
