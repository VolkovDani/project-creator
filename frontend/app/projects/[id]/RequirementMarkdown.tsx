'use client';

import { astToJSX, type MarkdownToJSX, parser, RuleType } from 'markdown-to-jsx/react';
import { Fragment, useMemo, useRef, useState } from 'react';
import { parse } from 'sugar-high/core';
import { lang, languages } from 'sugar-high/lang';
import { type Locale, t } from '../../../lib/i18n';
import { GlossaryText } from './GlossaryActions';
import { type GlossaryMatches, MatchedText, type RenderMatch } from './MatchedText';
import { markdownGlossary } from './markdownGlossary';
import './markdown.css';

function destination(value: string) {
  try {
    const url = new URL(value, 'https://markdown.invalid');
    return ['https:', 'http:', 'mailto:'].includes(url.protocol) ? value : null;
  } catch {
    return null;
  }
}

const options = {
  disableParsingRawHTML: true,
  disableFrontmatter: true,
  enforceAtxHeadings: true,
  forceBlock: true,
  sanitizer: destination,
};

function CodeBlock({
  text,
  language,
  locale,
}: {
  text: string;
  language?: string;
  locale: Locale;
}) {
  const code = useRef<HTMLElement>(null);
  const [status, setStatus] = useState('');
  const tokens = useMemo(() => {
    const name = language && lang(language);
    const config = languages.find((item) => item.id === name)?.config;
    if (!config || name === 'plaintext') return text;
    return parse(text, config).lines.map((line) => {
      let offset = 0;
      return (
        <Fragment key={line.index}>
          {line.index > 0 && '\n'}
          {line.tokens.map((token) => {
            const start = offset;
            offset += token.value.length;
            return (
              <span key={start} className={`md-${token.type}`}>
                {token.value}
              </span>
            );
          })}
        </Fragment>
      );
    });
  }, [text, language]);
  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
      setStatus('markdown.copied');
    } catch {
      if (code.current) window.getSelection()?.selectAllChildren(code.current);
      setStatus('glossary.copyManually');
    }
  }
  return (
    <div className="markdown-code">
      <div className="markdown-code-tools" data-glossary-exclude="">
        {language && <span>{language}</span>}
        <button type="button" onClick={copy}>
          {t(locale, 'markdown.copy')}
        </button>
      </div>
      <pre>
        <code ref={code}>{tokens}</code>
      </pre>
      {status && (
        <p role="status" data-glossary-exclude="">
          {t(locale, status)}
        </p>
      )}
    </div>
  );
}

function formatted(
  ast: MarkdownToJSX.ASTNode[],
  locale: Locale,
  projected: ReturnType<typeof markdownGlossary>,
  renderMatch: RenderMatch,
) {
  return astToJSX(ast, {
    ...options,
    wrapper: null,
    renderRule(next, node, _children, state) {
      const segments = projected.get(node);
      if (segments)
        return (
          <Fragment key={state.key}>
            {segments.map((part, index) =>
              typeof part === 'string' ? part : renderMatch(part.match, part.label, index),
            )}
          </Fragment>
        );
      if (node.type === RuleType.image) return node.alt || '';
      if (node.type === RuleType.codeBlock)
        return <CodeBlock key={state.key} text={node.text} language={node.lang} locale={locale} />;
      return next();
    },
  });
}

const emptyMatches: GlossaryMatches = [];

export function RequirementMarkdown({
  text,
  locale,
  sourceId,
  version,
  projectId,
  matches = emptyMatches,
  highlight,
}: {
  sourceId?: string;
  version?: number;
  projectId: string;
  matches?: GlossaryMatches;
  highlight?: { start: number; end: number };
  text: string;
  locale: Locale;
}) {
  const [source, setSource] = useState(false);
  const ast = useMemo(() => parser(text, options), [text]);
  const hasMarkup =
    ast.length > 1 ||
    ast.some(
      (node) =>
        node.type !== RuleType.paragraph ||
        node.children.some((part) => part.type !== RuleType.text),
    );
  const projected = useMemo(
    () =>
      markdownGlossary(
        text,
        ast,
        hasMarkup && !highlight && !source ? matches : emptyMatches,
        options,
      ),
    [text, ast, hasMarkup, highlight, source, matches],
  );
  const matchedProps = { projectId, text, matches, locale };
  const exactText = (
    <GlossaryText className="user-content" sourceId={sourceId} version={version}>
      <MatchedText {...matchedProps} highlight={highlight} />
    </GlossaryText>
  );
  if (highlight || !hasMarkup) return exactText;
  return (
    <>
      <button
        className="markdown-switch"
        type="button"
        title={source ? undefined : t(locale, 'markdown.sourceHelp')}
        onClick={() => setSource(!source)}
      >
        {t(locale, source ? 'markdown.preview' : 'markdown.source')}
      </button>
      {source ? (
        exactText
      ) : (
        <GlossaryText
          as="div"
          className="requirement-markdown"
          sourceId={sourceId}
          version={version}
        >
          <MatchedText {...matchedProps}>
            {(renderMatch) => formatted(ast, locale, projected, renderMatch)}
          </MatchedText>
        </GlossaryText>
      )}
    </>
  );
}
