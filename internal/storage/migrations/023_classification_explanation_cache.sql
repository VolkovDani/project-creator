CREATE TABLE classification_explanation_cache (
 workspace TEXT NOT NULL,
 project_id TEXT NOT NULL,
 requirement_id TEXT NOT NULL REFERENCES requirements(id) ON DELETE CASCADE,
 axis TEXT NOT NULL CHECK(axis IN ('meaning','form')),
 cache_key BYTEA NOT NULL CHECK(octet_length(cache_key)=32),
 result JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY (workspace,project_id,requirement_id,axis,cache_key),
 FOREIGN KEY (workspace,project_id) REFERENCES projects(workspace,id) ON DELETE CASCADE
);
CREATE INDEX classification_explanation_cache_project ON classification_explanation_cache(project_id);
