import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import test from 'node:test';

const workflow = readFileSync(new URL('../.github/workflows/ci.yml', import.meta.url), 'utf8');
const containers = workflow.split('\n  containers:\n')[1];
const steps = containers.split(/\n(?=      - )/);
const builds = steps.filter((step) => /uses: docker\/build-push-action@/.test(step));

test('Docker CI caches three independent image builds with pinned official actions', () => {
  assert.match(containers, /uses: docker\/setup-buildx-action@[a-f0-9]{40}/);
  assert.equal(builds.length, 3);
  const scopes = new Set();
  for (const step of builds) {
    assert.match(step, /uses: docker\/build-push-action@[a-f0-9]{40}/);
    assert.match(step, /context: \./);
    assert.match(step, /load: true/);
    assert.match(step, /push: false/);
    const from = step.match(/cache-from: type=gha,version=2,scope=([^\s,]+)/);
    const to = step.match(/cache-to: type=gha,version=2,scope=([^\s,]+),mode=max,ignore-error=true,timeout=60s/);
    assert.ok(from && to, 'Cache v2 must be imported and exported with bounded best-effort writes');
    assert.equal(from[1], to[1]);
    scopes.add(from[1]);
  }
  assert.equal(scopes.size, 3, 'Different images must not overwrite each other\'s cache');
});

test('Compose consumes the already loaded cached build outputs without rebuilding', () => {
  const start = steps.find((step) => step.includes('Build and start clean disposable stack'));
  assert.ok(start);
  assert.match(start, /up -d --no-build --wait --wait-timeout 180/);
  assert.doesNotMatch(start, /--build\b/);
  const project = containers.match(/COMPOSE_PROJECT_NAME: (\S+)/)[1];
  const result = spawnSync('docker', ['compose', '--env-file', '.env.smoke.example', '-f', 'compose.yml', '-f', 'compose.smoke.yml', '-p', project, 'config', '--images'], {
    cwd: new URL('../', import.meta.url), encoding: 'utf8',
  });
  assert.equal(result.status, 0, result.error?.message ?? result.stderr);
  const images = result.stdout.trim().split('\n');
  for (const name of ['backend', 'frontend']) {
    const step = builds.find((step) => step.includes(`ci-${name}-`));
    assert.ok(step, `Missing cached ${name} build`);
    const tag = step.match(/tags: (\S+)/)[1];
    assert.equal(tag, `${project}-${name}:latest`);
    assert.ok(images.includes(tag.replace(/:latest$/, '')) || images.includes(tag));
  }
  const intel = builds.find((step) => step.includes('file: Dockerfile.ollama-intel'));
  assert.ok(intel);
  assert.match(intel, /tags: project-creator-ollama-intel:0\.40\.11/);
});

test('cache hits still run smoke verification and disposable-stack cleanup', () => {
  assert.match(containers, /run: node scripts\/compose-smoke\.mjs/);
  assert.match(containers, /name: Remove disposable containers and data\n        if: always\(\)/);
  assert.match(containers, /down --volumes --remove-orphans/);
  assert.match(containers, /docker run --rm --entrypoint sh project-creator-ollama-intel:0\.40\.11/);
  for (const step of builds) assert.doesNotMatch(step, /\n        if:/);
});
