#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
out="${MESHCLAW_STATUS_INDEX:-$root/.meshclaw-dev-loop/STATUS.md}"
quiet=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --quiet)
      quiet=1
      ;;
    -h|--help)
      echo "usage: scripts/status-index.sh [--quiet]"
      echo
      echo "Writes .meshclaw-dev-loop/STATUS.md from dev-loop-status JSON."
      echo
      echo "The status index is the human handoff file for product readiness,"
      echo "release readiness, stale MCP children, sealed Open WebUI state,"
      echo "and next_actions with severity and approval requirements."
      echo
      echo "Override output path with MESHCLAW_STATUS_INDEX=/path/to/STATUS.md."
      exit 0
      ;;
    *)
      echo "usage: scripts/status-index.sh [--quiet]" >&2
      exit 2
      ;;
  esac
  shift
done

mkdir -p "$(dirname "$out")"
payload="$("$root/scripts/dev-loop-status.sh" --json)"

python3 - "$payload" "$out" <<'PY'
import json
import sys
from pathlib import Path

data = json.loads(sys.argv[1])
out = Path(sys.argv[2])

checks = data.get("checks") or {}
router = checks.get("router_smoke") or {}
mcp = data.get("mcp_stale") or {}
openwebui = data.get("latest_openwebui") or {}
gateway = openwebui.get("gateway_summary") or {}
chat = openwebui.get("chat_summary") or {}
gateway_size = openwebui.get("gateway_output_size") or {}
chat_size = openwebui.get("chat_output_size") or {}
gateway_age = openwebui.get("gateway_age") or {}
chat_age = openwebui.get("chat_age") or {}
release = data.get("release_preflight") or {}
release_dev = data.get("release_preflight_dev") or {}
next_actions = data.get("next_actions") if isinstance(data.get("next_actions"), list) else []

def ready(value):
    if value is True:
        return "READY"
    if value is False:
        return "NOT READY"
    return "UNKNOWN"

def csv(values):
    if not values:
        return "none"
    return ", ".join(str(value) for value in values)

release_failed = release.get("failed_steps") or []
openwebui_gate_lines = []
if openwebui.get("sealed"):
    openwebui_gate_lines.append("- OpenWebUI: SEALED, ignored for product/release health; unseal with `MESHCLAW_OPENWEBUI_UNSEAL=1` for manual legacy checks")
else:
    openwebui_gate_lines.extend(
        [
            f"- OpenWebUI gateway: {gateway.get('ok')}/{gateway.get('total')} ok, fresh={gateway_age.get('fresh')}, max_chars={gateway_size.get('max_chars')}",
            f"- OpenWebUI chat: {chat.get('ok')}/{chat.get('total')} ok, fresh={chat_age.get('fresh')}, max_chars={chat_size.get('max_chars')}",
        ]
    )

lines = [
    "# MeshClaw Status Index",
    "",
    f"- stamp_utc: {data.get('stamp_utc')}",
    f"- product_ready: {ready(data.get('product_ready'))}",
    f"- local_dev_loop: {data.get('status')}",
    f"- artifact_dir: {data.get('artifact_dir')}",
    "- source_json: `scripts/dev-loop-status.sh --json`",
    "- product_health_json: `scripts/product-health.sh --json`",
    "",
    "## Product Gates",
    "",
    f"- router_smoke: {router.get('ok')}/{router.get('total')} ok, fail={router.get('fail')}",
    *openwebui_gate_lines,
    f"- stale_MCP_children: {mcp.get('stale_count')}",
    "",
    "## Release Gates",
    "",
    f"- strict_release_preflight: {ready(release.get('ok'))}, passed={release.get('passed')}/{release.get('total')}, failed={csv(release_failed)}",
    f"- dev_skip_strict_preflight: {ready(release_dev.get('ok'))}, passed={release_dev.get('passed')}/{release_dev.get('total')}",
    "",
    "## Actions",
    "",
]

next_action_ids = {str(action.get("id")) for action in next_actions if isinstance(action, dict)}

if mcp.get("stale_count") and "inspect_stale_mcp" not in next_action_ids:
    lines.append("- stale MCP: run `scripts/mcp-restart-stale.sh --dry-run --json`; restart the client app or run `scripts/mcp-restart-stale.sh --execute` only when approved.")
if release.get("ok") is False and release.get("failed_steps") and "inspect_strict_release_preflight" not in next_action_ids:
    lines.append("- release blocked: inspect `.meshclaw-dev-loop/release-preflight-strict-latest.md`.")
if data.get("product_ready") is False:
    lines.append("- product not ready: run `scripts/dev-loop-status.sh --json` and inspect `not_ready_reasons`.")
for action in next_actions[:5]:
    if not isinstance(action, dict):
        continue
    if action.get("id") == "no_immediate_action":
        continue
    summary = action.get("summary") or action.get("id")
    severity = action.get("severity") or "info"
    approval = "approval_required" if action.get("approval_required_for_fix") else "no_approval_required"
    command = action.get("command")
    if command:
        lines.append(f"- [{severity}; {approval}] {summary}: `{command}`")
    else:
        lines.append(f"- [{severity}; {approval}] {summary}")
if len(lines) and lines[-1] == "":
    pass
elif not any(line.startswith("- ") for line in lines[-4:]):
    lines.append("- no immediate action required.")

lines.extend(
    [
        "",
        "## Files",
        "",
        "- local loop detail: `.meshclaw-dev-loop/dev-loop-latest.md` / `.meshclaw-dev-loop/dev-loop-latest.json`",
        "- strict release gate: `.meshclaw-dev-loop/release-preflight-strict-latest.md` / `.meshclaw-dev-loop/release-preflight-strict-latest.json`",
        "- development release gate: `.meshclaw-dev-loop/release-preflight-dev-latest.md` / `.meshclaw-dev-loop/release-preflight-dev-latest.json`",
        "- sealed OpenWebUI gateway evidence, not a readiness gate: `.meshclaw-dev-loop/openwebui-gateway-smoke-latest.md` / `.meshclaw-dev-loop/openwebui-gateway-smoke-latest.json`",
        "- sealed OpenWebUI chat evidence, not a readiness gate: `.meshclaw-dev-loop/openwebui-chat-smoke-latest.md` / `.meshclaw-dev-loop/openwebui-chat-smoke-latest.json`",
    ]
)

out.write_text("\n".join(lines) + "\n")
PY

if [[ "$quiet" != "1" ]]; then
  echo "status index: $out"
fi
