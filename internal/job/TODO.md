# internal/job — status

S0–S4 done for Harness Task 006 MVP:

- recovery / store errors / timed_out
- EngineRunner child Loop
- agent_spawn / task / wait / list / log / cancel
- TUI footer shows live job count
- automated S4 acceptance tests in `internal/tools/agent_s4_test.go`

## Optional later

- Richer TUI job panel (per-job progress beyond footer count)
- Cascade-cancel on Esc (product choice; currently wait-only)
