package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func init() {
	InputLabelStyle = lipgloss.NewStyle()
	ErrorStyle = lipgloss.NewStyle()
	HelpStyle = lipgloss.NewStyle()
}

// testLabeler implements PromptLabeler for testing.
type testLabeler struct{}

func (t testLabeler) PromptLabel(mode PromptMode) string {
	switch mode {
	case 1:
		return "New item"
	case 2:
		return "Rename"
	}
	return "Unknown"
}

func (t testLabeler) ConfirmMessage(action ConfirmAction, target string) string {
	return "Confirm " + target
}

const (
	testModeNew     PromptMode    = 1
	testModeRename  PromptMode    = 2
	testModeConfirm PromptMode    = 10
	testActionDel   ConfirmAction = 1
)

func newTestPrompt() *Prompt {
	p := NewPrompt(testLabeler{}, testModeConfirm)
	return &p
}

func keyMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune(s),
	}
}

func specialKeyMsg(t tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: t}
}

func TestPromptActive(t *testing.T) {
	p := newTestPrompt()
	if p.Active() {
		t.Error("new prompt should not be active")
	}

	p.Start(testModeNew, "")
	if !p.Active() {
		t.Error("started prompt should be active")
	}

	p.Cancel()
	if p.Active() {
		t.Error("cancelled prompt should not be active")
	}
}

func TestPromptStart(t *testing.T) {
	p := newTestPrompt()
	p.Start(testModeRename, "default value")

	if p.Mode != testModeRename {
		t.Errorf("Mode = %v, want %v", p.Mode, testModeRename)
	}
	if p.Value() != "default value" {
		t.Errorf("Value = %q, want %q", p.Value(), "default value")
	}
}

func TestPromptStartConfirm(t *testing.T) {
	p := newTestPrompt()
	p.StartConfirm(testActionDel, "My Item")

	if p.Mode != testModeConfirm {
		t.Errorf("Mode = %v, want %v", p.Mode, testModeConfirm)
	}
	if p.ConfirmAction != testActionDel {
		t.Errorf("ConfirmAction = %v, want %v", p.ConfirmAction, testActionDel)
	}
	if p.ConfirmTarget != "My Item" {
		t.Errorf("ConfirmTarget = %q, want %q", p.ConfirmTarget, "My Item")
	}
}

func TestHandleKeyNotActive(t *testing.T) {
	p := newTestPrompt()
	result, handled, _ := p.HandleKey(keyMsg("a"))
	if result != nil || handled {
		t.Error("HandleKey on inactive prompt should return nil, false")
	}
}

func TestHandleKeyEnter(t *testing.T) {
	p := newTestPrompt()
	p.Start(testModeNew, "my item")

	result, handled, _ := p.HandleKey(specialKeyMsg(tea.KeyEnter))
	if !handled {
		t.Error("enter should be handled")
	}
	if result == nil {
		t.Fatal("enter should produce a result")
	}
	if result.Mode != testModeNew {
		t.Errorf("result Mode = %v, want %v", result.Mode, testModeNew)
	}
	if result.Value != "my item" {
		t.Errorf("result Value = %q, want %q", result.Value, "my item")
	}
	if p.Active() {
		t.Error("prompt should be inactive after enter")
	}
}

func TestHandleKeyEnterEmpty(t *testing.T) {
	p := newTestPrompt()
	p.Start(testModeNew, "")

	result, handled, _ := p.HandleKey(specialKeyMsg(tea.KeyEnter))
	if !handled {
		t.Error("enter should be handled")
	}
	if result != nil {
		t.Error("enter with empty value should return nil result")
	}
	if p.Active() {
		t.Error("prompt should be cancelled")
	}
}

func TestHandleKeyEsc(t *testing.T) {
	p := newTestPrompt()
	p.Start(testModeNew, "some text")

	result, handled, _ := p.HandleKey(specialKeyMsg(tea.KeyEsc))
	if !handled {
		t.Error("esc should be handled")
	}
	if result != nil {
		t.Error("esc should return nil result")
	}
	if p.Active() {
		t.Error("prompt should be inactive after esc")
	}
}

func TestHandleKeyConfirmYes(t *testing.T) {
	p := newTestPrompt()
	p.StartConfirm(testActionDel, "Target")

	result, handled, _ := p.HandleKey(keyMsg("y"))
	if !handled {
		t.Error("y should be handled")
	}
	if result == nil {
		t.Fatal("y should produce a result")
	}
	if !result.Confirmed {
		t.Error("result.Confirmed should be true")
	}
	if result.ConfirmAction != testActionDel {
		t.Errorf("result.ConfirmAction = %v, want %v", result.ConfirmAction, testActionDel)
	}
	if result.ConfirmTarget != "Target" {
		t.Errorf("result.ConfirmTarget = %q, want %q", result.ConfirmTarget, "Target")
	}
}

func TestHandleKeyConfirmNo(t *testing.T) {
	p := newTestPrompt()
	p.StartConfirm(testActionDel, "Target")

	result, handled, _ := p.HandleKey(keyMsg("n"))
	if !handled {
		t.Error("n should be handled")
	}
	if result != nil {
		t.Error("n should return nil result")
	}
	if p.Active() {
		t.Error("prompt should be inactive after n")
	}
}

func TestAssignShortcuts(t *testing.T) {
	opts := AssignShortcuts([]string{"Changes", "Bugfix", "Cleanup"})
	if len(opts) != 3 {
		t.Fatalf("expected 3 options, got %d", len(opts))
	}
	// Each should have a unique key
	keys := make(map[rune]bool)
	for _, o := range opts {
		if keys[o.Key] {
			t.Errorf("duplicate key: %c", o.Key)
		}
		keys[o.Key] = true
	}
}

func TestHandleKeyTyping(t *testing.T) {
	p := newTestPrompt()
	p.Start(testModeNew, "")

	for _, ch := range "abc" {
		result, handled, _ := p.HandleKey(keyMsg(string(ch)))
		if result != nil {
			t.Error("typing should not produce a result")
		}
		if !handled {
			t.Error("typing should be handled")
		}
	}

	if p.Value() != "abc" {
		t.Errorf("after typing, Value = %q, want %q", p.Value(), "abc")
	}
}
