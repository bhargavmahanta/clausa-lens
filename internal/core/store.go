package core

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/causalens/causalens/internal/contracts"
	graphpkg "github.com/causalens/causalens/internal/graph"
)

// Repository is the A2 persistence seam used by the Core HTTP API.
type Repository interface {
	IngestEvent(context.Context, contracts.ExecutionEvent) error
	ListIncidents(context.Context, contracts.IncidentListQuery) (contracts.IncidentListResponse, error)
	GetIncidentDetail(context.Context, string) (contracts.IncidentDetailResponse, error)
}

type Store struct {
	mu        sync.RWMutex
	events    map[string]contracts.ExecutionEvent
	incidents map[string]contracts.Incident
	graphs    map[string]contracts.ExecutionGraph
}

func NewStore() *Store {
	return &Store{events: map[string]contracts.ExecutionEvent{}, incidents: map[string]contracts.Incident{}, graphs: map[string]contracts.ExecutionGraph{}}
}

func (s *Store) IngestEvent(ctx context.Context, e contracts.ExecutionEvent) error {
	if err := ctx.Err(); err != nil {
		return ErrInternal
	}
	if err := e.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.events[e.EventID]; exists {
		return ErrConflict
	}
	s.events[e.EventID] = e
	return nil
}

// PutIncident atomically validates and stores an incident and its graph.
func (s *Store) PutIncident(ctx context.Context, i contracts.Incident, g contracts.ExecutionGraph) error {
	if err := ctx.Err(); err != nil {
		return ErrInternal
	}
	if err := i.Validate(); err != nil {
		return ErrInternal
	}
	if err := g.Validate(); err != nil || g.IncidentID != i.IncidentID || i.GraphID != g.GraphID {
		return ErrInternal
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.incidents[i.IncidentID]; exists {
		return ErrConflict
	}
	if _, exists := s.graphs[g.GraphID]; exists {
		return ErrConflict
	}
	referenced := make([]contracts.ExecutionEvent, 0, len(g.Nodes))
	nodeEvents := make(map[string]struct{}, len(g.Nodes))
	for _, node := range g.Nodes {
		e, exists := s.events[node.EventID]
		if !exists || e.ExecutionID != i.ExecutionID || e.TraceID != i.TraceID {
			return ErrInternal
		}
		nodeEvents[node.EventID] = struct{}{}
		referenced = append(referenced, e)
	}
	for _, evidenceID := range i.EvidenceEventIDs {
		if _, exists := nodeEvents[evidenceID]; !exists {
			return ErrInternal
		}
	}
	if _, err := graphpkg.Order(referenced, g); err != nil {
		return ErrInternal
	}
	s.incidents[i.IncidentID] = i
	s.graphs[g.GraphID] = g
	return nil
}

func (s *Store) ListIncidents(ctx context.Context, q contracts.IncidentListQuery) (contracts.IncidentListResponse, error) {
	if err := ctx.Err(); err != nil {
		return contracts.IncidentListResponse{}, ErrInternal
	}
	limit := 20
	if q.Limit != nil {
		limit = *q.Limit
	}
	if limit < 1 || limit > 100 {
		return contracts.IncidentListResponse{}, ErrConflict
	}
	var cursor incidentCursor
	if q.Cursor != "" {
		var err error
		cursor, err = decodeCursor(q.Cursor)
		if err != nil {
			return contracts.IncidentListResponse{}, ErrConflict
		}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]contracts.Incident, 0, len(s.incidents))
	for _, i := range s.incidents {
		if q.Status != "" && i.Status != q.Status {
			continue
		}
		detected, err := time.Parse(time.RFC3339Nano, i.DetectedAt)
		if err != nil {
			return contracts.IncidentListResponse{}, ErrInternal
		}
		if q.Cursor != "" && (detected.After(cursor.DetectedAt) || detected.Equal(cursor.DetectedAt) && i.IncidentID >= cursor.IncidentID) {
			continue
		}
		items = append(items, i)
	}
	sort.Slice(items, func(a, b int) bool {
		at, _ := time.Parse(time.RFC3339Nano, items[a].DetectedAt)
		bt, _ := time.Parse(time.RFC3339Nano, items[b].DetectedAt)
		if at.Equal(bt) {
			return items[a].IncidentID > items[b].IncidentID
		}
		return at.After(bt)
	})
	response := contracts.IncidentListResponse{Items: items}
	if len(items) > limit {
		last := items[limit-1]
		detected, _ := time.Parse(time.RFC3339Nano, last.DetectedAt)
		response.Items = items[:limit]
		response.NextCursor = encodeCursor(incidentCursor{DetectedAt: detected, IncidentID: last.IncidentID})
	}
	return response, nil
}

func (s *Store) GetIncidentDetail(ctx context.Context, id string) (contracts.IncidentDetailResponse, error) {
	if err := ctx.Err(); err != nil {
		return contracts.IncidentDetailResponse{}, ErrInternal
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	i, exists := s.incidents[id]
	if !exists {
		return contracts.IncidentDetailResponse{}, ErrNotFound
	}
	g, exists := s.graphs[i.GraphID]
	if !exists || g.IncidentID != i.IncidentID {
		return contracts.IncidentDetailResponse{}, ErrInternal
	}
	events := make([]contracts.ExecutionEvent, 0, len(g.Nodes))
	for _, node := range g.Nodes {
		e, exists := s.events[node.EventID]
		if !exists {
			return contracts.IncidentDetailResponse{}, ErrInternal
		}
		events = append(events, e)
	}
	ordered, err := graphpkg.Order(events, g)
	if err != nil {
		return contracts.IncidentDetailResponse{}, ErrInternal
	}
	return contracts.IncidentDetailResponse{Incident: i, Graph: g, Events: ordered}, nil
}

var _ Repository = (*Store)(nil)
var _ Repository = (*PostgresRepository)(nil)
