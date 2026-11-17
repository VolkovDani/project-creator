import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { existsSync } from 'node:fs';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

const suite = new URL('../../scripts/git-hooks.test.mjs', import.meta.url);
test('repository Git hooks work with the pinned Biome', {
  // The production Docker build copies frontend sources without Git tooling.
  skip: existsSync(suite) ? false : 'Git tooling tests require the source checkout',
}, () => {
  assert.ok(
    existsSync(new URL('../node_modules/@biomejs/biome/bin/biome', import.meta.url)),
    'Install the pinned Biome with bun install --frozen-lockfile before running hook integration tests',
  );
  // A nested Node test runner must not inherit its parent's worker context.
  const env = { ...process.env };
  delete env.NODE_TEST_CONTEXT;
  const result = spawnSync(process.execPath, ['--test', fileURLToPath(suite)], {
    cwd: fileURLToPath(new URL('../../', import.meta.url)),
    env,
    encoding: 'utf8',
  });
  assert.equal(result.status, 0, result.error?.message ?? result.stdout + result.stderr);
  assert.match(result.stdout, /pre-commit formats and stages supported files/);
});
