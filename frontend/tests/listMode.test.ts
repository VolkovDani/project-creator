import assert from 'node:assert/strict';
import { test } from 'node:test';
import { listLocation, readListLocation } from '../lib/listMode.ts';

test('infinite is default; pages have bounded nonnegative URL state', () => {
  assert.deepEqual(readListLocation(''), { mode: 'infinite', page: 0 });
  assert.deepEqual(readListLocation('?listMode=pages&page=3'), { mode: 'pages', page: 2 });
  for (const page of ['-1', 'NaN', '1.5', 'Infinity', '999999999999'])
    assert.equal(readListLocation(`?listMode=pages&page=${page}`).page, 0);
});

test('changing mode/page preserves source query and hash for browser history', () => {
  assert.equal(
    listLocation('/projects/a/requirements?requirement=r#requirement-r', 'pages', 2),
    '/projects/a/requirements?requirement=r&listMode=pages&page=3#requirement-r',
  );
  assert.equal(listLocation('/?listMode=pages&page=4', 'infinite', 0), '/');
});
