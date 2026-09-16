# Runbook: deploy the pinger to Railway

The pinger runs as an always-on Railway service (primary). GitHub Actions
is now a 30-minute watchdog only (secondary) — see `.github/workflows/watchdog.yml`
and `docs/002-architecture.md`.

`targets.json` is baked into the Docker image at build time (`/app/targets.json`).
Editing it means committing + redeploying (`railway up`), same as any other
code change — there is no separate config store.

## Live deployment (as of 2026-09-16)

- Railway project `primosa-uptime` (workspace "Nick's Projects"):
  `24001f00-baf1-4016-a19a-9f478ef5ba4b`
  https://railway.com/project/24001f00-baf1-4016-a19a-9f478ef5ba4b
- Environment `production`: `6f47b978-5a2f-46eb-8484-6b4d48e75bc9`
- Service `pinger`: `6d2051ea-9dd8-461d-9add-ee7022c4951d`
- Volume `pinger-volume` (`d5e3b9c5-22d5-4d84-8a1c-85dda21cb9f4`) mounted at `/data`
- Public domain: https://pinger-production-3ee7.up.railway.app
  (`PINGER_HEALTHZ_URL` repo variable = `https://pinger-production-3ee7.up.railway.app/healthz`)
- Source: GitHub `nickx720/primosa-uptime` branch `main` — every push to
  `main` auto-deploys (Dockerfile build). `railway up` still works for an
  ad-hoc deploy of the working tree but is not needed for normal changes.

To link a fresh checkout: `railway link --project 24001f00-baf1-4016-a19a-9f478ef5ba4b`
then `railway service link pinger`.

## One-time setup

Run these from the repo root. Do not run them yourself as an agent —
these are the owner's steps.

```sh
# Install/auth the CLI if not already done
railway login

# Link this repo to a new (or existing) Railway project/service
railway init          # or: railway link, if the project already exists

# Create the volume that state.json lives on — survives redeploys
railway volume add --mount-path /data

# Set the required variables
railway variables --set "UPTIME_LOOP=60s" \
  --set "STATE_PATH=/data/state.json" \
  --set "TELEGRAM_BOT_TOKEN=<token>" \
  --set "TELEGRAM_CHAT_ID=<chat id>"

# Deploy
railway up

# Get the public URL for /healthz (used by the GitHub watchdog)
railway domain
```

After `railway domain` returns a URL, set it as a **repo variable** (not
secret) named `PINGER_HEALTHZ_URL` in GitHub: Settings > Secrets and
variables > Actions > Variables tab. Full URL including path, e.g.
`https://primosa-uptime-production.up.railway.app/healthz`.

`railway.json` at the repo root already configures the build (Dockerfile)
and deploy (`restartPolicyType: ALWAYS`, health check on `/healthz`,
60s timeout) — no dashboard clicking needed for that part.

## Redeploying after a targets.json change

```sh
git add targets.json
git commit -m "targets: add <service>"
git push            # push to main auto-deploys; `railway up` only if you need to skip git
```

## Verifying it's live

```sh
curl -s "$(railway domain)/healthz" | jq .
```

Expect `{"ok": true, "last_loop_at": "...", "targets": {...}}` within
one interval (`UPTIME_LOOP`) of deploy.

## Rolling back

`railway up` from a previous commit, or use the Railway dashboard's
deployment history to redeploy an earlier build. State on the volume
(`/data/state.json`) is untouched by a redeploy.
