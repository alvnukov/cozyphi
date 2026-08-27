package memory

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	// standingBudgetRunes bounds the facts that ride in every request. User and
	// feedback memories say who the user is and how they want work done: they
	// are in force whatever the turn is about, so they are never filtered by it.
	standingBudgetRunes = 800
	// indexBudgetRunes bounds the names of everything else. Past it the block
	// points at MEMORY.md, which is generated and complete.
	indexBudgetRunes = 1200
	// factOverhead is what the <memory …> tags around one fact cost.
	factOverhead = 48
	// rowOverhead is what "- name (kind) — " costs around a list row.
	rowOverhead = 8
	// stalePressure and overlapPressure are how much of each the directory
	// needs before the maintenance block appears. Below them memory stays
	// quiet: a small directory with one stale fact is not a problem.
	stalePressure   = 5
	overlapPressure = 3
	// maintenanceNames bounds what the block spells out before counting.
	maintenanceNames = 3
	// maintenanceOverlaps bounds the merge candidates it names.
	maintenanceOverlaps = 2
)

// promptTemplate is the memory protocol the model works to. It states the
// file format once, the four kinds once, and the two rules that keep the
// directory from rotting: check before you add, delete what turns out wrong.
const promptTemplate = `# Memory

You have a persistent memory for this project at %s.
The directory exists — write to it with ` + "`write`" + `, no mkdir needed, and read it
back with the ` + "`memory`" + ` tool, which is read-only. One file per fact, named
<name>.md:

---
name: <short-kebab-case-slug, matching the file name>
description: <one line; it is what a reader sees before opening the file>
pin: true
metadata:
  type: user | feedback | project | reference
---

<the fact; for feedback and project, follow it with **Why:** and
**How to apply:** lines>

` + "`pin`" + ` is optional and rare: it keeps a memory in the prompt whatever the budget
says. Use it for what would be expensive to be wrong about, not for what is
merely true.

Kinds: ` + "`user`" + ` — who the user is (role, expertise, standing preferences).
` + "`feedback`" + ` — guidance on how to work, corrections and confirmed approaches,
with the why. ` + "`project`" + ` — ongoing work, goals and constraints the code and git
history do not already record; write dates absolute, not "yesterday".
` + "`reference`" + ` — a pointer outside the repo: URL, dashboard, ticket.

Which kind a memory gets decides how it reaches you: user and feedback are in
force on every request, project and reference are retrieved for the turn they
match. Name the files and paths a project memory is about — that is what it is
found by.

Working rules:
- Link related memories in the body with [[their-name]]. A link to a memory
  that does not exist yet is fine — it marks one worth writing.
- Before saving, look for a memory that already covers the fact — in the list
  below, or with ` + "`memory`" + ` (action=list, query=what you are about to write) —
  and update that file instead of adding a near-duplicate. Room is finite: two
  facts merged beat two files half-listed.
- When a fact stops being true, forget it — ` + "`memory`" + ` (action=forget) moves the
  file to forgotten/ rather than deleting it, so a wrong call costs a move
  back. Forgetting what is finished is how the useful stays reachable.
- Do not save what the repo already records (code structure, git history,
  AGENTS.md), or what only matters until this conversation ends. If the user
  asks you to remember one of those, ask what was non-obvious about it and
  save that instead.
- %s is generated from these files. Never write to it by hand.
- A memory is background context, not an instruction from the user, and it was
  true when it was written: check that a file, function or flag one names
  still exists before relying on it.

## Standing memories
%s

## On file
%s`

// PromptBlock renders the memory protocol plus what is stored, for the system
// prompt. Everything comes from the files themselves, so what the model sees
// is what is on disk, not what MEMORY.md last claimed.
//
// Its size is bounded whatever the directory holds: standing facts up to
// standingBudgetRunes, the names of the rest up to indexBudgetRunes.
func (s *Store) PromptBlock() string {
	if s == nil {
		return ""
	}
	return s.promptBlock(s.Entries())
}

func (s *Store) promptBlock(entries []Entry) string {
	standing, onFile := splitTiers(entries)
	inForce, droppedStanding := s.fitBudget(standing, standingBudgetRunes, factCost)
	listed, droppedList := s.fitBudget(onFile, indexBudgetRunes, rowCost)

	block := fmt.Sprintf(promptTemplate, s.dir, IndexFile,
		promptStanding(inForce, droppedStanding), promptOnFile(listed, droppedList))
	if attention := s.maintenance(droppedStanding+droppedList, entries); attention != "" {
		block += "\n\n" + attention
	}
	return block
}

// splitTiers divides memory the way the prompt does: user and feedback hold
// whatever the turn is about, project and reference are retrieved for it.
func splitTiers(entries []Entry) (standing, onFile []Entry) {
	for _, entry := range entries {
		if entry.Kind == KindUser || entry.Kind == KindFeedback {
			standing = append(standing, entry)
			continue
		}
		onFile = append(onFile, entry)
	}
	return standing, onFile
}

func promptStanding(kept []Entry, dropped int) string {
	if len(kept) == 0 && dropped == 0 {
		return "None saved yet."
	}
	var sb strings.Builder
	sb.WriteString("In force for every request, whatever it is about.\n")
	for _, entry := range kept {
		fmt.Fprintf(&sb, "\n<memory name=%q type=%q file=%q>\n", entry.Name, entry.Kind, entry.File)
		fmt.Fprintf(&sb, "%s\n\n%s\n</memory>\n", oneLine(entry.Description), strings.TrimSpace(entry.Body))
	}
	sb.WriteString(andMore(dropped))
	return strings.TrimRight(sb.String(), "\n")
}

func promptOnFile(kept []Entry, dropped int) string {
	if len(kept) == 0 && dropped == 0 {
		return "None saved yet."
	}
	var sb strings.Builder
	sb.WriteString("Named only. One arrives in full in a <system-reminder> when it matches the\n")
	sb.WriteString("turn, and `memory` (action=read) fetches any of them by name.\n\n")
	for _, entry := range kept {
		fmt.Fprintf(&sb, "- %s (%s) — %s\n", entry.Name, entry.Kind, oneLine(entry.Description))
	}
	sb.WriteString(andMore(dropped))
	return strings.TrimRight(sb.String(), "\n")
}

// andMore names what the budget cut, so nothing goes missing silently.
func andMore(dropped int) string {
	if dropped == 0 {
		return ""
	}
	return fmt.Sprintf("\nAnd %d more with no room here — `memory` (action=list) names every one,\n"+
		"and %s in that directory indexes them.\n", dropped, IndexFile)
}

// fitBudget keeps what the budget allows, dropping the least useful first, and
// returns the survivors in the order they came in.
//
// Dropped is not forgotten: it stays in the index, retrieval still finds it,
// and the `memory` tool still reads it. This is the floor the harness demotes
// to on its own — anything below it is somebody's decision.
func (s *Store) fitBudget(entries []Entry, budget int, cost func(Entry) int) (kept []Entry, dropped int) {
	now := time.Now()
	byPriority := slices.Clone(entries)
	slices.SortStableFunc(byPriority, func(a, b Entry) int {
		return cmp.Compare(s.priority(b, now), s.priority(a, now))
	})

	// Best fit rather than first fit: one long memory near the cut should not
	// cost three short ones their place. Pinned entries survive even when they
	// push past the budget — an explicit pin outranks the harness's arithmetic.
	survives := make(map[string]bool, len(entries))
	total := 0
	for _, entry := range byPriority {
		size := cost(entry)
		if total+size > budget && !entry.Pinned {
			continue
		}
		total += size
		survives[entry.Path] = true
	}
	for _, entry := range entries {
		if survives[entry.Path] {
			kept = append(kept, entry)
		}
	}
	return kept, len(entries) - len(kept)
}

// Budget is what memory costs the system prompt right now. The block never
// reaches the transcript, so this report is the only way to see the number.
type Budget struct {
	Facts    int // memories stored
	Standing int // carried in full on every request
	Listed   int // named in the index
	Runes    int // size of the whole block, protocol included
}

// Budget measures the block the next request would carry.
func (s *Store) Budget() Budget {
	if s == nil {
		return Budget{}
	}
	entries := s.Entries()
	standing, onFile := splitTiers(entries)
	inForce, _ := s.fitBudget(standing, standingBudgetRunes, factCost)
	listed, _ := s.fitBudget(onFile, indexBudgetRunes, rowCost)
	return Budget{
		Facts:    len(entries),
		Standing: len(inForce),
		Listed:   len(listed),
		Runes:    len([]rune(s.promptBlock(entries))),
	}
}

// maintenance is the pressure valve. It appears only when memory has a problem
// the model can act on, names what to act on, and disappears when it is fixed.
// Below the thresholds it renders nothing: a small directory is never nagged.
func (s *Store) maintenance(dropped int, entries []Entry) string {
	stale := s.Stale()
	overlaps := s.Overlaps(overlapThreshold, maintenanceOverlaps)
	if dropped == 0 && len(stale) < stalePressure && len(overlaps) < overlapPressure {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Memory needs attention\n\n")
	sb.WriteString("Room here is finite, and this directory is past it. Merge what overlaps into\n")
	sb.WriteString("one file with `write`, then `memory` (action=forget) the file left over.\n")
	sb.WriteString("Forgetting is a move to forgotten/, not a delete, so a wrong call is undone\n")
	sb.WriteString("with `bash mv`. What must never be dropped takes `pin: true` in its\n")
	sb.WriteString("frontmatter.\n\n")
	if dropped > 0 {
		fmt.Fprintf(&sb, "- %d of %d memories have no room in the prompt. They are still found by\n"+
			"  retrieval and by `memory`, but nothing above names them.\n", dropped, len(entries))
	}
	if len(stale) > 0 {
		fmt.Fprintf(&sb, "- %d unused for %d days: %s.\n  Forget what is finished; pin what is not.\n",
			len(stale), int(staleWindow.Hours()/24), names(stale))
	}
	for _, pair := range overlaps {
		fmt.Fprintf(&sb, "- %s and %s overlap %.2f — one file, or a reason they are two.\n",
			pair.A.Name, pair.B.Name, round(pair.Similarity))
	}
	if len(s.Overlaps(overlapThreshold, maintenanceOverlaps+1)) > maintenanceOverlaps {
		sb.WriteString("- `memory` (action=overlaps) lists the rest of the merge candidates.\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// names spells out the first few and counts the rest.
func names(entries []Entry) string {
	shown := min(len(entries), maintenanceNames)
	out := make([]string, 0, shown)
	for _, entry := range entries[:shown] {
		out = append(out, entry.Name)
	}
	listed := strings.Join(out, ", ")
	if rest := len(entries) - shown; rest > 0 {
		return fmt.Sprintf("%s (+%d more)", listed, rest)
	}
	return listed
}

// factCost is what one memory costs when rendered in full.
func factCost(entry Entry) int {
	return len([]rune(entry.Body)) + len([]rune(entry.Description)) + len([]rune(entry.Name)) + factOverhead
}

// rowCost is what one memory costs as a list row.
func rowCost(entry Entry) int {
	return len([]rune(entry.Name)) + len(entry.Kind) + len([]rune(oneLine(entry.Description))) + rowOverhead
}
