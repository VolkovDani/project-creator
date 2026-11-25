// Revisions are decimal strings: PostgreSQL bigint exceeds JavaScript's safe integers.
export type AnalysisVersion = {
  id: string;
  projectId: string;
  revision: string;
  glossaryRevision?: string;
  createdAt: string;
  stale?: boolean;
};
export function acceptSnapshot<T extends AnalysisVersion>(
  current: T | null | undefined,
  next: T,
): T {
  if (!current) return next;
  if (current.projectId !== next.projectId) return current;
  if (current.id === next.id) {
    const revision = BigInt(next.revision) - BigInt(current.revision);
    const glossaryRevision =
      BigInt(next.glossaryRevision || '0') - BigInt(current.glossaryRevision || '0');
    if (revision < 0n || glossaryRevision < 0n) return current;
    if (glossaryRevision > 0n) return next;
    if (revision > 0n) return current.stale && !next.stale ? { ...next, stale: true } : next;
    return revision === 0n && next.stale === true && !current.stale ? next : current;
  }
  // RFC3339Nano omits a zero fraction. Pad it before lexical comparison.
  const timestamp = (value: string) =>
    value.replace(
      /(\d{2}:\d{2}:\d{2})(?:\.(\d+))?Z$/,
      (_, seconds: string, fraction: string | undefined) =>
        `${seconds}.${(fraction || '').padEnd(9, '0')}Z`,
    );
  return timestamp(next.createdAt) > timestamp(current.createdAt) ? next : current;
}

// One pending value per project, independent of the number of events/subscribers.
export class AnalysisChannel<T extends AnalysisVersion = AnalysisVersion> {
  private current = new Map<string, T>();
  private pending = new Map<string, T>();
  private listeners = new Map<string, Set<(snapshot: T) => void>>();
  subscribe(projectId: string, listener: (snapshot: T) => void) {
    const listeners = this.listeners.get(projectId) ?? new Set();
    this.listeners.set(projectId, listeners);
    listeners.add(listener);
    const value = this.current.get(projectId);
    if (value && !this.pending.has(projectId)) listener(value);
    return () => {
      listeners.delete(listener);
      if (!listeners.size) this.listeners.delete(projectId);
    };
  }
  push(snapshot: T) {
    const previous = this.current.get(snapshot.projectId);
    const accepted = acceptSnapshot(previous, snapshot);
    if (accepted === previous) return;
    this.current.set(accepted.projectId, accepted);
    this.pending.set(accepted.projectId, accepted);
  }
  flush() {
    const pending = [...this.pending];
    this.pending.clear();
    for (const [projectId, snapshot] of pending)
      for (const listener of this.listeners.get(projectId) ?? []) listener(snapshot);
  }
  forgetProject(projectId: string) {
    this.current.delete(projectId);
    this.pending.delete(projectId);
  }
  clear() {
    this.current.clear();
    this.pending.clear();
  }
}
