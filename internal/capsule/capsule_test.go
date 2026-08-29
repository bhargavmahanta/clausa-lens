package capsule

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/causalens/causalens/internal/contracts"
)

// fakePack supplies deterministic fixtures, a replay plan, allowed
// interventions, and receives a real capsule for validation. It does NOT
// import internal/systempack; it implements the frozen contracts.SystemPack
// seam so Compile is testable before the real pack exists.
type fakePack struct {
	descriptor     contracts.SystemPackRef
	fixtures       contracts.FixtureSet
	plan           contracts.ReplayPlan
	interventions  []contracts.InterventionSpec
	validateIssues []contracts.ValidationIssue
}

func (p *fakePack) Descriptor() contracts.SystemPackRef { return p.descriptor }
func (p *fakePack) Normalize(context.Context, contracts.RawEvidence) ([]contracts.ExecutionEvent, error) {
	return nil, nil
}
func (p *fakePack) DetectIncident(context.Context, []contracts.ExecutionEvent) (contracts.OracleResult, error) {
	return contracts.OracleResult{}, nil
}
func (p *fakePack) ExtractFixtures(context.Context, contracts.Incident, []contracts.ExecutionEvent) (contracts.FixtureSet, error) {
	return p.fixtures, nil
}
func (p *fakePack) BuildReplayPlan(context.Context, contracts.Incident, contracts.FixtureSet) (contracts.ReplayPlan, error) {
	return p.plan, nil
}
func (p *fakePack) ValidateCapsule(context.Context, contracts.ReplayCapsule) []contracts.ValidationIssue {
	return p.validateIssues
}
func (p *fakePack) AllowedInterventions() []contracts.InterventionSpec { return p.interventions }
func (p *fakePack) ApplyIntervention(context.Context, contracts.ReplayPlan, contracts.Intervention) (contracts.ReplayPlan, error) {
	return contracts.ReplayPlan{}, nil
}
func (p *fakePack) Compare(context.Context, string, contracts.ReplayExecution, contracts.ReplayExecution) (contracts.ReplayDiff, error) {
	return contracts.ReplayDiff{}, nil
}
func (p *fakePack) EvaluateOutcome(context.Context, contracts.ReplayExecution) (contracts.OracleResult, error) {
	return contracts.OracleResult{}, nil
}
func (p *fakePack) Labels() contracts.LabelSet { return contracts.LabelSet{} }

func (p *fakePack) withIssues(issues ...contracts.ValidationIssue) *fakePack {
	clone := *p
	clone.validateIssues = issues
	return &clone
}

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

func readyIncident() contracts.Incident {
	return contracts.Incident{
		SchemaVersion:      contracts.ContractVersion,
		IncidentID:         "inc-8271",
		Status:             contracts.IncidentReady,
		FailureOracle:      contracts.FailureOracleRef{ID: "duplicate_ledger_effect", Version: "1.0.0"},
		SystemPack:         contracts.SystemPackRef{ID: "checkout_duplicate_effect", Version: "1.0.0", InterfaceVersion: contracts.ContractVersion},
		TraceID:            "trace-8271",
		ExecutionID:        "exec-original-8271",
		DetectedAt:         "2026-08-29T10:32:01.561Z",
		Summary:            "Timeout-driven retry committed two ledger effects.",
		EvidenceEventIDs:   []string{"evt-timeout", "evt-retry"},
		GraphID:            "graph-8271",
		SanitizationStatus: contracts.SanitizationPass,
	}
}

func testEvents() []contracts.ExecutionEvent {
	return []contracts.ExecutionEvent{
		validEvent("evt-timeout", "checkout-8271", contracts.EventTimeout, contracts.EventTimedOut),
		validEvent("evt-retry", "checkout-8271", contracts.EventRetry, contracts.EventSuccess),
	}
}

func validEvent(id, logical string, etype contracts.EventType, status contracts.EventStatus) contracts.ExecutionEvent {
	return contracts.ExecutionEvent{
		SchemaVersion:      contracts.ContractVersion,
		EventID:            id,
		ExecutionID:        "exec-original-8271",
		TraceID:            "trace-8271",
		Component:          contracts.ComponentRef{Name: "payment", Instance: "payment-1"},
		Operation:          contracts.OperationRef{Name: "authorize", Kind: contracts.OperationDependency},
		EventType:          etype,
		Attempt:            1,
		LogicalOperationID: logical,
		OccurredAt:         "2026-08-29T10:32:01.015Z",
		Sequence:           1,
		Status:             status,
		Attributes:         map[string]any{},
	}
}

var lowerHex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

func TestCanonicalMarshalExcludesDigestAndDeterministic(t *testing.T) {
	first, err := Compile(newFakePack(), newFakePack().descriptor, readyIncident(), testEvents(),
		testSource(), testTrigger(), "cap-8271", "graph-8271", "2026-08-29T10:33:00Z")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	second := first
	second.Integrity.Digest = strings.Repeat("f", 64)

	b1, err := CanonicalMarshal(first)
	if err != nil {
		t.Fatalf("canonical first: %v", err)
	}
	b2, err := CanonicalMarshal(second)
	if err != nil {
		t.Fatalf("canonical second: %v", err)
	}
	if string(b1) != string(b2) {
		t.Fatalf("canonical marshal depends on integrity.digest\nb1=%s\nb2=%s", b1, b2)
	}
	var decoded struct {
		Integrity contracts.Integrity `json:"integrity"`
	}
	if err := json.Unmarshal(b1, &decoded); err != nil {
		t.Fatalf("unmarshal canonical: %v", err)
	}
	if decoded.Integrity.Digest != "" {
		t.Fatalf("canonical marshal must blank integrity.digest, got %q", decoded.Integrity.Digest)
	}
}

func TestComputeDigestIsLowercaseHexAndMatches(t *testing.T) {
	c, err := Compile(newFakePack(), newFakePack().descriptor, readyIncident(), testEvents(),
		testSource(), testTrigger(), "cap-8271", "graph-8271", "2026-08-29T10:33:00Z")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	digest, err := ComputeDigest(c)
	if err != nil {
		t.Fatalf("compute digest: %v", err)
	}
	if !lowerHex64.MatchString(digest) {
		t.Fatalf("digest must be 64 lowercase hex chars, got %q", digest)
	}
	if digest != c.Integrity.Digest {
		t.Fatalf("compiled digest %q != computed %q", c.Integrity.Digest, digest)
	}
}

func TestVerifyDigest(t *testing.T) {
	c, err := Compile(newFakePack(), newFakePack().descriptor, readyIncident(), testEvents(),
		testSource(), testTrigger(), "cap-8271", "graph-8271", "2026-08-29T10:33:00Z")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !VerifyDigest(c) {
		t.Fatal("valid capsule digest must verify")
	}
	changed := c
	changed.EventIDs = []string{"evt-timeout", "evt-retry", "evt-extra"}
	if VerifyDigest(changed) {
		t.Fatal("digest must fail when event_ids change")
	}
}

func TestCompileRejectsNotReadyIncident(t *testing.T) {
	incident := readyIncident()
	incident.Status = contracts.IncidentDetected
	_, err := Compile(newFakePack(), newFakePack().descriptor, incident, testEvents(),
		testSource(), testTrigger(), "cap-8271", "graph-8271", "2026-08-29T10:33:00Z")
	if !errors.Is(err, ErrNotReady) {
		t.Fatalf("non-READY incident error = %v, want ErrNotReady", err)
	}
}

func TestCompileProducesValidCapsule(t *testing.T) {
	pack := newFakePack()
	c, err := Compile(pack, pack.descriptor, readyIncident(), testEvents(),
		testSource(), testTrigger(), "cap-8271", "graph-8271", "2026-08-29T10:33:00Z")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("compiled capsule fails contract validation: %v", err)
	}
	if c.SchemaVersion != contracts.ContractVersion || c.CapsuleID != "cap-8271" || c.GraphID != "graph-8271" {
		t.Fatalf("compiled identity fields wrong: %+v", c)
	}
	if len(c.EventIDs) != 2 || c.EventIDs[0] != "evt-timeout" || c.EventIDs[1] != "evt-retry" {
		t.Fatalf("event_ids not derived from events: %v", c.EventIDs)
	}
	if !lowerHex64.MatchString(c.Integrity.Digest) {
		t.Fatalf("integrity digest invalid: %q", c.Integrity.Digest)
	}
}

func TestCompileRejectsPackValidationIssue(t *testing.T) {
	pack := newFakePack().withIssues(contracts.ValidationIssue{
		Code: contracts.SchemaInvalid, Path: "/safety", Message: "unsafe policy",
	})
	_, err := Compile(pack, newFakePack().descriptor, readyIncident(), testEvents(),
		testSource(), testTrigger(), "cap-8271", "graph-8271", "2026-08-29T10:33:00Z")
	if !errors.Is(err, ErrPackValidation) {
		t.Fatalf("pack validation issue error = %v, want ErrPackValidation", err)
	}
}

func TestCompileRejectsPlanThatBreaksContract(t *testing.T) {
	pack := newFakePack()
	pack.plan.RequiredComponents = []string{"gateway"}
	_, err := Compile(pack, pack.descriptor, readyIncident(), testEvents(),
		testSource(), testTrigger(), "cap-8271", "graph-8271", "2026-08-29T10:33:00Z")
	if !errors.Is(err, ErrInvalidCapsule) {
		t.Fatalf("contract-invalid plan error = %v, want ErrInvalidCapsule", err)
	}
}

func TestSafetyPolicyDefaultDeny(t *testing.T) {
	policy := SafePolicy()
	if policy.PolicyVersion != contracts.ContractVersion || policy.SanitizationStatus != contracts.SanitizationPass || policy.CredentialProfile != contracts.CredentialReplayOnly {
		t.Fatalf("SafePolicy wrong literals: %+v", policy)
	}
	if len(policy.BlockedDestinations) == 0 {
		t.Fatal("SafePolicy must have non-empty blocked_destinations")
	}
	if !contains(policy.BlockedDestinations, "production-databases") || !contains(policy.BlockedDestinations, "public-internet") || !contains(policy.BlockedDestinations, "real-payment-provider") {
		t.Fatalf("SafePolicy must block production/public/real-payment destinations, got %v", policy.BlockedDestinations)
	}
	for _, destination := range policy.AllowedDestinations {
		if destination != "payment-simulator" && destination != "replay-postgres" {
			t.Fatalf("SafePolicy allowed a non-replay destination %q", destination)
		}
	}
	// The contract gate rejects a non-replay allowed destination and an empty
	// blocked list, proving default-deny cannot be weakened by a pack.
	pack := newFakePack()
	c, err := Compile(pack, pack.descriptor, readyIncident(), testEvents(),
		testSource(), testTrigger(), "cap-8271", "graph-8271", "2026-08-29T10:33:00Z")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	unsafe := c
	unsafe.Safety.AllowedDestinations = []string{"payment-simulator", "production-database"}
	if err := unsafe.Validate(); err == nil {
		t.Fatal("an unsafe allowed destination must fail default-deny validation")
	}
	empty := c
	empty.Safety.BlockedDestinations = nil
	if err := empty.Validate(); err == nil {
		t.Fatal("empty blocked destinations must fail default-deny validation")
	}
}

func testSource() contracts.CapsuleSource {
	return contracts.CapsuleSource{
		IncidentID:         "inc-8271",
		TraceID:            "trace-8271",
		ExecutionID:        "exec-original-8271",
		CaptureEnvironment: contracts.CaptureDemo,
		CapturedAt:         "2026-08-29T10:32:01.561Z",
	}
}

func testTrigger() contracts.Trigger {
	return contracts.Trigger{
		RequestOrMessage: map[string]any{"method": "POST", "path": "/checkout"},
		SanitizedHeaders: map[string]string{"content-type": "application/json"},
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
