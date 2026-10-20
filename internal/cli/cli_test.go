package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inpyu/codelearn/internal/analysis"
	"github.com/inpyu/codelearn/internal/cli"
	"github.com/inpyu/codelearn/internal/testrepo"
)

func prepared(t *testing.T) *testrepo.Repo {
	t.Helper()
	r := testrepo.New(t)
	r.Write("cache.go", "package cache\n\nfunc Get() int {\n\treturn create()\n}\n")
	r.Commit("init")
	r.Write("cache.go", "package cache\n\nfunc Get() int {\n\treturn reuse()\n}\n")
	return r
}

func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := cli.Run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestExportWritesTheChangeUnits(t *testing.T) {
	r := prepared(t)
	output := filepath.Join(t.TempDir(), "changes.json")

	code, stdout, stderr := run(t, "export", r.Root, "--output", output)
	if code != cli.ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Snapshot: sha256:") {
		t.Fatalf("stdout = %q", stdout)
	}

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var export analysis.Export
	if err := json.Unmarshal(data, &export); err != nil {
		t.Fatal(err)
	}
	if len(export.Changes) == 0 || export.Changes[0].File != "cache.go" {
		t.Fatalf("export = %+v", export.Changes)
	}
}

func TestImportRoundTrip(t *testing.T) {
	r := prepared(t)
	dir := t.TempDir()
	exportPath := filepath.Join(dir, "changes.json")

	if code, _, stderr := run(t, "export", r.Root, "-o", exportPath); code != cli.ExitOK {
		t.Fatalf("export failed: %s", stderr)
	}
	data, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	var export analysis.Export
	if err := json.Unmarshal(data, &export); err != nil {
		t.Fatal(err)
	}

	doc := analysis.Analysis{
		SchemaVersion: "1.0",
		SnapshotID:    export.SnapshotID,
		Steps: []analysis.Step{{
			ChangeID: export.Changes[0].ID,
			Title:    "Cache reuse",
			What:     "Creation became reuse.",
			How:      "The existing object is returned.",
		}},
	}
	analysisPath := filepath.Join(dir, "analysis.json")
	encoded, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(analysisPath, encoded, 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := run(t, "import", analysisPath, r.Root)
	if code != cli.ExitOK {
		t.Fatalf("import failed (%d): %s", code, stderr)
	}
	if !strings.Contains(stdout, "Imported 1 learning step(s)") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestImportOfAMissingFileFails(t *testing.T) {
	r := prepared(t)

	code, _, stderr := run(t, "import", filepath.Join(t.TempDir(), "nope.json"), r.Root)
	if code != cli.ExitInvalidInput {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(stderr, "file not found") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestAPathOutsideARepositoryIsReported(t *testing.T) {
	dir := t.TempDir()

	code, _, stderr := run(t, "export", dir, "-o", filepath.Join(dir, "x.json"))
	if code != cli.ExitGitError {
		t.Fatalf("exit = %d, want %d", code, cli.ExitGitError)
	}
	if !strings.Contains(stderr, "not a git repository") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestVersionAndHelp(t *testing.T) {
	if code, stdout, _ := run(t, "--version"); code != cli.ExitOK || !strings.Contains(stdout, "0.3.0") {
		t.Fatalf("version: exit=%d stdout=%q", code, stdout)
	}
	if code, stdout, _ := run(t, "--help"); code != cli.ExitOK || !strings.Contains(stdout, "Usage:") {
		t.Fatalf("help: exit=%d stdout=%q", code, stdout)
	}
}
