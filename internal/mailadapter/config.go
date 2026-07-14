package mailadapter

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/meshclaw/meshclaw/internal/guardvault"
)

func ConfigPath() string {
	if path := strings.TrimSpace(os.Getenv("MESHCLAW_MAIL_ACCOUNTS")); path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".meshclaw", "mail-accounts.json")
	}
	return filepath.Join(home, ".meshclaw", "mail-accounts.json")
}

func DraftDir() string {
	if path := strings.TrimSpace(os.Getenv("MESHCLAW_MAIL_DRAFT_DIR")); path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".meshclaw", "mail-drafts")
	}
	return filepath.Join(home, ".meshclaw", "mail-drafts")
}

func ListAccounts() (PublicStore, error) {
	store, err := loadStore()
	if err != nil {
		return PublicStore{}, err
	}
	out := PublicStore{Kind: "meshclaw_mail_accounts", Path: store.Path, Accounts: []AccountPublic{}}
	for _, account := range store.Accounts {
		out.Accounts = append(out.Accounts, publicAccount(account))
	}
	return out, nil
}

func UpsertAccount(account Account) (PublicStore, AccountPublic, error) {
	account, err := normalizeAccount(account)
	if err != nil {
		return PublicStore{}, AccountPublic{}, err
	}
	store, err := loadStore()
	if err != nil {
		return PublicStore{}, AccountPublic{}, err
	}
	found := false
	for i := range store.Accounts {
		if strings.EqualFold(store.Accounts[i].ID, account.ID) {
			store.Accounts[i] = account
			found = true
			break
		}
	}
	if !found {
		store.Accounts = append(store.Accounts, account)
	}
	store.Kind = "meshclaw_mail_accounts"
	store.Path = ConfigPath()
	if err := writeStore(store); err != nil {
		return PublicStore{}, AccountPublic{}, err
	}
	public, err := ListAccounts()
	if err != nil {
		return PublicStore{}, AccountPublic{}, err
	}
	return public, publicAccount(account), nil
}

func writeStore(store Store) error {
	store.Kind = "meshclaw_mail_accounts"
	store.Path = ConfigPath()
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(store.Path), 0700); err != nil {
		return err
	}
	return os.WriteFile(store.Path, append(data, '\n'), 0600)
}

func FindAccount(id string) (Account, error) {
	store, err := loadStore()
	if err != nil {
		return Account{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" && len(store.Accounts) == 1 {
		return normalizeAccount(store.Accounts[0])
	}
	for _, account := range store.Accounts {
		if strings.EqualFold(account.ID, id) {
			return normalizeAccount(account)
		}
	}
	if id == "" {
		return Account{}, errors.New("mail account is required when multiple accounts are configured")
	}
	return Account{}, fmt.Errorf("mail account %q was not found", id)
}

func loadStore() (Store, error) {
	path := ConfigPath()
	store := Store{Kind: "meshclaw_mail_accounts", Path: path, Accounts: []Account{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return store, err
	}
	if err := json.Unmarshal(data, &store); err != nil {
		return store, err
	}
	store.Kind = "meshclaw_mail_accounts"
	store.Path = path
	return store, nil
}

func normalizeAccount(account Account) (Account, error) {
	account.ID = strings.TrimSpace(account.ID)
	account.Backend = strings.ToLower(strings.TrimSpace(account.Backend))
	if account.Backend == "" {
		account.Backend = "imap"
	}
	account.Email = strings.TrimSpace(account.Email)
	account.Host = strings.TrimSpace(account.Host)
	account.SMTPHost = strings.TrimSpace(account.SMTPHost)
	account.Username = strings.TrimSpace(account.Username)
	account.PasswordEnv = strings.TrimSpace(account.PasswordEnv)
	account.PasswordHandle = strings.TrimSpace(account.PasswordHandle)
	account.Mailbox = strings.TrimSpace(account.Mailbox)
	if account.Mailbox == "" {
		account.Mailbox = "INBOX"
	}
	if account.Port == 0 {
		account.Port = 993
	}
	if account.SMTPHost == "" {
		account.SMTPHost = account.Host
	}
	if account.SMTPPort == 0 {
		account.SMTPPort = 465
	}
	if !account.TLS && account.Port == 993 {
		account.TLS = true
	}
	if !account.SMTPTLS && account.SMTPPort == 465 {
		account.SMTPTLS = true
	}
	if account.ID == "" {
		return account, errors.New("mail account id is required")
	}
	if account.Backend != "imap" {
		return account, fmt.Errorf("mail account %s backend %q is not supported yet", account.ID, account.Backend)
	}
	if account.Host == "" || account.Username == "" {
		return account, fmt.Errorf("mail account %s requires host and username", account.ID)
	}
	if account.PasswordEnv == "" && account.PasswordHandle == "" {
		return account, fmt.Errorf("mail account %s requires password_env or password_handle", account.ID)
	}
	return account, nil
}

func LoadDraft(id string) (Draft, error) {
	id = sanitizeID(id)
	if id == "" {
		return Draft{}, errors.New("draft id is required")
	}
	path := filepath.Join(DraftDir(), id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Draft{}, err
	}
	var draft Draft
	if err := json.Unmarshal(data, &draft); err != nil {
		return Draft{}, err
	}
	if draft.ID == "" {
		draft.ID = id
	}
	draft.Path = path
	return draft, nil
}

func accountPassword(account Account) (string, error) {
	if account.PasswordEnv != "" {
		value := os.Getenv(account.PasswordEnv)
		if value == "" {
			return "", fmt.Errorf("mail account %s password env %s is empty", account.ID, account.PasswordEnv)
		}
		return value, nil
	}
	if account.PasswordHandle != "" {
		resolved, _, err := guardvault.ResolveEnv(map[string]string{"MESHCLAW_MAIL_PASSWORD": account.PasswordHandle})
		if err != nil {
			return "", err
		}
		value := resolved["MESHCLAW_MAIL_PASSWORD"]
		if value == "" {
			return "", fmt.Errorf("mail account %s password handle resolved empty", account.ID)
		}
		return value, nil
	}
	return "", fmt.Errorf("mail account %s has no password source", account.ID)
}

func SaveDraft(draft Draft) (Draft, error) {
	if draft.CreatedAt.IsZero() {
		draft.CreatedAt = time.Now().UTC()
	}
	if strings.TrimSpace(draft.ID) == "" {
		draft.ID = "draft-" + draft.CreatedAt.Format("20060102T150405Z")
	}
	draft.ID = sanitizeID(draft.ID)
	if draft.Status == "" {
		draft.Status = "draft"
	}
	if draft.Policy == "" {
		draft.Policy = "not sent; email_send requires approval and evidence"
	}
	if draft.ApprovalHint == "" {
		draft.ApprovalHint = "Run mail send only after an approval record exists for this draft."
	}
	dir := DraftDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return draft, err
	}
	draft.Path = filepath.Join(dir, draft.ID+".json")
	data, err := json.MarshalIndent(draft, "", "  ")
	if err != nil {
		return draft, err
	}
	return draft, os.WriteFile(draft.Path, append(data, '\n'), 0600)
}

func sanitizeID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-._")
}
