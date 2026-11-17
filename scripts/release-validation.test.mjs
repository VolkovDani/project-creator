import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { validateAcceptanceIssue, validateReleaseInput, validateSuccessfulMainCi } from './release-validation.mjs';

const sha = 'a'.repeat(40);

test('accepts v0.1.0 as a GitHub prerelease and full source SHA on main', () => {
  assert.deepEqual(validateReleaseInput({ tag: 'v0.1.0', sourceSha: sha, ref: 'refs/heads/main' }), {
    tag: 'v0.1.0', sourceSha: sha,
  });
});

test('accepts future semantic versions with prerelease identifiers', () => {
  assert.equal(validateReleaseInput({ tag: 'v1.2.3-rc.2', sourceSha: sha, ref: 'refs/heads/main' }).tag, 'v1.2.3-rc.2');
});

test('rejects malformed tags, abbreviated SHAs, and non-main dispatches', () => {
  for (const input of [
    { tag: 'latest', sourceSha: sha, ref: 'refs/heads/main' },
    { tag: 'v0.1.0-rc.1', sourceSha: sha.slice(0, 7), ref: 'refs/heads/main' },
    { tag: 'v0.1.0-rc.1', sourceSha: sha, ref: 'refs/heads/develop' },
  ]) assert.throws(() => validateReleaseInput(input));
});

test('release workflow is manual-only, main-only, acceptance-gated, and creates a draft', () => {
  const workflow = readFileSync(new URL('../.github/workflows/release.yml', import.meta.url), 'utf8');
  const ci = readFileSync(new URL('../.github/workflows/ci.yml', import.meta.url), 'utf8');
  assert.match(workflow, /workflow_dispatch:/);
  assert.doesNotMatch(workflow, /^\s+(push|pull_request|create):/m);
  assert.match(workflow, /if: github\.ref == 'refs\/heads\/main'/);
  assert.match(workflow, /hardware-accepted/);
  assert.match(workflow, /git merge-base --is-ancestor/);
  assert.match(workflow, /ref: \$\{\{ inputs\.source_sha \}\}/);
  assert.match(workflow, /actions: read/);
  assert.match(workflow, /--workflow ci\.yml --branch main --commit/);
  assert.match(workflow, /validateSuccessfulMainCi/);
  assert.match(workflow, /gh release create .*--draft --prerelease/);
  assert.ok(workflow.includes('notes_en="docs/release-notes-${RELEASE_TAG}.md"'));
  assert.ok(workflow.includes('notes_ru="docs/release-notes-${RELEASE_TAG}.ru.md"'));
  assert.match(workflow, /test -s "\$notes_en"[\s\S]*?test -s "\$notes_ru"/);
  assert.match(ci, /workflow_dispatch:/);
  assert.match(ci, /if: github\.event_name != 'workflow_dispatch' \|\| github\.ref == 'refs\/heads\/main'/);
});

test('hardware issue form uses GitHub issue form schema and machine-readable exact candidate fields', () => {
  const form = readFileSync(new URL('../.github/ISSUE_TEMPLATE/hardware-release-acceptance.yml', import.meta.url), 'utf8');
  assert.match(form, /^description:/m);
  assert.doesNotMatch(form, /^about:/m);
  assert.match(form, /- type: textarea\n    id: candidate\n    attributes:\n      label:/);
  assert.match(form, /release-tag: v0\.1\.0\n\s*source-sha: <40-character SHA>/);
  assert.match(form, /id: batch[\s\S]*?required: false/);
});

test('acceptance requires the exact tag and source SHA in maintainer-approved evidence', () => {
  const accepted = { state: 'closed', labels: [{ name: 'hardware-accepted' }],
    body: `release-tag: v0.1.0\nsource-sha: ${sha}\nReviewed.` };
  assert.doesNotThrow(() => validateAcceptanceIssue(accepted, 'v0.1.0', sha));
  assert.throws(() => validateAcceptanceIssue(accepted, 'v0.1.0', 'b'.repeat(40)), /exact release-tag/);
  assert.throws(() => validateAcceptanceIssue({ ...accepted, body: `release-tag: v0.1.01\nsource-sha: ${sha}` }, 'v0.1.0', sha), /exact release-tag/);
  assert.throws(() => validateAcceptanceIssue({ ...accepted, state: 'open' }, 'v0.1.0', sha), /closed/);
});

test('release requires successful CI on main at the exact SHA', () => {
  const passing = [{ headSha: sha, status: 'completed', conclusion: 'success', event: 'push', headBranch: 'main' }];
  assert.doesNotThrow(() => validateSuccessfulMainCi(passing, sha));
  assert.doesNotThrow(() => validateSuccessfulMainCi([{ ...passing[0], event: 'workflow_dispatch' }], sha));
  for (const run of [
    { ...passing[0], headSha: 'b'.repeat(40) },
    { ...passing[0], status: 'in_progress' },
    { ...passing[0], conclusion: 'failure' },
    { ...passing[0], event: 'pull_request' },
    { ...passing[0], headBranch: 'develop' },
  ]) assert.throws(() => validateSuccessfulMainCi([run], sha), /main CI run/);
});
