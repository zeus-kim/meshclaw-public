package reconciler

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type DesiredState struct {
	SchemaVersion string                     `json:"schema_version,omitempty"`
	Nodes         []DesiredNode              `json:"nodes"`
	ParseFindings []DesiredValidationFinding `json:"parse_findings,omitempty"`
}

type DesiredValidationFinding struct {
	Severity string `json:"severity"`
	NodeID   string `json:"node_id,omitempty"`
	Field    string `json:"field,omitempty"`
	Message  string `json:"message"`
}

type desiredStateYAML struct {
	SchemaVersion string                     `yaml:"schema_version"`
	Nodes         map[string]desiredNodeYAML `yaml:"nodes"`
}

type desiredNodeYAML struct {
	Roles      []string                      `yaml:"roles"`
	Tags       []string                      `yaml:"tags"`
	Services   map[string]serviceStateYAML   `yaml:"services"`
	Containers map[string]containerStateYAML `yaml:"containers"`
	Capacity   capacityYAML                  `yaml:"capacity"`
}

type serviceStateYAML struct {
	Value   string
	Desired string `yaml:"desired"`
}

type containerStateYAML struct {
	Value   string
	Desired string `yaml:"desired"`
	Image   string `yaml:"image"`
	Health  string `yaml:"health"`
	Restart string `yaml:"restart"`
}

type capacityYAML struct {
	AllowModelJobs   *bool    `yaml:"allow_model_jobs"`
	MinDiskFreePct   *float64 `yaml:"min_disk_free_pct"`
	MaxMemoryUsedPct *float64 `yaml:"max_memory_used_pct"`
}

func (s *serviceStateYAML) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		s.Value = strings.TrimSpace(value.Value)
		return nil
	case yaml.MappingNode:
		type alias serviceStateYAML
		var out alias
		if err := value.Decode(&out); err != nil {
			return err
		}
		*s = serviceStateYAML(out)
		return nil
	default:
		return fmt.Errorf("service desired state must be a scalar or mapping")
	}
}

func (c *containerStateYAML) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		c.Value = strings.TrimSpace(value.Value)
		return nil
	case yaml.MappingNode:
		type alias containerStateYAML
		var out alias
		if err := value.Decode(&out); err != nil {
			return err
		}
		*c = containerStateYAML(out)
		return nil
	default:
		return fmt.Errorf("container desired state must be a scalar or mapping")
	}
}

func LoadDesiredState(path string) (DesiredState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return DesiredState{}, err
	}
	return ParseDesiredStateYAML(data)
}

func ParseDesiredStateYAML(data []byte) (DesiredState, error) {
	var raw desiredStateYAML
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return DesiredState{}, err
	}
	if len(raw.Nodes) == 0 {
		return DesiredState{}, fmt.Errorf("desired state must define at least one node")
	}
	names := make([]string, 0, len(raw.Nodes))
	for name := range raw.Nodes {
		name = strings.TrimSpace(name)
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	state := DesiredState{
		SchemaVersion: strings.TrimSpace(raw.SchemaVersion),
		Nodes:         make([]DesiredNode, 0, len(names)),
		ParseFindings: desiredStateIgnoredApplyFindings(data),
	}
	for _, name := range names {
		node := raw.Nodes[name]
		desired := DesiredNode{
			ID:               name,
			Roles:            cleanStrings(node.Roles),
			Tags:             cleanStrings(node.Tags),
			Services:         cleanServices(node.Services),
			Containers:       cleanContainers(node.Containers),
			AllowModelJobs:   node.Capacity.AllowModelJobs,
			MinDiskFreePct:   node.Capacity.MinDiskFreePct,
			MaxMemoryUsedPct: node.Capacity.MaxMemoryUsedPct,
		}
		state.Nodes = append(state.Nodes, desired)
	}
	if len(state.Nodes) == 0 {
		return DesiredState{}, fmt.Errorf("desired state nodes must have non-empty ids")
	}
	return state, nil
}

func ValidateDesiredState(state DesiredState) []DesiredValidationFinding {
	findings := append([]DesiredValidationFinding{}, state.ParseFindings...)
	if !isAllowedDesiredSchemaVersion(state.SchemaVersion) {
		findings = append(findings, DesiredValidationFinding{Severity: "warning", Field: "schema_version", Message: "desired state schema_version should be v1, v2, or v3"})
	}
	seenNodes := map[string]struct{}{}
	for _, node := range state.Nodes {
		nodeID := strings.TrimSpace(node.ID)
		if nodeID == "" {
			findings = append(findings, DesiredValidationFinding{Severity: "critical", Field: "nodes", Message: "desired node has empty id"})
			continue
		}
		if _, ok := seenNodes[nodeID]; ok {
			findings = append(findings, DesiredValidationFinding{Severity: "critical", NodeID: nodeID, Field: "nodes", Message: "desired node id is duplicated"})
		}
		seenNodes[nodeID] = struct{}{}
		findings = append(findings, validateUniqueStrings(nodeID, "roles", node.Roles)...)
		findings = append(findings, validateUniqueStrings(nodeID, "tags", node.Tags)...)
		if len(node.Roles) == 0 && len(node.Tags) == 0 && len(node.Services) == 0 && len(node.Containers) == 0 && node.AllowModelJobs == nil && node.MinDiskFreePct == nil && node.MaxMemoryUsedPct == nil {
			findings = append(findings, DesiredValidationFinding{Severity: "warning", NodeID: nodeID, Field: "node", Message: "desired node has no roles, tags, services, containers, or capacity policy"})
		}
		for service, desired := range node.Services {
			if strings.TrimSpace(service) == "" {
				findings = append(findings, DesiredValidationFinding{Severity: "critical", NodeID: nodeID, Field: "services", Message: "service name is empty"})
			}
			if strings.TrimSpace(desired) == "" {
				findings = append(findings, DesiredValidationFinding{Severity: "critical", NodeID: nodeID, Field: "services", Message: "service desired state is empty"})
			}
			if strings.TrimSpace(desired) != "" && !isAllowedDesiredServiceState(desired) {
				findings = append(findings, DesiredValidationFinding{Severity: "warning", NodeID: nodeID, Field: "services.desired", Message: "service desired state should be running, stopped, active, inactive, enabled, disabled, present, or absent"})
			}
		}
		for name, container := range node.Containers {
			if strings.TrimSpace(name) == "" {
				findings = append(findings, DesiredValidationFinding{Severity: "critical", NodeID: nodeID, Field: "containers", Message: "container name is empty"})
			}
			if strings.TrimSpace(container.Desired) == "" && strings.TrimSpace(container.Image) == "" && strings.TrimSpace(container.Health) == "" && strings.TrimSpace(container.Restart) == "" {
				findings = append(findings, DesiredValidationFinding{Severity: "critical", NodeID: nodeID, Field: "containers", Message: "container desired state, image, health, or restart is required"})
			}
			if strings.TrimSpace(container.Health) != "" && !isAllowedDesiredContainerHealth(container.Health) {
				findings = append(findings, DesiredValidationFinding{Severity: "warning", NodeID: nodeID, Field: "containers.health", Message: "container health should be healthy, unhealthy, none, or unknown"})
			}
			if strings.TrimSpace(container.Desired) != "" && !isAllowedDesiredContainerState(container.Desired) {
				findings = append(findings, DesiredValidationFinding{Severity: "warning", NodeID: nodeID, Field: "containers.desired", Message: "container desired state should be running, stopped, absent, or present"})
			}
			if strings.EqualFold(strings.TrimSpace(container.Desired), "absent") && (strings.TrimSpace(container.Image) != "" || strings.TrimSpace(container.Health) != "") {
				findings = append(findings, DesiredValidationFinding{Severity: "warning", NodeID: nodeID, Field: "containers.absent", Message: "absent containers should not also specify image or health"})
			}
			if strings.TrimSpace(container.Restart) != "" && !isAllowedDesiredContainerRestart(container.Restart) {
				findings = append(findings, DesiredValidationFinding{Severity: "warning", NodeID: nodeID, Field: "containers.restart", Message: "container restart should be always, unless-stopped, on-failure, no, manual, or approval_required"})
			}
		}
		if node.MinDiskFreePct != nil && (*node.MinDiskFreePct < 0 || *node.MinDiskFreePct > 100) {
			findings = append(findings, DesiredValidationFinding{Severity: "critical", NodeID: nodeID, Field: "capacity.min_disk_free_pct", Message: "min disk free percent must be between 0 and 100"})
		}
		if node.MaxMemoryUsedPct != nil && (*node.MaxMemoryUsedPct < 0 || *node.MaxMemoryUsedPct > 100) {
			findings = append(findings, DesiredValidationFinding{Severity: "critical", NodeID: nodeID, Field: "capacity.max_memory_used_pct", Message: "max memory used percent must be between 0 and 100"})
		}
	}
	if len(seenNodes) == 0 {
		findings = append(findings, DesiredValidationFinding{Severity: "critical", Field: "nodes", Message: "desired state must define at least one node"})
	}
	return findings
}

func desiredStateIgnoredApplyFindings(data []byte) []DesiredValidationFinding {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil
	}
	if len(root.Content) == 0 {
		return nil
	}
	var findings []DesiredValidationFinding
	collectIgnoredApplyKeys(root.Content[0], "", "", &findings)
	return findings
}

func collectIgnoredApplyKeys(node *yaml.Node, path, nodeID string, findings *[]DesiredValidationFinding) {
	if node == nil {
		return
	}
	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]
			key := strings.TrimSpace(keyNode.Value)
			nextPath := key
			if path != "" {
				nextPath = path + "." + key
			}
			nextNodeID := nodeID
			if strings.HasPrefix(nextPath, "nodes.") && strings.Count(nextPath, ".") == 1 {
				nextNodeID = key
			}
			if isIgnoredDesiredApplyKey(key) {
				*findings = append(*findings, DesiredValidationFinding{
					Severity: "warning",
					NodeID:   nextNodeID,
					Field:    nextPath,
					Message:  "desired-state YAML key " + key + " is ignored and does not grant apply, execute, or approval",
				})
			}
			collectIgnoredApplyKeys(valueNode, nextPath, nextNodeID, findings)
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			collectIgnoredApplyKeys(child, path, nodeID, findings)
		}
	}
}

func isIgnoredDesiredApplyKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "apply", "execute", "auto_apply", "auto-apply", "approve", "approved", "approved_by":
		return true
	default:
		return false
	}
}

func isAllowedDesiredSchemaVersion(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "v1", "v2", "v3":
		return true
	default:
		return false
	}
}

func validateUniqueStrings(nodeID, field string, values []string) []DesiredValidationFinding {
	var findings []DesiredValidationFinding
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			findings = append(findings, DesiredValidationFinding{Severity: "critical", NodeID: nodeID, Field: field, Message: field + " contains an empty value"})
			continue
		}
		if _, ok := seen[value]; ok {
			findings = append(findings, DesiredValidationFinding{Severity: "warning", NodeID: nodeID, Field: field, Message: field + " contains a duplicate value: " + value})
		}
		seen[value] = struct{}{}
	}
	return findings
}

func cleanServices(values map[string]serviceStateYAML) map[string]string {
	if len(values) == 0 {
		return nil
	}
	services := map[string]string{}
	for name, state := range values {
		name = strings.TrimSpace(name)
		desired := strings.TrimSpace(firstNonEmpty(state.Desired, state.Value))
		if name != "" && desired != "" {
			services[name] = desired
		}
	}
	if len(services) == 0 {
		return nil
	}
	return services
}

func cleanContainers(values map[string]containerStateYAML) map[string]DesiredContainer {
	if len(values) == 0 {
		return nil
	}
	containers := map[string]DesiredContainer{}
	for name, state := range values {
		name = strings.TrimSpace(name)
		container := DesiredContainer{
			Desired: strings.TrimSpace(firstNonEmpty(state.Desired, state.Value)),
			Image:   strings.TrimSpace(state.Image),
			Health:  strings.TrimSpace(state.Health),
			Restart: strings.TrimSpace(state.Restart),
		}
		if name != "" && (container.Desired != "" || container.Image != "" || container.Health != "" || container.Restart != "") {
			containers[name] = container
		}
	}
	if len(containers) == 0 {
		return nil
	}
	return containers
}

func isAllowedDesiredContainerState(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "running", "stopped", "absent", "present":
		return true
	default:
		return false
	}
}

func isAllowedDesiredServiceState(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "running", "stopped", "active", "inactive", "enabled", "disabled", "present", "absent":
		return true
	default:
		return false
	}
}

func isAllowedDesiredContainerHealth(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "healthy", "unhealthy", "none", "unknown":
		return true
	default:
		return false
	}
}

func isAllowedDesiredContainerRestart(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "always", "unless-stopped", "on-failure", "no", "manual", "approval_required":
		return true
	default:
		return false
	}
}

func cleanStrings(values []string) []string {
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
