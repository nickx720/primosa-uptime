# Runbook: monitor has gone silent

No alerts, but also no confirmation anything is actually running.

## Check 1 — is the workflow disabled?

GitHub auto-disables scheduled workflows on a repo with no activity for
60 days. Since `state.json` is committed on every run, an active monitor
keeps itself alive — silence for that long usually means it was already
disabled earlier and stopped self-renewing.

- Actions tab > uptime workflow > look for a banner saying the workflow
  was disabled due to inactivity.
- Fix: click "Enable workflow", then manually run `workflow_dispatch`
  once to confirm it goes green and re-establishes the commit cadence.

## Check 2 — cron lag

GitHub's scheduler can lag several minutes behind the stated cron. A gap
of 5-15 minutes between runs is normal, not a fault. Only escalate if the
gap is hours, not minutes.

## Check 3 — secrets rotated or missing

If `TELEGRAM_BOT_TOKEN` or `TELEGRAM_CHAT_ID` were rotated or removed
from GitHub Actions secrets, `main.go` silently falls back to
dry-run/console-log mode (see the `sendTelegram` function) — the
workflow still runs and commits `state.json`, but no Telegram message is
ever sent. Check the workflow run logs for `[dry-run] Telegram message:`
lines — if present, that's the smoking gun; re-add the secrets in
Settings > Secrets and variables > Actions.

## Check 4 — bot removed from the group, or chat_id wrong

If secrets are present but Telegram still isn't receiving messages,
check the run logs for `Telegram send failed: <status> <body>` — a 403
usually means the bot was removed from the group or blocked; a 400 often
means `TELEGRAM_CHAT_ID` is stale (e.g. group was upgraded to a
supergroup, which changes the id). Re-derive the chat id per the README
steps and update the secret.
