// Package golden holds the checkout_duplicate_effect scenario's checked-in
// sanitized fixture content: the replay-only empty ledger state, a
// sanitized checkout request (no real customer/order/payment identity),
// and the payment simulator's golden baseline response and delay
// (docs/SYSTEM_PACKS.md "Fixtures"). internal/systempack/checkout embeds
// these as the single source of truth for the state/dependency fixtures it
// hands to the Replay Capsule compiler.
package golden

import _ "embed"

// LedgerEmptyStateContent is the sanitized, resettable empty ledger state
// fixture content. Its SHA-256 digest is what
// internal/systempack/checkout.ExtractFixtures reports as the
// state-ledger-empty fixture's content_digest.
//
//go:embed ledger_empty_state.json
var LedgerEmptyStateContent []byte

// CheckoutRequestContent is the sanitized checkout request used as the
// Replay Capsule trigger's request_or_message content for the golden
// scenario (docs/CONTRACTS.md ReplayCapsule P0 example).
//
//go:embed checkout_request.json
var CheckoutRequestContent []byte

// PaymentResponseContent is the payment simulator's golden baseline
// dependency fixture content (350 ms latency, APPROVED response, 2
// invocations).
//
//go:embed payment_response.json
var PaymentResponseContent []byte
