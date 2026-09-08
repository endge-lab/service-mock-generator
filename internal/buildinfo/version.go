package buildinfo

import "os"
import "strings"

// Version is embedded from VERSION by Make, Air and Docker.
var Version string

func Resolve() string {
	if v := strings.TrimSpace(Version); v != "" {
		return v
	}
	if b, e := os.ReadFile("VERSION"); e == nil {
		return strings.TrimSpace(string(b))
	}
	return "0.0.0-unversioned"
}
