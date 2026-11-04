export type ListMode = 'infinite' | 'pages';

export function readListLocation(search: string): { mode: ListMode; page: number } {
  const params = new URLSearchParams(search);
  const mode = params.get('listMode') === 'pages' ? 'pages' : 'infinite';
  const requested = Number(params.get('page') ?? 1);
  const page =
    Number.isSafeInteger(requested) && requested > 0 && requested <= 100000 ? requested - 1 : 0;
  return { mode, page: mode === 'pages' ? page : 0 };
}

export function listLocation(location: string, mode: ListMode, page: number): string {
  const url = new URL(location, 'http://local');
  url.searchParams.delete('listMode');
  url.searchParams.delete('page');
  if (mode === 'pages') {
    url.searchParams.set('listMode', mode);
    url.searchParams.set('page', String(page + 1));
  }
  return url.pathname + url.search + url.hash;
}
