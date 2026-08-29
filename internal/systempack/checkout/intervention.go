package checkout

import (
	"context"
	"fmt"

	"github.com/causalens/causalens/internal/contracts"
)

// paymentLatencySpec is the sole P0 intervention the pack accepts, matching
// docs/CONTRACTS.md's frozen InterventionSpec example exactly.
func paymentLatencySpec() contracts.InterventionSpec {
	return contracts.InterventionSpec{
		Type:      contracts.InterventionPaymentLatency,
		ValueType: contracts.InterventionValueInteger,
		Unit:      contracts.InterventionUnitMilliseconds,
		Minimum:   0,
		Maximum:   5000,
	}
}

// AllowedInterventions implements contracts.SystemPack: exactly the single
// PAYMENT_LATENCY intervention the golden scenario supports for P0.
func (p *Pack) AllowedInterventions() []contracts.InterventionSpec {
	return []contracts.InterventionSpec{paymentLatencySpec()}
}

// ApplyIntervention implements contracts.SystemPack: it validates the
// intervention against AllowedInterventions and returns a deep copy of plan
// -- the caller (replay runtime) applies the intervention's effective value
// to a fresh payment dependency fixture for the run; ApplyIntervention never
// mutates the capsule-derived plan it was given.
func (p *Pack) ApplyIntervention(_ context.Context, plan contracts.ReplayPlan, intervention contracts.Intervention) (contracts.ReplayPlan, error) {
	spec := paymentLatencySpec()
	if intervention.Type != spec.Type {
		return contracts.ReplayPlan{}, fmt.Errorf("checkout: unsupported intervention type %q", intervention.Type)
	}
	if intervention.Unit != spec.Unit {
		return contracts.ReplayPlan{}, fmt.Errorf("checkout: unsupported intervention unit %q", intervention.Unit)
	}
	if intervention.To < spec.Minimum || intervention.To > spec.Maximum {
		return contracts.ReplayPlan{}, fmt.Errorf("checkout: intervention value %d out of range [%d,%d]", intervention.To, spec.Minimum, spec.Maximum)
	}
	if intervention.From == intervention.To {
		return contracts.ReplayPlan{}, fmt.Errorf("checkout: intervention from and to must differ")
	}

	copied := plan
	copied.RequiredComponents = append([]string(nil), plan.RequiredComponents...)
	copied.FixtureLoadOrder = append([]string(nil), plan.FixtureLoadOrder...)
	return copied, nil
}
