CREATE TABLE IF NOT EXISTS execution_events (
  event_id TEXT PRIMARY KEY CHECK (event_id <> ''),
  execution_id TEXT NOT NULL CHECK (execution_id <> ''),
  trace_id TEXT NOT NULL CHECK (trace_id <> ''),
  replay_run_id TEXT,
  payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object' AND payload->>'event_id' = event_id),
  occurred_at TIMESTAMPTZ NOT NULL,
  sequence BIGINT NOT NULL CHECK (sequence >= 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS execution_events_execution_idx ON execution_events (execution_id, occurred_at, event_id);
CREATE INDEX IF NOT EXISTS execution_events_trace_idx ON execution_events (trace_id);
CREATE INDEX IF NOT EXISTS execution_events_replay_run_idx ON execution_events (replay_run_id) WHERE replay_run_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS incidents (
  incident_id TEXT PRIMARY KEY CHECK (incident_id <> ''),
  status TEXT NOT NULL CHECK (status IN ('DETECTED', 'READY', 'BLOCKED')),
  detected_at TIMESTAMPTZ NOT NULL,
  payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object' AND payload->>'incident_id' = incident_id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS incidents_list_idx ON incidents (detected_at DESC, incident_id DESC);
CREATE INDEX IF NOT EXISTS incidents_status_list_idx ON incidents (status, detected_at DESC, incident_id DESC);

CREATE TABLE IF NOT EXISTS execution_graphs (
  graph_id TEXT PRIMARY KEY CHECK (graph_id <> ''),
  incident_id TEXT NOT NULL UNIQUE REFERENCES incidents(incident_id) ON DELETE CASCADE,
  payload JSONB NOT NULL CHECK (
    jsonb_typeof(payload) = 'object'
    AND payload->>'graph_id' = graph_id
    AND payload->>'incident_id' = incident_id
  )
);

CREATE TABLE IF NOT EXISTS replay_capsules (
  capsule_id TEXT PRIMARY KEY CHECK (capsule_id <> ''),
  incident_id TEXT NOT NULL REFERENCES incidents(incident_id),
  created_at TIMESTAMPTZ NOT NULL,
  payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object' AND payload->>'capsule_id' = capsule_id)
);
CREATE INDEX IF NOT EXISTS replay_capsules_incident_idx ON replay_capsules (incident_id, created_at DESC);

CREATE TABLE IF NOT EXISTS replay_runs (
  run_id TEXT PRIMARY KEY CHECK (run_id <> ''),
  capsule_id TEXT NOT NULL REFERENCES replay_capsules(capsule_id),
  status TEXT NOT NULL CHECK (status IN ('CREATED', 'VALIDATING', 'RUNNING', 'COMPLETED', 'FAILED', 'BLOCKED')),
  outcome TEXT CHECK (outcome IN ('REPRODUCED', 'NOT_REPRODUCED', 'MITIGATED', 'UNCHANGED', 'INCONCLUSIVE')),
  payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object' AND payload->>'run_id' = run_id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK ((status = 'COMPLETED') = (outcome IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS replay_runs_capsule_idx ON replay_runs (capsule_id, created_at DESC);
CREATE INDEX IF NOT EXISTS replay_runs_status_idx ON replay_runs (status) WHERE status IN ('CREATED', 'VALIDATING', 'RUNNING');

CREATE TABLE IF NOT EXISTS replay_diffs (
  diff_id TEXT PRIMARY KEY CHECK (diff_id <> ''),
  baseline_run_id TEXT NOT NULL REFERENCES replay_runs(run_id),
  comparison_run_id TEXT NOT NULL REFERENCES replay_runs(run_id),
  payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object' AND payload->>'diff_id' = diff_id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (baseline_run_id <> comparison_run_id),
  UNIQUE (baseline_run_id, comparison_run_id)
);
CREATE INDEX IF NOT EXISTS replay_diffs_comparison_idx ON replay_diffs (comparison_run_id);
