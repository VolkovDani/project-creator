import { expect, type Page, test } from '@playwright/test';

const candidate = {
  abbreviation: 'ЭЦП',
  name: '',
  before: '😀 Запросить ',
  after: ' для подписания документа.',
  beforeTruncated: false,
  afterTruncated: false,
  sourceId: 'r1',
  sourceVersion: 3,
  start: 12,
  end: 15,
};
const proposed = 'Подпись для подтверждения документа.\nСохраняет  исходный текст.';

async function visitGlossary(page: Page, results: string[], candidates = [candidate]) {
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
        data = { glossaryCandidatesConnection: { items: candidates, pageInfo } };
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
  return { requests, saves, release: () => release?.() };
}

async function openCandidate(page: Page, results = ['suggested']) {
  const operations = await visitGlossary(page, results);
  await page.getByRole('button', { name: /ЭЦП/ }).click();
  const dialog = page.getByRole('dialog');
  return {
    dialog,
    ...operations,
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

for (const theme of ['light', 'dark']) {
  test(`wrapped candidate excerpts fade only where the source continues in the ${theme} theme`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 360, height: 800 });
    const before = `${'𐐨'.repeat(59)} `;
    const after = ` ${'я'.repeat(59)}`;
    const candidates = [
      [false, false],
      [true, false],
      [false, true],
      [true, true],
    ].map(([beforeTruncated, afterTruncated], i) => {
      const start = beforeTruncated ? 61 : 60;
      return {
        ...candidate,
        sourceId: `r${i}`,
        before,
        after,
        start,
        end: start + 3,
        beforeTruncated,
        afterTruncated,
      };
    });
    await visitGlossary(page, [], candidates);
    if (theme === 'dark') await page.getByRole('button', { name: 'Тёмная тема' }).click();
    await expect(page.locator('html')).toHaveCSS('color-scheme', theme);
    for (const c of candidates) {
      const card = page
        .getByRole('article')
        .filter({ has: page.locator(`a[href*="requirement=${c.sourceId}"]`) });
      const excerpt = card.locator('p');
      await expect(excerpt).toHaveText(c.before + c.abbreviation + c.after);
      const mark = excerpt.locator('mark');
      await expect(mark).toHaveText(c.abbreviation);
      const beforeSpan = excerpt.locator('span').first();
      const afterSpan = excerpt.locator('span').last();
      for (const [side, clipped, gradient] of [
        [beforeSpan, c.beforeTruncated, /linear-gradient\((?:to right|90deg),/],
        [afterSpan, c.afterTruncated, /linear-gradient\((?:to left|270deg|-90deg),/],
      ] as const) {
        await expect(side).toHaveCSS('mask-image', clipped ? gradient : 'none');
        await expect(side).toHaveCSS('box-decoration-break', 'slice');
        expect(await side.evaluate((el) => el.getClientRects().length)).toBeGreaterThan(1);
      }
      await expect(mark).toHaveCSS('mask-image', 'none');
      const link = card.getByRole('link', { name: 'Открыть в тексте' });
      await link.focus();
      await expect(link).toBeFocused();
      await expect(link).toHaveAttribute(
        'href',
        `/projects/demo/requirements?requirement=${c.sourceId}&version=3&start=${c.start}&end=${c.end}#requirement-${c.sourceId}`,
      );
      expect(
        await excerpt.evaluate((el) => {
          const selection = window.getSelection();
          selection?.selectAllChildren(el);
          return selection?.toString();
        }),
      ).toBe(c.before + c.abbreviation + c.after);
    }
  });
}
