package panel

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/ignaciotcrespo/gitshelf/internal/types"
	"github.com/ignaciotcrespo/gitshelf/pkg/tui"
)

// Region stores the screen coordinates of a panel for mouse hit-testing.
type Region = tui.Region

// Styles used by panel rendering. Set these from the parent package.
// These proxy to the framework styles.
var (
	ActiveBorderStyle   lipgloss.Style
	InactiveBorderStyle lipgloss.Style
	TitleStyle          lipgloss.Style
	StatusBarStyle      lipgloss.Style
)

// syncStyles copies local styles to the framework.
func syncStyles() {
	tui.ActiveBorderStyle = ActiveBorderStyle
	tui.InactiveBorderStyle = InactiveBorderStyle
	tui.TitleStyle = TitleStyle
	tui.StatusBarStyle = StatusBarStyle
}

// ScrollInfo provides scroll position data for rendering a scrollbar on the right border.
type ScrollInfo = tui.ScrollInfo

// BoxOpts holds optional rendering parameters for Box.
type BoxOpts = tui.BoxOpts

// Box renders a bordered panel with a numbered title in the top border.
func Box(num int, title, content string, width, height int, active bool, opts ...BoxOpts) string {
	syncStyles()
	return tui.Box(num, title, content, width, height, active, opts...)
}

// CyclePanelState cycles a toggleable panel through: normal → maximized → hidden → normal.
// Returns the new state and whether focus should move away (when hidden).
func CyclePanelState(current types.PanelState, focused bool) (types.PanelState, bool) {
	return tui.CyclePanelState(current, focused)
}
