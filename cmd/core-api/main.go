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
	"sync"
	"time"

	"github.com/causalens/causalens/internal/capsule"
	"github.com/causalens/causalens/internal/contracts"
	"github.com/causalens/causalens/internal/core"
	"github.com/causalens/causalens/internal/differential"
	"github.com/causalens/causalens/internal/packregistry"
	"github.com/causalens/causalens/internal/replay"
)

// APIRepository is the persistence seam the Core HTTP API consumes. It embeds
// the A2 Repository and adds the run/capsule/diff/reset methods the frozen
// replay routes need. core.Store and core.PostgresRepository both satisfy it.
type APIRepository interface {
	core.Repository
	PutIncident(context.Context, contracts.Incident, contracts.ExecutionGraph) error
	PutCapsule(context.Context, contracts.ReplayCapsule) error
	GetCapsule(context.Context, string) (contracts.ReplayCapsule, error)
	PutRun(context.Context, contracts.ReplayRun) error
	GetRun(context.Context, string) (contracts.ReplayRun, error)
	TransitionRun(context.Context, contracts.ReplayRunStatus, contracts.ReplayRun) error
	PutDiff(context.Context, contracts.ReplayDiff) error
	GetDiff(context.Context, string) (contracts.ReplayDiff, error)
	EventsForRun(context.Context, contracts.ReplayRun) ([]contracts.ExecutionEvent, error)
	GraphsForRun(context.Context, contracts.ReplayRun) (contracts.ExecutionGraph, error)
	Reset(context.Context) (core.ResetCounts, error)
}

// HandlerDeps carries the optional live components the replay routes need. A
// SystemPack is required to compile capsules and align diffs; routes that need
// it return PACK_UNAVAILABLE when none is supplied instead of panicking.
type HandlerDeps struct {
	Pack contracts.SystemPack
}

// ID generators: deterministic monotonic counters for run and diff resources.
var idCounter struct {
	sync.Mutex
	value int64
}

func nextID(prefix string) string {
	idCounter.Lock()
	defer idCounter.Unlock()
	idCounter.value++
	return prefix + strconv.FormatInt(idCounter.value, 10)
}

func handler(repository APIRepository) http.Handler {
	return handlerWithDeps(repository, HandlerDeps{})
}

func handlerWithDeps(repository APIRepository, deps HandlerDeps) http.Handler {
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
		// After a valid event is accepted, evaluate the accumulated execution
		// against the configured System Pack and persist a detected incident when
		// the failure oracle matches. Detection is best-effort: it never fails the
		// already-accepted event, so the ingest contract (202) is preserved.
		if err := detectAndPersistIncident(r.Context(), repository, deps.Pack, event.ExecutionID); err != nil {
			log.Printf("event %s ingested but incident detection failed: %v", event.EventID, err)
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
		id := strings.TrimPrefix(r.URL.Path, "/v1/incidents/")
		if id == "" {
			writeNotFound(w)
			return
		}
		if r.Method == http.MethodPost && strings.HasSuffix(id, "/capsules") {
			incidentID := strings.TrimSuffix(id, "/capsules")
			if incidentID == "" || strings.Contains(incidentID, "/") {
				writeNotFound(w)
				return
			}
			postCapsule(w, r, repository, deps, incidentID)
			return
		}
		if r.Method != http.MethodGet || strings.Contains(id, "/") {
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
	mux.HandleFunc("/v1/capsules/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeNotFound(w)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/v1/capsules/")
		if id == "" {
			writeNotFound(w)
			return
		}
		if !strings.HasSuffix(id, "/runs") {
			writeNotFound(w)
			return
		}
		capsuleID := strings.TrimSuffix(id, "/runs")
		if capsuleID == "" || strings.Contains(capsuleID, "/") {
			writeNotFound(w)
			return
		}
		postRun(w, r, repository, deps, capsuleID)
	})
	mux.HandleFunc("/v1/runs/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeNotFound(w)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/v1/runs/")
		if id == "" || strings.Contains(id, "/") {
			writeNotFound(w)
			return
		}
		run, err := repository.GetRun(r.Context(), id)
		if err != nil {
			writeMappedError(w, err, "")
			return
		}
		writeJSON(w, http.StatusOK, run)
	})
	mux.HandleFunc("/v1/diffs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeNotFound(w)
			return
		}
		postDiff(w, r, repository, deps)
	})
	mux.HandleFunc("/v1/diffs/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeNotFound(w)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/v1/diffs/")
		if id == "" || strings.Contains(id, "/") {
			writeNotFound(w)
			return
		}
		diff, err := repository.GetDiff(r.Context(), id)
		if err != nil {
			writeMappedError(w, err, "")
			return
		}
		writeJSON(w, http.StatusOK, diff)
	})
	mux.HandleFunc("/v1/demo/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeNotFound(w)
			return
		}
		postReset(w, r, repository)
	})
	return mux
}

func postCapsule(w http.ResponseWriter, r *http.Request, repository APIRepository, deps HandlerDeps, incidentID string) {
	if deps.Pack == nil {
		writeContractError(w, http.StatusUnprocessableEntity, contracts.PackUnavailable, "replay capsule requires a System Pack")
		return
	}
	detail, err := repository.GetIncidentDetail(r.Context(), incidentID)
	if err != nil {
		writeMappedError(w, err, "")
		return
	}
	source := contracts.CapsuleSource{
		IncidentID:         detail.Incident.IncidentID,
		TraceID:            detail.Incident.TraceID,
		ExecutionID:        detail.Incident.ExecutionID,
		CaptureEnvironment: contracts.CaptureDemo,
		CapturedAt:         detail.Incident.DetectedAt,
	}
	// A capsule is immutable and keyed by a per-incident id; compiling is
	// deterministic. Re-requesting an already-compiled capsule returns the
	// existing one rather than failing with a conflict.
	capsuleID := "cap-" + incidentID + "-1"
	if existing, err := repository.GetCapsule(r.Context(), capsuleID); err == nil {
		writeJSON(w, http.StatusCreated, existing)
		return
	}
	c, err := capsule.Compile(deps.Pack, detail.Incident.SystemPack, detail.Incident, detail.Events,
		source, contracts.Trigger{RequestOrMessage: map[string]any{}, SanitizedHeaders: map[string]string{}},
		capsuleID, detail.Incident.GraphID, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		writeCapsuleError(w, err)
		return
	}
	if err := repository.PutCapsule(r.Context(), c); err != nil {
		writeMappedError(w, err, "")
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func postRun(w http.ResponseWriter, r *http.Request, repository APIRepository, deps HandlerDeps, capsuleID string) {
	var request contracts.CreateRunRequest
	if err := contracts.DecodeStrict(r.Body, &request); err != nil {
		writeContractError(w, http.StatusBadRequest, contracts.SchemaInvalid, "request does not match the contract")
		return
	}
	if err := request.Validate(); err != nil {
		writeContractError(w, http.StatusBadRequest, contracts.SchemaInvalid, "request does not match the contract")
		return
	}
	c, err := repository.GetCapsule(r.Context(), capsuleID)
	if err != nil {
		writeMappedError(w, err, "")
		return
	}
	runID := nextID("run-")
	run, err := replay.NewRun(runID, "exec-replay-"+runID, c.CapsuleID, c.Integrity.Digest,
		request.RunType, request.BaselineRunID, request.Intervention, 1)
	if err != nil {
		writeContractError(w, http.StatusBadRequest, contracts.SchemaInvalid, "request does not match the contract")
		return
	}
	if err := repository.PutRun(r.Context(), run); err != nil {
		writeMappedError(w, err, "")
		return
	}
	writeJSON(w, http.StatusAccepted, run)
}

func postDiff(w http.ResponseWriter, r *http.Request, repository APIRepository, deps HandlerDeps) {
	var request contracts.CreateDiffRequest
	if err := contracts.DecodeStrict(r.Body, &request); err != nil || request.Validate() != nil {
		writeContractError(w, http.StatusBadRequest, contracts.SchemaInvalid, "request does not match the contract")
		return
	}
	align, ok := deps.Pack.(differential.Alignment)
	if !ok || deps.Pack == nil {
		writeContractError(w, http.StatusUnprocessableEntity, contracts.PackUnavailable, "differential analysis requires a System Pack")
		return
	}
	diff, err := differential.Build(r.Context(), repository, nextID("diff-"), request.BaselineRunID, request.ComparisonRunID, align)
	if err != nil {
		if errors.Is(err, differential.ErrDiffPrerequisites) {
			writeContractError(w, http.StatusConflict, contracts.SchemaInvalid, "replay diff prerequisites not met")
			return
		}
		writeMappedError(w, err, "")
		return
	}
	if err := repository.PutDiff(r.Context(), diff); err != nil {
		writeMappedError(w, err, "")
		return
	}
	writeJSON(w, http.StatusCreated, diff)
}

func postReset(w http.ResponseWriter, r *http.Request, repository APIRepository) {
	var request contracts.ResetRequest
	if err := contracts.DecodeStrict(r.Body, &request); err != nil || request.Validate() != nil {
		writeContractError(w, http.StatusBadRequest, contracts.SchemaInvalid, "request does not match the contract")
		return
	}
	counts, err := repository.Reset(r.Context())
	if err != nil {
		writeContractError(w, http.StatusInternalServerError, contracts.InternalFailure, "internal failure")
		return
	}
	writeJSON(w, http.StatusOK, contracts.ResetResult{
		SchemaVersion:          contracts.ContractVersion,
		ResetID:                "reset-1",
		Status:                 contracts.ResetCompleted,
		ClearedIncidentCount:   counts.Incidents,
		ClearedRunCount:        counts.Runs,
		ClearedLedgerCount:     0,
		FixtureVersion:         "1.0.0",
		ConfiguredLatencyMS:    350,
		DeduplicationEnabled:   false,
		NextLogicalOperationID: "checkout-8271",
	})
}

func writeCapsuleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, capsule.ErrNotReady):
		writeContractError(w, http.StatusBadRequest, contracts.SchemaInvalid, "incident is not READY")
	case errors.Is(err, capsule.ErrIntegrityMismatch):
		writeContractError(w, http.StatusUnprocessableEntity, contracts.IntegrityMismatch, "replay capsule integrity mismatch")
	case errors.Is(err, capsule.ErrPackValidation):
		writeContractError(w, http.StatusUnprocessableEntity, contracts.SanitizationFailed, "replay capsule failed pack validation")
	default:
		// Compilation failed for a reason other than a refused pre-condition or
		// a pack validation issue: treat as an internal failure rather than
		// masking it as PACK_UNAVAILABLE. PACK_UNAVAILABLE is returned only by
		// postCapsule when no pack is wired, before compilation starts.
		writeContractError(w, http.StatusInternalServerError, contracts.InternalFailure, "replay capsule could not be compiled")
	}
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
	h := handlerWithDeps(core.NewPostgresRepository(db), HandlerDeps{Pack: resolvePack()})
	if err := http.ListenAndServe(":8080", h); err != nil {
		log.Fatal("core API stopped")
	}
}

// resolvePack selects the System Pack for this deployment from the PACK_IMPL
// environment value via the pack registry. PACK_IMPL may be empty, in which
// case no pack is wired and the capsule/diff routes return PACK_UNAVAILABLE
// rather than failing; or it may name the dev pack ("dev") or, once Member 1
// lands their work, the real checkout pack implementation token. The registry
// keeps core logic pack-agnostic; only this call site reads the environment.
func resolvePack() contracts.SystemPack {
	return packregistry.Resolve(os.Getenv("PACK_IMPL"))
}
