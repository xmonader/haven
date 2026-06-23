package cli

import (
	"fmt"
	"strings"
)

// flagValue extracts the value of a flag from args, supporting both
// "-m value" and "-m=value" forms. Returns "" if the flag is absent.
func flagValue(args []string, flag string) (string, error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == flag {
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s requires a value", flag)
			}
			return args[i+1], nil
		}
		if strings.HasPrefix(a, flag+"=") {
			return a[len(flag)+1:], nil
		}
	}
	return "", nil
}

// hasFlag reports whether a boolean flag is present.
func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

// positional returns args that are not flags (do not start with '-') and are
// not the value consumed by a known value-flag. For the simple commands here,
// callers pass the set of value-flags to skip.
func positional(args []string, valueFlags ...string) []string {
	skip := map[string]bool{}
	for _, f := range valueFlags {
		skip[f] = true
	}
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if skip[a] {
			i++ // skip its value
			continue
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		out = append(out, a)
	}
	return out
}
