'use client';
import { useCallback, useEffect, useRef, useState } from 'react';
import { query } from '../../../lib/client';
import { RequirementsDocument, type RequirementsQuery } from '../../../lib/generated';
import { Latest } from '../../../lib/latest';
import { useInvalidation } from '../../../lib/live';

export function useRequirementsPage(
  projectId: string,
  clearDraft: () => void = () => {},
  options?: {
    enabled: boolean;
    page: number;
    setPage: (page: number) => void;
  },
) {
  const [localPage, setLocalPage] = useState(0);
  const page = options?.page ?? localPage;
  const setPage = options?.setPage ?? setLocalPage;
  const enabled = options?.enabled ?? true;
  const [snapshot, setSnapshot] = useState<{
    projectId: string;
    page: number;
    rows: RequirementsQuery['requirements'];
  }>();
  const [loading, setLoading] = useState(true);
  const [failure, setFailure] = useState(false);
  const latest = useRef(new Latest());
  const clearRef = useRef(clearDraft);
  clearRef.current = clearDraft;
  const load = useCallback(async () => {
    if (!enabled) return;
    const generation = latest.current.begin();
    setLoading(true);
    try {
      const data = await query(RequirementsDocument, { projectId, limit: 20, offset: page * 20 });
      if (latest.current.isCurrent(generation)) {
        setSnapshot({ projectId, page, rows: data.requirements });
        setFailure(false);
      }
    } catch {
      if (latest.current.isCurrent(generation)) setFailure(true);
    } finally {
      if (latest.current.isCurrent(generation)) setLoading(false);
    }
  }, [projectId, page, enabled]);
  useEffect(() => {
    void load();
    const state = latest.current;
    return () => state.invalidate();
  }, [load]);
  useInvalidation(
    [`project:${projectId}`, `project:${projectId}:requirements`],
    () => void load(),
    () => {
      latest.current.invalidate();
      setSnapshot(undefined);
      setFailure(false);
      setLoading(true);
      if (enabled) setPage(0);
      clearRef.current();
    },
  );
  const data = snapshot?.projectId === projectId ? snapshot : undefined;
  return {
    rows: data?.rows ?? [],
    page: data?.page ?? page,
    requestedPage: page,
    setPage,
    loading,
    failure,
    setFailure,
    loaded: !!data,
    load,
  };
}
