package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	srv := &http.Server{Addr: addr, Handler: logRequests(protocol.NewServer(r.DB, kind).Handler(), out)}

	// Shut down cleanly on SIGINT/SIGTERM so in-flight requests finish.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		fmt.Fprintln(out, "\nshutting down…")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	scheme := "http"
	listen := srv.ListenAndServe
	if cert != "" {
		scheme = "https"
		listen = func() error { return srv.ListenAndServeTLS(cert, key) }
	} else {
		fmt.Fprintln(out, "note: no TLS — signed requests prevent forgery, not eavesdropping")
	}
	fmt.Fprintf(out, "serving %s repository on %s://%s (Ctrl-C to stop)\n", kind, scheme, addr)

	if err := listen(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// statusRecorder captures the response status for logging.
type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (r *statusRecorder) WriteHeader(c int) {
	r.code = c
	r.ResponseWriter.WriteHeader(c)
}

// logRequests writes one access-log line per request: method, path, status,
// duration, and the requester's key prefix (or "anon").
func logRequests(h http.Handler, out io.Writer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
		start := time.Now()
		h.ServeHTTP(rec, req)
		who := "anon"
		if pub := req.Header.Get(protocol.HdrPub); pub != "" {
			if len(pub) > 12 {
				pub = pub[:12]
			}
			who = pub
		}
		fmt.Fprintf(out, "%s %s %d %s %s\n",
			req.Method, req.URL.Path, rec.code, time.Since(start).Round(time.Millisecond), who)
	})
}
