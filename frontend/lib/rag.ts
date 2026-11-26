export function indexStatus(
  index: { version: number; status: string } | undefined,
  currentVersion: number,
): 'pending' | 'running' | 'ready' | 'failed' {
  if (!index || index.version !== currentVersion) return 'pending';
  return index.status === 'ready' || index.status === 'failed' || index.status === 'running'
    ? index.status
    : 'pending';
}
export function requirementAnchor(id: string) {
  return `#requirement-${encodeURIComponent(id)}`;
}

export function requirementURL(
  projectId: string,
  id: string,
  source?: { sourceVersion: number } & (
    | { start: number; end: number }
    | { start?: never; end?: never }
  ),
) {
  const version = source ? `&version=${source.sourceVersion}` : '';
  const range = source?.start !== undefined ? `&start=${source.start}&end=${source.end}` : '';
  return `/projects/${encodeURIComponent(projectId)}/requirements?requirement=${encodeURIComponent(id)}${version}${range}${requirementAnchor(id)}`;
}
