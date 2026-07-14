package guard

import (
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

type Mode string

const (
	ModeCredential Mode = "credential"
	ModePosture    Mode = "posture"
	ModeVuln       Mode = "vuln"
)

type ModeInfo struct {
	Mode        Mode     `json:"mode"`
	Purpose     string   `json:"purpose"`
	WhatItDoes  []string `json:"what_it_does"`
	SafetyRules []string `json:"safety_rules"`
}

type Finding struct {
	Mode        Mode   `json:"mode"`
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	Severity    string `json:"severity"`
	Target      string `json:"target"`
	Evidence    string `json:"evidence"`
	Replacement string `json:"replacement,omitempty"`
}

type Action struct {
	ID       string `json:"id"`
	Mode     string `json:"mode"`
	Target   string `json:"target"`
	Reason   string `json:"reason"`
	Next     string `json:"next"`
	Approval bool   `json:"approval_required"`
}

type Report struct {
	Mode      Mode      `json:"mode"`
	Target    string    `json:"target"`
	Status    string    `json:"status"`
	Redacted  string    `json:"redacted,omitempty"`
	Findings  []Finding `json:"findings"`
	Actions   []Action  `json:"actions"`
	Principle string    `json:"principle"`
}

type PostureCheck struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Severity string `json:"severity"`
	Evidence string `json:"evidence"`
	Next     string `json:"next"`
}

type PostureReport struct {
	Mode      Mode           `json:"mode"`
	Target    string         `json:"target"`
	Status    string         `json:"status"`
	Checks    []PostureCheck `json:"checks"`
	Principle string         `json:"principle"`
}

type CleanupCandidate struct {
	ClientID        string `json:"client_id"`
	Path            string `json:"path"`
	Exists          bool   `json:"exists"`
	Kind            string `json:"kind"`
	Risk            string `json:"risk"`
	Approval        string `json:"approval"`
	RecommendedNext string `json:"recommended_next"`
}

type CleanupPlan struct {
	Mode       Mode               `json:"mode"`
	Target     string             `json:"target"`
	Status     string             `json:"status"`
	Candidates []CleanupCandidate `json:"candidates"`
	Principle  string             `json:"principle"`
}

type LocalModelPlan struct {
	Name             string   `json:"name"`
	Purpose          string   `json:"purpose"`
	RecommendedSize  string   `json:"recommended_size"`
	RecommendedUIs   []string `json:"recommended_uis"`
	RequiredSettings []string `json:"required_settings"`
	ModelMayDo       []string `json:"model_may_do"`
	ModelMustNotDo   []string `json:"model_must_not_do"`
	GuardAuthority   []string `json:"guard_authority"`
	StartupPrompt    string   `json:"startup_prompt"`
}

type VaultPlan struct {
	Purpose        string   `json:"purpose"`
	LocalOnly      bool     `json:"local_only"`
	HandleScheme   string   `json:"handle_scheme"`
	ConversationUI string   `json:"conversation_ui"`
	Operations     []Action `json:"operations"`
	ModelRules     []string `json:"model_rules"`
	StorageRules   []string `json:"storage_rules"`
}

type ClientGuide struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Verdict          string   `json:"verdict"`
	Risk             string   `json:"risk"`
	RecommendedUse   string   `json:"recommended_use"`
	RequiredSettings []string `json:"required_settings"`
	LocalStores      []string `json:"local_stores,omitempty"`
	Notes            []string `json:"notes,omitempty"`
}

type Intent struct {
	Input              string   `json:"input"`
	Intent             string   `json:"intent"`
	Mode               Mode     `json:"mode"`
	Confidence         string   `json:"confidence"`
	RequiresLocalOnly  bool     `json:"requires_local_only"`
	ApprovalRequired   bool     `json:"approval_required"`
	RecommendedCommand string   `json:"recommended_command,omitempty"`
	Arguments          []string `json:"arguments,omitempty"`
	Warnings           []string `json:"warnings,omitempty"`
	Principle          string   `json:"principle"`
}

type secretPattern struct {
	kind  string
	label string
	rx    *regexp.Regexp
}

var patterns = []secretPattern{
	{kind: "private_key", label: "private key block", rx: regexp.MustCompile(`-----BEGIN (?:RSA |OPENSSH |EC |DSA |)?PRIVATE KEY-----[\s\S]*?-----END (?:RSA |OPENSSH |EC |DSA |)?PRIVATE KEY-----`)},
	{kind: "github_token", label: "GitHub token", rx: regexp.MustCompile(`\b(?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9_]{20,}\b`)},
	{kind: "anthropic_key", label: "Anthropic key", rx: regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{20,}\b`)},
	{kind: "openai_key", label: "OpenAI key", rx: regexp.MustCompile(`\bsk-(?:proj-)?[A-Za-z0-9_-]{32,}\b`)},
	{kind: "aws_access_key", label: "AWS access key id", rx: regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`)},
	{kind: "stripe_key", label: "Stripe key", rx: regexp.MustCompile(`\b(?:sk|rk)_(?:live|test)_[A-Za-z0-9]{16,}\b`)},
	{kind: "jwt", label: "JWT", rx: regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`)},
	{kind: "labeled_secret", label: "labeled credential", rx: regexp.MustCompile(`(?i)\b(?:api[_-]?key|secret|token|password|passwd|pwd|client[_-]?secret)\s*[:=]\s*['"]?[^'"\s]{8,}`)},
}

func Modes() []ModeInfo {
	return []ModeInfo{
		{
			Mode:       ModeCredential,
			Purpose:    "Protect secrets, credentials, and capability handles before AI operators see or use them.",
			WhatItDoes: []string{"detect pasted secrets", "redact text", "turn raw values into use-only handles", "route secret-adjacent actions through approval"},
			SafetyRules: []string{
				"raw secret values are never returned to models",
				"evidence stores redacted samples only",
				"secret use is a capability lease, not a reveal",
			},
		},
		{
			Mode:       ModePosture,
			Purpose:    "Check whether the local operator environment is safe for AI-assisted work.",
			WhatItDoes: []string{"check policy and evidence paths", "flag risky chat-memory stores", "verify local state permissions"},
			SafetyRules: []string{
				"posture checks are read-only",
				"local chat history and RAG stores are treated as sensitive",
			},
		},
		{
			Mode:       ModeVuln,
			Purpose:    "Find code, log, and config hygiene risks, plus known package CVEs, without exposing raw values.",
			WhatItDoes: []string{"scan local text or files for leaked credentials", "scan fleet node packages (deb/PyPI/npm/brew/cargo) against the OSV.dev CVE database", "return redacted findings", "suggest bounded cleanup or approval-gated rotation"},
			SafetyRules: []string{
				"scan output is redacted by default",
				"package CVE scans are read-only; only names and versions leave the host",
				"rotation, patching, and deletion require approval",
			},
		},
	}
}

func LocalModel() LocalModelPlan {
	return LocalModelPlan{
		Name:            "MeshClaw Guard Chat",
		Purpose:         "Optional local-only conversation layer for explaining credential, posture, and vulnerability findings to the user.",
		RecommendedSize: "Small local instruct model is enough; prefer a private Ollama, LM Studio, or Open WebUI endpoint for sensitive conversations.",
		RecommendedUIs: []string{
			"Open WebUI with memory and RAG disabled",
			"LM Studio local chat with history cleanup",
			"Ollama-compatible local frontend with no cloud sync",
		},
		RequiredSettings: []string{
			"disable persistent memory before sensitive conversations",
			"disable RAG ingestion and workspace indexing for raw secrets",
			"disable cloud sync and telemetry where the frontend allows it",
			"delete local chat history after credential work",
			"run meshclaw guard-posture before and after sensitive sessions",
		},
		ModelMayDo: []string{
			"explain redacted Guard findings in Korean or English",
			"help the user decide whether a pasted value should be rotated",
			"turn Guard actions into a human cleanup checklist",
			"guide the user through safe password-manager or provider-console steps",
		},
		ModelMustNotDo: []string{
			"store or repeat raw passwords, API keys, private keys, or tokens",
			"decide that a raw value is safe when Guard marked it sensitive",
			"perform rotation, deletion, account creation, or DNS changes without MeshClaw policy approval",
			"write raw secret values into evidence, logs, RAG stores, or long-term memory",
		},
		GuardAuthority: []string{
			"Guard scanner decides what is sensitive",
			"MeshClaw policy decides whether an action is allowed, approval-required, or denied",
			"evidence writer stores redacted samples only",
			"vault/provider adapters handle actual secret storage and use-only injection",
		},
		StartupPrompt: "You are MeshClaw Guard Chat. Before sensitive work, confirm memory, RAG, cloud sync, and chat-history persistence are disabled. Never ask the user to reveal raw secrets unless Guard is running locally and the value will be redacted or moved to a vault handle. Explain Guard findings, but do not override Guard policy.",
	}
}

func VaultConversationPlan() VaultPlan {
	return VaultPlan{
		Purpose:        "Let the user manage passwords, tokens, and keys through a local-only Guard Chat while cloud AI operators see only vault handles and redacted metadata.",
		LocalOnly:      true,
		HandleScheme:   "vault://meshclaw/<scope>/<name>",
		ConversationUI: "MeshClaw Guard Chat backed by a local model with memory, RAG, cloud sync, and history persistence disabled.",
		Operations: []Action{
			{ID: "vault-import", Mode: "local_only", Target: "secret", Reason: "Raw values may be typed only into the local Guard boundary.", Next: "Store the secret and return a vault:// handle plus redacted evidence.", Approval: false},
			{ID: "vault-metadata", Mode: "safe_plan", Target: "secret", Reason: "Cloud AI operators need names, scopes, expiration, and allowed uses, not raw values.", Next: "Return handle metadata and policy, never the secret value.", Approval: false},
			{ID: "vault-use", Mode: "requires_approval", Target: "adapter", Reason: "Using a secret can mutate external systems even when the value is hidden.", Next: "Approve a bounded use-only lease for one adapter/workflow step.", Approval: true},
			{ID: "vault-rotate", Mode: "requires_approval", Target: "secret", Reason: "Rotation can break services and must be deliberate.", Next: "Plan provider-side rotation, update the vault handle, and attach redacted evidence.", Approval: true},
			{ID: "vault-delete", Mode: "requires_strong_approval", Target: "secret", Reason: "Deleting the only copy of a credential is destructive.", Next: "Confirm backup/recovery path before deletion.", Approval: true},
		},
		ModelRules: []string{
			"the local model may explain vault operations but must not repeat raw values",
			"Codex, Claude, Cursor, and remote models receive only handles and redacted findings",
			"the local model cannot override Guard scanner or MeshClaw policy decisions",
		},
		StorageRules: []string{
			"prefer OS keychain, pass, age-encrypted files, or an existing password manager adapter",
			"store evidence separately from raw secret storage",
			"delete local chat history after credential sessions",
			"never write raw values into workflow evidence, logs, or RAG indexes",
		},
	}
}

func ClientGuides() []ClientGuide {
	return []ClientGuide{
		{
			ID:             "ollama-cli",
			Name:           "Ollama CLI",
			Verdict:        "safest local chat surface",
			Risk:           "low",
			RecommendedUse: "Daily local Guard conversations when no persistent chat UI is needed.",
			RequiredSettings: []string{
				"use local models only",
				"avoid shell history for raw secrets; prefer Guard vault import",
				"run guard-detect before copying redacted context into cloud AI clients",
			},
			Notes: []string{"Ollama CLI is generally stateless, but terminal scrollback is still visible to the local user."},
		},
		{
			ID:             "open-webui",
			Name:           "Open WebUI",
			Verdict:        "usable only with memory, RAG, and persistence controlled",
			Risk:           "medium",
			RecommendedUse: "Local Guard Chat UI for explanations and checklists after disabling memory and document ingestion.",
			RequiredSettings: []string{
				"disable memory for sensitive chats",
				"disable RAG, document ingestion, and workspace indexing for raw secrets",
				"disable chat history where possible or delete sensitive conversations immediately",
				"point model traffic only at local Ollama/vLLM endpoints for secret-adjacent work",
			},
			LocalStores: []string{
				"open-webui/data/webui.db",
				".open-webui/data/webui.db",
				"Library/Application Support/open-webui/webui.db",
			},
			Notes: []string{"Cloud AI operators should receive vault handles and redacted findings only."},
		},
		{
			ID:             "lm-studio",
			Name:           "LM Studio",
			Verdict:        "convenient local UI, but chat files may persist on disk",
			Risk:           "medium",
			RecommendedUse: "Local Guard explanations with explicit cleanup after credential work.",
			RequiredSettings: []string{
				"use a local inference server endpoint",
				"disable conversation history if the UI exposes the option",
				"delete local chat history after credential sessions",
			},
			LocalStores: []string{
				".cache/lm-studio/conversations",
				".lmstudio",
				"Library/Application Support/LM Studio",
			},
		},
		{
			ID:             "enchanted",
			Name:           "Enchanted",
			Verdict:        "lightweight local Ollama frontend; verify conversation persistence settings",
			Risk:           "low",
			RecommendedUse: "Simple local Guard Chat when configured against local Ollama.",
			RequiredSettings: []string{
				"use local Ollama URL",
				"disable saved conversations if available",
				"start a fresh conversation for sensitive work",
			},
			LocalStores: []string{
				"Library/Containers/app.augustinmauroy.Enchanted/Data",
			},
		},
		{
			ID:             "cursor-continue-vscode",
			Name:           "Cursor / Continue / VS Code AI extensions",
			Verdict:        "good for code, risky near secrets because workspace context may be attached",
			Risk:           "medium",
			RecommendedUse: "Code-adjacent Guard scans and redacted summaries, not raw secret discussion.",
			RequiredSettings: []string{
				"disable automatic workspace context when discussing credentials",
				"scan files with guard-vuln before sending excerpts to cloud models",
				"use vault handles for execution and deployment secrets",
			},
			LocalStores: []string{
				".continue",
				".cursor",
				"Library/Application Support/Cursor",
				"Library/Application Support/Code/User",
			},
		},
		{
			ID:             "cloud-ai",
			Name:           "Claude / Codex / ChatGPT cloud clients",
			Verdict:        "never paste raw secrets",
			Risk:           "high",
			RecommendedUse: "Use MeshClaw MCP tools with vault handles, redacted findings, policy checks, and evidence.",
			RequiredSettings: []string{
				"send only vault handles and redacted context",
				"use meshclaw_guard_detect before moving text from local Guard Chat to cloud AI",
				"require approval for secret use, rotation, account creation, DNS changes, and email sends",
			},
			Notes: []string{"These clients are the reasoning layer, not the raw-secret boundary."},
		},
	}
}

func ParseIntent(input string) Intent {
	raw := strings.TrimSpace(input)
	lower := strings.ToLower(raw)
	intent := Intent{
		Input:      raw,
		Intent:     "unknown",
		Mode:       ModeCredential,
		Confidence: "low",
		Principle:  "Local Guard Chat may classify intent, but MeshClaw policy and vault commands execute the action.",
	}
	switch {
	case containsAny(lower, "저장", "import", "store", "save", "add"):
		intent.Intent = "vault_import"
		intent.Confidence = "medium"
		intent.RequiresLocalOnly = true
		intent.RecommendedCommand = "meshclaw guard-vault-put <scope> <name> [kind] [description] --backend <backend> --json"
		intent.Warnings = []string{"raw values must be entered only into the local Guard boundary", "cloud AI clients should see only the resulting vault:// handle"}
	case containsAny(lower, "목록", "list", "ls", "show handles"):
		intent.Intent = "vault_list"
		intent.Confidence = "high"
		intent.RecommendedCommand = "meshclaw guard-vault-list --json"
	case containsAny(lower, "검색", "찾", "metadata", "메타", "정보"):
		intent.Intent = "vault_metadata"
		intent.Confidence = "medium"
		intent.RecommendedCommand = "meshclaw guard-vault-metadata <vault://meshclaw/scope/name> --json"
	case containsAny(lower, "삭제", "delete", "remove"):
		intent.Intent = "vault_delete"
		intent.Confidence = "medium"
		intent.RequiresLocalOnly = true
		intent.ApprovalRequired = true
		intent.RecommendedCommand = "meshclaw guard-vault-delete <scope> <name> --json"
		intent.Warnings = []string{"deleting the only copy of a credential is destructive"}
	case containsAny(lower, "회전", "rotate", "재발급", "reissue"):
		intent.Intent = "vault_rotate_plan"
		intent.Confidence = "medium"
		intent.RequiresLocalOnly = true
		intent.ApprovalRequired = true
		intent.RecommendedCommand = "meshclaw guard-detect chat <redacted context> --json"
		intent.Warnings = []string{"rotation can break services and should be planned before execution"}
	case containsAny(lower, "복사", "clipboard", "clip"):
		intent.Intent = "vault_clip_plan"
		intent.Confidence = "medium"
		intent.RequiresLocalOnly = true
		intent.ApprovalRequired = true
		intent.RecommendedCommand = "not exposed yet; use OS password manager UI or future bounded clipboard lease"
	case containsAny(lower, "점검", "상태", "posture", "안전", "memory", "rag"):
		intent.Intent = "guard_posture"
		intent.Mode = ModePosture
		intent.Confidence = "high"
		intent.RecommendedCommand = "meshclaw guard-posture --json"
	case containsAny(lower, "스캔", "scan", "leak", "유출", "vuln", "취약"):
		intent.Intent = "guard_vuln"
		intent.Mode = ModeVuln
		intent.Confidence = "high"
		intent.RecommendedCommand = "meshclaw guard-vuln <path> --json"
	}
	if strings.Contains(lower, "password") || strings.Contains(lower, "token") || strings.Contains(lower, "비번") || strings.Contains(lower, "비밀번호") {
		intent.RequiresLocalOnly = true
	}
	return intent
}

func Detect(target, text string) Report {
	text = strings.TrimSpace(text)
	findings, redacted := detectAndRedact(target, text)
	actions := actionsForFindings(findings)
	status := "clean"
	if len(findings) > 0 {
		status = "findings"
	}
	return Report{
		Mode:      ModeCredential,
		Target:    defaultTarget(target),
		Status:    status,
		Redacted:  redacted,
		Findings:  findings,
		Actions:   actions,
		Principle: "Models may receive redacted context and handles, never raw secret values.",
	}
}

func Redact(target, text string) Report {
	report := Detect(target, text)
	report.Mode = ModeCredential
	return report
}

func Posture(home string) PostureReport {
	if strings.TrimSpace(home) == "" {
		if h, err := os.UserHomeDir(); err == nil {
			home = h
		}
	}
	checks := []PostureCheck{
		pathPermissionCheck("meshclaw-state", filepath.Join(home, ".meshclaw"), true),
		pathPermissionCheck("meshclaw-evidence", filepath.Join(home, ".meshclaw", "evidence"), false),
	}
	for _, guide := range ClientGuides() {
		for _, store := range guide.LocalStores {
			checks = append(checks, chatStoreCheck(guide.ID, home, store))
		}
	}
	status := "ok"
	for _, check := range checks {
		if check.Severity == "high" || check.Severity == "medium" {
			status = "attention"
			break
		}
	}
	return PostureReport{
		Mode:      ModePosture,
		Target:    home,
		Status:    status,
		Checks:    checks,
		Principle: "Local model memory, RAG, and chat history are part of the trust boundary and must be cleaned or disabled for sensitive work.",
	}
}

func Cleanup(home string) CleanupPlan {
	if strings.TrimSpace(home) == "" {
		if h, err := os.UserHomeDir(); err == nil {
			home = h
		}
	}
	var candidates []CleanupCandidate
	for _, guide := range ClientGuides() {
		for _, store := range guide.LocalStores {
			path := store
			if !filepath.IsAbs(path) {
				path = filepath.Join(home, store)
			}
			info, err := os.Stat(path)
			exists := err == nil
			kind := "missing"
			if exists {
				kind = "file"
				if info.IsDir() {
					kind = "directory"
				}
			}
			next := "No local store found at this path."
			approval := "none"
			if exists {
				approval = "require_strong_approval"
				next = "After exporting anything needed, delete this client history/cache through the app UI or a future approved guard-clean-apply command."
			}
			candidates = append(candidates, CleanupCandidate{
				ClientID:        guide.ID,
				Path:            path,
				Exists:          exists,
				Kind:            kind,
				Risk:            guide.Risk,
				Approval:        approval,
				RecommendedNext: next,
			})
		}
	}
	status := "clean"
	for _, candidate := range candidates {
		if candidate.Exists {
			status = "attention"
			break
		}
	}
	return CleanupPlan{
		Mode:       ModePosture,
		Target:     home,
		Status:     status,
		Candidates: candidates,
		Principle:  "Guard cleanup is planned before deletion. Chat history, memory, and RAG stores can contain raw secrets and require explicit local approval before removal.",
	}
}

func chatStoreCheck(clientID, home, rel string) PostureCheck {
	path := rel
	if !filepath.IsAbs(path) {
		path = filepath.Join(home, rel)
	}
	check := pathPermissionCheck("chat-store-"+clientID+"-"+sanitizeID(rel), path, false)
	if check.Status == "ok" {
		check.Status = "present"
		check.Severity = "medium"
		check.Next = "This local chat/context store exists. Disable memory/RAG/history for sensitive work and delete related conversations after credential sessions."
	}
	return check
}

func VulnScanPath(path string) Report {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "."
	}
	info, err := os.Stat(path)
	if err != nil {
		return Report{Mode: ModeVuln, Target: path, Status: "failed", Findings: []Finding{{
			Mode:     ModeVuln,
			Kind:     "path_error",
			Label:    "path error",
			Severity: "error",
			Target:   path,
			Evidence: err.Error(),
		}}}
	}
	if !info.IsDir() {
		data, err := os.ReadFile(path)
		if err != nil {
			return Report{Mode: ModeVuln, Target: path, Status: "failed", Findings: []Finding{{
				Mode:     ModeVuln,
				Kind:     "read_error",
				Label:    "read error",
				Severity: "error",
				Target:   path,
				Evidence: err.Error(),
			}}}
		}
		report := Detect(path, string(data))
		report.Mode = ModeVuln
		return report
	}

	var all []Finding
	seen := 0
	_ = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil || seen >= 500 {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "__pycache__" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !interestingFile(p) {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > 2*1024*1024 {
			return nil
		}
		seen++
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		findings, _ := detectAndRedact(p, string(data))
		for _, finding := range findings {
			finding.Mode = ModeVuln
			all = append(all, finding)
		}
		return nil
	})
	sort.Slice(all, func(i, j int) bool {
		if all[i].Severity == all[j].Severity {
			return all[i].Target < all[j].Target
		}
		return severityRank(all[i].Severity) > severityRank(all[j].Severity)
	})
	if len(all) > 100 {
		all = all[:100]
	}
	status := "clean"
	if len(all) > 0 {
		status = "findings"
	}
	return Report{
		Mode:      ModeVuln,
		Target:    path,
		Status:    status,
		Findings:  all,
		Actions:   actionsForFindings(all),
		Principle: "Vulnerability and leak scans return redacted evidence only; cleanup and rotation stay approval-gated.",
	}
}

func detectAndRedact(target, text string) ([]Finding, string) {
	target = defaultTarget(target)
	redacted := text
	var findings []Finding
	counter := 0
	for _, pattern := range patterns {
		matches := pattern.rx.FindAllString(redacted, -1)
		for _, match := range matches {
			if strings.Contains(match, "<SECRET:") {
				continue
			}
			counter++
			replacement := "<SECRET:" + pattern.kind + ":" + itoa(counter) + ">"
			findings = append(findings, Finding{
				Mode:        ModeCredential,
				Kind:        pattern.kind,
				Label:       pattern.label,
				Severity:    severityForKind(pattern.kind),
				Target:      target,
				Evidence:    mask(match),
				Replacement: replacement,
			})
			redacted = strings.Replace(redacted, match, replacement, 1)
		}
	}
	for _, token := range highEntropyCandidates(redacted) {
		counter++
		replacement := "<SECRET:high_entropy:" + itoa(counter) + ">"
		findings = append(findings, Finding{
			Mode:        ModeCredential,
			Kind:        "high_entropy",
			Label:       "high entropy token",
			Severity:    "medium",
			Target:      target,
			Evidence:    mask(token),
			Replacement: replacement,
		})
		redacted = strings.Replace(redacted, token, replacement, 1)
	}
	return findings, redacted
}

func highEntropyCandidates(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune("\"'`,;()[]{}", r)
	})
	var out []string
	for _, field := range fields {
		field = strings.Trim(field, ":=")
		if len(field) < 32 || len(field) > 240 {
			continue
		}
		if strings.Contains(field, "://") || strings.Contains(field, ".") && strings.Contains(field, "/") {
			continue
		}
		if entropy(field) >= 3.8 {
			out = append(out, field)
		}
	}
	return out
}

func entropy(value string) float64 {
	counts := map[rune]float64{}
	for _, r := range value {
		counts[r]++
	}
	total := float64(len([]rune(value)))
	var e float64
	for _, count := range counts {
		p := count / total
		e -= p * math.Log2(p)
	}
	return e
}

func actionsForFindings(findings []Finding) []Action {
	if len(findings) == 0 {
		return nil
	}
	return []Action{
		{
			ID:       "store-as-capability-handle",
			Mode:     "safe_plan",
			Target:   "credential",
			Reason:   "AI operators should refer to secrets by handle, not raw value.",
			Next:     "Move the raw value into a local vault or provider credential store, then continue with a vault:// handle.",
			Approval: false,
		},
		{
			ID:       "rotate-if-exposed",
			Mode:     "requires_approval",
			Target:   "credential",
			Reason:   "If the raw value reached a chat log, RAG store, browser history, or remote model, rotation can break services and requires operator approval.",
			Next:     "Confirm exposure scope, rotate the provider token, and attach redacted evidence.",
			Approval: true,
		},
	}
}

func pathPermissionCheck(id, path string, required bool) PostureCheck {
	info, err := os.Stat(path)
	if err != nil {
		if required {
			return PostureCheck{ID: id, Status: "missing", Severity: "medium", Evidence: path, Next: "Create the path with owner-only permissions before sensitive work."}
		}
		return PostureCheck{ID: id, Status: "not_found", Severity: "info", Evidence: path, Next: "No local store found at this path."}
	}
	mode := info.Mode().Perm()
	if mode&0o077 != 0 {
		return PostureCheck{ID: id, Status: "too_open", Severity: "medium", Evidence: path + " mode=" + mode.String(), Next: "Restrict permissions before storing local model chats, evidence, or credentials."}
	}
	return PostureCheck{ID: id, Status: "ok", Severity: "info", Evidence: path + " mode=" + mode.String(), Next: "No action needed."}
}

func interestingFile(path string) bool {
	lower := strings.ToLower(filepath.Base(path))
	if strings.HasPrefix(lower, ".env") || strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "credential") {
		return true
	}
	switch filepath.Ext(lower) {
	case ".env", ".log", ".conf", ".json", ".yaml", ".yml", ".ini", ".toml", ".txt", ".md", ".py", ".go", ".js", ".ts", ".tsx", ".jsx", ".sh":
		return true
	default:
		return false
	}
}

func severityForKind(kind string) string {
	switch kind {
	case "private_key", "github_token", "openai_key", "anthropic_key", "aws_access_key", "stripe_key", "labeled_secret":
		return "high"
	case "jwt":
		return "medium"
	default:
		return "medium"
	}
}

func severityRank(value string) int {
	switch value {
	case "error":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	default:
		return 1
	}
}

func defaultTarget(target string) string {
	if strings.TrimSpace(target) == "" {
		return "input"
	}
	return strings.TrimSpace(target)
}

func mask(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "[REDACTED]"
	}
	if strings.Contains(value, "\n") {
		lines := strings.Split(value, "\n")
		return strings.TrimSpace(lines[0]) + "\n...[REDACTED BLOCK]...\n" + strings.TrimSpace(lines[len(lines)-1])
	}
	if len(value) <= 10 {
		return "[REDACTED]"
	}
	return value[:4] + "...[REDACTED]..." + value[len(value)-4:]
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = byte('0' + value%10)
		value /= 10
	}
	return string(buf[i:])
}

func sanitizeID(value string) string {
	value = strings.ToLower(value)
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if ok {
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
		return "store"
	}
	return out
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}
