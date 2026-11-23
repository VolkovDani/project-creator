-- Archiving changes visibility, not the text version or its provenance.
ALTER TABLE requirements ADD COLUMN archived_at TEXT;
CREATE VIEW active_requirements AS
 SELECT id, workspace, project_id, text, version, created_at, updated_at
 FROM requirements WHERE archived_at IS NULL;
CREATE INDEX archived_requirements_page ON requirements(workspace, project_id, created_at, id)
 WHERE archived_at IS NOT NULL;

DROP TRIGGER requirement_index_stale ON requirements;
CREATE TRIGGER requirement_index_stale AFTER UPDATE OF text,version ON requirements
 FOR EACH ROW EXECUTE FUNCTION invalidate_requirement_index();
DROP TRIGGER z_requirement_index_queue ON requirements;
CREATE TRIGGER z_requirement_index_queue AFTER INSERT OR UPDATE OF version ON requirements
 FOR EACH ROW WHEN (NEW.archived_at IS NULL) EXECUTE FUNCTION enqueue_requirement_index();
DROP TRIGGER z_requirement_summary_queue ON requirements;
CREATE TRIGGER z_requirement_summary_queue AFTER INSERT OR UPDATE OF version ON requirements
 FOR EACH ROW WHEN (NEW.archived_at IS NULL) EXECUTE FUNCTION enqueue_requirement_summary();

CREATE FUNCTION sync_archived_requirement_jobs() RETURNS trigger
LANGUAGE plpgsql SET search_path FROM CURRENT AS $$
BEGIN
 IF NEW.archived_at IS NOT NULL THEN
  -- Revoke in-flight attempts. Workers can only reclaim these jobs after restoration.
  UPDATE requirement_summaries SET status='queued',attempt='',lease_until=clock_timestamp()
   WHERE workspace=NEW.workspace AND project_id=NEW.project_id AND requirement_id=NEW.id AND status='running';
  UPDATE classification_jobs SET status='queued',attempt='',attempts=greatest(0,attempts-1),available_at=clock_timestamp()
   WHERE requirement_id=NEW.id AND status='classifying';
  UPDATE requirement_index SET status='queued',attempt='',attempts=greatest(0,attempts-1),available_at=clock_timestamp(),lease_until=clock_timestamp()
   WHERE workspace=NEW.workspace AND project_id=NEW.project_id AND requirement_id=NEW.id AND status IN ('pending','running');
  RETURN NULL;
 END IF;

 -- Activation backfills skip archived sources. Pick up the current profiles on restore.
 INSERT INTO requirement_summaries(workspace,project_id,requirement_id,version,profile,provenance,status)
 SELECT NEW.workspace,NEW.project_id,NEW.id,NEW.version,profile,provenance,'queued' FROM summary_profiles
 WHERE workspace=NEW.workspace AND profile<>''
 ON CONFLICT(workspace,project_id,requirement_id,version,profile) DO UPDATE
 SET status='queued',attempt='',error_code='',text='',provenance=excluded.provenance WHERE requirement_summaries.status='stale';

 INSERT INTO classification_jobs(workspace,project_id,requirement_id,version,profile)
 SELECT NEW.workspace,NEW.project_id,NEW.id,NEW.version,profile FROM classification_profiles
 WHERE workspace=NEW.workspace AND NOT EXISTS (
  SELECT FROM requirement_classifications WHERE requirement_id=NEW.id AND text_version=NEW.version AND status='confirmed')
 ON CONFLICT(requirement_id,version,profile) DO UPDATE
 SET status='queued',attempt='',attempts=0,error_code='',available_at=clock_timestamp(),reanalysis=false
 WHERE classification_jobs.status='stale';

 INSERT INTO requirement_index(workspace,project_id,requirement_id,profile,version,status,attempt,lease_until)
 SELECT NEW.workspace,NEW.project_id,NEW.id,profile,NEW.version,'queued','',clock_timestamp() FROM embedding_profiles
 WHERE workspace=NEW.workspace AND profile<>''
 ON CONFLICT(workspace,project_id,requirement_id,profile) DO UPDATE
 SET version=excluded.version,status='queued',attempt='',lease_until=clock_timestamp(),error_code='',attempts=0,available_at=clock_timestamp()
 WHERE requirement_index.version<>excluded.version
    OR (requirement_index.status='failed' AND requirement_index.error_code IN ('PROFILE_CHANGED','NOT_CONFIGURED'));
 RETURN NULL;
END;
$$;
CREATE TRIGGER requirement_archive_jobs AFTER UPDATE OF archived_at ON requirements
 FOR EACH ROW WHEN (OLD.archived_at IS DISTINCT FROM NEW.archived_at) EXECUTE FUNCTION sync_archived_requirement_jobs();

CREATE OR REPLACE VIEW current_requirement_classification AS
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
 FROM active_requirements r
 LEFT JOIN classification_profiles p ON p.workspace=r.workspace
 LEFT JOIN classification_jobs j ON j.requirement_id=r.id AND j.version=r.version AND j.profile=p.profile
 LEFT JOIN LATERAL (SELECT * FROM requirement_classifications h WHERE h.requirement_id=r.id
   ORDER BY h.text_version DESC,(h.status='confirmed') DESC,(h.profile=p.profile) DESC NULLS LAST,h.revision DESC LIMIT 1) h ON true
 -- A pending job belongs to its current source. Earlier decisions remain in history.
 LEFT JOIN LATERAL (SELECT h.* WHERE COALESCE(j.status,'') NOT IN ('queued','classifying','failed')
   OR (h.status='confirmed' AND h.text_version=r.version)) c ON true;
