package guard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type SessionPolicy struct {
	Name             string   `json:"name"`
	DefaultMode      string   `json:"default_mode"`
	Transcript       string   `json:"transcript"`
	Memory           string   `json:"memory"`
	RAG              string   `json:"rag"`
	AllowedSurfaces  []string `json:"allowed_surfaces"`
	DeniedSurfaces   []string `json:"denied_surfaces"`
	AllowedArtifacts []string `json:"allowed_artifacts"`
	ForbiddenData    []string `json:"forbidden_data"`
	EndActions       []string `json:"end_actions"`
	Principle        string   `json:"principle"`
}

type SignalPolicy struct {
	Name            string   `json:"name"`
	Role            string   `json:"role"`
	WhySignal       []string `json:"why_signal"`
	AllowedMembers  []string `json:"allowed_members"`
	DeniedMembers   []string `json:"denied_members"`
	RequiredRules   []string `json:"required_rules"`
	AllowedMessages []string `json:"allowed_messages"`
	AllowedReports  []string `json:"allowed_reports"`
	ForbiddenFlows  []string `json:"forbidden_flows"`
	Principle       string   `json:"principle"`
}

type Session struct {
	ID          string    `json:"id"`
	Status      string    `json:"status"`
	Surface     string    `json:"surface"`
	CreatedAt   time.Time `json:"created_at"`
	EndedAt     time.Time `json:"ended_at,omitempty"`
	RawStored   bool      `json:"raw_stored"`
	Transcript  string    `json:"transcript"`
	Memory      string    `json:"memory"`
	RAG         string    `json:"rag"`
	HandleCount int       `json:"handle_count"`
	Notes       []string  `json:"notes,omitempty"`
}

type SessionStartOptions struct {
	Surface string
}

func GuardSessionPolicy() SessionPolicy {
	return SessionPolicy{
		Name:        "MeshClaw Guard ephemeral session",
		DefaultMode: "ephemeral-local-only",
		Transcript:  "disabled; store metadata only",
		Memory:      "disabled",
		RAG:         "disabled",
		AllowedSurfaces: []string{
			"local terminal",
			"localhost Guard UI",
			"Signal Guard chat with disappearing messages",
		},
		DeniedSurfaces: []string{
			"Claude",
			"Codex",
			"ChatGPT",
			"Cursor cloud chat",
			"Matrix bridge",
			"search/RAG indexers",
			"shared logs",
		},
		AllowedArtifacts: []string{
			"vault:// handles",
			"redacted evidence",
			"approval record",
			"last-used timestamp",
			"fingerprint",
		},
		ForbiddenData: []string{
			"raw passwords",
			"raw API tokens",
			"private keys",
			"recovery codes",
			"copyable secret substrings",
		},
		EndActions: []string{
			"purge local session metadata when no longer needed",
			"delete Signal messages where possible",
			"run guard-clean-plan for local chat stores",
			"continue with vault:// handles only",
		},
		Principle: "Guard remembers handles, metadata, approval, and evidence; it does not remember raw secrets.",
	}
}

func SignalGuardPolicy() SignalPolicy {
	return SignalPolicy{
		Name: "Signal Guard secret ingress",
		Role: "Mobile-friendly local secret entry channel for MeshClaw Guard; not a general assistant room.",
		WhySignal: []string{
			"smaller surface than Matrix rooms and bridges",
			"disappearing messages are built into normal user behavior",
			"easier to keep Claude, Codex, GPT, and indexers out of the raw-secret channel",
		},
		AllowedMembers: []string{
			"owner user",
			"local MeshClaw Guard bot",
			"optional local-only model process",
		},
		DeniedMembers: []string{
			"Claude",
			"Codex",
			"ChatGPT",
			"external LLM bots",
			"Matrix bridges",
			"Open WebUI cloud endpoints",
			"RAG/search indexers",
		},
		RequiredRules: []string{
			"disappearing messages enabled",
			"no cloud model bridge in the Signal Guard chat",
			"raw secrets are imported into Keychain/pass/vault immediately",
			"responses return handles only",
			"delete local bot logs after import",
			"never forward raw Signal messages into MCP evidence",
		},
		AllowedMessages: []string{
			"store this token as cloudflare/main-token",
			"list handles",
			"approve one use of vault://meshclaw/...",
			"delete the local Guard session",
		},
		AllowedReports: []string{
			"Codex/Claude may post redacted summaries generated from MeshClaw evidence",
			"Codex/Claude may report workflow status, failed steps, next actions, and evidence paths",
			"Codex/Claude may ask the user to approve a handle use, but must not ask for raw values",
		},
		ForbiddenFlows: []string{
			"Claude/Codex reads the Signal Guard chat",
			"Signal raw message is pasted into MCP tools",
			"Signal raw message is archived into evidence",
			"Signal raw message enters RAG or chat memory",
			"Claude/Codex posts raw secrets into Signal reports",
		},
		Principle: "Signal can receive redacted operator reports and approvals, but raw secret ingress stays isolated from Claude, Codex, GPT, bridges, and indexes.",
	}
}

func SessionRoot() string {
	if path := strings.TrimSpace(os.Getenv("MESHCLAW_GUARD_SESSION_DIR")); path != "" {
		return path
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".meshclaw", "guard-sessions")
	}
	return filepath.Join(".meshclaw", "guard-sessions")
}

func StartSession(opts SessionStartOptions) (Session, error) {
	surface := strings.TrimSpace(opts.Surface)
	if surface == "" {
		surface = "local"
	}
	now := time.Now().UTC()
	session := Session{
		ID:         "guard-" + now.Format("20060102T150405Z"),
		Status:     "active",
		Surface:    surface,
		CreatedAt:  now,
		RawStored:  false,
		Transcript: "disabled",
		Memory:     "disabled",
		RAG:        "disabled",
		Notes: []string{
			"raw secrets must be stored through guard-vault-put or local Guard import",
			"cloud AI clients may receive handles only",
		},
	}
	return session, writeSession(session)
}

func ListSessions() ([]Session, error) {
	root := SessionRoot()
	paths, err := filepath.Glob(filepath.Join(root, "*.json"))
	if err != nil {
		return nil, err
	}
	out := make([]Session, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var session Session
		if json.Unmarshal(data, &session) == nil {
			out = append(out, session)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func EndSession(id string) (Session, error) {
	session, err := readSession(id)
	if err != nil {
		return Session{}, err
	}
	now := time.Now().UTC()
	session.Status = "ended"
	session.EndedAt = now
	session.RawStored = false
	session.Transcript = "purged"
	session.Memory = "purged"
	session.RAG = "purged"
	session.Notes = append(session.Notes, "session ended; continue with vault handles only")
	return session, writeSession(session)
}

func PurgeSessions() (map[string]interface{}, error) {
	root := SessionRoot()
	paths, err := filepath.Glob(filepath.Join(root, "*.json"))
	if err != nil {
		return nil, err
	}
	removed := 0
	for _, path := range paths {
		if err := os.Remove(path); err == nil {
			removed++
		}
	}
	return map[string]interface{}{
		"root":                root,
		"removed":             removed,
		"raw_secret_removed":  false,
		"raw_secret_stored":   false,
		"next":                "delete Signal disappearing-message remnants if any and run guard-clean-plan for local UI stores",
		"principle":           GuardSessionPolicy().Principle,
		"external_llm_access": "denied",
	}, nil
}

func writeSession(session Session) error {
	root := SessionRoot()
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, session.ID+".json"), append(data, '\n'), 0600)
}

func readSession(id string) (Session, error) {
	id = strings.TrimSpace(id)
	data, err := os.ReadFile(filepath.Join(SessionRoot(), id+".json"))
	if err != nil {
		return Session{}, err
	}
	var session Session
	return session, json.Unmarshal(data, &session)
}
