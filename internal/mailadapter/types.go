package mailadapter

import (
	"strings"
	"time"

	"github.com/meshclaw/meshclaw/internal/guardvault"
)

type Account struct {
	ID             string `json:"id"`
	Backend        string `json:"backend"`
	Email          string `json:"email,omitempty"`
	Host           string `json:"host,omitempty"`
	Port           int    `json:"port,omitempty"`
	SMTPHost       string `json:"smtp_host,omitempty"`
	SMTPPort       int    `json:"smtp_port,omitempty"`
	Username       string `json:"username,omitempty"`
	PasswordEnv    string `json:"password_env,omitempty"`
	PasswordHandle string `json:"password_handle,omitempty"`
	Mailbox        string `json:"mailbox,omitempty"`
	TLS            bool   `json:"tls"`
	SMTPTLS        bool   `json:"smtp_tls,omitempty"`
}

type AccountPublic struct {
	ID                  string `json:"id"`
	Backend             string `json:"backend"`
	Email               string `json:"email,omitempty"`
	Host                string `json:"host,omitempty"`
	Port                int    `json:"port,omitempty"`
	SMTPHost            string `json:"smtp_host,omitempty"`
	SMTPPort            int    `json:"smtp_port,omitempty"`
	Username            string `json:"username,omitempty"`
	Mailbox             string `json:"mailbox,omitempty"`
	TLS                 bool   `json:"tls"`
	SMTPTLS             bool   `json:"smtp_tls,omitempty"`
	PasswordConfigured  bool   `json:"password_configured"`
	PasswordHandle      string `json:"password_handle,omitempty"`
	PasswordEnvironment string `json:"password_env,omitempty"`
}

type SetupOptions struct {
	Email    string
	Mode     string
	Host     string
	Port     int
	Username string
	Service  string
	Account  string
	Execute  bool
}

type KeychainCandidate struct {
	Service string `json:"service"`
	Account string `json:"account"`
	Reason  string `json:"reason,omitempty"`
}

type LoginCandidate struct {
	Host            string `json:"host"`
	Port            int    `json:"port"`
	Username        string `json:"username"`
	KeychainService string `json:"keychain_service,omitempty"`
	KeychainAccount string `json:"keychain_account,omitempty"`
	Status          string `json:"status"`
	Error           string `json:"error,omitempty"`
}

type SetupResult struct {
	Kind        string              `json:"kind"`
	Email       string              `json:"email"`
	AccountID   string              `json:"account_id"`
	Mode        string              `json:"mode"`
	SetupMode   string              `json:"setup_mode"`
	MXHosts     []string            `json:"mx_hosts,omitempty"`
	IMAPHosts   []string            `json:"imap_hosts,omitempty"`
	Usernames   []string            `json:"usernames,omitempty"`
	Keychain    []KeychainCandidate `json:"keychain_candidates,omitempty"`
	Attempts    []LoginCandidate    `json:"attempts,omitempty"`
	Saved       bool                `json:"saved"`
	Account     *AccountPublic      `json:"account,omitempty"`
	ConfigPath  string              `json:"config_path"`
	Problems    []string            `json:"problems,omitempty"`
	NextActions []string            `json:"next_actions,omitempty"`
	GeneratedAt time.Time           `json:"generated_at"`
}

type DoctorOptions struct {
	Account    string
	CheckLogin bool
}

type AccountStatus struct {
	Account    AccountPublic `json:"account"`
	Status     string        `json:"status"`
	ConfigOK   bool          `json:"config_ok"`
	NetworkOK  bool          `json:"network_ok"`
	LoginOK    bool          `json:"login_ok"`
	MailboxOK  bool          `json:"mailbox_ok"`
	Backend    string        `json:"backend,omitempty"`
	Error      string        `json:"error,omitempty"`
	NextAction string        `json:"next_action,omitempty"`
}

type DoctorResult struct {
	Kind               string                     `json:"kind"`
	ConfigPath         string                     `json:"config_path"`
	Accounts           []AccountStatus            `json:"accounts"`
	KeychainCandidates []KeychainCandidate        `json:"keychain_candidates,omitempty"`
	Backends           []guardvault.BackendStatus `json:"backends,omitempty"`
	Problems           []string                   `json:"problems,omitempty"`
	NextActions        []string                   `json:"next_actions,omitempty"`
	GeneratedAt        time.Time                  `json:"generated_at"`
}

type DiscoverResult struct {
	Kind        string              `json:"kind"`
	Candidates  []KeychainCandidate `json:"candidates"`
	Problems    []string            `json:"problems,omitempty"`
	NextActions []string            `json:"next_actions,omitempty"`
	GeneratedAt time.Time           `json:"generated_at"`
}

type Store struct {
	Kind     string    `json:"kind"`
	Path     string    `json:"path"`
	Accounts []Account `json:"accounts"`
}

type PublicStore struct {
	Kind     string          `json:"kind"`
	Path     string          `json:"path"`
	Accounts []AccountPublic `json:"accounts"`
}

type SearchOptions struct {
	Account string
	Query   string
	Since   time.Time
	Limit   int
	Mailbox string
}

type ReadOptions struct {
	Account string
	ID      string
	Mailbox string
	MaxBody int
}

type ReadManyOptions struct {
	Account string
	IDs     []string
	Mailbox string
	MaxBody int
}

type MessageSummary struct {
	ID        string    `json:"id"`
	ThreadID  string    `json:"thread_id,omitempty"`
	Mailbox   string    `json:"mailbox,omitempty"`
	From      string    `json:"from,omitempty"`
	To        []string  `json:"to,omitempty"`
	Subject   string    `json:"subject,omitempty"`
	Date      time.Time `json:"date,omitempty"`
	Snippet   string    `json:"snippet,omitempty"`
	HasAttach bool      `json:"has_attach,omitempty"`
	Redacted  bool      `json:"redacted"`
}

type Message struct {
	Summary  MessageSummary    `json:"summary"`
	Body     string            `json:"body,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	Redacted bool              `json:"redacted"`
}

type ReadManyResult struct {
	Kind        string             `json:"kind"`
	Account     AccountPublic      `json:"account"`
	Messages    []Message          `json:"messages"`
	Errors      []MessageReadError `json:"errors,omitempty"`
	GeneratedAt time.Time          `json:"generated_at"`
}

type MessageReadError struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}

type MutateOptions struct {
	Account string
	ID      string
	IDs     []string
	Mailbox string
	Target  string
	Approve bool
}

type MutateResult struct {
	Kind             string        `json:"kind"`
	Account          AccountPublic `json:"account"`
	Action           string        `json:"action"`
	IDs              []string      `json:"ids"`
	Target           string        `json:"target,omitempty"`
	Executed         bool          `json:"executed"`
	ApprovalRequired bool          `json:"approval_required"`
	Status           string        `json:"status"`
	Error            string        `json:"error,omitempty"`
	GeneratedAt      time.Time     `json:"generated_at"`
}

type AttachmentOptions struct {
	Account string
	ID      string
	Mailbox string
	Dir     string
	Approve bool
}

type AttachmentFile struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type,omitempty"`
	Size        int64  `json:"size"`
	Path        string `json:"path"`
}

type AttachmentResult struct {
	Kind             string           `json:"kind"`
	Account          AccountPublic    `json:"account"`
	ID               string           `json:"id"`
	Dir              string           `json:"dir"`
	Files            []AttachmentFile `json:"files"`
	Executed         bool             `json:"executed"`
	ApprovalRequired bool             `json:"approval_required"`
	Error            string           `json:"error,omitempty"`
	GeneratedAt      time.Time        `json:"generated_at"`
}

type SendOptions struct {
	DraftID string
	Approve bool
}

type ComposeOptions struct {
	Account string
	To      []string
	Subject string
	Body    string
}

type SendResult struct {
	Kind             string        `json:"kind"`
	DraftID          string        `json:"draft_id"`
	Account          AccountPublic `json:"account"`
	To               []string      `json:"to"`
	Subject          string        `json:"subject"`
	Executed         bool          `json:"executed"`
	ApprovalRequired bool          `json:"approval_required"`
	Status           string        `json:"status"`
	Error            string        `json:"error,omitempty"`
	GeneratedAt      time.Time     `json:"generated_at"`
}

type WatchOptions struct {
	Account string
	Since   time.Duration
	Limit   int
}

type WatchResult struct {
	Kind        string           `json:"kind"`
	Account     AccountPublic    `json:"account"`
	Since       time.Time        `json:"since"`
	Messages    []MessageSummary `json:"messages"`
	GeneratedAt time.Time        `json:"generated_at"`
}

type SearchResult struct {
	Kind        string           `json:"kind"`
	Account     AccountPublic    `json:"account"`
	Query       string           `json:"query,omitempty"`
	Since       time.Time        `json:"since,omitempty"`
	Limit       int              `json:"limit"`
	Messages    []MessageSummary `json:"messages"`
	GeneratedAt time.Time        `json:"generated_at"`
}

type Draft struct {
	ID           string    `json:"id"`
	Account      string    `json:"account"`
	ThreadID     string    `json:"thread_id,omitempty"`
	To           []string  `json:"to"`
	Subject      string    `json:"subject"`
	Body         string    `json:"body"`
	Status       string    `json:"status"`
	Policy       string    `json:"policy"`
	ApprovalHint string    `json:"approval_hint"`
	Path         string    `json:"path,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

func publicAccount(account Account) AccountPublic {
	backend := strings.ToLower(strings.TrimSpace(account.Backend))
	if backend == "" {
		backend = "imap"
	}
	mailbox := strings.TrimSpace(account.Mailbox)
	if mailbox == "" {
		mailbox = "INBOX"
	}
	port := account.Port
	if port == 0 {
		port = 993
	}
	return AccountPublic{
		ID:                  strings.TrimSpace(account.ID),
		Backend:             backend,
		Email:               strings.TrimSpace(account.Email),
		Host:                strings.TrimSpace(account.Host),
		Port:                port,
		SMTPHost:            strings.TrimSpace(account.SMTPHost),
		SMTPPort:            account.SMTPPort,
		Username:            strings.TrimSpace(account.Username),
		Mailbox:             mailbox,
		TLS:                 account.TLS || account.Port == 0 || account.Port == 993,
		SMTPTLS:             account.SMTPTLS || account.SMTPPort == 0 || account.SMTPPort == 465,
		PasswordConfigured:  strings.TrimSpace(account.PasswordEnv) != "" || strings.TrimSpace(account.PasswordHandle) != "",
		PasswordHandle:      strings.TrimSpace(account.PasswordHandle),
		PasswordEnvironment: strings.TrimSpace(account.PasswordEnv),
	}
}
