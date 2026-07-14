#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
json=0
strict_mcp="${MESHCLAW_PRODUCT_HEALTH_STRICT_MCP:-0}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --json)
      json=1
      ;;
    --strict-mcp)
      strict_mcp=1
      ;;
    -h|--help)
      echo "usage: scripts/product-health.sh [--json] [--strict-mcp]"
      echo
      echo "Checks the latest MeshClaw product-readiness state using"
      echo "scripts/dev-loop-status.sh --fail-on-not-ready."
      echo "Open WebUI is sealed by default and ignored unless"
      echo "MESHCLAW_OPENWEBUI_UNSEAL=1 is set."
      echo
      echo "Text and JSON output include status_index and next_actions."
      echo "By default this also refreshes .meshclaw-dev-loop/STATUS.md."
      echo "Set MESHCLAW_PRODUCT_HEALTH_UPDATE_STATUS_INDEX=0 for a"
      echo "read-only probe."
      echo
      echo "--strict-mcp also fails when stale direct meshclaw/vssh MCP"
      echo "children are still running after a rebuild."
      exit 0
      ;;
    *)
      echo "usage: scripts/product-health.sh [--json] [--strict-mcp]" >&2
      exit 2
      ;;
  esac
  shift
done

set +e
payload="$("$root/scripts/dev-loop-status.sh" --json --fail-on-not-ready)"
status=$?
set -e

if [[ "${MESHCLAW_PRODUCT_HEALTH_UPDATE_STATUS_INDEX:-1}" != "0" && -x "$root/scripts/status-index.sh" ]]; then
  "$root/scripts/status-index.sh" --quiet >/dev/null 2>&1 || true
fi

if [[ "$json" == "1" ]]; then
  python3 - "$payload" "$status" "$strict_mcp" <<'PY'
import json
import sys

data = json.loads(sys.argv[1])
status = int(sys.argv[2])
strict_mcp = sys.argv[3] in {"1", "true", "yes"}
data.setdefault("status_index", ".meshclaw-dev-loop/STATUS.md")
mcp = data.get("mcp_stale") or {}
stale_count = int(mcp.get("stale_count") or 0)
if stale_count:
    data["mcp_action"] = "inspect with scripts/mcp-restart-stale.sh --dry-run --json; restart the client app or run scripts/mcp-restart-stale.sh --execute when approved"
else:
    data["mcp_action"] = "none"
if strict_mcp and stale_count:
    reasons = data.setdefault("not_ready_reasons", [])
    reason = f"stale direct MCP child processes detected: {stale_count}"
    if reason not in reasons:
        reasons.append(reason)
    data["product_ready"] = False
    data["ok"] = False
    data["strict_mcp"] = True
    status = 1
else:
    data["strict_mcp"] = strict_mcp
print(json.dumps(data, ensure_ascii=False, indent=2))
raise SystemExit(status)
PY
  exit "$?"
fi

python3 - "$payload" "$status" "$strict_mcp" <<'PY'
import json
import sys

data = json.loads(sys.argv[1])
status = int(sys.argv[2])
strict_mcp = sys.argv[3] in {"1", "true", "yes"}
mcp = data.get("mcp_stale") or {}
stale_count = int(mcp.get("stale_count") or 0)
if strict_mcp and stale_count:
    reasons = data.setdefault("not_ready_reasons", [])
    reason = f"stale direct MCP child processes detected: {stale_count}"
    if reason not in reasons:
        reasons.append(reason)
    data["product_ready"] = False
    status = 1
ready = bool(data.get("product_ready"))
print("MeshClaw product health: READY" if ready else "MeshClaw product health: NOT READY")
print(f"- stamp_utc: {data.get('stamp_utc')}")
print(f"- artifact_dir: {data.get('artifact_dir')}")
print(f"- status_index: {data.get('status_index') or '.meshclaw-dev-loop/STATUS.md'}")
print(f"- strict_mcp: {strict_mcp}")

reasons = data.get("not_ready_reasons") or []
if reasons:
    print("- not ready reasons:")
    for reason in reasons:
        print(f"  - {reason}")

router = (data.get("checks") or {}).get("router_smoke") or {}
print(f"- router smoke: {router.get('ok')}/{router.get('total')} ok, fail={router.get('fail')}")

openwebui = data.get("latest_openwebui") or {}
if openwebui.get("sealed"):
    print("- OpenWebUI: SEALED (ignored for product health; unseal with MESHCLAW_OPENWEBUI_UNSEAL=1)")
else:
    gateway = openwebui.get("gateway_summary") or {}
    chat = openwebui.get("chat_summary") or {}
    gateway_size = openwebui.get("gateway_output_size") or {}
    chat_size = openwebui.get("chat_output_size") or {}
    gateway_age = openwebui.get("gateway_age") or {}
    chat_age = openwebui.get("chat_age") or {}
    print(f"- OpenWebUI gateway: {gateway.get('ok')}/{gateway.get('total')} ok, fresh={gateway_age.get('fresh')}, max_chars={gateway_size.get('max_chars')}")
    print(f"- OpenWebUI chat: {chat.get('ok')}/{chat.get('total')} ok, fresh={chat_age.get('fresh')}, max_chars={chat_size.get('max_chars')}")

print(f"- stale MCP children: {mcp.get('stale_count')} (dry-run only)")
actions = data.get("next_actions") if isinstance(data.get("next_actions"), list) else []
visible_actions = [action for action in actions if isinstance(action, dict) and action.get("id") != "no_immediate_action"]
if visible_actions:
    print("- next actions:")
    for action in visible_actions[:5]:
        severity = action.get("severity") or "info"
        approval = "approval_required" if action.get("approval_required_for_fix") else "no_approval_required"
        summary = action.get("summary") or action.get("id")
        command = action.get("command")
        if command:
            print(f"  - [{severity}; {approval}] {summary}: {command}")
        else:
            print(f"  - [{severity}; {approval}] {summary}")
elif stale_count:
    print("- next actions:")
    print("  - [release_blocker; approval_required] stale MCP children: scripts/mcp-restart-stale.sh --dry-run --json")
raise SystemExit(status)
PY
