'use client';

import {
  type KeyboardEvent,
  type PointerEvent,
  useCallback,
  useEffect,
  useRef,
  useState,
  useSyncExternalStore,
} from 'react';
import { type Locale, t } from '../../lib/i18n';
import { diagramPng, type RenderedDiagram, renderMermaid } from '../../lib/renderMermaid';
import styles from './mermaid-viewer.module.css';

type NodeAction = { label: string; onSelect: () => void };
type ViewerProps = {
  diagramId: string;
  source: string;
  locale: Locale;
  nodeActions?: ReadonlyMap<string, NodeAction>;
};
type Preview = { source: string; diagram: RenderedDiagram | null; error?: true };
const fit = { x: 0, y: 0, zoom: 1 };
const panKeys = new Map([
  ['ArrowLeft', [40, 0]],
  ['ArrowRight', [-40, 0]],
  ['ArrowUp', [0, 40]],
  ['ArrowDown', [0, -40]],
]);

function subscribeTheme(notify: () => void) {
  const observer = new MutationObserver(notify);
  observer.observe(document.documentElement, { attributeFilter: ['data-theme'] });
  return () => observer.disconnect();
}

function download(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  link.click();
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}

export function MermaidViewer({ diagramId, ...props }: ViewerProps) {
  return <DiagramPreview key={diagramId} {...props} />;
}

function DiagramPreview({ source, locale, nodeActions }: Omit<ViewerProps, 'diagramId'>) {
  const dark = useSyncExternalStore(
    subscribeTheme,
    () => document.documentElement.dataset.theme === 'dark',
    () => true,
  );
  const [preview, setPreview] = useState<Preview>();
  const [view, setView] = useState(fit);
  const [fullscreen, setFullscreen] = useState(false);
  const [feedback, setFeedback] = useState('');
  const [copying, setCopying] = useState(false);
  const dialog = useRef<HTMLDialogElement>(null);
  const fullscreenButton = useRef<HTMLButtonElement>(null);
  const restoreFocus = useRef(false);
  const menu = useRef<HTMLDetailsElement>(null);
  const menuButton = useRef<HTMLElement>(null);
  const drag = useRef<{ id: number; x: number; y: number; startX: number; startY: number }>(null);
  const dragged = useRef(false);
  const { diagram } = preview ?? {};
  const label = t(locale, 'mermaid.label');
  const loading = preview?.source !== source;
  const actions = loading ? undefined : nodeActions;

  useEffect(() => {
    const controller = new AbortController();
    const { signal } = controller;
    renderMermaid(source, dark, signal).then(
      (diagram) => {
        if (!signal.aborted) setPreview({ source, diagram });
      },
      () => {
        if (!signal.aborted) setPreview({ source, diagram: null, error: true });
      },
    );
    return () => controller.abort();
  }, [source, dark]);

  useEffect(() => {
    if (fullscreen) {
      const element = dialog.current;
      element?.showModal();
      fullscreenButton.current?.focus();
      restoreFocus.current = true;
      return () => element?.close();
    }
    if (restoreFocus.current) {
      fullscreenButton.current?.focus();
      restoreFocus.current = false;
    }
  }, [fullscreen]);

  const draw = useCallback(
    (host: HTMLDivElement | null) => {
      const frame = host?.parentElement;
      if (!host || !frame) return;
      const shadow = host.shadowRoot ?? host.attachShadow({ mode: 'open' });
      if (!diagram) {
        shadow.replaceChildren();
        return;
      }
      const active = shadow.activeElement?.getAttribute('data-diagram-node');
      const svg = diagram.svg.cloneNode(true) as SVGSVGElement;
      svg.setAttribute('role', actions?.size ? 'group' : 'img');
      svg.setAttribute('aria-label', label);
      if (actions?.size) {
        for (const node of svg.querySelectorAll<SVGGElement>('g[data-diagram-node]')) {
          const id = node.dataset.diagramNode;
          const action = id && actions.get(id);
          if (!action) continue;
          node.setAttribute('role', 'button');
          node.setAttribute('tabindex', '0');
          node.setAttribute('aria-label', action.label);
          node.addEventListener('click', action.onSelect);
          node.addEventListener('keydown', (event) => {
            if ((event.key !== 'Enter' && event.key !== ' ') || event.repeat) return;
            event.preventDefault();
            action.onSelect();
          });
          node.addEventListener('focus', () => {
            const bounds = frame.getBoundingClientRect();
            const card = node.getBoundingClientRect();
            if (
              card.left < bounds.left ||
              card.right > bounds.right ||
              card.top < bounds.top ||
              card.bottom > bounds.bottom
            ) {
              setView((previous) => ({
                ...previous,
                x: previous.x + (bounds.left + bounds.right - card.left - card.right) / 2,
                y: previous.y + (bounds.top + bounds.bottom - card.top - card.bottom) / 2,
              }));
            }
          });
        }
      }
      const chrome = document.createElement('style');
      chrome.textContent =
        'svg{display:block} [role=button]{cursor:pointer} [role=button]:focus-visible{outline:2px solid var(--focus);outline-offset:2px}';
      shadow.replaceChildren(chrome, svg);
      if (active) {
        Array.from(svg.querySelectorAll<SVGElement>('[role="button"]'))
          .find((node) => node.getAttribute('data-diagram-node') === active)
          ?.focus();
      }
    },
    [diagram, label, actions],
  );

  function zoomBy(factor: number) {
    setView((previous) => ({
      ...previous,
      zoom: Math.min(8, Math.max(0.25, previous.zoom * factor)),
    }));
  }

  function onKeyDown(event: KeyboardEvent<HTMLElement>) {
    const delta = panKeys.get(event.key);
    if (!delta || !diagram) return;
    event.preventDefault();
    setView((previous) => ({ ...previous, x: previous.x + delta[0], y: previous.y + delta[1] }));
  }

  function startPan(event: PointerEvent<HTMLElement>) {
    if (!diagram || !event.isPrimary || event.button !== 0) return;
    dragged.current = false;
    drag.current = {
      id: event.pointerId,
      x: event.clientX,
      y: event.clientY,
      startX: view.x,
      startY: view.y,
    };
  }

  function movePan(event: PointerEvent<HTMLElement>) {
    const pointer = drag.current;
    if (!pointer || event.pointerId !== pointer.id) return;
    const x = event.clientX - pointer.x;
    const y = event.clientY - pointer.y;
    if (!dragged.current) {
      if (Math.hypot(x, y) < 4) return;
      event.currentTarget.setPointerCapture(pointer.id);
      dragged.current = true;
    }
    setView((previous) => ({ ...previous, x: pointer.startX + x, y: pointer.startY + y }));
  }

  function endPan(event: PointerEvent<HTMLElement>) {
    if (drag.current?.id === event.pointerId) drag.current = null;
  }

  function closeMenu() {
    if (menu.current) menu.current.open = false;
    menuButton.current?.focus();
  }

  async function copyImage() {
    if (!diagram || copying || loading) return;
    closeMenu();
    setCopying(true);
    setFeedback('');
    const bg = getComputedStyle(document.documentElement).getPropertyValue('--surface').trim();
    const image = diagramPng(diagram, bg);
    try {
      await navigator.clipboard.write([new ClipboardItem({ 'image/png': image })]);
      setFeedback('mermaid.imageCopied');
    } catch {
      try {
        download(await image, 'diagram.png');
        setFeedback('mermaid.imageDownloaded');
      } catch {
        setFeedback('mermaid.imageError');
      }
    } finally {
      setCopying(false);
    }
  }

  async function copySource() {
    closeMenu();
    setFeedback('');
    try {
      await navigator.clipboard.writeText(source);
      setFeedback('mermaid.sourceCopied');
    } catch {
      try {
        download(new Blob([source], { type: 'text/plain;charset=utf-8' }), 'diagram.mmd');
        setFeedback('mermaid.sourceDownloaded');
      } catch {
        setFeedback('mermaid.sourceError');
      }
    }
  }

  const controls = (
    <>
      <div className={styles.toolbar}>
        <button type="button" disabled={!diagram || view.zoom >= 8} onClick={() => zoomBy(1.25)}>
          {t(locale, 'mermaid.zoomIn')}
        </button>
        <button type="button" disabled={!diagram || view.zoom <= 0.25} onClick={() => zoomBy(0.8)}>
          {t(locale, 'mermaid.zoomOut')}
        </button>
        <button
          type="button"
          disabled={!diagram || (view.x === 0 && view.y === 0 && view.zoom === 1)}
          onClick={() => setView(fit)}
        >
          {t(locale, 'mermaid.fit')}
        </button>
        <button
          type="button"
          ref={fullscreenButton}
          disabled={!diagram && !fullscreen}
          onClick={() => setFullscreen(!fullscreen)}
        >
          {t(locale, fullscreen ? 'mermaid.exitFullscreen' : 'mermaid.fullscreen')}
        </button>
        <details className={styles.copy} ref={menu}>
          <summary ref={menuButton}>{t(locale, 'mermaid.copy')}</summary>
          <div className={styles.copyActions}>
            <button type="button" disabled={!diagram || loading || copying} onClick={copyImage}>
              {t(locale, 'mermaid.copyImage')}
            </button>
            <button type="button" disabled={!diagram || loading} onClick={copySource}>
              {t(locale, 'mermaid.copySource')}
            </button>
          </div>
        </details>
      </div>
      <section
        aria-label={t(locale, 'mermaid.viewport')}
        aria-busy={!diagram && loading}
        tabIndex={diagram ? 0 : undefined}
        className={styles.viewport}
        data-ready={!!diagram}
        onKeyDown={onKeyDown}
        onPointerDown={startPan}
        onPointerMove={movePan}
        onPointerUp={endPan}
        onPointerCancel={endPan}
        onLostPointerCapture={endPan}
        onClickCapture={(event) => {
          if (dragged.current && event.detail > 0) event.stopPropagation();
        }}
      >
        <div
          className={styles.drawing}
          ref={draw}
          style={{ transform: `translate(${view.x}px, ${view.y}px) scale(${view.zoom})` }}
        />
        {!diagram && (
          <p role="status">
            {t(
              locale,
              loading ? 'mermaid.loading' : preview?.error ? 'mermaid.error' : 'mermaid.empty',
            )}
          </p>
        )}
      </section>
      <div className={styles.status} role="status">
        {feedback && t(locale, feedback)}
      </div>
    </>
  );
  return (
    <section aria-label={label} className={styles.viewer}>
      {fullscreen ? (
        <dialog
          ref={dialog}
          className={styles.fullscreen}
          aria-label={label}
          onCancel={(event) => {
            event.preventDefault();
            setFullscreen(false);
          }}
        >
          {controls}
        </dialog>
      ) : (
        controls
      )}
    </section>
  );
}
