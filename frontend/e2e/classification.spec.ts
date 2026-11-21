import { expect, type Page, test } from '@playwright/test';

type PushWindow = Window & { pushClassification: (keys: string[]) => void };
async function fixture(page: Page, configured = true, olderDecisions = 0) {
  await page.addInitScript(() => {
    const fetch = window.fetch;
    let serial = 0;
    window.fetch = (input, init) => {
      if (String(input) !== '/api/events') return fetch(input, init);
      return Promise.resolve(
        new Response(
          new ReadableStream<Uint8Array>({
            start(controller) {
              (window as unknown as PushWindow).pushClassification = (keys) =>
                controller.enqueue(
                  new TextEncoder().encode(
                    `id: c-${++serial}\nevent: cache.invalidate\ndata: ${JSON.stringify({ schemaVersion: 1, keys })}\n\n`,
                  ),
                );
            },
          }),
          { headers: { 'content-type': 'text/event-stream' } },
        ),
      );
    };
  });
  const records = Array.from({ length: 205 }, (_, i) => ({
    projectId: 'demo',
    requirementId: String(i + 1),
    text: 'Исходная запись ' + (i + 1),
    textVersion: 1,
    revision: 1,
    status: i === 1 ? 'failed' : 'needs_review',
    semanticType: 'unknown',
    representationType: 'unknown',
    source: 'model',
    model: 'test-model:q4',
    digest: 'sha256:test',
    profile: 'profile-a',
    questionVersion: 'flat-v1',
    rulesVersion: '2',
    reason: 'uncalibrated',
    errorCode: i === 1 ? 'CLASSIFIER_UNAVAILABLE' : '',
    createdAt: '2026-11-19',
    alternatives: [
      { semanticType: 'business_rule', representationType: '', score: 0.61 },
      { semanticType: '', representationType: 'test_case', score: 0.88 },
    ],
  }));
  let allowed = true;
  const reads: string[] = [];
  const saves: unknown[] = [];
  const actions: string[] = [];
  await page.route('**/api/graphql', async (route) => {
    const { operationName: name, variables: v } = route.request().postDataJSON();
    if (!allowed) {
      await route.fulfill({ status: 403, json: { errors: [{ message: 'forbidden' }] } });
      return;
    }
    let data: unknown;
    if (name === 'ProjectDetail')
      data = {
        project: { id: 'demo', name: 'Демонстрация', goal: '', description: '' },
        analysisDefaultThreshold: 0.72,
      };
    else if (name === 'ClassificationProgress')
      data = {
        classificationProgress: {
          projectId: 'demo',
          total: records.length,
          unclassified: 0,
          queued: 0,
          classifying: 0,
          suggested: 0,
          needsReview: records.filter((r) => r.status === 'needs_review').length,
          confirmed: 0,
          failed: 1,
          stale: records.filter((r) => r.status === 'stale').length,
          configured,
          profile: 'profile-a',
        },
      };
    else if (name === 'ClassificationGroups')
      data = {
        classificationGroups: [
          { semanticType: 'unknown', representationType: 'unknown', count: records.length },
        ],
      };
    else if (name === 'ClassificationPageIds')
      data = {
        classificationList: records
          .filter(
            (r) =>
              (!v.status || r.status === v.status) &&
              (!v.semanticType || r.semanticType === v.semanticType) &&
              (!v.representationType || r.representationType === v.representationType),
          )
          .slice(v.offset, v.offset + v.limit)
          .map(({ requirementId }) => ({ requirementId })),
      };
    else if (name === 'ClassificationReviewCard') {
      reads.push(v.requirementId);
      const r = records.find((r) => r.requirementId === v.requirementId);
      data = {
        requirementClassification: r,
        classificationListByIds: r ? [{ text: r.text, textVersion: r.textVersion }] : [],
      };
    } else if (name === 'RequirementClassification') {
      data = {
        requirementClassification: records.find((r) => r.requirementId === v.requirementId) ?? null,
      };
    } else if (name === 'ConfirmClassification') {
      saves.push(v.input);
      const r = records.find((r) => r.requirementId === v.input.requirementId);
      if (!r) throw Error('missing requirement');
      Object.assign(r, {
        ...v.input,
        revision: r.revision + 1,
        status: 'confirmed',
        source: 'user',
        model: '',
        alternatives: [],
      });
      data = { confirmRequirementClassification: { classification: r, fieldErrors: [] } };
    } else if (name === 'ClassificationHistory') {
      const r = records.find((r) => r.requirementId === v.requirementId);
      data = {
        requirementClassificationHistory: r
          ? [
              {
                ...r,
                revision: olderDecisions + 2,
                source: 'model',
                status: 'needs_review',
                model: 'new-run',
                semanticType: 'unknown',
              },
              {
                ...r,
                revision: olderDecisions + 1,
                source: 'user',
                status: 'confirmed',
                model: '',
                semanticType: 'business_rule',
              },
              ...Array.from({ length: olderDecisions }, (_, i) => ({
                ...r,
                revision: olderDecisions - i,
              })),
            ].slice(v.offset, v.offset + v.limit)
          : [],
      };
    } else if (name === 'RetryClassification') {
      actions.push(name);
      data = { retryRequirementClassification: { queued: 1, fieldErrors: [] } };
    } else if (name === 'BackfillClassifications') {
      actions.push(name);
      data = { backfillClassifications: { queued: 1, fieldErrors: [] } };
    } else if (name === 'RetryFailedClassifications') {
      actions.push(name);
      data = { retryFailedClassifications: { queued: 1, fieldErrors: [] } };
    } else {
      throw Error('unexpected operation ' + name);
    }
    await route.fulfill({ json: { data } });
  });
  return {
    records,
    reads,
    saves,
    actions,
    revoke: () => {
      allowed = false;
    },
  };
}

test('classification updates one visible card in a 205-record project while retaining the open review, draft and focus', async ({
  page,
}) => {
  const { records, reads, saves } = await fixture(page);
  await page.goto('/projects/demo/classification');
  const first = page.locator('[data-classification-id="1"]');
  await expect(first).toContainText('test-model:q4');
  await first.getByRole('button', { name: 'Проверить классификацию' }).click();
  const dialog = page.getByRole('dialog');
  const select = dialog.getByRole('combobox', { name: 'Форма', exact: true });
  await select.selectOption('test_case');
  await select.focus();
  await expect.poll(() => new Set(reads).size).toBe(20);
  const before = reads.length;
  const scroll = await page.evaluate(() => window.scrollY);
  const record = records[0];
  if (!record) throw Error('missing fixture');
  record.revision++;
  await page.evaluate(() =>
    (window as unknown as PushWindow).pushClassification([
      'project:demo:classification:1',
      'project:demo:classifications',
    ]),
  );
  await expect(dialog.getByText(/изменились во время редактирования/)).toBeVisible();
  expect(await select.inputValue()).toBe('test_case');
  await expect(select).toBeFocused();
  expect(await page.evaluate(() => window.scrollY)).toBe(scroll);
  expect(new Set(reads.slice(before))).toEqual(new Set(['1']));
  expect(saves).toHaveLength(0);
  await dialog.getByRole('button', { name: 'Применить мой выбор к текущей версии' }).click();
  await dialog.getByRole('button', { name: 'Сохранить классификацию' }).click();
  await expect(dialog).toHaveCount(0);
  await expect(first).toContainText('Подтверждено пользователем');
  expect(saves).toMatchObject([
    { revision: 2, semanticType: 'unknown', representationType: 'test_case' },
  ]);
  await page.reload();
  await expect(first).toContainText('Подтверждено пользователем');
});

test('group filters visibly mark the pressed choice after focus leaves', async ({ page }) => {
  await fixture(page);
  await page.goto('/projects/demo/classification');
  await page.getByText('Группы и количество записей', { exact: true }).click();
  const filters = page.locator('.classification-groups button');
  const first = filters.first();
  await first.click();
  await page.keyboard.press('Tab');
  await page.mouse.move(0, 0);
  await expect(first).toHaveAttribute('aria-pressed', 'true');
  await expect(first).not.toHaveCSS(
    'font-weight',
    await filters.nth(1).evaluate((el) => getComputedStyle(el).fontWeight),
  );
});

test('manual review remains available when automatic classification is disabled', async ({
  page,
}) => {
  await fixture(page, false);
  await page.goto('/projects/demo/classification');
  await expect(
    page.getByText('Автоматическая классификация отключена. Обе метки можно сохранить вручную.', {
      exact: true,
    }),
  ).toBeVisible();
  await expect(
    page.getByRole('button', { name: 'Повторить неудачные классификации' }),
  ).toBeDisabled();
  await page.getByRole('combobox', { name: 'Статус', exact: true }).selectOption('failed');
  await expect(page.locator('[data-classification-id]')).toHaveCount(1);
  await expect(page.locator('[data-classification-id="2"]')).toContainText(
    'CLASSIFIER_UNAVAILABLE',
  );
  await page.getByRole('button', { name: 'Проверить классификацию' }).click();
  await expect(
    page.getByRole('dialog').getByRole('button', { name: 'Сохранить классификацию' }),
  ).toBeEnabled();
});

test('reanalysis keeps a confirmed decision visible and exposes scoped model evidence in history', async ({
  page,
}) => {
  const { records, actions } = await fixture(page);
  const entry = records[0];
  if (!entry) throw Error('missing fixture');
  Object.assign(entry, {
    status: 'confirmed',
    source: 'user',
    semanticType: 'business_rule',
    model: '',
  });
  await page.goto('/projects/demo/classification');
  const card = page.locator('[data-classification-id="1"]');
  await card.getByRole('button', { name: 'Повторить анализ с сохранением решения' }).click();
  await expect(card.getByRole('status')).toContainText('1');
  expect(actions).toEqual(['RetryClassification']);
  await card.getByText('История решений', { exact: true }).click();
  await expect(card.getByText('new-run', { exact: false })).toBeVisible();
  await expect(card.getByText('Подтверждено пользователем').first()).toBeVisible();
  await card
    .getByText(/Альтернативы модели/)
    .first()
    .click();
  await expect(card.getByText(/Некалиброванная оценка модели/).first()).toBeVisible();
});

test('backfills stale records when no unclassified records remain and retries only failed jobs', async ({
  page,
}) => {
  const { records, actions } = await fixture(page);
  const entry = records[0];
  if (!entry) throw Error('missing fixture');
  entry.status = 'stale';
  await page.goto('/projects/demo/classification');
  const backfill = page.getByRole('button', { name: 'Классифицировать необработанные записи' });
  await expect(backfill).toBeEnabled({ timeout: 1500 });
  await backfill.click();
  await expect(page.getByRole('status')).toContainText('1');
  await page.getByRole('button', { name: 'Повторить неудачные классификации' }).click();
  await expect
    .poll(() => actions)
    .toEqual(['BackfillClassifications', 'RetryFailedClassifications']);
});

test('revoking project access clears an open draft, source text and history', async ({ page }) => {
  const { revoke } = await fixture(page);
  await page.goto('/projects/demo/classification');
  const card = page.locator('[data-classification-id="1"]');
  await card.getByText('История решений', { exact: true }).click();
  await expect(card.getByText('new-run', { exact: false })).toBeVisible();
  await card.getByRole('button', { name: 'Проверить классификацию' }).click();
  revoke();
  await page.evaluate(() =>
    (window as unknown as PushWindow).pushClassification(['project:demo:classification:1']),
  );
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(page.getByText('Исходная запись 1', { exact: true })).toHaveCount(0);
  await expect(page.getByText('new-run', { exact: false })).toHaveCount(0);
});

test('confirms only explicitly selected current model suggestions and reports a changed selection', async ({
  page,
}) => {
  const { records, saves } = await fixture(page);
  for (const index of [0, 1, 3]) {
    const r = records[index];
    if (!r) throw Error('missing fixture');
    Object.assign(r, {
      status: 'suggested',
      source: 'model',
      reason: '',
      semanticType: 'business_rule',
      representationType: 'statement',
      errorCode: '',
    });
  }
  await page.goto('/projects/demo/classification');
  const first = page.locator('[data-classification-id="1"]');
  const second = page.locator('[data-classification-id="2"]');
  const uncalibrated = page.locator('[data-classification-id="3"]');
  await expect(first.getByRole('checkbox')).toHaveCount(1, { timeout: 2000 });
  await expect(first.getByRole('checkbox')).not.toBeChecked();
  await expect(uncalibrated.getByRole('checkbox')).toHaveCount(0);
  await first.getByRole('checkbox').check();
  await second.getByRole('checkbox').check();
  const changed = records[1];
  if (!changed) throw Error('missing fixture');
  changed.revision++;
  await page.evaluate(() =>
    (window as unknown as PushWindow).pushClassification([
      'project:demo:classification:2',
      'project:demo:classifications',
    ]),
  );
  await page.getByRole('button', { name: 'Подтвердить выбранные (2)', exact: true }).click();
  await expect(
    page.getByText('Подтверждено: 1. Требуют повторной проверки: 1. Ошибки: 0.', { exact: true }),
  ).toBeVisible();
  await expect(page.getByText('2 · Предложение изменилось', { exact: true })).toBeVisible();
  expect(saves).toMatchObject([
    {
      requirementId: '1',
      revision: 1,
      textVersion: 1,
      semanticType: 'business_rule',
      representationType: 'statement',
    },
  ]);
});

test('scrolling appends classification records and preserves selections across batches', async ({
  page,
}) => {
  const { records, saves } = await fixture(page);
  records.splice(25);
  for (const index of [0, 24]) {
    const record = records[index];
    if (!record) throw Error('missing fixture');
    Object.assign(record, {
      status: 'suggested',
      reason: '',
      semanticType: 'business_rule',
      representationType: 'statement',
    });
  }
  await page.goto('/projects/demo/classification');
  const cards = page.locator('[data-classification-id]');
  await expect(cards).toHaveCount(20);
  await cards.first().getByRole('checkbox').check();
  await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
  await expect(cards).toHaveCount(25);
  await expect(cards.first().getByRole('checkbox')).toBeChecked();
  await cards.last().getByRole('checkbox').check();
  await page.getByRole('button', { name: 'Подтвердить выбранные (2)' }).click();
  await expect.poll(() => saves.length).toBe(2);
  expect(saves).toMatchObject([{ requirementId: '1' }, { requirementId: '25' }]);
  await expect(page.getByRole('button', { name: 'Загрузить ещё' })).toHaveCount(0);
  await page.getByRole('combobox', { name: 'Статус', exact: true }).selectOption('failed');
  await expect(cards).toHaveCount(1);
  await expect(cards.first()).toHaveAttribute('data-classification-id', '2');
});

test('scrolling inside an open decision history appends the remaining revisions', async ({
  page,
}) => {
  await fixture(page, true, 11);
  await page.goto('/projects/demo/classification');
  const card = page.locator('[data-classification-id="1"]');
  await card.getByText('История решений', { exact: true }).click();
  const revisions = card.locator('details[aria-label="История решений"] > ol > li');
  await expect(revisions).toHaveCount(10);
  await card
    .getByRole('button', { name: 'Загрузить ещё' })
    .scrollIntoViewIfNeeded({ timeout: 1500 });
  await expect(revisions).toHaveCount(13);
  await expect(revisions.first()).toContainText('ревизия решения 13');
  await expect(revisions.last()).toContainText('ревизия решения 1');
  await expect(card.getByRole('button', { name: 'Загрузить ещё' })).toHaveCount(0);
});
