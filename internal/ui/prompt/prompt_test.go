package prompt

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ignaciotcrespo/gitshelf/internal/types"
)

func keyMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune(s),
	}
}

func specialKeyMsg(t tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{
		Type: t,
	}
}

func TestPromptActive(t *testing.T) {
	p := &Prompt{}
	if p.Active() {
		t.Error("new prompt should not be active")
	}

	p.Start(types.PromptNewChangelist, "")
	if !p.Active() {
		t.Error("started prompt should be active")
	}

	p.Cancel()
	if p.Active() {
		t.Error("cancelled prompt should not be active")
	}
}

func TestPromptStart(t *testing.T) {
	p := &Prompt{}
	p.Start(types.PromptCommit, "default value")

	if p.Mode() != types.PromptCommit {
		t.Errorf("Mode = %v, want %v", p.Mode(), types.PromptCommit)
	}
	if p.Value() != "default value" {
		t.Errorf("Value = %q, want %q", p.Value(), "default value")
	}
}

func TestPromptStartConfirm(t *testing.T) {
	p := &Prompt{}
	p.StartConfirm(types.ConfirmDeleteChangelist, "My CL")

	if p.Mode() != types.PromptConfirm {
		t.Errorf("Mode = %v, want %v", p.Mode(), types.PromptConfirm)
	}
	if p.ConfirmAction() != types.ConfirmDeleteChangelist {
		t.Errorf("ConfirmAction = %v, want %v", p.ConfirmAction(), types.ConfirmDeleteChangelist)
	}
	if p.ConfirmTarget() != "My CL" {
		t.Errorf("ConfirmTarget = %q, want %q", p.ConfirmTarget(), "My CL")
	}
}

func TestPromptCancel(t *testing.T) {
	p := &Prompt{}
	p.Start(types.PromptNewChangelist, "some value")
	p.Cancel()

	if p.Mode() != types.PromptNone {
		t.Errorf("after cancel, Mode = %v, want %v", p.Mode(), types.PromptNone)
	}
}

func TestHandleKeyNotActive(t *testing.T) {
	p := &Prompt{}
	result, handled, _ := p.HandleKey(keyMsg("a"))
	if result != nil || handled {
		t.Error("HandleKey on inactive prompt should return nil, false")
	}
}

func TestHandleKeyTyping(t *testing.T) {
	p := &Prompt{}
	p.Start(types.PromptNewChangelist, "")

	// Type "abc"
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

func TestHandleKeyBackspace(t *testing.T) {
	p := &Prompt{}
	p.Start(types.PromptNewChangelist, "hello")

	result, handled, _ := p.HandleKey(specialKeyMsg(tea.KeyBackspace))
	if result != nil {
		t.Error("backspace should not produce a result")
	}
	if !handled {
		t.Error("backspace should be handled")
	}
	if p.Value() != "hell" {
		t.Errorf("after backspace, Value = %q, want %q", p.Value(), "hell")
	}
}

func TestHandleKeyBackspaceEmpty(t *testing.T) {
	p := &Prompt{}
	p.Start(types.PromptNewChangelist, "")

	// Backspace on empty value should not panic
	result, handled, _ := p.HandleKey(specialKeyMsg(tea.KeyBackspace))
	if result != nil {
		t.Error("backspace on empty should not produce a result")
	}
	if !handled {
		t.Error("backspace should be handled")
	}
	if p.Value() != "" {
		t.Errorf("after backspace on empty, Value = %q, want empty", p.Value())
	}
}

func TestHandleKeyEnter(t *testing.T) {
	p := &Prompt{}
	p.Start(types.PromptNewChangelist, "my cl")

	result, handled, _ := p.HandleKey(specialKeyMsg(tea.KeyEnter))
	if !handled {
		t.Error("enter should be handled")
	}
	if result == nil {
		t.Fatal("enter should produce a result")
	}
	if result.Mode != types.PromptNewChangelist {
		t.Errorf("result Mode = %v, want %v", result.Mode, types.PromptNewChangelist)
	}
	if result.Value != "my cl" {
		t.Errorf("result Value = %q, want %q", result.Value, "my cl")
	}
	// Prompt should be cancelled after enter
	if p.Active() {
		t.Error("prompt should be inactive after enter")
	}
}

func TestHandleKeyEnterEmpty(t *testing.T) {
	p := &Prompt{}
	p.Start(types.PromptNewChangelist, "")

	result, handled, _ := p.HandleKey(specialKeyMsg(tea.KeyEnter))
	if !handled {
		t.Error("enter should be handled")
	}
	if result != nil {
		t.Error("enter with empty value should return nil result")
	}
	if p.Active() {
		t.Error("prompt should be cancelled after enter with empty value")
	}
}

func TestHandleKeyEnterWhitespace(t *testing.T) {
	p := &Prompt{}
	p.Start(types.PromptNewChangelist, "   ")

	result, handled, _ := p.HandleKey(specialKeyMsg(tea.KeyEnter))
	if !handled {
		t.Error("enter should be handled")
	}
	if result != nil {
		t.Error("enter with whitespace-only value should return nil result")
	}
}

func TestHandleKeyEsc(t *testing.T) {
	p := &Prompt{}
	p.Start(types.PromptNewChangelist, "some text")

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
	p := &Prompt{}
	p.StartConfirm(types.ConfirmDeleteChangelist, "Target CL")

	result, handled, _ := p.HandleKey(keyMsg("y"))
	if !handled {
		t.Error("y should be handled")
	}
	if result == nil {
		t.Fatal("y should produce a result")
	}
	if result.Mode != types.PromptConfirm {
		t.Errorf("result Mode = %v, want %v", result.Mode, types.PromptConfirm)
	}
	if !result.Confirmed {
		t.Error("result.Confirmed should be true")
	}
	if result.ConfirmAction != types.ConfirmDeleteChangelist {
		t.Errorf("result.ConfirmAction = %v, want %v", result.ConfirmAction, types.ConfirmDeleteChangelist)
	}
	if result.ConfirmTarget != "Target CL" {
		t.Errorf("result.ConfirmTarget = %q, want %q", result.ConfirmTarget, "Target CL")
	}
	if p.Active() {
		t.Error("prompt should be inactive after confirmation")
	}
}

func TestHandleKeyConfirmUpperY(t *testing.T) {
	p := &Prompt{}
	p.StartConfirm(types.ConfirmDropShelf, "my shelf")

	result, handled, _ := p.HandleKey(keyMsg("Y"))
	if !handled {
		t.Error("Y should be handled")
	}
	if result == nil {
		t.Fatal("Y should produce a result")
	}
	if !result.Confirmed {
		t.Error("result.Confirmed should be true for Y")
	}
}

func TestHandleKeyConfirmNo(t *testing.T) {
	p := &Prompt{}
	p.StartConfirm(types.ConfirmDeleteChangelist, "Target")

	result, handled, _ := p.HandleKey(keyMsg("n"))
	if !handled {
		t.Error("n should be handled")
	}
	if result != nil {
		t.Error("n should return nil result (cancel)")
	}
	if p.Active() {
		t.Error("prompt should be inactive after n")
	}
}

func TestHandleKeyConfirmOtherKey(t *testing.T) {
	p := &Prompt{}
	p.StartConfirm(types.ConfirmDeleteChangelist, "Target")

	result, handled, _ := p.HandleKey(keyMsg("x"))
	if !handled {
		t.Error("any key in confirm mode should be handled")
	}
	if result != nil {
		t.Error("non-y key should return nil result")
	}
	if p.Active() {
		t.Error("prompt should be inactive after non-y key")
	}
}

func TestHandleKeyFullWorkflow(t *testing.T) {
	p := &Prompt{}
	p.Start(types.PromptCommit, "")

	// Type "fix bug"
	for _, ch := range "fix bug" {
		p.HandleKey(keyMsg(string(ch)))
	}
	if p.Value() != "fix bug" {
		t.Fatalf("after typing, Value = %q, want %q", p.Value(), "fix bug")
	}

	// Backspace twice
	p.HandleKey(specialKeyMsg(tea.KeyBackspace))
	p.HandleKey(specialKeyMsg(tea.KeyBackspace))
	if p.Value() != "fix b" {
		t.Fatalf("after backspace, Value = %q, want %q", p.Value(), "fix b")
	}

	// Type "ug" again
	p.HandleKey(keyMsg("u"))
	p.HandleKey(keyMsg("g"))

	// Press enter
	result, _, _ := p.HandleKey(specialKeyMsg(tea.KeyEnter))
	if result == nil {
		t.Fatal("expected result on enter")
	}
	if result.Value != "fix bug" {
		t.Errorf("result.Value = %q, want %q", result.Value, "fix bug")
	}
	if result.Mode != types.PromptCommit {
		t.Errorf("result.Mode = %v, want %v", result.Mode, types.PromptCommit)
	}
}

func TestModeConstants(t *testing.T) {
	// Verify modes are distinct
	modes := []types.PromptMode{types.PromptNone, types.PromptNewChangelist, types.PromptRenameChangelist, types.PromptShelveFiles, types.PromptRenameShelf, types.PromptCommit, types.PromptMoveFile, types.PromptAmend, types.PromptConfirm}
	seen := map[types.PromptMode]bool{}
	for _, m := range modes {
		if seen[m] {
			t.Errorf("duplicate Mode value: %d", m)
		}
		seen[m] = true
	}
}

func TestConfirmActionConstants(t *testing.T) {
	actions := []types.ConfirmAction{types.ConfirmNone, types.ConfirmDeleteChangelist, types.ConfirmDropShelf}
	seen := map[types.ConfirmAction]bool{}
	for _, a := range actions {
		if seen[a] {
			t.Errorf("duplicate ConfirmAction value: %d", a)
		}
		seen[a] = true
	}
}
