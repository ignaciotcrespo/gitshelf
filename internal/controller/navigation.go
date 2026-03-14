package controller

import "github.com/ignaciotcrespo/gitshelf/internal/types"

// MoveUp moves the cursor up in the focused panel. Returns what needs refreshing.
func MoveUp(s *State, ctx KeyContext) RefreshFlag {
	switch s.Focus {
	case types.PanelChangelists:
		if s.CLSelected > 0 {
			s.CLSelected--
			return RefreshCLFiles
		}
	case types.PanelShelves:
		if s.ShelfSel > 0 {
			s.ShelfSel--
			return RefreshShelfFiles
		}
	case types.PanelFiles:
		if IsChangelistContext(*s) {
			if s.CLFileSel > 0 {
				s.CLFileSel--
				return RefreshDiff
			}
		} else {
			if s.ShelfFileSel > 0 {
				s.ShelfFileSel--
				return RefreshDiff
			}
		}
	case types.PanelDiff:
		if s.DiffScroll > 0 {
			s.DiffScroll--
		}
	case types.PanelLog:
		s.LogScroll++
	case types.PanelWorktrees:
		if s.WorktreeSel > 0 {
			s.WorktreeSel--
		}
	}
	return RefreshNone
}

// MoveDown moves the cursor down in the focused panel. Returns what needs refreshing.
func MoveDown(s *State, ctx KeyContext) RefreshFlag {
	switch s.Focus {
	case types.PanelChangelists:
		if s.CLSelected < ctx.CLCount-1 {
			s.CLSelected++
			return RefreshCLFiles
		}
	case types.PanelShelves:
		if s.ShelfSel < ctx.ShelfCount-1 {
			s.ShelfSel++
			return RefreshShelfFiles
		}
	case types.PanelFiles:
		if IsChangelistContext(*s) {
			if s.CLFileSel < ctx.CLFileCount-1 {
				s.CLFileSel++
				return RefreshDiff
			}
		} else {
			if s.ShelfFileSel < ctx.ShelfFileCount-1 {
				s.ShelfFileSel++
				return RefreshDiff
			}
		}
	case types.PanelDiff:
		s.DiffScroll++
	case types.PanelLog:
		if s.LogScroll > 0 {
			s.LogScroll--
		}
	case types.PanelWorktrees:
		if s.WorktreeSel < ctx.WorktreeCount-1 {
			s.WorktreeSel++
		}
	}
	return RefreshNone
}

// HandleEnter handles the enter key. Returns what needs refreshing.
func HandleEnter(s *State, ctx KeyContext) RefreshFlag {
	switch s.Focus {
	case types.PanelChangelists, types.PanelShelves:
		s.Focus = types.PanelFiles
		return RefreshDiff
	case types.PanelFiles:
		s.Focus = types.PanelDiff
		return RefreshDiff
	case types.PanelWorktrees:
		if s.WorktreeSel >= 0 && s.WorktreeSel < len(ctx.WorktreePaths) {
			path := ctx.WorktreePaths[s.WorktreeSel]
			if path == s.ActiveWorktreePath || path == ctx.CurrentWorktreePath {
				// Toggle off: back to current worktree
				s.ActiveWorktreePath = ""
			} else {
				s.ActiveWorktreePath = path
			}
			// Reset selection indices for new data source
			s.CLSelected = 0
			s.CLFileSel = 0
			s.ShelfSel = 0
			s.ShelfFileSel = 0
			s.SelectedFiles = make(map[string]bool)
			return RefreshAll
		}
	}
	return RefreshNone
}
