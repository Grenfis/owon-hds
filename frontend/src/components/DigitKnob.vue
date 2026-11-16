<script setup lang="ts">
// Digit-by-digit value entry, as on a bench generator: the mouse wheel over a
// digit changes that digit with carry, the arrows above and below a digit
// step it, the keyboard moves and steps the selected digit, and a double
// click opens a text field that accepts values like "1.5k" or "250m".
import { computed, nextTick, ref } from 'vue'
import { parseInput } from '../format'

const props = withDefaults(
  defineProps<{
    modelValue: number
    min: number
    max: number
    intDigits: number
    fracDigits: number
    unit: string
    label: string
    hint?: string
    color?: string
    disabled?: boolean
    startDigit?: number // exponent selected at first, 3 = thousands
  }>(),
  { color: 'var(--text)', startDigit: 0, hint: '' },
)
const emit = defineEmits<{ 'update:modelValue': [value: number] }>()

const selected = ref(Math.min(props.intDigits - 1, Math.max(-props.fracDigits, props.startDigit)))
const editing = ref(false)
const draft = ref('')
const input = ref<HTMLInputElement>()
let wheelAcc = 0

const signed = computed(() => props.min < 0)

interface Digit { exp: number; ch: string; lead: boolean; group: boolean }

const digits = computed<Digit[]>(() => {
  const a = Math.abs(props.modelValue)
  const [ip, fp = ''] = a.toFixed(props.fracDigits).split('.')
  const int = ip.padStart(props.intDigits, '0').slice(-props.intDigits)
  const firstSig = int.search(/[1-9]/)
  const out: Digit[] = []
  for (let i = 0; i < int.length; i++) {
    const exp = props.intDigits - 1 - i
    out.push({
      exp,
      ch: int[i],
      lead: exp > 0 && (firstSig < 0 || i < firstSig),
      group: i > 0 && exp % 3 === 2,
    })
  }
  for (let i = 0; i < fp.length; i++) out.push({ exp: -1 - i, ch: fp[i], lead: false, group: false })
  return out
})

function round(v: number) {
  const k = 10 ** props.fracDigits
  return Math.round(v * k) / k
}

function commit(v: number) {
  v = round(Math.min(props.max, Math.max(props.min, v)))
  if (v !== props.modelValue) emit('update:modelValue', v)
}

function step(exp: number, dir: number) {
  if (props.disabled) return
  selected.value = exp
  commit(props.modelValue + dir * 10 ** exp)
}

function wheel(exp: number, e: WheelEvent) {
  const n = wheelSteps(e)
  if (n) step(exp, n)
}

// wheelSteps turns wheel deltas into knob detents: a mouse notch is about
// 100 px, touchpads send many small deltas that add up.
function wheelSteps(e: WheelEvent): number {
  wheelAcc -= e.deltaMode === 1 ? e.deltaY * 33 : e.deltaY
  const n = Math.trunc(wheelAcc / 100)
  wheelAcc -= n * 100
  return n
}

function key(e: KeyboardEvent) {
  if (editing.value || props.disabled) return
  const lo = -props.fracDigits, hi = props.intDigits - 1
  switch (e.key) {
    case 'ArrowLeft': selected.value = Math.min(hi, selected.value + 1); break
    case 'ArrowRight': selected.value = Math.max(lo, selected.value - 1); break
    case 'ArrowUp': step(selected.value, 1); break
    case 'ArrowDown': step(selected.value, -1); break
    case 'Enter': startEdit(); break
    case '-': if (signed.value) commit(-props.modelValue); break
    default: return
  }
  e.preventDefault()
}

async function startEdit() {
  if (props.disabled) return
  draft.value = String(round(props.modelValue))
  editing.value = true
  await nextTick()
  input.value?.select()
}

function finishEdit(apply: boolean) {
  if (apply) {
    const v = parseInput(draft.value)
    if (v != null) commit(v)
  }
  editing.value = false
}
</script>

<template>
  <div class="knob" :class="{ disabled }" :style="{ '--c': color }">
    <div class="head">
      <span class="label">{{ label }}</span>
      <span class="hint mono">{{ hint }}</span>
    </div>
    <div v-if="!editing" class="display mono" tabindex="0" @keydown="key" @dblclick="startEdit" title="Колесо мыши над цифрой — изменить; двойной щелчок — ввести">
      <button v-if="signed" class="sign" :disabled="disabled" @click="commit(-modelValue)" title="Сменить знак">
        {{ modelValue < 0 ? '−' : '+' }}
      </button>
      <template v-for="d in digits" :key="d.exp">
        <span v-if="d.exp === -1" class="dot">.</span>
        <span
          class="digit"
          :class="{ lead: d.lead, sel: d.exp === selected, group: d.group }"
          @wheel.prevent="wheel(d.exp, $event)"
          @click="selected = d.exp"
        >
          <i class="up" @click.stop="step(d.exp, 1)">▲</i>
          {{ d.ch }}
          <i class="down" @click.stop="step(d.exp, -1)">▼</i>
        </span>
      </template>
      <span class="unit">{{ unit }}</span>
    </div>
    <div v-else class="display editing">
      <input
        ref="input"
        v-model="draft"
        type="text"
        class="mono"
        @keydown.enter="finishEdit(true)"
        @keydown.esc="finishEdit(false)"
        @blur="finishEdit(true)"
      />
      <span class="unit">{{ unit }}</span>
    </div>
  </div>
</template>

<style scoped>
.knob {
  background: var(--bg);
  border: 1px solid var(--line);
  border-radius: var(--radius);
  padding: 6px 10px 4px;
  min-width: 0;
}
.knob.disabled { opacity: .5; }
.head { display: flex; justify-content: space-between; gap: 12px; font-size: 12px; }
.label { color: var(--muted); }
.hint { color: var(--dim); white-space: nowrap; }
.display {
  display: flex;
  align-items: center;
  font-size: 25px;
  line-height: 1;
  padding: 13px 0 12px;
  outline: none;
  color: var(--c);
  white-space: nowrap;
}
.display:focus-visible { box-shadow: 0 1px 0 var(--accent); }
.digit {
  position: relative;
  padding: 0 1px;
  cursor: ns-resize;
  border-radius: 3px;
}
.digit.group { margin-left: 6px; }
.digit.lead { color: var(--dim); }
.digit:hover { background: var(--panel-2); }
.digit.sel { box-shadow: inset 0 -2px 0 var(--accent); }
.digit i {
  position: absolute;
  left: 50%;
  transform: translateX(-50%);
  font-size: 9px;
  font-style: normal;
  color: var(--muted);
  opacity: 0;
  cursor: pointer;
  padding: 0 3px;
}
.digit i.up { top: -11px; }
.digit i.down { bottom: -11px; }
.digit:hover i, .digit.sel i { opacity: 1; }
.digit i:hover { color: var(--accent); }
.dot { padding: 0 1px; color: var(--muted); }
.unit { font-size: 14px; color: var(--muted); margin-left: 8px; font-family: var(--sans); }
.sign {
  font-family: var(--mono);
  font-size: 18px;
  padding: 0 6px;
  margin-right: 4px;
  border-color: var(--line);
  color: var(--c);
}
.editing input { width: 100%; font-size: 20px; padding: 2px 6px; }
.disabled .digit { cursor: default; }
</style>
