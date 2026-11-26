import { execFileSync } from 'node:child_process';
import { constants, writeFileSync } from 'node:fs';
import { join } from 'node:path';

const [wikiDirectory, wikiUrl] = process.argv.slice(2);
const english = 'Recent-Changes';
const russian = 'Recent-Changes-RU';
const history = execFileSync('git', [
  'log', '--first-parent', '-10', '-z', '--format=%H%x00%P%x00%cs%x00%s', '--',
  `:(exclude,top)${english}.md`, `:(exclude,top)${russian}.md`,
], { cwd: wikiDirectory, encoding: 'utf8' }).split('\0');

const entries = [];
for (let i = 0; i < history.length - 1; i += 4) {
  const [sha, parents, date, subject] = history.slice(i, i + 4);
  const parent = parents.split(' ', 1)[0];
  const revision = parent ? `_compare/${parent}...${sha}` : `Home/${sha}`;
  const label = (subject || sha.slice(0, 7)).replace(/[\\`*_{}\[\]()<>#+\-.!|&~]/g, '\\$&');
  entries.push(`- ${date} · [${label}](${wikiUrl}/${revision})`);
}

function writePage(name, heading, languageLink, introduction, historyLabel) {
  const page = [
    `# ${heading}`,
    '',
    languageLink,
    '',
    introduction,
    '',
    ...entries,
    '',
    `[${historyLabel}](${wikiUrl}/_history)`,
    '',
  ].join('\n');
  writeFileSync(join(wikiDirectory, `${name}.md`), page, {
    flag: constants.O_WRONLY | constants.O_CREAT | constants.O_TRUNC | constants.O_NOFOLLOW,
  });
}

writePage(english, 'Recent changes', `[Русская версия](${wikiUrl}/${russian})`,
  'Up to 10 recent Wiki changes, newest first.', 'Full Wiki history');
writePage(russian, 'Недавние изменения', `[English](${wikiUrl}/${english})`,
  'До 10 последних изменений Wiki, начиная с самого нового.', 'Полная история Wiki');
