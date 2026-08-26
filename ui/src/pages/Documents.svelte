<script lang="ts">
  import { pb, uid, loadDocuments, loadTemplates, errMsg, fmtDate, type Document, type Template } from '../lib/pb'
  import { checkSchema, emptyData } from '../lib/schema'
  import { go } from '../lib/router'
  import Modal from '../lib/Modal.svelte'

  let docs = $state<Document[]>([])
  let templates = $state<Template[]>([])
  let renderCounts = $state<Record<string, number>>({})
  let error = $state(''), loading = $state(true)
  let filterTemplate = $state(''), search = $state('')
  let showNew = $state(false), busy = $state('')

  async function load() {
    loading = true
    try {
      const [d, t, r] = await Promise.all([
        loadDocuments(), loadTemplates(),
        pb.collection('renders').getFullList({ fields: 'id,document' }),
      ])
      docs = d; templates = t
      const c: Record<string, number> = {}
      for (const x of r) c[x.document] = (c[x.document] ?? 0) + 1
      renderCounts = c
    } catch (e) { error = errMsg(e) } finally { loading = false }
  }
  load().then(() => {
    // "#/documents?new=<templateId>" opens the picker (or creates directly)
    const q = new URLSearchParams(location.hash.split('?')[1] ?? '')
    const t = q.get('new')
    if (t !== null) { history.replaceState(null, '', '#/documents'); if (t && templates.some(x => x.id === t)) create(t); else showNew = true }
  })

  const filtered = $derived(docs.filter(d =>
    (!filterTemplate || d.template === filterTemplate) &&
    (!search || d.title.toLowerCase().includes(search.toLowerCase()))))

  async function create(templateId: string) {
    busy = templateId
    try {
      const tpl: Template = templates.find(t => t.id === templateId) ?? (await pb.collection<Template>('templates').getOne(templateId))
      const { schema } = checkSchema(tpl.schema ?? { fields: [] })
      const rec = await pb.collection<Document>('documents').create({ user: uid(), template: templateId, data: emptyData(schema?.fields ?? []) })
      go(`/documents/${rec.id}`)
    } catch (e) { error = errMsg(e); busy = '' }
  }
</script>

<div class="row" style="margin-bottom:1rem">
  <h1 style="margin:0">Documents</h1>
  <span class="spacer"></span>
  <button class="primary" onclick={() => (showNew = true)} disabled={!templates.length}>+ New document</button>
</div>
{#if error}<div class="notice error" style="margin-bottom:1rem">{error}</div>{/if}

<div class="toolbar">
  <input type="search" placeholder="Search titles…" bind:value={search} style="min-width:14rem" />
  <select bind:value={filterTemplate}>
    <option value="">All templates</option>
    {#each templates as t}<option value={t.id}>{t.name}</option>{/each}
  </select>
  <span class="muted small">{filtered.length} of {docs.length}</span>
</div>

{#if loading}
  <p class="muted">Loading…</p>
{:else if !docs.length}
  <div class="empty">No documents yet. {templates.length ? 'Create one from a template.' : 'Create a template first.'}</div>
{:else}
  <div class="panel table-wrap" style="padding:0">
    <table>
      <thead><tr><th>Title</th><th>Template</th><th>Updated</th><th class="num">Renders</th></tr></thead>
      <tbody>
        {#each filtered as d (d.id)}
          <tr class="clickable" onclick={() => go(`/documents/${d.id}`)}>
            <td><a href={`#/documents/${d.id}`} onclick={(e) => e.stopPropagation()}>{d.title || '(untitled)'}</a></td>
            <td class="muted">{d.expand?.template?.name ?? '—'}</td>
            <td class="muted small">{fmtDate(d.updated)}</td>
            <td class="num">{renderCounts[d.id] ?? 0}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
{/if}

{#if showNew}
  <Modal title="New document — pick a template" onclose={() => (showNew = false)}>
    <div class="col">
      {#each templates as t}
        <button onclick={() => create(t.id)} disabled={!!busy} style="text-align:left"><strong>{t.name}</strong> <span class="muted small">{t.page} · {t.locale}</span></button>
      {/each}
    </div>
  </Modal>
{/if}
