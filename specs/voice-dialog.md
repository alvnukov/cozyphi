# Voice Dialog Mode

Phase 2 of voice input. Phase 1 (`specs/voice-input.md`) gave the composer a
one-shot recording: press Ctrl+G, speak, stop, wait, text appears. This phase
turns the microphone into a *mode* the user stays in while talking to the
agent, with Space as the one control key and text arriving while they speak.

## Problem Statement

A dialog with the agent is many short utterances, not one dictation. With the
phase-1 loop every utterance costs a chord to start, a chord to stop, and a
wait during which nothing is visible. There is no way to fall silent for a
moment without ending the recording, no push-to-talk, and no feedback that
the words already said were understood. The user asked for:

- Ctrl+G turns the voice dialog mode on and the microphone starts at once;
  Ctrl+G again turns the mode off.
- Microphone on: a single Space press pauses listening; the next single press
  resumes. Holding Space pauses until it is released.
- Microphone paused: holding Space works as push-to-talk (listen while held,
  pause again on release).
- Every state must be clearly visible to the user.

Two facts of the platform shape the design:

- Key **releases** reach the app only on terminals speaking the kitty keyboard
  protocol (xui pushes `CSI >7u` when it detects `CapKittyKB`; a release is a
  `KeyEvent` with `Press=false`; auto-repeat arrives as another
  `Press=true`). `vx.Caps().KittyKeyboard` says whether releases will ever
  come. Terminal.app and tmux without passthrough never send them.
- While the mode is on, Space is a control key and cannot be typed. That is
  accepted: the mode is for talking, and Ctrl+G leaves it in one keystroke.

## Solution

### The mode

Ctrl+G (`keys.CmdVoice`) toggles **voice dialog mode**. Entering the mode
starts the microphone immediately (`Listening`). While in the mode the
composer keeps working as a text field, except that Space is reserved.

Leaving the mode:

- **Ctrl+G** ends the mode and keeps what was said: the open segment is
  closed and transcribed like any other, queued segments finish, then the
  microphone is released. The hint row shows `⋯ finishing…` until the last
  result lands.
- **Esc** ends the mode and discards: the open segment, the queue and any
  in-flight transcription are dropped; nothing more is inserted.
- The phase-1 one-shot behaviour, "Enter while recording stops without
  sending" and `voice.auto_send` are removed. There is one mode.

### The Space rule

One rule covers tap, hold-to-pause and push-to-talk:

- **Press** (the first `Press=true` after a release) flips the microphone at
  once: `Listening → Paused`, `Paused → Listening`.
- **Release** flips it back **only if** the key was held for at least
  `holdThreshold` (300 ms). A shorter press is a tap and the flip stands.
- **Repeat** (a `Press=true` while the composer still considers the key
  down) is ignored.

So in `Listening`, a tap pauses; holding pauses while held and resumes on
release. In `Paused`, a tap resumes; holding listens while held and pauses on
release, which is push-to-talk. The user never has to learn two rules.

Without releases (`HoldKeys=false`): every `Press=true` is a tap, taps closer
than `repeatGap` (250 ms) are treated as auto-repeat and ignored, and the hint
row never promises holding.

Space is a control key only when the composer would otherwise type it: not
while the slash, mention or palette pickers are open, and never with
modifiers (Shift+Space, Ctrl+Space and Alt+Space are passed through
unchanged, so they still insert a space or reach whatever owns them).

### Continuous listening

The capture process runs continuously while `Listening`. The session cuts the
stream into **segments**:

- A segment opens when speech is detected (chunk RMS above the speech
  threshold) and includes a short pre-roll of the silence before it.
- It closes after `segment_silence_ms` (800 ms) of trailing silence, or when
  it reaches `max_seconds` (30 s) of audio, whichever comes first. Trailing
  silence beyond a short tail is trimmed.
- Silence with no speech in it never produces a segment.

Closed segments go into a bounded FIFO queue and a **single worker**
transcribes them one at a time, in order. Each result is emitted as its own
`VoiceResultMsg` and inserted at the caret. The queue length is reported in
every state event (`Pending`) so the UI can show `⋯N`.

The transcriber prompt for a segment is the glossary hint followed by the
tail (last ~200 characters) of the previous successful result, so a sentence
split across two segments keeps its context. A failed segment shows one error
toast; the worker moves on to the next segment; `/voice retry` transcribes
the **last failed** segment again (its audio is kept in memory and at
`WAVPath` until a retry succeeds or the mode ends).

### Enter

Enter in the mode means "send what I said". It closes the open segment
(if any), waits until the queue is empty, then submits the composer as usual:

- While waiting the hint row shows `⋯ finishing… then send  Esc cancel` and
  the footer keeps `Listening…`. Typing stays possible.
- Esc during the wait cancels the pending submit only; the mode stays on and
  the queue keeps draining into the composer.
- A segment error during the wait cancels the pending submit (the user must
  see and fix the toast before sending).
- After submit the mode and `Listening` persist; the composer is empty and
  the next utterance starts a new prompt.
- Enter with nothing pending and an empty composer does nothing, as today.
- Enter while a picker is open still belongs to the picker.

### Pause

`Paused` keeps the capture process alive for `pauseGrace` (30 s), reading
and discarding audio, so push-to-talk and a quick resume are instant. After
the grace period the process is closed; the next resume restarts it and the
hint row shows `● starting…` until the first audio chunk arrives (the capture
start timeout of 3 s still applies; a failure is a toast and the mode stays
`Paused`).

`auto_pause_seconds` (300 s) of continuous silence in `Listening` moves the
session to `Paused` with a warning toast
`voice: paused after 5:00 of silence — Space resumes`.

### What the user sees

The composer's right hint row is the primary indicator; the placeholder and
the footer follow it. Glyphs: `●` listening, `‖` paused, `⋯` working.

| State | Hint row | Placeholder (empty composer) | Footer |
| --- | --- | --- | --- |
| Listening | `● ▃▅▆   Space pause · ^G done` | `Listening… speak, or type` | `Listening…` |
| Listening, key held (hold-to-pause armed) | `● ▃▅▆  talking  release to pause` | `Talking…` | `Listening…` |
| Paused | `‖ paused  Space resume · hold to talk · ^G done` | `Paused. Space to resume` | `Voice paused` |
| Paused, key held (push-to-talk) | `● ▃▅▆  talking  release to pause` | `Talking…` | `Listening…` |
| Paused, hold-to-pause in progress | `‖ paused  release to resume` | `Paused. Space to resume` | `Voice paused` |
| Starting the capture again | `● starting…` | `Listening… speak, or type` | `Listening…` |
| Segments queued | `● ▃▅▆  ⋯2  Space pause · ^G done` | as above | as above |
| Enter waiting | `⋯ finishing… then send  Esc cancel` | as above | `Listening…` |
| Leaving with Ctrl+G, queue not empty | `⋯ finishing…` | `Ask anything...` | `Transcribing…` |

Rules:

- `hold to talk` and `release to …` appear only when `HoldKeys` is true.
  Without releases the Paused row reads `‖ paused  Space resume · ^G done`.
- The meter (`▃▅▆`) is the phase-1 ramp; the elapsed timer is gone (a mode
  has no meaningful elapsed time). `⋯N` shows the number of queued segments
  including the one being transcribed, and is omitted when it is zero.
- Key hints are dropped first when the row does not fit: the composer
  measures the spans against the chat width and drops `Space … · ^G done`
  before anything else, because `paintHintsRow` does not truncate.
- The footer never overrides a running stream activity (phase-1 rule). A new
  activity `ActivityVoicePaused` with label `Voice paused` and no spinner
  joins `ActivityListening` and `ActivityTranscribing`; all three clear
  through `ClearIfActivityMsg`.
- No toast on entering or leaving the mode. Toasts are for errors, the
  silence auto-pause and the capture restart failure.
- `/voice status` reports the mode:
  `voice: dialog listening (2 queued), hold keys yes — capture ffmpeg on "default", transcriber whisper-cli (ggml-small.bin), language auto, segment 30s`
  and `voice: idle — …` when the mode is off. `hold keys no` is the honest
  answer on a terminal without releases.
- F1 catalog: `Ctrl+G` → `voice dialog on/off`; a second row
  `Space` → `pause or resume the microphone; hold to talk` under a "while
  voice dialog is on" note (a plain `Keys` row, not a `Cmd`).

## User Stories

- As a user I press Ctrl+G, start talking, and see my sentences appear in the
  composer one after another while I keep talking.
- As a user I tap Space to think, see `‖ paused`, tap Space and continue.
- As a user I hold Space while someone talks to me, see `release to pause`,
  and let go to continue.
- As a user in `Paused` I hold Space to say one thing, see `talking`, and let
  go; nothing else is recorded.
- As a user I press Enter mid-sentence; the composer says `finishing… then
  send`, my last words land, the prompt goes out, and I keep talking to the
  next prompt.
- As a user I press Ctrl+G to stop; the last thing I said still appears; the
  hint row goes back to normal.
- As a user on Terminal.app I see `Space resume · ^G done` and nothing about
  holding, and tapping works.
- As a user I walk away; five minutes later the composer says it paused and
  the microphone stops reading.
- As a contributor I can drive the whole state machine in tests with a
  scripted capture and a fake transcriber, without hardware.

## Implementation Decisions

### Package `internal/voice`

`Session` is redesigned around a mode rather than a recording. Public
surface (the phase-1 names that survive keep their meaning):

```go
type State int
const (
    StateIdle State = iota  // mode off
    StateListening          // capture running, segmenter active
    StatePaused             // mode on, audio discarded
    StateFinishing          // mode ending, queue draining, capture closed
)

type Event struct {
    Kind     EventKind      // EventState | EventResult | EventError | EventNotice
    Gen      int            // mode generation (increments on Start)
    Seq      int            // segment sequence within the mode (results, errors)
    State    State
    Level    float64        // meter, 0..1
    Pending  int            // segments queued + in flight
    Starting bool           // capture is being (re)started
    Text     string
    Language string
    Hint     string
}

func NewSession(opts Options, emit func(Event)) *Session
func (s *Session) Start(ctx context.Context)   // enter mode → Listening
func (s *Session) Pause()                      // Listening → Paused
func (s *Session) Resume(ctx context.Context)  // Paused → Listening (restarts capture after grace)
func (s *Session) Flush()                      // close the open segment now (Enter, Ctrl+G)
func (s *Session) End()                        // leave mode, keep speech: Flush + Finishing → Idle
func (s *Session) Discard()                    // leave mode, drop everything → Idle
func (s *Session) Retry(ctx context.Context)   // re-transcribe the last failed segment
func (s *Session) Close()                      // kill capture, drop queue (quit path)
func (s *Session) State() State
func (s *Session) Pending() int
func (s *Session) HasFailed() bool             // /voice retry precondition
func (s *Session) Status() string
```

- `Options` keeps `Config`, `Resolved`, `WAVPath`, `Capture`, `Transcriber`
  seams and adds `Clock`/timer seams only if the tests need them (prefer
  short real durations in tests over a fake clock).
- **Stream**: `Capture.Start` and `Stream` stay. Add a method
  `Drain() []int16` that returns and clears the samples accumulated since the
  previous call, so the session can consume audio incrementally without the
  buffer growing; `Samples()` keeps returning everything for phase-1 tests
  and `/voice devices`-style probes. `commandStream.maxSamples` becomes a
  ring bound (drop the oldest) sized from `max_seconds` plus pre-roll, not a
  hard stop, because the stream now lives for minutes.
- **Segmenter** (`segmenter.go`): a pure function object fed chunks from the
  ticker loop (every `meterInterval`, 100 ms, the session drains the stream
  and hands the chunk over). Parameters: speech threshold (`SpeechRMS =
  0.01`, distinct from `SilenceRMS` which stays the whole-buffer floor),
  pre-roll (300 ms), trailing silence (`segment_silence_ms`), max length
  (`max_seconds`). It returns a closed segment (`[]int16`) when one is ready.
  Silence-only audio never closes a segment. `Flush()` closes whatever is
  open if it contains speech; otherwise it drops it.
- **Worker**: one goroutine per mode generation, fed by a buffered channel
  (`segmentQueue = 8`). When the queue is full the oldest queued segment is
  dropped with an `EventNotice` (`voice: transcriber is behind, dropped one
  segment`), never blocking the capture loop. The worker transcribes with
  `Request{WAV, Language, Prompt: hint + tail(previous text)}`, emits
  `EventResult{Seq}` or `EventError{Seq}`, and decrements `Pending` through
  a state event. Results of an older `Gen` are dropped by the composer as
  today.
- **Pause**: sets a flag the ticker loop reads; audio drained while paused is
  discarded and the segmenter reset. A `pauseGrace` timer (30 s) closes the
  stream; `Resume` after that calls `Capture.Start` again and emits
  `Starting: true` until the first chunk arrives.
- **Auto-pause**: the segmenter reports continuous silence duration; when it
  exceeds `auto_pause_seconds` the session pauses and emits an
  `EventNotice` with the text above.
- **End**: `Flush`, close the capture, move to `StateFinishing`, let the
  worker drain, then `StateIdle` with a final state event. `Discard` cancels
  the worker context, drops the queue, closes the capture, emits `StateIdle`.
- **Retry**: keeps the last failed segment's WAV (memory + `WAVPath`, mode
  0600 as in phase 1) and re-queues it at the head of the queue. It is
  forgotten when the retry succeeds or the mode ends.
- Every event carries `Pending` and `Level` so the UI can render from the
  latest event alone.
- **Secrets**: nothing changes — the API key stays in the unexported field;
  errors pass through `redact` before they become events.

### Configuration

- `max_seconds`: now the **per-segment** limit. Default 30, range 5..120.
  The doc line changes from "auto-stop" to "longest single segment".
- `segment_silence_ms`: new, default 800, range 200..5000.
- `auto_pause_seconds`: new, default 300, range 30..3600.
- `auto_send`: **removed** from `Config`, `FileConfig`, `Defaults`,
  `DecodeConfig`, `String`, docs and the project config test. It shipped
  only in Unreleased, so no deprecation path is needed; an `auto_send:` key in
  a config file is a load-time error like any unknown key under `voice:`
  (check how the loader treats unknown keys; if it ignores them, add an
  explicit "voice.auto_send was removed; the dialog mode sends on Enter"
  error so the user learns the key is gone).
- `Config.String()` includes the two new fields.

### Composer (`internal/tui/composer`)

`VoiceController` becomes:

```go
type VoiceController interface {
    VoiceStart()
    VoicePause()
    VoiceResume()
    VoiceFlush()
    VoiceEnd()
    VoiceDiscard()
    VoiceHoldKeys() bool   // releases arrive (kitty keyboard protocol)
}
```

- State mirrored from events: `voiceState`, `voiceLevel`, `voicePending`,
  `voiceStarting`, plus local key state `spaceDown bool`,
  `spacePressedAt time.Time`, `lastSpaceTap time.Time`, and `voiceSubmitPending
  bool`.
- `Handle`: before the chat receives a key, and only when
  `voiceState != StateIdle` and no picker is open:
  - `Space` with `Mods == 0`, `Press=true`: if `spaceDown` → ignore
    (repeat). Else if `!HoldKeys` and `now-lastSpaceTap < repeatGap` → ignore.
    Else `spaceDown = true; spacePressedAt = now; lastSpaceTap = now`; flip.
  - `Space`, `Press=false`: if `!spaceDown` → ignore (release from another
    state, e.g. the key was pressed before the mode began). Else
    `spaceDown = false`; if `now-spacePressedAt >= holdThreshold` → flip
    back. Consume.
  - `Enter`, `Mods == 0`, `Press=true`: `VoiceFlush()`; if `Pending()` is
    zero and the composer has text → submit now; else `voiceSubmitPending =
    true`. Consume.
  - `Esc`, `Press=true`: if `voiceSubmitPending` → clear it; else
    `VoiceDiscard()`. Consume.
- `ApplyVoiceState`: update the mirror; when `voiceSubmitPending` and
  `Pending == 0` and the state is not `Finishing`-with-work → submit through
  `Chat.OnSubmit`. When the state becomes `Idle`, reset `spaceDown` and the
  hints.
- `ApplyVoiceResult`: `insertAtCaret` (phase-1 spacing rule), no auto-send.
- `ApplyVoiceError`: clear `voiceSubmitPending`; the toast is the editor's.
- `Ctrl+G` (`keys.CmdVoice`): `Idle` → `HideCompleters(); VoiceStart()`;
  otherwise `VoiceEnd()`.
- Placeholder: `applyPosture` keeps computing the base placeholder; a new
  `voicePlaceholder()` overrides it while the mode is on (table above), and
  `applyHints` sets both `Chat.HintsRight` and `Chat.Placeholder`.
- Hint fitting: `voiceHints(width int)` builds the spans, measures with
  `runewidth`-style width the chat already uses, and drops the key-hint
  span when `total > width-2`; the composer knows its width from `Layout`.
- Constants: `holdThreshold = 300 * time.Millisecond`, `repeatGap = 250 *
  time.Millisecond`, `previousTail = 200` runes.

### Editor (`internal/tui/editor`)

- `VoiceOptions` gains `HoldKeys bool`; `cmd/main.go` passes
  `vx.Caps().KittyKeyboard`.
- `Editor` implements the new `VoiceController`; `publishVoiceEvent` maps
  `Seq`, `Pending`, `Starting` into `VoiceStateMsg{Gen, State, Level,
  Pending, Starting}`, `VoiceResultMsg{Gen, Seq, Text, Language}`,
  `VoiceErrorMsg{Gen, Seq, Text, Hint}`, `VoiceNoticeMsg{Gen, Text}`.
- `applyVoiceState`: `Listening` → `ActivityListening`; `Paused` →
  `ActivityVoicePaused`; `Finishing` → `ActivityTranscribing`; `Idle` →
  clear all three through `ClearIfActivityMsg`.
- `VoiceStatus` appends `hold keys yes|no`.
- `VoiceRetry`: precondition `HasFailed()`; the "no recording" toast becomes
  `voice: nothing to retry — the last segments were transcribed`.
- `CloseVoice` unchanged (kills capture on every quit path).

### Commands, keys, docs

- `/voice status` output per the table; `/voice retry` semantics per above;
  `/voice devices` unchanged.
- `keys.go` catalog rows as described; `doc/tui.md`: routing table rows for
  the changed messages and the new activity, the "6. Voice input" section
  rewritten around the mode and the Space rule.
- `doc/voice.md`: "The loop" section rewritten (mode, Space rule, Enter, Esc,
  terminals without key releases, the `hold keys` line of `/voice status`),
  config block updated (`max_seconds: 30`, `segment_silence_ms`,
  `auto_pause_seconds`, `auto_send` gone), troubleshooting rows for the new
  notices.
- `CHANGELOG.md`: one bullet at the top of `## [Unreleased]`, above the
  phase-1 bullet, describing the dialog mode, the Space rule and the config
  change (mention that `auto_send` is gone and `max_seconds` is now the
  segment limit).

## Testing Decisions

No test starts ffmpeg, whisper-cli or a network call.

- `internal/voice`:
  - `segmenter_test.go`: table of PCM scripts (silence, speech, speech +
    silence, speech longer than max, two utterances) → expected segment
    boundaries; pre-roll and trailing-trim lengths; silence-only yields none.
  - `session_test.go` rewritten with a **scripted capture** (`scriptStream`
    that serves chunks from a slice on each `Drain`, then silence) and the
    existing `fakeTranscriber` extended with per-call delay and failure
    scripts: two utterances → two results in order with `Seq` 0 and 1;
    prompt of the second contains the tail of the first; `Pending` climbs to
    2 and returns to 0; a failure emits `EventError{Seq}` and the next
    segment still transcribes; `Retry` resends the failed audio and clears
    `HasFailed`; `Pause` discards audio and keeps the stream until grace
    (use a short grace through an unexported field or `Options` seam);
    `Resume` after grace restarts the capture and emits `Starting`; `End`
    drains then goes `Idle`; `Discard` drops queued results; auto-pause
    fires after the configured silence (short value in test); queue overflow
    drops the oldest with a notice; `Close` kills the capture.
  - `config_test.go`: new fields, ranges, `auto_send` removed.
- `internal/tui/composer` (`voice_test.go` rewritten, `fakeVoice` records the
  calls and reports `holdKeys`):
  - Ctrl+G starts; Ctrl+G again ends; Esc discards.
  - Tap in Listening → `Pause`; tap in Paused → `Resume`.
  - Hold in Listening (press, 350 ms, release) → `Pause` then `Resume`; a
    release after 100 ms does not flip back.
  - Hold in Paused → `Resume` then `Pause` (push-to-talk).
  - Repeat presses while down are ignored; a release with no prior press is
    ignored; with `holdKeys=false` two presses 100 ms apart count once.
  - Space with Shift/Ctrl/Alt or with a picker open reaches the chat.
  - Enter with pending segments sets the wait, shows `finishing… then send`,
    submits once `Pending` reaches 0; Esc cancels the wait and keeps the
    mode; an error cancels the wait.
  - Results insert in order at the caret; a stale `Gen` is dropped.
  - Hint row per state including `⋯N`, `starting…`, and the no-hold variant;
    key hints dropped at a narrow width; placeholders per state.
- `internal/tui/editor`: `applyVoiceState` maps states to activities and
  never overrides a running stream; `VoiceStatus` includes `hold keys`.
- `internal/tui/commands`: `/voice retry` precondition text.
- `internal/project`: config loading with the new keys; `auto_send`
  rejected.
- `make fmt-check lint test` passes.

## Out of Scope

- Typing a space while the mode is on (leave with Ctrl+G).
- Streaming partial results inside a segment.
- Voice activity detection beyond RMS thresholds (no VAD model).
- Session-derived vocabulary hints, LLM polish, question-overlay voice
  answers, native capture helper, Windows preset.
- A settings-pane toggle for the mode.

## Further Notes

- The Space rule is deliberately symmetric so hold-to-pause and push-to-talk
  are one code path; keep it that way when fixing bugs.
- The kitty keyboard protocol flag is read once at start; a terminal that
  gains or loses it mid-session is not handled.
- `segmentQueue`, `pauseGrace`, `holdThreshold`, `repeatGap`, `SpeechRMS` and
  the pre-roll are constants for now; promote to config only if users ask.
