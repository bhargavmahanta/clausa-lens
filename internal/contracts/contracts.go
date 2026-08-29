package contracts

import (
	"context"
	"fmt"
	"regexp"
)

type ComponentRef struct {
	Name     string `json:"name"`
	Instance string `json:"instance"`
}
type OperationRef struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}
type SystemPackRef struct {
	ID               string `json:"id"`
	Version          string `json:"version"`
	InterfaceVersion string `json:"interface_version"`
}
type FailureOracleRef struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

type ExecutionEvent struct {
	SchemaVersion      string         `json:"schema_version"`
	EventID            string         `json:"event_id"`
	ExecutionID        string         `json:"execution_id"`
	TraceID            string         `json:"trace_id"`
	ParentEventID      string         `json:"parent_event_id,omitempty"`
	ReplayRunID        string         `json:"replay_run_id,omitempty"`
	Component          ComponentRef   `json:"component"`
	Operation          OperationRef   `json:"operation"`
	EventType          string         `json:"event_type"`
	Attempt            int            `json:"attempt"`
	LogicalOperationID string         `json:"logical_operation_id"`
	OccurredAt         string         `json:"occurred_at"`
	Sequence           int            `json:"sequence"`
	DurationMS         *int           `json:"duration_ms,omitempty"`
	Status             string         `json:"status"`
	Attributes         map[string]any `json:"attributes"`
}
type Incident struct {
	SchemaVersion      string           `json:"schema_version"`
	IncidentID         string           `json:"incident_id"`
	Status             string           `json:"status"`
	BlockReason        *ValidationIssue `json:"block_reason,omitempty"`
	FailureOracle      FailureOracleRef `json:"failure_oracle"`
	SystemPack         SystemPackRef    `json:"system_pack"`
	TraceID            string           `json:"trace_id"`
	ExecutionID        string           `json:"execution_id"`
	DetectedAt         string           `json:"detected_at"`
	Summary            string           `json:"summary"`
	EvidenceEventIDs   []string         `json:"evidence_event_ids"`
	GraphID            string           `json:"graph_id,omitempty"`
	SanitizationStatus string           `json:"sanitization_status"`
}
type GraphNode struct {
	EventID       string `json:"event_id"`
	TimelineIndex int    `json:"timeline_index"`
}
type GraphEdge struct {
	EdgeID      string `json:"edge_id"`
	FromEventID string `json:"from_event_id"`
	ToEventID   string `json:"to_event_id"`
	Type        string `json:"type"`
}
type ExecutionGraph struct {
	SchemaVersion         string      `json:"schema_version"`
	GraphID               string      `json:"graph_id"`
	IncidentID            string      `json:"incident_id"`
	OrderingPolicyVersion string      `json:"ordering_policy_version"`
	Nodes                 []GraphNode `json:"nodes"`
	Edges                 []GraphEdge `json:"edges"`
}
type Intervention struct {
	Type string `json:"type"`
	From int    `json:"from"`
	To   int    `json:"to"`
	Unit string `json:"unit"`
}
type EffectSummary struct {
	PaymentAttemptCount int `json:"payment_attempt_count"`
	LedgerCommitCount   int `json:"ledger_commit_count"`
}
type OracleResult struct {
	Oracle                   FailureOracleRef `json:"oracle"`
	Matched                  bool             `json:"matched"`
	EffectSummary            EffectSummary    `json:"effect_summary"`
	RequiredEvidenceEventIDs []string         `json:"required_evidence_event_ids"`
	Explanation              string           `json:"explanation"`
}
type IsolationEvidence struct {
	PolicyVersion     string `json:"policy_version"`
	Verdict           string `json:"verdict"`
	RuntimeNamespace  string `json:"runtime_namespace"`
	CredentialProfile string `json:"credential_profile"`
	TeardownResult    string `json:"teardown_result"`
}
type ReplayRun struct {
	SchemaVersion     string             `json:"schema_version"`
	RunID             string             `json:"run_id"`
	CapsuleID         string             `json:"capsule_id"`
	RunType           string             `json:"run_type"`
	BaselineRunID     string             `json:"baseline_run_id,omitempty"`
	Intervention      *Intervention      `json:"intervention,omitempty"`
	Status            string             `json:"status"`
	Outcome           string             `json:"outcome,omitempty"`
	OracleResult      *OracleResult      `json:"oracle_result,omitempty"`
	IsolationEvidence *IsolationEvidence `json:"isolation_evidence,omitempty"`
}
type StateFixture struct {
	FixtureID          string `json:"fixture_id"`
	Kind               string `json:"kind"`
	ContentRef         string `json:"content_ref"`
	ContentDigest      string `json:"content_digest"`
	SanitizationStatus string `json:"sanitization_status"`
	ResetStrategy      string `json:"reset_strategy"`
}
type DependencyFixture struct {
	FixtureID       string         `json:"fixture_id"`
	Dependency      string         `json:"dependency"`
	RequestMatch    map[string]any `json:"request_match"`
	Response        map[string]any `json:"response"`
	LatencyMS       int            `json:"latency_ms"`
	FailureMode     string         `json:"failure_mode"`
	InvocationLimit int            `json:"invocation_limit"`
}
type TimingPolicy struct {
	ClockToleranceMS int `json:"clock_tolerance_ms"`
	TimeoutMS        int `json:"timeout_ms"`
}
type ReplayPlan struct {
	Entrypoint         string   `json:"entrypoint"`
	RequiredComponents []string `json:"required_components"`
	FixtureLoadOrder   []string `json:"fixture_load_order"`
	ResetStrategy      string   `json:"reset_strategy"`
}
type FailureOracleSpec struct {
	ID                    string        `json:"id"`
	Version               string        `json:"version"`
	ExpectedMatch         bool          `json:"expected_match"`
	ExpectedEffectSummary EffectSummary `json:"expected_effect_summary"`
}
type SafetyPolicy struct {
	PolicyVersion       string   `json:"policy_version"`
	SanitizationStatus  string   `json:"sanitization_status"`
	BlockedDestinations []string `json:"blocked_destinations"`
	AllowedDestinations []string `json:"allowed_destinations"`
	CredentialProfile   string   `json:"credential_profile"`
}
type Trigger struct {
	RequestOrMessage map[string]any    `json:"request_or_message"`
	SanitizedHeaders map[string]string `json:"sanitized_headers"`
}
type CapsuleSource struct {
	IncidentID         string `json:"incident_id"`
	TraceID            string `json:"trace_id"`
	ExecutionID        string `json:"execution_id"`
	CaptureEnvironment string `json:"capture_environment"`
	CapturedAt         string `json:"captured_at"`
}
type ReplayCapsule struct {
	SchemaVersion        string              `json:"schema_version"`
	CapsuleID            string              `json:"capsule_id"`
	CreatedAt            string              `json:"created_at"`
	Source               CapsuleSource       `json:"source"`
	SystemPack           SystemPackRef       `json:"system_pack"`
	Trigger              Trigger             `json:"trigger"`
	EventIDs             []string            `json:"event_ids"`
	GraphID              string              `json:"graph_id"`
	StateFixtures        []StateFixture      `json:"state_fixtures"`
	DependencyFixtures   []DependencyFixture `json:"dependency_fixtures"`
	TimingPolicy         TimingPolicy        `json:"timing_policy"`
	ReplayPlan           ReplayPlan          `json:"replay_plan"`
	FailureOracle        FailureOracleSpec   `json:"failure_oracle"`
	AllowedInterventions []InterventionSpec  `json:"allowed_interventions"`
	Safety               SafetyPolicy        `json:"safety"`
	Integrity            Integrity           `json:"integrity"`
}
type InterventionSpec struct {
	Type      string `json:"type"`
	ValueType string `json:"value_type"`
	Unit      string `json:"unit"`
	Minimum   int    `json:"minimum"`
	Maximum   int    `json:"maximum"`
}
type Integrity struct {
	Algorithm string `json:"algorithm"`
	Digest    string `json:"digest"`
}
type ReplayExecution struct {
	Run    ReplayRun        `json:"run"`
	Events []ExecutionEvent `json:"events"`
	Graph  ExecutionGraph   `json:"graph"`
}
type LabelSet struct {
	Components    map[string]string `json:"components"`
	Operations    map[string]string `json:"operations"`
	EventTypes    map[string]string `json:"event_types"`
	Effects       map[string]string `json:"effects"`
	Interventions map[string]string `json:"interventions"`
}
type RawEvidence struct {
	Source      string `json:"source"`
	ContentType string `json:"content_type"`
	ReceivedAt  string `json:"received_at"`
	Payload     []byte `json:"payload"`
}
type FixtureSet struct {
	StateFixtures      []StateFixture      `json:"state_fixtures"`
	DependencyFixtures []DependencyFixture `json:"dependency_fixtures"`
}
type SystemPack interface {
	Descriptor() SystemPackRef
	Normalize(context.Context, RawEvidence) ([]ExecutionEvent, error)
	DetectIncident(context.Context, []ExecutionEvent) (OracleResult, error)
	ExtractFixtures(context.Context, Incident, []ExecutionEvent) (FixtureSet, error)
	BuildReplayPlan(context.Context, Incident, FixtureSet) (ReplayPlan, error)
	ValidateCapsule(context.Context, ReplayCapsule) []ValidationIssue
	AllowedInterventions() []InterventionSpec
	ApplyIntervention(context.Context, ReplayPlan, Intervention) (ReplayPlan, error)
	Compare(context.Context, string, ReplayExecution, ReplayExecution) (ReplayDiff, error)
	EvaluateOutcome(context.Context, ReplayExecution) (OracleResult, error)
	Labels() LabelSet
}
type ReplayDiff struct {
	SchemaVersion   string      `json:"schema_version"`
	DiffID          string      `json:"diff_id"`
	BaselineRunID   string      `json:"baseline_run_id"`
	ComparisonRunID string      `json:"comparison_run_id"`
	EffectDelta     EffectDelta `json:"effect_delta"`
}
type EffectDelta struct {
	PaymentAttemptCount int `json:"payment_attempt_count"`
	LedgerCommitCount   int `json:"ledger_commit_count"`
}
type ResetResult struct {
	SchemaVersion string `json:"schema_version"`
	ResetID       string `json:"reset_id"`
	Status        string `json:"status"`
	Ready         bool   `json:"ready"`
}
type ErrorCode string

const (
	SchemaInvalid   ErrorCode = "SCHEMA_INVALID"
	InternalFailure ErrorCode = "INTERNAL_FAILURE"
)

type ValidationIssue struct {
	Code    ErrorCode `json:"code"`
	Path    string    `json:"path"`
	Message string    `json:"message"`
}
type RunError struct {
	Code      ErrorCode      `json:"code"`
	Message   string         `json:"message"`
	Retryable bool           `json:"retryable"`
	Details   map[string]any `json:"details,omitempty"`
}

var rfc3339 = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T.*Z$`)

func required(v, n string) error {
	if v == "" {
		return fmt.Errorf("%s is required", n)
	}
	return nil
}
func oneOf(v, n string, xs ...string) error {
	for _, x := range xs {
		if v == x {
			return nil
		}
	}
	return fmt.Errorf("%s has invalid value %q", n, v)
}
func (e ExecutionEvent) Validate() error {
	for _, p := range [][2]string{{e.SchemaVersion, "schema_version"}, {e.EventID, "event_id"}, {e.ExecutionID, "execution_id"}, {e.TraceID, "trace_id"}, {e.Component.Name, "component.name"}, {e.Component.Instance, "component.instance"}, {e.Operation.Name, "operation.name"}, {e.LogicalOperationID, "logical_operation_id"}, {e.OccurredAt, "occurred_at"}} {
		if x := required(p[0], p[1]); x != nil {
			return x
		}
	}
	if e.SchemaVersion != "1.0" {
		return fmt.Errorf("schema_version must be 1.0")
	}
	if !rfc3339.MatchString(e.OccurredAt) {
		return fmt.Errorf("occurred_at must be RFC3339 UTC")
	}
	if e.Attempt < 1 || e.Sequence < 0 {
		return fmt.Errorf("invalid attempt or sequence")
	}
	if x := oneOf(e.EventType, "event_type", "START", "COMPLETE", "ERROR", "TIMEOUT", "RETRY", "EFFECT", "STATE_OBSERVATION"); x != nil {
		return x
	}
	if x := oneOf(e.Status, "status", "RUNNING", "SUCCESS", "FAILED", "TIMEOUT", "BLOCKED"); x != nil {
		return x
	}
	if x := oneOf(e.Operation.Kind, "operation.kind", "ENTRYPOINT", "INTERNAL", "DEPENDENCY", "STATE_CHANGE", "SIDE_EFFECT", "CONTROL"); x != nil {
		return x
	}
	return nil
}
func (r ReplayRun) Validate() error {
	if r.SchemaVersion != "1.0" || r.RunID == "" || r.CapsuleID == "" {
		return fmt.Errorf("missing required replay run field")
	}
	if r.RunType != "BASELINE" && r.RunType != "WHAT_IF" {
		return fmt.Errorf("invalid run_type")
	}
	if r.RunType == "BASELINE" && r.Intervention != nil {
		return fmt.Errorf("baseline cannot include intervention")
	}
	if r.RunType == "WHAT_IF" && (r.BaselineRunID == "" || r.Intervention == nil) {
		return fmt.Errorf("what-if requires baseline_run_id and intervention")
	}
	if r.Status == "COMPLETED" {
		if r.Outcome == "" || r.OracleResult == nil || r.IsolationEvidence == nil {
			return fmt.Errorf("completed run requires outcome, oracle, isolation")
		}
		if r.IsolationEvidence.Verdict != "PASS" {
			return fmt.Errorf("completed run requires isolation PASS")
		}
	}
	if (r.Status == "BLOCKED" || r.Status == "FAILED") && r.Outcome != "" {
		return fmt.Errorf("blocked or failed run cannot have outcome")
	}
	return nil
}
