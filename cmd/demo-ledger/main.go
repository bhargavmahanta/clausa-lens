// Command demo-ledger runs the golden scenario's Ledger service.
package main

import (
	"log"
	"net/http"
	"os"

	ledgersvc "github.com/causalens/causalens/cmd/demo-ledger/service"
	"github.com/causalens/causalens/internal/capture"
	"github.com/causalens/causalens/internal/contracts"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8083"
	}

	sink := capture.Sink(capture.NewInMemorySink())
	if coreAPIURL := os.Getenv("CORE_API_EVENTS_URL"); coreAPIURL != "" {
		sink = capture.NewHTTPSink(coreAPIURL)
	}
	recorder := capture.NewRecorder(contracts.ComponentRef{Name: "ledger", Instance: "ledger-1"}, capture.NewIDGenerator(1), sink)

	svc := ledgersvc.New(recorder)
	handler := ledgersvc.Handler(svc)

	log.Printf("demo-ledger listening on :%s", port)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatalf("demo-ledger: %v", err)
	}
}
