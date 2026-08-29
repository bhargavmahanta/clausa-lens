package capture

import "testing"

func TestIDGenerator_ResetIsDeterministic(t *testing.T) {
	ids := NewIDGenerator(DefaultCheckoutSeed)
	first := ids.PeekNextLogicalOperationID()
	ids.NextCheckoutSeq()
	ids.NextCheckoutSeq()
	if ids.PeekNextLogicalOperationID() == first {
		t.Fatalf("expected next logical operation id to advance before reset")
	}

	ids.Reset()
	if got := ids.PeekNextLogicalOperationID(); got != first {
		t.Fatalf("after reset, next logical operation id = %q, want deterministic %q", got, first)
	}
	if got := LogicalOperationID(DefaultCheckoutSeed); got != "checkout-8271" {
		t.Fatalf("LogicalOperationID(%d) = %q, want checkout-8271", DefaultCheckoutSeed, got)
	}
}
