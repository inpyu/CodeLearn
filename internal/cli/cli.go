// Package cli implements the codelearn command line.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/inpyu/codelearn/internal/analysis"
	"github.com/inpyu/codelearn/internal/git"
	"github.com/inpyu/codelearn/internal/server"
)

// Version is the released version, also reported by the API.
const Version = "0.3.0"

// Exit codes. They are distinct so a script can tell "not a repository" from
// "the analysis was rejected".
const (
	ExitOK           = 0
	ExitInvalidInput = 1
	ExitGitError     = 2
)

var commands = map[string]bool{"serve": true, "export": true, "import": true}

// Run executes the CLI and returns a process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	server.Version = Version

	// "codelearn ." and "codelearn /path/to/repo" implicitly run the viewer:
	// a leading token that is neither a known command nor a flag means the
	// default command was omitted.
	if len(args) == 0 || (!commands[args[0]] && !isFlag(args[0])) {
		args = append([]string{"serve"}, args...)
	} else if isFlag(args[0]) && !isGlobalFlag(args[0]) {
		args = append([]string{"serve"}, args...)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch args[0] {
	case "serve":
		return runServe(ctx, args[1:], stdout, stderr)
	case "export":
		return runExport(ctx, args[1:], stdout, stderr)
	case "import":
		return runImport(ctx, args[1:], stdout, stderr)
	default:
		return runGlobal(args, stdout, stderr)
	}
}

func isFlag(arg string) bool { return len(arg) > 0 && arg[0] == '-' }

func isGlobalFlag(arg string) bool {
	switch arg {
	case "-h", "--help", "-help", "--version", "-version":
		return true
	}
	return false
}

func runGlobal(args []string, stdout, _ io.Writer) int {
	if args[0] == "--version" || args[0] == "-version" {
		fmt.Fprintf(stdout, "codelearn %s\n", Version)
		return ExitOK
	}
	usage(stdout)
	return ExitOK
}

func usage(w io.Writer) {
	fmt.Fprintf(w, `CodeLearn %s - local-first code change learning viewer

Usage:
  codelearn [path] [flags]          open the web viewer (default command)
  codelearn export [path] [flags]   export the current change units as JSON
  codelearn import <file> [path]    import an analysis produced by a skill

Flags for the viewer:
  --no-browser        do not launch a browser
  --port <n>          port to bind (default %d, falls back to a free port)
  --host <addr>       host to bind (default %s)

Flags for export:
  -o, --output <file> output file (default changes.json)

Other:
  --version           print the version
  -h, --help          print this message
`, Version, server.DefaultPort, server.DefaultHost)
}

func openRepository(ctx context.Context, path string, stderr io.Writer) (*git.Repository, int) {
	repo, err := git.Discover(ctx, path)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return nil, ExitGitError
	}
	return repo, ExitOK
}

func runServe(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	noBrowser := flags.Bool("no-browser", false, "do not launch a browser")
	port := flags.Int("port", server.DefaultPort, "port to bind")
	host := flags.String("host", server.DefaultHost, "host to bind")
	path, code := parse(flags, args, ".", stderr)
	if code != ExitOK {
		return code
	}

	repo, code := openRepository(ctx, path, stderr)
	if code != ExitOK {
		return code
	}
	snapshot, err := git.Collect(ctx, repo)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return ExitGitError
	}

	srv, err := server.New(repo, *host, *port)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return ExitGitError
	}

	fmt.Fprintf(stdout, "CodeLearn v%s\n\n", Version)
	fmt.Fprintf(stdout, "Repository: %s\n", repo.Root)
	fmt.Fprintf(stdout, "Branch: %s\n", snapshot.Branch)
	fmt.Fprintf(stdout, "Changed Files: %d\n\n", len(snapshot.Files))
	fmt.Fprintf(stdout, "Viewer: %s\n\n", srv.URL)
	fmt.Fprintf(stdout, "Press Ctrl+C to stop.\n")

	if !*noBrowser {
		if err := server.OpenBrowser(srv.URL); err != nil {
			fmt.Fprintf(stdout, "\nCould not launch a browser; open %s yourself.\n", srv.URL)
		}
	}
	if err := srv.Serve(ctx); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return ExitGitError
	}
	return ExitOK
}

func runExport(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("export", flag.ContinueOnError)
	flags.SetOutput(stderr)
	output := flags.String("output", "changes.json", "output file")
	flags.StringVar(output, "o", "changes.json", "output file")
	path, code := parse(flags, args, ".", stderr)
	if code != ExitOK {
		return code
	}

	repo, code := openRepository(ctx, path, stderr)
	if code != ExitOK {
		return code
	}
	snapshot, err := git.Collect(ctx, repo)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return ExitGitError
	}
	if err := analysis.WriteExport(snapshot, *output); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return ExitInvalidInput
	}
	fmt.Fprintf(stdout, "Exported %d change unit(s) from %d file(s)\n",
		len(snapshot.Changes()), len(snapshot.Files))
	// The snapshot id is echoed because it is the value the agent must copy
	// into its analysis.
	fmt.Fprintf(stdout, "Snapshot: %s\n", snapshot.ID)
	fmt.Fprintf(stdout, "Output: %s\n", *output)
	return ExitOK
}

func runImport(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("import", flag.ContinueOnError)
	flags.SetOutput(stderr)
	rest, code := positionals(flags, args, 2, stderr)
	if code != ExitOK {
		return code
	}
	if len(rest) == 0 {
		fmt.Fprintln(stderr, "error: import needs an analysis file")
		return ExitInvalidInput
	}
	file := rest[0]
	path := "."
	if len(rest) > 1 {
		path = rest[1]
	}

	repo, code := openRepository(ctx, path, stderr)
	if code != ExitOK {
		return code
	}
	snapshot, err := git.Collect(ctx, repo)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return ExitGitError
	}

	result, err := analysis.ImportFile(repo.Root, snapshot, file)
	if err != nil {
		if result != nil {
			for _, problem := range result.Problems {
				fmt.Fprintf(stderr, "  %s\n", problem)
			}
		}
		if errors.Is(err, analysis.ErrInvalid) {
			fmt.Fprintln(stderr, "error: the analysis was rejected; nothing was stored")
		} else {
			fmt.Fprintf(stderr, "error: %v\n", err)
		}
		return ExitInvalidInput
	}
	fmt.Fprintf(stdout, "Imported %d learning step(s)\n", result.Steps)
	fmt.Fprintf(stdout, "Stored: %s\n", result.StoredPath)
	fmt.Fprintln(stdout, "Refresh the CodeLearn viewer to see the learning steps.")
	return ExitOK
}

// positionals parses the flag set and collects the positional arguments.
//
// Go's flag package stops parsing at the first non-flag argument, so parsing
// is resumed after each positional. Without that, "codelearn export . -o
// x.json" would treat "-o" as a second positional rather than a flag.
func positionals(flags *flag.FlagSet, args []string, limit int, stderr io.Writer) ([]string, int) {
	var found []string
	rest := args
	for {
		if err := flags.Parse(rest); err != nil {
			return nil, ExitInvalidInput
		}
		rest = flags.Args()
		if len(rest) == 0 {
			break
		}
		found = append(found, rest[0])
		rest = rest[1:]
	}
	if len(found) > limit {
		fmt.Fprintf(stderr, "error: unexpected arguments: %v\n", found[limit:])
		return nil, ExitInvalidInput
	}
	return found, ExitOK
}

// parse returns the single optional positional argument of a subcommand.
func parse(flags *flag.FlagSet, args []string, fallback string, stderr io.Writer) (string, int) {
	found, code := positionals(flags, args, 1, stderr)
	if code != ExitOK {
		return "", code
	}
	if len(found) == 1 {
		return found[0], ExitOK
	}
	return fallback, ExitOK
}
