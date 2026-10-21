// Package git collects uncommitted changes by driving the git command line.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// EmptyTree is git's well known hash for a tree with no entries. It is used as
// the comparison base in a repository without commits, so that case needs no
// special handling anywhere downstream.
const EmptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// commandTimeout bounds every git invocation so a hung git - or an
// unresponsive SSH host - cannot hang the viewer.
const commandTimeout = 30 * time.Second

// ErrNotARepository is returned when a path is not inside a git repository.
var ErrNotARepository = errors.New("not a git repository")

// Repository is a git repository, addressed by its root directory and reached
// through a transport. It caches nothing: every question is answered by
// running git again, so a long running viewer can never serve a stale answer.
type Repository struct {
	Root      string
	transport Transport
}

// Discover finds the repository containing path on this machine, which may be
// a file, a directory, or empty for the working directory.
func Discover(ctx context.Context, path string) (*Repository, error) {
	if path == "" {
		path = "."
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("path does not exist: %s", path)
	}
	if !info.IsDir() {
		abs = filepath.Dir(abs)
	}
	repo, err := discoverWith(ctx, Local(), abs, path)
	if err != nil {
		return nil, err
	}
	// Resolving symlinks here is what makes the containment check sound: it
	// assumes both sides are already resolved.
	if real, err := filepath.EvalSymlinks(repo.Root); err == nil {
		repo.Root = real
	}
	return repo, nil
}

// DiscoverRemote finds the repository containing path on a transport's host.
// Nothing is installed there: it only needs git and a POSIX shell.
func DiscoverRemote(ctx context.Context, transport Transport, path string) (*Repository, error) {
	if path == "" {
		return nil, fmt.Errorf("a remote repository needs a path")
	}
	return discoverWith(ctx, transport, path, path)
}

func discoverWith(ctx context.Context, transport Transport, dir, shown string) (*Repository, error) {
	// Let git resolve the root rather than walking up looking for a .git
	// directory: that is what makes worktrees and submodules work for free.
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	out, err := transport.Run(ctx, dir, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("git executable not found: %w", err)
		}
		return nil, fmt.Errorf("%w: %s (%s)", ErrNotARepository, shown, transport.Label())
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return nil, fmt.Errorf("%w: %s", ErrNotARepository, shown)
	}
	return &Repository{Root: root, transport: transport}, nil
}

// Name is the repository's directory name. Git has no authoritative project
// name - a remote may be absent or renamed - so the directory is the one label
// that always exists and matches what the user typed.
func (r *Repository) Name() string {
	return filepath.Base(filepath.ToSlash(r.Root))
}

// Location describes where the repository lives, for the CLI banner.
func (r *Repository) Location() string {
	if r.transport.Label() == "local" {
		return r.Root
	}
	return r.transport.Label() + r.Root
}

// Remote reports whether the repository lives on another host.
func (r *Repository) Remote() bool { return r.transport.Label() != "local" }

// RunBytes executes a git command in the repository and returns raw stdout.
// Bytes rather than a string, because the caller decides what counts as text.
func (r *Repository) RunBytes(ctx context.Context, args ...string) ([]byte, error) {
	return r.runWithInput(ctx, nil, args...)
}

func (r *Repository) runWithInput(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	// core.quotepath=false keeps non-ASCII paths unescaped.
	full := append([]string{"-c", "core.quotepath=false"}, args...)
	return r.transport.Run(ctx, r.Root, stdin, full...)
}

// Run executes a git command and returns stdout as a string.
func (r *Repository) Run(ctx context.Context, args ...string) (string, error) {
	out, err := r.RunBytes(ctx, args...)
	return string(out), err
}

// HeadCommit is the current commit hash, or "" in a repository with no
// commits. That is a normal state to display, not an error.
func (r *Repository) HeadCommit(ctx context.Context) string {
	out, err := r.RunBytes(ctx, "rev-parse", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Branch names the current branch. The two git commands fail in opposite
// situations - rev-parse prints "HEAD" when detached but works on an unborn
// branch, symbolic-ref is the reverse - so both are tried.
func (r *Repository) Branch(ctx context.Context) string {
	if out, err := r.RunBytes(ctx, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		if name := strings.TrimSpace(string(out)); name != "" && name != "HEAD" {
			return name
		}
	}
	if out, err := r.RunBytes(ctx, "symbolic-ref", "--short", "HEAD"); err == nil {
		if name := strings.TrimSpace(string(out)); name != "" {
			return name
		}
	}
	return "(detached)"
}

// BaseRev is the revision the working tree is compared against. This is the
// single place the no-commit case is handled.
func (r *Repository) BaseRev(ctx context.Context) string {
	if r.HeadCommit(ctx) != "" {
		return "HEAD"
	}
	return EmptyTree
}

// ReadAtRev returns a file's content as of rev. The "before" side of a diff is
// not on disk at all, so it has to come from the object store. A missing path
// reports ok=false rather than an error, which is what lets an added file flow
// through the same code path as a modified one.
//
// An empty rev lets the caller pass a complete object name, such as ":path"
// for stage 0 of the index.
func (r *Repository) ReadAtRev(ctx context.Context, rev, path string) ([]byte, bool) {
	object := path
	if rev != "" {
		object = rev + ":" + path
	}
	out, err := r.RunBytes(ctx, "show", object)
	if err != nil {
		return nil, false
	}
	return out, true
}

// CheckInside refuses a repository-relative path that would escape it.
//
// Paths reach this layer from the HTTP API and from imported analyses, so
// traversal is rejected outright rather than cleaned away: filepath.Clean
// would quietly rewrite "../x" into "x", turning an attack into a successful
// read of a different file.
func (r *Repository) CheckInside(path string) error {
	if path == "" {
		return fmt.Errorf("empty path")
	}
	if filepath.IsAbs(path) || strings.HasPrefix(path, "/") {
		return fmt.Errorf("absolute path not allowed: %s", path)
	}
	cleaned := filepath.ToSlash(filepath.Clean(path))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
		return fmt.Errorf("path escapes the repository: %s", path)
	}
	return nil
}

// ReadWorktree reads one working tree file.
func (r *Repository) ReadWorktree(ctx context.Context, path string) ([]byte, bool) {
	files := r.ReadWorktreeFiles(ctx, []string{path})
	data, ok := files[path]
	return data, ok
}

// ReadWorktreeFiles reads many working tree files at once.
//
// Batching matters for a remote repository, where a round trip per file would
// dominate collection; the local transport simply loops.
func (r *Repository) ReadWorktreeFiles(ctx context.Context, paths []string) map[string][]byte {
	safe := make([]string, 0, len(paths))
	for _, path := range paths {
		if r.CheckInside(path) == nil {
			safe = append(safe, path)
		}
	}
	if len(safe) == 0 {
		return map[string][]byte{}
	}
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	return r.transport.ReadFiles(ctx, r.Root, safe)
}

// ReadManyAtRev reads many blobs from one revision in a single git process.
//
// Reading the "before" side one `git show` at a time costs one subprocess per
// changed file - and, on a remote repository, one round trip per file.
// `cat-file --batch` streams them all through one invocation.
func (r *Repository) ReadManyAtRev(ctx context.Context, rev string, paths []string) map[string][]byte {
	result := make(map[string][]byte, len(paths))
	var batched []string
	for _, path := range paths {
		// A path containing a newline cannot be expressed in the batch
		// protocol's line-delimited input, so it falls back to one call.
		if strings.ContainsAny(path, "\n\r") {
			if data, ok := r.ReadAtRev(ctx, rev, path); ok {
				result[path] = data
			}
			continue
		}
		batched = append(batched, path)
	}
	if len(batched) == 0 {
		return result
	}

	var input strings.Builder
	for _, path := range batched {
		input.WriteString(rev + ":" + path + "\n")
	}
	out, err := r.runWithInput(ctx, []byte(input.String()), "cat-file", "--batch")
	if err != nil {
		// Fall back to one call per path rather than losing the before side.
		for _, path := range batched {
			if data, ok := r.ReadAtRev(ctx, rev, path); ok {
				result[path] = data
			}
		}
		return result
	}
	parseBatch(out, batched, result)
	return result
}

// parseBatch walks the `cat-file --batch` response, which is a header line of
// "<sha> <type> <size>" followed by that many bytes and a newline, or
// "<object> missing" for anything that does not exist.
func parseBatch(out []byte, paths []string, into map[string][]byte) {
	offset := 0
	for _, path := range paths {
		newline := bytes.IndexByte(out[offset:], '\n')
		if newline < 0 {
			return
		}
		header := string(out[offset : offset+newline])
		offset += newline + 1

		fields := strings.Fields(header)
		if len(fields) < 3 {
			continue // "<object> missing": nothing to read, nothing to skip
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || offset+size > len(out) {
			return
		}
		content := make([]byte, size)
		copy(content, out[offset:offset+size])
		into[path] = content
		offset += size + 1 // the trailing newline git appends
	}
}
