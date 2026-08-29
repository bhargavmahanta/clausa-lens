// Command demo-payment runs the golden scenario's Payment dependency
// simulator.
package main

import (
	"log"
	"net/http"
	"os"
	"strconv"

	paymentsvc "github.com/causalens/causalens/cmd/demo-payment/service"
	"github.com/causalens/causalens/internal/capture"
	"github.com/causalens/causalens/internal/contracts"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	sink := capture.Sink(capture.NewInMemorySink())
	if coreAPIURL := os.Getenv("CORE_API_EVENTS_URL"); coreAPIURL != "" {
		sink = capture.NewHTTPSink(coreAPIURL)
	}
	recorder := capture.NewRecorder(contracts.ComponentRef{Name: "payment", Instance: "payment-1"}, capture.NewIDGenerator(1), sink)

	svc := paymentsvc.New(recorder)
	if raw := os.Getenv("PAYMENT_LATENCY_MS"); raw != "" {
		if ms, err := strconv.Atoi(raw); err == nil {
			svc.SetLatencyMs(ms)
		} else {
			log.Printf("demo-payment: ignoring invalid PAYMENT_LATENCY_MS=%q: %v", raw, err)
		}
	}
	handler := paymentsvc.Handler(svc)

	log.Printf("demo-payment listening on :%s (latency_ms=%d)", port, svc.LatencyMs())
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatalf("demo-payment: %v", err)
	}
}
