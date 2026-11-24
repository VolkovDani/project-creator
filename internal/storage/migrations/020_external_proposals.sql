CREATE TABLE external_proposals (
 id TEXT PRIMARY KEY,
 workspace TEXT NOT NULL,
 project_id TEXT NOT NULL,
 request_id TEXT NOT NULL,
 analysis TEXT NOT NULL,
 limitations TEXT NOT NULL,
 client TEXT NOT NULL,
 model TEXT NOT NULL,
 sharing TEXT NOT NULL CHECK (sharing IN ('local', 'cloud')),
 sources JSONB NOT NULL CHECK (jsonb_array_length(sources) BETWEEN 1 AND 20),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(workspace, project_id, request_id),
 FOREIGN KEY(workspace, project_id) REFERENCES projects(workspace, id) ON DELETE CASCADE
);
CREATE INDEX external_proposals_page ON external_proposals(workspace, project_id, created_at DESC, id DESC);

CREATE FUNCTION record_external_proposal_change() RETURNS trigger
LANGUAGE plpgsql SET search_path FROM CURRENT AS $$
BEGIN
 PERFORM pg_advisory_xact_lock(724183092);
 INSERT INTO cache_events(id, workspace, keys)
 VALUES(md5(random()::text || clock_timestamp()::text || txid_current()::text), NEW.workspace,
 ARRAY['project:' || NEW.project_id || ':external-proposals']);
 DELETE FROM cache_events WHERE seq < (SELECT seq FROM cache_events ORDER BY seq DESC OFFSET 999 LIMIT 1);
 RETURN NULL;
END;
$$;
CREATE TRIGGER external_proposals_cache_event AFTER INSERT ON external_proposals
 FOR EACH ROW EXECUTE FUNCTION record_external_proposal_change();
