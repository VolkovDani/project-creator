import assert from 'node:assert/strict';
import { test } from 'node:test';
import { indexStatus, requirementAnchor, requirementURL } from '../lib/rag.ts';

test('source URLs select the project and requirement safely across pages', () => {
  assert.equal(
    requirementURL('project/one', 'a/#? b'),
    '/projects/project%2Fone/requirements?requirement=a%2F%23%3F%20b#requirement-a%2F%23%3F%20b',
  );
});
test('index from an old requirement version is shown as pending', () => {
  assert.equal(indexStatus({ version: 1, status: 'ready' }, 2), 'pending');
  assert.equal(indexStatus({ version: 2, status: 'ready' }, 2), 'ready');
  assert.equal(indexStatus({ version: 2, status: 'failed' }, 2), 'failed');
  assert.equal(indexStatus(undefined, 1), 'pending');
});
test('source fragment cannot introduce another URL', () => {
  assert.equal(requirementAnchor('a/#? b'), '#requirement-a%2F%23%3F%20b');
});

test('queued and running indexing are distinct from ready and stale versions', () => {
  assert.equal(indexStatus({ version: 2, status: 'queued' }, 2), 'pending');
  assert.equal(indexStatus({ version: 2, status: 'running' }, 2), 'running');
  assert.equal(indexStatus({ version: 1, status: 'running' }, 2), 'pending');
});

test('candidate links select a versioned code point range', () => {
  assert.equal(
    requirementURL('p', 'r', { sourceVersion: 2, start: 3, end: 5 }),
    '/projects/p/requirements?requirement=r&version=2&start=3&end=5#requirement-r',
  );
});
