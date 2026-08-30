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

// configLatencyWireRequest is the wire shape for POST /config/latency, the
// demo control used by the Gateway's healthy-control scenario. It is not
// part of the frozen Core API contract.
type configLatencyWireRequest struct {
	LatencyMs int `json:"latency_ms"`
}

type configLatencyWireResponse struct {
	LatencyMs int `json:"latency_ms"`
}

// Handler exposes Service over HTTP as POST /authorize.
func Handler(svc *Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/config/latency", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var wire configLatencyWireRequest
		if err := json.NewDecoder(r.Body).Decode(&wire); err != nil {
			http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
			return
		}
		if wire.LatencyMs < 1 || wire.LatencyMs > 10000 {
			http.Error(w, "latency_ms must be between 1 and 10000", http.StatusBadRequest)
			return
		}
		svc.SetLatencyMs(wire.LatencyMs)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(configLatencyWireResponse{LatencyMs: svc.LatencyMs()})
	})
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

// SetLatency reconfigures the simulator's per-attempt latency via the demo
// POST /config/latency control endpoint.
func (c *Client) SetLatency(ctx context.Context, latencyMs int) error {
	body, err := json.Marshal(configLatencyWireRequest{LatencyMs: latencyMs})
	if err != nil {
		return fmt.Errorf("payment client: marshal config request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/config/latency", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("payment client: build config request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("payment client: config request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("payment client: unexpected config status %d", resp.StatusCode)
	}
	return nil
}

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
