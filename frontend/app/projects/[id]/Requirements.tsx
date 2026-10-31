'use client';
import { useSearchParams } from 'next/navigation';
import { useCallback, useEffect, useRef, useState } from 'react';
import contract from '../../../../contracts/requirement.json';
import { query } from '../../../lib/client';
import {
  AddRequirementDocument,
  EditRequirementDocument,
  RemoveRequirementDocument,
  RequirementsDocument,
} from '../../../lib/generated';
import { type Locale, t } from '../../../lib/i18n';
import { findRequirementPage } from '../../../lib/requirementsView';
import { validateRequirement } from '../../../lib/requirementValidation';
import Group from '../../components/Group';
import ResourceState from '../../components/ResourceState';
import { useRequirementsPage } from './useRequirementsPage';

export default function Requirements({ projectId, locale }: { projectId: string; locale: Locale }) {
  const [busy, setBusy] = useState(false);
  const [text, setText] = useState('');
  const [editing, setEditing] = useState<{ id: string; version: number } | null>(null);
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
    setEditing(null);
    setValidation(null);
    setBusy(false);
    setSourceTarget(undefined);
    setSourceError(null);
  }, []);
  const list = useRequirementsPage(projectId, clear);
  const { rows, page, loading, loaded, failure, setFailure, setPage, load } = list;
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
        setPage(target);
      })
      .catch(() => {
        if (current && epoch === operation.current) setSourceError('failed');
      });
    return () => {
      current = false;
    };
  }, [projectId, source, sourceAttempt, setPage]);
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
    const checked = validateRequirement(contract, text),
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
      setText('');
      setEditing(null);
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
            disabled={busy}
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
          {t(locale, editing ? 'requirements.save' : 'requirements.add')}
        </button>
        {editing &&
          validation?.key === 'validation.conflict' &&
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
        {editing && (
          <button
            type="button"
            disabled={busy}
            onClick={() => {
              setEditing(null);
              setText('');
              setValidation(null);
            }}
          >
            {t(locale, 'requirements.cancel')}
          </button>
        )}
      </form>
      <ResourceState
        locale={locale}
        loading={loading}
        error={failure || sourceError === 'failed'}
        hasData={loaded}
        retry={() => {
          void load();
          setSourceAttempt((value) => value + 1);
        }}
      />
      {sourceError === 'missing' && (
        <p role="alert" className="error">
          {t(locale, 'ui.sourceNotFound')}
        </p>
      )}
      <Group
        id={`${projectId}:requirements`}
        title={t(locale, 'ui.requirementsGroup')}
        summary={`${t(locale, 'ui.currentPage')} · ${rows.length}`}
        defaultOpen
      >
        {loaded && rows.length === 0 && <p>{t(locale, 'requirements.empty')}</p>}
        {rows.map((row) => (
          <article className="card" id={`requirement-${row.id}`} key={row.id} tabIndex={-1}>
            <p>{row.text}</p>
            <p className="muted">{t(locale, 'requirements.version', { version: row.version })}</p>
            <button
              type="button"
              disabled={busy || list.requestedPage !== page}
              onClick={() => {
                setEditing({ id: row.id, version: row.version });
                setText(row.text);
                setValidation(null);
              }}
            >
              {t(locale, 'requirements.edit')}
            </button>
            <button
              type="button"
              disabled={busy || list.requestedPage !== page}
              onClick={() => void remove(row.id, row.version)}
            >
              {t(locale, 'requirements.delete')}
            </button>
          </article>
        ))}
      </Group>
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
    </section>
  );
}
