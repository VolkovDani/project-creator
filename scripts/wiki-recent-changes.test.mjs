import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

const command = fileURLToPath(new URL('./wiki-recent-changes.mjs', import.meta.url));
const wikiUrl = 'https://github.com/owner/project/wiki';
const pages = ['Recent-Changes.md', 'Recent-Changes-RU.md'];

function createWiki(t) {
  const directory = mkdtempSync(join(tmpdir(), 'wiki-recent-changes-test-'));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  const date = '2026-11-26T10:00:00Z';
  const git = (...args) => execFileSync('git', args, {
    cwd: directory,
    env: { ...process.env, GIT_AUTHOR_DATE: date, GIT_COMMITTER_DATE: date },
    encoding: 'utf8',
  }).trim();
  git('init', '--quiet');
  git('config', 'user.name', 'Wiki test');
  git('config', 'user.email', 'wiki-test@example.invalid');
  git('config', 'commit.gpgsign', 'false');

  return {
    commit(subject, files) {
      for (const [file, content] of Object.entries(files)) writeFileSync(join(directory, file), content);
      git('add', '--all');
      git('commit', '--quiet', '-m', subject);
      return git('rev-parse', 'HEAD');
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

test('shows the available Wiki revisions on both linked language pages', (t) => {
  const wiki = createWiki(t);
  const first = wiki.commit('Initial Home', { 'Home.md': 'Home' });
  const second = wiki.commit('Clarify [setup] *steps*', { 'Home.md': 'Updated home' });
  const [english, russian] = wiki.render();

  assert.ok(english.includes(`[Русская версия](${wikiUrl}/Recent-Changes-RU)`));
  assert.ok(russian.includes(`[English](${wikiUrl}/Recent-Changes)`));
  assert.deepEqual(entries(english), [
    `- 2026-11-26 · [Clarify \\[setup\\] \\*steps\\*](${wikiUrl}/_compare/${first}...${second})`,
    `- 2026-11-26 · [Initial Home](${wikiUrl}/Home/${first})`,
  ]);
  assert.deepEqual(entries(russian), entries(english));
});

test('keeps the ten newest content edits across repeated refreshes', (t) => {
  const wiki = createWiki(t);
  for (let i = 0; i < 12; i += 1) {
    wiki.commit(`Edit ${i}`, { 'Home.md': `Home ${i}` });
  }
  wiki.commit('Generated pages only', Object.fromEntries(pages.map((file) => [file, 'Old feed'])));
  wiki.commit('Content and feed updated together', { 'Home.md': 'Real edit', [pages[0]]: 'Old English feed' });
  const rendered = wiki.render();
  const expectedSubjects = ['Content and feed updated together', ...Array.from({ length: 9 }, (_, i) => `Edit ${11 - i}`)];
  for (const page of rendered) {
    assert.deepEqual(entries(page).map((line) => line.match(/\[(.*?)\]/)[1]), expectedSubjects);
  }

  wiki.commit('Refresh generated recent changes', Object.fromEntries(pages.map((file, index) => [file, rendered[index]])));
  assert.deepEqual(wiki.render(), rendered);
});
