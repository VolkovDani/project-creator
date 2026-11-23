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

// Tagging a URL scheme, an entity, or a block prefix can change the parse of
// surrounding prose. Keep these delimiters literal in the auxiliary input.
function markdownDelimiters(text: string) {
  const protectedChars = new Uint8Array(text.length);
  const delimiters =
    /\b(?:https?:\/\/|mailto:|www\.(?=\S))|&(?:[a-z][a-z\d]*|#(?:\d+|x[\da-f]+));|^[ \t>]*(?:\d{1,9}[.)]|#{1,6}|[*+-])(?=\s|$)/gim;
  for (const match of text.matchAll(delimiters))
    protectedChars.fill(1, match.index, match.index + match[0].length);
  return protectedChars;
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
  const delimiters = markdownDelimiters(text);
  const byTerms = new Map<string, number>();
  const chunks: string[] = [];
  let cursor = 0;
  let utf16Offset = 0;
  for (const [index, match] of matches.entries()) {
    const prefix = chars.slice(cursor, match.start).join('');
    const label = chars.slice(match.start, match.end).join('');
    chunks.push(prefix);
    utf16Offset += prefix.length;
    const inSyntax = delimiters.subarray(utf16Offset, utf16Offset + label.length).includes(1);
    utf16Offset += label.length;
    cursor = match.end;
    if (inSyntax) {
      chunks.push(label);
      continue;
    }
    // Reference labels and definitions must still agree after case folding.
    const termsKey = JSON.stringify(match.terms.map((term) => term.id));
    const slot = byTerms.get(termsKey) ?? index;
    byTerms.set(termsKey, slot);
    chunks.push(marker, String(slot), 'x', label, marker);
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
