package messenger

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ScheduledDeliveryPlanOptions struct {
	Target      string
	Schedule    string
	Content     string
	ContentType string
	Delivery    string
	Execute     bool
	Approve     bool
	Now         time.Time
}

type ScheduledDeliveryApplyOptions struct {
	Target          string
	Schedule        string
	Content         string
	ContentType     string
	Delivery        string
	Execute         bool
	Approve         bool
	PreviewApproved bool
	Now             time.Time
}

type ScheduledDeliveryPlan struct {
	Kind                    string    `json:"kind"`
	Action                  string    `json:"action"`
	Generated               time.Time `json:"generated"`
	Status                  string    `json:"status"`
	Target                  string    `json:"target"`
	ResolvedTarget          *Target   `json:"resolved_target,omitempty"`
	TargetCandidates        []Target  `json:"target_candidates,omitempty"`
	TargetError             string    `json:"target_error,omitempty"`
	Schedule                string    `json:"schedule"`
	Content                 string    `json:"content"`
	ContentType             string    `json:"content_type"`
	Delivery                string    `json:"delivery"`
	Execute                 bool      `json:"execute"`
	Approved                bool      `json:"approved"`
	ApprovalRequired        bool      `json:"approval_required"`
	ApprovalMissing         bool      `json:"approval_missing,omitempty"`
	FirstRunPreviewRequired bool      `json:"first_run_preview_required"`
	UserMessage             string    `json:"user_message"`
	ApprovalNote            string    `json:"approval_note"`
	StopBefore              []string  `json:"stop_before"`
	Next                    []string  `json:"next"`
}

type ScheduledDeliveryApplyResult struct {
	Kind                    string                  `json:"kind"`
	Action                  string                  `json:"action"`
	Generated               time.Time               `json:"generated"`
	Status                  string                  `json:"status"`
	Execute                 bool                    `json:"execute"`
	Approved                bool                    `json:"approved"`
	PreviewApproved         bool                    `json:"preview_approved"`
	ApprovalRequired        bool                    `json:"approval_required"`
	ApprovalMissing         bool                    `json:"approval_missing,omitempty"`
	FirstRunPreviewRequired bool                    `json:"first_run_preview_required"`
	Plan                    ScheduledDeliveryPlan   `json:"plan"`
	Job                     *ScheduledDeliveryJob   `json:"job,omitempty"`
	Store                   *ScheduledDeliveryStore `json:"store,omitempty"`
	Path                    string                  `json:"path,omitempty"`
	UserMessage             string                  `json:"user_message"`
	ApprovalNote            string                  `json:"approval_note"`
	Next                    []string                `json:"next"`
}

type ScheduledDeliveryStore struct {
	Kind      string                 `json:"kind"`
	Path      string                 `json:"path"`
	UpdatedAt time.Time              `json:"updated_at"`
	Jobs      []ScheduledDeliveryJob `json:"jobs"`
}

type ScheduledDeliveryJob struct {
	ID          string    `json:"id"`
	Enabled     bool      `json:"enabled"`
	Status      string    `json:"status"`
	TargetID    string    `json:"target_id"`
	TargetLabel string    `json:"target_label,omitempty"`
	Schedule    string    `json:"schedule"`
	Content     string    `json:"content"`
	ContentType string    `json:"content_type"`
	Delivery    string    `json:"delivery"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	LastRunAt   time.Time `json:"last_run_at,omitempty"`
}

func PlanScheduledDelivery(opts ScheduledDeliveryPlanOptions) ScheduledDeliveryPlan {
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	targetRef := strings.TrimSpace(opts.Target)
	scheduleText := strings.TrimSpace(opts.Schedule)
	content := strings.TrimSpace(opts.Content)
	contentType := normalizeScheduledContentType(opts.ContentType, content)
	delivery := normalizeScheduledDelivery(opts.Delivery, contentType)
	plan := ScheduledDeliveryPlan{
		Kind:                    "meshclaw_scheduled_delivery_plan",
		Action:                  "scheduled_delivery_plan",
		Generated:               now.UTC(),
		Status:                  "review_ready",
		Target:                  targetRef,
		Schedule:                scheduleText,
		Content:                 content,
		ContentType:             contentType,
		Delivery:                delivery,
		Execute:                 opts.Execute,
		Approved:                opts.Approve,
		ApprovalRequired:        true,
		FirstRunPreviewRequired: true,
		UserMessage:             "예약 발송 계획을 만들었습니다. 실제 등록이나 발송은 아직 하지 않았습니다.",
		ApprovalNote:            "This is plan-only. A future apply step must show the first-run preview, resolved Signal target, schedule, delivery mode, and require explicit approval before registering or sending.",
		StopBefore: []string{
			"registering a recurring job",
			"sending the first message",
			"calling a Signal target",
			"auto-creating a broad or ambiguous contact target",
		},
		Next: []string{
			"Review the resolved target, schedule, content type, and first-run preview.",
			"Ask the user for explicit approval before registering the schedule.",
			"Use voice/report creation tools at run time only after the schedule is approved.",
		},
	}
	if targetRef == "" || scheduleText == "" || content == "" {
		plan.Status = "missing_fields"
		if targetRef == "" {
			plan.Next = append(plan.Next, "Ask who should receive the scheduled delivery.")
		}
		if scheduleText == "" {
			plan.Next = append(plan.Next, "Ask when or how often the delivery should run.")
		}
		if content == "" {
			plan.Next = append(plan.Next, "Ask what message, report topic, or voice content should be sent.")
		}
		return plan
	}
	target, candidates, err := resolveAssistantVoiceTarget(targetRef)
	if err != nil {
		plan.Status = "target_review_required"
		plan.TargetError = err.Error()
		plan.TargetCandidates = candidates
		plan.UserMessage = "예약 발송 대상을 확정하지 못했습니다. 후보를 확인해야 합니다."
		return plan
	}
	plan.ResolvedTarget = &target
	if opts.Execute && !opts.Approve {
		plan.Status = "approval_required"
		plan.ApprovalMissing = true
		return plan
	}
	if opts.Execute && opts.Approve {
		plan.Status = "review_ready"
		plan.UserMessage = "예약 발송 계획을 만들었습니다. 등록은 첫 실행 미리보기 승인 후 apply 도구로만 진행합니다."
		return plan
	}
	return plan
}

func ApplyScheduledDelivery(opts ScheduledDeliveryApplyOptions) (ScheduledDeliveryApplyResult, error) {
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	plan := PlanScheduledDelivery(ScheduledDeliveryPlanOptions{
		Target:      opts.Target,
		Schedule:    opts.Schedule,
		Content:     opts.Content,
		ContentType: opts.ContentType,
		Delivery:    opts.Delivery,
		Execute:     opts.Execute,
		Approve:     opts.Approve,
		Now:         now,
	})
	result := ScheduledDeliveryApplyResult{
		Kind:                    "meshclaw_scheduled_delivery_apply",
		Action:                  "scheduled_delivery_apply",
		Generated:               now.UTC(),
		Status:                  "review_required",
		Execute:                 opts.Execute,
		Approved:                opts.Approve,
		PreviewApproved:         opts.PreviewApproved,
		ApprovalRequired:        true,
		FirstRunPreviewRequired: true,
		Plan:                    plan,
		UserMessage:             "예약 발송 등록을 검토했습니다. 실제 발송은 하지 않았습니다.",
		ApprovalNote:            "Registering a scheduled delivery requires execute=true, approve=true, and preview_approved=true after the first-run preview is shown to the user.",
		Next: []string{
			"Show the first-run preview and resolved target before registering.",
			"Register only after execute=true, approve=true, and preview_approved=true.",
			"Scheduled delivery execution remains a separate runner step; this apply step stores the approved job only.",
		},
	}
	if plan.Status == "missing_fields" || plan.Status == "target_review_required" {
		result.Status = plan.Status
		return result, nil
	}
	if !opts.Execute || !opts.Approve || !opts.PreviewApproved {
		result.Status = "approval_required"
		result.ApprovalMissing = true
		return result, nil
	}
	if plan.ResolvedTarget == nil {
		result.Status = "target_review_required"
		return result, nil
	}
	store, err := LoadScheduledDeliveryStore()
	if err != nil {
		result.Status = "store_error"
		return result, err
	}
	job := ScheduledDeliveryJob{
		ID:          scheduledDeliveryJobID(*plan.ResolvedTarget, plan.Schedule, plan.Content, plan.ContentType, plan.Delivery),
		Enabled:     true,
		Status:      "registered",
		TargetID:    plan.ResolvedTarget.ID,
		TargetLabel: plan.ResolvedTarget.Label,
		Schedule:    plan.Schedule,
		Content:     plan.Content,
		ContentType: plan.ContentType,
		Delivery:    plan.Delivery,
		CreatedAt:   now.UTC(),
		UpdatedAt:   now.UTC(),
	}
	replaced := false
	for i := range store.Jobs {
		if store.Jobs[i].ID == job.ID {
			job.CreatedAt = store.Jobs[i].CreatedAt
			store.Jobs[i] = job
			replaced = true
			break
		}
	}
	if !replaced {
		store.Jobs = append(store.Jobs, job)
	}
	store.UpdatedAt = now.UTC()
	if err := SaveScheduledDeliveryStore(store); err != nil {
		result.Status = "store_error"
		return result, err
	}
	result.Status = "registered"
	result.UserMessage = "승인된 예약 발송 작업을 등록했습니다. 이 단계에서는 메시지를 보내지 않았습니다."
	result.Job = &job
	result.Store = &store
	result.Path = store.Path
	return result, nil
}

func normalizeScheduledContentType(value, content string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "message", "report", "voice", "voice_report":
		return value
	}
	lower := strings.ToLower(content)
	switch {
	case strings.Contains(lower, "음성") || strings.Contains(lower, "voice"):
		if strings.Contains(lower, "보고") || strings.Contains(lower, "report") {
			return "voice_report"
		}
		return "voice"
	case strings.Contains(lower, "보고") || strings.Contains(lower, "report"):
		return "report"
	default:
		return "message"
	}
}

func normalizeScheduledDelivery(value, contentType string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "signal", "voice_note", "call":
		return value
	}
	if contentType == "voice" || contentType == "voice_report" {
		return "voice_note"
	}
	return "signal"
}

func LoadScheduledDeliveryStore() (ScheduledDeliveryStore, error) {
	path := scheduledDeliveryStorePath()
	store := ScheduledDeliveryStore{Kind: "meshclaw_scheduled_deliveries", Path: path, Jobs: []ScheduledDeliveryJob{}}
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
	store.Kind = "meshclaw_scheduled_deliveries"
	store.Path = path
	if store.Jobs == nil {
		store.Jobs = []ScheduledDeliveryJob{}
	}
	return store, nil
}

func SaveScheduledDeliveryStore(store ScheduledDeliveryStore) error {
	if strings.TrimSpace(store.Path) == "" {
		store.Path = scheduledDeliveryStorePath()
	}
	store.Kind = "meshclaw_scheduled_deliveries"
	if err := os.MkdirAll(filepath.Dir(store.Path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(store.Path, append(data, '\n'), 0600)
}

func scheduledDeliveryStorePath() string {
	if path := strings.TrimSpace(os.Getenv("MESHCLAW_SCHEDULED_DELIVERIES")); path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return filepath.Join(".meshclaw", "scheduled-deliveries.json")
	}
	return filepath.Join(home, ".meshclaw", "scheduled-deliveries.json")
}

func scheduledDeliveryJobID(target Target, schedule, content, contentType, delivery string) string {
	sum := sha1.Sum([]byte(strings.Join([]string{
		target.ID,
		strings.TrimSpace(schedule),
		strings.TrimSpace(content),
		strings.TrimSpace(contentType),
		strings.TrimSpace(delivery),
	}, "\x00")))
	return fmt.Sprintf("scheduled-%s", hex.EncodeToString(sum[:])[:12])
}
