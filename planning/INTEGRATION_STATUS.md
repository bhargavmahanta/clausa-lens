# Integration Status — rolling coordination log

> Single source of truth for WHAT has landed on `team/integration`, WHO owns the
> last change, and what is BLOCKED. Read before implementing. Append, never
> rewrite, history. This file is maintained by the connector (Bhargav's
> coordination chat) as a read-only observer; the actual merges/pushes happen in
> Bhargav's implementing chat.
>
> Normative authorities (read first): `COMMON.md` (roles/branch model),
> `docs/CONTRACTS.md` (sole normative implementation authority),
> `planning/TEAM_WORKSTREAMS.md` (frozen ownership),
> `planning/E0-E3.md` and `planning/IMPLEMENTATION_ROADMAP.md` (phases/exit
> criteria). These win over anything in this log.

## Current branch state

- target_branch: `team/integration`
- tip_commit: `477d8b8`
- last_verified_commit: `477d8b8` (full suite green: build, test 16 pkgs, test -race, vet)
- combined_demo_status: `NOT_STARTED` (needs wired pack + one end-to-end run)

## Ownership (from `planning/TEAM_WORKSTREAMS.md`)

| Member  | Owned paths                                                   |
|---------|---------------------------------------------------------------|
| Trinabha | `cmd/demo-*`, `internal/capture`, `internal/systempack/checkout`, `test/fixtures/golden` |
| Shaurya  | `web`, `test/integration`                                     |
| Bhargav  | `cmd/core-api`, `cmd/replay-worker`, `internal/contracts`, `internal/core`, `internal/graph`, `internal/capsule`, `internal/replay`, `internal/differential`, `db/migrations`, `deploy/` |

`internal/contracts` changes require all-member review.

## Submission ledger (append-only; newest last)

| # | Member | Branch | Commit | Changed paths (owned only?) | Verified (build/test/test -race/vet) | Status | Date |
|---|--------|--------|--------|-----------------------------|--------------------------------------|--------|------|
| 1 | Bhargav | team/integration | 7b8d273 | COMMON.md | n/a (docs) | LANDED | 2026-08-29 |
| 2 | Bhargav | team/integration | 7fc971f | cmd/core-api, internal/core, internal/capsule, internal/replay, internal/differential, db/migrations, deploy | PASS | LANDED | 2026-08-29 |
| 3 | Trinabha | team/integration | 3109746 | internal/capture | PASS | LANDED | 2026-08-29 |
| 4 | Trinabha | team/integration | 5369ec6 | cmd/demo-* | PASS | LANDED | 2026-08-29 |
| 5 | Trinabha | team/integration | f4bbb41 | internal/systempack/checkout | PASS | LANDED | 2026-08-29 |
| 6 | Trinabha | team/integration | cf1ea47 | test/fixtures/golden | PASS | LANDED | 2026-08-29 |
| 7 | Bhargav | team/integration | 477d8b8 | cmd/core-api (packs.go + test) | PASS | LANDED | 2026-08-29 |

## Pool of pending submissions

- None. Both teammates have pushed their Member-1 scope; nothing awaiting merge.

## Blockers / CONTRACT CHANGE requests

| Raised by | Request | Owner action | Resolved? |
|-----------|---------|--------------|-----------|
| Trinabha | Register real pack (`RegisterDefault("checkout_duplicate_effect", checkout.New)`) so live routes resolve it | Bhargav: registered in `cmd/core-api/packs.go` @ `477d8b8` | YES |

## Next integration gates

- E1 exit criterion end-to-end (Trinabha, with wired pack): clean reset → golden request → one inspectable incident with timeout, retry, two payment attempts, two ledger effects, graph, true oracle evidence.
- Shaurya Member-3 submission (pending report).
- E2 exit criterion (baseline `COMPLETED`/`REPRODUCED`, isolation evidence, unsafe → `BLOCKED`).
- E3 what-if + diff (latency 350→50 removes timeout/retry/duplicate; first divergence highlighted).

## Last sync

- date: 2026-08-29
- submitter: Trinabha (P0 re-verification report), then Bhargav (pack wiring)
- verdict: PASS (both)
- outstanding: no merges pending; next is E1 end-to-end check and Shaurya's report.
