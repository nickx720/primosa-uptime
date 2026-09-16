# Runbook: alert received

A 🔴 or 🟢 message arrived in the Telegram group. What to check.

## 🔴 DOWN alert

1. **Read the detail in the message** — it's either `status <code>` (the
   endpoint responded but not with 2xx) or `timeout` / a fetch error
   (network-level failure, no response within 15s).
2. **Check the platform the service runs on:**
   - Railway: `railway status` / check the service's deployment status
     in the dashboard, or use the `railway` MCP tools
     (`environment_status`, `get_logs`) if available in this session.
   - Fly: `fly status -a <app>` and `fly logs -a <app>`.
3. **Hit the health URL directly**: `curl -i <url>` from your machine —
   confirms whether it's actually down or a one-off from GitHub's
   network (rare, but the 2-attempt retry in `main.go` already guards
   against normal transients).
4. **Known health paths** (see `targets.json`): `/health` for Proof of
   Life apps, `/healthz` for Unsub and Watermark. A 404 on the health
   path itself usually means a deploy changed the route — check recent
   commits/deploys on that service, not just "is it up."
5. Once resolved, no action needed in this repo — the next successful
   check will send the 🟢 UP alert automatically.

## 🟢 UP alert

Informational — confirms recovery and how long it was down (`since` in
`state.json`). If the outage was notable, consider a quick postmortem
note in the relevant project's own repo, not here.
