-- E2/container hardening: add a lease to replay runs so a worker that crashes
-- after claiming a run (CREATED -> VALIDATING) but before the runtime starts
-- (no started_at on VALIDATING) can be recovered without re-executing it.
--
-- lease_until is owned by the replay worker only. It is set on claim
-- (CREATED -> VALIDATING) and renewed on the RUNNING transition, and an expired
-- lease lets a surviving worker fail (never re-run) the orphaned run. A NULL
-- lease is treated as expired so pre-existing stranded runs are reclaimed once
-- (a single-worker deployment has no concurrent owner).
ALTER TABLE replay_runs ADD COLUMN IF NOT EXISTS lease_until timestamptz;

CREATE INDEX IF NOT EXISTS replay_runs_lease_idx ON replay_runs (status, lease_until);
