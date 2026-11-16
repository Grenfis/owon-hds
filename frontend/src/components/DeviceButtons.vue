<script setup lang="ts">
// Two device settings that matter while it is driven from the computer:
// the auto-shutdown, which would cut the connection, and the backlight,
// which is not needed with the screen on the computer.
import { computed } from 'vue'
import { dev, setDev } from '../state'

const PREV_KEY = 'owon.shutdownPrev'

const autoOff = computed(() => dev.shutdown !== 'UNLIMITED')
const autoTitle = computed(() =>
  autoOff.value
    ? `Прибор сам выключится через ${dev.shutdown} мин без нажатий на нём, и связь пропадёт. Нажмите, чтобы отключить автовыключение`
    : 'Автовыключение отключено. Нажмите, чтобы прибор снова выключался сам через 30 мин без нажатий',
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
}

const dark = computed(() => dev.backlight !== 'UNLIMITED')
const lightTitle = computed(() =>
  dark.value
    ? `Экран прибора гаснет через ${dev.backlight} с без нажатий на нём; любая кнопка прибора его включает. Нажмите, чтобы экран горел всегда`
    : 'Экран прибора горит всегда. Нажмите, чтобы он гас через 30 с без нажатий: при работе с компьютера он не нужен и тратит батарею',
)

function toggleLight() {
  setDev('backlight', dark.value ? 'UNLIMITED' : '30')
}
</script>

<template>
  <div v-if="dev.shutdown || dev.backlight" class="devbtns">
    <button v-if="dev.shutdown" class="ghost" :class="{ on: !autoOff }" :title="autoTitle" @click="toggleAutoOff">
      <span class="ico">⏻</span>{{ autoOff ? `Автовыкл. ${dev.shutdown} мин` : 'Без автовыкл.' }}
    </button>
    <button v-if="dev.backlight" class="ghost" :class="{ on: dark }" :title="lightTitle" @click="toggleLight">
      <span class="ico">{{ dark ? '☾' : '☀' }}</span>{{ dark ? `Экран гаснет: ${dev.backlight} с` : 'Экран горит' }}
    </button>
  </div>
</template>

<style scoped>
.devbtns { display: flex; gap: 2px; }
button { font-size: 12px; padding: 4px 8px; display: inline-flex; align-items: center; gap: 5px; white-space: nowrap; }
.ico { font-size: 13px; line-height: 1; opacity: .8; }
button.on { color: var(--ok); }
</style>
