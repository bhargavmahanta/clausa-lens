package capture

import (
	"fmt"
	"sync"
)

// DefaultCheckoutSeed matches the golden scenario identifiers used
// throughout docs/CONTRACTS.md and docs/DEMO_SCENARIO.md (checkout-8271,
// trace-8271, exec-original-8271) and the frozen ResetResult
// next_logical_operation_id.
const DefaultCheckoutSeed = 8271

// IDGenerator produces the identifiers a demo run needs: a new logical
// checkout (and its derived trace/execution ids) when the caller does not
// supply one, and a globally unique event_id for every emitted
// ExecutionEvent. It is shared across every Recorder in a process so
// event_id stays globally unique the way docs/CONTRACTS.md requires.
type IDGenerator struct {
	mu       sync.Mutex
	seed     int
	nextSeq  int
	eventSeq uint64
}

// NewIDGenerator constructs a generator seeded at seed. Use
// DefaultCheckoutSeed for the golden scenario.
func NewIDGenerator(seed int) *IDGenerator {
	g := &IDGenerator{seed: seed}
	g.Reset()
	return g
}

// Reset restores the generator to its deterministic starting state, matching
// the reset contract's requirement that next_logical_operation_id is
// deterministic after a reset.
func (g *IDGenerator) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nextSeq = g.seed
	g.eventSeq = 0
}

// NextCheckoutSeq allocates the next deterministic checkout sequence number.
func (g *IDGenerator) NextCheckoutSeq() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	seq := g.nextSeq
	g.nextSeq++
	return seq
}

// PeekNextLogicalOperationID reports the logical_operation_id the next call
// to NextCheckoutSeq will allocate, without consuming it.
func (g *IDGenerator) PeekNextLogicalOperationID() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return LogicalOperationID(g.nextSeq)
}

// NextEventID returns a globally unique event_id for the given component
// name. Event ids need not be deterministic across resets, only unique for
// the life of the process.
func (g *IDGenerator) NextEventID(component string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.eventSeq++
	return fmt.Sprintf("evt-%s-%d", component, g.eventSeq)
}

// LogicalOperationID derives the logical_operation_id for a checkout
// sequence number, e.g. 8271 -> "checkout-8271".
func LogicalOperationID(seq int) string { return fmt.Sprintf("checkout-%d", seq) }

// TraceID derives the trace_id for a checkout sequence number.
func TraceID(seq int) string { return fmt.Sprintf("trace-%d", seq) }

// ExecutionID derives the execution_id for a checkout sequence number.
func ExecutionID(seq int) string { return fmt.Sprintf("exec-original-%d", seq) }
