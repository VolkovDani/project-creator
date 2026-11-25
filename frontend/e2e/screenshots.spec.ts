import { mkdir } from 'node:fs/promises';
import { resolve } from 'node:path';
import { expect, test } from '@playwright/test';

test('screenshots: desktop projects and requirements', async ({ page }, testInfo) => {
  if (process.env.SCREENSHOT_FIXTURES === '1') {
    let project: Record<string, unknown> | null = null;
    const requirements: Record<string, unknown>[] = [];
    await page.route('**/api/events', (route) =>
      route.fulfill({ status: 200, contentType: 'text/event-stream', body: ': heartbeat\n\n' }),
    );
    await page.route('**/api/graphql', async (route) => {
      const { query, variables } = route.request().postDataJSON();
      let data: unknown;
      if (query.includes('createProject')) {
        project = {
          ...variables.input,
          id: 'demo-project',
          createdAt: '2026-10-27T12:00:00Z',
          updatedAt: '2026-10-27T12:00:00Z',
        };
        data = { createProject: { project, fieldErrors: [] } };
      } else if (query.includes('createRequirement')) {
        const requirement = {
          ...variables.input,
          id: `requirement-${requirements.length + 1}`,
          version: 1,
          createdAt: '2026-10-27T12:00:00Z',
          updatedAt: '2026-10-27T12:00:00Z',
        };
        requirements.push(requirement);
        data = { createRequirement: { requirement, fieldErrors: [] } };
      } else if (query.includes('analysisHistory(')) data = { analysisHistory: [] };
      else if (query.includes('projectAnalysis(')) data = { projectAnalysis: null };
      else if (query.includes('requirementIndex('))
        data = {
          requirementIndex: requirements.map((item) => ({
            requirementId: item.id,
            version: item.version,
            status: 'pending',
            profile: '',
            errorCode: '',
          })),
        };
      else if (query.includes('requirementsConnection(')) {
        const offset = variables.after ? Number(String(variables.after).replace('cursor-', '')) : 0;
        const batch = requirements.slice(offset, offset + Number(variables.first));
        data = {
          requirementsConnection: {
            edges: batch.map((node, index) => ({ cursor: `cursor-${offset + index + 1}`, node })),
            pageInfo: {
              endCursor: batch.length ? `cursor-${offset + batch.length}` : null,
              hasNextPage: offset + batch.length < requirements.length,
            },
            totalCount: requirements.length,
          },
        };
      } else if (query.includes('requirements(')) data = { requirements };
      else if (query.includes('projectsConnection('))
        data = {
          projectsConnection: {
            edges: project ? [{ cursor: 'project-1', node: project }] : [],
            pageInfo: { endCursor: project ? 'project-1' : null, hasNextPage: false },
            totalCount: project ? 1 : 0,
          },
        };
      else if (query.includes('projects(')) data = { projects: project ? [project] : [] };
      else data = { project };
      await route.fulfill({ json: { data } });
    });
  }
  await page.setViewportSize({ width: 1920, height: 1080 });
  await page.goto('/projects/new');
  await page.getByLabel('Название').fill('Контроль бизнес-требований');
  await page
    .getByLabel('Цель')
    .fill('Согласовывать требования и находить возможные противоречия до начала разработки.');
  await page
    .getByLabel('Описание')
    .fill('Единое пространство для проектов, требований и истории изменений.');
  await page.getByRole('button', { name: 'Создать проект' }).click();
  await expect(
    page.getByRole('heading', { name: 'Контроль бизнес-требований', exact: true }),
  ).toBeVisible();
  await page.getByRole('link', { name: 'Все проекты', exact: true }).click();
  const link = page.getByRole('link', { name: 'Контроль бизнес-требований', exact: true });
  const projectList = page.getByRole('region', { name: 'Проекты', exact: true });
  await expect(projectList).toHaveAttribute('aria-busy', 'false');
  if (!(await link.isVisible())) await page.keyboard.press('End');
  await expect(link).toBeVisible();
  const directory = resolve('screenshots');
  await mkdir(directory, { recursive: true });
  const projects = resolve(directory, 'projects-desktop.png');
  await page.screenshot({ path: projects, fullPage: false });
  await testInfo.attach('Projects — 1920×1080', { path: projects, contentType: 'image/png' });
  await link.click();
  await expect(
    page.getByRole('heading', { name: 'Контроль бизнес-требований', exact: true }),
  ).toBeVisible();
  await page.getByRole('link', { name: 'Требования', exact: true }).first().click();
  const list = page.locator('#requirements-list');
  const samples = [
    'Менеджер может создавать проекты и описывать их цели.',
    'Требования сохраняются между перезапусками приложения.',
    'При одновременном редактировании система предотвращает потерю изменений.',
  ];
  for (const text of samples) {
    await page.getByRole('button', { name: 'Добавить требование', exact: true }).click();
    const create = page.getByRole('dialog', { name: 'Добавить требование', exact: true });
    await create.getByLabel('Текст требования').fill(text);
    await create.getByRole('button', { name: 'Добавить требование', exact: true }).click();
    await expect(create).toBeHidden();
    await expect(list.getByRole('button', { name: text, exact: true })).toBeVisible();
  }
  await list.getByRole('button', { name: samples[0], exact: true }).click();
  await expect(page.getByRole('article', { name: 'Сведения о требовании' })).toBeVisible();
  const requirements = resolve(directory, 'project-requirements-desktop.png');
  await page.screenshot({ path: requirements, fullPage: false });
  await testInfo.attach('Project requirements — 1920×1080', {
    path: requirements,
    contentType: 'image/png',
  });
});
