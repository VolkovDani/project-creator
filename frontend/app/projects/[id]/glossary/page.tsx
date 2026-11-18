'use client';
import { useCallback, useState } from 'react';
import { query } from '../../../../lib/client';
import {
  DeleteTermDocument,
  GlossaryCandidatesDocument,
  GlossaryListDocument,
  type GlossaryFieldsFragment as Term,
} from '../../../../lib/generated';
import { t } from '../../../../lib/i18n';
import { useResource } from '../../../../lib/useResource';
import ResourceState from '../../../components/ResourceState';
import GlossaryEditor, { type GlossaryDraft } from '../GlossaryEditor';
import { useProject } from '../ProjectShell';

export default function GlossaryPage() {
  const { project, locale } = useProject();
  const projectId = project.id;
  const tag = `project:${projectId}:glossary`,
    reqTag = `project:${projectId}:requirements`;
  const [search, setSearch] = useState(''),
    [offset, setOffset] = useState(0),
    [candidateOffset, setCandidateOffset] = useState(0);
  const [draft, setDraft] = useState<GlossaryDraft>(),
    [remove, setRemove] = useState<Term>(),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(false);
  const load = useCallback(
    async () =>
      (await query(GlossaryListDocument, { projectId, search, offset, limit: 21 }, { tags: [tag] }))
        .glossaryTerms,
    [projectId, search, offset, tag],
  );
  const scan = useCallback(
    async () =>
      (
        await query(
          GlossaryCandidatesDocument,
          { projectId, offset: candidateOffset, limit: 20 },
          { tags: [tag, reqTag] },
        )
      ).glossaryCandidates,
    [projectId, candidateOffset, tag, reqTag],
  );
  const terms = useResource(load, [tag]),
    candidates = useResource(scan, [tag, reqTag]);
  return (
    <section>
      <h2>{t(locale, 'nav.glossary')}</h2>
      <label>
        {t(locale, 'glossary.search')}
        <input
          type="search"
          value={search}
          onChange={(e) => {
            setSearch(e.target.value);
            setOffset(0);
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
      <ResourceState
        locale={locale}
        loading={terms.loading}
        error={terms.error}
        hasData={!!terms.data}
        retry={terms.reload}
      />
      {terms.data?.length === 0 && <p>{t(locale, 'glossary.empty')}</p>}
      {terms.data?.slice(0, 20).map((term) => (
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
      ))}
      <button type="button" disabled={!offset} onClick={() => setOffset(offset - 20)}>
        {t(locale, 'analysis.previous')}
      </button>
      <button
        type="button"
        disabled={(terms.data?.length ?? 0) <= 20}
        onClick={() => setOffset(offset + 20)}
      >
        {t(locale, 'analysis.next')}
      </button>
      <h3>{t(locale, 'glossary.candidates')}</h3>
      <ResourceState
        locale={locale}
        loading={candidates.loading}
        error={candidates.error}
        hasData={!!candidates.data}
        retry={candidates.reload}
      />
      {candidates.data?.items.length === 0 && <p>{t(locale, 'glossary.noCandidates')}</p>}
      {candidates.data?.items.map((c) => (
        <button
          key={JSON.stringify(c)}
          type="button"
          onClick={() => setDraft({ ...c, variant: c.abbreviation })}
        >
          {c.name} ({c.abbreviation})
        </button>
      ))}
      <button
        type="button"
        disabled={!candidateOffset}
        onClick={() => setCandidateOffset(candidateOffset - 20)}
      >
        {t(locale, 'analysis.previous')}
      </button>
      <button
        type="button"
        disabled={!candidates.data?.hasMore}
        onClick={() => setCandidateOffset(candidateOffset + 20)}
      >
        {t(locale, 'analysis.next')}
      </button>
    </section>
  );
}
