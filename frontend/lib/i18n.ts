import en from '../../contracts/i18n/en.json';
import ru from '../../contracts/i18n/ru.json';
export type Locale = 'ru' | 'en';
const dictionaries = { ru, en };
export function t(
  locale: Locale,
  key: string,
  params: Record<string, number | string> = {},
): string {
  const dict: Record<string, string> = dictionaries[locale];
  const template = dict[key] || dict['app.validationUnknown'];
  return template.replace(/\{(\w+)\}/g, (_, k: string) => String(params[k] ?? ''));
}

export function fieldErrorMessage(
  locale: Locale,
  error: { messageKey: string; params: { key: string; value: number }[] },
) {
  return t(locale, error.messageKey, Object.fromEntries(error.params.map((p) => [p.key, p.value])));
}

export function summaryErrorMessage(locale: Locale, code: string) {
  const key = `summary.error.${code}`;
  return Object.hasOwn(dictionaries[locale], key)
    ? t(locale, key)
    : t(locale, 'summary.error', { code });
}
