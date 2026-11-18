import './style.css';
import BackToTop from './components/BackToTop';
import LocaleProvider from './components/LocaleProvider';
export default function Layout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="ru">
      <body>
        <LocaleProvider>
          {children}
          <BackToTop />
        </LocaleProvider>
      </body>
    </html>
  );
}
