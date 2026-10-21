package analysis

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"sort"
	"time"
)

// Record is what is persisted: the analysis plus enough provenance to tell
// later whether it still describes the current code.
type Record struct {
	RepositoryID   string    `json:"repository_id"`
	RepositoryPath string    `json:"repository_path"`
	SnapshotID     string    `json:"snapshot_id"`
	CreatedAt      time.Time `json:"created_at"`
	Analysis       Analysis  `json:"analysis"`
}

// CacheRoot is the directory analyses are stored in. CODELEARN_CACHE_DIR
// overrides it, which is how the tests keep out of a developer's real cache.
func CacheRoot() (string, error) {
	if override := os.Getenv("CODELEARN_CACHE_DIR"); override != "" {
		return override, nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "codelearn"), nil
}

// RepositoryID identifies a repository by the location it was opened from -
// an absolute path, or "ssh://user@host/path" for a remote one.
//
// The directory name alone collides between two clones of one project, and
// between the same path on two hosts; a bare hash is unreadable when
// inspecting the cache. Both are used, so the same project on a laptop and on
// a server keep separate analyses.
func RepositoryID(location string) string {
	sum := sha256.Sum256([]byte(path.Clean(filepath.ToSlash(location))))
	return path.Base(filepath.ToSlash(location)) + "-" + hex.EncodeToString(sum[:])[:12]
}

func repositoryDir(root string) (string, error) {
	cache, err := CacheRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, RepositoryID(root)), nil
}

// AnalysisPath is where the analysis for one snapshot of one repository lives.
func AnalysisPath(root, snapshotID string) (string, error) {
	dir, err := repositoryDir(root)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, safeSegment(snapshotID), "analysis.json"), nil
}

// safeSegment turns a snapshot id into a single path segment.
func safeSegment(id string) string {
	out := []rune(id)
	for index, char := range out {
		switch char {
		case '/', '\\', ':', '.':
			out[index] = '-'
		}
	}
	return string(out)
}

// Save writes a record atomically: a temporary file in the destination
// directory, then a rename, so a crash or a concurrent reader never sees a
// half-written analysis.
func Save(root, snapshotID string, doc Analysis) (string, error) {
	path, err := AnalysisPath(root, snapshotID)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	record := Record{
		RepositoryID:   RepositoryID(root),
		RepositoryPath: root,
		SnapshotID:     snapshotID,
		CreatedAt:      time.Now().UTC(),
		Analysis:       doc,
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return "", err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), "analysis-*.tmp")
	if err != nil {
		return "", err
	}
	defer os.Remove(temporary.Name())
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		temporary.Close()
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(temporary.Name(), path); err != nil {
		return "", err
	}
	return path, nil
}

// Load reads the record stored for an exact snapshot.
func Load(root, snapshotID string) (*Record, bool) {
	path, err := AnalysisPath(root, snapshotID)
	if err != nil {
		return nil, false
	}
	return readRecord(path)
}

// LoadLatest reads the most recently written record for a repository,
// whatever snapshot it was generated for. The caller decides which of its
// steps still apply.
func LoadLatest(root string) (*Record, bool) {
	dir, err := repositoryDir(root)
	if err != nil {
		return nil, false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, false
	}
	type candidate struct {
		path string
		at   time.Time
	}
	var found []candidate
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		found = append(found, candidate{filepath.Join(dir, entry.Name(), "analysis.json"), info.ModTime()})
	}
	sort.Slice(found, func(i, j int) bool { return found[i].at.After(found[j].at) })
	for _, item := range found {
		if record, ok := readRecord(item.path); ok {
			return record, true
		}
	}
	return nil, false
}

// readRecord treats damage as absence. The cache is a convenience, never the
// source of truth, so a corrupt file degrades to "no explanations yet" rather
// than taking down a tool whose main job is showing a diff.
func readRecord(path string) (*Record, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, false
	}
	return &record, true
}
