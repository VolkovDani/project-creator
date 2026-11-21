import { t } from '../../lib/i18n';
import { useLocale } from './LocaleProvider';

export default function PageMetadata({
  titleKey,
  descriptionKey = 'app.metaDescription',
}: {
  titleKey: string;
  descriptionKey?: string;
}) {
  const { locale } = useLocale();
  return (
    <>
      <title>{t(locale, 'app.pageTitle', { page: t(locale, titleKey) })}</title>
      <meta name="description" content={t(locale, descriptionKey)} />
    </>
  );
}
