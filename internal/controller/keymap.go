package controller

import (
	"fmt"

	"github.com/ignaciotcrespo/gitshelf/internal/types"
	"github.com/ignaciotcrespo/gitshelf/pkg/tui"
)

// CyclePanelState cycles a toggleable panel through: normal → maximized → hidden → normal.
// Returns the new state and whether focus should move away (when hidden).
func CyclePanelState(current types.PanelState, focused bool) (types.PanelState, bool) {
	return tui.CyclePanelState(current, focused)
}

// HandleKey processes all key presses and returns the resulting state change.
// This is a pure function: no I/O, no side effects.
func HandleKey(key string, state State, ctx KeyContext) KeyResult {
	r := KeyResult{State: state}

	// Help screen intercepts all keys
	if r.State.ShowHelp {
		switch key {
		case "?", "q", "esc":
			r.State.ShowHelp = false
			r.State.HelpScroll = 0
		case "down", "j":
			r.State.HelpScroll++
		case "up", "k":
			if r.State.HelpScroll > 0 {
				r.State.HelpScroll--
			}
		case "d":
			r.OpenURL = "https://github.com/sponsors/ignaciotcrespo"
		case "a":
			r.OpenURL = "https://github.com/ignaciotcrespo/gitshelf/discussions"
		case "r":
			r.OpenURL = "https://github.com/ignaciotcrespo/gitshelf/issues"
		}
		return r
	}

	switch key {
	// --- Universal keys ---

	case "?":
		r.State.ShowHelp = true
		r.State.HelpScroll = 0
		return r

	case "q", "ctrl+c":
		r.Quit = true
		return r

	case "tab":
		r.State.Focus = tui.TabPanel(r.State.Focus, ctx.TabFlow, 1)
		r.Refresh = RefreshAll
		return r

	case "shift+tab":
		r.State.Focus = tui.TabPanel(r.State.Focus, ctx.TabFlow, -1)
		r.Refresh = RefreshAll
		return r

	case "1":
		r.State.Focus = types.PanelChangelists
		r.State.Pivot = types.PanelChangelists
		r.Refresh = RefreshAll
		return r

	case "2":
		r.State.Focus = types.PanelShelves
		r.State.Pivot = types.PanelShelves
		r.Refresh = RefreshAll
		return r

	case "3":
		r.State.Focus = types.PanelFiles
		r.Refresh = RefreshAll
		return r

	case "4":
		focused := r.State.Focus == types.PanelDiff
		newState, moveFocus := tui.CyclePanelState(r.State.DiffState, focused)
		r.State.DiffState = newState
		if moveFocus {
			r.State.Focus = r.State.Pivot
		} else if !focused {
			r.State.Focus = types.PanelDiff
		}
		r.Refresh = RefreshAll
		return r

	case "5":
		focused := r.State.Focus == types.PanelLog
		newState, moveFocus := tui.CyclePanelState(r.State.LogState, focused)
		r.State.LogState = newState
		if moveFocus {
			r.State.Focus = r.State.Pivot
		} else if !focused {
			r.State.Focus = types.PanelLog
		}
		r.Refresh = RefreshAll
		return r

	// --- Navigation keys ---

	case "up", "k":
		r.Refresh = MoveUp(&r.State, ctx)
		return r

	case "down", "j":
		r.Refresh = MoveDown(&r.State, ctx)
		return r

	case "left", "h":
		if r.State.Focus == types.PanelDiff && !r.State.DiffWrap && r.State.DiffHScroll > 0 {
			r.State.DiffHScroll -= 4
			if r.State.DiffHScroll < 0 {
				r.State.DiffHScroll = 0
			}
		}
		return r

	case "right", "l":
		if r.State.Focus == types.PanelDiff && !r.State.DiffWrap {
			r.State.DiffHScroll += 4
		}
		return r

	case "w":
		if r.State.Focus == types.PanelDiff {
			r.State.DiffWrap = !r.State.DiffWrap
			r.State.DiffHScroll = 0
		}
		return r

	case "enter":
		r.Refresh = HandleEnter(&r.State)
		return r
	}

	// Copy patch — works in all panels
	if key == "y" {
		switch r.State.Focus {
		case types.PanelChangelists:
			if ctx.CLCount > 0 {
				r.CopyPatch = CopyPatchReq{Source: CopyPatchChangelist}
			}
		case types.PanelShelves:
			if ctx.ShelfCount > 0 {
				r.CopyPatch = CopyPatchReq{Source: CopyPatchShelf}
			}
		case types.PanelFiles:
			if IsChangelistContext(r.State) && ctx.CLFileCount > 0 {
				r.CopyPatch = CopyPatchReq{Source: CopyPatchFiles}
			} else if !IsChangelistContext(r.State) && ctx.ShelfFileCount > 0 {
				r.CopyPatch = CopyPatchReq{Source: CopyPatchFiles}
			}
		case types.PanelDiff:
			r.CopyPatch = CopyPatchReq{Source: CopyPatchDiff}
		}
		return r
	}

	// Context-specific keys
	if r.State.Focus == types.PanelChangelists || (r.State.Focus == types.PanelFiles && IsChangelistContext(r.State)) {
		return handleChangelistKey(key, r, ctx)
	}
	if r.State.Focus == types.PanelShelves {
		return handleShelfKey(key, r, ctx)
	}
	return handleChangelistKey(key, r, ctx)
}

func handleChangelistKey(key string, r KeyResult, ctx KeyContext) KeyResult {
	switch key {
	case "n":
		r.StartPrompt = &PromptReq{Mode: types.PromptNewChangelist}

	case "m":
		if r.State.Focus == types.PanelFiles && ctx.CLFileCount > 0 && r.State.CLFileSel < ctx.CLFileCount {
			if ctx.SelectedCount == 0 {
				r.State.MoveFile = ctx.CLFiles[r.State.CLFileSel]
			}
			r.StartPrompt = &PromptReq{Mode: types.PromptMoveFile, Options: ctx.CLNames}
		}

	case " ":
		if r.State.Focus == types.PanelFiles && ctx.CLFileCount > 0 && r.State.CLFileSel < ctx.CLFileCount {
			file := ctx.CLFiles[r.State.CLFileSel]
			if r.State.SelectedFiles == nil {
				r.State.SelectedFiles = make(map[string]bool)
			}
			if r.State.SelectedFiles[file] {
				delete(r.State.SelectedFiles, file)
			} else {
				r.State.SelectedFiles[file] = true
			}
			if r.State.CLFileSel < ctx.CLFileCount-1 {
				r.State.CLFileSel++
			}
		}

	case "a":
		if r.State.Focus == types.PanelFiles && ctx.CLFileCount > 0 {
			if r.State.SelectedFiles == nil {
				r.State.SelectedFiles = make(map[string]bool)
			}
			for _, f := range ctx.CLFiles {
				r.State.SelectedFiles[f] = true
			}
		} else if r.State.Focus == types.PanelChangelists && ctx.CLCount > 0 {
			name := ctx.CLNames[r.State.CLSelected]
			if name != ctx.UnversionedName {
				r.SetActive = name
				r.StatusMsg = fmt.Sprintf("Active: %s", name)
			}
		}
		return r

	case "x":
		if r.State.Focus == types.PanelFiles {
			r.State.SelectedFiles = make(map[string]bool)
		}

	case "s":
		if ctx.CLCount == 0 {
			return r
		}
		clName := ctx.CLNames[r.State.CLSelected]
		if clName == ctx.UnversionedName {
			r.ErrorMsg = "Cannot shelve unversioned files"
			return r
		}
		if r.State.Focus == types.PanelFiles && ctx.SelectedCount > 0 {
			r.StartPrompt = &PromptReq{Mode: types.PromptShelveFiles, DefaultValue: clName}
		} else if r.State.Focus == types.PanelChangelists {
			r.StartPrompt = &PromptReq{Mode: types.PromptShelveFiles, DefaultValue: clName}
		} else if r.State.Focus == types.PanelFiles && ctx.SelectedCount == 0 {
			r.ErrorMsg = "Select files with space first"
		}

	case "c":
		if ctx.SelectedCount == 0 {
			r.ErrorMsg = "Select files with space before committing"
		} else {
			r.StartPrompt = &PromptReq{Mode: types.PromptCommit}
			r.StatusMsg = fmt.Sprintf("Committing %d selected file(s)", ctx.SelectedCount)
		}

	case "A":
		if ctx.SelectedCount == 0 {
			r.ErrorMsg = "Select files with space before amending"
		} else {
			r.StartPrompt = &PromptReq{Mode: types.PromptAmend, DefaultValue: ctx.LastCommitMsg}
			r.StatusMsg = fmt.Sprintf("Amending with %d selected file(s)", ctx.SelectedCount)
		}

	case "r":
		if r.State.Focus == types.PanelChangelists && ctx.CLCount > 0 {
			name := ctx.CLNames[r.State.CLSelected]
			if name != ctx.UnversionedName {
				r.StartPrompt = &PromptReq{Mode: types.PromptRenameChangelist, DefaultValue: name}
			}
		}

	case "d":
		if r.State.Focus == types.PanelChangelists && ctx.CLCount > 0 {
			name := ctx.CLNames[r.State.CLSelected]
			if name != ctx.UnversionedName && name != ctx.DefaultName {
				r.StartPrompt = &PromptReq{
					Mode:    types.PromptConfirm,
					Confirm: types.ConfirmDeleteChangelist,
					Target:  name,
				}
			}
		}

	case "p":
		r.StartPrompt, r.RunRemote = buildRemoteAction(types.PromptPush, ctx.Remotes)

	case "P":
		r.StartPrompt, r.RunRemote = buildRemoteAction(types.PromptPull, ctx.Remotes)

	case "B":
		if r.State.Focus == types.PanelChangelists && ctx.CLCount > 0 {
			name := ctx.CLNames[r.State.CLSelected]
			if ctx.DirtyCLs[name] {
				// Count dirty files in this CL
				dirtyCount := 0
				for _, f := range ctx.CLFiles {
					if ctx.DirtyFiles[f] {
						dirtyCount++
					}
				}
				r.StartPrompt = &PromptReq{
					Mode:    types.PromptConfirm,
					Confirm: types.ConfirmAcceptDirty,
					Target:  fmt.Sprintf("cl:%s:%d", name, dirtyCount),
				}
			}
		} else if r.State.Focus == types.PanelFiles {
			// Accept selected dirty files, or current file if none selected
			var dirtyCount int
			if ctx.SelectedCount > 0 {
				for f := range r.State.SelectedFiles {
					if ctx.DirtyFiles[f] {
						dirtyCount++
					}
				}
			} else if ctx.CLFileCount > 0 && r.State.CLFileSel < ctx.CLFileCount {
				if ctx.DirtyFiles[ctx.CLFiles[r.State.CLFileSel]] {
					dirtyCount = 1
				}
			}
			if dirtyCount > 0 {
				clName := ""
				if ctx.CLCount > 0 {
					clName = ctx.CLNames[r.State.CLSelected]
				}
				r.StartPrompt = &PromptReq{
					Mode:    types.PromptConfirm,
					Confirm: types.ConfirmAcceptDirty,
					Target:  fmt.Sprintf("files:%s:%d", clName, dirtyCount),
				}
			}
		}
	}
	return r
}

// buildRemoteAction decides whether to prompt or execute immediately.
// 0 or 1 remote → immediate execution (no prompt).
// 2+ remotes → prompt with quick-select options.
func buildRemoteAction(mode types.PromptMode, remotes []string) (*PromptReq, *RemoteReq) {
	if len(remotes) <= 1 {
		remote := "origin"
		if len(remotes) == 1 {
			remote = remotes[0]
		}
		return nil, &RemoteReq{Mode: mode, Remote: remote}
	}
	return &PromptReq{
		Mode:         mode,
		DefaultValue: remotes[0],
		Options:      remotes,
	}, nil
}

func handleShelfKey(key string, r KeyResult, ctx KeyContext) KeyResult {
	switch key {
	case "u":
		if ctx.ShelfCount > 0 {
			r.StartPrompt = &PromptReq{
				Mode:         types.PromptUnshelve,
				DefaultValue: ctx.ActiveCL,
				Target:       ctx.ShelfNames[r.State.ShelfSel],
				Options:      ctx.CLNames,
			}
		}

	case "d":
		if ctx.ShelfCount > 0 {
			r.StartPrompt = &PromptReq{
				Mode:    types.PromptConfirm,
				Confirm: types.ConfirmDropShelf,
				Target:  ctx.ShelfNames[r.State.ShelfSel],
			}
		}

	case "r":
		if ctx.ShelfCount > 0 {
			r.StartPrompt = &PromptReq{
				Mode:         types.PromptRenameShelf,
				DefaultValue: ctx.ShelfNames[r.State.ShelfSel],
			}
		}
	}
	return r
}
