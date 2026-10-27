<script lang="ts">
  import { clockifySettings, saveClockify, disconnectClockify, errMsg, type ClockifySettings } from '../lib/pb'

  let cfg = $state<ClockifySettings | undefined>()
  let key = $state(''), workspace = $state(''), rate = $state<number | null>(null)
  let error = $state(''), notice = $state(''), busy = $state(false)

  function show(c: ClockifySettings) {
    cfg = c
    workspace = c.workspace ?? ''
    rate = c.rate || null
  }
  clockifySettings().then(show).catch(e => (error = errMsg(e)))

  async function run(fn: () => Promise<ClockifySettings>, msg: string) {
    busy = true; error = ''
    try { show(await fn()); notice = msg; setTimeout(() => (notice = ''), 2500) } catch (e) { error = errMsg(e) } finally { busy = false }
  }
  function connect(e: SubmitEvent) {
    e.preventDefault()
    run(() => saveClockify({ api_key: key, rate: rate ?? 0 }), 'Connected').then(() => (key = ''))
  }
  const save = () => run(() => saveClockify({ workspace, rate: rate ?? 0 }), 'Saved')
  function disconnect() {
    if (confirm('Remove the Clockify connection and its API key from paperparrot?')) run(disconnectClockify, 'Disconnected')
  }
</script>

<h1>Settings</h1>
{#if error}<div class="notice error" style="margin-bottom:1rem">{error}</div>{/if}

<div class="panel">
  <h2>Clockify</h2>
  <p class="muted small">
    Lets list rows with <code>"clockify": true</code> in their template schema fetch your tracked time for a date range.
    paperparrot only ever <em>reads</em> from Clockify, and only your own entries. The API key stays on the server.
    Get one in Clockify under <em>Profile settings → Advanced → API keys</em>.
  </p>
  {#if !cfg}
    <p class="muted">Loading…</p>
  {:else}
    {#if cfg.connected}
      <p>Connected as <strong>{cfg.name}</strong> <span class="muted small">· time zone {cfg.timezone || 'UTC'}</span></p>
      <div class="row" style="align-items:flex-end; margin-bottom:1rem">
        <label class="field">Workspace
          <select bind:value={workspace}>
            {#each cfg.workspaces ?? [] as w (w.id)}<option value={w.id}>{w.name}</option>{/each}
          </select>
        </label>
        <label class="field">Default hourly rate
          <span class="adorn"><input type="number" bind:value={rate} min="0" step="0.01" placeholder="0.00" style="width:8rem" /><span class="muted">€</span></span>
        </label>
        <button class="primary" onclick={save} disabled={busy}>Save</button>
        <button class="danger" onclick={disconnect} disabled={busy}>Disconnect</button>
        {#if notice}<span class="muted small">{notice}</span>{/if}
      </div>
      <p class="muted tiny">Without a default rate, a row uses its own rate, or else the rate Clockify still stores on your entries.</p>
    {/if}
    <form class="row" style="align-items:flex-end" onsubmit={connect}>
      <label class="field">{cfg.connected ? 'Replace API key' : 'API key'}
        <input type="password" bind:value={key} required autocomplete="off" placeholder={cfg.connected ? '•••••••• saved' : 'paste your Clockify API key'} style="width:24rem" />
      </label>
      {#if !cfg.connected}
        <label class="field">Default hourly rate
          <span class="adorn"><input type="number" bind:value={rate} min="0" step="0.01" placeholder="optional" style="width:8rem" /><span class="muted">€</span></span>
        </label>
      {/if}
      <button class:primary={!cfg.connected} disabled={busy || !key.trim()}>{busy ? 'Checking…' : cfg.connected ? 'Replace key' : 'Connect'}</button>
      {#if notice && !cfg.connected}<span class="muted small">{notice}</span>{/if}
    </form>
    <p class="muted tiny">Connecting checks the key with 2 requests to Clockify; each “Sync from Clockify” is usually 1.</p>
  {/if}
</div>
