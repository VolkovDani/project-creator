import { type BrowserContext, expect, type Locator, type Page, test } from '@playwright/test';

let fixtureRevision = 0;

const sources = [
  { id: 'left', version: 1, text: 'Оплата разрешена только картой.' },
  { id: 'right', version: 2, text: 'Оплата разрешена только наличными.' },
];
const finding = {
  leftId: 'left',
  rightId: 'right',
  leftVersion: 1,
  rightVersion: 2,
  verdict: 'collision',
  explanation: 'Для одной оплаты указаны взаимоисключающие способы.',
};
const candidate = (id: string, position: number, status: string, verdict = '') => ({
  id,
  position,
  status,
  left: sources[0],
  right: sources[1],
  verdict,
  explanation: verdict ? finding.explanation : '',
  errorCode: '',
  selectionReasons: ['semantic_similarity'],
  similarityScore: 0.8,
  resultOrigin: 'model',
  reusedFrom: '',
  checkedAt: 'now',
  generatorProfile: 'g',
});
const complete = (overrides: Record<string, unknown> = {}) => {
  fixtureRevision++;
  const pairs = (overrides.pairs ||
    ((overrides.findings as unknown[] | undefined)?.length === 0
      ? []
      : [candidate('legacy-0', 0, 'completed', 'collision')])) as ReturnType<typeof candidate>[];
  const count = (status: string, verdict = '') =>
    pairs.filter((pair) => pair.status === status && (!verdict || pair.verdict === verdict)).length;
  return {
    mode: 'related',
    similarityThreshold: 0.65,
    selectionProfile: 'related-v1',
    possiblePairs: 10,
    notSelectedPairs: 9,
    duplicatePairs: count('completed', 'duplicate'),
    id: 'run-1',
    projectId: 'demo',
    revision: String(fixtureRevision),
    status: 'completed',
    outcome: 'possible_collision',
    errorCode: '',
    reasonCode: '',
    missingRequirementIds: [],
    createdAt: new Date(Date.UTC(2026, 0, 1) + fixtureRevision).toISOString(),
    completedAt: 'now',
    stale: false,
    generatorProfile: 'g',
    generatorProvenance: '{"adapter":"fake","model":"test","digest":"abc"}',
    embeddingProfile: 'e',
    stage: 'finished',
    totalPairs: 1,
    processedPairs: 1,
    activePairId: '',
    sources,
    findings: [finding],
    reusedPairs: 0,
    newPairs: count('completed'),
    completedPairs: count('completed'),
    pendingPairs: count('pending'),
    runningPairs: count('running'),
    failedPairs: count('failed'),
    collisionPairs: count('completed', 'collision'),
    compatiblePairs: count('completed', 'compatible'),
    insufficientPairs: count('completed', 'insufficient'),
    ...overrides,
    pairs,
  };
};
function pairPage(run: ReturnType<typeof complete> | null, variables: Record<string, unknown>) {
  return (run?.pairs || [])
    .filter(
      (pair) =>
        (variables.status === 'pending'
          ? pair.status === 'pending' || pair.status === 'running'
          : !variables.status || pair.status === variables.status) &&
        (!variables.verdict || pair.verdict === variables.verdict),
    )
    .slice(Number(variables.offset), Number(variables.offset) + Number(variables.limit));
}
function pairDetail(run: ReturnType<typeof complete> | null, pairId: unknown) {
  return run?.pairs.find((pair) => pair.id === pairId) || null;
}
function pairConnection(
  run: ReturnType<typeof complete> | null,
  variables: Record<string, unknown>,
) {
  const rows = run?.pairs || [];
  const filtered = rows.filter(
    (pair) =>
      (variables.status === 'pending'
        ? pair.status === 'pending' || pair.status === 'running'
        : !variables.status || pair.status === variables.status) &&
      (!variables.verdict || pair.verdict === variables.verdict),
  );
  const offset = variables.after ? Number(String(variables.after).replace('cursor-', '')) : 0;
  const batch = filtered.slice(offset, offset + Number(variables.first ?? 40));
  return {
    edges: batch.map((node, index) => ({ cursor: `cursor-${offset + index + 1}`, node })),
    pageInfo: {
      endCursor: batch.length ? `cursor-${offset + batch.length}` : null,
      hasNextPage: offset + batch.length < filtered.length,
    },
    totalCount: filtered.length,
  };
}
function requirementConnection<T extends { id: string }>(
  rows: T[],
  variables: Record<string, unknown>,
) {
  const offset = variables.after ? Number(String(variables.after).replace('cursor-', '')) : 0;
  const batch = rows.slice(offset, offset + Number(variables.first ?? 40));
  return {
    edges: batch.map((node, index) => ({ cursor: `cursor-${offset + index + 1}`, node })),
    pageInfo: {
      endCursor: batch.length ? `cursor-${offset + batch.length}` : null,
      hasNextPage: offset + batch.length < rows.length,
    },
    totalCount: rows.length,
  };
}
async function fixture(
  page: Page,
  initial: unknown = null,
  onStart?: () => unknown,
  onCancel?: () => Promise<unknown>,
) {
  let latest = initial as ReturnType<typeof complete> | null,
    calls = 0;
  await page.route('**/api/events', (route) =>
    route.fulfill({ status: 200, contentType: 'text/event-stream', body: ': heartbeat\n\n' }),
  );
  await page.route('**/api/graphql', async (route) => {
    const { query, variables } = route.request().postDataJSON();
    let data: unknown;
    if (query.includes('startProjectAnalysis(')) {
      calls++;
      latest = (onStart ? onStart() : complete()) as ReturnType<typeof complete>;
      data = { startProjectAnalysis: latest };
    } else if (query.includes('cancelProjectAnalysis(')) {
      expect(variables).toEqual({ projectId: 'demo', analysisId: latest?.id });
      latest = (await onCancel?.()) as ReturnType<typeof complete>;
      data = { cancelProjectAnalysis: latest };
    } else if (query.includes('analysisHistory('))
      data = { analysisHistory: latest ? [latest] : [] };
    else if (query.includes('analysisPairsConnection('))
      data = { analysisPairsConnection: pairConnection(latest, variables) };
    else if (query.includes('analysisPair('))
      data = { analysisPair: latest?.pairs.find((pair) => pair.id === variables.pairId) || null };
    else if (query.includes('analysisPairs('))
      data = { analysisPairs: pairPage(latest, variables) };
    else if (
      query.includes('projectAnalysisSummary(') ||
      query.includes('analysisRunMetadata(') ||
      query.includes('projectAnalysis(')
    )
      data = { projectAnalysis: latest };
    else if (query.includes('requirementIndex(')) data = { requirementIndex: [] };
    else if (query.includes('requirementsConnection('))
      data = {
        requirementsConnection: requirementConnection(
          sources.map((source) => ({
            ...source,
            projectId: 'demo',
            createdAt: 'now',
            updatedAt: 'now',
          })),
          variables,
        ),
      };
    else if (query.includes('requirements('))
      data = {
        requirements: sources.map((source) => ({
          ...source,
          projectId: 'demo',
          createdAt: 'now',
          updatedAt: 'now',
        })),
      };
    else
      data = {
        project: {
          id: 'demo',
          name: 'Анализ проекта',
          goal: 'G',
          description: '',
          createdAt: 'now',
          updatedAt: 'now',
        },
      };
    await route.fulfill({ json: { data } });
  });
  return () => calls;
}
async function expandGroup(group: Locator) {
  if ((await group.getAttribute('open')) === null) await group.locator('summary').click();
}
async function expandPair(group: Locator, index = 0) {
  const article = group.getByRole('article').nth(index);
  const details = article.locator('details').first();
  if ((await details.getAttribute('open')) === null) await details.locator('summary').click();
  return article;
}
test('analysis runs only after explicit action and shows versioned sources', async ({ page }) => {
  const calls = await fixture(page);
  await page.goto('/projects/demo/analysis');
  await expect(page.getByRole('heading', { name: 'Анализ возможных коллизий' })).toBeVisible();
  expect(calls()).toBe(0);
  await page.getByRole('button', { name: 'Проверить требования', exact: true }).click();
  await expect(page.getByText('Возможная коллизия найдена', { exact: true })).toBeVisible();
  expect(calls()).toBe(1);
  const analysis = page.getByRole('region', { name: 'Анализ возможных коллизий' });
  const collision = analysis.getByRole('group', { name: /^Возможные противоречия/ });
  await expandGroup(collision);
  await expandPair(collision);
  await expect(
    page.getByText('Для одной оплаты указаны взаимоисключающие способы.', { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText('Отсутствие найденных коллизий не доказывает согласованность требований.'),
  ).toBeVisible();
  await expect(analysis.getByText('left · Версия 1', { exact: true })).toBeVisible();
  await expect(analysis.getByText('right · Версия 2', { exact: true })).toBeVisible();
  await analysis.getByRole('link', { name: 'Перейти к требованию' }).first().click();
  await expect(page).toHaveURL(/#requirement-left$/);
});
test('failed analysis can be retried and stale no-collision result is qualified', async ({
  page,
}) => {
  let attempt = 0;
  const calls = await fixture(page, null, () =>
    ++attempt === 1
      ? complete({
          status: 'failed',
          outcome: 'insufficient',
          errorCode: 'GENERATOR_FAILED',
          findings: [],
        })
      : complete({
          outcome: 'no_collisions_found',
          stale: true,
          findings: [],
          pairs: [candidate('safe-0', 0, 'completed', 'compatible')],
        }),
  );
  await page.goto('/projects/demo/analysis');
  await page.getByRole('button', { name: 'Проверить требования', exact: true }).click();
  await expect(
    page.getByText('Анализ остановлен. Готовые результаты сохранены.', { exact: true }),
  ).toBeVisible();
  await page.getByRole('button', { name: 'Начать новый анализ', exact: true }).click();
  await expect.poll(calls).toBe(2);
  await expect(
    page.getByText('В проверенных парах коллизий не найдено', { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText(
      'Результат устарел: требования или профиль модели изменились. Повторите анализ.',
      { exact: true },
    ),
  ).toBeVisible();
  const compatible = analysisRegion(page).getByRole('group', {
    name: /^Противоречий не обнаружено/,
  });
  await expandGroup(compatible);
  const pair = await expandPair(compatible);
  await expect(pair.getByText('left · Версия 1', { exact: true })).toBeVisible();
  await expect(pair.getByText('right · Версия 2', { exact: true })).toBeVisible();
});

test('a late metadata response cannot replace an explicitly started run', async ({ page }) => {
  let releaseMetadata!: () => void, releaseStart!: () => void;
  const metadata = new Promise<void>((resolve) => (releaseMetadata = resolve));
  const start = new Promise<void>((resolve) => (releaseStart = resolve));
  let first = true;
  await page.route('**/api/events', (route) =>
    route.fulfill({ status: 200, contentType: 'text/event-stream', body: ': heartbeat\n\n' }),
  );
  await page.route('**/api/graphql', async (route) => {
    const { query, variables } = route.request().postDataJSON();
    let data: unknown;
    if (query.includes('startProjectAnalysis(')) {
      await start;
      data = { startProjectAnalysis: complete() };
    } else if (query.includes('analysisHistory(')) data = { analysisHistory: [] };
    else if (query.includes('analysisPairsConnection('))
      data = { analysisPairsConnection: pairConnection(complete(), variables) };
    else if (query.includes('analysisPair('))
      data = { analysisPair: pairDetail(complete(), variables.pairId) };
    else if (query.includes('analysisPairs('))
      data = { analysisPairs: pairPage(complete(), variables) };
    else if (
      query.includes('projectAnalysisSummary(') ||
      query.includes('analysisRunMetadata(') ||
      query.includes('projectAnalysis(')
    ) {
      if (first) {
        first = false;
        await metadata;
        data = { projectAnalysis: null };
      } else data = { projectAnalysis: complete() };
    } else if (query.includes('requirementIndex(')) data = { requirementIndex: [] };
    else if (query.includes('requirements(')) data = { requirements: [] };
    else
      data = {
        project: {
          id: 'demo',
          name: 'Metadata race',
          goal: 'G',
          description: '',
          createdAt: 'now',
          updatedAt: 'now',
        },
      };
    await route.fulfill({ json: { data } });
  });
  await page.goto('/projects/demo/analysis');
  await page.getByRole('button', { name: 'Проверить требования', exact: true }).click();
  await expect(
    page.getByRole('button', { name: 'Выполняется анализ…', exact: true }),
  ).toBeDisabled();
  await expect(page.getByText('Анализ выполняется', { exact: true })).toBeVisible();
  releaseStart();
  await expect(page.getByText('Возможная коллизия найдена', { exact: true })).toBeVisible();
  releaseMetadata();
  await page.evaluate(
    () =>
      new Promise<void>((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
      ),
  );
  await expect(page.getByText('Возможная коллизия найдена', { exact: true })).toBeVisible();
});

test('persisted running analysis refreshes status without starting another run', async ({
  page,
  context,
}) => {
  const live = await liveFixture(
    context,
    complete({ status: 'running', outcome: '', findings: [] }),
  );
  await page.goto('/projects/demo/analysis');
  await expect(page.getByText('Анализ выполняется', { exact: true })).toBeVisible();
  expect(live.stats().starts).toBe(0);
  live.set(complete());
  await page.getByRole('button', { name: 'Обновить статус', exact: true }).click();
  expect(live.stats().starts).toBe(0);
  await expect(page.getByText('Возможная коллизия найдена', { exact: true })).toBeVisible();
});

test('invalidation refreshes a completed result to stale without rerunning the model', async ({
  page,
}) => {
  let release!: () => void;
  const event = new Promise<void>((resolve) => (release = resolve));
  let current = complete();
  let calls = 0;
  await page.route('**/api/events', async (route) => {
    await event;
    await route.fulfill({
      status: 200,
      contentType: 'text/event-stream',
      body: 'id: changed\nevent: cache.invalidate\ndata: {"schemaVersion":1,"keys":["project:demo:requirements"]}\n\n',
    });
  });
  await page.route('**/api/graphql', async (route) => {
    const { query, variables } = route.request().postDataJSON();
    let data: unknown;
    if (query.includes('startProjectAnalysis(')) {
      calls++;
      data = { startProjectAnalysis: current };
    } else if (query.includes('analysisHistory('))
      data = { analysisHistory: current ? [current] : [] };
    else if (query.includes('analysisPairsConnection('))
      data = { analysisPairsConnection: pairConnection(current, variables) };
    else if (query.includes('analysisPair('))
      data = { analysisPair: pairDetail(current, variables.pairId) };
    else if (query.includes('analysisPairs('))
      data = { analysisPairs: pairPage(current, variables) };
    else if (
      query.includes('projectAnalysisSummary(') ||
      query.includes('analysisRunMetadata(') ||
      query.includes('projectAnalysis(')
    )
      data = { projectAnalysis: current };
    else if (query.includes('requirementIndex(')) data = { requirementIndex: [] };
    else if (query.includes('requirementsConnection('))
      data = { requirementsConnection: requirementConnection([], variables) };
    else if (query.includes('requirements(')) data = { requirements: [] };
    else
      data = {
        project: {
          id: 'demo',
          name: 'Source change',
          goal: 'G',
          description: '',
          createdAt: 'now',
          updatedAt: 'now',
        },
      };
    await route.fulfill({ json: { data } });
  });
  await page.goto('/projects/demo/analysis');
  await expect(page.getByText('Возможная коллизия найдена', { exact: true })).toBeVisible();
  current = complete({ stale: true });
  release();
  await expect(
    page.getByText(
      'Результат устарел: требования или профиль модели изменились. Повторите анализ.',
      { exact: true },
    ),
  ).toBeVisible();
  expect(calls).toBe(0);
});

test('incomplete index has a specific explanation and source links', async ({ page }) => {
  await fixture(
    page,
    complete({
      outcome: 'insufficient',
      reasonCode: 'INDEX_INCOMPLETE',
      missingRequirementIds: ['right'],
      findings: [],
    }),
  );
  await page.goto('/projects/demo/analysis');
  const analysis = page.getByRole('region', { name: 'Анализ возможных коллизий' });
  await expect(
    analysis.getByText(
      'Не все требования проиндексированы. Проиндексируйте актуальные версии требований и повторите анализ.',
      { exact: true },
    ),
  ).toBeVisible();
  const missing = analysis.getByRole('group', { name: 'Требуют индексации: 1' });
  await missing.getByRole('link', { name: 'right' }).click();
  await expect(page).toHaveURL(/#requirement-right$/);
});

test('model insufficient conditions is distinct from missing index', async ({ page }) => {
  await fixture(
    page,
    complete({
      outcome: 'insufficient',
      reasonCode: 'MODEL_INSUFFICIENT',
      missingRequirementIds: [],
      findings: [],
    }),
  );
  await page.goto('/projects/demo/analysis');
  const analysis = page.getByRole('region', { name: 'Анализ возможных коллизий' });
  await expect(
    analysis.getByText(
      'В требованиях недостаточно условий для сравнения. Уточните их с учётом объяснения модели.',
      { exact: true },
    ),
  ).toBeVisible();
  await expect(
    analysis.getByText('Не все требования проиндексированы.', { exact: false }),
  ).toHaveCount(0);
});

async function liveFixture(
  context: BrowserContext,
  initial: ReturnType<typeof complete> | null,
  onStart?: () => ReturnType<typeof complete>,
) {
  let current = initial,
    starts = 0,
    retries = 0,
    denied = false;
  const waiting = new Map<number, (body: string) => void>();
  const connections = new Map<Page, number>();
  let connection = 0;
  const operations: { name: string; variables: Record<string, unknown> }[] = [];
  await context.route('**/api/events', async (route) => {
    const id = ++connection;
    const page = route.request().frame().page();
    connections.set(page, (connections.get(page) || 0) + 1);
    const body = await new Promise<string>((resolve) => waiting.set(id, resolve));
    waiting.delete(id);
    await route.fulfill({ status: 200, contentType: 'text/event-stream', body }).catch(() => {});
  });
  await context.route('**/api/graphql', async (route) => {
    if (denied) {
      await route.fulfill({ status: 403, json: { error: 'forbidden' } });
      return;
    }
    const { query, variables } = route.request().postDataJSON();
    let data: unknown;
    if (query.includes('retryProjectAnalysis(')) {
      retries++;
      operations.push({ name: 'retry', variables });
      data = { retryProjectAnalysis: current };
    } else if (query.includes('startProjectAnalysis(')) {
      starts++;
      operations.push({ name: 'start', variables });
      if (onStart) current = onStart();
      data = { startProjectAnalysis: current };
    } else if (query.includes('analysisHistory('))
      data = { analysisHistory: current ? [current] : [] };
    else if (query.includes('analysisPairsConnection('))
      data = { analysisPairsConnection: pairConnection(current, variables) };
    else if (query.includes('analysisPair('))
      data = { analysisPair: pairDetail(current, variables.pairId) };
    else if (query.includes('analysisPairs('))
      data = { analysisPairs: pairPage(current, variables) };
    else if (
      query.includes('projectAnalysisSummary(') ||
      query.includes('analysisRunMetadata(') ||
      query.includes('projectAnalysis(')
    )
      data = { projectAnalysis: current };
    else if (query.includes('requirementIndex(')) data = { requirementIndex: [] };
    else if (query.includes('requirementsConnection('))
      data = {
        requirementsConnection: requirementConnection(
          sources.map((source) => ({
            ...source,
            projectId: 'demo',
            createdAt: 'now',
            updatedAt: 'now',
          })),
          variables,
        ),
      };
    else if (query.includes('requirements('))
      data = {
        requirements: sources.map((source) => ({
          ...source,
          projectId: 'demo',
          createdAt: 'now',
          updatedAt: 'now',
        })),
      };
    else
      data = {
        project: {
          id: 'demo',
          name: 'Потоковый анализ',
          goal: 'G',
          description: '',
          createdAt: 'now',
          updatedAt: 'now',
        },
      };
    await route.fulfill({ json: { data } });
  });
  return {
    set(value: ReturnType<typeof complete>) {
      current = value;
    },
    deny() {
      denied = true;
    },
    stats() {
      return { starts, retries, operations, waiting: waiting.size };
    },
    connectionsFor(page: Page) {
      return connections.get(page) || 0;
    },
    send(id: string, reset = false, keys = ['project:demo:analysis']) {
      const body = `id: ${id}\nevent: ${reset ? 'cache.reset' : 'cache.invalidate'}\ndata: ${JSON.stringify({ schemaVersion: 1, keys })}\n\n`;
      for (const resolve of waiting.values()) resolve(body);
      waiting.clear();
    },
  };
}

const analysisRegion = (page: Page) =>
  page.getByRole('region', { name: 'Анализ возможных коллизий' });

test('preparation stages do not invent a total and candidates expose numeric progress', async ({
  page,
  context,
}) => {
  const live = await liveFixture(context, null, () =>
    complete({
      status: 'running',
      stage: 'checking_index',
      totalPairs: null,
      processedPairs: 0,
      outcome: '',
      findings: [],
      pairs: [],
    }),
  );
  const initialStatus = page.waitForResponse(
    (response) =>
      response.url().endsWith('/api/graphql') &&
      /projectAnalysisSummary\s*\(|analysisRunMetadata\s*\(|projectAnalysis\s*\(/.test(
        response.request().postDataJSON().query,
      ),
  );
  await page.goto('/projects/demo/analysis');
  expect((await (await initialStatus).json()).data.projectAnalysis).toBeNull();
  await expect(
    page.getByRole('button', { name: 'Проверить требования', exact: true }),
  ).toBeEnabled();
  await page.getByRole('button', { name: 'Проверить требования', exact: true }).click();
  const area = analysisRegion(page);
  await expect(area.getByText('Проверяем актуальность индекса…', { exact: true })).toBeVisible();
  await expect(area.getByText(/^Обработано \d+ из/)).toHaveCount(0);
  await expect.poll(() => live.stats().waiting).toBeGreaterThan(0);
  live.set(
    complete({
      status: 'running',
      stage: 'finding_pairs',
      totalPairs: null,
      processedPairs: 0,
      outcome: '',
      findings: [],
      pairs: [],
    }),
  );
  live.send('finding');
  await expect(area.getByText('Ищем пары для сравнения…', { exact: true })).toBeVisible();
  await expect(area.getByText(/^Обработано \d+ из/)).toHaveCount(0);
  await expect.poll(() => live.stats().waiting).toBeGreaterThan(0);
  live.set(
    complete({
      status: 'running',
      stage: 'comparing',
      totalPairs: 2,
      processedPairs: 0,
      activePairId: 'one',
      outcome: '',
      findings: [],
      pairs: [candidate('one', 0, 'running'), candidate('two', 1, 'pending')],
    }),
  );
  live.send('candidates');
  await expect(area.getByText('Обработано 0 из 2', { exact: true })).toBeVisible();
  await expect(area.getByText('На проверке: 2', { exact: true })).toBeVisible();
  const pendingGroup = area.getByRole('group', { name: /^На проверке/ });
  await expandGroup(pendingGroup);
  await expect(
    pendingGroup
      .getByRole('article', { name: 'Пара 1' })
      .getByText('Проверяется сейчас', { exact: true }),
  ).toBeVisible();
  await expect(
    area.getByRole('button', { name: 'Начать новый анализ', exact: true }),
  ).toBeDisabled();
  expect(live.stats().starts).toBe(1);
});

test('all verdict groups have counts and collapse without changing source snapshots', async ({
  page,
}) => {
  await fixture(
    page,
    complete({
      totalPairs: 5,
      processedPairs: 4,
      findings: [],
      pairs: [
        candidate('collision', 0, 'completed', 'collision'),
        candidate('compatible', 1, 'completed', 'compatible'),
        candidate('insufficient', 2, 'completed', 'insufficient'),
        candidate('pending', 3, 'pending'),
        { ...candidate('failed', 4, 'failed'), errorCode: 'GENERATION_FAILED' },
      ],
    }),
  );
  await page.goto('/projects/demo/analysis');
  const area = analysisRegion(page);
  for (const text of [
    'Возможные противоречия: 1',
    'Противоречий не обнаружено: 1',
    'Недостаточно данных / нужно уточнить условия: 1',
    'На проверке: 1',
    'Ошибки обработки: 1',
  ])
    await expect(area.getByText(text, { exact: true })).toBeVisible();
  const compatible = area.getByRole('group', { name: /^Противоречий не обнаружено(?:: \d+)?$/ });
  await expandGroup(compatible);
  await expandPair(compatible);
  await expect(
    compatible.getByText('В этой паре противоречие не найдено', { exact: true }),
  ).toBeVisible();
  await expect(compatible.getByText('Не связаны требования', { exact: false })).toHaveCount(0);
  const collision = area.getByRole('group', { name: /^Возможные противоречия(?:: \d+)?$/ });
  await expandGroup(collision);
  await expandPair(collision);
  await expect(collision.getByText(sources[0].text, { exact: true })).toBeVisible();
  await collision.locator(':scope > summary').click();
  await expect(collision.getByRole('article')).not.toBeVisible();
  await collision.locator(':scope > summary').click();
  await expandPair(collision);
  await expect(collision.getByText('right · Версия 2', { exact: true })).toBeVisible();
  await collision.getByRole('link', { name: 'Перейти к требованию' }).last().click();
  await expect(page).toHaveURL(/#requirement-right$/);
});

test('pair completion moves cards in two clients and progress survives reload and reconnect reset', async ({
  context,
}) => {
  const initial = complete({
    status: 'running',
    stage: 'comparing',
    outcome: '',
    findings: [],
    totalPairs: 2,
    processedPairs: 0,
    activePairId: 'one',
    pairs: [candidate('one', 0, 'running'), candidate('two', 1, 'pending')],
  });
  const live = await liveFixture(context, initial);
  const first = await context.newPage(),
    second = await context.newPage();
  await Promise.all([
    first.goto('/projects/demo/analysis'),
    second.goto('/projects/demo/analysis'),
  ]);
  for (const page of [first, second])
    await expect(analysisRegion(page).getByText('На проверке: 2', { exact: true })).toBeVisible();
  await expect.poll(() => live.stats().waiting).toBeGreaterThanOrEqual(2);
  live.set(
    complete({
      status: 'running',
      stage: 'comparing',
      outcome: '',
      findings: [finding],
      totalPairs: 2,
      processedPairs: 1,
      activePairId: 'two',
      pairs: [candidate('one', 0, 'completed', 'collision'), candidate('two', 1, 'running')],
    }),
  );
  live.send('pair-one');
  for (const page of [first, second]) {
    const area = analysisRegion(page);
    await expect(area.getByText('Обработано 1 из 2', { exact: true })).toBeVisible();
    await expect(area.getByText('Возможные противоречия: 1', { exact: true })).toBeVisible();
    await expect(area.getByText('На проверке: 1', { exact: true })).toBeVisible();
    const pending = area.getByRole('group', { name: /^На проверке/ });
    await expandGroup(pending);
    await expect(
      area
        .getByRole('group', { name: /^На проверке(?:: \d+)?$/ })
        .getByRole('article', { name: 'Пара 1', exact: true }),
    ).toHaveCount(0);
  }
  const priorConnections = live.connectionsFor(second);
  await second.reload();
  await expect(
    analysisRegion(second).getByText('Обработано 1 из 2', { exact: true }),
  ).toBeVisible();
  await expect.poll(() => live.connectionsFor(second)).toBeGreaterThan(priorConnections);
  await expect.poll(() => live.stats().waiting).toBeGreaterThanOrEqual(2);
  live.set(
    complete({
      totalPairs: 2,
      processedPairs: 2,
      activePairId: '',
      pairs: [
        candidate('one', 0, 'completed', 'collision'),
        candidate('two', 1, 'completed', 'compatible'),
      ],
    }),
  );
  live.send('after-reconnect', true);
  for (const page of [first, second]) {
    const area = analysisRegion(page);
    await expect(area.getByText('Обработано 2 из 2', { exact: true })).toBeVisible();
    await expect(area.getByText('Противоречий не обнаружено: 1', { exact: true })).toBeVisible();
    await expect(area.getByText('На проверке: 0', { exact: true })).toBeVisible();
    for (const name of [/^Возможные противоречия/, /^Противоречий не обнаружено/]) {
      const group = area.getByRole('group', { name });
      await expandGroup(group);
    }
    await expect(area.getByRole('article')).toHaveCount(2);
  }
  expect(live.stats().starts).toBe(0);
});

test('failed pair retry preserves completed cards and targets the same analysis', async ({
  page,
  context,
}) => {
  const done = candidate('done', 0, 'completed', 'collision');
  const failed = { ...candidate('failed', 1, 'failed'), errorCode: 'TIMEOUT' };
  const initial = complete({
    status: 'failed',
    errorCode: 'TIMEOUT',
    totalPairs: 2,
    processedPairs: 2,
    pairs: [done, failed],
  });
  const live = await liveFixture(context, initial);
  await page.goto('/projects/demo/analysis');
  const area = analysisRegion(page);
  await expandGroup(area.getByRole('group', { name: /^Возможные противоречия/ }));
  await expandGroup(area.getByRole('group', { name: /^Ошибки обработки/ }));
  await expect(area.getByText('Возможные противоречия: 1', { exact: true })).toBeVisible();
  await expect(area.getByText('Ошибки обработки: 1', { exact: true })).toBeVisible();
  live.set(
    complete({
      status: 'running',
      stage: 'comparing',
      outcome: '',
      totalPairs: 2,
      processedPairs: 1,
      pairs: [done, candidate('failed', 1, 'running')],
      activePairId: 'failed',
    }),
  );
  await area.getByRole('button', { name: 'Повторить неудавшиеся пары', exact: true }).click();
  await expect(area.getByText('Возможные противоречия: 1', { exact: true })).toBeVisible();
  await expect(area.getByText('На проверке: 1', { exact: true })).toBeVisible();
  await expect(area.getByRole('article', { name: 'Пара 1', exact: true })).toHaveCount(1);
  expect(live.stats().starts).toBe(0);
  expect(live.stats().retries).toBe(1);
  expect(live.stats().operations[0]).toEqual({
    name: 'retry',
    variables: { projectId: 'demo', analysisId: 'run-1' },
  });
  await expect.poll(() => live.stats().waiting).toBeGreaterThan(0);
  live.set(
    complete({
      stale: true,
      status: 'failed',
      errorCode: 'SOURCE_CHANGED',
      totalPairs: 2,
      processedPairs: 2,
      pairs: [done, failed],
    }),
  );
  live.send('changed', false, ['project:demo:requirements']);
  await expect(
    area.getByText(
      'Результат устарел: требования или профиль модели изменились. Повторите анализ.',
      { exact: true },
    ),
  ).toBeVisible();
  await expect(
    area.getByRole('button', { name: 'Повторить неудавшиеся пары', exact: true }),
  ).toHaveCount(0);
});

test('large groups use global coverage and fetch bounded pair pages', async ({ page }) => {
  const pairs = Array.from({ length: 45 }, (_, index) =>
    candidate(`compatible-${index}`, index, 'completed', 'compatible'),
  );
  await fixture(
    page,
    complete({
      totalPairs: 4950,
      processedPairs: 45,
      reusedPairs: 30,
      newPairs: 15,
      completedPairs: 45,
      compatiblePairs: 45,
      pendingPairs: 4905,
      pairs,
    }),
  );
  const requests: Record<string, unknown>[] = [];
  page.on('request', (request) => {
    if (request.url().endsWith('/api/graphql')) {
      const payload = request.postDataJSON();
      if (payload.query.includes('analysisPairs(')) requests.push(payload.variables);
    }
  });
  await page.goto('/projects/demo/analysis');
  const area = analysisRegion(page);
  await expect(area.getByText('Обработано 45 из 4950', { exact: true })).toBeVisible();
  await expect(area.getByText('Противоречий не обнаружено: 45', { exact: true })).toBeVisible();
  await expect(area.getByText('На проверке: 4905', { exact: true })).toBeVisible();
  await expect(
    area.getByText(
      'Переиспользовано: 30; проверено заново: 15; ожидает: 4905; в работе: 0; ошибок: 0.',
      { exact: true },
    ),
  ).toBeVisible();
  const group = area.getByRole('group', { name: 'Противоречий не обнаружено: 45', exact: true });
  await expandGroup(group);
  await group.getByLabel('Режим списка').selectOption('pages');
  await expect(group.getByText('1–20 из 45', { exact: true })).toBeVisible();
  await group.getByRole('button', { name: 'Далее', exact: true }).click();
  await expect(group.getByText('21–40 из 45', { exact: true })).toBeVisible();
  await group.getByRole('button', { name: 'Далее', exact: true }).click();
  await expect(group.getByRole('button', { name: 'Далее', exact: true })).toBeDisabled();
  expect(requests.every((variables) => variables.limit === 20)).toBe(true);
  expect(
    requests.some((variables) => variables.offset === 40 && variables.verdict === 'compatible'),
  ).toBe(true);
});

test('closed analysis groups load independently only when expanded', async ({ page }) => {
  const compatible = candidate('compatible-one', 0, 'completed', 'compatible');
  const duplicate = candidate('duplicate-one', 1, 'completed', 'duplicate');
  await fixture(
    page,
    complete({
      totalPairs: 2,
      completedPairs: 2,
      compatiblePairs: 1,
      duplicatePairs: 1,
      pairs: [compatible, duplicate],
    }),
  );
  await page.addInitScript(() => {
    for (const key of ['collision', 'duplicate', 'compatible', 'insufficient', 'pending', 'failed'])
      sessionStorage.setItem(`group:analysis:demo:${key}`, 'false');
  });
  const requests: Record<string, unknown>[] = [];
  const summaries: string[] = [];
  page.on('request', (request) => {
    if (request.url().endsWith('/api/graphql')) {
      const payload = request.postDataJSON();
      if (payload.query.includes('ProjectAnalysis')) summaries.push(payload.query);
      if (payload.query.includes('analysisPairsConnection(')) requests.push(payload.variables);
    }
  });
  await page.goto('/projects/demo/analysis');
  const area = analysisRegion(page);
  const duplicateGroup = area.getByRole('group', { name: 'Дубли требований: 1', exact: true });
  const compatibleGroup = area.getByRole('group', {
    name: 'Противоречий не обнаружено: 1',
    exact: true,
  });
  await expect(duplicateGroup).toBeVisible();
  await expect(compatibleGroup).toBeVisible();
  await expect.poll(() => requests.length).toBe(0);
  expect(summaries.some((query) => /\bpairs\s*\{/.test(query))).toBe(false);
  expect(summaries.some((query) => /\bfindings\s*\{/.test(query))).toBe(false);
  await duplicateGroup.locator('summary').click();
  await expect.poll(() => requests.length).toBe(1);
  expect(requests[0]?.verdict).toBe('duplicate');
  await compatibleGroup.locator('summary').click();
  await expect.poll(() => requests.length).toBe(2);
  expect(requests[1]?.verdict).toBe('compatible');
});

test('history preserves the original generator profile and reused comparison provenance', async ({
  page,
}) => {
  const old = complete({
    id: 'old',
    createdAt: '2026-10-01',
    generatorProfile: 'old-profile',
    generatorProvenance: '{"model":"old-model","digest":"immutable-old"}',
    stale: true,
    pairs: [
      {
        ...candidate('old-pair', 0, 'completed', 'collision'),
        generatorProfile: 'old-profile',
        reusedFrom: 'original-check',
        checkedAt: '2026-10-01',
      },
    ],
  });
  const current = complete({
    id: 'new',
    createdAt: '2026-10-02',
    generatorProfile: 'new-profile',
    generatorProvenance: '{"model":"new-model","digest":"immutable-new"}',
    outcome: 'no_collisions_found',
    pairs: [candidate('new-pair', 0, 'completed', 'compatible')],
  });
  await page.route('**/api/events', (route) =>
    route.fulfill({ status: 200, contentType: 'text/event-stream', body: ': heartbeat\n\n' }),
  );
  await page.route('**/api/graphql', async (route) => {
    const { query, variables } = route.request().postDataJSON();
    let data: unknown;
    const selected = variables.analysisId === 'old' ? old : current;
    if (query.includes('analysisHistory(')) data = { analysisHistory: [current, old] };
    else if (query.includes('analysisPairsConnection('))
      data = {
        scope: { projectId: 'demo' },
        analysisPairsConnection: pairConnection(selected, variables),
      };
    else if (query.includes('analysisPair('))
      data = { analysisPair: pairDetail(selected, variables.pairId) };
    else if (query.includes('analysisPairs('))
      data = { scope: { projectId: 'demo' }, analysisPairs: pairPage(selected, variables) };
    else if (
      query.includes('projectAnalysisSummary(') ||
      query.includes('analysisRunMetadata(') ||
      query.includes('projectAnalysis(')
    )
      data = { projectAnalysis: selected };
    else
      data = {
        project: {
          id: 'demo',
          name: 'History',
          goal: 'G',
          description: '',
          createdAt: 'now',
          updatedAt: 'now',
        },
      };
    await route.fulfill({
      json: {
        data,
        extensions: { cacheTags: ['projects', 'project:demo', 'project:demo:requirements'] },
      },
    });
  });
  await page.goto('/projects/demo/analysis');
  const area = analysisRegion(page);
  await expect(
    area.getByText('В проверенных парах коллизий не найдено', { exact: true }),
  ).toBeVisible();
  await area.getByRole('combobox', { name: 'История запусков' }).selectOption('old');
  await expect(area.getByText('Возможная коллизия найдена', { exact: true })).toBeVisible();
  const collision = area.getByRole('group', { name: /^Возможные противоречия/ });
  await expandGroup(collision);
  const oldPair = await expandPair(collision);
  await expect(
    oldPair.getByText('Использован результат проверки original-check', { exact: true }),
  ).toBeVisible();
  await area.getByText('Профиль генератора и происхождение проверки', { exact: true }).click();
  await expect(area.locator('pre')).toHaveText(old.generatorProvenance);
  await area.getByRole('combobox', { name: 'История запусков' }).selectOption('');
  await expect(
    area.getByText('В проверенных парах коллизий не найдено', { exact: true }),
  ).toBeVisible();
  await expect(
    area.getByText('Использован результат проверки original-check', { exact: true }),
  ).toHaveCount(0);
});

test('selection defaults to related with configurable threshold and all pairs is explicit', async ({
  page,
}) => {
  const calls = await fixture(page);
  const starts: Record<string, unknown>[] = [];
  page.on('request', (request) => {
    if (request.url().endsWith('/api/graphql')) {
      const payload = request.postDataJSON();
      if (payload.query.includes('startProjectAnalysis(')) starts.push(payload.variables);
    }
  });
  await page.goto('/projects/demo/analysis');
  const area = analysisRegion(page);
  await expect(area.getByRole('combobox', { name: 'Режим анализа' })).toHaveValue('related');
  await expect(area.getByRole('spinbutton', { name: 'Порог семантической близости' })).toHaveValue(
    '0.65',
  );
  expect(calls()).toBe(0);
  await area.getByRole('spinbutton', { name: 'Порог семантической близости' }).fill('0.78');
  await area.getByRole('button', { name: 'Проверить требования', exact: true }).click();
  await expect(area.getByText('Возможная коллизия найдена', { exact: true })).toBeVisible();
  expect(starts[0]).toEqual({ projectId: 'demo', mode: 'related', threshold: 0.78 });
  await area.getByRole('combobox', { name: 'Режим анализа' }).selectOption('all');
  await expect(area.getByRole('spinbutton', { name: 'Порог семантической близости' })).toHaveCount(
    0,
  );
  await area.getByRole('button', { name: 'Начать новый анализ', exact: true }).click();
  await expect.poll(() => starts.length).toBe(2);
  expect(starts[1]).toEqual({ projectId: 'demo', mode: 'all', threshold: 0.78 });
});

test('duplicate group distinguishes deterministic equality from model suggestions and shows unselected coverage', async ({
  page,
}) => {
  const exact = {
    ...candidate('exact', 0, 'completed', 'duplicate'),
    resultOrigin: 'exact_text_v1',
    selectionReasons: ['exact_text'],
    similarityScore: null,
    explanation: 'analysis.exactDuplicateExplanation',
  };
  const semantic = {
    ...candidate('semantic', 1, 'completed', 'duplicate'),
    resultOrigin: 'model',
    explanation: 'Возможно, оба требования описывают одно обязательство.',
  };
  await fixture(
    page,
    complete({
      totalPairs: 2,
      possiblePairs: 4950,
      notSelectedPairs: 4948,
      processedPairs: 2,
      pairs: [exact, semantic],
    }),
  );
  await page.goto('/projects/demo/analysis');
  const area = analysisRegion(page),
    group = area.getByRole('group', { name: 'Дубли требований: 2', exact: true });
  await expandGroup(group);
  const exactCard = await expandPair(group, 0);
  const semanticCard = await expandPair(group, 1);
  await expect(group.getByRole('article')).toHaveCount(2);
  await expect(
    exactCard.getByText('Точное совпадение после нормализации пробелов и переносов', {
      exact: true,
    }),
  ).toBeVisible();
  await expect(
    semanticCard.getByText(
      'Возможный смысловой дубль — предложение модели для проверки человеком',
      {
        exact: true,
      },
    ),
  ).toBeVisible();
  await expect(
    exactCard.getByText('analysis.exactDuplicateExplanation', { exact: true }),
  ).toHaveCount(0);
  await expect(
    exactCard.getByText('Источник: алгоритм exact_text_v1; без вызова модели', { exact: true }),
  ).toBeVisible();
  await expect(
    semanticCard.getByText('Возможно, оба требования описывают одно обязательство.', {
      exact: true,
    }),
  ).toBeVisible();
  await expect(area.getByText('Всего возможных пар проекта: 4950', { exact: true })).toBeVisible();
  await expect(area.getByText('Отобрано в очередь: 2', { exact: true })).toBeVisible();
  await expect(
    area.getByText('Не отобрано и не проверено в этом запуске: 4948', { exact: true }),
  ).toBeVisible();
});

test('revoked access clears saved pair texts and history after invalidation', async ({
  page,
  context,
}) => {
  const live = await liveFixture(context, complete());
  await page.goto('/projects/demo/analysis');
  const collision = analysisRegion(page).getByRole('group', { name: /^Возможные противоречия/ });
  await expandGroup(collision);
  await expandPair(collision);
  await expect(analysisRegion(page).getByText(sources[0].text, { exact: true })).toBeVisible();
  await expect.poll(() => live.stats().waiting).toBeGreaterThan(0);
  live.deny();
  live.send('access-revoked', false, ['project:demo:requirements']);
  await expect(page.getByText(sources[0].text, { exact: true })).toHaveCount(0);
  await expect(page.getByText(sources[1].text, { exact: true })).toHaveCount(0);
  await expect(page.getByRole('combobox', { name: 'История запусков' })).toHaveCount(0);
});

test('an empty final history page keeps navigation back to earlier runs', async ({ page }) => {
  await fixture(page, complete());
  await page.route('**/api/graphql', async (route) => {
    const { query, variables } = route.request().postDataJSON();
    if (!query.includes('analysisHistory(')) {
      await route.fallback();
      return;
    }
    const history =
      variables.offset === 0
        ? Array.from({ length: 20 }, (_, index) => ({ ...complete(), id: `history-${index}` }))
        : [];
    await route.fulfill({ json: { data: { analysisHistory: history } } });
  });
  await page.goto('/projects/demo/analysis');
  const area = analysisRegion(page);
  await area.getByRole('button', { name: 'Далее', exact: true }).click();
  await expect(area.getByRole('button', { name: 'Далее', exact: true })).toBeDisabled();
  await expect(area.getByRole('button', { name: 'Назад', exact: true })).toBeEnabled();
  await area.getByRole('button', { name: 'Назад', exact: true }).click();
  await expect(
    area.getByRole('combobox', { name: 'История запусков' }).getByRole('option'),
  ).toHaveCount(21);
});

test('historical failures cannot retry work after a newer run exists', async ({ page }) => {
  const current = complete({
    id: 'latest',
    status: 'failed',
    errorCode: 'TIMEOUT',
    pairs: [{ ...candidate('failed', 0, 'failed'), errorCode: 'TIMEOUT' }],
  });
  const old = { ...current, id: 'old', createdAt: '2025-12-31T23:59:59.999Z' };
  await fixture(page, current);
  await page.route('**/api/graphql', async (route) => {
    const { query, variables } = route.request().postDataJSON();
    if (query.includes('analysisHistory(')) {
      await route.fulfill({ json: { data: { analysisHistory: [current, old] } } });
    } else if (
      (query.includes('projectAnalysis(') || query.includes('analysisRunMetadata(')) &&
      !query.includes('analysisPairs(')
    ) {
      await route.fulfill({
        json: { data: { projectAnalysis: variables.analysisId === 'old' ? old : current } },
      });
    } else await route.fallback();
  });
  await page.goto('/projects/demo/analysis');
  const area = analysisRegion(page);
  const retry = area.getByRole('button', { name: 'Повторить неудавшиеся пары', exact: true });
  await expect(retry).toBeVisible();
  await area.getByRole('combobox', { name: 'История запусков' }).selectOption('old');
  await expect(area.getByText('Ошибки обработки: 1', { exact: true })).toBeVisible();
  await expect(retry).toHaveCount(0);
  await area.getByRole('combobox', { name: 'История запусков' }).selectOption('');
  await expect(retry).toBeVisible();
});

test('cancel stops the whole run, preserves results and persists across reload', async ({
  page,
}) => {
  let release!: () => void;
  const pending = new Promise<void>((resolve) => {
    release = resolve;
  });
  const saved = candidate('saved', 0, 'completed', 'collision');
  const running = complete({
    status: 'running',
    outcome: '',
    totalPairs: 3,
    pairs: [saved, candidate('active', 1, 'running'), candidate('queued', 2, 'pending')],
  });
  const cancelled = complete({
    status: 'failed',
    errorCode: 'CANCELLED',
    outcome: '',
    totalPairs: 3,
    processedPairs: 1,
    pairs: [
      saved,
      ...['active', 'queued'].map((id, i) => ({
        ...candidate(id, i + 1, 'failed'),
        errorCode: 'CANCELLED',
      })),
    ],
  });
  const starts = await fixture(page, running, undefined, async () => {
    await pending;
    return cancelled;
  });
  await page.goto('/projects/demo/analysis');
  const cancel = page.getByRole('button', { name: 'Отменить анализ всех пар', exact: true });
  await cancel.click();
  await expect(cancel).toBeDisabled();
  release();
  await expect(page.getByText('Анализ отменён', { exact: true })).toBeVisible();
  await expect(cancel).toHaveCount(0);
  await expect(
    page.getByRole('button', { name: 'Начать новый анализ', exact: true }),
  ).toBeEnabled();
  await expect(
    page.getByRole('group', { name: 'Возможные противоречия: 1', exact: true }),
  ).toBeVisible();
  await page.reload();
  await expect(page.getByText('Анализ отменён', { exact: true })).toBeVisible();
  expect(starts()).toBe(0);
});

test('cancel failure leaves the running analysis available for another attempt', async ({
  page,
}) => {
  await fixture(page, complete({ status: 'running', outcome: '' }));
  await page.route('**/api/graphql', async (route) => {
    if (!route.request().postDataJSON().query.includes('cancelProjectAnalysis(')) {
      await route.fallback();
      return;
    }
    await route.fulfill({ json: { errors: [{ message: 'unavailable' }] } });
  });
  await page.goto('/projects/demo/analysis');
  const cancel = page.getByRole('button', { name: 'Отменить анализ всех пар', exact: true });
  await cancel.click();
  await expect(cancel).toBeEnabled();
  await expect(page.getByText('Анализ выполняется', { exact: true })).toBeVisible();
  await expect(page.getByText('Анализ отменён', { exact: true })).toHaveCount(0);
});
