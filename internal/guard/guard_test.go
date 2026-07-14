package guard

import (
	"strings"
	"testing"
)

func TestDetectRedactsKnownSecrets(t *testing.T) {
	fakeToken := "ghp_" + "abcdefghijklmnopqrstuvwxyz123456"
	report := Detect("chat", "deploy with token="+fakeToken)
	if report.Status != "findings" {
		t.Fatalf("status=%s want findings", report.Status)
	}
	if len(report.Findings) == 0 {
		t.Fatalf("expected findings")
	}
	if strings.Contains(report.Redacted, "abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("redacted output leaked token: %s", report.Redacted)
	}
	if !strings.Contains(report.Redacted, "<SECRET:") {
		t.Fatalf("expected replacement marker: %s", report.Redacted)
	}
}

func TestDetectCleanText(t *testing.T) {
	report := Detect("chat", "hello, check service status")
	if report.Status != "clean" {
		t.Fatalf("status=%s want clean", report.Status)
	}
	if len(report.Findings) != 0 {
		t.Fatalf("findings=%d want 0", len(report.Findings))
	}
}

func TestModesExposeThreeGuardModes(t *testing.T) {
	modes := Modes()
	if len(modes) != 3 {
		t.Fatalf("modes=%d want 3", len(modes))
	}
	got := map[Mode]bool{}
	for _, mode := range modes {
		got[mode.Mode] = true
	}
	for _, want := range []Mode{ModeCredential, ModePosture, ModeVuln} {
		if !got[want] {
			t.Fatalf("missing mode %s", want)
		}
	}
}

func TestAnthropicKeyDetectedBeforeOpenAIKey(t *testing.T) {
	fakeKey := "sk-ant-" + "api03-abcdefghijklmnopqrstuvwxyz1234567890"
	report := Detect("chat", "key="+fakeKey)
	if len(report.Findings) == 0 {
		t.Fatalf("expected finding")
	}
	if report.Findings[0].Kind != "anthropic_key" {
		t.Fatalf("kind=%s want anthropic_key", report.Findings[0].Kind)
	}
}

func TestLocalModelPlanKeepsGuardAuthoritative(t *testing.T) {
	plan := LocalModel()
	if plan.Name == "" || len(plan.ModelMustNotDo) == 0 || len(plan.GuardAuthority) == 0 {
		t.Fatalf("incomplete local model plan: %+v", plan)
	}
	joined := strings.Join(plan.RequiredSettings, " ")
	if !strings.Contains(joined, "memory") || !strings.Contains(joined, "RAG") {
		t.Fatalf("expected memory and RAG settings: %s", joined)
	}
}

func TestDetectDoesNotReclassifyReplacementMarkers(t *testing.T) {
	fakeToken := "ghp_" + "abcdefghijklmnopqrstuvwxyz123456"
	report := Detect("chat", "token="+fakeToken+" password=dragon1234")
	if len(report.Findings) != 2 {
		t.Fatalf("findings=%d want 2: %+v", len(report.Findings), report.Findings)
	}
	if strings.Contains(report.Redacted, "ghp_") || strings.Contains(report.Redacted, "dragon1234") {
		t.Fatalf("redacted output leaked raw value: %s", report.Redacted)
	}
}

func TestVaultConversationPlanIsLocalOnly(t *testing.T) {
	plan := VaultConversationPlan()
	if !plan.LocalOnly {
		t.Fatalf("vault conversation must be local only")
	}
	if len(plan.Operations) == 0 || !strings.HasPrefix(plan.HandleScheme, "vault://") {
		t.Fatalf("incomplete vault plan: %+v", plan)
	}
}

func TestGuardSessionPolicyDeniesCloudAndMemory(t *testing.T) {
	policy := GuardSessionPolicy()
	if policy.Memory != "disabled" || policy.RAG != "disabled" {
		t.Fatalf("memory/rag must be disabled: %+v", policy)
	}
	joined := strings.Join(policy.DeniedSurfaces, " ")
	if !strings.Contains(joined, "Claude") || !strings.Contains(joined, "Matrix") {
		t.Fatalf("cloud/bridge surfaces should be denied: %s", joined)
	}
}

func TestSignalGuardPolicyAllowsReportsButNotRawIngress(t *testing.T) {
	policy := SignalGuardPolicy()
	if len(policy.AllowedReports) == 0 {
		t.Fatalf("Signal policy should allow redacted reports")
	}
	forbidden := strings.Join(policy.ForbiddenFlows, " ")
	if !strings.Contains(forbidden, "Claude/Codex reads") || !strings.Contains(forbidden, "raw") {
		t.Fatalf("Signal policy should forbid external LLM raw access: %s", forbidden)
	}
}

func TestClientGuidesCoverLocalAndCloudClients(t *testing.T) {
	guides := ClientGuides()
	if len(guides) < 5 {
		t.Fatalf("guides=%d want at least 5", len(guides))
	}
	seen := map[string]ClientGuide{}
	for _, guide := range guides {
		seen[guide.ID] = guide
	}
	for _, id := range []string{"ollama-cli", "open-webui", "lm-studio", "cloud-ai"} {
		if seen[id].ID == "" {
			t.Fatalf("missing client guide %s", id)
		}
	}
	if seen["cloud-ai"].Risk != "high" {
		t.Fatalf("cloud clients must be high risk: %+v", seen["cloud-ai"])
	}
	if len(seen["open-webui"].LocalStores) == 0 {
		t.Fatalf("open-webui guide should expose local stores")
	}
}

func TestPostureIncludesClientChatStores(t *testing.T) {
	report := Posture("/tmp/meshclaw-guard-test-home")
	var found bool
	for _, check := range report.Checks {
		if strings.Contains(check.ID, "open-webui") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("posture checks should include Open WebUI stores: %+v", report.Checks)
	}
}

func TestParseIntentClassifiesVaultAndPostureRequests(t *testing.T) {
	store := ParseIntent("이 Cloudflare 토큰 저장해줘")
	if store.Intent != "vault_import" || !store.RequiresLocalOnly {
		t.Fatalf("store intent mismatch: %+v", store)
	}
	posture := ParseIntent("로컬 모델 memory rag 상태 점검")
	if posture.Intent != "guard_posture" || posture.Mode != ModePosture {
		t.Fatalf("posture intent mismatch: %+v", posture)
	}
	remove := ParseIntent("github token 삭제")
	if remove.Intent != "vault_delete" || !remove.ApprovalRequired {
		t.Fatalf("delete intent mismatch: %+v", remove)
	}
}

func TestCleanupPlanIncludesApprovalGatedChatStores(t *testing.T) {
	plan := Cleanup("/tmp/meshclaw-guard-test-home")
	if len(plan.Candidates) == 0 {
		t.Fatalf("expected cleanup candidates")
	}
	for _, candidate := range plan.Candidates {
		if candidate.Exists && candidate.Approval != "require_strong_approval" {
			t.Fatalf("existing cleanup candidate should require approval: %+v", candidate)
		}
	}
}
