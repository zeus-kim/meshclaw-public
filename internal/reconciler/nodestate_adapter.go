package reconciler

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/meshclaw/meshclaw/internal/nodestate"
)

func LoadActualNodeReport(path string) (ActualNode, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ActualNode{}, err
	}
	var report nodestate.Report
	if err := json.Unmarshal(data, &report); err != nil {
		return ActualNode{}, err
	}
	return ActualNodeFromReport(report, path), nil
}

func ActualNodeFromReport(report nodestate.Report, evidenceID string) ActualNode {
	actual := ActualNode{
		ID:            firstNonEmpty(report.NodeName, report.Hostname),
		Online:        true,
		Roles:         roleList(report.Inventory),
		Tags:          cleanList(report.Inventory.Tags),
		Services:      serviceMap(report.Services),
		Containers:    containerMap(report.Docker),
		DiskUsedPct:   percentPtr(report.System.DiskPct),
		MemoryUsedPct: percentPtr(report.System.MemoryPct),
		EvidenceID:    strings.TrimSpace(evidenceID),
	}
	return actual
}

func roleList(inventory nodestate.InventoryHint) []string {
	var roles []string
	if inventory.PrimaryRole != "" {
		roles = append(roles, inventory.PrimaryRole)
	}
	roles = append(roles, inventory.SecondaryRoles...)
	return cleanList(roles)
}

func serviceMap(services []nodestate.ServiceState) map[string]string {
	if len(services) == 0 {
		return nil
	}
	out := map[string]string{}
	for _, service := range services {
		name := strings.TrimSpace(service.Name)
		if name == "" {
			continue
		}
		state := firstNonEmpty(service.Active, service.State)
		if state == "" {
			state = "unknown"
		}
		out[name] = state
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func containerMap(docker nodestate.DockerState) map[string]ActualContainer {
	if len(docker.Containers) == 0 {
		return nil
	}
	out := map[string]ActualContainer{}
	for _, container := range docker.Containers {
		name := strings.TrimSpace(container.Name)
		if name == "" {
			continue
		}
		out[name] = ActualContainer{
			Image:         strings.TrimSpace(container.Image),
			State:         strings.TrimSpace(container.State),
			Health:        strings.TrimSpace(container.HealthStatus),
			RestartPolicy: strings.TrimSpace(container.RestartPolicy),
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func percentPtr(value float64) *float64 {
	if value <= 0 {
		return nil
	}
	out := value
	return &out
}

func cleanList(values []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value != "" && !seen[key] {
			seen[key] = true
			out = append(out, value)
		}
	}
	return out
}
