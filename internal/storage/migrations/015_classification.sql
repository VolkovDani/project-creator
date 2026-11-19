CREATE TABLE requirement_classifications (
 requirement_id TEXT NOT NULL REFERENCES requirements(id) ON DELETE CASCADE,
 workspace TEXT NOT NULL, project_id TEXT NOT NULL,
 revision INTEGER NOT NULL CHECK (revision > 0),
 text_version INTEGER NOT NULL CHECK (text_version > 0),
 semantic_type TEXT NOT NULL, representation_type TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('queued','classifying','suggested','needs_review','confirmed','failed','stale')),
 source TEXT NOT NULL CHECK(source IN ('model','user')),
 rules_version TEXT NOT NULL, model TEXT NOT NULL DEFAULT '',
 alternatives JSONB NOT NULL DEFAULT '[]' CHECK(jsonb_typeof(alternatives)='array'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(requirement_id,revision),
 CHECK(status <> 'confirmed' OR source='user')
);
CREATE TRIGGER classifications_cache_event AFTER INSERT ON requirement_classifications
 FOR EACH ROW EXECUTE FUNCTION record_cache_change();

CREATE FUNCTION check_classification_source() RETURNS trigger
LANGUAGE plpgsql SET search_path FROM CURRENT AS $$
BEGIN
 PERFORM 1 FROM requirements WHERE workspace=NEW.workspace AND project_id=NEW.project_id
  AND id=NEW.requirement_id AND version=NEW.text_version FOR UPDATE;
 IF NOT FOUND THEN
  RAISE EXCEPTION 'classification source version changed' USING ERRCODE='23514';
 END IF;
 IF NEW.source='model' AND EXISTS(SELECT FROM requirement_classifications
   WHERE requirement_id=NEW.requirement_id AND text_version=NEW.text_version AND status='confirmed') THEN
  RAISE EXCEPTION 'source version already confirmed' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER check_classification_before_insert BEFORE INSERT ON requirement_classifications
 FOR EACH ROW EXECUTE FUNCTION check_classification_source();
