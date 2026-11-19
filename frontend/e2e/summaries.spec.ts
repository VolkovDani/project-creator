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
  },
) {
  let status = options?.initialStatus ?? 'ready';
  let sourceVersion = 3;
  let sourceText = original;
  let summaryVersion = options?.summaryVersion ?? sourceVersion;
  let failSummaryReads = options?.failSummaryReads ?? false;
  let retries = 0;
  let cancellations = 0;
  let summaryReads = 0;
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
      if (failSummaryReads) {
        await route.fulfill({ status: 503, json: { error: 'fixture summary failure' } });
        return;
      }
      data = { requirementSummariesByIds: [summary()] };
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
  await expect(card.getByText(original, { exact: true })).toBeHidden();
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
  await card.getByText('Полный исходный текст требования', { exact: true }).click();
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
