import { expect, test } from '@playwright/test';
import type { ProcessGraphRunFieldsFragment } from '../lib/generated';

type BatchOutcome = Omit<ProcessGraphRunFieldsFragment, 'fieldErrors' | 'draft'> & {
  draft?: ProcessGraphRunFieldsFragment['draft'];
};

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

test('a keyword or glossary term builds cited branches and source changes invalidate them', async ({
  page,
}) => {
  const anchors: unknown[] = [];
  const batches: unknown[] = [];
  const preparation = Promise.withResolvers<void>();
  const split = Promise.withResolvers<void>();
  const failure = Promise.withResolvers<void>();
  const lastBatch = Promise.withResolvers<void>();
  const cancelledBatch = Promise.withResolvers<void>();
  const released = Promise.withResolvers<void>();
  const invalidation = Promise.withResolvers<void>();
  let streams = 0;
  await page.route('**/api/events', async (route) => {
    const firstStream = streams++ === 0;
    if (firstStream) await invalidation.promise;
    const body = firstStream
      ? 'id: 17\nevent: cache.invalidate\ndata: {"schemaVersion":1,"keys":["project:demo:requirements"]}\n\n'
      : ': heartbeat\n\n';
    await route.fulfill({ contentType: 'text/event-stream', body });
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
      case 'StartProcessGraph': {
        anchors.push(variables.input);
        const id = `run-${anchors.length}`;
        if (id === 'run-1') await preparation.promise;
        data = {
          startProcessGraph: {
            status: 'ready',
            run: { id, revision: 1, completed: 0, total: id === 'run-1' ? 2 : 1, failed: 0 },
            draft: null,
            fieldErrors: [],
          },
        };
        break;
      }
      case 'ProcessGraphBatch': {
        batches.push(variables.input);
        const { runId: id, revision } = variables.input;
        let outcome: BatchOutcome;
        if (id === 'run-1' && revision === 1) {
          await split.promise;
          outcome = {
            status: 'ready',
            run: { id, revision: 2, completed: 0, total: 3, failed: 0 },
          };
        } else if (id === 'run-1' && revision === 2) {
          await failure.promise;
          outcome = {
            status: 'ready',
            run: { id, revision: 3, completed: 1, total: 3, failed: 1 },
          };
        } else if (id === 'run-1' && revision === 3) {
          outcome = {
            status: 'ready',
            run: { id, revision: 4, completed: 2, total: 3, failed: 1 },
          };
        } else if (id === 'run-3' && revision === 1) {
          await cancelledBatch.promise;
          outcome = {
            status: 'ready',
            run: { id, revision: 2, completed: 0, total: 2, failed: 0 },
          };
        } else {
          const total = id === 'run-1' ? 3 : 1;
          if (id === 'run-1') await lastBatch.promise;
          outcome = {
            status: 'suggested',
            run: {
              id,
              revision: revision + 1,
              completed: total,
              total,
              failed: id === 'run-1' ? 1 : 0,
            },
            draft,
          };
        }
        data = { processGraphBatch: { draft: null, fieldErrors: [], ...outcome } };
        if (id === 'run-3' && revision === 1) {
          await route.fulfill({ json: { data } }).catch(() => {});
          released.resolve();
          return;
        }
        break;
      }
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
    await build.click();
    await expect(status).toContainText('Подбираем требования для графа');
    preparation.resolve();
    await expect(status).toContainText('Обрабатываем порцию 1 из 2');
    await expect(status).toContainText('Завершено 0 из 2');
    await expect(build).toBeDisabled();
    split.resolve();
    await expect(status).toContainText('Обрабатываем порцию 1 из 3');
    failure.resolve();
    await expect(status).toContainText('Обрабатываем порцию 3 из 3');
    await expect(status).toContainText('Завершено 2 из 3');
    await expect(status).toContainText('Порций с ошибками — 1');
    await expect(heading).toHaveCount(0);
    lastBatch.resolve();
    await expect(heading).toBeVisible();
    await expect(status).toContainText('Завершено 3 из 3');
    await expect(status).toContainText('Порций с ошибками — 1');
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
    expect(anchors[0]).toEqual({ projectId: 'demo', keyword: 'заявка' });
    expect(batches).toEqual(
      [1, 2, 3, 4].map((revision) => ({ projectId: 'demo', runId: 'run-1', revision })),
    );
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(
      390,
    );

    invalidation.resolve();
    await expect(heading).toHaveCount(0);
    await expect(page.getByText('Данные проекта изменились.', { exact: false })).toBeVisible();

    await page.getByRole('radio', { name: 'По термину словаря' }).check();
    await page.getByRole('button', { name: 'Заявка', exact: true }).click();
    await build.click();
    await expect(heading).toBeVisible();
    await expect(status).toContainText('Завершено 1 из 1');
    await expect(status).not.toContainText('Порций с ошибками');
    expect(anchors[1]).toEqual({ projectId: 'demo', termId: 'request-term' });
    expect(batches[4]).toEqual({ projectId: 'demo', runId: 'run-2', revision: 1 });

    await build.click();
    await expect.poll(() => batches.length).toBe(6);
    const aborted = page.waitForEvent('requestfailed', {
      predicate: (request) =>
        request.url().endsWith('/api/graphql') &&
        request.postDataJSON()?.variables?.input?.runId === 'run-3',
    });
    await page.getByRole('radio', { name: 'По ключевому слову' }).check();
    await aborted;
    await keyword.fill('другая тема');
    await build.click();
    await expect(heading).toBeVisible();
    cancelledBatch.resolve();
    await released.promise;
    await expect(status).toContainText('Завершено 1 из 1');
    expect(batches.slice(5)).toEqual([
      { projectId: 'demo', runId: 'run-3', revision: 1 },
      { projectId: 'demo', runId: 'run-4', revision: 1 },
    ]);
  } finally {
    preparation.resolve();
    split.resolve();
    failure.resolve();
    lastBatch.resolve();
    cancelledBatch.resolve();
    invalidation.resolve();
  }
});
