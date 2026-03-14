package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/ignaciotcrespo/gitshelf/internal/controller"
	"github.com/ignaciotcrespo/gitshelf/internal/git"
	"github.com/ignaciotcrespo/gitshelf/internal/types"
	"github.com/ignaciotcrespo/gitshelf/internal/ui/panel"
)

// panelAccent returns bright white if focused, light gray if not.
func panelAccent(focused bool) lipgloss.Color {
	if focused {
		return selectedAccent
	}
	return contextAccent
}

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	if m.state.ShowHelp {
		return m.renderHelpScreen()
	}

	isCL := controller.IsChangelistContext(m.state)

	// Branch header
	branch := git.CurrentBranch()
	commit := git.HeadCommit()
	header := lipgloss.NewStyle().Bold(true).Foreground(selectedAccent).Render(" "+branch) + statusBarStyle.Render(" "+commit)
	if m.ahead > 0 {
		header += aheadStyle.Render(fmt.Sprintf(" ↑%d", m.ahead))
	}
	if m.behind > 0 {
		header += behindStyle.Render(fmt.Sprintf(" ↓%d", m.behind))
	}

	// Handle maximized panels
	if m.state.DiffState == types.PanelMaximized {
		maxH := m.height - 3
		maxW := m.width - 2
		dr := m.renderDiffContent(maxH, maxW)
		p := panel.Box(4, "Diff", dr.content, maxW, maxH, true, panel.BoxOpts{Scroll: dr.scroll, Accent: selectedAccent})
		return lipgloss.JoinVertical(lipgloss.Left, header, p, m.renderHelp())
	}
	if m.state.LogState == types.PanelMaximized {
		maxH := m.height - 3
		maxW := m.width - 2
		logPanel := m.renderLogPanel(maxH, maxW)
		return lipgloss.JoinVertical(lipgloss.Left, header, logPanel, m.renderHelp())
	}

	// Calculate log height
	logH := 0
	if m.state.LogState == types.PanelNormal {
		logH = 8
	}
	contentH := m.height - 5 - logH

	// Calculate panel widths
	leftW := m.width / 4
	middleW := m.width / 4
	var rightW int
	if m.state.DiffState == types.PanelHidden {
		rightW = 0
		middleW = m.width - leftW - 4
	} else {
		rightW = m.width - leftW - middleW - 6
	}

	// Left column: stacked panels (CL, Shelves, optionally Worktrees)
	// Each panel.Box adds 2 lines (top+bottom border), so subtract accordingly.
	var clH, shH, wtH int
	switch m.state.WorktreeState {
	case types.PanelNormal:
		// 3 panels × 2 border lines = 6
		leftInner := contentH - 4 // contentH+2 total - 6 borders
		clH = leftInner / 3
		shH = leftInner / 3
		wtH = leftInner - clH - shH
	default: // Minimized
		// 2 panels × 2 border lines = 4, plus 1 minimized bar line
		leftInner := contentH - 3 // contentH+2 total - 4 borders - 1 min bar
		clH = leftInner / 2
		shH = leftInner - clH
		wtH = 1
	}

	// Changelists panel: in-context accent when CL context, gray when shelf context
	clFocused := m.state.Focus == types.PanelChangelists
	clPC := m.renderChangelistContent(clH)
	var clOpts panel.BoxOpts
	clOpts.Scroll = clPC.scroll
	if isCL {
		clOpts.Accent = panelAccent(clFocused)
	}
	clBox := panel.Box(1, "Changelists", clPC.content, leftW, clH, clFocused, clOpts)

	// Shelves panel: in-context accent when shelf context, gray when CL context
	shFocused := m.state.Focus == types.PanelShelves
	shPC := m.renderShelvesContent(shH)
	var shOpts panel.BoxOpts
	shOpts.Scroll = shPC.scroll
	if !isCL {
		shOpts.Accent = panelAccent(shFocused)
	}
	shBox := panel.Box(2, "Shelves", shPC.content, leftW, shH, shFocused, shOpts)

	var leftColumn string
	switch m.state.WorktreeState {
	case types.PanelNormal:
		wtFocused := m.state.Focus == types.PanelWorktrees
		wtPC := m.renderWorktreesContent(wtH)
		wtBox := panel.Box(6, "Worktrees", wtPC.content, leftW, wtH, wtFocused, panel.BoxOpts{Scroll: wtPC.scroll})
		leftColumn = lipgloss.JoinVertical(lipgloss.Left, clBox, shBox, wtBox)
	default: // Minimized
		currentWT := git.WorktreeName()
		minBar := m.renderMinimizedWorktreeBar(leftW+2, currentWT)
		leftColumn = lipgloss.JoinVertical(lipgloss.Left, clBox, shBox, minBar)
	}

	// Files panel — always in context
	filesFocused := m.state.Focus == types.PanelFiles
	filesMaxLines := contentH
	var filesPC panelContent
	if isCL {
		filesPC = m.renderChangelistFilesContent(filesMaxLines)
	} else {
		filesPC = m.renderFilesContent(m.shelfFiles, m.state.ShelfFileSel, filesMaxLines)
	}
	middle := panel.Box(3, "Files", filesPC.content, middleW, contentH, filesFocused, panel.BoxOpts{Scroll: filesPC.scroll, Accent: panelAccent(filesFocused)})

	var topPanels string
	if m.state.DiffState == types.PanelHidden {
		topPanels = lipgloss.JoinHorizontal(lipgloss.Top, leftColumn, middle)
	} else {
		diffFocused := m.state.Focus == types.PanelDiff
		dr := m.renderDiffContent(contentH, rightW)
		right := panel.Box(4, "Diff", dr.content, rightW, contentH, diffFocused, panel.BoxOpts{Scroll: dr.scroll, Accent: panelAccent(diffFocused)})
		topPanels = lipgloss.JoinHorizontal(lipgloss.Top, leftColumn, middle, right)
	}

	// Record panel regions for mouse click detection
	clBoxH := clH + 2
	shBoxH := shH + 2
	leftColW := leftW + 2
	m.panelRegions[types.PanelChangelists] = panel.Region{X: 0, Y: 0, W: leftColW, H: clBoxH}
	m.panelRegions[types.PanelShelves] = panel.Region{X: 0, Y: clBoxH, W: leftColW, H: shBoxH}
	if m.state.WorktreeState == types.PanelNormal {
		wtBoxH := wtH + 2
		m.panelRegions[types.PanelWorktrees] = panel.Region{X: 0, Y: clBoxH + shBoxH, W: leftColW, H: wtBoxH}
	}
	middleX := leftColW
	middleBoxW := middleW + 2
	m.panelRegions[types.PanelFiles] = panel.Region{X: middleX, Y: 0, W: middleBoxW, H: contentH + 2}
	if m.state.DiffState != types.PanelHidden {
		diffX := middleX + middleBoxW
		m.panelRegions[types.PanelDiff] = panel.Region{X: diffX, Y: 0, W: rightW + 2, H: contentH + 2}
	}
	if m.state.LogState == types.PanelNormal {
		logY := contentH + 2
		m.panelRegions[types.PanelLog] = panel.Region{X: 0, Y: logY, W: m.width, H: logH}
	}

	// Build final layout
	var parts []string
	parts = append(parts, header)
	parts = append(parts, topPanels)
	if m.state.LogState == types.PanelNormal {
		parts = append(parts, m.renderLogPanel(logH, m.width-2))
	}
	if m.prompt.Active() {
		parts = append(parts, m.prompt.Render())
	}
	parts = append(parts, m.renderHelp())
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
