package analysis_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inpyu/codelearn/internal/analysis"
	"github.com/inpyu/codelearn/internal/git"
	"github.com/inpyu/codelearn/internal/testrepo"
)

// prepared builds a repository with one changed function.
func prepared(t *testing.T) (*testrepo.Repo, *git.Repository, *git.Snapshot) {
	t.Helper()
	r := testrepo.New(t)
	r.Write("cache.go", "package cache\n\nfunc Get() int {\n\treturn create()\n}\n")
	r.Commit("init")
	r.Write("cache.go", "package cache\n\nfunc Get() int {\n\treturn reuse()\n}\n")
	repo, snapshot := snapshotOf(t, r)
	return r, repo, snapshot
}

func snapshotOf(t *testing.T, r *testrepo.Repo) (*git.Repository, *git.Snapshot) {
	t.Helper()
	repo, err := git.Discover(context.Background(), r.Root)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := git.Collect(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	return repo, snapshot
}

func writeAnalysis(t *testing.T, dir string, doc map[string]any) string {
	t.Helper()
	path := filepath.Join(dir, "analysis.json")
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func validDoc(snapshot *git.Snapshot) map[string]any {
	return map[string]any{
		"schema_version": "1.0",
		"snapshot_id":    snapshot.ID,
		"generator":      "test-agent",
		"steps": []map[string]any{{
			"change_id": snapshot.Changes()[0].ID,
			"title":     "Cache reuse",
			"what":      "Creation became reuse.",
			"how":       "The existing cache object is returned.",
		}},
	}
}

func TestExportShape(t *testing.T) {
	_, _, snapshot := prepared(t)
	export := analysis.BuildExport(snapshot)

	if export.SchemaVersion != "1.0" || export.SnapshotID != snapshot.ID {
		t.Fatalf("header = %+v", export)
	}
	if len(export.Changes) == 0 || export.Changes[0].File != "cache.go" {
		t.Fatalf("changes = %+v", export.Changes)
	}
	if export.Changes[0].Label != "Get()" {
		t.Fatalf("label = %q", export.Changes[0].Label)
	}
}

func TestWriteExportCreatesReadableJSON(t *testing.T) {
	_, _, snapshot := prepared(t)
	path := filepath.Join(t.TempDir(), "nested", "changes.json")

	if err := analysis.WriteExport(snapshot, path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var export analysis.Export
	if err := json.Unmarshal(data, &export); err != nil {
		t.Fatal(err)
	}
	if len(export.Changes) == 0 {
		t.Fatal("no changes in the written export")
	}
}

func TestImportAcceptsAMatchingAnalysis(t *testing.T) {
	_, repo, snapshot := prepared(t)
	path := writeAnalysis(t, t.TempDir(), validDoc(snapshot))

	result, err := analysis.ImportFile(repo.Root, snapshot, path)
	if err != nil {
		t.Fatalf("import failed: %v (%v)", err, result)
	}
	if result.Steps != 1 {
		t.Fatalf("steps = %d", result.Steps)
	}
	if strings.HasPrefix(result.StoredPath, repo.Root) {
		t.Fatalf("analysis was stored inside the repository: %s", result.StoredPath)
	}
}

func TestImportIsAtomicWhenOneStepIsUnknown(t *testing.T) {
	_, repo, snapshot := prepared(t)
	doc := validDoc(snapshot)
	steps := doc["steps"].([]map[string]any)
	doc["steps"] = append(steps, map[string]any{
		"change_id": "sha256:" + strings.Repeat("f", 64),
		"title":     "Nowhere",
		"what":      "x",
		"how":       "y",
	})
	path := writeAnalysis(t, t.TempDir(), doc)

	if _, err := analysis.ImportFile(repo.Root, snapshot, path); err == nil {
		t.Fatal("expected rejection")
	}
	// Nothing may be stored: a half-applied analysis must not exist on disk.
	if view := analysis.StoredView(repo.Root, snapshot); view.Available {
		t.Fatal("a rejected import left data behind")
	}
}

func TestImportRejectsASnapshotMismatch(t *testing.T) {
	_, repo, snapshot := prepared(t)
	doc := validDoc(snapshot)
	doc["snapshot_id"] = "sha256:" + strings.Repeat("b", 64)
	path := writeAnalysis(t, t.TempDir(), doc)

	result, err := analysis.ImportFile(repo.Root, snapshot, path)
	if err == nil {
		t.Fatal("expected rejection")
	}
	if !strings.Contains(strings.Join(result.Problems, " "), "snapshot_id") {
		t.Fatalf("problems = %v", result.Problems)
	}
}

func TestImportRejectsDuplicateAndMalformedSteps(t *testing.T) {
	_, repo, snapshot := prepared(t)

	duplicate := validDoc(snapshot)
	steps := duplicate["steps"].([]map[string]any)
	duplicate["steps"] = append(steps, steps[0])
	if _, err := analysis.ImportFile(repo.Root, snapshot, writeAnalysis(t, t.TempDir(), duplicate)); err == nil {
		t.Fatal("duplicate change id was accepted")
	}

	missing := validDoc(snapshot)
	missing["steps"].([]map[string]any)[0]["how"] = ""
	if _, err := analysis.ImportFile(repo.Root, snapshot, writeAnalysis(t, t.TempDir(), missing)); err == nil {
		t.Fatal("a step without 'how' was accepted")
	}

	version := validDoc(snapshot)
	version["schema_version"] = "2.0"
	if _, err := analysis.ImportFile(repo.Root, snapshot, writeAnalysis(t, t.TempDir(), version)); err == nil {
		t.Fatal("a wrong schema version was accepted")
	}
}

func TestImportRejectsUnknownFieldsAndMissingFiles(t *testing.T) {
	_, repo, snapshot := prepared(t)
	doc := validDoc(snapshot)
	doc["steps"].([]map[string]any)[0]["line"] = 12
	if _, err := analysis.ImportFile(repo.Root, snapshot, writeAnalysis(t, t.TempDir(), doc)); err == nil {
		t.Fatal("an invented field was accepted")
	}
	if _, err := analysis.ImportFile(repo.Root, snapshot, filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("a missing file was accepted")
	}
}

func TestImportRejectsADifferentRepository(t *testing.T) {
	_, repo, snapshot := prepared(t)
	doc := validDoc(snapshot)
	doc["repository"] = map[string]any{"name": "other-project"}

	result, err := analysis.ImportFile(repo.Root, snapshot, writeAnalysis(t, t.TempDir(), doc))
	if err == nil {
		t.Fatal("expected rejection")
	}
	if !strings.Contains(strings.Join(result.Problems, " "), "repository") {
		t.Fatalf("problems = %v", result.Problems)
	}
}

func TestStoredViewExposesTheCodeLocation(t *testing.T) {
	_, repo, snapshot := prepared(t)
	if _, err := analysis.ImportFile(repo.Root, snapshot, writeAnalysis(t, t.TempDir(), validDoc(snapshot))); err != nil {
		t.Fatal(err)
	}

	view := analysis.StoredView(repo.Root, snapshot)
	if !view.Available || view.Stale || view.SnapshotChanged {
		t.Fatalf("view = %+v", view)
	}
	step := view.Steps[0]
	if step.File != "cache.go" || step.Label != "Get()" {
		t.Fatalf("step = %+v", step)
	}
	if step.NewRange != snapshot.Changes()[0].NewRange {
		t.Fatalf("range = %+v", step.NewRange)
	}
}

func TestStaleExplanationsAreNotAttachedToChangedCode(t *testing.T) {
	r, repo, snapshot := prepared(t)
	if _, err := analysis.ImportFile(repo.Root, snapshot, writeAnalysis(t, t.TempDir(), validDoc(snapshot))); err != nil {
		t.Fatal(err)
	}

	// The explained function itself changes.
	r.Write("cache.go", "package cache\n\nfunc Get() int {\n\treturn shared()\n}\n")
	_, updated := snapshotOf(t, r)

	view := analysis.StoredView(repo.Root, updated)
	if view.Available || !view.Stale || view.Dropped != 1 {
		t.Fatalf("view = %+v", view)
	}
	if len(view.Steps) != 0 {
		t.Fatalf("a stale step survived: %+v", view.Steps)
	}
}

func TestAnUnrelatedEditDoesNotInvalidateTheAnalysis(t *testing.T) {
	r, repo, snapshot := prepared(t)
	if _, err := analysis.ImportFile(repo.Root, snapshot, writeAnalysis(t, t.TempDir(), validDoc(snapshot))); err != nil {
		t.Fatal(err)
	}

	// A new file elsewhere moves the snapshot but touches no explained unit.
	r.Write("unrelated.txt", "noise\n")
	_, updated := snapshotOf(t, r)

	view := analysis.StoredView(repo.Root, updated)
	if !view.Available || view.Stale || !view.SnapshotChanged || len(view.Steps) != 1 {
		t.Fatalf("view = %+v", view)
	}
}

func TestRepositoryIDSeparatesTwoCheckoutsOfOneProject(t *testing.T) {
	first := analysis.RepositoryID(filepath.Join(t.TempDir(), "a", "CodeLearn"))
	second := analysis.RepositoryID(filepath.Join(t.TempDir(), "b", "CodeLearn"))

	if first == second {
		t.Fatal("two checkouts share a cache id")
	}
	if !strings.HasPrefix(first, "CodeLearn-") {
		t.Fatalf("id is not recognisable: %s", first)
	}
}
