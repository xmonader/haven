package cli

import (
	"fmt"
	"io"

	"haven/internal/config"
)

var cmdConfig = Command{
	Name:    "config",
	Summary: "get or set repository config (e.g. user.name)",
	Run:     runConfig,
}

func runConfig(args []string, out, errOut io.Writer) error {
	r, _, err := openRepo()
	if err != nil {
		return err
	}
	defer r.Close()

	switch len(args) {
	case 1:
		v, ok, err := config.Get(r.DB, args[0])
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%s: not set", args[0])
		}
		fmt.Fprintln(out, v)
		return nil
	case 2:
		return config.Set(r.DB, args[0], args[1])
	default:
		return fmt.Errorf("usage: hv config <key> [<value>]")
	}
}
