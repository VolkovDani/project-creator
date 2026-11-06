export type LayaDecision = 'related' | 'unrelated' | 'uncertain';
export function layaPairErrorMessageKey(errorCode: string): string | null {
  return errorCode === 'LAYA_INPUT_TOO_LONG' ? 'analysis.pair.error.layaInputTooLong' : null;
}

export type LayaPreclassification = {
  decision: LayaDecision;
  scores: Record<LayaDecision, number>;
};

export function parseLayaPreclassification(
  value: string | null | undefined,
): LayaPreclassification | null {
  if (!value) return null;
  try {
    const parsed: unknown = JSON.parse(value);
    if (!parsed || typeof parsed !== 'object') return null;
    const record = parsed as Record<string, unknown>;
    const scores = record.scores;
    if (
      !['related', 'unrelated', 'uncertain'].includes(String(record.decision)) ||
      !scores ||
      typeof scores !== 'object'
    )
      return null;
    const scoreRecord = scores as Record<string, unknown>;
    const values = ['related', 'unrelated', 'uncertain'] as const;
    if (
      values.some(
        (key) =>
          typeof scoreRecord[key] !== 'number' ||
          !Number.isFinite(scoreRecord[key]) ||
          Number(scoreRecord[key]) < 0 ||
          Number(scoreRecord[key]) > 1,
      )
    )
      return null;
    return {
      decision: record.decision as LayaDecision,
      scores: {
        related: scoreRecord.related as number,
        unrelated: scoreRecord.unrelated as number,
        uncertain: scoreRecord.uncertain as number,
      },
    };
  } catch {
    return null;
  }
}

export type LayaPoolCounts = {
  layaPendingPairs: number;
  layaRunningPairs: number;
  layaCompletedPairs: number;
  layaFailedPairs: number;
  completedPairs: number;
  runningPairs: number;
  failedPairs: number;
};

export function getLayaPools(counts: LayaPoolCounts) {
  return {
    awaitingLaya: counts.layaPendingPairs + counts.layaRunningPairs,
    awaitingModel: Math.max(
      0,
      counts.layaCompletedPairs -
        counts.completedPairs -
        (counts.failedPairs - counts.layaFailedPairs),
    ),
    completed: counts.completedPairs,
    layaRunning: counts.layaRunningPairs,
    modelRunning: counts.runningPairs,
    layaFailed: counts.layaFailedPairs,
    modelFailed: Math.max(0, counts.failedPairs - counts.layaFailedPairs),
  };
}
