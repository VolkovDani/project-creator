'use client';
import Link from 'next/link';
import type { RequirementIndexListQuery, RequirementsQuery } from '../../../lib/generated';
import { type Locale, t } from '../../../lib/i18n';
import { indexStatus, requirementURL } from '../../../lib/rag';
import Group from '../../components/Group';
import { VirtualList } from '../../components/VirtualList';
import { RequirementIndexAction } from './RagTools';

const statuses = ['failed', 'pending', 'ready'] as const;

export default function IndexGroups({
  projectId,
  locale,
  rows,
  entries,
  busy,
  index,
  virtual = false,
}: {
  projectId: string;
  locale: Locale;
  rows: RequirementsQuery['requirements'];
  entries: RequirementIndexListQuery['requirementIndex'];
  busy?: string;
  index: (id: string) => Promise<void>;
  virtual?: boolean;
}) {
  const byId = new Map(entries.map((entry) => [entry.requirementId, entry]));
  return statuses.map((status) => {
    const grouped = rows.filter((row) => indexStatus(byId.get(row.id), row.version) === status);
    const renderItem = (row: RequirementsQuery['requirements'][number]) => (
      <article className="card" id={`indexing-${row.id}`} key={row.id}>
        <p>{row.text}</p>
        <p className="muted">{t(locale, 'requirements.version', { version: row.version })}</p>
        <Link href={requirementURL(projectId, row.id)}>{t(locale, 'rag.source')}</Link>
        <RequirementIndexAction
          entry={byId.get(row.id)}
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
        {virtual ? (
          <VirtualList
            identity={`indexing:${projectId}:${status}`}
            items={grouped}
            keyOf={(row) => row.id}
            renderItem={renderItem}
          />
        ) : (
          grouped.map(renderItem)
        )}
      </Group>
    );
  });
}
