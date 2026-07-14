package capability

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/meshclaw/meshclaw/internal/inventory"
)

type Kind string

const (
	KindModel       Kind = "model"
	KindAPI         Kind = "api"
	KindService     Kind = "service"
	KindProvisioner Kind = "provisioner"
)

type SecretPolicy string

const (
	SecretNone          SecretPolicy = "none"
	SecretUseOnly       SecretPolicy = "use_without_reveal"
	SecretApprovalGated SecretPolicy = "approval_gated"
)

type Capability struct {
	ID           string       `json:"id"`
	Kind         Kind         `json:"kind"`
	Provider     string       `json:"provider"`
	Host         string       `json:"host,omitempty"`
	Description  string       `json:"description"`
	Capabilities []string     `json:"capabilities"`
	Status       string       `json:"status"`
	SecretPolicy SecretPolicy `json:"secret_policy"`
	Policy       string       `json:"policy"`
}

type Store struct {
	Version      int          `json:"version"`
	GeneratedBy  string       `json:"generated_by"`
	Capabilities []Capability `json:"capabilities"`
}

type ValidationIssue struct {
	Capability string `json:"capability,omitempty"`
	Field      string `json:"field"`
	Message    string `json:"message"`
}

type ValidationReport struct {
	Path         string            `json:"path"`
	Valid        bool              `json:"valid"`
	Count        int               `json:"count"`
	Errors       []ValidationIssue `json:"errors"`
	Warnings     []ValidationIssue `json:"warnings"`
	Capabilities []Capability      `json:"capabilities"`
}

type RecommendationReport struct {
	Intent              string                    `json:"intent"`
	Class               string                    `json:"class"`
	Registry            ValidationReport          `json:"registry"`
	Candidates          []RecommendationCandidate `json:"candidates"`
	Rejected            []RecommendationCandidate `json:"rejected,omitempty"`
	RecommendedNextCall string                    `json:"recommended_next_call"`
}

type RecommendationCandidate struct {
	Capability       Capability `json:"capability"`
	Score            int        `json:"score"`
	Reasons          []string   `json:"reasons"`
	Cautions         []string   `json:"cautions,omitempty"`
	ApprovalRequired bool       `json:"approval_required"`
}

const storeVersion = 1

func Path() string {
	if path := strings.TrimSpace(os.Getenv("MESHCLAW_CAPABILITY_FILE")); path != "" {
		return path
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".meshclaw", "capabilities.json")
	}
	return filepath.Join(".meshclaw", "capabilities.json")
}

func List() []Capability {
	configured, err := Load()
	if err != nil || len(configured) == 0 {
		configured = defaultCapabilities()
	}
	return normalizeCapabilities(Merge(configured, DiscoverFromInventory()))
}

func Load() ([]Capability, error) {
	return LoadPath(Path())
}

func LoadPath(path string) ([]Capability, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var store Store
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, err
	}
	caps := normalizeCapabilities(store.Capabilities)
	if len(caps) == 0 {
		return nil, errors.New("capability registry has no capabilities")
	}
	return caps, nil
}

func Validate(path string) ValidationReport {
	if strings.TrimSpace(path) == "" {
		path = Path()
	}
	report := ValidationReport{Path: path, Valid: true}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			report.Capabilities = normalizeCapabilities(Merge(defaultCapabilities(), DiscoverFromInventory()))
			report.Count = len(report.Capabilities)
			report.Warnings = append(report.Warnings, ValidationIssue{Field: "path", Message: "configured capability registry does not exist; using built-in defaults plus inventory-discovered capabilities"})
			return report
		}
		report.Valid = false
		report.Errors = append(report.Errors, ValidationIssue{Field: "path", Message: err.Error()})
		return report
	}
	var store Store
	if err := json.Unmarshal(data, &store); err != nil {
		report.Valid = false
		report.Errors = append(report.Errors, ValidationIssue{Field: "json", Message: err.Error()})
		return report
	}
	report.Capabilities = normalizeCapabilities(store.Capabilities)
	report.Count = len(report.Capabilities)
	if store.Version == 0 {
		report.Warnings = append(report.Warnings, ValidationIssue{Field: "version", Message: "version is missing; MeshClaw will treat this as the current registry schema"})
	} else if store.Version != storeVersion {
		report.Warnings = append(report.Warnings, ValidationIssue{Field: "version", Message: "registry version differs from this MeshClaw binary"})
	}
	if len(store.Capabilities) == 0 {
		report.Errors = append(report.Errors, ValidationIssue{Field: "capabilities", Message: "capability registry has no capabilities"})
	}
	seen := map[string]bool{}
	for i, cap := range store.Capabilities {
		cap = normalizeCapability(cap)
		label := cap.ID
		if label == "" {
			label = fmt.Sprintf("capability[%d]", i)
		}
		if cap.ID == "" {
			report.Errors = append(report.Errors, ValidationIssue{Capability: label, Field: "id", Message: "id is required"})
		} else if seen[cap.ID] {
			report.Errors = append(report.Errors, ValidationIssue{Capability: label, Field: "id", Message: "duplicate capability id"})
		}
		seen[cap.ID] = true
		if !validKind(cap.Kind) {
			report.Errors = append(report.Errors, ValidationIssue{Capability: label, Field: "kind", Message: "kind must be one of model, api, service, provisioner"})
		}
		if cap.Provider == "" {
			report.Errors = append(report.Errors, ValidationIssue{Capability: label, Field: "provider", Message: "provider is required"})
		}
		if len(cap.Capabilities) == 0 {
			report.Warnings = append(report.Warnings, ValidationIssue{Capability: label, Field: "capabilities", Message: "capability should expose at least one machine-readable capability tag"})
		}
		if !validSecretPolicy(cap.SecretPolicy) {
			report.Errors = append(report.Errors, ValidationIssue{Capability: label, Field: "secret_policy", Message: "secret_policy must be one of none, use_without_reveal, approval_gated"})
		}
		if cap.SecretPolicy != SecretNone && !strings.Contains(strings.ToLower(cap.Policy), "approval") && !strings.Contains(strings.ToLower(cap.Policy), "use") {
			report.Warnings = append(report.Warnings, ValidationIssue{Capability: label, Field: "policy", Message: "secret-bearing capabilities should describe use-only or approval-gated handling"})
		}
	}
	report.Valid = len(report.Errors) == 0
	return report
}

func Recommend(intent string) RecommendationReport {
	intent = strings.TrimSpace(intent)
	class := classifyIntent(intent)
	registry := Validate("")
	caps := registry.Capabilities
	if len(caps) == 0 && registry.Valid {
		caps = List()
		registry.Capabilities = caps
		registry.Count = len(caps)
	}
	registrySummary := registry
	registrySummary.Capabilities = nil
	report := RecommendationReport{
		Intent:              intent,
		Class:               class,
		Registry:            registrySummary,
		Candidates:          []RecommendationCandidate{},
		Rejected:            []RecommendationCandidate{},
		RecommendedNextCall: "meshclaw placement-plan",
	}
	rejected := []RecommendationCandidate{}
	for _, cap := range caps {
		candidate := scoreRecommendation(class, intent, cap)
		if candidate.Score > 0 {
			report.Candidates = append(report.Candidates, candidate)
		} else {
			rejected = append(rejected, candidate)
		}
	}
	sort.Slice(report.Candidates, func(i, j int) bool {
		if report.Candidates[i].Score == report.Candidates[j].Score {
			return report.Candidates[i].Capability.ID < report.Candidates[j].Capability.ID
		}
		return report.Candidates[i].Score > report.Candidates[j].Score
	})
	if len(report.Candidates) > 8 {
		report.Candidates = report.Candidates[:8]
	}
	if len(report.Candidates) == 0 {
		sort.Slice(rejected, func(i, j int) bool {
			return rejected[i].Capability.ID < rejected[j].Capability.ID
		})
		if len(rejected) > 8 {
			rejected = rejected[:8]
		}
		report.Rejected = rejected
	}
	return report
}

func Save(caps []Capability) error {
	caps = normalizeCapabilities(caps)
	if err := os.MkdirAll(filepath.Dir(Path()), 0700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(Store{
		Version:      storeVersion,
		GeneratedBy:  "meshclaw capability registry",
		Capabilities: caps,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(), append(payload, '\n'), 0600)
}

func classifyIntent(intent string) string {
	lower := strings.ToLower(intent)
	switch {
	case containsAny(lower, "ollama", "llm", "model", "gpu", "vllm", "inference", "coding", "qwen", "gemma", "llama", "local model", "모델", "올라마", "추론", "코딩", "gpu"):
		return "model"
	case containsAny(lower, "cloudflare", "provider_token", "dns_change", "api", "token", "openai", "claude", "cloud", "provider", "키", "토큰"):
		return "api"
	case containsAny(lower, "n8n", "workflow", "automation", "browser", "screenshot", "mail-client", "client-install", "automate", "자동화", "워크플로", "브라우저", "스크린샷"):
		return "automation"
	case containsAny(lower, "mail", "smtp", "imap", "mox", "email", "메일", "이메일"):
		return "mail"
	case containsAny(lower, "backup", "artifact", "storage", "nas", "archive", "disk", "백업", "스토리지", "아카이브", "디스크"):
		return "storage"
	case containsAny(lower, "vps", "provision", "rent", "server create", "bootstrap", "임대", "서버 생성", "프로비전"):
		return "provision"
	default:
		return "general"
	}
}

func scoreRecommendation(class, intent string, cap Capability) RecommendationCandidate {
	cap = normalizeCapability(cap)
	statusScore := 0
	score := 0
	reasons := []string{}
	cautions := []string{}
	lowerIntent := strings.ToLower(intent)
	switch strings.ToLower(cap.Status) {
	case "available", "online":
		statusScore = 30
		reasons = append(reasons, "capability is available now")
	case "configured_elsewhere", "configured":
		statusScore = 14
		reasons = append(reasons, "capability exists but depends on external configuration")
	case "plan_only":
		statusScore = 8
		reasons = append(reasons, "capability is planning-only")
	default:
		cautions = append(cautions, "availability is unknown")
	}
	switch cap.SecretPolicy {
	case SecretUseOnly:
		cautions = append(cautions, "secret-bearing capability: use handles only, never reveal values")
	case SecretApprovalGated:
		cautions = append(cautions, "approval-gated secret or provider capability")
	}
	if strings.Contains(strings.ToLower(cap.Policy), "approval") {
		cautions = append(cautions, "policy mentions approval")
	}
	matchScore := classScore(class, cap, &reasons) + textScore(lowerIntent, cap, &reasons)
	if class == "general" || matchScore > 0 {
		score = statusScore + matchScore
	}
	if class == "automation" && capabilityHasAny(cap, "mox", "smtp", "imap", "webmail") && !containsAny(lowerIntent, "mox", "smtp", "imap", "webmail", "email_send") && score > 0 {
		score -= 45
		reasons = append(reasons, "mail-server capability ranked behind client/browser automation for this intent")
	}
	if class != "general" && strings.HasSuffix(cap.ID, "-node") && score > 0 {
		score -= 20
		reasons = append(reasons, "generic node capability ranked behind specific role capabilities")
	}
	if score == 0 {
		reasons = append(reasons, "no strong match for this intent")
	}
	return RecommendationCandidate{
		Capability:       cap,
		Score:            score,
		Reasons:          mergeStrings(nil, reasons),
		Cautions:         mergeStrings(nil, cautions),
		ApprovalRequired: cap.SecretPolicy == SecretApprovalGated || strings.Contains(strings.ToLower(cap.Policy), "require approval") || strings.Contains(strings.ToLower(cap.Policy), "requires approval"),
	}
}

func classScore(class string, cap Capability, reasons *[]string) int {
	score := 0
	switch class {
	case "model":
		if cap.Kind == KindModel || capabilityHasAny(cap, "gpu", "model_worker", "batch_inference", "ollama_candidate", "chat", "vision", "embeddings") {
			score += 45
			*reasons = append(*reasons, "matches model or inference work")
		}
	case "mail":
		if capabilityHasAny(cap, "mox", "smtp", "imap", "webmail") || strings.Contains(strings.ToLower(cap.Provider), "mail") {
			score += 45
			*reasons = append(*reasons, "matches mail infrastructure work")
		}
	case "automation":
		if capabilityHasAny(cap, "n8n", "workflow_worker", "workflow-controller", "browser", "codex", "verifier") {
			score += 40
			*reasons = append(*reasons, "matches workflow or browser automation")
		}
	case "storage":
		if capabilityHasAny(cap, "storage", "artifact_archive", "backup_candidate", "render-video") {
			score += 40
			*reasons = append(*reasons, "matches storage, artifact, or disk work")
		}
	case "provision":
		if cap.Kind == KindProvisioner || capabilityHasAny(cap, "provision_plan", "provision_server", "bootstrap_server", "deprovision_server") {
			score += 45
			*reasons = append(*reasons, "matches provisioning work")
		}
	case "api":
		if cap.Kind == KindAPI || cap.SecretPolicy != SecretNone {
			score += 35
			*reasons = append(*reasons, "matches API or token-backed work")
		}
	default:
		if capabilityHasAny(cap, "inventory", "facts", "run_evidence", "server_list", "fleet_status") {
			score += 20
			*reasons = append(*reasons, "general operational capability")
		}
	}
	return score
}

func textScore(lowerIntent string, cap Capability, reasons *[]string) int {
	if lowerIntent == "" {
		return 0
	}
	score := 0
	fields := []string{cap.ID, cap.Provider, cap.Host, cap.Description, string(cap.Kind)}
	fields = append(fields, cap.Capabilities...)
	for _, field := range fields {
		field = strings.ToLower(field)
		if field == "" {
			continue
		}
		if strings.Contains(lowerIntent, field) {
			score += 20
			*reasons = append(*reasons, "intent explicitly mentions "+field)
			continue
		}
		for _, token := range strings.Fields(lowerIntent) {
			token = strings.Trim(token, ".,:;()[]{}'\"`")
			if len(token) >= 3 && !recommendationStopword(token) && strings.Contains(field, token) {
				score += 6
				*reasons = append(*reasons, "intent keyword matches "+token)
				break
			}
		}
	}
	return score
}

func recommendationStopword(token string) bool {
	switch token {
	case "the", "and", "for", "with", "from", "into", "onto", "after", "before", "through", "using", "check", "checking", "send", "test", "choose", "worker", "node":
		return true
	default:
		return false
	}
}

func capabilityHasAny(cap Capability, values ...string) bool {
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if strings.Contains(strings.ToLower(cap.ID), value) || strings.Contains(strings.ToLower(cap.Provider), value) || strings.Contains(strings.ToLower(cap.Description), value) {
			return true
		}
		for _, got := range cap.Capabilities {
			got = strings.ToLower(strings.TrimSpace(got))
			if got == value || strings.Contains(got, value) {
				return true
			}
		}
	}
	return false
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func validKind(kind Kind) bool {
	switch kind {
	case KindModel, KindAPI, KindService, KindProvisioner:
		return true
	default:
		return false
	}
}

func validSecretPolicy(policy SecretPolicy) bool {
	switch policy {
	case SecretNone, SecretUseOnly, SecretApprovalGated:
		return true
	default:
		return false
	}
}

func Init(force bool) ([]Capability, error) {
	if !force {
		if caps, err := Load(); err == nil && len(caps) > 0 {
			return List(), nil
		}
	}
	caps := Merge(defaultCapabilities(), DiscoverFromInventory())
	if err := Save(caps); err != nil {
		return nil, err
	}
	return normalizeCapabilities(caps), nil
}

func DiscoverFromInventory() []Capability {
	nodes := inventory.DefaultNodes()
	caps := make([]Capability, 0, len(nodes))
	for _, node := range nodes {
		caps = append(caps, nodeCapability(node))
		if hasAny(node, "gpu") {
			caps = append(caps, Capability{
				ID:           node.Name + "-gpu-compute",
				Kind:         KindService,
				Provider:     "gpu-node",
				Host:         node.Name,
				Description:  "GPU-capable node discovered from MeshClaw inventory tags.",
				Capabilities: []string{"gpu", "model_worker", "batch_inference"},
				Status:       nodeStatus(node),
				SecretPolicy: SecretNone,
				Policy:       "read-only inspection allowed; workload placement and mutation are policy-gated",
			})
		}
		if hasAny(node, "storage") || strings.Contains(strings.ToLower(node.Role), "nas") {
			caps = append(caps, Capability{
				ID:           node.Name + "-storage",
				Kind:         KindService,
				Provider:     "storage-node",
				Host:         node.Name,
				Description:  "Storage/NAS capability discovered from MeshClaw inventory.",
				Capabilities: []string{"storage", "artifact_archive", "backup_candidate"},
				Status:       nodeStatus(node),
				SecretPolicy: SecretNone,
				Policy:       "artifact collection is policy-gated and redacted by default",
			})
		}
		if hasAny(node, "mail") || strings.Contains(strings.ToLower(node.Role), "mail") {
			caps = append(caps, Capability{
				ID:           node.Name + "-mail",
				Kind:         KindService,
				Provider:     "mail-server",
				Host:         node.Name,
				Description:  "Mail server capability discovered from MeshClaw inventory.",
				Capabilities: []string{"mox", "smtp", "imap", "webmail"},
				Status:       nodeStatus(node),
				SecretPolicy: SecretUseOnly,
				Policy:       "mail inspection is allowed; account changes and email sends require approval",
			})
		}
		if hasAny(node, "automation") || strings.Contains(strings.ToLower(node.Role), "automation") {
			caps = append(caps, Capability{
				ID:           node.Name + "-automation",
				Kind:         KindService,
				Provider:     "automation-node",
				Host:         node.Name,
				Description:  "Automation worker capability discovered from MeshClaw inventory.",
				Capabilities: []string{"n8n", "workflow_worker", "ollama_candidate"},
				Status:       nodeStatus(node),
				SecretPolicy: SecretUseOnly,
				Policy:       "workflow changes and external API use require policy approval",
			})
		}
	}
	return normalizeCapabilities(caps)
}

func Merge(base, incoming []Capability) []Capability {
	merged := map[string]Capability{}
	for _, cap := range append(base, incoming...) {
		cap = normalizeCapability(cap)
		if cap.ID == "" {
			continue
		}
		if existing, ok := merged[cap.ID]; ok {
			merged[cap.ID] = mergeCapability(existing, cap)
		} else {
			merged[cap.ID] = cap
		}
	}
	out := make([]Capability, 0, len(merged))
	for _, cap := range merged {
		out = append(out, cap)
	}
	return normalizeCapabilities(out)
}

func defaultCapabilities() []Capability {
	return []Capability{
		{
			ID:           "vssh-native",
			Kind:         KindService,
			Provider:     "vssh",
			Description:  "Fleet execution through vssh-native over Tailscale/private routes.",
			Capabilities: []string{"server_list", "fleet_status", "server_info", "run", "fleet_exec"},
			Status:       "available",
			SecretPolicy: SecretNone,
			Policy:       "raw execution is policy-gated and evidence-backed through MeshClaw",
		},
		{
			ID:           "macbook-controller",
			Kind:         KindService,
			Provider:     "local-runtime",
			Host:         "macbook",
			Description:  "Local Codex Desktop controller, verifier, and artifact render node.",
			Capabilities: []string{"codex", "browser", "mail-client", "render-video", "workflow-controller", "verifier"},
			Status:       "available",
			SecretPolicy: SecretUseOnly,
			Policy:       "local controller actions are policy-gated when they mutate external systems",
		},
		{
			ID:           "openai-api",
			Kind:         KindAPI,
			Provider:     "openai",
			Description:  "External model API capability placeholder.",
			Capabilities: []string{"chat", "embeddings", "vision"},
			Status:       "configured_elsewhere",
			SecretPolicy: SecretUseOnly,
			Policy:       "external data policy applies",
		},
		{
			ID:           "cloudflare-api",
			Kind:         KindAPI,
			Provider:     "cloudflare",
			Description:  "Cloudflare DNS and zone API capability placeholder.",
			Capabilities: []string{"dns", "zone_read", "dns_change", "provider_token"},
			Status:       "configured_elsewhere",
			SecretPolicy: SecretUseOnly,
			Policy:       "DNS reads are inspectable through use-only tokens; DNS changes require approval",
		},
		{
			ID:           "mail-ops",
			Kind:         KindService,
			Provider:     "mail-runtime",
			Description:  "Generic mail workflow capability placeholder for first-run planning.",
			Capabilities: []string{"email_send", "account_configure", "mail-client"},
			Status:       "plan_only",
			SecretPolicy: SecretUseOnly,
			Policy:       "mail sends, account setup, and client password use require approval",
		},
		{
			ID:           "temporary-vps",
			Kind:         KindProvisioner,
			Provider:     "provider-api",
			Description:  "Approved temporary server capacity hook placeholder.",
			Capabilities: []string{"provision_plan", "provision_server", "bootstrap_server", "deprovision_server"},
			Status:       "plan_only",
			SecretPolicy: SecretApprovalGated,
			Policy:       "cost-incurring actions require explicit approval",
		},
	}
}

func nodeCapability(node inventory.Node) Capability {
	capabilities := []string{"inventory", "facts", "run_evidence"}
	for _, tag := range node.Tags {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			capabilities = append(capabilities, "tag:"+tag)
		}
	}
	if node.Role != "" {
		capabilities = append(capabilities, "role:"+node.Role)
	}
	return Capability{
		ID:           node.Name + "-node",
		Kind:         KindService,
		Provider:     "meshclaw-inventory",
		Host:         node.Name,
		Description:  "Inventory-backed server capability discovered by MeshClaw.",
		Capabilities: capabilities,
		Status:       nodeStatus(node),
		SecretPolicy: SecretNone,
		Policy:       "read-only state is allowed; mutating operations are policy-gated",
	}
}

func nodeStatus(node inventory.Node) string {
	if node.Online {
		return "available"
	}
	return "unknown"
}

func hasAny(node inventory.Node, values ...string) bool {
	role := strings.ToLower(node.Role)
	for _, value := range values {
		value = strings.ToLower(value)
		if strings.Contains(role, value) {
			return true
		}
		for _, tag := range node.Tags {
			if strings.EqualFold(tag, value) || strings.Contains(strings.ToLower(tag), value) {
				return true
			}
		}
	}
	return false
}

func mergeCapability(existing, incoming Capability) Capability {
	if incoming.Kind != "" {
		existing.Kind = incoming.Kind
	}
	if incoming.Provider != "" {
		existing.Provider = incoming.Provider
	}
	if incoming.Host != "" {
		existing.Host = incoming.Host
	}
	if incoming.Description != "" {
		existing.Description = incoming.Description
	}
	if len(incoming.Capabilities) > 0 {
		existing.Capabilities = mergeStrings(existing.Capabilities, incoming.Capabilities)
	}
	if incoming.Status != "" {
		existing.Status = incoming.Status
	}
	if incoming.SecretPolicy != "" {
		existing.SecretPolicy = incoming.SecretPolicy
	}
	if incoming.Policy != "" {
		existing.Policy = incoming.Policy
	}
	return normalizeCapability(existing)
}

func normalizeCapabilities(caps []Capability) []Capability {
	out := make([]Capability, 0, len(caps))
	for _, cap := range caps {
		cap = normalizeCapability(cap)
		if cap.ID == "" {
			continue
		}
		out = append(out, cap)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func normalizeCapability(cap Capability) Capability {
	cap.ID = strings.TrimSpace(cap.ID)
	cap.Provider = strings.TrimSpace(cap.Provider)
	cap.Host = strings.TrimSpace(cap.Host)
	cap.Description = strings.TrimSpace(cap.Description)
	cap.Status = strings.TrimSpace(cap.Status)
	cap.Policy = strings.TrimSpace(cap.Policy)
	cap.Capabilities = mergeStrings(nil, cap.Capabilities)
	if cap.Status == "" {
		cap.Status = "unknown"
	}
	if cap.SecretPolicy == "" {
		cap.SecretPolicy = SecretNone
	}
	return cap
}

func mergeStrings(base, incoming []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range append(base, incoming...) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
