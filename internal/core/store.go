package core

import (
	"fmt"
	"github.com/causalens/causalens/internal/contracts"
	"sync"
)

type Store struct {
	mu     sync.RWMutex
	events map[string]contracts.ExecutionEvent
}

func NewStore() *Store { return &Store{events: map[string]contracts.ExecutionEvent{}} }
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
