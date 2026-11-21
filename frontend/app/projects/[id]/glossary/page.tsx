'use client';
import Link from 'next/link';
import { useCallback, useState } from 'react';
import { query } from '../../../../lib/client';
import {
  DeleteTermDocument,
  GlossaryCandidatePagesDocument,
  GlossaryTermPagesDocument,
  type GlossaryFieldsFragment as Term,
} from '../../../../lib/generated';
import { t } from '../../../../lib/i18n';
import { requirementURL } from '../../../../lib/rag';
import GlossaryEditor, { type GlossaryDraft } from '../GlossaryEditor';
import { useProject } from '../ProjectShell';
import styles from './CandidateExcerpt.module.css';
import { GlossaryListView } from './GlossaryListView';
import GlossaryMentions from './GlossaryMentions';
export default function GlossaryPage() {
  const [mentions, setMentions] = useState<Term>();
  const { project, locale } = useProject();
  const projectId = project.id;
  const tag = `project:${projectId}:glossary`,
    reqTag = `project:${projectId}:requirements`;
  const [search, setSearch] = useState('');
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
      {mentions && (
        <GlossaryMentions
          projectId={projectId}
          locale={locale}
          term={mentions}
          onClose={() => setMentions(undefined)}
        />
      )}
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
            <button type="button" onClick={() => setMentions(term)}>
              {t(locale, 'glossary.mentions')}
            </button>
            <p>{term.definition}</p>
            <p>{term.context}</p>
            {term.variants.map((v) => (
              <div key={v.text}>
                {v.text} · {t(locale, `glossary.${v.kind}`)}
              </div>
            ))}
            <button
              className="destructive-action"
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
                  className="destructive-action"
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
        locale={locale}
        load={scan}
        identity={JSON.stringify(['glossaryCandidates', projectId])}
        tags={[reqTag, tag]}
        empty={t(locale, 'glossary.noCandidates')}
        render={(c) => (
          <article className="card" key={c.id}>
            <button
              type="button"
              onClick={() => setDraft({ name: c.name, variant: c.abbreviation, candidate: c })}
            >
              {c.name ? `${c.name} (${c.abbreviation})` : c.abbreviation}
            </button>
            <p className="user-content">
              <span className={c.beforeTruncated ? styles.fadeStart : undefined}>{c.before}</span>
              <mark>{c.abbreviation}</mark>
              <span className={c.afterTruncated ? styles.fadeEnd : undefined}>{c.after}</span>
            </p>
            <Link href={requirementURL(projectId, c.sourceId, c)}>
              {t(locale, 'glossary.openMention')}
            </Link>
          </article>
        )}
      />
    </section>
  );
}
