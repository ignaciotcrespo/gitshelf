package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Region stores the screen coordinates of a panel for mouse hit-testing.
type Region struct {
	X, Y, W, H int
}

// Contains returns true if the given screen coordinates fall within this region.
func (r Region) Contains(x, y int) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

// Styles used by panel rendering. Set these from the parent package.
var (
	ActiveBorderStyle   lipgloss.Style
	InactiveBorderStyle lipgloss.Style
	TitleStyle          lipgloss.Style
	StatusBarStyle      lipgloss.Style
)

// ScrollInfo provides scroll position data for rendering a scrollbar on the right border.
type ScrollInfo struct {
	TotalLines   int
	VisibleLines int
	ScrollPos    int
}

// BoxOpts holds optional rendering parameters for Box.
type BoxOpts struct {
	Scroll ScrollInfo
	Accent lipgloss.Color // override border/title color (empty = default)
}

// Box renders a bordered panel with a numbered title in the top border.
// This is the single source of truth for panel rendering constraints.
// All content is truncated to fit within width×height, preventing overflow.
func Box(num int, title, content string, width, height int, active bool, opts ...BoxOpts) string {
	style := InactiveBorderStyle
	borderColor := lipgloss.Color("240")
	if active {
		style = ActiveBorderStyle
		borderColor = lipgloss.Color("255")
	}

	// Apply accent color override
	var accent lipgloss.Color
	if len(opts) > 0 && opts[0].Accent != "" {
		accent = opts[0].Accent
		borderColor = accent
		style = style.BorderForeground(accent)
	}

	if height < 1 {
		height = 1
	}
	if width < 1 {
		width = 1
	}

	// Truncate content to fit inside the panel
	maxLines := height
	lines := strings.Split(content, "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	for len(lines) < maxLines {
		lines = append(lines, "")
	}
	// Truncate each line by visual width to prevent lipgloss from wrapping
	truncStyle := lipgloss.NewStyle().MaxWidth(width)
	for i, line := range lines {
		if lipgloss.Width(line) > width {
			lines[i] = truncStyle.Render(line)
		}
	}
	truncated := strings.Join(lines, "\n")

	// Render the box with lipgloss
	rendered := style.
		Width(width).
		Height(height).
		MaxHeight(height + 2).
		Render(truncated)

	// Replace the top border line with one that includes the title
	renderedLines := strings.Split(rendered, "\n")
	if len(renderedLines) > 0 {
		bc := lipgloss.NewStyle().Foreground(borderColor)
		var titleText string
		if num > 0 {
			titleText = fmt.Sprintf(" %d %s ", num, title)
		} else {
			titleText = fmt.Sprintf(" %s ", title)
		}
		var titleRendered string
		if active {
			ts := TitleStyle
			if accent != "" {
				ts = ts.Foreground(accent)
			}
			titleRendered = ts.Render(titleText)
		} else if accent != "" {
			titleRendered = lipgloss.NewStyle().Foreground(accent).Render(titleText)
		} else {
			titleRendered = StatusBarStyle.Render(titleText)
		}

		topWidth := lipgloss.Width(renderedLines[0])
		dashCount := topWidth - lipgloss.Width(titleText) - 2 // -2 for corners
		if dashCount < 0 {
			dashCount = 0
		}
		renderedLines[0] = bc.Render("╭") + titleRendered + bc.Render(strings.Repeat("─", dashCount)+"╮")
	}

	// Apply scrollbar to right border if scroll info provided
	var si ScrollInfo
	if len(opts) > 0 {
		si = opts[0].Scroll
	}
	if si.TotalLines > si.VisibleLines {
		// Content lines are between first (top border) and last (bottom border)
		contentCount := len(renderedLines) - 2
		if contentCount > 0 {
			thumbSize := max(1, contentCount*si.VisibleLines/si.TotalLines)
			maxPos := contentCount - thumbSize
			thumbPos := 0
			if si.TotalLines > si.VisibleLines {
				thumbPos = si.ScrollPos * maxPos / (si.TotalLines - si.VisibleLines)
			}
			if thumbPos > maxPos {
				thumbPos = maxPos
			}

			thumbStyle := lipgloss.NewStyle().Foreground(borderColor)
			borderStr := "│"
			for i := 0; i < contentCount; i++ {
				line := renderedLines[i+1]
				idx := strings.LastIndex(line, borderStr)
				if idx < 0 {
					continue
				}
				if i >= thumbPos && i < thumbPos+thumbSize {
					renderedLines[i+1] = line[:idx] + thumbStyle.Render("█") + line[idx+len(borderStr):]
				}
			}
		}
	}

	return strings.Join(renderedLines, "\n")
}

// CycleWorktreeState cycles the worktrees panel through: normal → minimized → hidden → normal.
// Returns the new state and whether focus should move away (when hidden).
func CycleWorktreeState(current PanelState, focused bool) (PanelState, bool) {
	if !focused {
		if current == PanelHidden {
			return PanelNormal, false
		}
		return current, false
	}
	switch current {
	case PanelNormal:
		return PanelMinimized, true
	case PanelMinimized:
		return PanelHidden, true
	case PanelHidden:
		return PanelNormal, false
	}
	return current, false
}

// CyclePanelState cycles a toggleable panel through: normal → maximized → hidden → normal.
// Returns the new state and whether focus should move away (when hidden).
func CyclePanelState(current PanelState, focused bool) (PanelState, bool) {
	if !focused {
		if current == PanelHidden {
			return PanelNormal, false
		}
		return current, false
	}
	switch current {
	case PanelNormal:
		return PanelMaximized, false
	case PanelMaximized:
		return PanelHidden, true
	case PanelHidden:
		return PanelNormal, false
	}
	return current, false
}
