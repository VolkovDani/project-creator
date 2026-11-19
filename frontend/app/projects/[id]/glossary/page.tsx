'use client';
import { useCallback, useState } from 'react';
import { query } from '../../../../lib/client';
import {
  DeleteTermDocument,
  GlossaryCandidatePagesDocument,
  GlossaryTermPagesDocument,
  type GlossaryFieldsFragment as Term,
} from '../../../../lib/generated';
import { t } from '../../../../lib/i18n';
import { useListMode } from '../../../../lib/useListMode';
import { ListModeSelect } from '../../../components/ListControls';
import GlossaryEditor, { type GlossaryDraft } from '../GlossaryEditor';
import { useProject } from '../ProjectShell';
import { GlossaryListView } from './GlossaryListView';

export default function GlossaryPage() {
  const { project, locale } = useProject();
  const projectId = project.id;
  const tag = `project:${projectId}:glossary`,
    reqTag = `project:${projectId}:requirements`;
  const [search, setSearch] = useState('');
  const { mode, setMode } = useListMode();
  const [draft, setDraft] = useState<GlossaryDraft>(),
    [remove, setRemove] = useState<Term>(),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(false);
  const load = useCallback(
    async (after: string | null) =>
      (await query(GlossaryTermPagesDocument, { projectId, search, after }, { tags: [tag] }))
        .glossaryTermsConnection,
    [projectId, search, tag],
  );
  const scan = useCallback(
    async (after: string | null) => {
      const page = (
        await query(GlossaryCandidatePagesDocument, { projectId, after }, { tags: [tag, reqTag] })
      ).glossaryCandidatesConnection;
      return {
        ...page,
        items: page.items.map((item) => ({
          ...item,
          id: JSON.stringify([item.sourceId, item.name, item.abbreviation]),
        })),
      };
    },
    [projectId, tag, reqTag],
  );
  return (
    <section>
      <h2>{t(locale, 'nav.glossary')}</h2>
      <ListModeSelect locale={locale} mode={mode} onChange={setMode} />
      <label>
        {t(locale, 'glossary.search')}
        <input
          type="search"
          value={search}
          onChange={(e) => {
            setSearch(e.target.value);
          }}
        />
      </label>
      <button type="button" onClick={() => setDraft({ name: '' })}>
        {t(locale, 'glossary.new')}
      </button>
      {draft && (
        <GlossaryEditor
          key={JSON.stringify(draft)}
          draft={draft}
          projectId={projectId}
          locale={locale}
          onClose={() => setDraft(undefined)}
        />
      )}
      <GlossaryListView
        key={search}
        mode={mode}
        locale={locale}
        load={load}
        identity={JSON.stringify(['glossary', projectId, search])}
        tags={[tag]}
        empty={t(locale, 'glossary.empty')}
        render={(term) => (
          <article className="card" key={term.id}>
            <h3>
              <button type="button" onClick={() => setDraft({ name: term.name, term })}>
                {term.name}
              </button>
            </h3>
            <p>{term.definition}</p>
            <p>{term.context}</p>
            {term.variants.map((v) => (
              <div key={v.text}>
                {v.text} · {t(locale, `glossary.${v.kind}`)}
              </div>
            ))}
            <button
              type="button"
              onClick={() => {
                setRemove(term);
                setError(false);
              }}
            >
              {t(locale, 'requirements.delete')}
            </button>
            {remove?.id === term.id && (
              <div>
                <button
                  type="button"
                  disabled={busy}
                  onClick={async () => {
                    setBusy(true);
                    try {
                      const r = await query(
                        DeleteTermDocument,
                        { projectId, id: term.id, version: remove.version },
                        { tags: [tag] },
                      );
                      if (!r.deleteGlossaryTerm.deleted) setError(true);
                      else setRemove(undefined);
                    } catch {
                      setError(true);
                    } finally {
                      setBusy(false);
                    }
                  }}
                >
                  {t(locale, 'glossary.confirmDelete')}
                </button>
                <button type="button" onClick={() => setRemove(undefined)}>
                  {t(locale, 'requirements.cancel')}
                </button>
                {error && <p role="alert">{t(locale, 'validation.conflict')}</p>}
              </div>
            )}
          </article>
        )}
      />
      <h3>{t(locale, 'glossary.candidates')}</h3>
      <GlossaryListView
        mode={mode}
        locale={locale}
        load={scan}
        identity={JSON.stringify(['glossaryCandidates', projectId])}
        tags={[reqTag, tag]}
        empty={t(locale, 'glossary.noCandidates')}
        render={(c) => (
          <button
            key={JSON.stringify(c)}
            type="button"
            onClick={() => setDraft({ ...c, variant: c.abbreviation })}
          >
            {c.name} ({c.abbreviation})
          </button>
        )}
      />
    </section>
  );
}
