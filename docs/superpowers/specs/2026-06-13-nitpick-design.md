# nitpick — design spec

**Status:** approved design (2026-06-13). Covers phases 1–3.

## One-liner

A Claude Code gate that runs the `reliability-architect-review` skill at the right
moments, files every finding in a Dolt database, **blocks a push to `main` until the
must-fix (P0/P1) findings are genuinely fixed — or waived with a written reason** — and
hands the rest forward with enough context to fix later.

## Why it exists

Reliability reviews produce findings that either (a) must be fixed now or (b) get
deferred — and deferred work rots because nothing carries its context forward and nothing
re-surfaces it. nitpick makes the must-fix half *enforced* and the defer half *durable and
auditable*.

## Foundational decisions

1. **Focused, clean seams.** Built around `reliability-architect-review` specifically, but
   the persistence/gate/verification machinery (`engine`, `findings`, `loop`) is
   skill-agnostic; RAR specifics live in `rar/`. A second skill is addable later without a
   rewrite. We do NOT build the general framework now (YAGNI).
2. **Hard-block P0/P1** at the push-to-`main` gate, with per-finding **waivers** (written
   reason, recorded). P2/P3 auto-defer silently. The fix loop is **fuel-bounded** (stull);
   if fuel exhausts with open P0/P1, the push stays blocked until fixed or waived.
3. **Evidence-gated re-check + slimemold** verification. A claimed fix only counts when
   (a) a deterministic guard confirms cited evidence is real, (b) a scoped RAR re-check
   Cell says the finding is resolved, and (c) slimemold does not flag the "fixed" claim as
   `basis=vibes` / premature closure.

## Key architectural insight

stull Cells are *fenced oracles with a small decidable grammar* — they cannot emit a
free-form 5-phase review. So:

- **The review and the fixing** are done by the **main Claude session** (full context),
  prompted via stull's `Inject` effect, emitting findings in the skill's existing
  `RAR-NN` format.
- **nitpick deterministically parses** that structured output into Dolt rows.
- **Only the gate decisions are Cells** — tiny grammars (`resolved|unresolved`,
  `grounded|vibes`) that fit stull's model exactly.

stull = control flow + small decidable oracle checks. The session does the cognition.
Dolt holds the truth.

## Architecture

One `go install`'d binary with two faces:

```
nitpick/
  cmd/nitpick/       # go install target; subcommands
  engine/            # stull machine + hook dispatcher        (skill-agnostic)
  findings/          # Dolt schema, store, RAR-NN parser, defer(skill-agnostic)
  loop/              # evidence guards + re-check + slimemold  (skill-agnostic)
  rar/               # RAR triggers, prompt, RAR-NN parser     (RAR-specific)
  machine/           # the assembled stull spec.Machine
```

- **Hook dispatcher** (`nitpick hook`) — invoked by Claude Code hooks; runs the stull
  machine; blocks/injects. Wired by `nitpick install`.
- **CLI** (`nitpick review|list|resolve|waive|defer|init`) — invoked by the session (via
  Bash) or a human, to record/query findings in Dolt.

### Dolt access

The store shells out to the `dolt` CLI (`dolt sql -q ... --result-format json` for reads,
`dolt sql -q` for writes, `dolt add -A && dolt commit` per state change). No CGO, no
embedded engine, small binary — consistent with stull's stdlib ethos. Requires `dolt` on
PATH. A single standalone Dolt repo at `~/.local/share/nitpick/db` (override:
`$NITPICK_DB`), **not** inside the code repo (keeps findings out of git history) and
**not** shared with defn's code-graph DB (different concern). Findings are scoped by a
`repo` column. Optionally pushable to DoltHub for backup.

## Data model

```sql
CREATE TABLE findings (
  repo          VARCHAR(255) NOT NULL,   -- e.g. github.com/sudarkoff/twocal
  finding_id    VARCHAR(32)  NOT NULL,   -- RAR-03 (stable)
  skill         VARCHAR(64)  NOT NULL,   -- 'reliability-architect-review'
  severity      VARCHAR(4)   NOT NULL,   -- P0|P1|P2|P3
  status        VARCHAR(16)  NOT NULL,   -- open|resolved|deferred|waived
  promise       TEXT, component TEXT, failure_mode TEXT,
  detection_gap TEXT, recommendation TEXT,
  evidence      TEXT,                    -- sha:… / test:… / defn:… / alert:…
  waiver_reason TEXT,
  first_seen_at TIMESTAMP, resolved_at TIMESTAMP, deferred_at TIMESTAMP,
  session_id    VARCHAR(64),
  PRIMARY KEY (repo, finding_id)
);
```

Status semantics: `open` (must-fix, gates), `resolved` (verified fixed), `deferred`
(P2/P3 carried forward, or a P0/P1 explicitly waived — `waiver_reason` set), `waived` is
folded into `deferred` with a reason (single deferred state, reason distinguishes).
Every state change is a `dolt commit` → per-row audit trail (`dolt history`, `AS OF`):
when did we defer this, in which session, what changed.

## Triggers ("the right moments")

- **Hard gate — PreToolUse on push-to-main.** Matches the standing rule (RAR before every
  push to main) and is the last responsible moment. Matcher catches `git push` variants
  when the target is `main` (`git push`, `git push origin`, `git push origin main`,
  `git push -u origin main`).
- **SessionStart surface.** Inject "N open P0/P1, M deferred findings for this repo" at
  session start — the payoff of persistence; the backlog greets you.
- **Stop-hook sensitivity watch.** When a turn's diff touches reliability-sensitive paths
  (sync engine, jobs/queue, db pool, webhook handlers, external API clients, health
  checks) and no review covers it, nudge + mark "review due." Catches gaps when context is
  fresh so the push gate is rarely a surprise. Sensitivity paths are configurable.
- **Not now (future):** marketing-copy promise-audit (PreToolUse on uptime/SLA copy);
  deploy-gating is conceptually ideal but the prod deploy is a GitHub Actions
  `workflow_dispatch`, not locally hookable — out of scope.

## Implementation resolutions (from stull's real API, v0.1.0)

- **Pure guards / impure dispatcher.** stull guards must be pure (no DB/FS). So
  `nitpick hook` (the dispatcher) computes DB-derived facts — open-P0/P1 count,
  whether this push targets `main` (needs `git`), the session summary — and
  injects them as event fields (`event.nitpick_open_blockers`,
  `event.nitpick_block_reason`, `event.nitpick_summary`, `event.nitpick_review_due`).
  The machine's guards read those event fields via `event.<path>` Reads, exactly
  as stull's `denyguard` reads `event.tool_input.command`.
- **Push gate is hybrid.** Machine guard = `ToolIs(Bash) AND CommandMatches(/git
  push/) AND open_blockers>0`. The command match is the in-machine lens; the
  dispatcher sets `open_blockers>0` only when the push actually targets `main`
  (it checks the branch) and Dolt has open P0/P1.
- **Verification is CLI-side, not a hook machine.** `nitpick resolve` is a direct
  CLI call, not a hook event, so Phase-3 evidence verification (SHA/test/defn) +
  re-check + slimemold run inside the `resolve` code path, not inside the stull
  machine. stull owns the *gate*; the CLI owns *verification*.
- **Fuel = anti-brick, not "blocks forever".** stull's Fuel guarantees totality:
  the gate blocks for up to `Fuel` push attempts, then fails open *loudly*
  (stull's no-brick guarantee). Fuel is set generously (256); in practice a
  finding is fixed or waived long before the budget.
- **Wiring.** `nitpick hook` drives `runtime` (LoadContext -> enrich event with DB
  facts -> Dispatch -> SaveContext -> Emit). `nitpick install` =
  `compile.MergeHooks`. Sim tests use `sim.Run` (no model call); CI runs
  `check.Validate`.

## The stull state machine

```
state: idle
  on PreToolUse(Bash, cmd ~= push-to-main)
     guard: open P0/P1 in Dolt for this repo?      yes -> Block + Inject finding list -> gated
     guard: no review recorded this session?            -> Block + Inject RAR prompt   -> reviewing
     else                                               -> allow push                  -> clear

state: reviewing
  session runs RAR, calls `nitpick review --from <file>`; parser writes rows
  recompute open P0/P1:  >0 -> gated   ==0 -> clear

state: gated
  per `nitpick resolve RAR-NN --evidence …`:
     GUARD (deterministic): evidence real? sha/test/defn  --no--> reject
     CELL  re-check:        finding resolved?             --no--> reject
     CELL  slimemold:       basis=vibes / premature?      --yes--> reject
     all pass -> status=resolved
  per `nitpick waive RAR-NN --reason …` -> status=deferred (+reason)
  open P0/P1 == 0 -> clear
  fuel exhausted with open P0/P1 -> stays blocked (must fix or waive)

state: clear (terminal) -> push allowed
```

P2/P3 are written `deferred` on ingest and never gate.

## Integration points

- **stull** (`github.com/justinstimatze/stull`) — imported as a library; nitpick *is* a
  stull dispatcher. `nitpick install` merges its hook fragment into `~/.claude/settings.json`
  (idempotent, backup — stull's install pattern) and installs the `reliability-architect-review`
  skill into `~/.claude/skills/`.
- **defn** — optional, Go-only. Used as a `defn:` evidence type (confirm a structural
  change landed) and for blast-radius help while fixing. Absent/non-Go → evidence falls
  back to `sha:`/`test:`.
- **slimemold** — optional but recommended. Queried at the resolution gate to reject
  `basis=vibes`/premature-closure "it's fixed" claims. Absent → that sub-gate is skipped
  (evidence + re-check still required).
- `nitpick install` checks for defn/slimemold and prints what's degraded if missing;
  never hard-fails on them.

## Error handling / fail-safe

**Fail-open**, per stull's design: a dispatcher error allows the push. A bug in nitpick
must never permanently wedge the ability to push. Cost (one crash bypasses the gate) is
acceptable; a brick is not. Dolt writes are transactional (commit per transition).

## Testing

- `stull sim` (no API) over the machine: clean→unblock, open-P0→block, fake-evidence→reject,
  real-evidence→unblock, waiver path, fuel-exhaustion→stays-blocked.
- `stull check` static soundness in CI.
- Go unit tests: RAR-NN parser, Dolt store (against a temp dolt dir), each evidence guard.

## Build phases (this spec implements 1–3)

1. **Persistence + CLI** — Dolt store, RAR-NN parser, `init/review/list/resolve/waive/defer`.
   Usable by hand immediately.
2. **The gate** — stull machine + hook dispatcher + `install`: block push on open P0/P1,
   inject RAR prompt, SessionStart surface, Stop-watch.
3. **Verification loop** — evidence guards (sha/test/defn) + scoped re-check Cell +
   slimemold integration.
4. **(future)** marketing-copy trigger; DoltHub backup; a second skill via the clean seams.

## Defaults

name `nitpick`; standalone Dolt at `~/.local/share/nitpick/db` (`$NITPICK_DB` override);
trigger = PreToolUse push-to-main + SessionStart + Stop-watch; fail-open.

## Implementation status (2026-06-13)

- **Phase 1 — DONE, shipped.** `findings` (RAR-NN parser, Dolt store with
  re-ingest preservation, severity-policy ingest) + CLI (`init/review/list/
  resolve/waive/defer`). Tested + verified end-to-end.
- **Phase 2 — DONE, shipped.** `machine` (stull gate: push-to-main Block,
  SessionStart surface, Stop nudge; sim-tested, passes `check.Validate`) +
  `engine` (event-enriching dispatcher `nitpick run`, `nitpick install` via
  `compile.MergeHooks`). Verified: push-to-main with an open P0 blocks (exit 2);
  resolving unblocks it.
- **Phase 3 — DONE.** `loop.VerifyEvidence` verifies `sha:` (commit exists) and
  `test:` (a matching test runs and passes); non-verifiable evidence is rejected
  toward `waive`. `loop.Recheck` is a fenced resolved/unresolved oracle (a stull
  Cell driven by `runtime.AnthropicModel`) that runs after evidence passes: it
  degrades to skip without `ANTHROPIC_API_KEY` and fail-safe-releases on an
  out-of-language answer; only a clean "unresolved" rejects. `loop.SlimemoldConcerns`
  surfaces slimemold premature-closure/weak-basis flags as a NON-blocking advisory
  at resolve (project-wide epistemic state, not a per-finding verdict, so it
  informs rather than gates). `nitpick doctor` reports dependency availability.
  All wired into `nitpick resolve`; decision logic unit-tested with scripted
  models and a fake slimemold binary.
  - **Deferred:** `defn:` auto-verification — defn needs a per-repo graph
    (`defn init`), so it is honestly rejected (cite `sha:`/`test:` or `waive`)
    until repo-managed defn is in scope.

## Skill install + ingestion loop (2026-06-13)

`nitpick install` now installs the skill, not just the hooks: the
`reliability-architect-review` SKILL.md is vendored under `skills/` and embedded
via `go:embed`, so a `go install`-ed binary carries it. `install`
writes it to `<settings-dir>/skills/reliability-architect-review/` (backing up a
differing existing copy to `.bak`) and merges the hook fragment. The shipped
skill ends with a "Persisting findings to nitpick" step that runs
`nitpick review --from <file>` — closing the previously-missing loop from "review
emitted findings" to "findings in the DB". An engine test asserts the embedded
skill contains every parser field label, the `FINDING RAR` header form, and the
`nitpick review` call, so the skill and parser cannot drift apart silently.

## install/init split + git pre-push gate (2026-06-13)

Two setup scopes, matching `git init` conventions:

- **`nitpick install`** — machine setup (global by default, or `--project`):
  ensures the findings DB (`findings.Open`), installs the embedded skill, merges
  the Claude Code hook fragment. The Claude Code hooks gate the *agent's* pushes.
- **`nitpick init`** — repository setup: installs a git `pre-push` hook (in the
  repo's hooks dir, resolved via `git rev-parse --git-path hooks`, backing up a
  foreign hook to `.bak`). The hook calls `nitpick precheck`, which reads the
  pushed refs on stdin and exits non-zero to block a push to `main` while open
  P0/P1 findings remain. This closes the gap that the Claude Code hook can't:
  a push typed directly in the terminal. It fails open if nitpick is not on PATH
  and is bypassable with `git push --no-verify` (a git hook is a speed bump, not
  a wall). Per-repo by nature: git hooks live in `.git/hooks` and are not shared.

The DB init moved out of `init` (now repo-scoped) and into `install` (machine
setup). `precheck` is the internal git-hook callback; `refsPushToMain` (pure ref
parsing) and `initRepoAt`/`precheckAt` (dir-parameterized) are unit-tested, and
the real `git push` → hook → block chain was verified against a bare remote.

## install writes by default (2026-06-13)

`nitpick install` now applies changes by default; `--dry-run` previews instead
(the prior `--write` requirement is gone, accepted as a silent no-op for
compatibility). Because installing now runs `dolt init`, `findings.Open` passes
an explicit `--name`/`--email` to dolt so it no longer fails on a machine where
dolt has no global identity configured.
