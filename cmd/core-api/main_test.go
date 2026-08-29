package main

import (
	"github.com/causalens/causalens/internal/core"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEventsEndpointRejectsInvalidPayload(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/events", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	handler(core.NewStore()).ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatalf("got %d", w.Code)
	}
}
func TestIncidentsEndpointReturnsItems(t *testing.T) {
	r := httptest.NewRequest("GET", "/v1/incidents", nil)
	w := httptest.NewRecorder()
	handler(core.NewStore()).ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("got %d", w.Code)
	}
}
