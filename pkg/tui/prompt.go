package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Styles used by prompt rendering. Set these from the parent package.
var (
	InputLabelStyle lipgloss.Style
	ErrorStyle      lipgloss.Style
	HelpStyle       lipgloss.Style
)

// PromptLabeler provides display text for prompt modes and confirm actions.
type PromptLabeler interface {
	PromptLabel(mode PromptMode) string
	ConfirmMessage(action ConfirmAction, target string) string
}

// QuickOption is a named option with a shortcut key.
type QuickOption struct {
	Key  rune
	Name string
}

// Prompt holds the current input/confirmation state.
type Prompt struct {
	Mode          PromptMode
	Input         textinput.Model
	ConfirmAction ConfirmAction
	ConfirmTarget string
	Options       []QuickOption
	Labeler       PromptLabeler
	confirmMode   PromptMode // the PromptMode value that means "confirm"
}

// NewPrompt creates a Prompt configured with a labeler and the consumer's confirm mode value.
func NewPrompt(labeler PromptLabeler, confirmMode PromptMode) Prompt {
	return Prompt{
		Labeler:     labeler,
		confirmMode: confirmMode,
	}
}

// Active returns true if a prompt is currently showing.
func (p *Prompt) Active() bool {
	return p.Mode != 0
}

// Value returns the current text input value.
func (p *Prompt) Value() string {
	return p.Input.Value()
}

func (p *Prompt) newInput(defaultValue string) tea.Cmd {
	p.Input = textinput.New()
	p.Input.Prompt = ""
	p.Input.SetValue(defaultValue)
	p.Input.CursorEnd()
	return p.Input.Focus()
}

// Start begins a new input prompt with the given mode and default value.
func (p *Prompt) Start(mode PromptMode, defaultValue string) tea.Cmd {
	p.Mode = mode
	p.Options = nil
	return p.newInput(defaultValue)
}

// StartWithOptions begins a prompt with quick-select options.
// Each option gets a unique shortcut key assigned from its name.
func (p *Prompt) StartWithOptions(mode PromptMode, defaultValue string, names []string) tea.Cmd {
	p.Mode = mode
	p.Options = AssignShortcuts(names)
	return p.newInput(defaultValue)
}

// StartConfirm begins a confirmation prompt.
func (p *Prompt) StartConfirm(action ConfirmAction, target string) {
	p.Mode = p.confirmMode
	p.ConfirmAction = action
	p.ConfirmTarget = target
}

// Cancel dismisses the current prompt.
func (p *Prompt) Cancel() {
	p.Mode = 0
	p.Input.Blur()
	p.ConfirmAction = 0
	p.ConfirmTarget = ""
	p.Options = nil
}

// Result represents the outcome of a completed prompt.
type Result struct {
	Mode          PromptMode
	Value         string
	Confirmed     bool // for Confirm mode: true=yes, false=cancelled
	ConfirmAction ConfirmAction
	ConfirmTarget string
}

// HandleKey processes a key event for the active prompt.
// Returns (result, handled, cmd). If result is non-nil, the prompt completed.
func (p *Prompt) HandleKey(msg tea.KeyMsg) (*Result, bool, tea.Cmd) {
	if !p.Active() {
		return nil, false, nil
	}

	// Confirmation mode
	if p.Mode == p.confirmMode {
		switch msg.String() {
		case "y", "Y":
			r := &Result{
				Mode:          p.confirmMode,
				Confirmed:     true,
				ConfirmAction: p.ConfirmAction,
				ConfirmTarget: p.ConfirmTarget,
			}
			p.Cancel()
			return r, true, nil
		default:
			p.Cancel()
			return nil, true, nil
		}
	}

	// Quick-select: if user presses a shortcut key before typing, select that option
	if len(p.Options) > 0 && len(msg.String()) == 1 {
		ch := rune(msg.String()[0])
		for _, opt := range p.Options {
			if ch == opt.Key || ch == opt.Key+32 || ch == opt.Key-32 { // case-insensitive
				r := &Result{
					Mode:  p.Mode,
					Value: opt.Name,
				}
				p.Cancel()
				return r, true, nil
			}
		}
	}

	// Text input mode — intercept enter and esc
	switch msg.String() {
	case "enter":
		value := strings.TrimSpace(p.Input.Value())
		if value == "" {
			p.Cancel()
			return nil, true, nil
		}
		r := &Result{
			Mode:  p.Mode,
			Value: value,
		}
		p.Cancel()
		return r, true, nil

	case "esc":
		p.Cancel()
		return nil, true, nil

	default:
		// Delegate to bubbles textinput
		var cmd tea.Cmd
		p.Input, cmd = p.Input.Update(msg)
		return nil, true, cmd
	}
}

// Update processes non-key messages (e.g. cursor blink) for the textinput.
func (p *Prompt) Update(msg tea.Msg) tea.Cmd {
	if !p.Active() || p.Mode == p.confirmMode {
		return nil
	}
	var cmd tea.Cmd
	p.Input, cmd = p.Input.Update(msg)
	return cmd
}

// Render returns the prompt bar string.
func (p *Prompt) Render() string {
	if p.Mode == p.confirmMode {
		action := p.Labeler.ConfirmMessage(p.ConfirmAction, p.ConfirmTarget)
		return ErrorStyle.Render(action+"? ") + InputLabelStyle.Render("(y/n) ")
	}

	label := p.Labeler.PromptLabel(p.Mode)
	line := InputLabelStyle.Render(label+": ") + p.Input.View()
	if len(p.Options) > 0 {
		var opts []string
		for _, opt := range p.Options {
			opts = append(opts, RenderOption(opt))
		}
		line += "  " + HelpStyle.Render(strings.Join(opts, " "))
	}
	return line
}

// RenderHelp returns help text for the active prompt.
func (p *Prompt) RenderHelp() string {
	return HelpStyle.Render(" enter confirm · esc cancel")
}

// AssignShortcuts assigns a unique shortcut key to each name.
// It picks the first unused uppercase letter from each name.
func AssignShortcuts(names []string) []QuickOption {
	used := make(map[rune]bool)
	var opts []QuickOption
	for _, name := range names {
		var key rune
		for _, ch := range strings.ToUpper(name) {
			if ch >= 'A' && ch <= 'Z' && !used[ch] {
				key = ch
				break
			}
		}
		if key == 0 {
			// No unique letter found, try digits
			for _, ch := range name {
				if ch >= '0' && ch <= '9' && !used[ch] {
					key = ch
					break
				}
			}
		}
		if key != 0 {
			used[key] = true
			opts = append(opts, QuickOption{Key: key, Name: name})
		}
	}
	return opts
}

// RenderOption renders a name with its shortcut key highlighted: [C]hanges
func RenderOption(opt QuickOption) string {
	upper := strings.ToUpper(string(opt.Key))
	name := opt.Name
	// Find the position of the shortcut letter (case-insensitive)
	idx := strings.Index(strings.ToUpper(name), upper)
	if idx < 0 {
		return fmt.Sprintf("[%s]%s", upper, name)
	}
	before := name[:idx]
	letter := name[idx : idx+len(upper)]
	after := name[idx+len(upper):]
	return before + InputLabelStyle.Render("["+letter+"]") + after
}
