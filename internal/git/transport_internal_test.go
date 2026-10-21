package git

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// shellBacked returns the SSH transport with its ssh client replaced by a
// local POSIX shell.
//
// SSH hands its command to the remote login shell, so the risk this guards is
// "does the quoting survive a shell". Running the composed script through a
// real /bin/sh tests exactly that, without needing an sshd.
func shellBacked() *sshTransport {
	return &sshTransport{
		host: "shell",
		run: func(ctx context.Context, stdin []byte, script string) ([]byte, error) {
			cmd := exec.CommandContext(ctx, "sh", "-c", script)
			if stdin != nil {
				cmd.Stdin = bytes.NewReader(stdin)
			}
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				return nil, err
			}
			return stdout.Bytes(), nil
		},
	}
}

// hostile covers the characters a shell would otherwise act on. Every name is
// legal on a POSIX filesystem, so these double as real file names.
var hostile = []string{
	"plain.txt",
	"with space.txt",
	"it's.txt",
	"$(touch pwned).txt",
	"`touch pwned`.txt",
	"semi;colon.txt",
	"pipe|bar.txt",
	"dollar$VAR.txt",
	"new\tline.txt",
	"quote\".txt",
	"star*.txt",
	"tilde~.txt",
	"-dash-leading.txt",
	"back\\slash.txt",
}

func TestQuotedArgumentsSurviveARealShell(t *testing.T) {
	transport := shellBacked()

	for _, argument := range hostile {
		script := "printf '%s' " + shellQuote(argument)
		out, err := transport.run(context.Background(), nil, script)
		if err != nil {
			t.Fatalf("%q: %v", argument, err)
		}
		if string(out) != argument {
			t.Fatalf("%q came back as %q", argument, string(out))
		}
	}
}

func TestQuotingDoesNotLetAPathRunACommand(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pwned")
	transport := shellBacked()

	// A file name is attacker-controlled in the sense that a repository can
	// contain anything; none of these may execute.
	for _, argument := range []string{
		"$(touch " + marker + ")",
		"`touch " + marker + "`",
		"; touch " + marker,
		"&& touch " + marker,
		"' ; touch " + marker + " ; '",
	} {
		if _, err := transport.run(
			context.Background(), nil, "printf '%s' "+shellQuote(argument),
		); err != nil {
			t.Fatalf("%q: %v", argument, err)
		}
		if _, err := os.Stat(marker); err == nil {
			t.Fatalf("%q executed on the remote side", argument)
		}
	}
}

func TestReadFilesScriptFramesContentThroughARealShell(t *testing.T) {
	dir := t.TempDir()
	want := map[string][]byte{}
	for index, name := range hostile {
		content := bytes.Repeat([]byte{byte('a' + index)}, index*7)
		content = append(content, []byte("\nline two\n")...)
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
			t.Fatalf("writing %q: %v", name, err)
		}
		want[name] = content
	}

	got := shellBacked().ReadFiles(context.Background(), dir, hostile)

	for name, content := range want {
		if !bytes.Equal(got[name], content) {
			t.Fatalf("%q: got %q, want %q", name, got[name], content)
		}
	}
	// A name that tries to run a command must not have run it.
	if _, err := os.Stat(filepath.Join(dir, "pwned")); err == nil {
		t.Fatal("a file name executed during the read")
	}
}

func TestReadFilesRefusesWhatIsNotARegularFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "real.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "a-directory"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "link.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	got := shellBacked().ReadFiles(context.Background(), dir,
		[]string{"real.txt", "a-directory", "link.txt", "missing.txt"})

	if string(got["real.txt"]) != "ok\n" {
		t.Fatalf("real.txt = %q", got["real.txt"])
	}
	// A symlink is refused rather than followed, so a link pointing out of the
	// repository cannot be read - the same rule the local transport applies.
	for _, name := range []string{"a-directory", "link.txt", "missing.txt"} {
		if _, present := got[name]; present {
			t.Fatalf("%q was returned", name)
		}
	}
}

func TestBinaryContentSurvivesTheFraming(t *testing.T) {
	dir := t.TempDir()
	content := []byte{0, 1, 2, '\n', 255, '-', '\n', 0}
	if err := os.WriteFile(filepath.Join(dir, "blob.bin"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	got := shellBacked().ReadFiles(context.Background(), dir, []string{"blob.bin"})

	if !bytes.Equal(got["blob.bin"], content) {
		t.Fatalf("got %v, want %v", got["blob.bin"], content)
	}
}

// TestRemotePathProducesTheSameSnapshotAsTheLocalOne is the end-to-end check
// for the remote transport: the same repository, collected twice, once by
// running git directly and once by composing a shell command the way SSH
// does. Anything the quoting or the framing got wrong shows up as a
// difference here.
func TestRemotePathProducesTheSameSnapshotAsTheLocalOne(t *testing.T) {
	root := buildSampleRepository(t)
	ctx := context.Background()

	local, err := Collect(ctx, &Repository{Root: root, transport: Local()})
	if err != nil {
		t.Fatalf("local collect: %v", err)
	}
	remote, err := Collect(ctx, &Repository{Root: root, transport: shellBacked()})
	if err != nil {
		t.Fatalf("remote collect: %v", err)
	}

	if local.ID != remote.ID {
		t.Fatalf("snapshot ids differ:\n local  %s\n remote %s", local.ID, remote.ID)
	}
	if len(local.Files) != len(remote.Files) {
		t.Fatalf("file counts differ: %d vs %d", len(local.Files), len(remote.Files))
	}
	for index := range local.Files {
		a, b := local.Files[index], remote.Files[index]
		if a.Path != b.Path || a.Status != b.Status || a.Before != b.Before || a.After != b.After {
			t.Fatalf("file %d differs:\n local  %+v\n remote %+v", index, a, b)
		}
		if len(a.Changes) != len(b.Changes) {
			t.Fatalf("%s: unit counts differ: %d vs %d", a.Path, len(a.Changes), len(b.Changes))
		}
		for unit := range a.Changes {
			if a.Changes[unit].ID != b.Changes[unit].ID {
				t.Fatalf("%s: unit %d ids differ", a.Path, unit)
			}
		}
	}
}

// buildSampleRepository makes a repository exercising every change type, with
// file names chosen to break naive shell quoting.
func buildSampleRepository(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "sample")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatalf("writing %q: %v", name, err)
		}
	}

	run("init", "-q", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	run("config", "commit.gpgsign", "false")

	write("mod.go", "package m\n\nfunc A() int {\n\treturn 1\n}\n")
	write("it's a file.txt", "original\n")
	write("$(echo hi).txt", "original\n")
	write("gone.txt", "bye\n")
	run("add", "-A")
	run("commit", "-q", "-m", "init")

	write("mod.go", "package m\n\nfunc A() int {\n\treturn 2\n}\n")
	write("it's a file.txt", "edited\n")
	write("$(echo hi).txt", "edited\n")
	if err := os.Remove(filepath.Join(root, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	write("new file; with semicolon.txt", "fresh\n")
	write("staged.txt", "staged\n")
	run("add", "staged.txt")
	write("staged.txt", "")
	if err := os.Remove(filepath.Join(root, "staged.txt")); err != nil {
		t.Fatal(err)
	}
	return root
}
