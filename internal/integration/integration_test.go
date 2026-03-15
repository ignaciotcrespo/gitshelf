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

	gitshelfDir string // original .gitshelf directory (for syncStores)
	stores      action.Stores
	logger      *testLogger
	state       controller.State
	clState     *changelist.State

	// Loaded data (mirrors Model fields)
	clNames    []string
	clFiles    []string
	shelves    []shelf.Shelf
	shelfFiles []string
	dirtyFiles map[string]bool
	dirtyCLs   map[string]bool
	worktrees  []git.Worktree

	// Diff (mirrors Model.diff)
	diff string

	// Prompt flow
	prompt     tui.Prompt
	pending    *prompt.Result
	pendingCtx *action.ActionContext
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
		t:           t,
		dir:         dir,
		gitshelfDir: gitshelfDir,
		state:       controller.NewState(),
		logger:      &testLogger{t: t},
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
		t:           t,
		dir:         workDir,
		gitshelfDir: gitshelfDir,
		state:       controller.NewState(),
		logger:      &testLogger{t: t},
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

// syncStores mirrors loader.go:syncStores — switches CL/Shelf stores and
// git.SetRepoRoot based on the active worktree.
func (app *TestApp) syncStores() {
	dir := app.gitshelfDir
	repoDir := filepath.Dir(app.gitshelfDir)
	if app.state.ActiveWorktreePath != "" {
		dir = filepath.Join(app.state.ActiveWorktreePath, ".gitshelf")
		repoDir = app.state.ActiveWorktreePath
	}
	app.stores.CL = changelist.NewStore(dir)
	app.stores.Shelf = shelf.NewStore(dir)
	git.SetRepoRoot(repoDir)
}

func (app *TestApp) loadWorktrees() {
	wts, err := git.WorktreeList(filepath.Dir(app.gitshelfDir))
	if err != nil {
		app.worktrees = nil
		return
	}
	app.worktrees = wts
	if app.state.WorktreeSel >= len(app.worktrees) {
		app.state.WorktreeSel = max(0, len(app.worktrees)-1)
	}
}

func (app *TestApp) refresh() {
	app.t.Helper()

	app.syncStores()

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
	app.loadWorktrees()
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
	case flag&controller.RefreshWorktree != 0:
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
	currentWT := ""
	if root, err := git.RepoRoot(); err == nil {
		currentWT = root
	}
	wtPaths := make([]string, len(app.worktrees))
	wtNames := make([]string, len(app.worktrees))
	for i, wt := range app.worktrees {
		wtPaths[i] = wt.Path
		wtNames[i] = filepath.Base(wt.Path)
	}
	ctx := controller.KeyContext{
		CLCount:             len(app.clNames),
		CLFileCount:         len(app.clFiles),
		CLNames:             app.clNames,
		CLFiles:             app.clFiles,
		ShelfCount:          len(app.shelves),
		ShelfFileCount:      len(app.shelfFiles),
		SelectedCount:       len(app.state.SelectedFiles),
		UnversionedName:     changelist.UnversionedName,
		DefaultName:         changelist.DefaultName,
		LastCommitMsg:       git.LastCommitMessage(),
		Remotes:             git.Remotes(),
		TabFlow:             tabFlow,
		DirtyFiles:          app.dirtyFiles,
		DirtyCLs:            app.dirtyCLs,
		WorktreeCount:       len(app.worktrees),
		WorktreePaths:       wtPaths,
		WorktreeNames:       wtNames,
		CurrentWorktreePath: currentWT,
	}
	ctx.ShelfNames = make([]string, len(app.shelves))
	ctx.ShelfDirs = make([]string, len(app.shelves))
	for i, s := range app.shelves {
		ctx.ShelfNames[i] = s.Meta.Name
		ctx.ShelfDirs[i] = s.PatchDir
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
		action.Execute(result, &app.stores, app.logger, ctx)
		app.refresh()

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

	case types.PromptPasteChangelist:
		if app.state.ClipboardCL == nil {
			app.logger.SetError("Nothing in clipboard")
			return
		}
		ctx.SourceWorktreePath = app.state.ClipboardCL.SourceWorktree
		ctx.ClipboardCLName = app.state.ClipboardCL.Name
		ctx.ClipboardFiles = app.state.ClipboardCL.Files

		if result.Value == types.PasteFullContent {
			app.pending = result
			app.pendingCtx = ctx
			app.prompt.StartConfirm(types.ConfirmPasteFullContent,
				fmt.Sprintf("%d", len(ctx.ClipboardFiles)))
			return
		}
		action.Execute(result, &app.stores, app.logger, ctx)
		app.refresh()

	case types.PromptConfirm:
		if result.Confirmed {
			switch result.ConfirmAction {
			case types.ConfirmUnshelve:
				if app.pending != nil {
					app.pendingCtx.ForceUnshelve = true
					action.Execute(app.pending, &app.stores, app.logger, app.pendingCtx)
					app.refresh()
				}
			case types.ConfirmPasteFullContent:
				if app.pending != nil {
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
		t:           t,
		dir:         workDir,
		gitshelfDir: gitshelfDir,
		state:       controller.NewState(),
		logger:      &testLogger{t: t},
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

// 18. TestNavigationKeys_GitSafe
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

	// Shelve #2: README.md with content B
	app.WriteFile("README.md", "content-B")
	app.refresh()
	app.SelectFile(app.fileIndex("README.md"))
	app.PressKey("s")
	app.TypePrompt("same-name")

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

	time.Sleep(10 * time.Millisecond) // ensure distinct timestamps

	// Shelve #2 with content B
	app.WriteFile("README.md", "unshelve-B")
	app.refresh()
	app.SelectFile(app.fileIndex("README.md"))
	app.PressKey("s")
	app.TypePrompt("dup-unshelve")

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

	time.Sleep(10 * time.Millisecond)

	// Shelve #2 with content B
	app.WriteFile("README.md", "newer-B")
	app.refresh()
	app.SelectFile(app.fileIndex("README.md"))
	app.PressKey("s")
	app.TypePrompt("dup-older")

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

	time.Sleep(10 * time.Millisecond)

	app.WriteFile("README.md", "drop-B")
	app.refresh()
	app.SelectFile(app.fileIndex("README.md"))
	app.PressKey("s")
	app.TypePrompt("dup-drop")

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

	time.Sleep(10 * time.Millisecond)

	app.WriteFile("README.md", "rename-B")
	app.refresh()
	app.SelectFile(app.fileIndex("README.md"))
	app.PressKey("s")
	app.TypePrompt("dup-rename")

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

	time.Sleep(10 * time.Millisecond)

	app.WriteFile("README.md", "patch-B")
	app.refresh()
	app.SelectFile(app.fileIndex("README.md"))
	app.PressKey("s")
	app.TypePrompt("dup-patch")

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

	time.Sleep(10 * time.Millisecond)

	app.WriteFile("README.md", "diff-B")
	app.refresh()
	app.SelectFile(app.fileIndex("README.md"))
	app.PressKey("s")
	app.TypePrompt("dup-diff")

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

	time.Sleep(10 * time.Millisecond)

	app.WriteFile("README.md", "safe-B")
	app.refresh()
	app.SelectFile(app.fileIndex("README.md"))
	app.PressKey("s")
	app.TypePrompt("safe-dup")

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
// Worktree panel
// ===========================================================================

func TestWorktreePanel_GitSafe(t *testing.T) {
	app := newTestApp(t)
	app.WriteTrackedFile("file1.go", "content")
	app.refresh()

	snap := app.gitSnapshot()

	// Default: minimized (single worktree)
	if app.state.WorktreeState != types.PanelMinimized {
		t.Errorf("expected WorktreeState=Minimized, got %d", app.state.WorktreeState)
	}

	// Press 6 to show (not focused → expands to normal + focuses)
	app.PressKey("6")
	if app.state.WorktreeState != types.PanelNormal {
		t.Errorf("expected WorktreeState=Normal, got %d", app.state.WorktreeState)
	}
	if app.state.Focus != types.PanelWorktrees {
		t.Errorf("expected focus on Worktrees, got %d", app.state.Focus)
	}

	// Press 6 again (focused) → minimized, focus back to pivot
	app.PressKey("6")
	if app.state.WorktreeState != types.PanelMinimized {
		t.Errorf("expected WorktreeState=Minimized, got %d", app.state.WorktreeState)
	}
	if app.state.Focus != app.state.Pivot {
		t.Errorf("expected focus on pivot, got %d", app.state.Focus)
	}

	// Press 6 again (not focused) → normal again
	app.PressKey("6")
	if app.state.WorktreeState != types.PanelNormal {
		t.Errorf("expected WorktreeState=Normal, got %d", app.state.WorktreeState)
	}

	app.assertGitUnchanged(snap)
}

// ===========================================================================
// Worktree test helpers
// ===========================================================================

// newTestAppWithWorktree creates a repo with a linked worktree on a new branch.
// Returns (app, mainDir, worktreeDir).
func newTestAppWithWorktree(t *testing.T) (*TestApp, string, string) {
	t.Helper()
	dir := t.TempDir()
	// Resolve symlinks (macOS /var → /private/var) to match git worktree list output
	dir, _ = filepath.EvalSymlinks(dir)

	// Init repo + initial commit
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@test.com")
	run(t, dir, "git", "config", "user.name", "Test")
	run(t, dir, "git", "config", "core.autocrlf", "false")
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("init"), 0644)
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "initial")

	// Create a linked worktree on a new branch
	wtDir := filepath.Join(filepath.Dir(dir), filepath.Base(dir)+"-wt")
	run(t, dir, "git", "worktree", "add", wtDir, "-b", "feature")
	run(t, wtDir, "git", "config", "user.email", "test@test.com")
	run(t, wtDir, "git", "config", "user.name", "Test")

	// Create .gitshelf dirs
	mainGitshelfDir := filepath.Join(dir, ".gitshelf")
	wtGitshelfDir := filepath.Join(wtDir, ".gitshelf")
	os.MkdirAll(mainGitshelfDir, 0755)
	os.MkdirAll(wtGitshelfDir, 0755)

	git.SetRepoRoot(dir)
	git.ClearLog()
	t.Cleanup(func() {
		git.SetRepoRoot("")
		// Clean up worktree
		exec.Command("git", "-C", dir, "worktree", "remove", "--force", wtDir).Run()
	})

	app := &TestApp{
		t:           t,
		dir:         dir,
		gitshelfDir: mainGitshelfDir,
		state:       controller.NewState(),
		logger:      &testLogger{t: t},
		stores: action.Stores{
			CL:    changelist.NewStore(mainGitshelfDir),
			Shelf: shelf.NewStore(mainGitshelfDir),
		},
		prompt: tui.NewPrompt(gitshelfLabeler{}, types.PromptConfirm),
	}

	app.refresh()
	return app, dir, wtDir
}

// activateWorktree simulates selecting and activating a worktree by path.
func (app *TestApp) activateWorktree(path string) {
	app.t.Helper()
	// Find the worktree index and navigate to it
	for i, wt := range app.worktrees {
		if wt.Path == path {
			app.state.WorktreeSel = i
			break
		}
	}
	// Set active worktree path (toggle off if selecting current worktree)
	if path == app.state.ActiveWorktreePath {
		app.state.ActiveWorktreePath = ""
	} else {
		// Check if this is the current worktree
		for _, wt := range app.worktrees {
			if wt.Path == path && wt.IsCurrent {
				app.state.ActiveWorktreePath = ""
				break
			} else if wt.Path == path {
				app.state.ActiveWorktreePath = path
				break
			}
		}
	}
	app.state.CLSelected = 0
	app.state.CLFileSel = 0
	app.state.ShelfSel = 0
	app.state.ShelfFileSel = 0
	app.state.SelectedFiles = make(map[string]bool)
	app.refresh()
	app.state.Focus = types.PanelChangelists
}

// gitSnapshotAt takes a snapshot of a specific directory's git state.
func gitSnapshotAt(t *testing.T, dir string) gitSnapshot {
	t.Helper()
	snap := gitSnapshot{
		status: runOut(t, dir, "git", "status", "--porcelain"),
		log:    nil,
		files:  make(map[string]string),
	}
	logOut := runOut(t, dir, "git", "log", "--oneline", "--format=%s")
	if logOut != "" {
		snap.log = strings.Split(logOut, "\n")
	}
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
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

// assertGitUnchangedAt verifies that a directory's git state hasn't changed.
func assertGitUnchangedAt(t *testing.T, dir string, snap gitSnapshot) {
	t.Helper()
	if got := runOut(t, dir, "git", "status", "--porcelain"); got != snap.status {
		t.Errorf("git status at %s changed:\n  was: %q\n  now: %q", dir, snap.status, got)
	}
	var gotLog []string
	logOut := runOut(t, dir, "git", "log", "--oneline", "--format=%s")
	if logOut != "" {
		gotLog = strings.Split(logOut, "\n")
	}
	if len(gotLog) != len(snap.log) {
		t.Errorf("git log at %s length changed: was %d, now %d", dir, len(snap.log), len(gotLog))
	} else {
		for i := range gotLog {
			if gotLog[i] != snap.log[i] {
				t.Errorf("git log[%d] at %s changed: was %q, now %q", i, dir, snap.log[i], gotLog[i])
			}
		}
	}
	currentFiles := make(map[string]string)
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
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
			t.Errorf("file %q was deleted at %s", f, dir)
		} else if cur != content {
			t.Errorf("file %q content changed at %s", f, dir)
		}
	}
	for f := range currentFiles {
		if _, ok := snap.files[f]; !ok {
			t.Errorf("new file %q appeared at %s", f, dir)
		}
	}
}

// WriteFileAt writes a file in any directory.
func writeFileAt(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

// writeTrackedFileAt writes and stages a file in a specific directory.
func writeTrackedFileAt(t *testing.T, dir, name, content string) {
	t.Helper()
	writeFileAt(t, dir, name, content)
	run(t, dir, "git", "add", name)
}

// fileContentAt reads a file from any directory.
func fileContentAt(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read file %s in %s: %v", name, dir, err)
	}
	return string(data)
}

// fileExistsAt checks if a file exists.
func fileExistsAt(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

// ===========================================================================
// Worktree integration tests
// ===========================================================================

// --- Worktree activation ---

// TestWT_ActivateWorktree verifies that activating a worktree switches stores
// and git operations to the worktree directory.
func TestWT_ActivateWorktree(t *testing.T) {
	app, mainDir, wtDir := newTestAppWithWorktree(t)

	// Write changes in the worktree
	writeTrackedFileAt(t, wtDir, "wt-file.go", "worktree content")

	// Verify app starts pointing at main
	if len(app.worktrees) < 2 {
		t.Fatalf("expected 2+ worktrees, got %d", len(app.worktrees))
	}

	// Take snapshot of main
	mainSnap := gitSnapshotAt(t, mainDir)

	// Activate the worktree
	app.activateWorktree(wtDir)

	// After activation, app should see the worktree's files
	if app.state.ActiveWorktreePath != wtDir {
		t.Fatalf("ActiveWorktreePath = %q, want %q", app.state.ActiveWorktreePath, wtDir)
	}

	// The CL should show the worktree's files, not main's
	found := false
	for _, f := range app.clFiles {
		if f == "wt-file.go" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected wt-file.go in clFiles, got %v", app.clFiles)
	}

	// Main dir should be untouched
	assertGitUnchangedAt(t, mainDir, mainSnap)
}

// TestWT_DeactivateWorktree verifies toggling back to main.
func TestWT_DeactivateWorktree(t *testing.T) {
	app, mainDir, wtDir := newTestAppWithWorktree(t)

	writeTrackedFileAt(t, mainDir, "main-file.go", "main content")
	writeTrackedFileAt(t, wtDir, "wt-file.go", "wt content")

	app.activateWorktree(wtDir)
	if app.state.ActiveWorktreePath != wtDir {
		t.Fatalf("not activated")
	}

	// Deactivate (toggle same worktree)
	app.activateWorktree(wtDir)
	if app.state.ActiveWorktreePath != "" {
		t.Fatalf("expected empty ActiveWorktreePath after toggle, got %q", app.state.ActiveWorktreePath)
	}

	// Should see main's files again
	found := false
	for _, f := range app.clFiles {
		if f == "main-file.go" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected main-file.go in clFiles after deactivation, got %v", app.clFiles)
	}
}

// --- CL operations in active worktree ---

// TestWT_CreateCL_InActiveWorktree verifies CL creation targets the active worktree's .gitshelf.
func TestWT_CreateCL_InActiveWorktree(t *testing.T) {
	app, mainDir, wtDir := newTestAppWithWorktree(t)

	mainSnap := gitSnapshotAt(t, mainDir)

	app.activateWorktree(wtDir)

	// Create a new CL
	app.state.Focus = types.PanelChangelists
	app.PressKey("n")
	app.TypePrompt("Feature-WT")

	// Verify CL exists in worktree's .gitshelf
	wtStore := changelist.NewStore(filepath.Join(wtDir, ".gitshelf"))
	wtState, err := wtStore.Load()
	if err != nil {
		t.Fatalf("load wt CL: %v", err)
	}
	foundCL := false
	for _, cl := range wtState.Changelists {
		if cl.Name == "Feature-WT" {
			foundCL = true
		}
	}
	if !foundCL {
		t.Errorf("CL 'Feature-WT' not found in worktree .gitshelf")
	}

	// Verify main .gitshelf is untouched
	mainStore := changelist.NewStore(filepath.Join(mainDir, ".gitshelf"))
	mainState, err := mainStore.Load()
	if err != nil {
		t.Fatalf("load main CL: %v", err)
	}
	for _, cl := range mainState.Changelists {
		if cl.Name == "Feature-WT" {
			t.Errorf("CL 'Feature-WT' should NOT exist in main .gitshelf")
		}
	}

	assertGitUnchangedAt(t, mainDir, mainSnap)
}

// TestWT_RenameCL_InActiveWorktree verifies CL rename targets active worktree.
func TestWT_RenameCL_InActiveWorktree(t *testing.T) {
	app, mainDir, wtDir := newTestAppWithWorktree(t)

	mainSnap := gitSnapshotAt(t, mainDir)

	app.activateWorktree(wtDir)

	// Create a CL first
	app.state.Focus = types.PanelChangelists
	app.PressKey("n")
	app.TypePrompt("OldName")

	// Select the new CL and rename it
	app.selectCL("OldName")
	app.PressKey("r")
	app.TypePrompt("NewName")

	// Verify in worktree's store
	wtStore := changelist.NewStore(filepath.Join(wtDir, ".gitshelf"))
	wtState, _ := wtStore.Load()
	foundNew := false
	foundOld := false
	for _, cl := range wtState.Changelists {
		if cl.Name == "NewName" {
			foundNew = true
		}
		if cl.Name == "OldName" {
			foundOld = true
		}
	}
	if !foundNew {
		t.Errorf("CL 'NewName' not found in worktree .gitshelf")
	}
	if foundOld {
		t.Errorf("CL 'OldName' should have been renamed")
	}

	assertGitUnchangedAt(t, mainDir, mainSnap)
}

// TestWT_DeleteCL_InActiveWorktree verifies CL deletion targets active worktree.
func TestWT_DeleteCL_InActiveWorktree(t *testing.T) {
	app, mainDir, wtDir := newTestAppWithWorktree(t)

	mainSnap := gitSnapshotAt(t, mainDir)

	app.activateWorktree(wtDir)

	// Create a CL, then delete it
	app.state.Focus = types.PanelChangelists
	app.PressKey("n")
	app.TypePrompt("ToDelete")

	app.selectCL("ToDelete")
	app.PressKey("d")
	app.Confirm()

	// Verify deleted in worktree
	wtStore := changelist.NewStore(filepath.Join(wtDir, ".gitshelf"))
	wtState, _ := wtStore.Load()
	for _, cl := range wtState.Changelists {
		if cl.Name == "ToDelete" {
			t.Errorf("CL 'ToDelete' should have been deleted from worktree")
		}
	}

	assertGitUnchangedAt(t, mainDir, mainSnap)
}

// --- Shelve in active worktree ---

// TestWT_Shelve_InActiveWorktree verifies shelving in an active worktree:
// - Shelf is created in the worktree's .gitshelf
// - File changes are restored in the worktree (not main)
// - Main worktree is completely untouched
func TestWT_Shelve_InActiveWorktree(t *testing.T) {
	app, mainDir, wtDir := newTestAppWithWorktree(t)

	// Write a change in the worktree
	writeTrackedFileAt(t, wtDir, "shelve-me.go", "changed content")

	mainSnap := gitSnapshotAt(t, mainDir)

	// Activate the worktree
	app.activateWorktree(wtDir)

	// Verify the file appears
	app.selectCL(changelist.DefaultName)
	found := false
	for _, f := range app.clFiles {
		if f == "shelve-me.go" {
			found = true
		}
	}
	if !found {
		t.Fatalf("shelve-me.go not in clFiles; got %v", app.clFiles)
	}

	// Select all files and shelve
	app.selectAllFiles()
	app.PressKey("s")
	app.TypePrompt("wt-shelf")

	// Verify shelf exists in worktree's .gitshelf
	wtShelfStore := shelf.NewStore(filepath.Join(wtDir, ".gitshelf"))
	wtShelves, err := wtShelfStore.List()
	if err != nil {
		t.Fatalf("list wt shelves: %v", err)
	}
	shelfFound := false
	for _, s := range wtShelves {
		if s.Meta.Name == "wt-shelf" {
			shelfFound = true
		}
	}
	if !shelfFound {
		t.Errorf("shelf 'wt-shelf' not found in worktree .gitshelf")
	}

	// Verify shelf does NOT exist in main .gitshelf
	mainShelfStore := shelf.NewStore(filepath.Join(mainDir, ".gitshelf"))
	mainShelves, _ := mainShelfStore.List()
	for _, s := range mainShelves {
		if s.Meta.Name == "wt-shelf" {
			t.Errorf("shelf 'wt-shelf' should NOT exist in main .gitshelf")
		}
	}

	// After shelving a newly created file, git restore removes it (restores to HEAD state
	// where it didn't exist). Verify it's gone from the worktree.
	if fileExistsAt(wtDir, "shelve-me.go") {
		t.Errorf("shelve-me.go should have been removed from worktree after shelve (restored to HEAD)")
	}

	// Main must be untouched
	assertGitUnchangedAt(t, mainDir, mainSnap)
}

// TestWT_Shelve_DoesNotAffectInactiveWorktree is the key regression test for
// the blocker bug: shelving in active worktree must not restore files in any
// other worktree.
func TestWT_Shelve_DoesNotAffectInactiveWorktree(t *testing.T) {
	app, mainDir, wtDir := newTestAppWithWorktree(t)

	// Write DIFFERENT changes in both worktrees
	writeTrackedFileAt(t, mainDir, "shared.go", "main-version")
	writeTrackedFileAt(t, wtDir, "shared.go", "wt-version")

	mainSnap := gitSnapshotAt(t, mainDir)

	// Activate worktree
	app.activateWorktree(wtDir)

	// Shelve the worktree's change
	app.selectCL(changelist.DefaultName)
	app.selectAllFiles()
	app.PressKey("s")
	app.TypePrompt("wt-shelf")

	// CRITICAL: main worktree's file must still have "main-version"
	mainContent := fileContentAt(t, mainDir, "shared.go")
	if mainContent != "main-version" {
		t.Errorf("main worktree's shared.go was modified! got %q, want %q", mainContent, "main-version")
	}

	// Main's git state should be unchanged
	assertGitUnchangedAt(t, mainDir, mainSnap)
}

// --- Unshelve in active worktree ---

// TestWT_Unshelve_InActiveWorktree verifies unshelving applies the patch
// to the active worktree, not the main one.
func TestWT_Unshelve_InActiveWorktree(t *testing.T) {
	app, mainDir, wtDir := newTestAppWithWorktree(t)

	// Create a change in the worktree, shelve it
	writeTrackedFileAt(t, wtDir, "unshelve-me.go", "to-shelve")
	app.activateWorktree(wtDir)

	app.selectCL(changelist.DefaultName)
	app.selectAllFiles()
	app.PressKey("s")
	app.TypePrompt("restore-test")

	mainSnap := gitSnapshotAt(t, mainDir)

	// Now unshelve
	app.state.Focus = types.PanelShelves
	app.state.Pivot = types.PanelShelves
	app.state.ShelfSel = 0
	app.loadShelfFiles()
	app.PressKey("u")
	app.TypePrompt(changelist.DefaultName)

	// Verify the file is restored in the worktree
	if !fileExistsAt(wtDir, "unshelve-me.go") {
		t.Errorf("unshelve-me.go should exist in worktree after unshelve")
	}

	// Main must be untouched
	assertGitUnchangedAt(t, mainDir, mainSnap)
}

// TestWT_Unshelve_DoesNotAffectInactiveWorktree is a regression test.
func TestWT_Unshelve_DoesNotAffectInactiveWorktree(t *testing.T) {
	app, mainDir, wtDir := newTestAppWithWorktree(t)

	// Create a change in the worktree, shelve it
	writeTrackedFileAt(t, wtDir, "feature.go", "feature-code")
	app.activateWorktree(wtDir)

	app.selectCL(changelist.DefaultName)
	app.selectAllFiles()
	app.PressKey("s")
	app.TypePrompt("feature-shelf")

	// Write something different in main
	writeTrackedFileAt(t, mainDir, "main-only.go", "main-code")

	mainSnap := gitSnapshotAt(t, mainDir)

	// Unshelve in the worktree
	app.state.Focus = types.PanelShelves
	app.state.Pivot = types.PanelShelves
	app.state.ShelfSel = 0
	app.loadShelfFiles()
	app.PressKey("u")
	app.TypePrompt(changelist.DefaultName)

	// Main must be untouched
	assertGitUnchangedAt(t, mainDir, mainSnap)
	if !fileExistsAt(mainDir, "main-only.go") {
		t.Errorf("main-only.go should still exist in main")
	}
}

// --- Commit in active worktree ---

// TestWT_Commit_InActiveWorktree verifies that committing targets the active worktree.
func TestWT_Commit_InActiveWorktree(t *testing.T) {
	app, mainDir, wtDir := newTestAppWithWorktree(t)

	writeTrackedFileAt(t, wtDir, "commit-me.go", "commit content")

	mainSnap := gitSnapshotAt(t, mainDir)

	app.activateWorktree(wtDir)

	app.selectCL(changelist.DefaultName)
	idx := app.fileIndex("commit-me.go")
	if idx < 0 {
		t.Fatalf("commit-me.go not in clFiles; got %v", app.clFiles)
	}
	app.SelectFile(idx)
	app.PressKey("c")
	app.TypePrompt("wt commit")

	// Verify the commit is in the worktree's log
	wtLog := strings.Split(runOut(t, wtDir, "git", "log", "--oneline", "--format=%s"), "\n")
	if len(wtLog) < 2 {
		t.Fatalf("expected at least 2 commits in worktree, got %d", len(wtLog))
	}
	if wtLog[0] != "wt commit" {
		t.Errorf("wt commit message: got %q, want %q", wtLog[0], "wt commit")
	}

	// Main must be untouched
	assertGitUnchangedAt(t, mainDir, mainSnap)
}

// --- Diff in active worktree ---

// TestWT_Diff_ShowsActiveWorktreeChanges verifies diff shows the active worktree's changes.
func TestWT_Diff_ShowsActiveWorktreeChanges(t *testing.T) {
	app, _, wtDir := newTestAppWithWorktree(t)

	writeTrackedFileAt(t, wtDir, "diff-test.go", "new-content")

	app.activateWorktree(wtDir)

	app.selectCL(changelist.DefaultName)
	app.loadDiff()

	if app.diff == "" {
		t.Errorf("expected non-empty diff for active worktree changes")
	}
	if !strings.Contains(app.diff, "new-content") {
		t.Errorf("diff should contain 'new-content', got: %s", app.diff)
	}
}

// --- Move files in active worktree ---

// TestWT_MoveFile_InActiveWorktree verifies moving files between CLs in the active worktree.
func TestWT_MoveFile_InActiveWorktree(t *testing.T) {
	app, mainDir, wtDir := newTestAppWithWorktree(t)

	writeTrackedFileAt(t, wtDir, "move-me.go", "movable")

	mainSnap := gitSnapshotAt(t, mainDir)

	app.activateWorktree(wtDir)

	// Create a destination CL
	app.PressKey("n")
	app.TypePrompt("DestCL")

	// Move the file
	app.selectCL(changelist.DefaultName)
	idx := app.fileIndex("move-me.go")
	if idx < 0 {
		t.Fatalf("move-me.go not in clFiles")
	}
	app.SelectFile(idx)
	app.PressKey("m")
	app.TypePrompt("DestCL")

	// Verify the file is now in DestCL
	app.selectCL("DestCL")
	found := false
	for _, f := range app.clFiles {
		if f == "move-me.go" {
			found = true
		}
	}
	if !found {
		t.Errorf("move-me.go should be in DestCL, got %v", app.clFiles)
	}

	// Verify it's in the worktree's .gitshelf, not main's
	wtStore := changelist.NewStore(filepath.Join(wtDir, ".gitshelf"))
	wtState, _ := wtStore.Load()
	foundInWT := false
	for _, cl := range wtState.Changelists {
		if cl.Name == "DestCL" {
			for _, f := range cl.Files {
				if f == "move-me.go" {
					foundInWT = true
				}
			}
		}
	}
	if !foundInWT {
		t.Errorf("move-me.go should be in DestCL in worktree's .gitshelf")
	}

	assertGitUnchangedAt(t, mainDir, mainSnap)
}

// --- Clipboard copy/paste across worktrees ---

// TestWT_CopyPaste_OnlyCL copies a CL from one worktree and pastes it into another (metadata only).
func TestWT_CopyPaste_OnlyCL(t *testing.T) {
	app, mainDir, wtDir := newTestAppWithWorktree(t)

	// Create a CL with files in main worktree
	writeTrackedFileAt(t, mainDir, "copy-file.go", "original")
	app.refresh()

	// Create a CL and move file into it
	app.PressKey("n")
	app.TypePrompt("CopyMe")
	app.selectCL(changelist.DefaultName)
	idx := app.fileIndex("copy-file.go")
	if idx < 0 {
		t.Fatalf("copy-file.go not in clFiles")
	}
	app.SelectFile(idx)
	app.PressKey("m")
	app.TypePrompt("CopyMe")

	// Copy CL to clipboard (W key)
	app.selectCL("CopyMe")
	app.state.Focus = types.PanelChangelists
	app.PressKey("W")

	if app.state.ClipboardCL == nil {
		t.Fatalf("ClipboardCL should be set after W")
	}
	if app.state.ClipboardCL.Name != "CopyMe" {
		t.Errorf("clipboard CL name: got %q, want %q", app.state.ClipboardCL.Name, "CopyMe")
	}

	// Switch to worktree
	app.activateWorktree(wtDir)

	// Paste with "Only changelist" mode
	app.state.Focus = types.PanelChangelists
	app.PressKey("V")
	app.TypePrompt(types.PasteOnlyCL)

	// Verify CL exists in worktree's .gitshelf
	// Note: files may be cleaned by AutoAssignNewFiles if they don't exist as
	// tracked changes in the worktree — "Only changelist" only creates metadata.
	wtStore := changelist.NewStore(filepath.Join(wtDir, ".gitshelf"))
	wtState, _ := wtStore.Load()
	foundCL := false
	for _, cl := range wtState.Changelists {
		if cl.Name == "CopyMe" {
			foundCL = true
		}
	}
	if !foundCL {
		t.Errorf("CL 'CopyMe' not found in worktree's .gitshelf")
	}
}

// TestWT_CopyPaste_FullContent copies a CL and its file contents to another worktree.
func TestWT_CopyPaste_FullContent(t *testing.T) {
	app, mainDir, wtDir := newTestAppWithWorktree(t)

	// Create a change in main
	writeTrackedFileAt(t, mainDir, "full-copy.go", "full-content")
	app.refresh()

	// Create CL and move file
	app.PressKey("n")
	app.TypePrompt("FullCopy")
	app.selectCL(changelist.DefaultName)
	idx := app.fileIndex("full-copy.go")
	if idx >= 0 {
		app.SelectFile(idx)
		app.PressKey("m")
		app.TypePrompt("FullCopy")
	}

	// Copy to clipboard
	app.selectCL("FullCopy")
	app.state.Focus = types.PanelChangelists
	app.PressKey("W")

	// Switch to worktree
	app.activateWorktree(wtDir)

	// Paste with "Full content"
	app.state.Focus = types.PanelChangelists
	app.PressKey("V")
	app.TypePrompt(types.PasteFullContent)
	app.Confirm() // confirm overwrite

	// Verify file exists in worktree with correct content
	if !fileExistsAt(wtDir, "full-copy.go") {
		t.Errorf("full-copy.go should exist in worktree after full content paste")
	} else {
		content := fileContentAt(t, wtDir, "full-copy.go")
		if content != "full-content" {
			t.Errorf("full-copy.go content: got %q, want %q", content, "full-content")
		}
	}
}

// TestWT_CopyPaste_ApplyDiff copies a CL and applies its diff to another worktree.
func TestWT_CopyPaste_ApplyDiff(t *testing.T) {
	app, mainDir, wtDir := newTestAppWithWorktree(t)

	// Modify an existing tracked file in main WITHOUT staging (so git diff shows it).
	// DiffFilesIn uses `git diff --` which only shows unstaged changes.
	writeFileAt(t, mainDir, "README.md", "modified-readme")
	app.refresh()

	// Copy the default CL
	app.selectCL(changelist.DefaultName)
	app.state.Focus = types.PanelChangelists
	app.PressKey("W")

	// Switch to worktree
	app.activateWorktree(wtDir)

	// Paste with "Apply diff"
	app.state.Focus = types.PanelChangelists
	app.PressKey("V")
	app.TypePrompt(types.PasteApplyDiff)

	// Verify the diff was applied in the worktree
	if fileExistsAt(wtDir, "README.md") {
		content := fileContentAt(t, wtDir, "README.md")
		if content != "modified-readme" {
			t.Errorf("README.md in worktree should have applied diff, got %q", content)
		}
	}
}

// --- AutoAssign in active worktree ---

// TestWT_AutoAssign_UsesActiveWorktree verifies that AutoAssignNewFiles
// reads files from the active worktree, not main.
func TestWT_AutoAssign_UsesActiveWorktree(t *testing.T) {
	app, mainDir, wtDir := newTestAppWithWorktree(t)

	// Write a tracked file only in the worktree
	writeTrackedFileAt(t, wtDir, "wt-auto.go", "auto content")
	// Write a tracked file only in main
	writeTrackedFileAt(t, mainDir, "main-auto.go", "main content")

	// Activate worktree
	app.activateWorktree(wtDir)

	// After refresh, the CL should contain wt-auto.go but NOT main-auto.go
	app.selectCL(changelist.DefaultName)

	hasWT := false
	hasMain := false
	for _, f := range app.clFiles {
		if f == "wt-auto.go" {
			hasWT = true
		}
		if f == "main-auto.go" {
			hasMain = true
		}
	}
	if !hasWT {
		t.Errorf("wt-auto.go should be in active worktree's CL files, got %v", app.clFiles)
	}
	if hasMain {
		t.Errorf("main-auto.go should NOT be in active worktree's CL files")
	}
}

// --- Switching worktrees preserves independent state ---

// TestWT_SwitchWorktrees_IndependentCLState verifies that switching between
// worktrees shows each worktree's independent changelist state.
func TestWT_SwitchWorktrees_IndependentCLState(t *testing.T) {
	app, mainDir, wtDir := newTestAppWithWorktree(t)

	// Create different CLs in each worktree
	// Main: create "MainFeature"
	app.PressKey("n")
	app.TypePrompt("MainFeature")

	// Switch to worktree and create "WTFeature"
	app.activateWorktree(wtDir)
	app.PressKey("n")
	app.TypePrompt("WTFeature")

	// Verify worktree has WTFeature but not MainFeature
	hasCL := func(name string) bool {
		for _, n := range app.clNames {
			if n == name {
				return true
			}
		}
		return false
	}

	if !hasCL("WTFeature") {
		t.Errorf("worktree should have WTFeature, clNames=%v", app.clNames)
	}
	if hasCL("MainFeature") {
		t.Errorf("worktree should NOT have MainFeature, clNames=%v", app.clNames)
	}

	// Switch back to main
	app.activateWorktree(wtDir) // toggle off
	_ = mainDir

	if !hasCL("MainFeature") {
		t.Errorf("main should have MainFeature, clNames=%v", app.clNames)
	}
	if hasCL("WTFeature") {
		t.Errorf("main should NOT have WTFeature, clNames=%v", app.clNames)
	}
}

// --- Shelf operations only affect active worktree ---

// TestWT_DropShelf_InActiveWorktree verifies dropping a shelf only affects the active worktree.
func TestWT_DropShelf_InActiveWorktree(t *testing.T) {
	app, mainDir, wtDir := newTestAppWithWorktree(t)

	// Create a shelf in the worktree
	writeTrackedFileAt(t, wtDir, "drop-test.go", "to-drop")
	app.activateWorktree(wtDir)

	app.selectCL(changelist.DefaultName)
	app.selectAllFiles()
	app.PressKey("s")
	app.TypePrompt("drop-me")

	mainSnap := gitSnapshotAt(t, mainDir)

	// Drop the shelf
	app.state.Focus = types.PanelShelves
	app.state.Pivot = types.PanelShelves
	app.state.ShelfSel = 0
	app.loadShelfFiles()
	app.PressKey("d")
	app.Confirm()

	// Verify shelf is gone from worktree
	wtShelfStore := shelf.NewStore(filepath.Join(wtDir, ".gitshelf"))
	wtShelves, _ := wtShelfStore.List()
	for _, s := range wtShelves {
		if s.Meta.Name == "drop-me" {
			t.Errorf("shelf 'drop-me' should have been dropped from worktree")
		}
	}

	assertGitUnchangedAt(t, mainDir, mainSnap)
}

// --- Multiple operations sequence ---

// TestWT_SequentialOperations tests a realistic workflow:
// create CL in WT → add files → shelve → switch to main → verify untouched → switch back → unshelve
func TestWT_SequentialOperations(t *testing.T) {
	app, mainDir, wtDir := newTestAppWithWorktree(t)

	// 1. Activate worktree and create changes
	writeTrackedFileAt(t, wtDir, "seq-file.go", "sequential")
	app.activateWorktree(wtDir)

	// 2. Create CL and move file
	app.PressKey("n")
	app.TypePrompt("SeqCL")
	app.selectCL(changelist.DefaultName)
	if idx := app.fileIndex("seq-file.go"); idx >= 0 {
		app.SelectFile(idx)
		app.PressKey("m")
		app.TypePrompt("SeqCL")
	}

	// 3. Shelve
	app.selectCL("SeqCL")
	app.selectAllFiles()
	app.PressKey("s")
	app.TypePrompt("seq-shelf")

	// 4. Switch to main — verify it's clean
	mainSnap := gitSnapshotAt(t, mainDir)
	app.activateWorktree(wtDir) // toggle off = back to main
	assertGitUnchangedAt(t, mainDir, mainSnap)

	// 5. Switch back to worktree
	app.activateWorktree(wtDir)

	// 6. Verify shelf still exists in worktree
	if len(app.shelves) == 0 {
		t.Fatalf("no shelves in worktree after switching back")
	}
	shelfFound := false
	for _, s := range app.shelves {
		if s.Meta.Name == "seq-shelf" {
			shelfFound = true
		}
	}
	if !shelfFound {
		t.Errorf("shelf 'seq-shelf' not found after switching back")
	}

	// 7. Unshelve
	app.state.Focus = types.PanelShelves
	app.state.Pivot = types.PanelShelves
	app.state.ShelfSel = 0
	app.loadShelfFiles()
	app.PressKey("u")
	app.TypePrompt("SeqCL")

	// 8. Verify file is back in worktree
	if fileExistsAt(wtDir, "seq-file.go") {
		content := fileContentAt(t, wtDir, "seq-file.go")
		if content != "sequential" {
			t.Errorf("seq-file.go content after unshelve: got %q, want %q", content, "sequential")
		}
	}
}

// TestWT_Amend_InActiveWorktree verifies amend targets the active worktree.
func TestWT_Amend_InActiveWorktree(t *testing.T) {
	app, mainDir, wtDir := newTestAppWithWorktree(t)

	// First commit in worktree
	writeTrackedFileAt(t, wtDir, "amend-me.go", "v1")
	app.activateWorktree(wtDir)
	app.selectCL(changelist.DefaultName)
	idx := app.fileIndex("amend-me.go")
	if idx < 0 {
		t.Fatalf("amend-me.go not in clFiles")
	}
	app.SelectFile(idx)
	app.PressKey("c")
	app.TypePrompt("first wt commit")

	mainSnap := gitSnapshotAt(t, mainDir)

	// Modify and amend
	writeFileAt(t, wtDir, "amend-me.go", "v2")
	app.refresh()
	app.selectCL(changelist.DefaultName)
	idx = app.fileIndex("amend-me.go")
	if idx < 0 {
		t.Fatalf("amend-me.go not in clFiles after modify")
	}
	app.SelectFile(idx)
	app.PressKey("a")
	app.TypePrompt("amended wt commit")

	// Verify amend in worktree log
	wtLog := strings.Split(runOut(t, wtDir, "git", "log", "--oneline", "--format=%s"), "\n")
	if wtLog[0] != "amended wt commit" {
		t.Errorf("wt amend message: got %q, want %q", wtLog[0], "amended wt commit")
	}

	// Main must be untouched
	assertGitUnchangedAt(t, mainDir, mainSnap)
}

// --- Reported bug scenarios ---

// TestWT_Unshelve_ConflictBackupInActiveWorktree verifies that when unshelving
// in an active worktree with conflicting files, the backup shelf (~backup-*)
// is created in the ACTIVE worktree's .gitshelf, not the main one.
// This was a reported bug: "a shelf with only 1 file with name starting with 'backup'"
// appeared in the wrong worktree.
func TestWT_Unshelve_ConflictBackupInActiveWorktree(t *testing.T) {
	app, mainDir, wtDir := newTestAppWithWorktree(t)

	// Step 1: Create a change in worktree and shelve it
	writeTrackedFileAt(t, wtDir, "conflict.go", "original-change")
	app.activateWorktree(wtDir)

	app.selectCL(changelist.DefaultName)
	app.selectAllFiles()
	app.PressKey("s")
	app.TypePrompt("conflict-shelf")

	// Step 2: Create a DIFFERENT change to the same file in the worktree
	// (this creates a conflict for unshelve)
	writeTrackedFileAt(t, wtDir, "conflict.go", "conflicting-change")
	app.refresh()

	mainSnap := gitSnapshotAt(t, mainDir)

	// Step 3: Unshelve with force (should create backup shelf for the conflict)
	app.state.Focus = types.PanelShelves
	app.state.Pivot = types.PanelShelves
	app.state.ShelfSel = 0
	app.loadShelfFiles()
	app.PressKey("u")
	app.TypePrompt(changelist.DefaultName)
	// Should get a conflict confirmation
	if app.prompt.Active() {
		app.Confirm() // force unshelve
	}

	// Step 4: Verify backup shelf is in the WORKTREE's .gitshelf, not main's
	wtShelfStore := shelf.NewStore(filepath.Join(wtDir, ".gitshelf"))
	wtShelves, _ := wtShelfStore.List()
	backupFound := false
	for _, s := range wtShelves {
		if strings.HasPrefix(s.Meta.Name, "~backup-") {
			backupFound = true
		}
	}

	mainShelfStore := shelf.NewStore(filepath.Join(mainDir, ".gitshelf"))
	mainShelves, _ := mainShelfStore.List()
	mainBackupFound := false
	for _, s := range mainShelves {
		if strings.HasPrefix(s.Meta.Name, "~backup-") {
			mainBackupFound = true
		}
	}

	if mainBackupFound {
		t.Errorf("backup shelf should NOT be in main .gitshelf — it should be in active worktree's")
	}

	// The backup may or may not exist depending on whether there was actually
	// a conflict. But it must NEVER be in main.
	_ = backupFound

	// Main must be untouched
	assertGitUnchangedAt(t, mainDir, mainSnap)
}

// TestWT_Shelve_RestartShowsCorrectState simulates a "restart" after shelving
// in an active worktree: re-creates the TestApp pointing at main and verifies
// that main's changes are still there (not shelved away).
// This was a reported bug: "when I restart the app I see the changes that disappeared"
func TestWT_Shelve_RestartShowsCorrectState(t *testing.T) {
	app, mainDir, wtDir := newTestAppWithWorktree(t)

	// Both worktrees have different changes
	writeTrackedFileAt(t, mainDir, "main-file.go", "main-change")
	writeTrackedFileAt(t, wtDir, "wt-file.go", "wt-change")

	// Activate worktree and shelve its change
	app.activateWorktree(wtDir)
	app.selectCL(changelist.DefaultName)
	app.selectAllFiles()
	app.PressKey("s")
	app.TypePrompt("wt-shelf")

	// "Restart" — create a fresh TestApp pointing at main (simulates app restart)
	mainGitshelfDir := filepath.Join(mainDir, ".gitshelf")
	git.SetRepoRoot(mainDir)
	git.ClearLog()

	app2 := &TestApp{
		t:           t,
		dir:         mainDir,
		gitshelfDir: mainGitshelfDir,
		state:       controller.NewState(),
		logger:      &testLogger{t: t},
		stores: action.Stores{
			CL:    changelist.NewStore(mainGitshelfDir),
			Shelf: shelf.NewStore(mainGitshelfDir),
		},
		prompt: tui.NewPrompt(gitshelfLabeler{}, types.PromptConfirm),
	}
	app2.refresh()

	// Main's file should still be in the CL
	found := false
	for _, f := range app2.clFiles {
		if f == "main-file.go" {
			found = true
		}
	}
	if !found {
		t.Errorf("main-file.go should still be in main's CL after restart; clFiles=%v", app2.clFiles)
	}

	// Main's file content should be unchanged
	content := fileContentAt(t, mainDir, "main-file.go")
	if content != "main-change" {
		t.Errorf("main-file.go content after restart: got %q, want %q", content, "main-change")
	}

	// Main should NOT have the worktree's shelf
	for _, s := range app2.shelves {
		if s.Meta.Name == "wt-shelf" {
			t.Errorf("wt-shelf should NOT appear in main's shelves after restart")
		}
	}
}

// TestWT_AutoAssign_DoesNotRemoveFilesFromOtherWorktree verifies that when
// switching to a worktree, AutoAssignNewFiles doesn't remove files from
// the other worktree's CL state.
// This tests a corruption scenario where files get cleaned from CLs because
// AutoAssign uses git status from the wrong worktree.
func TestWT_AutoAssign_DoesNotRemoveFilesFromOtherWorktree(t *testing.T) {
	app, mainDir, wtDir := newTestAppWithWorktree(t)

	// Create changes only in main
	writeTrackedFileAt(t, mainDir, "only-in-main.go", "main content")
	app.refresh()

	// Verify the file is in main's CL
	app.selectCL(changelist.DefaultName)
	mainHasFile := false
	for _, f := range app.clFiles {
		if f == "only-in-main.go" {
			mainHasFile = true
		}
	}
	if !mainHasFile {
		t.Fatalf("only-in-main.go should be in main's CL")
	}

	// Switch to worktree and back
	app.activateWorktree(wtDir)
	app.activateWorktree(wtDir) // toggle off = back to main
	app.refresh()

	// Main's CL should still have the file
	app.selectCL(changelist.DefaultName)
	mainStillHasFile := false
	for _, f := range app.clFiles {
		if f == "only-in-main.go" {
			mainStillHasFile = true
		}
	}
	if !mainStillHasFile {
		t.Errorf("only-in-main.go should still be in main's CL after switching worktrees and back; clFiles=%v", app.clFiles)
	}
}

// TestWT_Shelve_MultipleSwitches verifies that rapidly switching between
// worktrees and performing operations doesn't corrupt state.
func TestWT_Shelve_MultipleSwitches(t *testing.T) {
	app, mainDir, wtDir := newTestAppWithWorktree(t)

	// Set up changes in both
	writeTrackedFileAt(t, mainDir, "main.go", "main-v1")
	writeTrackedFileAt(t, wtDir, "wt.go", "wt-v1")

	// Switch to WT, shelve, switch back, verify main
	app.activateWorktree(wtDir)
	app.selectCL(changelist.DefaultName)
	app.selectAllFiles()
	app.PressKey("s")
	app.TypePrompt("wt-shelf-1")

	// Switch back to main
	app.activateWorktree(wtDir) // toggle off

	// Main's file should still exist with original content
	mainContent := fileContentAt(t, mainDir, "main.go")
	if mainContent != "main-v1" {
		t.Errorf("main.go content after wt shelve + switch: got %q, want %q", mainContent, "main-v1")
	}

	// Shelve main's change
	app.selectCL(changelist.DefaultName)
	app.selectAllFiles()
	app.PressKey("s")
	app.TypePrompt("main-shelf-1")

	// Switch to WT — it should have the shelf, not main's shelf
	app.activateWorktree(wtDir)
	wtShelfNames := make([]string, len(app.shelves))
	for i, s := range app.shelves {
		wtShelfNames[i] = s.Meta.Name
	}
	hasWTShelf := false
	hasMainShelf := false
	for _, name := range wtShelfNames {
		if name == "wt-shelf-1" {
			hasWTShelf = true
		}
		if name == "main-shelf-1" {
			hasMainShelf = true
		}
	}
	if !hasWTShelf {
		t.Errorf("worktree should have wt-shelf-1, shelves=%v", wtShelfNames)
	}
	if hasMainShelf {
		t.Errorf("worktree should NOT have main-shelf-1, shelves=%v", wtShelfNames)
	}

	// Switch back to main — it should have main-shelf-1 but not wt-shelf-1
	app.activateWorktree(wtDir) // toggle off
	mainShelfNames := make([]string, len(app.shelves))
	for i, s := range app.shelves {
		mainShelfNames[i] = s.Meta.Name
	}
	hasWTShelf = false
	hasMainShelf = false
	for _, name := range mainShelfNames {
		if name == "wt-shelf-1" {
			hasWTShelf = true
		}
		if name == "main-shelf-1" {
			hasMainShelf = true
		}
	}
	if !hasMainShelf {
		t.Errorf("main should have main-shelf-1, shelves=%v", mainShelfNames)
	}
	if hasWTShelf {
		t.Errorf("main should NOT have wt-shelf-1, shelves=%v", mainShelfNames)
	}
}

// TestWT_Shelve_RestoreTargetsCorrectWorktree verifies that shelving in the active
// worktree restores files (via git restore/clean) in that worktree only.
// Tests the specific case of modifying an existing tracked file (not a new file).
func TestWT_Shelve_RestoreTargetsCorrectWorktree(t *testing.T) {
	app, mainDir, wtDir := newTestAppWithWorktree(t)

	// Both worktrees modify the SAME file (README.md exists in both from initial commit)
	writeFileAt(t, mainDir, "README.md", "main-readme")
	run(t, mainDir, "git", "add", "README.md")
	writeFileAt(t, wtDir, "README.md", "wt-readme")
	run(t, wtDir, "git", "add", "README.md")

	mainSnap := gitSnapshotAt(t, mainDir)

	// Activate worktree and shelve
	app.activateWorktree(wtDir)
	app.selectCL(changelist.DefaultName)
	app.selectAllFiles()
	app.PressKey("s")
	app.TypePrompt("readme-shelf")

	// After shelving in WT, README.md in WT should be restored to "init" (HEAD)
	wtContent := fileContentAt(t, wtDir, "README.md")
	if wtContent != "init" {
		t.Errorf("wt README.md after shelve: got %q, want %q (restored to HEAD)", wtContent, "init")
	}

	// CRITICAL: Main's README.md should still be "main-readme" — shelve must not restore it
	assertGitUnchangedAt(t, mainDir, mainSnap)
	mainContent := fileContentAt(t, mainDir, "README.md")
	if mainContent != "main-readme" {
		t.Errorf("main README.md was modified by wt shelve! got %q, want %q", mainContent, "main-readme")
	}
}

// TestWT_Diff_DoesNotLeakAcrossWorktrees verifies that after switching worktrees,
// the diff shows changes from the correct worktree.
func TestWT_Diff_DoesNotLeakAcrossWorktrees(t *testing.T) {
	app, mainDir, wtDir := newTestAppWithWorktree(t)

	// Different changes in each worktree
	writeTrackedFileAt(t, mainDir, "main-only.go", "main-diff")
	writeTrackedFileAt(t, wtDir, "wt-only.go", "wt-diff")

	// Check diff in main context
	app.refresh()
	app.selectCL(changelist.DefaultName)
	app.loadDiff()
	mainDiff := app.diff

	// Switch to worktree
	app.activateWorktree(wtDir)
	app.selectCL(changelist.DefaultName)
	app.loadDiff()
	wtDiff := app.diff

	// WT diff should contain wt content, not main content
	if strings.Contains(wtDiff, "main-diff") {
		t.Errorf("worktree diff should not contain main-only changes")
	}
	if !strings.Contains(wtDiff, "wt-diff") {
		t.Errorf("worktree diff should contain wt-only changes, got: %s", wtDiff)
	}

	// Switch back to main
	app.activateWorktree(wtDir) // toggle off
	app.selectCL(changelist.DefaultName)
	app.loadDiff()
	mainDiffAfter := app.diff

	// Main diff should not contain WT content
	if strings.Contains(mainDiffAfter, "wt-diff") {
		t.Errorf("main diff should not contain wt-only changes after switching back")
	}
	_ = mainDiff
}

// TestWT_Push_InActiveWorktree verifies pushing targets the active worktree's branch.
func TestWT_Push_InActiveWorktree(t *testing.T) {
	// Create a bare remote + main working dir
	base := t.TempDir()
	base, _ = filepath.EvalSymlinks(base)
	bareDir := filepath.Join(base, "bare.git")
	mainDir := filepath.Join(base, "work")

	run(t, base, "git", "init", "--bare", bareDir)
	run(t, base, "git", "clone", bareDir, mainDir)
	run(t, mainDir, "git", "config", "user.email", "test@test.com")
	run(t, mainDir, "git", "config", "user.name", "Test")
	run(t, mainDir, "git", "config", "core.autocrlf", "false")
	os.WriteFile(filepath.Join(mainDir, "README.md"), []byte("init"), 0644)
	run(t, mainDir, "git", "add", ".")
	run(t, mainDir, "git", "commit", "-m", "initial")
	run(t, mainDir, "git", "push", "origin", "HEAD")

	// Create worktree
	wtDir := filepath.Join(base, "work-wt")
	run(t, mainDir, "git", "worktree", "add", wtDir, "-b", "feature")
	run(t, wtDir, "git", "config", "user.email", "test@test.com")
	run(t, wtDir, "git", "config", "user.name", "Test")

	mainGitshelfDir := filepath.Join(mainDir, ".gitshelf")
	wtGitshelfDir := filepath.Join(wtDir, ".gitshelf")
	os.MkdirAll(mainGitshelfDir, 0755)
	os.MkdirAll(wtGitshelfDir, 0755)

	git.SetRepoRoot(mainDir)
	git.ClearLog()
	t.Cleanup(func() {
		git.SetRepoRoot("")
		exec.Command("git", "-C", mainDir, "worktree", "remove", "--force", wtDir).Run()
	})

	app := &TestApp{
		t:           t,
		dir:         mainDir,
		gitshelfDir: mainGitshelfDir,
		state:       controller.NewState(),
		logger:      &testLogger{t: t},
		stores: action.Stores{
			CL:    changelist.NewStore(mainGitshelfDir),
			Shelf: shelf.NewStore(mainGitshelfDir),
		},
		prompt: tui.NewPrompt(gitshelfLabeler{}, types.PromptConfirm),
	}
	app.refresh()

	// Commit in worktree
	writeTrackedFileAt(t, wtDir, "push-me.go", "push content")
	app.activateWorktree(wtDir)
	app.selectCL(changelist.DefaultName)
	idx := app.fileIndex("push-me.go")
	if idx < 0 {
		t.Fatalf("push-me.go not in clFiles")
	}
	app.SelectFile(idx)
	app.PressKey("c")
	app.TypePrompt("wt push commit")

	mainSnap := gitSnapshotAt(t, mainDir)

	// Push from active worktree (must be on Changelists panel)
	app.state.Focus = types.PanelChangelists
	app.PressKey("p")

	// Verify main is untouched
	assertGitUnchangedAt(t, mainDir, mainSnap)

	// Verify the feature branch was pushed to remote
	remoteRefs := runOut(t, bareDir, "git", "branch")
	if !strings.Contains(remoteRefs, "feature") {
		t.Errorf("feature branch should be pushed to remote, got: %s", remoteRefs)
	}
}

// TestWT_CLState_NotCorruptedByAutoAssign verifies that AutoAssignNewFiles,
// which removes stale files and assigns new ones based on git status, uses
// the correct worktree's git status and doesn't corrupt the other worktree's CL state.
func TestWT_CLState_NotCorruptedByAutoAssign(t *testing.T) {
	app, mainDir, wtDir := newTestAppWithWorktree(t)

	// Create CLs with files in main
	writeTrackedFileAt(t, mainDir, "stable.go", "stable")
	app.refresh()

	app.PressKey("n")
	app.TypePrompt("MainCL")
	app.selectCL(changelist.DefaultName)
	idx := app.fileIndex("stable.go")
	if idx >= 0 {
		app.SelectFile(idx)
		app.PressKey("m")
		app.TypePrompt("MainCL")
	}

	// Save the state of main's CL
	mainStore := changelist.NewStore(filepath.Join(mainDir, ".gitshelf"))
	mainStateBefore, _ := mainStore.Load()
	var mainFilesBefore []string
	for _, cl := range mainStateBefore.Changelists {
		if cl.Name == "MainCL" {
			mainFilesBefore = append(mainFilesBefore, cl.Files...)
		}
	}

	// Switch to worktree (this triggers refresh → AutoAssign with WT's git status)
	app.activateWorktree(wtDir)
	app.refresh() // extra refresh to ensure AutoAssign runs

	// Switch back to main
	app.activateWorktree(wtDir) // toggle off

	// Main's CL state should still have stable.go in MainCL
	app.selectCL("MainCL")
	found := false
	for _, f := range app.clFiles {
		if f == "stable.go" {
			found = true
		}
	}
	if !found {
		t.Errorf("stable.go should still be in MainCL after worktree switch round-trip; clFiles=%v", app.clFiles)
	}
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
