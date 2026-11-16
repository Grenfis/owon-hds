<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { TIMEBASES, eng } from '../format'
import { gen, readGen, set, setGen } from '../state'
import DigitKnob from './DigitKnob.vue'

const props = defineProps<{ disabled: boolean }>()

const BASIC = [
  { value: 'SINE', label: 'Синус', path: 'M1 7 C4 -1 8 -1 11 7 S18 15 21 7' },
  { value: 'SQUARE', label: 'Меандр', path: 'M1 11 H6 V3 H13 V11 H20' },
  { value: 'RAMP', label: 'Пила', path: 'M1 11 L11 3 L21 11' },
  { value: 'PULSE', label: 'Импульс', path: 'M1 11 H5 V3 H8 V11 H14 V3 H17 V11 H21' },
]
const OTHER = [
  { value: 'AMPALT', label: 'AmpALT — нарастающая амплитуда' },
  { value: 'ATTALT', label: 'AttALT — затухающая амплитуда' },
  { value: 'STAIRDN', label: 'Ступени вниз' },
  { value: 'STAIRUP', label: 'Ступени вверх' },
  { value: 'STAIRUD', label: 'Ступени вверх-вниз' },
  { value: 'BESSELJ', label: 'Функция Бесселя J' },
  { value: 'BESSELY', label: 'Функция Бесселя Y' },
  { value: 'SINC', label: 'Sinc' },
]
const PRESETS = [50, 1e3, 10e3, 100e3, 1e6]

const wave = computed(() => gen.wave.toUpperCase())
const isOther = computed(() => !BASIC.some((b) => wave.value.startsWith(b.value.slice(0, 4))))
const waveIs = (w: string) => wave.value.startsWith(w.slice(0, 4))

// Frequency limits from the HDS272S specification.
const maxFreq = computed(() => (waveIs('SINE') ? 25e6 : waveIs('RAMP') ? 1e6 : 5e6))
const swingOk = computed(() => Math.abs(gen.offset) + gen.amp / 2 <= 2.5 + 1e-9)

function setWave(w: string) {
  setGen('wave', w)
  if (gen.freq > maxFreq.value) setGen('freq', maxFreq.value)
}

// Follow the frequency with the timebase, so that two to five periods stay
// on screen while the frequency knob turns.
const followTime = ref(localStorage.getItem('owon.followTime') === '1')
watch(followTime, (v) => localStorage.setItem('owon.followTime', v ? '1' : '0'))
let followTimer = 0
watch(
  () => gen.freq,
  (f) => {
    if (!followTime.value || props.disabled || !(f > 0)) return
    clearTimeout(followTimer)
    followTimer = window.setTimeout(() => {
      const want = 2.5 / (f * 12)
      const opt = TIMEBASES.find((o) => o.value >= want * 0.999) ?? TIMEBASES[TIMEBASES.length - 1]
      set('hor.scale', opt.cmd)
    }, 120)
  },
)

const reading = ref(false)
async function reread() {
  reading.value = true
  try {
    await readGen()
  } finally {
    reading.value = false
  }
}
</script>

<template>
  <section class="gen" :class="{ disabled }">
    <div class="left">
      <div class="title">
        <h3>Генератор</h3>
        <button
          class="output"
          :class="{ on: gen.output }"
          :disabled="disabled"
          :title="gen.output ? 'Выключить выход GEN Out' : 'Включить выход GEN Out'"
          @click="setGen('output', !gen.output)"
        >
          <span class="led" />
          Выход {{ gen.output ? 'включён' : 'выключен' }}
        </button>
      </div>
      <div class="waves">
        <button
          v-for="b in BASIC"
          :key="b.value"
          class="wave"
          :class="{ on: waveIs(b.value) }"
          :disabled="disabled"
          @click="setWave(b.value)"
        >
          <svg width="22" height="14" viewBox="0 0 22 14"><path :d="b.path" /></svg>
          {{ b.label }}
        </button>
        <select
          class="other"
          :class="{ on: isOther }"
          :value="isOther ? wave : ''"
          :disabled="disabled"
          @change="setWave(($event.target as HTMLSelectElement).value)"
        >
          <option value="" disabled>Другие формы…</option>
          <option v-for="o in OTHER" :key="o.value" :value="o.value">{{ o.label }}</option>
        </select>
      </div>
      <div class="opts">
        <label title="При смене частоты подбирать время/дел так, чтобы на экране было 2–5 периодов">
          <input v-model="followTime" type="checkbox" :disabled="disabled" />
          Развёртка за частотой
        </label>
        <button class="ghost" :disabled="disabled || reading" title="Прочитать настройки генератора с прибора" @click="reread">
          {{ reading ? 'Читаю…' : '↻ С прибора' }}
        </button>
      </div>
    </div>

    <div class="knobs">
      <div class="freq">
        <DigitKnob
          label="Частота"
          unit="Гц"
          :model-value="gen.freq"
          :min="0.1"
          :max="maxFreq"
          :int-digits="8"
          :frac-digits="1"
          :start-digit="2"
          :hint="`${eng(gen.freq, 'Гц', 6)} · T ${eng(1 / gen.freq, 'с', 4)}`"
          color="var(--text)"
          :disabled="disabled"
          @update:model-value="setGen('freq', $event)"
        />
        <div class="presets">
          <button v-for="p in PRESETS" :key="p" class="ghost" :disabled="disabled || p > maxFreq" @click="setGen('freq', p)">
            {{ eng(p, 'Гц', 3, true) }}
          </button>
        </div>
      </div>
      <DigitKnob
        label="Амплитуда"
        unit="Vpp"
        :model-value="gen.amp"
        :min="0.01"
        :max="5"
        :int-digits="1"
        :frac-digits="3"
        :start-digit="-1"
        :hint="waveIs('SINE') ? `СКЗ ${eng(gen.amp / 2 / Math.SQRT2, 'В', 3)}` : `±${eng(gen.amp / 2, 'В', 3)}`"
        :disabled="disabled"
        @update:model-value="setGen('amp', $event)"
      />
      <DigitKnob
        label="Смещение"
        unit="В"
        :model-value="gen.offset"
        :min="-2.5"
        :max="2.5"
        :int-digits="1"
        :frac-digits="3"
        :start-digit="-1"
        :hint="swingOk ? '' : 'выше ±2.5 В'"
        :disabled="disabled"
        @update:model-value="setGen('offset', $event)"
      />
      <DigitKnob
        v-if="waveIs('PULSE')"
        label="Заполнение"
        unit="%"
        :model-value="gen.duty"
        :min="1"
        :max="99"
        :int-digits="2"
        :frac-digits="1"
        :start-digit="0"
        :disabled="disabled"
        @update:model-value="setGen('duty', $event)"
      />
      <DigitKnob
        v-if="waveIs('RAMP')"
        label="Симметрия"
        unit="%"
        :model-value="gen.symmetry"
        :min="0"
        :max="100"
        :int-digits="3"
        :frac-digits="1"
        :start-digit="0"
        :disabled="disabled"
        @update:model-value="setGen('symmetry', $event)"
      />
    </div>
  </section>
</template>

<style scoped>
.gen {
  display: flex;
  gap: 16px;
  align-items: stretch;
  border: 1px solid var(--line);
  border-radius: var(--radius);
  padding: 10px 12px;
  background: var(--panel);
}
.left { display: flex; flex-direction: column; gap: 9px; width: 330px; flex: none; }
.title { display: flex; align-items: center; justify-content: space-between; }
h3 { margin: 0; font-size: 12px; font-weight: 600; color: var(--muted); text-transform: uppercase; letter-spacing: .06em; }
.output { display: flex; align-items: center; gap: 8px; padding: 5px 12px; }
.output .led { width: 9px; height: 9px; border-radius: 50%; background: var(--dim); }
.output.on { background: var(--ok-bg); border-color: #23603b; color: #c9f7d9; }
.output.on .led { background: var(--ok); box-shadow: 0 0 8px var(--ok); }
.waves { display: grid; grid-template-columns: repeat(4, 1fr); gap: 5px; }
.wave {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 3px;
  padding: 6px 4px 5px;
  font-size: 12px;
  color: var(--muted);
}
.wave svg path { fill: none; stroke: currentColor; stroke-width: 1.6; stroke-linejoin: round; }
.wave.on { color: var(--text); background: var(--panel-3); border-color: #4a5366; }
.wave.on svg path { stroke: var(--accent); }
.other { grid-column: 1 / -1; font-size: 12px; color: var(--muted); }
.other.on { color: var(--text); border-color: #4a5366; }
.opts { display: flex; align-items: center; justify-content: space-between; font-size: 12px; color: var(--muted); }
.opts label { display: flex; align-items: center; gap: 6px; cursor: pointer; }
.knobs { display: flex; gap: 10px; flex: 1; min-width: 0; align-items: flex-start; flex-wrap: wrap; }
.knobs > * { flex: 1 1 170px; }
.freq { flex: 2 1 300px; display: flex; flex-direction: column; gap: 4px; }
.presets { display: flex; gap: 2px; flex-wrap: wrap; }
.presets button { font-size: 12px; padding: 2px 8px; }
.disabled .knobs, .disabled .waves { opacity: .55; }
</style>
