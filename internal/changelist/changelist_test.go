package changelist

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ignaciotcrespo/gitshelf/internal/git"
)

func setupCLRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup %v failed: %v\n%s", args, err, out)
		}
	}

	// Create and commit initial file
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

	// Point the git package at this repo via exported function workaround:
	// Since git.repoRoot is unexported, we call git.RepoRoot() from within the dir
	// We need to set cwd temporarily or use the test helper.
	// Actually, we need to set the git package's repoRoot. Since we can't access
	// unexported vars from another package, we'll use os.Chdir.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	// Force git package to re-resolve repo root by clearing its cache.
	// We can't access the unexported var, so we rely on RepoRoot() discovering it.
	// The trick: call a git command that will work from this dir.
	git.ClearLog()
	t.Cleanup(func() {
		os.Chdir(origDir)
	})

	return dir
}

func TestStoreLoadSave(t *testing.T) {
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "gitshelf")
	store := NewStore(storeDir)

	// Load from non-existent file should return defaults
	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(state.Changelists) != 1 {
		t.Fatalf("default changelists len = %d, want 1", len(state.Changelists))
	}
	if state.Changelists[0].Name != DefaultName {
		t.Errorf("default changelist name = %q, want %q", state.Changelists[0].Name, DefaultName)
	}

	// Save and reload
	state.Changelists[0].Files = []string{"file1.txt", "file2.txt"}
	if err := store.Save(state); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load() after save error: %v", err)
	}
	if len(loaded.Changelists[0].Files) != 2 {
		t.Errorf("loaded files len = %d, want 2", len(loaded.Changelists[0].Files))
	}
	if loaded.Changelists[0].Files[0] != "file1.txt" {
		t.Errorf("loaded file[0] = %q, want %q", loaded.Changelists[0].Files[0], "file1.txt")
	}
}

func TestAddChangelist(t *testing.T) {
	state := &State{
		Active:      DefaultName,
		Changelists: []Changelist{{Name: DefaultName}},
	}

	AddChangelist(state, "Feature A")
	if len(state.Changelists) != 2 {
		t.Fatalf("after add, len = %d, want 2", len(state.Changelists))
	}
	if state.Changelists[1].Name != "Feature A" {
		t.Errorf("new changelist name = %q, want %q", state.Changelists[1].Name, "Feature A")
	}

	// Adding duplicate should not create another
	AddChangelist(state, "Feature A")
	if len(state.Changelists) != 2 {
		t.Errorf("duplicate add created extra: len = %d, want 2", len(state.Changelists))
	}
}

func TestRemoveChangelist(t *testing.T) {
	state := &State{
		Active: "Feature A",
		Changelists: []Changelist{
			{Name: DefaultName},
			{Name: "Feature A", Files: []string{"a.txt"}},
		},
	}

	RemoveChangelist(state, "Feature A")
	if len(state.Changelists) != 1 {
		t.Fatalf("after remove, len = %d, want 1", len(state.Changelists))
	}
}

func TestRemoveChangelist_CannotRemoveDefault(t *testing.T) {
	state := &State{
		Changelists: []Changelist{
			{Name: DefaultName},
			{Name: "Other"},
		},
	}

	RemoveChangelist(state, DefaultName)
	if len(state.Changelists) != 2 {
		t.Errorf("should not remove default changelist, len = %d, want 2", len(state.Changelists))
	}
}

func TestRenameChangelist(t *testing.T) {
	state := &State{
		Changelists: []Changelist{
			{Name: DefaultName},
			{Name: "Old Name", Files: []string{"x.txt"}},
		},
	}

	RenameChangelist(state, "Old Name", "New Name")
	if state.Changelists[1].Name != "New Name" {
		t.Errorf("renamed changelist = %q, want %q", state.Changelists[1].Name, "New Name")
	}
	// Files should be preserved
	if len(state.Changelists[1].Files) != 1 || state.Changelists[1].Files[0] != "x.txt" {
		t.Errorf("files not preserved after rename: %v", state.Changelists[1].Files)
	}
}

func TestAssignFile(t *testing.T) {
	state := &State{
		Active: DefaultName,
		Changelists: []Changelist{
			{Name: DefaultName, Files: []string{"a.txt", "b.txt"}},
			{Name: "Feature", Files: []string{}},
		},
	}

	AssignFile(state, "a.txt", "Feature")

	// Should be removed from default
	for _, f := range state.Changelists[0].Files {
		if f == "a.txt" {
			t.Error("a.txt should be removed from default changelist")
		}
	}
	// Should be in Feature
	found := false
	for _, f := range state.Changelists[1].Files {
		if f == "a.txt" {
			found = true
		}
	}
	if !found {
		t.Error("a.txt should be in Feature changelist")
	}
}

func TestAssignFile_MoveBetweenChangelists(t *testing.T) {
	state := &State{
		Active: DefaultName,
		Changelists: []Changelist{
			{Name: DefaultName},
			{Name: "CL1", Files: []string{"file.txt"}},
			{Name: "CL2"},
		},
	}

	AssignFile(state, "file.txt", "CL2")

	// Should NOT be in CL1
	for _, f := range state.Changelists[1].Files {
		if f == "file.txt" {
			t.Error("file.txt should be removed from CL1")
		}
	}
	// Should be in CL2
	found := false
	for _, f := range state.Changelists[2].Files {
		if f == "file.txt" {
			found = true
		}
	}
	if !found {
		t.Error("file.txt should be in CL2")
	}
}

func TestAllNames(t *testing.T) {
	state := &State{
		Changelists: []Changelist{
			{Name: "A"},
			{Name: "B"},
			{Name: "C"},
		},
	}

	names := AllNames(state)
	if len(names) != 3 {
		t.Fatalf("AllNames() len = %d, want 3", len(names))
	}
	expected := []string{"A", "B", "C"}
	for i, n := range names {
		if n != expected[i] {
			t.Errorf("AllNames()[%d] = %q, want %q", i, n, expected[i])
		}
	}
}

func TestAutoAssignNewFiles(t *testing.T) {
	dir := setupCLRepo(t)

	state := &State{
		Active: DefaultName,
		Changelists: []Changelist{
			{Name: DefaultName},
			{Name: UnversionedName},
		},
	}

	// Modify a tracked file and create an untracked file
	if err := os.WriteFile(filepath.Join(dir, "initial.txt"), []byte("modified\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "newfile.txt"), []byte("new\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := AutoAssignNewFiles(state); err != nil {
		t.Fatalf("AutoAssignNewFiles() error: %v", err)
	}

	// Tracked changed file should be in active (default) changelist
	foundTracked := false
	for _, f := range state.Changelists[0].Files {
		if f == "initial.txt" {
			foundTracked = true
		}
	}
	if !foundTracked {
		t.Error("tracked file should be in active changelist")
	}

	// Untracked file should be in Unversioned Files
	foundUntracked := false
	for _, f := range state.Changelists[1].Files {
		if f == "newfile.txt" {
			foundUntracked = true
		}
	}
	if !foundUntracked {
		t.Error("untracked file should be in Unversioned Files changelist")
	}
}

func TestAutoAssignNewFiles_SkipsAlreadyAssigned(t *testing.T) {
	dir := setupCLRepo(t)

	state := &State{
		Active: DefaultName,
		Changelists: []Changelist{
			{Name: DefaultName, Files: []string{"initial.txt"}},
			{Name: UnversionedName},
		},
	}

	// Modify the already-assigned file
	if err := os.WriteFile(filepath.Join(dir, "initial.txt"), []byte("changed\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := AutoAssignNewFiles(state); err != nil {
		t.Fatal(err)
	}

	// Should still only have one entry (not duplicated)
	count := 0
	for _, f := range state.Changelists[0].Files {
		if f == "initial.txt" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("initial.txt appears %d times, want 1", count)
	}
}

func TestFilesForChangelist(t *testing.T) {
	dir := setupCLRepo(t)

	state := &State{
		Active: DefaultName,
		Changelists: []Changelist{
			{Name: DefaultName, Files: []string{"initial.txt", "gone.txt"}},
		},
	}

	// Only modify initial.txt; gone.txt is no longer changed
	if err := os.WriteFile(filepath.Join(dir, "initial.txt"), []byte("changed\n"), 0644); err != nil {
		t.Fatal(err)
	}

	files, err := FilesForChangelist(state, DefaultName)
	if err != nil {
		t.Fatalf("FilesForChangelist() error: %v", err)
	}

	// Should only include initial.txt (actually changed), not gone.txt
	if len(files) != 1 {
		t.Fatalf("FilesForChangelist() returned %d files, want 1: %v", len(files), files)
	}
	if files[0] != "initial.txt" {
		t.Errorf("FilesForChangelist() = %v, want [initial.txt]", files)
	}
}

func TestFilesForChangelist_WithUntrackedFiles(t *testing.T) {
	dir := setupCLRepo(t)

	// Create an untracked file
	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("new\n"), 0644); err != nil {
		t.Fatal(err)
	}

	state := &State{
		Active: DefaultName,
		Changelists: []Changelist{
			{Name: DefaultName},
			{Name: UnversionedName, Files: []string{"untracked.txt"}},
		},
	}

	files, err := FilesForChangelist(state, UnversionedName)
	if err != nil {
		t.Fatalf("FilesForChangelist() error: %v", err)
	}
	if len(files) != 1 || files[0] != "untracked.txt" {
		t.Errorf("FilesForChangelist() = %v, want [untracked.txt]", files)
	}
}

func TestFilesForChangelist_NonExistent(t *testing.T) {
	setupCLRepo(t)

	state := &State{
		Changelists: []Changelist{
			{Name: DefaultName},
		},
	}

	files, err := FilesForChangelist(state, "does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	if files != nil {
		t.Errorf("FilesForChangelist() for non-existent = %v, want nil", files)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "gitshelf")
	store := NewStore(storeDir)

	// Create the directory and write invalid JSON to the store path
	if err := os.MkdirAll(storeDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storeDir, "changelists.json"), []byte("{corrupted!!!"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := store.Load()
	if err == nil {
		t.Fatal("Load() should return error for invalid JSON")
	}
}

func TestLoadNonIsNotExistError(t *testing.T) {
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "gitshelf")
	store := NewStore(storeDir)

	// Create a directory at the path where Load expects a file.
	// Reading a directory returns a non-IsNotExist error.
	filePath := filepath.Join(storeDir, "changelists.json")
	if err := os.MkdirAll(filePath, 0755); err != nil {
		t.Fatal(err)
	}

	_, err := store.Load()
	if err == nil {
		t.Fatal("Load() should return error when path is a directory")
	}
	if os.IsNotExist(err) {
		t.Fatal("error should not be IsNotExist")
	}
}

func TestSaveMarshalError(t *testing.T) {
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "gitshelf")
	store := NewStore(storeDir)

	origMarshal := marshalIndentFn
	t.Cleanup(func() { marshalIndentFn = origMarshal })

	marshalIndentFn = func(v any, prefix, indent string) ([]byte, error) {
		return nil, errors.New("marshal error")
	}

	state := &State{Active: DefaultName, Changelists: []Changelist{{Name: DefaultName}}}
	err := store.Save(state)
	if err == nil || err.Error() != "marshal error" {
		t.Fatalf("Save() error = %v, want 'marshal error'", err)
	}
}

func TestSaveMkdirAllError(t *testing.T) {
	dir := t.TempDir()
	// Create a regular file where the store directory should be.
	// MkdirAll will fail because it can't create a directory over a file.
	blockingFile := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blockingFile, []byte("I block MkdirAll"), 0644); err != nil {
		t.Fatal(err)
	}
	// Store directory is under the blocking file path, so MkdirAll fails.
	storeDir := filepath.Join(blockingFile, "subdir")
	store := NewStore(storeDir)

	state := &State{Active: DefaultName, Changelists: []Changelist{{Name: DefaultName}}}
	err := store.Save(state)
	if err == nil {
		t.Fatal("Save() should return error when MkdirAll fails")
	}
}

func TestAutoAssignNewFiles_TrackedError(t *testing.T) {
	origTracked := trackedChangedFilesFn
	t.Cleanup(func() { trackedChangedFilesFn = origTracked })

	trackedChangedFilesFn = func() ([]string, error) {
		return nil, errors.New("tracked error")
	}

	state := &State{
		Active:      DefaultName,
		Changelists: []Changelist{{Name: DefaultName}},
	}

	err := AutoAssignNewFiles(state)
	if err == nil || err.Error() != "tracked error" {
		t.Fatalf("AutoAssignNewFiles() error = %v, want 'tracked error'", err)
	}
}

func TestAutoAssignNewFiles_UntrackedError(t *testing.T) {
	origTracked := trackedChangedFilesFn
	origUntracked := untrackedFilesFn
	t.Cleanup(func() {
		trackedChangedFilesFn = origTracked
		untrackedFilesFn = origUntracked
	})

	trackedChangedFilesFn = func() ([]string, error) {
		return []string{"a.txt"}, nil
	}
	untrackedFilesFn = func() ([]string, error) {
		return nil, errors.New("untracked error")
	}

	state := &State{
		Active:      DefaultName,
		Changelists: []Changelist{{Name: DefaultName}},
	}

	err := AutoAssignNewFiles(state)
	if err == nil || err.Error() != "untracked error" {
		t.Fatalf("AutoAssignNewFiles() error = %v, want 'untracked error'", err)
	}
}

func TestFilesForChangelist_TrackedError(t *testing.T) {
	origTracked := trackedChangedFilesFn
	t.Cleanup(func() { trackedChangedFilesFn = origTracked })

	trackedChangedFilesFn = func() ([]string, error) {
		return nil, errors.New("tracked error")
	}

	state := &State{
		Active:      DefaultName,
		Changelists: []Changelist{{Name: DefaultName, Files: []string{"a.txt"}}},
	}

	_, err := FilesForChangelist(state, DefaultName)
	if err == nil || err.Error() != "tracked error" {
		t.Fatalf("FilesForChangelist() error = %v, want 'tracked error'", err)
	}
}

func TestFilesForChangelist_UntrackedError(t *testing.T) {
	origTracked := trackedChangedFilesFn
	origUntracked := untrackedFilesFn
	t.Cleanup(func() {
		trackedChangedFilesFn = origTracked
		untrackedFilesFn = origUntracked
	})

	trackedChangedFilesFn = func() ([]string, error) {
		return []string{"a.txt"}, nil
	}
	untrackedFilesFn = func() ([]string, error) {
		return nil, errors.New("untracked error")
	}

	state := &State{
		Active:      DefaultName,
		Changelists: []Changelist{{Name: DefaultName, Files: []string{"a.txt"}}},
	}

	_, err := FilesForChangelist(state, DefaultName)
	if err == nil || err.Error() != "untracked error" {
		t.Fatalf("FilesForChangelist() error = %v, want 'untracked error'", err)
	}
}
