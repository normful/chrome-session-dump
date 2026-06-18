# chrome-session-dump

Parses Chrome/Chromium SNSS session files and outputs tab data as JSON or formatted text.

## Commands

- `go test ./...` — run all tests
- `go vet ./...` — static analysis (must pass before commit)
- `go fmt ./...` — format all Go files (run before commit)

## Project structure

- `chrome-session-dump.go` — single-file entry point. Contains: SNSS parser (`parse()`), output builder (`buildResult()`), session file finder (`findSession()`), printf-style formatter (`tabPrintf()`), CLI (`main()`).
- `main_test.go` — unit tests for `buildResult()`. Tests tab history preservation and edge cases.
- `go.mod` — module `github.com/normful/chrome-session-dump`, Go 1.26.

## Architecture

**SNSS file format:**
- Header: `"SNSS"` magic + int32 version (1 or 3).
- Body: sequence of pickled commands: `int16(size) int8(type_id) payload`.
- Commands reconstruct tab state: tab navigation, window/tab grouping, tab groups with color/collapse, window titles.

**Key functions:**
- `parse(path string) Result` — opens a session file, reads all commands, builds internal `tab`/`window`/`group` maps.
- `buildResult(tabs, windows, activeWindow) Result` — converts internal structures to public output structs (`Result` → `Window[]` → `Tab[]` → `HistoryItem[]`).
- `findSession(path string) string` — recursively finds the most recent `Session_*` file under a Chrome profile directory.
- `tabPrintf(format, tab, includeHistory)` — output formatting with `%u`/`%t`/`%g` placeholders.

**Output types:**
- `Result.Windows` — list of windows, each with `Tabs`, `TabGroups`, `Active`, `Deleted`, `UserTitle`
- `Tab` — `Url`, `Title`, `History[]`, `Active`, `Deleted`, `Group`, `GroupColor`, `GroupCollapsed`
- All history items are preserved (no truncation at current navigation index).

## Conventions

- Errors: `panic()` on any I/O or parse failure. Fail fast, full stack trace.
- No external dependencies beyond stdlib.
- The `tab`/`window`/`group` lowercase types are internal (SNSS-adjacent). The `Tab`/`Window`/`TabGroup`/`Result`/`HistoryItem` uppercase types are public output structs.
