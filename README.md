# CozyPhi

A comfortable, feature-rich terminal coding agent written in Go.

![CozyPhi terminal UI](doc/cozyphi.png)

## Highlights

- Interactive terminal UI with streaming output, session history, and resume support.
- Permission gates and approval-aware plans keep tool execution under user control.
- Built-in file, search, shell, LSP, question, and sub-agent tools.
- MCP integrations without loading every remote tool schema into the model context.
- Project and user hooks for customizing the agent workflow.
- Per-project agent memory: what the agent learns about you rides in every prompt, what it learns about the work is retrieved for the turn that needs it, and what stops earning its place is compacted or forgotten — reversibly.
- Headless mode with JSONL output for scripts and CI.

## Install

### macOS and Linux

```sh
curl -fsSL https://raw.githubusercontent.com/alvnukov/cozyphi/main/scripts/install.sh | bash
```

### Windows

```powershell
irm https://raw.githubusercontent.com/alvnukov/cozyphi/main/scripts/install.ps1 | iex
```

Release binaries are also available on the [GitHub Releases](https://github.com/alvnukov/cozyphi/releases) page.

## Quick start

Configure a model and API key, then start the TUI in your project directory:

```sh
cozyphi config
cozyphi
```

You can also configure the default model with `COZYPHI_MODEL` and `COZYPHI_API_KEY`.

Useful commands:

```text
cozyphi -c                   resume the newest session for this directory
cozyphi --resume ID          resume a session by id or unique prefix
cozyphi run -p "..."         run one agent loop headlessly
cozyphi sessions list        list sessions for this directory
cozyphi memory               show what the agent remembers here
cozyphi mcp --help           manage MCP servers
cozyphi update               install the latest release
```

Run `cozyphi --help` or `cozyphi <command> --help` for the full command reference.

## Documentation

- [Hooks](doc/hooks.md)
- [MCP](doc/mcp.md)
- [Terminal UI](doc/tui.md)
- [Project layout](doc/project-layout.md)
- [Contributing](CONTRIBUTING.md)

## Build from source

CozyPhi requires Go 1.26.3 or newer.

```sh
git clone https://github.com/alvnukov/cozyphi.git
cd cozyphi
make build
```

Before submitting a change, run:

```sh
make fmt-check lint test
```

## License

[MIT](LICENSE)
