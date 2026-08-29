// Package checkout implements the checkout_duplicate_effect System Pack:
// the frozen contracts.SystemPack interface for the golden
// Gateway-Checkout-Payment-Ledger scenario (docs/SYSTEM_PACKS.md,
// docs/CONTRACTS.md #SystemPack v1.0). Pack implements the interface;
// internal/contracts itself is never modified here.
package checkout

import (
	"github.com/causalens/causalens/internal/contracts"
	"github.com/causalens/causalens/internal/differential"
)

// PackID and PackVersion match the frozen P0 SystemPackDescriptor example
// in docs/CONTRACTS.md.
const (
	PackID      = "checkout_duplicate_effect"
	PackVersion = "1.0.0"
)

// Pack implements contracts.SystemPack for the checkout_duplicate_effect
// scenario. It holds no mutable state; every method is deterministic for
// the same validated input, per the frozen interface contract.
type Pack struct{}

// New returns the checkout_duplicate_effect System Pack.
func New() *Pack { return &Pack{} }

// Descriptor implements contracts.SystemPack.
func (p *Pack) Descriptor() contracts.SystemPackRef {
	return contracts.SystemPackRef{ID: PackID, Version: PackVersion, InterfaceVersion: contracts.ContractVersion}
}

// Labels implements contracts.SystemPack: stable judge-facing names for the
// golden scenario's components, operations, event types, effects, and
// interventions.
func (p *Pack) Labels() contracts.LabelSet {
	return contracts.LabelSet{
		Components: map[string]string{
			"gateway":  "Gateway",
			"checkout": "Checkout",
			"payment":  "Payment Simulator",
			"ledger":   "Ledger",
		},
		Operations: map[string]string{
			"checkout":         "Checkout Request",
			"checkout.process": "Checkout Processing",
			"authorize":        "Payment Authorization",
			"ledger.commit":    "Ledger Commit",
		},
		EventTypes: map[contracts.EventType]string{
			contracts.EventStart:            "Started",
			contracts.EventComplete:         "Completed",
			contracts.EventError:            "Errored",
			contracts.EventTimeout:          "Timed Out",
			contracts.EventRetry:            "Retried",
			contracts.EventEffect:           "Effect Recorded",
			contracts.EventStateObservation: "State Observed",
		},
		Effects: map[string]string{
			"ledger.commit": "Ledger Effect",
		},
		Interventions: map[string]string{
			string(contracts.InterventionPaymentLatency): "Payment Latency",
		},
	}
}

// Compile-time proof that Pack satisfies both the frozen contracts.SystemPack
// interface and Bhargav's differential.Alignment seam (the duck-typed extra
// method internal/differential's diff route needs -- see Align in
// compare.go). internal/contracts and internal/differential are never
// modified here; Pack only implements what they declare.
var (
	_ contracts.SystemPack   = (*Pack)(nil)
	_ differential.Alignment = (*Pack)(nil)
)
