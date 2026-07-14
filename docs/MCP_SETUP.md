# MCP Setup

MeshClaw is not a chat assistant. Codex, Claude, Cursor, Open WebUI, or a local
LLM talks to the user. MeshClaw exposes server state, policy decisions, safe
execution, cleanup plans, workflow runs, and evidence as tools.

## Binary

Install from PyPI:

```sh
pip install -U meshclaw vssh
meshclaw --install-binary
meshclaw --print-binary
```

Build locally:

```sh
git clone https://github.com/zeus-kim/meshclaw.git
cd meshclaw
go test ./cmd/meshclaw ./internal/monitor
go build -o ./bin/meshclaw ./cmd/meshclaw
./bin/meshclaw --help
```

Optionally install the locally built binary somewhere stable on your controller
machine:

```sh
mkdir -p /Users/example/bin
cp ./bin/meshclaw /Users/example/bin/meshclaw
```

Run the MCP server:

```sh
/Users/example/bin/meshclaw mcp
```

Verify the local setup before connecting or debugging an AI client:

```sh
meshclaw quickstart --json
meshclaw setup-check --json
meshclaw doctor --json
meshclaw run fleet-readonly-execute-demo --execute --json
```

`quickstart` runs the first 5 minutes path in one safe command: setup doctor,
workflow inspection, dry-run evidence bundle, and next actions. `setup-check`
and `doctor` report the effective MeshClaw binary, the effective VSSH binary,
stale VSSH binaries elsewhere on PATH, whether a VSSH secret source is
configured, and which Codex/Claude/Cursor MCP config files mention MeshClaw.
The read-only execute demo is the first live check: it gathers bounded real
server evidence without mutating hosts.

## Codex/Claude/Cursor MCP Command

Use this command for the MCP server:

```text
meshclaw mcp
```

For a pinned local binary, use `/Users/example/bin/meshclaw mcp` or set
`MESHCLAW_BIN=/Users/example/bin/meshclaw` before launching the PyPI wrapper.

Recommended environment when vssh-native is enabled:

```text
MESHCLAW_VSSH_BINARY=/Users/example/bin/vssh
VSSH_SECRET=<same secret used by vsshd on the fleet>
```

Codex config example:

```toml
[mcp_servers.meshclaw]
command = "/Users/example/bin/meshclaw"
args = ["mcp"]

[mcp_servers.meshclaw.env]
MESHCLAW_VSSH_BINARY = "/Users/example/bin/vssh"
VSSH_SECRET = "..."
```

Claude Desktop / Cursor JSON shape:

```json
{
  "mcpServers": {
    "meshclaw": {
      "command": "/Users/example/bin/meshclaw",
      "args": ["mcp"],
      "env": {
        "MESHCLAW_VSSH_BINARY": "/Users/example/bin/vssh",
        "VSSH_SECRET": "..."
      }
    }
  }
}
```

Without these variables MeshClaw can still start, but native remote execution
may fall back or fail depending on node configuration. MeshClaw uses:

- vssh-native first execution over Tailscale/private routes
- `/Users/example/.meshclaw/evidence` for local evidence records
- policy from `~/.meshclaw/policy.json` or `MESHCLAW_POLICY_FILE`
- capabilities from `~/.meshclaw/capabilities.json` or
  `MESHCLAW_CAPABILITY_FILE`

Default execution requires `Tailscale/private route + vssh server + VSSH_SECRET`.
SSH remains a compatibility fallback for nodes without vssh daemon coverage.

## Desktop Operating Model

Use the MacBook as the interactive controller when Codex, Claude, or Cursor is
running there. This mode works while the MacBook is awake and is the right
place for browser work, screenshots, and human review.

Use the Mac mini as the always-on worker when MeshClaw needs continuous checks,
Open WebUI/Ollama services, Matrix/Open WebUI bridge processes, or unattended
evidence collection. The same MCP tools can still be called from Codex/Claude
on the MacBook; the long-running processes do not need to live in the same
place as the human chat UI.

## Canonical Tools

- `meshclaw_ai_guide`
- `meshclaw_quickstart`
- `meshclaw_doctor`
- `meshclaw_tool_recommend`
- `meshclaw_workflow_list`
- `meshclaw_workflow_inspect`
- `meshclaw_workflow_run`
- `meshclaw_workflow_plan_execute`
- `meshclaw_workflow_resume`
- `meshclaw_evidence_latest`
- `meshclaw_ops_control`
- `meshclaw_inventory_override_list`
- `meshclaw_inventory_override_set`
- `meshclaw_inventory_override_remove`
- `meshclaw_capability_list`
- `meshclaw_capability_validate`
- `meshclaw_capability_recommend`
- `meshclaw_autoheal_plan`
- `meshclaw_autoheal_apply_safe`
- `meshclaw_service_triage`
- `meshclaw_service_check`
- `meshclaw_disk_investigate`
- `meshclaw_data_clean_plan`
- `meshclaw_guard_modes`
- `meshclaw_guard_model`
- `meshclaw_guard_session_policy`
- `meshclaw_guard_signal_policy`
- `meshclaw_messenger_report`
- `meshclaw_messenger_approval_request`
- `meshclaw_messenger_targets`
- `meshclaw_signal_rooms_doctor`
- `meshclaw_signal_rooms_cleanup`
- `meshclaw_messenger_target_add`
- `meshclaw_messenger_send_report`
- `meshclaw_messenger_send_approval_request`
- `meshclaw_guard_vault`
- `meshclaw_guard_vault_list`
- `meshclaw_guard_vault_metadata`
- `meshclaw_guard_detect`
- `meshclaw_guard_redact`
- `meshclaw_guard_posture`
- `meshclaw_guard_vuln`
- `meshclaw_guard_vuln_scan`
- `meshclaw_guard_code_review`
- `meshclaw_policy_check`
- `meshclaw_run_evidence`

Tool names must use alphanumeric characters and underscores only. Do not use
legacy dotted names such as `meshclaw.autoheal_plan`.

## First MCP Workflow

When an AI client has just connected, use this sequence before any mutating
operation:

1. Read `meshclaw_architecture`.
2. Read `meshclaw_mcp_surface`.
3. Inspect inventory, capabilities, and workspace state.
4. Run a dry-run workflow or read-only diagnostic.
5. Read the generated evidence.
6. If a step is blocked by approval, build or send a redacted approval request.
7. Record approval only when the human explicitly approves.
8. Resume the selected approved step with `approvals_ref`.

Approval is not a conversational guess. It is a MeshClaw policy/workflow state.
If a tool returns `require_approval`, the model should not improvise around it.
It should send a redacted request through `meshclaw_messenger_send_approval_request`
or tell the user which approval call is needed.

Signal/Argos is the notification and local assistant surface. Codex and Claude
remain the primary high-agency MCP clients. Do not route Codex/Claude CLI
sessions through Signal; instead, store work results in MeshClaw evidence and
let Signal/local models summarize or deliver those results on demand.

Detailed tool sequence:

1. Call `meshclaw_ai_guide` or `meshclaw_mcp_surface`.
2. Call `meshclaw_server_list` to read current inventory truth.
3. Call `meshclaw_inventory_override_list`; if the user clarifies private fleet
   meaning, call `meshclaw_inventory_override_set`.
4. Call `meshclaw_capability_validate` if placement, model/API selection, or
   execute mode depends on local capabilities.
5. Call `meshclaw_capability_recommend` when the model needs to choose a
   node, model, API, storage lane, mail server, or provisioner for an intent.
6. Call `meshclaw_workflow_list`.
7. For user-authored workflows or files under `examples/workflows`, call
   `meshclaw_workflow_validate` first.
8. Start with `meshclaw_workflow_run` using
   `{"name":"fleet-health-demo","mode":"dry-run"}`.
9. Call `meshclaw_evidence_latest` and read `execution.json`,
   `steps.jsonl`, and `meshclaw-actions.md`.
10. Call `meshclaw_workflow_plan_execute` before execute mode; it returns
   `ready`, approval blockers, vault blockers, repair blockers,
   capability-registry validity, and the exact execute call when safe to
   continue. It also returns `recommended_mcp`; follow those structured tool
   calls before guessing from prose.
11. Use `meshclaw_workflow_resume` if any step is failed, retryable,
   degraded, or approval-pending.

When the model is unsure which tool to use, call `meshclaw_mcp_surface` or
`meshclaw_tool_recommend` before running low-level tools. The default product
path is workflow/policy/evidence first; direct vssh is for transport debugging
or low-level primitives.

Use `meshclaw-ops-orchestration-demo` only when the operator wants the full
private fleet/email/model orchestration narrative. It is the advanced demo, not
the default OSS smoke test.

## Operating Rule

Read-only and evidence-producing tools are allowed by default. Mutating actions
must be represented as plan/apply flows:

- `meshclaw_ops_control`, `meshclaw_fleet_scan`,
  `meshclaw_service_triage`, `meshclaw_autoheal_plan`,
  `meshclaw_node_repair_plan`, and `meshclaw_workflow_plan_execute` return
  `recommended_mcp`. AI clients should follow those structured calls instead
  of inferring the next action from human prose.
- `meshclaw_autoheal_plan` returns `policy_decision` and `approval_required`
  for every action.
- `meshclaw_autoheal_apply_safe` executes only `mode=auto_safe`,
  `policy_decision=allow`, `approval_required=false` actions.
- `meshclaw_data_clean_plan` creates a path manifest plus structured JSONL
  sidecar with category, risk, size, and reason.
- `meshclaw_data_clean_apply` applies only a generated manifest and remains
  approval-gated by policy.
- service quarantine/remove actions require approval and post-action evidence.

Policy is loaded from `~/.meshclaw/policy.json`, or `MESHCLAW_POLICY_FILE`.
Configured rules are evaluated before built-in defaults.

## Claude/Codex Use

Claude and Codex should use MeshClaw MCP when they need shared operational
truth:

- current fleet state
- service/log/security evidence
- workload placement
- safe remediation candidates
- policy decisions
- evidence-backed remote execution

They should not use MeshClaw as a conversation model. The model talks to the
user; MeshClaw answers tool calls.
