# Changelog

All notable changes to CodeLearn are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project
follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Repositories on another machine.** `--remote [user@]host:/path` drives git
  over SSH, so you can install CodeLearn on your laptop and read a repository
  you are editing on a server. Nothing is installed on the host; it needs
  `git` and a POSIX shell. Accepted by the viewer, `export` and `import`.
  Connections are multiplexed over one SSH master, and working tree files are
  read in a single framed request rather than a round trip each.
- Every argument sent to a remote shell is quoted, with tests that drive
  hostile file names through a real `/bin/sh`.
- Analyses are cached per location, so the same project on a laptop and on a
  server keeps separate explanations.

### Changed

- **The Learning Guide lists every unit of the selected file**, explained or
  not, and is the single place to navigate them. The row of chips over the
  diff is gone; the diff header names the current unit, and clicking code
  selects the unit containing that line.
- The panes are resizable, with widths remembered per browser, and the header
  and file list carry repository totals. Fixed widths had been costing the
  diff enough room to truncate ordinary lines of code.
- The Vite build rewrites `web/dist/.gitkeep` itself, so a fresh clone
  compiles before the frontend has ever been built regardless of how it was
  built.

### Fixed

- A brace inside a **multi-line block comment** no longer ends the enclosing
  function early, and code quoted inside a **multi-line raw string** is no
  longer matched as a declaration — the outline used to invent a symbol that
  did not exist in the file. `/* ... */` doc comments are now attached to the
  symbol they document.

## [0.3.0]

The first working implementation: a Go CLI and HTTP server with an embedded
React viewer.

### Added

- `codelearn .` opens a local viewer on the uncommitted changes of a git
  repository: a side-by-side Monaco diff and a stepwise What / How / Why
  guide anchored to the code.
- `codelearn export` emits change units as JSON; `codelearn import` validates
  an agent's analysis and stores it in the user cache directory.
- The shared, agent-independent [skill](skills/codelearn/SKILL.md) and its
  JSON Schema.
- Change units are symbol-level — one function, method or class — where a file
  can be outlined, falling back to raw diff hunks otherwise. This amends the
  specification, which defined the unit as the hunk.
- A single binary per platform with the viewer embedded.

### Fixed

- **Staged-only changes were invisible.** A change that was staged and then
  reverted in the working tree produced no `git diff HEAD` output at all, so
  the file vanished from the viewer. Such files are now found through
  porcelain status, compared against the index, and marked `staged only`.
- Python definitions with multi-line signatures lost their entire body;
  TypeScript generic arrow functions were not recognised; multi-line type
  aliases were clipped to their first line.
- Change ids no longer hash line ranges, so inserting one import stops
  invalidating the explanation of every unit below it.
- Collection cost: `git cat-file --batch` replaced one `git show` per file,
  and the server holds its snapshot instead of re-collecting on every request.
  On 1,000 modified files, opening the viewer went from 5.5 s to 0.012 s.

[Unreleased]: https://github.com/inpyu/codelearn/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/inpyu/codelearn/releases/tag/v0.3.0
