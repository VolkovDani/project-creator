import type { TypedDocumentNode } from '@graphql-typed-document-node/core';
import { print } from 'graphql';
import { TaggedCache } from './cache.ts';
export const clientCache = new TaggedCache();
let unauthorized = false;
let sessionEpoch = 0;
export function clientSessionVersion() {
  return sessionEpoch;
}
export function clearClientSession() {
  sessionEpoch++;
  unauthorized = false;
  clientCache.reset();
}
export function rejectClientSession() {
  if (!unauthorized) {
    unauthorized = true;
    sessionEpoch++;
    clientCache.reset();
  }
}
function ensureSession(epoch: number) {
  if (epoch !== sessionEpoch) throw Error('session changed');
}
function fallbackTags(variables: unknown) {
  const v = variables as { id?: string; projectId?: string; input?: { projectId?: string } };
  const id = v.projectId || v.input?.projectId || v.id;
  return ['projects', ...(id ? [`project:${id}`, `project:${id}:requirements`] : [])];
}
export async function query<T, V>(
  document: TypedDocumentNode<T, V>,
  variables: V,
  options?: { tags?: string[]; version?: string; notify?: boolean },
): Promise<T> {
  const epoch = sessionEpoch;
  const source = print(document);
  const operation = document.definitions.find((def) => def.kind === 'OperationDefinition');
  const mutation = operation?.kind === 'OperationDefinition' && operation.operation === 'mutation';
  const load = async () => {
    const response = await fetch('/api/graphql', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({
        query: source,
        variables,
        operationName:
          operation?.kind === 'OperationDefinition' ? operation.name?.value : undefined,
      }),
      cache: 'no-store',
    });
    ensureSession(epoch);
    if (response.status === 401 || response.status === 403) {
      rejectClientSession();
      throw Error('unauthorized');
    }
    if (!response.ok) throw Error('request failed');
    unauthorized = false;
    const payload = await response.json();
    ensureSession(epoch);
    if (payload.errors?.length || !payload.data) throw Error('graphql error');
    const tags = Array.isArray(payload.extensions?.cacheTags)
      ? payload.extensions.cacheTags.filter((tag: unknown) => typeof tag === 'string')
      : fallbackTags(variables);
    return { data: payload.data as T, tags: options?.tags ?? tags };
  };
  if (mutation) {
    const result = await load();
    ensureSession(epoch);
    if (options?.notify === false) clientCache.evict(result.tags);
    else clientCache.invalidate(result.tags);
    return result.data;
  }
  const result = await clientCache.read(
    JSON.stringify([source, variables, options?.version]),
    load,
  );
  ensureSession(epoch);
  return result;
}
