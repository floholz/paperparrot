<script lang="ts">
  /** Form for exactly the theme tokens of SPEC.md §6b. Mutates `theme` in place. */
  let { theme = $bindable(), families = [] }: { theme: Record<string, string>; families?: string[] } = $props()

  const DEFAULTS: Record<string, string> = {
    'font-body': 'Inter', 'font-heading': '', 'font-size': '11',
    'color-text': '#111111', 'color-heading': '#111111', 'color-accent': '#111111', 'color-muted': '#666666', 'color-block-bg': '#e8e8e8',
    'margin': '2cm',
  }
  const LABELS: Record<string, string> = {
    'font-body': 'Body font', 'font-heading': 'Heading font', 'font-size': 'Font size (pt)',
    'color-text': 'Text', 'color-heading': 'Headings', 'color-accent': 'Accent', 'color-muted': 'Muted', 'color-block-bg': 'Block background',
    'margin': 'Page margin',
  }
  const set = (k: string, v: string) => { if (v === '' || v === DEFAULTS[k]) delete theme[k]; else theme[k] = v }
  const get = (k: string) => theme[k] ?? ''
  const OTHER = '__other__'
  let custom = $state<Record<string, boolean>>({})
  const isCustom = (k: string) => custom[k] || (get(k) !== '' && !families.includes(get(k)))
  function pickFont(k: string, v: string) {
    if (v === OTHER) { custom[k] = true; return }
    custom[k] = false; set(k, v)
  }
  const validColor = (v: string) => /^#[0-9a-fA-F]{6}$/.test(v)
</script>

<div class="theme-form">
  <h3>Fonts</h3>
  {#each ['font-body', 'font-heading'] as k}
    <label class="field">{LABELS[k]}
      <span class="row nowrap">
        <select value={isCustom(k) ? OTHER : get(k)} onchange={(e) => pickFont(k, (e.target as HTMLSelectElement).value)}>
          <option value="">{k === 'font-heading' ? 'same as body' : `default (${DEFAULTS[k]})`}</option>
          {#each families as f}<option value={f}>{f}</option>{/each}
          <option value={OTHER}>other…</option>
        </select>
        {#if isCustom(k)}<input type="text" placeholder="Family name" value={get(k)} oninput={(e) => set(k, (e.target as HTMLInputElement).value)} />{/if}
      </span>
    </label>
  {/each}
  <label class="field">{LABELS['font-size']}
    <input type="number" min="6" max="40" step="0.5" placeholder={DEFAULTS['font-size']} value={get('font-size')} oninput={(e) => set('font-size', (e.target as HTMLInputElement).value)} />
  </label>

  <h3>Colours</h3>
  <div class="colors">
    {#each ['color-text', 'color-heading', 'color-accent', 'color-muted', 'color-block-bg'] as k}
      <label class="field">{LABELS[k]}
        <span class="row nowrap">
          <input type="color" value={validColor(get(k)) ? get(k) : DEFAULTS[k]} oninput={(e) => set(k, (e.target as HTMLInputElement).value)} />
          <input type="text" placeholder={DEFAULTS[k]} value={get(k)} oninput={(e) => set(k, (e.target as HTMLInputElement).value)} style="width:7rem" />
        </span>
      </label>
    {/each}
  </div>

  <h3>Page</h3>
  <label class="field">{LABELS['margin']}
    <input type="text" placeholder={DEFAULTS['margin']} value={get('margin')} oninput={(e) => set('margin', (e.target as HTMLInputElement).value)} />
    <span class="help">CSS margin shorthand, e.g. <code>2cm</code> or <code>2cm 2.5cm</code> or <code>25mm 20mm 20mm</code>.</span>
  </label>
  <p class="muted small" style="margin-top:1rem">Tokens are available in the template CSS as <code>var(--pp-color-accent)</code>, <code>var(--pp-font-heading)</code> etc.</p>
</div>

<style>
  .theme-form { display: flex; flex-direction: column; gap: .6rem; padding: .2rem .1rem; }
  .theme-form h3 { margin: .6rem 0 0; }
  .colors { display: grid; grid-template-columns: repeat(auto-fill, minmax(190px, 1fr)); gap: .6rem; }
  code { font-size: .8em; background: var(--hover); padding: .05rem .3rem; border-radius: 4px; }
</style>
