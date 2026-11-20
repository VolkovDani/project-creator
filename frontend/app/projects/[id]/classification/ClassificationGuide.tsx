import rules from '../../../../../contracts/classification.json';
import { type Locale, t } from '../../../../lib/i18n';
export default function ClassificationGuide({ locale }: { locale: Locale }) {
  return (
    <details>
      <summary>{t(locale, 'review.guide')}</summary>
      <p>{t(locale, 'review.guideHint')}</p>
      {(['semantic', 'representation'] as const).map((axis) => (
        <section key={axis}>
          <h3>{t(locale, axis === 'semantic' ? 'review.meaning' : 'review.form')}</h3>
          <dl>
            {rules[axis].map((code) => (
              <div key={code}>
                <dt>
                  <strong>{t(locale, `classification.${code}`)}</strong>
                </dt>
                <dd>
                  {t(locale, `classification.guide.${code}.clue`)}
                  <ul>
                    {(['example', 'counterexample'] as const).map((kind) => (
                      <li key={kind}>
                        {t(locale, `review.${kind}`)} ·{' '}
                        {t(locale, `classification.guide.${code}.${kind}`)}
                      </li>
                    ))}
                  </ul>
                </dd>
              </div>
            ))}
          </dl>
        </section>
      ))}
    </details>
  );
}
