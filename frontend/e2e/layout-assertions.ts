import { expect, type Page } from '@playwright/test';

export const layoutCases = [
  ...[320, 390, 768, 960, 1920].map((width) => ({ width, zoom: 1 })),
  { width: 1920, zoom: 2 },
];

// CSS zoom exercises 200% text/control scaling; 960px also checks desktop reflow.
export async function setLayout(page: Page, width: number, zoom: number) {
  await page.setViewportSize({ width, height: 1080 });
  await page.addInitScript((scale) => {
    document.addEventListener('DOMContentLoaded', () => {
      document.documentElement.style.zoom = String(scale);
    });
  }, zoom);
}

export async function expectLayout(page: Page) {
  await expect
    .poll(async () =>
      page.evaluate(() => {
        const overflow: string[] = [];
        if (document.documentElement.scrollWidth > window.innerWidth + 1) overflow.push('viewport');
        for (const element of document.querySelectorAll<HTMLElement>(
          'main, section, article, details, fieldset, legend, p, h1, h2, h3, button, dl, dd, pre',
        )) {
          if (
            element.getClientRects().length &&
            element.clientWidth &&
            element.scrollWidth > element.clientWidth + 1
          ) {
            overflow.push(
              `${element.tagName}.${element.className}: ${element.scrollWidth} > ${element.clientWidth}`,
            );
          }
        }
        for (const control of document.querySelectorAll<HTMLElement>(
          'button, select, input, textarea',
        )) {
          if (!control.getClientRects().length) continue;
          const rect = control.getBoundingClientRect();
          const parent = control.parentElement?.getBoundingClientRect();
          if (parent && (rect.left < parent.left - 1 || rect.right > parent.right + 1))
            overflow.push(`${control.tagName} outside parent`);
        }
        return overflow;
      }),
    )
    .toEqual([]);
}
