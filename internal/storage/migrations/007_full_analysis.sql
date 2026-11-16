-- Queue and immutable source snapshots are separate rows, never a quadratic JSONB array.
ALTER TABLE analysis_runs DROP CONSTRAINT analysis_runs_total_pairs_check;
ALTER TABLE analysis_runs DROP CONSTRAINT analysis_runs_processed_pairs_check;
ALTER TABLE analysis_runs ADD CONSTRAINT analysis_runs_total_pairs_check CHECK(total_pairs >= 0);
ALTER TABLE analysis_runs ADD CONSTRAINT analysis_runs_processed_pairs_check CHECK(processed_pairs >= 0);
ALTER TABLE analysis_runs ADD COLUMN generator_provenance TEXT NOT NULL DEFAULT '';
ALTER TABLE analysis_runs ADD COLUMN pair_cursor_left TEXT NOT NULL DEFAULT '';
ALTER TABLE analysis_runs ADD COLUMN pair_cursor_right TEXT NOT NULL DEFAULT '';
ALTER TABLE analysis_runs ADD COLUMN full_queue BOOLEAN NOT NULL DEFAULT false;
CREATE TABLE analysis_sources (
 run_id TEXT NOT NULL REFERENCES analysis_runs(id),
 requirement_id TEXT NOT NULL,
 source JSONB NOT NULL,
 PRIMARY KEY(run_id,requirement_id)
);
CREATE TABLE analysis_pairs (
 id TEXT PRIMARY KEY,
 run_id TEXT NOT NULL REFERENCES analysis_runs(id),
 workspace TEXT NOT NULL,
 project_id TEXT NOT NULL,
 position INTEGER NOT NULL,
 left_source JSONB NOT NULL,
 right_source JSONB NOT NULL,
 generator_profile TEXT NOT NULL,
 cache_key TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('pending','running','completed','failed')),
 verdict TEXT NOT NULL DEFAULT '' CHECK(verdict IN ('','collision','compatible','insufficient','duplicate')),
 explanation TEXT NOT NULL DEFAULT '',
 error_code TEXT NOT NULL DEFAULT '',
 reused_from TEXT NOT NULL DEFAULT '',
 checked_at TIMESTAMPTZ,
 UNIQUE(run_id,position),
 FOREIGN KEY(workspace,project_id) REFERENCES projects(workspace,id)
);
CREATE UNIQUE INDEX analysis_pairs_identity ON analysis_pairs(run_id,(left_source->>'id'),(right_source->>'id'));
CREATE INDEX analysis_pairs_page ON analysis_pairs(run_id,position);
CREATE INDEX analysis_pairs_pending ON analysis_pairs(run_id,position) WHERE status='pending';
CREATE INDEX analysis_pairs_cache ON analysis_pairs(workspace,project_id,generator_profile,cache_key) WHERE status='completed' AND reused_from='';
INSERT INTO analysis_sources SELECT r.id,s->>'id',s FROM analysis_runs r CROSS JOIN LATERAL jsonb_array_elements(r.sources) s;
INSERT INTO analysis_pairs(id,run_id,workspace,project_id,position,left_source,right_source,generator_profile,cache_key,status,verdict,explanation,error_code,checked_at)
 SELECT p->>'id',r.id,r.workspace,r.project_id,(p->>'position')::integer,p->'left',p->'right',r.generator_profile,'',p->>'status',coalesce(p->>'verdict',''),coalesce(p->>'explanation',''),coalesce(p->>'errorCode',''),CASE WHEN p->>'status'='completed' THEN coalesce(r.completed_at,r.created_at) END
 FROM analysis_runs r CROSS JOIN LATERAL jsonb_array_elements(r.pairs) p;
-- Migrated results retain their historical profile. Their old incomplete profile
-- is not eligible for new cache keys, so migration never invents provenance.
UPDATE analysis_runs SET sources='[]',pairs='[]',findings='[]';

ALTER TABLE analysis_runs ADD COLUMN mode TEXT NOT NULL DEFAULT 'all' CHECK(mode IN ('all','related'));
ALTER TABLE analysis_runs ADD COLUMN similarity_threshold DOUBLE PRECISION NOT NULL DEFAULT 0.65 CHECK(similarity_threshold BETWEEN 0 AND 1);
ALTER TABLE analysis_runs ADD COLUMN selection_profile TEXT NOT NULL DEFAULT 'legacy';
ALTER TABLE analysis_runs ADD COLUMN possible_pairs INTEGER;
ALTER TABLE analysis_pairs ADD COLUMN selection_reasons JSONB NOT NULL DEFAULT '[]';
ALTER TABLE analysis_pairs ADD COLUMN similarity_score DOUBLE PRECISION;
ALTER TABLE analysis_pairs ADD COLUMN result_origin TEXT NOT NULL DEFAULT 'model';
-- Serialize source edits and result commits in the same project scope. Holding
-- this row only during SQL transactions never blocks on a model call.
CREATE FUNCTION lock_analysis_source_scope() RETURNS trigger
LANGUAGE plpgsql SET search_path FROM CURRENT AS $$
BEGIN
 IF TG_OP='DELETE' THEN
  PERFORM 1 FROM projects WHERE workspace=OLD.workspace AND id=OLD.project_id FOR UPDATE;
 ELSE
  PERFORM 1 FROM projects WHERE workspace=NEW.workspace AND id=NEW.project_id FOR UPDATE;
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER requirements_analysis_scope BEFORE INSERT OR UPDATE OR DELETE ON requirements
 FOR EACH ROW EXECUTE FUNCTION lock_analysis_source_scope();
