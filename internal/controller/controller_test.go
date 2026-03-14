package controller

import (
	"testing"

	"github.com/ignaciotcrespo/gitshelf/internal/types"
)

func TestIsChangelistContext(t *testing.T) {
	tests := []struct {
		pivot types.PanelID
		want  bool
	}{
		{types.PanelChangelists, true},
		{types.PanelShelves, false},
	}
	for _, tt := range tests {
		s := State{Pivot: tt.pivot}
		if got := IsChangelistContext(s); got != tt.want {
			t.Errorf("pivot=%d: got %v, want %v", tt.pivot, got, tt.want)
		}
	}
}

func TestMoveUp(t *testing.T) {
	ctx := KeyContext{CLCount: 3, CLFileCount: 3, ShelfCount: 2, ShelfFileCount: 2}

	t.Run("changelists", func(t *testing.T) {
		s := State{Focus: types.PanelChangelists, CLSelected: 1, Pivot: types.PanelChangelists}
		r := MoveUp(&s, ctx)
		if s.CLSelected != 0 {
			t.Errorf("expected 0, got %d", s.CLSelected)
		}
		if r != RefreshCLFiles {
			t.Errorf("expected RefreshCLFiles, got %d", r)
		}
	})

	t.Run("changelists at top", func(t *testing.T) {
		s := State{Focus: types.PanelChangelists, CLSelected: 0, Pivot: types.PanelChangelists}
		r := MoveUp(&s, ctx)
		if s.CLSelected != 0 {
			t.Errorf("expected 0, got %d", s.CLSelected)
		}
		if r != RefreshNone {
			t.Errorf("expected RefreshNone, got %d", r)
		}
	})

	t.Run("shelves", func(t *testing.T) {
		s := State{Focus: types.PanelShelves, ShelfSel: 1, Pivot: types.PanelShelves}
		r := MoveUp(&s, ctx)
		if s.ShelfSel != 0 {
			t.Errorf("expected 0, got %d", s.ShelfSel)
		}
		if r != RefreshShelfFiles {
			t.Errorf("expected RefreshShelfFiles, got %d", r)
		}
	})

	t.Run("files changelist context", func(t *testing.T) {
		s := State{Focus: types.PanelFiles, CLFileSel: 2, Pivot: types.PanelChangelists}
		r := MoveUp(&s, ctx)
		if s.CLFileSel != 1 {
			t.Errorf("expected 1, got %d", s.CLFileSel)
		}
		if r != RefreshDiff {
			t.Errorf("expected RefreshDiff, got %d", r)
		}
	})

	t.Run("files shelf context", func(t *testing.T) {
		s := State{Focus: types.PanelFiles, ShelfFileSel: 1, Pivot: types.PanelShelves}
		r := MoveUp(&s, ctx)
		if s.ShelfFileSel != 0 {
			t.Errorf("expected 0, got %d", s.ShelfFileSel)
		}
		if r != RefreshDiff {
			t.Errorf("expected RefreshDiff, got %d", r)
		}
	})

	t.Run("diff scroll", func(t *testing.T) {
		s := State{Focus: types.PanelDiff, DiffScroll: 5}
		MoveUp(&s, ctx)
		if s.DiffScroll != 4 {
			t.Errorf("expected 4, got %d", s.DiffScroll)
		}
	})

	t.Run("log scroll increases", func(t *testing.T) {
		s := State{Focus: types.PanelLog, LogScroll: 0}
		MoveUp(&s, ctx)
		if s.LogScroll != 1 {
			t.Errorf("expected 1, got %d", s.LogScroll)
		}
	})
}

func TestMoveDown(t *testing.T) {
	ctx := KeyContext{CLCount: 3, CLFileCount: 3, ShelfCount: 2, ShelfFileCount: 2}

	t.Run("changelists", func(t *testing.T) {
		s := State{Focus: types.PanelChangelists, CLSelected: 0, Pivot: types.PanelChangelists}
		r := MoveDown(&s, ctx)
		if s.CLSelected != 1 {
			t.Errorf("expected 1, got %d", s.CLSelected)
		}
		if r != RefreshCLFiles {
			t.Errorf("expected RefreshCLFiles, got %d", r)
		}
	})

	t.Run("changelists at bottom", func(t *testing.T) {
		s := State{Focus: types.PanelChangelists, CLSelected: 2, Pivot: types.PanelChangelists}
		r := MoveDown(&s, ctx)
		if s.CLSelected != 2 {
			t.Errorf("expected 2, got %d", s.CLSelected)
		}
		if r != RefreshNone {
			t.Errorf("expected RefreshNone, got %d", r)
		}
	})

	t.Run("diff scroll increases", func(t *testing.T) {
		s := State{Focus: types.PanelDiff, DiffScroll: 3}
		MoveDown(&s, ctx)
		if s.DiffScroll != 4 {
			t.Errorf("expected 4, got %d", s.DiffScroll)
		}
	})

	t.Run("log scroll decreases", func(t *testing.T) {
		s := State{Focus: types.PanelLog, LogScroll: 5}
		MoveDown(&s, ctx)
		if s.LogScroll != 4 {
			t.Errorf("expected 4, got %d", s.LogScroll)
		}
	})
}

func TestHandleEnter(t *testing.T) {
	ctx := KeyContext{}

	t.Run("changelists to files", func(t *testing.T) {
		s := State{Focus: types.PanelChangelists}
		r := HandleEnter(&s, ctx)
		if s.Focus != types.PanelFiles {
			t.Errorf("expected Files, got %d", s.Focus)
		}
		if r != RefreshDiff {
			t.Errorf("expected RefreshDiff")
		}
	})

	t.Run("files to diff", func(t *testing.T) {
		s := State{Focus: types.PanelFiles}
		r := HandleEnter(&s, ctx)
		if s.Focus != types.PanelDiff {
			t.Errorf("expected Diff, got %d", s.Focus)
		}
		if r != RefreshDiff {
			t.Errorf("expected RefreshDiff")
		}
	})

	t.Run("diff does nothing", func(t *testing.T) {
		s := State{Focus: types.PanelDiff}
		r := HandleEnter(&s, ctx)
		if s.Focus != types.PanelDiff {
			t.Errorf("focus should not change")
		}
		if r != RefreshNone {
			t.Errorf("expected RefreshNone")
		}
	})

	t.Run("worktree select activates", func(t *testing.T) {
		s := State{Focus: types.PanelWorktrees, WorktreeSel: 1, SelectedFiles: make(map[string]bool)}
		wtCtx := KeyContext{
			WorktreeCount:       2,
			WorktreePaths:       []string{"/repo", "/repo-wt"},
			CurrentWorktreePath: "/repo",
		}
		r := HandleEnter(&s, wtCtx)
		if s.ActiveWorktreePath != "/repo-wt" {
			t.Errorf("expected ActiveWorktreePath=/repo-wt, got %q", s.ActiveWorktreePath)
		}
		if r != RefreshAll {
			t.Errorf("expected RefreshAll")
		}
		if s.CLSelected != 0 || s.ShelfSel != 0 {
			t.Error("expected selection indices to reset")
		}
	})

	t.Run("worktree select current toggles off", func(t *testing.T) {
		s := State{Focus: types.PanelWorktrees, WorktreeSel: 0, ActiveWorktreePath: "/repo", SelectedFiles: make(map[string]bool)}
		wtCtx := KeyContext{
			WorktreeCount:       2,
			WorktreePaths:       []string{"/repo", "/repo-wt"},
			CurrentWorktreePath: "/repo",
		}
		r := HandleEnter(&s, wtCtx)
		if s.ActiveWorktreePath != "" {
			t.Errorf("expected empty ActiveWorktreePath, got %q", s.ActiveWorktreePath)
		}
		if r != RefreshAll {
			t.Errorf("expected RefreshAll")
		}
	})
}

func TestHandleKey_NewChangelist(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelChangelists
	ctx := KeyContext{CLCount: 1}
	r := HandleKey("n", s, ctx)
	if r.StartPrompt == nil {
		t.Fatal("expected prompt")
	}
	if r.StartPrompt.Mode != types.PromptNewChangelist {
		t.Errorf("expected NewChangelist mode, got %d", r.StartPrompt.Mode)
	}
}

func TestHandleKey_CommitNoSelection(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelChangelists
	ctx := KeyContext{CLCount: 1, SelectedCount: 0, CLNames: []string{"Changes"}}
	r := HandleKey("c", s, ctx)
	if r.ErrorMsg == "" {
		t.Error("expected error message")
	}
	if r.StartPrompt != nil {
		t.Error("should not start prompt")
	}
}

func TestHandleKey_CommitWithSelection(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelChangelists
	ctx := KeyContext{CLCount: 1, SelectedCount: 2, CLNames: []string{"Changes"}}
	r := HandleKey("c", s, ctx)
	if r.StartPrompt == nil {
		t.Fatal("expected prompt")
	}
	if r.StartPrompt.Mode != types.PromptCommit {
		t.Errorf("expected Commit mode")
	}
}

func TestHandleKey_SpaceToggle(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelFiles
	s.Pivot = types.PanelChangelists
	ctx := KeyContext{CLCount: 1, CLFileCount: 3, CLFiles: []string{"a.go", "b.go", "c.go"}}

	r := HandleKey(" ", s, ctx)
	if !r.State.SelectedFiles["a.go"] {
		t.Error("expected a.go selected")
	}
	if r.State.CLFileSel != 1 {
		t.Errorf("cursor should move down, got %d", r.State.CLFileSel)
	}

	r.State.CLFileSel = 0
	r2 := HandleKey(" ", r.State, ctx)
	if r2.State.SelectedFiles["a.go"] {
		t.Error("expected a.go deselected")
	}
}

func TestHandleKey_ShelveUnversioned(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelChangelists
	ctx := KeyContext{
		CLCount:         1,
		CLNames:         []string{"Unversioned Files"},
		UnversionedName: "Unversioned Files",
	}
	r := HandleKey("s", s, ctx)
	if r.ErrorMsg == "" {
		t.Error("expected error for unversioned shelve")
	}
}

func TestHandleKey_ShelfUnshelve(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelShelves
	s.Pivot = types.PanelShelves
	ctx := KeyContext{ShelfCount: 1, ShelfNames: []string{"my-shelf"}, CLNames: []string{"Changes"}, ActiveCL: "Changes"}
	r := HandleKey("u", s, ctx)
	if r.StartPrompt == nil {
		t.Fatal("expected prompt")
	}
	if r.StartPrompt.Mode != types.PromptUnshelve {
		t.Errorf("expected PromptUnshelve, got %d", r.StartPrompt.Mode)
	}
	if r.StartPrompt.DefaultValue != "Changes" {
		t.Errorf("expected default value Changes, got %s", r.StartPrompt.DefaultValue)
	}
	if len(r.StartPrompt.Options) != 1 || r.StartPrompt.Options[0] != "Changes" {
		t.Errorf("expected options [Changes], got %v", r.StartPrompt.Options)
	}
}

func TestHandleKey_ShelfDrop(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelShelves
	s.Pivot = types.PanelShelves
	ctx := KeyContext{ShelfCount: 1, ShelfNames: []string{"my-shelf"}}
	r := HandleKey("d", s, ctx)
	if r.StartPrompt == nil {
		t.Fatal("expected confirm prompt")
	}
	if r.StartPrompt.Confirm != types.ConfirmDropShelf {
		t.Errorf("expected ConfirmDropShelf")
	}
}

func TestHandleKey_MoveFile(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelFiles
	s.Pivot = types.PanelChangelists
	ctx := KeyContext{CLCount: 1, CLFileCount: 2, CLNames: []string{"Changes"}, CLFiles: []string{"a.go", "b.go"}}
	r := HandleKey("m", s, ctx)
	if r.StartPrompt == nil {
		t.Fatal("expected prompt")
	}
	if r.StartPrompt.Mode != types.PromptMoveFile {
		t.Errorf("expected MoveFile mode")
	}
	if r.State.MoveFile != "a.go" {
		t.Errorf("expected a.go, got %s", r.State.MoveFile)
	}
}

func TestHandleKey_SetActive(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelChangelists
	ctx := KeyContext{CLCount: 1, CLNames: []string{"Changes"}, UnversionedName: "Unversioned Files"}
	r := HandleKey("a", s, ctx)
	if r.SetActive != "Changes" {
		t.Errorf("expected SetActive=Changes, got %s", r.SetActive)
	}
}

func TestHandleKey_SetActiveUnversioned(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelChangelists
	ctx := KeyContext{CLCount: 1, CLNames: []string{"Unversioned Files"}, UnversionedName: "Unversioned Files"}
	r := HandleKey("a", s, ctx)
	if r.SetActive != "" {
		t.Errorf("should not set active for unversioned")
	}
}

// --- CyclePanelState ---

func TestCyclePanelState_FocusedHidden(t *testing.T) {
	got, moved := CyclePanelState(types.PanelHidden, true)
	if got != types.PanelNormal || moved {
		t.Errorf("expected (Normal, false), got (%d, %v)", got, moved)
	}
}

func TestCyclePanelState_UnknownStateFallback(t *testing.T) {
	unknown := types.PanelState(99)
	got, moved := CyclePanelState(unknown, true)
	if got != unknown || moved {
		t.Errorf("expected (%d, false), got (%d, %v)", unknown, got, moved)
	}
}

func TestCyclePanelState_NotFocusedNonHidden(t *testing.T) {
	got, moved := CyclePanelState(types.PanelNormal, false)
	if got != types.PanelNormal || moved {
		t.Errorf("expected (Normal, false), got (%d, %v)", got, moved)
	}
}

// --- HandleKey: up/down dispatching ---

func TestHandleKey_UpDown(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelChangelists
	ctx := KeyContext{CLCount: 3}
	r := HandleKey("down", s, ctx)
	if r.State.CLSelected != 1 {
		t.Errorf("expected CLSelected=1, got %d", r.State.CLSelected)
	}
	r = HandleKey("up", r.State, ctx)
	if r.State.CLSelected != 0 {
		t.Errorf("expected CLSelected=0, got %d", r.State.CLSelected)
	}
}

// --- HandleKey: left/h (diff hscroll decrease) ---

func TestHandleKey_Left_DiffHScrollDecrease(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelDiff
	s.DiffWrap = false
	s.DiffHScroll = 8
	r := HandleKey("left", s, KeyContext{})
	if r.State.DiffHScroll != 4 {
		t.Errorf("expected 4, got %d", r.State.DiffHScroll)
	}

	// Decrease to 0 when hscroll < 4
	s.DiffHScroll = 2
	r = HandleKey("h", s, KeyContext{})
	if r.State.DiffHScroll != 0 {
		t.Errorf("expected 0, got %d", r.State.DiffHScroll)
	}
}

func TestHandleKey_Left_NoOpWhenWrapped(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelDiff
	s.DiffWrap = true
	s.DiffHScroll = 8
	r := HandleKey("left", s, KeyContext{})
	if r.State.DiffHScroll != 8 {
		t.Errorf("expected no change, got %d", r.State.DiffHScroll)
	}
}

func TestHandleKey_Left_NoOpWhenScrollZero(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelDiff
	s.DiffWrap = false
	s.DiffHScroll = 0
	r := HandleKey("left", s, KeyContext{})
	if r.State.DiffHScroll != 0 {
		t.Errorf("expected 0, got %d", r.State.DiffHScroll)
	}
}

// --- HandleKey: right/l (diff hscroll increase) ---

func TestHandleKey_Right_DiffHScrollIncrease(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelDiff
	s.DiffWrap = false
	s.DiffHScroll = 0
	r := HandleKey("right", s, KeyContext{})
	if r.State.DiffHScroll != 4 {
		t.Errorf("expected 4, got %d", r.State.DiffHScroll)
	}

	// Also test "l" alias
	r = HandleKey("l", r.State, KeyContext{})
	if r.State.DiffHScroll != 8 {
		t.Errorf("expected 8, got %d", r.State.DiffHScroll)
	}
}

func TestHandleKey_Right_NoOpWhenWrapped(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelDiff
	s.DiffWrap = true
	s.DiffHScroll = 0
	r := HandleKey("right", s, KeyContext{})
	if r.State.DiffHScroll != 0 {
		t.Errorf("expected no change, got %d", r.State.DiffHScroll)
	}
}

// --- HandleKey: w (wrap toggle) ---

func TestHandleKey_W_WrapToggle(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelDiff
	s.DiffWrap = false
	s.DiffHScroll = 8
	r := HandleKey("w", s, KeyContext{})
	if !r.State.DiffWrap {
		t.Error("expected DiffWrap true")
	}
	if r.State.DiffHScroll != 0 {
		t.Errorf("expected hscroll reset to 0, got %d", r.State.DiffHScroll)
	}

	// Toggle back
	r = HandleKey("w", r.State, KeyContext{})
	if r.State.DiffWrap {
		t.Error("expected DiffWrap false")
	}
}

func TestHandleKey_W_NotDiff(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelChangelists
	s.DiffWrap = false
	r := HandleKey("w", s, KeyContext{})
	if r.State.DiffWrap {
		t.Error("w should not toggle wrap when not on diff panel")
	}
}

// --- HandleKey: enter ---

func TestHandleKey_Enter(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelChangelists
	r := HandleKey("enter", s, KeyContext{})
	if r.State.Focus != types.PanelFiles {
		t.Errorf("expected Files, got %d", r.State.Focus)
	}
}

// --- HandleKey: fallback to handleChangelistKey ---

func TestHandleKey_FallbackToChangelistKey(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelDiff
	ctx := KeyContext{}
	r := HandleKey("n", s, ctx)
	if r.StartPrompt == nil {
		t.Fatal("expected prompt from fallback handleChangelistKey")
	}
	if r.StartPrompt.Mode != types.PromptNewChangelist {
		t.Errorf("expected NewChangelist mode")
	}
}

// --- handleChangelistKey: "m" with selected files (skip MoveFile) ---

func TestHandleKey_MoveFile_WithSelectedFiles(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelFiles
	s.Pivot = types.PanelChangelists
	ctx := KeyContext{
		CLCount:       1,
		CLFileCount:   2,
		CLFiles:       []string{"a.go", "b.go"},
		CLNames:       []string{"Changes"},
		SelectedCount: 2,
	}
	r := HandleKey("m", s, ctx)
	if r.StartPrompt == nil {
		t.Fatal("expected prompt")
	}
	if r.State.MoveFile != "" {
		t.Errorf("MoveFile should be empty when SelectedCount > 0, got %s", r.State.MoveFile)
	}
}

// --- handleChangelistKey: space with nil SelectedFiles ---

func TestHandleKey_Space_NilSelectedFiles(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelFiles
	s.Pivot = types.PanelChangelists
	s.SelectedFiles = nil
	ctx := KeyContext{CLFileCount: 2, CLFiles: []string{"a.go", "b.go"}}
	r := HandleKey(" ", s, ctx)
	if r.State.SelectedFiles == nil {
		t.Fatal("expected SelectedFiles to be initialized")
	}
	if !r.State.SelectedFiles["a.go"] {
		t.Error("expected a.go to be selected")
	}
}

// --- handleChangelistKey: "a" select-all in files panel ---

func TestHandleKey_A_SelectAllFiles(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelFiles
	s.Pivot = types.PanelChangelists
	s.SelectedFiles = nil
	ctx := KeyContext{CLFileCount: 3, CLFiles: []string{"a.go", "b.go", "c.go"}}
	r := HandleKey("a", s, ctx)
	if len(r.State.SelectedFiles) != 3 {
		t.Errorf("expected 3 selected files, got %d", len(r.State.SelectedFiles))
	}
	for _, f := range ctx.CLFiles {
		if !r.State.SelectedFiles[f] {
			t.Errorf("expected %s selected", f)
		}
	}
}

// --- handleChangelistKey: "x" clear selection ---

func TestHandleKey_X_ClearSelection(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelFiles
	s.Pivot = types.PanelChangelists
	s.SelectedFiles = map[string]bool{"a.go": true, "b.go": true}
	r := HandleKey("x", s, KeyContext{CLCount: 1, CLNames: []string{"Changes"}})
	if len(r.State.SelectedFiles) != 0 {
		t.Errorf("expected empty selection, got %d", len(r.State.SelectedFiles))
	}
}

// --- handleChangelistKey: "s" ---

func TestHandleKey_S_NoCLs(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelChangelists
	ctx := KeyContext{CLCount: 0}
	r := HandleKey("s", s, ctx)
	if r.StartPrompt != nil {
		t.Error("should not start prompt when no CLs")
	}
}

func TestHandleKey_S_FromChangelistsPanel(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelChangelists
	ctx := KeyContext{CLCount: 1, CLNames: []string{"MyChange"}, UnversionedName: "Unversioned Files"}
	r := HandleKey("s", s, ctx)
	if r.StartPrompt == nil {
		t.Fatal("expected prompt")
	}
	if r.StartPrompt.Mode != types.PromptShelveFiles {
		t.Errorf("expected PromptShelveFiles, got %d", r.StartPrompt.Mode)
	}
}

func TestHandleKey_S_FilesWithSelection(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelFiles
	s.Pivot = types.PanelChangelists
	ctx := KeyContext{CLCount: 1, CLNames: []string{"MyChange"}, SelectedCount: 2, UnversionedName: "Unversioned Files"}
	r := HandleKey("s", s, ctx)
	if r.StartPrompt == nil {
		t.Fatal("expected prompt")
	}
	if r.StartPrompt.DefaultValue != "MyChange" {
		t.Errorf("expected MyChange, got %s", r.StartPrompt.DefaultValue)
	}
}

func TestHandleKey_S_FilesNoSelection(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelFiles
	s.Pivot = types.PanelChangelists
	ctx := KeyContext{CLCount: 1, CLNames: []string{"MyChange"}, SelectedCount: 0, UnversionedName: "Unversioned Files"}
	r := HandleKey("s", s, ctx)
	if r.ErrorMsg == "" {
		t.Error("expected error about selecting files first")
	}
}

// --- handleChangelistKey: "A" amend ---

func TestHandleKey_Amend_WithSelection(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelChangelists
	ctx := KeyContext{CLCount: 1, CLNames: []string{"Changes"}, SelectedCount: 3, LastCommitMsg: "previous commit msg"}
	r := HandleKey("A", s, ctx)
	if r.StartPrompt == nil {
		t.Fatal("expected prompt")
	}
	if r.StartPrompt.Mode != types.PromptAmend {
		t.Errorf("expected PromptAmend, got %d", r.StartPrompt.Mode)
	}
	if r.StartPrompt.DefaultValue != "previous commit msg" {
		t.Errorf("expected previous commit msg, got %s", r.StartPrompt.DefaultValue)
	}
}

func TestHandleKey_Amend_NoSelection(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelChangelists
	ctx := KeyContext{CLCount: 1, CLNames: []string{"Changes"}, SelectedCount: 0}
	r := HandleKey("A", s, ctx)
	if r.ErrorMsg == "" {
		t.Error("expected error about selecting files")
	}
}

// --- handleChangelistKey: "r" rename changelist ---

func TestHandleKey_RenameChangelist(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelChangelists
	ctx := KeyContext{CLCount: 2, CLNames: []string{"MyChange", "Other"}, UnversionedName: "Unversioned Files"}
	r := HandleKey("r", s, ctx)
	if r.StartPrompt == nil {
		t.Fatal("expected prompt")
	}
	if r.StartPrompt.Mode != types.PromptRenameChangelist {
		t.Errorf("expected PromptRenameChangelist, got %d", r.StartPrompt.Mode)
	}
	if r.StartPrompt.DefaultValue != "MyChange" {
		t.Errorf("expected MyChange, got %s", r.StartPrompt.DefaultValue)
	}
}

func TestHandleKey_RenameChangelist_Unversioned(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelChangelists
	ctx := KeyContext{CLCount: 1, CLNames: []string{"Unversioned Files"}, UnversionedName: "Unversioned Files"}
	r := HandleKey("r", s, ctx)
	if r.StartPrompt != nil {
		t.Error("should not allow renaming unversioned")
	}
}

// --- handleChangelistKey: "d" delete changelist ---

func TestHandleKey_DeleteChangelist(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelChangelists
	ctx := KeyContext{CLCount: 2, CLNames: []string{"MyChange", "Default"}, UnversionedName: "Unversioned Files", DefaultName: "Default"}
	r := HandleKey("d", s, ctx)
	if r.StartPrompt == nil {
		t.Fatal("expected confirm prompt")
	}
	if r.StartPrompt.Confirm != types.ConfirmDeleteChangelist {
		t.Errorf("expected ConfirmDeleteChangelist")
	}
	if r.StartPrompt.Target != "MyChange" {
		t.Errorf("expected MyChange, got %s", r.StartPrompt.Target)
	}
}

func TestHandleKey_DeleteChangelist_Unversioned(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelChangelists
	ctx := KeyContext{CLCount: 1, CLNames: []string{"Unversioned Files"}, UnversionedName: "Unversioned Files", DefaultName: "Default"}
	r := HandleKey("d", s, ctx)
	if r.StartPrompt != nil {
		t.Error("should not allow deleting unversioned")
	}
}

func TestHandleKey_DeleteChangelist_Default(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelChangelists
	ctx := KeyContext{CLCount: 1, CLNames: []string{"Default"}, UnversionedName: "Unversioned Files", DefaultName: "Default"}
	r := HandleKey("d", s, ctx)
	if r.StartPrompt != nil {
		t.Error("should not allow deleting default changelist")
	}
}

// --- handleShelfKey: "r" rename shelf ---

func TestHandleKey_RenameShelf(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelShelves
	s.Pivot = types.PanelShelves
	ctx := KeyContext{ShelfCount: 2, ShelfNames: []string{"shelf-1", "shelf-2"}}
	r := HandleKey("r", s, ctx)
	if r.StartPrompt == nil {
		t.Fatal("expected prompt")
	}
	if r.StartPrompt.Mode != types.PromptRenameShelf {
		t.Errorf("expected PromptRenameShelf, got %d", r.StartPrompt.Mode)
	}
	if r.StartPrompt.DefaultValue != "shelf-1" {
		t.Errorf("expected shelf-1, got %s", r.StartPrompt.DefaultValue)
	}
}

// --- MoveDown additional cases ---

func TestMoveDown_Shelves(t *testing.T) {
	s := State{Focus: types.PanelShelves, ShelfSel: 0, Pivot: types.PanelShelves}
	ctx := KeyContext{ShelfCount: 3}
	r := MoveDown(&s, ctx)
	if s.ShelfSel != 1 {
		t.Errorf("expected 1, got %d", s.ShelfSel)
	}
	if r != RefreshShelfFiles {
		t.Errorf("expected RefreshShelfFiles, got %d", r)
	}
}

func TestMoveDown_Shelves_AtBottom(t *testing.T) {
	s := State{Focus: types.PanelShelves, ShelfSel: 2, Pivot: types.PanelShelves}
	ctx := KeyContext{ShelfCount: 3}
	r := MoveDown(&s, ctx)
	if s.ShelfSel != 2 {
		t.Errorf("expected 2, got %d", s.ShelfSel)
	}
	if r != RefreshNone {
		t.Errorf("expected RefreshNone, got %d", r)
	}
}

func TestMoveDown_FilesChangelistContext(t *testing.T) {
	s := State{Focus: types.PanelFiles, CLFileSel: 0, Pivot: types.PanelChangelists}
	ctx := KeyContext{CLFileCount: 3}
	r := MoveDown(&s, ctx)
	if s.CLFileSel != 1 {
		t.Errorf("expected 1, got %d", s.CLFileSel)
	}
	if r != RefreshDiff {
		t.Errorf("expected RefreshDiff, got %d", r)
	}
}

func TestMoveDown_FilesChangelistContext_AtBottom(t *testing.T) {
	s := State{Focus: types.PanelFiles, CLFileSel: 2, Pivot: types.PanelChangelists}
	ctx := KeyContext{CLFileCount: 3}
	r := MoveDown(&s, ctx)
	if s.CLFileSel != 2 {
		t.Errorf("expected 2, got %d", s.CLFileSel)
	}
	if r != RefreshNone {
		t.Errorf("expected RefreshNone, got %d", r)
	}
}

func TestMoveDown_FilesShelfContext(t *testing.T) {
	s := State{Focus: types.PanelFiles, ShelfFileSel: 0, Pivot: types.PanelShelves}
	ctx := KeyContext{ShelfFileCount: 3}
	r := MoveDown(&s, ctx)
	if s.ShelfFileSel != 1 {
		t.Errorf("expected 1, got %d", s.ShelfFileSel)
	}
	if r != RefreshDiff {
		t.Errorf("expected RefreshDiff, got %d", r)
	}
}

func TestMoveDown_FilesShelfContext_AtBottom(t *testing.T) {
	s := State{Focus: types.PanelFiles, ShelfFileSel: 2, Pivot: types.PanelShelves}
	ctx := KeyContext{ShelfFileCount: 3}
	r := MoveDown(&s, ctx)
	if s.ShelfFileSel != 2 {
		t.Errorf("expected 2, got %d", s.ShelfFileSel)
	}
	if r != RefreshNone {
		t.Errorf("expected RefreshNone, got %d", r)
	}
}

func TestMoveDown_LogScrollAtZero(t *testing.T) {
	s := State{Focus: types.PanelLog, LogScroll: 0}
	ctx := KeyContext{}
	r := MoveDown(&s, ctx)
	if s.LogScroll != 0 {
		t.Errorf("expected 0, got %d", s.LogScroll)
	}
	if r != RefreshNone {
		t.Errorf("expected RefreshNone, got %d", r)
	}
}

// --- Push/Pull ---

func TestHandleKey_PushNoRemotes(t *testing.T) {
	s := NewState()
	ctx := KeyContext{}
	r := HandleKey("p", s, ctx)
	if r.RunRemote == nil {
		t.Fatal("expected immediate remote action")
	}
	if r.RunRemote.Mode != types.PromptPush {
		t.Errorf("expected PromptPush, got %d", r.RunRemote.Mode)
	}
	if r.RunRemote.Remote != "origin" {
		t.Errorf("expected origin, got %s", r.RunRemote.Remote)
	}
}

func TestHandleKey_PushSingleRemote(t *testing.T) {
	s := NewState()
	ctx := KeyContext{Remotes: []string{"upstream"}}
	r := HandleKey("p", s, ctx)
	if r.RunRemote == nil {
		t.Fatal("expected immediate remote action")
	}
	if r.RunRemote.Remote != "upstream" {
		t.Errorf("expected upstream, got %s", r.RunRemote.Remote)
	}
}

func TestHandleKey_PushMultipleRemotes(t *testing.T) {
	s := NewState()
	ctx := KeyContext{Remotes: []string{"origin", "upstream", "fork"}}
	r := HandleKey("p", s, ctx)
	if r.StartPrompt == nil {
		t.Fatal("expected prompt for multiple remotes")
	}
	if len(r.StartPrompt.Options) != 3 {
		t.Errorf("expected 3 options, got %d", len(r.StartPrompt.Options))
	}
}

func TestHandleKey_PullNoRemotes(t *testing.T) {
	s := NewState()
	ctx := KeyContext{}
	r := HandleKey("P", s, ctx)
	if r.RunRemote == nil {
		t.Fatal("expected immediate remote action")
	}
	if r.RunRemote.Mode != types.PromptPull {
		t.Errorf("expected PromptPull, got %d", r.RunRemote.Mode)
	}
}

func TestHandleKey_PullMultipleRemotes(t *testing.T) {
	s := NewState()
	ctx := KeyContext{Remotes: []string{"origin", "upstream"}}
	r := HandleKey("P", s, ctx)
	if r.StartPrompt == nil {
		t.Fatal("expected prompt for multiple remotes")
	}
	if len(r.StartPrompt.Options) != 2 {
		t.Errorf("expected 2 options, got %d", len(r.StartPrompt.Options))
	}
}

func TestBuildRemoteAction(t *testing.T) {
	prompt, remote := buildRemoteAction(types.PromptPush, nil)
	if prompt != nil {
		t.Error("expected no prompt for no remotes")
	}
	if remote == nil || remote.Remote != "origin" {
		t.Errorf("expected immediate origin, got %v", remote)
	}

	prompt, remote = buildRemoteAction(types.PromptPush, []string{"myremote"})
	if prompt != nil {
		t.Error("expected no prompt for single remote")
	}
	if remote == nil || remote.Remote != "myremote" {
		t.Errorf("expected immediate myremote, got %v", remote)
	}

	prompt, remote = buildRemoteAction(types.PromptPull, []string{"a", "b"})
	if remote != nil {
		t.Error("expected no immediate action for multiple remotes")
	}
	if prompt == nil || len(prompt.Options) != 2 {
		t.Errorf("expected prompt with 2 options, got %v", prompt)
	}
}

func TestHandleKey_Quit(t *testing.T) {
	for _, key := range []string{"q", "ctrl+c"} {
		kr := HandleKey(key, NewState(), KeyContext{})
		if !kr.Quit {
			t.Errorf("%s: expected quit", key)
		}
	}
}

func TestHandleKey_Tab(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelChangelists
	flow := []types.PanelID{types.PanelChangelists, types.PanelFiles, types.PanelDiff}

	kr := HandleKey("tab", s, KeyContext{TabFlow: flow})
	if kr.State.Focus != types.PanelFiles {
		t.Errorf("expected Files, got %d", kr.State.Focus)
	}
	if kr.Refresh != RefreshAll {
		t.Errorf("expected RefreshAll, got %d", kr.Refresh)
	}
}

func TestHandleKey_ShiftTab(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelFiles
	flow := []types.PanelID{types.PanelChangelists, types.PanelFiles, types.PanelDiff}

	kr := HandleKey("shift+tab", s, KeyContext{TabFlow: flow})
	if kr.State.Focus != types.PanelChangelists {
		t.Errorf("expected Changelists, got %d", kr.State.Focus)
	}
}

func TestHandleKey_PanelSwitch(t *testing.T) {
	s := NewState()

	// "1" → focus + pivot on changelists
	kr := HandleKey("1", s, KeyContext{})
	if kr.State.Focus != types.PanelChangelists || kr.State.Pivot != types.PanelChangelists {
		t.Error("expected focus+pivot on Changelists")
	}

	// "2" → focus + pivot on shelves
	kr = HandleKey("2", s, KeyContext{})
	if kr.State.Focus != types.PanelShelves || kr.State.Pivot != types.PanelShelves {
		t.Error("expected focus+pivot on Shelves")
	}

	// "3" → focus on files, pivot unchanged
	kr = HandleKey("3", s, KeyContext{})
	if kr.State.Focus != types.PanelFiles {
		t.Errorf("expected Files, got %d", kr.State.Focus)
	}
	if kr.State.Pivot != types.PanelChangelists {
		t.Errorf("pivot should not change, got %d", kr.State.Pivot)
	}
}

func TestHandleKey_DiffCycle(t *testing.T) {
	s := NewState()
	s.Focus = types.PanelDiff

	// Focused: Normal → Maximized
	kr := HandleKey("4", s, KeyContext{})
	if kr.State.DiffState != types.PanelMaximized {
		t.Errorf("expected Maximized, got %d", kr.State.DiffState)
	}

	// Focused: Maximized → Hidden, focus moves to pivot
	kr = HandleKey("4", kr.State, KeyContext{})
	if kr.State.DiffState != types.PanelHidden {
		t.Errorf("expected Hidden, got %d", kr.State.DiffState)
	}
	if kr.State.Focus != kr.State.Pivot {
		t.Error("expected focus to move to pivot")
	}

	// Not focused, hidden → Normal + focus
	kr.State.Focus = types.PanelChangelists
	kr = HandleKey("4", kr.State, KeyContext{})
	if kr.State.DiffState != types.PanelNormal {
		t.Errorf("expected Normal, got %d", kr.State.DiffState)
	}
	if kr.State.Focus != types.PanelDiff {
		t.Errorf("expected focus on Diff, got %d", kr.State.Focus)
	}
}

func TestHandleKey_LogCycle(t *testing.T) {
	s := NewState()

	// Not focused → focus on Log
	kr := HandleKey("5", s, KeyContext{})
	if kr.State.Focus != types.PanelLog {
		t.Errorf("expected focus on Log, got %d", kr.State.Focus)
	}

	// Focused: Normal → Maximized
	kr = HandleKey("5", kr.State, KeyContext{})
	if kr.State.LogState != types.PanelMaximized {
		t.Errorf("expected Maximized, got %d", kr.State.LogState)
	}

	// Focused: Maximized → Hidden, focus to pivot
	kr = HandleKey("5", kr.State, KeyContext{})
	if kr.State.LogState != types.PanelHidden {
		t.Errorf("expected Hidden, got %d", kr.State.LogState)
	}
	if kr.State.Focus != kr.State.Pivot {
		t.Error("expected focus to move to pivot")
	}
}

func TestHandleKey_HelpScreen(t *testing.T) {
	ctx := KeyContext{CLCount: 2, CLFileCount: 1}

	t.Run("? opens help", func(t *testing.T) {
		s := NewState()
		kr := HandleKey("?", s, ctx)
		if !kr.State.ShowHelp {
			t.Error("expected ShowHelp true")
		}
		if kr.State.HelpScroll != 0 {
			t.Errorf("expected HelpScroll 0, got %d", kr.State.HelpScroll)
		}
	})

	t.Run("? closes help", func(t *testing.T) {
		s := NewState()
		s.ShowHelp = true
		s.HelpScroll = 5
		kr := HandleKey("?", s, ctx)
		if kr.State.ShowHelp {
			t.Error("expected ShowHelp false")
		}
		if kr.State.HelpScroll != 0 {
			t.Error("expected HelpScroll reset to 0")
		}
	})

	t.Run("q closes help without quitting", func(t *testing.T) {
		s := NewState()
		s.ShowHelp = true
		kr := HandleKey("q", s, ctx)
		if kr.State.ShowHelp {
			t.Error("expected ShowHelp false")
		}
		if kr.Quit {
			t.Error("q in help should not quit the app")
		}
	})

	t.Run("esc closes help", func(t *testing.T) {
		s := NewState()
		s.ShowHelp = true
		kr := HandleKey("esc", s, ctx)
		if kr.State.ShowHelp {
			t.Error("expected ShowHelp false")
		}
	})

	t.Run("scroll down in help", func(t *testing.T) {
		s := NewState()
		s.ShowHelp = true
		kr := HandleKey("j", s, ctx)
		if kr.State.HelpScroll != 1 {
			t.Errorf("expected HelpScroll 1, got %d", kr.State.HelpScroll)
		}
		if !kr.State.ShowHelp {
			t.Error("help should remain open")
		}
	})

	t.Run("scroll up in help", func(t *testing.T) {
		s := NewState()
		s.ShowHelp = true
		s.HelpScroll = 3
		kr := HandleKey("k", s, ctx)
		if kr.State.HelpScroll != 2 {
			t.Errorf("expected HelpScroll 2, got %d", kr.State.HelpScroll)
		}
	})

	t.Run("scroll up at top stays at 0", func(t *testing.T) {
		s := NewState()
		s.ShowHelp = true
		s.HelpScroll = 0
		kr := HandleKey("k", s, ctx)
		if kr.State.HelpScroll != 0 {
			t.Errorf("expected HelpScroll 0, got %d", kr.State.HelpScroll)
		}
	})

	t.Run("other keys ignored in help", func(t *testing.T) {
		s := NewState()
		s.ShowHelp = true
		origFocus := s.Focus
		kr := HandleKey("n", s, ctx)
		if !kr.State.ShowHelp {
			t.Error("help should remain open")
		}
		if kr.State.Focus != origFocus {
			t.Error("focus should not change in help")
		}
		if kr.StartPrompt != nil {
			t.Error("no prompts should start in help mode")
		}
	})
}

// --- CopyPatch: "y" key across all panels ---

func TestHandleKey_CopyPatch(t *testing.T) {
	t.Run("changelists panel", func(t *testing.T) {
		s := NewState()
		s.Focus = types.PanelChangelists
		s.Pivot = types.PanelChangelists
		ctx := KeyContext{CLCount: 2, CLNames: []string{"Changes", "Feature"}}
		r := HandleKey("y", s, ctx)
		if r.CopyPatch.Source != CopyPatchChangelist {
			t.Errorf("expected CopyPatchChangelist, got %d", r.CopyPatch.Source)
		}
	})

	t.Run("changelists panel empty", func(t *testing.T) {
		s := NewState()
		s.Focus = types.PanelChangelists
		ctx := KeyContext{CLCount: 0}
		r := HandleKey("y", s, ctx)
		if r.CopyPatch.Source != CopyPatchNone {
			t.Errorf("expected CopyPatchNone, got %d", r.CopyPatch.Source)
		}
	})

	t.Run("shelves panel", func(t *testing.T) {
		s := NewState()
		s.Focus = types.PanelShelves
		s.Pivot = types.PanelShelves
		ctx := KeyContext{ShelfCount: 1, ShelfNames: []string{"my-shelf"}}
		r := HandleKey("y", s, ctx)
		if r.CopyPatch.Source != CopyPatchShelf {
			t.Errorf("expected CopyPatchShelf, got %d", r.CopyPatch.Source)
		}
	})

	t.Run("shelves panel empty", func(t *testing.T) {
		s := NewState()
		s.Focus = types.PanelShelves
		s.Pivot = types.PanelShelves
		ctx := KeyContext{ShelfCount: 0}
		r := HandleKey("y", s, ctx)
		if r.CopyPatch.Source != CopyPatchNone {
			t.Errorf("expected CopyPatchNone, got %d", r.CopyPatch.Source)
		}
	})

	t.Run("files panel CL context", func(t *testing.T) {
		s := NewState()
		s.Focus = types.PanelFiles
		s.Pivot = types.PanelChangelists
		ctx := KeyContext{CLFileCount: 2, CLFiles: []string{"a.txt", "b.txt"}}
		r := HandleKey("y", s, ctx)
		if r.CopyPatch.Source != CopyPatchFiles {
			t.Errorf("expected CopyPatchFiles, got %d", r.CopyPatch.Source)
		}
	})

	t.Run("files panel CL context empty", func(t *testing.T) {
		s := NewState()
		s.Focus = types.PanelFiles
		s.Pivot = types.PanelChangelists
		ctx := KeyContext{CLFileCount: 0}
		r := HandleKey("y", s, ctx)
		if r.CopyPatch.Source != CopyPatchNone {
			t.Errorf("expected CopyPatchNone, got %d", r.CopyPatch.Source)
		}
	})

	t.Run("files panel shelf context", func(t *testing.T) {
		s := NewState()
		s.Focus = types.PanelFiles
		s.Pivot = types.PanelShelves
		ctx := KeyContext{ShelfFileCount: 1}
		r := HandleKey("y", s, ctx)
		if r.CopyPatch.Source != CopyPatchFiles {
			t.Errorf("expected CopyPatchFiles, got %d", r.CopyPatch.Source)
		}
	})

	t.Run("files panel shelf context empty", func(t *testing.T) {
		s := NewState()
		s.Focus = types.PanelFiles
		s.Pivot = types.PanelShelves
		ctx := KeyContext{ShelfFileCount: 0}
		r := HandleKey("y", s, ctx)
		if r.CopyPatch.Source != CopyPatchNone {
			t.Errorf("expected CopyPatchNone, got %d", r.CopyPatch.Source)
		}
	})

	t.Run("diff panel", func(t *testing.T) {
		s := NewState()
		s.Focus = types.PanelDiff
		ctx := KeyContext{}
		r := HandleKey("y", s, ctx)
		if r.CopyPatch.Source != CopyPatchDiff {
			t.Errorf("expected CopyPatchDiff, got %d", r.CopyPatch.Source)
		}
	})

	t.Run("ignored in help mode", func(t *testing.T) {
		s := NewState()
		s.ShowHelp = true
		s.Focus = types.PanelChangelists
		ctx := KeyContext{CLCount: 1, CLNames: []string{"Changes"}}
		r := HandleKey("y", s, ctx)
		if r.CopyPatch.Source != CopyPatchNone {
			t.Errorf("expected CopyPatchNone in help mode, got %d", r.CopyPatch.Source)
		}
	})
}

// --- HandleKey "6": Worktree panel cycling ---

func TestHandleKey_6(t *testing.T) {
	t.Run("not focused, hidden to normal + focus", func(t *testing.T) {
		s := NewState()
		s.WorktreeState = types.PanelHidden
		s.Focus = types.PanelChangelists
		kr := HandleKey("6", s, KeyContext{})
		if kr.State.WorktreeState != types.PanelNormal {
			t.Errorf("expected Normal, got %d", kr.State.WorktreeState)
		}
		if kr.State.Focus != types.PanelWorktrees {
			t.Errorf("expected focus on Worktrees, got %d", kr.State.Focus)
		}
	})

	t.Run("focused, normal to minimized + focus to pivot", func(t *testing.T) {
		s := NewState()
		s.WorktreeState = types.PanelNormal
		s.Focus = types.PanelWorktrees
		kr := HandleKey("6", s, KeyContext{})
		if kr.State.WorktreeState != types.PanelMinimized {
			t.Errorf("expected Minimized, got %d", kr.State.WorktreeState)
		}
		if kr.State.Focus != kr.State.Pivot {
			t.Errorf("expected focus on pivot, got %d", kr.State.Focus)
		}
	})

	t.Run("focused, minimized to hidden + focus to pivot", func(t *testing.T) {
		s := NewState()
		s.WorktreeState = types.PanelMinimized
		s.Focus = types.PanelWorktrees
		kr := HandleKey("6", s, KeyContext{})
		if kr.State.WorktreeState != types.PanelHidden {
			t.Errorf("expected Hidden, got %d", kr.State.WorktreeState)
		}
		if kr.State.Focus != kr.State.Pivot {
			t.Errorf("expected focus on pivot, got %d", kr.State.Focus)
		}
	})

	t.Run("not focused, normal stays", func(t *testing.T) {
		s := NewState()
		s.WorktreeState = types.PanelNormal
		s.Focus = types.PanelChangelists
		kr := HandleKey("6", s, KeyContext{})
		if kr.State.WorktreeState != types.PanelNormal {
			t.Errorf("expected Normal unchanged, got %d", kr.State.WorktreeState)
		}
		// Should focus the worktrees panel
		if kr.State.Focus != types.PanelWorktrees {
			t.Errorf("expected focus on Worktrees, got %d", kr.State.Focus)
		}
	})
}

// --- Worktree Navigation ---

func TestWorktreeNavigation(t *testing.T) {
	t.Run("MoveDown", func(t *testing.T) {
		s := State{Focus: types.PanelWorktrees, WorktreeSel: 0}
		ctx := KeyContext{WorktreeCount: 3}
		MoveDown(&s, ctx)
		if s.WorktreeSel != 1 {
			t.Errorf("expected 1, got %d", s.WorktreeSel)
		}
	})

	t.Run("MoveDown at bottom", func(t *testing.T) {
		s := State{Focus: types.PanelWorktrees, WorktreeSel: 2}
		ctx := KeyContext{WorktreeCount: 3}
		MoveDown(&s, ctx)
		if s.WorktreeSel != 2 {
			t.Errorf("expected 2, got %d", s.WorktreeSel)
		}
	})

	t.Run("MoveUp", func(t *testing.T) {
		s := State{Focus: types.PanelWorktrees, WorktreeSel: 1}
		ctx := KeyContext{WorktreeCount: 3}
		MoveUp(&s, ctx)
		if s.WorktreeSel != 0 {
			t.Errorf("expected 0, got %d", s.WorktreeSel)
		}
	})

	t.Run("MoveUp at top", func(t *testing.T) {
		s := State{Focus: types.PanelWorktrees, WorktreeSel: 0}
		ctx := KeyContext{WorktreeCount: 3}
		MoveUp(&s, ctx)
		if s.WorktreeSel != 0 {
			t.Errorf("expected 0, got %d", s.WorktreeSel)
		}
	})
}
