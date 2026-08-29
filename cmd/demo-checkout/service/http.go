package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/causalens/causalens/internal/capture"
)

// processWireRequest/processWireResponse are the demo Checkout's internal
// HTTP wire shapes. They are not part of the frozen Core API contract in
// CONTRACTS.md.
type processWireRequest struct {
	CheckoutID string `json:"checkout_id"`
}

type processWireResponse struct {
	Attempts int `json:"attempts"`
}

// Handler exposes Service over HTTP as POST /checkout.
func Handler(svc *Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/checkout", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var wire processWireRequest
		if err := json.NewDecoder(r.Body).Decode(&wire); err != nil {
			http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
			return
		}
		req := Request{
			ExecutionID:        r.Header.Get(capture.HeaderExecutionID),
			TraceID:            r.Header.Get(capture.HeaderTraceID),
			LogicalOperationID: r.Header.Get(capture.HeaderLogicalOperationID),
			ParentEventID:      r.Header.Get(capture.HeaderParentEventID),
			CheckoutID:         wire.CheckoutID,
		}
		result, err := svc.Process(r.Context(), req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(processWireResponse{Attempts: result.Attempts})
	})
	return mux
}

// Client is an HTTP-backed checkout client for consumers such as the demo
// Gateway service.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// NewClient returns a Client pointed at baseURL (e.g. http://demo-checkout:8081).
func NewClient(baseURL string) *Client { return &Client{BaseURL: baseURL, HTTP: http.DefaultClient} }

// Process calls the demo Checkout's POST /checkout endpoint.
func (c *Client) Process(ctx context.Context, req Request) (Result, error) {
	body, err := json.Marshal(processWireRequest{CheckoutID: req.CheckoutID})
	if err != nil {
		return Result{}, fmt.Errorf("checkout client: marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/checkout", bytes.NewReader(body))
	if err != nil {
		return Result{}, fmt.Errorf("checkout client: build request: %w", err)
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
		return Result{}, fmt.Errorf("checkout client: request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("checkout client: unexpected status %d", resp.StatusCode)
	}
	var wire processWireResponse
	if err := json.NewDecoder(resp.Body).Decode(&wire); err != nil {
		return Result{}, fmt.Errorf("checkout client: decode response: %w", err)
	}
	return Result{Attempts: wire.Attempts}, nil
}
