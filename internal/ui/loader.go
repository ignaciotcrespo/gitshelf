package ui

import (
	"fmt"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ignaciotcrespo/gitshelf/internal/changelist"
	"github.com/ignaciotcrespo/gitshelf/internal/controller"
	"github.com/ignaciotcrespo/gitshelf/internal/diff"
	"github.com/ignaciotcrespo/gitshelf/internal/git"
	"github.com/ignaciotcrespo/gitshelf/internal/shelf"
	"github.com/ignaciotcrespo/gitshelf/internal/types"
)

func (m *Model) loadChangelists() {
	state, err := m.stores.CL.Load()
	if err != nil {
		m.SetError(fmt.Sprintf("Error: %v", err))
		return
	}
	m.clState = state
	m.stores.State = state

	if err := changelist.AutoAssignNewFiles(state); err != nil {
		m.SetError(fmt.Sprintf("Error: %v", err))
	}

	// Compute dirty state by comparing diff hashes
	currentHashes := git.FileDiffHashes()
	m.dirtyFiles, m.dirtyCLs = changelist.ComputeDirty(state, currentHashes)

	if err := m.stores.CL.Save(state); err != nil {
		m.SetError(fmt.Sprintf("Save error: %v", err))
	}

	m.clNames = changelist.AllNames(state)
	if m.state.CLSelected >= len(m.clNames) {
		m.state.CLSelected = max(0, len(m.clNames)-1)
	}
	m.loadChangelistFiles()
}

func (m *Model) loadChangelistFiles() {
	if m.clState == nil || len(m.clNames) == 0 {
		m.clFiles = nil
		return
	}
	name := m.clNames[m.state.CLSelected]
	files, err := changelist.FilesForChangelist(m.clState, name)
	if err != nil {
		m.SetError(fmt.Sprintf("Error: %v", err))
		return
	}
	m.clFiles = files
	if m.state.CLFileSel >= len(m.clFiles) {
		m.state.CLFileSel = max(0, len(m.clFiles)-1)
	}
	m.state.SelectedFiles = make(map[string]bool)
	if controller.IsChangelistContext(m.state) {
		m.loadDiff()
	}
	m.state.DiffScroll = 0
}

func (m *Model) loadShelves() {
	shelves, err := m.stores.Shelf.List()
	if err != nil {
		m.SetError(fmt.Sprintf("Error loading shelves: %v", err))
		return
	}
	m.shelves = shelves
	if m.state.ShelfSel >= len(m.shelves) {
		m.state.ShelfSel = max(0, len(m.shelves)-1)
	}
	m.loadShelfFiles()
}

func (m *Model) loadShelfFiles() {
	if len(m.shelves) == 0 {
		m.shelfFiles = nil
		if m.state.Focus == types.PanelShelves {
			m.diff = ""
			m.state.DiffScroll = 0
		}
		return
	}
	m.shelfFiles = m.shelves[m.state.ShelfSel].Meta.Files
	if m.state.ShelfFileSel >= len(m.shelfFiles) {
		m.state.ShelfFileSel = max(0, len(m.shelfFiles)-1)
	}
	if m.state.Focus == types.PanelShelves {
		m.loadDiff()
		m.state.DiffScroll = 0
	}
}

// loadChangelistFilesNoDiff reloads changelist files without loading the diff.
func (m *Model) loadChangelistFilesNoDiff() {
	if m.clState == nil || len(m.clNames) == 0 {
		m.clFiles = nil
		return
	}
	name := m.clNames[m.state.CLSelected]
	files, err := changelist.FilesForChangelist(m.clState, name)
	if err != nil {
		m.SetError(fmt.Sprintf("Error: %v", err))
		return
	}
	m.clFiles = files
	if m.state.CLFileSel >= len(m.clFiles) {
		m.state.CLFileSel = max(0, len(m.clFiles)-1)
	}
	m.state.SelectedFiles = make(map[string]bool)
	m.state.DiffScroll = 0
}

// loadShelfFilesNoDiff reloads shelf files without loading the diff.
func (m *Model) loadShelfFilesNoDiff() {
	if len(m.shelves) == 0 {
		m.shelfFiles = nil
		if m.state.Focus == types.PanelShelves {
			m.diff = ""
			m.state.DiffScroll = 0
		}
		return
	}
	m.shelfFiles = m.shelves[m.state.ShelfSel].Meta.Files
	if m.state.ShelfFileSel >= len(m.shelfFiles) {
		m.state.ShelfFileSel = max(0, len(m.shelfFiles)-1)
	}
	m.state.DiffScroll = 0
}

func (m *Model) loadDiff() {
	if controller.IsChangelistContext(m.state) {
		if len(m.clFiles) > 0 && m.state.CLFileSel < len(m.clFiles) {
			file := m.clFiles[m.state.CLFileSel]
			d, err := git.DiffFile(file)
			if err != nil {
				m.diff = ""
				return
			}
			m.diff = d
			m.state.DiffScroll = 0
			return
		}
	} else {
		if len(m.shelfFiles) > 0 && m.state.ShelfFileSel < len(m.shelfFiles) {
			file := m.shelfFiles[m.state.ShelfFileSel]
			patch, err := m.stores.Shelf.GetPatchDir(m.shelves[m.state.ShelfSel].PatchDir)
			if err == nil {
				m.diff = diff.ExtractFileDiff(patch, file)
			}
			m.state.DiffScroll = 0
			return
		}
	}
	m.diff = ""
}

func (m *Model) loadWorktrees() {
	wts, err := git.WorktreeList(filepath.Dir(m.gitshelfDir))
	if err != nil {
		m.worktrees = nil
		return
	}
	m.worktrees = wts
	if m.state.WorktreeSel >= len(m.worktrees) {
		m.state.WorktreeSel = max(0, len(m.worktrees)-1)
	}
}

func (m *Model) refresh() {
	// Switch stores if active worktree changed
	m.syncStores()
	m.loadChangelists()
	m.loadShelves()
	m.loadWorktrees()
	m.ahead, m.behind = git.AheadBehind()
}

// syncStores switches the CL and Shelf stores and the git repo root to point
// at the active worktree, or back to the original if no worktree is active.
func (m *Model) syncStores() {
	dir := m.gitshelfDir
	repoDir := filepath.Dir(m.gitshelfDir)
	if m.state.ActiveWorktreePath != "" {
		dir = filepath.Join(m.state.ActiveWorktreePath, ".gitshelf")
		repoDir = m.state.ActiveWorktreePath
	}
	m.stores.CL = changelist.NewStore(dir)
	m.stores.Shelf = shelf.NewStore(dir)
	git.SetRepoRoot(repoDir)
}

// applyRefresh performs data loading based on the refresh flag.
// Returns a tea.Cmd for debounced diff loading when appropriate.
func (m *Model) applyRefresh(flag controller.RefreshFlag) tea.Cmd {
	switch {
	case flag&controller.RefreshAll != 0:
		m.refresh()
		return nil
	case flag&controller.RefreshWorktree != 0:
		return m.scheduleWorktreeSwitch()
	case flag&controller.RefreshCLFiles != 0:
		m.loadChangelistFilesNoDiff()
		return m.scheduleDiffLoad()
	case flag&controller.RefreshShelfFiles != 0:
		m.loadShelfFilesNoDiff()
		return m.scheduleDiffLoad()
	case flag&controller.RefreshDiff != 0:
		return m.scheduleDiffLoad()
	}
	return nil
}
