// A small Markdown renderer for the built-in guide: headings, paragraphs,
// nested lists, code blocks, bold, inline code and links. The text is the
// project's own, but it is escaped all the same. Links to other files of
// the repository are not there in the program, so they stay plain text.

export interface Heading {
  id: string
  text: string
  level: number
}

const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')

export function slug(s: string): string {
  return s.toLowerCase().replace(/[^\p{L}\p{N}]+/gu, '-').replace(/^-+|-+$/g, '')
}

function inline(s: string): string {
  // Code spans first, so that what is inside them stays as written.
  return s
    .split(/(`[^`]+`)/)
    .map((p) => {
      if (p.length > 1 && p.startsWith('`') && p.endsWith('`')) return `<code>${esc(p.slice(1, -1))}</code>`
      return esc(p)
        .replace(/\*\*(.+?)\*\*/g, '<b>$1</b>')
        .replace(/\[([^\]]+)\]\(([^)\s]+)\)/g, (_, text: string, url: string) =>
          /^https?:/.test(url) ? `<a href="${url}" target="_blank" rel="noopener">${text}</a>` : text,
        )
    })
    .join('')
}

export function render(md: string): { html: string; headings: Heading[] } {
  const out: string[] = []
  const headings: Heading[] = []
  const lists: { indent: number; tag: 'ul' | 'ol' }[] = []
  let para: string[] = []
  let code: string[] | null = null

  const flush = () => {
    if (para.length) out.push(`<p>${inline(para.join(' '))}</p>`)
    para = []
  }
  const closeLists = (indent = -1) => {
    while (lists.length && lists[lists.length - 1].indent > indent) out.push(`</li></${lists.pop()!.tag}>`)
  }

  for (const line of md.replace(/\r/g, '').split('\n')) {
    if (code) {
      if (line.startsWith('```')) {
        out.push(`<pre><code>${esc(code.join('\n'))}</code></pre>`)
        code = null
      } else code.push(line)
      continue
    }
    if (line.startsWith('```')) {
      flush()
      closeLists()
      code = []
      continue
    }
    if (!line.trim()) {
      flush()
      continue
    }
    const h = line.match(/^(#{1,6})\s+(.*)$/)
    if (h) {
      flush()
      closeLists()
      const text = h[2].trim()
      const id = slug(text)
      headings.push({ id, text, level: h[1].length })
      out.push(`<h${h[1].length} id="${id}">${inline(text)}</h${h[1].length}>`)
      continue
    }
    const li = line.match(/^(\s*)([-*]|\d+\.)\s+(.*)$/)
    if (li) {
      flush()
      const indent = li[1].length
      const tag = /\d/.test(li[2]) ? 'ol' : 'ul'
      closeLists(indent)
      const top = lists[lists.length - 1]
      if (top && top.indent === indent && top.tag !== tag) closeLists(indent - 1)
      if (lists.length && lists[lists.length - 1].indent === indent) out.push('</li><li>')
      else {
        out.push(`<${tag}><li>`)
        lists.push({ indent, tag })
      }
      out.push(inline(li[3]))
      continue
    }
    if (lists.length && /^\s/.test(line)) {
      out.push(' ' + inline(line.trim())) // the item goes on
      continue
    }
    closeLists()
    para.push(line.trim())
  }
  flush()
  closeLists()
  if (code) out.push(`<pre><code>${esc((code as string[]).join('\n'))}</code></pre>`)
  return { html: out.join(''), headings }
}
