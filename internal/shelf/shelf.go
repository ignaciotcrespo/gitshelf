package shelf

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ignaciotcrespo/gitshelf/internal/git"
)

// Metadata holds information about a shelf.
type Metadata struct {
	Name      string   `json:"name"`
	Message   string   `json:"message,omitempty"`
	Branch    string   `json:"branch"`
	Commit    string   `json:"commit"`
	CreatedAt string   `json:"createdAt"`
	Files     []string `json:"files"`
	Untracked []string `json:"untracked,omitempty"`
	Worktree  string   `json:"worktree,omitempty"`
	Snapshot  string   `json:"snapshot,omitempty"`
}

// Shelf represents a saved shelf with its metadata.
type Shelf struct {
	Meta     Metadata
	PatchDir string
}

// Store manages shelf persistence.
type Store struct {
	dir string
}

// NewStore creates a store rooted at the given .git/gitshelf/shelves directory.
func NewStore(gitshelfDir string) *Store {
	return &Store{dir: filepath.Join(gitshelfDir, "shelves")}
}

// Create saves a new shelf from the given files.
// It generates a patch, saves metadata, and optionally restores the files.
// The directory name is timestamp-based so multiple shelves can share the same name.
func (s *Store) Create(name string, files []string, restore bool) error {
	if len(files) == 0 {
		return fmt.Errorf("no files to shelve")
	}

	dirName := fmt.Sprintf("%s_%s", time.Now().Format("20060102-150405.000"), sanitizeName(name))
	shelfDir := filepath.Join(s.dir, dirName)
	if err := os.MkdirAll(shelfDir, 0755); err != nil {
		return err
	}

	// Generate patch
	diff, err := git.DiffFiles(files...)
	if err != nil {
		return fmt.Errorf("failed to get diff: %w", err)
	}
	if diff == "" {
		return fmt.Errorf("no changes to shelve")
	}

	// Ensure patch ends with newline — git apply requires it
	if len(diff) > 0 && diff[len(diff)-1] != '\n' {
		diff += "\n"
	}

	patchPath := filepath.Join(shelfDir, "patch.diff")
	if err := os.WriteFile(patchPath, []byte(diff), 0644); err != nil {
		return err
	}

	// Save metadata
	meta := Metadata{
		Name:      name,
		Branch:    git.CurrentBranch(),
		Commit:    git.HeadCommit(),
		CreatedAt: time.Now().Format(time.RFC3339Nano),
		Files:     files,
		Untracked: untrackedSubset(files),
		Worktree:  git.WorktreeName(),
	}
	metaData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	metaPath := filepath.Join(shelfDir, "metadata.json")
	if err := os.WriteFile(metaPath, metaData, 0644); err != nil {
		return err
	}

	// Restore files (revert changes)
	if restore {
		return git.RestoreFiles(files...)
	}
	return nil
}

// CreateSnapshot creates a shelf with a snapshot group ID.
func (s *Store) CreateSnapshot(name string, files []string, restore bool, snapshotID string) error {
	if len(files) == 0 {
		return fmt.Errorf("no files to shelve")
	}

	dirName := fmt.Sprintf("%s_%s", time.Now().Format("20060102-150405.000"), sanitizeName(name))
	shelfDir := filepath.Join(s.dir, dirName)
	if err := os.MkdirAll(shelfDir, 0755); err != nil {
		return err
	}

	diff, err := git.DiffFiles(files...)
	if err != nil {
		return fmt.Errorf("failed to get diff: %w", err)
	}
	if diff == "" {
		// No changes for these files — clean up and skip
		os.RemoveAll(shelfDir)
		return nil
	}

	if len(diff) > 0 && diff[len(diff)-1] != '\n' {
		diff += "\n"
	}

	patchPath := filepath.Join(shelfDir, "patch.diff")
	if err := os.WriteFile(patchPath, []byte(diff), 0644); err != nil {
		return err
	}

	meta := Metadata{
		Name:      name,
		Branch:    git.CurrentBranch(),
		Commit:    git.HeadCommit(),
		CreatedAt: time.Now().Format(time.RFC3339Nano),
		Files:     files,
		Untracked: untrackedSubset(files),
		Worktree:  git.WorktreeName(),
		Snapshot:  snapshotID,
	}
	metaData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	metaPath := filepath.Join(shelfDir, "metadata.json")
	if err := os.WriteFile(metaPath, metaData, 0644); err != nil {
		return err
	}

	if restore {
		return git.RestoreFiles(files...)
	}
	return nil
}

// List returns all shelves sorted by creation time (newest first).
func (s *Store) List() ([]Shelf, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var shelves []Shelf
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		metaPath := filepath.Join(s.dir, entry.Name(), "metadata.json")
		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}
		var meta Metadata
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}
		shelves = append(shelves, Shelf{
			Meta:     meta,
			PatchDir: filepath.Join(s.dir, entry.Name()),
		})
	}

	sort.Slice(shelves, func(i, j int) bool {
		return shelves[i].Meta.CreatedAt > shelves[j].Meta.CreatedAt
	})

	return shelves, nil
}

// ApplyDir applies a shelf's patch to the working tree using the shelf's directory path.
func (s *Store) ApplyDir(shelfDir string, force bool) error {
	patchPath := filepath.Join(shelfDir, "patch.diff")

	if _, err := os.Stat(patchPath); os.IsNotExist(err) {
		return fmt.Errorf("shelf not found at %s", shelfDir)
	}

	if force {
		meta, err := loadMetaFrom(shelfDir)
		if err == nil && len(meta.Files) > 0 {
			// Find conflicting files (exist in working tree as changes)
			changed := git.ChangedFileSet()
			var conflicting []string
			for _, f := range meta.Files {
				if changed[f] {
					conflicting = append(conflicting, f)
				}
			}
			if len(conflicting) > 0 {
				// Backup conflicting files to a temporary shelf
				backupName := "~backup-" + meta.Name
				_ = s.Create(backupName, conflicting, true)
			}
		}
	}

	if err := git.ApplyPatch(patchPath); err != nil {
		return err
	}

	// Stage the unshelved files so they become tracked
	meta, err := loadMetaFrom(shelfDir)
	if err == nil && len(meta.Files) > 0 {
		git.StageFiles(meta.Files...)
		// Unstage files that were originally untracked so they return to untracked state
		if len(meta.Untracked) > 0 {
			git.UnstageFiles(meta.Untracked...)
		}
	}
	return nil
}

// Apply applies a shelf's patch by name (legacy — searches for the shelf directory).
func (s *Store) Apply(name string, force bool) error {
	shelfDir := filepath.Join(s.dir, sanitizeName(name))
	return s.ApplyDir(shelfDir, force)
}

// DropDir deletes a shelf by its directory path.
func (s *Store) DropDir(shelfDir string) error {
	if _, err := os.Stat(shelfDir); os.IsNotExist(err) {
		return fmt.Errorf("shelf not found at %s", shelfDir)
	}
	return os.RemoveAll(shelfDir)
}

// Drop deletes a shelf by name (legacy — searches for the shelf directory).
func (s *Store) Drop(name string) error {
	return s.DropDir(filepath.Join(s.dir, sanitizeName(name)))
}

// RenameDir renames a shelf (updates metadata only, directory stays the same).
func (s *Store) RenameDir(shelfDir, newName string) error {
	metaPath := filepath.Join(shelfDir, "metadata.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return fmt.Errorf("shelf not found at %s", shelfDir)
	}
	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return err
	}
	meta.Name = newName
	newData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(metaPath, newData, 0644)
}

// Rename renames a shelf by name (legacy).
func (s *Store) Rename(oldName, newName string) error {
	oldDir := filepath.Join(s.dir, sanitizeName(oldName))
	newDir := filepath.Join(s.dir, sanitizeName(newName))

	if _, err := os.Stat(oldDir); os.IsNotExist(err) {
		return fmt.Errorf("shelf %q not found", oldName)
	}

	if err := os.Rename(oldDir, newDir); err != nil {
		return err
	}

	// Update metadata
	metaPath := filepath.Join(newDir, "metadata.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return err
	}
	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return err
	}
	meta.Name = newName
	newData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(metaPath, newData, 0644)
}

// GetPatchDir reads the patch content for a shelf by directory path.
func (s *Store) GetPatchDir(shelfDir string) (string, error) {
	patchPath := filepath.Join(shelfDir, "patch.diff")
	data, err := os.ReadFile(patchPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// GetPatch reads the patch content for a shelf by name (legacy).
func (s *Store) GetPatch(name string) (string, error) {
	return s.GetPatchDir(filepath.Join(s.dir, sanitizeName(name)))
}

// GetMetadataDir reads the metadata for a shelf by directory path.
func (s *Store) GetMetadataDir(shelfDir string) (*Metadata, error) {
	return loadMetaFrom(shelfDir)
}

// GetMetadata reads the metadata for a shelf by name (legacy).
func (s *Store) GetMetadata(name string) (*Metadata, error) {
	return loadMetaFrom(filepath.Join(s.dir, sanitizeName(name)))
}

func loadMetaFrom(shelfDir string) (*Metadata, error) {
	metaPath := filepath.Join(shelfDir, "metadata.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, err
	}
	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func sanitizeName(name string) string {
	r := strings.NewReplacer(" ", "-", "/", "-", "\\", "-")
	return r.Replace(name)
}

// untrackedSubset returns the subset of files that are currently untracked.
func untrackedSubset(files []string) []string {
	untracked, _ := git.UntrackedFiles()
	set := make(map[string]bool, len(untracked))
	for _, f := range untracked {
		set[f] = true
	}
	var result []string
	for _, f := range files {
		if set[f] {
			result = append(result, f)
		}
	}
	return result
}
