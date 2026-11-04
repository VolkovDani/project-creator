'use client';
import { type Locale, t } from '../../lib/i18n';
import type { ListMode } from '../../lib/listMode';

export function ListModeSelect({
  locale,
  mode,
  onChange,
}: {
  locale: Locale;
  mode: ListMode;
  onChange: (value: ListMode) => void;
}) {
  return (
    <label>
      {t(locale, 'list.mode')}{' '}
      <select
        value={mode}
        onChange={(event) => onChange(event.target.value === 'pages' ? 'pages' : 'infinite')}
      >
        <option value="infinite">{t(locale, 'list.infinite')}</option>
        <option value="pages">{t(locale, 'list.pages')}</option>
      </select>
    </label>
  );
}

export function LoadMore({
  locale,
  hasNextPage,
  loading,
  error,
  loadMore,
}: {
  locale: Locale;
  hasNextPage: boolean;
  loading: boolean;
  error: boolean;
  loadMore: () => void;
}) {
  return hasNextPage && !error ? (
    <button type="button" disabled={loading} onClick={loadMore}>
      {t(locale, loading ? 'app.loading' : 'list.more')}
    </button>
  ) : null;
}
