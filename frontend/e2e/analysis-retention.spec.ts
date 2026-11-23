import { expect, test } from '@playwright/test';

test('scheduled deletion reaches another history tab and permanent deletion clears both', async ({
  page,
  context,
  request,
}) => {
  const created = await request.post('/api/graphql', {
    data: {
      query:
        'mutation { createProject(input: { name: "History retention", goal: "Review analyses", description: "" }) { project { id } } }',
    },
  });
  const project = (await created.json()).data.createProject.project.id;
  const started = await request.post('/api/graphql', {
    data: {
      query: 'mutation($id: ID!) { startProjectAnalysis(projectId: $id, mode: "all") { id } }',
      variables: { id: project },
    },
  });
  expect((await started.json()).data.startProjectAnalysis.id).toBeTruthy();

  const historyPath = `/projects/${project}/technical/analysis-history`;
  const other = await context.newPage();
  await Promise.all([page.goto(historyPath), other.goto(historyPath)]);
  const schedule = 'Удалить через 2 дня';
  await expect(other.getByRole('button', { name: schedule })).toBeVisible();
  await page.getByRole('button', { name: schedule }).click();
  const permanent = 'Удалить безвозвратно';
  await other.getByRole('button', { name: permanent }).click();
  await other.getByRole('dialog').getByRole('button', { name: permanent }).click();
  for (const tab of [page, other]) {
    await expect(tab.getByText('Сохранённых запусков пока нет.')).toBeVisible();
  }
});
