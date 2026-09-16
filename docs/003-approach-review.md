# Approach review

Produced using `.agents/skills/codebase-design` (module boundary sanity
check) and `.agents/skills/yagni` (scope check) before finalizing the
architecture.

## Alternatives considered

- **GitHub Actions cron → Go binary → Telegram** (chosen). Runs in an
  infra domain independent of Railway/Fly, so a Railway-wide outage
  doesn't also take down the thing watching for it. Zero hosting cost.
  One repo covers every `*.primosa.ai` project — no per-project setup.
- **A small always-on cron service on Railway/Fly.** Rejected: shares a
  failure domain with the services it's supposed to watch. If Railway
  has an incident, the monitor goes down with the targets.
- **Hosted tool (Uptime Kuma self-hosted, UptimeRobot free tier).**
  Uptime Kuma still needs a host (same failure-domain problem, or a new
  bill). UptimeRobot's free tier is a real option but is a third-party
  dependency for a single owner's side projects, with its own account
  and alert-channel setup — more moving parts than a repo the owner
  already controls end to end. Deferred, not ruled out, if target count
  grows past what a 5-minute GitHub cron comfortably covers.
- **Railway/Fly native webhooks straight to Telegram.** These fire on
  deploy/crash events, not on runtime health, and each platform has its
  own webhook shape — would need per-platform glue instead of one
  uniform health-check loop. Folded into `docs/001-plan.md` phase 3 as
  a *supplement* to this pinger, not a replacement.

## Why this wins for this owner

Independent failure domain, zero cost, one repo for all projects, and
Go + Docker means CI needs no language-specific setup action — just a
container. Matches the owner's existing Go project conventions (see
`/home/nickk/Documents/unsub`).

## Known weaknesses and mitigations

- **5-minute cron granularity + scheduling lag.** GitHub's scheduler can
  run several minutes late. *Accepted*: this is a "did it come back"
  tool, not a paging system with SLA-grade latency.
- **No alert if GitHub Actions itself is down.** Single point of
  failure by construction — the monitor and the alert path share
  GitHub's infra. *Accepted*: mitigating this would mean a second
  independent monitor, which defeats the "zero cost, one thing to
  maintain" reason this approach was chosen in the first place (YAGNI).
- **60-day inactivity auto-disable.** *Found and fixed*, not just
  documented: the original design only committed `state.json` when a
  target's status changed. A repo with all targets steady for 60+ days
  would produce zero commits, GitHub would silently disable the
  schedule, and monitoring would stop with no signal to the owner. Fix:
  `state.json` now carries a top-level `checked_at` that updates on
  *every* run, so the "commit if changed" step always has something to
  commit. See `main.go`'s `State` struct and
  `docs/002-architecture.md`'s "Heartbeat" note.
- **State committed to git.** Every run's diff is a commit — acceptable
  at 5-minute intervals for ~4 targets today; would need reconsidering
  (e.g. batching, or a real datastore) only if the target count or check
  frequency grows by an order of magnitude. Not needed now (YAGNI).

## Verdict

Design holds. One concrete fix applied (heartbeat field); no other
changes needed.

## Revision: GitHub cron demoted to watchdog after observed drops (2026-09-16)

The "No alert if GitHub Actions itself is down" weakness above was
**accepted** on the reasoning that a second independent monitor would
defeat the "zero cost, one thing to maintain" rationale. That reasoning
held as long as GitHub's cron itself was reliable enough to trust as the
*only* scheduler. It isn't: GitHub's own docs note scheduled workflows
can be delayed or dropped under load, and in practice this repo observed
one scheduled run in a 75-minute window (expected: ~15 runs at */5).
A monitor that silently misses most of its runs is worse than a slower
one that's honest about staleness — the whole point is to know when
something's actually wrong.

**New split**: the owner already has a Railway-adjacent home for these
projects, so the "shares a failure domain with what it watches" concern
against a Railway-hosted monitor (raised above) is addressed differently
than rejecting Railway outright — by keeping GitHub in the loop as an
*independent second layer* instead of the primary:

- **Primary**: an always-on Go service loop on Railway (`UPTIME_LOOP`),
  checking targets every ~60s instead of every ~5 minutes, with no
  scheduler-reliability dependency at all — it's a live process, not a
  cron trigger.
- **Secondary**: GitHub Actions, demoted to a 30-minute watchdog that
  only asks "is the Railway pinger's `/healthz` responding and fresh."
  This keeps GitHub's original virtue (independent failure domain from
  Railway) without depending on its cron precision for the actual
  monitoring cadence — a watchdog firing late by 10-20 minutes is a
  minor drift, not a missed monitoring cycle.
- **Trade-off accepted**: the pinger and its own alert path (Telegram)
  now share Railway as a failure domain for the *primary* signal. This
  is why the watchdog exists — it's the fallback for exactly that case.
  A simultaneous failure of both Railway and GitHub is not covered
  (YAGNI: a third independent layer isn't justified by anything observed
  so far).
- **`state.json` no longer lives in git.** It moved to a Railway volume
  (`STATE_PATH=/data/state.json`) since the always-on process holds it
  in memory/disk locally between checks — committing it every 60s would
  be excessive git churn. The 60-day-inactivity concern shifts to the
  GitHub *watchdog* repo instead, covered by its own
  `watchdog-state.json` heartbeat commit.

See `docs/002-architecture.md` for the resulting two-layer flow diagram
and `docs/runbooks/deploy-railway.md` / `docs/runbooks/monitor-silent.md`
for the operational side.
