// Presets: named snapshots of the scope settings, of the generator and of
// the screen modes, kept in the browser storage and applied in one go. The
// generator output and run/stop are not part of a preset: they act on the
// circuit, not on how it is looked at.

export interface Preset {
  name: string
  at: number // ms since 1970, when it was saved
  scope: Record<string, string>
  gen?: Record<string, string | number>
  display: Record<string, unknown>
}

export const PRESETS_FORMAT = 'owon-hds272s-presets'

// The order the settings go to the device: the probe before the scale, which
// the device takes with the probe factor; the time base before its offset;
// the trigger source before its level.
export const SCOPE_ORDER = [
  ...['ch1', 'ch2'].flatMap((c) => ['display', 'coupling', 'probe', 'scale', 'offset'].map((k) => `${c}.${k}`)),
  'hor.scale', 'hor.offset', 'acq.mode', 'acq.depmem',
  'trig.source', 'trig.coupling', 'trig.edge', 'trig.sweep', 'trig.level',
]
const NUMERIC = new Set(['ch1.offset', 'ch2.offset', 'hor.offset', 'trig.level'])
export const DISPLAY_KEYS = ['math', 'mathScale', 'mathOffset', 'xy', 'ref', 'avg', 'persist']

export function snapshot(
  name: string,
  scope: Record<string, string>,
  gen: Record<string, string | number | boolean> | null,
  display: Record<string, unknown>,
): Preset {
  const p: Preset = { name, at: Date.now(), scope: {}, display: {} }
  for (const k of SCOPE_ORDER) if (k in scope) p.scope[k] = scope[k]
  if (gen) {
    p.gen = {}
    for (const k of ['wave', 'freq', 'amp', 'offset', 'duty', 'symmetry']) {
      const v = gen[k]
      if (typeof v === 'string' || typeof v === 'number') p.gen[k] = v
    }
  }
  for (const k of DISPLAY_KEYS) if (k in display) p.display[k] = display[k]
  return p
}

// changes lists the scope settings of a preset that differ from the current
// ones, in the order they go out. A new probe makes the device rescale the
// channel, so the scale goes out with it.
export function changes(p: Preset, cur: Record<string, string>): [string, string][] {
  const out: [string, string][] = []
  for (const k of SCOPE_ORDER) {
    const v = p.scope[k]
    if (v == null) continue
    const same = NUMERIC.has(k) ? Math.abs(+v - +cur[k]) < 1e-9 : v === cur[k]
    const probe = k.replace('.scale', '.probe')
    const reprobed = k.endsWith('.scale') && p.scope[probe] != null && p.scope[probe] !== cur[probe]
    if (!same || reprobed) out.push([k, v])
  }
  return out
}

// describe sums a preset up in a line.
export function describe(p: Preset): string {
  const s = p.scope
  const chans = ['ch1', 'ch2'].filter((c) => s[`${c}.display`] !== 'OFF').map((c) => `${c.toUpperCase()} ${s[`${c}.scale`] ?? '?'}/дел`)
  const parts = [chans.join(', ') || 'каналы выключены', `${s['hor.scale'] ?? '?'}/дел`]
  if (s['trig.sweep']) parts.push(`запуск ${({ AUTO: 'авто', NORMal: 'ждущий', SINGle: 'однократный' } as Record<string, string>)[s['trig.sweep']] ?? s['trig.sweep']}`)
  if (p.gen) parts.push(`генератор ${p.gen.wave} ${p.gen.freq} Гц`)
  return parts.join(' · ')
}

function isPreset(p: any): p is Preset {
  return (
    typeof p?.name === 'string' && p.name !== '' && typeof p.at === 'number' &&
    p.scope && typeof p.scope === 'object' && Object.values(p.scope).every((v) => typeof v === 'string') &&
    p.display && typeof p.display === 'object' && (p.gen == null || typeof p.gen === 'object')
  )
}

export function presetsJSON(list: Preset[]): string {
  return JSON.stringify({ format: PRESETS_FORMAT, version: 1, presets: list }, null, 1)
}

// parsePresets reads a file of presets, or a stored list, and returns what
// is wrong with it when there is nothing to take.
export function parsePresets(text: string): Preset[] | string {
  let d: any
  try {
    d = JSON.parse(text)
  } catch {
    return 'Это не JSON'
  }
  const list = Array.isArray(d) ? d : d?.format === PRESETS_FORMAT ? d.presets : null
  if (!Array.isArray(list)) return 'Это не файл пресетов этой программы'
  const ok = list.filter(isPreset)
  return ok.length ? ok : 'В файле нет пресетов'
}

// merge puts presets into a list; one with a name already there replaces it.
export function merge(list: Preset[], add: Preset[]): Preset[] {
  const out = [...list]
  for (const p of add) {
    const i = out.findIndex((q) => q.name === p.name)
    if (i >= 0) out[i] = p
    else out.push(p)
  }
  return out
}
