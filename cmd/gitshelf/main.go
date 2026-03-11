package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ignaciotcrespo/gitshelf/internal/git"
	"github.com/ignaciotcrespo/gitshelf/internal/ui"
)

// version is set by goreleaser via ldflags.
var version = "dev"

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println("gitshelf", version)
		return
	}

	// Find git repo root
	root, err := git.RepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: not a git repository")
		os.Exit(1)
	}

	gitshelfDir := filepath.Join(root, ".gitshelf")

	// Migrate from old location (.git/gitshelf) if needed
	oldDir := filepath.Join(root, ".git", "gitshelf")
	if info, err := os.Stat(oldDir); err == nil && info.IsDir() {
		if _, err := os.Stat(gitshelfDir); os.IsNotExist(err) {
			if err := os.Rename(oldDir, gitshelfDir); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not migrate %s → %s: %v\n", oldDir, gitshelfDir, err)
			}
		}
	}

	// Ensure .gitshelf directory exists
	if err := os.MkdirAll(gitshelfDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating gitshelf directory: %v\n", err)
		os.Exit(1)
	}

	// Add .gitshelf/ to .gitignore if not already there
	ensureGitignore(root)

	model := ui.NewModel(gitshelfDir, version)

	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithReportFocus())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// ensureGitignore adds .gitshelf/ to .gitignore if not already present.
func ensureGitignore(repoRoot string) {
	gitignorePath := filepath.Join(repoRoot, ".gitignore")
	entry := ".gitshelf/"

	// Read existing content (if any)
	existing, _ := os.ReadFile(gitignorePath)

	// Check if already ignored
	for _, line := range strings.Split(string(existing), "\n") {
		line = strings.TrimSpace(line)
		if line == entry || line == ".gitshelf" {
			return
		}
	}

	// Build the line to append
	var prefix string
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		prefix = "\n"
	}

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	f.WriteString(prefix + entry + "\n")
	fmt.Fprintf(os.Stderr, "Added %s to .gitignore\n", entry)
}
