package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func init() {
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
			result := Box(1, "Test", "content", tt.width, tt.height, false)
			if result == "" {
				t.Error("Box() returned empty string")
			}
		})
	}
}

func TestBoxContainsTitle(t *testing.T) {
	result := Box(3, "My Panel", "hello", 40, 10, true)
	if !strings.Contains(result, "3") || !strings.Contains(result, "My Panel") {
		t.Errorf("Box() should contain panel number and title")
	}
}

func TestCyclePanelState(t *testing.T) {
	tests := []struct {
		name        string
		current     PanelState
		focused     bool
		wantState   PanelState
		wantMoveFoc bool
	}{
		{"normal_focused_to_maximized", PanelNormal, true, PanelMaximized, false},
		{"maximized_focused_to_hidden", PanelMaximized, true, PanelHidden, true},
		{"hidden_focused_to_normal", PanelHidden, true, PanelNormal, false},
		{"normal_not_focused", PanelNormal, false, PanelNormal, false},
		{"maximized_not_focused", PanelMaximized, false, PanelMaximized, false},
		{"hidden_not_focused_to_normal", PanelHidden, false, PanelNormal, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotState, gotMove := CyclePanelState(tt.current, tt.focused)
			if gotState != tt.wantState {
				t.Errorf("state = %v, want %v", gotState, tt.wantState)
			}
			if gotMove != tt.wantMoveFoc {
				t.Errorf("moveFocus = %v, want %v", gotMove, tt.wantMoveFoc)
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

func TestCycleWorktreeState(t *testing.T) {
	tests := []struct {
		name        string
		current     PanelState
		focused     bool
		wantState   PanelState
		wantMoveFoc bool
	}{
		{"normal_focused_to_minimized", PanelNormal, true, PanelMinimized, true},
		{"minimized_focused_to_hidden", PanelMinimized, true, PanelHidden, true},
		{"hidden_focused_to_normal", PanelHidden, true, PanelNormal, false},
		{"normal_not_focused", PanelNormal, false, PanelNormal, false},
		{"minimized_not_focused", PanelMinimized, false, PanelMinimized, false},
		{"hidden_not_focused_to_normal", PanelHidden, false, PanelNormal, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotState, gotMove := CycleWorktreeState(tt.current, tt.focused)
			if gotState != tt.wantState {
				t.Errorf("state = %v, want %v", gotState, tt.wantState)
			}
			if gotMove != tt.wantMoveFoc {
				t.Errorf("moveFocus = %v, want %v", gotMove, tt.wantMoveFoc)
			}
		})
	}
}
