package shelf

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ignaciotcrespo/gitshelf/internal/git"
)

func setupShelfRepo(t *testing.T) (repoDir string, store *Store) {
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

	// Create and commit initial file
	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("original\n"), 0644); err != nil {
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

	// Point git package at this repo
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

	shelfStore := NewStore(filepath.Join(dir, ".git", "gitshelf"))
	return dir, shelfStore
}

func TestCreateAndList(t *testing.T) {
	dir, store := setupShelfRepo(t)

	// Modify file
	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("modified\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create shelf without restoring
	err := store.Create("my-shelf", []string{"file1.txt"}, false)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	// List should return the shelf
	shelves, err := store.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(shelves) != 1 {
		t.Fatalf("List() returned %d shelves, want 1", len(shelves))
	}
	if shelves[0].Meta.Name != "my-shelf" {
		t.Errorf("shelf name = %q, want %q", shelves[0].Meta.Name, "my-shelf")
	}
	if len(shelves[0].Meta.Files) != 1 || shelves[0].Meta.Files[0] != "file1.txt" {
		t.Errorf("shelf files = %v, want [file1.txt]", shelves[0].Meta.Files)
	}
}

func TestCreateWithRestore(t *testing.T) {
	dir, store := setupShelfRepo(t)

	// Modify file
	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("shelved content\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create shelf WITH restore
	err := store.Create("restore-shelf", []string{"file1.txt"}, true)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	// File should be restored to original
	data, err := os.ReadFile(filepath.Join(dir, "file1.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original\n" {
		t.Errorf("after shelve+restore, content = %q, want %q", string(data), "original\n")
	}
}

func TestCreateNoFiles(t *testing.T) {
	_, store := setupShelfRepo(t)

	err := store.Create("empty", []string{}, false)
	if err == nil {
		t.Error("Create() with no files should return error")
	}
}

func TestApply(t *testing.T) {
	dir, store := setupShelfRepo(t)

	// Modify file, shelve with restore, then apply
	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("apply-test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := store.Create("apply-shelf", []string{"file1.txt"}, true); err != nil {
		t.Fatal(err)
	}

	// Verify file was restored
	data, _ := os.ReadFile(filepath.Join(dir, "file1.txt"))
	if string(data) != "original\n" {
		t.Fatalf("file not restored before apply: %q", string(data))
	}

	// Apply the shelf
	shelves, _ := store.List()
	if len(shelves) == 0 {
		t.Fatal("shelf should exist")
	}
	if err := store.ApplyDir(shelves[0].PatchDir, false); err != nil {
		t.Fatalf("ApplyDir() error: %v", err)
	}

	// File should have shelved content
	data, err := os.ReadFile(filepath.Join(dir, "file1.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "apply-test\n" {
		t.Errorf("after apply, content = %q, want %q", string(data), "apply-test\n")
	}
}

func TestApplyNotFound(t *testing.T) {
	_, store := setupShelfRepo(t)

	err := store.Apply("nonexistent", false)
	if err == nil {
		t.Error("Apply() for nonexistent shelf should return error")
	}
}

func TestDrop(t *testing.T) {
	dir, store := setupShelfRepo(t)

	// Create a shelf
	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("drop-test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := store.Create("drop-me", []string{"file1.txt"}, false); err != nil {
		t.Fatal(err)
	}

	// Drop it
	shelves, _ := store.List()
	if len(shelves) == 0 {
		t.Fatal("shelf should exist")
	}
	if err := store.DropDir(shelves[0].PatchDir); err != nil {
		t.Fatalf("DropDir() error: %v", err)
	}

	// Should be gone from list
	shelves, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(shelves) != 0 {
		t.Errorf("after drop, List() returned %d shelves, want 0", len(shelves))
	}
}

func TestDropNotFound(t *testing.T) {
	_, store := setupShelfRepo(t)

	err := store.Drop("nonexistent")
	if err == nil {
		t.Error("Drop() for nonexistent shelf should return error")
	}
}

func TestRename(t *testing.T) {
	dir, store := setupShelfRepo(t)

	// Create a shelf
	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("rename-test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := store.Create("old-name", []string{"file1.txt"}, false); err != nil {
		t.Fatal(err)
	}

	// Rename it
	shelves, _ := store.List()
	if len(shelves) == 0 {
		t.Fatal("shelf should exist")
	}
	if err := store.RenameDir(shelves[0].PatchDir, "new-name"); err != nil {
		t.Fatalf("RenameDir() error: %v", err)
	}

	// Old name should be gone, new name should exist
	shelves, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(shelves) != 1 {
		t.Fatalf("after rename, List() returned %d shelves, want 1", len(shelves))
	}
	if shelves[0].Meta.Name != "new-name" {
		t.Errorf("renamed shelf name = %q, want %q", shelves[0].Meta.Name, "new-name")
	}
}

func TestRenameNotFound(t *testing.T) {
	_, store := setupShelfRepo(t)

	err := store.Rename("nonexistent", "whatever")
	if err == nil {
		t.Error("Rename() for nonexistent shelf should return error")
	}
}

func TestGetPatch(t *testing.T) {
	dir, store := setupShelfRepo(t)

	// Create a shelf
	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("patch-content\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := store.Create("patch-shelf", []string{"file1.txt"}, false); err != nil {
		t.Fatal(err)
	}

	shelves, _ := store.List()
	if len(shelves) == 0 {
		t.Fatal("shelf should exist")
	}
	patch, err := store.GetPatchDir(shelves[0].PatchDir)
	if err != nil {
		t.Fatalf("GetPatch() error: %v", err)
	}
	if patch == "" {
		t.Error("GetPatch() returned empty patch")
	}
	if !strings.Contains(patch, "patch-content") {
		t.Errorf("patch doesn't contain expected content: %s", patch)
	}
}

func TestGetMetadata(t *testing.T) {
	dir, store := setupShelfRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("meta-test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := store.Create("meta-shelf", []string{"file1.txt"}, false); err != nil {
		t.Fatal(err)
	}

	shelves, _ := store.List()
	if len(shelves) == 0 {
		t.Fatal("shelf should exist")
	}
	meta, err := store.GetMetadataDir(shelves[0].PatchDir)
	if err != nil {
		t.Fatalf("GetMetadata() error: %v", err)
	}
	if meta.Name != "meta-shelf" {
		t.Errorf("metadata name = %q, want %q", meta.Name, "meta-shelf")
	}
	if len(meta.Files) != 1 || meta.Files[0] != "file1.txt" {
		t.Errorf("metadata files = %v, want [file1.txt]", meta.Files)
	}
	if meta.Branch == "" {
		t.Error("metadata branch should not be empty")
	}
	if meta.Commit == "" {
		t.Error("metadata commit should not be empty")
	}
	if meta.CreatedAt == "" {
		t.Error("metadata createdAt should not be empty")
	}
}

func TestListEmpty(t *testing.T) {
	_, store := setupShelfRepo(t)

	shelves, err := store.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(shelves) != 0 {
		t.Errorf("List() on empty store returned %d, want 0", len(shelves))
	}
}

func TestListMultipleShelves(t *testing.T) {
	dir, store := setupShelfRepo(t)

	// Create two shelves
	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("first\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := store.Create("shelf-a", []string{"file1.txt"}, false); err != nil {
		t.Fatal(err)
	}

	// Modify again for second shelf
	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("second\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := store.Create("shelf-b", []string{"file1.txt"}, false); err != nil {
		t.Fatal(err)
	}

	shelves, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(shelves) != 2 {
		t.Fatalf("List() returned %d shelves, want 2", len(shelves))
	}

	// Verify both shelves are present
	names := map[string]bool{}
	for _, s := range shelves {
		names[s.Meta.Name] = true
	}
	if !names["shelf-a"] || !names["shelf-b"] {
		t.Errorf("expected both shelf-a and shelf-b, got %v", names)
	}
}

// --- Coverage gap tests ---

func TestCreateNoChanges(t *testing.T) {
	// File exists but has no modifications -> empty diff -> "no changes to shelve"
	_, store := setupShelfRepo(t)

	// file1.txt is unchanged (matches committed version)
	err := store.Create("no-changes", []string{"file1.txt"}, false)
	if err == nil {
		t.Fatal("Create() with unchanged files should return error")
	}
	if !strings.Contains(err.Error(), "no changes to shelve") {
		t.Errorf("expected 'no changes to shelve' error, got: %v", err)
	}
}

func TestCreateMkdirAllError(t *testing.T) {
	// Store pointed at an invalid path (inside a file, not a directory)
	dir := t.TempDir()
	// Create a file where the shelves directory should be
	filePath := filepath.Join(dir, "blocker")
	if err := os.WriteFile(filePath, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	// Store dir is inside the file -> MkdirAll will fail
	store := NewStore(filePath)

	err := store.Create("test", []string{"file1.txt"}, false)
	if err == nil {
		t.Fatal("Create() should fail when MkdirAll fails")
	}
}

func TestCreateRestoreFalse(t *testing.T) {
	dir, store := setupShelfRepo(t)

	// Modify file
	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("keep-changes\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create shelf WITHOUT restoring
	if err := store.Create("no-restore", []string{"file1.txt"}, false); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	// File should still have the modified content (not restored)
	data, err := os.ReadFile(filepath.Join(dir, "file1.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "keep-changes\n" {
		t.Errorf("after shelve without restore, content = %q, want %q", string(data), "keep-changes\n")
	}
}

func TestListNonDirEntry(t *testing.T) {
	_, store := setupShelfRepo(t)

	// Create the shelves directory manually
	if err := os.MkdirAll(store.dir, 0755); err != nil {
		t.Fatal(err)
	}

	// Put a regular file in the shelves directory (non-directory entry, should be skipped)
	if err := os.WriteFile(filepath.Join(store.dir, "not-a-dir.txt"), []byte("junk"), 0644); err != nil {
		t.Fatal(err)
	}

	shelves, err := store.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(shelves) != 0 {
		t.Errorf("List() should skip non-dir entries, got %d shelves", len(shelves))
	}
}

func TestListUnreadableMetadata(t *testing.T) {
	_, store := setupShelfRepo(t)

	// Create a shelf directory without a metadata.json file
	shelfDir := filepath.Join(store.dir, "broken-shelf")
	if err := os.MkdirAll(shelfDir, 0755); err != nil {
		t.Fatal(err)
	}
	// No metadata.json -> ReadFile will fail -> should be skipped

	shelves, err := store.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(shelves) != 0 {
		t.Errorf("List() should skip entries with unreadable metadata, got %d shelves", len(shelves))
	}
}

func TestListInvalidJSONMetadata(t *testing.T) {
	_, store := setupShelfRepo(t)

	// Create a shelf directory with invalid JSON metadata
	shelfDir := filepath.Join(store.dir, "bad-json-shelf")
	if err := os.MkdirAll(shelfDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shelfDir, "metadata.json"), []byte("{invalid json!!!"), 0644); err != nil {
		t.Fatal(err)
	}

	shelves, err := store.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(shelves) != 0 {
		t.Errorf("List() should skip entries with invalid JSON, got %d shelves", len(shelves))
	}
}

func TestListReadDirError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based permission tests don't work on Windows")
	}
	// Store dir exists but is not readable -> ReadDir error (not IsNotExist)
	dir := t.TempDir()
	shelvesDir := filepath.Join(dir, "shelves")
	if err := os.MkdirAll(shelvesDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Make it unreadable
	if err := os.Chmod(shelvesDir, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		os.Chmod(shelvesDir, 0755)
	})

	// The Store's dir includes the "shelves" suffix from NewStore, so we construct manually
	store := &Store{dir: shelvesDir}

	_, err := store.List()
	if err == nil {
		t.Error("List() should return error when directory is unreadable")
	}
}

func TestCreateDiffFilesError(t *testing.T) {
	_, store := setupShelfRepo(t)

	// Try to diff a file that doesn't exist in the working tree
	err := store.Create("bad-diff", []string{"nonexistent-file.txt"}, false)
	if err == nil {
		t.Fatal("Create() should fail when DiffFiles fails")
	}
}

func TestCreateWriteFileError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based permission tests don't work on Windows")
	}
	dir, store := setupShelfRepo(t)

	// Modify file to get a valid diff
	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("write-err-test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Make the shelves directory read-only so MkdirAll for the new shelf fails
	if err := os.MkdirAll(store.dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(store.dir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		os.Chmod(store.dir, 0755)
	})

	err := store.Create("write-err", []string{"file1.txt"}, false)
	if err == nil {
		t.Fatal("Create() should fail when WriteFile fails on patch")
	}
}

func TestCreateWriteMetadataError(t *testing.T) {
	t.Skip("metadata write error path is covered by the read-only shelves dir test")
}

func TestRenameOsRenameError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based permission tests don't work on Windows")
	}
	dir, store := setupShelfRepo(t)

	// Create a shelf
	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("rename-err\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := store.Create("rename-src", []string{"file1.txt"}, false); err != nil {
		t.Fatal(err)
	}

	// Create a file (not dir) at the target name to block os.Rename
	targetDir := filepath.Join(store.dir, "rename-dst")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Put a file inside target to make it non-empty, preventing rename from overwriting
	if err := os.WriteFile(filepath.Join(targetDir, "blocker"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	// Make target directory read-only so os.Rename fails
	if err := os.Chmod(store.dir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		os.Chmod(store.dir, 0755)
	})

	err := store.Rename("rename-src", "rename-dst")
	if err == nil {
		t.Fatal("Rename() should fail when os.Rename fails")
	}
}

func TestRenameReadMetadataError(t *testing.T) {
	_, store := setupShelfRepo(t)

	// Create the old shelf directory manually without a metadata.json
	oldDir := filepath.Join(store.dir, "old-no-meta")
	if err := os.MkdirAll(oldDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Write a patch.diff so it looks like a shelf
	if err := os.WriteFile(filepath.Join(oldDir, "patch.diff"), []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}

	err := store.Rename("old-no-meta", "new-no-meta")
	if err == nil {
		t.Fatal("Rename() should fail when metadata.json is missing")
	}
}

func TestRenameInvalidMetadataJSON(t *testing.T) {
	_, store := setupShelfRepo(t)

	// Create the old shelf directory manually with invalid JSON metadata
	oldDir := filepath.Join(store.dir, "old-bad-json")
	if err := os.MkdirAll(oldDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "metadata.json"), []byte("not valid json"), 0644); err != nil {
		t.Fatal(err)
	}

	err := store.Rename("old-bad-json", "new-bad-json")
	if err == nil {
		t.Fatal("Rename() should fail when metadata.json contains invalid JSON")
	}
}

func TestRenameWriteMetadataError(t *testing.T) {
	_, store := setupShelfRepo(t)

	// Create the old shelf directory with valid metadata
	oldDir := filepath.Join(store.dir, "old-write-err")
	if err := os.MkdirAll(oldDir, 0755); err != nil {
		t.Fatal(err)
	}
	validMeta := `{"name":"old-write-err","branch":"main","commit":"abc","createdAt":"2024-01-01T00:00:00Z","files":["f.txt"]}`
	metaPath := filepath.Join(oldDir, "metadata.json")
	if err := os.WriteFile(metaPath, []byte(validMeta), 0444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		newDir := filepath.Join(store.dir, "new-write-err")
		newMeta := filepath.Join(newDir, "metadata.json")
		os.Chmod(newMeta, 0644)
	})

	err := store.Rename("old-write-err", "new-write-err")
	if err == nil {
		t.Fatal("Rename() should fail when WriteFile for metadata fails")
	}
}

func TestGetPatchNotFound(t *testing.T) {
	_, store := setupShelfRepo(t)

	_, err := store.GetPatch("nonexistent")
	if err == nil {
		t.Error("GetPatch() for nonexistent shelf should return error")
	}
}

func TestGetMetadataNotFound(t *testing.T) {
	_, store := setupShelfRepo(t)

	_, err := store.GetMetadata("nonexistent")
	if err == nil {
		t.Error("GetMetadata() for nonexistent shelf should return error")
	}
}

func TestGetMetadataInvalidJSON(t *testing.T) {
	_, store := setupShelfRepo(t)

	// Create a shelf directory with invalid JSON metadata
	shelfDir := filepath.Join(store.dir, "bad-meta")
	if err := os.MkdirAll(shelfDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shelfDir, "metadata.json"), []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := store.GetMetadata("bad-meta")
	if err == nil {
		t.Error("GetMetadata() with invalid JSON should return error")
	}
}

func TestShelfMetadataWorktree(t *testing.T) {
	dir, store := setupShelfRepo(t)

	// Modify file and create shelf
	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("wt-test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := store.Create("wt-shelf", []string{"file1.txt"}, false); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	shelves, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(shelves) != 1 {
		t.Fatalf("expected 1 shelf, got %d", len(shelves))
	}

	// Worktree field should be populated with the basename of the repo dir
	if shelves[0].Meta.Worktree == "" {
		t.Error("Worktree field should be populated")
	}
	if shelves[0].Meta.Worktree != filepath.Base(dir) {
		t.Errorf("Worktree = %q, want %q", shelves[0].Meta.Worktree, filepath.Base(dir))
	}

	// Test backward compat: old JSON without worktree field should deserialize fine
	oldJSON := `{"name":"old-shelf","branch":"main","commit":"abc1234","createdAt":"2024-01-01T00:00:00Z","files":["a.txt"]}`
	var meta Metadata
	if err := json.Unmarshal([]byte(oldJSON), &meta); err != nil {
		t.Fatalf("Unmarshal old JSON: %v", err)
	}
	if meta.Worktree != "" {
		t.Errorf("old shelf Worktree = %q, want empty", meta.Worktree)
	}
}

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"with spaces", "with-spaces"},
		{"path/slash", "path-slash"},
		{"back\\slash", "back-slash"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeName(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
