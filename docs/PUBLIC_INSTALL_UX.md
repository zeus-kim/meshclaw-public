# MeshClaw Public Install UX

## Product Shape

MeshClaw should work for a normal user with one always-on Mac first.

```text
Phone / desktop AI client
  -> Argos Signal identity or MeshClaw MCP
  -> MeshClaw Runtime on one Mac
  -> policy / evidence / approvals / local app automation / publication
```

Advanced users may install MeshClaw or vssh agents on more machines, but that is
an extension, not the default mental model.

## User Profiles

### One-Machine Assistant

This is the public default.

- Install MeshClaw on one Mac that can stay awake.
- Link one Signal account as Argos.
- Use one Argos Signal identity for assistant/chat/reporting.
- Use `meshclaw setup signal` and `meshclaw setup argos-runner` for onboarding.
- Use MCP from Claude/Codex/AI Studio only as another client of the same Runtime.

The user should not need to understand Core/Mission/Runtime terms to start.
Those are internal architecture boundaries.

### Multi-Node Ops

This is for operators with several servers.

- Keep one controller Runtime as the source of inventory, policy, evidence, and
  reports.
- Add vssh/MeshClaw agents on nodes only when local collection or execution is
  needed.
- MCP clients call the controller tools: `meshclaw_server_list`,
  `meshclaw_agent_workloads`, `meshclaw_ops_control`, and workflows.
- Do not ask every node to become a separate assistant.

### Development Topology

This repo currently dogfoods a more complex split:

- Mac mini: user-facing Argos Signal Runtime and scheduled reports.
- MacBook: development controller, Codex/Claude/Cursor/AI Studio MCP clients,
  and canonical Mission writes.

This is not the public default. It is an operator-specific topology for
development and verification.

## First Run

```sh
pip install -U meshclaw vssh
meshclaw init
meshclaw setup assistant --json
meshclaw model-config status --json
meshclaw quickstart --json
meshclaw setup signal --json
meshclaw setup argos-runner --json
meshclaw mcp
```

The first-run UX should answer four questions:

- Is MeshClaw installed and reachable?
- Is Signal linked and is the dispatcher healthy?
- Is Argos UI Runner ready for bounded macOS automation?
- Which chat model gateway will Signal/MCP-backed assistant rooms use?
- Which MCP tools should Claude/Codex/AI Studio/local models use first?

`meshclaw setup assistant --json` is the product onboarding summary. It is
read-only: it combines doctor, Signal, Argos Runner, MCP catalog, and the first
successful tasks without installing daemons or sending messages. If it reports
problems, the user can then run focused commands such as
`meshclaw setup signal --repair --json` or
`meshclaw setup argos-runner --json`.

## Model Gateway Paste-In

MeshClaw expects a shared OpenAI-compatible chat gateway for Signal chat and
assistant rooms. Local Ollama can stay as the default, and GPT/Claude can be
added later through a gateway that exposes `/v1/chat/completions`.

The user-facing paste-in shape is:

```json
{
  "base_url": "https://YOUR_API/v1",
  "api_key": "YOUR_GATEWAY_KEY",
  "model": "gpt-4.1",
  "max_tokens": 4096,
  "temperature": 0.45
}
```

Apply it on the Runtime Mac:

```sh
meshclaw model-config import --json < model-config.json
meshclaw model-config test "연결 테스트" --json
```

The status command and MCP tool only return a masked key:

```sh
meshclaw model-config status --json
```

MCP clients should use `meshclaw_model_config_status` to inspect model gateway
readiness. Raw API keys are write-only through the local CLI and should not be
sent through Signal or returned through MCP.

## First Successful Tasks

After setup, the user should be able to verify the product with concrete
assistant outcomes, not abstract health checks.

### Signal

- Send `오늘 브리핑` to the Argos assistant room.
- Receive a short briefing, not a raw data-doctor or evidence counter dump.
- Ask a follow-up such as `1번 자세히` in the assistant room.
- Send `내일 회의 자료 만들어줘`.
- Receive a human-readable answer and actual document attachments. The reply
  should say what was prepared, include enough summary to read directly in
  Signal, and not dump local paths or evidence IDs.
- Send `방금 만든 파일 다시 보내줘`.
- Receive the recent DOCX/Markdown/HTML/PPTX attachments without being asked
  which file again.
- Send `방금 만든 PPT를 3장으로 줄여줘` or `방금 만든 문서를 더 짧게
  다듬어줘`.
- Receive a new edited attachment. Argos should revise from the recent artifact
  context instead of asking for the file path again, and should not mutate the
  original file in place.
- Send `최근 메일 요약해줘`.
- Receive a concise grouped mail summary across configured accounts. Argos
  should not ask which mailbox unless the user is sending, mutating, or
  explicitly scoping an account.
- Send `메일에서 Google Workspace 찾아서 최근 것 읽어줘`.
- Receive grouped search results for the requested keyword, not a generic
  "new mail" watch result. The Signal reply should not mention evidence,
  local paths, draft IDs, `SECRET`, or `작업 기록도 저장했습니다`.
- Send `첫 번째 메일에 정중하게 답장 초안 써줘` within the same assistant
  conversation.
- Receive an unsent draft reply for that recent message. Argos should remember
  only the recent account/message reference long enough for the follow-up; it
  should not send mail, mutate the mailbox, or expose the local draft path in
  the Signal reply.
- Send `브라우저로 Apple Migration Assistant Mac 검색해서 결과를 엑셀 표로 정리해줘`.
- Receive editable XLSX/CSV/HTML attachments. The table should include source
  type, usage notes, links, and concise body excerpts when source pages can be
  fetched; it should not merely repeat result titles in the summary column.
- Send `쿠팡에서 러닝 벨트 가격이랑 배송 조건 비교해줘`.
- Receive a comparison/search result and, if a logged-in browser is used, a
  visible proof artifact. Argos may prepare a cart or form only when asked, but
  it must stop before final purchase, payment, booking, subscription change,
  cancellation, or form submission and ask for explicit approval. A final
  shopping purchase click is only allowed through `meshclaw_purchase_click` with
  merchant, item, total, shipping, payment, checkout URL, button coordinate,
  `execute=true`, `approve=true`, and exact confirmation text `구매 실행 승인`.

### Local App Automation

- Ask an MCP client or Signal Argos to create a small document.
- Expected artifact: an Obsidian-friendly Markdown source, mobile HTML preview,
  editable DOCX, and Signal/mobile attachment when used from Signal.
- Ask for a simple presentation.
- Expected artifact: `.pptx`, outline Markdown, lightweight preview HTML, and
  a validation result.
- Ask `다운로드 폴더 정리 후보 보여줘`.
- Expected result: a review-only cleanup plan that classifies installer,
  archive, large, and old files without moving, renaming, archiving, or deleting
  anything.
- If Obsidian is installed, Markdown briefs and notes should open there by
  default. DOCX should remain editable in Word/Pages/iPhone apps, and PPTX
  should remain editable in PowerPoint/Keynote-compatible apps.

### Personal Tools

- Ask for today's calendar events.
- Ask to create a reminder, first receiving a plan; execute only when the user
  confirms.
- Ask for a place or route, receiving a Maps URL. If the user says "show me",
  "photo", or "capture", Argos should open the map and attach a short screen
  proof artifact as well as the link. Screen proof starts as a plan with a
  privacy check; actual recording runs only after explicit approval.

These tasks define success better than "the daemon is running". A normal user
cares that the assistant produced the requested thing.

## Default MCP Tool Groups

The `one-machine-assistant` profile should expose a compact, task-oriented
surface first:

| Group | Tools | Expected result |
| --- | --- | --- |
| Setup | `meshclaw_mcp_catalog`, `meshclaw_setup_assistant`, `meshclaw_model_config_status`, `meshclaw_setup_signal`, `meshclaw_setup_argos_runner` | confirms the assistant can run on this Mac and shows masked model gateway status |
| Publication | `meshclaw_argos_research`, `meshclaw_news_document` | saved report/news document with source links; cited `[S1]` summaries when enough source text was fetched |
| Documents | `meshclaw_document_create`, `meshclaw_document_export` | editable document artifacts |
| Spreadsheets | `meshclaw_spreadsheet_create` | editable XLSX/CSV artifacts for budgets, invoices, trackers, checklists, tables, and search/mail results |
| Presentations | `meshclaw_presentation_create`, `meshclaw_presentation_verify`, `meshclaw_presentation_edit`, `meshclaw_presentation_export` | usable deck, validation, recent-artifact revision, optional PDF |
| Personal assistant | `meshclaw_calendar_list`, `meshclaw_reminders_list`, `meshclaw_contacts_search`, `meshclaw_maps_search`, `meshclaw_maps_directions`, `meshclaw_maps_proof` | calendar/reminder/contact/map answers; map tasks return clickable links and can open/capture the visible map UI |
| Mail | `meshclaw_mail_accounts`, `meshclaw_mail_summarize`, `meshclaw_mail_search`, `meshclaw_mail_thread`, `meshclaw_mail_draft_reply` | read/search/draft results with no send or mailbox mutation; Signal may use recent numbered mail follow-ups for draft replies |
| Communication and alerts | `meshclaw_notification_show`, `meshclaw_reminder_create`, `meshclaw_schedule_status`, `meshclaw_schedule_run_once`, `meshclaw_scheduled_delivery_plan`, `meshclaw_scheduled_delivery_apply`, `meshclaw_signal_call_doctor`, `meshclaw_signal_call` | local notifications, reminders, morning briefings, scheduled friend/room delivery plans, approved schedule registration, and approved Signal call preflight/execution |
| Media handoff | `meshclaw_media_play`, `meshclaw_screen_capture`, `meshclaw_screen_record` | music, radio, podcast, or video requests start as source-choice plans; approved execution opens only a visible app/URL |
| Audio transcription | `meshclaw_audio_transcribe`, `meshclaw_result_save` | local voice notes or recordings start as a privacy/dependency plan; approved execution uses local Whisper and saves a local transcript |
| App/settings handoff | `meshclaw_app_settings_plan`, `meshclaw_screen_capture`, `meshclaw_screen_record` | System Settings, app settings, privacy panes, and account-setting pages may be opened for review; toggles/saves/account changes need separate approval |
| Account/subscription preflight | `meshclaw_account_action_plan`, `meshclaw_screen_capture`, `meshclaw_screen_record` | billing, cancellation, renewal, plan-change, and account pages may be opened for review; final account action requires a separate explicit approval |
| Transactional web | `meshclaw_visible_browser_search`, `meshclaw_browser_fetch`, `meshclaw_automation_open_url`, `meshclaw_argos_ask`, `meshclaw_screen_capture`, `meshclaw_screen_record`, `meshclaw_result_save`, `meshclaw_account_action_plan`, `meshclaw_purchase_click`, `meshclaw_subscription_frontends` | shopping, booking, forms, account pages, and AI frontends prepared with privacy-checked proof; final purchase click requires strong explicit approval |
| File cleanup planning | `meshclaw_downloads_cleanup_plan`, `meshclaw_downloads_cleanup_apply`, `meshclaw_result_save` | review-only Downloads cleanup candidates first; approved apply only moves explicit files to a review folder and never deletes/trashes/archives |
| Controlled execution | `meshclaw_terminal_run`, `meshclaw_shortcut_text_run`, `meshclaw_result_save` | command/Shortcut results saved as artifacts |

Mutation tools should be plan-only by default. The model should choose them
because the description matches the user's intent; MeshClaw policy/approval
should block dangerous execution.

## macOS Toolsets For Models

Models should choose the most specific OS toolset before falling back to a
generic natural-language Mac action:

| User intent | Prefer | Avoid first |
| --- | --- | --- |
| Read Calendar, Reminders, or Contacts | `meshclaw_calendar_list`, `meshclaw_reminders_list`, `meshclaw_contacts_search` | `meshclaw_argos_ask` |
| Create/update local personal data | `meshclaw_calendar_create_event`, `meshclaw_reminder_create`, `meshclaw_note_create` with `execute=false` first | direct UI clicks |
| Notify, remind, brief, or call | `meshclaw_notification_show`, `meshclaw_reminder_create`, `meshclaw_schedule_run_once`, `meshclaw_signal_call_doctor`, then `meshclaw_signal_call` | real calls without `approve=true` and `execute=true` |
| Schedule a message/report/voice note to a friend or room | `meshclaw_scheduled_delivery_plan` first, then `meshclaw_scheduled_delivery_apply` only after preview approval | immediate send or recurring job registration before first-run preview approval |
| Transcribe a local voice note or recording | `meshclaw_audio_transcribe` with `execute=false` first, then `execute=true` plus `approve=true` | uploading private audio to a generic web service |
| Open an app, file, URL, map, or notification | `meshclaw_automation_open_app`, `meshclaw_automation_open_file`, `meshclaw_automation_open_url`, `meshclaw_maps_*`, `meshclaw_screen_capture`, `meshclaw_screen_record`, `meshclaw_notification_show` | terminal scripts |
| Play music, radio, podcast, or video | `meshclaw_media_play` with `execute=false`; after user approval, `execute=true` and `approve=true` opens the selected app/URL | clicking final playback, purchases, subscriptions, or account changes |
| Open app or macOS settings pages | `meshclaw_app_settings_plan` with `execute=false`; after user approval, `execute=true` and `approve=true` opens the app/settings pane only | permission toggles, saving settings, sign-out, account connection/disconnection, data deletion |
| Open billing, subscription, or cancellation pages | `meshclaw_account_action_plan` with `execute=false`; after user approval, `execute=true` and `approve=true` opens the selected URL only | final cancellation, payment, plan change, account deletion, or form submission |
| Shop, book, use logged-in websites, fill forms, or ask ChatGPT in browser | `meshclaw_visible_browser_search`, `meshclaw_automation_open_url`, `meshclaw_argos_ask`, `meshclaw_screen_capture`, `meshclaw_screen_record`, `meshclaw_result_save`, `meshclaw_purchase_click`, `meshclaw_subscription_frontends` | final purchase/payment/booking/submission without explicit approval |
| Plan Downloads cleanup | `meshclaw_downloads_cleanup_plan`, then optionally `meshclaw_result_save`; if `status=needs_access`, explain Full Disk Access or ask for a readable folder path | moving/deleting files directly |
| Apply approved Downloads cleanup | `meshclaw_downloads_cleanup_apply` with explicit selected paths, `execute=true`, and `approve=true`; only moves regular files to a review folder | deleting, trashing, archiving, overwriting, or moving folders |
| Run a local command or Shortcut and keep the result | `meshclaw_terminal_run`, `meshclaw_shortcut_text_run`, then `meshclaw_result_save` | answering from memory only |
| Ambiguous visible Mac task with no first-class tool | `meshclaw_argos_ask`, optionally `meshclaw_screen_capture` or `meshclaw_screen_record` | broad keyword routing |

This keeps model selection predictable: first-class read/write tools for known
OS domains, artifact-saving tools for durable work, and bounded UI fallback only
when the request really needs the visible Mac session.

Calls and wake-up style requests are interruption-heavy. Normal reminders,
notifications, and morning briefings should use Reminders, notifications, or
scheduled jobs first. A real Signal call must pass call doctor readiness, target
approval, `approve=true`, and `execute=true`; otherwise the tool should return a
dry-run/preflight result.

## Signal Reply Style

Signal is a phone-first surface. Replies should be short Korean prose with the
result first:

- For documents and decks, say what was made and attach the files. Avoid raw
  local path lists unless the user asks where the files live.
- For research, attach the report and mention that sources were collected. A
  report based only on search snippets must clearly say it is a source-candidate
  note, not a verified long-form analysis. When enough source page text was
  fetched, include a short `핵심 요약 (원문 발췌 기반)` section with `[S1]`
  source labels and `출처별 근거` instead of dumping raw search results.
- For Calendar, Reminders, Mail, Maps, and weather, answer directly in readable
  Korean. Do not show internal action names, evidence paths, JSON, RFC3339/UTC
  timestamps, or model routing text.
- Successful evidence storage is internal. Normal Signal replies should not say
  `작업 기록도 저장했습니다`; surface evidence only when the user explicitly asks
  for audit details.
- For failed PDF/export work, explain the missing local dependency such as
  LibreOffice or pandoc and attach the editable original when possible.

## macOS-First Document Policy

The default assistant should work well before any external SaaS account is
connected. For document work, MeshClaw should prefer local macOS capabilities:

- Store source documents under the Argos Vault as Markdown that Obsidian can
  open and organize.
- Produce editable interchange files by default: DOCX for documents and PPTX
  for presentations.
- Produce HTML/PDF-style previews when useful for phone viewing or quick
  inspection, but do not make the user open HTML just to understand the result.
- Send actual files as Signal attachments, not only local paths or links. Report
  delivery should attach the Obsidian Markdown source by default; HTML previews
  are optional.
- Use macOS apps as the first editing surface: Obsidian for notes/briefs, Pages
  or Word for DOCX, Keynote or PowerPoint for decks, Preview for PDFs/images,
  Shortcuts for user-installed app workflows.

Google Docs, Google Drive, Notion, and similar services should be provider
adapters layered on top of these artifacts, not the default authoring path. A
good external-service adapter takes an already-created local artifact and
performs a clear task such as upload, convert, share, export, or sync. This
keeps the assistant useful offline/local-first and makes account authorization
explicit per provider.

## Artifact Contract

Task-level tools should return a result the user can inspect:

- `url`: the primary human-readable preview or artifact.
- `markdown`: editable outline/report where useful.
- `pptx`, `docx`, or `pdf`: office artifact paths when generated.
- `preview`: thumbnail/image path when available.
- `validation`: proof that the requested file exists and is structurally usable.

Evidence exists to audit this outcome. It is not the product output.

## MCP Catalog

Use `meshclaw_mcp_catalog` when a model or frontend needs a product-level view
of MeshClaw capabilities.

Profiles:

- `one-machine-assistant`
- `multi-node-ops`
- `development`
- `all`

The catalog groups tools by capability instead of dumping a flat list:

- setup
- personal assistant
- visible work
- messenger
- fleet ops
- Core/Mission

This is the preferred path over adding another wrapper UI. Frontends should call
MeshClaw MCP task-level tools directly.

## Signal Rule

Public installs should present Argos as one Signal person.

Users may create separate rooms for assistant, briefing, and ops, but they
should not need multiple Argos phone numbers. Development setups may temporarily
keep two numbers for observation, but MacBook Signal should not become a sender
unless explicitly configured as a private development memo sink.
