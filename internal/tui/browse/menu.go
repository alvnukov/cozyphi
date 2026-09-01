package browse

// MenuItem is one row of a `.` action menu: a label naming the command
// with the chord that runs it directly ("Move step down (Alt+↓)"), and
// the command itself. The menu is a choice list the pane renders like any
// other; the kit fixes the item shape so every pane's menu reads the
// same, and the pane runs Run only after leaving the menu — the command
// must act in the mode the menu came from, exactly as its chord would.
type MenuItem struct {
	Label string
	Run   func()
}
