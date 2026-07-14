#!/usr/bin/env bash
set -euo pipefail

meshclaw_bin="${MESHCLAW_BIN:-/Users/example/bin/meshclaw}"
vssh_bin="${VSSH_BIN:-/Users/example/bin/vssh}"
execute=0
json=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --execute)
      execute=1
      ;;
    --dry-run)
      execute=0
      ;;
    --json)
      json=1
      ;;
    -h|--help)
      echo "usage: scripts/mcp-restart-stale.sh [--dry-run|--execute] [--json]"
      echo
      echo "Dry-run is the default. --execute sends SIGTERM only to direct"
      echo "/Users/example/bin/meshclaw mcp or /Users/example/bin/vssh mcp child processes"
      echo "that started before the current binary timestamp."
      exit 0
      ;;
    *)
      echo "usage: scripts/mcp-restart-stale.sh [--dry-run|--execute] [--json]" >&2
      exit 2
      ;;
  esac
  shift
done

python3 - "$execute" "$json" "$meshclaw_bin" "$vssh_bin" <<'PY'
import json
import os
import re
import signal
import subprocess
import sys
from datetime import datetime

execute = sys.argv[1] == "1"
json_out = sys.argv[2] == "1"
bins = {path: os.path.getmtime(path) for path in sys.argv[3:] if os.path.exists(path)}


def emit(payload: dict, exit_code: int = 0) -> None:
    if json_out:
        print(json.dumps(payload, ensure_ascii=False, indent=2))
        raise SystemExit(exit_code)
    if payload.get("error"):
        print(str(payload["error"]))
        raise SystemExit(exit_code)
    mode = "EXECUTE" if payload.get("execute") else "DRY-RUN"
    stale = payload.get("stale") or []
    if not stale:
        print("No stale MCP child processes detected.")
        raise SystemExit(exit_code)
    print(f"{mode}: stale MCP child processes={len(stale)}")
    for item in stale:
        print(f"- pid={item['pid']} ppid={item['ppid']} {item['name']} started={item['started']} older_than_binary={item['built']}")
    if not payload.get("execute"):
        print("No process was terminated. Re-run with --execute to terminate only the stale MCP child processes above.")
    elif payload.get("failed"):
        print("Some stale MCP processes could not be terminated:")
        for item in payload["failed"]:
            print(f"- pid={item['pid']}: {item['error']}")
    else:
        print("Sent SIGTERM to stale MCP child processes. Restart or refocus the client app if it does not reconnect automatically.")
    raise SystemExit(exit_code)


if not bins:
    emit({"ok": False, "execute": execute, "error": "No MeshClaw/vssh binaries found.", "stale": []}, 1)

out = subprocess.check_output(
    ["ps", "-axo", "pid=,ppid=,lstart=,command="],
    text=True,
    stderr=subprocess.DEVNULL,
)
line_re = re.compile(r"^\s*(\d+)\s+(\d+)\s+(.{24})\s+(.+)$")
stale = []
seen = set()
for line in out.splitlines():
    if "/Users/example/bin/meshclaw mcp" not in line and "/Users/example/bin/vssh mcp" not in line:
        continue
    match = line_re.match(line)
    if not match:
        continue
    pid_s, ppid_s, started_raw, command = match.groups()
    try:
        started = datetime.strptime(started_raw.strip(), "%a %b %d %H:%M:%S %Y").timestamp()
    except ValueError:
        continue
    for path, mtime in bins.items():
        command = command.strip()
        if not command.startswith(f"{path} mcp") or started >= mtime:
            continue
        pid = int(pid_s)
        if pid in seen:
            continue
        seen.add(pid)
        stale.append(
            {
                "pid": pid,
                "ppid": int(ppid_s),
                "name": os.path.basename(path),
                "started": started_raw.strip(),
                "built": datetime.fromtimestamp(mtime).strftime("%Y-%m-%d %H:%M:%S"),
            }
        )

payload = {"ok": True, "execute": execute, "stale_count": len(stale), "stale": stale, "terminated": [], "failed": []}
if not stale or not execute:
    emit(payload, 0)

for item in stale:
    try:
        os.kill(item["pid"], signal.SIGTERM)
        payload["terminated"].append(item["pid"])
    except ProcessLookupError:
        continue
    except PermissionError as exc:
        payload["failed"].append({"pid": item["pid"], "error": str(exc)})

if payload["failed"]:
    payload["ok"] = False
    emit(payload, 1)

emit(payload, 0)
PY
