'use client';
import { useSyncExternalStore } from 'react';

const narrowQuery = '(max-width: 60rem)';

function subscribeToNarrow(change: () => void) {
  const media = window.matchMedia(narrowQuery);
  media.addEventListener('change', change);
  return () => media.removeEventListener('change', change);
}

const isNarrow = () => window.matchMedia(narrowQuery).matches;
const isWideOnServer = () => false;

export function useNarrowWorkspace() {
  return useSyncExternalStore(subscribeToNarrow, isNarrow, isWideOnServer);
}
