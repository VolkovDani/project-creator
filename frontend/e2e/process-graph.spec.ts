import { expect, test } from '@playwright/test';

const nodes = [
  { id: 'check', kind: 'action', label: 'Проверить заявку' },
  { id: 'approve', kind: 'state', label: 'Принята' },
  { id: 'return', kind: 'state', label: 'Возвращена' },
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
  ],
  sourceCount: 2,
  partial: true,
};

test('a keyword or glossary term builds cited branches and source changes invalidate them', async ({
  page,
}) => {
  const inputs: unknown[] = [];
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
      case 'BuildProcessGraph':
        inputs.push(variables.input);
        data = { buildProcessGraph: { status: 'suggested', draft, fieldErrors: [] } };
        break;
      default:
        data = { projectsConnection: { items: [], pageInfo } };
    }
    await route.fulfill({ json: { data } });
  });

  try {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto('/projects/demo/process-graphs');
    await page.getByRole('textbox', { name: 'Ключевое слово или фраза' }).fill('  заявка  ');
    const build = page.getByRole('button', { name: 'Построить граф процесса' });
    const heading = page.getByRole('heading', { name: 'Черновик графа процесса' });
    await build.click();
    await expect(heading).toBeVisible();
    await expect(page.getByText('Черновик охватывает часть доступных сведений.')).toBeVisible();
    await expect(page.getByText('Ошибок нет', { exact: true }).last()).toBeVisible();
    await expect(page.getByText('Найдены ошибки', { exact: true }).last()).toBeVisible();
    await page.getByText('Цитаты из требований', { exact: true }).first().click();
    await expect(page.getByText(draft.edges[0].evidence[0].quote)).toBeVisible();
    await expect(page.getByRole('link', { name: 'Требование req-1 · Версия 2' })).toHaveAttribute(
      'href',
      '/projects/demo/requirements?requirement=req-1&version=2&start=24&end=61#requirement-req-1',
    );
    expect(inputs[0]).toEqual({ projectId: 'demo', keyword: 'заявка' });
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
    expect(inputs[1]).toEqual({ projectId: 'demo', termId: 'request-term' });
  } finally {
    invalidation.resolve();
  }
});
