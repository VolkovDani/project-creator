import { expect, type Page, test } from '@playwright/test';
import { expectLayout } from './layout-assertions';

const project = { id: 'demo', name: 'Demo', goal: 'G', description: '' };
const analysis = '<img src="/external-output" alt="external output">';
const saved = {
  id: 'proposal-a',
  analysis,
  limitations: 'The proposal only considers the supplied requirements.',
  client: 'Local assistant',
  model: 'Example model',
  sharing: 'local',
  status: 'pending_review',
  createdAt: '2026-11-05T00:00:00Z',
  glossarySources: [{ termId: 'payment-term', version: 3, status: 'current' }],
  sources: [
    { requirementId: 'payment', version: 2, status: 'current' },
    { requirementId: 'deleted', version: 1, status: 'unavailable' },
  ],
};

type EventWindow = Window & {
  sendInvalidation: (id: string, keys: string[]) => void;
};

async function fixture(page: Page) {
  let current = saved;
  let denied = false;
  const cursors: (string | null)[] = [];
  await page.addInitScript(() => {
    localStorage.setItem('project-creator-locale', 'en');
    const original = window.fetch;
    window.fetch = (input, init) => {
      if (String(input) !== '/api/events') return original(input, init);
      const stream = new ReadableStream<Uint8Array>({
        start(controller) {
          (window as unknown as EventWindow).sendInvalidation = (id, keys) => {
            const data = JSON.stringify({ schemaVersion: 1, keys });
            controller.enqueue(
              new TextEncoder().encode(`id: ${id}\nevent: cache.invalidate\ndata: ${data}\n\n`),
            );
          };
        },
      });
      return Promise.resolve(
        new Response(stream, { headers: { 'content-type': 'text/event-stream' } }),
      );
    };
    class ManualPagingObserver extends IntersectionObserver {
      override observe() {}
    }
    window.IntersectionObserver = ManualPagingObserver;
  });
  await page.route('**/api/graphql', async (route) => {
    const { query, variables } = route.request().postDataJSON();
    let data: unknown;
    if (query.includes('externalProposals(')) {
      expect(variables.projectId).toBe(project.id);
      if (denied) return route.fulfill({ status: 403 });
      cursors.push(variables.after ?? null);
      data = {
        externalProposals: variables.after
          ? {
              items: [{ ...saved, id: 'proposal-b', analysis: 'Older proposal', sharing: 'cloud' }],
              pageInfo: { endCursor: null, hasNextPage: false },
            }
          : {
              items: [current],
              pageInfo: { endCursor: 'next', hasNextPage: true },
            },
      };
    } else if (query.includes('projectAnalysisSummary(')) {
      data = { projectAnalysis: null };
    } else {
      data = { project, analysisDefaultThreshold: 0.65 };
    }
    await route.fulfill({ json: { data } });
  });
  return {
    cursors,
    deny: () => {
      denied = true;
    },
    changeSource: () => {
      current = {
        ...saved,
        sources: [{ ...saved.sources[0], status: 'stale' }, saved.sources[1]],
      };
    },
    changeGlossary: () => {
      current = {
        ...current,
        glossarySources: [{ ...saved.glossarySources[0], status: 'stale' }],
      };
    },
    changeProposal: () => {
      current = { ...current, limitations: 'Updated set of limitations.' };
    },
  };
}

test('opens the review from Analysis, loads older proposals and refreshes cited source status', async ({
  page,
}) => {
  const data = await fixture(page);
  await page.goto('/projects/demo/analysis');
  await page.getByRole('link', { name: 'External proposals' }).click();
  await expect(page.getByRole('heading', { name: 'External proposals' })).toBeVisible();
  await expect(page).toHaveTitle('External proposals — Project Creator');
  const menu = page.getByRole('button', { name: 'Open project menu' });
  const collapsed = await menu.isVisible();
  if (collapsed) await menu.click();
  const section = page.getByRole('navigation').getByRole('link', { name: 'Analysis', exact: true });
  await expect(section).toHaveAttribute('aria-current', 'location');
  if (collapsed) {
    await expect(section).toBeFocused();
    await page.keyboard.press('Escape');
  }
  const proposal = page.getByRole('article').first();
  await expect(proposal.getByText(analysis, { exact: true })).toBeVisible();
  await expect(proposal.getByRole('img')).toHaveCount(0);
  await expect(proposal.getByText(saved.limitations, { exact: true })).toBeVisible();
  await expect(proposal.getByText(saved.client, { exact: true })).toBeVisible();
  await expect(proposal.getByText(saved.model, { exact: true })).toBeVisible();
  await expect(proposal.getByText('Pending review', { exact: true })).toBeVisible();
  await expect(proposal.getByText('Local', { exact: true })).toBeVisible();
  await expect(proposal.locator('time')).toHaveAttribute('datetime', saved.createdAt);
  await expect(proposal.getByText('payment', { exact: true })).toBeVisible();
  await expect(proposal.getByText('Version 2', { exact: true })).toBeVisible();
  const citedTerm = proposal.getByRole('listitem').filter({ hasText: 'payment-term' });
  await expect(citedTerm.getByText('Version 3', { exact: true })).toBeVisible();
  await expect(citedTerm.getByText('Current', { exact: true })).toBeVisible();
  await expect(proposal.getByText('Unavailable', { exact: true })).toBeVisible();
  await expect(proposal.getByRole('link')).toHaveCount(1);
  await expect(proposal.getByRole('link')).toHaveAttribute(
    'href',
    '/projects/demo/requirements?requirement=payment#requirement-payment',
  );
  await page.getByRole('button', { name: 'Load more' }).focus();
  await page.keyboard.press('Enter');
  await expect(page.getByText('Older proposal', { exact: true })).toBeVisible();
  await expect(page.getByText('Cloud', { exact: true })).toBeVisible();
  expect(data.cursors).toEqual([null, 'next']);

  data.changeSource();
  await page.evaluate(() =>
    (window as unknown as EventWindow).sendInvalidation('1', ['project:demo:requirements']),
  );
  await expect(proposal.getByText('Stale', { exact: true })).toBeVisible();
  await expect(page.getByText('Older proposal', { exact: true })).toBeVisible();
  expect(data.cursors).toEqual([null, 'next', null, 'next']);

  data.changeGlossary();
  await page.evaluate(() =>
    (window as unknown as EventWindow).sendInvalidation('2', ['project:demo:glossary']),
  );
  await expect(citedTerm.getByText('Stale', { exact: true })).toBeVisible();

  data.changeProposal();
  await page.evaluate(() =>
    (window as unknown as EventWindow).sendInvalidation('3', ['project:demo:external-proposals']),
  );
  await expect(proposal.getByText('Updated set of limitations.', { exact: true })).toBeVisible();
  expect(data.cursors).toEqual([null, 'next', null, 'next', null, 'next', null, 'next']);
  await expectLayout(page);

  data.deny();
  await page.evaluate(() =>
    (window as unknown as EventWindow).sendInvalidation('4', ['project:demo:requirements']),
  );
  await expect(page.getByRole('article')).toHaveCount(0);
});
