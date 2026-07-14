# Argos Commercial Assistant Baseline

Last updated: 2026-06-21

This document defines the product baseline that should be kept while turning
Argos into a commercial everyday Signal assistant.

Use this with `docs/ASSISTANT_PRODUCT_DIRECTION.md` and
`docs/ARGOS_COMMERCIAL_ASSISTANT_CONTRACT.md`. If an older contract, scenario,
handoff, or automation document suggests shopping, Coupang, purchase, payment,
or transaction automation as core assistant behavior, treat that guidance as
historical and out of scope.

## Commercial Baseline To Keep

Argos already has useful product foundations:

- A user-facing Signal assistant identity backed by MeshClaw Platform, with the
  Mac mini as the only user-facing Signal runtime/sender.
- A clear interface boundary: Signal is the primary daily Argos assistant UX;
  Claude, Cursor, Codex, and MCP remain developer, operator, or power-user
  integration surfaces.
- A concise capability card for `뭐 할 수 있어?` that shows real commands instead
  of internal feature names.
- Visible reply contracts that keep normal Signal replies phone-readable and
  avoid local paths, evidence IDs, exit status, attachment markers, and raw
  diagnostics.
- Daily-use assistant axes: memory, skill creation/activation, reusable work
  cards, and natural-language scheduled delivery plans.
- Useful first workflows: mail priority summaries, daily agenda-style planning,
  industry news briefs, DOCX/PPTX meeting material packages, and scheduled brief
  delivery approval.
- Regression coverage for assistant-room replies, including no false claims that
  mail, booking, purchase, payment, or scheduled delivery actions completed when
  they only reached a plan or approval step.

These are commercial product assets. Productization work should make them more
reliable and easier to use before adding new domains.

Commercial product work should prioritize the Signal UX first. MCP, Claude,
Cursor, and Codex integration should remain strong for advanced users and
operators, but those surfaces should not make a model provider or worker the
assistant identity. Argos Assistant is the product identity; MeshClaw Platform is the shared core
that supplies approval, evidence, execution, OS/macOS tools, policy, and guarded
dispatch/deploy capabilities behind it.

## Product Readiness Gate

Before treating a Signal assistant command as product-ready, it should pass this
gate:

1. The command is a natural user sentence, not an internal tool name.
2. The first reply is a short useful card with a clear next action.
3. Read-only work runs without unnecessary approval.
4. State-changing work stops at an approval or registration card.
5. The visible reply does not expose local paths, evidence IDs, exit status,
   router names, raw logs, or attachment markers.
6. The reply does not claim a send, booking, purchase, payment, deletion, or
   schedule registration happened unless it actually did.
7. The behavior has focused regression coverage.
8. If the behavior depends on deployed Signal routing, Mac mini live Signal is
   verified only after an intentional deploy.

## Non-Baseline Work

The following are not part of the commercial baseline:

- Shopping, Coupang, purchase finalization, payment, checkout, reorder, receipt,
  return, refund, or transaction automation.
- MacBook user-facing Signal sending or MacBook launchd automation.
- Approval-heavy UX for read-only summaries, research, document drafts, or
  reusable-work suggestions.
- Broad scenario catalog expansion before the first everyday Signal commands are
  reliable.

Read-only product research can stay, but it should be framed as research,
comparison, drafting, or reminder setup. It must not advertise automatic
purchase or payment completion.

## Next Productization Priorities

1. Make the first ten Signal commands from `docs/ASSISTANT_PRODUCT_DIRECTION.md`
   repeatedly pass the visible-reply and Signal regression contracts.
2. Replace historical purchase-oriented scenario guidance with research-only or
   explicitly out-of-scope wording where it appears in current docs and tests.
3. Tighten onboarding around one quick-start loop: capability card, memory
   preference, industry brief, document package, scheduled delivery plan, and
   automation status.
