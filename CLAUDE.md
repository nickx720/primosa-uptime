# primosa-uptime

Free uptime monitoring for `*.primosa.ai` projects. A GitHub Actions cron
(every ~5 min) builds and runs a Go stdlib-only binary in Docker, checks
each target in `targets.json`, diffs against `state.json`, and posts to
a Telegram group only on up/down transitions. See README.md for setup;
`docs/002-architecture.md` for the state machine.

## YAGNI

This is a ~150-line Go tool by design (see `.agents/skills/yagni`). Do
not add dependencies, config layers, or abstractions beyond what the
task in front of you needs. If a change would roughly double the line
count, it probably belongs in a separate tool, not here.

## Session convention

Read `execution.json` (gitignored, session-resume state — not secrets)
first each session. Update it at the end of each session: repo state,
targets, secret names expected, and `next_up`.

## Running

- Dry-run locally (no secrets set, messages print to stdout instead of
  sending): `make run`
- Real run: same command, with `TELEGRAM_BOT_TOKEN` and `TELEGRAM_CHAT_ID`
  exported in the shell.
- `make lint` (gofmt + go vet), `make build` (image only).
- Manual trigger in CI: Actions tab > uptime > Run workflow
  (`workflow_dispatch`).

## Secrets

Never commit `TELEGRAM_BOT_TOKEN` or `TELEGRAM_CHAT_ID`. They live only
in GitHub Actions secrets. Local dry-run works without them.

## Process

Sonnet implements (`.claude/agents/uptime-ops-engineer.md`), Opus reviews
(`.claude/agents/sre-reviewer.md`) — flapping risk, secret hygiene,
workflow permissions.
