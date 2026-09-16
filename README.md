# primosa-uptime

Free uptime monitoring for `*.primosa.ai` projects, independent of Railway/Fly.
A GitHub Actions cron hits each target's health URL every ~5 minutes and posts
to a Telegram group on up/down transitions only. Post `/status` in the group
and the same cron run answers it (up to a ~5 minute lag — see below).

## Add a target

Edit `targets.json`, add `{ "name": "...", "url": "https://...", "expect": 200 }`.
A newly added target is checked immediately and reported in a one-line
"👋 now monitoring" Telegram message on its first run, rather than staying
silent until it later flips status. Removing a target drops it from
`state.json` on the next run.

`pol.primosa.ai` (Proof of Life production) is intentionally absent until prod
is promoted; re-add it then.

## Set up the Telegram bot (group chat)

1. Message [@BotFather](https://t.me/BotFather), run `/newbot`, save the token.
2. Add the bot to your Telegram group.
3. Post any message in the group (e.g. "hello").
4. Run `curl https://api.telegram.org/bot<TOKEN>/getUpdates` and find
   `message.chat.id` — for a group this is a negative number like `-100...`.
   That's `TELEGRAM_CHAT_ID`.
5. The bot now also needs to *read* group messages (to see `/status`).
   Message [@BotFather](https://t.me/BotFather), pick your bot, run
   `/setprivacy` → **Disable**. If you'd rather leave privacy mode on,
   make the bot a group admin instead — either option lets it see
   `/status` messages.

## `/status` command

Post `/status` (or `/status@primosa_uptime_bot`) in the Telegram group.
There's no always-on bot process — the next scheduled cron run (within
~5 minutes) notices the message and replies once, in-thread, with a
status line per target using that run's fresh check results:

```
📊 Status — 2026-09-16 18:40 UTC
✅ Proof of Life (staging) — up · 212 ms · up for 3h 12m
❌ Proof of Life — down (404) · down for 1h 05m
❌ Unsub — down (timeout) · down for 1h 05m
```

If the bot can't see the message at all, double check the BotFather
privacy setting above.

## GitHub secrets

In the repo's Settings > Secrets and variables > Actions, add:
- `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_CHAT_ID`

## Notes

- GitHub's cron scheduler can lag a few minutes behind the stated schedule.
- Scheduled workflows on a repo with no activity for 60 days get
  auto-disabled by GitHub. The `state.json` commits count as activity, so
  the workflow keeps itself alive as long as it keeps running.

## Re-sending the "now monitoring" summary

Manual trigger only: Actions tab > uptime > Run workflow > check `reset`.
This ignores the existing `state.json` for that run — every target is
treated as first-seen, so the full status summary is sent again to
Telegram (useful for confirming the alert path end to end without
waiting for a real transition). It does not delete `state.json`; the run
just overwrites it with fresh first-seen entries, same as any other run.

## Local dry-run

Runs in Docker (matches how CI runs it — Go stdlib only, no local Go
toolchain required). Without the two env vars set, messages are printed
to stdout instead of sent:

```
make run
```

`make lint` runs `go test` (in Docker), then `gofmt`/`go vet` locally if
you have Go installed; `make test` runs just the Go tests, in Docker
(no local Go toolchain required); `make build` just builds the image.
