package core

import (
	"context"
	"reflect"
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
	capsules  map[string]contracts.ReplayCapsule
	runs      map[string]contracts.ReplayRun
	diffs     map[string]contracts.ReplayDiff
}

func NewStore() *Store {
	return &Store{
		events:    map[string]contracts.ExecutionEvent{},
		incidents: map[string]contracts.Incident{},
		graphs:    map[string]contracts.ExecutionGraph{},
		capsules:  map[string]contracts.ReplayCapsule{},
		runs:      map[string]contracts.ReplayRun{},
		diffs:     map[string]contracts.ReplayDiff{},
	}
}

// Reset clears all persisted state and returns how many incidents and runs
// were cleared so the /demo/reset route can report deterministic counts.
func (s *Store) Reset(ctx context.Context) (ResetCounts, error) {
	if err := ctx.Err(); err != nil {
		return ResetCounts{}, ErrInternal
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	counts := ResetCounts{Incidents: len(s.incidents), Runs: len(s.runs)}
	s.events = map[string]contracts.ExecutionEvent{}
	s.incidents = map[string]contracts.Incident{}
	s.graphs = map[string]contracts.ExecutionGraph{}
	s.capsules = map[string]contracts.ReplayCapsule{}
	s.runs = map[string]contracts.ReplayRun{}
	s.diffs = map[string]contracts.ReplayDiff{}
	return counts, nil
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

func (s *Store) PutCapsule(ctx context.Context, c contracts.ReplayCapsule) error {
	if err := ctx.Err(); err != nil {
		return ErrInternal
	}
	if err := c.Validate(); err != nil {
		return ErrInternal
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.capsules[c.CapsuleID]; exists {
		return ErrConflict
	}
	s.capsules[c.CapsuleID] = c
	return nil
}

func (s *Store) GetCapsule(ctx context.Context, id string) (contracts.ReplayCapsule, error) {
	if err := ctx.Err(); err != nil {
		return contracts.ReplayCapsule{}, ErrInternal
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, exists := s.capsules[id]
	if !exists {
		return contracts.ReplayCapsule{}, ErrNotFound
	}
	return c, nil
}

func (s *Store) PutRun(ctx context.Context, run contracts.ReplayRun) error {
	if err := ctx.Err(); err != nil {
		return ErrInternal
	}
	if err := run.Validate(); err != nil {
		return ErrInternal
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.runs[run.RunID]; exists {
		return ErrConflict
	}
	if run.RunType == contracts.RunTypeWhatIf {
		baseline, exists := s.runs[run.BaselineRunID]
		if !exists {
			return ErrInvalidLifecycle
		}
		if baseline.RunID != run.BaselineRunID || baseline.RunType != contracts.RunTypeBaseline || baseline.Status != contracts.ReplayRunCompleted || baseline.Outcome != contracts.ReplayOutcomeReproduced || baseline.IsolationEvidence == nil || baseline.IsolationEvidence.Verdict != contracts.VerdictPass || baseline.CapsuleID != run.CapsuleID || baseline.CapsuleHash != run.CapsuleHash || baseline.Intervention != nil {
			return ErrInvalidLifecycle
		}
	}
	s.runs[run.RunID] = run
	return nil
}

func (s *Store) GetRun(ctx context.Context, id string) (contracts.ReplayRun, error) {
	if err := ctx.Err(); err != nil {
		return contracts.ReplayRun{}, ErrInternal
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, exists := s.runs[id]
	if !exists {
		return contracts.ReplayRun{}, ErrNotFound
	}
	return run, nil
}

func (s *Store) TransitionRun(ctx context.Context, from contracts.ReplayRunStatus, run contracts.ReplayRun) error {
	if err := ctx.Err(); err != nil {
		return ErrInternal
	}
	if !contracts.CanTransitionReplayRun(from, run.Status) || run.Validate() != nil {
		return ErrInvalidLifecycle
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, exists := s.runs[run.RunID]
	if !exists {
		return ErrNotFound
	}
	if stored.Status != from || stored.ExecutionID != run.ExecutionID || stored.CapsuleID != run.CapsuleID || stored.CapsuleHash != run.CapsuleHash || stored.RunType != run.RunType || stored.BaselineRunID != run.BaselineRunID || !reflect.DeepEqual(stored.Intervention, run.Intervention) || stored.TrialNumber != run.TrialNumber {
		return ErrInvalidLifecycle
	}
	s.runs[run.RunID] = run
	return nil
}

func (s *Store) PutDiff(ctx context.Context, d contracts.ReplayDiff) error {
	if err := ctx.Err(); err != nil {
		return ErrInternal
	}
	if err := d.Validate(); err != nil {
		return ErrInternal
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.diffs[d.DiffID]; exists {
		return ErrConflict
	}
	s.diffs[d.DiffID] = d
	return nil
}

func (s *Store) GetDiff(ctx context.Context, id string) (contracts.ReplayDiff, error) {
	if err := ctx.Err(); err != nil {
		return contracts.ReplayDiff{}, ErrInternal
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, exists := s.diffs[id]
	if !exists {
		return contracts.ReplayDiff{}, ErrNotFound
	}
	return d, nil
}

// EventsForRun returns the events of a run in a stable order. It prefers the
// run's observed_event_ids, else the referenced capsule's event_ids.
func (s *Store) EventsForRun(ctx context.Context, run contracts.ReplayRun) ([]contracts.ExecutionEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, ErrInternal
	}
	ids := run.ObservedEventIDs
	if len(ids) == 0 {
		c, err := s.GetCapsule(ctx, run.CapsuleID)
		if err != nil {
			return nil, err
		}
		ids = c.EventIDs
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	events := make([]contracts.ExecutionEvent, 0, len(ids))
	for _, id := range ids {
		e, exists := s.events[id]
		if !exists {
			return nil, ErrNotFound
		}
		events = append(events, e)
	}
	return stableOrder(events), nil
}

// GraphsForRun returns the execution graph referenced by the run's capsule.
func (s *Store) GraphsForRun(ctx context.Context, run contracts.ReplayRun) (contracts.ExecutionGraph, error) {
	c, err := s.GetCapsule(ctx, run.CapsuleID)
	if err != nil {
		return contracts.ExecutionGraph{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	g, exists := s.graphs[c.GraphID]
	if !exists {
		return contracts.ExecutionGraph{}, ErrNotFound
	}
	return g, nil
}

func stableOrder(events []contracts.ExecutionEvent) []contracts.ExecutionEvent {
	sort.SliceStable(events, func(a, b int) bool {
		at, aerr := time.Parse(time.RFC3339Nano, events[a].OccurredAt)
		bt, berr := time.Parse(time.RFC3339Nano, events[b].OccurredAt)
		if aerr == nil && berr == nil && !at.Equal(bt) {
			return at.Before(bt)
		}
		if events[a].Sequence != events[b].Sequence {
			return events[a].Sequence < events[b].Sequence
		}
		return events[a].EventID < events[b].EventID
	})
	return events
}

var _ Repository = (*Store)(nil)
var _ Repository = (*PostgresRepository)(nil)
