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
    const to = step.match(/cache-to:[^\n]*type=gha,version=2,scope=([^\s,]+),mode=max,ignore-error=true,timeout=60s/);
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

test('large Intel layers have disk budget and are released before application builds', () => {
  assert.match(containers, /runs-on: ubuntu-(?:latest|24\.04)\n/);
  const release = steps.find((step) => step.includes('Release verified Intel image'));
  const prepare = steps.find((step) => step.includes('Free space for large Docker layers'));
  assert.ok(prepare && release);
  assert.match(prepare, /sudo rm -rf \/usr\/share\/dotnet \/usr\/local\/lib\/android/);
  assert.match(release, /docker image rm project-creator-ollama-intel:0\.40\.11/);
  assert.match(release, /docker buildx prune --all --force/);
  assert.ok(steps.indexOf(prepare) < steps.indexOf(builds[0]));
  assert.ok(steps.indexOf(release) > steps.findIndex((step) => step.includes('name: Verify Intel Vulkan runtime')));
  assert.ok(steps.indexOf(release) < steps.indexOf(builds[1]));
});

test('private-repository CI avoids duplicate runs and superseded work', () => {
  const header = workflow.split('\njobs:')[0];
  assert.match(header, /push:\n    branches:\n      - main\n/);
  assert.match(header, /pull_request:/);
  assert.match(header, /concurrency:[\s\S]*github\.event\.pull_request\.number \|\| github\.ref/);
  assert.match(header, /cancel-in-progress: true/);
  const checks = workflow.split('\n  checks:\n')[1].split('\n  containers:\n')[0];
  assert.match(checks, /timeout-minutes: 20/);
});

test('only main writes large Docker caches and build-record artifacts are disabled', () => {
  assert.match(containers, /DOCKER_BUILD_RECORD_UPLOAD: 'false'/);
  for (const step of builds) {
    const to = step.split('\n').find((line) => line.includes('cache-to:'));
    assert.match(to, /github\.event_name == 'push' && github\.ref == 'refs\/heads\/main'/);
    assert.match(to, /\|\| ''/);
  }
  assert.match(workflow, /name: desktop-screenshots[\s\S]*retention-days: 3/);
});

test('browser setup uses supported LTS and downloads only the needed headless shell', () => {
  const checks = workflow.split('\n  checks:\n')[1].split('\n  containers:\n')[0];
  assert.match(checks, /runs-on: ubuntu-24\.04\n/);
  assert.match(checks, /playwright install --with-deps --only-shell chromium/);
  assert.doesNotMatch(checks, /mcr\.microsoft\.com\/playwright/);
});
