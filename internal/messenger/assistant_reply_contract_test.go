package messenger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meshclaw/meshclaw/internal/assistantprofile"
	"github.com/meshclaw/meshclaw/internal/mailadapter"
)

func TestAssistantBaselineUserFacingPromptContracts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_MAIL_CONFIG", filepath.Join(home, "mail.json"))
	t.Setenv("MESHCLAW_MAIL_DRAFT_DIR", filepath.Join(home, "mail-drafts"))
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_ASSISTANT_TOOL_LOOP", "0")
	t.Setenv("MESHCLAW_BOOKING_SEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_ASSISTANT_RESEARCH_DISABLE", "1")
	t.Setenv("MAIL_PASSWORD", "test-password")
	if _, _, err := mailadapter.UpsertAccount(mailadapter.Account{
		ID:          "101",
		Backend:     "imap",
		Email:       "101@example.com",
		Host:        "imap.example.com",
		Port:        993,
		SMTPHost:    "smtp.example.com",
		SMTPPort:    587,
		Username:    "101@example.com",
		PasswordEnv: "MAIL_PASSWORD",
		TLS:         true,
		SMTPTLS:     true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := UpsertTarget(Target{
		ID:      "argos-briefing",
		Channel: "signal",
		GroupID: "group.argos-briefing",
		Label:   "보고방",
		Mode:    "briefing",
	}); err != nil {
		t.Fatal(err)
	}

	opts := ListenOptions{TargetID: "argos-assistant-baseline", Mode: "assistant"}
	cases := []struct {
		name  string
		input string
		reply func() string
		want  []string
	}{
		{
			name:  "identity",
			input: "넌 누구야? 무엇을 할 수 있어?",
			reply: func() string { return assistantReply(opts, "넌 누구야? 무엇을 할 수 있어?") },
			want:  []string{"저는 Argos", "Mac mini", "MeshClaw", "승인"},
		},
		{
			name:  "chemical_market_research",
			input: "최근 화학회사들이 민감하게 반응할만한 뉴스를 찾아서 분석해서 보고서를 마련해줘. 내일 회의에서 사용할 중요한 토픽들을 정해줘.",
			reply: func() string {
				return assistantReply(opts, "최근 화학회사들이 민감하게 반응할만한 뉴스를 찾아서 분석해서 보고서를 마련해줘. 내일 회의에서 사용할 중요한 토픽들을 정해줘.")
			},
			want: []string{"화학회사 민감 뉴스 분석 보고서", "내일 회의 핵심 토픽", "PFAS", "회의에서 바로 던질 질문", "결론"},
		},
		{
			name:  "chemical_market_research_to_briefing",
			input: "최근 화학회사들이 민감하게 반응할만한 뉴스를 찾아서 분석해서 보고서를 보고방에 보내줘. 내일 회의에서 사용할 중요한 토픽들을 정해줘.",
			reply: func() string {
				return assistantReply(opts, "최근 화학회사들이 민감하게 반응할만한 뉴스를 찾아서 분석해서 보고서를 보고방에 보내줘. 내일 회의에서 사용할 중요한 토픽들을 정해줘.")
			},
			want: []string{"화학회사 민감 뉴스 분석 보고서를 Signal로 보낼 준비", "대상: 보고방", "내일 회의 핵심 토픽", "출처 후보/확인 상태", "첨부: 전체 Markdown/DOCX 보고서와 검색 리서치 노트"},
		},
		{
			name:  "weekly_travel_plan",
			input: "일주일 동안 내가 가장 시간이 많이 나는 시기는 언제인지 확인하고 여행계획을 잡아줘. 비행기표와 호텔 후보도 같이 봐줘.",
			reply: func() string {
				return assistantReply(opts, "일주일 동안 내가 가장 시간이 많이 나는 시기는 언제인지 확인하고 여행계획을 잡아줘. 비행기표와 호텔 후보도 같이 봐줘.")
			},
			want: []string{"다음 7일 가용시간 기반 여행계획", "가장 유력한 시간대", "추천 일정안", "항공/호텔 확인 기준", "다음 진행"},
		},
		{
			name:  "weekly_travel_plan_to_briefing",
			input: "일주일 동안 내가 가장 시간이 많이 나는 시기는 언제인지 확인하고 제주 여행계획을 잡아서 보고방에 보내줘. 비행기표와 호텔 후보도 같이 봐줘.",
			reply: func() string {
				return assistantReply(opts, "일주일 동안 내가 가장 시간이 많이 나는 시기는 언제인지 확인하고 제주 여행계획을 잡아서 보고방에 보내줘. 비행기표와 호텔 후보도 같이 봐줘.")
			},
			want: []string{"여행계획서를 Signal로 보낼 준비", "대상: 보고방", "가장 유력한 시간대", "항공 후보 링크", "호텔 후보 링크", "첨부: 전체 Markdown/DOCX 여행계획서와 항공/호텔 리서치 노트"},
		},
		{
			name:  "travel_candidates_narrow",
			input: "1안으로 항공/호텔 후보 3개씩 좁혀줘",
			reply: func() string {
				return assistantReply(opts, "1안으로 항공/호텔 후보 3개씩 좁혀줘")
			},
			want: []string{"1안 기준 항공/호텔 후보", "여행지: 제주", "항공 후보 3개", "호텔 후보 3개", "예약 전 확인"},
		},
		{
			name:  "travel_itinerary_combination",
			input: "첫 번째 항공과 두 번째 호텔 조합으로 일정표 만들어줘",
			reply: func() string {
				return assistantReply(opts, "첫 번째 항공과 두 번째 호텔 조합으로 일정표 만들어줘")
			},
			want: []string{"제주 1안 2박 3일 조합 일정표", "선택 조합: 1번 항공 + 2번 호텔", "일정표:", "예산 확인표", "예약 전 마지막 확인"},
		},
		{
			name:  "news_voice_plan",
			input: "오늘 주요뉴스를 음성으로 보내줘",
			reply: func() string {
				return executeAssistantToolCall(opts, "오늘 주요뉴스를 음성으로 보내줘", "send_tts_voice", `{"topic":"오늘 주요뉴스","content":"Argos 뉴스 브리핑입니다.","voice_note":true,"execute":false}`)
			},
			want: []string{"음성파일 생성 계획", "엔진: edge-tts"},
		},
		{
			name:  "weather_capability",
			input: "오늘 서울 날씨 알려줘",
			reply: func() string { return assistantCapabilitiesReply() },
			want:  []string{"제약회사 최신 뉴스", "DART/SEC", "최근 메일 요약해줘", "오늘 일정", "승인 필요"},
		},
		{
			name:  "memory_recall",
			input: "기억 확인",
			reply: func() string {
				root := filepath.Join(home, ".meshclaw")
				if _, err := assistantprofile.InitDefaults(time.Now(), root, true); err != nil {
					t.Fatal(err)
				}
				if _, err := assistantprofile.AddMemory(time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC), root, "user", "사용자는 짧은 한국어 답변을 선호한다", true); err != nil {
					t.Fatal(err)
				}
				return assistantReply(opts, "기억 확인")
			},
			want: []string{"현재 승인된 Argos memory", "짧은 한국어 답변", "비밀번호/토큰"},
		},
		{
			name:  "memory_plan_from_signal",
			input: "앞으로 답변은 짧게 먼저 해줘 기억해",
			reply: func() string {
				return assistantReply(opts, "앞으로 답변은 짧게 먼저 해줘 기억해")
			},
			want: []string{"Argos memory에 저장할까요", "범위: assistant", "답변은 짧게 먼저 해줘", "저장해"},
		},
		{
			name:  "memory_snapshot",
			input: "메모리 구조",
			reply: func() string {
				return assistantReply(opts, "메모리 구조")
			},
			want: []string{"Argos memory snapshot", "short_term", "long_term", "procedural", "ops"},
		},
		{
			name:  "memory_reject_secret",
			input: "token=placeholder-secret-value-123456 기억해",
			reply: func() string {
				return assistantReply(opts, "token=placeholder-secret-value-123456 기억해")
			},
			want: []string{"민감값 원문은 답장에 반복하지 않고"},
		},
		{
			name:  "maps_directions",
			input: "강남역에서 서울역까지 길찾기",
			reply: func() string {
				return executeAssistantToolCall(opts, "강남역에서 서울역까지 길찾기", "get_directions", `{"from":"강남역","to":"서울역"}`)
			},
			want: []string{"길찾기 링크입니다.", "https://www.google.com/maps/dir/", "iPhone에서 열면 Google Maps"},
		},
		{
			name:  "browser_search_permission",
			input: "브라우저에서 OpenClaw Hermes 비교 검색해줘",
			reply: func() string { return assistantReply(opts, "브라우저에서 OpenClaw Hermes 비교 검색해줘") },
			want:  []string{"Mac 작업 실행 권한", "작업: visible_browser_search", "검색: OpenClaw Hermes 비교"},
		},
		{
			name:  "browser_fetch_permission",
			input: "https://example.com 읽고 요약해줘",
			reply: func() string {
				reply, _ := runSignalArgosAction(opts, "https://example.com 읽고 요약해줘", 0)
				return reply
			},
			want: []string{"Mac 작업 실행 권한", "작업: browser_fetch", "URL: https://example.com"},
		},
		{
			name:  "note_create_permission",
			input: "Notes에 메모해줘. 제목은 장보기. 내용은 우유와 커피.",
			reply: func() string {
				return assistantReply(opts, "Notes에 메모해줘. 제목은 장보기. 내용은 우유와 커피.")
			},
			want: []string{"Mac 작업 실행 권한", "작업: note_create", "메모: 장보기"},
		},
		{
			name:  "reminders_list_permission",
			input: "오늘 할 일 뭐 있어?",
			reply: func() string { return assistantReply(opts, "오늘 할 일 뭐 있어?") },
			want:  []string{"Mac 작업 실행 권한", "작업: reminders_list", "조회 범위:"},
		},
		{
			name:  "reminder_create_permission",
			input: "내일 오전 9시에 우유 사기 리마인더 추가해줘",
			reply: func() string {
				return assistantReply(opts, "내일 오전 9시에 우유 사기 리마인더 추가해줘")
			},
			want: []string{"Mac 작업 실행 권한", "작업: reminder_create", "리마인더: 우유 사기"},
		},
		{
			name:  "reminder_complete_confirmation",
			input: "우유 사기 리마인더 완료해줘",
			reply: func() string { return assistantReply(opts, "우유 사기 리마인더 완료해줘") },
			want:  []string{"Mac 작업 실행 권한", "작업: reminder_complete", "매번 확인"},
		},
		{
			name:  "calendar_list_permission",
			input: "내일 일정 뭐 있어?",
			reply: func() string { return assistantReply(opts, "내일 일정 뭐 있어?") },
			want:  []string{"Mac 작업 실행 권한", "작업: calendar_events_list", "조회 범위:"},
		},
		{
			name:  "calendar_create_permission",
			input: "내일 오후 3시에 Argos 회의 일정 추가해줘",
			reply: func() string { return assistantReply(opts, "내일 오후 3시에 Argos 회의 일정 추가해줘") },
			want:  []string{"Mac 작업 실행 권한", "작업: calendar_event_create", "일정: Argos 회의"},
		},
		{
			name:  "contacts_search_permission",
			input: "연락처에서 홍길동 전화번호 찾아줘",
			reply: func() string { return assistantReply(opts, "연락처에서 홍길동 전화번호 찾아줘") },
			want:  []string{"Mac 작업 실행 권한", "작업: contacts_search", "검색어: 홍길동"},
		},
		{
			name:  "mail_summary",
			input: "최근 중요한 메일 요약해줘",
			reply: func() string {
				return executeAssistantToolCall(opts, "최근 중요한 메일 요약해줘", "summarize_mail", `{"limit":10}`)
			},
			want: []string{"메일 조회에 실패했습니다"},
		},
		{
			name:  "mail_draft",
			input: "to yoon@example.com subject: 회의 body: 내일 3시에 볼까요 메일 초안 만들어줘",
			reply: func() string {
				return assistantReply(opts, "to yoon@example.com subject: 회의 body: 내일 3시에 볼까요 메일 초안 만들어줘")
			},
			want: []string{"메일 초안을 만들었습니다", "to: yoon@example.com", "subject: 회의", "승인"},
		},
		{
			name:  "document_create",
			input: "문서 만들어줘. 제목은 baseline. 내용은 기본 비서 기능 점검.",
			reply: func() string {
				return executeAssistantToolCall(opts, "문서 만들어줘", "create_document", `{"title":"baseline","body":"기본 비서 기능 점검"}`)
			},
			want: []string{"문서를 작성했습니다.", "Signal에서 바로 읽기:"},
		},
		{
			name:  "meeting_minutes_with_mail_context_still_creates_document",
			input: "오늘 회의록을 작성해서 보고방에 보내줘. 논의: 사용자는 내부 점검보다 실제 결과물을 원한다. 결정: 회의록, 시장조사, 여행계획, 메일 답장 흐름을 우선 강화한다. 할 일: 매일 보고방에는 결과물 중심으로 보낸다.",
			reply: func() string {
				return assistantReply(opts, "오늘 회의록을 작성해서 보고방에 보내줘. 논의: 사용자는 내부 점검보다 실제 결과물을 원한다. 결정: 회의록, 시장조사, 여행계획, 메일 답장 흐름을 우선 강화한다. 할 일: 매일 보고방에는 결과물 중심으로 보낸다.")
			},
			want: []string{"문서/보고서를 Signal로 보낼 준비", "대상: 보고방", "회의록", "Signal에서 바로 읽기:"},
		},
		{
			name:  "meeting_materials_package_to_briefing",
			input: "내일 제품회의용 회의자료 패키지를 만들어서 보고방에 보내줘. 내용: Argos 비서는 회의록, 시장조사, 여행계획, 메일 요약을 실제 결과물로 보내야 한다. 5장 PPT와 요약 문서로 준비해줘.",
			reply: func() string {
				return assistantReply(opts, "내일 제품회의용 회의자료 패키지를 만들어서 보고방에 보내줘. 내용: Argos 비서는 회의록, 시장조사, 여행계획, 메일 요약을 실제 결과물로 보내야 한다. 5장 PPT와 요약 문서로 준비해줘.")
			},
			want: []string{"회의 자료 패키지를 Signal로 보낼 준비", "대상: 보고방", "회의 자료 패키지입니다", "발표자료: PowerPoint", "PPTX"},
		},
		{
			name:  "presentation_create",
			input: "baseline 발표자료 만들어줘",
			reply: func() string {
				return executeAssistantToolCall(opts, "발표자료 만들어줘", "create_presentation", `{"title":"baseline 발표","body":"# 목표\n- 기본 비서 기능 점검","slide_count":4}`)
			},
			want: []string{"발표자료를 작성하고 검증했습니다.", "PowerPoint에서 바로 열 수 있는 PPTX"},
		},
		{
			name:  "spreadsheet_create",
			input: "비용표 엑셀 만들어줘",
			reply: func() string {
				return executeAssistantToolCall(opts, "비용표 엑셀 만들어줘", "create_spreadsheet", `{"title":"비용표","body":"| 항목 | 금액 |\n| --- | --- |\n| 서버 | 100000 |"}`)
			},
			want: []string{"표 파일을 작성했습니다.", "Numbers나 Excel에서 바로 열 수 있는 XLSX"},
		},
		{
			name:  "scheduled_delivery_plan",
			input: "매주 월요일 9시에 보고방에 업무 브리핑 보내줘",
			reply: func() string {
				return executeAssistantToolCall(opts, "매주 월요일 9시에 보고방에 업무 브리핑 보내줘", "scheduled_delivery_plan", `{"target":"보고방","schedule":"매주 월요일 9시","content":"업무 브리핑","content_type":"message","delivery":"signal"}`)
			},
			want: []string{"예약 발송 계획을 만들었습니다.", "아직 예약 등록이나 발송은 하지 않았습니다.", "다음 단계"},
		},
		{
			name:  "audio_transcribe_capability",
			input: "이 음성파일을 전사해줘",
			reply: func() string { return assistantCapabilitiesReply() },
			want:  []string{"바로 해볼 예시", "최근 메일 요약해줘", "DART/SEC", "승인 필요"},
		},
		{
			name:  "tts_voice_note_plan",
			input: "이 문장을 보이스노트로 만들어줘",
			reply: func() string {
				return executeAssistantToolCall(opts, "이 문장을 보이스노트로 만들어줘", "send_tts_voice", `{"content":"테스트 음성 안내입니다.","topic":"테스트 음성","voice_note":true,"execute":false}`)
			},
			want: []string{"음성파일 생성 계획", "글자 수:", "엔진: edge-tts"},
		},
		{
			name:  "app_open_permission",
			input: "Safari 열어줘",
			reply: func() string { return assistantReply(opts, "Safari 열어줘") },
			want:  []string{"Mac 작업 실행 권한", "작업: open_app", "앱: Safari"},
		},
		{
			name:  "url_open_permission",
			input: "https://chatgpt.com 열어줘",
			reply: func() string { return assistantReply(opts, "https://chatgpt.com 열어줘") },
			want:  []string{"Mac 작업 실행 권한", "작업: open_url", "URL: https://chatgpt.com"},
		},
		{
			name:  "shortcut_permission",
			input: "Argos Morning 단축어 실행",
			reply: func() string { return assistantReply(opts, "Argos Morning 단축어 실행") },
			want:  []string{"Mac 작업 실행 권한", "작업: shortcut_run", "단축어: Argos Morning"},
		},
		{
			name:  "shopping_research",
			input: "검정색 M 사이즈 러닝 벨트 3만원 이하 찾아줘",
			reply: func() string {
				return executeAssistantToolCall(opts, "검정색 M 사이즈 러닝 벨트 3만원 이하 찾아줘", "search_shopping", `{"query":"검정색 M 사이즈 러닝 벨트 3만원 이하"}`)
			},
			want: []string{"Mac 작업 실행 권한", "작업: open_url"},
		},
		{
			name:  "coupang_tool_search",
			input: "쿠팡에서 생수 500ml 20개 로켓배송으로 가격, 리뷰 좋은 후보 3개 비교해줘",
			reply: func() string {
				return executeAssistantToolCall(opts, "쿠팡에서 생수 500ml 20개 로켓배송으로 가격, 리뷰 좋은 후보 3개 비교해줘", "search_shopping", `{"query":"쿠팡 생수 500ml 20개 로켓배송"}`)
			},
			want: []string{"쿠팡 후보 비교를 시작", "검색어: 생수 500ml 20", "바로 확인할 항목", "Mac 작업 실행 권한", "작업: open_url", "coupang.com/np/search", "구매 실행 승인"},
		},
		{
			name:  "booking_research",
			input: "내일 저녁 7시에 강남 파스타 식당 2명 예약 후보 찾아줘",
			reply: func() string {
				return executeAssistantToolCall(opts, "내일 저녁 7시에 강남 파스타 식당 2명 예약 후보 찾아줘", "find_booking", `{"query":"내일 저녁 7시 강남 파스타 식당 2명"}`)
			},
			want: []string{"예약 후보를 찾는 링크입니다.", "요청: 내일 저녁 7시 강남 파스타 식당 2명", "예약 전에 바로 확인할 것", "마지막 단계에서는 멈춘 뒤 Signal에서 다시 승인"},
		},
		{
			name:  "booking_alias_restaurants",
			input: "내일 저녁 7시에 강남 파스타 식당 2명 예약 후보 찾아줘",
			reply: func() string {
				return executeAssistantToolCall(opts, "내일 저녁 7시에 강남 파스타 식당 2명 예약 후보 찾아줘", "find_restaurants", `{"location":"강남","cuisine":"파스타","time":"내일 저녁 7시","party_size":"2명"}`)
			},
			want: []string{"예약 후보를 찾는 링크입니다.", "요청: 내일 저녁 7시 강남 파스타 2명", "지도 후보", "예약 검색"},
		},
		{
			name:  "booking_natural_route",
			input: "내일 저녁 7시에 강남 파스타 식당 2명 예약 후보 찾아줘",
			reply: func() string {
				return assistantReply(opts, "내일 저녁 7시에 강남 파스타 식당 2명 예약 후보 찾아줘")
			},
			want: []string{"예약 후보를 찾는 링크입니다.", "조건표", "예약 전에 바로 확인할 것", "지도 후보", "예약 검색"},
		},
		{
			name:  "booking_proceed_draft",
			input: "첫 번째 후보로 내일 저녁 7시 2명 예약 진행 문구 만들어줘",
			reply: func() string {
				return assistantReply(opts, "첫 번째 후보로 내일 저녁 7시 2명 예약 진행 문구 만들어줘")
			},
			want: []string{"예약 진행 문구 초안", "대상: 첫 번째 후보", "조건: 내일 저녁 7시 / 2명", "전화/메시지 초안", "예약자명과 연락처"},
		},
		{
			name:  "booking_draft_name_revision",
			input: "예약자명은 홍길동으로 넣어줘",
			reply: func() string {
				return assistantReply(opts, "예약자명은 홍길동으로 넣어줘")
			},
			want: []string{"예약 문구를 반영했습니다.", "반영: 예약자명 홍길동", "전화/메시지 초안", "예약자명은 홍길동입니다"},
		},
		{
			name:  "booking_draft_short_call",
			input: "전화용으로 짧게",
			reply: func() string {
				return assistantReply(opts, "전화용으로 짧게")
			},
			want: []string{"전화용 짧은 예약 문구", "전화 문장:", "예약 가능할까요", "예약금이나 취소 조건"},
		},
		{
			name:  "booking_draft_phone_revision",
			input: "연락처 010-1234-5678 넣어줘",
			reply: func() string {
				return assistantReply(opts, "연락처 010-1234-5678 넣어줘")
			},
			want: []string{"예약 문구를 반영했습니다.", "반영: 연락처 010-1234-5678", "전화/메시지 초안", "연락처는 010-1234-5678입니다"},
		},
		{
			name:  "destructive_refusal",
			input: "서버 재시작하고 배포 적용해",
			reply: func() string { return assistantReply(opts, "서버 재시작하고 배포 적용해") },
			want:  []string{"바로 실행하지 않습니다", "Ops 방", "안전한 실행 계획"},
		},
		{
			name:  "secret_guard",
			input: "비밀번호는 super-secret-value 야",
			reply: func() string { return assistantReply(opts, "비밀번호는 super-secret-value 야") },
			want:  []string{"민감값 원문은 답장에 반복하지 않고", "저장하려면"},
		},
	}

	if len(cases) < 20 {
		t.Fatalf("baseline prompt coverage too small: got %d, want at least 20", len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reply := tc.reply()
			visible := signalReplyVisibleText(reply)
			if strings.TrimSpace(visible) == "" {
				t.Fatalf("empty visible reply for input %q; raw=%q", tc.input, reply)
			}
			for _, want := range tc.want {
				if !strings.Contains(visible, want) {
					t.Fatalf("input %q visible reply missing %q:\n%s", tc.input, want, visible)
				}
			}
			if tc.name == "secret_guard" && strings.Contains(visible, "super-secret-value") {
				t.Fatalf("secret guard leaked the provided secret:\n%s", visible)
			}
			if tc.name == "weekly_travel_plan_to_briefing" {
				if strings.Contains(visible, "예약 문의 문구") || strings.Contains(visible, "전화/메시지 초안") {
					t.Fatalf("travel plan was misrouted into booking draft forwarding:\n%s", visible)
				}
				if strings.Contains(strings.ToLower(reply), ".html\n") || strings.Contains(strings.ToLower(reply), ".html\r") {
					t.Fatalf("travel plan should not attach raw HTML files:\n%s", reply)
				}
			}
			if tc.name == "meeting_materials_package_to_briefing" {
				if strings.Contains(visible, "Argos 회의록 문서/보고서") {
					t.Fatalf("meeting-material package was misrouted into meeting minutes:\n%s", visible)
				}
				if strings.Contains(strings.ToLower(reply), ".html\n") || strings.Contains(strings.ToLower(reply), ".html\r") {
					t.Fatalf("meeting-material package should not attach raw HTML files:\n%s", reply)
				}
			}
		})
	}
}

func TestAssistantMemorySignalApprovalFlow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	opts := ListenOptions{TargetID: "argos-assistant-memory-flow", Mode: "assistant"}
	plan := assistantReply(opts, "앞으로 뉴스 브리핑은 세 줄 요약을 먼저 보여줘 기억해")
	for _, want := range []string{"Argos memory에 저장할까요", "범위: assistant", "뉴스 브리핑은 세 줄 요약"} {
		if !strings.Contains(plan, want) {
			t.Fatalf("plan missing %q:\n%s", want, plan)
		}
	}
	saved := assistantReply(opts, "저장해")
	for _, want := range []string{"기억했습니다.", "범위: assistant", "뉴스 브리핑은 세 줄 요약", "다음 확인:", "기억 확인", "개인화 미리보기:", "메모리는 답변 맥락일 뿐 권한이 아닙니다"} {
		if !strings.Contains(saved, want) {
			t.Fatalf("saved missing %q:\n%s", want, saved)
		}
	}
	recall := assistantReply(opts, "기억 확인")
	for _, want := range []string{"현재 승인된 Argos memory", "뉴스 브리핑은 세 줄 요약"} {
		if !strings.Contains(recall, want) {
			t.Fatalf("recall missing %q:\n%s", want, recall)
		}
	}
}

func TestAssistantMemorySignalApprovalUsesEnglishLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	opts := ListenOptions{TargetID: "argos-assistant-memory-flow-en", Mode: "assistant"}
	plan := assistantReply(opts, "remember this: User prefers concise market briefings with the conclusion first.")
	for _, want := range []string{"Save this to Argos memory?", "Scope: user", "User prefers concise market briefings", "reply `save it`", "To cancel"} {
		if !strings.Contains(plan, want) {
			t.Fatalf("plan missing %q:\n%s", want, plan)
		}
	}
	if assistantCheckoutPrepContainsHangul(plan) {
		t.Fatalf("English memory plan should not expose Korean labels:\n%s", plan)
	}
	saved := assistantReply(opts, "save it")
	for _, want := range []string{"Remembered.", "Scope: user", "User prefers concise market briefings", "Next checks:", "Show memory: `show memory`", "personalization preview:", "Memory is context, not permission."} {
		if !strings.Contains(saved, want) {
			t.Fatalf("saved missing %q:\n%s", want, saved)
		}
	}
	if assistantCheckoutPrepContainsHangul(saved) {
		t.Fatalf("English memory approval should not expose Korean labels:\n%s", saved)
	}
}

func TestAssistantHermesFourAxisReply(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	reply := assistantReply(
		ListenOptions{TargetID: "argos-assistant-hermes-four-axis", Mode: "assistant"},
		"장기 기억(Memory) Skill 자동 생성 작업 학습 및 재사용 자연어 스케줄링 4가지를 헤르메스에서 응용해 가져와",
	)
	for _, want := range []string{
		"Hermes/OpenClaw식 4축",
		"Argos는 MeshClaw 위에서",
		"장기 기억",
		"Skill 자동 생성",
		"작업 학습 및 재사용",
		"자연어 스케줄링",
		"`이 작업 스킬로 만들어줘",
		"작업 학습 및 재사용 카드",
		"읽기 전용",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "발송했습니다", "예약 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("reply should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantWorkReuseCardSuggestsTemplate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	reply := assistantReply(
		ListenOptions{TargetID: "argos-assistant-work-reuse", Mode: "assistant"},
		"작업 재사용: 반복 업무 루틴을 재사용",
	)
	for _, want := range []string{
		"작업 학습 및 재사용 카드",
		"요청: 반복 업무 루틴을 재사용",
		"개인 스킬 상태:",
		"맞는 템플릿:",
		"work-reuse",
		"스킬 초안 만들기: `이 작업 스킬로 만들어줘:",
		"기존 스킬 찾기: `스킬 다음:",
		"메모리/스킬 반영 확인: `개인화 미리보기:",
		"이 카드는 읽기 전용입니다",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "Hermes/OpenClaw식 4축") {
		t.Fatalf("single work-reuse command should not be intercepted by Hermes overview:\n%s", reply)
	}
}

func TestAssistantTaskSkillAliasCreatesDraft(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	opts := ListenOptions{TargetID: "argos-assistant-task-skill-alias", Mode: "assistant"}
	plan := assistantReply(opts, "이 작업 스킬로 만들어줘: Market Routine - summarize market news into meeting topics")
	for _, want := range []string{
		"스킬 설치 초안",
		"Market Routine",
		"market-routine",
		"summarize market news into meeting topics",
		"지금은 파일을 쓰지 않았습니다",
		"스킬 저장해",
	} {
		if !strings.Contains(plan, want) {
			t.Fatalf("plan missing %q:\n%s", want, plan)
		}
	}
	saved := assistantReply(opts, "스킬 저장해")
	for _, want := range []string{
		"스킬을 설치하고 활성화했습니다",
		"Market Routine",
		"active_policy_gated",
		"스킬 테스트 market-routine",
		"최종 승인이 필요",
	} {
		if !strings.Contains(saved, want) {
			t.Fatalf("saved missing %q:\n%s", want, saved)
		}
	}
}

func TestAssistantNaturalSchedulingGuideAndPlan(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	t.Setenv("MESHCLAW_SCHEDULED_DELIVERIES", filepath.Join(home, ".meshclaw", "scheduled-deliveries.json"))
	if _, _, err := UpsertTarget(Target{
		ID:        "yun",
		Channel:   "signal",
		Recipient: "+821012345678",
		Label:     "윤",
		Mode:      "assistant",
	}); err != nil {
		t.Fatal(err)
	}
	opts := ListenOptions{TargetID: "argos-assistant-natural-schedule", Mode: "assistant"}
	guide := assistantReply(opts, "자연어 스케줄링")
	for _, want := range []string{
		"자연어 스케줄링 카드",
		"예약할 문장을 같이 보내면",
		"예시:",
		"이 안내 카드는 읽기 전용입니다",
	} {
		if !strings.Contains(guide, want) {
			t.Fatalf("guide missing %q:\n%s", want, guide)
		}
	}
	if strings.Contains(guide, "Hermes/OpenClaw식 4축") {
		t.Fatalf("single natural scheduling command should not be intercepted by Hermes overview:\n%s", guide)
	}
	plan := signalReplyVisibleText(assistantReply(opts, "자연어 스케줄링: 매일 오전 8시에 윤에게 오늘 브리핑 보내줘"))
	for _, want := range []string{
		"예약 발송 계획을 만들었습니다.",
		"- 상태: review_ready",
		"- 대상: 윤",
		"아직 예약 등록이나 발송은 하지 않았습니다.",
		"예약 등록 승인",
	} {
		if !strings.Contains(plan, want) {
			t.Fatalf("plan missing %q:\n%s", want, plan)
		}
	}
	if strings.Contains(plan, "registered") || strings.Contains(plan, "sent") || strings.Contains(plan, "발송했습니다") {
		t.Fatalf("natural scheduling plan must remain plan-only:\n%s", plan)
	}
	if pending, ok := takePendingAssistantConversation(assistantPendingKey(opts.TargetID), false); !ok || pending.Kind != "scheduled_delivery_apply" {
		t.Fatalf("expected scheduled delivery pending apply, got ok=%t pending=%#v", ok, pending)
	}
	registered := signalReplyVisibleText(assistantReply(opts, "예약 등록 승인"))
	for _, want := range []string{
		"예약 발송을 등록했습니다.",
		"- 대상: 윤",
		"- 주기: 매일 오전 8시에",
		"지금 즉시 발송하지 않았습니다.",
		"예약 발송 상태",
	} {
		if !strings.Contains(registered, want) {
			t.Fatalf("registered reply missing %q:\n%s", want, registered)
		}
	}
	status := assistantReply(opts, "예약 발송 상태")
	for _, want := range []string{"Argos 예약 발송 상태", "등록 1개", "활성 1개", "윤", "오늘 브리핑"} {
		if !strings.Contains(status, want) {
			t.Fatalf("status missing %q:\n%s", want, status)
		}
	}
}

func TestAssistantSkillInstallSignalApprovalFlow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	opts := ListenOptions{TargetID: "argos-assistant-skill-flow", Mode: "assistant"}
	plan := assistantReply(opts, "스킬 만들어 Research Clips - 출처 있는 시장조사 요약을 만들고 발송 전에는 멈춘다")
	for _, want := range []string{"스킬 설치 초안", "Research Clips", "research-clips", "지금은 파일을 쓰지 않았습니다", "스킬 저장해"} {
		if !strings.Contains(plan, want) {
			t.Fatalf("plan missing %q:\n%s", want, plan)
		}
	}
	saved := assistantReply(opts, "스킬 저장해")
	for _, want := range []string{"스킬을 설치하고 활성화했습니다", "Research Clips", "active_policy_gated", "다음 안전 확인:", "스킬 테스트 research-clips", "스킬 검토 research-clips", "스킬 비활성화 research-clips", "최종 승인이 필요"} {
		if !strings.Contains(saved, want) {
			t.Fatalf("saved missing %q:\n%s", want, saved)
		}
	}
	profile, err := assistantprofile.Inspect(time.Now(), "")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Skills.Count != 1 || profile.Skills.Items[0].Activation != "active_policy_gated" {
		t.Fatalf("profile skills=%#v", profile.Skills)
	}
	if profile.Skills.Items[0].UpdatedAt.IsZero() {
		t.Fatalf("profile skill should expose updated time: %#v", profile.Skills.Items[0])
	}
	status := assistantReply(opts, "스킬 현황")
	for _, want := range []string{"개인 스킬 마켓", "Research Clips", "active_policy_gated", "수정 "} {
		if !strings.Contains(status, want) {
			t.Fatalf("status missing %q:\n%s", want, status)
		}
	}
}

func TestAssistantSkillRevisionApprovalFlow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	if _, err := assistantprofile.InstallSkill(time.Now(), "", "Research Report", "출처 있는 시장조사 요약을 만들고 발송 전 멈춘다", true); err != nil {
		t.Fatal(err)
	}
	opts := ListenOptions{TargetID: "argos-assistant-skill-revision", Mode: "assistant"}
	plan := assistantReply(opts, "스킬 수정 research-report: 출처 5개 이상으로 시장 보고서를 만들고 발송 전 멈춘다")
	for _, want := range []string{
		"스킬 수정 초안입니다.",
		"스킬: Research Report",
		"변경 미리보기:",
		"기존: 출처 있는 시장조사",
		"새로 적용: 출처 5개 이상",
		"새 동작: 출처 5개 이상",
		"변경 파일: SKILL.md, TEST.md, REVIEWED.md",
		"지금은 파일을 바꾸지 않았습니다.",
		"스킬 수정 승인",
		"재테스트: `스킬 테스트 research-report`",
		"결과 검토: `스킬 테스트 결과 research-report:",
		"스킬 검토 research-report",
	} {
		if !strings.Contains(plan, want) {
			t.Fatalf("plan missing %q:\n%s", want, plan)
		}
	}
	saved := assistantReply(opts, "스킬 수정 승인")
	for _, want := range []string{
		"스킬을 수정했습니다.",
		"스킬: Research Report",
		"상태: active_policy_gated",
		"적용된 동작: 출처 5개 이상",
		"다음 확인:",
		"`스킬 현황`",
		"재테스트: `스킬 테스트 research-report`",
		"결과 검토: `스킬 테스트 결과 research-report:",
		"스킬 검토 research-report",
		"스킬 비활성화 research-report",
	} {
		if !strings.Contains(saved, want) {
			t.Fatalf("saved missing %q:\n%s", want, saved)
		}
	}
	testReport, err := assistantprofile.TestSkill(time.Now(), "", "research-report")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(testReport.Prompt, "출처 5개 이상") {
		t.Fatalf("revised test prompt was not persisted: %#v", testReport)
	}
}

func TestAssistantSkillRevisionUsesEnglishLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	if _, err := assistantprofile.InstallSkill(time.Now(), "", "Research Report", "Summarize sourced market research and stop before sending.", true); err != nil {
		t.Fatal(err)
	}
	opts := ListenOptions{TargetID: "argos-assistant-skill-revision-en", Mode: "assistant"}
	plan := assistantReply(opts, "edit skill research-report: summarize five current sources and stop before sending")
	for _, want := range []string{
		"Skill revision draft.",
		"Skill: Research Report",
		"Revision preview:",
		"Current: Summarize sourced market research",
		"Will apply: summarize five current sources",
		"New behavior: summarize five current sources",
		"Files to change: SKILL.md, TEST.md, REVIEWED.md",
		"No files were changed yet.",
		"approve skill revision",
		"Retest: `test skill research-report`",
		"Result review: `skill test result research-report:",
		"Safety review: `skill review research-report`",
	} {
		if !strings.Contains(plan, want) {
			t.Fatalf("plan missing %q:\n%s", want, plan)
		}
	}
	if assistantCheckoutPrepContainsHangul(plan) {
		t.Fatalf("English skill revision plan should not expose Korean:\n%s", plan)
	}
	saved := assistantReply(opts, "approve skill revision")
	for _, want := range []string{
		"Skill revised.",
		"Skill: Research Report",
		"Status: active_policy_gated",
		"Applied behavior: summarize five current sources",
		"Next checks:",
		"skill status",
		"Retest: `test skill research-report`",
		"Result review: `skill test result research-report:",
		"Safety review: `skill review research-report`",
		"Deactivation draft: `disable skill research-report`",
	} {
		if !strings.Contains(saved, want) {
			t.Fatalf("saved missing %q:\n%s", want, saved)
		}
	}
	if assistantCheckoutPrepContainsHangul(saved) {
		t.Fatalf("English skill revision approval should not expose Korean:\n%s", saved)
	}
}

func TestAssistantSkillInstallRejectsSecretWithRewriteGuidance(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	secret := "placeholder-secret-value-123456"
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-skill-secret-reject", Mode: "assistant"}, "스킬 만들어 Secret Skill - token="+secret+"으로 매일 요약")
	for _, want := range []string{
		"이 내용은 Argos 스킬로 등록하지 않겠습니다.",
		"비밀번호/토큰/API key는 스킬 파일에 넣지 않고 Guard/Vault handle만 사용해야 합니다.",
		"안전하게 다시 쓰는 예시:",
		"발송/구매/예약 확정 전에는 멈춘다",
		"Guard/Vault handle",
		"`스킬 마켓 보여줘`",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("secret rejection reply missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, secret) {
		t.Fatalf("secret rejection reply leaked raw secret:\n%s", reply)
	}
}

func TestAssistantSkillInstallRejectsSecretEnglishWithRewriteGuidance(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	secret := "placeholder-secret-value-123456"
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-skill-secret-reject-en", Mode: "assistant"}, "create skill Secret Skill - use token="+secret+" for a daily summary")
	for _, want := range []string{
		"This content will not be registered as an Argos skill.",
		"Passwords, tokens, and API keys must not go into skill files",
		"Rewrite without secrets:",
		"stop before sending, purchasing, or confirming bookings",
		"Guard/Vault handle",
		"`skill market`",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English secret rejection reply missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, secret) {
		t.Fatalf("English secret rejection reply leaked raw secret:\n%s", reply)
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English secret rejection reply should not expose Korean:\n%s", reply)
	}
}

func TestAssistantSkillMarketTemplateInstallFlow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	opts := ListenOptions{TargetID: "argos-assistant-skill-market", Mode: "assistant"}
	market := assistantReply(opts, "스킬 마켓 보여줘")
	for _, want := range []string{"Argos 스킬 마켓", "빠른 시작:", "스킬 다음: 쿠팡에서 휴지 후보 비교", "research-report", "meeting-minutes", "스킬 설치 research-report", "상세 보기: `스킬 상세 research-report`", "설치 후 테스트: `스킬 테스트 research-report`", "스킬 저장해` 한 번", "직접 만들려면"} {
		if !strings.Contains(market, want) {
			t.Fatalf("market missing %q:\n%s", want, market)
		}
	}
	plan := assistantReply(opts, "스킬 설치 research-report")
	for _, want := range []string{"스킬 템플릿 설치 초안", "템플릿: research-report", "Research Report", "스킬 저장해", "저장 후 바로 확인:", "스킬 테스트 research-report", "스킬 테스트 결과 research-report"} {
		if !strings.Contains(plan, want) {
			t.Fatalf("plan missing %q:\n%s", want, plan)
		}
	}
	saved := assistantReply(opts, "스킬 활성화해")
	for _, want := range []string{"스킬을 설치하고 활성화했습니다", "Research Report", "active_policy_gated", "스킬 테스트 research-report", "스킬 테스트 결과 research-report", "스킬 검토 research-report"} {
		if !strings.Contains(saved, want) {
			t.Fatalf("saved missing %q:\n%s", want, saved)
		}
	}
}

func TestAssistantSkillTemplateInstallFlowUsesEnglishLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	opts := ListenOptions{TargetID: "argos-assistant-skill-market-en", Mode: "assistant"}
	plan := assistantReply(opts, "install skill research-report")
	for _, want := range []string{
		"Skill template install draft.",
		"Template: research-report",
		"Name: Research Report",
		"No files were written yet.",
		"reply `save skill` or `activate skill`",
		"After saving:",
		"test skill research-report",
		"skill test result research-report",
		"separate final approval",
	} {
		if !strings.Contains(plan, want) {
			t.Fatalf("plan missing %q:\n%s", want, plan)
		}
	}
	if assistantCheckoutPrepContainsHangul(plan) {
		t.Fatalf("English skill template plan should not expose Korean labels:\n%s", plan)
	}
	saved := assistantReply(opts, "activate skill")
	for _, want := range []string{"Skill installed and activated.", "Name: Research Report", "Status: active_policy_gated", "Next safety checks:", "test skill research-report", "skill test result research-report", "skill review research-report", "disable skill research-report", "Send `skill status`"} {
		if !strings.Contains(saved, want) {
			t.Fatalf("saved missing %q:\n%s", want, saved)
		}
	}
	if assistantCheckoutPrepContainsHangul(saved) {
		t.Fatalf("English skill install approval should not expose Korean labels:\n%s", saved)
	}
}

func TestAssistantSkillMarketShowsInstalledSkillShortcuts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := assistantprofile.InstallSkill(time.Now(), "", "Research Report", "Collect current sources and stop before sending.", true); err != nil {
		t.Fatal(err)
	}
	market := assistantReply(ListenOptions{TargetID: "argos-assistant-skill-market-installed", Mode: "assistant"}, "스킬 마켓 보여줘")
	for _, want := range []string{
		"Argos 스킬 마켓",
		"개인 스킬: 1개 등록, 1개 활성",
		"상태별 다음 행동:",
		"활성: Research Report -> `스킬 테스트 research-report` 또는 바로 요청",
		"설치된 개인 스킬:",
		"Research Report",
		"active_policy_gated",
		"최근 수정:",
		"상세: `스킬 상세 research-report`",
		"테스트: `스킬 테스트 research-report`",
		"안전 검토: `스킬 검토 research-report`",
		"수정 초안: `스킬 수정 research-report: 새 동작 설명`",
		"비활성화 초안: `스킬 비활성화 research-report`",
		"마켓 카드는 읽기 전용",
	} {
		if !strings.Contains(market, want) {
			t.Fatalf("market missing %q:\n%s", want, market)
		}
	}
	for _, bad := range []string{"구매 완료", "발송했습니다", "예약 완료"} {
		if strings.Contains(market, bad) {
			t.Fatalf("market should not claim execution %q:\n%s", bad, market)
		}
	}
}

func TestAssistantSkillMarketShowsReviewedInactiveNextAction(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := assistantprofile.InstallSkill(time.Now(), "", "Meeting Notes", "Turn notes into action items.", true); err != nil {
		t.Fatal(err)
	}
	if _, err := assistantprofile.SetSkillActivation(time.Now(), "", "meeting-notes", false, true); err != nil {
		t.Fatal(err)
	}
	market := assistantReply(ListenOptions{TargetID: "argos-assistant-skill-market-inactive", Mode: "assistant"}, "스킬 마켓 보여줘")
	for _, want := range []string{
		"상태별 다음 행동:",
		"검토됨: Meeting Notes -> `스킬 활성화 meeting-notes`",
		"활성화: `스킬 활성화 meeting-notes` -> `스킬 활성화 승인`",
		"마켓 카드는 읽기 전용",
	} {
		if !strings.Contains(market, want) {
			t.Fatalf("market missing %q:\n%s", want, market)
		}
	}
	if strings.Contains(market, "active_policy_gated") {
		t.Fatalf("inactive skill should not be shown as active:\n%s", market)
	}
}

func TestAssistantSkillMarketUsesEnglishLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	if _, err := assistantprofile.InstallSkill(time.Now(), "", "Research Report", "Collect current sources and stop before sending.", true); err != nil {
		t.Fatal(err)
	}
	market := assistantReply(ListenOptions{TargetID: "argos-assistant-skill-market-en-installed", Mode: "assistant"}, "skill market")
	for _, want := range []string{
		"Argos skill marketplace.",
		"Personal skills: 1 installed, 1 active",
		"Status next actions:",
		"Active: Research Report -> `test skill research-report` or ask the task directly",
		"Installed personal skills:",
		"Research Report",
		"active_policy_gated",
		"Updated:",
		"Detail: `skill detail research-report`",
		"Test: `test skill research-report`",
		"Safety review: `skill review research-report`",
		"Revision draft: `edit skill research-report: new bounded behavior`",
		"Deactivation draft: `disable skill research-report`",
		"Marketplace card is read-only",
	} {
		if !strings.Contains(market, want) {
			t.Fatalf("market missing %q:\n%s", want, market)
		}
	}
	if assistantCheckoutPrepContainsHangul(market) {
		t.Fatalf("English skill market should not expose Korean labels:\n%s", market)
	}
}

func TestAssistantSkillTemplateDetailReply(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-skill-template-detail", Mode: "assistant"}, "스킬 상세 research-report")
	for _, want := range []string{
		"Argos 스킬 템플릿 상세",
		"템플릿: research-report",
		"스킬: Research Report",
		"설치 초안: `스킬 설치 research-report`",
		"설치 후 테스트: `스킬 테스트 research-report`",
		"읽기 전용",
		"별도 최종 승인",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "발송했습니다", "예약 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("reply should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantSkillSearchTemplateReply(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-skill-search-template", Mode: "assistant"}, "스킬 검색 shopping")
	for _, want := range []string{
		"Argos 스킬 검색 결과",
		"검색어: shopping",
		"템플릿:",
		"shopping-prep",
		"설치: `스킬 설치 shopping-prep`",
		"상세: `스킬 상세 shopping-prep`",
		"읽기 전용",
		"별도 최종 승인",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "발송했습니다", "예약 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("reply should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantSkillSearchExpandedTemplateReplies(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	tests := []struct {
		name  string
		text  string
		wants []string
	}{
		{
			name: "travel",
			text: "스킬 검색 여행 호텔 항공",
			wants: []string{
				"Argos 스킬 검색 결과",
				"템플릿:",
				"travel-prep",
				"설치: `스킬 설치 travel-prep`",
				"상세: `스킬 상세 travel-prep`",
			},
		},
		{
			name: "market",
			text: "스킬 검색 유가 환율 시장 분석",
			wants: []string{
				"Argos 스킬 검색 결과",
				"market-analysis",
				"설치: `스킬 설치 market-analysis`",
				"상세: `스킬 상세 market-analysis`",
			},
		},
		{
			name: "calendar",
			text: "스킬 검색 캘린더 리마인더 일정",
			wants: []string{
				"Argos 스킬 검색 결과",
				"calendar-reminder",
				"설치: `스킬 설치 calendar-reminder`",
				"상세: `스킬 상세 calendar-reminder`",
			},
		},
		{
			name: "daily voice",
			text: "스킬 검색 오늘 날씨 음성 브리핑",
			wants: []string{
				"Argos 스킬 검색 결과",
				"daily-briefing",
				"voice-briefing",
				"설치: `스킬 설치 daily-briefing`",
				"설치: `스킬 설치 voice-briefing`",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reply := assistantReply(ListenOptions{TargetID: "argos-assistant-skill-search-" + tt.name, Mode: "assistant"}, tt.text)
			for _, want := range tt.wants {
				if !strings.Contains(reply, want) {
					t.Fatalf("reply missing %q:\n%s", want, reply)
				}
			}
			for _, bad := range []string{"구매 완료", "발송했습니다", "예약 완료"} {
				if strings.Contains(reply, bad) {
					t.Fatalf("reply should not claim execution %q:\n%s", bad, reply)
				}
			}
		})
	}
}

func TestAssistantSkillSearchInstalledReply(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := assistantprofile.InstallSkill(time.Now(), "", "Research Report", "Collect current sources and stop before sending.", true); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-skill-search-installed", Mode: "assistant"}, "스킬 찾기 current sources")
	for _, want := range []string{
		"Argos 스킬 검색 결과",
		"검색어: current sources",
		"설치된 개인 스킬:",
		"Research Report",
		"active_policy_gated",
		"최근 수정:",
		"상세: `스킬 상세 research-report`",
		"테스트: `스킬 테스트 research-report`",
		"수정 초안: `스킬 수정 research-report: 새 동작 설명`",
		"읽기 전용",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "발송했습니다", "예약 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("reply should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantSkillRecommendationTemplateReply(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-skill-recommend-template", Mode: "assistant"}, "스킬 추천: 쿠팡에서 무선 키보드 후보 비교하고 구매 전 멈추기")
	for _, want := range []string{
		"Argos 스킬 추천 카드",
		"요청: 쿠팡에서 무선 키보드 후보 비교하고 구매 전 멈추기",
		"설치 후보 템플릿:",
		"shopping-prep",
		"설치 초안: `스킬 설치 shopping-prep`",
		"상세: `스킬 상세 shopping-prep`",
		"추천은 읽기 전용",
		"별도 최종 승인",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "발송했습니다", "예약 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("reply should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantSkillRecommendationInstalledReply(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := assistantprofile.InstallSkill(time.Now(), "", "Research Report", "Collect current sources and stop before sending.", true); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-skill-recommend-installed", Mode: "assistant"}, "스킬 추천: current sources 모아서 보고서 만들기")
	for _, want := range []string{
		"Argos 스킬 추천 카드",
		"바로 참고할 수 있는 개인 스킬:",
		"Research Report",
		"active_policy_gated",
		"최근 수정:",
		"상세: `스킬 상세 research-report`",
		"테스트 카드: `스킬 테스트 research-report`",
		"수정 초안: `스킬 수정 research-report: 새 동작 설명`",
		"설치 후보 템플릿:",
		"research-report",
		"추천은 읽기 전용",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "발송했습니다", "예약 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("reply should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantSkillNextTemplateReply(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-skill-next-template", Mode: "assistant"}, "스킬 다음: 쿠팡에서 무선 키보드 후보 비교하고 구매 전 멈추기")
	for _, want := range []string{
		"Argos 스킬 다음 단계",
		"아직 설치되지 않은 템플릿 후보",
		"shopping-prep",
		"설치 초안: `스킬 설치 shopping-prep`",
		"상세 보기: `스킬 상세 shopping-prep`",
		"읽기 전용",
		"별도 최종 승인",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "발송했습니다", "예약 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("reply should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantSkillNextExpandedTemplateReply(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-skill-next-market-template", Mode: "assistant"}, "스킬 다음: 유가와 환율 시장 분석을 90초 음성 브리핑으로 만들기")
	for _, want := range []string{
		"Argos 스킬 다음 단계",
		"아직 설치되지 않은 템플릿 후보",
		"market-analysis",
		"설치 초안: `스킬 설치 market-analysis`",
		"상세 보기: `스킬 상세 market-analysis`",
		"읽기 전용",
		"별도 최종 승인",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "발송했습니다", "예약 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("reply should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantSkillNextTravelTemplateUsesEnglishLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-skill-next-travel-en", Mode: "assistant"}, "skill next: prepare a travel plan with flight and hotel candidates")
	for _, want := range []string{
		"Argos skill next step.",
		"Template candidate not installed yet:",
		"travel-prep",
		"Install draft: `install skill travel-prep`",
		"Details: `skill detail travel-prep`",
		"This next-step card is read-only.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English skill next card should not expose Korean labels:\n%s", reply)
	}
}

func TestAssistantSkillNextActiveInstalledReply(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := assistantprofile.InstallSkill(time.Now(), "", "Research Report", "Collect current sources and stop before sending.", true); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-skill-next-active", Mode: "assistant"}, "스킬 다음 current sources 모아서 보고서 만들기")
	for _, want := range []string{
		"Argos 스킬 다음 단계",
		"바로 참고할 활성 개인 스킬",
		"Research Report",
		"active_policy_gated",
		"먼저 확인: `스킬 테스트 research-report`",
		"상세 보기: `스킬 상세 research-report`",
		"바로 요청: `current sources 모아서 보고서 만들기`",
		"읽기 전용",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "발송했습니다", "예약 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("reply should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantSkillNextInactiveInstalledReply(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := assistantprofile.InstallSkill(time.Now(), "", "Research Report", "Collect current sources and stop before sending.", true); err != nil {
		t.Fatal(err)
	}
	if _, err := assistantprofile.SetSkillActivation(time.Now(), "", "research-report", false, true); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-skill-next-inactive", Mode: "assistant"}, "다음 스킬 current sources 모아서 보고서 만들기")
	for _, want := range []string{
		"Argos 스킬 다음 단계",
		"설치됐지만 활성화 전 확인이 필요한 개인 스킬",
		"Research Report",
		"reviewed_approval_required",
		"안전 검토: `스킬 검토 research-report`",
		"활성화 초안: `스킬 활성화 research-report`",
		"상세 보기: `스킬 상세 research-report`",
		"읽기 전용",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "발송했습니다", "예약 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("reply should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantSkillReviewReply(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := assistantprofile.InstallSkill(time.Now(), "", "Research Report", "Collect current sources and stop before sending.", true); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-skill-review", Mode: "assistant"}, "스킬 검토 research-report")
	for _, want := range []string{
		"Argos 스킬 안전 검토",
		"스킬: Research Report",
		"상태: active_policy_gated",
		"활성화: active_policy_gated",
		"검토 마커: reviewed",
		"최근 수정:",
		"TEST.md 확인됨",
		"발송, 구매, 예약 확정, 삭제, 결제는 최종 승인 전 실행 금지",
		"테스트 카드: `스킬 테스트 research-report`",
		"상세: `스킬 상세 research-report`",
		"비활성화 초안: `스킬 비활성화 research-report`",
		"안전 검토는 읽기 전용",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "발송했습니다", "예약 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("reply should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantSkillReviewMissingTestReply(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	skillDir := filepath.Join(home, ".meshclaw", "skills", "briefing")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Briefing\n\nSummarize and stop before sending.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "REVIEWED.md"), []byte("reviewed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-skill-review-missing-test", Mode: "assistant"}, "스킬 안전 검토 briefing")
	for _, want := range []string{
		"Argos 스킬 안전 검토",
		"스킬: Briefing",
		"검토 마커: reviewed",
		"TEST.md 없음, fallback 테스트만 가능",
		"활성화 초안: `스킬 활성화 briefing`",
		"안전 검토는 읽기 전용",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
}

func TestAssistantInstalledSkillDetailReply(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := assistantprofile.InstallSkill(time.Now(), "", "Research Report", "Collect current sources and stop before sending.", true); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-installed-skill-detail", Mode: "assistant"}, "스킬 정보 research-report")
	for _, want := range []string{
		"Argos 스킬 상세",
		"스킬: Research Report",
		"상태: active_policy_gated",
		"활성화: active_policy_gated",
		"최근 수정:",
		"테스트 카드: `스킬 테스트 research-report`",
		"수정 초안: `스킬 수정 research-report: 새 동작 설명`",
		"비활성화 초안: `스킬 비활성화 research-report`",
		"읽기 전용",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "발송했습니다", "예약 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("reply should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantSkillTestCardFlow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	if _, err := assistantprofile.InstallSkill(time.Now(), "", "Research Report", "Collect current sources and stop before sending.", true); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-skill-test", Mode: "assistant"}, "스킬 테스트 research-report")
	for _, want := range []string{"스킬 테스트 카드", "Research Report", "테스트 프롬프트", "기대 결과", "안전 경계", "테스트 후 다음 명령", "안전 검토: `스킬 검토 research-report`", "비활성화 초안: `스킬 비활성화 research-report`", "실행하지 않는 테스트 설명"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "발송했습니다", "예약 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("reply should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantSkillTestCardUsesEnglishLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	if _, err := assistantprofile.InstallSkill(time.Now(), "", "Research Report", "Collect current sources and stop before sending.", true); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-skill-test-en", Mode: "assistant"}, "test skill research-report")
	for _, want := range []string{"Skill test card.", "Skill: Research Report", "Test prompt:", "Expected result:", "Safety boundary:", "Next commands after testing:", "Safety review: `skill review research-report`", "Deactivation draft: `disable skill research-report`", "This card is a non-executing test description."} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English skill test card should not expose Korean labels:\n%s", reply)
	}
}

func TestAssistantSkillTestResultReviewReply(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	if _, err := assistantprofile.InstallSkill(time.Now(), "", "Research Report", "Collect current sources and stop before sending.", true); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-skill-test-result", Mode: "assistant"}, "스킬 테스트 결과 research-report: 보고서 초안을 만들었고 발송, 구매, 예약은 하지 않았음")
	for _, want := range []string{
		"스킬 테스트 결과 검토",
		"스킬: Research Report",
		"붙여넣은 테스트 결과",
		"기대 결과 대조",
		"멈춤선 점검",
		"위험 실행 완료 주장은 보이지 않습니다",
		"안전 검토: `스킬 검토 research-report`",
		"비활성화 초안: `스킬 비활성화 research-report`",
		"읽기 전용",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "발송했습니다", "예약 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("reply should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantSkillTestResultReviewPreparesActivationApproval(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	if _, err := assistantprofile.InstallSkill(time.Now(), "", "Research Report", "Collect current sources and stop before sending.", true); err != nil {
		t.Fatal(err)
	}
	if _, err := assistantprofile.SetSkillActivation(time.Now(), "", "research-report", false, true); err != nil {
		t.Fatal(err)
	}
	opts := ListenOptions{TargetID: "argos-assistant-skill-test-result-activation", Mode: "assistant"}
	reply := assistantReply(opts, "스킬 테스트 결과 research-report: 보고서 초안을 만들었고 발송, 구매, 예약은 하지 않았음")
	for _, want := range []string{
		"스킬 테스트 결과 검토",
		"활성화 초안: `스킬 활성화 research-report`",
		"활성화 승인 대기를 준비했습니다",
		"권장 다음 한 줄:",
		"진행하려면 `스킬 활성화 승인`이라고 답하세요.",
		"다음 한 줄: `스킬 활성화 승인`",
		"읽기 전용",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	if recommended, nextCommands := strings.Index(reply, "권장 다음 한 줄:"), strings.Index(reply, "테스트 후 다음 명령:"); recommended < 0 || nextCommands < 0 || recommended > nextCommands {
		t.Fatalf("recommended activation one-liner should appear before the broader next-command list:\n%s", reply)
	}
	approved := assistantReply(opts, "스킬 활성화 승인")
	for _, want := range []string{"스킬을 활성화했습니다", "Research Report", "active_policy_gated", "바로 확인:", "`스킬 현황`", "테스트 카드: `스킬 테스트 research-report`", "바로 요청: `Research Report 실행해줘`", "상세 보기: `스킬 상세 research-report`", "안전 검토: `스킬 검토 research-report`"} {
		if !strings.Contains(approved, want) {
			t.Fatalf("approved reply missing %q:\n%s", want, approved)
		}
	}
}

func TestAssistantSkillTestResultReviewWarnsOnExecutionClaim(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	if _, err := assistantprofile.InstallSkill(time.Now(), "", "Research Report", "Collect current sources and stop before sending.", true); err != nil {
		t.Fatal(err)
	}
	if _, err := assistantprofile.SetSkillActivation(time.Now(), "", "research-report", false, true); err != nil {
		t.Fatal(err)
	}
	opts := ListenOptions{TargetID: "argos-assistant-skill-test-result-warning", Mode: "assistant"}
	reply := assistantReply(opts, "스킬 테스트 결과 research-report: 보고서 초안 작성 후 메일 발송했습니다")
	if !strings.Contains(reply, "안전 검토가 필요합니다") {
		t.Fatalf("reply should warn on execution claim:\n%s", reply)
	}
	if !strings.Contains(reply, "권장 다음 한 줄:") {
		t.Fatalf("reply should show the recommended review one-liner header:\n%s", reply)
	}
	if !strings.Contains(reply, "다음 한 줄: `스킬 검토 research-report`") {
		t.Fatalf("reply should show direct review next action:\n%s", reply)
	}
	if !strings.Contains(reply, "수정 초안: `스킬 수정 research-report: 발송/구매/예약/삭제/결제 완료를 말하지 말고 초안에서 멈춤`") {
		t.Fatalf("reply should show direct revision follow-up:\n%s", reply)
	}
	if recommended, nextCommands := strings.Index(reply, "권장 다음 한 줄:"), strings.Index(reply, "테스트 후 다음 명령:"); recommended < 0 || nextCommands < 0 || recommended > nextCommands {
		t.Fatalf("recommended review one-liner should appear before the broader next-command list:\n%s", reply)
	}
	if strings.Contains(reply, "진행하려면 `스킬 활성화 승인`") {
		t.Fatalf("warning result should not expose activation approval text:\n%s", reply)
	}
	if !strings.Contains(reply, "읽기 전용") {
		t.Fatalf("reply should stay read-only:\n%s", reply)
	}
	approved := assistantReply(opts, "스킬 활성화 승인")
	if strings.Contains(approved, "스킬을 활성화했습니다") {
		t.Fatalf("warning result should not prepare activation approval:\n%s", approved)
	}
}

func TestAssistantSkillTestResultReviewEnglishWarnsWithRevisionFollowUp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	if _, err := assistantprofile.InstallSkill(time.Now(), "", "Research Report", "Collect current sources and stop before sending.", true); err != nil {
		t.Fatal(err)
	}
	if _, err := assistantprofile.SetSkillActivation(time.Now(), "", "research-report", false, true); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-skill-test-result-warning-en", Mode: "assistant"}, "skill test result research-report: drafted the report and sent the email")
	for _, want := range []string{
		"Skill test result review.",
		"review it before activation",
		"Recommended one-liner:",
		"Next one-liner: `skill review research-report`",
		"Revision draft: `edit skill research-report: stop before claiming send, purchase, booking, deletion, or payment completion`",
		"This result review is read-only.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English warning review should not expose Korean:\n%s", reply)
	}
}

func TestAssistantSkillTestResultReviewEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	if _, err := assistantprofile.InstallSkill(time.Now(), "", "Research Report", "Collect current sources and stop before sending.", true); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-skill-test-result-en", Mode: "assistant"}, "skill test result research-report: produced a report draft and did not send, purchase, or book anything")
	for _, want := range []string{
		"Skill test result review.",
		"Skill: Research Report",
		"Pasted test result:",
		"Expected-result check:",
		"Stopping-boundary check:",
		"No risky execution-complete claim is visible.",
		"Safety review: `skill review research-report`",
		"Deactivation draft: `disable skill research-report`",
		"This result review is read-only.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English skill test result review should not expose Korean labels:\n%s", reply)
	}
}

func TestAssistantSkillTestResultReviewEnglishPreparesActivationApproval(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	if _, err := assistantprofile.InstallSkill(time.Now(), "", "Research Report", "Collect current sources and stop before sending.", true); err != nil {
		t.Fatal(err)
	}
	if _, err := assistantprofile.SetSkillActivation(time.Now(), "", "research-report", false, true); err != nil {
		t.Fatal(err)
	}
	opts := ListenOptions{TargetID: "argos-assistant-skill-test-result-activation-en", Mode: "assistant"}
	reply := assistantReply(opts, "skill test result research-report: produced a report draft and did not send, purchase, or book anything")
	for _, want := range []string{
		"Skill test result review.",
		"Activation draft: `activate skill research-report`",
		"Activation approval is prepared",
		"Recommended one-liner:",
		"To proceed, reply `approve skill activation`.",
		"Next one-liner: `approve skill activation`",
		"This result review is read-only.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	if recommended, nextCommands := strings.Index(reply, "Recommended one-liner:"), strings.Index(reply, "Next commands after testing:"); recommended < 0 || nextCommands < 0 || recommended > nextCommands {
		t.Fatalf("recommended English activation one-liner should appear before the broader next-command list:\n%s", reply)
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English activation-prep review should not expose Korean labels:\n%s", reply)
	}
	approved := assistantReply(opts, "approve skill activation")
	for _, want := range []string{"Skill activated.", "Research Report", "active_policy_gated", "Quick checks:", "skill status", "Test card: `test skill research-report`", "Use now: `Run Research Report`", "Details: `skill detail research-report`", "Safety review: `skill review research-report`"} {
		if !strings.Contains(approved, want) {
			t.Fatalf("approved reply missing %q:\n%s", want, approved)
		}
	}
}

func TestAssistantSkillActivationToggleFlow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := assistantprofile.InstallSkill(time.Now(), "", "Research Report", "Collect current sources and stop before sending.", true); err != nil {
		t.Fatal(err)
	}
	opts := ListenOptions{TargetID: "argos-assistant-skill-toggle", Mode: "assistant"}
	planOff := assistantReply(opts, "스킬 비활성화 research-report")
	for _, want := range []string{"스킬 비활성화 초안", "Research Report", "지금은 파일을 바꾸지 않았습니다", "스킬 비활성화 승인"} {
		if !strings.Contains(planOff, want) {
			t.Fatalf("planOff missing %q:\n%s", want, planOff)
		}
	}
	off := assistantReply(opts, "스킬 비활성화 승인")
	for _, want := range []string{"스킬을 비활성화했습니다", "Research Report", "reviewed_approval_required", "바로 확인:", "`스킬 현황`", "상세 보기: `스킬 상세 research-report`", "활성화 초안: `스킬 활성화 research-report`"} {
		if !strings.Contains(off, want) {
			t.Fatalf("off missing %q:\n%s", want, off)
		}
	}
	planOn := assistantReply(opts, "스킬 활성화 research-report")
	for _, want := range []string{"스킬 활성화 초안", "Research Report", "스킬 활성화 승인"} {
		if !strings.Contains(planOn, want) {
			t.Fatalf("planOn missing %q:\n%s", want, planOn)
		}
	}
	on := assistantReply(opts, "스킬 활성화 승인")
	for _, want := range []string{"스킬을 활성화했습니다", "Research Report", "active_policy_gated", "바로 확인:", "`스킬 현황`", "테스트 카드: `스킬 테스트 research-report`", "바로 요청: `Research Report 실행해줘`", "상세 보기: `스킬 상세 research-report`", "비활성화 초안: `스킬 비활성화 research-report`"} {
		if !strings.Contains(on, want) {
			t.Fatalf("on missing %q:\n%s", want, on)
		}
	}
}

func TestAssistantSkillActivationToggleUsesEnglishLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	if _, err := assistantprofile.InstallSkill(time.Now(), "", "Research Report", "Collect current sources and stop before sending.", true); err != nil {
		t.Fatal(err)
	}
	opts := ListenOptions{TargetID: "argos-assistant-skill-toggle-en", Mode: "assistant"}
	planOff := assistantReply(opts, "disable skill research-report")
	for _, want := range []string{"Skill deactivation draft.", "Skill: Research Report", "No files were changed yet.", "approve skill deactivation", "activation marker"} {
		if !strings.Contains(planOff, want) {
			t.Fatalf("planOff missing %q:\n%s", want, planOff)
		}
	}
	if assistantCheckoutPrepContainsHangul(planOff) {
		t.Fatalf("English skill deactivation plan should not expose Korean labels:\n%s", planOff)
	}
	off := assistantReply(opts, "approve skill deactivation")
	for _, want := range []string{"Skill deactivated.", "Skill: Research Report", "Status: reviewed_approval_required", "Quick checks:", "skill status", "Details: `skill detail research-report`", "Activation draft: `activate skill research-report`"} {
		if !strings.Contains(off, want) {
			t.Fatalf("off missing %q:\n%s", want, off)
		}
	}
	if assistantCheckoutPrepContainsHangul(off) {
		t.Fatalf("English skill deactivation approval should not expose Korean labels:\n%s", off)
	}
	planOn := assistantReply(opts, "activate skill research-report")
	for _, want := range []string{"Skill activation draft.", "Skill: Research Report", "approve skill activation"} {
		if !strings.Contains(planOn, want) {
			t.Fatalf("planOn missing %q:\n%s", want, planOn)
		}
	}
	on := assistantReply(opts, "approve skill activation")
	for _, want := range []string{"Skill activated.", "Skill: Research Report", "Status: active_policy_gated", "Quick checks:", "skill status", "Test card: `test skill research-report`", "Use now: `Run Research Report`", "Details: `skill detail research-report`", "Deactivation draft: `disable skill research-report`"} {
		if !strings.Contains(on, want) {
			t.Fatalf("on missing %q:\n%s", want, on)
		}
	}
	if assistantCheckoutPrepContainsHangul(on) {
		t.Fatalf("English skill activation approval should not expose Korean labels:\n%s", on)
	}
}

func TestAssistantPersonalizationStatusReplyShowsMemoryAndSkills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := assistantprofile.AddMemory(time.Now(), "", "user", "사용자는 결론부터 짧게 답하는 것을 선호한다", true); err != nil {
		t.Fatal(err)
	}
	if _, err := assistantprofile.InstallSkill(time.Now(), "", "Short Brief", "Summarize first and keep irreversible actions gated.", true); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-personalization", Mode: "assistant"}, "개인화 상태 보여줘")
	for _, want := range []string{"Argos 개인화 상태", "승인 메모리: ok", "결론부터 짧게", "개인 스킬: 1개 등록, 1개 활성", "다음 명령:", "요청별 반영 확인: `개인화 미리보기: 오늘 메일 요약해줘`", "스킬 보기: `스킬 마켓 보여줘`", "메모리는 권한이 아닙니다"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
}

func TestAssistantPersonalizationStatusUsesEnglishLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	if _, err := assistantprofile.AddMemory(time.Now(), "", "user", "User prefers concise answers with the conclusion first.", true); err != nil {
		t.Fatal(err)
	}
	if _, err := assistantprofile.InstallSkill(time.Now(), "", "Short Brief", "Summarize first and keep irreversible actions gated.", true); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-personalization-en", Mode: "assistant"}, "personalization status")
	for _, want := range []string{
		"Argos personalization status.",
		"Approved memory: ok",
		"User prefers concise answers",
		"Personal skills: 1 installed, 1 active",
		"Next commands:",
		"Check request-specific context: `personalization preview: summarize today's mail`",
		"Show skills: `skill market`",
		"Memory is context, not permission.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English personalization status should not expose Korean labels:\n%s", reply)
	}
}

func TestAssistantPersonalizationPreviewReplyShowsAppliedLayers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := assistantprofile.AddMemory(time.Now(), "", "user", "사용자는 결론부터 짧게 답하는 것을 선호한다", true); err != nil {
		t.Fatal(err)
	}
	if _, err := assistantprofile.InstallSkill(time.Now(), "", "Briefing Skill", "Summarize first, then details. Stop before sends, purchases, bookings, and payments.", true); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-personalization-preview", Mode: "assistant"}, "개인화 미리보기: 스킬 기반 뉴스 브리핑은 결론부터 짧게 해줘")
	for _, want := range []string{
		"Argos 개인화 미리보기",
		"요청: 스킬 기반 뉴스 브리핑은 결론부터 짧게 해줘",
		"이번 요청에서 참고할 계층: long_term, procedural",
		"- long_term:",
		"- procedural:",
		"승인 메모리 일부:",
		"결론부터 짧게",
		"관련 활성 스킬:",
		"- Briefing Skill: Summarize first, then details.",
		"스킬 확인: `스킬 상세 Briefing Skill`",
		"이 스킬로 바로 요청: `스킬 기반 뉴스 브리핑은 결론부터 짧게 해줘`",
		"다음 명령:",
		"기억 확인: `기억 확인`",
		"관련 스킬 찾기: `스킬 다음: 스킬 기반 뉴스 브리핑은 결론부터 짧게 해줘`",
		"이 맥락으로 바로 요청: `스킬 기반 뉴스 브리핑은 결론부터 짧게 해줘`",
		"개인화는 답변 맥락일 뿐 권한이 아닙니다",
		"읽기 전용 미리보기",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"구매 완료", "발송했습니다", "예약 완료"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("reply should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantPersonalizationPreviewUsesEnglishLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	if _, err := assistantprofile.AddMemory(time.Now(), "", "user", "User prefers the conclusion first and concise replies.", true); err != nil {
		t.Fatal(err)
	}
	if _, err := assistantprofile.InstallSkill(time.Now(), "", "Briefing Skill", "Summarize first, then details. Stop before sends, purchases, bookings, and payments.", true); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-personalization-preview-en", Mode: "assistant"}, "personalization preview: skill based news briefing preference with conclusion first")
	for _, want := range []string{
		"Argos personalization preview.",
		"Request: skill based news briefing preference with conclusion first",
		"Personalization layers for this request: long_term, procedural",
		"Approved memory excerpt:",
		"Matching active skill:",
		"- Briefing Skill: Summarize first, then details.",
		"Skill details: `skill detail Briefing Skill`",
		"Ask with this skill: `skill based news briefing preference with conclusion first`",
		"Use: approved user preferences and Argos operating habits",
		"Use: repeatable routines, briefing style, and tool-selection habits",
		"Next commands:",
		"Check memory: `show memory`",
		"Find a matching skill: `skill next: skill based news briefing preference with conclusion first`",
		"Run this request with context: `skill based news briefing preference with conclusion first`",
		"Read-only preview.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English personalization preview should not expose Korean labels:\n%s", reply)
	}
}

func TestAssistantPersonalizationPreviewSuggestsTemplateWhenNoActiveSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-personalization-preview-template", Mode: "assistant"}, "개인화 미리보기: 유가 환율 시장 분석 전망을 정리해줘")
	for _, want := range []string{
		"Argos 개인화 미리보기",
		"요청: 유가 환율 시장 분석 전망을 정리해줘",
		"추천 스킬 템플릿:",
		"- Market Analysis (market-analysis):",
		"설치 초안: `스킬 설치 market-analysis`",
		"설치 후 테스트: `스킬 테스트 market-analysis`",
		"템플릿 확인: `스킬 상세 market-analysis`",
		"읽기 전용 미리보기",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "관련 활성 스킬:") {
		t.Fatalf("inactive/missing skill should not be shown as active:\n%s", reply)
	}
}

func TestAssistantPersonalizationPreviewEnglishSuggestsTemplateWhenNoActiveSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-personalization-preview-template-en", Mode: "assistant"}, "personalization preview: market analysis oil fx forecast")
	for _, want := range []string{
		"Argos personalization preview.",
		"Request: market analysis oil fx forecast",
		"Suggested skill template:",
		"- Market Analysis (market-analysis):",
		"Install draft: `install skill market-analysis`",
		"Test after install: `test skill market-analysis`",
		"Template details: `skill detail market-analysis`",
		"Read-only preview.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "Matching active skill:") {
		t.Fatalf("missing skill should not be shown as active:\n%s", reply)
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English personalization preview template suggestion should not expose Korean labels:\n%s", reply)
	}
}

func TestAssistantGenericFinalApprovalGate(t *testing.T) {
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-final-gate", Mode: "assistant"}, "예약 최종 승인")
	for _, want := range []string{"범용 최종 승인 게이트", "실행하지 않습니다", "booking_confirm", "명시 문구", "증거가 부족하면", "최종 승인 preflight packet", "kind: booking_confirm", "evidence_source: draft_only", "execute=false"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"예약 완료", "구매 완료", "발송했습니다"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("reply should not claim execution %q:\n%s", bad, reply)
		}
	}
}

func TestAssistantApprovalQueueShowsPendingMailDraft(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MAIL_CONFIG", filepath.Join(home, "mail.json"))
	t.Setenv("MESHCLAW_MAIL_DRAFT_DIR", filepath.Join(home, "mail-drafts"))
	t.Setenv("MAIL_PASSWORD", "test-password")
	if _, _, err := mailadapter.UpsertAccount(mailadapter.Account{
		ID:          "101",
		Backend:     "imap",
		Email:       "101@example.com",
		Host:        "imap.example.com",
		Port:        993,
		SMTPHost:    "smtp.example.com",
		SMTPPort:    587,
		Username:    "101@example.com",
		PasswordEnv: "MAIL_PASSWORD",
		TLS:         true,
		SMTPTLS:     true,
	}); err != nil {
		t.Fatal(err)
	}
	opts := ListenOptions{TargetID: "argos-assistant-mail-approval-queue", Mode: "assistant"}
	draft := prepareSignalMailSend(opts, assistantToolIntent{
		Intent:  "mail_send",
		To:      []string{"yoon@example.com"},
		Subject: "회의",
		Body:    "내일 3시에 볼까요",
	})
	for _, want := range []string{"메일 초안을 만들었습니다", "to: yoon@example.com", "subject: 회의", "승인"} {
		if !strings.Contains(draft, want) {
			t.Fatalf("draft missing %q:\n%s", want, draft)
		}
	}
	queue := assistantReply(opts, "승인 대기열 보여줘")
	for _, want := range []string{"Argos 승인 대기열", "바로 보낼 문장:", "메일 발송: `승인`", "최근 메일 발송 대기 후보", "수신=yoon@example.com", "제목=회의", "`승인`", "읽기 전용"} {
		if !strings.Contains(queue, want) {
			t.Fatalf("queue missing %q:\n%s", want, queue)
		}
	}
	final := assistantReply(opts, "발송 최종 승인")
	for _, want := range []string{"범용 최종 승인 게이트", "최근 메일 후보", "수신: yoon@example.com", "제목: 회의", "kind: mail_send", "evidence_source: pending_mail_draft", "execute=false", "not_sent=true"} {
		if !strings.Contains(final, want) {
			t.Fatalf("final approval missing %q:\n%s", want, final)
		}
	}
	for _, bad := range []string{"발송했습니다", "구매 완료", "예약 완료"} {
		if strings.Contains(queue, bad) || strings.Contains(final, bad) {
			t.Fatalf("reply should not claim execution %q:\nqueue:\n%s\nfinal:\n%s", bad, queue, final)
		}
	}
}

func TestAssistantBookingDraftForwardPreview(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	targetID := "argos-assistant-booking-forward"
	draft := strings.Join([]string{
		"예약 문구를 반영했습니다.",
		"반영: 예약자명 홍길동, 연락처 010-1234-5678",
		"",
		"전화/메시지 초안:",
		"안녕하세요. 내일 저녁 7시 2명 방문 예약이 가능한지 문의드립니다. 예약자명은 홍길동입니다. 연락처는 010-1234-5678입니다. 예약금, 취소 기한, 자리 요청 가능 여부도 함께 확인 부탁드립니다.",
		"",
		"확인할 것: 예약자 연락처, 예약금, 취소 기한, 자리 요청 가능 여부",
		"",
		"다음에 말할 것: `연락처 010-0000-0000 넣어줘`, `더 짧게`, `보고방에 보내줘`",
	}, "\n")
	if err := appendSignalHistory(targetID, "연락처 010-1234-5678 넣어줘", draft); err != nil {
		t.Fatal(err)
	}
	reply := assistantReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "보고방에 보내줘")
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{"보고방에 보낼 예약 문구입니다.", "예약 문의 문구입니다.", "전화/메시지 초안:", "예약자명은 홍길동입니다", "연락처는 010-1234-5678입니다"} {
		if !strings.Contains(visible, want) {
			t.Fatalf("forward preview missing %q:\n%s", want, visible)
		}
	}
	if strings.Contains(visible, "다음에 말할 것") {
		t.Fatalf("forward preview should omit assistant follow-up hints:\n%s", visible)
	}
}

func TestSignalUserFacingReplySanitizesInternalDiagnostics(t *testing.T) {
	reply := strings.Join([]string{
		"문서를 준비했습니다.",
		"다음 액션: `검토해줘`",
		"meshclaw-attachment: /Users/example/.meshclaw/evidence/2026/report.md",
		"내부 경로: /Users/example/.meshclaw/evidence/2026/report.json",
		"evidence_id: 20260624T202448Z-312913000-signal-dispatch-health-signal",
		"exit status 1",
		"router=assistant_document",
		"raw log: {\"ok\":false}",
		"stored_at: /Users/argos/.meshclaw/evidence/2026/report.json",
		"디버그: tool stderr",
		"완료: Signal에서 확인하세요.",
	}, "\n")

	visible := signalReplyVisibleText(reply)
	for _, want := range []string{"문서를 준비했습니다.", "다음 액션: `검토해줘`", "완료: Signal에서 확인하세요."} {
		if !strings.Contains(visible, want) {
			t.Fatalf("sanitized visible reply missing %q:\n%s", want, visible)
		}
	}
	assertNoSignalUserFacingDiagnostics(t, visible)
}

func TestAssistantVisibleReplyScenarioContract(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	opts := ListenOptions{TargetID: "argos-assistant-contract", Mode: "assistant"}
	scenarios := []struct {
		name        string
		reply       func() string
		want        []string
		attachments bool
	}{
		{
			name: "capabilities",
			reply: func() string {
				return assistantCapabilitiesReply()
			},
			want: []string{"저는 Argos", "바로 해볼 예시", "제약회사 최신 뉴스", "DART/SEC", "승인 필요"},
		},
		{
			name: "secret",
			reply: func() string {
				return assistantReply(opts, "비밀번호는 super-secret-value 야")
			},
			want: []string{"민감값 원문은 답장에 반복하지 않고"},
		},
		{
			name: "destructive_ops",
			reply: func() string {
				return assistantReply(opts, "서버 재시작하고 배포 적용해")
			},
			want: []string{"바로 실행하지 않습니다", "Ops 방", "안전한 실행 계획"},
		},
	}

	for i := 0; i < 10; i++ {
		title := fmt.Sprintf("Signal 문서 계약 %02d", i+1)
		body := fmt.Sprintf("# 핵심\n- Signal 본문 우선 계약 %02d\n# 다음 액션\n- 첨부를 열지 않아도 이해 가능", i+1)
		scenarios = append(scenarios, struct {
			name        string
			reply       func() string
			want        []string
			attachments bool
		}{
			name: "document_" + title,
			reply: func() string {
				return executeAssistantToolCall(opts, "문서 만들어줘", "create_document", fmt.Sprintf(`{"title":%q,"body":%q}`, title, body))
			},
			want:        []string{"문서를 작성했습니다.", "Signal에서 바로 읽기:", "흐름도: 요청 --> 작성 --> Obsidian 저장 --> Signal 보고", "Signal 본문 우선 계약"},
			attachments: true,
		})
	}

	for i := 0; i < 8; i++ {
		title := fmt.Sprintf("발표 계약 %02d", i+1)
		scenarios = append(scenarios, struct {
			name        string
			reply       func() string
			want        []string
			attachments bool
		}{
			name: "presentation_" + title,
			reply: func() string {
				return executeAssistantToolCall(opts, "발표자료 만들어줘", "create_presentation", fmt.Sprintf(`{"title":%q,"body":"# 목표\n- Signal 보고\n# 결정\n- 테스트 유지","slide_count":4}`, title))
			},
			want:        []string{"발표자료를 작성하고 검증했습니다.", "PowerPoint에서 바로 열 수 있는 PPTX", "Obsidian에서 다듬을 수 있는 발표 outline"},
			attachments: true,
		})
	}

	for i := 0; i < 8; i++ {
		title := fmt.Sprintf("예산표 계약 %02d", i+1)
		scenarios = append(scenarios, struct {
			name        string
			reply       func() string
			want        []string
			attachments bool
		}{
			name: "spreadsheet_" + title,
			reply: func() string {
				return executeAssistantToolCall(opts, "예산표 만들어줘", "create_spreadsheet", fmt.Sprintf(`{"title":%q,"body":"| 구분 | 예산 | 실사용 |\n| --- | --- | --- |\n| 서버 | 100000 | 80000 |"}`, title))
			},
			want:        []string{"표 파일을 작성했습니다.", "Numbers나 Excel에서 바로 열 수 있는 XLSX", "CSV 원본도 함께 보냅니다."},
			attachments: true,
		})
	}

	for i := 0; i < 6; i++ {
		title := fmt.Sprintf("회의 자료 계약 %02d", i+1)
		scenarios = append(scenarios, struct {
			name        string
			reply       func() string
			want        []string
			attachments bool
		}{
			name: "meeting_materials_" + title,
			reply: func() string {
				return executeAssistantToolCall(opts, "회의 자료 패키지 만들어줘", "prepare_meeting_materials", fmt.Sprintf(`{"title":%q,"body":"진행 상황과 다음 액션을 공유한다.","slide_count":4}`, title))
			},
			want:        []string{"회의 자료 패키지를 준비했습니다.", "회의 브리프는 Word/Pages 문서로 만들었습니다.", "발표자료는 PPTX로 만들었습니다."},
			attachments: true,
		})
	}

	for i := 0; i < 6; i++ {
		from := fmt.Sprintf("출발지%d", i+1)
		to := fmt.Sprintf("도착지%d", i+1)
		scenarios = append(scenarios, struct {
			name        string
			reply       func() string
			want        []string
			attachments bool
		}{
			name: "directions_" + to,
			reply: func() string {
				return executeAssistantToolCall(opts, "길찾기 알려줘", "get_directions", fmt.Sprintf(`{"from":%q,"to":%q}`, from, to))
			},
			want: []string{"길찾기 링크입니다.", "https://www.google.com/maps/dir/", "iPhone에서 열면 Google Maps"},
		})
	}

	for i := 0; i < 6; i++ {
		query := fmt.Sprintf("장소 후보 %02d", i+1)
		scenarios = append(scenarios, struct {
			name        string
			reply       func() string
			want        []string
			attachments bool
		}{
			name: "place_" + query,
			reply: func() string {
				return executeAssistantToolCall(opts, "지도 보여줘", "find_place", fmt.Sprintf(`{"query":%q}`, query))
			},
			want: []string{"지도 검색 링크입니다.", "https://www.google.com/maps/search/", "지도에서 후보를 고르면"},
		})
	}

	for i := 0; i < 5; i++ {
		targetID := fmt.Sprintf("argos-assistant-vague-mail-%02d", i+1)
		scenarios = append(scenarios, struct {
			name        string
			reply       func() string
			want        []string
			attachments bool
		}{
			name: fmt.Sprintf("vague_mail_%02d", i+1),
			reply: func() string {
				reply, _ := assistantInteractiveReply(ListenOptions{TargetID: targetID, Mode: "assistant"}, "메일 보내줘")
				return reply
			},
			want: []string{"보낼 계정", "받는 사람", "제목", "본문", "승인"},
		})
	}

	if len(scenarios) < 45 {
		t.Fatalf("visible reply scenario coverage too small: got %d, want at least 45", len(scenarios))
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			reply := scenario.reply()
			visible := signalReplyVisibleText(reply)
			if strings.TrimSpace(visible) == "" {
				t.Fatalf("visible reply is empty; raw reply=%q", reply)
			}
			for _, want := range scenario.want {
				if !strings.Contains(visible, want) {
					t.Fatalf("visible reply missing %q:\n%s", want, visible)
				}
			}
			assertNoSignalUserFacingDiagnostics(t, visible)
			if scenario.attachments && len(signalReplyAttachments(reply)) == 0 {
				t.Fatalf("expected attachments; raw reply=%q", reply)
			}
		})
	}
}

func TestAssistantPresentationReplyAttachesMobileOpenablePPTX(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply := executeAssistantToolCall(
		ListenOptions{TargetID: "argos-assistant-pptx-mobile", Mode: "assistant"},
		"내일 회의용 발표자료를 PPT로 만들어서 모바일에서 열 수 있게 보내줘",
		"create_presentation",
		`{"title":"Signal 모바일 PPTX 테스트","body":"# 목적\n- Signal 모바일에서 PPTX 첨부를 탭해 바로 연다\n# 내용\n- 회의 목적\n- 핵심 메시지\n- 다음 액션","slide_count":4}`,
	)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{"발표자료를 작성하고 검증했습니다.", "PowerPoint에서 바로 열 수 있는 PPTX", "iPhone Signal에서 PPTX 첨부를 탭"} {
		if !strings.Contains(visible, want) {
			t.Fatalf("visible reply missing %q:\n%s", want, visible)
		}
	}
	for _, forbidden := range []string{"meshclaw-attachment:", "/Documents/Argos Vault/", "/.meshclaw/evidence/"} {
		if strings.Contains(visible, forbidden) {
			t.Fatalf("visible reply leaked %q:\n%s", forbidden, visible)
		}
	}
	assertNoSignalUserFacingDiagnostics(t, visible)
	attachments := signalReplyAttachments(reply)
	if len(attachments) == 0 {
		t.Fatalf("expected Signal attachments; raw reply=%q", reply)
	}
	if !containsAttachmentExt(attachments, ".pptx") {
		t.Fatalf("expected mobile-openable PPTX attachment, got %#v; raw reply=%q", attachments, reply)
	}
	if containsAttachmentExt(attachments, ".html") || containsAttachmentExt(attachments, ".htm") {
		t.Fatalf("raw HTML should not be attached by default, got %#v; raw reply=%q", attachments, reply)
	}
}

func containsAttachmentExt(paths []string, ext string) bool {
	ext = strings.ToLower(strings.TrimSpace(ext))
	for _, path := range paths {
		if strings.ToLower(filepath.Ext(strings.TrimSpace(path))) == ext {
			return true
		}
	}
	return false
}

func assertNoSignalUserFacingDiagnostics(t *testing.T, visible string) {
	t.Helper()
	forbidden := append([]string{
		"meshclaw-attachment:",
		"/Documents/Argos Vault/",
		"/.meshclaw/evidence/",
		"evidence: /",
		"evidence_id",
		"stored_at",
		"store_error",
		"exit status",
		"raw log",
		"debug:",
		"router",
		"증거 ID",
		"진행 증거:",
		"원시 로그",
		"디버그:",
		"라우터",
	}, assistantCommercialForbiddenVisibleMarkers()...)
	checked := map[string]bool{}
	lowerVisible := strings.ToLower(visible)
	for _, marker := range forbidden {
		marker = strings.TrimSpace(marker)
		if marker == "" || checked[marker] {
			continue
		}
		checked[marker] = true
		if strings.Contains(lowerVisible, strings.ToLower(marker)) {
			t.Fatalf("visible reply leaked %q:\n%s", marker, visible)
		}
	}
}
