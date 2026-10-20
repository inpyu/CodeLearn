// Package web carries the built frontend, embedded in the binary so an end
// user needs neither Node.js nor a separate web server.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embedded embed.FS

// Dist returns the built viewer as a filesystem rooted at index.html, and
// reports whether a build is actually present. A source checkout legitimately
// lacks one, and the server says so rather than serving a blank page.
func Dist() (fs.FS, bool) {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		return nil, false
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil, false
	}
	return sub, true
}
