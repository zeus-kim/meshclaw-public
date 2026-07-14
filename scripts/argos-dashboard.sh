#!/usr/bin/env bash
set -euo pipefail

meshclaw="${MESHCLAW_BIN:-meshclaw}"
public_root="${MESHCLAW_ARGOS_PUBLIC_ROOT:-$HOME/.meshclaw/public}"
out="${MESHCLAW_ARGOS_DASHBOARD:-$public_root/argos/dashboard.html}"
lang="${MESHCLAW_LANG:-ko}"

mkdir -p "$(dirname "$out")"

health_json="$("$meshclaw" messenger dispatch-health --runtime-required --json 2>/dev/null || printf '{}')"
schedule_json="$("$meshclaw" schedule plan --json 2>/dev/null || printf '{}')"
schedule_status_json="$("$meshclaw" schedule status --json 2>/dev/null || printf '{}')"
approval_json="$("$meshclaw" workflows plan-execute latest --json 2>/dev/null || printf '{}')"
targets_json="$("$meshclaw" messenger targets --json 2>/dev/null || printf '{}')"

python3 - "$health_json" "$schedule_json" "$schedule_status_json" "$approval_json" "$targets_json" "$out" "$public_root" "$lang" <<'PY'
import html
import json
import os
import re
import sys
from collections import Counter, defaultdict
from datetime import datetime, timezone
from pathlib import Path
from urllib.parse import urlparse

health = json.loads(sys.argv[1] or "{}")
schedule = json.loads(sys.argv[2] or "{}")
live_schedule = json.loads(sys.argv[3] or "{}")
approval = json.loads(sys.argv[4] or "{}")
targets = json.loads(sys.argv[5] or "{}")
out = Path(sys.argv[6])
public_root = Path(sys.argv[7])
lang = (sys.argv[8] or "ko").lower()
if not lang.startswith("en"):
    lang = "ko"

PACKS = {
    "ko": {
        "title": "Argos 관제 센터",
        "subtitle": "Health, approvals, evidence, reports, schedules, and Signal rooms",
        "health": "Health",
        "approvals": "Approvals",
        "evidence": "Evidence",
        "reports": "Recent Reports",
        "activity": "Today Activity",
        "activity_empty": "오늘 기록된 작업 결과가 아직 없습니다.",
        "evidence_filter": "Evidence 필터",
        "filter_match": "일치",
        "jobs": "Jobs",
        "agents": "Agents",
        "rooms": "Signal Rooms",
        "runtime": "Runtime",
        "dispatcher": "Signal dispatcher",
        "due": "Due",
        "next": "Next",
        "updated": "Updated",
        "healthy": "정상",
        "warning": "주의",
        "unknown": "확인 필요",
        "read_only": "읽기 전용 대시보드입니다. 이 페이지는 실행, 발송, 배포, 승인 처리를 하지 않습니다. 관찰은 Dashboard, 승인은 Signal에서 합니다.",
    },
    "en": {
        "title": "Argos Operations Center",
        "subtitle": "Health, approvals, evidence, reports, schedules, and Signal rooms",
        "health": "Health",
        "approvals": "Approvals",
        "evidence": "Evidence",
        "reports": "Recent Reports",
        "activity": "Today Activity",
        "activity_empty": "No activity has been recorded today.",
        "evidence_filter": "Evidence filter",
        "filter_match": "match",
        "jobs": "Jobs",
        "agents": "Agents",
        "rooms": "Signal Rooms",
        "runtime": "Runtime",
        "dispatcher": "Signal dispatcher",
        "due": "Due",
        "next": "Next",
        "updated": "Updated",
        "healthy": "Healthy",
        "warning": "Warning",
        "unknown": "Needs check",
        "read_only": "Read-only dashboard. This page does not execute, send, deploy, or approve. Observe in Dashboard; approve in Signal.",
    },
}

T = PACKS[lang].get
home = Path(os.environ.get("HOME", ""))
result = health.get("result") or {}
schedule_status = result.get("schedule_status") or {}
dispatcher = result.get("dispatcher") or {}
jobs = schedule.get("jobs") or []
live_jobs = {str(job.get("id") or ""): job for job in (live_schedule.get("jobs") or []) if isinstance(job, dict)}
plan = approval.get("result") if isinstance(approval.get("result"), dict) else approval
if not isinstance(plan, dict):
    plan = {}
status = result.get("status") or "unknown"
status_label = T("healthy") if status == "healthy" else T("warning") if status == "warning" else T("unknown")
status_class = "ok" if status == "healthy" else "warn"
updated = datetime.now(timezone.utc).astimezone().strftime("%Y-%m-%d %H:%M:%S %Z")

def esc(value):
    return html.escape(str(value if value is not None else ""))

def parse_time(value):
    if not value:
        return None
    text = str(value).strip()
    if text.endswith("Z"):
        text = text[:-1] + "+00:00"
    try:
        return datetime.fromisoformat(text)
    except ValueError:
        return None

def fmt_time(value):
    dt = parse_time(value)
    if not dt:
        return "-"
    return dt.astimezone().strftime("%m-%d %H:%M")

def item_time(item):
    dt = parse_time(item.get("time"))
    if dt:
        return dt.astimezone()
    try:
        return datetime.fromtimestamp(float(item.get("mtime") or 0), timezone.utc).astimezone()
    except Exception:
        return None

def slug(value):
    text = re.sub(r"[^A-Za-z0-9._-]+", "-", str(value or "item")).strip("-")
    return text[:96] or "item"

def normalized_kind(value):
    return re.sub(r"[^a-z0-9]+", "_", str(value or "").strip().lower()).strip("_")

def is_open_url_kind(value):
    kind = normalized_kind(value)
    return kind == "open_url" or kind.endswith("_open_url") or "automation_open_url" in kind

def first_url(value):
    if value is None:
        return ""
    if isinstance(value, str):
        match = re.search(r"https?://[^\s<>'\")]+", value)
        return match.group(0) if match else ""
    if isinstance(value, dict):
        for key in ("url", "raw_url", "target_url", "link", "href", "summary", "text"):
            if key in value:
                found = first_url(value.get(key))
                if found:
                    return found
        for nested in value.values():
            found = first_url(nested)
            if found:
                return found
    if isinstance(value, list):
        for nested in value:
            found = first_url(nested)
            if found:
                return found
    return ""

def display_host(url):
    try:
        return urlparse(url).netloc
    except Exception:
        return ""

def redact_text(value):
    text = str(value or "")
    text = re.sub(r"https?://[^\s<>'\")]+", lambda m: "URL(" + (display_host(m.group(0)) or "hidden") + ")", text)
    text = re.sub(r"/Users/[^\s<>'\")]+", "[local path hidden]", text)
    return text

def sensitive_json_key(key):
    key_norm = normalized_kind(key)
    if key_norm in {"command", "stdout", "stderr", "group_id", "recipient", "password", "password_handle", "secret", "token", "api_key", "authorization", "bearer", "cookie"}:
        return True
    return any(part in key_norm for part in ("password", "secret", "token", "api_key", "authorization", "bearer", "cookie"))

def redact_public_json(value, key=""):
    key_norm = normalized_kind(key)
    if sensitive_json_key(key):
        return "[redacted]"
    if key_norm in {"url", "raw_url", "target_url", "href", "link"}:
        host = display_host(str(value or ""))
        return "URL(" + (host or "hidden") + ")"
    if isinstance(value, dict):
        out = {}
        redacted = 0
        for k, v in value.items():
            if sensitive_json_key(str(k)):
                redacted += 1
                continue
            out[str(k)] = redact_public_json(v, str(k))
        if redacted:
            out["_redacted_fields"] = redacted
        return out
    if isinstance(value, list):
        if key_norm == "command":
            return "[redacted]"
        return [redact_public_json(v, key) for v in value]
    if isinstance(value, str):
        return redact_text(value)
    return value

def evidence_display_summary(item):
    summary = str(item.get("summary") or item.get("id") or "").strip()
    kind = str(item.get("kind") or "").strip().lower()
    url = first_url(summary) or first_url(item.get("data"))
    if is_open_url_kind(kind) or url:
        host = display_host(url)
        if host:
            return f"URL 열기 요청 · {host}"
        return "URL 열기 요청"
    summary = re.sub(r"\s+", " ", summary).strip()
    if len(summary) > 120:
        summary = summary[:117].rstrip() + "..."
    return summary or str(item.get("id") or "-")

def evidence_display_kind(item):
    kind = str(item.get("kind") or "").strip()
    if is_open_url_kind(kind):
        return "URL 열기 요청" if lang == "ko" else "Open URL request"
    return kind or "evidence"

def load_schedule_state():
    path = Path(os.environ.get("MESHCLAW_SCHEDULE_STATE", "")) if os.environ.get("MESHCLAW_SCHEDULE_STATE") else home / ".meshclaw" / "schedule-state.json"
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except Exception:
        return {}
    return data.get("last_run") or {}

def find_evidence():
    root = home / ".meshclaw" / "evidence"
    items = []
    if not root.exists():
        return items
    for path in root.glob("*/*.json"):
        if path.parent.name == "latest":
            continue
        try:
            stat = path.stat()
        except OSError:
            continue
        data = {}
        try:
            data = json.loads(path.read_text(encoding="utf-8"))
        except Exception:
            pass
        items.append({
            "path": path,
            "mtime": stat.st_mtime,
            "id": data.get("id") or path.stem,
            "time": data.get("time") or data.get("created_at") or "",
            "kind": data.get("kind") or evidence_kind_from_name(path.name),
            "host": data.get("host") or "",
            "summary": data.get("summary") or "",
            "data": data,
        })
    items.sort(key=lambda item: item["mtime"], reverse=True)
    return items

def evidence_kind_from_name(name):
    stem = Path(name).stem
    parts = stem.split("-")
    if len(parts) >= 4 and parts[0].startswith("20"):
        return "-".join(parts[2:-1]) or "evidence"
    return "evidence"

def evidence_display_id(value):
    match = re.search(r"\b\d{8}[Tt]\d{6}[Zz]-\d{1,9}\b", str(value or ""))
    return match.group(0).upper() if match else ""

def write_evidence_pages(items):
    detail_dir = public_root / "argos" / "evidence"
    detail_dir.mkdir(parents=True, exist_ok=True)
    for old in detail_dir.glob("*.html"):
        try:
            old.unlink()
        except OSError:
            pass
    links = {}
    for item in items[:40]:
        name = slug(item["id"]) + ".html"
        target = detail_dir / name
        public_data = redact_public_json(item["data"] or {"path": str(item["path"])})
        body = json.dumps(public_data, ensure_ascii=False, indent=2)
        display_summary = evidence_display_summary(item)
        target.write_text(f"""<!doctype html>
<html lang="{esc(lang)}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex,nofollow"><title>{esc(item['id'])}</title>
<style>body{{margin:0;font:16px/1.55 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:#f7f8fb;color:#111827}}main{{max-width:980px;margin:0 auto;padding:20px}}pre{{white-space:pre-wrap;word-break:break-word;background:#fff;border:1px solid #d8dde6;border-radius:8px;padding:14px}}a{{color:#2563eb}}</style></head>
<body><main><p><a href="../dashboard.html">Argos dashboard</a></p><h1>{esc(evidence_display_kind(item))}</h1><p>{esc(item['time'])} {esc(item['host'])}</p><p><strong>결과:</strong> {esc(display_summary)}</p><p class="muted">Public view: sensitive fields and local paths are redacted.</p><pre>{esc(body)}</pre></main></body></html>
""", encoding="utf-8")
        links[str(item["path"])] = "evidence/" + name
    return links

last_run = load_schedule_state()
evidence_items = find_evidence()
evidence_links = write_evidence_pages(evidence_items)
evidence_by_kind = Counter(item["kind"] for item in evidence_items)
evidence_by_job = defaultdict(int)
for item in evidence_items:
    kind = item["kind"]
    for job in jobs:
        job_id = job.get("id", "")
        job_kind = job.get("kind", "")
        if job_id and (job_id in kind or kind == "schedule-" + job_kind or job_kind in kind):
            evidence_by_job[job_id] += 1

approval_counts = plan.get("counts") or {}
approval_pending = int(approval_counts.get("approval_pending") or 0)
vault_missing = int(approval_counts.get("vault_missing") or 0)
approval_items = plan.get("approval_pending") or []
approval_blockers = approval_items or plan.get("vault_missing") or []

report_kinds = {"schedule-local_ai_briefing", "schedule-assistant_morning", "news-brief", "ops-brief", "serverops-quickcheck", "schedule-serverops_quickcheck", "schedule-argos_health", "schedule-assistant_auto_check"}
recent_reports = [item for item in evidence_items if item["kind"] in report_kinds or "brief" in item["kind"] or "report" in item["kind"]][:8]
recent_evidence = evidence_items[:8]
today = datetime.now(timezone.utc).astimezone().date()
today_items = [item for item in evidence_items if item_time(item) and item_time(item).date() == today]
today_reports = [item for item in today_items if item["kind"] in report_kinds or "brief" in item["kind"] or "report" in item["kind"]]
today_actions = Counter(evidence_display_kind(item) for item in today_items)

room_targets = targets.get("targets") or []
room_rows = []
for target in room_targets:
    tid = target.get("id") or "-"
    mode = target.get("mode") or "-"
    role = "interactive" if mode in {"assistant", "chat", "guard"} else "one-way/no-reply"
    if tid in {"argos-briefing", "argos-ops", "report-room"}:
        role = "one-way/no-reply"
    room_rows.append(
        "<tr>"
        f"<td data-label='Room'>{esc(tid)}</td>"
        f"<td data-label='Mode'>{esc(mode)}</td>"
        f"<td data-label='Role'>{esc(role)}</td>"
        f"<td data-label='Label'>{esc(target.get('label') or '')}</td>"
        "</tr>"
    )
if not room_rows:
    room_rows.append("<tr><td colspan='4'>-</td></tr>")

def cadence(job):
    return job.get("interval") or job.get("hourly_at") or job.get("daily_at") or "-"

def job_status(job):
    jid = job.get("id")
    live = live_jobs.get(jid, {})
    enabled = live.get("enabled", job.get("enabled", True))
    if not enabled:
        return "disabled"
    if live.get("due"):
        return "due"
    return "OK" if jid in last_run else "pending"

def job_last(job):
    jid = job.get("id")
    live = live_jobs.get(jid, {})
    return fmt_time(live.get("last_run") or last_run.get(jid))

def job_next(job):
    jid = job.get("id")
    live = live_jobs.get(jid, {})
    return fmt_time(live.get("next_due"))

job_rows = []
for job in jobs:
    jid = job.get("id") or "-"
    target = job.get("target_id") or "-"
    last = job_last(job)
    next_due = job_next(job)
    ev_count = evidence_by_job.get(jid, 0)
    job_rows.append(
        "<tr>"
        f"<td data-label='Job'><strong>{esc(jid)}</strong><div class='muted'>{esc(job.get('kind') or '')}</div></td>"
        f"<td data-label='Status'>{esc(job_status(job))}</td>"
        f"<td data-label='Last'>{esc(last)}</td>"
        f"<td data-label='Next'>{esc(next_due)}</td>"
        f"<td data-label='Cadence'>{esc(cadence(job))}</td>"
        f"<td data-label='Room'>{esc(target)}</td>"
        f"<td data-label='Evidence'>{ev_count}</td>"
        "</tr>"
    )
if not job_rows:
    job_rows.append("<tr><td colspan='7'>-</td></tr>")

agent_map = [
    ("News Agent", ["news-brief", "schedule-local_ai_briefing"], "뉴스 선별/브리핑"),
    ("Mail Agent", ["schedule-mail_watch"], "메일 감시"),
    ("Memory Agent", ["schedule-assistant_auto_check"], "메모리/프로필 점검"),
    ("DevOps Agent", ["ops-brief", "schedule-serverops_quickcheck", "schedule-data_doctor", "schedule-local_hygiene"], "서버/보안 관제"),
]
agent_cards = []
for name, kinds, desc in agent_map:
    count = sum(evidence_by_kind.get(kind, 0) for kind in kinds)
    latest = next((item for item in evidence_items if item["kind"] in kinds), None)
    state = "healthy" if latest else "unknown"
    cls = "ok" if latest else "warn"
    agent_cards.append(f"<div class='card'><div class='label'>{esc(name)}</div><div class='value {cls}'>{esc(state)}</div><div class='muted'>{esc(desc)}</div><div class='muted'>Evidence {count} · Last {esc(fmt_time(latest['time']) if latest else '-')}</div></div>")

def evidence_list(items):
    rows = []
    for item in items:
        href = evidence_links.get(str(item["path"]), "#")
        evidence_id = evidence_display_id(item["id"])
        attr = f" data-evidence-id='{esc(evidence_id)}'" if evidence_id else ""
        id_line = f"<br><span class='muted evidence-id'>ID {esc(evidence_id)}</span>" if evidence_id else ""
        rows.append(f"<li{attr}><a href='{esc(href)}'>{esc(fmt_time(item['time']) or '-')} · {esc(evidence_display_kind(item))}</a><br><span class='muted'>{esc(evidence_display_summary(item))}</span>{id_line}</li>")
    if not rows:
        rows.append("<li class='muted'>-</li>")
    return "".join(rows)

approval_preview = []
for item in approval_blockers[:4]:
    if not isinstance(item, dict):
        continue
    label = item.get("step") or item.get("title") or "approval item"
    node = item.get("node") or item.get("resource") or "-"
    reason = item.get("reason") or item.get("next_action") or ""
    approval_preview.append(f"<li><strong>{esc(label)}</strong><br><span class='muted'>{esc(node)} · {esc(reason)}</span></li>")
if not approval_preview:
    approval_preview.append("<li class='muted'>현재 대기 중인 승인은 없습니다.</li>")

def activity_list():
    if not today_items:
        return f"<li class='muted'>{esc(T('activity_empty'))}</li>"
    lines = []
    for label, count in today_actions.most_common(6):
        lines.append(f"<li><strong>{esc(label)}</strong> <span class='muted'>{count}</span></li>")
    return "".join(lines)

latest_activity = today_items[0] if today_items else None
activity_block = f"""
<section class="card activity">
  <h2>{esc(T('activity'))}</h2>
  <div class="activity-grid">
    <div><div class="label">Total</div><div class="value">{len(today_items)}</div><div class="muted">reports {len(today_reports)} · linked details {min(len(today_items), 40)}</div></div>
    <div><div class="label">Latest</div><div class="value small">{esc(evidence_display_kind(latest_activity) if latest_activity else '-')}</div><div class="muted">{esc(evidence_display_summary(latest_activity) if latest_activity else T('activity_empty'))}</div></div>
    <div><div class="label">Breakdown</div><ul>{activity_list()}</ul></div>
  </div>
</section>
"""

top_cards = f"""
<section class="grid top">
  <div class="card"><div class="label">{esc(T('health'))}</div><div class="value {status_class}">{esc(status_label)}</div><div class="muted">{esc(T('runtime'))}: {esc(status)} · {esc(T('due'))}: {esc(schedule_status.get('due_count', '-'))}</div></div>
  <div class="card"><div class="label">{esc(T('approvals'))}</div><div class="value {'warn' if approval_pending or vault_missing else 'ok'}">{approval_pending}</div><div class="muted">vault blockers {vault_missing} · observe only</div></div>
  <div class="card"><div class="label">{esc(T('evidence'))}</div><div class="value">{len(evidence_items)}</div><div class="muted">recent {len(recent_evidence)} linked logs</div></div>
  <div class="card"><div class="label">{esc(T('reports'))}</div><div class="value">{len(recent_reports)}</div><div class="muted">{esc(T('next'))}: {esc(schedule_status.get('next_due_job') or '-')}</div></div>
</section>
"""

html_text = f"""<!doctype html>
<html lang="{esc(lang)}">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="robots" content="noindex,nofollow">
  <title>{esc(T('title'))}</title>
  <style>
    :root {{ color-scheme: light dark; --ok:#087f5b; --warn:#b7791f; --bad:#c2410c; --line:#d8dde6; --muted:#667085; --bg:#f7f8fb; --card:#fff; }}
    body {{ margin:0; font:16px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif; background:var(--bg); color:#111827; -webkit-text-size-adjust:100%; }}
    main {{ max-width:1180px; margin:0 auto; padding:24px; }}
    h1 {{ margin:0; font-size:28px; }}
    h2 {{ margin:28px 0 10px; font-size:20px; }}
    a {{ color:#2563eb; text-decoration:none; }}
    a:hover {{ text-decoration:underline; }}
    .sub,.muted {{ color:var(--muted); }}
    .sub {{ margin:4px 0 20px; }}
    .grid {{ display:grid; grid-template-columns:repeat(auto-fit,minmax(230px,1fr)); gap:12px; }}
    .top {{ grid-template-columns:repeat(auto-fit,minmax(170px,1fr)); }}
    .split {{ display:grid; grid-template-columns:minmax(0,1fr) minmax(0,1fr); gap:12px; }}
    .activity {{ margin:12px 0; }}
    .activity h2 {{ margin-top:0; }}
    .activity-grid {{ display:grid; grid-template-columns:0.7fr 1.3fr 1fr; gap:14px; align-items:start; }}
    .card {{ background:var(--card); border:1px solid var(--line); border-radius:8px; padding:16px; min-width:0; }}
    .label {{ color:var(--muted); font-size:13px; }}
    .value {{ font-size:28px; font-weight:700; margin-top:4px; word-break:break-word; }}
    .value.small {{ font-size:20px; line-height:1.25; }}
    .ok {{ color:var(--ok); }}
    .warn {{ color:var(--warn); }}
    .bad {{ color:var(--bad); }}
    .filter-note {{ margin:0 0 10px; padding:10px 12px; border:1px solid #93c5fd; border-radius:8px; background:#eff6ff; color:#1d4ed8; }}
    .evidence-match {{ outline:2px solid #2563eb; border-radius:8px; padding:8px; background:rgba(37,99,235,.08); }}
    .evidence-dim {{ opacity:.42; }}
    table {{ width:100%; border-collapse:collapse; margin-top:10px; background:var(--card); border:1px solid var(--line); }}
    th,td {{ text-align:left; border-bottom:1px solid var(--line); padding:10px; vertical-align:top; }}
    th {{ font-size:13px; color:var(--muted); }}
    ul {{ margin:8px 0 0; padding-left:20px; }}
    li {{ margin:0 0 8px; }}
    li a {{ display:inline-block; padding:2px 0; }}
    footer {{ color:var(--muted); margin-top:22px; font-size:13px; }}
    @media (max-width:760px) {{
      main {{ padding:16px; }}
      h1 {{ font-size:24px; line-height:1.2; }}
      h2 {{ margin-top:22px; font-size:18px; }}
      .sub {{ font-size:14px; }}
      .grid, .top, .split {{ grid-template-columns:1fr; gap:10px; }}
      .activity-grid {{ grid-template-columns:1fr; gap:10px; }}
      .top {{ grid-template-columns:repeat(2,minmax(0,1fr)); }}
      .card {{ padding:14px; }}
      .value {{ font-size:24px; }}
      table, thead, tbody, tr, th, td {{ display:block; }}
      table {{ border:0; background:transparent; }}
      thead {{ position:absolute; width:1px; height:1px; padding:0; margin:-1px; overflow:hidden; clip:rect(0,0,0,0); white-space:nowrap; border:0; }}
      tr {{ margin:0 0 10px; background:var(--card); border:1px solid var(--line); border-radius:8px; overflow:hidden; }}
      td {{ display:flex; justify-content:space-between; gap:14px; padding:10px 12px; border-bottom:1px solid var(--line); text-align:right; word-break:break-word; }}
      td:last-child {{ border-bottom:0; }}
      td::before {{ content:attr(data-label); flex:0 0 88px; color:var(--muted); font-size:13px; text-align:left; }}
      td strong, td .muted {{ display:block; }}
      footer {{ font-size:12px; line-height:1.45; }}
    }}
    @media (max-width:390px) {{
      main {{ padding:12px; }}
      .top {{ grid-template-columns:1fr; }}
      td {{ display:block; text-align:left; }}
      td::before {{ display:block; margin-bottom:4px; }}
    }}
    @media (prefers-color-scheme: dark) {{
      body {{ color:#f9fafb; }}
      :root {{ --bg:#111827; --card:#18202f; --line:#344054; --muted:#a5adba; }}
      .filter-note {{ background:#172554; border-color:#1d4ed8; color:#bfdbfe; }}
    }}
  </style>
</head>
<body>
<main>
  <h1>{esc(T('title'))}</h1>
  <p class="sub">{esc(T('subtitle'))}</p>
  {top_cards}
  {activity_block}

  <section class="split">
    <div class="card">
      <h2>{esc(T('approvals'))}</h2>
      <ul>{''.join(approval_preview)}</ul>
    </div>
    <div class="card">
      <h2>{esc(T('reports'))}</h2>
      <ul>{evidence_list(recent_reports)}</ul>
    </div>
  </section>

  <h2>{esc(T('jobs'))}</h2>
  <table>
    <thead><tr><th>Job</th><th>Status</th><th>Last</th><th>Next</th><th>Cadence</th><th>Room</th><th>Evidence</th></tr></thead>
    <tbody>{''.join(job_rows)}</tbody>
  </table>

  <h2>{esc(T('agents'))}</h2>
  <section class="grid">{''.join(agent_cards)}</section>

  <section class="split">
    <div>
      <h2 id="evidence">{esc(T('evidence'))}</h2>
      <div id="evidence-filter" class="filter-note" hidden></div>
      <div class="card"><ul>{evidence_list(recent_evidence)}</ul></div>
    </div>
    <div>
      <h2>{esc(T('rooms'))}</h2>
      <table>
        <thead><tr><th>Room</th><th>Mode</th><th>Role</th><th>Label</th></tr></thead>
        <tbody>{''.join(room_rows)}</tbody>
      </table>
    </div>
  </section>

  <footer>{esc(T('read_only'))}<br>{esc(T('dispatcher'))}: {esc('running' if dispatcher.get('running') else 'stopped')} {esc(dispatcher.get('pid') or '')} · {esc(T('updated'))}: {esc(updated)} · binary {esc((result.get('binary_sha256') or '')[:12] or '-')}</footer>
</main>
<script>
(function() {{
  const params = new URLSearchParams(window.location.search);
  const evidence = (params.get("evidence") || params.get("evidence_id") || "").trim().toUpperCase();
  if (!evidence) return;
  const items = Array.from(document.querySelectorAll("[data-evidence-id]"));
  let matched = 0;
  for (const item of items) {{
    if ((item.dataset.evidenceId || "").toUpperCase() === evidence) {{
      item.classList.add("evidence-match");
      matched += 1;
    }} else {{
      item.classList.add("evidence-dim");
    }}
  }}
  const note = document.getElementById("evidence-filter");
  if (note) {{
    note.hidden = false;
    note.textContent = "{esc(T('evidence_filter'))}: " + evidence + " · " + matched + " {esc(T('filter_match'))}";
  }}
  const section = document.getElementById("evidence");
  if (section) section.scrollIntoView({{block: "start"}});
}})();
</script>
</body>
</html>
"""

out.write_text(html_text, encoding="utf-8")
print(str(out))
PY
