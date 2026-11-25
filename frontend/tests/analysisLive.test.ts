import assert from 'node:assert/strict';
import test from 'node:test';
import { AnalysisChannel, acceptSnapshot } from '../lib/analysisLive.ts';
import { parseAnalysisSnapshot } from '../lib/analysisSnapshot.ts';

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

test('legacy SSE snapshots without Laya fields remain valid while malformed Laya counts are rejected', () => {
  const legacy = {
    ...snapshot(),
    possiblePairs: null,
    notSelectedPairs: null,
    outcome: '',
    errorCode: '',
    reasonCode: '',
    completedAt: '',
    mode: 'related',
    activePairId: '',
    duplicatePairs: 0,
    reusedPairs: 0,
    newPairs: 0,
    completedPairs: 0,
    pendingPairs: 0,
    runningPairs: 0,
    failedPairs: 0,
    collisionPairs: 0,
    compatiblePairs: 0,
    insufficientPairs: 0,
    stale: false,
    similarityThreshold: 0.65,
    missingRequirementIds: [],
  };
  assert.equal(parseAnalysisSnapshot(legacy).deletionScheduledFor, null);
  assert.throws(() => parseAnalysisSnapshot({ ...legacy, deletionScheduledFor: {} }));
  assert.equal(parseAnalysisSnapshot(legacy).layaSkippedPairs, 0);
  assert.throws(() => parseAnalysisSnapshot({ ...legacy, layaRunningPairs: -1 }));
});

test('legacy SSE snapshots default skipped Laya count to zero and validate it', () => {
  const legacy = {
    ...snapshot(),
    possiblePairs: null,
    notSelectedPairs: null,
    outcome: '',
    errorCode: '',
    reasonCode: '',
    completedAt: '',
    mode: 'related',
    activePairId: '',
    duplicatePairs: 0,
    reusedPairs: 0,
    newPairs: 0,
    completedPairs: 0,
    pendingPairs: 0,
    runningPairs: 0,
    failedPairs: 0,
    collisionPairs: 0,
    compatiblePairs: 0,
    insufficientPairs: 0,
    stale: false,
    similarityThreshold: 0.65,
    missingRequirementIds: [],
  };
  assert.equal(parseAnalysisSnapshot(legacy).layaSkippedPairs, 0);
  assert.throws(() => parseAnalysisSnapshot({ ...legacy, layaSkippedPairs: -1 }));
});

test('removing the latest run allows an older surviving snapshot for that project', () => {
  const channel = new AnalysisChannel();
  const latest = snapshot('deleted', '5', '2026-11-07');
  const survivor = snapshot('survivor', '1', '2026-11-06');
  const other = { ...latest, projectId: 'q' };
  const received: unknown[] = [];
  channel.subscribe('p', (value) => received.push(value));
  channel.subscribe('q', (value) => received.push(value));
  channel.push(latest);
  channel.push(other);
  channel.forgetProject('p');
  channel.push(survivor);
  channel.flush();
  assert.deepEqual(received, [other, survivor]);
});

test('glossary revisions can restore freshness and reject an older glossary snapshot', () => {
  const stale = { ...snapshot('run', '2'), stale: true, glossaryRevision: '1' };
  const restored = { ...stale, stale: false, glossaryRevision: '2' };
  assert.equal(acceptSnapshot(stale, restored), restored);
  assert.equal(acceptSnapshot(restored, stale), restored);
  assert.equal(acceptSnapshot(restored, { ...stale, revision: '3' }), restored);
});
