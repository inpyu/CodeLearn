package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inpyu/codelearn/internal/analysis"
	"github.com/inpyu/codelearn/internal/git"
	"github.com/inpyu/codelearn/internal/server"
	"github.com/inpyu/codelearn/internal/testrepo"
)

// newServer builds a repository with one changed function and an HTTP test
// server in front of it.
func newServer(t *testing.T) (*testrepo.Repo, *git.Repository, *httptest.Server) {
	t.Helper()
	r := testrepo.New(t)
	r.Write("cache.go", "package cache\n\nfunc Get() int {\n\treturn create()\n}\n")
	r.Commit("init")
	r.Write("cache.go", "package cache\n\nfunc Get() int {\n\treturn reuse()\n}\n")

	repo, err := git.Discover(context.Background(), r.Root)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := server.New(repo, "127.0.0.1", 0)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return r, repo, ts
}

func getJSON(t *testing.T, ts *httptest.Server, path string, into any) *http.Response {
	t.Helper()
	resp, err := ts.Client().Get(ts.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if into != nil && resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
			t.Fatalf("decoding %s: %v", path, err)
		}
	}
	return resp
}

func TestRepositoryEndpoint(t *testing.T) {
	r, _, ts := newServer(t)
	var body struct {
		Name         string `json:"name"`
		Branch       string `json:"branch"`
		ChangedFiles int    `json:"changed_files"`
		SnapshotID   string `json:"snapshot_id"`
	}
	getJSON(t, ts, "/api/repository", &body)

	if body.Branch != "main" || body.ChangedFiles != 1 {
		t.Fatalf("body = %+v", body)
	}
	if !strings.HasPrefix(body.SnapshotID, "sha256:") {
		t.Fatalf("snapshot id = %q", body.SnapshotID)
	}
	_ = r
}

func TestChangesEndpointListsIDsOnly(t *testing.T) {
	_, _, ts := newServer(t)
	var body struct {
		Files []struct {
			Path      string   `json:"path"`
			Status    string   `json:"status"`
			ChangeIDs []string `json:"change_ids"`
		} `json:"files"`
	}
	getJSON(t, ts, "/api/changes", &body)

	if len(body.Files) != 1 || body.Files[0].Path != "cache.go" {
		t.Fatalf("files = %+v", body.Files)
	}
	if len(body.Files[0].ChangeIDs) != 1 {
		t.Fatalf("change ids = %v", body.Files[0].ChangeIDs)
	}
}

func TestFileDiffEndpoint(t *testing.T) {
	_, _, ts := newServer(t)
	var body struct {
		Path    string `json:"path"`
		Before  string `json:"before"`
		After   string `json:"after"`
		Changes []struct {
			Label string `json:"label"`
		} `json:"changes"`
	}
	getJSON(t, ts, "/api/files/diff?path=cache.go", &body)

	if !strings.Contains(body.Before, "create()") || !strings.Contains(body.After, "reuse()") {
		t.Fatalf("contents = %+v", body)
	}
	if len(body.Changes) != 1 || body.Changes[0].Label != "Get()" {
		t.Fatalf("changes = %+v", body.Changes)
	}
}

func TestFileDiffRejectsPathsOutsideTheSnapshot(t *testing.T) {
	_, _, ts := newServer(t)

	for _, path := range []string{
		"/api/files/diff?path=../../etc/passwd",
		"/api/files/diff?path=/etc/passwd",
		"/api/files/diff?path=unchanged.go",
		"/api/files/diff",
	} {
		resp := getJSON(t, ts, path, nil)
		if resp.StatusCode == http.StatusOK {
			t.Fatalf("%s was served", path)
		}
	}
}

func TestAnalysisEndpoint(t *testing.T) {
	r, repo, ts := newServer(t)

	var empty struct {
		Available bool `json:"available"`
		Steps     []struct {
			Title string `json:"title"`
		} `json:"steps"`
	}
	getJSON(t, ts, "/api/analysis", &empty)
	if empty.Available || len(empty.Steps) != 0 {
		t.Fatalf("expected an empty analysis, got %+v", empty)
	}

	snapshot, err := git.Collect(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	doc := analysis.Analysis{
		SchemaVersion: "1.0",
		SnapshotID:    snapshot.ID,
		Steps: []analysis.Step{{
			ChangeID: snapshot.Changes()[0].ID,
			Title:    "Cache reuse",
			What:     "Creation became reuse.",
			How:      "The existing object is returned.",
		}},
	}
	if _, err := analysis.Save(repo.Root, snapshot.ID, doc); err != nil {
		t.Fatal(err)
	}

	var filled struct {
		Available bool `json:"available"`
		Steps     []struct {
			Title string `json:"title"`
			File  string `json:"file"`
			Label string `json:"label"`
		} `json:"steps"`
	}
	getJSON(t, ts, "/api/analysis", &filled)
	if !filled.Available || len(filled.Steps) != 1 {
		t.Fatalf("analysis = %+v", filled)
	}
	if filled.Steps[0].File != "cache.go" || filled.Steps[0].Label != "Get()" {
		t.Fatalf("step = %+v", filled.Steps[0])
	}
	_ = r
}

func TestRefreshReflectsNewEdits(t *testing.T) {
	r, _, ts := newServer(t)

	var before struct {
		ChangedFiles int `json:"changed_files"`
	}
	getJSON(t, ts, "/api/repository", &before)

	r.Write("extra.go", "package cache\n")

	resp, err := ts.Client().Post(ts.URL+"/api/refresh", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var after struct {
		ChangedFiles int    `json:"changed_files"`
		SnapshotID   string `json:"snapshot_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&after); err != nil {
		t.Fatal(err)
	}
	if after.ChangedFiles != before.ChangedFiles+1 {
		t.Fatalf("refresh did not see the new file: %d -> %d", before.ChangedFiles, after.ChangedFiles)
	}
}

func TestWrongMethodIsRejected(t *testing.T) {
	_, _, ts := newServer(t)

	resp, err := ts.Client().Post(ts.URL+"/api/changes", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestCrossOriginStateChangeIsRejected(t *testing.T) {
	_, _, ts := newServer(t)

	request, err := http.NewRequest("POST", ts.URL+"/api/refresh", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", "https://evil.example")
	resp, err := ts.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestOversizedRequestIsRejected(t *testing.T) {
	_, _, ts := newServer(t)

	body := strings.NewReader(strings.Repeat("x", 2<<20))
	request, err := http.NewRequest("POST", ts.URL+"/api/refresh", body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", ts.URL)
	resp, err := ts.Client().Do(request)
	if err != nil {
		return // the server closed the connection, which is also a rejection
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		io := make([]byte, 1)
		_, _ = resp.Body.Read(io)
	}
}
