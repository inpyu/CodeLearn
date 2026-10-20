package analysis

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/inpyu/codelearn/internal/git"
)

// ImportResult reports the outcome of importing one analysis file.
type ImportResult struct {
	Steps      int
	StoredPath string
	Problems   []string
}

// ImportFile reads, validates and stores an analysis.
//
// Import is atomic per file: if any step fails validation nothing is stored,
// so a half-applied analysis can never exist on disk.
func ImportFile(root string, snapshot *git.Snapshot, path string) (*ImportResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("file not found: %s", path)
	}
	if info.Size() > MaxAnalysisBytes {
		return nil, fmt.Errorf("analysis is larger than %d bytes", MaxAnalysisBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc Analysis
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	problems := Validate(&doc)
	problems = append(problems, checkAgainstSnapshot(&doc, snapshot)...)
	if len(problems) > 0 {
		return &ImportResult{Problems: problems}, ErrInvalid
	}

	stored, err := Save(root, snapshot.ID, doc)
	if err != nil {
		return nil, err
	}
	return &ImportResult{Steps: len(doc.Steps), StoredPath: stored}, nil
}

// checkAgainstSnapshot rejects anything that does not describe the code that
// is on disk right now.
func checkAgainstSnapshot(doc *Analysis, snapshot *git.Snapshot) []string {
	var problems []string
	if doc.Repository != nil && doc.Repository.Name != "" && doc.Repository.Name != snapshot.Repository {
		problems = append(problems, fmt.Sprintf(
			"repository: analysis was generated for %q, current repository is %q",
			doc.Repository.Name, snapshot.Repository))
	}
	if doc.SnapshotID != snapshot.ID {
		problems = append(problems, "snapshot_id: does not match the current changes; "+
			"re-run 'codelearn export' and regenerate the analysis")
	}
	seen := map[string]bool{}
	for index, step := range doc.Steps {
		if seen[step.ChangeID] {
			problems = append(problems, fmt.Sprintf("steps/%d/change_id: duplicate", index))
			continue
		}
		seen[step.ChangeID] = true
		if _, ok := snapshot.Change(step.ChangeID); !ok {
			problems = append(problems, fmt.Sprintf(
				"steps/%d/change_id: not a change unit of the current snapshot", index))
		}
	}
	return problems
}

// ViewStep is a stored step resolved against the current snapshot, carrying
// the code location the viewer needs.
type ViewStep struct {
	ChangeID string    `json:"change_id"`
	Title    string    `json:"title"`
	What     string    `json:"what"`
	How      string    `json:"how"`
	Why      string    `json:"why,omitempty"`
	File     string    `json:"file"`
	Symbol   *string   `json:"symbol"`
	Kind     string    `json:"kind"`
	Label    string    `json:"label"`
	OldRange git.Range `json:"old_range"`
	NewRange git.Range `json:"new_range"`
}

// View is what GET /api/analysis returns.
type View struct {
	SnapshotID      string     `json:"snapshot_id"`
	Available       bool       `json:"available"`
	Stale           bool       `json:"stale"`
	SnapshotChanged bool       `json:"snapshot_changed"`
	Dropped         int        `json:"dropped"`
	CreatedAt       string     `json:"created_at,omitempty"`
	Generator       string     `json:"generator,omitempty"`
	Steps           []ViewStep `json:"steps"`
}

// StoredView resolves the stored analysis against the current snapshot.
//
// A step is kept only when its change id still exists, and a change id is a
// hash of the unit's own code, so a surviving step provably describes code
// that has not changed. An edit elsewhere in the repository moves the snapshot
// without invalidating anything, which is why Stale counts dropped steps
// rather than simply comparing snapshot ids.
func StoredView(root string, snapshot *git.Snapshot) View {
	view := View{SnapshotID: snapshot.ID, Steps: []ViewStep{}}

	record, ok := Load(root, snapshot.ID)
	if !ok {
		record, ok = LoadLatest(root)
	}
	if !ok {
		return view
	}

	view.SnapshotChanged = record.SnapshotID != snapshot.ID
	view.CreatedAt = record.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
	view.Generator = record.Analysis.Generator

	for _, step := range record.Analysis.Steps {
		change, found := snapshot.Change(step.ChangeID)
		if !found {
			view.Dropped++
			continue
		}
		view.Steps = append(view.Steps, ViewStep{
			ChangeID: step.ChangeID,
			Title:    step.Title,
			What:     step.What,
			How:      step.How,
			Why:      step.Why,
			File:     change.File,
			Symbol:   change.Symbol,
			Kind:     change.Kind,
			Label:    change.Label,
			OldRange: change.OldRange,
			NewRange: change.NewRange,
		})
	}
	view.Available = len(view.Steps) > 0
	view.Stale = view.Dropped > 0
	return view
}
