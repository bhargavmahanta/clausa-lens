ALTER TABLE incidents ADD COLUMN IF NOT EXISTS detected_at TIMESTAMPTZ;
UPDATE incidents SET detected_at = created_at WHERE detected_at IS NULL;
ALTER TABLE incidents ALTER COLUMN detected_at SET NOT NULL;

ALTER TABLE replay_capsules ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ;
UPDATE replay_capsules SET created_at = now() WHERE created_at IS NULL;
ALTER TABLE replay_capsules ALTER COLUMN created_at SET NOT NULL;

ALTER TABLE replay_runs ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE replay_diffs ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();

DO $upgrade$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'execution_events'::regclass AND conname = 'execution_events_event_id_nonempty') THEN
    ALTER TABLE execution_events ADD CONSTRAINT execution_events_event_id_nonempty CHECK (event_id <> '');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'execution_events'::regclass AND conname = 'execution_events_execution_id_nonempty') THEN
    ALTER TABLE execution_events ADD CONSTRAINT execution_events_execution_id_nonempty CHECK (execution_id <> '');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'execution_events'::regclass AND conname = 'execution_events_trace_id_nonempty') THEN
    ALTER TABLE execution_events ADD CONSTRAINT execution_events_trace_id_nonempty CHECK (trace_id <> '');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'execution_events'::regclass AND conname = 'execution_events_payload_identity') THEN
    ALTER TABLE execution_events ADD CONSTRAINT execution_events_payload_identity CHECK (jsonb_typeof(payload) = 'object' AND payload->>'event_id' = event_id);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'execution_events'::regclass AND conname = 'execution_events_sequence_nonnegative') THEN
    ALTER TABLE execution_events ADD CONSTRAINT execution_events_sequence_nonnegative CHECK (sequence >= 0);
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'incidents'::regclass AND conname = 'incidents_incident_id_nonempty') THEN
    ALTER TABLE incidents ADD CONSTRAINT incidents_incident_id_nonempty CHECK (incident_id <> '');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'incidents'::regclass AND conname = 'incidents_status_valid') THEN
    ALTER TABLE incidents ADD CONSTRAINT incidents_status_valid CHECK (status IN ('DETECTED', 'READY', 'BLOCKED'));
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'incidents'::regclass AND conname = 'incidents_payload_identity') THEN
    ALTER TABLE incidents ADD CONSTRAINT incidents_payload_identity CHECK (jsonb_typeof(payload) = 'object' AND payload->>'incident_id' = incident_id);
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'execution_graphs'::regclass AND conname = 'execution_graphs_graph_id_nonempty') THEN
    ALTER TABLE execution_graphs ADD CONSTRAINT execution_graphs_graph_id_nonempty CHECK (graph_id <> '');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'execution_graphs'::regclass AND conname = 'execution_graphs_payload_identity') THEN
    ALTER TABLE execution_graphs ADD CONSTRAINT execution_graphs_payload_identity CHECK (jsonb_typeof(payload) = 'object' AND payload->>'graph_id' = graph_id AND payload->>'incident_id' = incident_id);
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'replay_capsules'::regclass AND conname = 'replay_capsules_capsule_id_nonempty') THEN
    ALTER TABLE replay_capsules ADD CONSTRAINT replay_capsules_capsule_id_nonempty CHECK (capsule_id <> '');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'replay_capsules'::regclass AND conname = 'replay_capsules_payload_identity') THEN
    ALTER TABLE replay_capsules ADD CONSTRAINT replay_capsules_payload_identity CHECK (jsonb_typeof(payload) = 'object' AND payload->>'capsule_id' = capsule_id);
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'replay_runs'::regclass AND conname = 'replay_runs_run_id_nonempty') THEN
    ALTER TABLE replay_runs ADD CONSTRAINT replay_runs_run_id_nonempty CHECK (run_id <> '');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'replay_runs'::regclass AND conname = 'replay_runs_status_valid') THEN
    ALTER TABLE replay_runs ADD CONSTRAINT replay_runs_status_valid CHECK (status IN ('CREATED', 'VALIDATING', 'RUNNING', 'COMPLETED', 'FAILED', 'BLOCKED'));
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'replay_runs'::regclass AND conname = 'replay_runs_outcome_valid') THEN
    ALTER TABLE replay_runs ADD CONSTRAINT replay_runs_outcome_valid CHECK (outcome IN ('REPRODUCED', 'NOT_REPRODUCED', 'MITIGATED', 'UNCHANGED', 'INCONCLUSIVE'));
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'replay_runs'::regclass AND conname = 'replay_runs_payload_identity') THEN
    ALTER TABLE replay_runs ADD CONSTRAINT replay_runs_payload_identity CHECK (jsonb_typeof(payload) = 'object' AND payload->>'run_id' = run_id);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'replay_runs'::regclass AND conname = 'replay_runs_completed_outcome') THEN
    ALTER TABLE replay_runs ADD CONSTRAINT replay_runs_completed_outcome CHECK ((status = 'COMPLETED') = (outcome IS NOT NULL));
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'replay_diffs'::regclass AND conname = 'replay_diffs_diff_id_nonempty') THEN
    ALTER TABLE replay_diffs ADD CONSTRAINT replay_diffs_diff_id_nonempty CHECK (diff_id <> '');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'replay_diffs'::regclass AND conname = 'replay_diffs_payload_identity') THEN
    ALTER TABLE replay_diffs ADD CONSTRAINT replay_diffs_payload_identity CHECK (jsonb_typeof(payload) = 'object' AND payload->>'diff_id' = diff_id);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'replay_diffs'::regclass AND conname = 'replay_diffs_distinct_runs') THEN
    ALTER TABLE replay_diffs ADD CONSTRAINT replay_diffs_distinct_runs CHECK (baseline_run_id <> comparison_run_id);
  END IF;
END
$upgrade$;

DO $upgrade$
DECLARE
  fk_name name;
BEGIN
  SELECT conname INTO fk_name FROM pg_constraint
  WHERE conrelid = 'execution_graphs'::regclass AND contype = 'f' AND confrelid = 'incidents'::regclass AND confdeltype <> 'c'
  LIMIT 1;
  IF fk_name IS NOT NULL THEN
    EXECUTE format('ALTER TABLE execution_graphs DROP CONSTRAINT %I', fk_name);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'execution_graphs'::regclass AND contype = 'f' AND confrelid = 'incidents'::regclass AND confdeltype = 'c') THEN
    ALTER TABLE execution_graphs ADD CONSTRAINT execution_graphs_incident_id_fkey FOREIGN KEY (incident_id) REFERENCES incidents(incident_id) ON DELETE CASCADE;
  END IF;
END
$upgrade$;

CREATE INDEX IF NOT EXISTS execution_events_execution_idx ON execution_events (execution_id, occurred_at, event_id);
CREATE INDEX IF NOT EXISTS execution_events_trace_idx ON execution_events (trace_id);
CREATE INDEX IF NOT EXISTS execution_events_replay_run_idx ON execution_events (replay_run_id) WHERE replay_run_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS incidents_list_idx ON incidents (detected_at DESC, incident_id DESC);
CREATE INDEX IF NOT EXISTS incidents_status_list_idx ON incidents (status, detected_at DESC, incident_id DESC);
CREATE UNIQUE INDEX IF NOT EXISTS execution_graphs_incident_idx ON execution_graphs (incident_id);
CREATE INDEX IF NOT EXISTS replay_capsules_incident_idx ON replay_capsules (incident_id, created_at DESC);
CREATE INDEX IF NOT EXISTS replay_runs_capsule_idx ON replay_runs (capsule_id, created_at DESC);
CREATE INDEX IF NOT EXISTS replay_runs_status_idx ON replay_runs (status) WHERE status IN ('CREATED', 'VALIDATING', 'RUNNING');
CREATE UNIQUE INDEX IF NOT EXISTS replay_diffs_runs_idx ON replay_diffs (baseline_run_id, comparison_run_id);
CREATE INDEX IF NOT EXISTS replay_diffs_comparison_idx ON replay_diffs (comparison_run_id);
