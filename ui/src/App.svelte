<script lang="ts">
  import { pb, status, type Status } from './lib/pb'
  import { parse } from './lib/router'
  import Login from './pages/Login.svelte'
  import Templates from './pages/Templates.svelte'
  import TemplateEditor from './pages/TemplateEditor.svelte'
  import Documents from './pages/Documents.svelte'
  import DocumentEditor from './pages/DocumentEditor.svelte'
  import Fragments from './pages/Fragments.svelte'
  import Fonts from './pages/Fonts.svelte'
  import ThemeToggle from './lib/ThemeToggle.svelte'
  import './lib/theme'

  let authed = $state(pb.authStore.isValid)
  pb.authStore.onChange(() => (authed = pb.authStore.isValid))

  let route = $state(parse(location.hash))
  window.addEventListener('hashchange', () => (route = parse(location.hash)))

  let info = $state<Status>({ registration: false, render: false, version: '' })
  status().then(s => (info = s)).catch(() => {})

  const nav = [
    ['templates', 'Templates'],
    ['documents', 'Documents'],
    ['fragments', 'Fragments'],
    ['fonts', 'Fonts'],
  ] as const
  const wide = $derived(!!route.id && (route.page === 'templates' || route.page === 'documents'))
</script>

{#if !authed}
  <div class="login-wrap"><ThemeToggle /><Login /></div>
{:else}
  <header class="top">
    <a class="brand" href="#/templates"><img src="/logo.svg" alt="" /><span>paperparrot</span></a>
    <nav>
      {#each nav as [page, label]}
        <a href={'#/' + page} class:active={route.page === page}>{label}</a>
      {/each}
    </nav>
    <span class="spacer"></span>
    {#if !info.render}<span class="tag" title="No Chromium found on the server — set PP_CHROME">no PDF</span>{/if}
    <span class="muted small user">{pb.authStore.record?.name || pb.authStore.record?.email}</span>
    <ThemeToggle />
    <button onclick={() => pb.authStore.clear()}>Logout</button>
  </header>
  <main class:wide>
    {#key route.raw}
      {#if route.page === 'templates' && route.id}
        <TemplateEditor id={route.id} canPdf={info.render} />
      {:else if route.page === 'templates'}
        <Templates />
      {:else if route.page === 'documents' && route.id}
        <DocumentEditor id={route.id} canPdf={info.render} />
      {:else if route.page === 'documents'}
        <Documents />
      {:else if route.page === 'fragments'}
        <Fragments />
      {:else if route.page === 'fonts'}
        <Fonts />
      {:else}
        <Templates />
      {/if}
    {/key}
  </main>
{/if}

<style>
  @media (max-width: 720px) { .user { display: none; } }
</style>
