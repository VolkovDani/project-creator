import { expect, test } from '@playwright/test';
import type { ProcessGraphRunFieldsFragment } from '../lib/generated';

const nodes = [
  { id: 'check', kind: 'action', label: 'Проверить заявку' },
  { id: 'approve', kind: 'state', label: 'Принята' },
  { id: 'return', kind: 'state', label: 'Возвращена' },
  { id: 'operator', kind: 'actor', label: 'Оператор' },
];
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

test('a graph continues in the background and restores progress and cited results', async ({
  page,
}) => {
  const anchors: unknown[] = [];
  const batches: unknown[] = [];
  const latestReads: unknown[] = [];
  const preparation = Promise.withResolvers<void>();
  const invalidation = Promise.withResolvers<void>();
  let failNextRead = false;
  let failedReads = 0;
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
      run: { id, topic, revision: total === 0 ? 1 : completed + 2, completed, total, failed },
      draft: status === 'suggested' ? draft : null,
      fieldErrors: [],
    };
  }

  await page.route('**/api/events', async (route) => {
    await invalidation.promise;
    const body =
      'id: 17\nevent: cache.invalidate\ndata: {"schemaVersion":1,"keys":["project:demo:requirements"]}\n\n';
    // A reload can disconnect an earlier stream. The client deduplicates the replay.
    await route.fulfill({ contentType: 'text/event-stream', body }).catch(() => {});
  });
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
    latest = snapshot('run-1', 'заявка', 0, 2, 0);
    await expect(status).toContainText('Обрабатываем порцию 1 из 2');
    await expect(status).toContainText('Завершено 0 из 2');
    await expect(build).toBeDisabled();

    await openOverview();
    latest = snapshot('run-1', 'заявка', 2, 3, 1);
    failNextRead = true;
    await page.goBack();
    await expect.poll(() => failedReads).toBe(1);
    await expect(status).toContainText('Обрабатываем порцию 3 из 3');
    await expect(status).toContainText('Завершено 2 из 3');
    await expect(status).toContainText('Порций с ошибками — 1');
    await expect(status).toContainText('Тема графа — заявка');
    await expect(keyword).toHaveValue('');
    await expect(heading).toHaveCount(0);

    latest = snapshot('run-1', 'заявка', 3, 3, 1);
    await expect(heading).toBeVisible();
    await page.reload();
    await expect(heading).toBeVisible();
    await expect(status).toContainText('Завершено 3 из 3');
    await expect(status).toContainText('Порций с ошибками — 1');
    await expect(status).toContainText('Тема графа — заявка');
    await expect(page.getByText('Черновик охватывает часть доступных сведений.')).toBeVisible();
    await expect(page.getByText('Ошибок нет', { exact: true }).last()).toBeVisible();
    await expect(page.getByText('Найдены ошибки', { exact: true }).last()).toBeVisible();
    await expect(page.getByText('Нужно', { exact: true }).last()).toBeVisible();
    await page.getByText('Цитаты из требований', { exact: true }).first().click();
    await expect(page.getByText(draft.edges[0].evidence[0].quote)).toBeVisible();
    await expect(page.getByRole('link', { name: 'Требование req-1 · Версия 2' })).toHaveAttribute(
      'href',
      '/projects/demo/requirements?requirement=req-1&version=2&start=24&end=61#requirement-req-1',
    );
    expect(anchors).toEqual([{ projectId: 'demo', keyword: 'заявка' }]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(
      390,
    );

    latest = { status: 'stale', run: null, draft: null, fieldErrors: [] };
    invalidation.resolve();
    await expect(heading).toHaveCount(0);
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
  } finally {
    preparation.resolve();
    invalidation.resolve();
  }
});
