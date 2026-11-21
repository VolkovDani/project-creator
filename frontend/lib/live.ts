import { useEffect, useRef } from 'react';
import { AnalysisChannel } from './analysisLive';
import { type AnalysisSnapshot, parseAnalysisSnapshot } from './analysisSnapshot';
import {
  clearClientSession,
  clientCache,
  clientSessionVersion,
  rejectClientSession,
} from './client';
import { nextCursor, retryDelay, SSEParser } from './sse';

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
      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      const parser = new SSEParser((frame) => {
        if (session !== clientSessionVersion()) return;
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
            clientCache.evict([`project:${snapshot.projectId}:analysis`]);
            analysisChannel.push(snapshot);
            scheduleUpdates();
          } else if (frame.event === 'cache.reset') {
            clearUpdates();
            clientCache.reset(false);
          } else {
            if (
              !Array.isArray(payload.keys) ||
              payload.keys.some((key: unknown) => typeof key !== 'string')
            )
              throw Error('invalid keys');
            if (frame.id && seenInvalidations.has(frame.id)) return;
            if (frame.id) {
              seenInvalidations.add(frame.id);
              if (seenInvalidations.size > 1024) {
                const oldest = seenInvalidations.values().next().value;
                if (oldest !== undefined) seenInvalidations.delete(oldest);
              }
            }
            // Evict immediately so an in-flight response cannot be marked fresh.
            clientCache.evict(payload.keys);
            for (const key of payload.keys) pendingKeys.add(key);
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
