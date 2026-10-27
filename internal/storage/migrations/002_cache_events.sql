-- The durable journal is the outbox. Row changes and invalidations commit together.
CREATE TABLE cache_events (
 seq BIGSERIAL PRIMARY KEY,
 id TEXT NOT NULL UNIQUE,
 workspace TEXT NOT NULL,
 keys TEXT[] NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX cache_events_workspace_seq ON cache_events(workspace,seq);
CREATE FUNCTION record_cache_change() RETURNS trigger
LANGUAGE plpgsql SET search_path FROM CURRENT AS $$
DECLARE
 ws TEXT;
 project_id TEXT;
BEGIN
 -- Serialize event allocation through commit: a later sequence cannot commit
 -- before an earlier one and be skipped by a polling cursor.
 PERFORM pg_advisory_xact_lock(724183092);
 IF TG_TABLE_NAME = 'projects' THEN
  IF TG_OP = 'DELETE' THEN ws := OLD.workspace; project_id := OLD.id;
  ELSE ws := NEW.workspace; project_id := NEW.id; END IF;
 ELSE
  IF TG_OP = 'DELETE' THEN ws := OLD.workspace; project_id := OLD.project_id;
  ELSE ws := NEW.workspace; project_id := NEW.project_id; END IF;
 END IF;
 INSERT INTO cache_events(id,workspace,keys)
 VALUES(md5(random()::text || clock_timestamp()::text || txid_current()::text),ws,
 ARRAY['projects','project:' || project_id,'project:' || project_id || ':requirements']);
 -- Bounded history; slow/offline clients detect a missing cursor and reset.
 DELETE FROM cache_events WHERE seq < (SELECT seq FROM cache_events ORDER BY seq DESC OFFSET 999 LIMIT 1);
 RETURN NULL;
END;
$$;
CREATE TRIGGER projects_cache_event AFTER INSERT OR UPDATE OR DELETE ON projects
 FOR EACH ROW EXECUTE FUNCTION record_cache_change();
CREATE TRIGGER requirements_cache_event AFTER INSERT OR UPDATE OR DELETE ON requirements
 FOR EACH ROW EXECUTE FUNCTION record_cache_change();
