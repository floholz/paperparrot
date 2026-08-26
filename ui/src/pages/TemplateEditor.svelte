<script lang="ts">
  import { pb, loadTemplate, loadFonts, builtinFonts, fileToken, fileUrl, originalName, errMsg, PAGES, LOCALES, type Template, type Page, type Locale, type PreviewBody } from '../lib/pb'
  import { parseSchema } from '../lib/schema'
  import { go } from '../lib/router'
  import CodeEditor from '../lib/CodeEditor.svelte'
  import Preview from '../lib/Preview.svelte'
  import ThemeForm from '../lib/ThemeForm.svelte'

  let { id, canPdf = false }: { id: string; canPdf?: boolean } = $props()

  type Tab = 'html' | 'css' | 'schema' | 'theme' | 'sample' | 'assets' | 'settings'
  const TABS: [Tab, string][] = [['html', 'HTML'], ['css', 'CSS'], ['schema', 'Schema'], ['theme', 'Theme'], ['sample', 'Sample data'], ['assets', 'Assets'], ['settings', 'Settings']]
  let tab = $state<Tab>('html')

  let tpl = $state<Template | undefined>()
  let name = $state(''), html = $state(''), css = $state(''), schemaText = $state(''), sampleText = $state('')
  let theme = $state<Record<string, string>>({})
  let titleFormat = $state(''), page = $state<Page>('A4'), locale = $state<Locale>('de-AT')
  let assets = $state<string[]>([])
  let saved = $state('')       // JSON snapshot of the saved state, for dirty tracking
  let error = $state(''), notice = $state(''), busy = $state(false)
  let families = $state<string[]>([])
  let token = $state('')

  const pretty = (v: any) => (v == null || v === '' ? '' : JSON.stringify(v, null, 2))
  const snapshot = () => JSON.stringify({ name, html, css, schemaText, theme, sampleText, titleFormat, page, locale })
  const dirty = $derived(!!tpl && snapshot() !== saved)

  async function load() {
    try {
      tpl = await loadTemplate(id)
      name = tpl.name; html = tpl.html ?? ''; css = tpl.css ?? ''
      schemaText = pretty(tpl.schema) || '{\n  "fields": []\n}'
      sampleText = pretty(tpl.sample) || '{}'
      theme = { ...(tpl.theme ?? {}) }
      titleFormat = tpl.title_format ?? ''; page = tpl.page || 'A4'; locale = tpl.locale || 'de-AT'
      assets = tpl.assets ?? []
      saved = snapshot()
      const [b, u] = await Promise.all([builtinFonts().catch(() => []), loadFonts().catch(() => [])])
      families = [...new Set([...b.map(f => f.family), ...u.map(f => f.family)])].sort()
      token = await fileToken().catch(() => '')
    } catch (e) { error = errMsg(e) }
  }
  load()

  // parsed schema / sample with inline errors
  const schemaParsed = $derived(parseSchema(schemaText))
  const sampleParsed = $derived.by(() => { try { return { data: JSON.parse(sampleText || '{}') } } catch (e: any) { return { error: 'Invalid JSON: ' + e.message } } })

  let lastGood = $state<{ schema: any; data: any }>({ schema: undefined, data: {} })
  $effect(() => {
    if (schemaParsed.schema) lastGood.schema = schemaParsed.schema
    if (sampleParsed.data) lastGood.data = sampleParsed.data
  })
  const body = $derived<PreviewBody>({ template: id, html, css, schema: lastGood.schema, theme, page, locale, data: lastGood.data })

  async function save() {
    if (!tpl) return
    if (schemaParsed.error) { error = 'Schema: ' + schemaParsed.error; tab = 'schema'; return }
    if (sampleParsed.error) { error = 'Sample data: ' + sampleParsed.error; tab = 'sample'; return }
    busy = true; error = ''; notice = ''
    try {
      const t = await pb.collection<Template>('templates').update(id, {
        name, html, css, schema: schemaParsed.schema, theme, sample: sampleParsed.data, title_format: titleFormat, page, locale,
      })
      tpl = t
      saved = snapshot()
      notice = `Saved (v${t.version})`
      setTimeout(() => (notice = ''), 2500)
    } catch (e) { error = errMsg(e) } finally { busy = false }
  }

  async function uploadAssets(e: Event) {
    const input = e.target as HTMLInputElement
    if (!input.files?.length || !tpl) return
    const fd = new FormData()
    for (const f of input.files) fd.append('assets+', f)
    busy = true; error = ''
    try { const t = await pb.collection<Template>('templates').update(id, fd); tpl = t; assets = t.assets }
    catch (err) { error = errMsg(err) } finally { busy = false; input.value = '' }
  }
  async function removeAsset(a: string) {
    if (!confirm(`Remove ${originalName(a)}?`)) return
    try { const t = await pb.collection<Template>('templates').update(id, { 'assets-': [a] }); tpl = t; assets = t.assets }
    catch (err) { error = errMsg(err) }
  }
  const isImage = (a: string) => /\.(png|jpe?g|gif|webp|svg)$/i.test(a)

  function onKey(e: KeyboardEvent) {
    if ((e.ctrlKey || e.metaKey) && e.key === 's') { e.preventDefault(); save() }
  }
  const beforeUnload = (e: BeforeUnloadEvent) => { if (dirty) e.preventDefault() }
</script>

<svelte:window onkeydown={onKey} onbeforeunload={beforeUnload} />

{#if !tpl}
  <p class="muted">{error || 'Loading…'}</p>
{:else}
  <div class="editor-head">
    <a href="#/templates" class="muted small">← Templates</a>
    <input class="title" bind:value={name} placeholder="Template name" />
    <span class="tag" title="Revision — bumped when HTML, CSS, schema, theme, page or locale change">v{tpl.version}</span>
    {#if dirty}<span class="tag accent">unsaved</span>{/if}
    <span class="spacer"></span>
    {#if notice}<span class="muted small">{notice}</span>{/if}
    <button onclick={() => go(`/documents?new=${id}`)}>New document</button>
    <button class="primary" onclick={save} disabled={busy || !dirty}>Save</button>
  </div>
  {#if error}<div class="notice error" style="margin-bottom:.6rem">{error}</div>{/if}

  <div class="split">
    <div class="pane">
      <div class="tabs">
        {#each TABS as [t, l]}<button class:on={tab === t} onclick={() => (tab = t)}>{l}{#if t === 'schema' && schemaParsed.error} ⚠{/if}{#if t === 'sample' && sampleParsed.error} ⚠{/if}</button>{/each}
      </div>
      {#if tab === 'html'}
        <div class="grow"><CodeEditor bind:value={html} lang="html" /></div>
        <p class="muted tiny" style="margin:.4rem 0 0">Go <code>html/template</code> syntax with <code>.</code> as the document data. Functions: money, num, date, sum, add/sub/mul/div, nl2br, default, upper/lower/title, asset, theme.</p>
      {:else if tab === 'css'}
        <div class="grow"><CodeEditor bind:value={css} lang="css" /></div>
        <p class="muted tiny" style="margin:.4rem 0 0">Appended after the base stylesheet. Use <code>var(--pp-color-accent)</code>, <code>.box</code>, <code>.muted</code>, <code>.num</code>, <code>.cols</code>, <code>.page-break</code>.</p>
      {:else if tab === 'schema'}
        <div class="grow"><CodeEditor bind:value={schemaText} lang="json" /></div>
        {#if schemaParsed.error}<div class="notice error" style="margin-top:.4rem">{schemaParsed.error}</div>{:else}<p class="muted tiny" style="margin:.4rem 0 0">Types: text, textarea, number, money, date, bool, select, object, list, asset, sequence. The document form is generated from this.</p>{/if}
      {:else if tab === 'theme'}
        <div class="grow" style="overflow:auto"><ThemeForm bind:theme {families} /></div>
      {:else if tab === 'sample'}
        <div class="grow"><CodeEditor bind:value={sampleText} lang="json" /></div>
        {#if sampleParsed.error}<div class="notice error" style="margin-top:.4rem">{sampleParsed.error}</div>{:else}<p class="muted tiny" style="margin:.4rem 0 0">Used only for this preview. Documents start empty (with defaults).</p>{/if}
      {:else if tab === 'assets'}
        <div class="grow" style="overflow:auto">
          <p class="muted small">Images used by the template, e.g. <code>&lt;img src="{'{{asset "logo.png"}}'}"&gt;</code>, or picked by <code>asset</code> fields in documents. Max 20 files, 5 MB each.</p>
          <input type="file" multiple accept="image/*,.svg" onchange={uploadAssets} disabled={busy} />
          {#if assets.length}
            <div class="grid" style="margin-top:.8rem">
              {#each assets as a (a)}
                <div class="card">
                  {#if isImage(a) && token}<img src={fileUrl(tpl, a, token)} alt={originalName(a)} style="max-height:80px; object-fit:contain; align-self:flex-start" />{/if}
                  <div class="small" title={a}>{originalName(a)}</div>
                  <div class="row actions"><button class="danger" onclick={() => removeAsset(a)}>Remove</button></div>
                </div>
              {/each}
            </div>
          {:else}<p class="muted small" style="margin-top:.8rem">No assets yet.</p>{/if}
        </div>
      {:else if tab === 'settings'}
        <div class="grow col" style="overflow:auto; max-width:32rem">
          <label class="field">Title format
            <input type="text" bind:value={titleFormat} placeholder={'{{.number}} {{.recipient.name}}'} />
            <span class="help">Go template for document titles and PDF file names.</span>
          </label>
          <label class="field">Page size <select bind:value={page}>{#each PAGES as p}<option value={p}>{p}</option>{/each}</select></label>
          <label class="field">Locale
            <select bind:value={locale}>{#each LOCALES as l}<option value={l}>{l}</option>{/each}</select>
            <span class="help">Number and date formatting of <code>money</code>, <code>num</code>, <code>date</code>.</span>
          </label>
          {#if tpl.sequences && Object.keys(tpl.sequences).length}
            <div class="field"><span>Sequence counters</span><pre class="small" style="margin:.2rem 0">{JSON.stringify(tpl.sequences, null, 1)}</pre></div>
          {/if}
        </div>
      {/if}
    </div>
    <div class="pane">
      <Preview {body} {canPdf} title={name} />
    </div>
  </div>
{/if}

<style>
  code { font-size: .85em; background: var(--hover); padding: .05rem .3rem; border-radius: 4px; }
</style>
