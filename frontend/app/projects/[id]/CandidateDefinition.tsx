'use client';
import { useState } from 'react';
import { query } from '../../../lib/client';
import {
  type GlossaryCandidatePagesQuery,
  SuggestGlossaryDefinitionDocument,
  type SuggestGlossaryDefinitionMutation,
} from '../../../lib/generated';
import { fieldErrorMessage, type Locale, t } from '../../../lib/i18n';

export type Candidate =
  GlossaryCandidatePagesQuery['glossaryCandidatesConnection']['items'][number];
type Suggestion = SuggestGlossaryDefinitionMutation['suggestGlossaryDefinition'];

function feedback(locale: Locale, result: Suggestion) {
  if (result.fieldErrors.length) {
    return result.fieldErrors.map((error) => fieldErrorMessage(locale, error)).join(' ');
  }
  if (result.status === 'insufficient') return t(locale, 'glossary.insufficientContext');
  if (result.status === 'stale') return t(locale, 'glossary.staleCandidate');
  return t(locale, 'glossary.generationUnavailable');
}

export default function CandidateDefinition({
  projectId,
  candidate,
  name,
  locale,
  onApply,
}: {
  projectId: string;
  candidate: Candidate;
  name: string;
  locale: Locale;
  onApply: (text: string) => void;
}) {
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<Suggestion>();
  const preview = result?.status === 'suggested' && result.definition;
  async function generate() {
    setBusy(true);
    setResult(undefined);
    const { sourceId, sourceVersion, start, end } = candidate;
    try {
      const data = await query(
        SuggestGlossaryDefinitionDocument,
        { input: { projectId, sourceId, sourceVersion, start, end, name } },
        { tags: [], notify: false },
      );
      setResult(data.suggestGlossaryDefinition);
    } catch {
      setResult({ status: 'unavailable', definition: null, fieldErrors: [] });
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <button type="button" disabled={busy} onClick={generate}>
        {t(locale, 'glossary.generateDefinition')}
      </button>
      <div aria-live="polite">
        {busy && <p>{t(locale, 'glossary.generatingDefinition')}</p>}
        {preview ? (
          <>
            <strong>{t(locale, 'glossary.aiSuggestion')}</strong>
            <p className="user-content">{preview}</p>
            <button
              type="button"
              onClick={() => {
                onApply(preview);
                setResult(undefined);
              }}
            >
              {t(locale, 'glossary.applyDefinition')}
            </button>
          </>
        ) : (
          result && <p>{feedback(locale, result)}</p>
        )}
      </div>
    </>
  );
}
