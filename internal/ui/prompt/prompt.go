package prompt

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ignaciotcrespo/gitshelf/internal/types"
	"github.com/ignaciotcrespo/gitshelf/pkg/tui"
)

// Styles used by prompt rendering. Set these from the parent package.
var (
	InputLabelStyle lipgloss.Style
	ErrorStyle      lipgloss.Style
	HelpStyle       lipgloss.Style
)

// syncStyles copies local styles to the framework.
func syncStyles() {
	tui.InputLabelStyle = InputLabelStyle
	tui.ErrorStyle = ErrorStyle
	tui.HelpStyle = HelpStyle
}

// QuickOption is a named option with a shortcut key.
type QuickOption = tui.QuickOption

// Result represents the outcome of a completed prompt.
type Result = tui.Result

// gitshelfLabeler implements tui.PromptLabeler for gitshelf-specific prompts.
type gitshelfLabeler struct{}

func (g gitshelfLabeler) PromptLabel(mode types.PromptMode) string {
	switch mode {
	case types.PromptNewChangelist:
		return "New changelist name"
	case types.PromptRenameChangelist:
		return "Rename changelist"
	case types.PromptShelveFiles:
		return "Shelf name"
	case types.PromptRenameShelf:
		return "Rename shelf"
	case types.PromptMoveFile:
		return "Move to changelist"
	case types.PromptUnshelve:
		return "Unshelve to changelist"
	case types.PromptCommit:
		return "Commit message"
	case types.PromptAmend:
		return "Amend commit message"
	case types.PromptPush:
		return "Push to remote"
	case types.PromptPull:
		return "Pull from remote"
	case types.PromptPasteChangelist:
		return "Paste mode"
	}
	return ""
}

func (g gitshelfLabeler) ConfirmMessage(action types.ConfirmAction, target string) string {
	switch action {
	case types.ConfirmDeleteChangelist:
		return fmt.Sprintf("Delete changelist '%s'", target)
	case types.ConfirmDropShelf:
		return fmt.Sprintf("Drop shelf '%s'", target)
	case types.ConfirmAcceptDirty:
		return formatAcceptDirtyMessage(target)
	case types.ConfirmUnshelve:
		return formatUnshelveMessage(target)
	case types.ConfirmPasteFullContent:
		return fmt.Sprintf("Overwrite %s files in working tree?", target)
	case types.ConfirmSnapshotUnshelve:
		return "Unshelve all shelves in this group?"
	}
	return ""
}

// Prompt holds the current input/confirmation state.
type Prompt struct {
	inner tui.Prompt
}

// Mode returns the current prompt mode.
func (p *Prompt) Mode() types.PromptMode {
	return p.inner.Mode
}

// ConfirmAction returns the current confirm action.
func (p *Prompt) ConfirmAction() types.ConfirmAction {
	return p.inner.ConfirmAction
}

// ConfirmTarget returns the current confirm target.
func (p *Prompt) ConfirmTarget() string {
	return p.inner.ConfirmTarget
}

// Active returns true if a prompt is currently showing.
func (p *Prompt) Active() bool {
	return p.inner.Active()
}

// Value returns the current text input value.
func (p *Prompt) Value() string {
	return p.inner.Value()
}

// Start begins a new input prompt with the given mode and default value.
func (p *Prompt) Start(mode types.PromptMode, defaultValue string) tea.Cmd {
	p.ensureInit()
	syncStyles()
	return p.inner.Start(mode, defaultValue)
}

// StartWithOptions begins a prompt with quick-select options.
func (p *Prompt) StartWithOptions(mode types.PromptMode, defaultValue string, names []string) tea.Cmd {
	p.ensureInit()
	syncStyles()
	return p.inner.StartWithOptions(mode, defaultValue, names)
}

// StartConfirm begins a confirmation prompt.
func (p *Prompt) StartConfirm(action types.ConfirmAction, target string) {
	p.ensureInit()
	syncStyles()
	p.inner.StartConfirm(action, target)
}

// Cancel dismisses the current prompt.
func (p *Prompt) Cancel() {
	p.inner.Cancel()
}

// HandleKey processes a key event for the active prompt.
func (p *Prompt) HandleKey(msg tea.KeyMsg) (*Result, bool, tea.Cmd) {
	return p.inner.HandleKey(msg)
}

// Update processes non-key messages (e.g. cursor blink) for the textinput.
func (p *Prompt) Update(msg tea.Msg) tea.Cmd {
	return p.inner.Update(msg)
}

// Render returns the prompt bar string.
func (p *Prompt) Render() string {
	syncStyles()
	return p.inner.Render()
}

// RenderHelp returns help text for the active prompt.
func (p *Prompt) RenderHelp() string {
	syncStyles()
	return p.inner.RenderHelp()
}

func (p *Prompt) ensureInit() {
	if p.inner.Labeler == nil {
		p.inner = tui.NewPrompt(gitshelfLabeler{}, types.PromptConfirm)
	}
}

// formatAcceptDirtyMessage builds the confirm message for accept dirty.
// Target format: "cl:<name>:<count>" or "files:<clName>:<count>"
func formatAcceptDirtyMessage(target string) string {
	parts := strings.SplitN(target, ":", 3)
	if len(parts) != 3 {
		return "Accept dirty changes"
	}
	kind, name, count := parts[0], parts[1], parts[2]
	if kind == "cl" {
		return fmt.Sprintf("Accept all %s dirty file(s) in '%s' as baseline", count, name)
	}
	return fmt.Sprintf("Accept %s dirty file(s) in '%s' as baseline", count, name)
}

// formatUnshelveMessage builds the confirm message for unshelve.
// Target format: "<shelfName>:<totalFiles>:<conflicting>"
func formatUnshelveMessage(target string) string {
	parts := strings.SplitN(target, ":", 3)
	if len(parts) != 3 {
		return "Unshelve files"
	}
	return fmt.Sprintf("Unshelve '%s' — %s conflicting file(s) will be backed up to '~backup-%s'", parts[0], parts[2], parts[0])
}
