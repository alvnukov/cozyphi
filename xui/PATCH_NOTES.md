# Fork notes

Vendored copy of `github.com/pulseaiclub/xui` (base tag: **v0.1.3**, Apache-2.0).
Consumed via a filesystem `replace` directive in the root `go.mod`.

Local divergences from upstream v0.1.3:

1. `render`: `Renderer` keeps a cross-frame cursor cache. `RenderDiff` writes
   zero bytes when the frame has no dirty cells and the cursor state is
   unchanged; cursor sequences are emitted only on state changes, and
   hide/show bracketing happens only on frames that paint cells.
2. `xui`: `XUI.Render` skips `Screen.Present` when nothing was written.
3. Repo formatters (`make fmt`) normalized comments/line breaks in
   `alias.go`, `input/parser.go`, and `term/tty_windows.go` — cosmetic only.
4. `term`: raw-mode reads are bounded by `VMIN=0/VTIME=1` (100 ms), and
   `unixTTY.Read` reads the raw descriptor so a VTIME expiry stays
   distinguishable from EOF. Upstream's `Interrupt()` relies on
   `SetReadDeadline`, but Go never registers `/dev/tty` with the runtime
   poller on darwin (deadline calls fail with `ErrNoDeadline`), so
   `Loop.Stop` hung forever on `wg.Wait` and Ctrl+C never exited the app.
5. `input`: the legacy Alt+key path (`parseESC` default branch) also maps
   control bytes after ESC — CR/LF to Enter, TAB to Tab, DEL/BS to
   Backspace — as the key with ModAlt. Upstream requires the byte after
   ESC to be printable, so ESC CR decodes as a lone Escape followed by a
   bare Enter, and a composer that submits on Enter fires on Alt+Enter.
   (`altControlKey` in `input/parser.go`; tests in `parser_test.go`.)
6. `input`: `KeyEvent` carries `Repeat`, set from kitty `event_type` 2, so
   consumers can tell an OS auto-repeat from a fresh press; upstream collapses
   both into `Press: true`. (`Repeat` in `input/event.go`, `parseModField` and
   `parseModsAndEvent` in `input/parser.go`; tests in `parser_test.go`.)
7. `input`: legacy C0 bytes 0x1c–0x1f decode as Ctrl+`\`, `]`, `^`, `_`
   (byte+0x40) instead of upstream's uniform byte+0x60, which named them
   Ctrl+`|`, `}`, `~` and DEL. Upstream's mapping is right only for the
   letters, so Ctrl+] used to arrive as Ctrl+} on a legacy terminal while the
   kitty protocol reported it as `]`, and a binding could match only one of
   the two. (`parseOne` in `input/parser.go`; test in `parser_test.go`.)
8. `render`: a frame that paints cells turns autowrap off (DECAWM `?7l`)
   for its writes and back on before the sync reset; `ExitAltScreenSeq` and
   the non-alt-screen `Close` path turn it on again. A whole row that holds a
   non-ASCII glyph is erased (`CSI K` with the reset pen) before it is
   repainted, and its trailing blanks are skipped. When the terminal draws a
   glyph wider than xui's width model (✅, ⚡), upstream's row repaint shifts
   the row's tail and autowrap spills the last cell into column 0 of the next
   row; when it draws one narrower, the old frame's last columns survive.
   Upstream hides this only when every frame repaints the whole screen.
   Follows ultraviolet's `repaintLine` / `putCellLR`. (`writeRow`,
   `mayDrift` in `render/render.go`; tests in `render/drift_test.go`.)

To re-sync with upstream: copy the new version over this directory, then
re-apply the patches above (1–2 and 8 are confined to `render/` and the
`Render` and `Close` methods in `xui.go`; 4 lives in `term/tty_unix.go`;
5–7 live in `input/parser.go` and `input/event.go`; tests live next to
them).
