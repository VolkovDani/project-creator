'use client';
import { type ReactNode, useEffect, useId, useRef } from 'react';
import { type Locale, t } from '../../lib/i18n';

export default function ConfirmationDialog({
  title,
  children,
  confirmLabel,
  destructive,
  locale,
  onCancel,
  onConfirm,
}: {
  title: string;
  children: ReactNode;
  confirmLabel: string;
  destructive: boolean;
  locale: Locale;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const heading = useId();
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
  return (
    <dialog
      ref={dialog}
      aria-labelledby={heading}
      onCancel={(event) => {
        event.preventDefault();
        dismiss();
      }}
    >
      <h3 id={heading}>{title}</h3>
      {children}
      <button type="button" ref={cancel} onClick={dismiss}>
        {t(locale, 'requirements.cancel')}
      </button>
      <button
        type="button"
        className={destructive ? 'destructive-action' : undefined}
        onClick={onConfirm}
      >
        {confirmLabel}
      </button>
    </dialog>
  );
}
