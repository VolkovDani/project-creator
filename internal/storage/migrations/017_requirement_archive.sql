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
CREATE TRIGGER z_requirement_index_queue AFTER INSERT OR UPDATE OF version,archived_at ON requirements
 FOR EACH ROW WHEN (NEW.archived_at IS NULL) EXECUTE FUNCTION enqueue_requirement_index();
DROP TRIGGER z_requirement_summary_queue ON requirements;
CREATE TRIGGER z_requirement_summary_queue AFTER INSERT OR UPDATE OF version,archived_at ON requirements
 FOR EACH ROW WHEN (NEW.archived_at IS NULL) EXECUTE FUNCTION enqueue_requirement_summary();
DROP TRIGGER z_requirement_classification_queue ON requirements;
CREATE TRIGGER z_requirement_classification_queue AFTER INSERT OR UPDATE OF version,archived_at ON requirements
 FOR EACH ROW WHEN (NEW.archived_at IS NULL) EXECUTE FUNCTION enqueue_requirement_classification();

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
