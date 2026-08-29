package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/causalens/causalens/internal/capture"
)

// commitWireRequest/commitWireResponse are the demo Ledger's internal HTTP
// wire shapes. They are not part of the frozen Core API contract in
// CONTRACTS.md.
type commitWireRequest struct {
	CheckoutID string `json:"checkout_id"`
	Attempt    int    `json:"attempt"`
}

type commitWireResponse struct {
	EffectID  string `json:"effect_id"`
	Committed bool   `json:"committed"`
}

// Handler exposes Service over HTTP as POST /effects.
func Handler(svc *Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/effects", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var wire commitWireRequest
		if err := json.NewDecoder(r.Body).Decode(&wire); err != nil {
			http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
			return
		}
		req := CommitRequest{
			ExecutionID:        r.Header.Get(capture.HeaderExecutionID),
			TraceID:            r.Header.Get(capture.HeaderTraceID),
			LogicalOperationID: r.Header.Get(capture.HeaderLogicalOperationID),
			ParentEventID:      r.Header.Get(capture.HeaderParentEventID),
			CheckoutID:         wire.CheckoutID,
			Attempt:            wire.Attempt,
		}
		result, err := svc.Commit(r.Context(), req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(commitWireResponse{EffectID: result.EffectID, Committed: result.Committed})
	})
	return mux
}

// Client is an HTTP-backed ledger client for consumers such as the demo
// Checkout service.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// NewClient returns a Client pointed at baseURL (e.g. http://demo-ledger:8083).
func NewClient(baseURL string) *Client { return &Client{BaseURL: baseURL, HTTP: http.DefaultClient} }

// Commit calls the demo Ledger's POST /effects endpoint.
func (c *Client) Commit(ctx context.Context, req CommitRequest) (CommitResult, error) {
	body, err := json.Marshal(commitWireRequest{CheckoutID: req.CheckoutID, Attempt: req.Attempt})
	if err != nil {
		return CommitResult{}, fmt.Errorf("ledger client: marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/effects", bytes.NewReader(body))
	if err != nil {
		return CommitResult{}, fmt.Errorf("ledger client: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(capture.HeaderExecutionID, req.ExecutionID)
	httpReq.Header.Set(capture.HeaderTraceID, req.TraceID)
	httpReq.Header.Set(capture.HeaderLogicalOperationID, req.LogicalOperationID)
	httpReq.Header.Set(capture.HeaderParentEventID, req.ParentEventID)

	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return CommitResult{}, fmt.Errorf("ledger client: request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return CommitResult{}, fmt.Errorf("ledger client: unexpected status %d", resp.StatusCode)
	}
	var wire commitWireResponse
	if err := json.NewDecoder(resp.Body).Decode(&wire); err != nil {
		return CommitResult{}, fmt.Errorf("ledger client: decode response: %w", err)
	}
	return CommitResult{EffectID: wire.EffectID, Committed: wire.Committed}, nil
}
