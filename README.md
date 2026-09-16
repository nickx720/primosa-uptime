# primosa-uptime

Free uptime monitoring for `*.primosa.ai` projects, independent of Railway/Fly.
A GitHub Actions cron hits each target's health URL every ~5 minutes and posts
to a Telegram group on up/down transitions only.

## Add a target

Edit `targets.json`, add `{ "name": "...", "url": "https://...", "expect": 200 }`.

## Set up the Telegram bot (group chat)

1. Message [@BotFather](https://t.me/BotFather), run `/newbot`, save the token.
2. Add the bot to your Telegram group.
3. Post any message in the group (e.g. "hello").
4. Run `curl https://api.telegram.org/bot<TOKEN>/getUpdates` and find
   `message.chat.id` — for a group this is a negative number like `-100...`.
   That's `TELEGRAM_CHAT_ID`.
5. No need to disable BotFather's `/setprivacy` — the bot only sends
   messages, it never needs to read group messages.

## GitHub secrets

In the repo's Settings > Secrets and variables > Actions, add:
- `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_CHAT_ID`

## Notes

- GitHub's cron scheduler can lag a few minutes behind the stated schedule.
- Scheduled workflows on a repo with no activity for 60 days get
  auto-disabled by GitHub. The `state.json` commits count as activity, so
  the workflow keeps itself alive as long as it keeps running.

## Local dry-run

Runs in Docker (matches how CI runs it — Go stdlib only, no local Go
toolchain required). Without the two env vars set, messages are printed
to stdout instead of sent:

```
make run
```

`make lint` runs `gofmt`/`go vet` locally if you have Go installed;
`make build` just builds the image.
