CREATE TABLE summary_profiles (
 workspace TEXT PRIMARY KEY, profile TEXT NOT NULL, provenance TEXT NOT NULL
);
CREATE TABLE requirement_summaries (
 workspace TEXT NOT NULL, project_id TEXT NOT NULL, requirement_id TEXT NOT NULL,
 version INTEGER NOT NULL CHECK(version>0), profile TEXT NOT NULL, provenance TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('queued','running','ready','failed','cancelled','stale')),
 text TEXT NOT NULL DEFAULT '', error_code TEXT NOT NULL DEFAULT '', attempt TEXT NOT NULL DEFAULT '',
 lease_until TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(), created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(workspace,project_id,requirement_id,version,profile),
 FOREIGN KEY(workspace,project_id,requirement_id) REFERENCES requirements(workspace,project_id,id) ON DELETE CASCADE,
 CHECK(status<>'ready' OR (octet_length(text) BETWEEN 1 AND 350))
);
CREATE INDEX requirement_summaries_queue ON requirement_summaries(status,created_at) WHERE status='queued';
CREATE TRIGGER summary_profiles_write_order BEFORE INSERT OR UPDATE OR DELETE ON summary_profiles
 FOR EACH STATEMENT EXECUTE FUNCTION lock_index_queue_writes();
CREATE TRIGGER requirement_summaries_write_order BEFORE INSERT OR UPDATE OR DELETE ON requirement_summaries
 FOR EACH STATEMENT EXECUTE FUNCTION lock_index_queue_writes();
CREATE FUNCTION enqueue_requirement_summary() RETURNS trigger LANGUAGE plpgsql SET search_path FROM CURRENT AS $$
DECLARE active summary_profiles%ROWTYPE;
BEGIN
 SELECT * INTO active FROM summary_profiles WHERE workspace=NEW.workspace;
 UPDATE requirement_summaries SET status='stale',attempt='' WHERE workspace=NEW.workspace AND project_id=NEW.project_id AND requirement_id=NEW.id AND version<>NEW.version AND status IN ('queued','running');
 IF active.profile IS NOT NULL AND active.profile<>'' THEN
 INSERT INTO requirement_summaries(workspace,project_id,requirement_id,version,profile,provenance,status)
 VALUES(NEW.workspace,NEW.project_id,NEW.id,NEW.version,active.profile,active.provenance,'queued') ON CONFLICT DO NOTHING;
 END IF;
 RETURN NULL;
END;$$;
CREATE TRIGGER z_requirement_summary_queue AFTER INSERT OR UPDATE ON requirements
 FOR EACH ROW EXECUTE FUNCTION enqueue_requirement_summary();
CREATE FUNCTION record_summary_change() RETURNS trigger LANGUAGE plpgsql SET search_path FROM CURRENT AS $$
DECLARE ws TEXT; pid TEXT;
BEGIN
 PERFORM pg_advisory_xact_lock(724183092);
 IF TG_OP='DELETE' THEN ws:=OLD.workspace;pid:=OLD.project_id; ELSE ws:=NEW.workspace;pid:=NEW.project_id;END IF;
 INSERT INTO cache_events(id,workspace,keys) VALUES(md5(random()::text||clock_timestamp()::text||txid_current()::text),ws,ARRAY['project:'||pid||':summaries']);
 DELETE FROM cache_events WHERE seq < (SELECT seq FROM cache_events ORDER BY seq DESC OFFSET 999 LIMIT 1);
 RETURN NULL;
END;$$;
CREATE TRIGGER requirement_summaries_event AFTER INSERT OR UPDATE OR DELETE ON requirement_summaries
 FOR EACH ROW EXECUTE FUNCTION record_summary_change();
