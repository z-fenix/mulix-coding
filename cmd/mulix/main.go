// Command mulix is the CLI entry point for mulix-coding: a spec-driven
// development workflow with enforced phase gates for Claude Code.
package main

import (
	"fmt"
	"os"

	"github.com/mulix-dev/mulix-coding/internal/cliutil"
)

// version is overridden at build time via -ldflags, e.g.:
//
//	go build -ldflags "-X main.version=1.2.3" ./cmd/mulix
var version = "dev"

func main() {
	cliutil.Version = version
	if err := cliutil.NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "mulix: "+err.Error())
		os.Exit(1)
	}
}
