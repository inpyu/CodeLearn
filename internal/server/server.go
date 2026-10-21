// Package server runs the local web viewer.
package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/inpyu/codelearn/internal/git"
)

// DefaultHost is the loopback interface. The viewer renders uncommitted code,
// which is the least reviewed code there is, so it is never reachable from the
// network.
const DefaultHost = "127.0.0.1"

// DefaultPort is tried first, so the URL stays predictable between runs.
const DefaultPort = 8765

// maxRequestBytes bounds any request body the API will read.
const maxRequestBytes = 1 << 20

// Server owns the listener and the HTTP handler for one repository.
//
// It holds the current snapshot so that drawing one screen - the file list,
// a file's diff and the analysis - costs one collection rather than one per
// request. The snapshot is rebuilt only on POST /api/refresh, which is also
// what the Refresh button calls: CodeLearn deliberately does not watch the
// filesystem, so the moment the user sees new data is the moment they ask.
type Server struct {
	repo     *git.Repository
	http     *http.Server
	ln       net.Listener
	URL      string
	mu       sync.RWMutex
	snapshot *git.Snapshot
}

// New binds a listener and builds the handler. The port is chosen here rather
// than inside Run so the caller can print a URL that is already correct.
//
// An initial snapshot may be supplied by the caller, which lets the CLI reuse
// the collection it already made for its startup banner.
func New(repo *git.Repository, host string, port int, initial *git.Snapshot) (*Server, error) {
	ln, err := listen(host, port)
	if err != nil {
		return nil, err
	}
	s := &Server{
		repo:     repo,
		ln:       ln,
		URL:      fmt.Sprintf("http://%s", ln.Addr().String()),
		snapshot: initial,
	}
	s.http = &http.Server{
		Handler:           s.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s, nil
}

// listen prefers the requested port and falls back to one the OS picks, so a
// second repository being viewed is a different port rather than a crash.
func listen(host string, port int) (net.Listener, error) {
	if port != 0 {
		if ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port)); err == nil {
			return ln, nil
		}
	}
	return net.Listen("tcp", fmt.Sprintf("%s:0", host))
}

// Handler exposes the HTTP handler, so tests can drive the API through
// httptest without binding the real listener.
func (s *Server) Handler() http.Handler { return s.http.Handler }

// Serve runs until the context is cancelled, then shuts down gracefully.
func (s *Server) Serve(ctx context.Context) error {
	errs := make(chan error, 1)
	go func() {
		err := s.http.Serve(s.ln)
		if err == http.ErrServerClosed {
			err = nil
		}
		errs <- err
	}()
	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.http.Shutdown(shutdown)
	}
}

// OpenBrowser launches the platform's browser. Failure is reported to the
// caller rather than treated as fatal: the URL has already been printed.
func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
