# OpenCode integration

CozyPhi can use provider API keys, model definitions, and MCP servers already
configured by [OpenCode](https://opencode.ai/) as a read-only source. Nothing is
copied into `~/.cozyphi`, and CozyPhi never updates OpenCode files or OAuth
state.

## Enable or disable

The integration is enabled when `opencode.enabled` is absent or `true`:

```yaml
opencode:
  enabled: false
```

The same setting is available under `/settings` → General as **OpenCode
integration**. The source and MCP pool are loaded at startup, so restart
CozyPhi after changing the setting or OpenCode configuration.

## Files and paths

CozyPhi reads OpenCode's global files directly:

| Data | Default | Overrides |
| --- | --- | --- |
| Configuration | `~/.config/opencode/opencode.json` | `OPENCODE_CONFIG`, then `OPENCODE_CONFIG_DIR/opencode.json`, then `$XDG_CONFIG_HOME/opencode/opencode.json` |
| Credentials | `~/.local/share/opencode/auth.json` | `$XDG_DATA_HOME/opencode/auth.json` |

Missing files produce an empty source. Configuration may use JSONC comments,
trailing commas, and `{env:NAME}` substitutions. Files larger than 16 MiB are
rejected.

Only non-empty `type: "api"` credentials are used. OAuth and `wellknown`
credentials are ignored because refreshing them would require OpenCode's OAuth
client and mutable state.

## Providers and models

The model list is the union of the providers holding an API key in `auth.json`
and the providers declared in opencode.json's `provider` section. A provider
named in the top-level `disabled_providers` array is skipped entirely, and an
`auth.json` entry for a provider CozyPhi knows neither from its catalog nor
from opencode.json is ignored.

Each provider needs an endpoint, at least one model, and a supported
protocol:

- The endpoint is the catalog's URL, or `options.baseURL` when the entry sets
  one (a trailing `/` is dropped).
- The protocol is the catalog's, or the adapter named by `npm`:
  - `@ai-sdk/openai`
  - `@ai-sdk/openai-compatible`
  - `@ai-sdk/anthropic`

A provider that exists only in `auth.json` still needs that key. A provider
declared in opencode.json imports even without one — a local gateway, say —
and its calls carry an empty key.

The credential for a provider is the `auth.json` API key when present, else
`options.apiKey`, else empty. `options.apiKey` accepts a plain string and
reference forms:

- `{env:NAME}` or `{"env":"NAME"}` — the environment variable's value,
  empty when unset.
- `{file:PATH}` or `{"file":"PATH"}` — the file's content with surrounding
  whitespace trimmed. A missing or unreadable file yields an empty key: the
  provider still imports, and the endpoint's own auth error reports the
  problem instead of the model silently disappearing.

`{env:…}` tokens expand anywhere in the config file — that pass runs over the
whole file before parsing, mirroring opencode. `{file:PATH}` never gets that
text pass: splicing file content into the raw text could corrupt the JSON
parse. It expands after parsing instead, as an embedded token in MCP `headers`
and `environment` values — so `Authorization: Bearer {file:…}` sends the
file's trimmed content with the surrounding text intact. An `apiKey`
reference is still recognised only as a whole value; one embedded in a longer
string stays part of the key. A missing or unreadable file expands to an empty
string and never fails the load — the endpoint's own auth error reports the
problem.

A `models` section overlays the catalog list instead of replacing it: catalog
models stay, an entry with a known id overrides it (`limit.context` and
`limit.output` win only when set above zero), and an entry with a new id is
added — its `id` field names the model, with the map key as fallback.

Imported model names always identify their source:

```text
opencode/<provider>/<model>
```

For example, `opencode/anthropic/claude-sonnet-4-5` selects the OpenCode-backed
credential without adding it to CozyPhi's provider store.

## MCP servers

Enabled local and remote entries under `mcp` are imported without a name
prefix. Local command arrays map to stdio command and arguments; remote entries
map to HTTP URL and headers. `headers` and `environment` values expand embedded
`{file:PATH}` tokens after parsing, so a header can carry a credential read
from disk. OpenCode timeouts are interpreted as milliseconds. Disabled entries,
incomplete entries, and remote servers requiring OAuth are skipped.

OpenCode is the lowest-priority MCP source. A same-named server in
`~/.cozyphi/mcp.json` or `<project>/.cozyphi/mcp.json` wins. See [MCP](mcp.md)
for the full loading and invocation flow.
