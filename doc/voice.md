# Voice Input

Voice input is an input method, not an agent mode. Speech becomes text, the
text lands in the composer at the caret as ordinary editable input, and you
send it with Enter as always. Nothing reaches the model on its own.

| Audience | This document |
| --- | --- |
| Users | Setup on macOS and Linux, the chord, `/voice`, troubleshooting |
| Contributors | Config reference, backend resolution, where the audio lives |

---

## The loop

Press **Ctrl+G** in the composer. The right hint area turns into a level meter
with a timer (`● ▁▂▃▅   00:07`) and the footer says `Listening…`.

- **Ctrl+G again** or **Enter** stops the recording and starts transcription.
  Enter while recording does *not* send the prompt — it only stops the mic.
- **Esc** cancels: the audio is discarded, the meter clears, no toast.
- Typing while recording keeps working; the transcript is inserted wherever the
  caret ends up.

The footer shows `Transcribing…` until the text arrives. The transcript is
inserted as one block at the caret, with a single space added on either side
when it would otherwise collide with a word. A model run in progress keeps the
footer for itself — the composer meter is the primary indicator.

The prompt is **never** sent automatically unless `voice.auto_send: true` *and*
the composer was empty when recording started.

## Setup

Voice needs two things: a **capture** command that writes raw audio to stdout,
and a **transcriber**. Both resolve automatically when the usual tools are
installed.

### macOS

```sh
brew install ffmpeg whisper-cpp
# a model — small is a good default at ~500 MB
mkdir -p ~/.cozyphi/models
curl -L -o ~/.cozyphi/models/ggml-small.bin \
  https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin
```

The first recording asks for microphone access for your terminal application.
If you never see the prompt, grant it by hand in System Settings → Privacy &
Security → Microphone; a terminal without permission records silence.

### Linux

```sh
sudo apt install ffmpeg          # or your package manager's equivalent
# whisper-cpp: distro package, or build from https://github.com/ggml-org/whisper.cpp
mkdir -p ~/.cozyphi/models
curl -L -o ~/.cozyphi/models/ggml-small.bin \
  https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin
```

Capture uses PulseAudio when `pactl` is on PATH and ALSA otherwise.

### Windows

There is no capture preset. Set `voice.capture.command` to a command line that
writes s16le 16 kHz mono PCM to stdout.

### Without a local model

Any OpenAI-compatible `/audio/transcriptions` endpoint works — OpenAI, Groq, or
a local `whisper-server`:

```yaml
voice:
  stt:
    base_url: https://api.groq.com/openai/v1
    api_key: gsk_…
    model: whisper-large-v3-turbo
```

## `/voice`

| Command | What it does |
| --- | --- |
| `/voice` or `/voice status` | One line: state, capture command, device, transcriber, language, `max_seconds` |
| `/voice devices` | Lists the microphones the capture backend can see, in the spelling `capture.device` expects |
| `/voice retry` | Transcribes the last recording again, without re-recording |

`/voice retry` is what a failed transcription leaves you: the recording is kept
on disk until a transcription of it succeeds.

## Configuration

Everything lives under `voice:` in `~/.cozyphi/config.yaml`. The section is
optional — these are the defaults.

```yaml
voice:
  enabled: true
  language: auto          # ru | en | auto — passed to the backend as-is
  auto_send: false        # send after insertion, but only if the composer was empty
  max_seconds: 300        # auto-stop, 10..1800
  hints: glossary         # glossary | off
  glossary: [cozyphi, worktree, goreleaser, xui]
  capture:
    command: auto         # auto, or a command line emitting s16le 16 kHz mono PCM on stdout
    device: default       # device name or index for the preset; {device} for a custom command
  stt:
    backend: auto         # auto | command | http
    command: "whisper-cli -m {model} -l {lang} --prompt {hint} -nt -f {file}"
    model: ""             # command: path to a ggml-*.bin; http: the model name
    base_url: ""          # http only, e.g. https://api.groq.com/openai/v1
    api_key: ""           # http only
    timeout_seconds: 60   # one transcription request, 1..3600
keybinds:
  voice: Ctrl+G
```

Unknown enum values and out-of-range numbers are load-time errors, so a typo in
`voice:` fails the start with the line to fix rather than silently disabling the
microphone.

### How `auto` resolves

- **Capture** — the ffmpeg preset for this OS when `ffmpeg` is on PATH:
  `avfoundation` on macOS, `pulse` (or `alsa`) on Linux. A custom
  `capture.command` is used verbatim, with `{device}` expanded.
- **Transcriber** — `command` when the binary in `stt.command` is on PATH *and*
  a model resolves; `http` when both `base_url` and `api_key` are set;
  otherwise voice stays unconfigured and `/voice status` says what to install.
- **Model** — `stt.model` when it points at an existing file, else the first
  `ggml-*.bin` in `~/.cozyphi/models`, else in whisper-cpp's packaged
  directories (`/opt/homebrew/share/whisper-cpp`, `/usr/local/share/whisper-cpp`,
  `/usr/share/whisper-cpp`).

### Placeholders

`stt.command` is expanded before it runs:

| Placeholder | Becomes |
| --- | --- |
| `{file}` | The WAV file for this recording |
| `{model}` | The resolved model path |
| `{lang}` | `voice.language` |
| `{hint}` | The glossary, comma-joined — empty when `hints: off` |

`capture.command` supports `{device}`.

### Glossary

`hints: glossary` sends `glossary` to the transcriber as a vocabulary prompt,
which is how `cozyphi`, `worktree` and `goreleaser` come back spelled correctly
in the middle of a Russian sentence. Add your own project's identifiers. Set
`hints: off` to send nothing.

## Troubleshooting

Every failure is one line starting with `voice:`, naming the step that failed
and the next thing to do.

| Line | What to do |
| --- | --- |
| `no capture command found — install ffmpeg or set voice.capture.command` | Install ffmpeg, or point `capture.command` at your own recorder |
| `no transcriber configured — install whisper-cpp and a ggml model, or set voice.stt.base_url and api_key` | Download a model into `~/.cozyphi/models`, or configure the HTTP backend |
| `capture produced no audio from "default" — try /voice devices` | Wrong device, or the terminal has no microphone permission (macOS) |
| `heard only silence — check the input device with /voice devices` | The recording was below the silence threshold; the audio is kept for `/voice retry` |
| `recording stopped at 5:00 (voice.max_seconds)` | The auto-stop fired; the audio is still transcribed |
| `transcription failed (HTTP 401)` | Check `voice.stt.api_key`; `/voice retry` reuses the recording |
| `voice: off (set voice.enabled: true)` | The section is disabled in config |

`/voice status` is the fastest way to see which half is missing. `/voice
devices` prints the device spellings `capture.device` accepts — on macOS the
index alone (`0`) is enough.

## Privacy

- The last recording is written to `~/.cozyphi/voice/last.wav` (file `0600`,
  directory `0700`) so `/voice retry` can resend it. It is **deleted as soon as
  a transcription of it succeeds**; it survives only a failure, and only until
  the next recording replaces it. It never enters a project directory and is
  never added to a session transcript.
- With the local `command` backend, the audio never leaves the machine.
- With the `http` backend, the WAV is uploaded to the endpoint you configured
  and is subject to that provider's retention policy. `base_url` is yours to
  choose — a local `whisper-server` keeps the audio on the machine while still
  using the HTTP path.
- `voice.stt.api_key` lives in an unexported field, is copied once into the
  transcriber and appears in no bus message, toast, log line or error string.
  A provider that echoes the key back in an error body has it redacted before
  the text is shown.
