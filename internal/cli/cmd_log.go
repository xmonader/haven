package cli

import (
	"fmt"
	"io"
	"time"

	"haven/internal/ref"
)

var cmdLog = Command{
	Name:    "log",
	Summary: "show commit history of the current ref",
	Run:     runLog,
}

func runLog(args []string, out, errOut io.Writer) error {
	r, store, err := openRepo()
	if err != nil {
		return err
	}
	defer r.Close()

	headRef, err := r.Head()
	if err != nil {
		return err
	}
	cur, err := ref.Resolve(r.DB, headRef)
	if err != nil {
		return err
	}
	if cur == "" {
		fmt.Fprintln(out, "no commits yet")
		return nil
	}

	for cur != "" {
		c, err := store.GetCommit(cur)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "commit %s\n", cur)
		fmt.Fprintf(out, "Author: %s <%s>\n", c.Author, c.Email)
		fmt.Fprintf(out, "Date:   %s\n\n", time.Unix(c.When, 0).Format(time.RFC1123))
		for _, line := range splitLines(c.Message) {
			fmt.Fprintf(out, "    %s\n", line)
		}
		fmt.Fprintln(out)
		if len(c.Parents) == 0 {
			break
		}
		cur = c.Parents[0]
	}
	return nil
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}
