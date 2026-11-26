'use client';
import { useSearchParams } from 'next/navigation';
import { useCallback, useEffect, useRef, useState } from 'react';
import contract from '../../../../contracts/requirement.json';
import { query } from '../../../lib/client';
import {
  AddRequirementDocument,
  ArchivedRequirementsDocument,
  type ArchivedRequirementsQuery,
  ArchiveRequirementDocument,
  EditRequirementDocument,
  RemoveRequirementDocument,
  RequirementConnectionDocument,
  RestoreRequirementDocument,
} from '../../../lib/generated';
import { type Locale, t } from '../../../lib/i18n';
import { useInvalidation } from '../../../lib/live';
import { validateChangeReason, validateRequirement } from '../../../lib/requirementValidation';
import { useInfinite } from '../../../lib/useInfinite';
import Group from '../../components/Group';
import { LoadMore } from '../../components/ListControls';
import ResourceState from '../../components/ResourceState';
import { VirtualList } from '../../components/VirtualList';
import ProjectSearch from './RagTools';
import RequirementConfirmation from './RequirementConfirmation';
import RequirementDetails from './RequirementDetails';
import { useRequirementSummaries } from './RequirementSummary';
import RetryFailedSummaries from './RetryFailedSummaries';
import styles from './requirements/requirements.module.css';
import { useNarrowWorkspace } from './useNarrowWorkspace';

type Archived = ArchivedRequirementsQuery['requirementsConnection']['edges'][number]['node'];
type Selection = { id: string; archived: boolean };
type Confirmation = {
  id: string;
  version: number;
  text: string;
  archivedAt?: string | null;
  action: 'archive' | 'delete';
};

export default function Requirements({
  projectId,
  projectName,
  locale,
}: {
  projectId: string;
  projectName: string;
  locale: Locale;
}) {
  const [busy, setBusy] = useState(false);
  const [adding, setAdding] = useState(false);
  const [text, setText] = useState('');
  const [editText, setEditText] = useState('');
  const [changeReason, setChangeReason] = useState('');
  const [editing, setEditing] = useState<{ id: string; version: number } | null>(null);
  const [confirmation, setConfirmation] = useState<Confirmation | null>(null);
  const [archiveError, setArchiveError] = useState<
    'validation.conflict' | 'requirements.error' | null
  >(null);
  const [selection, setSelection] = useState<Selection | null>(null);
  const [showDetails, setShowDetails] = useState(false);
  const [archiveOpen, setArchiveOpen] = useState(false);
  const narrow = useNarrowWorkspace();
  const addTrigger = useRef<HTMLButtonElement>(null);
  const createDialog = useRef<HTMLDialogElement>(null);
  const editTrigger = useRef<HTMLButtonElement>(null);
  const rowTrigger = useRef<HTMLButtonElement>(null);
  const dialogRef = useRef<HTMLDialogElement>(null);
  const mobileDetails = useRef<HTMLDialogElement>(null);
  const detailArticle = useRef<HTMLElement>(null);
  const detailPane = useRef<HTMLDivElement>(null);
  const [validation, setValidation] = useState<{
    key: string;
    field?: string;
    code: string;
    params: Record<string, number>;
  } | null>(null);
  const [sourceTarget, setSourceTarget] = useState<string>();
  const operation = useRef(0);
  const active = useRef(true);
  const clear = useCallback(() => {
    operation.current++;
    setAdding(false);
    setText('');
    setEditText('');
    setChangeReason('');
    setEditing(null);
    setConfirmation(null);
    setArchiveError(null);
    setSelection(null);
    setShowDetails(false);
    setValidation(null);
    setFailure(false);
    setBusy(false);
    setSourceTarget(undefined);
  }, []);
  useEffect(() => {
    const dialog = createDialog.current;
    if (adding && dialog && !dialog.open) dialog.showModal();
    if (!adding && !busy && dialog?.open) {
      dialog.close();
      addTrigger.current?.focus({ preventScroll: true });
    }
  }, [adding, busy]);
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
  const loadArchived = useCallback(
    async (after: string | null) =>
      (await query(ArchivedRequirementsDocument, { projectId, first: 40, after }))
        .requirementsConnection,
    [projectId],
  );
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
  const previousSource = useRef(source);
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
    const nextSource = params.get('requirement');
    if (previousSource.current && !nextSource) setShowDetails(false);
    previousSource.current = nextSource;
    setSourceTarget(nextSource ?? undefined);
  }, [params]);
  const seeking = !!sourceTarget && sourceTarget === source;
  const activeTarget = seeking ? rows.find((row) => row.id === sourceTarget) : undefined;
  const seekArchive =
    seeking && loaded && !loading && !infinite.error && !activeTarget && !infinite.hasNextPage;
  const archived = useInfinite({
    identity: `archived-requirements:${projectId}`,
    tags: [`project:${projectId}`, `project:${projectId}:requirements`],
    load: loadArchived,
    enabled: archiveOpen || seekArchive || selection?.archived === true,
  });
  const archivedTarget = seekArchive
    ? archived.rows.find((row) => row.id === sourceTarget)
    : undefined;
  const activeSelection = selection?.archived
    ? undefined
    : selection
      ? rows.find((row) => row.id === selection.id)
      : rows[0];
  const archivedSelection = selection?.archived
    ? archived.rows.find((row) => row.id === selection.id)
    : undefined;
  const selected = activeSelection ?? archivedSelection;
  const selectedId = selected?.id;

  useEffect(() => {
    if (!seeking || !loaded || loading || infinite.error) return;
    const target = activeTarget ?? archivedTarget;
    if (target) {
      if (archivedTarget) {
        const archive = document.querySelector<HTMLDetailsElement>('#requirements-archive');
        if (archive) archive.open = true;
      }
      const trigger = document.getElementById(`requirement-row-${target.id}`);
      rowTrigger.current = trigger instanceof HTMLButtonElement ? trigger : null;
      setSelection({ id: target.id, archived: !!archivedTarget });
      setShowDetails(true);
    } else if (!seekArchive) more();
    else if (archived.loaded && !archived.loading && !archived.error && archived.hasNextPage)
      void archived.loadMore().catch(() => {});
  }, [
    seeking,
    loaded,
    loading,
    infinite.error,
    activeTarget,
    archivedTarget,
    seekArchive,
    more,
    archived.loaded,
    archived.loading,
    archived.error,
    archived.hasNextPage,
    archived.loadMore,
  ]);

  useEffect(() => {
    const modal = mobileDetails.current;
    if (narrow && showDetails && selectedId && modal && !modal.open) modal.showModal();
    if (modal?.open && (!narrow || !showDetails || !selectedId)) modal.close();
  }, [narrow, showDetails, selectedId]);

  useEffect(() => {
    if (selectedId && detailPane.current) detailPane.current.scrollTop = 0;
  }, [selectedId]);

  useEffect(() => {
    if (!seeking || selectedId !== sourceTarget || !showDetails) return;
    const article = detailArticle.current;
    if (!article) return;
    (article.querySelector('mark[data-candidate]') ?? article).scrollIntoView({ block: 'nearest' });
    article.focus({ preventScroll: true });
    setSourceTarget(undefined);
  }, [seeking, selectedId, sourceTarget, showDetails]);

  function closeDetails() {
    mobileDetails.current?.close();
    setShowDetails(false);
    rowTrigger.current?.focus({ preventScroll: true });
  }

  function chooseRow(row: { id: string; text: string }, inArchive: boolean) {
    const excerpt = row.text.trim().replace(/\s+/g, ' ');
    const preview = excerpt.length > 160 ? `${excerpt.slice(0, 159)}…` : excerpt;
    return (
      <button
        id={`requirement-row-${row.id}`}
        type="button"
        className={styles.row}
        aria-pressed={selectedId === row.id}
        onClick={(event) => {
          rowTrigger.current = event.currentTarget;
          setSourceTarget(undefined);
          setArchiveError(null);
          setSelection({ id: row.id, archived: inArchive });
          setShowDetails(true);
        }}
      >
        {preview}
      </button>
    );
  }
  const current = (epoch: number) => active.current && operation.current === epoch;
  function closeCreate() {
    setAdding(false);
    setText('');
    setValidation(null);
    setFailure(false);
  }
  async function save(event: React.FormEvent) {
    event.preventDefault();
    if (busy || (!adding && !editing)) return;
    const submittedText = editing ? editText : text;
    const checked = validateRequirement(contract, submittedText);
    const first =
      checked.errors[0] ?? (editing ? validateChangeReason(contract, changeReason)[0] : undefined);
    setValidation(
      first
        ? {
            field: first.path.at(-1),
            code: first.code,
            key: first.messageKey,
            params: first.params,
          }
        : null,
    );
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
              changeReason,
            },
          })
        : await query(AddRequirementDocument, { input: { projectId, text: checked.normalized } });
      if (!current(epoch)) return;
      const result = 'updateRequirement' in out ? out.updateRequirement : out.createRequirement;
      if (result.fieldErrors.length) {
        const first = result.fieldErrors[0];
        setValidation({
          field: first.path.at(-1),
          code: first.code,
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
        setChangeReason('');
      } else {
        closeCreate();
        setSelection({ id: result.requirement.id, archived: false });
      }
      await infinite.refresh();
    } catch {
      if (current(epoch)) setFailure(true);
    } finally {
      if (current(epoch)) setBusy(false);
    }
  }
  async function changeArchive(
    entry: Pick<Archived, 'id' | 'version'> & { archivedAt?: string | null },
    action: 'archive' | 'restore' | 'delete',
  ) {
    setConfirmation(null);
    if (busy || (action !== 'archive' && !entry.archivedAt)) return;
    setArchiveError(null);
    const epoch = ++operation.current;
    setBusy(true);
    const identity = { id: entry.id, projectId, version: entry.version };
    try {
      const archivedAt = entry.archivedAt ?? '';
      const vars = { ...identity, archivedAt };
      const changed =
        action === 'archive'
          ? (await query(ArchiveRequirementDocument, identity)).archiveRequirement
          : action === 'restore'
            ? (await query(RestoreRequirementDocument, vars)).restoreRequirement
            : (await query(RemoveRequirementDocument, vars)).deleteRequirement.deleted;
      if (!current(epoch)) return;
      if (changed) {
        setSelection(null);
        closeDetails();
      } else setArchiveError('validation.conflict');
      await Promise.all([infinite.refresh(), ...(archived.loaded ? [archived.refresh()] : [])]);
    } catch {
      if (current(epoch)) setArchiveError('requirements.error');
    } finally {
      if (current(epoch)) setBusy(false);
    }
  }
  const details = selected && (!seeking || selectedId === sourceTarget) && (
    <RequirementDetails
      key={selected.id}
      row={selected}
      projectId={projectId}
      projectName={projectName}
      locale={locale}
      articleRef={detailArticle}
      highlight={
        source === selected.id && mentionVersion === selected.version ? mention : undefined
      }
      mentionChanged={source === selected.id && !!mention && mentionVersion !== selected.version}
      summaryProps={
        activeSelection && {
          summary: summaries.get(selected.id),
          loading: summaryLoading,
          loadFailure: summaryFailure,
          refresh: refreshSummaries,
        }
      }
    >
      {archiveError && (
        <p role="alert" className="error">
          {t(locale, archiveError)}
        </p>
      )}
      {archivedSelection ? (
        <>
          <button
            type="button"
            disabled={busy}
            onClick={() => void changeArchive(archivedSelection, 'restore')}
          >
            {t(locale, 'requirements.restore')}
          </button>
          <button
            type="button"
            className="destructive-action"
            disabled={busy}
            onClick={() => setConfirmation({ ...archivedSelection, action: 'delete' })}
          >
            {t(locale, 'requirements.purge')}
          </button>
        </>
      ) : (
        <>
          <button
            type="button"
            disabled={busy}
            onClick={(event) => {
              editTrigger.current = event.currentTarget;
              setEditing({ id: selected.id, version: selected.version });
              setEditText(selected.text);
              setChangeReason('');
              setValidation(null);
              setFailure(false);
            }}
          >
            {t(locale, 'requirements.edit')}
          </button>
          <button
            type="button"
            disabled={busy}
            onClick={() => setConfirmation({ ...selected, action: 'archive' })}
          >
            {t(locale, 'requirements.archive')}
          </button>
        </>
      )}
    </RequirementDetails>
  );
  return (
    <section className="ui-panel" aria-labelledby="requirements-heading">
      <header className={styles.heading}>
        <h2 id="requirements-heading">{t(locale, 'nav.requirements')}</h2>
        <button
          type="button"
          id="requirement-entry"
          aria-haspopup="dialog"
          ref={addTrigger}
          disabled={busy}
          onClick={() => {
            setFailure(false);
            setValidation(null);
            setAdding(true);
          }}
        >
          {t(locale, 'requirements.add')}
        </button>
      </header>
      <ProjectSearch
        projectId={projectId}
        locale={locale}
        onSource={(id) => {
          if (id === source) {
            setSourceTarget(id);
            setShowDetails(true);
          }
        }}
      />
      <dialog
        ref={createDialog}
        aria-labelledby="requirement-create-title"
        onCancel={(event) => {
          event.preventDefault();
          if (!busy) closeCreate();
        }}
      >
        <h3 id="requirement-create-title">{t(locale, 'requirements.add')}</h3>
        <form id="requirement-create-form" onSubmit={save} noValidate>
          <label>
            {t(locale, 'requirements.text')}
            <textarea
              disabled={busy}
              value={text}
              onChange={(event) => setText(event.target.value)}
              aria-invalid={adding && validation?.field === 'text'}
              aria-describedby={adding && validation ? 'requirement-create-error' : undefined}
            />
          </label>
          {adding && validation && (
            <p id="requirement-create-error" role="alert" className="error">
              {t(locale, validation.key, validation.params)}
            </p>
          )}
          {adding && mutationFailure && (
            <p role="alert" className="error">
              {t(locale, 'requirements.error')}
            </p>
          )}
          <button type="submit" disabled={busy}>
            {t(locale, 'requirements.add')}
          </button>
          <button type="button" disabled={busy} onClick={closeCreate}>
            {t(locale, 'requirements.cancel')}
          </button>
        </form>
      </dialog>
      {confirmation && (
        <RequirementConfirmation
          action={confirmation.action}
          text={confirmation.text}
          locale={locale}
          onCancel={() => setConfirmation(null)}
          onConfirm={() => void changeArchive(confirmation, confirmation.action)}
        />
      )}
      {editing && (
        <dialog
          ref={dialogRef}
          aria-labelledby="requirement-edit-title"
          onCancel={(event) => {
            if (busy) event.preventDefault();
            else {
              setEditing(null);
              setEditText('');
              setChangeReason('');
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
                aria-invalid={validation?.field === 'text'}
                aria-describedby={
                  validation?.field === 'text' ? 'requirement-edit-error' : undefined
                }
              />
            </label>
            <label>
              {t(locale, 'requirements.changeReason')}
              <textarea
                required
                disabled={busy}
                value={changeReason}
                onChange={(event) => setChangeReason(event.target.value)}
                aria-invalid={validation?.field === 'changeReason'}
                aria-describedby={
                  validation?.field === 'changeReason' ? 'requirement-edit-error' : undefined
                }
              />
            </label>
            {validation && (
              <p id="requirement-edit-error" role="alert" className="error">
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
            {validation?.code === 'CONFLICT' && rows.find((row) => row.id === editing.id) && (
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
                setChangeReason('');
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
        error={infinite.error}
        hasData={loaded}
        retry={() => {
          setFailure(false);
          void infinite.retry().catch(() => {});
        }}
      />
      {seekArchive &&
        archived.loaded &&
        !archived.loading &&
        !archived.error &&
        !archivedTarget &&
        !archived.hasNextPage && (
          <p role="alert" className="error">
            {t(locale, 'ui.sourceNotFound')}
          </p>
        )}
      <RetryFailedSummaries projectId={projectId} locale={locale} />
      <div className={styles.workspace}>
        <div className={styles.listPane}>
          <Group
            anchorId="requirements-list"
            id={`${projectId}:requirements`}
            title={t(locale, 'ui.requirementsGroup')}
            summary={t(locale, 'list.total', { total: infinite.totalCount })}
            defaultOpen
          >
            {loaded && rows.length === 0 && <p>{t(locale, 'requirements.empty')}</p>}
            <VirtualList
              identity={`requirements:${projectId}:rows`}
              items={rows}
              keyOf={(row) => row.id}
              focusedId={activeSelection?.id}
              scrollToId={activeTarget?.id}
              estimatedRowHeight={60}
              onEndReached={more}
              renderItem={(row) => chooseRow(row, false)}
            />
            <LoadMore locale={locale} {...infinite} loadMore={more} />
          </Group>
          <Group
            id={`${projectId}:archive`}
            anchorId="requirements-archive"
            title={t(locale, 'requirements.archiveTitle')}
            defaultOpen={false}
            onOpenChange={setArchiveOpen}
            lazy
          >
            <ResourceState
              locale={locale}
              loading={archived.loading}
              error={archived.error}
              hasData={archived.loaded}
              retry={() => void archived.retry().catch(() => {})}
            />
            {archived.loaded && archived.rows.length === 0 && (
              <p>{t(locale, 'requirements.archiveEmpty')}</p>
            )}
            {archived.rows.map((row) => (
              <div key={row.id}>{chooseRow(row, true)}</div>
            ))}
            <LoadMore
              locale={locale}
              {...archived}
              observe
              loadMore={() => void archived.loadMore().catch(() => {})}
            />
          </Group>
        </div>
        {narrow ? (
          <dialog
            ref={mobileDetails}
            className={styles.mobileDetails}
            aria-label={t(locale, 'requirements.detailsTitle')}
            onCancel={(event) => {
              if (event.target !== event.currentTarget) return;
              event.preventDefault();
              closeDetails();
            }}
          >
            <button type="button" onClick={closeDetails}>
              {t(locale, 'requirements.backToList')}
            </button>
            {showDetails && details}
          </dialog>
        ) : (
          <div className={styles.detailPane} ref={detailPane}>
            {details}
          </div>
        )}
      </div>
    </section>
  );
}
