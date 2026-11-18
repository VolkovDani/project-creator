CREATE TABLE glossary_terms (
 id TEXT PRIMARY KEY, workspace TEXT NOT NULL, project_id TEXT NOT NULL,
 name TEXT NOT NULL, definition TEXT NOT NULL, meaning_context TEXT NOT NULL,
 variants JSONB NOT NULL, version INTEGER NOT NULL CHECK (version > 0),
 source_id TEXT NOT NULL, source_version INTEGER NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(), updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY (workspace,project_id) REFERENCES projects(workspace,id) ON DELETE CASCADE
);
CREATE INDEX glossary_project ON glossary_terms(workspace, project_id, name, id);
CREATE TRIGGER glossary_write_order BEFORE INSERT OR UPDATE OR DELETE ON glossary_terms
 FOR EACH STATEMENT EXECUTE FUNCTION lock_index_queue_writes();
