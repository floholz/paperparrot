<script lang="ts">
  import { pb, uid, loadTemplates, starters, createFromStarter, errMsg, fmtDate, type Template, type Starter } from '../lib/pb'
  import { go } from '../lib/router'
  import Modal from '../lib/Modal.svelte'

  let templates = $state<Template[]>([])
  let counts = $state<Record<string, number>>({})
  let error = $state(''), loading = $state(true)
  let showNew = $state(false)
  let starterList = $state<Starter[]>([])
  let busy = $state('')

  async function load() {
    loading = true
    try {
      const [t, docs] = await Promise.all([
        loadTemplates(),
        pb.collection('documents').getFullList({ fields: 'id,template' }),
      ])
      templates = t
      const c: Record<string, number> = {}
      for (const d of docs) c[d.template] = (c[d.template] ?? 0) + 1
      counts = c
    } catch (e) { error = errMsg(e) } finally { loading = false }
  }
  load()

  async function openNew() {
    showNew = true
    if (!starterList.length) starterList = await starters().catch(() => [])
  }

  async function createBlank() {
    busy = 'blank'
    try {
      const rec = await pb.collection<Template>('templates').create({
        user: uid(), name: uniqueName('New template'), page: 'A4', locale: 'de-AT',
        html: '<h1>{{.title}}</h1>\n<p>{{nl2br .body}}</p>\n',
        css: '',
        schema: { fields: [{ key: 'title', type: 'text', label: 'Title' }, { key: 'body', type: 'textarea', label: 'Text' }] },
        theme: {}, sample: { title: 'Hello', body: 'Edit the HTML, CSS, schema and theme on the left.\nThe preview updates as you type.' },
        title_format: '{{.title}}',
      })
      go(`/templates/${rec.id}`)
    } catch (e) { error = errMsg(e); busy = '' }
  }

  async function createStarter(id: string) {
    busy = id
    try { const rec = await createFromStarter(id); go(`/templates/${rec.id}`) }
    catch (e) { error = errMsg(e); busy = '' }
  }

  async function duplicate(t: Template) {
    busy = t.id
    try {
      const rec = await pb.collection<Template>('templates').create({
        user: uid(), name: uniqueName(t.name + ' (copy)'), html: t.html, css: t.css, schema: t.schema, theme: t.theme,
        sample: t.sample, title_format: t.title_format, page: t.page, locale: t.locale,
      })
      go(`/templates/${rec.id}`)
    } catch (e) { error = errMsg(e); busy = '' }
  }

  async function remove(t: Template) {
    if ((counts[t.id] ?? 0) > 0) { error = `"${t.name}" is used by ${counts[t.id]} document(s). Delete those first.`; return }
    if (!confirm(`Delete template "${t.name}"?`)) return
    try { await pb.collection('templates').delete(t.id); await load() } catch (e) { error = errMsg(e) }
  }

  function uniqueName(base: string) {
    let name = base
    for (let i = 2; templates.some(t => t.name === name); i++) name = `${base} (${i})`
    return name
  }
</script>

<div class="row" style="margin-bottom:1rem">
  <h1 style="margin:0">Templates</h1>
  <span class="spacer"></span>
  <button class="primary" onclick={openNew}>+ New template</button>
</div>
{#if error}<div class="notice error" style="margin-bottom:1rem">{error}</div>{/if}

{#if loading}
  <p class="muted">Loading…</p>
{:else if !templates.length}
  <div class="empty">No templates yet. Create one from a starter to get going.</div>
{:else}
  <div class="grid">
    {#each templates as t (t.id)}
      <div class="card">
        <h2><a href={`#/templates/${t.id}`}>{t.name}</a></h2>
        <div class="muted small">{t.page} · {t.locale} · v{t.version} · {counts[t.id] ?? 0} document{(counts[t.id] ?? 0) === 1 ? '' : 's'}</div>
        <div class="muted tiny">updated {fmtDate(t.updated)}</div>
        <div class="row actions">
          <button onclick={() => go(`/templates/${t.id}`)}>Edit</button>
          <button onclick={() => go(`/documents?new=${t.id}`)} title="Create a document from this template">New document</button>
          <button onclick={() => duplicate(t)} disabled={busy === t.id}>Duplicate</button>
          <button class="danger" onclick={() => remove(t)} disabled={(counts[t.id] ?? 0) > 0} title={(counts[t.id] ?? 0) > 0 ? 'Used by documents' : 'Delete'}>Delete</button>
        </div>
      </div>
    {/each}
  </div>
{/if}

{#if showNew}
  <Modal title="New template" onclose={() => (showNew = false)}>
    <div class="col">
      <button onclick={createBlank} disabled={!!busy}><strong>Blank</strong> — a minimal HTML template with a two-field schema</button>
      {#each starterList as s}
        <button onclick={() => createStarter(s.id)} disabled={!!busy} style="text-align:left; white-space:normal">
          <strong>{s.name}</strong> — {s.description}
          <div class="muted tiny">{s.page} · {s.locale}</div>
        </button>
      {/each}
    </div>
  </Modal>
{/if}
