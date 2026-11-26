import { execFileSync } from 'node:child_process';
import { constants, writeFileSync } from 'node:fs';
import { basename, join } from 'node:path';

const [wikiDirectory, wikiUrl] = process.argv.slice(2);
const english = 'Recent-Changes';
const russian = 'Recent-Changes-RU';
const indexPages = new Set(['Home.md', 'Home-en.md', `${english}.md`, `${russian}.md`]);
const git = (...args) => execFileSync('git', ['--literal-pathspecs', ...args], { cwd: wikiDirectory, encoding: 'utf8' });

const documents = new Map();
for (const path of git('ls-tree', '-r', '--name-only', '-z', 'HEAD').split('\0')) {
  if (!path.endsWith('.md') || indexPages.has(path) || basename(path).startsWith('_')) continue;
  const isRussian = path.endsWith('-RU.md');
  const key = path.slice(0, isRussian ? -6 : -3);
  const versions = documents.get(key) ?? {};
  versions[isRussian ? 'ru' : 'en'] = path;
  documents.set(key, versions);
}

const recent = [...documents.values()]
  .map((versions) => ({
    ...versions,
    timestamp: Number(git('log', '-1', '--first-parent', '--format=%ct', '--', ...Object.values(versions))),
  }))
  .sort((a, b) => b.timestamp - a.timestamp)
  .slice(0, 10);

function documentEntry(document, language) {
  const path = document[language] ?? document.en ?? document.ru;
  const slug = basename(path, '.md');
  const url = `${wikiUrl}/${encodeURIComponent(slug.replaceAll(' ', '-'))}`;
  const title = git('show', `HEAD:${path}`).match(/^# +(.+)$/m)?.[1].trim() || slug.replaceAll('-', ' ');
  const label = title.replace(/[\\`*_{}\[\]()<>#+\-.!|&~]/g, '\\$&');
  const date = new Date(document.timestamp * 1000).toISOString().slice(0, 10);
  return `- ${date} · [${label}](${url})`;
}

function writePage(name, language, heading, languageLink, introduction, historyLabel) {
  const page = [
    `# ${heading}`,
    '',
    languageLink,
    '',
    introduction,
    '',
    ...recent.map((document) => documentEntry(document, language)),
    '',
    `[${historyLabel}](${wikiUrl}/_history)`,
    '',
  ].join('\n');
  writeFileSync(join(wikiDirectory, `${name}.md`), page, {
    flag: constants.O_WRONLY | constants.O_CREAT | constants.O_TRUNC | constants.O_NOFOLLOW,
  });
}

writePage(english, 'en', 'Recent changes', `[Русская версия](${wikiUrl}/${russian})`,
  'Up to 10 recently updated Wiki documents. Each date reflects the latest edit to either language version.',
  'Full Wiki history');
writePage(russian, 'ru', 'Недавние изменения', `[English](${wikiUrl}/${english})`,
  'До 10 недавно обновлённых документов Wiki. Дата показывает последнюю правку в любой языковой версии.',
  'Полная история Wiki');
