package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/causalens/causalens/internal/capsule"
	"github.com/causalens/causalens/internal/contracts"
	"github.com/causalens/causalens/internal/core"
	"github.com/causalens/causalens/internal/packregistry"
	"github.com/causalens/causalens/internal/systempack/checkout"
)

const validEventJSON = `{"schema_version":"1.0","event_id":"e1","execution_id":"exec","trace_id":"trace","component":{"name":"c","instance":"i"},"operation":{"name":"o","kind":"INTERNAL"},"event_type":"START","attempt":1,"logical_operation_id":"logical","occurred_at":"2026-08-29T10:32:01Z","sequence":0,"status":"RUNNING","attributes":{}}`

func request(t *testing.T, h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q", got)
	}
	return w
}

func TestEventSuccessGoldenBody(t *testing.T) {
	w := request(t, handler(core.NewStore()), http.MethodPost, "/v1/events", validEventJSON)
	if w.Code != http.StatusAccepted || w.Body.String() != "{\"event_id\":\"e1\",\"status\":\"ACCEPTED\"}\n" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestEventsRejectInvalidUnknownTrailingAndDuplicate(t *testing.T) {
	h := handler(core.NewStore())
	cases := []string{`{}`, strings.TrimSuffix(validEventJSON, "}") + `,"unknown":true}`, validEventJSON + `{}`}
	for n, body := range cases {
		w := request(t, h, http.MethodPost, "/v1/events", body)
		want := "{\"error\":{\"code\":\"SCHEMA_INVALID\",\"message\":\"request does not match the contract\",\"retryable\":false,\"details\":{}}}\n"
		if w.Code != http.StatusBadRequest || w.Body.String() != want {
			t.Fatalf("case %d: %d %s", n, w.Code, w.Body.String())
		}
	}
	if first := request(t, h, http.MethodPost, "/v1/events", validEventJSON); first.Code != http.StatusAccepted {
		t.Fatalf("setup: %d", first.Code)
	}
	w := request(t, h, http.MethodPost, "/v1/events", validEventJSON)
	want := "{\"error\":{\"code\":\"SCHEMA_INVALID\",\"message\":\"request does not match the contract\",\"retryable\":false,\"details\":{}}}\n"
	if w.Code != http.StatusBadRequest || w.Body.String() != want {
		t.Fatalf("duplicate: %d %s", w.Code, w.Body.String())
	}
}

type fakeRepository struct {
	list   contracts.IncidentListResponse
	detail contracts.IncidentDetailResponse
	err    error
	query  contracts.IncidentListQuery

	capsules     map[string]contracts.ReplayCapsule
	capsuleErr   error
	runs         map[string]contracts.ReplayRun
	runErr       error
	putRunErr    error
	diffs        map[string]contracts.ReplayDiff
	diffErr      error
	eventsByRun  map[string][]contracts.ExecutionEvent
	graphsByRun  map[string]contracts.ExecutionGraph
	resetCounts  core.ResetCounts
	resetErr     error
	transitioned *contracts.ReplayRun
}

func (f *fakeRepository) IngestEvent(context.Context, contracts.ExecutionEvent) error { return f.err }
func (f *fakeRepository) ListIncidents(_ context.Context, q contracts.IncidentListQuery) (contracts.IncidentListResponse, error) {
	f.query = q
	return f.list, f.err
}
func (f *fakeRepository) GetIncidentDetail(context.Context, string) (contracts.IncidentDetailResponse, error) {
	return f.detail, f.err
}

func (f *fakeRepository) PutCapsule(_ context.Context, c contracts.ReplayCapsule) error {
	if f.capsuleErr != nil {
		return f.capsuleErr
	}
	if f.capsules == nil {
		f.capsules = map[string]contracts.ReplayCapsule{}
	}
	f.capsules[c.CapsuleID] = c
	return nil
}
func (f *fakeRepository) GetCapsule(_ context.Context, id string) (contracts.ReplayCapsule, error) {
	if f.capsuleErr != nil {
		return contracts.ReplayCapsule{}, f.capsuleErr
	}
	c, ok := f.capsules[id]
	if !ok {
		return contracts.ReplayCapsule{}, core.ErrNotFound
	}
	return c, nil
}
func (f *fakeRepository) PutRun(_ context.Context, run contracts.ReplayRun) error {
	if f.putRunErr != nil {
		return f.putRunErr
	}
	if f.runErr != nil {
		return f.runErr
	}
	if f.runs == nil {
		f.runs = map[string]contracts.ReplayRun{}
	}
	f.runs[run.RunID] = run
	return nil
}
func (f *fakeRepository) GetRun(_ context.Context, id string) (contracts.ReplayRun, error) {
	if f.runErr != nil {
		return contracts.ReplayRun{}, f.runErr
	}
	run, ok := f.runs[id]
	if !ok {
		return contracts.ReplayRun{}, core.ErrNotFound
	}
	return run, nil
}
func (f *fakeRepository) TransitionRun(_ context.Context, from contracts.ReplayRunStatus, run contracts.ReplayRun) error {
	if f.runErr != nil {
		return f.runErr
	}
	f.transitioned = &run
	return nil
}
func (f *fakeRepository) PutDiff(_ context.Context, d contracts.ReplayDiff) error {
	if f.runErr != nil {
		return f.runErr
	}
	if f.diffs == nil {
		f.diffs = map[string]contracts.ReplayDiff{}
	}
	f.diffs[d.DiffID] = d
	return nil
}
func (f *fakeRepository) GetDiff(_ context.Context, id string) (contracts.ReplayDiff, error) {
	if f.diffErr != nil {
		return contracts.ReplayDiff{}, f.diffErr
	}
	d, ok := f.diffs[id]
	if !ok {
		return contracts.ReplayDiff{}, core.ErrNotFound
	}
	return d, nil
}
func (f *fakeRepository) EventsForRun(_ context.Context, run contracts.ReplayRun) ([]contracts.ExecutionEvent, error) {
	if f.runErr != nil {
		return nil, f.runErr
	}
	return f.eventsByRun[run.RunID], nil
}
func (f *fakeRepository) GraphsForRun(_ context.Context, run contracts.ReplayRun) (contracts.ExecutionGraph, error) {
	if f.runErr != nil {
		return contracts.ExecutionGraph{}, f.runErr
	}
	return f.graphsByRun[run.RunID], nil
}
func (f *fakeRepository) Reset(context.Context) (core.ResetCounts, error) {
	if f.resetErr != nil {
		return core.ResetCounts{}, f.resetErr
	}
	return f.resetCounts, nil
}

type fakePack struct {
	descriptor    contracts.SystemPackRef
	fixtures      contracts.FixtureSet
	plan          contracts.ReplayPlan
	interventions []contracts.InterventionSpec
	issues        []contracts.ValidationIssue
	alignment     *fakeAlignment
}

func (p *fakePack) Descriptor() contracts.SystemPackRef { return p.descriptor }
func (p *fakePack) Normalize(context.Context, contracts.RawEvidence) ([]contracts.ExecutionEvent, error) {
	return nil, nil
}
func (p *fakePack) DetectIncident(context.Context, []contracts.ExecutionEvent) (contracts.OracleResult, error) {
	return contracts.OracleResult{Oracle: contracts.FailureOracleRef{ID: p.descriptor.ID, Version: p.descriptor.Version}, Matched: true, EffectSummary: contracts.EffectSummary{PaymentAttemptCount: 2, LedgerCommitCount: 2}, RequiredEvidenceEventIDs: []string{"e"}, Explanation: "matched"}, nil
}
func (p *fakePack) ExtractFixtures(context.Context, contracts.Incident, []contracts.ExecutionEvent) (contracts.FixtureSet, error) {
	return p.fixtures, nil
}
func (p *fakePack) BuildReplayPlan(context.Context, contracts.Incident, contracts.FixtureSet) (contracts.ReplayPlan, error) {
	return p.plan, nil
}
func (p *fakePack) ValidateCapsule(context.Context, contracts.ReplayCapsule) []contracts.ValidationIssue {
	return p.issues
}
func (p *fakePack) AllowedInterventions() []contracts.InterventionSpec { return p.interventions }
func (p *fakePack) ApplyIntervention(context.Context, contracts.ReplayPlan, contracts.Intervention) (contracts.ReplayPlan, error) {
	return p.plan, nil
}
func (p *fakePack) Compare(context.Context, string, contracts.ReplayExecution, contracts.ReplayExecution) (contracts.ReplayDiff, error) {
	return contracts.ReplayDiff{}, nil
}
func (p *fakePack) EvaluateOutcome(context.Context, contracts.ReplayExecution) (contracts.OracleResult, error) {
	return contracts.OracleResult{Oracle: contracts.FailureOracleRef{ID: p.descriptor.ID, Version: p.descriptor.Version}, Matched: true, EffectSummary: contracts.EffectSummary{PaymentAttemptCount: 2, LedgerCommitCount: 2}, RequiredEvidenceEventIDs: []string{"e"}, Explanation: "ok"}, nil
}
func (p *fakePack) Labels() contracts.LabelSet { return contracts.LabelSet{} }

func (p *fakePack) Align(context.Context, string, contracts.ReplayExecution, contracts.ReplayExecution) ([]contracts.EventAlignment, []string, []string, []contracts.EventChange, error) {
	if p.alignment == nil {
		return nil, nil, nil, nil, errors.New("no alignment")
	}
	return p.alignment.matched, p.alignment.added, p.alignment.removed, p.alignment.changed, p.alignment.err
}

type fakeAlignment struct {
	matched []contracts.EventAlignment
	added   []string
	removed []string
	changed []contracts.EventChange
	err     error
}

var validDigest = strings.Repeat("a", 64)
var validDigestB = strings.Repeat("b", 64)

func newFakePack() *fakePack {
	return &fakePack{
		descriptor: contracts.SystemPackRef{ID: "checkout_duplicate_effect", Version: "1.0.0", InterfaceVersion: contracts.ContractVersion},
		fixtures: contracts.FixtureSet{
			StateFixtures: []contracts.StateFixture{{
				FixtureID:          "state-ledger-empty",
				Kind:               contracts.StateFixturePostgresRowset,
				ContentRef:         "fixture://golden/ledger-empty-v1",
				ContentDigest:      strings.Repeat("b", 64),
				SanitizationStatus: contracts.SanitizationPass,
				ResetStrategy:      contracts.FixtureTruncateAndLoad,
			}},
			DependencyFixtures: []contracts.DependencyFixture{{
				FixtureID:       "dependency-payment-350ms",
				Dependency:      contracts.DependencyPaymentSimulator,
				RequestMatch:    map[string]any{"logical_operation_id": "checkout-8271"},
				Response:        map[string]any{"status": "APPROVED"},
				LatencyMS:       350,
				FailureMode:     contracts.FailureModeNone,
				InvocationLimit: 2,
			}},
		},
		plan: contracts.ReplayPlan{
			Entrypoint:         "gateway.checkout",
			RequiredComponents: []string{"gateway", "checkout", "payment", "ledger"},
			FixtureLoadOrder:   []string{"state-ledger-empty", "dependency-payment-350ms"},
			ResetStrategy:      contracts.ReplayResetGoldenV1,
		},
		interventions: []contracts.InterventionSpec{{
			Type:      contracts.InterventionPaymentLatency,
			ValueType: contracts.InterventionValueInteger,
			Unit:      contracts.InterventionUnitMilliseconds,
			Minimum:   0,
			Maximum:   5000,
		}},
	}
}

func readyDetail() contracts.IncidentDetailResponse {
	return contracts.IncidentDetailResponse{
		Incident: contracts.Incident{
			SchemaVersion:      contracts.ContractVersion,
			IncidentID:         "inc-1",
			Status:             contracts.IncidentReady,
			FailureOracle:      contracts.FailureOracleRef{ID: "duplicate_ledger_effect", Version: "1.0.0"},
			SystemPack:         contracts.SystemPackRef{ID: "checkout_duplicate_effect", Version: "1.0.0", InterfaceVersion: contracts.ContractVersion},
			TraceID:            "trace-1",
			ExecutionID:        "exec-1",
			DetectedAt:         "2026-08-29T10:32:01.561Z",
			Summary:            "summary",
			EvidenceEventIDs:   []string{"e1"},
			GraphID:            "graph-1",
			SanitizationStatus: contracts.SanitizationPass,
		},
		Graph: contracts.ExecutionGraph{SchemaVersion: "1.0", GraphID: "graph-1", IncidentID: "inc-1", OrderingPolicyVersion: "1.0", Nodes: []contracts.GraphNode{{EventID: "e1", TimelineIndex: 0}}, Edges: []contracts.GraphEdge{}},
		Events: []contracts.ExecutionEvent{{
			SchemaVersion: "1.0", EventID: "e1", ExecutionID: "exec-1", TraceID: "trace-1",
			Component: contracts.ComponentRef{Name: "c", Instance: "i"}, Operation: contracts.OperationRef{Name: "o", Kind: contracts.OperationInternal},
			EventType: contracts.EventStart, Attempt: 1, LogicalOperationID: "logical",
			OccurredAt: "2026-08-29T10:32:01Z", Sequence: 0, Status: contracts.EventRunning, Attributes: map[string]any{},
		}},
	}
}

func TestCreateCapsuleReturnsValidCapsule(t *testing.T) {
	f := &fakeRepository{detail: readyDetail()}
	h := handlerWithDeps(f, HandlerDeps{Pack: newFakePack()})
	w := request(t, h, http.MethodPost, "/v1/incidents/inc-1/capsules", "")
	if w.Code != http.StatusCreated {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var got contracts.ReplayCapsule
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("capsule fails Validate: %v", err)
	}
	if got.CapsuleID != "cap-inc-1-1" {
		t.Fatalf("capsule_id = %q", got.CapsuleID)
	}
	if !sha256Re.MatchString(got.Integrity.Digest) {
		t.Fatalf("digest invalid: %q", got.Integrity.Digest)
	}
	if !capsule.VerifyDigest(got) {
		t.Fatal("VerifyDigest false")
	}
}

func TestCreateCapsuleIsIdempotent(t *testing.T) {
	f := &fakeRepository{detail: readyDetail()}
	h := handlerWithDeps(f, HandlerDeps{Pack: newFakePack()})
	first := request(t, h, http.MethodPost, "/v1/incidents/inc-1/capsules", "")
	if first.Code != http.StatusCreated {
		t.Fatalf("first: %d %s", first.Code, first.Body.String())
	}
	second := request(t, h, http.MethodPost, "/v1/incidents/inc-1/capsules", "")
	if second.Code != http.StatusCreated {
		t.Fatalf("second: %d %s", second.Code, second.Body.String())
	}
	if first.Body.String() != second.Body.String() {
		t.Fatal("re-requesting the same capsule must return the identical capsule")
	}
}

func TestCreateCapsuleNoPackUnavailable(t *testing.T) {
	f := &fakeRepository{detail: readyDetail()}
	w := request(t, handler(f), http.MethodPost, "/v1/incidents/inc-1/capsules", "")
	want := "{\"error\":{\"code\":\"PACK_UNAVAILABLE\",\"message\":\"replay capsule requires a System Pack\",\"retryable\":false,\"details\":{}}}\n"
	if w.Code != http.StatusUnprocessableEntity || w.Body.String() != want {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestResolvePackFollowsEnv(t *testing.T) {
	t.Setenv("PACK_IMPL", "dev")
	pack := resolvePack()
	if pack == nil {
		t.Fatal("PACK_IMPL=dev must resolve to a pack")
	}
	if got := pack.Descriptor().ID; got != "checkout_duplicate_effect_dev" {
		t.Fatalf("resolved pack id = %q", got)
	}

	t.Setenv("PACK_IMPL", "")
	if pack := resolvePack(); pack != nil {
		t.Fatal("empty PACK_IMPL must resolve to no pack")
	}
	t.Setenv("PACK_IMPL", "unknown-token")
	if pack := resolvePack(); pack != nil {
		t.Fatal("unknown PACK_IMPL must resolve to no pack")
	}
}

func TestCreateCapsuleWithResolvedPackSucceeds(t *testing.T) {
	t.Setenv("PACK_IMPL", "dev")
	f := &fakeRepository{detail: readyDetail()}
	h := handlerWithDeps(f, HandlerDeps{Pack: resolvePack()})
	w := request(t, h, http.MethodPost, "/v1/incidents/inc-1/capsules", "")
	if w.Code != http.StatusCreated {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var got contracts.ReplayCapsule
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("capsule fails Validate: %v", err)
	}
	if !capsule.VerifyDigest(got) {
		t.Fatal("VerifyDigest false")
	}
}

func TestCreateDiffWithResolvedPackSucceeds(t *testing.T) {
	t.Setenv("PACK_IMPL", "dev")
	f := &fakeRepository{
		runs: map[string]contracts.ReplayRun{
			"run-base": goldenBaseline(),
			"run-comp": goldenWhatIf(),
		},
		eventsByRun: map[string][]contracts.ExecutionEvent{
			"run-base": goldenEvents("be1", "be2"),
			"run-comp": goldenEvents("ce1"),
		},
		graphsByRun: map[string]contracts.ExecutionGraph{
			"run-base": goldenGraph("be1", "be2"),
			"run-comp": goldenGraph("ce1"),
		},
	}
	h := handlerWithDeps(f, HandlerDeps{Pack: resolvePack()})
	w := request(t, h, http.MethodPost, "/v1/diffs", `{"baseline_run_id":"run-base","comparison_run_id":"run-comp"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var diff contracts.ReplayDiff
	if err := json.Unmarshal(w.Body.Bytes(), &diff); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := diff.Validate(); err != nil {
		t.Fatalf("diff fails Validate: %v", err)
	}
}

func TestCreateRunAcceptsWhatIf(t *testing.T) {
	f := &fakeRepository{capsules: map[string]contracts.ReplayCapsule{
		"cap-1": {CapsuleID: "cap-1", Integrity: contracts.Integrity{Algorithm: contracts.IntegritySHA256, Digest: validDigest}},
	}}
	w := request(t, handler(f), http.MethodPost, "/v1/capsules/cap-1/runs", `{"run_type":"WHAT_IF","baseline_run_id":"run-base","intervention":{"type":"PAYMENT_LATENCY","from":350,"to":50,"unit":"ms"}}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var run contracts.ReplayRun
	if err := json.Unmarshal(w.Body.Bytes(), &run); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := run.Validate(); err != nil {
		t.Fatalf("run fails Validate: %v", err)
	}
	if run.RunType != contracts.RunTypeWhatIf || run.Status != contracts.ReplayRunCreated || run.CapsuleHash != validDigest {
		t.Fatalf("run fields wrong: %+v", run)
	}
}

func TestCreateRunWhatIfRejectingBaselineConflicts(t *testing.T) {
	f := &fakeRepository{
		capsules:  map[string]contracts.ReplayCapsule{"cap-1": {CapsuleID: "cap-1", Integrity: contracts.Integrity{Algorithm: contracts.IntegritySHA256, Digest: validDigest}}},
		putRunErr: core.ErrInvalidLifecycle,
	}
	w := request(t, handler(f), http.MethodPost, "/v1/capsules/cap-1/runs", `{"run_type":"WHAT_IF","baseline_run_id":"run-base","intervention":{"type":"PAYMENT_LATENCY","from":350,"to":50,"unit":"ms"}}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	assertEnvelope(t, w.Body.String(), contracts.SchemaInvalid)
}

func TestGetRun(t *testing.T) {
	run := contracts.ReplayRun{SchemaVersion: "1.0", RunID: "run-1", ExecutionID: "e", CapsuleID: "c", CapsuleHash: validDigest, RunType: contracts.RunTypeBaseline, TrialNumber: 1, Status: contracts.ReplayRunCreated}
	f := &fakeRepository{runs: map[string]contracts.ReplayRun{"run-1": run}}
	w := request(t, handler(f), http.MethodGet, "/v1/runs/run-1", "")
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var got contracts.ReplayRun
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.RunID != "run-1" {
		t.Fatalf("run id: %+v", got)
	}
	missing := request(t, handler(&fakeRepository{}), http.MethodGet, "/v1/runs/missing", "")
	assertEnvelope(t, missing.Body.String(), contracts.SchemaInvalid)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing: %d", missing.Code)
	}
}

func TestCreateDiffReturnsValidDiff(t *testing.T) {
	pack := newFakePack()
	pack.alignment = &fakeAlignment{
		matched: []contracts.EventAlignment{{BaselineEventID: "be1", ComparisonEventID: "ce1"}},
		added:   []string{},
		removed: []string{"be2"},
		changed: []contracts.EventChange{{BaselineEventID: "be1", ComparisonEventID: "ce1", Field: "event_type", BaselineValue: "FAILED", ComparisonValue: "SUCCESS"}},
	}
	f := &fakeRepository{
		runs: map[string]contracts.ReplayRun{
			"run-base": goldenBaseline(),
			"run-comp": goldenWhatIf(),
		},
		eventsByRun: map[string][]contracts.ExecutionEvent{
			"run-base": goldenEvents("be1", "be2"),
			"run-comp": goldenEvents("ce1"),
		},
		graphsByRun: map[string]contracts.ExecutionGraph{
			"run-base": goldenGraph("be1", "be2"),
			"run-comp": goldenGraph("ce1"),
		},
	}
	w := request(t, handlerWithDeps(f, HandlerDeps{Pack: pack}), http.MethodPost, "/v1/diffs", `{"baseline_run_id":"run-base","comparison_run_id":"run-comp"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var diff contracts.ReplayDiff
	if err := json.Unmarshal(w.Body.Bytes(), &diff); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := diff.Validate(); err != nil {
		t.Fatalf("diff fails Validate: %v", err)
	}
	if diff.BaselineRunID != "run-base" || diff.ComparisonRunID != "run-comp" {
		t.Fatalf("diff runs: %+v", diff)
	}
}

func TestCreateDiffPrerequisiteFailureConflicts(t *testing.T) {
	pack := newFakePack()
	pack.alignment = &fakeAlignment{}
	f := &fakeRepository{
		runs: map[string]contracts.ReplayRun{
			"run-base": goldenBaseline(),
			"run-comp": goldenWhatIf(),
		},
	}
	comp := f.runs["run-comp"]
	comp.BaselineRunID = "other"
	f.runs["run-comp"] = comp
	w := request(t, handlerWithDeps(f, HandlerDeps{Pack: pack}), http.MethodPost, "/v1/diffs", `{"baseline_run_id":"run-base","comparison_run_id":"run-comp"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	assertEnvelope(t, w.Body.String(), contracts.SchemaInvalid)
}

func TestGetDiff(t *testing.T) {
	f := &fakeRepository{diffs: map[string]contracts.ReplayDiff{"diff-1": {SchemaVersion: "1.0", DiffID: "diff-1", BaselineRunID: "a", ComparisonRunID: "b"}}}
	w := request(t, handler(f), http.MethodGet, "/v1/diffs/diff-1", "")
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	missing := request(t, handler(&fakeRepository{}), http.MethodGet, "/v1/diffs/missing", "")
	assertEnvelope(t, missing.Body.String(), contracts.SchemaInvalid)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing: %d", missing.Code)
	}
}

func TestPostResetReturnsFrozenValues(t *testing.T) {
	f := &fakeRepository{resetCounts: core.ResetCounts{Incidents: 3, Runs: 2}}
	w := request(t, handler(f), http.MethodPost, "/v1/demo/reset", `{"scenario_id":"checkout_duplicate_effect"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var res contracts.ResetResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := res.Validate(); err != nil {
		t.Fatalf("ResetResult fails Validate: %v", err)
	}
	if res.ResetID != "reset-1" || res.Status != contracts.ResetCompleted || res.ClearedIncidentCount != 3 || res.ClearedRunCount != 2 || res.ClearedLedgerCount != 0 {
		t.Fatalf("frozen reset values: %+v", res)
	}
}

func assertEnvelope(t *testing.T, body string, code contracts.ErrorCode) {
	t.Helper()
	if !strings.Contains(body, `"code":"`+string(code)+`"`) || !strings.Contains(body, `"details":{}`) {
		t.Fatalf("bad envelope: %s", body)
	}
}

var sha256Re = regexp.MustCompile(`^[0-9a-f]{64}$`)

func goldenBaseline() contracts.ReplayRun {
	return contracts.ReplayRun{
		SchemaVersion: contracts.ContractVersion, RunID: "run-base", ExecutionID: "exec-base", CapsuleID: "cap-1", CapsuleHash: validDigest,
		RunType: contracts.RunTypeBaseline, TrialNumber: 1, Status: contracts.ReplayRunCompleted, Outcome: contracts.ReplayOutcomeReproduced,
		StartedAt: "2026-08-29T10:34:00Z", CompletedAt: "2026-08-29T10:34:01Z",
		EffectSummary:       &contracts.EffectSummary{PaymentAttemptCount: 2, LedgerCommitCount: 2},
		FailureOracleResult: &contracts.OracleResult{Oracle: contracts.FailureOracleRef{ID: "duplicate_ledger_effect", Version: "1.0.0"}, Matched: true, EffectSummary: contracts.EffectSummary{PaymentAttemptCount: 2, LedgerCommitCount: 2}, RequiredEvidenceEventIDs: []string{"be1", "be2"}, Explanation: "reproduced"},
		IsolationEvidence:   &contracts.IsolationEvidence{PolicyVersion: contracts.ContractVersion, Verdict: contracts.VerdictPass, RuntimeNamespace: "ns", NetworkPolicy: contracts.VerdictPass, CredentialProfile: contracts.CredentialReplayOnly, DatastoreDestinations: []string{"postgres://replay/ledger"}, SimulatorInteractions: []contracts.DependencyInteraction{}, DeniedInteractions: []contracts.DependencyInteraction{}, TeardownResult: contracts.VerdictPass},
	}
}

func goldenWhatIf() contracts.ReplayRun {
	return contracts.ReplayRun{
		SchemaVersion: contracts.ContractVersion, RunID: "run-comp", ExecutionID: "exec-comp", CapsuleID: "cap-1", CapsuleHash: validDigest,
		RunType: contracts.RunTypeWhatIf, BaselineRunID: "run-base", Intervention: &contracts.Intervention{Type: contracts.InterventionPaymentLatency, From: 350, To: 50, Unit: contracts.InterventionUnitMilliseconds},
		TrialNumber: 1, Status: contracts.ReplayRunCompleted, Outcome: contracts.ReplayOutcomeMitigated,
		StartedAt: "2026-08-29T10:35:00Z", CompletedAt: "2026-08-29T10:35:01Z",
		EffectSummary:       &contracts.EffectSummary{PaymentAttemptCount: 1, LedgerCommitCount: 1},
		FailureOracleResult: &contracts.OracleResult{Oracle: contracts.FailureOracleRef{ID: "duplicate_ledger_effect", Version: "1.0.0"}, Matched: false, EffectSummary: contracts.EffectSummary{PaymentAttemptCount: 1, LedgerCommitCount: 1}, RequiredEvidenceEventIDs: []string{"ce1"}, Explanation: "mitigated"},
		IsolationEvidence:   &contracts.IsolationEvidence{PolicyVersion: contracts.ContractVersion, Verdict: contracts.VerdictPass, RuntimeNamespace: "ns", NetworkPolicy: contracts.VerdictPass, CredentialProfile: contracts.CredentialReplayOnly, DatastoreDestinations: []string{"postgres://replay/ledger"}, SimulatorInteractions: []contracts.DependencyInteraction{}, DeniedInteractions: []contracts.DependencyInteraction{}, TeardownResult: contracts.VerdictPass},
	}
}

func goldenEvents(ids ...string) []contracts.ExecutionEvent {
	events := make([]contracts.ExecutionEvent, len(ids))
	for i, id := range ids {
		events[i] = contracts.ExecutionEvent{
			SchemaVersion: contracts.ContractVersion, EventID: id, ExecutionID: "exec", TraceID: "trace",
			Component: contracts.ComponentRef{Name: "c", Instance: "i"}, Operation: contracts.OperationRef{Name: "o", Kind: contracts.OperationInternal},
			EventType: contracts.EventStart, Attempt: 1, LogicalOperationID: "logical",
			OccurredAt: "2026-08-29T10:32:01Z", Sequence: i, Status: contracts.EventRunning, Attributes: map[string]any{},
		}
	}
	return events
}

func goldenGraph(ids ...string) contracts.ExecutionGraph {
	nodes := make([]contracts.GraphNode, len(ids))
	for i, id := range ids {
		nodes[i] = contracts.GraphNode{EventID: id, TimelineIndex: i}
	}
	return contracts.ExecutionGraph{SchemaVersion: contracts.ContractVersion, GraphID: "graph-" + strings.Join(ids, "-"), IncidentID: "inc-1", OrderingPolicyVersion: contracts.ContractVersion, Nodes: nodes, Edges: []contracts.GraphEdge{}}
}

func TestListExactShapeAndQuery(t *testing.T) {
	f := &fakeRepository{list: contracts.IncidentListResponse{Items: []contracts.Incident{}, NextCursor: "next"}}
	w := request(t, handler(f), http.MethodGet, "/v1/incidents?status=READY&limit=1&cursor=cursor", "")
	if w.Code != 200 || w.Body.String() != "{\"items\":[],\"next_cursor\":\"next\"}\n" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if f.query.Status != contracts.IncidentReady || f.query.Cursor != "cursor" || f.query.Limit == nil || *f.query.Limit != 1 {
		t.Fatalf("query: %#v", f.query)
	}
}

func TestListRejectsInvalidQuery(t *testing.T) {
	for _, target := range []string{"/v1/incidents?status=NOPE", "/v1/incidents?limit=0", "/v1/incidents?limit=101", "/v1/incidents?limit=x"} {
		w := request(t, handler(&fakeRepository{}), http.MethodGet, target, "")
		if w.Code != 400 {
			t.Fatalf("%s: %d", target, w.Code)
		}
	}
}

func TestMissingDetailExactEnvelope(t *testing.T) {
	w := request(t, handler(&fakeRepository{err: core.ErrNotFound}), http.MethodGet, "/v1/incidents/missing", "")
	want := "{\"error\":{\"code\":\"SCHEMA_INVALID\",\"message\":\"resource not found\",\"retryable\":false,\"details\":{}}}\n"
	if w.Code != 404 || w.Body.String() != want {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestIncidentDetailUsesTimelineIndexOrder(t *testing.T) {
	store := core.NewStore()
	ctx := context.Background()
	for _, raw := range []string{
		strings.Replace(validEventJSON, `"event_id":"e1"`, `"event_id":"e1"`, 1),
		strings.Replace(validEventJSON, `"event_id":"e1"`, `"event_id":"e2"`, 1),
	} {
		var event contracts.ExecutionEvent
		if err := contracts.DecodeStrict(strings.NewReader(raw), &event); err != nil {
			t.Fatal(err)
		}
		if err := store.IngestEvent(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	incident := contracts.Incident{SchemaVersion: "1.0", IncidentID: "inc", Status: contracts.IncidentReady, FailureOracle: contracts.FailureOracleRef{ID: "oracle", Version: "1.0.0"}, SystemPack: contracts.SystemPackRef{ID: "pack", Version: "1.0.0", InterfaceVersion: "1.0"}, TraceID: "trace", ExecutionID: "exec", DetectedAt: "2026-08-29T10:32:02Z", Summary: "summary", EvidenceEventIDs: []string{"e1"}, GraphID: "graph", SanitizationStatus: contracts.SanitizationPass}
	graph := contracts.ExecutionGraph{SchemaVersion: "1.0", GraphID: "graph", IncidentID: "inc", OrderingPolicyVersion: "1.0", Nodes: []contracts.GraphNode{{EventID: "e2", TimelineIndex: 0}, {EventID: "e1", TimelineIndex: 1}}, Edges: []contracts.GraphEdge{}}
	if err := store.PutIncident(ctx, incident, graph); err != nil {
		t.Fatal(err)
	}

	w := request(t, handler(store), http.MethodGet, "/v1/incidents/inc", "")
	if w.Code != http.StatusOK || strings.Index(w.Body.String(), `"event_id":"e2"`) > strings.Index(w.Body.String(), `"event_id":"e1"`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestInternalReadDoesNotLeak(t *testing.T) {
	w := request(t, handler(&fakeRepository{err: errors.New("password=secret database exploded")}), http.MethodGet, "/v1/incidents", "")
	want := "{\"error\":{\"code\":\"INTERNAL_FAILURE\",\"message\":\"internal failure\",\"retryable\":false,\"details\":{}}}\n"
	if w.Code != 500 || w.Body.String() != want || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestLifecycleAndSafetyMapperUseFrozenCodes(t *testing.T) {
	for _, tc := range []struct {
		err    error
		code   contracts.ErrorCode
		status int
	}{
		{core.ErrInvalidLifecycle, contracts.SchemaInvalid, 409},
		{errors.New("blocked"), contracts.DestinationBlocked, 422},
	} {
		w := httptest.NewRecorder()
		writeMappedError(w, tc.err, tc.code)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), `"code":"`+string(tc.code)+`"`) || !strings.Contains(w.Body.String(), `"details":{}`) {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
}

func TestPackRegistrationResolvesBothImplementations(t *testing.T) {
	for _, token := range []string{packregistry.DevImplementation, checkout.PackID} {
		p := packregistry.Resolve(token)
		if p == nil {
			t.Fatalf("Resolve(%q) = nil", token)
		}
		if p.Descriptor().ID == "" {
			t.Fatalf("Resolve(%q).Descriptor().ID is empty", token)
		}
	}
}
