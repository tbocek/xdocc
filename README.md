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

Two independent questions decide what happens to a file:

- **Does the name start with an order prefix?** → it is *content*: it is listed in its
  directory's index and published under a clean URL.
- **Does its extension have a handler?** → it is *transformed* into HTML; otherwise it
  is copied, or symlinked, verbatim.

| | how it's recognised | what happens |
|---|---|---|
| **content** | `1-about.md` | `about.html`, listed in the index |
| **content, no handler** | `1-photo.jpg` | `photo.jpg`, listed in the index |
| **no order prefix** | `logo.svg` | `logo.svg`, not listed |
| **no order prefix, handler** | `notes.md` | `notes.html`, not listed |
| **hidden** | `.draft.md`, `notes.md~`, `1-a\|hidden.md` | not in the output at all |

```
site/                            www/
├── .xdocc                       │
├── .templates/                  │
│   ├── page.html                │
│   └── list.html                │
├── 1-intro.md            ────►  ├── intro.html
├── 2-about[About]nav/    ────►  ├── about/
│   ├── 1-team.md         ────►  │   ├── team.html
│   └── photo.jpg         ────►  │   └── photo.jpg      (copied)
├── logo.svg              ────►  ├── logo.svg           (copied)
                                 └── index.html         (listing of intro + about)
```

Every directory also produces an `index.html` listing its items.

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
| `label-1.jpg` | no order prefix: copied as `label-1.jpg` |

Directories work the same way: `3-news[News]nav/` is third in order, published at
`news/`, titled "News", and shown in the navigation.

### URL

The text between the `-` and the first `.` or `|` becomes the output filename:
`1-about.md` → `about.html`. Nested directories nest the URL:
`2-docs/1-intro.md` → `docs/intro.html`.

An **empty URL means `index`**, so `7-.md` produces `index.html`. That is the same
thing `1-index.md` does, and a plain `index.md` without an order prefix as well: an
item whose URL is `index` becomes the page of its directory and **replaces the
generated listing**. It is written even when splitting is off, because it *is* the
directory's page.

### Display name

Three equivalent ways to title an item "About us":

```
1-about[About us].md
1-about|name=About us.md
1-about:About us.md
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
split: false
symlink: true
layout: default
```

`.xdocc` is YAML, with one convenience: a line that is a bare word is a flag, and
several may share a line the way they do in a filename.

```
nosplit
nav|layout=wide
```

Four properties **never inherit**, because inheriting them is never what you mean:
`nav`, `name`, `noindex` and `promote`. They describe one item. A `.xdocc` may still
set them — it then describes *its own* directory, not the ones below it:

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
date: 2025-06-02
layout: wide
---
#### ${name}
(${date}) Congratulations to the winners …
```

Any property may be set here. `date` overrides the date parsed from the filename, and
with it the sort key.

---

## 4. Property reference

### Structural — describe one item, never inherited

| Property | Value | Applies to | Meaning |
|---|---|---|---|
| `nav` | flag | directory | include in the navigation tree |
| `name` | text | any | display name |
| `noindex` | flag | directory | do not write an `index.html` here |
| `promote` | flag or a number | directory | merge this directory's items into the parent's listing; `promote=1` merges only the first |

### Settings — inherited down the tree

| Property | Value | Default | Meaning |
|---|---|---|---|
| `split` | bool | `true` | `false`: no per-item pages — items only appear in their directory's `index.html` |
| `layout` | text | — | free-form string handed to templates as `.Layout`; templates decide what it means |
| `sort` | `auto`, `asc`, `desc` | `auto` | `auto` sorts dated items newest first and numbered items ascending |
| `hidden` | bool | `false` | keep out of the output entirely |
| `visible` | bool | `false` | list items *without* an order prefix too; they sort last, by filename |
| `copy` | bool | `false` | never transform — copy even if a handler exists |
| `date` | date | — | usually set in front matter; overrides the date in the filename |

### Site settings — root `.xdocc`

| Property | Value | Meaning |
|---|---|---|
| `symlink` | bool | symlink assets into the output instead of copying them |
| `post-processing` | command | run with `sh -c` in the output directory after a successful build |

### Legacy spellings

Still accepted, so existing trees keep working:

`nosplit`, `page`, `pag` → `split=false` · `hid`, `hide` → `hidden` · `vis` → `visible` ·
`cp` → `copy` · `nidx` → `noindex` · `prm` → `promote` · `prm1`, `promote1` →
`promote=1` · `n` → `name` · `l` → `layout` · `asc`, `desc`, `dsc` → `sort=…` ·
`pp` → `post-processing`

Properties of the Java version that no longer exist — `content`, `paging`, `crop`,
`dir-command`, `command-odt` and friends — are accepted and ignored.

---

## 5. Splitting

By default each content item becomes its own page **and** appears in its directory's
`index.html`:

```
1-news/
├── 2025-06-02-winner.md   ──►  news/winner.html   +  entry in news/index.html
└── 2025-01-10-launch.md   ──►  news/launch.html   +  entry in news/index.html
```

With `nosplit` in `1-news/.xdocc`, the individual pages are not written and everything
lands in `news/index.html`. Put it in the root `.xdocc` and the whole site works that
way: one page per directory.

`split` also works on a single item — `0-title|nosplit.md` contributes to the index but
gets no page of its own. Templates should link to items with `.Link`, which points at
the item's own page when it has one and at its directory's index when it has not.

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

## 7. Navigation and promotion

**Navigation** is built from directories marked `nav`, recursively — a directory that
is not in the navigation also keeps its children out of it. Templates get `.GlobalNav`
(the tree from the site root), `.LocalNav` (the tree below the current directory),
`.CurrentNav` (the entry of the current directory, if it is in the navigation),
`.Breadcrumb` (every directory from the site root down to this one) and
`.IsGlobalNav` (whether the current directory is reachable in the global tree).

**Promotion** lifts a subdirectory's items into its parent's listing:

```
site/
├── 2-item2.md
└── 1-featured|promote/
    └── 1-item1.md          ──►  index.html contains item1, item2
```

Without `promote`, a subdirectory appears in its parent's listing as a single entry
that links to it, and is compiled independently.

---

## 8. Templates

Go templates in `.templates/`. Built-in defaults exist for all of them, so you only add
the ones you want to override. Any other `.html` file in that directory can be called
with `{{ template "name.html" . }}`.

| Template | Renders |
|---|---|
| `page.html` | the surrounding page frame — `<html>`, header, navigation |
| `list.html` | a directory's listing |
| `markdown.html` | one markdown item |
| `html.html` | one HTML item |
| `link.html` | the result of a `.link` file |
| `file.html` | one asset shown in a listing |
| `directory.html` | one subdirectory shown in a listing |
| `nav.html` | a navigation tree, called with a list of navigation entries |

Available in every template:

| | |
|---|---|
| item | `.Name` `.URL` `.Link` `.Content` `.Date` `.Nr` `.Layout` `.Props` `.FileName` `.FileSize` `.Ext` `.Depth` |
| listing | `.Items` `.ItemsByURL` |
| navigation | `.GlobalNav` `.LocalNav` `.CurrentNav` `.Breadcrumb` `.IsGlobalNav` |
| paths | `.Root` — the way back to the site root from the page being rendered, `""` or `"../../"` |
| flags | `.IsDir` `.IsContent` `.IsIndex` `.IsNav` `.IsPromoted` `.IsTransformed` `.Split` |

`.URL` is the file an item produces, relative to the site root (`docs/about.html`);
`.Link` is where to link to it, which differs only for items that do not split. Write
links as `{{ .Root }}{{ .Link }}`.

A navigation entry has `.Name`, `.Path` (`docs/api`), `.URL` (`docs/api/index.html`),
`.Href` (ready to use in the page being rendered), `.Active` (the current page is in
this directory or below it), `.Current` and `.Children`.

`.ItemsByURL` keys the current listing by output file name, which lets a front page
place specific pieces rather than listing them:

```gotemplate
{{ (index .ItemsByURL "intro.html").Content }}
{{ (index .ItemsByURL "news.html").Content }}
```

Besides the standard ones, these functions are available: `base` `dir` `date` `html`
`join` `lower` `upper` `replace` `hasPrefix` `hasSuffix` `trimPrefix` `trimSuffix`.

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
| anything else | asset: copied, or symlinked when `symlink` is set |

The rightmost extension picks the handler, and known extensions stack:
`1-recipe.link.md` is markdown.

---

## 10. Watching, caching, cleanup

With `-w`, xdocc stays running and watches the source tree recursively, including
`.templates` and every `.xdocc`. Changes are collected for 200 ms before it recompiles,
so an editor writing a burst of files causes one build.

### What is cached

Only what a handler produces from one file, and only because it cannot depend on
anything else: the file's front matter and the HTML of its body. Handlers never look at
templates, at `.xdocc`, or at other files, so nothing but the file itself can change
that HTML.

An entry is valid while the file is byte for byte the one that was rendered — the cache
hashes the file rather than trusting its timestamp. A `git checkout`, an `rsync` or a
`touch` therefore costs nothing, and an edit that keeps the size and lands inside the
resolution of the file system's clock is still seen. The file has to be read anyway;
hashing it is cheap next to rendering it.

### What is never cached

Everything that depends on more than one file is recomputed on every run: the listing
of a directory, the items promoted into it, `.link` results, the navigation tree, the
breadcrumb, `${...}` substitution, and every template. **A promoted subdirectory cannot
go stale in its parent**: the parent's listing is rebuilt from the current item tree
every time, and only the unchanged children inside it are served from the cache. The
same holds for a `.link` file whose target changed, for a `.xdocc` that changed the
sort order, and for a template change, which reaches every page.

The tests in `internal/xdocc/cache_test.go` compile each of these cases twice with a
cache file reopened from disk in between.

With `-c` the cache survives restarts; `-x` clears it. Entries for files that no longer
exist are dropped at the end of each run.

### Writing and cleanup

Output is only written when it actually differs, so unchanged pages keep their
modification time and `rsync` has little to do. Output that this run did not produce is
deleted, so removing or renaming a source file removes its old page. Two sources that
would write the same file are reported as a warning.

---

## 11. Running as a service

Binaries for linux, macOS, FreeBSD and windows are attached to every
[release](https://github.com/tbocek/xdocc/releases); `./release.sh vX.Y.Z` builds, tags
and publishes one (`--dry-run` builds into `dist/` without touching GitHub).

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
output directory as the only writable path). If the root `.xdocc` sets
`post-processing`, that command runs under the same restrictions — give it its own
`ReadWritePaths` or relax `ProtectSystem`.

---

## History

Ported to Go from the Java implementation that still powers
[dsl.i.ost.ch](https://dsl.i.ost.ch/); its sources are kept under `old/` for reference,
together with the site itself under `old/site`, whose Freemarker templates have been
translated to Go templates next to the originals.

The Go version drops what the Java version accumulated: wikitext, pandoc and external
command handlers, the image pipeline, and about half of the properties.
