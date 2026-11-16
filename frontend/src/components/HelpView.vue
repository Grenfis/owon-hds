<script setup lang="ts">
// The guide to the modes, docs/modes.md, built into the program. It opens
// over the window, so the device keeps being polled behind it.
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import guide from '../../../docs/modes.md?raw'
import { render } from '../md'

const emit = defineEmits<{ close: [] }>()

const { html, headings } = render(guide)
const toc = headings.filter((h) => h.level === 2)
const body = ref<HTMLElement>()
const active = ref(toc[0]?.id ?? '')

function go(id: string) {
  const el = body.value?.querySelector<HTMLElement>(`#${CSS.escape(id)}`)
  if (el) body.value!.scrollTo({ top: el.offsetTop - 12, behavior: 'smooth' })
}

// The section at the top of the view is the one marked in the contents.
function scrolled() {
  const b = body.value
  if (!b) return
  lastScroll = b.scrollTop
  let id = toc[0]?.id ?? ''
  for (const h of toc) {
    const el = b.querySelector<HTMLElement>(`#${CSS.escape(h.id)}`)
    if (el && el.offsetTop - 40 <= b.scrollTop) id = h.id
  }
  active.value = id
}

function key(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close')
}

onMounted(async () => {
  window.addEventListener('keydown', key)
  await nextTick()
  if (body.value) body.value.scrollTop = lastScroll
  scrolled()
})
onBeforeUnmount(() => window.removeEventListener('keydown', key))
</script>

<script lang="ts">
// Where the reader was, kept while the program runs.
let lastScroll = 0
</script>

<template>
  <div class="backdrop" @mousedown.self="emit('close')">
    <div class="help">
      <nav class="toc">
        <div class="head">Справка</div>
        <button v-for="h in toc" :key="h.id" class="item" :class="{ on: active === h.id }" @click="go(h.id)">{{ h.text }}</button>
      </nav>
      <article ref="body" class="doc" @scroll="scrolled" v-html="html" />
      <button class="ghost close" title="Закрыть (Esc)" @click="emit('close')">✕</button>
    </div>
  </div>
</template>

<style scoped>
.backdrop {
  position: fixed;
  inset: 0;
  z-index: 50;
  background: rgba(5, 6, 9, .6);
  display: grid;
  place-items: center;
}
.help {
  position: relative;
  width: min(1100px, 92vw);
  height: 88vh;
  display: flex;
  background: var(--panel);
  border: 1px solid var(--line-2);
  border-radius: 12px;
  overflow: hidden;
  box-shadow: 0 20px 60px rgba(0, 0, 0, .5);
}
.toc {
  width: 190px;
  flex: none;
  padding: 14px 8px;
  border-right: 1px solid var(--line);
  background: var(--bg);
  display: flex;
  flex-direction: column;
  gap: 2px;
  overflow-y: auto;
}
.head { font-size: 12px; font-weight: 600; color: var(--muted); text-transform: uppercase; letter-spacing: .06em; padding: 0 8px 8px; }
.item { text-align: left; border: 0; background: transparent; color: var(--muted); padding: 5px 8px; border-radius: 6px; font-size: 13px; }
.item:hover { color: var(--text); background: var(--panel-2); }
.item.on { color: var(--text); background: var(--accent-bg); }
.close { position: absolute; top: 10px; right: 12px; }
.doc { flex: 1; overflow-y: auto; padding: 18px 36px 40px 28px; font-size: 14px; line-height: 1.6; user-select: text; }
.doc :deep(> *) { max-width: 760px; }
.doc :deep(h1) { font-size: 22px; margin: 0 0 10px; }
.doc :deep(h2) { font-size: 17px; margin: 28px 0 8px; padding-top: 14px; border-top: 1px solid var(--line); }
.doc :deep(h3) { font-size: 15px; margin: 18px 0 6px; }
.doc :deep(p) { margin: 6px 0; }
.doc :deep(ul), .doc :deep(ol) { margin: 4px 0 8px; padding-left: 22px; }
.doc :deep(li) { margin: 2px 0; }
.doc :deep(li > ul), .doc :deep(li > ol) { margin: 2px 0; }
.doc :deep(b) { color: var(--text); font-weight: 600; }
.doc :deep(code) { font-family: var(--mono); font-size: 12.5px; background: var(--panel-3); padding: 1px 5px; border-radius: 4px; }
.doc :deep(pre) { background: var(--bg); border: 1px solid var(--line); border-radius: 6px; padding: 10px 12px; overflow-x: auto; }
.doc :deep(pre code) { background: none; padding: 0; }
.doc :deep(a) { color: var(--accent); }
</style>
