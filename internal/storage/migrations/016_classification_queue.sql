ALTER TABLE requirement_classifications
 ADD COLUMN profile TEXT NOT NULL DEFAULT '',
 ADD COLUMN digest TEXT NOT NULL DEFAULT '',
 ADD COLUMN question_version TEXT NOT NULL DEFAULT '',
 ADD COLUMN reason TEXT NOT NULL DEFAULT '',
 ADD COLUMN reanalysis BOOLEAN NOT NULL DEFAULT false;

CREATE TABLE classification_profiles (
 workspace TEXT PRIMARY KEY, profile TEXT NOT NULL, model TEXT NOT NULL, digest TEXT NOT NULL,
 question_version TEXT NOT NULL, rules_version TEXT NOT NULL
);
-- An empty project_id denotes catch-up across the configured workspace.
CREATE TABLE classification_backfills (
 workspace TEXT NOT NULL, project_id TEXT NOT NULL, profile TEXT NOT NULL, failed_only BOOLEAN NOT NULL,
 requested_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(workspace,project_id,profile,failed_only)
);
-- The call slot outlives a superseded job until the caller finishes or its lease expires.
CREATE TABLE classification_call (
 singleton BOOLEAN PRIMARY KEY DEFAULT true CHECK(singleton),
 attempt TEXT NOT NULL, lease_until TIMESTAMPTZ NOT NULL
);
CREATE TABLE classification_jobs (
 workspace TEXT NOT NULL, project_id TEXT NOT NULL, requirement_id TEXT NOT NULL,
 version INTEGER NOT NULL, profile TEXT NOT NULL,
 status TEXT NOT NULL DEFAULT 'queued' CHECK(status IN ('queued','classifying','done','failed','stale')),
 attempts INTEGER NOT NULL DEFAULT 0, attempt TEXT NOT NULL DEFAULT '',
 available_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(), error_code TEXT NOT NULL DEFAULT '',
 reanalysis BOOLEAN NOT NULL DEFAULT false,
 PRIMARY KEY(requirement_id,version,profile),
 FOREIGN KEY(workspace,project_id,requirement_id) REFERENCES requirements(workspace,project_id,id) ON DELETE CASCADE
);
CREATE INDEX classification_jobs_available ON classification_jobs(profile,available_at,requirement_id)
 WHERE status IN ('queued','classifying');
CREATE INDEX classification_jobs_by_project ON classification_jobs(workspace,project_id,status);

CREATE FUNCTION enqueue_requirement_classification() RETURNS trigger LANGUAGE plpgsql SET search_path FROM CURRENT AS $$
BEGIN
 UPDATE classification_jobs SET status='stale',attempt='' WHERE requirement_id=NEW.id AND version<>NEW.version AND status IN ('queued','classifying','failed');
 INSERT INTO classification_jobs(workspace,project_id,requirement_id,version,profile)
 SELECT NEW.workspace,NEW.project_id,NEW.id,NEW.version,profile FROM classification_profiles WHERE workspace=NEW.workspace
 ON CONFLICT DO NOTHING;
 RETURN NULL;
END;$$;
CREATE TRIGGER z_requirement_classification_queue AFTER INSERT OR UPDATE OF version ON requirements
 FOR EACH ROW EXECUTE FUNCTION enqueue_requirement_classification();

CREATE OR REPLACE FUNCTION check_classification_source() RETURNS trigger
LANGUAGE plpgsql SET search_path FROM CURRENT AS $$
BEGIN
 PERFORM 1 FROM requirements WHERE workspace=NEW.workspace AND project_id=NEW.project_id
  AND id=NEW.requirement_id AND version=NEW.text_version FOR UPDATE;
 IF NOT FOUND THEN
  RAISE EXCEPTION 'classification source version changed' USING ERRCODE='23514';
 END IF;
 IF NEW.source='model' AND NOT NEW.reanalysis AND EXISTS(SELECT FROM requirement_classifications
   WHERE requirement_id=NEW.requirement_id AND text_version=NEW.text_version AND status='confirmed') THEN
  RAISE EXCEPTION 'source version already confirmed' USING ERRCODE='23514';
 END IF;
 IF NEW.status='confirmed' THEN
  UPDATE classification_jobs SET status='done',attempt='',error_code='' WHERE requirement_id=NEW.requirement_id
   AND version=NEW.text_version AND NOT reanalysis AND status IN ('queued','classifying','failed');
 END IF;
 RETURN NEW;
END;
$$;

CREATE FUNCTION record_classification_change() RETURNS trigger LANGUAGE plpgsql SET search_path FROM CURRENT AS $$
DECLARE w TEXT; p TEXT; rid TEXT;
BEGIN
 PERFORM pg_advisory_xact_lock(724183092);
 IF TG_TABLE_NAME='requirements' THEN
  IF TG_OP='DELETE' THEN w:=OLD.workspace; p:=OLD.project_id; rid:=OLD.id;
  ELSE w:=NEW.workspace; p:=NEW.project_id; rid:=NEW.id; END IF;
 ELSE w:=NEW.workspace; p:=NEW.project_id; rid:=NEW.requirement_id; END IF;
 INSERT INTO cache_events(id,workspace,keys) VALUES(md5(json_build_array('classification',txid_current(),w,p,rid)::text),w,
 ARRAY['project:'||p||':classifications','project:'||p||':classification:'||rid])
 ON CONFLICT(id) DO NOTHING;
 IF NOT FOUND THEN RETURN NULL; END IF;
 DELETE FROM cache_events WHERE seq < (SELECT seq FROM cache_events ORDER BY seq DESC OFFSET 999 LIMIT 1);
 RETURN NULL;
END;$$;
CREATE TRIGGER classification_source_event AFTER INSERT OR UPDATE OR DELETE ON requirements
 FOR EACH ROW EXECUTE FUNCTION record_classification_change();
DROP TRIGGER classifications_cache_event ON requirement_classifications;
CREATE TRIGGER classifications_cache_event AFTER INSERT ON requirement_classifications
 FOR EACH ROW EXECUTE FUNCTION record_classification_change();
CREATE TRIGGER classification_job_event AFTER INSERT OR UPDATE OF status ON classification_jobs
 FOR EACH ROW EXECUTE FUNCTION record_classification_change();

CREATE VIEW current_requirement_classification AS
 SELECT r.workspace,r.project_id,r.id,r.version,r.text,r.created_at AS requirement_created_at,
 COALESCE(h.revision,0) AS revision,
 CASE WHEN c.status='confirmed' AND c.text_version=r.version THEN 'confirmed'
      WHEN j.status IN ('queued','classifying','failed') THEN j.status
      WHEN c.text_version<>r.version OR (c.profile<>'' AND c.profile IS DISTINCT FROM p.profile) THEN 'stale'
      ELSE COALESCE(c.status,'unclassified') END AS status,
 COALESCE(c.semantic_type,'unknown') AS semantic_type,COALESCE(c.representation_type,'unknown') AS representation_type,
 COALESCE(c.text_version,r.version) AS decision_version,COALESCE(c.source,'') AS source,
 COALESCE(c.rules_version,CASE WHEN j.profile IS NOT NULL THEN p.rules_version END,'') AS rules_version,COALESCE(c.model,CASE WHEN j.profile IS NOT NULL THEN p.model END,'') AS model,
 COALESCE(c.created_at::text,'') AS decision_created_at,COALESCE(c.alternatives,'[]') AS alternatives,
 COALESCE(c.digest,CASE WHEN j.profile IS NOT NULL THEN p.digest END,'') AS digest,COALESCE(c.profile,j.profile,'') AS profile,
 COALESCE(c.question_version,CASE WHEN j.profile IS NOT NULL THEN p.question_version END,'') AS question_version,COALESCE(c.reason,'') AS reason,
 CASE WHEN p.profile IS NULL AND c.revision IS NULL THEN 'NOT_CONFIGURED' ELSE COALESCE(j.error_code,'') END AS error_code
 FROM requirements r
 LEFT JOIN classification_profiles p ON p.workspace=r.workspace
 LEFT JOIN classification_jobs j ON j.requirement_id=r.id AND j.version=r.version AND j.profile=p.profile
 LEFT JOIN LATERAL (SELECT * FROM requirement_classifications h WHERE h.requirement_id=r.id
   ORDER BY h.text_version DESC,(h.status='confirmed') DESC,(h.profile=p.profile) DESC NULLS LAST,h.revision DESC LIMIT 1) h ON true
 -- A pending job belongs to its current source. Earlier decisions remain in history.
 LEFT JOIN LATERAL (SELECT h.* WHERE COALESCE(j.status,'') NOT IN ('queued','classifying','failed')
   OR (h.status='confirmed' AND h.text_version=r.version)) c ON true;
CREATE INDEX classification_effective_revision ON requirement_classifications(requirement_id,text_version DESC,(status='confirmed') DESC,revision DESC);
