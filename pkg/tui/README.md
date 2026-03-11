# pkg/tui — Panel-Based TUI Framework

A reusable framework for building panel-based terminal UIs with [Bubbletea](https://github.com/charmbracelet/bubbletea). Extracted from [Gitshelf](https://github.com/ignaciotcrespo/gitshelf).

## What You Get

- **Panel rendering** — bordered boxes with numbered titles, accent colors, and scrollbars
- **Prompt system** — text input and confirmation dialogs with quick-select shortcuts
- **Navigation helpers** — cursor clamping, visible-range windowing, tab cycling, panel state cycling (normal → maximized → hidden)
- **App orchestration** — a Bubbletea model that handles window resize, focus, mouse clicks, prompt lifecycle, and delegates domain keys to your handler
- **Refresh flags** — bitmask system so your loader only reloads what changed

## Quick Start

```go
package main

import (
    "fmt"
    "os"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"

    "github.com/ignaciotcrespo/gitshelf/pkg/tui"
)

// 1. Define your panel IDs and prompt modes as int constants.

const (
    PanelList tui.PanelID = iota
    PanelDetail
    PanelPreview
)

const (
    PromptNone   tui.PromptMode = iota
    PromptAdd
    PromptRename
    PromptConfirm // must match the confirmMode passed to NewPrompt
)

const (
    ConfirmNone   tui.ConfirmAction = iota
    ConfirmDelete
)

// 2. Implement the PromptLabeler interface.

type myLabeler struct{}

func (myLabeler) PromptLabel(mode tui.PromptMode) string {
    switch mode {
    case PromptAdd:
        return "New item name"
    case PromptRename:
        return "Rename"
    }
    return ""
}

func (myLabeler) ConfirmMessage(action tui.ConfirmAction, target string) string {
    switch action {
    case ConfirmDelete:
        return fmt.Sprintf("Delete '%s'", target)
    }
    return ""
}

// 3. Implement PanelRenderer.

type myRenderer struct {
    items []string
    sel   int
}

func (r *myRenderer) RenderPanel(id tui.PanelID, width, height int, focused bool) (string, *tui.ScrollInfo) {
    // Return rendered content and optional scroll info
    return "panel content here", nil
}

func (r *myRenderer) RenderHeader(width int) string {
    return lipgloss.NewStyle().Bold(true).Render(" My App")
}

func (r *myRenderer) RenderHelp(promptActive bool) string {
    if promptActive {
        return " enter confirm · esc cancel"
    }
    return " n new · d delete · q quit"
}

// 4. Implement DataLoader.

type myLoader struct{}

func (myLoader) Refresh(flag tui.RefreshFlag) {
    // Reload data based on flag
}

// 5. Implement Logger.

type myLogger struct{}

func (myLogger) SetStatus(msg string) { /* log status */ }
func (myLogger) SetError(msg string)  { /* log error */ }

// 6. Wire it all together.

func main() {
    renderer := &myRenderer{items: []string{"alpha", "beta", "gamma"}}
    logger := myLogger{}

    config := tui.AppConfig{
        Panels: []tui.PanelDef{
            {ID: PanelList, Title: "Items", Num: 1},
            {ID: PanelDetail, Title: "Detail", Num: 2},
            {ID: PanelPreview, Title: "Preview", Num: 3},
        },
        PivotIDs: []tui.PanelID{PanelList},
        KeyHandler: func(key string, state *tui.AppState) tui.KeyResult {
            switch key {
            case "q", "ctrl+c":
                return tui.KeyResult{Quit: true}
            case "n":
                return tui.KeyResult{
                    Prompt: &tui.PromptReq{Mode: PromptAdd},
                }
            case "d":
                return tui.KeyResult{
                    Prompt: &tui.PromptReq{
                        Confirm: ConfirmDelete,
                        Target:  "current item",
                    },
                }
            }
            return tui.KeyResult{}
        },
        Labeler:  myLabeler{},
        Renderer: renderer,
        Loader:   myLoader{},
    }

    app := tui.NewApp(config, logger)

    // Handle prompt completions
    app.OnPromptResult = func(result *tui.Result) bool {
        switch result.Mode {
        case PromptAdd:
            logger.SetStatus(fmt.Sprintf("Created: %s", result.Value))
        case PromptConfirm:
            if result.Confirmed {
                logger.SetStatus("Deleted")
            }
        }
        return false
    }

    p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
    if _, err := p.Run(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

## Core Types

### Type Aliases

The framework uses plain `int` aliases so consumers define their own constants without importing an enum package:

```go
type PanelID      = int
type PanelState   = int   // PanelNormal (0), PanelMaximized (1), PanelHidden (2)
type PromptMode   = int
type ConfirmAction = int
```

### RefreshFlag

Bitmask telling the loader what data to reload:

```go
tui.RefreshNone       // nothing
tui.RefreshDiff       // reload diff/preview
tui.RefreshCLFiles    // reload file list (implies diff)
tui.RefreshShelfFiles // reload secondary file list (implies diff)
tui.RefreshAll        // reload everything
```

Return the appropriate flag from your `KeyHandler` so the loader does minimal work.

## Panel Rendering

### Box

```go
content := tui.Box(num, title, body, width, height, focused, tui.BoxOpts{
    Scroll: tui.ScrollInfo{TotalLines: 100, VisibleLines: 20, ScrollPos: 5},
    Accent: lipgloss.Color("251"), // override border color
})
```

- Truncates content to fit `width × height` (no overflow)
- Renders a scrollbar on the right border when `TotalLines > VisibleLines`
- Set `Accent` for in-context but unfocused panels (e.g. light gray vs bright white)

Before calling `Box`, initialize the package-level styles:

```go
tui.ActiveBorderStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("255"))
tui.InactiveBorderStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240"))
tui.TitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
tui.StatusBarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
```

### CyclePanelState

```go
newState, shouldMoveFocus := tui.CyclePanelState(currentState, isFocused)
// normal → maximized → hidden → normal
// Returns shouldMoveFocus=true when hiding so you can move focus elsewhere
```

### Region

```go
r := tui.Region{X: 10, Y: 0, W: 40, H: 20}
if r.Contains(mouseX, mouseY) {
    // clicked inside this panel
}
```

## Navigation Helpers

### VisibleRange

Windowed scrolling that keeps the selected item visible:

```go
start, end := tui.VisibleRange(selectedIndex, totalItems, maxVisibleLines, linesPerItem)
for i := start; i < end; i++ {
    // render item i
}
```

### ClampCursor

```go
cursor = tui.ClampCursor(cursor, len(items)) // keeps cursor in [0, len-1]
```

### TabPanel

Cycle focus through a list of panel IDs:

```go
flow := []tui.PanelID{PanelList, PanelDetail, PanelPreview}
nextFocus := tui.TabPanel(currentFocus, flow, +1) // forward
prevFocus := tui.TabPanel(currentFocus, flow, -1) // backward
```

## Prompt System

### Setup

```go
prompt := tui.NewPrompt(myLabeler{}, PromptConfirm)
```

The second argument is the `PromptMode` value your app uses for confirmations. The prompt uses this to distinguish confirmation dialogs from text input.

### Text Input

```go
cmd := prompt.Start(PromptAdd, "default value")
// or with quick-select options:
cmd := prompt.StartWithOptions(PromptRename, "current", []string{"Option A", "Option B"})
```

Quick-select assigns a unique letter shortcut to each option (e.g. `[O]ption A [p]tion B`). Pressing the letter picks that option immediately.

### Confirmation

```go
prompt.StartConfirm(ConfirmDelete, "item name")
// Shows: "Delete 'item name'? (y/n)"
// Message text comes from your PromptLabeler.ConfirmMessage()
```

### Handling Results

```go
result, handled, cmd := prompt.HandleKey(keyMsg)
if result != nil {
    // result.Mode tells you which prompt completed
    // result.Value has the text (for input prompts)
    // result.Confirmed is true/false (for confirmations)
    // result.ConfirmAction and result.ConfirmTarget echo back what you passed
}
```

### Prompt Styles

Initialize before first render:

```go
tui.InputLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
tui.ErrorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
tui.HelpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
```

## App Model

`tui.App` is a ready-made Bubbletea model. It handles:

- `tea.WindowSizeMsg` — stores width/height
- `tea.FocusMsg` — triggers `RefreshAll`
- `tea.MouseMsg` — hit-tests panel regions, updates focus/pivot
- Prompt lifecycle — active prompt intercepts keys
- Everything else → your `KeyHandler`

### AppConfig

```go
config := tui.AppConfig{
    Panels:     []tui.PanelDef{...},
    PivotIDs:   []tui.PanelID{...},     // panels that switch context (e.g. list vs tree)
    KeyHandler: func(key string, state *tui.AppState) tui.KeyResult { ... },
    Labeler:    myLabeler{},
    Renderer:   myRenderer{},
    Loader:     myLoader{},
}
```

### AppState

The framework manages generic navigation state:

```go
type AppState struct {
    Focus       PanelID
    Pivot       PanelID
    Cursors     map[PanelID]int
    Selections  map[string]bool
    PanelStates map[PanelID]PanelState
    Scrolls     map[PanelID]int
    HScrolls    map[PanelID]int
    Custom      any  // your domain state goes here
}
```

Put domain-specific state in `Custom` and type-assert in your `KeyHandler`/`Renderer`:

```go
type myState struct {
    WrapMode bool
    Filter   string
}

// In KeyHandler:
custom := state.Custom.(*myState)
custom.WrapMode = !custom.WrapMode
```

### Custom View Layout

The default `App.View()` is minimal (header + prompt + help). For custom layouts, embed `tui.App` in your own model and override `View()`:

```go
type MyModel struct {
    tui.App
    // your fields
}

func (m MyModel) View() string {
    // Use m.App.Width, m.App.Height for dimensions
    // Use m.App.Config.Renderer.RenderPanel(...) for each panel
    // Use tui.Box(...) for bordered panels
    // Use m.App.Prompt.Render() for the prompt bar
    // Use m.App.Config.Renderer.RenderHelp(...) for the help bar
    // Record m.App.PanelRegions for mouse click detection
    return lipgloss.JoinVertical(lipgloss.Left, header, panels, help)
}
```

## Design Principles

- **No generics** — `int` aliases + `any` for custom state keeps it simple
- **No circular deps** — the framework has zero knowledge of your domain
- **Pure functions** — `VisibleRange`, `ClampCursor`, `TabPanel`, `CyclePanelState` are all stateless
- **Styles are package vars** — set them once at init, the framework uses them everywhere
- **Consumer owns layout** — the framework provides building blocks, not a rigid grid
