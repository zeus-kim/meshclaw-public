package reconciler

import "testing"

func TestDiffNodeDetectsOfflineBeforeOtherActions(t *testing.T) {
	result := DiffNode(
		DesiredNode{ID: "s1", Roles: []string{"storage"}, Services: map[string]string{"vsshd": "running"}},
		ActualNode{ID: "s1", Online: false, EvidenceID: "ev1"},
		DiffOptions{},
	)
	if result.Matched {
		t.Fatalf("result should not match: %+v", result)
	}
	if len(result.Actions) != 1 {
		t.Fatalf("actions = %d, want 1: %+v", len(result.Actions), result.Actions)
	}
	action := result.Actions[0]
	if action.Kind != "diagnose_offline_node" || action.Severity != "critical" || !action.Retryable {
		t.Fatalf("bad offline action: %+v", action)
	}
}

func TestDiffNodeDetectsServiceAndCapacityDrift(t *testing.T) {
	minFree := 20.0
	maxMem := 80.0
	diskUsed := 88.0
	memUsed := 91.0
	result := DiffNode(
		DesiredNode{
			ID:               "g4",
			Roles:            []string{"openwebui-worker", "ollama-worker"},
			Services:         map[string]string{"open-webui": "running"},
			MinDiskFreePct:   &minFree,
			MaxMemoryUsedPct: &maxMem,
		},
		ActualNode{
			ID:            "g4",
			Online:        true,
			Roles:         []string{"openwebui-worker"},
			Services:      map[string]string{"open-webui": "failed"},
			DiskUsedPct:   &diskUsed,
			MemoryUsedPct: &memUsed,
			EvidenceID:    "ev-g4",
		},
		DiffOptions{},
	)
	if result.Matched {
		t.Fatalf("result should not match: %+v", result)
	}
	kinds := map[string]bool{}
	for _, action := range result.Actions {
		kinds[action.Kind] = true
		if len(action.EvidenceRefs) != 1 || action.EvidenceRefs[0] != "ev-g4" {
			t.Fatalf("missing evidence ref in action: %+v", action)
		}
	}
	for _, want := range []string{"service_drift", "capacity_drift", "inventory_drift"} {
		if !kinds[want] {
			t.Fatalf("missing action kind %q in %+v", want, result.Actions)
		}
	}
}

func TestDiffNodeDetectsContainerDrift(t *testing.T) {
	result := DiffNode(
		DesiredNode{
			ID: "g4",
			Containers: map[string]DesiredContainer{
				"api":    {Desired: "running", Image: "api:v2", Health: "healthy", Restart: "approval_required"},
				"worker": {Desired: "running"},
			},
		},
		ActualNode{
			ID:     "g4",
			Online: true,
			Containers: map[string]ActualContainer{
				"api": {Image: "api:v1", State: "running", Health: "healthy"},
			},
			EvidenceID: "ev-g4",
		},
		DiffOptions{},
	)
	if result.Matched {
		t.Fatalf("result should not match: %+v", result)
	}
	actions := map[string]Action{}
	for _, action := range result.Actions {
		actions[action.ID] = action
	}
	if actions["g4:container:api"].PolicyAction != "container_pull_redeploy" {
		t.Fatalf("expected image drift pull/redeploy action: %+v", actions["g4:container:api"])
	}
	if actions["g4:container:api"].Metadata["container"] != "api" || actions["g4:container:api"].Metadata["desired_image"] != "api:v2" {
		t.Fatalf("expected container metadata on action: %+v", actions["g4:container:api"].Metadata)
	}
	if actions["g4:container:api"].Metadata["desired_restart"] != "approval_required" {
		t.Fatalf("expected restart policy metadata on action: %+v", actions["g4:container:api"].Metadata)
	}
	if actions["g4:container:worker"].PolicyAction != "container_recreate" || !actions["g4:container:worker"].ApprovalRequired {
		t.Fatalf("expected missing container recreate action: %+v", actions["g4:container:worker"])
	}
}

func TestDiffNodeDetectsHealthOnlyContainerDrift(t *testing.T) {
	result := DiffNode(
		DesiredNode{
			ID: "g4",
			Containers: map[string]DesiredContainer{
				"api": {Health: "healthy"},
			},
		},
		ActualNode{
			ID:     "g4",
			Online: true,
			Containers: map[string]ActualContainer{
				"api": {State: "running", Image: "api:v1", Health: "unhealthy"},
			},
			EvidenceID: "ev-g4",
		},
		DiffOptions{},
	)
	if result.Matched || len(result.Actions) != 1 {
		t.Fatalf("health-only drift should produce one action: %+v", result)
	}
	action := result.Actions[0]
	if action.PolicyAction != "container_restart" || action.Severity != "high" || !action.ApprovalRequired {
		t.Fatalf("bad health-only drift action: %+v", action)
	}
	if action.Metadata["desired_health"] != "healthy" || action.Metadata["actual_health"] != "unhealthy" {
		t.Fatalf("bad health metadata: %+v", action.Metadata)
	}
}

func TestDiffNodeDetectsRestartPolicyContainerDrift(t *testing.T) {
	result := DiffNode(
		DesiredNode{
			ID: "g4",
			Containers: map[string]DesiredContainer{
				"api": {Restart: "unless-stopped"},
			},
		},
		ActualNode{
			ID:     "g4",
			Online: true,
			Containers: map[string]ActualContainer{
				"api": {State: "running", Image: "api:v1", RestartPolicy: "no"},
			},
			EvidenceID: "ev-g4",
		},
		DiffOptions{},
	)
	if result.Matched || len(result.Actions) != 1 {
		t.Fatalf("restart policy drift should produce one action: %+v", result)
	}
	action := result.Actions[0]
	if action.PolicyAction != "container_restart" || action.Severity != "medium" || !action.ApprovalRequired {
		t.Fatalf("bad restart policy drift action: %+v", action)
	}
	if action.Metadata["desired_restart"] != "unless-stopped" || action.Metadata["actual_restart"] != "no" {
		t.Fatalf("bad restart metadata: %+v", action.Metadata)
	}
}

func TestDiffNodeTreatsPresentContainerAsExists(t *testing.T) {
	result := DiffNode(
		DesiredNode{
			ID: "g4",
			Containers: map[string]DesiredContainer{
				"api": {Desired: "present"},
			},
		},
		ActualNode{
			ID:     "g4",
			Online: true,
			Containers: map[string]ActualContainer{
				"api": {State: "stopped", Image: "api:v1"},
			},
			EvidenceID: "ev-g4",
		},
		DiffOptions{},
	)
	if !result.Matched || len(result.Actions) != 0 {
		t.Fatalf("present container should match any existing state: %+v", result)
	}
}

func TestDiffNodeDetectsMissingPresentContainer(t *testing.T) {
	result := DiffNode(
		DesiredNode{
			ID: "g4",
			Containers: map[string]DesiredContainer{
				"api": {Desired: "present"},
			},
		},
		ActualNode{
			ID:         "g4",
			Online:     true,
			Containers: map[string]ActualContainer{},
			EvidenceID: "ev-g4",
		},
		DiffOptions{},
	)
	if result.Matched || len(result.Actions) != 1 {
		t.Fatalf("missing present container should produce one action: %+v", result)
	}
	action := result.Actions[0]
	if action.PolicyAction != "container_recreate" || action.Metadata["desired_state"] != "present" || action.Metadata["actual_state"] != "missing" {
		t.Fatalf("bad present container action: %+v", action)
	}
}

func TestDiffNodeMatchedWhenDesiredEqualsActual(t *testing.T) {
	minFree := 20.0
	maxMem := 80.0
	diskUsed := 30.0
	memUsed := 40.0
	result := DiffNode(
		DesiredNode{
			ID:               "g4",
			Roles:            []string{"openwebui-worker"},
			Services:         map[string]string{"open-webui": "running"},
			Containers:       map[string]DesiredContainer{"open-webui": {Desired: "running", Image: "web:v1", Health: "healthy"}},
			MinDiskFreePct:   &minFree,
			MaxMemoryUsedPct: &maxMem,
		},
		ActualNode{
			ID:            "g4",
			Online:        true,
			Roles:         []string{"openwebui-worker"},
			Services:      map[string]string{"open-webui": "active"},
			Containers:    map[string]ActualContainer{"open-webui": {State: "running", Image: "web:v1", Health: "healthy"}},
			DiskUsedPct:   &diskUsed,
			MemoryUsedPct: &memUsed,
		},
		DiffOptions{},
	)
	if !result.Matched || len(result.Actions) != 0 {
		t.Fatalf("expected match without actions: %+v", result)
	}
}
