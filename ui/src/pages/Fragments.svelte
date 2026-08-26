<script lang="ts">
  import { pb, uid, loadFragments, loadTemplates, errMsg, type Fragment, type Template } from '../lib/pb'
  import { checkSchema, findByKind, fragmentKinds, normalize, clone, type Field } from '../lib/schema'
  import SchemaForm from '../lib/SchemaForm.svelte'
  import Modal from '../lib/Modal.svelte'

  let fragments = $state<Fragment[]>([])
  let templates = $state<Template[]>([])
  let error = $state(''), loading = $state(true)

  // kinds known from template schemas → their sub-schema (first template that declares it)
  let kinds = $state<Record<string, Field | undefined>>({})

  async function load() {
    loading = true
    try {
      ;[fragments, templates] = await Promise.all([loadFragments(), loadTemplates()])
      const k: Record<string, Field | undefined> = {}
      for (const t of templates) {
        const { schema } = checkSchema(t.schema ?? { fields: [] })
        for (const kind of fragmentKinds(schema?.fields ?? [])) k[kind] ??= findByKind(schema?.fields ?? [], kind)
      }
      for (const f of fragments) k[f.kind] ??= undefined
      kinds = k
    } catch (e) { error = errMsg(e) } finally { loading = false }
  }
  load()

  const grouped = $derived.by(() => {
    const g: Record<string, Fragment[]> = {}
    for (const f of fragments) (g[f.kind] ??= []).push(f)
    return Object.entries(g).sort(([a], [b]) => a.localeCompare(b))
  })

  // editor modal
  let edit = $state<{ id: string; name: string; kind: string; data: Record<string, any>; json: string; asJson: boolean } | undefined>()
  let busy = $state(false)
  const subFields = $derived(edit ? kinds[edit.kind]?.fields ?? [] : [])

  function open(f?: Fragment, kind = '') {
    const k = f?.kind ?? kind
    const fields = kinds[k]?.fields ?? []
    const data = normalize(fields, clone(f?.data ?? {}))
    edit = { id: f?.id ?? '', name: f?.name ?? '', kind: k, data, json: JSON.stringify(data, null, 2), asJson: !fields.length }
  }
  function toggleJson() {
    if (!edit) return
    if (edit.asJson) {
      try { edit.data = normalize(subFields, JSON.parse(edit.json || '{}')); edit.asJson = false } catch (e: any) { error = 'Invalid JSON: ' + e.message }
    } else { edit.json = JSON.stringify($state.snapshot(edit.data), null, 2); edit.asJson = true }
  }
  async function save() {
    if (!edit) return
    let data = edit.data
    if (edit.asJson) { try { data = JSON.parse(edit.json || '{}') } catch (e: any) { error = 'Invalid JSON: ' + e.message; return } }
    if (!edit.name.trim() || !edit.kind.trim()) { error = 'Name and kind are required'; return }
    busy = true; error = ''
    try {
      const payload = { user: uid(), name: edit.name.trim(), kind: edit.kind.trim(), data: $state.snapshot(data) }
      if (edit.id) await pb.collection('fragments').update(edit.id, payload)
      else await pb.collection('fragments').create(payload)
      edit = undefined
      await load()
    } catch (e) { error = errMsg(e) } finally { busy = false }
  }
  async function remove(f: Fragment) {
    if (!confirm(`Delete ${f.kind} "${f.name}"?`)) return
    try { await pb.collection('fragments').delete(f.id); fragments = fragments.filter(x => x.id !== f.id) } catch (e) { error = errMsg(e) }
  }
  const summary = (f: Fragment) => Object.values(f.data ?? {}).filter(v => typeof v === 'string' && v).slice(0, 3).join(' · ')
</script>

<div class="row" style="margin-bottom:1rem">
  <h1 style="margin:0">Fragments</h1>
  <span class="muted small">Reusable data blocks — a client, your sender details. Inserting one copies its values into the document.</span>
  <span class="spacer"></span>
  <button class="primary" onclick={() => open(undefined, Object.keys(kinds)[0] ?? '')}>+ New fragment</button>
</div>
{#if error}<div class="notice error" style="margin-bottom:1rem">{error}</div>{/if}

{#if loading}
  <p class="muted">Loading…</p>
{:else if !fragments.length}
  <div class="empty">No fragments yet. {Object.keys(kinds).length ? `Your templates use: ${Object.keys(kinds).join(', ')}.` : 'Tag an object field in a template schema with "fragment": "recipient" to use them.'}</div>
{:else}
  {#each grouped as [kind, list] (kind)}
    <div class="panel">
      <div class="row" style="margin-bottom:.4rem"><h2 style="margin:0">{kind}</h2><span class="spacer"></span><button onclick={() => open(undefined, kind)}>+ New {kind}</button></div>
      <table>
        <tbody>
          {#each list as f (f.id)}
            <tr>
              <td style="width:30%"><strong>{f.name}</strong></td>
              <td class="muted small">{summary(f)}</td>
              <td class="num"><button class="link" onclick={() => open(f)}>Edit</button> · <button class="link danger" onclick={() => remove(f)}>Delete</button></td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/each}
{/if}

{#if edit}
  <Modal title={edit.id ? 'Edit fragment' : 'New fragment'} onclose={() => (edit = undefined)} dismissable={false}>
    <div class="col">
      <div class="row">
        <label class="field" style="flex:1">Name <input bind:value={edit.name} placeholder="Example FlexCo" /></label>
        <label class="field">Kind
          <input bind:value={edit.kind} list="kinds" placeholder="recipient" onchange={() => { if (edit) edit.data = normalize(kinds[edit.kind]?.fields ?? [], edit.data) }} />
          <datalist id="kinds">{#each Object.keys(kinds) as k}<option value={k}></option>{/each}</datalist>
        </label>
      </div>
      {#if edit.asJson}
        <textarea bind:value={edit.json} rows="10" style="font-family:ui-monospace,monospace; font-size:.85rem"></textarea>
      {:else}
        <SchemaForm fields={subFields} bind:data={edit.data} />
      {/if}
      <div class="row">
        <button class="link" onclick={toggleJson} disabled={!subFields.length && !edit.asJson}>{edit.asJson ? (subFields.length ? 'Edit as form' : 'No template declares this kind — JSON only') : 'Edit as JSON'}</button>
        <span class="spacer"></span>
        <button onclick={() => (edit = undefined)}>Cancel</button>
        <button class="primary" onclick={save} disabled={busy}>Save</button>
      </div>
    </div>
  </Modal>
{/if}
