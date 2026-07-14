#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

log_dir="${MESHCLAW_DEV_LOOP_LOG_DIR:-$root/.meshclaw-dev-loop}"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
iter_dir="$log_dir/assistant-release-regression-$stamp"
dev_fast="${MESHCLAW_ASSISTANT_RELEASE_DEV_FAST:-1}"
require_clean="${MESHCLAW_ASSISTANT_RELEASE_REQUIRE_CLEAN:-0}"

export MESHCLAW_SHOPPING_BROWSER_DISABLE="${MESHCLAW_SHOPPING_BROWSER_DISABLE:-1}"
export MESHCLAW_SHOPPING_SEARCH_DISABLE="${MESHCLAW_SHOPPING_SEARCH_DISABLE:-1}"

mkdir -p "$iter_dir"

is_true() {
  case "${1:-}" in
    1|true|TRUE|True|yes|YES|Yes|on|ON|On) return 0 ;;
    *) return 1 ;;
  esac
}

record_step() {
  local name="$1"
  shift
  local out="$iter_dir/$name.out"
  local err="$iter_dir/$name.err"
  local status="$iter_dir/$name.status"
  local cmd="$iter_dir/$name.cmd"
  local code=0

  echo "$name" >> "$iter_dir/step-order.txt"
  printf '%q ' "$@" > "$cmd"
  printf '\n' >> "$cmd"
  printf '==> %s\n' "$name"

  if "$@" > "$out" 2> "$err"; then
    code=0
  else
    code=$?
  fi
  printf '%s\n' "$code" > "$status"
}

: > "$iter_dir/step-order.txt"

record_step "assistant-e2e-smoke" scripts/assistant-e2e-smoke.sh

if is_true "$dev_fast"; then
  record_step "dev-fast-no-deploy" env MESHCLAW_FAST_INSTALL_LOCAL=0 MESHCLAW_FAST_DEPLOY=0 scripts/dev-fast.sh
fi

record_step "release-dirty-audit" scripts/release-dirty-audit.sh "$iter_dir/dirty-status.txt"
git status --short > "$iter_dir/git-status.txt"

python3 - "$iter_dir" "$stamp" "$dev_fast" "$require_clean" <<'PY'
import json
import sys
from pathlib import Path

iter_dir = Path(sys.argv[1])
stamp = sys.argv[2]
dev_fast = sys.argv[3]
require_clean = sys.argv[4] in {"1", "true", "TRUE", "True", "yes", "YES", "Yes", "on", "ON", "On"}

step_names = [
    line.strip()
    for line in (iter_dir / "step-order.txt").read_text(errors="replace").splitlines()
    if line.strip()
]

steps = []
for name in step_names:
    status_path = iter_dir / f"{name}.status"
    code = int(status_path.read_text(errors="replace").strip() or "1")
    stdout = (iter_dir / f"{name}.out").read_text(errors="replace")
    stderr = (iter_dir / f"{name}.err").read_text(errors="replace")
    command = (iter_dir / f"{name}.cmd").read_text(errors="replace").strip()
    steps.append(
        {
            "name": name,
            "ok": code == 0,
            "exit_code": code,
            "command": command,
            "stdout_path": str(iter_dir / f"{name}.out"),
            "stderr_path": str(iter_dir / f"{name}.err"),
            "stdout_preview": stdout[-2000:],
            "stderr_preview": stderr[-2000:],
        }
    )

status_lines = [
    line
    for line in (iter_dir / "git-status.txt").read_text(errors="replace").splitlines()
    if line.strip()
]
tracked_changed = sum(1 for line in status_lines if not line.startswith("??"))
untracked = sum(1 for line in status_lines if line.startswith("??"))
total_dirty = len(status_lines)
regression_passed = all(step["ok"] for step in steps)
release_ready = regression_passed and total_dirty == 0
ok = regression_passed and (release_ready or not require_clean)

next_actions = []
if not regression_passed:
    for step in steps:
        if not step["ok"]:
            next_actions.append(
                {
                    "id": f"inspect_{step['name']}",
                    "severity": "regression_blocker",
                    "summary": f"assistant release regression failed at {step['name']}",
                    "artifact": step["stderr_path"] if step.get("stderr_preview") else step["stdout_path"],
                }
            )
if regression_passed and total_dirty:
    next_actions.append(
        {
            "id": "clean_or_accept_dirty_tree",
            "severity": "release_blocker",
            "summary": f"working tree has {total_dirty} dirty entries; release-ready build needs intentional commit/stash/acceptance",
            "artifact": str(iter_dir / "dirty-status.txt"),
        }
    )
if regression_passed and not total_dirty:
    next_actions.append(
        {
            "id": "release_ready",
            "severity": "info",
            "summary": "assistant regression is clean and the working tree is release-ready",
            "artifact": str(iter_dir / "assistant-release-regression.json"),
        }
    )

payload = {
    "ok": ok,
    "kind": "assistant_release_regression",
    "stamp_utc": stamp,
    "artifact_dir": str(iter_dir),
    "regression_passed": regression_passed,
    "release_ready": release_ready,
    "require_clean": require_clean,
    "dev_fast_requested": dev_fast,
    "covered_surface": [
        "skill marketplace/install/activation UX",
        "safe user-created skill registration and testing",
        "memory and personalization reflected in Signal replies",
        "skill search/recommendation/detail/review cards",
        "skill next-step card for marketplace/install/activation guidance",
        "generic final approval gate and execute=false preflight packet for irreversible actions",
        "approval queue visibility for pending mail-send and Mac action candidates",
        "shopping/Coupang browser execution disabled by default in dev and regression loops",
        "dev-fast focused build/test gate with MacBook local install disabled",
    ],
    "dirty": {
        "total": total_dirty,
        "tracked_changed": tracked_changed,
        "untracked": untracked,
        "status_path": str(iter_dir / "dirty-status.txt"),
        "git_status_path": str(iter_dir / "git-status.txt"),
    },
    "steps": steps,
    "next_actions": next_actions,
}

(iter_dir / "assistant-release-regression.json").write_text(
    json.dumps(payload, ensure_ascii=False, indent=2) + "\n"
)

lines = [
    "# Assistant Release Regression",
    "",
    f"- stamp_utc: {stamp}",
    f"- status: {'PASS' if ok else 'FAIL'}",
    f"- regression_passed: {str(regression_passed).lower()}",
    f"- release_ready: {str(release_ready).lower()}",
    f"- require_clean: {str(require_clean).lower()}",
    f"- dev_fast_requested: {dev_fast}",
    f"- dirty_total: {total_dirty}",
    f"- tracked_changed: {tracked_changed}",
    f"- untracked: {untracked}",
    f"- artifact_dir: `{iter_dir}`",
    "",
    "## Covered Surface",
]
for item in payload["covered_surface"]:
    lines.append(f"- {item}")
lines.extend(["", "## Steps"])
for step in steps:
    lines.append(f"- {step['name']}: {'PASS' if step['ok'] else 'FAIL'} (exit={step['exit_code']})")
lines.extend(["", "## Next Actions"])
for action in next_actions:
    lines.append(f"- [{action['severity']}] {action['summary']}")
    lines.append(f"  artifact: `{action['artifact']}`")
lines.extend(
    [
        "",
        "## Artifacts",
        f"- JSON: `{iter_dir / 'assistant-release-regression.json'}`",
        f"- dirty audit: `{iter_dir / 'dirty-status.txt'}`",
        f"- git status: `{iter_dir / 'git-status.txt'}`",
    ]
)
(iter_dir / "assistant-release-regression.md").write_text("\n".join(lines) + "\n")
(iter_dir / "exit-code.txt").write_text("0\n" if ok else "1\n")
PY

cp "$iter_dir/assistant-release-regression.json" "$log_dir/assistant-release-regression-latest.json"
cp "$iter_dir/assistant-release-regression.md" "$log_dir/assistant-release-regression-latest.md"

cat "$log_dir/assistant-release-regression-latest.md"
exit "$(cat "$iter_dir/exit-code.txt")"
