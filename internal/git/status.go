package git

import (
	"context"
	"strings"
)

// statusEntry is one line of `git status --porcelain=v1`.
type statusEntry struct {
	index    byte // staged state
	worktree byte // unstaged state
	path     string
}

// porcelainStatus reads the index and working tree state of every changed
// path in one call.
//
// `git diff HEAD` answers "how does the final file differ from HEAD", which is
// the right question for the viewer but blind to a change that exists only in
// the index - staged, then reverted on disk. Porcelain status reports the two
// states separately, so that case can be found and shown.
func porcelainStatus(ctx context.Context, repo *Repository) ([]statusEntry, error) {
	raw, err := repo.Run(ctx, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	fields := strings.Split(raw, "\x00")
	var entries []statusEntry
	for index := 0; index < len(fields); index++ {
		field := fields[index]
		if len(field) < 4 {
			continue
		}
		entry := statusEntry{index: field[0], worktree: field[1], path: field[3:]}
		// A rename entry is followed by its original path in the next field.
		if entry.index == 'R' || entry.index == 'C' {
			index++
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// indexOnlyChanges finds paths that differ between HEAD and the index while
// the working tree matches HEAD, and builds a file entry for each by
// comparing HEAD with the staged content.
func indexOnlyChanges(
	ctx context.Context,
	repo *Repository,
	baseRev, baseID string,
	seen map[string]bool,
) ([]FileChange, error) {
	entries, err := porcelainStatus(ctx, repo)
	if err != nil {
		return nil, err
	}

	var files []FileChange
	for _, entry := range entries {
		status, ok := statusCodes[entry.index]
		if !ok || seen[entry.path] {
			continue // unchanged in the index, or already covered by the diff
		}

		var before string
		if status != StatusAdded {
			data, present := repo.ReadAtRev(ctx, baseRev, entry.path)
			before, _ = decodeText(data, present)
		}
		// ":path" addresses stage 0 of the index.
		stagedData, present := repo.ReadAtRev(ctx, "", ":"+entry.path)
		after, binary := decodeText(stagedData, present)
		if before == after {
			continue
		}

		file := FileChange{
			Path:      entry.path,
			Status:    status,
			IndexOnly: true,
			Binary:    binary,
			Before:    before,
			After:     after,
		}
		if !file.Binary {
			whole := []hunk{{
				old: Range{Start: 1, Count: lineCount(before)},
				new: Range{Start: 1, Count: lineCount(after)},
			}}
			file.Changes = buildChanges(&file, whole, baseID)
			for _, change := range file.Changes {
				file.Additions += change.NewRange.Count
				file.Deletions += change.OldRange.Count
			}
		}
		files = append(files, file)
	}
	return files, nil
}
