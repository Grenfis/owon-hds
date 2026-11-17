<script setup lang="ts">
// Recording of frames and their playback, and the capture of rare events:
// in the single-shot mode every trigger is saved to PNG and CSV, and the
// trigger is armed again.
import { computed, nextTick, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { chooseFolder, writeFile } from '../api'
import { frameCSV } from '../export'
import { eng, stamp } from '../format'
import { mathOf } from '../math'
import { MAX_FRAMES, parseRecording, recordingJSON, sameData } from '../record'
import { display, frame, lastError, rec, recFrames, recordFrame, scope, set, showRecorded, stopViewing } from '../state'
import type { Frame } from '../types'
import Seg from './Seg.vue'

const props = defineProps<{ online: boolean; png: () => Promise<Blob | null> }>()
const emit = defineEmits<{ save: [name: string, blob: Blob] }>()

const n = computed(() => recFrames.value.length)
const duration = computed(() => (n.value > 1 ? (recFrames.value[n.value - 1].t - recFrames.value[0].t) / 1000 : 0))
const clock = (t: number) => {
  const d = new Date(t)
  return `${d.toLocaleTimeString('ru-RU')}.${String(d.getMilliseconds()).padStart(3, '0')}`
}

// ---- recording ----

function record() {
  stopTrap()
  pause()
  stopViewing()
  recFrames.value = []
  rec.recording = true
}

function save() {
  emit('save', `hds272s_frames_${stamp()}.json`, new Blob([recordingJSON(recFrames.value)], { type: 'application/json' }))
}

const fileInput = ref<HTMLInputElement>()
const loadError = ref('')
async function load(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  const r = parseRecording(await file.text())
  if (typeof r === 'string') {
    loadError.value = r
    return
  }
  loadError.value = ''
  stopTrap()
  pause()
  rec.recording = false
  recFrames.value = r
  showRecorded(0)
}

function clear() {
  pause()
  stopViewing()
  rec.recording = false
  recFrames.value = []
}

// ---- playback ----

const SPEEDS = [
  { value: '0.25', label: '×¼', title: 'В четыре раза медленнее, чем записано' },
  { value: '1', label: '×1', title: 'Как записано' },
  { value: '4', label: '×4', title: 'В четыре раза быстрее' },
]
const speed = ref('1')
const playing = ref(false)
let raf = 0, startedAt = 0, startT = 0

function play() {
  if (!n.value) return
  if (!rec.viewing || rec.index >= n.value - 1) showRecorded(rec.viewing ? 0 : rec.index)
  playing.value = true
  startedAt = performance.now()
  startT = recFrames.value[rec.index].t
  raf = requestAnimationFrame(step)
}

// step shows the last frame recorded before the time the playback reached.
function step() {
  if (!playing.value) return
  const target = startT + (performance.now() - startedAt) * +speed.value
  const r = recFrames.value
  let i = rec.index
  while (i + 1 < r.length && r[i + 1].t <= target) i++
  if (i !== rec.index) showRecorded(i)
  if (i >= r.length - 1) {
    playing.value = false
    return
  }
  raf = requestAnimationFrame(step)
}

function pause() {
  playing.value = false
  cancelAnimationFrame(raf)
}

watch(speed, () => {
  if (!playing.value) return
  startedAt = performance.now()
  startT = recFrames.value[rec.index].t
})

function go(i: number) {
  pause()
  showRecorded(i)
}

function live() {
  pause()
  stopViewing()
}

// ---- capture of rare events ----

const trap = reactive({ on: false, count: 0, dir: '', last: 0, armed: false })
let lastCapture: Frame | null = null
let sweepBefore = ''

const trapBlocked = computed(() => {
  if (!props.online) return 'Нет связи с прибором'
  if (display.avg > 1) return 'Выключите усреднение: в однократном режиме оно смешивает разные срабатывания'
  return ''
})

async function startTrap() {
  if (trapBlocked.value) return
  try {
    if (!trap.dir) trap.dir = await chooseFolder()
  } catch (e: any) {
    lastError.value = String(e.message ?? e)
    return
  }
  if (!trap.dir) return
  pause()
  stopViewing()
  rec.recording = false
  recFrames.value = []
  // A capture the device holds from before is not a new one.
  lastCapture = frame.value?.run === 'stop' ? frame.value : null
  Object.assign(trap, { on: true, count: 0, last: 0, armed: false })
  sweepBefore = scope['trig.sweep']
  if (sweepBefore !== 'SINGle') set('trig.sweep', 'SINGle')
  set('run', 'RUN')
}

// stopTrap puts the trigger mode back as it was.
function stopTrap() {
  if (!trap.on) return
  trap.on = false
  if (sweepBefore && sweepBefore !== 'SINGle') {
    set('trig.sweep', sweepBefore)
    set('run', 'RUN')
  }
}

async function changeDir() {
  try {
    const d = await chooseFolder()
    if (d) trap.dir = d
  } catch (e: any) {
    lastError.value = String(e.message ?? e)
  }
}

// A stopped device keeps sending its capture; a capture is new when the
// device was seen waiting for a trigger after the last one, or when its
// samples differ, for a trigger that came before the next read.
watch(frame, async (f) => {
  if (!trap.on || !f || rec.viewing) return
  if (f.run !== 'stop') {
    trap.armed = true
    return
  }
  if (!trap.armed && lastCapture && sameData(f, lastCapture)) return
  trap.armed = false
  lastCapture = f
  trap.count++
  trap.last = Date.now()
  recordFrame(f)
  const name = `hds272s_trap_${stamp()}_${String(trap.count).padStart(3, '0')}`
  await nextTick()
  const png = props.png() // the picture is taken now, before the next frame
  const csv = frameCSV(f, mathOf(f, display.math)?.v ?? null, display.math)
  set('run', 'RUN')
  try {
    const b = await png
    if (b) await writeFile(`${name}.png`, b)
    await writeFile(`${name}.csv`, new Blob([csv], { type: 'text/csv' }))
  } catch (e: any) {
    lastError.value = `Ловушка: ${e.message ?? e}`
    stopTrap()
  }
})

onBeforeUnmount(() => {
  stopTrap()
  pause()
  stopViewing()
  rec.recording = false
})
</script>

<template>
  <div class="rec">
    <div class="bar">
      <span class="title">Запись</span>
      <button v-if="!rec.recording" :disabled="!online || trap.on" title="Записывать каждый кадр с экрана, до 2000 кадров" @click="record">● Записать</button>
      <button v-else class="stop" title="Закончить запись" @click="rec.recording = false">■ Закончить</button>
      <span class="mono info">
        <template v-if="n">{{ n }} кадр. · {{ eng(duration, 'с', 3) }}</template>
        <template v-else>нет кадров</template>
        <span v-if="n >= MAX_FRAMES" class="warn">предел {{ MAX_FRAMES }}</span>
      </span>
      <span class="grow" />
      <button class="ghost" title="Открыть запись из файла JSON" @click="fileInput?.click()">Открыть…</button>
      <button class="ghost" :disabled="!n || rec.recording" title="Сохранить запись в файл JSON" @click="save">Сохранить</button>
      <button class="ghost" :disabled="!n" @click="clear">Очистить</button>
      <input ref="fileInput" type="file" accept=".json,application/json" hidden @change="load" />
    </div>
    <p v-if="loadError" class="err">{{ loadError }}</p>

    <div v-if="n && !rec.recording" class="bar">
      <button :class="{ primary: rec.viewing }" :title="rec.viewing ? 'Вернуться к живому сигналу' : 'Показать записанные кадры на экране'" @click="rec.viewing ? live() : go(rec.index)">
        {{ rec.viewing ? 'Живой сигнал' : 'Просмотр' }}
      </button>
      <button class="ghost nav" title="Первый кадр" @click="go(0)">⏮</button>
      <button class="ghost nav" title="Предыдущий кадр" @click="go(rec.index - 1)">◀</button>
      <button v-if="!playing" class="ghost nav" title="Воспроизвести с записанной скоростью" @click="play">▶</button>
      <button v-else class="ghost nav" title="Пауза" @click="pause">⏸</button>
      <button class="ghost nav" title="Следующий кадр" @click="go(rec.index + 1)">▶|</button>
      <input class="slider" type="range" min="0" :max="n - 1" :value="rec.index" @input="go(+($event.target as HTMLInputElement).value)" />
      <span class="mono info">{{ rec.index + 1 }}/{{ n }} · {{ clock(recFrames[rec.index].t) }}</span>
      <Seg v-model="speed" :options="SPEEDS" />
    </div>

    <div class="bar">
      <span class="title sub">Ловушка</span>
      <button v-if="!trap.on" :disabled="!!trapBlocked" :title="trapBlocked || 'Однократный запуск: каждое срабатывание сохраняется в PNG и CSV, и запуск взводится снова'" @click="startTrap">Взвести</button>
      <button v-else class="stop" title="Снять ловушку и вернуть прежний режим запуска" @click="stopTrap">Снять</button>
      <span class="mono info">
        <template v-if="trap.on || trap.count">поймано {{ trap.count }}<template v-if="trap.last"> · последнее {{ clock(trap.last) }}</template></template>
        <span v-if="trap.on" :class="trap.armed ? 'ok' : 'muted'">{{ trap.armed ? 'ждёт запуска' : 'взводится' }}</span>
      </span>
      <span class="grow" />
      <span v-if="trap.dir" class="dir" :title="trap.dir">{{ trap.dir }}</span>
      <button class="ghost" :disabled="trap.on" title="Папка для снимков" @click="changeDir">Папка…</button>
    </div>
  </div>
</template>

<style scoped>
.rec { display: flex; flex-direction: column; gap: 6px; }
.bar { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; font-size: 12px; }
.title { font-weight: 600; color: var(--muted); text-transform: uppercase; letter-spacing: .06em; font-size: 12px; margin-right: 4px; min-width: 64px; }
.title.sub { font-weight: 500; }
.grow { flex: 1; }
.info { color: var(--muted); display: inline-flex; gap: 8px; }
.stop { background: var(--err-bg); border-color: #6b2a2f; color: #ffd0d0; }
.nav { padding: 3px 7px; }
.slider { flex: 1; min-width: 120px; }
.dir { color: var(--muted); max-width: 260px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; direction: rtl; }
.ok { color: var(--ok); }
.warn { color: var(--warn); }
.err { margin: 0; font-size: 12px; color: var(--err); }
</style>
