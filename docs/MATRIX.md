# Matrix Bridge

Matrix is allowed, but only as an operations surface.

Current default direction is Signal/Argos for assistant delivery and Open WebUI
through MeshClaw Router for local-model work. Matrix remains optional
ops-room/legacy compatibility.

It should be used for:

- shared incident rooms
- notifications
- approval prompts
- evidence summaries
- narrow MeshClaw MCP/CLI command calls

It should not become:

- the assistant brain
- a general personal assistant
- a separate policy system
- a replacement for Codex, Claude, ChatGPT, or local LLM frontends

## Flow

```text
human / Codex / Claude / local LLM
  -> Matrix ops room
  -> Matrix bridge
  -> MeshClaw MCP / CLI
  -> policy_check
  -> vssh / fleet scan / evidence
  -> concise Matrix result + evidence link
```

## Command Surface

Safe defaults:

- `workers`
- `server_list`
- `capability_list`
- `monitor_check`
- `fleet_scan`
- `evidence_list`
- `policy_check`
- `policy_show`
- `analyze_logs`
- `security_check`
- `hygiene_scan_host`

Approval-gated:

- `run_command`
- `autoheal_apply_safe`
- `service_remove`
- `provision_server`
- `deprovision_server`
- `rotate_secret`

Use:

```sh
meshclaw matrix-plan
meshclaw matrix-config-init --force
meshclaw matrix-post "MeshClaw Matrix bridge connected"
meshclaw matrix-sync-once
meshclaw matrix-bridge
meshclaw workers
```

The real bridge is intentionally small. It reads `~/.meshclaw/matrix.json`
with `homeserver`, `user_id`, `access_token`, `room_id`, and optional
`command_prefix`. If that file is missing, it can fall back to the previous
Argos Matrix config only for migration.

The Matrix command maps `!workers` to `meshclaw_workers`. That command does not
resurrect the old personal-assistant worker runtime; it lists the active
operator surfaces and execution workers:

- Codex
- Claude
- local LLM / Open WebUI
- Matrix ops room
- MeshClaw MCP
- vssh daemon

Workspace commands should also be exposed in Matrix:

- `!workspaces` -> `meshclaw_workspace_list`
- `!workspace add ...` -> `meshclaw_workspace_add`
- `!activity ...` -> `meshclaw_workspace_activity`

These commands answer the operational question: which model or human is working
on which server, folder, branch, and task.

For local testing without Matrix network calls, use the local conversation
harness:

```sh
meshclaw ops-chat
meshclaw ops-chat workers
meshclaw ops-chat "show server status"
meshclaw ops-dispatch matrix "!workers"
```

For a persistent macOS bridge, run it under launchd with:

```sh
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/ai.meshclaw.matrix-bridge.plist
launchctl kickstart -k gui/$(id -u)/ai.meshclaw.matrix-bridge
```
