package osauto

import "testing"

func TestArgosMassScenarioClassification(t *testing.T) {
	groups := []struct {
		action string
		inputs []string
	}{
		{"help", []string{
			"뭘 할 수 있어?", "뭐 할 수 있지?", "무엇을 할 수 있는지 알려줘", "할 수 있는 일 알려줘",
			"가능한 일 목록 보여줘", "기능 알려줘", "기능 설명해줘", "사용법 알려줘",
			"도움말", "help", "what can you do", "what can i do",
		}},
		{"open_app", []string{
			"Safari 열어줘", "사파리 켜줘", "Chrome 열어줘", "크롬 실행해줘",
			"Notes 열어줘", "노트 앱 열어줘", "Finder 열어줘", "파인더 켜줘",
			"Mail 열어줘", "캘린더 열어줘", "터미널 열어줘", "계산기 앱 열어줘",
			"Preview 열어줘", "사진 앱 열어줘", "음악 앱 열어줘", "Signal 열어줘",
			"Claude 열어줘", "Codex 실행해줘", "시스템 설정 열어줘", "TextEdit 열어줘",
		}},
		{"browser_search", []string{
			"가나 경제 뉴스 검색해줘", "가나 경제 뉴스 찾아봐", "이순신 자료 조사해줘", "서울 날씨 웹에서 찾아줘",
			"Argos MCP 검색", "search web for Ghana economy news", "search for Apple Migration Assistant",
			"인터넷에서 세종대왕 자료 찾아줘", "구글에서 한글 창제 자료 검색해줘", "서버 비용 절감 방법 조사해줘",
			"AI Studio MCP 문서 검색해줘", "OpenAI API 가격 검색해줘", "강남역 맛집 찾아줘", "쿠팡 러닝 벨트 찾아줘",
			"메일 백업 방법 조사해줘",
		}},
		{"visible_browser_search", []string{
			"브라우저에서 가나 경제 뉴스 검색해줘", "브라우저로 세종대왕 자료 검색해줘", "브라우저에서 OpenAI 뉴스 검색해줘",
			"사파리에서 Apple 지원 문서 검색해줘", "화면에 띄워서 지도 검색해줘", "열어서 검색해줘 Argos 문서",
			"보이게 브라우저 검색해줘 쿠팡 러닝 벨트", "visible search Ghana economy news",
			"in browser search for Apple Migration Assistant", "visible search Argos MCP", "브라우저에서 서울 날씨 찾아줘",
			"브라우저로 회의 자료 예시 찾아줘",
		}},
		{"browser_fetch", []string{
			"https://example.com 읽고 요약해줘", "https://openai.com 확인해줘", "example.com 요약해줘",
			"openai.com 읽어줘", "https://example.com check", "https://example.com summarize",
			"developer.apple.com 확인해줘", "https://github.com 읽어줘", "https://docs.example.com 요약",
			"example.org check please",
		}},
		{"open_url", []string{
			"https://example.com 열어줘", "openai.com 열어", "https://chatgpt.com", "github.com 열어줘",
			"https://claude.ai/new 열어줘", "example.com 열어줘", "https://apple.com", "example.org",
			"https://calendar.google.com 열어줘", "https://mail.google.com 열어줘",
		}},
		{"document_create", []string{
			"문서 작성해줘. 제목은 테스트. 내용은 본문.", "보고서 만들어줘. 제목은 주간 보고. 내용은 진행 상황.",
			"회의록 작성해줘. 제목은 월요 회의. 내용은 결정사항.", "초안 만들어줘. 제목은 제안서. 내용은 첫 버전.",
			"Pages 문서 작성해줘. 제목은 Argos. 내용은 테스트.", "문서 저장해줘. 내용은 Signal 보고.",
			"회의 자료 문서 만들어줘", "보고서 초안 작성해줘", "document create 문서 작성해줘 title: Argos body: runtime summary",
			"report 문서 만들어줘 title: Status body: all systems healthy", "내일 회의 문서 만들어줘", "브리핑 문서 작성해줘",
			"문서명은 운영 보고서 내용은 dispatcher healthy 로 저장해줘", "자료 초안 만들어줘", "보고서 저장해줘",
		}},
		{"mac_runner_doctor", []string{
			"맥 실행기 상태 한번 봐줘", "맥 실행기 점검해줘", "등록된 맥 상태 체크", "mac runner doctor",
			"mac runner check", "device runner status", "맥러너 준비 상태", "등록된 맥 실행기 진단",
			"mac-runner ready check", "device-runner doctor",
		}},
		{"mac_runner_command", []string{
			"아이맥에서 Pages 문서 작성해줘", "맥북에서 Safari 열어줘", "맥에서 문서 작성해줘",
			"등록된 맥에서 Keynote 실행", "macbook run Pages", "imac execute browser search",
			"device runner run Numbers", "mac runner execute report", "맥으로 앱 열어줘", "등록된 mac에서 처리해줘",
		}},
		{"note_create", []string{
			"Notes에 메모해줘. 제목은 장보기. 내용은 우유와 커피.", "메모해줘 내용은 회의 준비",
			"노트에 기록해줘 내용은 Argos 테스트", "메모 저장해줘. 내용은 오늘 제품 방향 결정.",
			"note write body: buy milk", "create note body: hello", "메모 작성해줘 내용은 배포 확인",
			"노트 만들어줘 내용은 Signal 본문 우선", "메모 적어줘 내용은 테스트", "Notes에 기록해줘 내용은 회고",
			"메모 제목은 점검 내용은 dispatcher healthy", "메모해 내용은 일정 확인",
		}},
		{"contacts_search", []string{
			"연락처에서 홍길동 전화번호 찾아줘", "주소록에서 김철수 이메일 찾아줘", "contacts search Alice",
			"contact find Bob phone", "연락처 홍길동 조회", "주소록에서 박영희 찾아줘", "연락처에서 odt 검색",
			"contacts lookup operator email", "연락처에서 010 검색해줘", "주소록에서 회사 이름 찾아줘",
			"contact phone Jane", "연락처 이메일 zeus 찾아줘",
		}},
		{"calendar_events_list", []string{
			"내일 일정 뭐 있어?", "오늘 일정 확인해줘", "이번 주 캘린더 보여줘", "calendar list tomorrow",
			"schedule show today", "agenda check", "모레 일정 알려줘", "이번주 일정 조회",
			"캘린더에 뭐 있어?", "오늘 캘린더 보여줘", "내일 일정 확인", "일정 목록 보여줘",
			"calendar what today", "schedule check week", "내 일정 알려줘",
		}},
		{"calendar_event_create", []string{
			"내일 오후 3시에 Argos 회의 일정 추가해줘", "오늘 6시에 운동 약속 등록해줘",
			"모레 오전 10시에 병원 일정 만들어줘", "내일 9시에 meeting add", "내일 오후 2시에 캘린더에 회의 추가",
			"오늘 밤 8시에 약속 잡아줘", "내일 오전 11시에 일정 등록", "모레 3시에 회의 만들기",
			"내일 7시에 저녁 약속 추가", "오늘 5시에 calendar event create",
			"내일 오후 4시에 'Argos 테스트' 일정 추가해줘", "내일 10시에 미팅 일정 잡아줘",
		}},
		{"calendar_event_delete", []string{
			"내일 Argos 회의 일정 삭제해줘", "오늘 운동 약속 취소해줘", "모레 병원 일정 지워줘",
			"calendar event delete Argos", "meeting cancel tomorrow", "내일 Argos 회의 일정 삭제",
			"일정에서 테스트 지워줘", "캘린더 Argos 삭제", "Argos 약속 취소해줘", "내일 테스트 회의 일정 삭제해줘",
		}},
		{"reminders_list", []string{
			"오늘 할 일 뭐 있어?", "내일 할일 보여줘", "이번 주 리마인더 확인해줘", "todo list today",
			"reminders show tomorrow", "리마인더 뭐 있어?", "알림 목록 보여줘", "오늘 할 일 알려줘",
			"모레 todo check", "이번주 할일 조회", "reminder list", "할 일 확인",
			"오늘 알림 뭐 있어?", "내일 리마인더 보여줘", "todo what today",
		}},
		{"reminder_create", []string{
			"내일 오전 9시에 우유 사기 리마인더 추가해줘", "20분 뒤에 알려줘", "오늘 6시에 운동 알림 만들어줘",
			"내일 8시에 약 챙기기 할 일 등록", "remind me in 20 minutes", "todo add buy milk",
			"내일 오후 2시에 전화하기 알려줘", "모레 10시에 병원 예약 리마인더 만들기",
			"오늘 밤 9시에 쓰레기 버리기 알림 만들어줘", "내일 7시에 산책 할일 추가",
			"reminder create call mom", "내일 오전 11시에 회의 준비 리마인더 기억해줘",
		}},
		{"reminder_complete", []string{
			"우유 사기 리마인더 완료해줘", "운동 할 일 완료", "todo buy milk done", "reminder call mom complete",
			"약 챙기기 알림 끝냈어", "회의 준비 리마인더 완료", "할 일 산책 끝내줘", "쓰레기 버리기 todo done",
		}},
		{"reminder_delete", []string{
			"우유 사기 리마인더 삭제해줘", "운동 할 일 지워줘", "todo buy milk delete", "reminder call mom remove",
			"약 챙기기 알림 삭제", "장보기 할 일 지워", "할 일 산책 삭제해줘", "쓰레기 버리기 todo remove",
		}},
		{"shortcut_run", []string{
			"Argos Morning 단축어 실행", "Argos Evening 단축어 실행해줘", "shortcut Argos Morning run",
			"단축어 Argos Health 실행", "Focus Mode 단축어 켜줘", "Water Log 단축어 실행",
			"Argos Briefing 단축어 실행해", "shortcut Daily Report run",
		}},
		{"ai_handoff", []string{
			"클로드로 넘겨줘: 이 에러 원인 분석", "claude ask why build failed", "코덱스에게 질문: 테스트 고쳐줘",
			"codex handoff refactor plan", "지피티에게 물어봐: 가나 경제 뉴스 요약", "chatgpt ask summarize this",
			"챗지피티 질문: 여행 일정", "Claude에게 프롬프트: 코드 리뷰", "코덱스로 넘겨줘: 문서 정리", "chatgpt handoff draft reply",
		}},
		{"clipboard_set", []string{
			"클립보드에 hello 복사", "클립보드에 회의 링크 복사해줘", "clipboard copy hello",
			"이 문장 복사", "클립보드 복사 테스트", "복사해 Argos", "clipboard Signal summary", "텍스트 복사해줘",
		}},
		{"work_demo", []string{
			"작업 데모 보여줘. 검색어는 가나 경제 뉴스", "작업 데모 보여줘", "검색 작업 데모 주제는 AI Studio",
			"브라우저 데모 검색어는 MeshClaw", "work demo search query: Ghana economy news", "demo search about Argos",
			"문서 저장 데모 검색어는 OpenAI", "파일 작업 데모 topic: deployment", "시연 검색어는 Signal", "작업 시연 검색어는 회의 자료",
		}},
	}

	total := 0
	for _, group := range groups {
		for _, input := range group.inputs {
			total++
			t.Run(group.action+"/"+input, func(t *testing.T) {
				action := ClassifyArgosAction(input)
				if action.Action != group.action {
					t.Fatalf("action=%q, want %q for input %q; error=%q next=%v", action.Action, group.action, input, action.Error, action.NextActions)
				}
			})
		}
	}
	if total < 240 {
		t.Fatalf("scenario coverage too small: got %d, want at least 240", total)
	}
}

func TestArgosMassScenarioVagueContactsRejected(t *testing.T) {
	for _, input := range []string{
		"연락처에서 아무나 한 명 전화번호 찾아줘",
		"주소록에서 아무 사람 찾아줘",
		"contacts find anyone",
		"연락처에서 아무나 알려줘",
	} {
		t.Run(input, func(t *testing.T) {
			action := ClassifyArgosAction(input)
			if action.Action != "none" || action.Error == "" {
				t.Fatalf("vague contacts should be rejected, got action=%q error=%q", action.Action, action.Error)
			}
		})
	}
}
