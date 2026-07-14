# MeshClaw MCP Personal Assistant Upgrade

## Goal

Make MeshClaw strong enough that Claude, Codex, AI Studio, local models, and Signal can use MeshClaw directly as the assistant runtime instead of depending on a separate Open WebUI/OpenClaw-style wrapper.

The model should understand intent and choose tools. MeshClaw should own runtime state, policy, approval, evidence, artifacts, and execution.

## Product Surface

Expose task-level MCP tools, not only low-level UI primitives.

| Area | MCP surface | Runtime boundary |
| --- | --- | --- |
| Calendar | `meshclaw_calendar_list`, `meshclaw_calendar_create_event` | local macOS Calendar through Argos UI Runner |
| Reminders | `meshclaw_reminders_list`, `meshclaw_reminder_create`, `meshclaw_reminder_complete`, `meshclaw_reminder_delete` | local macOS Reminders |
| Contacts | `meshclaw_contacts_search` | read-only local Contacts |
| Apps/files/notifications | `meshclaw_automation_open_app`, `meshclaw_automation_open_url`, `meshclaw_automation_open_file`, `meshclaw_notification_show` | local user-visible handoff |
| App/settings handoff | `meshclaw_app_settings_plan`, `meshclaw_screen_capture`, `meshclaw_screen_record` | System Settings, app settings, privacy panes, and account-setting pages may be opened for review; toggles/saves/account changes need separate approval |
| Maps/directions | `meshclaw_maps_search`, `meshclaw_maps_directions`, `meshclaw_maps_proof`, `meshclaw_screen_capture`, `meshclaw_screen_record` | Apple Maps / Google Maps URL generation, optional visible open, and privacy-checked still screenshot or short recording when the user asks to see/capture the map |
| Media handoff | `meshclaw_media_play`, `meshclaw_screen_capture`, `meshclaw_screen_record` | music, radio, podcast, or video requests start as source-choice plans; approved execution opens a visible app/URL only |
| Audio transcription | `meshclaw_audio_transcribe`, `meshclaw_result_save` | local audio files can be transcribed with local Whisper; default is privacy review only, execution requires approval and writes a local transcript |
| Terminal/Shortcuts/results | `meshclaw_terminal_run`, `meshclaw_shortcut_text_run`, `meshclaw_result_save` | local command/Shortcut execution with saved Markdown/HTML result artifacts |
| Notes/dev log | `meshclaw_note_create`, `meshclaw dev-memo ...` | local Notes / `~/.meshclaw/dev-memos` |
| Mail | `meshclaw_mail_search`, `meshclaw_mail_summarize`, `meshclaw_mail_thread`, `meshclaw_mail_draft_reply`, `meshclaw_mail_compose`, `meshclaw_mail_send` | read/search/summarize/draft separated; send requires approval |
| Communication/alerts | `meshclaw_notification_show`, `meshclaw_reminder_create`, `meshclaw_schedule_status`, `meshclaw_schedule_run_once`, `meshclaw_scheduled_delivery_plan`, `meshclaw_scheduled_delivery_apply`, `meshclaw_signal_call_doctor`, `meshclaw_signal_call` | local notifications, reminders, morning briefings, scheduled friend/room delivery plans, approved schedule registration, and approved Signal voice-call flows |
| Account/subscription preflight | `meshclaw_account_action_plan`, `meshclaw_screen_capture`, `meshclaw_screen_record` | billing, cancellation, renewal, plan-change, and account pages may be opened for review; final account action requires a separate explicit approval |
| Transactional web tasks | `meshclaw_visible_browser_search`, `meshclaw_browser_fetch`, `meshclaw_automation_open_url`, `meshclaw_argos_ask`, `meshclaw_screen_capture`, `meshclaw_screen_record`, `meshclaw_result_save`, `meshclaw_account_action_plan`, `meshclaw_purchase_click`, `meshclaw_subscription_frontends` | shopping, booking, logged-in web, forms, and AI-frontends may be prepared and captured; final purchase click requires strong explicit approval |
| File cleanup planning | `meshclaw_downloads_cleanup_plan`, `meshclaw_downloads_cleanup_apply`, `meshclaw_result_save` | review-only Downloads cleanup candidates first; approved apply only moves explicit regular files to a review folder and never deletes/trashes/archives |
| Browser/publication | `meshclaw_browser_*`, `meshclaw_visible_browser_search`, `meshclaw_argos_research`, `meshclaw_news_document` | browser/search/docs via unified publish runtime |
| Documents/presentations/proof | `meshclaw_document_create`, `meshclaw_document_export`, `meshclaw_presentation_create`, `meshclaw_presentation_verify`, `meshclaw_presentation_edit`, `meshclaw_presentation_export`, `meshclaw_screen_capture`, `meshclaw_screen_record` | local Argos document bundle / DOCX/PDF export / PPTX deck creation, validation, editing, and PDF export / privacy-checked screen proof |
| Core | `mission_*`, `task_*`, `artifact_add` | MacBook canonical Core write |
| Runtime/fleet | `meshclaw_agent_*`, `meshclaw_opsdb_*`, `meshclaw_inventory_*` | node state, evidence, fleet |

## Safety Rules

- Read/list/search tools may execute directly and store evidence.
- User-visible UI actions default to plan-only unless the tool is explicitly a low-level primitive such as `meshclaw_automation_open_url`.
- Personal-data mutations default to plan-only: caller must pass `execute=true`.
- Terminal commands and Shortcuts require both `execute=true` and `approve=true`, and should save results under `~/.meshclaw/automation-results/`.
- Mail send, deletes, purchases, calls, booking/cancellation, and destructive operations require explicit approval.
- Signal calls are interruption-heavy: use `meshclaw_signal_call_doctor` first,
  then `meshclaw_signal_call` dry-run, and only place a real call when
  `execute=true` and `approve=true` are both present.
- Mail is split into read-only search/summary/body tools, draft-only compose/reply tools, and approval-required send/delete/move tools.
- Mac mini Signal remains the user-facing Argos runtime. MacBook Signal is only a private dev-memo sink if configured.
- Report/briefing rooms remain one-way/no-reply.

## Model Selection Toolsets

The catalog exposes model-friendly macOS toolsets so assistant models do not
have to infer everything from a flat tool list:

- `read_personal_data`: use Calendar, Reminders, and Contacts read-only tools
  before any generic Mac action.
- `mutate_personal_data`: use Calendar/Reminder/Notes mutation tools in
  plan-only mode first, then execute only after confirmation.
- `visible_handoff`: use app/file/URL/map/notification tools for visible handoff
  instead of Terminal or raw click automation. Use `meshclaw_media_play` for
  music/radio/podcast/video requests; it plans source choices first and
  approved execution only opens the selected visible app or URL. Use
  `meshclaw_app_settings_plan` for settings panes; it may open the visible
  settings surface but must stop before toggles, saves, sign-out, account
  connection/disconnection, or data deletion.
- `audio_transcription`: use `meshclaw_audio_transcribe` for local voice notes
  or recordings. Default `execute=false` returns a privacy/dependency plan;
  `execute=true` also needs `approve=true` and keeps the transcript local unless
  the user separately asks to send or attach it.
- `visible_map_handoff`: always return a clickable Maps URL; when the user asks
  to see the map, open Apple/Google Maps visibly and attach a short
  screen-proof artifact instead of replying with coordinates only.
- `communication_alerts`: use notification/reminder/schedule tools for ordinary
  alerts and morning briefings. Use `meshclaw_scheduled_delivery_plan` when the
  user asks to regularly send a message/report/voice note to a friend or room;
  it resolves the target and stores the first-run preview plan only. Use
  `meshclaw_scheduled_delivery_apply` only after the user approves that
  preview; it registers the job definition only and does not send the first
  message. Use Signal calls only after readiness and human approval.
- `transactional_web_handoff`: use browser/search/open/Argos visible work tools
  for shopping, booking, forms, and logged-in web tasks. It may compare options,
  prepare carts/forms, and capture proof, but it must stop before final
  purchase, payment, booking, subscription change, cancellation, or form
  submission unless using `meshclaw_purchase_click` with merchant, item, total,
  shipping, payment, checkout URL, final button coordinate, `execute=true`,
  `approve=true`, and exact confirmation text `구매 실행 승인`.
- `account_subscription_preflight`: use `meshclaw_account_action_plan` for
  billing, cancellation, renewal, plan-change, and account pages. It may open a
  visible page after approval, but final account actions need a separate
  explicit approval after summarizing service, action, cost/terms, and
  consequence.
- `file_cleanup_planning`: use Downloads cleanup planning before any file
  organization mutation. Plans may classify candidates and save a review
  report, but must not move, delete, rename, or archive files. Apply only with
  `meshclaw_downloads_cleanup_apply`, explicit selected file paths,
  `execute=true`, and `approve=true`; the apply step only moves regular files
  to a review folder.
- `durable_os_work`: use Terminal/Shortcuts tools when an OS action must leave a
  Markdown/HTML artifact, and use `meshclaw_result_save` after browser/app work.
- `bounded_ui_fallback`: use `meshclaw_argos_ask` and screen recording only when
  no first-class tool matches; coordinate clicks are advanced fallback.

The expected model behavior is: pick the narrowest task-level OS tool, keep
mutations plan-only by default, save durable results, and use generic UI
automation only as a last-mile fallback.

## OpenClaw and Hermes Parity Baseline

MeshClaw should match the useful assistant surface of OpenClaw and Hermes while
keeping MeshClaw's policy, evidence, and approval boundary as the higher-level
runtime contract.

OpenClaw reference shape observed from the g1 install:

- Gateway config lives under `~/.openclaw/openclaw.json`.
- Workspace identity is Markdown-backed: `IDENTITY.md`, `SOUL.md`, `USER.md`,
  `AGENTS.md`, `TOOLS.md`, `BOOTSTRAP.md`, and `HEARTBEAT.md`.
- Skill installation state is tracked through ClawHub under `~/.clawhub`.
- Gateway settings include mode, bind, port, auth, allowed control UI origins,
  and device-auth posture.
- The product model is multi-channel assistant gateway plus skills, voice,
  browser/canvas, local tools, and node pairing.

Hermes reference shape:

- Profile config lives under `~/.hermes`.
- `SOUL.md` is the primary identity file.
- Long-term memory is bounded Markdown/SQLite-backed memory with explicit write
  approval.
- Skills can be bundled, installed, or agent-created, with write approval.
- Platform gateways include chat services such as Telegram, Discord, Slack,
  WhatsApp, Signal, email, and webhooks.
- Cron jobs, subagents, browser automation, terminal backends, MCP servers, and
  provider/model routing are first-class settings.

MeshClaw parity does not mean giving the assistant direct unrestricted host
control. The MeshClaw equivalent is:

| OpenClaw/Hermes area | MeshClaw equivalent | Required boundary |
| --- | --- | --- |
| Identity/persona | `assistant.identity` files and deterministic Signal identity answer | User-facing Argos name, Mac mini runtime, MeshClaw control plane |
| Memory | bounded user/assistant Markdown memory plus evidence/session search | Memory writes require approval; raw secrets stay in Guard/Vault |
| Skills | allowlisted `~/.meshclaw/skills` and bundled toolsets | Skill install/write requires approval and test contract |
| Gateway/platforms | Signal, Open WebUI, MCP, and future webhook/channel targets | Per-target mode and delivery policy |
| Schedules/cron | MeshClaw scheduler and scheduled delivery plans | Register only after preview approval |
| Browser/web | visible browser/search/fetch/research tools | Stop before purchase, booking, form submit, account change |
| Voice | edge-tts default voice notes, transcription, ops/news voice briefs | Report rooms stay one-way; assistant/chat may reply |
| Subagents/workers | Codex/thread workers and MeshClaw task/work queues | Record task, actor, evidence, and result |
| Terminal/backends | local/vssh/SSH/Mac runner tools | Mutations/destructive ops require Ops/MCP approval |
| MCP/tool routing | model-friendly MeshClaw MCP tools | Destructive/direct tools hidden from unsafe profiles |

Implemented parity slices:

- `meshclaw assistant profile` inspects the Argos identity, bounded memory, and
  reviewed skill roots without creating or executing skills.
- `meshclaw assistant profile init --execute` creates default Argos identity and
  bounded memory files without overwriting existing files.
- `meshclaw assistant memory add --text ...` is plan-only by default, and
  `--approve` appends a bounded Markdown memory entry after Guard rejects
  raw-secret-like text.
- `meshclaw assistant memory snapshot --text ...` reports the short-term,
  working, long-term, episodic, semantic, procedural, and ops memory layers
  that should be used for a request. In Signal, `메모리 구조` shows the same
  architecture in a mobile-readable form.
- Signal memory layer contract:
  - Short-term memory is Signal room history for immediate follow-ups such as
    `방금`, `아까`, `그거`, or `이어서`; it is room-scoped and not permanent
    truth.
  - Working memory is pending approvals and recent artifacts/mail/news contexts
    for requests such as `승인`, `저장해`, `다시 보내`, or `1번`; it expires
    unless promoted to an artifact or evidence record.
  - Long-term memory is only approved `~/.meshclaw/memory/USER.md` and
    `MEMORY.md` content, mainly user preferences and Argos answer habits. For
    `내 답변 선호 기억해?`, Signal should use `long_term`.
  - Episodic memory is redacted evidence/opsdb history for past sends, reports,
    work results, and incidents. For `지난번 서버 보안 보고 기억해?`, Signal
    should use `long_term`, `episodic`, and `ops`.
  - Procedural memory is reviewed skills and MCP tool contracts for repeated
    workflows and tool-choice habits.
  - Ops memory is opsdb/monitor/agent history for node status, security, ports,
    processes, logs, and DevOps reports. It stays separate from personal
    assistant memory.
  - Identity/capability questions such as `넌 누구고 뭘 할 수 있어?` should
    answer from the Argos identity contract: Argos is the Signal-facing
    assistant, the Mac mini is the user-facing runtime, and MeshClaw is the
    control plane for tools, policy, approvals, and records.
- `meshclaw assistant self-test` includes a `프로필/메모리/스킬` check so
  Signal-visible health covers the OpenClaw/Hermes identity-memory-skill axis.
- The `local-lite` MCP profile is assistant-first: `meshclaw_local_assistant`
  is first, and destructive/direct tools are not listed or callable by name
  through `tools/call`.

The target settings model is:

```yaml
assistant:
  id: argos
  display_name: Argos
  language: ko
  identity_files:
    soul: ~/.meshclaw/assistant/SOUL.md
    user: ~/.meshclaw/assistant/USER.md
    instructions: ~/.meshclaw/assistant/AGENTS.md

channels:
  signal:
    targets:
      - id: argos-assistant
        mode: assistant
        delivery_policy: private_replies
      - id: argos-chat
        mode: chat
        delivery_policy: private_replies
      - id: argos-briefing
        mode: briefing
        delivery_policy: one_way_reports
      - id: argos-ops
        mode: ops
        delivery_policy: one_way_reports

memory:
  user_profile:
    path: ~/.meshclaw/memory/USER.md
    write_approval: true
  assistant_notes:
    path: ~/.meshclaw/memory/MEMORY.md
    write_approval: true
  ops_facts:
    source: evidence
    writable_by_assistant: false

skills:
  roots:
    - ~/.meshclaw/skills
  write_policy:
    approval_required: true

approvals:
  default: deny_for_mutation
  assistant:
    read_personal: allow
    send_or_modify: require_approval
  ops:
    read_only: allow
    mutate: require_approval
  guard:
    raw_secret: deny
```

## Implementation Order

1. Expose first-class personal assistant MCP tools for calendar/reminders/contacts/notes.
2. Improve descriptions so models choose these tools instead of generic `argos_ask`.
3. Add `dev-memo` MCP wrappers after the CLI stabilizes.
4. Add browser visible-task tools for "open site, search, extract, summarize" workflows.
5. Add document tools for Markdown/Obsidian/docx/pdf export.
6. Add terminal, Shortcuts, and explicit result-save tools so app work leaves durable artifacts.
7. Tighten public install UX: one-machine assistant first, multi-node fleet optional.

## Current Status

Initial first-class personal assistant MCP tools are registered. Calendar/reminder/note mutations return plan-only by default and only mutate when `execute=true`.

Visible work tools are now exposed as first-class MCP tools:

- `meshclaw_visible_browser_search`: plan or open a local browser search, then collect structured result evidence.
- `meshclaw_document_create`: plan or create a local Argos Markdown/HTML/DOCX document bundle.
- `meshclaw_document_export`: plan or export Markdown to DOCX/PDF.
- `meshclaw_presentation_create`: plan or create a PowerPoint/PPTX deck plus Markdown outline, then verify the PPTX package.
- `meshclaw_presentation_verify`: read-only validation that a local PPTX has presentation metadata and slides.
- `meshclaw_presentation_edit`: plan or create a verified edited copy of a PPTX with added slide content and optional backup.
- `meshclaw_presentation_export`: plan or export PPTX to PDF through LibreOffice/soffice after verification.
- `meshclaw_screen_capture`: plan or capture a still local screenshot for map photos, shopping/account review, forms, or AI frontend answers. The default plan includes purpose, output, Korean user guidance, and a privacy checklist; actual capture requires `execute=true` plus `approve=true`.
- `meshclaw_screen_record`: plan or capture a short local screen recording through Argos UI Runner. The default plan includes purpose, output, Korean user guidance, and a privacy checklist; actual recording requires explicit execution.
- `meshclaw_media_play`: plan media playback source choices and, after explicit approval, open the selected app or URL without clicking final play, purchasing, subscribing, or changing accounts.
- `meshclaw_app_settings_plan`: plan app/System Settings handoff and, after explicit approval, open the selected app, settings pane, or URL without toggling permissions, saving settings, signing out, connecting/disconnecting accounts, or deleting data.
- `meshclaw_account_action_plan`: plan billing/subscription/account-management requests and, after explicit approval, open the selected page without canceling, paying, changing plans, deleting accounts, or submitting forms.
- `meshclaw_purchase_click`: execute one final approved purchase/order button click on a visible shopping checkout page. Requires merchant, item, total, shipping, payment, checkout URL, button coordinate, `execute=true`, `approve=true`, and confirmation exactly `구매 실행 승인`; otherwise it returns an approval-required plan.

Signal assistant artifact follow-ups should remember the recent document or
presentation for that assistant room. Requests such as `방금 만든 PPT를 3장으로
줄여줘` or `방금 만든 문서를 더 짧게 다듬어줘` should create a new edited
artifact and attach it back, without asking for a local path and without
mutating the original file in place.

Result-generating macOS work tools are also exposed:

- `meshclaw_terminal_run`: plan or run a local shell command, then save stdout/stderr as Markdown/HTML.
- `meshclaw_shortcut_text_run`: plan or run a named macOS Shortcut with text input, then save output as Markdown/HTML.
- `meshclaw_result_save`: save a synthesized work result after browser/app/terminal work so the assistant leaves an artifact, not just UI state.
- `meshclaw_maps_search`: return a usable Apple/Google Maps place-search URL, optionally opening it; pair with `meshclaw_screen_capture` for a map photo/capture, or `meshclaw_screen_record` for short video proof.
- `meshclaw_maps_directions`: return a usable Apple/Google Maps directions URL for origin/destination/mode, optionally opening it; pair with `meshclaw_screen_capture` when the user asks to see the route.
- `meshclaw_maps_proof`: return the Maps URL and, after `execute=true` plus `approve=true`, open the visible map and capture a still screenshot proof.

Publication tools now distinguish source-grounded reports from search-result
notes. `meshclaw_argos_research` tries to fetch accessible source page text and
creates a cited Korean summary with `[S1]` source labels only when enough source
content exists. If it only has search snippets, it should produce a conservative
source-candidate note and ask follow-up questions instead of pretending to have
verified a long-form report.

Mail tool descriptions now explicitly separate read-only discovery, read-only body reads, draft-only writing, and approval-required transmission/mutation so models can choose the correct tool without a deterministic intent router.

Signal assistant replies are now expected to be natural-language first. For
artifact requests, Argos should attach the created files and hide raw local
paths unless asked. For Calendar, Reminders, Mail, Maps, and research results,
Argos should return concise Korean text without internal action names, evidence
paths, JSON, or UTC/RFC3339 timestamps.

Mail summary requests such as `최근 메일 요약해줘` are read-only and should use
configured accounts by default. The assistant should ask which account only for
send/move/delete/download actions or when the user explicitly scopes an account.
After a mail summary or search, Signal Argos may accept short follow-ups such as
`첫 번째 메일에 답장 초안 써줘` or `2번 메일에 정중하게 회신 초안 만들어줘`.
That follow-up is draft-only: it uses the recent account/message reference,
creates an unsent local draft, and still requires separate explicit approval
before any transmission. Signal replies should say the draft was saved but
should not expose the local draft file path unless the user explicitly asks for
diagnostics.

These are intended to let Claude/Codex/AI Studio/local models choose concrete MeshClaw tools directly instead of falling back to an external wrapper UI.
