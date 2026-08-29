// Command demo-gateway runs the golden scenario's Gateway service, the
// ENTRYPOINT for POST /checkout.
package main

import (
	"log"
	"net/http"
	"os"

	checkoutsvc "github.com/causalens/causalens/cmd/demo-checkout/service"
	gatewaysvc "github.com/causalens/causalens/cmd/demo-gateway/service"
	"github.com/causalens/causalens/internal/capture"
	"github.com/causalens/causalens/internal/contracts"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	checkoutURL := os.Getenv("CHECKOUT_URL")
	if checkoutURL == "" {
		checkoutURL = "http://localhost:8081"
	}

	sink := capture.Sink(capture.NewInMemorySink())
	if coreAPIURL := os.Getenv("CORE_API_EVENTS_URL"); coreAPIURL != "" {
		sink = capture.NewHTTPSink(coreAPIURL)
	}
	recorder := capture.NewRecorder(contracts.ComponentRef{Name: "gateway", Instance: "gateway-1"}, capture.NewIDGenerator(1), sink)
	ids := capture.NewIDGenerator(capture.DefaultCheckoutSeed)

	checkout := checkoutsvc.NewClient(checkoutURL)
	svc := gatewaysvc.New(checkout, ids, recorder)
	handler := gatewaysvc.Handler(svc)

	log.Printf("demo-gateway listening on :%s (checkout=%s)", port, checkoutURL)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatalf("demo-gateway: %v", err)
	}
}
