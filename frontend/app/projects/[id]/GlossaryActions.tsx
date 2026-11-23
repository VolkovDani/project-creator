'use client';
import { createContext, type ReactNode, useContext, useEffect, useState } from 'react';
import { createPortal } from 'react-dom';
import { type Locale, t } from '../../../lib/i18n';
import GlossaryEditor, { type GlossaryDraft } from './GlossaryEditor';

const EditorContext = createContext<(draft: GlossaryDraft) => void>(() => {});
export const useGlossaryEditor = () => useContext(EditorContext);

export function GlossaryText({
  children,
  sourceId,
  version,
  className,
  as: Element = 'p',
}: {
  children: ReactNode;
  sourceId?: string;
  version?: number;
  className?: string;
  as?: 'p' | 'div';
}) {
  return (
    <>
      <Element
        className={className}
        data-glossary-text=""
        data-source-id={sourceId}
        data-source-version={version}
      >
        {children}
      </Element>
      <span data-glossary-action="" />
    </>
  );
}

export function GlossaryActions({
  children,
  projectId,
  locale,
}: {
  children: ReactNode;
  projectId: string;
  locale: Locale;
}) {
  const [draft, setDraft] = useState<GlossaryDraft>();
  const [selection, setSelection] = useState<{ target: Element; draft: GlossaryDraft }>();
  useEffect(() => {
    function update() {
      const current = window.getSelection();
      const root = (node: Node | null | undefined) =>
        (node instanceof Element ? node : node?.parentElement)?.closest<HTMLElement>(
          '[data-glossary-text]',
        );
      const block = root(current?.anchorNode);
      const target = block?.nextElementSibling;
      const name = current?.toString().trim();
      if (
        name &&
        block &&
        target?.hasAttribute('data-glossary-action') &&
        root(current?.focusNode) === block &&
        current?.rangeCount &&
        !Array.from(block.querySelectorAll('[data-glossary-exclude]')).some((element) =>
          current.getRangeAt(0).intersectsNode(element),
        )
      ) {
        setSelection({
          target,
          draft: {
            name,
            sourceId: block.dataset.sourceId,
            sourceVersion: block.dataset.sourceVersion
              ? Number(block.dataset.sourceVersion)
              : undefined,
          },
        });
      } else setSelection(undefined);
    }
    document.addEventListener('selectionchange', update);
    return () => document.removeEventListener('selectionchange', update);
  }, []);
  return (
    <EditorContext.Provider value={setDraft}>
      {children}
      {!draft &&
        selection?.target.isConnected &&
        createPortal(
          <button
            type="button"
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => setDraft(selection.draft)}
          >
            {t(locale, 'glossary.addSelection')}
          </button>,
          selection.target,
        )}
      {draft && (
        <GlossaryEditor
          projectId={projectId}
          locale={locale}
          draft={draft}
          onClose={() => setDraft(undefined)}
        />
      )}
    </EditorContext.Provider>
  );
}
