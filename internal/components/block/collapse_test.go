package block

import "testing"

// TestCollapseOnClickFoldsOnlyExpandedBodies: a clean click folds an
// expanded block exactly once, reports the fold through OnToggle, and
// leaves collapsed or bodyless blocks alone.
func TestCollapseOnClickFoldsOnlyExpandedBodies(t *testing.T) {
	var toggled []bool
	onToggle := func(expanded bool) { toggled = append(toggled, expanded) }

	cases := []struct {
		name  string
		block interface{ CollapseOnClick() bool }
		want  bool
	}{
		{"expanded bash", &BashBlock{Command: "ls", Output: "a\nb", Expanded: true, OnToggle: onToggle}, true},
		{"bash without output", &BashBlock{Command: "ls", Expanded: true, OnToggle: onToggle}, false},
		{"expanded tool", &ToolBlock{Name: "read", Output: "12 lines", Expanded: true, OnToggle: onToggle}, true},
		{
			"expanded diff",
			&DiffBlock{Name: "edit", Diff: "@@ -1 +1 @@\n-a\n+b", Expanded: true, OnToggle: onToggle},
			true,
		},
		{"expanded agent", &AgentBlock{Name: "agent", Summary: "done", Expanded: true, OnToggle: onToggle}, true},
		{"expanded thinking", &ThinkingBlock{Text: "hmm", Expanded: true, OnToggle: onToggle}, true},
		{
			"expanded compaction",
			&CompactionBlock{Summary: "the story so far", Expanded: true, OnToggle: onToggle},
			true,
		},
		{"collapsed tool", &ToolBlock{Name: "read", Output: "12 lines", OnToggle: onToggle}, false},
	}
	for _, tc := range cases {
		toggled = nil
		if got := tc.block.CollapseOnClick(); got != tc.want {
			t.Fatalf("%s: CollapseOnClick() = %v, want %v", tc.name, got, tc.want)
		}
		if !tc.want {
			if len(toggled) != 0 {
				t.Fatalf("%s: OnToggle fired %v on a no-op", tc.name, toggled)
			}
			continue
		}
		if len(toggled) != 1 || toggled[0] {
			t.Fatalf("%s: OnToggle got %v, want one collapse", tc.name, toggled)
		}
		if tc.block.CollapseOnClick() {
			t.Fatalf("%s: a second click folded an already collapsed block", tc.name)
		}
	}
}
