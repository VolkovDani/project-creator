import { type BrowserContext, expect, type Page, test } from '@playwright/test';

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
    status: 'completed',
    outcome: 'possible_collision',
    errorCode: '',
    reasonCode: '',
    missingRequirementIds: [],
    createdAt: 'now',
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
async function fixture(page: Page, initial: unknown = null, onStart?: () => unknown) {
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
    } else if (query.includes('analysisHistory('))
      data = { analysisHistory: latest ? [latest] : [] };
    else if (query.includes('analysisPairs('))
      data = { analysisPairs: pairPage(latest, variables) };
    else if (query.includes('projectAnalysis(')) data = { projectAnalysis: latest };
    else if (query.includes('requirementIndex(')) data = { requirementIndex: [] };
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
test('analysis runs only after explicit action and shows versioned sources', async ({ page }) => {
  const calls = await fixture(page);
  await page.goto('/projects/demo/analysis');
  await expect(page.getByRole('heading', { name: 'Анализ возможных коллизий' })).toBeVisible();
  expect(calls()).toBe(0);
  await page.getByRole('button', { name: 'Проверить требования', exact: true }).click();
  await expect(page.getByText('Возможная коллизия найдена', { exact: true })).toBeVisible();
  expect(calls()).toBe(1);
  await expect(
    page.getByText('Для одной оплаты указаны взаимоисключающие способы.', { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText('Отсутствие найденных коллизий не доказывает согласованность требований.'),
  ).toBeVisible();
  const analysis = page.getByRole('region', { name: 'Анализ возможных коллизий' });
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
      : complete({ outcome: 'no_collisions_found', stale: true, findings: [] }),
  );
  await page.goto('/projects/demo/analysis');
  await page.getByRole('button', { name: 'Проверить требования', exact: true }).click();
  await expect(
    page.getByText('Анализ остановлен. Готовые результаты сохранены.', { exact: true }),
  ).toBeVisible();
  await page.getByRole('button', { name: 'Начать новый анализ', exact: true }).click();
  expect(calls()).toBe(2);
  await expect(
    page.getByText('В проверенных парах коллизий не найдено', { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText(
      'Результат устарел: требования или профиль модели изменились. Повторите анализ.',
      { exact: true },
    ),
  ).toBeVisible();
  await expect(
    page.getByText('Сохранённые версии источников на момент анализа', { exact: true }),
  ).toBeVisible();
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
    else if (query.includes('analysisPairs('))
      data = { analysisPairs: pairPage(complete(), variables) };
    else if (query.includes('projectAnalysis(')) {
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
      body: 'id: changed\nevent: cache.invalidate\ndata: {"schemaVersion":1,"keys":["project:demo"]}\n\n',
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
    else if (query.includes('analysisPairs('))
      data = { analysisPairs: pairPage(current, variables) };
    else if (query.includes('projectAnalysis(')) data = { projectAnalysis: current };
    else if (query.includes('requirementIndex(')) data = { requirementIndex: [] };
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
    else if (query.includes('analysisPairs('))
      data = { analysisPairs: pairPage(current, variables) };
    else if (query.includes('projectAnalysis(')) data = { projectAnalysis: current };
    else if (query.includes('requirementIndex(')) data = { requirementIndex: [] };
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
    send(id: string, reset = false) {
      const body = `id: ${id}\nevent: ${reset ? 'cache.reset' : 'cache.invalidate'}\ndata: ${JSON.stringify({ schemaVersion: 1, keys: ['project:demo'] })}\n\n`;
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
      response.request().postDataJSON().query.includes('projectAnalysis('),
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
  await expect(
    area.getByRole('article', { name: 'Пара 1' }).getByText('Проверяется сейчас', { exact: true }),
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
  await expect(
    compatible.getByText('В этой паре противоречие не найдено', { exact: true }),
  ).toBeVisible();
  await expect(compatible.getByText('Не связаны требования', { exact: false })).toHaveCount(0);
  const collision = area.getByRole('group', { name: /^Возможные противоречия(?:: \d+)?$/ });
  await expect(collision.getByText(sources[0].text, { exact: true })).toBeVisible();
  await collision.locator('summary').click();
  await expect(collision.getByRole('article')).not.toBeVisible();
  await collision.locator('summary').click();
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
  live.send('changed');
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
  await expect(group.getByRole('article')).toHaveCount(20);
  await expect(group.getByText('1–20 из 45', { exact: true })).toBeVisible();
  await group.getByRole('button', { name: 'Далее', exact: true }).click();
  await expect(group.getByRole('article')).toHaveCount(20);
  await expect(group.getByRole('article', { name: 'Пара 21', exact: true })).toBeVisible();
  await expect(group.getByText('21–40 из 45', { exact: true })).toBeVisible();
  await group.getByRole('button', { name: 'Далее', exact: true }).click();
  await expect(group.getByRole('article')).toHaveCount(5);
  await expect(group.getByRole('button', { name: 'Далее', exact: true })).toBeDisabled();
  expect(requests.every((variables) => variables.limit === 20)).toBe(true);
  expect(
    requests.some((variables) => variables.offset === 40 && variables.verdict === 'compatible'),
  ).toBe(true);
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
    else if (query.includes('analysisPairs('))
      data = { scope: { projectId: 'demo' }, analysisPairs: pairPage(selected, variables) };
    else if (query.includes('projectAnalysis(')) data = { projectAnalysis: selected };
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
  await expect(
    area.getByText('Использован результат проверки original-check', { exact: true }),
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
  await expect(group.getByRole('article')).toHaveCount(2);
  await expect(
    group.getByText('Точное совпадение после нормализации пробелов и переносов', { exact: true }),
  ).toBeVisible();
  await expect(
    group.getByText('Возможный смысловой дубль — предложение модели для проверки человеком', {
      exact: true,
    }),
  ).toBeVisible();
  await expect(group.getByText('analysis.exactDuplicateExplanation', { exact: true })).toHaveCount(
    0,
  );
  await expect(
    group.getByText('Источник: алгоритм exact_text_v1; без вызова модели', { exact: true }),
  ).toBeVisible();
  await expect(
    group.getByText('Возможно, оба требования описывают одно обязательство.', { exact: true }),
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
  await expect(analysisRegion(page).getByText(sources[0].text, { exact: true })).toBeVisible();
  await expect.poll(() => live.stats().waiting).toBeGreaterThan(0);
  live.deny();
  live.send('access-revoked');
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
