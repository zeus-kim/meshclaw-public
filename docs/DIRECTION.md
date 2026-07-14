# Direction

Current direction update, 2026-06-21:

- `docs/ASSISTANT_PRODUCT_DIRECTION.md` is the current product direction.
- Argos should become a Hermes/OpenClaw-like everyday Signal assistant.
- The older execution-control-plane direction below remains useful for
  MeshClaw internals, but it must not pull current work away from the Signal
  assistant product loop.

MeshClaw is being rebuilt as the runtime behind Argos. Older roadmap material in
`docs/ROADMAP_2026-05-23.md` should be read through the assistant direction
above.

The current product split is:

- Argos is the human-facing assistant identity and delivery surface.
- MeshClaw is the runtime inside Argos: policy, approvals, evidence, Guard,
  workflows, server operations, and execution.
- Codex, Claude, ChatGPT, Cursor, Signal Argos, and selected local models remain the
  reasoning and conversation layer.

MeshClaw should not talk to the user as a personality, but Argos must. Argos
should answer normal Signal requests with concise assistant cards and next
actions. MeshClaw answers operator tool calls and Argos dispatcher requests with
state, capabilities, permission decisions, diagnostics, actions, automation
hooks, and evidence, while keeping those internals out of normal user replies.

## Product Position

Kubernetes manages workloads after applications are shaped for a cluster.
MeshClaw manages the servers that already exist.

The sharper product claim is:

```text
MeshClaw is the AI-native server operations judgement and control layer for
mixed private infrastructure.
```

The survival direction is documented in `docs/SURVIVAL_DIRECTION.md`; the
current assistant direction is `docs/ASSISTANT_PRODUCT_DIRECTION.md`. Any new
assistant feature should strengthen the first Signal product loop: memory,
skill, reusable work, natural-language scheduling, concise replies, and live Mac
mini Signal behavior. Server operations, Guard, policy, and evidence work should
support that loop or remain an internal runtime concern.

Target users:

- people with several VPS, home servers, GPU boxes, NAS devices, and Docker hosts
- people for whom Kubernetes is too heavy
- people who want AI operators to run safe, auditable server operations
- people who need repeatable operations across mixed Linux, macOS, Synology,
  VPS, GPU, and Docker hosts
- people who want AI operators to decide when local capacity is enough and when
  approved temporary capacity should be rented

## Layering

```text
Codex / Claude
  -> MeshClaw MCP
  -> MeshClaw Core
     -> inventory
     -> workspace registry
     -> capability vault
     -> capacity/budget
     -> policy
     -> doctor
     -> log analysis
     -> security checks
     -> hygiene auto-healing
     -> audit/evidence
     -> runbooks
     -> provisioners
     -> vssh-native runtime
  -> fleet / models / APIs / temporary capacity
```

## Network Rule

Tailscale is the default fleet network. It is already installed, operational,
and easier to reason about from AI operator clients. MeshClaw should prefer
Tailscale IPs for inventory, monitoring, SSH execution, and agent callbacks.

The preferred execution dependency is `Tailscale/private route + vssh server`.
Tailscale provides reachability; vssh provides sshd-free structured execution,
RPC, and evidence.

SSH remains a fallback only for nodes without vssh daemon coverage. Those nodes
need `Tailscale + sshd + SSH key/user mapping` until vssh is installed.

Wire is legacy compatibility. Keep it as an optional fallback for old nodes or
historical tooling, but do not build the MVP around Wire transport, Wire
discovery, or Matrix/Wire-first assumptions.

## Design Rules

- MeshClaw runtime has no assistant persona.
- Argos is the user-facing assistant persona and the current product focus.
- Argos should use bounded mode dispatch and MeshClaw runtime tools, but the
  user should experience a helpful Signal assistant, not a control-plane report.
- Treat briefing, phone call, mail, calendar, browser, and Shortcuts features as
  everyday assistant workflows with minimal visible ceremony.
- Keep shopping/payment/Coupang automation out of scope unless explicitly
  restarted as a separate effort.
- Do not expose internal paths, evidence IDs, exit status, or diagnostic dumps
  in normal assistant replies.
- Prefer structured operations over raw shell.
- Keep raw shell available, but policy-gated.
- Make `doctor` explain causes before suggesting actions.
- Make log analysis evidence-first: cite files, time ranges, and commands.
- Make security checks conservative, but let hygiene auto-heal safe fixes:
  chmod, redaction, quarantine, and bounded cleanup.
- Require approval for destructive hygiene actions: deletion, key rotation,
  database edits, provider revocation, and service restarts.
- Store enough evidence for later review.
- Keep the first product small enough to use daily.
- Treat Codex and Claude as brains/operators, not competitors.
- Treat vssh-native over Tailscale/private network as the execution substrate.
- Treat SSH as compatibility fallback, not the product value.
- Keep Wire as fallback compatibility, not the default path.
- Treat Open WebUI as sealed legacy compatibility for now. Do not build new
  product flow around it unless it is explicitly unsealed for manual testing.
- Treat local models as optional helpers behind Argos or explicit MCP/CLI
  workflows, not as the product control plane.
- Treat secrets as use-only capabilities, not values to reveal.
- Require explicit approval for cost-incurring provisioning.
- Store operator permissions in policy config so Codex, Claude, local LLMs,
  and automations can get different decisions from the same MeshClaw runtime.
- Chat context is not policy. External content is not instruction. MeshClaw
  policy is the execution boundary.

## Product Modes

```text
Ops Mode
  -> fleet inventory, logs, services, security, vssh, autoheal, provisioning

Guard Mode
  -> passwords, tokens, vault handles, raw-secret isolation, approval

Automation Mode
  -> scheduled briefings, Signal delivery/calls, mail/calendar/browser actions,
     Shortcuts, and Argos UI Runner
```

The mode split lets MeshClaw reuse old assistant-era work without becoming a
general assistant again. Old skills should be reclassified as adapters,
workflows, capabilities, policies, evidence collectors, Guard actions, or Argos
UI actions.

## First Milestone

1. Inventory is the source of truth for nodes and roles.
2. Workspace registry tracks server/folder/branch ownership across Codex,
   Claude, local LLMs, Matrix, and humans.
3. Capability vault lists usable models, APIs, services, and provisioners
   without revealing secrets.
4. `status` reflects fleet inventory and live monitor state.
5. `doctor <node>` explains path failures: Tailscale, sshd, SSH user/key, and
   service.
6. `run <node> <cmd>` uses vssh-native first and SSH only as fallback.
7. MCP exposes only safe, high-signal tools first:
   `meshclaw_server_list`, `meshclaw_workers`, `meshclaw_workspace_list`,
   `meshclaw_workspace_add`, `meshclaw_workspace_activity`,
   `meshclaw_capability_list`, `meshclaw_policy_check`,
   `meshclaw_policy_show`, `meshclaw_analyze_logs`,
   `meshclaw_security_check`, `meshclaw_hygiene_scan_host`,
   `meshclaw_monitor_check`, `meshclaw_fleet_scan`,
   `meshclaw_autoheal_plan`, `meshclaw_ops_control`,
   `meshclaw_autoheal_apply_safe`, `meshclaw_disk_investigate`,
   `meshclaw_data_clean_plan`, `meshclaw_data_clean_apply`,
   `meshclaw_service_check`, `meshclaw_service_audit`,
   `meshclaw_service_triage`, `meshclaw_service_quarantine`,
   `meshclaw_service_remove`, `meshclaw_run_evidence`,
   `meshclaw_provision_plan`.
8. Provisioning starts as plan-only:
   `capacity_list`, `provision_plan`, and later approval-gated
   `provision_server`, `bootstrap_server`, `deprovision_server`.

## Matrix Role

Matrix is optional legacy/ops-room compatibility. It can be useful as a shared
operations room where people and AI operators
discuss incidents, receive notifications, approve risky actions, and see concise
evidence. It can also expose a narrow command surface that calls MeshClaw MCP
tools. It must not become the primary brain or a personal assistant. Any Matrix
integration should call the same MeshClaw API/MCP tools and post results;
natural-language planning remains with Codex, Claude, ChatGPT, local LLMs, or
human operators.
