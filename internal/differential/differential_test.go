package differential

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/causalens/causalens/internal/contracts"
)

type fakeStore struct {
	runs        map[string]contracts.ReplayRun
	events      map[string][]contracts.ExecutionEvent
	graphs      map[string]contracts.ExecutionGraph
	eventsError error
	graphError  error
}

func (s *fakeStore) GetRun(_ context.Context, runID string) (contracts.ReplayRun, error) {
	run, ok := s.runs[runID]
	if !ok {
		return contracts.ReplayRun{}, errors.New("run not found")
	}
	return run, nil
}

func (s *fakeStore) EventsForRun(_ context.Context, run contracts.ReplayRun) ([]contracts.ExecutionEvent, error) {
	if s.eventsError != nil {
		return nil, s.eventsError
	}
	return s.events[run.RunID], nil
}

func (s *fakeStore) GraphsForRun(_ context.Context, run contracts.ReplayRun) (contracts.ExecutionGraph, error) {
	if s.graphError != nil {
		return contracts.ExecutionGraph{}, s.graphError
	}
	return s.graphs[run.RunID], nil
}

type fakeAlignment struct {
	matched []contracts.EventAlignment
	added   []string
	removed []string
	changed []contracts.EventChange
	err     error
}

func (f fakeAlignment) Align(_ context.Context, diffID string, baseline, comparison contracts.ReplayExecution) ([]contracts.EventAlignment, []string, []string, []contracts.EventChange, error) {
	return f.matched, f.added, f.removed, f.changed, f.err
}

var goldenCapsuleHash = strings.Repeat("a", 64)

func goldenBaselineRun() contracts.ReplayRun {
	return contracts.ReplayRun{
		SchemaVersion: contracts.ContractVersion,
		RunID:         "run-baseline",
		ExecutionID:   "exec-base",
		CapsuleID:     "cap-1",
		CapsuleHash:   goldenCapsuleHash,
		RunType:       contracts.RunTypeBaseline,
		TrialNumber:   1,
		Status:        contracts.ReplayRunCompleted,
		Outcome:       contracts.ReplayOutcomeReproduced,
		StartedAt:     "2026-08-29T10:34:00Z",
		CompletedAt:   "2026-08-29T10:34:01Z",
		ObservedEventIDs: []string{
			"evt-base-pay-start", "evt-base-timeout", "evt-base-retry",
			"evt-base-pay2-start", "evt-base-ledger-1", "evt-base-ledger-2",
		},
		EffectSummary: &contracts.EffectSummary{PaymentAttemptCount: 2, LedgerCommitCount: 2},
		FailureOracleResult: &contracts.OracleResult{
			Oracle:                   contracts.FailureOracleRef{ID: "duplicate_ledger_effect", Version: "1.0.0"},
			Matched:                  true,
			EffectSummary:            contracts.EffectSummary{PaymentAttemptCount: 2, LedgerCommitCount: 2},
			RequiredEvidenceEventIDs: []string{"evt-base-timeout", "evt-base-retry", "evt-base-ledger-1", "evt-base-ledger-2"},
			Explanation:              "Baseline reproduced the duplicate ledger effect.",
		},
		IsolationEvidence: &contracts.IsolationEvidence{
			PolicyVersion:         contracts.ContractVersion,
			Verdict:               contracts.VerdictPass,
			RuntimeNamespace:      "replay-run-base",
			NetworkPolicy:         contracts.VerdictPass,
			CredentialProfile:     contracts.CredentialReplayOnly,
			DatastoreDestinations: []string{"postgres://replay/ledger_base"},
			SimulatorInteractions: []contracts.DependencyInteraction{},
			DeniedInteractions:    []contracts.DependencyInteraction{},
			TeardownResult:        contracts.VerdictPass,
		},
	}
}

func goldenComparisonRun() contracts.ReplayRun {
	return contracts.ReplayRun{
		SchemaVersion: contracts.ContractVersion,
		RunID:         "run-comparison",
		ExecutionID:   "exec-whatif",
		CapsuleID:     "cap-1",
		CapsuleHash:   goldenCapsuleHash,
		RunType:       contracts.RunTypeWhatIf,
		BaselineRunID: "run-baseline",
		Intervention:  &contracts.Intervention{Type: contracts.InterventionPaymentLatency, From: 350, To: 50, Unit: contracts.InterventionUnitMilliseconds},
		TrialNumber:   1,
		Status:        contracts.ReplayRunCompleted,
		Outcome:       contracts.ReplayOutcomeMitigated,
		StartedAt:     "2026-08-29T10:35:00Z",
		CompletedAt:   "2026-08-29T10:35:00.067Z",
		ObservedEventIDs: []string{
			"evt-whatif-pay-start", "evt-whatif-pay-complete", "evt-whatif-ledger-1",
		},
		EffectSummary: &contracts.EffectSummary{PaymentAttemptCount: 1, LedgerCommitCount: 1},
		FailureOracleResult: &contracts.OracleResult{
			Oracle:                   contracts.FailureOracleRef{ID: "duplicate_ledger_effect", Version: "1.0.0"},
			Matched:                  false,
			EffectSummary:            contracts.EffectSummary{PaymentAttemptCount: 1, LedgerCommitCount: 1},
			RequiredEvidenceEventIDs: []string{"evt-whatif-pay-complete", "evt-whatif-ledger-1"},
			Explanation:              "Payment completed before timeout; no duplicate effect occurred.",
		},
		IsolationEvidence: &contracts.IsolationEvidence{
			PolicyVersion:         contracts.ContractVersion,
			Verdict:               contracts.VerdictPass,
			RuntimeNamespace:      "replay-run-whatif",
			NetworkPolicy:         contracts.VerdictPass,
			CredentialProfile:     contracts.CredentialReplayOnly,
			DatastoreDestinations: []string{"postgres://replay/ledger_whatif"},
			SimulatorInteractions: []contracts.DependencyInteraction{},
			DeniedInteractions:    []contracts.DependencyInteraction{},
			TeardownResult:        contracts.VerdictPass,
		},
	}
}

func goldenBaselineEvents() []contracts.ExecutionEvent {
	return []contracts.ExecutionEvent{
		evt("evt-base-pay-start", 0, "payment", contracts.OperationDependency, contracts.EventStart, contracts.EventRunning, 1),
		evt("evt-base-timeout", 1, "checkout", contracts.OperationInternal, contracts.EventTimeout, contracts.EventTimedOut, 1),
		evt("evt-base-retry", 2, "checkout", contracts.OperationControl, contracts.EventRetry, contracts.EventRunning, 2),
		evt("evt-base-pay2-start", 3, "payment", contracts.OperationDependency, contracts.EventStart, contracts.EventRunning, 2),
		evt("evt-base-ledger-1", 4, "ledger", contracts.OperationStateChange, contracts.EventEffect, contracts.EventSuccess, 1),
		evt("evt-base-ledger-2", 5, "ledger", contracts.OperationStateChange, contracts.EventEffect, contracts.EventSuccess, 1),
	}
}

func goldenComparisonEvents() []contracts.ExecutionEvent {
	return []contracts.ExecutionEvent{
		evt("evt-whatif-pay-start", 0, "payment", contracts.OperationDependency, contracts.EventStart, contracts.EventRunning, 1),
		evt("evt-whatif-pay-complete", 1, "payment", contracts.OperationDependency, contracts.EventComplete, contracts.EventSuccess, 1),
		evt("evt-whatif-ledger-1", 2, "ledger", contracts.OperationStateChange, contracts.EventEffect, contracts.EventSuccess, 1),
	}
}

func evt(id string, seq int, component string, kind contracts.OperationKind, etype contracts.EventType, status contracts.EventStatus, attempt int) contracts.ExecutionEvent {
	return contracts.ExecutionEvent{
		SchemaVersion:      contracts.ContractVersion,
		EventID:            id,
		ExecutionID:        "exec-" + component,
		TraceID:            "trace-1",
		Component:          contracts.ComponentRef{Name: component, Instance: component + "-instance"},
		Operation:          contracts.OperationRef{Name: component + "-op", Kind: kind},
		EventType:          etype,
		Attempt:            attempt,
		LogicalOperationID: "checkout-1",
		OccurredAt:         "2026-08-29T10:32:01Z",
		Sequence:           seq,
		Status:             status,
		Attributes:         map[string]any{},
	}
}

func goldenGraph(eventIDs []string) contracts.ExecutionGraph {
	nodes := make([]contracts.GraphNode, len(eventIDs))
	for i, id := range eventIDs {
		nodes[i] = contracts.GraphNode{EventID: id, TimelineIndex: i}
	}
	return contracts.ExecutionGraph{
		SchemaVersion:         contracts.ContractVersion,
		GraphID:               "graph-" + strings.Join(eventIDs, "-"),
		IncidentID:            "inc-1",
		OrderingPolicyVersion: contracts.ContractVersion,
		Nodes:                 nodes,
		Edges:                 []contracts.GraphEdge{},
	}
}

func baselineStore() *fakeStore {
	return &fakeStore{
		runs: map[string]contracts.ReplayRun{
			"run-baseline":   goldenBaselineRun(),
			"run-comparison": goldenComparisonRun(),
		},
		events: map[string][]contracts.ExecutionEvent{
			"run-baseline":   goldenBaselineEvents(),
			"run-comparison": goldenComparisonEvents(),
		},
		graphs: map[string]contracts.ExecutionGraph{
			"run-baseline":   goldenGraph([]string{"evt-base-pay-start", "evt-base-timeout", "evt-base-retry", "evt-base-pay2-start", "evt-base-ledger-1", "evt-base-ledger-2"}),
			"run-comparison": goldenGraph([]string{"evt-whatif-pay-start", "evt-whatif-pay-complete", "evt-whatif-ledger-1"}),
		},
	}
}

func goldenAlignment() fakeAlignment {
	return fakeAlignment{
		matched: []contracts.EventAlignment{
			{BaselineEventID: "evt-base-pay-start", ComparisonEventID: "evt-whatif-pay-start"},
			{BaselineEventID: "evt-base-ledger-1", ComparisonEventID: "evt-whatif-ledger-1"},
		},
		added:   []string{},
		removed: []string{"evt-base-timeout", "evt-base-retry", "evt-base-ledger-2"},
		changed: []contracts.EventChange{
			{BaselineEventID: "evt-base-timeout", ComparisonEventID: "evt-whatif-pay-complete", Field: "event_type", BaselineValue: "TIMEOUT", ComparisonValue: "COMPLETE"},
		},
	}
}

func TestBuildRejectsPrerequisites(t *testing.T) {
	tests := []struct {
		name string
		edit func(store *fakeStore)
	}{
		{"baseline not completed", func(s *fakeStore) {
			r := s.runs["run-baseline"]
			r.Status = contracts.ReplayRunFailed
			s.runs["run-baseline"] = r
		}},
		{"baseline not reproduced", func(s *fakeStore) {
			r := s.runs["run-baseline"]
			r.Outcome = contracts.ReplayOutcomeNotReproduced
			s.runs["run-baseline"] = r
		}},
		{"comparison not what-if", func(s *fakeStore) {
			r := s.runs["run-comparison"]
			r.RunType = contracts.RunTypeBaseline
			s.runs["run-comparison"] = r
		}},
		{"comparison baseline_run_id mismatch", func(s *fakeStore) {
			r := s.runs["run-comparison"]
			r.BaselineRunID = "run-some-other"
			s.runs["run-comparison"] = r
		}},
		{"mismatched capsule hash", func(s *fakeStore) {
			r := s.runs["run-comparison"]
			r.CapsuleHash = strings.Repeat("b", 64)
			s.runs["run-comparison"] = r
		}},
		{"comparison zero intervention", func(s *fakeStore) {
			r := s.runs["run-comparison"]
			r.Intervention = nil
			s.runs["run-comparison"] = r
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := baselineStore()
			tt.edit(store)
			_, err := Build(context.Background(), store, "diff-1", "run-baseline", "run-comparison", goldenAlignment())
			if !errors.Is(err, ErrDiffPrerequisites) {
				t.Fatalf("expected ErrDiffPrerequisites, got %v", err)
			}
		})
	}
}

func TestBuildGoldenProducesValidDiff(t *testing.T) {
	diff, err := Build(context.Background(), baselineStore(), "diff-1", "run-baseline", "run-comparison", goldenAlignment())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := diff.Validate(); err != nil {
		t.Fatalf("produced diff fails validation: %v", err)
	}
	if diff.DiffID != "diff-1" || diff.BaselineRunID != "run-baseline" || diff.ComparisonRunID != "run-comparison" {
		t.Fatalf("wrong diff ids: %+v", diff)
	}
	if diff.Intervention != (contracts.Intervention{Type: contracts.InterventionPaymentLatency, From: 350, To: 50, Unit: contracts.InterventionUnitMilliseconds}) {
		t.Fatalf("wrong intervention: %+v", diff.Intervention)
	}
	if got := diff.EffectDelta; got != (contracts.EffectDelta{PaymentAttemptCount: -1, LedgerCommitCount: -1}) {
		t.Fatalf("wrong effect delta: %+v", got)
	}
	if diff.FirstMeaningfulDivergence == nil {
		t.Fatal("expected first meaningful divergence present")
	}
	if got := diff.FirstMeaningfulDivergence; got.BaselineEventID != "evt-base-timeout" || got.ComparisonEventID != "evt-whatif-pay-complete" || got.BaselineTimelineIndex != 1 || got.ComparisonTimelineIndex != 1 {
		t.Fatalf("wrong first divergence: %+v", got)
	}
}

func TestBuildEffectDeltaIsComparisonMinusBaseline(t *testing.T) {
	diff, err := Build(context.Background(), baselineStore(), "diff-1", "run-baseline", "run-comparison", goldenAlignment())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	want := contracts.EffectDelta{
		PaymentAttemptCount: diff.ComparisonEffectSummary.PaymentAttemptCount - diff.BaselineEffectSummary.PaymentAttemptCount,
		LedgerCommitCount:   diff.ComparisonEffectSummary.LedgerCommitCount - diff.BaselineEffectSummary.LedgerCommitCount,
	}
	if diff.EffectDelta != want {
		t.Fatalf("effect delta mismatch: got %+v want %+v", diff.EffectDelta, want)
	}
}

func TestBuildAlignmentSetsComeFromSeam(t *testing.T) {
	align := goldenAlignment()
	diff, err := Build(context.Background(), baselineStore(), "diff-1", "run-baseline", "run-comparison", align)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !reflect.DeepEqual(diff.MatchedEvents, align.matched) {
		t.Fatalf("matched mismatch: got %+v", diff.MatchedEvents)
	}
	if !reflect.DeepEqual(diff.AddedEventIDs, align.added) {
		t.Fatalf("added mismatch: got %+v", diff.AddedEventIDs)
	}
	if !reflect.DeepEqual(diff.RemovedEventIDs, align.removed) {
		t.Fatalf("removed mismatch: got %+v", diff.RemovedEventIDs)
	}
	if !reflect.DeepEqual(diff.ChangedEvents, align.changed) {
		t.Fatalf("changed mismatch: got %+v", diff.ChangedEvents)
	}
}

func TestBuildNilDivergenceWhenInconclusive(t *testing.T) {
	store := baselineStore()
	r := store.runs["run-comparison"]
	r.Outcome = contracts.ReplayOutcomeInconclusive
	store.runs["run-comparison"] = r
	diff, err := Build(context.Background(), store, "diff-1", "run-baseline", "run-comparison", goldenAlignment())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if diff.FirstMeaningfulDivergence != nil {
		t.Fatalf("expected nil divergence for INCONCLUSIVE, got %+v", diff.FirstMeaningfulDivergence)
	}
}

func TestBuildNilDivergenceWhenStructurallyIdentical(t *testing.T) {
	store := &fakeStore{
		runs: map[string]contracts.ReplayRun{
			"run-baseline":   goldenBaselineRun(),
			"run-comparison": goldenComparisonRun(),
		},
		events: map[string][]contracts.ExecutionEvent{
			"run-baseline":   goldenBaselineEvents(),
			"run-comparison": goldenBaselineEvents(),
		},
		graphs: map[string]contracts.ExecutionGraph{
			"run-baseline":   goldenGraph([]string{"evt-base-pay-start", "evt-base-timeout", "evt-base-retry", "evt-base-pay2-start", "evt-base-ledger-1", "evt-base-ledger-2"}),
			"run-comparison": goldenGraph([]string{"evt-base-pay-start", "evt-base-timeout", "evt-base-retry", "evt-base-pay2-start", "evt-base-ledger-1", "evt-base-ledger-2"}),
		},
	}
	r := store.runs["run-comparison"]
	r.RunType = contracts.RunTypeWhatIf
	r.Outcome = contracts.ReplayOutcomeUnchanged
	r.BaselineRunID = "run-baseline"
	r.EffectSummary = &contracts.EffectSummary{PaymentAttemptCount: 2, LedgerCommitCount: 2}
	r.FailureOracleResult = &contracts.OracleResult{
		Oracle:                   contracts.FailureOracleRef{ID: "duplicate_ledger_effect", Version: "1.0.0"},
		Matched:                  true,
		EffectSummary:            contracts.EffectSummary{PaymentAttemptCount: 2, LedgerCommitCount: 2},
		RequiredEvidenceEventIDs: []string{"evt-base-timeout"},
		Explanation:              "Unchanged.",
	}
	store.runs["run-comparison"] = r
	align := fakeAlignment{
		matched: []contracts.EventAlignment{
			{BaselineEventID: "evt-base-timeout", ComparisonEventID: "evt-base-timeout"},
			{BaselineEventID: "evt-base-retry", ComparisonEventID: "evt-base-retry"},
		},
		added:   []string{},
		removed: []string{},
		changed: []contracts.EventChange{},
	}
	diff, err := Build(context.Background(), store, "diff-1", "run-baseline", "run-comparison", align)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if diff.FirstMeaningfulDivergence != nil {
		t.Fatalf("expected nil divergence for structurally identical runs, got %+v", diff.FirstMeaningfulDivergence)
	}
}

func TestBuildDeterministicOrderingUnderShuffle(t *testing.T) {
	baseEvents := goldenBaselineEvents()
	compEvents := goldenComparisonEvents()

	shuffled1 := shuffleCopy(baseEvents)
	shuffled2 := shuffleCopy(compEvents)
	shuffled3 := shuffleCopy(baseEvents)
	shuffled4 := shuffleCopy(compEvents)

	storeA := &fakeStore{
		runs: map[string]contracts.ReplayRun{
			"run-baseline":   goldenBaselineRun(),
			"run-comparison": goldenComparisonRun(),
		},
		events: map[string][]contracts.ExecutionEvent{
			"run-baseline":   shuffled1,
			"run-comparison": shuffled2,
		},
		graphs: map[string]contracts.ExecutionGraph{
			"run-baseline":   goldenGraph([]string{"evt-base-pay-start", "evt-base-timeout", "evt-base-retry", "evt-base-pay2-start", "evt-base-ledger-1", "evt-base-ledger-2"}),
			"run-comparison": goldenGraph([]string{"evt-whatif-pay-start", "evt-whatif-pay-complete", "evt-whatif-ledger-1"}),
		},
	}
	storeB := &fakeStore{
		runs: map[string]contracts.ReplayRun{
			"run-baseline":   goldenBaselineRun(),
			"run-comparison": goldenComparisonRun(),
		},
		events: map[string][]contracts.ExecutionEvent{
			"run-baseline":   shuffled3,
			"run-comparison": shuffled4,
		},
		graphs: map[string]contracts.ExecutionGraph{
			"run-baseline":   goldenGraph([]string{"evt-base-pay-start", "evt-base-timeout", "evt-base-retry", "evt-base-pay2-start", "evt-base-ledger-1", "evt-base-ledger-2"}),
			"run-comparison": goldenGraph([]string{"evt-whatif-pay-start", "evt-whatif-pay-complete", "evt-whatif-ledger-1"}),
		},
	}

	diffA, err := Build(context.Background(), storeA, "diff-1", "run-baseline", "run-comparison", goldenAlignment())
	if err != nil {
		t.Fatalf("Build A: %v", err)
	}
	diffB, err := Build(context.Background(), storeB, "diff-1", "run-baseline", "run-comparison", goldenAlignment())
	if err != nil {
		t.Fatalf("Build B: %v", err)
	}
	if !reflect.DeepEqual(diffA, diffB) {
		t.Fatal("shuffled input produced different diffs")
	}
}

func TestLimitationsReturnsNonEmptySlice(t *testing.T) {
	lim := Limitations("checkout_duplicate_effect v1.0.0")
	if len(lim) == 0 || strings.TrimSpace(lim[0]) == "" {
		t.Fatalf("Limitations returned invalid slice: %+v", lim)
	}
	if lim := Limitations(""); len(lim) == 0 || strings.TrimSpace(lim[0]) == "" {
		t.Fatalf("Limitations(empty) returned invalid slice: %+v", lim)
	}
}

func TestFirstDivergenceFrom(t *testing.T) {
	base := goldenBaselineEvents()
	comp := goldenComparisonEvents()
	changes := []contracts.EventChange{
		{BaselineEventID: "evt-base-timeout", ComparisonEventID: "evt-whatif-pay-complete", Field: "event_type", BaselineValue: "TIMEOUT", ComparisonValue: "COMPLETE"},
	}
	got := FirstDivergenceFrom(base, comp, changes, []string{}, []string{"evt-base-timeout"})
	if got == nil {
		t.Fatal("expected divergence")
	}
	if got.BaselineTimelineIndex != 1 || got.ComparisonTimelineIndex != 1 {
		t.Fatalf("wrong indices: %+v", got)
	}
	if got.Rule != "event_type" {
		t.Fatalf("wrong rule: %+v", got.Rule)
	}
	if got.BaselineValue != "TIMEOUT" || got.ComparisonValue != "COMPLETE" {
		t.Fatalf("wrong values: %+v", got)
	}
}

func shuffleCopy(events []contracts.ExecutionEvent) []contracts.ExecutionEvent {
	out := make([]contracts.ExecutionEvent, len(events))
	copy(out, events)
	for i := 1; i < len(out); i++ {
		out[i], out[i-1] = out[i-1], out[i]
	}
	return out
}
