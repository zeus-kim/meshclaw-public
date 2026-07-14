# Argos Capabilities and Scenario Contract

This document defines what MeshClaw Argos can do from Signal today, what kind of
reply the user should see, and which behavior is covered by scenario tests.

## Runtime Shape

- User-facing runtime/sender: Mac mini.
- MacBook is a development/MCP client surface, not the user-facing Signal sender.
- Interactive replies are allowed only in assistant/chat/Guard targets.
- Report, briefing, and argos-ops targets are one-way/no-reply.
- Mission JSON is MacBook-only under `~/.meshclaw/state/missions/`.
- Document work is macOS-first: Obsidian Markdown source, editable office files,
  and a phone-readable Signal body.

## Reply Contract

Signal replies must be useful without opening an attachment.

- Do include the answer, summary, status, or next action in visible Signal text.
- Do include a compact flow line for document/report work, for example:
  `흐름도: 요청 --> 작성 --> Obsidian 저장 --> Signal 보고`.
- Do attach files when the user asked for a document, deck, table, or proof.
- For reports, attach the Obsidian Markdown source by default rather than only
  showing the saved local path.
- Messenger reports use `messenger-report.md` as the canonical user-facing
  report artifact; JSON remains internal machine-readable evidence.
- Do not expose local paths, evidence IDs, or `meshclaw-attachment:` markers in
  visible Signal text.
- Do not require the user to open HTML just to understand the result.
- HTML previews remain optional diagnostics, not the default report attachment.

## Capability Matrix

| Area | Example requests | Expected visible reply | Execution notes |
| --- | --- | --- | --- |
| Help/capabilities | `뭘 할 수 있어?`, `도움말` | Clear menu of supported tasks and rules | No model needed |
| News/weather | `오늘 주요뉴스`, `서울 날씨` | Compact source-based briefing | No hidden paths |
| Maps/places | `광화문 교보문고 지도`, `분당에서 광화문까지` | Clickable Google Maps link and short guidance | Current-location requests ask for origin |
| Browser/search | `브라우저에서 가나 경제 뉴스 검색` | Search/report summary plus attachments | Browser/search artifacts may attach |
| Documents | `내일 회의 자료 만들어줘` | Signal-readable summary, flow line, attachments | Saves Obsidian Markdown, HTML, DOCX |
| Presentations | `발표자료 만들어줘` | PPTX creation and validation summary | Saves PPTX and outline |
| Meeting materials | `회의 자료 패키지 만들어줘` | Brief + deck summary | Creates document and presentation |
| Spreadsheets | `예산표를 엑셀로 만들어줘` | XLSX/CSV summary | Sends editable files |
| Recent artifacts | `방금 만든 파일 다시 보내줘`, `더 짧게 다듬어줘` | Uses recent artifact context | Does not ask for local path again |
| Calendar | `내일 일정 뭐 있어?`, `내일 3시 회의 일정 추가` | List/plan/result text | Mutations require runtime permission |
| Reminders | `오늘 할 일`, `20분 뒤에 알려줘`, `운동 완료해줘` | List/plan/result text | Mutations require runtime permission |
| Contacts | `연락처에서 홍길동 전화번호 찾아줘` | Contact search result or ask for specificity | Vague contact requests are rejected |
| Notes | `Notes에 메모해줘` | Save result or permission prompt | Local macOS helper path |
| Apps/URLs | `Safari 열어줘`, `https://example.com 읽어줘` | App/open/fetch summary | Mac control permission applies |
| Shortcuts | `Argos Morning 단축어 실행` | Shortcut execution summary | User-installed shortcut required |
| AI handoff | `클로드로 넘겨줘: ...` | Handoff/open frontend summary | No destructive action by default |
| Mail | `최근 메일 요약`, `첫 번째 메일 답장 초안` | Grouped summary or unsent draft | Send/delete/move require approval |
| Shopping/booking | `러닝 벨트 찾아줘`, `예약 후보 찾아줘` | Research/comparison or approval boundary | Final purchase/booking must stop |
| Sensitive values | `비밀번호 ...` | Acknowledges without repeating secret | Store only via explicit Guard flow |
| Ops/destructive | `서버 재시작`, `rm -rf` | Refuses direct assistant execution | Route to Ops/MCP plan |

## Identity Answer

When the user asks `넌 누구지?`, `뭘 할 수 있지?`, or `what can you do`,
Argos must answer deterministically instead of guessing through a model.

Required answer shape:

- `저는 Argos입니다.`
- Argos is the Signal-facing personal assistant identity.
- The Mac mini is the user-facing runtime/sender.
- MeshClaw is the internal runtime/control plane that handles tools, policy,
  evidence, and approval boundaries.
- Argos can help with news, weather, maps, Mac mini actions, mail, documents,
  reports, voice briefings, and read-only operations/security checks.
- Sending mail, deleting data, buying, booking, restarting services, or other
  irreversible actions require approval or an Ops/MCP flow.

## Mac Mini Scenario Catalog

These are the practical scenarios Argos should support from the Mac mini Signal
runtime. Each scenario should either complete safely, ask only for missing
slots, or stop at an approval boundary.

| Scenario | User phrase | Expected behavior |
| --- | --- | --- |
| Identity | `넌 누구지? 무엇을 할 수 있지?` | Explain Argos, Mac mini, MeshClaw, approval boundary |
| News text | `오늘 주요뉴스 5개` | Fetch/summarize news in Korean, visible Signal text |
| News voice | `오늘 뉴스를 음성으로 보내줘` | Build a natural news script and attach voice note |
| Weather | `오늘 서울 날씨는?` | Return current weather via weather API path |
| Clothing | `오늘 뭐 입고 나가?` | Weather-based clothing advice |
| Maps | `광화문 교보문고 지도 보여줘` | Return Google Maps link |
| Directions | `분당에서 광화문까지 출근길` | Return route/travel-time guidance or ask missing origin |
| Parking | `성수동 난포 주차 돼?` | Search/answer with caveat and link |
| Browser fetch | `https://example.com 읽어줘` | Fetch page and summarize |
| Browser search | `브라우저에서 가나 경제 뉴스 검색해줘` | Search, summarize, attach report if requested |
| Notes | `Notes에 메모해줘. 제목은 장보기...` | Use Mac helper or ask permission |
| Reminder list | `오늘 할 일 뭐 있어?` | Read reminders if permitted |
| Reminder create | `20분 뒤에 알려줘` | Create reminder after permission/slot check |
| Calendar read | `내일 일정 뭐 있어?` | Summarize calendar if permitted |
| Calendar create | `내일 3시 회의 일정 추가` | Ask missing title/location, then approval |
| Contacts | `연락처에서 홍길동 전화번호 찾아줘` | Search contacts; reject vague broad dumps |
| Mail summary | `최근 메일 요약` | Summarize inbox without exposing secrets |
| Mail draft | `첫 번째 메일 답장 초안 써줘` | Draft only; do not send without approval |
| Document | `회의 자료 문서로 만들어줘` | Create Obsidian Markdown and editable attachment |
| Presentation | `발표자료 만들어줘` | Create deck outline/PPTX artifact |
| Spreadsheet | `예산표 엑셀로 만들어줘` | Create spreadsheet artifact |
| Recent artifact | `방금 만든 파일 다시 보내줘` | Reuse recent artifact context |
| TTS | `이 문장을 edge tts로 음성 파일로 보내줘` | Generate voice note, attach to current allowed target |
| Ops voice | `DevOps 보안 상황 음성으로 보고해줘` | Read-only ops/security voice note to report room |
| Open app | `Safari 열어줘` | Open app through Mac runner if permitted |
| Open URL | `이 링크 열어줘` | Open URL through Mac runner if permitted |
| Shortcut | `Argos Morning 단축어 실행` | Run installed shortcut if permitted |
| AI handoff | `클로드로 넘겨줘: 이 에러 분석` | Prepare handoff/open frontend, no hidden execution claims |
| Shopping research | `러닝 벨트 3만원 아래 찾아줘` | Research/compare; purchase requires approval |
| Booking research | `내일 저녁 강남 파스타 예약 후보` | Find candidates; final booking requires approval |
| Guard secret | `비밀번호 ...` | Do not repeat raw value; route to Guard/Vault flow |
| Destructive request | `서버 재시작해` | Do not execute in Assistant; route to Ops/MCP plan |

## Scenario Test Scope

Scenario tests cover two layers:

1. `osauto.ClassifyArgosAction` routing.
   This verifies Korean/English user phrases map to expected Argos actions such
   as `browser_search`, `document_create`, `reminder_create`, and `contacts_search`.

2. Signal assistant reply contract.
   This verifies visible replies are non-empty, hide internal attachment markers
   and local paths, and include expected user-facing phrases for capability,
   document, presentation, spreadsheet, and safety flows.

These tests intentionally avoid real app clicks, real Signal sends, purchases,
mail sends, calendar mutations, or destructive operations. End-to-end dispatch
health is checked separately with `meshclaw messenger dispatch-health`.

## Operational Self-Test

From an assistant/chat target, send:

```text
기능테스트
```

or run locally:

```sh
meshclaw assistant self-test --json
```

The reply should include:

- News/weather/map readiness.
- Mail account and price-watch readiness.
- Mac permission posture.
- `기능 매트릭스`: representative Argos routing checks for help, browser,
  documents, calendar, reminders, contacts, app open, Shortcuts, and AI handoff.
- `Signal 답변 계약`: verifies document replies include visible Signal summary,
  flow line, hidden attachment markers, and actual attachments.

This is a fast operational contract check. It does not send a real Signal
message, click apps, mutate mail/calendar/reminders, or run the full Go test
suite.

## Current Large Scenario Coverage

The deterministic large scenario suite currently checks:

- 240+ Argos classification prompts across help, browser, document, runner,
  notes, contacts, calendar, reminders, URLs, apps, shortcuts, AI handoff, and
  clipboard handling.
- 40+ Signal assistant visible-reply prompts across help, safety boundaries,
  documents, decks, spreadsheets, meetings, recent-artifact workflows, and
  vague/missing-info handling.

The suite is meant to grow by adding examples to the matrices rather than by
adding one-off tests for every incident.
