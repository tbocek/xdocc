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
when splitting is off, because it *is* the directory's page. A plain `index.md` has no
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
nosplit
nav|layout=wide
```

Three properties **never inherit**, because inheriting them is never what you mean:
`nav`, `name` and `split`. They describe one item. A `.xdocc` may
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
| `split` | bool, default `true` | directory or item | `false`: no page of its own — the item appears only inside its directory's `index.html`. On a directory it speaks for the items directly inside it and reaches no deeper, so `nosplit` in the root `.xdocc` folds the front page together without flattening the sections below it |

### Settings — inherited down the tree

| Property | Value | Default | Meaning |
|---|---|---|---|
| `layout` | text | — | free-form string handed to templates as `.Layout`; templates decide what it means |
| `sort` | `auto`, `asc`, `desc` | `auto` | `auto` sorts dated items newest first and numbered items ascending |

### Site settings — root `.xdocc`

| Property | Value | Default | Meaning |
|---|---|---|---|
| `symlink` | bool | `true` | symlink assets into the output instead of copying them |

Assets are symlinked by default. A site whose weight is in its files — a lecture
folder full of video, a directory of PDFs — is then generated in milliseconds and
takes no second copy of the disk. Set `symlink: false` in the root `.xdocc` if the
output is handed to something that cannot follow a link pointing out of the output
tree. Where the file system has no symlinks at all, xdocc says so once and copies
instead, so this is a preference and not a promise.

### Legacy spellings

One property, one word, with three short forms kept because they read better than
what they stand for:

`nosplit`, `page` → `split=false` · `l` → `layout` · `asc`, `desc` → `sort=…`

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
| `n`, `pag`, `dsc` | the words they abbreviated: `name`, `nosplit`, `desc` |
| `content`, `paging`, `crop`, `dir-command`, `command-odt` … | — |

---

## 5. What a directory writes

A directory with an order prefix writes two things: a **listing** (`index.html`) and a
**page for each item** in it. Everything below is one of those two being turned off.

| | listing | pages |
|---|---|---|
| `3-news/` | generated | yes |
| `3-news/` containing `1-index.md` | **that file, instead of the generated one** | yes |
| `3-news\|nosplit/` | generated | no |
| `news/` — no order prefix | none | yes — each name inside is still judged on its own |

Reach for `1-index.md` first: writing the directory's page yourself is what people
usually mean, and it is the one that leaves the items reachable.

### Splitting

By default each content item becomes its own page **and** appears in its directory's
`index.html`:

```
1-news/
├── 2025-06-02-winner.md   ──►  news/winner.html   +  entry in news/index.html
└── 2025-01-10-launch.md   ──►  news/launch.html   +  entry in news/index.html
```

With `nosplit` in `1-news/.xdocc`, the individual pages are not written and everything
lands in `news/index.html`. It applies to that directory only and does not reach the
directories below it, so `nosplit` in the root `.xdocc` gives you a front page that is
one long scroll while the sections keep their own pages.

`split` also works on a single item — `0-title|nosplit.md` contributes to the index but
gets no page of its own. A `.bib` never splits unless you ask it to with `|split`: a
list of citations belongs in a listing and has no page to be. So a directory of
`2006-pub[2006].bib` … `2024-pub[2024].bib` is one publication page, one heading per
year. Templates should link to items with `.Link`, which points at
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
| `bib.html` | the citations of a `.bib` file |
| `file.html` | one asset or subdirectory shown in a listing — anything a listing links to rather than shows |
| `nav.html` | a navigation tree, called with a list of navigation entries |

Available in every template:

| | |
|---|---|
| item | `.Name` `.URL` `.Link` `.Content` `.Date` `.Nr` `.Layout` `.Props` `.FileName` `.FileSize` `.Ext` `.Depth` |
| listing | `.Items` `.ItemsByURL` |
| navigation | `.GlobalNav` `.CurrentNav` `.Breadcrumb` |
| paths | `.Root` — the way back to the site root from the page being rendered, `""` or `"../../"` |
| flags | `.IsDir` `.IsIndex` `.IsNav` `.IsTransformed` `.Split` |

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
| `.bib` | BibTeX → one `<div class="citation">` per entry, in file order |
| anything else | asset: symlinked into the output, or copied — see `symlink` |

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
output directory as the only writable path). To publish after every build, add an
`ExecStartPost=` of your own or let the output directory be what the web server
serves.

---

## History

Ported to Go from the Java implementation that still powers
[dsl.i.ost.ch](https://dsl.i.ost.ch/); its sources are kept under `old/` for reference,
together with the site itself under `old/site`, whose Freemarker templates have been
translated to Go templates next to the originals.

The Go version drops what the Java version accumulated: wikitext, pandoc and external
command handlers, the image pipeline, and about half of the properties.
