---
name: reliability-architect-review
description: Use when reviewing architecture, implementation specs, codebase, or operational practices to verify that reliability and performance promises hold under load, failure, and scale. Use after incidents, before shipping reliability-adjacent features, when writing SLA or uptime copy, or during architecture planning sessions.
---

# Reliability Architect Review

## Overview

Adopt the mindset of a principal engineer who has operated large-scale public-facing services and been paged at 3am when they broke. The review has one goal: **surface every gap between the promises being made and the code that keeps them**, at current scale and at 10×, 100×, and 1000× growth.

The question driving every finding: *"If this breaks in production, how quickly do we know, and can we recover without waking someone up?"*

## When to Use

- After an incident — before calling it closed
- Before shipping anything that touches reliability, sync, queues, or external dependencies  
- When writing uptime/reliability marketing copy or SLA commitments
- During architecture proposals for new components
- Quarterly "are we still solvent?" reviews as the user base grows

## Review Framework

Work through these five phases in order. Skip none.

### Phase 1 — Promise Audit

Catalog every reliability promise being made to users, explicitly or implicitly:
- Uptime/availability claims ("always on," "never miss a sync")
- Latency claims ("syncs propagate in seconds")
- Data integrity claims ("events never get lost or duplicated")
- Recovery claims ("we alert you if sync breaks")

For each promise, locate the exact code or infrastructure that keeps it. A promise without a matching implementation is a liability.

**Red flag:** Marketing copy that describes desired behavior as if it's implemented behavior.

### Phase 2 — Failure Mode Analysis

For each major component (queue processor, external API client, DB layer, webhook handler), enumerate its top failure modes and answer:

| Failure mode | Detected in <5min? | Auto-recovers? | Runbook exists? |
|---|---|---|---|
| DB pool exhausted | ? | ? | ? |
| External API hung (no response) | ? | ? | ? |
| Job stuck in processing forever | ? | ? | ? |
| Webhook subscription expired | ? | ? | ? |
| Worker process healthy but doing nothing | ? | ? | ? |

**Unacceptable answers:** "No" in detected column for a user-facing failure. "No" in auto-recovers for anything that fails more than once a week.

**The liveness trap:** A process that responds to health checks is not the same as a process that is doing useful work. Health checks must verify the critical path, not just TCP connectivity. A worker that passes `/healthz` while its goroutines are deadlocked is worse than a worker that crashes — it silently fails for hours while appearing healthy.

### Phase 3 — Resource Exhaustion Scenarios

Every bounded resource is a potential cliff. Audit each one:

**Connection pools:** What is the max? What holds connections longest? What happens when the pool is full — does the caller time out, or block forever? Is there a timeout on pool.Acquire itself?

**Goroutine/thread pools:** What launches goroutines? Can they accumulate unboundedly? Are all external HTTP calls wrapped with context deadlines? What happens if a goroutine's external call never returns?

**Queue depth:** What is the maximum rate of job creation? What happens if workers fall behind? Is there backpressure? At what depth does user-visible latency degrade?

**File descriptors, memory:** What grows with user count? Is it bounded?

**Neon/external DB specifics:** Transaction-mode poolers (PgBouncer) only hold server connections during active transactions. A goroutine blocking on pool.Acquire while holding no DB connection but waiting for pool capacity is invisible to pg_stat_activity — it looks like idle capacity but isn't. Set acquire timeouts; never block forever.

### Phase 4 — Scale Tier Analysis

Run this analysis at three forcing functions: 10×, 100×, and 1000× current active users.

For each tier, ask:
- Which component saturates first?
- What is the failure mode at saturation (graceful degradation or hard fail)?
- What is the minimum viable change to push the cliff to the next tier?
- Is the change architectural (hard) or operational (config, scale out)?

**Common cliffs by tier:**

| Tier | Common first bottleneck |
|---|---|
| 10× | Single-machine DB connection pool; single-threaded job processing |
| 100× | DB write throughput; webhook fanout; single-region latency |
| 1000× | Schema design (global vs. per-tenant); cross-region consistency; cold-start latency |

Do not solve 1000× problems at 10× scale. Do not ignore the 10× cliff when you're at 1×.

### Phase 5 — Operational Readiness

For each failure mode identified in Phase 2, verify:

**Detection:** Is there a metric or log event that fires when this failure occurs? What is the detection lag (how long between failure and alert)?

**Alerting:** Is the metric connected to an alert? Is the alert threshold calibrated (not too noisy, not too silent)? Does the alert page the right person?

**Recovery — automatic:** Can the system self-heal without human intervention? Examples: restart on crash, retry with backoff, dead-letter queue with replay, watchdog that kills stuck jobs older than N minutes.

**Recovery — manual:** If human intervention is required, does a runbook exist? Is it tested? Is it short enough to follow at 3am?

**Key metric to instrument for any background job system:**
```
last_successful_job_completed_at (per job type, per worker)
```
Alert if this exceeds 2× the expected interval. This catches the silent failure: worker process alive, jobs never running.

---

## Anti-Patterns to Flag

These appear repeatedly across systems. Flag every instance found.

**No timeout on external HTTP calls.** An external API that stops responding without closing the connection will hang a goroutine forever. `http.Client` must have a `Timeout`. The Go default is no timeout.

**Pool acquire with no deadline.** Acquiring a connection from pgxpool/database/sql with a context that has no deadline means one saturated pool turns into a permanent deadlock. Always pass a context with timeout to pool operations.

**Health check that doesn't check the job path.** `/healthz` returning 200 while the worker's job goroutines are deadlocked is silent failure. Liveness should include: can I query the DB, and did a job complete recently?

**Goroutine leak on external call.** A goroutine blocked on `http.Get` or `WaitForNotification` with no context deadline is invisible to the pool but counts against system resources. Every blocking call needs a context with a deadline.

**Single retry loop with no dead-letter.** Jobs that fail repeatedly should be moved to a dead-letter state, not retried forever. Unbounded retries saturate the queue and starve healthy jobs.

**Alert on errors, not absence of success.** Error-rate alerts miss the failure mode where no errors are produced (silent stall). Monitor success rate and recency, not just error count.

**Making reliability promises before implementing the monitoring for them.** If you can't detect a failure mode within 5 minutes, you cannot honestly promise reliability for that component.

---

## Output Format

Structure findings as:

```
FINDING RAR-01 [P0/P1/P2/P3]
Promise at risk: <exact claim being made to users>
Component: <file, service, or layer>
Failure mode: <what breaks and how>
Detection gap: <how long before we know, or "undetected">
Recommendation: <specific, implementable fix>
```

**Finding IDs:** Give every finding a stable, human-readable ID of the form
`RAR-NN`, numbered sequentially (`RAR-01`, `RAR-02`, …) in order of appearance,
regardless of severity. The ID is so the reader can refer to a finding ("close
RAR-03") without retyping its title. Number across the *whole* review (do not
restart per phase or per severity), and never renumber a finding once assigned —
if severity changes on re-review, keep the ID and change only the `[Pn]` tag. If
the review produces a summary list, refer to findings by ID there too.

**Severity:**
- **P0** — Silent failure possible; user data at risk; or failure undetectable for >1 hour
- **P1** — Failure detectable but no auto-recovery; manual intervention required each time
- **P2** — Recovers automatically but detection lag is too high; or known cliff at next scale tier
- **P3** — Operational improvement; no immediate risk

---

## Scope by Scale Tier

When the target scale is specified, focus the review:

**Reviewing for "right now":** Phases 1–3 in full. Phase 4 at 10× only.

**Reviewing for 10K users:** All five phases. Prioritize connection pool sizing, queue throughput, and job liveness detection.

**Reviewing for 100K users:** Heavy emphasis on Phase 4. Look for architectural changes required (read replicas, queue partitioning, horizontal worker scaling). Verify that every config-only scaling option is documented before requiring code changes.

**Reviewing for 1M users:** Phase 4 only at this tier. Flag every component that is not horizontally scalable. Flag every place where user data is co-mingled without tenant isolation. Flag every place where a single slow user can affect others.

---

## The Question That Frames Everything

Before writing any finding, ask: *"If I'm a user whose sync has been silently broken for 9 hours, would anyone have noticed?"*

If the answer is no, that is a P0.

---

## Persisting findings to nitpick

The review is not complete until its findings are saved. After emitting every
`FINDING …` block above, persist them so the gate can enforce and track them:

1. Write all `FINDING …` blocks, verbatim and unmodified, to a file
   (for example `/tmp/nitpick-findings.txt`).
2. From the repository root, ingest them:

   ```bash
   nitpick review --from /tmp/nitpick-findings.txt
   ```

3. Confirm the output reads `ingested N findings`, where N equals the number of
   findings above. If it reports `ingested 0 findings`, the format drifted —
   verify each header line is exactly `FINDING RAR-NN [Pn]` (one of P0/P1/P2/P3)
   with nothing after the bracket, then re-run.

nitpick keys findings by the repository's git origin and by stable ID
(`RAR-NN`), so re-running the review updates findings in place — it never
reopens anything already resolved or waived. Once ingested, P0/P1 findings block
a push to `main` until they are fixed (`nitpick resolve <ID> --evidence …`) or
waived with a reason (`nitpick waive <ID> --reason …`); P2/P3 are filed for
later. List them anytime with `nitpick list`.
