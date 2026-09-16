# Plan

## Phase 0 — Scaffold ✅

`main.go` (Go stdlib only), `targets.json`, `Dockerfile`, `Makefile`,
`.github/workflows/uptime.yml`, README. Local dry-run works via
`make run` (no Telegram secrets set, prints to stdout) — verified
against a Docker build.

## Phase 1 — Go live

- [ ] Create the Telegram bot and group chat, get `TELEGRAM_CHAT_ID`
      (see README "Set up the Telegram bot").
- [ ] Add `TELEGRAM_BOT_TOKEN` and `TELEGRAM_CHAT_ID` as GitHub Actions
      secrets.
- [ ] Trigger `workflow_dispatch` once manually and confirm the run is
      green and `state.json` gets committed.
- [ ] Investigate: local dry-run against the real deployments shows
      Unsub and Watermark `/healthz` currently returning non-2xx — confirm
      whether the path is wrong or the services are actually unhealthy
      before going live (see `execution.json` `next_up`).

## Phase 2 — Add targets per project

As new `*.primosa.ai` (or other) services ship, add an entry to
`targets.json`: `{ "name", "url", "expect" }`. See
`docs/runbooks/add-target.md`.

## Phase 3 — Not now: deploy-crash webhooks

Idea: have Railway/Fly post deploy-crash events into the same Telegram
group via a small Cloudflare Worker (receives the platform webhook,
reformats, forwards to `sendMessage`). Deliberately out of scope until
the basic pinger has run reliably for a while — would add a second
delivery path and a new piece of infra to maintain, against the YAGNI
rule in `CLAUDE.md`.
