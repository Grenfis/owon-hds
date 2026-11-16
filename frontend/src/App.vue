<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { call, saveFile } from './api'
import ConsoleView from './components/ConsoleView.vue'
import DmmView from './components/DmmView.vue'
import GenPanel from './components/GenPanel.vue'
import MeasureStrip from './components/MeasureStrip.vue'
import ScopeCanvas from './components/ScopeCanvas.vue'
import ScopeControls from './components/ScopeControls.vue'
import SpectrumView from './components/SpectrumView.vue'
import { stamp } from './format'
import { connect, disconnect, frame, init, lastError, readGen, scope, set, setTrigLevel, status } from './state'

type View = 'scope' | 'dmm' | 'console'
const view = ref<View>('scope')
const frozen = ref(false)
const connecting = ref<'' | 'usb' | 'sim'>('')
const screen = ref<InstanceType<typeof ScopeCanvas>>()

const FFT_KEY = 'owon.fftShown'
const fftShown = ref(localStorage.getItem(FFT_KEY) === '1')
watch(fftShown, (v) => localStorage.setItem(FFT_KEY, v ? '1' : '0'))

const TABS: { id: View; label: string }[] = [
  { id: 'scope', label: 'Осциллограф и генератор' },
  { id: 'dmm', label: 'Мультиметр' },
  { id: 'console', label: 'Консоль SCPI' },
]

const online = computed(() => status.value.connected)
const running = computed(() => scope.run !== 'STOP')

const RUN_STATES: Record<string, [string, string]> = {
  trig: ['Синхронизирован', 'ok'], auto: ['Авто', 'ok'], ready: ['Ждёт запуска', 'warn'],
  scan: ['Сканирование', 'ok'], stop: ['Остановлен', 'warn'],
}
const runState = computed(() => RUN_STATES[frame.value?.run ?? ''] ?? [frame.value?.run || '—', ''])

async function doConnect(mode: 'usb' | 'sim') {
  connecting.value = mode
  await connect(mode)
  connecting.value = ''
}

// Immediate: after a page reload the backend may still poll another view.
// The backend also switches the device screen to match the view. Back on
// the scope, the generator is read again: it may have been changed on the
// device while it showed the multimeter.
watch(view, (v, old) => {
  call('SetView', v)
  if (v === 'scope' && old && online.value) readGen().catch(() => {})
}, { immediate: true })
watch(frozen, (f) => call('SetAcquire', !f), { immediate: true })

async function exportCSV() {
  const f = frame.value
  if (!f) return
  const chans = f.channels.filter((c) => c.on && c.raw?.length)
  const n = Math.max(...chans.map((c) => c.raw!.length))
  const lines = [`# OWON HDS272S, ${f.timeScale}/div, ${f.sampleRate}; time from trigger point`, ['t_s', ...chans.map((c) => `${c.name}_V`)].join(',')]
  for (let i = 0; i < n; i++) {
    // A positive offset puts the trigger point hOffset divisions left of centre.
    const t = ((i / (n - 1)) * 12 - 6 + f.hOffset) * f.timeDiv
    lines.push([t.toPrecision(6), ...chans.map((c) => (((c.raw![i] - c.offset) / 25) * c.voltsPerDiv).toPrecision(5))].join(','))
  }
  await save(`hds272s_${stamp()}.csv`, new Blob([lines.join('\n') + '\n'], { type: 'text/csv' }))
}

async function exportPNG() {
  const b = await screen.value?.toPNG()
  if (b) await save(`hds272s_${stamp()}.png`, b)
}

const saved = ref('')
async function save(name: string, blob: Blob) {
  try {
    const path = await saveFile(name, blob)
    if (path) {
      saved.value = `Сохранено: ${path}`
      setTimeout(() => (saved.value = ''), 4000)
    }
  } catch (e: any) {
    lastError.value = String(e.message ?? e)
  }
}

onMounted(init)
</script>

<template>
  <div class="app">
    <header>
      <div class="brand">
        <span class="logo">HDS272S</span>
        <span class="muted">пульт</span>
      </div>
      <nav>
        <button v-for="t in TABS" :key="t.id" class="tab" :class="{ on: view === t.id }" @click="view = t.id">{{ t.label }}</button>
      </nav>
      <div class="conn">
        <template v-if="online">
          <span class="chip ok" :title="status.detail">
            <span class="dot" />{{ status.mode === 'sim' ? 'Имитатор' : 'USB' }} · <span class="mono">{{ status.idn }}</span>
          </span>
          <button class="ghost" @click="disconnect">Отключить</button>
        </template>
        <template v-else-if="status.reconnecting">
          <span class="chip warn" :title="status.error"><span class="dot" />Переподключение…</span>
          <button class="ghost" @click="disconnect">Отмена</button>
        </template>
        <template v-else>
          <button class="primary" :disabled="!!connecting" @click="doConnect('usb')">{{ connecting === 'usb' ? 'Подключаю…' : 'Подключить по USB' }}</button>
          <button :disabled="!!connecting" title="Работа без прибора: CH1 — генератор, CH2 — RC-фильтр 10 кГц" @click="doConnect('sim')">Имитатор</button>
        </template>
      </div>
    </header>

    <div v-if="lastError && (online || view !== 'scope')" class="banner err">
      {{ lastError }}
      <button class="ghost" @click="lastError = ''">✕</button>
    </div>

    <main v-if="view === 'scope'" class="scope">
      <div class="center">
        <div class="toolbar">
          <button class="run" :class="running ? 'running' : 'stopped'" :disabled="!online" @click="set('run', running ? 'STOP' : 'RUN')">
            {{ running ? '■ Стоп' : '▶ Пуск' }}
          </button>
          <button :disabled="!online" title="Автонастройка масштабов, как кнопка Auto на приборе" @click="set('auto', '')">Авто</button>
          <button :class="{ primary: frozen }" :disabled="!online" title="Не запрашивать новые кадры; прибор продолжает работать" @click="frozen = !frozen">
            {{ frozen ? 'Экран заморожен' : 'Заморозить экран' }}
          </button>
          <button :class="{ primary: fftShown }" title="Спектр сигнала (FFT) под осциллограммой" @click="fftShown = !fftShown">Спектр</button>
          <div class="info">
            <span v-if="frame" class="state" :class="runState[1]">{{ runState[0] }}</span>
            <span v-if="frame" class="mono">{{ frame.fps.toFixed(1) }} кадр/с</span>
            <span v-if="frame?.sampleRate" class="mono dim">{{ frame.sampleRate }}</span>
          </div>
          <span v-if="saved" class="saved">{{ saved }}</span>
          <button class="ghost" :disabled="!frame" title="Сохранить точки осциллограммы в CSV" @click="exportCSV">CSV</button>
          <button class="ghost" :disabled="!frame" title="Сохранить картинку экрана" @click="exportPNG">PNG</button>
        </div>
        <ScopeCanvas ref="screen" class="canvas" :frame="frame" :scope="scope" :stretch="fftShown" @offset="(ch, d) => set(`${ch}.offset`, d.toFixed(2))" @level="setTrigLevel">
          <div v-if="!online" class="overlay">
            <div class="card">
              <h2>{{ status.reconnecting ? 'Связь потеряна' : 'Нет связи с прибором' }}</h2>
              <p v-if="status.error || lastError" class="err-text">{{ status.error || lastError }}</p>
              <ol>
                <li>Включите прибор. В меню <b>System → F4 → USB</b> выберите <b>HID</b>.</li>
                <li>Подключите кабель USB.</li>
                <li>Нажмите <b>«Подключить по USB»</b> вверху справа.</li>
              </ol>
              <p class="muted">Без прибора можно запустить имитатор: в нём CH1 показывает генератор, а CH2 — тот же сигнал после RC-фильтра.</p>
            </div>
          </div>
        </ScopeCanvas>
        <SpectrumView v-if="fftShown" class="spectrum" :frame="frame" @save="save" />
        <MeasureStrip :frame="frame" />
      </div>
      <aside>
        <ScopeControls :disabled="!online" />
      </aside>
      <GenPanel class="genpanel" :disabled="!online" />
    </main>

    <main v-else-if="view === 'dmm'" class="page"><DmmView :disabled="!online" /></main>
    <main v-else class="page"><ConsoleView /></main>
  </div>
</template>

<style scoped>
.app { display: flex; flex-direction: column; height: 100%; }
header {
  display: flex;
  align-items: center;
  gap: 18px;
  padding: 8px 14px;
  border-bottom: 1px solid var(--line);
  background: var(--panel);
}
.brand { display: flex; align-items: baseline; gap: 8px; }
.logo { font-weight: 700; letter-spacing: .04em; }
nav { display: flex; gap: 2px; flex: 1; }
.tab { border: 0; background: transparent; color: var(--muted); padding: 6px 12px; }
.tab:hover:not(:disabled) { background: var(--panel-2); color: var(--text); }
.tab.on { background: var(--panel-3); color: var(--text); }
.conn { display: flex; align-items: center; gap: 8px; }
.chip {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 4px 10px;
  border-radius: 999px;
  font-size: 12px;
  max-width: 460px;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}
.chip .dot { width: 7px; height: 7px; border-radius: 50%; background: currentColor; flex: none; }
.chip.ok { background: var(--ok-bg); color: var(--ok); }
.chip.warn { background: var(--warn-bg); color: var(--warn); }
.chip .mono { color: var(--text); opacity: .85; }
.banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 14px;
  font-size: 12px;
}
.banner.err { background: var(--err-bg); color: #ffd0d0; }
.scope {
  flex: 1;
  min-height: 0;
  display: grid;
  grid-template-columns: minmax(0, 1fr) 340px;
  grid-template-rows: minmax(0, 1fr) auto;
  gap: 10px;
  padding: 10px;
}
.center { display: flex; flex-direction: column; gap: 8px; min-height: 0; }
.toolbar { display: flex; align-items: center; gap: 6px; }
.run { min-width: 86px; font-weight: 600; }
.run.running { background: var(--err-bg); border-color: #6b2a2f; color: #ffd0d0; }
.run.stopped { background: var(--ok-bg); border-color: #23603b; color: #c9f7d9; }
.info { display: flex; align-items: center; gap: 12px; margin-left: 10px; flex: 1; font-size: 12px; }
.state { padding: 2px 8px; border-radius: 4px; background: var(--panel-2); }
.state.ok { color: var(--ok); }
.state.warn { color: var(--warn); }
.saved { font-size: 12px; color: var(--ok); max-width: 420px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.canvas { flex: 1; }
.spectrum { flex: 1; }
aside { overflow-y: auto; min-height: 0; padding-right: 2px; }
.genpanel { grid-column: 1 / -1; }
.overlay { position: absolute; inset: 0; display: grid; place-items: center; pointer-events: none; }
.overlay .card {
  pointer-events: auto;
  max-width: 500px;
  background: rgba(20, 23, 30, .94);
  border: 1px solid var(--line-2);
  border-radius: 12px;
  padding: 18px 22px;
  line-height: 1.55;
}
.overlay h2 { margin: 0 0 8px; font-size: 16px; }
.overlay ol { padding-left: 20px; margin: 8px 0; }
.err-text { color: var(--err); margin: 0 0 6px; font-size: 12px; }
.page { flex: 1; min-height: 0; }
</style>
