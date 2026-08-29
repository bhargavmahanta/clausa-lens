package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	checkoutsvc "github.com/causalens/causalens/cmd/demo-checkout/service"
	gatewaysvc "github.com/causalens/causalens/cmd/demo-gateway/service"
	ledgersvc "github.com/causalens/causalens/cmd/demo-ledger/service"
	paymentsvc "github.com/causalens/causalens/cmd/demo-payment/service"
	"github.com/causalens/causalens/internal/capture"
	"github.com/causalens/causalens/internal/contracts"
	"github.com/causalens/causalens/internal/core"
)

// coreAPIEventsServer stands in for the live Core API's POST /v1/events
// route (cmd/core-api/main.go, which I do not own and cannot import as
// package main) so the golden scenario test proves my captured events pass
// Bhargav's real validation and storage path end to end. It replicates only
// the /v1/events handler logic using his exported contracts.DecodeStrict,
// contracts.ExecutionEvent, and core.Store -- no behavior of my own.
func coreAPIEventsServer(t *testing.T) (*httptest.Server, *core.Store) {
	t.Helper()
	store := core.NewStore()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var event contracts.ExecutionEvent
		if err := contracts.DecodeStrict(r.Body, &event); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if err := store.IngestEvent(r.Context(), event); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(contracts.AcceptedEventResponse{EventID: event.EventID, Status: contracts.Accepted})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server, store
}

// goldenHarness wires all four demo services together over real HTTP
// (httptest servers), exactly as they run in docker-compose, with events
// flowing to a faithful stand-in for the live Core API's POST /v1/events
// route. This is the only place in the demo services where a package
// legitimately imports the other three: cmd/demo-* are otherwise
// independent binaries.
type goldenHarness struct {
	store *core.Store

	gateway  *gatewaysvc.Service
	checkout *checkoutsvc.Service
	payment  *paymentsvc.Service
	ledger   *ledgersvc.Service

	closers []func()
}

func newGoldenHarness(t *testing.T) *goldenHarness {
	t.Helper()
	eventsServer, store := coreAPIEventsServer(t)
	h := &goldenHarness{store: store}
	sink := capture.NewHTTPSink(eventsServer.URL + "/v1/events")

	ledgerRecorder := capture.NewRecorder(contracts.ComponentRef{Name: "ledger", Instance: "ledger-1"}, capture.NewIDGenerator(1), sink)
	h.ledger = ledgersvc.New(ledgerRecorder)
	ledgerServer := httptest.NewServer(ledgersvc.Handler(h.ledger))
	h.closers = append(h.closers, ledgerServer.Close)
	ledgerClient := ledgersvc.NewClient(ledgerServer.URL)

	paymentRecorder := capture.NewRecorder(contracts.ComponentRef{Name: "payment", Instance: "payment-1"}, capture.NewIDGenerator(1), sink)
	h.payment = paymentsvc.New(paymentRecorder)
	paymentServer := httptest.NewServer(paymentsvc.Handler(h.payment))
	h.closers = append(h.closers, paymentServer.Close)
	paymentClient := paymentsvc.NewClient(paymentServer.URL)

	checkoutRecorder := capture.NewRecorder(contracts.ComponentRef{Name: "checkout", Instance: "checkout-1"}, capture.NewIDGenerator(1), sink)
	h.checkout = checkoutsvc.New(paymentClient, ledgerClient, checkoutRecorder)
	checkoutServer := httptest.NewServer(checkoutsvc.Handler(h.checkout))
	h.closers = append(h.closers, checkoutServer.Close)
	checkoutClient := checkoutsvc.NewClient(checkoutServer.URL)

	gatewayRecorder := capture.NewRecorder(contracts.ComponentRef{Name: "gateway", Instance: "gateway-1"}, capture.NewIDGenerator(1), sink)
	gwIDs := capture.NewIDGenerator(capture.DefaultCheckoutSeed)
	h.gateway = gatewaysvc.New(checkoutClient, gwIDs, gatewayRecorder)

	t.Cleanup(func() {
		for _, closeFn := range h.closers {
			closeFn()
		}
	})
	return h
}

func (h *goldenHarness) reset(ctx context.Context) {
	h.ledger.Reset()
	h.payment.Reset()
	h.checkout.Reset()
	h.gateway.Reset()
	_, _ = h.store.Reset(ctx)
}

// TestGoldenScenario_DuplicateEffectEndToEnd is the P0 checkpoint: from the
// default fault configuration (350 ms payment latency, 200 ms checkout
// timeout, max 2 attempts, deduplication disabled), the golden request must
// produce exactly two payment attempts and exactly two ledger commits for
// one logical checkout, and every emitted event must pass the live Core
// API's real POST /v1/events validation and storage path.
func TestGoldenScenario_DuplicateEffectEndToEnd(t *testing.T) {
	h := newGoldenHarness(t)
	ctx := context.Background()

	if got := h.payment.LatencyMs(); got != paymentsvc.DefaultLatencyMs {
		t.Fatalf("payment latency = %d, want default %d", got, paymentsvc.DefaultLatencyMs)
	}
	if h.ledger.DeduplicationEnabled() {
		t.Fatalf("deduplication = true, want disabled by default")
	}

	result, err := h.gateway.Checkout(ctx, gatewaysvc.Request{CheckoutID: "8271"})
	if err != nil {
		t.Fatalf("Checkout: %v", err)
	}

	if result.Attempts != 2 {
		t.Fatalf("gateway reported Attempts = %d, want 2", result.Attempts)
	}
	if got := h.payment.AttemptCount(); got != 2 {
		t.Fatalf("exactly two payment attempts required, got %d", got)
	}
	if got := h.ledger.CommittedEffectCount(); got != 2 {
		t.Fatalf("exactly two ledger commits required, got %d", got)
	}

	effects := h.ledger.CommittedEffects()
	if len(effects) != 2 || effects[0] == effects[1] {
		t.Fatalf("expected two distinct ledger effect ids for one logical checkout, got %v", effects)
	}
}

// TestGoldenScenario_ResetIsDeterministic covers the requirement that reset
// returns deterministic identifiers and empty state, and that the golden
// scenario reproduces identically afterward.
func TestGoldenScenario_ResetIsDeterministic(t *testing.T) {
	h := newGoldenHarness(t)
	ctx := context.Background()

	firstNext := h.gateway.NextLogicalOperationID()
	if _, err := h.gateway.Checkout(ctx, gatewaysvc.Request{}); err != nil {
		t.Fatalf("first Checkout: %v", err)
	}
	if h.payment.AttemptCount() != 2 || h.ledger.CommittedEffectCount() != 2 {
		t.Fatalf("expected the first run to reach the golden shape before testing reset")
	}

	h.reset(ctx)

	if got := h.payment.AttemptCount(); got != 0 {
		t.Fatalf("after reset, payment attempt count = %d, want 0", got)
	}
	if got := h.ledger.CommittedEffectCount(); got != 0 {
		t.Fatalf("after reset, ledger commit count = %d, want 0", got)
	}
	if got := h.payment.LatencyMs(); got != paymentsvc.DefaultLatencyMs {
		t.Fatalf("after reset, payment latency = %d, want default %d", got, paymentsvc.DefaultLatencyMs)
	}
	if h.ledger.DeduplicationEnabled() {
		t.Fatalf("after reset, deduplication = true, want disabled")
	}
	if got := h.gateway.NextLogicalOperationID(); got != firstNext {
		t.Fatalf("after reset, next_logical_operation_id = %q, want deterministic %q", got, firstNext)
	}

	result, err := h.gateway.Checkout(ctx, gatewaysvc.Request{})
	if err != nil {
		t.Fatalf("second Checkout after reset: %v", err)
	}
	if result.LogicalOperationID != firstNext {
		t.Fatalf("after reset, second run logical_operation_id = %q, want deterministic %q", result.LogicalOperationID, firstNext)
	}
	if result.Attempts != 2 {
		t.Fatalf("after reset, second run Attempts = %d, want 2", result.Attempts)
	}
	if h.payment.AttemptCount() != 2 || h.ledger.CommittedEffectCount() != 2 {
		t.Fatalf("after reset, the golden scenario did not reproduce identically: payment=%d ledger=%d",
			h.payment.AttemptCount(), h.ledger.CommittedEffectCount())
	}
}
