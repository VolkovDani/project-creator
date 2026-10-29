import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {test} from 'node:test';

for (const arc of [false, true]) {
  test(`NPU override uses only a local socket and preserves ${arc ? 'Intel Arc' : 'CPU'} embeddings`, () => {
    const binary = process.env.COMPOSE_BINARY || 'docker';
    const args = [...(process.env.COMPOSE_BINARY ? [] : ['compose']), '-f', 'compose.yml',
      ...(arc ? ['-f', 'compose.intel.yml'] : []), '-f', 'compose.npu.yml', 'config', '--format', 'json'];
    const run = spawnSync(binary, args, {encoding: 'utf8', env: {...process.env,
      POSTGRES_PASSWORD: 'test-only', PROJECT_CREATOR_TOKEN: 'test-only',
      PROJECT_CREATOR_NPU_PROFILE: 'a'.repeat(64), NPU_SOCKET_DIR: '/tmp/project-creator-npu', NPU_SOCKET_GID: '1000'}});
    assert.equal(run.status, 0, run.stderr || run.error?.message);
    const {services, volumes} = JSON.parse(run.stdout);
    const b = services.backend;
    assert.equal(b.environment.PROJECT_CREATOR_GENERATOR_RUNTIME, 'openvino-npu');
    assert.equal(b.environment.PROJECT_CREATOR_NPU_PROFILE, 'a'.repeat(64));
    assert.equal(b.environment.PROJECT_CREATOR_NPU_SOCKET, '/run/project-creator-npu/runtime.sock');
    assert.deepEqual(b.group_add, ['1000']);
    assert.equal(b.environment.PROJECT_CREATOR_OLLAMA_URL, 'http://ollama:11434');
    const mount = b.volumes.find(v => v.target === '/run/project-creator-npu');
    assert.equal(mount.type, 'bind');
    assert.equal(mount.source, '/tmp/project-creator-npu');
    assert.equal(mount.read_only, true);
    assert.ok(!mount.bind?.create_host_path);
    assert.ok(!b.privileged);
    assert.ok(!b.devices?.length);
    assert.ok(!b.ports?.length);
    assert.ok(volumes.postgres_data && volumes.ollama_data);
    assert.equal(services.ollama.environment.OLLAMA_NO_CLOUD, '1');
    assert.equal(services.ollama.environment.OLLAMA_VULKAN, arc ? '1' : undefined);
    assert.ok(!services.ollama.ports?.length);
  });
}
