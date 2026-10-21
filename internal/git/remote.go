package git

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RemoteTarget is a repository on another host, written the way scp and rsync
// write one: "[user@]host:/path/to/repo".
type RemoteTarget struct {
	Host string
	Path string
}

// ParseRemote reads a remote target.
func ParseRemote(spec string) (RemoteTarget, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return RemoteTarget{}, fmt.Errorf("empty remote target")
	}

	host, path := "", ""
	if strings.HasPrefix(spec, "[") {
		// A bracketed IPv6 literal: [::1]:/path.
		end := strings.Index(spec, "]:")
		if end < 0 {
			return RemoteTarget{}, fmt.Errorf(
				"remote target must look like [user@]host:/path, got %q", spec)
		}
		host, path = spec[:end+1], spec[end+2:]
	} else {
		colon := strings.Index(spec, ":")
		if colon <= 0 {
			return RemoteTarget{}, fmt.Errorf(
				"remote target must look like [user@]host:/path, got %q", spec)
		}
		host, path = spec[:colon], spec[colon+1:]
	}

	if path == "" {
		return RemoteTarget{}, fmt.Errorf("remote target %q has no path", spec)
	}
	if !strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "~") {
		return RemoteTarget{}, fmt.Errorf(
			"remote path must be absolute or start with ~, got %q", path)
	}
	return RemoteTarget{Host: host, Path: path}, nil
}

// String renders the target in the form it was parsed from.
func (t RemoteTarget) String() string { return t.Host + ":" + t.Path }

// ControlPath is the socket the SSH connections are multiplexed over. It is
// derived from the host rather than named after it, because a unix socket path
// is short and a host name is not.
func (t RemoteTarget) ControlPath() string {
	sum := sha256.Sum256([]byte(t.Host))
	return filepath.Join(os.TempDir(), "codelearn-"+hex.EncodeToString(sum[:])[:12]+".sock")
}

// Transport builds the SSH transport for this target.
func (t RemoteTarget) Transport() Transport { return SSH(t.Host, t.ControlPath()) }
