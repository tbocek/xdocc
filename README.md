# xdocc

A static site generator where **the filename is the configuration**.

No config files to maintain, no frontmatter required, no `weight: 3` sprinkled across
a hundred documents. A file called `2-about[About us]nav.md` tells xdocc everything it
needs: sort it second, publish it as `about.html`, title it "About us", put it in the
navigation.

xdocc compiles a source tree into a static HTML tree, and can stay running as a service
that watches the source and recompiles only what changed.

```
go install github.com/tbocek/xdocc/cmd/xdocc@latest    # or: go build ./cmd/xdocc

xdocc -s ./site -o ./www          # compile ./site into ./www and exit
xdocc -s ./site -o ./www -w       # keep running and recompile on every change
```

| Flag | Long form | Meaning |
|---|---|---|
| `-s <dir>` | `-source` | source directory (required) |
| `-o <dir>` | `-output` | output directory (required) |
| `-c <file>` | `-cache` | cache file; without it the cache lives in memory |
| `-w` | `-watch` | keep running and recompile when the source changes |
| `-x` | `-clear` | clear the cache before compiling |
| `-v` | `-version` | print the version and exit |

---

## 1. The model

**The order prefix decides whether xdocc touches a file at all.**

- **`1-about.md`** — an order prefix: xdocc takes charge. The file is transformed if its
  extension has a handler, listed in its directory's index, and published under a clean
  URL (`about.html`).
- **`hash.html`** — no order prefix: xdocc passes it through byte for byte, whatever its
  extension, and does not list it. A directory without an order prefix keeps its name,
  is not listed, and gets no generated index.

That second rule is what lets a self-contained thing — a demo, a generated report,
someone else's web app with its own `<head>`, its own CSS and its own `index.html` — be
dropped into the source tree and come out unharmed.

The rule is asked of **every name on its own**, and a directory does not answer for the
files inside it. So `demo/hash.html` is passed through, while `demo/1-a.md` in the same
directory still becomes `demo/a.html` — a directory with no order prefix is a place that
holds pages without being part of the structure.

| | how it's recognised | what happens |
|---|---|---|
| **content** | `1-about.md` | `about.html`, listed in the index |
| **content, no handler** | `1-photo.jpg` | `photo.jpg`, listed in the index |
| **passed through** | `logo.svg`, `notes.md`, `demo/` | byte for byte, not listed |
| **hidden** | `.draft.md`, `.old-dir/`, `notes.md~`, `notes.md.bak` | not in the output at all |

```
site/                            www/
├── .xdocc                       │
├── .templates/                  │
│   ├── page.html                │
│   └── list.html                │
├── 1-intro.md            ────►  ├── intro.html
├── 2-about[About]nav/    ────►  ├── about/
│   ├── 1-team.md         ────►  │   ├── team.html
│   └── photo.jpg         ────►  │   └── photo.jpg      (as is)
├── logo.svg              ────►  ├── logo.svg           (as is)
├── demo/                 ────►  ├── demo/              (as is, no index added)
│   └── index.html        ────►  │   └── index.html
                                 └── index.html         (listing of intro + about)
```

Every directory **that has an order prefix** also produces an `index.html` listing its
items. A directory without one gets a page only if it contains an ordered item called
`index` — `2025-02-17-index[FS25].md` gives `fs25/index.html` without making `fs25/`
part of the site's structure.

---

---

## 2. Filenames

```
  1-about[About us]|layout=wide|nav.md
  │ │      │        │               │
  │ │      │        │               └── extension, picks the handler
  │ │      │        └── properties, each introduced by |
  │ │      └── display name
  │ └── URL: about.html
  └── order
```

Formally:

```
filename := order "-" url [ "[" name "]" ] { "|" key [ "=" value ] } "." extension
```

Everything after the order is optional. `1-about.md` is a perfectly good name.

### Order

The prefix sets the sort key, and is what marks a file as content.

| Prefix | Sort key |
|---|---|
| `7-report.md` | 7 |
| `2024-06-03-launch.md` | that date |
| `2024-06-03_15:30:00-launch.md` | that date and time |
| `0-title.md` | pinned — `0` always sorts first, whatever the sort direction |

The `-` after the order is required, and it is the only thing that distinguishes the
two columns here:

| Name | Read as |
|---|---|
| `7-report.md` | content, order 7 |
| `7.md` | no order prefix: `7.html`, not listed |
| `2024-01-01-launch.md` | content, dated |
| `2024-01-01.md` | no order prefix: `2024-01-01.html`, not listed |
| `2024-launch.md` | content, order 2024 — `2024` is read as a number |
| `label-1.jpg` | no order prefix: passed through as `label-1.jpg` |

Directories work the same way: `3-news[News]nav/` is third in order, published at
`news/`, titled "News", and shown in the navigation.

### URL

The text between the `-` and the first `.` or `|` becomes the output filename:
`1-about.md` → `about.html`. Nested directories nest the URL:
`2-docs/1-intro.md` → `docs/intro.html`.

An item whose URL is **`index`** becomes the page of its directory and **replaces the
generated listing** — `1-index.md`, or `2025-02-17-index[FS25].md`. It is written even
where `show` leaves `page` out, because it *is* the directory's page. A plain `index.md` has no
order prefix, so xdocc leaves it alone — it is passed through, and the web server serves
it as the directory page by itself.

An order prefix with nothing after it — `7-.md` — has no URL to be published under, and
a name xdocc cannot read is a name it leaves alone: the file is passed through as
`7-.md`.

### Display name

Two equivalent ways to title an item "About us":

```
1-about[About us].md
1-about|name=About us.md
```

Without one, the name is the URL. A `name:` key in the front matter also works, and
wins over the filename.

### Properties

Properties are appended with `|`, as flags or as `key=value`:

```
1-gallery|nav|sort=desc|layout=wide.md
```

A `|` is not needed directly after a `]`, so `1-news[News]nav.md` is valid.

---

## 3. Where settings live

Three places, and one rule for how they combine:

> **`.xdocc` sets defaults for its directory and everything below it. A filename or
> front matter sets a single item. The more specific one wins.**

Lookup order, first hit wins:

```
filename → front matter → .xdocc here → .xdocc above → … → root .xdocc → default
```

So the root `.xdocc` is your site-wide configuration — not because it is special-cased,
but because it is the last one in the chain.

```yaml
# site/.xdocc — applies to the whole site
symlink: false
layout: default
```

`.xdocc` is YAML, with one convenience: a line that is a bare word is a flag, and
several may share a line the way they do in a filename.

```
show=list-link
nav|layout=wide
```

Three properties **never inherit**, because inheriting them is never what you mean:
`nav`, `name` and `show`. They describe one item. A `.xdocc` may
still set them — it then describes *its own* directory, not the ones below it:

```
1-news[News]/.xdocc     nav          →  the news directory is in the navigation,
                                        its subdirectories are not
```

### Front matter

An optional YAML block at the top of a file that is turned into HTML, fenced by three
or more dashes:

```markdown
---
name: Challenge Task Winner FS25
layout: wide
---
#### ${name}
(${date}) Congratulations to the winners …
```

Any property may be set here, and keys xdocc does not know are handed to templates as
`.Props`. The date is not among them: it comes from the filename, which is the one
place it decides the sort order as well.

---

## 4. Property reference

### Structural — describe one item, never inherited

| Property | Value | Applies to | Meaning |
|---|---|---|---|
| `nav` | flag | directory | include in the navigation tree |
| `name` | text | any | display name |
| `show` | places joined by `-`, default `page-list-link` | directory or item | where the item is shown — see below |

`show` lists the places an item appears in. There are three, and each one is
independent of the other two, so they are one set rather than three flags:

| Place | Meaning |
|---|---|
| `page` | a page of its own. On a **directory** this speaks for the items directly inside it and reaches no deeper, so `show=list-link` in the root `.xdocc` folds the front page together without flattening the sections below it |
| `list` | the generated listing of the directory it is in |
| `link` | what a `.link` file pulls in |

Write the places in any order, joined by `-`: `show=page-link` and `show=link-page`
are the same thing. What you leave out is what the item stays out of.

```
show=page-list-link   everywhere — the default, so you never have to write it
show=list-link        no page of its own: it appears inside its directory's index.html
show=page-link        out of the listing, but a .link file still pulls it in —
                      it shows only where you link to it
show=page             a page nothing links to on its own
```

A `.bib` defaults to `list-link` instead: a list of citations belongs in a listing and
has no page to be. `show=page-list-link` on one gives it a page anyway.

A place xdocc does not know is a typo, and xdocc says so and shows the item
everywhere — losing content to a misspelling is the worse way to be wrong.

### Settings — inherited down the tree

| Property | Value | Default | Meaning |
|---|---|---|---|
| `layout` | text | — | free-form string handed to templates as `.Layout`; templates decide what it means. Set on a directory itself — in its name or its `.xdocc`, not merely inherited — it also selects the list template: the listing renders with `list-<value>.html` if that file is in `.templates`. `layout: root` in the root `.xdocc` gives the front page its own list |
| `sort` | `auto`, `asc`, `desc` | `auto` | `auto` sorts dated items newest first and numbered items ascending |

### Site settings — root `.xdocc`

| Property | Value | Default | Meaning |
|---|---|---|---|
| `symlink` | bool | `true` | symlink assets into the output instead of copying them |
| `minify` | bool | `true` | minify HTML, CSS, JS, SVG, JSON and XML on the way out |
| `compress` | bool | `true` | write a `.gz` and a `.br` next to every text file in the output |
| `markdown` | bool | `true` | write a `.md` copy of every page next to it, for clients asking for `Accept: text/markdown` |
| `rescan` | duration | `10m` | how often the watcher rereads the whole tree even though nothing was reported; `off` disables it |
| `workers` | count | processors + 1 | how many output files are minified, compressed and written at once |

Assets are symlinked by default. A site whose weight is in its files — a lecture
folder full of video, a directory of PDFs — is then generated in milliseconds and
takes no second copy of the disk. Set `symlink: false` in the root `.xdocc` if the
output is handed to something that cannot follow a link pointing out of the output
tree. Links are relative, so the output tree may be moved as long as its position
relative to the source stays the same; to ship it standalone, dereference the links
while copying (`rsync -aL`). Where the file system has no symlinks at all, xdocc
says so once and copies instead, so this is a preference and not a promise.

Text is **minified** on the way out: every generated page, and the `.css`, `.js`,
`.svg`, `.json` and `.xml` files beside them. A minified file is one xdocc wrote rather
than one it points at, so those are written into the output even where `symlink` is on
— everything else, which is where the weight of a site is, is still a link. A file the
minifier cannot parse is written unchanged with a word in the log, so a single
malformed SVG cannot take a build down. `minify: false` turns it off.

Every text output also gets a **`.gz` and a `.br`** beside it, at the highest setting
gzip and brotli have: a static site is compressed once and served many times, so the
time belongs in the build and not in the request. Web servers pick the files up by
themselves — `precompressed br gzip` in Caddy, `gzip_static on` with `ngx_brotli` in
nginx. Files under 256 bytes are skipped, and so are the formats that are compressed
already: images, video, PDFs, archives. `compress: false` turns it off.

If the source tree still holds `.gz` or `.br` files from an earlier build, next to the
file they belong to, xdocc ignores them: it writes those paths itself now, so the copies
in the source are never read. Nothing is said about it — the build is neither wrong nor
in doubt because of them — but they are dead weight and can be deleted.

Every page is also written **as markdown** — `about.md` beside `about.html`, `index.md`
beside a directory's `index.html` — so that one URL can serve a browser its HTML and an
agent the prose without the frame around it. That is the generator's half of the
`Accept: text/markdown` convention; the other half is three lines of web server config.
What the copy contains, and how a server picks between the two,
is [§12](#12-serving-markdown-to-agents). `markdown: false` turns it off.

### Legacy spellings

One property, one word, with two short forms kept because they read better than
what they stand for:

`l` → `layout` · `asc`, `desc` → `sort=…`

Everything else that used to be a property is accepted and ignored, so an old tree
still compiles:

| Gone | Say this instead |
|---|---|
| `hidden`, `hide`, `hid` | a leading dot: `.draft.md`, `.old-dir/` |
| `copy`, `cp` | leave the order prefix off — that is what "do not transform" means now |
| `visible`, `vis` | give the file an order prefix |
| `promote`, `prm`, `prm1` | a `.link` file, which pulls from anywhere and not just from a child |
| `date` in front matter | the date in the filename |
| `post-processing`, `pp` | whatever starts xdocc: `ExecStartPost=`, a wrapper script, a `Makefile` |
| `noindex`, `nidx` | leave the order prefix off the directory: it then has pages and no listing, and nothing links to a page that is not there |
| `split`, `nosplit`, `page` | `show`: `nosplit` and `page` are `show=list-link`, `split` is the default |
| `nolist` | `show=page` |
| `linkonly` | `show=page-link` |
| `n`, `pag`, `dsc` | the words they abbreviated: `name`, `page`, `desc` |
| `content`, `paging`, `crop`, `dir-command`, `command-odt` … | — |

---

## 5. What a directory writes

A directory with an order prefix writes two things: a **listing** (`index.html`) and a
**page for each item** in it. Everything below is one of those two being turned off.
(Each of those pages also gets a `.md` copy of itself beside it — see
[§12](#12-serving-markdown-to-agents) — but that is one page written twice, not a
second page.)

| | listing | pages |
|---|---|---|
| `3-news/` | generated | yes |
| `3-news/` containing `1-index.md` | **that file, instead of the generated one** | yes |
| `3-news\|show=list-link/` | generated | no |
| `news/` — no order prefix | none | yes — each name inside is still judged on its own |

Reach for `1-index.md` first: writing the directory's page yourself is what people
usually mean, and it is the one that leaves the items reachable.

### Where an item is shown

By default each content item becomes its own page **and** appears in its directory's
`index.html`:

```
1-news/
├── 2025-06-02-winner.md   ──►  news/winner.html   +  entry in news/index.html
└── 2025-01-10-launch.md   ──►  news/launch.html   +  entry in news/index.html
```

With `show=list-link` in `1-news/.xdocc`, the individual pages are not written and
everything lands in `news/index.html`. It applies to that directory only and does not
reach the directories below it, so `show=list-link` in the root `.xdocc` gives you a
front page that is one long scroll while the sections keep their own pages.

`show` works on a single item just as well — `0-title|show=list-link.md` contributes to
the index but gets no page of its own. A `.bib` is `list-link` from the start unless you
ask for a page with `|show=page-list-link`, so a directory of `2006-pub[2006].bib` …
`2024-pub[2024].bib` is one publication page, one heading per year.

Templates should link to items with `.Link`, which points at the item's own page when it
has one and at its directory's index when it has not.

---

## 6. Pulling in content: `.link` files

A `.link` file pulls content from elsewhere in the tree and renders it in place. It is
how you build a front page out of pieces that live in their own directories.

`2-news|layout=4.link`:

```properties
url=news/*
limit=5
```

| Key | Meaning |
|---|---|
| `url` | what to pull; may be repeated, or comma separated |
| `limit` | maximum number of items |

URL patterns:

| Pattern | Resolves to |
|---|---|
| `news` | the item or directory `news` next to this file |
| `news/*` | every content item inside `news` |
| `/news` | `news` from the site root |
| `../news` | up one level |

A pulled directory is rendered as its own listing; a pulled file is rendered as itself.
Items keep the order the patterns were written in, and each directory's own sort order
inside that; a `sort` property on the `.link` file re-sorts everything it pulled in.
`limit` applies last.

---

## 7. Navigation

**Navigation** is built from directories marked `nav`, recursively — a directory that
is not in the navigation also keeps its children out of it. Templates get `.GlobalNav`
(the tree from the site root), `.CurrentNav` (the entry of the current directory, if it
is in the navigation, whose `.Children` are the navigation below it) and `.Breadcrumb`
(every directory from the site root down to this one).

A subdirectory appears in its parent's listing as a single entry that links to it, and
is compiled independently. To lift its items into another page instead, use a `.link`
file (section 6).

---

## 8. Templates

Liquid templates in `.templates/` — `{{ }}` for output, `{% %}` for blocks
(`{% if %}`, `{% for %}` with `forloop.index0`/`first`/`last`). Built-in defaults
exist for all of them, so you only add the ones you want to override. The data
structure is bound as `data`, so fields are `data.Name`, `data.Content`, …; inside
a `{% for x in data.Items %}` loop, the item is `x`.

| Template | Renders |
|---|---|
| `page.html` | the surrounding page frame — `<html>`, header, navigation |
| `list.html` | a directory's listing |
| `markdown.html` | one markdown item |
| `html.html` | one HTML item |
| `link.html` | the result of a `.link` file |
| `bib.html` | the citations of a `.bib` file |
| `file.html` | one asset or subdirectory shown in a listing — anything a listing links to rather than shows |

The navigation tree is not a template: it is recursive, and Liquid renders includes
with an empty context, so xdocc builds it in Go and the page template inlines it as
`{{ data.NavHTML }}`.

Available in every template (bound as `data`):

| | |
|---|---|
| item | `data.Name` `data.URL` `data.Link` `data.Content` `data.Date` `data.Nr` `data.Layout` `data.FileName` `data.FileSize` |
| listing | `data.Items` `data.ItemsByURL` |
| navigation | `data.GlobalNav` `data.CurrentNav` `data.Breadcrumb` |
| paths | `data.Root` — the way back to the site root from the page being rendered, `""` or `"../../"` |
| markdown | `data.Markdown` — the same content as markdown, `data.MarkdownURL` — the `.md` written next to this page, `""` when it has none |
| flags | `data.IsDir` `data.IsIndex` `data.IsNav` `data.IsTransformed` `data.Show.Page` `data.Show.List` `data.Show.Link` |

`data.URL` is the file an item produces, relative to the site root
(`docs/about.html`); `data.Link` is where to link to it, which differs only for
items without a page of their own. Write links as `{{ data.Root }}{{ data.Link }}`.

A navigation entry has `.Name`, `.Path` (`docs/api`), `.URL` (`docs/api/index.html`),
`.Href` (ready to use in the page being rendered), `.Active` (the current page is in
this directory or below it), `.Current` and `.Children`.

`data.ItemsByURL` keys the current listing by output file name, which lets a front
page place specific pieces rather than listing them:

```liquid
{{ data.ItemsByURL["intro.html"].Content }}
{{ data.ItemsByURL["news.html"].Content }}
```

Liquid has no arithmetic in `{{ }}` — use the standard filters `plus`, `minus`,
`modulo`, `divided_by` (and note that filters cannot appear inside `{% if %}`
conditions; precompute with `{% assign %}`). Besides the standard ones, these
filters are available: `base` `dir` `date` (Go layout string, e.g.
`{{ data.Date | date: "2006-01-02" }}`) `join` `lower` `upper` `replace`
`hasPrefix` `hasSuffix` `trimPrefix` `trimSuffix`.

### Substitution inside content

These placeholders are replaced **inside rendered content**, so markdown files can use
them:

| | |
|---|---|
| `${name}` | the item's display name |
| `${date}` | its date, as `2006-01-02` |
| `${nr}` | its sort key |
| `${url}` | a link to the item's page, from the page it is rendered on |
| `${path}` | a link to the item's directory, from the page it is rendered on |
| `${root}` | the way back to the site root |

They are substituted per item, so an item pulled onto the front page by a `.link` still
resolves against its own name and its own location.

---

## 9. Handlers

| Extension | Handled as |
|---|---|
| `.md`, `.markdown` | markdown → HTML, by [goldmark](https://github.com/yuin/goldmark): CommonMark plus GFM (tables, strikethrough, autolinks, task lists), footnotes, definition lists, automatic heading anchors, and inline HTML passed through |
| `.html`, `.htm` | HTML; the content of `<body>` if there is one |
| `.link` | content pulled from elsewhere (see above) |
| `.bib` | BibTeX → one `<div class="citation">` per entry, in file order |
| anything else | asset: symlinked into the output, or copied — see `symlink` |

The rightmost extension picks the handler, and known extensions stack:
`1-recipe.link.md` is markdown.

---

## 10. Watching, caching, cleanup

With `-w`, xdocc stays running and watches the source tree recursively, including
`.templates` and every `.xdocc`. Changes are collected for 200 ms before it recompiles,
so an editor writing a burst of files causes one build.

### Doing several files at once

Minifying and compressing at the highest setting is where a build spends its time, and
one output file's share of it has nothing to do with any other's. The walk stays single
file at a time — it renders pages in order and gathers them into listings — and hands
off everything after that to a pool of workers: reading the source, minifying it,
compressing it twice, writing it out.

The pool is one worker per processor plus one, the extra covering the moments a worker
is waiting on the disk rather than working. On a 12-processor machine, rebuilding
`old/site` — 2982 outputs, of which 1948 are compressed copies:

| workers | 1 | 2 | 4 | 8 | 12 | 13 | 16 | 24 |
|---|---|---|---|---|---|---|---|---|
| time | 1900 ms | 998 ms | 581 ms | 426 ms | 399 ms | 406 ms | 430 ms | 437 ms |

It flattens once the processors are busy, which is what a processor-bound build should
do. Set `workers: 1` in the root `.xdocc` to make a build single file at a time. The
site that comes out is the same either way: where two sources would write the same
output — the mistake xdocc reports as a warning — the second waits for the first, so
the result never depends on which worker finished when.

### What a change costs

A running xdocc keeps the item tree, the rendered HTML and the state of the output tree
in memory, so a rebuild reads only what moved:

| The watcher reports | xdocc does |
|---|---|
| a file was written | rereads that one file |
| a file appeared, vanished or was renamed | walks the source tree again |
| a `.xdocc` or a template changed | walks the source tree again |
| the kernel's watch queue overflowed | walks the source tree again |
| nothing at all, once per `rescan` | walks the source tree again |

Everything is *rendered* again either way. Rendering from memory is cheap, and it is
what keeps a page from going stale because of a file it does not contain; only the disk
is narrowed. On the site under `old/site` — 2600 files, 91 GB, 3000 outputs — editing
one markdown file is a rebuild of a few tens of milliseconds that reads that one file
and writes twelve: the page, the listing above it, the markdown copy of each, and a
`.gz` and a `.br` for all four.

File system notifications are best effort: a network share, a container bind mount, or
a burst that overran the kernel queue can swallow one, and then a page stays stale until
someone touches it again. `rescan` is the backstop. Every ten minutes by default xdocc
walks the whole tree whether or not anything was reported, and finding nothing costs
nothing. Set `rescan: 30m` or `rescan: off` in the root `.xdocc`; it is read once, at
startup.

The output tree is xdocc's to keep. It remembers what it put at every path, which is
what spares it stating every symlink and reading back every page on every build. Change
the output from outside and xdocc will not notice until it is restarted.

### What is cached

Only what a handler produces from one file, and only because it cannot depend on
anything else: the file's front matter, the HTML of its body, and the markdown of it.
Handlers never look at templates, at `.xdocc`, or at other files, so nothing but the
file itself can change either rendition — and both come out of the same read, so the
markdown copy costs no second trip to the disk.

An entry is valid while the file is byte for byte the one that was rendered — the cache
hashes the file rather than trusting its timestamp. A `git checkout`, an `rsync` or a
`touch` therefore costs nothing, and an edit that keeps the size and lands inside the
resolution of the file system's clock is still seen. The file has to be read anyway;
hashing it is cheap next to rendering it.

### What is never cached

Everything that depends on more than one file is recomputed on every run: the listing
of a directory, `.link` results, the navigation tree, the breadcrumb, `${...}`
substitution, and every template. **A page cannot go stale because of a file it does
not contain**: every listing is rebuilt from the current item tree, and only the
unchanged items inside it are served from the cache. That covers a `.link` whose target
changed, a `.xdocc` that changed the sort order, and a template change, which reaches
every page.

The tests in `internal/xdocc/cache_test.go` compile each of these cases twice with a
cache file reopened from disk in between.

With `-c` the cache survives restarts; `-x` clears it. Entries for files that no longer
exist are dropped at the end of each run.

### Writing and cleanup

Output is only written when it actually differs, so unchanged pages keep their
modification time and `rsync` has little to do. Compressing is the one expensive thing
in a build, so a `.gz` and a `.br` are only rebuilt when the bytes under them moved.
Output that this run did not produce is deleted, so removing or renaming a source file
removes its old page. Two sources that would write the same file are reported as a
warning.

Every run says what it did:

```
xdocc: 3163 written, 0 unchanged (61 pages, 3102 assets), 235 read in 1.22s   ← at startup
xdocc: 12 written, 3151 unchanged (61 pages, 3102 assets), 1 read in 39ms    ← after an edit
```

Pages and assets are what the site is made of, compressed copies and markdown copies
counted as assets — a page's `.md` is the same page, not a second one, so it is counted
where its `.gz` and its `.br` are; written and
unchanged are what this run had to touch. **Read** is the source files that
came off the disk rather than out of a cache — the work neither cache could spare. It
is worth knowing that it is never zero on a full walk: the hash that decides whether a
file changed is a hash of the file's bytes, so the walk has to read every content file
to find out that it need not render it. What the caches save is the parsing, the
rendering, the minifying and the compressing, not the read. A rebuild driven by the
watcher skips the walk altogether and reads only the file that changed.

The first build is the expensive one — nothing is cached and the output tree is
whatever was left behind — so it is timed like every rebuild after it. Watching starts
once it is done, and says in one line what it is watching and how often it will reread
it:

```
xdocc: 3163 written, 0 unchanged (61 pages, 3102 assets), 235 read in 1.22s
xdocc: watching /srv/site, rereading the whole tree every 10m0s
```

After that the line appears only when something really was written or removed, so a
quiet tree stays quiet in the log.

---

## 11. Running as a service

A linux/amd64 binary is attached to every
[release](https://github.com/tbocek/xdocc/releases), with an image at
`ghcr.io/tbocek/xdocc` for the same platform. Anywhere else, build it: xdocc uses no
CGo, so `go build ./cmd/xdocc` cross-compiles to whatever Go targets with nothing but
`GOOS` and `GOARCH` set.

### In a container

The image runs the watching mode as uid 1000, and nothing else: no shell in the
entrypoint, no network, no root.

```
docker run -d --name xdocc \
  -v ./site:/srv/site:ro \
  -v ./www:/srv/www \
  -v xdocc-cache:/var/cache/xdocc \
  ghcr.io/tbocek/xdocc:latest
```

The source can be mounted read-only — xdocc never writes to it. The cache volume is
what makes a restart cheap rather than a full rebuild, and the arguments are the
default `CMD`, so override them only to change the paths:

```
docker run --rm -v ./site:/srv/site:ro -v ./www:/srv/www \
  ghcr.io/tbocek/xdocc:latest -s /srv/site -o /srv/www          # build once and exit
```

One thing to know: the links xdocc writes for assets are **relative** to the output
directory, so `/srv/www/photo.jpg` points at `../site/photo.jpg`. Inside the container
that always resolves. On the host it resolves only if the two directories sit next to
each other the same way — `./site` and `./www` above do. If the web server reads the
output from somewhere else, put `symlink: false` in the root `.xdocc` and every asset is
copied instead.

### With Portainer

[`.portainer/docker-compose.yml`](.portainer/docker-compose.yml) is a Swarm stack for an
ingress that routes by label, [caddy-docker-proxy](https://github.com/lucaslorentz/caddy-docker-proxy)
in particular. Add it from this repository with that as the compose path — not through
the web editor, which has no `./Caddyfile` next to it for the `configs:` entry to read.

It is **two services**, because xdocc listens on nothing: `xdocc` generates into a
volume, and `web` serves that volume and carries the label the site name resolves
against. Change `xdocc.sifs0005.infs.ch` to the name you want; the port stays 8080,
which is what [`.portainer/Caddyfile`](.portainer/Caddyfile) listens on.

That Caddyfile is the content negotiation of [§12](#12-serving-markdown-to-agents), so
the stack answers `Accept: text/markdown` out of the box and serves the `.br` and `.gz`
xdocc already wrote. It reaches the container as a Swarm config rather than a mount,
because a volume would start empty and a bind mount would have to exist on whatever node
the task lands on. Swarm configs are immutable, so **editing the Caddyfile means bumping
`xdocc-caddyfile-v1` in the stack** — a redeploy that changes the content under an
unchanged name fails with `only updates to Labels are allowed`.

`/dav` on that same port is **WebDAV onto the source**, so the site can be edited by
mounting `https://xdocc.sifs0005.infs.ch/dav/` in a file manager: drop a file in, the
watcher next door notices and regenerates. It is a path and not a second hostname
because WebDAV answers PROPFIND with absolute paths, which means the prefix cannot
simply be stripped — the Caddyfile matches `/dav/*` without stripping and passes
`prefix /dav` to the module, and the site itself never sees those requests. That
needs a Caddy with the module compiled in — plugins are Go packages linked into the
binary — so [`.portainer/Dockerfile`](.portainer/Dockerfile) runs `xcaddy build --with
github.com/mholt/caddy-webdav` and the release workflow pushes the result as
`ghcr.io/tbocek/xdocc-caddy:latest`.

The `basic_auth` block is in the Caddyfile, with the username in plain sight and only
the hash coming from a **Swarm secret** — made in Portainer under *Secrets* before the
stack is deployed, named `xdocc-webdav-env-v1`, and holding one line:

```
WEBDAV_HASH=$2a$14$Xk...the.rest.of.what.caddy.hash-password.printed
```

Caddy substitutes environment variables into a Caddyfile, not files, and a Swarm secret
is a file — so the stack starts Caddy with `--envfile /run/secrets/webdav_env` and the
Caddyfile writes `{$WEBDAV_HASH}`. The `$` in the hash needs no escaping there, which it
would in a compose `environment:` value. Secrets are immutable like configs, so changing
the password means a new secret under a new name and the same bump in the stack file,
and the secret has to exist — Caddy will not start without the env file, which takes the
site down with it.

Basic auth sends the password in clear, so the ingress in front has to be the one
terminating HTTPS. To do without WebDAV entirely, drop the two `/dav` handles and the
`secrets:` entries, and go back to `caddy:2-alpine`.

`site` is an ordinary volume and starts empty — fill it over WebDAV, or replace it with
a bind mount to a source already on the host. Note that `web` mounts it too: the asset
links are relative to `/srv/www`, so `/srv/site` has to be in the serving container at
the same place. `symlink: false` in the root `.xdocc` makes the output standalone and lets
that mount go. And since the volumes are local, both services have to run on the same
node — one host, or a `placement` constraint on each.

### As a systemd unit

`contrib/xdocc.service` is a systemd unit for the watching mode:

```ini
[Service]
ExecStart=/usr/local/bin/xdocc -s /srv/xdocc/site -o /srv/www/site -c ${CACHE_DIRECTORY}/cache.gob -w
CacheDirectory=xdocc
Restart=on-failure
```

```
install -m 755 xdocc /usr/local/bin/xdocc
install -m 644 contrib/xdocc.service /etc/systemd/system/xdocc.service
systemctl daemon-reload && systemctl enable --now xdocc
journalctl -u xdocc -f
```

`CacheDirectory=xdocc` gives the service `/var/cache/xdocc` and passes it in
`$CACHE_DIRECTORY`, so a restart is not a full rebuild. The unit runs unprivileged and
hardened (`ProtectSystem=strict`, `NoNewPrivileges`, an empty capability set, and the
output directory as the only writable path). To publish after every build, add an
`ExecStartPost=` of your own or let the output directory be what the web server
serves.

### Cutting a release

Versions count up one at a time — v1, v2, v3 — so a release needs no decision about what
to call it. `./release.sh` takes no arguments: it checks that the tree is clean, tags the
next number and pushes it; that starts [the workflow](.github/workflows/build.yml), which
runs the tests, builds the linux/amd64 binary, attaches it to the release with the
generated changelog, and pushes the image. The script then polls until
that run has finished, so you learn at the terminal whether the release is good rather
than by looking later.

```
./release.sh        # tag the next version, push, wait for the build
```

There is nothing to pass: the version is always one past the highest tag, so the only
decision a release needs is whether to make one.

Everything is built by the workflow and nothing locally, so `release.sh` needs only git,
curl and jq. Setting `GITHUB_TOKEN` is worth it: polling costs up to 41 API calls against
an unauthenticated limit of 60 per hour, so a second release within the hour would
otherwise be refused.

Go's module system will not accept a tag like `v3`, so every release is tagged twice: `v3`
is the version, and `v1.3.0` is the same commit under a name Go recognises — the release
number is the minor. The alias is pushed only after the build has succeeded, so a failed
release leaves no version for `go install` to find, and the workflow skips any tag with a
dot in it so the alias cannot cut a second release.

The major stays at 1 and never moves, because this module has no importable API to break:
everything is under `internal/`, which nothing outside the module may import, and
`cmd/xdocc` is a `main` package. A major bump would announce a breaking change to a public
surface that does not exist — and it would drag the module path with it, since from `v2`
on Go puts the major version in the path itself.

```
go install github.com/tbocek/xdocc/cmd/xdocc@v1.3.0    # a particular release
go install github.com/tbocek/xdocc/cmd/xdocc@latest    # the newest one
```

---

## 12. Serving markdown to agents

Every page is written twice: `about.html`, and `about.md` next to it; a directory's
`index.html`, and `index.md` next to that. The `.md` is the page without the frame —
no header, no navigation, no styles, no layout wrappers — which is what a scraper, a
crawler or an agent reading the site is after, at a fraction of the bytes. On
[dsl.i.ost.ch](https://dsl.i.ost.ch/) the publication list is 29 KB of HTML against
21 KB of markdown, and a news item 1008 bytes against 261.

That is the generator's half of the [`Accept: text/markdown`](https://acceptmarkdown.com/)
convention: one URL, two renditions, and the web server hands over whichever the
request asked for. xdocc never opens a socket, so it cannot negotiate anything itself;
what it can do is put the markdown where a server finds it knowing nothing about the
tree — same path, `.md` instead of `.html`.

Pages also point at their own copy, so a client that does not negotiate can still find
it:

```html
<link rel="alternate" type="text/markdown" href="about.md">
```

The built-in `page.html` emits that. A site with a `page.html` of its own adds it by
hand — `data.MarkdownURL` is empty when the page has no copy, so it can be tested:

```liquid
{% if data.MarkdownURL %}<link rel="alternate" type="text/markdown" href="{{ data.MarkdownURL }}">{% endif %}
```

### What the copy contains

The copy follows the **items**, not the templates. Templates are HTML and there is
nothing to run them over here, so the markdown of a page is the markdown of what is on
it and none of what a template puts around it.

| Item | In the copy |
|---|---|
| a markdown file | itself, front matter stripped — the source as it was written |
| a `.bib` file | the same citations, as a markdown list |
| an HTML file | itself, unchanged: markdown carries inline HTML, and turning HTML back into markdown would be guessing at what the author meant |
| a `.link` file | the markdown of whatever it pulled in, the same as the page |
| a listing | the markdown of its items, in the order the listing has them, blank line between |
| anything not transformed — a subdirectory, a PDF, an asset | a markdown link, which is what a listing makes of it too |

`${name}`, `${url}` and the other placeholders are substituted in the copy exactly as
they are in the page, so both say the same thing.

Two consequences worth knowing before pointing an agent at it:

- **A listing loses what its list template added.** Year headings over a publication
  list, a wrapper class, an item the template filters out by name — the copy has the
  items themselves and nothing that only `list.html` knew about. Filtering by `show`
  in the filename rather than in the template keeps the two in step.
- **A page with nothing to say in markdown gets no copy**, rather than an empty one:
  a directory that only links to files, an empty listing. The server then has a
  missing file to fall back on and not a blank answer.

If a file in the source tree already writes to that path — a passed-through `about.md`
next to a generated `about.html` — the source wins and xdocc says so once in the log,
rather than quietly replacing it. Copies are compressed like every other text output,
so the `.gz` and the `.br` are there too, and they are counted as assets: a page's
markdown is the same page, not a second one.

`markdown: false` in the root `.xdocc` turns the whole thing off.

### Caddy

```caddyfile
example.com {
	root * /srv/site

	# the answer depends on Accept whether or not this request asked for
	# markdown, so every response says so
	header Vary Accept

	@markdown header Accept *text/markdown*
	handle @markdown {
		# "/a/b.html" has "/a/b.md", and a directory has the copy of its index
		@page path_regexp page ^(.*)\.html$
		rewrite @page {re.page.1}.md
		@dir path */
		rewrite @dir {path}index.md
		# and back to what was asked for when the page has no copy
		try_files {path} {http.request.orig_uri.path}
	}

	# serves the .gz and .br xdocc already wrote instead of compressing again
	file_server {
		precompressed br gzip
	}
}
```

Caddy knows the `.md` extension, so `Content-Type: text/markdown; charset=utf-8` comes
out by itself, and `precompressed` picks up the `.br` of a copy the same as the `.br`
of a page.

### nginx

nginx has no entry for `.md`, so add one line to `mime.types`:

```nginx
text/markdown  md;
```

It goes in that file rather than in a `types { … }` block of your own, because such a
block *replaces* the table `include mime.types` brought in instead of adding to it.
Then:

```nginx
# the copy next to a page: "/a/b.html" has "/a/b.md", "/a/" has "/a/index.md"
map $uri $md_uri {
    default                "/.no-markdown";
    "~^(?<page>.*)\.html$" "$page.md";
    "~^(?<dir>.*/)$"       "${dir}index.md";
}

# ...but only for a client that asked for it. Everything else keeps a name that
# is never in the output tree, so the try_files below simply misses on it and
# goes on to the page.
map $http_accept $md_try {
    default           "/.no-markdown";
    "~*text/markdown" $md_uri;
}

server {
    root /srv/site;
    index index.html;

    gzip_static on;                    # and ngx_brotli for the .br
    add_header Vary Accept always;

    location / {
        try_files $md_try $uri $uri/ =404;
    }
}
```

`Vary: Accept` is not decoration in either: without it a cache in front of the site
will hand an agent's markdown to the next browser that asks for the same URL.

Either way, this is what it looks like from the outside:

```
$ curl -sI https://example.com/pub/ | grep -i content-type
content-type: text/html; charset=utf-8

$ curl -sI -H 'Accept: text/markdown' https://example.com/pub/ | grep -i content-type
content-type: text/markdown; charset=utf-8
```

---

## History

Ported to Go from the Java implementation that still powers
[dsl.i.ost.ch](https://dsl.i.ost.ch/); its sources are kept under `old/` for reference,
together with the site itself under `old/site`, whose Freemarker templates have been
translated to Liquid templates next to the originals.

The Go version drops what the Java version accumulated: wikitext, pandoc and external
command handlers, the image pipeline, and about half of the properties.

---

## License

MIT, see [LICENSE](LICENSE).
