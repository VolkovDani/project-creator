import { indexStatus } from './rag.ts';

/** Locate a source using bounded metadata pages; undefined means the view changed. */
export async function findRequirementPage(
  id: string,
  load: (offset: number) => Promise<ReadonlyArray<{ id: string }>>,
  current: () => boolean,
): Promise<number | null | undefined> {
  for (let offset = 0; offset < 10000; offset += 100) {
    const rows = await load(offset);
    if (!current()) return undefined;
    const index = rows.findIndex((row) => row.id === id);
    if (index >= 0) return Math.floor((offset + index) / 20);
    if (rows.length < 100) return null;
  }
  return null;
}

/** Counts describe only the supplied requirements, never the whole project. */
export function summarizeIndex(
  rows: ReadonlyArray<{ id: string; version: number }>,
  entries: ReadonlyArray<{ requirementId: string; version: number; status: string }>,
) {
  const counts = { total: rows.length, ready: 0, pending: 0, running: 0, failed: 0 };
  for (const row of rows)
    counts[
      indexStatus(
        entries.find((entry) => entry.requirementId === row.id),
        row.version,
      )
    ]++;
  return counts;
}
