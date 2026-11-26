import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

const command = fileURLToPath(new URL('./wiki-recent-changes.mjs', import.meta.url));
const wikiUrl = 'https://github.com/owner/project/wiki';
const pages = ['Recent-Changes.md', 'Recent-Changes-RU.md'];

function createWiki(t) {
  const directory = mkdtempSync(join(tmpdir(), 'wiki-recent-changes-test-'));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  let day = 1;
  const git = (...args) => {
    const date = `2026-11-${String(day).padStart(2, '0')}T10:00:00Z`;
    return execFileSync('git', args, {
      cwd: directory,
      env: { ...process.env, GIT_AUTHOR_DATE: date, GIT_COMMITTER_DATE: date },
      encoding: 'utf8',
    }).trim();
  };
  git('init', '--quiet');
  git('config', 'user.name', 'Wiki test');
  git('config', 'user.email', 'wiki-test@example.invalid');
  git('config', 'commit.gpgsign', 'false');

  return {
    commit(files) {
      for (const [file, content] of Object.entries(files)) {
        const path = join(directory, file);
        if (content === null) {
          rmSync(path);
        } else {
          mkdirSync(dirname(path), { recursive: true });
          writeFileSync(path, content);
        }
      }
      git('add', '--all');
      git('commit', '--quiet', '-m', 'Edit Wiki pages');
      day += 1;
    },
    render() {
      execFileSync(process.execPath, [command, directory, wikiUrl]);
      return pages.map((file) => readFileSync(join(directory, file), 'utf8'));
    },
  };
}

function entries(page) {
  return page.split('\n').filter((line) => line.startsWith('- '));
}

test('links to one entry per document, orders by the latest language edit, and uses localized titles', (t) => {
  const wiki = createWiki(t);
  wiki.commit({ 'Home.md': '# Wiki Home', 'topics/Setup.md': '# Set up [CLI]', 'topics/Setup-RU.md': '# Установка CLI' });
  wiki.commit({ 'Other.md': '# Another document', 'One language-RU.md': '# Документ' });
  wiki.commit({ 'topics/Setup-RU.md': '# Установка CLI\n\nНовый раздел' });
  const [english, russian] = wiki.render();

  assert.ok(english.includes(`[Русская версия](${wikiUrl}/Recent-Changes-RU)`));
  assert.ok(russian.includes(`[English](${wikiUrl}/Recent-Changes)`));
  const shared = [
    `- 2026-11-02 · [Документ](${wikiUrl}/One-language-RU)`,
    `- 2026-11-02 · [Another document](${wikiUrl}/Other)`,
  ];
  assert.deepEqual(entries(english), [`- 2026-11-03 · [Set up \\[CLI\\]](${wikiUrl}/Setup)`, ...shared]);
  assert.deepEqual(entries(russian), [`- 2026-11-03 · [Установка CLI](${wikiUrl}/Setup-RU)`, ...shared]);
});

test('keeps the ten newest existing documents and skips navigation and generated pages', (t) => {
  const wiki = createWiki(t);
  for (let i = 0; i < 13; i += 1) {
    wiki.commit({ [`Page-${i}.md`]: `# Page ${i}` });
  }
  wiki.commit({ 'Home.md': '# Home', 'Home-en.md': '# Home EN', '_Sidebar.md': '# Sidebar', 'Page-12.md': null });
  wiki.commit({ 'Page-0.md': '# Page 0\n\nUpdated', [pages[0]]: 'Old feed' });
  const rendered = wiki.render();
  const expectedTitles = ['Page 0', ...Array.from({ length: 9 }, (_, i) => `Page ${11 - i}`)];
  for (const page of rendered) {
    assert.deepEqual(entries(page).map((line) => line.match(/\[(.*?)\]/)[1]), expectedTitles);
  }

  wiki.commit(Object.fromEntries(pages.map((file, index) => [file, rendered[index]])));
  assert.deepEqual(wiki.render(), rendered);
});
