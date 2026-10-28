// Deterministic test double for the container smoke test. Never used by compose.yml.
// It checks application wiring and persistence, not the quality of real models.
import {createServer} from 'node:http';
import {pathToFileURL} from 'node:url';

export const EMBEDDING_MODEL = 'smoke-embed:1';
export const GENERATOR_MODEL = 'smoke-generator:1';
export const EMBEDDING_DIGEST = `sha256:${'1'.repeat(64)}`;
export const GENERATOR_DIGEST = `sha256:${'2'.repeat(64)}`;

function send(response, status, body) {
  response.writeHead(status, {'content-type': 'application/json'});
  response.end(JSON.stringify(body));
}

export function createFixtureServer() {
  return createServer(async (request, response) => {
    if (request.method === 'GET' && request.url === '/api/tags') {
      send(response, 200, {models: [{name: EMBEDDING_MODEL, digest: EMBEDDING_DIGEST}, {name: GENERATOR_MODEL, digest: GENERATOR_DIGEST}]});
      return;
    }
    if (request.method === 'GET' && request.url === '/health') {
      send(response, 200, {status: 'test-fixture'});
      return;
    }
    if (request.method !== 'POST' || !['/api/embed', '/api/chat'].includes(request.url)) {
      send(response, 404, {error: 'Test fixture route not found'});
      return;
    }
    try {
      let data = '';
      for await (const chunk of request) {
        data += chunk;
        if (data.length > 1024 * 1024) throw Error('Request too large');
      }
      const body = JSON.parse(data);
      if (request.url === '/api/embed') {
        if (body.model !== EMBEDDING_MODEL || !Array.isArray(body.input) || body.input.length < 1 || body.input.length > 32 || body.input.some(text => typeof text !== 'string' || !text.trim())) throw Error('Invalid embedding request');
        // All fixture requirements are related; exact cosine retrieval must find both.
        const vector = Array(1024).fill(0); vector[0] = 1;
        send(response, 200, {embeddings: body.input.map(() => vector)});
        return;
      }
      if (body.model !== GENERATOR_MODEL || body.stream !== false || !Array.isArray(body.messages)) throw Error('Invalid comparison request');
      const pair = JSON.parse(body.messages.find(message => message.role === 'user')?.content);
      for (const source of [pair.left, pair.right]) {
        if (typeof source?.id !== 'string' || !Number.isInteger(source.version) || source.version < 1 || typeof source.text !== 'string') throw Error('Invalid source');
      }
      const finding = {leftId: pair.left.id, rightId: pair.right.id, leftVersion: pair.left.version, rightVersion: pair.right.version, verdict: 'collision', explanation: 'Тестовая пара требует взаимоисключающих способов оплаты в одинаковых условиях. Нужна проверка человеком.'};
      send(response, 200, {done: true, done_reason: 'stop', message: {role: 'assistant', content: JSON.stringify(finding)}});
    } catch {
      send(response, 400, {error: 'Invalid test fixture request'});
    }
  });
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  createFixtureServer().listen(11434, '0.0.0.0', () => console.log('Smoke-only Ollama fixture listening on 11434'));
}
