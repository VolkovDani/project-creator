import { expect, test } from '@playwright/test';

test('a saved proposal appears in the open review and retains the cited version after editing', async ({
  context,
  page,
  request,
}) => {
  async function graphql(query: string, variables?: Record<string, unknown>) {
    const response = await request.post('/api/graphql', { data: { query, variables } });
    expect(response.ok()).toBe(true);
    const { data, errors } = await response.json();
    expect(errors).toBeUndefined();
    return data;
  }

  const { createProject } = await graphql(
    'mutation { createProject(input: { name: "External review", goal: "Review assistant proposals", description: "" }) { project { id } } }',
  );
  const projectId = String(createProject.project.id);
  const { createRequirement } = await graphql(
    `mutation($project: ID!) {
      createRequirement(input: { projectId: $project, text: "Set a payment deadline." }) {
        requirement { id version }
      }
    }`,
    { project: projectId },
  );
  const { id: requirementId, version } = createRequirement.requirement;

  await page.goto(`/projects/${projectId}/analysis`);
  await page.getByRole('link', { name: 'Внешние предложения' }).click();
  await expect(page.getByText('Внешних предложений пока нет.')).toBeVisible();

  const analysis = 'Clarify when the payment deadline starts.';
  const { saveExternalProposal } = await graphql(
    `mutation($input: SaveExternalProposalInput!) {
      saveExternalProposal(input: $input) { proposal { status } fieldErrors { code } }
    }`,
    {
      input: {
        projectId,
        requestId: 'browser-external-review',
        analysis,
        limitations: 'This review considers the cited requirement only.',
        client: 'Test assistant',
        model: 'Synthetic model',
        sharing: 'local',
        sources: [{ requirementId, version }],
      },
    },
  );
  expect(saveExternalProposal).toEqual({ proposal: { status: 'pending_review' }, fieldErrors: [] });
  const proposal = page.getByRole('article').filter({ hasText: analysis });
  await expect(proposal.getByText('Ожидает проверки', { exact: true })).toBeVisible();
  await expect(proposal.getByText('Актуальная', { exact: true })).toBeVisible();
  await expect(proposal.getByText('Версия 1', { exact: true })).toBeVisible();

  const citedRequirement = proposal.getByRole('link', { name: requirementId });
  const sourceURL = `/projects/${projectId}/requirements?requirement=${requirementId}#requirement-${requirementId}`;
  await expect(citedRequirement).toHaveAttribute('href', sourceURL);
  const editor = await context.newPage();
  await editor.goto(sourceURL);
  await editor
    .locator(`#requirement-${requirementId}`)
    .getByRole('button', { name: 'Изменить', exact: true })
    .click();
  const dialog = editor.getByRole('dialog');
  const text = dialog.getByLabel('Текст требования');
  await expect(text).toHaveValue('Set a payment deadline.');
  await text.fill('Payment is due 30 days after the invoice date.');
  await dialog.getByLabel('Причина изменения').fill('Clarify how the deadline is calculated.');
  await dialog.getByRole('button', { name: 'Сохранить', exact: true }).click();
  await expect(proposal.getByText('Устарела', { exact: true })).toBeVisible();
  await expect(proposal.getByText('Версия 1', { exact: true })).toBeVisible();
});
