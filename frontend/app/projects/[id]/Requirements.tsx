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
  RequirementsDocument,
} from '../../../lib/generated';
import { type Locale, t } from '../../../lib/i18n';
import { findRequirementPage } from '../../../lib/requirementsView';
import { validateRequirement } from '../../../lib/requirementValidation';
import { useInfinite } from '../../../lib/useInfinite';
import { useListMode } from '../../../lib/useListMode';
import Group from '../../components/Group';
import { ListModeSelect, LoadMore } from '../../components/ListControls';
import ResourceState from '../../components/ResourceState';
import { VirtualList } from '../../components/VirtualList';
import RequirementSummary, { useRequirementSummaries } from './RequirementSummary';
import { useRequirementsPage } from './useRequirementsPage';

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
  const [sourceError, setSourceError] = useState<'missing' | 'failed' | null>(null);
  const [sourceAttempt, setSourceAttempt] = useState(0);
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
    setSourceError(null);
  }, []);
  useEffect(() => {
    const dialog = dialogRef.current;
    if (editing && dialog && !dialog.open) dialog.showModal();
    if (!editing && dialog?.open) dialog.close();
    if (!editing) editTrigger.current?.focus({ preventScroll: true });
  }, [editing]);
  const location = useListMode();
  const { mode, setMode, setPage, replacePage } = location;
  const paged = useRequirementsPage(projectId, clear, {
    enabled: mode === 'pages',
    page: location.page,
    setPage,
  });
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
    enabled: mode === 'infinite',
  });
  const [mutationFailure, setFailure] = useState(false);
  const rows = mode === 'pages' ? paged.rows : infinite.rows;
  const {
    summaries,
    failure: summaryFailure,
    retryLoad: refreshSummaries,
  } = useRequirementSummaries(
    projectId,
    rows.map((row) => ({ id: row.id, version: row.version })),
  );
  const page = mode === 'pages' ? paged.page : 0;
  const loading = mode === 'pages' ? paged.loading : infinite.loading;
  const loaded = mode === 'pages' ? paged.loaded : infinite.loaded;
  const failure = mutationFailure || (mode === 'pages' ? paged.failure : infinite.error);
  const load = mode === 'pages' ? paged.load : infinite.refresh;
  const more = useCallback(() => {
    if (!infinite.loading && !infinite.error && infinite.hasNextPage)
      void infinite.loadMore().catch(() => {});
  }, [infinite.loading, infinite.error, infinite.hasNextPage, infinite.loadMore]);
  const params = useSearchParams();
  const source = params.get('requirement');
  useEffect(() => {
    void projectId;
    active.current = true;
    return () => {
      active.current = false;
      operation.current++;
    };
  }, [projectId]);
  useEffect(() => {
    void sourceAttempt;
    if (!source) return;
    let current = true;
    const epoch = operation.current;
    setSourceError(null);
    void findRequirementPage(
      source,
      async (offset) =>
        (await query(RequirementsDocument, { projectId, limit: 100, offset })).requirements,
      () => current && epoch === operation.current,
    )
      .then((target) => {
        if (!current || target === undefined) return;
        if (target === null) {
          setSourceError('missing');
          return;
        }
        setSourceTarget(source);
        replacePage(target);
      })
      .catch(() => {
        if (current && epoch === operation.current) setSourceError('failed');
      });
    return () => {
      current = false;
    };
  }, [projectId, source, sourceAttempt, replacePage]);
  useEffect(() => {
    if (!sourceTarget || loading || !rows.some((row) => row.id === sourceTarget)) return;
    const card = document.getElementById(`requirement-${sourceTarget}`);
    const group = card?.closest('details');
    if (group) group.open = true;
    card?.scrollIntoView({ block: 'center' });
    card?.focus({ preventScroll: true });
    setSourceTarget(undefined);
  }, [rows, loading, sourceTarget]);
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
        if (first.code === 'CONFLICT') await load();
        return;
      }
      if (!result.requirement) throw Error('missing requirement');
      if (editing) {
        setEditing(null);
        setEditText('');
      } else setText('');
      if (!editing && page !== 0) setPage(0);
      else await load();
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
        await load();
        return;
      }
      // Stay on a populated page after deleting its last row.
      if (rows.length === 1 && page > 0) setPage(page - 1);
      else await load();
    } catch {
      if (current(epoch)) setFailure(true);
    } finally {
      if (current(epoch)) setBusy(false);
    }
  }
  return (
    <section aria-labelledby="requirements-heading">
      <h2 id="requirements-heading">{t(locale, 'requirements.title')}</h2>
      <form onSubmit={save} noValidate>
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
        error={failure || sourceError === 'failed'}
        hasData={loaded}
        retry={() => {
          setFailure(false);
          if (mode === 'pages') void paged.load();
          else void infinite.retry().catch(() => {});
          setSourceAttempt((value) => value + 1);
        }}
      />
      {sourceError === 'missing' && (
        <p role="alert" className="error">
          {t(locale, 'ui.sourceNotFound')}
        </p>
      )}
      <ListModeSelect locale={locale} mode={mode} onChange={setMode} />
      <Group
        id={`${projectId}:requirements`}
        title={t(locale, 'ui.requirementsGroup')}
        summary={
          mode === 'pages'
            ? `${t(locale, 'ui.currentPage')} · ${rows.length}`
            : t(locale, 'list.total', { total: infinite.totalCount })
        }
        defaultOpen
      >
        {loaded && rows.length === 0 && <p>{t(locale, 'requirements.empty')}</p>}
        <VirtualList
          identity={`requirements:${projectId}:${mode}:${page}`}
          restoreScroll={mode === 'infinite'}
          overscan={mode === 'pages' ? 20 : 4}
          items={rows}
          keyOf={(row) => row.id}
          focusedId={editing?.id}
          scrollToId={sourceTarget}
          onEndReached={mode === 'infinite' ? more : undefined}
          renderItem={(row) => (
            <article className="card" id={`requirement-${row.id}`} key={row.id} tabIndex={-1}>
              <RequirementSummary
                projectId={projectId}
                requirementId={row.id}
                requirementVersion={row.version}
                original={row.text}
                locale={locale}
                summary={summaries.get(row.id)}
                loadFailure={summaryFailure}
                refresh={refreshSummaries}
              />
              <p className="muted">{t(locale, 'requirements.version', { version: row.version })}</p>
              <button
                type="button"
                disabled={busy || (mode === 'pages' && paged.requestedPage !== page)}
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
                disabled={busy || (mode === 'pages' && paged.requestedPage !== page)}
                onClick={() => void remove(row.id, row.version)}
              >
                {t(locale, 'requirements.delete')}
              </button>
            </article>
          )}
        />
        {mode === 'infinite' && <LoadMore locale={locale} {...infinite} loadMore={more} />}
      </Group>
      {mode === 'pages' && (
        <div className="pagination">
          <button
            type="button"
            disabled={page === 0 || loading || busy}
            onClick={() => setPage(page - 1)}
          >
            {t(locale, 'app.previous')}
          </button>
          <span>{t(locale, 'app.page', { page: page + 1 })}</span>
          <button
            type="button"
            disabled={rows.length < 20 || loading || busy}
            onClick={() => setPage(page + 1)}
          >
            {t(locale, 'app.next')}
          </button>
        </div>
      )}
    </section>
  );
}
