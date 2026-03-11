package ui

import "github.com/charmbracelet/lipgloss"

// Context accent colors
var (
	selectedAccent = lipgloss.Color("35") // green — focused/selected panel
	contextAccent  = lipgloss.Color("251") // light gray — in-context but not selected
	dimColor       = lipgloss.Color("240") // gray — not in context
)

var (
	// Panel borders (defaults — overridden per-panel by accent colors)
	activeBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("35"))

	inactiveBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(dimColor)

	// Title
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("255"))

	// List items
	selectedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("39")).
				Bold(true)

	dimSelectedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255"))

	normalItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	focusedItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	// Active changelist marker
	activeMarkerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("35"))

	// Diff colors
	diffAddStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("35"))

	diffRemoveStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	diffHeaderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39"))

	diffHunkStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("140"))

	// Status bar
	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	// Input
	inputLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Bold(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("35")).
			Bold(true)

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("220"))

	aheadStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("167")) // subtle red — commits to push

	behindStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("173")) // subtle orange — commits to pull
)
