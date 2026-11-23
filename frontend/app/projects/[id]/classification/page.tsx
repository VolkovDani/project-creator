'use client';
import { useCallback, useState } from 'react';
import rules from '../../../../../contracts/classification.json';
import { classificationCounts } from '../../../../lib/classificationReview';
import { query } from '../../../../lib/client';
import {
  BackfillClassificationsDocument,
  ClassificationGroupsDocument,
  ClassificationPageIdsDocument,
  ClassificationProgressDocument,
  type ClassificationProgressQuery,
  RetryFailedClassificationsDocument,
} from '../../../../lib/generated';
import { fieldErrorMessage, t } from '../../../../lib/i18n';
import { offsetConnection } from '../../../../lib/infinite';
import { useInfinite } from '../../../../lib/useInfinite';
import { useResource } from '../../../../lib/useResource';
import Group from '../../../components/Group';
import { LoadMore } from '../../../components/ListControls';
import ResourceState from '../../../components/ResourceState';
import { useProject } from '../ProjectShell';
import ClassificationCard from './ClassificationCard';
import ClassificationGuide from './ClassificationGuide';
import useSelectedSuggestions from './useSelectedSuggestions';

const pageSize = 20;
const statusFields: Record<
  string,
  keyof Pick<
    NonNullable<ClassificationProgressQuery['classificationProgress']>,
    | 'unclassified'
    | 'queued'
    | 'classifying'
    | 'suggested'
    | 'needsReview'
    | 'confirmed'
    | 'failed'
    | 'stale'
  >
> = {
  unclassified: 'unclassified',
  queued: 'queued',
  classifying: 'classifying',
  suggested: 'suggested',
  needs_review: 'needsReview',
  confirmed: 'confirmed',
  failed: 'failed',
  stale: 'stale',
};
const filterCodes = {
  semanticType: rules.semantic,
  representationType: rules.representation,
  status: rules.states,
};
const filterLabels = {
  semanticType: 'review.meaning',
  representationType: 'review.form',
  status: 'review.status',
};
const groupLabels = {
  semanticType: 'review.meaningGroup',
  representationType: 'review.formGroup',
  status: 'review.statusGroup',
};
const initialFilters = { semanticType: '', representationType: '', status: '' };
type Axis = keyof typeof filterCodes;

export default function ClassificationPage() {
  const { project, locale } = useProject();
  const projectId = project.id;
  const tag = `project:${projectId}:classifications`;
  const requirementsTag = `project:${projectId}:requirements`;
  const batch = useSelectedSuggestions(projectId);
  const [filters, setFilters] = useState(initialFilters);
  const [frozenIds, setFrozenIds] = useState<string[]>();
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState('');
  const [actionError, setActionError] = useState('');
  const hasFilters = Object.values(filters).some(Boolean);
  const progressLoader = useCallback(
    async () =>
      (await query(ClassificationProgressDocument, { projectId }, { tags: [tag, requirementsTag] }))
        .classificationProgress,
    [projectId, tag, requirementsTag],
  );
  const groupsLoader = useCallback(
    async () =>
      (await query(ClassificationGroupsDocument, { projectId }, { tags: [tag, requirementsTag] }))
        .classificationGroups,
    [projectId, tag, requirementsTag],
  );
  const progress = useResource(progressLoader, [tag, requirementsTag]);
  const groups = useResource(groupsLoader, [tag, requirementsTag]);
  const idsLoader = useCallback(
    async (after: string | null) => {
      const response = await query(
        ClassificationPageIdsDocument,
        {
          projectId,
          semanticType: filters.semanticType || null,
          representationType: filters.representationType || null,
          status: filters.status || null,
          limit: pageSize + 1,
          offset: Number(after),
        },
        { tags: hasFilters ? [tag, requirementsTag] : [requirementsTag] },
      );
      return offsetConnection(
        response.classificationList.map(({ requirementId }) => ({ id: requirementId })),
        after,
        pageSize,
      );
    },
    [projectId, filters, hasFilters, tag, requirementsTag],
  );
  const list = useInfinite({
    identity: JSON.stringify(['classification', projectId, filters]),
    tags: hasFilters ? [tag, requirementsTag] : [requirementsTag],
    load: idsLoader,
  });
  const loadMore = useCallback(() => void list.loadMore().catch(() => {}), [list.loadMore]);
  const loadedIds = list.rows.map((row) => row.id);
  const ids =
    frozenIds ?? (batch.pinnedIds ? [...new Set([...batch.pinnedIds, ...loadedIds])] : loadedIds);
  const info = progress.data;
  const counts = classificationCounts(groups.data ?? []);
  function count(axis: Axis, code: string) {
    if (axis === 'status') {
      const field = statusFields[code];
      return info && field ? info[field] : undefined;
    }
    if (!groups.data) return undefined;
    return (axis === 'semanticType' ? counts.semantic : counts.representation).get(code) ?? 0;
  }
  function filter(axis: Axis, value: string) {
    if (batch.busy) return;
    batch.clear();
    setFilters({ ...filters, [axis]: value });
  }
  async function enqueue(failed: boolean) {
    setBusy(true);
    setActionError('');
    setNotice('');
    try {
      const variables = { projectId };
      const options = { tags: [tag] };
      const payload = failed
        ? (await query(RetryFailedClassificationsDocument, variables, options))
            .retryFailedClassifications
        : (await query(BackfillClassificationsDocument, variables, options))
            .backfillClassifications;
      const error = payload.fieldErrors[0];
      if (error) setActionError(fieldErrorMessage(locale, error));
      else setNotice(t(locale, 'review.queuedCount', { count: payload.queued }));
    } catch {
      setActionError(t(locale, 'app.error'));
    } finally {
      setBusy(false);
    }
  }
  const axes = ['semanticType', 'representationType', 'status'] as const;
  return (
    <section className="ui-panel" aria-labelledby="classification-heading">
      <h2 id="classification-heading">{t(locale, 'nav.classification')}</h2>
      <ClassificationGuide locale={locale} />
      <section aria-label={t(locale, 'review.progress')}>
        <h3>{t(locale, 'review.progress')}</h3>
        <ResourceState
          locale={locale}
          loading={!info && progress.loading}
          error={progress.error}
          hasData={!!info}
          retry={progress.reload}
        />
        {info && (
          <>
            <p>
              {t(locale, 'review.total', { count: info.total })} ·{' '}
              {t(locale, 'review.remaining', {
                queued: info.queued,
                running: info.classifying,
                failed: info.failed,
              })}
            </p>
            {!info.configured && <p>{t(locale, 'review.offline')}</p>}
            <button
              type="button"
              disabled={
                batch.busy || busy || !info.configured || info.unclassified + info.stale === 0
              }
              onClick={() => void enqueue(false)}
            >
              {t(locale, 'review.backfill')}
            </button>
            <button
              type="button"
              disabled={batch.busy || busy || !info.configured || info.failed === 0}
              onClick={() => void enqueue(true)}
            >
              {t(locale, 'review.retryFailed')}
            </button>
            <button
              type="button"
              disabled={batch.busy}
              onClick={() => {
                batch.clear();
                setFilters({ ...initialFilters, status: 'needs_review' });
              }}
            >
              {t(locale, 'review.needsReview', { count: info.needsReview })}
            </button>
          </>
        )}
        {notice && <p role="status">{notice}</p>}
        {actionError && <p role="alert">{actionError}</p>}
      </section>
      <Group
        id={projectId + ':classification:groups'}
        title={t(locale, 'review.groups')}
        defaultOpen={false}
      >
        <ResourceState
          locale={locale}
          loading={!groups.data && groups.loading}
          hasData={!!groups.data}
          error={groups.error}
          retry={groups.reload}
        />
        <div className="classification-groups">
          {axes.map((axis) => (
            <div key={axis}>
              <h4>{t(locale, groupLabels[axis])}</h4>
              <ul>
                {filterCodes[axis].map((code) => (
                  <li key={code}>
                    <button
                      type="button"
                      disabled={batch.busy}
                      aria-pressed={filters[axis] === code}
                      onClick={() => filter(axis, code)}
                    >
                      {t(locale, `classification.${code}`)} · {count(axis, code) ?? '—'}
                    </button>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      </Group>
      <fieldset disabled={batch.busy}>
        <legend>{t(locale, 'review.filters')}</legend>
        {axes.map((axis) => (
          <label key={axis}>
            {t(locale, filterLabels[axis])}
            <select value={filters[axis]} onChange={(event) => filter(axis, event.target.value)}>
              <option value="">{t(locale, 'review.all')}</option>
              {filterCodes[axis].map((code) => (
                <option value={code} key={code}>
                  {t(locale, `classification.${code}`)}
                </option>
              ))}
            </select>
          </label>
        ))}
        {hasFilters && (
          <button
            type="button"
            onClick={() => {
              batch.clear();
              setFilters(initialFilters);
            }}
          >
            {t(locale, 'review.clear')}
          </button>
        )}
      </fieldset>
      <Group id={projectId + ':classification:records'} title={t(locale, 'review.records')}>
        <ResourceState
          locale={locale}
          loading={list.loading}
          error={list.error}
          hasData={list.loaded}
          retry={() => void list.retry().catch(() => {})}
        />
        {list.loaded && !ids.length && <p>{t(locale, 'review.empty')}</p>}
        <button
          type="button"
          disabled={!batch.selected.size || batch.busy || !!frozenIds}
          onClick={() => void batch.confirm()}
        >
          {t(locale, 'review.confirmSelected', { count: batch.selected.size })}
        </button>
        {batch.outcomes.size > 0 && (
          <>
            <p role="status">
              {t(locale, 'review.batchSummary', {
                confirmed: [...batch.outcomes.values()].filter((value) => value === 'confirmed')
                  .length,
                changed: [...batch.outcomes.values()].filter((value) => value === 'changed').length,
                failed: [...batch.outcomes.values()].filter((value) => value === 'failed').length,
              })}
            </p>
            <ul>
              {[...batch.outcomes].map(([id, outcome]) => (
                <li key={id}>
                  {id} · {t(locale, `review.batch.${outcome}`)}
                </li>
              ))}
            </ul>
          </>
        )}
        {ids.map((id) => (
          <ClassificationCard
            key={id}
            projectId={projectId}
            requirementId={id}
            configured={info?.configured}
            selection={{
              checked: batch.selected.has(id),
              disabled: batch.busy,
              change: (proposal) => batch.select(id, proposal, ids),
            }}
            locale={locale}
            onEditing={(editing) => setFrozenIds(editing ? ids : undefined)}
          />
        ))}
        {!frozenIds && !batch.busy && (
          <LoadMore locale={locale} {...list} observe loadMore={loadMore} />
        )}
      </Group>
    </section>
  );
}
