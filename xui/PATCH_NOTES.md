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

To re-sync with upstream: copy the new version over this directory, then
re-apply the patches above (1–2 are confined to `render/render.go` and the
`Render` method in `xui.go`; 4 lives in `term/tty_unix.go`; tests live next
to them).
