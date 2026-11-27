import { mermaidLimits, mermaidSource } from './mermaidSource';

export type RenderedDiagram = {
  svg: SVGSVGElement;
  width: number;
  height: number;
};

let renderQueue = Promise.resolve();
let renderId = 0;

export function renderMermaid(source: string, dark: boolean, signal: AbortSignal) {
  const result = renderQueue.then(async (): Promise<RenderedDiagram | null> => {
    signal.throwIfAborted();
    const diagram = mermaidSource(source);
    if (!diagram) return null;

    const [{ default: mermaid }, { default: purify }] = await Promise.all([
      import('mermaid'),
      import('dompurify'),
    ]);
    signal.throwIfAborted();

    // Mermaid's configuration is global. Keep initialization and rendering in the same queue.
    mermaid.initialize({
      startOnLoad: false,
      securityLevel: 'strict',
      htmlLabels: false,
      suppressErrorRendering: true,
      maxTextSize: mermaidLimits.maxTextSize,
      maxEdges: mermaidLimits.maxEdges,
      theme: dark ? 'dark' : 'default',
    });

    const staging = document.createElement('div');
    staging.setAttribute('aria-hidden', 'true');
    staging.style.cssText = 'position:fixed;left:-100000px;top:0;pointer-events:none';
    document.body.append(staging);
    try {
      const id = `mermaid-${++renderId}`;
      const { svg } = await mermaid.render(id, source, staging);
      signal.throwIfAborted();
      const fragment = purify.sanitize(svg, {
        USE_PROFILES: { svg: true, svgFilters: true },
        FORBID_TAGS: [
          'a',
          'image',
          'foreignObject',
          'script',
          'use',
          'animate',
          'animateMotion',
          'animateTransform',
          'set',
        ],
        FORBID_ATTR: ['href', 'xlink:href', 'src', 'data-diagram-node'],
        RETURN_DOM_FRAGMENT: true,
      });
      const element = fragment.firstElementChild;
      if (!(element instanceof SVGSVGElement)) throw new Error('Missing Mermaid SVG');
      const { width, height } = element.viewBox.baseVal;
      if (!(width > 0 && height > 0 && Number.isFinite(width) && Number.isFinite(height))) {
        throw new Error('Invalid Mermaid dimensions');
      }
      element.removeAttribute('style');
      element.setAttribute('width', '100%');
      element.setAttribute('height', '100%');
      const nodeIds = new Set(diagram.nodeIds);
      const prefix = `${id}-`;
      for (const node of element.querySelectorAll('g[id]')) {
        if (!node.id.startsWith(prefix)) continue;
        const domId = node.id.slice(prefix.length);
        const alias =
          diagram.kind === 'flowchart' ? /^flowchart-(.+)-[0-9]+$/.exec(domId)?.[1] : domId;
        if (alias && nodeIds.has(alias)) node.setAttribute('data-diagram-node', alias);
      }
      return { svg: element, width, height };
    } finally {
      staging.remove();
    }
  });
  renderQueue = result.then(
    () => undefined,
    () => undefined,
  );
  return result;
}

export async function diagramPng({ svg, width, height }: RenderedDiagram, background: string) {
  const scale = Math.min(2, 3600 / Math.max(width, height));
  const canvas = document.createElement('canvas');
  canvas.width = Math.max(1, Math.round(width * scale));
  canvas.height = Math.max(1, Math.round(height * scale));
  const context = canvas.getContext('2d');
  if (!context) throw new Error('Canvas unavailable');
  const copy = svg.cloneNode(true) as SVGSVGElement;
  copy.setAttribute('width', String(canvas.width));
  copy.setAttribute('height', String(canvas.height));
  const source = new XMLSerializer().serializeToString(copy);
  const url = URL.createObjectURL(new Blob([source], { type: 'image/svg+xml;charset=utf-8' }));
  try {
    const image = new Image();
    image.src = url;
    await image.decode();
    context.fillStyle = background;
    context.fillRect(0, 0, canvas.width, canvas.height);
    context.drawImage(image, 0, 0);
    return await new Promise<Blob>((resolve, reject) => {
      canvas.toBlob((blob) => {
        if (blob) resolve(blob);
        else reject(new Error('PNG encoding failed'));
      }, 'image/png');
    });
  } finally {
    URL.revokeObjectURL(url);
  }
}
