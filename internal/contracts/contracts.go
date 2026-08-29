package contracts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"time"
)

const ContractVersion = "1.0"

type OperationKind string
type EventType string
type EventStatus string
type IncidentStatus string
type SanitizationStatus string
type GraphEdgeType string
type RunType string
type ReplayRunStatus string
type ReplayOutcome string
type Verdict string
type DependencyInteractionResult string
type ResetStatus string
type ErrorCode string
type CaptureEnvironment string
type StateFixtureKind string
type FixtureResetStrategy string
type DependencyName string
type FailureMode string
type ReplayResetStrategy string
type InterventionType string
type InterventionValueType string
type InterventionUnit string
type IntegrityAlgorithm string
type CredentialProfile string
type AcceptedEventStatus string

const (
	OperationEntrypoint  OperationKind = "ENTRYPOINT"
	OperationInternal    OperationKind = "INTERNAL"
	OperationDependency  OperationKind = "DEPENDENCY"
	OperationStateChange OperationKind = "STATE_CHANGE"
	OperationSideEffect  OperationKind = "SIDE_EFFECT"
	OperationControl     OperationKind = "CONTROL"

	EventStart            EventType = "START"
	EventComplete         EventType = "COMPLETE"
	EventError            EventType = "ERROR"
	EventTimeout          EventType = "TIMEOUT"
	EventRetry            EventType = "RETRY"
	EventEffect           EventType = "EFFECT"
	EventStateObservation EventType = "STATE_OBSERVATION"

	EventRunning  EventStatus = "RUNNING"
	EventSuccess  EventStatus = "SUCCESS"
	EventFailed   EventStatus = "FAILED"
	EventTimedOut EventStatus = "TIMEOUT"
	EventBlocked  EventStatus = "BLOCKED"

	IncidentDetected IncidentStatus = "DETECTED"
	IncidentReady    IncidentStatus = "READY"
	IncidentBlocked  IncidentStatus = "BLOCKED"

	SanitizationPass SanitizationStatus = "PASS"
	SanitizationFail SanitizationStatus = "FAIL"
	VerdictPass      Verdict            = "PASS"
	VerdictFail      Verdict            = "FAIL"

	GraphEdgeParentChild GraphEdgeType = "PARENT_CHILD"
	GraphEdgeTemporal    GraphEdgeType = "TEMPORAL"
	GraphEdgeDependency  GraphEdgeType = "DEPENDENCY"
	GraphEdgeRetry       GraphEdgeType = "RETRY"
	GraphEdgeState       GraphEdgeType = "STATE"
	GraphEdgeSideEffect  GraphEdgeType = "SIDE_EFFECT"

	RunTypeBaseline RunType = "BASELINE"
	RunTypeWhatIf   RunType = "WHAT_IF"

	ReplayRunCreated    ReplayRunStatus = "CREATED"
	ReplayRunValidating ReplayRunStatus = "VALIDATING"
	ReplayRunRunning    ReplayRunStatus = "RUNNING"
	ReplayRunCompleted  ReplayRunStatus = "COMPLETED"
	ReplayRunFailed     ReplayRunStatus = "FAILED"
	ReplayRunBlocked    ReplayRunStatus = "BLOCKED"

	ReplayOutcomeReproduced    ReplayOutcome = "REPRODUCED"
	ReplayOutcomeNotReproduced ReplayOutcome = "NOT_REPRODUCED"
	ReplayOutcomeMitigated     ReplayOutcome = "MITIGATED"
	ReplayOutcomeUnchanged     ReplayOutcome = "UNCHANGED"
	ReplayOutcomeInconclusive  ReplayOutcome = "INCONCLUSIVE"

	InteractionSimulated DependencyInteractionResult = "SIMULATED"
	InteractionAllowed   DependencyInteractionResult = "ALLOWED"
	InteractionDenied    DependencyInteractionResult = "DENIED"

	ResetCompleted ResetStatus         = "COMPLETED"
	ResetFailed    ResetStatus         = "FAILED"
	Accepted       AcceptedEventStatus = "ACCEPTED"

	CaptureDemo                  CaptureEnvironment    = "DEMO"
	StateFixturePostgresRowset   StateFixtureKind      = "POSTGRES_ROWSET"
	FixtureTruncateAndLoad       FixtureResetStrategy  = "TRUNCATE_AND_LOAD"
	DependencyPaymentSimulator   DependencyName        = "payment_simulator"
	FailureModeNone              FailureMode           = "NONE"
	ReplayResetGoldenV1          ReplayResetStrategy   = "GOLDEN_RESET_V1"
	InterventionPaymentLatency   InterventionType      = "PAYMENT_LATENCY"
	InterventionValueInteger     InterventionValueType = "INTEGER"
	InterventionUnitMilliseconds InterventionUnit      = "ms"
	IntegritySHA256              IntegrityAlgorithm    = "SHA-256"
	CredentialReplayOnly         CredentialProfile     = "replay-only"

	SchemaInvalid       ErrorCode = "SCHEMA_INVALID"
	IntegrityMismatch   ErrorCode = "INTEGRITY_MISMATCH"
	PackUnavailable     ErrorCode = "PACK_UNAVAILABLE"
	FixtureMissing      ErrorCode = "FIXTURE_MISSING"
	SanitizationFailed  ErrorCode = "SANITIZATION_FAILED"
	IsolationViolation  ErrorCode = "ISOLATION_VIOLATION"
	DestinationBlocked  ErrorCode = "DESTINATION_BLOCKED"
	GraphCycle          ErrorCode = "GRAPH_CYCLE"
	OracleUnavailable   ErrorCode = "ORACLE_UNAVAILABLE"
	InterventionInvalid ErrorCode = "INTERVENTION_INVALID"
	InternalFailure     ErrorCode = "INTERNAL_FAILURE"
)

type ComponentRef struct {
	Name     string `json:"name"`
	Instance string `json:"instance"`
}
type OperationRef struct {
	Name string        `json:"name"`
	Kind OperationKind `json:"kind"`
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
	EventType          EventType      `json:"event_type"`
	Attempt            int            `json:"attempt"`
	LogicalOperationID string         `json:"logical_operation_id"`
	OccurredAt         string         `json:"occurred_at"`
	Sequence           int            `json:"sequence"`
	DurationMS         *int           `json:"duration_ms,omitempty"`
	Status             EventStatus    `json:"status"`
	Attributes         map[string]any `json:"attributes"`
}

type Incident struct {
	SchemaVersion      string             `json:"schema_version"`
	IncidentID         string             `json:"incident_id"`
	Status             IncidentStatus     `json:"status"`
	BlockReason        *ValidationIssue   `json:"block_reason,omitempty"`
	FailureOracle      FailureOracleRef   `json:"failure_oracle"`
	SystemPack         SystemPackRef      `json:"system_pack"`
	TraceID            string             `json:"trace_id"`
	ExecutionID        string             `json:"execution_id"`
	DetectedAt         string             `json:"detected_at"`
	Summary            string             `json:"summary"`
	EvidenceEventIDs   []string           `json:"evidence_event_ids"`
	GraphID            string             `json:"graph_id,omitempty"`
	SanitizationStatus SanitizationStatus `json:"sanitization_status"`
}

type GraphNode struct {
	EventID       string `json:"event_id"`
	TimelineIndex int    `json:"timeline_index"`
}
type GraphEdge struct {
	EdgeID      string        `json:"edge_id"`
	FromEventID string        `json:"from_event_id"`
	ToEventID   string        `json:"to_event_id"`
	Type        GraphEdgeType `json:"type"`
}
type ExecutionGraph struct {
	SchemaVersion         string      `json:"schema_version"`
	GraphID               string      `json:"graph_id"`
	IncidentID            string      `json:"incident_id"`
	OrderingPolicyVersion string      `json:"ordering_policy_version"`
	Nodes                 []GraphNode `json:"nodes"`
	Edges                 []GraphEdge `json:"edges"`
}

type CapsuleSource struct {
	IncidentID         string             `json:"incident_id"`
	TraceID            string             `json:"trace_id"`
	ExecutionID        string             `json:"execution_id"`
	CaptureEnvironment CaptureEnvironment `json:"capture_environment"`
	CapturedAt         string             `json:"captured_at"`
}
type Trigger struct {
	RequestOrMessage map[string]any    `json:"request_or_message"`
	SanitizedHeaders map[string]string `json:"sanitized_headers"`
}
type StateFixture struct {
	FixtureID          string               `json:"fixture_id"`
	Kind               StateFixtureKind     `json:"kind"`
	ContentRef         string               `json:"content_ref"`
	ContentDigest      string               `json:"content_digest"`
	SanitizationStatus SanitizationStatus   `json:"sanitization_status"`
	ResetStrategy      FixtureResetStrategy `json:"reset_strategy"`
}
type DependencyFixture struct {
	FixtureID       string         `json:"fixture_id"`
	Dependency      DependencyName `json:"dependency"`
	RequestMatch    map[string]any `json:"request_match"`
	Response        map[string]any `json:"response"`
	LatencyMS       int            `json:"latency_ms"`
	FailureMode     FailureMode    `json:"failure_mode"`
	InvocationLimit int            `json:"invocation_limit"`
}
type TimingPolicy struct {
	ClockToleranceMS int `json:"clock_tolerance_ms"`
	TimeoutMS        int `json:"timeout_ms"`
}
type ReplayPlan struct {
	Entrypoint         string              `json:"entrypoint"`
	RequiredComponents []string            `json:"required_components"`
	FixtureLoadOrder   []string            `json:"fixture_load_order"`
	ResetStrategy      ReplayResetStrategy `json:"reset_strategy"`
}
type FailureOracleSpec struct {
	ID                    string        `json:"id"`
	Version               string        `json:"version"`
	ExpectedMatch         bool          `json:"expected_match"`
	ExpectedEffectSummary EffectSummary `json:"expected_effect_summary"`
}
type InterventionSpec struct {
	Type      InterventionType      `json:"type"`
	ValueType InterventionValueType `json:"value_type"`
	Unit      InterventionUnit      `json:"unit"`
	Minimum   int                   `json:"minimum"`
	Maximum   int                   `json:"maximum"`
}
type SafetyPolicy struct {
	PolicyVersion       string             `json:"policy_version"`
	SanitizationStatus  SanitizationStatus `json:"sanitization_status"`
	BlockedDestinations []string           `json:"blocked_destinations"`
	AllowedDestinations []string           `json:"allowed_destinations"`
	CredentialProfile   CredentialProfile  `json:"credential_profile"`
}
type Integrity struct {
	Algorithm IntegrityAlgorithm `json:"algorithm"`
	Digest    string             `json:"digest"`
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

type Intervention struct {
	Type InterventionType `json:"type"`
	From int              `json:"from"`
	To   int              `json:"to"`
	Unit InterventionUnit `json:"unit"`
}
type EffectSummary struct {
	PaymentAttemptCount int `json:"payment_attempt_count"`
	LedgerCommitCount   int `json:"ledger_commit_count"`
}
type EffectDelta struct {
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

type DependencyInteraction struct {
	Dependency  string                      `json:"dependency"`
	Destination string                      `json:"destination"`
	Operation   string                      `json:"operation"`
	Result      DependencyInteractionResult `json:"result"`
}
type IsolationEvidence struct {
	PolicyVersion         string                  `json:"policy_version"`
	Verdict               Verdict                 `json:"verdict"`
	RuntimeNamespace      string                  `json:"runtime_namespace"`
	NetworkPolicy         Verdict                 `json:"network_policy"`
	CredentialProfile     CredentialProfile       `json:"credential_profile"`
	DatastoreDestinations []string                `json:"datastore_destinations"`
	SimulatorInteractions []DependencyInteraction `json:"simulator_interactions"`
	DeniedInteractions    []DependencyInteraction `json:"denied_interactions"`
	TeardownResult        Verdict                 `json:"teardown_result"`
}

type ReplayRun struct {
	SchemaVersion       string             `json:"schema_version"`
	RunID               string             `json:"run_id"`
	ExecutionID         string             `json:"execution_id"`
	CapsuleID           string             `json:"capsule_id"`
	CapsuleHash         string             `json:"capsule_hash"`
	RunType             RunType            `json:"run_type"`
	BaselineRunID       string             `json:"baseline_run_id,omitempty"`
	Intervention        *Intervention      `json:"intervention,omitempty"`
	TrialNumber         int                `json:"trial_number"`
	Status              ReplayRunStatus    `json:"status"`
	Outcome             ReplayOutcome      `json:"outcome,omitempty"`
	StartedAt           string             `json:"started_at,omitempty"`
	CompletedAt         string             `json:"completed_at,omitempty"`
	ObservedEventIDs    []string           `json:"observed_event_ids"`
	EffectSummary       *EffectSummary     `json:"effect_summary,omitempty"`
	FailureOracleResult *OracleResult      `json:"failure_oracle_result,omitempty"`
	IsolationEvidence   *IsolationEvidence `json:"isolation_evidence,omitempty"`
	Error               *RunError          `json:"error,omitempty"`
}

type EventAlignment struct {
	BaselineEventID   string `json:"baseline_event_id"`
	ComparisonEventID string `json:"comparison_event_id"`
}
type EventChange struct {
	BaselineEventID   string `json:"baseline_event_id"`
	ComparisonEventID string `json:"comparison_event_id"`
	Field             string `json:"field"`
	BaselineValue     any    `json:"baseline_value"`
	ComparisonValue   any    `json:"comparison_value"`
}
type FirstDivergence struct {
	BaselineEventID         string `json:"baseline_event_id,omitempty"`
	ComparisonEventID       string `json:"comparison_event_id,omitempty"`
	Rule                    string `json:"rule"`
	BaselineValue           any    `json:"baseline_value"`
	ComparisonValue         any    `json:"comparison_value"`
	BaselineTimelineIndex   int    `json:"baseline_timeline_index"`
	ComparisonTimelineIndex int    `json:"comparison_timeline_index"`
}
type ReplayDiff struct {
	SchemaVersion             string           `json:"schema_version"`
	DiffID                    string           `json:"diff_id"`
	BaselineRunID             string           `json:"baseline_run_id"`
	ComparisonRunID           string           `json:"comparison_run_id"`
	AlignmentVersion          string           `json:"alignment_version"`
	Intervention              Intervention     `json:"intervention"`
	BaselineOracleResult      OracleResult     `json:"baseline_oracle_result"`
	ComparisonOracleResult    OracleResult     `json:"comparison_oracle_result"`
	MatchedEvents             []EventAlignment `json:"matched_events"`
	AddedEventIDs             []string         `json:"added_event_ids"`
	RemovedEventIDs           []string         `json:"removed_event_ids"`
	ChangedEvents             []EventChange    `json:"changed_events"`
	FirstMeaningfulDivergence *FirstDivergence `json:"first_meaningful_divergence,omitempty"`
	BaselineEffectSummary     EffectSummary    `json:"baseline_effect_summary"`
	ComparisonEffectSummary   EffectSummary    `json:"comparison_effect_summary"`
	EffectDelta               EffectDelta      `json:"effect_delta"`
	EvidenceSummary           string           `json:"evidence_summary"`
	Limitations               []string         `json:"limitations"`
}

type ResetRequest struct {
	ScenarioID string `json:"scenario_id"`
}
type ResetResult struct {
	SchemaVersion          string      `json:"schema_version"`
	ResetID                string      `json:"reset_id"`
	Status                 ResetStatus `json:"status"`
	ClearedIncidentCount   int         `json:"cleared_incident_count"`
	ClearedRunCount        int         `json:"cleared_run_count"`
	ClearedLedgerCount     int         `json:"cleared_ledger_count"`
	FixtureVersion         string      `json:"fixture_version"`
	ConfiguredLatencyMS    int         `json:"configured_latency_ms"`
	DeduplicationEnabled   bool        `json:"deduplication_enabled"`
	NextLogicalOperationID string      `json:"next_logical_operation_id"`
	Error                  *RunError   `json:"error,omitempty"`
}
type SystemPackDescriptor struct {
	ID               string `json:"id"`
	Version          string `json:"version"`
	InterfaceVersion string `json:"interface_version"`
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
type ReplayExecution struct {
	Run    ReplayRun        `json:"run"`
	Events []ExecutionEvent `json:"events"`
	Graph  ExecutionGraph   `json:"graph"`
}
type LabelSet struct {
	Components    map[string]string    `json:"components"`
	Operations    map[string]string    `json:"operations"`
	EventTypes    map[EventType]string `json:"event_types"`
	Effects       map[string]string    `json:"effects"`
	Interventions map[string]string    `json:"interventions"`
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

type RunError struct {
	Code      ErrorCode      `json:"code"`
	Message   string         `json:"message"`
	Retryable bool           `json:"retryable"`
	Details   map[string]any `json:"details"`
}
type ValidationIssue struct {
	Code    ErrorCode `json:"code"`
	Path    string    `json:"path"`
	Message string    `json:"message"`
}

type AcceptedEventResponse struct {
	EventID string              `json:"event_id"`
	Status  AcceptedEventStatus `json:"status"`
}
type IncidentListResponse struct {
	Items      []Incident `json:"items"`
	NextCursor string     `json:"next_cursor,omitempty"`
}
type IncidentListQuery struct {
	Status IncidentStatus `json:"status,omitempty"`
	Cursor string         `json:"cursor,omitempty"`
	Limit  *int           `json:"limit,omitempty"`
}
type IncidentDetailResponse struct {
	Incident Incident         `json:"incident"`
	Graph    ExecutionGraph   `json:"graph"`
	Events   []ExecutionEvent `json:"events"`
}
type CreateRunRequest struct {
	RunType       RunType       `json:"run_type"`
	BaselineRunID string        `json:"baseline_run_id,omitempty"`
	Intervention  *Intervention `json:"intervention,omitempty"`
}
type CreateDiffRequest struct {
	BaselineRunID   string `json:"baseline_run_id"`
	ComparisonRunID string `json:"comparison_run_id"`
}
type APIErrorResponse struct {
	Error RunError `json:"error"`
}

// DecodeStrict decodes exactly one contract object and rejects unknown fields.
func DecodeStrict(r io.Reader, dst any) error {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return fmt.Errorf("trailing JSON: %w", err)
	}
	return nil
}

func (e ExecutionEvent) Validate() error {
	for name, value := range map[string]string{"schema_version": e.SchemaVersion, "event_id": e.EventID, "execution_id": e.ExecutionID, "trace_id": e.TraceID, "component.name": e.Component.Name, "component.instance": e.Component.Instance, "operation.name": e.Operation.Name, "logical_operation_id": e.LogicalOperationID, "occurred_at": e.OccurredAt} {
		if value == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	if e.SchemaVersion != ContractVersion {
		return fmt.Errorf("schema_version must be %s", ContractVersion)
	}
	if !validOperationKind(e.Operation.Kind) || !validEventType(e.EventType) || !validEventStatus(e.Status) {
		return fmt.Errorf("invalid execution event enum")
	}
	if e.Attempt < 1 {
		return fmt.Errorf("attempt must be at least 1")
	}
	if e.Sequence < 0 {
		return fmt.Errorf("sequence must be non-negative")
	}
	if e.DurationMS != nil && *e.DurationMS < 0 {
		return fmt.Errorf("duration_ms must be non-negative")
	}
	if !validUTCTime(e.OccurredAt) {
		return fmt.Errorf("occurred_at must be RFC3339 UTC")
	}
	if e.Attributes == nil {
		return fmt.Errorf("attributes must be an object")
	}
	for key, value := range e.Attributes {
		switch key {
		case "configured_latency_ms":
			if !nonnegativeInteger(value) {
				return fmt.Errorf("attributes.%s must be a non-negative integer", key)
			}
		case "checkout_timeout_ms":
			if !positiveInteger(value) {
				return fmt.Errorf("attributes.%s must be a positive integer", key)
			}
		case "effect_id", "dependency_name":
			if _, ok := value.(string); !ok {
				return fmt.Errorf("attributes.%s must be a string", key)
			}
		case "effect_committed":
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("attributes.%s must be a boolean", key)
			}
		default:
			return fmt.Errorf("attribute %q is not allowed", key)
		}
	}
	return nil
}

func (i Incident) Validate() error {
	if i.SchemaVersion != ContractVersion || i.IncidentID == "" || i.TraceID == "" || i.ExecutionID == "" || i.Summary == "" || i.FailureOracle.ID == "" || i.FailureOracle.Version == "" || i.SystemPack.ID == "" || i.SystemPack.Version == "" || i.SystemPack.InterfaceVersion != ContractVersion {
		return fmt.Errorf("missing or invalid incident field")
	}
	if !validUTCTime(i.DetectedAt) || len(i.EvidenceEventIDs) == 0 || !oneOf(string(i.SanitizationStatus), "PASS", "FAIL") || !oneOf(string(i.Status), "DETECTED", "READY", "BLOCKED") {
		return fmt.Errorf("invalid incident field")
	}
	if i.Status == IncidentBlocked && i.BlockReason == nil {
		return fmt.Errorf("blocked incident requires block_reason")
	}
	if i.Status != IncidentBlocked && i.BlockReason != nil {
		return fmt.Errorf("only blocked incident may have block_reason")
	}
	if i.Status == IncidentReady && (i.GraphID == "" || i.SanitizationStatus != SanitizationPass) {
		return fmt.Errorf("ready incident requires graph and passing sanitization")
	}
	if i.BlockReason != nil {
		return i.BlockReason.Validate()
	}
	return nil
}

func (g ExecutionGraph) Validate() error {
	if g.SchemaVersion != ContractVersion || g.GraphID == "" || g.IncidentID == "" || g.OrderingPolicyVersion != ContractVersion {
		return fmt.Errorf("missing or invalid graph field")
	}
	nodes, indices := map[string]bool{}, map[int]bool{}
	for _, n := range g.Nodes {
		if n.EventID == "" || n.TimelineIndex < 0 || nodes[n.EventID] || indices[n.TimelineIndex] {
			return fmt.Errorf("invalid or duplicate graph node")
		}
		nodes[n.EventID], indices[n.TimelineIndex] = true, true
	}
	adj, edgeIDs := map[string][]string{}, map[string]bool{}
	for _, e := range g.Edges {
		if e.EdgeID == "" || edgeIDs[e.EdgeID] || !nodes[e.FromEventID] || !nodes[e.ToEventID] || e.FromEventID == e.ToEventID || !validGraphEdgeType(e.Type) {
			return fmt.Errorf("invalid graph edge")
		}
		edgeIDs[e.EdgeID] = true
		if e.Type != GraphEdgeTemporal {
			adj[e.FromEventID] = append(adj[e.FromEventID], e.ToEventID)
		}
	}
	state := map[string]uint8{}
	var visit func(string) bool
	visit = func(n string) bool {
		if state[n] == 1 {
			return true
		}
		if state[n] == 2 {
			return false
		}
		state[n] = 1
		for _, next := range adj[n] {
			if visit(next) {
				return true
			}
		}
		state[n] = 2
		return false
	}
	for n := range nodes {
		if visit(n) {
			return fmt.Errorf("hard ordering constraints contain a cycle")
		}
	}
	return nil
}

func (c ReplayCapsule) Validate() error {
	if c.SchemaVersion != ContractVersion || c.CapsuleID == "" || c.GraphID == "" || !validUTCTime(c.CreatedAt) {
		return fmt.Errorf("missing or invalid replay capsule field")
	}
	if c.Source.IncidentID == "" || c.Source.TraceID == "" || c.Source.ExecutionID == "" || c.Source.CaptureEnvironment != CaptureDemo || !validUTCTime(c.Source.CapturedAt) {
		return fmt.Errorf("invalid capsule source")
	}
	if c.SystemPack.ID == "" || c.SystemPack.Version == "" || c.SystemPack.InterfaceVersion != ContractVersion {
		return fmt.Errorf("invalid system pack")
	}
	if !validJSONObject(c.Trigger.RequestOrMessage) || c.Trigger.SanitizedHeaders == nil || len(c.EventIDs) == 0 || !nonemptyStrings(c.EventIDs) {
		return fmt.Errorf("invalid capsule trigger or event_ids")
	}
	fixtureIDs := make(map[string]bool, len(c.StateFixtures)+len(c.DependencyFixtures))
	if len(c.StateFixtures) == 0 || len(c.DependencyFixtures) == 0 {
		return fmt.Errorf("fixture arrays are required")
	}
	for _, fixture := range c.StateFixtures {
		if fixture.FixtureID == "" || fixtureIDs[fixture.FixtureID] || fixture.Kind != StateFixturePostgresRowset || !strings.HasPrefix(fixture.ContentRef, "fixture://") || !sha256Pattern.MatchString(fixture.ContentDigest) || fixture.SanitizationStatus != SanitizationPass || fixture.ResetStrategy != FixtureTruncateAndLoad {
			return fmt.Errorf("invalid state fixture")
		}
		fixtureIDs[fixture.FixtureID] = true
	}
	for _, fixture := range c.DependencyFixtures {
		if fixture.FixtureID == "" || fixtureIDs[fixture.FixtureID] || fixture.Dependency != DependencyPaymentSimulator || !validJSONObject(fixture.RequestMatch) || !validJSONObject(fixture.Response) || fixture.LatencyMS < 0 || fixture.FailureMode != FailureModeNone || fixture.InvocationLimit < 1 {
			return fmt.Errorf("invalid dependency fixture")
		}
		fixtureIDs[fixture.FixtureID] = true
	}
	if c.TimingPolicy.ClockToleranceMS != 5 || c.TimingPolicy.TimeoutMS != 200 || c.ReplayPlan.Entrypoint != "gateway.checkout" || !reflect.DeepEqual(c.ReplayPlan.RequiredComponents, []string{"gateway", "checkout", "payment", "ledger"}) || c.ReplayPlan.FixtureLoadOrder == nil || c.ReplayPlan.ResetStrategy != ReplayResetGoldenV1 {
		return fmt.Errorf("invalid replay timing or plan")
	}
	for _, fixtureID := range c.ReplayPlan.FixtureLoadOrder {
		if !fixtureIDs[fixtureID] {
			return fmt.Errorf("fixture_load_order contains unknown fixture")
		}
	}
	if len(c.ReplayPlan.FixtureLoadOrder) != len(fixtureIDs) {
		return fmt.Errorf("fixture_load_order must contain every fixture exactly once")
	}
	if c.FailureOracle.ID != "duplicate_ledger_effect" || c.FailureOracle.Version != "1.0.0" || !c.FailureOracle.ExpectedMatch || c.FailureOracle.ExpectedEffectSummary != (EffectSummary{PaymentAttemptCount: 2, LedgerCommitCount: 2}) {
		return fmt.Errorf("invalid failure oracle")
	}
	if len(c.AllowedInterventions) != 1 || c.AllowedInterventions[0] != (InterventionSpec{Type: InterventionPaymentLatency, ValueType: InterventionValueInteger, Unit: InterventionUnitMilliseconds, Minimum: 0, Maximum: 5000}) {
		return fmt.Errorf("invalid allowed interventions")
	}
	if c.Safety.PolicyVersion != ContractVersion || c.Safety.SanitizationStatus != SanitizationPass || len(c.Safety.BlockedDestinations) == 0 || !nonemptyStrings(c.Safety.BlockedDestinations) || c.Safety.AllowedDestinations == nil || c.Safety.CredentialProfile != CredentialReplayOnly {
		return fmt.Errorf("invalid safety policy")
	}
	for _, destination := range c.Safety.AllowedDestinations {
		if destination != "payment-simulator" && destination != "replay-postgres" {
			return fmt.Errorf("allowed destination is not replay-only")
		}
	}
	if c.Integrity.Algorithm != IntegritySHA256 || !sha256Pattern.MatchString(c.Integrity.Digest) {
		return fmt.Errorf("invalid capsule integrity")
	}
	return nil
}

func (i IsolationEvidence) Validate() error {
	if i.PolicyVersion != ContractVersion || !oneOf(string(i.Verdict), "PASS", "FAIL") || i.RuntimeNamespace == "" || !oneOf(string(i.NetworkPolicy), "PASS", "FAIL") || i.CredentialProfile != CredentialReplayOnly || i.DatastoreDestinations == nil || i.SimulatorInteractions == nil || i.DeniedInteractions == nil || !oneOf(string(i.TeardownResult), "PASS", "FAIL") || !nonemptyStrings(i.DatastoreDestinations) {
		return fmt.Errorf("invalid isolation evidence")
	}
	for _, interaction := range i.SimulatorInteractions {
		if err := interaction.validate(); err != nil || interaction.Result == InteractionDenied || !replayOnlyHost(interaction.Destination) {
			return fmt.Errorf("invalid simulator interaction")
		}
	}
	for _, interaction := range i.DeniedInteractions {
		if err := interaction.validate(); err != nil || interaction.Result != InteractionDenied {
			return fmt.Errorf("invalid denied interaction")
		}
	}
	failed := i.NetworkPolicy == VerdictFail || i.TeardownResult == VerdictFail || len(i.DeniedInteractions) != 0
	for _, destination := range i.DatastoreDestinations {
		failed = failed || !strings.HasPrefix(destination, "postgres://replay/")
	}
	if (i.Verdict == VerdictFail) != failed {
		return fmt.Errorf("isolation verdict is inconsistent with evidence")
	}
	return nil
}

func (i DependencyInteraction) validate() error {
	if i.Dependency == "" || i.Destination == "" || i.Operation == "" || !oneOf(string(i.Result), "SIMULATED", "ALLOWED", "DENIED") {
		return fmt.Errorf("invalid dependency interaction")
	}
	return nil
}

// replayOnlyHost reports whether an interaction destination resolves to a
// replay-only simulator rather than an uncontrolled or production endpoint.
func replayOnlyHost(destination string) bool {
	host := destination
	if parsed, err := url.Parse(destination); err == nil && parsed.Host != "" {
		host = parsed.Hostname()
	}
	return host == "payment-simulator" || host == "replay-postgres" || strings.HasPrefix(host, "payment-simulator:") || strings.HasPrefix(host, "replay-postgres:")
}

func (d ReplayDiff) Validate() error {
	if d.SchemaVersion != ContractVersion || d.DiffID == "" || d.BaselineRunID == "" || d.ComparisonRunID == "" || d.BaselineRunID == d.ComparisonRunID || d.AlignmentVersion != ContractVersion || strings.TrimSpace(d.EvidenceSummary) == "" {
		return fmt.Errorf("missing or invalid replay diff field")
	}
	if err := d.Intervention.validate(); err != nil {
		return err
	}
	if err := d.BaselineOracleResult.validate(); err != nil {
		return err
	}
	if err := d.ComparisonOracleResult.validate(); err != nil {
		return err
	}
	if err := d.BaselineEffectSummary.validate(); err != nil {
		return err
	}
	if err := d.ComparisonEffectSummary.validate(); err != nil {
		return err
	}
	if d.BaselineOracleResult.EffectSummary != d.BaselineEffectSummary || d.ComparisonOracleResult.EffectSummary != d.ComparisonEffectSummary || d.EffectDelta.PaymentAttemptCount != d.ComparisonEffectSummary.PaymentAttemptCount-d.BaselineEffectSummary.PaymentAttemptCount || d.EffectDelta.LedgerCommitCount != d.ComparisonEffectSummary.LedgerCommitCount-d.BaselineEffectSummary.LedgerCommitCount {
		return fmt.Errorf("effect summaries or delta are inconsistent")
	}
	if d.MatchedEvents == nil || d.AddedEventIDs == nil || d.RemovedEventIDs == nil || d.ChangedEvents == nil || d.Limitations == nil || !nonemptyStrings(d.AddedEventIDs) || !nonemptyStrings(d.RemovedEventIDs) || !nonemptyStrings(d.Limitations) {
		return fmt.Errorf("invalid replay diff array")
	}
	for _, alignment := range d.MatchedEvents {
		if alignment.BaselineEventID == "" || alignment.ComparisonEventID == "" {
			return fmt.Errorf("invalid event alignment")
		}
	}
	for _, change := range d.ChangedEvents {
		if change.BaselineEventID == "" || change.ComparisonEventID == "" || change.Field == "" || !validJSONValue(change.BaselineValue) || !validJSONValue(change.ComparisonValue) || reflect.DeepEqual(change.BaselineValue, change.ComparisonValue) {
			return fmt.Errorf("invalid event change")
		}
	}
	if d.FirstMeaningfulDivergence != nil {
		v := d.FirstMeaningfulDivergence
		if v.BaselineEventID == "" && v.ComparisonEventID == "" || v.Rule == "" || v.BaselineTimelineIndex < 0 || v.ComparisonTimelineIndex < 0 || !validJSONValue(v.BaselineValue) || !validJSONValue(v.ComparisonValue) || reflect.DeepEqual(v.BaselineValue, v.ComparisonValue) {
			return fmt.Errorf("invalid first meaningful divergence")
		}
	}
	return nil
}

func (r ResetRequest) Validate() error {
	if r.ScenarioID != "checkout_duplicate_effect" {
		return fmt.Errorf("scenario_id must be checkout_duplicate_effect")
	}
	return nil
}

func (r ResetResult) Validate() error {
	if r.SchemaVersion != ContractVersion || r.ResetID == "" || !oneOf(string(r.Status), "COMPLETED", "FAILED") || r.ClearedIncidentCount < 0 || r.ClearedRunCount < 0 || r.ClearedLedgerCount < 0 || r.FixtureVersion != "1.0.0" || r.ConfiguredLatencyMS != 350 || r.DeduplicationEnabled || r.NextLogicalOperationID != "checkout-8271" {
		return fmt.Errorf("missing or invalid reset result field")
	}
	if (r.Status == ResetFailed) != (r.Error != nil) {
		return fmt.Errorf("error is required exactly for failed reset")
	}
	if r.Error != nil {
		return r.Error.Validate()
	}
	return nil
}

func (r CreateRunRequest) Validate() error {
	if r.RunType == RunTypeBaseline {
		if r.BaselineRunID != "" || r.Intervention != nil {
			return fmt.Errorf("baseline must omit baseline_run_id and intervention")
		}
		return nil
	}
	if r.RunType != RunTypeWhatIf || r.BaselineRunID == "" || r.Intervention == nil {
		return fmt.Errorf("what-if requires baseline_run_id and intervention")
	}
	return r.Intervention.validate()
}

func (r CreateDiffRequest) Validate() error {
	if r.BaselineRunID == "" || r.ComparisonRunID == "" || r.BaselineRunID == r.ComparisonRunID {
		return fmt.Errorf("distinct baseline_run_id and comparison_run_id are required")
	}
	return nil
}

func (r ReplayRun) Validate() error {
	if r.SchemaVersion != ContractVersion || r.RunID == "" || r.ExecutionID == "" || r.CapsuleID == "" || !sha256Pattern.MatchString(r.CapsuleHash) || r.TrialNumber < 1 || !validRunStatus(r.Status) {
		return fmt.Errorf("missing or invalid replay run field")
	}
	if r.RunType == RunTypeBaseline {
		if r.BaselineRunID != "" || r.Intervention != nil {
			return fmt.Errorf("baseline must omit baseline_run_id and intervention")
		}
	} else if r.RunType == RunTypeWhatIf {
		if r.BaselineRunID == "" || r.Intervention == nil {
			return fmt.Errorf("what-if requires baseline_run_id and intervention")
		}
	} else {
		return fmt.Errorf("invalid run_type")
	}
	if r.Intervention != nil {
		if err := r.Intervention.validate(); err != nil {
			return err
		}
	}
	if r.IsolationEvidence != nil && r.Status != ReplayRunCompleted {
		if err := r.IsolationEvidence.Validate(); err != nil {
			return err
		}
	}
	if r.StartedAt != "" && !validUTCTime(r.StartedAt) {
		return fmt.Errorf("started_at must be RFC3339 UTC")
	}
	if r.CompletedAt != "" && !validUTCTime(r.CompletedAt) {
		return fmt.Errorf("completed_at must be RFC3339 UTC")
	}
	if (r.Status == ReplayRunCreated || r.Status == ReplayRunValidating) && r.StartedAt != "" {
		return fmt.Errorf("started_at must be absent before runtime")
	}
	if r.Status == ReplayRunRunning && r.StartedAt == "" {
		return fmt.Errorf("started_at is required once runtime begins")
	}
	if (r.Status == ReplayRunCompleted || r.Status == ReplayRunFailed) && r.StartedAt == "" {
		return fmt.Errorf("started_at is required for runtime-terminal statuses")
	}
	terminal := r.Status == ReplayRunCompleted || r.Status == ReplayRunFailed || r.Status == ReplayRunBlocked
	if terminal != (r.CompletedAt != "") {
		return fmt.Errorf("completed_at is required exactly for terminal statuses")
	}
	if r.Status == ReplayRunCompleted {
		if r.Outcome == "" || r.EffectSummary == nil || r.FailureOracleResult == nil || r.IsolationEvidence == nil || r.IsolationEvidence.Verdict != VerdictPass || r.Error != nil {
			return fmt.Errorf("completed run lacks required safe result")
		}
		if r.RunType == RunTypeBaseline && !oneOf(string(r.Outcome), "REPRODUCED", "NOT_REPRODUCED", "INCONCLUSIVE") {
			return fmt.Errorf("outcome is invalid for baseline")
		}
		if r.RunType == RunTypeWhatIf && !oneOf(string(r.Outcome), "MITIGATED", "UNCHANGED", "INCONCLUSIVE") {
			return fmt.Errorf("outcome is invalid for what-if")
		}
		if err := r.EffectSummary.validate(); err != nil {
			return err
		}
		if err := r.FailureOracleResult.validate(); err != nil {
			return err
		}
		if err := r.IsolationEvidence.Validate(); err != nil {
			return err
		}
		if r.RunType == RunTypeBaseline {
			if r.Outcome == ReplayOutcomeReproduced && !r.FailureOracleResult.Matched {
				return fmt.Errorf("REPRODUCED requires a matched oracle")
			}
			if r.Outcome == ReplayOutcomeNotReproduced && r.FailureOracleResult.Matched {
				return fmt.Errorf("NOT_REPRODUCED requires an unmatched oracle")
			}
		} else if r.RunType == RunTypeWhatIf {
			if r.Outcome == ReplayOutcomeMitigated && r.FailureOracleResult.Matched {
				return fmt.Errorf("MITIGATED requires an unmatched oracle")
			}
			if r.Outcome == ReplayOutcomeUnchanged && !r.FailureOracleResult.Matched {
				return fmt.Errorf("UNCHANGED requires a matched oracle")
			}
		}
	} else if r.Outcome != "" {
		return fmt.Errorf("non-completed run must omit outcome")
	}
	needsError := r.Status == ReplayRunFailed || r.Status == ReplayRunBlocked
	if needsError != (r.Error != nil) {
		return fmt.Errorf("error is required exactly for failed and blocked runs")
	}
	if r.Error != nil {
		if r.Status == ReplayRunBlocked && (r.Error.Code == IsolationViolation || r.Error.Code == DestinationBlocked) && r.IsolationEvidence == nil {
			return fmt.Errorf("isolation-related blocked run requires isolation evidence")
		}
		return r.Error.Validate()
	}
	return nil
}

func (i Intervention) validate() error {
	if i.Type != InterventionPaymentLatency || i.Unit != InterventionUnitMilliseconds || i.From < 0 || i.From > 5000 || i.To < 0 || i.To > 5000 || i.From == i.To {
		return fmt.Errorf("invalid intervention")
	}
	return nil
}

func (e EffectSummary) validate() error {
	if e.PaymentAttemptCount < 0 || e.LedgerCommitCount < 0 {
		return fmt.Errorf("effect counts must be non-negative")
	}
	return nil
}

func (o OracleResult) validate() error {
	if o.Oracle.ID == "" || o.Oracle.Version == "" || o.RequiredEvidenceEventIDs == nil || !nonemptyStrings(o.RequiredEvidenceEventIDs) || strings.TrimSpace(o.Explanation) == "" {
		return fmt.Errorf("invalid oracle result")
	}
	return o.EffectSummary.validate()
}

func CanTransitionReplayRun(from, to ReplayRunStatus) bool {
	switch from {
	case ReplayRunCreated:
		return to == ReplayRunValidating
	case ReplayRunValidating:
		return to == ReplayRunRunning || to == ReplayRunBlocked || to == ReplayRunFailed
	case ReplayRunRunning:
		return to == ReplayRunCompleted || to == ReplayRunBlocked || to == ReplayRunFailed
	}
	return false
}

func (e RunError) Validate() error {
	if !validErrorCode(e.Code) || strings.TrimSpace(e.Message) == "" || e.Details == nil {
		return fmt.Errorf("invalid run error")
	}
	return nil
}
func (v ValidationIssue) Validate() error {
	if !validErrorCode(v.Code) || !strings.HasPrefix(v.Path, "/") || strings.TrimSpace(v.Message) == "" {
		return fmt.Errorf("invalid validation issue")
	}
	return nil
}

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func validUTCTime(s string) bool {
	t, err := time.Parse(time.RFC3339Nano, s)
	return err == nil && strings.HasSuffix(s, "Z") && t.Location() == time.UTC
}
func oneOf(v string, values ...string) bool {
	for _, candidate := range values {
		if v == candidate {
			return true
		}
	}
	return false
}
func nonemptyStrings(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return false
		}
	}
	return true
}
func validJSONObject(value map[string]any) bool {
	return value != nil && validJSONValue(value)
}
func validJSONValue(value any) bool {
	_, err := json.Marshal(value)
	return err == nil
}
func validOperationKind(v OperationKind) bool {
	return oneOf(string(v), "ENTRYPOINT", "INTERNAL", "DEPENDENCY", "STATE_CHANGE", "SIDE_EFFECT", "CONTROL")
}
func validEventType(v EventType) bool {
	return oneOf(string(v), "START", "COMPLETE", "ERROR", "TIMEOUT", "RETRY", "EFFECT", "STATE_OBSERVATION")
}
func validEventStatus(v EventStatus) bool {
	return oneOf(string(v), "RUNNING", "SUCCESS", "FAILED", "TIMEOUT", "BLOCKED")
}
func validGraphEdgeType(v GraphEdgeType) bool {
	return oneOf(string(v), "PARENT_CHILD", "TEMPORAL", "DEPENDENCY", "RETRY", "STATE", "SIDE_EFFECT")
}
func validRunStatus(v ReplayRunStatus) bool {
	return oneOf(string(v), "CREATED", "VALIDATING", "RUNNING", "COMPLETED", "FAILED", "BLOCKED")
}
func validErrorCode(v ErrorCode) bool {
	return oneOf(string(v), "SCHEMA_INVALID", "INTEGRITY_MISMATCH", "PACK_UNAVAILABLE", "FIXTURE_MISSING", "SANITIZATION_FAILED", "ISOLATION_VIOLATION", "DESTINATION_BLOCKED", "GRAPH_CYCLE", "ORACLE_UNAVAILABLE", "INTERVENTION_INVALID", "INTERNAL_FAILURE")
}
func nonnegativeInteger(v any) bool {
	switch n := v.(type) {
	case int:
		return n >= 0
	case int8:
		return n >= 0
	case int16:
		return n >= 0
	case int32:
		return n >= 0
	case int64:
		return n >= 0
	case uint, uint8, uint16, uint32, uint64:
		return true
	case float64:
		return n >= 0 && !math.IsInf(n, 0) && !math.IsNaN(n) && n == math.Trunc(n)
	case json.Number:
		i, err := n.Int64()
		return err == nil && i >= 0
	}
	return false
}
func positiveInteger(v any) bool {
	if !nonnegativeInteger(v) {
		return false
	}
	switch n := v.(type) {
	case int:
		return n > 0
	case int8:
		return n > 0
	case int16:
		return n > 0
	case int32:
		return n > 0
	case int64:
		return n > 0
	case uint:
		return n > 0
	case uint8:
		return n > 0
	case uint16:
		return n > 0
	case uint32:
		return n > 0
	case uint64:
		return n > 0
	case float64:
		return n > 0
	case json.Number:
		i, _ := n.Int64()
		return i > 0
	}
	return false
}
