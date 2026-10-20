// Package testrepo builds throwaway git repositories for tests.
//
// Tests run against real git output rather than fixtures, which is what keeps
// the diff parsing honest about what git actually prints.
package testrepo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Repo is a temporary git repository.
type Repo struct {
	t    *testing.T
	Root string
}

// New creates an initialised repository and redirects the analysis cache into
// the test's temporary directory, so tests never touch a real user cache.
func New(t *testing.T) *Repo {
	t.Helper()
	root := filepath.Join(t.TempDir(), "sample-repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODELEARN_CACHE_DIR", filepath.Join(t.TempDir(), "cache"))

	r := &Repo{t: t, Root: root}
	r.Git("init", "-q", "-b", "main")
	r.Git("config", "user.email", "test@example.com")
	r.Git("config", "user.name", "Test")
	r.Git("config", "commit.gpgsign", "false")
	return r
}

// Git runs a git command in the repository and fails the test on error.
func (r *Repo) Git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", r.Root}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// Write creates or replaces a file.
func (r *Repo) Write(path, content string) {
	r.t.Helper()
	target := filepath.Join(r.Root, path)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

// WriteBytes creates or replaces a file with raw bytes.
func (r *Repo) WriteBytes(path string, content []byte) {
	r.t.Helper()
	if err := os.WriteFile(filepath.Join(r.Root, path), content, 0o644); err != nil {
		r.t.Fatal(err)
	}
}

// Remove deletes a file from the working tree.
func (r *Repo) Remove(path string) {
	r.t.Helper()
	if err := os.Remove(filepath.Join(r.Root, path)); err != nil {
		r.t.Fatal(err)
	}
}

// Commit stages everything and commits.
func (r *Repo) Commit(message string) {
	r.t.Helper()
	r.Git("add", "-A")
	r.Git("commit", "-q", "-m", message)
}
