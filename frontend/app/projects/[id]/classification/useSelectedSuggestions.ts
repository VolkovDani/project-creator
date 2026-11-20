import { useEffect, useRef, useState } from 'react';
import { eligibleSuggestion, type SavedSuggestion } from '../../../../lib/classificationReview';
import { clientCache, clientSessionVersion, query } from '../../../../lib/client';
import {
  ConfirmClassificationDocument,
  RequirementClassificationDocument,
} from '../../../../lib/generated';

type Outcome = 'confirmed' | 'changed' | 'failed';
export default function useSelectedSuggestions(projectId: string) {
  const [selected, setSelected] = useState(new Map<string, SavedSuggestion>());
  const [pageIds, setPageIds] = useState<string[]>();
  const [busy, setBusy] = useState(false);
  const running = useRef(false);
  const mounted = useRef(false);
  const [outcomes, setOutcomes] = useState(new Map<string, Outcome>());
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  function clear() {
    setSelected(new Map());
    setPageIds(undefined);
    setOutcomes(new Map());
  }
  function select(id: string, proposal: SavedSuggestion | undefined, ids: string[]) {
    if (running.current) return;
    const next = new Map(selected);
    if (proposal) next.set(id, proposal);
    else next.delete(id);
    setSelected(next);
    setPageIds(next.size ? ids : undefined);
    setOutcomes(new Map());
  }
  async function confirm() {
    if (!selected.size || running.current) return;
    running.current = true;
    setBusy(true);
    const session = clientSessionVersion();
    const current = () => mounted.current && session === clientSessionVersion();
    const results = new Map<string, Outcome>();
    const listTag = `project:${projectId}:classifications`;
    const tags = [listTag];
    try {
      for (const [requirementId, proposal] of selected) {
        if (!current()) return;
        const recordTag = `project:${projectId}:classification:${requirementId}`;
        tags.push(recordTag);
        try {
          const { requirementClassification: entry } = await query(
            RequirementClassificationDocument,
            { projectId, requirementId },
            { cache: false },
          );
          if (!current()) return;
          if (
            !entry ||
            !eligibleSuggestion(entry, proposal.textVersion) ||
            entry.revision !== proposal.revision ||
            entry.semanticType !== proposal.semanticType ||
            entry.representationType !== proposal.representationType
          ) {
            results.set(requirementId, 'changed');
            continue;
          }
          const { confirmRequirementClassification: response } = await query(
            ConfirmClassificationDocument,
            { input: { projectId, ...proposal } },
            { tags: [recordTag, listTag], notify: false },
          );
          if (response.classification?.status === 'confirmed' && !response.fieldErrors.length)
            results.set(requirementId, 'confirmed');
          else
            results.set(
              requirementId,
              response.fieldErrors.some((error) => error.code === 'CONFLICT')
                ? 'changed'
                : 'failed',
            );
        } catch {
          results.set(requirementId, 'failed');
        }
      }
      if (current()) {
        setSelected(new Map());
        setPageIds(undefined);
        setOutcomes(results);
        clientCache.invalidate(tags);
      }
    } finally {
      running.current = false;
      if (current()) setBusy(false);
    }
  }
  return { selected, pageIds, busy, outcomes, select, clear, confirm };
}
