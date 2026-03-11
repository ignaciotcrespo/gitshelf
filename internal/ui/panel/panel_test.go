package panel

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/ignaciotcrespo/gitshelf/internal/types"
)

func init() {
	// Initialize styles needed by Box rendering
	ActiveBorderStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("39"))
	InactiveBorderStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240"))
	TitleStyle = lipgloss.NewStyle().Bold(true)
	StatusBarStyle = lipgloss.NewStyle()
}

func TestBoxDoesNotPanic(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
	}{
		{"zero_dimensions", 0, 0},
		{"negative_width", -5, 10},
		{"negative_height", 10, -5},
		{"both_negative", -1, -1},
		{"width_one", 1, 1},
		{"normal", 40, 10},
		{"very_small", 2, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			result := Box(1, "Test", "content", tt.width, tt.height, false)
			if result == "" {
				t.Error("Box() returned empty string")
			}
		})
	}
}

func TestBoxActiveVsInactive(t *testing.T) {
	active := Box(1, "Panel", "content", 30, 5, true)
	inactive := Box(1, "Panel", "content", 30, 5, false)

	if active == "" {
		t.Error("Box(active=true) returned empty")
	}
	if inactive == "" {
		t.Error("Box(active=false) returned empty")
	}
	// Both should contain the title
	if !strings.Contains(active, "Panel") {
		t.Error("active box should contain title")
	}
	if !strings.Contains(inactive, "Panel") {
		t.Error("inactive box should contain title")
	}
}

func TestBoxContainsTitle(t *testing.T) {
	result := Box(3, "My Panel", "hello", 40, 10, true)
	if !strings.Contains(result, "3") || !strings.Contains(result, "My Panel") {
		t.Errorf("Box() should contain panel number and title, got:\n%s", result)
	}
}

func TestBoxTruncatesContent(t *testing.T) {
	// Create content with many lines
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "line content"
	}
	content := strings.Join(lines, "\n")

	// Box with small height should not have all 50 lines
	result := Box(1, "Test", content, 40, 5, false)
	resultLines := strings.Split(result, "\n")
	// The rendered box should be close to height+2 (borders)
	if len(resultLines) > 10 {
		t.Errorf("Box() should truncate content, got %d lines", len(resultLines))
	}
}

func TestBoxPadding(t *testing.T) {
	// Box with empty content should still render
	result := Box(1, "Empty", "", 30, 5, false)
	if result == "" {
		t.Error("Box() with empty content returned empty")
	}
}

func TestCyclePanelState(t *testing.T) {
	tests := []struct {
		name        string
		current     types.PanelState
		focused     bool
		wantState   types.PanelState
		wantMoveFoc bool
	}{
		{"normal_focused_to_maximized", types.PanelNormal, true, types.PanelMaximized, false},
		{"maximized_focused_to_hidden", types.PanelMaximized, true, types.PanelHidden, true},
		{"hidden_focused_to_normal", types.PanelHidden, true, types.PanelNormal, false},
		{"normal_not_focused", types.PanelNormal, false, types.PanelNormal, false},
		{"maximized_not_focused", types.PanelMaximized, false, types.PanelMaximized, false},
		{"hidden_not_focused_to_normal", types.PanelHidden, false, types.PanelNormal, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotState, gotMove := CyclePanelState(tt.current, tt.focused)
			if gotState != tt.wantState {
				t.Errorf("CyclePanelState(%v, %v) state = %v, want %v", tt.current, tt.focused, gotState, tt.wantState)
			}
			if gotMove != tt.wantMoveFoc {
				t.Errorf("CyclePanelState(%v, %v) moveFocus = %v, want %v", tt.current, tt.focused, gotMove, tt.wantMoveFoc)
			}
		})
	}
}

func TestRegionContains(t *testing.T) {
	r := Region{X: 10, Y: 20, W: 30, H: 15}

	tests := []struct {
		name string
		x, y int
		want bool
	}{
		{"inside", 15, 25, true},
		{"top_left_corner", 10, 20, true},
		{"bottom_right_edge", 39, 34, true},
		{"outside_right", 40, 25, false},
		{"outside_bottom", 15, 35, false},
		{"outside_left", 9, 25, false},
		{"outside_top", 15, 19, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.Contains(tt.x, tt.y)
			if got != tt.want {
				t.Errorf("Region.Contains(%d, %d) = %v, want %v", tt.x, tt.y, got, tt.want)
			}
		})
	}
}

func TestPanelIDConstants(t *testing.T) {
	// Verify the panel IDs are distinct
	ids := []types.PanelID{types.PanelChangelists, types.PanelShelves, types.PanelFiles, types.PanelDiff, types.PanelLog}
	seen := map[types.PanelID]bool{}
	for _, id := range ids {
		if seen[id] {
			t.Errorf("duplicate panel ID: %d", id)
		}
		seen[id] = true
	}
}

func TestStateConstants(t *testing.T) {
	if types.PanelNormal != 0 || types.PanelMaximized != 1 || types.PanelHidden != 2 {
		t.Error("State constants have unexpected values")
	}
}
