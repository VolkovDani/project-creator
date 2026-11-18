import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import { test } from 'node:test';

const root = new URL('../', import.meta.url);
const read = (path) => readFileSync(new URL(path, root), 'utf8');
const packageJson = JSON.parse(read('frontend/package.json'));

test('frontend pins Bun and commits a Bun lockfile instead of npm lockfiles', () => {
  assert.equal(packageJson.packageManager, 'bun@1.4.4');
  assert.deepEqual(packageJson.trustedDependencies, []);
  assert.equal(existsSync(new URL('frontend/bun.lock', root)), true);
  assert.equal(existsSync(new URL('frontend/package-lock.json', root)), false);
});

test('frontend build keeps deterministic unit tests on Node before the production build', () => {
  assert.equal(packageJson.scripts.test, 'node --experimental-strip-types --test tests/*.test.ts');
  assert.equal(packageJson.scripts.prebuild, 'node --experimental-strip-types --test tests/*.test.ts');
  assert.equal(packageJson.scripts.build, 'bun --bun next build');
});

test('Docker and GitHub workflows install from the frozen Bun lockfile', () => {
  const dockerfile = read('frontend/Dockerfile');
  assert.match(dockerfile, /bun install --frozen-lockfile/);
  assert.doesNotMatch(dockerfile, /npm ci/);
  assert.match(dockerfile, /bun run build/);
  assert.match(dockerfile, /FROM node:26\.13\.1-bookworm-slim AS build/);
  assert.match(dockerfile, /FROM node:26\.13\.1-bookworm-slim AS runtime/);
  assert.match(read('compose.yml'), /image: node:26\.13\.1-bookworm-slim/);
  assert.match(read('compose.smoke.yml'), /image: node:26\.13\.1-bookworm-slim/);
  for (const workflow of ['.github/workflows/ci.yml', '.github/workflows/release.yml']) {
    assert.match(read(workflow), /node-version: '26\.13\.1'/);
  }
  for (const workflow of ['.github/workflows/ci.yml', '.github/workflows/release.yml']) {
    const content = read(workflow);
    assert.match(content, /oven-sh\/setup-bun@0c5077e51419868618aeaa5fe8019c62421857d6 # v2\.2\.0/);
    assert.match(content, /bun install --frozen-lockfile/);
    assert.match(content, /bun audit/);
    assert.match(content, /actions\/cache@55cc8345863c7cc4c66a329aec7e433d2d1c52a9 # v6\.1\.0/);
    assert.match(content, /path: ~\/\.bun\/install\/cache/);
    assert.match(content, /hashFiles\('frontend\/bun\.lock'\)/);
    assert.doesNotMatch(content, /npm ci/);
  }

  const auditWorkflow = read('.github/workflows/bun-audit.yml');
  assert.match(auditWorkflow, /schedule:/);
  assert.match(auditWorkflow, /workflow_dispatch:/);
  assert.match(auditWorkflow, /bun audit/);
  assert.match(auditWorkflow, /oven-sh\/setup-bun@0c5077e51419868618aeaa5fe8019c62421857d6 # v2\.2\.0/);

  const dependabot = read('.github/dependabot.yml');
  assert.match(dependabot, /package-ecosystem: bun/);
  assert.doesNotMatch(dependabot, /package-ecosystem: npm/);

  for (const config of ['frontend/playwright.config.ts', 'frontend/screenshots.config.ts', 'frontend/ux.playwright.config.ts']) {
    const content = read(config);
    assert.match(content, /bun run start/);
    assert.doesNotMatch(content, /npm run start/);
  }
});
