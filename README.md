# paperparrot

Make a document template once, fill in the details, render a PDF — and again next time
with new details. Like a parrot: tell it something once and it repeats it, in whatever
colours you like.

- **templates** are HTML + CSS with a declared data schema (text, money, date, lists, images…);
  the app builds the input form from the schema
- one template, many **documents** — an invoice per month, a letter per client — with
  every **render** kept as an immutable PDF (what you sent stays what you sent)
- **live preview** in the browser; the PDF comes out of the same engine (headless Chromium)
- simple **theme tokens** (fonts, colours, block backgrounds) instead of a layout builder
- multi-user, single binary, single SQLite file — Go + [PocketBase](https://pocketbase.io) + Svelte

Successor to [mochatex](https://github.com/floholz/mochatex) and
[baristex](https://github.com/floholz/baristex), minus the LaTeX. Sibling of
[murmelmoney](https://github.com/floholz/murmelmoney).

## Status

Design phase — see [`SPEC.md`](SPEC.md).

## License

MIT
