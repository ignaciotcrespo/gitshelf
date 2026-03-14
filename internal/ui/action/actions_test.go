package action

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ignaciotcrespo/gitshelf/internal/changelist"
	"github.com/ignaciotcrespo/gitshelf/internal/git"
	"github.com/ignaciotcrespo/gitshelf/internal/shelf"
	"github.com/ignaciotcrespo/gitshelf/internal/types"
	"github.com/ignaciotcrespo/gitshelf/internal/ui/prompt"
)

// testLogger implements Logger for testing.
type testLogger struct {
	status string
	err    string
}

func (l *testLogger) SetStatus(msg string) { l.status = msg }
func (l *testLogger) SetError(msg string)  { l.err = msg }

func setupActionRepo(t *testing.T) (dir string, stores *Stores) {
	t.Helper()
	dir = t.TempDir()

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "config", "core.autocrlf", "false"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup %v failed: %v\n%s", args, err, out)
		}
	}

	// Create initial commit
	if err := os.WriteFile(filepath.Join(dir, "initial.txt"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "initial"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup %v failed: %v\n%s", args, err, out)
		}
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	git.SetRepoRoot(dir)
	git.ClearLog()
	t.Cleanup(func() {
		git.SetRepoRoot("")
		os.Chdir(origDir)
	})

	gitshelfDir := filepath.Join(dir, ".git", "gitshelf")
	clStore := changelist.NewStore(gitshelfDir)
	shelfStore := shelf.NewStore(gitshelfDir)
	state := &changelist.State{
		Active: changelist.DefaultName,
		Changelists: []changelist.Changelist{
			{Name: changelist.DefaultName, Files: []string{"initial.txt"}},
			{Name: changelist.UnversionedName},
		},
	}

	return dir, &Stores{
		CL:    clStore,
		Shelf: shelfStore,
		State: state,
	}
}

func TestExecuteNilResult(t *testing.T) {
	_, stores := setupActionRepo(t)
	log := &testLogger{}

	modified := Execute(nil, stores, log, nil)
	if modified {
		t.Error("Execute(nil) should return false")
	}
}

func TestExecuteCommit(t *testing.T) {
	dir, stores := setupActionRepo(t)
	log := &testLogger{}

	// Modify file
	if err := os.WriteFile(filepath.Join(dir, "initial.txt"), []byte("committed\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result := &prompt.Result{
		Mode:  types.PromptCommit,
		Value: "test commit",
	}
	ctx := &ActionContext{
		SelectedFiles: map[string]bool{"initial.txt": true},
		CLName:        changelist.DefaultName,
	}

	modified := Execute(result, stores, log, ctx)
	if !modified {
		t.Error("Execute(commit) should return true")
	}
	if log.err != "" {
		t.Errorf("unexpected error: %s", log.err)
	}
	if !strings.Contains(log.status, "Commit") {
		t.Errorf("status = %q, expected to contain 'Commit'", log.status)
	}

	// Verify commit was made
	msg := git.LastCommitMessage()
	if msg != "test commit" {
		t.Errorf("commit message = %q, want %q", msg, "test commit")
	}

	// File should be removed from changelist
	for _, f := range stores.State.Changelists[0].Files {
		if f == "initial.txt" {
			t.Error("committed file should be removed from changelist")
		}
	}
}

func TestExecuteCommit_NoFiles(t *testing.T) {
	_, stores := setupActionRepo(t)
	log := &testLogger{}

	result := &prompt.Result{
		Mode:  types.PromptCommit,
		Value: "no files",
	}
	ctx := &ActionContext{
		SelectedFiles: map[string]bool{},
	}

	modified := Execute(result, stores, log, ctx)
	if modified {
		t.Error("Execute(commit) with no files should return false")
	}
	if log.err == "" {
		t.Error("expected error for commit with no files")
	}
}

func TestExecuteAmend(t *testing.T) {
	dir, stores := setupActionRepo(t)
	log := &testLogger{}

	// Modify file
	if err := os.WriteFile(filepath.Join(dir, "initial.txt"), []byte("amended\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result := &prompt.Result{
		Mode:  types.PromptAmend,
		Value: "amended message",
	}
	ctx := &ActionContext{
		SelectedFiles: map[string]bool{"initial.txt": true},
		CLName:        changelist.DefaultName,
	}

	modified := Execute(result, stores, log, ctx)
	if !modified {
		t.Error("Execute(amend) should return true")
	}

	msg := git.LastCommitMessage()
	if msg != "amended message" {
		t.Errorf("after amend, message = %q, want %q", msg, "amended message")
	}
}

func TestExecuteShelve(t *testing.T) {
	dir, stores := setupActionRepo(t)
	log := &testLogger{}

	// Modify file
	if err := os.WriteFile(filepath.Join(dir, "initial.txt"), []byte("shelved\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result := &prompt.Result{
		Mode:  types.PromptShelveFiles,
		Value: "my-shelf",
	}
	ctx := &ActionContext{
		SelectedFiles: map[string]bool{"initial.txt": true},
		CLName:        changelist.DefaultName,
	}

	modified := Execute(result, stores, log, ctx)
	if !modified {
		t.Error("Execute(shelve) should return true")
	}
	if log.err != "" {
		t.Errorf("unexpected error: %s", log.err)
	}
	if !strings.Contains(log.status, "Shelve") {
		t.Errorf("status = %q, expected to contain 'Shelve'", log.status)
	}

	// File should be restored (shelve with restore=true)
	data, err := os.ReadFile(filepath.Join(dir, "initial.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello\n" {
		t.Errorf("after shelve, file content = %q, want %q", string(data), "hello\n")
	}

	// File should be removed from changelist
	for _, f := range stores.State.Changelists[0].Files {
		if f == "initial.txt" {
			t.Error("shelved file should be removed from changelist")
		}
	}
}

func TestExecuteNewChangelist(t *testing.T) {
	_, stores := setupActionRepo(t)
	log := &testLogger{}

	result := &prompt.Result{
		Mode:  types.PromptNewChangelist,
		Value: "Feature X",
	}

	modified := Execute(result, stores, log, nil)
	if !modified {
		t.Error("Execute(new changelist) should return true")
	}

	found := false
	for _, cl := range stores.State.Changelists {
		if cl.Name == "Feature X" {
			found = true
		}
	}
	if !found {
		t.Error("new changelist not added to state")
	}
}

func TestExecuteRenameChangelist(t *testing.T) {
	_, stores := setupActionRepo(t)
	log := &testLogger{}

	result := &prompt.Result{
		Mode:  types.PromptRenameChangelist,
		Value: "Renamed CL",
	}
	ctx := &ActionContext{
		OldName: changelist.DefaultName,
	}

	modified := Execute(result, stores, log, ctx)
	if !modified {
		t.Error("Execute(rename changelist) should return true")
	}

	found := false
	for _, cl := range stores.State.Changelists {
		if cl.Name == "Renamed CL" {
			found = true
		}
	}
	if !found {
		t.Error("changelist not renamed in state")
	}
}

func TestExecuteDeleteChangelist(t *testing.T) {
	_, stores := setupActionRepo(t)
	log := &testLogger{}

	// Add a changelist to delete
	changelist.AddChangelist(stores.State, "To Delete")

	result := &prompt.Result{
		Mode:          types.PromptConfirm,
		Confirmed:     true,
		ConfirmAction: types.ConfirmDeleteChangelist,
		ConfirmTarget: "To Delete",
	}

	modified := Execute(result, stores, log, nil)
	if !modified {
		t.Error("Execute(delete changelist) should return true")
	}

	for _, cl := range stores.State.Changelists {
		if cl.Name == "To Delete" {
			t.Error("deleted changelist still in state")
		}
	}
}

func TestExecuteConfirmNotConfirmed(t *testing.T) {
	_, stores := setupActionRepo(t)
	log := &testLogger{}

	result := &prompt.Result{
		Mode:          types.PromptConfirm,
		Confirmed:     false,
		ConfirmAction: types.ConfirmDeleteChangelist,
		ConfirmTarget: "Something",
	}

	modified := Execute(result, stores, log, nil)
	if modified {
		t.Error("unconfirmed action should return false")
	}
}

func TestExecuteDropShelf(t *testing.T) {
	dir, stores := setupActionRepo(t)
	log := &testLogger{}

	// Create a shelf first
	if err := os.WriteFile(filepath.Join(dir, "initial.txt"), []byte("to drop\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := stores.Shelf.Create("drop-me", []string{"initial.txt"}, false); err != nil {
		t.Fatal(err)
	}

	// Get the shelf dir for the drop operation
	shelves, err := stores.Shelf.List()
	if err != nil || len(shelves) == 0 {
		t.Fatal("shelf should exist before drop")
	}

	result := &prompt.Result{
		Mode:          types.PromptConfirm,
		Confirmed:     true,
		ConfirmAction: types.ConfirmDropShelf,
		ConfirmTarget: "drop-me",
	}
	ctx := &ActionContext{
		ShelfDir: shelves[0].PatchDir,
	}

	modified := Execute(result, stores, log, ctx)
	if !modified {
		t.Error("Execute(drop shelf) should return true")
	}
	if !strings.Contains(log.status, "Dropped") {
		t.Errorf("status = %q, expected to contain 'Dropped'", log.status)
	}

	// Verify shelf is gone
	shelves2, err := stores.Shelf.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(shelves2) != 0 {
		t.Error("shelf should be dropped")
	}
}

func TestExecuteRenameShelf(t *testing.T) {
	dir, stores := setupActionRepo(t)
	log := &testLogger{}

	// Create a shelf
	if err := os.WriteFile(filepath.Join(dir, "initial.txt"), []byte("rename shelf\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := stores.Shelf.Create("old-shelf", []string{"initial.txt"}, false); err != nil {
		t.Fatal(err)
	}

	// Get the shelf dir for rename
	shelves, err := stores.Shelf.List()
	if err != nil || len(shelves) == 0 {
		t.Fatal("shelf should exist before rename")
	}

	result := &prompt.Result{
		Mode:  types.PromptRenameShelf,
		Value: "new-shelf",
	}
	ctx := &ActionContext{
		OldName:  "old-shelf",
		ShelfDir: shelves[0].PatchDir,
	}

	modified := Execute(result, stores, log, ctx)
	if !modified {
		t.Error("Execute(rename shelf) should return true")
	}

	shelves2, err := stores.Shelf.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(shelves2) != 1 || shelves2[0].Meta.Name != "new-shelf" {
		t.Errorf("shelf name = %q, want %q", shelves2[0].Meta.Name, "new-shelf")
	}
}

func TestExecuteMoveFile(t *testing.T) {
	_, stores := setupActionRepo(t)
	log := &testLogger{}

	result := &prompt.Result{
		Mode:  types.PromptMoveFile,
		Value: "New CL",
	}
	ctx := &ActionContext{
		MoveFile: "initial.txt",
	}

	modified := Execute(result, stores, log, ctx)
	if !modified {
		t.Error("Execute(move) should return true")
	}

	// File should be in "New CL"
	found := false
	for _, cl := range stores.State.Changelists {
		if cl.Name == "New CL" {
			for _, f := range cl.Files {
				if f == "initial.txt" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("file not moved to new changelist")
	}
}

func TestExecutePasteOnlyCL(t *testing.T) {
	_, stores := setupActionRepo(t)
	log := &testLogger{}

	// Add another CL with a file that will be reassigned
	changelist.AddChangelist(stores.State, "Other CL")
	changelist.AssignFile(stores.State, "initial.txt", "Other CL")

	result := &prompt.Result{
		Mode:  types.PromptPasteChangelist,
		Value: types.PasteOnlyCL,
	}
	ctx := &ActionContext{
		ClipboardCLName: "Pasted CL",
		ClipboardFiles:  []string{"initial.txt"},
	}

	modified := Execute(result, stores, log, ctx)
	if !modified {
		t.Error("Execute(paste only CL) should return true")
	}
	if !strings.Contains(log.status, "Pasted") {
		t.Errorf("status = %q, expected to contain 'Pasted'", log.status)
	}

	// initial.txt should be in "Pasted CL", not in "Other CL"
	for _, cl := range stores.State.Changelists {
		if cl.Name == "Other CL" {
			for _, f := range cl.Files {
				if f == "initial.txt" {
					t.Error("initial.txt should have been removed from 'Other CL'")
				}
			}
		}
		if cl.Name == "Pasted CL" {
			found := false
			for _, f := range cl.Files {
				if f == "initial.txt" {
					found = true
				}
			}
			if !found {
				t.Error("initial.txt should be in 'Pasted CL'")
			}
		}
	}
}

func TestExecutePasteFullContent(t *testing.T) {
	dir, stores := setupActionRepo(t)
	log := &testLogger{}

	// Create a source worktree directory with a file
	srcDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(srcDir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "initial.txt"), []byte("from source\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result := &prompt.Result{
		Mode:  types.PromptPasteChangelist,
		Value: types.PasteFullContent,
	}
	ctx := &ActionContext{
		SourceWorktreePath: srcDir,
		ClipboardCLName:    "Pasted CL",
		ClipboardFiles:     []string{"initial.txt"},
	}

	modified := Execute(result, stores, log, ctx)
	if !modified {
		t.Error("Execute(paste full content) should return true")
	}
	if log.err != "" {
		t.Errorf("unexpected error: %s", log.err)
	}

	// Verify file was copied
	data, err := os.ReadFile(filepath.Join(dir, "initial.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "from source\n" {
		t.Errorf("file content = %q, want %q", string(data), "from source\n")
	}

	// Verify CL was created
	found := false
	for _, cl := range stores.State.Changelists {
		if cl.Name == "Pasted CL" {
			found = true
		}
	}
	if !found {
		t.Error("changelist 'Pasted CL' not created")
	}
}

func TestExecutePasteNilClipboard(t *testing.T) {
	_, stores := setupActionRepo(t)
	log := &testLogger{}

	result := &prompt.Result{
		Mode:  types.PromptPasteChangelist,
		Value: types.PasteOnlyCL,
	}
	ctx := &ActionContext{}

	modified := Execute(result, stores, log, ctx)
	if modified {
		t.Error("Execute(paste with empty clipboard) should return false")
	}
	if log.err == "" {
		t.Error("expected error for empty clipboard")
	}
}

func TestExecuteMoveSelectedFiles(t *testing.T) {
	_, stores := setupActionRepo(t)
	log := &testLogger{}

	// Add another file to the state
	stores.State.Changelists[0].Files = append(stores.State.Changelists[0].Files, "other.txt")

	result := &prompt.Result{
		Mode:  types.PromptMoveFile,
		Value: "Target CL",
	}
	ctx := &ActionContext{
		SelectedFiles: map[string]bool{"initial.txt": true, "other.txt": true},
	}

	modified := Execute(result, stores, log, ctx)
	if !modified {
		t.Error("Execute(move selected) should return true")
	}
	if !strings.Contains(log.status, "2 file(s)") {
		t.Errorf("status = %q, expected to contain '2 file(s)'", log.status)
	}
}
