'use client';
import { useEffect, useId, useLayoutEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import type { GlossaryFieldsFragment as Term } from '../../../lib/generated';
import { type Locale, t } from '../../../lib/i18n';
import { useGlossaryEditor } from './GlossaryActions';

export default function GlossaryMatch({
  text,
  terms,
  locale,
  loading,
  error,
  reload,
}: {
  text: string;
  terms: Term[];
  locale: Locale;
  loading: boolean;
  error: boolean;
  reload: () => Promise<void>;
}) {
  const [mode, setMode] = useState<'preview' | 'pinned'>();
  const trigger = useRef<HTMLButtonElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const keyboardActionPending = useRef(false);
  const id = useId();
  const edit = useGlossaryEditor();
  const open = !!mode;
  function preview() {
    clearTimeout(timer.current);
    setMode((value) => value ?? 'preview');
  }
  function leave() {
    clearTimeout(timer.current);
    timer.current = setTimeout(() => {
      if (document.activeElement !== trigger.current)
        setMode((value) => (value === 'pinned' ? value : undefined));
    }, 180);
  }
  useEffect(() => () => clearTimeout(timer.current), []);
  useLayoutEffect(() => {
    if (!open || !trigger.current || !panel.current) return;
    const anchor = trigger.current,
      box = panel.current;
    function position() {
      const bounds = anchor.getBoundingClientRect();
      if (bounds.bottom < 0 || bounds.top > window.innerHeight) {
        setMode(undefined);
        return;
      }
      const left = Math.max(8, Math.min(bounds.left, window.innerWidth - box.offsetWidth - 8));
      const top =
        bounds.bottom + box.offsetHeight + 12 <= window.innerHeight
          ? bounds.bottom + 4
          : Math.max(8, bounds.top - box.offsetHeight - 4);
      box.style.left = `${left}px`;
      box.style.top = `${top}px`;
    }
    position();
    const observer = new ResizeObserver(position);
    observer.observe(box);
    observer.observe(anchor);
    window.addEventListener('scroll', position, true);
    window.addEventListener('resize', position);
    function outside(e: Event) {
      if (e.target instanceof Node && !box.contains(e.target) && !anchor.contains(e.target))
        setMode(undefined);
    }
    function dismissOnEscape(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        if (box.contains(document.activeElement)) anchor.focus();
        setMode(undefined);
      }
    }
    document.addEventListener('pointerdown', outside);
    document.addEventListener('focusin', outside);
    document.addEventListener('keydown', dismissOnEscape);
    return () => {
      observer.disconnect();
      window.removeEventListener('scroll', position, true);
      window.removeEventListener('resize', position);
      document.removeEventListener('pointerdown', outside);
      document.removeEventListener('focusin', outside);
      document.removeEventListener('keydown', dismissOnEscape);
    };
  }, [open]);
  useLayoutEffect(() => {
    if (!mode) keyboardActionPending.current = false;
    if (mode !== 'pinned' || !keyboardActionPending.current || loading || (!error && !terms.length))
      return;
    keyboardActionPending.current = false;
    panel.current?.querySelector('button')?.focus();
  }, [mode, loading, error, terms.length]);
  return (
    <>
      <button
        ref={trigger}
        type="button"
        className="glossary-match"
        aria-describedby={open ? id : undefined}
        onPointerEnter={(e) => {
          if (e.pointerType !== 'touch') preview();
        }}
        onPointerLeave={leave}
        onFocus={preview}
        onClick={(e) => {
          keyboardActionPending.current = e.detail === 0;
          const action = mode === 'pinned' ? panel.current?.querySelector('button') : undefined;
          if (keyboardActionPending.current && action) {
            keyboardActionPending.current = false;
            action.focus();
          }
          setMode('pinned');
        }}
      >
        {text}
      </button>
      {mode &&
        createPortal(
          <div
            ref={panel}
            className="glossary-tooltip"
            onPointerEnter={() => clearTimeout(timer.current)}
            onPointerLeave={leave}
          >
            <div role="tooltip" id={id}>
              {loading ? (
                <p role="status">{t(locale, 'glossary.definitionsLoading')}</p>
              ) : error ? (
                <p role="status">{t(locale, 'glossary.definitionsError')}</p>
              ) : terms.length === 0 ? (
                <p>{t(locale, 'glossary.definitionsUnavailable')}</p>
              ) : (
                terms.map((term) => (
                  <div key={term.id}>
                    <strong>{term.name}</strong>
                    {term.definition && <p>{term.definition}</p>}
                    {term.context && <p>{term.context}</p>}
                  </div>
                ))
              )}
            </div>
            {mode === 'pinned' && error && (
              <button
                type="button"
                onClick={(e) => {
                  keyboardActionPending.current = e.detail === 0;
                  trigger.current?.focus();
                  void reload();
                }}
              >
                {t(locale, 'glossary.definitionsRetry')}
              </button>
            )}
            {mode === 'pinned' &&
              !loading &&
              !error &&
              terms.map((term) => (
                <button
                  type="button"
                  key={term.id}
                  onClick={() => {
                    setMode(undefined);
                    edit({ name: term.name, term });
                  }}
                >
                  {t(locale, 'glossary.edit')}
                  {terms.length > 1 && ` · ${term.name}`}
                </button>
              ))}
          </div>,
          document.body,
        )}
    </>
  );
}
