import PocketBase, { type RecordModel } from 'pocketbase'

export const pb = new PocketBase(window.location.origin)
pb.autoCancellation(false)

export const uid = () => pb.authStore.record?.id ?? ''

// ---- records -----------------------------------------------------------------

export type Page = 'A4' | 'Letter'
export type Locale = 'de-AT' | 'de-DE' | 'en'
export const PAGES: Page[] = ['A4', 'Letter']
export const LOCALES: Locale[] = ['de-AT', 'de-DE', 'en']

export interface Template extends RecordModel {
  name: string
  html: string
  css: string
  schema: any
  theme: Record<string, string> | null
  sample: any
  title_format: string
  page: Page
  locale: Locale
  sequences: Record<string, Record<string, number>> | null
  assets: string[]
  version: number
}

export interface Revision extends RecordModel {
  template: string
  version: number
}

export interface Document extends RecordModel {
  template: string
  title: string
  data: any
  expand?: { template?: Template }
}

export interface Render extends RecordModel {
  document: string
  revision: string
  data: any
  html: string
  pdf: string
  title: string
  expand?: { revision?: Revision }
}

export interface Fragment extends RecordModel {
  name: string
  kind: string
  data: any
}

export interface Font extends RecordModel {
  family: string
  weight: number
  style: 'normal' | 'italic'
  file: string
}

export interface Family { family: string; weights: string[]; styles: string[]; builtin: boolean }

export interface Starter {
  id: string
  name: string
  description: string
  page: Page
  locale: Locale
  title_format: string
}

export interface Status { registration: boolean; render: boolean; version: string }

// ---- custom API ----------------------------------------------------------------

const headers = () => ({ Authorization: pb.authStore.token, 'Content-Type': 'application/json' })

async function fail(res: Response): Promise<never> {
  let msg = `${res.status} ${res.statusText}`
  try {
    const j = await res.json()
    if (j?.message) msg = j.message
    if (j?.data?.errors) msg += '\n' + j.data.errors.map((e: any) => `${e.path}: ${e.message}`).join('\n')
  } catch { /* not json */ }
  throw new Error(msg)
}

export const status = (): Promise<Status> => pb.send('/api/pp/status', { method: 'GET' })
export const builtinFonts = (): Promise<Family[]> => pb.send('/api/pp/fonts/builtin', { method: 'GET' })
export const starters = (): Promise<Starter[]> => pb.send('/api/pp/starters', { method: 'GET' })
export const createFromStarter = (id: string): Promise<Template> => pb.send(`/api/pp/starters/${id}`, { method: 'POST' })
export const renderDocument = (id: string): Promise<Render> => pb.send(`/api/pp/documents/${id}/render`, { method: 'POST' })
export const duplicateDocument = (id: string): Promise<Document> => pb.send(`/api/pp/documents/${id}/duplicate`, { method: 'POST' })

/** Body of /preview: inline fields override the stored template. */
export interface PreviewBody {
  template?: string
  html?: string
  css?: string
  schema?: any
  theme?: any
  page?: Page
  locale?: Locale
  data: any
}

export async function preview(body: PreviewBody, signal?: AbortSignal): Promise<string> {
  const res = await fetch('/api/pp/preview', { method: 'POST', headers: headers(), body: JSON.stringify(body), signal })
  if (!res.ok) await fail(res)
  return res.text()
}

export async function previewPdf(body: PreviewBody, signal?: AbortSignal): Promise<Blob> {
  const res = await fetch('/api/pp/preview.pdf', { method: 'POST', headers: headers(), body: JSON.stringify(body), signal })
  if (!res.ok) await fail(res)
  return res.blob()
}

// ---- files -------------------------------------------------------------------

/** Assets and PDFs are protected: URLs need a short-lived file token of the owning user. */
export const fileUrl = (rec: RecordModel, name: string, token: string, download = false) =>
  pb.files.getURL(rec, name, download ? { token, download: true } : { token })
export const fileToken = () => pb.files.getToken()

/** "logo_Ab12Cd34Ef.png" → "logo.png" (PocketBase adds a random suffix on upload). */
export const originalName = (stored: string) => stored.replace(/_[a-zA-Z0-9]{10}(\.[^.]+)?$/, '$1')

// ---- loaders -----------------------------------------------------------------

export const loadTemplates = () => pb.collection<Template>('templates').getFullList({ sort: 'name' })
export const loadTemplate = (id: string) => pb.collection<Template>('templates').getOne(id)
export const loadDocuments = () => pb.collection<Document>('documents').getFullList({ sort: '-updated', expand: 'template', fields: 'id,title,template,updated,created,expand.template.name' })
export const loadDocument = (id: string) => pb.collection<Document>('documents').getOne(id, { expand: 'template' })
export const loadRenders = (doc: string) =>
  pb.collection<Render>('renders').getFullList({ filter: pb.filter('document = {:doc}', { doc }), sort: '-created', expand: 'revision', fields: 'id,title,pdf,created,revision,expand.revision.version' })
export const loadFragments = () => pb.collection<Fragment>('fragments').getFullList({ sort: 'kind,name' })
export const loadFonts = () => pb.collection<Font>('fonts').getFullList({ sort: 'family,weight' })

/** Error message from a PocketBase ClientResponseError or a plain Error. */
export function errMsg(err: any): string {
  const data = err?.data?.data
  if (data && typeof data === 'object' && Object.keys(data).length) {
    return Object.entries(data).map(([k, v]: any) => `${k}: ${v?.message ?? v}`).join('\n')
  }
  return err?.data?.message ?? err?.message ?? String(err)
}

export const fmtDate = (s: string) => (s ? new Date(s).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' }) : '')
