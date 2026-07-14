package messenger

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meshclaw/meshclaw/internal/osauto"
)

func setupPurchaseSignalApprovalTest(t *testing.T, targetID string) ListenOptions {
	return setupPurchaseSignalApprovalLanguageTest(t, targetID, "ko")
}

func setupPurchaseSignalApprovalLanguageTest(t *testing.T, targetID, locale string) ListenOptions {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", locale)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	t.Setenv("MESHCLAW_OPSDB", filepath.Join(home, ".meshclaw", "state"))
	t.Setenv("MESHCLAW_PENDING_ASSISTANT_CONVERSATIONS", filepath.Join(home, "pending-assistant-conversations.json"))
	t.Setenv("MESHCLAW_SHOPPING_SEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_SIGNAL_LOCAL_AUTO_GRANT", "0")

	assistantConversationPending.Lock()
	assistantConversationPending.items = map[string]pendingAssistantConversation{}
	assistantConversationPending.Unlock()
	t.Cleanup(func() {
		assistantConversationPending.Lock()
		defer assistantConversationPending.Unlock()
		assistantConversationPending.items = map[string]pendingAssistantConversation{}
	})

	return ListenOptions{TargetID: targetID, Mode: "assistant"}
}

func TestPurchaseSignalKoreanRequestStartsOneApprovalPath(t *testing.T) {
	opts := setupPurchaseSignalApprovalTest(t, "purchase-signal-one-approval-start")

	reply := signalReplyVisibleText(assistantReply(opts, "휴지 구매해"))

	for _, want := range []string{
		"구매 승인 한 번만 받겠습니다.",
		"상품: 휴지",
		"`구매 승인`, `ㅇㅇ`, `ㄱㄱ`, 또는 `ㅇㅋ`",
		"최종 주문 클릭",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("purchase prep reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{
		"google.com/search",
		"https://www.google.com/search?tbm=shop",
		"Mac 작업 실행 권한",
		"작업: open_url",
		"상위 3개 후보 비교표",
		"검색 후에는 리뷰/배송비/총액을 비교",
		"구매 완료",
		"주문 완료",
	} {
		if strings.Contains(reply, bad) {
			t.Fatalf("purchase prep reply should stay on one-approval path, found %q:\n%s", bad, reply)
		}
	}
	if got := nonEmptyLineCount(reply); got > 4 {
		t.Fatalf("purchase prep reply should stay concise, got %d non-empty lines:\n%s", got, reply)
	}

	pending, ok := takePendingAssistantConversation(assistantPendingKey(opts.TargetID), false)
	if !ok {
		t.Fatal("purchase prep did not store a pending purchase context")
	}
	if pending.Kind != "shopping_direct_purchase" || pending.Intent.Query != "휴지" || pending.Intent.Reason != directPurchasePendingReasonOneApproval {
		t.Fatalf("pending purchase context = %#v", pending)
	}
}

func TestPurchaseSignalEnglishRequestStartsOneApprovalPathWithoutKoreanLeakage(t *testing.T) {
	opts := setupPurchaseSignalApprovalLanguageTest(t, "purchase-signal-one-approval-start-en", "en")

	reply := signalReplyVisibleText(assistantReply(opts, "Order me toilet paper on Coupang"))

	for _, want := range []string{
		"I will ask for purchase approval once.",
		"Item: toilet paper",
		"Reply `purchase approved`, `yes`, `go`, or `ok`",
		"final order click",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English purchase prep reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{
		"google.com/search",
		"https://www.google.com/search?tbm=shop",
		"Live purchase request workflow.",
		"Started Coupang candidate comparison.",
		"purchase complete",
		"order complete",
	} {
		if strings.Contains(reply, bad) {
			t.Fatalf("English purchase prep reply should stay on one-approval path, found %q:\n%s", bad, reply)
		}
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English purchase prep reply leaked Korean text:\n%s", reply)
	}

	pending, ok := takePendingAssistantConversation(assistantPendingKey(opts.TargetID), false)
	if !ok {
		t.Fatal("English purchase prep did not store a pending purchase context")
	}
	if pending.Kind != "shopping_direct_purchase" || pending.Intent.Query != "toilet paper" || pending.Intent.Reason != directPurchasePendingReasonOneApproval {
		t.Fatalf("English pending purchase context = %#v", pending)
	}
}

func TestPurchaseSignalShortApprovalUsesOnlyPendingPurchaseContext(t *testing.T) {
	opts := setupPurchaseSignalApprovalTest(t, "purchase-signal-short-approval-pending")
	start := signalReplyVisibleText(assistantReply(opts, "휴지 구매해"))
	if !strings.Contains(start, "구매 승인 한 번만 받겠습니다.") {
		t.Fatalf("purchase request did not start one-approval path:\n%s", start)
	}

	reply := signalReplyVisibleText(assistantReply(opts, "ㅇㅇ"))

	for _, want := range []string{
		"`휴지` 구매 승인을 받았습니다.",
		"현재 실행 모드가 아니어서 클릭은 하지 않았습니다.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("short approval with pending purchase missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{
		"이어서 할 일을 한 문장으로 보내세요",
		"구매 승인 한 번만 받겠습니다.",
		"google.com/search",
		"https://www.google.com/search?tbm=shop",
	} {
		if strings.Contains(reply, bad) {
			t.Fatalf("short approval should consume pending purchase context, found %q:\n%s", bad, reply)
		}
	}
	if pending, ok := takePendingAssistantConversation(assistantPendingKey(opts.TargetID), false); ok {
		t.Fatalf("pending purchase context should be consumed after approval: %#v", pending)
	}
}

func TestPurchaseSignalShortApprovalWithoutPendingContextIsBackchannel(t *testing.T) {
	opts := setupPurchaseSignalApprovalTest(t, "purchase-signal-short-approval-no-pending")

	reply := signalReplyVisibleText(assistantReply(opts, "ㅇㅇ"))

	if !strings.Contains(reply, "좋습니다.") && !strings.Contains(reply, "무엇을 도와드릴까요?") && !strings.Contains(reply, "무슨 도움이 필요하신가요?") {
		t.Fatalf("short reply without pending purchase should stay conversational:\n%s", reply)
	}
	for _, bad := range []string{
		"구매 승인을 받았습니다.",
		"구매 자동",
		"구매 승인 한 번만 받겠습니다.",
		"최종 주문 클릭",
		"구매 실행 승인",
	} {
		if strings.Contains(reply, bad) {
			t.Fatalf("short reply without pending purchase should not approve a purchase, found %q:\n%s", bad, reply)
		}
	}
	if pending, ok := takePendingAssistantConversation(assistantPendingKey(opts.TargetID), false); ok {
		t.Fatalf("short reply without pending purchase should not create purchase context: %#v", pending)
	}
}

func TestPurchaseSignalSearchResultsWithInlineCTAsStillClickFirstResult(t *testing.T) {
	ocr := strings.ToLower(strings.Join(strings.Fields(strings.Join([]string{
		"쿠팡 검색결과",
		"휴지",
		"관련도순",
		"로켓배송",
		"내일 도착",
		"12,900원",
		"리뷰 3,124",
		"장바구니 담기",
		"바로구매",
	}, "\n")), " "))

	action, ok := inferDirectPurchaseScreenAction(ocr, "")
	if !ok {
		t.Fatal("search results with inline CTA buttons should still be actionable")
	}
	if action.Kind != "search_result" {
		t.Fatalf("search results with inline CTA buttons should click first product, got %#v", action)
	}
}

func TestPurchaseSignalSearchResultsWithLayoutAndInlineCartStillClickFirstResult(t *testing.T) {
	ocr := strings.ToLower(strings.Join(strings.Fields(strings.Join([]string{
		"쿠팡",
		"검색 결과",
		"휴지 30롤",
		"관련도순",
		"로켓배송",
		"내일 도착",
		"무료배송",
		"12,900원",
		"리뷰 3,124",
		"별점 4.8",
		"와우 할인",
		"장바구니 담기",
	}, "\n")), " "))

	action, ok := inferDirectPurchaseScreenAction(ocr, "")
	if !ok {
		t.Fatal("search results with layout cues and inline cart CTA should still be actionable")
	}
	if action.Kind != "search_result" {
		t.Fatalf("search results with layout cues and inline cart CTA should click first product, got %#v", action)
	}
}

func TestPurchaseSignalProductDetailWithCartAndBuyStillClicksCartAdd(t *testing.T) {
	ocr := strings.ToLower(strings.Join(strings.Fields(strings.Join([]string{
		"쿠팡 상품 상세",
		"휴지 30롤 로켓배송",
		"내일 도착",
		"무료배송",
		"12,900원",
		"상품평 3,124",
		"장바구니 담기",
		"바로구매",
	}, "\n")), " "))

	action, ok := inferDirectPurchaseScreenAction(ocr, "")
	if !ok {
		t.Fatal("product detail with cart and buy CTA should be actionable")
	}
	if action.Kind != "product_cart_add" {
		t.Fatalf("product detail with cart and buy CTA should click cart add, got %#v", action)
	}
}

func TestPurchaseSignalOneApprovalExecutesFromReadableCoupangCartAndCheckoutText(t *testing.T) {
	opts := setupPurchaseSignalApprovalTest(t, "purchase-signal-one-approval-readable-cart")
	start := signalReplyVisibleText(assistantReply(opts, "휴지 구매해"))
	if !strings.Contains(start, "구매 승인 한 번만 받겠습니다.") {
		t.Fatalf("purchase request did not start one-approval path:\n%s", start)
	}

	clicks, openedURL := stubPurchaseSignalExecutionScreens(t, func(call int) string {
		switch call {
		case 1, 2, 3:
			return strings.Join([]string{
				"장바구니",
				"휴지 30롤",
				"수량 1개",
				"상품금액 12,900원",
				"내일 도착",
				"주문하기",
			}, "\n")
		case 4:
			return strings.Join([]string{
				"주문서",
				"휴지 30롤",
				"수량 1개",
				"총 결제금액 12,900원",
				"배송지",
				"결제수단 신용카드",
				"내일 도착",
				"결제하기",
			}, "\n")
		default:
			return strings.Join([]string{
				"주문 완료",
				"주문번호: 123456789012",
				"내일 도착 예정",
				"쿠팡 로켓배송",
			}, "\n")
		}
	})

	execOpts := opts
	execOpts.Execute = true
	reply := signalReplyVisibleText(assistantReply(execOpts, "현재 화면에서 계속 진행해"))

	if *openedURL != "" {
		t.Fatalf("readable current cart/checkout screens should not open a generic search/product URL, opened=%q\n%s", *openedURL, reply)
	}
	if *clicks < 2 {
		t.Fatalf("expected checkout and final-order clicks from readable screens, got %d:\n%s", *clicks, reply)
	}
	for _, want := range []string{
		"최종 구매 클릭을 실행했습니다.",
		"상품: 휴지 30롤",
		"총액: 12,900원",
		"도착 예정: 내일 도착",
		"주문 결과 화면에서 읽은 내용:",
		"주문번호: ********9012",
		"실행 증거:",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("readable cart/checkout execution reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{
		"이어서 할 일을 한 문장으로 보내세요",
		"구매 자동 진행이 멈췄습니다",
		"쿠팡 검색 화면을 열었습니다",
		"Final purchase click executed.",
		"Started automatic purchase execution.",
		"Item:",
		"Total:",
		"Read from the order-result screen:",
		"/.meshclaw/evidence/",
		"/Users/",
		".json",
	} {
		if strings.Contains(reply, bad) {
			t.Fatalf("Korean execution reply should not fall back or leak English copy %q:\n%s", bad, reply)
		}
	}
	if pending, ok := takePendingAssistantConversation(assistantPendingKey(opts.TargetID), false); ok {
		t.Fatalf("successful one-approval purchase should consume pending context: %#v", pending)
	}
}

func TestPurchaseSignalBlockedUnknownScreenDoesNotRepeatSameStepLine(t *testing.T) {
	opts := setupPurchaseSignalApprovalTest(t, "purchase-signal-unknown-screen-dedupe")
	start := signalReplyVisibleText(assistantReply(opts, "휴지 구매해"))
	if !strings.Contains(start, "구매 승인 한 번만 받겠습니다.") {
		t.Fatalf("purchase request did not start one-approval path:\n%s", start)
	}

	stubPurchaseSignalExecutionScreens(t, func(call int) string {
		return strings.Join([]string{
			"쿠팡",
			"휴지",
			"아직 다음 구매 버튼을 읽을 수 없는 화면",
		}, "\n")
	})

	execOpts := opts
	execOpts.Execute = true
	reply := signalReplyVisibleText(assistantReply(execOpts, "ㅇㅇ"))

	const repeated = "화면에서 다음 구매 버튼을 확정하지 못해 다시 읽었습니다."
	if got := strings.Count(reply, repeated); got != 1 {
		t.Fatalf("blocked reply should show repeated unknown-screen step once, got %d:\n%s", got, reply)
	}
	if !strings.Contains(reply, "`휴지` 구매 자동 진행이 멈췄습니다.") {
		t.Fatalf("unknown screen should still block and keep retry context:\n%s", reply)
	}
	pending, ok := takePendingAssistantConversation(assistantPendingKey(opts.TargetID), false)
	if !ok || pending.Kind != "shopping_direct_purchase" || pending.Intent.Query != "휴지" {
		t.Fatalf("unknown-screen blocked reply should keep pending purchase: %#v ok=%v", pending, ok)
	}
}

func TestPurchaseSignalBlockedPartialCheckoutReportsMissingExecutionFields(t *testing.T) {
	opts := setupPurchaseSignalApprovalTest(t, "purchase-signal-partial-checkout-missing-fields")
	start := signalReplyVisibleText(assistantReply(opts, "휴지 구매해"))
	if !strings.Contains(start, "구매 승인 한 번만 받겠습니다.") {
		t.Fatalf("purchase request did not start one-approval path:\n%s", start)
	}

	stubPurchaseSignalExecutionScreens(t, func(call int) string {
		return strings.Join([]string{
			"주문서",
			"휴지 30롤",
			"수량 1개",
			"총 결제금액 12,900원",
			"배송지",
			"내일 도착",
		}, "\n")
	})

	execOpts := opts
	execOpts.Execute = true
	reply := signalReplyVisibleText(assistantReply(execOpts, "현재 화면에서 다시 해봐"))

	for _, want := range []string{
		"`현재 화면 상품` 구매 자동 진행이 멈췄습니다.",
		"다시 채울 값:",
		"결제수단 표시",
		"최종 주문 버튼 좌표 x/y",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("partial checkout blocked reply missing %q:\n%s", want, reply)
		}
	}
	if got := strings.Count(reply, "다시 채울 값:"); got != 1 {
		t.Fatalf("partial checkout missing-field step should be deduped once, got %d:\n%s", got, reply)
	}
}

func TestPurchaseSignalBlockedPendingShortApprovalRetriesCurrentScreenWithoutSearch(t *testing.T) {
	opts := setupPurchaseSignalApprovalTest(t, "purchase-signal-blocked-short-retry-current-screen")
	rememberPendingShoppingDirectPurchaseWithURLAndBlocker(opts, "휴지 구매해", "휴지", "", directPurchaseBlockerNotReady)

	_, openedURL := stubPurchaseSignalExecutionScreens(t, func(call int) string {
		return strings.Join([]string{
			"쿠팡",
			"휴지",
			"아직 다음 구매 버튼을 읽을 수 없는 화면",
		}, "\n")
	})

	execOpts := opts
	execOpts.Execute = true
	reply := signalReplyVisibleText(assistantReply(execOpts, "ㅇㅇ"))

	if *openedURL != "" {
		t.Fatalf("blocked short retry without selected URL should not open a new search/product URL, opened=%q\n%s", *openedURL, reply)
	}
	for _, want := range []string{
		"`휴지` 구매 자동 진행이 멈췄습니다.",
		"새 검색을 열지 않고 현재 화면에서 구매 진행을 이어갑니다.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("current-screen retry reply missing %q:\n%s", want, reply)
		}
	}
	pending, ok := takePendingAssistantConversation(assistantPendingKey(opts.TargetID), false)
	if !ok || pending.Kind != "shopping_direct_purchase" || pending.Intent.Query != "휴지" {
		t.Fatalf("blocked current-screen retry should keep pending purchase: %#v ok=%v", pending, ok)
	}
}

func TestPurchaseSignalOneApprovalExecutesFromEnglishReadableFinalOrderText(t *testing.T) {
	opts := setupPurchaseSignalApprovalLanguageTest(t, "purchase-signal-one-approval-readable-final-en", "en")
	start := signalReplyVisibleText(assistantReply(opts, "Order me toilet paper on Coupang"))
	if !strings.Contains(start, "I will ask for purchase approval once.") {
		t.Fatalf("English purchase request did not start one-approval path:\n%s", start)
	}

	clicks, openedURL := stubPurchaseSignalExecutionScreens(t, func(call int) string {
		if call == 1 {
			return strings.Join([]string{
				"Toilet paper 30 rolls",
				"Quantity 1",
				"Order total KRW 12,900",
				"Shipping address",
				"Payment method card",
				"Arrives tomorrow",
				"Place order",
			}, "\n")
		}
		return strings.Join([]string{
			"Order complete",
			"Order number: CP1234567890",
			"Arrives tomorrow",
			"Carrier: Coupang Rocket delivery Tracking number: 987654321000",
		}, "\n")
	})

	execOpts := opts
	execOpts.Execute = true
	reply := signalReplyVisibleText(assistantReply(execOpts, "continue from current screen"))

	if *openedURL != "" {
		t.Fatalf("readable current final-order screen should not open a generic search/product URL, opened=%q\n%s", *openedURL, reply)
	}
	if *clicks != 1 {
		t.Fatalf("expected exactly one final-order click from readable final screen, got %d:\n%s", *clicks, reply)
	}
	for _, want := range []string{
		"Final purchase click executed.",
		"Execution result: clicked the confirmed final order-button coordinate once.",
		"Item: Toilet paper 30 rolls",
		"Total: 12,900",
		"Arrival estimate: Arrives tomorrow",
		"Read from the order-result screen:",
		"Order status: order complete",
		"Order number: ********7890",
		"Delivery/carrier text: Coupang Rocket delivery",
		"Tracking number: ********1000",
		"Execution evidence:",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English readable final-order execution reply missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "987654321000") {
		t.Fatalf("English execution reply leaked raw tracking number:\n%s", reply)
	}
	if assistantCheckoutPrepContainsHangul(reply) {
		t.Fatalf("English execution reply leaked Korean text:\n%s", reply)
	}
	for _, bad := range []string{
		"이어서 할 일을 한 문장으로 보내세요",
		"구매 자동 진행이 멈췄습니다",
		"구매 자동 진행을 시작했습니다",
		"최종 구매 클릭을 실행했습니다",
		"Opened the Coupang search screen",
		"Automatic purchase execution stopped",
		"/.meshclaw/evidence/",
		"/Users/",
		".json",
	} {
		if strings.Contains(reply, bad) {
			t.Fatalf("English execution reply should not fall back or leak Korean/search copy %q:\n%s", bad, reply)
		}
	}
	if pending, ok := takePendingAssistantConversation(assistantPendingKey(opts.TargetID), false); ok {
		t.Fatalf("successful English one-approval purchase should consume pending context: %#v", pending)
	}
}

func nonEmptyLineCount(value string) int {
	count := 0
	for _, line := range strings.Split(value, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func stubPurchaseSignalExecutionScreens(t *testing.T, ocr func(call int) string) (*int, *string) {
	t.Helper()
	oldCapture := shoppingCheckoutScreenCapture
	oldOCR := shoppingCheckoutScreenOCR
	oldOpenURL := shoppingDirectPurchaseOpenURL
	t.Cleanup(func() {
		shoppingCheckoutScreenCapture = oldCapture
		shoppingCheckoutScreenOCR = oldOCR
		shoppingDirectPurchaseOpenURL = oldOpenURL
	})

	clicks := 0
	openedURL := ""
	ocrCalls := 0
	shoppingCheckoutScreenCapture = func(ctx context.Context, output string) osauto.Result {
		if err := os.WriteFile(output, []byte("fake checkout proof"), 0600); err != nil {
			t.Fatal(err)
		}
		return osauto.Result{Kind: "meshclaw_automation_screen_capture", Action: "screen_capture", OK: true}
	}
	shoppingCheckoutScreenOCR = func(path string) (string, string) {
		ocrCalls++
		return ocr(ocrCalls), ""
	}
	shoppingDirectPurchaseOpenURL = func(ctx context.Context, rawURL string) osauto.Result {
		openedURL = rawURL
		return osauto.Result{Kind: "meshclaw_automation_open_url", Action: "open_url", URL: rawURL, OK: true}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/click" {
			t.Fatalf("unexpected runner path: %s", r.URL.Path)
		}
		clicks++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("MESHCLAW_ARGOS_UI_RUNNER_URL", server.URL)
	return &clicks, &openedURL
}
