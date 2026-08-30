package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/causalens/causalens/internal/capture"
	"github.com/causalens/causalens/internal/contracts"
)

func TestHandler_ConfigLatencyConfiguresSimulator(t *testing.T) {
	sink := capture.NewInMemorySink()
	recorder := capture.NewRecorder(contracts.ComponentRef{Name: "payment", Instance: "payment-1"}, capture.NewIDGenerator(1), sink)
	svc := New(recorder)
	server := httptest.NewServer(Handler(svc))
	defer server.Close()

	resp, err := http.Post(server.URL+"/config/latency", "application/json", strings.NewReader(`{"latency_ms":50}`))
	if err != nil {
		t.Fatalf("POST /config/latency: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var wire configLatencyWireResponse
	if err := json.NewDecoder(resp.Body).Decode(&wire); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if wire.LatencyMs != 50 {
		t.Fatalf("response latency_ms = %d, want 50", wire.LatencyMs)
	}
	if got := svc.LatencyMs(); got != 50 {
		t.Fatalf("LatencyMs() = %d, want 50", got)
	}
}

func TestHandler_ConfigLatencyRejectsInvalidRangeWithoutMutating(t *testing.T) {
	sink := capture.NewInMemorySink()
	recorder := capture.NewRecorder(contracts.ComponentRef{Name: "payment", Instance: "payment-1"}, capture.NewIDGenerator(1), sink)
	svc := New(recorder)
	server := httptest.NewServer(Handler(svc))
	defer server.Close()

	resp, err := http.Post(server.URL+"/config/latency", "application/json", strings.NewReader(`{"latency_ms":0}`))
	if err != nil {
		t.Fatalf("POST /config/latency: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if got := svc.LatencyMs(); got != DefaultLatencyMs {
		t.Fatalf("rejected request must not mutate latency: got %d, want %d", got, DefaultLatencyMs)
	}
}

func TestClient_SetLatencyConfiguresRemoteSimulator(t *testing.T) {
	sink := capture.NewInMemorySink()
	recorder := capture.NewRecorder(contracts.ComponentRef{Name: "payment", Instance: "payment-1"}, capture.NewIDGenerator(1), sink)
	svc := New(recorder)
	server := httptest.NewServer(Handler(svc))
	defer server.Close()

	client := NewClient(server.URL)
	if err := client.SetLatency(context.Background(), 50); err != nil {
		t.Fatalf("SetLatency: %v", err)
	}
	if got := svc.LatencyMs(); got != 50 {
		t.Fatalf("LatencyMs() = %d, want 50", got)
	}
}
