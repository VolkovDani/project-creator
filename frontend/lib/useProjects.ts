import { query } from './client';
import { ProjectConnectionDocument } from './generated';
import { useInfinite } from './useInfinite';

async function loadProjects(after: string | null) {
  return (await query(ProjectConnectionDocument, { first: 40, after })).projectsConnection;
}

export function useProjects() {
  return useInfinite({ identity: 'projects', tags: ['projects'], load: loadProjects });
}
