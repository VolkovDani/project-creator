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

// Exercise the actual smoke entry point with deterministic HTTP, Docker and time.
// No subprocess launched by the smoke can reach Docker or a real model.
const fakeServices = String.raw`
import childProcess from 'node:child_process';
import timers from 'node:timers/promises';
import {syncBuiltinESMExports} from 'node:module';
import assert from 'node:assert/strict';
const mode = process.env.SMOKE_TEST_MODE;
let clock = 0, starts = 0, polls = 0;
Object.defineProperty(globalThis, 'performance', {value: {now: () => clock}});
childProcess.execFileSync = () => Buffer.from('fixture');
timers.setTimeout = async milliseconds => { clock += mode === 'timeout' ? 2_700_001 : milliseconds; };
syncBuiltinESMExports();
const requirements = [];
let project;
function analysis(status = 'completed') {
  return {id: 'run-a', status, outcome: status === 'completed' ? 'possible_collision' : '', stale: false, errorCode: '',
    sources: requirements.map(source => ({...source})), findings: status === 'completed' ? [{leftId: 'r1', rightId: 'r2', leftVersion: 1,
      rightVersion: 1, verdict: 'collision', explanation: 'Synthetic explanation.'}] : []};
}
globalThis.fetch = async (_url, options) => {
  if (!options?.body) return {status: 200, text: async () => '<html></html>'};
  const {query, variables} = JSON.parse(options.body);
  let data;
  if (query.includes('startProjectAnalysis(')) {
    starts++;
    assert.equal(starts, 1, 'polling must never start another analysis');
    data = {startProjectAnalysis: analysis('running')};
    if (mode === 'missing-id') delete data.startProjectAnalysis.id;
  } else if (query.includes('project(id:')) {
    data = {project, requirements, requirementIndex: requirements.map(item => ({requirementId: item.id, status: 'ready', version: 1})),
      projectAnalysis: analysis(), searchRequirements: requirements.map(item => ({requirementId: item.id, version: 1, sourceText: item.text}))};
  } else if (query.includes('projectAnalysis(')) {
    polls++;
    assert.deepEqual(variables, {project: 'p'});
    let result = analysis(mode === 'complete' && polls > 1 ? 'completed' : 'running');
    if (mode === 'failure') result = {...result, status: 'failed', errorCode: 'GENERATION_FAILED'};
    if (mode === 'swapped-run') result = {...analysis(), id: 'other-run'};
    if (mode === 'changed-source') result.sources[0].version = 2;
    data = {projectAnalysis: result};
  } else if (query.includes('createProject(')) {
    project = {id: 'p', name: variables.input.name, goal: variables.input.goal};
    data = {createProject: {project, fieldErrors: []}};
  } else if (query.includes('createRequirement(')) {
    const requirement = {id: 'r'+(requirements.length+1), version: 1, text: variables.input.text};
    requirements.push(requirement);
    data = {createRequirement: {requirement, fieldErrors: []}};
  } else if (query.includes('indexRequirement(')) {
    data = {indexRequirement: {requirementId: variables.requirement, status: mode === 'complete' ? 'running' : 'ready', version: 1, errorCode: ''}};
  } else if (query.includes('requirementIndexByIds(')) {
    assert.equal(variables.project, 'p');
    assert.ok(requirements.some(row => row.id === variables.requirement));
    data = {requirementIndexByIds: [{requirementId: variables.requirement, status: 'ready', version: 1, errorCode: ''}]};
  } else if (query.includes('projects(')) {
    data = {projects: []};
  } else throw new Error('Unexpected diagnostic operation: '+query);
  return {status: 200, json: async () => ({data})};
};
process.on('exit', () => console.log('FIXTURE_COUNTS '+JSON.stringify({starts, polls})));
`;
function asyncSmoke(mode) {
  const preload = 'data:text/javascript,'+encodeURIComponent(fakeServices);
  const result = spawnSync(process.execPath, ['--import', preload, script], {encoding: 'utf8', timeout: 10_000,
    env: {...process.env, COMPOSE_PROJECT_NAME: 'project-creator-smoke', COMPOSE_FILE: 'compose.yml:compose.smoke.yml', SMOKE_TEST_MODE: mode}});
  assert.ifError(result.error);
  const counts = JSON.parse(result.stdout.match(/FIXTURE_COUNTS (\{[^\n]+\})/)[1]);
  assert.equal(counts.starts, 1, result.stderr);
  return {...result, counts};
}
test('container smoke waits for the same analysis to complete and validates its persisted snapshots', () => {
  const result = asyncSmoke('complete');
  assert.equal(result.status, 0, result.stderr);
  assert.equal(result.counts.polls, 2);
  assert.match(result.stdout, /Compose smoke passed/);
});
test('container smoke reports an asynchronous analysis failure without restarting it', () => {
  const result = asyncSmoke('failure');
  assert.equal(result.status, 1);
  assert.equal(result.counts.polls, 1);
  assert.match(result.stderr, /Analysis failed: GENERATION_FAILED/);
});
test('container smoke rejects another run instead of accepting its completed result', () => {
  const result = asyncSmoke('swapped-run');
  assert.equal(result.status, 1);
  assert.equal(result.counts.polls, 1);
  assert.match(result.stderr, /run changed/i);
});
test('container smoke requires the id of an asynchronous analysis', () => {
  const result = asyncSmoke('missing-id');
  assert.equal(result.status, 1);
  assert.equal(result.counts.polls, 0);
  assert.match(result.stderr, /analysis.*id/i);
});
test('container smoke rejects a changed source snapshot while analysis is still running', () => {
  const result = asyncSmoke('changed-source');
  assert.equal(result.status, 1);
  assert.equal(result.counts.polls, 1);
  assert.match(result.stderr, /source snapshot/i);
});
test('container smoke bounds the asynchronous wait using monotonic time', () => {
  const result = asyncSmoke('timeout');
  assert.equal(result.status, 1);
  assert.ok(result.counts.polls <= 1);
  assert.match(result.stderr, /timed out/i);
});
