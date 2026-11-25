ALTER TABLE analysis_runs ADD COLUMN glossary_revision BIGINT NOT NULL DEFAULT 0 CHECK (glossary_revision>=0);

ALTER TABLE analysis_pairs
 ADD COLUMN glossary_context JSONB NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(glossary_context)='array'),
 ADD COLUMN glossary_stale BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX analysis_pairs_right_source ON analysis_pairs(run_id,(right_source->>'id'));
CREATE INDEX analysis_pairs_glossary_stale ON analysis_pairs(run_id) WHERE glossary_stale;

ALTER TABLE external_proposals
 ADD COLUMN glossary_sources JSONB NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(glossary_sources)='array');
