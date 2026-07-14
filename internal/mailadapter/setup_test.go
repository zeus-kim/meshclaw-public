package mailadapter

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupGmailDryRunUsesProviderPreset(t *testing.T) {
	t.Setenv("MESHCLAW_MAIL_ACCOUNTS", filepath.Join(t.TempDir(), "mail-accounts.json"))
	result, err := Setup(SetupOptions{Email: "operator@gmail.com"})
	if err != nil {
		t.Fatal(err)
	}
	if result.SetupMode != "provider" || result.Mode != "dry-run" {
		t.Fatalf("result=%#v", result)
	}
	if result.Account == nil || result.Account.Host != "imap.gmail.com" || result.Account.Username != "operator@gmail.com" {
		t.Fatalf("account=%#v", result.Account)
	}
	if result.Saved {
		t.Fatal("dry run should not save")
	}
}

func TestSetupKeychainRequiresReference(t *testing.T) {
	t.Setenv("MESHCLAW_MAIL_ACCOUNTS", filepath.Join(t.TempDir(), "mail-accounts.json"))
	result, err := Setup(SetupOptions{Email: "operator@example.com", Mode: "keychain"})
	if err == nil {
		t.Fatal("expected keychain reference error")
	}
	if result.SetupMode != "keychain" || len(result.Problems) == 0 {
		t.Fatalf("result=%#v", result)
	}
}

func TestSetupDirectExecuteSavesWithoutPassword(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mail-accounts.json")
	t.Setenv("MESHCLAW_MAIL_ACCOUNTS", path)
	result, err := Setup(SetupOptions{
		Email:    "you@example.com",
		Mode:     "direct",
		Host:     "mail.example.com",
		Port:     993,
		Username: "foolsai",
		Execute:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Saved || result.Account == nil {
		t.Fatalf("result=%#v", result)
	}
	store, err := loadStore()
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Accounts) != 1 || store.Accounts[0].PasswordHandle == "" {
		t.Fatalf("store=%#v", store)
	}
}

func TestParseKeychainCandidatesFiltersMailMetadata(t *testing.T) {
	text := `keychain: "/Users/user/Library/Keychains/login.keychain-db"
    "acct"<blob>="operator@example.com"
    "svce"<blob>="Mox: operator@example.com"
keychain: "/Users/user/Library/Keychains/login.keychain-db"
    "acct"<blob>="operator@example.com"
    "svce"<blob>="cloudflare-token"
keychain: "/Users/user/Library/Keychains/login.keychain-db"
    "acct"<blob>="person@gmail.com"
    "svce"<blob>="Gmail"`
	candidates := parseKeychainCandidates(text)
	if len(candidates) != 2 {
		t.Fatalf("candidates=%#v", candidates)
	}
	if candidates[0].Account != "operator@example.com" || candidates[1].Account != "person@gmail.com" {
		t.Fatalf("candidates=%#v", candidates)
	}
}

func TestDoctorNoLoginReportsConfigured(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mail-accounts.json")
	t.Setenv("MESHCLAW_MAIL_ACCOUNTS", path)
	_, _, err := UpsertAccount(Account{
		ID:             "personal",
		Backend:        "imap",
		Email:          "person@example.com",
		Host:           "imap.example.com",
		Port:           993,
		Username:       "person@example.com",
		PasswordHandle: "vault://meshclaw/mail/person-app-password",
		Mailbox:        "INBOX",
		TLS:            true,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := Doctor(DoctorOptions{CheckLogin: false})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Accounts) != 1 || result.Accounts[0].Status != "configured" {
		t.Fatalf("result=%#v", result)
	}
}

func TestMutatingMailActionsRequireApproval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mail-accounts.json")
	t.Setenv("MESHCLAW_MAIL_ACCOUNTS", path)
	_, _, err := UpsertAccount(Account{
		ID:             "personal",
		Backend:        "imap",
		Email:          "person@example.com",
		Host:           "imap.example.com",
		Port:           993,
		Username:       "person@example.com",
		PasswordHandle: "vault://meshclaw/mail/person-app-password",
		Mailbox:        "INBOX",
		TLS:            true,
	})
	if err != nil {
		t.Fatal(err)
	}
	move, err := Move(MutateOptions{Account: "personal", IDs: []string{"1"}, Target: "Archive"})
	if err != nil {
		t.Fatal(err)
	}
	if move.Executed || move.Status != "approval_required" {
		t.Fatalf("move=%#v", move)
	}
	del, err := Delete(MutateOptions{Account: "personal", IDs: []string{"1"}})
	if err != nil {
		t.Fatal(err)
	}
	if del.Executed || del.Status != "approval_required" {
		t.Fatalf("delete=%#v", del)
	}
}

func TestSendDraftRequiresApproval(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MESHCLAW_MAIL_ACCOUNTS", filepath.Join(dir, "mail-accounts.json"))
	t.Setenv("MESHCLAW_MAIL_DRAFT_DIR", filepath.Join(dir, "drafts"))
	_, _, err := UpsertAccount(Account{
		ID:             "personal",
		Backend:        "imap",
		Email:          "person@example.com",
		Host:           "imap.example.com",
		Port:           993,
		Username:       "person@example.com",
		PasswordHandle: "vault://meshclaw/mail/person-app-password",
		Mailbox:        "INBOX",
		TLS:            true,
	})
	if err != nil {
		t.Fatal(err)
	}
	draft, err := SaveDraft(Draft{ID: "test-draft", Account: "personal", To: []string{"to@example.com"}, Subject: "Hello", Body: "Body"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := SendDraft(SendOptions{DraftID: draft.ID})
	if err != nil {
		t.Fatal(err)
	}
	if result.Executed || result.Status != "approval_required" {
		t.Fatalf("send=%#v", result)
	}
}

func TestDraftReplyBodyForListingMailIsUsable(t *testing.T) {
	body := draftReplyBody(Message{
		Summary: MessageSummary{
			From:    "Listing <no_reply@listing.co>",
			Subject: "[리스팅] 홍길동님의 관심 조건에 따른 M&A 매물을 제안 드립니다",
			Snippet: "<html><body>AI 다이어리 앱 매물입니다. 매각 금액 1.5억</body></html>",
		},
		Body: "<html><body><div>홍길동님께서 최근 관심을 보인 기업의 업종을 바탕으로 M&A 매물을 추천드립니다.</div></body></html>",
	}, "정중하게 답장 초안")
	for _, want := range []string{
		"안녕하세요.",
		"제안 주셔서 감사합니다.",
		"M&A 매물 자료는 확인했습니다.",
		"상세 소개서",
		"최근 매출/영업이익 자료",
		"홍길동 드림",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("draft body missing %q:\n%s", want, body)
		}
	}
	for _, bad := range []string{"초안입니다", "요청 의도", "<html", "<body"} {
		if strings.Contains(body, bad) {
			t.Fatalf("draft body exposed %q:\n%s", bad, body)
		}
	}
}
