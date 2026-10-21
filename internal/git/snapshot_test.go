package git_test

import (
	"context"
	"testing"

	"github.com/inpyu/codelearn/internal/git"
	"github.com/inpyu/codelearn/internal/testrepo"
)

func collect(t *testing.T, r *testrepo.Repo) *git.Snapshot {
	t.Helper()
	repo, err := git.Discover(context.Background(), r.Root)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	snapshot, err := git.Collect(context.Background(), repo)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	return snapshot
}

func fileOf(t *testing.T, snapshot *git.Snapshot, path string) git.FileChange {
	t.Helper()
	file, ok := snapshot.File(path)
	if !ok {
		var paths []string
		for _, f := range snapshot.Files {
			paths = append(paths, f.Path)
		}
		t.Fatalf("file %q not in snapshot; have %v", path, paths)
	}
	return file
}

func labels(file git.FileChange) []string {
	out := make([]string, 0, len(file.Changes))
	for _, change := range file.Changes {
		out = append(out, change.Label)
	}
	return out
}

func TestDiscoverRejectsNonRepository(t *testing.T) {
	if _, err := git.Discover(context.Background(), t.TempDir()); err == nil {
		t.Fatal("expected an error outside a repository")
	}
}

func TestCleanRepositoryHasNoChanges(t *testing.T) {
	r := testrepo.New(t)
	r.Write("a.txt", "one\n")
	r.Commit("init")

	snapshot := collect(t, r)
	if len(snapshot.Files) != 0 {
		t.Fatalf("expected no changes, got %d", len(snapshot.Files))
	}
	if snapshot.BaseCommit == "" {
		t.Fatal("expected a base commit")
	}
}

func TestRepositoryWithoutCommits(t *testing.T) {
	r := testrepo.New(t)
	r.Write("a.txt", "one\n")

	snapshot := collect(t, r)
	if snapshot.BaseCommit != "" {
		t.Fatalf("expected no base commit, got %q", snapshot.BaseCommit)
	}
	if got := fileOf(t, snapshot, "a.txt").Status; got != git.StatusUntracked {
		t.Fatalf("status = %q", got)
	}
}

func TestModifiedFile(t *testing.T) {
	r := testrepo.New(t)
	r.Write("a.txt", "1\n2\n3\n4\n5\n6\n7\n8\n")
	r.Commit("init")
	r.Write("a.txt", "1\n2\n3\nfour\n5\n6\n7\n8\n")

	file := fileOf(t, collect(t, r), "a.txt")
	if file.Status != git.StatusModified {
		t.Fatalf("status = %q", file.Status)
	}
	if len(file.Changes) != 1 {
		t.Fatalf("expected 1 unit, got %d", len(file.Changes))
	}
	change := file.Changes[0]
	if change.Before != "4" || change.After != "four" {
		t.Fatalf("context was not trimmed: %q -> %q", change.Before, change.After)
	}
	if change.OldRange != (git.Range{Start: 4, Count: 1}) {
		t.Fatalf("old range = %+v", change.OldRange)
	}
}

func TestStagedAndUnstagedAreMergedOnce(t *testing.T) {
	r := testrepo.New(t)
	r.Write("a.txt", "1\n2\n3\n")
	r.Commit("init")
	r.Write("a.txt", "1\nstaged\n3\n")
	r.Git("add", "a.txt")
	r.Write("a.txt", "1\nstaged\nunstaged\n")

	snapshot := collect(t, r)
	if len(snapshot.Files) != 1 {
		t.Fatalf("expected the file once, got %d entries", len(snapshot.Files))
	}
	if got := fileOf(t, snapshot, "a.txt").After; got != "1\nstaged\nunstaged\n" {
		t.Fatalf("after = %q", got)
	}
}

func TestStagedThenDeletedFromWorkingTree(t *testing.T) {
	r := testrepo.New(t)
	r.Write("a.txt", "original\n")
	r.Commit("init")
	r.Write("a.txt", "staged\n")
	r.Git("add", "a.txt")
	r.Remove("a.txt")

	// The final state the user sees is "gone", and that is what must be shown
	// even though the index still holds a version of the file.
	file := fileOf(t, collect(t, r), "a.txt")
	if file.Status != git.StatusDeleted {
		t.Fatalf("status = %q, want deleted", file.Status)
	}
	if file.After != "" {
		t.Fatalf("after = %q, want empty", file.After)
	}
}

func TestAddedDeletedAndUntracked(t *testing.T) {
	r := testrepo.New(t)
	r.Write("keep.txt", "1\n")
	r.Write("gone.txt", "bye\n")
	r.Commit("init")
	r.Write("added.txt", "fresh\n")
	r.Git("add", "added.txt")
	r.Remove("gone.txt")
	r.Write("untracked.txt", "hello\n")

	snapshot := collect(t, r)

	if got := fileOf(t, snapshot, "added.txt"); got.Status != git.StatusAdded || got.Before != "" {
		t.Fatalf("added: %+v", got.Status)
	}
	deleted := fileOf(t, snapshot, "gone.txt")
	if deleted.Status != git.StatusDeleted || !deleted.Changes[0].NewRange.Empty() {
		t.Fatalf("deleted: status=%q newRange=%+v", deleted.Status, deleted.Changes[0].NewRange)
	}
	untracked := fileOf(t, snapshot, "untracked.txt")
	if untracked.Status != git.StatusUntracked || !untracked.Changes[0].OldRange.Empty() {
		t.Fatalf("untracked: status=%q oldRange=%+v", untracked.Status, untracked.Changes[0].OldRange)
	}
}

func TestRenamedFileKeepsBothPaths(t *testing.T) {
	r := testrepo.New(t)
	r.Write("old.txt", "stable content for rename detection\nsecond line\nthird line\n")
	r.Commit("init")
	r.Git("mv", "old.txt", "new.txt")

	file := fileOf(t, collect(t, r), "new.txt")
	if file.Status != git.StatusRenamed || file.OldPath != "old.txt" {
		t.Fatalf("status=%q oldPath=%q", file.Status, file.OldPath)
	}
}

func TestBinaryFileHasNoUnits(t *testing.T) {
	r := testrepo.New(t)
	r.Write("a.txt", "1\n")
	r.Commit("init")
	r.WriteBytes("image.bin", []byte{0, 1, 2, 3, 0, 255})
	r.Git("add", "image.bin")

	file := fileOf(t, collect(t, r), "image.bin")
	if !file.Binary || len(file.Changes) != 0 {
		t.Fatalf("binary=%v units=%d", file.Binary, len(file.Changes))
	}
}

func TestPathsWithSpacesAndSpecialCharacters(t *testing.T) {
	r := testrepo.New(t)
	r.Write("dir with space/café – file.txt", "one\n")
	r.Commit("init")
	r.Write("dir with space/café – file.txt", "two\n")

	file := fileOf(t, collect(t, r), "dir with space/café – file.txt")
	if len(file.Changes) != 1 || file.Changes[0].After != "two" {
		t.Fatalf("units=%d", len(file.Changes))
	}
}

func TestFileWithoutOutlineFallsBackToHunks(t *testing.T) {
	r := testrepo.New(t)
	r.Write("notes.txt", "alpha\nbravo\ncharlie\n")
	r.Commit("init")
	r.Write("notes.txt", "alpha\nBRAVO\ncharlie\n")

	change := fileOf(t, collect(t, r), "notes.txt").Changes[0]
	if change.Symbol != nil || change.Kind != git.KindBlock {
		t.Fatalf("symbol=%v kind=%q", change.Symbol, change.Kind)
	}
	if change.Before != "bravo" || change.After != "BRAVO" {
		t.Fatalf("%q -> %q", change.Before, change.After)
	}
}

func TestSymbolUnitsSplitAHunkSpanningTwoFunctions(t *testing.T) {
	r := testrepo.New(t)
	r.Write("mod.go", "package m\n\nfunc First() int {\n\treturn 1\n}\n\nfunc Second() int {\n\treturn 2\n}\n")
	r.Commit("init")
	r.Write("mod.go", "package m\n\nfunc First() int {\n\treturn 10\n}\n\nfunc Second() int {\n\treturn 20\n}\n")

	file := fileOf(t, collect(t, r), "mod.go")
	got := labels(file)
	if len(got) != 2 || got[0] != "First()" || got[1] != "Second()" {
		t.Fatalf("labels = %v", got)
	}
	if file.Changes[0].After != "func First() int {\n\treturn 10\n}" {
		t.Fatalf("unit does not cover the whole function: %q", file.Changes[0].After)
	}
}

func TestUnchangedSymbolProducesNoUnit(t *testing.T) {
	r := testrepo.New(t)
	r.Write("mod.go", "package m\n\nfunc Kept() int {\n\treturn 1\n}\n\nfunc Edited() int {\n\treturn 2\n}\n")
	r.Commit("init")
	r.Write("mod.go", "package m\n\nfunc Kept() int {\n\treturn 1\n}\n\nfunc Edited() int {\n\treturn 3\n}\n")

	if got := labels(fileOf(t, collect(t, r), "mod.go")); len(got) != 1 || got[0] != "Edited()" {
		t.Fatalf("labels = %v", got)
	}
}

func TestDeletedSymbolKeepsItsReadingPosition(t *testing.T) {
	r := testrepo.New(t)
	r.Write("mod.go", "package m\n\nfunc A() int {\n\treturn 1\n}\n\nfunc Gone() int {\n\treturn 2\n}\n\nfunc Z() int {\n\treturn 3\n}\n")
	r.Commit("init")
	r.Write("mod.go", "package m\n\nfunc A() int {\n\treturn 1\n}\n\nfunc Z() int {\n\treturn 30\n}\n")

	got := labels(fileOf(t, collect(t, r), "mod.go"))
	if len(got) != 2 || got[0] != "Gone()" || got[1] != "Z()" {
		t.Fatalf("labels = %v", got)
	}
}

func TestChangeIDIsDeterministic(t *testing.T) {
	r := testrepo.New(t)
	r.Write("mod.go", "package m\n\nfunc A() int {\n\treturn 1\n}\n")
	r.Commit("init")
	r.Write("mod.go", "package m\n\nfunc A() int {\n\treturn 2\n}\n")

	first := collect(t, r).Changes()[0].ID
	second := collect(t, r).Changes()[0].ID
	if first != second {
		t.Fatalf("change id is not stable: %s vs %s", first, second)
	}

	r.Write("mod.go", "package m\n\nfunc A() int {\n\treturn 3\n}\n")
	if third := collect(t, r).Changes()[0].ID; third == first {
		t.Fatal("change id survived an edit to its own code")
	}
}

func TestSnapshotIDIsDeterministic(t *testing.T) {
	r := testrepo.New(t)
	r.Write("a.txt", "1\n")
	r.Commit("init")
	r.Write("a.txt", "2\n")

	first := collect(t, r).ID
	if second := collect(t, r).ID; first != second {
		t.Fatalf("snapshot id is not stable: %s vs %s", first, second)
	}
	r.Write("a.txt", "3\n")
	if third := collect(t, r).ID; third == first {
		t.Fatal("snapshot id survived a change")
	}
	if len(first) != len("sha256:")+64 {
		t.Fatalf("unexpected snapshot id shape: %s", first)
	}
}

func TestResolveInsideRefusesEscapes(t *testing.T) {
	r := testrepo.New(t)
	r.Write("a.txt", "1\n")
	r.Commit("init")
	repo, _ := git.Discover(context.Background(), r.Root)

	for _, path := range []string{"../outside.txt", "../../etc/passwd", "a/../../x"} {
		if _, err := repo.ResolveInside(path); err == nil {
			t.Fatalf("%q was accepted", path)
		}
	}
	if _, err := repo.ResolveInside("a.txt"); err != nil {
		t.Fatalf("a.txt was rejected: %v", err)
	}
}

func TestStagedThenRevertedInTheWorkingTree(t *testing.T) {
	r := testrepo.New(t)
	r.Write("b.txt", "original\n")
	r.Commit("init")
	r.Write("b.txt", "staged\n")
	r.Git("add", "b.txt")
	r.Write("b.txt", "original\n")

	// The working tree matches HEAD, so "git diff HEAD" reports nothing, but
	// the change still exists in the index and must be visible.
	file := fileOf(t, collect(t, r), "b.txt")
	if !file.IndexOnly {
		t.Fatalf("expected an index-only entry, got %+v", file)
	}
	if file.Status != git.StatusModified {
		t.Fatalf("status = %q", file.Status)
	}
	if len(file.Changes) != 1 || file.Changes[0].After != "staged" {
		t.Fatalf("units = %+v", file.Changes)
	}
}

func TestStagedAndThenEditedAgainIsNotReportedTwice(t *testing.T) {
	r := testrepo.New(t)
	r.Write("b.txt", "original\n")
	r.Commit("init")
	r.Write("b.txt", "staged\n")
	r.Git("add", "b.txt")
	r.Write("b.txt", "unstaged\n")

	snapshot := collect(t, r)
	if len(snapshot.Files) != 1 {
		t.Fatalf("expected one entry, got %d", len(snapshot.Files))
	}
	file := fileOf(t, snapshot, "b.txt")
	if file.IndexOnly {
		t.Fatal("a file with a working tree difference must not be index-only")
	}
	if file.After != "unstaged\n" {
		t.Fatalf("after = %q", file.After)
	}
}

func TestStagedAdditionRevertedOnDiskIsStillVisible(t *testing.T) {
	r := testrepo.New(t)
	r.Write("keep.txt", "1\n")
	r.Commit("init")
	r.Write("new.txt", "fresh\n")
	r.Git("add", "new.txt")
	r.Remove("new.txt")

	file := fileOf(t, collect(t, r), "new.txt")
	if !file.IndexOnly || file.Status != git.StatusAdded {
		t.Fatalf("file = %+v", file)
	}
	if file.Changes[0].After != "fresh" {
		t.Fatalf("units = %+v", file.Changes)
	}
}

func TestACleanRepositoryStillHasNoIndexOnlyEntries(t *testing.T) {
	r := testrepo.New(t)
	r.Write("a.txt", "one\n")
	r.Commit("init")

	if snapshot := collect(t, r); len(snapshot.Files) != 0 {
		t.Fatalf("expected no changes, got %+v", snapshot.Files)
	}
}

func TestChangeIDSurvivesCodeMovingWithinItsFile(t *testing.T) {
	r := testrepo.New(t)
	r.Write("mod.go", "package m\n\nfunc A() int {\n\treturn 1\n}\n")
	r.Commit("init")
	r.Write("mod.go", "package m\n\nfunc A() int {\n\treturn 99\n}\n")
	before := collect(t, r).Changes()[0].ID

	// An unrelated insertion above A() shifts its line numbers but leaves its
	// code byte-identical, so its explanation must stay valid.
	r.Write("mod.go", "package m\n\nimport \"os\"\n\nvar _ = os.Args\n\nfunc A() int {\n\treturn 99\n}\n")

	var found bool
	for _, change := range collect(t, r).Changes() {
		if change.Symbol != nil && *change.Symbol == "A" {
			found = true
			if change.ID != before {
				t.Fatalf("id changed although A() did not: %s -> %s", before, change.ID)
			}
		}
	}
	if !found {
		t.Fatal("A() is no longer a unit")
	}
}

func TestIdenticalUnitsInOneFileGetDistinctIDs(t *testing.T) {
	r := testrepo.New(t)
	r.Write("notes.txt", "x\n\n\n\n\n\n\n\n\n\nsame\n\n\n\n\n\n\n\n\n\nsame\n")
	r.Commit("init")
	r.Write("notes.txt", "x\n\n\n\n\n\n\n\n\n\nedited\n\n\n\n\n\n\n\n\n\nedited\n")

	changes := fileOf(t, collect(t, r), "notes.txt").Changes
	if len(changes) != 2 {
		t.Fatalf("expected 2 units, got %d", len(changes))
	}
	if changes[0].ID == changes[1].ID {
		t.Fatalf("two units share an id: %s", changes[0].ID)
	}
}
