<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { TIMEBASES, eng, parseInput } from '../format'
import { fmtFreq, sweepError } from '../gensweep'
import { gen, genSweep, readGen, set, setGen, startGenSweep, stopGenSweep } from '../state'
import DigitKnob from './DigitKnob.vue'
import Seg from './Seg.vue'

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
    // A sweep would change the time base at every step.
    if (!followTime.value || props.disabled || genSweep.running || !(f > 0)) return
    clearTimeout(followTimer)
    followTimer = window.setTimeout(() => {
      const want = 2.5 / (f * 12)
      const opt = TIMEBASES.find((o) => o.value >= want * 0.999) ?? TIMEBASES[TIMEBASES.length - 1]
      set('hor.scale', opt.cmd)
    }, 120)
  },
)

// ---- frequency sweep ----

const SWEEP_SCALES = [
  { value: 'log', label: 'Лог', title: 'Равные шаги логарифма частоты: каждая декада занимает одинаковое время' },
  { value: 'lin', label: 'Лин', title: 'Равные шаги частоты' },
]
const sweepShown = ref(localStorage.getItem('owon.genSweepShown') === '1')
watch(sweepShown, (v) => localStorage.setItem('owon.genSweepShown', v ? '1' : '0'))
const sweepProblem = computed(() => sweepError(genSweep, maxFreq.value))
watch(() => props.disabled, (d) => d && stopGenSweep(false))

function setNum(key: 'f1' | 'f2' | 'time', e: Event) {
  const input = e.target as HTMLInputElement
  const v = parseInput(input.value)
  if (v != null && v > 0) genSweep[key] = v
  input.value = key === 'time' ? String(genSweep.time) : fmtFreq(genSweep[key])
}

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
          <button v-for="p in PRESETS" :key="p" class="ghost" :disabled="disabled || p > maxFreq || genSweep.running" @click="setGen('freq', p)">
            {{ eng(p, 'Гц', 3, true) }}
          </button>
          <span class="grow" />
          <button class="ghost" :class="{ on: sweepShown }" title="Качание частоты: генератор проходит от одной частоты до другой за заданное время" @click="sweepShown = !sweepShown">Качание…</button>
        </div>
        <div v-if="sweepShown || genSweep.running" class="gsweep">
          <label>от <input class="num mono" :value="fmtFreq(genSweep.f1)" :disabled="genSweep.running" title="Начальная частота, Гц: 100, 1k, 2.5M" @change="setNum('f1', $event)" /></label>
          <label>до <input class="num mono" :value="fmtFreq(genSweep.f2)" :disabled="genSweep.running" title="Конечная частота, Гц" @change="setNum('f2', $event)" /></label>
          <label>за <input class="num short mono" :value="genSweep.time" :disabled="genSweep.running" title="Время прохода, с" @change="setNum('time', $event)" /> с</label>
          <Seg :options="SWEEP_SCALES" :model-value="genSweep.log ? 'log' : 'lin'" :disabled="genSweep.running" @update:model-value="genSweep.log = $event === 'log'" />
          <label title="Проходить снова и снова, пока не остановить"><input v-model="genSweep.repeat" type="checkbox" :disabled="genSweep.running" /> по кругу</label>
          <button v-if="!genSweep.running" :disabled="disabled || !!sweepProblem" :title="sweepProblem || 'Начать качание'" @click="startGenSweep">▶ Качать</button>
          <button v-else class="stopsw" title="Остановить и вернуть прежнюю частоту" @click="stopGenSweep()">■ Остановить</button>
          <span v-if="genSweep.running" class="progress"><span :style="{ width: `${genSweep.at * 100}%` }" /></span>
          <span v-if="sweepProblem" class="warn">{{ sweepProblem }}</span>
          <span v-else-if="!gen.output" class="muted">выход выключен</span>
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
.presets .grow { flex: 1; }
.presets button.on { color: var(--text); background: var(--panel-3); }
.gsweep { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; font-size: 12px; color: var(--muted); }
.gsweep label { display: flex; align-items: center; gap: 4px; }
.gsweep .num { width: 64px; font-size: 12px; padding: 2px 5px; }
.gsweep .num.short { width: 40px; }
.gsweep button { font-size: 12px; padding: 3px 9px; }
.stopsw { background: var(--err-bg); border-color: #6b2a2f; color: #ffd0d0; }
.progress { width: 80px; height: 4px; border-radius: 2px; background: var(--panel-3); overflow: hidden; }
.progress span { display: block; height: 100%; background: var(--accent); }
.warn { color: var(--warn); }
.disabled .knobs, .disabled .waves { opacity: .55; }
</style>
