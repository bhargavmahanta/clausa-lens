package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/causalens/causalens/internal/capture"
)

// authorizeWireRequest/authorizeWireResponse are the demo Payment
// simulator's internal HTTP wire shapes. They are not part of the frozen
// Core API contract in CONTRACTS.md.
type authorizeWireRequest struct {
	Attempt int `json:"attempt"`
}

type authorizeWireResponse struct {
	Status string `json:"status"`
}

// Handler exposes Service over HTTP as POST /authorize.
func Handler(svc *Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var wire authorizeWireRequest
		if err := json.NewDecoder(r.Body).Decode(&wire); err != nil {
			http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
			return
		}
		req := AuthorizeRequest{
			ExecutionID:        r.Header.Get(capture.HeaderExecutionID),
			TraceID:            r.Header.Get(capture.HeaderTraceID),
			LogicalOperationID: r.Header.Get(capture.HeaderLogicalOperationID),
			ParentEventID:      r.Header.Get(capture.HeaderParentEventID),
			Attempt:            wire.Attempt,
		}
		result, err := svc.Authorize(r.Context(), req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(authorizeWireResponse{Status: result.Status})
	})
	return mux
}

// Client is an HTTP-backed payment client for consumers such as the demo
// Checkout service.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// NewClient returns a Client pointed at baseURL (e.g. http://demo-payment:8082).
func NewClient(baseURL string) *Client { return &Client{BaseURL: baseURL, HTTP: http.DefaultClient} }

// Authorize calls the demo Payment simulator's POST /authorize endpoint.
func (c *Client) Authorize(ctx context.Context, req AuthorizeRequest) (AuthorizeResult, error) {
	body, err := json.Marshal(authorizeWireRequest{Attempt: req.Attempt})
	if err != nil {
		return AuthorizeResult{}, fmt.Errorf("payment client: marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/authorize", bytes.NewReader(body))
	if err != nil {
		return AuthorizeResult{}, fmt.Errorf("payment client: build request: %w", err)
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
		return AuthorizeResult{}, fmt.Errorf("payment client: request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return AuthorizeResult{}, fmt.Errorf("payment client: unexpected status %d", resp.StatusCode)
	}
	var wire authorizeWireResponse
	if err := json.NewDecoder(resp.Body).Decode(&wire); err != nil {
		return AuthorizeResult{}, fmt.Errorf("payment client: decode response: %w", err)
	}
	return AuthorizeResult{Status: wire.Status}, nil
}
