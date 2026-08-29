package contracts

import (
	"encoding/json"
	"testing"
)

func TestExecutionEventGoldenJSONDecodes(t *testing.T) {
	var event ExecutionEvent
	if err := json.Unmarshal([]byte(`{"schema_version":"1.0","event_id":"evt-payment-1-start","execution_id":"exec-original-8271","trace_id":"trace-8271","parent_event_id":"evt-checkout-start","component":{"name":"payment","instance":"payment-1"},"operation":{"name":"authorize","kind":"DEPENDENCY"},"event_type":"START","attempt":1,"logical_operation_id":"checkout-8271","occurred_at":"2026-08-29T10:32:01.015Z","sequence":1,"status":"RUNNING","attributes":{"configured_latency_ms":350}}`), &event); err != nil {
		t.Fatal(err)
	}
	if err := event.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestExecutionEventRejectsInvalidEnum(t *testing.T) {
	e := validEvent()
	e.EventType = "NOPE"
	if err := e.Validate(); err == nil {
		t.Fatal("expected invalid enum to fail")
	}
}

func TestExecutionEventRejectsMissingRequiredField(t *testing.T) {
	e := validEvent()
	e.EventID = ""
	if err := e.Validate(); err == nil {
		t.Fatal("expected missing event_id to fail")
	}
}

func TestReplayRunLifecycleRules(t *testing.T) {
	r := ReplayRun{SchemaVersion: "1.0", RunID: "run-1", CapsuleID: "cap-1", RunType: "BASELINE", Status: "COMPLETED", Outcome: "REPRODUCED", IsolationEvidence: &IsolationEvidence{PolicyVersion: "1.0", Verdict: "PASS", RuntimeNamespace: "ns", CredentialProfile: "replay-only", TeardownResult: "PASS"}, OracleResult: &OracleResult{Matched: true}}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	r.Intervention = &Intervention{Type: "PAYMENT_LATENCY", From: 350, To: 50, Unit: "ms"}
	if err := r.Validate(); err == nil {
		t.Fatal("baseline with intervention should fail")
	}
}

func validEvent() ExecutionEvent {
	return ExecutionEvent{SchemaVersion: "1.0", EventID: "e", ExecutionID: "x", TraceID: "t", Component: ComponentRef{Name: "c", Instance: "i"}, Operation: OperationRef{Name: "o", Kind: "INTERNAL"}, EventType: "START", Attempt: 1, LogicalOperationID: "l", OccurredAt: "2026-08-29T10:32:01Z", Status: "RUNNING", Attributes: map[string]any{}}
}
