package nodestate

import "testing"

func TestClassifyProcessPurpose(t *testing.T) {
	cases := map[string]string{
		"/usr/bin/ollama runner --model gemma4:e4b": "llm_inference",
		"open-webui serve":                          "ai_chat_ui",
		"/usr/bin/vsshd --listen :48291":            "remote_execution",
		"python -m uvicorn app:app":                 "app_runtime",
		"/usr/sbin/sshd -D":                         "ssh_access",
	}
	for command, want := range cases {
		if got := classifyProcessPurpose(command); got != want {
			t.Fatalf("classifyProcessPurpose(%q) = %q, want %q", command, got, want)
		}
	}
}

func TestSummarizeWorkloads(t *testing.T) {
	workloads := summarizeWorkloads(
		[]ProcessState{
			{CPUPct: 45, MemPct: 12, Purpose: classifyProcessPurpose("ollama runner"), Command: "ollama runner"},
			{CPUPct: 3, MemPct: 2, Purpose: classifyProcessPurpose("open-webui serve"), Command: "open-webui serve"},
		},
		DockerState{
			Available: true,
			Running:   1,
			Containers: []DockerContainer{{
				Name:  "web",
				State: "running",
				Ports: "0.0.0.0:8080->8080/tcp",
			}},
			Ports: []DockerPort{{Container: "web", Mapping: "0.0.0.0:8080->8080/tcp", Public: true}},
		},
		NetworkState{Listeners: []NetworkListener{{
			Protocol: "tcp",
			Address:  "0.0.0.0",
			Port:     "8080",
			Process:  "web",
			Public:   true,
		}}},
	)
	if !hasWorkloadPurpose(workloads, "llm_inference") {
		t.Fatalf("missing llm_inference workload: %#v", workloads)
	}
	if !hasWorkloadPurpose(workloads, "containerized_app") {
		t.Fatalf("missing containerized_app workload: %#v", workloads)
	}
	if !hasWorkloadPurpose(workloads, "public_network_service") {
		t.Fatalf("missing public_network_service workload: %#v", workloads)
	}
}

func hasWorkloadPurpose(values []WorkloadState, purpose string) bool {
	for _, value := range values {
		if value.Purpose == purpose {
			return true
		}
	}
	return false
}
