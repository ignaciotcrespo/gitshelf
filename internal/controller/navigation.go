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
	}
	return RefreshNone
}

// HandleEnter handles the enter key. Returns what needs refreshing.
func HandleEnter(s *State) RefreshFlag {
	switch s.Focus {
	case types.PanelChangelists, types.PanelShelves:
		s.Focus = types.PanelFiles
		return RefreshDiff
	case types.PanelFiles:
		s.Focus = types.PanelDiff
		return RefreshDiff
	}
	return RefreshNone
}
