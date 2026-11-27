import { readFile } from 'node:fs/promises';
import { expect, type Page, test } from '@playwright/test';
import type { ProcessGraphRunFieldsFragment } from '../lib/generated';

const nodes = [
  { id: 'check', kind: 'action', label: 'Проверить заявку' },
  { id: 'approve', kind: 'state', label: 'Принята' },
  { id: 'return', kind: 'state', label: 'Возвращена' },
  { id: 'operator', kind: 'actor', label: 'Оператор' },
].map((node) => ({ ...node, requirementId: null, version: null }));
const draft = {
  nodes,
  edges: [
    {
      from: 'check',
      to: 'approve',
      kind: 'leads_to',
      label: 'Ошибок нет',
      evidence: [
        {
          requirementId: 'req-1',
          version: 2,
          start: 24,
          end: 61,
          quote: 'При отсутствии ошибок принять заявку.',
        },
      ],
    },
    {
      from: 'check',
      to: 'return',
      kind: 'leads_to',
      label: 'Найдены ошибки',
      evidence: [
        {
          requirementId: 'req-2',
          version: 1,
          start: 0,
          end: 41,
          quote: 'При наличии ошибок вернуть заявку автору.',
        },
      ],
    },
    {
      from: 'operator',
      to: 'check',
      kind: 'needs',
      label: '',
      evidence: [
        {
          requirementId: 'req-3',
          version: 1,
          start: 0,
          end: 37,
          quote: 'Как оператор я хочу проверить заявку.',
        },
      ],
    },
  ],
  sourceCount: 3,
  partial: true,
};

type StreamWindow = Window & {
  graphPush: (frame: string) => void;
  graphClose: () => void;
  graphConnections: Array<string | null>;
};

async function mockStream(page: Page) {
  await page.addInitScript(() => {
    let active: ReadableStreamDefaultController<Uint8Array>;
    const target = window as unknown as StreamWindow;
    target.graphConnections = [];
    const original = window.fetch;
    window.fetch = (input, init) => {
      if (String(input) !== '/api/events') return original(input, init);
      target.graphConnections.push(new Headers(init?.headers).get('Last-Event-ID'));
      const body = new ReadableStream<Uint8Array>({
        start(controller) {
          active = controller;
        },
      });
      return Promise.resolve(
        new Response(body, { headers: { 'content-type': 'text/event-stream' } }),
      );
    };
    target.graphPush = (frame) => active.enqueue(new TextEncoder().encode(frame));
    target.graphClose = () => active.close();
  });
}

async function pushProgress(
  page: Page,
  { status, run }: ProcessGraphRunFieldsFragment,
  projectId = 'demo',
) {
  await page.evaluate(
    (payload) =>
      (window as unknown as StreamWindow).graphPush(
        `event: process_graph.progress
data: ${JSON.stringify(payload)}

`,
      ),
    {
      schemaVersion: 1,
      projectId,
      status,
      run: run?.mode === 'PROCESS' ? { ...run, mode: undefined } : run,
    },
  );
}

test('a graph continues in the background and restores progress and cited results', async ({
  page,
}) => {
  const anchors: unknown[] = [];
  const batches: unknown[] = [];
  const latestReads: unknown[] = [];
  const preparation = Promise.withResolvers<void>();
  let failNextRead = false;
  let failedReads = 0;
  let latestReadGate: ReturnType<typeof Promise.withResolvers<void>> | undefined;
  let latest: ProcessGraphRunFieldsFragment | null = null;
  function snapshot(
    id: string,
    topic: string,
    completed: number,
    total: number,
    failed: number,
  ): ProcessGraphRunFieldsFragment {
    const status = total === 0 ? 'planning' : completed === total ? 'suggested' : 'ready';
    return {
      status,
      run: {
        id,
        topic,
        mode: 'PROCESS',
        revision: total === 0 ? 1 : completed + 2,
        completed,
        total,
        failed,
      },
      draft: status === 'suggested' ? draft : null,
      fieldErrors: [],
    };
  }

  await mockStream(page);
  await page.route('**/api/graphql', async (route) => {
    const { operationName, variables } = route.request().postDataJSON();
    const pageInfo = { hasNextPage: false, endCursor: null };
    let data: unknown;
    switch (operationName) {
      case 'ProjectDetail':
        data = {
          project: { id: 'demo', name: 'Заявки', goal: '', description: '' },
          analysisDefaultThreshold: 0.7,
        };
        break;
      case 'GlossaryTermPages':
        data = {
          glossaryTermsConnection: {
            items: [
              {
                id: 'request-term',
                name: 'Заявка',
                definition: 'Запрос пользователя',
                context: '',
                variants: [],
                version: 1,
                sourceId: '',
                sourceVersion: 0,
                createdAt: '',
                updatedAt: '',
              },
            ],
            pageInfo,
          },
        };
        break;
      case 'StartBackgroundProcessGraph':
        anchors.push(variables.input);
        if (anchors.length === 1) {
          await preparation.promise;
          latest = snapshot('run-1', 'заявка', 0, 0, 0);
        } else {
          latest = snapshot('run-2', 'Заявка', 1, 1, 0);
        }
        data = { startBackgroundProcessGraph: latest };
        break;
      case 'LatestProcessGraphRun':
        latestReads.push(variables);
        if (failNextRead) {
          failNextRead = false;
          failedReads++;
          await route.fulfill({ status: 502 });
          return;
        }
        data = { latestProcessGraphRun: latest };
        if (latestReadGate) {
          const gate = latestReadGate;
          latestReadGate = undefined;
          await gate.promise;
        }
        break;
      case 'ProcessGraphBatch':
        batches.push(variables.input);
        data = {};
        break;
      default:
        data = { projectsConnection: { items: [], pageInfo } };
    }
    await route.fulfill({ json: { data } });
  });

  try {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto('/projects/demo/process-graphs');
    const keyword = page.getByRole('textbox', { name: 'Ключевое слово или фраза' });
    await keyword.fill('  заявка  ');
    const build = page.getByRole('button', { name: 'Построить граф процесса' });
    const heading = page.getByRole('heading', { name: 'Черновик графа процесса' });
    const status = page.locator('#process-graph-status');
    async function openOverview() {
      await page.getByRole('link', { name: 'Заявки', exact: true }).click();
      await expect(page).toHaveURL('/projects/demo');
      await expect(status).toHaveCount(0);
    }

    await build.click();
    await expect.poll(() => anchors.length).toBe(1);
    await expect(status).toContainText('Подбираем требования для графа');
    await openOverview();
    await page.goBack();
    await expect(status).toContainText('Подбираем требования для графа');
    await expect(keyword).toHaveValue('');
    preparation.resolve();
    const planning = 'Планируем порции для графа';
    await expect(status).toContainText(planning);
    await expect(status).toContainText('Тема графа — заявка');
    await expect(status).not.toContainText('Завершено');
    await keyword.fill('другая тема');
    await expect(build).toBeDisabled();

    await openOverview();
    await page.goBack();
    await expect(status).toContainText(planning);
    await expect(keyword).toHaveValue('');
    const firstBatch = snapshot('run-1', 'заявка', 0, 2, 0);
    const planningReads = latestReads.length;
    await pushProgress(page, firstBatch, 'other-project');
    await expect(status).toContainText(planning);
    await pushProgress(page, firstBatch);
    await expect(status).toContainText('Обрабатываем порцию 1 из 2');
    await expect(status).toContainText('Завершено 0 из 2');
    await expect(build).toBeDisabled();
    const buildHelp = page.getByRole('tooltip').filter({
      hasText: 'Дождитесь результата текущего запуска.',
    });
    await build.hover();
    await expect(buildHelp).toBeVisible();
    await page.mouse.move(0, 0);
    await expect(buildHelp).toBeHidden();
    await keyword.focus();
    await page.keyboard.press('Tab');
    await expect(build).toBeFocused();
    await expect(buildHelp).toBeVisible();
    await page.keyboard.press('Enter');
    expect(anchors).toHaveLength(1);
    await page.keyboard.press('Escape');
    await expect(buildHelp).toBeHidden();
    await expect(build).toBeFocused();
    await pushProgress(page, firstBatch);
    await pushProgress(page, snapshot('run-1', 'заявка', 0, 0, 0));
    await expect(status).toContainText('Обрабатываем порцию 1 из 2');
    await new Promise((resolve) => setTimeout(resolve, 2200));
    expect(latestReads).toHaveLength(planningReads);

    await openOverview();
    latest = snapshot('run-1', 'заявка', 1, 3, 0);
    const beforeReentry = latestReads.length;
    failNextRead = true;
    await page.goBack();
    await expect.poll(() => failedReads).toBe(1);
    await expect(status).toContainText('Обрабатываем порцию 2 из 3');
    expect(latestReads).toHaveLength(beforeReentry + 2);
    const beforeReconnect = latestReads.length;
    latest = snapshot('run-1', 'заявка', 2, 3, 1);
    const delayedReconnect = Promise.withResolvers<void>();
    latestReadGate = delayedReconnect;
    try {
      await page.evaluate(() => {
        const stream = window as unknown as StreamWindow;
        stream.graphPush(
          'id: 17\nevent: cache.invalidate\ndata: {"schemaVersion":1,"keys":["unrelated"]}\n\n',
        );
        stream.graphClose();
      });
      await expect.poll(() => latestReads.length).toBe(beforeReconnect + 1);
      expect(
        await page.evaluate(() => (window as unknown as StreamWindow).graphConnections),
      ).toEqual([null, '17']);

      await pushProgress(page, latest);
      await expect(status).toContainText('Обрабатываем порцию 3 из 3');
      await expect(status).toContainText('Завершено 2 из 3');
      await expect(status).toContainText('Порций с ошибками — 1');
      await expect(status).toContainText('Тема графа — заявка');
      await expect(keyword).toHaveValue('');
      await expect(heading).toHaveCount(0);

      latest = snapshot('run-1', 'заявка', 3, 3, 1);
      await pushProgress(page, latest);
      await pushProgress(page, latest);
      await expect(heading).toBeVisible();
      expect(latestReads).toHaveLength(beforeReconnect + 2);
    } finally {
      delayedReconnect.resolve();
    }
    await expect(heading).toBeVisible();
    await page.reload();
    await expect(heading).toBeVisible();
    await expect(status).toContainText('Завершено 3 из 3');
    await expect(status).toContainText('Порций с ошибками — 1');
    await expect(status).toContainText('Тема графа — заявка');
    await expect(page.getByText('Черновик охватывает часть доступных сведений.')).toBeVisible();

    const completedReads = latestReads.length;
    await openOverview();
    await pushProgress(page, latest);
    await pushProgress(page, snapshot('run-1', 'заявка', 2, 3, 1));
    await page.goBack();
    await expect(heading).toBeVisible();
    expect(latestReads).toHaveLength(completedReads);

    await page.evaluate(() => (window as unknown as StreamWindow).graphClose());
    await expect.poll(() => latestReads.length).toBe(completedReads + 1);
    await expect(heading).toBeVisible();

    await expect(page.getByText('Ошибок нет', { exact: true }).last()).toBeVisible();
    await expect(page.getByText('Найдены ошибки', { exact: true }).last()).toBeVisible();
    await expect(page.getByText('Нужно', { exact: true }).last()).toBeVisible();
    await page.getByText('Цитаты из требований', { exact: true }).first().click();
    await expect(page.getByText(draft.edges[0].evidence[0].quote)).toBeVisible();
    await expect(page.getByRole('link', { name: 'Требование req-1 · Версия 2' })).toHaveAttribute(
      'href',
      '/projects/demo/requirements?requirement=req-1&version=2&start=24&end=61#requirement-req-1',
    );
    const preview = page.getByText('Просмотр Mermaid', { exact: true });
    await expect(page.getByRole('region', { name: 'Диаграмма Mermaid' })).toHaveCount(0);
    await preview.press('Enter');
    const viewer = page.getByRole('region', { name: 'Диаграмма Mermaid' });
    const svg = viewer.locator('svg');
    await expect(svg.locator('text').filter({ hasText: /Проверить\s*заявку/ })).toBeVisible();
    await expect(svg.locator('a, foreignObject, image')).toHaveCount(0);

    const zoom = viewer.getByRole('button', { name: 'Увеличить' });
    const fit = viewer.getByRole('button', { name: 'Вписать в экран' });
    await zoom.press('Enter');
    await expect(fit).toBeEnabled();
    const viewport = viewer.getByLabel('Область диаграммы');
    const initialBox = await svg.boundingBox();
    await viewport.press('ArrowRight');
    expect((await svg.boundingBox())?.x).toBeLessThan(initialBox?.x ?? 0);
    const fullscreen = viewer.getByRole('button', { name: 'На весь экран' });
    await fullscreen.press('Enter');
    const dialog = page.getByRole('dialog', { name: 'Диаграмма Mermaid' });
    await expect(dialog).toBeVisible();
    await expect(
      dialog.getByRole('button', { name: 'Выйти из полноэкранного режима' }),
    ).toBeFocused();
    await page.keyboard.press('Escape');
    await expect(fullscreen).toBeFocused();
    await expect(fit).toBeEnabled();
    await fit.press('Enter');
    await expect(fit).toBeDisabled();

    await page.context().grantPermissions(['clipboard-read', 'clipboard-write']);
    const copyMenu = viewer.getByText('Копировать', { exact: true });
    await copyMenu.press('Enter');
    await viewer.getByRole('button', { name: 'Скопировать код Mermaid' }).press('Enter');
    await expect(viewer.getByText('Код Mermaid скопирован.')).toBeVisible();
    const copied = await page.evaluate(() => navigator.clipboard.readText());
    expect(copied).toContain('flowchart TD');
    expect(copied).toContain('n1["Проверить заявку"]');
    expect(copied).not.toContain('click ');

    await copyMenu.press('Enter');
    const copyImage = viewer.getByRole('button', { name: 'Скопировать диаграмму' });
    await copyImage.press('Enter');
    await expect(viewer.getByText('Изображение скопировано.')).toBeVisible();
    expect(await page.evaluate(async () => (await navigator.clipboard.read())[0]?.types)).toContain(
      'image/png',
    );

    await page.evaluate(() => {
      Object.defineProperty(navigator, 'clipboard', {
        configurable: true,
        value: { write: () => Promise.reject(new Error('Denied')) },
      });
    });
    await copyMenu.click();
    const imageReady = page.waitForEvent('download');
    await copyImage.click();
    const image = await imageReady;
    expect(image.suggestedFilename()).toBe('diagram.png');
    expect((await readFile(await image.path())).subarray(0, 8)).toEqual(
      Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]),
    );
    await expect(viewer.getByText('Изображение скачано.')).toBeVisible();
    await preview.click();

    const exportButton = page.getByRole('button', { name: 'Скачать Mermaid' });
    const downloadReady = page.waitForEvent('download');
    await exportButton.press('Enter');
    const download = await downloadReady;
    expect(download.suggestedFilename()).toBe('process-graph.mmd');
    const mermaid = await readFile(await download.path(), 'utf8');
    expect(mermaid).toContain('n1["Проверить заявку"]');
    expect(mermaid).toContain('n1 -->|"Ошибок нет"| n2');
    expect(mermaid).toContain('n4 -->|"Нужно"| n1');
    expect(mermaid).toContain(
      `click n2 href "${new URL(page.url()).origin}/projects/demo/requirements?requirement=req-1&version=2&start=24&end=61#requirement-req-1"`,
    );
    expect(anchors).toEqual([{ projectId: 'demo', keyword: 'заявка' }]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(
      390,
    );

    latest = { status: 'stale', run: null, draft: null, fieldErrors: [] };
    await page.evaluate(() =>
      (window as unknown as StreamWindow).graphPush(
        'id: 18\nevent: cache.invalidate\ndata: {"schemaVersion":1,"keys":["project:demo:requirements"]}\n\n',
      ),
    );
    await expect(heading).toHaveCount(0);
    await expect(exportButton).toHaveCount(0);
    await expect(status).toContainText('Данные проекта изменились.');

    await page.getByRole('radio', { name: 'По термину словаря' }).check();
    await page.getByRole('button', { name: 'Заявка', exact: true }).click();
    await build.click();
    await expect(heading).toBeVisible();
    await expect(status).toContainText('Завершено 1 из 1');
    await expect(status).not.toContainText('Порций с ошибками');
    expect(anchors[1]).toEqual({ projectId: 'demo', termId: 'request-term' });
    expect(latestReads.length).toBeGreaterThanOrEqual(4);
    expect(latestReads).toEqual(latestReads.map(() => ({ projectId: 'demo' })));
    expect(batches).toEqual([]);

    const beforeReadingReplacement = latestReads.length;
    await openOverview();
    await page.goBack();
    await expect(heading).toBeVisible();
    expect(latestReads).toHaveLength(beforeReadingReplacement + 1);

    const replacementReads = latestReads.length;
    await openOverview();
    await page.goBack();
    await expect(heading).toBeVisible();
    expect(latestReads).toHaveLength(replacementReads);

    await openOverview();
    latest = snapshot('run-3', 'новая заявка', 0, 0, 0);
    await pushProgress(page, latest);
    await page.goBack();
    await expect(heading).toHaveCount(0);
    await expect(status).toContainText('Тема графа — новая заявка');
    expect(latestReads).toHaveLength(replacementReads + 1);

    latest = snapshot('run-3', 'новая заявка', 2, 2, 0);
    await pushProgress(page, latest);
    await expect(heading).toBeVisible();
    const beforeClear = latestReads.length;
    latest = null;
    await page.evaluate(() =>
      (window as unknown as StreamWindow).graphPush(
        'id: 19\nevent: cache.invalidate\ndata: {"schemaVersion":1,"keys":["project:demo:process-graph"]}\n\n',
      ),
    );
    await expect(heading).toHaveCount(0);
    expect(latestReads).toHaveLength(beforeClear + 1);
  } finally {
    preparation.resolve();
    latestReadGate?.resolve();
  }
});

test('a requirement diagram restores its mode, cites versions, and exports its typed links', async ({
  page,
}) => {
  const source = draft.edges[0].evidence[0];
  const requirements: ProcessGraphRunFieldsFragment = {
    status: 'suggested',
    run: {
      id: 'run-requirements',
      topic: 'заявка',
      mode: 'REQUIREMENTS',
      revision: 2,
      completed: 1,
      total: 1,
      failed: 0,
    },
    draft: {
      nodes: [
        {
          id: 'general',
          kind: 'requirement',
          label: 'Регистрация заявки',
          requirementId: 'req-2',
          version: 1,
        },
        {
          id: 'specific',
          kind: 'requirement',
          label: 'Принятие заявки',
          requirementId: 'req-1',
          version: 2,
        },
      ],
      edges: [{ from: 'specific', to: 'general', kind: 'refines', label: '', evidence: [source] }],
      sourceCount: 2,
      partial: false,
    },
    fieldErrors: [],
  };
  const planning = {
    ...requirements,
    status: 'planning',
    run: requirements.run && { ...requirements.run, revision: 1, completed: 0, total: 0 },
    draft: null,
  };
  let latest: ProcessGraphRunFieldsFragment | null = null;
  const inputs: unknown[] = [];
  await mockStream(page);
  await page.route('**/api/graphql', async (route) => {
    const { operationName, variables } = route.request().postDataJSON();
    const pageInfo = { hasNextPage: false, endCursor: null };
    let data: unknown;
    switch (operationName) {
      case 'ProjectDetail':
        data = { project: { id: 'demo', name: 'Заявки', goal: '', description: '' } };
        break;
      case 'LatestProcessGraphRun':
        data = { latestProcessGraphRun: latest };
        break;
      case 'StartBackgroundProcessGraph':
        inputs.push(variables.input);
        latest = planning;
        data = { startBackgroundProcessGraph: latest };
        break;
      default:
        data = { projectsConnection: { items: [], pageInfo } };
    }
    await route.fulfill({ json: { data } });
  });

  await page.goto('/projects/demo/process-graphs');
  const flowchart = page.getByRole('radio', { name: 'Граф процесса (Flowchart)' });
  const requirementMode = page.getByRole('radio', {
    name: 'Диаграмма требований (requirementDiagram)',
  });
  await expect(
    page.getByRole('group', { name: 'Вид диаграммы' }).getByRole('radio').first(),
  ).toBeChecked();
  await expect(flowchart).toBeChecked();
  await requirementMode.check();
  await page.getByRole('textbox', { name: 'Ключевое слово или фраза' }).fill('заявка');
  const build = page.getByRole('button', { name: 'Построить диаграмму требований' });
  await build.click();
  await expect(build).toBeDisabled();
  await expect(requirementMode).toBeDisabled();
  await expect(page.locator('#process-graph-status')).toContainText('Планируем порции для графа');
  latest = requirements;
  await pushProgress(page, requirements);
  const heading = page.getByRole('heading', { name: 'Черновик диаграммы требований' });
  await expect(heading).toBeVisible();
  expect(inputs).toEqual([{ projectId: 'demo', keyword: 'заявка', mode: 'REQUIREMENTS' }]);
  await expect(page.getByText('Уточняет', { exact: true }).last()).toBeVisible();
  await expect(
    page.getByRole('link', { name: 'Требование req-2 · Версия 1', exact: true }),
  ).toHaveAttribute(
    'href',
    '/projects/demo/requirements?requirement=req-2&version=1#requirement-req-2',
  );
  await page.getByText('Цитаты из требований', { exact: true }).click();
  await expect(page.getByText(source.quote)).toBeVisible();
  const evidencePath =
    '/projects/demo/requirements?requirement=req-1&version=2&start=24&end=61#requirement-req-1';
  await expect(
    page.locator('details').getByRole('link', { name: 'Требование req-1 · Версия 2' }),
  ).toHaveAttribute('href', evidencePath);
  const diagramNode = page
    .getByRole('region', { name: 'Схема связей требований' })
    .getByRole('link', {
      name: 'Принятие заявки · Требование req-1 · Версия 2',
    });
  await expect(diagramNode).toHaveAttribute('href', evidencePath);

  await page.getByText('Просмотр Mermaid', { exact: true }).click();
  const viewer = page.getByRole('region', { name: 'Диаграмма Mermaid' });
  const mermaidNode = viewer.getByRole('button', {
    name: 'Принятие заявки · Требование req-1 · Версия 2',
  });
  await expect(mermaidNode).toBeVisible();
  await viewer.getByRole('button', { name: 'Увеличить' }).click();
  const svg = viewer.locator('svg');
  const darkRenderId = await svg.getAttribute('id');
  await page.getByRole('button', { name: 'Тёмная тема' }).click();
  await expect(svg).not.toHaveAttribute('id', darkRenderId ?? '');
  await expect(viewer.getByRole('button', { name: 'Вписать в экран' })).toBeEnabled();
  await page.getByLabel('Язык').selectOption('en');
  const englishViewer = page.getByRole('region', { name: 'Mermaid diagram' });
  await expect(englishViewer.getByRole('button', { name: 'Fit to screen' })).toBeEnabled();
  await page.getByLabel('Language').selectOption('ru');

  await flowchart.check();
  await expect(heading).toBeVisible();
  const diagramReady = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Скачать Mermaid' }).click();
  const diagram = await diagramReady;
  expect(diagram.suggestedFilename()).toBe('requirement-diagram.mmd');
  expect(await readFile(await diagram.path(), 'utf8')).toContain('r2 - refines -> r1');
  const evidenceReady = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Скачать Markdown с источниками' }).click();
  const evidence = await evidenceReady;
  expect(evidence.suggestedFilename()).toBe('requirement-diagram.md');
  const markdown = await readFile(await evidence.path(), 'utf8');
  expect(markdown).toContain('```mermaid\nrequirementDiagram');
  expect(markdown).toContain(`${new URL(page.url()).origin}${evidencePath}`);
  expect(markdown).toContain(source.quote);

  await page.getByRole('link', { name: 'Заявки', exact: true }).click();
  await expect(page).toHaveURL('/projects/demo');
  await page.goBack();
  await expect(heading).toBeVisible();
  await expect(requirementMode).toBeChecked();
  await page.getByText('Просмотр Mermaid', { exact: true }).click();
  await mermaidNode.press('Enter');
  await expect(page).toHaveURL(evidencePath);
});

async function mockSavedGraph(page: Page, readDraft: () => ProcessGraphRunFieldsFragment['draft']) {
  await mockStream(page);
  await page.route('**/api/graphql', async (route) => {
    const { operationName } = route.request().postDataJSON();
    const data: Record<string, unknown> = {
      ProjectDetail: { project: { id: 'demo', name: 'Заявки', goal: '', description: '' } },
      LatestProcessGraphRun: {
        latestProcessGraphRun: {
          status: 'suggested',
          run: {
            id: 'saved-graph',
            topic: 'заявка',
            mode: 'PROCESS',
            revision: 2,
            completed: 1,
            total: 1,
            failed: 0,
          },
          draft: readDraft(),
          fieldErrors: [],
        } satisfies ProcessGraphRunFieldsFragment,
      },
    };
    await route.fulfill({
      json: {
        data: data[operationName] ?? {
          projectsConnection: { items: [], pageInfo: { hasNextPage: false, endCursor: null } },
        },
      },
    });
  });
}

test('the preview reports an empty or oversized diagram without blocking the project page', async ({
  page,
}) => {
  let savedDraft: ProcessGraphRunFieldsFragment['draft'] = {
    ...draft,
    nodes: [{ ...draft.nodes[0], label: 'Строка '.repeat(12_000) }],
    edges: [],
  };
  await mockSavedGraph(page, () => savedDraft);
  await page.goto('/projects/demo/process-graphs');
  const toggle = page.getByText('Просмотр Mermaid', { exact: true });
  const viewer = page.getByRole('region', { name: 'Диаграмма Mermaid' });
  await toggle.press('Enter');
  await expect(
    viewer.getByText('Не удалось показать диаграмму. Проверьте её формат и размер.'),
  ).toBeVisible();
  await expect(viewer.locator('svg')).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Скачать Mermaid' })).toBeEnabled();

  savedDraft = { ...draft, nodes: [], edges: [], sourceCount: 0 };
  await page.reload();
  await toggle.press('Enter');
  await expect(viewer.getByText('В диаграмме нет узлов.')).toBeVisible();
  await expect(viewer.getByRole('button', { name: 'Увеличить' })).toBeDisabled();
});

test.describe('Mermaid on a touch screen', () => {
  test.use({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });

  test('touch pan follows successive movements and fit restores the initial view', async ({
    page,
  }) => {
    await mockSavedGraph(page, () => draft);
    await page.goto('/projects/demo/process-graphs');
    await page.getByText('Просмотр Mermaid', { exact: true }).tap();
    const viewport = page.getByRole('region', { name: 'Область диаграммы' });
    const svg = viewport.locator('svg');
    await expect(svg).toBeVisible();
    await viewport.scrollIntoViewIfNeeded();
    const frame = await viewport.boundingBox();
    const initial = await svg.boundingBox();
    if (!frame || !initial) throw new Error('Diagram bounds are missing');
    const x = Math.round(frame.x + frame.width / 4);
    const y = Math.round(frame.y + frame.height / 2);
    const client = await page.context().newCDPSession(page);

    await client.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x, y }] });
    await client.send('Input.dispatchTouchEvent', {
      type: 'touchMove',
      touchPoints: [{ x: x + 30, y }],
    });
    await expect.poll(async () => (await svg.boundingBox())?.x).toBeCloseTo(initial.x + 30, 0);
    await client.send('Input.dispatchTouchEvent', {
      type: 'touchMove',
      touchPoints: [{ x: x + 60, y }],
    });
    await client.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
    await expect.poll(async () => (await svg.boundingBox())?.x).toBeCloseTo(initial.x + 60, 0);

    await page.getByRole('button', { name: 'Вписать в экран' }).tap();
    await expect.poll(async () => (await svg.boundingBox())?.x).toBeCloseTo(initial.x, 0);
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(
      390,
    );
  });
});
