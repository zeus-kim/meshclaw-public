#!/usr/bin/env bash
set -euo pipefail

meshclaw_bin="${MESHCLAW_BIN:-/Users/example/bin/meshclaw}"
vssh_bin="${VSSH_BIN:-/Users/example/bin/vssh}"

echo "MeshClaw MCP client status"
echo

if [[ -x "$meshclaw_bin" ]]; then
  echo "meshclaw: $meshclaw_bin"
  ls -l "$meshclaw_bin"
  "$meshclaw_bin" version 2>/dev/null || true
else
  echo "meshclaw: missing or not executable: $meshclaw_bin"
fi

echo
if [[ -x "$vssh_bin" ]]; then
  echo "vssh: $vssh_bin"
  ls -l "$vssh_bin"
  "$vssh_bin" version 2>/dev/null || true
else
  echo "vssh: missing or not executable: $vssh_bin"
fi

echo
echo "Configured MCP clients:"
for cfg in \
  "$HOME/Library/Application Support/Claude/claude_desktop_config.json" \
  "$HOME/Library/Application Support/Claude-3p/claude_desktop_config.json" \
  "$HOME/.cursor/mcp.json"
do
  [[ -f "$cfg" ]] || continue
  echo
  echo "== $cfg =="
  jq -r '
    (.mcpServers // .servers // {}) as $servers
    | $servers
    | to_entries[]
    | select(.key == "meshclaw" or .key == "vssh")
    | "\(.key): command=\(.value.command // "") args=\((.value.args // [])|join(" ")) timeout=\(.value.timeout // "default")"
  ' "$cfg" 2>/dev/null || true
done

echo
echo "Running MCP processes:"
ps -axo pid,ppid,lstart,command \
  | rg '(/Users/example/bin/(meshclaw|vssh) mcp|Claude\.app/Contents/Helpers/disclaimer /Users/example/bin/(meshclaw|vssh) mcp)' \
  | rg -v 'rg \(' || true

echo
python3 - "$meshclaw_bin" "$vssh_bin" <<'PY' || true
import os
import re
import subprocess
import sys
from datetime import datetime

bins = {path: os.path.getmtime(path) for path in sys.argv[1:] if os.path.exists(path)}
if not bins:
    raise SystemExit(0)

out = subprocess.check_output(
    ["ps", "-axo", "pid=,ppid=,lstart=,command="],
    text=True,
    stderr=subprocess.DEVNULL,
)
stale = []
line_re = re.compile(r"^\s*(\d+)\s+(\d+)\s+(.{24})\s+(.+)$")
for line in out.splitlines():
    if "/Users/example/bin/meshclaw mcp" not in line and "/Users/example/bin/vssh mcp" not in line:
        continue
    match = line_re.match(line)
    if not match:
        continue
    pid, ppid, started_raw, command = match.groups()
    try:
        started = datetime.strptime(started_raw.strip(), "%a %b %d %H:%M:%S %Y").timestamp()
    except ValueError:
        continue
    for path, mtime in bins.items():
        command = command.strip()
        if command.startswith(f"{path} mcp") and started < mtime:
            stale.append(
                {
                    "pid": pid,
                    "ppid": ppid,
                    "name": os.path.basename(path),
                    "started": started_raw.strip(),
                    "started_ts": started,
                    "built": datetime.fromtimestamp(mtime).strftime("%Y-%m-%d %H:%M:%S"),
                }
            )

if stale:
    counts = {}
    for item in stale:
        counts[item["name"]] = counts.get(item["name"], 0) + 1
    oldest = min(stale, key=lambda item: item["started_ts"])
    summary = " ".join(f"{name}={count}" for name, count in sorted(counts.items()))
    print(f"Stale MCP summary: total={len(stale)} {summary} oldest_pid={oldest['pid']} oldest_started={oldest['started']}")
    print("Stale MCP processes:")
    for item in stale:
        print(f"- pid={item['pid']} ppid={item['ppid']} {item['name']} started={item['started']} older_than_binary={item['built']}")
    print("Action: restart the affected client app, or terminate only those MCP child processes and let the client reconnect.")
else:
    print("Stale MCP processes: none detected")
PY
echo
echo "Note: after rebuilding /Users/example/bin/meshclaw, already-running MCP processes keep the old in-memory binary. Restart the client app or terminate only that client MCP child process."
