import './style.css';
import './ui-panel.css';
import { themeStorageKey } from '../lib/theme';
import BackToTop from './components/BackToTop';
import LocaleProvider from './components/LocaleProvider';
export default function Layout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="ru" suppressHydrationWarning>
      <head>
        <link rel="icon" type="image/png" sizes="32x32" href="/favicon-32x32.png" />
        <link rel="icon" type="image/png" sizes="16x16" href="/favicon-16x16.png" />
        <link rel="apple-touch-icon" sizes="180x180" href="/apple-touch-icon.png" />
        <script>{`try { if (localStorage.getItem('${themeStorageKey}') === 'dark') document.documentElement.dataset.theme = 'dark'; } catch {}`}</script>
      </head>
      <body>
        <LocaleProvider>
          {children}
          <BackToTop />
        </LocaleProvider>
      </body>
    </html>
  );
}
