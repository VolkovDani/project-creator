import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { chmodSync, copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const source = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const biomeTest = existsSync(join(source, 'frontend/node_modules/@biomejs/biome/bin/biome')) ? test : test.skip;
function fixture(t, dependencies = true) {
  const root = mkdtempSync(join(tmpdir(), 'pc-git-hooks-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const run = (command, args, options = {}) => spawnSync(command, args, { cwd: root, encoding: 'utf8', ...options });
  const git = (...args) => {
    const result = run('git', args);
    assert.equal(result.status, 0, result.stderr);
    return result.stdout;
  };
  git('init', '-q');
  git('config', 'user.name', 'Hook test');
  git('config', 'user.email', 'hook@example.invalid');
  for (const path of ['.githooks/pre-commit', 'scripts/biome-pre-commit.mjs', 'scripts/install-git-hooks.mjs', 'frontend/biome.jsonc', 'frontend/.gitignore']) {
    mkdirSync(dirname(join(root, path)), { recursive: true });
    copyFileSync(join(source, path), join(root, path));
  }
  chmodSync(join(root, '.githooks/pre-commit'), 0o755);
  writeFileSync(join(root, '.gitignore'), 'frontend/node_modules/\n');
  if (dependencies) symlinkSync(join(source, 'frontend/node_modules'), join(root, 'frontend/node_modules'), 'dir');
  git('add', '.');
  git('commit', '-qm', 'fixture before enabling hook');
  const install = () => run(process.execPath, ['scripts/install-git-hooks.mjs']);
  const hook = () => run('sh', ['.githooks/pre-commit']);
  const write = (path, data) => { mkdirSync(dirname(join(root, path)), { recursive: true }); writeFileSync(join(root, path), data); };
  return { root, git, run, install, hook, write };
}

biomeTest('pre-commit formats and stages supported files, including spaces and newlines', (t) => {
  const f = fixture(t);
  for (const path of ['frontend/demo.ts', 'frontend/space name.ts', 'frontend/line\nbreak.ts', 'frontend/config.json']) {
    f.write(path, path.endsWith('.json') ? '{"value":1}\n' : 'export const demo={value:1};\n');
    f.git('add', '--', path);
  }
  assert.equal(f.install().status, 0);
  const result = f.run('git', ['commit', '-qm', 'autoformatted']);
  assert.equal(result.status, 0, result.stderr);
  for (const path of ['frontend/demo.ts', 'frontend/space name.ts', 'frontend/line\nbreak.ts', 'frontend/config.json']) {
    const content = readFileSync(join(f.root, path), 'utf8');
    assert.equal(content, path.endsWith('.json') ? '{ "value": 1 }\n' : 'export const demo = { value: 1 };\n');
    assert.equal(f.git('show', `HEAD:${path}`), content);
  }
  assert.equal(f.git('status', '--porcelain'), '');
});

biomeTest('partial staging checks the index without including or changing unstaged work', (t) => {
  const f = fixture(t);
  const path = 'frontend/partial.ts';
  const staged = 'export const demo = { value: 1 };\n';
  const working = `${staged}// Private unfinished work\nexport const unfinished=\n`;
  f.write(path, staged); f.git('add', '--', path); f.write(path, working);
  assert.equal(f.install().status, 0);
  const result = f.run('git', ['commit', '-qm', 'only staged content']);
  assert.equal(result.status, 0, result.stderr);
  assert.equal(f.git('show', `HEAD:${path}`), staged);
  assert.equal(readFileSync(join(f.root, path), 'utf8'), working);
});

biomeTest('unformatted partially staged file blocks without changing any index or working content', (t) => {
  const f = fixture(t);
  const path = 'frontend/partial.ts';
  const staged = 'export const demo={value:1};\n';
  const working = `${staged}// Unstaged work\n`;
  f.write(path, staged); f.git('add', '--', path); f.write(path, working);
  const index = f.git('write-tree');
  const result = f.hook();
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /partial|частич/i);
  assert.equal(f.git('write-tree'), index);
  assert.equal(readFileSync(join(f.root, path), 'utf8'), working);
});

biomeTest('lint failures block and leave all staged files unchanged', (t) => {
  const f = fixture(t);
  f.write('frontend/good.ts', 'export const good={value:1};\n');
  f.write('frontend/bad.ts', 'export const bad: any = 1;\n');
  f.git('add', 'frontend');
  const index = f.git('write-tree');
  const result = f.hook();
  assert.notEqual(result.status, 0);
  assert.equal(f.git('write-tree'), index);
  assert.equal(readFileSync(join(f.root, 'frontend/good.ts'), 'utf8'), 'export const good={value:1};\n');
});

biomeTest('Biome exclusions preserve generated files', (t) => {
  const f = fixture(t);
  f.write('frontend/lib/generated.ts', 'generated placeholder, deliberately not TypeScript\n');
  f.git('add', 'frontend/lib/generated.ts');
  const index = f.git('write-tree');
  assert.equal(f.hook().status, 0);
  assert.equal(f.git('write-tree'), index);
});

test('documentation-only commits need no installed Biome; frontend commits fail clearly if absent', (t) => {
  const f = fixture(t, false);
  f.write('README.md', 'Documentation\n'); f.git('add', 'README.md');
  assert.equal(f.hook().status, 0);
  f.write('frontend/demo.ts', 'export const demo = 1;\n'); f.git('add', 'frontend/demo.ts');
  const result = f.hook();
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /npm ci/);
});

test('installer is repeatable and connects tracked executable hook', (t) => {
  const f = fixture(t, false);
  assert.equal(f.install().status, 0);
  assert.equal(f.install().status, 0);
  assert.equal(f.git('config', '--local', '--get', 'core.hooksPath').trim(), '.githooks');
  assert.ok(existsSync(join(f.root, '.githooks/pre-commit')));
});

test('installer preserves existing custom hooksPath', (t) => {
  const f = fixture(t, false);
  f.git('config', '--local', 'core.hooksPath', 'existing-hooks');
  const result = f.install();
  assert.notEqual(result.status, 0);
  assert.equal(f.git('config', '--local', '--get', 'core.hooksPath').trim(), 'existing-hooks');
});

test('installer preserves an existing default pre-commit hook', (t) => {
  const f = fixture(t, false);
  const original = '#!/bin/sh\nexit 0\n';
  f.write('.git/hooks/pre-commit', original);
  const result = f.install();
  assert.notEqual(result.status, 0);
  assert.equal(readFileSync(join(f.root, '.git/hooks/pre-commit'), 'utf8'), original);
  assert.equal(f.run('git', ['config', '--get', 'core.hooksPath']).status, 1);
});

biomeTest('unstaged configuration cannot disable staged formatting rules', (t) => {
  const f = fixture(t);
  const path = 'frontend/demo.ts';
  f.write(path, 'export const demo={value:1};\n'); f.git('add', path);
  const configPath = join(f.root, 'frontend/biome.jsonc');
  const originalConfig = readFileSync(configPath, 'utf8');
  const workingConfig = originalConfig.replace('"formatter": {', '"formatter": { "enabled": false,');
  writeFileSync(configPath, workingConfig);
  const result = f.hook();
  assert.equal(result.status, 0, result.stderr);
  assert.equal(f.git('show', `:${path}`), 'export const demo = { value: 1 };\n');
  assert.equal(f.git('show', ':frontend/biome.jsonc'), originalConfig);
  assert.equal(readFileSync(configPath, 'utf8'), workingConfig);
});

test('frontend deletions and files outside Biome scope need no dependencies', (t) => {
  const f = fixture(t, false);
  f.write('frontend/deleted.ts', 'export const demo = 1;\n');
  f.git('add', 'frontend/deleted.ts'); f.git('commit', '-qm', 'before deletion');
  f.git('rm', 'frontend/deleted.ts');
  f.write('frontend/app/style.css', 'body {color:red}\n'); f.git('add', 'frontend/app/style.css');
  assert.equal(f.hook().status, 0);
});

biomeTest('hook refuses supported symlinks without following or changing their target', (t) => {
  const f = fixture(t);
  const target = join(f.root, 'outside.ts');
  const content = 'export const outside={value:1};\n';
  writeFileSync(target, content);
  symlinkSync(target, join(f.root, 'frontend/link.ts'));
  f.git('add', 'frontend/link.ts');
  const index = f.git('write-tree');
  const result = f.hook();
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /regular file|symlink/i);
  assert.equal(readFileSync(target, 'utf8'), content);
  assert.equal(f.git('write-tree'), index);
});
