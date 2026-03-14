package git

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// LogLevel controls which git commands are logged.
type LogLevel int

const (
	LogActions LogLevel = iota // only user-triggered commands (default)
	LogAll                     // all git commands including queries
)

// LogEntry represents a git command execution.
type LogEntry struct {
	Command string
	Output  string
	Error   string
}

var (
	logMu    sync.Mutex
	cmdLog   []LogEntry
	repoRoot string   // cached repo root, set by InitRepoRoot
	Level    LogLevel // controls what gets logged; default LogActions
)

// GetLog returns all logged git commands.
func GetLog() []LogEntry {
	logMu.Lock()
	defer logMu.Unlock()
	result := make([]LogEntry, len(cmdLog))
	copy(result, cmdLog)
	return result
}

// ClearLog clears the command log.
func ClearLog() {
	logMu.Lock()
	defer logMu.Unlock()
	cmdLog = nil
}

// SetRepoRoot overrides the cached repo root. Use empty string to clear.
func SetRepoRoot(root string) {
	repoRoot = root
}

// AddUserLog adds a non-command log entry (for status/error messages from the app).
func AddUserLog(output, errMsg string) {
	logMu.Lock()
	defer logMu.Unlock()
	cmdLog = append(cmdLog, LogEntry{
		Command: "",
		Output:  output,
		Error:   errMsg,
	})
}

func addLog(args []string, stdout, stderr string) {
	logMu.Lock()
	defer logMu.Unlock()
	cmdLog = append(cmdLog, LogEntry{
		Command: "git " + strings.Join(args, " "),
		Output:  stdout,
		Error:   stderr,
	})
}

// RepoRoot returns the root directory of the current git repository.
// It caches the result so all git commands run from the correct directory.
func RepoRoot() (string, error) {
	if repoRoot != "" {
		return repoRoot, nil
	}
	root, err := query("rev-parse", "--show-toplevel")
	if err == nil {
		repoRoot = root
	}
	return root, err
}

// CurrentBranch returns the current branch name.
func CurrentBranch() string {
	out, err := query("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	return out
}

// HeadCommit returns the current HEAD commit hash (short).
func HeadCommit() string {
	out, err := query("rev-parse", "--short", "HEAD")
	if err != nil {
		return ""
	}
	return out
}

// TrackedChangedFiles returns modified/deleted/staged files (not untracked).
func TrackedChangedFiles() ([]string, error) {
	out, err := query("status", "--porcelain")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}

	var files []string
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 3 {
			continue
		}
		if line[0] == '?' && line[1] == '?' {
			continue
		}
		file := strings.TrimSpace(line[2:])
		if idx := strings.Index(file, " -> "); idx >= 0 {
			file = file[idx+4:]
		}
		files = append(files, unquoteGitPath(file))
	}
	return files, nil
}

// DiffFiles generates a unified diff for the given files.
// Handles both tracked and untracked files.
func DiffFiles(files ...string) (string, error) {
	// Get untracked file set
	untracked, _ := UntrackedFiles()
	untrackedSet := make(map[string]bool, len(untracked))
	for _, f := range untracked {
		untrackedSet[f] = true
	}

	// Split into tracked and untracked
	var tracked []string
	var untrackedFiles []string
	for _, f := range files {
		if untrackedSet[f] {
			untrackedFiles = append(untrackedFiles, f)
		} else {
			tracked = append(tracked, f)
		}
	}

	var parts []string

	// Tracked files: diff against HEAD to capture both staged and unstaged changes
	if len(tracked) > 0 {
		args := []string{"diff", "HEAD", "--"}
		args = append(args, tracked...)
		out, err := queryRaw(args...)
		if err != nil {
			return "", err
		}
		if out != "" {
			parts = append(parts, out)
		}
	}

	// Untracked files: use --no-index against /dev/null
	for _, f := range untrackedFiles {
		out, _ := queryIgnoreExit("diff", "--no-index", "--", "/dev/null", f)
		if out != "" {
			parts = append(parts, out)
		}
	}

	return strings.Join(parts, ""), nil
}

// DiffFile returns the diff for a single file.
// For untracked files, it uses --no-index to show full content as a diff.
func DiffFile(file string) (string, error) {
	out, err := query("diff", "--", file)
	if err == nil && out != "" {
		return out, nil
	}
	// No diff output — file may be untracked. Use --no-index against /dev/null.
	// Since all commands run from repo root, the relative path works directly.
	noIndex, _ := queryIgnoreExit("diff", "--no-index", "--", "/dev/null", file)
	if noIndex != "" {
		return noIndex, nil
	}
	return out, err
}

// DiffAll returns the diff for all working tree changes.
func DiffAll() (string, error) {
	return query("diff")
}

// UntrackedFiles returns files that are new/untracked.
func UntrackedFiles() ([]string, error) {
	out, err := query("ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		lines[i] = unquoteGitPath(l)
	}
	return lines, nil
}

// RestoreFiles reverts the given files to HEAD state.
// Tracked files are restored via git restore; untracked files are deleted.
func RestoreFiles(files ...string) error {
	untracked, _ := UntrackedFiles()
	untrackedSet := make(map[string]bool, len(untracked))
	for _, f := range untracked {
		untrackedSet[f] = true
	}

	var tracked []string
	var untrackedFiles []string
	for _, f := range files {
		if untrackedSet[f] {
			untrackedFiles = append(untrackedFiles, f)
		} else {
			tracked = append(tracked, f)
		}
	}

	// Delete untracked files using git clean -f for each file
	for _, f := range untrackedFiles {
		action("clean", "-f", "--", f)
	}

	if len(tracked) > 0 {
		// Restore both staged and unstaged changes
		args := []string{"restore", "--staged", "--worktree", "--"}
		args = append(args, tracked...)
		_, err := action(args...)
		return err
	}
	return nil
}

// ApplyPatch applies a patch file to the working tree.
func ApplyPatch(patchPath string) error {
	absPath, err := filepath.Abs(patchPath)
	if err != nil {
		return err
	}
	_, err = action("apply", "--whitespace=fix", absPath)
	return err
}

// StageFiles stages the given files (git add).
func StageFiles(files ...string) error {
	args := []string{"add", "--"}
	args = append(args, files...)
	_, err := action(args...)
	return err
}

// CommitFiles stages the given files and commits with the given message.
// Uses "git commit -- <files>" to commit only the specified files
// without disturbing any other staged changes in the index.
func CommitFiles(files []string, message string) error {
	// Stage files first (needed for new/untracked files)
	args := []string{"add", "--"}
	args = append(args, files...)
	if _, err := action(args...); err != nil {
		return err
	}

	// Commit only these files — leaves other staged files untouched
	commitArgs := []string{"commit", "-m", message, "--"}
	commitArgs = append(commitArgs, files...)
	_, err := action(commitArgs...)
	return err
}

// LastCommitMessage returns the message of the most recent commit.
func LastCommitMessage() string {
	out, err := query("log", "-1", "--format=%s")
	if err != nil {
		return ""
	}
	return out
}

// AmendFiles stages the given files and amends the last commit.
// Uses "git commit -- <files>" to amend only with the specified files
// without disturbing any other staged changes in the index.
func AmendFiles(files []string, message string) error {
	// Stage files first (needed for new/untracked files)
	args := []string{"add", "--"}
	args = append(args, files...)
	if _, err := action(args...); err != nil {
		return err
	}

	// Amend only with these files — leaves other staged files untouched
	commitArgs := []string{"commit", "--amend", "-m", message, "--"}
	commitArgs = append(commitArgs, files...)
	_, err := action(commitArgs...)
	return err
}

// AheadBehind returns how many commits the current branch is ahead/behind its upstream.
// Returns (0, 0) if there's no upstream or on error.
func AheadBehind() (ahead, behind int) {
	out, err := query("rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	if err != nil {
		return 0, 0
	}
	fmt.Sscanf(out, "%d\t%d", &ahead, &behind)
	return ahead, behind
}

// Remotes returns the list of configured remote names.
func Remotes() []string {
	out, err := query("remote")
	if err != nil || out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

// Push pushes the current branch to the given remote.
func Push(remote string) error {
	branch := CurrentBranch()
	if branch == "" {
		return fmt.Errorf("cannot determine current branch")
	}
	_, err := action("push", remote, branch)
	return err
}

// Pull pulls the current branch from the given remote.
func Pull(remote string) error {
	branch := CurrentBranch()
	if branch == "" {
		return fmt.Errorf("cannot determine current branch")
	}
	_, err := action("pull", remote, branch)
	return err
}

// action runs a git command that modifies state (always logged).
func action(args ...string) (string, error) {
	return run(args, true)
}

// query runs a git command that reads state (logged only if Level >= LogAll).
func query(args ...string) (string, error) {
	return run(args, Level >= LogAll)
}

func run(args []string, log bool) (string, error) {
	cmd := exec.Command("git", args...)
	if repoRoot != "" {
		cmd.Dir = repoRoot
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()

	stdout := strings.TrimSpace(string(out))
	stderrStr := strings.TrimSpace(stderr.String())

	if err != nil {
		if log {
			addLog(args, stdout, stderrStr)
		}
		errMsg := stderrStr
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("%s", errMsg)
	}

	if log {
		// Command succeeded — stderr is informational, merge into output
		output := stdout
		if stderrStr != "" {
			if output != "" {
				output += "\n" + stderrStr
			} else {
				output = stderrStr
			}
		}
		addLog(args, output, "")
	}
	return stdout, nil
}

// queryIgnoreExit runs a query and returns stdout even if the exit code is non-zero.
func queryIgnoreExit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if repoRoot != "" {
		cmd.Dir = repoRoot
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, _ := cmd.Output()

	stdout := strings.TrimSpace(string(out))
	return stdout, nil
}

// queryRaw runs a query preserving raw output (no trimming).
func queryRaw(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if repoRoot != "" {
		cmd.Dir = repoRoot
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()

	stdout := string(out)
	if err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		errMsg := stderrStr
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("%s", errMsg)
	}
	return stdout, nil
}

// FileDiffHashes returns a map of file path to a hash of its diff content.
// Uses a single "git diff" call and splits the output by file.
func FileDiffHashes() map[string]string {
	out, err := queryRaw("diff")
	if err != nil || out == "" {
		return nil
	}

	result := make(map[string]string)
	// Split by "diff --git" boundaries
	sections := strings.Split(out, "diff --git ")
	for _, section := range sections[1:] { // skip first empty element
		// Extract file path from "a/path b/path\n..."
		firstLine := section
		if idx := strings.Index(section, "\n"); idx >= 0 {
			firstLine = section[:idx]
		}
		// Parse "a/path b/path"
		parts := strings.SplitN(firstLine, " b/", 2)
		if len(parts) != 2 {
			continue
		}
		file := parts[1]

		// Hash the full section content
		h := uint64(0)
		for _, b := range []byte(section) {
			h = h*31 + uint64(b)
		}
		result[file] = fmt.Sprintf("%x", h)
	}
	return result
}

// Worktree represents a git worktree entry.
type Worktree struct {
	Path      string
	Branch    string
	Commit    string
	IsCurrent bool
}

// WorktreeList returns all worktrees for the current repository.
// launchPath is the worktree where gitshelf was launched; it determines IsCurrent.
// If empty, falls back to RepoRoot().
func WorktreeList(launchPath string) ([]Worktree, error) {
	out, err := query("worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}

	root := launchPath
	if root == "" {
		root, _ = RepoRoot()
	}

	var worktrees []Worktree
	var current Worktree
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			if current.Path != "" {
				worktrees = append(worktrees, current)
			}
			current = Worktree{Path: strings.TrimPrefix(line, "worktree ")}
		case strings.HasPrefix(line, "HEAD "):
			commit := strings.TrimPrefix(line, "HEAD ")
			if len(commit) > 7 {
				commit = commit[:7]
			}
			current.Commit = commit
		case strings.HasPrefix(line, "branch "):
			branch := strings.TrimPrefix(line, "branch ")
			branch = strings.TrimPrefix(branch, "refs/heads/")
			current.Branch = branch
		case line == "bare":
			current.Branch = "(bare)"
		case line == "detached":
			current.Branch = "(detached)"
		}
	}
	if current.Path != "" {
		worktrees = append(worktrees, current)
	}

	// Mark current worktree
	for i := range worktrees {
		if worktrees[i].Path == root {
			worktrees[i].IsCurrent = true
		}
	}

	return worktrees, nil
}

// WorktreeName returns the basename of the current worktree directory.
// For the main worktree this is the repo folder name; for linked worktrees it's the worktree folder name.
func WorktreeName() string {
	root, err := RepoRoot()
	if err != nil {
		return ""
	}
	return filepath.Base(root)
}

// ChangedFileSet returns the set of all currently changed files (tracked + untracked).
func ChangedFileSet() map[string]bool {
	tracked, _ := TrackedChangedFiles()
	untracked, _ := UntrackedFiles()
	set := make(map[string]bool, len(tracked)+len(untracked))
	for _, f := range tracked {
		set[f] = true
	}
	for _, f := range untracked {
		set[f] = true
	}
	return set
}

// DiffFilesIn generates a unified diff for the given files in a specific directory.
func DiffFilesIn(dir string, files ...string) (string, error) {
	args := []string{"diff", "--"}
	args = append(args, files...)
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	stdout := strings.TrimSpace(string(out))
	if err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			return "", fmt.Errorf("%s", stderrStr)
		}
		return "", err
	}
	addLog(args, stdout, "")
	return stdout, nil
}

// ApplyPatchFromString applies a unified diff patch string to the current working tree.
func ApplyPatchFromString(patch string) error {
	root, err := RepoRoot()
	if err != nil {
		return err
	}
	cmd := exec.Command("git", "apply", "--whitespace=nowarn", "-")
	cmd.Dir = root
	cmd.Stdin = strings.NewReader(patch)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		addLog([]string{"apply"}, "", stderrStr)
		if stderrStr != "" {
			return fmt.Errorf("%s", stderrStr)
		}
		return err
	}
	addLog([]string{"apply"}, "Patch applied", "")
	return nil
}

// unquoteGitPath strips quotes that git adds around paths containing spaces or special chars.
func unquoteGitPath(path string) string {
	if len(path) >= 2 && path[0] == '"' && path[len(path)-1] == '"' {
		return path[1 : len(path)-1]
	}
	return path
}
