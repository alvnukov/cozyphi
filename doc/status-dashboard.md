# Status dashboard

`/status` (also the command palette's **status dashboard**) opens an opaque,
full-screen dashboard. `/usage` and `/settings` remain available independently.

- **F2 / F3**: previous / next dashboard tab. Outside Config, Left / Right and
  Tab / Shift+Tab also switch tabs.
- **Up / Down, j / k, PgUp / PgDn, Home / End, mouse wheel**: scroll read-only
  pages. Text reflows when the terminal becomes narrow.
- **Esc**: close. In Config, an active search or picker closes first.
- **Config** embeds the existing settings editor. Its Tab key changes settings
  sections; `/` searches the active section, Enter edits, and Ctrl+S validates
  and saves through the same Store as `/settings`. Switching dashboard tabs
  retains the draft. A save failure retains it too; a successful save closes.
- **Usage** reuses the subscription quota and live-session usage presentation.
  `r` refreshes. A changed model/provider invalidates the previous view and asks
  for refresh, rather than showing another provider's quota.
- **Stats**: `p` cycles All time / 7d / 30d, `m` toggles Overview / Models, and
  `r` reloads. Calendar-week heatmap cells encode recorded rounds relative to
  the busiest day in the selected snapshot; empty weeks are omitted, with dates
  showing the gaps. Partial observations and unknown metadata remain explicit.
  Overview summarizes recorded activity in the selected period: distinct known
  UTC days with rounds, the longest consecutive active-day streak (not a current
  streak), and the known model with the most rounds (lexical name breaks ties).
  Duplicate days count once; missing dates never create activity. With no known
  model having rounds, the favorite is unavailable; `unknown` is never a favorite.

## Data boundaries

Status uses allowlisted session/version/directory/model/provider metadata, MCP
names and connection states, and configuration paths/source modes. It never
prints raw configuration, API URLs, credentials, environment values or MCP error
payloads. Account identity is explicitly unavailable. Cost and cache-write
counts are unavailable, not estimated as zero. The dashboard starts no I/O from
Draw; settings save and quota/history refresh happen through event callbacks.

History covers persisted journals for the current working directory, not all
projects or every worktree of a repository. Scans never repair journals. Limits
are 4096 directory entries, 200000 lines, 256 MiB total and 8 MiB per line;
incomplete observations are marked partial. Concurrent appends are not an atomic
snapshot, and unflushed live usage is not part of historical totals.

## Assembly seams

Stable tab IDs are `status`, `config`, `usage`, `stats`. The greatest persisted
close count wins; `usage` wins when tied for that maximum and is the default.
Remaining ties follow Status / Config / Stats order. Merely visiting a tab does
not increment its count. `cmd` creates a `controller.StatusHistory` with the session
loader and current working directory, calls `editor.ConfigureStatusDashboard`,
and defers the loader's `Close` on every application exit.

The editor connects `ConfigureStatusTabs` to controller preference methods.
Opening calls `LoadUIState`; actual closing calls `MutateUIState` and
`RecordStatusClose` exactly once, preserving sibling preferences. Load/save
failures surface as warnings; malformed preference files are never overwritten.
Neither the pane nor editor reads or writes preference files.

`ConfigureStatusHistory` starts asynchronous `session.HistoryStats` scans.
Each period change, refresh, or reopening cancels the preceding request and
advances a generation. Closing cancels and invalidates queued results; application
exit cancels and joins the workers. The Bus delivers detached results and wakes
the UI; `Editor.Update` checks visibility and generation before calling
`ApplyStatusHistory`. No background worker mutates widgets.

Seven- and thirty-day periods include today plus the preceding six or 29 UTC
calendar days. All time uses a zero cutoff. A load failure displays a safe retry
label rather than raw filesystem errors or fixture data.

The presentation snapshot contains `Totals{Sessions, Rounds, Input, Output,
Cached, Total}`, `Days []Day{Date time.Time, Totals}`, and
`Models []Model{Name, Totals}`. Map persisted history's UTC day strings to dates;
unknown dates stay out of heatmap cells. `Partial`, `UnknownUsage`,
`UnknownModels`, `UnknownDates` and display-safe `Warnings` preserve uncertainty.
Set `Unavailable` to an actionable display-safe explanation on load failure.
The pane copies input slices and owns no history storage.
