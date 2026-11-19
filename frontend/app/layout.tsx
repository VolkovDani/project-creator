import './style.css';
import './ui-panel.css';
import { themeStorageKey } from '../lib/theme';
import BackToTop from './components/BackToTop';
import LocaleProvider from './components/LocaleProvider';
export default function Layout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="ru" suppressHydrationWarning>
      <head>
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
