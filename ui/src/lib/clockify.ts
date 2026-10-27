/** "Sync from Clockify": filter and sum fetched entries in the browser, so trying filters costs no requests. */
import type { ClockifyEntry, ClockifyEntries } from './pb'
import type { ClockifyDefaults, Field } from './schema'

export interface Filter {
  billable: boolean
  /** Project ids to include ('' = entries without a project); empty means all. */
  projects: string[]
  /** Case-insensitive "description contains". */
  text: string
}

export interface Project { id: string; name: string; label: string; seconds: number }

/** A project id no entry has: "only these projects" with none of them present. */
const NONE = '-'

const projectLabel = (e: ClockifyEntry) =>
  e.project ? (e.client ? `${e.client} – ${e.project_name}` : e.project_name) : '(no project)'

/** Projects present in the fetched entries, most hours first. */
export function projects(entries: ClockifyEntry[]): Project[] {
  const m = new Map<string, Project>()
  for (const e of entries) {
    const p = m.get(e.project) ?? { id: e.project, name: e.project_name, label: projectLabel(e), seconds: 0 }
    p.seconds += e.seconds
    m.set(e.project, p)
  }
  return [...m.values()].sort((a, b) => b.seconds - a.seconds)
}

/** The template's default filter (`"clockify": {...}` on the list field); project names map to ids. */
export function defaultFilter(field: Field, entries: ClockifyEntry[]): Filter {
  const d: ClockifyDefaults = typeof field.clockify === 'object' ? field.clockify : {}
  const names = new Set((d.projects ?? []).map(n => n.toLowerCase()))
  const ids = projects(entries).filter(p => names.has(p.label.toLowerCase()) || names.has(p.name.toLowerCase())).map(p => p.id)
  // named projects with no time in this range must not fall back to "all"
  return { billable: !!d.billable, projects: names.size && !ids.length ? [NONE] : ids, text: '' }
}

export function matches(e: ClockifyEntry, f: Filter): boolean {
  if (f.billable && !e.billable) return false
  if (f.projects.length && !f.projects.includes(e.project)) return false
  if (f.text && !e.description.toLowerCase().includes(f.text.toLowerCase())) return false
  return true
}

export function total(entries: ClockifyEntry[], f: Filter): { seconds: number; count: number } {
  let seconds = 0, count = 0
  for (const e of entries) if (matches(e, f)) { seconds += e.seconds; count++ }
  return { seconds, count }
}

const round = (v: number, d: number) => Math.round(v * 10 ** d) / 10 ** d

/**
 * Write hours, rate and amount into a row. Hours are the exact tracked time
 * to two decimals; the amount is computed from those two-decimal hours so the
 * invoice line "hours × rate = amount" adds up for whoever reads it.
 */
export function applyToRow(row: Record<string, any>, seconds: number, rate: number | null): void {
  const hours = round(seconds / 3600, 2)
  if ('hours' in row) row.hours = hours
  if (rate !== null && 'rate' in row) row.rate = rate
  if (rate !== null) row.amount = round(hours * rate, 2)
}

/** Effective rate: the row's own, then the one from Settings, then what Clockify still stores. */
export function pickRate(row: Record<string, any>, settingsRate: number, data: ClockifyEntries): number | null {
  if (typeof row.rate === 'number' && row.rate > 0) return row.rate
  if (settingsRate > 0) return settingsRate
  return data.clockify_rate
}

export const fmtHours = (seconds: number) => {
  const h = Math.floor(seconds / 3600), m = Math.floor((seconds % 3600) / 60), s = seconds % 60
  return `${(seconds / 3600).toFixed(2)} h (${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')})`
}
