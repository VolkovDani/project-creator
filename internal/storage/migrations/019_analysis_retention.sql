ALTER TABLE analysis_runs ADD COLUMN deletion_scheduled_for TIMESTAMPTZ
 CHECK (deletion_scheduled_for IS NULL OR status IN ('completed','failed'));
CREATE INDEX analysis_runs_expiry ON analysis_runs(deletion_scheduled_for)
 WHERE deletion_scheduled_for IS NOT NULL;

-- Snapshots and pairs belong to exactly one run. Other runs retain their own
-- snapshots, verdicts and immutable reused_from identifiers.
ALTER TABLE analysis_pairs DROP CONSTRAINT analysis_pairs_run_id_fkey,
 ADD FOREIGN KEY(run_id) REFERENCES analysis_runs(id) ON DELETE CASCADE;
ALTER TABLE analysis_sources DROP CONSTRAINT analysis_sources_run_id_fkey,
 ADD FOREIGN KEY(run_id) REFERENCES analysis_runs(id) ON DELETE CASCADE;

CREATE OR REPLACE FUNCTION bump_analysis_revision() RETURNS trigger
LANGUAGE plpgsql SET search_path FROM CURRENT AS $$
BEGIN
 IF ROW(NEW.status,NEW.outcome,NEW.error_code,NEW.stage,NEW.total_pairs,NEW.processed_pairs,
        NEW.active_pair_id,NEW.reason_code,NEW.missing_requirement_ids,NEW.incomplete,
        NEW.generator_provenance,NEW.mode,NEW.similarity_threshold,NEW.selection_profile,
        NEW.possible_pairs,NEW.classifier_tick,NEW.deletion_scheduled_for)
    IS DISTINCT FROM
    ROW(OLD.status,OLD.outcome,OLD.error_code,OLD.stage,OLD.total_pairs,OLD.processed_pairs,
        OLD.active_pair_id,OLD.reason_code,OLD.missing_requirement_ids,OLD.incomplete,
        OLD.generator_provenance,OLD.mode,OLD.similarity_threshold,OLD.selection_profile,
        OLD.possible_pairs,OLD.classifier_tick,OLD.deletion_scheduled_for) THEN
   NEW.revision := OLD.revision + 1;
 ELSE NEW.revision := OLD.revision;
 END IF;
 RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION record_analysis_cache_change() RETURNS trigger
LANGUAGE plpgsql SET search_path FROM CURRENT AS $$
DECLARE
 ws TEXT;
 project_id TEXT;
 event_keys TEXT[];
BEGIN
 PERFORM pg_advisory_xact_lock(724183092);
 IF TG_OP = 'DELETE' THEN ws := OLD.workspace; project_id := OLD.project_id;
 ELSE ws := NEW.workspace; project_id := NEW.project_id; END IF;
 IF TG_OP = 'UPDATE' AND NEW.revision = OLD.revision THEN RETURN NULL; END IF;
 event_keys := ARRAY['project:' || project_id || ':analysis'];
 IF TG_OP IN ('INSERT','DELETE') OR (TG_OP='UPDATE' AND NEW.deletion_scheduled_for IS DISTINCT FROM OLD.deletion_scheduled_for) THEN
  event_keys := array_append(event_keys,'project:' || project_id || ':analysis-history');
 END IF;
 INSERT INTO cache_events(id,workspace,keys)
 VALUES(md5(random()::text || clock_timestamp()::text || txid_current()::text),ws,event_keys);
 DELETE FROM cache_events WHERE seq < (SELECT seq FROM cache_events ORDER BY seq DESC OFFSET 999 LIMIT 1);
 RETURN NULL;
END;
$$;
