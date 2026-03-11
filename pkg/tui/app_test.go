package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	panelList    PanelID = 0
	panelShelves PanelID = 1
	panelFiles   PanelID = 2
	panelDiff    PanelID = 3
	panelLog     PanelID = 4
)

func testApp() *App {
	config := AppConfig{
		Panels: []PanelDef{
			{ID: panelList, Title: "List", Num: 1, Pivot: true},
			{ID: panelShelves, Title: "Shelves", Num: 2, Pivot: true},
			{ID: panelFiles, Title: "Files", Num: 3},
			{ID: panelDiff, Title: "Diff", Num: 4, Toggle: true},
			{ID: panelLog, Title: "Log", Num: 5, Toggle: true},
		},
		TabFlow: func(focus, pivot PanelID, panelStates map[PanelID]PanelState) []PanelID {
			flow := []PanelID{pivot, panelFiles}
			if panelStates[panelDiff] != PanelHidden {
				flow = append(flow, panelDiff)
			}
			return flow
		},
	}
	app := NewApp(config, nil)
	// Set initial state matching gitshelf defaults
	app.State.Focus = panelList
	app.State.Pivot = panelList
	return &app
}

func TestHandleUniversalKey_Quit(t *testing.T) {
	app := testApp()

	for _, key := range []string{"q", "ctrl+c"} {
		handled, quit, _ := app.HandleUniversalKey(key)
		if !handled {
			t.Errorf("%s: expected handled", key)
		}
		if !quit {
			t.Errorf("%s: expected quit", key)
		}
	}
}

func TestHandleUniversalKey_Tab(t *testing.T) {
	app := testApp()
	app.State.Focus = panelList

	handled, _, refresh := app.HandleUniversalKey("tab")
	if !handled {
		t.Fatal("expected handled")
	}
	if app.State.Focus != panelFiles {
		t.Errorf("expected focus on Files, got %d", app.State.Focus)
	}
	if refresh != RefreshAll {
		t.Errorf("expected RefreshAll, got %d", refresh)
	}
}

func TestHandleUniversalKey_ShiftTab(t *testing.T) {
	app := testApp()
	app.State.Focus = panelFiles
	app.State.Pivot = panelList

	handled, _, refresh := app.HandleUniversalKey("shift+tab")
	if !handled {
		t.Fatal("expected handled")
	}
	if app.State.Focus != panelList {
		t.Errorf("expected focus on List, got %d", app.State.Focus)
	}
	if refresh != RefreshAll {
		t.Errorf("expected RefreshAll, got %d", refresh)
	}
}

func TestHandleUniversalKey_TabDiffHidden(t *testing.T) {
	app := testApp()
	app.State.Focus = panelFiles
	app.State.Pivot = panelList
	app.State.PanelStates[panelDiff] = PanelHidden

	app.HandleUniversalKey("tab")
	// Flow: [List, Files] (no Diff since hidden) → from Files → wraps to List
	if app.State.Focus != panelList {
		t.Errorf("expected focus on List when diff hidden, got %d", app.State.Focus)
	}
}

func TestHandleUniversalKey_PivotPanel(t *testing.T) {
	app := testApp()

	// "1" → focus + pivot to List
	handled, _, refresh := app.HandleUniversalKey("1")
	if !handled {
		t.Fatal("expected handled")
	}
	if app.State.Focus != panelList || app.State.Pivot != panelList {
		t.Errorf("expected focus+pivot on List")
	}
	if refresh != RefreshAll {
		t.Errorf("expected RefreshAll")
	}

	// "2" → focus + pivot to Shelves
	app.HandleUniversalKey("2")
	if app.State.Focus != panelShelves || app.State.Pivot != panelShelves {
		t.Errorf("expected focus+pivot on Shelves")
	}
}

func TestHandleUniversalKey_SimplePanel(t *testing.T) {
	app := testApp()
	app.State.Pivot = panelList

	// "3" → focus on Files, pivot unchanged
	app.HandleUniversalKey("3")
	if app.State.Focus != panelFiles {
		t.Errorf("expected focus on Files, got %d", app.State.Focus)
	}
	if app.State.Pivot != panelList {
		t.Errorf("pivot should not change, got %d", app.State.Pivot)
	}
}

func TestHandleUniversalKey_TogglePanel_Focus(t *testing.T) {
	app := testApp()

	// "4" when not focused on Diff → focus on Diff
	handled, _, refresh := app.HandleUniversalKey("4")
	if !handled {
		t.Fatal("expected handled")
	}
	if app.State.Focus != panelDiff {
		t.Errorf("expected focus on Diff, got %d", app.State.Focus)
	}
	if refresh != RefreshAll {
		t.Errorf("expected RefreshAll when focusing")
	}
}

func TestHandleUniversalKey_TogglePanel_Cycle(t *testing.T) {
	app := testApp()
	app.State.Focus = panelDiff

	// "4" when focused: Normal → Maximized
	app.HandleUniversalKey("4")
	if app.State.PanelStates[panelDiff] != PanelMaximized {
		t.Errorf("expected Maximized, got %d", app.State.PanelStates[panelDiff])
	}

	// "4" again: Maximized → Hidden, focus moves to pivot
	app.HandleUniversalKey("4")
	if app.State.PanelStates[panelDiff] != PanelHidden {
		t.Errorf("expected Hidden, got %d", app.State.PanelStates[panelDiff])
	}
	if app.State.Focus != app.State.Pivot {
		t.Errorf("expected focus to move to pivot")
	}
}

func TestHandleUniversalKey_TogglePanel_Unhide(t *testing.T) {
	app := testApp()
	app.State.PanelStates[panelDiff] = PanelHidden
	app.State.Focus = panelList

	// "4" when hidden and not focused → show (Normal) + focus
	app.HandleUniversalKey("4")
	if app.State.PanelStates[panelDiff] != PanelNormal {
		t.Errorf("expected Normal, got %d", app.State.PanelStates[panelDiff])
	}
	if app.State.Focus != panelDiff {
		t.Errorf("expected focus on Diff, got %d", app.State.Focus)
	}
}

func TestHandleUniversalKey_LogToggle(t *testing.T) {
	app := testApp()

	// "5" not focused → focus on Log
	app.HandleUniversalKey("5")
	if app.State.Focus != panelLog {
		t.Errorf("expected focus on Log, got %d", app.State.Focus)
	}

	// "5" focused: Normal → Maximized
	app.HandleUniversalKey("5")
	if app.State.PanelStates[panelLog] != PanelMaximized {
		t.Errorf("expected Maximized, got %d", app.State.PanelStates[panelLog])
	}

	// "5" focused: Maximized → Hidden, focus to pivot
	app.HandleUniversalKey("5")
	if app.State.PanelStates[panelLog] != PanelHidden {
		t.Errorf("expected Hidden, got %d", app.State.PanelStates[panelLog])
	}
	if app.State.Focus != app.State.Pivot {
		t.Errorf("expected focus to move to pivot")
	}
}

func TestHandleUniversalKey_NonUniversal(t *testing.T) {
	app := testApp()
	handled, _, _ := app.HandleUniversalKey("n")
	if handled {
		t.Error("'n' should not be handled as universal key")
	}
}

func TestHandleMouse_ClickPanel(t *testing.T) {
	app := testApp()
	app.PanelRegions[panelList] = Region{X: 0, Y: 0, W: 20, H: 10}
	app.PanelRegions[panelShelves] = Region{X: 0, Y: 10, W: 20, H: 10}
	app.PanelRegions[panelFiles] = Region{X: 20, Y: 0, W: 20, H: 20}
	app.State.Focus = panelList

	msg := tea.MouseMsg{X: 25, Y: 5, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	handled, refresh := app.HandleMouse(msg)
	if !handled {
		t.Fatal("expected handled")
	}
	if app.State.Focus != panelFiles {
		t.Errorf("expected focus on Files, got %d", app.State.Focus)
	}
	if refresh != RefreshAll {
		t.Errorf("expected RefreshAll")
	}
}

func TestHandleMouse_ClickPivotPanel(t *testing.T) {
	app := testApp()
	app.PanelRegions[panelList] = Region{X: 0, Y: 0, W: 20, H: 10}
	app.PanelRegions[panelShelves] = Region{X: 0, Y: 10, W: 20, H: 10}
	app.State.Focus = panelList
	app.State.Pivot = panelList

	// Click on Shelves (pivot panel)
	msg := tea.MouseMsg{X: 5, Y: 15, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	app.HandleMouse(msg)
	if app.State.Focus != panelShelves {
		t.Errorf("expected focus on Shelves, got %d", app.State.Focus)
	}
	if app.State.Pivot != panelShelves {
		t.Errorf("expected pivot on Shelves, got %d", app.State.Pivot)
	}
}

func TestHandleMouse_ClickSamePanel(t *testing.T) {
	app := testApp()
	app.PanelRegions[panelList] = Region{X: 0, Y: 0, W: 20, H: 10}
	app.State.Focus = panelList

	msg := tea.MouseMsg{X: 5, Y: 5, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	handled, refresh := app.HandleMouse(msg)
	if !handled {
		t.Fatal("expected handled")
	}
	if refresh != RefreshNone {
		t.Errorf("clicking same panel should not refresh, got %d", refresh)
	}
}

func TestHandleMouse_RightClick(t *testing.T) {
	app := testApp()
	app.PanelRegions[panelList] = Region{X: 0, Y: 0, W: 20, H: 10}

	msg := tea.MouseMsg{X: 5, Y: 5, Action: tea.MouseActionPress, Button: tea.MouseButtonRight}
	handled, _ := app.HandleMouse(msg)
	if handled {
		t.Error("right click should not be handled")
	}
}
