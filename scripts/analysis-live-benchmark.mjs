#!/usr/bin/env node
// Reproducible, mocked browser baseline for issue #47. Run from any directory:
// node scripts/analysis-live-benchmark.mjs [--mode=dev|prod|both] [--root=PATH]
// [--transport=invalidate|push] [--expand-groups] [--output=DIR] [--revision=SHA]
// `root` must be a checkout/archive containing frontend; target dependencies may be symlinked.
import { spawn } from 'node:child_process';
import { createServer } from 'node:net';
import { mkdir, writeFile } from 'node:fs/promises';
import { openSync, closeSync } from 'node:fs';
import { resolve, dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { parse, Kind } from '../frontend/node_modules/graphql/index.mjs';
import { chromium } from '../frontend/node_modules/@playwright/test/index.mjs';

const scriptDir = dirname(fileURLToPath(import.meta.url));
const args = {};
for (let index = 2; index < process.argv.length; index++) {
  const token = process.argv[index];
  if (!token.startsWith('--')) continue;
  const [key, ...rest] = token.slice(2).split('=');
  args[key] = rest.length ? rest.join('=') :
    process.argv[index + 1] && !process.argv[index + 1].startsWith('--') ? process.argv[++index] : 'true';
}
const root = resolve(args.root || join(scriptDir, '../logs/issue-47/baseline'));
const mode = args.mode || 'both';
const transport = args.transport || 'invalidate';
const expandGroups = args['expand-groups'] === 'true';
const outputDir = resolve(args.output || join(scriptDir, '../logs/issue-47'));
const chromiumPath = args.chromium || process.env.E2E_CHROMIUM_PATH;
const pairs = Array.from({ length: 20 }, (_, index) => ({
  id: `pair-${index + 1}`, position: index, status: 'pending', verdict: '',
  explanation: `Mock collision ${index + 1}`, errorCode: '', selectionReasons: ['semantic_similarity'],
  similarityScore: 0.8, resultOrigin: 'model', reusedFrom: '', checkedAt: '2026-01-01T00:00:00Z',
  generatorProfile: 'mock',
  left: { id: `source-${index * 2 + 1}`, version: 1, text: `Source left ${index + 1}: ${'fixture '.repeat(30)}` },
  right: { id: `source-${index * 2 + 2}`, version: 1, text: `Source right ${index + 1}: ${'fixture '.repeat(30)}` },
}));
const sources = pairs.flatMap(({ left, right }) => [left, right]);
const run = {
  __typename: 'ProjectAnalysis', mode: 'related', similarityThreshold: 0.65,
  selectionProfile: 'related-v1', possiblePairs: 20, notSelectedPairs: 0,
  duplicatePairs: 0, reusedPairs: 0, newPairs: 20, completedPairs: 20, pendingPairs: 0,
  runningPairs: 0, failedPairs: 0, collisionPairs: 20, compatiblePairs: 0, insufficientPairs: 0,
  generatorProvenance: '{"adapter":"mock","model":"fixed"}', id: 'run-1', projectId: 'demo',
  status: 'completed', outcome: 'possible_collision', errorCode: '', reasonCode: '',
  missingRequirementIds: [], createdAt: '2026-01-01T00:00:00Z', completedAt: '2026-01-01T00:00:01Z',
  stale: false, generatorProfile: 'mock', embeddingProfile: 'mock', stage: 'finished',
  totalPairs: 20, processedPairs: 20, activePairId: '', sources,
  findings: [], pairs,
};
let progress = 0;
function advanceRun(nextProgress) {
  progress = Math.min(20, nextProgress);
  for (const [index, pair] of pairs.entries()) {
    pair.status = index < progress ? 'completed' : 'pending';
    pair.verdict = index < progress ? 'collision' : '';
  }
  run.status = progress === 20 ? 'completed' : 'running';
  run.outcome = progress ? 'possible_collision' : '';
  run.stage = progress === 20 ? 'finished' : 'comparing';
  run.revision = String(progress + 1);
  run.processedPairs = progress;
  run.activePairId = progress < 20 ? `pair-${progress + 1}` : '';
  run.completedPairs = progress;
  run.pendingPairs = 20 - progress;
  run.collisionPairs = progress;
  run.newPairs = 20;
  run.findings = pairs.slice(0, progress).map((pair) => ({ leftId: pair.left.id, rightId: pair.right.id,
    leftVersion: 1, rightVersion: 1, verdict: pair.verdict, explanation: pair.explanation }));
}
advanceRun(0);
const compactSnapshotFields = [
  'revision', 'mode', 'similarityThreshold', 'possiblePairs', 'notSelectedPairs', 'duplicatePairs',
  'reusedPairs', 'newPairs', 'completedPairs', 'pendingPairs', 'runningPairs', 'failedPairs',
  'collisionPairs', 'compatiblePairs', 'insufficientPairs', 'id', 'projectId', 'status', 'outcome',
  'errorCode', 'reasonCode', 'missingRequirementIds', 'createdAt', 'completedAt', 'stale', 'stage',
  'totalPairs', 'processedPairs', 'activePairId',
];
function compactSnapshot() {
  return Object.fromEntries(compactSnapshotFields.map((field) => [field, run[field]]));
}
const historyItem = { ...run };
const sampleProject = { __typename: 'Project', id: 'demo', name: 'Benchmark project',
  goal: 'Mocked project for live analysis benchmark', description: 'Deterministic fixture',
  createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z', requirements: [] };
const samples = {
  project: run,
  analysisHistory: [historyItem],
  analysisPairsConnection: { __typename: 'AnalysisPairConnection', totalCount: 20,
    edges: pairs.map((node, i) => ({ __typename: 'AnalysisPairEdge', cursor: `cursor-${i + 1}`, node })),
    pageInfo: { __typename: 'PageInfo', endCursor: 'cursor-20', hasNextPage: false } },
  analysisPairs: pairs,
  projectById: sampleProject,
  projectInfo: sampleProject,
  projects: [sampleProject],
  projectsConnection: { totalCount: 1, edges: [{ cursor: 'cursor-1', node: sampleProject }],
    pageInfo: { endCursor: 'cursor-1', hasNextPage: false } },
  scope: run,
};
function select(value, selections, fragments = new Map()) {
  if (value == null) return value;
  if (Array.isArray(value)) return value.map((item) => select(item, selections, fragments));
  if (!selections) return value;
  const out = {};
  for (const item of selections) {
    if (item.kind === Kind.FIELD) {
      const key = item.name.value;
      const alias = item.alias?.value || key;
      if (key === '__typename') out[alias] = value.__typename || 'ProjectAnalysis';
      else if (Object.hasOwn(value, key)) out[alias] = select(value[key], item.selectionSet?.selections, fragments);
      else out[alias] = null;
    } else if (item.kind === Kind.INLINE_FRAGMENT) Object.assign(out, select(value, item.selectionSet.selections, fragments));
    else if (item.kind === Kind.FRAGMENT_SPREAD) {
      const fragment = fragments.get(item.name.value);
      if (fragment) Object.assign(out, select(value, fragment.selectionSet.selections, fragments));
    }
  }
  return out;
}
function queryData(body) {
  const doc = parse(body.query);
  const fragments = new Map(doc.definitions.filter((node) => node.kind === Kind.FRAGMENT_DEFINITION)
    .map((fragment) => [fragment.name.value, fragment]));
  const operation = doc.definitions.find((node) => node.kind === Kind.OPERATION_DEFINITION);
  const opName = operation?.name?.value || '(anonymous)';
  const data = {};
  for (const field of operation?.selectionSet.selections || []) {
    if (field.kind !== Kind.FIELD) continue;
    const key = field.name.value;
    const alias = field.alias?.value || key;
    const variables = body.variables || {};
    const matchingPairs = pairs.filter((pair) =>
      (!variables.verdict || pair.verdict === variables.verdict) &&
      (!variables.status || (variables.status === 'pending'
        ? pair.status === 'pending' || pair.status === 'running'
        : pair.status === variables.status)));
    const offset = variables.after ? Number(String(variables.after).replace('cursor-', '')) : Number(variables.offset || 0);
    const limit = Number(variables.first ?? variables.limit ?? matchingPairs.length);
    const pagePairs = matchingPairs.slice(offset, offset + limit);
    const pageConnection = { __typename: 'AnalysisPairConnection', totalCount: matchingPairs.length,
      edges: pagePairs.map((node, i) => ({ __typename: 'AnalysisPairEdge', cursor: `cursor-${offset + i + 1}`, node })),
      pageInfo: { __typename: 'PageInfo', endCursor: pagePairs.length ? `cursor-${offset + pagePairs.length}` : null,
        hasNextPage: offset + pagePairs.length < matchingPairs.length } };
    const value = key === 'projectAnalysis' || key === 'projectAnalysisSummary' ? run
      : key === 'analysisHistory' ? samples.analysisHistory
      : key === 'analysisPairsConnection' ? pageConnection
      : key === 'analysisPairs' ? pagePairs
      : key === 'analysisPair' ? pairs.find((pair) => pair.id === variables.pairId) || null
      : key === 'project' ? sampleProject
      : key === 'scope' ? run
      : key === 'startProjectAnalysis' || key === 'retryProjectAnalysis' || key === 'cancelProjectAnalysis' ? run
      : key === 'projectsConnection' ? samples.projectsConnection
      : key === 'projects' ? samples.projects
      : null;
    data[alias] = select(value, field.selectionSet?.selections, fragments);
  }
  return { opName, data };
}

async function availablePort() {
  const server = createServer();
  await new Promise((resolveListen) => server.listen(0, '127.0.0.1', resolveListen));
  const port = server.address().port;
  await new Promise((resolveClose) => server.close(resolveClose));
  return port;
}
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function waitForPage(url, child) {
  for (let attempt = 0; attempt < 120; attempt++) {
    if (child.exitCode !== null) throw Error(`Next exited early with code ${child.exitCode}`);
    try { const response = await fetch(url); if (response.ok) return; } catch {}
    await sleep(500);
  }
  throw Error(`Timed out waiting for ${url}`);
}

async function benchmark(profile) {
  advanceRun(0);
  const frontendDir = join(root, 'frontend');
  const port = await availablePort();
  const url = `http://127.0.0.1:${port}`;
  const logPath = join(outputDir, `${profile}-server.log`);
  await mkdir(outputDir, { recursive: true });
  let command, commandArgs;
  if (profile === 'dev') {
    command = join(frontendDir, 'node_modules/.bin/next'); commandArgs = ['dev', '-H', '127.0.0.1', '-p', String(port)];
  } else {
    command = join(frontendDir, 'node_modules/.bin/next'); commandArgs = ['start', '-H', '127.0.0.1', '-p', String(port)];
  }
  const logFile = openSync(logPath, 'w');
  const child = spawn(command, commandArgs, { cwd: frontendDir, env: {
    ...process.env, PORT: String(port), PROJECT_CREATOR_TOKEN: 'benchmark-token',
    NEXT_TELEMETRY_DISABLED: '1', NODE_ENV: profile === 'dev' ? 'development' : 'production',
  }, stdio: ['ignore', logFile, logFile] });
  const result = { profile, url, requestedPairs: 20, transport, expandGroups, graphql: {}, eventStreams: [],
    maxInflightRequests: 0, maxInflightGraphql: 0, maxObservedPending: 0,
    summaryResponseBytes: [], summaryUnchangedRetransfers: 0, resultVisibleAtMs: null,
    firstPairDataAtMs: null, pairRowsTransferred: 0, unchangedSourceBytesRetransferred: 0,
    sseResponseBytes: 0,
    firstEventAtMs: null, firstPostEventSummaryAtMs: null };
  const active = new Map();
  const activeGraphql = new Set();
  const uniquePairIds = new Set();
  const uniquePairVersions = new Set();
  const uniqueSourceVersions = new Set();
  const start = Date.now();
  let browser;
  try {
    await waitForPage(url, child);
    browser = await chromium.launch(chromiumPath ? { headless: true, executablePath: chromiumPath } : { headless: true });
  } catch (error) {
    if (String(error).includes('Executable') && profile === 'dev') throw error;
    if (!browser) {
      try { browser = await chromium.launch(chromiumPath ? { headless: true, executablePath: chromiumPath } : { headless: true }); }
      catch { throw error; }
    }
  }
  try {
    const context = await browser.newContext();
    const page = await context.newPage();
    page.on('request', (request) => {
      active.set(request, Date.now());
      result.maxInflightRequests = Math.max(result.maxInflightRequests, active.size);
      result.maxObservedPending = Math.max(result.maxObservedPending, active.size);
      if (request.url().includes('/api/graphql')) {
        activeGraphql.add(request);
        result.maxInflightGraphql = Math.max(result.maxInflightGraphql, activeGraphql.size);
      }
    });
    page.on('requestfinished', (request) => { active.delete(request); activeGraphql.delete(request); });
    page.on('requestfailed', (request) => { active.delete(request); activeGraphql.delete(request); });
    await page.route('**/api/graphql', async (route) => {
      const request = route.request();
      const raw = request.postData() || '';
      const body = JSON.parse(raw);
      const { opName, data } = queryData(body);
      const entry = result.graphql[opName] ||= { calls: 0, requestBytes: 0, responseBytes: 0, callsAtMs: [] };
      entry.calls++; entry.requestBytes += Buffer.byteLength(raw); entry.callsAtMs.push(Date.now() - start);
      const response = JSON.stringify({ data });
      const responseBytes = Buffer.byteLength(response);
      entry.responseBytes += responseBytes;
      if (opName === 'ProjectAnalysisSummary') {
        result.summaryResponseBytes.push(responseBytes);
        const sourceBody = JSON.stringify(data.projectAnalysis?.sources || []);
        const sourceBytes = Buffer.byteLength(sourceBody);
        if (sourceBody === result.lastSummarySources) result.unchangedSourceBytesRetransferred += sourceBytes;
        result.lastSummarySources = sourceBody;
        if (result.firstEventAtMs !== null && result.firstPostEventSummaryAtMs === null)
          result.firstPostEventSummaryAtMs = Date.now() - start;
        if (result.summaryResponseBytes.length > 1 && response === result.lastSummaryResponse)
          result.summaryUnchangedRetransfers++;
        result.lastSummaryResponse = response;
      }
      if (opName === 'AnalysisPairConnection') {
        const edges = data.analysisPairsConnection?.edges || [];
        result.pairRowsTransferred += edges.length;
        for (const edge of edges) {
          uniquePairIds.add(edge.node.id);
          uniquePairVersions.add(`${edge.node.id}:${edge.node.position}`);
          for (const side of ['left', 'right']) {
            const source = edge.node[side];
            const key = `${source.id}:${source.version}`;
            if (uniqueSourceVersions.has(key)) {
              result.unchangedSourceReferenceCount = (result.unchangedSourceReferenceCount || 0) + 1;
              result.unchangedSourceReferenceBytes = (result.unchangedSourceReferenceBytes || 0) + Buffer.byteLength(JSON.stringify({ [side]: source }));
            } else uniqueSourceVersions.add(key);
          }
        }
        if (edges.length && result.firstPairDataAtMs === null) result.firstPairDataAtMs = Date.now() - start;
      }
      await route.fulfill({ status: 200, contentType: 'application/json', body: response });
    });
    await page.route('**/api/events', async (route) => {
      const cursor = route.request().headers()['last-event-id'] || '';
      const streamIndex = result.eventStreams.length;
      const eventId = String(streamIndex + 1);
      const since = Date.now() - start;
      result.eventStreams.push({ cursor: cursor || null, openedAtMs: since, eventId });
      if (result.firstEventAtMs === null) result.firstEventAtMs = since;
      const keys = JSON.stringify({ schemaVersion: 1, keys: ['project:demo'] });
      result.eventStreams.at(-1).invalidationSent = streamIndex < 20;
      if (streamIndex < 20) {
        advanceRun(streamIndex + 1);
        if (progress === 20) result.allPairsCompleteAtMs = Date.now() - start;
      }
      const eventName = transport === 'push' ? 'analysis.progress' : 'cache.invalidate';
      const payload = transport === 'push'
        ? JSON.stringify({ schemaVersion: 1, projectId: 'demo', runId: 'run-1', revision: run.revision, snapshot: compactSnapshot() })
        : keys;
      const streamBody = streamIndex < 20
        ? `id: ${eventId}\nevent: ${eventName}\ndata: ${payload}\n\n`
        : ': finite stream complete\n\n';
      result.eventStreams.at(-1).responseBytes = Buffer.byteLength(streamBody);
      result.sseResponseBytes += Buffer.byteLength(streamBody);
      await route.fulfill({ status: 200, contentType: 'text/event-stream', headers: { 'cache-control': 'no-cache' },
        body: streamBody });
    });
    await page.goto(`${url}/projects/demo/analysis`, { waitUntil: 'domcontentloaded', timeout: 60000 });
    try {
      await page.getByText(/(?:Обработано [1-9]\d* из 20|Processed (?:[1-9]|1\d|20) of 20)/).waitFor({ timeout: 15000 });
      result.firstProgressAssert = true;
    } catch { result.firstProgressAssert = false; }
    result.resultVisibleAtMs = result.firstProgressAssert ? Date.now() - start : null;
    const streamDeadline = Date.now() + 15000;
    while (result.eventStreams.filter((event) => event.invalidationSent).length < 20 && Date.now() < streamDeadline) {
      if (expandGroups) {
        await page.locator('details.data-group').evaluateAll((elements) => {
          for (const element of elements) {
            if (element instanceof HTMLDetailsElement && !element.open &&
              /:\s*\d+$/.test(element.querySelector(':scope > summary')?.textContent?.trim() || '')) {
              element.querySelector(':scope > summary')?.click();
            }
          }
        });
      }
      await page.waitForTimeout(100);
    }
    try {
      await page.getByText(/(?:Обработано 20 из 20|Processed 20 of 20)/).waitFor({ timeout: 5000 });
      result.finalProgressAssert = true;
    } catch {
      result.finalProgressAssert = false;
      throw Error(`${profile}/${transport}: UI did not show all 20 processed pairs`);
    }
    await page.waitForTimeout(2500);
    result.visiblePairCount = await page.locator('.analysis-pair[data-pair-id]').count();
    result.pair20Visible = await page.locator('[data-pair-id="pair-20"]').count() > 0;
    result.uniquePairIdsReturned = uniquePairIds.size;
    result.uniquePairVersionsReturned = uniquePairVersions.size;
    result.uniqueSourceVersionsReturned = uniqueSourceVersions.size;
    result.repeatedPairRowsTransferred = Math.max(0, result.pairRowsTransferred - uniquePairVersions.size);
    result.strictModeExpected = profile === 'dev';
    result.documentTitle = await page.title();
    result.graphqlRequestBytes = Object.values(result.graphql).reduce((sum, entry) => sum + entry.requestBytes, 0);
    result.graphqlResponseBytes = Object.values(result.graphql).reduce((sum, entry) => sum + entry.responseBytes, 0);
    result.totalMockApiBytes = result.graphqlRequestBytes + result.graphqlResponseBytes + result.sseResponseBytes;
    await context.close();
  } finally {
    await browser?.close();
    child.kill('SIGTERM');
    await Promise.race([
      new Promise((resolveClose) => child.once('exit', resolveClose)),
      sleep(4000).then(() => child.kill('SIGKILL')),
    ]);
    closeSync(logFile);
  }
  delete result.lastSummaryResponse;
  return result;
}

await mkdir(outputDir, { recursive: true });
if (!['dev', 'prod', 'both'].includes(mode)) throw Error(`Invalid mode: ${mode}`);
if (!['invalidate', 'push'].includes(transport)) throw Error(`Invalid transport: ${transport}`);
const profiles = mode === 'both' ? ['prod', 'dev'] : [mode];
if (profiles.includes('prod') && args['skip-build'] !== 'true') {
  const build = spawn(join(root, 'frontend/node_modules/.bin/next'), ['build'], {
    cwd: join(root, 'frontend'), env: { ...process.env, NEXT_TELEMETRY_DISABLED: '1', PROJECT_CREATOR_TOKEN: 'benchmark-token' },
    stdio: 'inherit',
  });
  const code = await new Promise((resolveExit) => build.once('exit', resolveExit));
  if (code !== 0) throw Error(`Baseline production build failed: ${code}`);
}
const runs = [];
for (const profile of profiles) runs.push(await benchmark(profile));
const report = { generatedAt: new Date().toISOString(), revision: args.revision || 'd5e1bb23d67bd1191da92b716339c130901c8b5b',
  note: 'Mocked browser measurements only; not actual network traffic or backend payload volume.', runs };
const reportPath = join(outputDir, 'baseline.json');
await writeFile(reportPath, `${JSON.stringify(report, null, 2)}\n`);
console.log(`Wrote ${reportPath}`);
