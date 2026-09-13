module github.com/alvnukov/cozyphi

go 1.26.3

// Local fork of the TUI framework: renderer patches live here (see xui/PATCH_NOTES.md).
replace github.com/pulseaiclub/xui => ./xui

require (
	github.com/alecthomas/chroma/v2 v2.27.0
	github.com/alvnukov/cozy-tools v0.2.0
	github.com/pulseaiclub/xui v0.1.3
	github.com/rivo/uniseg v0.4.7
	github.com/stretchr/testify v1.12.1
	github.com/yuin/goldmark v1.8.6
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/dlclark/regexp2/v2 v2.2.1 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
)
