import { spawnSync } from 'node:child_process';
import { existsSync, lstatSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';

const maxBuffer = 32 * 1024 * 1024;
function git(args, input) {
  const result = spawnSync('git', args, { input, maxBuffer });
  if (result.error || result.status !== 0)
    throw Error(result.error?.message ?? result.stderr.toString());
  return result.stdout;
}
function regularContent(path) {
  try {
    return lstatSync(path).isFile() ? readFileSync(path) : null;
  } catch (error) {
    if (error.code === 'ENOENT') return null;
    throw error;
  }
}
function indexEntry(path) {
  const entry = git(['ls-files', '--stage', '-z', '--', path]).toString();
  const [mode, hash, stage] = entry.slice(0, entry.indexOf('\t')).split(' ');
  if (stage !== '0') throw Error(`Unmerged or missing index entry: ${JSON.stringify(path)}`);
  return { mode, content: git(['cat-file', 'blob', hash]) };
}

let snapshot;
try {
  const root = git(['rev-parse', '--show-toplevel']).toString().trimEnd();
  process.chdir(root);
  const paths = git(['diff', '--cached', '--name-only', '--diff-filter=ACMR', '-z'])
    .toString().split('\0').filter((path) => path.startsWith('frontend/') && /\.(?:ts|tsx|json|jsonc)$/.test(path));
  if (paths.length) {
    const biome = join(root, 'frontend', 'node_modules', '@biomejs', 'biome', 'bin', 'biome');
    if (!existsSync(biome)) throw Error('Install the pinned Biome first: cd frontend && bun install --frozen-lockfile');
    snapshot = mkdtempSync(join(tmpdir(), 'pc-biome-index-'));
    const copy = (path, content) => {
      const target = join(snapshot, path);
      mkdirSync(dirname(target), { recursive: true });
      writeFileSync(target, content);
    };
    // Read configuration and ignore files from the index too: unstaged settings
    // must not change which rules are applied to the proposed commit.
    const configs = git(['ls-files', '-z', '--', '.gitignore', ':(glob)frontend/**/.gitignore', ':(glob)frontend/**/biome.json', ':(glob)frontend/**/biome.jsonc'])
      .toString().split('\0').filter(Boolean);
    for (const path of configs) {
      const entry = indexEntry(path);
      if (entry.mode !== '100644' && entry.mode !== '100755')
        throw Error(`Biome configuration must be a regular file: ${JSON.stringify(path)}`);
      copy(path, entry.content);
    }
    const files = [];
    for (const path of paths) {
      const { mode, content: original } = indexEntry(path);
      if (mode !== '100644' && mode !== '100755')
        throw Error(`Biome requires a regular file, not a symlink: ${JSON.stringify(path)}`);
      const partial = !regularContent(path)?.equals(original);
      files.push({ path, mode, original, partial });
      copy(path, original);
    }
    const changes = [];
    if (files.length) {
      const result = spawnSync(process.execPath, [biome, 'check', '--write', '--error-on-warnings', '--no-errors-on-unmatched', ...files.map(({ path }) => `./${path.slice('frontend/'.length)}`)], {
        cwd: join(snapshot, 'frontend'), maxBuffer,
      });
      if (result.error || result.status !== 0)
        throw Error(result.error?.message ?? `${result.stdout.toString()}${result.stderr.toString()}`);
      for (const file of files) {
        const formatted = readFileSync(join(snapshot, file.path));
        if (formatted.equals(file.original)) continue;
        if (file.partial)
          throw Error(`Partially staged file needs Biome fixes: ${JSON.stringify(file.path)}. Format and stage the intended changes; index and working files were preserved.`);
        changes.push({ ...file, formatted });
      }
    }
    // Recheck before writing: never include working changes made while Biome ran.
    for (const change of changes)
      if (!regularContent(change.path)?.equals(change.original))
        throw Error(`File changed during pre-commit: ${JSON.stringify(change.path)}. Retry the commit.`);
    const entries = [];
    for (const change of changes) {
      const hash = git(['hash-object', '-w', '--stdin'], change.formatted).toString().trim();
      writeFileSync(change.path, change.formatted);
      entries.push(`${change.mode} ${hash}\t${change.path}\0`);
    }
    if (entries.length) {
      git(['update-index', '-z', '--index-info'], entries.join(''));
      console.log(`Biome formatted and staged ${entries.length} file(s).`);
    }
  }
} catch (error) {
  console.error(`pre-commit: ${error.message}`);
  process.exitCode = 1;
} finally {
  if (snapshot) rmSync(snapshot, { recursive: true, force: true });
}
