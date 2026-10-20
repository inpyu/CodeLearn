// Command codelearn is a local-first code change learning viewer.
package main

import (
	"os"

	"github.com/inpyu/codelearn/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
