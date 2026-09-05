# Status dashboard

`/status` (also the command palette's **status dashboard**) opens an opaque,
full-screen dashboard. `/usage` and `/settings` remain available independently.

- **F2 / F3, Left / Right, Tab / Shift+Tab**: previous / next dashboard tab.
- **Up / Down, j / k, PgUp / PgDn, Home / End, mouse wheel**: scroll.
- **Esc**: close from every tab.
- **Config** is a read-only allowlisted settings snapshot taken at open. It has
  no settings editor, draft, paste handler, save action or persistence capability.
  `/settings` remains the independent editor. Config shows the effective session
  model/provider and controller mode, sub-agent enablement and task access;
  selected harness scalars and safe source paths come from the existing Store.
  Configured agent pins explicitly say effective unavailable: unavailable pins
  may fall back, and existing agents retain their model. Automatic compaction
  defaults and winning sources also say effective/source unavailable rather than
  claiming resolution. No raw config, token, notification sound name, URLs or
  environment values are displayed.
- **Usage** reuses subscription quota and live-session usage. `r` refreshes.
  Changed model/provider invalidates the previous view and asks for refresh.
- **Stats** has Overview / Models subnavigation (`m`); `p` selects All time /
  Last 7 days / Last 30 days and `r` reloads. Overview renders a horizontal
  calendar: week columns, seven weekday rows, month labels and Mon/Wed/Fri
  guides. Muted empty cells and three increasing orange intensities represent
  rounds relative to the busiest visible day; Less / More shows the scale.
  Empty weeks retain their columns. The window ends in the current UTC week,
  is capped at 53 weeks, shrinks to the recent weeks that fit the terminal, and
  discloses its dates and week count. Future days are blank. Geometry never wraps.
  The period selector and coverage caveats appear before the calendar. Loading
  and unavailable history show no grid. Dates outside the selected period use
  `·`; partial-history cells without observed rounds use `?`, not a known zero.
  Narrow screens stack prose when needed. Overview schedules a redraw at the next
  UTC midnight for calendar/current streak changes, without I/O. The selected
  cutoff stays fixed at the request date until refresh; recorded data is not live.
- Below the calendar, Overview pairs favorite model / total tokens, sessions /
  longest session, active days / longest recorded streak, and most active date /
  current streak in two columns at 76+ cells; narrower terminals stack each pair.
  Token breakdown follows. Favorite and most-active date use recorded rounds;
  lexical model name / earliest UTC date break ties. Unknown is not a favorite.
  Current streak counts consecutive UTC recorded days ending today or yesterday,
  bounded by the selected period, not inferred activity. Longest session is
  unavailable: the history API has no duration. Models shows real per-model
  round and token aggregates. Empty, unknown, period-bounded and partial data
  remain explicit; missing observations never become fabricated activity.

## Data boundaries

Status uses allowlisted session/version/directory/model/provider metadata, MCP
names and connection states, and configuration paths/source modes. It never
prints raw configuration, API URLs, credentials, environment values or MCP error
payloads. Account identity is explicitly unavailable. Cost and cache-write
counts are unavailable, not estimated as zero. The dashboard starts no I/O from
Draw; quota/history refresh happen through event callbacks. Config does not save.

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
The pane copies input slices and owns no history storage. The editor selects and
copies settings display rows immediately at ShowStatus, never retaining the
Store's potentially aliased AgentModels map for Draw.
