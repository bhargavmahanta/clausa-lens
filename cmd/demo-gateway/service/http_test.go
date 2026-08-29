package service

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	checkoutsvc "github.com/causalens/causalens/cmd/demo-checkout/service"
	"github.com/causalens/causalens/internal/capture"
	"github.com/causalens/causalens/internal/contracts"
)

// TestHandler_Checkout_NormalizesContractCheckoutID sends the frozen
// contract's ReplayCapsule trigger body (docs/CONTRACTS.md P0 example) and
// verifies the HTTP response carries canonical identifiers, never a
// "checkout-checkout-" double prefix.
func TestHandler_Checkout_NormalizesContractCheckoutID(t *testing.T) {
	sink := capture.NewInMemorySink()
	recorder := capture.NewRecorder(contracts.ComponentRef{Name: "gateway", Instance: "gateway-1"}, capture.NewIDGenerator(1), sink)
	ids := capture.NewIDGenerator(capture.DefaultCheckoutSeed)
	checkout := &stubCheckout{result: checkoutsvc.Result{Attempts: 2}}
	svc := New(checkout, ids, recorder)

	body := []byte(`{"checkout_id": "checkout-8271", "amount_minor": 4999, "currency": "INR"}`)
	req := httptest.NewRequest("POST", "/checkout", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	Handler(svc).ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var got checkoutWireResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v; body: %s", err, w.Body.String())
	}

	want := checkoutWireResponse{
		TraceID:            "trace-8271",
		ExecutionID:        "exec-original-8271",
		LogicalOperationID: "checkout-8271",
		Attempts:           2,
	}
	if got != want {
		t.Fatalf("response = %+v, want %+v", got, want)
	}

	for _, ev := range sink.Events() {
		if ev.LogicalOperationID != "checkout-8271" {
			t.Fatalf("captured event logical_operation_id = %q, want checkout-8271 (never checkout-checkout-8271)", ev.LogicalOperationID)
		}
	}
}
