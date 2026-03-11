package controller

import (
	"github.com/ignaciotcrespo/gitshelf/internal/types"
	"github.com/ignaciotcrespo/gitshelf/pkg/tui"
)

// State holds all navigational/selection state. No data, no stores.
type State struct {
	Focus  types.PanelID
	Pivot  types.PanelID

	CLSelected    int
	CLFileSel     int
	SelectedFiles map[string]bool

	ShelfSel     int
	ShelfFileSel int

	DiffScroll  int
	DiffHScroll int
	DiffWrap    bool
	LogScroll   int

	DiffState types.PanelState
	LogState  types.PanelState

	ShowHelp   bool
	HelpScroll int

	MoveFile string
}

// NewState creates an initial state.
func NewState() State {
	return State{
		Focus:         types.PanelChangelists,
		Pivot:         types.PanelChangelists,
		SelectedFiles: make(map[string]bool),
		DiffState:     types.PanelNormal,
		LogState:      types.PanelNormal,
	}
}

// RefreshFlag tells the coordinator what data to reload.
type RefreshFlag = tui.RefreshFlag

const (
	RefreshNone       = tui.RefreshNone
	RefreshDiff       = tui.RefreshDiff
	RefreshCLFiles    = tui.RefreshCLFiles
	RefreshShelfFiles = tui.RefreshShelfFiles
	RefreshAll        = tui.RefreshAll
)

// KeyContext is a read-only snapshot of data the controller needs for decisions.
type KeyContext struct {
	CLCount         int
	CLFileCount     int
	CLNames         []string
	CLFiles         []string
	ShelfCount      int
	ShelfNames      []string
	ShelfDirs       []string // PatchDir paths for each shelf
	ShelfFileCount  int
	SelectedCount   int
	ActiveCL        string
	UnversionedName string
	DefaultName     string
	LastCommitMsg   string
	Remotes         []string
	DirtyFiles      map[string]bool
	DirtyCLs        map[string]bool
	TabFlow         []types.PanelID // panels for tab cycling
}

// KeyResult is the output of HandleKey.
type KeyResult struct {
	State       State
	Refresh     RefreshFlag
	StartPrompt *PromptReq
	RunRemote   *RemoteReq // immediate push/pull (no prompt needed)
	SetActive   string     // changelist to mark active (empty = no change)
	CopyPatch   CopyPatchReq // request to copy a patch to clipboard
	OpenURL     string     // URL to open in browser
	StatusMsg   string
	ErrorMsg    string
	Quit        bool
}

// CopyPatchReq describes what patch content to copy to clipboard.
type CopyPatchReq struct {
	Source CopyPatchSource
}

// CopyPatchSource identifies the origin of the patch to copy.
type CopyPatchSource int

const (
	CopyPatchNone       CopyPatchSource = iota
	CopyPatchChangelist                         // all files in selected changelist
	CopyPatchShelf                              // full patch of selected shelf
	CopyPatchFiles                              // selected file(s) or current file
	CopyPatchDiff                               // currently visible diff
)

// RemoteReq describes an immediate push/pull action (single remote, no prompt).
type RemoteReq struct {
	Mode   types.PromptMode // PromptPush or PromptPull
	Remote string
}

// PromptReq describes a prompt the coordinator should start.
type PromptReq struct {
	Mode         types.PromptMode
	DefaultValue string
	Confirm      types.ConfirmAction
	Target       string
	Options      []string // quick-select options (e.g. changelist names)
}
