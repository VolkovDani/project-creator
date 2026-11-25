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

test('search result groups align and literal hits show the original match', async ({ page }) => {
  const matchedText = 'Согласовать [этап 2].';
  const hit = {
    id: 'source-1',
    text:
      'Введение к длинному требованию. '.repeat(12) +
      `Перед запуском. ${matchedText} После проверки.`,
    version: 1,
    archivedAt: null,
  };
  await page.route('**/api/events', (route) =>
    route.fulfill({ status: 200, contentType: 'text/event-stream', body: ': heartbeat\n\n' }),
  );
  await page.route('**/api/graphql', (route) => {
    const { operationName, variables } = route.request().postDataJSON();
    const project = { id: 'demo', name: 'Поиск требований', goal: 'G', description: '' };
    const data =
      operationName === 'SearchRequirements'
        ? {
            searchRequirements: [
              {
                requirementId: hit.id,
                projectId: 'demo',
                text: hit.text.slice(0, 25),
                sourceText: hit.text,
                version: hit.version,
                start: 0,
                end: 25,
                score: 0.9,
              },
            ],
          }
        : operationName === 'ProjectDetail'
          ? { project }
          : {
              requirementsConnection: connection(
                operationName === 'RequirementMatches' ? [hit] : [],
                variables,
              ),
            };
    return route.fulfill({ json: { data } });
  });
  await page.goto('/projects/demo/requirements');
  const search = page.getByRole('searchbox', { name: 'Поисковый запрос' });
  const literal = page.getByRole('region', { name: 'Совпадения по ID и тексту' });
  const semantic = page.getByRole('region', { name: 'Поиск по смыслу' });
  const result = literal.getByRole('article');
  await search.fill('согласовать [ЭТАП 2].');
  await search.press('Enter');
  await expect(result.locator('p mark')).toHaveText(matchedText);
  await expect(result).toContainText(`Перед запуском. ${matchedText} После проверки.`);
  await expect(result.locator('[data-glossary-text]')).not.toHaveText(hit.text);
  const countLabel = 'Результаты поиска: 1';
  const literalCount = literal.getByText(countLabel, { exact: true });
  const semanticCount = semantic.getByText(countLabel, { exact: true });
  const semanticCard = semantic.getByRole('article');
  await expect(literalCount).toBeVisible();
  await expect(semanticCount).toBeVisible();
  await expect(semanticCard).toBeVisible();
  expect((await literalCount.boundingBox())?.y).toBe((await semanticCount.boundingBox())?.y);
  expect((await result.boundingBox())?.y).toBe((await semanticCard.boundingBox())?.y);
  await expect(result.getByRole('link', { name: 'Перейти к требованию' })).toHaveAttribute(
    'href',
    '/projects/demo/requirements?requirement=source-1#requirement-source-1',
  );

  await search.fill('SOURCE-1');
  await search.press('Enter');
  await expect(result.locator('code mark')).toHaveText(hit.id);
  await expect(result.locator('[data-glossary-text]')).toContainText(matchedText);
});

test('requirements search submits both queries and keeps text matches when semantic search fails', async ({
  page,
}) => {
  const matches = Array.from({ length: 41 }, (_, index) => ({
    id: `source-${index}`,
    projectId: 'demo',
    text: `Черновик ${index}`,
    version: 2,
    createdAt: '2026-11-22T12:00:00Z',
    updatedAt: '2026-11-22T12:00:00Z',
    archivedAt: index === 40 ? '2026-11-23T12:00:00Z' : null,
  }));
  const literalQueries: Array<{ match: string; after: string | null }> = [];
  const semanticQueries: string[] = [];
  let literalStarted!: () => void;
  const literalRequested = new Promise<void>((resolve) => {
    literalStarted = resolve;
  });
  await page.route('**/api/events', (route) =>
    route.fulfill({ status: 200, contentType: 'text/event-stream', body: ': heartbeat\n\n' }),
  );
  await page.route('**/api/graphql', async (route) => {
    const { query, variables } = route.request().postDataJSON();
    let data: unknown;
    if (query.includes('query RequirementMatches')) {
      literalQueries.push({ match: variables.match, after: variables.after });
      literalStarted();
      data = { requirementsConnection: connection(matches, variables) };
    } else if (query.includes('searchRequirements(')) {
      semanticQueries.push(variables.text);
      await literalRequested;
      await route.fulfill({ json: { errors: [{ message: 'search unavailable' }] } });
      return;
    } else if (query.includes('requirementsConnection('))
      data = {
        requirementsConnection: connection(
          query.includes('query ArchivedRequirements')
            ? matches.filter((match) => match.archivedAt)
            : [],
          variables,
        ),
      };
    else if (query.includes('requirementIndexByIds(')) data = { requirementIndexByIds: [] };
    else
      data = {
        project: {
          id: 'demo',
          name: 'Текстовый поиск',
          goal: 'G',
          description: '',
          createdAt: 'now',
          updatedAt: 'now',
        },
      };
    await route.fulfill({ json: { data } });
  });

  await page.goto('/projects/demo/requirements');
  const search = page.getByLabel('Поисковый запрос');
  await search.fill('Черновик');
  expect(literalQueries).toEqual([]);
  expect(semanticQueries).toEqual([]);
  await search.press('Enter');
  const literalResults = page.getByRole('region', { name: 'Совпадения по ID и тексту' });
  const semanticResults = page.getByRole('region', { name: 'Поиск по смыслу' });
  await expect(literalResults.getByText('Черновик 0', { exact: true })).toBeVisible();
  await expect(semanticResults.getByRole('alert')).toBeVisible();
  expect(literalQueries).toEqual([{ match: 'Черновик', after: null }]);
  expect(semanticQueries).toEqual(['Черновик']);

  const more = literalResults.getByRole('button', { name: 'Загрузить ещё' });
  await more.press('Enter');
  const archived = literalResults.getByRole('article').filter({ hasText: 'Черновик 40' });
  await expect(archived.getByText('В архиве')).toBeVisible();
  await expect(archived.getByText('source-40', { exact: true })).toBeVisible();
  await expect(archived.getByRole('link', { name: 'Перейти к требованию' })).toHaveAttribute(
    'href',
    '/projects/demo/requirements?requirement=source-40#requirement-source-40',
  );
  expect(literalQueries).toEqual([
    { match: 'Черновик', after: null },
    { match: 'Черновик', after: 'cursor-40' },
  ]);
  await archived.getByRole('link', { name: 'Перейти к требованию' }).click();
  await expect(page.locator('#requirement-source-40')).toBeFocused();
  await expect(page).toHaveURL(/#requirement-source-40$/);
  await expect(literalResults.getByText('Черновик 40', { exact: true })).toBeVisible();
});

test('search discards superseded responses and clears both groups when access is revoked', async ({
  page,
}) => {
  const previousStarted = Promise.withResolvers<void>();
  const previousRelease = Promise.withResolvers<void>();
  const previousFinished = Promise.withResolvers<void>();
  let previousRequests = 0;
  let previousResponses = 0;
  let allowed = true;
  await page.route('**/api/events', (route) =>
    allowed
      ? route.fulfill({ status: 200, contentType: 'text/event-stream', body: ': heartbeat\n\n' })
      : route.fulfill({ status: 403 }),
  );
  await page.route('**/api/graphql', async (route) => {
    const { query, variables } = route.request().postDataJSON();
    const keyword = query.includes('query RequirementMatches');
    const semantic = query.includes('searchRequirements(');
    const search = variables.match ?? variables.text;
    if (keyword || semantic) {
      const previous = search === 'предыдущий';
      if (previous) {
        if (++previousRequests === 2) previousStarted.resolve();
        await previousRelease.promise;
      }
      const data = keyword
        ? {
            requirementsConnection: connection(
              [{ id: 'literal', text: `${search} текст`, version: 1 }],
              variables,
            ),
          }
        : {
            searchRequirements: [
              {
                requirementId: 'semantic',
                projectId: 'demo',
                text: `${search} смысл`,
                sourceText: `Исходный текст: ${search} смысл`,
                version: 1,
                start: 0,
                end: 5,
                score: 1,
              },
            ],
          };
      await route.fulfill({ json: { data } }).catch(() => {});
      if (previous && ++previousResponses === 2) previousFinished.resolve();
      return;
    }
    const data = query.includes('requirementsConnection(')
      ? { requirementsConnection: connection([], variables) }
      : {
          project: {
            id: 'demo',
            name: 'Повторный поиск',
            goal: 'G',
            description: '',
            createdAt: 'now',
            updatedAt: 'now',
          },
        };
    await route.fulfill({ json: { data } });
  });
  await page.goto('/projects/demo/requirements');
  const search = page.getByRole('searchbox', { name: 'Поисковый запрос' });
  await search.fill('предыдущий');
  await search.press('Enter');
  await previousStarted.promise;
  await search.fill('новый');
  await search.press('Enter');
  const literal = page.getByRole('region', { name: 'Совпадения по ID и тексту' });
  const semantic = page.getByRole('region', { name: 'Поиск по смыслу' });
  await expect(literal.getByText('новый текст', { exact: true })).toBeVisible();
  await expect(semantic.getByText('новый смысл', { exact: true })).toBeVisible();
  previousRelease.resolve();
  await previousFinished.promise;
  await expect(literal.getByText('предыдущий текст', { exact: true })).toHaveCount(0);
  await expect(semantic.getByText('предыдущий смысл', { exact: true })).toHaveCount(0);
  await search.clear();
  await expect(literal).toHaveCount(0);
  await expect(semantic).toHaveCount(0);
  await search.fill('новый');
  await search.press('Enter');
  await expect(literal.getByText('новый текст', { exact: true })).toBeVisible();
  await expect(semantic.getByText('новый смысл', { exact: true })).toBeVisible();
  allowed = false;
  await expect(literal).toHaveCount(0);
  await expect(semantic).toHaveCount(0);
});

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
    } else if (query.includes('query RequirementMatches'))
      data = { requirementsConnection: connection([], variables) };
    else if (query.includes('requirementsConnection('))
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
  await expect(
    page.locator('#indexing-source-1 svg[aria-hidden="true"] use[href$="#file-text"]'),
  ).toBeVisible();
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
  await expect(page).toHaveURL(/\/requirements#requirement-search$/);
  await expect(page.getByRole('link', { name: 'Поиск', exact: true })).toHaveCount(0);
  const editor = page.getByRole('dialog');
  await page.getByLabel('Поисковый запрос').fill('черновик');
  await page.getByRole('button', { name: 'Найти требования' }).click();
  const semantic = page.getByRole('region', { name: 'Поиск по смыслу' });
  const snippet = semantic.getByText('сохранить черновик', { exact: true });
  await expect(snippet).toBeVisible();
  await expect(
    semantic.getByRole('article').locator('svg[aria-hidden="true"] use[href$="#file-text"]'),
  ).toBeVisible();
  await snippet.evaluate((el) => {
    const node = el.firstChild;
    if (node) window.getSelection()?.setBaseAndExtent(node, 10, node, 18);
  });
  await page.getByRole('button', { name: 'Добавить выделение в словарь' }).click();
  await expect(editor.getByLabel('Название понятия')).toHaveValue('черновик');
  await page.keyboard.press('Escape');
  expect(searchCalls).toBe(1);
  await semantic.getByText('Исходный текст этой версии', { exact: true }).click();
  await expect(semantic.getByText(text, { exact: true })).toBeVisible();
  await semantic.getByRole('link', { name: 'Перейти к требованию' }).click();
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
    else if (query.includes('query RequirementMatches'))
      data = { requirementsConnection: connection([], variables) };
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
  await page.goto('/projects/demo/requirements');
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
  await page.getByRole('link', { name: 'Обзор', exact: true }).click();
  await expect(page).toHaveURL(/\/projects\/demo$/);
  release();
  await expect(page.getByRole('heading', { name: 'Обзор' })).toBeVisible();
  await expect(page).toHaveURL(/\/projects\/demo$/);
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
  await page.getByRole('button', { name: 'Добавить требование', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Добавить требование' });
  const draft = dialog.getByLabel('Текст требования');
  await draft.fill('Draft');
  await dialog.getByRole('button', { name: 'Добавить требование', exact: true }).click();
  await sent;
  try {
    await expect(draft).toBeDisabled();
    await page.keyboard.press('Escape');
    await expect(dialog).toBeVisible();
  } finally {
    release();
  }
  await expect(dialog).toBeHidden();
});
