# paperparrot — make a template once, render it forever

Spec / implementation plan. A small self-hosted app that turns **HTML templates + structured data**
into **PDFs**, keeps every rendered PDF, and makes the "same document, new details" loop effortless.
Invoices are the first use case, but nothing in the app knows what an invoice is.

## 1. Goals & non-goals

**Goals**
- A **template** = HTML + CSS + a declared **data schema**. The app builds the input form from the schema.
- One template → many **documents** (an invoice per month, a letter per client). Repeating content
  (invoice lines) is a `list` field, so one template covers 1…n lines.
- Every **render** is kept as an immutable PDF together with the exact data and template revision
  that produced it. What you sent stays what you sent.
- **Live preview** in the browser while editing template or data; the PDF comes from the same engine
  (headless Chromium), so preview ≈ PDF.
- Simple styling through **theme tokens** (fonts, colours, block background, page margins) — a form,
  not a layout builder. Full CSS stays available for those who want it.
- **Duplicate** a document to start the next one pre-filled; auto-numbering via a `sequence` field.
- Reusable data **fragments** (a client, your sender block) that can be dropped into any document.
- Multi-user (register/login, strict per-user data), single binary, one Docker image, one data volume.
  Same skeleton as [murmelmoney](https://github.com/floholz/murmelmoney).
- A `render` CLI subcommand that uses the same engine without the database (scripting, CI).

**Non-goals** (v1)
- Drag-and-drop / WYSIWYG layout builder. Layout is HTML/CSS in a code editor with live preview.
- Invoice-specific features (clients table, VAT logic, dunning, e-invoicing/XRechnung). An invoice is a
  shipped starter template.
- Computed fields in the schema — computation happens in the template via functions (`sum`, `money`…).
- Sending (mail), signing, or storing documents anywhere but locally.
- Live Google Fonts fetching at render time; fonts are shipped or uploaded.
- Integration with murmelmoney — see §11; murmelmoney stays untouched either way.

## 2. Stack

| Layer | Choice | Notes |
|---|---|---|
| Backend | Go + PocketBase (framework mode, v0.39+) | SQLite, REST, auth, file storage, admin UI for free |
| Templating | Go `html/template` (default `{{ }}` delims) + small `FuncMap` | contextual auto-escaping: `&` in data just works, no more `\\&` |
| PDF | headless Chromium via `chromedp`, one long-lived instance, `Page.printToPDF` | `@page` size/margins honoured (`preferCSSPageSize`) |
| Frontend | Svelte 5 + Vite, plain CSS, `pocketbase` JS SDK, CodeMirror 6 for the editors | hash routing, no router lib |
| Fonts | a few OFL fonts embedded (`go:embed`) + per-user uploads (woff2/ttf) | inlined as `data:` URIs into the rendered HTML |
| Packaging | `//go:embed ui/dist`, `apis.Static` | one binary; the Docker image adds `chromium` |
| Auth | PocketBase `users` | registration open until first user, `PP_REGISTRATION=true/false` forces it |

**Why Chromium and not X:** LaTeX (mochatex) made looping/branching painful and needed a 1 GB image.
Typst would give a true static binary but no in-browser preview. wkhtmltopdf is dead upstream.
Pure-Go PDF libs mean layout-in-code, not user-editable templates. Chromium costs ~200 MB in the
image and buys "the preview engine is the PDF engine". The render backend sits behind a tiny
interface (`Renderer.PDF(ctx, html) ([]byte, error)`) so it can be swapped later.

## 3. Repository layout

```
paperparrot/
├── main.go                 # pocketbase app: embed ui, migrations, routes, superuser bootstrap, CLI
├── internal/
│   ├── schema/             # schema parsing + validation of data against schema
│   ├── tmpl/               # html/template execution, FuncMap, compose self-contained HTML
│   ├── theme/              # theme tokens → CSS, base stylesheet, built-in fonts (embedded)
│   ├── render/             # Renderer interface + chromedp backend (lazy start, pool of 1–2 tabs)
│   └── api/                # custom routes (§7)
├── migrations/             # 0001_init.go, …
├── templates/              # shipped starter templates (invoice, letter, …) — html, css, schema, sample
├── ui/                     # Svelte app (pages in §5)
├── Dockerfile, docker-compose.yml, Makefile, README.md, SPEC.md
```

## 4. Data model (PocketBase collections)

All base collections carry `user` (relation → users, required, cascade delete) and owner-only API rules:
`@request.auth.id != '' && user = @request.auth.id`, create additionally `@request.body.user = @request.auth.id`.

### `templates`
| field | type | notes |
|---|---|---|
| `name` | text | required, unique per user |
| `html` | text | body markup, Go template syntax |
| `css` | text | template-specific CSS, appended after the base stylesheet |
| `schema` | json | field definitions, §6 |
| `theme` | json | token values, §6b |
| `sample` | json | sample data used by the template editor preview |
| `title_format` | text | Go template for document titles / PDF filenames, e.g. `{{.number}} {{.recipient.name}}` |
| `page` | select `A4` \| `Letter` | default `A4` |
| `locale` | select `de-AT` \| `de-DE` \| `en` | number/date formatting in `money`/`date` funcs |
| `sequences` | json | counters for `sequence` fields, e.g. `{"number": {"2026": 3}}`; written only by Go hooks |
| `assets` | file, multiple (max 20, 5 MB each), protected | logo, signature…; referenced as `{{asset "logo.png"}}` |
| `version` | number | current revision number, bumped by hook |

### `template_revisions` — immutable
| field | type | notes |
|---|---|---|
| `template` | relation → templates | cascade delete |
| `version` | number | |
| `html`, `css`, `schema`, `theme`, `page`, `locale` | as above | snapshot |

Created by an `OnRecordAfterUpdateSuccess("templates")` hook whenever one of the snapshotted fields
changed (and once on create). No update/delete rules for users.

### `documents`
| field | type | notes |
|---|---|---|
| `template` | relation → templates | required; deleting a template with documents is refused in the UI |
| `title` | text | derived from `title_format` on save (hook), editable |
| `data` | json | current data, shape defined by the template schema |
| `created`, `updated` | autodate | |

Index on `(user, template, updated)`.

### `renders` — immutable
| field | type | notes |
|---|---|---|
| `document` | relation → documents | cascade delete |
| `revision` | relation → template_revisions | which template version produced it |
| `data` | json | snapshot of the document data at render time |
| `html` | text | the composed, self-contained HTML (cheap, makes the PDF exactly reproducible) |
| `pdf` | file, single, protected | |
| `title` | text | title at render time → download filename |

Created only by the render route (`createRule` = null). Users may delete, never update.

### `fragments`
| field | type | notes |
|---|---|---|
| `name` | text | "Seedback FlexCo", "My sender block" |
| `kind` | text | free tag matched by `object` fields' `fragment` property, e.g. `client`, `sender` |
| `data` | json | object matching the field's sub-schema |

Inserting a fragment **copies** its data into the document (snapshot). Old invoices keep old addresses.

### `fonts`
| field | type | notes |
|---|---|---|
| `family` | text | e.g. "Lora" |
| `weight` | number | 400, 700… |
| `style` | select `normal` \| `italic` | |
| `file` | file, single (woff2/ttf/otf, 2 MB) | |

Built-in fonts (Inter, Source Serif 4, JetBrains Mono) are not records; the font picker merges both lists.

## 5. Pages / UX

**Login / Register** — as murmelmoney; `GET /api/pp/status` tells the login page whether sign-up is open.

**Templates (`#/templates`)** — list with document counts; New (blank or from a shipped starter); duplicate; delete (refused while documents exist).

**Template editor (`#/templates/:id`)** — split view.
- Left, tabs: **HTML** · **CSS** · **Schema** · **Theme** · **Sample data** (CodeMirror; Schema and Sample as JSON with validation errors inline; Theme as a form, §6b).
- Right: live preview (debounced `POST /api/pp/preview` with the unsaved state → sandboxed `srcdoc` iframe on an A4-wide sheet). Toggle **PDF** to render via Chromium and show the real paginated result in the browser's PDF viewer.
- Save bumps a revision only if something rendered-relevant changed.

**Documents (`#/documents`)** — list: title · template · updated · render count; filter by template, text search on title; **New** → pick template.

**Document editor (`#/documents/:id`)** — split view.
- Left: form generated from the schema (§6). `list` fields have add/remove/reorder rows; `object` fields with `fragment` get a "insert fragment ▾" picker and "save as fragment"; `sequence` fields are prefilled and editable; `asset` fields upload to the template's assets.
- Right: the same live preview / PDF toggle.
- Actions: **Save**, **Render PDF** (saves first, creates a render, shows it), **Duplicate** (copies data, assigns the next sequence values, resets `date` fields with `default: "today"`), Delete.
- **Renders** panel: list of renders (date · title · template version) → open/download; delete.

**Fragments (`#/fragments`)** — list by kind, edit as JSON or via the sub-schema form of any template that declares that kind.

**Fonts (`#/fonts`)** — built-in list + upload (family/weight/style + file).

Mobile: usable for viewing documents and downloading renders; editing is a desktop affair (no PWA in v1).

## 6. Schema

Stored on the template as JSON. A flat `fields` array; `object` and `list` nest.

```jsonc
{
  "fields": [
    { "key": "number",    "type": "sequence", "label": "Number", "format": "HN-{yy}-{n}", "reset": "year" },
    { "key": "date",      "type": "date",     "label": "Date",   "default": "today" },
    { "key": "sender",    "type": "object",   "label": "From",   "fragment": "sender", "fields": [
        { "key": "name",    "type": "text" },
        { "key": "address", "type": "textarea" },
        { "key": "iban",    "type": "text", "required": false }
    ]},
    { "key": "recipient", "type": "object",   "label": "To",     "fragment": "client", "fields": [
        { "key": "name",    "type": "text" },
        { "key": "address", "type": "textarea" },
        { "key": "email",   "type": "text", "required": false },
        { "key": "uid",     "type": "text", "required": false, "label": "UID" }
    ]},
    { "key": "items",     "type": "list",     "label": "Items", "min": 1, "fields": [
        { "key": "text",   "type": "textarea", "label": "Service" },
        { "key": "period", "type": "text",     "required": false },
        { "key": "amount", "type": "money" }
    ]},
    { "key": "note",      "type": "textarea", "required": false },
    { "key": "logo",      "type": "asset",    "required": false, "accept": "image/*" }
  ]
}
```

Common properties: `key` (identifier, `[a-z][a-z0-9_]*`), `label` (defaults to key), `help`,
`required` (default `true`), `default`.

| type | stored as | form control | extras |
|---|---|---|---|
| `text` | string | input | `placeholder`, `pattern` |
| `textarea` | string | textarea | line breaks preserved via `{{nl2br .x}}` |
| `number` | number | input[type=number] | `min`, `max`, `step` |
| `money` | number (major units, 2 dp) | input with currency adornment | `currency` (default from locale) |
| `date` | `YYYY-MM-DD` string | date picker | `default: "today"` |
| `bool` | boolean | checkbox | |
| `select` | string | select | `options: [{value,label}]` |
| `object` | object | fieldset | `fields`, `fragment` |
| `list` | array of objects | repeatable rows | `fields`, `min`, `max` |
| `asset` | filename string | file picker into template assets | `accept` |
| `sequence` | string | prefilled text | `format` (`{n}`, `{n:3}` zero-padded, `{yy}`, `{yyyy}`), `reset: "never" \| "year"` |

Validation runs in Go on document save and render (`internal/schema`); the UI mirrors it for
instant feedback. Unknown keys in `data` are rejected; missing optional keys are fine.

**Sequence assignment** — `OnRecordCreate("documents")` hook (and Duplicate): for each `sequence`
field whose value is empty, read `templates.sequences[key][period]`, increment, format, write back.
`period` is the year for `reset: "year"`, `"all"` otherwise. Manual edits are allowed (they don't touch
the counter); PocketBase's SQLite write lock makes the increment safe.

### 6a. Template functions

Templates are `html/template` executed with the document data as `.`. Available funcs:

| func | example | notes |
|---|---|---|
| `money` | `{{money .amount}}` → `€ 1.200,00` | locale-aware; `{{money .x "USD"}}` |
| `num` | `{{num .qty 1}}` | decimals arg |
| `date` | `{{date .date "02.01.2006"}}` | Go layout; default layout from locale |
| `sum` | `{{money (sum .items "amount")}}` | sum a key over a list |
| `add` `sub` `mul` `div` | `{{mul .qty .rate}}` | |
| `nl2br` | `{{nl2br .address}}` | textarea → `<br>` (returns `template.HTML`) |
| `default` | `{{default "—" .uid}}` | |
| `upper` `lower` `title` | | |
| `asset` | `<img src="{{asset .logo}}">` / `{{asset "logo.png"}}` | data URI of a template asset |
| `theme` | `{{theme "color-accent"}}` | rarely needed; tokens are CSS vars anyway |

Data keys that are missing render as empty (`missingkey=zero`) — a typo in a template must not
fail the render, the preview makes it obvious.

### 6b. Theme tokens

Stored as `theme` JSON, emitted as CSS custom properties on `:root` before the base stylesheet.
The Theme tab is a form for exactly these; nothing else.

| token | UI | default |
|---|---|---|
| `font-body` | font picker | Inter |
| `font-heading` | font picker | (same as body) |
| `font-size` | number (pt) | 11 |
| `color-text` | colour | `#111` |
| `color-heading` | colour | `#111` |
| `color-accent` | colour | `#111` |
| `color-muted` | colour | `#666` |
| `color-block-bg` | colour | `#e8e8e8` |
| `margin` | text (`2cm` or `2cm 2.5cm`) | `2cm` |

Base stylesheet (`internal/theme/base.css`): `@page { size: <page>; margin: var(--pp-margin) }`,
body font/size/colour, headings in `--pp-font-heading`/`--pp-color-heading`, `.box` with
`--pp-color-block-bg`, `.muted`, sane `table`, `tr { break-inside: avoid }`, `.page-break`.
Templates use `var(--pp-color-accent)` etc.; a bare-text template needs almost no CSS of its own.

Fonts referenced by the theme (built-in or user-uploaded) are inlined as `@font-face` with `data:`
URIs, so preview and Chromium see identical fonts and the PDF embeds them.

## 7. Backend specifics

**Compose** (`internal/tmpl`): `Compose(rev, data, assets, fonts) (html string, err)` builds one
**self-contained** HTML document: `<style>` with token vars + base CSS + template CSS + `@font-face`
data URIs, then the executed body. No external URLs at all → same string works for the `srcdoc`
preview iframe and for Chromium, no auth tokens in URLs, no network from the renderer.

**Render** (`internal/render`): `chromedp` allocator started lazily on first use and kept alive
(cold start ~1 s, subsequent renders ~300 ms). `PP_CHROME` overrides the binary path; if none is
found, `GET /api/pp/status` reports `render: false`, the UI hides PDF actions, preview still works.
Per render: new tab, `emulation.SetScriptExecutionDisabled(true)`, block all non-`data:`
requests (`network.SetBlockedURLs`), `page.SetDocumentContent`, wait for fonts, `printToPDF`
with `preferCSSPageSize` + `printBackground`, close tab. 30 s timeout, at most 2 concurrent tabs
(semaphore). Chromium flags: `--headless=new --disable-gpu --no-sandbox` (content is self-authored
and network-isolated; the container runs as non-root anyway).

**Routes** (all require auth unless noted):
- `GET  /api/pp/status` — public: `{ registration, render, version }`
- `POST /api/pp/preview` — body `{ template?: id, html?, css?, schema?, theme?, page?, locale?, data }`
  → composed HTML (`text/html`). Inline fields override the stored template (unsaved editor state).
- `POST /api/pp/preview.pdf` — same body → PDF bytes (the editor's PDF toggle).
- `POST /api/pp/documents/{id}/render` — validate data, snapshot current template revision, compose,
  render, create `renders` record → returns it.
- `POST /api/pp/documents/{id}/duplicate` — copy data, assign sequences, apply `today` defaults → new record.
- `GET  /api/pp/fonts/builtin` — `[{family, weights}]` for the picker.

**Hooks**: registration policy and superuser bootstrap (`PP_REGISTRATION`, `PP_ADMIN_EMAIL`,
`PP_ADMIN_PASSWORD`) exactly as murmelmoney; template revision snapshot on create/update; document
title derivation and sequence assignment on create; refuse template delete while documents exist;
on first user, copy the shipped starter templates (`templates/` dir) into their account.

**CLI**: `paperparrot render -t body.html [-c style.css] [-s schema.json] [--theme theme.json] -d data.json -o out.pdf`
— no database, same compose + render code. `paperparrot serve` as usual (default `127.0.0.1:8072`).

**Migrations policy**: as murmelmoney — always a new numbered file, never edit an applied one.

## 8. Docker

```Dockerfile
FROM node:22-alpine AS ui          # vite build
FROM golang:1.26-alpine AS build   # CGO_ENABLED=0, embed ui/dist, -X main.version
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata chromium ttf-dejavu \
 && adduser -D -u 1000 pp
COPY --from=build /paperparrot /paperparrot
USER pp
ENV PP_CHROME=/usr/bin/chromium
VOLUME /pb_data
EXPOSE 8072
ENTRYPOINT ["/paperparrot", "serve", "--http=0.0.0.0:8072", "--dir=/pb_data"]
```
Image ≈ 220 MB (Chromium). `docker-compose.yml`: one service, `./data:/pb_data`, env for admin bootstrap,
`shm_size: 256m` (Chromium), `restart: unless-stopped`. Bare-metal: any installed Chrome/Chromium works.

CI / release as murmelmoney: build on push, `v*` tag → binaries + `ghcr.io/floholz/paperparrot`.

## 9. Implementation order

1. `internal/tmpl` + `internal/theme` + `internal/render` + CLI: port `hn-template.tex` to
   `templates/invoice/` and reproduce an existing Honorarnote PDF from its JSON. Proves the engine
   before any UI exists.
2. Go skeleton: PocketBase app, `0001_init.go` (all collections in §4), hooks, routes, status endpoint.
3. Svelte skeleton: login gate, nav, `pb.ts`; Templates list + editor with live preview.
4. Schema form generator + Documents list/editor; render + renders panel; duplicate.
5. Fragments, fonts, theme form, starter templates seeding.
6. Embed UI, Dockerfile, compose, README.

Rough size: ~1 200 lines Go, ~1 500 lines Svelte/TS. A few evenings; step 1 alone is one.

## 10. Open points / later ideas (not v1)
- Fetch-from-Google-Fonts helper (download once into `fonts`).
- Template import/export as a zip (html, css, schema, theme, sample, assets) for sharing.
- Computed/derived fields in the schema if template-side `sum` proves annoying.
- Partials shared between templates (`{{template "address" .}}`).
- Paginated preview without Chromium (e.g. `paged.js`) — only if the PDF toggle feels too slow.
- Per-document theme overrides.

## 11. murmelmoney integration (later, optional)

murmelmoney stays a finance tracker and needs **no changes**: it already exposes a PocketBase API.
The optional piece lives in paperparrot as a per-user **post-render webhook**: after a render,
POST multipart (`pdf`, `data`, `title`, `template`) to a configured URL with a bearer token.
A tiny mapping (`amount ← sum(items.amount)`, `date ← date`, `tag ← recipient.name`) lets a
murmelmoney target create the income transaction with the PDF attached. Until then: download the
PDF, attach it in murmelmoney by hand.
