// Package tui provides a reusable framework for building panel-based terminal UIs with Bubbletea.
package tui

// PanelID identifies a panel in the layout.
type PanelID = int

// PanelState represents the display state of a toggleable panel.
type PanelState = int

const (
	PanelNormal    PanelState = 0
	PanelMaximized PanelState = 1
	PanelHidden    PanelState = 2
	PanelMinimized PanelState = 3
)

// PromptMode identifies the current input prompt type.
type PromptMode = int

// ConfirmAction identifies what dangerous action is pending confirmation.
type ConfirmAction = int

// RefreshFlag tells the coordinator what data to reload.
type RefreshFlag int

const (
	RefreshNone       RefreshFlag = 0
	RefreshDiff       RefreshFlag = 1 << iota
	RefreshCLFiles                // reload changelist files (implies diff)
	RefreshShelfFiles             // reload shelf files (implies diff)
	RefreshAll                    // reload everything
	RefreshWorktree               // debounced worktree switch + reload
)
