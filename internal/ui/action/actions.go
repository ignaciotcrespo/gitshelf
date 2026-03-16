package action

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ignaciotcrespo/gitshelf/internal/changelist"
	"github.com/ignaciotcrespo/gitshelf/internal/git"
	"github.com/ignaciotcrespo/gitshelf/internal/shelf"
	"github.com/ignaciotcrespo/gitshelf/internal/types"
	"github.com/ignaciotcrespo/gitshelf/internal/ui/prompt"
)


// Stores holds references to the data stores needed by actions.
type Stores struct {
	CL    *changelist.Store
	Shelf *shelf.Store
	State *changelist.State
}

// Logger is the interface for status/error reporting.
type Logger interface {
	SetStatus(msg string)
	SetError(msg string)
}

// Execute processes a completed prompt result and performs the corresponding action.
// Returns true if data was modified and a refresh is needed.
func Execute(r *prompt.Result, stores *Stores, log Logger, ctx *ActionContext) bool {
	if r == nil {
		return false
	}

	switch r.Mode {
	case types.PromptConfirm:
		if r.Confirmed {
			return executeConfirm(r, stores, log, ctx)
		}
		return false

	case types.PromptNewChangelist:
		changelist.AddChangelist(stores.State, r.Value)
		saveCL(stores, log)
		log.SetStatus(fmt.Sprintf("Created: %s", r.Value))
		return true

	case types.PromptRenameChangelist:
		changelist.RenameChangelist(stores.State, ctx.OldName, r.Value)
		saveCL(stores, log)
		log.SetStatus(fmt.Sprintf("Renamed → %s", r.Value))
		return true

	case types.PromptShelveFiles:
		return executeShelve(r, stores, log, ctx)

	case types.PromptRenameShelf:
		if ctx.ShelfDir != "" {
			if err := stores.Shelf.RenameDir(ctx.ShelfDir, r.Value); err != nil {
				log.SetError(fmt.Sprintf("Error: %v", err))
				return false
			}
		} else if err := stores.Shelf.Rename(ctx.OldName, r.Value); err != nil {
			log.SetError(fmt.Sprintf("Error: %v", err))
			return false
		}
		log.SetStatus(fmt.Sprintf("Renamed → %s", r.Value))
		return true

	case types.PromptMoveFile:
		return executeMove(r, stores, log, ctx)

	case types.PromptCommit:
		return executeCommit(r, stores, log, ctx)

	case types.PromptAmend:
		return executeAmend(r, stores, log, ctx)

	case types.PromptUnshelve:
		return executeUnshelve(r, stores, log, ctx)

	case types.PromptPasteChangelist:
		return executePasteChangelist(r, stores, log, ctx)

	case types.PromptPush:
		return executePush(r, log)

	case types.PromptPull:
		return executePull(r, log)
	}

	return false
}

// ActionContext provides data needed by specific actions.
type ActionContext struct {
	SelectedFiles      map[string]bool   // files selected for commit/shelve
	CLName             string            // current changelist name
	OldName            string            // for rename operations
	MoveFile           string            // file being moved
	ShelfName          string            // shelf name (display)
	ShelfDir           string            // shelf directory path (for operations)
	ForceUnshelve      bool              // overwrite existing files on unshelve
	DirtyFiles         map[string]bool   // currently dirty files
	DiffHashes         map[string]string // current diff hashes for accept dirty
	SourceWorktreePath string   // for paste: source worktree to read files/diffs from
	ClipboardCLName    string   // for paste: CL name to create
	ClipboardFiles     []string // for paste: files to assign
}

func executeConfirm(r *prompt.Result, stores *Stores, log Logger, ctx *ActionContext) bool {
	switch r.ConfirmAction {
	case types.ConfirmDeleteChangelist:
		changelist.RemoveChangelist(stores.State, r.ConfirmTarget)
		saveCL(stores, log)
		log.SetStatus(fmt.Sprintf("Deleted: %s", r.ConfirmTarget))
		return true

	case types.ConfirmDropShelf:
		if ctx != nil && ctx.ShelfDir != "" {
			if err := stores.Shelf.DropDir(ctx.ShelfDir); err != nil {
				log.SetError(fmt.Sprintf("Error: %v", err))
				return false
			}
		} else if err := stores.Shelf.Drop(r.ConfirmTarget); err != nil {
			log.SetError(fmt.Sprintf("Error: %v", err))
			return false
		}
		log.SetStatus(fmt.Sprintf("Dropped: %s", r.ConfirmTarget))
		return true

	case types.ConfirmAcceptDirty:
		return executeAcceptDirty(r.ConfirmTarget, stores, log, ctx)

	case types.ConfirmSnapshotUnshelve:
		return executeSnapshotUnshelve(r.ConfirmTarget, stores, log)
	}
	return false
}

func executeShelve(r *prompt.Result, stores *Stores, log Logger, ctx *ActionContext) bool {
	var files []string
	if len(ctx.SelectedFiles) > 0 {
		for f := range ctx.SelectedFiles {
			files = append(files, f)
		}
	} else {
		files, _ = changelist.FilesForChangelist(stores.State, ctx.CLName)
	}

	if len(files) == 0 {
		log.SetError("No changed files to shelve")
		return false
	}

	log.SetStatus(fmt.Sprintf("Shelve: %s (%d files)", r.Value, len(files)))
	err := stores.Shelf.Create(r.Value, files, true)
	if err != nil {
		log.SetError(fmt.Sprintf("Shelve error: %v", err))
		return false
	}

	// Remove shelved files from changelist
	for i := range stores.State.Changelists {
		if stores.State.Changelists[i].Name == ctx.CLName {
			for _, f := range files {
				stores.State.Changelists[i].Files = removeStr(stores.State.Changelists[i].Files, f)
			}
			break
		}
	}
	saveCL(stores, log)
	return true
}

func executeMove(r *prompt.Result, stores *Stores, log Logger, ctx *ActionContext) bool {
	// Create changelist if it doesn't exist
	changelist.AddChangelist(stores.State, r.Value)

	// Move checked files if any, otherwise move the single cursor file
	var files []string
	if len(ctx.SelectedFiles) > 0 {
		for f := range ctx.SelectedFiles {
			files = append(files, f)
		}
	} else if ctx.MoveFile != "" {
		files = []string{ctx.MoveFile}
	}

	for _, f := range files {
		changelist.AssignFile(stores.State, f, r.Value)
	}
	saveCL(stores, log)
	log.SetStatus(fmt.Sprintf("Moved %d file(s) → %s", len(files), r.Value))
	return true
}

func executeCommit(r *prompt.Result, stores *Stores, log Logger, ctx *ActionContext) bool {
	var files []string
	for f := range ctx.SelectedFiles {
		files = append(files, f)
	}
	if len(files) == 0 {
		log.SetError("No files to commit")
		return false
	}

	log.SetStatus(fmt.Sprintf("Commit %d file(s): %s", len(files), r.Value))
	err := git.CommitFiles(files, r.Value)
	if err != nil {
		log.SetError(fmt.Sprintf("Commit error: %v", err))
		return false
	}

	for i := range stores.State.Changelists {
		for _, f := range files {
			stores.State.Changelists[i].Files = removeStr(stores.State.Changelists[i].Files, f)
		}
	}
	saveCL(stores, log)
	return true
}

func executeAmend(r *prompt.Result, stores *Stores, log Logger, ctx *ActionContext) bool {
	var files []string
	for f := range ctx.SelectedFiles {
		files = append(files, f)
	}
	if len(files) == 0 {
		log.SetError("No files to amend")
		return false
	}

	log.SetStatus(fmt.Sprintf("Amend %d file(s): %s", len(files), r.Value))
	err := git.AmendFiles(files, r.Value)
	if err != nil {
		log.SetError(fmt.Sprintf("Amend error: %v", err))
		return false
	}

	for i := range stores.State.Changelists {
		for _, f := range files {
			stores.State.Changelists[i].Files = removeStr(stores.State.Changelists[i].Files, f)
		}
	}
	saveCL(stores, log)
	return true
}

func executeUnshelve(r *prompt.Result, stores *Stores, log Logger, ctx *ActionContext) bool {
	if ctx.ShelfDir == "" && ctx.ShelfName == "" {
		log.SetError("No shelf to unshelve")
		return false
	}

	log.SetStatus(fmt.Sprintf("Unshelve: %s → %s", ctx.ShelfName, r.Value))
	var applyErr error
	if ctx.ShelfDir != "" {
		applyErr = stores.Shelf.ApplyDir(ctx.ShelfDir, ctx.ForceUnshelve)
	} else {
		applyErr = stores.Shelf.Apply(ctx.ShelfName, ctx.ForceUnshelve)
	}
	if applyErr != nil {
		log.SetError(fmt.Sprintf("Unshelve error: %v", applyErr))
		return false
	}

	// Assign unshelved files directly to the target changelist
	targetCL := r.Value
	changelist.AddChangelist(stores.State, targetCL)

	// Read shelf metadata to get file list and pre-assign them
	var meta *shelf.Metadata
	var err error
	if ctx.ShelfDir != "" {
		meta, err = stores.Shelf.GetMetadataDir(ctx.ShelfDir)
	} else {
		meta, err = stores.Shelf.GetMetadata(ctx.ShelfName)
	}
	if err == nil {
		for _, f := range meta.Files {
			changelist.AssignFile(stores.State, f, targetCL)
		}
	}
	saveCL(stores, log)
	return true
}

func executePush(r *prompt.Result, log Logger) bool {
	remote := r.Value
	log.SetStatus(fmt.Sprintf("Push to %s", remote))
	if err := git.Push(remote); err != nil {
		log.SetError(fmt.Sprintf("Push error: %v", err))
		return false
	}
	return false
}

func executePull(r *prompt.Result, log Logger) bool {
	remote := r.Value
	log.SetStatus(fmt.Sprintf("Pull from %s", remote))
	if err := git.Pull(remote); err != nil {
		log.SetError(fmt.Sprintf("Pull error: %v", err))
		return false
	}
	return true
}

func executeAcceptDirty(target string, stores *Stores, log Logger, ctx *ActionContext) bool {
	parts := strings.SplitN(target, ":", 3)
	if len(parts) != 3 {
		return false
	}
	kind, clName := parts[0], parts[1]

	currentHashes := git.FileDiffHashes()

	if kind == "cl" {
		changelist.AcceptDirtyCL(stores.State, clName, currentHashes)
		saveCL(stores, log)
		log.SetStatus(fmt.Sprintf("Accepted dirty changes in '%s'", clName))
		return true
	}

	// files mode — accept selected dirty files or single file
	var files []string
	if ctx != nil && len(ctx.SelectedFiles) > 0 {
		for f := range ctx.SelectedFiles {
			if ctx.DirtyFiles[f] {
				files = append(files, f)
			}
		}
	}
	if len(files) == 0 {
		return false
	}
	changelist.AcceptDirtyFiles(stores.State, clName, files, currentHashes)
	saveCL(stores, log)
	log.SetStatus(fmt.Sprintf("Accepted %d dirty file(s) in '%s'", len(files), clName))
	return true
}

func executePasteChangelist(r *prompt.Result, stores *Stores, log Logger, ctx *ActionContext) bool {
	if ctx == nil || ctx.ClipboardCLName == "" || len(ctx.ClipboardFiles) == 0 {
		log.SetError("Nothing to paste")
		return false
	}

	clName := ctx.ClipboardCLName
	files := ctx.ClipboardFiles

	switch r.Value {
	case types.PasteFullContent:
		return pasteFullContent(stores, log, ctx, clName, files)
	case types.PasteApplyDiff:
		return pasteApplyDiff(stores, log, ctx, clName, files)
	case types.PasteOnlyCL:
		return pasteOnlyCL(stores, log, clName, files)
	default:
		log.SetError(fmt.Sprintf("Unknown paste mode: %s", r.Value))
		return false
	}
}

// pasteFullContent copies file contents from the source worktree and assigns them to the CL.
func pasteFullContent(stores *Stores, log Logger, ctx *ActionContext, clName string, files []string) bool {
	root, err := git.RepoRoot()
	if err != nil {
		log.SetError(fmt.Sprintf("Error: %v", err))
		return false
	}

	var copied int
	for _, f := range files {
		srcPath := filepath.Join(ctx.SourceWorktreePath, f)
		dstPath := filepath.Join(root, f)

		data, err := os.ReadFile(srcPath)
		if err != nil {
			continue // skip files that don't exist in source
		}

		// Ensure destination directory exists
		if dir := filepath.Dir(dstPath); dir != "." {
			os.MkdirAll(dir, 0755)
		}

		if err := os.WriteFile(dstPath, data, 0644); err != nil {
			log.SetError(fmt.Sprintf("Error writing %s: %v", f, err))
			return false
		}
		copied++
	}

	changelist.AddChangelist(stores.State, clName)
	for _, f := range files {
		changelist.AssignFile(stores.State, f, clName)
	}
	saveCL(stores, log)

	log.SetStatus(fmt.Sprintf("Pasted '%s': %d file(s) copied", clName, copied))
	return true
}

// pasteApplyDiff generates diffs from the source worktree and applies them.
func pasteApplyDiff(stores *Stores, log Logger, ctx *ActionContext, clName string, files []string) bool {
	patch, err := git.DiffFilesIn(ctx.SourceWorktreePath, files...)
	if err != nil {
		log.SetError(fmt.Sprintf("Diff error: %v", err))
		return false
	}

	if patch == "" {
		log.SetError("No diff to apply")
		return false
	}

	if err := git.ApplyPatchFromString(patch); err != nil {
		log.SetError(fmt.Sprintf("Apply error: %v", err))
		return false
	}

	changelist.AddChangelist(stores.State, clName)
	for _, f := range files {
		changelist.AssignFile(stores.State, f, clName)
	}
	saveCL(stores, log)

	log.SetStatus(fmt.Sprintf("Pasted '%s': diff applied (%d files)", clName, len(files)))
	return true
}

// pasteOnlyCL assigns the clipboard files to a new changelist without modifying any files.
func pasteOnlyCL(stores *Stores, log Logger, clName string, files []string) bool {
	changelist.AddChangelist(stores.State, clName)
	for _, f := range files {
		changelist.AssignFile(stores.State, f, clName)
	}
	saveCL(stores, log)

	log.SetStatus(fmt.Sprintf("Pasted '%s': %d file(s) assigned", clName, len(files)))
	return true
}

func saveCL(stores *Stores, log Logger) {
	if err := stores.CL.Save(stores.State); err != nil {
		log.SetError(fmt.Sprintf("Save error: %v", err))
	}
}

// ExecuteSnapshotShelve shelves all changelists that have changed files, grouped by a shared snapshot ID.
func ExecuteSnapshotShelve(stores *Stores, log Logger) bool {
	snapshotID := time.Now().Format("20060102-150405.000")
	changed := git.ChangedFileSet()
	var totalFiles int

	for _, cl := range stores.State.Changelists {
		// Only shelve files that are actually changed in the working tree
		var files []string
		for _, f := range cl.Files {
			if changed[f] {
				files = append(files, f)
			}
		}
		if len(files) == 0 {
			continue
		}

		err := stores.Shelf.CreateSnapshot(cl.Name, files, true, snapshotID)
		if err != nil {
			log.SetError(fmt.Sprintf("Snapshot shelve error on '%s': %v", cl.Name, err))
			return false
		}
		totalFiles += len(files)

		// Remove shelved files from CL
		for i := range stores.State.Changelists {
			if stores.State.Changelists[i].Name == cl.Name {
				for _, f := range files {
					stores.State.Changelists[i].Files = removeStr(stores.State.Changelists[i].Files, f)
				}
				break
			}
		}
	}

	if totalFiles == 0 {
		log.SetError("No changed files to shelve")
		return false
	}

	saveCL(stores, log)
	log.SetStatus(fmt.Sprintf("Snapshot shelved %d file(s)", totalFiles))
	return true
}

func executeSnapshotUnshelve(snapshotID string, stores *Stores, log Logger) bool {
	shelves, err := stores.Shelf.List()
	if err != nil {
		log.SetError(fmt.Sprintf("Error listing shelves: %v", err))
		return false
	}

	var restored int
	for _, s := range shelves {
		if s.Meta.Snapshot != snapshotID {
			continue
		}
		if err := stores.Shelf.ApplyDir(s.PatchDir, false); err != nil {
			log.SetError(fmt.Sprintf("Unshelve error for '%s': %v", s.Meta.Name, err))
			return false
		}
		// Assign files to a CL named after the shelf
		changelist.AddChangelist(stores.State, s.Meta.Name)
		for _, f := range s.Meta.Files {
			changelist.AssignFile(stores.State, f, s.Meta.Name)
		}
		restored++
	}

	if restored == 0 {
		log.SetError("No snapshot shelves found")
		return false
	}

	saveCL(stores, log)
	log.SetStatus(fmt.Sprintf("Unshelved %d changelist(s)", restored))
	return true
}

func removeStr(slice []string, s string) []string {
	var result []string
	for _, v := range slice {
		if v != s {
			result = append(result, v)
		}
	}
	return result
}
