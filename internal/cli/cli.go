// Package cli implements the hv command-line: dispatch and subcommands.
package cli

import (
	"fmt"
	"io"
)

// Command is one hv subcommand. args are the arguments following the
// subcommand name. out/errOut are where human output goes.
type Command struct {
	Name    string
	Summary string
	Run     func(args []string, out, errOut io.Writer) error
}

// registry of all subcommands, in display order.
var registry = []Command{
	cmdInit,
	cmdConfig,
	cmdAdd,
	cmdCommit,
	cmdStatus,
	cmdLog,
	cmdBranch,
	cmdHaven,
	cmdPublish,
	cmdMerge,
	cmdDiff,
	cmdReset,
	cmdRestore,
	cmdTag,
	cmdCherryPick,
	cmdRevert,
	cmdRebase,
	cmdStash,
	cmdRemote,
	cmdServe,
	cmdPush,
	cmdFetch,
	cmdPull,
	cmdClone,
	cmdSync,
	cmdKey,
	cmdMember,
	cmdGroup,
	cmdGrant,
	cmdRevoke,
	cmdRestrict,
	cmdPolicy,
	cmdSecret,
	cmdFsck,
	cmdGc,
}

// Dispatch routes argv (excluding the program name) to a subcommand.
// Returns a process exit code.
func Dispatch(argv []string, out, errOut io.Writer) int {
	if len(argv) == 0 || argv[0] == "help" || argv[0] == "-h" || argv[0] == "--help" {
		usage(out)
		return 0
	}
	name := argv[0]
	for _, c := range registry {
		if c.Name == name {
			if err := c.Run(argv[1:], out, errOut); err != nil {
				fmt.Fprintf(errOut, "hv %s: %v\n", name, err)
				return 1
			}
			return 0
		}
	}
	fmt.Fprintf(errOut, "hv: unknown command %q\n", name)
	usage(errOut)
	return 2
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "Haven — a VCS with built-in privacy and secrets")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "usage: hv <command> [args]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "commands:")
	for _, c := range registry {
		fmt.Fprintf(w, "  %-10s %s\n", c.Name, c.Summary)
	}
}
