# MeshClaw Reconciliation Controller

Last updated: 2026-05-26

This document defines the next controller milestone for MeshClaw.

MeshClaw should borrow the Kubernetes controller pattern, but it should not
become Kubernetes. The target is mixed real infrastructure: MacBook, Mac mini,
Linux GPU servers, VPS nodes, NAS nodes, model endpoints, mail servers, browser
automation, Signal delivery, and ordinary systemd/Docker services.

## Product Position

MeshClaw is an AI-native operations runtime.

Argos and AI frontends can behave like an assistant, but the runtime must keep
ops state separate from broad personal memory.

```text
personal context / documents / conversations
  -> meshdb or other personal memory stores

server operations state / desired intent / evidence / policy
  -> opsdb and MeshClaw evidence stores
```

meshdb may help an assistant remember the broader user's work, but that is
exactly why it should not be the DevOps source of truth. meshdb can contain
nearly everything about the user and the user's computers: documents,
conversations, projects, scripts, local application context, and personal
workflow history. Most of that is irrelevant and too sensitive for server
management.

DevOps reconciliation needs a smaller, auditable, operational store. The name
for that store is `opsdb`.

`opsdb` should have explicit retention and redaction. It should contain only
facts needed to answer questions like:

- what node is this?
- what role is it supposed to serve?
- what is running now?
- what changed recently?
- what policy applies?
- what evidence supports the next action?

Initial path:

```text
~/.meshclaw/state/
  nodes/
  history/
  desired/
  drift/
  approvals/
  evidence-index.jsonl
```

Override for tests or deployments:

```sh
MESHCLAW_OPSDB=/var/lib/meshclaw/opsdb
```

## Why Reconciliation

Today MeshClaw already has most pieces of a controller loop:

```text
observe -> detect -> plan -> policy gate -> apply -> verify -> evidence
```

The missing product layer is a controller that repeatedly compares:

- desired operational intent
- current node-local agent state
- inventory and capability registry
- policy and approval state
- previous evidence and failures

Then it produces bounded actions:

- no-op when actual state matches intent
- read-only diagnosis when state is ambiguous
- safe autoheal plan when the policy allows it
- approval request when risk is higher
- denial when the action is disallowed

This is similar to Kubernetes reconciliation, Puppet/Chef convergence, or
GitOps drift detection, but the MeshClaw version is optimized for small,
heterogeneous fleets and AI operators.

## Desired Intent

The desired layer should not require every user to hand-write YAML.

Supported sources should be:

- `desired-state.yaml` for explicit operators
- inventory overrides created by Codex/Claude/MeshClaw tools
- Argos/Open WebUI conversations that result in approved desired changes
- workflow outputs that create repeatable operational intent
- policy defaults such as "do not mutate without approval"

Human language can create a desired intent, but it must become structured state
before the reconciler acts.

Example:

```yaml
nodes:
  g4:
    roles: [openwebui-worker, ollama-worker]
    services:
      open-webui:
        desired: running
        restart: approval_required
    containers:
      open-webui:
        desired: running
        image: ghcr.io/open-webui/open-webui:main
        health: healthy
        restart: approval_required
    capacity:
      allow_model_jobs: true
      min_disk_free_pct: 20
      max_memory_used_pct: 85
```

The initial parser accepts this structure under `internal/reconciler`.
Service desired state may be written either as a scalar:

```yaml
services:
  vsshd: running
```

or as a mapping, which leaves room for future restart and rollout policy:

```yaml
services:
  open-webui:
    desired: running
```

Container desired state is optional and currently parsed/validated before any
executor exists:

```yaml
containers:
  open-webui:
    desired: running
    image: ghcr.io/open-webui/open-webui:main
    health: healthy
    restart: approval_required
  redis: running
```

The first CLI surface is dry-run only:

```sh
meshclaw reconcile validate-desired --desired desired-state.yaml --json
meshclaw reconcile plan --desired desired-state.yaml --actual-report node-report.json --json
meshclaw reconcile approval-request --desired desired-state.yaml --actual-report node-report.json --json
meshclaw reconcile apply-gate --desired desired-state.yaml --approval-evidence evidence.json --approved-by operator --json
meshclaw reconcile apply-plan --gate-evidence gate-evidence.json --json
meshclaw reconcile execution-preview --apply-plan-evidence apply-plan-evidence.json --json
meshclaw reconcile verification-plan --execution-preview-evidence execution-preview-evidence.json --json
meshclaw reconcile runbook --verification-plan-evidence verification-plan-evidence.json --json
meshclaw reconcile runbook-check --runbook-evidence runbook-evidence.json --json
meshclaw reconcile rollback-plan --runbook-check-evidence runbook-check-evidence.json --json
meshclaw reconcile completion-plan --rollback-plan-evidence rollback-plan-evidence.json --json
meshclaw reconcile readiness-summary --completion-plan-evidence completion-plan-evidence.json --json
meshclaw reconcile run-once --dry-run --desired desired-state.yaml --actual-report node-report.json --json
```

`validate-desired` parses desired-state YAML, reports critical/warning
findings, stores `reconcile-desired-validation` evidence, and refuses
`--apply` / `--execute`. Critical findings block future apply readiness.
`plan` stores `reconcile-plan` evidence and refuses `--apply` / `--execute`
until policy decisions and verification evidence are wired. `approval-request`
uses the same dry-run diff, separates policy-denied actions from
`require_approval` actions, and stores `reconcile-approval-request` evidence
without executing anything. `apply-gate` reads that approval evidence and
returns not-ready when blocked actions exist, the desired path does not match,
or approval-required actions do not have `--approved-by`. It also stores
`reconcile-apply-gate` evidence but still does not execute. `apply-plan` reads
ready gate evidence and emits ordered, structured apply steps for a future
executor; it stores `reconcile-apply-plan` evidence and still does not run
commands. `execution-preview` turns apply-plan evidence into inert command
templates with required verification hints, stores `reconcile-execution-preview`
evidence, and still does not run commands. `verification-plan` turns execution
preview evidence into required post-action evidence checks and failure handling
rules, stores `reconcile-verification-plan` evidence, and still does not run
commands. `runbook` combines command previews and verification requirements
into review-only operator steps, stores `reconcile-runbook` evidence, and still
does not run commands. `runbook-check` validates that runbook steps have command
templates, required evidence, and verification gates before any future executor
can consume them; it stores `reconcile-runbook-check` evidence and still does
not run commands. `rollback-plan` derives rollback guidance from ready
runbook-check evidence, stores `reconcile-rollback-plan` evidence, and still
does not run commands. `completion-plan` defines final evidence requirements
before any future reconcile loop can be marked complete, stores
`reconcile-completion-plan` evidence, and still does not run commands.
`readiness-summary` summarizes the approval-to-completion chain from
completion-plan evidence, stores `reconcile-readiness-summary` evidence, and
still does not run commands. Without
`--actual-report`, the plan treats desired nodes as missing/offline. With
`--actual-report`, the command binds a local
`meshclaw_node_report` JSON file to the matching desired node and performs a
real dry-run diff without contacting the server.

MCP clients can call the same dry-run surface:

```text
meshclaw_reconcile_validate_desired({
  "desired_path": "desired-state.yaml"
})

meshclaw_reconcile_plan({
  "desired_path": "desired-state.yaml",
  "actual_report_path": "node-report.json"
})

meshclaw_reconcile_approval_request({
  "desired_path": "desired-state.yaml",
  "actual_report_path": "node-report.json"
})

meshclaw_reconcile_apply_gate({
  "desired_path": "desired-state.yaml",
  "approval_evidence_path": "evidence.json",
  "approved_by": "operator"
})

meshclaw_reconcile_apply_plan({
  "gate_evidence_path": "gate-evidence.json"
})

meshclaw_reconcile_execution_preview({
  "apply_plan_evidence_path": "apply-plan-evidence.json"
})

meshclaw_reconcile_verification_plan({
  "execution_preview_evidence_path": "execution-preview-evidence.json"
})

meshclaw_reconcile_runbook({
  "verification_plan_evidence_path": "verification-plan-evidence.json"
})

meshclaw_reconcile_runbook_check({
  "runbook_evidence_path": "runbook-evidence.json"
})

meshclaw_reconcile_rollback_plan({
  "runbook_check_evidence_path": "runbook-check-evidence.json"
})

meshclaw_reconcile_completion_plan({
  "rollback_plan_evidence_path": "rollback-plan-evidence.json"
})

meshclaw_reconcile_readiness_summary({
  "completion_plan_evidence_path": "completion-plan-evidence.json"
})

meshclaw_reconcile_run_once({
  "desired_path": "desired-state.yaml",
  "actual_report_path": "node-report.json",
  "dry_run": true
})
```

The MCP tool has the same safety boundary: it stores evidence and rejects
`apply` or `execute`. `run-once` also requires an explicit dry-run flag until
the apply loop has policy-gated execution and verification evidence.

Each planned action is annotated with MeshClaw policy metadata:

```json
{
  "policy_action": "service_check",
  "policy_resource": "server",
  "policy_decision": "allow",
  "policy_reason": "read-only diagnostic action is allowed"
}
```

Future apply loops must consume these fields and stop on `require_approval` or
`deny` before running any command.

## Controller Shape

The first Go package should live under:

```text
internal/reconciler/
```

Minimal interface:

```go
type Reconciler interface {
    Reconcile(ctx context.Context, nodeID string) (Result, error)
}

type Result struct {
    RequeueAfter time.Duration
    Actions      []Action
    EvidenceID   string
}
```

Runtime loop:

```text
watch desired state -> enqueue node
periodic sync       -> enqueue node
agent update        -> enqueue node

workqueue
  -> serialize by node
  -> load desired intent
  -> load latest node report
  -> diff desired vs actual
  -> policy_check
  -> plan/apply_safe/approval queue
  -> evidence
  -> requeue with backoff
```

## Backoff And Safety

The controller must never hammer a broken node.

Rules:

- one reconcile at a time per node
- exponential backoff after failure
- ceiling on retries
- after repeated failure, create approval evidence instead of looping forever
- mutating actions require policy approval unless explicitly allow-listed
- destructive actions remain denied or strong-approval-only
- node-local agents remain read-only

Suggested initial defaults:

```text
periodic sync: 5m
minimum requeue: 30s
maximum backoff: 30m
failure threshold before approval queue: 3
```

## Data Boundary

This boundary is a product requirement:

| Store | Owns | Does Not Own |
| --- | --- | --- |
| meshdb | everything about the user and local computers: personal documents, conversations, scripts, projects, broad workspace memory | live server desired state, fleet drift, approval queue |
| opsdb | node snapshots, history, desired ops intent, drift, approvals, inventory meaning, evidence indexes | broad personal assistant memory |
| MeshClaw evidence | actions, plans, reports, audit trail | raw secrets |
| Guard/vault | secret handles and local vault references | model-visible raw credentials |

The reason is trust. A personal assistant can know many things, but DevOps
actions need a narrower, auditable state store. Server reports should not pull
in personal chat or unrelated desktop memory just because it looks semantically
similar. Personal summaries should not accidentally include fleet secrets,
private node state, or incident details.

When a DevOps agent needs broader context from meshdb, it should request a
bounded handoff:

```text
question -> allowed scope -> redacted excerpts -> evidence reference
```

The default path should stay inside `opsdb`.

## MVP Build Order

1. Add `internal/reconciler` types and tests.
2. Add read-only desired-state parser.
3. Add diff from desired node roles/services/capacity to cached agent report.
4. Emit plan-only actions and evidence.
5. Add `meshclaw reconcile plan --json`.
6. Add `meshclaw reconcile run-once --dry-run --json`.
7. Add policy check integration.
8. Add safe apply only for read-only refresh or already existing safe actions.
9. Add approval queue integration.
10. Add daemon loop after the dry-run path is stable.

The first milestone is not automatic repair. The first milestone is reliable
drift detection and plan generation that Codex, Claude, Open WebUI, or Argos can
explain to a human.
