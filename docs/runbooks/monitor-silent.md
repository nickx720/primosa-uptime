# Runbook: monitor has gone silent

No alerts, but also no confirmation anything is actually running. Two
independent layers can fail separately — check both.

## Layer 1 — the Railway pinger (primary)

### Check 1 — is the service even running?

```sh
curl -s "$(railway domain)/healthz" | jq .
```

- No response / connection refused: the Railway service itself is down.
  Check `railway status`, the dashboard's deployment history, and
  `railway logs` (or the `get_logs` MCP tool).
- `{"ok": false, ...}`: the process is up but the last loop is stale
  (older than 3x `UPTIME_LOOP`) — check logs for a panic or a stuck
  cycle (see `docs/002-architecture.md`'s tick-skip note; consecutive
  skipped ticks mean one cycle never finished, usually a hung HTTP call
  to a target or Telegram).

### Check 2 — secrets/variables missing or rotated

If `TELEGRAM_BOT_TOKEN` or `TELEGRAM_CHAT_ID` are unset or rotated on
Railway, the process falls back to printing `[dry-run] Telegram
message:` to its logs instead of sending — `/healthz` still reports
`ok: true` (the loop itself is healthy), so this only shows up in
`railway logs`. Re-set via `railway variables --set ...`.

### Check 3 — volume / state

If `STATE_PATH` doesn't point at the mounted volume (`/data/state.json`),
state resets on every redeploy — targets look "first-seen" again after
each deploy (silent reseed, not an alert, but loses history/duration).
Check `railway variables` and that the volume is attached
(`docs/runbooks/deploy-railway.md`).

## Layer 2 — the GitHub watchdog (secondary)

The watchdog polls `/healthz` every 30 minutes and alerts (via the
*separate* `WATCHDOG_BOT_TOKEN` bot) only when Railway itself is
unreachable or stale — it does not replace Layer 1's own Telegram
alerts.

### Check 1 — is the watchdog workflow disabled?

GitHub auto-disables scheduled workflows on a repo with no activity for
60 days. `watchdog-state.json` is committed on every run specifically to
prevent this — silence for 60+ days usually means it was already
disabled and stopped self-renewing.

- Actions tab > watchdog workflow > look for a banner saying it was
  disabled due to inactivity.
- Fix: click "Enable workflow", then run `workflow_dispatch` once.

### Check 2 — `PINGER_HEALTHZ_URL` not set or stale

If the repo variable `PINGER_HEALTHZ_URL` was never set (new setup) or
points at an old Railway domain, the watchdog job logs "repo variable
not set yet" or gets a connection failure on every run and alerts
every time (never recovers) — check Settings > Secrets and variables >
Actions > Variables, and `railway domain` for the current URL.

### Check 3 — `WATCHDOG_BOT_TOKEN` missing

If unset, the watchdog job still runs and evaluates health, but skips
sending Telegram messages entirely — check the job's own run log
("Watchdog check FAILED: ...") rather than expecting a Telegram message.

### Check 4 — you're only getting one alert, not a stream

This is intentional: the watchdog sends one message on the failure ->
ok transition and one on the recovery (ok -> failure) transition, using
`watchdog-state.json` the same way the pinger's `state.json` avoids
re-alerting on steady state. If Railway is down for hours, expect
exactly one 🐶 message, not one per 30-minute tick.
