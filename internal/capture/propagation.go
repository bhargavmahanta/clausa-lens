package capture

// HTTP header names the demo services use to propagate identifiers between
// hops. These are internal to the Gateway-Checkout-Payment-Ledger demo
// wiring; they are not part of the frozen Core API resource contract in
// CONTRACTS.md.
const (
	HeaderTraceID            = "X-Causalens-Trace-Id"
	HeaderExecutionID        = "X-Causalens-Execution-Id"
	HeaderLogicalOperationID = "X-Causalens-Logical-Operation-Id"
	HeaderParentEventID      = "X-Causalens-Parent-Event-Id"
)
