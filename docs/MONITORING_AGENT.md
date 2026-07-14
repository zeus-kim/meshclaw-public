# Monitoring Agent

MeshClaw's monitoring agent is not a conversational assistant.

It is a long-running server-operations sensor:

```text
monitor-agent
  -> discover inventory/Tailscale nodes
  -> collect fleet state
  -> detect alerts
  -> optionally scan redacted hygiene findings
  -> store evidence
  -> let Codex/Claude/local LLM decide the next action through MCP
```

## Command

```sh
meshclaw monitor-agent 5m
meshclaw monitor-agent 10m --hygiene
```

The interval argument is optional and accepts Go duration syntax such as `30s`,
`1m`, or `5m`. Intervals below `10s` are refused.

`--hygiene` adds a conservative remote scan for likely log/config leaks. The
remote command redacts matched values before evidence is stored.

## Role

The monitor agent owns observation, not conversation.

It now has two complementary collection paths:

- central controller: fleet-wide monitor and evidence writer
- node-local sensor: `meshclaw agent collect --json`
- MCP/CLI: on-demand status and remediation workflows

## Direction

The preferred execution substrate is:

```text
Tailscale/private route -> vssh server -> structured evidence
```

SSH remains a compatibility fallback only while a node does not have vssh
native daemon coverage.

The controller loop is:

```text
observe -> detect -> plan -> policy gate -> apply -> verify -> evidence
```

This is the MeshClaw equivalent of a Kubernetes controller loop, but aimed at
mixed real servers rather than only container workloads.

The reconciliation controller is the next layer above monitoring. Monitoring
answers "what is true now?" Reconciliation answers "does that match the
approved operational intent, and what should happen next?" The design is
documented in [`RECONCILIATION_CONTROLLER.md`](RECONCILIATION_CONTROLLER.md).

`fleet-scan` is the on-demand batch form of the same idea. It runs monitor,
security, log, and redacted hygiene checks across selected hosts and stores one
evidence bundle that Codex, Claude, or a local MCP client can inspect.

## Node-Local Sensor

`meshclaw agent collect --json` is the first small node-local sensor command.
It is intentionally read-only and does not expose arbitrary command execution.

It collects:

- OS, architecture, CPU count, load, memory, disk, uptime
- GPU summary when `nvidia-smi` is available
- Docker running/stopped/image counts when Docker is available
- Docker containers and public port mappings
- network interfaces, default routes, DNS, and listening ports
- firewall, fail2ban, cron/timer, reboot, time-sync, update, swap, mount, auth, and kernel health signals
- top process summary
- process purpose/resource classification
- workload rollups for "what this node is being used for right now"
- selected systemd service state on Linux
- bounded recent error-log samples on Linux
- redacted security/capacity findings
- agent metadata, redacted node identity fingerprints, remote-access state, login state
- inventory hints: primary role, secondary roles, tags, confidence, observed workers
- capacity hints: CPU/memory/disk/GPU headroom, schedulability, blockers, suggested/avoid uses
- `ai_view`: plain-language summary, current purpose, safe uses, avoid list, concerns, and recommended actions

The stable report contract is documented in
[`NODE_REPORT_SCHEMA.md`](NODE_REPORT_SCHEMA.md).

Example:

```sh
meshclaw agent collect --json
meshclaw agent collect --top 5 --services ssh,open-webui
meshclaw agent collect --store --evidence --json
meshclaw agent run --interval 60s
meshclaw agent pull g4 --json
meshclaw agent history g4 --limit 20
meshclaw agent workloads
meshclaw agent changes all
```

## Workload Understanding

The node-local report keeps raw process data, but it also adds an
LLM-friendly `workloads` section. This is designed for Codex, Claude, Open
WebUI, and small local models that need to answer questions like:

```text
What is this server being used for right now?
Which process is consuming resources?
Is this load from LLM inference, Docker, Open WebUI, a database, or remote access?
```

Each top process receives:

```json
{
  "purpose": "llm_inference",
  "resource_class": "medium"
}
```

The report then rolls processes, containers, and public listeners into
purpose-level summaries:

```json
{
  "purpose": "llm_inference",
  "processes": 1,
  "cpu_pct": 45.2,
  "mem_pct": 12.0,
  "examples": ["ollama runner ..."]
}
```

Common purposes include:

- `llm_inference`
- `ai_chat_ui`
- `meshclaw_monitoring`
- `meshclaw_runtime`
- `remote_execution`
- `file_sync`
- `containerized_app`
- `container_runtime`
- `public_network_service`
- `database`
- `web_proxy`
- `app_runtime`
- `network_remote_access`
- `ssh_access`
- `system_service`
- `system_observer`

This does not replace raw evidence. It is a structured interpretation layer so
models do not need to infer everything from `ps`, `docker`, and `ss` output.

Product rule:

```text
agent = read-only sensor
vssh  = fresh diagnosis and controlled execution
MeshClaw = policy, approval, evidence, and workflow state
```

The next monitoring milestone is to store node-local reports in `opsdb`
and make `monitor-check` read cached agent state first, then call VSSH only for
stale, missing, or high-risk details.

Current state path:

```text
~/.meshclaw/state/nodes/<node>.json
~/.meshclaw/state/nodes/latest.json
```

`monitor-check` already merges fresh node-local state into its fleet output.
The node-local loop is `meshclaw agent run --interval 60s`, which repeatedly
runs the same read-only collector and stores the latest report.

The controller-side pull path is:

```sh
meshclaw agent pull g4 --json
meshclaw agent pull g1,g2,g3,g4 --services ollama,open-webui --json
meshclaw agent workloads --json
meshclaw agent changes g4 --json
meshclaw agent security --json
meshclaw agent inventory-plan --json
meshclaw ops-brief --fast --json
```

This uses VSSH only as the transport to invoke the remote read-only collector.
It does not expose a node-local command execution API.

`meshclaw agent workloads` does not contact remote hosts. It reads the latest
cached node reports from:

```text
~/.meshclaw/state/nodes/*.json
```

Use it when an AI operator asks:

```text
Which servers are being used for LLM inference?
What process is consuming resources?
What public services are exposed?
Which node should Codex/Claude inspect next?
```

The output is intentionally both human-readable and model-readable. JSON mode
contains per-node `natural_summary`, `workloads`, `top_processes`, and
medium/high `risks`. It also includes `next_actions`, which are conservative
operator steps such as checking fail2ban, triaging failed systemd units,
reviewing public Docker ports, or running disk investigation before cleanup.

`meshclaw agent changes` compares the latest two history samples and reports:

- load, memory, and disk deltas
- newly observed workload purposes
- removed workload purposes
- new medium/high security findings
- cleared security findings

This gives local models a compact answer to "what changed recently?" without
asking them to diff raw JSON by themselves.

`meshclaw agent security` summarizes cached node-local security and operations
posture:

- public listeners and Docker port mappings
- firewall warnings
- fail2ban availability/running state and jail count
- cron, systemd timer, and launchd scheduled work
- failed systemd units
- medium/high findings and conservative next actions

It is the preferred first response for broad questions such as "check ports,
firewall, fail2ban, Docker, and cron across the fleet" because it avoids a
fresh SSH fan-out and gives LLMs typed fields instead of raw `ss`, `docker`,
or `crontab` output.

`meshclaw agent inventory-plan` turns cached workload observations into
operator-owned inventory role/tag proposals. It is plan-only by default:

```sh
meshclaw agent inventory-plan --json
```

The report includes the proposed role, tags, reason, confidence, current
inventory values, and the exact `meshclaw inventory-override set ...` command
that would be used. This lets Codex, Claude, Open WebUI, or Argos explain the
proposal before mutating local `opsdb` inventory overrides.

Apply mode is explicit:

```sh
meshclaw agent inventory-plan --apply --json
```

Apply mode writes local inventory overrides only. It does not contact or modify
the remote server. After applying, run `meshclaw inventory-diff --json` and
placement/capability tools so node choice uses the updated fleet meaning.

For MCP clients, `meshclaw_ops_brief` defaults to fast mode. Fast mode uses
monitor state plus cached node-local agent reports and skips fleet service
audit, so it is suitable for Claude/Codex/Open WebUI timeout budgets. Run the
explicit service tools when deeper service triage is needed.

## Local State and History

Monitoring state is not stored like meshdb documents. meshdb is broad
personal/workspace memory: documents, scripts, conversations, and whole-computer
context. `opsdb` stores DevOps operational state: node snapshots, bounded
history, desired intent, inventory meaning, drift, approvals, and evidence
indexes. Reconciliation desired intent is also `opsdb` state, not personal
memory.

Each node can keep:

```text
~/.meshclaw/state/nodes/<node>.json      # latest local snapshot
~/.meshclaw/state/nodes/latest.json      # latest local snapshot alias
~/.meshclaw/state/history/<node>.jsonl   # bounded local history
```

Default history retention is 1440 entries, which is roughly one day at a
60-second interval. Override it with:

```sh
meshclaw agent run --interval 60s --max-history 2880
MESHCLAW_AGENT_MAX_HISTORY=2880 meshclaw agent run --interval 60s
```

For the controller, `meshclaw agent pull <hosts>` also appends remote reports to
the controller-side history, so Codex/Claude/Open WebUI can ask not only "what
is broken now?" but also "when did it start changing?"

On macOS this is available through launchd. On Linux it is available through a
user-level systemd unit.

```sh
meshclaw daemon install agent-collector --interval 60
meshclaw daemon start agent-collector
meshclaw daemon status agent-collector
```

Logs:

```text
~/.meshclaw/logs/agent-collector.log
~/.meshclaw/logs/agent-collector.err.log
```

Installed service files:

```text
macOS: ~/Library/LaunchAgents/ai.meshclaw.agent-collector.plist
Linux: ~/.config/systemd/user/ai.meshclaw.agent-collector.service
```
