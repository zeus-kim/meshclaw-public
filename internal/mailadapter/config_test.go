package mailadapter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListAccountsRedactsPasswordSources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mail-accounts.json")
	t.Setenv("MESHCLAW_MAIL_ACCOUNTS", path)
	data := []byte(`{
  "accounts": [
    {
      "id": "personal",
      "backend": "imap",
      "email": "operator@example.com",
      "host": "imap.example.com",
      "port": 993,
      "username": "operator@example.com",
      "password_handle": "vault://meshclaw/mail/app-password",
      "mailbox": "INBOX",
      "tls": true
    }
  ]
}`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := ListAccounts()
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Accounts) != 1 {
		t.Fatalf("accounts=%#v", store.Accounts)
	}
	account := store.Accounts[0]
	if !account.PasswordConfigured || account.PasswordHandle == "" {
		t.Fatalf("account=%#v", account)
	}
}

func TestSaveDraftUsesDraftDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MESHCLAW_MAIL_DRAFT_DIR", dir)
	draft, err := SaveDraft(Draft{Account: "personal", Subject: "Re: hello", Body: "draft"})
	if err != nil {
		t.Fatal(err)
	}
	if draft.ID == "" || draft.Path == "" {
		t.Fatalf("draft=%#v", draft)
	}
	if _, err := os.Stat(draft.Path); err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(draft.Path) != dir {
		t.Fatalf("draft path=%s dir=%s", draft.Path, dir)
	}
}

func TestSummarizeBodyRedactsSecrets(t *testing.T) {
	fakeToken := "ghp_" + "abcdefghijklmnopqrstuvwxyz1234567890"
	text, changed := summarizeBody("token "+fakeToken, 200)
	if !changed {
		t.Fatal("expected redaction")
	}
	if text == "" || text == "token "+fakeToken {
		t.Fatalf("text=%q", text)
	}
}
