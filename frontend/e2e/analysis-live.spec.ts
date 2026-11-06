import { expect, type Page, test } from '@playwright/test';

type LayaSnapshot = {
  layaPendingPairs: number;
  layaRunningPairs: number;
  layaCompletedPairs: number;
  layaFailedPairs: number;
  classifierProfile: string;
};
type SnapshotOverrides = Partial<LayaSnapshot> & {
  runningPairs?: number;
  failedPairs?: number;
  completedPairs?: number;
  collisionPairs?: number;
};
function snapshot(revision = '1', processedPairs = 0, laya: SnapshotOverrides = {}) {
  return {
    revision,
    id: 'run-live',
    projectId: 'demo',
    createdAt: '2026-11-05T00:00:00Z',
    mode: 'all',
    similarityThreshold: 0.65,
    possiblePairs: 20,
    notSelectedPairs: 0,
    duplicatePairs: 0,
    reusedPairs: 0,
    newPairs: processedPairs,
    completedPairs: processedPairs,
    pendingPairs: 20 - processedPairs,
    runningPairs: 0,
    failedPairs: 0,
    collisionPairs: processedPairs,
    compatiblePairs: 0,
    insufficientPairs: 0,
    status: processedPairs === 20 ? 'completed' : 'running',
    outcome: processedPairs === 20 ? 'possible_collision' : '',
    errorCode: '',
    reasonCode: '',
    missingRequirementIds: [],
    completedAt: '',
    stale: false,
    stage: processedPairs === 20 ? 'finished' : 'comparing',
    totalPairs: 20,
    processedPairs,
    activePairId: '',
    layaPendingPairs: 0,
    layaRunningPairs: 0,
    layaCompletedPairs: 0,
    layaFailedPairs: 0,
    classifierProfile: '',
    ...laya,
  };
}
type StreamWindow = Window & { analysisPush: (frame: string) => void; analysisClose: () => void };
async function stream(page: Page) {
  await page.addInitScript(() => {
    let streamController: ReadableStreamDefaultController<Uint8Array>;
    const original = window.fetch;
    window.fetch = (input, init) => {
      if (String(input) !== '/api/events') return original(input, init);
      const body = new ReadableStream<Uint8Array>({
        start(controller) {
          streamController = controller;
        },
      });
      return Promise.resolve(
        new Response(body, { headers: { 'content-type': 'text/event-stream' } }),
      );
    };
    const target = window as unknown as StreamWindow;
    target.analysisPush = (frame) => streamController.enqueue(new TextEncoder().encode(frame));
    target.analysisClose = () => streamController.close();
  });
}
async function push(page: Page, value: ReturnType<typeof snapshot>) {
  await page.evaluate(
    (value) =>
      (window as unknown as StreamWindow).analysisPush(
        `id: event-${value.revision}\nevent: analysis.progress\ndata: ${JSON.stringify({
          schemaVersion: 1,
          projectId: value.projectId,
          runId: value.id,
          revision: value.revision,
          snapshot: value,
        })}\n\n`,
      ),
    value,
  );
}
async function fixture(page: Page, firstSummaryGate?: Promise<void>) {
  await stream(page);
  let current = snapshot();
  const requests: string[] = [];
  let pair = {
    id: 'pair-1',
    position: 0,
    status: 'completed',
    verdict: 'collision',
    explanation: 'Confirmed explanation',
    errorCode: '',
    checkedAt: '2026-11-05',
    resultOrigin: 'model',
    reusedFrom: '',
    selectionReasons: [],
    similarityScore: null,
    generatorProfile: 'g',
    layaStatus: '',
    preclassification: null as string | null,
    layaCheckedAt: null as string | null,
    left: { id: 'a', version: 1, text: 'Saved left source' },
    right: { id: 'b', version: 1, text: 'Saved right source' },
  };
  await page.route('**/api/graphql', async (route) => {
    const { query } = route.request().postDataJSON();
    requests.push(query.match(/(?:query|mutation) (\w+)/)?.[1] || 'unknown');
    let data: unknown;
    if (query.includes('analysisPairsConnection('))
      data = {
        scope: { projectId: 'demo' },
        analysisPairsConnection: {
          edges: current.processedPairs ? [{ cursor: 'cursor', node: pair }] : [],
          pageInfo: { endCursor: 'cursor', hasNextPage: false },
          totalCount: current.processedPairs,
        },
      };
    else if (query.includes('analysisPair(')) data = { analysisPair: pair };
    else if (query.includes('analysisRunMetadata('))
      data = {
        projectAnalysis: {
          id: current.id,
          projectId: 'demo',
          generatorProfile: 'g',
          generatorProvenance: '{"model":"fixed"}',
          embeddingProfile: 'e',
          selectionProfile: 'all',
        },
      };
    else if (query.includes('projectAnalysisSummary(')) {
      const responseSnapshot = current;
      if (requests.filter((name) => name === 'ProjectAnalysisSummary').length === 1)
        await firstSummaryGate;
      data = { projectAnalysis: responseSnapshot };
    } else if (query.includes('analysisHistory(')) data = { analysisHistory: [current] };
    else
      data = {
        project: {
          id: 'demo',
          name: 'Live',
          goal: 'G',
          description: '',
          createdAt: 'now',
          updatedAt: 'now',
        },
      };
    await route.fulfill({ json: { data, extensions: { cacheTags: ['project:demo:analysis'] } } });
  });
  return {
    requests,
    set(value: ReturnType<typeof snapshot>) {
      current = value;
    },
    setPair(value: typeof pair) {
      pair = value;
    },
  };
}
test('compact push updates twenty pairs without GraphQL progress refetch and preserves form focus', async ({
  page,
}) => {
  const live = await fixture(page);
  await page.goto('/projects/demo/analysis');
  await expect(page.getByText('Обработано 0 из 20', { exact: true })).toBeVisible();
  const requests = live.requests.length;
  const input = page.getByRole('combobox', { name: 'Режим анализа' });
  await input.focus();
  live.set(snapshot('21', 20));
  for (let n = 1; n <= 20; n++) await push(page, snapshot(String(n + 1), n));
  await expect(page.getByText('Обработано 20 из 20', { exact: true })).toBeVisible();
  await expect(input).toBeFocused();
  expect(live.requests.length).toBe(requests);
  await push(page, snapshot('3', 2));
  await expect(page.getByText('Обработано 20 из 20', { exact: true })).toBeVisible();
  expect(live.requests.filter((name) => name === 'ProjectDetail')).toHaveLength(1);
});
test('group loads compact rows and only an opened pair fetches texts; reopen reuses details', async ({
  page,
}) => {
  const live = await fixture(page);
  live.set(snapshot('2', 1));
  await page.goto('/projects/demo/analysis');
  const group = page.getByRole('group', { name: 'Возможные противоречия: 1', exact: true });
  await expect(group).toBeVisible();
  expect(live.requests).not.toContain('AnalysisPairConnection');
  await group.locator(':scope > summary').click();
  await expect(group.getByRole('article')).toHaveCount(1);
  expect(live.requests).not.toContain('AnalysisPairDetail');
  const details = group.getByRole('group', { name: 'Тексты и объяснение' });
  await details.locator('summary').click();
  await expect(details.getByText('Confirmed explanation', { exact: true })).toBeVisible();
  await details.locator('summary').click();
  await details.locator('summary').click();
  await expect(details.getByText('Saved left source', { exact: true })).toBeVisible();
  expect(live.requests.filter((name) => name === 'AnalysisPairDetail')).toHaveLength(1);
  const oldCount = live.requests.length;
  await push(page, snapshot('3', 1));
  await expect(group).toHaveAttribute('open', '');
  expect(live.requests.length).toBe(oldCount);
});
test('Laya classification pools advance over SSE while final verdict groups remain intact', async ({
  page,
}) => {
  const live = await fixture(page);
  live.set(
    snapshot('1', 0, {
      layaPendingPairs: 3,
      layaRunningPairs: 1,
      layaCompletedPairs: 2,
      runningPairs: 1,
    }),
  );
  await page.goto('/projects/demo/analysis');
  await expect(page.getByText('Ещё не классифицировано: 4')).toBeVisible();
  await expect(page.getByText('Активно в Laya: 1')).toBeVisible();
  await expect(page.getByText('Активно в модели сравнения: 1')).toBeVisible();

  live.set(
    snapshot('2', 0, {
      layaPendingPairs: 1,
      layaRunningPairs: 1,
      layaCompletedPairs: 4,
      runningPairs: 2,
    }),
  );
  await push(
    page,
    snapshot('2', 0, {
      layaPendingPairs: 1,
      layaRunningPairs: 1,
      layaCompletedPairs: 4,
      runningPairs: 2,
    }),
  );
  await expect(page.getByText('Ещё не классифицировано: 2')).toBeVisible();
  await expect(
    page.getByText('Классифицировано, ожидает или проходит проверку моделью сравнения: 4'),
  ).toBeVisible();
  await expect(page.getByText('Активно в Laya: 1')).toBeVisible();
  await expect(page.getByText('Активно в модели сравнения: 2')).toBeVisible();

  live.set(
    snapshot('3', 1, {
      layaPendingPairs: 1,
      layaCompletedPairs: 4,
      runningPairs: 1,
    }),
  );
  live.setPair({
    id: 'pair-1',
    position: 0,
    status: 'completed',
    verdict: 'collision',
    explanation: 'Confirmed explanation',
    errorCode: '',
    checkedAt: '2026-11-05',
    resultOrigin: 'model',
    reusedFrom: '',
    selectionReasons: [],
    similarityScore: null,
    generatorProfile: 'g',
    layaStatus: 'completed',
    preclassification:
      '{"decision":"related","scores":{"related":0.8,"unrelated":0.1,"uncertain":0.1}}',
    layaCheckedAt: '2026-11-06',
    left: { id: 'a', version: 1, text: 'Saved left source' },
    right: { id: 'b', version: 1, text: 'Saved right source' },
  });
  await push(
    page,
    snapshot('3', 1, {
      layaPendingPairs: 1,
      layaCompletedPairs: 4,
      runningPairs: 1,
    }),
  );
  await expect(page.getByText('Завершено обоими этапами: 1')).toBeVisible();
  const group = page.getByRole('group', { name: 'Возможные противоречия: 1', exact: true });
  await expect(group).toBeVisible();
  await group.locator(':scope > summary').click();
  const details = group.getByRole('group', { name: 'Тексты и объяснение' });
  await details.locator('summary').click();
  await expect(details.getByText('Предложение классификатора Laya')).toBeVisible();
  await expect(details.getByText('Предложение: связаны')).toBeVisible();
  await expect(details.getByText(/не вероятность правильности/)).toBeVisible();
  await expect(page.getByText('Возможная коллизия в паре')).toBeVisible();
});
test('oversized Laya input explains the local classifier budget on the failed pair', async ({
  page,
}) => {
  const live = await fixture(page);
  live.set(snapshot('2', 1, { failedPairs: 1, completedPairs: 0, collisionPairs: 0 }));
  live.setPair({
    id: 'pair-1',
    position: 0,
    status: 'failed',
    verdict: '',
    explanation: '',
    errorCode: 'LAYA_INPUT_TOO_LONG',
    checkedAt: '',
    resultOrigin: '',
    reusedFrom: '',
    selectionReasons: [],
    similarityScore: null,
    generatorProfile: 'g',
    layaStatus: '',
    preclassification: null,
    layaCheckedAt: null,
    left: { id: 'a', version: 1, text: 'Saved left source' },
    right: { id: 'b', version: 1, text: 'Saved right source' },
  });
  await page.goto('/projects/demo/analysis');
  const group = page.getByRole('group', { name: 'Ошибки обработки: 1', exact: true });
  await expect(group).toBeVisible();
  await group.locator(':scope > summary').click();
  await expect(group.getByText(/превышает лимит локального классификатора/i)).toBeVisible();
});
test('reset recovers a missed terminal snapshot without hiding the current result', async ({
  page,
}) => {
  const live = await fixture(page);
  await page.goto('/projects/demo/analysis');
  await expect(page.getByText('Обработано 0 из 20', { exact: true })).toBeVisible();
  live.set(snapshot('30', 20));
  await page.evaluate(() =>
    (window as unknown as StreamWindow).analysisPush(
      'event: cache.reset\ndata: {"schemaVersion":1,"keys":[]}\n\n',
    ),
  );
  await expect(page.getByText('Обработано 20 из 20', { exact: true })).toBeVisible();
  expect(live.requests.filter((name) => name === 'ProjectAnalysisSummary')).toHaveLength(2);
});

test('immutable provenance is requested only on expansion and reused after progress', async ({
  page,
}) => {
  const live = await fixture(page);
  await page.goto('/projects/demo/analysis');
  await expect(page.getByText('Обработано 0 из 20', { exact: true })).toBeVisible();
  expect(live.requests).not.toContain('AnalysisRunMetadata');
  const provenance = page.getByRole('group', {
    name: 'Профиль генератора и происхождение проверки',
    exact: true,
  });
  await provenance.locator(':scope > summary').click();
  await expect(provenance.locator('pre')).toHaveText('{"model":"fixed"}');
  live.set(snapshot('2', 1));
  await push(page, snapshot('2', 1));
  await expect(page.getByText('Обработано 1 из 20', { exact: true })).toBeVisible();
  await provenance.locator(':scope > summary').click();
  await provenance.locator(':scope > summary').click();
  await expect(provenance.locator('pre')).toHaveText('{"model":"fixed"}');
  expect(live.requests.filter((name) => name === 'AnalysisRunMetadata')).toHaveLength(1);
});
test('late slow snapshot cannot overwrite newer push and only one catch-up query is needed', async ({
  page,
}) => {
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  const live = await fixture(page, gate);
  await page.goto('/projects/demo/analysis');
  await expect.poll(() => live.requests.includes('ProjectAnalysisSummary')).toBe(true);
  live.set(snapshot('21', 20));
  await push(page, snapshot('21', 20));
  await expect(page.getByText('Обработано 20 из 20', { exact: true })).toBeVisible();
  release();
  await expect
    .poll(() => live.requests.filter((name) => name === 'ProjectAnalysisSummary').length)
    .toBe(2);
  await expect(page.getByText('Обработано 20 из 20', { exact: true })).toBeVisible();
});

test('buffered newer pair progress preserves known source staleness', async ({ page }) => {
  const live = await fixture(page);
  live.set({ ...snapshot('5'), stale: true });
  await page.goto('/projects/demo/analysis');
  const warning = page.getByText(
    'Результат устарел: требования или профиль модели изменились. Повторите анализ.',
    { exact: true },
  );
  await expect(warning).toBeVisible();
  await push(page, snapshot('6', 1));
  await expect(page.getByText('Обработано 1 из 20', { exact: true })).toBeVisible();
  await expect(warning).toBeVisible();
});
