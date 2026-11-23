'use client';
import { type Locale, t } from '../../../lib/i18n';
import IndexProgress from './IndexProgress';
import InfiniteIndexing from './InfiniteIndexing';
export default function Indexing({ projectId, locale }: { projectId: string; locale: Locale }) {
  return (
    <section aria-labelledby="indexing-heading">
      <h2 id="indexing-heading">{t(locale, 'ui.indexing')}</h2>
      <IndexProgress key={projectId} projectId={projectId} locale={locale} />
      <InfiniteIndexing key={projectId} projectId={projectId} locale={locale} />
    </section>
  );
}
