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
import { type Locale, t } from '../../../lib/i18n';
import { useResource } from '../../../lib/useResource';
export type GlossaryDraft = {
  name: string;
  variant?: string;
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
                  sourceId: draft.sourceId || term?.sourceId || null,
                  sourceVersion: draft.sourceVersion || term?.sourceVersion || null,
                },
              },
              { tags: [tag] },
            );
            const payload = result.saveGlossaryTerm;
            if (payload.term) {
              onClose();
              return;
            }
            setErrors(
              payload.fieldErrors.map((err) =>
                t(
                  locale,
                  err.messageKey,
                  Object.fromEntries(err.params.map((p) => [p.key, p.value])),
                ),
              ),
            );
          } catch {
            setErrors([t(locale, 'app.submitError')]);
          } finally {
            setBusy(false);
          }
        }}
      >
        <fieldset disabled={busy}>
          <legend>{t(locale, 'glossary.editor')}</legend>
          {draft.sourceId && !draft.term && (
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
          <label>
            {t(locale, 'glossary.context')}
            <textarea value={context} onChange={(e) => setContext(e.target.value)} />
          </label>
          {variants.map((v, i) => (
            <div key={v.key}>
              <label>
                {t(locale, 'glossary.variant')}
                <input
                  required
                  value={v.text}
                  onChange={(e) =>
                    setVariants(
                      variants.map((a, n) => (n === i ? { ...a, text: e.target.value } : a)),
                    )
                  }
                />
              </label>
              <label>
                {t(locale, 'glossary.kind')}
                <select
                  value={v.kind}
                  onChange={(e) =>
                    setVariants(
                      variants.map((a, n) => (n === i ? { ...a, kind: e.target.value } : a)),
                    )
                  }
                >
                  {limits.kinds.map((kind) => (
                    <option key={kind} value={kind}>
                      {t(locale, `glossary.${kind}`)}
                    </option>
                  ))}
                </select>
              </label>
              <button type="button" onClick={() => setVariants(variants.filter((_, n) => i !== n))}>
                {t(locale, 'glossary.removeVariant')}
              </button>
            </div>
          ))}
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
