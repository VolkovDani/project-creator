'use client';

import { type FormEvent, useEffect, useRef, useState } from 'react';
import { clientCache, query } from '../../../../lib/client';
import {
  BuildProcessGraphDocument,
  type BuildProcessGraphMutation,
} from '../../../../lib/generated';
import { fieldErrorMessage, type Locale, t } from '../../../../lib/i18n';
import { useInvalidation } from '../../../../lib/live';
import { useProject } from '../ProjectShell';
import { ProcessGraph } from './ProcessGraph';
import { ProcessGraphGlossary } from './ProcessGraphGlossary';
import styles from './process-graphs.module.css';

type Payload = BuildProcessGraphMutation['buildProcessGraph'];
type Result = { status: 'pending' | 'stale' | 'unavailable' } | Payload;
type Term = { id: string; name: string };

const statusKeys: Record<string, string> = {
  pending: 'processGraph.pending',
  insufficient: 'processGraph.insufficient',
  no_sources: 'processGraph.noSources',
  unavailable: 'processGraph.unavailable',
  stale: 'processGraph.stale',
  invalid: 'processGraph.invalidAnchor',
  not_found: 'processGraph.notFound',
};

export default function ProcessGraphsPage() {
  const { project, locale } = useProject();
  return <ProcessGraphBuilder key={project.id} projectId={project.id} locale={locale} />;
}

function ProcessGraphBuilder({ projectId, locale }: { projectId: string; locale: Locale }) {
  const [mode, setMode] = useState<'keyword' | 'term'>('keyword');
  const [keyword, setKeyword] = useState('');
  const [term, setTerm] = useState<Term>();
  const [result, setResult] = useState<Result>();
  const active = useRef<AbortController | null>(null);
  const glossaryTag = `project:${projectId}:glossary`;
  const tags = [`project:${projectId}`, `project:${projectId}:requirements`, glossaryTag];
  const glossarySnapshot = useRef({
    generation: clientCache.generation,
    revision: clientCache.revision,
  });
  const anchor = mode === 'keyword' ? keyword.trim() : term?.id;
  const invalid = result?.status === 'invalid';

  useEffect(() => () => active.current?.abort(), []);
  useInvalidation(
    tags,
    () => {
      active.current?.abort();
      active.current = null;
      const next = { generation: clientCache.generation, revision: clientCache.revision };
      if (
        next.generation !== glossarySnapshot.current.generation ||
        clientCache.invalidatedSince(glossarySnapshot.current.revision, [glossaryTag])
      ) {
        setTerm(undefined);
      }
      glossarySnapshot.current = next;
      setResult((previous) => previous && { status: 'stale' });
    },
    () => {
      setKeyword('');
      setTerm(undefined);
      setMode('keyword');
      setResult(undefined);
    },
  );

  function clearResult() {
    active.current?.abort();
    active.current = null;
    setResult(undefined);
  }

  async function build(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!anchor || active.current) return;
    const controller = new AbortController();
    active.current = controller;
    const { generation, revision } = clientCache;
    const isCurrent = () => active.current === controller && !controller.signal.aborted;
    const changed = () =>
      generation !== clientCache.generation || clientCache.invalidatedSince(revision, tags);
    setResult({ status: 'pending' });
    try {
      const response = await query(
        BuildProcessGraphDocument,
        {
          input: { projectId, ...(mode === 'keyword' ? { keyword: anchor } : { termId: anchor }) },
        },
        { tags: [], notify: false, signal: controller.signal },
      );
      if (isCurrent()) setResult(changed() ? { status: 'stale' } : response.buildProcessGraph);
    } catch {
      if (isCurrent()) setResult({ status: changed() ? 'stale' : 'unavailable' });
    } finally {
      if (active.current === controller) active.current = null;
    }
  }

  const draft = result?.status === 'suggested' && 'draft' in result && result.draft;
  const fieldError = invalid && 'fieldErrors' in result ? result.fieldErrors[0] : undefined;
  const status = result && (statusKeys[result.status] ?? 'processGraph.unavailable');

  return (
    <>
      <section>
        <h2>{t(locale, 'nav.process-graphs')}</h2>
        <form onSubmit={build}>
          <fieldset className={styles.modes}>
            <legend>{t(locale, 'processGraph.anchor')}</legend>
            <label>
              <input
                type="radio"
                name="process-graph-anchor"
                checked={mode === 'keyword'}
                onChange={() => {
                  clearResult();
                  setMode('keyword');
                }}
              />
              {t(locale, 'processGraph.keywordMode')}
            </label>
            <label>
              <input
                type="radio"
                name="process-graph-anchor"
                checked={mode === 'term'}
                onChange={() => {
                  clearResult();
                  setMode('term');
                }}
              />
              {t(locale, 'processGraph.termMode')}
            </label>
          </fieldset>
          {mode === 'keyword' ? (
            <label>
              {t(locale, 'processGraph.keyword')}
              <input
                value={keyword}
                aria-invalid={invalid || undefined}
                aria-describedby={invalid ? 'process-graph-status' : undefined}
                onChange={(event) => {
                  clearResult();
                  setKeyword(event.target.value);
                }}
              />
            </label>
          ) : (
            <ProcessGraphGlossary
              projectId={projectId}
              locale={locale}
              tag={glossaryTag}
              selected={term}
              select={(next) => {
                clearResult();
                setTerm(next);
              }}
            />
          )}
          <button type="submit" disabled={!anchor || result?.status === 'pending'}>
            {t(locale, 'processGraph.build')}
          </button>
          <output id="process-graph-status" className={styles.status}>
            {draft
              ? t(locale, 'processGraph.ready')
              : fieldError
                ? fieldErrorMessage(locale, fieldError)
                : status && t(locale, status)}
          </output>
        </form>
      </section>
      {draft && <ProcessGraph draft={draft} projectId={projectId} locale={locale} />}
    </>
  );
}
