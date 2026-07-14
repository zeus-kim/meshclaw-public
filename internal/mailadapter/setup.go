package mailadapter

import (
	"fmt"
	"net"
	"net/mail"
	"sort"
	"strings"
	"time"

	"github.com/meshclaw/meshclaw/internal/guardvault"
)

type providerPreset struct {
	ID       string
	Domains  []string
	Host     string
	Port     int
	Mailbox  string
	Username string
	Notes    []string
}

func Setup(opts SetupOptions) (SetupResult, error) {
	email := strings.ToLower(strings.TrimSpace(opts.Email))
	mode := strings.ToLower(strings.TrimSpace(opts.Mode))
	if mode == "" {
		mode = "auto"
	}
	result := SetupResult{
		Kind:        "meshclaw_mail_setup",
		Email:       email,
		Mode:        "dry-run",
		SetupMode:   mode,
		ConfigPath:  ConfigPath(),
		GeneratedAt: time.Now().UTC(),
	}
	if opts.Execute {
		result.Mode = "execute"
	}
	if _, err := mail.ParseAddress(email); err != nil {
		result.Problems = append(result.Problems, "이메일 주소 형식이 올바르지 않습니다.")
		return result, fmt.Errorf("invalid email address")
	}
	local, domain := splitEmail(email)
	result.AccountID = sanitizeID(local + "-" + domain)
	preset := presetForDomain(domain)
	if mode == "auto" {
		if preset.ID == "generic" {
			mode = "direct"
		} else {
			mode = "provider"
		}
	}
	result.SetupMode = mode
	if mode != "provider" && mode != "keychain" && mode != "direct" {
		result.Problems = append(result.Problems, "지원하는 setup mode는 provider, keychain, direct 입니다.")
		return result, fmt.Errorf("unsupported mail setup mode %q", mode)
	}
	account := Account{
		ID:             result.AccountID,
		Backend:        "imap",
		Email:          email,
		Host:           preset.Host,
		Port:           preset.Port,
		SMTPHost:       smtpHostForPreset(preset, domain),
		SMTPPort:       465,
		Username:       presetUsername(preset, email, local),
		PasswordHandle: guardvault.Handle("mail", result.AccountID+"-app-password"),
		Mailbox:        firstNonEmpty(preset.Mailbox, "INBOX"),
		TLS:            true,
		SMTPTLS:        true,
	}
	if mode == "direct" || strings.TrimSpace(opts.Host) != "" {
		account.Host = strings.TrimSpace(opts.Host)
		if account.Host == "" {
			account.Host = "imap." + domain
		}
		if opts.Port > 0 {
			account.Port = opts.Port
		}
		if strings.TrimSpace(opts.Username) != "" {
			account.Username = strings.TrimSpace(opts.Username)
		}
	}
	if mode == "keychain" {
		service := strings.TrimSpace(opts.Service)
		keychainAccount := strings.TrimSpace(opts.Account)
		if service == "" || keychainAccount == "" {
			result.Keychain = keychainCandidates(email, local, domain, preset)
			result.Problems = append(result.Problems, "keychain mode는 --service 와 --account 가 필요합니다.")
			result.NextActions = append(result.NextActions, "예: meshclaw mail setup you@example.com --mode keychain --service 'Mox: you@example.com' --account you@example.com --execute")
			return result, fmt.Errorf("keychain setup requires service and account")
		}
		result.Keychain = []KeychainCandidate{{Service: service, Account: keychainAccount, Reason: "user-selected existing keychain item"}}
	} else {
		result.Keychain = keychainCandidates(email, local, domain, preset)
	}
	result.IMAPHosts = uniqueStrings([]string{account.Host, "imap." + domain, "mail." + domain})
	result.Usernames = uniqueStrings([]string{account.Username, email, local})
	result.MXHosts = lookupMXHosts(domain)
	if preset.ID == "gmail" {
		result.NextActions = append(result.NextActions,
			"Gmail은 보통 Google 계정 비밀번호가 아니라 앱 비밀번호 또는 OAuth가 필요합니다.",
			"Google 계정에서 2단계 인증을 켠 뒤 앱 비밀번호를 만들고 Guard에 저장하세요.",
		)
	}
	if preset.ID == "naver" {
		result.NextActions = append(result.NextActions,
			"Naver는 환경에 따라 IMAP 사용 설정과 앱 비밀번호가 필요할 수 있습니다.",
			"Naver Mail 설정에서 IMAP/SMTP 사용 여부를 먼저 확인하세요.",
		)
	}
	if !opts.Execute {
		result.Account = ptr(publicAccount(account))
		result.NextActions = append(result.NextActions,
			"실제 저장은 --execute로 실행합니다.",
			"비밀번호 원문은 설정 파일에 쓰지 않습니다. 앱 비밀번호는 Guard/Keychain handle로 연결하세요.",
		)
		return result, nil
	}
	if mode == "keychain" {
		entry, err := guardvault.LinkKeychain(guardvault.LinkKeychainOptions{
			Scope:       "mail",
			Name:        result.AccountID + "-app-password",
			Kind:        "app-password",
			Description: email + " existing keychain password",
			Service:     strings.TrimSpace(opts.Service),
			Account:     strings.TrimSpace(opts.Account),
		})
		if err != nil {
			result.Problems = append(result.Problems, err.Error())
			return result, err
		}
		account.PasswordHandle = entry.Handle
	}
	_, public, err := UpsertAccount(account)
	if err != nil {
		result.Problems = append(result.Problems, err.Error())
		return result, err
	}
	result.Saved = true
	result.Account = &public
	if mode != "keychain" {
		result.NextActions = append(result.NextActions,
			"앱 비밀번호를 Guard에 저장하세요: meshclaw guard-vault-put mail "+result.AccountID+"-app-password app-password \"mail app password\" --backend keychain",
			"기존 키체인 항목이 있으면 keychain mode로 다시 실행하세요.",
		)
	}
	result.NextActions = append(result.NextActions, "그 뒤 meshclaw mail search --account "+result.AccountID+" --limit 5 --json 으로 테스트하세요.")
	return result, nil
}

func smtpHostForPreset(preset providerPreset, domain string) string {
	switch preset.ID {
	case "gmail":
		return "smtp.gmail.com"
	case "naver":
		return "smtp.naver.com"
	case "daum":
		return "smtp.daum.net"
	case "outlook":
		return "smtp.office365.com"
	case "icloud":
		return "smtp.mail.me.com"
	default:
		return "mail." + domain
	}
}

func presetForDomain(domain string) providerPreset {
	presets := []providerPreset{
		{ID: "gmail", Domains: []string{"gmail.com", "googlemail.com"}, Host: "imap.gmail.com", Port: 993, Mailbox: "INBOX", Username: "email"},
		{ID: "naver", Domains: []string{"naver.com"}, Host: "imap.naver.com", Port: 993, Mailbox: "INBOX", Username: "email"},
		{ID: "daum", Domains: []string{"daum.net", "hanmail.net"}, Host: "imap.daum.net", Port: 993, Mailbox: "INBOX", Username: "email"},
		{ID: "outlook", Domains: []string{"outlook.com", "hotmail.com", "live.com"}, Host: "outlook.office365.com", Port: 993, Mailbox: "INBOX", Username: "email"},
		{ID: "icloud", Domains: []string{"icloud.com", "me.com", "mac.com"}, Host: "imap.mail.me.com", Port: 993, Mailbox: "INBOX", Username: "email"},
	}
	for _, preset := range presets {
		for _, candidate := range preset.Domains {
			if domain == candidate {
				return preset
			}
		}
	}
	return providerPreset{ID: "generic", Host: "imap." + domain, Port: 993, Mailbox: "INBOX", Username: "email"}
}

func presetUsername(preset providerPreset, email, local string) string {
	switch preset.Username {
	case "local":
		return local
	default:
		return email
	}
}

func splitEmail(email string) (string, string) {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return email, ""
	}
	return parts[0], parts[1]
}

func lookupMXHosts(domain string) []string {
	records, err := net.LookupMX(domain)
	if err != nil {
		return nil
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Pref < records[j].Pref })
	hosts := make([]string, 0, len(records))
	for _, record := range records {
		hosts = append(hosts, strings.TrimSuffix(record.Host, "."))
	}
	return hosts
}

func keychainCandidates(email, local, domain string, preset providerPreset) []KeychainCandidate {
	labels := []string{
		email,
		"Mail: " + email,
		"Mox: " + email,
		preset.ID + ": " + email,
	}
	accounts := []string{email, local}
	out := []KeychainCandidate{}
	for _, service := range labels {
		for _, account := range accounts {
			out = append(out, KeychainCandidate{Service: service, Account: account, Reason: "common macOS keychain naming"})
		}
	}
	if domain == "gmail.com" {
		out = append(out, KeychainCandidate{Service: "Gmail", Account: email, Reason: "provider preset"})
	}
	if domain == "naver.com" {
		out = append(out, KeychainCandidate{Service: "Naver", Account: email, Reason: "provider preset"})
	}
	return out
}

func uniqueStrings(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func ptr[T any](value T) *T {
	return &value
}
