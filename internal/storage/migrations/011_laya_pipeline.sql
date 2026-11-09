-- Historical runs retain empty classifier identity and never acquire invented scores.
ALTER TABLE analysis_runs ADD COLUMN classifier_profile TEXT NOT NULL DEFAULT '';
ALTER TABLE analysis_runs ADD COLUMN classifier_tick BIGINT NOT NULL DEFAULT 0;
ALTER TABLE analysis_pairs ADD COLUMN laya_status TEXT NOT NULL DEFAULT '' CHECK(laya_status IN ('','pending','running','completed','failed'));
ALTER TABLE analysis_pairs ADD COLUMN laya_result JSONB;
ALTER TABLE analysis_pairs ADD COLUMN laya_checked_at TIMESTAMPTZ;
ALTER TABLE analysis_pairs ADD CONSTRAINT analysis_pairs_laya_result CHECK((laya_status='completed') = (laya_result IS NOT NULL));
CREATE INDEX analysis_pairs_active_stages ON analysis_pairs(run_id,position) WHERE status='running' OR laya_status='running';
CREATE INDEX analysis_pairs_laya_pending ON analysis_pairs(run_id,position) WHERE status='pending' AND laya_status='pending';
CREATE INDEX analysis_pairs_llm_pending ON analysis_pairs(run_id,position) WHERE status='pending' AND laya_status='completed';

CREATE OR REPLACE FUNCTION bump_analysis_revision() RETURNS trigger
LANGUAGE plpgsql SET search_path FROM CURRENT AS $$
BEGIN
 IF ROW(NEW.status,NEW.outcome,NEW.error_code,NEW.stage,NEW.total_pairs,NEW.processed_pairs,
        NEW.active_pair_id,NEW.reason_code,NEW.missing_requirement_ids,NEW.incomplete,
        NEW.generator_provenance,NEW.mode,NEW.similarity_threshold,NEW.selection_profile,
        NEW.possible_pairs,NEW.classifier_tick)
    IS DISTINCT FROM
    ROW(OLD.status,OLD.outcome,OLD.error_code,OLD.stage,OLD.total_pairs,OLD.processed_pairs,
        OLD.active_pair_id,OLD.reason_code,OLD.missing_requirement_ids,OLD.incomplete,
        OLD.generator_provenance,OLD.mode,OLD.similarity_threshold,OLD.selection_profile,
        OLD.possible_pairs,OLD.classifier_tick) THEN
   NEW.revision := OLD.revision + 1;
 ELSE NEW.revision := OLD.revision;
 END IF;
 RETURN NEW;
END;
$$;
