CREATE TABLE process_graph_cache (
 workspace TEXT NOT NULL,
 project_id TEXT NOT NULL,
 profile TEXT NOT NULL,
 format_version INTEGER NOT NULL,
 snapshot_key TEXT NOT NULL,
 reusable BOOLEAN NOT NULL,
 term_id TEXT NOT NULL,
 keyword TEXT NOT NULL,
 result JSONB NOT NULL,
 latest_order BIGINT GENERATED ALWAYS AS IDENTITY,
 PRIMARY KEY (workspace,project_id,profile,format_version,snapshot_key,reusable),
 FOREIGN KEY (workspace,project_id) REFERENCES projects(workspace,id) ON DELETE CASCADE
);
CREATE INDEX process_graph_cache_latest
 ON process_graph_cache(workspace,project_id,profile,format_version,latest_order DESC);
CREATE INDEX process_graph_cache_project ON process_graph_cache(project_id);

CREATE FUNCTION record_process_graph_cache_clear() RETURNS trigger
LANGUAGE plpgsql SET search_path FROM CURRENT AS $$
BEGIN
 IF NOT EXISTS (SELECT 1 FROM cleared_graphs) THEN RETURN NULL; END IF;
 -- Keep allocation and commit in the same order as other outbox writers.
 PERFORM pg_advisory_xact_lock(724183092);
 INSERT INTO cache_events(id,workspace,keys)
 SELECT md5(random()::text || clock_timestamp()::text || txid_current()::text),workspace,
  ARRAY['project:' || project_id || ':process-graph']
 FROM cleared_graphs GROUP BY workspace,project_id ORDER BY workspace,project_id;
 DELETE FROM cache_events WHERE seq < (SELECT seq FROM cache_events ORDER BY seq DESC OFFSET 999 LIMIT 1);
 RETURN NULL;
END;
$$;
CREATE TRIGGER process_graph_cache_clear_event AFTER DELETE ON process_graph_cache
 REFERENCING OLD TABLE AS cleared_graphs
 FOR EACH STATEMENT EXECUTE FUNCTION record_process_graph_cache_clear();
