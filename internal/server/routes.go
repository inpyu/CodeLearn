package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/url"
	"strings"

	"github.com/inpyu/codelearn/internal/analysis"
	"github.com/inpyu/codelearn/internal/git"
	"github.com/inpyu/codelearn/web"
)

// maxDiffBytes is the largest file the viewer will render. Beyond it Monaco
// becomes unusable, so a notice is shown instead of a diff.
const maxDiffBytes = 2 << 20

type repositoryResponse struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	Branch       string `json:"branch"`
	Base         string `json:"base"`
	BaseCommit   string `json:"base_commit"`
	SnapshotID   string `json:"snapshot_id"`
	ChangedFiles int    `json:"changed_files"`
	Version      string `json:"version"`
}

type changedFile struct {
	Path      string   `json:"path"`
	OldPath   string   `json:"old_path,omitempty"`
	Status    string   `json:"status"`
	Binary    bool     `json:"binary"`
	Additions int      `json:"additions"`
	Deletions int      `json:"deletions"`
	ChangeIDs []string `json:"change_ids"`
}

type changesResponse struct {
	SnapshotID string        `json:"snapshot_id"`
	Files      []changedFile `json:"files"`
}

type diffChange struct {
	ID       string    `json:"id"`
	Symbol   *string   `json:"symbol"`
	Kind     string    `json:"kind"`
	Label    string    `json:"label"`
	OldRange git.Range `json:"old_range"`
	NewRange git.Range `json:"new_range"`
}

type diffResponse struct {
	SnapshotID string       `json:"snapshot_id"`
	Path       string       `json:"path"`
	Binary     bool         `json:"binary"`
	TooLarge   bool         `json:"too_large"`
	Before     string       `json:"before"`
	After      string       `json:"after"`
	Changes    []diffChange `json:"changes"`
}

type refreshResponse struct {
	SnapshotID   string `json:"snapshot_id"`
	ChangedFiles int    `json:"changed_files"`
}

// Version is stamped by the CLI so the API can report it.
var Version = "dev"

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/repository", s.methods("GET", s.handleRepository))
	mux.HandleFunc("/api/changes", s.methods("GET", s.handleChanges))
	mux.HandleFunc("/api/files/diff", s.methods("GET", s.handleFileDiff))
	mux.HandleFunc("/api/analysis", s.methods("GET", s.handleAnalysis))
	mux.HandleFunc("/api/refresh", s.methods("POST", s.handleRefresh))

	if dist, ok := web.Dist(); ok {
		mux.Handle("/", staticHandler(dist))
	} else {
		mux.HandleFunc("/", missingFrontend)
	}
	return limitBody(mux)
}

// methods rejects the wrong verb and, for state-changing requests, any request
// whose Origin is not this server. A page on the open internet can otherwise
// POST to a known loopback port from the user's own browser.
func (s *Server) methods(method string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			w.Header().Set("Allow", method)
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if method != "GET" && !sameOrigin(r) {
			writeError(w, http.StatusForbidden, "cross-origin request rejected")
			return
		}
		next(w, r)
	}
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // not a browser-initiated cross-origin request
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return parsed.Host == r.Host
}

func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
		next.ServeHTTP(w, r)
	})
}

// snapshot re-reads git on every request. That is why Refresh needs no cache
// invalidation and no file watcher: the server holds no state to go stale.
func (s *Server) snapshot(w http.ResponseWriter, r *http.Request) (*git.Snapshot, bool) {
	snapshot, err := git.Collect(r.Context(), s.repo)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return nil, false
	}
	return snapshot, true
}

func (s *Server) handleRepository(w http.ResponseWriter, r *http.Request) {
	snapshot, ok := s.snapshot(w, r)
	if !ok {
		return
	}
	writeJSON(w, repositoryResponse{
		Name:         snapshot.Repository,
		Path:         s.repo.Root,
		Branch:       snapshot.Branch,
		Base:         snapshot.Base,
		BaseCommit:   snapshot.BaseCommit,
		SnapshotID:   snapshot.ID,
		ChangedFiles: len(snapshot.Files),
		Version:      Version,
	})
}

func (s *Server) handleChanges(w http.ResponseWriter, r *http.Request) {
	snapshot, ok := s.snapshot(w, r)
	if !ok {
		return
	}
	files := make([]changedFile, 0, len(snapshot.Files))
	for _, file := range snapshot.Files {
		files = append(files, changedFile{
			Path:      file.Path,
			OldPath:   file.OldPath,
			Status:    file.Status,
			Binary:    file.Binary,
			Additions: file.Additions,
			Deletions: file.Deletions,
			ChangeIDs: file.ChangeIDs(),
		})
	}
	writeJSON(w, changesResponse{SnapshotID: snapshot.ID, Files: files})
}

func (s *Server) handleFileDiff(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	snapshot, ok := s.snapshot(w, r)
	if !ok {
		return
	}
	// The path is resolved against the snapshot's changed file list, never
	// used to read the filesystem directly.
	file, found := snapshot.File(path)
	if !found {
		writeError(w, http.StatusNotFound, "not a changed file in the current snapshot")
		return
	}

	response := diffResponse{
		SnapshotID: snapshot.ID,
		Path:       file.Path,
		Binary:     file.Binary,
		Changes:    make([]diffChange, 0, len(file.Changes)),
	}
	for _, change := range file.Changes {
		response.Changes = append(response.Changes, diffChange{
			ID:       change.ID,
			Symbol:   change.Symbol,
			Kind:     change.Kind,
			Label:    change.Label,
			OldRange: change.OldRange,
			NewRange: change.NewRange,
		})
	}
	if len(file.Before) > maxDiffBytes || len(file.After) > maxDiffBytes {
		response.TooLarge = true
	} else if !file.Binary {
		response.Before = file.Before
		response.After = file.After
	}
	writeJSON(w, response)
}

func (s *Server) handleAnalysis(w http.ResponseWriter, r *http.Request) {
	snapshot, ok := s.snapshot(w, r)
	if !ok {
		return
	}
	writeJSON(w, analysis.StoredView(s.repo.Root, snapshot))
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	snapshot, ok := s.snapshot(w, r)
	if !ok {
		return
	}
	writeJSON(w, refreshResponse{SnapshotID: snapshot.ID, ChangedFiles: len(snapshot.Files)})
}

// staticHandler serves the embedded viewer with caching disabled, so a
// rebuilt binary is never shadowed by a stale asset in the browser.
func staticHandler(dist fs.FS) http.Handler {
	files := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		files.ServeHTTP(w, r)
	})
}

func missingFrontend(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusServiceUnavailable,
		"the web viewer is not built; run 'make build' or 'npm run build' in frontend/")
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeError(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"detail": detail})
}
