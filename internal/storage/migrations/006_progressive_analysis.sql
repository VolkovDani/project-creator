ALTER TABLE analysis_runs
 ADD COLUMN stage TEXT NOT NULL DEFAULT 'finished' CHECK(stage IN ('checking_index','finding_pairs','comparing','finished')),
 ADD COLUMN total_pairs INTEGER CHECK(total_pairs BETWEEN 0 AND 20),
 ADD COLUMN processed_pairs INTEGER NOT NULL DEFAULT 0 CHECK(processed_pairs BETWEEN 0 AND 20),
 ADD COLUMN active_pair_id TEXT NOT NULL DEFAULT '',
 ADD COLUMN pairs JSONB NOT NULL DEFAULT '[]'::jsonb,
 ADD COLUMN incomplete BOOLEAN NOT NULL DEFAULT false,
 ADD COLUMN worker_id TEXT NOT NULL DEFAULT '';

-- Historical findings keep their exact source snapshots. No model is invoked
-- during migration, and unfinished legacy attempts require an explicit retry.
UPDATE analysis_runs r SET pairs = coalesce((
 SELECT jsonb_agg(jsonb_build_object(
  'id', r.id || ':' || (f.ordinality-1)::text,
  'position', f.ordinality-1, 'status', 'completed',
  'left', (SELECT s FROM jsonb_array_elements(r.sources) s WHERE s->>'id'=f.value->>'leftId' AND s->>'version'=f.value->>'leftVersion'),
  'right', (SELECT s FROM jsonb_array_elements(r.sources) s WHERE s->>'id'=f.value->>'rightId' AND s->>'version'=f.value->>'rightVersion'),
  'verdict', f.value->>'verdict', 'explanation', f.value->>'explanation', 'errorCode', '') ORDER BY f.ordinality)
 FROM jsonb_array_elements(r.findings) WITH ORDINALITY f(value,ordinality)
), '[]'::jsonb),
 processed_pairs=jsonb_array_length(findings),
 total_pairs=CASE WHEN status='completed' THEN jsonb_array_length(findings) ELSE NULL END;
UPDATE analysis_runs SET status='failed', error_code='INTERRUPTED', completed_at=clock_timestamp(), lease_until=clock_timestamp()
 WHERE status='running';
