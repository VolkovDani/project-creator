import assert from 'node:assert/strict';
import { mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';
import { loadConfig } from 'graphql-config';

// Exercise graphql-config's use of the security-patched minimatch override.
test('codegen config matches schema, documents and include/exclude globs', async (t) => {
  const root = await mkdtemp(join(tmpdir(), 'pc-codegen-config-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  const filepath = join(root, '.graphqlrc.json');
  await writeFile(
    filepath,
    JSON.stringify({
      schema: 'schema/*.graphql',
      documents: 'operations/**/*.graphql',
      include: ['src/**/*.{ts,tsx}'],
      exclude: ['src/generated/**'],
    }),
  );
  const config = await loadConfig({ filepath });
  assert.ok(config);
  const project = config.getDefault();
  for (const path of [
    'schema/main.graphql',
    'operations/projects.graphql',
    'operations/nested/query.graphql',
    'src/page.tsx',
    'src/.hidden/query.ts',
  ]) {
    assert.equal(project.match(join(root, path)), true, path);
  }
  for (const path of ['src/generated/query.ts', 'src/page.css', 'outside/query.graphql']) {
    assert.equal(project.match(join(root, path)), false, path);
  }
});
