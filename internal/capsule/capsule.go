package capsule

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/causalens/causalens/internal/contracts"
)

const (
	canonicalSchemaVersion = contracts.ContractVersion
	defaultClockTolerance  = 5
	defaultTimeout         = 200
)

var (
	// ErrNotReady reports that Compile received an incident that is not READY.
	ErrNotReady = errors.New("capsule: incident is not READY")
	// ErrInvalidCapsule reports that the assembled capsule violates the frozen
	// ReplayCapsule contract.
	ErrInvalidCapsule = errors.New("capsule: assembled capsule is invalid")
	// ErrPackValidation reports that the System Pack rejected the assembled capsule.
	ErrPackValidation = errors.New("capsule: system pack validation failed")
	// ErrIntegrityMismatch reports that the computed digest does not match content.
	ErrIntegrityMismatch = errors.New("capsule: integrity mismatch")
)

// CanonicalMarshal returns deterministic JSON of the capsule with
// integrity.digest blanked, object keys lexicographically ordered, arrays in
// contract order (this is what encoding/json gives for a struct + string-map).
func CanonicalMarshal(c contracts.ReplayCapsule) ([]byte, error) {
	c.Integrity.Digest = ""
	return json.Marshal(c)
}

// ComputeDigest returns the lowercase hex SHA-256 over CanonicalMarshal.
func ComputeDigest(c contracts.ReplayCapsule) (string, error) {
	canonical, err := CanonicalMarshal(c)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

// VerifyDigest reports whether capsule integrity matches its canonical content.
func VerifyDigest(c contracts.ReplayCapsule) bool {
	digest, err := ComputeDigest(c)
	if err != nil {
		return false
	}
	return digest == c.Integrity.Digest
}

// SafePolicy returns the default-deny replay-only safety policy for the capsule.
func SafePolicy() contracts.SafetyPolicy {
	return contracts.SafetyPolicy{
		PolicyVersion:       contracts.ContractVersion,
		SanitizationStatus:  contracts.SanitizationPass,
		BlockedDestinations: []string{"production-databases", "public-internet", "real-payment-provider"},
		AllowedDestinations: []string{"payment-simulator", "replay-postgres"},
		CredentialProfile:   contracts.CredentialReplayOnly,
	}
}

// Compile assembles a validated Ready capsule from a READY incident, its
// resolved events, and the active System Pack. It returns a fully-populated
// ReplayCapsule whose integrity.digest is computed over all fields except the
// digest itself.
func Compile(pack contracts.SystemPack, packRef contracts.SystemPackRef,
	incident contracts.Incident, events []contracts.ExecutionEvent,
	source contracts.CapsuleSource, trigger contracts.Trigger,
	capsuleID, graphID string, createdAt string) (contracts.ReplayCapsule, error) {

	if incident.Status != contracts.IncidentReady {
		return contracts.ReplayCapsule{}, fmt.Errorf("%w: incident %s has status %s", ErrNotReady, incident.IncidentID, incident.Status)
	}

	ctx := context.Background()

	fixtures, err := pack.ExtractFixtures(ctx, incident, events)
	if err != nil {
		return contracts.ReplayCapsule{}, fmt.Errorf("extract fixtures: %w", err)
	}
	plan, err := pack.BuildReplayPlan(ctx, incident, fixtures)
	if err != nil {
		return contracts.ReplayCapsule{}, fmt.Errorf("build replay plan: %w", err)
	}
	interventions := pack.AllowedInterventions()

	eventIDs := make([]string, 0, len(events))
	for _, event := range events {
		eventIDs = append(eventIDs, event.EventID)
	}

	capsule := contracts.ReplayCapsule{
		SchemaVersion:      canonicalSchemaVersion,
		CapsuleID:          capsuleID,
		CreatedAt:          createdAt,
		Source:             source,
		SystemPack:         packRef,
		Trigger:            trigger,
		EventIDs:           eventIDs,
		GraphID:            graphID,
		StateFixtures:      fixtures.StateFixtures,
		DependencyFixtures: fixtures.DependencyFixtures,
		TimingPolicy:       contracts.TimingPolicy{ClockToleranceMS: defaultClockTolerance, TimeoutMS: defaultTimeout},
		ReplayPlan:         plan,
		FailureOracle: contracts.FailureOracleSpec{
			ID:                    incident.FailureOracle.ID,
			Version:               incident.FailureOracle.Version,
			ExpectedMatch:         true,
			ExpectedEffectSummary: contracts.EffectSummary{PaymentAttemptCount: 2, LedgerCommitCount: 2},
		},
		AllowedInterventions: interventions,
		Safety:               SafePolicy(),
	}

	capsule.Integrity.Algorithm = contracts.IntegritySHA256
	digest, err := ComputeDigest(capsule)
	if err != nil {
		return contracts.ReplayCapsule{}, fmt.Errorf("compute digest: %w", err)
	}
	capsule.Integrity.Digest = digest

	if err := capsule.Validate(); err != nil {
		return contracts.ReplayCapsule{}, fmt.Errorf("%w: %v", ErrInvalidCapsule, err)
	}
	if issues := pack.ValidateCapsule(ctx, capsule); len(issues) > 0 {
		return contracts.ReplayCapsule{}, fmt.Errorf("%w: %v", ErrPackValidation, issues)
	}
	if !VerifyDigest(capsule) {
		return contracts.ReplayCapsule{}, fmt.Errorf("%w: computed digest does not match content", ErrIntegrityMismatch)
	}
	return capsule, nil
}
