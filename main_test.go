package main

import (
	"sort"
	"testing"
)

// assignTabsToWindows replicates the tab→window assignment done in parse() before buildResult.
func assignTabsToWindows(tabs map[uint32]*tab, windows map[uint32]*window) {
	for _, t := range tabs {
		sort.Slice(t.history, func(i, j int) bool {
			return t.history[i].idx < t.history[j].idx
		})
		w, ok := windows[t.win]
		if !ok {
			w = &window{id: t.win}
			windows[t.win] = w
		}
		w.tabs = append(w.tabs, t)
	}
	for _, w := range windows {
		sort.Slice(w.tabs, func(i, j int) bool {
			return w.tabs[i].idx < w.tabs[j].idx
		})
	}
}

func TestBuildResultIncludesAllHistory(t *testing.T) {
	// Simulate a tab where user visited A→B→C→D then navigated BACK to B.
	// currentHistoryIdx=1 means B is the active page, but C and D
	// are forward-history items that should still appear in the output.
	tabs := map[uint32]*tab{
		1: {
			id: 1,
			history: []*histItem{
				{idx: 0, url: "https://a.com", title: "Page A"},
				{idx: 1, url: "https://b.com", title: "Page B"},
				{idx: 2, url: "https://c.com", title: "Page C"},
				{idx: 3, url: "https://d.com", title: "Page D"},
			},
			currentHistoryIdx: 1, // user navigated back to B
			idx:               0,
			win:               1,
		},
	}
	windows := map[uint32]*window{
		1: {id: 1, activeTabIdx: 0},
	}
	assignTabsToWindows(tabs, windows)

	result := buildResult(tabs, windows, nil)

	if len(result.Windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(result.Windows))
	}

	win := result.Windows[0]
	if len(win.Tabs) != 1 {
		t.Fatalf("expected 1 tab, got %d", len(win.Tabs))
	}

	tab := win.Tabs[0]

	// ALL history items should be present (including forward-history C and D)
	if len(tab.History) != 4 {
		t.Fatalf("expected 4 history items (including forward-history after currentIdx), got %d", len(tab.History))
	}

	// Url/Title should reflect the CURRENT position (idx=1, Page B)
	if tab.Url != "https://b.com" {
		t.Errorf("expected Url to be https://b.com, got %s", tab.Url)
	}
	if tab.Title != "Page B" {
		t.Errorf("expected Title to be 'Page B', got '%s'", tab.Title)
	}

	// Verify all items are correct
	expected := []struct {
		url   string
		title string
	}{
		{"https://a.com", "Page A"},
		{"https://b.com", "Page B"},
		{"https://c.com", "Page C"},
		{"https://d.com", "Page D"},
	}
	for i, e := range expected {
		if tab.History[i].Url != e.url {
			t.Errorf("history[%d].Url = %q, want %q", i, tab.History[i].Url, e.url)
		}
		if tab.History[i].Title != e.title {
			t.Errorf("history[%d].Title = %q, want %q", i, tab.History[i].Title, e.title)
		}
	}
}

func TestBuildResultWithCurrentIdxAtEnd(t *testing.T) {
	// Tab where currentHistoryIdx is the LAST item — should include all history.
	tabs2 := map[uint32]*tab{
		1: {
			id: 1,
			history: []*histItem{
				{idx: 0, url: "https://a.com", title: "Page A"},
				{idx: 1, url: "https://b.com", title: "Page B"},
			},
			currentHistoryIdx: 1,
			idx:               0,
			win:               1,
		},
	}
	windows2 := map[uint32]*window{
		1: {id: 1, activeTabIdx: 0},
	}
	assignTabsToWindows(tabs2, windows2)

	result := buildResult(tabs2, windows2, nil)
	tab := result.Windows[0].Tabs[0]

	if len(tab.History) != 2 {
		t.Fatalf("expected 2 history items, got %d", len(tab.History))
	}
	if tab.Url != "https://b.com" {
		t.Errorf("expected Url https://b.com, got %s", tab.Url)
	}
}

func TestBuildResultWithCurrentIdxBeyondHistory(t *testing.T) {
	// currentHistoryIdx beyond any item — should include all history,
	// and Url/Title should be empty (no item matched).
	tabs3 := map[uint32]*tab{
		1: {
			id: 1,
			history: []*histItem{
				{idx: 0, url: "https://a.com", title: "Page A"},
				{idx: 1, url: "https://b.com", title: "Page B"},
			},
			currentHistoryIdx: 99,
			idx:               0,
			win:               1,
		},
	}
	windows3 := map[uint32]*window{
		1: {id: 1, activeTabIdx: 0},
	}
	assignTabsToWindows(tabs3, windows3)

	result := buildResult(tabs3, windows3, nil)
	tab := result.Windows[0].Tabs[0]

	if len(tab.History) != 2 {
		t.Fatalf("expected 2 history items, got %d", len(tab.History))
	}
	// Url/Title not set because no item matched currentHistoryIdx
	if tab.Url != "" {
		t.Errorf("expected empty Url when currentHistoryIdx beyond history, got %s", tab.Url)
	}
	if tab.Title != "" {
		t.Errorf("expected empty Title when currentHistoryIdx beyond history, got '%s'", tab.Title)
	}
}

func TestBuildResultHandlesEmptyHistory(t *testing.T) {
	// Tab with no history at all — should not crash, history should be nil/empty.
	tabs4 := map[uint32]*tab{
		1: {id: 1, idx: 0, win: 1, history: nil, currentHistoryIdx: 0},
	}
	windows4 := map[uint32]*window{
		1: {id: 1, activeTabIdx: 0},
	}
	assignTabsToWindows(tabs4, windows4)

	result := buildResult(tabs4, windows4, nil)
	tab := result.Windows[0].Tabs[0]
	if tab.History == nil {
		t.Log("history is nil (acceptable for empty tab)")
	}
	if len(tab.History) != 0 {
		t.Errorf("expected empty history, got %d items", len(tab.History))
	}
}
