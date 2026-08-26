// Command gg is a cross-platform grep implementation. See PROCESS.md at
// the module root for the design and build plan.
package main

import (
	"os"

	"gg/internal/cli"
)

func main() {
	os.Exit(cli.Execute(os.Args[1:]))
}
