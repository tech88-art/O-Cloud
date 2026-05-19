// Package version exposes build-time metadata injected via -ldflags.
//
// Defaults below are used for local `go build` (no ldflags) so the binary
// still runs and reports something useful. CI / Dockerfile / Makefile
// override Version, Commit and BuildTime via -ldflags "-X .../version.X=...".
package version

var (
	// Version is the semantic version of the exporter. Injected at build time
	// via -ldflags "-X .../version.Version=...". Default "dev" for local builds.
	Version = "dev"
	// Commit is the short git SHA. Injected via -ldflags. Default "none".
	Commit = "none"
	// BuildTime is the ISO-8601 UTC timestamp of the build. Injected via
	// -ldflags. Default "unknown".
	BuildTime = "unknown"
)
