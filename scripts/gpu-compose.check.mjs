import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {test} from 'node:test';

// Run in containers CI: Compose config needs neither a GPU nor a running daemon.
for (const [device, file] of [['nvidia', 'compose.gpu.yml'], ['amd', 'compose.amd.yml'], ['amd-vulkan', 'compose.amd-vulkan.yml']]) {
  test(`${device} profile isolates the chosen GPU backend and preserves local data`, () => {
    const result = spawnSync('docker', ['compose', '--profile', 'tools', '-f', 'compose.yml', '-f', file, 'config', '--format', 'json'], {
      encoding: 'utf8', env: {...process.env, POSTGRES_PASSWORD: 'test-only', PROJECT_CREATOR_TOKEN: 'test-only', PROJECT_CREATOR_GPU_ID: 'GPU-1234abcd', GGML_VK_VISIBLE_DEVICES: '0'},
    });
    assert.equal(result.status, 0, result.stderr || result.error?.message);
    const {services, volumes} = JSON.parse(result.stdout);
    const o = services.ollama;
    assert.equal(o.environment.OLLAMA_VULKAN, device === 'amd-vulkan' ? '1' : '0');
    assert.equal(o.environment.OLLAMA_NO_CLOUD, '1');
    assert.ok(!o.privileged);
    assert.ok(!o.ports?.length);
    assert.ok(volumes.postgres_data && volumes.ollama_data);
    assert.ok(o.volumes.some(v => v.source === 'ollama_data' && v.target === '/root/.ollama'));
    if (device === 'nvidia') {
      const [request] = o.deploy.resources.reservations.devices;
      assert.equal(request.driver, 'nvidia');
      assert.deepEqual(request.device_ids, ['GPU-1234abcd']);
      assert.ok(!request.count);
      assert.ok(!o.devices?.length);
    } else if (device === 'amd') {
      assert.equal(o.image, 'ollama/ollama:0.40.11-rocm');
      assert.equal(o.environment.ROCR_VISIBLE_DEVICES, 'GPU-1234abcd');
      assert.equal(o.environment.CUDA_VISIBLE_DEVICES, '-1');
      assert.deepEqual(o.devices.map(d => [d.source, d.target]), [['/dev/kfd', '/dev/kfd'], ['/dev/dri', '/dev/dri']]);
      assert.ok(!o.deploy?.resources?.reservations?.devices);
    } else {
      assert.equal(o.image, 'project-creator-ollama-amd-vulkan:0.40.11');
      assert.equal(o.pull_policy, 'build');
      assert.equal(o.build.dockerfile, 'Dockerfile.ollama-amd-vulkan');
      assert.equal(o.environment.ROCR_VISIBLE_DEVICES, '-1');
      assert.equal(o.environment.CUDA_VISIBLE_DEVICES, '-1');
      assert.equal(o.environment.GGML_VK_VISIBLE_DEVICES, '0');
      assert.equal(o.environment.VK_DRIVER_FILES, '/etc/project-creator-amd-vulkan.json');
      assert.equal(o.environment.VK_ICD_FILENAMES, '/etc/project-creator-amd-vulkan.json');
      assert.deepEqual(o.devices.map(d => [d.source, d.target]), [['/dev/dri', '/dev/dri']]);
      assert.ok(!o.deploy?.resources?.reservations?.devices);
    }
    for (const name of ['postgres', 'backend', 'frontend', 'model-info']) {
      assert.ok(!services[name].devices?.length);
      assert.ok(!services[name].deploy?.resources?.reservations?.devices);
    }
  });
}
