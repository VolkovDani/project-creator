import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {test} from 'node:test';

// Run explicitly in the containers CI job; needs Compose, never a GPU or daemon.
test('Intel override keeps the local stack and grants only DRM GPU access', () => {
  const result = spawnSync('docker', ['compose', '--profile', 'tools', '-f', 'compose.yml', '-f', 'compose.intel.yml', 'config', '--format', 'json'], {
    encoding: 'utf8',
    env: {...process.env, POSTGRES_PASSWORD: 'config-test-password', PROJECT_CREATOR_TOKEN: 'config-test-token', GGML_VK_VISIBLE_DEVICES: '0'},
  });
  assert.equal(result.status, 0, result.stderr || result.error?.message);
  const {services, volumes} = JSON.parse(result.stdout);
  const o = services.ollama;
  assert.equal(o.environment.OLLAMA_VULKAN, '1');
  assert.equal(o.environment.CUDA_VISIBLE_DEVICES, '-1');
  assert.equal(o.environment.ROCR_VISIBLE_DEVICES, '-1');
  assert.equal(o.environment.VK_DRIVER_FILES, '/etc/project-creator-intel-vulkan.json');
  assert.equal(o.environment.VK_ICD_FILENAMES, '/etc/project-creator-intel-vulkan.json');
  assert.equal(o.environment.GGML_VK_VISIBLE_DEVICES, '0');
  assert.equal(o.environment.OLLAMA_NO_CLOUD, '1');
  assert.equal(o.environment.OLLAMA_MAX_LOADED_MODELS, '1');
  assert.equal(o.environment.OLLAMA_NUM_PARALLEL, '1');
  assert.equal(o.build.dockerfile, 'Dockerfile.ollama-intel');
  assert.equal(o.image, 'project-creator-ollama-intel:0.40.11');
  assert.equal(o.pull_policy, 'build');
  assert.deepEqual(o.devices.map(d => [d.source, d.target]), [['/dev/dri', '/dev/dri']]);
  assert.ok(!o.privileged);
  assert.ok(!o.deploy?.resources?.reservations?.devices);
  assert.ok(!o.ports?.length);
  assert.ok(o.volumes.some(v => v.source === 'ollama_data' && v.target === '/root/.ollama'));
  assert.ok(volumes.ollama_data);
  assert.equal(o.healthcheck.test[1], 'ollama');
  assert.equal(services.backend.environment.PROJECT_CREATOR_OLLAMA_URL, 'http://ollama:11434');
  for (const name of ['postgres', 'backend', 'frontend', 'model-info']) {
    assert.ok(!services[name].devices?.length, `${name} must not get GPU access`);
  }
});

