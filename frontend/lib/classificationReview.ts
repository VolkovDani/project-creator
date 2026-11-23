import rules from '../../contracts/classification.json' with { type: 'json' };

type Decision = {
  requirementId: string;
  textVersion: number;
  revision: number;
  semanticType: string;
  representationType: string;
};

export function classificationDraft(entry: Decision, currentVersion: number): Decision {
  return {
    requirementId: entry.requirementId,
    textVersion: currentVersion,
    revision: entry.revision,
    semanticType: entry.semanticType,
    representationType: entry.representationType,
  };
}

export function draftIsCurrent(draft: Decision, entry: Decision, currentVersion: number) {
  return (
    draft.requirementId === entry.requirementId &&
    draft.revision === entry.revision &&
    draft.textVersion === currentVersion
  );
}

export function classificationCounts(
  groups: ReadonlyArray<{
    semanticType: string;
    representationType: string;
    count: number;
  }>,
) {
  const semantic = new Map<string, number>();
  const representation = new Map<string, number>();
  for (const group of groups) {
    semantic.set(group.semanticType, (semantic.get(group.semanticType) ?? 0) + group.count);
    representation.set(
      group.representationType,
      (representation.get(group.representationType) ?? 0) + group.count,
    );
  }
  return { semantic, representation };
}

export function groupAlternatives<
  T extends { semanticType: string; representationType: string; score: number | null },
>(alternatives: readonly T[]) {
  const ranked = [...alternatives].sort((a, b) => (b.score ?? -1) - (a.score ?? -1));
  return {
    meaning: ranked.filter((item) => item.semanticType && !item.representationType).slice(0, 3),
    form: ranked.filter((item) => item.representationType && !item.semanticType).slice(0, 3),
    joint: ranked.filter((item) => item.semanticType && item.representationType).slice(0, 3),
  };
}

export type SavedSuggestion = ReturnType<typeof classificationDraft>;
export function eligibleSuggestion(
  entry: Decision & { source: string; status: string; reason: string },
  currentVersion: number,
) {
  return (
    entry.status === 'suggested' &&
    entry.source === 'model' &&
    entry.textVersion === currentVersion &&
    !['uncalibrated', 'outside_training_context', 'input_limit', 'reserved_token'].includes(
      entry.reason,
    ) &&
    entry.semanticType !== 'unknown' &&
    entry.representationType !== 'unknown' &&
    rules.semantic.includes(entry.semanticType) &&
    rules.representation.includes(entry.representationType)
  );
}
