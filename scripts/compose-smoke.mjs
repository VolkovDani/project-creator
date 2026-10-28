// Run only against a dedicated clean Compose project. Requires Node.js >=24.
// The default CI stack uses a deterministic Ollama fixture; real weights are opt-in.
import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {setTimeout as delay} from 'node:timers/promises';

const base = process.env.SMOKE_URL || `http://127.0.0.1:${process.env.APP_PORT || '3000'}`;
const endpoint = `${base.replace(/\/$/, '')}/api/graphql`;
const projectName = process.env.COMPOSE_PROJECT_NAME;
assert.ok(projectName?.endsWith('-smoke'), 'COMPOSE_PROJECT_NAME must end in -smoke; use a disposable, clean test stack');
const fixtureMode = (process.env.COMPOSE_FILE || '').split(':').some(path => /(^|\/)compose\.smoke\.yml$/.test(path));
assert.ok(fixtureMode || process.env.REAL_MODEL_SMOKE === '1', 'Use compose.smoke.yml or explicitly opt in to real weights with REAL_MODEL_SMOKE=1');
const envFileArgs = process.env.COMPOSE_ENV_FILE ? ['--env-file', process.env.COMPOSE_ENV_FILE] : [];
const compose = (...args) => execFileSync('docker', ['compose', ...envFileArgs, ...args], {stdio: 'inherit', timeout: 240_000});
compose('version');

async function graphql(query, variables = {}, timeout = 210_000) {
  const response = await fetch(endpoint, {method: 'POST', headers: {'content-type': 'application/json'}, body: JSON.stringify({query, variables}), signal: AbortSignal.timeout(timeout)});
  assert.equal(response.status, 200, `GraphQL returned HTTP ${response.status}`);
  const body = await response.json();
  assert.equal(body.errors, undefined, `GraphQL errors: ${JSON.stringify(body.errors)}`);
  assert.ok(body.data, 'GraphQL returned no data');
  return body.data;
}

async function ready() {
  const deadline = Date.now() + 120_000;
  while (true) {
    try { await graphql('{projects(limit:1,offset:0){id}}', {}, 5000); return; }
    catch (error) { if (Date.now() >= deadline) throw error; await delay(1000); }
  }
}

await ready();
assert.equal((await graphql('{projects(limit:1,offset:0){id}}')).projects.length, 0, 'Smoke test requires an empty database; refusing to add fixtures to existing data');
const homepage = await fetch(base, {signal: AbortSignal.timeout(10_000)});
assert.equal(homepage.status, 200, 'Frontend homepage is unavailable');
assert.match(await homepage.text(), /<html/);

const input = {name: 'Compose smoke test', goal: 'Проверка локального сценария без облачных API.', description: 'Synthetic smoke data only.'};
const created = await graphql('mutation($input:ProjectInput!){createProject(input:$input){project{id name goal} fieldErrors{code}}}', {input});
assert.deepEqual(created.createProject.fieldErrors, []);
const project = created.createProject.project;
assert.ok(project?.id, 'Project was not created');
const texts = ['При любом заказе оплата разрешена только банковской картой. Другие способы запрещены.', 'При любом заказе оплата разрешена только наличными. Другие способы запрещены.'];
const requirements = [];
for (const text of texts) {
  const data = await graphql('mutation($input:CreateRequirementInput!){createRequirement(input:$input){requirement{id version text} fieldErrors{code}}}', {input: {projectId: project.id, text}});
  assert.deepEqual(data.createRequirement.fieldErrors, []);
  assert.ok(data.createRequirement.requirement?.id, 'Requirement was not created');
  requirements.push(data.createRequirement.requirement);
}
for (const requirement of requirements) {
  const data = await graphql('mutation($project:ID!,$requirement:ID!){indexRequirement(projectId:$project,requirementId:$requirement){status version errorCode}}', {project: project.id, requirement: requirement.id});
  assert.equal(data.indexRequirement.status, 'ready', `Indexing failed: ${data.indexRequirement.errorCode}`);
  assert.equal(data.indexRequirement.version, 1);
}
const analysisFields = 'id status outcome stale errorCode findings{leftId rightId leftVersion rightVersion verdict explanation} sources{id version text}';
const started = await graphql(`mutation($project:ID!){startProjectAnalysis(projectId:$project){${analysisFields}}}`, {project: project.id});
const analysis = started.startProjectAnalysis;
assert.equal(analysis.status, 'completed', `Analysis failed: ${analysis.errorCode}`);
assert.equal(analysis.outcome, 'possible_collision');
assert.equal(analysis.stale, false);
assert.equal(analysis.sources.length, 2);
assert.ok(analysis.findings.some(finding => finding.verdict === 'collision'), 'Known collision was not reported');
const known = new Map(requirements.map(requirement => [requirement.id, requirement]));
for (const source of analysis.sources) assert.deepEqual(source, known.get(source.id), 'Analysis did not preserve the source snapshot');
for (const finding of analysis.findings) {
  assert.ok(known.has(finding.leftId) && known.has(finding.rightId), 'Analysis invented a source');
  assert.equal(finding.leftVersion, 1); assert.equal(finding.rightVersion, 1);
  assert.ok(finding.explanation.trim());
}
console.log('Created project, requirements, embeddings and versioned collision analysis through the frontend proxy.');

// Restart the database first, wait for readiness, then restart the backend. No volume deletion.
compose('restart', 'postgres');
compose('up', '-d', '--wait', '--wait-timeout', '120', 'postgres');
compose('restart', 'backend');
compose('up', '-d', '--wait', '--wait-timeout', '120');
await ready();
const persisted = await graphql(`query($project:ID!){project(id:$project){id name goal} requirements(projectId:$project,limit:20,offset:0){id version text} requirementIndex(projectId:$project,limit:20,offset:0){requirementId status version} projectAnalysis(projectId:$project){${analysisFields}} searchRequirements(projectId:$project,text:"оплата заказа",limit:5){requirementId version sourceText}}`, {project: project.id});
assert.deepEqual(persisted.project, project, 'Project was lost after restart');
assert.deepEqual(persisted.requirements.toSorted((a, b) => a.id.localeCompare(b.id)), requirements.toSorted((a, b) => a.id.localeCompare(b.id)), 'Requirements changed after restart');
assert.deepEqual(persisted.projectAnalysis, analysis, 'Analysis or source snapshots changed after restart');
assert.equal(persisted.requirementIndex.length, 2);
assert.ok(persisted.requirementIndex.every(index => index.status === 'ready' && index.version === 1));
assert.equal(persisted.searchRequirements.length, 2, 'Stored vectors are not searchable after restart');
for (const hit of persisted.searchRequirements) {
  assert.equal(hit.version, 1);
  assert.equal(hit.sourceText, known.get(hit.requirementId)?.text);
}
console.log('Compose smoke passed: project, source versions, vectors and analysis survived PostgreSQL/backend restart.');
