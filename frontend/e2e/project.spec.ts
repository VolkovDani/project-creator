import { expect, type Page, test } from '@playwright/test';

async function addRequirement(page: Page, text: string) {
  await page.locator('#requirement-entry').click();
  const dialog = page.getByRole('dialog', { name: 'Добавить требование' });
  await dialog.getByRole('textbox', { name: 'Текст требования', exact: true }).fill(text);
  await dialog.getByRole('button', { name: 'Добавить требование' }).click();
  await expect(dialog).toBeHidden();
}

test('requirement creation restores focus and the saved record can be found by ID and text', async ({
  page,
}) => {
  await page.goto('/projects/new');
  await page.getByLabel('Название').fill('Проверка создания требования');
  await page.getByLabel('Цель').fill('Проверить диалог');
  await page.getByRole('button', { name: 'Создать проект' }).click();
  await page.getByRole('navigation').getByRole('link', { name: 'Требования', exact: true }).click();

  const opener = page.locator('#requirement-entry');
  const dialog = page.getByRole('dialog', { name: 'Добавить требование' });
  const draft = dialog.getByRole('textbox', { name: 'Текст требования' });
  const submit = dialog.getByRole('button', { name: 'Добавить требование' });
  await expect(page.getByRole('textbox', { name: 'Текст требования' })).toHaveCount(0);
  await opener.press('Enter');
  await expect(dialog).toBeVisible();
  await expect(draft).toBeFocused();
  await submit.click();
  await expect(draft).toHaveAttribute('aria-invalid', 'true');
  await draft.fill('Отменённый черновик');
  await page.keyboard.press('Escape');
  await expect(dialog).toBeHidden();
  await expect(opener).toBeFocused();
  await expect(page.getByText('Требований пока нет')).toBeVisible();

  await opener.click();
  await expect(draft).toHaveValue('');
  await draft.fill('Сохранённое требование');
  await submit.click();
  await expect(dialog).toBeHidden();
  await expect(opener).toBeFocused();
  const saved = page.locator('#requirements-list').getByRole('button', {
    name: 'Сохранённое требование',
  });
  await expect(saved).toBeVisible();
  const id = (await saved.getAttribute('id'))?.replace('requirement-row-', '');
  if (!id) throw Error('Missing requirement ID');
  const search = page.getByRole('searchbox', { name: 'Поисковый запрос' });
  const matches = page.getByRole('region', { name: 'Совпадения по ID и тексту' });
  await search.fill('СОХРАНЁННОЕ');
  await search.press('Enter');
  await expect(matches).toContainText('Сохранённое требование');
  await search.fill(id);
  await search.press('Enter');
  await expect(matches.getByText(id, { exact: true })).toBeVisible();
  await matches.getByRole('link', { name: 'Перейти к требованию' }).click();
  await expect(page.locator(`#requirement-${id}`)).toBeFocused();
});

test('project section help is shown on hover and keyboard focus without a click', async ({
  page,
}) => {
  await page.goto('/projects/new');
  await page.getByLabel('Название').fill('Проверка справки');
  await page.getByLabel('Цель').fill('Проверить подсказки меню');
  await page.getByRole('button', { name: 'Создать проект' }).click();

  const nav = page.getByRole('navigation', { name: 'Разделы проекта' });
  const requirements = nav.getByRole('button', { name: 'Справка о разделе «Требования»' });
  const tooltip = nav.getByRole('tooltip');
  await expect(requirements).toBeVisible();
  await expect(tooltip).toHaveCount(0);
  await requirements.hover();
  await expect(tooltip).toContainText('Ищите и редактируйте бизнес-требования');
  await page.mouse.move(0, 0);
  await expect(tooltip).toHaveCount(0);

  await nav.getByRole('link', { name: 'Требования', exact: true }).focus();
  await page.keyboard.press('Tab');
  await expect(requirements).toBeFocused();
  await expect(tooltip).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(tooltip).toHaveCount(0);
  await expect(requirements).toBeFocused();
  await expect(nav.getByRole('link', { name: 'Поиск', exact: true })).toHaveCount(0);
  await expect(nav.getByRole('button', { name: /Справка о разделе/ })).toHaveCount(7);
  await nav.getByRole('link', { name: 'Графы процессов', exact: true }).focus();
  await page.keyboard.press('Tab');
  await expect(
    nav.getByRole('button', { name: 'Справка о разделе «Графы процессов»' }),
  ).toBeFocused();
  await expect(tooltip).toContainText(/ключев/);
  await page.keyboard.press('Escape');

  await nav.getByRole('link', { name: 'Технический раздел' }).click();
  const indexing = nav.getByRole('link', { name: 'Индексация', exact: true });
  await indexing.focus();
  await page.keyboard.press('Tab');
  await expect(tooltip).toContainText(/индексац/i);
  await expect(
    nav.getByRole('button', { name: 'Справка о разделе «История запусков»' }),
  ).toBeVisible();
});

test('create a project and open it after reload', async ({ page }) => {
  const name = `Проект ${Date.now()}`;
  await page.goto('/projects/new');
  await page.getByRole('button', { name: 'Создать проект' }).click();
  await expect(page.getByText('Заполните поле').first()).toBeVisible();
  await page.getByLabel('Название').fill(name);
  await page.getByLabel('Цель').fill('Найти коллизии');
  await page.getByLabel('Описание').fill('Тестовая карточка');
  await page.getByRole('button', { name: 'Создать проект' }).click();
  await expect(page.getByRole('heading', { name, exact: true })).toBeVisible();
  await page.reload();
  await expect(page.getByRole('heading', { name, exact: true })).toBeVisible();
  await expect(page.getByText('Найти коллизии')).toBeVisible();
  await page.getByRole('link', { name: 'Требования', exact: true }).first().click();
  await addRequirement(page, 'Требование один');
  const details = page.locator('article[id^="requirement-"]');
  await expect(details.getByText('Требование один', { exact: true })).toBeVisible();
  await page.reload();
  await expect(details.getByText('Требование один', { exact: true })).toBeVisible();
  const other = await page.context().newPage();
  await other.goto(page.url());
  const otherDetails = other.locator('article[id^="requirement-"]');
  await otherDetails.getByRole('button', { name: 'Изменить', exact: true }).click();
  await other.getByRole('dialog').getByLabel('Текст требования').fill('Мой черновик');
  await other.getByLabel('Причина изменения').fill('Учесть решение заказчика');
  await details.getByRole('button', { name: 'Изменить', exact: true }).click();
  await page.getByRole('dialog').getByLabel('Текст требования').fill('Требование два');
  const reason = page.getByRole('dialog').getByLabel('Причина изменения');
  await reason.fill('   ');
  await page.getByRole('dialog').getByRole('button', { name: 'Сохранить' }).click();
  await expect(reason).toHaveAttribute('aria-invalid', 'true');
  await reason.fill('Уточнить сроки');
  await page.getByRole('dialog').getByRole('button', { name: 'Сохранить' }).click();
  await expect(details.getByRole('definition').filter({ hasText: /^2$/ })).toBeVisible();
  await other.getByRole('dialog').getByRole('button', { name: 'Сохранить' }).click();
  await expect(
    other.getByRole('dialog').getByText('Требование изменилось или недоступно. Обновите данные.'),
  ).toBeVisible();
  await expect(other.getByRole('dialog').getByLabel('Текст требования')).toHaveValue(
    'Мой черновик',
  );
  await expect(other.getByLabel('Причина изменения')).toHaveValue('Учесть решение заказчика');
  await other
    .getByRole('dialog')
    .getByRole('button', { name: 'Сохранить черновик поверх актуальной версии' })
    .click();
  await other.getByRole('dialog').getByRole('button', { name: 'Сохранить', exact: true }).click();
  await expect(otherDetails.getByRole('definition').filter({ hasText: /^3$/ })).toBeVisible();
  await page.reload();
  await expect(details.getByText('Мой черновик', { exact: true })).toBeVisible();
  const history = details.getByRole('group', { name: 'История текста требования' });
  await history.locator('summary').click();
  const versions = history.getByRole('listitem');
  await expect(versions).toHaveCount(3);
  await expect(versions.nth(0)).toContainText('Версия 2 → 3');
  await expect(versions.nth(0)).toContainText('Требование два');
  await expect(versions.nth(0)).toContainText('Мой черновик');
  await expect(versions.nth(0)).toContainText('Учесть решение заказчика');
  await expect(versions.nth(1)).toContainText('Уточнить сроки');
  await expect(versions.nth(2)).toContainText('Требование один');
  await other.close();
});
test('only the selected archived requirement can be permanently deleted after confirmation', async ({
  page,
}) => {
  await page.goto('/projects/new');
  await page.getByLabel('Название').fill('Проверка архива');
  await page.getByLabel('Цель').fill('Безопасное удаление');
  await page.getByRole('button', { name: 'Создать проект' }).click();
  await page.getByRole('navigation').getByRole('link', { name: 'Требования', exact: true }).click();
  const selected = 'Проверить срок поставки';
  const survivor = 'Согласовать цену поставки';
  const list = page.locator('#requirements-list');
  const selectedRow = list.getByRole('button', { name: selected, exact: true });
  for (const text of [selected, survivor]) {
    await addRequirement(page, text);
    await expect(list.getByRole('button', { name: text, exact: true })).toBeVisible();
  }
  const mutations: string[] = [];
  page.on('request', (request) => {
    if (request.url().endsWith('/api/graphql') && request.method() === 'POST') {
      const operation = request.postDataJSON()?.operationName;
      if (operation) mutations.push(operation);
    }
  });
  const details = page.locator('article[id^="requirement-"]').filter({ hasText: selected });
  const move = details.getByRole('button', { name: 'В архив', exact: true });
  await expect(page.getByRole('button', { name: 'Удалить навсегда' })).toHaveCount(0);
  await selectedRow.click();
  await move.click();
  const dialog = page.getByRole('dialog');
  await expect(dialog.getByText(selected, { exact: true })).toBeVisible();
  await expect(dialog.getByRole('button', { name: 'Отмена' })).toBeFocused();
  await dialog.getByRole('button', { name: 'Отмена' }).click();
  await expect(move).toBeFocused();
  expect(mutations).not.toContain('ArchiveRequirement');
  await move.click();
  await dialog.getByRole('button', { name: 'Переместить в архив' }).click();
  const archive = page.getByRole('group', { name: 'Архив требований' });
  await archive.locator('summary').click();
  const archivedRow = archive.getByRole('button', { name: selected, exact: true });
  await archivedRow.click();
  const purge = details.getByRole('button', { name: 'Удалить навсегда' });
  await purge.click();
  await expect(dialog.getByRole('button', { name: 'Отмена' })).toBeFocused();
  await page.keyboard.press('Escape');
  await expect(purge).toBeFocused();
  expect(mutations).not.toContain('RemoveRequirement');
  await details.getByRole('button', { name: 'Восстановить' }).click();
  await selectedRow.click();
  await expect(move).toBeVisible();
  await move.click();
  await dialog.getByRole('button', { name: 'Переместить в архив' }).click();
  await archivedRow.click();
  await purge.click();
  await dialog.getByRole('button', { name: 'Удалить навсегда' }).click();
  await expect(archive.getByText('Архив пуст')).toBeVisible();
  expect(mutations.filter((name) => name === 'RemoveRequirement')).toHaveLength(1);
  await expect(list.getByRole('button', { name: survivor, exact: true })).toBeVisible();
});
test('pair analysis uses the threshold configured on the running backend', async ({ page }) => {
  await page.goto('/projects/new');
  await page.getByLabel('Название').fill('Настраиваемый порог');
  await page.getByLabel('Цель').fill('Проверка отбора пар');
  await page.getByRole('button', { name: 'Создать проект' }).click();
  await page.getByRole('navigation').getByRole('link', { name: 'Анализ', exact: true }).click();

  const input = page.getByRole('spinbutton', { name: 'Порог семантической близости' });
  await expect(input).toHaveValue('0.735');
  const started = page.waitForRequest(
    (request) =>
      request.url().endsWith('/api/graphql') &&
      request.postDataJSON()?.operationName === 'StartProjectAnalysis',
  );
  await page.getByRole('button', { name: 'Проверить требования', exact: true }).click();
  expect((await started).postDataJSON().variables.threshold).toBe(0.735);
});
test('create stays locked until its response completes', async ({ page }) => {
  let release!: () => void;
  const pending = new Promise<void>((resolve) => {
    release = resolve;
  });
  let calls = 0;
  await page.route('**/api/events', (route) =>
    route.fulfill({ status: 200, contentType: 'text/event-stream', body: ': heartbeat\n\n' }),
  );
  await page.route('**/api/graphql', async (route) => {
    const payload = route.request().postDataJSON();
    if (payload.query.includes('mutation CreateProject')) {
      calls++;
      await pending;
      await route.fulfill({
        json: {
          data: {
            createProject: {
              project: {
                id: 'created-project',
                name: 'new',
                goal: 'goal',
                description: '',
                createdAt: 'now',
                updatedAt: 'now',
              },
              fieldErrors: [],
            },
          },
        },
      });
    } else {
      await route.fulfill({
        json: {
          data: {
            project: {
              id: 'created-project',
              name: 'new',
              goal: 'goal',
              description: '',
              createdAt: 'now',
              updatedAt: 'now',
            },
          },
        },
      });
    }
  });
  await page.goto('/projects/new');
  await page.getByLabel('Название').fill('new');
  await page.getByLabel('Цель').fill('goal');
  await page.getByRole('button', { name: 'Создать проект' }).click();
  await expect(page.getByRole('button', { name: 'Создание…' })).toBeDisabled();
  expect(calls).toBe(1);
  release();
  await expect(page.getByRole('heading', { name: 'new', exact: true })).toBeVisible();
});

test('two tabs receive project and requirement invalidations without reload', async ({ page }) => {
  const watcher = await page.context().newPage();
  await page.goto('/');
  await watcher.addInitScript(() => localStorage.setItem('project-creator-locale', 'en'));
  await watcher.goto('/');
  const projectCount = watcher
    .getByRole('region', { name: 'Projects', exact: true })
    .getByText(/^Total: [0-9]+$/);
  const initialCount = Number((await projectCount.textContent())?.split(' ').at(-1));
  const name = `Live ${Date.now()}`;
  await page.getByRole('link', { name: 'Создать проект', exact: true }).click();
  await page.getByLabel('Название').fill(name);
  await page.getByLabel('Цель').fill('SSE');
  await page.getByRole('button', { name: 'Создать проект' }).click();
  await expect(projectCount).toHaveText(`Total: ${initialCount + 1}`, { timeout: 15000 });
  const projectLink = watcher.getByRole('link', { name, exact: true });
  if (!(await projectLink.isVisible())) await watcher.keyboard.press('End');
  await projectLink.click();
  await page.getByRole('link', { name: 'Требования', exact: true }).first().click();
  await watcher.getByRole('link', { name: 'Requirements', exact: true }).first().click();
  await addRequirement(page, 'Live requirement');
  const watcherList = watcher.locator('#requirements-list');
  await watcherList.getByRole('button', { name: 'Live requirement', exact: true }).click({
    timeout: 15000,
  });
  const details = page.locator('article[id^="requirement-"]');
  const watcherDetails = watcher.locator('article[id^="requirement-"]');
  await details.getByRole('button', { name: 'Изменить', exact: true }).click();
  const history = watcherDetails.getByRole('group', { name: 'Requirement text history' });
  await history.locator('summary').click();
  await expect(history.getByRole('listitem')).toHaveCount(1);
  await page.getByRole('dialog').getByLabel('Текст требования').fill('Changed in another tab');
  await page.getByRole('dialog').getByLabel('Причина изменения').fill('Clarify the deadline');
  await page.getByRole('dialog').getByRole('button', { name: 'Сохранить', exact: true }).click();
  const revision = history.getByRole('listitem').first();
  await expect(revision).toContainText('Version 1 → 2', { timeout: 15000 });
  await expect(revision).toContainText('Changed in another tab');
  await expect(revision).toContainText('Clarify the deadline');
  await details.getByRole('button', { name: 'В архив', exact: true }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Переместить в архив' }).click();
  await expect(watcher.getByText('No requirements yet')).toBeVisible({ timeout: 15000 });
  const archive = watcher.getByRole('group', { name: 'Requirements archive' });
  await archive.locator('summary').click();
  await archive.getByRole('button', { name: 'Changed in another tab', exact: true }).click();
  await watcherDetails.getByRole('button', { name: 'Restore', exact: true }).click();
  await expect(details.getByRole('definition').filter({ hasText: /^2$/ })).toBeVisible({
    timeout: 15000,
  });
  await watcherList.getByRole('button', { name: 'Changed in another tab', exact: true }).click();
  await expect(revision).toContainText('Clarify the deadline');
  await expect(history.getByRole('listitem')).toHaveCount(2);
  await watcher.close();
});

test('an abbreviation candidate becomes a glossary term only after confirmation', async ({
  page,
}) => {
  await page.goto('/projects/new');
  await page.getByLabel('Название').fill('Глоссарий');
  await page.getByLabel('Цель').fill('Проверить сокращения');
  await page.getByRole('button', { name: 'Создать проект' }).click();
  await page.getByRole('link', { name: 'Требования', exact: true }).first().click();
  await addRequirement(
    page,
    'Генеральный подрядчик (ГП), также Генподрядчик, проверяет документы.',
  );
  await addRequirement(page, '😀 Запросить ЭЦП у ГП.');
  await expect(
    page.locator('#requirements-list').getByRole('button', {
      name: '😀 Запросить ЭЦП у ГП.',
      exact: true,
    }),
  ).toBeVisible();
  await page.getByRole('link', { name: 'Словарь', exact: true }).first().click();
  await expect(page.getByRole('heading', { name: 'Словарь проекта', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Новое понятие', exact: true }).click();
  await expect(page.getByRole('dialog').getByLabel('Название понятия')).toHaveValue('');
  await page.keyboard.press('Escape');
  await expect(page.getByLabel('Режим списка')).toHaveCount(0);
  const standalone = page
    .locator('article')
    .filter({ has: page.getByRole('button', { name: 'ЭЦП', exact: true }) });
  await expect(standalone.locator('mark')).toHaveText('ЭЦП');
  await standalone.getByRole('link', { name: 'Открыть в тексте' }).click();
  await expect(page.locator('mark[data-candidate]')).toHaveText('ЭЦП');
  const stale = new URL(page.url());
  stale.searchParams.set('version', '99');
  await page.goto(stale.toString());
  await expect(page.getByText(/Требование изменилось/)).toBeVisible();
  await expect(page.locator('mark[data-candidate]')).toHaveCount(0);
  await page.getByRole('link', { name: 'Словарь', exact: true }).first().click();
  await standalone.getByRole('button', { name: 'ЭЦП', exact: true }).click();
  await expect(page.getByLabel('Название понятия')).toHaveValue('');
  await expect(page.getByLabel('Найти существующее понятие для варианта')).toBeVisible();
  await page.keyboard.press('Escape');
  await page.getByRole('button', { name: 'Генеральный подрядчик (ГП)', exact: true }).click();
  await expect(page.getByRole('dialog')).toBeVisible();
  await expect(page.getByLabel('Название понятия')).toHaveValue('Генеральный подрядчик');
  await page.getByLabel('Определение', { exact: true }).fill('Исполнитель работ');
  await page.getByRole('button', { name: 'Подтвердить и сохранить' }).click();
  await expect(
    page.getByRole('button', { name: 'Генеральный подрядчик', exact: true }),
  ).toBeVisible();
  await expect(page.getByRole('button', { name: 'ГП', exact: true })).toHaveCount(0);
  await page.getByRole('button', { name: 'Где упоминается' }).click();
  const mention = page.getByRole('dialog').getByRole('link', { name: /Генеральный подрядчик/ });
  await expect(mention).toHaveAttribute('href', /requirements\?requirement=/);
  await mention.click();
  const termButton = page
    .locator('article')
    .filter({ hasText: 'Генподрядчик' })
    .getByRole('button', { name: 'ГП', exact: true });
  await termButton.scrollIntoViewIfNeeded();
  const termPosition = await termButton.boundingBox();
  await termButton.hover();
  await expect(termButton).toHaveAttribute('aria-describedby', /.+/);
  const definitionId = await termButton.getAttribute('aria-describedby');
  const definition = page.locator(`[role="tooltip"][id="${definitionId}"]`);
  await expect(definition).toContainText('Исполнитель работ');
  expect(await termButton.boundingBox()).toEqual(termPosition);
  await page.keyboard.press('Escape');
  await expect(definition).toHaveCount(0);
  await termButton.focus();
  await expect(definition).toBeVisible();
  await termButton.click();
  await page.getByRole('button', { name: 'Изменить понятие', exact: true }).click();
  await expect(page.getByRole('dialog').getByLabel('Название понятия')).toHaveValue(
    'Генеральный подрядчик',
  );
  await page.keyboard.press('Escape');
  const original = page.locator('article.card p.user-content').filter({ hasText: 'Генподрядчик' });
  const before = await original.textContent();
  await original.evaluate((paragraph) => {
    const node = paragraph.lastChild;
    if (!node?.textContent) throw new Error('Missing original text');
    const start = node.textContent.indexOf('Генподрядчик');
    const selection = window.getSelection();
    selection?.setBaseAndExtent(node, start, node, start + 'Генподрядчик'.length);
  });
  const add = page.getByRole('button', { name: 'Добавить выделение в словарь' });
  await expect(add).toBeVisible();
  await add.focus();
  await page.keyboard.press('Enter');
  await page.getByLabel('Найти существующее понятие для варианта').fill('подрядчик');
  await page
    .getByRole('form')
    .getByRole('button', { name: 'Генеральный подрядчик', exact: true })
    .click();
  await page.getByRole('button', { name: 'Подтвердить и сохранить' }).click();
  await expect(original.getByRole('button', { name: 'Генподрядчик', exact: true })).toBeVisible();
  await expect(original).toHaveText(before ?? '');
});
