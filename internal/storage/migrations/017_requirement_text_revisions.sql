CREATE TABLE requirement_text_revisions (
 requirement_id TEXT NOT NULL REFERENCES requirements(id) ON DELETE CASCADE,
 version INTEGER NOT NULL CHECK(version > 0),
 text TEXT NOT NULL,
 change_reason TEXT,
 author TEXT,
 created_at TEXT NOT NULL,
 PRIMARY KEY(requirement_id, version)
);

-- Existing installations and SQLite imports only have the latest source text.
INSERT INTO requirement_text_revisions(requirement_id, version, text, created_at)
 SELECT id, version, text, updated_at FROM requirements;

CREATE FUNCTION record_initial_requirement_revision() RETURNS trigger
LANGUAGE plpgsql SET search_path FROM CURRENT AS $$
BEGIN
 INSERT INTO requirement_text_revisions(requirement_id, version, text, created_at)
 VALUES(NEW.id, NEW.version, NEW.text, NEW.updated_at);
 RETURN NULL;
END;
$$;
CREATE TRIGGER requirements_initial_revision AFTER INSERT ON requirements
 FOR EACH ROW EXECUTE FUNCTION record_initial_requirement_revision();
