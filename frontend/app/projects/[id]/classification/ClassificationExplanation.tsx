'use client';

import { useEffect, useRef, useState } from 'react';
import rules from '../../../../../contracts/classification.json';
import { query } from '../../../../lib/client';
import {
  type ClassificationFieldsFragment,
  ExplainRequirementClassificationDocument,
  type ExplainRequirementClassificationMutation,
} from '../../../../lib/generated';
import { fieldErrorMessage, type Locale, t } from '../../../../lib/i18n';

type Payload = ExplainRequirementClassificationMutation['explainRequirementClassification'];
type Result = NonNullable<Payload['result']>;

function Assessment({
  title,
  axis,
  locale,
}: {
  title: string;
  axis: Result['meaning'];
  locale: Locale;
}) {
  return (
    <section>
      <h4>{title}</h4>
      <strong>{t(locale, `explanation.verdict.${axis.verdict}`)}</strong>
      <p className="user-content">{axis.reason}</p>
      <ul>
        {axis.criteria.map((criterion) => (
          <li key={criterion.name}>
            <strong>{criterion.name}</strong> ·{' '}
            {t(locale, `explanation.criterion.${criterion.status}`)}
            {criterion.evidence && (
              <blockquote className="user-content">{criterion.evidence}</blockquote>
            )}
            {criterion.note && <p className="user-content">{criterion.note}</p>}
          </li>
        ))}
      </ul>
    </section>
  );
}

function SuggestedWording({ text, locale }: { text: string; locale: Locale }) {
  const paragraph = useRef<HTMLParagraphElement>(null);
  const [feedback, setFeedback] = useState('');
  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
      setFeedback('explanation.copied');
    } catch {
      if (!paragraph.current) return;
      window.getSelection()?.selectAllChildren(paragraph.current);
      setFeedback('glossary.copyManually');
    }
  }
  return (
    <section>
      <h4>{t(locale, 'explanation.wording')}</h4>
      <p ref={paragraph} className="user-content">
        {text}
      </p>
      <button type="button" onClick={copy}>
        {t(locale, 'explanation.copy')}
      </button>
      {feedback && <p role="status">{t(locale, feedback)}</p>}
    </section>
  );
}

export default function ClassificationExplanation({
  projectId,
  entry,
  suggestionRevision,
  locale,
}: {
  projectId: string;
  entry: ClassificationFieldsFragment;
  suggestionRevision: number;
  locale: Locale;
}) {
  const [targets, setTargets] = useState({
    semanticType: entry.semanticType,
    representationType: entry.representationType,
  });
  const pending = useRef<AbortController | null>(null);
  const [loading, setLoading] = useState(false);
  const [payload, setPayload] = useState<Payload>();
  useEffect(() => () => pending.current?.abort(), []);
  function cancel() {
    pending.current?.abort();
    pending.current = null;
    setLoading(false);
    setPayload(undefined);
  }
  async function explain() {
    if (pending.current) return;
    const controller = new AbortController();
    pending.current = controller;
    setLoading(true);
    setPayload(undefined);
    try {
      const { requirementId, textVersion, revision } = entry;
      const { explainRequirementClassification } = await query(
        ExplainRequirementClassificationDocument,
        {
          input: {
            projectId,
            requirementId,
            textVersion,
            revision,
            suggestionRevision,
            ...targets,
            locale,
          },
        },
        { tags: [], notify: false, signal: controller.signal },
      );
      if (!controller.signal.aborted) setPayload(explainRequirementClassification);
    } catch {
      if (!controller.signal.aborted)
        setPayload({ status: 'request_failed', result: null, fieldErrors: [] });
    } finally {
      if (!controller.signal.aborted) {
        pending.current = null;
        setLoading(false);
      }
    }
  }
  const result = payload?.status === 'ready' && payload.result;
  const error = payload?.fieldErrors[0];
  return (
    <>
      <p>{t(locale, 'explanation.prompt')}</p>
      {(['semanticType', 'representationType'] as const).map((key) => (
        <label key={key}>
          {t(locale, `explanation.${key}`)}
          <select
            value={targets[key]}
            onChange={(event) => {
              cancel();
              setTargets({ ...targets, [key]: event.target.value });
            }}
          >
            {(key === 'semanticType' ? rules.semantic : rules.representation).map((code) => (
              <option key={code} value={code}>
                {t(locale, `classification.${code}`)}
              </option>
            ))}
          </select>
        </label>
      ))}
      <button type="button" disabled={loading} onClick={explain}>
        {t(locale, 'explanation.request')}
      </button>
      {loading && (
        <>
          <button type="button" onClick={cancel}>
            {t(locale, 'requirements.cancel')}
          </button>
          <p role="status">{t(locale, 'explanation.loading')}</p>
        </>
      )}
      {payload && !result && (
        <p role="status">
          {error
            ? fieldErrorMessage(locale, error)
            : t(locale, `explanation.status.${payload.status}`)}
        </p>
      )}
      {result && (
        <>
          <p className="muted">
            {t(locale, 'explanation.source', {
              version: entry.textVersion,
              revision: suggestionRevision,
            })}
          </p>
          {(['meaning', 'form'] as const).map((axis) => {
            const target = axis === 'meaning' ? targets.semanticType : targets.representationType;
            return (
              <Assessment
                key={axis}
                title={`${t(locale, `review.${axis}`)} · ${t(locale, `classification.${target}`)}`}
                axis={result[axis]}
                locale={locale}
              />
            );
          })}
          {result.questions.length > 0 && (
            <section>
              <h4>{t(locale, 'explanation.questions')}</h4>
              <ul>
                {result.questions.map((question) => (
                  <li key={question}>{question}</li>
                ))}
              </ul>
            </section>
          )}
          {result.wording && <SuggestedWording text={result.wording} locale={locale} />}
          <h4>{t(locale, 'explanation.limitations')}</h4>
          <p className="user-content">{result.limitations}</p>
        </>
      )}
    </>
  );
}
