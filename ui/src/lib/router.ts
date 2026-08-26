/** Tiny hash router: "#/documents/abc" → { page: 'documents', id: 'abc' }. */
export interface Route { page: string; id: string; raw: string }

export function parse(hash: string): Route {
  const raw = (hash.startsWith('#') ? hash.slice(1) : hash) || '/templates'
  const [, page = 'templates', id = ''] = raw.split('/')
  return { page, id, raw }
}

export const go = (path: string) => { location.hash = path }
