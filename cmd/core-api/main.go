package main

import (
	"encoding/json"
	"github.com/causalens/causalens/internal/contracts"
	"github.com/causalens/causalens/internal/core"
	"net/http"
	"strings"
)

func main() {
	s := core.NewStore()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.NotFound(w, r)
			return
		}
		var e contracts.ExecutionEvent
		if json.NewDecoder(r.Body).Decode(&e) != nil || s.IngestEvent(e) != nil {
			http.Error(w, `{"error":{"code":"VALIDATION_ERROR","message":"invalid event"}}`, 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		json.NewEncoder(w).Encode(map[string]any{"event_id": e.EventID, "status": "ACCEPTED"})
	})
	mux.HandleFunc("/v1/incidents/", func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimPrefix(r.URL.Path, "/v1/incidents/") == "" {
			http.NotFound(w, r)
			return
		}
		http.NotFound(w, r)
	})
	http.ListenAndServe(":8080", mux)
}
