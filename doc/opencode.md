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
| Configuration | `~/.config/opencode/` | `OPENCODE_CONFIG` (one file merged over the globals), then `OPENCODE_CONFIG_DIR/`, then `$XDG_CONFIG_HOME/opencode/` |
| Credentials | `~/.local/share/opencode/auth.json` | `$XDG_DATA_HOME/opencode/auth.json` |

Like opencode, the configuration is the three global files `config.json`,
`opencode.json` and `opencode.jsonc` from the config directory, loaded in that
order and deep-merged: maps merge recursively (a provider entry from one file
keeps fields another file sets), while scalars and arrays are replaced by the
later file. A set `OPENCODE_CONFIG` names one more file that loads after the
three and merges over them. Files may use JSONC comments and trailing
commas. Missing files produce an empty source; a file that exists but fails
substitution or parsing fails the whole load with the error (opencode silently
falls back to an empty config there). Config files and `{file:}` references
larger than 16 MiB are rejected.

Only non-empty `type: "api"` credentials are used. OAuth and `wellknown`
credentials are ignored because refreshing them would require OpenCode's OAuth
client and mutable state.

## Substitution

`{env:NAME}` and `{file:PATH}` expand in the raw file text before parsing, so
they work in any string value — `options.apiKey`, `options.baseURL`, `api`,
MCP `headers` and `environment` alike. The passes mirror opencode:

- `{env:NAME}` runs over the whole text first; a missing variable becomes the
  empty string.
- `{file:PATH}` then runs once over the result. A `~/` prefix expands against
  the home directory; any other relative path resolves against the config
  file's directory. The content is whitespace-trimmed and spliced in as an
  escaped JSON string body, so quotes, backslashes and newlines land as the
  exact parsed value.
- A token on a line whose text before it starts with `//` stays literal, so a
  commented-out reference is never an error.
- A missing, unreadable or oversized file is an error: the load fails with
  `bad file reference` naming the token and, when the file does not exist,
  the resolved path — any other cause rides along wrapped. File content never
  appears in the error.
- Because the file pass runs after the env pass over the same text, an
  environment value containing `{file:…}` still expands. File content is never
  re-expanded: a token inside a spliced value stays literal.

## Providers and models

The model list is the union of the providers holding an API key in `auth.json`
and the providers declared in the config's `provider` section. A provider named
in `disabled_providers` is skipped entirely. When `enabled_providers` is set it
is an allowlist for both auth and config providers — an empty array allows
nothing, as in opencode — and `disabled_providers` still wins over it. An
`auth.json` entry for a provider CozyPhi knows neither from its catalog nor
from the config is ignored.

The credential ladder mirrors opencode: `options.apiKey` (after substitution)
wins when present — an explicitly empty string is a value that blocks the
fallback — else the `auth.json` API key, else empty. A provider declared in
the config imports keyless either way (a local gateway, say), and its calls
carry an empty key.

The endpoint is `options.baseURL` (a trailing `/` is dropped) when set —
provider-level runtime config, it applies to every model. Otherwise each
model uses the provider-level `api` url when the config's `models` section
lists it (opencode folds `provider.api` in its per-model loop), else the
catalog's URL. The protocol is the catalog's, or the adapter named by `npm`
(`@ai-sdk/openai`, `@ai-sdk/openai-compatible`, `@ai-sdk/anthropic`); a config
provider CozyPhi does not know defaults to the openai-compatible protocol,
like opencode's fallback.

A provider is skipped only when CozyPhi cannot speak its protocol or it ends up
with no models.

A `models` section overlays the catalog list instead of replacing it, following
opencode's rules:

- The map key is the model's id; the entry's `id` field is the API id override
  — the model is listed under the key and requested under the `id`.
- A catalog model is matched by the entry's `id` when set, else by the key.
  The match's limits are overridden where the config sets them — an explicit
  `0` wins too — and kept where it leaves them unset.
- An id without a catalog match is appended with the configured limits or 0.
  When the key and the matched id differ, both entries stay listed.

Imported model names always identify their source:

```text
opencode/<provider>/<model>
```

For example, `opencode/anthropic/claude-sonnet-4-5` selects the OpenCode-backed
credential without adding it to CozyPhi's provider store.

## MCP servers

Enabled local and remote entries under `mcp` are imported without a name
prefix. Local command arrays map to stdio command and arguments; remote entries
map to HTTP URL and headers. `headers` and `environment` values arrive already
substituted, so a header can carry a credential read from disk. OpenCode
timeouts are interpreted as milliseconds. Disabled entries, incomplete entries,
and remote servers requiring OAuth are skipped.

OpenCode is the lowest-priority MCP source. A same-named server in
`~/.cozyphi/mcp.json` or `<project>/.cozyphi/mcp.json` wins. See [MCP](mcp.md)
for the full loading and invocation flow.

## Intentional deviations

CozyPhi imports opencode's configuration semantics, not opencode itself. These
differences are deliberate:

- **Global config only.** Project, remote and managed configuration layers do
  not exist here; CozyPhi reads the global files and `auth.json` only.
- **Protocol capability filter.** Providers speaking a protocol CozyPhi has no
  client for (an unrecognized `npm` adapter) are skipped rather than imported
  and failing at call time.
- **Deterministic order.** Config `models` keys are walked sorted and the
  result is sorted by name, so the import never depends on map iteration
  order.
- **No plugin auth.** opencode's plugin and custom auth loaders run arbitrary
  code; CozyPhi has no equivalent layer.
- **No per-model provider overrides.** A model entry's `provider.api` and
  `provider.npm` are ignored; the provider-level `npm` sets the protocol for
  the whole provider, and the provider-level `api` reaches only models listed
  in the config.
- **No costs or variants.** Pricing, model variants and per-variant limits are
  not modeled.
- **No model `limit.input`.** Only `limit.context` and `limit.output` are
  imported.
- **No environment-variable credential layer.** opencode resolves a
  provider-declared env var below `options.apiKey` and `auth.json`; CozyPhi's
  catalog carries no env list, so the ladder is config key, then `auth.json`,
  then empty.
- **Errors surface instead of vanishing.** A config file that fails
  substitution or parsing fails the load with the error; opencode logs and
  continues with an empty config.
