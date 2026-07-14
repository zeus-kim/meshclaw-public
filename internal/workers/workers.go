package workers

import (
	"os"
	"strings"
)

type Worker struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Subject      string   `json:"subject,omitempty"`
	Status       string   `json:"status"`
	Description  string   `json:"description"`
	Capabilities []string `json:"capabilities"`
	Policy       string   `json:"policy"`
	Surface      string   `json:"surface"`
}

func DefaultWorkers() []Worker {
	return []Worker{
		{
			ID:           "codex",
			Kind:         "model_operator",
			Subject:      "codex",
			Status:       "external_app",
			Description:  "Codex app or Codex CLI acts as a natural-language DevOps operator.",
			Capabilities: []string{"planning", "coding", "tool_use", "mcp"},
			Policy:       "devops preset allows read/evidence tools; raw execution remains policy-gated",
			Surface:      "Codex app / CLI / optional Matrix room",
		},
		{
			ID:           "claude",
			Kind:         "model_operator",
			Subject:      "claude",
			Status:       "external_app",
			Description:  "Claude acts as a natural-language reviewer, planner, and MCP operator.",
			Capabilities: []string{"planning", "review", "analysis", "mcp"},
			Policy:       "devops preset allows read/evidence tools; raw execution remains policy-gated",
			Surface:      "Claude app / optional Matrix room",
		},
		{
			ID:           "local-llm",
			Kind:         "model_operator",
			Subject:      "local-llm",
			Status:       statusFromEnv("MESHCLAW_LOCAL_LLM", "optional"),
			Description:  "Local model operator through Open WebUI, Ollama, LM Studio, or another MCP-capable bridge.",
			Capabilities: []string{"private_analysis", "log_summary", "mcp"},
			Policy:       "devops preset allows read/evidence tools and approval-gates mutating actions",
			Surface:      "Open WebUI / Ollama / local MCP bridge / optional Matrix room",
		},
		{
			ID:           "openwebui",
			Kind:         "model_frontend",
			Subject:      "openwebui",
			Status:       statusFromEnv("MESHCLAW_OPENWEBUI", "optional"),
			Description:  "Local or private model frontend that can be wired to MeshClaw MCP.",
			Capabilities: []string{"chat", "local_models", "mcp_bridge"},
			Policy:       "same as local-llm unless overridden in policy.json",
			Surface:      "Open WebUI",
		},
		{
			ID:           "matrix-room",
			Kind:         "ops_room",
			Status:       statusFromEnv("MESHCLAW_MATRIX_ROOM", matrixRoomStatus()),
			Description:  "Shared room for humans, friends, models, approvals, notifications, and evidence summaries.",
			Capabilities: []string{"shared_context", "approval", "notifications", "handoff"},
			Policy:       "Matrix bridge must call MeshClaw policy before execution",
			Surface:      "Matrix",
		},
		{
			ID:           "meshclaw-mcp",
			Kind:         "control_plane",
			Status:       "available",
			Description:  "MCP server exposing server state, policy, evidence, workflows, and vssh-backed execution.",
			Capabilities: []string{"server_list", "fleet_scan", "policy_check", "run_evidence", "evidence"},
			Policy:       "source of truth for execution policy",
			Surface:      "meshclaw mcp",
		},
		{
			ID:           "vssh-daemon",
			Kind:         "execution_worker",
			Status:       "available",
			Description:  "Native remote execution daemon and RPC substrate over Tailscale/private routes.",
			Capabilities: []string{"run_many", "rpc_many", "facts_many", "jobs", "artifact_collect"},
			Policy:       "called through MeshClaw policy for AI operators",
			Surface:      "vssh server / vssh mcp",
		},
	}
}

func matrixRoomStatus() string {
	if _, err := os.Stat(os.ExpandEnv("$HOME/.meshclaw/matrix.json")); err == nil {
		return "connected"
	}
	return "planned"
}

func statusFromEnv(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}
