# MeshClaw Agent Architecture

MeshClaw uses a hybrid operations model.

It does not choose between a central controller and node-local agents. It uses
both, because they solve different problems.

## Product Decision

MeshClaw Runtime is the control plane:

- policy
- approval
- evidence
- workflow state
- node registry
- router decisions
- MCP surface for Codex, Claude, Open WebUI, and local agents

VSSH is the execution substrate:

- real-time command execution
- structured RPC
- artifact collection
- pty/shell access
- fresh diagnosis when cached state is stale

`meshclaw agent` is the node-local sensor:

- local resource facts
- process summary
- Docker summary
- selected service state
- recent error-log samples
- conservative security/capacity findings
- redaction before storage or transport

The agent is not a chatbot and it is not a remote command endpoint.

## Why Not Only VSSH?

VSSH is excellent when an AI operator needs fresh detail or needs to execute a
specific action. It is not ideal as the only monitoring path because a central
controller must repeatedly connect to every node.

For a fleet with mixed nodes, the better default is:

```text
node-local agent -> cached state -> MeshClaw router/control plane
                                     |
                                     v
                              vssh fresh check
                              when needed
```

This gives LLMs a fast default answer while still allowing exact, real-time
diagnosis through VSSH.

## Why Not Only Push Agents?

Push agents are good at continuous observation, but weak at interactive
debugging. They can report that a service is failing, but the AI operator still
needs a controlled execution path to inspect logs, collect artifacts, or apply a
safe fix.

That execution path remains VSSH.

## Safety Boundary

The node-local agent must stay read-only in the first product milestone.

Allowed:

- read system facts
- read process summaries
- read Docker metadata
- read selected service status
- read bounded log samples
- redact secrets before returning data

Denied:

- arbitrary command execution
- service restart
- file deletion
- package install
- credential retrieval
- long-running mutation

Mutating actions must go through MeshClaw policy and VSSH execution.

## State Flow

```text
meshclaw agent collect --json
  -> meshclaw_node_report
  -> redacted structured JSON
  -> state ingest / evidence bundle
  -> monitor-check and ops-brief use cached state first
  -> vssh is used for stale, missing, or high-risk detail
```

## Legacy Sources Kept

The current architecture preserves the useful parts of earlier experiments:

- mpop resource-state heartbeat shape
- server-agent push/TTL/stale model
- argos-devops Docker/process/service collectors
- prompt-event-schema resource_state concept
- prompt-security-sentinel categories
- prompt-sanitize redaction-first rule

The current architecture drops:

- full mesh as the default monitoring model
- Wire as a required dependency
- node-local command execution RPC
- Python as the product core
- assistant/chatbot behavior inside MeshClaw itself

## First CLI Surface

```sh
meshclaw agent collect --json
meshclaw agent collect --top 5 --services ssh,open-webui
meshclaw agent collect --store --evidence --json
meshclaw agent run --interval 60s
meshclaw agent pull g4 --json
meshclaw agent pull g1,g2,g3,g4 --services ollama,open-webui --json
meshclaw agent history g4 --limit 20
meshclaw daemon install agent-collector --interval 60
meshclaw daemon start agent-collector
meshclaw daemon status agent-collector
```

This command is intentionally small. It is the stable local sensor primitive
that later daemon, scheduler, state ingest, and MCP tools can reuse.

`--store` writes the latest report to:

```text
~/.meshclaw/state/nodes/<node>.json
~/.meshclaw/state/nodes/latest.json
```

`monitor-check` can merge fresh stored agent reports into its fleet state. This
is the first step toward a faster hybrid monitor: local sensors report cheap
state continuously, and VSSH is reserved for fresh detail or controlled action.

On macOS, `meshclaw daemon install agent-collector` writes a LaunchAgent at:

```text
~/Library/LaunchAgents/ai.meshclaw.agent-collector.plist
```

The daemon runs:

```sh
meshclaw agent run --interval 60s --store
```

This keeps the local node report fresh without granting any mutation capability
to the agent.

The controller can also pull a fresh remote agent report through VSSH:

```sh
meshclaw agent pull g4 --json
```

This runs the remote node's read-only collector, stores the result in the
controller's `~/.meshclaw/state/nodes/<node>.json`, and lets `monitor-check`
merge that cache into fleet state.

On Linux, the same command writes a user-level systemd unit at:

```text
~/.config/systemd/user/ai.meshclaw.agent-collector.service
```

It uses `systemctl --user`, so it does not require root. This matches the VSSH
direction: MeshClaw's node sensor should be installable as the normal account
that owns the node automation lane.

## opsdb Storage Model

MeshClaw monitoring storage is `opsdb`. It is intentionally different from
meshdb.

meshdb stores durable knowledge: documents, scripts, conversations, and
workspace material.

`opsdb` stores operational state:

```text
latest snapshot: ~/.meshclaw/state/nodes/<node>.json
history:         ~/.meshclaw/state/history/<node>.jsonl
```

The default history size is 1440 entries. At a 60-second interval this is about
one day of local state. Longer retention should be explicit because process and
log samples can be operationally sensitive even after redaction.
