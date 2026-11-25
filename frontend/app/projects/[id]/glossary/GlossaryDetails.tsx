'use client';
import { useEffect, useRef, useState } from 'react';
import { query } from '../../../../lib/client';
import { DeleteTermDocument, type GlossaryFieldsFragment as Term } from '../../../../lib/generated';
import { type Locale, t } from '../../../../lib/i18n';
import ConfirmationDialog from '../../../components/ConfirmationDialog';
import GlossaryEditor from '../GlossaryEditor';
import layout from '../requirements/requirements.module.css';
import GlossaryMentions from './GlossaryMentions';
import styles from './glossary.module.css';

export function GlossaryDetails({
  projectId,
  locale,
  term,
  narrow,
  onClose,
  onDeleted,
}: {
  projectId: string;
  locale: Locale;
  term: Term;
  narrow: boolean;
  onClose: () => void;
  onDeleted: () => void;
}) {
  const [editing, setEditing] = useState(false);
  const [mentions, setMentions] = useState(false);
  const [remove, setRemove] = useState<{ name: string; version: number }>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(false);
  const modal = useRef<HTMLDialogElement>(null);
  const hasActionDialog = editing || mentions || !!remove;

  useEffect(() => {
    if (!narrow) return;
    const element = modal.current;
    return () => element?.close();
  }, [narrow]);

  useEffect(() => {
    const element = modal.current;
    if (narrow && !hasActionDialog && element && !element.open) element.showModal();
  }, [narrow, hasActionDialog]);

  function closeDetails() {
    modal.current?.close();
    onClose();
  }

  async function deleteTerm(version: number) {
    setBusy(true);
    setError(false);
    try {
      const result = await query(
        DeleteTermDocument,
        { projectId, id: term.id, version },
        { tags: [`project:${projectId}:glossary`] },
      );
      if (result.deleteGlossaryTerm.deleted) onDeleted();
      else setError(true);
    } catch {
      setError(true);
    } finally {
      setBusy(false);
    }
  }

  const article = (
    <article className="card">
      <div className={layout.heading}>
        <h3>{term.name}</h3>
        <button type="button" onClick={() => setEditing(true)}>
          {t(locale, 'glossary.edit')}
        </button>
      </div>
      {term.definition && (
        <>
          <h4>{t(locale, 'glossary.definition')}</h4>
          <p className="user-content">{term.definition}</p>
        </>
      )}
      {term.context && (
        <>
          <h4>{t(locale, 'glossary.context')}</h4>
          <p className="user-content">{term.context}</p>
        </>
      )}
      {term.variants.length > 0 && (
        <table className={styles.variants} aria-label={t(locale, 'glossary.variants')}>
          <thead>
            <tr>
              <th scope="col">{t(locale, 'glossary.variant')}</th>
              <th scope="col">{t(locale, 'glossary.kind')}</th>
            </tr>
          </thead>
          <tbody>
            {term.variants.map(({ text, kind }) => (
              <tr key={text}>
                <td>{text}</td>
                <td>{t(locale, `glossary.${kind}`)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <button type="button" onClick={() => setMentions(true)}>
        {t(locale, 'glossary.mentions')}
      </button>
      <button
        className="destructive-action"
        type="button"
        onClick={() => {
          setRemove({ name: term.name, version: term.version });
          setError(false);
        }}
      >
        {t(locale, 'requirements.delete')}
      </button>
    </article>
  );

  return (
    <>
      {narrow ? (
        <dialog
          ref={modal}
          className={layout.mobileDetails}
          aria-label={term.name}
          onCancel={(event) => {
            event.preventDefault();
            closeDetails();
          }}
        >
          <button type="button" onClick={closeDetails}>
            {t(locale, 'glossary.backToList')}
          </button>
          {article}
        </dialog>
      ) : (
        <div className={layout.detailPane}>{article}</div>
      )}
      {editing && (
        <GlossaryEditor
          draft={{ name: term.name, term }}
          projectId={projectId}
          locale={locale}
          onClose={() => setEditing(false)}
        />
      )}
      {mentions && (
        <GlossaryMentions
          projectId={projectId}
          locale={locale}
          term={term}
          onClose={() => setMentions(false)}
        />
      )}
      {remove && (
        <ConfirmationDialog
          title={t(locale, 'glossary.deleteQuestion', { name: remove.name })}
          confirmLabel={t(locale, 'glossary.confirmDelete')}
          destructive
          busy={busy}
          locale={locale}
          onCancel={() => setRemove(undefined)}
          onConfirm={() => void deleteTerm(remove.version)}
        >
          {error && <p role="alert">{t(locale, 'validation.conflict')}</p>}
        </ConfirmationDialog>
      )}
    </>
  );
}
