<script lang="ts">
  import { pb, uid, loadDocument, loadRenders, loadFragments, renderDocument, duplicateDocument, fileToken, fileUrl, errMsg, fmtDate, clockifySettings, clockifyEntries, type Document, type Template, type Render, type Fragment, type PreviewBody, type ClockifyEntries, type ClockifySettings } from '../lib/pb'
  import { defaultFilter, total, applyToRow, pickRate, projects, fmtHours, type Filter } from '../lib/clockify'
  import { checkSchema, normalize, validate, clone, pick, type Field } from '../lib/schema'
  import { go } from '../lib/router'
  import SchemaForm from '../lib/SchemaForm.svelte'
  import Preview from '../lib/Preview.svelte'
  import Modal from '../lib/Modal.svelte'

  let { id, canPdf = false }: { id: string; canPdf?: boolean } = $props()

  let doc = $state<Document | undefined>()
  let tpl = $state<Template | undefined>()
  let fields = $state<Field[]>([])
  let data = $state<Record<string, any>>({})
  let title = $state('')
  let saved = $state('')
  let error = $state(''), notice = $state(''), busy = $state('')
  let renders = $state<Render[]>([])
  let token = $state('')
  let tab = $state<'data' | 'renders'>('data')
  let serverErrors = $state<Record<string, string>>({})

  const snapshot = () => JSON.stringify({ data, title })
  const dirty = $derived(!!doc && snapshot() !== saved)
  const clientErrors = $derived(Object.fromEntries(validate(fields, data, false).map(e => [e.path, e.message])))
  const errors = $derived({ ...clientErrors, ...serverErrors })
  const missing = $derived(validate(fields, data, true).length)
  const body = $derived<PreviewBody>({ template: tpl?.id, data })

  async function load() {
    try {
      doc = await loadDocument(id)
      tpl = doc.expand?.template
      if (!tpl) throw new Error('Template not found')
      const { schema, error: se } = checkSchema(tpl.schema ?? { fields: [] })
      if (se) error = 'Template schema: ' + se
      fields = schema?.fields ?? []
      data = normalize(fields, clone(doc.data ?? {}))
      title = doc.title ?? ''
      saved = snapshot()
      token = await fileToken().catch(() => '')
      renders = await loadRenders(id)
    } catch (e) { error = errMsg(e) }
  }
  load()

  async function save(): Promise<boolean> {
    if (!doc) return false
    busy = 'save'; error = ''; serverErrors = {}
    try {
      const d = await pb.collection<Document>('documents').update(id, { data: pick(fields, $state.snapshot(data)), title }, { expand: 'template' })
      doc = d
      title = d.title
      saved = snapshot()
      flash('Saved')
      return true
    } catch (e) { error = errMsg(e); return false } finally { busy = '' }
  }

  async function render() {
    if (dirty && !(await save())) return
    busy = 'render'; error = ''; serverErrors = {}
    try {
      const r = await renderDocument(id)
      renders = await loadRenders(id)
      tab = 'renders'
      flash('Rendered')
      window.open(fileUrl(r, r.pdf, token || (token = await fileToken())), '_blank')
    } catch (e: any) {
      error = errMsg(e)
      const list = e?.data?.data?.errors ?? e?.data?.errors
      if (Array.isArray(list)) serverErrors = Object.fromEntries(list.map((x: any) => [x.path, x.message]))
    } finally { busy = '' }
  }

  async function duplicate() {
    if (dirty && !(await save())) return
    busy = 'dup'
    try { const d = await duplicateDocument(id); go(`/documents/${d.id}`) }
    catch (e) { error = errMsg(e); busy = '' }
  }

  async function remove() {
    if (!confirm(`Delete "${title}" and its ${renders.length} render(s)?`)) return
    try { await pb.collection('documents').delete(id); go('/documents') } catch (e) { error = errMsg(e) }
  }

  async function removeRender(r: Render) {
    if (!confirm(`Delete the render from ${fmtDate(r.created)}?`)) return
    try { await pb.collection('renders').delete(r.id); renders = renders.filter(x => x.id !== r.id) } catch (e) { error = errMsg(e) }
  }

  const pdfUrl = (r: Render, download = false) => fileUrl(r, r.pdf, token, download)

  // ---- assets: uploads from the document go to the template ----
  async function uploadAsset(file: File): Promise<string> {
    if (!tpl) throw new Error('no template')
    const fd = new FormData(); fd.append('assets+', file)
    const before = new Set(tpl.assets ?? [])
    const t = await pb.collection<Template>('templates').update(tpl.id, fd)
    tpl = t
    return (t.assets ?? []).find(a => !before.has(a)) ?? ''
  }

  // ---- fragments ----
  let picker = $state<{ field: Field; path: string; list: Fragment[] } | undefined>()
  async function openFragment(field: Field, path: string) {
    try {
      const all = await loadFragments()
      picker = { field, path, list: all.filter(f => f.kind === field.fragment) }
    } catch (e) { error = errMsg(e) }
  }
  function insertFragment(f: Fragment) {
    if (!picker) return
    setPath(data, picker.path, normalize(picker.field.fields ?? [], pick(picker.field.fields ?? [], clone(f.data ?? {}))))
    picker = undefined
  }
  async function saveFragment(field: Field, value: any) {
    const name = prompt(`Save this ${field.fragment} as…`, value?.name ?? '')
    if (!name) return
    try { await pb.collection('fragments').create({ user: uid(), name, kind: field.fragment, data: pick(field.fields ?? [], $state.snapshot(value)) }); flash(`Saved ${field.fragment} "${name}"`) }
    catch (e) { error = errMsg(e) }
  }
  // ---- Clockify: fetch once per click, filter and sum locally ----
  let cfg = $state<ClockifySettings | undefined>()
  let synced = $state<Record<string, { res: ClockifyEntries; filter: Filter }>>({})
  let clockifyNotes = $state<Record<string, string>>({})
  let filterDlg = $state<{ path: string; filter: Filter } | undefined>()

  async function syncClockify(field: Field, path: string, adjust: boolean) {
    if (adjust) {
      const s = synced[path]
      if (s) filterDlg = { path, filter: clone(s.filter) }
      return
    }
    if (busy) return
    const row = getPath(data, path)
    busy = 'clockify'; error = ''
    try {
      cfg ??= await clockifySettings()
      if (!cfg.connected) { error = 'Clockify is not connected — add your API key under Settings.'; return }
      const res = await clockifyEntries(row.from, row.to)
      // a re-sync keeps the filter chosen for this row
      const filter = synced[path]?.filter ?? defaultFilter(field, res.entries)
      synced[path] = { res, filter }
      applyClockify(path, filter)
    } catch (e) { error = errMsg(e) } finally { busy = '' }
  }

  function applyClockify(path: string, filter: Filter) {
    const s = synced[path]
    if (!s) return
    s.filter = filter
    const row = getPath(data, path)
    const t = total(s.res.entries, filter)
    const rate = pickRate(row, cfg?.rate ?? 0, s.res)
    applyToRow(row, t.seconds, rate)
    const parts = [fmtHours(t.seconds), `${t.count} of ${s.res.entries.length} entries`]
    if (filter.billable || filter.projects.length || filter.text) parts.push('filtered')
    if (s.res.running) parts.push(`${s.res.running} running timer skipped`)
    if (rate === null) parts.push('no hourly rate — set one in Settings or on the row')
    clockifyNotes[path] = parts.join(' · ')
  }

  const dlgTotal = $derived(filterDlg && synced[filterDlg.path] ? total(synced[filterDlg.path].res.entries, filterDlg.filter) : undefined)
  function toggleProject(id: string) {
    if (!filterDlg) return
    const ps = filterDlg.filter.projects.filter(p => p !== '-')
    filterDlg.filter.projects = ps.includes(id) ? ps.filter(p => p !== id) : [...ps, id]
  }

  function getPath(obj: any, path: string): any {
    return path.split('.').reduce((cur, p) => cur?.[p], obj)
  }
  function setPath(obj: any, path: string, value: any) {
    const parts = path.split('.')
    let cur = obj
    for (const p of parts.slice(0, -1)) cur = cur[p]
    cur[parts[parts.length - 1]] = value
  }

  function flash(msg: string) { notice = msg; setTimeout(() => (notice = ''), 2500) }
  function onKey(e: KeyboardEvent) { if ((e.ctrlKey || e.metaKey) && e.key === 's') { e.preventDefault(); save() } }
  const beforeUnload = (e: BeforeUnloadEvent) => { if (dirty) e.preventDefault() }
</script>

<svelte:window onkeydown={onKey} onbeforeunload={beforeUnload} />

{#if !doc || !tpl}
  <p class="muted">{error || 'Loading…'}</p>
{:else}
  <div class="editor-head">
    <a href="#/documents" class="muted small">← Documents</a>
    <input class="title" bind:value={title} placeholder="Title (derived from the template's title format)" />
    <a class="tag" href={`#/templates/${tpl.id}`} title="Template">{tpl.name} · v{tpl.version}</a>
    {#if dirty}<span class="tag accent">unsaved</span>{/if}
    <span class="spacer"></span>
    {#if notice}<span class="muted small">{notice}</span>{/if}
    <button onclick={duplicate} disabled={!!busy} title="Copy this document with the next number and today's date">Duplicate</button>
    <button class="danger" onclick={remove} disabled={!!busy}>Delete</button>
    <button onclick={save} disabled={!!busy || !dirty}>Save</button>
    <button class="primary" onclick={render} disabled={!!busy || !canPdf || missing > 0} title={!canPdf ? 'PDF rendering unavailable on this instance' : missing ? `${missing} field(s) still missing` : 'Save and render a PDF'}>{busy === 'render' ? 'Rendering…' : 'Render PDF'}</button>
  </div>
  {#if error}<div class="notice error" style="margin-bottom:.6rem">{error}</div>{/if}

  <div class="split">
    <div class="pane scroll">
      <div class="tabs">
        <button class:on={tab === 'data'} onclick={() => (tab = 'data')}>Data{#if missing} <span class="muted">({missing} missing)</span>{/if}</button>
        <button class:on={tab === 'renders'} onclick={() => (tab = 'renders')}>Renders ({renders.length})</button>
      </div>
      {#if tab === 'data'}
        <SchemaForm {fields} bind:data {errors} assets={tpl.assets ?? []} onUpload={uploadAsset} onFragment={openFragment} onSaveFragment={saveFragment} onClockify={syncClockify} {clockifyNotes} />
      {:else}
        {#if !renders.length}
          <p class="muted">No renders yet. Every “Render PDF” keeps an immutable copy here.</p>
        {:else}
          <table>
            <thead><tr><th>Rendered</th><th>Title</th><th>Template</th><th></th></tr></thead>
            <tbody>
              {#each renders as r (r.id)}
                <tr>
                  <td class="small">{fmtDate(r.created)}</td>
                  <td>{r.title}</td>
                  <td class="muted small">v{r.expand?.revision?.version ?? '?'}</td>
                  <td class="num">
                    <a href={pdfUrl(r)} target="_blank" rel="noopener">Open</a> ·
                    <a href={pdfUrl(r, true)}>Download</a> ·
                    <button class="link danger" onclick={() => removeRender(r)}>Delete</button>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        {/if}
      {/if}
    </div>
    <div class="pane">
      <Preview {body} {canPdf} {title} />
    </div>
  </div>
{/if}

{#if picker}
  <Modal title={`Insert ${picker.field.fragment}`} onclose={() => (picker = undefined)}>
    {#if !picker.list.length}
      <p class="muted">No saved {picker.field.fragment}s yet. Fill in the fields and use “Save as {picker.field.fragment}”, or manage them under <a href="#/fragments">Fragments</a>.</p>
    {:else}
      <div class="col">
        {#each picker.list as f (f.id)}
          <button onclick={() => insertFragment(f)} style="text-align:left"><strong>{f.name}</strong> <span class="muted small">{f.data?.name ?? ''}</span></button>
        {/each}
      </div>
    {/if}
  </Modal>
{/if}

{#if filterDlg && synced[filterDlg.path]}
  {@const s = synced[filterDlg.path]}
  <Modal title="Clockify filter" onclose={() => (filterDlg = undefined)}>
    <div class="col">
      <p class="muted small" style="margin:0">{s.res.from} – {s.res.to} · {s.res.entries.length} entries · changing the filter needs no new request.</p>
      <label class="field check"><input type="checkbox" bind:checked={filterDlg.filter.billable} /> Billable entries only</label>
      <div>
        <div class="muted small" style="margin-bottom:.3rem">Projects <span class="tiny">(none ticked = all)</span></div>
        {#each projects(s.res.entries) as p (p.id)}
          <label class="field check"><input type="checkbox" checked={filterDlg.filter.projects.includes(p.id)} onchange={() => toggleProject(p.id)} /> {p.label} <span class="muted small">{(p.seconds / 3600).toFixed(2)} h</span></label>
        {/each}
      </div>
      <label class="field">Description contains <input bind:value={filterDlg.filter.text} placeholder="e.g. Kundenportal" /></label>
      {#if dlgTotal}<div class="notice ok"><strong>{fmtHours(dlgTotal.seconds)}</strong> · {dlgTotal.count} entries</div>{/if}
      <div class="row" style="justify-content:flex-end">
        <button onclick={() => (filterDlg = undefined)}>Cancel</button>
        <button class="primary" onclick={() => { if (filterDlg) { applyClockify(filterDlg.path, clone(filterDlg.filter)); filterDlg = undefined } }}>Apply to row</button>
      </div>
    </div>
  </Modal>
{/if}
