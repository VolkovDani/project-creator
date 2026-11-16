import type { CodegenConfig } from '@graphql-codegen/cli';

const config: CodegenConfig = {
  schema: '../graph/schema.graphqls',
  documents: 'operations/*.graphql',
  generates: {
    'lib/generated.ts': { plugins: ['typescript-operations', 'typed-document-node'] },
  },
};
export default config;
