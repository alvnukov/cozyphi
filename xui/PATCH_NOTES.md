# Fork notes

Vendored copy of `github.com/pulseaiclub/xui` (base tag: **v0.1.3**, Apache-2.0).
Consumed via a filesystem `replace` directive in the root `go.mod`.

Local divergences from upstream v0.1.3:

1. `render`: `Renderer` keeps a cross-frame cursor cache. `RenderDiff` writes
   zero bytes when the frame has no dirty cells and the cursor state is
   unchanged; cursor sequences are emitted only on state changes, and
   hide/show bracketing happens only on frames that paint cells.
2. `xui`: `XUI.Render` skips `Screen.Present` when nothing was written.

To re-sync with upstream: copy the new version over this directory, then
re-apply the patches above (they are confined to `render/render.go` and the
`Render` method in `xui.go`; tests live next to them).
