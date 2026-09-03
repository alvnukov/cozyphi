package version

// Version is the current CozyPhi release shown on the welcome screen and
// used by `cozyphi update`. The fork numbers its own releases starting at
// v0.1.0 (upstream cozyphi history stays in CHANGELOG.md). Override at build
// time with:
//
//	go build -ldflags="-X github.com/alvnukov/cozyphi/internal/version.Version=v0.1.0"
var Version = "v0.19.0"
