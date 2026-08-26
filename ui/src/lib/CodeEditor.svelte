<script lang="ts">
  import { EditorView, keymap, lineNumbers, highlightActiveLine, highlightActiveLineGutter, drawSelection, highlightSpecialChars } from '@codemirror/view'
  import { EditorState, Compartment } from '@codemirror/state'
  import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands'
  import { syntaxHighlighting, defaultHighlightStyle, bracketMatching, indentOnInput } from '@codemirror/language'
  import { html } from '@codemirror/lang-html'
  import { css } from '@codemirror/lang-css'
  import { json } from '@codemirror/lang-json'
  import { oneDark } from '@codemirror/theme-one-dark'
  import { current, watch } from './theme'

  let { value = $bindable(''), lang = 'html' as 'html' | 'css' | 'json', readonly = false }:
    { value?: string; lang?: 'html' | 'css' | 'json'; readonly?: boolean } = $props()

  let host: HTMLDivElement
  let view: EditorView | undefined
  const themeComp = new Compartment()
  const langComp = new Compartment()

  const language = (l: string) => (l === 'css' ? css() : l === 'json' ? json() : html())
  const themeExt = () => (current() === 'dark' ? oneDark : [])

  $effect(() => {
    view = new EditorView({
      parent: host,
      state: EditorState.create({
        doc: value,
        extensions: [
          lineNumbers(), highlightActiveLineGutter(), highlightSpecialChars(), history(), drawSelection(),
          indentOnInput(), bracketMatching(), highlightActiveLine(),
          syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
          keymap.of([...defaultKeymap, ...historyKeymap, indentWithTab]),
          EditorState.tabSize.of(2),
          EditorView.lineWrapping,
          EditorState.readOnly.of(readonly),
          langComp.of(language(lang)),
          themeComp.of(themeExt()),
          EditorView.updateListener.of(u => { if (u.docChanged) value = u.state.doc.toString() }),
        ],
      }),
    })
    const unwatch = watch(() => view?.dispatch({ effects: themeComp.reconfigure(themeExt()) }))
    return () => { unwatch(); view?.destroy(); view = undefined }
  })

  // external value changes (load, tab switch) → replace the doc
  $effect(() => {
    const v = value
    if (view && v !== view.state.doc.toString()) {
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: v } })
    }
  })
  $effect(() => { const l = lang; view?.dispatch({ effects: langComp.reconfigure(language(l)) }) })
</script>

<div class="code-editor" bind:this={host}></div>

<style>
  .code-editor { height: 100%; min-height: 12rem; border: 1px solid var(--line); border-radius: 6px; overflow: hidden; background: var(--panel); }
  .code-editor :global(.cm-editor) { height: 100%; font-size: .85rem; }
  .code-editor :global(.cm-scroller) { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
  .code-editor :global(.cm-editor.cm-focused) { outline: 2px solid var(--accent); outline-offset: -1px; }
</style>
