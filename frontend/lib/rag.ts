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

export function requirementURL(projectId: string, id: string) {
  return `/projects/${encodeURIComponent(projectId)}/requirements?requirement=${encodeURIComponent(id)}${requirementAnchor(id)}`;
}
