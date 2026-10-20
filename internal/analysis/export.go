package analysis

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/inpyu/codelearn/internal/git"
)

// Export is the document handed to the CodeLearn skill.
type Export struct {
	SchemaVersion string       `json:"schema_version"`
	SnapshotID    string       `json:"snapshot_id"`
	Repository    Repository   `json:"repository"`
	Changes       []git.Change `json:"changes"`
}

// BuildExport flattens a snapshot into the export document. Change units are
// a flat list so the agent never has to walk a file tree to find them.
func BuildExport(snapshot *git.Snapshot) Export {
	changes := snapshot.Changes()
	if changes == nil {
		changes = []git.Change{}
	}
	return Export{
		SchemaVersion: SchemaVersion,
		SnapshotID:    snapshot.ID,
		Repository: Repository{
			Name:       snapshot.Repository,
			Base:       snapshot.Base,
			BaseCommit: snapshot.BaseCommit,
		},
		Changes: changes,
	}
}

// WriteExport writes the export document to path. It never touches the
// repository's own files.
func WriteExport(snapshot *git.Snapshot, path string) error {
	data, err := json.MarshalIndent(BuildExport(snapshot), "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
