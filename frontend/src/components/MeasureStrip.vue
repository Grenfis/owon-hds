<script setup lang="ts">
// Measurements of each channel and of CH2 against CH1. The set to show is
// chosen in a menu and kept between runs.
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { eng } from '../format'
import { mathName, mathStats, mathUnit } from '../math'
import { display, mathTrace } from '../state'
import type { Frame, FrameChannel, Measurements, Pair } from '../types'

const props = defineProps<{ frame: Frame | null }>()

interface Def {
  id: string
  label: string
  abbr: string
  title: string
  show: (m: Measurements, dt: number) => string
}

const volts = (k: keyof Measurements) => (m: Measurements) => eng(m[k] as number | null, 'В', 3)
const pct = (v: number | null) => (v == null ? '—' : `${v.toFixed(1)} %`)
// An edge shorter than two samples is only known to be that short.
const edge = (k: 'rise' | 'fall') => (m: Measurements, dt: number) => {
  const v = m[k]
  return v != null && v < 2 * dt ? `< ${eng(2 * dt, 'с', 3, true)}` : eng(v, 'с', 3)
}

const GROUPS: { name: string; defs: Def[] }[] = [
  {
    name: 'Напряжение',
    defs: [
      { id: 'pp', label: 'Размах', abbr: 'Vpp', title: 'От минимума до максимума, с выбросами и шумом', show: volts('pp') },
      { id: 'amp', label: 'Амплитуда', abbr: 'Vamp', title: 'От низа до верха: размах без выбросов. У синуса равна размаху', show: volts('amp') },
      { id: 'max', label: 'Максимум', abbr: 'Vmax', title: 'Самая высокая точка на экране', show: volts('max') },
      { id: 'min', label: 'Минимум', abbr: 'Vmin', title: 'Самая низкая точка на экране', show: volts('min') },
      { id: 'top', label: 'Верх', abbr: 'Vtop', title: 'Вершина импульса без выброса: самый частый уровень в верхней половине. У синуса равен максимуму', show: volts('top') },
      { id: 'base', label: 'Низ', abbr: 'Vbase', title: 'Основание импульса: самый частый уровень в нижней половине. У синуса равно минимуму', show: volts('base') },
      { id: 'overshoot', label: 'Выброс', abbr: '', title: 'На сколько максимум выше верха, в процентах амплитуды', show: (m) => pct(m.overshoot) },
      { id: 'preshoot', label: 'Предвыброс', abbr: '', title: 'На сколько минимум ниже низа, в процентах амплитуды', show: (m) => pct(m.preshoot) },
    ],
  },
  {
    name: 'Средние',
    defs: [
      { id: 'avg', label: 'Среднее', abbr: 'Vavg', title: 'Среднее по всему экрану', show: volts('avg') },
      { id: 'rms', label: 'СКЗ', abbr: 'Vrms', title: 'Среднеквадратичное по всему экрану, вместе с постоянной составляющей', show: volts('rms') },
      { id: 'cycAvg', label: 'Среднее пер.', abbr: 'Vavg', title: 'Среднее по целым периодам: часть периода на краю экрана не искажает результат', show: volts('cycAvg') },
      { id: 'cycRms', label: 'СКЗ пер.', abbr: 'Vrms', title: 'Среднеквадратичное по целым периодам, вместе с постоянной составляющей', show: volts('cycRms') },
      { id: 'acRms', label: 'СКЗ перем.', abbr: 'AC', title: 'Среднеквадратичное переменной составляющей по целым периодам: то, что покажет мультиметр в режиме ~V', show: volts('acRms') },
    ],
  },
  {
    name: 'Время',
    defs: [
      { id: 'freq', label: 'Частота', abbr: 'f', title: 'Частоту считает сам прибор', show: (m) => eng(m.freq, 'Гц', 5) },
      { id: 'period', label: 'Период', abbr: 'T', title: 'Период по частоте, которую считает прибор', show: (m) => eng(m.period, 'с', 4) },
      { id: 'rise', label: 'Фронт', abbr: 'tr', title: 'Время нарастания от 10 до 90 % амплитуды', show: edge('rise') },
      { id: 'fall', label: 'Спад', abbr: 'tf', title: 'Время спада от 90 до 10 % амплитуды', show: edge('fall') },
      { id: 'pWidth', label: 'Ширина +', abbr: '+W', title: 'Длительность импульса выше уровня 50 %', show: (m) => eng(m.pWidth, 'с', 4) },
      { id: 'nWidth', label: 'Ширина −', abbr: '−W', title: 'Длительность паузы ниже уровня 50 %', show: (m) => eng(m.nWidth, 'с', 4) },
      { id: 'duty', label: 'Заполн. +', abbr: '+D', title: 'Доля периода выше уровня 50 %', show: (m) => pct(m.duty) },
      { id: 'nDuty', label: 'Заполн. −', abbr: '−D', title: 'Доля периода ниже уровня 50 %', show: (m) => pct(m.duty == null ? null : 100 - m.duty) },
    ],
  },
]

interface PairDef {
  id: string
  label: string
  abbr: string
  title: string
  show: (p: Pair) => string
}

const PAIR: PairDef[] = [
  { id: 'gain', label: 'Усиление', abbr: 'K', title: 'Отношение амплитуд основной гармоники CH2 и CH1, в разах и в децибелах, как на графике АЧХ. Шум и постоянная составляющая на него почти не влияют', show: (p) => (p.gain == null ? '—' : `${p.gain.toPrecision(3)} · ${signedDb(p.gain)}`) },
  { id: 'phase', label: 'Фаза', abbr: 'φ', title: 'Сдвиг фазы основной гармоники CH2 относительно CH1. Минус: CH2 отстаёт, как на выходе ФНЧ', show: (p) => (p.phase == null ? '—' : `${p.phase.toFixed(1).replace('-', '−')}°`) },
  { id: 'delay', label: 'Задержка', abbr: 'Δt', title: 'Время от фронта CH1 до ближайшего фронта CH2 по уровню 50 %, в пределах полупериода', show: (p) => eng(p.delay, 'с', 4) },
]

function signedDb(k: number) {
  if (!(k > 0)) return '—'
  const d = 20 * Math.log10(k)
  return `${d >= 0 ? '+' : '−'}${Math.abs(d).toFixed(1)} дБ`
}

const ALL = GROUPS.flatMap((g) => g.defs)
const DEFAULT = ['freq', 'period', 'pp', 'max', 'min', 'avg', 'acRms', 'rise', 'duty', 'gain', 'phase']

const KEY = 'owon.meas'
function load(): string[] {
  try {
    const v = JSON.parse(localStorage.getItem(KEY) ?? 'null')
    if (Array.isArray(v)) return v
  } catch {}
  return DEFAULT
}
const chosen = ref<string[]>(load())
watch(chosen, (v) => {
  try {
    localStorage.setItem(KEY, JSON.stringify(v))
  } catch {}
})

function toggle(id: string) {
  chosen.value = chosen.value.includes(id) ? chosen.value.filter((x) => x !== id) : [...chosen.value, id]
}

function dtOf(f: Frame, c: FrameChannel) {
  return (f.timeDiv * 12) / (c.raw?.length || 600)
}

const rows = computed(() => {
  const f = props.frame
  if (!f) return []
  const defs = ALL.filter((d) => chosen.value.includes(d.id))
  return f.channels
    .filter((c) => c.on && c.raw?.length)
    .map((c) => ({
      name: c.name,
      clipped: c.meas.clipped,
      items: defs.map((d) => ({ ...d, value: d.show(c.meas, dtOf(f, c)) })),
    }))
})

// The math channel is measured here over the whole screen, with a fixed set.
const mathItems = computed(() => {
  const m = mathTrace.value
  if (!m) return null
  const s = mathStats(m.v), u = mathUnit(display.math)
  return {
    name: mathName(display.math),
    clipped: m.clipped,
    items: [
      { id: 'pp', label: 'Размах', value: eng(s.pp, u, 3) },
      { id: 'max', label: 'Максимум', value: eng(s.max, u, 3) },
      { id: 'min', label: 'Минимум', value: eng(s.min, u, 3) },
      { id: 'avg', label: 'Среднее', value: eng(s.avg, u, 3) },
      { id: 'rms', label: 'СКЗ', value: eng(s.rms, u, 3) },
    ],
  }
})

const pairItems = computed(() => {
  const p = props.frame?.pair
  if (!p) return []
  return PAIR.filter((d) => chosen.value.includes(d.id)).map((d) => ({ ...d, value: d.show(p) }))
})

// ---- menu ----

const open = ref(false)
const menu = ref<HTMLDivElement>()
const place = ref<Record<string, string>>({})

// The menu opens above its button, inside the window even when the strip
// is tall.
function toggleMenu(e: MouseEvent) {
  const b = (e.currentTarget as HTMLElement).getBoundingClientRect()
  place.value = {
    right: `${Math.max(8, window.innerWidth - b.right)}px`,
    bottom: `${window.innerHeight - b.top + 6}px`,
    maxHeight: `${Math.max(160, b.top - 14)}px`,
  }
  open.value = !open.value
}

// The menu button toggles the menu itself, so a press on it is not outside.
function outside(e: MouseEvent) {
  const t = e.target as Element
  if (!menu.value?.contains(t) && !t.closest?.('.cfg')) open.value = false
}
watch(open, (o) => (o ? window.addEventListener('mousedown', outside) : window.removeEventListener('mousedown', outside)))
onBeforeUnmount(() => window.removeEventListener('mousedown', outside))
</script>

<template>
  <div class="strip">
    <div v-for="(r, i) in rows" :key="r.name" class="row" :class="r.name.toLowerCase()">
      <span class="tag">{{ r.name }}</span>
      <div v-for="it in r.items" :key="it.id" class="item" :title="it.title">
        <span class="k">{{ it.label }} <span v-if="it.abbr" class="ab">{{ it.abbr }}</span></span>
        <span class="v mono">{{ it.value }}</span>
      </div>
      <span v-if="!r.items.length" class="muted hint">Выберите измерения в меню справа</span>
      <span class="end">
        <span v-if="r.clipped" class="clip" title="Часть сигнала за краем экрана: размах и амплитуда занижены. Увеличьте В/дел.">за экраном</span>
        <button v-if="i === 0" class="ghost cfg" :class="{ on: open }" title="Выбрать, какие измерения показывать" @click="toggleMenu">⚙</button>
      </span>
    </div>
    <div v-if="mathItems" class="row math">
      <span class="tag" :title="`Математика: ${mathItems.name}, по всему экрану`">M</span>
      <span class="mono fn">{{ mathItems.name }}</span>
      <div v-for="it in mathItems.items" :key="it.id" class="item">
        <span class="k">{{ it.label }}</span>
        <span class="v mono">{{ it.value }}</span>
      </div>
      <span class="end">
        <span v-if="mathItems.clipped" class="clip" title="Канал за краем экрана: и результат неверен. Увеличьте В/дел.">за экраном</span>
      </span>
    </div>
    <div v-if="pairItems.length" class="row pair">
      <span class="tag" title="CH2 относительно CH1">1→2</span>
      <div v-for="it in pairItems" :key="it.id" class="item" :title="it.title">
        <span class="k">{{ it.label }} <span class="ab">{{ it.abbr }}</span></span>
        <span class="v mono">{{ it.value }}</span>
      </div>
    </div>
    <div v-if="!rows.length" class="row empty muted">Измерения появятся, когда придут данные с прибора</div>

    <div v-if="open" ref="menu" class="menu" :style="place">
      <div class="cols">
        <div v-for="g in GROUPS" :key="g.name" class="col">
          <div class="gname">{{ g.name }}</div>
          <label v-for="d in g.defs" :key="d.id" :title="d.title">
            <input type="checkbox" :checked="chosen.includes(d.id)" @change="toggle(d.id)" />
            {{ d.label }} <span v-if="d.abbr" class="ab">{{ d.abbr }}</span>
          </label>
        </div>
        <div class="col">
          <div class="gname">CH2 относительно CH1</div>
          <label v-for="d in PAIR" :key="d.id" :title="d.title">
            <input type="checkbox" :checked="chosen.includes(d.id)" @change="toggle(d.id)" />
            {{ d.label }} <span class="ab">{{ d.abbr }}</span>
          </label>
          <p class="note muted">Строка «1→2» видна, когда включены оба канала.</p>
        </div>
      </div>
      <div class="actions">
        <button class="ghost" @click="chosen = [...DEFAULT]">По умолчанию</button>
        <button class="ghost" @click="chosen = [...ALL.map((d) => d.id), ...PAIR.map((d) => d.id)]">Все</button>
        <button class="ghost" @click="chosen = []">Ничего</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.strip { display: flex; flex-direction: column; gap: 4px; position: relative; }
.row {
  display: flex;
  align-items: center;
  gap: 4px 16px;
  flex-wrap: wrap;
  padding: 6px 10px;
  background: var(--panel);
  border: 1px solid var(--line);
  border-left: 3px solid var(--c, var(--line));
  border-radius: 6px;
  min-height: 38px;
}
.row.ch1 { --c: var(--ch1); }
.row.ch2 { --c: var(--ch2); }
.row.pair { --c: var(--accent); }
.row.math { --c: var(--math); }
.fn { font-size: 12px; color: var(--muted); min-width: 74px; }
.tag { font-weight: 600; color: var(--c); width: 30px; white-space: nowrap; }
.item { display: flex; flex-direction: column; min-width: 74px; }
.k { font-size: 11px; color: var(--muted); white-space: nowrap; }
.ab { color: var(--dim); font-size: 10px; }
.v { font-size: 13px; white-space: nowrap; }
.hint { font-size: 12px; }
.end { margin-left: auto; display: flex; align-items: center; gap: 8px; }
.clip {
  font-size: 11px;
  color: var(--warn);
  background: var(--warn-bg);
  border-radius: 4px;
  padding: 2px 7px;
}
.cfg { font-size: 16px; line-height: 1; padding: 4px 6px; }
.cfg.on { background: var(--panel-3); color: var(--text); }
.empty { font-size: 12px; }
.menu {
  position: fixed;
  z-index: 20;
  overflow-y: auto;
  background: var(--panel-2);
  border: 1px solid var(--line-2);
  border-radius: 10px;
  box-shadow: 0 12px 32px rgba(0, 0, 0, .45);
  padding: 12px 14px 8px;
}
.cols { display: flex; gap: 22px; }
.col { display: flex; flex-direction: column; gap: 3px; min-width: 130px; }
.gname { font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: .06em; color: var(--muted); margin-bottom: 4px; }
.col label { display: flex; align-items: center; gap: 7px; font-size: 13px; cursor: pointer; padding: 2px 0; white-space: nowrap; }
.note { font-size: 11px; max-width: 170px; margin: 6px 0 0; line-height: 1.35; }
.actions { display: flex; gap: 4px; justify-content: flex-end; border-top: 1px solid var(--line); margin-top: 10px; padding-top: 6px; }
.actions button { font-size: 12px; }
</style>
