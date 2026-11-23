'use client';

import Link from 'next/link';
import { useCallback, useEffect, useRef, useState } from 'react';
import rules from '../../../../../contracts/classification.json';
import {
  classificationDraft,
  draftIsCurrent,
  eligibleSuggestion,
  type SavedSuggestion,
} from '../../../../lib/classificationReview';
import { query } from '../../../../lib/client';
import {
  type ClassificationFieldsFragment,
  ClassificationReviewCardDocument,
  ConfirmClassificationDocument,
  RequirementClassificationDocument,
  RetryClassificationDocument,
} from '../../../../lib/generated';
import { fieldErrorMessage, type Locale, t } from '../../../../lib/i18n';
import { useResource } from '../../../../lib/useResource';
import Group from '../../../components/Group';
import ResourceState from '../../../components/ResourceState';
import ClassificationHistory from './ClassificationHistory';
import DecisionEvidence from './DecisionEvidence';

type Props = {
  projectId: string;
  requirementId: string;
  locale: Locale;
  original?: string;
  version?: number;
  configured?: boolean;
  onEditing?: (editing: boolean) => void;
  selection?: {
    checked: boolean;
    disabled: boolean;
    change: (proposal: SavedSuggestion | undefined) => void;
  };
};
export default function ClassificationCard({
  projectId,
  requirementId,
  locale,
  original,
  version,
  onEditing,
  configured = false,
  selection,
}: Props) {
  const tag = `project:${projectId}:classification:${requirementId}`;
  const profileTag = `project:${projectId}:classification-profile`;
  const load = useCallback(async () => {
    const vars = { projectId, requirementId };
    if (original !== undefined && version !== undefined) {
      const { requirementClassification } = await query(RequirementClassificationDocument, vars, {
        tags: [tag, profileTag],
      });
      return { entry: requirementClassification, text: original, version };
    }
    const { requirementClassification, classificationListByIds } = await query(
      ClassificationReviewCardDocument,
      vars,
      { tags: [tag, profileTag, `project:${projectId}:requirements`] },
    );
    const source = classificationListByIds[0];
    return (
      source && { entry: requirementClassification, text: source.text, version: source.textVersion }
    );
  }, [projectId, requirementId, tag, profileTag, original, version]);
  const { data, loading, error, reload } = useResource(
    load,
    original === undefined
      ? [tag, profileTag, `project:${projectId}:requirements`]
      : [tag, profileTag],
  );
  return (
    <>
      {(error || (!data && loading)) && (
        <ResourceState
          locale={locale}
          loading={loading && !data}
          error={error}
          hasData={!!data}
          retry={() => void reload()}
        />
      )}
      {data?.entry && (
        <DecisionCard
          key={requirementId}
          projectId={projectId}
          entry={data.entry}
          original={data.text}
          version={data.version}
          locale={locale}
          showText={original === undefined}
          onEditing={onEditing}
          configured={configured}
          selection={selection}
        />
      )}
    </>
  );
}

function DecisionCard({
  projectId,
  entry,
  original,
  version,
  locale,
  showText,
  configured,
  selection,
  onEditing,
}: {
  projectId: string;
  entry: ClassificationFieldsFragment;
  original: string;
  version: number;
  locale: Locale;
  showText: boolean;
  configured: boolean;
  onEditing?: Props['onEditing'];
  selection?: Props['selection'];
}) {
  const [draft, setDraft] = useState<ReturnType<typeof classificationDraft>>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [runMessage, setRunMessage] = useState('');
  const dialog = useRef<HTMLDialogElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const open = !!draft;
  useEffect(() => {
    if (open && dialog.current && !dialog.current.open) dialog.current.showModal();
  }, [open]);
  const changed = !!draft && !draftIsCurrent(draft, entry, version);
  const detailTag = `project:${projectId}:classification:${entry.requirementId}`;
  async function requestModel() {
    if (busy) return;
    setBusy(true);
    setError('');
    setRunMessage('');
    try {
      const reanalyze = ['confirmed', 'suggested', 'needs_review'].includes(entry.status);
      const { retryRequirementClassification: response } = await query(
        RetryClassificationDocument,
        { projectId, requirementId: entry.requirementId, textVersion: version, reanalyze },
        { tags: [detailTag, `project:${projectId}:classifications`] },
      );
      if (response.fieldErrors[0]) setError(fieldErrorMessage(locale, response.fieldErrors[0]));
      else setRunMessage(t(locale, 'review.queuedCount', { count: response.queued }));
    } catch {
      setError(t(locale, 'app.error'));
    } finally {
      setBusy(false);
    }
  }
  function close() {
    dialog.current?.close();
    setDraft(undefined);
    setError('');
    onEditing?.(false);
    trigger.current?.focus({ preventScroll: true });
  }
  async function save(event: React.FormEvent) {
    event.preventDefault();
    if (!draft || busy || changed) return;
    setBusy(true);
    setError('');
    try {
      const response = await query(
        ConfirmClassificationDocument,
        { input: { projectId, ...draft } },
        { tags: [detailTag, `project:${projectId}:classifications`] },
      );
      const payload = response.confirmRequirementClassification;
      if (payload.fieldErrors[0]) setError(fieldErrorMessage(locale, payload.fieldErrors[0]));
      else if (payload.classification) close();
      else setError(t(locale, 'app.error'));
    } catch {
      setError(t(locale, 'app.error'));
    } finally {
      setBusy(false);
    }
  }
  return (
    <section
      aria-label={t(locale, 'nav.classification')}
      data-classification-id={entry.requirementId}
      className={showText ? 'card' : undefined}
    >
      {showText && (
        <>
          <p className="user-content">{original}</p>
          <Link
            href={`/projects/${encodeURIComponent(projectId)}/requirements?requirement=${encodeURIComponent(entry.requirementId)}`}
          >
            {t(locale, 'review.openSource')}
          </Link>
        </>
      )}
      {selection && (selection.checked || eligibleSuggestion(entry, version)) && (
        <label className="classification-selection">
          <input
            type="checkbox"
            checked={selection.checked}
            disabled={selection.disabled}
            onChange={(event) =>
              selection.change(
                event.target.checked ? classificationDraft(entry, version) : undefined,
              )
            }
          />
          {t(locale, 'review.selectSuggestion')}
        </label>
      )}
      <DecisionEvidence entry={entry} locale={locale} />
      {entry.textVersion !== version && (
        <p>{t(locale, 'review.stale', { previous: entry.textVersion, current: version })}</p>
      )}
      {entry.revision > 0 && (
        <Group
          id={detailTag + ':history'}
          title={t(locale, 'review.history')}
          defaultOpen={false}
          lazy
        >
          <ClassificationHistory
            projectId={projectId}
            requirementId={entry.requirementId}
            locale={locale}
          />
        </Group>
      )}
      {configured && !['queued', 'classifying'].includes(entry.status) && (
        <button
          type="button"
          disabled={busy || selection?.disabled}
          onClick={() => void requestModel()}
        >
          {t(
            locale,
            entry.status === 'confirmed'
              ? 'review.reanalyze'
              : entry.status === 'failed'
                ? 'review.retryOne'
                : 'review.analyze',
          )}
        </button>
      )}
      {!draft && error && <p role="alert">{error}</p>}
      {runMessage && <p role="status">{runMessage}</p>}
      <button
        ref={trigger}
        disabled={selection?.disabled}
        type="button"
        onClick={() => {
          setDraft(classificationDraft(entry, version));
          setError('');
          onEditing?.(true);
        }}
      >
        {t(locale, 'review.edit')}
      </button>
      {draft && (
        <dialog
          ref={dialog}
          aria-label={t(locale, 'review.edit')}
          onCancel={(event) => {
            event.preventDefault();
            if (!busy) close();
          }}
        >
          <h3>{t(locale, 'review.edit')}</h3>
          <p className="user-content">{original}</p>
          <form onSubmit={save}>
            {(['semanticType', 'representationType'] as const).map((key) => (
              <label key={key}>
                {t(locale, key === 'semanticType' ? 'review.meaning' : 'review.form')}
                <select
                  value={draft[key]}
                  disabled={busy}
                  onChange={(event) => setDraft({ ...draft, [key]: event.target.value })}
                >
                  {(key === 'semanticType' ? rules.semantic : rules.representation).map((code) => (
                    <option key={code} value={code}>
                      {t(locale, `classification.${code}`)}
                    </option>
                  ))}
                </select>
              </label>
            ))}
            {changed && (
              <>
                <p role="status">{t(locale, 'review.draftChanged')}</p>
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => {
                    setDraft({ ...draft, revision: entry.revision, textVersion: version });
                    setError('');
                  }}
                >
                  {t(locale, 'review.keepDraft')}
                </button>
              </>
            )}
            {error && <p role="alert">{error}</p>}
            <button type="submit" disabled={busy || changed}>
              {t(locale, 'review.save')}
            </button>
            <button type="button" disabled={busy} onClick={close}>
              {t(locale, 'requirements.cancel')}
            </button>
          </form>
        </dialog>
      )}
    </section>
  );
}
