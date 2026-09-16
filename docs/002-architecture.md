# Architecture

Module boundary follows `.agents/skills/codebase-design`: `main.go` is a
single deep module (small public surface — read targets, check, send,
persist) rather than split into packages the tool is too small to need.

## Flow

```mermaid
flowchart LR
    A[GitHub Actions cron<br/>*/5 * * * *] --> L[Lint: gofmt + go vet<br/>in golang:alpine]
    L --> B[docker build]
    B --> R[docker run primosa-uptime]
    R --> C[targets.json]
    R -->|fetch each URL,<br/>2-attempt confirmation| D[Target health endpoints]
    D --> R
    R -->|diff vs previous| E[state.json]
    R -->|on up/down transition only| F[Telegram group]
    R -->|git commit + push| E
```

The workflow builds a Docker image (`golang:1.26-alpine` build stage,
`gcr.io/distroless/static` runtime) and runs it with the working
directory bind-mounted to `/data`, so `targets.json` is read and
`state.json` is written back to the checked-out repo, not baked into the
image. `state.json` is then committed back to the repo each run — this
both persists the last-known status per target and keeps the repo
"active" so GitHub doesn't auto-disable the scheduled workflow after 60
days of inactivity.

## State machine

`state.json` is `{ checked_at, targets: { <name>: { status, since } } }`.
Each target has a status of `up` or `down`. `checked_at` updates on
*every* run regardless of whether any target transitioned — see
"heartbeat" below.

```mermaid
stateDiagram-v2
    [*] --> Unknown: first run seeds state
    Unknown --> Up: first check succeeds
    Unknown --> Down: first check fails (after retry)
    Up --> Down: 2 consecutive failed attempts
    Down --> Up: check succeeds
    Down --> Down: still failing (no re-alert)
    Up --> Up: still succeeding (no alert)
```

- **2-attempt confirmation**: a target is only marked `down` after
  `checkOnce` fails, a `retryDelay` (10s) wait, and a second `checkOnce`
  also fails. This absorbs a single transient blip so it never becomes
  an alert — the core flap-avoidance mechanism.
- **Alert only on transition**: a per-target Telegram message is sent
  only when `newStatus != prev.Status`. Steady `up` or steady `down`
  across runs is silent.
- **First sighting produces one summary message, not silence**: a
  target with no prior entry in `state.json` is seeded without a
  per-target up/down alert, but every first-seen target from a run is
  collected and reported in a single Telegram message
  (`formatFirstSeenSummary` in `main.go`) — never one message per
  target. If every target in the run is first-seen (a brand-new
  `state.json`), the message uses a "👋 Uptime monitoring started"
  header followed by one line per target (✅/❌ + status, with a short
  down reason e.g. `down (404)` or `down (timeout)`). If only some
  targets are first-seen (a target added later), it uses a "👋 now
  monitoring" header instead — collapsed to a single line
  (`👋 now monitoring <name> — up`) when exactly one target is new.
- **`UPTIME_RESET`**: when this env var is set (non-empty), the run
  ignores any existing `state.json` and treats every target as
  first-seen, so the summary message can be re-sent on demand. Set via
  the `reset` boolean input on the `workflow_dispatch` trigger, which
  the workflow only passes through to `docker run` as
  `-e UPTIME_RESET=1` when true — scheduled runs never set it.
- **`Since` drives duration**: preserved across unchanged polls, reset
  only on a transition; used to compute the "back UP after Nm/Nh/Nd"
  message.
- **Heartbeat (`checked_at`)**: without a field that changes on every
  run, a repo whose targets stay steady never produces a diff, so the
  workflow's "commit if changed" step never commits — after 60 days of
  that, GitHub auto-disables the schedule even though it's running fine.
  `checked_at` guarantees a commit (hence "repo activity") on every run.
  See `docs/003-approach-review.md`.
