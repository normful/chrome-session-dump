# chrome-session-dump

Parses Chrome/Chromium SNSS session files and outputs tab data as JSON or formatted text.

## Commands

- `go test ./...` — run all tests
- `go vet ./...` — static analysis (must pass before commit)
- `go fmt ./...` — format all Go files (run before commit)

## Usage

```bash
./bin/chrome-session-dump                              # needs a session file
./bin/chrome-session-dump ~/path/to/Session_XXXX       # explicit session file
./bin/chrome-session-dump /path/to/chrome/profile      # finds newest Session_* in dir
./bin/chrome-session-dump -json ...                    # JSON output (all tabs + history + metadata)
./bin/chrome-session-dump -history ...                 # include tab history in output
./bin/chrome-session-dump -active ...                  # active tab only
./bin/chrome-session-dump -deleted ...                 # include deleted tabs
./bin/chrome-session-dump -printf "%t → %u\\n" ...     # custom format (%u=url %t=title %g=group)
./bin/chrome-session-dump -merge-dedup ~/path/to/profile/dir  # merge all Session_* + *.bin, dedup by URL, output JSONL
./bin/chrome-session-dump -merge-dedup ~/dir -output out.jsonl # write merge-dedup output to file
```

### Flags

| Flag | Description |
|------|-------------|
| `-json` | JSON output including all tabs, history, metadata |
| `-active` | Print only the currently active tab |
| `-printf` | Custom printf-style format (`%u`=url, `%t`=title, `%g`=group) |
| `-deleted` | Include tabs marked as deleted |
| `-history` | Include each tab's navigation history |
| `-merge-dedup` | Batch-merge all session files under a directory, deduplicating by URL, outputting JSONL |
| `-output` | Write output to file instead of stdout (only with `-merge-dedup`) |

## Project structure

- `chrome-session-dump.go` — single-file entry point. Contains: SNSS parser (`parse()`), output builder (`buildResult()`), session file finder (`findSession()`), printf-style formatter (`tabPrintf()`), CLI (`main()` with flag registration).
- `merge_dedup.go` — merge-dedup batch mode. Contains: `findSessionFiles()` (recursive file discovery sorted by mtime), `safeParse()` (panic-recovering parse wrapper), `collectDedupedTabs()` (URL-keyed dedup with filters), `runMergeDedup()` (orchestrator).
- `main_test.go` — unit tests for `buildResult()`. Tests tab history preservation and edge cases.
- `merge_dedup_test.go` — tests for merge-dedup logic: file discovery (sorted mtime, empty dir, no-match filters), collectDedupedTabs (empty-URL/extension/about:blank filtering, dedup last-wins, URL sorting, nil-History normalization, multiple windows, empty/nil inputs, all-filtered edge case), end-to-end SNSS file generation + runMergeDedup (basic, cross-file dedup with timestamps, extension filtering, stdout capture).
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
- `findSessionFiles(root string) []sessionFile` — recursively collects all `Session_*` and `*.bin` files under root, sorted by mtime ascending (oldest-first so last-seen-wins for dedup).
- `safeParse(path string) (*Result, error)` — wraps `parse()` with panic recovery so corrupt session files don't abort batch.
- `collectDedupedTabs(results []*Result) []*SlimTab` — iterates parsed results, deduplicates by URL (later results overwrite earlier), filters out empty/chrome-extension/about:blank URLs, returns tabs sorted by URL.
- `runMergeDedup(dir string, outputPath string)` — orchestrator: discovers session files, parses each (skipping corrupt ones with stderr warning), deduplicates, writes JSONL to stdout or file.

**Merge-dedup types:**
- `SlimTab` — slimmed-down tab for JSONL output: `Url`, `Title`, `History[]` (no window/group metadata).
- `sessionFile` — pairs a discovered file path with its modification time for mtime-sorted processing.

**Output types:**
- `Result.Windows` — list of windows, each with `Tabs`, `TabGroups`, `Active`, `Deleted`, `UserTitle`
- `Tab` — `Url`, `Title`, `History[]`, `Active`, `Deleted`, `Group`, `GroupColor`, `GroupCollapsed`
- `SlimTab` — `Url`, `Title`, `History[]` (merge-dedup output, no window/group metadata)
- All history items are preserved (no truncation at current navigation index).

### Merge-dedup mode

Invoked via `-merge-dedup <dir>`. Scans directory recursively for `Session_*` and `*.bin` files, parses all valid ones (skipping corrupt files with `SKIP` stderr warnings), deduplicates by URL (last-seen-file by mtime wins), filters out `chrome-extension://*` and `about:blank`, and outputs deduped tabs as JSONL sorted alphabetically by URL. Use `-output <file>` to write to a file instead of stdout.

## Conventions

- Errors: `panic()` on any I/O or parse failure. Fail fast, full stack trace.
- No external dependencies beyond stdlib.
- The `tab`/`window`/`group` lowercase types are internal (SNSS-adjacent). The `Tab`/`Window`/`TabGroup`/`Result`/`HistoryItem` uppercase types are public output structs.
