package tui

import "testing"

func TestVisibleRange(t *testing.T) {
	tests := []struct {
		name         string
		selected     int
		count        int
		maxLines     int
		linesPerItem int
		wantStart    int
		wantEnd      int
	}{
		{"all visible", 0, 5, 10, 1, 0, 5},
		{"selected in middle", 5, 20, 10, 1, 0, 10},
		{"selected near end", 18, 20, 10, 1, 10, 20},
		{"selected at start", 0, 20, 10, 1, 0, 10},
		{"two lines per item", 3, 10, 10, 2, 1, 6},
		{"zero lines per item defaults to 1", 0, 5, 10, 0, 0, 5},
		{"negative lines per item defaults to 1", 0, 5, 10, -1, 0, 5},
		{"single visible", 5, 20, 1, 1, 5, 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end := VisibleRange(tt.selected, tt.count, tt.maxLines, tt.linesPerItem)
			if start != tt.wantStart || end != tt.wantEnd {
				t.Errorf("VisibleRange(%d, %d, %d, %d) = (%d, %d), want (%d, %d)",
					tt.selected, tt.count, tt.maxLines, tt.linesPerItem,
					start, end, tt.wantStart, tt.wantEnd)
			}
		})
	}
}

func TestClampCursor(t *testing.T) {
	tests := []struct {
		name   string
		cursor int
		count  int
		want   int
	}{
		{"within range", 3, 10, 3},
		{"at zero", 0, 10, 0},
		{"at max", 9, 10, 9},
		{"over max", 15, 10, 9},
		{"negative", -1, 10, 0},
		{"zero count", 5, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClampCursor(tt.cursor, tt.count)
			if got != tt.want {
				t.Errorf("ClampCursor(%d, %d) = %d, want %d", tt.cursor, tt.count, got, tt.want)
			}
		})
	}
}

func TestTabPanel(t *testing.T) {
	tests := []struct {
		name  string
		focus PanelID
		flow  []PanelID
		dir   int
		want  PanelID
	}{
		{"forward from first", 0, []PanelID{0, 2, 3}, 1, 2},
		{"forward from middle", 2, []PanelID{0, 2, 3}, 1, 3},
		{"forward wraps", 3, []PanelID{0, 2, 3}, 1, 0},
		{"backward from first wraps", 0, []PanelID{0, 2, 3}, -1, 3},
		{"backward from middle", 2, []PanelID{0, 2, 3}, -1, 0},
		{"two panels forward", 0, []PanelID{0, 2}, 1, 2},
		{"two panels backward", 0, []PanelID{0, 2}, -1, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TabPanel(tt.focus, tt.flow, tt.dir)
			if got != tt.want {
				t.Errorf("TabPanel(%d, %v, %d) = %d, want %d", tt.focus, tt.flow, tt.dir, got, tt.want)
			}
		})
	}
}
