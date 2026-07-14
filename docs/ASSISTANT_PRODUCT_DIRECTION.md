# Argos Assistant Product Direction

Last updated: 2026-06-21

This document is the current product direction for Argos/MeshClaw assistant
work. If older roadmap, scenario, automation, or handoff documents conflict,
use this direction first.

## Product Goal

Argos should become a Hermes/OpenClaw-like everyday assistant in Signal: one
assistant the user can message naturally and use every day for useful work.

MeshClaw remains the runtime behind Argos. It provides tools, policy,
scheduling, memory, evidence, and adapters, but the user-facing product is not a
sprawling automation/control-plane demo. The product succeeds when Signal feels
like a short, capable assistant conversation.

## Product Split

MeshClaw is the backend runtime and control plane: approval boundaries,
evidence, execution, OS tools, deployment guardrails, runtime health, policy, and
integration safety.

Argos is the user-facing assistant product: a Hermes/OpenClaw-like Signal
assistant with concise cards, memory, reusable skills, repeatable work,
natural-language scheduling, macOS tools, and clear next actions.

Model providers are implementation detail behind Argos. Codex, Claude, Gemini,
local models, or future providers may act as workers or reasoning models, but
the user-facing UX, reply contract, and assistant identity must not be tied to
Codex or any single provider.

Commercial success means Argos becomes useful in daily Signal use. MeshClaw
succeeds by being reliable invisible infrastructure behind that experience.

## Current Interface Surfaces

Argos is the user-facing assistant identity across the current interfaces.
Signal is the primary daily assistant room and the first commercial UX to make
excellent.

Current surfaces:

- Signal: primary user-facing Argos assistant room for daily work, concise
  replies, memory, skills, reusable work, scheduling, and briefings.
- Claude, Cursor, and Codex: developer, agent, and operator surfaces that may
  call MeshClaw or help build Argos, but must not become the product identity.
- MCP: the tool/runtime integration surface for capable clients and power users.

MeshClaw provides the runtime behind all surfaces: approvals, evidence,
execution, OS/macOS tools, policy, dispatch/deploy guardrails, and integration
safety. Models and workers are swappable behind Argos. No surface should make
Codex, Claude, Cursor, or any other worker look like the assistant product.

## Success Criterion

The success path is:

```text
user sends a natural Signal request
-> Argos understands the intent
-> Argos returns a short useful response
-> the response includes the next action the user can take
```

Good replies are concise cards. They do not expose local paths, evidence IDs,
exit statuses, internal router names, raw diagnostic dumps, or implementation
details unless the user explicitly asks for diagnostics.

## First Product Loop

Build around ten daily-use assistant commands, verified in the live Mac mini
Signal path:

1. `뭐 할 수 있어?`
2. `최근 메일 요약해줘. 답장 필요한 것만 5개 뽑아줘`
3. `오늘 일정, 할 일, 메일, 날씨를 하루 계획으로 묶어줘`
4. `제약회사 최신 뉴스 찾아서 정리해줘`
5. `제약회사 최신 뉴스 DOCX/PPTX 회의자료로 만들어줘`
6. `매일 오전 8시에 보고방에 제약/바이오 최신 뉴스 브리프 보내줘`
7. `답변은 결론부터 짧게 한다고 기억해`
8. `기억 확인`
9. `제약/바이오 최신 뉴스 브리프를 스킬로 만들어줘`
10. `자동화 목록`

These commands should demonstrate four assistant axes:

- Memory: remember stable preferences and use them in future replies.
- Skill: turn repeatable work into a reusable assistant skill.
- Reusable work: let the user repeat a useful workflow without rebuilding it.
- Natural-language schedule: register and inspect scheduled assistant work from
  plain language.

## What To Avoid

- Do not turn the product back into a Codex-style endless development loop.
- Do not grow huge scenario catalogs before the core ten commands work in live
  Signal.
- Do not make approval-heavy UX the default solution. Read-only summaries,
  research, drafts, and document preparation should execute directly; mail send,
  schedule registration, external sharing, deletion, and other state-changing
  actions need approval.
- Do not revive shopping, payment, Coupang, purchase finalization, or
  transaction automation. Keep that out of scope unless the user explicitly
  starts a separate project for it.
- Do not leak internal diagnostics into normal assistant replies.
- Do not revive MacBook user-facing Signal sending or MacBook launchd
  automation. The Mac mini remains the user-facing Signal runtime/sender.

## Development Method

Work one real user scenario at a time:

1. Pick one natural Signal request from the first product loop.
2. Reproduce the failure or awkward reply.
3. Fix the smallest routing, wording, or tool behavior needed.
4. Add a focused regression.
5. Verify the live Signal behavior when deployment is actually needed.

Avoid broad rewrites and parallel feature expansion. A future agent should be
able to answer: "Which user sentence got better, and what does the user now see
in Signal?"
