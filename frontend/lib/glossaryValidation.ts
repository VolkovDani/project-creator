import type limits from '../../contracts/glossary.json';

type Term = {
  name: string;
  context: string;
  definition: string;
  variants: Array<{ kind: string; text: string }>;
};
type Issue = { code: string; key: string; params?: Record<string, number> };
export function validateGlossary(c: typeof limits, v: Term): Issue | undefined {
  function field(s: string, max: number, required: boolean): Issue | undefined {
    const value = s.trim();
    if (!value.isWellFormed() || value.includes('\0'))
      return { code: 'INVALID_UTF8', key: 'validation.invalidEncoding' };
    if (required && !value) return { code: 'REQUIRED', key: 'validation.required' };
    if (Array.from(value).length > max)
      return { code: 'MAX_LENGTH', key: 'validation.maxLength', params: { max } };
  }
  const invalid =
    field(v.name, c.nameMax, true) ??
    field(v.definition, c.definitionMax, false) ??
    field(v.context, c.contextMax, false);
  if (invalid) return invalid;
  if (v.variants.length > c.variantLimit) return { code: 'LIMIT', key: 'glossary.invalidVariants' };
  const seen = new Set([v.name.trim().toLowerCase()]);
  for (const variant of v.variants) {
    const e = field(variant.text, c.variantMax, true);
    if (e) return e;
    const key = variant.text.trim().toLowerCase();
    if (seen.has(key) || !c.kinds.includes(variant.kind))
      return { code: 'INVALID_VARIANT', key: 'glossary.invalidVariants' };
    seen.add(key);
  }
}
