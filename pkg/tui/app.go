package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// App is the generic Bubbletea model for a panel-based TUI.
// It can be used as a standalone tea.Model or as a helper embedded in a consumer model.
type App struct {
	Config AppConfig
	State  AppState
	Prompt Prompt

	Width  int
	Height int

	// PanelRegions stores screen coordinates for mouse hit-testing.
	PanelRegions map[PanelID]Region

	// OnPromptResult is called when a prompt completes.
	// Returns true if a follow-up confirm was started (caller should return early).
	OnPromptResult func(result *Result) bool

	logger Logger
}

// NewApp creates a framework App with the given config.
func NewApp(config AppConfig, logger Logger) App {
	state := AppState{
		Cursors:     make(map[PanelID]int),
		Selections:  make(map[string]bool),
		PanelStates: make(map[PanelID]PanelState),
		Scrolls:     make(map[PanelID]int),
		HScrolls:    make(map[PanelID]int),
	}

	// Initialize focus/pivot from panel definitions
	for _, p := range config.Panels {
		state.PanelStates[p.ID] = PanelNormal
		if state.Focus == 0 && p.Num == 1 {
			state.Focus = p.ID
		}
		if state.Pivot == 0 && p.Pivot {
			state.Pivot = p.ID
		}
	}
	if len(config.Panels) > 0 && state.Focus == 0 {
		state.Focus = config.Panels[0].ID
	}

	return App{
		Config:       config,
		State:        state,
		Prompt:       NewPrompt(config.Labeler, 0),
		PanelRegions: make(map[PanelID]Region),
		logger:       logger,
	}
}

// HandleUniversalKey processes universal keys (quit, tab, panel numbers).
// Returns (handled, quit, refresh). If handled is true, the key was consumed.
func (a *App) HandleUniversalKey(key string) (handled bool, quit bool, refresh RefreshFlag) {
	switch key {
	case "q", "ctrl+c":
		return true, true, RefreshNone

	case "tab":
		if a.Config.TabFlow != nil {
			flow := a.Config.TabFlow(a.State.Focus, a.State.Pivot, a.State.PanelStates)
			a.State.Focus = TabPanel(a.State.Focus, flow, 1)
		}
		return true, false, RefreshAll

	case "shift+tab":
		if a.Config.TabFlow != nil {
			flow := a.Config.TabFlow(a.State.Focus, a.State.Pivot, a.State.PanelStates)
			a.State.Focus = TabPanel(a.State.Focus, flow, -1)
		}
		return true, false, RefreshAll

	default:
		// Number keys → panel focus/pivot/toggle
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
			num := int(key[0] - '0')
			for _, p := range a.Config.Panels {
				if p.Num == num {
					return a.handlePanelKey(p)
				}
			}
		}
	}
	return false, false, RefreshNone
}

func (a *App) handlePanelKey(p PanelDef) (bool, bool, RefreshFlag) {
	if p.Pivot {
		a.State.Focus = p.ID
		a.State.Pivot = p.ID
		return true, false, RefreshAll
	}
	if p.Toggle {
		focused := a.State.Focus == p.ID
		newState, moveFocus := CyclePanelState(a.State.PanelStates[p.ID], focused)
		a.State.PanelStates[p.ID] = newState
		if moveFocus {
			a.State.Focus = a.State.Pivot
		} else if !focused {
			a.State.Focus = p.ID
			return true, false, RefreshAll
		}
		return true, false, RefreshNone
	}
	// Simple panel: just focus
	a.State.Focus = p.ID
	return true, false, RefreshAll
}

// HandleMouse processes a mouse event for panel focus switching.
// Returns (handled, refresh).
func (a *App) HandleMouse(msg tea.MouseMsg) (bool, RefreshFlag) {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return false, RefreshNone
	}
	for pid, region := range a.PanelRegions {
		if region.Contains(msg.X, msg.Y) {
			if a.State.Focus != pid {
				a.State.Focus = pid
				for _, p := range a.Config.Panels {
					if p.ID == pid && p.Pivot {
						a.State.Pivot = pid
						break
					}
				}
				return true, RefreshAll
			}
			return true, RefreshNone
		}
	}
	return false, RefreshNone
}

// --- tea.Model implementation (for standalone use) ---

// Init implements tea.Model.
func (a App) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.Width = msg.Width
		a.Height = msg.Height
		return a, nil

	case tea.FocusMsg:
		if a.Config.Loader != nil {
			a.Config.Loader.Refresh(RefreshAll)
		}
		return a, nil

	case tea.MouseMsg:
		if handled, refresh := a.HandleMouse(msg); handled {
			if refresh != 0 && a.Config.Loader != nil {
				a.Config.Loader.Refresh(refresh)
			}
		}
		return a, nil

	case tea.KeyMsg:
		// Prompt handling takes priority
		if a.Prompt.Active() {
			result, handled, cmd := a.Prompt.HandleKey(msg)
			if handled {
				if result != nil && a.OnPromptResult != nil {
					a.OnPromptResult(result)
				}
				return a, cmd
			}
		}

		// Universal keys
		if handled, quit, refresh := a.HandleUniversalKey(msg.String()); handled {
			if quit {
				return a, tea.Quit
			}
			if refresh != 0 && a.Config.Loader != nil {
				a.Config.Loader.Refresh(refresh)
			}
			return a, nil
		}

		// Delegate to consumer key handler
		if a.Config.KeyHandler != nil {
			kr := a.Config.KeyHandler(msg.String(), &a.State)

			if kr.Quit {
				return a, tea.Quit
			}
			if kr.StatusMsg != "" && a.logger != nil {
				a.logger.SetStatus(kr.StatusMsg)
			}
			if kr.ErrorMsg != "" && a.logger != nil {
				a.logger.SetError(kr.ErrorMsg)
			}
			if kr.Prompt != nil {
				cmd := a.startPrompt(kr.Prompt)
				if a.Config.Loader != nil {
					a.Config.Loader.Refresh(kr.Refresh)
				}
				return a, cmd
			}
			if a.Config.Loader != nil {
				a.Config.Loader.Refresh(kr.Refresh)
			}
		}
		return a, nil
	}

	// Forward non-key messages to prompt
	if a.Prompt.Active() {
		cmd := a.Prompt.Update(msg)
		return a, cmd
	}

	return a, nil
}

// View implements tea.Model.
func (a App) View() string {
	if a.Width == 0 {
		return "Loading..."
	}

	header := a.Config.Renderer.RenderHeader(a.Width)

	var parts []string
	parts = append(parts, header)

	if a.Prompt.Active() {
		parts = append(parts, a.Prompt.Render())
	}
	parts = append(parts, a.Config.Renderer.RenderHelp(a.Prompt.Active()))

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (a *App) startPrompt(req *PromptReq) tea.Cmd {
	if req.Confirm != 0 {
		a.Prompt.StartConfirm(req.Confirm, req.Target)
		return nil
	} else if len(req.Options) > 0 {
		return a.Prompt.StartWithOptions(req.Mode, req.DefaultValue, req.Options)
	}
	return a.Prompt.Start(req.Mode, req.DefaultValue)
}
