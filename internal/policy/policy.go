package policy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Decision string

const (
	Allow           Decision = "allow"
	Deny            Decision = "deny"
	RequireApproval Decision = "require_approval"
)

type Request struct {
	Subject  string `json:"subject"`
	Action   string `json:"action"`
	Resource string `json:"resource"`
	Context  string `json:"context,omitempty"`
}

type Result struct {
	Decision Decision `json:"decision"`
	Reason   string   `json:"reason"`
	Mask     bool     `json:"mask"`
	RuleID   string   `json:"rule_id,omitempty"`
	Source   string   `json:"source,omitempty"`
}

type Config struct {
	Version string `json:"version"`
	Rules   []Rule `json:"rules"`
}

type Rule struct {
	ID       string   `json:"id"`
	Subject  string   `json:"subject,omitempty"`
	Action   string   `json:"action,omitempty"`
	Resource string   `json:"resource,omitempty"`
	Context  string   `json:"context,omitempty"`
	Decision Decision `json:"decision"`
	Reason   string   `json:"reason,omitempty"`
	Mask     bool     `json:"mask,omitempty"`
}

var configCache = struct {
	sync.Mutex
	path  string
	mtime time.Time
	size  int64
	cfg   Config
	ok    bool
}{}

var denied = []string{
	"rm -rf /",
	"mkfs",
	"dd if=",
	":(){",
	"shutdown",
	"poweroff",
	"reboot",
}

func CheckCommand(command string) error {
	result := Evaluate(Request{
		Subject:  "operator",
		Action:   "run_command",
		Resource: "server",
		Context:  command,
	})
	if result.Decision == Deny {
		return ErrDenied{Reason: result.Reason}
	}
	return nil
}

func Evaluate(request Request) Result {
	if result, ok := evaluateConfiguredPolicy(request); ok {
		return result
	}
	return evaluateBuiltIn(request)
}

func evaluateBuiltIn(request Request) Result {
	subject := normalize(request.Subject)
	action := normalize(request.Action)
	resource := normalize(request.Resource)
	context := strings.ToLower(request.Context)

	if subject == "" || action == "" || resource == "" {
		return Result{Decision: Deny, Reason: "subject, action, and resource are required", Source: "builtin"}
	}

	if action == "provider_token_use" {
		return Result{Decision: RequireApproval, Reason: "provider tokens are use-only credentials and require approval-gated access", Mask: true, Source: "builtin"}
	}

	if action == "guard_vault_use" {
		return Result{Decision: RequireApproval, Reason: "local vault use injects a secret into a child process and requires local operator approval", Mask: true, Source: "builtin"}
	}

	if resource == "secret" || resource == "secrets" || action == "reveal_secret" {
		return Result{Decision: Deny, Reason: "secrets are use-only capabilities and cannot be revealed", Mask: true, Source: "builtin"}
	}

	if strings.Contains(resource, "secret") || strings.Contains(resource, "credential") {
		return Result{Decision: RequireApproval, Reason: "secret-adjacent resources require explicit policy approval", Mask: true, Source: "builtin"}
	}

	if action == "read_state" || action == "server_list" || action == "fleet_status" || action == "capability_list" || action == "evidence_list" || action == "evidence_latest" || action == "workflow_list" || action == "guard_vault_list" || action == "guard_vault_metadata" {
		return Result{Decision: Allow, Reason: "read-only operational facts are allowed", Source: "builtin"}
	}

	if action == "workflow_run" {
		return Result{Decision: Allow, Reason: "workflow runner is allowed; each step is evaluated by MeshClaw policy before execution", Source: "builtin"}
	}

	if action == "fleet_scan" {
		return Result{Decision: Allow, Reason: "read-only fleet diagnostics are allowed", Source: "builtin"}
	}

	if action == "read_logs" || action == "read_only_diagnosis" || action == "disk_investigate" || action == "doctor" || action == "security_check" || action == "hygiene_plan" || action == "hygiene_scan_host" || action == "autoheal_plan" || action == "service_check" || action == "service_audit" || action == "service_triage" || action == "fleet_service_audit" {
		return Result{Decision: Allow, Reason: "read-only diagnostic action is allowed", Source: "builtin"}
	}

	if action == "artifact_collect" || action == "job_status" || action == "job_logs" {
		if looksSecretPath(context) {
			return Result{Decision: RequireApproval, Reason: "secret-adjacent artifact path requires explicit approval", Mask: true, Source: "builtin"}
		}
		return Result{Decision: Allow, Reason: "read-only artifact/job evidence action is allowed", Source: "builtin"}
	}

	if action == "job_cancel" {
		return Result{Decision: Allow, Reason: "canceling a MeshClaw/vssh job is bounded remediation", Source: "builtin"}
	}

	if action == "autoheal_apply_safe" {
		return Result{Decision: Allow, Reason: "bounded non-destructive remediation is allowed", Source: "builtin"}
	}

	if action == "autoheal_apply" {
		return Result{Decision: RequireApproval, Reason: "applying remediation changes live server state and requires operator approval plus evidence", Source: "builtin"}
	}

	if action == "provision_plan" || action == "capacity_plan" {
		return Result{Decision: Allow, Reason: "planning does not incur cost", Source: "builtin"}
	}

	if action == "provision_server" || action == "deprovision_server" || action == "provider_revoke" {
		return Result{Decision: RequireApproval, Reason: "provider and cost-changing actions require approval", Source: "builtin"}
	}

	if action == "cloudflare_dns_change" || action == "dns_change" || action == "dns_record_update" {
		return Result{Decision: RequireApproval, Reason: "DNS changes can affect public service availability and require approval", Source: "builtin"}
	}

	if action == "documentation_check" {
		return Result{Decision: Allow, Reason: "official documentation checks are read-only", Source: "builtin"}
	}

	if action == "screenshot_capture" {
		return Result{Decision: RequireApproval, Reason: "screenshots can contain sensitive user, client, or browser data and require redaction-aware approval", Mask: true, Source: "builtin"}
	}

	if action == "email_send" {
		return Result{Decision: RequireApproval, Reason: "sending real email requires approval and evidence", Source: "builtin"}
	}

	if action == "email_delete" {
		return Result{Decision: RequireApproval, Reason: "deleting email requires explicit approval and evidence", Source: "builtin"}
	}

	if action == "email_move" {
		return Result{Decision: RequireApproval, Reason: "moving email changes mailbox state and requires explicit approval plus evidence", Source: "builtin"}
	}

	if action == "email_attachment_download" {
		return Result{Decision: RequireApproval, Reason: "downloading email attachments can store sensitive files locally and requires approval", Mask: true, Source: "builtin"}
	}

	if action == "signal_call" {
		return Result{Decision: RequireApproval, Reason: "placing a real Signal call requires an approved target, explicit operator approval, and call evidence", Source: "builtin"}
	}

	if action == "signal_message_send" || action == "messenger_send" {
		return Result{Decision: Allow, Reason: "sending a redacted message to a configured messenger target is allowed", Source: "builtin"}
	}

	if action == "account_create" || action == "account_configure" || action == "account_update" {
		return Result{Decision: RequireApproval, Reason: "account changes require approval and audit evidence", Source: "builtin"}
	}

	if action == "account_delete" {
		return Result{Decision: RequireApproval, Reason: "account deletion requires strong operator approval", Source: "builtin"}
	}

	if action == "restart_service" || action == "service_quarantine" || action == "service_remove" || action == "container_restart" || action == "container_recreate" || action == "container_pull_redeploy" {
		return Result{Decision: RequireApproval, Reason: "service/container mutation requires approval and post-action evidence", Source: "builtin"}
	}

	if action == "data_clean_apply" || action == "delete_data" {
		return Result{Decision: RequireApproval, Reason: "data deletion requires approval, manifest evidence, and post-cleanup verification", Source: "builtin"}
	}

	if action == "rotate_secret" || action == "edit_database" {
		return Result{Decision: RequireApproval, Reason: "mutating infrastructure action requires approval", Source: "builtin"}
	}

	if action == "run_command" || action == "fleet_exec" {
		if containsDeniedCommand(context) {
			return Result{Decision: Deny, Reason: "blocked risky command pattern", Source: "builtin"}
		}
		if looksMutating(context) {
			return Result{Decision: RequireApproval, Reason: "raw mutating command requires approval", Source: "builtin"}
		}
		return Result{Decision: Allow, Reason: "read-only or low-risk command is allowed", Source: "builtin"}
	}

	if subject == "local-llm" || subject == "openwebui" {
		return Result{Decision: RequireApproval, Reason: "unknown local-model action requires policy approval", Source: "builtin"}
	}

	return Result{Decision: RequireApproval, Reason: "unknown action requires policy approval", Source: "builtin"}
}

func LoadConfig() (Config, string, error) {
	path := strings.TrimSpace(os.Getenv("MESHCLAW_POLICY_FILE"))
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Config{}, "", err
		}
		path = filepath.Join(home, ".meshclaw", "policy.json")
	}

	info, statErr := os.Stat(path)
	configCache.Lock()
	if configCache.ok && configCache.path == path {
		if statErr != nil && os.IsNotExist(statErr) && configCache.size < 0 {
			cfg := configCache.cfg
			configCache.Unlock()
			return cfg, path, nil
		}
		if statErr == nil && configCache.size == info.Size() && configCache.mtime.Equal(info.ModTime()) {
			cfg := configCache.cfg
			configCache.Unlock()
			return cfg, path, nil
		}
	}
	configCache.Unlock()

	if statErr != nil {
		if os.IsNotExist(statErr) {
			cfg := DefaultConfig()
			configCache.Lock()
			configCache.path = path
			configCache.mtime = time.Time{}
			configCache.size = -1
			configCache.cfg = cfg
			configCache.ok = true
			configCache.Unlock()
			return cfg, path, nil
		}
		return Config{}, path, statErr
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, path, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, path, err
	}
	configCache.Lock()
	configCache.path = path
	configCache.mtime = info.ModTime()
	configCache.size = info.Size()
	configCache.cfg = cfg
	configCache.ok = true
	configCache.Unlock()
	return cfg, path, nil
}

func DefaultConfig() Config {
	return PresetConfig("devops")
}

func PresetNames() []string {
	return []string{"devops", "strict"}
}

func PresetConfig(name string) Config {
	switch normalize(name) {
	case "", "devops":
		return devopsPreset()
	case "strict":
		return strictPreset()
	default:
		return devopsPreset()
	}
}

func devopsPreset() Config {
	rules := []Rule{
		{
			ID:       "deny-secret-reveal",
			Action:   "reveal_secret",
			Resource: "*",
			Decision: Deny,
			Reason:   "secrets are use-only capabilities and cannot be revealed",
			Mask:     true,
		},
		{
			ID:       "deny-shadow-read",
			Action:   "run_command",
			Resource: "server",
			Context:  "/etc/shadow",
			Decision: Deny,
			Reason:   "credential material must not be read",
			Mask:     true,
		},
	}
	readActions := []string{
		"server_list",
		"read_state",
		"fleet_status",
		"capability_list",
		"monitor_check",
		"fleet_scan",
		"evidence_list",
		"evidence_latest",
		"workflow_list",
		"workflow_run",
		"policy_check",
		"policy_show",
		"doctor",
		"analyze_logs",
		"security_check",
		"hygiene_scan_host",
		"disk_investigate",
		"data_clean_plan",
		"provision_plan",
		"job_status",
		"job_logs",
		"artifact_collect",
		"guard_vault_list",
		"guard_vault_metadata",
	}
	for _, subject := range []string{"codex", "claude", "chatgpt", "local-llm", "openwebui"} {
		for _, action := range readActions {
			rules = append(rules, Rule{
				ID:       subject + "-allow-" + action,
				Subject:  subject,
				Action:   action,
				Resource: "*",
				Decision: Allow,
				Reason:   "AI operator may inspect server state and evidence through MeshClaw",
			})
		}
	}
	for _, subject := range []string{"local-llm", "openwebui"} {
		rules = append(rules, Rule{
			ID:       subject + "-approval-gate",
			Subject:  subject,
			Action:   "*",
			Resource: "*",
			Decision: RequireApproval,
			Reason:   "local model mutating or unknown actions require explicit approval",
		})
	}
	return Config{Version: "1", Rules: rules}
}

func strictPreset() Config {
	return Config{
		Version: "1",
		Rules: []Rule{
			{
				ID:       "deny-secret-reveal",
				Action:   "reveal_secret",
				Resource: "*",
				Decision: Deny,
				Reason:   "secrets are use-only capabilities and cannot be revealed",
				Mask:     true,
			},
			{
				ID:       "local-llm-approval-gate",
				Subject:  "local-llm",
				Action:   "*",
				Resource: "*",
				Decision: RequireApproval,
				Reason:   "local model actions require explicit policy approval unless a narrower allow rule is added",
			},
		},
	}
}

func evaluateConfiguredPolicy(request Request) (Result, bool) {
	cfg, _, err := LoadConfig()
	if err != nil {
		return Result{}, false
	}
	for _, rule := range cfg.Rules {
		if !rule.matches(request) || !validDecision(rule.Decision) {
			continue
		}
		reason := rule.Reason
		if reason == "" {
			reason = "matched configured policy rule"
		}
		return Result{
			Decision: rule.Decision,
			Reason:   reason,
			Mask:     rule.Mask,
			RuleID:   rule.ID,
			Source:   "config",
		}, true
	}
	return Result{}, false
}

func (r Rule) matches(request Request) bool {
	return matchField(r.Subject, request.Subject) &&
		matchField(r.Action, request.Action) &&
		matchField(r.Resource, request.Resource) &&
		matchContext(r.Context, request.Context)
}

func matchField(pattern, value string) bool {
	pattern = normalize(pattern)
	if pattern == "" || pattern == "*" {
		return true
	}
	return pattern == normalize(value)
}

func matchContext(pattern, value string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if pattern == "" || pattern == "*" {
		return true
	}
	return strings.Contains(strings.ToLower(value), pattern)
}

func validDecision(decision Decision) bool {
	return decision == Allow || decision == Deny || decision == RequireApproval
}

func looksSecretPath(value string) bool {
	sensitive := []string{
		".env",
		"id_rsa",
		"id_ed25519",
		"secret",
		"credential",
		"password",
		"token",
		"apikey",
		"api_key",
	}
	lower := strings.ToLower(value)
	for _, token := range sensitive {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

func normalize(value string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "-", "_")
}

func containsDeniedCommand(command string) bool {
	lower := strings.ToLower(command)
	for _, token := range denied {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

func looksMutating(command string) bool {
	mutating := []string{
		" rm ",
		"rm -",
		"sudo rm",
		"mv ",
		"cp ",
		"chmod ",
		"chown ",
		"systemctl restart",
		"systemctl stop",
		"docker system prune",
		"apt-get install",
		"apt install",
		"pip install",
		"npm install",
		"tee ",
		">",
	}
	padded := " " + strings.ToLower(command) + " "
	for _, token := range mutating {
		if strings.Contains(padded, token) {
			return true
		}
	}
	return false
}

type ErrDenied struct {
	Reason string
}

func (e ErrDenied) Error() string {
	return e.Reason
}
