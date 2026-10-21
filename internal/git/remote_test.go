package git_test

import (
	"strings"
	"testing"

	"github.com/inpyu/codelearn/internal/git"
)

func TestParseRemoteAcceptsScpStyleTargets(t *testing.T) {
	cases := map[string]git.RemoteTarget{
		"host:/srv/project":        {Host: "host", Path: "/srv/project"},
		"me@host:/srv/project":     {Host: "me@host", Path: "/srv/project"},
		"me@host:~/project":        {Host: "me@host", Path: "~/project"},
		"[::1]:/srv/project":       {Host: "[::1]", Path: "/srv/project"},
		"host:/srv/with space/app": {Host: "host", Path: "/srv/with space/app"},
	}
	for spec, want := range cases {
		got, err := git.ParseRemote(spec)
		if err != nil {
			t.Fatalf("%q: %v", spec, err)
		}
		if got != want {
			t.Fatalf("%q parsed as %+v, want %+v", spec, got, want)
		}
	}
}

func TestParseRemoteRejectsWhatItCannotAddress(t *testing.T) {
	for _, spec := range []string{
		"",
		"host",               // no path
		"host:",              // empty path
		":/srv/project",      // no host
		"host:relative/path", /* a relative remote path has no fixed meaning */
		"[::1]/srv",          // missing the colon after the bracket
	} {
		if target, err := git.ParseRemote(spec); err == nil {
			t.Fatalf("%q was accepted as %+v", spec, target)
		}
	}
}

func TestControlPathIsShortAndPerHost(t *testing.T) {
	first, _ := git.ParseRemote("one@host-with-a-very-long-name.example.com:/srv/project")
	second, _ := git.ParseRemote("two@host-with-a-very-long-name.example.com:/srv/project")

	if first.ControlPath() == second.ControlPath() {
		t.Fatal("two users share one control socket")
	}
	// A unix socket path is limited to about 108 bytes, so it must not grow
	// with the host name.
	if length := len(first.ControlPath()); length > 80 {
		t.Fatalf("control path is %d bytes: %s", length, first.ControlPath())
	}
}

func TestRemoteTransportIsLabelled(t *testing.T) {
	target, err := git.ParseRemote("me@host:/srv/project")
	if err != nil {
		t.Fatal(err)
	}
	if label := target.Transport().Label(); !strings.Contains(label, "me@host") {
		t.Fatalf("label = %q", label)
	}
	if git.Local().Label() != "local" {
		t.Fatalf("local label = %q", git.Local().Label())
	}
}
