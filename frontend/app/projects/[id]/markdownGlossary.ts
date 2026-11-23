import { type MarkdownToJSX, parser, RuleType } from 'markdown-to-jsx/react';
import type { GlossaryMatches } from './MatchedText';

type Node = MarkdownToJSX.ASTNode;
type Segment = string | { match: GlossaryMatches[number]; label: string };

function proseChildren(node: Node): Node[][] {
  switch (node.type) {
    case RuleType.paragraph:
    case RuleType.heading:
    case RuleType.blockQuote:
    case RuleType.textFormatted:
      return [node.children];
    case RuleType.orderedList:
    case RuleType.unorderedList:
      return node.items;
    case RuleType.table:
      return [...node.header, ...node.cells.flat()];
    default:
      return [];
  }
}

// The Markdown parser exposes no source offsets. Parse a marked copy to locate
// confirmed source ranges in prose, but always render the original tree (URLs,
// heading IDs, code, and Markdown syntax must retain their original meaning).
export function markdownGlossary(
  text: string,
  ast: Node[],
  matches: GlossaryMatches,
  options: Parameters<typeof parser>[1],
) {
  const projected = new Map<Node, Segment[]>();
  if (!matches.length) return projected;

  const chars = Array.from(text);
  let nonce = 0;
  // Alphanumeric delimiters also remain inside an autolink at the end of its URL.
  let marker = `pcglossary${nonce}x`;
  while (text.includes(marker)) marker = `pcglossary${++nonce}x`;
  const byTerms = new Map<string, number>();
  const chunks: string[] = [];
  let cursor = 0;
  for (const [index, match] of matches.entries()) {
    const label = chars.slice(match.start, match.end).join('');
    // Reference labels and definitions must still agree after case folding.
    const termsKey = JSON.stringify(match.terms.map((term) => term.id));
    const slot = byTerms.get(termsKey) ?? index;
    byTerms.set(termsKey, slot);
    chunks.push(
      chars.slice(cursor, match.start).join(''),
      marker,
      String(slot),
      'x',
      label,
      marker,
    );
    cursor = match.end;
  }
  chunks.push(chars.slice(cursor).join(''));
  const marked = parser(chunks.join(''), options);

  function pair(nodes: Node[], markedNodes: Node[]) {
    if (nodes.length !== markedNodes.length) return;
    for (const [i, node] of nodes.entries()) {
      const other = markedNodes[i];
      if (node.type !== other.type) continue;
      if (node.type === RuleType.text && other.type === RuleType.text) {
        if (!other.text.includes(marker) || node.text.includes(marker)) continue;
        const parts = other.text.split(marker);
        const segments: Segment[] = [];
        for (const [index, part] of parts.entries()) {
          if (index % 2 === 0) {
            segments.push(part);
          } else {
            const tagged = /^(\d+)x([\s\S]*)$/.exec(part);
            if (!tagged || !matches[Number(tagged[1])]) break;
            segments.push({ match: matches[Number(tagged[1])], label: tagged[2] });
          }
        }
        if (
          segments.length === parts.length &&
          segments.map((part) => (typeof part === 'string' ? part : part.label)).join('') ===
            node.text
        )
          projected.set(node, segments);
      } else {
        const groups = proseChildren(node);
        const markedGroups = proseChildren(other);
        if (groups.length === markedGroups.length)
          for (const [n, group] of groups.entries()) pair(group, markedGroups[n]);
      }
    }
  }
  pair(ast, marked);
  return projected;
}
