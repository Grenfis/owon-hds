// Run with: npm test
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { render } from '../src/md.ts'

test('headings, paragraphs and inline marks', () => {
  const { html, headings } = render('# Режимы пульта\n\n**Что это.** Текст `a**b**<c>`\nи дальше.\n\n## Осциллограф\n')
  assert.equal(
    html,
    '<h1 id="режимы-пульта">Режимы пульта</h1><p><b>Что это.</b> Текст <code>a**b**&lt;c&gt;</code> и дальше.</p><h2 id="осциллограф">Осциллограф</h2>',
  )
  assert.deepEqual(headings.map((h) => [h.id, h.level]), [['режимы-пульта', 1], ['осциллограф', 2]])
})

test('nested and ordered lists', () => {
  const { html } = render('- один\n  - вложенный\n  - ещё\n- два\n\n1. первый\n2. второй\n\nАбзац.')
  assert.equal(
    html,
    '<ul><li>один<ul><li>вложенный</li><li>ещё</li></ul></li><li>два</li></ul>' +
      '<ol><li>первый</li><li>второй</li></ol><p>Абзац.</p>',
  )
})

test('links: the web opens, files of the repository stay text', () => {
  const { html } = render('См. [README](../README.md) и [сайт](https://example.com/?a=1&b=2).')
  assert.equal(html, '<p>См. README и <a href="https://example.com/?a=1&amp;b=2" target="_blank" rel="noopener">сайт</a>.</p>')
})

test('code blocks stay as written', () => {
  assert.equal(render('```sh\na < b\n```').html, '<pre><code>a &lt; b</code></pre>')
})

test('the guide renders with every list closed', () => {
  const { html, headings } = render(readFileSync(new URL('../../docs/modes.md', import.meta.url), 'utf8'))
  for (const tag of ['ul', 'ol', 'li', 'p']) {
    assert.equal(html.split(`<${tag}>`).length, html.split(`</${tag}>`).length, tag)
  }
  assert.ok(headings.filter((h) => h.level === 2).length >= 9)
})
