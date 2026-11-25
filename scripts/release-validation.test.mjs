import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import test from 'node:test';
import { validateAcceptanceIssue, validateReleaseInput, validateSuccessfulMainCi } from './release-validation.mjs';

const sha = 'a'.repeat(40);
const notesSha = 'b'.repeat(40);

test('accepts v0.1.0 as a GitHub prerelease and full source and notes SHAs on main', () => {
  assert.deepEqual(validateReleaseInput({ tag: 'v0.1.0', sourceSha: sha, notesSha, ref: 'refs/heads/main' }), {
    tag: 'v0.1.0', sourceSha: sha, notesSha,
  });
});

test('accepts future semantic versions with prerelease identifiers', () => {
  assert.equal(validateReleaseInput({ tag: 'v1.2.3-rc.2', sourceSha: sha, notesSha, ref: 'refs/heads/main' }).tag, 'v1.2.3-rc.2');
});

test('rejects malformed tags, abbreviated SHAs, and non-main dispatches', () => {
  for (const input of [
    { tag: 'v0.1.0', sourceSha: sha, ref: 'refs/heads/main' },
    { tag: 'v0.1.0', sourceSha: sha, notesSha: notesSha.slice(0, 7), ref: 'refs/heads/main' },
    { tag: 'latest', sourceSha: sha, notesSha, ref: 'refs/heads/main' },
    { tag: 'v0.1.0-rc.1', sourceSha: sha.slice(0, 7), notesSha, ref: 'refs/heads/main' },
    { tag: 'v0.1.0-rc.1', sourceSha: sha, notesSha, ref: 'refs/heads/develop' },
  ]) assert.throws(() => validateReleaseInput(input));
});

test('release workflow is manual-only, main-only, permits unverified hardware, and creates a draft', () => {
  const workflow = readFileSync(new URL('../.github/workflows/release.yml', import.meta.url), 'utf8');
  const ci = readFileSync(new URL('../.github/workflows/ci.yml', import.meta.url), 'utf8');
  assert.match(workflow, /workflow_dispatch:/);
  assert.doesNotMatch(workflow, /^\s+(push|pull_request|create):/m);
  assert.match(workflow, /if: github\.ref == 'refs\/heads\/main'/);
  assert.doesNotMatch(workflow, /acceptance_issue|validateAcceptanceIssue|hardware-accepted/);
  assert.match(workflow, /git merge-base --is-ancestor/);
  assert.match(workflow, /ref: \$\{\{ inputs\.source_sha \}\}/);
  assert.match(workflow, /actions: read/);
  assert.match(workflow, /--workflow ci\.yml --branch main --commit/);
  assert.match(workflow, /validateSuccessfulMainCi/);
  assert.match(workflow, /gh release create .*--draft --prerelease/);
  assert.match(workflow, /notes_sha:\n[^\n]+\n\s+required: true/);
  assert.match(workflow, /NOTES_SHA: \$\{\{ inputs\.notes_sha \}\}/);
  const prepare = workflow.indexOf('bash scripts/prepare-release-notes.sh');
  assert.ok(prepare >= 0 && prepare < workflow.indexOf('Build backend and frontend containers'));
  assert.ok(workflow.includes('--notes-file "$RUNNER_TEMP/release-notes-draft.md"'));
  assert.match(ci, /workflow_dispatch:/);
  assert.match(ci, /if: github\.event_name != 'workflow_dispatch' \|\| github\.ref == 'refs\/heads\/main'/);
});


test('prepares both Wiki notes from the requested commit and rejects missing or blank pages', (t) => {
  const remote = mkdtempSync(join(tmpdir(), 'release-wiki-test-'));
  t.after(() => rmSync(remote, { recursive: true, force: true }));
  const wiki = join(remote, 'owner', 'repo.wiki.git');
  const notes = join(wiki, 'releases', 'notes');
  const runner = join(remote, 'runner');
  const english = join(notes, 'Release-Notes-EN-v1-2-3-rc.2.md');
  const russian = join(notes, 'Release-Notes-RU-v1-2-3-rc.2.md');
  const draft = join(runner, 'release-notes-draft.md');
  mkdirSync(notes, { recursive: true });
  mkdirSync(runner);
  const git = (...args) => execFileSync('git', ['-C', wiki, ...args], { encoding: 'utf8' });
  const commit = (message) => {
    git('add', '.');
    git('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.com', '-c', 'commit.gpgsign=false',
      'commit', '-qm', message);
    return git('rev-parse', 'HEAD').trim();
  };
  git('init', '-q');
  writeFileSync(english, 'Approved English notes.');
  writeFileSync(russian, 'Утверждённые заметки.');
  const pinned = commit('reviewed notes');
  rmSync(english);
  const missing = commit('remove English notes');
  writeFileSync(english, '   \n');
  const blank = commit('blank English notes');
  const script = fileURLToPath(new URL('./prepare-release-notes.sh', import.meta.url));
  const prepare = (notesSha) => spawnSync('bash', [script], {
    encoding: 'utf8',
    env: {
      ...process.env,
      GITHUB_SERVER_URL: pathToFileURL(remote).href,
      GITHUB_REPOSITORY: 'owner/repo',
      RELEASE_TAG: 'v1.2.3-rc.2',
      SOURCE_SHA: sha,
      NOTES_SHA: notesSha,
      RUNNER_TEMP: runner,
    },
  });
  const approved = prepare(pinned);
  assert.equal(approved.status, 0, approved.stderr);
  const body = readFileSync(draft, 'utf8');
  assert.match(body, /Approved English notes\.\n\nУтверждённые заметки\.\n\n## Candidate under review/);
  assert.ok(body.includes('Source commit: ' + sha));
  assert.ok(body.includes('Wiki notes commit: ' + pinned));
  assert.ok(body.includes('/wiki/Release-Notes-EN-v1-2-3-rc.2/' + pinned));
  assert.ok(body.includes('/wiki/Release-Notes-RU-v1-2-3-rc.2/' + pinned));

  for (const rejectedSha of [missing, blank]) {
    const rejected = prepare(rejectedSha);
    assert.notEqual(rejected.status, 0);
    assert.match(rejected.stderr, /Release-Notes-EN-v1-2-3-rc\.2\.md/);
  }
  assert.equal(readFileSync(draft, 'utf8'), body);
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
