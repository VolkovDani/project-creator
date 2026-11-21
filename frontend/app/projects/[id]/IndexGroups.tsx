'use client';
import Link from 'next/link';
import type {
  RequirementConnectionQuery,
  RequirementIndexByIdsQuery,
} from '../../../lib/generated';
import { type Locale, t } from '../../../lib/i18n';
import { indexStatus, requirementURL } from '../../../lib/rag';
import Group from '../../components/Group';
import { VirtualList } from '../../components/VirtualList';
import { GlossaryText } from './GlossaryActions';
import { RequirementIndexAction } from './RagTools';

type IndexRow = RequirementConnectionQuery['requirementsConnection']['edges'][number]['node'] & {
  entry?: RequirementIndexByIdsQuery['requirementIndexByIds'][number];
};

const statuses = ['failed', 'running', 'pending', 'ready'] as const;

export default function IndexGroups({
  projectId,
  locale,
  rows,
  busy,
  index,
}: {
  projectId: string;
  locale: Locale;
  rows: IndexRow[];
  busy?: string;
  index: (id: string) => Promise<void>;
}) {
  return statuses.map((status) => {
    const grouped = rows.filter((row) => indexStatus(row.entry, row.version) === status);
    const renderItem = (row: IndexRow) => (
      <article className="card" id={`indexing-${row.id}`} key={row.id}>
        <GlossaryText sourceId={row.id} version={row.version}>
          {row.text}
        </GlossaryText>
        <p className="muted">{t(locale, 'requirements.version', { version: row.version })}</p>
        <Link href={requirementURL(projectId, row.id)}>{t(locale, 'rag.source')}</Link>
        <RequirementIndexAction
          entry={row.entry}
          requirementId={row.id}
          version={row.version}
          locale={locale}
          busy={busy}
          index={index}
        />
      </article>
    );
    return (
      <Group
        id={`${projectId}:indexing:${status}`}
        key={status}
        title={t(locale, `rag.status.${status}`)}
        summary={String(grouped.length)}
        defaultOpen={status !== 'ready'}
      >
        {grouped.length === 0 && <p className="muted">{t(locale, 'ui.groupEmpty')}</p>}
        <VirtualList
          identity={`indexing:${projectId}:${status}`}
          items={grouped}
          keyOf={(row) => row.id}
          renderItem={renderItem}
        />
      </Group>
    );
  });
}
