package checkout

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/causalens/causalens/internal/contracts"
)

func TestNormalize_DecodesJSONArrayOfEvents(t *testing.T) {
	p := New()
	payload, err := json.Marshal(goldenEvidence())
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}

	events, err := p.Normalize(context.Background(), contracts.RawEvidence{
		Source: "demo", ContentType: "application/json", ReceivedAt: fixedTime, Payload: payload,
	})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if len(events) != len(goldenEvidence()) {
		t.Fatalf("got %d events, want %d", len(events), len(goldenEvidence()))
	}
	for _, e := range events {
		if err := e.Validate(); err != nil {
			t.Fatalf("normalized event fails contract validation: %v", err)
		}
	}
}

func TestNormalize_RejectsUnsupportedContentType(t *testing.T) {
	p := New()
	_, err := p.Normalize(context.Background(), contracts.RawEvidence{
		Source: "demo", ContentType: "text/plain", ReceivedAt: fixedTime, Payload: []byte("not json"),
	})
	if err == nil {
		t.Fatalf("expected an error for unsupported content_type")
	}
}

func TestNormalize_RejectsMalformedPayload(t *testing.T) {
	p := New()
	_, err := p.Normalize(context.Background(), contracts.RawEvidence{
		Source: "demo", ContentType: "application/json", ReceivedAt: fixedTime, Payload: []byte(`{"not":"an array"}`),
	})
	if err == nil {
		t.Fatalf("expected an error for a payload that is not a JSON array of events")
	}
}

func TestNormalize_RejectsEventFailingContractValidation(t *testing.T) {
	p := New()
	invalid := goldenEvidence()
	invalid[0].Attempt = 0 // violates attempt >= 1
	payload, err := json.Marshal(invalid)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	_, err = p.Normalize(context.Background(), contracts.RawEvidence{
		Source: "demo", ContentType: "application/json", ReceivedAt: fixedTime, Payload: payload,
	})
	if err == nil {
		t.Fatalf("expected an error for an event that fails contract validation")
	}
}
