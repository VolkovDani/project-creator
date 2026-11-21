import { expect, test } from '@playwright/test';

function connection<T extends { id: string }>(rows: T[], variables: Record<string, unknown>) {
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

test('index retry and search are explicit and retain versioned sources', async ({ page }) => {
  let indexCalls = 0,
    searchCalls = 0;
  let status = 'pending';
  const text = 'Пользователь может сохранить черновик требования.';
  await page.route('**/api/events', (route) =>
    route.fulfill({ status: 200, contentType: 'text/event-stream', body: ': heartbeat\n\n' }),
  );
  await page.route('**/api/graphql', async (route) => {
    const { query, variables } = route.request().postDataJSON();
    let data: unknown;
    if (query.includes('indexRequirement(')) {
      indexCalls++;
      status = 'failed';
      data = {
        indexRequirement: {
          requirementId: 'source-1',
          version: 2,
          status,
          profile: 'test-profile',
          errorCode: 'EMBEDDING_FAILED',
        },
      };
    } else if (query.includes('analysisHistory(')) data = { analysisHistory: [] };
    else if (query.includes('projectAnalysis(')) data = { projectAnalysis: null };
    else if (query.includes('requirementIndex(') || query.includes('requirementIndexByIds('))
      data = {
        [query.includes('requirementIndexByIds(') ? 'requirementIndexByIds' : 'requirementIndex']: [
          {
            requirementId: 'source-1',
            version: 2,
            status,
            profile: 'test-profile',
            errorCode: status === 'failed' ? 'EMBEDDING_FAILED' : '',
          },
        ],
      };
    else if (query.includes('searchRequirements(')) {
      searchCalls++;
      data = {
        searchRequirements: [
          {
            requirementId: 'source-1',
            projectId: 'demo',
            text: 'сохранить черновик',
            sourceText: text,
            version: 2,
            start: 23,
            end: 41,
            score: 0.91,
          },
        ],
      };
    } else if (query.includes('requirementsConnection('))
      data = {
        requirementsConnection: connection(
          [
            {
              id: 'source-1',
              projectId: 'demo',
              text,
              version: 2,
              createdAt: 'now',
              updatedAt: 'now',
            },
          ],
          variables,
        ),
      };
    else if (query.includes('requirements('))
      data = {
        requirements: [
          {
            id: 'source-1',
            projectId: 'demo',
            text,
            version: 2,
            createdAt: 'now',
            updatedAt: 'now',
          },
        ],
      };
    else
      data = {
        project: {
          id: 'demo',
          name: 'Проект поиска',
          goal: 'Проверить поиск',
          description: '',
          createdAt: 'now',
          updatedAt: 'now',
        },
      };
    await route.fulfill({ json: { data } });
  });
  await page.goto('/projects/demo/indexing');
  await expect(page.getByText('Индекс: ожидает индексации')).toBeVisible();
  expect(indexCalls).toBe(0);
  expect(searchCalls).toBe(0);
  await page.getByRole('button', { name: 'Индексировать', exact: true }).click();
  await expect(
    page.getByText('Не удалось проиндексировать требование. Можно повторить попытку.'),
  ).toBeVisible();
  expect(indexCalls).toBe(1);
  await page.getByRole('button', { name: 'Повторить индексацию', exact: true }).click();
  await expect.poll(() => indexCalls).toBe(2);
  await page.goto('/projects/demo/search');
  await expect(page.getByRole('button', { name: 'Новое понятие', exact: true })).toHaveCount(0);
  const editor = page.getByRole('dialog');
  await page.getByLabel('Поисковый запрос').fill('черновик');
  await page.getByRole('button', { name: 'Найти требования' }).click();
  await expect(page.getByText('сохранить черновик', { exact: true })).toBeVisible();
  await page.getByText('сохранить черновик', { exact: true }).evaluate((el) => {
    const node = el.firstChild;
    if (node) window.getSelection()?.setBaseAndExtent(node, 10, node, 18);
  });
  await page.getByRole('button', { name: 'Добавить выделение в словарь' }).click();
  await expect(editor.getByLabel('Название понятия')).toHaveValue('черновик');
  await page.keyboard.press('Escape');
  expect(searchCalls).toBe(1);
  await page.getByText('Исходный текст этой версии', { exact: true }).click();
  await expect(page.getByText(text, { exact: true })).toHaveCount(1);
  await page.getByRole('link', { name: 'Перейти к требованию' }).click();
  await expect(page).toHaveURL(/#requirement-source-1$/);
});
test('search links focus a requirement from the next cursor batch', async ({ page }) => {
  const requirements = Array.from({ length: 41 }, (_, index) => ({
    id: `source-${index}`,
    projectId: 'demo',
    text: `Требование ${index}`,
    version: 1,
    createdAt: 'now',
    updatedAt: 'now',
  }));
  await page.route('**/api/events', (route) =>
    route.fulfill({ status: 200, contentType: 'text/event-stream', body: ': heartbeat\n\n' }),
  );
  await page.route('**/api/graphql', async (route) => {
    const { query, variables } = route.request().postDataJSON();
    let data: unknown;
    if (query.includes('requirementIndex(')) data = { requirementIndex: [] };
    else if (query.includes('searchRequirements('))
      data = {
        searchRequirements: [
          {
            requirementId: 'source-40',
            projectId: 'demo',
            text: 'Требование 40',
            sourceText: 'Требование 40',
            version: 1,
            start: 0,
            end: 13,
            score: 1,
          },
        ],
      };
    else if (query.includes('requirementsConnection('))
      data = { requirementsConnection: connection(requirements, variables) };
    else
      data = {
        project: {
          id: 'demo',
          name: 'Источники требований',
          goal: 'G',
          description: '',
          createdAt: 'now',
          updatedAt: 'now',
        },
      };
    await route.fulfill({ json: { data } });
  });
  await page.goto('/projects/demo/search');
  await page.getByLabel('Поисковый запрос').fill('40');
  await page.getByRole('button', { name: 'Найти требования' }).click();
  await page.getByRole('link', { name: 'Перейти к требованию' }).click();
  await expect(page.locator('#requirement-source-40')).toBeInViewport();
  await expect(page.locator('#requirement-source-40')).toBeFocused();
  await expect(page).toHaveURL(/#requirement-source-40$/);
});

test('navigation away cancels a delayed source scan', async ({ page }) => {
  const requirements = Array.from({ length: 41 }, (_, index) => ({
    id: `source-${index}`,
    projectId: 'demo',
    text: `Требование ${index}`,
    version: 1,
    createdAt: 'now',
    updatedAt: 'now',
  }));
  let release!: () => void;
  const pending = new Promise<void>((resolve) => {
    release = resolve;
  });
  let started!: () => void;
  const scanStarted = new Promise<void>((resolve) => {
    started = resolve;
  });
  await page.route('**/api/events', (route) =>
    route.fulfill({ status: 200, contentType: 'text/event-stream', body: ': heartbeat\n\n' }),
  );
  await page.route('**/api/graphql', async (route) => {
    const { query, variables } = route.request().postDataJSON();
    let data: unknown;
    if (query.includes('requirementIndex(')) data = { requirementIndex: [] };
    else if (query.includes('requirementsConnection(')) {
      if (variables.after === 'cursor-40') {
        started();
        await pending;
      }
      data = { requirementsConnection: connection(requirements, variables) };
    } else
      data = {
        project: {
          id: 'demo',
          name: 'Выбор источника',
          goal: 'G',
          description: '',
          createdAt: 'now',
          updatedAt: 'now',
        },
      };
    await route.fulfill({ json: { data } }).catch(() => {});
  });
  await page.goto('/projects/demo/requirements?requirement=source-40#requirement-source-40');
  await scanStarted;
  await page.getByRole('link', { name: 'Поиск', exact: true }).click();
  await expect(page).toHaveURL(/\/search$/);
  release();
  await expect(page.getByRole('heading', { name: 'Поиск по смыслу' })).toBeVisible();
  await expect(page).toHaveURL(/\/search$/);
  await expect(page.locator('#requirement-source-40')).toHaveCount(0);
});

test('requirement draft is locked until its save response completes', async ({ page }) => {
  let release!: () => void;
  const pending = new Promise<void>((resolve) => (release = resolve));
  let started!: () => void;
  const sent = new Promise<void>((resolve) => (started = resolve));
  await page.route('**/api/events', (route) =>
    route.fulfill({ status: 200, contentType: 'text/event-stream', body: ': heartbeat\n\n' }),
  );
  await page.route('**/api/graphql', async (route) => {
    const { query, variables } = route.request().postDataJSON();
    let data: unknown;
    if (query.includes('createRequirement(')) {
      started();
      await pending;
      data = {
        createRequirement: { requirement: { id: 'r', text: 'Draft', version: 1 }, fieldErrors: [] },
      };
    } else if (query.includes('analysisHistory(')) data = { analysisHistory: [] };
    else if (query.includes('projectAnalysis(')) data = { projectAnalysis: null };
    else if (query.includes('requirementIndex(')) data = { requirementIndex: [] };
    else if (query.includes('requirementsConnection('))
      data = { requirementsConnection: connection([], variables) };
    else if (query.includes('requirements(')) data = { requirements: [] };
    else
      data = {
        project: {
          id: 'p',
          name: 'Pending save',
          goal: 'G',
          description: '',
          createdAt: 'now',
          updatedAt: 'now',
        },
      };
    await route.fulfill({ json: { data } });
  });
  await page.goto('/projects/p/requirements');
  await page.getByLabel('Текст требования').fill('Draft');
  await page.getByRole('button', { name: 'Добавить требование', exact: true }).click();
  await sent;
  try {
    await expect(page.getByLabel('Текст требования')).toBeDisabled();
  } finally {
    release();
  }
});
