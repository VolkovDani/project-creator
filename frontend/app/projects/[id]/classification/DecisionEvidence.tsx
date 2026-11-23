import { groupAlternatives } from '../../../../lib/classificationReview';
import type { ClassificationFieldsFragment } from '../../../../lib/generated';
import { type Locale, t } from '../../../../lib/i18n';

const knownReasons = new Set([
  'uncalibrated',
  'outside_training_context',
  'input_limit',
  'reserved_token',
]);
const knownErrors = new Set([
  'NOT_CONFIGURED',
  'CLASSIFIER_UNAVAILABLE',
  'INVALID_CLASSIFIER_RESULT',
  'INTERRUPTED',
]);
export default function DecisionEvidence({
  entry,
  locale,
}: {
  entry: ClassificationFieldsFragment;
  locale: Locale;
}) {
  const provenance = [
    ['review.profile', entry.profile],
    ['review.rules', entry.rulesVersion],
    ['review.question', entry.questionVersion],
    ['review.digest', entry.digest],
  ].filter((pair) => pair[1]);
  const alternatives = groupAlternatives(entry.alternatives);
  const visibleCount =
    alternatives.meaning.length + alternatives.form.length + alternatives.joint.length;
  return (
    <>
      <dl className="classification-fields">
        <div>
          <dt>{t(locale, 'review.meaning')}</dt>
          <dd>{t(locale, `classification.${entry.semanticType}`)}</dd>
        </div>
        <div>
          <dt>{t(locale, 'review.form')}</dt>
          <dd>{t(locale, `classification.${entry.representationType}`)}</dd>
        </div>
        <div>
          <dt>{t(locale, 'review.status')}</dt>
          <dd>{t(locale, `classification.${entry.status}`)}</dd>
        </div>
        <div>
          <dt>{t(locale, 'review.source')}</dt>
          <dd>
            {t(
              locale,
              entry.source === 'user'
                ? 'review.manual'
                : entry.source === 'model'
                  ? 'review.automatic'
                  : 'review.none',
            )}
            {entry.model && <> · {entry.model}</>}
          </dd>
        </div>
      </dl>
      {entry.errorCode && (
        <p className={entry.errorCode === 'NOT_CONFIGURED' ? 'muted' : 'error'}>
          {knownErrors.has(entry.errorCode)
            ? t(locale, `review.error.${entry.errorCode}`)
            : t(locale, 'review.errorCode')}
          {entry.errorCode !== 'NOT_CONFIGURED' && (
            <>
              {' '}
              · <code>{entry.errorCode}</code>
            </>
          )}
        </p>
      )}
      {entry.reason && (
        <p className="muted">
          {knownReasons.has(entry.reason) ? (
            t(locale, `review.reason.${entry.reason}`)
          ) : (
            <>
              {t(locale, 'review.reason')} · {entry.reason}
            </>
          )}
        </p>
      )}
      {visibleCount > 0 && (
        <details>
          <summary>
            {t(locale, 'review.alternatives')} ({visibleCount})
          </summary>
          {(['meaning', 'form', 'joint'] as const).map(
            (axis) =>
              alternatives[axis].length > 0 && (
                <div key={axis}>
                  <h4>{t(locale, `review.${axis}`)}</h4>
                  <ol>
                    {alternatives[axis].map((item) => (
                      <li key={item.semanticType + '-' + item.representationType}>
                        {[item.semanticType, item.representationType]
                          .filter(Boolean)
                          .map((code) => t(locale, `classification.${code}`))
                          .join(' / ')}
                        {item.score !== null && (
                          <>
                            {' '}
                            ·{' '}
                            {t(locale, 'review.score', {
                              score: item.score.toLocaleString(locale, {
                                maximumSignificantDigits: 5,
                              }),
                            })}
                          </>
                        )}
                      </li>
                    ))}
                  </ol>
                </div>
              ),
          )}
        </details>
      )}
      {provenance.length > 0 && (
        <details>
          <summary>{t(locale, 'review.details')}</summary>
          <dl>
            {provenance.map(
              ([key, value]) =>
                key && (
                  <div key={key}>
                    <dt>{t(locale, key)}</dt>
                    <dd>{value}</dd>
                  </div>
                ),
            )}
          </dl>
        </details>
      )}
    </>
  );
}
