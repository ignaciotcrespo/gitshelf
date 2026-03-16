// Package types defines shared enums used across layers.
// This package has no dependencies on any other internal package,
// allowing both controller (logic) and ui (presentation) to import it
// without creating circular or upward dependencies.
package types

import "github.com/ignaciotcrespo/tui-framework"

// PanelID identifies a panel in the layout.
type PanelID = tui.PanelID

const (
	PanelChangelists PanelID = iota
	PanelShelves
	PanelFiles
	PanelDiff
	PanelLog
	PanelWorktrees
)

// PanelState represents the display state of a toggleable panel.
type PanelState = tui.PanelState

const (
	PanelNormal    = tui.PanelNormal
	PanelMaximized = tui.PanelMaximized
	PanelHidden    = tui.PanelHidden
	PanelMinimized = tui.PanelMinimized
)

// PromptMode identifies the current input prompt type.
type PromptMode = tui.PromptMode

const (
	PromptNone PromptMode = iota
	PromptNewChangelist
	PromptRenameChangelist
	PromptShelveFiles
	PromptRenameShelf
	PromptCommit
	PromptMoveFile
	PromptAmend
	PromptUnshelve
	PromptPush
	PromptPull
	PromptConfirm
	PromptPasteChangelist
)

// Paste mode options for copy-to-worktree clipboard.
const (
	PasteFullContent = "Full content"
	PasteApplyDiff   = "Apply diff"
	PasteOnlyCL      = "Only changelist"
)

// ConfirmAction identifies what dangerous action is pending confirmation.
type ConfirmAction = tui.ConfirmAction

const (
	ConfirmNone ConfirmAction = iota
	ConfirmDeleteChangelist
	ConfirmDropShelf
	ConfirmAcceptDirty
	ConfirmShelve
	ConfirmUnshelve
	ConfirmPasteFullContent
	ConfirmSnapshotUnshelve
)
