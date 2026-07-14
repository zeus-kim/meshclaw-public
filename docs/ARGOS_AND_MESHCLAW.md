# Argos and MeshClaw

Last updated: 2026-06-21

This document defines the canonical product split between Argos and MeshClaw.
If another document conflicts with this split, this document wins.

## Core Identity

Argos is the human-facing assistant product.

MeshClaw Platform has three product lines that share a common core:

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

All three share MeshClaw's policy, state, workflow, MCP, and evidence core.

```text
Human
  -> Signal / Claude / Cursor / Codex / MCP-capable clients
     -> Argos identity OR MeshClaw integration surface
     -> MeshClaw Router / MeshClaw MCP
        -> MeshClaw Platform shared core
           -> policy / approvals / evidence / guard / workflows
           -> OS/macOS tools / vssh / browser / mail / calendar / files / Signal / model APIs
```

Argos is not just a model. Argos is the assistant identity and product that can
talk through Signal, Claude, Cursor, Codex, MCP-capable clients, local chat,
audio calls, and later desktop/mobile surfaces. Signal is the primary daily
user-facing assistant room; Claude, Cursor, and Codex are developer, agent, and
operator surfaces; MCP is the tool/runtime integration surface.
Argos understands the user's request, chooses the right lane, and returns a
concise card with the next action in the user's channel.

MeshClaw is not the personality. MeshClaw owns facts, permissions, execution,
state, audit trail, OS/macOS tools, deployment/runtime guardrails, dispatch
guardrails, policy, approvals, evidence, and safety behind every surface.

Model providers are swappable behind Argos. Codex, Claude, Gemini, local models,
and future providers may be workers or reasoning models, but the user-facing
assistant contract belongs to Argos and must not depend on one provider's brand,
subscription, or UI.

Commercial success is daily Argos use. MeshClaw reliability is the invisible
infrastructure that makes that use safe and dependable.

MeshClaw may be used by a broader personal assistant system, but DevOps control
state must stay separate from broad personal memory. meshdb can index documents,
scripts, conversations, and whole-computer context. `opsdb` is the MeshClaw
DevOps state store and should hold only operational facts: node reports,
inventory meaning, desired ops intent, drift, policy decisions, approvals, and
evidence indexes.

## Names

| Name | Meaning |
| --- | --- |
| MeshClaw Platform | The shared core: policy, approvals, evidence, OS tools, deployment/runtime guardrails, workflows, Guard, vssh-backed execution |
| Argos Assistant | The user-facing assistant product line: Signal, Claude, Cursor, Codex, MCP-capable clients, local chat, calls, briefings, mail, calendar, documents |
| MeshClaw Ops | The server operations product line: server, VM, Docker operations automation, logs, security, vssh, autoheal, evidence |
| MeshClaw Agent | Lightweight agent installed on server nodes for local collection and execution |
| MeshClaw Router | The routing layer used by Argos/Open WebUI/local models before any model answers |
| MeshClaw MCP | The tool/runtime integration surface used by Codex, Claude, Cursor, and capable AI clients |
| Guard | Security feature within MeshClaw Platform: secret and password entry, local-only handling, vault handles |

## Human Entry Points

### Current Interface Model

Signal is the commercial priority because it is the daily user-facing Argos
assistant room. It should get the cleanest replies, onboarding, memory, skills,
reusable work, scheduling, and briefings first.

Claude, Cursor, and Codex are power-user/developer/operator surfaces. They may
run workers, review code, investigate incidents, or call MeshClaw MCP, but the
assistant product identity remains Argos. Codex is an implementation worker, not
the product brand.

MCP is the integration surface beneath capable clients. It exposes MeshClaw
runtime capabilities such as approvals, evidence, execution, policy, OS/macOS
tools, and guarded dispatch/deploy flows without forcing users to treat a
particular model or worker as the product.

### Future Channel Adapters

Argos should stay channel-agnostic. Signal is the current primary daily UX and
acceptance gate, but later Discord, Telegram, Slack, or similar adapters should
attach to the same Argos core contract instead of forking product behavior per
channel.

Channel adapters should normalize:

- inbound messages into the Argos request contract;
- outbound concise cards into each channel's formatting and delivery limits;
- attachments and generated artifacts into channel-safe delivery forms;
- approval replies and follow-up actions into the shared MeshClaw approval flow;
- delivery constraints such as rate limits, threading, room identity, and
  one-way/no-reply targets.

MeshClaw remains the shared runtime behind all channels: approval, evidence,
execution, macOS tools, policy, guardrails, scheduling, and dispatch safety.
Future channel work should document and implement adapters only after the Signal
assistant contract is useful and stable; do not implement Discord, Telegram, or
Slack integrations as part of the current documentation-first pass.

### Codex, Claude, Cursor

Use these for high-agency development, orchestration, refactoring, reviews, and
expert server work.

```text
Human
  -> Codex / Claude / Cursor
     -> MeshClaw MCP
        -> MeshClaw runtime
```

Codex and Claude are strong tool-using agents. They can call MeshClaw MCP
directly. MeshClaw should not proxy their subscription sessions behind Signal.

### Open WebUI

Use this as the local-model workbench.

```text
Human
  -> Open WebUI
     -> MeshClaw Router
        -> MeshClaw direct tools OR selected local/API model
```

In Open WebUI, users should select `MeshClaw Router` as the default model.
Directly selecting `gemma`, `gpt-oss`, `qwen`, or `claude-*` bypasses the
Router and can produce fake tool calls or "I cannot inspect servers" answers.

### Signal

Use this as the mobile assistant, report, call, and approval surface.

```text
Human
  -> Signal
     -> Argos
        -> MeshClaw Router
           -> MeshClaw runtime OR local model
```

Argos is modeled as one Signal person. Users create or choose rooms and invite
Argos. MeshClaw binds each room to a target mode such as assistant, ops, guard,
or briefing.

## Router Lanes

MeshClaw Router decides the lane before any model responds.

Router is a hybrid classifier:

- deterministic rules handle obvious operations and approval cases quickly;
- a small local model may classify ambiguous natural language into JSON intent;
- the classifier never executes actions and never overrides MeshClaw policy;
- policy converts the route into allow, approval-required, or deny.

| Lane | Used For | Example |
| --- | --- | --- |
| `meshclaw:direct` | Read-only facts and safe runtime queries | server status, weather, evidence, Open WebUI status |
| `meshclaw:approval` | Mutating or sensitive actions that need approval | send email, restart service, Signal call, delete data |
| `meshclaw:deny` | Blocked actions | reveal secret, read `/etc/shadow`, destructive unsafe commands |
| `model:local_chat` | Ordinary explanation, writing, reasoning | who is Yi Sun-sin, rewrite this text |
| `model:reasoning` | Larger local/API model for harder reasoning | architecture critique, long summary |
| `guard:local_only` | Secret handling without raw exposure to cloud tools | store token, rotate password, create vault handle |
| `handoff:codex_claude` | Expert app handoff when local models are not enough | deep refactor, complex incident review |

The first implemented lanes are `meshclaw:direct`, `meshclaw:approval`,
`meshclaw:deny`, and `model:local_chat`. The other lanes are product targets.

## Product Rule

The user should not have to choose a model or tool for normal work.

The user should ask Argos or MeshClaw Router naturally:

```text
show server status
show today's weather
check g4 Open WebUI status
who is Yi Sun-sin?
clean old checkpoints on d1
summarize the latest Codex work

Korean examples are also supported:
서버 상태 알려줘
오늘 날씨 알려줘
```

Router chooses the lane:

- direct factual operations run through MeshClaw;
- ordinary conversation goes to a local model;
- risky work creates approval evidence;
- secret work stays in Guard;
- expert work is handed off to Codex/Claude when needed.

## Why This Exists

Codex and Claude can already do a lot by themselves. MeshClaw exists because
infrastructure work needs shared truth, policy, repeatability, approvals, and
evidence across many AI frontends.

Local models and Open WebUI cannot reliably decide when to call MCP tools.
Therefore MeshClaw Router is the default entrance for local models and Argos.

Argos exists because humans need a natural assistant surface: Signal messages,
briefings, calls, local app automation, and simple status questions. Argos uses
MeshClaw so that assistant behavior remains safe, auditable, and useful.

## Reconciliation Direction

Argos may help a human express intent in natural language:

```text
keep Open WebUI healthy on g4
prefer idle GPU nodes for model jobs
do not page me for s1 when it is intentionally powered off
```

MeshClaw must convert that into structured desired ops intent before acting.
The reconciler then compares desired intent with node-local agent state and
produces a plan, approval request, or safe action. This is the Kubernetes
controller pattern adapted to mixed personal infrastructure, not a generic
assistant making unbounded changes.

The detailed controller plan is `docs/RECONCILIATION_CONTROLLER.md`.
