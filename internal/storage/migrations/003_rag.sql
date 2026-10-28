ALTER TABLE requirements ADD CONSTRAINT requirements_scope_id UNIQUE(workspace,project_id,id);
CREATE TABLE requirement_index (
 workspace TEXT NOT NULL,
 project_id TEXT NOT NULL,
 requirement_id TEXT NOT NULL,
 profile TEXT NOT NULL,
 version INTEGER NOT NULL CHECK(version>0),
 status TEXT NOT NULL CHECK(status IN ('pending','ready','failed')),
 error_code TEXT NOT NULL DEFAULT '',
 attempt TEXT NOT NULL,
 lease_until TIMESTAMPTZ NOT NULL,
 PRIMARY KEY(workspace,project_id,requirement_id,profile),
 FOREIGN KEY(workspace,project_id,requirement_id) REFERENCES requirements(workspace,project_id,id) ON DELETE CASCADE
);
CREATE TABLE requirement_chunks (
 workspace TEXT NOT NULL,
 project_id TEXT NOT NULL,
 requirement_id TEXT NOT NULL,
 profile TEXT NOT NULL,
 version INTEGER NOT NULL CHECK(version>0),
 ordinal INTEGER NOT NULL CHECK(ordinal>=0),
 start_offset INTEGER NOT NULL CHECK(start_offset>=0),
 end_offset INTEGER NOT NULL CHECK(end_offset>start_offset),
 text TEXT NOT NULL,
 embedding public.vector(1024) NOT NULL,
 PRIMARY KEY(workspace,project_id,requirement_id,profile,ordinal),
 FOREIGN KEY(workspace,project_id,requirement_id,profile) REFERENCES requirement_index(workspace,project_id,requirement_id,profile) ON DELETE CASCADE
);
CREATE INDEX requirement_chunks_project ON requirement_chunks(workspace,project_id,profile);
CREATE FUNCTION invalidate_requirement_index() RETURNS trigger
LANGUAGE plpgsql SET search_path FROM CURRENT AS $$
BEGIN
 DELETE FROM requirement_index WHERE requirement_id=OLD.id;
 RETURN NULL;
END;
$$;
CREATE TRIGGER requirement_index_stale AFTER UPDATE ON requirements
 FOR EACH ROW EXECUTE FUNCTION invalidate_requirement_index();
CREATE TRIGGER requirement_index_cache_event AFTER INSERT OR UPDATE OR DELETE ON requirement_index
 FOR EACH ROW EXECUTE FUNCTION record_cache_change();
