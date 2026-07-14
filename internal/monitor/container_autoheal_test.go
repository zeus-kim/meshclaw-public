package monitor

import (
	"strings"
	"testing"

	"github.com/meshclaw/meshclaw/internal/nodestate"
)

func TestContainerHealthPlanProposesRestartActions(t *testing.T) {
	actions := ContainerHealthPlan("g4", nodestate.DockerState{
		Available: true,
		Containers: []nodestate.DockerContainer{{
			Name:         "api",
			State:        "running",
			HealthStatus: "unhealthy",
		}, {
			Name:     "worker",
			State:    "exited",
			ExitCode: 137,
		}, {
			Name:         "db",
			State:        "running",
			HealthStatus: "healthy",
		}},
	})
	if len(actions) != 2 {
		t.Fatalf("actions = %d, want 2: %+v", len(actions), actions)
	}
	for _, action := range actions {
		if action.Type != "container_restart" || action.Mode != "propose" {
			t.Fatalf("container plan must stay propose-only: %+v", action)
		}
		if action.Command == "" || action.Verify == "" || action.Container == "" {
			t.Fatalf("missing command/verify/container: %+v", action)
		}
		if !strings.Contains(action.Verify, "meshclaw analyze-logs 'g4' container:'"+action.Container+"'") {
			t.Fatalf("missing container logscan verification hint: %+v", action)
		}
	}
	if actions[0].Severity != "high" || actions[0].Container != "api" {
		t.Fatalf("bad unhealthy action: %+v", actions[0])
	}
	if actions[1].Value != 137 {
		t.Fatalf("exit code should be preserved as value: %+v", actions[1])
	}
}

func TestContainerHealthPlanEscapesContainerName(t *testing.T) {
	actions := ContainerHealthPlan("g4", nodestate.DockerState{
		Available: true,
		Containers: []nodestate.DockerContainer{{
			Name:         "bad'name",
			HealthStatus: "unhealthy",
		}},
	})
	if len(actions) != 1 {
		t.Fatalf("actions = %+v", actions)
	}
	if actions[0].Command != "docker restart 'bad'\"'\"'name'" {
		t.Fatalf("container name was not shell escaped: %s", actions[0].Command)
	}
}

func TestContainerHealthPlanSkipsUnavailableDocker(t *testing.T) {
	if actions := ContainerHealthPlan("g4", nodestate.DockerState{}); len(actions) != 0 {
		t.Fatalf("expected no actions for unavailable docker: %+v", actions)
	}
}
