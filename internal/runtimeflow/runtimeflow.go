package runtimeflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/meshclaw/meshclaw/internal/capability"
	"github.com/meshclaw/meshclaw/internal/guardvault"
	"github.com/meshclaw/meshclaw/internal/inventory"
	"github.com/meshclaw/meshclaw/internal/policy"
	"github.com/meshclaw/meshclaw/internal/runtime"
)

type Mode string

const (
	DryRun  Mode = "dry-run"
	Execute Mode = "execute"
)

type Result struct {
	Success          bool                    `json:"success"`
	Workflow         string                  `json:"workflow"`
	Mode             Mode                    `json:"mode"`
	GeneratedAt      time.Time               `json:"generated_at"`
	BundleDir        string                  `json:"bundle_dir"`
	EvidenceBundle   string                  `json:"evidence_bundle"`
	PlanPath         string                  `json:"plan_path"`
	ExecutionPath    string                  `json:"execution_path"`
	StepsPath        string                  `json:"steps_path"`
	ReportPath       string                  `json:"report_path"`
	ActionsPath      string                  `json:"actions_path"`
	ApprovalsPath    string                  `json:"approvals_path"`
	CapabilitiesPath string                  `json:"capabilities_path"`
	AdaptersPath     string                  `json:"adapters_path"`
	Steps            []ExecutionResult       `json:"steps"`
	Capabilities     []capability.Capability `json:"capabilities"`
	Summary          Summary                 `json:"summary"`
}

type Summary struct {
	Total            int `json:"total"`
	Succeeded        int `json:"succeeded"`
	Failed           int `json:"failed"`
	ApprovalRequired int `json:"approval_required"`
	Skipped          int `json:"skipped"`
	Retryable        int `json:"retryable"`
}

type StepSpec struct {
	ID               string            `json:"id"`
	Title            string            `json:"title"`
	Node             string            `json:"node,omitempty"`
	Transport        string            `json:"transport"`
	Command          string            `json:"command,omitempty"`
	Action           string            `json:"action"`
	Resource         string            `json:"resource"`
	DependsOn        []string          `json:"depends_on,omitempty"`
	FallbackFor      []string          `json:"fallback_for,omitempty"`
	TimeoutSeconds   int               `json:"timeout_seconds,omitempty"`
	Retry            RetrySpec         `json:"retry,omitempty"`
	SecretEnv        map[string]string `json:"secret_env,omitempty"`
	VaultHandles     []string          `json:"vault_handles,omitempty"`
	ApprovalRequired bool              `json:"approval_required"`
	StrongApproval   bool              `json:"strong_approval,omitempty"`
	DryRunNote       string            `json:"dry_run_note,omitempty"`
	Artifacts        []string          `json:"artifacts,omitempty"`
}

type WorkflowInfo struct {
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	StepCount          int      `json:"step_count"`
	ApprovalActions    []string `json:"approval_actions"`
	ExecutableAdapters []string `json:"executable_adapters"`
	RecommendedMode    Mode     `json:"recommended_mode"`
}

type WorkflowDefinition struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Steps       []StepSpec `json:"steps"`
	Source      string     `json:"source,omitempty"`
}

type WorkflowInspection struct {
	Info               WorkflowInfo            `json:"info"`
	Steps              []StepInspection        `json:"steps"`
	ApprovalGates      []ApprovalGate          `json:"approval_gates"`
	RequiredAdapters   []string                `json:"required_adapters"`
	RequiredNodes      []string                `json:"required_nodes"`
	CapabilityMatches  map[string][]string     `json:"capability_matches"`
	CapabilityHints    map[string][]string     `json:"capability_hints"`
	CapabilitySnapshot []capability.Capability `json:"capability_snapshot"`
}

type WorkflowValidation struct {
	Valid       bool              `json:"valid"`
	Workflow    string            `json:"workflow"`
	Source      string            `json:"source,omitempty"`
	StepCount   int               `json:"step_count"`
	Errors      []ValidationIssue `json:"errors,omitempty"`
	Warnings    []ValidationIssue `json:"warnings,omitempty"`
	NextActions []string          `json:"next_actions,omitempty"`
}

type ValidationIssue struct {
	Severity string `json:"severity"`
	Step     string `json:"step,omitempty"`
	Field    string `json:"field,omitempty"`
	Message  string `json:"message"`
}

type StepInspection struct {
	Index                int           `json:"index"`
	Step                 StepSpec      `json:"step"`
	Adapter              AdapterSpec   `json:"adapter"`
	Policy               policy.Result `json:"policy"`
	VaultChecks          []VaultCheck  `json:"vault_checks,omitempty"`
	WillExecuteInDryRun  bool          `json:"will_execute_in_dry_run"`
	WillExecuteInExecute bool          `json:"will_execute_in_execute"`
}

type ApprovalGate struct {
	Step         string   `json:"step"`
	Title        string   `json:"title"`
	Action       string   `json:"action"`
	Resource     string   `json:"resource"`
	Reason       string   `json:"reason"`
	Strong       bool     `json:"strong"`
	VaultHandles []string `json:"vault_handles,omitempty"`
}

type VaultCheck struct {
	Handle       string `json:"handle"`
	Exists       bool   `json:"exists"`
	Kind         string `json:"kind,omitempty"`
	Description  string `json:"description,omitempty"`
	Fingerprint  string `json:"fingerprint,omitempty"`
	LastUsedAt   string `json:"last_used_at,omitempty"`
	RawAvailable bool   `json:"raw_available,omitempty"`
	Error        string `json:"error,omitempty"`
	ImportCLI    string `json:"import_cli,omitempty"`
	NextAction   string `json:"next_action,omitempty"`
}

type ResumePlan struct {
	Kind            string           `json:"kind"`
	BundleDir       string           `json:"bundle_dir"`
	Workflow        string           `json:"workflow"`
	PreviousMode    Mode             `json:"previous_mode"`
	PreviousSuccess bool             `json:"previous_success"`
	GeneratedAt     time.Time        `json:"generated_at"`
	ResumePath      string           `json:"resume_path"`
	ApprovalsPath   string           `json:"approvals_path"`
	Summary         Summary          `json:"summary"`
	Approvals       []ApprovalRecord `json:"approvals"`
	Items           []ResumeItem     `json:"items"`
	Next            []string         `json:"next"`
}

type ExecutePlan struct {
	Kind               string                      `json:"kind"`
	BundleDir          string                      `json:"bundle_dir"`
	Workflow           string                      `json:"workflow"`
	PreviousMode       Mode                        `json:"previous_mode"`
	Ready              bool                        `json:"ready"`
	Decision           string                      `json:"decision"`
	Reason             string                      `json:"reason"`
	Summary            Summary                     `json:"summary"`
	Counts             ExecuteCount                `json:"counts"`
	CapabilityRegistry capability.ValidationReport `json:"capability_registry"`
	ExecuteCLI         string                      `json:"execute_cli,omitempty"`
	ExecuteMCPCall     map[string]interface{}      `json:"execute_mcp_call,omitempty"`
	ReadySteps         []ResumeItem                `json:"ready_steps,omitempty"`
	ApprovalPending    []ResumeItem                `json:"approval_pending,omitempty"`
	VaultMissing       []ResumeItem                `json:"vault_missing,omitempty"`
	RetryableFailed    []ResumeItem                `json:"retryable_failed,omitempty"`
	Failed             []ResumeItem                `json:"failed,omitempty"`
	RepairNeeded       []ResumeItem                `json:"repair_needed,omitempty"`
	Next               []string                    `json:"next"`
	RecommendedMCP     []RecommendedMCPCall        `json:"recommended_mcp"`
}

type RecommendedMCPCall struct {
	Tool      string                 `json:"tool"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
	Reason    string                 `json:"reason"`
	Priority  string                 `json:"priority,omitempty"`
}

type ExecuteCount struct {
	Ready           int `json:"ready"`
	ApprovalPending int `json:"approval_pending"`
	VaultMissing    int `json:"vault_missing"`
	RetryableFailed int `json:"retryable_failed"`
	Failed          int `json:"failed"`
	RepairNeeded    int `json:"repair_needed"`
}

type ApprovalRecord struct {
	Time           time.Time `json:"time"`
	Actor          string    `json:"actor"`
	Workflow       string    `json:"workflow"`
	Step           string    `json:"step"`
	Title          string    `json:"title,omitempty"`
	Action         string    `json:"action"`
	Resource       string    `json:"resource"`
	Reason         string    `json:"reason"`
	StrongApproval bool      `json:"strong_approval,omitempty"`
	Bundle         string    `json:"bundle"`
	Source         string    `json:"source,omitempty"`
}

type ResumeItem struct {
	Index            int                    `json:"index"`
	Step             string                 `json:"step"`
	Title            string                 `json:"title"`
	Node             string                 `json:"node,omitempty"`
	Transport        string                 `json:"transport"`
	Action           string                 `json:"action,omitempty"`
	Resource         string                 `json:"resource,omitempty"`
	Status           string                 `json:"status"`
	Retryable        bool                   `json:"retryable"`
	ApprovalRequired bool                   `json:"approval_required"`
	StrongApproval   bool                   `json:"strong_approval,omitempty"`
	Approved         bool                   `json:"approved,omitempty"`
	ApprovalActor    string                 `json:"approval_actor,omitempty"`
	ApprovalTime     string                 `json:"approval_time,omitempty"`
	ApprovalSource   string                 `json:"approval_source,omitempty"`
	Reason           string                 `json:"reason"`
	Command          string                 `json:"command,omitempty"`
	VaultChecks      []VaultCheck           `json:"vault_checks,omitempty"`
	NextAction       string                 `json:"next_action"`
	ApprovalCLI      string                 `json:"approval_cli,omitempty"`
	ApprovalMCP      string                 `json:"approval_mcp,omitempty"`
	ApprovalMCPCall  map[string]interface{} `json:"approval_mcp_call,omitempty"`
	ExecuteCLI       string                 `json:"execute_cli,omitempty"`
	RepairCLI        string                 `json:"repair_cli,omitempty"`
	RepairMCPCall    map[string]interface{} `json:"repair_mcp_call,omitempty"`
}

type ExecutionResult struct {
	Success           bool              `json:"success"`
	Workflow          string            `json:"workflow"`
	Step              string            `json:"step"`
	Title             string            `json:"title"`
	Node              string            `json:"node,omitempty"`
	Transport         string            `json:"transport"`
	AdapterKind       string            `json:"adapter_kind"`
	AdapterExecutable bool              `json:"adapter_executable"`
	AdapterReason     string            `json:"adapter_reason,omitempty"`
	CapabilityHints   []string          `json:"capability_hints,omitempty"`
	CapabilityClass   string            `json:"capability_class,omitempty"`
	Command           string            `json:"command,omitempty"`
	Action            string            `json:"action,omitempty"`
	Resource          string            `json:"resource,omitempty"`
	DependsOn         []string          `json:"depends_on,omitempty"`
	FallbackFor       []string          `json:"fallback_for,omitempty"`
	TimeoutSeconds    int               `json:"timeout_seconds,omitempty"`
	Retry             RetrySpec         `json:"retry,omitempty"`
	Attempts          []AttemptResult   `json:"attempts,omitempty"`
	SecretEnv         map[string]string `json:"secret_env,omitempty"`
	VaultHandles      []string          `json:"vault_handles,omitempty"`
	VaultChecks       []VaultCheck      `json:"vault_checks,omitempty"`
	Stdout            string            `json:"stdout"`
	Stderr            string            `json:"stderr"`
	StdoutBytes       int               `json:"stdout_bytes,omitempty"`
	StderrBytes       int               `json:"stderr_bytes,omitempty"`
	StdoutTruncated   bool              `json:"stdout_truncated,omitempty"`
	StderrTruncated   bool              `json:"stderr_truncated,omitempty"`
	OutputArtifact    string            `json:"output_artifact,omitempty"`
	ExitCode          int               `json:"exit_code"`
	DurationMs        int64             `json:"duration_ms"`
	Status            string            `json:"status"`
	FailureKind       string            `json:"failure_kind,omitempty"`
	NextAction        string            `json:"next_action,omitempty"`
	Retryable         bool              `json:"retryable"`
	ApprovalRequired  bool              `json:"approval_required"`
	StrongApproval    bool              `json:"strong_approval,omitempty"`
	Approved          bool              `json:"approved,omitempty"`
	ApprovalActor     string            `json:"approval_actor,omitempty"`
	ApprovalTime      string            `json:"approval_time,omitempty"`
	ApprovalReason    string            `json:"approval_reason,omitempty"`
	ApprovalSource    string            `json:"approval_source,omitempty"`
	PolicyDecision    string            `json:"policy_decision"`
	PolicyReason      string            `json:"policy_reason"`
	Skipped           bool              `json:"skipped"`
	SkipReason        string            `json:"skip_reason,omitempty"`
	Artifacts         []string          `json:"artifacts"`
	Error             string            `json:"error,omitempty"`
}

type RetrySpec struct {
	MaxAttempts        int      `json:"max_attempts,omitempty"`
	DelaySeconds       int      `json:"delay_seconds,omitempty"`
	RetryOnExitCodes   []int    `json:"retry_on_exit_codes,omitempty"`
	RetryOnErrorSubstr []string `json:"retry_on_error_substr,omitempty"`
}

type AttemptResult struct {
	Attempt    int    `json:"attempt"`
	Success    bool   `json:"success"`
	ExitCode   int    `json:"exit_code"`
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
	Retryable  bool   `json:"retryable"`
}

func Run(name string, mode Mode) (Result, error) {
	return RunWithApprovals(name, mode, "")
}

func RunWithApprovals(name string, mode Mode, approvalsRef string) (Result, error) {
	return RunSelectedWithApprovals(name, mode, approvalsRef, nil)
}

func RunSelectedWithApprovals(name string, mode Mode, approvalsRef string, selectedSteps []string) (Result, error) {
	name = strings.TrimSpace(name)
	if mode == "" {
		mode = DryRun
	}
	steps, err := workflowSteps(name)
	if err != nil {
		return Result{}, err
	}
	steps, err = selectSteps(steps, selectedSteps)
	if err != nil {
		return Result{}, err
	}
	approvalMap := map[string]ApprovalRecord{}
	approvalRecords := []ApprovalRecord{}
	if strings.TrimSpace(approvalsRef) != "" {
		records, err := ListApprovals(approvalsRef)
		if err != nil {
			return Result{}, err
		}
		approvalRecords = records
		for _, record := range records {
			if record.Workflow == name && record.Step != "" {
				approvalMap[record.Step] = record
			}
		}
	}
	now := time.Now().UTC()
	bundleDir, err := createBundleDir(now, name)
	if err != nil {
		return Result{}, err
	}
	result := Result{
		Success:        true,
		Workflow:       name,
		Mode:           mode,
		GeneratedAt:    now,
		BundleDir:      bundleDir,
		EvidenceBundle: bundleDir,
	}
	result.PlanPath = filepath.Join(bundleDir, "plan.md")
	result.ExecutionPath = filepath.Join(bundleDir, "execution.json")
	result.StepsPath = filepath.Join(bundleDir, "steps.jsonl")
	result.ReportPath = filepath.Join(bundleDir, "report.html")
	result.ActionsPath = filepath.Join(bundleDir, "meshclaw-actions.md")
	result.ApprovalsPath = filepath.Join(bundleDir, "approvals.jsonl")
	result.CapabilitiesPath = filepath.Join(bundleDir, "capabilities.json")
	result.AdaptersPath = filepath.Join(bundleDir, "adapters.json")
	result.Capabilities = capability.List()

	completed := map[string]ExecutionResult{}
	for _, step := range steps {
		execResult := fallbackGateResult(name, mode, step, completed, bundleDir)
		if execResult.Step == "" {
			execResult = dependencyBlockedResult(name, mode, step, completed, bundleDir)
		}
		if execResult.Step == "" {
			execResult = runStep(name, mode, step, approvalMap[step.ID], bundleDir)
		}
		result.Steps = append(result.Steps, execResult)
		completed[step.ID] = execResult
		updateSummary(&result.Summary, execResult)
		if !execResult.Success {
			result.Success = false
		}
	}
	if err := writeBundle(result, steps, approvalRecords); err != nil {
		return result, err
	}
	return result, nil
}

func selectSteps(steps []StepSpec, selected []string) ([]StepSpec, error) {
	wanted := map[string]bool{}
	for _, raw := range selected {
		for _, part := range strings.Split(raw, ",") {
			id := strings.TrimSpace(part)
			if id != "" {
				wanted[id] = true
			}
		}
	}
	if len(wanted) == 0 {
		return steps, nil
	}
	out := []StepSpec{}
	for _, step := range steps {
		if wanted[step.ID] {
			out = append(out, step)
			delete(wanted, step.ID)
		}
	}
	if len(wanted) > 0 {
		missing := make([]string, 0, len(wanted))
		for id := range wanted {
			missing = append(missing, id)
		}
		sort.Strings(missing)
		return nil, fmt.Errorf("unknown workflow step(s): %s", strings.Join(missing, ","))
	}
	return out, nil
}

func Resume(ref string) (ResumePlan, error) {
	bundle, err := resolveBundle(ref)
	if err != nil {
		return ResumePlan{}, err
	}
	result, err := loadResult(filepath.Join(bundle, "execution.json"))
	if err != nil {
		return ResumePlan{}, err
	}
	plan := ResumePlan{
		Kind:            "meshclaw_workflow_resume_plan",
		BundleDir:       bundle,
		Workflow:        result.Workflow,
		PreviousMode:    result.Mode,
		PreviousSuccess: result.Success,
		GeneratedAt:     time.Now().UTC(),
		ResumePath:      filepath.Join(bundle, "resume-plan.json"),
		ApprovalsPath:   filepath.Join(bundle, "approvals.jsonl"),
		Summary:         result.Summary,
	}
	approvals, err := ListApprovals(bundle)
	if err != nil {
		return ResumePlan{}, err
	}
	plan.Approvals = approvals
	approvedSteps := approvalRecordByStep(approvals)
	for i, step := range result.Steps {
		item, ok := classifyResumeItem(i+1, step, result.Mode, approvedSteps[step.Step])
		if !ok {
			continue
		}
		plan.Items = append(plan.Items, item)
	}
	plan.Next = resumeNextActions(plan)
	if err := writeResumePlan(plan); err != nil {
		return plan, err
	}
	return plan, nil
}

func PlanExecute(ref string) (ExecutePlan, error) {
	resume, err := Resume(ref)
	if err != nil {
		return ExecutePlan{}, err
	}
	plan := ExecutePlan{
		Kind:               "meshclaw_workflow_execute_plan",
		BundleDir:          resume.BundleDir,
		Workflow:           resume.Workflow,
		PreviousMode:       resume.PreviousMode,
		Summary:            resume.Summary,
		CapabilityRegistry: capability.Validate(""),
	}
	for _, item := range resume.Items {
		switch item.Status {
		case "ready_for_execute", "approved_ready":
			plan.ReadySteps = append(plan.ReadySteps, item)
		case "approval_pending":
			plan.ApprovalPending = append(plan.ApprovalPending, item)
		case "vault_missing":
			plan.VaultMissing = append(plan.VaultMissing, item)
		case "retryable_failed":
			plan.RetryableFailed = append(plan.RetryableFailed, item)
		case "degraded_repair":
			plan.RepairNeeded = append(plan.RepairNeeded, item)
		case "failed":
			plan.Failed = append(plan.Failed, item)
		}
	}
	plan.Counts = ExecuteCount{
		Ready:           len(plan.ReadySteps),
		ApprovalPending: len(plan.ApprovalPending),
		VaultMissing:    len(plan.VaultMissing),
		RetryableFailed: len(plan.RetryableFailed),
		Failed:          len(plan.Failed),
		RepairNeeded:    len(plan.RepairNeeded),
	}
	plan.Ready = plan.CapabilityRegistry.Valid && plan.Counts.ApprovalPending == 0 && plan.Counts.VaultMissing == 0 && plan.Counts.RetryableFailed == 0 && plan.Counts.Failed == 0 && plan.Counts.RepairNeeded == 0 && plan.Counts.Ready > 0
	plan.Decision, plan.Reason = executeDecision(plan)
	if plan.Ready {
		plan.ExecuteCLI = executeCLI(plan.Workflow, "")
		plan.ExecuteMCPCall = map[string]interface{}{
			"name": "meshclaw_workflow_run",
			"arguments": map[string]interface{}{
				"name":          plan.Workflow,
				"mode":          string(Execute),
				"approvals_ref": "latest",
			},
		}
	}
	plan.Next = executeNextActions(plan)
	plan.RecommendedMCP = executeRecommendedMCP(plan)
	return plan, nil
}

func executeDecision(plan ExecutePlan) (string, string) {
	switch {
	case !plan.CapabilityRegistry.Valid:
		return "capability_registry_invalid", "capability registry validation failed; repair configured capabilities before execute mode"
	case plan.Ready:
		return "ready", "all known approval, vault, retryable, and failure blockers are resolved"
	case plan.Counts.ApprovalPending > 0:
		return "approval_required", "one or more steps require explicit approval before execute mode"
	case plan.Counts.VaultMissing > 0:
		return "vault_required", "one or more steps require local vault handles before execute mode"
	case plan.Counts.RetryableFailed > 0 || plan.Counts.RepairNeeded > 0:
		return "repair_required", "retryable or degraded worker conditions should be repaired before execute mode"
	case plan.Counts.Failed > 0:
		return "blocked", "failed steps must be inspected before execute mode"
	case plan.Counts.Ready == 0:
		return "no_executable_steps", "no executable ready steps were found in the evidence bundle"
	default:
		return "blocked", "execute readiness could not be established"
	}
}

func executeNextActions(plan ExecutePlan) []string {
	next := []string{}
	if plan.Ready {
		next = append(next, "Run "+plan.ExecuteCLI+".")
		return next
	}
	if !plan.CapabilityRegistry.Valid {
		next = append(next, "Run `meshclaw capabilities validate --json`, fix the configured capability registry, then rerun execute preflight.")
	}
	for _, item := range plan.ApprovalPending {
		if item.ApprovalCLI != "" {
			next = append(next, item.ApprovalCLI)
		}
	}
	for _, item := range plan.VaultMissing {
		for _, check := range item.VaultChecks {
			if check.ImportCLI != "" {
				next = append(next, check.ImportCLI)
			}
		}
	}
	for _, item := range append(append([]ResumeItem{}, plan.RetryableFailed...), plan.RepairNeeded...) {
		if item.RepairCLI != "" {
			next = append(next, item.RepairCLI)
		}
	}
	if len(next) == 0 {
		next = append(next, "Inspect the evidence bundle and rerun dry-run after resolving blockers.")
	}
	return dedupeStrings(next)
}

func executeRecommendedMCP(plan ExecutePlan) []RecommendedMCPCall {
	calls := []RecommendedMCPCall{}
	add := func(tool, reason, priority string, args map[string]interface{}) {
		if tool == "" {
			return
		}
		for _, existing := range calls {
			if existing.Tool == tool && sameMCPArguments(existing.Arguments, args) {
				return
			}
		}
		calls = append(calls, RecommendedMCPCall{Tool: tool, Arguments: args, Reason: reason, Priority: priority})
	}
	if !plan.CapabilityRegistry.Valid {
		add("meshclaw_capability_validate", "capability registry blocks execute mode; validate and repair registry metadata first", "high", map[string]interface{}{})
	}
	if plan.Ready {
		add("meshclaw_workflow_run", "execute preflight is ready; run execute mode with prior approvals evidence", "high", map[string]interface{}{"name": plan.Workflow, "mode": string(Execute), "approvals_ref": "latest"})
	}
	for _, item := range plan.ApprovalPending {
		if call := recommendedFromMCPCall(item.ApprovalMCPCall, "approval is pending; record an explicit approval before execute mode", "high"); call.Tool != "" {
			add(call.Tool, call.Reason, call.Priority, call.Arguments)
		}
	}
	for _, item := range plan.VaultMissing {
		for _, check := range item.VaultChecks {
			add("meshclaw_guard_vault", "missing vault handle blocks execute mode; use local vault flow to import or review secret handling for "+check.Handle, "high", map[string]interface{}{})
		}
	}
	for _, item := range append(append([]ResumeItem{}, plan.RetryableFailed...), plan.RepairNeeded...) {
		if call := recommendedFromMCPCall(item.RepairMCPCall, "retryable or degraded worker state needs repair planning before execute mode", "high"); call.Tool != "" {
			add(call.Tool, call.Reason, call.Priority, call.Arguments)
		}
	}
	if plan.Counts.Failed > 0 {
		add("meshclaw_evidence_latest", "non-retryable failed steps require evidence inspection before rerun", "high", map[string]interface{}{"actions_preview_bytes": 4000})
	}
	if len(calls) == 0 {
		add("meshclaw_workflow_resume", "no execute-ready action is available; recompute resume state from latest evidence", "medium", map[string]interface{}{"ref": "latest"})
	}
	add("meshclaw_evidence_latest", "inspect workflow evidence before approval, repair, or execute mode", "low", map[string]interface{}{"actions_preview_bytes": 4000})
	return limitRecommendedMCPCalls(calls, 8)
}

func recommendedFromMCPCall(call map[string]interface{}, reason, priority string) RecommendedMCPCall {
	tool, _ := call["name"].(string)
	args, _ := call["arguments"].(map[string]interface{})
	return RecommendedMCPCall{Tool: tool, Arguments: args, Reason: reason, Priority: priority}
}

func sameMCPArguments(a, b map[string]interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for key, av := range a {
		if fmt.Sprint(av) != fmt.Sprint(b[key]) {
			return false
		}
	}
	return true
}

func limitRecommendedMCPCalls(calls []RecommendedMCPCall, limit int) []RecommendedMCPCall {
	if len(calls) <= limit {
		return calls
	}
	if limit <= 0 {
		return nil
	}
	last := calls[len(calls)-1]
	if last.Tool == "meshclaw_evidence_latest" && limit > 1 {
		out := make([]RecommendedMCPCall, 0, limit)
		out = append(out, calls[:limit-1]...)
		out = append(out, last)
		return out
	}
	return calls[:limit]
}

func dedupeStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func GrantApproval(ref, step, actor, reason, source string) (ApprovalRecord, error) {
	bundle, err := resolveBundle(ref)
	if err != nil {
		return ApprovalRecord{}, err
	}
	result, err := loadResult(filepath.Join(bundle, "execution.json"))
	if err != nil {
		return ApprovalRecord{}, err
	}
	step = strings.TrimSpace(step)
	if step == "" {
		return ApprovalRecord{}, fmt.Errorf("approval step is required")
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return ApprovalRecord{}, fmt.Errorf("approval actor is required")
	}
	target, ok := findExecutionStep(result, step)
	if !ok {
		return ApprovalRecord{}, fmt.Errorf("step %q not found in workflow %s", step, result.Workflow)
	}
	if !target.ApprovalRequired {
		return ApprovalRecord{}, fmt.Errorf("step %q does not require approval", step)
	}
	if target.StrongApproval && strings.TrimSpace(reason) == "" {
		return ApprovalRecord{}, fmt.Errorf("step %q requires strong approval with an explicit reason", step)
	}
	record := ApprovalRecord{
		Time:           time.Now().UTC(),
		Actor:          actor,
		Workflow:       result.Workflow,
		Step:           target.Step,
		Title:          target.Title,
		Action:         actionFromExecution(target),
		Resource:       resourceFromExecution(target),
		Reason:         strings.TrimSpace(reason),
		StrongApproval: target.StrongApproval,
		Bundle:         bundle,
		Source:         strings.TrimSpace(source),
	}
	if record.Reason == "" {
		record.Reason = "approved by " + actor
	}
	path := filepath.Join(bundle, "approvals.jsonl")
	data, err := json.Marshal(record)
	if err != nil {
		return ApprovalRecord{}, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return ApprovalRecord{}, err
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return ApprovalRecord{}, err
	}
	return record, nil
}

func ListApprovals(ref string) ([]ApprovalRecord, error) {
	bundle, err := resolveBundle(ref)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(bundle, "approvals.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []ApprovalRecord{}, nil
		}
		return nil, err
	}
	records := []ApprovalRecord{}
	for lineNo, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var record ApprovalRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("invalid approval record line %d: %w", lineNo+1, err)
		}
		records = append(records, record)
	}
	return records, nil
}

func resolveBundle(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" || ref == "latest" {
		return LatestBundle()
	}
	info, err := os.Stat(ref)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return filepath.EvalSymlinks(ref)
	}
	if filepath.Base(ref) == "execution.json" {
		return filepath.Dir(ref), nil
	}
	return "", fmt.Errorf("resume target must be a bundle directory, execution.json, or latest")
}

func loadResult(path string) (Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Result{}, err
	}
	var result Result
	if err := json.Unmarshal(data, &result); err != nil {
		return Result{}, err
	}
	if result.Workflow == "" {
		return Result{}, fmt.Errorf("execution result has no workflow: %s", path)
	}
	return result, nil
}

func classifyResumeItem(index int, step ExecutionResult, mode Mode, approval ApprovalRecord) (ResumeItem, bool) {
	approved := approval.Step == step.Step
	item := ResumeItem{
		Index:            index,
		Step:             step.Step,
		Title:            step.Title,
		Node:             step.Node,
		Transport:        step.Transport,
		Action:           actionFromExecution(step),
		Resource:         resourceFromExecution(step),
		Retryable:        step.Retryable,
		ApprovalRequired: step.ApprovalRequired,
		StrongApproval:   step.StrongApproval,
		Approved:         approved,
		Command:          step.Command,
		VaultChecks:      append([]VaultCheck{}, step.VaultChecks...),
	}
	if approved {
		item.ApprovalActor = approval.Actor
		item.ApprovalTime = approval.Time.Format(time.RFC3339)
		item.ApprovalSource = approval.Source
	}
	switch {
	case !step.Success && step.Retryable:
		item.Status = "retryable_failed"
		item.Reason = firstNonEmpty(step.Error, step.Stderr, "step failed and is retryable")
		item.NextAction = "fix transport/service condition, then rerun workflow or targeted step"
		if host := firstNonEmpty(step.Node, degradedWorkerFromOutput(step.Stdout)); host != "" {
			item.RepairCLI = repairCLI(host)
			item.RepairMCPCall = repairMCPCall(host)
		}
		return item, true
	case !step.Success:
		item.Status = "failed"
		item.Reason = firstNonEmpty(step.Error, step.Stderr, "step failed")
		item.NextAction = "inspect failure evidence before rerun"
		if step.SkipReason == "vault handle preflight failed" || hasMissingVaultCheck(step.VaultChecks) {
			item.Status = "vault_missing"
			item.NextAction = "import missing vault handle locally, then rerun the targeted workflow step"
			return item, true
		}
		if host := firstNonEmpty(step.Node, degradedWorkerFromOutput(step.Stdout)); host != "" && looksLikeTransportIssue(step.Error+"\n"+step.Stderr+"\n"+step.Stdout) {
			item.RepairCLI = repairCLI(host)
			item.RepairMCPCall = repairMCPCall(host)
		}
		return item, true
	case step.ApprovalRequired && step.Skipped:
		if approved {
			item.Status = "approved_ready"
			item.Reason = "approval record exists for this step"
			item.NextAction = "execute workflow or targeted step when ready"
			item.ExecuteCLI = executeCLI(step.Workflow, step.Step)
		} else {
			item.Status = "approval_pending"
			item.Reason = firstNonEmpty(step.PolicyReason, step.SkipReason, "approval required")
			item.NextAction = "record approval before execute-mode rerun"
			item.ApprovalCLI = approvalCLI("latest", step.Step, "<actor>", "<reason>")
			item.ApprovalMCP = approvalMCP(step.Step, "<actor>", "<reason>")
			item.ApprovalMCPCall = approvalMCPCall(step.Step, "<actor>", "<reason>")
			item.ExecuteCLI = executeCLI(step.Workflow, step.Step)
		}
		return item, true
	case mode == DryRun && step.Skipped && step.Command != "" && step.PolicyDecision == string(policy.Allow):
		item.Status = "ready_for_execute"
		item.Reason = "dry-run skipped an executable policy-allowed step"
		item.NextAction = "execute workflow after reviewing approval gates"
		item.ExecuteCLI = executeCLI(step.Workflow, step.Step)
		return item, true
	case step.Success && strings.TrimSpace(degradedWorkerFromOutput(step.Stdout)) != "":
		host := degradedWorkerFromOutput(step.Stdout)
		item.Status = "degraded_repair"
		item.Node = host
		item.Retryable = true
		item.Reason = firstNonEmpty(degradedReasonFromOutput(step.Stdout), "degraded worker state recorded in workflow output")
		item.NextAction = "run node repair plan before assigning this worker lane again"
		item.RepairCLI = repairCLI(host)
		item.RepairMCPCall = repairMCPCall(host)
		item.ExecuteCLI = executeCLI(step.Workflow, step.Step)
		return item, true
	default:
		return ResumeItem{}, false
	}
}

func hasMissingVaultCheck(checks []VaultCheck) bool {
	for _, check := range checks {
		if !check.Exists {
			return true
		}
	}
	return false
}

func degradedWorkerFromOutput(output string) string {
	var payload struct {
		Status string `json:"status"`
		Worker string `json:"worker"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(output)), &payload) != nil {
		return ""
	}
	if strings.EqualFold(payload.Status, "degraded") && strings.TrimSpace(payload.Worker) != "" {
		return strings.TrimSpace(payload.Worker)
	}
	return ""
}

func degradedReasonFromOutput(output string) string {
	var payload struct {
		Reason string `json:"reason"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(output)), &payload) != nil {
		return ""
	}
	return strings.TrimSpace(payload.Reason)
}

func looksLikeTransportIssue(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "i/o timeout") ||
		strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "no route") ||
		strings.Contains(lower, "auth failed") ||
		strings.Contains(lower, "protocol_mismatch")
}

func repairCLI(host string) string {
	return fmt.Sprintf("meshclaw node-repair-plan --hosts %s", shellQuote(host))
}

func repairMCPCall(host string) map[string]interface{} {
	return map[string]interface{}{
		"name":      "meshclaw_node_repair_plan",
		"arguments": map[string]interface{}{"hosts": host},
	}
}

func approvalCLI(ref, step, actor, reason string) string {
	if strings.TrimSpace(ref) == "" {
		ref = "latest"
	}
	return fmt.Sprintf("meshclaw approvals grant %s %s --actor %s --reason %s", shellQuote(ref), shellQuote(step), shellQuote(actor), shellQuote(reason))
}

func approvalMCP(step, actor, reason string) string {
	payload := approvalMCPCall(step, actor, reason)
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(data)
}

func approvalMCPCall(step, actor, reason string) map[string]interface{} {
	return map[string]interface{}{
		"name": "meshclaw_approvals_grant",
		"arguments": map[string]interface{}{
			"ref":    "latest",
			"step":   step,
			"actor":  actor,
			"reason": reason,
		},
	}
}

func executeCLI(workflow, step string) string {
	if strings.TrimSpace(step) == "" {
		return fmt.Sprintf("meshclaw run %s --execute --approvals latest", shellQuote(workflow))
	}
	return fmt.Sprintf("meshclaw run %s --execute --approvals latest --step %s", shellQuote(workflow), shellQuote(step))
}

func resumeNextActions(plan ResumePlan) []string {
	if len(plan.Items) == 0 {
		return []string{"No failed or pending steps found. The workflow evidence can be treated as complete."}
	}
	approval, approved, retryable, failed, executable := 0, 0, 0, 0, 0
	for _, item := range plan.Items {
		switch item.Status {
		case "approval_pending":
			approval++
		case "approved_ready":
			approved++
		case "retryable_failed":
			retryable++
		case "failed":
			failed++
		case "ready_for_execute":
			executable++
		}
	}
	next := []string{}
	if approval > 0 {
		next = append(next, fmt.Sprintf("Resolve %d approval-pending step(s) before execute-mode rerun.", approval))
	}
	if approved > 0 {
		next = append(next, fmt.Sprintf("Approved steps=%d. They can proceed in execute mode if policy still allows.", approved))
	}
	if retryable > 0 {
		next = append(next, fmt.Sprintf("Retryable failures=%d. Check node health and rerun after repair.", retryable))
	}
	if failed > 0 {
		next = append(next, fmt.Sprintf("Non-retryable failures=%d. Inspect evidence before rerun.", failed))
	}
	if executable > 0 {
		next = append(next, fmt.Sprintf("Dry-run found %d executable allowed step(s). Execute mode can run them after approvals are handled.", executable))
	}
	next = append(next, "Use workflow inspect for preflight policy/capability checks before rerun.")
	return next
}

func approvalRecordByStep(records []ApprovalRecord) map[string]ApprovalRecord {
	out := map[string]ApprovalRecord{}
	for _, record := range records {
		if record.Step != "" {
			out[record.Step] = record
		}
	}
	return out
}

func findExecutionStep(result Result, step string) (ExecutionResult, bool) {
	for _, item := range result.Steps {
		if item.Step == step {
			return item, true
		}
	}
	return ExecutionResult{}, false
}

func actionFromExecution(step ExecutionResult) string {
	if step.Action != "" {
		return step.Action
	}
	switch {
	case step.PolicyDecision == string(policy.RequireApproval):
		if strings.Contains(strings.ToLower(step.PolicyReason), "dns") {
			return "cloudflare_dns_change"
		}
		if strings.Contains(strings.ToLower(step.PolicyReason), "email") {
			return "email_send"
		}
		if strings.Contains(strings.ToLower(step.PolicyReason), "account") {
			return "account_configure"
		}
	}
	return "workflow_step_approval"
}

func resourceFromExecution(step ExecutionResult) string {
	if step.Resource != "" {
		return step.Resource
	}
	switch actionFromExecution(step) {
	case "cloudflare_dns_change":
		return "dns"
	case "email_send":
		return "email"
	case "account_configure":
		return "account"
	default:
		return firstNonEmpty(step.Node, "workflow")
	}
}

func writeResumePlan(plan ResumePlan) error {
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(plan.ResumePath, append(data, '\n'), 0600)
}

func workflowSteps(name string) ([]StepSpec, error) {
	if def, err := loadWorkflowDefinitionFile(name); err == nil {
		return normalizeWorkflowSteps(def.Steps), nil
	}
	if def, err := loadWorkflowDefinition(name); err == nil {
		return normalizeWorkflowSteps(def.Steps), nil
	}
	switch name {
	case "fleet-health-demo":
		return normalizeWorkflowSteps(fleetHealthDemoSteps()), nil
	case "fleet-readonly-execute-demo":
		return normalizeWorkflowSteps(fleetReadonlyExecuteDemoSteps()), nil
	case "ollama-orchestration-demo":
		return ollamaSteps(), nil
	case "email-orchestration-demo":
		return emailSteps(), nil
	case "meshclaw-runtime-why-demo":
		return runtimeWhySteps(), nil
	default:
		return nil, fmt.Errorf("unknown workflow: %s", name)
	}
}

func normalizeWorkflowSteps(steps []StepSpec) []StepSpec {
	out := make([]StepSpec, len(steps))
	copy(out, steps)
	for i := range out {
		command := strings.TrimSpace(out[i].Command)
		if strings.HasPrefix(command, "meshclaw ") {
			out[i].Command = meshclawCommand(strings.TrimSpace(strings.TrimPrefix(command, "meshclaw ")))
		}
		out[i].Command = strings.ReplaceAll(out[i].Command, "fleet-scan --security=true --hygiene=true --logs=true --json", "fleet-scan --security --hygiene --logs --json")
		if out[i].TimeoutSeconds == 0 {
			out[i].TimeoutSeconds = defaultStepTimeoutSeconds(out[i])
		}
	}
	return out
}

func defaultStepTimeoutSeconds(step StepSpec) int {
	switch step.ID {
	case "fleet-overview":
		return 45
	case "service-audit", "autoheal-plan":
		return 90
	case "security-snapshot":
		return 120
	default:
		return 0
	}
}

func IsKnown(name string) bool {
	_, err := workflowSteps(name)
	return err == nil
}

func Names() []string {
	seen := map[string]bool{}
	out := []string{
		"fleet-health-demo",
		"fleet-readonly-execute-demo",
		"ollama-orchestration-demo",
		"email-orchestration-demo",
		"meshclaw-runtime-why-demo",
	}
	for _, name := range out {
		seen[name] = true
	}
	for _, def := range loadWorkflowDefinitions() {
		if def.Name != "" && !seen[def.Name] {
			out = append(out, def.Name)
			seen[def.Name] = true
		}
	}
	sort.Strings(out)
	return out
}

func loadWorkflowDefinition(name string) (WorkflowDefinition, error) {
	for _, def := range loadWorkflowDefinitions() {
		if def.Name == name {
			return def, nil
		}
	}
	return WorkflowDefinition{}, fmt.Errorf("workflow definition not found: %s", name)
}

func loadWorkflowDefinitions() []WorkflowDefinition {
	defs := []WorkflowDefinition{}
	for _, dir := range workflowSearchDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			var def WorkflowDefinition
			if err := json.Unmarshal(data, &def); err != nil {
				continue
			}
			if def.Name == "" {
				def.Name = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			}
			def.Source = path
			defs = append(defs, def)
		}
	}
	sort.Slice(defs, func(i, j int) bool {
		return defs[i].Name < defs[j].Name
	})
	return defs
}

func workflowSearchDirs() []string {
	dirs := []string{}
	if raw := strings.TrimSpace(os.Getenv("MESHCLAW_WORKFLOW_DIR")); raw != "" {
		for _, part := range filepath.SplitList(raw) {
			if part != "" {
				dirs = append(dirs, part)
			}
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		dirs = append(dirs, filepath.Join(cwd, "workflows"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".meshclaw", "workflows"))
	}
	return dirs
}

func WorkflowInfos() []WorkflowInfo {
	names := Names()
	infos := make([]WorkflowInfo, 0, len(names))
	for _, name := range names {
		steps, err := workflowSteps(name)
		if err != nil {
			continue
		}
		infos = append(infos, WorkflowInfo{
			Name:               name,
			Description:        workflowDescription(name),
			StepCount:          len(steps),
			ApprovalActions:    approvalActions(steps),
			ExecutableAdapters: executableAdapters(steps),
			RecommendedMode:    DryRun,
		})
	}
	return infos
}

func Validate(ref string) (WorkflowValidation, error) {
	def, err := workflowDefinitionForValidation(strings.TrimSpace(ref))
	if err != nil {
		return WorkflowValidation{}, err
	}
	if def.Name == "" {
		def.Name = strings.TrimSuffix(filepath.Base(def.Source), filepath.Ext(def.Source))
	}
	result := WorkflowValidation{
		Valid:     true,
		Workflow:  def.Name,
		Source:    def.Source,
		StepCount: len(def.Steps),
	}
	addError := func(step, field, message string) {
		result.Errors = append(result.Errors, ValidationIssue{Severity: "error", Step: step, Field: field, Message: message})
	}
	addWarning := func(step, field, message string) {
		result.Warnings = append(result.Warnings, ValidationIssue{Severity: "warning", Step: step, Field: field, Message: message})
	}
	if strings.TrimSpace(def.Name) == "" {
		addError("", "name", "workflow name is required")
	}
	if len(def.Steps) == 0 {
		addError("", "steps", "workflow must contain at least one step")
	}
	seen := map[string]int{}
	for i, step := range def.Steps {
		label := firstNonEmpty(step.ID, fmt.Sprintf("index:%d", i+1))
		if strings.TrimSpace(step.ID) == "" {
			addError(label, "id", "step id is required")
		} else if prev, ok := seen[step.ID]; ok {
			addError(step.ID, "id", fmt.Sprintf("duplicate step id; first seen at index %d", prev+1))
		}
		seen[step.ID] = i
		if strings.TrimSpace(step.Title) == "" {
			addWarning(label, "title", "step title is recommended for reports and AI handoff")
		}
		adapter, ok := adapterByName(step.Transport)
		if strings.TrimSpace(step.Transport) == "" {
			addError(label, "transport", "transport is required")
		} else if !ok {
			addError(label, "transport", "unknown adapter: "+step.Transport)
		}
		if adapter.RequiresCommand && strings.TrimSpace(step.Command) == "" {
			addError(label, "command", "adapter requires command")
		}
		if strings.TrimSpace(step.Action) == "" {
			addError(label, "action", "action is required for policy evaluation")
		}
		if strings.TrimSpace(step.Resource) == "" {
			addError(label, "resource", "resource is required for policy evaluation")
		}
		if step.TimeoutSeconds < 0 {
			addError(label, "timeout_seconds", "timeout_seconds cannot be negative")
		}
		if step.Retry.MaxAttempts < 0 || step.Retry.DelaySeconds < 0 {
			addError(label, "retry", "retry values cannot be negative")
		}
		if step.Retry.MaxAttempts > 10 {
			addWarning(label, "retry.max_attempts", "max_attempts is capped at 10 during execution")
		}
		for envName, handle := range step.SecretEnv {
			if !validWorkflowEnvName(envName) {
				addError(label, "secret_env."+envName, "environment variable name is invalid")
			}
			if _, _, err := guardvault.ParseHandle(handle); err != nil {
				addError(label, "secret_env."+envName, err.Error())
			}
		}
		for _, handle := range step.VaultHandles {
			if _, _, err := guardvault.ParseHandle(handle); err != nil {
				addError(label, "vault_handles", err.Error())
			}
		}
		decision := policy.Evaluate(policy.Request{
			Subject:  "meshclaw-runtime",
			Action:   step.Action,
			Resource: step.Resource,
			Context:  step.Command,
		})
		if decision.Decision == policy.Deny {
			addError(label, "policy", "policy denies this step: "+decision.Reason)
		} else if decision.Decision == policy.RequireApproval && !step.ApprovalRequired {
			addWarning(label, "approval_required", "policy requires approval; explicit approval_required is recommended")
		}
	}
	for i, step := range def.Steps {
		label := firstNonEmpty(step.ID, fmt.Sprintf("index:%d", i+1))
		for _, dep := range step.DependsOn {
			depIndex, ok := seen[dep]
			if !ok {
				addError(label, "depends_on", "unknown dependency: "+dep)
				continue
			}
			if dep == step.ID {
				addError(label, "depends_on", "step cannot depend on itself")
			}
			if depIndex >= i {
				addError(label, "depends_on", "dependency must appear before dependent step: "+dep)
			}
		}
		for _, source := range step.FallbackFor {
			sourceIndex, ok := seen[source]
			if !ok {
				addError(label, "fallback_for", "unknown fallback source: "+source)
				continue
			}
			if source == step.ID {
				addError(label, "fallback_for", "step cannot fallback for itself")
			}
			if sourceIndex >= i {
				addError(label, "fallback_for", "fallback source must appear before fallback step: "+source)
			}
		}
	}
	result.Valid = len(result.Errors) == 0
	if result.Valid {
		result.NextActions = append(result.NextActions, "Run dry-run mode and inspect plan/evidence before execute mode.")
	} else {
		result.NextActions = append(result.NextActions, "Fix validation errors before running this workflow.")
	}
	if len(result.Warnings) > 0 {
		result.NextActions = append(result.NextActions, "Review warnings; they may make AI handoff less reliable even when execution is allowed.")
	}
	return result, nil
}

func workflowDefinitionForValidation(ref string) (WorkflowDefinition, error) {
	if ref == "" {
		return WorkflowDefinition{}, fmt.Errorf("workflow name or file path is required")
	}
	if def, err := loadWorkflowDefinitionFile(ref); err == nil {
		return def, nil
	}
	if def, err := loadWorkflowDefinition(ref); err == nil {
		return def, nil
	}
	steps, err := workflowSteps(ref)
	if err != nil {
		return WorkflowDefinition{}, err
	}
	return WorkflowDefinition{Name: ref, Steps: steps, Source: "builtin:" + ref}, nil
}

func loadWorkflowDefinitionFile(path string) (WorkflowDefinition, error) {
	if strings.TrimSpace(path) == "" {
		return WorkflowDefinition{}, fmt.Errorf("workflow file path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkflowDefinition{}, err
	}
	var def WorkflowDefinition
	if err := json.Unmarshal(data, &def); err != nil {
		return WorkflowDefinition{}, err
	}
	if def.Name == "" {
		def.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	def.Source = path
	return def, nil
}

func validWorkflowEnvName(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		if i == 0 && r >= '0' && r <= '9' {
			return false
		}
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func Inspect(name string) (WorkflowInspection, error) {
	steps, err := workflowSteps(strings.TrimSpace(name))
	if err != nil {
		return WorkflowInspection{}, err
	}
	caps := capability.List()
	info := WorkflowInfo{
		Name:               name,
		Description:        workflowDescription(name),
		StepCount:          len(steps),
		ApprovalActions:    approvalActions(steps),
		ExecutableAdapters: executableAdapters(steps),
		RecommendedMode:    DryRun,
	}
	inspection := WorkflowInspection{
		Info:               info,
		Steps:              make([]StepInspection, 0, len(steps)),
		RequiredAdapters:   executableAdapters(steps),
		RequiredNodes:      requiredNodes(steps),
		CapabilityMatches:  map[string][]string{},
		CapabilityHints:    map[string][]string{},
		CapabilitySnapshot: caps,
	}
	for i, step := range steps {
		decision := policy.Evaluate(policy.Request{
			Subject:  "meshclaw-runtime",
			Action:   step.Action,
			Resource: step.Resource,
			Context:  step.Command,
		})
		adapter, _ := adapterByName(step.Transport)
		vaultChecks := checkVaultHandles(requiredVaultHandles(step))
		approvalRequired := step.ApprovalRequired || decision.Decision == policy.RequireApproval
		if approvalRequired {
			inspection.ApprovalGates = append(inspection.ApprovalGates, ApprovalGate{
				Step:         step.ID,
				Title:        step.Title,
				Action:       step.Action,
				Resource:     step.Resource,
				Reason:       decision.Reason,
				Strong:       step.StrongApproval,
				VaultHandles: append([]string{}, requiredVaultHandles(step)...),
			})
		}
		if step.Node != "" {
			inspection.CapabilityMatches[step.Node] = matchingCapabilityIDs(step.Node, caps)
		}
		inspection.CapabilityHints[step.ID] = topRecommendationIDs(capability.Recommend(stepRecommendationIntent(step)), 3)
		inspection.Steps = append(inspection.Steps, StepInspection{
			Index:                i + 1,
			Step:                 step,
			Adapter:              adapter,
			Policy:               decision,
			VaultChecks:          vaultChecks,
			WillExecuteInDryRun:  false,
			WillExecuteInExecute: stepExecutable(step) && !approvalRequired && decision.Decision != policy.Deny && vaultChecksOK(vaultChecks),
		})
	}
	return inspection, nil
}

func checkVaultHandles(handles []string) []VaultCheck {
	if len(handles) == 0 {
		return nil
	}
	checks := make([]VaultCheck, 0, len(handles))
	for _, handle := range handles {
		check := VaultCheck{Handle: handle}
		scope, name, parseErr := guardvault.ParseHandle(handle)
		if parseErr == nil {
			check.ImportCLI = vaultImportCLI(scope, name)
		}
		entry, err := guardvault.MetadataByHandle(handle)
		if err != nil {
			check.Error = err.Error()
			check.NextAction = "Import the secret locally with guard-vault-put before execute mode; do not paste the raw value into Codex, Claude, or workflow evidence."
			checks = append(checks, check)
			continue
		}
		check.Exists = true
		check.NextAction = "Handle is available locally; execute mode may continue after policy approval."
		check.Kind = entry.Kind
		check.Description = entry.Description
		check.Fingerprint = entry.Fingerprint
		check.RawAvailable = entry.RawAvailable
		if entry.LastUsedAt != nil {
			check.LastUsedAt = entry.LastUsedAt.Format(time.RFC3339)
		}
		checks = append(checks, check)
	}
	return checks
}

func vaultImportCLI(scope, name string) string {
	return fmt.Sprintf("printf '...' | meshclaw guard-vault-put %s %s <kind> <description> --json", shellQuote(scope), shellQuote(name))
}

func requiredVaultHandles(step StepSpec) []string {
	seen := map[string]bool{}
	var out []string
	for _, handle := range step.VaultHandles {
		handle = strings.TrimSpace(handle)
		if handle != "" && !seen[handle] {
			seen[handle] = true
			out = append(out, handle)
		}
	}
	envNames := make([]string, 0, len(step.SecretEnv))
	for envName := range step.SecretEnv {
		envNames = append(envNames, envName)
	}
	sort.Strings(envNames)
	for _, envName := range envNames {
		handle := strings.TrimSpace(step.SecretEnv[envName])
		if handle != "" && !seen[handle] {
			seen[handle] = true
			out = append(out, handle)
		}
	}
	return out
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func vaultChecksOK(checks []VaultCheck) bool {
	for _, check := range checks {
		if !check.Exists {
			return false
		}
	}
	return true
}

func redactValues(value string, secrets map[string]string) string {
	if value == "" || len(secrets) == 0 {
		return value
	}
	values := make([]string, 0, len(secrets))
	for _, secret := range secrets {
		if secret != "" {
			values = append(values, secret)
		}
	}
	sort.Slice(values, func(i, j int) bool {
		return len(values[i]) > len(values[j])
	})
	for _, secret := range values {
		value = strings.ReplaceAll(value, secret, "[REDACTED_SECRET]")
	}
	return value
}

func formatSecretEnv(secretEnv map[string]string) string {
	if len(secretEnv) == 0 {
		return ""
	}
	names := make([]string, 0, len(secretEnv))
	for name := range secretEnv {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+"="+secretEnv[name])
	}
	return strings.Join(parts, "`, `")
}

func missingVaultHandleSummary(checks []VaultCheck) string {
	var missing []string
	for _, check := range checks {
		if !check.Exists {
			if check.Error != "" {
				missing = append(missing, check.Handle+" ("+check.Error+")")
			} else {
				missing = append(missing, check.Handle)
			}
		}
	}
	if len(missing) == 0 {
		return ""
	}
	return "missing required vault handle(s): " + strings.Join(missing, ", ")
}

func workflowDescription(name string) string {
	if def, err := loadWorkflowDefinitionFile(name); err == nil && def.Description != "" {
		return def.Description
	}
	if def, err := loadWorkflowDefinition(name); err == nil && def.Description != "" {
		return def.Description
	}
	switch name {
	case "fleet-health-demo":
		return "Generic first-run fleet health workflow for AI operators: inspect fleet state, services, security posture, and approval-gated safe repair without private infrastructure assumptions."
	case "fleet-readonly-execute-demo":
		return "Fast read-only execute workflow: run bounded fleet status, service audit, and security/hygiene/log scan so the evidence bundle contains real server state without mutation."
	case "ollama-orchestration-demo":
		return "Replay the Codex multi-node model orchestration demo with worker lanes, model outputs, structured failure capture, and evidence."
	case "email-orchestration-demo":
		return "Replay the email/DNS/Mox orchestration demo with approval gates for DNS changes, account configuration, and real email sends."
	case "meshclaw-runtime-why-demo":
		return "Explain why MeshClaw and vssh exist when Codex and Claude can already perform manual orchestration."
	default:
		return "MeshClaw Runtime workflow."
	}
}

func approvalActions(steps []StepSpec) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, step := range steps {
		if !step.ApprovalRequired && !step.StrongApproval {
			continue
		}
		key := step.Action
		if step.StrongApproval {
			key += ":strong"
		}
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}

func executableAdapters(steps []StepSpec) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, step := range steps {
		if !stepExecutable(step) {
			continue
		}
		if step.Transport == "" || seen[step.Transport] {
			continue
		}
		seen[step.Transport] = true
		out = append(out, step.Transport)
	}
	return out
}

func requiredNodes(steps []StepSpec) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, step := range steps {
		if step.Node == "" || seen[step.Node] {
			continue
		}
		seen[step.Node] = true
		out = append(out, step.Node)
	}
	return out
}

func matchingCapabilityIDs(node string, caps []capability.Capability) []string {
	out := []string{}
	for _, cap := range caps {
		if cap.Host == node {
			out = append(out, cap.ID)
		}
	}
	return out
}

func stepRecommendationIntent(step StepSpec) string {
	parts := []string{step.ID, step.Title, step.Action, step.Resource, step.Node, step.Transport, step.Command}
	return strings.Join(parts, " ")
}

func topRecommendationIDs(report capability.RecommendationReport, limit int) []string {
	out := []string{}
	for _, candidate := range report.Candidates {
		if candidate.Capability.ID == "" {
			continue
		}
		out = append(out, candidate.Capability.ID)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func ollamaSteps() []StepSpec {
	return []StepSpec{
		manualStep("goal", "Capture orchestration request and output target", "macbook", "Codex Desktop receives orchestration demo request and target MP4 artifact."),
		manualStep("plan", "Plan lanes and evidence", "macbook", "Split node roles, server assignments, and evidence collection plan."),
		localStep("discover-network", "Discover reachable workers and mesh status", "macbook", "/Users/example/bin/vssh list; /Users/example/bin/vssh status"),
		localStep("access-alias", "Verify alias-based remote execution path", "macbook", "/Users/example/bin/meshclaw vssh-auth-paths --hosts g1,g2,g3,g4,macmini"),
		manualStep("macmini-claude", "Confirm macmini Claude review lane", "macmini", "Claude lane is available for review without forced GUI control."),
		manualStep("review-packet-handoff", "Record review packet handoff", "macbook", "macmini sender lane and MacBook receiver lane handoff are evidence events."),
		readStep("g4-services", "Inspect g4 automation services", "g4", "command -v n8n || true; command -v ollama || true; pgrep -af 'n8n|ollama' || true"),
		readStep("g1-planner", "Run g1 planning worker", "g1", "ollama run qwen2.5:7b 'Return one JSON object with role=\"planning\" and status=\"ready\".'"),
		readStep("g2-reviewer", "Run g2 reliability reviewer", "g2", "ollama run qwen2.5:7b 'Return one JSON object with role=\"reliability\" and status=\"ready\".'"),
		readStep("g3-evidence-schema", "Run g3 evidence schema worker", "g3", "ollama run llama3.2 'Return one JSON object with role=\"evidence_schema\" and status=\"ready\".'"),
		readStep("g4-failure", "Capture g4 model failure as evidence", "g4", "ollama run gemma3:4b 'Return ready status.'"),
		manualStep("fallback", "Mark fallback routing", "macbook", "Keep g1-g3 as active workers; mark g4 failure retryable instead of hiding it."),
		manualStep("evidence-pack", "Assemble evidence pack", "macbook", "Collect role outputs, success/failure states, and report.md."),
		localStep("remotion-render", "Render briefing artifact", "macbook", "cd /Users/example/screen/tech-video && node scripts/video-script.js validate orchestration-demo && node scripts/generate-video.js orchestration-demo --lang=ko --render-only"),
		localStep("outputs", "Record final outputs", "macbook", "ls -lh /Users/example/screen/tech-video/out/orchestration-demo-ko.mp4 /Users/example/screen/tech-video/public/audio/orchestration-demo-ko.srt 2>/dev/null || true"),
		manualStep("flow-summary", "Summarize orchestration flow", "", "Plan, access, execute, evidence, render."),
		manualStep("runtime-meaning", "Explain runtime boundary", "", "Codex performs orchestration; MeshClaw makes it repeatable, auditable, and safe."),
		manualStep("what-happened", "Record actual proof", "", "Remote checks, worker calls, and generated MP4 are captured as proof."),
		manualStep("conclusion", "Close workflow", "macbook", "Plan by Codex, execution by nodes, result is the video artifact."),
	}
}

func emailSteps() []StepSpec {
	return []StepSpec{
		manualStep("intro", "Introduce email server orchestration demo", "macbook", "Mox on c1, Cloudflare DNS, and evidence-first operations."),
		manualStep("objective", "Define email workflow objective", "", "Create or verify codex mailbox, DNS, and send/receive proof."),
		manualStep("plan-doc", "Create plan document and approval gates", "", "Separate workflow steps, approval boundaries, and evidence schema."),
		{
			ID:         "mox-doc-check",
			Title:      "Check current Mox documentation before mail changes",
			Transport:  "browser",
			Action:     "documentation_check",
			Resource:   "mox-docs",
			DryRunNote: "Dry-run only: verify current Mox docs for DNS, account, TLS, and client setup expectations before mutating mail configuration.",
			Artifacts:  []string{"artifacts/docs/mox-doc-check.json", "screenshots/mox-docs-redacted.png"},
		},
		{
			ID:               "cloudflare-doc-check",
			Title:            "Check current Cloudflare DNS/token documentation",
			Transport:        "browser",
			Action:           "documentation_check",
			Resource:         "cloudflare-docs",
			ApprovalRequired: true,
			DryRunNote:       "Dry-run only: verify current Cloudflare DNS and scoped API token docs before any provider mutation.",
			Artifacts:        []string{"artifacts/docs/cloudflare-dns-token-doc-check.json", "screenshots/cloudflare-docs-redacted.png"},
		},
		{
			ID:               "cloudflare-token-policy",
			Title:            "Resolve Cloudflare token as use-only capability",
			Transport:        "policy",
			Action:           "provider_token_use",
			Resource:         "credential",
			VaultHandles:     []string{"vault://meshclaw/cloudflare/dns-token"},
			ApprovalRequired: true,
			DryRunNote:       "Dry-run only: provider tokens are looked up through vault/capability handles and never revealed in chat or evidence.",
		},
		{
			ID:               "cloudflare-dns",
			Title:            "Check Cloudflare DNS checklist",
			Transport:        "manual",
			Action:           "cloudflare_dns_change",
			Resource:         "dns",
			VaultHandles:     []string{"vault://meshclaw/cloudflare/dns-token"},
			ApprovalRequired: true,
			DryRunNote:       "Dry-run only: validate MX/SPF/DKIM/DMARC checklist before any DNS mutation.",
		},
		readStep("c1-access", "Access c1 mail server", "c1", "hostname; command -v mox || true"),
		readStep("mox-live", "Check Mox service and mail ports", "c1", "systemctl is-active mox; ss -ltn | egrep ':(25|465|587|993|443)\\b' || true"),
		readStep("account-codex", "Verify codex mailbox without secrets", "c1", "mox config account list 2>/dev/null | grep -i '^codex\\b' || true"),
		manualStep("webmail", "Verify webmail route separation", "", "Check mail.example.com webmail path and admin/user separation without exposing credentials."),
		{
			ID:         "browser-screenshot-evidence",
			Title:      "Capture webmail or browser screenshot evidence",
			Node:       "macbook",
			Transport:  "browser",
			Action:     "screenshot_capture",
			Resource:   "mail-client",
			DryRunNote: "Dry-run only: capture redacted webmail/browser screenshots into evidence/screenshots after approval and setup.",
			Artifacts:  []string{"screenshots/macbook-webmail-redacted.png"},
		},
		{
			ID:               "send-approval",
			Title:            "Gate real email send behind approval",
			Transport:        "policy",
			Action:           "email_send",
			Resource:         "email",
			ApprovalRequired: true,
			DryRunNote:       "Dry-run only: recipient, subject, body, and evidence plan require approval before send.",
		},
		{
			ID:               "client-install",
			Title:            "Prepare macmini and MacBook mail clients",
			Transport:        "manual",
			Action:           "account_configure",
			Resource:         "mail-client",
			VaultHandles:     []string{"vault://meshclaw/mail/codex-app-password"},
			ApprovalRequired: true,
			DryRunNote:       "Dry-run only: IMAP 993 and SMTP 465 setup is explained without revealing passwords or tokens.",
		},
		{
			ID:               "macmini-to-macbook",
			Title:            "Send test mail from macmini to MacBook",
			Node:             "macmini",
			Transport:        "mail",
			Action:           "email_send",
			Resource:         "email",
			VaultHandles:     []string{"vault://meshclaw/mail/macmini-sender-password"},
			ApprovalRequired: true,
			DryRunNote:       "Dry-run only: real email sending requires approval.",
			Artifacts:        []string{"screenshots/macmini-sent-mail-redacted.png", "screenshots/macbook-received-mail-redacted.png"},
		},
		{
			ID:         "mail-flow-verify",
			Title:      "Verify send/receive mail flow with screenshots",
			Node:       "macbook",
			Transport:  "manual",
			Action:     "read_state",
			Resource:   "mail-client",
			DryRunNote: "Sent mailbox, inbox, reply view, and redacted screenshots verify the real flow after approval.",
			Artifacts:  []string{"screenshots/macbook-inbox-redacted.png", "screenshots/macbook-reply-redacted.png"},
		},
		manualStep("failure-map", "Map likely mail workflow failures", "", "DNS, TLS, SMTP auth, spam rejection, and receive failure are classified as retryable/non-retryable."),
		manualStep("evidence-json", "Write structured runtime evidence", "", "Each step records success, failure, retryable, and approval_required."),
		manualStep("workflow-result", "Summarize managed email workflow", "", "Account, DNS, and send/receive result are summarized."),
		localStep("final-artifact", "Render and record final video artifact", "macbook", "cd /Users/example/screen/tech-video && node scripts/video-script.js validate email-orchestration-demo && node scripts/generate-video.js email-orchestration-demo --lang=ko --render-only && ls -lh out/email-orchestration-demo-ko.mp4"),
	}
}

func runtimeWhySteps() []StepSpec {
	videoRoot := "/Users/example/screen/tech-video"
	video := filepath.Join(videoRoot, "out", "meshclaw-runtime-why-ko.mp4")
	audio := filepath.Join(videoRoot, "public", "audio", "series", "meshclaw-runtime-why-ko.mp3")
	srt := filepath.Join(videoRoot, "public", "audio", "series", "meshclaw-runtime-why-ko.srt")
	return []StepSpec{
		manualStep("question", "Frame the product question", "macbook", "Codex and Claude can do the work; MeshClaw must explain why runtime responsibility still matters."),
		manualStep("runtime-boundary", "Separate model work from runtime work", "macbook", "Models talk and reason; MeshClaw owns policy, state, approval, execution records, and evidence."),
		manualStep("vssh-boundary", "Explain why vssh is not just SSH", "macbook", "SSH is a single shell; vssh exposes fleet facts, parallel exec, RPC-shaped results, and MCP tool schemas."),
		localStep("script-validate", "Validate combined MeshClaw Runtime script", "macbook", "cd /Users/example/screen/tech-video && node scripts/video-script.js validate meshclaw-runtime-why"),
		localStep("timeline-sync", "Sync timeline from Korean TTS subtitles", "macbook", "cd /Users/example/screen/tech-video && node scripts/video-script.js sync meshclaw-runtime-why --lang=ko --no-lang"),
		localStep("render-video", "Render combined MeshClaw Runtime video", "macbook", "cd /Users/example/screen/tech-video && node scripts/generate-video.js meshclaw-runtime-why --lang=ko --render-only"),
		localStep("verify-artifacts", "Verify rendered video and narration artifacts", "macbook", fmt.Sprintf("ls -lh %s %s %s && ffprobe -v error -show_entries format=duration -of default=nk=1:nw=1 %s && ffprobe -v error -show_entries format=duration -of default=nk=1:nw=1 %s", shellQuote(video), shellQuote(audio), shellQuote(srt), shellQuote(audio), shellQuote(video))),
		manualStep("evidence-meaning", "Record why this workflow matters", "macbook", "The evidence bundle proves MeshClaw converted a positioning narrative into repeatable execution steps and artifacts."),
	}
}

func fleetHealthDemoSteps() []StepSpec {
	return []StepSpec{
		{
			ID:             "fleet-overview",
			Title:          "Collect fleet overview",
			Node:           "controller",
			Transport:      "local",
			Command:        meshclawCommand("monitor-check --json"),
			Action:         "read_state",
			Resource:       "fleet",
			TimeoutSeconds: 45,
			Retry: RetrySpec{
				MaxAttempts:        2,
				RetryOnErrorSubstr: []string{"timeout", "connection refused"},
			},
			DryRunNote: "Dry-run only: would collect monitored fleet state.",
		},
		{
			ID:             "service-audit",
			Title:          "Audit failed or restarting services",
			Node:           "controller",
			Transport:      "local",
			Command:        meshclawCommand("fleet-service-audit --json"),
			Action:         "read_state",
			Resource:       "service",
			DependsOn:      []string{"fleet-overview"},
			TimeoutSeconds: 90,
			DryRunNote:     "Dry-run only: would audit service failures across the fleet.",
		},
		{
			ID:             "security-snapshot",
			Title:          "Collect read-only security and hygiene snapshot",
			Node:           "controller",
			Transport:      "local",
			Command:        meshclawCommand("fleet-scan --security --hygiene --logs --json"),
			Action:         "read_state",
			Resource:       "security",
			DependsOn:      []string{"fleet-overview"},
			TimeoutSeconds: 120,
			DryRunNote:     "Dry-run only: would collect security, hygiene, and log evidence without mutating hosts.",
		},
		{
			ID:             "autoheal-plan",
			Title:          "Build non-destructive autoheal plan",
			Node:           "controller",
			Transport:      "local",
			Command:        meshclawCommand("autoheal-plan --json"),
			Action:         "read_state",
			Resource:       "autoheal",
			DependsOn:      []string{"service-audit", "security-snapshot"},
			TimeoutSeconds: 90,
			DryRunNote:     "Dry-run only: would generate safe repair candidates but not apply them.",
		},
		{
			ID:               "apply-safe-gate",
			Title:            "Gate safe remediation apply",
			Transport:        "policy",
			Action:           "autoheal_apply",
			Resource:         "server",
			ApprovalRequired: true,
			DependsOn:        []string{"autoheal-plan"},
			DryRunNote:       "Applying even safe remediation requires explicit operator approval.",
		},
	}
}

func fleetReadonlyExecuteDemoSteps() []StepSpec {
	hosts := firstRunDemoHosts()
	return []StepSpec{
		{
			ID:             "fleet-overview",
			Title:          "Collect live fleet overview",
			Node:           "controller",
			Transport:      "local",
			Command:        meshclawCommand("monitor-check --json"),
			Action:         "read_state",
			Resource:       "fleet",
			TimeoutSeconds: 45,
			DryRunNote:     "Dry-run only: would collect live fleet monitor state.",
		},
		{
			ID:             "bounded-service-audit",
			Title:          "Audit failed services on representative nodes",
			Node:           "controller",
			Transport:      "local",
			Command:        meshclawCommand("fleet-service-audit --hosts " + hosts + " --parallel 4 --json"),
			Action:         "read_state",
			Resource:       "service",
			DependsOn:      []string{"fleet-overview"},
			TimeoutSeconds: 90,
			DryRunNote:     "Dry-run only: would audit failed/restarting services on representative nodes.",
		},
		{
			ID:             "bounded-security-scan",
			Title:          "Collect bounded read-only security, hygiene, and log evidence",
			Node:           "controller",
			Transport:      "local",
			Command:        meshclawCommand("fleet-scan --hosts " + hosts + " --security --hygiene --logs --parallel 4 --json"),
			Action:         "read_state",
			Resource:       "security",
			DependsOn:      []string{"fleet-overview"},
			TimeoutSeconds: 120,
			DryRunNote:     "Dry-run only: would collect bounded read-only security, hygiene, and log evidence.",
		},
	}
}

func firstRunDemoHosts() string {
	available := map[string]bool{}
	for _, node := range inventory.DefaultNodes() {
		available[node.Name] = true
	}
	preferred := []string{"c1", "c2", "d1", "g4"}
	selected := []string{}
	for _, name := range preferred {
		if available[name] {
			selected = append(selected, name)
		}
	}
	if len(selected) == 0 {
		for _, node := range inventory.DefaultNodes() {
			if node.Name != "" && len(selected) < 4 {
				selected = append(selected, node.Name)
			}
		}
	}
	return strings.Join(selected, ",")
}

func meshclawCommand(args string) string {
	binary := strings.TrimSpace(os.Getenv("MESHCLAW_BIN"))
	if binary == "" {
		if exe, err := os.Executable(); err == nil && exe != "" {
			binary = exe
		}
	}
	if binary == "" {
		binary = "meshclaw"
	}
	return shellQuote(binary) + " " + args
}

func readStep(id, title, node, command string) StepSpec {
	return StepSpec{
		ID:        id,
		Title:     title,
		Node:      node,
		Transport: "vssh",
		Command:   command,
		Action:    "read_state",
		Resource:  "server",
	}
}

func manualStep(id, title, node, note string) StepSpec {
	return StepSpec{
		ID:         id,
		Title:      title,
		Node:       node,
		Transport:  "manual",
		Action:     "read_state",
		Resource:   "workflow",
		DryRunNote: note,
	}
}

func localStep(id, title, node, command string) StepSpec {
	return StepSpec{
		ID:        id,
		Title:     title,
		Node:      node,
		Transport: "local",
		Command:   command,
		Action:    "read_state",
		Resource:  "local-workspace",
	}
}

func dependencyBlockedResult(workflow string, mode Mode, step StepSpec, completed map[string]ExecutionResult, bundleDir string) ExecutionResult {
	if mode != Execute || len(step.DependsOn) == 0 {
		return ExecutionResult{}
	}
	for _, dep := range step.DependsOn {
		dep = strings.TrimSpace(dep)
		if dep == "" {
			continue
		}
		previous, ok := completed[dep]
		if !ok {
			result := baseSkippedResult(workflow, step, "dependency_missing")
			result.Error = "dependency not found or not selected: " + dep
			result.Stdout = result.Error
			finalizeStepOutput(bundleDir, &result)
			return result
		}
		if !previous.Success || previous.Skipped || previous.Status != "ok" {
			result := baseSkippedResult(workflow, step, "dependency_blocked")
			result.Error = fmt.Sprintf("dependency %s is not ok: status=%s success=%t skipped=%t", dep, previous.Status, previous.Success, previous.Skipped)
			result.Stdout = result.Error
			finalizeStepOutput(bundleDir, &result)
			return result
		}
	}
	return ExecutionResult{}
}

func fallbackGateResult(workflow string, mode Mode, step StepSpec, completed map[string]ExecutionResult, bundleDir string) ExecutionResult {
	if mode != Execute || len(step.FallbackFor) == 0 {
		return ExecutionResult{}
	}
	triggered := false
	for _, source := range step.FallbackFor {
		source = strings.TrimSpace(source)
		if source == "" {
			continue
		}
		previous, ok := completed[source]
		if !ok {
			result := baseSkippedResult(workflow, step, "fallback_source_missing")
			result.Error = "fallback source not found or not selected: " + source
			result.Stdout = result.Error
			finalizeStepOutput(bundleDir, &result)
			return result
		}
		if !previous.Success {
			triggered = true
		}
	}
	if triggered {
		return ExecutionResult{}
	}
	result := baseSkippedResult(workflow, step, "fallback_not_needed")
	result.Success = true
	result.Stdout = "fallback not needed; source step(s) completed successfully"
	finalizeStepOutput(bundleDir, &result)
	return result
}

func baseSkippedResult(workflow string, step StepSpec, reason string) ExecutionResult {
	adapter, _ := adapterByName(step.Transport)
	recommendation := capability.Recommend(stepRecommendationIntent(step))
	return ExecutionResult{
		Success:           false,
		Workflow:          workflow,
		Step:              step.ID,
		Title:             step.Title,
		Node:              step.Node,
		Transport:         step.Transport,
		AdapterKind:       adapter.Kind,
		AdapterExecutable: adapter.Executable && (!adapter.RequiresCommand || step.Command != ""),
		AdapterReason:     adapterReason(adapter, step),
		CapabilityHints:   topRecommendationIDs(recommendation, 3),
		CapabilityClass:   recommendation.Class,
		Command:           step.Command,
		Action:            step.Action,
		Resource:          step.Resource,
		DependsOn:         append([]string{}, step.DependsOn...),
		FallbackFor:       append([]string{}, step.FallbackFor...),
		TimeoutSeconds:    step.TimeoutSeconds,
		Retry:             step.Retry,
		SecretEnv:         copyStringMap(step.SecretEnv),
		VaultHandles:      append([]string{}, requiredVaultHandles(step)...),
		VaultChecks:       checkVaultHandles(requiredVaultHandles(step)),
		ExitCode:          0,
		PolicyDecision:    string(policy.Allow),
		PolicyReason:      "dependency gate evaluated before step execution",
		Skipped:           true,
		SkipReason:        reason,
		Artifacts:         append([]string{}, step.Artifacts...),
	}
}

func runStep(workflow string, mode Mode, step StepSpec, approval ApprovalRecord, bundleDir string) ExecutionResult {
	adapter, _ := adapterByName(step.Transport)
	recommendation := capability.Recommend(stepRecommendationIntent(step))
	decision := policy.Evaluate(policy.Request{
		Subject:  "meshclaw-runtime",
		Action:   step.Action,
		Resource: step.Resource,
		Context:  step.Command,
	})
	result := ExecutionResult{
		Workflow:          workflow,
		Step:              step.ID,
		Title:             step.Title,
		Node:              step.Node,
		Transport:         step.Transport,
		AdapterKind:       adapter.Kind,
		AdapterExecutable: adapter.Executable && (!adapter.RequiresCommand || step.Command != ""),
		AdapterReason:     adapterReason(adapter, step),
		CapabilityHints:   topRecommendationIDs(recommendation, 3),
		CapabilityClass:   recommendation.Class,
		Command:           step.Command,
		Action:            step.Action,
		Resource:          step.Resource,
		DependsOn:         append([]string{}, step.DependsOn...),
		FallbackFor:       append([]string{}, step.FallbackFor...),
		TimeoutSeconds:    step.TimeoutSeconds,
		Retry:             step.Retry,
		SecretEnv:         copyStringMap(step.SecretEnv),
		VaultHandles:      append([]string{}, requiredVaultHandles(step)...),
		VaultChecks:       checkVaultHandles(requiredVaultHandles(step)),
		ExitCode:          0,
		ApprovalRequired:  step.ApprovalRequired || decision.Decision == policy.RequireApproval,
		StrongApproval:    step.StrongApproval,
		Approved:          approval.Step == step.ID,
		PolicyDecision:    string(decision.Decision),
		PolicyReason:      decision.Reason,
		Artifacts:         append([]string{}, step.Artifacts...),
	}
	if result.Approved {
		result.ApprovalActor = approval.Actor
		result.ApprovalTime = approval.Time.Format(time.RFC3339)
		result.ApprovalReason = approval.Reason
		result.ApprovalSource = approval.Source
	}
	if decision.Decision == policy.Deny {
		result.Success = false
		result.Skipped = true
		result.SkipReason = "policy denied"
		result.Error = decision.Reason
		finalizeStepOutput(bundleDir, &result)
		return result
	}
	if mode == Execute && !vaultChecksOK(result.VaultChecks) {
		result.Success = false
		result.Skipped = true
		result.SkipReason = "vault handle preflight failed"
		result.Error = missingVaultHandleSummary(result.VaultChecks)
		result.Stdout = result.Error
		finalizeStepOutput(bundleDir, &result)
		return result
	}
	if mode == DryRun {
		result.Success = true
		result.Skipped = true
		result.SkipReason = firstNonEmpty(step.DryRunNote, "dry-run: execution skipped")
		result.Stdout = result.SkipReason
		finalizeStepOutput(bundleDir, &result)
		return result
	}
	if result.ApprovalRequired && !result.Approved {
		result.Success = true
		result.Skipped = true
		result.SkipReason = "approval required before execution"
		result.Stdout = result.SkipReason
		finalizeStepOutput(bundleDir, &result)
		return result
	}
	if result.ApprovalRequired && result.Approved && !approvalCanExecute(step) {
		result.Success = true
		result.Skipped = true
		result.SkipReason = "approved but transport is not executable by this runtime adapter"
		result.Stdout = result.SkipReason
		finalizeStepOutput(bundleDir, &result)
		return result
	}
	if step.Transport == "local" && step.Command != "" {
		secretEnv, _, err := guardvault.ResolveEnv(step.SecretEnv)
		if err != nil {
			result.Success = false
			result.Skipped = true
			result.SkipReason = "secret env resolve failed"
			result.Error = err.Error()
			result.Stdout = "secret env resolve failed: " + err.Error()
			finalizeStepOutput(bundleDir, &result)
			return result
		}
		local := localCommandResult{}
		attempts := maxAttempts(step.Retry)
		for attempt := 1; attempt <= attempts; attempt++ {
			local = runLocalCommand(stepTimeout(step, workflowLocalTimeout()), step.Command, secretEnv)
			retryable := shouldRetry(step.Retry, local.exitCode, local.stderr+local.stdout) && attempt < attempts
			result.Attempts = append(result.Attempts, AttemptResult{
				Attempt:    attempt,
				Success:    local.success,
				ExitCode:   local.exitCode,
				DurationMs: local.durationMs,
				Error:      attemptError(local),
				Retryable:  retryable,
			})
			if local.success || !retryable {
				break
			}
			sleepRetryDelay(step.Retry)
		}
		result.Success = local.success
		result.Stdout = cleanOutput(redactValues(redact(local.stdout), secretEnv))
		result.Stderr = cleanOutput(redactValues(redact(local.stderr), secretEnv))
		result.ExitCode = local.exitCode
		result.DurationMs = local.durationMs
		result.Retryable = !local.success && isRetryable(local.stderr+local.stdout)
		if !local.success {
			result.Error = "local execution failed"
		}
		finalizeStepOutput(bundleDir, &result)
		return result
	}
	if step.Transport != "vssh" || step.Node == "" || step.Command == "" {
		result.Success = true
		result.Skipped = true
		result.SkipReason = firstNonEmpty(step.DryRunNote, "no executable adapter for this step")
		result.Stdout = result.SkipReason
		finalizeStepOutput(bundleDir, &result)
		return result
	}
	runner := runtime.NewRunner()
	runner.Timeout = stepTimeout(step, workflowRemoteTimeout())
	var ev runtime.Evidence
	attempts := maxAttempts(step.Retry)
	for attempt := 1; attempt <= attempts; attempt++ {
		ev = runner.RunEvidence(step.Node, step.Command)
		retryable := shouldRetry(step.Retry, ev.ExitCode, ev.Stderr+ev.Stdout) && attempt < attempts
		result.Attempts = append(result.Attempts, AttemptResult{
			Attempt:    attempt,
			Success:    ev.Success,
			ExitCode:   ev.ExitCode,
			DurationMs: ev.DurationMs,
			Error:      firstNonEmpty(ev.Stderr, ev.Stdout),
			Retryable:  retryable,
		})
		if ev.Success || !retryable {
			break
		}
		sleepRetryDelay(step.Retry)
	}
	result.Success = ev.Success
	result.Stdout = cleanOutput(redact(ev.Stdout))
	result.Stderr = cleanOutput(redact(ev.Stderr))
	result.ExitCode = ev.ExitCode
	result.DurationMs = ev.DurationMs
	result.Retryable = !ev.Success && isRetryable(ev.Stderr+ev.Stdout)
	if !ev.Success {
		result.Error = "execution failed"
	}
	finalizeStepOutput(bundleDir, &result)
	return result
}

func finalizeStepOutput(bundleDir string, result *ExecutionResult) {
	classifyExecutionResult(result)
	result.StdoutBytes = len(result.Stdout)
	result.StderrBytes = len(result.Stderr)
	limit := workflowInlineOutputLimit()
	if limit <= 0 {
		return
	}
	if len(result.Stdout) <= limit && len(result.Stderr) <= limit {
		return
	}
	if bundleDir != "" {
		rel := filepath.Join("artifacts", sanitize(result.Step)+"-output.txt")
		abs := filepath.Join(bundleDir, rel)
		var b strings.Builder
		fmt.Fprintf(&b, "workflow: %s\nstep: %s\ntitle: %s\nnode: %s\ntransport: %s\n\n", result.Workflow, result.Step, result.Title, result.Node, result.Transport)
		if result.Stdout != "" {
			b.WriteString("## stdout\n\n")
			b.WriteString(result.Stdout)
			b.WriteString("\n\n")
		}
		if result.Stderr != "" {
			b.WriteString("## stderr\n\n")
			b.WriteString(result.Stderr)
			b.WriteString("\n")
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0700); err == nil {
			err = os.WriteFile(abs, []byte(b.String()), 0600)
			if err == nil {
				result.OutputArtifact = rel
				result.Artifacts = appendIfMissing(result.Artifacts, rel)
			}
		}
	}
	if len(result.Stdout) > limit {
		result.Stdout = truncate(result.Stdout, limit)
		result.StdoutTruncated = true
	}
	if len(result.Stderr) > limit {
		result.Stderr = truncate(result.Stderr, limit)
		result.StderrTruncated = true
	}
}

func classifyExecutionResult(result *ExecutionResult) {
	if result.Status != "" {
		return
	}
	switch {
	case result.Success && !result.Skipped && len(result.FallbackFor) > 0:
		result.Status = "fallback_ok"
		result.NextAction = "continue workflow with fallback evidence; keep the original failure in the bundle"
	case result.Success && !result.Skipped:
		result.Status = "ok"
		result.NextAction = "continue workflow"
	case result.Success && result.Skipped && result.SkipReason == "fallback_not_needed":
		result.Status = "fallback_not_needed"
		result.NextAction = "continue workflow; fallback path was not needed"
	case result.Success && result.Skipped && result.ApprovalRequired && !result.Approved:
		result.Status = "approval_pending"
		result.NextAction = "record explicit approval, then rerun the targeted step"
	case result.Success && result.Skipped && result.ApprovalRequired && result.Approved:
		result.Status = "approved_not_executable"
		result.NextAction = "use the named external adapter or manual lane and attach evidence"
	case result.Success && result.Skipped && result.PolicyDecision == string(policy.Allow):
		result.Status = "dry_run_skipped"
		result.NextAction = "execute the targeted step after reviewing plan, policy, and vault preflight"
	case result.Success && result.Skipped:
		result.Status = "skipped"
		result.NextAction = "inspect skip reason before rerun"
	case !result.Success && result.SkipReason == "policy denied":
		result.Status = "policy_denied"
		result.FailureKind = "policy_denied"
		result.NextAction = "change workflow intent or policy; do not rerun unchanged"
	case !result.Success && (result.SkipReason == "vault handle preflight failed" || hasMissingVaultCheck(result.VaultChecks)):
		result.Status = "vault_missing"
		result.FailureKind = "vault_missing"
		result.NextAction = "import missing vault handle locally, then rerun the targeted step"
	case !result.Success && result.SkipReason == "secret env resolve failed":
		result.Status = "secret_env_failed"
		result.FailureKind = "secret_env_resolve_failed"
		result.NextAction = "repair the vault handle or environment variable mapping, then rerun the targeted step"
	case !result.Success && result.SkipReason == "dependency_missing":
		result.Status = "dependency_missing"
		result.FailureKind = "dependency_missing"
		result.NextAction = "include the required dependency step or remove the dependency before rerun"
	case !result.Success && result.SkipReason == "dependency_blocked":
		result.Status = "dependency_blocked"
		result.FailureKind = "dependency_blocked"
		result.NextAction = "repair or rerun the blocked dependency before executing this step"
	case !result.Success && result.SkipReason == "fallback_source_missing":
		result.Status = "fallback_source_missing"
		result.FailureKind = "fallback_source_missing"
		result.NextAction = "include the fallback source step or remove the fallback declaration before rerun"
	case !result.Success && result.Retryable:
		result.Status = "retryable_failed"
		result.FailureKind = "retryable_execution"
		result.NextAction = "repair transient transport or service state, then rerun the targeted step"
	case !result.Success:
		result.Status = "failed"
		result.FailureKind = "execution_failed"
		result.NextAction = "inspect evidence and command output before rerun"
	default:
		result.Status = "unknown"
		result.NextAction = "inspect execution.json before continuing"
	}
}

func workflowInlineOutputLimit() int {
	raw := strings.TrimSpace(os.Getenv("MESHCLAW_WORKFLOW_INLINE_OUTPUT_BYTES"))
	if raw == "" {
		return 8000
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 8000
	}
	return n
}

func appendIfMissing(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func workflowLocalTimeout() time.Duration {
	return durationFromEnv("MESHCLAW_WORKFLOW_LOCAL_TIMEOUT_SECONDS", 90*time.Second)
}

func workflowRemoteTimeout() time.Duration {
	return durationFromEnv("MESHCLAW_WORKFLOW_REMOTE_TIMEOUT_SECONDS", 15*time.Second)
}

func stepTimeout(step StepSpec, fallback time.Duration) time.Duration {
	if step.TimeoutSeconds <= 0 {
		return fallback
	}
	return time.Duration(step.TimeoutSeconds) * time.Second
}

func maxAttempts(retry RetrySpec) int {
	if retry.MaxAttempts <= 1 {
		return 1
	}
	if retry.MaxAttempts > 10 {
		return 10
	}
	return retry.MaxAttempts
}

func shouldRetry(retry RetrySpec, exitCode int, output string) bool {
	if maxAttempts(retry) <= 1 {
		return false
	}
	if len(retry.RetryOnExitCodes) == 0 && len(retry.RetryOnErrorSubstr) == 0 {
		return exitCode != 0
	}
	for _, code := range retry.RetryOnExitCodes {
		if code == exitCode {
			return true
		}
	}
	lower := strings.ToLower(output)
	for _, pattern := range retry.RetryOnErrorSubstr {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern != "" && strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

func sleepRetryDelay(retry RetrySpec) {
	if retry.DelaySeconds <= 0 {
		return
	}
	if retry.DelaySeconds > 30 {
		time.Sleep(30 * time.Second)
		return
	}
	time.Sleep(time.Duration(retry.DelaySeconds) * time.Second)
}

func attemptError(local localCommandResult) string {
	if local.success {
		return ""
	}
	return firstNonEmpty(strings.TrimSpace(local.stderr), strings.TrimSpace(local.stdout), fmt.Sprintf("exit code %d", local.exitCode))
}

func durationFromEnv(name string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

func updateSummary(summary *Summary, result ExecutionResult) {
	summary.Total++
	if result.Success {
		summary.Succeeded++
	} else {
		summary.Failed++
	}
	if result.ApprovalRequired {
		summary.ApprovalRequired++
	}
	if result.Skipped {
		summary.Skipped++
	}
	if result.Retryable {
		summary.Retryable++
	}
}

type localCommandResult struct {
	success    bool
	stdout     string
	stderr     string
	exitCode   int
	durationMs int64
}

func runLocalCommand(timeout time.Duration, command string, env map[string]string) localCommandResult {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-lc", command)
	cmd.Env = mergedEnv(env)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		killProcessGroup(cmd)
	}
	result := localCommandResult{
		success:    err == nil,
		stdout:     stdout.String(),
		stderr:     stderr.String(),
		exitCode:   0,
		durationMs: time.Since(start).Milliseconds(),
	}
	if ctx.Err() == context.DeadlineExceeded {
		result.success = false
		result.exitCode = 124
		result.stderr = fmt.Sprintf("local command timed out after %s", timeout)
		return result
	}
	if err != nil {
		result.exitCode = -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.exitCode = exitErr.ExitCode()
		}
	}
	return result
}

func mergedEnv(extra map[string]string) []string {
	env := os.Environ()
	if len(extra) == 0 {
		return env
	}
	names := make([]string, 0, len(extra))
	for name := range extra {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		env = append(env, name+"="+extra[name])
	}
	return env
}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

func createBundleDir(now time.Time, workflow string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	root := filepath.Join(home, ".meshclaw", "evidence")
	stamp := fmt.Sprintf("%s-%09d", now.Format("20060102T150405Z"), now.Nanosecond())
	dir := filepath.Join(root, now.Format("2006-01-02"), stamp+"-"+sanitize(workflow))
	if err := os.MkdirAll(filepath.Join(dir, "screenshots"), 0700); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(dir, "artifacts"), 0700); err != nil {
		return "", err
	}
	latest := filepath.Join(root, "latest")
	_ = os.Remove(latest)
	_ = os.Symlink(dir, latest)
	return dir, nil
}

func writeBundle(result Result, specs []StepSpec, approvals []ApprovalRecord) error {
	if err := os.WriteFile(result.PlanPath, []byte(renderPlan(result, specs)), 0600); err != nil {
		return err
	}
	if err := os.WriteFile(result.ReportPath, []byte(renderHTMLReport(result)), 0600); err != nil {
		return err
	}
	if err := os.WriteFile(result.ActionsPath, []byte(renderActions(result)), 0600); err != nil {
		return err
	}
	capData, err := json.MarshalIndent(map[string]interface{}{
		"path":         capability.Path(),
		"snapshot_at":  result.GeneratedAt,
		"capabilities": result.Capabilities,
	}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(result.CapabilitiesPath, append(capData, '\n'), 0600); err != nil {
		return err
	}
	adapterData, err := json.MarshalIndent(map[string]interface{}{
		"snapshot_at": result.GeneratedAt,
		"adapters":    AdapterRegistry(),
	}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(result.AdaptersPath, append(adapterData, '\n'), 0600); err != nil {
		return err
	}
	if len(approvals) > 0 {
		if err := writeApprovalRecords(result.ApprovalsPath, approvals); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(result.ExecutionPath, append(data, '\n'), 0600); err != nil {
		return err
	}
	var lines strings.Builder
	for _, step := range result.Steps {
		data, err := json.Marshal(step)
		if err != nil {
			return err
		}
		lines.Write(data)
		lines.WriteByte('\n')
	}
	return os.WriteFile(result.StepsPath, []byte(lines.String()), 0600)
}

func writeApprovalRecords(path string, approvals []ApprovalRecord) error {
	var lines strings.Builder
	for _, record := range approvals {
		data, err := json.Marshal(record)
		if err != nil {
			return err
		}
		lines.Write(data)
		lines.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(lines.String()), 0600)
}

func renderPlan(result Result, specs []StepSpec) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", result.Workflow)
	fmt.Fprintf(&b, "- mode: `%s`\n- generated_at: `%s`\n- bundle: `%s`\n- actions: `%s`\n- approvals: `%s`\n- capabilities: `%s`\n- adapters: `%s`\n\n", result.Mode, result.GeneratedAt.Format(time.RFC3339), result.BundleDir, result.ActionsPath, result.ApprovalsPath, result.CapabilitiesPath, result.AdaptersPath)
	b.WriteString("## What MeshClaw Does\n\n")
	b.WriteString("- Loads a named workflow and turns it into deterministic steps.\n")
	b.WriteString("- Snapshots the current capability registry for repeatable placement and audit.\n")
	b.WriteString("- Evaluates policy for every step before execution.\n")
	b.WriteString("- Separates dry-run, executable, manual, and approval-required actions.\n")
	b.WriteString("- Runs allowed adapters and records structured execution results.\n")
	b.WriteString("- Writes plan, steps JSONL, execution JSON, action log, HTML report, and artifact expectations.\n")
	b.WriteString("- Treats browser/client screenshots as evidence when workflows require visual proof.\n\n")
	fmt.Fprintf(&b, "## Runtime Limits\n\n- local_step_timeout: `%s`\n- remote_step_timeout: `%s`\n\n", workflowLocalTimeout(), workflowRemoteTimeout())
	b.WriteString("## Canonical AI Operator Loop\n\n")
	for _, item := range canonicalOperatorLoop() {
		fmt.Fprintf(&b, "- %s\n", item)
	}
	b.WriteString("\n")
	b.WriteString("## Steps\n\n")
	for i, step := range specs {
		adapter, _ := adapterByName(step.Transport)
		fmt.Fprintf(&b, "%d. `%s` - %s\n", i+1, step.ID, step.Title)
		fmt.Fprintf(&b, "   - node: `%s`\n   - transport: `%s`\n   - adapter_kind: `%s`\n   - adapter_executable: `%t`\n   - action: `%s`\n   - approval_required: `%t`\n", step.Node, step.Transport, adapter.Kind, adapter.Executable, step.Action, step.ApprovalRequired)
		if len(step.DependsOn) > 0 {
			fmt.Fprintf(&b, "   - depends_on: `%s`\n", strings.Join(step.DependsOn, "`, `"))
		}
		if len(step.FallbackFor) > 0 {
			fmt.Fprintf(&b, "   - fallback_for: `%s`\n", strings.Join(step.FallbackFor, "`, `"))
		}
		if step.TimeoutSeconds > 0 {
			fmt.Fprintf(&b, "   - timeout_seconds: `%d`\n", step.TimeoutSeconds)
		}
		if step.Retry.MaxAttempts > 1 {
			fmt.Fprintf(&b, "   - retry_max_attempts: `%d`\n", maxAttempts(step.Retry))
		}
		requiredHandles := requiredVaultHandles(step)
		if len(step.VaultHandles) > 0 {
			fmt.Fprintf(&b, "   - vault_handles: `%s`\n", strings.Join(step.VaultHandles, "`, `"))
		}
		if formatted := formatSecretEnv(step.SecretEnv); formatted != "" {
			fmt.Fprintf(&b, "   - secret_env: `%s`\n", formatted)
		}
		for _, check := range checkVaultHandles(requiredHandles) {
			status := "missing"
			if check.Exists {
				status = "available"
			}
			fmt.Fprintf(&b, "   - vault_preflight: `%s` %s\n", check.Handle, status)
			if check.ImportCLI != "" && !check.Exists {
				fmt.Fprintf(&b, "   - vault_import_cli: `%s`\n", check.ImportCLI)
			}
		}
		if step.Command != "" {
			fmt.Fprintf(&b, "   - command: `%s`\n", redact(step.Command))
		}
		if step.DryRunNote != "" {
			fmt.Fprintf(&b, "   - dry_run: %s\n", step.DryRunNote)
		}
		if len(step.Artifacts) > 0 {
			fmt.Fprintf(&b, "   - expected_artifacts: `%s`\n", strings.Join(step.Artifacts, "`, `"))
		}
	}
	return b.String()
}

func renderActions(result Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# MeshClaw Actions: %s\n\n", result.Workflow)
	fmt.Fprintf(&b, "- mode: `%s`\n- success: `%t`\n- generated_at: `%s`\n\n", result.Mode, result.Success, result.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "## Operator Headline\n\n%s\n\n", workflowHeadline(result))
	fmt.Fprintf(&b, "## Why This Matters\n\n%s\n\n", workflowValueStatement(result))
	b.WriteString("## Runtime Summary\n\n")
	fmt.Fprintf(&b, "- total steps: `%d`\n- succeeded: `%d`\n- failed: `%d`\n- skipped: `%d`\n- approval required: `%d`\n- retryable: `%d`\n\n",
		result.Summary.Total, result.Summary.Succeeded, result.Summary.Failed, result.Summary.Skipped, result.Summary.ApprovalRequired, result.Summary.Retryable)
	fmt.Fprintf(&b, "- capability snapshot: `%s`\n- adapter snapshot: `%s`\n- capabilities captured: `%d`\n\n", result.CapabilitiesPath, result.AdaptersPath, len(result.Capabilities))
	b.WriteString("## Canonical Loop\n\n")
	for _, item := range canonicalOperatorLoop() {
		fmt.Fprintf(&b, "- %s\n", item)
	}
	b.WriteString("\n")
	b.WriteString("## Next Actions\n\n")
	for _, action := range reportNextActions(result) {
		fmt.Fprintf(&b, "- %s\n", action)
	}
	b.WriteString("\n")
	if findings := reportFindings(result); len(findings) > 0 {
		b.WriteString("## Notable Findings\n\n")
		for _, finding := range findings {
			fmt.Fprintf(&b, "- %s\n", finding)
		}
		b.WriteString("\n")
	}
	b.WriteString("## AI Handoff\n\n")
	b.WriteString("- Treat `execution.json` as the source of truth for step status.\n")
	b.WriteString("- Treat `steps.jsonl` as the append-only execution timeline.\n")
	b.WriteString("- Approval-required steps were intentionally skipped; do not infer failure from those skips.\n")
	b.WriteString("- Continue with `meshclaw workflows resume latest --json` before executing targeted follow-up steps.\n")
	b.WriteString("- Use `meshclaw approvals grant latest <step> --actor <actor> --reason <reason>` before rerunning approval-gated steps.\n\n")
	b.WriteString("## Step Actions\n\n")
	for _, step := range result.Steps {
		status := firstNonEmpty(step.Status, legacyStepStatus(step))
		fmt.Fprintf(&b, "### `%s` - %s\n\n", step.Step, step.Title)
		fmt.Fprintf(&b, "- status: `%s`\n- node: `%s`\n- transport: `%s`\n- adapter_kind: `%s`\n- adapter_executable: `%t`\n- policy: `%s`\n- approval_required: `%t`\n", status, step.Node, step.Transport, step.AdapterKind, step.AdapterExecutable, step.PolicyDecision, step.ApprovalRequired)
		if step.CapabilityClass != "" {
			fmt.Fprintf(&b, "- capability_class: `%s`\n", step.CapabilityClass)
		}
		if len(step.CapabilityHints) > 0 {
			fmt.Fprintf(&b, "- capability_hints: `%s`\n", strings.Join(step.CapabilityHints, "`, `"))
		}
		if len(step.DependsOn) > 0 {
			fmt.Fprintf(&b, "- depends_on: `%s`\n", strings.Join(step.DependsOn, "`, `"))
		}
		if len(step.FallbackFor) > 0 {
			fmt.Fprintf(&b, "- fallback_for: `%s`\n", strings.Join(step.FallbackFor, "`, `"))
		}
		if step.TimeoutSeconds > 0 {
			fmt.Fprintf(&b, "- timeout_seconds: `%d`\n", step.TimeoutSeconds)
		}
		if step.Retry.MaxAttempts > 1 {
			fmt.Fprintf(&b, "- retry_max_attempts: `%d`\n", maxAttempts(step.Retry))
		}
		if len(step.Attempts) > 0 {
			fmt.Fprintf(&b, "- attempts: `%d`\n", len(step.Attempts))
		}
		if step.FailureKind != "" {
			fmt.Fprintf(&b, "- failure_kind: `%s`\n", step.FailureKind)
		}
		if step.NextAction != "" {
			fmt.Fprintf(&b, "- next_action: %s\n", step.NextAction)
		}
		if len(step.VaultHandles) > 0 {
			fmt.Fprintf(&b, "- vault_handles: `%s`\n", strings.Join(step.VaultHandles, "`, `"))
		}
		if formatted := formatSecretEnv(step.SecretEnv); formatted != "" {
			fmt.Fprintf(&b, "- secret_env: `%s`\n", formatted)
		}
		for _, check := range step.VaultChecks {
			status := "missing"
			if check.Exists {
				status = "available"
			}
			fmt.Fprintf(&b, "- vault_preflight: `%s` %s\n", check.Handle, status)
			if check.ImportCLI != "" && !check.Exists {
				fmt.Fprintf(&b, "- vault_import_cli: `%s`\n", check.ImportCLI)
			}
		}
		if step.AdapterReason != "" {
			fmt.Fprintf(&b, "- adapter_reason: %s\n", step.AdapterReason)
		}
		if len(step.Artifacts) > 0 {
			fmt.Fprintf(&b, "- artifacts: `%s`\n", strings.Join(step.Artifacts, "`, `"))
		}
		if step.Command != "" {
			fmt.Fprintf(&b, "- command: `%s`\n", redact(step.Command))
		}
		if step.SkipReason != "" {
			fmt.Fprintf(&b, "- skip_reason: %s\n", step.SkipReason)
		}
		if step.Error != "" {
			fmt.Fprintf(&b, "- error: %s\n", step.Error)
		}
		if step.Stdout != "" {
			fmt.Fprintf(&b, "\nstdout:\n\n```text\n%s\n```\n", truncate(step.Stdout, 2000))
		}
		if step.Stderr != "" {
			fmt.Fprintf(&b, "\nstderr:\n\n```text\n%s\n```\n", truncate(step.Stderr, 2000))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func renderHTMLReport(result Result) string {
	nextActions := htmlList(reportNextActions(result))
	findings := htmlList(reportFindings(result))
	var rows strings.Builder
	for _, step := range result.Steps {
		status := firstNonEmpty(step.Status, legacyStepStatus(step))
		fmt.Fprintf(&rows, "<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%t</td><td>%s</td><td>%t</td><td>%s</td><td><code>%s</code></td></tr>\n",
			html.EscapeString(step.Step),
			html.EscapeString(step.Title),
			html.EscapeString(step.Node),
			html.EscapeString(status),
			step.ApprovalRequired,
			html.EscapeString(step.AdapterKind),
			step.AdapterExecutable,
			html.EscapeString(strings.Join(step.Artifacts, ", ")),
			html.EscapeString(truncate(redact(step.Command), 160)),
		)
	}
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>MeshClaw Runtime Report - %s</title>
<style>
body{font-family:-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif;margin:32px;color:#15191f}
table{border-collapse:collapse;width:100%%}td,th{border-bottom:1px solid #d8dee8;text-align:left;padding:8px}code{white-space:pre-wrap}
.summary{display:flex;gap:16px;margin:16px 0}.summary div{border:1px solid #d8dee8;padding:10px 12px;border-radius:6px}
.headline{font-size:18px;line-height:1.45;border-left:4px solid #0f766e;padding:10px 14px;background:#f5fbf9}
.grid{display:grid;grid-template-columns:1fr 1fr;gap:24px}.panel{border:1px solid #d8dee8;border-radius:6px;padding:12px 16px}
</style>
</head>
<body>
<h1>%s</h1>
<p>Mode: <code>%s</code> Generated: <code>%s</code></p>
<p class="headline">%s</p>
<section class="panel"><h2>Why this matters</h2><p>%s</p></section>
<h2>What MeshClaw did</h2>
<ul>
<li>Loaded workflow <code>%s</code> and evaluated policy for every step.</li>
<li>Captured the current capability registry at <code>%s</code>.</li>
<li>Captured the runtime adapter registry at <code>%s</code>.</li>
<li>Separated AI planning from execution authority: read-only steps are allowed, mutation stays behind approval gates.</li>
<li>Selected runtime adapters such as local, vssh, manual, and policy instead of asking the model to guess raw shell behavior.</li>
<li>Executed allowed vssh/local adapters, skipped manual and approval-required actions.</li>
<li>Wrote structured evidence: plan, execution JSON, steps JSONL, action log, expected artifacts, and this report.</li>
<li>Marked browser/client screenshots as evidence artifacts when workflow steps require visual proof.</li>
<li>Applied workflow step timeouts: local <code>%s</code>, remote <code>%s</code>.</li>
</ul>
<h2>AI Handoff</h2>
<ul>
<li><code>execution.json</code> is the source of truth for step status.</li>
<li><code>steps.jsonl</code> is the append-only execution timeline.</li>
<li>Approval-required skips are intentional gates, not workflow failures.</li>
<li>Run <code>meshclaw workflows resume latest --json</code> before targeted follow-up execution.</li>
</ul>
<div class="summary">
<div>Total %d</div><div>Skipped %d</div><div>Approval %d</div><div>Failed %d</div>
</div>
<div class="grid">
<section class="panel"><h2>Next Actions</h2>%s</section>
<section class="panel"><h2>Notable Findings</h2>%s</section>
</div>
<h2>Step Timeline</h2>
<table><thead><tr><th>Step</th><th>Title</th><th>Node</th><th>Status</th><th>Approval</th><th>Adapter</th><th>Exec</th><th>Artifacts</th><th>Command</th></tr></thead><tbody>
%s
</tbody></table>
</body></html>
`, html.EscapeString(result.Workflow), html.EscapeString(result.Workflow), result.Mode, result.GeneratedAt.Format(time.RFC3339), html.EscapeString(workflowHeadline(result)), html.EscapeString(workflowValueStatement(result)), html.EscapeString(result.Workflow), html.EscapeString(result.CapabilitiesPath), html.EscapeString(result.AdaptersPath), workflowLocalTimeout(), workflowRemoteTimeout(), result.Summary.Total, result.Summary.Skipped, result.Summary.ApprovalRequired, result.Summary.Failed, nextActions, findings, rows.String())
}

func canonicalOperatorLoop() []string {
	return []string{
		"Read inventory truth before choosing nodes.",
		"Apply operator-owned inventory overrides when the user clarifies private fleet meaning.",
		"Validate and recommend capabilities before placement, model/API use, or execute mode.",
		"Run workflows in dry-run mode first.",
		"Review evidence bundle files: execution.json, steps.jsonl, meshclaw-actions.md, and report.html.",
		"Resolve approval, vault, repair, and retry blockers.",
		"Execute or resume only targeted steps after blockers are resolved.",
	}
}

func workflowHeadline(result Result) string {
	switch {
	case result.Success && result.Summary.Failed == 0 && result.Summary.ApprovalRequired > 0:
		return fmt.Sprintf("MeshClaw completed the %s run without execution failures and held %d approval-gated action(s) at the policy boundary.", result.Workflow, result.Summary.ApprovalRequired)
	case result.Success && result.Summary.Failed == 0:
		return fmt.Sprintf("MeshClaw completed the %s run successfully and wrote structured evidence for continuation.", result.Workflow)
	case result.Summary.Retryable > 0:
		return fmt.Sprintf("MeshClaw found retryable failures in %s; repair transport or service state, then resume from evidence.", result.Workflow)
	default:
		return fmt.Sprintf("MeshClaw found blocking failures in %s; inspect failed steps before rerun.", result.Workflow)
	}
}

func workflowValueStatement(result Result) string {
	return fmt.Sprintf("Without MeshClaw, an AI operator would have to infer server state from ad hoc shell output and chat history. MeshClaw turns the same operation into a repeatable runtime run: %d typed step(s), %d captured capability record(s), policy decisions for every step, explicit approval gates, and an evidence bundle that another Codex, Claude, Cursor, or local model can continue from without seeing secrets.", result.Summary.Total, len(result.Capabilities))
}

func legacyStepStatus(step ExecutionResult) string {
	if !step.Success {
		return "failed"
	}
	if step.Skipped {
		return "skipped"
	}
	return "ok"
}

func reportNextActions(result Result) []string {
	next := []string{}
	if result.Summary.Failed > 0 {
		next = append(next, "Inspect failed steps in `execution.json` and `steps.jsonl`; rerun only targeted steps after repair.")
	}
	if result.Summary.Retryable > 0 {
		next = append(next, "Run `meshclaw workflows resume latest --json` to identify retryable steps and node repair prerequisites.")
	}
	if result.Summary.ApprovalRequired > 0 {
		next = append(next, "Review approval-gated steps, then record explicit approval with `meshclaw approvals grant latest <step> --actor <actor> --reason <reason>`.")
	}
	if result.Mode == DryRun {
		next = append(next, "Use execute mode only after reviewing approval gates and runtime capability placement.")
	}
	if result.Mode == Execute && result.Summary.Failed == 0 {
		next = append(next, "Use this evidence bundle as the source of truth for the next Codex/Claude orchestration turn.")
	}
	if len(next) == 0 {
		next = append(next, "No follow-up action is required; archive the evidence bundle or attach it to the operator report.")
	}
	return next
}

func reportFindings(result Result) []string {
	findings := []string{}
	if result.Summary.Skipped > 0 {
		findings = append(findings, fmt.Sprintf("%d step(s) were skipped intentionally due to dry-run, manual adapter, unsupported adapter, or approval policy.", result.Summary.Skipped))
	}
	if result.Summary.ApprovalRequired > 0 {
		findings = append(findings, fmt.Sprintf("%d approval gate(s) prevented mutation during this run; the model receives the decision, not unchecked authority.", result.Summary.ApprovalRequired))
	}
	if len(result.Capabilities) > 0 {
		findings = append(findings, fmt.Sprintf("%d capability record(s) were snapshotted so later turns can reason about the same fleet/API/model state.", len(result.Capabilities)))
	}
	if artifacts := outputArtifacts(result); len(artifacts) > 0 {
		findings = append(findings, fmt.Sprintf("Large command output was moved to artifact files: %s.", strings.Join(artifacts, ", ")))
	}
	for _, step := range result.Steps {
		if step.OutputArtifact != "" || step.StdoutTruncated || step.StderrTruncated {
			continue
		}
		if strings.Contains(strings.ToLower(step.Stdout), `"status":"degraded"`) || strings.Contains(strings.ToLower(step.Stdout), `"status": "degraded"`) {
			findings = append(findings, fmt.Sprintf("Step `%s` recorded degraded worker state without failing the workflow.", step.Step))
		}
		if !step.Success {
			findings = append(findings, fmt.Sprintf("Step `%s` failed on node `%s`: %s.", step.Step, step.Node, firstNonEmpty(step.Error, step.Stderr, "unknown error")))
		}
	}
	if len(findings) == 0 {
		findings = append(findings, "No blocking failures were recorded.")
	}
	return findings
}

func outputArtifacts(result Result) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, step := range result.Steps {
		if step.OutputArtifact != "" && !seen[step.OutputArtifact] {
			seen[step.OutputArtifact] = true
			out = append(out, "`"+step.OutputArtifact+"`")
		}
	}
	return out
}

func htmlList(items []string) string {
	if len(items) == 0 {
		return "<p>None.</p>"
	}
	var b strings.Builder
	b.WriteString("<ul>")
	for _, item := range items {
		fmt.Fprintf(&b, "<li>%s</li>", html.EscapeString(item))
	}
	b.WriteString("</ul>")
	return b.String()
}

func LatestBundle() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(home, ".meshclaw", "evidence", "latest")
	target, err := filepath.EvalSymlinks(path)
	if err == nil {
		return target, nil
	}
	return path, nil
}

func sanitize(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "workflow"
	}
	return out
}

func redact(value string) string {
	patterns := []string{"password=", "token=", "secret=", "dkim_private_key=", "api_key="}
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lower := strings.ToLower(line)
		for _, pattern := range patterns {
			if idx := strings.Index(lower, pattern); idx >= 0 {
				lines[i] = line[:idx+len(pattern)] + "[REDACTED]"
				break
			}
		}
	}
	return strings.Join(lines, "\n")
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func cleanOutput(value string) string {
	value = ansiPattern.ReplaceAllString(value, "")
	value = strings.ReplaceAll(value, "\r", "")
	lines := strings.Split(value, "\n")
	cleaned := lines[:0]
	for _, line := range lines {
		if strings.TrimSpace(line) == "" && len(cleaned) > 0 && strings.TrimSpace(cleaned[len(cleaned)-1]) == "" {
			continue
		}
		cleaned = append(cleaned, line)
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

func isRetryable(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "temporarily") ||
		strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "eof") ||
		strings.Contains(lower, "500 internal server error") ||
		strings.Contains(lower, "model failed to load") ||
		strings.Contains(lower, "resource limitations")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max] + "..."
}
