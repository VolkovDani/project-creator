import assert from 'node:assert/strict';
import test from 'node:test';
import { AnalysisChannel, acceptSnapshot } from '../lib/analysisLive.ts';

const snapshot = (id = 'run', revision = '1', createdAt = '2026-11-05T00:00:00Z') => ({
  id,
  projectId: 'p',
  revision,
  createdAt,
  status: 'running',
  stage: 'comparing',
  processedPairs: 0,
  totalPairs: 20,
});
test('snapshots never move backwards within a run or to an older run', () => {
  const current = snapshot('run', '9007199254740993');
  assert.equal(acceptSnapshot(current, snapshot('run', '9007199254740992')), current);
  assert.equal(acceptSnapshot(current, snapshot('run', '9007199254740993')), current);
  assert.equal(acceptSnapshot(current, snapshot('old', '9999999999999999', '2026-11-04')), current);
  const newer = snapshot('new', '1', '2026-11-06');
  assert.equal(acceptSnapshot(current, newer), newer);
  assert.equal(acceptSnapshot(current, { ...newer, projectId: 'other' }), current);
});
test('channel rejects duplicates and coalesces each project to latest confirmed state', () => {
  const channel = new AnalysisChannel();
  const received: unknown[] = [];
  channel.subscribe('p', (value) => received.push(value));
  channel.push(snapshot('run', '2'));
  channel.push(snapshot('run', '1'));
  channel.push(snapshot('run', '3'));
  channel.flush();
  assert.deepEqual(received, [snapshot('run', '3')]);
  channel.push(snapshot('run', '2'));
  channel.flush();
  assert.equal(received.length, 1);
  channel.clear();
  channel.push(snapshot('run', '1'));
  channel.flush();
  assert.equal(received.length, 2);
});
test('a subscriber mounted after push receives the latest pending snapshot', () => {
  const channel = new AnalysisChannel();
  channel.push(snapshot('run', '5'));
  const received: unknown[] = [];
  const stop = channel.subscribe('p', (value) => received.push(value));
  channel.flush();
  assert.deepEqual(received, [snapshot('run', '5')]);
  stop();
  channel.push(snapshot('run', '6'));
  channel.flush();
  assert.equal(received.length, 1);
});

test('source invalidation can make an unchanged run revision stale without an old event undoing it', () => {
  const current = { ...snapshot(), stale: false };
  const stale = { ...current, stale: true };
  assert.equal(acceptSnapshot(current, stale), stale);
  assert.equal(acceptSnapshot(stale, current), stale);
});
test('run creation timestamps compare subsecond precision across whole-second boundaries', () => {
  const current = snapshot('first', '1', '2026-11-05T00:00:00Z');
  const next = snapshot('second', '1', '2026-11-05T00:00:00.001Z');
  assert.equal(acceptSnapshot(current, next), next);
});

test('newer pair progress cannot undo known source staleness from another request', () => {
  const stale = { ...snapshot('run', '5'), stale: true };
  const buffered = { ...snapshot('run', '6'), stale: false, processedPairs: 1 };
  assert.deepEqual(acceptSnapshot(stale, buffered), { ...buffered, stale: true });
  const channel = new AnalysisChannel<typeof buffered>();
  const received: (typeof buffered)[] = [];
  channel.subscribe('p', (value) => received.push(value));
  channel.push(stale);
  channel.flush();
  channel.push(buffered);
  channel.flush();
  assert.equal(received.at(-1)?.stale, true);
  assert.equal(received.at(-1)?.revision, '6');
});
