import { expect, type Page, test } from '@playwright/test';

const project = {
  id: 'summary-project',
  name: 'Проект резюме',
  goal: 'Проверить сводки',
  description: '',
  createdAt: 'now',
  updatedAt: 'now',
};
const original = 'Полный исходный текст требования, который не должен заменяться.';
const suggestion = 'Краткое предложение по требованию.';

async function fixture(
  page: Page,
  options?: {
    markdown?: string;
    formattedSummary?: string;
    matchedSpelling?: string;
    failSummaryReads?: boolean;
    summaryVersion?: number;
    initialStatus?: string;
    offscreenFailures?: number;
    errorCode?: string;
    longPreview?: boolean;
    highlight?: boolean;
  },
) {
  let status = options?.initialStatus ?? 'ready';
  let sourceVersion = 3;
  let sourceText = options?.markdown ?? (options?.highlight ? `🧪 ${original}` : original);
  let summaryVersion = options?.summaryVersion ?? sourceVersion;
  let failSummaryReads = options?.failSummaryReads ?? false;
  let retries = 0;
  let bulkRetries = 0;
  let offscreenFailures = options?.offscreenFailures ?? 0;
  const failedCount = () => offscreenFailures + Number(status === 'failed');
  let cancellations = 0;
  let summaryReads = 0;
  const matchQueries: string[] = [];
  let termReads = 0;
  let releaseTerms = () => {};
  const pendingTerms = new Promise<void>((resolve) => {
    releaseTerms = resolve;
  });
  const term = {
    id: 'term',
    name: 'Предложение',
    definition: 'Определение из словаря проекта',
    context: '',
    variants: [],
    version: 1,
  };
  const summaryText = () =>
    options?.formattedSummary ??
    (options?.longPreview
      ? 'я'.repeat(176)
      : summaryVersion === 3
        ? suggestion
        : `${suggestion} v${summaryVersion}`);
  function matches(text: string, start: number, end: number) {
    if (options?.matchedSpelling) {
      const spelling = options.matchedSpelling;
      return Array.from(text.matchAll(new RegExp(RegExp.escape(spelling), 'gu')), (match) => {
        const offset = Array.from(text.slice(0, match.index)).length;
        return {
          start: offset,
          end: offset + Array.from(spelling).length,
          terms: [{ id: term.id }],
        };
      });
    }
    return options?.highlight ? [{ start, end, terms: [{ id: term.id }] }] : [];
  }
  const summary = () => ({
    requirementId: 'req-1',
    projectId: project.id,
    version: summaryVersion,
    status,
    text: summaryText(),
    glossaryMatches: matches(summaryText(), 8, 19),
    byteLength: new TextEncoder().encode(summaryText()).length,
    layaEligible: new TextEncoder().encode(summaryText()).length <= 350,
    profile: 'local-test-profile',
    provenance: 'local-test-generator',
    createdAt: '2026-11-06T12:00:00Z',
    errorCode: status === 'failed' ? (options?.errorCode ?? '') : '',
  });

  await page.route('**/api/events', (route) =>
    route.fulfill({ status: 200, contentType: 'text/event-stream', body: ': heartbeat\n\n' }),
  );
  await page.route('**/api/graphql', async (route) => {
    const { query } = route.request().postDataJSON();
    let data: unknown;
    if (query.includes('requirementSummariesByIds(')) {
      summaryReads++;
      matchQueries.push(query);
      if (failSummaryReads) {
        await route.fulfill({ status: 503, json: { error: 'fixture summary failure' } });
        return;
      }
      data = { requirementSummariesByIds: [summary()] };
    } else if (query.includes('requirementGlossary(')) {
      matchQueries.push(query);
      data = {
        requirementGlossary: {
          version: sourceVersion,
          matches: matches(sourceText, 2, 8),
        },
      };
    } else if (query.includes('glossaryTermsByIds(')) {
      termReads++;
      expect(route.request().postDataJSON().variables).toEqual({
        projectId: project.id,
        ids: [term.id],
      });
      await pendingTerms;
      if (termReads === 1 && !options?.matchedSpelling) {
        await route.fulfill({ status: 503, json: { error: 'fixture glossary failure' } });
        return;
      }
      data = { glossaryTermsByIds: [term] };
    } else if (query.includes('failedRequirementSummaryCount(')) {
      expect(route.request().postDataJSON().variables).toEqual({ projectId: project.id });
      data = { failedRequirementSummaryCount: failedCount() };
    } else if (query.includes('retryFailedRequirementSummaries(')) {
      expect(route.request().postDataJSON().variables).toEqual({ projectId: project.id });
      bulkRetries++;
      data = { retryFailedRequirementSummaries: failedCount() };
      offscreenFailures = 0;
      if (status === 'failed') status = 'queued';
    } else if (query.includes('retryRequirementSummary(')) {
      retries++;
      status = 'queued';
      data = { retryRequirementSummary: summary() };
    } else if (query.includes('cancelRequirementSummary(')) {
      cancellations++;
      status = 'cancelled';
      data = { cancelRequirementSummary: summary() };
    } else if (query.includes('updateRequirement(input:')) {
      sourceVersion = Number(route.request().postDataJSON().variables.input.version) + 1;
      sourceText = route.request().postDataJSON().variables.input.text;
      summaryVersion = sourceVersion;
      status = 'ready';
      data = {
        updateRequirement: {
          requirement: {
            id: 'req-1',
            text: sourceText,
            version: sourceVersion,
          },
          fieldErrors: [],
        },
      };
    } else if (query.includes('requirementsConnection('))
      data = {
        requirementsConnection: {
          edges: [
            {
              cursor: 'cursor-1',
              node: {
                id: 'req-1',
                projectId: project.id,
                text: sourceText,
                version: sourceVersion,
                createdAt: 'now',
                updatedAt: 'now',
              },
            },
          ],
          pageInfo: { endCursor: 'cursor-1', hasNextPage: false },
          totalCount: 1,
        },
      };
    else if (query.includes('project(id:')) data = { project };
    else data = {};
    await route.fulfill({ json: { data } }).catch(() => {});
  });
  return {
    matchQueries,
    termReads: () => termReads,
    releaseTerms,
    retries: () => retries,
    bulkRetries: () => bulkRetries,
    cancellations: () => cancellations,
    summaryReads: () => summaryReads,
    setStatus: (next: string) => {
      status = next;
    },
    failSummaryReads: (failed: boolean) => {
      failSummaryReads = failed;
    },
  };
}

test('summary is primary, original is disclosed, and provenance is visible', async ({ page }) => {
  await fixture(page);
  await page.goto('/projects/summary-project/requirements');
  const card = page.locator('#requirement-req-1');
  await expect(card.getByText(suggestion, { exact: true })).toBeVisible();
  const originalDetails = card.locator('details').filter({
    has: page.getByText('Полный исходный текст требования', { exact: true }),
  });
  const originalDisclosure = originalDetails.locator('summary');
  await expect(originalDetails).not.toHaveAttribute('open');
  await expect(card.getByText(original, { exact: true })).toBeHidden();

  await card.getByText('Версия и происхождение резюме', { exact: true }).click();
  await expect(card.getByText('local-test-profile')).toBeVisible();
  await expect(card.getByText('local-test-generator')).toBeVisible();
  await expect(card.getByText('2026-11-06T12:00:00Z')).toBeVisible();

  await originalDisclosure.click();
  await expect(card.getByText(original, { exact: true })).toBeVisible();
});

test('failed summary generation can be retried and cancelled', async ({ page }) => {
  const api = await fixture(page, { initialStatus: 'failed' });
  await page.goto('/projects/summary-project/requirements');
  const card = page.locator('#requirement-req-1');
  await expect(card.getByText('Не удалось создать резюме.', { exact: true })).toBeVisible();
  await card.getByRole('button', { name: 'Повторить создание резюме', exact: true }).click();
  await expect.poll(api.retries).toBe(1);
  await expect(card.getByText('Создание резюме в очереди.')).toBeVisible();
  await card.getByRole('button', { name: 'Отменить создание резюме', exact: true }).click();
  await expect.poll(api.cancellations).toBe(1);
  await expect(card.getByText('Создание резюме отменено.')).toBeVisible();
});

test('bulk retry includes offscreen failures and requires a click after invalidation', async ({
  page,
}) => {
  const api = await fixture(page, { offscreenFailures: 1, errorCode: 'INTERRUPTED' });
  const invalidation = Promise.withResolvers<void>();
  await page.route('**/api/events', async (route) => {
    await invalidation.promise;
    const payload = { schemaVersion: 1, keys: [`project:${project.id}:summaries`] };
    await route
      .fulfill({
        status: 200,
        contentType: 'text/event-stream',
        body: `id: 1\nevent: cache.invalidate\ndata: ${JSON.stringify(payload)}\n\n`,
      })
      .catch(() => {});
  });
  await page.goto('/projects/summary-project/requirements');
  const card = page.locator('#requirement-req-1');
  await expect(card.getByText(suggestion, { exact: true })).toBeVisible();
  const retry = (count: number) =>
    page.getByRole('button', {
      name: `Повторить создание всех резюме с ошибками (${count})`,
      exact: true,
    });
  await expect(retry(1)).toBeVisible();
  api.setStatus('failed');
  invalidation.resolve();
  await expect(retry(2)).toBeEnabled();
  await expect(
    card.getByRole('alert').filter({ hasText: 'Создание резюме было прервано.' }),
  ).toBeVisible();
  expect(api.bulkRetries()).toBe(0);
  await retry(2).click();
  await expect(
    page
      .getByRole('status')
      .filter({ hasText: 'В очередь на повторное создание добавлено резюме: 2.' }),
  ).toBeVisible();
  await expect(card.getByText('Создание резюме в очереди.')).toBeVisible();
  await expect(page.getByRole('button', { name: /Повторить создание всех резюме/ })).toHaveCount(0);
  expect(api.bulkRetries()).toBe(1);
  expect(api.retries()).toBe(0);
});

test('summary query failure preserves source access and offers refresh, not generation', async ({
  page,
}) => {
  const api = await fixture(page, { failSummaryReads: true });
  await page.goto('/projects/summary-project/requirements');
  const card = page.locator('#requirement-req-1');
  await expect(card.getByText('Не удалось загрузить резюме.', { exact: false })).toBeVisible();
  await expect(card.getByText(suggestion, { exact: true })).toHaveCount(0);
  await expect(card.getByRole('button', { name: 'Создать резюме', exact: true })).toHaveCount(0);
  await expect(card.getByText(original, { exact: true })).toBeVisible();
  api.failSummaryReads(false);
  await card.getByRole('button', { name: 'Обновить состояние резюме', exact: true }).click();
  await expect(card.getByText(suggestion, { exact: true })).toBeVisible();
});

test('an old summary version is never shown as the current suggestion', async ({ page }) => {
  await fixture(page, { summaryVersion: 2 });
  await page.goto('/projects/summary-project/requirements');
  const card = page.locator('#requirement-req-1');
  await expect(card.getByText('Это резюме устарело.', { exact: true })).toBeVisible();
  await expect(card.getByText(suggestion, { exact: true })).toHaveCount(0);
  await expect(
    card.getByRole('button', { name: 'Повторить создание резюме', exact: true }),
  ).toBeVisible();
  await expect(
    card.locator('summary').filter({ hasText: 'Полный исходный текст требования' }),
  ).toHaveCount(0);
  await expect(card.getByText(original, { exact: true })).toBeVisible();
});

test('saving a new source version refetches its summary without another SSE event', async ({
  page,
}) => {
  const api = await fixture(page);
  await page.goto('/projects/summary-project/requirements');
  const card = page.locator('#requirement-req-1');
  await expect(card.getByText(suggestion, { exact: true })).toBeVisible();
  const before = api.summaryReads();
  await card.getByRole('button', { name: 'Изменить', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Текст требования').fill('Обновлённая версия требования.');
  await dialog.getByLabel('Причина изменения').fill('Уточнили формулировку.');
  await dialog.getByRole('button', { name: 'Сохранить', exact: true }).click();
  await expect.poll(api.summaryReads).toBeGreaterThan(before);
  await expect(card.getByText(`${suggestion} v4`, { exact: true })).toBeVisible();
  await expect(card.getByText('Обновлённая версия требования.', { exact: true })).toBeHidden();
});

test('a ready preview over the Laya limit remains visible without an eligibility notice', async ({
  page,
}) => {
  await fixture(page, { longPreview: true });
  await page.goto('/projects/summary-project/requirements');
  const card = page.locator('#requirement-req-1');
  await expect(card.getByText('я'.repeat(176), { exact: true })).toBeVisible();
  await expect(card.getByText(/Предпросмотр занимает 352 байт UTF-8/)).toHaveCount(0);
  await expect(card.getByText(original, { exact: true })).toBeHidden();
});

test('a summary preloads its glossary details and the original reuses them', async ({ page }) => {
  const api = await fixture(page, { highlight: true });
  await page.goto('/projects/summary-project/requirements');
  const card = page.locator('#requirement-req-1');
  const term = card.getByRole('button', { name: 'предложение', exact: true });
  await expect(term).toBeVisible();
  await expect.poll(api.termReads).toBe(1);
  expect(api.matchQueries.filter((query) => query.includes('requirementGlossary('))).toHaveLength(
    0,
  );

  await term.focus();
  await expect(page.getByRole('tooltip')).toContainText('Загрузка определений');
  await term.press('Enter');
  api.releaseTerms();
  const retry = page.getByRole('button', { name: 'Повторить загрузку определений', exact: true });
  await expect(retry).toBeFocused();
  await retry.press('Enter');
  await expect(page.getByRole('tooltip')).toContainText('Определение из словаря проекта');
  const edit = page.getByRole('button', { name: 'Изменить понятие', exact: true });
  await expect(edit).toBeFocused();
  await edit.press('Escape');
  await expect(term).toBeFocused();

  await card.getByText('Полный исходный текст требования', { exact: true }).click();
  const originalMatch = card.getByRole('button', { name: 'Полный', exact: true });
  for (const highlight of [originalMatch, term]) {
    await expect(highlight).toHaveCSS('border-top-width', '0px');
    await expect(highlight).toHaveCSS('padding-inline-start', '0px');
    await expect(highlight).toHaveCSS('text-decoration-style', 'dotted');
  }
  await originalMatch.focus();
  await expect(page.getByRole('tooltip')).toContainText('Определение из словаря проекта');
  expect(api.termReads()).toBe(2);
  expect(api.matchQueries).toHaveLength(2);
  for (const query of api.matchQueries) {
    expect(query).toMatch(
      /(?:glossaryMatches|matches)\s*\{\s*start\s+end\s+terms\s*\{\s*id\s*\}\s*\}/,
    );
  }
});

test('selection in a requirement summary offers to link an existing term', async ({ page }) => {
  await fixture(page);
  await page.goto('/projects/summary-project/requirements');
  await page.getByText(suggestion, { exact: true }).evaluate((el) => {
    const range = document.createRange();
    range.selectNodeContents(el);
    window.getSelection()?.removeAllRanges();
    window.getSelection()?.addRange(range);
  });
  await page.getByRole('button', { name: 'Добавить выделение в словарь' }).click();
  await expect(page.getByLabel('Найти существующее понятие для варианта')).toBeVisible();
});

test('a language rejection explains the failure and offers retry', async ({ page }) => {
  await fixture(page, { initialStatus: 'failed', errorCode: 'INVALID_LANGUAGE' });
  await page.goto('/projects/summary-project/requirements');
  await expect(page.getByRole('alert').filter({ hasText: 'языку' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Повторить создание резюме' })).toBeVisible();
});

test('renders requirement Markdown and retains the exact source, including Unicode highlights', async ({
  page,
  context,
}) => {
  const markdown = [
    '# 📦 Перенос поставки',
    '',
    '> Условия',
    '',
    '- Получить согласование',
    '',
    '| Поле | Значение |',
    '| --- | --- |',
    '| Статус | Черновик |',
    '',
    '[Документ](https://example.org/docs)',
    '',
    '[Некорректная ссылка](ftp://example.org/x)',
    '',
    '![Вложение](https://example.org/pixel.png)',
    '',
    '<b>Исходный HTML</b>',
    '',
    '```js',
    'const days = 3;',
    '```',
  ].join('\n');
  await fixture(page, { markdown, initialStatus: 'missing', highlight: true });
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  await page.goto('/projects/summary-project/requirements');
  const card = page.locator('#requirement-req-1');
  const heading = card.getByRole('heading', { level: 1 });
  await expect(heading).toHaveText('📦 Перенос поставки');
  await expect(heading.getByRole('button', { name: '📦 Пере' })).toBeVisible();
  await expect(card.getByRole('cell', { name: 'Черновик' })).toBeVisible();
  await expect(card.locator('blockquote')).toHaveText('Условия');
  await expect(card.locator('li').filter({ hasText: 'Получить согласование' })).toBeVisible();
  const documentLink = card.getByRole('link', { name: 'Документ' });
  await expect(documentLink).toHaveAttribute('href', 'https://example.org/docs');
  await expect(documentLink).toHaveCSS('text-decoration-line', 'underline');
  await expect(card.getByRole('link', { name: 'Некорректная ссылка' })).toHaveCount(0);
  await expect(card.locator('img')).toHaveCount(0);
  await expect(card.getByText('<b>Исходный HTML</b>', { exact: true })).toBeVisible();
  await expect(card.locator('pre code span').filter({ hasText: 'const' })).toBeVisible();
  await card.getByRole('button', { name: 'Скопировать код' }).click();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('const days = 3;');
  await heading.evaluate((el) => window.getSelection()?.selectAllChildren(el));
  await expect(card.getByRole('button', { name: 'Добавить выделение в словарь' })).toBeVisible();
  await card
    .getByRole('button', { name: 'Скопировать код' })
    .evaluate((el) => window.getSelection()?.selectAllChildren(el));
  await expect(card.getByRole('button', { name: 'Добавить выделение в словарь' })).toHaveCount(0);
  await card.getByRole('button', { name: 'Исходная разметка' }).click();
  await expect(card.locator('[data-glossary-text]').first()).toHaveText(markdown);
  await expect(card.locator('[data-glossary-text]').first().getByRole('button')).toHaveText(
    '📦 Пере',
  );
});

test('formatted summaries and originals open glossary definitions within Markdown prose', async ({
  page,
}) => {
  const markdown =
    '[API]: https://example.org/API\n\n- [API] и `API`: 🧪 &amp; **API**\n\nhttps://example.org/API и API';
  const api = await fixture(page, {
    markdown,
    formattedSummary: '# 🧪 API',
    matchedSpelling: 'API',
  });
  api.releaseTerms();
  await page.goto('/projects/summary-project/requirements');
  const card = page.locator('#requirement-req-1');
  const summaryMatch = card.getByRole('heading').getByRole('button', { name: 'API' });
  await expect(summaryMatch).toHaveCSS('text-decoration-style', 'dotted');
  await summaryMatch.focus();
  await expect(page.getByRole('tooltip')).toContainText('Определение из словаря проекта');
  await page.keyboard.press('Escape');
  await card.getByText('Полный исходный текст требования', { exact: true }).click();
  const formatted = card.locator('details .requirement-markdown');
  await expect(formatted.locator('li strong').getByRole('button', { name: 'API' })).toBeVisible();
  await expect(formatted.getByRole('link', { name: 'API', exact: true })).toHaveAttribute(
    'href',
    'https://example.org/API',
  );
  await expect(formatted.locator('p').getByRole('button', { name: 'API' })).toBeVisible();
  await expect(formatted.locator('a button, code button')).toHaveCount(0);
  await card.locator('details').getByRole('button', { name: 'Исходная разметка' }).click();
  const source = card.locator('details [data-glossary-text]');
  await expect(source).toHaveText(markdown);
  await expect(source.getByRole('button', { name: 'API' })).toHaveCount(7);
});

test('formatted Markdown paragraphs keep glossary source identity', async ({ page }) => {
  const api = await fixture(page, {
    markdown: 'Заказ поставщика\n\nПеренос срока',
    initialStatus: 'missing',
  });
  await page.goto('/projects/summary-project/requirements');
  await expect.poll(api.summaryReads).toBeGreaterThan(0);
  const card = page.locator('#requirement-req-1');
  const paragraph = card.locator('p').filter({ hasText: /^Перенос срока$/ });
  await expect(paragraph).toBeVisible();
  const block = card.locator('[data-glossary-text]').filter({ hasText: 'Перенос срока' });
  await expect(block).toHaveAttribute('data-source-id', 'req-1');
  await expect(block).toHaveAttribute('data-source-version', '3');
  await paragraph.evaluate((element) => window.getSelection()?.selectAllChildren(element));
  await card.getByRole('button', { name: 'Добавить выделение в словарь' }).click();
  await expect(
    page.getByRole('dialog').getByLabel('Название понятия', { exact: true }),
  ).toHaveValue('Перенос срока');
});
