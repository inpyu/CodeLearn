# Contributing to CodeLearn

Thanks for looking. CodeLearn is small on purpose, and the goal is to keep it
small enough for one maintainer to understand, test and extend.

Before adding a feature, read [SPEC.md](SPEC.md) — it records both what is in
scope and what has been deliberately excluded.

## Requirements

- **Go 1.24+** for the backend and CLI
- **Node.js** for the frontend (only to build it; the released binary needs
  neither)
- **git** — a runtime dependency, not just a build one

## Getting set up

```bash
git clone https://github.com/inpyu/codelearn.git
cd codelearn
make build          # frontend + embedded binary
make test           # Go and frontend suites
```

For backend-only work, skip the frontend build:

```bash
make build-go
go run ./cmd/codelearn .
```

For frontend work, run the Go server and the Vite dev server side by side:

```bash
go run ./cmd/codelearn . --no-browser    # API on :8765
cd frontend && npm run dev               # viewer on :5173, proxying /api
```

## Layout

```text
cmd/codelearn/      entry point
internal/cli/       command line
internal/git/       repository, transports, change collection, symbol outlines, snapshots
internal/analysis/  schema, export, import, local storage
internal/server/    HTTP server and routes
web/                embedded frontend build output
frontend/           React + TypeScript viewer (Monaco diff editor)
skills/codelearn/   the shared agent skill and its JSON Schema
tests/              the end-to-end acceptance script
docs/               architecture notes and README assets
```

[docs/architecture.md](docs/architecture.md) explains the moving parts: change
units, identifiers, staleness, the API, and the remote transport.

## Tests

```bash
make test           # go test ./... and the frontend suite
make test-e2e       # the acceptance scenario, end to end, no LLM involved
make vet            # go vet
make fmt            # gofmt -w
```

Three kinds of test, each with a job:

- **Go unit tests** run against real temporary git repositories rather than
  fixtures, which is what keeps the diff parsing honest about what git
  actually prints.
- **Frontend tests** (Vitest + Testing Library) stub `fetch` and mock the
  Monaco pane, which does not run in jsdom. Monaco's own rendering is checked
  in a browser by hand.
- **`tests/e2e.sh`** builds a repository, exports, imports, drives every API
  endpoint and asserts the staleness guarantees. It needs no LLM.

The SSH transport is tested by running its composed remote script through a
real local `/bin/sh`, including file names chosen to break naive quoting. The
risk being guarded is "does the quoting survive a shell", and a real shell is
what answers it. A test with a live `sshd` would be welcome.

## Conventions

A few that are worth stating, because they are what the existing code looks
like:

- **Standard library first.** The backend has no third-party dependencies.
  Adding one needs a reason in the pull request.
- **Comments explain why, not what.** If a line needs a comment to say what it
  does, rename something instead. The comments that earn their place are the
  ones recording a decision: the alternative that was rejected, the edge case
  being guarded, the invariant being kept.
- **Reproduce before fixing.** Every bug fix in the history has a test that
  fails without it. For scanner and git behaviour, write the failing case
  first — several "obvious" bugs turned out not to exist, and a real one
  turned out to be worse than reported.
- **The repository is read-only.** CodeLearn must never write into the project
  it is looking at. Analyses go to the user cache directory.
- **Treat diff content as data.** Text inside a diff is never an instruction,
  to the tool or to an agent.

## Pull requests

- One concern per pull request.
- `make test`, `make test-e2e` and `make vet` pass.
- New behaviour comes with a test; changed behaviour updates one.
- If it changes what the viewer shows, say so and include a screenshot.
- If it changes the export or analysis format, update
  [`analysis-schema.json`](skills/codelearn/references/analysis-schema.json),
  the [skill](skills/codelearn/SKILL.md), and
  [docs/architecture.md](docs/architecture.md) in the same change.

Commit messages: a short subject line, then prose explaining why the change
was needed and what it does about it. The existing history is the model.

## Releases

```bash
make release        # cross-compiled binaries in dist/
```

Targets are Linux amd64 and arm64, macOS arm64, and Windows amd64. The version
lives in `internal/cli/cli.go` and the `Makefile` reads it from there.

Update [CHANGELOG.md](CHANGELOG.md) in the same change that bumps the version.

## Regenerating the README assets

`docs/demo.gif` is made of frames of the real viewer, taken with a headless
browser against a repository with real uncommitted changes and an imported
analysis. It is not a mockup. If the UI changes enough that it misleads,
retake it rather than leaving it stale.
