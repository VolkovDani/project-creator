const prereleaseTag = /^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?$/;
const fullSha = /^[a-f0-9]{40}$/;

export function validateReleaseInput({ tag, sourceSha, notesSha, ref }) {
  if (ref !== 'refs/heads/main') throw new Error('Release workflow must run from main.');
  if (!prereleaseTag.test(tag ?? '')) throw new Error('A semantic version prerelease tag is required.');
  if (!fullSha.test(sourceSha ?? '')) throw new Error('A full 40-character source SHA is required.');
  if (!fullSha.test(notesSha ?? '')) throw new Error('A full 40-character Wiki notes SHA is required.');
  return { tag, sourceSha, notesSha };
}

export function validateAcceptanceIssue(issue, tag, sourceSha) {
  if (!issue || issue.pull_request || issue.state !== 'closed' ||
      !issue.labels?.some(({ name }) => name === 'hardware-accepted')) {
    throw new Error('Acceptance issue must be closed and labeled hardware-accepted.');
  }
  const tagRecorded = issue.body?.split('\n').some((line) => line.trim() === `release-tag: ${tag}`);
  const shaRecorded = issue.body?.split('\n').some((line) => line.trim() === `source-sha: ${sourceSha}`);
  if (!tagRecorded || !shaRecorded) {
    throw new Error('Acceptance issue must record exact release-tag and source-sha fields.');
  }
}

export function validateSuccessfulMainCi(runs, sourceSha) {
  if (!runs.some(({ headSha, status, conclusion, event, headBranch }) =>
    headSha === sourceSha && status === 'completed' && conclusion === 'success' &&
    ['push', 'workflow_dispatch'].includes(event) && headBranch === 'main')) {
    throw new Error('A successful main CI run for the exact source SHA is required.');
  }
}
