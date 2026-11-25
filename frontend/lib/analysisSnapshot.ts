import type { AnalysisSnapshotFragment } from './generated';
export type AnalysisSnapshot = Omit<AnalysisSnapshotFragment, '__typename'>;

export function parseAnalysisSnapshot(value: unknown): AnalysisSnapshot {
  if (!value || typeof value !== 'object') throw Error('invalid analysis snapshot');
  const record = value as Record<string, unknown>;
  const strings = [
    'id',
    'projectId',
    'revision',
    'createdAt',
    'status',
    'stage',
    'outcome',
    'errorCode',
    'reasonCode',
    'completedAt',
    'mode',
    'activePairId',
  ];
  const counts = [
    'processedPairs',
    'duplicatePairs',
    'reusedPairs',
    'newPairs',
    'completedPairs',
    'pendingPairs',
    'runningPairs',
    'failedPairs',
    'collisionPairs',
    'compatiblePairs',
    'insufficientPairs',
  ];
  const optionalCounts = [
    'layaPendingPairs',
    'layaRunningPairs',
    'layaCompletedPairs',
    'layaFailedPairs',
    'layaSkippedPairs',
  ];
  if (
    strings.some((key) => typeof record[key] !== 'string') ||
    counts.some((key) => !Number.isSafeInteger(record[key]) || Number(record[key]) < 0) ||
    optionalCounts.some(
      (key) =>
        record[key] !== undefined &&
        (!Number.isSafeInteger(record[key]) || Number(record[key]) < 0),
    ) ||
    (record.deletionScheduledFor != null &&
      (typeof record.deletionScheduledFor !== 'string' ||
        !Number.isFinite(Date.parse(record.deletionScheduledFor)))) ||
    (record.classifierProfile !== undefined && typeof record.classifierProfile !== 'string') ||
    ['totalPairs', 'possiblePairs', 'notSelectedPairs'].some(
      (key) =>
        record[key] !== null && (!Number.isSafeInteger(record[key]) || Number(record[key]) < 0),
    ) ||
    typeof record.stale !== 'boolean' ||
    typeof record.similarityThreshold !== 'number' ||
    !Array.isArray(record.missingRequirementIds) ||
    record.missingRequirementIds.some((id) => typeof id !== 'string') ||
    (record.glossaryRevision !== undefined &&
      (typeof record.glossaryRevision !== 'string' || !/^\d+$/.test(record.glossaryRevision))) ||
    !/^\d+$/.test(String(record.revision))
  )
    throw Error('invalid analysis snapshot');
  return {
    ...value,
    deletionScheduledFor: record.deletionScheduledFor ?? null,
    glossaryRevision: record.glossaryRevision ?? '0',
    layaSkippedPairs: record.layaSkippedPairs ?? 0,
  } as AnalysisSnapshot;
}
