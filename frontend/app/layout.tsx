import './tokens.css';
import './style.css';
import './ui-panel.css';
import { themeStorageKey } from '../lib/theme';
import appleTouchIcon from '../public/apple-touch-icon.png';
import favicon16 from '../public/favicon-16x16.png';
import favicon32 from '../public/favicon-32x32.png';
import BackToTop from './components/BackToTop';
import LocaleProvider from './components/LocaleProvider';
export default function Layout({ children }: { children: React.ReactNode }) {
  return (
    <html data-theme="dark" lang="ru" suppressHydrationWarning>
      <head>
        <link rel="icon" type="image/png" sizes="32x32" href={favicon32.src} />
        <link rel="icon" type="image/png" sizes="16x16" href={favicon16.src} />
        <link rel="apple-touch-icon" sizes="180x180" href={appleTouchIcon.src} />
        <script>{`try { if (localStorage.getItem('${themeStorageKey}') === 'light') document.documentElement.dataset.theme = 'light'; } catch {}`}</script>
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
