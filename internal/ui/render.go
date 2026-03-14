package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/ignaciotcrespo/gitshelf/internal/changelist"
	"github.com/ignaciotcrespo/gitshelf/internal/controller"
	"github.com/ignaciotcrespo/gitshelf/internal/git"
	"github.com/ignaciotcrespo/gitshelf/internal/types"
	"github.com/ignaciotcrespo/gitshelf/internal/ui/panel"
	"github.com/ignaciotcrespo/gitshelf/pkg/tui"
)

// panelContent holds rendered content and scroll metadata for a panel.
type panelContent struct {
	content string
	scroll  panel.ScrollInfo
}

// visibleRange returns the start and end indices of items to display,
// ensuring the selected item is always visible within maxLines.
func visibleRange(selected, count, maxLines, linesPerItem int) (int, int) {
	return tui.VisibleRange(selected, count, maxLines, linesPerItem)
}

func (m Model) renderChangelistContent(maxLines int) panelContent {
	total := len(m.clNames)
	var b strings.Builder
	focused := m.state.Focus == types.PanelChangelists
	baseStyle := normalItemStyle
	if focused {
		baseStyle = focusedItemStyle
	}
	start, end := visibleRange(m.state.CLSelected, total, maxLines, 1)
	for i := start; i < end; i++ {
		name := m.clNames[i]
		cursor := "  "
		style := baseStyle
		if i == m.state.CLSelected {
			cursor = "▸ "
			if focused {
				style = selectedItemStyle
			} else {
				style = dimSelectedItemStyle
			}
		}

		fileCount := 0
		for _, cl := range m.clState.Changelists {
			if cl.Name == name {
				fileCount = len(cl.Files)
				break
			}
		}
		nameStr := style.Render(name)
		dirtyMark := ""
		if m.dirtyCLs[name] {
			nameStr = warningStyle.Render(name)
			dirtyMark = warningStyle.Render("* ")
		} else if name == changelist.UnversionedName && fileCount > 0 {
			nameStr = warningStyle.Render(name)
		}
		infoStr := fmt.Sprintf(" (%d files)", fileCount)
		info := statusBarStyle.Render(infoStr)
		if fileCount > 0 {
			info = activeMarkerStyle.Render(infoStr)
		}
		b.WriteString(cursor + dirtyMark + nameStr + info + "\n")
	}
	return panelContent{
		content: b.String(),
		scroll:  panel.ScrollInfo{TotalLines: total, VisibleLines: maxLines, ScrollPos: start},
	}
}

func (m Model) renderShelvesContent(maxLines int) panelContent {
	if len(m.shelves) == 0 {
		return panelContent{content: normalItemStyle.Render("  (no shelves)")}
	}
	total := len(m.shelves)
	focused := m.state.Focus == types.PanelShelves
	baseStyle := normalItemStyle
	if focused {
		baseStyle = focusedItemStyle
	}
	var b strings.Builder
	start, end := visibleRange(m.state.ShelfSel, total, maxLines, 2)
	for i := start; i < end; i++ {
		s := m.shelves[i]
		cursor := "  "
		style := baseStyle
		if i == m.state.ShelfSel {
			cursor = "▸ "
			if focused {
				style = selectedItemStyle
			} else {
				style = dimSelectedItemStyle
			}
		}
		info := statusBarStyle.Render(fmt.Sprintf(" (%d files)", len(s.Meta.Files)))
		b.WriteString(cursor + style.Render(s.Meta.Name) + info + "\n")
		branch := s.Meta.Branch
		if branch == "" {
			branch = "unknown"
		}
		ts := formatShelfTime(s.Meta.CreatedAt)
		shelfLine := fmt.Sprintf("%s@%s %s", branch, s.Meta.Commit, ts)
		if s.Meta.Worktree != "" && s.Meta.Worktree != git.WorktreeName() {
			shelfLine += fmt.Sprintf(" [%s]", s.Meta.Worktree)
		}
		b.WriteString("    " + statusBarStyle.Render(shelfLine) + "\n")
	}
	// For shelves, total lines = items * 2 (name + branch line)
	return panelContent{
		content: b.String(),
		scroll:  panel.ScrollInfo{TotalLines: total * 2, VisibleLines: maxLines, ScrollPos: start * 2},
	}
}

func (m Model) renderChangelistFilesContent(maxLines int) panelContent {
	if len(m.clFiles) == 0 {
		return panelContent{content: normalItemStyle.Render("  (no files)")}
	}
	total := len(m.clFiles)
	focused := m.state.Focus == types.PanelFiles && controller.IsChangelistContext(m.state)
	baseStyle := normalItemStyle
	if focused {
		baseStyle = focusedItemStyle
	}
	var b strings.Builder
	start, end := visibleRange(m.state.CLFileSel, total, maxLines, 1)
	for i := start; i < end; i++ {
		f := m.clFiles[i]
		cursor := "  "
		style := baseStyle
		if i == m.state.CLFileSel {
			cursor = "▸ "
			if focused {
				style = selectedItemStyle
			} else {
				style = dimSelectedItemStyle
			}
		}
		check := "[ ] "
		if m.state.SelectedFiles[f] {
			check = "[✓] "
		}
		fileStr := style.Render(f)
		dirtyMark := ""
		if m.dirtyFiles[f] {
			fileStr = warningStyle.Render(f)
			dirtyMark = warningStyle.Render("* ")
		}
		b.WriteString(cursor + activeMarkerStyle.Render(check) + dirtyMark + fileStr + "\n")
	}
	return panelContent{
		content: b.String(),
		scroll:  panel.ScrollInfo{TotalLines: total, VisibleLines: maxLines, ScrollPos: start},
	}
}

func (m Model) renderFilesContent(files []string, selected, maxLines int) panelContent {
	if len(files) == 0 {
		return panelContent{content: normalItemStyle.Render("  (no files)")}
	}
	total := len(files)
	focused := m.state.Focus == types.PanelFiles && !controller.IsChangelistContext(m.state)
	baseStyle := normalItemStyle
	if focused {
		baseStyle = focusedItemStyle
	}
	var b strings.Builder
	start, end := visibleRange(selected, total, maxLines, 1)
	for i := start; i < end; i++ {
		f := files[i]
		cursor := "  "
		style := baseStyle
		if i == selected {
			cursor = "▸ "
			if focused {
				style = selectedItemStyle
			} else {
				style = dimSelectedItemStyle
			}
		}
		b.WriteString(cursor + style.Render(f) + "\n")
	}
	return panelContent{
		content: b.String(),
		scroll:  panel.ScrollInfo{TotalLines: total, VisibleLines: maxLines, ScrollPos: start},
	}
}

func (m Model) renderDiffContent(maxLines, maxWidth int) panelContent {
	if m.diff == "" {
		return panelContent{content: normalItemStyle.Render("  (select a file)")}
	}

	// Expand tabs to spaces so rune count matches visual width
	expanded := strings.ReplaceAll(m.diff, "\t", "    ")
	rawLines := strings.Split(expanded, "\n")

	wrapAt := maxWidth - 2
	if wrapAt < 1 {
		wrapAt = 1
	}

	if m.state.DiffWrap {
		// Wrap mode: wrap long lines to wrapAt, then paginate
		var wrapped []wrappedLine
		for _, line := range rawLines {
			prefix := diffLinePrefix(line)
			runes := []rune(line)
			if len(runes) <= wrapAt {
				wrapped = append(wrapped, wrappedLine{text: line, prefix: prefix})
			} else {
				for len(runes) > 0 {
					cut := wrapAt
					if cut > len(runes) {
						cut = len(runes)
					}
					wrapped = append(wrapped, wrappedLine{text: string(runes[:cut]), prefix: prefix})
					runes = runes[cut:]
				}
			}
		}

		total := len(wrapped)
		start := m.state.DiffScroll
		if start >= total {
			start = max(0, total-1)
		}
		end := start + maxLines
		if end > total {
			end = total
		}

		var b strings.Builder
		for _, wl := range wrapped[start:end] {
			b.WriteString(renderDiffLine(wl.text, wl.prefix))
			b.WriteString("\n")
		}
		return panelContent{
			content: b.String(),
			scroll:  panel.ScrollInfo{TotalLines: total, VisibleLines: maxLines, ScrollPos: m.state.DiffScroll},
		}
	}

	// Non-wrap mode: horizontal scroll + truncate
	total := len(rawLines)
	start := m.state.DiffScroll
	if start >= total {
		start = max(0, total-1)
	}
	end := start + maxLines
	if end > total {
		end = total
	}

	hScroll := m.state.DiffHScroll
	var b strings.Builder
	for _, line := range rawLines[start:end] {
		prefix := diffLinePrefix(line)
		runes := []rune(line)
		if hScroll > 0 && len(runes) > hScroll {
			runes = runes[hScroll:]
		} else if hScroll > 0 {
			runes = nil
		}
		if len(runes) > wrapAt {
			runes = runes[:wrapAt]
		}
		b.WriteString(renderDiffLine(string(runes), prefix))
		b.WriteString("\n")
	}
	return panelContent{
		content: b.String(),
		scroll:  panel.ScrollInfo{TotalLines: total, VisibleLines: maxLines, ScrollPos: m.state.DiffScroll},
	}
}

type wrappedLine struct {
	text   string
	prefix string
}

func diffLinePrefix(line string) string {
	switch {
	case strings.HasPrefix(line, "+"):
		return "+"
	case strings.HasPrefix(line, "-"):
		return "-"
	case strings.HasPrefix(line, "@@"):
		return "@@"
	case strings.HasPrefix(line, "diff ") || strings.HasPrefix(line, "index "):
		return "diff"
	default:
		return ""
	}
}

func renderDiffLine(line, prefix string) string {
	switch prefix {
	case "+":
		return diffAddStyle.Render(line)
	case "-":
		return diffRemoveStyle.Render(line)
	case "@@":
		return diffHunkStyle.Render(line)
	case "diff":
		return diffHeaderStyle.Render(line)
	default:
		return line
	}
}

func (m Model) renderWorktreesContent(maxLines int) panelContent {
	if len(m.worktrees) == 0 {
		return panelContent{content: normalItemStyle.Render("  (no worktrees)")}
	}
	total := len(m.worktrees)
	focused := m.state.Focus == types.PanelWorktrees
	baseStyle := normalItemStyle
	if focused {
		baseStyle = focusedItemStyle
	}
	var b strings.Builder
	start, end := visibleRange(m.state.WorktreeSel, total, maxLines, 1)
	for i := start; i < end; i++ {
		wt := m.worktrees[i]
		cursor := "  "
		style := baseStyle
		if i == m.state.WorktreeSel {
			cursor = "▸ "
			if focused {
				style = selectedItemStyle
			} else {
				style = dimSelectedItemStyle
			}
		}
		name := filepath.Base(wt.Path)
		currentMark := ""
		if wt.IsCurrent {
			currentMark = activeMarkerStyle.Render(" ●")
		}
		activeMark := ""
		if wt.Path == m.state.ActiveWorktreePath {
			activeMark = activeMarkerStyle.Render(" ◆")
		}
		branchStr := statusBarStyle.Render(" " + wt.Branch)
		b.WriteString(cursor + style.Render(name) + currentMark + activeMark + branchStr + "\n")
	}
	return panelContent{
		content: b.String(),
		scroll:  panel.ScrollInfo{TotalLines: total, VisibleLines: maxLines, ScrollPos: start},
	}
}

func (m Model) renderMinimizedWorktreeBar(width int, currentWT string) string {
	text := fmt.Sprintf("▸ 6 Worktrees (%s)", currentWT)
	style := statusBarStyle
	truncStyle := lipgloss.NewStyle().MaxWidth(width)
	return truncStyle.Render(style.Render(text))
}

func (m Model) renderHelp() string {
	if m.prompt.Active() {
		return m.prompt.RenderHelp()
	}

	common := " · 1-6 panels · ? help · q quit"
	var hidden string
	if m.state.DiffState == types.PanelHidden {
		hidden += " · 4 show diff"
	}
	if m.state.LogState == types.PanelHidden {
		hidden += " · 5 show log"
	}
	switch m.state.Focus {
	case types.PanelChangelists:
		isDirty := len(m.clNames) > 0 && m.state.CLSelected < len(m.clNames) && m.dirtyCLs[m.clNames[m.state.CLSelected]]
		return m.buildFooter(controller.CLBindings, isDirty, hidden+common)
	case types.PanelShelves:
		return helpStyle.Render(" " + controller.FooterText(controller.ShelfBindings, nil) + hidden + common)
	case types.PanelFiles:
		if controller.IsChangelistContext(m.state) {
			hasDirty := false
			for _, f := range m.clFiles {
				if m.dirtyFiles[f] {
					hasDirty = true
					break
				}
			}
			return m.buildFooter(controller.CLFileBindings, hasDirty, hidden+common)
		}
		return helpStyle.Render(" " + controller.FooterText(controller.ShelfFileBindings, nil) + hidden + common)
	case types.PanelDiff:
		return helpStyle.Render(" ↑/↓ scroll · " + controller.FooterText(controller.DiffBindings, nil) + " · 4 maximize/hide" + hidden + common)
	case types.PanelLog:
		return helpStyle.Render(" ↑/↓ scroll · 5 maximize/hide" + hidden + common)
	case types.PanelWorktrees:
		return helpStyle.Render(" ↑/↓ scroll · 6 minimize/hide" + hidden + common)
	}
	return helpStyle.Render(hidden + common)
}

// buildFooter renders a help footer with optional "B accept" in warning style.
func (m Model) buildFooter(bindings []controller.KeyBinding, showDirtyWarning bool, suffix string) string {
	if showDirtyWarning {
		text := controller.FooterText(bindings, map[string]bool{"B": true})
		return helpStyle.Render(" "+text+" · ") + warningStyle.Render("B accept") + helpStyle.Render(suffix)
	}
	return helpStyle.Render(" " + controller.FooterText(bindings, nil) + suffix)
}

func (m Model) renderHelpScreen() string {
	sections := controller.HelpSections()

	var lines []string
	for _, sec := range sections {
		lines = append(lines, "  "+selectedItemStyle.Render(sec.Heading))
		lines = append(lines, "")
		for _, kv := range sec.Keys {
			key := inputLabelStyle.Render(fmt.Sprintf("  %-20s", kv[0]))
			lines = append(lines, key+normalItemStyle.Render(kv[1]))
		}
		lines = append(lines, "")
	}

	// Floating panel dimensions with padding
	pad := 4
	boxW := m.width - pad*2
	boxH := m.height - pad

	contentH := boxH - 2 // subtract box border
	total := len(lines)

	scroll := m.state.HelpScroll
	maxScroll := max(0, total-contentH)
	if scroll > maxScroll {
		scroll = maxScroll
	}
	end := scroll + contentH
	if end > total {
		end = total
	}
	visible := strings.Join(lines[scroll:end], "\n")

	scrollInfo := panel.ScrollInfo{
		TotalLines:   total,
		VisibleLines: contentH,
		ScrollPos:    scroll,
	}
	helpBox := panel.Box(0, "Keyboard Shortcuts", visible, boxW, contentH, true,
		panel.BoxOpts{Scroll: scrollInfo, Accent: selectedAccent})

	footerLeft := helpStyle.Render(" ↑/↓ scroll · mouse wheel · ? or q or esc to close")

	// Right side: d Donate, a Ask Question, r Report Issue, version (keys open browser)
	donateStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	askStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("177"))
	reportStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("167"))
	versionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	footerRight := donateStyle.Render("d Donate") + " " + askStyle.Render("a Ask Question") + " " + reportStyle.Render("r Report Issue") + " " + versionStyle.Render(m.version)

	// Compose footer: left-aligned help, right-aligned links
	rightVisualWidth := lipgloss.Width("d Donate") + 1 + lipgloss.Width("a Ask Question") + 1 + lipgloss.Width("r Report Issue") + 1 + lipgloss.Width(m.version)
	gap := boxW - lipgloss.Width(footerLeft) - rightVisualWidth
	if gap < 1 {
		gap = 1
	}
	footer := footerLeft + strings.Repeat(" ", gap) + footerRight

	// Center with left padding
	padStyle := lipgloss.NewStyle().PaddingLeft(pad).PaddingTop(pad / 2)
	return padStyle.Render(lipgloss.JoinVertical(lipgloss.Left, helpBox, footer))
}

func (m Model) renderLogPanel(maxHeight, maxWidth int) string {
	entries := git.GetLog()
	availLines := maxHeight - 2
	maxW := maxWidth - 4

	var allLines []string
	for _, e := range entries {
		if e.Command == "" {
			// User action message (status/error from actions)
			if e.Error != "" {
				line := truncate(e.Error, maxW)
				allLines = append(allLines, errorStyle.Render(line))
			} else if e.Output != "" {
				line := truncate(e.Output, maxW)
				allLines = append(allLines, warningStyle.Render(line))
			}
		} else {
			// Git command — show indented in white
			cmdLine := truncate("  $ "+e.Command, maxW)
			allLines = append(allLines, normalItemStyle.Render(cmdLine))
			if e.Error != "" {
				for _, line := range strings.Split(e.Error, "\n") {
					line = strings.TrimSpace(line)
					if line != "" {
						allLines = append(allLines, errorStyle.Render(truncate("    "+line, maxW)))
					}
				}
			} else if e.Output != "" {
				for _, line := range strings.Split(e.Output, "\n") {
					line = strings.TrimSpace(line)
					if line != "" {
						allLines = append(allLines, statusBarStyle.Render(truncate("    "+line, maxW)))
					}
				}
			}
		}
	}

	if len(allLines) == 0 {
		allLines = append(allLines, statusBarStyle.Render("  (no commands yet)"))
	}

	maxScroll := max(0, len(allLines)-availLines)
	scroll := min(m.state.LogScroll, maxScroll)
	end := len(allLines) - scroll
	start := max(0, end-availLines)
	visible := allLines[start:end]

	logFocused := m.state.Focus == types.PanelLog
	si := panel.ScrollInfo{TotalLines: len(allLines), VisibleLines: availLines, ScrollPos: start}
	content := strings.Join(visible, "\n")
	return panel.Box(5, "Git Log", content, maxWidth, maxHeight-2, logFocused, panel.BoxOpts{Scroll: si, Accent: panelAccent(logFocused)})
}

// formatShelfTime parses an RFC3339 timestamp and formats it in a short
// locale-aware format with timezone. Detects MM/DD vs DD/MM from LC_TIME/LANG.
func formatShelfTime(rfc3339 string) string {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return ""
	}
	t = t.Local()
	if usesMonthFirst() {
		return t.Format("01/02/2006 15:04")
	}
	return t.Format("02/01/2006 15:04")
}

// usesMonthFirst checks if the locale uses MM/DD date order (US-style).
func usesMonthFirst() bool {
	for _, env := range []string{"LC_TIME", "LC_ALL", "LANG"} {
		v := os.Getenv(env)
		if v == "" {
			continue
		}
		// en_US, en_PH, etc. use month-first
		if strings.HasPrefix(v, "en_US") || strings.HasPrefix(v, "en_PH") {
			return true
		}
		// Any other locale with a value set: day-first
		return false
	}
	// No locale set — default to day/month (international)
	return false
}

func truncate(s string, maxW int) string {
	if len(s) > maxW {
		return s[:maxW]
	}
	return s
}
