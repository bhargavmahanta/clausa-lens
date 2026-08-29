# Command Center Completion Design

## Scope

Complete the CausaLens P0 Command Center entirely within `web/` and
`test/integration/`. The frontend consumes only the frozen v1.0 resources in
`docs/CONTRACTS.md`. Until live Core API verification succeeds, development and
browser checks use contract-faithful mocked HTTP responses and visibly identify
fixture mode.

## Architecture

The UI uses one typed `CausaLensClient` as its only API boundary. The client
owns HTTP methods, paths, expected status codes, request validation, response
decoding, and frozen API-error decoding. Components receive decoded resources
or explicit loading/error states and never embed successful replay, effect,
oracle, integrity, or diff results.

Local development routes expose the same `/v1` resource shapes beneath a
development-only base URL. Selecting the live Core API requires only changing
`NEXT_PUBLIC_CAUSALENS_API_URL`; components and workflow logic remain
unchanged.

## User Workflow

The page presents a judge-visible sequence:

1. Load and select an incident from the incident collection.
2. Inspect trace metadata, deterministic timeline, execution graph, and oracle
   evidence.
3. Compile and inspect the immutable Replay Capsule, including validation,
   integrity, fixture, plan, and default-deny safety evidence.
4. Start and monitor the baseline replay. Display execution status and outcome
   separately.
5. Unlock exactly one what-if control only after a safely completed,
   `REPRODUCED` baseline. The control exposes only `PAYMENT_LATENCY`, from
   `350 ms` to `50 ms`.
6. Start and monitor the what-if replay, then request and inspect the Replay
   Diff, including both effect summaries, signed effect deltas, oracle
   comparison, and first meaningful divergence.
7. Confirm demo reset before issuing it. On success, clear all selected
   incident, capsule, run, diff, error, and confirmation state before reloading
   the incident collection.

## State and Error Semantics

Incident collection states are `loading`, `empty`, `ready`, malformed resource,
and API error. Selected incidents may additionally be `DETECTED` or `BLOCKED`.
Replay stages use explicit idle, pending, active, terminal, and error states.

`ReplayRun.status` and `ReplayRun.outcome` are distinct everywhere. Active runs
never display an outcome. `COMPLETED`, `BLOCKED`, and `FAILED` retain their
contract meanings, while completed outcomes remain limited by run type.

Frozen API codes remain visible, especially `PACK_UNAVAILABLE`,
`INTEGRITY_MISMATCH`, `ISOLATION_VIOLATION`, `INTERVENTION_INVALID`, and
`INTERNAL_FAILURE`. Friendly copy may supplement but never replace the code.
Malformed success or error payloads are identified as unsupported Core API
resources; the UI does not substitute fixture values.

## Incident and Graph Presentation

The incident detail shows incident, trace, execution, oracle, System Pack, and
sanitization metadata. Timeline entries are joined to graph nodes by `event_id`
and sorted by `timeline_index`, regardless of the response array's input order.

The execution graph is rendered directly from API nodes and edges. Each node is
labelled with its timeline index and matching event metadata; each edge exposes
its frozen type and endpoints. Missing joins or malformed topology are rejected
at contract decoding rather than patched in the component.

## Capsule, Replay, and Diff Presentation

Capsule panels expose source identity, pack, trigger summary, fixtures, timing,
replay plan, oracle expectations, allowed intervention spec, safety policy, and
SHA-256 integrity. Validation and isolation claims are based only on returned
fields.

Run panels show lifecycle status, optional outcome, timing, effects, oracle
result, isolation evidence, and terminal error independently. What-if controls
use a fixed read-only intervention preview and emit only the exact contract
request.

Diff panels show baseline and comparison effect counts, signed deltas, both
oracle matches and explanations, event changes, added/removed counts,
limitations, and the first meaningful divergence when present. No absent
divergence is invented.

## Accessibility and Responsive Behavior

The workflow uses one page heading followed by ordered section headings,
labelled navigation, semantic buttons, status regions, and a confirmation
dialog. Inactive visual panels cannot receive focus. Focus moves to newly
selected workflow content or confirmation controls, and all actions remain
keyboard operable. Desktop retains the evidence-rich command-center layout;
mobile uses a single-column layout without clipped status or error content.

## Testing and Verification

Implementation follows test-first development. Unit and component tests cover:

- collection loading, empty, success, malformed response, and API error;
- timeline ordering by `timeline_index` and graph rendering from nodes/edges;
- status/outcome distinctions across `COMPLETED`, `REPRODUCED`, `MITIGATED`,
  `BLOCKED`, and `FAILED`;
- rejection or prevention of invalid what-if inputs;
- reset confirmation, request contract, and complete UI-state clearing;
- absence of fabricated incident, capsule, run, and diff values;
- exact mocked methods, paths, statuses, and payloads.

Completion requires `npm install`, lint, type-check, unit tests, and production
build from `web/package.json`, followed by `go test ./... -race` and
`go vet ./...`. Browser verification covers desktop and mobile widths,
navigation, keyboard focus order, accessibility structure, network traffic,
loading/error resilience, and zero console errors or warnings.

Live incident, replay, and diff verification remains explicitly blocked until
the Core API workflow can be exercised successfully. Mocked success must never
be reported as a live E1 pass.
