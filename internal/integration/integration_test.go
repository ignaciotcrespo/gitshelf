package integration_test

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ignaciotcrespo/gitshelf/internal/changelist"
	"github.com/ignaciotcrespo/gitshelf/internal/controller"
	"github.com/ignaciotcrespo/gitshelf/internal/diff"
	"github.com/ignaciotcrespo/gitshelf/internal/git"
	"github.com/ignaciotcrespo/gitshelf/internal/shelf"
	"github.com/ignaciotcrespo/gitshelf/internal/types"
	"github.com/ignaciotcrespo/gitshelf/internal/ui/action"
	"github.com/ignaciotcrespo/gitshelf/internal/ui/prompt"
	"github.com/ignaciotcrespo/gitshelf/pkg/tui"
)

// ---------------------------------------------------------------------------
// testLogger
// ---------------------------------------------------------------------------

type testLogger struct {
	t      *testing.T
	status string
	err    string
}

func (l *testLogger) SetStatus(msg string) {
	l.t.Helper()
	git.AddUserLog(msg, "")
	l.status = msg
}
func (l *testLogger) SetError(msg string) {
	l.t.Helper()
	git.AddUserLog("", msg)
	l.err = msg
}

// ---------------------------------------------------------------------------
// TestApp — headless UI
// ---------------------------------------------------------------------------

type TestApp struct {
	t   *testing.T
	dir string

	stores  action.Stores
	logger  *testLogger
	state   controller.State
	clState *changelist.State

	// Loaded data (mirrors Model fields)
	clNames    []string
	clFiles    []string
	shelves    []shelf.Shelf
	shelfFiles []string
	dirtyFiles map[string]bool
	dirtyCLs   map[string]bool

	// Diff (mirrors Model.diff)
	diff string

	// Prompt flow
	prompt        tui.Prompt
	pending       *prompt.Result
	pendingCtx    *action.ActionContext
}

// newTestApp creates a temp git repo with an initial commit and returns a TestApp.
func newTestApp(t *testing.T) *TestApp {
	t.Helper()
	dir := t.TempDir()

	// Init repo + initial commit
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@test.com")
	run(t, dir, "git", "config", "user.name", "Test")
	run(t, dir, "git", "config", "core.autocrlf", "false")
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("init"), 0644)
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "initial")

	gitshelfDir := filepath.Join(dir, ".gitshelf")
	os.MkdirAll(gitshelfDir, 0755)

	clStore := changelist.NewStore(gitshelfDir)
	shelfStore := shelf.NewStore(gitshelfDir)

	git.SetRepoRoot(dir)
	git.ClearLog()
	t.Cleanup(func() { git.SetRepoRoot("") })

	app := &TestApp{
		t:      t,
		dir:    dir,
		state:  controller.NewState(),
		logger: &testLogger{t: t},
		stores: action.Stores{
			CL:    clStore,
			Shelf: shelfStore,
		},
		prompt: tui.NewPrompt(gitshelfLabeler{}, types.PromptConfirm),
	}

	app.refresh()
	return app
}

// newTestAppWithRemote creates a bare remote, clones it, sets up an initial commit.
func newTestAppWithRemote(t *testing.T) *TestApp {
	t.Helper()
	base := t.TempDir()

	bareDir := filepath.Join(base, "bare.git")
	workDir := filepath.Join(base, "work")

	// Create bare repo
	run(t, base, "git", "init", "--bare", bareDir)
	// Clone it
	run(t, base, "git", "clone", bareDir, workDir)
	// Configure
	run(t, workDir, "git", "config", "user.email", "test@test.com")
	run(t, workDir, "git", "config", "user.name", "Test")
	run(t, workDir, "git", "config", "core.autocrlf", "false")

	// Initial commit + push (use HEAD to avoid hardcoding branch name)
	os.WriteFile(filepath.Join(workDir, "README.md"), []byte("init"), 0644)
	run(t, workDir, "git", "add", ".")
	run(t, workDir, "git", "commit", "-m", "initial")
	run(t, workDir, "git", "push", "origin", "HEAD")

	gitshelfDir := filepath.Join(workDir, ".gitshelf")
	os.MkdirAll(gitshelfDir, 0755)

	clStore := changelist.NewStore(gitshelfDir)
	shelfStore := shelf.NewStore(gitshelfDir)

	git.SetRepoRoot(workDir)
	git.ClearLog()
	t.Cleanup(func() { git.SetRepoRoot("") })

	app := &TestApp{
		t:      t,
		dir:    workDir,
		state:  controller.NewState(),
		logger: &testLogger{t: t},
		stores: action.Stores{
			CL:    clStore,
			Shelf: shelfStore,
		},
		prompt: tui.NewPrompt(gitshelfLabeler{}, types.PromptConfirm),
	}
	app.refresh()
	return app
}

// ---------------------------------------------------------------------------
// TestApp methods — data loading (mirrors loader.go)
// ---------------------------------------------------------------------------

func (app *TestApp) refresh() {
	app.t.Helper()

	state, err := app.stores.CL.Load()
	if err != nil {
		app.t.Fatalf("load CL: %v", err)
	}
	app.clState = state
	app.stores.State = state

	if err := changelist.AutoAssignNewFiles(state); err != nil {
		app.t.Fatalf("auto assign: %v", err)
	}

	currentHashes := git.FileDiffHashes()
	app.dirtyFiles, app.dirtyCLs = changelist.ComputeDirty(state, currentHashes)

	if err := app.stores.CL.Save(state); err != nil {
		app.t.Fatalf("save CL: %v", err)
	}

	app.clNames = changelist.AllNames(state)
	if app.state.CLSelected >= len(app.clNames) {
		app.state.CLSelected = max(0, len(app.clNames)-1)
	}
	app.loadCLFiles()
	app.loadShelves()
}

func (app *TestApp) loadCLFiles() {
	if app.clState == nil || len(app.clNames) == 0 {
		app.clFiles = nil
		return
	}
	name := app.clNames[app.state.CLSelected]
	files, err := changelist.FilesForChangelist(app.clState, name)
	if err != nil {
		app.t.Fatalf("files for CL: %v", err)
	}
	app.clFiles = files
	if app.state.CLFileSel >= len(app.clFiles) {
		app.state.CLFileSel = max(0, len(app.clFiles)-1)
	}
	app.state.SelectedFiles = make(map[string]bool)
}

func (app *TestApp) loadShelves() {
	shelves, err := app.stores.Shelf.List()
	if err != nil {
		app.t.Fatalf("list shelves: %v", err)
	}
	app.shelves = shelves
	if app.state.ShelfSel >= len(app.shelves) {
		app.state.ShelfSel = max(0, len(app.shelves)-1)
	}
	app.loadShelfFiles()
}

func (app *TestApp) loadShelfFiles() {
	if len(app.shelves) == 0 {
		app.shelfFiles = nil
		return
	}
	app.shelfFiles = app.shelves[app.state.ShelfSel].Meta.Files
	if app.state.ShelfFileSel >= len(app.shelfFiles) {
		app.state.ShelfFileSel = max(0, len(app.shelfFiles)-1)
	}
}

func (app *TestApp) loadDiff() {
	if controller.IsChangelistContext(app.state) {
		if len(app.clFiles) > 0 && app.state.CLFileSel < len(app.clFiles) {
			file := app.clFiles[app.state.CLFileSel]
			d, err := git.DiffFile(file)
			if err != nil {
				app.diff = ""
				return
			}
			app.diff = d
			return
		}
	} else {
		if len(app.shelfFiles) > 0 && app.state.ShelfFileSel < len(app.shelfFiles) {
			file := app.shelfFiles[app.state.ShelfFileSel]
			patch, err := app.stores.Shelf.GetPatchDir(app.shelves[app.state.ShelfSel].PatchDir)
			if err == nil {
				app.diff = diff.ExtractFileDiff(patch, file)
			}
			return
		}
	}
	app.diff = ""
}

// CopyPatch simulates pressing 'y' and returns the patch that would be copied.
// Mirrors handleCopyPatch in app.go.
func (app *TestApp) CopyPatch() string {
	app.t.Helper()

	keyCtx := app.buildKeyContext()
	kr := controller.HandleKey("y", app.state, keyCtx)
	app.state = kr.State

	var patch string
	switch kr.CopyPatch.Source {
	case controller.CopyPatchChangelist:
		clName := app.clNames[app.state.CLSelected]
		files, _ := changelist.FilesForChangelist(app.clState, clName)
		d, err := git.DiffFiles(files...)
		if err != nil {
			app.t.Fatalf("DiffFiles: %v", err)
		}
		patch = d

	case controller.CopyPatchShelf:
		d, err := app.stores.Shelf.GetPatchDir(app.shelves[app.state.ShelfSel].PatchDir)
		if err != nil {
			app.t.Fatalf("GetPatch: %v", err)
		}
		patch = d

	case controller.CopyPatchFiles:
		if controller.IsChangelistContext(app.state) {
			var files []string
			if len(app.state.SelectedFiles) > 0 {
				for f := range app.state.SelectedFiles {
					files = append(files, f)
				}
			} else if len(app.clFiles) > 0 && app.state.CLFileSel < len(app.clFiles) {
				files = []string{app.clFiles[app.state.CLFileSel]}
			}
			d, err := git.DiffFiles(files...)
			if err != nil {
				app.t.Fatalf("DiffFiles: %v", err)
			}
			patch = d
		} else {
			file := app.shelfFiles[app.state.ShelfFileSel]
			fullPatch, err := app.stores.Shelf.GetPatchDir(app.shelves[app.state.ShelfSel].PatchDir)
			if err != nil {
				app.t.Fatalf("GetPatch: %v", err)
			}
			patch = diff.ExtractFileDiff(fullPatch, file)
		}

	case controller.CopyPatchDiff:
		patch = app.diff

	default:
		return ""
	}

	// Ensure trailing newline (same as app.go)
	if len(patch) > 0 && !strings.HasSuffix(patch, "\n") {
		patch += "\n"
	}
	return patch
}

func (app *TestApp) applyRefresh(flag controller.RefreshFlag) {
	switch {
	case flag&controller.RefreshAll != 0:
		app.refresh()
	case flag&controller.RefreshCLFiles != 0:
		app.loadCLFiles()
	case flag&controller.RefreshShelfFiles != 0:
		app.loadShelfFiles()
	}
}

// ---------------------------------------------------------------------------
// TestApp methods — key context (mirrors app.go:buildKeyContext)
// ---------------------------------------------------------------------------

func (app *TestApp) buildKeyContext() controller.KeyContext {
	tabFlow := []types.PanelID{app.state.Pivot, types.PanelFiles}
	if app.state.DiffState != types.PanelHidden {
		tabFlow = append(tabFlow, types.PanelDiff)
	}
	ctx := controller.KeyContext{
		CLCount:         len(app.clNames),
		CLFileCount:     len(app.clFiles),
		CLNames:         app.clNames,
		CLFiles:         app.clFiles,
		ShelfCount:      len(app.shelves),
		ShelfFileCount:  len(app.shelfFiles),
		SelectedCount:   len(app.state.SelectedFiles),
		UnversionedName: changelist.UnversionedName,
		DefaultName:     changelist.DefaultName,
		LastCommitMsg:   git.LastCommitMessage(),
		Remotes:         git.Remotes(),
		TabFlow:         tabFlow,
		DirtyFiles:      app.dirtyFiles,
		DirtyCLs:        app.dirtyCLs,
	}
	ctx.ShelfNames = make([]string, len(app.shelves))
	ctx.ShelfDirs = make([]string, len(app.shelves))
	for i, s := range app.shelves {
		ctx.ShelfNames[i] = s.Meta.Name
		ctx.ShelfDirs[i] = s.PatchDir
	}
	if app.clState != nil {
		ctx.ActiveCL = app.clState.Active
	}
	return ctx
}

func (app *TestApp) buildActionContext(r *prompt.Result) *action.ActionContext {
	ctx := &action.ActionContext{
		SelectedFiles: app.state.SelectedFiles,
	}
	if len(app.clNames) > 0 && app.state.CLSelected < len(app.clNames) {
		ctx.CLName = app.clNames[app.state.CLSelected]
	}
	switch r.Mode {
	case types.PromptRenameChangelist:
		if len(app.clNames) > 0 {
			ctx.OldName = app.clNames[app.state.CLSelected]
		}
	case types.PromptRenameShelf:
		if len(app.shelves) > 0 {
			ctx.OldName = app.shelves[app.state.ShelfSel].Meta.Name
			ctx.ShelfDir = app.shelves[app.state.ShelfSel].PatchDir
		}
	case types.PromptUnshelve:
		if len(app.shelves) > 0 {
			ctx.ShelfName = app.shelves[app.state.ShelfSel].Meta.Name
			ctx.ShelfDir = app.shelves[app.state.ShelfSel].PatchDir
		}
	}
	ctx.MoveFile = app.state.MoveFile
	ctx.DirtyFiles = app.dirtyFiles
	return ctx
}

// ---------------------------------------------------------------------------
// TestApp methods — user actions
// ---------------------------------------------------------------------------

// PressKey simulates a key press through the controller.
func (app *TestApp) PressKey(key string) {
	app.t.Helper()

	keyCtx := app.buildKeyContext()
	kr := controller.HandleKey(key, app.state, keyCtx)
	app.state = kr.State

	if kr.SetActive != "" {
		app.clState.Active = kr.SetActive
		if err := app.stores.CL.Save(app.clState); err != nil {
			app.t.Fatalf("save CL: %v", err)
		}
	}
	if kr.RunRemote != nil {
		result := &prompt.Result{Mode: kr.RunRemote.Mode, Value: kr.RunRemote.Remote}
		action.Execute(result, &app.stores, app.logger, nil)
		app.applyRefresh(kr.Refresh)
		app.refresh()
		return
	}
	if kr.StartPrompt != nil {
		app.startPrompt(kr.StartPrompt)
		app.applyRefresh(kr.Refresh)
		return
	}
	app.applyRefresh(kr.Refresh)
}

func (app *TestApp) startPrompt(req *controller.PromptReq) {
	if req.Mode == types.PromptConfirm {
		app.prompt.StartConfirm(req.Confirm, req.Target)
	} else if len(req.Options) > 0 {
		app.prompt.StartWithOptions(req.Mode, req.DefaultValue, req.Options)
	} else {
		app.prompt.Start(req.Mode, req.DefaultValue)
	}
}

// TypePrompt simulates typing a value and pressing enter in a prompt.
func (app *TestApp) TypePrompt(value string) {
	app.t.Helper()
	if !app.prompt.Active() {
		app.t.Fatal("TypePrompt: no active prompt")
	}

	mode := app.prompt.Mode
	result := &prompt.Result{
		Mode:  mode,
		Value: value,
	}
	app.handlePromptResult(result)
}

// Confirm simulates pressing 'y' on a confirmation prompt.
func (app *TestApp) Confirm() {
	app.t.Helper()
	if !app.prompt.Active() {
		app.t.Fatal("Confirm: no active prompt")
	}

	if app.prompt.Mode == types.PromptConfirm {
		result := &prompt.Result{
			Mode:          types.PromptConfirm,
			Confirmed:     true,
			ConfirmAction: app.prompt.ConfirmAction,
			ConfirmTarget: app.prompt.ConfirmTarget,
		}
		app.prompt.Cancel()
		app.handlePromptResult(result)
		return
	}
	app.t.Fatal("Confirm: not in confirm mode")
}

// handlePromptResult mirrors app.go:handlePromptResult.
func (app *TestApp) handlePromptResult(result *prompt.Result) {
	ctx := app.buildActionContext(result)

	switch result.Mode {
	case types.PromptShelveFiles:
		fileCount := 0
		if len(ctx.SelectedFiles) > 0 {
			fileCount = len(ctx.SelectedFiles)
		} else if app.clState != nil {
			for _, cl := range app.clState.Changelists {
				if cl.Name == ctx.CLName {
					fileCount = len(cl.Files)
					break
				}
			}
		}
		app.pending = result
		app.pendingCtx = ctx
		app.prompt.StartConfirm(types.ConfirmShelve,
			result.Value+":"+itoa(fileCount))

	case types.PromptUnshelve:
		var conflicting int
		if ctx.ShelfDir != "" {
			changed := git.ChangedFileSet()
			for _, s := range app.shelves {
				if s.PatchDir == ctx.ShelfDir {
					for _, f := range s.Meta.Files {
						if changed[f] {
							conflicting++
						}
					}
					break
				}
			}
		}
		if conflicting > 0 {
			app.pending = result
			app.pendingCtx = ctx
			app.prompt.StartConfirm(types.ConfirmUnshelve,
				ctx.ShelfName+":"+itoa(len(app.shelfFiles))+":"+itoa(conflicting))
		} else {
			action.Execute(result, &app.stores, app.logger, ctx)
			app.refresh()
		}

	case types.PromptConfirm:
		if result.Confirmed {
			switch result.ConfirmAction {
			case types.ConfirmShelve:
				if app.pending != nil {
					action.Execute(app.pending, &app.stores, app.logger, app.pendingCtx)
					app.refresh()
				}
			case types.ConfirmUnshelve:
				if app.pending != nil {
					app.pendingCtx.ForceUnshelve = true
					action.Execute(app.pending, &app.stores, app.logger, app.pendingCtx)
					app.refresh()
				}
			default:
				if result.ConfirmAction == types.ConfirmDropShelf && len(app.shelves) > 0 && app.state.ShelfSel < len(app.shelves) {
					ctx.ShelfDir = app.shelves[app.state.ShelfSel].PatchDir
				}
				action.Execute(result, &app.stores, app.logger, ctx)
				app.refresh()
			}
		}
		app.pending = nil
		app.pendingCtx = nil

	default:
		action.Execute(result, &app.stores, app.logger, ctx)
		app.refresh()
	}
}

// SelectFile selects a file by index in the files panel.
func (app *TestApp) SelectFile(idx int) {
	app.t.Helper()
	app.state.Focus = types.PanelFiles
	if idx >= len(app.clFiles) {
		app.t.Fatalf("SelectFile: index %d out of range (have %d files)", idx, len(app.clFiles))
	}
	app.state.CLFileSel = idx
	file := app.clFiles[idx]
	if app.state.SelectedFiles == nil {
		app.state.SelectedFiles = make(map[string]bool)
	}
	app.state.SelectedFiles[file] = true
}

// WriteFile writes a file in the repo working tree.
func (app *TestApp) WriteFile(name, content string) {
	app.t.Helper()
	path := filepath.Join(app.dir, name)
	os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		app.t.Fatalf("write file: %v", err)
	}
}

// WriteTrackedFile writes a file and stages it so it appears as a tracked change.
// This ensures it goes into the active CL (e.g. "Changes") rather than "Unversioned Files".
func (app *TestApp) WriteTrackedFile(name, content string) {
	app.t.Helper()
	app.WriteFile(name, content)
	run(app.t, app.dir, "git", "add", name)
}

// fileIndex returns the index of a file in clFiles, or -1.
func (app *TestApp) fileIndex(name string) int {
	for i, f := range app.clFiles {
		if f == name {
			return i
		}
	}
	return -1
}

// selectCL navigates to a changelist by name.
func (app *TestApp) selectCL(name string) {
	app.t.Helper()
	app.state.Focus = types.PanelChangelists
	for i, n := range app.clNames {
		if n == name {
			app.state.CLSelected = i
			app.loadCLFiles()
			return
		}
	}
	app.t.Fatalf("selectCL: CL %q not found in %v", name, app.clNames)
}

// selectAllFiles selects all files in current CL's file list.
func (app *TestApp) selectAllFiles() {
	app.t.Helper()
	app.state.Focus = types.PanelFiles
	for i := range app.clFiles {
		app.SelectFile(i)
	}
}

// ---------------------------------------------------------------------------
// Git verification helpers
// ---------------------------------------------------------------------------

func (app *TestApp) gitLog() []string {
	app.t.Helper()
	out := runOut(app.t, app.dir, "git", "log", "--oneline", "--format=%s")
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

func (app *TestApp) gitStatus() string {
	app.t.Helper()
	return runOut(app.t, app.dir, "git", "status", "--porcelain")
}

func (app *TestApp) gitShow(file string) string {
	app.t.Helper()
	return runOut(app.t, app.dir, "git", "show", "HEAD:"+file)
}

func (app *TestApp) fileContent(path string) string {
	app.t.Helper()
	data, err := os.ReadFile(filepath.Join(app.dir, path))
	if err != nil {
		app.t.Fatalf("read file %s: %v", path, err)
	}
	return string(data)
}

func (app *TestApp) fileExists(path string) bool {
	_, err := os.Stat(filepath.Join(app.dir, path))
	return err == nil
}

// ---------------------------------------------------------------------------
// Git snapshot for safety tests
// ---------------------------------------------------------------------------

type gitSnapshot struct {
	status string
	log    []string
	files  map[string]string // working tree contents, excludes .git/ and .gitshelf/
}

func (app *TestApp) gitSnapshot() gitSnapshot {
	app.t.Helper()
	snap := gitSnapshot{
		status: app.gitStatus(),
		log:    app.gitLog(),
		files:  make(map[string]string),
	}
	filepath.WalkDir(app.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(app.dir, path)
		if strings.HasPrefix(rel, ".git") || strings.HasPrefix(rel, ".gitshelf") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		snap.files[rel] = string(data)
		return nil
	})
	return snap
}

func (app *TestApp) assertGitUnchanged(snap gitSnapshot) {
	app.t.Helper()
	if got := app.gitStatus(); got != snap.status {
		app.t.Errorf("git status changed:\n  was: %q\n  now: %q", snap.status, got)
	}
	gotLog := app.gitLog()
	if len(gotLog) != len(snap.log) {
		app.t.Errorf("git log length changed: was %d, now %d", len(snap.log), len(gotLog))
	} else {
		for i := range gotLog {
			if gotLog[i] != snap.log[i] {
				app.t.Errorf("git log[%d] changed: was %q, now %q", i, snap.log[i], gotLog[i])
			}
		}
	}
	currentFiles := make(map[string]string)
	filepath.WalkDir(app.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(app.dir, path)
		if strings.HasPrefix(rel, ".git") || strings.HasPrefix(rel, ".gitshelf") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		data, _ := os.ReadFile(path)
		currentFiles[rel] = string(data)
		return nil
	})
	for f, content := range snap.files {
		if cur, ok := currentFiles[f]; !ok {
			app.t.Errorf("file %q was deleted", f)
		} else if cur != content {
			app.t.Errorf("file %q content changed", f)
		}
	}
	for f := range currentFiles {
		if _, ok := snap.files[f]; !ok {
			app.t.Errorf("new file %q appeared", f)
		}
	}
}

// ---------------------------------------------------------------------------
// gitshelfLabeler — mirrors prompt.go labeler for test prompts
// ---------------------------------------------------------------------------

type gitshelfLabeler struct{}

func (g gitshelfLabeler) PromptLabel(mode types.PromptMode) string { return "" }
func (g gitshelfLabeler) ConfirmMessage(action types.ConfirmAction, target string) string {
	return ""
}

// ---------------------------------------------------------------------------
// Shell helpers
// ---------------------------------------------------------------------------

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run %s %v in %s: %v\n%s", name, args, dir, err, out)
	}
}

func runOut(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

// ===========================================================================
// GIT-MODIFYING TESTS
// ===========================================================================

// 1. TestCommitSelectedFiles
func TestCommitSelectedFiles(t *testing.T) {
	app := newTestApp(t)

	app.WriteTrackedFile("hello.txt", "world")
	app.refresh()

	idx := app.fileIndex("hello.txt")
	if idx < 0 {
		t.Fatalf("hello.txt not in clFiles; clFiles=%v clNames=%v", app.clFiles, app.clNames)
	}
	app.SelectFile(idx)

	app.PressKey("c")
	app.TypePrompt("add hello")

	log := app.gitLog()
	if len(log) < 2 {
		t.Fatalf("expected at least 2 commits, got %d", len(log))
	}
	if log[0] != "add hello" {
		t.Errorf("commit message: got %q, want %q", log[0], "add hello")
	}
	if got := app.gitShow("hello.txt"); got != "world" {
		t.Errorf("committed content: got %q, want %q", got, "world")
	}
	if status := app.gitStatus(); strings.Contains(status, "hello.txt") {
		t.Errorf("hello.txt still in status: %s", status)
	}
}

// 2. TestAmendCommit
func TestAmendCommit(t *testing.T) {
	app := newTestApp(t)

	app.WriteTrackedFile("a.txt", "v1")
	app.refresh()
	app.SelectFile(app.fileIndex("a.txt"))
	app.PressKey("c")
	app.TypePrompt("first")

	countBefore := len(app.gitLog())

	// Modify again (file is now tracked, so modifying it = tracked change)
	app.WriteFile("a.txt", "v2")
	app.refresh()
	app.SelectFile(app.fileIndex("a.txt"))

	app.PressKey("A")
	app.TypePrompt("amended")

	logAfter := app.gitLog()
	if len(logAfter) != countBefore {
		t.Errorf("commit count changed: was %d, now %d", countBefore, len(logAfter))
	}
	if logAfter[0] != "amended" {
		t.Errorf("amended message: got %q, want %q", logAfter[0], "amended")
	}
	if got := app.gitShow("a.txt"); got != "v2" {
		t.Errorf("amended content: got %q, want %q", got, "v2")
	}
}

// 3. TestShelveRestoresFiles
func TestShelveRestoresFiles(t *testing.T) {
	app := newTestApp(t)

	// Modify an existing tracked file
	app.WriteFile("README.md", "modified-readme")
	app.refresh()

	idx := app.fileIndex("README.md")
	if idx < 0 {
		t.Fatalf("README.md not in clFiles; clFiles=%v", app.clFiles)
	}
	app.SelectFile(idx)

	app.PressKey("s")
	app.TypePrompt("my-shelf")
	app.Confirm()

	// File should be restored to original content
	if got := app.fileContent("README.md"); got != "init" {
		t.Errorf("file not restored: got %q, want %q", got, "init")
	}
	if status := app.gitStatus(); strings.Contains(status, "README.md") {
		t.Errorf("README.md still in status after shelve: %s", status)
	}

	shelves, _ := app.stores.Shelf.List()
	found := false
	for _, s := range shelves {
		if s.Meta.Name == "my-shelf" {
			found = true
		}
	}
	if !found {
		t.Error("shelf 'my-shelf' not found on disk")
	}
}

// 4. TestShelveFromCLPanel
func TestShelveFromCLPanel(t *testing.T) {
	app := newTestApp(t)

	app.WriteFile("README.md", "changed-readme")
	app.refresh()

	// Focus CL panel
	app.state.Focus = types.PanelChangelists

	app.PressKey("s")
	app.TypePrompt("cl-shelf")
	app.Confirm()

	status := app.gitStatus()
	if strings.Contains(status, "README.md") {
		t.Errorf("README.md still in status after shelve from CL: %s", status)
	}
}

// 5. TestUnshelve
func TestUnshelve(t *testing.T) {
	app := newTestApp(t)

	app.WriteFile("README.md", "shelved-content")
	app.refresh()

	app.SelectFile(app.fileIndex("README.md"))
	app.PressKey("s")
	app.TypePrompt("test-shelf")
	app.Confirm()

	// Switch to shelves panel
	app.PressKey("2")
	app.refresh()

	if len(app.shelves) == 0 {
		t.Fatal("no shelves found after shelving")
	}

	app.PressKey("u")
	app.TypePrompt("Changes")

	status := app.gitStatus()
	if !strings.Contains(status, "README.md") {
		t.Errorf("README.md not in status after unshelve: %s", status)
	}
}

// 6. TestShelveUnshelveRoundTrip
func TestShelveUnshelveRoundTrip(t *testing.T) {
	app := newTestApp(t)

	original := "specific-content-12345"
	app.WriteFile("README.md", original)
	app.refresh()

	app.SelectFile(app.fileIndex("README.md"))
	app.PressKey("s")
	app.TypePrompt("roundtrip")
	app.Confirm()

	// Verify file restored to HEAD
	if got := app.fileContent("README.md"); got != "init" {
		t.Errorf("file not restored after shelve: got %q", got)
	}

	// Unshelve
	app.PressKey("2")
	app.refresh()
	app.PressKey("u")
	app.TypePrompt("Changes")

	if got := app.fileContent("README.md"); got != original {
		t.Errorf("round trip content: got %q, want %q", got, original)
	}
}

// 7. TestUntrackedFileShelveUnshelve
func TestUntrackedFileShelveUnshelve(t *testing.T) {
	app := newTestApp(t)

	app.WriteTrackedFile("new-file.txt", "brand-new")
	app.refresh()

	idx := app.fileIndex("new-file.txt")
	if idx < 0 {
		t.Fatal("new-file.txt not in clFiles")
	}
	app.SelectFile(idx)

	app.PressKey("s")
	app.TypePrompt("untracked-shelf")
	app.Confirm()

	// File should be gone
	if app.fileExists("new-file.txt") {
		t.Error("new file still exists after shelve")
	}

	// Unshelve
	app.PressKey("2")
	app.refresh()
	app.PressKey("u")
	app.TypePrompt("Changes")

	if !app.fileExists("new-file.txt") {
		t.Fatal("new file not restored after unshelve")
	}
	if got := app.fileContent("new-file.txt"); got != "brand-new" {
		t.Errorf("unshelved content: got %q, want %q", got, "brand-new")
	}
}

// 8. TestMultipleFileSelectiveCommit
func TestMultipleFileSelectiveCommit(t *testing.T) {
	app := newTestApp(t)

	app.WriteTrackedFile("a.txt", "a-content")
	app.WriteTrackedFile("b.txt", "b-content")
	app.WriteTrackedFile("c.txt", "c-content")
	app.refresh()

	idxA := app.fileIndex("a.txt")
	idxB := app.fileIndex("b.txt")
	if idxA < 0 || idxB < 0 {
		t.Fatalf("files not found: a=%d b=%d; clFiles=%v", idxA, idxB, app.clFiles)
	}
	app.SelectFile(idxA)
	app.SelectFile(idxB)

	app.PressKey("c")
	app.TypePrompt("partial commit")

	status := app.gitStatus()
	if strings.Contains(status, "a.txt") {
		t.Error("a.txt still in status")
	}
	if strings.Contains(status, "b.txt") {
		t.Error("b.txt still in status")
	}
	if !strings.Contains(status, "c.txt") {
		t.Error("c.txt should still be in status")
	}
}

// 9. TestPush
func TestPush(t *testing.T) {
	app := newTestAppWithRemote(t)

	app.WriteTrackedFile("push-me.txt", "pushed")
	run(t, app.dir, "git", "commit", "-m", "to push")

	app.refresh()
	app.PressKey("p")

	bareDir := filepath.Join(filepath.Dir(app.dir), "bare.git")
	remoteLog := runOut(t, bareDir, "git", "log", "--oneline", "--format=%s")
	if !strings.Contains(remoteLog, "to push") {
		t.Errorf("remote doesn't have pushed commit: %s", remoteLog)
	}
}

// 10. TestPull
func TestPull(t *testing.T) {
	base := t.TempDir()
	bareDir := filepath.Join(base, "bare.git")
	workDir := filepath.Join(base, "work")
	clone2Dir := filepath.Join(base, "clone2")

	run(t, base, "git", "init", "--bare", bareDir)
	run(t, base, "git", "clone", bareDir, workDir)
	run(t, workDir, "git", "config", "user.email", "test@test.com")
	run(t, workDir, "git", "config", "user.name", "Test")
	run(t, workDir, "git", "config", "core.autocrlf", "false")
	os.WriteFile(filepath.Join(workDir, "README.md"), []byte("init"), 0644)
	run(t, workDir, "git", "add", ".")
	run(t, workDir, "git", "commit", "-m", "initial")
	run(t, workDir, "git", "push", "origin", "HEAD")

	run(t, base, "git", "clone", bareDir, clone2Dir)
	run(t, clone2Dir, "git", "config", "user.email", "test@test.com")
	run(t, clone2Dir, "git", "config", "user.name", "Test")
	run(t, clone2Dir, "git", "config", "core.autocrlf", "false")
	os.WriteFile(filepath.Join(clone2Dir, "pulled.txt"), []byte("from-clone2"), 0644)
	run(t, clone2Dir, "git", "add", ".")
	run(t, clone2Dir, "git", "commit", "-m", "from clone2")
	run(t, clone2Dir, "git", "push", "origin", "HEAD")

	gitshelfDir := filepath.Join(workDir, ".gitshelf")
	os.MkdirAll(gitshelfDir, 0755)

	git.SetRepoRoot(workDir)
	git.ClearLog()
	t.Cleanup(func() { git.SetRepoRoot("") })

	app := &TestApp{
		t:      t,
		dir:    workDir,
		state:  controller.NewState(),
		logger: &testLogger{t: t},
		stores: action.Stores{
			CL:    changelist.NewStore(gitshelfDir),
			Shelf: shelf.NewStore(gitshelfDir),
		},
		prompt: tui.NewPrompt(gitshelfLabeler{}, types.PromptConfirm),
	}
	app.refresh()

	app.PressKey("P")

	if !app.fileExists("pulled.txt") {
		t.Fatal("pulled.txt not found after pull")
	}
	if got := app.fileContent("pulled.txt"); got != "from-clone2" {
		t.Errorf("pulled content: got %q, want %q", got, "from-clone2")
	}
	found := false
	for _, msg := range app.gitLog() {
		if msg == "from clone2" {
			found = true
		}
	}
	if !found {
		t.Errorf("git log doesn't contain 'from clone2': %v", app.gitLog())
	}
}

// 11. TestCommitFromSpecificChangelist
func TestCommitFromSpecificChangelist(t *testing.T) {
	app := newTestApp(t)

	app.WriteTrackedFile("feat.txt", "feature")
	app.WriteTrackedFile("other.txt", "other")
	app.refresh()

	// Create "Feature" CL
	app.state.Focus = types.PanelChangelists
	app.PressKey("n")
	app.TypePrompt("Feature")

	// Move feat.txt to Feature CL
	app.state.Focus = types.PanelFiles
	app.refresh()
	idx := app.fileIndex("feat.txt")
	if idx < 0 {
		t.Fatalf("feat.txt not found; clFiles=%v", app.clFiles)
	}
	app.state.CLFileSel = idx
	app.PressKey("m")
	app.TypePrompt("Feature")

	// Navigate to Feature CL
	app.refresh()
	app.selectCL("Feature")

	idx = app.fileIndex("feat.txt")
	if idx < 0 {
		t.Fatalf("feat.txt not in Feature CL; clFiles=%v", app.clFiles)
	}
	app.SelectFile(idx)
	app.PressKey("c")
	app.TypePrompt("feature commit")

	status := app.gitStatus()
	if strings.Contains(status, "feat.txt") {
		t.Error("feat.txt still in status")
	}
	if !strings.Contains(status, "other.txt") {
		t.Error("other.txt should still be in status")
	}
	if app.gitLog()[0] != "feature commit" {
		t.Errorf("unexpected commit msg: %s", app.gitLog()[0])
	}
}

// ===========================================================================
// GIT-SAFE TESTS
// ===========================================================================

func setupModifiedFiles(t *testing.T, app *TestApp) {
	t.Helper()
	app.WriteTrackedFile("safe1.txt", "content1")
	app.WriteTrackedFile("safe2.txt", "content2")
	app.refresh()
}

// 12. TestCreateChangelist_GitSafe
func TestCreateChangelist_GitSafe(t *testing.T) {
	app := newTestApp(t)
	setupModifiedFiles(t, app)
	snap := app.gitSnapshot()

	app.state.Focus = types.PanelChangelists
	app.PressKey("n")
	app.TypePrompt("Feature CL")

	app.assertGitUnchanged(snap)
}

// 13. TestRenameChangelist_GitSafe
func TestRenameChangelist_GitSafe(t *testing.T) {
	app := newTestApp(t)
	setupModifiedFiles(t, app)

	// Create a CL first
	app.state.Focus = types.PanelChangelists
	app.PressKey("n")
	app.TypePrompt("MyList")
	app.refresh()

	// Navigate to MyList
	for i, name := range app.clNames {
		if name == "MyList" {
			app.state.CLSelected = i
			break
		}
	}

	snap := app.gitSnapshot()

	app.PressKey("r")
	app.TypePrompt("Renamed")

	app.assertGitUnchanged(snap)
}

// 14. TestDeleteChangelist_GitSafe
func TestDeleteChangelist_GitSafe(t *testing.T) {
	app := newTestApp(t)
	setupModifiedFiles(t, app)

	// Create CL + move files
	app.state.Focus = types.PanelChangelists
	app.PressKey("n")
	app.TypePrompt("ToDelete")
	app.refresh()

	snap := app.gitSnapshot()

	// Navigate to ToDelete
	for i, name := range app.clNames {
		if name == "ToDelete" {
			app.state.CLSelected = i
			break
		}
	}
	app.PressKey("d")
	app.Confirm()

	app.assertGitUnchanged(snap)
}

// 15. TestMoveFile_GitSafe
func TestMoveFile_GitSafe(t *testing.T) {
	app := newTestApp(t)
	setupModifiedFiles(t, app)

	// Create target CL
	app.state.Focus = types.PanelChangelists
	app.PressKey("n")
	app.TypePrompt("Target")
	app.refresh()

	snap := app.gitSnapshot()

	// Move file
	app.state.Focus = types.PanelFiles
	app.loadCLFiles()
	if len(app.clFiles) > 0 {
		app.state.CLFileSel = 0
		app.PressKey("m")
		app.TypePrompt("Target")
	}

	app.assertGitUnchanged(snap)
}

// 16. TestMoveSelectedFiles_GitSafe
func TestMoveSelectedFiles_GitSafe(t *testing.T) {
	app := newTestApp(t)
	setupModifiedFiles(t, app)

	// Create target CL
	app.state.Focus = types.PanelChangelists
	app.PressKey("n")
	app.TypePrompt("Target2")
	app.refresh()

	snap := app.gitSnapshot()

	// Select and move
	app.state.Focus = types.PanelFiles
	app.loadCLFiles()
	for i := range app.clFiles {
		app.SelectFile(i)
	}
	app.PressKey("m")
	app.TypePrompt("Target2")

	app.assertGitUnchanged(snap)
}

// 17. TestSelectDeselectFiles_GitSafe
func TestSelectDeselectFiles_GitSafe(t *testing.T) {
	app := newTestApp(t)
	setupModifiedFiles(t, app)
	snap := app.gitSnapshot()

	app.state.Focus = types.PanelFiles
	app.loadCLFiles()

	// space to select
	app.PressKey(" ")
	// a to select all
	app.PressKey("a")
	// x to deselect
	app.PressKey("x")

	app.assertGitUnchanged(snap)
}

// 18. TestSetActiveCL_GitSafe
func TestSetActiveCL_GitSafe(t *testing.T) {
	app := newTestApp(t)
	setupModifiedFiles(t, app)

	app.state.Focus = types.PanelChangelists
	app.PressKey("n")
	app.TypePrompt("ActiveTest")
	app.refresh()

	// Navigate to it
	for i, name := range app.clNames {
		if name == "ActiveTest" {
			app.state.CLSelected = i
			break
		}
	}

	snap := app.gitSnapshot()
	app.PressKey("a")
	app.assertGitUnchanged(snap)
}

// 19. TestNavigationKeys_GitSafe
func TestNavigationKeys_GitSafe(t *testing.T) {
	app := newTestApp(t)
	setupModifiedFiles(t, app)
	snap := app.gitSnapshot()

	keys := []string{"j", "k", "tab", "shift+tab", "1", "2", "3", "4", "5", "enter", "h", "l", "w"}
	for _, key := range keys {
		app.PressKey(key)
	}

	app.assertGitUnchanged(snap)
}

// 20. TestRenameShelf_GitSafe
func TestRenameShelf_GitSafe(t *testing.T) {
	app := newTestApp(t)

	// Create a file and shelve it
	app.WriteTrackedFile("shelf-rename.txt", "data")
	app.refresh()
	app.SelectFile(app.fileIndex("shelf-rename.txt"))
	app.PressKey("s")
	app.TypePrompt("OldName")
	app.Confirm()

	snap := app.gitSnapshot()

	// Switch to shelves
	app.PressKey("2")
	app.refresh()
	app.PressKey("r")
	app.TypePrompt("NewName")

	app.assertGitUnchanged(snap)
}

// 21. TestDropShelf_GitSafe
func TestDropShelf_GitSafe(t *testing.T) {
	app := newTestApp(t)

	app.WriteTrackedFile("shelf-drop.txt", "data")
	app.refresh()
	app.SelectFile(app.fileIndex("shelf-drop.txt"))
	app.PressKey("s")
	app.TypePrompt("DropMe")
	app.Confirm()

	snap := app.gitSnapshot()

	app.PressKey("2")
	app.refresh()
	app.PressKey("d")
	app.Confirm()

	app.assertGitUnchanged(snap)
}

// 22. TestAcceptDirty_GitSafe
func TestAcceptDirty_GitSafe(t *testing.T) {
	app := newTestApp(t)

	// Create a user CL and commit a file through it
	app.state.Focus = types.PanelChangelists
	app.PressKey("n")
	app.TypePrompt("DirtyCL")
	app.refresh()

	// Create and commit a file
	app.WriteTrackedFile("dirty.txt", "v1")
	app.refresh()

	// Move to DirtyCL
	app.state.Focus = types.PanelFiles
	idx := app.fileIndex("dirty.txt")
	if idx < 0 {
		t.Fatal("dirty.txt not found")
	}
	app.state.CLFileSel = idx
	app.PressKey("m")
	app.TypePrompt("DirtyCL")
	app.refresh()

	// Navigate to DirtyCL and select dirty.txt
	for i, name := range app.clNames {
		if name == "DirtyCL" {
			app.state.CLSelected = i
			break
		}
	}
	app.loadCLFiles()
	idx = app.fileIndex("dirty.txt")
	if idx < 0 {
		t.Fatal("dirty.txt not found in DirtyCL")
	}
	app.SelectFile(idx)
	app.PressKey("c")
	app.TypePrompt("commit dirty base")

	// Modify the file again (now it will be dirty relative to stored hash)
	app.WriteFile("dirty.txt", "v2")
	app.refresh()

	// Move to DirtyCL again
	app.state.Focus = types.PanelFiles
	idx = app.fileIndex("dirty.txt")
	if idx >= 0 {
		app.state.CLFileSel = idx
		app.PressKey("m")
		app.TypePrompt("DirtyCL")
	}
	app.refresh()

	// Navigate to DirtyCL
	for i, name := range app.clNames {
		if name == "DirtyCL" {
			app.state.CLSelected = i
			break
		}
	}
	app.loadCLFiles()

	snap := app.gitSnapshot()

	// Accept dirty on the CL
	app.state.Focus = types.PanelChangelists
	if app.dirtyCLs["DirtyCL"] {
		app.PressKey("B")
		app.Confirm()
	}

	app.assertGitUnchanged(snap)
}

// ===========================================================================
// PATCH INTEGRITY TESTS — files without trailing newlines
// ===========================================================================

// TestShelveUnshelve_FileWithoutTrailingNewline verifies that shelving and
// unshelving a file whose content has no trailing newline works correctly.
// This was a real bug: git apply requires patches to end with \n.
func TestShelveUnshelve_FileWithoutTrailingNewline(t *testing.T) {
	app := newTestApp(t)

	// Content deliberately has NO trailing newline
	content := "no newline at end"
	app.WriteTrackedFile("noeol.txt", content)
	app.refresh()

	idx := app.fileIndex("noeol.txt")
	if idx < 0 {
		t.Fatalf("noeol.txt not in clFiles; clFiles=%v", app.clFiles)
	}
	app.SelectFile(idx)

	// Shelve
	app.PressKey("s")
	app.TypePrompt("noeol-shelf")
	app.Confirm()

	if app.fileExists("noeol.txt") {
		t.Error("noeol.txt should be gone after shelve")
	}

	// Unshelve
	app.PressKey("2")
	app.refresh()
	app.PressKey("u")
	app.TypePrompt("Changes")

	if !app.fileExists("noeol.txt") {
		t.Fatal("noeol.txt not restored after unshelve")
	}
	if got := app.fileContent("noeol.txt"); got != content {
		t.Errorf("unshelved content: got %q, want %q", got, content)
	}
}

// TestShelveUnshelve_MultipleFilesWithoutTrailingNewline shelves multiple
// files where none have trailing newlines. The concatenated patch must still
// be valid for git apply.
func TestShelveUnshelve_MultipleFilesWithoutTrailingNewline(t *testing.T) {
	app := newTestApp(t)

	files := map[string]string{
		"noeol-a.txt": "content-a no newline",
		"noeol-b.txt": "content-b no newline",
		"noeol-c.txt": "content-c no newline",
	}
	for name, content := range files {
		app.WriteTrackedFile(name, content)
	}
	app.refresh()

	// Select all the noeol files
	for name := range files {
		idx := app.fileIndex(name)
		if idx < 0 {
			t.Fatalf("%s not in clFiles; clFiles=%v", name, app.clFiles)
		}
		app.SelectFile(idx)
	}

	// Shelve
	app.PressKey("s")
	app.TypePrompt("multi-noeol")
	app.Confirm()

	for name := range files {
		if app.fileExists(name) {
			t.Errorf("%s should be gone after shelve", name)
		}
	}

	// Unshelve
	app.PressKey("2")
	app.refresh()
	app.PressKey("u")
	app.TypePrompt("Changes")

	for name, want := range files {
		if !app.fileExists(name) {
			t.Errorf("%s not restored after unshelve", name)
			continue
		}
		if got := app.fileContent(name); got != want {
			t.Errorf("%s: got %q, want %q", name, got, want)
		}
	}
}

// TestShelveUnshelve_MixedNewlineFiles shelves a mix of files — some with
// trailing newlines, some without — to ensure the patch is valid.
func TestShelveUnshelve_MixedNewlineFiles(t *testing.T) {
	app := newTestApp(t)

	app.WriteTrackedFile("with-nl.txt", "has newline\n")
	app.WriteTrackedFile("without-nl.txt", "no newline")
	app.refresh()

	app.selectAllFiles()

	// Shelve
	app.PressKey("s")
	app.TypePrompt("mixed-shelf")
	app.Confirm()

	// Unshelve
	app.PressKey("2")
	app.refresh()
	app.PressKey("u")
	app.TypePrompt("Changes")

	if got := app.fileContent("with-nl.txt"); got != "has newline\n" {
		t.Errorf("with-nl.txt: got %q, want %q", got, "has newline\n")
	}
	if got := app.fileContent("without-nl.txt"); got != "no newline" {
		t.Errorf("without-nl.txt: got %q, want %q", got, "no newline")
	}
}

// TestShelveUnshelve_AutocrlfTrue verifies shelve/unshelve works when
// core.autocrlf=true (Windows default). Git normalizes to LF internally
// but checks out as CRLF. The patch must round-trip correctly.
func TestShelveUnshelve_AutocrlfTrue(t *testing.T) {
	app := newTestApp(t)

	// Enable autocrlf — this is the Windows default
	run(t, app.dir, "git", "config", "core.autocrlf", "true")

	// Re-checkout so existing files get CRLF
	run(t, app.dir, "git", "checkout", ".")

	// Write a file with explicit LF (git will convert to CRLF on checkout)
	app.WriteTrackedFile("crlf.txt", "line one\nline two\n")
	app.refresh()

	idx := app.fileIndex("crlf.txt")
	if idx < 0 {
		t.Fatalf("crlf.txt not in clFiles; clFiles=%v", app.clFiles)
	}
	app.SelectFile(idx)

	// Shelve
	app.PressKey("s")
	app.TypePrompt("crlf-shelf")
	app.Confirm()

	// File should be gone or restored to HEAD (which doesn't have crlf.txt)
	if app.fileExists("crlf.txt") {
		t.Error("crlf.txt should be gone after shelve")
	}

	// Unshelve
	app.PressKey("2")
	app.refresh()
	app.PressKey("u")
	app.TypePrompt("Changes")

	if !app.fileExists("crlf.txt") {
		t.Fatal("crlf.txt not restored after unshelve")
	}

	// Content should be present (may have CRLF on Windows, LF on Unix)
	content := app.fileContent("crlf.txt")
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	if normalized != "line one\nline two\n" {
		t.Errorf("unshelved content (normalized): got %q, want %q", normalized, "line one\nline two\n")
	}
}

// TestShelveUnshelve_AutocrlfTrue_ExistingFile tests shelve/unshelve of a
// modified tracked file (not new) with autocrlf=true.
func TestShelveUnshelve_AutocrlfTrue_ExistingFile(t *testing.T) {
	app := newTestApp(t)

	// Enable autocrlf
	run(t, app.dir, "git", "config", "core.autocrlf", "true")
	run(t, app.dir, "git", "checkout", ".")

	// Modify an existing tracked file
	app.WriteFile("README.md", "modified content\nwith multiple lines\n")
	app.refresh()

	idx := app.fileIndex("README.md")
	if idx < 0 {
		t.Fatalf("README.md not in clFiles; clFiles=%v", app.clFiles)
	}
	app.SelectFile(idx)

	// Shelve
	app.PressKey("s")
	app.TypePrompt("existing-crlf")
	app.Confirm()

	// README.md should be restored to HEAD content
	headContent := strings.ReplaceAll(app.fileContent("README.md"), "\r\n", "\n")
	if headContent != "init" {
		t.Errorf("file not restored to HEAD: got %q", headContent)
	}

	// Unshelve
	app.PressKey("2")
	app.refresh()
	app.PressKey("u")
	app.TypePrompt("Changes")

	content := strings.ReplaceAll(app.fileContent("README.md"), "\r\n", "\n")
	if content != "modified content\nwith multiple lines\n" {
		t.Errorf("unshelved content: got %q", content)
	}
}

// ===========================================================================
// COPY PATCH TESTS — "y" key across all panels
// ===========================================================================

// assertValidPatch checks that a patch is non-empty, ends with \n, and looks like a diff.
func assertValidPatch(t *testing.T, patch, label string) {
	t.Helper()
	if patch == "" {
		t.Fatalf("%s: patch is empty", label)
	}
	if !strings.HasSuffix(patch, "\n") {
		t.Errorf("%s: patch does not end with newline", label)
	}
	if !strings.Contains(patch, "diff --git") && !strings.Contains(patch, "---") {
		t.Errorf("%s: patch does not look like a diff:\n%s", label, patch[:min(200, len(patch))])
	}
}

func TestCopyPatch_Changelist(t *testing.T) {
	app := newTestApp(t)
	app.WriteTrackedFile("a.txt", "aaa\n")
	app.WriteTrackedFile("b.txt", "bbb\n")
	app.refresh()

	app.state.Focus = types.PanelChangelists
	patch := app.CopyPatch()

	assertValidPatch(t, patch, "changelist")
	if !strings.Contains(patch, "a.txt") || !strings.Contains(patch, "b.txt") {
		t.Error("changelist patch should contain all files")
	}
}

func TestCopyPatch_Changelist_SpecificCL(t *testing.T) {
	app := newTestApp(t)
	app.WriteTrackedFile("feat.txt", "feature\n")
	app.WriteTrackedFile("other.txt", "other\n")
	app.refresh()

	// Create "Feature" CL and move feat.txt there
	app.state.Focus = types.PanelChangelists
	app.PressKey("n")
	app.TypePrompt("Feature")
	app.refresh()

	app.state.Focus = types.PanelFiles
	idx := app.fileIndex("feat.txt")
	app.state.CLFileSel = idx
	app.PressKey("m")
	app.TypePrompt("Feature")
	app.refresh()

	// Select Feature CL and copy patch
	app.selectCL("Feature")
	app.state.Focus = types.PanelChangelists
	patch := app.CopyPatch()

	assertValidPatch(t, patch, "specific CL")
	if !strings.Contains(patch, "feat.txt") {
		t.Error("patch should contain feat.txt")
	}
	if strings.Contains(patch, "other.txt") {
		t.Error("patch should NOT contain other.txt (different CL)")
	}
}

func TestCopyPatch_Shelf(t *testing.T) {
	app := newTestApp(t)
	app.WriteTrackedFile("shelved.txt", "shelved-content\n")
	app.refresh()

	app.SelectFile(app.fileIndex("shelved.txt"))
	app.PressKey("s")
	app.TypePrompt("my-shelf")
	app.Confirm()

	// Switch to shelves and copy patch
	app.PressKey("2")
	app.refresh()
	app.state.Focus = types.PanelShelves
	patch := app.CopyPatch()

	assertValidPatch(t, patch, "shelf")
	if !strings.Contains(patch, "shelved.txt") {
		t.Error("shelf patch should contain shelved.txt")
	}
}

func TestCopyPatch_Files_SingleFile(t *testing.T) {
	app := newTestApp(t)
	app.WriteTrackedFile("x.txt", "xxx\n")
	app.WriteTrackedFile("y.txt", "yyy\n")
	app.refresh()

	// Focus files, cursor on first file, no selection
	app.state.Focus = types.PanelFiles
	app.state.CLFileSel = 0
	patch := app.CopyPatch()

	assertValidPatch(t, patch, "single file")
	// Should contain only one file's diff
	file := app.clFiles[0]
	if !strings.Contains(patch, file) {
		t.Errorf("patch should contain %s", file)
	}
}

func TestCopyPatch_Files_SelectedFiles(t *testing.T) {
	app := newTestApp(t)
	app.WriteTrackedFile("a.txt", "aaa\n")
	app.WriteTrackedFile("b.txt", "bbb\n")
	app.WriteTrackedFile("c.txt", "ccc\n")
	app.refresh()

	// Select a.txt and b.txt
	app.state.Focus = types.PanelFiles
	app.SelectFile(app.fileIndex("a.txt"))
	app.SelectFile(app.fileIndex("b.txt"))

	patch := app.CopyPatch()

	assertValidPatch(t, patch, "selected files")
	if !strings.Contains(patch, "a.txt") {
		t.Error("patch should contain a.txt")
	}
	if !strings.Contains(patch, "b.txt") {
		t.Error("patch should contain b.txt")
	}
	if strings.Contains(patch, "c.txt") {
		t.Error("patch should NOT contain c.txt (not selected)")
	}
}

func TestCopyPatch_Files_ShelfContext(t *testing.T) {
	app := newTestApp(t)
	app.WriteTrackedFile("sf.txt", "shelf-file\n")
	app.refresh()

	app.SelectFile(app.fileIndex("sf.txt"))
	app.PressKey("s")
	app.TypePrompt("file-shelf")
	app.Confirm()

	// Switch to shelves, then focus files panel (shelf context)
	app.PressKey("2")
	app.refresh()
	app.state.Focus = types.PanelFiles
	app.state.Pivot = types.PanelShelves

	patch := app.CopyPatch()

	assertValidPatch(t, patch, "shelf file")
	if !strings.Contains(patch, "sf.txt") {
		t.Error("patch should contain sf.txt")
	}
}

func TestCopyPatch_Diff(t *testing.T) {
	app := newTestApp(t)
	app.WriteTrackedFile("d.txt", "diff-content\n")
	app.refresh()

	// Load the diff (mirrors what the UI does when a file is selected)
	app.state.CLFileSel = app.fileIndex("d.txt")
	app.loadDiff()

	app.state.Focus = types.PanelDiff
	patch := app.CopyPatch()

	assertValidPatch(t, patch, "diff panel")
	if !strings.Contains(patch, "d.txt") {
		t.Error("diff patch should contain d.txt")
	}
}

func TestCopyPatch_FileWithoutTrailingNewline(t *testing.T) {
	app := newTestApp(t)
	app.WriteTrackedFile("noeol.txt", "no newline at end")
	app.refresh()

	app.state.Focus = types.PanelChangelists
	patch := app.CopyPatch()

	assertValidPatch(t, patch, "no-eol changelist")
	if !strings.HasSuffix(patch, "\n") {
		t.Error("patch must end with newline even for files without trailing newline")
	}
}

func TestCopyPatch_Empty_NoCLFiles(t *testing.T) {
	app := newTestApp(t)
	// No modified files
	app.state.Focus = types.PanelChangelists
	patch := app.CopyPatch()
	// clFiles is empty, so the controller should return CopyPatchNone
	// and CopyPatch() returns ""
	if patch != "" {
		t.Errorf("expected empty patch for empty CL, got %d bytes", len(patch))
	}
}

func TestCopyPatch_Empty_NoShelves(t *testing.T) {
	app := newTestApp(t)
	app.state.Focus = types.PanelShelves
	app.state.Pivot = types.PanelShelves
	patch := app.CopyPatch()
	if patch != "" {
		t.Errorf("expected empty patch for no shelves, got %d bytes", len(patch))
	}
}

func TestCopyPatch_GitSafe(t *testing.T) {
	app := newTestApp(t)
	app.WriteTrackedFile("safe.txt", "content\n")
	app.refresh()
	snap := app.gitSnapshot()

	// Copy from all panels
	app.state.Focus = types.PanelChangelists
	app.CopyPatch()

	app.state.Focus = types.PanelFiles
	app.CopyPatch()

	app.state.Focus = types.PanelDiff
	app.loadDiff()
	app.CopyPatch()

	app.assertGitUnchanged(snap)
}

// ===========================================================================
// DUPLICATE SHELF NAME TESTS
// ===========================================================================

// TestDuplicateShelfNames_CreateTwo verifies that two shelves with the same name
// can coexist. Each should have distinct content and PatchDir.
func TestDuplicateShelfNames_CreateTwo(t *testing.T) {
	app := newTestApp(t)

	// Shelve #1: README.md with content A
	app.WriteFile("README.md", "content-A")
	app.refresh()
	app.SelectFile(app.fileIndex("README.md"))
	app.PressKey("s")
	app.TypePrompt("same-name")
	app.Confirm()

	// Shelve #2: README.md with content B
	app.WriteFile("README.md", "content-B")
	app.refresh()
	app.SelectFile(app.fileIndex("README.md"))
	app.PressKey("s")
	app.TypePrompt("same-name")
	app.Confirm()

	shelves, err := app.stores.Shelf.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(shelves) != 2 {
		t.Fatalf("expected 2 shelves, got %d", len(shelves))
	}
	if shelves[0].Meta.Name != "same-name" || shelves[1].Meta.Name != "same-name" {
		t.Errorf("shelf names: %q, %q — both should be 'same-name'", shelves[0].Meta.Name, shelves[1].Meta.Name)
	}
	if shelves[0].PatchDir == shelves[1].PatchDir {
		t.Error("duplicate shelves have the same PatchDir — they should be distinct")
	}
}

// TestDuplicateShelfNames_UnshelveCorrectOne verifies that unshelving picks
// the selected shelf (by index), not a random one with the same name.
func TestDuplicateShelfNames_UnshelveCorrectOne(t *testing.T) {
	app := newTestApp(t)

	// Shelve #1 with content A
	app.WriteFile("README.md", "unshelve-A")
	app.refresh()
	app.SelectFile(app.fileIndex("README.md"))
	app.PressKey("s")
	app.TypePrompt("dup-unshelve")
	app.Confirm()

	time.Sleep(10 * time.Millisecond) // ensure distinct timestamps

	// Shelve #2 with content B
	app.WriteFile("README.md", "unshelve-B")
	app.refresh()
	app.SelectFile(app.fileIndex("README.md"))
	app.PressKey("s")
	app.TypePrompt("dup-unshelve")
	app.Confirm()

	// List is newest-first, so shelves[0]=B, shelves[1]=A
	app.PressKey("2")
	app.refresh()
	if len(app.shelves) != 2 {
		t.Fatalf("expected 2 shelves, got %d", len(app.shelves))
	}

	// Select the first shelf (newest = B)
	app.state.ShelfSel = 0
	app.PressKey("u")
	app.TypePrompt("Changes")

	if got := app.fileContent("README.md"); got != "unshelve-B" {
		t.Errorf("unshelved wrong shelf: got %q, want %q", got, "unshelve-B")
	}
}

// TestDuplicateShelfNames_UnshelveOlderOne verifies we can unshelve the older
// of two shelves with the same name.
func TestDuplicateShelfNames_UnshelveOlderOne(t *testing.T) {
	app := newTestApp(t)

	// Shelve #1 with content A
	app.WriteFile("README.md", "older-A")
	app.refresh()
	app.SelectFile(app.fileIndex("README.md"))
	app.PressKey("s")
	app.TypePrompt("dup-older")
	app.Confirm()

	time.Sleep(10 * time.Millisecond)

	// Shelve #2 with content B
	app.WriteFile("README.md", "newer-B")
	app.refresh()
	app.SelectFile(app.fileIndex("README.md"))
	app.PressKey("s")
	app.TypePrompt("dup-older")
	app.Confirm()

	// List is newest-first: shelves[0]=B, shelves[1]=A
	app.PressKey("2")
	app.refresh()

	// Select the second shelf (older = A)
	app.state.ShelfSel = 1
	app.PressKey("u")
	app.TypePrompt("Changes")

	if got := app.fileContent("README.md"); got != "older-A" {
		t.Errorf("unshelved wrong shelf: got %q, want %q", got, "older-A")
	}
}

// TestDuplicateShelfNames_DropOne verifies that dropping one shelf with a
// duplicate name does not affect the other.
func TestDuplicateShelfNames_DropOne(t *testing.T) {
	app := newTestApp(t)

	// Create two shelves with the same name
	app.WriteFile("README.md", "drop-A")
	app.refresh()
	app.SelectFile(app.fileIndex("README.md"))
	app.PressKey("s")
	app.TypePrompt("dup-drop")
	app.Confirm()

	time.Sleep(10 * time.Millisecond)

	app.WriteFile("README.md", "drop-B")
	app.refresh()
	app.SelectFile(app.fileIndex("README.md"))
	app.PressKey("s")
	app.TypePrompt("dup-drop")
	app.Confirm()

	app.PressKey("2")
	app.refresh()
	if len(app.shelves) != 2 {
		t.Fatalf("expected 2 shelves, got %d", len(app.shelves))
	}

	// Drop the first (newest = B)
	app.state.ShelfSel = 0
	app.PressKey("d")
	app.Confirm()
	app.refresh()

	shelves, _ := app.stores.Shelf.List()
	if len(shelves) != 1 {
		t.Fatalf("expected 1 shelf after drop, got %d", len(shelves))
	}
	if shelves[0].Meta.Name != "dup-drop" {
		t.Errorf("remaining shelf name = %q, want %q", shelves[0].Meta.Name, "dup-drop")
	}

	// Unshelve the remaining (A) to verify its content is intact
	app.state.ShelfSel = 0
	app.PressKey("u")
	app.TypePrompt("Changes")

	if got := app.fileContent("README.md"); got != "drop-A" {
		t.Errorf("remaining shelf has wrong content: got %q, want %q", got, "drop-A")
	}
}

// TestDuplicateShelfNames_RenameOne verifies that renaming one shelf with a
// duplicate name only affects that shelf, not the other.
func TestDuplicateShelfNames_RenameOne(t *testing.T) {
	app := newTestApp(t)

	app.WriteFile("README.md", "rename-A")
	app.refresh()
	app.SelectFile(app.fileIndex("README.md"))
	app.PressKey("s")
	app.TypePrompt("dup-rename")
	app.Confirm()

	time.Sleep(10 * time.Millisecond)

	app.WriteFile("README.md", "rename-B")
	app.refresh()
	app.SelectFile(app.fileIndex("README.md"))
	app.PressKey("s")
	app.TypePrompt("dup-rename")
	app.Confirm()

	app.PressKey("2")
	app.refresh()
	if len(app.shelves) != 2 {
		t.Fatalf("expected 2 shelves, got %d", len(app.shelves))
	}

	// Rename the first (newest = B) to a different name
	app.state.ShelfSel = 0
	app.PressKey("r")
	app.TypePrompt("renamed-B")
	app.refresh()

	shelves, _ := app.stores.Shelf.List()
	if len(shelves) != 2 {
		t.Fatalf("expected 2 shelves, got %d", len(shelves))
	}

	// Count names
	names := map[string]int{}
	for _, s := range shelves {
		names[s.Meta.Name]++
	}
	if names["renamed-B"] != 1 {
		t.Errorf("expected 1 shelf named 'renamed-B', got %d", names["renamed-B"])
	}
	if names["dup-rename"] != 1 {
		t.Errorf("expected 1 shelf still named 'dup-rename', got %d", names["dup-rename"])
	}
}

// TestDuplicateShelfNames_CopyPatchCorrectOne verifies that copying a patch
// from one of two same-named shelves gets the correct content.
func TestDuplicateShelfNames_CopyPatchCorrectOne(t *testing.T) {
	app := newTestApp(t)

	app.WriteFile("README.md", "patch-A")
	app.refresh()
	app.SelectFile(app.fileIndex("README.md"))
	app.PressKey("s")
	app.TypePrompt("dup-patch")
	app.Confirm()

	time.Sleep(10 * time.Millisecond)

	app.WriteFile("README.md", "patch-B")
	app.refresh()
	app.SelectFile(app.fileIndex("README.md"))
	app.PressKey("s")
	app.TypePrompt("dup-patch")
	app.Confirm()

	// Focus shelves panel, select the older shelf (A)
	app.PressKey("2")
	app.refresh()
	if len(app.shelves) != 2 {
		t.Fatalf("expected 2 shelves, got %d", len(app.shelves))
	}

	app.state.ShelfSel = 1 // older = A
	patch := app.CopyPatch()
	if !strings.Contains(patch, "patch-A") {
		t.Errorf("patch from older shelf should contain 'patch-A', got:\n%s", patch)
	}
	if strings.Contains(patch, "patch-B") {
		t.Errorf("patch from older shelf should NOT contain 'patch-B'")
	}
}

// TestDuplicateShelfNames_ViewDiffCorrectOne verifies that viewing the diff
// of a file in one of two same-named shelves shows the correct content.
func TestDuplicateShelfNames_ViewDiffCorrectOne(t *testing.T) {
	app := newTestApp(t)

	app.WriteFile("README.md", "diff-A")
	app.refresh()
	app.SelectFile(app.fileIndex("README.md"))
	app.PressKey("s")
	app.TypePrompt("dup-diff")
	app.Confirm()

	time.Sleep(10 * time.Millisecond)

	app.WriteFile("README.md", "diff-B")
	app.refresh()
	app.SelectFile(app.fileIndex("README.md"))
	app.PressKey("s")
	app.TypePrompt("dup-diff")
	app.Confirm()

	// Focus shelves panel
	app.PressKey("2")
	app.refresh()

	// Select older shelf (A), drill into files, load diff
	app.state.ShelfSel = 1
	app.loadShelfFiles()
	app.state.Focus = types.PanelFiles
	app.state.Pivot = types.PanelShelves
	app.state.ShelfFileSel = 0
	app.loadDiff()

	if !strings.Contains(app.diff, "diff-A") {
		t.Errorf("diff should contain 'diff-A', got:\n%s", app.diff)
	}
	if strings.Contains(app.diff, "diff-B") {
		t.Errorf("diff should NOT contain 'diff-B'")
	}
}

// TestDuplicateShelfNames_GitSafe verifies that renaming and dropping
// duplicate-named shelves never touches git state.
func TestDuplicateShelfNames_GitSafe(t *testing.T) {
	app := newTestApp(t)

	// Create two shelves with the same name
	app.WriteFile("README.md", "safe-A")
	app.refresh()
	app.SelectFile(app.fileIndex("README.md"))
	app.PressKey("s")
	app.TypePrompt("safe-dup")
	app.Confirm()

	time.Sleep(10 * time.Millisecond)

	app.WriteFile("README.md", "safe-B")
	app.refresh()
	app.SelectFile(app.fileIndex("README.md"))
	app.PressKey("s")
	app.TypePrompt("safe-dup")
	app.Confirm()

	// Snapshot AFTER shelves are created (files are restored to HEAD)
	snap := app.gitSnapshot()

	// Rename one
	app.PressKey("2")
	app.refresh()
	app.state.ShelfSel = 0
	app.PressKey("r")
	app.TypePrompt("renamed-safe")
	app.refresh()

	// Drop the other
	app.state.ShelfSel = 1
	app.PressKey("d")
	app.Confirm()

	app.assertGitUnchanged(snap)
}

// ===========================================================================
// Sorting helper for deterministic test assertions
// ===========================================================================

func sorted(ss []string) []string {
	out := make([]string, len(ss))
	copy(out, ss)
	sort.Strings(out)
	return out
}

var _ = sorted // avoid unused warning
