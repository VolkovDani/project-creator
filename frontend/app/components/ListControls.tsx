'use client';
import { useEffect, useRef } from 'react';
import { type Locale, t } from '../../lib/i18n';

export function LoadMore({
  locale,
  hasNextPage,
  loading,
  error,
  loadMore,
  observe = false,
}: {
  locale: Locale;
  hasNextPage: boolean;
  loading: boolean;
  error: boolean;
  loadMore: () => void;
  observe?: boolean;
}) {
  const button = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (
      !observe ||
      !hasNextPage ||
      loading ||
      error ||
      !button.current ||
      typeof IntersectionObserver === 'undefined'
    )
      return;
    const observer = new IntersectionObserver(([entry]) => {
      if (entry?.isIntersecting) loadMore();
    });
    observer.observe(button.current);
    return () => observer.disconnect();
  }, [observe, hasNextPage, loading, error, loadMore]);
  return hasNextPage && !error ? (
    <button ref={button} type="button" disabled={loading} onClick={loadMore}>
      {t(locale, loading ? 'app.loading' : 'list.more')}
    </button>
  ) : null;
}
