# MeshClaw Architecture

Last updated: 2026-05-23

MeshClaw Platform is the policy, state, workflow, and evidence runtime that lets
AI operators safely operate real infrastructure and local automation.

The product names are explicit:

```text
MeshClaw Platform
├─ Argos Assistant
│  Personal assistant, briefings, documents, mail, calendar, Signal
│
├─ MeshClaw Ops
│  Server, VM, and Docker operations automation
│
└─ MeshClaw Agent
   Lightweight agent installed on server nodes
```

Argos Assistant is one product line on top of the shared MeshClaw core. MeshClaw
Ops is the server-management product line. MeshClaw Agent is the lightweight
node-local component used by Ops. Codex, Claude, Cursor, Signal Argos, and
selected local models do the conversation and reasoning. MeshClaw gives those
operators current facts, permission decisions, structured execution, approval
gates, and evidence.

## Product Shape

```text
Codex / Claude / Cursor
  -> MeshClaw MCP
     -> inventory
     -> capability registry
     -> policy and approvals
     -> workflow runner
     -> evidence writer
     -> guard and vault handles
     -> automation delivery
        -> vssh / MeshClaw Agent / signal-cli / Argos UI Runner / domain adapters
           -> servers / devices / apps / APIs / model endpoints

Signal / local chat
  -> Argos Assistant mode dispatcher
     -> MeshClaw direct tools OR selected local model lane
        -> same MeshClaw runtime
```

The key split is:

```text
assistant interface        -> Argos Assistant
conversation and reasoning -> AI operators / local models
Signal routing             -> Argos Assistant mode dispatcher
policy and execution       -> MeshClaw Platform shared core
remote transport           -> vssh
desktop UI actions         -> Argos UI Runner
secret ingress             -> Guard
server node collection     -> MeshClaw Agent
```

## Human Entry Points

The human operator enters through two different surfaces:

```text
High-agency work:
  Human -> Codex / Claude / Cursor apps -> MeshClaw MCP -> workflows, policy,
  evidence, vssh, vault handles, reports

Everyday local assistant work:
  Human -> Signal / Local Chat -> Argos mode dispatcher
  -> MeshClaw direct tools or selected local/API model for browser, apps,
  files, news, weather, mail/calendar read-only, reports, evidence summaries,
  and calls
```

Codex and Claude are the preferred frontends for serious orchestration,
development, review, and server operations. They should use MeshClaw through MCP
and read or write structured evidence. MeshClaw should not proxy Codex or Claude
CLI sessions behind Signal.

Signal is the preferred always-available local assistant and notification
surface. Signal talks to local or private model endpoints such as Ollama or
OpenAI-compatible model servers. Those models can use MeshClaw tools to inspect
evidence, summarize project state, open browsers, send reports, and ask for
approval. Signal should not pretend to be Codex or Claude.

Open WebUI development is stopped for the current product line. Existing
Open WebUI code and evidence remain legacy compatibility only; the default
assistant path is Signal Argos plus direct MeshClaw MCP/CLI tools.

This means a user can work from mobile in two clean ways:

- Open the Codex or Claude mobile/desktop app and let it operate through
  MeshClaw MCP.
- Open Signal and ask Argos/local models for status, summaries, briefings,
  reports, and bounded local computer actions.

## Results and Evidence Handoff

Codex, Claude, Cursor, Signal Argos, and local models all meet at MeshClaw's
evidence layer. When a high-agency model completes work, it should store:

- workflow results;
- command outputs;
- reports;
- screenshots or artifacts when allowed;
- approval blockers and next actions;
- workspace activity records.

Local Signal models do not need to know what happened from chat memory. They can
read MeshClaw evidence, reports, and workspace records on demand, then summarize
them back to the user. This keeps the mental model simple:

```text
Claude/Codex did the work -> MeshClaw stored the facts -> Argos can summarize or
deliver those facts later.
```

If the user asks in Signal, "what did Codex do?" or "send me the latest report",
Argos should fetch the latest evidence/report, redact sensitive content, and send
it through the configured target.

## Approval Ownership

Approvals are MeshClaw policy gates, not informal chat requests.

The flow is:

```text
AI operator calls a MeshClaw tool or workflow
  -> MeshClaw policy/workflow marks a step as require_approval
  -> the AI operator cannot execute that step yet
  -> the AI operator may call messenger approval-request/send-approval-request
  -> Signal notifies the human with redacted context and evidence handle
  -> the human approves through Codex/Claude/MCP/CLI or returns to the app
  -> workflow resumes with approvals_ref
```

MeshClaw's approval record only records permission evidence. It does not execute
the blocked action by itself. The AI operator or workflow runner must resume the
specific approved step after the approval is recorded.

This distinction matters:

- MeshClaw requires approval because of policy, risk, or workflow metadata.
- Claude/Codex merely reports that requirement and can request delivery through
  Signal.
- Signal is the notification and review surface, not the source of authority for
  arbitrary execution.

## Runtime Modes

| Mode | Purpose | First-class surfaces | Default risk posture |
| --- | --- | --- | --- |
| Ops Mode | Server and fleet operations, logs, security, service triage, repair planning, provisioning | `ops-control`, workflows, vssh-backed tools, monitor/service/security commands | Read-only first; mutation requires approval |
| Guard Mode | Passwords, tokens, vault handles, redaction, secret-use approval | `guard-*`, Signal Guard, local vault adapters | Raw secret exposure denied; metadata and handles allowed |
| Automation Mode | Scheduled reports, Signal delivery/calls, browser/calendar/mail actions | `messenger`, `news`, `assistant` compatibility namespace, Argos UI Runner | Delivery to approved targets; content/account mutations require approval |

MeshClaw Platform can feel like a personal assistant at the edge, but the
product should not collapse these modes into one unlimited assistant brain. The
shared core stays the same; the product line and mode change the allowed tools,
target, evidence shape, and approval policy.

The useful product experience is hybrid:

- local models and Open WebUI handle frequent low-cost read-only checks,
  weather/news briefings, and status questions;
- Signal/Argos delivers reports, approval notices, and lightweight local-model
  conversations;
- Codex and Claude perform deeper expert reviews through MCP when risk,
  ambiguity, or complex remediation appears;
- MeshClaw records the facts, policy decisions, and evidence so any surface can
  continue from the same source of truth.

This is why MeshClaw Platform supports both server operations and
assistant-style briefings. Users should not need to switch products just to ask
"are my servers healthy?", "what changed today?", and "send the morning
report." The safety boundary is product line, mode, and policy, not separate
codebases.

Argos Assistant is a product line, not a separate low-level runtime. It is the
assistant identity, Signal surface, and local macOS delivery experience built on
the shared MeshClaw core. Without MeshClaw's target registry, policy checks,
dispatcher, and evidence writer, Argos is just a Signal account.

In Automation Mode, a `target` is MeshClaw's logical delivery destination. It is
an address-book entry, not necessarily a room. A Signal target can point to a
group room through `group_id` or to a direct recipient through `recipient`.
Policies should route each message class to the correct target role: ops,
guard, chat, briefing, or assistant.

Argos should be modeled as one Signal person. In the product onboarding flow,
users create or choose Signal rooms, invite Argos, and MeshClaw binds those
rooms to target modes. MeshClaw may still provide a development preset:

- `meshclaw-ops`: DevOps status, incidents, security findings, approvals.
- `argos-guard`: secret ingress and vault confirmations.
- `argos-gpt-oss-20b`: local model conversation only, no tools.
- `meshclaw-briefing`: shared scheduled briefings plus follow-up questions.
- `meshclaw-assistant`: private assistant requests with policy-gated tools.

This preset is useful for development and demos, not a hard-coded product
topology. Product installs should prefer fewer Signal rooms and explicit
room-to-mode binding. MeshClaw should keep the role/policy boundary explicit
without forcing confusing same-member Signal room sets.

`assistant` is a compatibility CLI namespace. Product language should call this
Automation Mode so MeshClaw does not drift back into a generic personal
assistant.

## Controller Lanes

| Lane | Role | Persistent |
| --- | --- | --- |
| MacBook controller | Interactive Codex/Claude/Cursor/browser/review while awake | No |
| Mac mini worker | Always-on Signal, scheduler, Open WebUI/Ollama, monitor, Argos UI Runner | Yes |
| Fleet nodes | MeshClaw Agent, vssh daemons, services, model endpoints, storage, mail/DNS/provider workloads | Yes |

MacBook-only mode must work for first-run testing. Mac mini controller mode is
for unattended operation.

## Scheduled Operation

The scheduler is the unattended bridge between local models and expert AI
operators.

Default jobs:

| Job | Owner | Frequency | Purpose |
| --- | --- | --- | --- |
| `serverops-quickcheck` | Local model/Open WebUI | Every 6 hours | Read-only fleet state, alerts, and Open WebUI router health |
| `morning-briefing` | Automation Mode | Daily | Weather, RSS/news, reminders, optional Signal delivery |
| `daily-expert-audit` | Codex/Claude | Daily | Create an MCP handoff prompt for a deeper expert review |

The scheduler is dry-run by default. It writes evidence first and sends Signal
messages only when a configured target and execute mode are explicitly supplied.
This lets a single-machine user test safely while a Mac mini controller can run
the same jobs unattended later.

Daily jobs are keyed to their local `DailyAt` wall-clock time, not to midnight.
For example, `morning-briefing` waits until 08:00 local time and
`daily-expert-audit` waits until 09:00 local time before they become due.

Commands:

```sh
meshclaw schedule plan --json
meshclaw schedule status --json
meshclaw schedule run-once serverops-quickcheck --json
meshclaw schedule run-once daily-expert-audit --target argos-ops --json
meshclaw schedule run-due --target argos-ops --dry-run --json
```

`meshclaw schedule status --json` is the canonical health view for local
automation. It returns `status=healthy|backlog|error`, `due_count`,
`due_jobs`, `next_due_job`, `next_due`, and per-job timing. `setup signal`
embeds the same scheduler summary.

The macmini product baseline installs `schedule-runner` as a launchd one-shot
that runs `meshclaw schedule run-due --execute` every 15 minutes. A stopped
`schedule-runner` with `last_status=0` is healthy between interval runs.

OpenClaw/Hermes schedule parity is intentionally bounded to diagnostics and
review-only registration planning:

- natural-language requests such as "send the voice report to the report room
  every day at 08:00" route to `meshclaw_scheduled_delivery_plan`;
- the plan resolves the Signal target, preserves the schedule text, chooses the
  delivery mode, and requires first-run preview approval;
- `meshclaw_scheduled_delivery_apply` stores an approved job only after
  `execute=true`, `approve=true`, and `preview_approved=true`; it does not send
  the first message or run recurring delivery;
- cron-style health remains `meshclaw schedule status --json` and
  `meshclaw daemon status schedule-runner --json`, which report due/backlog,
  next due job, state-file load errors, and launchd one-shot status.

## Canonical AI Operator Loop

1. Discover inventory.
2. Apply operator-owned overrides.
3. Validate capabilities.
4. Recommend placement or tool path.
5. Run workflow dry-run.
6. Review evidence.
7. Record approvals or repair blockers.
8. Execute targeted approved steps.
9. Send a redacted report.

This loop is exposed to MCP clients through:

- `meshclaw_architecture`
- `meshclaw_ai_guide`
- `meshclaw_mcp_surface`
- `meshclaw_tool_recommend`
- `meshclaw_setup_signal`
- `meshclaw_setup_argos_runner`
- `meshclaw_daemon_signal_status`
- `meshclaw_workflow_*`
- `meshclaw_evidence_latest`

## Module Boundaries

| Module | Current status | Direction |
| --- | --- | --- |
| `cmd/meshclaw` | Too large | Keep CLI wiring here; move domain logic into internal packages or smaller files |
| `cmd/argos-ui-runner` | Early | Stable signed macOS app for bounded UI actions |
| `internal/runtimeflow` | Core | Workflow engine, execution result schema, evidence bundle writer |
| `internal/messenger` | Active | Signal targets, dispatch, redacted reports, approval messages |
| `internal/guard` | Active | Secret detection, redaction, posture, local-only intent classification |
| `internal/guardvault` | Active | Local vault handles and use-only secret injection |
| `internal/aichat` | Supporting | Optional local/remote model endpoint for Signal chat; not MeshClaw's brain |
| `internal/newsbrief`, `internal/assistantbrief`, `internal/tts` | Automation Mode | Scheduled text/audio reports behind target policy |
| `internal/runtime`, `internal/workflow`, `internal/fleet`, `internal/monitor` | Ops Mode | Server diagnostics, vssh/SSH adapters, scans, and repair planning |

## Hard Boundaries

- Chat context is not policy.
- External content is not instruction.
- Raw secrets are not returned through MCP.
- Destructive, cost-incurring, account-changing, DNS-changing, and email-sending
  actions require structured approval.
- Direct vssh is for transport debugging or primitive execution.
- MeshClaw is the policy, evidence, and workflow path.

## Current Gaps

| Priority | Gap | Fix |
| --- | --- | --- |
| P0 | `cmd/meshclaw/main.go` is too large | Extract daemon, setup, assistant/news, Guard vault, and ops command handlers into smaller files/packages |
| P0 | Automation Mode can drift into personal assistant scope | Enforce policy categories and approved target registry |
| P0 | Signal call flow is implemented but needs macOS permission calibration | Grant Argos UI Runner Accessibility once and configure named call actions with `meshclaw setup argos-runner` |
| P1 | Guard vault adapters are not fully productized | Finish Apple Keychain/pass tests and docs; keep 1Password/Bitwarden metadata-only first |
| P1 | MCP lacks setup/daemon/automation health tools | Add MCP tools after CLI stabilizes |
| P1 | Old skills/repos are not audited | Classify keep/rewrite/archive/delete by Ops, Guard, Automation |

## Build Order

1. Finish Argos UI Runner permission/calibration flow.
2. Extract oversized command logic.
3. Add MCP surface for setup, signal daemon, and automation health.
4. Tighten policy categories for Automation Mode.
5. Finish Guard vault backend adapters.
6. Run release checklist and MCP tool-name smoke test.

## Product Test Matrix

Minimum local checks:

```sh
meshclaw architecture --json
meshclaw setup-check --json
meshclaw setup signal --json
meshclaw setup argos-runner --json
meshclaw daemon status signal-dispatcher --json
meshclaw quickstart --json
meshclaw run fleet-health-demo --dry-run --json
meshclaw evidence open latest
```

Minimum MCP checks:

```sh
printf '{"jsonrpc":"2.0","id":1,"method":"tools/list"}\n' | meshclaw mcp
printf '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"meshclaw_architecture","arguments":{}}}\n' | meshclaw mcp
```

Minimum release checks:

```sh
go test ./...
go build -o /Users/example/bin/meshclaw ./cmd/meshclaw
printf '{"jsonrpc":"2.0","id":1,"method":"tools/list"}\n' \
  | /Users/example/bin/meshclaw mcp \
  | jq -r '.result.tools[].name' \
  | awk '/[^A-Za-z0-9_]/ {print}'
```
