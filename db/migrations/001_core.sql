CREATE TABLE IF NOT EXISTS execution_events (
  event_id TEXT PRIMARY KEY, execution_id TEXT NOT NULL, trace_id TEXT NOT NULL,
  replay_run_id TEXT, payload JSONB NOT NULL, occurred_at TIMESTAMPTZ NOT NULL,
  sequence BIGINT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS incidents (
  incident_id TEXT PRIMARY KEY, status TEXT NOT NULL, payload JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS execution_graphs (
  graph_id TEXT PRIMARY KEY, incident_id TEXT NOT NULL REFERENCES incidents(incident_id), payload JSONB NOT NULL
);
CREATE TABLE IF NOT EXISTS replay_capsules (
  capsule_id TEXT PRIMARY KEY, incident_id TEXT NOT NULL REFERENCES incidents(incident_id), payload JSONB NOT NULL
);
CREATE TABLE IF NOT EXISTS replay_runs (
  run_id TEXT PRIMARY KEY, capsule_id TEXT NOT NULL REFERENCES replay_capsules(capsule_id), status TEXT NOT NULL, outcome TEXT, payload JSONB NOT NULL
);
CREATE TABLE IF NOT EXISTS replay_diffs (
  diff_id TEXT PRIMARY KEY, baseline_run_id TEXT NOT NULL REFERENCES replay_runs(run_id), comparison_run_id TEXT NOT NULL REFERENCES replay_runs(run_id), payload JSONB NOT NULL
);
