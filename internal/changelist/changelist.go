package changelist

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/ignaciotcrespo/gitshelf/internal/git"
)

const (
	UnversionedName = "Unversioned Files"
	DefaultName     = "Changes"
)

// Function variables for git operations, overridable in tests.
var (
	trackedChangedFilesFn = git.TrackedChangedFiles
	untrackedFilesFn      = git.UntrackedFiles
	marshalIndentFn       = json.MarshalIndent
)

// Changelist represents a logical group of changed files.
type Changelist struct {
	Name       string            `json:"name"`
	Files      []string          `json:"files"`
	DiffHashes map[string]string `json:"diff_hashes,omitempty"`
}

// State holds all changelist data.
type State struct {
	Active      string       `json:"active,omitempty"` // deprecated: kept for backward compat
	Changelists []Changelist `json:"changelists"`
}

// Store manages changelist persistence.
type Store struct {
	dir string
}

// NewStore creates a store rooted at the given .git/gitshelf directory.
func NewStore(gitshelfDir string) *Store {
	return &Store{dir: gitshelfDir}
}

func (s *Store) path() string {
	return filepath.Join(s.dir, "changelists.json")
}

// Load reads the changelist state from disk.
func (s *Store) Load() (*State, error) {
	data, err := os.ReadFile(s.path())
	if err != nil {
		if os.IsNotExist(err) {
			return &State{
				Active: DefaultName,
				Changelists: []Changelist{
					{Name: DefaultName},
				},
			}, nil
		}
		return nil, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// Save writes the changelist state to disk.
func (s *Store) Save(state *State) error {
	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return err
	}
	data, err := marshalIndentFn(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path(), data, 0644)
}

// AssignFile assigns a file to a changelist. Removes it from any other changelist first.
func AssignFile(state *State, file, changelistName string) {
	// Remove from all changelists (including hashes)
	for i := range state.Changelists {
		state.Changelists[i].Files = removeString(state.Changelists[i].Files, file)
		delete(state.Changelists[i].DiffHashes, file)
	}
	// Add to target (create CL if it doesn't exist)
	for i := range state.Changelists {
		if state.Changelists[i].Name == changelistName {
			state.Changelists[i].Files = append(state.Changelists[i].Files, file)
			return
		}
	}
	state.Changelists = append(state.Changelists, Changelist{Name: changelistName, Files: []string{file}})
}

// AutoAssignNewFiles assigns tracked changed files not in any changelist
// to the active changelist. Untracked files go to "Unversioned Files".
// It also removes stale files that are no longer changed from all changelists.
func AutoAssignNewFiles(state *State) error {
	tracked, err := trackedChangedFilesFn()
	if err != nil {
		return err
	}
	untracked, err := untrackedFilesFn()
	if err != nil {
		return err
	}

	// Build set of currently changed files
	changedSet := make(map[string]bool)
	for _, f := range tracked {
		changedSet[f] = true
	}
	for _, f := range untracked {
		changedSet[f] = true
	}

	// Remove stale files from all changelists
	for i := range state.Changelists {
		var kept []string
		for _, f := range state.Changelists[i].Files {
			if changedSet[f] {
				kept = append(kept, f)
			}
		}
		state.Changelists[i].Files = kept
	}

	// Build set of already-assigned files
	assigned := make(map[string]bool)
	for _, cl := range state.Changelists {
		for _, f := range cl.Files {
			assigned[f] = true
		}
	}

	for _, f := range tracked {
		if !assigned[f] {
			AssignFile(state, f, DefaultName)
		}
	}

	for _, f := range untracked {
		if !assigned[f] {
			AssignFile(state, f, UnversionedName)
		}
	}

	return nil
}

// FilesForChangelist returns the files in a changelist that are actually still changed.
func FilesForChangelist(state *State, name string) ([]string, error) {
	tracked, err := trackedChangedFilesFn()
	if err != nil {
		return nil, err
	}
	untracked, err := untrackedFilesFn()
	if err != nil {
		return nil, err
	}
	changedSet := make(map[string]bool)
	for _, f := range tracked {
		changedSet[f] = true
	}
	for _, f := range untracked {
		changedSet[f] = true
	}

	for _, cl := range state.Changelists {
		if cl.Name == name {
			var result []string
			for _, f := range cl.Files {
				if changedSet[f] {
					result = append(result, f)
				}
			}
			return result, nil
		}
	}
	return nil, nil
}

// AddChangelist creates a new changelist.
func AddChangelist(state *State, name string) {
	for _, cl := range state.Changelists {
		if cl.Name == name {
			return
		}
	}
	state.Changelists = append(state.Changelists, Changelist{Name: name})
}

// RemoveChangelist removes a changelist (files become unassigned).
func RemoveChangelist(state *State, name string) {
	if name == DefaultName {
		return
	}
	for i, cl := range state.Changelists {
		if cl.Name == name {
			state.Changelists = append(state.Changelists[:i], state.Changelists[i+1:]...)
			return
		}
	}
}

// RenameChangelist renames a changelist.
func RenameChangelist(state *State, oldName, newName string) {
	for i := range state.Changelists {
		if state.Changelists[i].Name == oldName {
			state.Changelists[i].Name = newName
			return
		}
	}
}

// AllNames returns all changelist names.
func AllNames(state *State) []string {
	var names []string
	for _, cl := range state.Changelists {
		names = append(names, cl.Name)
	}
	return names
}

// ComputeDirty compares stored diff hashes against current ones.
// For files with no stored hash, it stores the current hash (new baseline).
// Returns sets of dirty files and dirty changelist names.
// Only user-created changelists are checked (not "Changes" or "Unversioned Files").
func ComputeDirty(state *State, currentHashes map[string]string) (dirtyFiles map[string]bool, dirtyCLs map[string]bool) {
	dirtyFiles = make(map[string]bool)
	dirtyCLs = make(map[string]bool)

	for i := range state.Changelists {
		cl := &state.Changelists[i]
		if cl.Name == DefaultName || cl.Name == UnversionedName {
			continue
		}
		if cl.DiffHashes == nil {
			cl.DiffHashes = make(map[string]string)
		}
		for _, file := range cl.Files {
			currentHash := currentHashes[file]
			storedHash, exists := cl.DiffHashes[file]
			if !exists {
				// New file in this CL — store baseline
				cl.DiffHashes[file] = currentHash
			} else if storedHash != currentHash {
				dirtyFiles[file] = true
				dirtyCLs[cl.Name] = true
			}
		}
		// Clean up hashes for files no longer in this CL
		for file := range cl.DiffHashes {
			found := false
			for _, f := range cl.Files {
				if f == file {
					found = true
					break
				}
			}
			if !found {
				delete(cl.DiffHashes, file)
			}
		}
	}
	return dirtyFiles, dirtyCLs
}

// AcceptDirtyFiles updates the stored diff hashes for the given files
// in the specified changelist, setting the current hashes as the new baseline.
func AcceptDirtyFiles(state *State, clName string, files []string, currentHashes map[string]string) {
	for i := range state.Changelists {
		if state.Changelists[i].Name == clName {
			if state.Changelists[i].DiffHashes == nil {
				state.Changelists[i].DiffHashes = make(map[string]string)
			}
			for _, f := range files {
				state.Changelists[i].DiffHashes[f] = currentHashes[f]
			}
			return
		}
	}
}

// AcceptDirtyCL updates all stored diff hashes for a changelist to current values.
func AcceptDirtyCL(state *State, clName string, currentHashes map[string]string) {
	for i := range state.Changelists {
		if state.Changelists[i].Name == clName {
			if state.Changelists[i].DiffHashes == nil {
				state.Changelists[i].DiffHashes = make(map[string]string)
			}
			for _, f := range state.Changelists[i].Files {
				state.Changelists[i].DiffHashes[f] = currentHashes[f]
			}
			return
		}
	}
}

func removeString(slice []string, s string) []string {
	var result []string
	for _, v := range slice {
		if v != s {
			result = append(result, v)
		}
	}
	return result
}
