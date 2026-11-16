-- A single durable request belongs to an expected run. It is consumed together
-- with creation of its successor, under the project's authorization lock.
CREATE TABLE analysis_replacements (
 run_id TEXT PRIMARY KEY REFERENCES analysis_runs(id) ON DELETE CASCADE,
 mode TEXT NOT NULL CHECK (mode IN ('all','related')),
 threshold DOUBLE PRECISION NOT NULL CHECK (threshold >= 0 AND threshold <= 1),
 strategy TEXT NOT NULL CHECK (strategy IN ('wait','cancel')),
 successor_id TEXT REFERENCES analysis_runs(id) ON DELETE CASCADE
);

ALTER TABLE analysis_runs DROP CONSTRAINT analysis_runs_stage_check;
ALTER TABLE analysis_runs ADD CONSTRAINT analysis_runs_stage_check CHECK(stage IN ('checking_index','finding_pairs','comparing','finished','waiting_current_pair'));
