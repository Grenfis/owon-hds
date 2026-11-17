<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { call, saveFile } from './api'
import BodeView from './components/BodeView.vue'
import ConsoleView from './components/ConsoleView.vue'
import CurveView from './components/CurveView.vue'
import DistortionView from './components/DistortionView.vue'
import DmmView from './components/DmmView.vue'
import GenPanel from './components/GenPanel.vue'
import HelpView from './components/HelpView.vue'
import ImpedanceView from './components/ImpedanceView.vue'
import MeasureStrip from './components/MeasureStrip.vue'
import PresetMenu from './components/PresetMenu.vue'
import RecordView from './components/RecordView.vue'
import ScopeCanvas from './components/ScopeCanvas.vue'
import ScopeControls from './components/ScopeControls.vue'
import SpectrumView from './components/SpectrumView.vue'
import StepView from './components/StepView.vue'
import { frameCSV } from './export'
import { stamp } from './format'
import { scopeOf } from './record'
import { connect, display, disconnect, frame, init, lastError, mathTrace, readGen, rec, recFrames, scope, set, setTrigLevel, status, sweeping as sweepRunning } from './state'

type View = 'scope' | 'dmm' | 'console'
const view = ref<View>('scope')
const frozen = ref(false)
const connecting = ref<'' | 'usb' | 'sim'>('')
const screen = ref<InstanceType<typeof ScopeCanvas>>()

const FFT_KEY = 'owon.fftShown'
const fftShown = ref(localStorage.getItem(FFT_KEY) === '1')
watch(fftShown, (v) => localStorage.setItem(FFT_KEY, v ? '1' : '0'))
const BODE_KEY = 'owon.bodeShown'
const bodeShown = ref(localStorage.getItem(BODE_KEY) === '1')
watch(bodeShown, (v) => localStorage.setItem(BODE_KEY, v ? '1' : '0'))
const STEP_KEY = 'owon.stepShown'
const stepShown = ref(localStorage.getItem(STEP_KEY) === '1')
watch(stepShown, (v) => localStorage.setItem(STEP_KEY, v ? '1' : '0'))
const IMP_KEY = 'owon.impShown'
const impShown = ref(localStorage.getItem(IMP_KEY) === '1')
watch(impShown, (v) => localStorage.setItem(IMP_KEY, v ? '1' : '0'))
const CURVE_KEY = 'owon.curveShown'
const curveShown = ref(localStorage.getItem(CURVE_KEY) === '1')
watch(curveShown, (v) => localStorage.setItem(CURVE_KEY, v ? '1' : '0'))
const THD_KEY = 'owon.thdShown'
const thdShown = ref(localStorage.getItem(THD_KEY) === '1')
watch(thdShown, (v) => localStorage.setItem(THD_KEY, v ? '1' : '0'))
const REC_KEY = 'owon.recShown'
const recShown = ref(localStorage.getItem(REC_KEY) === '1')
watch(recShown, (v) => localStorage.setItem(REC_KEY, v ? '1' : '0'))

const TABS: { id: View; label: string }[] = [
  { id: 'scope', label: 'Осциллограф и генератор' },
  { id: 'dmm', label: 'Мультиметр' },
  { id: 'console', label: 'Консоль SCPI' },
]

const online = computed(() => status.value.connected)
// While a sweep runs, it owns the scope and the generator; the controls wait
// for it, as do the other tabs.
const sweeping = sweepRunning
const controls = computed(() => online.value && !sweeping.value)
const running = computed(() => scope.run !== 'STOP')
// A recorded frame is drawn with the settings it was taken with.
const shownScope = computed(() => (rec.viewing && frame.value ? scopeOf(frame.value, scope) : scope))

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
  const csv = frameCSV(f, mathTrace.value?.v ?? null, display.math)
  await save(`hds272s_${stamp()}.csv`, new Blob([csv], { type: 'text/csv' }))
}

const screenPNG = async () => (await screen.value?.toPNG()) ?? null

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

const helpShown = ref(false)
onMounted(() => {
  init()
  window.addEventListener('keydown', (e) => {
    if (e.key === 'F1') {
      e.preventDefault()
      helpShown.value = true
    }
  })
})
</script>

<template>
  <div class="app">
    <header>
      <div class="brand">
        <span class="logo">HDS272S</span>
        <span class="muted">пульт</span>
      </div>
      <nav>
        <button v-for="t in TABS" :key="t.id" class="tab" :class="{ on: view === t.id }" :disabled="sweeping && view !== t.id" @click="view = t.id">{{ t.label }}</button>
      </nav>
      <button class="ghost help" title="Руководство по режимам (F1)" @click="helpShown = true">Справка</button>
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
    <HelpView v-if="helpShown" @close="helpShown = false" />

    <div v-if="lastError && (online || view !== 'scope')" class="banner err">
      {{ lastError }}
      <button class="ghost" @click="lastError = ''">✕</button>
    </div>

    <main v-if="view === 'scope'" class="scope">
      <div class="center">
        <div class="toolbar">
          <button class="run" :class="running ? 'running' : 'stopped'" :disabled="!controls" @click="set('run', running ? 'STOP' : 'RUN')">
            {{ running ? '■ Стоп' : '▶ Пуск' }}
          </button>
          <button :disabled="!controls" title="Автонастройка масштабов, как кнопка Auto на приборе" @click="set('auto', '')">Авто</button>
          <button :class="{ primary: frozen }" :disabled="!controls" title="Не запрашивать новые кадры; прибор продолжает работать" @click="frozen = !frozen">
            {{ frozen ? 'Экран заморожен' : 'Заморозить экран' }}
          </button>
          <button :class="{ primary: fftShown }" title="Спектр сигнала (FFT) под осциллограммой" @click="fftShown = !fftShown">Спектр</button>
          <button :class="{ primary: bodeShown }" title="АЧХ и ФЧХ цепи: генератор перебирает частоты, CH1 на входе цепи, CH2 на выходе" @click="bodeShown = !bodeShown">АЧХ</button>
          <button :class="{ primary: thdShown }" title="КНИ от размаха: генератор перебирает размахи синуса, CH1 на входе цепи, CH2 на выходе" @click="thdShown = !thdShown">КНИ</button>
          <button :class="{ primary: stepShown }" title="Переходная характеристика: меандр на входе цепи и CH1, выход на CH2" @click="stepShown = !stepShown">Переходная</button>
          <button :class="{ primary: impShown }" title="Импеданс детали через эталонный резистор: C, L, R, ESR" @click="impShown = !impShown">Импеданс</button>
          <button :class="{ primary: curveShown }" title="Тестер компонентов: вольт-амперная характеристика детали через эталонный резистор" @click="curveShown = !curveShown">ВАХ</button>
          <button :class="{ primary: recShown }" title="Запись и просмотр кадров, ловушка редких событий" @click="recShown = !recShown">Запись</button>
          <PresetMenu :online="controls" @save="save" />
          <div class="info">
            <span v-if="rec.viewing" class="state warn" title="На экране записанный кадр, а не живой сигнал">Запись {{ rec.index + 1 }}/{{ recFrames.length }}</span>
            <span v-if="rec.recording" class="state rec">● запись {{ recFrames.length }}</span>
            <span v-if="frame && !rec.viewing" class="state" :class="runState[1]">{{ runState[0] }}</span>
            <span v-if="frame" class="mono">{{ frame.fps.toFixed(1) }} кадр/с</span>
            <span v-if="frame?.averaged" class="mono" :title="`Усреднение: в среднем ${frame.averaged} кадров из ${display.avg}`">усредн. {{ frame.averaged }}/{{ display.avg }}</span>
            <span v-if="frame?.sampleRate" class="mono dim">{{ frame.sampleRate }}</span>
          </div>
          <span v-if="saved" class="saved">{{ saved }}</span>
          <button class="ghost" :disabled="!frame" title="Сохранить точки осциллограммы в CSV" @click="exportCSV">CSV</button>
          <button class="ghost" :disabled="!frame" title="Сохранить картинку экрана" @click="exportPNG">PNG</button>
        </div>
        <ScopeCanvas ref="screen" class="canvas" :frame="frame" :scope="shownScope" :stretch="fftShown || bodeShown || thdShown || stepShown || impShown || curveShown || recShown" @offset="(ch, d) => set(`${ch}.offset`, d.toFixed(2))" @level="setTrigLevel">
          <div v-if="!online && !rec.viewing" class="overlay">
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
        <BodeView v-if="bodeShown" class="spectrum" :online="online" @save="save" />
        <DistortionView v-if="thdShown" class="spectrum" :online="online" @save="save" />
        <StepView v-if="stepShown" class="spectrum" :frame="frame" :online="controls" @save="save" />
        <ImpedanceView v-if="impShown" :frame="frame" :online="controls" />
        <CurveView v-if="curveShown" class="spectrum" :frame="frame" :online="controls" @save="save" />
        <RecordView v-if="recShown" :online="controls" :png="screenPNG" @save="save" />
        <MeasureStrip :frame="frame" />
      </div>
      <aside>
        <ScopeControls :disabled="!controls" />
      </aside>
      <GenPanel class="genpanel" :disabled="!controls" />
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
nav { display: flex; gap: 2px; flex: 1 0 auto; }
.tab { border: 0; background: transparent; color: var(--muted); padding: 6px 12px; white-space: nowrap; }
.tab:hover:not(:disabled) { background: var(--panel-2); color: var(--text); }
.tab.on { background: var(--panel-3); color: var(--text); }
/* When the header is short of room, the device name in the chip gives way. */
.conn { display: flex; align-items: center; gap: 8px; min-width: 0; }
.chip {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 4px 10px;
  border-radius: 999px;
  font-size: 12px;
  max-width: 460px;
  min-width: 90px;
  flex-shrink: 1;
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
.toolbar { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.toolbar button, .info > span { white-space: nowrap; }
.run { min-width: 86px; font-weight: 600; }
.run.running { background: var(--err-bg); border-color: #6b2a2f; color: #ffd0d0; }
.run.stopped { background: var(--ok-bg); border-color: #23603b; color: #c9f7d9; }
.info { display: flex; align-items: center; gap: 12px; margin-left: 10px; flex: 1; font-size: 12px; }
.state { padding: 2px 8px; border-radius: 4px; background: var(--panel-2); }
.state.ok { color: var(--ok); }
.state.warn { color: var(--warn); }
.state.rec { color: var(--err); }
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
