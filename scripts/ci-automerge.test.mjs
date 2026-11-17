import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { autoMerge } from './ci-automerge.mjs';

const repository = 'owner/repo';
const head = 'a'.repeat(40);
const base = 'b'.repeat(40);
const merge = 'c'.repeat(40);
function fixture() {
  const context = { repository, number: 81, head, merge, event: 'pull_request',
    needs: { checks: { result: 'success' }, containers: { result: 'success' } } };
  const pr = { state: 'open', draft: false, mergeable: true,
    labels: [{ name: 'automerge' }],
    head: { sha: head, repo: { full_name: repository } },
    base: { ref: 'develop', repo: { full_name: repository } } };
  const replies = {
    [`repos/${repository}/pulls/81`]: pr,
    [`repos/${repository}/git/commits/${merge}`]: { parents: [{ sha: base }, { sha: head }] },
    [`repos/${repository}/git/ref/heads/develop`]: { object: { sha: base } },
    [`repos/${repository}/pulls/81/merge`]: { merged: true, sha: 'd'.repeat(40) },
  };
  const calls = [];
  const request = async (path, body) => {
    calls.push({ path, body });
    assert.ok(path in replies, `Unexpected API request: ${path}`);
    return replies[path];
  };
  return { context, pr, replies, calls, request };
}
const writes = (f) => f.calls.filter((call) => call.body);

test('creates a merge commit for the tested head only, without deleting the branch', async () => {
  const f = fixture();
  const result = await autoMerge(f.context, f.request);
  assert.equal(result.merged, true);
  assert.deepEqual(writes(f), [{ path: `repos/${repository}/pulls/81/merge`,
    body: { sha: head, merge_method: 'merge' } }]);
});

for (const result of ['failure', 'cancelled', 'skipped', '']) {
  for (const job of ['checks', 'containers']) {
    test(`does not merge when ${job} result is ${result || 'unknown'}`, async () => {
      const f = fixture();
      f.context.needs[job].result = result;
      assert.equal((await autoMerge(f.context, f.request)).merged, false);
      assert.equal(f.calls.length, 0);
    });
  }
}

for (const [name, change] of [
  ['push run', (f) => { f.context.event = 'push'; }],
  ['removed label', (f) => { f.pr.labels = []; }],
  ['draft', (f) => { f.pr.draft = true; }],
  ['closed PR', (f) => { f.pr.state = 'closed'; }],
  ['fork', (f) => { f.pr.head.repo.full_name = 'someone/fork'; }],
  ['deleted head repository', (f) => { f.pr.head.repo = null; }],
  ['main target', (f) => { f.pr.base.ref = 'main'; }],
  ['different target', (f) => { f.pr.base.ref = 'release'; }],
  ['new head commit', (f) => { f.pr.head.sha = 'e'.repeat(40); }],
  ['conflict', (f) => { f.pr.mergeable = false; }],
  ['unknown mergeability', (f) => { f.pr.mergeable = null; }],
  ['develop advanced', (f) => { f.replies[`repos/${repository}/git/ref/heads/develop`].object.sha = 'e'.repeat(40); }],
  ['wrong tested head', (f) => { f.replies[`repos/${repository}/git/commits/${merge}`].parents[1].sha = 'e'.repeat(40); }],
]) {
  test(`does not merge: ${name}`, async () => {
    const f = fixture();
    change(f);
    assert.equal((await autoMerge(f.context, f.request)).merged, false);
    assert.equal(writes(f).length, 0);
  });
}

test('API errors fail the job instead of claiming a merge', async () => {
  const f = fixture();
  await assert.rejects(autoMerge(f.context, async () => { throw new Error('HTTP 403'); }), /403/);
});

test('unsuccessful merge response fails the job', async () => {
  const f = fixture();
  f.replies[`repos/${repository}/pulls/81/merge`] = { merged: false };
  await assert.rejects(autoMerge(f.context, f.request), /merge/i);
});

test('merge SHA race is not retried against a new untested head', async () => {
  const f = fixture();
  await assert.rejects(autoMerge(f.context, async (path, body) => {
    if (body) { f.calls.push({ path, body }); throw new Error('HTTP 409'); }
    return f.request(path);
  }), /409/);
  assert.equal(writes(f).length, 1);
  assert.equal(writes(f)[0].body.sha, head);
});

test('workflow runs merge only after both jobs and grants write only to merge job', () => {
  const workflow = readFileSync(new URL('../.github/workflows/ci.yml', import.meta.url), 'utf8');
  const job = workflow.split('\n  automerge:\n')[1];
  assert.ok(job);
  assert.match(job, /needs: \[checks, containers\]/);
  const dependencies = job.match(/needs: \[([^\]]+)\]/)[1].split(',').map((name) => name.trim());
  const otherJobs = [...workflow.split('\njobs:\n')[1].matchAll(/^  ([a-z][a-z_-]*):$/gm)]
    .map((match) => match[1]).filter((name) => name !== 'automerge');
  assert.deepEqual(dependencies.sort(), otherJobs.sort(), 'Every CI job must gate auto-merge');
  assert.match(job, /needs.checks.result == 'success'/);
  assert.match(job, /needs.containers.result == 'success'/);
  assert.match(job, /github.event_name == 'pull_request'/);
  assert.match(job, /github.event.pull_request.head.repo.full_name == github.repository/);
  assert.match(job, /github.event.pull_request.base.ref == 'develop'/);
  assert.match(job, /contents: write/);
  assert.match(job, /pull-requests: read/);
  assert.match(job, /persist-credentials: false/);
  assert.match(job, /node scripts\/ci-automerge.mjs/);
  assert.match(job, /NEEDS_JSON: \$\{\{ toJSON\(needs\) \}\}/);
  assert.doesNotMatch(workflow.split('\n  automerge:\n')[0], /: write/);
  assert.doesNotMatch(workflow, /pull_request_target|types:.*labeled/);
});
