'use client';
import { useEffect, useRef } from 'react';
import { type Locale, t } from '../../../lib/i18n';

export default function RequirementConfirmation({
  action,
  text,
  locale,
  onCancel,
  onConfirm,
}: {
  action: 'archive' | 'delete';
  text: string;
  locale: Locale;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const cancel = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const element = dialog.current;
    element?.showModal();
    cancel.current?.focus();
    return () => element?.close();
  }, []);
  function dismiss() {
    dialog.current?.close();
    onCancel();
  }
  const deleting = action === 'delete';
  const characters = Array.from(text);
  return (
    <dialog
      ref={dialog}
      aria-labelledby="archive-action-title"
      onCancel={(event) => {
        event.preventDefault();
        dismiss();
      }}
    >
      <h3 id="archive-action-title">
        {t(locale, deleting ? 'requirements.purgeQuestion' : 'requirements.archiveQuestion')}
      </h3>
      <p className="user-content">
        {characters.slice(0, 180).join('')}
        {characters.length > 180 ? '…' : ''}
      </p>
      <p>{t(locale, deleting ? 'requirements.purgeHint' : 'requirements.archiveHint')}</p>
      <button type="button" ref={cancel} onClick={dismiss}>
        {t(locale, 'requirements.cancel')}
      </button>
      <button
        type="button"
        className={deleting ? 'destructive-action' : undefined}
        onClick={onConfirm}
      >
        {t(locale, deleting ? 'requirements.purge' : 'requirements.confirmArchive')}
      </button>
    </dialog>
  );
}
