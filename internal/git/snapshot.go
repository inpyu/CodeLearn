package git

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Snapshot is the complete comparison between the base revision and the
// working tree at one point in time. Export, import and the viewer all refer
// to the same snapshot id.
type Snapshot struct {
	Repository string
	Branch     string
	Base       string
	BaseCommit string
	ID         string
	Files      []FileChange
}

// Changes flattens the units of every file.
func (s *Snapshot) Changes() []Change {
	var changes []Change
	for _, file := range s.Files {
		changes = append(changes, file.Changes...)
	}
	return changes
}

// Change finds a unit by id.
func (s *Snapshot) Change(id string) (Change, bool) {
	for _, file := range s.Files {
		for _, change := range file.Changes {
			if change.ID == id {
				return change, true
			}
		}
	}
	return Change{}, false
}

// File finds a changed file by path. Callers use it to validate a path that
// came from a client instead of touching the filesystem with it.
func (s *Snapshot) File(path string) (FileChange, bool) {
	for _, file := range s.Files {
		if file.Path == path {
			return file, true
		}
	}
	return FileChange{}, false
}

var statusCodes = map[byte]string{
	'M': StatusModified,
	'A': StatusAdded,
	'D': StatusDeleted,
	'R': StatusRenamed,
	'C': StatusAdded,
	'T': StatusTypeChanged,
}

// Collect compares the base revision with the working tree.
func Collect(ctx context.Context, repo *Repository) (*Snapshot, error) {
	baseCommit := repo.HeadCommit(ctx)
	baseRev := "HEAD"
	baseID := baseCommit
	if baseCommit == "" {
		baseRev = EmptyTree
		baseID = EmptyTree
	}

	tracked, err := trackedChanges(ctx, repo, baseRev, baseID)
	if err != nil {
		return nil, err
	}
	untracked, err := untrackedChanges(ctx, repo, baseID)
	if err != nil {
		return nil, err
	}

	files := append(tracked, untracked...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	return &Snapshot{
		Repository: repo.Name(),
		Branch:     repo.Branch(ctx),
		Base:       "HEAD",
		BaseCommit: baseCommit,
		ID:         snapshotID(baseID, files),
		Files:      files,
	}, nil
}

func trackedChanges(ctx context.Context, repo *Repository, baseRev, baseID string) ([]FileChange, error) {
	// Diffing against HEAD rather than the index is what merges staged and
	// unstaged edits into one entry per file, and what makes a file that was
	// staged and then deleted report its final state.
	nameStatus, err := repo.Run(ctx, "diff", baseRev, "-M", "--name-status", "-z",
		"--no-ext-diff", "--no-textconv", "--no-color")
	if err != nil {
		return nil, err
	}
	numstat, err := repo.Run(ctx, "diff", baseRev, "-M", "--numstat", "-z",
		"--no-ext-diff", "--no-textconv", "--no-color")
	if err != nil {
		return nil, err
	}
	patch, err := repo.Run(ctx, "diff", baseRev, "-M",
		fmt.Sprintf("--unified=%d", contextLines), "--no-ext-diff", "--no-textconv", "--no-color")
	if err != nil {
		return nil, err
	}

	hunksByFile := parsePatch(patch)
	stats := parseNumstat(numstat)

	var files []FileChange
	for _, entry := range parseNameStatus(nameStatus) {
		source := entry.path
		if entry.oldPath != "" {
			source = entry.oldPath
		}

		var before, after string
		var beforeBinary, afterBinary bool
		if entry.status != StatusAdded {
			data, ok := repo.ReadAtRev(ctx, baseRev, source)
			before, beforeBinary = decodeText(data, ok)
		}
		if entry.status != StatusDeleted {
			data, ok := repo.ReadWorktree(entry.path)
			after, afterBinary = decodeText(data, ok)
		}

		stat := stats[entry.path]
		file := FileChange{
			Path:      entry.path,
			OldPath:   entry.oldPath,
			Status:    entry.status,
			Binary:    stat.binary || beforeBinary || afterBinary,
			Additions: stat.added,
			Deletions: stat.deleted,
			Before:    before,
			After:     after,
		}
		if !file.Binary {
			file.Changes = buildChanges(&file, hunksByFile[entry.path], baseID)
		}
		files = append(files, file)
	}
	return files, nil
}

func untrackedChanges(ctx context.Context, repo *Repository, baseID string) ([]FileChange, error) {
	raw, err := repo.Run(ctx, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	var files []FileChange
	for _, path := range splitNUL(raw) {
		data, ok := repo.ReadWorktree(path)
		after, binary := decodeText(data, ok)
		file := FileChange{
			Path:   path,
			Status: StatusUntracked,
			Binary: binary,
			After:  after,
		}
		if !binary {
			lines := lineCount(after)
			file.Additions = lines
			// The whole file is one insertion; buildChanges then cuts it into
			// symbols exactly as it cuts a tracked file.
			whole := []hunk{{old: Range{Start: 1}, new: Range{Start: 1, Count: lines}}}
			file.Changes = buildChanges(&file, whole, baseID)
		}
		files = append(files, file)
	}
	return files, nil
}

type nameStatusEntry struct {
	status  string
	path    string
	oldPath string
}

func parseNameStatus(raw string) []nameStatusEntry {
	fields := splitNUL(raw)
	var entries []nameStatusEntry
	for index := 0; index < len(fields); {
		code := fields[index]
		if code == "" || index+1 >= len(fields) {
			break
		}
		status, ok := statusCodes[code[0]]
		if !ok {
			status = StatusModified
		}
		if (code[0] == 'R' || code[0] == 'C') && index+2 < len(fields) {
			entries = append(entries, nameStatusEntry{status, fields[index+2], fields[index+1]})
			index += 3
			continue
		}
		entries = append(entries, nameStatusEntry{status: status, path: fields[index+1]})
		index += 2
	}
	return entries
}

type numstatEntry struct {
	added   int
	deleted int
	binary  bool
}

func parseNumstat(raw string) map[string]numstatEntry {
	stats := map[string]numstatEntry{}
	fields := splitNUL(raw)
	for index := 0; index < len(fields); {
		parts := strings.SplitN(fields[index], "\t", 3)
		if len(parts) < 3 {
			index++
			continue
		}
		entry := numstatEntry{}
		if parts[0] == "-" || parts[1] == "-" {
			entry.binary = true
		} else {
			entry.added, _ = strconv.Atoi(parts[0])
			entry.deleted, _ = strconv.Atoi(parts[1])
		}
		path := parts[2]
		if path == "" && index+2 < len(fields) {
			// A rename is emitted as "added\tdeleted\t\0old\0new".
			path = fields[index+2]
			index += 3
		} else {
			index++
		}
		stats[path] = entry
	}
	return stats
}

func splitNUL(raw string) []string {
	var out []string
	for _, field := range strings.Split(raw, "\x00") {
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

// snapshotID hashes a canonical serialisation, so neither file ordering nor
// map iteration order can change the result for the same working tree.
func snapshotID(baseID string, files []FileChange) string {
	ordered := make([]FileChange, len(files))
	copy(ordered, files)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Path < ordered[j].Path })

	digest := sha256.New()
	digest.Write([]byte(baseID))
	for _, file := range ordered {
		digest.Write([]byte{0})
		fmt.Fprintf(digest, "%s\x00%s\x00%s\x00", file.Path, file.Status, file.OldPath)
		beforeSum := sha256.Sum256([]byte(file.Before))
		afterSum := sha256.Sum256([]byte(file.After))
		digest.Write(beforeSum[:])
		digest.Write(afterSum[:])
		for _, change := range file.Changes {
			digest.Write([]byte(change.ID))
		}
	}
	return "sha256:" + hex.EncodeToString(digest.Sum(nil))
}
