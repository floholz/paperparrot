export type Theme = 'light' | 'dark'
const KEY = 'pp_theme'

const system = (): Theme => (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')
export const current = (): Theme => (localStorage.getItem(KEY) as Theme | null) ?? system()

export function apply(t: Theme | null) {
  if (t) { document.documentElement.dataset.theme = t; localStorage.setItem(KEY, t) }
  else { delete document.documentElement.dataset.theme; localStorage.removeItem(KEY) }
}
export const toggle = () => { const next: Theme = current() === 'dark' ? 'light' : 'dark'; apply(next); return next }

/** Subscribe to effective theme changes (toggle or system). Returns an unsubscribe fn. */
export function watch(cb: (t: Theme) => void): () => void {
  const mo = new MutationObserver(() => cb(current()))
  mo.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] })
  const mq = matchMedia('(prefers-color-scheme: dark)')
  const onMq = () => cb(current())
  mq.addEventListener('change', onMq)
  return () => { mo.disconnect(); mq.removeEventListener('change', onMq) }
}

// apply persisted choice as early as possible
apply(localStorage.getItem(KEY) as Theme | null)
