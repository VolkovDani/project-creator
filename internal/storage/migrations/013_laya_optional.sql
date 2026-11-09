ALTER TABLE analysis_pairs DROP CONSTRAINT analysis_pairs_laya_status_check;
ALTER TABLE analysis_pairs ADD CONSTRAINT analysis_pairs_laya_status_check
 CHECK(laya_status IN ('','pending','running','completed','failed','skipped'));
ALTER TABLE analysis_pairs ADD COLUMN laya_error_code TEXT NOT NULL DEFAULT '';
DROP INDEX analysis_pairs_llm_pending;
CREATE INDEX analysis_pairs_llm_pending ON analysis_pairs(run_id,position)
 WHERE status='pending' AND laya_status IN ('completed','failed','skipped');

ALTER TABLE requirement_summaries DROP CONSTRAINT requirement_summaries_check;
ALTER TABLE requirement_summaries ADD CONSTRAINT requirement_summaries_check
 CHECK(status<>'ready' OR octet_length(text)>0);
