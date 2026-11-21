import { expect, type Page, test } from '@playwright/test';
import { expectLayout, layoutCases, setLayout } from './layout-assertions';

test.afterEach(async ({ page }, testInfo) => {
  if (testInfo.status === 'passed') await expectLayout(page);
});

test('section metadata follows navigation and the saved language', async ({ page }) => {
  await fixture(page, 1);
  await page.goto('/projects/demo/classification');
  const description = page.locator('head meta[name="description"]');
  await expect(page).toHaveTitle('Классификация — Project Creator');
  await expect(description).toHaveAttribute('content', /классификацию/);
  await page.getByLabel('Язык').selectOption('en');
  await expect(page).toHaveTitle('Classification — Project Creator');
  await expect(description).toHaveAttribute('content', /classifications/);
  await page.getByRole('link', { name: 'Indexing', exact: true }).click();
  await expect(page).toHaveTitle('Indexing — Project Creator');
  await expect(description).toHaveAttribute('content', /indexing/);
  await page.reload();
  await expect(page).toHaveTitle('Indexing — Project Creator');
  await expect(page.locator('head title')).toHaveCount(1);
});

test('pages advertise the browser and Apple icons and serve the supplied sizes', async ({
  page,
  request,
}) => {
  await fixture(page, 1);
  await page.goto('/projects/demo/indexing');
  const tabIcons = page.locator('head link[rel="icon"]');
  await expect(tabIcons).toHaveCount(3);
  for (const size of [16, 32]) {
    await expect(page.locator(`head link[rel="icon"][sizes="${size}x${size}"]`)).toHaveAttribute(
      'href',
      `/favicon-${size}x${size}.png`,
    );
  }
  await expect(page.locator('head link[rel="icon"][href^="/favicon.ico"]')).toHaveCount(1);
  await expect(page.locator('head link[rel="apple-touch-icon"][sizes="180x180"]')).toHaveAttribute(
    'href',
    '/apple-touch-icon.png',
  );
  const icon = await request.get('/favicon.ico');
  expect(icon.status()).toBe(200);
  expect(icon.headers()['content-type']).toMatch(/image\/(x-icon|vnd.microsoft.icon)/);
  const ico = await icon.body();
  expect([...ico.subarray(0, 6)]).toEqual([0, 0, 1, 0, 3, 0]);
  expect([ico[6], ico[22], ico[38]]).toEqual([16, 32, 48]);
  for (const [file, size] of [
    ['favicon-16x16', 16],
    ['favicon-32x32', 32],
    ['favicon-48x48', 48],
    ['favicon-64x64', 64],
    ['apple-touch-icon', 180],
    ['icon-192', 192],
    ['icon-512', 512],
    ['icon-maskable-512', 512],
  ] as const) {
    const image = await request.get(`/${file}.png`);
    expect(image.status()).toBe(200);
    expect(image.headers()['content-type']).toContain('image/png');
    const png = await image.body();
    expect([png.readUInt32BE(16), png.readUInt32BE(20)]).toEqual([size, size]);
  }
});

test('dark theme follows the user across navigation, language changes and reloads', async ({
  page,
}) => {
  await fixture(page, 1);
  await page.goto('/projects/new');
  const language = page.getByRole('combobox', { name: 'Язык' });
  const theme = page.getByRole('button', { name: 'Тёмная тема' });
  await expect(theme).toHaveAttribute('aria-pressed', 'false');
  await language.focus();
  await page.keyboard.press('Tab');
  await expect(theme).toBeFocused();
  await page.keyboard.press('Space');
  await expect(theme).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator('html')).toHaveCSS('background-color', 'rgb(27, 28, 28)');
  await expect(page).toHaveURL(/\/projects\/new$/);
  await language.selectOption('en');
  const englishTheme = page.getByRole('button', { name: 'Dark theme' });
  await expect(englishTheme).toHaveAttribute('aria-pressed', 'true');
  await page.locator('.top a[href="/"]').click();
  await expect(englishTheme).toHaveAttribute('aria-pressed', 'true');
  await page.reload();
  await expect(englishTheme).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator('html')).toHaveCSS('color-scheme', 'dark');
  await englishTheme.click();
  await page.reload();
  await expect(englishTheme).toHaveAttribute('aria-pressed', 'false');
  await expect(page.locator('html')).toHaveCSS('color-scheme', 'light');
});

test('requirement editing opens a native dialog and Escape discards the draft', async ({
  page,
}) => {
  await fixture(page, 21);
  await page.goto('/projects/demo/requirements');
  const addDraft = page.getByLabel('Текст требования');
  await addDraft.fill('Черновик добавления');
  const edit = page
    .locator('#requirement-r0')
    .getByRole('button', { name: 'Изменить', exact: true });
  await edit.click();
  const dialog = page.getByRole('dialog');
  await expect(dialog).toBeVisible();
  const editor = dialog.getByLabel('Текст требования');
  await editor.fill('Черновик');
  const scrollBefore = await page.evaluate(() => window.scrollY);
  await page.mouse.click(5, 5);
  await expect(dialog).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(dialog).not.toBeVisible();
  await expect(addDraft).toHaveValue('Черновик добавления');
  await expect(edit).toBeFocused();
  expect(await page.evaluate(() => window.scrollY)).toBe(scrollBefore);
  await expect(
    page.locator('#requirement-r0').getByText('Требование 0', { exact: true }).first(),
  ).toBeVisible();
});

test('requirement edit conflict keeps the draft and saving closes the dialog', async ({ page }) => {
  const api = await fixture(page, 3);
  await page.goto('/projects/demo/requirements');
  await page.getByLabel('Текст требования').fill('Черновик добавления');
  await page.getByRole('button', { name: 'Изменить', exact: true }).first().click();
  const dialog = page.getByRole('dialog');
  const editor = dialog.getByLabel('Текст требования');
  await editor.fill('Новая редакция');
  api.setText('Сохранённая другая редакция');
  api.conflictNextEdit();
  await dialog.getByRole('button', { name: 'Сохранить', exact: true }).click();
  await expect(dialog.getByRole('alert')).toBeVisible();
  await expect(editor).toHaveValue('Новая редакция');
  await dialog.getByRole('button', { name: 'Сохранить черновик поверх актуальной версии' }).click();
  await dialog.getByRole('button', { name: 'Сохранить', exact: true }).click();
  await expect(dialog).not.toBeVisible();
  expect(api.editWrites()).toEqual([{ id: 'r0', text: 'Новая редакция', version: 2 }]);
  await expect(page.getByLabel('Текст требования')).toHaveValue('Черновик добавления');
});

test('a busy requirement dialog rejects Escape and keeps network errors visible', async ({
  page,
}) => {
  const api = await fixture(page, 3);
  const request = api.delayEditFailure();
  await page.goto('/projects/demo/requirements');
  await page.getByRole('button', { name: 'Изменить', exact: true }).first().click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Текст требования').fill('Busy edit');
  const save = dialog.getByRole('button', { name: 'Сохранить', exact: true });
  await save.click();
  await request.started;
  await expect(save).toBeDisabled();
  await page.keyboard.press('Escape');
  await expect(dialog).toBeVisible();
  request.release();
  await expect(dialog.getByRole('alert')).toBeVisible();
  await expect(dialog).toBeVisible();
});

test('global back-to-top appears after scrolling and returns focus to the page heading', async ({
  page,
}) => {
  await fixture(page, 21);
  await page.goto('/projects/demo/requirements');
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
  await page.evaluate(() => {
    document.body.style.minHeight = '2000px';
    window.scrollTo(0, 900);
  });
  const button = page.getByRole('button', { name: 'Наверх', exact: true });
  await expect(button).toBeVisible();
  await button.click();
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBeLessThan(10);
  await expect(page.getByRole('heading', { level: 1 })).toBeFocused();
});

test('global back-to-top works outside project pages with reduced motion and English labels', async ({
  page,
}) => {
  await fixture(page, 1);
  await page.goto('/projects/new');
  await page.getByLabel('Язык').selectOption('en');
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.evaluate(() => {
    document.body.style.minHeight = '2000px';
    window.scrollTo(0, 700);
  });
  const button = page.getByRole('button', { name: 'Back to top', exact: true });
  await expect(button).toBeVisible();
  await button.click();
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBeLessThan(10);
  await expect(page.getByRole('heading', { name: 'Create project' })).toBeFocused();
});

const project = {
  id: 'demo',
  name: 'Стабильный проект',
  goal: 'Проверить обновления',
  description: 'Описание проекта',
  createdAt: 'now',
  updatedAt: 'now',
};
const requirement = (id: number) => ({
  id: `r${id}`,
  projectId: 'demo',
  text: `Требование ${id}`,
  version: 1,
  createdAt: 'now',
  updatedAt: 'now',
});

async function fixture(
  page: Page,
  count = 21,
  sourceSuffix = '',
  projectValue = project,
  summaryText?: string,
  maxBatchSize = Number.POSITIVE_INFINITY,
) {
  const rows = Array.from({ length: count }, (_, i) => ({
    ...requirement(i),
    text: requirement(i).text + sourceSuffix,
  }));
  const waiting = new Set<(body: string) => void>();
  let delay: Promise<void> | undefined;
  let delayAfterFirst = false;
  let failure = 0;
  let nextEditConflict = false;
  let editFailureStatus = 0;
  let editGate: Promise<void> | undefined;
  let editStarted: (() => void) | undefined;
  let indexFailure = 0;
  let connections = 0;
  let reads = 0;
  let progress = {
    projectId: 'demo',
    total: count,
    queued: Math.max(0, count - 2),
    running: 0,
    ready: Math.min(1, count),
    failed: count > 1 ? 1 : 0,
    profile: 'fixture',
    configured: true,
  };
  const connectionReads: Array<{ first: number; after: string | null }> = [];
  const indexReads: string[][] = [];
  const editWrites: Array<{ id: string; text: string; version: number }> = [];
  let indexes = rows.map((r, i) => ({
    requirementId: r.id,
    version: 1,
    status: i === 0 ? 'ready' : i === 1 ? 'failed' : 'pending',
    profile: 'fixture',
    errorCode: '',
  }));
  await page.route('**/api/events', async (route) => {
    connections++;
    const body = await new Promise<string>((resolve) => waiting.add(resolve));
    await route.fulfill({ status: 200, contentType: 'text/event-stream', body }).catch(() => {});
  });
  await page.route('**/api/graphql', async (route) => {
    const { query, variables } = route.request().postDataJSON();
    let data: unknown;
    if (query.includes('updateRequirement(') && editGate) {
      editStarted?.();
      await editGate;
      editGate = undefined;
    }
    if (query.includes('updateRequirement(') && editFailureStatus) {
      await route.fulfill({ status: editFailureStatus, json: { error: 'edit failure' } });
      editFailureStatus = 0;
      return;
    }
    if (query.includes('requirementIndexProgress(')) {
      if (failure) {
        await route.fulfill({ status: failure, json: { error: 'fixture failure' } });
        return;
      }
      data = { requirementIndexProgress: progress };
    } else if (query.includes('updateRequirement(')) {
      const input = variables.input;
      if (nextEditConflict) {
        nextEditConflict = false;
        data = {
          updateRequirement: {
            requirement: null,
            fieldErrors: [
              { path: ['text'], code: 'CONFLICT', messageKey: 'validation.conflict', params: [] },
            ],
          },
        };
      } else {
        const index = rows.findIndex((row) => row.id === input.id);
        editWrites.push({ id: input.id, text: input.text, version: input.version });
        rows[index] = { ...rows[index], text: input.text, version: rows[index].version + 1 };
        data = {
          updateRequirement: {
            requirement: { id: input.id, text: input.text, version: rows[index].version },
            fieldErrors: [],
          },
        };
      }
    } else if (query.includes('requirementSummariesByIds(')) {
      data = {
        requirementSummariesByIds: rows
          .filter((row) => variables.requirementIds.includes(row.id))
          .map((row) => ({
            requirementId: row.id,
            projectId: row.projectId,
            version: row.version,
            status: 'ready',
            text:
              summaryText ?? (sourceSuffix ? row.text.slice(0, -sourceSuffix.length) : row.text),
            profile: 'fixture-summary-profile',
            provenance: 'fixture-summary-origin',
            createdAt: 'now',
            errorCode: '',
          })),
      };
    } else if (query.includes('requirementsConnection(')) {
      reads++;
      connectionReads.push({ first: Number(variables.first), after: variables.after ?? null });
      if (delay && (!delayAfterFirst || variables.after)) await delay;
      if (failure) {
        await route.fulfill({ status: failure, json: { error: 'fixture failure' } });
        return;
      }
      const start = Number(variables.after ? String(variables.after).replace('cursor-', '') : 0);
      const batch = rows.slice(start, start + Math.min(Number(variables.first), maxBatchSize));
      data = {
        requirementsConnection: {
          edges: batch.map((node, index) => ({ cursor: `cursor-${start + index + 1}`, node })),
          pageInfo: {
            endCursor: batch.length ? `cursor-${start + batch.length}` : null,
            hasNextPage: start + batch.length < rows.length,
          },
          totalCount: rows.length,
        },
      };
    } else if (query.includes('requirementIndexByIds(')) {
      indexReads.push(variables.requirementIds);
      if (failure || indexFailure) {
        await route.fulfill({
          status: failure || indexFailure,
          json: { error: 'fixture failure' },
        });
        return;
      }
      data = {
        requirementIndexByIds: indexes
          .filter((entry) => variables.requirementIds.includes(entry.requirementId))
          .reverse(),
      };
    } else if (query.includes('requirementIndex('))
      data = {
        requirementIndex: indexes.slice(variables.offset, variables.offset + variables.limit),
      };
    else if (query.includes('searchRequirements('))
      data = {
        searchRequirements: [
          {
            requirementId: 'r20',
            projectId: 'demo',
            text: 'Результат поиска',
            sourceText: rows[20]?.text || rows[0]?.text || 'Требование 20',
            version: 1,
            start: 0,
            end: 12,
            score: 1,
          },
        ],
      };
    else if (query.includes('analysisHistory(')) data = { analysisHistory: [] };
    else if (query.includes('projectAnalysis(')) data = { projectAnalysis: null };
    else if (query.includes('projectsConnection('))
      data = {
        projectsConnection: {
          edges: [{ cursor: 'project-1', node: projectValue }],
          pageInfo: { endCursor: 'project-1', hasNextPage: false },
          totalCount: 1,
        },
      };
    else if (query.includes('projects(')) data = { projects: [projectValue] };
    else data = { project: projectValue, analysisDefaultThreshold: 0.65 };
    await route.fulfill({ json: { data } }).catch(() => {});
  });
  return {
    setProgress: (value: Partial<typeof progress>) => {
      progress = { ...progress, ...value };
    },
    reads: () => reads,
    connectionReads: () => connectionReads,
    indexReads: () => indexReads,
    editWrites: () => editWrites,
    connections: () => connections,
    fail: (status: number) => {
      failure = status;
    },
    failIndex: (status: number) => {
      indexFailure = status;
    },
    delay: (afterFirst = false) => {
      delayAfterFirst = afterFirst;
      let release!: () => void;
      delay = new Promise<void>((resolve) => (release = resolve));
      return () => {
        delay = undefined;
        release();
      };
    },
    setText: (text: string) => {
      rows[0] = { ...rows[0], text, version: rows[0].version + 1 };
    },
    conflictNextEdit: () => {
      nextEditConflict = true;
    },
    delayEditFailure: () => {
      let release!: () => void;
      let started!: () => void;
      editGate = new Promise<void>((resolve) => (release = resolve));
      const sent = new Promise<void>((resolve) => (started = resolve));
      editStarted = started;
      editFailureStatus = 500;
      return { started: sent, release };
    },
    setReady: () => {
      indexes = indexes.map((x) => ({ ...x, status: 'ready' }));
    },
    setRunning: () => {
      indexes[0] = { ...indexes[0], status: 'running' };
    },
    async event(reset = false) {
      await expect.poll(() => waiting.size).toBeGreaterThan(0);
      const body = `id: ${Date.now()}\nevent: ${reset ? 'cache.reset' : 'cache.invalidate'}\ndata: ${JSON.stringify({ schemaVersion: 1, keys: ['projects', 'project:demo', 'project:demo:requirements'] })}\n\n`;
      for (const resolve of waiting) resolve(body);
      waiting.clear();
    },
  };
}

test('index queue shows project-wide progress and running state after SSE and reset', async ({
  page,
}) => {
  const api = await fixture(page, 45);
  api.setProgress({ total: 200, ready: 120, queued: 78, running: 1, failed: 1 });
  api.setRunning();
  const cdp = await page.context().newCDPSession(page);
  await cdp.send('Network.enable');
  let eventsPriority: string | undefined;
  cdp.on('Network.requestWillBeSent', ({ request }) => {
    if (new URL(request.url).pathname === '/api/events') eventsPriority = request.initialPriority;
  });
  await page.goto('/projects/demo/indexing');
  await expect.poll(() => eventsPriority).toBe('Low');
  const summary = page.getByRole('region', { name: 'Прогресс индексации проекта', exact: true });
  await expect(summary).toContainText('Готово: 120 из 200');
  await expect(summary).toContainText('В работе: 1');
  await expect(
    page
      .getByRole('group', { name: 'индексируется', exact: true })
      .getByText('Требование 0', { exact: true }),
  ).toBeVisible();
  api.setProgress({ ready: 200, queued: 0, running: 0, failed: 0 });
  api.setReady();
  await api.event();
  await expect(summary).toContainText('Готово: 200 из 200');
  await api.event(true);
  await expect(summary).toContainText('Готово: 200 из 200');
});

test('infinite requirements append on scroll and stop at the terminal page', async ({ page }) => {
  const api = await fixture(page, 85);
  await page.goto('/projects/demo/requirements');
  await expect(page.locator('#requirement-r0')).toBeVisible();
  await expect.poll(() => api.connectionReads().length).toBe(1);
  expect(api.connectionReads()[0]?.first).toBe(40);
  await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
  // Auto-loading can consume the terminal page before a manual click.
  // Keep scrolling until all pages arrive; test the button independently below.
  await expect
    .poll(async () => {
      await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
      return api.connectionReads().length;
    })
    .toBe(3);
  expect(api.connectionReads().map((read) => read.after)).toEqual([null, 'cursor-40', 'cursor-80']);
  await expect(page.getByRole('button', { name: 'Загрузить ещё', exact: true })).toHaveCount(0);
});

test('dynamic requirement cards keep the load button outside their content', async ({ page }) => {
  const api = await fixture(
    page,
    8,
    '\nДлинное условие с пояснениями и ограничениями.'.repeat(20),
    project,
    undefined,
    4,
  );
  // Hold continuation responses before navigation can trigger prefetch.
  const release = api.delay(true);
  await page.goto('/projects/demo/requirements');
  await expect(page.locator('#requirement-r0')).toBeVisible();
  const last = page.locator('#requirement-r3');
  const disclosure = last.getByText('Полный исходный текст требования', { exact: true });
  await disclosure.click();
  await expect(last.locator('details[open]')).toContainText('Длинное условие');
  const group = page.locator('details.data-group').filter({ has: last });
  const more = group.locator(':scope > button');
  async function expectCardAboveButton() {
    await expect(more).toBeVisible();
    await expect
      .poll(async () => {
        const card = await last.boundingBox();
        const button = await more.boundingBox();
        return !!card && !!button && card.y + card.height <= button.y;
      })
      .toBe(true);
  }
  await expectCardAboveButton();
  await expect
    .poll(async () => {
      await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
      return api.connectionReads().length;
    })
    .toBe(2);
  await expect(more).toBeDisabled();
  await expectCardAboveButton();
  release();
  await expect(more).toHaveCount(0);
  await expect(page.locator('#requirement-r7')).toBeAttached();
});

test('infinite page retry keeps the already rendered rows', async ({ page }) => {
  const api = await fixture(page, 100);
  await page.goto('/projects/demo/requirements');
  await expect(page.locator('#requirement-r0')).toBeVisible();
  await api.fail(503);
  await page.getByRole('button', { name: 'Загрузить ещё', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Повторить', exact: true })).toBeVisible();
  await page.evaluate(() => window.scrollTo(0, 0));
  await expect(page.locator('#requirement-r0')).toBeVisible();
  api.fail(0);
  await page.getByRole('button', { name: 'Повторить', exact: true }).click();
  await expect
    .poll(() =>
      api
        .connectionReads()
        .map((read) => read.after)
        .slice(0, 3),
    )
    .toEqual([null, 'cursor-40', 'cursor-40']);
  await expect(page.getByRole('button', { name: 'Повторить', exact: true })).toHaveCount(0);
  await page.evaluate(() => window.scrollTo(0, 0));
  await expect(page.locator('#requirement-r0')).toBeVisible();
});

test('virtualized infinite list keeps the rendered DOM bounded', async ({ page }) => {
  const api = await fixture(page, 600);
  await page.goto('/projects/demo/requirements');
  await expect(page.locator('#requirement-r0')).toBeVisible();
  for (let i = 0; i < 7; i++) {
    await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
    await expect.poll(() => api.connectionReads().length).toBeGreaterThan(i + 1);
    await expect.poll(() => page.locator('[id^="requirement-r"]').count()).toBeLessThan(100);
  }
});

test('infinite requirements restore loaded rows and scroll after leaving the route', async ({
  page,
}) => {
  const api = await fixture(page, 100);
  await page.goto('/projects/demo/requirements');
  await expect(page.locator('#requirement-r0')).toBeVisible();
  await page.getByRole('button', { name: 'Загрузить ещё', exact: true }).click();
  await expect.poll(() => api.connectionReads().length).toBe(2);
  await page.evaluate(() => window.scrollTo(0, 1800));
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBeGreaterThan(1000);
  const savedOffset = await page.evaluate(() => window.scrollY);
  const savedReads = api.connectionReads().length;
  await page.getByRole('link', { name: 'Обзор', exact: true }).evaluate((link) => {
    if (link instanceof HTMLElement) link.click();
  });
  await expect(page).toHaveURL('/projects/demo');
  await page.goBack();
  await expect(page.locator('[id^="requirement-r"]').first()).toBeVisible();
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBeGreaterThan(savedOffset - 200);
  expect(api.connectionReads()).toHaveLength(savedReads);
});

test('legacy list URLs open the first cursor batch without pagination controls', async ({
  page,
}) => {
  const api = await fixture(page, 45);
  await page.goto('/projects/demo/requirements?listMode=pages&page=2&source=bookmark');
  await expect(page.getByRole('heading', { name: 'Бизнес-требования', exact: true })).toBeVisible();
  await expect(page.getByLabel('Режим списка')).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Назад', exact: true })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Далее', exact: true })).toHaveCount(0);
  await expect(page.locator('#requirement-r0')).toBeVisible();
  expect(api.connectionReads()).toEqual([{ first: 40, after: null }]);
  expect(new URL(page.url()).searchParams.get('source')).toBe('bookmark');
});

test('first load reserves space and a failed first request has a local retry', async ({ page }) => {
  const api = await fixture(page);
  const release = api.delay();
  await page.goto('/projects/demo/requirements');
  await expect.poll(api.reads).toBeGreaterThan(0);
  await expect(page.getByText('Загрузка…', { exact: true })).toBeVisible();
  await expect(page.locator('.skeleton')).toBeVisible();
  await expect(page.getByText('Требований пока нет', { exact: true })).toHaveCount(0);
  api.fail(503);
  release();
  await expect(page.getByRole('button', { name: 'Повторить', exact: true })).toBeVisible();
  await expect(page.locator('#requirement-r0')).toHaveCount(0);
  api.fail(0);
  await page.getByRole('button', { name: 'Повторить', exact: true }).click();
  await expect(page.locator('#requirement-r0')).toBeVisible();
});

test('slow SSE refetch and reconnect reset preserve cards, draft, focus and continuation', async ({
  page,
}) => {
  const api = await fixture(page, 41);
  await page.goto('/projects/demo/requirements');
  const card = page.locator('#requirement-r0');
  await expect(card).toBeVisible();
  const draft = page.getByLabel('Текст требования');
  await draft.fill('Несохранённый черновик');
  await draft.focus();
  const before = await card.boundingBox();
  const reads = api.reads();
  const connections = api.connections();
  const release = api.delay();
  await api.event();
  await expect.poll(api.reads).toBeGreaterThan(reads);
  await expect(card).toBeVisible();
  await expect(draft).toHaveValue('Несохранённый черновик');
  await expect(draft).toBeFocused();
  expect((await card.boundingBox())?.y).toBe(before?.y);
  api.setText('Обновлённое требование');
  release();
  await expect(
    card
      .getByRole('region', { name: 'Предложение, созданное ИИ', exact: true })
      .locator(':scope > p')
      .filter({ hasText: /^Обновлённое требование$/ }),
  ).toBeVisible();
  await expect.poll(api.connections).toBeGreaterThan(connections);
  const resetReads = api.reads();
  const releaseReset = api.delay();
  await api.event(true);
  await expect.poll(api.reads).toBeGreaterThan(resetReads);
  await expect(card).toBeVisible();
  await expect(draft).toHaveValue('Несохранённый черновик');
  releaseReset();
  await expect(draft).toBeFocused();
  await page.getByRole('button', { name: 'Загрузить ещё', exact: true }).click();
  await expect.poll(() => api.connectionReads().at(-1)?.after).toBe('cursor-40');
  await expect(page.getByRole('button', { name: 'Загрузить ещё', exact: true })).toHaveCount(0);
  await page.evaluate(() => window.scrollTo(0, 0));
  await expect(card).toBeVisible();
  await expect(draft).toHaveValue('Несохранённый черновик');
});

test('failed background request keeps stale data and local retry restores freshness', async ({
  page,
}) => {
  const api = await fixture(page);
  await page.goto('/projects/demo/requirements');
  await expect(page.locator('#requirement-r0')).toBeVisible();
  api.fail(503);
  await api.event();
  await expect(
    page.getByText('Показаны последние загруженные данные. Они могут быть устаревшими.', {
      exact: false,
    }),
  ).toBeVisible();
  await expect(page.locator('#requirement-r0')).toBeVisible();
  api.fail(0);
  api.setText('Успешный повтор');
  await page.getByRole('button', { name: 'Повторить', exact: true }).click();
  await expect(
    page
      .locator('#requirement-r0')
      .getByRole('region', { name: 'Предложение, созданное ИИ', exact: true })
      .locator(':scope > p')
      .filter({ hasText: /^Успешный повтор$/ }),
  ).toBeVisible();
  await expect(
    page.getByText('Показаны последние загруженные данные. Они могут быть устаревшими.', {
      exact: false,
    }),
  ).toHaveCount(0);
});

test('revoked access clears previously rendered data', async ({ page }) => {
  const api = await fixture(page);
  await page.goto('/projects/demo/requirements');
  await expect(page.locator('#requirement-r0')).toBeVisible();
  await page.getByRole('button', { name: 'Изменить', exact: true }).first().click();
  await page.getByRole('dialog').getByLabel('Текст требования').fill('Закрытый черновик');
  api.fail(403);
  await api.event();
  await expect(page.locator('#requirement-r0')).toHaveCount(0);
  await expect(page.getByText('Требование 0', { exact: true })).toHaveCount(0);
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(page.getByText('Закрытый черновик', { exact: true })).toHaveCount(0);
});

test('project pages share navigation, locale and browser history without overflow', async ({
  page,
}) => {
  await fixture(page);
  await page.goto('/');
  await expect(page.getByRole('link', { name: project.name, exact: true })).toBeVisible();
  await expect(page.getByLabel('Название', { exact: true })).toHaveCount(0);
  await page.getByRole('link', { name: 'Создать проект', exact: true }).click();
  await expect(page).toHaveURL(/\/projects\/new$/);
  await expect(page.getByLabel('Название', { exact: true })).toBeVisible();
  await page.goto('/projects/demo');
  await expect(page.getByRole('heading', { name: project.name, exact: true })).toBeVisible();
  await expect(page.getByLabel('Текст требования')).toHaveCount(0);
  await page.getByLabel('Язык').selectOption('en');
  for (const [name, path] of [
    ['Requirements', 'requirements'],
    ['Indexing', 'indexing'],
    ['Search', 'search'],
    ['Analysis', 'analysis'],
  ] as const) {
    await page.getByRole('link', { name, exact: true }).first().click();
    await expect(page).toHaveURL(new RegExp(`/${path}$`));
    await expect(page.getByRole('link', { name, exact: true }).first()).toHaveAttribute(
      'aria-current',
      'page',
    );
    await expect(page.getByLabel('Language')).toHaveValue('en');
  }
  await page.goBack();
  await expect(page).toHaveURL(/\/search$/);
  await page.goForward();
  await expect(page).toHaveURL(/\/analysis$/);
  await page.reload();
  await expect(page.getByLabel('Language')).toHaveValue('en');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
});

test('requirement source links select the correct card and survive direct opening', async ({
  page,
}) => {
  const api = await fixture(page, 21, '', project, undefined, 10);
  await page.goto('/projects/demo/search');
  await page.getByLabel('Поисковый запрос').fill('20');
  await page.getByRole('button', { name: 'Найти требования', exact: true }).click();
  await page.getByRole('link', { name: 'Перейти к требованию', exact: true }).click();
  await expect(page).toHaveURL(/\/requirements\?requirement=r20(?:&[^#]*)?#requirement-r20$/);
  await expect(page.locator('#requirement-r20')).toBeVisible();
  expect(api.connectionReads().map((read) => read.after)).toEqual([null, 'cursor-10', 'cursor-20']);
  await page.reload();
  await expect(page.locator('#requirement-r20')).toBeVisible();
  await page.goBack();
  await expect(page).toHaveURL(/\/search(?:\?.*)?$/);
});

test('indexing appends on scroll with matching statuses and stops at the end', async ({ page }) => {
  const api = await fixture(page, 85);
  await page.goto('/projects/demo/indexing');
  await expect(page.getByLabel('Режим списка')).toHaveCount(0);
  await expect(page.getByText('Требование 2', { exact: true })).toBeVisible();
  await expect.poll(() => api.indexReads().length).toBe(1);
  expect(api.indexReads()[0]).toHaveLength(40);
  await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
  await expect.poll(() => api.indexReads().length).toBeGreaterThan(1);
  await expect
    .poll(async () => {
      await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
      return api.indexReads().length;
    })
    .toBe(3);
  await expect(page.getByRole('button', { name: 'Загрузить ещё', exact: true })).toHaveCount(0);
  await page.evaluate(() => window.scrollTo(0, 0));
  const failed = page.getByRole('group', { name: 'ошибка', exact: true });
  await expect(failed.getByText('Требование 1', { exact: true })).toBeVisible();
});

test('indexing group state is restored across a Russian to English navigation', async ({
  page,
}) => {
  await fixture(page, 2);
  await page.goto('/projects/demo/indexing');
  const language = page.locator('.language select');
  await language.selectOption('ru');
  let ready = page.locator('.data-group').last();
  await expect(ready).toHaveAttribute('data-restored', 'true');
  if ((await ready.getAttribute('open')) === null) await ready.locator('summary').click();
  await expect(page.locator('#indexing-r0')).toBeVisible();
  await expect
    .poll(() => page.evaluate(() => sessionStorage.getItem('group:demo:indexing:ready')))
    .toBe('true');

  await language.selectOption('en');
  await page.goto('/projects/demo/requirements');
  await page.goto('/projects/demo/indexing');
  ready = page.locator('.data-group').last();
  await expect(ready).toHaveAttribute('data-restored', 'true');
  await expect(ready).toHaveAttribute('open', '');
  await expect(page.locator('#indexing-r0')).toBeVisible();
});

test('indexing load-more retry preserves loaded cards', async ({ page }) => {
  const api = await fixture(page, 85);
  await page.goto('/projects/demo/indexing');
  await expect(page.getByText('Требование 2', { exact: true })).toBeVisible();
  api.failIndex(503);
  await page.getByRole('button', { name: 'Загрузить ещё', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Повторить', exact: true })).toBeVisible();
  await page.evaluate(() => window.scrollTo(0, 0));
  await expect(page.getByText('Требование 2', { exact: true })).toBeVisible();
  api.failIndex(0);
  await page.getByRole('button', { name: 'Повторить', exact: true }).click();
  await expect.poll(() => api.indexReads().length).toBe(3);
});

test('infinite indexing refreshes statuses after SSE and clears cards on access revocation', async ({
  page,
}) => {
  const api = await fixture(page, 85);
  await page.goto('/projects/demo/indexing');
  const failed = page.getByRole('group', { name: 'ошибка', exact: true });
  await expect(failed.getByText('Требование 1', { exact: true })).toBeVisible();
  api.setReady();
  await api.event();
  await expect(failed.getByText('Требование 1', { exact: true })).toHaveCount(0);
  const ready = page.getByRole('group', { name: 'готов', exact: true });
  await ready.locator('summary').click();
  await expect(ready.getByText('Требование 1', { exact: true })).toBeVisible();
  api.fail(403);
  await api.event();
  await expect(page.locator('[id^="indexing-r"]')).toHaveCount(0);
});

test('index group summaries reflect loaded rows and update while collapsed', async ({ page }) => {
  const api = await fixture(page, 85);
  await page.goto('/projects/demo/indexing?listMode=pages&page=3');
  const failed = page.getByRole('group', { name: 'ошибка', exact: true });
  const pending = page.getByRole('group', { name: 'ожидает индексации', exact: true });
  const ready = page.getByRole('group', { name: 'готов', exact: true });
  await expect(page.getByText('Загружено: 40 из 85', { exact: true })).toBeVisible();
  await expect(failed.locator('summary')).toContainText('1');
  await expect(pending.locator('summary')).toContainText('38');
  await failed.locator('summary').focus();
  await page.keyboard.press('Enter');
  await expect(failed).not.toHaveAttribute('open', '');
  api.setReady();
  await api.event();
  await expect(failed.locator('summary')).toContainText('0');
  await expect(failed).not.toHaveAttribute('open', '');
  await expect(ready.locator('summary')).toContainText(/(?:40|80|85)/);
  await expect
    .poll(async () => {
      await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
      return ready.locator('summary').textContent();
    })
    .toContain('85');
});

test('a failed project continuation retries the same cursor and preserves earlier projects', async ({
  page,
}) => {
  let failed = true;
  const reads: Array<string | null> = [];
  await page.route('**/api/events', (route) =>
    route.fulfill({ status: 200, contentType: 'text/event-stream', body: ': heartbeat\n\n' }),
  );
  await page.route('**/api/graphql', async (route) => {
    const { variables } = route.request().postDataJSON();
    reads.push(variables.after);
    if (variables.after && failed) {
      await route.fulfill({ status: 503, json: { error: 'unavailable' } });
      return;
    }
    const start = variables.after ? 40 : 0;
    const edges = Array.from({ length: start === 40 ? 1 : 40 }, (_, i) => ({
      node: { ...project, id: `p${start + i}`, name: `Проект ${start + i}` },
      cursor: `cursor-${start + i + 1}`,
    }));
    await route.fulfill({
      json: {
        data: {
          projectsConnection: {
            edges,
            totalCount: 41,
            pageInfo: { endCursor: edges.at(-1)?.cursor, hasNextPage: !start },
          },
        },
      },
    });
  });
  await page.goto('/?listMode=pages&page=3');
  const first = page.getByRole('link', { name: 'Проект 0', exact: true });
  await expect(first).toBeVisible();
  const more = page.getByRole('button', { name: 'Загрузить ещё', exact: true });
  await more.focus();
  await page.keyboard.press('Enter');
  const retry = page.getByRole('button', { name: 'Повторить', exact: true });
  await expect(retry).toBeVisible();
  await page.evaluate(() => window.scrollTo(0, 0));
  await expect(first).toBeVisible();
  failed = false;
  await retry.click();
  await expect(more).toHaveCount(0);
  await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
  await expect(page.getByRole('link', { name: 'Проект 40', exact: true })).toBeVisible();
  expect(reads).toEqual([null, 'cursor-40', 'cursor-40']);
});

test('a late create response does not redirect after the user has left the form', async ({
  page,
}) => {
  let release!: () => void, started!: () => void;
  const pending = new Promise<void>((resolve) => (release = resolve)),
    sent = new Promise<void>((resolve) => (started = resolve));
  await page.route('**/api/events', (route) =>
    route.fulfill({ status: 200, contentType: 'text/event-stream', body: ': heartbeat\n\n' }),
  );
  await page.route('**/api/graphql', async (route) => {
    const { query } = route.request().postDataJSON();
    if (query.includes('createProject(')) {
      started();
      await pending;
      await route.fulfill({
        json: {
          data: { createProject: { project: { ...project, id: 'created' }, fieldErrors: [] } },
        },
      });
      return;
    }
    await route.fulfill({
      json: {
        data: query.includes('projectsConnection(')
          ? {
              projectsConnection: {
                edges: [{ cursor: 'project-1', node: project }],
                pageInfo: { endCursor: 'project-1', hasNextPage: false },
                totalCount: 1,
              },
            }
          : query.includes('projects(')
            ? { projects: [project] }
            : { project },
      },
    });
  });
  await page.goto('/projects/new');
  await page.getByLabel('Название').fill('Новый проект');
  await page.getByLabel('Цель').fill('Цель');
  await page.getByRole('button', { name: 'Создать проект', exact: true }).click();
  await sent;
  await page.getByRole('link', { name: 'Все проекты', exact: true }).click();
  await expect(page).toHaveURL(/\/$/);
  const response = page.waitForResponse(
    (response) =>
      response.url().endsWith('/api/graphql') &&
      response.request().postDataJSON().query.includes('createProject('),
  );
  release();
  await (await response).finished();
  await page.evaluate(
    () =>
      new Promise<void>((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
      ),
  );
  await expect(page).toHaveURL(/\/$/);
  await expect(page.getByRole('link', { name: project.name, exact: true })).toBeVisible();
});

for (const { width, zoom } of layoutCases) {
  test(`responsive content at ${width}px zoom ${zoom} preserves full project text`, async ({
    page,
  }, testInfo) => {
    test.skip(testInfo.project.name !== 'desktop', 'Explicit viewport matrix');
    await setLayout(page, width, zoom);
    const longText = `${'ТребованиеURL_identifier'.repeat(20)}\nВторая строка / second line`;
    const api = await fixture(
      page,
      3,
      longText,
      {
        ...project,
        name: 'Проект'.repeat(20),
        goal: longText,
        description: longText,
      },
      longText,
    );
    api.setProgress({
      total: 2147483647,
      ready: 1000000000,
      queued: 1147483645,
      running: 1,
      failed: 1,
    });
    await page.goto('/');
    await expect(page.getByRole('article')).toHaveCount(1);
    await expectLayout(page);
    await page.getByRole('article').getByRole('link').click();
    await expect(page.getByRole('heading', { name: 'Обзор', exact: true })).toBeVisible();
    await expectLayout(page);
    await page.screenshot({ path: testInfo.outputPath('overview.png'), fullPage: true });
    const description = page.locator('section > p').filter({ hasText: 'Вторая строка' }).first();
    await expect(description).toHaveText(longText);
    expect(await description.evaluate((el) => getComputedStyle(el).whiteSpace)).toMatch(/pre/);
    for (const locale of ['ru', 'en']) {
      await page.locator('.language select').selectOption(locale);
      for (const path of ['requirements', 'indexing', 'search', 'analysis']) {
        await page.goto(`/projects/demo/${path}`);
        await expect(page.locator('.project-nav a[aria-current="page"]')).toBeVisible();
        await expect(page.locator('main h2').first()).toBeVisible();
        if (path === 'requirements') {
          const card = page.locator('#requirement-r0');
          const original = card.locator('details').filter({
            has: page.getByText(
              locale === 'ru' ? 'Полный исходный текст требования' : 'Full original requirement',
              { exact: true },
            ),
          });
          await original.locator('summary').focus();
          await page.keyboard.press('Enter');
          await expect(original).toHaveAttribute('open', '');
          await expect(original.locator('p')).toHaveText(`Требование 0${longText}`);
          const edit = card.getByRole('button', {
            name: locale === 'ru' ? 'Изменить' : 'Edit',
            exact: true,
          });
          await edit.click();
          await expect(
            page
              .getByRole('dialog')
              .getByLabel(locale === 'ru' ? 'Текст требования' : 'Requirement text'),
          ).toHaveValue(`Требование 0${longText}`);
        }
        if (path === 'indexing') {
          await expect(page.locator('#indexing-r1')).toBeVisible();
          const ready = page.locator('.data-group').last();
          await expect(ready).toHaveAttribute('data-restored', 'true');
          if ((await ready.getAttribute('open')) === null) await ready.locator('summary').click();
          await expect(page.locator('#indexing-r0')).toBeVisible();
        }
        if (path === 'search') {
          await page
            .locator('#semantic-search-form textarea, #semantic-search-form input')
            .fill('text');
          await page.locator('#semantic-search-form button').click();
          await page.locator('article details summary').click();
          await expect(page.locator('article details p')).toHaveText(`Требование 0${longText}`);
        }
        await expectLayout(page);
      }
    }
  });
}

for (const route of ['requirements', 'analysis']) {
  test(`pilot actions on ${route} show hover and keyboard focus`, async ({ page }, testInfo) => {
    const renderingErrors: string[] = [];
    page.on('pageerror', (error) => renderingErrors.push(error.message));
    page.on('console', (message) => {
      if (message.type() === 'error' && /hydrat/i.test(message.text())) {
        renderingErrors.push(message.text());
      }
    });
    await fixture(page, 3);
    await page.goto(`/projects/demo/${route}`);
    const section = page.locator('main section[aria-labelledby]').last();
    const button = section.locator('button:enabled').first();
    await expect(button).toBeVisible();
    await page.screenshot({ path: testInfo.outputPath(`${route}.png`) });
    const rest = await button.evaluate((el) => getComputedStyle(el).backgroundColor);
    await button.hover();
    await expect(button).not.toHaveCSS('background-color', rest);
    await button.focus();
    await page.keyboard.press('Tab');
    await page.keyboard.press('Shift+Tab');
    await expect(button).toBeFocused();
    await expect(button).toHaveCSS('outline-style', 'solid');
    expect(renderingErrors).toEqual([]);
  });
}
