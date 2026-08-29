package core

import (
	"fmt"
	"github.com/causalens/causalens/internal/contracts"
	"sync"
)

type Store struct {
	mu        sync.RWMutex
	events    map[string]contracts.ExecutionEvent
	incidents map[string]contracts.Incident
	graphs    map[string]contracts.ExecutionGraph
}

func NewStore() *Store {
	return &Store{events: map[string]contracts.ExecutionEvent{}, incidents: map[string]contracts.Incident{}, graphs: map[string]contracts.ExecutionGraph{}}
}
func (s *Store) IngestEvent(e contracts.ExecutionEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := e.Validate(); err != nil {
		return err
	}
	if _, ok := s.events[e.EventID]; ok {
		return fmt.Errorf("duplicate event_id: %s", e.EventID)
	}
	s.events[e.EventID] = e
	return nil
}
func (s *Store) Events() []contracts.ExecutionEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]contracts.ExecutionEvent, 0, len(s.events))
	for _, e := range s.events {
		out = append(out, e)
	}
	return out
}

func (s *Store) PutIncident(i contracts.Incident, g contracts.ExecutionGraph) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i.IncidentID == "" || i.SchemaVersion != "1.0" {
		return fmt.Errorf("invalid incident")
	}
	if _, ok := s.incidents[i.IncidentID]; ok {
		return fmt.Errorf("duplicate incident_id: %s", i.IncidentID)
	}
	s.incidents[i.IncidentID] = i
	s.graphs[g.GraphID] = g
	return nil
}
func (s *Store) ListIncidents() []contracts.Incident {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]contracts.Incident, 0, len(s.incidents))
	for _, i := range s.incidents {
		out = append(out, i)
	}
	return out
}
func (s *Store) Incident(id string) (contracts.Incident, contracts.ExecutionGraph, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	i, ok := s.incidents[id]
	if !ok {
		return contracts.Incident{}, contracts.ExecutionGraph{}, false
	}
	return i, s.graphs[i.GraphID], true
}
