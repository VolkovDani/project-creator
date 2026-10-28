CREATE TABLE analysis_runs (
 id TEXT PRIMARY KEY,
 seq BIGSERIAL UNIQUE NOT NULL,
 workspace TEXT NOT NULL,
 project_id TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('running','failed','completed')),
 outcome TEXT NOT NULL DEFAULT '' CHECK(outcome IN ('','possible_collision','insufficient','no_collisions_found')),
 error_code TEXT NOT NULL DEFAULT '',
 generator_profile TEXT NOT NULL,
 embedding_profile TEXT NOT NULL,
 source_signature TEXT NOT NULL,
 sources JSONB NOT NULL,
 findings JSONB NOT NULL DEFAULT '[]',
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 completed_at TIMESTAMPTZ,
 lease_until TIMESTAMPTZ NOT NULL,
 FOREIGN KEY(workspace,project_id) REFERENCES projects(workspace,id)
);
CREATE INDEX analysis_runs_project ON analysis_runs(workspace,project_id,seq DESC);
CREATE UNIQUE INDEX analysis_runs_one_running ON analysis_runs(workspace,project_id) WHERE status='running';
CREATE TRIGGER analysis_runs_cache_event AFTER INSERT OR UPDATE OR DELETE ON analysis_runs
 FOR EACH ROW EXECUTE FUNCTION record_cache_change();
