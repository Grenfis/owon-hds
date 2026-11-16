<script setup lang="ts">
// The oscilloscope screen: 12 × 8 divisions, 25 sample counts per division,
// as on the device. Channel ground markers on the left and the trigger level
// marker on the right can be dragged.
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { eng } from '../format'
import type { Frame } from '../types'

const props = defineProps<{ frame: Frame | null; scope: Record<string, string> }>()
const emit = defineEmits<{ offset: [ch: 'ch1' | 'ch2', div: number]; level: [volts: number] }>()

const DIVX = 12, DIVY = 8, PX = 25
const M = { l: 28, r: 28, t: 16, b: 8 }

const box = ref<HTMLDivElement>()
const canvas = ref<HTMLCanvasElement>()
const hover = ref<{ x: number; y: number } | null>(null)
const drag = ref<{ kind: 'ch1' | 'ch2' | 'trig'; div: number } | null>(null)
let colors: Record<string, string> = {}
let size = { w: 0, h: 0, dpr: 1 }
let raf = 0

function plotRect() {
  const aw = size.w - M.l - M.r, ah = size.h - M.t - M.b
  const pw = Math.max(10, Math.min(aw, ah * DIVX / DIVY))
  const ph = pw * DIVY / DIVX
  const x = M.l + (aw - pw) / 2, y = M.t + (ah - ph) / 2
  return { x, y, w: pw, h: ph, div: pw / DIVX, cx: x + pw / 2, cy: y + ph / 2 }
}

function chInfo(name: 'ch1' | 'ch2') {
  const ch = props.frame?.channels.find((c) => c.name.toLowerCase() === name)
  const on = props.scope[`${name}.display`] === 'ON'
  const offsetDiv = drag.value?.kind === name ? drag.value.div : parseFloat(props.scope[`${name}.offset`]) || 0
  return { ch, on, offsetDiv, vdiv: ch?.voltsPerDiv ?? 0 }
}

// Trigger level in divisions from the screen centre.
function trigDiv(): number | null {
  if (drag.value?.kind === 'trig') return drag.value.div
  const src = (props.scope['trig.source'] || 'CH1').toLowerCase() as 'ch1' | 'ch2'
  const { vdiv, offsetDiv } = chInfo(src)
  if (!vdiv) return null
  return parseFloat(props.scope['trig.level']) / vdiv + offsetDiv
}

function schedule() {
  if (!raf) raf = requestAnimationFrame(() => { raf = 0; draw() })
}

function draw() {
  const c = canvas.value
  if (!c) return
  const g = c.getContext('2d')!
  const { dpr } = size
  g.setTransform(dpr, 0, 0, dpr, 0, 0)
  g.clearRect(0, 0, size.w, size.h)
  const r = plotRect()

  g.fillStyle = '#090b0f'
  g.fillRect(r.x, r.y, r.w, r.h)

  // Grid: dotted minor lines, solid centre axes with fifth-division ticks.
  g.lineWidth = 1
  g.strokeStyle = '#20252f'
  g.setLineDash([1, 3])
  g.beginPath()
  for (let i = 1; i < DIVX; i++) if (i !== DIVX / 2) line(g, r.x + i * r.div, r.y, r.x + i * r.div, r.y + r.h)
  for (let i = 1; i < DIVY; i++) if (i !== DIVY / 2) line(g, r.x, r.y + i * r.div, r.x + r.w, r.y + i * r.div)
  g.stroke()
  g.setLineDash([])
  g.strokeStyle = '#363d4b'
  g.beginPath()
  line(g, r.cx, r.y, r.cx, r.y + r.h)
  line(g, r.x, r.cy, r.x + r.w, r.cy)
  for (let i = 0; i <= DIVX * 5; i++) line(g, r.x + i * r.div / 5, r.cy - 3, r.x + i * r.div / 5, r.cy + 3)
  for (let i = 0; i <= DIVY * 5; i++) line(g, r.cx - 3, r.y + i * r.div / 5, r.cx + 3, r.y + i * r.div / 5)
  g.stroke()
  g.strokeStyle = '#2b313d'
  g.strokeRect(r.x + 0.5, r.y + 0.5, r.w - 1, r.h - 1)

  // Traces.
  g.save()
  g.beginPath()
  g.rect(r.x, r.y, r.w, r.h)
  g.clip()
  for (const name of ['ch2', 'ch1'] as const) {
    const { ch, on } = chInfo(name)
    const raw = ch?.raw
    if (!on || !raw || raw.length < 2) continue
    g.strokeStyle = colors[name]
    g.lineWidth = 1.6
    g.lineJoin = 'round'
    g.beginPath()
    const k = r.w / (raw.length - 1)
    for (let i = 0; i < raw.length; i++) {
      const x = r.x + i * k, y = r.cy - (raw[i] / PX) * r.div
      i ? g.lineTo(x, y) : g.moveTo(x, y)
    }
    g.stroke()
  }
  g.restore()

  // Ground markers of the channels, CH1 on top as with the traces.
  for (const t of groundTags(r).reverse()) marker(g, t.x, t.y, r, 'left', colors[t.name], t.name.slice(2))
  // Trigger level and trigger position.
  const td = trigDiv()
  const src = (props.scope['trig.source'] || 'CH1').toLowerCase()
  if (td != null) marker(g, r.x + r.w + 4, r.cy - td * r.div, r, 'right', colors[src] ?? colors.ch1, 'T')
  const hoff = parseFloat(props.scope['hor.offset']) || 0
  const tx = Math.max(r.x, Math.min(r.x + r.w, r.cx + hoff * r.div))
  g.fillStyle = colors.accent
  g.beginPath()
  g.moveTo(tx - 6, r.y - 11)
  g.lineTo(tx + 6, r.y - 11)
  g.lineTo(tx, r.y - 3)
  g.fill()

  if (hover.value && !drag.value) drawHover(g, r, hover.value.x)
}

function line(g: CanvasRenderingContext2D, x1: number, y1: number, x2: number, y2: number) {
  g.moveTo(Math.round(x1) + 0.5, Math.round(y1) + 0.5)
  g.lineTo(Math.round(x2) + 0.5, Math.round(y2) + 0.5)
}

// groundTags places the channel ground markers. When both sit at the same
// height, the CH2 tag moves left so that both stay visible and grabbable.
function groundTags(r: ReturnType<typeof plotRect>) {
  const tags: { name: 'ch1' | 'ch2'; x: number; y: number }[] = []
  for (const name of ['ch1', 'ch2'] as const) {
    const { on, offsetDiv } = chInfo(name)
    if (on) tags.push({ name, x: r.x - 4, y: r.cy - offsetDiv * r.div })
  }
  const clamp = (y: number) => Math.max(r.y, Math.min(r.y + r.h, y))
  if (tags.length === 2 && Math.abs(clamp(tags[0].y) - clamp(tags[1].y)) < 16 && r.x >= 50) tags[1].x -= 24
  return tags
}

// marker draws a tag pointing at the screen edge; off-screen positions show
// an arrow at the edge instead.
function marker(g: CanvasRenderingContext2D, x: number, y: number, r: ReturnType<typeof plotRect>, side: 'left' | 'right', color: string, text: string) {
  const out = y < r.y ? -1 : y > r.y + r.h ? 1 : 0
  y = Math.max(r.y, Math.min(r.y + r.h, y))
  const d = side === 'left' ? -1 : 1
  g.fillStyle = color
  g.beginPath()
  if (out) {
    g.moveTo(x + d * 10, y - out * 2)
    g.lineTo(x + d * 4, y + out * 8)
    g.lineTo(x + d * 16, y + out * 8)
  } else {
    g.moveTo(x, y)
    g.lineTo(x + d * 7, y - 8)
    g.lineTo(x + d * 22, y - 8)
    g.lineTo(x + d * 22, y + 8)
    g.lineTo(x + d * 7, y + 8)
  }
  g.closePath()
  g.fill()
  if (!out) {
    g.fillStyle = '#0b0d11'
    g.font = '600 11px Segoe UI, sans-serif'
    g.textAlign = 'center'
    g.textBaseline = 'middle'
    g.fillText(text, x + d * 14.5, y + 0.5)
  }
}

function drawHover(g: CanvasRenderingContext2D, r: ReturnType<typeof plotRect>, mx: number) {
  if (mx < r.x || mx > r.x + r.w) return
  g.strokeStyle = 'rgba(230,232,237,.35)'
  g.setLineDash([3, 3])
  g.beginPath()
  line(g, mx, r.y, mx, r.y + r.h)
  g.stroke()
  g.setLineDash([])
  const f = props.frame
  if (!f) return
  const xdiv = (mx - r.x) / r.div
  const hoff = parseFloat(props.scope['hor.offset']) || 0
  const rows: [string, string][] = [['t', eng((xdiv - DIVX / 2 - hoff) * f.timeDiv, 'с', 4)]]
  for (const name of ['ch1', 'ch2'] as const) {
    const { ch, on } = chInfo(name)
    if (!on || !ch?.raw?.length) continue
    const i = Math.round((xdiv / DIVX) * (ch.raw.length - 1))
    const v = ((ch.raw[i] - ch.offset) / PX) * ch.voltsPerDiv
    rows.push([name.toUpperCase(), eng(v, 'В', 4)])
    const y = r.cy - (ch.raw[i] / PX) * r.div
    g.fillStyle = colors[name]
    g.beginPath()
    g.arc(mx, y, 3, 0, Math.PI * 2)
    g.fill()
  }
  g.font = '12px Cascadia Mono, Consolas, monospace'
  const w = 150, h = rows.length * 17 + 8
  const bx = mx + 12 + w > r.x + r.w ? mx - 12 - w : mx + 12
  const by = r.y + 8
  g.fillStyle = 'rgba(20,23,30,.92)'
  g.strokeStyle = '#323947'
  g.beginPath()
  g.roundRect(bx, by, w, h, 6)
  g.fill()
  g.stroke()
  g.textBaseline = 'middle'
  rows.forEach(([k, v], i) => {
    const y = by + 13 + i * 17
    g.textAlign = 'left'
    g.fillStyle = colors[k.toLowerCase()] ?? '#8f97a8'
    g.fillText(k, bx + 9, y)
    g.textAlign = 'right'
    g.fillStyle = '#e6e8ed'
    g.fillText(v, bx + w - 9, y)
  })
}

function pos(e: MouseEvent) {
  const b = canvas.value!.getBoundingClientRect()
  return { x: e.clientX - b.left, y: e.clientY - b.top }
}

function snap(div: number) {
  return Math.round(Math.max(-DIVY / 2, Math.min(DIVY / 2, div)) * PX) / PX
}

function down(e: MouseEvent) {
  const p = pos(e), r = plotRect()
  const near = (y: number) => Math.abs(p.y - Math.max(r.y, Math.min(r.y + r.h, y))) < 10
  if (p.x < r.x) {
    const tag = groundTags(r).find((t) => near(t.y) && p.x >= t.x - 24 && p.x <= t.x + 2)
    if (tag) drag.value = { kind: tag.name, div: chInfo(tag.name).offsetDiv }
  } else if (p.x > r.x + r.w) {
    const td = trigDiv()
    if (td != null && near(r.cy - td * r.div)) drag.value = { kind: 'trig', div: td }
  }
  if (drag.value) {
    window.addEventListener('mousemove', dragMove)
    window.addEventListener('mouseup', dragEnd, { once: true })
  }
}

function dragMove(e: MouseEvent) {
  if (!drag.value) return
  const r = plotRect()
  const div = snap((r.cy - pos(e).y) / r.div)
  if (div === drag.value.div) return
  drag.value = { ...drag.value, div }
  if (drag.value.kind === 'trig') {
    const src = (props.scope['trig.source'] || 'CH1').toLowerCase() as 'ch1' | 'ch2'
    const { vdiv, offsetDiv } = chInfo(src)
    emit('level', (div - offsetDiv) * vdiv)
  } else {
    emit('offset', drag.value.kind, div)
  }
  schedule()
}

function dragEnd() {
  window.removeEventListener('mousemove', dragMove)
  drag.value = null
  schedule()
}

function move(e: MouseEvent) {
  hover.value = pos(e)
  const p = hover.value, r = plotRect()
  const c = canvas.value!
  c.style.cursor = p.x < r.x || p.x > r.x + r.w ? 'ns-resize' : 'crosshair'
  schedule()
}

function toPNG(): Promise<Blob | null> {
  return new Promise((res) => canvas.value!.toBlob(res, 'image/png'))
}
defineExpose({ toPNG })

let ro: ResizeObserver | undefined
onMounted(() => {
  const cs = getComputedStyle(document.documentElement)
  colors = { ch1: cs.getPropertyValue('--ch1').trim(), ch2: cs.getPropertyValue('--ch2').trim(), accent: cs.getPropertyValue('--accent').trim() }
  ro = new ResizeObserver(() => {
    const b = box.value!.getBoundingClientRect()
    size = { w: b.width, h: b.height, dpr: window.devicePixelRatio || 1 }
    canvas.value!.width = Math.round(b.width * size.dpr)
    canvas.value!.height = Math.round(b.height * size.dpr)
    draw()
  })
  ro.observe(box.value!)
})
onBeforeUnmount(() => ro?.disconnect())
watch(() => [props.frame, { ...props.scope }], schedule, { deep: false })
</script>

<template>
  <div ref="box" class="screen">
    <canvas ref="canvas" @mousedown="down" @mousemove="move" @mouseleave="hover = null; schedule()" />
    <slot />
  </div>
</template>

<style scoped>
.screen { position: relative; min-height: 0; min-width: 0; }
canvas { position: absolute; inset: 0; width: 100%; height: 100%; }
</style>
