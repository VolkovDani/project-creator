import { expect, type Page, test } from '@playwright/test';

const candidate = {
  abbreviation: 'ЭЦП',
  name: '',
  before: '😀 Запросить ',
  after: ' для подписания документа.',
  sourceId: 'r1',
  sourceVersion: 3,
  start: 12,
  end: 15,
};
const proposed = 'Подпись для подтверждения документа.\nСохраняет  исходный текст.';

async function openCandidate(page: Page, results = ['suggested']) {
  const requests: unknown[] = [];
  const saves: unknown[] = [];
  const pageInfo = { hasNextPage: false, endCursor: null };
  let release: (() => void) | undefined;
  await page.route('**/api/events', (route) =>
    route.fulfill({ contentType: 'text/event-stream', body: ': heartbeat\n\n' }),
  );
  await page.route('**/api/graphql', async (route) => {
    const { operationName, variables } = route.request().postDataJSON();
    let data: unknown;
    switch (operationName) {
      case 'ProjectDetail':
        data = { project: { id: 'demo', name: 'Проект', goal: '', description: '' } };
        break;
      case 'GlossaryTermPages':
        data = { glossaryTermsConnection: { items: [], pageInfo } };
        break;
      case 'GlossaryCandidatePages':
        data = { glossaryCandidatesConnection: { items: [candidate], pageInfo } };
        break;
      case 'SuggestGlossaryDefinition': {
        const status = results[requests.length];
        requests.push(variables.input);
        if (status === 'delayed')
          await new Promise<void>((resolve) => {
            release = resolve;
          });
        data = {
          suggestGlossaryDefinition: {
            status: status === 'delayed' ? 'suggested' : status,
            definition: ['suggested', 'delayed', 'hypothesis'].includes(status) ? proposed : null,
            fieldErrors: [],
          },
        };
        break;
      }
      case 'SaveTerm':
        saves.push(variables.input);
        data = {
          saveGlossaryTerm: {
            term: { ...variables.input, id: 'term', version: 1 },
            fieldErrors: [],
          },
        };
        break;
    }
    await route.fulfill({ json: { data } });
  });
  await page.goto('/projects/demo/glossary');
  await page.getByRole('button', { name: /ЭЦП/ }).click();
  const dialog = page.getByRole('dialog');
  return {
    dialog,
    requests,
    saves,
    release: () => release?.(),
    generate: dialog.getByRole('button', { name: 'Сгенерировать определение' }),
    apply: dialog.getByRole('button', { name: 'Применить' }),
    definition: dialog.getByRole('textbox', { name: 'Определение', exact: true }),
    name: dialog.getByLabel('Название понятия'),
  };
}

test('a standalone candidate shows its source and a suggestion is applied and saved explicitly', async ({
  page,
}) => {
  const { dialog, requests, saves, generate, apply, definition, name, release } =
    await openCandidate(page, ['delayed', 'suggested']);
  await expect(dialog.locator('mark')).toHaveText(candidate.abbreviation);
  await expect(
    dialog.getByText(candidate.before + candidate.abbreviation + candidate.after),
  ).toBeVisible();
  await expect(name).toHaveValue('');
  await generate.click();
  await expect(generate).toBeDisabled();
  await expect.poll(() => requests.length).toBe(1);
  await definition.fill('Моя формулировка');
  release();
  await expect(dialog.getByText(proposed)).toBeVisible();
  await expect(definition).toHaveValue('Моя формулировка');
  expect(requests).toEqual([
    { projectId: 'demo', sourceId: 'r1', sourceVersion: 3, start: 12, end: 15, name: '' },
  ]);
  await apply.click();
  await expect(definition).toHaveValue(proposed);
  expect(saves).toEqual([]);
  await name.fill('Электронная подпись');
  await definition.fill('Проверенное определение');
  await generate.click();
  await expect(dialog.getByText('Предложение ИИ')).toBeVisible();
  await expect(definition).toHaveValue('Проверенное определение');
  expect(requests[1]).toEqual(expect.objectContaining({ name: 'Электронная подпись' }));
  expect(saves).toEqual([]);
  await dialog.getByRole('button', { name: 'Подтвердить и сохранить' }).click();
  await expect(dialog).toHaveCount(0);
  expect(saves).toEqual([
    expect.objectContaining({
      sourceId: 'r1',
      sourceVersion: 3,
      definition: 'Проверенное определение',
    }),
  ]);
});

test('changing the concept name ignores a late suggestion and failed generation preserves the draft', async ({
  page,
}) => {
  const { dialog, requests, saves, generate, apply, definition, name, release } =
    await openCandidate(page, ['delayed', 'insufficient', 'stale', 'unavailable']);
  await definition.fill('Черновик');
  await generate.click();
  await expect.poll(() => requests.length).toBe(1);
  await name.fill('Новое название');
  const oldResponse = page.waitForResponse(
    (r) => r.request().postDataJSON()?.operationName === 'SuggestGlossaryDefinition',
  );
  release();
  await oldResponse;
  await expect(apply).toHaveCount(0);
  for (const message of [
    'Недостаточно контекста',
    'Источник изменился',
    'Не удалось предложить определение',
  ]) {
    await generate.click();
    await expect(dialog.getByText(message, { exact: false })).toBeVisible();
    await expect(definition).toHaveValue('Черновик');
    await expect(name).toHaveValue('Новое название');
    await expect(apply).toHaveCount(0);
  }
  expect(saves).toEqual([]);
});

test('copy writes the exact suggestion and leaves the draft and preview in place', async ({
  page,
  context,
}) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  const { dialog, definition, generate, apply, requests, saves } = await openCandidate(page);
  await definition.fill('Моя формулировка');
  await generate.click();
  await dialog.getByRole('button', { name: 'Скопировать' }).click();
  await expect(dialog.getByText('Определение скопировано.')).toBeVisible();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(proposed);
  await expect(definition).toHaveValue('Моя формулировка');
  await expect(apply).toBeVisible();
  expect(requests).toHaveLength(1);
  expect(saves).toEqual([]);
});

test('denied clipboard access selects only the definition for manual copying', async ({ page }) => {
  await page.addInitScript(() => {
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: () => Promise.reject(new DOMException('Denied', 'NotAllowedError')) },
    });
  });
  const { dialog, definition, generate, apply, saves } = await openCandidate(page);
  await generate.click();
  await dialog.getByRole('button', { name: 'Скопировать' }).click();
  await expect(dialog.getByText('Скопируйте выделенный текст вручную.')).toBeVisible();
  expect(await page.evaluate(() => window.getSelection()?.toString())).toBe(proposed);
  await expect(definition).toHaveValue('');
  await expect(apply).toBeVisible();
  expect(saves).toEqual([]);
});

test('an unconfirmed meaning is labeled as a hypothesis and can be copied or applied explicitly', async ({
  page,
  context,
}) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  const { dialog, definition, generate, apply, saves } = await openCandidate(page, ['hypothesis']);
  await definition.fill('Черновик');
  await generate.click();
  await expect(dialog.getByText('Гипотеза ИИ', { exact: true })).toBeVisible();
  await expect(dialog.getByText('Источник не подтверждает это значение.')).toBeVisible();
  await dialog.getByRole('button', { name: 'Скопировать' }).click();
  await expect(dialog.getByText('Определение скопировано.')).toBeVisible();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(proposed);
  await expect(definition).toHaveValue('Черновик');
  await apply.click();
  await expect(definition).toHaveValue(proposed);
  expect(saves).toEqual([]);
});
