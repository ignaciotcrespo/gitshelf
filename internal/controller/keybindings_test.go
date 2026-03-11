package controller

import (
	"testing"

	"github.com/ignaciotcrespo/gitshelf/internal/types"
)

// TestKeyBindings_FooterText verifies footer generation from bindings.
func TestKeyBindings_FooterText(t *testing.T) {
	bindings := []KeyBinding{
		{Key: "a", Display: "a", Short: "all"},
		{Key: "x", Display: "x", Short: "none"},
		{Key: "B", Display: "B", Short: "accept"},
	}

	got := FooterText(bindings, nil)
	want := "a all · x none · B accept"
	if got != want {
		t.Errorf("FooterText = %q, want %q", got, want)
	}

	got = FooterText(bindings, map[string]bool{"B": true})
	want = "a all · x none"
	if got != want {
		t.Errorf("FooterText with exclude = %q, want %q", got, want)
	}
}

// TestKeyBindings_HelpSections verifies all sections are non-empty.
func TestKeyBindings_HelpSections(t *testing.T) {
	sections := HelpSections()
	if len(sections) == 0 {
		t.Fatal("HelpSections returned empty")
	}
	for _, sec := range sections {
		if sec.Heading == "" {
			t.Error("section with empty heading")
		}
		if len(sec.Keys) == 0 {
			t.Errorf("section %q has no keys", sec.Heading)
		}
		for _, kv := range sec.Keys {
			if kv[0] == "" || kv[1] == "" {
				t.Errorf("section %q has empty key or description: %v", sec.Heading, kv)
			}
		}
	}
}

// TestKeyBindings_CLKeys verifies every key in CLBindings is handled by HandleKey
// when the Changelists panel is focused.
func TestKeyBindings_CLKeys(t *testing.T) {
	for _, b := range CLBindings {
		s := NewState()
		s.Focus = types.PanelChangelists
		s.Pivot = types.PanelChangelists
		s.CLSelected = 1 // "Feature" — deletable, non-default
		ctx := KeyContext{
			CLCount:         2,
			CLNames:         []string{"Changes", "Feature"},
			CLFiles:         []string{"a.txt"},
			CLFileCount:     1,
			SelectedCount:   1,
			UnversionedName: "Unversioned Files",
			DefaultName:     "Changes",
			LastCommitMsg:   "last",
			Remotes:         []string{"origin"},
			DirtyFiles:      map[string]bool{"a.txt": true},
			DirtyCLs:        map[string]bool{"Feature": true},
		}
		s.SelectedFiles = map[string]bool{"a.txt": true}

		r := HandleKey(b.Key, s, ctx)
		// The key should produce some effect: prompt, status, setActive, copyPatch, quit, or state change
		hasEffect := r.StartPrompt != nil ||
			r.RunRemote != nil ||
			r.StatusMsg != "" ||
			r.ErrorMsg != "" ||
			r.SetActive != "" ||
			r.CopyPatch.Source != CopyPatchNone ||
			r.Quit ||
			r.Refresh != RefreshNone
		if !hasEffect {
			t.Errorf("CLBindings key %q produced no effect", b.Key)
		}
	}
}

// TestKeyBindings_ShelfKeys verifies every key in ShelfBindings is handled.
func TestKeyBindings_ShelfKeys(t *testing.T) {
	for _, b := range ShelfBindings {
		s := NewState()
		s.Focus = types.PanelShelves
		s.Pivot = types.PanelShelves
		ctx := KeyContext{
			ShelfCount: 1,
			ShelfNames: []string{"my-shelf"},
			CLNames:    []string{"Changes"},
			CLCount:    1,
		}

		r := HandleKey(b.Key, s, ctx)
		hasEffect := r.StartPrompt != nil ||
			r.CopyPatch.Source != CopyPatchNone ||
			r.StatusMsg != "" ||
			r.ErrorMsg != ""
		if !hasEffect {
			t.Errorf("ShelfBindings key %q produced no effect", b.Key)
		}
	}
}

// TestKeyBindings_CLFileKeys verifies every key in CLFileBindings is handled.
func TestKeyBindings_CLFileKeys(t *testing.T) {
	for _, b := range CLFileBindings {
		s := NewState()
		s.Focus = types.PanelFiles
		s.Pivot = types.PanelChangelists
		s.SelectedFiles = map[string]bool{"a.txt": true}
		prevSelCount := len(s.SelectedFiles)
		prevFileSel := s.CLFileSel
		ctx := KeyContext{
			CLCount:         1,
			CLNames:         []string{"Changes"},
			CLFiles:         []string{"a.txt", "b.txt"},
			CLFileCount:     2,
			SelectedCount:   1,
			UnversionedName: "Unversioned Files",
			DefaultName:     "Changes",
			LastCommitMsg:   "last",
			Remotes:         []string{"origin"},
			DirtyFiles:      map[string]bool{"a.txt": true},
			DirtyCLs:        map[string]bool{"Changes": true},
		}

		r := HandleKey(b.Key, s, ctx)
		hasEffect := r.StartPrompt != nil ||
			r.RunRemote != nil ||
			r.StatusMsg != "" ||
			r.ErrorMsg != "" ||
			r.CopyPatch.Source != CopyPatchNone ||
			r.Refresh != RefreshNone ||
			len(r.State.SelectedFiles) != prevSelCount ||
			r.State.CLFileSel != prevFileSel
		if !hasEffect {
			t.Errorf("CLFileBindings key %q produced no effect", b.Key)
		}
	}
}

// TestKeyBindings_DiffKeys verifies every key in DiffBindings is handled.
func TestKeyBindings_DiffKeys(t *testing.T) {
	for _, b := range DiffBindings {
		s := NewState()
		s.Focus = types.PanelDiff
		s.DiffHScroll = 10 // so left scroll can work
		ctx := KeyContext{}

		r := HandleKey(b.Key, s, ctx)
		hasEffect := r.CopyPatch.Source != CopyPatchNone ||
			r.State.DiffWrap != s.DiffWrap ||
			r.State.DiffHScroll != s.DiffHScroll
		if !hasEffect {
			t.Errorf("DiffBindings key %q produced no effect", b.Key)
		}
	}
}
