package main

import (
	"encoding/json"
	"github.com/causalens/causalens/internal/contracts"
	"github.com/causalens/causalens/internal/core"
	"net/http"
	"sort"
	"strings"
)

func handler(s *core.Store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.NotFound(w, r)
			return
		}
		var e contracts.ExecutionEvent
		if json.NewDecoder(r.Body).Decode(&e) != nil || s.IngestEvent(e) != nil {
			writeError(w, 400, contracts.RunError{Code: contracts.SchemaInvalid, Message: "invalid event"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		json.NewEncoder(w).Encode(map[string]any{"event_id": e.EventID, "status": "ACCEPTED"})
	})
	mux.HandleFunc("/v1/incidents", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"items": s.ListIncidents()})
	})
	mux.HandleFunc("/v1/incidents/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v1/incidents/")
		i, g, ok := s.Incident(id)
		if !ok {
			http.NotFound(w, r)
			return
		}
		events := s.Events()
		sort.SliceStable(events, func(a, b int) bool { return events[a].OccurredAt < events[b].OccurredAt })
		json.NewEncoder(w).Encode(map[string]any{"incident": i, "graph": g, "events": events})
	})
	return mux
}
func writeError(w http.ResponseWriter, status int, e contracts.RunError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"error": e})
}
func main() { http.ListenAndServe(":8080", handler(core.NewStore())) }
