<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { on } from '../api'
import { eng } from '../format'
import { set, status } from '../state'

defineProps<{ disabled: boolean }>()

const raw = ref('')
const at = ref(0)
const history = ref<number[]>([])
const mode = ref(localStorage.getItem('owon.dmmMode') ?? 'dcv')
const rel = ref(false)

const MODES = [
  { id: 'dcv', sym: 'V⎓', label: 'Пост. напряжение', key: 'dmm.voltage', value: 'DC', unit: 'В' },
  { id: 'acv', sym: 'V~', label: 'Перем. напряжение', key: 'dmm.voltage', value: 'AC', unit: 'В' },
  { id: 'dca', sym: 'A⎓', label: 'Пост. ток', key: 'dmm.current', value: 'DC', unit: 'А' },
  { id: 'aca', sym: 'A~', label: 'Перем. ток', key: 'dmm.current', value: 'AC', unit: 'А' },
  { id: 'res', sym: 'Ω', label: 'Сопротивление', key: 'dmm.func', value: 'RESistance', unit: 'Ом' },
  { id: 'diod', sym: '⊳|', label: 'Диод', key: 'dmm.func', value: 'DIODe', unit: 'В' },
  { id: 'cont', sym: '•))', label: 'Прозвонка', key: 'dmm.func', value: 'CONTinuity', unit: 'Ом' },
  { id: 'cap', sym: '⊣⊢', label: 'Ёмкость', key: 'dmm.func', value: 'CAPacitance', unit: 'Ф' },
]

function pick(m: (typeof MODES)[number]) {
  mode.value = m.id
  localStorage.setItem('owon.dmmMode', m.id)
  history.value = []
  set(m.key, m.value)
}

// :DMM:MEAS? replies with a bare number in base units, "1.2345e+00", so the
// unit comes from the mode picked here. A mode changed on the device itself
// is not known to the program.
const unit = computed(() => MODES.find((m) => m.id === mode.value)?.unit ?? '')
const value = computed(() => {
  const v = parseFloat(raw.value)
  return Number.isFinite(v) ? v : null
})
const shown = computed(() => {
  if (value.value == null) return { num: raw.value || '—', unit: '' }
  const s = eng(value.value, unit.value, 5)
  const i = s.lastIndexOf(' ')
  return { num: s.slice(0, i), unit: s.slice(i + 1) }
})

const spark = computed(() => {
  const h = history.value
  if (h.length < 2) return ''
  const lo = Math.min(...h), hi = Math.max(...h)
  const span = hi - lo || 1
  return h.map((v, i) => `${(i / (h.length - 1)) * 600},${58 - ((v - lo) / span) * 52}`).join(' ')
})
const range = computed(() => (history.value.length ? [Math.min(...history.value), Math.max(...history.value)] : null))

let off: (() => void) | undefined
onMounted(() => {
  off = on('dmm', (d: { raw: string; time: number }) => {
    raw.value = d.raw
    at.value = d.time
    const v = value.value
    if (v != null) {
      history.value.push(v)
      if (history.value.length > 150) history.value.shift()
    }
  })
})
onBeforeUnmount(() => off?.())
</script>

<template>
  <div class="dmm">
    <div class="note">
      Переключите прибор на экран мультиметра кнопкой <b>Mode</b>: по USB экран не переключается.
      Прибор присылает только число, без единицы. Единица берётся из режима, выбранного справа, поэтому выберите тот же режим, что на приборе.
    </div>

    <div class="main">
      <div class="readout" :class="{ stale: !status.connected }">
        <div class="value mono">{{ shown.num }}</div>
        <div class="unit">{{ shown.unit }}</div>
        <div class="raw mono">ответ прибора: {{ raw || 'нет данных' }}</div>
        <svg v-if="spark" class="spark" viewBox="0 0 600 60" preserveAspectRatio="none">
          <polyline :points="spark" />
        </svg>
        <div v-if="range" class="range mono dim">мин {{ eng(range[0], unit, 5) }} · макс {{ eng(range[1], unit, 5) }} · {{ history.length }} отсчётов</div>
      </div>

      <div class="side">
        <div class="modes">
          <button
            v-for="m in MODES"
            :key="m.id"
            class="mode"
            :class="{ on: mode === m.id }"
            :disabled="disabled"
            @click="pick(m)"
          >
            <span class="sym">{{ m.sym }}</span>
            <span class="lbl">{{ m.label }}</span>
          </button>
        </div>
        <div class="actions">
          <button :disabled="disabled" @click="set('dmm.auto', '')">Авто диапазон</button>
          <button :disabled="disabled" @click="set('dmm.range', '')">Следующий диапазон</button>
          <button :class="{ primary: rel }" :disabled="disabled" @click="rel = !rel; set('dmm.rel', rel ? 'ON' : 'OFF')">
            Относительный (REL)
          </button>
          <button class="ghost" :disabled="!history.length" @click="history = []">Сбросить график</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.dmm { display: flex; flex-direction: column; gap: 12px; padding: 14px; height: 100%; overflow: auto; }
.note {
  font-size: 12px;
  color: var(--muted);
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: var(--radius);
  padding: 8px 12px;
}
.main { display: flex; gap: 14px; align-items: flex-start; flex-wrap: wrap; }
.readout {
  flex: 1 1 520px;
  background: #090b0f;
  border: 1px solid var(--line);
  border-radius: 12px;
  padding: 24px 28px 18px;
  display: grid;
  grid-template-columns: auto 1fr;
  align-items: baseline;
  column-gap: 16px;
}
.readout.stale { opacity: .55; }
.value { font-size: 88px; line-height: 1; color: var(--ok); letter-spacing: -.02em; }
.unit { font-size: 34px; color: var(--muted); }
.raw { grid-column: 1 / -1; font-size: 12px; color: var(--dim); margin-top: 10px; user-select: text; }
.spark { grid-column: 1 / -1; width: 100%; height: 60px; margin-top: 14px; }
.spark polyline { fill: none; stroke: var(--ok); stroke-width: 1.5; vector-effect: non-scaling-stroke; }
.range { grid-column: 1 / -1; font-size: 11px; margin-top: 4px; }
.side { flex: 0 1 380px; display: flex; flex-direction: column; gap: 12px; }
.modes { display: grid; grid-template-columns: repeat(4, 1fr); gap: 6px; }
.mode { display: flex; flex-direction: column; align-items: center; gap: 4px; padding: 10px 4px; }
.mode .sym { font-size: 18px; font-family: var(--mono); }
.mode .lbl { font-size: 11px; color: var(--muted); text-align: center; }
.mode.on { background: var(--panel-3); border-color: #4a5366; }
.mode.on .sym { color: var(--ok); }
.actions { display: flex; flex-wrap: wrap; gap: 6px; }
</style>
