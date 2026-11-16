<script setup lang="ts">
// The oscilloscope screen: 12 × 8 divisions, 25 sample counts per division,
// as on the device. Channel ground markers on the left and the trigger level
// marker on the right can be dragged, and so can the cursors on the screen
// and the marker of the math channel. The reference waveform lies under the
// live traces, and so do the persistence traces. With stretch the screen fills its box,
// and a division is wider than it is tall. In the X-Y mode the screen is a
// square of 8 × 8 divisions with CH1 across and CH2 up.
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { hasT, hasV, levelAt, sampleAt, snapT, snapV, timeAt, voltsAt } from '../cursors'
import { eng } from '../format'
import { mathUnit } from '../math'
import { Layer, fadeFactor, hexRgb } from '../persist'
import { refAt, type RefName, type Reference } from '../ref'
import { cursors, display, mathTrace, persistEpoch, reference } from '../state'
import type { Frame, FrameChannel } from '../types'

const props = defineProps<{ frame: Frame | null; scope: Record<string, string>; stretch?: boolean }>()
const emit = defineEmits<{ offset: [ch: 'ch1' | 'ch2', div: number]; level: [volts: number] }>()

const DIVX = 12, DIVY = 8, PX = 25
const M = { l: 28, r: 28, t: 16, b: 8 }

const box = ref<HTMLDivElement>()
const canvas = ref<HTMLCanvasElement>()
const hover = ref<{ x: number; y: number } | null>(null)
type Drag = { kind: 'ch1' | 'ch2' | 'trig' | 'math'; div: number } | { kind: 't'; i: 0 | 1 } | { kind: 'v'; i: 0 | 1 }
const drag = ref<Drag | null>(null)
let colors: Record<string, string> = {}
let size = { w: 0, h: 0, dpr: 1 }
let raf = 0

function plotRect() {
  const aw = size.w - M.l - M.r, ah = size.h - M.t - M.b
  const nx = display.xy ? DIVY : DIVX
  let pw = Math.max(10, Math.min(aw, ah * nx / DIVY))
  let ph = pw * DIVY / nx
  if (props.stretch && !display.xy) [pw, ph] = [Math.max(10, aw), Math.max(10, ah)]
  const x = M.l + (aw - pw) / 2, y = M.t + (ah - ph) / 2
  return { x, y, w: pw, h: ph, nx, dx: pw / nx, dy: ph / DIVY, cx: x + pw / 2, cy: y + ph / 2 }
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
  for (let i = 1; i < r.nx; i++) if (i !== r.nx / 2) line(g, r.x + i * r.dx, r.y, r.x + i * r.dx, r.y + r.h)
  for (let i = 1; i < DIVY; i++) if (i !== DIVY / 2) line(g, r.x, r.y + i * r.dy, r.x + r.w, r.y + i * r.dy)
  g.stroke()
  g.setLineDash([])
  g.strokeStyle = '#363d4b'
  g.beginPath()
  line(g, r.cx, r.y, r.cx, r.y + r.h)
  line(g, r.x, r.cy, r.x + r.w, r.cy)
  for (let i = 0; i <= r.nx * 5; i++) line(g, r.x + i * r.dx / 5, r.cy - 3, r.x + i * r.dx / 5, r.cy + 3)
  for (let i = 0; i <= DIVY * 5; i++) line(g, r.cx - 3, r.y + i * r.dy / 5, r.cx + 3, r.y + i * r.dy / 5)
  g.stroke()
  g.strokeStyle = '#2b313d'
  g.strokeRect(r.x + 0.5, r.y + 0.5, r.w - 1, r.h - 1)

  // Traces.
  g.save()
  g.beginPath()
  g.rect(r.x, r.y, r.w, r.h)
  g.clip()
  accumulate(r)
  if (display.xy) {
    const ok = drawXY(g, r)
    g.restore()
    xyLabels(g, r, ok)
    if (hover.value) drawHoverXY(g, r, hover.value)
    return
  }
  if (display.ref && reference.value) drawRef(g, r, reference.value)
  paintLayers(g, r)
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
      const x = r.x + i * k, y = r.cy - (raw[i] / PX) * r.dy
      i ? g.lineTo(x, y) : g.moveTo(x, y)
    }
    g.stroke()
  }
  const mt = mathTrace.value
  if (mt && mt.v.length > 1) {
    g.strokeStyle = colors.math
    g.beginPath()
    const k = r.w / (mt.v.length - 1)
    mt.v.forEach((v, i) => {
      const x = r.x + i * k, y = mathY(r, v)
      i ? g.lineTo(x, y) : g.moveTo(x, y)
    })
    g.stroke()
  }
  g.restore()

  // Ground markers of the channels, CH1 on top as with the traces.
  for (const t of groundTags(r).reverse()) marker(g, t.x, t.y, r, 'left', colors[t.name], t.label)
  // Trigger level and trigger position.
  const td = trigDiv()
  const src = (props.scope['trig.source'] || 'CH1').toLowerCase()
  if (td != null) marker(g, r.x + r.w + 4, r.cy - td * r.dy, r, 'right', colors[src] ?? colors.ch1, 'T')
  const hoff = parseFloat(props.scope['hor.offset']) || 0
  // A positive offset moves the trigger point left of the centre.
  const tx = Math.max(r.x, Math.min(r.x + r.w, r.cx - hoff * r.dx))
  g.fillStyle = colors.accent
  g.beginPath()
  g.moveTo(tx - 6, r.y - 11)
  g.lineTo(tx + 6, r.y - 11)
  g.lineTo(tx, r.y - 3)
  g.fill()

  drawCursors(g, r)
  if (hover.value && !drag.value) drawHover(g, r, hover.value.x)
}

// mathY places a value of the math channel on the screen.
function mathY(r: ReturnType<typeof plotRect>, v: number) {
  return r.cy - (v / display.mathScale + display.mathOffset) * r.dy
}

// drawXY draws CH2 against CH1, each at its own scale and offset, as the
// samples follow each other. It needs both channels.
function drawXY(g: CanvasRenderingContext2D, r: ReturnType<typeof plotRect>) {
  const a = chInfo('ch1'), b = chInfo('ch2')
  if (display.ref && reference.value && a.ch && b.ch) drawRefXY(g, r, reference.value, a.ch, b.ch)
  paintLayers(g, r)
  const xs = a.ch?.raw, ys = b.ch?.raw
  if (!a.on || !b.on || !xs || !ys || xs.length < 2 || xs.length !== ys.length) return false
  g.strokeStyle = colors.accent
  g.lineWidth = 1.4
  g.beginPath()
  for (let i = 0; i < xs.length; i++) {
    const x = r.cx + (xs[i] / PX) * r.dx, y = r.cy - (ys[i] / PX) * r.dy
    i ? g.lineTo(x, y) : g.moveTo(x, y)
  }
  g.stroke()
  return true
}

// Persistence: a layer per trace, in CSS pixels of the plot, cleared when
// anything that places the traces on the screen changes.
let layers = new Map<string, { layer: Layer; canvas: HTMLCanvasElement }>()
let layerKey = ''
let lastSeq = -1
let lastAdd = 0
let layersDirty = false

function accumulate(r: ReturnType<typeof plotRect>) {
  const f = props.frame
  if (display.persist === 'off' || !f) {
    layers.clear()
    layerKey = ''
    return
  }
  const key = [
    Math.round(r.w), Math.round(r.h), display.persist, display.xy, display.math, display.mathScale, display.mathOffset,
    persistEpoch.value, f.timeDiv, f.hOffset, ...f.channels.map((c) => `${c.on} ${c.voltsPerDiv} ${c.offset}`),
  ].join('|')
  if (key !== layerKey) {
    layers = new Map()
    layerKey = key
    lastSeq = -1
  }
  if (f.seq === lastSeq) return
  const now = performance.now()
  const k = lastSeq < 0 ? 1 : fadeFactor(display.persist, (now - lastAdd) / 1000)
  lastSeq = f.seq
  lastAdd = now
  for (const [name, xs, ys] of layerTraces(r, f)) {
    let l = layers.get(name)
    if (!l) {
      const canvas = document.createElement('canvas')
      const layer = new Layer(r.w, r.h)
      ;[canvas.width, canvas.height] = [layer.w, layer.h]
      layers.set(name, (l = { layer, canvas }))
    }
    l.layer.fade(k)
    l.layer.add(xs, ys)
  }
  layersDirty = true
}

// layerTraces gives the traces of a frame in pixels from the plot corner.
function layerTraces(r: ReturnType<typeof plotRect>, f: Frame): [string, number[], number[]][] {
  const ch = (name: string) => f.channels.find((c) => c.name.toLowerCase() === name && c.on && c.raw && c.raw.length > 1)?.raw
  if (display.xy) {
    const a = ch('ch1'), b = ch('ch2')
    if (!a || !b || a.length !== b.length) return []
    return [['xy', a.map((x) => r.w / 2 + (x / PX) * r.dx), b.map((y) => r.h / 2 - (y / PX) * r.dy)]]
  }
  const out: [string, number[], number[]][] = []
  const xsOf = (n: number) => Array.from({ length: n }, (_, i) => (i * r.w) / (n - 1))
  for (const name of ['ch1', 'ch2']) {
    const raw = ch(name)
    if (raw) out.push([name, xsOf(raw.length), raw.map((y) => r.h / 2 - (y / PX) * r.dy)])
  }
  const mt = mathTrace.value
  if (mt && mt.v.length > 1) out.push(['math', xsOf(mt.v.length), mt.v.map((v) => mathY(r, v) - r.y)])
  return out
}

function paintLayers(g: CanvasRenderingContext2D, r: ReturnType<typeof plotRect>) {
  for (const [name, { layer, canvas }] of layers) {
    if (layersDirty) {
      const cg = canvas.getContext('2d')!
      const img = cg.createImageData(layer.w, layer.h)
      layer.render(img, hexRgb(name === 'xy' ? colors.accent : colors[name]))
      cg.putImageData(img, 0, 0)
    }
    g.drawImage(canvas, r.x, r.y, layer.w, layer.h)
  }
  layersDirty = false
}

const refColor = (name: RefName) => colors[name === 'M' ? 'math' : name.toLowerCase()]

// refY places a value of a reference trace at the scale its source has now.
function refY(r: ReturnType<typeof plotRect>, name: RefName): ((v: number) => number) | null {
  if (name === 'M') return (v) => mathY(r, v)
  const ch = props.frame?.channels.find((c) => c.name === name)
  if (!ch?.voltsPerDiv) return null
  return (v) => r.cy - (v / ch.voltsPerDiv + ch.offset / PX) * r.dy
}

// drawRef draws the reference wide and faint, so that the live trace over it
// stays readable, at the time base of the live frame.
function drawRef(g: CanvasRenderingContext2D, r: ReturnType<typeof plotRect>, ref: Reference) {
  const f = props.frame
  if (!f?.timeDiv) return
  const xOf = (t: number) => r.x + (t / f.timeDiv + DIVX / 2 - f.hOffset) * r.dx
  g.save()
  g.globalAlpha = 0.35
  g.lineWidth = 3.5
  g.lineJoin = 'round'
  for (const c of ref.traces) {
    const yOf = refY(r, c.name)
    if (!yOf) continue
    g.strokeStyle = refColor(c.name)
    g.beginPath()
    c.v.forEach((v, i) => {
      const x = xOf(ref.t[i]), y = yOf(v)
      i ? g.lineTo(x, y) : g.moveTo(x, y)
    })
    g.stroke()
  }
  g.restore()
}

function drawRefXY(g: CanvasRenderingContext2D, r: ReturnType<typeof plotRect>, ref: Reference, a: FrameChannel, b: FrameChannel) {
  const xs = ref.traces.find((c) => c.name === 'CH1')?.v, ys = ref.traces.find((c) => c.name === 'CH2')?.v
  if (!xs || !ys || !a.voltsPerDiv || !b.voltsPerDiv) return
  g.save()
  g.globalAlpha = 0.35
  g.lineWidth = 3.5
  g.lineJoin = 'round'
  g.strokeStyle = colors.accent
  g.beginPath()
  xs.forEach((u, i) => {
    const x = r.cx + (u / a.voltsPerDiv + a.offset / PX) * r.dx, y = r.cy - (ys[i] / b.voltsPerDiv + b.offset / PX) * r.dy
    i ? g.lineTo(x, y) : g.moveTo(x, y)
  })
  g.stroke()
  g.restore()
}

function xyLabels(g: CanvasRenderingContext2D, r: ReturnType<typeof plotRect>, ok: boolean) {
  g.textBaseline = 'middle'
  if (!ok) {
    g.font = '13px Segoe UI, sans-serif'
    g.textAlign = 'center'
    g.fillStyle = '#8f97a8'
    g.fillText('Для X‑Y включите оба канала', r.cx, r.cy - 20)
    return
  }
  g.font = '11px Cascadia Mono, Consolas, monospace'
  g.textAlign = 'right'
  const rows: [string, string][] = [
    [`X: CH1 ${eng(chInfo('ch1').vdiv, 'В', 3, true)}/дел`, colors.ch1],
    [`Y: CH2 ${eng(chInfo('ch2').vdiv, 'В', 3, true)}/дел`, colors.ch2],
  ]
  rows.forEach(([t, color], i) => {
    g.fillStyle = color
    g.fillText(t, r.x + r.w - 8, r.y + r.h - 12 - (rows.length - 1 - i) * 15)
  })
}

// drawHoverXY reads both channels at the mouse.
function drawHoverXY(g: CanvasRenderingContext2D, r: ReturnType<typeof plotRect>, p: { x: number; y: number }) {
  if (p.x < r.x || p.x > r.x + r.w || p.y < r.y || p.y > r.y + r.h) return
  const a = chInfo('ch1').ch, b = chInfo('ch2').ch
  if (!a || !b) return
  g.strokeStyle = 'rgba(230,232,237,.35)'
  g.setLineDash([3, 3])
  g.beginPath()
  line(g, p.x, r.y, p.x, r.y + r.h)
  line(g, r.x, p.y, r.x + r.w, p.y)
  g.stroke()
  g.setLineDash([])
  tooltip(g, r, p.x, [
    ['CH1', eng(voltsAt(((p.x - r.cx) / r.dx) * PX, a.offset, a.voltsPerDiv), 'В', 4), colors.ch1],
    ['CH2', eng(voltsAt(((r.cy - p.y) / r.dy) * PX, b.offset, b.voltsPerDiv), 'В', 4), colors.ch2],
  ])
}

// drawCursors draws the cursor lines and, in the lower left corner, what
// they read: the time cursors the interval and every channel at them, the
// voltage cursors the levels of their channel.
function drawCursors(g: CanvasRenderingContext2D, r: ReturnType<typeof plotRect>) {
  const mode = cursors.mode
  if (mode === 'off') return
  const src = cursors.src
  g.save()
  g.beginPath()
  g.rect(r.x, r.y, r.w, r.h)
  g.clip()
  g.lineWidth = 1
  g.font = '600 10px Segoe UI, sans-serif'
  g.setLineDash([6, 4])
  if (hasT(mode)) {
    g.strokeStyle = 'rgba(230,232,237,.7)'
    g.fillStyle = '#e6e8ed'
    g.textAlign = 'left'
    g.textBaseline = 'top'
    cursors.t.forEach((t, i) => {
      const x = r.x + t * r.dx
      g.beginPath()
      line(g, x, r.y, x, r.y + r.h)
      g.stroke()
      g.fillText(String(i + 1), x + 3, r.y + 3)
    })
  }
  if (hasV(mode)) {
    g.strokeStyle = colors[src]
    g.fillStyle = colors[src]
    g.textAlign = 'right'
    g.textBaseline = 'bottom'
    cursors.v.forEach((v, i) => {
      const y = r.cy - v * r.dy
      g.beginPath()
      line(g, r.x, y, r.x + r.w, y)
      g.stroke()
      g.fillText(String(i + 1), r.x + r.w - 4, y - 2)
    })
  }
  g.setLineDash([])
  g.restore()

  const f = props.frame
  if (!f) return
  const rows: { label: string; color: string; text: string }[] = []
  const pair = (a: number, b: number, unit: string) => `${eng(b - a, unit, 4)}   ${eng(a, unit, 3)} / ${eng(b, unit, 3)}`
  if (hasT(mode) && f.timeDiv > 0) {
    const hoff = parseFloat(props.scope['hor.offset']) || 0
    const [ta, tb] = cursors.t.map((x) => timeAt(x, f.timeDiv, hoff))
    const dt = tb - ta
    rows.push({ label: 'Δt', color: '#e6e8ed', text: dt ? `${eng(dt, 'с', 4)}   ${eng(1 / Math.abs(dt), 'Гц', 4)}` : '0' })
    for (const name of ['ch1', 'ch2'] as const) {
      const { ch, on } = chInfo(name)
      if (!on || !ch?.raw || ch.raw.length < 2) continue
      const [va, vb] = cursors.t.map((x) => voltsAt(sampleAt(ch.raw!, x), ch.offset, ch.voltsPerDiv))
      rows.push({ label: name.toUpperCase(), color: colors[name], text: pair(va, vb, 'В') })
    }
    const mt = mathTrace.value
    if (mt) {
      const [ma, mb] = cursors.t.map((x) => sampleAt(mt.v, x))
      rows.push({ label: 'M', color: colors.math, text: pair(ma, mb, mathUnit(display.math)) })
    }
  }
  if (hasV(mode)) {
    const { ch } = chInfo(src)
    if (ch?.voltsPerDiv) {
      const [va, vb] = cursors.v.map((y) => levelAt(y, ch.offset, ch.voltsPerDiv))
      rows.push({ label: 'ΔV', color: colors[src], text: pair(va, vb, 'В') })
    }
  }
  if (!rows.length) return
  g.font = '12px Cascadia Mono, Consolas, monospace'
  const w = 52 + Math.max(...rows.map((row) => g.measureText(row.text).width)) + 12
  const h = rows.length * 17 + 8
  const bx = r.x + 8, by = r.y + r.h - h - 8
  g.fillStyle = 'rgba(20,23,30,.85)'
  g.strokeStyle = '#323947'
  g.beginPath()
  g.roundRect(bx, by, w, h, 6)
  g.fill()
  g.stroke()
  g.textBaseline = 'middle'
  rows.forEach((row, i) => {
    const y = by + 13 + i * 17
    g.textAlign = 'left'
    g.fillStyle = row.color
    g.fillText(row.label, bx + 9, y)
    g.textAlign = 'right'
    g.fillStyle = '#e6e8ed'
    g.fillText(row.text, bx + w - 9, y)
  })
}

// cursorAt finds the cursor line within a few pixels of a point on the
// screen, the nearest when two are close.
function cursorAt(p: { x: number; y: number }, r: ReturnType<typeof plotRect>): Drag | null {
  if (p.x < r.x || p.x > r.x + r.w || p.y < r.y || p.y > r.y + r.h) return null
  let best: Drag | null = null, bd = 8
  if (hasT(cursors.mode)) {
    cursors.t.forEach((t, i) => {
      const d = Math.abs(p.x - (r.x + t * r.dx))
      if (d < bd) [best, bd] = [{ kind: 't', i: i as 0 | 1 }, d]
    })
  }
  if (hasV(cursors.mode)) {
    cursors.v.forEach((v, i) => {
      const d = Math.abs(p.y - (r.cy - v * r.dy))
      if (d < bd) [best, bd] = [{ kind: 'v', i: i as 0 | 1 }, d]
    })
  }
  return best
}

function line(g: CanvasRenderingContext2D, x1: number, y1: number, x2: number, y2: number) {
  g.moveTo(Math.round(x1) + 0.5, Math.round(y1) + 0.5)
  g.lineTo(Math.round(x2) + 0.5, Math.round(y2) + 0.5)
}

// groundTags places the channel ground markers. When both sit at the same
// height, the CH2 tag moves left so that both stay visible and grabbable.
function groundTags(r: ReturnType<typeof plotRect>) {
  const tags: { name: 'ch1' | 'ch2' | 'math'; label: string; x: number; y: number }[] = []
  for (const name of ['ch1', 'ch2'] as const) {
    const { on, offsetDiv } = chInfo(name)
    if (on) tags.push({ name, label: name.slice(2), x: r.x - 4, y: r.cy - offsetDiv * r.dy })
  }
  if (mathTrace.value) tags.push({ name: 'math', label: 'M', x: r.x - 4, y: r.cy - display.mathOffset * r.dy })
  // A tag at the height of one before it moves left while there is room.
  const clamp = (y: number) => Math.max(r.y, Math.min(r.y + r.h, y))
  for (let i = 1; i < tags.length; i++) {
    for (let moved = true; moved; ) {
      moved = false
      for (let j = 0; j < i; j++) {
        if (tags[j].x === tags[i].x && Math.abs(clamp(tags[i].y) - clamp(tags[j].y)) < 16 && tags[i].x >= 46) {
          tags[i].x -= 24
          moved = true
        }
      }
    }
  }
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
  const xdiv = (mx - r.x) / r.dx
  const hoff = parseFloat(props.scope['hor.offset']) || 0
  const t = (xdiv - DIVX / 2 + hoff) * f.timeDiv
  const rows: [string, string, string][] = [['t', eng(t, 'с', 4), '#8f97a8']]
  const dot = (y: number, color: string) => {
    g.fillStyle = color
    g.beginPath()
    g.arc(mx, y, 3, 0, Math.PI * 2)
    g.fill()
  }
  for (const name of ['ch1', 'ch2'] as const) {
    const { ch, on } = chInfo(name)
    if (!on || !ch?.raw?.length) continue
    const i = Math.round((xdiv / DIVX) * (ch.raw.length - 1))
    const v = ((ch.raw[i] - ch.offset) / PX) * ch.voltsPerDiv
    rows.push([name.toUpperCase(), eng(v, 'В', 4), colors[name]])
    dot(r.cy - (ch.raw[i] / PX) * r.dy, colors[name])
  }
  const mt = mathTrace.value
  if (mt) {
    const i = Math.round((xdiv / DIVX) * (mt.v.length - 1))
    rows.push(['M', eng(mt.v[i], mathUnit(display.math), 4), colors.math])
    dot(mathY(r, mt.v[i]), colors.math)
  }
  const ref = display.ref ? reference.value : null
  for (const c of ref?.traces ?? []) {
    const v = refAt(ref!, c.v, t)
    if (v != null) rows.push([`оп. ${c.name}`, eng(v, c.name === 'M' ? mathUnit(ref!.math) : 'В', 4), refColor(c.name)])
  }
  tooltip(g, r, mx, rows)
}

// tooltip draws the readout box next to the mouse at the top of the screen.
function tooltip(g: CanvasRenderingContext2D, r: ReturnType<typeof plotRect>, mx: number, rows: [string, string, string][]) {
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
  rows.forEach(([k, v, color], i) => {
    const y = by + 13 + i * 17
    g.textAlign = 'left'
    g.fillStyle = color
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
  if (display.xy) return
  const p = pos(e), r = plotRect()
  const near = (y: number) => Math.abs(p.y - Math.max(r.y, Math.min(r.y + r.h, y))) < 10
  if (p.x < r.x) {
    const tag = groundTags(r).find((t) => near(t.y) && p.x >= t.x - 24 && p.x <= t.x + 2)
    if (tag) drag.value = tag.name === 'math' ? { kind: 'math', div: display.mathOffset } : { kind: tag.name, div: chInfo(tag.name).offsetDiv }
  } else if (p.x > r.x + r.w) {
    const td = trigDiv()
    if (td != null && near(r.cy - td * r.dy)) drag.value = { kind: 'trig', div: td }
  } else {
    drag.value = cursorAt(p, r)
  }
  if (drag.value) {
    window.addEventListener('mousemove', dragMove)
    window.addEventListener('mouseup', dragEnd, { once: true })
  }
}

function dragMove(e: MouseEvent) {
  if (!drag.value) return
  const r = plotRect()
  const d = drag.value
  if (d.kind === 't' || d.kind === 'v') {
    const p = pos(e)
    if (d.kind === 't') cursors.t[d.i] = snapT((p.x - r.x) / r.dx)
    else cursors.v[d.i] = snapV((r.cy - p.y) / r.dy)
    return
  }
  const div = snap((r.cy - pos(e).y) / r.dy)
  if (div === d.div) return
  drag.value = { ...d, div }
  if (d.kind === 'math') {
    display.mathOffset = div
  } else if (d.kind === 'trig') {
    const src = (props.scope['trig.source'] || 'CH1').toLowerCase() as 'ch1' | 'ch2'
    const { vdiv, offsetDiv } = chInfo(src)
    emit('level', (div - offsetDiv) * vdiv)
  } else {
    emit('offset', d.kind, div)
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
  const hit = drag.value ?? cursorAt(p, r)
  c.style.cursor = display.xy ? 'crosshair' : p.x < r.x || p.x > r.x + r.w || hit?.kind === 'v' ? 'ns-resize' : hit?.kind === 't' ? 'ew-resize' : 'crosshair'
  schedule()
}

function toPNG(): Promise<Blob | null> {
  return new Promise((res) => canvas.value!.toBlob(res, 'image/png'))
}
defineExpose({ toPNG })

let ro: ResizeObserver | undefined
onMounted(() => {
  const cs = getComputedStyle(document.documentElement)
  colors = {
    ch1: cs.getPropertyValue('--ch1').trim(),
    ch2: cs.getPropertyValue('--ch2').trim(),
    math: cs.getPropertyValue('--math').trim(),
    accent: cs.getPropertyValue('--accent').trim(),
  }
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
watch(() => [props.frame, { ...props.scope }, props.stretch], schedule, { deep: false })
watch([cursors, display], schedule, { deep: true })
watch([reference, persistEpoch], schedule)
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
