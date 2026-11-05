type Entry = { data: unknown; tags: string[] };
type Listener = { tags: string[]; callback: (reset: boolean) => void };
export class TaggedCache {
  private entries = new Map<string, Entry>();
  private reads = new Map<string, Promise<unknown>>();
  private listeners = new Set<Listener>();
  private epoch = 0;
  private invalidations: Array<{ serial: number; tags: string[] }> = [];
  private serial = 0;
  private droppedThrough = 0;
  private seen = new Set<string>();
  subscribe(tags: string[], callback: (reset: boolean) => void) {
    const item = { tags, callback };
    this.listeners.add(item);
    return () => {
      this.listeners.delete(item);
    };
  }
  async read<T>(key: string, load: () => Promise<{ data: T; tags: string[] }>): Promise<T> {
    const cached = this.entries.get(key);
    if (cached) return cached.data as T;
    const existing = this.reads.get(key);
    if (existing) return existing as Promise<T>;
    const epoch = this.epoch;
    const task = (async () => {
      for (;;) {
        const started = this.serial;
        const result = await load();
        if (epoch !== this.epoch) return result.data;
        const stale =
          this.droppedThrough > started ||
          this.invalidations.some(
            (item) => item.serial > started && item.tags.some((tag) => result.tags.includes(tag)),
          );
        if (stale) continue;
        this.entries.set(key, result);
        return result.data;
      }
    })();
    this.reads.set(key, task);
    try {
      return await task;
    } finally {
      if (this.reads.get(key) === task) this.reads.delete(key);
    }
  }
  invalidate(tags: string[], id?: string) {
    if (id && this.seen.has(id)) return;
    if (id) {
      this.seen.add(id);
      if (this.seen.size > 1024) {
        const oldest = this.seen.values().next().value;
        if (oldest !== undefined) this.seen.delete(oldest);
      }
    }
    this.evict(tags);
    const intersects = (values: string[]) => values.some((tag) => tags.includes(tag));
    for (const listener of this.listeners) if (intersects(listener.tags)) listener.callback(false);
  }
  evict(tags: string[]) {
    this.recordInvalidation(tags);
    const intersects = (values: string[]) => values.some((tag) => tags.includes(tag));
    for (const [key, value] of this.entries) if (intersects(value.tags)) this.entries.delete(key);
  }
  reset(clear = true) {
    this.epoch++;
    this.entries.clear();
    this.reads.clear();
    this.invalidations = [];
    this.seen.clear();
    for (const listener of this.listeners) listener.callback(clear);
  }
  private recordInvalidation(tags: string[]) {
    this.invalidations.push({ serial: ++this.serial, tags: [...tags] });
    if (this.invalidations.length > 256) {
      const removed = this.invalidations.shift();
      if (removed) this.droppedThrough = removed.serial;
    }
  }
}
