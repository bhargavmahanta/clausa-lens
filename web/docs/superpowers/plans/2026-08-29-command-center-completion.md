# Command Center Completion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver and locally verify the complete contract-driven incident-to-diff Command Center using typed development HTTP fixtures while live Core API verification remains blocked.

**Architecture:** A single `CausaLensClient` remains the component-facing API boundary. Focused view models and a workflow reducer transform decoded v1.0 resources for presentation, while development-only Next.js route handlers delegate to a shared mutable fixture service that reproduces the exact Core API methods, paths, statuses, and payload shapes.

**Tech Stack:** Next.js 16.2.11, React 19.2, TypeScript 6.0, Zod 4.1, Vitest 4, Motion 13, CSS.

**Spec:** `web/docs/superpowers/specs/2026-08-29-command-center-completion-design.md`

## Global Constraints

- Modify only `web/` and `test/integration/`.
- Consume the frozen v1.0 names, enums, lifecycle rules, methods, paths, statuses, and payload shapes from `docs/CONTRACTS.md`.
- Successful values may exist in typed HTTP fixtures but never inside UI components.
- Expose only `PAYMENT_LATENCY` from `350 ms` to `50 ms`.
- Keep `ReplayRun.status` and `ReplayRun.outcome` visually and logically separate.
- Keep frozen error codes visible and do not report mocked success as a live E1 pass.
- Changing `NEXT_PUBLIC_CAUSALENS_API_URL` must switch to the live API without component changes.
- Preserve keyboard operation, sensible headings and labels, responsive layout, and zero browser console errors or warnings.

---

### Task 1: Contract-Driven Incident Timeline and Execution Graph

**Files:**
- Modify: `web/src/features/incidents/view-model.ts`
- Modify: `web/src/features/incidents/incident-dashboard.tsx`
- Modify: `web/tests/incidents/view-model.test.ts`
- Modify: `web/tests/components/incident-dashboard.test.tsx`

**Interfaces:**
- Consumes: `IncidentDetailResponse` decoded by `incidentDetailResponseSchema`.
- Produces: `buildIncidentView(detail): IncidentEvidenceView` with timeline entries sorted by `timelineIndex`, graph nodes joined to events, and edges retained exactly as returned.

- [ ] **Step 1: Write failing ordering and graph-join tests**

Add a test that reverses `goldenIncidentDetail.events`, calls `buildIncidentView`, and expects event IDs in graph `timeline_index` order. Add a component test that expects the rendered graph to include `#00`, `gateway`, `PARENT_CHILD`, source and target event IDs, while an arbitrary value absent from the response is absent from markup.

- [ ] **Step 2: Run focused tests and confirm failure**

Run `npm test -- --run tests/incidents/view-model.test.ts tests/components/incident-dashboard.test.tsx` from `web/`. Expect the reverse-order assertion or graph presentation assertion to fail against the current implementation.

- [ ] **Step 3: Implement deterministic joining and graph presentation**

Update the view model to build joined nodes:

```ts
type ExecutionGraphNodeView = {
  event: ExecutionEvent;
  timelineIndex: number;
};

const graphNodes = detail.graph.nodes
  .map((node) => ({ event: eventById.get(node.event_id), timelineIndex: node.timeline_index }))
  .filter((node): node is ExecutionGraphNodeView => Boolean(node.event))
  .sort((left, right) => left.timelineIndex - right.timelineIndex);
```

Render a labelled execution-graph section from `graphNodes` and `structuralEdges`; do not derive new edges or causality copy.

- [ ] **Step 4: Run focused tests and confirm pass**

Run `npm test -- --run tests/incidents/view-model.test.ts tests/components/incident-dashboard.test.tsx`. Expect all focused tests to pass.

- [ ] **Step 5: Commit the incident graph slice**

Commit only the four owned files with message `feat(web): render ordered incident execution graph`.

### Task 2: Complete Contract-Faithful Development HTTP Boundary

**Files:**
- Modify: `web/src/features/incidents/development-fixture.ts`
- Create: `web/src/features/replay/development-fixture.ts`
- Create: `web/src/features/replay/development-api.ts`
- Create: `web/src/app/api/dev/v1/incidents/[incidentId]/capsules/route.ts`
- Create: `web/src/app/api/dev/v1/capsules/[capsuleId]/runs/route.ts`
- Create: `web/src/app/api/dev/v1/runs/[runId]/route.ts`
- Create: `web/src/app/api/dev/v1/diffs/route.ts`
- Create: `web/src/app/api/dev/v1/diffs/[diffId]/route.ts`
- Create: `web/src/app/api/dev/v1/demo/reset/route.ts`
- Modify: `web/tests/incidents/development-routes.test.ts`
- Create: `web/tests/replay/development-routes.test.ts`

**Interfaces:**
- Consumes: request JSON decoded with `createRunRequestSchema`, `createDiffRequestSchema`, and `resetRequestSchema`.
- Produces: thin `GET`/`POST` Next.js handlers with exact contract success statuses (`201`, `202`, `200`) and `APIErrorResponse` failures.

- [ ] **Step 1: Write failing route contract tests**

Test the exact baseline and what-if request bodies, capsule and diff creation status codes, run polling progression, invalid intervention `400 INTERVENTION_INVALID`, missing resource `404 INTERNAL_FAILURE`, and reset result `200`. Parse every success body with its exported Zod schema and every failure with `apiErrorResponseSchema`.

- [ ] **Step 2: Run route tests and confirm missing handlers fail**

Run `npm test -- --run tests/incidents/development-routes.test.ts tests/replay/development-routes.test.ts`. Expect module resolution or endpoint assertions to fail.

- [ ] **Step 3: Implement typed fixture resources and handlers**

Create frozen fixture values satisfying `ReplayCapsule`, baseline `ReplayRun`, what-if `ReplayRun`, `ReplayDiff`, and `ResetResult`. Implement a shared in-memory development state that returns lifecycle snapshots without mutating resource fields and clears incidents, runs, diffs, and counters on reset. Route handlers must contain only parameter/body plumbing and calls to the fixture service.

- [ ] **Step 4: Run route and contract tests**

Run `npm test -- --run tests/incidents/development-fixture.test.ts tests/incidents/development-routes.test.ts tests/replay/development-routes.test.ts tests/contracts/resource-contracts.test.ts`. Expect all to pass.

- [ ] **Step 5: Commit the development API slice**

Commit the fixture, handler, and route-test files with message `feat(web): add contract-faithful replay HTTP fixtures`.

### Task 3: Replay Workflow State Machine and View Models

**Files:**
- Modify: `web/src/features/replay/workflow.ts`
- Modify: `web/src/features/replay/run-view.ts`
- Create: `web/src/features/replay/diff-view.ts`
- Create: `web/src/features/replay/command-center-state.ts`
- Modify: `web/tests/replay/workflow.test.ts`
- Modify: `web/tests/replay/run-view.test.ts`
- Create: `web/tests/replay/diff-view.test.ts`
- Create: `web/tests/replay/command-center-state.test.ts`

**Interfaces:**
- Consumes: decoded `ReplayCapsule`, `ReplayRun`, `ReplayDiff`, `CausaLensApiError`, and `ContractDecodeError`.
- Produces: pure reducer actions for compilation, baseline/what-if polling, diff creation, errors, reset confirmation, reset success, and collection reload; `buildRunView`; `buildDiffView`; `deriveWhatIfGate`.

- [ ] **Step 1: Write failing lifecycle, intervention, diff, and reset tests**

Assert active runs have no outcome row; baseline `COMPLETED/REPRODUCED`, what-if `COMPLETED/MITIGATED`, `BLOCKED`, and `FAILED` remain distinguishable; only the exact intervention object passes; diff counts and first divergence equal API values; reset success returns the initial clean state while failed reset preserves evidence and exposes its error.

- [ ] **Step 2: Run focused pure tests and confirm failure**

Run `npm test -- --run tests/replay/workflow.test.ts tests/replay/run-view.test.ts tests/replay/diff-view.test.ts tests/replay/command-center-state.test.ts`. Expect missing reducer and diff-view assertions to fail.

- [ ] **Step 3: Implement pure state transitions and projections**

Use a discriminated state with explicit resources:

```ts
type CommandCenterState = {
  incident: IncidentStage;
  capsule: ResourceStage<ReplayCapsule>;
  baseline: ResourceStage<ReplayRun>;
  whatIf: ResourceStage<ReplayRun>;
  diff: ResourceStage<ReplayDiff>;
  reset: "idle" | "confirming" | "submitting" | "completed" | "failed";
};
```

The reducer must never synthesize resource values. The only accepted what-if request is the literal frozen intervention and a safely reproduced matching baseline.

- [ ] **Step 4: Run focused pure tests and confirm pass**

Run the four focused test files and expect all to pass.

- [ ] **Step 5: Commit the workflow-model slice**

Commit the workflow and test files with message `feat(web): model replay and diff lifecycle states`.

### Task 4: Judge-Visible Replay Workspace and Orchestrator

**Files:**
- Create: `web/src/features/replay/replay-workspace.tsx`
- Create: `web/src/features/replay/replay-diff.tsx`
- Create: `web/src/features/replay/reset-dialog.tsx`
- Create: `web/src/features/command-center/command-center.tsx`
- Create: `web/src/features/command-center/index.ts`
- Modify: `web/src/app/page.tsx`
- Modify: `web/src/features/replay/index.ts`
- Modify: `web/tests/components/replay-workspace.test.tsx`
- Create: `web/tests/components/command-center.test.tsx`

**Interfaces:**
- Consumes: `CausaLensClient`, pure workflow state/actions, capsule/run/diff view models, and selected incident ID.
- Produces: accessible controls for capsule compilation, baseline replay, exact what-if replay, diff creation, and confirmed reset.

- [ ] **Step 1: Write failing component markup tests**

Assert capsule validation, integrity, isolation, separate status/outcome labels, exact fixed intervention, diff effect counts/oracles/divergence, visible frozen errors, confirmation dialog semantics, and absence of fixture-only values when resources are absent.

- [ ] **Step 2: Run focused component tests and confirm failure**

Run `npm test -- --run tests/components/replay-workspace.test.tsx tests/components/command-center.test.tsx`. Expect missing component modules or required copy assertions to fail.

- [ ] **Step 3: Implement the workspace and API orchestration**

The orchestrator calls only client methods, dispatches pending/success/failure actions, and polls active runs using `pollRunUntilTerminal`. It creates the diff only after both runs are safely completed. Reset opens a labelled modal, posts `{scenario_id: "checkout_duplicate_effect"}`, dispatches clean reset on success, and reloads incidents. Buttons are disabled whenever lifecycle gates are not satisfied.

- [ ] **Step 4: Run focused component tests and confirm pass**

Run the two focused component files and expect all tests to pass.

- [ ] **Step 5: Commit the workspace slice**

Commit the component, page, and test files with message `feat(web): complete incident replay command center`.

### Task 5: Responsive Styling and Accessibility Details

**Files:**
- Modify: `web/src/app/globals.css`
- Modify: `web/src/components/system/command-center-shell.tsx`
- Modify: `web/tests/components/foundation.test.tsx`
- Modify: `web/tests/components/home-page.test.tsx`

**Interfaces:**
- Consumes: semantic class names and state attributes from Tasks 1 and 4.
- Produces: desktop evidence layout, mobile single-column layout, visible focus indicators, stable loading/error sizing, and heading/navigation landmarks.

- [ ] **Step 1: Write failing semantic shell tests**

Assert one `main`, one page-level `h1`, labelled workflow navigation, a skip link target, and no focusable controls inside hidden/inert panels.

- [ ] **Step 2: Run shell tests and confirm failure**

Run `npm test -- --run tests/components/foundation.test.tsx tests/components/home-page.test.tsx`. Expect new semantic assertions to fail.

- [ ] **Step 3: Implement responsive and focus styling**

Add focused CSS sections for workflow navigation, evidence grids, graph, capsule, run, diff, status/error, and dialog. Add media queries at `960px` and `640px`; ensure long IDs and error details wrap with `overflow-wrap: anywhere`, controls remain at least `44px` high, and `:focus-visible` has a high-contrast outline.

- [ ] **Step 4: Run shell tests and confirm pass**

Run the two shell test files and expect all tests to pass.

- [ ] **Step 5: Commit the presentation slice**

Commit styling and shell tests with message `feat(web): polish responsive accessible workflow`.

### Task 6: Full Automated and Browser Verification

**Files:**
- Create: `test/integration/command-center-mock-boundary.test.ts` only if the `web/` suites cannot cover an exact browser-observed network assertion without a new browser dependency.
- Modify: `web/next-env.d.ts` only to restore the generated stable `./.next/types/routes.d.ts` reference if the development server changed it.

**Interfaces:**
- Consumes: the complete Command Center.
- Produces: an evidence-backed completion report and final owned-only commit.

- [ ] **Step 1: Run the frontend completion gate**

From `web/`, run `npm install`, `npm run lint`, `npm run typecheck`, `npm test`, and `npm run build`. Capture exact pass/fail counts and repair failures test-first.

- [ ] **Step 2: Run repository Go gates**

From the repository root, run `go test ./... -race` and `go vet ./...`. Record any blocker caused by the environment or another owner's code without modifying unowned paths.

- [ ] **Step 3: Run the local frontend and verify desktop**

Start `npm run dev`, open the local page at a desktop viewport, and exercise incident panels, capsule compilation, baseline, what-if, diff, and reset confirmation. Verify headings, labels, focus order, keyboard activation, network methods/paths/statuses/payloads, and zero console errors or warnings.

- [ ] **Step 4: Verify mobile and adverse states**

Repeat at a mobile viewport. Exercise loading, empty, frozen API error, and malformed-resource views through development fixture variants or route tests; confirm no clipping, overflow, focus loss, or layout collapse.

- [ ] **Step 5: Check ownership and commit final verification fixes**

Run `git diff --name-only upstream/team/integration...HEAD` and `git status --short`; every path must begin with `web/` or `test/integration/`. Commit remaining owned changes with message `test(web): verify command center completion`.

- [ ] **Step 6: Rename and push the feature branch**

Rename the local branch to `shaurya/command-center` if remote policy accepts slash-named branches, then push it without merging into `team/integration`. Because the branch was rebased, update only the owned feature branch and never `main`.
