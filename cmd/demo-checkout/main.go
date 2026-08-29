// Command demo-checkout runs the golden scenario's Checkout service. It
// owns the 200 ms timeout and maximum-two-attempts retry policy.
package main

import (
	"log"
	"net/http"
	"os"

	checkoutsvc "github.com/causalens/causalens/cmd/demo-checkout/service"
	ledgersvc "github.com/causalens/causalens/cmd/demo-ledger/service"
	paymentsvc "github.com/causalens/causalens/cmd/demo-payment/service"
	"github.com/causalens/causalens/internal/capture"
	"github.com/causalens/causalens/internal/contracts"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}
	paymentURL := os.Getenv("PAYMENT_URL")
	if paymentURL == "" {
		paymentURL = "http://localhost:8082"
	}
	ledgerURL := os.Getenv("LEDGER_URL")
	if ledgerURL == "" {
		ledgerURL = "http://localhost:8083"
	}

	sink := capture.Sink(capture.NewInMemorySink())
	if coreAPIURL := os.Getenv("CORE_API_EVENTS_URL"); coreAPIURL != "" {
		sink = capture.NewHTTPSink(coreAPIURL)
	}
	recorder := capture.NewRecorder(contracts.ComponentRef{Name: "checkout", Instance: "checkout-1"}, capture.NewIDGenerator(1), sink)

	payment := paymentsvc.NewClient(paymentURL)
	ledger := ledgersvc.NewClient(ledgerURL)
	svc := checkoutsvc.New(payment, ledger, recorder)
	handler := checkoutsvc.Handler(svc)

	log.Printf("demo-checkout listening on :%s (payment=%s ledger=%s)", port, paymentURL, ledgerURL)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatalf("demo-checkout: %v", err)
	}
}
