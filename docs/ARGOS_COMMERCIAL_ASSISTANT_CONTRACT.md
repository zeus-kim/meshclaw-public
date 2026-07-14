# Argos Commercial Assistant Contract

Last updated: 2026-06-21

This is the acceptance contract for turning Argos into a commercial
Hermes/OpenClaw-like Signal assistant.

Use this with `docs/ASSISTANT_PRODUCT_DIRECTION.md` and
`docs/ARGOS_COMMERCIAL_BASELINE.md`. If an older contract, scenario, handoff, or
automation document conflicts with this file, treat the older guidance as
historical unless the user explicitly reopens that area as a separate project.

## Product Acceptance Contract

Argos is accepted as a commercial assistant when a normal Signal user can send a
natural request and receive:

1. a concise useful response card;
2. the next action they can take;
3. no internal path, evidence ID, exit status, router name, raw log, or test
   artifact in the visible reply;
4. a clear stop before state-changing work such as mail send, schedule
   registration, external sharing, deletion, or device control;
5. direct execution for read-only summaries, research, drafts, and document
   preparation without unnecessary approval ceremony.

The default user-facing loop is:

```text
Natural Signal message
  -> Argos understands the request
  -> Argos uses MeshClaw tools only where useful
  -> Argos returns a short card and next command
```

## Product Boundary

Argos is the commercial assistant product. It owns the user-facing Signal
experience, concise cards, memory, skills, reusable work, natural-language
scheduling, and macOS-assisted workflows.

MeshClaw Platform is the shared core behind Argos Assistant. It owns approvals,
evidence, execution, OS/tool adapters, policy, deployment/runtime guardrails,
and operational reliability. Those internals should be visible only through
explicit diagnostics or operator workflows, not normal assistant replies.

Codex, Claude, Gemini, local models, and future providers may be selected behind
Argos as workers or models. The commercial contract is provider-neutral: Argos
must remain the assistant product even when the model changes.

Commercial acceptance is measured by repeated daily Argos use. MeshClaw wins when
that use feels dependable and the infrastructure fades into the background.

## Four Commercial Axes

The first product loop must demonstrate four assistant axes:

- Memory: approved stable preferences can shape later replies.
- Skills: useful repeatable work can become a reusable assistant skill.
- Reusable work: a completed workflow can be repeated without rebuilding it.
- Natural-language scheduling: the user can register and inspect scheduled
  assistant work from plain language.

These axes are product behavior, not internal architecture demos.

## macOS-As-Tool Contract

Argos may use macOS as a tool when the action is safe, explicit, and helpful:

- opening a URL or app the user named;
- preparing browser research;
- reading visible state for a user-requested task;
- creating drafts, previews, or candidate plans;
- using Argos UI Runner for bounded local actions.

Argos must stop before risky device changes, external submissions, payments,
deletions, sends, or sharing unless the relevant approval contract exists. The
Mac mini remains the user-facing Signal runtime/sender. MacBook user-facing
Signal send and MacBook launchd automation remain disabled.

## Out Of Scope

The following are not part of the current commercial assistant contract:

- shopping, Coupang, checkout, payment, reorder, refund, receipt, or purchase
  automation;
- trading or other financial execution;
- internal evidence/reporting flows as the normal user experience;
- endless Codex-style development loops;
- huge scenario matrix expansion before the first everyday Signal commands are
  reliable.

Read-only product research, comparison, summaries, reminders, and drafts may
remain. They must not advertise that Argos can complete purchases, payments, or
transactions.

Future transaction support requires a separate official-API integration,
explicit risk model, approval contract, rollback/audit story, and user request
to reopen the scope.

## Acceptance Evidence

Acceptance should come from a small core of live Signal commands and focused
regression tests, not from a large scenario catalog.

Current acceptance candidates are the first ten commands in
`docs/ASSISTANT_PRODUCT_DIRECTION.md`, covering capability discovery, mail,
daily planning, industry research, document packages, scheduled delivery,
memory, skills, reusable work, and automation status.

For each accepted command, keep evidence of:

1. the natural user sentence;
2. the visible Signal reply contract;
3. the approval boundary, if any;
4. the focused regression that protects it;
5. live Mac mini Signal verification only when an intentional deploy was needed.
