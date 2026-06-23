package cli

import (
	"fmt"
	"io"
	"net/http"

	"haven/internal/protocol"
)

var cmdServe = Command{
	Name:    "serve",
	Summary: "serve this repository over HTTP (--addr, --kind team|personal)",
	Run:     runServe,
}

func runServe(args []string, out, errOut io.Writer) error {
	addr, err := flagValue(args, "--addr")
	if err != nil {
		return err
	}
	if addr == "" {
		addr = ":8473"
	}
	kind, err := flagValue(args, "--kind")
	if err != nil {
		return err
	}
	if kind == "" {
		kind = protocol.KindTeam
	}
	if kind != protocol.KindTeam && kind != protocol.KindPersonal {
		return fmt.Errorf("--kind must be team or personal")
	}

	r, _, err := openRepo()
	if err != nil {
		return err
	}
	defer r.Close()

	srv := protocol.NewServer(r.DB, kind)
	fmt.Fprintf(out, "serving %s repository on %s\n", kind, addr)
	return http.ListenAndServe(addr, srv.Handler())
}
