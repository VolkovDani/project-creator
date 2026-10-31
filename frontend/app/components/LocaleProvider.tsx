'use client';
import { createContext, useContext, useEffect, useState } from 'react';
import { type Locale, t } from '../../lib/i18n';

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
export function Language() {
  const { locale, setLocale } = useLocale();
  return (
    <label className="language">
      {t(locale, 'app.language')}{' '}
      <select value={locale} onChange={(event) => setLocale(event.target.value as Locale)}>
        <option value="ru">Русский</option>
        <option value="en">English</option>
      </select>
    </label>
  );
}
