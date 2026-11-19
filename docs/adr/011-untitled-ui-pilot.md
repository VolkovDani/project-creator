# ADR-011 — Untitled UI styles for requirements and analysis

[Русская версия](011-untitled-ui-pilot.ru.md)

## Decision

Use the free Untitled UI React brand colors and small shadow as CSS tokens on the existing HTML components. The source is pinned to [fd321c4](https://github.com/untitleduico/react/blob/fd321c459f7c03173ffabf89bfe174827039643c/styles/theme.css). The adapted stylesheet retains the upstream [MIT notice](https://github.com/untitleduico/react/blob/fd321c459f7c03173ffabf89bfe174827039643c/LICENSE).

The pilot runs on Next.js 16.4.6 and React 19.3.1. Native buttons, labels, selects, radios, dialogs and disclosures already support the required interactions. Applying styles directly keeps their keyboard behavior and server-rendered markup and adds no JavaScript dependency. Paid assets and user-selectable dark mode are outside the pilot.

## Usage

Apply `ui-panel` to a requirements or analysis section. The stylesheet supplies semantic colors, spacing, control typography, borders, radii, focus and elevation. Form submit buttons use primary styling. Apply `primary-action` to the main analysis action and `destructive-action` to deletion. Keep native disabled and invalid states. Use the existing Group and ResourceState components for disclosures, loading and retry. Alerts and progress messages retain their existing live-region roles.

## Verification and remaining screens

Browser fixtures cover RU/EN text, errors, loading, empty results, disabled controls, keyboard focus, open and closed groups, analysis progress and responsive layouts at 320, 390, 768, 960 and 1920 pixels, including 200% CSS zoom. Primary button text against its background has a 6.62:1 contrast ratio. The focus ring against white measures 4.96:1.

Future migration candidates are the project list and creation form, project overview and navigation, indexing and search. Keep each migration independently reviewable and preserve its data flow. Issue #46 supplies historical accessibility and component requirements.
