# Workspaces

MeshClaw tracks where work is happening.

This is separate from server monitoring. It answers:

- which server is involved
- which folder is involved
- which model, human, or Matrix room owns the lane
- what branch or purpose the lane has
- what happened recently

## CLI

```sh
meshclaw workspace-list
meshclaw workspace-add meshclaw-local local /Users/example/Projects/meshclaw codex serverops
meshclaw workspace-activity meshclaw-local codex edit "added workspace registry"
```

## MCP

- `meshclaw_workspace_list`
- `meshclaw_workspace_add`
- `meshclaw_workspace_activity`

## Storage

The registry is stored at:

```text
~/.meshclaw/workspaces.json
```

Activity is stored as normal MeshClaw evidence with kind:

```text
workspace-activity
```

## Operator Rooms

Matrix is optional legacy/ops-room compatibility. Signal/Argos and Open WebUI
are the preferred current user-facing surfaces. Any room-based frontend should
map workspace requests to the same MCP/CLI tools:

- `!workspaces`
- `!workspace add ...`
- `!activity ...`

This keeps Codex app work, Claude web work, Open WebUI work, local shell work,
and Matrix work visible in one operational timeline.

## Local Conversation Test

```sh
meshclaw ops-chat
meshclaw ops-dispatch openwebui "workspaces"
```

Try:

```text
workers
workspaces
add current workspace
show server status
show recent evidence
show policy

Korean examples are also supported:
현재 워크스페이스 등록
서버 상태 보여줘
```
