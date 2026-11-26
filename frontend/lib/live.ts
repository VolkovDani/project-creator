import { useEffect, useRef } from 'react';
import { AnalysisChannel } from './analysisLive';
import { type AnalysisSnapshot, parseAnalysisSnapshot } from './analysisSnapshot';
import {
  clearClientSession,
  clientCache,
  clientSessionVersion,
  rejectClientSession,
} from './client';
import type { ProcessGraphRunFieldsFragment } from './generated';
import { nextCursor, retryDelay, SSEParser } from './sse';

export type ProcessGraphProgress = {
  status: string;
  run: NonNullable<ProcessGraphRunFieldsFragment['run']>;
};

type ProcessGraphListener = {
  update: (progress: ProcessGraphProgress) => void;
  reconnect: () => void;
};
const processGraphListeners = new Map<string, Set<ProcessGraphListener>>();
const processGraphStatuses = new Set([
  'planning',
  'ready',
  'suggested',
  'stale',
  'insufficient',
  'unavailable',
]);
function parseProcessGraphProgress(data: string) {
  const payload: unknown = JSON.parse(data);
  if (
    !payload ||
    typeof payload !== 'object' ||
    !('schemaVersion' in payload) ||
    payload.schemaVersion !== 1 ||
    !('projectId' in payload) ||
    typeof payload.projectId !== 'string' ||
    !payload.projectId ||
    !('status' in payload) ||
    typeof payload.status !== 'string' ||
    !processGraphStatuses.has(payload.status) ||
    !('run' in payload) ||
    !payload.run ||
    typeof payload.run !== 'object'
  )
    return;
  const { projectId, status, run } = payload;
  const isCount = (value: unknown): value is number =>
    typeof value === 'number' && Number.isSafeInteger(value) && value >= 0;
  if (
    !('id' in run) ||
    typeof run.id !== 'string' ||
    !run.id ||
    !('topic' in run) ||
    typeof run.topic !== 'string' ||
    !('revision' in run) ||
    !isCount(run.revision) ||
    !run.revision ||
    !('completed' in run) ||
    !isCount(run.completed) ||
    !('total' in run) ||
    !isCount(run.total) ||
    !('failed' in run) ||
    !isCount(run.failed) ||
    run.completed > run.total ||
    run.failed > run.completed ||
    (status === 'ready' && run.completed === run.total)
  )
    return;
  const { id, topic, revision, completed, total, failed } = run;
  return { projectId, status, run: { id, topic, revision, completed, total, failed } };
}

const analysisChannel = new AnalysisChannel<AnalysisSnapshot>();
let updateTimer: ReturnType<typeof setTimeout> | undefined;
const pendingKeys = new Set<string>();
const seenInvalidations = new Set<string>();
function scheduleUpdates() {
  if (updateTimer) return;
  updateTimer = setTimeout(() => {
    updateTimer = undefined;
    if (pendingKeys.size) {
      const keys = [...pendingKeys];
      pendingKeys.clear();
      clientCache.invalidate(keys);
    }
    analysisChannel.flush();
  }, 50);
}
function queueInvalidations(keys: unknown, eventId?: string) {
  if (!Array.isArray(keys) || keys.some((key: unknown) => typeof key !== 'string'))
    throw Error('invalid keys');
  if (eventId && seenInvalidations.has(eventId)) return;
  if (eventId) {
    seenInvalidations.add(eventId);
    if (seenInvalidations.size > 1024) {
      const oldest = seenInvalidations.values().next().value;
      if (oldest !== undefined) seenInvalidations.delete(oldest);
    }
  }
  // Discard superseded snapshots before accepting the server's new latest run.
  for (const key of keys) {
    if (key.startsWith('project:') && key.endsWith(':analysis-history'))
      analysisChannel.forgetProject(key.slice('project:'.length, -':analysis-history'.length));
    pendingKeys.add(key);
  }
  clientCache.evict(keys);
}
function clearUpdates() {
  clearTimeout(updateTimer);
  updateTimer = undefined;
  pendingKeys.clear();
  seenInvalidations.clear();
  analysisChannel.clear();
}
// A GraphQL authorization failure also clears retained push snapshots.
clientCache.subscribe([], (clear) => {
  if (clear) clearUpdates();
});
let users = 0;
let controller: AbortController | undefined;
let cursor: string | undefined;
const wait = (ms: number, signal: AbortSignal) =>
  new Promise<void>((resolve) => {
    if (signal.aborted) {
      resolve();
      return;
    }
    const done = () => {
      clearTimeout(timer);
      signal.removeEventListener('abort', done);
      resolve();
    };
    const timer = setTimeout(done, ms);
    signal.addEventListener('abort', done, { once: true });
  });
async function stream(signal: AbortSignal) {
  const session = clientSessionVersion();
  let attempt = 0;
  let connectedBefore = false;
  while (!signal.aborted && session === clientSessionVersion()) {
    try {
      const headers: Record<string, string> = { accept: 'text/event-stream' };
      if (cursor) headers['Last-Event-ID'] = cursor;
      const response = await fetch('/api/events', {
        headers,
        cache: 'no-store',
        priority: 'low',
        signal,
      });
      if (signal.aborted) break;
      if (response.status === 401 || response.status === 403) {
        cursor = undefined;
        clearUpdates();
        rejectClientSession();
        throw Error('unauthorized');
      }
      if (!response.ok || !response.body) throw Error('events unavailable');
      if (session !== clientSessionVersion()) break;
      if (connectedBefore || attempt > 0) {
        for (const listeners of processGraphListeners.values())
          for (const listener of listeners) listener.reconnect();
      }
      connectedBefore = true;
      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      const parser = new SSEParser((frame) => {
        if (session !== clientSessionVersion()) return;
        if (frame.event === 'process_graph.progress') {
          let progress: ReturnType<typeof parseProcessGraphProgress>;
          try {
            progress = parseProcessGraphProgress(frame.data);
          } catch {
            return;
          }
          if (progress) {
            attempt = 0;
            for (const listener of processGraphListeners.get(progress.projectId) ?? [])
              listener.update(progress);
          }
          return;
        }
        if (!['cache.invalidate', 'cache.reset', 'analysis.progress'].includes(frame.event)) return;
        try {
          const payload = JSON.parse(frame.data);
          if (payload.schemaVersion !== 1) throw Error('unknown event');
          if (frame.event === 'analysis.progress') {
            const snapshot = parseAnalysisSnapshot(payload.snapshot);
            if (
              snapshot.projectId !== payload.projectId ||
              snapshot.id !== payload.runId ||
              snapshot.revision !== payload.revision
            )
              throw Error('invalid analysis scope');
            if (payload.keys !== undefined) queueInvalidations(payload.keys, frame.id);
            clientCache.evict([`project:${snapshot.projectId}:analysis`]);
            analysisChannel.push(snapshot);
            scheduleUpdates();
          } else if (frame.event === 'cache.reset') {
            clearUpdates();
            clientCache.reset(false);
          } else {
            queueInvalidations(payload.keys, frame.id);
            scheduleUpdates();
          }
          cursor = nextCursor(cursor, frame.id);
          attempt = 0;
        } catch {
          cursor = undefined;
          clearUpdates();
          clientCache.reset(false);
        }
      });
      try {
        for (;;) {
          const { done, value } = await reader.read();
          if (done || session !== clientSessionVersion()) break;
          if (!signal.aborted) parser.push(decoder.decode(value, { stream: true }));
        }
      } finally {
        await reader.cancel().catch(() => {});
        reader.releaseLock();
      }
    } catch {
      if (signal.aborted) break;
    }
    if (!signal.aborted) await wait(retryDelay(attempt++), signal);
  }
}
function connect() {
  users++;
  if (!controller) {
    controller = new AbortController();
    void stream(controller.signal);
  }
  return () => {
    if (--users === 0) {
      controller?.abort();
      controller = undefined;
      clearTimeout(updateTimer);
      updateTimer = undefined;
      pendingKeys.clear();
    }
  };
}
export function useInvalidation(tags: string[], reload: () => void, clear: () => void) {
  const callbacks = useRef({ reload, clear });
  callbacks.current = { reload, clear };
  const key = JSON.stringify(tags);
  useEffect(() => {
    const stop = clientCache.subscribe(JSON.parse(key), (reset) => {
      if (reset) callbacks.current.clear();
      callbacks.current.reload();
    });
    const disconnect = connect();
    return () => {
      stop();
      disconnect();
    };
  }, [key]);
}
// Call when a future authenticated session changes; no old-session cache or cursor survives.
export function resetSession() {
  clearUpdates();
  cursor = undefined;
  clearClientSession();
  controller?.abort();
  controller = undefined;
  if (users) {
    controller = new AbortController();
    void stream(controller.signal);
  }
}

export function useAnalysisProgress(
  projectId: string,
  update: (snapshot: AnalysisSnapshot) => void,
) {
  const callback = useRef(update);
  callback.current = update;
  useEffect(
    () => analysisChannel.subscribe(projectId, (snapshot) => callback.current(snapshot)),
    [projectId],
  );
}

export function useProcessGraphProgress(
  projectId: string,
  sourceTags: string[],
  update: ProcessGraphListener['update'],
  reconnect: ProcessGraphListener['reconnect'],
) {
  const callbacks = useRef({ update, reconnect });
  callbacks.current = { update, reconnect };
  useEffect(() => {
    const listener: ProcessGraphListener = {
      update: (progress) => {
        // A queued source invalidation will refresh GraphQL against the new sources.
        if (!sourceTags.some((tag) => pendingKeys.has(tag))) callbacks.current.update(progress);
      },
      reconnect: () => callbacks.current.reconnect(),
    };
    const listeners = processGraphListeners.get(projectId) ?? new Set<ProcessGraphListener>();
    processGraphListeners.set(projectId, listeners);
    listeners.add(listener);
    const disconnect = connect();
    return () => {
      listeners.delete(listener);
      if (!listeners.size) processGraphListeners.delete(projectId);
      disconnect();
    };
  }, [projectId, sourceTags]);
}
