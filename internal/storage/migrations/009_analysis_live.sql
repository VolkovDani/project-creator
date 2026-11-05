ALTER TABLE analysis_runs ADD COLUMN revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0);

CREATE FUNCTION bump_analysis_revision() RETURNS trigger
LANGUAGE plpgsql SET search_path FROM CURRENT AS $$
BEGIN
 IF ROW(NEW.status,NEW.outcome,NEW.error_code,NEW.stage,NEW.total_pairs,NEW.processed_pairs,
        NEW.active_pair_id,NEW.reason_code,NEW.missing_requirement_ids,NEW.incomplete,
        NEW.generator_provenance,NEW.mode,NEW.similarity_threshold,NEW.selection_profile,
        NEW.possible_pairs)
    IS DISTINCT FROM
    ROW(OLD.status,OLD.outcome,OLD.error_code,OLD.stage,OLD.total_pairs,OLD.processed_pairs,
        OLD.active_pair_id,OLD.reason_code,OLD.missing_requirement_ids,OLD.incomplete,
        OLD.generator_provenance,OLD.mode,OLD.similarity_threshold,OLD.selection_profile,
        OLD.possible_pairs) THEN
   NEW.revision := OLD.revision + 1;
 ELSE
   NEW.revision := OLD.revision;
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER analysis_runs_revision BEFORE UPDATE ON analysis_runs
 FOR EACH ROW EXECUTE FUNCTION bump_analysis_revision();

DROP TRIGGER analysis_runs_cache_event ON analysis_runs;
CREATE FUNCTION record_analysis_cache_change() RETURNS trigger
LANGUAGE plpgsql SET search_path FROM CURRENT AS $$
DECLARE
 ws TEXT;
 project_id TEXT;
BEGIN
 PERFORM pg_advisory_xact_lock(724183092);
 IF TG_OP = 'DELETE' THEN ws := OLD.workspace; project_id := OLD.project_id;
 ELSE ws := NEW.workspace; project_id := NEW.project_id; END IF;
 IF TG_OP = 'UPDATE' AND NEW.revision = OLD.revision THEN RETURN NULL; END IF;
 INSERT INTO cache_events(id,workspace,keys)
 VALUES(md5(random()::text || clock_timestamp()::text || txid_current()::text),ws,
        ARRAY['project:' || project_id || ':analysis']);
 DELETE FROM cache_events WHERE seq < (SELECT seq FROM cache_events ORDER BY seq DESC OFFSET 999 LIMIT 1);
 RETURN NULL;
END;
$$;
CREATE TRIGGER analysis_runs_cache_event AFTER INSERT OR UPDATE OR DELETE ON analysis_runs
 FOR EACH ROW EXECUTE FUNCTION record_analysis_cache_change();
