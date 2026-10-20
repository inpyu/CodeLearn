package git

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// File statuses.
const (
	StatusModified    = "modified"
	StatusAdded       = "added"
	StatusDeleted     = "deleted"
	StatusRenamed     = "renamed"
	StatusUntracked   = "untracked"
	StatusTypeChanged = "type_changed"
)

// contextLines is the diff context git is asked for. It is trimmed back off
// each unit; it exists only so hunks that touch are merged by git.
const contextLines = 3

// Range is an inclusive 1-based line range expressed as a start and a count.
// A Count of 0 marks a side with no lines, which is how a pure insertion or
// deletion is represented.
type Range struct {
	Start int `json:"start"`
	Count int `json:"count"`
}

// Empty reports whether the range covers no lines.
func (r Range) Empty() bool { return r.Count <= 0 }

// End is the last line of the range, or Start-1 when it is empty.
func (r Range) End() int { return r.Start + r.Count - 1 }

func rangeFromHeader(start, count int) Range {
	if count == 0 {
		// git points at the line before an insertion point; normalise that to
		// an explicitly empty range starting at the next line.
		return Range{Start: start + 1, Count: 0}
	}
	return Range{Start: start, Count: count}
}

// Change is one change unit: a symbol that changed, or a raw hunk where the
// file has no symbol outline.
type Change struct {
	ID       string  `json:"id"`
	File     string  `json:"file"`
	Symbol   *string `json:"symbol"`
	Kind     string  `json:"kind"`
	Label    string  `json:"label"`
	OldRange Range   `json:"old_range"`
	NewRange Range   `json:"new_range"`
	Before   string  `json:"before"`
	After    string  `json:"after"`
}

// FileChange is a changed file together with its change units.
type FileChange struct {
	Path      string   `json:"path"`
	OldPath   string   `json:"old_path,omitempty"`
	Status    string   `json:"status"`
	Binary    bool     `json:"binary"`
	Additions int      `json:"additions"`
	Deletions int      `json:"deletions"`
	Before    string   `json:"-"`
	After     string   `json:"-"`
	Changes   []Change `json:"-"`
}

// ChangeIDs lists the ids of this file's change units.
func (f FileChange) ChangeIDs() []string {
	ids := make([]string, 0, len(f.Changes))
	for _, change := range f.Changes {
		ids = append(ids, change.ID)
	}
	return ids
}

type hunk struct {
	old Range
	new Range
}

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// decodeText reports whether data is usable as text, and its content when it
// is. Binary detection is a NUL byte or invalid UTF-8, matching what git's own
// heuristics call binary closely enough for display purposes.
func decodeText(data []byte, present bool) (string, bool) {
	if !present || len(data) == 0 {
		return "", false
	}
	if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		return "", true
	}
	return string(data), false
}

// splitLines splits text into lines, ignoring the empty element a trailing
// newline produces.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func lineCount(text string) int { return len(splitLines(text)) }

func sliceLines(text string, r Range) string {
	if r.Empty() {
		return ""
	}
	lines := splitLines(text)
	start, end := r.Start-1, r.End()
	if start < 0 {
		start = 0
	}
	if end > len(lines) {
		end = len(lines)
	}
	if start >= end {
		return ""
	}
	return strings.Join(lines[start:end], "\n")
}

// parsePatch extracts the hunk ranges of every file in a unified diff.
func parsePatch(patch string) map[string][]hunk {
	hunks := map[string][]hunk{}
	current := ""
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "+++ "):
			target := line[4:]
			if target == "/dev/null" {
				continue // a deletion keeps the path from the "--- a/" line
			}
			current = stripPathPrefix(target)
			if _, ok := hunks[current]; !ok {
				hunks[current] = nil
			}
		case strings.HasPrefix(line, "--- "):
			source := line[4:]
			if source == "/dev/null" {
				continue
			}
			current = stripPathPrefix(source)
			if _, ok := hunks[current]; !ok {
				hunks[current] = nil
			}
		case strings.HasPrefix(line, "@@") && current != "":
			m := hunkHeader.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			oldStart, _ := strconv.Atoi(m[1])
			newStart, _ := strconv.Atoi(m[3])
			oldCount, newCount := 1, 1
			if m[2] != "" {
				oldCount, _ = strconv.Atoi(m[2])
			}
			if m[4] != "" {
				newCount, _ = strconv.Atoi(m[4])
			}
			hunks[current] = append(hunks[current], hunk{
				old: rangeFromHeader(oldStart, oldCount),
				new: rangeFromHeader(newStart, newCount),
			})
		}
	}
	return hunks
}

// stripPathPrefix turns a "--- a/path" or "+++ b/path" operand into a plain
// path. Git appends a tab and an optional timestamp when the path contains a
// space, so everything from the first tab is dropped.
func stripPathPrefix(path string) string {
	if tab := strings.IndexByte(path, '\t'); tab >= 0 {
		path = path[:tab]
	}
	if strings.HasPrefix(path, "a/") || strings.HasPrefix(path, "b/") {
		return path[2:]
	}
	return path
}

// trimContext drops the leading and trailing context git adds around a hunk,
// so a unit is the change itself rather than a window around it.
func trimContext(h hunk, beforeLines, afterLines []string) hunk {
	oldLines := linesIn(beforeLines, h.old)
	newLines := linesIn(afterLines, h.new)
	lead := 0
	for lead < len(oldLines) && lead < len(newLines) && oldLines[lead] == newLines[lead] {
		lead++
	}
	tail := 0
	for tail < len(oldLines)-lead && tail < len(newLines)-lead &&
		oldLines[len(oldLines)-1-tail] == newLines[len(newLines)-1-tail] {
		tail++
	}
	trimmed := hunk{
		old: Range{Start: h.old.Start + lead, Count: h.old.Count - lead - tail},
		new: Range{Start: h.new.Start + lead, Count: h.new.Count - lead - tail},
	}
	if trimmed.old.Empty() && trimmed.new.Empty() {
		return h
	}
	return trimmed
}

func linesIn(lines []string, r Range) []string {
	if r.Empty() {
		return nil
	}
	start, end := r.Start-1, r.End()
	if start < 0 {
		start = 0
	}
	if end > len(lines) {
		end = len(lines)
	}
	if start >= end {
		return nil
	}
	return lines[start:end]
}
