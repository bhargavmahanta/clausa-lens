# COMMON — CausaLens Team Workflow Context

> Read this before implementing anything. It is the shared, cross-workstream
> operating context for all CausaLens agents (Trinabha, Shaurya, Bhargav).
> It does **not** redefine any contract. For authoritative field names,
> enums, validation rules, and lifecycle semantics always read
> [`docs/CONTRACTS.md`](docs/CONTRACTS.md) — that file is the sole normative
> implementation authority and wins over anything here.

## Project

**CausaLens** — Distributed Incident Replay & Investigation.
North-star promise: **make distributed incidents replayable.**

Capture a request's execution across an instrumented distributed system,
reconstruct its path, compile the minimum controlled evidence into a
**Replay Capsule**, replay it inside an isolated environment, reproduce the
failure, change one approved condition, replay again, and inspect the **first
meaningful divergence**.

## The golden scenario (P0)

A checkout request experiences payment latency, times out, retries, and
creates a **duplicate ledger effect**. CausaLens captures that incident,
reconstructs its trace, compiles a valid Replay Capsule, **reproduces** the
duplicate effect in isolation, changes exactly one approved variable, and
highlights the first meaningful execution divergence.

```text
Gateway -> Checkout -> Payment Simulator -> Ledger
                |            |
                +---- 350ms latency ----+
```

P0 fixed values:
- Payment latency: `350 ms` (higher than timeout)
- Checkout timeout: `200 ms`
- Maximum payment attempts: `2`
- Deduplication: disabled
- P0 what-if intervention: `PAYMENT_LATENCY` `350 ms -> 50 ms`

Baseline must produce two payment attempts and two committed ledger effects.

## Product workflow

```text
Capture evidence -> Detect incident -> Reconstruct trace/timeline
  -> Compile Replay Capsule -> Verify isolation -> Run baseline replay
  -> Reproduce failure -> Change one condition -> Run what-if replay
  -> Find first divergence -> Produce evidence-backed explanation
```

## Team ownership (frozen — do not touch another member's paths)

| Owner | Branch | Owned paths | Builds |
|-------|--------|-------------|--------|
| **Bhargav** (Architecture & Core) | `team/integration` (owner) | `cmd/core-api`, `cmd/replay-worker`, `internal/contracts`, `internal/core`, `internal/graph`, `internal/capsule`, `internal/replay`, `internal/differential`, `db/migrations`, `deploy/compose.yaml` | Canonical contracts, Core API, PostgreSQL persistence, graph/timeline, capsule compilation, replay worker + isolation, baseline authorization, diff analyzer, orchestration |
| **Trinabha** (Distributed Systems) | own fork → rebase onto `team/integration` | `cmd/demo-*`, `internal/capture`, `internal/systempack/checkout`, `test/fixtures/golden` | Gateway/Checkout/Payment/Ledger demo, canonical event capture, `checkout_duplicate_effect` System Pack, failure oracle, sanitized golden fixtures, deterministic reset |
| **Shaurya** (Frontend & Command Center) | own fork → rebase onto `team/integration` | `web`, `test/integration` | Next.js/TypeScript Command Center, incident/trace/timeline/graph views, capsule + isolation evidence, baseline + what-if controls, Replay Diff, first-divergence UI, API integration tests |

**Rules:**
- Work **only** in your owned paths.
- `internal/contracts` is shared; changes there require **all-member review**.
- Trinabha implements the frozen `SystemPack` interface (already in
  `internal/contracts`) — do **not** edit it to fit your pack; implement it.
- Shaurya consumes **only** the frozen JSON contracts and live API — never
  hardcode replay results, effect counts, or explanations.

## Branch & integration model

```text
main (frozen, nothing merges here until the demo passes)
   └─ team/integration   (owned by Bhargav)
        1. Bhargav: A2 persisted Core + API   (base, committed)
        2. Trinabha: rebases onto team/integration -> builds demo+pack -> pushes
        3. Shaurya: rebases onto updated team/integration -> builds UI -> pushes
```

- Base your work on **`team/integration`**, not raw `main`.
- Rebase your fork onto `team/integration` when ready; contact Bhargav for
  integration/merge conflicts.
- Line endings are normalized to **LF** via `.gitattributes` — preserve LF.

## Key contracts (abbreviated — see CONTRACTS.md for exact shapes)

### ExecutionEvent
Required fields: `schema_version="1.0"`, `event_id`, `execution_id`,
`trace_id`, `component{name,instance}`, `operation{name,kind}`,
`event_type`, `attempt>=1`, `logical_operation_id`, `occurred_at` (RFC3339
UTC), `sequence>=0`, `status`, `attributes` (object).
Attributes allow-list: `configured_latency_ms`(int>=0),
`checkout_timeout_ms`(int>=1), `effect_id`(string),
`effect_committed`(bool), `dependency_name`(string).
Reject unknown fields, invalid enums, non-allow-listed attributes, and
duplicate event IDs.

### ReplayRun lifecycle
```text
CREATED -> VALIDATING -> RUNNING -> COMPLETED | FAILED | BLOCKED
```
Status and outcome are separate everywhere:
- `status`: `CREATED | VALIDATING | RUNNING | COMPLETED | FAILED | BLOCKED`
- `outcome`: `REPRODUCED | NOT_REPRODUCED | MITIGATED | UNCHANGED | INCONCLUSIVE`
- Baseline completed: `REPRODUCED | NOT_REPRODUCED | INCONCLUSIVE`
- What-if completed: `MITIGATED | UNCHANGED | INCONCLUSIVE`
- `COMPLETED` requires outcome, `completed_at`, effect summary, oracle result,
  and passing isolation evidence.
- `FAILED`/`BLOCKED` require `error`; other statuses forbid it.

### Baseline gate
A what-if replay is **only** authorized after a baseline with:
`status=COMPLETED`, `outcome=REPRODUCED`, and passing isolation evidence,
with a matching capsule hash.

### Error codes (frozen, no invented aliases)
`SCHEMA_INVALID | INTEGRITY_MISMATCH | PACK_UNAVAILABLE | FIXTURE_MISSING |
SANITIZATION_FAILED | ISOLATION_VIOLATION | DESTINATION_BLOCKED |
GRAPH_CYCLE | ORACLE_UNAVAILABLE | INTERVENTION_INVALID | INTERNAL_FAILURE`

### HTTP status mapping
Validation `400`, missing resource `404`, invalid lifecycle `409`, blocked
safety `422`, internal `500`. Error bodies use `APIErrorResponse`.
Response objects use the frozen contract objects.

## API resources (frozen)

| Method & path | Request | Success |
|---------------|---------|---------|
| `POST /v1/events` | `ExecutionEvent` | `202 AcceptedEventResponse` |
| `GET /v1/incidents` | `IncidentListQuery` | `200 IncidentListResponse` |
| `GET /v1/incidents/{incident_id}` | none | `200 IncidentDetailResponse` |
| `POST /v1/incidents/{incident_id}/capsules` | none | `201 ReplayCapsule` |
| `POST /v1/capsules/{capsule_id}/runs` | `CreateRunRequest` | `202 ReplayRun` |
| `GET /v1/runs/{run_id}` | none | `200 ReplayRun` |
| `POST /v1/diffs` | `CreateDiffRequest` | `201 ReplayDiff` |
| `GET /v1/diffs/{diff_id}` | none | `200 ReplayDiff` |
| `POST /v1/demo/reset` | `ResetRequest` | `200 ResetResult` |

## Checkpoint protocol

Each milestone, every owner reports:
**checkpoint, status, branch, commit, changed paths, verification commands,
results, dependencies, blockers, next steps.**

Verification baseline (run before claiming done):
```bash
go test ./...
go test ./... -race
go vet ./...
```
(`go` must be Go `1.26.7`.)

## Safety invariants (never weaken)

- Replay is **default-deny**: no production credentials, no production
  datastores, no uncontrolled outbound access at replay time.
- A production destination, degraded interaction, or failed teardown forces
  `verdict = FAIL`; such a run cannot be `COMPLETED`.
- Replay Capsules are **immutable**; changing content creates a new capsule ID.
- Baseline reproduction must pass before any what-if replay.
