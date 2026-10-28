import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {test} from 'node:test';
import {fileURLToPath} from 'node:url';

const script = fileURLToPath(new URL('./compose-smoke.mjs', import.meta.url));
test('container smoke refuses an ordinary project before calling Docker or writing data', () => {
  const result = spawnSync(process.execPath, [script], {encoding: 'utf8', env: {...process.env, COMPOSE_PROJECT_NAME: 'production', COMPOSE_FILE: 'compose.yml:compose.smoke.yml'}});
  assert.equal(result.status, 1);
  assert.match(result.stderr, /COMPOSE_PROJECT_NAME must end in -smoke/);
});
test('real model container smoke requires an explicit opt-in', () => {
  const result = spawnSync(process.execPath, [script], {encoding: 'utf8', env: {...process.env, COMPOSE_PROJECT_NAME: 'project-creator-smoke', COMPOSE_FILE: 'compose.yml', REAL_MODEL_SMOKE: ''}});
  assert.equal(result.status, 1);
  assert.match(result.stderr, /explicitly opt in to real weights with REAL_MODEL_SMOKE=1/);
});
