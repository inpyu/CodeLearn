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
	"strings"
	"time"
)

// EmptyTree is git's well known hash for a tree with no entries. It is used as
// the comparison base in a repository without commits, so that case needs no
// special handling anywhere downstream.
const EmptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// commandTimeout bounds every git invocation so a hung git cannot hang the
// viewer, which serves one request per git call.
const commandTimeout = 30 * time.Second

// ErrNotARepository is returned when a path is not inside a git repository.
var ErrNotARepository = errors.New("not a git repository")

// Repository is a local git repository, addressed by its root directory.
// It caches nothing: every question is answered by running git again, so a
// long running viewer can never serve a stale answer.
type Repository struct {
	Root string
}

// Discover finds the repository containing path, which may be a file, a
// directory, or empty for the working directory.
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
	// Let git resolve the root rather than walking up looking for a .git
	// directory: that is what makes worktrees and submodules work for free.
	out, err := runIn(ctx, abs, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrNotARepository, path)
	}
	root, err := filepath.EvalSymlinks(strings.TrimSpace(string(out)))
	if err != nil {
		root = strings.TrimSpace(string(out))
	}
	return &Repository{Root: root}, nil
}

// Name is the repository's directory name. Git has no authoritative project
// name - a remote may be absent or renamed - so the directory is the one
// label that always exists and matches what the user typed.
func (r *Repository) Name() string {
	return filepath.Base(r.Root)
}

func runIn(ctx context.Context, dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	// core.quotepath=false keeps non-ASCII paths unescaped. Arguments are
	// passed as a slice, never interpolated into a shell string.
	full := append([]string{"-C", dir, "-c", "core.quotepath=false"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("git %s timed out", strings.Join(args, " "))
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.Bytes(), nil
}

// RunBytes executes a git command in the repository and returns raw stdout.
// Bytes rather than a string, because the caller decides what counts as text.
func (r *Repository) RunBytes(ctx context.Context, args ...string) ([]byte, error) {
	return runIn(ctx, r.Root, args...)
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
func (r *Repository) ReadAtRev(ctx context.Context, rev, path string) ([]byte, bool) {
	out, err := r.RunBytes(ctx, "show", rev+":"+path)
	if err != nil {
		return nil, false
	}
	return out, true
}

// ResolveInside resolves a repository-relative path and refuses anything that
// escapes the repository. Resolving before comparing is the part that matters:
// it collapses ".." segments and follows symlinks first.
func (r *Repository) ResolveInside(path string) (string, error) {
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("absolute path not allowed: %s", path)
	}
	// Reject traversal outright rather than letting Clean rewrite it: an
	// escaping path must be an error, not a silent read of a different file.
	cleaned := filepath.ToSlash(filepath.Clean(path))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
		return "", fmt.Errorf("path escapes the repository: %s", path)
	}
	joined := filepath.Join(r.Root, cleaned)
	resolved := joined
	if real, err := filepath.EvalSymlinks(joined); err == nil {
		resolved = real
	}
	rel, err := filepath.Rel(r.Root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes the repository: %s", path)
	}
	return joined, nil
}

// ReadWorktree reads a working tree file. This is the only function that
// touches the filesystem directly, which is why the containment check is here.
func (r *Repository) ReadWorktree(path string) ([]byte, bool) {
	target, err := r.ResolveInside(path)
	if err != nil {
		return nil, false
	}
	info, err := os.Lstat(target)
	if err != nil || !info.Mode().IsRegular() {
		return nil, false
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return nil, false
	}
	return data, true
}
