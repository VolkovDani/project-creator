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
const proposed = 'Подпись для подтверждения документа.';

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
        const success = status === 'suggested' || status === 'delayed';
        data = {
          suggestGlossaryDefinition: {
            status: success ? 'suggested' : status,
            definition: success ? proposed : null,
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
