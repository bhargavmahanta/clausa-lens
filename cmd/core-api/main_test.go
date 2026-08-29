package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/causalens/causalens/internal/contracts"
	"github.com/causalens/causalens/internal/core"
)

const validEventJSON = `{"schema_version":"1.0","event_id":"e1","execution_id":"exec","trace_id":"trace","component":{"name":"c","instance":"i"},"operation":{"name":"o","kind":"INTERNAL"},"event_type":"START","attempt":1,"logical_operation_id":"logical","occurred_at":"2026-08-29T10:32:01Z","sequence":0,"status":"RUNNING","attributes":{}}`

func request(t *testing.T, h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q", got)
	}
	return w
}

func TestEventSuccessGoldenBody(t *testing.T) {
	w := request(t, handler(core.NewStore()), http.MethodPost, "/v1/events", validEventJSON)
	if w.Code != http.StatusAccepted || w.Body.String() != "{\"event_id\":\"e1\",\"status\":\"ACCEPTED\"}\n" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestEventsRejectInvalidUnknownTrailingAndDuplicate(t *testing.T) {
	h := handler(core.NewStore())
	cases := []string{`{}`, strings.TrimSuffix(validEventJSON, "}") + `,"unknown":true}`, validEventJSON + `{}`}
	for n, body := range cases {
		w := request(t, h, http.MethodPost, "/v1/events", body)
		want := "{\"error\":{\"code\":\"SCHEMA_INVALID\",\"message\":\"request does not match the contract\",\"retryable\":false,\"details\":{}}}\n"
		if w.Code != http.StatusBadRequest || w.Body.String() != want {
			t.Fatalf("case %d: %d %s", n, w.Code, w.Body.String())
		}
	}
	if first := request(t, h, http.MethodPost, "/v1/events", validEventJSON); first.Code != http.StatusAccepted {
		t.Fatalf("setup: %d", first.Code)
	}
	w := request(t, h, http.MethodPost, "/v1/events", validEventJSON)
	want := "{\"error\":{\"code\":\"SCHEMA_INVALID\",\"message\":\"request does not match the contract\",\"retryable\":false,\"details\":{}}}\n"
	if w.Code != http.StatusBadRequest || w.Body.String() != want {
		t.Fatalf("duplicate: %d %s", w.Code, w.Body.String())
	}
}

type fakeRepository struct {
	list   contracts.IncidentListResponse
	detail contracts.IncidentDetailResponse
	err    error
	query  contracts.IncidentListQuery
}

func (f *fakeRepository) IngestEvent(context.Context, contracts.ExecutionEvent) error { return f.err }
func (f *fakeRepository) ListIncidents(_ context.Context, q contracts.IncidentListQuery) (contracts.IncidentListResponse, error) {
	f.query = q
	return f.list, f.err
}
func (f *fakeRepository) GetIncidentDetail(context.Context, string) (contracts.IncidentDetailResponse, error) {
	return f.detail, f.err
}

func TestListExactShapeAndQuery(t *testing.T) {
	f := &fakeRepository{list: contracts.IncidentListResponse{Items: []contracts.Incident{}, NextCursor: "next"}}
	w := request(t, handler(f), http.MethodGet, "/v1/incidents?status=READY&limit=1&cursor=cursor", "")
	if w.Code != 200 || w.Body.String() != "{\"items\":[],\"next_cursor\":\"next\"}\n" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if f.query.Status != contracts.IncidentReady || f.query.Cursor != "cursor" || f.query.Limit == nil || *f.query.Limit != 1 {
		t.Fatalf("query: %#v", f.query)
	}
}

func TestListRejectsInvalidQuery(t *testing.T) {
	for _, target := range []string{"/v1/incidents?status=NOPE", "/v1/incidents?limit=0", "/v1/incidents?limit=101", "/v1/incidents?limit=x"} {
		w := request(t, handler(&fakeRepository{}), http.MethodGet, target, "")
		if w.Code != 400 {
			t.Fatalf("%s: %d", target, w.Code)
		}
	}
}

func TestMissingDetailExactEnvelope(t *testing.T) {
	w := request(t, handler(&fakeRepository{err: core.ErrNotFound}), http.MethodGet, "/v1/incidents/missing", "")
	want := "{\"error\":{\"code\":\"SCHEMA_INVALID\",\"message\":\"resource not found\",\"retryable\":false,\"details\":{}}}\n"
	if w.Code != 404 || w.Body.String() != want {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestIncidentDetailUsesTimelineIndexOrder(t *testing.T) {
	store := core.NewStore()
	ctx := context.Background()
	for _, raw := range []string{
		strings.Replace(validEventJSON, `"event_id":"e1"`, `"event_id":"e1"`, 1),
		strings.Replace(validEventJSON, `"event_id":"e1"`, `"event_id":"e2"`, 1),
	} {
		var event contracts.ExecutionEvent
		if err := contracts.DecodeStrict(strings.NewReader(raw), &event); err != nil {
			t.Fatal(err)
		}
		if err := store.IngestEvent(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	incident := contracts.Incident{SchemaVersion: "1.0", IncidentID: "inc", Status: contracts.IncidentReady, FailureOracle: contracts.FailureOracleRef{ID: "oracle", Version: "1.0.0"}, SystemPack: contracts.SystemPackRef{ID: "pack", Version: "1.0.0", InterfaceVersion: "1.0"}, TraceID: "trace", ExecutionID: "exec", DetectedAt: "2026-08-29T10:32:02Z", Summary: "summary", EvidenceEventIDs: []string{"e1"}, GraphID: "graph", SanitizationStatus: contracts.SanitizationPass}
	graph := contracts.ExecutionGraph{SchemaVersion: "1.0", GraphID: "graph", IncidentID: "inc", OrderingPolicyVersion: "1.0", Nodes: []contracts.GraphNode{{EventID: "e2", TimelineIndex: 0}, {EventID: "e1", TimelineIndex: 1}}, Edges: []contracts.GraphEdge{}}
	if err := store.PutIncident(ctx, incident, graph); err != nil {
		t.Fatal(err)
	}

	w := request(t, handler(store), http.MethodGet, "/v1/incidents/inc", "")
	if w.Code != http.StatusOK || strings.Index(w.Body.String(), `"event_id":"e2"`) > strings.Index(w.Body.String(), `"event_id":"e1"`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestInternalReadDoesNotLeak(t *testing.T) {
	w := request(t, handler(&fakeRepository{err: errors.New("password=secret database exploded")}), http.MethodGet, "/v1/incidents", "")
	want := "{\"error\":{\"code\":\"INTERNAL_FAILURE\",\"message\":\"internal failure\",\"retryable\":false,\"details\":{}}}\n"
	if w.Code != 500 || w.Body.String() != want || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestLifecycleAndSafetyMapperUseFrozenCodes(t *testing.T) {
	for _, tc := range []struct {
		err    error
		code   contracts.ErrorCode
		status int
	}{
		{core.ErrInvalidLifecycle, contracts.SchemaInvalid, 409},
		{errors.New("blocked"), contracts.DestinationBlocked, 422},
	} {
		w := httptest.NewRecorder()
		writeMappedError(w, tc.err, tc.code)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), `"code":"`+string(tc.code)+`"`) || !strings.Contains(w.Body.String(), `"details":{}`) {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
}
