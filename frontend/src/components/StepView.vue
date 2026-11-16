<script setup lang="ts">
// Step response of a circuit, from every frame: a square wave from the
// generator feeds the circuit and CH1, CH2 sees the output. The curve is
// the output scaled from its level before an edge of CH1 to its level at
// the end of the half period, averaged over edges and frames.
import { computed, onBeforeUnmount, onMounted, reactive, ref, shallowRef, watch } from 'vue'
import { TIMEBASES, closest, eng, fitScale, probeFactor, stamp, vdivOptions } from '../format'
import { gen, set, setGen, setTrigLevel } from '../state'
import { average, metrics, steps, type Step, type StepMetrics } from '../step'
import type { Frame } from '../types'
import Seg from './Seg.vue'

const props = defineProps<{ frame: Frame | null; online: boolean }>()
const emit = defineEmits<{ save: [name: string, blob: Blob] }>()

const M = { l: 50, r: 14, t: 10, b: 24 }

const SETTINGS_KEY = 'owon.step'
const opts = reactive({ edge: 'up', band: '5', avg: '4' })
try {
  Object.assign(opts, JSON.parse(localStorage.getItem(SETTINGS_KEY) ?? '{}'))
} catch {}
watch(opts, () => localStorage.setItem(SETTINGS_KEY, JSON.stringify(opts)))

const EDGES = [
  { value: 'up', label: '↑', title: 'Перепад вверх' },
  { value: 'down', label: '↓', title: 'Перепад вниз' },
]
const BANDS = ['2', '5', '10'].map((v) => ({ value: v, label: `±${v} %`, title: `Время установления в пределах ±${v} % от перепада` }))
const AVGS = [
  { value: '1', label: '1', title: 'Только этот кадр' },
  { value: '4', label: '4', title: 'Среднее по 4 кадрам' },
  { value: '16', label: '16', title: 'Среднее по 16 кадрам: меньше шума' },
]

interface Result {
  s: Step
  m: StepMetrics
  dt: number
  count: number // steps averaged
  counts: number // the output step in ADC counts
  clipped: boolean
  self: boolean // no CH2: CH1 against itself
  square: boolean // the input edge is short against the step
}

const result = shallowRef<Result | null>(null)
let history: Step[][] = []
let historyKey = ''

function compute(f: Frame | null) {
  const c1 = f?.channels.find((c) => c.name === 'CH1')
  const c2 = f?.channels.find((c) => c.name === 'CH2')
  if (!f || !c1?.on || !c1.raw?.length || !(f.timeDiv > 0)) {
    result.value = null
    return
  }
  const out = c2?.on && c2.raw?.length === c1.raw.length ? c2 : c1
  const volts = (c: typeof c1) => c.raw!.map((r) => ((r - c.offset) / 25) * c.voltsPerDiv)
  const dt = (f.timeDiv * 12) / c1.raw.length
  // A change of the timebase, a scale or the edge starts the average over.
  const key = [opts.edge, f.timeDiv, f.hOffset, c1.voltsPerDiv, out.voltsPerDiv, out.name].join('|')
  if (key !== historyKey) {
    history = []
    historyKey = key
  }
  const ss = steps(volts(c1), volts(out), opts.edge === 'up')
  if (ss.length) {
    history.push(ss)
    if (history.length > +opts.avg) history.splice(0, history.length - +opts.avg)
  }
  const s = average(history.flat())
  if (!s) {
    result.value = null
    return
  }
  const m = metrics(s, dt, +opts.band / 100)
  result.value = {
    s, m, dt, count: history.flat().length,
    counts: Math.abs(s.swing) / (out.voltsPerDiv / 25),
    clipped: c1.meas.clipped || out.meas.clipped, self: out === c1,
    square: m.inRise != null && m.inRise < 0.1 * (s.y.length - s.pre) * dt,
  }
}

watch(() => props.frame, compute)
watch(() => [opts.edge, opts.band, opts.avg], () => {
  historyKey = ''
  compute(props.frame)
})

// prepare sets up a square wave and a screen that shows one edge of it
// from 1.5 divisions on, and fits both channels to about six divisions.
function prepare() {
  const up = opts.edge === 'up'
  if (!gen.wave.toUpperCase().startsWith('SQU')) setGen('wave', 'SQUARE')
  if (!gen.output) setGen('output', true)
  const f = props.frame
  const c1 = f?.channels.find((c) => c.name === 'CH1')
  set('trig.source', 'CH1')
  set('trig.edge', up ? 'RISE' : 'FALL')
  const mid = c1?.meas.top != null && c1.meas.base != null ? (c1.meas.top + c1.meas.base) / 2 : gen.offset
  setTrigLevel(mid)
  // Half a period over about eight divisions.
  const half = 1 / (2 * gen.freq)
  const tb = TIMEBASES.find((t) => t.value * 8 >= half) ?? TIMEBASES[TIMEBASES.length - 1]
  set('hor.scale', tb.cmd)
  set('hor.offset', '4.5')
  for (const ch of f?.channels ?? []) {
    const k = ch.name.toLowerCase()
    if (k !== 'ch1' && k !== 'ch2') continue
    if (!ch.on) set(`${k}.display`, 'ON')
    const m = ch.meas
    if (m.pp == null || m.clipped) continue
    const scales = vdivOptions(probeFactor(ch.probe))
    const v = fitScale(ch.probe, m.pp)
    if (v.value !== scales[closest(scales, ch.voltsPerDiv)].value) set(`${k}.scale`, v.cmd)
    const centre = m.top != null && m.base != null ? (m.top + m.base) / 2 : (m.avg ?? 0)
    set(`${k}.offset`, Math.max(-8, Math.min(8, -centre / v.value)).toFixed(2))
  }
}

// ---- readouts ----

const band = computed(() => +opts.band / 100)
const notes = computed(() => {
  const r = result.value
  if (!r) return []
  const out: string[] = []
  if (!r.square) return ['На CH1 не меандр: перепад входа слишком пологий. Нажмите «Подготовить»']
  if (r.clipped) out.push('Сигнал выходит за экран: подберите масштаб')
  if (r.counts < 12) out.push('Перепад на выходе меньше полделения: уменьшите В/дел у CH2')
  if (Math.abs(r.s.drift) > band.value) out.push('Выход не успевает установиться за полпериода: уменьшите частоту генератора')
  if (r.m.rise != null && r.m.rise < 3 * r.dt) out.push('Фронт короче трёх отсчётов: уменьшите время/дел')
  else if (r.m.rise != null && r.m.inRise != null && r.m.rise < 1.5 * r.m.inRise && !r.self) {
    out.push('Фронт выхода почти как у генератора: точнее его не измерить')
  }
  return out
})

function pct(v: number) {
  return `${(v * 100).toFixed(v < 0.1 ? 1 : 0)} %`
}

// ---- drawing ----

const box = ref<HTMLDivElement>()
const canvas = ref<HTMLCanvasElement>()
const hover = ref<number | null>(null)
let colors: Record<string, string> = {}
let size = { w: 0, h: 0, dpr: 1 }
let raf = 0

function plotRect() {
  return { x: M.l, y: M.t, w: Math.max(10, size.w - M.l - M.r), h: Math.max(10, size.h - M.t - M.b) }
}

function schedule() {
  if (!raf) raf = requestAnimationFrame(() => { raf = 0; draw() })
}

// niceStep returns a 1-2-5 step that cuts span into about n parts.
function niceStep(span: number, n: number) {
  const raw = span / n, e = 10 ** Math.floor(Math.log10(raw))
  return [1, 2, 5, 10].map((m) => m * e).find((s) => s >= raw) ?? 10 * e
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
  g.strokeStyle = '#2b313d'
  g.strokeRect(p.x + 0.5, p.y + 0.5, p.w - 1, p.h - 1)
  const r = result.value
  if (!r) return

  const { s, m, dt } = r
  const n = s.y.length
  const t1 = (0 - m.t0) * dt, t2 = (n - 1 - m.t0) * dt
  const lo = Math.min(-0.1, ...s.y), hi = Math.max(1.2, ...s.y) + 0.05
  const xOf = (t: number) => p.x + ((t - t1) / (t2 - t1)) * p.w
  const yOf = (v: number) => p.y + ((hi - v) / (hi - lo)) * p.h
  const tOf = (i: number) => (i - m.t0) * dt

  // The settling band, the 0 and 100 % levels and the 10, 50 and 90 % lines.
  g.fillStyle = 'rgba(74,222,128,.07)'
  g.fillRect(p.x, yOf(1 + band.value), p.w, yOf(1 - band.value) - yOf(1 + band.value))
  g.lineWidth = 1
  g.strokeStyle = '#20252f'
  g.setLineDash([1, 3])
  g.beginPath()
  for (const v of [0.1, 0.5, 0.9]) line(g, p.x, yOf(v), p.x + p.w, yOf(v))
  const step = niceStep(t2 - t1, 8)
  for (let t = Math.ceil(t1 / step) * step; t <= t2; t += step) line(g, xOf(t), p.y, xOf(t), p.y + p.h)
  g.stroke()
  g.setLineDash([])
  g.strokeStyle = '#2f3542'
  g.beginPath()
  for (const v of [0, 1]) line(g, p.x, yOf(v), p.x + p.w, yOf(v))
  line(g, xOf(0), p.y, xOf(0), p.y + p.h)
  g.stroke()

  g.font = '11px Cascadia Mono, Consolas, monospace'
  g.fillStyle = '#5e6678'
  g.textAlign = 'right'
  g.textBaseline = 'middle'
  for (const v of [0, 0.5, 1]) g.fillText(`${v * 100} %`, p.x - 6, yOf(v))
  g.textAlign = 'center'
  g.textBaseline = 'top'
  let lastX = -1e9
  for (let t = Math.ceil(t1 / step) * step; t <= t2; t += step) {
    const x = xOf(t)
    if (x - lastX < 60) continue
    g.fillText(Math.abs(t) < step / 1e3 ? '0' : eng(t, 'с', 3, true), Math.min(p.x + p.w - 26, Math.max(p.x + 26, x)), p.y + p.h + 6)
    lastX = x
  }

  g.save()
  g.beginPath()
  g.rect(p.x, p.y, p.w, p.h)
  g.clip()
  const curve = (v: number[], color: string, width: number, alpha: number) => {
    g.globalAlpha = alpha
    g.strokeStyle = color
    g.lineWidth = width
    g.beginPath()
    v.forEach((y, i) => (i ? g.lineTo(xOf(tOf(i)), yOf(y)) : g.moveTo(xOf(tOf(i)), yOf(y))))
    g.stroke()
    g.globalAlpha = 1
  }
  if (!r.self) curve(s.x, colors.ch1, 1, 0.55)
  curve(s.y, r.self ? colors.ch1 : colors.ch2, 1.6, 1)

  // Settling time.
  if (m.settling != null) {
    const x = xOf(m.settling)
    g.strokeStyle = 'rgba(74,222,128,.6)'
    g.setLineDash([4, 3])
    g.beginPath()
    line(g, x, p.y, x, p.y + p.h)
    g.stroke()
    g.setLineDash([])
    g.font = '10px Segoe UI, sans-serif'
    g.fillStyle = '#4ade80'
    g.textAlign = 'left'
    g.textBaseline = 'top'
    g.fillText('уст.', x + 4, p.y + 4)
  }
  g.restore()

  if (hover.value != null) {
    const mx = hover.value
    if (mx < p.x || mx > p.x + p.w) return
    const t = t1 + ((mx - p.x) / p.w) * (t2 - t1)
    const i = Math.max(0, Math.min(n - 1, Math.round(t / dt + m.t0)))
    const x = xOf(tOf(i))
    g.strokeStyle = 'rgba(230,232,237,.35)'
    g.setLineDash([3, 3])
    g.beginPath()
    line(g, x, p.y, x, p.y + p.h)
    g.stroke()
    g.setLineDash([])
    const rows: [string, string][] = [[eng(tOf(i), 'с', 4), '#e6e8ed'], [`выход ${pct(s.y[i])}`, r.self ? colors.ch1 : colors.ch2]]
    if (!r.self) rows.push([`вход ${pct(s.x[i])}`, colors.ch1])
    g.font = '12px Cascadia Mono, Consolas, monospace'
    const w = 140, h = rows.length * 17 + 8
    const bx = x + 12 + w > p.x + p.w ? x - 12 - w : x + 12
    g.fillStyle = 'rgba(20,23,30,.92)'
    g.strokeStyle = '#323947'
    g.beginPath()
    g.roundRect(bx, p.y + 8, w, h, 6)
    g.fill()
    g.stroke()
    g.textAlign = 'right'
    g.textBaseline = 'middle'
    rows.forEach(([txt, color], k) => {
      g.fillStyle = color
      g.fillText(txt, bx + w - 9, p.y + 21 + k * 17)
    })
  }
}

function line(g: CanvasRenderingContext2D, x1: number, y1: number, x2: number, y2: number) {
  g.moveTo(Math.round(x1) + 0.5, Math.round(y1) + 0.5)
  g.lineTo(Math.round(x2) + 0.5, Math.round(y2) + 0.5)
}

function move(e: MouseEvent) {
  hover.value = e.clientX - canvas.value!.getBoundingClientRect().left
  schedule()
}

// ---- export ----

function exportCSV() {
  const r = result.value
  if (!r) return
  const f = (v: number | null) => (v == null ? '' : v.toPrecision(4))
  const lines = [
    `# OWON HDS272S step response, ${opts.edge === 'up' ? 'rising' : 'falling'} edge, ${r.count} steps averaged; 0 and 1 are the levels before and after`,
    `# delay ${f(r.m.delay)} s, rise ${f(r.m.rise)} s, overshoot ${f(r.m.overshoot)}, settling ±${opts.band}% ${f(r.m.settling)} s, gain ${f(r.s.gain)}`,
    't_s,input,output',
  ]
  r.s.y.forEach((y, i) => lines.push(`${((i - r.m.t0) * r.dt).toPrecision(6)},${r.s.x[i].toFixed(4)},${y.toFixed(4)}`))
  emit('save', `hds272s_step_${stamp()}.csv`, new Blob([lines.join('\n') + '\n'], { type: 'text/csv' }))
}

async function exportPNG() {
  const b = await new Promise<Blob | null>((res) => canvas.value!.toBlob(res, 'image/png'))
  if (b) emit('save', `hds272s_step_${stamp()}.png`, b)
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
  cancelAnimationFrame(raf)
})
watch([result, hover], schedule)
</script>

<template>
  <div class="step">
    <div class="bar">
      <span class="title" title="Переходная характеристика: отклик CH2 на перепад меандра на CH1">Переходная</span>
      <Seg v-model="opts.edge" :options="EDGES" />
      <span class="lbl">установление</span>
      <Seg v-model="opts.band" :options="BANDS" />
      <span class="lbl">усреднение</span>
      <Seg v-model="opts.avg" :options="AVGS" />
      <button
        :disabled="!props.online"
        title="Включить меандр на генераторе, синхронизацию по перепаду CH1, развёртку на полпериода и подобрать масштабы каналов"
        @click="prepare"
      >
        Подготовить
      </button>
      <span class="grow" />
      <button class="ghost" :disabled="!result" title="Сохранить кривую в CSV" @click="exportCSV">CSV</button>
      <button class="ghost" :disabled="!result" title="Сохранить картинку" @click="exportPNG">PNG</button>
    </div>
    <div ref="box" class="plot">
      <canvas ref="canvas" @mousemove="move" @mouseleave="hover = null; schedule()" />
      <div v-if="!result" class="empty">
        <p>Меандр с генератора подайте на вход цепи и на CH1, выход цепи — на CH2.</p>
        <p class="muted">Нужен хотя бы один перепад {{ opts.edge === 'up' ? 'вверх' : 'вниз' }} на CH1 с полупериодом перед ним. «Подготовить» включит меандр и настроит экран.</p>
      </div>
    </div>
    <div v-if="result" class="facts">
      <template v-if="result.square">
      <span class="mono kv" title="От 50 % перепада на входе до 50 % на выходе">Задержка {{ eng(result.m.delay, 'с', 3) }}</span>
      <span
        class="mono kv"
        :title="result.m.rise ? `Для цепи первого порядка полоса ≈ 0.35 / фронт = ${eng(0.35 / result.m.rise, 'Гц', 3)}` : undefined"
      >
        Фронт 10–90 % {{ eng(result.m.rise, 'с', 3) }}
      </span>
      <span v-if="result.m.overshoot > 4 * result.m.noise" class="mono kv">Выброс {{ pct(result.m.overshoot) }}</span>
      <span v-else class="mono kv" :title="`Пик выше конечного уровня на ${pct(result.m.overshoot)}, это в пределах шума`">Выброса нет</span>
      <span class="mono kv" :title="`От перепада на входе до последнего выхода из полосы ±${opts.band} %`">
        Установление {{ result.m.settling != null ? eng(result.m.settling, 'с', 3) : 'не уложилось' }}
      </span>
      <span v-if="result.m.ring" class="mono kv" title="Частота колебаний около установившегося уровня">Звон {{ eng(result.m.ring, 'Гц', 3) }}</span>
      <span v-if="!result.self" class="mono kv" title="Перепад на выходе к перепаду на входе">Усиление {{ result.s.gain.toFixed(3) }}</span>
      <span v-if="!result.self && result.m.inRise != null" class="mono dim" title="Фронт на входе, 10–90 %">вход {{ eng(result.m.inRise, 'с', 3) }}</span>
      <span class="mono dim">{{ result.count }} пер.</span>
      </template>
      <span v-for="(t, i) in notes" :key="i" class="warn">{{ t }}</span>
    </div>
  </div>
</template>

<style scoped>
.step { display: flex; flex-direction: column; gap: 6px; min-height: 0; }
.bar { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; font-size: 12px; }
.title { font-weight: 600; color: var(--muted); text-transform: uppercase; letter-spacing: .06em; font-size: 12px; margin-right: 4px; }
.lbl { color: var(--muted); margin-left: 4px; }
.grow { flex: 1; }
.plot { position: relative; flex: 1; min-height: 140px; }
canvas { position: absolute; inset: 0; width: 100%; height: 100%; cursor: crosshair; }
.empty {
  position: absolute;
  inset: 0;
  display: grid;
  place-content: center;
  text-align: center;
  font-size: 13px;
  line-height: 1.5;
  padding: 0 24px;
  pointer-events: none;
}
.empty p { margin: 2px 0; max-width: 560px; }
.facts { display: flex; align-items: center; gap: 14px; flex-wrap: wrap; font-size: 12px; min-height: 18px; }
.kv { color: var(--text); }
.warn { color: var(--warn); }
.dim { color: var(--dim); }
</style>
