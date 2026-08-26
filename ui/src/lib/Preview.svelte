<script lang="ts">
  import { preview, previewPdf, type PreviewBody } from './pb'

  /** Live preview: debounced POST /api/pp/preview into a sandboxed srcdoc iframe, or the real PDF. */
  let { body, canPdf = false, title = '' }: { body: PreviewBody; canPdf?: boolean; title?: string } = $props()

  let mode = $state<'html' | 'pdf'>('html')
  let html = $state('')
  let pdfUrl = $state('')
  let error = $state('')
  let busy = $state(false)
  let stale = $state(false)
  let iframe = $state<HTMLIFrameElement | undefined>()
  let timer: ReturnType<typeof setTimeout> | undefined
  let ctrl: AbortController | undefined
  let scrollY = 0

  async function refresh(json: string) {
    ctrl?.abort()
    ctrl = new AbortController()
    busy = true
    try {
      const b = JSON.parse(json) as PreviewBody
      if (mode === 'pdf') {
        const blob = await previewPdf(b, ctrl.signal)
        if (pdfUrl) URL.revokeObjectURL(pdfUrl)
        pdfUrl = URL.createObjectURL(blob)
      } else {
        try { scrollY = iframe?.contentWindow?.scrollY ?? scrollY } catch { /* cross-origin */ }
        html = await preview(b, ctrl.signal)
      }
      error = ''
      stale = false
    } catch (e: any) {
      if (e?.name === 'AbortError') return
      error = e?.message ?? String(e)
    } finally { busy = false }
  }

  // Debounce on any change of the body (deep, via JSON) or the mode.
  $effect(() => {
    const json = JSON.stringify(body)
    const m = mode
    void m
    stale = true
    clearTimeout(timer)
    timer = setTimeout(() => refresh(json), 350)
    return () => clearTimeout(timer)
  })

  // Scale the sheet down so a full page width fits the pane (zoom keeps layout, only display shrinks).
  let frame = $state<HTMLDivElement | undefined>()
  function fit() {
    try {
      const d = iframe?.contentDocument
      if (!d?.body || !frame) return
      const html = d.documentElement as HTMLElement
      html.style.zoom = '1'
      const sheet = d.body.offsetWidth + 24
      const scale = Math.min(1, frame.clientWidth / sheet)
      html.style.zoom = String(scale)
    } catch { /* cross-origin */ }
  }
  $effect(() => {
    if (!frame) return
    const ro = new ResizeObserver(fit)
    ro.observe(frame)
    return () => ro.disconnect()
  })
  const restoreScroll = () => { fit(); try { iframe?.contentWindow?.scrollTo(0, scrollY) } catch { /* cross-origin */ } }

  function openPdf() {
    if (mode !== 'pdf' || !pdfUrl) return
    const a = document.createElement('a')
    a.href = pdfUrl; a.download = (title || 'preview') + '.pdf'; a.click()
  }
</script>

<div class="preview" class:busy>
  <div class="bar">
    <div class="seg row">
      <button type="button" class:on={mode === 'html'} onclick={() => (mode = 'html')}>Preview</button>
      <button type="button" class:on={mode === 'pdf'} onclick={() => (mode = 'pdf')} disabled={!canPdf} title={canPdf ? 'Render with Chromium' : 'PDF rendering unavailable on this instance'}>PDF</button>
    </div>
    <span class="muted small status">{busy ? 'rendering…' : stale ? 'pending…' : ''}</span>
    <span class="spacer"></span>
    {#if mode === 'pdf' && pdfUrl}<button type="button" onclick={openPdf}>Download</button>{/if}
    <button type="button" onclick={() => refresh(JSON.stringify(body))} title="Refresh">↻</button>
  </div>
  <div class="frame" bind:this={frame}>
    {#if mode === 'pdf'}
      {#if pdfUrl}<iframe title="PDF preview" src={pdfUrl}></iframe>{/if}
    {:else}
      <iframe title="Preview" bind:this={iframe} sandbox="allow-same-origin" srcdoc={html} onload={restoreScroll}></iframe>
    {/if}
    {#if error}<pre class="error overlay">{error}</pre>{/if}
  </div>
</div>

<style>
  .preview { display: flex; flex-direction: column; height: 100%; min-height: 24rem; border: 1px solid var(--line); border-radius: 8px; overflow: hidden; background: var(--panel); }
  .bar { display: flex; align-items: center; gap: .5rem; padding: .4rem .6rem; border-bottom: 1px solid var(--line); }
  .bar .spacer { flex: 1; }
  .frame { position: relative; flex: 1; background: #9a9a9a; }
  iframe { width: 100%; height: 100%; border: 0; display: block; background: #9a9a9a; }
  .overlay { position: absolute; left: .6rem; right: .6rem; top: .6rem; margin: 0; padding: .6rem .8rem; background: var(--panel); border: 1px solid var(--danger); border-radius: 6px; font-size: .8rem; max-height: 40%; overflow: auto; white-space: pre-wrap; }
  .busy .frame::after { content: ''; position: absolute; left: 0; top: 0; height: 2px; width: 100%; background: var(--accent); animation: pulse 1s infinite alternate; }
  @keyframes pulse { from { opacity: .3 } to { opacity: 1 } }
</style>
