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
- **fragments** (a client, your sender block) to drop into any document, **sequences** for
  auto-numbering, **duplicate** to start the next one pre-filled
- multi-user, single binary, single SQLite file — Go + [PocketBase](https://pocketbase.io) + Svelte

Successor to [mochatex](https://github.com/floholz/mochatex) and
[baristex](https://github.com/floholz/baristex), minus the LaTeX. Sibling of
[murmelmoney](https://github.com/floholz/murmelmoney). Design notes in [`SPEC.md`](SPEC.md).

## Run it

**Docker** (recommended — the image ships Chromium):

```sh
mkdir data && docker compose up -d      # → http://localhost:8072
```

`docker-compose.yml` mounts `./data` (must be writable by uid 1000) and sets `shm_size: 256m`
for Chromium.

**Binary**: grab one from the releases page, then

```sh
./paperparrot serve                    # http://127.0.0.1:8072, data in ./pb_data
```

Any installed Chrome/Chromium is picked up automatically; set `PP_CHROME=/path/to/chromium`
if it lives somewhere unusual. Without a browser everything works except PDF output (the
status line in the UI says "no PDF").

The first visitor registers and becomes the owner; registration then closes. Every new user
starts with the shipped starter templates (an Austrian *Honorarnote* invoice and a letter).

| env | effect |
|---|---|
| `PP_REGISTRATION` | `true` always open · `false` always closed · unset: open until the first user |
| `PP_ADMIN_EMAIL` / `PP_ADMIN_PASSWORD` | create the PocketBase superuser for `/_/` on first start |
| `PP_CHROME` | Chromium/Chrome binary for PDF rendering |

## CLI

The same engine without the database — handy for scripting and CI:

```sh
paperparrot render -t body.html -c style.css -s schema.json --theme theme.json \
                   -d data.json -a ./assets -o out.pdf
paperparrot render -t body.html -d data.json --html out.html    # composed HTML only, no Chromium
```

`templates/invoice/` and `templates/letter/` in this repo are complete examples
(`body.html`, `style.css`, `schema.json`, `theme.json`, `sample.json`).

## Writing templates

A template is Go [`html/template`](https://pkg.go.dev/html/template) markup (the `<body>`
content) executed with the document data as `.`, plus CSS appended after a small base
stylesheet. Auto-escaping is contextual, so `&` in data just works.

```html
<h1>{{.number}}</h1>
{{nl2br .recipient.address}}
{{range .items}}<tr><td>{{.text}}</td><td class="num">{{money .amount}}</td></tr>{{end}}
<strong>{{money (sum .items "amount")}}</strong>
```

| function | example | notes |
|---|---|---|
| `money` | `{{money .amount}}` → `€ 1.200,00` | locale-aware; `{{money .x "USD"}}` |
| `num` | `{{num .qty 1}}` | decimals argument |
| `date` | `{{date .date}}`, `{{date .date "2. January 2006"}}` | Go layout; default from locale, month names localised |
| `sum` | `{{sum .items "amount"}}` | sum a key over a list |
| `add` `sub` `mul` `div` | `{{mul .qty .rate}}` | |
| `nl2br` | `{{nl2br .address}}` | textarea line breaks → `<br>` |
| `default` | `{{default "—" .uid}}` | |
| `upper` `lower` `title` | | |
| `asset` | `<img src="{{asset "logo.png"}}">` / `{{asset .logo}}` | template asset as a data URI |
| `theme` | `{{theme "color-accent"}}` | tokens are also CSS variables |
| `join`, `seq` | `{{join .tags ", "}}` | |

Missing keys render as empty. A missing *object* (`{{.nope.deep}}`) is an error that the
preview shows — use `{{with .nope}}{{.deep}}{{end}}` for optional blocks.

**Schema** — a `fields` array; `object` and `list` nest (max 3 levels):

```jsonc
{ "fields": [
  { "key": "number", "type": "sequence", "format": "HN-{yy}-{n}", "reset": "year" },
  { "key": "date",   "type": "date", "default": "today" },
  { "key": "recipient", "type": "object", "fragment": "recipient", "fields": [
      { "key": "name", "type": "text" }, { "key": "address", "type": "textarea" } ] },
  { "key": "items", "type": "list", "min": 1, "fields": [
      { "key": "text", "type": "textarea" }, { "key": "amount", "type": "money" } ] },
  { "key": "logo", "type": "asset", "required": false, "accept": "image/*" }
] }
```

Types: `text` (`placeholder`, `pattern`), `textarea`, `number` (`min`/`max`/`step`), `money`,
`date`, `bool`, `select` (`options`), `object` (`fields`, `fragment`), `list` (`fields`,
`min`/`max`), `asset` (`accept`), `sequence` (`format` with `{n}`, `{n:3}`, `{yy}`, `{yyyy}`,
`{mm}`; `reset: "never" | "year"`). Common: `label`, `help`, `required` (default true),
`default`. Drafts may be incomplete; required fields are enforced when rendering.

**Theme tokens** (Theme tab): `font-body`, `font-heading`, `font-size`, `color-text`,
`color-heading`, `color-accent`, `color-muted`, `color-block-bg`, `margin`. Available in CSS
as `var(--pp-color-accent)` etc. The base stylesheet provides `.box`, `.muted`, `.small`,
`.num`, `.right`, `.cols`, `.page-break` and sane tables. Built-in fonts: Inter,
Source Serif 4, JetBrains Mono; upload more (woff2/ttf/otf) under *Fonts*. Fonts are inlined
into the HTML, so preview and PDF are identical and the PDF embeds them.

## Development

```sh
make dev                 # Go API on :8072 (creates ./pb_data)
cd ui && npm run dev     # Vite on :5173, proxies /api
make test                # go vet + go test + svelte-check
make build               # UI + single binary with the UI embedded
```

`go build` needs `ui/dist` to exist (`make ui`, or an empty `ui/dist/index.html` for
API-only work). Migrations live in `migrations/` — always add a new numbered file, never
edit an applied one.

## License

MIT
