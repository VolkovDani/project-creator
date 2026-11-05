import { spawnSync } from 'node:child_process';
import { accessSync, constants, existsSync } from 'node:fs';
import { join, resolve } from 'node:path';

function git(args, optional = false) {
  const result = spawnSync('git', args, { encoding: 'utf8' });
  if (!result.error && (result.status === 0 || (optional && result.status === 1)))
    return result.stdout.trimEnd();
  throw Error(result.error?.message ?? result.stderr);
}
try {
  const root = git(['rev-parse', '--show-toplevel']);
  process.chdir(root);
  const current = git(['config', '--get', 'core.hooksPath'], true);
  if (current && current !== '.githooks')
    throw Error(`Existing core.hooksPath=${JSON.stringify(current)} was preserved. Integrate .githooks/pre-commit into your hook setup explicitly.`);
  if (!current && existsSync(resolve(git(['rev-parse', '--git-path', 'hooks/pre-commit']))))
    throw Error('Existing .git/hooks/pre-commit was preserved. Integrate .githooks/pre-commit into that hook explicitly.');
  accessSync(join(root, '.githooks/pre-commit'), constants.X_OK);
  git(['config', '--local', 'core.hooksPath', '.githooks']);
  console.log('Git hooks enabled for this clone: core.hooksPath=.githooks');
} catch (error) {
  console.error(`hooks:install: ${error.message}`);
  process.exitCode = 1;
}
