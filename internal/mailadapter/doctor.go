package mailadapter

import (
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/meshclaw/meshclaw/internal/guardvault"
)

func Doctor(opts DoctorOptions) (DoctorResult, error) {
	result := DoctorResult{
		Kind:        "meshclaw_mail_doctor",
		ConfigPath:  ConfigPath(),
		Backends:    guardvault.Backends(),
		GeneratedAt: time.Now().UTC(),
	}
	store, err := loadStore()
	if err != nil {
		result.Problems = append(result.Problems, err.Error())
		return result, err
	}
	for _, account := range store.Accounts {
		if opts.Account != "" && !strings.EqualFold(account.ID, opts.Account) {
			continue
		}
		status := checkAccount(account, opts.CheckLogin)
		result.Accounts = append(result.Accounts, status)
		if status.Status != "ok" && status.Status != "configured" {
			result.Problems = append(result.Problems, account.ID+": "+status.Error)
		}
	}
	if opts.Account != "" && len(result.Accounts) == 0 {
		err := fmt.Errorf("mail account %q was not found", opts.Account)
		result.Problems = append(result.Problems, err.Error())
		return result, err
	}
	if len(store.Accounts) == 0 {
		result.NextActions = append(result.NextActions, "계정이 없습니다. Gmail/Naver는 `meshclaw mail setup user@gmail.com --json`으로 시작하세요.")
	}
	candidates, discoverErr := DiscoverKeychain()
	if discoverErr == nil {
		result.KeychainCandidates = candidates.Candidates
	} else {
		result.Problems = append(result.Problems, discoverErr.Error())
	}
	if len(result.Problems) == 0 {
		result.NextActions = append(result.NextActions, "메일 읽기 준비가 됐습니다. `meshclaw mail search --account <id> --limit 5 --json`으로 확인하세요.")
	}
	return result, nil
}

func checkAccount(account Account, checkLogin bool) AccountStatus {
	normalized, err := normalizeAccount(account)
	status := AccountStatus{Account: publicAccount(account), ConfigOK: err == nil}
	if err != nil {
		status.Status = "config_error"
		status.Error = err.Error()
		status.NextAction = "mail setup을 다시 실행하거나 mail-accounts.json의 host, username, password_handle을 확인하세요."
		return status
	}
	status.Account = publicAccount(normalized)
	status.Backend = normalized.Backend
	if !checkLogin {
		status.Status = "configured"
		status.NextAction = "로그인 검사는 `meshclaw mail doctor --json`으로 실행하세요."
		return status
	}
	client, err := dialIMAP(normalized)
	if err != nil {
		status.Status = "network_error"
		status.Error = err.Error()
		status.NextAction = "IMAP host/port/TLS와 네트워크 방화벽을 확인하세요."
		return status
	}
	status.NetworkOK = true
	defer client.close()
	if err := client.login(normalized); err != nil {
		status.Status = "login_error"
		status.Error = err.Error()
		status.NextAction = "앱 비밀번호 또는 Keychain 연결이 맞는지 확인하세요. Gmail/Naver는 일반 계정 비밀번호가 막힐 수 있습니다."
		return status
	}
	status.LoginOK = true
	if err := client.selectMailbox(normalized.Mailbox); err != nil {
		status.Status = "mailbox_error"
		status.Error = err.Error()
		status.NextAction = "mailbox 이름을 확인하세요. 기본값은 INBOX입니다."
		return status
	}
	status.MailboxOK = true
	status.Status = "ok"
	return status
}

func DiscoverKeychain() (DiscoverResult, error) {
	result := DiscoverResult{Kind: "meshclaw_mail_discover_keychain", GeneratedAt: time.Now().UTC()}
	path, err := exec.LookPath("security")
	if err != nil {
		result.Problems = append(result.Problems, "macOS security command not found")
		result.NextActions = append(result.NextActions, "Gmail/Naver 앱 비밀번호는 `meshclaw guard-vault-put`으로 저장하거나 provider setup을 사용하세요.")
		return result, err
	}
	out, err := exec.Command(path, "dump-keychain").CombinedOutput()
	if err != nil {
		result.Problems = append(result.Problems, "Keychain metadata scan failed: "+err.Error())
		return result, err
	}
	result.Candidates = parseKeychainCandidates(string(out))
	if len(result.Candidates) == 0 {
		result.NextActions = append(result.NextActions, "메일 Keychain 후보가 없습니다. provider setup 후 앱 비밀번호를 Guard에 저장하세요.")
		return result, nil
	}
	result.NextActions = append(result.NextActions, "원하는 후보를 골라 `meshclaw mail setup <email> --mode keychain --service '<service>' --account '<account>' --execute`를 실행하세요.")
	return result, nil
}

func parseKeychainCandidates(text string) []KeychainCandidate {
	acctRe := regexp.MustCompile(`"acct"<blob>="([^"]+)"`)
	svceRe := regexp.MustCompile(`"svce"<blob>="([^"]+)"`)
	service := ""
	account := ""
	out := []KeychainCandidate{}
	seen := map[string]bool{}
	flush := func() {
		if !looksLikeMailKeychain(service, account) {
			service, account = "", ""
			return
		}
		key := service + "\x00" + account
		if !seen[key] {
			seen[key] = true
			out = append(out, KeychainCandidate{Service: service, Account: account, Reason: "macOS keychain mail credential candidate"})
		}
		service, account = "", ""
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "keychain:") {
			flush()
			continue
		}
		if match := acctRe.FindStringSubmatch(line); len(match) == 2 {
			account = match[1]
			continue
		}
		if match := svceRe.FindStringSubmatch(line); len(match) == 2 {
			service = match[1]
			continue
		}
	}
	flush()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Account == out[j].Account {
			return out[i].Service < out[j].Service
		}
		return out[i].Account < out[j].Account
	})
	return out
}

func looksLikeMailKeychain(service, account string) bool {
	service = strings.TrimSpace(service)
	account = strings.TrimSpace(account)
	if service == "" || !strings.Contains(account, "@") {
		return false
	}
	lower := strings.ToLower(service + " " + account)
	for _, token := range []string{"mail", "mox", "gmail", "google", "naver", "imap", "smtp", "outlook", "icloud"} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}
