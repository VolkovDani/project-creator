'use client';
import { useCallback, useSyncExternalStore } from 'react';
import { type ListMode, listLocation, readListLocation } from './listMode';

const subscribe = (changed: () => void) => {
  window.addEventListener('popstate', changed);
  window.addEventListener('list-location', changed);
  return () => {
    window.removeEventListener('popstate', changed);
    window.removeEventListener('list-location', changed);
  };
};
export function useListMode() {
  const search = useSyncExternalStore(
    subscribe,
    () => window.location.search,
    () => '',
  );
  const { mode, page } = readListLocation(search);
  const navigate = useCallback((nextMode: ListMode, nextPage: number, replace = false) => {
    window.history[replace ? 'replaceState' : 'pushState'](
      null,
      '',
      listLocation(window.location.href, nextMode, nextPage),
    );
    window.dispatchEvent(new Event('list-location'));
  }, []);
  return {
    mode,
    page,
    setMode: (value: ListMode) => navigate(value, 0),
    setPage: useCallback((value: number) => navigate('pages', value), [navigate]),
    replacePage: useCallback((value: number) => navigate('pages', value, true), [navigate]),
  };
}
