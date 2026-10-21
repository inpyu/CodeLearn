package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Transport runs git commands and reads working tree files, either on this
// machine or on a host reached over SSH.
//
// Every repository operation goes through one of these two methods, which is
// what lets a remote repository work without installing anything on the host:
// the remote side only needs git and a POSIX shell.
type Transport interface {
	// Run executes git in dir, optionally feeding it stdin.
	Run(ctx context.Context, dir string, stdin []byte, args ...string) ([]byte, error)
	// ReadFiles reads working tree files, given as paths relative to dir.
	// A path that is missing, is not a regular file, or is a symlink is
	// simply absent from the result.
	ReadFiles(ctx context.Context, dir string, paths []string) map[string][]byte
	// Label describes the transport for messages, e.g. "ssh://user@host".
	Label() string
}

// -- local ------------------------------------------------------------------

type localTransport struct{}

// Local returns the transport for a repository on this machine.
func Local() Transport { return localTransport{} }

func (localTransport) Label() string { return "local" }

func (localTransport) Run(ctx context.Context, dir string, stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, commandError(ctx, args, stderr.String(), err)
	}
	return stdout.Bytes(), nil
}

func (localTransport) ReadFiles(_ context.Context, dir string, paths []string) map[string][]byte {
	result := make(map[string][]byte, len(paths))
	for _, path := range paths {
		target := filepath.Join(dir, path)
		// Lstat rather than Stat: a symlink is refused rather than followed,
		// so a link pointing out of the repository cannot be read.
		info, err := os.Lstat(target)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if data, err := os.ReadFile(target); err == nil {
			result[path] = data
		}
	}
	return result
}

// -- ssh --------------------------------------------------------------------

type sshTransport struct {
	host string
	// run executes the ssh client; tests replace it to drive the composed
	// remote script through a local shell instead of a network.
	run func(ctx context.Context, stdin []byte, script string) ([]byte, error)
}

// SSH returns a transport that drives git on host over SSH.
//
// Connections are multiplexed over one master, so the many short commands a
// snapshot needs cost one handshake rather than one each.
func SSH(host string, controlPath string) Transport {
	return &sshTransport{host: host, run: sshRunner(host, controlPath)}
}

func (s *sshTransport) Label() string { return "ssh://" + s.host }

func sshRunner(host, controlPath string) func(context.Context, []byte, string) ([]byte, error) {
	return func(ctx context.Context, stdin []byte, script string) ([]byte, error) {
		args := []string{
			"-o", "BatchMode=yes",
			"-o", "ControlMaster=auto",
			"-o", "ControlPath=" + controlPath,
			"-o", "ControlPersist=60",
			host, script,
		}
		cmd := exec.CommandContext(ctx, "ssh", args...)
		if stdin != nil {
			cmd.Stdin = bytes.NewReader(stdin)
		}
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("ssh %s: %s", host, firstLine(stderr.String(), err))
		}
		return stdout.Bytes(), nil
	}
}

func (s *sshTransport) Run(ctx context.Context, dir string, stdin []byte, args ...string) ([]byte, error) {
	script := "git -C " + shellQuote(dir)
	for _, arg := range args {
		script += " " + shellQuote(arg)
	}
	out, err := s.run(ctx, stdin, script)
	if err != nil {
		return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

// readFilesScript emits each file as a byte count on its own line followed by
// exactly that many bytes, so the stream can be split without a delimiter
// that file contents could contain. A path that is missing, is not a regular
// file, or is a symlink emits "-" instead.
func readFilesScript(dir string, paths []string) string {
	var script strings.Builder
	script.WriteString("cd " + shellQuote(dir) + " || exit 1\n")
	for _, path := range paths {
		quoted := shellQuote(path)
		script.WriteString("if [ -f " + quoted + " ] && [ ! -L " + quoted + " ]; then ")
		script.WriteString("wc -c < " + quoted + "; cat -- " + quoted + "; ")
		script.WriteString("else echo -; fi\n")
	}
	return script.String()
}

func (s *sshTransport) ReadFiles(ctx context.Context, dir string, paths []string) map[string][]byte {
	result := make(map[string][]byte, len(paths))
	if len(paths) == 0 {
		return result
	}
	out, err := s.run(ctx, nil, readFilesScript(dir, paths))
	if err != nil {
		return result
	}
	parseFramedFiles(out, paths, result)
	return result
}

// parseFramedFiles walks the length-prefixed stream readFilesScript produces.
func parseFramedFiles(out []byte, paths []string, into map[string][]byte) {
	offset := 0
	for _, path := range paths {
		newline := bytes.IndexByte(out[offset:], '\n')
		if newline < 0 {
			return
		}
		header := strings.TrimSpace(string(out[offset : offset+newline]))
		offset += newline + 1
		if header == "-" {
			continue
		}
		size, err := strconv.Atoi(header)
		if err != nil || size < 0 || offset+size > len(out) {
			return
		}
		content := make([]byte, size)
		copy(content, out[offset:offset+size])
		into[path] = content
		offset += size
	}
}

// shellQuote wraps an argument so a POSIX shell passes it through untouched.
//
// SSH hands its command to the remote login shell, and paths come from the
// repository being inspected, so a file named "; rm -rf ~" would otherwise be
// executed on the remote host. Single quotes suppress every expansion; the
// only character they cannot contain is a single quote, which is closed,
// escaped and reopened.
func shellQuote(argument string) string {
	return "'" + strings.ReplaceAll(argument, "'", `'\''`) + "'"
}

func commandError(ctx context.Context, args []string, stderr string, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("git %s timed out", strings.Join(args, " "))
	}
	return fmt.Errorf("git %s: %s", strings.Join(args, " "), firstLine(stderr, err))
}

func firstLine(stderr string, err error) string {
	message := strings.TrimSpace(stderr)
	if message == "" {
		return err.Error()
	}
	if newline := strings.IndexByte(message, '\n'); newline >= 0 {
		message = message[:newline]
	}
	return message
}
