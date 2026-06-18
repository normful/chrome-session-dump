package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SlimTab is the slimmed-down tab representation for JSONL output.
type SlimTab struct {
	Url     string         `json:"url"`
	Title   string         `json:"title"`
	History []*HistoryItem `json:"history"`
}

// sessionFile pairs a discovered session file with its modification time
// so we can sort oldest-first (last-seen-wins for dedup).
type sessionFile struct {
	path  string
	mtime time.Time
}

// findSessionFiles recursively collects all *.bin and Session_* files under root,
// sorted by mtime ascending (oldest first).
func findSessionFiles(root string) []sessionFile {
	var files []sessionFile

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible entries
		}
		if info.IsDir() {
			return nil
		}
		name := info.Name()
		if strings.HasSuffix(name, ".bin") || strings.HasPrefix(name, "Session_") {
			files = append(files, sessionFile{path, info.ModTime()})
		}
		return nil
	})

	sort.Slice(files, func(i, j int) bool {
		return files[i].mtime.Before(files[j].mtime)
	})

	return files
}

// safeParse wraps parse() with panic recovery so a corrupt session file
// doesn't abort the entire batch.
func safeParse(path string) (result *Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("parse %s: %v", path, r)
		}
	}()
	r := parse(path)
	return &r, nil
}

// collectDedupedTabs processes parsed Results, deduplicating by URL,
// filtering out empty/extension/blank URLs, and returning tabs sorted by URL.
// Later results in the slice overwrite earlier ones for the same URL
// (caller should pass files in mtime order so last-seen-timestamp wins).
func collectDedupedTabs(results []*Result) []*SlimTab {
	tabsByURL := make(map[string]*SlimTab)
	for _, result := range results {
		for _, w := range result.Windows {
			for _, t := range w.Tabs {
				if t.Url == "" || strings.HasPrefix(t.Url, "chrome-extension://") || strings.HasPrefix(t.Url, "about:blank") {
					continue
				}
				history := t.History
				if history == nil {
					history = []*HistoryItem{}
				}
				tabsByURL[t.Url] = &SlimTab{
					Url:     t.Url,
					Title:   t.Title,
					History: history,
				}
			}
		}
	}

	sorted := make([]*SlimTab, 0, len(tabsByURL))
	for _, st := range tabsByURL {
		sorted = append(sorted, st)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Url < sorted[j].Url
	})
	return sorted
}

func runMergeDedup(dir string, outputPath string) {
	files := findSessionFiles(dir)
	if len(files) == 0 {
		panic(fmt.Errorf("no session files found under %s", dir))
	}

	var results []*Result
	for _, sf := range files {
		result, err := safeParse(sf.path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "SKIP %s: %v\n", sf.path, err)
			continue
		}
		results = append(results, result)
	}

	sorted := collectDedupedTabs(results)
	if len(sorted) == 0 {
		panic(fmt.Errorf("no tabs found in session files under %s", dir))
	}

	// Write output
	w := os.Stdout
	if outputPath != "" {
		f, err := os.Create(outputPath)
		if err != nil {
			panic(err)
		}
		defer f.Close()
		w = f
	}

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, st := range sorted {
		if err := enc.Encode(st); err != nil {
			panic(err)
		}
	}
}
