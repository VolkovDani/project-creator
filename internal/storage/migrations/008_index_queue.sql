ALTER TABLE requirement_index DROP CONSTRAINT requirement_index_status_check;
ALTER TABLE requirement_index ADD CONSTRAINT requirement_index_status_check CHECK(status IN ('pending','queued','running','ready','failed'));
ALTER TABLE requirement_index ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0 CHECK(attempts >= 0);
ALTER TABLE requirement_index ADD COLUMN available_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp();

CREATE TABLE embedding_profiles (
 workspace TEXT PRIMARY KEY,
 profile TEXT NOT NULL,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

-- The outbox already serializes write transactions. Acquire it before row locks,
-- including CRUD, so activation, queue publication and updates use one order.
CREATE FUNCTION lock_index_queue_writes() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
 PERFORM pg_advisory_xact_lock(724183092);
 RETURN NULL;
END;
$$;
CREATE TRIGGER index_queue_write_order BEFORE INSERT OR UPDATE OR DELETE ON projects
 FOR EACH STATEMENT EXECUTE FUNCTION lock_index_queue_writes();
CREATE TRIGGER index_queue_write_order BEFORE INSERT OR UPDATE OR DELETE ON requirements
 FOR EACH STATEMENT EXECUTE FUNCTION lock_index_queue_writes();
CREATE TRIGGER index_queue_write_order BEFORE INSERT OR UPDATE OR DELETE ON requirement_index
 FOR EACH STATEMENT EXECUTE FUNCTION lock_index_queue_writes();
CREATE TRIGGER index_queue_write_order BEFORE INSERT OR UPDATE OR DELETE ON embedding_profiles
 FOR EACH STATEMENT EXECUTE FUNCTION lock_index_queue_writes();

CREATE FUNCTION enqueue_requirement_index() RETURNS trigger
LANGUAGE plpgsql SET search_path FROM CURRENT AS $$
DECLARE active_profile TEXT;
BEGIN
 SELECT profile INTO active_profile FROM embedding_profiles WHERE workspace=NEW.workspace;
 IF active_profile IS NULL OR active_profile='' THEN RETURN NULL; END IF;
 INSERT INTO requirement_index(workspace,project_id,requirement_id,profile,version,status,attempt,lease_until,error_code,attempts,available_at)
 VALUES(NEW.workspace,NEW.project_id,NEW.id,active_profile,NEW.version,'queued','',clock_timestamp(),'',0,clock_timestamp())
 ON CONFLICT(workspace,project_id,requirement_id,profile) DO UPDATE
 SET version=excluded.version,status='queued',attempt='',lease_until=clock_timestamp(),error_code='',attempts=0,available_at=clock_timestamp();
 RETURN NULL;
END;
$$;
CREATE TRIGGER z_requirement_index_queue AFTER INSERT OR UPDATE ON requirements
 FOR EACH ROW EXECUTE FUNCTION enqueue_requirement_index();
