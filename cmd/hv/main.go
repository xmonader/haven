// Command hv is the Haven version control system.
package main

import (
	"os"

	"haven/internal/cli"
)

func main() {
	os.Exit(cli.Dispatch(os.Args[1:], os.Stdout, os.Stderr))
}
