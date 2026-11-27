-- Existing drafts have no generation timestamp. Start their age at migration.
ALTER TABLE process_graph_cache ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT statement_timestamp();
