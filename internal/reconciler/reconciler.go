package reconciler

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Reconciler compares approved desired operations intent with observed node
// state and returns bounded actions. Implementations must be deterministic and
// policy-aware; mutation is a later layer, not part of basic diffing.
type Reconciler interface {
	Reconcile(ctx context.Context, nodeID string) (Result, error)
}

type Result struct {
	NodeID       string        `json:"node_id"`
	Matched      bool          `json:"matched"`
	RequeueAfter time.Duration `json:"requeue_after"`
	Actions      []Action      `json:"actions"`
	EvidenceID   string        `json:"evidence_id,omitempty"`
}

type Action struct {
	ID               string            `json:"id"`
	Kind             string            `json:"kind"`
	NodeID           string            `json:"node_id"`
	Severity         string            `json:"severity"`
	Summary          string            `json:"summary"`
	PolicyAction     string            `json:"policy_action"`
	PolicyResource   string            `json:"policy_resource,omitempty"`
	PolicyDecision   string            `json:"policy_decision,omitempty"`
	PolicyReason     string            `json:"policy_reason,omitempty"`
	ApprovalRequired bool              `json:"approval_required"`
	Retryable        bool              `json:"retryable"`
	EvidenceRefs     []string          `json:"evidence_refs,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

type DesiredNode struct {
	ID               string                      `json:"id"`
	Roles            []string                    `json:"roles,omitempty"`
	Tags             []string                    `json:"tags,omitempty"`
	Services         map[string]string           `json:"services,omitempty"`
	Containers       map[string]DesiredContainer `json:"containers,omitempty"`
	AllowModelJobs   *bool                       `json:"allow_model_jobs,omitempty"`
	MinDiskFreePct   *float64                    `json:"min_disk_free_pct,omitempty"`
	MaxMemoryUsedPct *float64                    `json:"max_memory_used_pct,omitempty"`
}

type DesiredContainer struct {
	Desired string `json:"desired,omitempty"`
	Image   string `json:"image,omitempty"`
	Health  string `json:"health,omitempty"`
	Restart string `json:"restart,omitempty"`
}

type ActualNode struct {
	ID            string                     `json:"id"`
	Online        bool                       `json:"online"`
	Roles         []string                   `json:"roles,omitempty"`
	Tags          []string                   `json:"tags,omitempty"`
	Services      map[string]string          `json:"services,omitempty"`
	Containers    map[string]ActualContainer `json:"containers,omitempty"`
	DiskUsedPct   *float64                   `json:"disk_used_pct,omitempty"`
	MemoryUsedPct *float64                   `json:"memory_used_pct,omitempty"`
	EvidenceID    string                     `json:"evidence_id,omitempty"`
}

type ActualContainer struct {
	Image         string `json:"image,omitempty"`
	State         string `json:"state,omitempty"`
	Health        string `json:"health,omitempty"`
	RestartPolicy string `json:"restart_policy,omitempty"`
}

type DiffOptions struct {
	DefaultRequeueAfter time.Duration
}

func DiffNode(desired DesiredNode, actual ActualNode, opts DiffOptions) Result {
	requeue := opts.DefaultRequeueAfter
	if requeue <= 0 {
		requeue = 5 * time.Minute
	}
	result := Result{NodeID: firstNonEmpty(desired.ID, actual.ID), Matched: true, RequeueAfter: requeue}
	if actual.EvidenceID != "" {
		result.EvidenceID = actual.EvidenceID
	}
	if result.NodeID == "" {
		result.Matched = false
		result.Actions = append(result.Actions, Action{
			ID:           "missing-node-id",
			Kind:         "invalid_desired_state",
			Severity:     "high",
			Summary:      "desired and actual node state are both missing node id",
			PolicyAction: "deny",
			Retryable:    false,
		})
		return result
	}
	if !actual.Online {
		result.Matched = false
		result.Actions = append(result.Actions, Action{
			ID:           result.NodeID + ":node-offline",
			Kind:         "diagnose_offline_node",
			NodeID:       result.NodeID,
			Severity:     "critical",
			Summary:      fmt.Sprintf("%s is offline or missing from the latest agent state", result.NodeID),
			PolicyAction: "read_only_diagnosis",
			Retryable:    true,
			EvidenceRefs: refs(actual.EvidenceID),
		})
		return result
	}
	for _, role := range missingItems(desired.Roles, actual.Roles) {
		result.Matched = false
		result.Actions = append(result.Actions, Action{
			ID:           result.NodeID + ":missing-role:" + role,
			Kind:         "inventory_drift",
			NodeID:       result.NodeID,
			Severity:     "medium",
			Summary:      fmt.Sprintf("%s is desired to have role %q, but the latest state does not show it", result.NodeID, role),
			PolicyAction: "plan_only",
			Retryable:    false,
			EvidenceRefs: refs(actual.EvidenceID),
		})
	}
	for service, desiredState := range desired.Services {
		actualState := strings.TrimSpace(actual.Services[service])
		if actualState == "" {
			actualState = "missing"
		}
		if normalizeState(actualState) == normalizeState(desiredState) {
			continue
		}
		result.Matched = false
		approval := desiredState == "running" || desiredState == "active"
		result.Actions = append(result.Actions, Action{
			ID:               result.NodeID + ":service:" + service,
			Kind:             "service_drift",
			NodeID:           result.NodeID,
			Severity:         serviceSeverity(desiredState, actualState),
			Summary:          fmt.Sprintf("%s service %q desired=%s actual=%s", result.NodeID, service, desiredState, actualState),
			PolicyAction:     "service_check",
			ApprovalRequired: approval,
			Retryable:        true,
			EvidenceRefs:     refs(actual.EvidenceID),
		})
	}
	for name, desiredContainer := range desired.Containers {
		action, ok := containerDriftAction(result.NodeID, name, desiredContainer, actual.Containers[name], actual.EvidenceID)
		if !ok {
			continue
		}
		result.Matched = false
		result.Actions = append(result.Actions, action)
	}
	if desired.MinDiskFreePct != nil && actual.DiskUsedPct != nil {
		free := 100 - *actual.DiskUsedPct
		if free < *desired.MinDiskFreePct {
			result.Matched = false
			result.Actions = append(result.Actions, Action{
				ID:           result.NodeID + ":disk-free",
				Kind:         "capacity_drift",
				NodeID:       result.NodeID,
				Severity:     "high",
				Summary:      fmt.Sprintf("%s disk free %.1f%% is below desired minimum %.1f%%", result.NodeID, free, *desired.MinDiskFreePct),
				PolicyAction: "disk_investigate",
				Retryable:    true,
				EvidenceRefs: refs(actual.EvidenceID),
			})
		}
	}
	if desired.MaxMemoryUsedPct != nil && actual.MemoryUsedPct != nil {
		if *actual.MemoryUsedPct > *desired.MaxMemoryUsedPct {
			result.Matched = false
			result.Actions = append(result.Actions, Action{
				ID:           result.NodeID + ":memory-used",
				Kind:         "capacity_drift",
				NodeID:       result.NodeID,
				Severity:     "medium",
				Summary:      fmt.Sprintf("%s memory used %.1f%% is above desired maximum %.1f%%", result.NodeID, *actual.MemoryUsedPct, *desired.MaxMemoryUsedPct),
				PolicyAction: "process_top",
				Retryable:    true,
				EvidenceRefs: refs(actual.EvidenceID),
			})
		}
	}
	sort.SliceStable(result.Actions, func(i, j int) bool {
		return actionRank(result.Actions[i]) < actionRank(result.Actions[j])
	})
	return result
}

func containerDriftAction(nodeID, name string, desired DesiredContainer, actual ActualContainer, evidenceID string) (Action, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Action{}, false
	}
	rawDesiredState := strings.ToLower(strings.TrimSpace(desired.Desired))
	desiredState := normalizeContainerState(desired.Desired)
	actualState := normalizeContainerState(actual.State)
	if actualState == "" {
		actualState = "missing"
	}
	actualImage := strings.TrimSpace(actual.Image)
	desiredImage := strings.TrimSpace(desired.Image)
	desiredHealth := strings.ToLower(strings.TrimSpace(desired.Health))
	actualHealth := strings.ToLower(strings.TrimSpace(actual.Health))
	desiredRestart := strings.ToLower(strings.TrimSpace(desired.Restart))
	actualRestart := strings.ToLower(strings.TrimSpace(actual.RestartPolicy))
	stateDrift := desiredState != "" && desiredState != actualState
	if rawDesiredState == "present" && actualState != "missing" {
		stateDrift = false
	}
	metadataDesiredState := desiredState
	if rawDesiredState == "present" {
		metadataDesiredState = "present"
	}

	policyAction := "container_restart"
	severity := "medium"
	reason := ""
	switch {
	case stateDrift:
		reason = fmt.Sprintf("desired=%s actual=%s", metadataDesiredState, actualState)
		if desiredState == "running" && actualState == "missing" {
			severity = "high"
			policyAction = "container_recreate"
		}
		if rawDesiredState == "absent" {
			policyAction = "container_recreate"
		}
	case desiredImage != "" && actualImage != "" && desiredImage != actualImage:
		reason = fmt.Sprintf("desired image=%s actual image=%s", desiredImage, actualImage)
		policyAction = "container_pull_redeploy"
	case desiredImage != "" && actualImage == "":
		reason = fmt.Sprintf("desired image=%s actual image=missing", desiredImage)
		policyAction = "container_recreate"
	case desiredHealth != "" && desiredHealth != "unknown" && actualHealth != "" && desiredHealth != actualHealth:
		reason = fmt.Sprintf("desired health=%s actual health=%s", desiredHealth, actualHealth)
		severity = "high"
	case desiredRestart != "" && actualRestart != "" && desiredRestart != actualRestart:
		reason = fmt.Sprintf("desired restart=%s actual restart=%s", desiredRestart, actualRestart)
	default:
		return Action{}, false
	}
	metadata := cleanMetadata(map[string]string{
		"container":       name,
		"desired_state":   metadataDesiredState,
		"actual_state":    actualState,
		"desired_image":   desiredImage,
		"actual_image":    actualImage,
		"desired_health":  desiredHealth,
		"actual_health":   actualHealth,
		"desired_restart": desiredRestart,
		"actual_restart":  actualRestart,
	})
	return Action{
		ID:               nodeID + ":container:" + name,
		Kind:             "container_drift",
		NodeID:           nodeID,
		Severity:         severity,
		Summary:          fmt.Sprintf("%s container %q drift: %s", nodeID, name, reason),
		PolicyAction:     policyAction,
		ApprovalRequired: true,
		Retryable:        true,
		EvidenceRefs:     refs(evidenceID),
		Metadata:         metadata,
	}, true
}

func cleanMetadata(values map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range values {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeContainerState(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "running", "up", "active", "present":
		return "running"
	case "stopped", "exited", "dead", "inactive", "down":
		return "stopped"
	case "absent", "missing":
		return "missing"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func missingItems(want, got []string) []string {
	seen := map[string]bool{}
	for _, item := range got {
		seen[strings.ToLower(strings.TrimSpace(item))] = true
	}
	var missing []string
	for _, item := range want {
		key := strings.ToLower(strings.TrimSpace(item))
		if key != "" && !seen[key] {
			missing = append(missing, item)
		}
	}
	return missing
}

func normalizeState(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "active", "running", "up":
		return "running"
	case "inactive", "stopped", "down":
		return "stopped"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func serviceSeverity(desired, actual string) string {
	if normalizeState(desired) == "running" && (actual == "missing" || normalizeState(actual) == "stopped") {
		return "high"
	}
	return "medium"
}

func actionRank(action Action) int {
	switch action.Severity {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	default:
		return 3
	}
}

func refs(values ...string) []string {
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
