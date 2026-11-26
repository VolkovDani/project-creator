import { expect, test } from '@playwright/test';

const labels = {
  actor: 'Участник',
  action: 'Действие',
  condition: 'Условие',
  event: 'Событие',
  state: 'Состояние',
  object: 'Объект',
  rule: 'Правило',
};

test('the legend matches the diagram and relation cards in both themes', async ({ page }) => {
  await page.route('**/api/events', (route) =>
    route.fulfill({ contentType: 'text/event-stream', body: ': heartbeat\n\n' }),
  );
  await page.route('**/api/graphql', async (route) => {
    const { operationName } = route.request().postDataJSON();
    const data =
      operationName === 'ProjectDetail'
        ? {
            project: { id: 'demo', name: 'Заявки', goal: '', description: '' },
            analysisDefaultThreshold: 0.7,
          }
        : {
            latestProcessGraphRun: {
              status: 'suggested',
              draft: {
                nodes: Object.keys(labels).map((kind) => ({ id: kind, kind, label: kind })),
                edges: [],
                sourceCount: 1,
                partial: false,
              },
              run: null,
              fieldErrors: [],
            },
          };
    await route.fulfill({ json: { data } });
  });

  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/projects/demo/process-graphs');
  const legend = page.getByRole('list', { name: 'Типы узлов' });
  await expect(legend).toBeVisible();
  for (const label of Object.values(labels)) await expect(legend.getByText(label)).toBeVisible();

  for (const theme of ['light', 'dark']) {
    await page.locator('html').evaluate((html, theme) => {
      html.setAttribute('data-theme', theme);
    }, theme);
    const accents = await legend.locator('li').evaluateAll((items) =>
      items.map((item) => {
        const kind = item.getAttribute('data-kind');
        const swatch = item.querySelector('[aria-hidden="true"]');
        const box = document.querySelector(`svg g[data-kind="${kind}"] rect`);
        const card = document.querySelector(`div[data-kind="${kind}"]`);
        return {
          swatch: swatch && getComputedStyle(swatch).backgroundColor,
          diagram: box && getComputedStyle(box).stroke,
          card: card && getComputedStyle(card).borderTopColor,
        };
      }),
    );
    expect(accents).toHaveLength(7);
    expect(new Set(accents.map(({ swatch }) => swatch)).size).toBe(7);
    for (const accent of accents) {
      expect(accent.swatch).toBeTruthy();
      expect(accent.diagram).toBe(accent.swatch);
      expect(accent.card).toBe(accent.swatch);
    }
  }
});
