'use client';
import { createContext, useContext, useEffect, useState } from 'react';
import { type Locale, t } from '../../lib/i18n';
import { themeStorageKey } from '../../lib/theme';

const Context = createContext<{ locale: Locale; setLocale: (locale: Locale) => void }>({
  locale: 'ru',
  setLocale: () => {},
});
export default function LocaleProvider({ children }: { children: React.ReactNode }) {
  const [locale, setLocale] = useState<Locale>('ru');
  useEffect(() => {
    try {
      const saved = localStorage.getItem('project-creator-locale');
      if (saved === 'ru' || saved === 'en') setLocale(saved);
    } catch {}
  }, []);
  useEffect(() => {
    document.documentElement.lang = locale;
  }, [locale]);
  function select(value: Locale) {
    setLocale(value);
    try {
      localStorage.setItem('project-creator-locale', value);
    } catch {}
  }
  return <Context.Provider value={{ locale, setLocale: select }}>{children}</Context.Provider>;
}
export function useLocale() {
  return useContext(Context);
}
export function InterfacePreferences() {
  const { locale, setLocale } = useLocale();
  const [dark, setDark] = useState(true);
  useEffect(() => {
    setDark(document.documentElement.dataset.theme === 'dark');
  }, []);
  function toggleTheme() {
    const next = document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark';
    document.documentElement.dataset.theme = next;
    setDark(next === 'dark');
    try {
      localStorage.setItem(themeStorageKey, next);
    } catch {}
  }
  return (
    <div className="interface-preferences">
      <label className="language">
        {t(locale, 'app.language')}{' '}
        <select value={locale} onChange={(event) => setLocale(event.target.value as Locale)}>
          <option value="ru">Русский</option>
          <option value="en">English</option>
        </select>
      </label>
      <button type="button" className="theme-toggle" aria-pressed={dark} onClick={toggleTheme}>
        {t(locale, 'app.darkTheme')}
      </button>
    </div>
  );
}
