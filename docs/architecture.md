# How CodeLearn works

CodeLearn calls no language model. It splits the work in two:

- **CodeLearn** finds the changes, cuts them into units, gives each a
  content-derived id, and renders them in a local web viewer.
- **Your coding agent** writes the explanations through the shared
  [skill](../skills/codelearn/SKILL.md) and hands them back as JSON.

```text
codelearn .            ->  git analysis + local web viewer
codelearn export       ->  changes.json   (change units)
   your agent's skill  ->  analysis.json  (What / How / Why)
codelearn import       ->  validated, then shown next to the code
```

Because the agent only ever fills in explanations for ids CodeLearn generated,
an explanation can never point at the wrong code.

## Change units

A change unit is what one step explains.

Git hunks are what *detect* a change, but a hunk is an arbitrary grouping: it
can span three functions, and a new file is one hunk covering everything. So
wherever CodeLearn can outline a file, it cuts the change along symbol
boundaries instead.

| Language | Units |
| --- | --- |
| Go | functions, types, methods |
| Python | functions, classes, methods |
| TypeScript, JavaScript, TSX, JSX | top level functions, arrow consts, classes, methods, types |
| C, C++, Java, C#, Rust, Swift, Kotlin | top level functions, classes, methods |
| anything else | the raw diff hunk |

A unit covers the whole symbol, not only the edited lines, so a one-line
change is read in the context of the function it lives in. Code outside any
symbol — imports, constants, top level statements — becomes a `module level`
unit. A symbol whose text is unchanged produces no unit at all.

Outlines are produced by scanning, with no parser dependency, and are best
effort. The scan strips comments and string literals while carrying that state
across lines, so a brace inside a block comment does not end a function early
and code quoted inside a raw string is never mistaken for a declaration.
Multi-line signatures, generics, generic arrow functions and multi-line type
aliases are handled. Deeply unusual syntax may still fall back to a module
level unit, which is harmless: a unit never points at the wrong code.

## Identifiers

A **change id** is a SHA-256 of the file path, the symbol name, the comparison
base and the code on both sides of the unit. Editing a unit invalidates its
id, so an id that survives is proof the code behind it is unchanged — which is
what lets the viewer tell a still-valid explanation from a stale one.

Line numbers are deliberately *not* part of the id. Including them would make
every id depend on all the code above it, so adding a single import would
invalidate the explanation of every function below it in that file.

A **snapshot id** covers the base commit plus every changed file's path,
status and content. It is canonical: neither file ordering nor map iteration
order can change it for the same working tree.

## What is compared

The base is `HEAD`; the target is the file you see on disk, which merges
staged and unstaged edits into one entry per file.

That misses one case: a change that was staged and then reverted in the
working tree produces no HEAD-to-disk difference at all. Those files are found
through porcelain status, compared against the **index** instead, and marked
`staged only` in the viewer — a staged change your working tree has since
undone is exactly what you want to notice before committing.

Untracked files appear as new files with an empty Before. Binary files are
listed but not compared.

## Staleness

Import is atomic: if any step fails validation, nothing is stored.

On read, steps whose unit no longer exists are withheld rather than shown
against the wrong lines, and the viewer reports how many were dropped. An edit
elsewhere in the repository moves the snapshot without invalidating anything.

Analyses are stored under the OS user cache directory
(`os.UserCacheDir()/codelearn/<repository-id>/<snapshot-id>/analysis.json`),
never inside the repository, and written atomically via a temporary file. The
repository id includes the location it was opened from, so the same project on
a laptop and on a server keeps separate analyses.

## Data formats

`codelearn export` writes the change units:

```json
{
  "schema_version": "1.0",
  "snapshot_id": "sha256:...",
  "repository": { "name": "my-project", "base": "HEAD", "base_commit": "abc123" },
  "changes": [
    {
      "id": "sha256:...",
      "file": "internal/cache/cache.go",
      "symbol": "Cache.Get",
      "kind": "method",
      "label": "Cache.Get()",
      "old_range": { "start": 20, "count": 9 },
      "new_range": { "start": 20, "count": 16 },
      "before": "create()",
      "after": "reuse()"
    }
  ]
}
```

The skill returns learning steps that reference those ids and nothing else:

```json
{
  "schema_version": "1.0",
  "snapshot_id": "sha256:...",
  "steps": [
    {
      "change_id": "sha256:...",
      "title": "Cache reuse",
      "what": "Cache creation was replaced with reuse of the existing cache.",
      "how": "The method now returns the object the lookup gives back.",
      "why": "Avoids paying the construction cost on every call."
    }
  ]
}
```

`why` is optional: an agent that cannot find evidence is expected to leave it
out rather than guess. All fields accept Markdown. The full contract is
[`analysis-schema.json`](../skills/codelearn/references/analysis-schema.json).

Line ranges are `{ start, count }`, 1-based and inclusive; a `count` of 0 means
that side has no lines (a pure insertion has an empty old range).

## HTTP API

The server binds the loopback interface and serves the embedded viewer.

| Endpoint | Returns |
| --- | --- |
| `GET /api/repository` | name, branch, base commit, snapshot id, file count |
| `GET /api/changes` | the changed files and the unit ids of each |
| `GET /api/files/diff?path=` | one file's Before/After and its units |
| `GET /api/analysis` | the stored steps resolved against the current snapshot |
| `POST /api/refresh` | re-reads git and replaces the snapshot |

Reads are served from the snapshot the server already holds, so drawing one
screen costs one collection rather than one per endpoint, and the three panes
can never disagree about which state they show. `POST /api/refresh` is the
only thing that picks up new edits — it is what the Refresh button calls.

## Remote repositories

`--remote [user@]host:/path` drives git on another host over SSH. Nothing is
installed there; it needs `git` and a POSIX shell.

Every repository operation goes through one of two methods — run a git
command, read working tree files — so everything above is written once and
does not know which side of a connection the repository is on.

Two things make it usable in practice. Connections are multiplexed over one
SSH master, so a snapshot costs a single handshake rather than one per
command. And working tree files are read in a single framed request — each
file is a byte count followed by exactly that many bytes — rather than a round
trip each.

## Performance

Measured on a repository of 1,000 single-function Go files, all modified, on a
Linux x86-64 workstation:

| Scenario | Time |
| --- | --- |
| `codelearn export` (one full collection) | 0.24 s |
| Opening the viewer (file list + analysis) | 0.012 s |
| Clicking a file (its diff) | 0.0003 s |
| Pressing Refresh (full re-collection) | 0.24 s |

All "before" blobs come from a single `git cat-file --batch` process rather
than one `git show` per file, which turned 1,007 git invocations per
collection into 9.

These are measurements, not guarantees; the test suite does not assert them.
