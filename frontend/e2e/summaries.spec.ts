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
    failSummaryReads?: boolean;
    summaryVersion?: number;
    initialStatus?: string;
    longPreview?: boolean;
    highlight?: boolean;
  },
) {
  let status = options?.initialStatus ?? 'ready';
  let sourceVersion = 3;
  let sourceText = options?.highlight ? `🧪 ${original}` : original;
  let summaryVersion = options?.summaryVersion ?? sourceVersion;
  let failSummaryReads = options?.failSummaryReads ?? false;
  let retries = 0;
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
    options?.longPreview
      ? 'я'.repeat(176)
      : summaryVersion === 3
        ? suggestion
        : `${suggestion} v${summaryVersion}`;
  const summary = () => ({
    requirementId: 'req-1',
    projectId: project.id,
    version: summaryVersion,
    status,
    text: summaryText(),
    glossaryMatches: options?.highlight
      ? [
          {
            start: 8,
            end: 19,
            terms: [{ id: term.id }],
          },
        ]
      : [],
    byteLength: new TextEncoder().encode(summaryText()).length,
    layaEligible: new TextEncoder().encode(summaryText()).length <= 350,
    profile: 'local-test-profile',
    provenance: 'local-test-generator',
    createdAt: '2026-11-06T12:00:00Z',
    errorCode: '',
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
          matches: options?.highlight ? [{ start: 2, end: 8, terms: [{ id: term.id }] }] : [],
        },
      };
    } else if (query.includes('glossaryTermsByIds(')) {
      termReads++;
      expect(route.request().postDataJSON().variables).toEqual({
        projectId: project.id,
        ids: [term.id],
      });
      await pendingTerms;
      if (termReads === 1) {
        await route.fulfill({ status: 503, json: { error: 'fixture glossary failure' } });
        return;
      }
      data = { glossaryTermsByIds: [term] };
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
  const originalDetails = card.locator('details').last();
  const originalDisclosure = originalDetails.getByText('Полный исходный текст требования', {
    exact: true,
  });
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
