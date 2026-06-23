package cli

import (
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
)

// Version is the build version, injected at link time via
//
//	-ldflags "-X haven/internal/cli.Version=v1.2.3"
//
// When built without that flag (e.g. `go build`), it falls back to the VCS
// revision recorded in the build info, or "dev".
var Version = ""

var cmdVersion = Command{
	Name:    "version",
	Summary: "print the hv version and build info",
	Run:     runVersion,
}

func runVersion(args []string, out, errOut io.Writer) error {
	fmt.Fprintln(out, versionString())
	return nil
}

// versionString renders the version plus the platform/toolchain it was built
// with, so a bug report identifies the exact binary.
func versionString() string {
	v := Version
	if v == "" {
		v = "dev"
		if bi, ok := debug.ReadBuildInfo(); ok {
			for _, s := range bi.Settings {
				if s.Key == "vcs.revision" {
					rev := s.Value
					if len(rev) > 12 {
						rev = rev[:12]
					}
					v = "dev+" + rev
				}
			}
		}
	}
	return fmt.Sprintf("hv %s (%s/%s, %s)", v, runtime.GOOS, runtime.GOARCH, runtime.Version())
}
