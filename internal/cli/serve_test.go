package cli

import (
	"strings"
	"testing"
)

func TestServeRequiresBothTLSFlags(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	out, code := run(t, dir, "serve", "--tls-cert", "x.pem")
	if code == 0 || !strings.Contains(out, "must be given together") {
		t.Fatalf("expected TLS flag-pairing error, got code=%d out=%q", code, out)
	}
}
