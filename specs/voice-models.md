# Voice: speech model install

Status: approved design, 2026-09-03. Companion to `specs/voice-input.md` and
`specs/voice-dialog.md`.

## Problem

Homebrew's `whisper-cpp` ships no usable model (`for-tests-ggml-tiny.bin`
deliberately does not match `ggml-*.bin`), so everyone who followed the
"brew install ffmpeg whisper-cpp" step lands in the same state: capture works,
`whisper-cli` is on PATH, and cozyphi answers Ctrl+G with

    no transcriber configured — install whisper-cpp and a ggml model, or set voice.stt.base_url and api_key

The hint is wrong (whisper-cpp *is* installed) and unhelpful (the user has to
find the model URL, the directory and the file name themselves). The user's
words: instead of an error, say that the model is not installed and offer to
download and set everything up.

## Goals

- When `whisper-cli` is present and no model is, Ctrl+G shows an offer, not an
  error. Enter downloads the default model in the background, selects it
  without a restart, persists the choice to `config.yaml` and tells the user
  voice is ready. Esc / "Not now" leaves a one-line hint how to do it later.
- `/voice install [name]` and `/voice models` give the same power explicitly
  and let the user pick a bigger model.
- The download is resumable, verified and atomic. Quitting cozyphi mid-way
  loses nothing.
- Resolver hints tell apart "no whisper-cli", "no model" and "voice.stt.model
  points nowhere".
- Nothing is downloaded without the user's Enter or an explicit command.

## Non-goals

- Installing `whisper-cpp` or `ffmpeg` (the hint names `brew install`).
- Core ML / quantized variants, checksum verification against a manifest,
  a persistent whisper server. (Follow-ups below.)
- Changing the language-detection behaviour (`voice.language: auto` costs a
  second encoder pass per segment; separate task).

## Default model: `small`

Measured on the user's Apple M2 with Homebrew whisper-cli 1.9.2 (Metal),
`jfk.wav` (11 s), whole process wall time, warm cache:

| model            | `-l en` | `-l auto` |
|------------------|---------|-----------|
| small            | 3.3 s   | 3.9 s     |
| large-v3-turbo   | 9.4 s   | 16.2 s    |

whisper.cpp pads every segment to a 30 s encoder window and the turbo encoder
is the large-v3 encoder, so per-segment latency is model-bound, not
length-bound: a 0.6 s "1, 2, 3" took 17.5 s with turbo. Dialog mode sends many
short segments, so `small` (~466 MB) is the default the offer downloads;
better Russian accuracy is one `/voice install medium` or
`/voice install large-v3-turbo` away and the catalog line says so.

## Catalog (`internal/voice/models.go`)

```go
// Model is one whisper.cpp ggml model cozyphi knows how to fetch.
type Model struct {
    Name        string // "small"
    File        string // "ggml-small.bin"
    URL         string // https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin
    ApproxBytes int64  // display only, never used for verification
    rank        int    // auto-selection order, higher is better
}

const DefaultModel = "small"

func Catalog() []Model                       // tiny, base, small, medium, large-v3-turbo, large-v3
func LookupModel(name string) (Model, bool)  // "small" | "ggml-small" | "ggml-small.bin"
```

| name           | approx size | rank |
|----------------|-------------|------|
| tiny           | 75 MB       | 0    |
| base           | 142 MB      | 1    |
| small          | 466 MB      | 2    |
| medium         | 1.5 GB      | 3    |
| large-v3       | 3.1 GB      | 4    |
| large-v3-turbo | 1.6 GB      | 5    |

`FormatBytes(n int64) string` renders `466 MB` / `1.6 GB` (one decimal for GB,
none for MB) and is used for both approx sizes and progress.

`InstalledModels(dirs []string) []Installed` lists every `ggml-*.bin` in the
model dirs (models dir first, then the extra dirs), each with `Path`, `Bytes`
and the catalog entry it maps to (longest catalog name that prefixes the part
between `ggml-` and `.bin`, so `ggml-medium-q5_0.bin` maps to medium and
`ggml-large-v3-turbo-q8_0.bin` to large-v3-turbo; no match → rank -1).

## Resolver changes (`internal/voice/config.go`)

`ResolvedSTT` gains a typed reason the UI can branch on:

```go
type Missing int

const (
    MissingNone            Missing = iota
    MissingBinary                  // the STT binary is not on PATH
    MissingModel                   // binary present, no ggml-*.bin in any model dir
    MissingConfiguredModel         // voice.stt.model is set but no file matches
)

type ResolvedSTT struct {
    Backend   Backend
    Command   string
    ModelPath string
    Hint      string
    Missing   Missing
}
```

Hints (exact strings; the first 8 words are asserted by tests):

- `MissingBinary`, auto mode: `whisper-cli not found — brew install whisper-cpp, or set voice.stt.base_url and api_key`
- `MissingBinary`, explicit command: unchanged `voice.stt.command needs <bin> on PATH — install whisper-cpp`
- `MissingModel` (both modes): `no speech model installed — /voice install downloads ggml-small (~466 MB)`
- `MissingConfiguredModel`: `voice.stt.model not found: <value> — /voice install, or fix the path`

Auto-mode order becomes: command backend usable → use it; else HTTP configured
(`base_url` set) → use it; else binary present but no model → `MissingModel`;
else `MissingBinary`. A command template without a `{model}` placeholder does
not need a model and never reports `MissingModel`.

`voice.stt.model` accepts three forms:

1. a path (contains a separator, or ends in `.bin` and exists as given) — as today;
2. a catalog name (`small`) → `ggml-small.bin` looked up in the model dirs;
3. a bare file name (`ggml-small.bin`) → looked up in the model dirs.

Not found → `MissingConfiguredModel` (never a silent fallback to another model).

With no pin and several installed models, `findModel` picks the highest
catalog rank instead of the alphabetically first file; ties prefer the models
dir over the extra dirs, then the exact catalog file name, then alphabetical.
`Resolved.STT.ModelPath` stays the chosen absolute path.

## Installer (`internal/voice/install.go`)

```go
type InstallProgress struct {
    Name  string
    Done  int64
    Total int64 // 0 when the server did not say
}

type InstallOptions struct {
    Dir      string                 // models dir, created if missing
    Client   *http.Client           // nil → http.DefaultClient (env proxies honoured)
    Progress func(InstallProgress)  // called on the downloader goroutine, at most every 200 ms and once at the end
    URL      string                 // override for tests; empty → Model.URL
}

// Install downloads m into opts.Dir and returns the final path.
func Install(ctx context.Context, m Model, opts InstallOptions) (string, error)
```

Algorithm:

1. `final := Dir/m.File`, `part := final + ".part"`. If `final` exists and its
   first four bytes are the ggml magic `lmgg`, return it without a request.
2. `offset` = size of `part` (0 when absent). GET with `Range: bytes=<offset>-`
   when `offset > 0`.
3. `200` → truncate `part`, write from the start. `206` → append. `416` with
   `offset > 0` → treat the part as complete and go to verification. Anything
   else → `download failed: HTTP <status>` (keep `part`).
4. `Total` = `offset + Content-Length` for 206, `Content-Length` for 200,
   0 when unknown.
5. Stream to `part` with progress. `ctx` cancelled → keep `part`, return
   `download cancelled — /voice install resumes it` wrapping `ctx.Err()`.
   Body ends before `Total` → keep `part`, return
   `download interrupted at <done> of <total> — /voice install resumes it`.
6. Verify: size equals `Total` when known, magic is `lmgg`. Failure → remove
   `part`, return `downloaded file is not a ggml model — try /voice install again`.
7. `os.Rename(part, final)`.

Error strings are what the toast shows: short, no full URLs, no paths other
than the models dir. Only one install runs per cozyphi process.

## UI flow

### Ctrl+G with `Missing == MissingModel`

`VoiceStart` no longer calls `Session.Start` when the resolved STT is
`MissingModel`. It publishes a `controller.QuestionAskMsg` (the same overlay
the question tool uses) with one question:

- Header: `Voice`
- Question: `Speech model not installed. Download ggml-small (~466 MB) to ~/.cozyphi/models and set it up?`
  (models dir shown with `~` when under `$HOME`)
- Options:
  - `Download and set up` — `Whisper small: fast, fine for dialog. Bigger models: /voice install medium`
  - `Not now` — `/voice install later, or set voice.stt.model`

A goroutine waits on the reply channel and publishes `VoiceOfferReplyMsg{Accept bool}`
(an empty reply — Esc — is a decline). If another question overlay is already
open, skip the overlay and toast the `MissingModel` hint instead.

- Accept → `startInstall(DefaultModel)`.
- Decline → toast (warning, 6 s):
  `voice: no speech model — /voice install when ready, or set voice.stt.model`.

Ctrl+G while an install is running → toast `voice: downloading ggml-small — 42%`
(progress from the last `InstallProgress`). Ctrl+G with `MissingBinary` or
`MissingConfiguredModel` → the hint as today (error path, no offer).

### Download

`startInstall(name)`:

- toast (success, 4 s): `voice: downloading ggml-small (~466 MB) to ~/.cozyphi/models…`
- footer activity `ActivityDownloadingModel`, label `Downloading model…`; the
  footer renders progress after the label when known: `Downloading model… 42%`.
  To carry it, `SetActivityMsg` gains an optional `Detail string` that the
  activity handler stores and the footer appends; every existing sender leaves
  it empty.
- goroutine: `voice.Install(ctx, m, InstallOptions{Dir: env.ModelsDir, Progress: publish VoiceInstallProgressMsg})`,
  then `VoiceInstallDoneMsg{Name, Path, Err}`.
- `CloseVoice` cancels the context; `.part` stays for the next resume.

### Completion

On `VoiceInstallDoneMsg` without error, on the UI goroutine:

1. `cfg.STT.Model = name` (the catalog name, form 2 above), `resolved = voice.Resolve(cfg, env)`.
2. `Session.Reconfigure(cfg, resolved)` — new method, allowed only while the
   session is off/idle (nothing captured, nothing queued); otherwise it returns
   an error and the toast says `restart cozyphi to use it`.
3. `PersistModel(name)` (see below). Failure → warning toast
   `voice: ggml-small installed but config.yaml not updated — <err>`; the
   in-memory selection stays.
4. toast (success, 6 s): `voice: ggml-small installed — press Ctrl+G to talk`.
5. clear the activity.

With an error: clear the activity, toast (warning, 8 s) `voice: <err>`.

### `/voice install [name]`

- No name → `DefaultModel`. Unknown name → `voice: unknown model "<name>" — /voice models lists them`.
- Install running → `voice: still downloading ggml-<x> — 42%`.
- Model file already installed → no download: select and persist it, toast
  `voice: ggml-medium selected — press Ctrl+G to talk`.
- Otherwise `startInstall(name)`. The command itself is the consent.

### `/voice models`

One toast line (10 s), catalog order, `✓` for installed, `(active)` for the
resolved one:

    voice: models — tiny 75 MB · base 142 MB · small 466 MB ✓ (active) · medium 1.5 GB · large-v3 3.1 GB · large-v3-turbo 1.6 GB ✓ — /voice install <name>

### `/voice status`

Unchanged shape; with `MissingModel` it reads
`voice: not ready — no speech model installed — /voice install downloads ggml-small (~466 MB)`.

### Command surface

`voiceSubcommands = status, devices, retry, models, install`; usage line and
the command description follow. `install` completes catalog names as its
second argument.

## Host / editor / session seams

`commands.Host` gains:

```go
VoiceModels() []VoiceModelInfo   // Name, Size string, Installed, Active bool
VoiceInstall(name string) error  // validation errors are returned; progress goes through toasts/footer
```

`editor.VoiceOptions` gains `PersistModel func(name string) error`, wired in
`cmd/main.go` to `settingsManager.SetVoiceModel`. The editor keeps `Config`
and `Env` from `VoiceOptions` so it can re-resolve.

`harnesssettings.Manager.SetVoiceModel(ctx, name string) error` writes
`voice.stt.model: <name>` through `configfile.Edit` under the manager mutex,
preserving every other key (same shape as `setNotifications`).

`voice.Session.Reconfigure(cfg Config, resolved Resolved) error` swaps the
config, resolved and transcriber while idle/off.

Messages: `VoiceOfferReplyMsg`, `VoiceInstallProgressMsg`, `VoiceInstallDoneMsg`
in `internal/tui/controller/msg.go`; `ActivityDownloadingModel` next to the
other voice activities.

## Docs and changelog

- `doc/voice.md`: setup says "install ffmpeg and whisper-cpp, press Ctrl+G and
  accept the download" with the curl route kept as the manual alternative;
  document `/voice models`, `/voice install [name]`, the three `voice.stt.model`
  forms, the rank rule, the `.part` resume, and replace the troubleshooting
  row for the old hint with rows for the three new ones.
- `CHANGELOG.md`, top of `## [Unreleased]`:
  `- Voice: when whisper-cli is installed but no speech model is, Ctrl+G offers to download ggml-small and sets it up; /voice models and /voice install [name]; voice.stt.model accepts a model name; separate hints for a missing whisper-cli, a missing model and a wrong model path.`

## Tests

- `install_test.go` with `httptest.Server`: full download; resume from a
  `.part` (server sees `Range`, answers 206, file is byte-exact); `200` on a
  resume request restarts from zero; bad magic removes the part and errors;
  body shorter than `Content-Length` keeps the part and errors; cancelled
  context keeps the part; `404`; existing final file short-circuits without a
  request; progress callback throttled and final call has `Done == Total`.
- `config_test.go`: the three hints and `Missing` values in auto and explicit
  modes; `voice.stt.model` as catalog name, file name and path; rank selection
  with several installed files including a quantized variant; a command
  template without `{model}` needs no model.
- `models_test.go`: `LookupModel` forms, `FormatBytes`, `InstalledModels`
  mapping.
- `internal/tui/commands`: `/voice models` rendering, `/voice install` argument
  handling and completion, with the fake host.
- `harnesssettings`: `SetVoiceModel` writes the key and keeps the rest of the file.
- No test touches the network.

## Follow-ups (separate tasks)

- Pin the language after the first auto-detection in a session
  (`voice.language: auto` doubles the encoder work per segment).
- Optional persistent transcriber (`whisper-server`) to drop the per-segment
  model load.
- Core ML encoder variants on Apple silicon when the installed whisper-cli
  supports them.
