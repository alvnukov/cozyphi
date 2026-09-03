# Voice Input

## Problem Statement

Long, conversational task descriptions are the input cozyphi is best at, and
they are exactly what people dislike typing. The OS dictation that exists today
(macOS, GNOME) does not know the project vocabulary (`worktree`, `goreleaser`,
`xui`, file names), lands text in whatever widget has focus, and has no idea
that Enter in the composer sends a prompt. Every third-party path ends with the
user re-reading a mangled transcript and fixing it by hand.

The feature must be an input method, not a new agent mode: speech becomes text,
text lands in the composer at the caret as ordinary editable input, and the user
sends it with Enter as always. Nothing reaches the model on its own.

Constraints that shape the design:

- Release binaries are built with `CGO_ENABLED=0` (`.goreleaser.yaml`), so no
  CGo audio library. Capture goes through an external process.
- Secrets never appear in bus messages, logs, toasts or error text
  (`internal/redact`).
- The TUI is demand-driven (`xui`): goroutines talk to the UI only through
  `controller.Bus.Publish`, stale async results are dropped by generation.
- Existing precedents to mirror, not reinvent: the `notifications` config
  section (feature package owns `DecodeConfig` with defaults, loader and
  settings manager both call it), `@`-mention async search in the composer
  (generation counter + `MentionResultsMsg`), `internal/proc.Start` for
  long-lived subprocess stdout, `internal/clipboard` for GOOS dispatch with an
  injected runner.

## Solution

One rebindable chord toggles recording in the composer. While recording, the
composer shows a timer and a microphone level meter in its right hint area and
the footer reports the activity. Pressing the chord again or Enter stops the
recording and starts transcription; Esc cancels and discards the audio. The
transcript is inserted at the caret as one block; the text already typed and
the caret position outside the insertion are left alone. The prompt is never
sent automatically unless `voice.auto_send` is explicitly enabled, and even
then only when the composer was empty before dictation.

Audio capture is an external command that writes raw PCM (s16le, 16 kHz, mono)
to stdout. cozyphi ships presets for `ffmpeg` on darwin (`avfoundation`) and
linux (`pulse`, falling back to `alsa`), and accepts a user command for
anything else. Go reads the stream, computes the RMS level for the meter,
buffers the samples in memory, and writes a WAV file for the transcriber.

Speech-to-text sits behind one `Transcriber` interface with two backends:
`command` runs a local CLI (`whisper-cli` from whisper-cpp by default) on the
WAV file; `http` posts the WAV as multipart to an OpenAI-compatible
`/audio/transcriptions` endpoint (OpenAI, Groq, a local whisper-server). The
`auto` backend prefers local when the binary and a model file are present and
uses `http` only when `base_url` and a key are configured explicitly. If
nothing is usable, the chord shows one sentence saying what to install or
configure and the feature stays available for the next attempt.

A `/voice` slash command shows the resolved status (backend, model, device,
chord), lists capture devices (`/voice devices`) and retries the last
recording (`/voice retry`).

The feature ships in phases. This spec fixes phase 1; phases 2 and 3 are listed
under Further Notes so that phase-1 interfaces leave room for them.

## User Stories

- As a user I press Ctrl+G, describe a task in my own words for a minute, press
  Enter, read the transcript in the composer, fix two words and press Enter to
  send. At no point did anything leave the composer without me.
- As a user I press Ctrl+G by accident and press Esc; nothing is transcribed,
  nothing is inserted, no toast nags me.
- As a user on a machine without ffmpeg I press Ctrl+G and see one line:
  `voice: no capture command found — install ffmpeg or set voice.capture.command`.
  The composer keeps working; the next press repeats the hint, it does not go
  silent.
- As a user with whisper-cpp installed and a model downloaded I get local
  transcription with no configuration at all.
- As a user with only a Groq or OpenAI key I set `voice.stt.base_url`,
  `voice.stt.model` and `voice.stt.api_key` (or `voice.stt.provider: openai` to
  reuse a stored credential) and get cloud transcription. My key never shows up
  in an error message, a toast or a log line.
- As a user whose transcript came back wrong because the request timed out I
  run `/voice retry` and the same recording is transcribed again without
  re-recording.
- As a user I speak Russian with English identifiers mixed in and the
  transcript keeps `cozyphi`, `worktree`, `goreleaser` spelled correctly
  because they are in `voice.glossary`.
- As a user I rebind the chord to F5 in `keybinds:` and the footer hint, F1
  help and `/voice` status all show F5.
- As a user I leave a recording running and walk away; it stops itself at
  `max_seconds` and tells me why, and the audio is still transcribed.
- As a user I record thirty seconds of silence; cozyphi tells me it heard
  nothing instead of sending an empty file to the transcriber.

## Implementation Decisions

### Package layout

- `internal/voice` — everything that does not know about the TUI:
  - `config.go`: `Config`, `DecodeConfig(raw FileConfig) (Config, error)` with
    defaults, validation and the resolved-backend logic. `FileConfig` is the
    YAML mirror (pointer fields, `yaml` tags). This is the single decoder the
    project loader and the settings manager call.
  - `capture.go`: `Capture` interface (`Start(ctx, device) (Stream, error)`),
    `Stream` (`Level() float64`, `Samples() []int16` snapshot, `Duration()`,
    `Stop() ([]int16, error)`), the command-backed implementation over
    `proc.Start`, presets per GOOS, device listing (`ListDevices`).
  - `wav.go`: `EncodeWAV(samples []int16, rate int) []byte`, `RMS`, silence
    detection (whole-buffer RMS below a fixed threshold, no VAD).
  - `stt.go`: `Transcriber` interface
    `Transcribe(ctx, Request) (Result, error)` with
    `Request{WAV []byte, Language string, Prompt string}` and
    `Result{Text string, Language string}`.
  - `stt_command.go`: placeholder expansion (`{file}`, `{lang}`, `{hint}`,
    `{model}`), runs via `proc.Run` with a timeout, reads stdout, strips
    whisper-cli timestamps when `-nt` is absent (defensive), trims.
  - `stt_http.go`: multipart POST to `{base_url}/audio/transcriptions` with
    `file`, `model`, `language` (omitted for `auto`), `prompt`,
    `response_format=json`; uses `util.DefaultHTTPClient()` and
    `util.DoWithRetry` where the existing helpers fit; the Authorization
    header is set from a resolved key that lives only inside the transcriber
    value; error bodies pass through `redact` before becoming an error string.
  - `session.go`: `Session` state machine (Idle → Recording → Transcribing →
    Idle) with a generation counter, `Start`, `Stop`, `Cancel`, `Retry`,
    `LastWAV` persistence in `~/.cozyphi/voice/last.wav` (0600, directory
    0700) kept until a successful transcription of that recording, and the
    `max_seconds` auto-stop. `Session` is goroutine-safe and reports results
    through a callback `func(Event)` so the TUI adapter can wrap them in bus
    messages. It never imports the TUI.
- `internal/tui/controller/msg.go`: new messages
  `VoiceStateMsg{Gen int, State voice.State, Elapsed time.Duration, Level float64}`,
  `VoiceResultMsg{Gen int, Text string, Language string}`,
  `VoiceErrorMsg{Gen int, Text string, Hint string}`. None of them carries
  config or credentials. `doc/tui.md` routing table gets a row per message.
- `internal/tui/composer/pane.go`: owns the `*voice.Session` handle through a
  small `voiceHost` adapter, handles the chord and Enter/Esc while recording,
  renders the meter in `HintsRight`, inserts the result via
  `ReplaceRange(caret, caret, text)`, drops results whose `Gen` is stale.
- `internal/tui/keys`: `CmdVoice Command = "voice"` in `table.go` with default
  `Ctrl+G`; catalog row in `keys.go` under `ScopeComposer`
  (`{Keys: ["Ctrl+G"], Desc: "start or stop voice input"}`); a
  `ScopeVoice`-style transient row is not needed, the recording hints stay in
  the composer's `HintsRight`.
- `internal/tui/commands/builtins.go`: `voice` command with subcommands
  `status` (default), `devices`, `retry`. `Host` gains
  `VoiceStatus() string`, `VoiceDevices() ([]string, error)`, `VoiceRetry()`.
- `internal/project/config.go`: `Voice voice.Config` on `Config`,
  `Voice *voice.FileConfig` on `fileConfig`, default from
  `voice.DecodeConfig(voice.FileConfig{})`, decoding next to notifications.
  `keybinds.voice` works through the existing `keys` override path with no new
  code beyond the table entry.
- `internal/harnesssettings`: no new checkbox in phase 1. The manager must
  keep the `voice` section intact when it rewrites config.yaml
  (`configfile.Edit` already preserves unknown keys; add a test that proves it
  for `voice`).

### Configuration

```yaml
voice:
  enabled: true
  language: auto          # ru | en | auto (passed to the backend as-is)
  auto_send: false        # send the prompt after insertion when the composer was empty
  max_seconds: 300        # auto-stop; 10..1800
  hints: glossary         # glossary | off (phase 2 adds `session`)
  glossary: [cozyphi, worktree, goreleaser, xui]
  capture:
    command: auto         # auto | a command line emitting s16le 16 kHz mono PCM on stdout
    device: default       # preset device name/index; passed to the preset only
  stt:
    backend: auto         # auto | command | http
    command: "whisper-cli -m {model} -l {lang} --prompt {hint} -nt -f {file}"
    model: ""             # command: model file path; http: model name
    base_url: ""          # http only, e.g. https://api.groq.com/openai/v1
    api_key: ""           # http only; or `provider: openai` to reuse a stored credential
    provider: ""
    timeout_seconds: 60
keybinds:
  voice: Ctrl+G
```

Resolution rules in `DecodeConfig`:

- `backend: auto` → `command` when the command's binary is on PATH (via
  `LookBin`) and a model file resolves (explicit `stt.model`, else the first
  `ggml-*.bin` under `~/.cozyphi/models`, else whisper-cpp's default model
  directory when it exists); otherwise `http` when `base_url` and a key
  (inline or via provider) are both present; otherwise `unconfigured` with a
  one-line `Hint` explaining the shortest fix. Resolution that touches the
  filesystem lives in a `Resolve(cfg, lookBin) Resolved` step so
  `DecodeConfig` stays pure and testable.
- `capture.command: auto` → ffmpeg preset for the GOOS when ffmpeg is on PATH;
  otherwise `unconfigured` with a hint. A user command is used verbatim
  (`{device}` placeholder supported).
- Unknown enum values are errors at load time, matching notifications.
- `api_key` is stored in `Config` as a `redact.Secret`-style opaque value if
  such a type exists; otherwise as a private field with no `String` output.
  It is copied into the transcriber at construction and nowhere else.

### Capture

- Presets:
  - darwin: `ffmpeg -hide_banner -loglevel error -f avfoundation -i ":{device}" -ac 1 -ar 16000 -f s16le -` where `default` maps to `:default` (device index or name otherwise).
  - linux: `ffmpeg -hide_banner -loglevel error -f pulse -i {device} -ac 1 -ar 16000 -f s16le -`; `alsa` when pulse is absent (`default` → `default`).
  - windows: unconfigured in phase 1 with a hint to set `capture.command`.
- `proc.Start` with the recording context; stderr captured with a small limit
  for the error message; the process is killed on `Stop`, `Cancel`, context
  cancellation and on editor shutdown (the composer's `Close` path).
- A start timeout (3 s) fires if no bytes arrive: the error text names the
  device and suggests `/voice devices`; on darwin it also mentions microphone
  permission for the terminal app.
- Level: RMS of each 100 ms window, smoothed, exposed as 0..1 for the meter;
  the meter is a short bar (`▁▂▃▅▆▇`) plus `mm:ss`, updated by `ctx.WakeIn`
  ticks of 100 ms while recording.
- Buffer cap equals `max_seconds` of samples; hitting it stops the recording
  with an explanatory toast and proceeds to transcription.
- Whole-buffer silence (RMS below threshold for the entire recording) is
  reported as `voice: heard only silence — check the input device` and is not
  transcribed; the WAV is still kept for `/voice retry`.

### Transcription

- Prompt hint: `strings.Join(glossary, ", ")` when `hints: glossary`; empty
  when `off`. Passed as `{hint}` (shell-quoted) or the multipart `prompt`
  field.
- Language `auto` → `-l auto` for whisper-cli; omitted for http.
- Timeouts: `stt.timeout_seconds` per request, one retry on transport errors
  for http, none for command.
- Text normalisation: trim, collapse internal runs of whitespace to one space,
  keep sentence punctuation as returned; when inserting at a caret that
  follows a non-space character, prefix one space; when the caret precedes a
  non-space character, suffix one space.
- Insertion is a single `ReplaceRange`, so one Ctrl+Z (if the composer has
  undo) or a manual delete removes it as one block.
- `auto_send` sends only when the composer was empty before recording began
  and the transcript is non-empty; otherwise the text is inserted and left for
  the user.

### State machine and key handling

- States: `Idle`, `Recording`, `Transcribing`. A generation increments on every
  `Start`; results and errors carrying an older generation are ignored.
- Chord in `Idle` → `Start`; in `Recording` → `Stop`; in `Transcribing` →
  toast `voice: still transcribing…` and no state change.
- Enter in `Recording` → `Stop` (the prompt is not sent). Enter in
  `Transcribing` sends what is already in the composer as usual; the result
  arriving later is inserted at the caret (the generation is still current).
- Esc in `Recording` or `Transcribing` → `Cancel`, meter cleared, no toast.
- Other keys keep working while recording; typing during recording is allowed.
- Starting a recording while a mention picker is open closes the picker
  first.
- `Session.Start` when the feature is `unconfigured` or `enabled: false`
  publishes `VoiceErrorMsg` with the hint and stays in `Idle`.
- Footer: `ActivityListening` and `ActivityTranscribing` join
  `controller.Activity` with labels `Listening…` and `Transcribing…`, cleared
  by the same `ClearIfActivityMsg` path as the others, and they do not
  override a running stream activity (a stream in progress wins; the meter in
  the composer is the primary indicator).

### Error text

Every error is one line, English, starts with `voice:`, names the failed step
and the next action, and passes through `redact.String` before it becomes a
toast or log line. Examples:

- `voice: no capture command found — install ffmpeg or set voice.capture.command`
- `voice: no transcriber configured — install whisper-cpp and a ggml model, or set voice.stt.base_url and api_key`
- `voice: capture produced no audio from "default" — try /voice devices`
- `voice: transcription failed (HTTP 401) — check voice.stt.api_key; /voice retry keeps the recording`
- `voice: recording stopped at 5:00 (voice.max_seconds)`

### Documentation and changelog

- `doc/voice.md`: user-facing guide (setup on macOS and Linux, config
  reference, `/voice`, troubleshooting, privacy note about `last.wav` and
  cloud backends). Add it to the design docs list in `AGENTS.md`.
- `doc/tui.md`: routing table rows for the three new messages and the two new
  activities.
- `CHANGELOG.md`: one bullet at the top of `## [Unreleased]`.

## Testing Decisions

- `internal/voice`:
  - `DecodeConfig` table tests: defaults, every enum, range checks, error on
    unknown values; `Resolve` with a fake `lookBin` and a temp model dir.
  - Capture over a fake command (`sh -c` printing a fixed PCM byte pattern, or
    a Go test helper binary via `os.Args[0]` re-exec) verifying level,
    duration, stop, start timeout, kill on cancel.
  - WAV encoder golden bytes; RMS and silence threshold.
  - Command transcriber with a fake script that echoes its arguments to prove
    placeholder expansion and quoting; timeout path.
  - HTTP transcriber against `httptest.Server`: multipart fields, headers,
    `language` omitted for `auto`, `prompt` present, error body redacted, key
    never in the error string (assert with the literal key).
  - Session: state transitions, generation drop, max_seconds auto-stop,
    silence path, `last.wav` mode 0600 and removal after success, retry.
- `internal/tui/composer`: with the existing `fakeBus` and `newTestPane`, a
  fake `voice.Session` (interface `voiceController` in the composer) covering
  chord toggle, Enter-stops-not-sends, Esc cancels, insertion at caret with
  spacing rules, stale generation ignored, auto_send only when empty before.
- `internal/tui/commands`: `/voice`, `/voice devices`, `/voice retry` dispatch
  to the host.
- `internal/project`: loading the `voice` section, `keybinds.voice` override,
  defaults when the section is absent; settings manager preserves `voice` on
  rewrite.
- No test starts ffmpeg, whisper-cli or a network call; hardware paths are
  covered manually per the task's verification plan.

## Out of Scope

- Session-derived vocabulary hints (recent prompts and tool paths).
- Incremental transcription in chunks split on silence.
- Push-to-talk on key release (kitty keyboard protocol).
- Microphone permission diagnosis beyond the hint text.
- Settings pane checkbox; `/voice setup` model download.
- LLM post-processing of the transcript, voice answers to the question
  overlay, native macOS helper, Windows capture preset.

## Further Notes

- Phase 2: `hints: session` using `Engine.memoryQuery`-style recent prompts
  and `memory.pathish` identifiers, capped to a few hundred characters;
  chunked transcription on silence so long dictations appear progressively;
  PTT on kitty terminals reusing the same `Session`; mic permission check on
  darwin; settings checkbox; `/voice setup` that downloads a ggml model into
  `~/.cozyphi/models`.
- Phase 3: optional LLM clean-up step (`voice.polish: true`) that removes
  fillers and fixes identifiers using the glossary; `auto_send` refinements;
  question-overlay voice answers; native macOS helper binary for capture
  without ffmpeg.
- The `Transcriber` and `Capture` interfaces are the extension points for all
  of the above; nothing in phase 1 should assume a single recording per
  session or a single result per recording.
