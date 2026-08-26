<script lang="ts">
  import { pb, uid, loadFonts, builtinFonts, errMsg, type Font, type Family } from '../lib/pb'

  let builtin = $state<Family[]>([])
  let fonts = $state<Font[]>([])
  let error = $state(''), busy = $state(false)
  let family = $state(''), weight = $state(400), style = $state<'normal' | 'italic'>('normal')
  let file = $state<HTMLInputElement | undefined>()

  async function load() {
    try { ;[builtin, fonts] = await Promise.all([builtinFonts(), loadFonts()]) } catch (e) { error = errMsg(e) }
  }
  load()

  async function upload(e: SubmitEvent) {
    e.preventDefault()
    const f = file?.files?.[0]
    if (!f) return
    busy = true; error = ''
    try {
      const fd = new FormData()
      fd.append('user', uid()); fd.append('family', family.trim()); fd.append('weight', String(weight)); fd.append('style', style); fd.append('file', f)
      await pb.collection('fonts').create(fd)
      if (file) file.value = ''
      await load()
    } catch (err) { error = errMsg(err) } finally { busy = false }
  }
  async function remove(f: Font) {
    if (!confirm(`Delete ${f.family} ${f.weight} ${f.style}?`)) return
    try { await pb.collection('fonts').delete(f.id); fonts = fonts.filter(x => x.id !== f.id) } catch (e) { error = errMsg(e) }
  }
  const fmtWeight = (w: string) => (w.includes(' ') ? `variable (${w})` : w)
</script>

<h1>Fonts</h1>
<p class="muted small">Fonts are inlined into every preview and PDF, so what you see is what gets embedded. Pick them in a template's <em>Theme</em> tab.</p>
{#if error}<div class="notice error" style="margin-bottom:1rem">{error}</div>{/if}

<div class="panel">
  <h2>Built-in</h2>
  <table>
    <thead><tr><th>Family</th><th>Weights</th><th>Styles</th></tr></thead>
    <tbody>
      {#each builtin as b (b.family)}
        <tr><td>{b.family}</td><td class="muted">{b.weights.map(fmtWeight).join(', ')}</td><td class="muted">{b.styles.join(', ')}</td></tr>
      {/each}
    </tbody>
  </table>
</div>

<div class="panel">
  <h2>Uploaded</h2>
  {#if fonts.length}
    <table style="margin-bottom:1rem">
      <thead><tr><th>Family</th><th>Weight</th><th>Style</th><th>File</th><th></th></tr></thead>
      <tbody>
        {#each fonts as f (f.id)}
          <tr><td>{f.family}</td><td>{f.weight}</td><td>{f.style}</td><td class="muted small">{f.file}</td><td class="num"><button class="link danger" onclick={() => remove(f)}>Delete</button></td></tr>
        {/each}
      </tbody>
    </table>
  {:else}
    <p class="muted small">No uploaded fonts.</p>
  {/if}
  <form class="row" onsubmit={upload}>
    <label class="field">Family <input bind:value={family} required placeholder="Lora" /></label>
    <label class="field">Weight <input type="number" bind:value={weight} min="100" max="900" step="100" required style="width:6rem" /></label>
    <label class="field">Style <select bind:value={style}><option value="normal">normal</option><option value="italic">italic</option></select></label>
    <label class="field">File (woff2 / ttf / otf, ≤ 2 MB) <input type="file" bind:this={file} accept=".woff2,.woff,.ttf,.otf" required /></label>
    <button class="primary" disabled={busy} style="align-self:flex-end">Upload</button>
  </form>
</div>
