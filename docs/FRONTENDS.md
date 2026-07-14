# Frontends

MeshClaw does not force one UI.

The frontend rule is simple: the frontend talks; MeshClaw proves and acts.

## Codex / Claude / Cursor

Codex, Claude, and Cursor are the strongest high-agency work surfaces. They use
MeshClaw MCP when work needs shared infrastructure truth, policy decisions,
safe execution, approvals, or evidence.

Use it for:

- coding and refactoring
- architecture work
- expert incident review
- server operations with MCP-backed evidence
- workflow planning and execution preflight

## Open WebUI

Open WebUI is the local/private model workbench.

Users should normally select `MeshClaw Router`, not a raw local model. The
Router decides whether the message should be handled by MeshClaw direct tools,
approval flow, denial, or a model lane.

Use it for:

- local model chat
- private summaries
- read-only server status
- routine checks
- testing whether local models can work through MeshClaw Router

Examples:

```text
server status
node health
who is Yi Sun-sin?
check g4 Open WebUI status
```

## Signal / Argos

Signal is the mobile assistant, report, call, and approval surface.

Argos is the Signal identity. Users create or choose rooms, invite Argos, and
MeshClaw binds each room to a target mode such as `assistant`, `ops`, `guard`,
or `briefing`.

Use it for:

- briefings
- status reports
- approvals and reminders
- local model chat
- bounded macOS app automation through MeshClaw
- safe password/secret ingress through Guard
- audio calls and voice files through Argos UI Runner

For desktop work, Argos is the assistant identity and MeshClaw is the runtime
that opens apps, searches the web, writes notes, runs Shortcuts, captures
evidence, and reports back. See `ARGOS_MACOS_AUTOMATION.md`.

## Matrix

Matrix is optional legacy/ops-room compatibility. It is not the default
assistant direction.

Use it for:

- shared incident timelines
- evidence links
- approval notifications
- narrow CLI/MCP command dispatch

Do not use Matrix as the Guard password room or the primary assistant brain.

## Difference

```text
Codex/Claude/Cursor = expert coding, review, and high-agency operations
Open WebUI          = local model workbench through MeshClaw Router
Signal / Argos      = mobile assistant, reports, calls, approvals
Matrix              = optional legacy ops room
MeshClaw            = policy, workspace, evidence, Guard, operations runtime
vssh                = remote execution substrate
```
