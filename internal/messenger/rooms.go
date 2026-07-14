package messenger

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

type RoomDoctorOptions struct {
	SignalCLI string
	Timeout   time.Duration
}

type RoomBindOptions struct {
	SignalCLI string
	Timeout   time.Duration
	Room      string
	TargetID  string
	Mode      string
	Label     string
	Model     string
	BaseURL   string
}

type RoomCleanupOptions struct {
	SignalCLI string
	Timeout   time.Duration
	Execute   bool
	Approve   bool
}

type SignalRoom struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name,omitempty"`
	IsMember              bool     `json:"is_member"`
	Blocked               bool     `json:"blocked,omitempty"`
	MessageExpirationTime int      `json:"message_expiration_time,omitempty"`
	Members               []string `json:"members,omitempty"`
	Admins                []string `json:"admins,omitempty"`
}

type RoomStatus struct {
	Room       SignalRoom `json:"room"`
	TargetID   string     `json:"target_id,omitempty"`
	Protected  bool       `json:"protected"`
	Class      string     `json:"class"`
	Reason     string     `json:"reason"`
	CanDelete  bool       `json:"can_delete"`
	NextAction string     `json:"next_action,omitempty"`
}

type RoomWarning struct {
	ID       string   `json:"id"`
	Severity string   `json:"severity"`
	Rooms    []string `json:"rooms,omitempty"`
	Message  string   `json:"message"`
	Next     string   `json:"next,omitempty"`
}

type DispatcherPauseStatus struct {
	Requested  bool   `json:"requested"`
	WasRunning bool   `json:"was_running"`
	Stopped    bool   `json:"stopped"`
	Restarted  bool   `json:"restarted"`
	StopError  string `json:"stop_error,omitempty"`
	StartError string `json:"start_error,omitempty"`
}

type RoomDoctorResult struct {
	Kind        string                 `json:"kind"`
	TargetsPath string                 `json:"targets_path"`
	Command     []string               `json:"command,omitempty"`
	Dispatcher  *DispatcherPauseStatus `json:"dispatcher_pause,omitempty"`
	Rooms       []RoomStatus           `json:"rooms"`
	Missing     []Target               `json:"missing_targets,omitempty"`
	Warnings    []RoomWarning          `json:"warnings,omitempty"`
	Problems    []string               `json:"problems,omitempty"`
	NextActions []string               `json:"next_actions,omitempty"`
	GeneratedAt time.Time              `json:"generated_at"`
}

type RoomBindResult struct {
	Kind        string        `json:"kind"`
	Room        SignalRoom    `json:"room"`
	Target      Target        `json:"target"`
	Store       TargetStore   `json:"store"`
	Warnings    []RoomWarning `json:"warnings,omitempty"`
	GeneratedAt time.Time     `json:"generated_at"`
}

type RoomCleanupResult struct {
	Kind        string                 `json:"kind"`
	Mode        string                 `json:"mode"`
	Command     []string               `json:"command,omitempty"`
	Dispatcher  *DispatcherPauseStatus `json:"dispatcher_pause,omitempty"`
	Candidates  []RoomStatus           `json:"candidates"`
	Deleted     []RoomStatus           `json:"deleted,omitempty"`
	Skipped     []RoomStatus           `json:"skipped,omitempty"`
	Problems    []string               `json:"problems,omitempty"`
	GeneratedAt time.Time              `json:"generated_at"`
}

func RoomsDoctor(opts RoomDoctorOptions) (RoomDoctorResult, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = 15 * time.Second
	}
	store, err := ListTargets()
	result := RoomDoctorResult{
		Kind:        "meshclaw_signal_rooms_doctor",
		TargetsPath: TargetPath(),
		GeneratedAt: time.Now().UTC(),
	}
	if err != nil {
		result.Problems = append(result.Problems, "messenger target registry could not be read: "+err.Error())
		return result, err
	}
	result.TargetsPath = store.Path
	rooms, command, err := listSignalRooms(opts.SignalCLI, opts.Timeout)
	result.Command = command
	if err != nil {
		result.Problems = append(result.Problems, "signal groups could not be listed: "+err.Error())
		result.NextActions = append(result.NextActions, "Verify signal-cli is installed, linked, and not locked by another process.")
		return result, err
	}
	result.Rooms, result.Missing = classifyRooms(store, rooms)
	result.Warnings = analyzeRoomTopology(result.Rooms)
	for _, status := range result.Rooms {
		switch status.Class {
		case "protected_inactive":
			result.Problems = append(result.Problems, fmt.Sprintf("protected target %s is not an active Signal member", status.TargetID))
		case "orphan_member":
			result.NextActions = append(result.NextActions, fmt.Sprintf("Review orphan room %q; delete only if it is an abandoned test room.", status.Room.Name))
		}
	}
	for _, target := range result.Missing {
		result.Problems = append(result.Problems, fmt.Sprintf("target %s points to a Signal group not present in this account", target.ID))
	}
	if len(result.Missing) > 0 {
		result.NextActions = append(result.NextActions, "Repair target group IDs with meshclaw messenger target-add, or recreate the missing Signal group.")
	}
	sort.Strings(result.Problems)
	sort.Strings(result.NextActions)
	return result, nil
}

func RoomsCleanup(opts RoomCleanupOptions) (RoomCleanupResult, error) {
	doctor, err := RoomsDoctor(RoomDoctorOptions{SignalCLI: opts.SignalCLI, Timeout: opts.Timeout})
	result := RoomCleanupResult{
		Kind:        "meshclaw_signal_rooms_cleanup",
		Mode:        "dry-run",
		Command:     doctor.Command,
		GeneratedAt: time.Now().UTC(),
	}
	if err != nil {
		result.Problems = append(result.Problems, doctor.Problems...)
		return result, err
	}
	for _, status := range doctor.Rooms {
		if status.CanDelete {
			result.Candidates = append(result.Candidates, status)
		} else {
			result.Skipped = append(result.Skipped, status)
		}
	}
	if !opts.Execute {
		return result, nil
	}
	if !opts.Approve {
		result.Problems = append(result.Problems, "cleanup execute requires --approve; protected rooms are never deleted")
		return result, fmt.Errorf("cleanup execute requires --approve")
	}
	result.Mode = "execute"
	for _, candidate := range result.Candidates {
		command, delErr := quitSignalGroup(opts.SignalCLI, opts.Timeout, candidate.Room.ID)
		result.Command = command
		if delErr != nil {
			candidate.NextAction = delErr.Error()
			result.Skipped = append(result.Skipped, candidate)
			result.Problems = append(result.Problems, fmt.Sprintf("failed to delete room %q: %s", candidate.Room.Name, delErr))
			continue
		}
		result.Deleted = append(result.Deleted, candidate)
	}
	return result, nil
}

func BindRoom(opts RoomBindOptions) (RoomBindResult, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = 15 * time.Second
	}
	result := RoomBindResult{Kind: "meshclaw_signal_room_bind", GeneratedAt: time.Now().UTC()}
	rooms, _, err := listSignalRooms(opts.SignalCLI, opts.Timeout)
	if err != nil {
		return result, err
	}
	room, ok := findSignalRoom(rooms, opts.Room)
	if !ok {
		return result, fmt.Errorf("signal room %q was not found", opts.Room)
	}
	if !room.IsMember {
		return result, fmt.Errorf("signal room %q is not an active room for this Signal account", firstNonEmpty(room.Name, room.ID))
	}
	targetID := strings.TrimSpace(opts.TargetID)
	if targetID == "" {
		targetID = roomTargetID(room, opts.Mode)
	}
	mode := strings.ToLower(strings.TrimSpace(opts.Mode))
	if mode == "" {
		mode = guessRoomMode(room.Name)
	}
	label := strings.TrimSpace(opts.Label)
	if label == "" {
		label = firstNonEmpty(room.Name, targetID)
	}
	store, target, err := UpsertTarget(Target{
		ID:      targetID,
		Channel: "signal",
		GroupID: room.ID,
		Label:   label,
		Mode:    mode,
		Model:   opts.Model,
		BaseURL: opts.BaseURL,
	})
	if err != nil {
		return result, err
	}
	result.Room = room
	result.Target = target
	result.Store = store
	if mode == "guard" {
		result.Warnings = append(result.Warnings, RoomWarning{
			ID:       "guard_signal_advanced",
			Severity: "warning",
			Rooms:    []string{firstNonEmpty(room.Name, room.ID)},
			Message:  "Signal Guard is an advanced option. Product defaults should keep raw secret ingress local-first and use Signal for redacted status or approval prompts.",
			Next:     "Verify the Argos safety number before entering passwords or tokens in this room.",
		})
	}
	return result, nil
}

func classifyRooms(store TargetStore, rooms []SignalRoom) ([]RoomStatus, []Target) {
	protected := map[string]Target{}
	for _, target := range store.Targets {
		if target.Channel == "signal" && strings.TrimSpace(target.GroupID) != "" {
			protected[target.GroupID] = target
		}
	}
	seen := map[string]bool{}
	statuses := make([]RoomStatus, 0, len(rooms))
	for _, room := range rooms {
		status := RoomStatus{Room: room}
		if target, ok := protected[room.ID]; ok {
			seen[room.ID] = true
			status.TargetID = target.ID
			status.Protected = true
			status.CanDelete = false
			if room.IsMember {
				status.Class = "protected_active"
				status.Reason = "registered MeshClaw target room"
				status.NextAction = "keep"
			} else {
				status.Class = "protected_inactive"
				status.Reason = "registered MeshClaw target exists but this Signal account is not an active member"
				status.NextAction = "repair membership or update target group id"
			}
		} else if room.IsMember {
			status.Class = "orphan_member"
			status.Reason = "Signal account is still a member, but no MeshClaw target references this group"
			status.CanDelete = looksLikeMeshClawTestRoom(room.Name)
			if status.CanDelete {
				status.NextAction = "cleanup candidate; dry-run by default"
			} else {
				status.NextAction = "review manually before any cleanup"
			}
		} else {
			status.Class = "left_or_deleted"
			status.Reason = "Signal account is no longer an active member"
			status.CanDelete = false
			status.NextAction = "ignore or clean local client cache"
		}
		statuses = append(statuses, status)
	}
	missing := []Target{}
	for id, target := range protected {
		if !seen[id] {
			missing = append(missing, target)
		}
	}
	sort.Slice(statuses, func(i, j int) bool {
		if statuses[i].Protected != statuses[j].Protected {
			return statuses[i].Protected
		}
		if statuses[i].Class != statuses[j].Class {
			return statuses[i].Class < statuses[j].Class
		}
		return statuses[i].Room.Name < statuses[j].Room.Name
	})
	sort.Slice(missing, func(i, j int) bool { return missing[i].ID < missing[j].ID })
	return statuses, missing
}

func analyzeRoomTopology(statuses []RoomStatus) []RoomWarning {
	type groupedRoom struct {
		id     string
		name   string
		target string
	}
	byMembers := map[string][]groupedRoom{}
	for _, status := range statuses {
		if !status.Protected || !status.Room.IsMember || len(status.Room.Members) == 0 {
			continue
		}
		key := memberSignature(status.Room.Members)
		if key == "" {
			continue
		}
		name := strings.TrimSpace(status.Room.Name)
		if name == "" {
			name = status.Room.ID
		}
		byMembers[key] = append(byMembers[key], groupedRoom{id: status.Room.ID, name: name, target: status.TargetID})
	}
	warnings := []RoomWarning{}
	for _, group := range byMembers {
		if len(group) < 3 {
			continue
		}
		rooms := make([]string, 0, len(group))
		for _, room := range group {
			label := room.name
			if room.target != "" {
				label += " (" + room.target + ")"
			}
			rooms = append(rooms, label)
		}
		sort.Strings(rooms)
		warnings = append(warnings, RoomWarning{
			ID:       "same_members_many_role_rooms",
			Severity: "info",
			Rooms:    rooms,
			Message:  "Several registered Signal rooms have the same member set. Signal is phone-number/person centric, so many role-specific rooms with the same Argos account can create confusing name/trust UI.",
			Next:     "This is acceptable for development. For product onboarding, prefer fewer Signal rooms, invite Argos as a person, and bind each user-created room to a MeshClaw mode only when needed.",
		})
	}
	sort.Slice(warnings, func(i, j int) bool { return warnings[i].ID < warnings[j].ID })
	return warnings
}

func findSignalRoom(rooms []SignalRoom, ref string) (SignalRoom, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return SignalRoom{}, false
	}
	for _, room := range rooms {
		if room.ID == ref || strings.EqualFold(room.Name, ref) {
			return room, true
		}
	}
	return SignalRoom{}, false
}

func roomTargetID(room SignalRoom, mode string) string {
	base := strings.TrimSpace(room.Name)
	if base == "" {
		base = "signal-room"
	}
	if mode = strings.TrimSpace(mode); mode != "" {
		base = base + "-" + mode
	}
	id := sanitizeID(base)
	if id == "" {
		id = "signal-room"
	}
	return id
}

func guessRoomMode(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	switch {
	case containsAnyText(lower, "ops", "운영", "장애", "incident", "server", "서버", "보안", "security"):
		return "ops"
	case containsAnyText(lower, "brief", "briefing", "브리핑", "뉴스", "news", "morning", "저녁", "기도"):
		return "briefing"
	case containsAnyText(lower, "guard", "vault", "비번", "비밀번호", "password", "secret", "token", "토큰"):
		return "guard"
	case containsAnyText(lower, "chat", "local", "ollama", "model", "대화"):
		return "chat"
	default:
		return "assistant"
	}
}

func containsAnyText(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func memberSignature(members []string) string {
	normalized := make([]string, 0, len(members))
	for _, member := range members {
		member = strings.TrimSpace(member)
		if member != "" {
			normalized = append(normalized, member)
		}
	}
	if len(normalized) == 0 {
		return ""
	}
	sort.Strings(normalized)
	return strings.Join(normalized, "\x00")
}

func listSignalRooms(binary string, timeout time.Duration) ([]SignalRoom, []string, error) {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		binary = signalCLIBinary()
	}
	args := []string{"-o", "json", "listGroups", "-d"}
	command := append([]string{binary}, args...)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, command, ctx.Err()
	}
	if err != nil {
		return nil, command, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	rooms, parseErr := ParseSignalRoomsJSON(out)
	if parseErr != nil {
		return nil, command, parseErr
	}
	return rooms, command, nil
}

func quitSignalGroup(binary string, timeout time.Duration, groupID string) ([]string, error) {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		binary = signalCLIBinary()
	}
	args := []string{"quitGroup", "-g", groupID, "--delete"}
	command := append([]string{binary}, args...)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return command, ctx.Err()
	}
	if err != nil {
		return command, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return command, nil
}

func ParseSignalRoomsJSON(data []byte) ([]SignalRoom, error) {
	data = extractJSONArray(data)
	var raw []map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	rooms := make([]SignalRoom, 0, len(raw))
	for _, item := range raw {
		room := SignalRoom{
			ID:                    firstString(item, "id", "groupId", "group_id"),
			Name:                  firstString(item, "name", "title"),
			IsMember:              firstBool(item, true, "isMember", "is_member", "active"),
			Blocked:               firstBool(item, false, "blocked", "isBlocked", "is_blocked"),
			MessageExpirationTime: firstInt(item, "messageExpirationTime", "message_expiration_time", "expirationTimer"),
			Members:               firstStringSlice(item, "members", "groupMembers"),
			Admins:                firstStringSlice(item, "admins", "groupAdmins"),
		}
		if room.ID == "" {
			room.ID = encodedGroupID(item)
		}
		if room.ID != "" {
			rooms = append(rooms, room)
		}
	}
	return rooms, nil
}

func extractJSONArray(data []byte) []byte {
	text := strings.TrimSpace(string(data))
	if strings.HasPrefix(text, "[") {
		return []byte(text)
	}
	start := strings.Index(text, "[")
	end := strings.LastIndex(text, "]")
	if start >= 0 && end > start {
		return []byte(text[start : end+1])
	}
	return data
}

func firstString(item map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := item[key]; ok {
			if s, ok := value.(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func firstBool(item map[string]interface{}, fallback bool, keys ...string) bool {
	for _, key := range keys {
		if value, ok := item[key]; ok {
			if b, ok := value.(bool); ok {
				return b
			}
		}
	}
	return fallback
}

func firstInt(item map[string]interface{}, keys ...string) int {
	for _, key := range keys {
		switch value := item[key].(type) {
		case float64:
			return int(value)
		case int:
			return value
		}
	}
	return 0
}

func firstStringSlice(item map[string]interface{}, keys ...string) []string {
	for _, key := range keys {
		values, ok := item[key].([]interface{})
		if !ok {
			continue
		}
		out := []string{}
		for _, value := range values {
			switch v := value.(type) {
			case string:
				out = append(out, strings.TrimSpace(v))
			case map[string]interface{}:
				if s := firstString(v, "number", "uuid", "address"); s != "" {
					out = append(out, s)
				}
			}
		}
		return out
	}
	return nil
}

func encodedGroupID(item map[string]interface{}) string {
	if id := firstString(item, "groupId"); id != "" {
		return id
	}
	if raw, ok := item["groupId"].([]interface{}); ok {
		buf := make([]byte, 0, len(raw))
		for _, value := range raw {
			if n, ok := value.(float64); ok {
				buf = append(buf, byte(n))
			}
		}
		if len(buf) > 0 {
			return base64.StdEncoding.EncodeToString(buf)
		}
	}
	return ""
}

func looksLikeMeshClawTestRoom(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	for _, marker := range []string{"argos", "meshclaw", "guard", "ops", "gpt-oss"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}
