<script lang="ts">
  import SchemaForm from './SchemaForm.svelte'
  import { type Field, label, required, emptyData, emptyValue } from './schema'
  import { originalName } from './pb'

  /** Form generated from a field list; mutates `data` in place (a $state proxy from the page). */
  let { fields, data = $bindable(), errors = {}, path = '', assets = [], onUpload, onFragment, onSaveFragment, compact = false }: {
    fields: Field[]
    data: Record<string, any>
    errors?: Record<string, string>
    path?: string
    assets?: string[]
    onUpload?: (file: File) => Promise<string>
    onFragment?: (field: Field, path: string) => void
    onSaveFragment?: (field: Field, value: any) => void
    compact?: boolean
  } = $props()

  // nested objects/lists must exist before a child form binds into them
  $effect.pre(() => {
    for (const f of fields) if ((f.type === 'object' || f.type === 'list') && (data[f.key] === undefined || data[f.key] === null)) data[f.key] = emptyValue(f)
  })

  const p = (k: string) => (path ? `${path}.${k}` : k)
  const err = (k: string) => errors[p(k)] ?? ''

  const addRow = (f: Field) => { data[f.key] = [...(data[f.key] ?? []), emptyData(f.fields ?? [])] }
  const removeRow = (f: Field, i: number) => { data[f.key].splice(i, 1) }
  const moveRow = (f: Field, i: number, d: number) => {
    const arr = data[f.key]; const j = i + d
    if (j < 0 || j >= arr.length) return
    ;[arr[i], arr[j]] = [arr[j], arr[i]]
  }
  let uploading = $state('')
  async function upload(f: Field, e: Event) {
    const input = e.target as HTMLInputElement
    const file = input.files?.[0]
    if (!file || !onUpload) return
    uploading = f.key
    try { data[f.key] = await onUpload(file) } finally { uploading = ''; input.value = '' }
  }
</script>

<div class="sf">
  {#each fields as f (f.key)}
    {#if f.type === 'object'}
      <fieldset>
        <legend>
          {label(f)}
          {#if f.fragment && (onFragment || onSaveFragment)}
            <span class="legend-actions">
              {#if onFragment}<button type="button" onclick={() => onFragment(f, p(f.key))} title="Insert a saved {f.fragment}">Insert {f.fragment} ▾</button>{/if}
              {#if onSaveFragment}<button type="button" onclick={() => onSaveFragment(f, data[f.key])} title="Save the current values as a reusable {f.fragment}">Save as {f.fragment}</button>{/if}
            </span>
          {/if}
        </legend>
        {#if f.help}<div class="muted tiny">{f.help}</div>{/if}
        <SchemaForm fields={f.fields ?? []} bind:data={data[f.key]} {errors} path={p(f.key)} {assets} {onUpload} {onFragment} {onSaveFragment} compact />
      </fieldset>
    {:else if f.type === 'list'}
      <fieldset>
        <legend>{label(f)} <span class="muted">({(data[f.key] ?? []).length})</span></legend>
        {#if f.help}<div class="muted tiny">{f.help}</div>{/if}
        {#if err(f.key)}<div class="field-error">{err(f.key)}</div>{/if}
        {#each data[f.key] ?? [] as _row, i (i)}
          <div class="list-row">
            <SchemaForm fields={f.fields ?? []} bind:data={data[f.key][i]} {errors} path={`${p(f.key)}.${i}`} {assets} {onUpload} compact />
            <div class="row-actions">
              <button type="button" onclick={() => moveRow(f, i, -1)} disabled={i === 0} title="Move up">↑</button>
              <button type="button" onclick={() => moveRow(f, i, 1)} disabled={i === (data[f.key]?.length ?? 0) - 1} title="Move down">↓</button>
              <button type="button" class="danger" onclick={() => removeRow(f, i)} disabled={(f.min ?? 0) >= (data[f.key]?.length ?? 0)} title="Remove">✕</button>
            </div>
          </div>
        {/each}
        <div><button type="button" onclick={() => addRow(f)} disabled={f.max !== undefined && (data[f.key]?.length ?? 0) >= f.max}>+ Add {label(f).replace(/s$/, '').toLowerCase()}</button></div>
      </fieldset>
    {:else if f.type === 'bool'}
      <label class="field check"><input type="checkbox" bind:checked={data[f.key]} /> {label(f)}{#if f.help}<span class="muted tiny">{f.help}</span>{/if}</label>
    {:else}
      <label class="field" class:invalid={!!err(f.key)}>
        <span>{label(f)}{#if required(f)}<span class="muted"> *</span>{/if}</span>
        {#if f.type === 'textarea'}
          <textarea bind:value={data[f.key]} rows={compact ? 2 : 3} placeholder={f.placeholder}></textarea>
        {:else if f.type === 'number'}
          <input type="number" bind:value={data[f.key]} min={f.min} max={f.max} step={f.step ?? 'any'} placeholder={f.placeholder} />
        {:else if f.type === 'money'}
          <span class="adorn"><input type="number" bind:value={data[f.key]} min={f.min} max={f.max} step="0.01" placeholder="0.00" /><span class="muted">{f.currency ?? '€'}</span></span>
        {:else if f.type === 'date'}
          <input type="date" bind:value={data[f.key]} />
        {:else if f.type === 'select'}
          <select bind:value={data[f.key]}>
            {#if !required(f)}<option value="">—</option>{/if}
            {#each f.options ?? [] as o}<option value={o.value}>{o.label ?? o.value}</option>{/each}
          </select>
        {:else if f.type === 'sequence'}
          <input type="text" bind:value={data[f.key]} placeholder="assigned automatically" />
        {:else if f.type === 'asset'}
          <span class="adorn">
            <select bind:value={data[f.key]}>
              <option value="">—</option>
              {#each assets as a}<option value={a}>{originalName(a)}</option>{/each}
            </select>
            {#if onUpload}
              <input type="file" accept={f.accept} onchange={(e) => upload(f, e)} style="display:none" id={'up-' + p(f.key)} />
              <button type="button" onclick={() => document.getElementById('up-' + p(f.key))?.click()} disabled={uploading === f.key}>{uploading === f.key ? '…' : 'Upload'}</button>
            {/if}
          </span>
        {:else}
          <input type="text" bind:value={data[f.key]} placeholder={f.placeholder} pattern={f.pattern} />
        {/if}
        {#if err(f.key)}<span class="field-error">{err(f.key)}</span>{:else if f.help}<span class="help">{f.help}</span>{/if}
      </label>
    {/if}
  {/each}
</div>
