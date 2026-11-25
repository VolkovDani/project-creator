import { expect, test } from '@playwright/test';

test('a saved manual classification survives reload and becomes stale when its source changes', async ({
  page,
  context,
}) => {
  const name = `Проверка классификации ${Date.now()}`;
  const original = 'Откройте список заявок. Ожидаемый результат: в списке три заявки.';
  const edited =
    'Откройте пустой список заявок. Ожидаемый результат: показано сообщение «Нет заявок».';
  await page.goto('/projects/new');
  await page.getByLabel('Название').fill(name);
  await page.getByLabel('Цель').fill('Проверить поведение списка заявок');
  await page.getByRole('button', { name: 'Создать проект' }).click();
  await expect(page.getByRole('heading', { name, exact: true })).toBeVisible();

  const navigation = page.getByRole('navigation');
  await navigation.getByRole('link', { name: 'Требования', exact: true }).click();
  await page.getByRole('button', { name: 'Добавить требование', exact: true }).click();
  const create = page.getByRole('dialog', { name: 'Добавить требование', exact: true });
  await create.getByLabel('Текст требования').fill(original);
  await create.getByRole('button', { name: 'Добавить требование', exact: true }).click();
  await expect(create).toBeHidden();
  await expect(
    page.locator('#requirements-list').getByRole('button', { name: original, exact: true }),
  ).toBeVisible();

  await navigation.getByRole('link', { name: 'Классификация', exact: true }).click();
  const card = page.locator('[data-classification-id]').filter({ hasText: original });
  await expect(
    page.getByText('Автоматическая классификация отключена. Обе метки можно сохранить вручную.', {
      exact: true,
    }),
  ).toBeVisible();
  await card.getByRole('button', { name: 'Проверить классификацию' }).click();
  const dialog = page.getByRole('dialog');
  await dialog
    .getByRole('combobox', { name: 'Смысл', exact: true })
    .selectOption('functional_requirement');
  await dialog.getByRole('combobox', { name: 'Форма', exact: true }).selectOption('test_case');
  await dialog.getByRole('button', { name: 'Сохранить классификацию' }).click();
  await expect(dialog).toHaveCount(0);
  await expect(card).toContainText('Подтверждено пользователем');
  await page.reload();
  await expect(card).toContainText('Подтверждено пользователем');
  await expect(card).toContainText('Функциональное требование');
  await expect(card).toContainText('Тест-кейс');

  const recordId = await card.getAttribute('data-classification-id');
  const review = await context.newPage();
  await review.goto(page.url());
  const liveCard = review
    .locator('[data-classification-id]')
    .filter({ hasText: 'Подтверждено пользователем' });
  await expect(liveCard).toContainText(original);

  await card.getByRole('link', { name: 'Открыть требование' }).click();
  await expect(page).toHaveURL(
    (url) =>
      url.pathname.endsWith('/requirements') && url.searchParams.get('requirement') === recordId,
  );
  await expect(page.getByText(original, { exact: true }).first()).toBeVisible();
  await page.getByRole('button', { name: 'Изменить', exact: true }).click();
  const editor = page.getByRole('dialog');
  await editor.getByLabel('Текст требования').fill(edited);
  await editor.getByLabel('Причина изменения').fill('Уточнили условие классификации.');
  await editor.getByRole('button', { name: 'Сохранить', exact: true }).click();
  await expect(page.getByText(edited, { exact: true }).first()).toBeVisible();

  const stale = review.locator('[data-classification-id]').filter({ hasText: edited });
  await expect(stale).toContainText('Устарело');
  await expect(stale).toContainText(
    'Решение относится к версии текста 1. Текущая версия текста — 2.',
  );
  await stale.getByText('История решений', { exact: true }).click();
  const savedRevision = stale
    .getByRole('listitem')
    .filter({ hasText: 'Версия текста 1, ревизия решения 1' });
  await expect(savedRevision).toContainText('Подтверждено пользователем');
  await expect(savedRevision).toContainText('Тест-кейс');
  await review.getByRole('combobox', { name: 'Язык' }).selectOption('en');
  await expect(stale).toContainText('Decision for text version 1. The current text is version 2.');
  await expect(stale.getByRole('listitem')).toContainText('Test case');
  await expect(stale.getByText(edited, { exact: true })).toBeVisible();
  await review.close();
});
