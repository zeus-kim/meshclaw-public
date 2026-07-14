#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
bin_path="${MESHCLAW_RELEASE_BIN:-/Users/example/bin/meshclaw}"
log_dir="${MESHCLAW_DEV_LOOP_LOG_DIR:-$root/.meshclaw-dev-loop}"
json=0
skip_strict_mcp=0

usage() {
  cat <<'EOF'
usage: scripts/release-preflight.sh [--json] [--skip-strict-mcp]

Runs the safe release preflight without publishing anything:
  - local dev-loop refresh, including go test/build and router smoke
  - product-health strict MCP gate
  - core CLI JSON surfaces
  - MCP tool-name validation

Writes strict and development latest artifacts under .meshclaw-dev-loop/.
Text and JSON output include next_actions with severity and approval
requirements.

Use --skip-strict-mcp only for local development when stale app-side MCP
children are expected and the release is not being published.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --json)
      json=1
      ;;
    --skip-strict-mcp)
      skip_strict_mcp=1
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      exit 2
      ;;
  esac
  shift
done

cd "$root"
mkdir -p "$log_dir"

tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/meshclaw-release-preflight.XXXXXX")"
cleanup() {
  rm -rf "$tmpdir"
}
trap cleanup EXIT

log() {
  if [[ "$json" != "1" ]]; then
    printf '==> %s\n' "$*"
  fi
}

run_json_step() {
  local name="$1"
  shift
  local out="$tmpdir/${name}.out"
  local err="$tmpdir/${name}.err"
  local code=0
  if "$@" >"$out" 2>"$err"; then
    code=0
  else
    code=$?
  fi
  python3 - "$name" "$code" "$out" "$err" <<'PY'
import json
import sys
from pathlib import Path

name, code, out_path, err_path = sys.argv[1], int(sys.argv[2]), Path(sys.argv[3]), Path(sys.argv[4])
stdout = out_path.read_text(errors="replace")
stderr = err_path.read_text(errors="replace")
json_summary = {}
try:
    decoded = json.loads(stdout)
    if isinstance(decoded, dict):
        json_summary = {
            key: decoded.get(key)
            for key in (
                "ok",
                "product_ready",
                "not_ready_reasons",
                "mcp_action",
                "strict_mcp",
                "status",
                "stamp_utc",
                "failed_steps",
                "failed_count",
            )
            if key in decoded
        }
        mcp_stale = decoded.get("mcp_stale")
        if isinstance(mcp_stale, dict) and "stale_count" in mcp_stale:
            json_summary["mcp_stale_count"] = mcp_stale.get("stale_count")
except Exception:
    pass
print(json.dumps({
    "name": name,
    "ok": code == 0,
    "exit_code": code,
    "json_summary": json_summary,
    "stdout_preview": stdout[-4000:],
    "stderr_preview": stderr[-4000:],
}, ensure_ascii=False))
PY
  return "$code"
}

step_names=()
step_json=()
failed=0

run_step() {
  local name="$1"
  shift
  step_names+=("$name")
  log "$name"
  local result
  set +e
  result="$(run_json_step "$name" "$@")"
  local code=$?
  set -e
  step_json+=("$result")
  if [[ "$code" -ne 0 ]]; then
    failed=1
    if [[ "$json" != "1" ]]; then
      python3 - "$result" <<'PY'
import json
import sys

data = json.loads(sys.argv[1])
print(f"FAILED: {data['name']} (exit={data['exit_code']})")
summary = data.get("json_summary") or {}
if summary:
    print(json.dumps(summary, ensure_ascii=False, indent=2))
if data.get("stderr_preview"):
    print(data["stderr_preview"])
elif data.get("stdout_preview") and not summary:
    print(data["stdout_preview"])
PY
    fi
    return "$code"
  fi
}

run_step "dev-loop-local" scripts/dev-loop.sh || true

if [[ "$skip_strict_mcp" == "1" ]]; then
  run_step "product-health" scripts/product-health.sh --json || true
else
  run_step "product-health-strict-mcp" scripts/product-health.sh --strict-mcp --json || true
fi

run_step "cli-help" "$bin_path" help || true
run_step "cli-architecture-json" "$bin_path" architecture --json || true
run_step "cli-schedule-status-json" "$bin_path" schedule status --json || true
run_step "mcp-tools-list" bash -c "printf '{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/list\"}\\n' | '$bin_path' mcp" || true
run_step "mcp-tool-name-policy" bash -c "printf '{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/list\"}\\n' | '$bin_path' mcp | jq -e '.result.tools | all(.[]; .name | test(\"^[A-Za-z0-9_]+$\"))'" || true

summary_json="$tmpdir/release-preflight.json"
summary_md="$tmpdir/release-preflight.md"
python3 - "$failed" "$skip_strict_mcp" "${step_json[@]}" > "$summary_json" <<'PY'
import json
import sys
from datetime import datetime, timezone
from pathlib import Path

failed = sys.argv[1] == "1"
skip_strict_mcp = sys.argv[2] == "1"
steps = [json.loads(item) for item in sys.argv[3:]]

def build_next_actions(steps):
    actions = []
    for step in steps:
        if step.get("ok"):
            continue
        name = step.get("name")
        summary = step.get("json_summary") or {}
        if name == "product-health-strict-mcp":
            stale = summary.get("mcp_stale_count")
            if stale:
                actions.append(
                    {
                        "id": "inspect_stale_mcp",
                        "severity": "release_blocker",
                        "summary": f"stale direct MCP child processes detected: {stale}",
                        "command": "scripts/mcp-restart-stale.sh --dry-run --json",
                        "approval_required_for_fix": True,
                        "fix": "restart the owning client app, or run scripts/mcp-restart-stale.sh --execute only after operator approval",
                    }
                )
            actions.append(
                {
                    "id": "inspect_strict_release_preflight",
                    "severity": "release_blocker",
                    "summary": "strict release preflight is not ready: product-health-strict-mcp",
                    "command": "cat .meshclaw-dev-loop/release-preflight-strict-latest.md",
                    "approval_required_for_fix": False,
                }
            )
        else:
            actions.append(
                {
                    "id": f"inspect_{name}",
                    "severity": "release_blocker",
                    "summary": f"release preflight step failed: {name}",
                    "command": f"jq '.steps[] | select(.name==\"{name}\")' .meshclaw-dev-loop/release-preflight-strict-latest.json",
                    "approval_required_for_fix": False,
                }
            )
    if not actions:
        actions.append(
            {
                "id": "no_release_blocker",
                "severity": "info",
                "summary": "no release blocker detected by this preflight",
                "command": "scripts/product-health.sh",
                "approval_required_for_fix": False,
            }
        )
    return actions

files = {
    "status_index_md": str(Path(".meshclaw-dev-loop/STATUS.md").resolve()),
    "strict_release_md": str(Path(".meshclaw-dev-loop/release-preflight-strict-latest.md").resolve()),
    "strict_release_json": str(Path(".meshclaw-dev-loop/release-preflight-strict-latest.json").resolve()),
    "dev_release_md": str(Path(".meshclaw-dev-loop/release-preflight-dev-latest.md").resolve()),
    "dev_release_json": str(Path(".meshclaw-dev-loop/release-preflight-dev-latest.json").resolve()),
    "legacy_release_md": str(Path(".meshclaw-dev-loop/release-preflight-latest.md").resolve()),
    "legacy_release_json": str(Path(".meshclaw-dev-loop/release-preflight-latest.json").resolve()),
}

print(json.dumps({
    "ok": not failed,
    "failed": failed,
    "skip_strict_mcp": skip_strict_mcp,
    "stamp_utc": datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ"),
    "status_index": files["status_index_md"],
    "files": files,
    "total": len(steps),
    "passed": sum(1 for step in steps if step.get("ok")),
    "failed_count": sum(1 for step in steps if not step.get("ok")),
    "failed_steps": [step.get("name") for step in steps if not step.get("ok")],
    "next_actions": build_next_actions(steps),
    "steps": steps,
}, ensure_ascii=False, indent=2))
PY

python3 - "$summary_json" > "$summary_md" <<'PY'
import json
import sys
from pathlib import Path

data = json.loads(Path(sys.argv[1]).read_text())
print("# MeshClaw Release Preflight Latest")
print()
print(f"- stamp_utc: {data.get('stamp_utc')}")
print(f"- status: {'READY' if data.get('ok') else 'NOT READY'}")
print(f"- skip_strict_mcp: {data.get('skip_strict_mcp')}")
print(f"- passed: {data.get('passed')}/{data.get('total')}")
print(f"- status_index: {data.get('status_index')}")
failed = data.get("failed_steps") or []
if failed:
    print(f"- failed_steps: {', '.join(failed)}")
files = data.get("files") or {}
if files:
    print()
    print("## Files")
    print()
    for key in ("strict_release_md", "strict_release_json", "dev_release_md", "dev_release_json", "status_index_md"):
        if files.get(key):
            print(f"- {key}: `{files.get(key)}`")
print()
print("## Next Actions")
print()
for action in data.get("next_actions") or []:
    severity = action.get("severity") or "info"
    approval = "approval_required" if action.get("approval_required_for_fix") else "no_approval_required"
    summary = action.get("summary") or action.get("id")
    command = action.get("command")
    if command:
        print(f"- [{severity}; {approval}] {summary}: `{command}`")
    else:
        print(f"- [{severity}; {approval}] {summary}")
print()
print("## Steps")
print()
for step in data.get("steps") or []:
    marker = "PASS" if step.get("ok") else "FAIL"
    line = f"- {marker}: {step.get('name')} (exit={step.get('exit_code')})"
    summary = step.get("json_summary") or {}
    reasons = summary.get("not_ready_reasons") if isinstance(summary, dict) else None
    if reasons:
        line += " - " + "; ".join(str(reason) for reason in reasons[:3])
    elif isinstance(summary, dict) and "mcp_stale_count" in summary:
        line += f" - stale_mcp={summary.get('mcp_stale_count')}"
    print(line)
PY

if [[ "$skip_strict_mcp" == "1" ]]; then
  cp "$summary_json" "$log_dir/release-preflight-dev-latest.json"
  cp "$summary_md" "$log_dir/release-preflight-dev-latest.md"
else
  cp "$summary_json" "$log_dir/release-preflight-strict-latest.json"
  cp "$summary_md" "$log_dir/release-preflight-strict-latest.md"
  cp "$summary_json" "$log_dir/release-preflight-latest.json"
  cp "$summary_md" "$log_dir/release-preflight-latest.md"
fi

if [[ "$json" == "1" ]]; then
  cat "$summary_json"
else
  cat "$summary_md"
fi

if [[ "$failed" == "1" ]]; then
  exit 1
fi
