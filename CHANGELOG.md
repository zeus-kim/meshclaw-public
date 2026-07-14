# Changelog

## Unreleased

- Added risk registers to MCP rollout plans.
- Added MCP command compatibility guidance to rollout plans.
- Added explicit container self-heal apply-loop gates to readiness summaries.
- Added container self-heal logscan likely-cause and suggested-action hints to readiness reports.
- Propagated container self-heal logscan pattern requirements through verification, runbook, rollback, completion, and readiness-summary reports.
- Expanded container self-heal logscan patterns with likely-cause and suggested-action hints.
- Added copy-paste status lines to MCP rollout plans.
- Added operator briefs to MCP rollout plans.
- Added operator acceptance checklists to MCP rollout plans.
- Added client refresh matrix metadata to MCP rollout plans.
- Added readiness labels to MCP rollout plans.
- Added summary counts to MCP rollout plans.
- Added rollback decision points to MCP rollout plans.
- Added stack review discipline to MCP rollout plans.
- Added local verification steps to MCP rollout plans.
- Added operator handoff guidance to MCP rollout plans.
- Added success criteria to MCP rollout plans.
- Added evidence checklists to MCP rollout plans.
- Added troubleshooting guidance to MCP rollout plans.
- Added post-reload smoke prompts to MCP rollout plans.
- Added operator checkpoints to MCP rollout plans.
- Added rollout-readiness summaries to MCP rollout plans.
- Added readiness summaries to MCP smoke-test plans.
- Added approval-boundary metadata to MCP smoke-test plans.
- Added summary counts to MCP smoke-test plans for ready-now, fixture-required, and non-mutating checks.
- Added evidence capture ordering to MCP smoke-test run strategies.
- Added a run strategy to MCP smoke-test plans to separate ready-now checks from fixture-required checks.
- Added a fixture manifest to MCP smoke-test plans for desired-state and autoheal evidence prerequisites.
- Added safe sample arguments and fixture markers to MCP smoke-test plan steps.
- Embedded Claude lite profile visibility previews in default MCP smoke and rollout plans.
- Included desired-state, container self-heal, and logscan tools in default MCP smoke and rollout plans.
- Isolated runtime VSSH unit tests from local inventory state.
- Blocked GUI launcher commands during unit tests to prevent Safari or local apps from opening.
- Exposed log analysis in the Claude lite MCP profile for container-logscan evidence.
- Exposed container self-heal planning tools in the Claude lite MCP profile.
- Exposed desired-state reconcile planning tools in the Claude lite MCP profile.
- Covered full-profile and alias MCP visibility checks.
- Exposed MCP profile visibility checks in the Claude lite profile.
- Included MCP profile visibility checks in rollout plans.
- Included MCP profile visibility checks in smoke-test plans.
- Added a read-only MCP profile visibility check surface.
- Added a plan-only MCP automation-rule writer preview surface.
- Added a summary-only MCP automation-rule readiness surface.
- Added a gate-only MCP automation-rule check surface.
- Added a plan-only MCP automation-rule planning surface.
- Added a plan-only MCP smoke-test planning surface.
- Added a plan-only MCP client rollout planning surface.
- Added a plan-only MCP ops-tool integration planning surface.
- Added a plan-only MCP storage/backup guardrail planning surface.
- Added a plan-only MCP capacity/autoscale planning surface.
- Added a plan-only MCP service registry/LB-lite surface.
- Added MCP surface guidance for model, core runtime, worker, and execution transport boundaries.
- Added semantic evidence contract metadata to MCP tool recommendations.
- Added explicit execution policy metadata to MCP tool recommendations.
- Inferred autoheal-plan evidence JSON paths in MCP container apply-plan recommendations.
- Inferred container completion-plan evidence JSON paths in MCP container readiness-summary recommendations.
- Inferred container rollback-plan evidence JSON paths in MCP container completion-plan recommendations.
- Inferred container runbook-check evidence JSON paths in MCP container rollback-plan recommendations.
- Inferred container runbook evidence JSON paths in MCP container runbook-check recommendations.
- Inferred container verification-plan evidence JSON paths in MCP container runbook recommendations.
- Inferred container apply-plan evidence JSON paths in MCP container verification-plan recommendations.
- Inferred reconcile completion-plan evidence JSON paths in MCP readiness-summary recommendations.
- Inferred reconcile rollback-plan evidence JSON paths in MCP completion-plan recommendations.
- Inferred reconcile runbook-check evidence JSON paths in MCP rollback-plan recommendations.
- Inferred reconcile runbook evidence JSON paths in MCP runbook-check recommendations.
- Inferred reconcile verification-plan evidence JSON paths in MCP runbook recommendations.
- Inferred reconcile apply-plan evidence JSON paths in MCP execution-preview recommendations.
- Inferred reconcile apply-gate evidence JSON paths in MCP apply-plan recommendations.
- Inferred reconcile approval evidence JSON paths in MCP apply-gate recommendations.
- Inferred desired-state YAML paths in MCP apply-gate recommendations.
- Inferred desired-state YAML paths in MCP validate, plan, and approval-request recommendations.
- Recognized `<node> <container> 컨테이너 로그` patterns in MCP container log recommendations.
- Recognized `on/from <host> <container>` patterns in MCP container log recommendations.
- Added MCP recommendation coverage for system-log and ambiguous container-log source boundaries.
- Inferred container log recommendation arguments as `source=container:<name>` when the intent names a container.
- Routed container apply, verification, runbook, rollback, completion, and readiness-summary intents to the MCP container planning chain.
- Routed desired-state runbook-check, rollback, completion, and readiness-summary intents to the MCP reconcile completion chain.
- Routed desired-state apply-plan, execution-preview, and runbook intents to the MCP reconcile review chain.
- Routed desired-state approval-request and apply-gate intents to the MCP reconcile approval surface.
- Routed desired-state YAML validation and reconcile intents to the MCP reconcile planning surface.
- Added desired-state, reconcile, and container self-heal steps to the MCP surface canonical loop.
- Added MCP recommendation coverage separating container log analysis from container self-heal planning.
- Routed container restart/recovery intents to the approval-gated self-heal planning chain in MCP tool recommendations.
- Added MCP catalog default-surface coverage for the container self-heal planning chain.
- Added MCP catalog coverage for the full container self-heal planning chain.
- Added a tested text summary formatter for autoheal-plan output.
- Added a tested text summary formatter for container readiness-summary output.
- Added a tested text summary formatter for container completion-plan output.
- Added a tested text summary formatter for container rollback-plan output.
- Added a tested text summary formatter for container runbook-check output.
- Added a tested text summary formatter for container runbook output.
- Added a tested text summary formatter for container verification-plan output.
- Added a tested text summary formatter for container apply-plan output.
- Printed desired-state schema_version in reconcile readiness-summary text summaries.
- Printed desired-state schema_version in reconcile completion-plan text summaries.
- Printed desired-state schema_version in reconcile rollback-plan text summaries.
- Printed desired-state schema_version in reconcile runbook-check text summaries.
- Printed desired-state schema_version in reconcile runbook text summaries.
- Printed desired-state schema_version in reconcile verification-plan text summaries.
- Printed desired-state schema_version in reconcile execution-preview text summaries.
- Printed desired-state schema_version in reconcile apply-plan text summaries.
- Printed desired-state schema_version in reconcile apply-gate text summaries.
- Printed desired-state schema_version in reconcile approval-request text summaries.
- Printed desired-state schema_version in reconcile text summaries.
- Exposed desired-state schema_version in reconcile readiness-summary reports.
- Exposed desired-state schema_version in reconcile completion-plan reports.
- Exposed desired-state schema_version in reconcile rollback-plan reports.
- Exposed desired-state schema_version in reconcile runbook-check reports.
- Exposed desired-state schema_version in reconcile runbook reports.
- Exposed desired-state schema_version in reconcile verification-plan reports.
- Exposed desired-state schema_version in reconcile execution-preview reports.
- Exposed desired-state schema_version in reconcile apply-plan reports.
- Exposed desired-state schema_version in reconcile apply-gate reports.
- Exposed desired-state schema_version in reconcile approval requests.
- Exposed desired-state schema_version in reconcile plan reports.
- Exposed desired-state schema_version in reconcile validation reports.
- Accepted optional desired-state schema_version values in reconcile YAML parsing.
- Printed reconcile readiness closeout gates in text summaries.
- Printed container readiness closeout gates in text summaries.
- Printed reconcile readiness operator-result gates in text summaries.
- Printed container readiness operator-result gates in text summaries.
- Printed reconcile readiness post-apply gates in text summaries.
- Printed container readiness post-apply gates in text summaries.
- Printed reconcile readiness operator-apply gates in text summaries.
- Printed container readiness operator-apply gates in text summaries.
- Printed reconcile readiness operator-handoff gates in text summaries.
- Printed container readiness operator-handoff gates in text summaries.
- Printed reconcile readiness stage-readiness gates in text summaries.
- Printed container readiness stage-readiness gates in text summaries.
- Printed reconcile readiness completion-readiness gates in text summaries.
- Printed container readiness completion-readiness gates in text summaries.
- Printed reconcile readiness verification-readiness gates in text summaries.
- Printed container readiness verification-readiness gates in text summaries.
- Printed reconcile readiness rollback-readiness gates in text summaries.
- Printed container readiness rollback-readiness gates in text summaries.
- Printed reconcile readiness evidence gates in text summaries.
- Printed container readiness evidence gates in text summaries.
- Printed reconcile readiness blocker gates in text summaries.
- Printed container readiness blocker gates in text summaries.
- Printed reconcile readiness review gates in text summaries.
- Printed container readiness review gates in text summaries.
- Printed reconcile readiness dry-run gates in text summaries.
- Printed container readiness dry-run gates in text summaries.
- Printed reconcile readiness approval gates in text summaries.
- Printed container readiness approval gates in text summaries.
- Printed reconcile readiness MCP gates in text summaries.
- Printed container readiness MCP gates in text summaries.
- Printed reconcile readiness executor gates in text summaries.
- Printed container readiness executor gates in text summaries.
- Printed reconcile readiness promotion gates in text summaries.
- Printed container readiness promotion gates in text summaries.
- Printed reconcile readiness handoff gates in text summaries.
- Printed container readiness handoff gates in text summaries.
- Printed reconcile readiness audit gates in text summaries.
- Printed container readiness audit gates in text summaries.
- Printed reconcile readiness execution gates in text summaries.
- Printed container readiness execution gates in text summaries.
- Printed reconcile readiness operator gates in text summaries.
- Printed container readiness operator gates in text summaries.
- Printed reconcile readiness freshness gates in text summaries.
- Printed container readiness freshness gates in text summaries.
- Printed reconcile readiness policy gates in text summaries.
- Printed container readiness policy gates in text summaries.
- Printed reconcile readiness rollback gates in text summaries.
- Printed container readiness rollback gates in text summaries.
- Printed reconcile readiness completion gates in text summaries.
- Printed container readiness completion gates in text summaries.
- Printed reconcile readiness verification gates in text summaries.
- Printed container readiness verification gates in text summaries.
- Printed reconcile readiness apply gates in text summaries.
- Printed container readiness apply gates in text summaries.
- Printed reconcile readiness overview lines in text summaries.
- Printed container readiness overview lines in text summaries.
- Printed reconcile readiness generation timestamps in text summaries.
- Printed container readiness generation timestamps in text summaries.
- Printed reconcile readiness modes in text summaries.
- Printed container readiness modes in text summaries.
- Printed reconcile readiness evidence paths in text summaries.
- Printed container readiness evidence paths in text summaries.
- Printed reconcile readiness evidence links in text summaries.
- Printed container readiness evidence links in text summaries.
- Printed reconcile readiness decisions in text summaries.
- Printed container readiness decisions in text summaries.
- Labeled reconcile readiness blockers in text summaries.
- Labeled container readiness blockers in text summaries.
- Printed reconcile readiness next actions in text summaries.
- Printed container readiness next actions in text summaries.
- Printed reconcile readiness safety gates in text summaries.
- Printed container readiness safety gates in text summaries.
- Printed reconcile readiness stage names in text summaries.
- Printed container readiness stage names in text summaries.
- Included reconcile readiness blocker counts in summary count lines.
- Included container readiness blocker counts in summary count lines.
- Counted missing reconcile completion requirements in readiness summaries.
- Counted missing container completion requirements in readiness summaries.
- Counted blocked reconcile completion plans in readiness summaries.
- Counted blocked container completion plans in readiness summaries.
- Counted runbook-ready findings in reconcile runbook checks and readiness summaries.
- Counted runbook-ready findings in container runbook checks and readiness summaries.
- Counted steps findings in reconcile runbook checks and readiness summaries.
- Counted steps findings in container runbook checks and readiness summaries.
- Counted target findings in reconcile runbook checks and readiness summaries.
- Counted target findings in container runbook checks and readiness summaries.
- Counted requires-verification findings in reconcile runbook checks and readiness summaries.
- Counted requires-verification findings in container runbook checks and readiness summaries.
- Counted failure-action findings in reconcile runbook checks and readiness summaries.
- Counted failure-action findings in container runbook checks and readiness summaries.
- Counted success-criteria findings in reconcile runbook checks and readiness summaries.
- Counted success-criteria findings in container runbook checks and readiness summaries.
- Counted required-evidence findings in reconcile runbook checks and readiness summaries.
- Counted required-evidence findings in container runbook checks and readiness summaries.
- Blocked container runbooks when steps omit container-logscan evidence.
- Covered container command reference matching for quoted values and prefix rejection.
- Blocked container runbooks when command templates do not reference the step container.
- Included command-template finding counts in container readiness summary count lines.
- Preserved command-template finding counts in container readiness summaries.
- Counted command-template findings in container runbook checks.
- Included runbook finding counts in container readiness summary count lines.
- Included command-template finding counts in reconcile readiness summary count lines.
- Preserved command-template finding counts in reconcile readiness summaries.
- Included runbook finding counts in reconcile readiness summary count lines.
- Preserved container runbook finding counts in reconcile readiness summaries.
- Counted container-specific findings in reconcile runbook checks.
- Counted command-template findings in reconcile runbook checks.
- Covered reconcile command flag value matching for quoted values and prefix rejection.
- Blocked reconcile runbooks when action-id command flags differ from action metadata.
- Blocked reconcile runbooks when container command templates target different container metadata.
- Blocked reconcile runbooks when step command templates target a different operation.
- Tightened reconcile runbook node flag matching so prefixed node ids do not pass the gate.
- Blocked reconcile runbooks when step command templates target a different node id.
- Blocked reconcile runbooks when step command templates omit the action-id flag.
- Blocked reconcile runbooks when step command templates omit the require-evidence flag.
- Blocked reconcile runbooks when container steps omit required container-logscan evidence.
- Blocked reconcile runbooks when container desired metadata lacks matching success criteria.
- Added desired container metadata-specific success criteria to reconcile verification plans.
- Rendered actual container state, image, and health metadata in reconcile command previews.
- Rendered actual restart policy metadata in reconcile command previews.
- Covered actual restart policy metadata in MCP reconcile container flow.
- Propagated actual container restart policy metadata through reconcile planning stages.
- Collected Docker restart policy and planned desired restart-policy drift.
- Covered health-only desired container drift in the reconciler.
- Allowed desired-state containers to specify health or restart policy without also requiring state/image.
- Warned on unsupported service desired states in desired-state YAML.
- Warned when desired-state YAML marks a container absent while also specifying image or health.
- Documented focused container-logscan evidence in autoheal container MCP tool descriptions.
- Aligned autoheal container lifecycle tests with focused container-logscan verification evidence.
- Covered desired/actual container metadata propagation through reconcile runbook, rollback, and completion plans.
- Documented and tested MCP reconcile container desired metadata flow.
- Propagated desired container state, image, and health metadata through reconcile previews and verification plans.
- Required focused container logscan evidence in reconcile container verification plans.
- Treated desired container state `present` as an existence check instead of forcing `running`.
- Required container logscan evidence in container verification plans.
- Documented `container:<name>` log sources in MCP analyze-logs metadata and recommendations.
- Recommended focused container logscan MCP calls for container restart proposals.
- Added `container:<name>` log analysis support and container logscan verification hints.
- Covered desired container restart metadata propagation through reconcile runbook, rollback, and completion stages.
- Included desired container restart policy in reconcile execution-preview command templates.
- Propagated desired-state container restart policy through reconcile preview and verification metadata.
- Propagated desired-state container restart policy into reconcile action metadata.
- Validated desired-state container restart policies.
- Printed reconcile plan counts in non-JSON CLI output.
- Added structured reconcile plan counts to report, MCP payloads, and evidence summaries.
- Returned autoheal apply-safe counts from MCP apply-safe responses.
- Printed autoheal apply-safe gated result counts in CLI/evidence summaries.
- Added autoheal apply-safe action count summarization for gated results.
- Covered autoheal apply-safe policy and mode gates for skipped actions.
- Covered container readiness-summary counts in MCP readiness tests.
- Printed key container readiness-summary counts in non-JSON CLI output.
- Added readiness stage and blocker counts to container readiness-summary reports.
- Covered reconcile readiness-summary counts in MCP readiness tests.
- Printed key reconcile readiness-summary counts in non-JSON CLI output.
- Added readiness stage and blocker counts to reconcile readiness-summary reports.
- Propagated completion counts into reconcile readiness-summary reports with summary gate counts.
- Added completion and container completion counts to reconcile completion-plan reports.
- Added rollback and container rollback counts to reconcile rollback-plan reports.
- Propagated runbook counts into reconcile runbook-check reports with gate counts.
- Propagated verification counts into reconcile runbook reports with runbook step counts.
- Added verification and container verification counts to reconcile verification-plan reports.
- Added command and container command counts to reconcile execution-preview reports.
- Added apply-step and container apply-step counts to reconcile apply-plan reports.
- Added approval-request count propagation into reconcile apply-gate reports.
- Added container-specific counts to reconcile approval-request reports.
- Added metadata to desired-state container drift actions and propagated it into
  reconcile apply-plan steps.
- Added container-specific reconcile readiness-summary counts across apply,
  preview, verification, runbook, rollback, and completion stages.
- Propagated desired-state container metadata into reconcile rollback and
  completion planning.
- Propagated desired-state container metadata through reconcile preview,
  verification, runbook, and runbook-check stages.
- Added container-specific reconcile verification requirements for desired-state
  container drift actions.
- Added container-specific reconcile apply-plan metadata and execution preview
  hints for desired-state container drift actions.
- Added plan-only reconcile drift detection for desired-state containers
  against observed Docker container state.
- Added optional desired-state YAML `containers` parsing and validation for
  future approval-gated container reconciliation.
- Added summary-only container readiness-summary CLI/MCP surfaces from
  container completion-plan evidence.
- Added plan-only container completion-plan CLI/MCP surfaces from ready
  container rollback-plan evidence.
- Added plan-only container rollback-plan CLI/MCP surfaces from ready
  container runbook-check evidence.
- Added gate-only container runbook-check CLI/MCP surfaces that validate
  container remediation runbooks before any future executor consumes them.
- Added review-only container runbook CLI/MCP surfaces for approved container
  remediation plans.
- Added container verification-plan CLI/MCP surfaces that define post-action
  evidence checks for approved container apply plans.
- Added approval-gated container apply-plan CLI/MCP surfaces that turn
  autoheal-plan evidence into inert container restart step templates.
- Added cached node-report container health candidates to `autoheal-plan` as
  approval-gated propose-only actions.
- Added propose-only container health autoheal planning helpers for unhealthy,
  exited, OOM-killed, and high-restart Docker containers.
- Added optional node report `logs.log_findings` entries powered by
  `internal/logscan`.
- Wired `internal/logscan` findings into existing `analyze-logs` workflow
  reports as optional `log_findings` evidence.
- Added `internal/logscan` analysis core for redacted OOM, crash-loop, auth
  failure, and HTTP 5xx log findings.
- Added reconcile validate-desired CLI/MCP surfaces that store desired-state
  YAML validation findings before plan/apply readiness.
- Added reconcile readiness-summary CLI/MCP surfaces that summarize
  approval-to-completion readiness from completion-plan evidence without
  executing changes.
- Added reconcile completion-plan CLI/MCP surfaces that define final evidence
  requirements before a future reconcile loop can be marked complete.
- Added reconcile rollback-plan CLI/MCP surfaces that derive rollback guidance
  from ready runbook-check evidence without executing changes.
- Added reconcile runbook-check CLI/MCP surfaces that validate review-only
  runbooks before any future executor consumes them.
- Added reconcile runbook CLI/MCP surfaces that combine verification-plan
  evidence into review-only operator steps.
- Added reconcile verification-plan CLI/MCP surfaces that turn execution
  preview evidence into post-action evidence requirements.
- Added desired-state reconcile readiness summary to the default MCP rollout and
  smoke-test chains so post-reload checks verify executor-contract visibility
  before future reconcile apply paths.
- Added an executor gate contract to desired-state reconcile readiness summaries
  so future apply loops can read required evidence, forbidden actions, and
  refresh triggers before any approval-gated reconcile executor is considered.
- Added container readiness summary to the default MCP rollout expected-tool
  chain so post-reload smoke prompts verify apply-loop gate and executor
  contract visibility before future apply paths.
- Added an executor gate contract to container self-heal readiness summaries so
  future apply loops can read required evidence, forbidden actions, and refresh
  triggers before any approval-gated executor is considered.
- Added MCP recommendation and smoke-test coverage for container self-heal
  apply-loop gate readiness, keeping the surface summary-only and fixture-gated.
- Added reconcile execution-preview CLI/MCP surfaces that render inert command
  templates from apply-plan evidence without running server changes.
- Added reconcile apply-plan CLI/MCP surfaces that turn ready apply-gate
  evidence into structured, non-executing apply steps.
- Added reconcile apply-gate CLI/MCP surfaces that validate approval evidence
  before any future mutating reconcile apply loop.
- Added reconcile approval request CLI/MCP surfaces that turn policy-gated
  reconcile actions into evidence without executing server changes.
- Added `meshclaw reconcile run-once --dry-run` and
  `meshclaw_reconcile_run_once` MCP dry-run surfaces, with execute/apply still
  rejected.
- Added policy decision annotations to reconcile dry-run actions so future
  apply loops can stop on approval-required or denied actions.
- Added optional local node-report actual-state binding for reconcile dry-run
  plans in CLI and MCP surfaces.
- Added `meshclaw_reconcile_plan` MCP tool for dry-run desired-state
  reconciliation plans with evidence and explicit apply/execute rejection.
- Added a dry-run `meshclaw reconcile plan --desired <file>` CLI surface that
  parses desired-state YAML, emits plan results, and stores evidence without
  applying changes.
- Added desired-state YAML parsing for reconciliation nodes, roles, tags,
  service intent, and capacity thresholds.
- Added node report schema v3 container health diagnostics: Docker inspect
  health status, restart/OOM/exit metadata, and health warning rollups.
- Started Unified Publication & Actions U1/U2 by extracting Argos news Markdown
  document rendering/saving into `internal/publish` and exposing the first shared
  MCP publication tool, `meshclaw_news_document`.
- Added the first unified research publication MVP, `meshclaw_argos_research`,
  which writes Work Reports Markdown, mobile HTML previews, and private document
  links through the shared publish package.
- Added a U2 remote MCP bridge for publication tools: MacBook MCP callers default
  to `target=macmini`, while `target=local` remains available for development.
- Consolidated remaining Argos search document and public-link helpers in
  `internal/osauto` and `cmd/meshclaw` onto `internal/publish`.
- Started U4 briefing migration with a `meshclaw briefing` command surface that
  forwards `morning`, `news`, `evening`, `menu`, and RSS daily-news flows to the
  existing Go briefing implementations, plus updated wrapper scripts to call it.
- Added `meshclaw briefing local-ai` as a transitional wrapper for the richer
  `~/.meshclaw/bin/local_ai_daily_briefing.py` pipeline so launchd can migrate
  to a meshclaw-owned command surface before the Python implementation is fully
  absorbed.
- Added `scripts/local-ai-briefing.sh` and moved the evening wrapper to
  `meshclaw briefing evening`, leaving active LaunchAgents untouched until a
  verified deploy/migration window.
- Added a staged `meshclaw daemon install local-ai-briefing` launchd path that
  generates a plist invoking `meshclaw briefing local-ai`, so the active
  MacBook briefing LaunchAgent can be migrated away from direct Python once
  verified.
- Added help and `--check` validation for the transitional local AI briefing
  command surface, so operators can verify the Python script path before any
  LaunchAgent migration.
- Added staged plist path options for `meshclaw daemon install
  local-ai-briefing`, allowing dry-run LaunchAgent rendering without touching
  the active MacBook briefing plist.
- Started Mission Phase 3 Core writes with MacBook-only MCP tools:
  `mission_update`, `task_add`, `task_complete`, and `artifact_add`, plus
  atomic JSON file writes in `internal/mission`.
- Started U5 artifact linking by letting unified publication MCP tools append
  produced document/report refs to MacBook Mission artifacts while suppressing
  remote macmini Mission writes.
