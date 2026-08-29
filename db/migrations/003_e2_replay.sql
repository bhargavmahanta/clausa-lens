-- E2 replay workflow support. Additive only; never edit 001/002.
-- These are read-path and linkage indexes that the E2 replay worker,
-- baseline authorization, and diff analyzer rely on beyond A2.

-- Support diff retrieval by baseline referenced run.
CREATE INDEX IF NOT EXISTS replay_diffs_baseline_idx ON replay_diffs (baseline_run_id);

-- Support authorizing a what-if run by finding its capsule's baseline runs
-- quickly. run_type lives inside the JSON payload.
CREATE INDEX IF NOT EXISTS replay_runs_payload_run_type_idx
  ON replay_runs ((payload->>'run_type'));

-- Guard the run payload's immutable identity against its relational row.
-- The payload's run_id and capsule_id must not drift from the columns that
-- drive lifecycle and authorization decisions.
DO $upgrade$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint
    WHERE conrelid = 'replay_runs'::regclass AND conname = 'replay_runs_payload_identity_e2') THEN
    ALTER TABLE replay_runs
      ADD CONSTRAINT replay_runs_payload_identity_e2
      CHECK (
        jsonb_typeof(payload) = 'object'
        AND payload->>'run_id' = run_id
        AND payload->>'capsule_id' = capsule_id
      );
  END IF;
END
$upgrade$;
