# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test

```bash
./build.sh              # go build + go install
go test ./...           # run all tests
go test ./internal/controller/ -run TestHandleKey_Tab   # single test
go test ./internal/integration/ -v                      # integration tests (22 end-to-end)
./test.sh --cover       # coverage report (coverage.out)
./test.sh --cover --html  # open HTML coverage
```

## Architecture

Gitshelf is a Go TUI (Bubbletea) that manages git changes via changelists and shelves, inspired by IntelliJ IDEA. Entry point: `cmd/gitshelf/main.go` → finds repo root → creates `.gitshelf/` dir → boots `ui.NewModel`.

**Three-layer design:**

- **Controller** (`internal/controller/`) — Pure state machine, no I/O. `HandleKey()` takes immutable state + `KeyContext` snapshot, returns new state + `RefreshFlag` + optional `PromptReq`. All logic is testable without mocks.
- **Stores** (`internal/changelist/`, `internal/shelf/`, `internal/git/`, `internal/diff/`) — Data access and business logic. Changelist state persists as JSON in `.gitshelf/changelists.json`. Shelves store patches in `.gitshelf/shelves/`.
- **UI** (`internal/ui/`) — Bubbletea Model implementing `Init`/`Update`/`View`. Split into `app.go` (model + update), `view.go` (layout), `loader.go` (data loading), `render.go` (panel rendering), `styles.go` (colors).

**Shared types** (`internal/types/`) — Enums (PanelID, PanelState, PromptMode, ConfirmAction) live here to avoid circular imports between controller and ui.

**Reusable framework** (`pkg/tui/`) — Generic panel-based TUI building blocks extracted from gitshelf. Provides `Box`, `Prompt`, `App`, navigation helpers, refresh flags. The `internal/ui/` layer wraps and configures `pkg/tui/` for gitshelf's domain. See `pkg/tui/README.md` for API docs.

**Key concepts:**
- **Pivot**: Either Changelists or Shelves — determines which context the Files and Diff panels reflect.
- **RefreshFlag**: Bitmask returned by controller telling the UI what to reload (RefreshDiff, RefreshCLFiles, RefreshAll). Panel focus changes trigger RefreshAll.
- **PromptReq**: Controller requests a prompt (text input or confirm dialog) instead of handling actions directly. The UI's `action/` package executes completed prompts.
- **Two-step prompts**: Shelve and unshelve require a text input followed by a confirmation dialog. The UI stores `pendingResult`/`pendingCtx` between the two steps (see `app.go:handlePromptResult`).
- **Quick-select options**: Move and unshelve prompts show `[X]` letter shortcuts for changelist picking.
- **Panel states**: Diff and Log panels cycle Normal → Maximized → Hidden.
- **Accent colors**: Bright white (255) for focused panel, light gray (251) for in-context, dim gray (240) for non-context.

## Testing Patterns

- Tests use temp git repos created by helpers (`setupCLRepo`, `setupShelfRepo`, `setupActionRepo`)
- Dependency injection via package-level function variables (e.g., `trackedChangedFilesFn`) — no mocking libraries
- Table-driven tests are standard, especially in controller
- All non-UI packages target ~100% coverage
- `git.SetRepoRoot(dir)` + `git.ClearLog()` for test isolation — points all git operations at a temp repo

## Integration Tests

`internal/integration/integration_test.go` contains 22 end-to-end tests using a **TestApp** — a headless replica of `app.go` that wires controller + stores + actions without Bubbletea rendering.

**TestApp API**: `PressKey(key)`, `TypePrompt(value)`, `Confirm()`, `SelectFile(idx)`, `WriteFile`/`WriteTrackedFile`, `refresh()`.

**Two test categories:**
- **Git-modifying** (11 tests) — commit, amend, shelve, unshelve, push, pull. Verify git outcomes via `gitLog()`, `gitStatus()`, `gitShow(file)`, `fileContent()`.
- **Git-safe** (11 tests) — create/rename/delete CL, move files, navigation, etc. Take a `gitSnapshot()` before the action, then `assertGitUnchanged(snap)` after. Snapshot compares `git status`, `git log`, and every working-tree file's content (excluding `.git/` and `.gitshelf/`).

**Important**: new files must be staged with `WriteTrackedFile` (calls `git add`) to appear in the "Changes" CL. Otherwise they go to "Unversioned Files".
