'use client';

import { useEffect, useState } from 'react';
import { t } from '../../lib/i18n';
import { useLocale } from './LocaleProvider';

export default function BackToTop() {
  const { locale } = useLocale();
  const [visible, setVisible] = useState(false);
  useEffect(() => {
    const update = () => setVisible(window.scrollY > 400);
    update();
    window.addEventListener('scroll', update, { passive: true });
    return () => window.removeEventListener('scroll', update);
  }, []);
  if (!visible) return null;
  return (
    <button
      className="back-to-top"
      type="button"
      onClick={() => {
        const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
        window.scrollTo({ top: 0, behavior: reducedMotion ? 'instant' : 'smooth' });
        const heading = document.querySelector('main h1');
        if (heading instanceof HTMLElement) {
          if (!heading.hasAttribute('tabindex')) heading.tabIndex = -1;
          heading.focus({ preventScroll: true });
        }
      }}
    >
      {t(locale, 'ui.backToTop')}
    </button>
  );
}
