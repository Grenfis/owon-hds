<script setup lang="ts">
// Component tester: the generator drives the part through a reference
// resistor, CH1 on the generator, CH2 on the part. The panel plots the
// current through the part against the voltage across it and names the
// part from the curve.
import { computed, onBeforeUnmount, onMounted, reactive, ref, shallowRef, watch } from 'vue'
import { analyze, type CurveReading } from '../curve'
import { TIMEBASES, closest, eng, fitScale, parseInput, probeFactor, stamp, vdivOptions } from '../format'
import { gen, set, setGen, setTrigLevel } from '../state'
import type { Frame } from '../types'
import Seg from './Seg.vue'

const props = defineProps<{ frame: Frame | null; online: boolean }>()
const emit = defineEmits<{ save: [name: string, blob: Blob] }>()

const M = { l: 64, r: 14, t: 10, b: 24 }

const SETTINGS_KEY = 'owon.curve'
const opts = reactive({ rRef: '1k', avg: '4' })
try {
  Object.assign(opts, JSON.parse(localStorage.getItem(SETTINGS_KEY) ?? '{}'))
} catch {}
watch(opts, () => localStorage.setItem(SETTINGS_KEY, JSON.stringify(opts)))

const AVGS = [
  { value: '1', label: '1', title: 'Только этот кадр' },
  { value: '4', label: '4', title: 'Среднее по 4 кадрам' },
  { value: '16', label: '16', title: 'Среднее по 16 кадрам: меньше шума' },
]

const rRef = computed(() => parseInput(opts.rRef))
const rOk = computed(() => rRef.value != null && rRef.value > 0)

interface Result {
  v: number[] // across the part
  i: number[] // through it
  vg: number // the generator's peak
  rd: CurveReading
  f: number
  count: number // frames averaged
  clipped: boolean
}

const result = shallowRef<Result | null>(null)
let history: [number[], number[]][] = []
let historyKey = ''

function compute(fr: Frame | null) {
  const c1 = fr?.channels.find((c) => c.name === 'CH1')
  const c2 = fr?.channels.find((c) => c.name === 'CH2')
  if (!fr || !c1?.on || !c2?.on || !c1.raw?.length || c2.raw?.length !== c1.raw.length || !(fr.timeDiv > 0) || !rOk.value) {
    result.value = null
    return
  }
  const f = c1.meas.freq ?? gen.freq ?? 0
  // The frames line up while the trigger, the screen and the frequency stay.
  const key = [fr.timeDiv, fr.hOffset, c1.voltsPerDiv, c2.voltsPerDiv, c1.offset, c2.offset, Math.round(Math.log(f || 1) / 0.02)].join('|')
  if (key !== historyKey) {
    history = []
    historyKey = key
  }
  const volts = (c: typeof c1) => c.raw!.map((r) => ((r - c.offset) / 25) * c.voltsPerDiv)
  history.push([volts(c1), volts(c2)])
  if (history.length > +opts.avg) history.splice(0, history.length - +opts.avg)
  const mean = (k: 0 | 1) => history[0][k].map((_, j) => history.reduce((a, h) => a + h[k][j], 0) / history.length)
  const ch1 = mean(0), ch2 = mean(1)
  const dt = (fr.timeDiv * 12) / ch1.length
  const r = rRef.value!
  const rd = analyze(ch1, ch2, r, f, dt, c1.voltsPerDiv / 25, c2.voltsPerDiv / 25)
  result.value = {
    v: ch2,
    i: ch1.map((x, k) => (x - ch2[k]) / r),
    vg: Math.max(...ch1.map(Math.abs)),
    rd, f, count: history.length,
    clipped: c1.meas.clipped || c2.meas.clipped,
  }
}
watch(() => props.frame, compute)
watch(() => [opts.avg, opts.rRef], () => {
  historyKey = ''
  compute(props.frame)
})

// ---- readouts ----

const NAMES: Record<CurveReading['kind'], string> = {
  open: 'Обрыв',
  short: 'Замыкание',
  resistor: 'Резистор',
  capacitor: 'Конденсатор',
  inductor: 'Катушка',
  diode: 'Диод',
  zener: 'Стабилитрон',
  twoway: 'Встречные диоды',
  other: 'Нелинейная деталь',
}

const main = computed(() => {
  const rd = result.value?.rd
  if (!rd) return ''
  if (rd.kind === 'resistor') return `R = ${eng(rd.r, 'Ом', 3)}`
  if (rd.kind === 'capacitor') return `C = ${eng(rd.c, 'Ф', 3)}`
  if (rd.kind === 'inductor') return `L = ${eng(rd.l, 'Гн', 3)}`
  return NAMES[rd.kind]
})

const facts = computed(() => {
  const rd = result.value?.rd
  if (!rd) return []
  const v = (x: number) => `${x.toFixed(2)} В`
  const at = `при ${eng(rd.ith, 'А', 2)}`
  const side = rd.anodeUp ? 'анод к точке CH2' : 'катод к точке CH2'
  switch (rd.kind) {
    case 'open':
      return [`ток меньше ${eng(rd.ith, 'А', 2)}`, `сопротивление больше ≈ ${eng(rd.vMax / rd.ith, 'Ом', 2)}`]
    case 'short':
      return [`напряжение меньше ${eng(rd.vth, 'В', 2)}`, `сопротивление меньше ≈ ${eng(rd.vth / rd.iMax, 'Ом', 2)}`]
    case 'resistor':
      return []
    case 'capacitor':
      return rd.r != null ? [`утечка ≈ ${eng(rd.r, 'Ом', 2)}`] : []
    case 'inductor':
      return [`R ≈ ${eng(rd.r, 'Ом', 2)}`]
    case 'diode':
      return [side, `Uпр ${v(rd.fwd!)} ${at}`]
    case 'zener':
      return [side, `Uпр ${v(rd.fwd!)}`, `Uст ${v(Math.abs(rd.rev!))} ${at}`]
    case 'twoway':
      return [`открываются при +${v(rd.fwd!)} и −${v(Math.abs(rd.rev!))} ${at}`]
    default:
      return []
  }
})

const notes = computed(() => {
  const r = result.value
  if (!r) return []
  const out: string[] = []
  if (r.clipped) out.push('Сигнал выходит за экран: нажмите «Подготовить»')
  if (r.vg < 1 && ['open', 'other', 'diode'].includes(r.rd.kind)) {
    out.push('Генератор даёт меньше ±1 В: кремниевый диод может не открыться. «Подготовить» ставит размах 5 В')
  }
  const rd = r.rd
  if ((rd.kind === 'capacitor' || rd.kind === 'inductor') && target.value != null) {
    const x = rd.kind === 'capacitor' ? 1 / (2 * Math.PI * r.f * rd.c!) : 2 * Math.PI * r.f * rd.l!
    const q = x / rRef.value!
    if (q > 10 || q < 0.1) out.push(`Сопротивление детали в ${(q > 1 ? q : 1 / q).toFixed(0)} раз ${q > 1 ? 'больше' : 'меньше'} эталона: нажмите «Подобрать частоту»`)
  }
  return out
})

// ---- actions ----

// prepare sets a sine of 5 V peak to peak without offset, the trigger on
// CH1 at 0 V, two to five periods on the screen and the same scale on both
// channels that fits the generator.
function prepare() {
  if (!gen.wave.toUpperCase().startsWith('SIN')) setGen('wave', 'SINE')
  if (gen.offset !== 0) setGen('offset', 0)
  if (gen.amp !== 5) setGen('amp', 5)
  if (!gen.output) setGen('output', true)
  set('trig.source', 'CH1')
  set('trig.edge', 'RISE')
  setTrigLevel(0)
  setTimebase(gen.freq)
  const fr = props.frame
  for (const ch of fr?.channels ?? []) {
    const k = ch.name.toLowerCase()
    if (k !== 'ch1' && k !== 'ch2') continue
    if (!ch.on) set(`${k}.display`, 'ON')
    const scales = vdivOptions(probeFactor(ch.probe))
    const v = fitScale(ch.probe, 5)
    if (v.value !== scales[closest(scales, ch.voltsPerDiv)].value) set(`${k}.scale`, v.cmd)
    set(`${k}.offset`, '0.00')
  }
}

function setTimebase(f: number) {
  const tb = TIMEBASES.find((t) => f * 12 * t.value >= 2) ?? TIMEBASES[TIMEBASES.length - 1]
  set('hor.scale', tb.cmd)
}

// The frequency at which a capacitor or an inductor has the reactance of
// the reference: there the curve is a round ellipse and reads best.
const target = computed(() => {
  const rd = result.value?.rd
  if (!rd || !rOk.value) return null
  const f = rd.kind === 'capacitor' ? 1 / (2 * Math.PI * rRef.value! * rd.c!) : rd.kind === 'inductor' ? rRef.value! / (2 * Math.PI * rd.l!) : null
  if (f == null || !Number.isFinite(f) || f <= 0) return null
  return +Math.min(1e6, Math.max(10, f)).toPrecision(3)
})

function tune() {
  const f = target.value
  if (f == null) return
  setGen('freq', f)
  setTimebase(f)
}

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

function schedule() {
  if (!raf) raf = requestAnimationFrame(() => { raf = 0; draw() })
}

// niceStep returns a 1-2-5 step that cuts span into about n parts.
function niceStep(span: number, n: number) {
  const raw = span / n, e = 10 ** Math.floor(Math.log10(raw))
  return [1, 2, 5, 10].map((m) => m * e).find((s) => s >= raw) ?? 10 * e
}

// The axes span the generator's peak and the current it drives into a
// short: an open part lies along the voltage axis, a short along the
// current axis and a part equal to the reference on the diagonal.
function axes(r: Result) {
  const vr = Math.max(0.1, r.vg) * 1.08
  return { vr, ir: vr / rRef.value! }
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

  const { vr, ir } = axes(r)
  const xOf = (v: number) => p.x + ((v + vr) / (2 * vr)) * p.w
  const yOf = (i: number) => p.y + ((ir - i) / (2 * ir)) * p.h
  const vs = niceStep(2 * vr, 8), is = niceStep(2 * ir, 6)

  g.lineWidth = 1
  g.strokeStyle = '#20252f'
  g.setLineDash([1, 3])
  g.beginPath()
  for (let v = Math.ceil(-vr / vs) * vs; v <= vr; v += vs) line(g, xOf(v), p.y, xOf(v), p.y + p.h)
  for (let i = Math.ceil(-ir / is) * is; i <= ir; i += is) line(g, p.x, yOf(i), p.x + p.w, yOf(i))
  g.stroke()
  g.setLineDash([])
  g.strokeStyle = '#2f3542'
  g.beginPath()
  line(g, xOf(0), p.y, xOf(0), p.y + p.h)
  line(g, p.x, yOf(0), p.x + p.w, yOf(0))
  g.stroke()

  g.font = '11px Cascadia Mono, Consolas, monospace'
  g.fillStyle = '#5e6678'
  g.textAlign = 'right'
  g.textBaseline = 'middle'
  for (let i = Math.ceil(-ir / is) * is; i <= ir; i += is) {
    g.fillText(Math.abs(i) < is / 1e3 ? '0' : eng(i, 'А', 3, true), p.x - 6, yOf(i))
  }
  g.textAlign = 'center'
  g.textBaseline = 'top'
  let lastX = -1e9
  for (let v = Math.ceil(-vr / vs) * vs; v <= vr; v += vs) {
    const x = xOf(v)
    if (x - lastX < 50) continue
    g.fillText(Math.abs(v) < vs / 1e3 ? '0' : eng(v, 'В', 3, true), Math.min(p.x + p.w - 20, Math.max(p.x + 20, x)), p.y + p.h + 6)
    lastX = x
  }

  g.save()
  g.beginPath()
  g.rect(p.x, p.y, p.w, p.h)
  g.clip()
  g.strokeStyle = colors.ch2
  g.lineWidth = 1.6
  g.lineJoin = 'round'
  g.beginPath()
  r.v.forEach((v, k) => (k ? g.lineTo(xOf(v), yOf(r.i[k])) : g.moveTo(xOf(v), yOf(r.i[k]))))
  g.stroke()
  g.restore()

  // The point of the curve nearest to the mouse.
  const h = hover.value
  if (!h) return
  let best = -1, bd = 30 * 30
  r.v.forEach((v, k) => {
    const d = (xOf(v) - h.x) ** 2 + (yOf(r.i[k]) - h.y) ** 2
    if (d < bd) [best, bd] = [k, d]
  })
  if (best < 0) return
  const x = xOf(r.v[best]), y = yOf(r.i[best])
  g.fillStyle = '#e6e8ed'
  g.beginPath()
  g.arc(x, y, 3.5, 0, 2 * Math.PI)
  g.fill()
  const rows = [eng(r.v[best], 'В', 3), eng(r.i[best], 'А', 3)]
  g.font = '12px Cascadia Mono, Consolas, monospace'
  const w = 104, bh = rows.length * 17 + 8
  const bx = x + 12 + w > p.x + p.w ? x - 12 - w : x + 12
  const by = Math.min(p.y + p.h - bh - 4, Math.max(p.y + 4, y - bh / 2))
  g.fillStyle = 'rgba(20,23,30,.92)'
  g.strokeStyle = '#323947'
  g.beginPath()
  g.roundRect(bx, by, w, bh, 6)
  g.fill()
  g.stroke()
  g.fillStyle = '#e6e8ed'
  g.textAlign = 'right'
  g.textBaseline = 'middle'
  rows.forEach((t, k) => g.fillText(t, bx + w - 9, by + 13 + k * 17))
}

function line(g: CanvasRenderingContext2D, x1: number, y1: number, x2: number, y2: number) {
  g.moveTo(Math.round(x1) + 0.5, Math.round(y1) + 0.5)
  g.lineTo(Math.round(x2) + 0.5, Math.round(y2) + 0.5)
}

function move(e: MouseEvent) {
  const b = canvas.value!.getBoundingClientRect()
  hover.value = { x: e.clientX - b.left, y: e.clientY - b.top }
  schedule()
}

// ---- export ----

function exportCSV() {
  const r = result.value
  if (!r) return
  const lines = [
    `# OWON HDS272S current-voltage curve through ${opts.rRef} ohm at ${eng(r.f, 'Hz', 4)}, ${r.count} frames averaged; ${NAMES[r.rd.kind]} ${main.value}`,
    'v_V,i_A',
  ]
  r.v.forEach((v, k) => lines.push(`${v.toFixed(4)},${r.i[k].toPrecision(4)}`))
  emit('save', `hds272s_curve_${stamp()}.csv`, new Blob([lines.join('\n') + '\n'], { type: 'text/csv' }))
}

async function exportPNG() {
  const b = await new Promise<Blob | null>((res) => canvas.value!.toBlob(res, 'image/png'))
  if (b) emit('save', `hds272s_curve_${stamp()}.png`, b)
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
  <div class="curve">
    <div class="bar">
      <span class="title" title="Тестер компонентов: вольт-амперная характеристика детали">ВАХ</span>
      <span class="lbl">R эталон</span>
      <input v-model="opts.rRef" type="text" class="num" :class="{ bad: !rOk }" title="Сопротивление эталонного резистора: 100, 1k, 10k" />
      <span class="lbl">Ом</span>
      <span class="lbl">усреднение</span>
      <Seg v-model="opts.avg" :options="AVGS" />
      <button
        :disabled="!props.online"
        title="Синус 5 В размахом без смещения, синхронизация по CH1, 2–5 периодов на экране, одинаковый масштаб каналов"
        @click="prepare"
      >
        Подготовить
      </button>
      <button
        v-if="target != null"
        :disabled="!props.online"
        :title="`Поставить ${eng(target, 'Гц', 3)}: на этой частоте сопротивление детали равно эталону`"
        @click="tune"
      >
        Подобрать частоту
      </button>
      <span class="grow" />
      <button class="ghost" :disabled="!result" title="Сохранить кривую в CSV" @click="exportCSV">CSV</button>
      <button class="ghost" :disabled="!result" title="Сохранить картинку" @click="exportPNG">PNG</button>
    </div>
    <div ref="box" class="plot">
      <canvas ref="canvas" @mousemove="move" @mouseleave="hover = null; schedule()" />
      <div v-if="!result" class="empty">
        <p>Генератор → R эталон → деталь → земля. CH1 на выходе генератора, CH2 на детали.</p>
        <p class="muted">Нужны оба канала. «Подготовить» включит синус и настроит экран.</p>
      </div>
    </div>
    <div v-if="result" class="facts">
      <span class="main mono">{{ main }}</span>
      <span v-for="(t, i) in facts" :key="i" class="mono kv">{{ t }}</span>
      <span class="mono dim">на {{ eng(result.f, 'Гц', 3) }}</span>
      <span v-for="(t, i) in notes" :key="'n' + i" class="warn">{{ t }}</span>
    </div>
  </div>
</template>

<style scoped>
.curve { display: flex; flex-direction: column; gap: 6px; }
.bar { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; font-size: 12px; }
.title { font-weight: 600; color: var(--muted); text-transform: uppercase; letter-spacing: .06em; font-size: 12px; margin-right: 4px; }
.lbl { color: var(--muted); }
.grow { flex: 1; }
.num { width: 58px; font-size: 12px; padding: 3px 6px; font-family: var(--mono); }
.num.bad { border-color: var(--err); }
.plot { position: relative; flex: 1; min-height: 120px; }
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
.facts { display: flex; align-items: baseline; gap: 14px; flex-wrap: wrap; font-size: 12px; }
.main { font-size: 20px; font-weight: 600; margin-right: 4px; }
.kv { color: var(--text); }
.warn { color: var(--warn); }
.dim { color: var(--dim); }
</style>
