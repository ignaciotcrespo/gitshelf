package tui

// PanelDef defines a panel in the layout.
type PanelDef struct {
	ID     PanelID
	Title  string
	Num    int  // number key shortcut (1-9)
	Pivot  bool // pressing number also sets this as pivot
	Toggle bool // pressing number cycles Normal → Maximized → Hidden when focused
}

// AppConfig wires consumer logic into the framework.
type AppConfig struct {
	Panels     []PanelDef
	TabFlow    func(focus, pivot PanelID, panelStates map[PanelID]PanelState) []PanelID
	KeyHandler func(key string, state *AppState) KeyResult // domain key handling
	Labeler    PromptLabeler
	Renderer   PanelRenderer
	Loader     DataLoader
}

// AppState is framework-owned generic state.
type AppState struct {
	Focus       PanelID
	Pivot       PanelID
	Cursors     map[PanelID]int        // cursor position per panel
	Selections  map[string]bool        // selected items (generic keys)
	PanelStates map[PanelID]PanelState // Normal/Maximized/Hidden per panel
	Scrolls     map[PanelID]int        // vertical scroll per panel
	HScrolls    map[PanelID]int        // horizontal scroll per panel
	Custom      any                    // consumer-specific state
}

// KeyResult is the output of the consumer's key handler.
type KeyResult struct {
	Refresh   RefreshFlag
	Prompt    *PromptReq
	StatusMsg string
	ErrorMsg  string
	Quit      bool
}

// PromptReq describes a prompt the coordinator should start.
type PromptReq struct {
	Mode         PromptMode
	DefaultValue string
	Confirm      ConfirmAction
	Target       string
	Options      []string
}

// PanelRenderer renders panel content and help bar.
type PanelRenderer interface {
	RenderPanel(id PanelID, width, height int, focused bool) (content string, scroll *ScrollInfo)
	RenderHeader(width int) string
	RenderHelp(promptActive bool) string
}

// DataLoader refreshes data based on refresh flags.
type DataLoader interface {
	Refresh(flag RefreshFlag)
}

// Logger provides status/error reporting.
type Logger interface {
	SetStatus(msg string)
	SetError(msg string)
}
