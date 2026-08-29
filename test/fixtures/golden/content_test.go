package golden

import (
	"encoding/json"
	"testing"
)

func TestLedgerEmptyStateContent_IsValidAndEmpty(t *testing.T) {
	var doc struct {
		Table string        `json:"table"`
		Rows  []interface{} `json:"rows"`
	}
	if err := json.Unmarshal(LedgerEmptyStateContent, &doc); err != nil {
		t.Fatalf("LedgerEmptyStateContent is not valid JSON: %v", err)
	}
	if doc.Table != "ledger_effects" {
		t.Fatalf("table = %q, want ledger_effects", doc.Table)
	}
	if len(doc.Rows) != 0 {
		t.Fatalf("expected an empty, resettable ledger state, got %d rows", len(doc.Rows))
	}
}

func TestCheckoutRequestContent_IsSanitized(t *testing.T) {
	var doc struct {
		Method string `json:"method"`
		Path   string `json:"path"`
		Body   struct {
			CheckoutID  string `json:"checkout_id"`
			AmountMinor int    `json:"amount_minor"`
			Currency    string `json:"currency"`
		} `json:"body"`
	}
	if err := json.Unmarshal(CheckoutRequestContent, &doc); err != nil {
		t.Fatalf("CheckoutRequestContent is not valid JSON: %v", err)
	}
	if doc.Method != "POST" || doc.Path != "/checkout" {
		t.Fatalf("unexpected request shape: %+v", doc)
	}
	if doc.Body.CheckoutID == "" || doc.Body.Currency == "" {
		t.Fatalf("expected a sanitized, replay-only checkout_id and currency, got %+v", doc.Body)
	}

	// Sanitized means no real customer/order/payment identity fields.
	forbidden := []string{"email", "customer_id", "card_number", "name", "phone"}
	raw := map[string]any{}
	if err := json.Unmarshal(CheckoutRequestContent, &raw); err != nil {
		t.Fatalf("re-decode as map: %v", err)
	}
	for _, key := range forbidden {
		if _, ok := raw[key]; ok {
			t.Fatalf("fixture is not sanitized: found forbidden key %q", key)
		}
	}
}

func TestPaymentResponseContent_MatchesGoldenBaseline(t *testing.T) {
	var doc struct {
		LatencyMS       int    `json:"latency_ms"`
		FailureMode     string `json:"failure_mode"`
		InvocationLimit int    `json:"invocation_limit"`
		Response        struct {
			Status string `json:"status"`
		} `json:"response"`
	}
	if err := json.Unmarshal(PaymentResponseContent, &doc); err != nil {
		t.Fatalf("PaymentResponseContent is not valid JSON: %v", err)
	}
	if doc.LatencyMS != 350 {
		t.Fatalf("latency_ms = %d, want 350 (P0 fixed value)", doc.LatencyMS)
	}
	if doc.InvocationLimit != 2 {
		t.Fatalf("invocation_limit = %d, want 2 (P0 max attempts)", doc.InvocationLimit)
	}
	if doc.Response.Status != "APPROVED" {
		t.Fatalf("response.status = %q, want APPROVED", doc.Response.Status)
	}
}
