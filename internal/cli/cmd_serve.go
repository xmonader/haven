package cli

import (
	"fmt"
	"io"
	"net/http"

	"haven/internal/protocol"
)

var cmdServe = Command{
	Name:    "serve",
	Summary: "serve this repository over HTTP(S) (--addr, --kind, --tls-cert/--tls-key)",
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
	cert, err := flagValue(args, "--tls-cert")
	if err != nil {
		return err
	}
	key, err := flagValue(args, "--tls-key")
	if err != nil {
		return err
	}
	if (cert == "") != (key == "") {
		return fmt.Errorf("--tls-cert and --tls-key must be given together")
	}

	r, _, err := openRepo()
	if err != nil {
		return err
	}
	defer r.Close()

	srv := protocol.NewServer(r.DB, kind)
	if cert != "" {
		fmt.Fprintf(out, "serving %s repository on https://%s\n", kind, addr)
		return http.ListenAndServeTLS(addr, cert, key, srv.Handler())
	}
	fmt.Fprintf(out, "serving %s repository on %s (no TLS — signed requests prevent forgery, not eavesdropping)\n", kind, addr)
	return http.ListenAndServe(addr, srv.Handler())
}
