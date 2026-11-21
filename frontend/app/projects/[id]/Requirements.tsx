'use client';
import { useSearchParams } from 'next/navigation';
import { useCallback, useEffect, useRef, useState } from 'react';
import contract from '../../../../contracts/requirement.json';
import { query } from '../../../lib/client';
import {
  AddRequirementDocument,
  EditRequirementDocument,
  RemoveRequirementDocument,
  RequirementConnectionDocument,
} from '../../../lib/generated';
import { type Locale, t } from '../../../lib/i18n';
import { useInvalidation } from '../../../lib/live';
import { validateRequirement } from '../../../lib/requirementValidation';
import { useInfinite } from '../../../lib/useInfinite';
import Group from '../../components/Group';
import { LoadMore } from '../../components/ListControls';
import ResourceState from '../../components/ResourceState';
import { VirtualList } from '../../components/VirtualList';
import ClassificationCard from './classification/ClassificationCard';
import RequirementSummary, { useRequirementSummaries } from './RequirementSummary';
import RetryFailedSummaries from './RetryFailedSummaries';

export default function Requirements({ projectId, locale }: { projectId: string; locale: Locale }) {
  const [busy, setBusy] = useState(false);
  const [text, setText] = useState('');
  const [editText, setEditText] = useState('');
  const [editing, setEditing] = useState<{ id: string; version: number } | null>(null);
  const editTrigger = useRef<HTMLButtonElement>(null);
  const dialogRef = useRef<HTMLDialogElement>(null);
  const [validation, setValidation] = useState<{
    key: string;
    params: Record<string, number>;
  } | null>(null);
  const [sourceTarget, setSourceTarget] = useState<string>();
  const operation = useRef(0);
  const active = useRef(true);
  const clear = useCallback(() => {
    operation.current++;
    setText('');
    setEditText('');
    setEditing(null);
    setValidation(null);
    setFailure(false);
    setBusy(false);
    setSourceTarget(undefined);
  }, []);
  useEffect(() => {
    const dialog = dialogRef.current;
    if (editing && dialog && !dialog.open) dialog.showModal();
    if (!editing && dialog?.open) dialog.close();
    if (!editing) editTrigger.current?.focus({ preventScroll: true });
  }, [editing]);
  useInvalidation([`project:${projectId}`, `project:${projectId}:requirements`], () => {}, clear);
  const connectionLoader = useCallback(
    async (after: string | null) =>
      (await query(RequirementConnectionDocument, { projectId, first: 40, after }))
        .requirementsConnection,
    [projectId],
  );
  const infinite = useInfinite({
    identity: `requirements:${projectId}`,
    tags: [`project:${projectId}`, `project:${projectId}:requirements`],
    load: connectionLoader,
  });
  const [mutationFailure, setFailure] = useState(false);
  const { rows, loading, loaded } = infinite;
  const {
    summaries,
    failure: summaryFailure,
    loading: summaryLoading,
    retryLoad: refreshSummaries,
  } = useRequirementSummaries(
    projectId,
    rows.map((row) => ({ id: row.id, version: row.version })),
  );
  const more = useCallback(() => {
    if (!infinite.loading && !infinite.error && infinite.hasNextPage)
      void infinite.loadMore().catch(() => {});
  }, [infinite.loading, infinite.error, infinite.hasNextPage, infinite.loadMore]);
  const params = useSearchParams();
  const source = params.get('requirement');
  const mentionVersion = Number(params.get('version'));
  const start = Number(params.get('start'));
  const end = Number(params.get('end'));
  const mention =
    params.has('start') &&
    Number.isSafeInteger(mentionVersion) &&
    mentionVersion > 0 &&
    Number.isSafeInteger(start) &&
    Number.isSafeInteger(end) &&
    start >= 0 &&
    end > start
      ? { start, end }
      : undefined;
  useEffect(() => {
    void projectId;
    active.current = true;
    return () => {
      active.current = false;
      operation.current++;
    };
  }, [projectId]);
  useEffect(() => {
    setSourceTarget(params.get('requirement') ?? undefined);
  }, [params]);
  const seeking = !!sourceTarget && sourceTarget === source;
  const found = seeking && rows.some((row) => row.id === sourceTarget);
  useEffect(() => {
    if (!seeking || !loaded || loading || infinite.error) return;
    if (!found) {
      more();
      return;
    }
    const card = document.getElementById(`requirement-${sourceTarget}`);
    if (!card) return;
    const group = card.closest('details');
    if (group) group.open = true;
    (card.querySelector('mark[data-candidate]') ?? card).scrollIntoView({ block: 'center' });
    card.focus({ preventScroll: true });
    setSourceTarget(undefined);
  }, [seeking, found, loaded, loading, infinite.error, more, sourceTarget]);
  const current = (epoch: number) => active.current && operation.current === epoch;
  async function save(event: React.FormEvent) {
    event.preventDefault();
    const submittedText = editing ? editText : text;
    const checked = validateRequirement(contract, submittedText),
      first = checked.errors[0];
    setValidation(first ? { key: first.messageKey, params: first.params } : null);
    if (first) return;
    const epoch = ++operation.current;
    setBusy(true);
    setFailure(false);
    try {
      const out = editing
        ? await query(EditRequirementDocument, {
            input: {
              id: editing.id,
              version: editing.version,
              projectId,
              text: checked.normalized,
            },
          })
        : await query(AddRequirementDocument, { input: { projectId, text: checked.normalized } });
      if (!current(epoch)) return;
      const result = 'updateRequirement' in out ? out.updateRequirement : out.createRequirement;
      if (result.fieldErrors.length) {
        const first = result.fieldErrors[0];
        setValidation({
          key: first.messageKey,
          params: Object.fromEntries(first.params.map((p) => [p.key, p.value])),
        });
        if (first.code === 'CONFLICT') await infinite.refresh();
        return;
      }
      if (!result.requirement) throw Error('missing requirement');
      if (editing) {
        setEditing(null);
        setEditText('');
      } else setText('');
      await infinite.refresh();
    } catch {
      if (current(epoch)) setFailure(true);
    } finally {
      if (current(epoch)) setBusy(false);
    }
  }
  async function remove(id: string, version: number) {
    const epoch = ++operation.current;
    setBusy(true);
    setFailure(false);
    try {
      const result = await query(RemoveRequirementDocument, { id, projectId, version });
      if (!current(epoch)) return;
      if (!result.deleteRequirement.deleted) {
        setValidation({ key: 'validation.conflict', params: {} });
        await infinite.refresh();
        return;
      }
      await infinite.refresh();
    } catch {
      if (current(epoch)) setFailure(true);
    } finally {
      if (current(epoch)) setBusy(false);
    }
  }
  return (
    <section className="ui-panel" aria-labelledby="requirements-heading">
      <h2 id="requirements-heading">{t(locale, 'requirements.title')}</h2>
      <form id="requirement-entry" onSubmit={save} noValidate>
        <label>
          {t(locale, 'requirements.text')}
          <textarea
            disabled={busy || !!editing}
            value={text}
            onChange={(event) => setText(event.target.value)}
            aria-invalid={!!validation}
          />
        </label>
        {validation && (
          <p role="alert" className="error">
            {t(locale, validation.key, validation.params)}
          </p>
        )}
        <button type="submit" disabled={busy}>
          {t(locale, 'requirements.add')}
        </button>
      </form>
      {editing && (
        <dialog
          ref={dialogRef}
          aria-labelledby="requirement-edit-title"
          onCancel={(event) => {
            if (busy) event.preventDefault();
            else {
              setEditing(null);
              setEditText('');
              setValidation(null);
            }
          }}
        >
          <h3 id="requirement-edit-title">{t(locale, 'requirements.edit')}</h3>
          <form onSubmit={save} noValidate>
            <label>
              {t(locale, 'requirements.text')}
              <textarea
                disabled={busy}
                value={editText}
                onChange={(event) => setEditText(event.target.value)}
                aria-invalid={!!validation}
              />
            </label>
            {validation && (
              <p role="alert" className="error">
                {t(locale, validation.key, validation.params)}
              </p>
            )}
            {mutationFailure && (
              <p role="alert" className="error">
                {t(locale, 'requirements.error')}
              </p>
            )}
            <button type="submit" disabled={busy}>
              {t(locale, 'requirements.save')}
            </button>
            {validation?.key === 'validation.conflict' &&
              rows.find((row) => row.id === editing.id) && (
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => {
                    const latestRow = rows.find((row) => row.id === editing.id);
                    if (latestRow) {
                      setEditing({ id: latestRow.id, version: latestRow.version });
                      setValidation(null);
                    }
                  }}
                >
                  {t(locale, 'requirements.useLatest')}
                </button>
              )}
            <button
              type="button"
              disabled={busy}
              onClick={() => {
                setEditing(null);
                setEditText('');
                setValidation(null);
              }}
            >
              {t(locale, 'requirements.cancel')}
            </button>
          </form>
        </dialog>
      )}
      <ResourceState
        locale={locale}
        loading={loading}
        error={mutationFailure || infinite.error}
        hasData={loaded}
        retry={() => {
          setFailure(false);
          void infinite.retry().catch(() => {});
        }}
      />
      {seeking && loaded && !loading && !infinite.error && !found && !infinite.hasNextPage && (
        <p role="alert" className="error">
          {t(locale, 'ui.sourceNotFound')}
        </p>
      )}
      <RetryFailedSummaries projectId={projectId} locale={locale} />
      <Group
        anchorId="requirements-list"
        id={`${projectId}:requirements`}
        title={t(locale, 'ui.requirementsGroup')}
        summary={t(locale, 'list.total', { total: infinite.totalCount })}
        defaultOpen
      >
        {loaded && rows.length === 0 && <p>{t(locale, 'requirements.empty')}</p>}
        <VirtualList
          identity={`requirements:${projectId}`}
          items={rows}
          keyOf={(row) => row.id}
          focusedId={editing?.id}
          scrollToId={sourceTarget}
          onEndReached={more}
          renderItem={(row) => (
            <article className="card" id={`requirement-${row.id}`} key={row.id} tabIndex={-1}>
              {source === row.id && mention && mentionVersion !== row.version && (
                <p role="status">{t(locale, 'glossary.mentionChanged')}</p>
              )}
              <RequirementSummary
                showOriginal={source === row.id && !!mention}
                projectId={projectId}
                requirementId={row.id}
                requirementVersion={row.version}
                highlight={
                  source === row.id && mentionVersion === row.version ? mention : undefined
                }
                original={row.text}
                locale={locale}
                summary={summaries.get(row.id)}
                loadFailure={summaryFailure}
                loading={summaryLoading}
                refresh={refreshSummaries}
              />
              <Group
                id={`${projectId}:requirement:${row.id}:classification`}
                title={t(locale, 'nav.classification')}
                defaultOpen={false}
                lazy
              >
                <ClassificationCard
                  projectId={projectId}
                  requirementId={row.id}
                  original={row.text}
                  version={row.version}
                  locale={locale}
                />
              </Group>
              <p className="muted">{t(locale, 'requirements.version', { version: row.version })}</p>
              <button
                type="button"
                disabled={busy}
                onClick={(event) => {
                  editTrigger.current = event.currentTarget;
                  setEditing({ id: row.id, version: row.version });
                  setEditText(row.text);
                  setValidation(null);
                  setFailure(false);
                }}
              >
                {t(locale, 'requirements.edit')}
              </button>
              <button
                type="button"
                disabled={busy}
                className="destructive-action"
                onClick={() => void remove(row.id, row.version)}
              >
                {t(locale, 'requirements.delete')}
              </button>
            </article>
          )}
        />
        <LoadMore locale={locale} {...infinite} loadMore={more} />
      </Group>
    </section>
  );
}
