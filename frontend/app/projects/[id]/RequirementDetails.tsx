'use client';
import { type ComponentProps, type ReactNode, type Ref, useId } from 'react';
import { type Locale, t } from '../../../lib/i18n';
import Group from '../../components/Group';
import ClassificationCard from './classification/ClassificationCard';
import ClassificationHistory from './classification/ClassificationHistory';
import GlossaryOriginal from './GlossaryOriginal';
import RequirementRevisionHistory from './RequirementRevisionHistory';
import RequirementSummary from './RequirementSummary';

type Props = {
  row: {
    id: string;
    text: string;
    version: number;
    createdAt: string;
    updatedAt: string;
    archivedAt?: string | null;
  };
  projectId: string;
  projectName: string;
  locale: Locale;
  articleRef: Ref<HTMLElement>;
  children: ReactNode;
  summaryProps?: Pick<
    ComponentProps<typeof RequirementSummary>,
    'summary' | 'loadFailure' | 'loading' | 'refresh'
  >;
  highlight?: { start: number; end: number };
  mentionChanged?: boolean;
};

function MetadataDate({ value, locale }: { value: string; locale: Locale }) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return (
    <time dateTime={value}>
      {date.toLocaleString(locale, { dateStyle: 'medium', timeStyle: 'medium' })}
    </time>
  );
}

export default function RequirementDetails({
  row,
  projectId,
  projectName,
  locale,
  articleRef,
  children,
  summaryProps,
  highlight,
  mentionChanged,
}: Props) {
  const heading = useId();
  const { id, text, version, createdAt, updatedAt, archivedAt } = row;
  const source = { projectId, requirementId: id, original: text, locale };
  return (
    <article
      key={id}
      className="card"
      id={`requirement-${id}`}
      aria-labelledby={heading}
      tabIndex={-1}
      ref={articleRef}
    >
      <h3 id={heading}>{t(locale, 'requirements.detailsTitle')}</h3>
      <dl>
        <div>
          <dt>{t(locale, 'requirements.metaId')}</dt>
          <dd>{id}</dd>
        </div>
        <div>
          <dt>{t(locale, 'requirements.metaProject')}</dt>
          <dd>{projectName}</dd>
        </div>
        <div>
          <dt>{t(locale, 'requirements.metaVersion')}</dt>
          <dd>{version}</dd>
        </div>
        <div>
          <dt>{t(locale, 'requirements.metaStatus')}</dt>
          <dd>
            {t(locale, archivedAt ? 'requirements.statusArchived' : 'requirements.statusActive')}
          </dd>
        </div>
        <div>
          <dt>{t(locale, 'requirements.metaCreatedAt')}</dt>
          <dd>
            <MetadataDate value={createdAt} locale={locale} />
          </dd>
        </div>
        <div>
          <dt>{t(locale, 'requirements.metaUpdatedAt')}</dt>
          <dd>
            <MetadataDate value={updatedAt} locale={locale} />
          </dd>
        </div>
        {archivedAt && (
          <div>
            <dt>{t(locale, 'requirements.metaArchivedAt')}</dt>
            <dd>
              <MetadataDate value={archivedAt} locale={locale} />
            </dd>
          </div>
        )}
      </dl>
      {children}
      {mentionChanged && <p role="status">{t(locale, 'glossary.mentionChanged')}</p>}
      {summaryProps && !archivedAt ? (
        <RequirementSummary
          {...source}
          {...summaryProps}
          showOriginal
          requirementVersion={version}
          highlight={highlight}
        />
      ) : (
        <GlossaryOriginal {...source} version={version} highlight={highlight} collapsible={false} />
      )}
      <Group
        id={`${projectId}:requirement:${id}:classification`}
        title={t(locale, 'nav.classification')}
        lazy
      >
        {archivedAt ? (
          <ClassificationHistory projectId={projectId} requirementId={id} locale={locale} />
        ) : (
          <ClassificationCard {...source} version={version} />
        )}
      </Group>
      <Group
        id={`${projectId}:requirement:${id}:revisions`}
        title={t(locale, 'requirements.history')}
        defaultOpen={false}
        lazy
      >
        <RequirementRevisionHistory
          projectId={projectId}
          requirementId={id}
          version={version}
          locale={locale}
        />
      </Group>
    </article>
  );
}
