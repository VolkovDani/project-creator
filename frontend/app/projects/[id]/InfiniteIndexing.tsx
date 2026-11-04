'use client';
import { useCallback, useEffect, useRef, useState } from 'react';
import { query } from '../../../lib/client';
import {
  IndexRequirementDocument,
  RequirementConnectionDocument,
  RequirementIndexByIdsDocument,
} from '../../../lib/generated';
import { type Locale, t } from '../../../lib/i18n';
import { useInfinite } from '../../../lib/useInfinite';
import Group from '../../components/Group';
import { LoadMore } from '../../components/ListControls';
import ResourceState from '../../components/ResourceState';
import IndexGroups from './IndexGroups';

export default function InfiniteIndexing({
  projectId,
  locale,
}: {
  projectId: string;
  locale: Locale;
}) {
  const load = useCallback(
    async (after: string | null) => {
      const { requirementsConnection: connection } = await query(RequirementConnectionDocument, {
        projectId,
        first: 40,
        after,
      });
      const ids = connection.edges.map(({ node }) => node.id);
      const entries = ids.length
        ? (
            await query(RequirementIndexByIdsDocument, {
              projectId,
              requirementIds: ids,
            })
          ).requirementIndexByIds
        : [];
      const byId = new Map(entries.map((entry) => [entry.requirementId, entry]));
      return {
        ...connection,
        edges: connection.edges.map((edge) => ({
          ...edge,
          node: { ...edge.node, entry: byId.get(edge.node.id) },
        })),
      };
    },
    [projectId],
  );
  const infinite = useInfinite({
    identity: `indexing:${projectId}`,
    tags: [`project:${projectId}`, `project:${projectId}:requirements`],
    load,
  });
  const [busy, setBusy] = useState<string>();
  const [mutationError, setMutationError] = useState(false);
  const active = useRef(true);
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);
  const index = async (requirementId: string) => {
    setBusy(requirementId);
    setMutationError(false);
    try {
      await query(IndexRequirementDocument, { projectId, requirementId });
      await infinite.refresh();
    } catch {
      if (active.current) setMutationError(true);
    } finally {
      if (active.current) setBusy(undefined);
    }
  };
  const more = useCallback(() => {
    if (!infinite.loading && !infinite.error && infinite.hasNextPage && !busy)
      void infinite.loadMore().catch(() => {});
  }, [infinite.loading, infinite.error, infinite.hasNextPage, infinite.loadMore, busy]);
  const end = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const element = end.current;
    if (!element) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) more();
      },
      { rootMargin: '500px' },
    );
    observer.observe(element);
    return () => observer.disconnect();
  }, [more]);
  const entries = infinite.rows.flatMap((row) => (row.entry ? [row.entry] : []));
  return (
    <>
      <ResourceState
        locale={locale}
        loading={infinite.loading}
        error={infinite.error || mutationError}
        hasData={infinite.loaded}
        retry={() => {
          setMutationError(false);
          void infinite.retry().catch(() => {});
        }}
      />
      <Group
        id={`${projectId}:indexing`}
        title={t(locale, 'ui.indexing')}
        summary={t(locale, 'list.loaded', {
          count: infinite.rows.length,
          total: infinite.totalCount,
        })}
      >
        {infinite.loaded && infinite.rows.length === 0 && <p>{t(locale, 'requirements.empty')}</p>}
        <IndexGroups
          projectId={projectId}
          locale={locale}
          rows={infinite.rows}
          entries={entries}
          busy={busy}
          index={index}
          virtual
        />
        <div ref={end} aria-hidden="true" style={{ height: 1 }} />
        <LoadMore
          locale={locale}
          {...infinite}
          loading={infinite.loading || !!busy}
          loadMore={more}
        />
      </Group>
    </>
  );
}
