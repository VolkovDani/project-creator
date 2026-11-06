import type { ProjectAnalysisStatusQuery } from './generated';

export type AnalysisRun = NonNullable<ProjectAnalysisStatusQuery['projectAnalysis']>;
export type AnalysisPairView = AnalysisRun['pairs'][number];
export type AnalysisGroupKey =
  | 'collision'
  | 'duplicate'
  | 'compatible'
  | 'insufficient'
  | 'pending'
  | 'failed';
export type AnalysisGroup = { key: AnalysisGroupKey; pairs: AnalysisPairView[]; count: number };

export function groupPairs(run: AnalysisRun): AnalysisGroup[] {
  const groups: AnalysisGroup[] = [
    'collision',
    'duplicate',
    'compatible',
    'insufficient',
    'pending',
    'failed',
  ].map((key) => ({
    key: key as AnalysisGroupKey,
    pairs: [],
    count: groupCount(run, key as AnalysisGroupKey),
  }));
  const legacy: AnalysisPairView[] = run.pairs.length
    ? []
    : run.findings.flatMap((finding, position) => {
        const left = run.sources.find(
          (source) => source.id === finding.leftId && source.version === finding.leftVersion,
        );
        const right = run.sources.find(
          (source) => source.id === finding.rightId && source.version === finding.rightVersion,
        );
        return left && right
          ? [
              {
                id: `legacy-${position}`,
                position,
                status: 'completed',
                left,
                right,
                verdict: finding.verdict,
                explanation: finding.explanation,
                errorCode: '',
                selectionReasons: [],
                similarityScore: null,
                resultOrigin: 'model',
                reusedFrom: '',
                checkedAt: '',
                generatorProfile: run.generatorProfile,
              },
            ]
          : [];
      });
  for (const pair of [...run.pairs, ...legacy].sort(
    (a, b) => a.position - b.position || a.id.localeCompare(b.id),
  )) {
    const key =
      pair.status === 'pending' || pair.status === 'running'
        ? 'pending'
        : pair.status === 'failed'
          ? 'failed'
          : pair.verdict;
    groups.find((group) => group.key === key)?.pairs.push(pair);
  }
  return groups;
}

export function canRetryPairs(run: AnalysisRun): boolean {
  return (
    run.status === 'failed' &&
    run.errorCode !== 'REPLACED' &&
    !run.stale &&
    (run.failedPairs > 0 || run.totalPairs == null)
  );
}

export function groupCount(run: AnalysisRun, key: AnalysisGroupKey): number {
  switch (key) {
    case 'collision':
      return run.collisionPairs;
    case 'duplicate':
      return run.duplicatePairs;
    case 'compatible':
      return run.compatiblePairs;
    case 'insufficient':
      return run.insufficientPairs;
    case 'pending':
      return run.pendingPairs + run.runningPairs;
    case 'failed':
      return run.failedPairs;
  }
}
export function groupFilter(key: AnalysisGroupKey): { status: string; verdict: string } {
  return key === 'pending' || key === 'failed'
    ? { status: key, verdict: '' }
    : { status: 'completed', verdict: key };
}
