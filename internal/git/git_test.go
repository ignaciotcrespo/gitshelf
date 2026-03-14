package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupRepo creates a temp git repo with an initial commit and returns its path.
// It also sets the package-level repoRoot so all git functions target this repo.
func setupRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

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

	// Create and commit an initial file
	if err := os.WriteFile(filepath.Join(dir, "initial.txt"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "initial commit"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup %v failed: %v\n%s", args, err, out)
		}
	}

	// Point the package at this repo
	repoRoot = dir
	t.Cleanup(func() { repoRoot = "" })
	ClearLog()

	return dir
}

func TestRepoRoot(t *testing.T) {
	dir := setupRepo(t)
	root, err := RepoRoot()
	if err != nil {
		t.Fatalf("RepoRoot() error: %v", err)
	}
	if root != dir {
		t.Errorf("RepoRoot() = %q, want %q", root, dir)
	}
}

func TestRepoRootCaching(t *testing.T) {
	dir := setupRepo(t)

	// First call should cache
	root1, err := RepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if root1 != dir {
		t.Errorf("first call got %q, want %q", root1, dir)
	}

	// Second call should return same cached value
	root2, err := RepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if root1 != root2 {
		t.Errorf("caching broken: %q != %q", root1, root2)
	}
}

func TestTrackedChangedFiles(t *testing.T) {
	dir := setupRepo(t)

	// Modify tracked file
	if err := os.WriteFile(filepath.Join(dir, "initial.txt"), []byte("modified\n"), 0644); err != nil {
		t.Fatal(err)
	}

	files, err := TrackedChangedFiles()
	if err != nil {
		t.Fatalf("TrackedChangedFiles() error: %v", err)
	}
	if len(files) != 1 || files[0] != "initial.txt" {
		t.Errorf("TrackedChangedFiles() = %v, want [initial.txt]", files)
	}
}

func TestTrackedChangedFiles_ExcludesUntracked(t *testing.T) {
	dir := setupRepo(t)

	// Create an untracked file (should NOT appear)
	if err := os.WriteFile(filepath.Join(dir, "newfile.txt"), []byte("new\n"), 0644); err != nil {
		t.Fatal(err)
	}

	files, err := TrackedChangedFiles()
	if err != nil {
		t.Fatalf("TrackedChangedFiles() error: %v", err)
	}
	for _, f := range files {
		if f == "newfile.txt" {
			t.Error("TrackedChangedFiles() should not include untracked files")
		}
	}
}

func TestTrackedChangedFiles_NoChanges(t *testing.T) {
	setupRepo(t)

	files, err := TrackedChangedFiles()
	if err != nil {
		t.Fatalf("TrackedChangedFiles() error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("TrackedChangedFiles() = %v, want empty", files)
	}
}

func TestUntrackedFiles(t *testing.T) {
	dir := setupRepo(t)

	// Create untracked files
	if err := os.WriteFile(filepath.Join(dir, "new1.txt"), []byte("a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new2.txt"), []byte("b\n"), 0644); err != nil {
		t.Fatal(err)
	}

	files, err := UntrackedFiles()
	if err != nil {
		t.Fatalf("UntrackedFiles() error: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("UntrackedFiles() returned %d files, want 2: %v", len(files), files)
	}

	found := map[string]bool{}
	for _, f := range files {
		found[f] = true
	}
	if !found["new1.txt"] || !found["new2.txt"] {
		t.Errorf("UntrackedFiles() = %v, want [new1.txt, new2.txt]", files)
	}
}

func TestUntrackedFiles_Empty(t *testing.T) {
	setupRepo(t)

	files, err := UntrackedFiles()
	if err != nil {
		t.Fatalf("UntrackedFiles() error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("UntrackedFiles() = %v, want empty", files)
	}
}

func TestDiffFile(t *testing.T) {
	dir := setupRepo(t)

	// Modify a tracked file
	if err := os.WriteFile(filepath.Join(dir, "initial.txt"), []byte("changed content\n"), 0644); err != nil {
		t.Fatal(err)
	}

	diff, err := DiffFile("initial.txt")
	if err != nil {
		t.Fatalf("DiffFile() error: %v", err)
	}
	if diff == "" {
		t.Error("DiffFile() returned empty diff for modified file")
	}
	if !strings.Contains(diff, "changed content") {
		t.Errorf("DiffFile() diff doesn't contain new content: %s", diff)
	}
}

func TestDiffFile_UntrackedFile(t *testing.T) {
	dir := setupRepo(t)

	// Create an untracked file
	if err := os.WriteFile(filepath.Join(dir, "brand_new.txt"), []byte("brand new\n"), 0644); err != nil {
		t.Fatal(err)
	}

	diff, err := DiffFile("brand_new.txt")
	if err != nil {
		t.Fatalf("DiffFile() error: %v", err)
	}
	if !strings.Contains(diff, "brand new") {
		t.Errorf("DiffFile() for untracked file doesn't show content: %s", diff)
	}
}

func TestDiffAll(t *testing.T) {
	dir := setupRepo(t)

	// Modify tracked file
	if err := os.WriteFile(filepath.Join(dir, "initial.txt"), []byte("diff all test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	diff, err := DiffAll()
	if err != nil {
		t.Fatalf("DiffAll() error: %v", err)
	}
	if !strings.Contains(diff, "diff all test") {
		t.Errorf("DiffAll() doesn't contain change: %s", diff)
	}
}

func TestDiffAll_NoChanges(t *testing.T) {
	setupRepo(t)

	diff, err := DiffAll()
	if err != nil {
		t.Fatalf("DiffAll() error: %v", err)
	}
	if diff != "" {
		t.Errorf("DiffAll() should be empty with no changes, got: %s", diff)
	}
}

func TestCommitFiles(t *testing.T) {
	dir := setupRepo(t)

	// Modify tracked file
	if err := os.WriteFile(filepath.Join(dir, "initial.txt"), []byte("commit test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	err := CommitFiles([]string{"initial.txt"}, "test commit message")
	if err != nil {
		t.Fatalf("CommitFiles() error: %v", err)
	}

	msg := LastCommitMessage()
	if msg != "test commit message" {
		t.Errorf("LastCommitMessage() = %q, want %q", msg, "test commit message")
	}

	// File should no longer show as changed
	files, err := TrackedChangedFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Errorf("After commit, TrackedChangedFiles() = %v, want empty", files)
	}
}

func TestAmendFiles(t *testing.T) {
	dir := setupRepo(t)

	// Create and commit a file
	if err := os.WriteFile(filepath.Join(dir, "amend.txt"), []byte("v1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := CommitFiles([]string{"amend.txt"}, "first version"); err != nil {
		t.Fatalf("CommitFiles() error: %v", err)
	}

	// Modify and amend
	if err := os.WriteFile(filepath.Join(dir, "amend.txt"), []byte("v2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := AmendFiles([]string{"amend.txt"}, "amended message"); err != nil {
		t.Fatalf("AmendFiles() error: %v", err)
	}

	msg := LastCommitMessage()
	if msg != "amended message" {
		t.Errorf("After amend, LastCommitMessage() = %q, want %q", msg, "amended message")
	}
}

func TestCurrentBranch(t *testing.T) {
	setupRepo(t)

	branch := CurrentBranch()
	// Default branch could be "main" or "master" depending on git config
	if branch == "" {
		t.Error("CurrentBranch() returned empty string")
	}
}

func TestHeadCommit(t *testing.T) {
	setupRepo(t)

	commit := HeadCommit()
	if commit == "" {
		t.Error("HeadCommit() returned empty string")
	}
	// Should be a short hash
	if len(commit) < 4 || len(commit) > 12 {
		t.Errorf("HeadCommit() = %q, expected short hash", commit)
	}
}

func TestGetLogAndClearLog(t *testing.T) {
	setupRepo(t)
	ClearLog()

	// Use action() to ensure it gets logged
	_, _ = action("status", "--porcelain")
	log := GetLog()
	if len(log) == 0 {
		t.Error("GetLog() returned empty after running a command")
	}
	if !strings.Contains(log[0].Command, "git status") {
		t.Errorf("log entry command = %q, expected to contain 'git status'", log[0].Command)
	}

	ClearLog()
	log = GetLog()
	if len(log) != 0 {
		t.Errorf("After ClearLog(), GetLog() = %v, want empty", log)
	}
}

func TestAddUserLog(t *testing.T) {
	setupRepo(t)
	ClearLog()

	AddUserLog("some output", "some error")
	log := GetLog()
	if len(log) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(log))
	}
	if log[0].Command != "" {
		t.Errorf("user log Command = %q, want empty", log[0].Command)
	}
	if log[0].Output != "some output" {
		t.Errorf("user log Output = %q, want %q", log[0].Output, "some output")
	}
	if log[0].Error != "some error" {
		t.Errorf("user log Error = %q, want %q", log[0].Error, "some error")
	}
}

func TestUnquoteGitPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`"path with spaces.txt"`, `path with spaces.txt`},
		{`normal.txt`, `normal.txt`},
		{`"quoted"`, `quoted`},
		{`""`, ``},
		{`a`, `a`},
		{``, ``},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := unquoteGitPath(tt.input)
			if got != tt.want {
				t.Errorf("unquoteGitPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDiffFiles(t *testing.T) {
	dir := setupRepo(t)

	// Modify tracked file
	if err := os.WriteFile(filepath.Join(dir, "initial.txt"), []byte("difffiles test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	diff, err := DiffFiles("initial.txt")
	if err != nil {
		t.Fatalf("DiffFiles() error: %v", err)
	}
	if !strings.Contains(diff, "difffiles test") {
		t.Errorf("DiffFiles() diff doesn't contain new content: %s", diff)
	}
}

func TestRestoreFiles(t *testing.T) {
	dir := setupRepo(t)

	// Modify tracked file
	if err := os.WriteFile(filepath.Join(dir, "initial.txt"), []byte("to be restored\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Verify it shows as changed
	files, err := TrackedChangedFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 changed file before restore, got %d", len(files))
	}

	// Restore
	err = RestoreFiles("initial.txt")
	if err != nil {
		t.Fatalf("RestoreFiles() error: %v", err)
	}

	// Verify no longer changed
	files, err = TrackedChangedFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Errorf("After RestoreFiles(), TrackedChangedFiles() = %v, want empty", files)
	}

	// Verify content was restored
	data, err := os.ReadFile(filepath.Join(dir, "initial.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello\n" {
		t.Errorf("file content = %q, want %q", string(data), "hello\n")
	}
}

func TestRepoRootError(t *testing.T) {
	// Point repoRoot at empty string and run from a non-git directory
	repoRoot = ""
	t.Cleanup(func() { repoRoot = "" })

	dir := t.TempDir() // not a git repo
	oldDir, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(oldDir) })

	_, err := RepoRoot()
	if err == nil {
		t.Error("RepoRoot() in non-git dir should return error")
	}
}

func TestRepoRootCachedPath(t *testing.T) {
	dir := setupRepo(t)

	// Clear cache and call twice to exercise caching code path
	repoRoot = ""
	root1, err := RepoRoot()
	if err != nil {
		t.Fatal(err)
	}

	// Second call should use the cached value (repoRoot != "")
	root2, err := RepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if root1 != root2 {
		t.Errorf("cached root mismatch: %q vs %q", root1, root2)
	}
	_ = dir
}

func TestCurrentBranchError(t *testing.T) {
	// Run in a non-git directory
	repoRoot = ""
	t.Cleanup(func() { repoRoot = "" })

	dir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(oldDir) })

	branch := CurrentBranch()
	if branch != "" {
		t.Errorf("CurrentBranch() in non-git dir = %q, want empty", branch)
	}
}

func TestHeadCommitError(t *testing.T) {
	repoRoot = ""
	t.Cleanup(func() { repoRoot = "" })

	dir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(oldDir) })

	commit := HeadCommit()
	if commit != "" {
		t.Errorf("HeadCommit() in non-git dir = %q, want empty", commit)
	}
}

func TestLastCommitMessageError(t *testing.T) {
	repoRoot = ""
	t.Cleanup(func() { repoRoot = "" })

	dir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(oldDir) })

	msg := LastCommitMessage()
	if msg != "" {
		t.Errorf("LastCommitMessage() in non-git dir = %q, want empty", msg)
	}
}

func TestUntrackedFilesError(t *testing.T) {
	repoRoot = ""
	t.Cleanup(func() { repoRoot = "" })

	dir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(oldDir) })

	files, err := UntrackedFiles()
	if err == nil {
		t.Error("UntrackedFiles() in non-git dir should return error")
	}
	if files != nil {
		t.Errorf("UntrackedFiles() error case should return nil, got %v", files)
	}
}

func TestTrackedChangedFilesError(t *testing.T) {
	repoRoot = ""
	t.Cleanup(func() { repoRoot = "" })

	dir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(oldDir) })

	files, err := TrackedChangedFiles()
	if err == nil {
		t.Error("TrackedChangedFiles() in non-git dir should return error")
	}
	if files != nil {
		t.Errorf("TrackedChangedFiles() error case should return nil, got %v", files)
	}
}

func TestTrackedChangedFilesRename(t *testing.T) {
	dir := setupRepo(t)

	// Rename the file using git mv to produce a "R  old -> new" status line
	cmd := exec.Command("git", "mv", "initial.txt", "renamed.txt")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git mv failed: %v\n%s", err, out)
	}

	files, err := TrackedChangedFiles()
	if err != nil {
		t.Fatalf("TrackedChangedFiles() error: %v", err)
	}

	found := false
	for _, f := range files {
		if f == "renamed.txt" {
			found = true
		}
	}
	if !found {
		t.Errorf("TrackedChangedFiles() = %v, expected to contain 'renamed.txt'", files)
	}
}

func TestCommitFilesAddError(t *testing.T) {
	setupRepo(t)

	// Try to commit a nonexistent file — "git add" should fail
	err := CommitFiles([]string{"nonexistent_file.txt"}, "should fail")
	if err == nil {
		t.Error("CommitFiles() with nonexistent file should return error")
	}
}

func TestAmendFilesAddError(t *testing.T) {
	setupRepo(t)

	err := AmendFiles([]string{"nonexistent_file.txt"}, "should fail")
	if err == nil {
		t.Error("AmendFiles() with nonexistent file should return error")
	}
}

func TestDiffFileBothEmpty(t *testing.T) {
	setupRepo(t)

	// DiffFile on a file that doesn't exist: both diff and no-index return empty
	diff, err := DiffFile("does_not_exist.txt")
	// Both paths return empty, so we get ("", err) back
	if diff != "" {
		t.Errorf("DiffFile() for nonexistent file = %q, want empty", diff)
	}
	_ = err // err may or may not be nil depending on git version
}

func TestDiffFilesError(t *testing.T) {
	repoRoot = ""
	t.Cleanup(func() { repoRoot = "" })

	dir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(oldDir) })

	_, err := DiffFiles("somefile.txt")
	if err == nil {
		t.Error("DiffFiles() in non-git dir should return error")
	}
}


func TestApplyPatch(t *testing.T) {
	dir := setupRepo(t)

	// Modify, get the diff, restore, then apply patch
	if err := os.WriteFile(filepath.Join(dir, "initial.txt"), []byte("patched content\n"), 0644); err != nil {
		t.Fatal(err)
	}

	diff, err := DiffFiles("initial.txt")
	if err != nil {
		t.Fatal(err)
	}

	// Save patch to temp file
	patchFile := filepath.Join(dir, "test.patch")
	if err := os.WriteFile(patchFile, []byte(diff), 0644); err != nil {
		t.Fatal(err)
	}

	// Restore file
	if err := RestoreFiles("initial.txt"); err != nil {
		t.Fatal(err)
	}

	// Apply patch
	if err := ApplyPatch(patchFile); err != nil {
		t.Fatalf("ApplyPatch() error: %v", err)
	}

	// Verify content was re-applied
	data, err := os.ReadFile(filepath.Join(dir, "initial.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "patched content\n" {
		t.Errorf("after patch, content = %q, want %q", string(data), "patched content\n")
	}
}

func TestWorktreeList(t *testing.T) {
	dir := setupRepo(t)
	// Resolve symlinks (macOS /var -> /private/var)
	dir, _ = filepath.EvalSymlinks(dir)
	repoRoot = dir

	// Single worktree (main repo)
	wts, err := WorktreeList("")
	if err != nil {
		t.Fatalf("WorktreeList() error: %v", err)
	}
	if len(wts) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(wts))
	}
	if wts[0].Path != dir {
		t.Errorf("worktree path = %q, want %q", wts[0].Path, dir)
	}
	if !wts[0].IsCurrent {
		t.Error("expected main worktree to be marked as current")
	}
	if wts[0].Branch == "" {
		t.Error("expected worktree to have a branch")
	}
	if wts[0].Commit == "" {
		t.Error("expected worktree to have a commit")
	}

	// Add a second worktree
	wtDir := filepath.Join(t.TempDir(), "second-wt")
	cmd := exec.Command("git", "worktree", "add", "-b", "feature-x", wtDir)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add failed: %v\n%s", err, out)
	}
	wtDir, _ = filepath.EvalSymlinks(wtDir)

	wts, err = WorktreeList("")
	if err != nil {
		t.Fatalf("WorktreeList() error: %v", err)
	}
	if len(wts) != 2 {
		t.Fatalf("expected 2 worktrees, got %d", len(wts))
	}

	// Verify both entries
	foundMain := false
	foundSecond := false
	for _, wt := range wts {
		if wt.Path == dir {
			foundMain = true
			if !wt.IsCurrent {
				t.Error("main worktree should be current")
			}
		}
		if wt.Path == wtDir {
			foundSecond = true
			if wt.IsCurrent {
				t.Error("second worktree should not be current")
			}
			if wt.Branch != "feature-x" {
				t.Errorf("second worktree branch = %q, want %q", wt.Branch, "feature-x")
			}
		}
	}
	if !foundMain {
		t.Error("main worktree not found in list")
	}
	if !foundSecond {
		t.Error("second worktree not found in list")
	}
}
