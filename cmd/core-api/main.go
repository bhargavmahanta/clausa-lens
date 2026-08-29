package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/causalens/causalens/internal/contracts"
	"github.com/causalens/causalens/internal/core"
)

func handler(repository core.Repository) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeNotFound(w)
			return
		}
		var event contracts.ExecutionEvent
		if err := contracts.DecodeStrict(r.Body, &event); err != nil {
			writeContractError(w, http.StatusBadRequest, contracts.SchemaInvalid, "request does not match the contract")
			return
		}
		if err := repository.IngestEvent(r.Context(), event); err != nil {
			if errors.Is(err, core.ErrConflict) {
				writeContractError(w, http.StatusBadRequest, contracts.SchemaInvalid, "request does not match the contract")
				return
			}
			if _, ok := err.(*core.RepositoryError); !ok {
				writeContractError(w, http.StatusBadRequest, contracts.SchemaInvalid, "request does not match the contract")
				return
			}
			writeMappedError(w, err, "")
			return
		}
		writeJSON(w, http.StatusAccepted, contracts.AcceptedEventResponse{EventID: event.EventID, Status: contracts.Accepted})
	})
	mux.HandleFunc("/v1/incidents", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeNotFound(w)
			return
		}
		query, ok := incidentListQuery(r)
		if !ok {
			writeContractError(w, http.StatusBadRequest, contracts.SchemaInvalid, "request does not match the contract")
			return
		}
		response, err := repository.ListIncidents(r.Context(), query)
		if err != nil {
			writeMappedError(w, err, "")
			return
		}
		if response.Items == nil {
			response.Items = []contracts.Incident{}
		}
		writeJSON(w, http.StatusOK, response)
	})
	mux.HandleFunc("/v1/incidents/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeNotFound(w)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/v1/incidents/")
		if id == "" || strings.Contains(id, "/") {
			writeNotFound(w)
			return
		}
		response, err := repository.GetIncidentDetail(r.Context(), id)
		if err != nil {
			writeMappedError(w, err, "")
			return
		}
		if response.Events == nil {
			response.Events = []contracts.ExecutionEvent{}
		}
		writeJSON(w, http.StatusOK, response)
	})
	return mux
}

func incidentListQuery(r *http.Request) (contracts.IncidentListQuery, bool) {
	values := r.URL.Query()
	for key, entries := range values {
		if key != "status" && key != "cursor" && key != "limit" || len(entries) != 1 {
			return contracts.IncidentListQuery{}, false
		}
	}
	query := contracts.IncidentListQuery{Cursor: values.Get("cursor")}
	if status := values.Get("status"); status != "" {
		query.Status = contracts.IncidentStatus(status)
		if query.Status != contracts.IncidentDetected && query.Status != contracts.IncidentReady && query.Status != contracts.IncidentBlocked {
			return contracts.IncidentListQuery{}, false
		}
	}
	if raw := values.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			return contracts.IncidentListQuery{}, false
		}
		query.Limit = &limit
	}
	return query, true
}

func writeMappedError(w http.ResponseWriter, err error, safetyCode contracts.ErrorCode) {
	if errors.Is(err, core.ErrInvalidLifecycle) {
		writeContractError(w, http.StatusConflict, contracts.SchemaInvalid, "invalid lifecycle transition")
		return
	}
	if safetyCode == contracts.SanitizationFailed || safetyCode == contracts.IsolationViolation || safetyCode == contracts.DestinationBlocked {
		writeContractError(w, http.StatusUnprocessableEntity, safetyCode, "request blocked by safety policy")
		return
	}
	if errors.Is(err, core.ErrNotFound) {
		writeContractError(w, http.StatusNotFound, contracts.SchemaInvalid, "resource not found")
		return
	}
	if errors.Is(err, core.ErrConflict) {
		writeContractError(w, http.StatusBadRequest, contracts.SchemaInvalid, "request does not match the contract")
		return
	}
	writeContractError(w, http.StatusInternalServerError, contracts.InternalFailure, "internal failure")
}

func writeNotFound(w http.ResponseWriter) {
	writeContractError(w, http.StatusNotFound, contracts.SchemaInvalid, "resource not found")
}
func writeContractError(w http.ResponseWriter, status int, code contracts.ErrorCode, message string) {
	writeJSON(w, status, contracts.APIErrorResponse{Error: contracts.RunError{Code: code, Message: message, Retryable: false, Details: map[string]any{}}})
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	db, err := core.OpenPostgres(databaseURL)
	if err != nil {
		log.Fatal("failed to initialize database")
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatal("database is unavailable")
	}
	if err := http.ListenAndServe(":8080", handler(core.NewPostgresRepository(db))); err != nil {
		log.Fatal("core API stopped")
	}
}
