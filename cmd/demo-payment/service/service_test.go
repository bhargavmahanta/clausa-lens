package service

import (
	"context"
	"testing"
	"time"

	"github.com/causalens/causalens/internal/capture"
	"github.com/causalens/causalens/internal/contracts"
)

func newTestService(t *testing.T) (*Service, *capture.InMemorySink, *[]time.Duration) {
	t.Helper()
	sink := capture.NewInMemorySink()
	recorder := capture.NewRecorder(contracts.ComponentRef{Name: "payment", Instance: "payment-1"}, capture.NewIDGenerator(1), sink)
	svc := New(recorder)
	var slept []time.Duration
	svc.sleep = func(d time.Duration) { slept = append(slept, d) }
	return svc, sink, &slept
}

func TestService_DefaultLatencyIs350ms(t *testing.T) {
	svc, _, _ := newTestService(t)
	if got := svc.LatencyMs(); got != DefaultLatencyMs {
		t.Fatalf("LatencyMs() = %d, want %d", got, DefaultLatencyMs)
	}
	if DefaultLatencyMs != 350 {
		t.Fatalf("DefaultLatencyMs = %d, want 350 (P0 fixed value)", DefaultLatencyMs)
	}
}

func TestService_AuthorizeSleepsExactlyConfiguredLatency(t *testing.T) {
	svc, sink, slept := newTestService(t)
	svc.SetLatencyMs(50)
	ctx := context.Background()

	result, err := svc.Authorize(ctx, AuthorizeRequest{
		ExecutionID: "exec-1", TraceID: "trace-1", LogicalOperationID: "checkout-1", Attempt: 1,
	})
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if result.Status != "APPROVED" {
		t.Fatalf("Status = %q, want APPROVED", result.Status)
	}
	if len(*slept) != 1 || (*slept)[0] != 50*time.Millisecond {
		t.Fatalf("expected exactly one deterministic 50ms sleep, got %v", *slept)
	}

	events := sink.Events()
	if len(events) != 2 {
		t.Fatalf("expected START and COMPLETE events, got %d", len(events))
	}
	for _, ev := range events {
		if err := ev.Validate(); err != nil {
			t.Fatalf("captured event fails contract validation: %v", err)
		}
	}
	if events[0].EventType != contracts.EventStart {
		t.Fatalf("first event = %s, want START", events[0].EventType)
	}
	if events[0].Attributes["configured_latency_ms"] != 50 {
		t.Fatalf("configured_latency_ms = %v, want 50", events[0].Attributes["configured_latency_ms"])
	}
	if events[1].EventType != contracts.EventComplete {
		t.Fatalf("second event = %s, want COMPLETE", events[1].EventType)
	}
	if events[1].DurationMS == nil || *events[1].DurationMS != 50 {
		t.Fatalf("duration_ms = %v, want 50", events[1].DurationMS)
	}
}

func TestService_AttemptCountAndReset(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	for i := 1; i <= 2; i++ {
		if _, err := svc.Authorize(ctx, AuthorizeRequest{
			ExecutionID: "exec-1", TraceID: "trace-1", LogicalOperationID: "checkout-1", Attempt: i,
		}); err != nil {
			t.Fatalf("Authorize attempt %d: %v", i, err)
		}
	}
	if got := svc.AttemptCount(); got != 2 {
		t.Fatalf("AttemptCount() = %d, want 2", got)
	}

	svc.SetLatencyMs(50)
	svc.Reset()
	if got := svc.AttemptCount(); got != 0 {
		t.Fatalf("Reset did not clear attempt count, got %d", got)
	}
	if got := svc.LatencyMs(); got != DefaultLatencyMs {
		t.Fatalf("Reset did not restore default latency, got %d", got)
	}
}
