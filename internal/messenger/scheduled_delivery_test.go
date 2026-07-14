package messenger

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meshclaw/meshclaw/internal/osauto"
)

func TestPlanScheduledDeliveryResolvesTargetAndDoesNotExecute(t *testing.T) {
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(t.TempDir(), "targets.json"))
	if _, _, err := UpsertTarget(Target{ID: "yun", Channel: "signal", Recipient: "+821012345678", Label: "윤", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}

	plan := PlanScheduledDelivery(ScheduledDeliveryPlanOptions{
		Target:      "윤",
		Schedule:    "매주 금요일 오전 9시",
		Content:     "주간 기도문 음성 보고",
		ContentType: "voice_report",
		Now:         time.Date(2026, 6, 3, 5, 0, 0, 0, time.UTC),
	})
	if plan.Kind != "meshclaw_scheduled_delivery_plan" || plan.Action != "scheduled_delivery_plan" {
		t.Fatalf("unexpected kind/action: %#v", plan)
	}
	if plan.Status != "review_ready" || plan.Execute || plan.Approved {
		t.Fatalf("plan should be review-only: %#v", plan)
	}
	if plan.ResolvedTarget == nil || plan.ResolvedTarget.ID != "yun" {
		t.Fatalf("target not resolved: %#v", plan)
	}
	if plan.Delivery != "voice_note" || !plan.FirstRunPreviewRequired {
		t.Fatalf("voice report should default to voice-note preview: %#v", plan)
	}
}

func TestPlanScheduledDeliveryExecutionRequiresApproval(t *testing.T) {
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(t.TempDir(), "targets.json"))
	if _, _, err := UpsertTarget(Target{ID: "report-room", Channel: "signal", GroupID: "group-123", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}

	plan := PlanScheduledDelivery(ScheduledDeliveryPlanOptions{
		Target:   "보고방",
		Schedule: "매일 오전 8시",
		Content:  "오늘 업무 보고",
		Execute:  true,
	})
	if plan.Status != "approval_required" || !plan.ApprovalMissing {
		t.Fatalf("execute should require approval: %#v", plan)
	}
	if plan.ResolvedTarget == nil || plan.ResolvedTarget.ID != "report-room" {
		t.Fatalf("target not resolved: %#v", plan)
	}
}

func TestPlanScheduledDeliveryMissingFields(t *testing.T) {
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(t.TempDir(), "targets.json"))
	plan := PlanScheduledDelivery(ScheduledDeliveryPlanOptions{Target: "윤"})
	if plan.Status != "missing_fields" {
		t.Fatalf("missing fields should be structured: %#v", plan)
	}
	if len(plan.Next) < 3 {
		t.Fatalf("missing next guidance: %#v", plan)
	}
}

func TestPlanScheduledDeliveryUnknownTargetNeedsReview(t *testing.T) {
	oldSearch := assistantSearchContacts
	assistantSearchContacts = func(ctx context.Context, query string) osauto.Result {
		return osauto.Result{
			OK:     false,
			Error:  "mocked no contacts",
			Stdout: "",
		}
	}
	defer func() { assistantSearchContacts = oldSearch }()

	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(t.TempDir(), "targets.json"))
	if _, _, err := UpsertTarget(Target{
		ID:        "yun",
		Channel:   "signal",
		Recipient: "+821012345678",
		Label:     "윤",
		Mode:      "briefing",
	}); err != nil {
		t.Fatal(err)
	}

	plan := PlanScheduledDelivery(ScheduledDeliveryPlanOptions{
		Target:   "unknown-target-x",
		Schedule: "매일 오전 8시",
		Content:  "업무 브리핑",
	})
	if plan.Status != "target_review_required" {
		t.Fatalf("unknown target should require review: %#v", plan)
	}
	if plan.ResolvedTarget != nil {
		t.Fatalf("expected unresolved target: %#v", plan)
	}
	if !strings.Contains(plan.TargetError, "no Signal target or exact contact phone matches") {
		t.Fatalf("expected informative target error: %#v", plan)
	}
}

func TestApplyScheduledDeliveryRequiresPreviewApproval(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(dir, "targets.json"))
	storePath := filepath.Join(dir, "scheduled.json")
	t.Setenv("MESHCLAW_SCHEDULED_DELIVERIES", storePath)
	if _, _, err := UpsertTarget(Target{ID: "report-room", Channel: "signal", GroupID: "group-123", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}

	result, err := ApplyScheduledDelivery(ScheduledDeliveryApplyOptions{
		Target:      "보고방",
		Schedule:    "매일 오전 8시",
		Content:     "오늘 업무 보고",
		ContentType: "voice_report",
		Execute:     true,
		Approve:     true,
		Now:         time.Date(2026, 6, 3, 5, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("ApplyScheduledDelivery error = %v", err)
	}
	if result.Status != "approval_required" || !result.ApprovalMissing {
		t.Fatalf("apply should require preview approval: %#v", result)
	}
	if _, err := os.Stat(storePath); !os.IsNotExist(err) {
		t.Fatalf("store should not be written without preview approval, stat err=%v", err)
	}
}

func TestApplyScheduledDeliveryRegistersApprovedJobWithoutSending(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(dir, "targets.json"))
	storePath := filepath.Join(dir, "scheduled.json")
	t.Setenv("MESHCLAW_SCHEDULED_DELIVERIES", storePath)
	if _, _, err := UpsertTarget(Target{ID: "report-room", Channel: "signal", GroupID: "group-123", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}

	result, err := ApplyScheduledDelivery(ScheduledDeliveryApplyOptions{
		Target:          "보고방",
		Schedule:        "매일 오전 8시",
		Content:         "오늘 업무 보고",
		ContentType:     "voice_report",
		Execute:         true,
		Approve:         true,
		PreviewApproved: true,
		Now:             time.Date(2026, 6, 3, 5, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("ApplyScheduledDelivery error = %v", err)
	}
	if result.Status != "registered" || result.Job == nil {
		t.Fatalf("approved apply should register job only: %#v", result)
	}
	if result.Job.TargetID != "report-room" || !result.Job.Enabled || result.Job.Delivery != "voice_note" {
		t.Fatalf("unexpected job: %#v", result.Job)
	}
	store, err := LoadScheduledDeliveryStore()
	if err != nil {
		t.Fatalf("LoadScheduledDeliveryStore error = %v", err)
	}
	if store.Path != storePath || len(store.Jobs) != 1 {
		t.Fatalf("unexpected store: %#v", store)
	}
}
