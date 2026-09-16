# primosa-uptime

Free uptime monitoring for `*.primosa.ai` projects. A GitHub Actions cron
(every ~5 min) checks each target in `targets.json`, diffs against
`state.json`, and posts to a Telegram group only on up/down transitions.
See README.md for setup; `docs/002-architecture.md` for the state machine.

## YAGNI

This is a ~100-line project by design. Do not add dependencies, config
layers, or abstractions beyond what the task in front of you needs. If a
change would roughly double the line count, it probably belongs in a
separate tool, not here.

## Session convention

Read `execution.json` (gitignored, session-resume state — not secrets)
first each session. Update it at the end of each session: repo state,
targets, secret names expected, and `next_up`.

## Running

- Dry-run locally (no secrets set, messages print to stdout instead of
  sending): `npm run check`
- Real run: same command, with `TELEGRAM_BOT_TOKEN` and `TELEGRAM_CHAT_ID`
  exported in the shell.
- Manual trigger in CI: Actions tab > uptime > Run workflow
  (`workflow_dispatch`).

## Secrets

Never commit `TELEGRAM_BOT_TOKEN` or `TELEGRAM_CHAT_ID` to this repo.
They live only in GitHub Actions secrets (Settings > Secrets and
variables > Actions). Local dry-run works without them.

## Process

Sonnet implements (`.claude/agents/uptime-ops-engineer.md`), Opus reviews
(`.claude/agents/sre-reviewer.md`) per completed change — flapping risk,
secret hygiene, workflow permissions.
