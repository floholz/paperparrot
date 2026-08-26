<script lang="ts">
  import type { Snippet } from 'svelte'
  /** Closes only via the ✕ button (or Escape when `dismissable`). A click on the backdrop never closes it — too easy to lose typed input. */
  let { title = '', onclose, children, wide = false, dismissable = true }: { title?: string; onclose: () => void; children: Snippet; wide?: boolean; dismissable?: boolean } = $props()
  const onkey = (e: KeyboardEvent) => { if (e.key === 'Escape' && dismissable) onclose() }
</script>

<svelte:window onkeydown={onkey} />
<div class="modal-bg" role="presentation">
  <div class="modal" class:wide role="dialog" aria-modal="true" aria-label={title}>
    <div class="row" style="margin-bottom:.8rem">
      <h2 style="margin:0; flex:1">{title}</h2>
      <button type="button" class="link" onclick={onclose} aria-label="Close">✕</button>
    </div>
    {@render children()}
  </div>
</div>
