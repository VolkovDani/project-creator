import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';

// GitHub's needs gate is repeated here so a manual job rerun fails closed too.
export async function autoMerge(context, request) {
  const skip = (reason) => ({ merged: false, reason });
  if (context.event !== 'pull_request' ||
      context.needs?.checks?.result !== 'success' ||
      context.needs?.containers?.result !== 'success') {
    return skip('Both CI jobs must succeed in a pull_request run.');
  }
  const { repository, number, head, merge } = context;
  if (!/^[\w.-]+\/[\w.-]+$/.test(repository) || !Number.isSafeInteger(number) || number < 1 ||
      !/^[a-f0-9]{40}$/.test(head) || !/^[a-f0-9]{40}$/.test(merge)) {
    throw new Error('Invalid CI merge context.');
  }
  const root = `repos/${repository}`;
  const pr = await request(`${root}/pulls/${number}`);
  if (pr.state !== 'open' || pr.draft !== false ||
      pr.head?.repo?.full_name !== repository || pr.base?.repo?.full_name !== repository ||
      pr.base?.ref !== 'develop' || pr.head?.sha !== head || pr.mergeable !== true ||
      !pr.labels?.some((label) => label.name === 'automerge')) {
    return skip('PR must be open, ready, mergeable, labeled automerge, and still at the tested head.');
  }
  // PR jobs test the synthetic merge commit. Do not merge after develop has moved.
  const tested = await request(`${root}/git/commits/${merge}`);
  const currentBase = await request(`${root}/git/ref/heads/develop`);
  if (tested.parents?.length !== 2 || tested.parents[1].sha !== head ||
      tested.parents[0].sha !== currentBase.object?.sha) {
    return skip('The tested merge is stale; update the branch and run CI again.');
  }
  // The API rejects a head change racing this request; never retry another SHA.
  const result = await request(`${root}/pulls/${number}/merge`, { sha: head, merge_method: 'merge' });
  if (result.merged !== true) throw new Error('GitHub did not confirm the merge.');
  return { merged: true, sha: result.sha };
}

function githubRequest(path, body) {
  const args = ['api', '--method', body ? 'PUT' : 'GET', path,
    '-H', 'Accept: application/vnd.github+json', '-H', 'X-GitHub-Api-Version: 2022-11-28'];
  if (body) args.push('--input', '-');
  return JSON.parse(execFileSync('gh', args, {
    encoding: 'utf8', input: body ? JSON.stringify(body) : undefined,
    timeout: 30_000, stdio: ['pipe', 'pipe', 'pipe'],
  }));
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    const event = JSON.parse(readFileSync(process.env.GITHUB_EVENT_PATH, 'utf8'));
    const result = await autoMerge({
      repository: process.env.GITHUB_REPOSITORY,
      number: event.pull_request?.number,
      head: event.pull_request?.head?.sha,
      merge: process.env.GITHUB_SHA,
      event: process.env.GITHUB_EVENT_NAME,
      needs: JSON.parse(process.env.NEEDS_JSON),
    }, githubRequest);
    console.log(result.merged ? `Merged: ${result.sha}` : `Auto-merge skipped: ${result.reason}`);
  } catch (error) {
    console.error(`Auto-merge failed: ${error.message}`);
    process.exitCode = 1;
  }
}
