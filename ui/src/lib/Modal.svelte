<script lang="ts">
  import type { Snippet } from 'svelte'
  let { title = '', onclose, children, wide = false }: { title?: string; onclose: () => void; children: Snippet; wide?: boolean } = $props()
  const onkey = (e: KeyboardEvent) => { if (e.key === 'Escape') onclose() }
</script>

<svelte:window onkeydown={onkey} />
<div class="modal-bg" onclick={(e) => { if (e.target === e.currentTarget) onclose() }} role="presentation">
  <div class="modal" class:wide role="dialog" aria-modal="true" aria-label={title}>
    <div class="row" style="margin-bottom:.8rem">
      <h2 style="margin:0; flex:1">{title}</h2>
      <button type="button" class="link" onclick={onclose} aria-label="Close">✕</button>
    </div>
    {@render children()}
  </div>
</div>
