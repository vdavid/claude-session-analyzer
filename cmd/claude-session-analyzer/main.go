// Command claude-session-analyzer reconstructs where a Claude Code session's time went.
//
// Everything it does lives in internal/cli, so the whole surface can be tested without a process.
package main

import (
	"os"

	"github.com/vdavid/claude-session-analyzer/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
