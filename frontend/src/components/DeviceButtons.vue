<script setup lang="ts">
// Two device settings that matter while it is driven from the computer:
// the auto-shutdown, which would cut the connection, and the backlight,
// which is not needed with the screen on the computer.
//
// The device takes both times over USB but keeps counting down the time it
// loaded at the last key press: it reloads the timers only on a key press,
// at power-on and on a factory reset. So a change needs one press of a key
// on the device; Return does nothing else when no menu is open.
import { computed, onBeforeUnmount, ref } from 'vue'
import { dev, setDev } from '../state'

const APPLY = 'Прибор применит это после нажатия любой кнопки на нём, например Return'
const hint = ref(false)
let hintTimer = 0
function askForKey() {
  hint.value = true
  clearTimeout(hintTimer)
  hintTimer = window.setTimeout(() => (hint.value = false), 20000)
}
onBeforeUnmount(() => clearTimeout(hintTimer))

const PREV_KEY = 'owon.shutdownPrev'

const autoOff = computed(() => dev.shutdown !== 'UNLIMITED')
const autoTitle = computed(() =>
  autoOff.value
    ? `Прибор сам выключится через ${dev.shutdown} мин без нажатий на нём, и связь пропадёт. Нажмите, чтобы отключить автовыключение. ${APPLY}`
    : `Автовыключение отключено. Нажмите, чтобы прибор снова выключался сам через 30 мин без нажатий. ${APPLY}`,
)

function toggleAutoOff() {
  if (autoOff.value) {
    try {
      localStorage.setItem(PREV_KEY, dev.shutdown)
    } catch {}
    setDev('shutdown', 'UNLIMITED')
  } else {
    let prev = '30'
    try {
      prev = localStorage.getItem(PREV_KEY) || '30'
    } catch {}
    setDev('shutdown', prev)
  }
  askForKey()
}

const dark = computed(() => dev.backlight !== 'UNLIMITED')
const lightTitle = computed(() =>
  dark.value
    ? `Экран прибора гаснет через ${dev.backlight} с без нажатий на нём; любая кнопка прибора его включает. Нажмите, чтобы экран горел всегда. ${APPLY}`
    : `Экран прибора горит всегда. Нажмите, чтобы он гас через 30 с без нажатий: при работе с компьютера он не нужен и тратит батарею. ${APPLY}`,
)

function toggleLight() {
  setDev('backlight', dark.value ? 'UNLIMITED' : '30')
  askForKey()
}
</script>

<template>
  <div v-if="dev.shutdown || dev.backlight" class="devbtns">
    <span v-if="hint" class="hint" :title="APPLY" @click="hint = false">Нажмите Return на приборе</span>
    <button v-if="dev.shutdown" class="ghost" :class="{ on: !autoOff }" :title="autoTitle" @click="toggleAutoOff">
      <span class="ico">⏻</span>{{ autoOff ? `Автовыкл. ${dev.shutdown} мин` : 'Без автовыкл.' }}
    </button>
    <button v-if="dev.backlight" class="ghost" :class="{ on: dark }" :title="lightTitle" @click="toggleLight">
      <span class="ico">{{ dark ? '☾' : '☀' }}</span>{{ dark ? `Экран гаснет: ${dev.backlight} с` : 'Экран горит' }}
    </button>
  </div>
</template>

<style scoped>
.devbtns { display: flex; align-items: center; gap: 2px; }
.hint {
  font-size: 11px;
  color: var(--warn);
  background: var(--warn-bg);
  border-radius: 4px;
  padding: 3px 8px;
  margin-right: 4px;
  white-space: nowrap;
  cursor: default;
}
button { font-size: 12px; padding: 4px 8px; display: inline-flex; align-items: center; gap: 5px; white-space: nowrap; }
.ico { font-size: 13px; line-height: 1; opacity: .8; }
button.on { color: var(--ok); }
</style>
