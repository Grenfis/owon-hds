<script setup lang="ts">
// Spectrum of one channel, computed here from the screen samples of every
// frame: the device has an FFT menu of its own, but no commands for it.
// Cursors measure the frequency and level between two points.
import { computed, onBeforeUnmount, onMounted, reactive, ref, shallowRef, watch } from 'vue'
import { WINDOWS, db, isInterleaveImage, peakNear, peaks, spectrum, thd, type Peak, type Spectrum, type WindowName } from '../fft'
import { eng, stamp } from '../format'
import type { Frame } from '../types'
import Seg from './Seg.vue'

const props = defineProps<{ frame: Frame | null }>()
const emit = defineEmits<{ save: [name: string, blob: Blob] }>()

const DIVX = 10, DIVY = 8
const M = { l: 58, r: 14, t: 10, b: 24 }

const SETTINGS_KEY = 'owon.fft'
// Cursor positions are shares of the plot width and height.
const opts = reactive({ source: 'CH1', window: 'hann' as WindowName, scale: 'db', dbDiv: '10', zoom: '1', avg: '0', cur: 'off', cf: [0.2, 0.4], ca: [0.25, 0.6] })
try {
  Object.assign(opts, JSON.parse(localStorage.getItem(SETTINGS_KEY) ?? '{}'))
} catch {}
watch(opts, () => localStorage.setItem(SETTINGS_KEY, JSON.stringify(opts)))

const SOURCES = [{ value: 'CH1', label: 'CH1' }, { value: 'CH2', label: 'CH2' }]
const SCALES = [
  { value: 'db', label: 'дБВ', title: 'Логарифмическая шкала: 0 дБВ = 1 В СКЗ. Видно и сильные, и слабые составляющие' },
  { value: 'lin', label: 'В', title: 'Линейная шкала в вольтах СКЗ' },
]
const DB_DIVS = ['5', '10', '20'].map((v) => ({ value: v, label: v }))
const ZOOMS = ['1', '2', '5', '10'].map((v) => ({ value: v, label: `×${v}`, title: `Показать первую 1/${v} полосы` }))
const CURSOR_MODES = [
  { value: 'off', label: 'Выкл' },
  { value: 'f', label: 'f', title: 'Две вертикальные линии: частоты и уровни спектра на них. Линия встаёт на ближайший пик' },
  { value: 'a', label: 'A', title: 'Две горизонтальные линии: уровни' },
  { value: 'fa', label: 'f+A', title: 'Обе пары линий' },
]
const hasF = (m: string) => m === 'f' || m === 'fa'
const hasA = (m: string) => m === 'a' || m === 'fa'

const AVGS = [
  { value: '0', label: 'Выкл' },
  { value: '4', label: '4', title: 'Усреднять мощность по 4 кадрам' },
  { value: '16', label: '16', title: 'Усреднять мощность по 16 кадрам' },
]

interface Result {
  s: Spectrum
  peaks: Peak[]
  images: boolean[] // which peaks are images from the interleaved ADCs
  thd: number | null
  fsRms: number // RMS of a sine that fills the screen height
  dt: number
  n: number // samples on the screen
  clipped: boolean
  name: string
}

const result = shallowRef<Result | null>(null)
let avgPow: Float64Array | null = null
let avgKey = ''
let avgCount = 0

function compute(f: Frame | null) {
  const ch = f?.channels.find((c) => c.name === opts.source)
  if (!f || !ch?.on || !ch.raw || ch.raw.length < 16 || !(f.timeDiv > 0)) {
    result.value = null
    return
  }
  const raw = ch.raw
  const volts = raw.map((r) => ((r - ch.offset) / 25) * ch.voltsPerDiv)
  const dt = (f.timeDiv * 12) / raw.length
  const s = spectrum(volts, dt, opts.window)

  // Power averaging; a change of anything that moves the bins starts over.
  const n = +opts.avg
  const key = [opts.source, opts.window, f.timeDiv, raw.length, ch.voltsPerDiv].join('|')
  if (n > 1) {
    if (key !== avgKey || !avgPow || avgPow.length !== s.rms.length) {
      avgPow = new Float64Array(s.rms.length)
      avgKey = key
      avgCount = 0
    }
    avgCount = Math.min(avgCount + 1, n)
    for (let k = 0; k < s.rms.length; k++) {
      avgPow[k] += (s.rms[k] * s.rms[k] - avgPow[k]) / avgCount
      s.rms[k] = Math.sqrt(avgPow[k])
    }
  } else {
    avgPow = null
  }

  const ps = peaks(s, 5)
  result.value = {
    s, peaks: ps, images: ps.map((p) => isInterleaveImage(s, p, ps[0])), thd: ps.length ? thd(s, ps[0]) : null,
    fsRms: (4 * ch.voltsPerDiv) / Math.SQRT2, dt, n: raw.length, clipped: ch.meas.clipped, name: ch.name,
  }
}

watch(() => props.frame, compute)
watch(() => [opts.source, opts.window, opts.avg], () => {
  avgKey = ''
  compute(props.frame)
})

// ---- drawing ----

const box = ref<HTMLDivElement>()
const canvas = ref<HTMLCanvasElement>()
const hover = ref<{ x: number; y: number } | null>(null)
let colors: Record<string, string> = {}
let size = { w: 0, h: 0, dpr: 1 }
let raf = 0

function plotRect() {
  return { x: M.l, y: M.t, w: Math.max(10, size.w - M.l - M.r), h: Math.max(10, size.h - M.t - M.b) }
}

function span(r: Result) {
  return 0.5 / r.dt / +opts.zoom
}

// Vertical axis: top and bottom in dBV or volts.
function yRange(r: Result): [number, number] {
  if (opts.scale === 'lin') return [r.fsRms, 0]
  const d = +opts.dbDiv
  const top = Math.ceil(db(r.fsRms) / d) * d
  return [top, top - DIVY * d]
}

function level(r: Result, v: number) {
  return opts.scale === 'lin' ? v : db(v)
}

function fmtLevel(v: number) {
  return opts.scale === 'lin' ? eng(v, 'В', 3) : `${db(v).toFixed(1).replace('-', '−')} дБВ`
}

function schedule() {
  if (!raf) raf = requestAnimationFrame(() => { raf = 0; draw() })
}

function draw() {
  const c = canvas.value
  if (!c) return
  const g = c.getContext('2d')!
  g.setTransform(size.dpr, 0, 0, size.dpr, 0, 0)
  g.clearRect(0, 0, size.w, size.h)
  const p = plotRect()
  g.fillStyle = '#090b0f'
  g.fillRect(p.x, p.y, p.w, p.h)
  g.strokeStyle = '#20252f'
  g.lineWidth = 1
  g.setLineDash([1, 3])
  g.beginPath()
  for (let i = 1; i < DIVX; i++) line(g, p.x + (i * p.w) / DIVX, p.y, p.x + (i * p.w) / DIVX, p.y + p.h)
  for (let i = 1; i < DIVY; i++) line(g, p.x, p.y + (i * p.h) / DIVY, p.x + p.w, p.y + (i * p.h) / DIVY)
  g.stroke()
  g.setLineDash([])
  g.strokeStyle = '#2b313d'
  g.strokeRect(p.x + 0.5, p.y + 0.5, p.w - 1, p.h - 1)

  const r = result.value
  if (!r) return
  const sp = span(r)
  const [top, bottom] = yRange(r)
  const yOf = (v: number) => p.y + ((top - level(r, v)) / (top - bottom)) * p.h

  // Axis labels.
  g.font = '11px Cascadia Mono, Consolas, monospace'
  g.fillStyle = '#5e6678'
  g.textBaseline = 'top'
  for (let i = 0; i <= DIVX; i += 2) {
    g.textAlign = i === 0 ? 'left' : i === DIVX ? 'right' : 'center'
    g.fillText(eng((i * sp) / DIVX, 'Гц', 3, true), p.x + (i * p.w) / DIVX, p.y + p.h + 6)
  }
  g.textAlign = 'right'
  g.textBaseline = 'middle'
  for (let i = 0; i <= DIVY; i += 2) {
    const v = top - ((top - bottom) * i) / DIVY
    const txt = opts.scale === 'lin' ? eng(v, 'В', 2, true) : `${v.toFixed(0).replace('-', '−')} дБВ`
    g.fillText(txt, p.x - 6, p.y + (i * p.h) / DIVY)
  }

  // Trace: the strongest bin of each pixel column, so narrow peaks survive.
  const color = colors[r.name.toLowerCase()] ?? '#e6e8ed'
  const bins = Math.min(r.s.rms.length - 1, Math.ceil(sp / r.s.df))
  g.save()
  g.beginPath()
  g.rect(p.x, p.y, p.w, p.h)
  g.clip()
  g.beginPath()
  let col = -1, colMax = 0
  const pts: [number, number][] = []
  for (let k = 0; k <= bins; k++) {
    const x = Math.floor(p.x + ((k * r.s.df) / sp) * p.w)
    if (x !== col) {
      if (col >= 0) pts.push([col, yOf(colMax)])
      col = x
      colMax = 0
    }
    colMax = Math.max(colMax, r.s.rms[k])
  }
  pts.push([col, yOf(colMax)])
  pts.forEach(([x, y], i) => (i ? g.lineTo(x, y) : g.moveTo(x, y)))
  g.strokeStyle = color
  g.lineWidth = 1.3
  g.stroke()
  g.lineTo(pts[pts.length - 1][0], p.y + p.h)
  g.lineTo(pts[0][0], p.y + p.h)
  g.closePath()
  g.globalAlpha = 0.12
  g.fillStyle = color
  g.fill()
  g.globalAlpha = 1

  // Peak markers.
  g.font = '600 10px Segoe UI, sans-serif'
  g.textAlign = 'center'
  g.textBaseline = 'middle'
  r.peaks.forEach((pk, i) => {
    if (pk.freq > sp) return
    // Kept inside the plot: images sit right at its right edge.
    const x = Math.min(p.x + p.w - 8, Math.max(p.x + 8, p.x + (pk.freq / sp) * p.w))
    const y = Math.max(p.y + 9, yOf(pk.rms) - 10)
    g.fillStyle = r.images[i] ? '#5e6678' : color
    g.beginPath()
    g.arc(x, y, 7, 0, Math.PI * 2)
    g.fill()
    g.fillStyle = '#0b0d11'
    g.fillText(String(i + 1), x, y + 0.5)
  })
  g.restore()

  drawCursors(g, p, r, sp, yOf, top, bottom)
  if (hover.value && !drag.value) drawHover(g, p, r, sp, yOf)
}

// The bin under a frequency cursor.
function cursorBin(r: Result, frac: number, sp: number) {
  return Math.max(0, Math.min(r.s.rms.length - 1, Math.round((frac * sp) / r.s.df)))
}

// drawCursors draws the cursor lines and, in the upper right corner, what
// they read: the frequency cursors the spectrum at them, the level cursors
// their levels, and the difference from cursor 1 to cursor 2.
function drawCursors(g: CanvasRenderingContext2D, p: ReturnType<typeof plotRect>, r: Result, sp: number, yOf: (v: number) => number, top: number, bottom: number) {
  const mode = opts.cur
  if (mode === 'off') return
  g.save()
  g.beginPath()
  g.rect(p.x, p.y, p.w, p.h)
  g.clip()
  g.lineWidth = 1
  g.font = '600 10px Segoe UI, sans-serif'
  g.setLineDash([6, 4])
  g.strokeStyle = 'rgba(230,232,237,.7)'
  g.fillStyle = '#e6e8ed'
  if (hasF(mode)) {
    g.textAlign = 'left'
    g.textBaseline = 'bottom'
    opts.cf.forEach((frac, i) => {
      const x = p.x + frac * p.w
      g.beginPath()
      line(g, x, p.y, x, p.y + p.h)
      g.stroke()
      g.fillText(String(i + 1), x + 3, p.y + p.h - 3)
      const k = cursorBin(r, frac, sp)
      g.beginPath()
      g.arc(p.x + ((k * r.s.df) / sp) * p.w, yOf(r.s.rms[k]), 3, 0, Math.PI * 2)
      g.fill()
    })
  }
  if (hasA(mode)) {
    g.textAlign = 'left'
    g.textBaseline = 'bottom'
    opts.ca.forEach((frac, i) => {
      const y = p.y + frac * p.h
      g.beginPath()
      line(g, p.x, y, p.x + p.w, y)
      g.stroke()
      g.fillText(String(i + 1), p.x + 4, y - 2)
    })
  }
  g.setLineDash([])
  g.restore()

  const lin = opts.scale === 'lin'
  const ratio = (a: number, b: number) => (a > 0 ? `×${(b / a).toPrecision(3)}` : '')
  const dbDiff = (d: number) => `${d > 0 ? '+' : ''}${d.toFixed(1).replace('-', '−')} дБ`
  const rows: [string, string][] = []
  if (hasF(mode)) {
    const [k1, k2] = opts.cf.map((frac) => cursorBin(r, frac, sp))
    const [v1, v2] = [r.s.rms[k1], r.s.rms[k2]]
    rows.push(['1', `${eng(k1 * r.s.df, 'Гц', 4)}  ${fmtLevel(v1)}`])
    rows.push(['2', `${eng(k2 * r.s.df, 'Гц', 4)}  ${fmtLevel(v2)}`])
    rows.push(['Δ', `${eng((k2 - k1) * r.s.df, 'Гц', 4)}  ${dbDiff(db(v2) - db(v1))}  ${ratio(v1, v2)}`])
  }
  if (hasA(mode)) {
    const [l1, l2] = opts.ca.map((frac) => top - frac * (top - bottom))
    const fmt = (l: number) => (lin ? eng(l, 'В', 3) : `${l.toFixed(1).replace('-', '−')} дБВ`)
    rows.push(['A', `${fmt(l1)} / ${fmt(l2)}  Δ ${lin ? `${eng(l2 - l1, 'В', 3)} ${ratio(l1, l2)}` : dbDiff(l2 - l1)}`])
  }
  g.font = '12px Cascadia Mono, Consolas, monospace'
  const w = 26 + Math.max(...rows.map(([, t]) => g.measureText(t).width)) + 12
  const h = rows.length * 17 + 8
  const bx = p.x + p.w - w - 8, by = p.y + 8
  g.fillStyle = 'rgba(20,23,30,.85)'
  g.strokeStyle = '#323947'
  g.beginPath()
  g.roundRect(bx, by, w, h, 6)
  g.fill()
  g.stroke()
  g.textBaseline = 'middle'
  rows.forEach(([k, t], i) => {
    const y = by + 13 + i * 17
    g.textAlign = 'left'
    g.fillStyle = '#8f97a8'
    g.fillText(k, bx + 9, y)
    g.textAlign = 'right'
    g.fillStyle = '#e6e8ed'
    g.fillText(t, bx + w - 9, y)
  })
}

function drawHover(g: CanvasRenderingContext2D, p: ReturnType<typeof plotRect>, r: Result, sp: number, yOf: (v: number) => number) {
  const mx = hover.value!.x
  if (mx < p.x || mx > p.x + p.w) return
  const f = ((mx - p.x) / p.w) * sp
  // Snap to the strongest bin within a few pixels, so that peaks are easy to hit.
  const k = peakNear(r.s.rms, Math.round(f / r.s.df), Math.max(1, Math.round(((4 / p.w) * sp) / r.s.df)))
  const x = p.x + ((k * r.s.df) / sp) * p.w, y = yOf(r.s.rms[k])
  g.strokeStyle = 'rgba(230,232,237,.35)'
  g.setLineDash([3, 3])
  g.beginPath()
  line(g, x, p.y, x, p.y + p.h)
  g.stroke()
  g.setLineDash([])
  g.fillStyle = '#e6e8ed'
  g.beginPath()
  g.arc(x, y, 3, 0, Math.PI * 2)
  g.fill()
  const rows = [eng(k * r.s.df, 'Гц', 5), fmtLevel(r.s.rms[k]), opts.scale === 'lin' ? `${db(r.s.rms[k]).toFixed(1).replace('-', '−')} дБВ` : eng(r.s.rms[k], 'В', 3)]
  g.font = '12px Cascadia Mono, Consolas, monospace'
  const w = 132, h = rows.length * 17 + 8
  const bx = x + 12 + w > p.x + p.w ? x - 12 - w : x + 12
  g.fillStyle = 'rgba(20,23,30,.92)'
  g.strokeStyle = '#323947'
  g.beginPath()
  g.roundRect(bx, p.y + 8, w, h, 6)
  g.fill()
  g.stroke()
  g.textAlign = 'right'
  g.textBaseline = 'middle'
  rows.forEach((t, i) => {
    g.fillStyle = i ? '#8f97a8' : '#e6e8ed'
    g.fillText(t, bx + w - 9, p.y + 21 + i * 17)
  })
}

function line(g: CanvasRenderingContext2D, x1: number, y1: number, x2: number, y2: number) {
  g.moveTo(Math.round(x1) + 0.5, Math.round(y1) + 0.5)
  g.lineTo(Math.round(x2) + 0.5, Math.round(y2) + 0.5)
}

function pos(e: MouseEvent) {
  const b = canvas.value!.getBoundingClientRect()
  return { x: e.clientX - b.left, y: e.clientY - b.top }
}

function move(e: MouseEvent) {
  const p = pos(e)
  hover.value = p
  const hit = drag.value ?? cursorAt(p)
  canvas.value!.style.cursor = hit?.kind === 'f' ? 'ew-resize' : hit?.kind === 'a' ? 'ns-resize' : 'crosshair'
  schedule()
}

// ---- cursor dragging ----

type CursorDrag = { kind: 'f' | 'a'; i: 0 | 1 }
const drag = ref<CursorDrag | null>(null)

function cursorAt(q: { x: number; y: number }): CursorDrag | null {
  const p = plotRect()
  if (!result.value || q.x < p.x || q.x > p.x + p.w || q.y < p.y || q.y > p.y + p.h) return null
  let best: CursorDrag | null = null, bd = 8
  if (hasF(opts.cur)) {
    opts.cf.forEach((frac, i) => {
      const d = Math.abs(q.x - (p.x + frac * p.w))
      if (d < bd) [best, bd] = [{ kind: 'f', i: i as 0 | 1 }, d]
    })
  }
  if (hasA(opts.cur)) {
    opts.ca.forEach((frac, i) => {
      const d = Math.abs(q.y - (p.y + frac * p.h))
      if (d < bd) [best, bd] = [{ kind: 'a', i: i as 0 | 1 }, d]
    })
  }
  return best
}

function down(e: MouseEvent) {
  drag.value = cursorAt(pos(e))
  if (!drag.value) return
  window.addEventListener('mousemove', dragMove)
  window.addEventListener('mouseup', dragEnd, { once: true })
}

function dragMove(e: MouseEvent) {
  const d = drag.value, q = pos(e), p = plotRect(), r = result.value
  if (!d) return
  const clamp = (v: number) => Math.max(0, Math.min(1, v))
  if (d.kind === 'a') {
    opts.ca[d.i] = clamp((q.y - p.y) / p.h)
  } else if (r) {
    // A frequency cursor lands on a peak within a few pixels.
    const sp = span(r)
    const k = peakNear(r.s.rms, Math.round((clamp((q.x - p.x) / p.w) * sp) / r.s.df), Math.max(1, Math.round(((6 / p.w) * sp) / r.s.df)))
    opts.cf[d.i] = clamp((k * r.s.df) / sp)
  }
  schedule()
}

function dragEnd() {
  window.removeEventListener('mousemove', dragMove)
  drag.value = null
  schedule()
}

// ---- readouts and export ----

const info = computed(() => {
  const r = result.value
  if (!r) return null
  const screenBin = 1 / (r.dt * r.n)
  const f1 = r.peaks[0]
  return {
    fs: 1 / r.dt,
    resolution: screenBin,
    // The main lobe reaching 0 Hz gives a wide peak and a rough level: three
    // periods on the screen are enough for Hann, the flat top needs eight.
    fewPeriods: f1 != null && f1.freq < 1.5 * r.s.lobe * r.s.df,
    nearNyquist: f1 != null && f1.freq > 0.4 / r.dt,
  }
})

function exportCSV() {
  const r = result.value
  if (!r) return
  const lines = [`# OWON HDS272S spectrum, ${r.name}, window ${opts.window}, ${opts.avg === '0' ? 'no averaging' : `average of ${opts.avg}`}`, 'f_Hz,Vrms,dBV']
  r.s.rms.forEach((v, k) => lines.push(`${(k * r.s.df).toPrecision(7)},${v.toPrecision(5)},${db(v).toFixed(2)}`))
  emit('save', `hds272s_fft_${stamp()}.csv`, new Blob([lines.join('\n') + '\n'], { type: 'text/csv' }))
}

async function exportPNG() {
  const b = await new Promise<Blob | null>((res) => canvas.value!.toBlob(res, 'image/png'))
  if (b) emit('save', `hds272s_fft_${stamp()}.png`, b)
}

let ro: ResizeObserver | undefined
onMounted(() => {
  const cs = getComputedStyle(document.documentElement)
  colors = { ch1: cs.getPropertyValue('--ch1').trim(), ch2: cs.getPropertyValue('--ch2').trim() }
  ro = new ResizeObserver(() => {
    const b = box.value!.getBoundingClientRect()
    size = { w: b.width, h: b.height, dpr: window.devicePixelRatio || 1 }
    canvas.value!.width = Math.round(b.width * size.dpr)
    canvas.value!.height = Math.round(b.height * size.dpr)
    draw()
  })
  ro.observe(box.value!)
  compute(props.frame)
})
onBeforeUnmount(() => {
  ro?.disconnect()
  window.removeEventListener('mousemove', dragMove)
})
watch([result, () => opts.scale, () => opts.dbDiv, () => opts.zoom, () => opts.cur], schedule)
</script>

<template>
  <div class="spectrum">
    <div class="bar">
      <span class="title">Спектр</span>
      <Seg v-model="opts.source" :options="SOURCES" />
      <select v-model="opts.window" title="Окно FFT">
        <option v-for="w in WINDOWS" :key="w.value" :value="w.value" :title="w.title">{{ w.label }}</option>
      </select>
      <Seg v-model="opts.scale" :options="SCALES" />
      <Seg v-if="opts.scale === 'db'" v-model="opts.dbDiv" :options="DB_DIVS" title="дБ на деление" />
      <Seg v-model="opts.zoom" :options="ZOOMS" />
      <span class="lbl">Усреднение</span>
      <Seg v-model="opts.avg" :options="AVGS" />
      <span class="lbl">Курсоры</span>
      <Seg v-model="opts.cur" :options="CURSOR_MODES" />
      <span class="grow" />
      <button class="ghost" :disabled="!result" title="Сохранить спектр в CSV" @click="exportCSV">CSV</button>
      <button class="ghost" :disabled="!result" title="Сохранить картинку спектра" @click="exportPNG">PNG</button>
    </div>
    <div ref="box" class="plot">
      <canvas ref="canvas" @mousedown="down" @mousemove="move" @mouseleave="hover = null; schedule()" />
      <div v-if="!result" class="empty muted">Нет данных: включите канал {{ opts.source }} или подключите прибор</div>
    </div>
    <div v-if="result" class="peaks" :style="{ '--c': `var(--${result.name.toLowerCase()})` }">
      <span
        v-for="(p, i) in result.peaks"
        :key="i"
        class="peak"
        :class="{ image: result.images[i] }"
        :title="result.images[i] ? 'Не сигнал: прибор берёт отсчёты от двух АЦП по очереди, и их небольшое расхождение зеркалит сигнал около половины частоты отсчётов' : undefined"
      >
        <span class="num">{{ i + 1 }}</span>
        <span class="mono">{{ eng(p.freq, 'Гц', 5) }}</span>
        <span class="mono dim">{{ fmtLevel(p.rms) }}</span>
        <span v-if="result.images[i]" class="tag">АЦП</span>
      </span>
      <span v-if="result.thd != null" class="mono kv" title="Коэффициент гармоник первого пика: гармоники 2…10, что помещаются в полосу">
        КНИ {{ (result.thd * 100).toFixed(result.thd < 0.01 ? 3 : 2) }} %
      </span>
      <span v-if="info" class="mono dim kv" :title="`Полоса до ${eng(info.fs / 2, 'Гц', 3)}: половина частоты отсчётов экрана`">
        шаг {{ eng(info.resolution, 'Гц', 3) }} · отсчёты {{ eng(info.fs, 'Гц', 3) }}
      </span>
      <span v-if="result.clipped" class="warn">Сигнал выходит за экран: в спектре появятся ложные гармоники</span>
      <span v-else-if="info?.fewPeriods" class="warn">Мало периодов на экране для этого окна: увеличьте время/дел</span>
      <span v-else-if="info?.nearNyquist" class="warn" title="Прибор прореживает отсчёты для экрана, и частоты выше половины частоты отсчётов отражаются вниз">
        Пик у края полосы: возможно наложение, уменьшите время/дел
      </span>
    </div>
  </div>
</template>

<style scoped>
.spectrum { display: flex; flex-direction: column; gap: 6px; }
.bar { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; font-size: 12px; }
.title { font-weight: 600; color: var(--muted); text-transform: uppercase; letter-spacing: .06em; font-size: 12px; }
.lbl { color: var(--muted); margin-left: 4px; }
.grow { flex: 1; }
.bar select { font-size: 12px; padding: 3px 6px; }
.plot { position: relative; flex: 1; min-height: 120px; }
canvas { position: absolute; inset: 0; width: 100%; height: 100%; cursor: crosshair; }
.empty { position: absolute; inset: 0; display: grid; place-items: center; font-size: 13px; }
.peaks {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 16px;
  padding: 6px 10px;
  background: var(--panel);
  border: 1px solid var(--line);
  border-left: 3px solid var(--c);
  border-radius: 6px;
  font-size: 12px;
}
.peak { display: inline-flex; align-items: center; gap: 6px; }
.num {
  display: inline-grid;
  place-items: center;
  width: 15px;
  height: 15px;
  border-radius: 50%;
  background: var(--c);
  color: #0b0d11;
  font-size: 10px;
  font-weight: 600;
}
.peak.image { opacity: .6; }
.peak.image .num { background: var(--dim); }
.tag { font-size: 10px; color: var(--muted); border: 1px solid var(--line-2); border-radius: 3px; padding: 0 4px; }
.kv { white-space: nowrap; }
.warn { color: var(--warn); background: var(--warn-bg); border-radius: 4px; padding: 2px 7px; font-size: 11px; }
</style>
