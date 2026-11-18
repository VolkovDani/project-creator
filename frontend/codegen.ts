import { readdir, readFile, writeFile } from 'node:fs/promises';
import { codegen } from '@graphql-codegen/core';
import * as typedDocumentNode from '@graphql-codegen/typed-document-node';
import * as typescriptOperations from '@graphql-codegen/typescript-operations';
import { buildASTSchema, lexicographicSortSchema, parse, Source } from 'graphql';

const filename = 'lib/generated.ts';

export function generate(schema: string, operations: { location: string; text: string }[]) {
  const document = parse(schema);
  return codegen({
    filename,
    schema: document,
    schemaAst: lexicographicSortSchema(buildASTSchema(document)),
    documents: operations.map(({ location, text }) => ({
      location,
      document: parse(new Source(text, location)),
    })),
    config: {},
    plugins: [{ typescriptOperations: {} }, { typedDocumentNode: {} }],
    pluginMap: { typescriptOperations, typedDocumentNode },
  });
}

if (import.meta.main) {
  const paths = (await readdir('operations')).filter((name) => name.endsWith('.graphql')).sort();
  if (!paths.length) throw new Error('No GraphQL operations found');
  const operations = await Promise.all(
    paths.map(async (name) => {
      const location = `operations/${name}`;
      return { location, text: await readFile(location, 'utf8') };
    }),
  );
  const output = await generate(await readFile('../graph/schema.graphqls', 'utf8'), operations);
  await writeFile(filename, output);
}
