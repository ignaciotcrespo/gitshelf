package ui

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ignaciotcrespo/gitshelf/internal/changelist"
	"github.com/ignaciotcrespo/gitshelf/internal/controller"
	"github.com/ignaciotcrespo/gitshelf/internal/diff"
	"github.com/ignaciotcrespo/gitshelf/internal/git"
	"github.com/ignaciotcrespo/gitshelf/internal/shelf"
	"github.com/ignaciotcrespo/gitshelf/internal/types"
	"github.com/ignaciotcrespo/gitshelf/internal/ui/action"
	"github.com/ignaciotcrespo/gitshelf/internal/ui/panel"
	"github.com/ignaciotcrespo/gitshelf/internal/ui/prompt"
	"github.com/ignaciotcrespo/gitshelf/pkg/tui"
)

// diffLoadMsg is sent after a debounce delay to trigger diff loading.
type diffLoadMsg struct{ seq int }

const diffDebounce = 150 * time.Millisecond

func init() {
	panel.ActiveBorderStyle = activeBorderStyle
	panel.InactiveBorderStyle = inactiveBorderStyle
	panel.TitleStyle = titleStyle
	panel.StatusBarStyle = statusBarStyle

	prompt.InputLabelStyle = inputLabelStyle
	prompt.ErrorStyle = errorStyle
	prompt.HelpStyle = helpStyle
}

// Model is the main TUI model.
type Model struct {
	// Controller state (navigation, selection, panel states)
	state controller.State

	// Framework app for universal key/mouse handling
	fw tui.App

	// Layout
	width  int
	height int

	// Data (loaded from stores)
	clNames    []string
	clFiles    []string
	shelves    []shelf.Shelf
	shelfFiles []string
	diff       string
	ahead      int
	behind     int
	dirtyFiles map[string]bool
	dirtyCLs   map[string]bool
	worktrees  []git.Worktree

	// Prompt
	prompt        prompt.Prompt
	pendingResult *prompt.Result          // stored result awaiting confirmation
	pendingCtx    *action.ActionContext   // stored context for pending action

	// Panel regions for mouse click detection
	panelRegions map[types.PanelID]panel.Region

	// Stores
	stores  action.Stores
	clState *changelist.State

	// Debounce for diff loading
	diffSeq int // incremented on each request; only the latest fires

	// Original gitshelf directory (for switching back from worktree)
	gitshelfDir string

	// Version string (set via ldflags)
	version string
}

// openURL opens a URL in the user's default browser.
func openURL(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

// SetStatus implements action.Logger.
func (m *Model) SetStatus(msg string) {
	git.AddUserLog(msg, "")
	m.state.LogScroll = 0
}

// SetError implements action.Logger.
func (m *Model) SetError(msg string) {
	git.AddUserLog("", msg)
	m.state.LogScroll = 0
}

// NewModel creates the initial model.
func NewModel(gitshelfDir, version string) Model {
	clStore := changelist.NewStore(gitshelfDir)
	shelfStore := shelf.NewStore(gitshelfDir)

	fw := tui.NewApp(tui.AppConfig{
		Panels: []tui.PanelDef{
			{ID: types.PanelChangelists, Title: "Changelists", Num: 1, Pivot: true},
			{ID: types.PanelShelves, Title: "Shelves", Num: 2, Pivot: true},
			{ID: types.PanelFiles, Title: "Files", Num: 3},
			{ID: types.PanelDiff, Title: "Diff", Num: 4, Toggle: true},
			{ID: types.PanelLog, Title: "Log", Num: 5, Toggle: true},
			{ID: types.PanelWorktrees, Title: "Worktrees", Num: 6},
		},
		TabFlow: func(focus, pivot tui.PanelID, panelStates map[tui.PanelID]tui.PanelState) []tui.PanelID {
			flow := []tui.PanelID{pivot, types.PanelFiles}
			if panelStates[types.PanelDiff] != tui.PanelHidden {
				flow = append(flow, types.PanelDiff)
			}
			return flow
		},
	}, nil)

	m := Model{
		state:        controller.NewState(),
		fw:           fw,
		panelRegions: make(map[types.PanelID]panel.Region, 6),
		stores: action.Stores{
			CL:    clStore,
			Shelf: shelfStore,
		},
		gitshelfDir: gitshelfDir,
		version:     version,
	}

	m.syncFwState()
	m.refresh()

	// Default worktree panel state: Hidden if ≤1 worktree, Normal if 2+
	if len(m.worktrees) > 1 {
		m.state.WorktreeState = types.PanelNormal
	}

	return m
}

// syncFwState copies controller state to framework app state for mouse hit-testing.
func (m *Model) syncFwState() {
	m.fw.State.Focus = m.state.Focus
	m.fw.State.Pivot = m.state.Pivot
	m.fw.State.PanelStates[types.PanelDiff] = m.state.DiffState
	m.fw.State.PanelStates[types.PanelLog] = m.state.LogState
	m.fw.State.PanelStates[types.PanelWorktrees] = m.state.WorktreeState
}

// --- Bubbletea interface ---

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.FocusMsg:
		m.refresh()
		return m, nil

	case diffLoadMsg:
		if msg.seq == m.diffSeq {
			m.loadDiff()
		}
		return m, nil

	case tea.MouseMsg:
		cmd := m.handleMouse(msg)
		return m, cmd

	case tea.KeyMsg:
		// Prompt handling takes priority
		if m.prompt.Active() {
			result, handled, cmd := m.prompt.HandleKey(msg)
			if handled {
				if result != nil {
					if followUp := m.handlePromptResult(result); followUp {
						return m, cmd
					}
				} else if !m.prompt.Active() {
					m.SetStatus("Canceled")
					m.pendingResult = nil
					m.pendingCtx = nil
				}
				return m, cmd
			}
		}

		// All keys handled by controller (universal + domain)
		keyCtx := m.buildKeyContext()
		kr := controller.HandleKey(msg.String(), m.state, keyCtx)
		m.state = kr.State

		if kr.Quit {
			return m, tea.Quit
		}
		if kr.StatusMsg != "" {
			m.SetStatus(kr.StatusMsg)
		}
		if kr.ErrorMsg != "" {
			m.SetError(kr.ErrorMsg)
		}
		if kr.SetActive != "" {
			m.clState.Active = kr.SetActive
			if err := m.stores.CL.Save(m.clState); err != nil {
				m.SetError(fmt.Sprintf("Save error: %v", err))
			}
		}
		if kr.OpenURL != "" {
			openURL(kr.OpenURL)
			return m, nil
		}
		if kr.CopyPatch.Source != controller.CopyPatchNone {
			m.handleCopyPatch(kr.CopyPatch)
			return m, nil
		}
		if kr.RunRemote != nil {
			m.executeRemote(kr.RunRemote)
			refreshCmd := m.applyRefresh(kr.Refresh)
			return m, refreshCmd
		}
		if kr.StartPrompt != nil {
			cmd := m.startPrompt(kr.StartPrompt)
			refreshCmd := m.applyRefresh(kr.Refresh)
			return m, tea.Batch(cmd, refreshCmd)
		}
		refreshCmd := m.applyRefresh(kr.Refresh)
		return m, refreshCmd
	}

	// Forward non-key messages to prompt (e.g. cursor blink)
	if m.prompt.Active() {
		cmd := m.prompt.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *Model) buildKeyContext() controller.KeyContext {
	// Build tab flow based on current pivot and panel visibility
	tabFlow := []types.PanelID{m.state.Pivot, types.PanelFiles}
	if m.state.DiffState != types.PanelHidden {
		tabFlow = append(tabFlow, types.PanelDiff)
	}

	// Build worktree paths and names
	wtPaths := make([]string, len(m.worktrees))
	wtNames := make([]string, len(m.worktrees))
	for i, wt := range m.worktrees {
		wtPaths[i] = wt.Path
		wtNames[i] = filepath.Base(wt.Path)
	}
	currentWT := ""
	if root, err := git.RepoRoot(); err == nil {
		currentWT = root
	}

	ctx := controller.KeyContext{
		CLCount:             len(m.clNames),
		CLFileCount:         len(m.clFiles),
		CLNames:             m.clNames,
		CLFiles:             m.clFiles,
		ShelfCount:          len(m.shelves),
		ShelfFileCount:      len(m.shelfFiles),
		SelectedCount:       len(m.state.SelectedFiles),
		UnversionedName:     changelist.UnversionedName,
		DefaultName:         changelist.DefaultName,
		LastCommitMsg:       git.LastCommitMessage(),
		Remotes:             git.Remotes(),
		TabFlow:             tabFlow,
		WorktreeCount:       len(m.worktrees),
		WorktreePaths:       wtPaths,
		WorktreeNames:       wtNames,
		CurrentWorktreePath: currentWT,
	}
	// Build shelf names and dirs
	ctx.ShelfNames = make([]string, len(m.shelves))
	ctx.ShelfDirs = make([]string, len(m.shelves))
	for i, s := range m.shelves {
		ctx.ShelfNames[i] = s.Meta.Name
		ctx.ShelfDirs[i] = s.PatchDir
	}
	if m.clState != nil {
		ctx.ActiveCL = m.clState.Active
	}
	ctx.DirtyFiles = m.dirtyFiles
	ctx.DirtyCLs = m.dirtyCLs
	return ctx
}

func (m *Model) buildActionContext(r *prompt.Result) *action.ActionContext {
	ctx := &action.ActionContext{
		SelectedFiles: m.state.SelectedFiles,
	}
	if len(m.clNames) > 0 && m.state.CLSelected < len(m.clNames) {
		ctx.CLName = m.clNames[m.state.CLSelected]
	}
	switch r.Mode {
	case types.PromptRenameChangelist:
		if len(m.clNames) > 0 {
			ctx.OldName = m.clNames[m.state.CLSelected]
		}
	case types.PromptRenameShelf:
		if len(m.shelves) > 0 {
			ctx.OldName = m.shelves[m.state.ShelfSel].Meta.Name
			ctx.ShelfDir = m.shelves[m.state.ShelfSel].PatchDir
		}
	case types.PromptUnshelve:
		if len(m.shelves) > 0 {
			ctx.ShelfName = m.shelves[m.state.ShelfSel].Meta.Name
			ctx.ShelfDir = m.shelves[m.state.ShelfSel].PatchDir
		}
	}
	ctx.MoveFile = m.state.MoveFile
	ctx.DirtyFiles = m.dirtyFiles
	return ctx
}

// scheduleDiffLoad returns a tea.Cmd that will load the diff after a debounce delay.
func (m *Model) scheduleDiffLoad() tea.Cmd {
	m.diffSeq++
	seq := m.diffSeq
	return tea.Tick(diffDebounce, func(time.Time) tea.Msg {
		return diffLoadMsg{seq: seq}
	})
}

const headerLines = 1 // branch header above panels

func (m *Model) handleMouse(msg tea.MouseMsg) tea.Cmd {
	// Only handle press and wheel events
	if msg.Action != tea.MouseActionPress {
		return nil
	}

	// Help screen intercepts all mouse events
	if m.state.ShowHelp {
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if m.state.HelpScroll > 0 {
				m.state.HelpScroll = max(0, m.state.HelpScroll-3)
			}
		case tea.MouseButtonWheelDown:
			m.state.HelpScroll += 3
		case tea.MouseButtonLeft:
			// Click anywhere closes help
			m.state.ShowHelp = false
			m.state.HelpScroll = 0
		}
		return nil
	}

	// Adjust for header offset
	mx, my := msg.X, msg.Y-headerLines

	// Find which panel was hit
	var hitPanel types.PanelID = -1
	for pid, region := range m.panelRegions {
		if region.Contains(mx, my) {
			hitPanel = pid
			break
		}
	}
	if hitPanel < 0 {
		return nil
	}

	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.focusPanel(hitPanel)
		m.scrollPanel(hitPanel, -3)
		return m.afterScroll(hitPanel)

	case tea.MouseButtonWheelDown:
		m.focusPanel(hitPanel)
		m.scrollPanel(hitPanel, 3)
		return m.afterScroll(hitPanel)

	case tea.MouseButtonLeft:
		region := m.panelRegions[hitPanel]
		contentRow := my - region.Y - 1
		m.focusPanel(hitPanel)
		if contentRow >= 0 {
			prevCL := m.state.CLSelected
			prevShelf := m.state.ShelfSel
			prevFile := m.state.CLFileSel
			prevShelfFile := m.state.ShelfFileSel

			m.clickItem(hitPanel, contentRow)

			switch hitPanel {
			case types.PanelChangelists:
				if m.state.CLSelected != prevCL {
					m.loadChangelistFiles()
				}
			case types.PanelShelves:
				if m.state.ShelfSel != prevShelf {
					m.loadShelfFiles()
				}
			case types.PanelFiles:
				if m.state.CLFileSel != prevFile || m.state.ShelfFileSel != prevShelfFile {
					return m.scheduleDiffLoad()
				}
			}
		}
	}
	return nil
}

// afterScroll reloads dependent data after a wheel scroll on the given panel.
func (m *Model) afterScroll(pid types.PanelID) tea.Cmd {
	switch pid {
	case types.PanelChangelists:
		m.loadChangelistFilesNoDiff()
		return m.scheduleDiffLoad()
	case types.PanelShelves:
		m.loadShelfFilesNoDiff()
		return m.scheduleDiffLoad()
	case types.PanelFiles:
		return m.scheduleDiffLoad()
	}
	// Diff, Log — no dependent reload needed
	return nil
}

// focusPanel sets focus (and pivot if needed) to the given panel.
func (m *Model) focusPanel(pid types.PanelID) {
	m.state.Focus = pid
	switch pid {
	case types.PanelChangelists:
		m.state.Pivot = types.PanelChangelists
	case types.PanelShelves:
		m.state.Pivot = types.PanelShelves
	}
}

// scrollPanel moves the cursor in the given panel by delta items.
func (m *Model) scrollPanel(pid types.PanelID, delta int) {
	switch pid {
	case types.PanelChangelists:
		m.state.CLSelected = clamp(m.state.CLSelected+delta, 0, max(0, len(m.clNames)-1))
	case types.PanelShelves:
		m.state.ShelfSel = clamp(m.state.ShelfSel+delta, 0, max(0, len(m.shelves)-1))
	case types.PanelFiles:
		if controller.IsChangelistContext(m.state) {
			m.state.CLFileSel = clamp(m.state.CLFileSel+delta, 0, max(0, len(m.clFiles)-1))
		} else {
			m.state.ShelfFileSel = clamp(m.state.ShelfFileSel+delta, 0, max(0, len(m.shelfFiles)-1))
		}
	case types.PanelDiff:
		m.state.DiffScroll = max(0, m.state.DiffScroll+delta)
	case types.PanelLog:
		m.state.LogScroll = max(0, m.state.LogScroll+delta)
	case types.PanelWorktrees:
		m.state.WorktreeSel = clamp(m.state.WorktreeSel+delta, 0, max(0, len(m.worktrees)-1))
	}
}

// clickItem selects the item at the given content row within a panel.
func (m *Model) clickItem(pid types.PanelID, row int) {
	switch pid {
	case types.PanelChangelists:
		region := m.panelRegions[pid]
		maxLines := region.H - 2
		start, _ := visibleRange(m.state.CLSelected, len(m.clNames), maxLines, 1)
		idx := start + row
		if idx >= 0 && idx < len(m.clNames) {
			m.state.CLSelected = idx
		}
	case types.PanelShelves:
		region := m.panelRegions[pid]
		maxLines := region.H - 2
		start, _ := visibleRange(m.state.ShelfSel, len(m.shelves), maxLines, 2)
		idx := start + row/2 // shelves use 2 lines per item
		if idx >= 0 && idx < len(m.shelves) {
			m.state.ShelfSel = idx
		}
	case types.PanelFiles:
		if controller.IsChangelistContext(m.state) {
			region := m.panelRegions[pid]
			maxLines := region.H - 2
			start, _ := visibleRange(m.state.CLFileSel, len(m.clFiles), maxLines, 1)
			idx := start + row
			if idx >= 0 && idx < len(m.clFiles) {
				m.state.CLFileSel = idx
			}
		} else {
			region := m.panelRegions[pid]
			maxLines := region.H - 2
			start, _ := visibleRange(m.state.ShelfFileSel, len(m.shelfFiles), maxLines, 1)
			idx := start + row
			if idx >= 0 && idx < len(m.shelfFiles) {
				m.state.ShelfFileSel = idx
			}
		}
	case types.PanelWorktrees:
		region := m.panelRegions[pid]
		maxLines := region.H - 2
		start, _ := visibleRange(m.state.WorktreeSel, len(m.worktrees), maxLines, 1)
		idx := start + row
		if idx >= 0 && idx < len(m.worktrees) {
			m.state.WorktreeSel = idx
		}
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// handlePromptResult intercepts prompt results that need confirmation before executing.
// Returns true if a follow-up confirm was started (caller should return early).
func (m *Model) handlePromptResult(result *prompt.Result) bool {
	ctx := m.buildActionContext(result)

	switch result.Mode {
	case types.PromptShelveFiles:
		// Count files that will be shelved
		fileCount := 0
		if len(ctx.SelectedFiles) > 0 {
			fileCount = len(ctx.SelectedFiles)
		} else if m.clState != nil {
			for _, cl := range m.clState.Changelists {
				if cl.Name == ctx.CLName {
					fileCount = len(cl.Files)
					break
				}
			}
		}
		m.pendingResult = result
		m.pendingCtx = ctx
		m.prompt.StartConfirm(types.ConfirmShelve,
			fmt.Sprintf("%s:%d", result.Value, fileCount))
		return true

	case types.PromptUnshelve:
		// Check if any shelf files already exist in working tree
		var conflicting int
		if ctx.ShelfDir != "" {
			changed := git.ChangedFileSet()
			for _, s := range m.shelves {
				if s.PatchDir == ctx.ShelfDir {
					for _, f := range s.Meta.Files {
						if changed[f] {
							conflicting++
						}
					}
					break
				}
			}
		}
		if conflicting > 0 {
			m.pendingResult = result
			m.pendingCtx = ctx
			m.prompt.StartConfirm(types.ConfirmUnshelve,
				fmt.Sprintf("%s:%d:%d", ctx.ShelfName, len(m.shelfFiles), conflicting))
			return true
		}
		// No conflicts — execute directly
		action.Execute(result, &m.stores, m, ctx)
		m.refresh()
		return false

	case types.PromptPasteChangelist:
		if m.state.ClipboardCL == nil {
			m.SetError("Nothing in clipboard")
			return false
		}
		ctx.SourceWorktreePath = m.state.ClipboardCL.SourceWorktree
		ctx.ClipboardCLName = m.state.ClipboardCL.Name
		ctx.ClipboardFiles = m.state.ClipboardCL.Files

		if result.Value == types.PasteFullContent {
			// Full content requires confirmation
			m.pendingResult = result
			m.pendingCtx = ctx
			m.prompt.StartConfirm(types.ConfirmPasteFullContent,
				fmt.Sprintf("%d", len(ctx.ClipboardFiles)))
			return true
		}
		// Apply diff and Only changelist execute directly
		action.Execute(result, &m.stores, m, ctx)
		m.refresh()
		return false

	case types.PromptConfirm:
		if result.Confirmed {
			switch result.ConfirmAction {
			case types.ConfirmShelve:
				if m.pendingResult != nil {
					action.Execute(m.pendingResult, &m.stores, m, m.pendingCtx)
					m.refresh()
				}
			case types.ConfirmUnshelve:
				if m.pendingResult != nil {
					m.pendingCtx.ForceUnshelve = true
					action.Execute(m.pendingResult, &m.stores, m, m.pendingCtx)
					m.refresh()
				}
			case types.ConfirmPasteFullContent:
				if m.pendingResult != nil {
					action.Execute(m.pendingResult, &m.stores, m, m.pendingCtx)
					m.refresh()
				}
			default:
				// For drop shelf, set ShelfDir from current selection
				if result.ConfirmAction == types.ConfirmDropShelf && len(m.shelves) > 0 && m.state.ShelfSel < len(m.shelves) {
					ctx.ShelfDir = m.shelves[m.state.ShelfSel].PatchDir
				}
				action.Execute(result, &m.stores, m, ctx)
				m.refresh()
			}
		} else {
			m.SetStatus("Canceled")
		}
		m.pendingResult = nil
		m.pendingCtx = nil
		return false

	default:
		action.Execute(result, &m.stores, m, ctx)
		m.refresh()
		return false
	}
}

func (m *Model) executeRemote(req *controller.RemoteReq) {
	result := &prompt.Result{Mode: req.Mode, Value: req.Remote}
	action.Execute(result, &m.stores, m, nil)
	m.refresh()
}

// handleCopyPatch generates the appropriate patch content and copies it to the clipboard.
func (m *Model) handleCopyPatch(req controller.CopyPatchReq) {
	var patch string
	var desc string

	switch req.Source {
	case controller.CopyPatchChangelist:
		if len(m.clNames) == 0 || m.state.CLSelected >= len(m.clNames) {
			return
		}
		clName := m.clNames[m.state.CLSelected]
		files, err := changelist.FilesForChangelist(m.clState, clName)
		if err != nil || len(files) == 0 {
			m.SetError("No files to copy")
			return
		}
		d, err := git.DiffFiles(files...)
		if err != nil {
			m.SetError(fmt.Sprintf("Diff error: %v", err))
			return
		}
		patch = d
		desc = fmt.Sprintf("changelist '%s' (%d files)", clName, len(files))

	case controller.CopyPatchShelf:
		if len(m.shelves) == 0 || m.state.ShelfSel >= len(m.shelves) {
			return
		}
		s := m.shelves[m.state.ShelfSel]
		d, err := m.stores.Shelf.GetPatchDir(s.PatchDir)
		if err != nil {
			m.SetError(fmt.Sprintf("Patch error: %v", err))
			return
		}
		patch = d
		desc = fmt.Sprintf("shelf '%s' (%d files)", s.Meta.Name, len(s.Meta.Files))

	case controller.CopyPatchFiles:
		if controller.IsChangelistContext(m.state) {
			// Selected files or current file
			var files []string
			if len(m.state.SelectedFiles) > 0 {
				for f := range m.state.SelectedFiles {
					files = append(files, f)
				}
			} else if len(m.clFiles) > 0 && m.state.CLFileSel < len(m.clFiles) {
				files = []string{m.clFiles[m.state.CLFileSel]}
			}
			if len(files) == 0 {
				m.SetError("No files to copy")
				return
			}
			d, err := git.DiffFiles(files...)
			if err != nil {
				m.SetError(fmt.Sprintf("Diff error: %v", err))
				return
			}
			patch = d
			desc = fmt.Sprintf("%d file(s)", len(files))
		} else {
			// Shelf files context: selected file or current file
			if len(m.shelfFiles) == 0 || m.state.ShelfFileSel >= len(m.shelfFiles) {
				return
			}
			file := m.shelfFiles[m.state.ShelfFileSel]
			fullPatch, err := m.stores.Shelf.GetPatchDir(m.shelves[m.state.ShelfSel].PatchDir)
			if err != nil {
				m.SetError(fmt.Sprintf("Patch error: %v", err))
				return
			}
			patch = diff.ExtractFileDiff(fullPatch, file)
			desc = fmt.Sprintf("file '%s' from shelf '%s'", file, m.shelves[m.state.ShelfSel].Meta.Name)
		}

	case controller.CopyPatchDiff:
		patch = m.diff
		desc = "visible diff"
	}

	if patch == "" {
		m.SetError("Nothing to copy")
		return
	}

	// Ensure trailing newline
	if !strings.HasSuffix(patch, "\n") {
		patch += "\n"
	}

	if err := clipboard.WriteAll(patch); err != nil {
		m.SetError(fmt.Sprintf("Clipboard error: %v", err))
		return
	}

	lines := strings.Count(patch, "\n")
	m.SetStatus(fmt.Sprintf("Copied patch to clipboard: %s, %d lines, %d bytes", desc, lines, len(patch)))
}

func (m *Model) startPrompt(req *controller.PromptReq) tea.Cmd {
	if req.Mode == types.PromptConfirm {
		m.prompt.StartConfirm(req.Confirm, req.Target)
		return nil
	} else if len(req.Options) > 0 {
		return m.prompt.StartWithOptions(req.Mode, req.DefaultValue, req.Options)
	}
	return m.prompt.Start(req.Mode, req.DefaultValue)
}
