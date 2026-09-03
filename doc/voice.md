# Voice Input

Voice input is an input method, not an agent mode. Speech becomes text, the
text lands in the composer at the caret as ordinary editable input, and you
send it with Enter as always. Nothing reaches the model on its own.

| Audience | This document |
| --- | --- |
| Users | Setup on macOS and Linux, the dialog mode, `/voice`, troubleshooting |
| Contributors | Config reference, backend resolution, where the audio lives |

---

## The loop

Press **Ctrl+G** in the composer to turn **voice dialog mode** on. The
microphone opens at once, the right hint area becomes a level meter
(`● ▃▅▆   Space pause · ^G done`) and the footer says `Listening…`.

Then talk. Each time you fall silent for `segment_silence_ms` (0.8 s) what you
just said is cut off as a **segment**, transcribed on its own and inserted at
the caret while you keep talking. `⋯2` in the hint row is the number of
segments still waiting for the transcriber.

**Space is the one control key while the mode is on**, and a single rule
covers every way of pressing it:

- A **tap** flips the microphone: listening → paused, paused → listening.
- **Holding** it flips back on release. Holding while listening is "wait, I am
  being talked to"; holding while paused is push-to-talk.

The other keys:

| Key | What it does |
| --- | --- |
| **Enter** | Sends the message. The open segment is closed first, so the last thing you said still lands — the hint row shows `⋯ finishing… then send` until it does. The mode stays on, and the next sentence starts the next prompt. |
| **Ctrl+G** | Leaves the mode keeping what was said: the queue drains (`⋯ finishing…`) and the text lands before the microphone is released. |
| **Esc** | Leaves the mode and discards the open segment, the queue and anything in flight. While a send is waiting, the first Esc cancels only that send and the mode stays on. |

Typing keeps working throughout, and a transcript is inserted wherever the
caret ends up, with a single space added on either side when it would
otherwise collide with a word. A model run in progress keeps the footer for
itself — the composer hint row is the primary indicator.

Nothing is ever sent to the model on its own: Enter sends. Falling silent for
`auto_pause_seconds` (5 minutes) pauses the mode by itself and says so.

### Terminals without key releases

Holding needs the kitty keyboard protocol, which is the only way a key
*release* reaches the app. Terminal.app and tmux without passthrough never
send one. There, every press is a tap, the hint row never mentions holding
(`‖ paused  Space resume · ^G done`), and `/voice status` says `hold keys no`.

Space is a control key only where the composer would otherwise type it: a
picker (`/`, `@`, the palette) keeps its own Space, and `Shift+Space` still
types one.

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

The first time the microphone opens, macOS asks for access for your terminal
application.
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
| `/voice` or `/voice status` | One line: the mode and its queue, whether the terminal reports key releases, capture command, device, transcriber, language, segment limit |
| `/voice devices` | Lists the microphones the capture backend can see, in the spelling `capture.device` expects |
| `/voice retry` | Transcribes the last failed segment again, without re-recording |

```
voice: dialog listening (2 queued), hold keys yes — capture ffmpeg on "default", transcriber whisper-cli (ggml-small.bin), language auto, segment 30s
```

`/voice retry` is what a failed transcription leaves you: that segment's audio
is kept until a transcription of it succeeds or the mode ends. With nothing
left behind it says `nothing to retry`.

## Configuration

Everything lives under `voice:` in `~/.cozyphi/config.yaml`. The section is
optional — these are the defaults.

```yaml
voice:
  enabled: true
  language: auto            # ru | en | auto — passed to the backend as-is
  max_seconds: 30           # longest single segment, 5..120
  segment_silence_ms: 800   # trailing silence that closes a segment, 200..5000
  auto_pause_seconds: 300   # silence before the mode pauses itself, 30..3600
  hints: glossary           # glossary | off
  glossary: [cozyphi, worktree, goreleaser, xui]
  capture:
    command: auto           # auto, or a command line emitting s16le 16 kHz mono PCM on stdout
    device: default         # device name or index for the preset; {device} for a custom command
  stt:
    backend: auto           # auto | command | http
    command: "whisper-cli -m {model} -l {lang} --prompt {hint} -nt -f {file}"
    model: ""               # command: path to a ggml-*.bin; http: the model name
    base_url: ""            # http only, e.g. https://api.groq.com/openai/v1
    api_key: ""             # http only
    timeout_seconds: 60     # one transcription request, 1..3600
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
| `{file}` | The WAV file for this segment |
| `{model}` | The resolved model path |
| `{lang}` | `voice.language` |
| `{hint}` | The glossary, comma-joined, followed by the tail of the previous segment — empty when `hints: off` and there is no previous segment |

`capture.command` supports `{device}`.

### Glossary

`hints: glossary` sends `glossary` to the transcriber as a vocabulary prompt,
which is how `cozyphi`, `worktree` and `goreleaser` come back spelled correctly
in the middle of a Russian sentence. Add your own project's identifiers. Set
`hints: off` to send nothing.

The tail of the previous segment's transcript is appended to the hint, so a
sentence split across two segments keeps its context.

## Troubleshooting

Every failure is one line starting with `voice:`, naming the step that failed
and the next thing to do.

| Line | What to do |
| --- | --- |
| `no capture command found — install ffmpeg or set voice.capture.command` | Install ffmpeg, or point `capture.command` at your own recorder |
| `no transcriber configured — install whisper-cpp and a ggml model, or set voice.stt.base_url and api_key` | Download a model into `~/.cozyphi/models`, or configure the HTTP backend |
| `capture produced no audio from "default" — try /voice devices` | Wrong device, or the terminal has no microphone permission (macOS) |
| `paused after 5:00 of silence — Space resumes` | The mode heard nothing for `auto_pause_seconds`; tap Space to listen again |
| `… — Space retries the microphone` | The capture command would not restart after a pause; the mode stays paused |
| `transcriber is behind, dropped one segment` | Speech is arriving faster than the transcriber finishes it — a smaller model, or the HTTP backend, keeps up better |
| `transcription failed (HTTP 401)` | Check `voice.stt.api_key`; `/voice retry` transcribes that segment again |
| `voice: off (set voice.enabled: true)` | The section is disabled in config |

`/voice status` is the fastest way to see which half is missing. `/voice
devices` prints the device spellings `capture.device` accepts — on macOS the
index alone (`0`) is enough.

## Privacy

- Each segment is written to `~/.cozyphi/voice/last.wav` (file `0600`,
  directory `0700`) so `/voice retry` can resend it. It is **deleted as soon as
  a transcription of it succeeds**; it survives only a failure, and only until
  the next segment replaces it. It never enters a project directory and is
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
