package controller

import "github.com/ignaciotcrespo/gitshelf/internal/types"

// KeyBinding defines a single key action with its display metadata.
type KeyBinding struct {
	Key     string         // the key as matched by HandleKey (e.g. "c", "A", "space")
	Display string         // how to show the key in help text (e.g. "space", "h / ←")
	Short   string         // short label for the footer bar (e.g. "commit", "select")
	Desc    string         // full description for the help screen
	Panels  []types.PanelID // which panels this binding applies to (empty = universal)
}

// Panel-scoped binding groups. These are the single source of truth for all
// key-to-action mappings shown in the UI.  The actual key handling lives in
// keymap.go and must stay in sync — a test enforces this.

// Navigation bindings (universal, shown in the help screen only).
var NavBindings = []KeyBinding{
	{Key: "1", Display: "1", Short: "changelists", Desc: "Focus Changelists panel"},
	{Key: "2", Display: "2", Short: "shelves", Desc: "Focus Shelves panel"},
	{Key: "3", Display: "3", Short: "files", Desc: "Focus Files panel"},
	{Key: "4", Display: "4", Short: "diff", Desc: "Cycle Diff panel (normal → maximized → hidden)"},
	{Key: "5", Display: "5", Short: "log", Desc: "Cycle Git Log panel (normal → maximized → hidden)"},
	{Key: "6", Display: "6", Short: "worktrees", Desc: "Toggle Worktrees panel (normal ↔ minimized)"},
	{Key: "tab", Display: "tab / shift+tab", Short: "cycle", Desc: "Cycle between panels"},
	{Key: "up", Display: "j / ↑", Short: "up", Desc: "Move cursor up"},
	{Key: "down", Display: "k / ↓", Short: "down", Desc: "Move cursor down"},
	{Key: "enter", Display: "enter", Short: "enter", Desc: "Drill into files / diff"},
	{Key: "?", Display: "?", Short: "help", Desc: "Toggle this help screen"},
	{Key: "q", Display: "q", Short: "quit", Desc: "Quit"},
}

// CLBindings are keys available when the Changelists panel is focused.
var CLBindings = []KeyBinding{
	{Key: "n", Display: "n", Short: "new", Desc: "New changelist"},
	{Key: "r", Display: "r", Short: "rename", Desc: "Rename changelist"},
	{Key: "d", Display: "d", Short: "delete", Desc: "Delete changelist"},
	{Key: "s", Display: "s", Short: "shelve", Desc: "Shelve all files in changelist"},
	{Key: "c", Display: "c", Short: "commit", Desc: "Commit selected files"},
	{Key: "A", Display: "A", Short: "amend", Desc: "Amend last commit with selected files"},
	{Key: "p", Display: "p", Short: "push", Desc: "Push to remote"},
	{Key: "P", Display: "P", Short: "pull", Desc: "Pull from remote"},
	{Key: "B", Display: "B", Short: "accept", Desc: "Accept dirty changes as new baseline"},
	{Key: "W", Display: "W", Short: "copy", Desc: "Copy changelist to clipboard (for pasting in another worktree)"},
	{Key: "V", Display: "V", Short: "paste", Desc: "Paste changelist from clipboard"},
	{Key: "y", Display: "y", Short: "copy patch", Desc: "Copy changelist diff as patch to clipboard"},
}

// CLFileBindings are keys available in the Files panel under changelist context.
var CLFileBindings = []KeyBinding{
	{Key: " ", Display: "space", Short: "select", Desc: "Toggle file selection"},
	{Key: "a", Display: "a", Short: "all", Desc: "Select all files"},
	{Key: "x", Display: "x", Short: "none", Desc: "Deselect all files"},
	{Key: "c", Display: "c", Short: "commit", Desc: "Commit selected files"},
	{Key: "A", Display: "A", Short: "amend", Desc: "Amend last commit with selected files"},
	{Key: "s", Display: "s", Short: "shelve", Desc: "Shelve selected files"},
	{Key: "m", Display: "m", Short: "move", Desc: "Move file(s) to another changelist"},
	{Key: "p", Display: "p", Short: "push", Desc: "Push to remote"},
	{Key: "P", Display: "P", Short: "pull", Desc: "Pull from remote"},
	{Key: "B", Display: "B", Short: "accept", Desc: "Accept dirty changes as new baseline"},
	{Key: "y", Display: "y", Short: "copy patch", Desc: "Copy selected file(s) diff as patch to clipboard"},
}

// ShelfBindings are keys available when the Shelves panel is focused.
var ShelfBindings = []KeyBinding{
	{Key: "u", Display: "u", Short: "unshelve", Desc: "Unshelve (restore changes to working tree)"},
	{Key: "r", Display: "r", Short: "rename", Desc: "Rename shelf"},
	{Key: "d", Display: "d", Short: "drop", Desc: "Drop shelf"},
	{Key: "y", Display: "y", Short: "copy patch", Desc: "Copy shelf patch to clipboard"},
}

// ShelfFileBindings are keys available in the Files panel under shelf context.
var ShelfFileBindings = []KeyBinding{
	{Key: "u", Display: "u", Short: "unshelve", Desc: "Unshelve (restore changes to working tree)"},
	{Key: "d", Display: "d", Short: "drop", Desc: "Drop shelf"},
	{Key: "y", Display: "y", Short: "copy patch", Desc: "Copy selected file(s) diff as patch to clipboard"},
}

// DiffBindings are keys available when the Diff panel is focused.
var DiffBindings = []KeyBinding{
	{Key: "left", Display: "h / ←", Short: "hscroll", Desc: "Scroll left"},
	{Key: "right", Display: "l / →", Short: "hscroll", Desc: "Scroll right"},
	{Key: "w", Display: "w", Short: "wrap", Desc: "Toggle word wrap"},
	{Key: "y", Display: "y", Short: "copy patch", Desc: "Copy visible diff to clipboard"},
}

// LogBindings are keys available when the Log panel is focused.
var LogBindings = []KeyBinding{
	// Log panel only supports scroll (handled by navigation) and maximize/hide.
}

// WorktreeBindings are keys available when the Worktrees panel is focused.
var WorktreeBindings = []KeyBinding{
	// Worktrees panel is display-only for now.
}

// RemoteBindings are shown in the help screen as a separate section.
var RemoteBindings = []KeyBinding{
	{Key: "p", Display: "p", Short: "push", Desc: "Push to remote"},
	{Key: "P", Display: "P", Short: "pull", Desc: "Pull from remote"},
}

// FooterText builds " key short · key short · ..." from a binding list.
// Keys in the exclude set are omitted (e.g. to handle "B accept" specially).
func FooterText(bindings []KeyBinding, exclude map[string]bool) string {
	var parts []string
	for _, b := range bindings {
		if exclude != nil && exclude[b.Key] {
			continue
		}
		parts = append(parts, b.Display+" "+b.Short)
	}
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += " · "
		}
		result += p
	}
	return result
}

// HelpSection returns the data needed to render one section of the help screen.
type HelpSection struct {
	Heading string
	Keys    [][2]string // [display, description]
}

// HelpSections returns all sections for the full help screen.
func HelpSections() []HelpSection {
	toKeys := func(bindings []KeyBinding) [][2]string {
		keys := make([][2]string, len(bindings))
		for i, b := range bindings {
			keys[i] = [2]string{b.Display, b.Desc}
		}
		return keys
	}
	return []HelpSection{
		{"Navigation", toKeys(NavBindings)},
		{"Changelists (panel 1)", toKeys(CLBindings)},
		{"Files (panel 3)", toKeys(CLFileBindings)},
		{"Shelves (panel 2)", toKeys(ShelfBindings)},
		{"Diff (panel 4)", toKeys(DiffBindings)},
		{"Remote", toKeys(RemoteBindings)},
	}
}
