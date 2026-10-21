# CodeLearn

**Local-first code change learning viewer.**

CodeLearn shows the uncommitted changes in a git repository as a Before/After
diff and pairs each change with a What / How / Why explanation that you walk
through one step at a time, the way CodeTour walks through a codebase.

Diff viewers tell you *which lines* changed. CodeLearn is for understanding
*what the new implementation does* - which matters when the code was written
by a coding agent and you are the one who has to review it.

```bash
cd my-project
codelearn .
```

```text
CodeLearn v0.3.0

Repository: /home/user/my-project
Branch: main
Changed Files: 4

Viewer: http://127.0.0.1:8765

Press Ctrl+C to stop.
```

## How it works

CodeLearn calls no language model and needs no API key. It splits the work in
two:

- **CodeLearn** finds the changes, cuts them into change units - a function, a
  method, a class - gives each a content-derived id, and renders them in a
  local web viewer.
- **Your coding agent** - Claude Code, Codex, or anything that can run a skill
  - writes the explanations through the shared
  [CodeLearn skill](skills/codelearn/SKILL.md) and hands them back as JSON.

```text
codelearn .            ->  git analysis + local web viewer
codelearn export       ->  changes.json   (change units)
   your agent's skill  ->  analysis.json  (What / How / Why)
codelearn import       ->  validated, then shown next to the code
```

Because the agent only ever fills in explanations for ids CodeLearn generated,
an explanation can never point at the wrong code.

## Install

A single binary. Git must be installed; nothing else is required.

Download a release binary for your platform, or build from source:

```bash
git clone https://github.com/inpyu/codelearn.git
cd codelearn
make build          # builds the frontend and embeds it in ./codelearn
```

Building needs Go 1.24+ and Node.js. Running the built binary needs neither.

## Usage

```bash
codelearn                     # open the viewer for the current repository
codelearn /path/to/repo       # open the viewer for another repository
codelearn . --no-browser      # do not launch a browser
codelearn . --port 9000       # bind a specific port
codelearn . --host 100.x.y.z  # bind another interface (see below)

codelearn export --output changes.json    # export the current change units
codelearn import analysis.json            # import an analysis from a skill
```

The server binds `127.0.0.1` and picks a free port when the default is taken.

### What appears in the viewer

| Pane | Contents |
| --- | --- |
| Changed Files | every modified, staged, unstaged, added, deleted, renamed and untracked file, with totals |
| Diff Viewer | side-by-side Before/After in a Monaco diff editor; clicking code selects the unit it belongs to |
| Learning Guide | every change unit of the selected file, explained or not, with Prev/Next and What / How / Why |

Unit navigation lives in one place. The Learning Guide lists all the units of
the selected file, so a unit without an explanation is visible as such rather
than hidden, and Prev/Next walks them in file order before carrying on into
the next file that has explanations.

Drag the dividers between the panes to rebalance them - a shallow file list
needs little width, a wide diff needs a lot - and double-click a divider to
reset it. The widths are remembered per browser.

Staged and unstaged edits to the same file are merged into one entry, untracked
files show up as new files with an empty Before, and binary files are listed
but not compared.

Press **Refresh** to re-read the working tree. Everything else is served from
the snapshot the server already holds, so one screen costs one collection
rather than one per request, and the three panes can never disagree about
which state they are showing. CodeLearn does not watch the filesystem.

**Staged-only changes.** The comparison is HEAD against the file you see on
disk, which misses a change that was staged and then reverted in the working
tree. Such a file is listed separately, marked `staged only`, and compared
against the index instead - a staged change your working tree has since
undone is exactly what you want to notice before committing.

Selecting a file scopes the Learning Guide to that file. Prev/Next walks the
units of the file and then carries on into the next file that has
explanations, so the tour still runs end to end.

### Change units

A change unit is what one step explains. Git hunks are what *detect* a change,
but a hunk is an arbitrary grouping - it can span three functions, and a new
file is one hunk covering everything. So wherever CodeLearn can outline a
file, it cuts the change along symbol boundaries instead:

| Language | Units |
| --- | --- |
| Go | functions, types, methods |
| Python | functions, classes, methods |
| TypeScript, JavaScript, TSX, JSX | top level functions, arrow consts, classes, methods, types |
| C, C++, Java, C#, Rust, Swift, Kotlin | top level functions, classes, methods |
| anything else | the raw diff hunk |

Outlines are produced by scanning, with no parser dependency, and are best
effort: a file CodeLearn cannot outline falls back to hunks and still works.
Multi-line signatures, generics, generic arrow functions and multi-line type
aliases are handled; deeply unusual syntax may still fall back to a module
level unit, which is harmless because a unit never points at the wrong code.

A unit covers the whole symbol, not only the edited lines, so a one-line
change is read in the context of the function it lives in. Code outside any
symbol - imports, constants, top level statements - becomes a `module level`
unit. A symbol whose text is unchanged produces no unit at all.

### Viewing a repository on a remote machine

Keep the default loopback binding and forward the port:

```bash
# from your laptop
ssh -N -L 8765:127.0.0.1:8765 user@remote-host
```

Then open `http://localhost:8765`. VS Code Remote-SSH does the same through
its **Ports** panel.

If the host is on a private network such as Tailscale or a VPN, binding that
interface directly also works:

```bash
codelearn . --host 100.x.y.z
```

Do not bind `0.0.0.0` unless you know every interface it covers. On a machine
with a public address that publishes your uncommitted code to the internet.

## Generating explanations

Install the shared skill for your agent (see
[skills/README.md](skills/README.md)), then ask in your own words:

> Analyse the current changes with CodeLearn so I can learn them.

The agent exports the change units, reads the code around them, writes one
step per unit, and imports the result. Press Refresh in the viewer.

Without a skill, everything except the Learning Guide still works - CodeLearn
is a perfectly good pre-commit diff viewer on its own.

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
[`analysis-schema.json`](skills/codelearn/references/analysis-schema.json).

Line ranges are `{ start, count }`, 1-based and inclusive; a `count` of 0 means
that side has no lines (a pure insertion has an empty old range).

Change ids are a SHA-256 of the file path, the symbol, the comparison base and
the code on both sides of the unit, so editing a unit invalidates its id. An
id that survives is therefore proof the code behind it is unchanged, which is
what lets the viewer tell a still-valid explanation from a stale one.

Line numbers are deliberately not part of the id. Including them would make
every id depend on all the code above it, so adding a single import would
invalidate the explanation of every function below it in that file.

Import is atomic: if any step fails validation, nothing is stored. On read,
steps whose unit no longer exists are withheld rather than shown against the
wrong lines, and the viewer reports how many were dropped. An edit elsewhere
in the repository moves the snapshot without invalidating anything.

Analyses are stored under the OS user cache directory
(`os.UserCacheDir()/codelearn/<repository-id>/<snapshot-id>/analysis.json`),
never inside the repository, and written atomically via a temporary file.

## Performance

Measured on a repository of 1,000 single-function Go files, all modified,
on a Linux x86-64 workstation:

| Scenario | Time |
| --- | --- |
| `codelearn export` (one full collection) | 0.24 s |
| Opening the viewer (file list + analysis) | 0.012 s |
| Clicking a file (its diff) | 0.0003 s |
| Pressing Refresh (full re-collection) | 0.24 s |

Two things make this flat. All "before" blobs come from a single
`git cat-file --batch` process rather than one `git show` per file, which
turned 1,007 git invocations per collection into 9. And reads are served from
the snapshot the server holds, so drawing a screen costs one collection rather
than one per endpoint.

These are measurements, not guarantees; `tests/e2e.sh` does not assert them.

## Security and privacy

- CodeLearn never writes to the repository it is looking at.
- The server listens on the loopback interface only.
- A file path from the client is resolved against the snapshot's changed file
  list, never used to read the filesystem directly.
- State-changing requests are rejected when their `Origin` is not this server,
  so a page on the open internet cannot drive the local API.
- Request bodies are size-limited, and files over 2 MiB are not rendered.
- Imported Markdown is rendered by react-markdown with raw HTML disabled, and
  links are rendered as plain text.
- Text inside a diff is data. Neither CodeLearn nor the skill treats
  instructions found in changed code as commands.
- The skill runs inside your coding agent, so the code it reads is handled
  under that agent's data policy. CodeLearn itself sends nothing anywhere.

## Development

```bash
make test            # go test ./... and the frontend suite
make vet             # go vet ./...
make build           # frontend + embedded binary
make release         # cross-compiled binaries in dist/

go run ./cmd/codelearn .            # backend only, against this repository
cd frontend && npm run dev          # viewer on :5173, proxying /api to :8765
```

Run `go run ./cmd/codelearn . --no-browser` in one terminal and `npm run dev`
in another to work on the UI against a live repository.

Layout:

```text
cmd/codelearn/      entry point
internal/cli/       command line
internal/git/       repository, change collection, symbol outlines, snapshots
internal/analysis/  schema, export, import, local storage
internal/server/    HTTP server and routes
web/                embedded frontend build output
frontend/           React + TypeScript viewer (Monaco diff editor)
skills/codelearn/   the shared agent skill and its JSON Schema
```

The specification this implements is [SPEC.md](SPEC.md).

## Scope

This is the v0.3 MVP. It deliberately leaves out PR integrations, a VS Code
extension, an MCP server, any built-in LLM, call graphs, quality scoring and
quizzes.

One thing grew past the original plan: it defined the change unit as the raw
git hunk and ruled out symbol level analysis. In practice a hunk is a poor
unit to learn from, so CodeLearn outlines files and cuts changes along symbol
boundaries, falling back to hunks where it cannot. SPEC.md records the
amendment.

## License

MIT - see [LICENSE](LICENSE).
