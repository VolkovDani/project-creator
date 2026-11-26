'use client';
import { useCallback, useEffect, useRef, useState } from 'react';
import limits from '../../../../contracts/glossary.json';
import { query } from '../../../lib/client';
import {
  GlossaryListDocument,
  SaveTermDocument,
  type GlossaryFieldsFragment as Term,
} from '../../../lib/generated';
import { validateGlossary } from '../../../lib/glossaryValidation';
import { fieldErrorMessage, type Locale, t } from '../../../lib/i18n';
import { useResource } from '../../../lib/useResource';
import CandidateDefinition, { type Candidate } from './CandidateDefinition';
import CandidateExcerpt from './glossary/CandidateExcerpt';
import styles from './glossary/glossary.module.css';
export type GlossaryDraft = {
  name: string;
  variant?: string;
  candidate?: Candidate;
  sourceId?: string;
  sourceVersion?: number;
  term?: Term;
};
function editableVariants(items: Term['variants']) {
  return items.map((v) => ({ ...v, key: v.text }));
}

export default function GlossaryEditor({
  projectId,
  locale,
  draft,
  onClose,
}: {
  projectId: string;
  locale: Locale;
  draft: GlossaryDraft;
  onClose: () => void;
}) {
  const [term, setTerm] = useState(draft.term);
  const [name, setName] = useState(draft.term?.name ?? draft.name);
  const [definition, setDefinition] = useState(draft.term?.definition ?? '');
  const [context, setContext] = useState(draft.term?.context ?? '');
  const [variants, setVariants] = useState(() =>
    editableVariants(
      draft.term?.variants ??
        (draft.variant ? [{ kind: 'abbreviation', text: draft.variant }] : []),
    ),
  );
  const [search, setSearch] = useState('');
  const [busy, setBusy] = useState(false);
  const [errors, setErrors] = useState<string[]>([]);
  const tag = `project:${projectId}:glossary`;
  const load = useCallback(
    async () =>
      !search.trim()
        ? []
        : (
            await query(
              GlossaryListDocument,
              { projectId, search, offset: 0, limit: 20 },
              { tags: [tag] },
            )
          ).glossaryTerms,
    [projectId, search, tag],
  );
  const options = useResource(load, [tag]);
  function select(existing: Term) {
    setTerm(existing);
    setName(existing.name);
    setDefinition(existing.definition);
    setContext(existing.context);
    const text = draft.variant ?? draft.name;
    const add =
      text &&
      ![existing.name, ...existing.variants.map((v) => v.text)].some(
        (v) => v.toLowerCase() === text.toLowerCase(),
      );
    setVariants(
      editableVariants(
        add
          ? [...existing.variants, { text, kind: draft.variant ? 'abbreviation' : 'synonym' }]
          : existing.variants,
      ),
    );
    setSearch('');
  }
  const dialog = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const element = dialog.current;
    element?.showModal();
    return () => element?.close();
  }, []);
  return (
    <dialog
      ref={dialog}
      className={styles.editor}
      aria-label={t(locale, 'glossary.editor')}
      onCancel={(e) => {
        e.preventDefault();
        if (!busy) onClose();
      }}
    >
      <form
        aria-label={t(locale, 'glossary.editor')}
        onSubmit={async (e) => {
          e.preventDefault();
          if (busy) return;
          const invalid = validateGlossary(limits, { name, definition, context, variants });
          if (invalid) {
            setErrors([t(locale, invalid.key, invalid.params)]);
            return;
          }
          setBusy(true);
          setErrors([]);
          try {
            const result = await query(
              SaveTermDocument,
              {
                input: {
                  projectId,
                  id: term?.id,
                  version: term?.version,
                  name,
                  definition,
                  context,
                  variants: variants.map(({ kind, text }) => ({ kind, text })),
                  sourceId: draft.candidate?.sourceId || draft.sourceId || term?.sourceId || null,
                  sourceVersion:
                    draft.candidate?.sourceVersion ||
                    draft.sourceVersion ||
                    term?.sourceVersion ||
                    null,
                },
              },
              { tags: [tag] },
            );
            const payload = result.saveGlossaryTerm;
            if (payload.term) {
              onClose();
              return;
            }
            setErrors(payload.fieldErrors.map((error) => fieldErrorMessage(locale, error)));
          } catch {
            setErrors([t(locale, 'app.submitError')]);
          } finally {
            setBusy(false);
          }
        }}
      >
        <fieldset disabled={busy}>
          <legend>{t(locale, 'glossary.editor')}</legend>
          {draft.candidate && (
            <blockquote className="user-content">
              <CandidateExcerpt candidate={draft.candidate} />
            </blockquote>
          )}
          {(draft.name || draft.variant) && !draft.term && (
            <>
              <label>
                {t(locale, 'glossary.linkExisting')}
                <input value={search} onChange={(e) => setSearch(e.target.value)} />
              </label>
              {search &&
                options.data?.map((item) => (
                  <button key={item.id} type="button" onClick={() => select(item)}>
                    {item.name}
                  </button>
                ))}
            </>
          )}
          <label>
            {t(locale, 'glossary.name')}
            <input required value={name} onChange={(e) => setName(e.target.value)} />
          </label>
          <label>
            {t(locale, 'glossary.definition')}
            <textarea value={definition} onChange={(e) => setDefinition(e.target.value)} />
          </label>
          {draft.candidate && (
            <CandidateDefinition
              key={JSON.stringify([term?.id, name])}
              projectId={projectId}
              candidate={draft.candidate}
              name={name}
              locale={locale}
              onApply={setDefinition}
            />
          )}
          <label>
            {t(locale, 'glossary.context')}
            <textarea value={context} onChange={(e) => setContext(e.target.value)} />
          </label>
          {variants.length > 0 && (
            <table
              className={`${styles.variants} ${styles.editableVariants}`}
              aria-label={t(locale, 'glossary.variants')}
            >
              <thead>
                <tr>
                  <th scope="col">{t(locale, 'glossary.variant')}</th>
                  <th scope="col">{t(locale, 'glossary.kind')}</th>
                  <th scope="col" aria-label={t(locale, 'glossary.removeVariant')} />
                </tr>
              </thead>
              <tbody>
                {variants.map((variant, index) => {
                  const label = (key: string) =>
                    t(locale, 'glossary.variantControl', { label: t(locale, key), row: index + 1 });
                  return (
                    <tr key={variant.key}>
                      <td>
                        <input
                          aria-label={label('glossary.variant')}
                          required
                          value={variant.text}
                          onChange={(event) =>
                            setVariants(
                              variants.map((item) =>
                                item.key === variant.key
                                  ? { ...item, text: event.target.value }
                                  : item,
                              ),
                            )
                          }
                        />
                      </td>
                      <td>
                        <select
                          aria-label={label('glossary.kind')}
                          title={t(locale, `glossary.${variant.kind}`)}
                          value={variant.kind}
                          onChange={(event) =>
                            setVariants(
                              variants.map((item) =>
                                item.key === variant.key
                                  ? { ...item, kind: event.target.value }
                                  : item,
                              ),
                            )
                          }
                        >
                          {limits.kinds.map((kind) => (
                            <option key={kind} value={kind}>
                              {t(locale, `glossary.${kind}`)}
                            </option>
                          ))}
                        </select>
                      </td>
                      <td>
                        <button
                          type="button"
                          aria-label={label('glossary.removeVariant')}
                          title={t(locale, 'glossary.removeVariant')}
                          onClick={() =>
                            setVariants(variants.filter((item) => item.key !== variant.key))
                          }
                        >
                          <span aria-hidden="true">×</span>
                        </button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
          <button
            type="button"
            disabled={variants.length >= limits.variantLimit}
            onClick={() =>
              setVariants([...variants, { key: crypto.randomUUID(), text: '', kind: 'synonym' }])
            }
          >
            {t(locale, 'glossary.addVariant')}
          </button>
          {errors.length > 0 && <p role="alert">{errors.join(' ')}</p>}
          <button type="submit">{t(locale, 'glossary.confirm')}</button>
          <button type="button" onClick={onClose}>
            {t(locale, 'requirements.cancel')}
          </button>
        </fieldset>
      </form>
    </dialog>
  );
}
