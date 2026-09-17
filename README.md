# primosa-uptime

Free uptime monitoring for `*.primosa.ai` projects.

**Topology (two layers):**
1. **Primary**: an always-on Go service on Railway loops every
   `UPTIME_LOOP` (e.g. 60s), checks each target in `targets.json`, and
   posts to a Telegram group on up/down transitions only. It also
   answers `/status` posted to the group, checked each loop cycle, and
   serves `GET /healthz` for the watchdog below.
2. **Secondary (watchdog)**: a GitHub Actions workflow polls the
   Railway service's `/healthz` every 30 minutes. If it's unreachable or
   its last completed loop is stale, it sends ONE alert via a *separate*
   Telegram bot (`WATCHDOG_BOT_TOKEN`) to the same group — because if
   GitHub's cron were the only thing watching, a GitHub outage or a
   Railway outage both go unnoticed. This layer intentionally does not
   check business logic, only "is the pinger alive."

GitHub cron was originally the primary scheduler; it was demoted after
observed drops (delayed/dropped runs under load — see
`docs/003-approach-review.md`'s revision note) in favor of an always-on
Railway loop, with GitHub kept only as an independent watchdog.

See `docs/002-architecture.md` for the full flow and state machine, and
`docs/runbooks/deploy-railway.md` for deploying the Railway service.

## Add a target

Edit `targets.json`, add `{ "name": "...", "url": "https://...", "expect": 200 }`.
`targets.json` is baked into the Docker image at build time, so a new
target takes effect on the *next Railway deploy* (`railway up`), not
automatically — see `docs/runbooks/add-target.md`. A newly added target
is checked on the loop's next cycle after deploy and reported in a
one-line "👋 now monitoring" Telegram message on its first run, rather
than staying silent until it later flips status. Removing a target
drops it from `state.json` on the next cycle. To park a target without
deleting it, set `"enabled": false` (JSON has no comments) — it is
skipped and pruned from state.

`wavly.primosa.ai` (Wavly production) is intentionally absent until prod
is promoted; re-add it then.

## Set up the Telegram bots

Two separate bots, same group — kept separate so a watchdog alert never
depends on the same bot/token path the primary pinger uses.

**Pinger bot** (business alerts, `/status`):
1. Message [@BotFather](https://t.me/BotFather), run `/newbot`, save the token.
2. Add the bot to your Telegram group.
3. Post any message in the group (e.g. "hello").
4. Run `curl https://api.telegram.org/bot<TOKEN>/getUpdates` and find
   `message.chat.id` — for a group this is a negative number like `-100...`.
   That's `TELEGRAM_CHAT_ID` (shared by both bots below).
5. The bot needs to *read* group messages (to see `/status`). Message
   [@BotFather](https://t.me/BotFather), pick your bot, run
   `/setprivacy` → **Disable**. If you'd rather leave privacy mode on,
   make the bot a group admin instead — either option lets it see
   `/status` messages.

**Watchdog bot** (Railway-is-down alerts only):
1. Message [@BotFather](https://t.me/BotFather), `/newbot`, name it
   something like `Primosa Watchdog` / username `primosa_watchdog_bot`.
2. Add it to the same Telegram group.
3. No privacy-mode change needed — it only sends, never reads.

## `/status` command

Post `/status` (or `/status@primosa_uptime_bot`) in the Telegram group.
The Railway loop checks for it every cycle (`UPTIME_LOOP`, e.g. 60s) and
replies once, in-thread, with a status line per target using that
cycle's fresh check results:

```
📊 Status — 2026-09-16 18:40 UTC
✅ Wavly (staging) — up · 212 ms · up for 3h 12m
❌ Wavly — down (404) · down for 1h 05m
❌ Unsub — down (timeout) · down for 1h 05m
```

If the bot can't see the message at all, double check the BotFather
privacy setting above.

## Configuration (Railway service)

Environment variables:
- `UPTIME_LOOP` — e.g. `60s`. Empty = one-shot mode (used for local
  dry-runs and was the GitHub-cron mode; kept for that path).
- `STATE_PATH` — default `./state.json`; on Railway, `/data/state.json`
  (on the mounted volume, so it survives redeploys).
- `PORT` — default `8080`; serves `GET /healthz`.
- `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID` — the pinger bot.

`GET /healthz` returns `200 {"ok": true, "last_loop_at": "<RFC3339>", "targets": {...}}`,
or `503 {"ok": false, ...}` if the last completed loop is older than 3x
`UPTIME_LOOP`.

## GitHub secrets/variables (watchdog only)

In the repo's Settings > Secrets and variables > Actions:
- Secret `WATCHDOG_BOT_TOKEN`
- Secret `TELEGRAM_CHAT_ID` (same group)
- Variable `PINGER_HEALTHZ_URL` — the Railway service's public
  `/healthz` URL (from `railway domain`)

## Notes

- The watchdog's 30-minute cron can lag a few minutes; that's normal.
- Scheduled workflows on a repo with no activity for 60 days get
  auto-disabled by GitHub. `watchdog-state.json` is committed on every
  watchdog run specifically to keep that workflow "active" — `state.json`
  for the pinger itself now lives only on the Railway volume, not in git.

## Local dry-run

Runs in Docker (matches how CI runs it — Go stdlib only, no local Go
toolchain required). Without the two env vars set, messages are printed
to stdout instead of sent:

```
make run
```

Loop mode locally, for 2-3 cycles:

```
docker build -t primosa-uptime .
docker run --rm -p 8080:8080 -e UPTIME_LOOP=5s primosa-uptime
curl -s localhost:8080/healthz | jq .
```

`make lint` runs `go test` (in Docker), then `gofmt`/`go vet` locally if
you have Go installed; `make test` runs just the Go tests, in Docker
(no local Go toolchain required); `make build` just builds the image.
