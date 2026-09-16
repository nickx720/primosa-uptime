# Runbook: add a target

1. Confirm the service exposes a health endpoint that returns 2xx when
   healthy (don't point this at a page that requires auth or redirects).
2. Add an entry to `targets.json`:
   ```json
   { "name": "My Service", "url": "https://my-service.primosa.ai/health", "expect": 200 }
   ```
3. Optionally dry-run locally to sanity-check the URL resolves and
   responds: `make run` (no Telegram secrets needed — prints
   `[dry-run]` lines instead of sending).
4. Commit and push, then redeploy the Railway service (`railway up` —
   see `docs/runbooks/deploy-railway.md`). `targets.json` is baked into
   the Docker image at build time, so a new target only takes effect
   after a redeploy, not automatically. The next loop cycle seeds the
   new target in `state.json` on its first check — this is silent, no
   alert fires for the initial seed.
5. Update `execution.json`'s `targets` list to keep session state in
   sync (not required for the workflow itself, just for continuity).
