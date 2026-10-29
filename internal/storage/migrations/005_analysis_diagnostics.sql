ALTER TABLE analysis_runs ADD COLUMN reason_code TEXT NOT NULL DEFAULT '';
ALTER TABLE analysis_runs ADD COLUMN missing_requirement_ids JSONB NOT NULL DEFAULT '[]'::jsonb;
