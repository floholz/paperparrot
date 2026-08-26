/** Client-side mirror of internal/schema (SPEC.md §6): types, empty values, validation. */

export const TYPES = ['text', 'textarea', 'number', 'money', 'date', 'bool', 'select', 'object', 'list', 'asset', 'sequence'] as const
export type FieldType = (typeof TYPES)[number]

export interface Option { value: string; label?: string }

export interface Field {
  key: string
  type: FieldType
  label?: string
  help?: string
  required?: boolean
  default?: any
  placeholder?: string
  pattern?: string
  min?: number
  max?: number
  step?: number
  currency?: string
  options?: Option[]
  fields?: Field[]
  fragment?: string
  accept?: string
  format?: string
  reset?: 'never' | 'year'
}

export interface Schema { fields: Field[] }
export interface Err { path: string; message: string }

const keyRe = /^[a-z][a-z0-9_]*$/
const dateRe = /^\d{4}-\d{2}-\d{2}$/

export const label = (f: Field) => f.label || f.key
export const required = (f: Field) => f.required !== false
export const today = () => new Date().toISOString().slice(0, 10)

/** Parse schema JSON text; returns the schema or an error message. */
export function parseSchema(text: string): { schema?: Schema; error?: string } {
  if (!text.trim()) return { schema: { fields: [] } }
  let raw: any
  try { raw = JSON.parse(text) } catch (e: any) { return { error: 'Invalid JSON: ' + e.message } }
  return checkSchema(raw)
}

export function checkSchema(raw: any): { schema?: Schema; error?: string } {
  if (!raw || typeof raw !== 'object' || !Array.isArray(raw.fields)) return { error: 'Schema must be an object with a "fields" array' }
  const err = checkFields(raw.fields, '', 0)
  return err ? { error: err } : { schema: raw as Schema }
}

function checkFields(fields: any[], path: string, depth: number): string {
  const seen = new Set<string>()
  for (const f of fields) {
    const p = path ? `${path}.${f?.key}` : f?.key
    if (!f || typeof f !== 'object') return `field at "${path || 'root'}" must be an object`
    if (typeof f.key !== 'string' || !keyRe.test(f.key)) return `invalid key "${f.key}" at "${path || 'root'}" (want [a-z][a-z0-9_]*)`
    if (seen.has(f.key)) return `duplicate key "${p}"`
    seen.add(f.key)
    if (!TYPES.includes(f.type)) return `field "${p}" has unknown type "${f.type}"`
    if (f.type === 'object' || f.type === 'list') {
      if (!Array.isArray(f.fields) || !f.fields.length) return `field "${p}" (${f.type}) needs sub-fields`
      if (depth >= 2) return `field "${p}" nests too deep (max 3 levels)`
      const e = checkFields(f.fields, p, depth + 1)
      if (e) return e
    }
    if (f.type === 'select' && (!Array.isArray(f.options) || !f.options.length)) return `select "${p}" needs options`
    if (f.type === 'sequence') {
      if (depth > 0) return `sequence "${p}" must be top-level`
      if (f.reset && f.reset !== 'never' && f.reset !== 'year') return `sequence "${p}": reset must be "never" or "year"`
    }
    if (f.type === 'text' && f.pattern) {
      try { new RegExp(f.pattern) } catch (e: any) { return `field "${p}": bad pattern: ${e.message}` }
    }
  }
  return ''
}

/** Initial value of a field (defaults applied, "today" for dates). */
export function emptyValue(f: Field): any {
  switch (f.type) {
    case 'object': return emptyData(f.fields ?? [])
    case 'list': return Array.from({ length: f.min ?? 0 }, () => emptyData(f.fields ?? []))
    case 'date': return f.default === 'today' ? today() : (typeof f.default === 'string' ? f.default : '')
    case 'bool': return typeof f.default === 'boolean' ? f.default : false
    case 'number': case 'money': return typeof f.default === 'number' ? f.default : null
    default: return typeof f.default === 'string' ? f.default : ''
  }
}

export const emptyData = (fields: Field[]): Record<string, any> =>
  Object.fromEntries(fields.map(f => [f.key, emptyValue(f)]))

/** Fill missing keys so the form has a stable shape (keeps existing values). */
export function normalize(fields: Field[], data: any): Record<string, any> {
  const out: Record<string, any> = data && typeof data === 'object' && !Array.isArray(data) ? data : {}
  for (const f of fields) {
    const v = out[f.key]
    if (f.type === 'object') out[f.key] = normalize(f.fields ?? [], v)
    else if (f.type === 'list') out[f.key] = Array.isArray(v) ? v.map(row => normalize(f.fields ?? [], row)) : emptyValue(f)
    else if (v === undefined || v === null) out[f.key] = emptyValue(f)
  }
  return out
}

const isEmpty = (v: any) => v === null || v === undefined || v === '' || (Array.isArray(v) && v.length === 0)

/** Validate data; strict enforces required + list min (render time). */
export function validate(fields: Field[], data: any, strict: boolean, path = '', errs: Err[] = []): Err[] {
  const known = new Set(fields.map(f => f.key))
  for (const k of Object.keys(data ?? {})) if (!known.has(k)) errs.push({ path: join(path, k), message: 'unknown field' })
  for (const f of fields) {
    const p = join(path, f.key)
    const v = data?.[f.key]
    if (isEmpty(v)) { if (strict && required(f)) errs.push({ path: p, message: 'required' }); continue }
    switch (f.type) {
      case 'text': case 'textarea': case 'asset': case 'sequence': case 'select':
        if (typeof v !== 'string') { errs.push({ path: p, message: 'must be a string' }); break }
        if (f.type === 'text' && f.pattern && !new RegExp(f.pattern).test(v)) errs.push({ path: p, message: 'does not match pattern' })
        if (f.type === 'select' && !f.options?.some(o => o.value === v)) errs.push({ path: p, message: 'not one of the options' })
        break
      case 'number': case 'money':
        if (typeof v !== 'number' || Number.isNaN(v)) { errs.push({ path: p, message: 'must be a number' }); break }
        if (f.min !== undefined && v < f.min) errs.push({ path: p, message: `must be ≥ ${f.min}` })
        if (f.max !== undefined && v > f.max) errs.push({ path: p, message: `must be ≤ ${f.max}` })
        break
      case 'date':
        if (typeof v !== 'string' || !dateRe.test(v) || Number.isNaN(Date.parse(v))) errs.push({ path: p, message: 'must be a date (YYYY-MM-DD)' })
        break
      case 'bool':
        if (typeof v !== 'boolean') errs.push({ path: p, message: 'must be true or false' })
        break
      case 'object':
        if (!v || typeof v !== 'object' || Array.isArray(v)) { errs.push({ path: p, message: 'must be an object' }); break }
        validate(f.fields ?? [], v, strict, p, errs)
        break
      case 'list':
        if (!Array.isArray(v)) { errs.push({ path: p, message: 'must be a list' }); break }
        if (strict && f.min !== undefined && v.length < f.min) errs.push({ path: p, message: `needs at least ${f.min} rows` })
        if (f.max !== undefined && v.length > f.max) errs.push({ path: p, message: `at most ${f.max} rows` })
        v.forEach((row, i) => validate(f.fields ?? [], row, strict, `${p}.${i}`, errs))
        break
    }
  }
  return errs
}

/** Keep only the keys the schema declares (recursively) — drops leftovers from fragments or older schemas. */
export function pick(fields: Field[], data: any): Record<string, any> {
  const out: Record<string, any> = {}
  if (!data || typeof data !== 'object') return out
  for (const f of fields) {
    const v = data[f.key]
    if (v === undefined) continue
    if (f.type === 'object') out[f.key] = pick(f.fields ?? [], v)
    else if (f.type === 'list') out[f.key] = Array.isArray(v) ? v.map(row => pick(f.fields ?? [], row)) : v
    else out[f.key] = v
  }
  return out
}

/** Deep clone that also works on Svelte state proxies. */
export const clone = <T>(v: T): T => JSON.parse(JSON.stringify(v ?? null)) ?? ({} as T)

const join = (a: string, b: string) => (a ? `${a}.${b}` : b)

/** Object fields tagged with a fragment kind, at any depth. */
export function fragmentKinds(fields: Field[]): string[] {
  const out = new Set<string>()
  const walk = (fs: Field[]) => fs.forEach(f => { if (f.type === 'object' && f.fragment) out.add(f.fragment); if (f.fields) walk(f.fields) })
  walk(fields)
  return [...out]
}

/** First object field with the given fragment kind. */
export function findByKind(fields: Field[], kind: string): Field | undefined {
  for (const f of fields) {
    if (f.type === 'object' && f.fragment === kind) return f
    if (f.fields) { const r = findByKind(f.fields, kind); if (r) return r }
  }
  return undefined
}
