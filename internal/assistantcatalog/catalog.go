package assistantcatalog

type Risk string

const (
	RiskRead     Risk = "read"
	RiskWrite    Risk = "write"
	RiskPrivate  Risk = "private"
	RiskExternal Risk = "external"
	RiskMoney    Risk = "money"
)

type Scenario struct {
	ID               string `json:"id"`
	Category         string `json:"category"`
	Title            string `json:"title"`
	Example          string `json:"example"`
	Risk             Risk   `json:"risk"`
	Adapter          string `json:"adapter"`
	RequiresApproval bool   `json:"requires_approval"`
	Status           string `json:"status"`
}

func Catalog() []Scenario {
	return append([]Scenario(nil), scenarios...)
}

func ByID(id string) (Scenario, bool) {
	for _, scenario := range scenarios {
		if scenario.ID == id {
			return scenario, true
		}
	}
	return Scenario{}, false
}

func ByCategory(category string) []Scenario {
	var matched []Scenario
	for _, scenario := range scenarios {
		if scenario.Category == category {
			matched = append(matched, scenario)
		}
	}
	return matched
}

func SignalExamples(category string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	var examples []string
	for _, scenario := range scenarios {
		if scenario.Category != category || scenario.Example == "" {
			continue
		}
		examples = append(examples, scenario.Example)
		if len(examples) == limit {
			break
		}
	}
	return examples
}

var scenarios = []Scenario{
	{ID: "assistant_identity", Category: "identity", Title: "Assistant identity", Example: "너는 누구야?", Risk: RiskRead, Adapter: "assistantcatalog/model", Status: "implemented"},
	{ID: "assistant_capability_menu", Category: "identity", Title: "Capability menu", Example: "뭐 할 수 있어?", Risk: RiskRead, Adapter: "assistantcatalog", Status: "implemented"},
	{ID: "assistant_help_examples", Category: "identity", Title: "Example prompts", Example: "Signal에서 바로 쓸 예문 보여줘", Risk: RiskRead, Adapter: "assistantcatalog", Status: "implemented"},
	{ID: "assistant_privacy_rules", Category: "identity", Title: "Privacy and approval rules", Example: "어떤 작업에서 내 승인이 필요해?", Risk: RiskRead, Adapter: "policy/assistantcatalog", Status: "implemented"},
	{ID: "assistant_status_summary", Category: "identity", Title: "Assistant runtime status", Example: "Argos 상태 요약해줘", Risk: RiskRead, Adapter: "meshclaw-router/assistantcatalog", Status: "partial"},
	{ID: "assistant_onboarding", Category: "identity", Title: "First-run onboarding", Example: "처음 쓰는 사람한테 사용법 알려줘", Risk: RiskRead, Adapter: "assistantcatalog/model", Status: "planned"},
	{ID: "voice_morning_brief", Category: "voice", Title: "Spoken morning brief", Example: "아침 브리핑 음성으로 읽어줘", Risk: RiskPrivate, Adapter: "assistantbrief/tts", RequiresApproval: true, Status: "partial"},
	{ID: "voice_news_brief", Category: "voice", Title: "Spoken news brief", Example: "오늘 주요뉴스 음성으로 들려줘", Risk: RiskRead, Adapter: "newsbrief/tts", RequiresApproval: true, Status: "partial"},
	{ID: "voice_weather_brief", Category: "voice", Title: "Spoken weather brief", Example: "오늘 날씨 음성으로 알려줘", Risk: RiskRead, Adapter: "weather/tts", RequiresApproval: true, Status: "partial"},
	{ID: "voice_mail_brief", Category: "voice", Title: "Spoken mail brief", Example: "중요 메일만 음성으로 요약해줘", Risk: RiskPrivate, Adapter: "mail/tts", RequiresApproval: true, Status: "planned"},
	{ID: "voice_calendar_brief", Category: "voice", Title: "Spoken calendar brief", Example: "오늘 일정 음성으로 읽어줘", Risk: RiskPrivate, Adapter: "calendar/tts", RequiresApproval: true, Status: "planned"},
	{ID: "tts_read_text", Category: "voice", Title: "Read text aloud", Example: "이 문장 읽어줘", Risk: RiskRead, Adapter: "tts", RequiresApproval: true, Status: "implemented"},
	{ID: "tts_read_document", Category: "voice", Title: "Read document aloud", Example: "이 문서 핵심만 읽어줘", Risk: RiskPrivate, Adapter: "files/model/tts", RequiresApproval: true, Status: "planned"},
	{ID: "tts_stop", Category: "voice", Title: "Stop speaking", Example: "읽는 거 멈춰", Risk: RiskWrite, Adapter: "tts", RequiresApproval: true, Status: "partial"},
	{ID: "tts_voice_select", Category: "voice", Title: "Choose voice", Example: "목소리 차분한 걸로 바꿔줘", Risk: RiskWrite, Adapter: "tts/settings", RequiresApproval: true, Status: "planned"},
	{ID: "place_search", Category: "maps", Title: "Find a place", Example: "근처 조용한 카페 찾아줘", Risk: RiskRead, Adapter: "maps/browser", Status: "partial"},
	{ID: "place_details", Category: "maps", Title: "Place details", Example: "이 식당 영업시간 알려줘", Risk: RiskRead, Adapter: "maps/browser", Status: "planned"},
	{ID: "place_map_link", Category: "maps", Title: "Send map link", Example: "위치 지도 링크로 보내줘", Risk: RiskRead, Adapter: "maps/browser", Status: "partial"},
	{ID: "directions", Category: "maps", Title: "Directions", Example: "여기서 서울역까지 가는 길 알려줘", Risk: RiskRead, Adapter: "maps/browser", Status: "partial"},
	{ID: "travel_time", Category: "maps", Title: "Travel time", Example: "강남역에서 서울역까지 지금 얼마나 걸려?", Risk: RiskRead, Adapter: "maps/browser", Status: "partial"},
	{ID: "parking_check", Category: "maps", Title: "Parking check", Example: "그 식당 주차 되는지 봐줘", Risk: RiskRead, Adapter: "maps/browser", Status: "partial"},
	{ID: "nearby_open_now", Category: "maps", Title: "Open now nearby", Example: "지금 연 약국 찾아줘", Risk: RiskRead, Adapter: "maps/browser", Status: "partial"},
	{ID: "share_location_summary", Category: "maps", Title: "Location summary", Example: "이 장소를 친구에게 보낼 문장으로 정리해줘", Risk: RiskRead, Adapter: "model", Status: "planned"},
	{ID: "save_place", Category: "maps", Title: "Save place note", Example: "이 식당 후보로 저장해줘", Risk: RiskWrite, Adapter: "notes/files", RequiresApproval: true, Status: "planned"},
	{ID: "compare_places", Category: "maps", Title: "Compare places", Example: "이 세 식당 비교해줘", Risk: RiskRead, Adapter: "browser/model", Status: "planned"},
	{ID: "reminder_create", Category: "time", Title: "Create reminder", Example: "내일 오전 9시에 우유 사기 알려줘", Risk: RiskWrite, Adapter: "reminders", RequiresApproval: true, Status: "implemented"},
	{ID: "reminder_list", Category: "time", Title: "List reminders", Example: "오늘 할 일 뭐 있어?", Risk: RiskPrivate, Adapter: "reminders", Status: "implemented"},
	{ID: "reminder_complete", Category: "time", Title: "Complete reminder", Example: "우유 사기 완료 처리해줘", Risk: RiskWrite, Adapter: "reminders", RequiresApproval: true, Status: "partial"},
	{ID: "reminder_delete", Category: "time", Title: "Delete reminder", Example: "그 리마인더 지워줘", Risk: RiskWrite, Adapter: "reminders", RequiresApproval: true, Status: "partial"},
	{ID: "calendar_create", Category: "time", Title: "Create calendar event", Example: "내일 3시에 미팅 일정 넣어줘", Risk: RiskWrite, Adapter: "calendar", RequiresApproval: true, Status: "implemented"},
	{ID: "calendar_list", Category: "time", Title: "List calendar events", Example: "내일 일정 알려줘", Risk: RiskPrivate, Adapter: "calendar", Status: "implemented"},
	{ID: "calendar_reschedule", Category: "time", Title: "Reschedule event", Example: "그 미팅 30분 뒤로 미뤄줘", Risk: RiskWrite, Adapter: "calendar", RequiresApproval: true, Status: "planned"},
	{ID: "calendar_delete", Category: "time", Title: "Delete event", Example: "내일 저녁 약속 일정 지워줘", Risk: RiskWrite, Adapter: "calendar", RequiresApproval: true, Status: "partial"},
	{ID: "timer_alarm", Category: "time", Title: "Timer or alarm", Example: "20분 뒤에 알려줘", Risk: RiskWrite, Adapter: "reminders/shortcuts", RequiresApproval: true, Status: "partial"},
	{ID: "daily_plan", Category: "time", Title: "Daily plan", Example: "오늘 일정 보고 동선 짜줘", Risk: RiskPrivate, Adapter: "calendar/maps/model", Status: "partial"},
	{ID: "contact_search", Category: "contacts", Title: "Find contact", Example: "연락처에서 김민수 전화번호 찾아줘", Risk: RiskPrivate, Adapter: "contacts", Status: "implemented"},
	{ID: "contact_share_card", Category: "contacts", Title: "Share contact card", Example: "그 연락처 공유해줘", Risk: RiskPrivate, Adapter: "contacts", RequiresApproval: true, Status: "planned"},
	{ID: "message_draft", Category: "contacts", Title: "Draft message", Example: "민수에게 늦는다고 보낼 문장 써줘", Risk: RiskPrivate, Adapter: "model", Status: "planned"},
	{ID: "message_send", Category: "contacts", Title: "Send message", Example: "민수에게 10분 늦는다고 보내줘", Risk: RiskExternal, Adapter: "messages/signal", RequiresApproval: true, Status: "planned"},
	{ID: "signal_call", Category: "contacts", Title: "Signal call", Example: "엄마에게 시그널 전화 걸어줘", Risk: RiskExternal, Adapter: "signal", RequiresApproval: true, Status: "partial"},
	{ID: "phone_call", Category: "contacts", Title: "Phone call", Example: "식당에 전화해줘", Risk: RiskExternal, Adapter: "facetime/phone", RequiresApproval: true, Status: "planned"},
	{ID: "call_script", Category: "contacts", Title: "Call script", Example: "예약 전화할 말 정리해줘", Risk: RiskRead, Adapter: "model", Status: "planned"},
	{ID: "missed_followup", Category: "contacts", Title: "Follow up missed contact", Example: "부재중 연락 확인하고 답장 초안 만들어줘", Risk: RiskPrivate, Adapter: "contacts/messages", RequiresApproval: true, Status: "planned"},
	{ID: "group_invite_text", Category: "contacts", Title: "Group invite text", Example: "저녁 모임 초대 문구 만들어줘", Risk: RiskRead, Adapter: "model", Status: "planned"},
	{ID: "birthday_greeting", Category: "contacts", Title: "Greeting draft", Example: "생일 축하 문구 써줘", Risk: RiskRead, Adapter: "model", Status: "planned"},
	{ID: "mail_accounts", Category: "mail", Title: "List mail accounts", Example: "연결된 이메일 뭐 있어?", Risk: RiskPrivate, Adapter: "mail", Status: "implemented"},
	{ID: "mail_summary", Category: "mail", Title: "Summarize mail", Example: "최근 메일 요약해줘", Risk: RiskPrivate, Adapter: "mail", Status: "implemented"},
	{ID: "mail_search", Category: "mail", Title: "Search mail", Example: "101.band 관련 메일 찾아줘", Risk: RiskPrivate, Adapter: "mail", Status: "implemented"},
	{ID: "mail_thread_read", Category: "mail", Title: "Read mail thread", Example: "이 메일 내용 읽어줘", Risk: RiskPrivate, Adapter: "mail", Status: "implemented"},
	{ID: "mail_draft_reply", Category: "mail", Title: "Draft reply", Example: "첫 번째 메일에 답장 초안 써줘", Risk: RiskPrivate, Adapter: "mail/model", Status: "implemented"},
	{ID: "mail_send", Category: "mail", Title: "Send mail", Example: "이 내용으로 이메일 보내줘", Risk: RiskExternal, Adapter: "mail", RequiresApproval: true, Status: "partial"},
	{ID: "mail_attachment_save", Category: "mail", Title: "Save attachment", Example: "첨부파일 저장해줘", Risk: RiskPrivate, Adapter: "mail/files", RequiresApproval: true, Status: "implemented"},
	{ID: "mail_move", Category: "mail", Title: "Move mail", Example: "이 메일 보관함으로 옮겨줘", Risk: RiskWrite, Adapter: "mail", RequiresApproval: true, Status: "implemented"},
	{ID: "mail_delete", Category: "mail", Title: "Delete mail", Example: "이 메일 삭제해줘", Risk: RiskWrite, Adapter: "mail", RequiresApproval: true, Status: "implemented"},
	{ID: "mail_watch", Category: "mail", Title: "Watch new mail", Example: "새 메일 확인해줘", Risk: RiskPrivate, Adapter: "mail", Status: "implemented"},
	{ID: "mail_important_brief", Category: "mail", Title: "Important mail brief", Example: "중요해 보이는 메일만 골라서 요약해줘", Risk: RiskPrivate, Adapter: "mail/model", Status: "implemented"},
	{ID: "mail_action_items", Category: "mail", Title: "Mail action items", Example: "내가 답해야 할 메일만 찾아줘", Risk: RiskPrivate, Adapter: "mail/model", Status: "partial"},
	{ID: "mail_followup_draft", Category: "mail", Title: "Follow-up draft", Example: "답장 안 한 메일에 보낼 팔로업 초안 써줘", Risk: RiskPrivate, Adapter: "mail/model", Status: "partial"},
	{ID: "mail_forward_draft", Category: "mail", Title: "Forward draft", Example: "이 메일 팀에 전달할 문장 만들어줘", Risk: RiskPrivate, Adapter: "mail/model", Status: "planned"},
	{ID: "mail_meeting_extract", Category: "mail", Title: "Extract meeting from mail", Example: "메일에서 회의 시간 찾아서 일정 후보로 만들어줘", Risk: RiskPrivate, Adapter: "mail/calendar/model", RequiresApproval: true, Status: "partial"},
	{ID: "news_fast", Category: "research", Title: "Fast news", Example: "오늘 주요뉴스 알려줘", Risk: RiskRead, Adapter: "news/cache", Status: "implemented"},
	{ID: "news_refresh", Category: "research", Title: "Refresh news", Example: "뉴스 새로 정리해줘", Risk: RiskRead, Adapter: "news/local-model", Status: "partial"},
	{ID: "news_sources", Category: "research", Title: "News sources", Example: "출처 보여줘", Risk: RiskRead, Adapter: "evidence/news", Status: "implemented"},
	{ID: "news_detail", Category: "research", Title: "News detail", Example: "3번 자세히 설명해줘", Risk: RiskRead, Adapter: "news/browser", Status: "implemented"},
	{ID: "web_search", Category: "research", Title: "Web search", Example: "가나 경제 뉴스 검색해줘", Risk: RiskRead, Adapter: "browser", Status: "implemented"},
	{ID: "web_fetch", Category: "research", Title: "Read URL", Example: "이 링크 읽고 요약해줘", Risk: RiskRead, Adapter: "browser", Status: "implemented"},
	{ID: "price_check", Category: "research", Title: "Price check", Example: "이 제품 최저가 찾아줘", Risk: RiskRead, Adapter: "browser", Status: "planned"},
	{ID: "fact_check", Category: "research", Title: "Fact check", Example: "이 말 사실인지 확인해줘", Risk: RiskRead, Adapter: "browser/model", Status: "planned"},
	{ID: "translate_page", Category: "research", Title: "Translate page", Example: "이 페이지 한국어로 정리해줘", Risk: RiskRead, Adapter: "browser/model", Status: "planned"},
	{ID: "monitor_topic", Category: "research", Title: "Monitor topic", Example: "이 주제 계속 지켜봐줘", Risk: RiskRead, Adapter: "scheduler/news", RequiresApproval: true, Status: "planned"},
	{ID: "monitor_price", Category: "research", Title: "Monitor price", Example: "이 상품 3만원 아래로 내려가면 알려줘", Risk: RiskRead, Adapter: "assistantwatch/browser", RequiresApproval: true, Status: "partial"},
	{ID: "monitor_news_topic", Category: "research", Title: "Monitor news topic", Example: "OpenClaw 관련 새 소식 나오면 알려줘", Risk: RiskRead, Adapter: "assistantwatch/news", RequiresApproval: true, Status: "planned"},
	{ID: "monitor_website_change", Category: "research", Title: "Monitor website changes", Example: "이 페이지 바뀌면 알려줘", Risk: RiskRead, Adapter: "assistantwatch/browser", RequiresApproval: true, Status: "planned"},
	{ID: "monitor_watch_list", Category: "research", Title: "List monitors", Example: "지금 감시 중인 항목 보여줘", Risk: RiskPrivate, Adapter: "assistantwatch", Status: "partial"},
	{ID: "monitor_watch_stop", Category: "research", Title: "Stop monitor", Example: "러닝화 가격 알림 중지해줘", Risk: RiskWrite, Adapter: "assistantwatch", RequiresApproval: true, Status: "partial"},
	{ID: "restaurant_reserve", Category: "booking", Title: "Restaurant reservation", Example: "내일 7시에 강남 식당 2명 예약해줘", Risk: RiskExternal, Adapter: "browser/phone/calendar", RequiresApproval: true, Status: "partial"},
	{ID: "restaurant_availability", Category: "booking", Title: "Check reservation availability", Example: "오늘 저녁 예약 가능한지 봐줘", Risk: RiskRead, Adapter: "browser/phone", Status: "partial"},
	{ID: "restaurant_cancel", Category: "booking", Title: "Cancel restaurant booking", Example: "내일 예약 취소해줘", Risk: RiskExternal, Adapter: "browser/phone", RequiresApproval: true, Status: "partial"},
	{ID: "hotel_search", Category: "booking", Title: "Hotel search", Example: "도쿄 호텔 후보 찾아줘", Risk: RiskRead, Adapter: "browser", Status: "partial"},
	{ID: "hotel_book", Category: "booking", Title: "Hotel booking", Example: "이 호텔 예약 진행해줘", Risk: RiskMoney, Adapter: "browser", RequiresApproval: true, Status: "planned"},
	{ID: "flight_search", Category: "booking", Title: "Flight search", Example: "서울 방콕 항공권 찾아줘", Risk: RiskRead, Adapter: "browser", Status: "partial"},
	{ID: "flight_book", Category: "booking", Title: "Flight booking", Example: "이 항공권 예약해줘", Risk: RiskMoney, Adapter: "browser", RequiresApproval: true, Status: "planned"},
	{ID: "ticket_search", Category: "booking", Title: "Event ticket search", Example: "콘서트 티켓 남았는지 봐줘", Risk: RiskRead, Adapter: "browser", Status: "partial"},
	{ID: "ticket_buy", Category: "booking", Title: "Buy ticket", Example: "티켓 예매해줘", Risk: RiskMoney, Adapter: "browser", RequiresApproval: true, Status: "planned"},
	{ID: "booking_calendar_add", Category: "booking", Title: "Add booking to calendar", Example: "예약되면 캘린더에 넣어줘", Risk: RiskWrite, Adapter: "calendar", RequiresApproval: true, Status: "planned"},
	{ID: "shopping_research", Category: "shopping", Title: "Research product", Example: "가벼운 맥북 가방 추천해줘", Risk: RiskRead, Adapter: "browser/model", Status: "partial"},
	{ID: "shopping_compare", Category: "shopping", Title: "Compare products", Example: "이 제품 세 개 비교해줘", Risk: RiskRead, Adapter: "browser/model", Status: "partial"},
	{ID: "shopping_cart", Category: "shopping", Title: "Add to cart", Example: "장바구니에 담아줘", Risk: RiskExternal, Adapter: "browser", RequiresApproval: true, Status: "planned"},
	{ID: "shopping_buy", Category: "shopping", Title: "Buy product", Example: "이걸로 주문해줘", Risk: RiskMoney, Adapter: "browser", RequiresApproval: true, Status: "planned"},
	{ID: "delivery_track", Category: "shopping", Title: "Track delivery", Example: "택배 어디쯤 왔어?", Risk: RiskPrivate, Adapter: "mail/browser", Status: "implemented"},
	{ID: "return_request", Category: "shopping", Title: "Request return", Example: "이 상품 반품 신청해줘", Risk: RiskExternal, Adapter: "browser/mail", RequiresApproval: true, Status: "partial"},
	{ID: "coupon_find", Category: "shopping", Title: "Find coupon", Example: "할인 코드 찾아줘", Risk: RiskRead, Adapter: "browser", Status: "partial"},
	{ID: "grocery_list", Category: "shopping", Title: "Grocery list", Example: "이번 주 장보기 목록 만들어줘", Risk: RiskWrite, Adapter: "notes/reminders", RequiresApproval: true, Status: "partial"},
	{ID: "subscription_check", Category: "shopping", Title: "Subscription check", Example: "내 구독 서비스 정리해줘", Risk: RiskPrivate, Adapter: "mail/browser", Status: "implemented"},
	{ID: "receipt_find", Category: "shopping", Title: "Find receipt", Example: "지난달 영수증 찾아줘", Risk: RiskPrivate, Adapter: "mail/files", Status: "implemented"},
	{ID: "file_find", Category: "files", Title: "Find file", Example: "어제 만든 보고서 찾아줘", Risk: RiskPrivate, Adapter: "files", Status: "planned"},
	{ID: "file_summarize", Category: "files", Title: "Summarize file", Example: "이 PDF 요약해줘", Risk: RiskPrivate, Adapter: "files/model", Status: "planned"},
	{ID: "file_convert", Category: "files", Title: "Convert file", Example: "이 문서 PDF로 바꿔줘", Risk: RiskWrite, Adapter: "files", RequiresApproval: true, Status: "planned"},
	{ID: "file_rename", Category: "files", Title: "Rename file", Example: "이 파일 이름 정리해줘", Risk: RiskWrite, Adapter: "files", RequiresApproval: true, Status: "planned"},
	{ID: "file_share_link", Category: "files", Title: "Prepare share link", Example: "이 파일 공유 링크 만들어줘", Risk: RiskExternal, Adapter: "files/cloud", RequiresApproval: true, Status: "planned"},
	{ID: "note_create", Category: "files", Title: "Create note", Example: "회의 메모로 저장해줘", Risk: RiskWrite, Adapter: "notes", RequiresApproval: true, Status: "implemented"},
	{ID: "note_search", Category: "files", Title: "Search notes", Example: "지난번 아이디어 메모 찾아줘", Risk: RiskPrivate, Adapter: "notes", Status: "planned"},
	{ID: "doc_draft", Category: "files", Title: "Draft document", Example: "한 페이지 보고서 만들어줘", Risk: RiskWrite, Adapter: "docs/model", RequiresApproval: true, Status: "planned"},
	{ID: "doc_meeting_minutes", Category: "files", Title: "Meeting minutes", Example: "회의록 문서로 만들어줘", Risk: RiskWrite, Adapter: "docs/model", RequiresApproval: true, Status: "planned"},
	{ID: "doc_proposal_draft", Category: "files", Title: "Proposal draft", Example: "제안서 초안 만들어줘", Risk: RiskWrite, Adapter: "docs/model", RequiresApproval: true, Status: "planned"},
	{ID: "ppt_meeting_pack", Category: "files", Title: "Meeting pack with deck", Example: "내일 회의자료를 요약 문서와 5장 PPT로 만들어서 보고방에 보내줘", Risk: RiskWrite, Adapter: "docs/presentation/signal", RequiresApproval: true, Status: "implemented"},
	{ID: "ppt_market_research", Category: "files", Title: "Market research deck", Example: "화학회사 민감 뉴스 분석을 내일 회의용 PPT 6장으로 만들어줘", Risk: RiskWrite, Adapter: "research/presentation/signal", RequiresApproval: true, Status: "partial"},
	{ID: "ppt_sales_brief", Category: "files", Title: "Sales briefing deck", Example: "신규 고객 미팅용으로 문제, 제안, 가격, 다음 액션이 보이는 세일즈 덱 만들어줘", Risk: RiskWrite, Adapter: "docs/presentation", RequiresApproval: true, Status: "partial"},
	{ID: "ppt_ops_review", Category: "files", Title: "Ops review deck", Example: "이번 주 서버 운영과 보안 위험을 임원 보고용 PPT로 정리해줘", Risk: RiskWrite, Adapter: "ops/presentation", RequiresApproval: true, Status: "partial"},
	{ID: "ppt_travel_plan", Category: "files", Title: "Travel plan deck", Example: "제주 여행 후보를 항공, 호텔, 일정, 예산이 보이는 PPT로 만들어줘", Risk: RiskWrite, Adapter: "booking/presentation", RequiresApproval: true, Status: "partial"},
	{ID: "ppt_mobile_resend", Category: "files", Title: "Resend deck to mobile", Example: "방금 만든 PPT를 휴대폰에서 열 수 있게 다시 보내줘", Risk: RiskWrite, Adapter: "presentation/signal", RequiresApproval: true, Status: "implemented"},
	{ID: "note_append", Category: "files", Title: "Append to note", Example: "이 내용을 프로젝트 노트에 추가해줘", Risk: RiskWrite, Adapter: "notes", RequiresApproval: true, Status: "planned"},
	{ID: "note_daily_journal", Category: "files", Title: "Daily journal note", Example: "오늘 한 일 일지로 정리해줘", Risk: RiskWrite, Adapter: "notes/model", RequiresApproval: true, Status: "planned"},
	{ID: "file_package_for_share", Category: "files", Title: "Package files for sharing", Example: "이 자료들 공유용 폴더로 묶어줘", Risk: RiskWrite, Adapter: "files", RequiresApproval: true, Status: "planned"},
	{ID: "screenshot_read", Category: "files", Title: "Read screenshot", Example: "이 캡처 이미지 내용 설명해줘", Risk: RiskPrivate, Adapter: "vision", Status: "partial"},
	{ID: "archive_cleanup", Category: "files", Title: "Archive cleanup", Example: "다운로드 폴더 정리 계획 세워줘", Risk: RiskWrite, Adapter: "files", RequiresApproval: true, Status: "planned"},
	{ID: "weather_now", Category: "daily", Title: "Current weather", Example: "지금 날씨 어때?", Risk: RiskRead, Adapter: "weather", Status: "implemented"},
	{ID: "weather_forecast", Category: "daily", Title: "Weather forecast", Example: "내일 비 와?", Risk: RiskRead, Adapter: "weather", Status: "partial"},
	{ID: "outfit_advice", Category: "daily", Title: "Outfit advice", Example: "오늘 뭐 입고 나가?", Risk: RiskRead, Adapter: "weather/model", Status: "partial"},
	{ID: "commute_brief", Category: "daily", Title: "Commute brief", Example: "출근길 상황 알려줘", Risk: RiskRead, Adapter: "maps/weather", Status: "partial"},
	{ID: "morning_brief", Category: "daily", Title: "Morning brief", Example: "아침 브리핑 해줘", Risk: RiskPrivate, Adapter: "assistantbrief", Status: "implemented"},
	{ID: "evening_brief", Category: "daily", Title: "Evening brief", Example: "저녁 정리해줘", Risk: RiskPrivate, Adapter: "assistantbrief", Status: "implemented"},
	{ID: "meal_idea", Category: "daily", Title: "Meal idea", Example: "오늘 저녁 뭐 먹지?", Risk: RiskRead, Adapter: "model/maps", Status: "partial"},
	{ID: "habit_check", Category: "daily", Title: "Habit check", Example: "오늘 운동 체크해줘", Risk: RiskPrivate, Adapter: "reminders", RequiresApproval: true, Status: "partial"},
	{ID: "packing_list", Category: "daily", Title: "Packing list", Example: "출장 짐 목록 만들어줘", Risk: RiskWrite, Adapter: "notes/reminders", RequiresApproval: true, Status: "partial"},
	{ID: "family_brief", Category: "daily", Title: "Family brief", Example: "가족 일정 정리해줘", Risk: RiskPrivate, Adapter: "calendar/contacts", Status: "planned"},
	{ID: "open_app", Category: "mac", Title: "Open app", Example: "Safari 열어줘", Risk: RiskWrite, Adapter: "argos-runner", RequiresApproval: true, Status: "implemented"},
	{ID: "open_url", Category: "mac", Title: "Open URL", Example: "chatgpt.com 열어줘", Risk: RiskWrite, Adapter: "argos-runner", RequiresApproval: true, Status: "implemented"},
	{ID: "clipboard_set", Category: "mac", Title: "Set clipboard", Example: "이 문장 복사해줘", Risk: RiskPrivate, Adapter: "clipboard", RequiresApproval: true, Status: "implemented"},
	{ID: "shortcut_run", Category: "mac", Title: "Run Shortcut", Example: "모닝 단축어 실행해줘", Risk: RiskWrite, Adapter: "shortcuts", RequiresApproval: true, Status: "implemented"},
	{ID: "screen_record", Category: "mac", Title: "Record work", Example: "작업하는 화면 녹화해줘", Risk: RiskPrivate, Adapter: "screen-recording", RequiresApproval: true, Status: "implemented"},
	{ID: "window_focus", Category: "mac", Title: "Focus window", Example: "브라우저 앞으로 가져와줘", Risk: RiskWrite, Adapter: "argos-runner", RequiresApproval: true, Status: "planned"},
	{ID: "type_text", Category: "mac", Title: "Type text", Example: "이 내용을 입력해줘", Risk: RiskWrite, Adapter: "argos-runner", RequiresApproval: true, Status: "planned"},
	{ID: "app_status", Category: "mac", Title: "App status", Example: "Signal 켜져 있어?", Risk: RiskRead, Adapter: "process", Status: "planned"},
	{ID: "permission_doctor", Category: "mac", Title: "Permission doctor", Example: "맥 권한 상태 봐줘", Risk: RiskRead, Adapter: "argos-doctor", Status: "implemented"},
	{ID: "runner_update", Category: "mac", Title: "Runner update", Example: "Argos Runner 업데이트해줘", Risk: RiskWrite, Adapter: "setup", RequiresApproval: true, Status: "partial"},
	{ID: "mac_screenshot", Category: "mac", Title: "Take screenshot", Example: "현재 화면 캡처해줘", Risk: RiskPrivate, Adapter: "argos-runner/screen", RequiresApproval: true, Status: "partial"},
	{ID: "mac_display_sleep", Category: "mac", Title: "Display sleep", Example: "화면 꺼줘", Risk: RiskWrite, Adapter: "argos-runner", RequiresApproval: true, Status: "planned"},
	{ID: "mac_volume_set", Category: "mac", Title: "Set volume", Example: "맥 볼륨 30%로 낮춰줘", Risk: RiskWrite, Adapter: "argos-runner", RequiresApproval: true, Status: "planned"},
	{ID: "mac_wifi_status", Category: "mac", Title: "Wi-Fi status", Example: "와이파이 연결 상태 봐줘", Risk: RiskRead, Adapter: "network", Status: "planned"},
	{ID: "mac_battery_status", Category: "mac", Title: "Battery status", Example: "배터리 상태 알려줘", Risk: RiskRead, Adapter: "system_profiler", Status: "planned"},
	{ID: "server_status", Category: "ops", Title: "Server status", Example: "서버 상태 알려줘", Risk: RiskRead, Adapter: "meshclaw-router", Status: "implemented"},
	{ID: "openwebui_status", Category: "ops", Title: "Open WebUI status", Example: "오픈웹유아이 연결됐어?", Risk: RiskRead, Adapter: "openwebui", Status: "implemented"},
	{ID: "service_check", Category: "ops", Title: "Service check", Example: "g4 ollama 상태 봐줘", Risk: RiskRead, Adapter: "workflow", Status: "implemented"},
	{ID: "log_check", Category: "ops", Title: "Log check", Example: "최근 에러 로그 봐줘", Risk: RiskRead, Adapter: "workflow", Status: "implemented"},
	{ID: "disk_check", Category: "ops", Title: "Disk check", Example: "디스크 용량 확인해줘", Risk: RiskRead, Adapter: "workflow", Status: "implemented"},
	{ID: "security_check", Category: "ops", Title: "Security check", Example: "보안 상태 점검해줘", Risk: RiskRead, Adapter: "workflow", Status: "implemented"},
	{ID: "evidence_list", Category: "ops", Title: "Evidence list", Example: "최근 작업 기록 보여줘", Risk: RiskRead, Adapter: "evidence", Status: "implemented"},
	{ID: "approval_request", Category: "ops", Title: "Approval request", Example: "이 작업 승인 요청 보내줘", Risk: RiskPrivate, Adapter: "policy/messenger", Status: "implemented"},
	{ID: "service_restart", Category: "ops", Title: "Restart service", Example: "nginx 재시작해줘", Risk: RiskExternal, Adapter: "workflow", RequiresApproval: true, Status: "partial"},
	{ID: "secret_store", Category: "ops", Title: "Store secret", Example: "이 토큰 저장해줘", Risk: RiskPrivate, Adapter: "guard", RequiresApproval: true, Status: "partial"},
	{ID: "ops_voice_report", Category: "ops", Title: "Spoken ops report", Example: "서버 상태 음성으로 보고해줘", Risk: RiskRead, Adapter: "workflow/tts", RequiresApproval: true, Status: "partial"},
	{ID: "ops_incident_summary", Category: "ops", Title: "Incident summary", Example: "최근 장애 징후 요약해줘", Risk: RiskRead, Adapter: "opsdb/workflow", Status: "partial"},
	{ID: "ops_security_digest", Category: "ops", Title: "Security digest", Example: "오늘 보안 이벤트 정리해줘", Risk: RiskPrivate, Adapter: "opsdb/guard", Status: "partial"},
	{ID: "ops_backup_check", Category: "ops", Title: "Backup check", Example: "백업이 제대로 됐는지 확인해줘", Risk: RiskRead, Adapter: "workflow", Status: "planned"},
	{ID: "ops_deploy_status", Category: "ops", Title: "Deploy status", Example: "최근 배포 상태 알려줘", Risk: RiskRead, Adapter: "opsdb/git", Status: "planned"},
	{ID: "ops_runbook_draft", Category: "ops", Title: "Runbook draft", Example: "이 장애 대응 절차 문서로 만들어줘", Risk: RiskWrite, Adapter: "docs/opsdb", RequiresApproval: true, Status: "planned"},
	{ID: "approval_write_file", Category: "approval", Title: "Approve file write", Example: "파일 저장 전에 나한테 확인해줘", Risk: RiskWrite, Adapter: "policy/files", RequiresApproval: true, Status: "implemented"},
	{ID: "approval_send_message", Category: "approval", Title: "Approve message send", Example: "메시지는 보내기 전에 꼭 물어봐", Risk: RiskExternal, Adapter: "policy/messenger", RequiresApproval: true, Status: "implemented"},
	{ID: "approval_purchase", Category: "approval", Title: "Approve purchase", Example: "결제는 마지막에 내 승인 받고 진행해", Risk: RiskMoney, Adapter: "policy/browser", RequiresApproval: true, Status: "implemented"},
	{ID: "approval_delete", Category: "approval", Title: "Approve destructive action", Example: "삭제 작업은 확인 받고 해", Risk: RiskWrite, Adapter: "policy", RequiresApproval: true, Status: "implemented"},
	{ID: "approval_persistent_grant", Category: "approval", Title: "Persistent narrow grant", Example: "날씨 브리핑은 매일 아침 자동으로 보내도 돼", Risk: RiskPrivate, Adapter: "policy/scheduler", RequiresApproval: true, Status: "planned"},
	{ID: "music_play", Category: "media", Title: "Play music", Example: "음악 틀어줘", Risk: RiskWrite, Adapter: "media-router", RequiresApproval: true, Status: "planned"},
	{ID: "music_choose_source", Category: "media", Title: "Choose music source", Example: "유튜브로 들을지 라디오로 들을지 골라줘", Risk: RiskRead, Adapter: "media-router", Status: "planned"},
	{ID: "youtube_music_play", Category: "media", Title: "Play YouTube music", Example: "유튜브에서 재즈 틀어줘", Risk: RiskWrite, Adapter: "browser/airplay", RequiresApproval: true, Status: "planned"},
	{ID: "radio_play", Category: "media", Title: "Play radio", Example: "라디오 틀어줘", Risk: RiskWrite, Adapter: "browser/shortcuts", RequiresApproval: true, Status: "planned"},
	{ID: "iphone_audio_handoff", Category: "media", Title: "Play on iPhone", Example: "아이폰에서 바로 틀어줘", Risk: RiskExternal, Adapter: "shortcuts/airplay", RequiresApproval: true, Status: "planned"},
	{ID: "podcast_play", Category: "media", Title: "Play podcast", Example: "AI 팟캐스트 틀어줘", Risk: RiskWrite, Adapter: "browser/podcasts", RequiresApproval: true, Status: "planned"},
	{ID: "playlist_create", Category: "media", Title: "Create playlist", Example: "집중할 때 들을 음악 목록 만들어줘", Risk: RiskWrite, Adapter: "notes/music", RequiresApproval: true, Status: "planned"},
	{ID: "video_play", Category: "media", Title: "Play video", Example: "이 유튜브 영상 틀어줘", Risk: RiskWrite, Adapter: "browser/airplay", RequiresApproval: true, Status: "planned"},
	{ID: "video_summary", Category: "media", Title: "Summarize video", Example: "이 영상 내용 요약해줘", Risk: RiskRead, Adapter: "browser/model", Status: "planned"},
	{ID: "ambient_sound", Category: "media", Title: "Ambient sound", Example: "빗소리 틀어줘", Risk: RiskWrite, Adapter: "browser/shortcuts", RequiresApproval: true, Status: "planned"},
	{ID: "ott_choose_service", Category: "media", Title: "Choose OTT service", Example: "오늘 볼만한 거 추천해줘", Risk: RiskRead, Adapter: "media-router", Status: "partial"},
	{ID: "ott_open_service", Category: "media", Title: "Open OTT service", Example: "넷플릭스 열어줘", Risk: RiskWrite, Adapter: "browser/app", RequiresApproval: true, Status: "partial"},
	{ID: "ott_recommend_today", Category: "media", Title: "Recommend what to watch", Example: "웨이브에서 오늘 볼만한 드라마 추천해줘", Risk: RiskRead, Adapter: "browser/model", Status: "partial"},
	{ID: "ott_latest_movies", Category: "media", Title: "Latest movies", Example: "애플TV 최신 영화 뭐 있어?", Risk: RiskRead, Adapter: "browser/model", Status: "partial"},
	{ID: "ott_continue_watching", Category: "media", Title: "Continue watching", Example: "보던 드라마 이어서 틀어줘", Risk: RiskPrivate, Adapter: "browser/app", RequiresApproval: true, Status: "planned"},
	{ID: "ott_subscription_manage", Category: "media", Title: "Manage OTT subscription", Example: "넷플릭스 구독 관리 열어줘", Risk: RiskMoney, Adapter: "browser/policy", RequiresApproval: true, Status: "partial"},
	{ID: "ott_billing_check", Category: "media", Title: "Check OTT billing", Example: "OTT 결제일 정리해줘", Risk: RiskPrivate, Adapter: "mail/browser", RequiresApproval: true, Status: "planned"},
	{ID: "ott_cancel_before_confirm", Category: "media", Title: "Prepare OTT cancellation", Example: "쿠팡플레이 해지 화면까지 가줘", Risk: RiskMoney, Adapter: "browser/policy", RequiresApproval: true, Status: "planned"},
	{ID: "product_browser_buy", Category: "shopping", Title: "Browser product purchase", Example: "브라우저로 이 상품 구매 진행해줘", Risk: RiskMoney, Adapter: "browser", RequiresApproval: true, Status: "planned"},
	{ID: "product_option_select", Category: "shopping", Title: "Select product options", Example: "검정색 M 사이즈로 골라줘", Risk: RiskExternal, Adapter: "browser", RequiresApproval: true, Status: "planned"},
	{ID: "product_stock_check", Category: "shopping", Title: "Check stock", Example: "이 상품 재고 있는지 봐줘", Risk: RiskRead, Adapter: "browser", Status: "partial"},
	{ID: "product_review_summary", Category: "shopping", Title: "Summarize reviews", Example: "리뷰 보고 살만한지 정리해줘", Risk: RiskRead, Adapter: "browser/model", Status: "partial"},
	{ID: "product_shipping_compare", Category: "shopping", Title: "Compare shipping", Example: "배송 빠른 곳으로 골라줘", Risk: RiskRead, Adapter: "browser", Status: "partial"},
	{ID: "product_payment_pause", Category: "shopping", Title: "Pause before payment", Example: "결제 직전까지만 해줘", Risk: RiskMoney, Adapter: "browser/policy", RequiresApproval: true, Status: "planned"},
	{ID: "product_order_confirm", Category: "shopping", Title: "Confirm order", Example: "최종 주문 전에 나한테 확인해줘", Risk: RiskMoney, Adapter: "browser/policy", RequiresApproval: true, Status: "planned"},
	{ID: "product_price_alert", Category: "shopping", Title: "Price alert", Example: "가격 내려가면 알려줘", Risk: RiskRead, Adapter: "scheduler/browser", RequiresApproval: true, Status: "partial"},
	{ID: "product_wishlist", Category: "shopping", Title: "Wishlist item", Example: "이 상품 찜 목록에 넣어줘", Risk: RiskExternal, Adapter: "browser/notes", RequiresApproval: true, Status: "planned"},
	{ID: "product_receipt_archive", Category: "shopping", Title: "Archive receipt", Example: "구매 후 영수증 저장해줘", Risk: RiskPrivate, Adapter: "mail/files", RequiresApproval: true, Status: "planned"},
}
