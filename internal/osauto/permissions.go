package osauto

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ArgosPermissionGrant struct {
	ID        string    `json:"id"`
	Action    string    `json:"action"`
	Scope     string    `json:"scope"`
	Label     string    `json:"label,omitempty"`
	GrantedBy string    `json:"granted_by,omitempty"`
	Source    string    `json:"source,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type ArgosPermissionDecision struct {
	Grantable bool                  `json:"grantable"`
	Allowed   bool                  `json:"allowed"`
	Action    string                `json:"action"`
	Scope     string                `json:"scope,omitempty"`
	Label     string                `json:"label,omitempty"`
	Reason    string                `json:"reason,omitempty"`
	Grant     *ArgosPermissionGrant `json:"grant,omitempty"`
}

type argosPermissionStore struct {
	Kind      string                 `json:"kind"`
	Version   int                    `json:"version"`
	Grants    []ArgosPermissionGrant `json:"grants"`
	UpdatedAt time.Time              `json:"updated_at"`
}

func CheckArgosPermission(action ArgosAction) ArgosPermissionDecision {
	decision := argosPermissionDecisionFor(action)
	if !decision.Grantable {
		return decision
	}
	store, err := loadArgosPermissionStore()
	if err != nil {
		decision.Reason = err.Error()
		return decision
	}
	for _, grant := range store.Grants {
		if grant.Action == decision.Action && grant.Scope == decision.Scope {
			decision.Allowed = true
			copied := grant
			decision.Grant = &copied
			return decision
		}
	}
	return decision
}

func GrantArgosPermission(action ArgosAction, grantedBy, source string) (ArgosPermissionGrant, error) {
	decision := argosPermissionDecisionFor(action)
	if !decision.Grantable {
		return ArgosPermissionGrant{}, errors.New(decision.Reason)
	}
	store, err := loadArgosPermissionStore()
	if err != nil {
		return ArgosPermissionGrant{}, err
	}
	now := time.Now().UTC()
	grant := ArgosPermissionGrant{
		ID:        argosPermissionID(decision.Action, decision.Scope),
		Action:    decision.Action,
		Scope:     decision.Scope,
		Label:     decision.Label,
		GrantedBy: strings.TrimSpace(grantedBy),
		Source:    strings.TrimSpace(source),
		CreatedAt: now,
	}
	replaced := false
	for i, existing := range store.Grants {
		if existing.Action == grant.Action && existing.Scope == grant.Scope {
			if grant.GrantedBy == "" {
				grant.GrantedBy = existing.GrantedBy
			}
			if grant.Source == "" {
				grant.Source = existing.Source
			}
			store.Grants[i] = grant
			replaced = true
			break
		}
	}
	if !replaced {
		store.Grants = append(store.Grants, grant)
	}
	store.UpdatedAt = now
	if err := saveArgosPermissionStore(store); err != nil {
		return ArgosPermissionGrant{}, err
	}
	return grant, nil
}

func ListArgosPermissions() ([]ArgosPermissionGrant, error) {
	store, err := loadArgosPermissionStore()
	if err != nil {
		return nil, err
	}
	return append([]ArgosPermissionGrant(nil), store.Grants...), nil
}

func RevokeArgosPermission(ref string) (ArgosPermissionGrant, bool, error) {
	ref = strings.ToLower(strings.TrimSpace(ref))
	if ref == "" {
		return ArgosPermissionGrant{}, false, errors.New("permission reference is required")
	}
	store, err := loadArgosPermissionStore()
	if err != nil {
		return ArgosPermissionGrant{}, false, err
	}
	for i, grant := range store.Grants {
		if argosPermissionMatches(grant, ref) {
			store.Grants = append(store.Grants[:i], store.Grants[i+1:]...)
			store.UpdatedAt = time.Now().UTC()
			if err := saveArgosPermissionStore(store); err != nil {
				return ArgosPermissionGrant{}, false, err
			}
			return grant, true, nil
		}
	}
	return ArgosPermissionGrant{}, false, nil
}

func RevokeArgosPermissionForAction(action ArgosAction) (ArgosPermissionGrant, bool, error) {
	decision := argosPermissionDecisionFor(action)
	if !decision.Grantable {
		return ArgosPermissionGrant{}, false, errors.New(decision.Reason)
	}
	return RevokeArgosPermission(decision.Action + ":" + decision.Scope)
}

func argosPermissionMatches(grant ArgosPermissionGrant, ref string) bool {
	ref = strings.ToLower(strings.TrimSpace(ref))
	if ref == "" {
		return false
	}
	if ref == strings.ToLower(grant.ID) ||
		ref == strings.ToLower(grant.Action) ||
		ref == strings.ToLower(grant.Scope) ||
		ref == strings.ToLower(grant.Label) ||
		ref == strings.ToLower(grant.Action+":"+grant.Scope) {
		return true
	}
	return strings.Contains(strings.ToLower(grant.Label), ref)
}

func argosPermissionDecisionFor(action ArgosAction) ArgosPermissionDecision {
	decision := ArgosPermissionDecision{Action: action.Action}
	switch action.Action {
	case "help":
		decision.Allowed = true
		decision.Scope = "help"
		decision.Label = "Argos 도움말"
	case "browser_search", "visible_browser_search":
		decision.Grantable = true
		decision.Action = "browser_search"
		decision.Scope = "web_search"
		decision.Label = "웹 검색"
	case "work_demo":
		decision.Grantable = true
		decision.Scope = "work_demo"
		decision.Label = "Argos 작업 데모"
	case "document_create":
		decision.Grantable = true
		decision.Scope = "argos_document"
		decision.Label = "Argos 문서 생성"
	case "mac_runner_command":
		decision.Grantable = true
		decision.Scope = "mac_runner"
		decision.Label = "Mac 앱 실행기"
	case "macbook_command", "device_runner_command":
		decision.Grantable = true
		decision.Action = "mac_runner_command"
		decision.Scope = "mac_runner"
		decision.Label = "Mac 앱 실행기"
	case "browser_fetch", "open_url":
		host := argosURLHost(action.URL)
		if host == "" {
			decision.Reason = "URL host를 확인할 수 없어 반복 허용할 수 없습니다."
			return decision
		}
		decision.Grantable = true
		decision.Scope = host
		decision.Label = host
	case "open_app":
		app := strings.TrimSpace(action.App)
		if app == "" {
			decision.Reason = "앱 이름을 확인할 수 없어 반복 허용할 수 없습니다."
			return decision
		}
		decision.Grantable = true
		decision.Scope = strings.ToLower(app)
		decision.Label = app
	case "note_create":
		decision.Grantable = true
		decision.Scope = "notes"
		decision.Label = "Notes 메모 생성"
	case "reminder_create":
		decision.Grantable = true
		decision.Scope = "reminders"
		decision.Label = "Reminders 할 일 생성"
	case "reminders_list":
		decision.Grantable = true
		decision.Scope = "reminders_read"
		decision.Label = "Reminders 할 일 조회"
	case "reminder_complete":
		decision.Scope = "reminders_mutation"
		decision.Label = "Reminders 할 일 완료"
		decision.Reason = "리마인더 완료는 개인 데이터 변경이라 매번 확인합니다."
	case "reminder_delete":
		decision.Scope = "reminders_mutation"
		decision.Label = "Reminders 할 일 삭제"
		decision.Reason = "리마인더 삭제는 되돌리기 어려운 개인 데이터 변경이라 매번 확인합니다."
	case "calendar_event_create":
		decision.Grantable = true
		decision.Scope = "calendar"
		decision.Label = "Calendar 일정 생성"
	case "calendar_events_list":
		decision.Grantable = true
		decision.Scope = "calendar_read"
		decision.Label = "Calendar 일정 조회"
	case "calendar_event_delete":
		decision.Scope = "calendar_mutation"
		decision.Label = "Calendar 일정 삭제"
		decision.Reason = "캘린더 일정 삭제는 되돌리기 어려운 개인 데이터 변경이라 매번 확인합니다."
	case "contacts_search":
		decision.Grantable = true
		decision.Scope = "contacts_read"
		decision.Label = "Contacts 연락처 조회"
	case "shortcut_run":
		shortcut := strings.TrimSpace(action.Shortcut)
		if shortcut == "" {
			decision.Reason = "단축어 이름을 확인할 수 없어 반복 허용할 수 없습니다."
			return decision
		}
		decision.Grantable = true
		decision.Scope = strings.ToLower(shortcut)
		decision.Label = shortcut
	case "ai_handoff":
		provider := strings.TrimSpace(action.Provider)
		if provider == "" {
			provider = "default"
		}
		decision.Grantable = true
		decision.Scope = strings.ToLower(provider)
		decision.Label = provider + " handoff"
	default:
		decision.Reason = "이 작업은 반복 허용 대상이 아닙니다."
	}
	return decision
}

func argosURLHost(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parsed.Hostname()))
}

func argosPermissionID(action, scope string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(action)) + "\x00" + strings.ToLower(strings.TrimSpace(scope))))
	return hex.EncodeToString(sum[:8])
}

func loadArgosPermissionStore() (argosPermissionStore, error) {
	path := argosPermissionPath()
	store := argosPermissionStore{Kind: "meshclaw_argos_permissions", Version: 1, Grants: []ArgosPermissionGrant{}}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return store, nil
	}
	if err != nil {
		return store, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return store, nil
	}
	if err := json.Unmarshal(data, &store); err != nil {
		return store, err
	}
	if store.Kind == "" {
		store.Kind = "meshclaw_argos_permissions"
	}
	if store.Version == 0 {
		store.Version = 1
	}
	if store.Grants == nil {
		store.Grants = []ArgosPermissionGrant{}
	}
	return store, nil
}

func saveArgosPermissionStore(store argosPermissionStore) error {
	path := argosPermissionPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}

func argosPermissionPath() string {
	if path := strings.TrimSpace(os.Getenv("MESHCLAW_ARGOS_PERMISSIONS")); path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "meshclaw-argos-permissions.json")
	}
	return filepath.Join(home, ".meshclaw", "argos-permissions.json")
}
