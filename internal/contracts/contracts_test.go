package contracts

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestDecodeStrictRejectsUnknownAndTrailingJSON(t *testing.T) {
	for _, input := range []string{
		`{"schema_version":"1.0","event_id":"e","unknown":true}`,
		`{"schema_version":"1.0"}{"schema_version":"1.0"}`,
	} {
		var event ExecutionEvent
		if err := DecodeStrict(strings.NewReader(input), &event); err == nil {
			t.Fatalf("DecodeStrict(%q) unexpectedly succeeded", input)
		}
	}
}

func TestExecutionEventValidation(t *testing.T) {
	valid := validEvent()
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid event: %v", err)
	}

	tests := []struct {
		name string
		edit func(*ExecutionEvent)
	}{
		{"operation enum", func(e *ExecutionEvent) { e.Operation.Kind = "OTHER" }},
		{"event enum", func(e *ExecutionEvent) { e.EventType = "OTHER" }},
		{"status enum", func(e *ExecutionEvent) { e.Status = "OTHER" }},
		{"attempt", func(e *ExecutionEvent) { e.Attempt = 0 }},
		{"sequence", func(e *ExecutionEvent) { e.Sequence = -1 }},
		{"duration", func(e *ExecutionEvent) { n := -1; e.DurationMS = &n }},
		{"timestamp offset", func(e *ExecutionEvent) { e.OccurredAt = "2026-08-29T12:32:01+02:00" }},
		{"timestamp invalid", func(e *ExecutionEvent) { e.OccurredAt = "2026-02-30T10:00:00Z" }},
		{"unknown attribute", func(e *ExecutionEvent) { e.Attributes["customer_id"] = "secret" }},
		{"fractional integer", func(e *ExecutionEvent) { e.Attributes["configured_latency_ms"] = 1.5 }},
		{"wrong boolean", func(e *ExecutionEvent) { e.Attributes["effect_committed"] = "true" }},
		{"null attributes", func(e *ExecutionEvent) { e.Attributes = nil }},
		{"empty required", func(e *ExecutionEvent) { e.EventID = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := validEvent()
			tt.edit(&e)
			if err := e.Validate(); err == nil {
				t.Fatal("expected validation failure")
			}
		})
	}
}

func TestExecutionEventJSONNumbersValidateAsIntegers(t *testing.T) {
	data := `{"schema_version":"1.0","event_id":"e","execution_id":"x","trace_id":"t","component":{"name":"c","instance":"i"},"operation":{"name":"o","kind":"INTERNAL"},"event_type":"START","attempt":1,"logical_operation_id":"l","occurred_at":"2026-08-29T10:32:01Z","sequence":0,"status":"RUNNING","attributes":{"checkout_timeout_ms":200}}`
	var event ExecutionEvent
	if err := DecodeStrict(strings.NewReader(data), &event); err != nil || event.Validate() != nil {
		t.Fatalf("valid decoded event: decode=%v validate=%v", err, event.Validate())
	}
}

func TestFrozenJSONShape(t *testing.T) {
	run := ReplayRun{FailureOracleResult: &OracleResult{}}
	b, err := json.Marshal(run)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte(`"failure_oracle_result"`)) || bytes.Contains(b, []byte(`"oracle_result"`)) {
		t.Fatalf("wrong ReplayRun JSON shape: %s", b)
	}
	reset, _ := json.Marshal(ResetResult{})
	if bytes.Contains(reset, []byte(`"ready"`)) {
		t.Fatalf("ResetResult contains non-contract field: %s", reset)
	}
}

func TestExecutionGraphValidation(t *testing.T) {
	valid := ExecutionGraph{SchemaVersion: ContractVersion, GraphID: "g", IncidentID: "i", OrderingPolicyVersion: ContractVersion, Nodes: []GraphNode{{EventID: "a", TimelineIndex: 0}, {EventID: "b", TimelineIndex: 1}}, Edges: []GraphEdge{{EdgeID: "e", FromEventID: "a", ToEventID: "b", Type: GraphEdgeRetry}}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	cyclic := valid
	cyclic.Edges = append(cyclic.Edges, GraphEdge{EdgeID: "e2", FromEventID: "b", ToEventID: "a", Type: GraphEdgeDependency})
	if err := cyclic.Validate(); err == nil {
		t.Fatal("expected hard-edge cycle rejection")
	}
	dangling := valid
	dangling.Edges = []GraphEdge{{EdgeID: "e", FromEventID: "a", ToEventID: "missing", Type: GraphEdgeTemporal}}
	if err := dangling.Validate(); err == nil {
		t.Fatal("expected dangling edge rejection")
	}
}

func TestIncidentValidation(t *testing.T) {
	base := Incident{SchemaVersion: ContractVersion, IncidentID: "i", Status: IncidentReady, FailureOracle: FailureOracleRef{ID: "o", Version: "1.0.0"}, SystemPack: SystemPackRef{ID: "p", Version: "1.0.0", InterfaceVersion: ContractVersion}, TraceID: "t", ExecutionID: "e", DetectedAt: "2026-08-29T10:00:00Z", Summary: "s", EvidenceEventIDs: []string{"event"}, GraphID: "g", SanitizationStatus: SanitizationPass}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	blocked := base
	blocked.Status = IncidentBlocked
	blocked.BlockReason = nil
	if err := blocked.Validate(); err == nil {
		t.Fatal("BLOCKED requires block_reason")
	}
}

func TestReplayRunValidationAndTransitions(t *testing.T) {
	r := validCompletedRun()
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if !CanTransitionReplayRun(ReplayRunCreated, ReplayRunValidating) || CanTransitionReplayRun(ReplayRunCreated, ReplayRunRunning) || CanTransitionReplayRun(ReplayRunCompleted, ReplayRunRunning) {
		t.Fatal("lifecycle transition table is incorrect")
	}
	bad := r
	bad.CompletedAt = ""
	if err := bad.Validate(); err == nil {
		t.Fatal("terminal run requires completed_at")
	}
	bad = r
	bad.Error = &RunError{Code: InternalFailure, Message: "x", Details: map[string]any{}}
	if err := bad.Validate(); err == nil {
		t.Fatal("COMPLETED forbids error")
	}
	bad = r
	bad.Outcome = ReplayOutcomeMitigated
	if err := bad.Validate(); err == nil {
		t.Fatal("baseline forbids what-if outcome")
	}
	bad = r
	bad.FailureOracleResult.Matched = false
	if err := bad.Validate(); err == nil {
		t.Fatal("baseline REPRODUCED accepted with unmatched oracle")
	}
	bad = r
	bad.Outcome = ReplayOutcomeNotReproduced
	bad.FailureOracleResult.Matched = true
	if err := bad.Validate(); err == nil {
		t.Fatal("baseline NOT_REPRODUCED accepted with matched oracle")
	}
	whatIf := r
	whatIf.RunType, whatIf.BaselineRunID = RunTypeWhatIf, "run-base"
	whatIf.Intervention = &Intervention{Type: InterventionPaymentLatency, From: 350, To: 50, Unit: InterventionUnitMilliseconds}
	whatIf.Outcome = ReplayOutcomeMitigated
	whatIf.FailureOracleResult.Matched = true
	if err := whatIf.Validate(); err == nil {
		t.Fatal("what-if MITIGATED accepted with matched oracle")
	}
	whatIf = r
	whatIf.RunType, whatIf.BaselineRunID = RunTypeWhatIf, "run-base"
	whatIf.Intervention = &Intervention{Type: InterventionPaymentLatency, From: 350, To: 50, Unit: InterventionUnitMilliseconds}
	whatIf.Outcome = ReplayOutcomeUnchanged
	whatIf.FailureOracleResult.Matched = false
	if err := whatIf.Validate(); err == nil {
		t.Fatal("what-if UNCHANGED accepted with unmatched oracle")
	}
}

func TestReplayRunStartedAtAndIsolationBlockedRules(t *testing.T) {
	created := validCompletedRun()
	created.Status, created.Outcome, created.StartedAt, created.CompletedAt = ReplayRunCreated, "", "", ""
	created.EffectSummary, created.FailureOracleResult, created.IsolationEvidence = nil, nil, nil
	if err := created.Validate(); err != nil {
		t.Fatalf("valid CREATED: %v", err)
	}
	for _, status := range []ReplayRunStatus{ReplayRunCreated, ReplayRunValidating} {
		run := created
		run.Status, run.StartedAt = status, "2026-08-29T10:00:00Z"
		if err := run.Validate(); err == nil {
			t.Fatalf("%s accepted started_at", status)
		}
	}
	running := created
	running.Status = ReplayRunRunning
	if err := running.Validate(); err == nil {
		t.Fatal("RUNNING accepted without started_at")
	}
	running.StartedAt = "2026-08-29T10:00:00Z"
	if err := running.Validate(); err != nil {
		t.Fatalf("valid RUNNING: %v", err)
	}
	failed := created
	failed.Status, failed.CompletedAt = ReplayRunFailed, "2026-08-29T10:00:01Z"
	failed.Error = &RunError{Code: InternalFailure, Message: "runtime failed", Details: map[string]any{}}
	if err := failed.Validate(); err == nil {
		t.Fatal("FAILED accepted without started_at")
	}
	blocked := created
	blocked.Status, blocked.CompletedAt = ReplayRunBlocked, "2026-08-29T10:00:01Z"
	blocked.Error = &RunError{Code: SchemaInvalid, Message: "pre-runtime", Details: map[string]any{}}
	if err := blocked.Validate(); err != nil {
		t.Fatalf("valid pre-runtime BLOCKED: %v", err)
	}
	blocked.Error.Code = IsolationViolation
	if err := blocked.Validate(); err == nil {
		t.Fatal("isolation BLOCKED accepted without evidence")
	}
	blocked.IsolationEvidence = func() *IsolationEvidence {
		v := validIsolationEvidence()
		v.Verdict, v.NetworkPolicy = VerdictFail, VerdictFail
		return &v
	}()
	if err := blocked.Validate(); err != nil {
		t.Fatalf("valid isolation BLOCKED: %v", err)
	}
}

func TestRunErrorAndValidationIssueValidation(t *testing.T) {
	if err := (RunError{Code: IntegrityMismatch, Message: "bad", Details: map[string]any{}}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (RunError{Code: ErrorCode("OTHER"), Message: "bad", Details: map[string]any{}}).Validate(); err == nil {
		t.Fatal("unknown error code accepted")
	}
	if err := (ValidationIssue{Code: SchemaInvalid, Path: "/attempt", Message: "bad"}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestReplayCapsuleValidation(t *testing.T) {
	valid := validCapsule()
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid capsule: %v", err)
	}
	tests := []struct {
		name string
		edit func(*ReplayCapsule)
	}{
		{"created timestamp", func(c *ReplayCapsule) { c.CreatedAt = "2026-08-29T12:33:00+02:00" }},
		{"source field", func(c *ReplayCapsule) { c.Source.TraceID = "" }},
		{"source literal", func(c *ReplayCapsule) { c.Source.CaptureEnvironment = "PRODUCTION" }},
		{"trigger object", func(c *ReplayCapsule) { c.Trigger.RequestOrMessage = nil }},
		{"event ids nonempty", func(c *ReplayCapsule) { c.EventIDs = nil }},
		{"state fixture digest", func(c *ReplayCapsule) { c.StateFixtures[0].ContentDigest = "ABC" }},
		{"state fixture literal", func(c *ReplayCapsule) { c.StateFixtures[0].Kind = "FILE" }},
		{"dependency fixture object", func(c *ReplayCapsule) { c.DependencyFixtures[0].Response = nil }},
		{"dependency fixture bound", func(c *ReplayCapsule) { c.DependencyFixtures[0].InvocationLimit = 0 }},
		{"timing literal", func(c *ReplayCapsule) { c.TimingPolicy.TimeoutMS = 201 }},
		{"plan components", func(c *ReplayCapsule) { c.ReplayPlan.RequiredComponents = []string{"gateway"} }},
		{"fixture load order reference", func(c *ReplayCapsule) { c.ReplayPlan.FixtureLoadOrder[0] = "missing" }},
		{"oracle literal", func(c *ReplayCapsule) { c.FailureOracle.ID = "other" }},
		{"intervention bounds", func(c *ReplayCapsule) { c.AllowedInterventions[0].Maximum = 4999 }},
		{"blocked destinations nonempty", func(c *ReplayCapsule) { c.Safety.BlockedDestinations = nil }},
		{"unsafe allowed destination", func(c *ReplayCapsule) { c.Safety.AllowedDestinations[0] = "production-database" }},
		{"integrity algorithm", func(c *ReplayCapsule) { c.Integrity.Algorithm = "MD5" }},
		{"integrity format", func(c *ReplayCapsule) { c.Integrity.Digest = strings.Repeat("A", 64) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validCapsule()
			tt.edit(&c)
			if err := c.Validate(); err == nil {
				t.Fatal("expected validation failure")
			}
		})
	}
}

func TestIsolationEvidenceValidation(t *testing.T) {
	valid := validIsolationEvidence()
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, evidence := range []IsolationEvidence{
		func() IsolationEvidence { v := validIsolationEvidence(); v.PolicyVersion = "2.0"; return v }(),
		func() IsolationEvidence { v := validIsolationEvidence(); v.DatastoreDestinations = nil; return v }(),
		func() IsolationEvidence {
			v := validIsolationEvidence()
			v.SimulatorInteractions[0].Result = "OTHER"
			return v
		}(),
		func() IsolationEvidence {
			v := validIsolationEvidence()
			v.DeniedInteractions = append(v.DeniedInteractions, DependencyInteraction{Dependency: "p", Destination: "d", Operation: "o", Result: InteractionDenied})
			return v
		}(),
		func() IsolationEvidence { v := validIsolationEvidence(); v.NetworkPolicy = VerdictFail; return v }(),
		func() IsolationEvidence {
			v := validIsolationEvidence()
			v.SimulatorInteractions[0].Destination = "http://prod-payment-api.example.com:8080"
			return v
		}(),
	} {
		if err := evidence.Validate(); err == nil {
			t.Fatal("invalid isolation evidence accepted")
		}
	}
	failing := validIsolationEvidence()
	failing.Verdict = VerdictFail
	failing.NetworkPolicy = VerdictFail
	if err := failing.Validate(); err != nil {
		t.Fatalf("consistent FAIL evidence: %v", err)
	}
}

func TestReplayDiffValidation(t *testing.T) {
	valid := validDiff()
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid diff: %v", err)
	}
	tests := []struct {
		name string
		edit func(*ReplayDiff)
	}{
		{"required id", func(d *ReplayDiff) { d.DiffID = "" }},
		{"alignment version", func(d *ReplayDiff) { d.AlignmentVersion = "2.0" }},
		{"intervention literal", func(d *ReplayDiff) { d.Intervention.Unit = "seconds" }},
		{"oracle nested", func(d *ReplayDiff) { d.BaselineOracleResult.Explanation = "" }},
		{"negative effect", func(d *ReplayDiff) { d.BaselineEffectSummary.PaymentAttemptCount = -1 }},
		{"wrong delta", func(d *ReplayDiff) { d.EffectDelta.LedgerCommitCount = 0 }},
		{"nil required array", func(d *ReplayDiff) { d.MatchedEvents = nil }},
		{"alignment field", func(d *ReplayDiff) { d.MatchedEvents[0].BaselineEventID = "" }},
		{"unchanged event", func(d *ReplayDiff) {
			d.ChangedEvents = []EventChange{{BaselineEventID: "a", ComparisonEventID: "b", Field: "status", BaselineValue: "SAME", ComparisonValue: "SAME"}}
		}},
		{"non-json value", func(d *ReplayDiff) { d.FirstMeaningfulDivergence.BaselineValue = math.Inf(1) }},
		{"negative timeline index", func(d *ReplayDiff) { d.FirstMeaningfulDivergence.BaselineTimelineIndex = -1 }},
		{"empty evidence", func(d *ReplayDiff) { d.EvidenceSummary = " " }},
		{"empty limitation", func(d *ReplayDiff) { d.Limitations = []string{""} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := validDiff()
			tt.edit(&d)
			if err := d.Validate(); err == nil {
				t.Fatal("expected validation failure")
			}
		})
	}
}

func TestResetAndCreateRequestValidation(t *testing.T) {
	if err := (ResetRequest{ScenarioID: "checkout_duplicate_effect"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (ResetRequest{ScenarioID: "other"}).Validate(); err == nil {
		t.Fatal("unknown scenario accepted")
	}
	result := ResetResult{SchemaVersion: ContractVersion, ResetID: "reset-1", Status: ResetCompleted, ClearedIncidentCount: 1, ClearedRunCount: 2, ClearedLedgerCount: 3, FixtureVersion: "1.0.0", ConfiguredLatencyMS: 350, NextLogicalOperationID: "checkout-8271"}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := result
	bad.Status = ResetFailed
	if err := bad.Validate(); err == nil {
		t.Fatal("failed reset without error accepted")
	}
	bad = result
	bad.Error = &RunError{Code: InternalFailure, Message: "bad", Details: map[string]any{}}
	if err := bad.Validate(); err == nil {
		t.Fatal("completed reset with error accepted")
	}
	bad = result
	bad.ClearedRunCount = -1
	if err := bad.Validate(); err == nil {
		t.Fatal("negative count accepted")
	}

	baseline := CreateRunRequest{RunType: RunTypeBaseline}
	whatIf := CreateRunRequest{RunType: RunTypeWhatIf, BaselineRunID: "run-base", Intervention: &Intervention{Type: InterventionPaymentLatency, From: 350, To: 50, Unit: InterventionUnitMilliseconds}}
	if baseline.Validate() != nil || whatIf.Validate() != nil {
		t.Fatal("valid create run request rejected")
	}
	whatIf.Intervention.To = -1
	if whatIf.Validate() == nil || (CreateRunRequest{RunType: RunTypeBaseline, BaselineRunID: "unexpected"}).Validate() == nil {
		t.Fatal("invalid create run request accepted")
	}
	if (CreateDiffRequest{BaselineRunID: "base", ComparisonRunID: "comparison"}).Validate() != nil || (CreateDiffRequest{BaselineRunID: "same", ComparisonRunID: "same"}).Validate() == nil {
		t.Fatal("create diff request validation mismatch")
	}
}

func TestFrozenBoundaryTypesCompileAndMarshal(t *testing.T) {
	values := []any{
		IsolationEvidence{NetworkPolicy: VerdictPass, DatastoreDestinations: []string{}, SimulatorInteractions: []DependencyInteraction{}, DeniedInteractions: []DependencyInteraction{}},
		ReplayDiff{AlignmentVersion: ContractVersion, Intervention: Intervention{}, BaselineOracleResult: OracleResult{}, ComparisonOracleResult: OracleResult{}, MatchedEvents: []EventAlignment{}, AddedEventIDs: []string{}, RemovedEventIDs: []string{}, ChangedEvents: []EventChange{}, BaselineEffectSummary: EffectSummary{}, ComparisonEffectSummary: EffectSummary{}, Limitations: []string{}},
		ResetRequest{ScenarioID: "checkout_duplicate_effect"},
		ResetResult{ClearedIncidentCount: 1, ClearedRunCount: 2, ClearedLedgerCount: 3, FixtureVersion: "1.0.0", ConfiguredLatencyMS: 350, DeduplicationEnabled: false, NextLogicalOperationID: "checkout-8271"},
		SystemPackDescriptor{ID: "p", Version: "1.0.0", InterfaceVersion: ContractVersion},
		AcceptedEventResponse{EventID: "e", Status: Accepted}, IncidentListResponse{}, IncidentListQuery{}, IncidentDetailResponse{}, CreateRunRequest{}, CreateDiffRequest{}, APIErrorResponse{},
	}
	for _, value := range values {
		if _, err := json.Marshal(value); err != nil {
			t.Fatalf("marshal %T: %v", value, err)
		}
	}
}

func validEvent() ExecutionEvent {
	return ExecutionEvent{SchemaVersion: ContractVersion, EventID: "e", ExecutionID: "x", TraceID: "t", Component: ComponentRef{Name: "c", Instance: "i"}, Operation: OperationRef{Name: "o", Kind: OperationInternal}, EventType: EventStart, Attempt: 1, LogicalOperationID: "l", OccurredAt: "2026-08-29T10:32:01Z", Sequence: 0, Status: EventRunning, Attributes: map[string]any{}}
}

func validCompletedRun() ReplayRun {
	return ReplayRun{SchemaVersion: ContractVersion, RunID: "r", ExecutionID: "e", CapsuleID: "c", CapsuleHash: strings.Repeat("a", 64), RunType: RunTypeBaseline, TrialNumber: 1, Status: ReplayRunCompleted, Outcome: ReplayOutcomeReproduced, StartedAt: "2026-08-29T10:00:00Z", CompletedAt: "2026-08-29T10:00:01Z", ObservedEventIDs: []string{}, EffectSummary: &EffectSummary{}, FailureOracleResult: &OracleResult{Oracle: FailureOracleRef{ID: "o", Version: "1.0.0"}, Matched: true, EffectSummary: EffectSummary{}, RequiredEvidenceEventIDs: []string{}, Explanation: "x"}, IsolationEvidence: &IsolationEvidence{PolicyVersion: ContractVersion, Verdict: VerdictPass, RuntimeNamespace: "ns", NetworkPolicy: VerdictPass, CredentialProfile: "replay-only", DatastoreDestinations: []string{}, SimulatorInteractions: []DependencyInteraction{}, DeniedInteractions: []DependencyInteraction{}, TeardownResult: VerdictPass}}
}

func validCapsule() ReplayCapsule {
	return ReplayCapsule{SchemaVersion: ContractVersion, CapsuleID: "cap-1", CreatedAt: "2026-08-29T10:33:00Z", Source: CapsuleSource{IncidentID: "inc-1", TraceID: "trace-1", ExecutionID: "exec-1", CaptureEnvironment: CaptureDemo, CapturedAt: "2026-08-29T10:32:01Z"}, SystemPack: SystemPackRef{ID: "checkout_duplicate_effect", Version: "1.0.0", InterfaceVersion: ContractVersion}, Trigger: Trigger{RequestOrMessage: map[string]any{"method": "POST"}, SanitizedHeaders: map[string]string{}}, EventIDs: []string{"evt-1"}, GraphID: "graph-1", StateFixtures: []StateFixture{{FixtureID: "state-1", Kind: StateFixturePostgresRowset, ContentRef: "fixture://golden/state-1", ContentDigest: strings.Repeat("b", 64), SanitizationStatus: SanitizationPass, ResetStrategy: FixtureTruncateAndLoad}}, DependencyFixtures: []DependencyFixture{{FixtureID: "dependency-1", Dependency: DependencyPaymentSimulator, RequestMatch: map[string]any{}, Response: map[string]any{}, LatencyMS: 350, FailureMode: FailureModeNone, InvocationLimit: 2}}, TimingPolicy: TimingPolicy{ClockToleranceMS: 5, TimeoutMS: 200}, ReplayPlan: ReplayPlan{Entrypoint: "gateway.checkout", RequiredComponents: []string{"gateway", "checkout", "payment", "ledger"}, FixtureLoadOrder: []string{"state-1", "dependency-1"}, ResetStrategy: ReplayResetGoldenV1}, FailureOracle: FailureOracleSpec{ID: "duplicate_ledger_effect", Version: "1.0.0", ExpectedMatch: true, ExpectedEffectSummary: EffectSummary{PaymentAttemptCount: 2, LedgerCommitCount: 2}}, AllowedInterventions: []InterventionSpec{{Type: InterventionPaymentLatency, ValueType: InterventionValueInteger, Unit: InterventionUnitMilliseconds, Minimum: 0, Maximum: 5000}}, Safety: SafetyPolicy{PolicyVersion: ContractVersion, SanitizationStatus: SanitizationPass, BlockedDestinations: []string{"production-databases"}, AllowedDestinations: []string{"payment-simulator", "replay-postgres"}, CredentialProfile: CredentialReplayOnly}, Integrity: Integrity{Algorithm: IntegritySHA256, Digest: strings.Repeat("a", 64)}}
}

func validIsolationEvidence() IsolationEvidence {
	return IsolationEvidence{PolicyVersion: ContractVersion, Verdict: VerdictPass, RuntimeNamespace: "replay-run-1", NetworkPolicy: VerdictPass, CredentialProfile: CredentialReplayOnly, DatastoreDestinations: []string{"postgres://replay/run-1"}, SimulatorInteractions: []DependencyInteraction{{Dependency: "payment_simulator", Destination: "http://payment-simulator:8080", Operation: "authorize", Result: InteractionSimulated}}, DeniedInteractions: []DependencyInteraction{}, TeardownResult: VerdictPass}
}

func validOracleResult(matched bool, attempts, commits int) OracleResult {
	return OracleResult{Oracle: FailureOracleRef{ID: "duplicate_ledger_effect", Version: "1.0.0"}, Matched: matched, EffectSummary: EffectSummary{PaymentAttemptCount: attempts, LedgerCommitCount: commits}, RequiredEvidenceEventIDs: []string{"evt-1"}, Explanation: "complete evidence"}
}

func validDiff() ReplayDiff {
	return ReplayDiff{SchemaVersion: ContractVersion, DiffID: "diff-1", BaselineRunID: "run-base", ComparisonRunID: "run-whatif", AlignmentVersion: ContractVersion, Intervention: Intervention{Type: InterventionPaymentLatency, From: 350, To: 50, Unit: InterventionUnitMilliseconds}, BaselineOracleResult: validOracleResult(true, 2, 2), ComparisonOracleResult: validOracleResult(false, 1, 1), MatchedEvents: []EventAlignment{{BaselineEventID: "evt-base", ComparisonEventID: "evt-comparison"}}, AddedEventIDs: []string{}, RemovedEventIDs: []string{"evt-timeout"}, ChangedEvents: []EventChange{}, FirstMeaningfulDivergence: &FirstDivergence{BaselineEventID: "evt-timeout", ComparisonEventID: "evt-success", Rule: "PAYMENT_COMPLETES_BEFORE_TIMEOUT", BaselineValue: "TIMEOUT", ComparisonValue: "SUCCESS", BaselineTimelineIndex: 3, ComparisonTimelineIndex: 3}, BaselineEffectSummary: EffectSummary{PaymentAttemptCount: 2, LedgerCommitCount: 2}, ComparisonEffectSummary: EffectSummary{PaymentAttemptCount: 1, LedgerCommitCount: 1}, EffectDelta: EffectDelta{PaymentAttemptCount: -1, LedgerCommitCount: -1}, EvidenceSummary: "retry and duplicate effect disappeared", Limitations: []string{}}
}
