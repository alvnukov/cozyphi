module github.com/pulseaiclub/phi

go 1.26.3

// Local fork of the TUI framework: renderer patches live here (see xui/PATCH_NOTES.md).
replace github.com/pulseaiclub/xui => ./xui

require (
	github.com/alecthomas/chroma/v2 v2.27.0
	github.com/pulseaiclub/xui v0.1.3
	github.com/stretchr/testify v1.11.1
	github.com/yuin/goldmark v1.8.5
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dlclark/regexp2/v2 v2.2.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
)
