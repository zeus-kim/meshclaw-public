# Argos Assistant Scenario Catalog

Last updated: 2026-06-21

Current direction: read `docs/ASSISTANT_PRODUCT_DIRECTION.md` and
`docs/ARGOS_COMMERCIAL_ASSISTANT_CONTRACT.md` first. This catalog is reference
material, not the product roadmap or acceptance contract. Do not expand it
before the first ten everyday Signal assistant commands are reliable in live Mac
mini Signal use.

Argos is the human-facing assistant identity. MeshClaw is the runtime that
classifies requests, checks policy, records operator evidence where useful, and
calls bounded adapters. Normal Signal replies must not show evidence paths,
router names, exit status, or raw diagnostics.

The historical source-of-truth machine catalog is:

```text
internal/assistantcatalog/catalog.go
```

That catalog currently contains more than 100 common assistant scenarios. Treat
that breadth as backlog, not proof of product readiness. Each
scenario has:

- `id`: stable intent id
- `category`: user-facing domain
- `risk`: read, write, private, external, or money
- `adapter`: the execution lane Argos should use
- `requires_approval`: whether Argos must ask before executing
- `status`: implemented, partial, or planned

## Request Flow

```text
Signal / Open WebUI / Siri / MCP
  -> Argos natural-language request
  -> MeshClaw Router
  -> assistant scenario catalog
  -> slot collection
  -> policy and permission check
  -> bounded adapter
  -> operator evidence, when needed
  -> Signal result
```

Argos should not jump directly from a vague sentence to risky execution. It
should keep read-only summaries, research, drafts, and document preparation
fast and concise; only state-changing work such as mail send, schedule
registration, external sharing, deletion, and similar actions should enter
approval flow.

## Example: Maps and Travel Time

User:

```text
강남역에서 서울역까지 지금 얼마나 걸려?
```

Argos should return one mobile-friendly Google Maps directions link with
`travelmode=transit`, without exposing an evidence path. If the user says
`여기서 서울역까지 얼마나 걸려?`, Argos must ask for a shared location or an
explicit start place instead of pretending it knows the current location.

User:

```text
그 식당 주차 되는지 봐줘
```

Argos must not guess what `그 식당` means. It asks for a place name or map link.
If the user sends a named place such as `성수동 난포 주차 되는지 봐줘`, Argos sends
one Google Maps search link focused on parking information.

## Example: Music

User:

```text
음악 틀어줘
```

Argos should not guess one playback surface immediately. It should reply:

```text
어디에서 틀까요?
1. YouTube
2. 라디오
3. iPhone에서 바로 재생
4. Mac에서 재생
```

If the user says `유튜브에서 재즈`, Argos uses:

```text
music_play -> youtube_music_play -> browser/airplay
```

If the user says `아이폰에서`, Argos uses:

```text
music_play -> iphone_audio_handoff -> Shortcuts/AirPlay
```

Because playback changes the user's device state, Argos needs either a one-time
approval or a persistent narrow grant for that source.

## Example: Restaurant Reservation

User:

```text
내일 저녁 7시에 강남 식당 2명 예약해줘
```

Argos should collect:

- restaurant name or search constraints
- date and time
- party size
- name and phone number to use
- booking surface: web, phone, Naver, CatchTable, Google, or restaurant site
- calendar preference after success

Execution:

```text
restaurant_reserve
  -> restaurant_availability
  -> browser or phone adapter
  -> final confirmation before submitting
  -> booking_calendar_add
  -> evidence and Signal report
```

Reservation is external-world action, so the final submit step always requires
approval.

## Example: Maps and Directions

User:

```text
광화문 교보문고 지도 보여줘
```

Argos should return a clickable map link immediately. This path should not wait
for a model because the user usually wants the link on the phone.

```text
place_map_link -> maps URL -> Signal reply -> evidence
```

User:

```text
강남역에서 성수역까지 길찾기
```

Argos should return a directions link:

```text
directions -> Google Maps directions URL -> Signal reply -> evidence
```

Current location is not guessed from Signal. If the user asks `내 위치 알려줘`,
Argos asks for a shared location or place name first.

## Example: Weather and Outfit

User:

```text
오늘 뭐 입고 나가?
```

Argos should use the weather adapter directly and answer with current
temperature, feels-like temperature, rain/wind notes, and simple outfit advice.
This should be a fast tool path, not a general chat response.

```text
weather_now -> weather API -> Signal reply -> evidence
```

For commute requests, Argos combines weather with a slot question unless the
route is already present:

```text
출근길 상황 알려줘
```

Argos asks for origin and destination, then the maps direction path handles the
route.

## Example: Calendar, Reminders, Contacts

User:

```text
내일 일정 뭐 있어?
```

Argos should route this to the macOS Calendar adapter through MeshClaw
permissions, not to a chat model:

```text
calendar_list -> Argos UI Runner -> Calendar helper -> Signal reply -> evidence
```

The same pattern applies to:

```text
오늘 할 일 뭐 있어?
연락처에서 홍길동 전화번호 찾아줘
내일 오전 9시에 우유 사기 리마인더 추가해줘
20분 뒤에 알려줘
remind me in 20 minutes
오늘 운동 체크해줘
```

Calendar/reminder/contact data is private. Signal should ask for a one-time
execution or a narrow persistent grant before returning private results.
Habit checks are implemented as a Reminders mutation, so `오늘 운동 체크해줘`
becomes a bounded `reminder_complete` request for the `운동` reminder.

## Example: Assistant Capabilities

User:

```text
뭐 할 수 있어?
```

Argos should return a fixed capability menu with concrete examples. This keeps
new users out of vague model chat and gives them phrases that exercise real
MeshClaw adapters:

```text
오늘 주요뉴스
오늘 서울 날씨
광화문 교보문고 지도 보여줘
내일 일정 뭐 있어?
브라우저에서 가나 경제 뉴스 검색해줘
```

## Historical Example: Shopping And Purchase Automation

This section used to describe browser purchase, Coupang, checkout, reorder,
receipt, refund, and direct purchase execution scenarios. That guidance is no
longer part of the current assistant contract.

Current commercial rule:

- Shopping, Coupang, purchase finalization, payment, checkout, reorder, refund,
  receipt, subscription, and transaction execution are out of scope.
- Argos may help with read-only product research, comparison, review summaries,
  price watch drafts, or reminders, but it must not claim it can complete a
  purchase or payment.
- User phrases such as `휴지 구매해`, `쿠팡에서 바로 주문해`, `이걸로 주문해줘`,
  `결제해`, `재구매해`, or `Buy this item on Coupang` should return an
  out-of-scope card with safe alternatives such as comparison, reminder, draft,
  or browser-open guidance.
- Historical tests or scenarios that expect native checkout completion should be
  treated as legacy coverage until the user explicitly reopens purchase
  automation as a separate official-API project with its own risk and approval
  contract.

Acceptable product behavior:

```text
이 영역은 현재 Argos 범위 밖입니다.
대신 가격 비교, 리뷰 요약, 구매 체크리스트, 리마인더를 도와드릴 수 있습니다.
다음 명령: "이 제품 세 개 비교해줘"
```

## Example: Daily Life Planning

User:

```text
성수동에서 오늘 저녁 뭐 먹지?
출장 짐 목록 만들어줘
이번 주 장보기 목록 만들어줘
오늘 일정 보고 동선 짜줘
```

Argos should keep these fast and concrete:

```text
meal_idea -> maps link -> concise reply
packing_list -> checklist draft -> optional Notes or Reminders permission gate
grocery_list -> checklist draft -> optional Notes or Reminders permission gate
daily_plan -> Calendar permission gate -> Reminders permission gate -> route planning
```

The first response should not be vague model chat. It either returns a useful
draft/link immediately, or it explains the private-data permission gate needed
for Calendar and Reminders.

Follow-up save phrases are first-class routes:

```text
출장 짐 목록 메모로 저장해줘
장보기 목록 리마인더로 저장해줘
오늘 일정 보고 동선 짜줘
```

These go through the same MeshClaw macOS permission boundary as direct Notes,
Reminders, and Calendar commands. Any operator evidence stays internal.

## Example: Mail-Based Personal Memory

Many useful assistant tasks start from the user's mailbox. These must not wait
for a model classifier because they are common, latency-sensitive requests.

```text
택배 어디쯤 왔어?
지난달 영수증 찾아줘
구독 서비스 결제일 정리해줘
최근 메일 요약해줘
새 메일 왔어?
메일 계정 뭐 붙어 있어?
```

Argos maps these phrases directly to the mail adapter:

```text
delivery tracking -> mail_search
receipt/invoice lookup -> mail_search
subscription billing lookup -> mail_search
recent/important mail summary -> mail_summary
new mail check -> mail_watch
mail account list -> mail_accounts
```

Reading mail is private-data access and remains under MeshClaw account routing
and internal operator evidence. Sending, deleting, moving, or archiving mail
requires a separate approval step.

For read/search/watch requests, Argos searches every connected mailbox by
default and returns a grouped result:

```text
[101-101.band] ...
[user-mail] ...
```

It should ask which account to use only for sending or for explicitly scoped
account operations. A pending account-choice prompt must not block a new request.
Signal output should stay phone-readable: account headings, sender, subject, and
KST time are enough for summaries. Raw MIME/HTML/quoted-printable snippets and
internal evidence paths should not be shown.

After a summary/search/watch result, Argos may remember only the recent
account/message references for a short follow-up window. The user can then say:

```text
첫 번째 메일에 답장 초안 써줘
2번 메일에 정중하게 회신 초안 만들어줘
```

That route creates a local unsent draft reply. It never sends the email, and it
does not turn the recent-mail context into broad mailbox mutation permission.

## Example: OTT and Subscriptions

User:

```text
넷플릭스 구독 관리 열어줘
웨이브에서 오늘 볼만한 드라마 추천해줘
```

Argos can open account/recommendation pages through the browser adapter. It may
prepare read-only account screens or recommendation summaries, but plan
changes, cancellation, subscription purchase, and payment actions are outside
the current commercial assistant contract.

## 비서방에서 바로 시도할 20개 빠른 문장

1. 뭐 할 수 있어?
2. Signal에서 바로 쓸 예문 보여줘
3. 지금 날씨 어때?
4. 오늘 주요뉴스 알려줘
5. 아침 브리핑 해줘
6. 최근 메일 요약해줘
7. 내가 답해야 할 메일만 찾아줘
8. 첫 번째 메일에 답장 초안 써줘
9. 내일 일정 알려줘
10. 오늘 할 일 뭐 있어?
11. 내일 오전 9시에 우유 사기 알려줘
12. 근처 조용한 카페 찾아줘
13. 강남역에서 서울역까지 지금 얼마나 걸려?
14. 연락처에서 김민수 전화번호 찾아줘
15. 민수에게 늦는다고 보낼 문장 써줘
16. 가벼운 맥북 가방 추천해줘
17. 택배 어디쯤 왔어?
18. 이 PDF 요약해줘
19. Safari 열어줘
20. 서버 상태 알려줘

## Signal-Ready Scenario Catalog

These are user-facing phrases that should work directly from Signal. They are
organized by capability area so a user can feel the assistant surface without
knowing adapter names.

### Identity and Help

1. 너는 누구야?
2. Argos랑 MeshClaw가 각각 뭐 하는 거야?
3. 뭐 할 수 있어?
4. Signal에서 바로 쓸 예문 보여줘
5. 내 승인이 필요한 작업만 알려줘
6. 읽기 작업과 쓰기 작업 차이를 설명해줘
7. 오늘 쓸 만한 명령 10개 추천해줘
8. 처음 쓰는 사람한테 사용법 알려줘
9. 지금 연결된 기능 상태 요약해줘
10. 안전하게 할 수 있는 작업부터 보여줘

### Weather, News, and Voice Briefs

1. 지금 날씨 어때?
2. 오늘 서울 날씨 알려줘
3. 내일 비 와?
4. 오늘 뭐 입고 나가?
5. 출근길 날씨랑 교통 같이 봐줘
6. 아침 브리핑 해줘
7. 아침 브리핑 음성으로 읽어줘
8. 저녁 정리해줘
9. 오늘 주요뉴스 알려줘
10. 오늘 주요뉴스 음성으로 들려줘
11. 뉴스 출처 보여줘
12. 3번 뉴스 자세히 설명해줘
13. 오늘 날씨 음성으로 알려줘
14. 중요한 일정이랑 날씨만 짧게 브리핑해줘
15. 자기 전에 내일 준비할 것 알려줘

### Mail

1. 최근 메일 요약해줘
2. 새 메일 왔어?
3. 중요해 보이는 메일만 골라서 요약해줘
4. 내가 답해야 할 메일만 찾아줘
5. 첫 번째 메일에 답장 초안 써줘
6. 2번 메일에 정중하게 회신 초안 만들어줘
7. 답장 안 한 메일에 보낼 팔로업 초안 써줘
8. 이 메일 팀에 전달할 문장 만들어줘
9. 101.band 관련 메일 찾아줘
10. 지난달 영수증 찾아줘
11. 구독 서비스 결제일 정리해줘
12. 택배 어디쯤 왔어?
13. 첨부파일 저장해줘
14. 이 메일 보관함으로 옮겨줘
15. 이 내용으로 이메일 보내줘

### Calendar, Reminders, and Time

1. 내일 일정 뭐 있어?
2. 오늘 할 일 뭐 있어?
3. 내일 오전 9시에 우유 사기 알려줘
4. 20분 뒤에 알려줘
5. 내일 3시에 미팅 일정 넣어줘
6. 그 미팅 30분 뒤로 미뤄줘
7. 내일 저녁 약속 일정 지워줘
8. 우유 사기 완료 처리해줘
9. 오늘 일정 보고 동선 짜줘
10. 이번 주 중요한 일정만 요약해줘
11. 회의 메일에서 시간 찾아서 일정 후보로 만들어줘
12. 매주 월요일 아침 운동 리마인더 만들어줘
13. 오늘 운동 체크해줘
14. 내일 준비할 일 목록 만들어줘
15. 가족 일정 정리해줘

### Contacts, Calls, and Messages

1. 연락처에서 김민수 전화번호 찾아줘
2. 민수에게 늦는다고 보낼 문장 써줘
3. 민수에게 10분 늦는다고 보내줘
4. 엄마에게 시그널 전화 걸어줘
5. 식당에 전화해줘
6. 예약 전화할 말 정리해줘
7. 부재중 연락 확인하고 답장 초안 만들어줘
8. 저녁 모임 초대 문구 만들어줘
9. 생일 축하 문구 써줘
10. 그 연락처 공유해줘

### Files, Notes, and Documents

1. 어제 만든 보고서 찾아줘
2. 이 PDF 요약해줘
3. 이 문서 PDF로 바꿔줘
4. 이 파일 이름 정리해줘
5. 이 파일 공유 링크 만들어줘
6. 회의 메모로 저장해줘
7. 지난번 아이디어 메모 찾아줘
8. 한 페이지 보고서 만들어줘
9. 회의록 문서로 만들어줘
10. 제안서 초안 만들어줘
11. 이 내용을 프로젝트 노트에 추가해줘
12. 오늘 한 일 일지로 정리해줘
13. 이 자료들 공유용 폴더로 묶어줘
14. 이 캡처 이미지 내용 설명해줘
15. 다운로드 폴더 정리 계획 세워줘

### Maps, Places, and Movement

1. 근처 조용한 카페 찾아줘
2. 이 식당 영업시간 알려줘
3. 광화문 교보문고 지도 보여줘
4. 여기서 서울역까지 가는 길 알려줘
5. 강남역에서 서울역까지 지금 얼마나 걸려?
6. 그 식당 주차 되는지 봐줘
7. 지금 연 약국 찾아줘
8. 이 장소를 친구에게 보낼 문장으로 정리해줘
9. 이 식당 후보로 저장해줘
10. 이 세 식당 비교해줘
11. 출근길 상황 알려줘
12. 성수동에서 오늘 저녁 뭐 먹지?

### Research, Price, and Topic Monitoring

1. 가나 경제 뉴스 검색해줘
2. 이 링크 읽고 요약해줘
3. 이 제품 후보 비교해줘
4. 이 말 사실인지 확인해줘
5. 이 페이지 한국어로 정리해줘
6. 이 주제 계속 지켜봐줘
7. 이 상품 가격 지켜볼 리마인더 초안 만들어줘
8. OpenClaw 관련 새 소식 나오면 알려줘
9. 이 페이지 바뀌면 알려줘
10. 지금 감시 중인 항목 보여줘
11. 가격 리마인더 중지 방법 알려줘
12. 출처 있는 답만 줘

### Booking and Research

1. 내일 7시에 강남 식당 2명 예약해줘
2. 오늘 저녁 예약 가능한지 봐줘
3. 내일 예약 취소해줘
4. 도쿄 호텔 후보 찾아줘
5. 이 호텔 예약 전에 확인할 체크리스트 만들어줘
6. 서울 방콕 항공권 찾아줘
7. 이 항공권 예약 전에 확인할 체크리스트 만들어줘
8. 콘서트 티켓 남았는지 봐줘
9. 티켓 예매 전에 확인할 조건 정리해줘
10. 예약되면 캘린더에 넣어줘
11. 가벼운 맥북 가방 추천해줘
12. 이 제품 세 개 비교해줘
13. 구매 전 체크리스트 만들어줘
14. 이 구매는 Argos 범위 밖인지 알려줘
15. 결제 전에 확인할 위험만 정리해줘
16. 검정색 M 사이즈로 골라줘
17. 리뷰 보고 살만한지 정리해줘
18. 배송 빠른 곳으로 골라줘
19. 영수증 정리 방법 알려줘
20. 반품 신청 전에 필요한 정보를 정리해줘

### Mac, TTS, and Local Actions

1. Safari 열어줘
2. chatgpt.com 열어줘
3. 이 문장 복사해줘
4. 모닝 단축어 실행해줘
5. 작업하는 화면 녹화해줘
6. 브라우저 앞으로 가져와줘
7. 이 내용을 입력해줘
8. Signal 켜져 있어?
9. 맥 권한 상태 봐줘
10. Argos Runner 업데이트해줘
11. 현재 화면 캡처해줘
12. 화면 꺼줘
13. 맥 볼륨 30%로 낮춰줘
14. 와이파이 연결 상태 봐줘
15. 이 문장 읽어줘
16. 이 문서 핵심만 읽어줘
17. 읽는 거 멈춰
18. 목소리 차분한 걸로 바꿔줘

### Server, Security, and DevOps

1. 서버 상태 알려줘
2. 오픈웹유아이 연결됐어?
3. g4 ollama 상태 봐줘
4. 최근 에러 로그 봐줘
5. 디스크 용량 확인해줘
6. 보안 상태 점검해줘
7. 최근 작업 기록 보여줘
8. 이 작업 승인 요청 보내줘
9. nginx 재시작해줘
10. 이 토큰 저장해줘
11. 서버 상태 음성으로 보고해줘
12. 최근 장애 징후 요약해줘
13. 오늘 보안 이벤트 정리해줘
14. 백업이 제대로 됐는지 확인해줘
15. 최근 배포 상태 알려줘
16. 이 장애 대응 절차 문서로 만들어줘

### Media and Entertainment

1. 음악 틀어줘
2. 유튜브에서 재즈 틀어줘
3. 라디오 틀어줘
4. 아이폰에서 바로 틀어줘
5. AI 팟캐스트 틀어줘
6. 집중할 때 들을 음악 목록 만들어줘
7. 이 유튜브 영상 틀어줘
8. 이 영상 내용 요약해줘
9. 빗소리 틀어줘
10. 오늘 볼만한 거 추천해줘
11. 넷플릭스 열어줘
12. 웨이브에서 오늘 볼만한 드라마 추천해줘
13. 애플TV 최신 영화 뭐 있어?
14. 보던 드라마 이어서 틀어줘
15. OTT 결제일 정리해줘
16. 쿠팡플레이 해지 화면까지 가줘

### Approval-First Write Actions

1. 파일 저장 전에 나한테 확인해줘
2. 메시지는 보내기 전에 꼭 물어봐
3. 결제/구매/거래 실행은 현재 범위 밖이라고 알려줘
4. 삭제 작업은 확인 받고 해
5. 날씨 브리핑은 매일 아침 자동으로 보내도 돼
6. 이 메일 삭제해도 되는지 먼저 물어봐
7. 예약 제출 직전에 멈춰
8. 서비스 재시작 전에 승인 요청 보내줘
9. 공유 링크 만들기 전에 대상 확인해줘
10. 외부로 나가는 작업은 먼저 확인하고 짧게 결과만 알려줘

## Scenario Groups

| Group | Examples | Adapter |
| --- | --- | --- |
| Identity and help | identity, capability menu, approval rules | assistantcatalog/policy |
| Voice and TTS | spoken brief, read text, stop speech | TTS/assistantbrief |
| Maps and places | place search, map link, directions, parking | maps/browser |
| Time | reminders, calendar, alarms, daily plan | EventKit/Shortcuts |
| Contacts | contact lookup, draft message, Signal call | Contacts/Signal |
| Mail | summary, search, read, draft, send, attachments | mail adapter |
| Research | fast news, refreshed news, web search, fact check | news/browser/model |
| Booking | restaurants, hotels, flights, tickets | browser/phone/calendar |
| Product research | comparison, review summary, purchase checklist | browser/model |
| Files and docs | find, summarize, convert, note, screenshot read | files/notes/vision |
| Daily life | weather, commute, outfit, meal, packing | weather/maps/model |
| Mac control | open app, open URL, clipboard, shortcut, recording | Argos UI Runner |
| Media | music, YouTube, radio, podcast, iPhone playback | browser/Shortcuts/AirPlay |
| Ops | server status, logs, disk, security, approvals | MeshClaw Runtime |
| Approval policy | write, send, delete, persistent grants | policy/evidence |

## Approval Rules

Read-only:

- search, summarize, compare, show location, show status
- no approval unless private data is involved

Private:

- mail, calendar, contacts, files, screenshots
- may read after user has granted domain permission
- sending or mutation still requires approval

External:

- calling, messaging, booking, posting, external sharing
- requires approval unless a narrow persistent grant exists

Money:

- purchase, checkout, payment, trading, subscription, ticket purchase, hotel or
  flight payment, and transaction execution are outside the current commercial
  assistant contract
- direct commands such as `구매해`, `사줘`, `주문해`, `구독해`, `buy it`, or
  `subscribe` do not authorize execution in this product phase
- Argos may offer research, comparison, reminder, checklist, or draft help

Destructive:

- delete mail, delete files, cancel bookings, restart services
- requires explicit approval and internal operator evidence

## Product Principle

Argos should feel like a practical assistant:

```text
understand request -> return useful card -> show next action -> stop before risk
```

MeshClaw should remain the internal runtime:

```text
policy -> permissions -> adapter -> internal evidence -> concise Signal reply
```
