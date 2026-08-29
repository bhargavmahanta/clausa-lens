package checkout

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/causalens/causalens/internal/contracts"
)

// Normalize implements contracts.SystemPack: it converts raw demo-service
// evidence into canonical ExecutionEvent records. The demo services already
// emit canonical contracts.ExecutionEvent JSON, so raw.Payload is expected
// to be a JSON array of ExecutionEvent objects; RawEvidence is
// adapter-owned input and is never persisted as canonical evidence without
// this normalization and validation step.
func (p *Pack) Normalize(_ context.Context, raw contracts.RawEvidence) ([]contracts.ExecutionEvent, error) {
	if raw.ContentType != "application/json" {
		return nil, fmt.Errorf("checkout: unsupported content_type %q", raw.ContentType)
	}

	var rawEvents []json.RawMessage
	if err := json.Unmarshal(raw.Payload, &rawEvents); err != nil {
		return nil, fmt.Errorf("checkout: payload is not a JSON array of events: %w", err)
	}

	events := make([]contracts.ExecutionEvent, 0, len(rawEvents))
	for i, rawEvent := range rawEvents {
		var event contracts.ExecutionEvent
		if err := contracts.DecodeStrict(bytes.NewReader(rawEvent), &event); err != nil {
			return nil, fmt.Errorf("checkout: event %d does not match the ExecutionEvent contract: %w", i, err)
		}
		if err := event.Validate(); err != nil {
			return nil, fmt.Errorf("checkout: event %d (%s) is invalid: %w", i, event.EventID, err)
		}
		events = append(events, event)
	}
	return events, nil
}
