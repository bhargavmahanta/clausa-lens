package service

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// checkoutWireRequest/checkoutWireResponse are the demo Gateway's public
// HTTP wire shapes for POST /checkout. checkout_id is optional; amount and
// currency are accepted for shape-fidelity with docs/CONTRACTS.md's
// ReplayCapsule trigger example but are not used by the golden scenario
// logic.
type checkoutWireRequest struct {
	CheckoutID  string `json:"checkout_id,omitempty"`
	AmountMinor int    `json:"amount_minor,omitempty"`
	Currency    string `json:"currency,omitempty"`
	Scenario    string `json:"scenario,omitempty"`
}

type checkoutWireResponse struct {
	TraceID            string `json:"trace_id"`
	ExecutionID        string `json:"execution_id"`
	LogicalOperationID string `json:"logical_operation_id"`
	Attempts           int    `json:"attempts"`
}

// Handler exposes Service over HTTP as POST /checkout, the golden
// scenario's declared entrypoint (docs/CONTRACTS.md ReplayPlan.entrypoint
// "gateway.checkout").
func Handler(svc *Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/checkout", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var wire checkoutWireRequest
		if r.ContentLength != 0 {
			if err := json.NewDecoder(r.Body).Decode(&wire); err != nil {
				http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
				return
			}
		}
		result, err := svc.Checkout(r.Context(), Request{CheckoutID: wire.CheckoutID, Scenario: wire.Scenario})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(checkoutWireResponse{
			TraceID:            result.TraceID,
			ExecutionID:        result.ExecutionID,
			LogicalOperationID: result.LogicalOperationID,
			Attempts:           result.Attempts,
		})
	})
	return mux
}
