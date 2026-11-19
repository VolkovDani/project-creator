import { expect, test } from '@playwright/test';

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
  await page.getByLabel('Текст требования').fill('Требование один');
  await page.getByRole('button', { name: 'Добавить требование' }).click();
  await expect(page.getByText('Требование один', { exact: true })).toBeVisible();
  await page.reload();
  await expect(page.getByText('Требование один', { exact: true })).toBeVisible();
  const other = await page.context().newPage();
  await other.goto(page.url());
  await other.getByRole('button', { name: 'Изменить' }).click();
  await other.getByRole('dialog').getByLabel('Текст требования').fill('Мой черновик');
  await page.getByRole('button', { name: 'Изменить' }).click();
  await page.getByRole('dialog').getByLabel('Текст требования').fill('Требование два');
  await page.getByRole('dialog').getByRole('button', { name: 'Сохранить' }).click();
  await expect(page.getByText('Версия 2')).toBeVisible();
  await other.getByRole('dialog').getByRole('button', { name: 'Сохранить' }).click();
  await expect(
    other.getByRole('dialog').getByText('Требование изменилось или недоступно. Обновите данные.'),
  ).toBeVisible();
  await expect(other.getByRole('dialog').getByLabel('Текст требования')).toHaveValue(
    'Мой черновик',
  );
  await other
    .getByRole('dialog')
    .getByRole('button', { name: 'Сохранить черновик поверх актуальной версии' })
    .click();
  await other.getByRole('dialog').getByRole('button', { name: 'Сохранить', exact: true }).click();
  await expect(other.getByText('Версия 3')).toBeVisible();
  await page.reload();
  await expect(page.getByText('Мой черновик', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Удалить' }).click();
  await expect(page.getByText('Требований пока нет')).toBeVisible();
  await other.close();
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
  await watcher.goto('/');
  const name = `Live ${Date.now()}`;
  await page.getByRole('link', { name: 'Создать проект', exact: true }).click();
  await page.getByLabel('Название').fill(name);
  await page.getByLabel('Цель').fill('SSE');
  await page.getByRole('button', { name: 'Создать проект' }).click();
  await expect(watcher.getByRole('link', { name })).toBeVisible({ timeout: 15000 });
  await watcher.getByRole('link', { name }).click();
  await page.getByRole('link', { name: 'Требования', exact: true }).first().click();
  await watcher.getByRole('link', { name: 'Требования', exact: true }).first().click();
  await page.getByLabel('Текст требования').fill('Live requirement');
  await page.getByRole('button', { name: 'Добавить требование' }).click();
  await expect(watcher.getByText('Live requirement', { exact: true })).toBeVisible({
    timeout: 15000,
  });
  await page.getByRole('button', { name: 'Изменить' }).click();
  await page.getByRole('dialog').getByLabel('Текст требования').fill('Changed in another tab');
  await page.getByRole('dialog').getByRole('button', { name: 'Сохранить', exact: true }).click();
  await expect(watcher.getByText('Changed in another tab', { exact: true })).toBeVisible({
    timeout: 15000,
  });
  await page.getByRole('button', { name: 'Удалить' }).click();
  await expect(watcher.getByText('Требований пока нет')).toBeVisible({ timeout: 15000 });
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
  await page
    .getByLabel('Текст требования')
    .fill('Генеральный подрядчик (ГП), также Генподрядчик, проверяет документы.');
  await page.getByRole('button', { name: 'Добавить требование' }).click();
  await page.getByRole('link', { name: 'Словарь проекта', exact: true }).first().click();
  await expect(page.getByRole('heading', { name: 'Словарь проекта', exact: true })).toBeVisible();
  await expect(page.getByLabel('Режим списка')).toHaveValue('infinite');
  await page.getByLabel('Режим списка').selectOption('pages');
  await expect(page.getByText('Страница 1', { exact: true })).toHaveCount(2);
  await page.getByLabel('Режим списка').selectOption('infinite');
  await page.getByRole('button', { name: 'Генеральный подрядчик (ГП)', exact: true }).click();
  await expect(page.getByRole('dialog')).toBeVisible();
  await expect(page.getByLabel('Название понятия')).toHaveValue('Генеральный подрядчик');
  await page.getByLabel('Определение', { exact: true }).fill('Исполнитель работ');
  await page.getByRole('button', { name: 'Подтвердить и сохранить' }).click();
  await expect(
    page.getByRole('button', { name: 'Генеральный подрядчик', exact: true }),
  ).toBeVisible();
  await page.getByRole('button', { name: 'Где упоминается' }).click();
  const mention = page.getByRole('dialog').getByRole('link', { name: /Генеральный подрядчик/ });
  await expect(mention).toHaveAttribute('href', /requirements\?requirement=/);
  await mention.click();
  const termButton = page.getByRole('button', { name: 'ГП', exact: true });
  await termButton.scrollIntoViewIfNeeded();
  const termPosition = await termButton.boundingBox();
  await termButton.hover();
  const definition = page.getByRole('tooltip');
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
