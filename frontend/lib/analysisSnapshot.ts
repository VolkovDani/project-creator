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
  if (
    strings.some((key) => typeof record[key] !== 'string') ||
    counts.some((key) => !Number.isSafeInteger(record[key]) || Number(record[key]) < 0) ||
    ['totalPairs', 'possiblePairs', 'notSelectedPairs'].some(
      (key) =>
        record[key] !== null && (!Number.isSafeInteger(record[key]) || Number(record[key]) < 0),
    ) ||
    typeof record.stale !== 'boolean' ||
    typeof record.similarityThreshold !== 'number' ||
    !Array.isArray(record.missingRequirementIds) ||
    record.missingRequirementIds.some((id) => typeof id !== 'string') ||
    !/^\d+$/.test(String(record.revision))
  )
    throw Error('invalid analysis snapshot');
  return value as AnalysisSnapshot;
}
