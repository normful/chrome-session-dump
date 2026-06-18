package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

// ── SNSS file generation helpers ──────────────────────────────────────────

func leUint16(v uint16) []byte { return []byte{byte(v), byte(v >> 8)} }
func leUint32(v uint32) []byte { return []byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)} }

// pickledString matches readString: uint32(len) + bytes + padded to 4 bytes.
func pickledString(s string) []byte {
	b := []byte(s)
	sz := len(b)
	padded := sz
	if pad := padded % 4; pad != 0 {
		padded += 4 - pad
	}
	buf := make([]byte, 4+padded)
	binary.LittleEndian.PutUint32(buf[:4], uint32(sz))
	copy(buf[4:], b)
	return buf
}

// pickledString16 matches readString16: uint32(len) + UTF-16 BE per code-unit + padded to 4 bytes.
func pickledString16(s string) []byte {
	runes := utf16.Encode([]rune(s))
	sz := len(runes)
	dataLen := sz * 2
	padded := dataLen
	if pad := padded % 4; pad != 0 {
		padded += 4 - pad
	}
	buf := make([]byte, 4+padded)
	binary.LittleEndian.PutUint32(buf[:4], uint32(sz))
	for i, r := range runes {
		buf[4+i*2] = byte(r)        // low byte  (b[i]   in readString16)
		buf[4+i*2+1] = byte(r >> 8) // high byte (b[i+1] in readString16)
	}
	return buf
}

// snssCommand builds a single SNSS command: int16(1+len(payload)) int8(type) payload.
func snssCommand(typ uint8, payload []byte) []byte {
	var b bytes.Buffer
	b.Write(leUint16(uint16(1 + len(payload))))
	b.WriteByte(typ)
	b.Write(payload)
	return b.Bytes()
}

// snssCommandTabNav builds a kCommandUpdateTabNavigation command.
// The pickled payload starts with a "size again" uint32 covering the rest.
func snssCommandTabNav(id, histIdx uint32, url, title string) []byte {
	urlB := pickledString(url)
	titleB := pickledString16(title)

	var inner bytes.Buffer
	inner.Write(leUint32(id))
	inner.Write(leUint32(histIdx))
	inner.Write(urlB)
	inner.Write(titleB)

	var payload bytes.Buffer
	payload.Write(leUint32(uint32(inner.Len()))) // "size again"
	payload.Write(inner.Bytes())

	return snssCommand(kCommandUpdateTabNavigation, payload.Bytes())
}

// writeSnssFile creates a minimal valid SNSS file with the given named-tabs
// all in window 1. Each tab gets one history entry at index 0.
func writeSnssFile(t *testing.T, path string, urls, titles []string) {
	t.Helper()

	var b bytes.Buffer
	b.WriteString("SNSS")
	b.Write(leUint32(1)) // version

	for i, url := range urls {
		id := uint32(i + 1)
		title := ""
		if i < len(titles) {
			title = titles[i]
		}

		// kCommandSetTabWindow (0): win=1, id=tabID
		payload := append(leUint32(1), leUint32(id)...)
		b.Write(snssCommand(kCommandSetTabWindow, payload))

		// kCommandUpdateTabNavigation (6)
		b.Write(snssCommandTabNav(id, 0, url, title))
	}

	if err := os.WriteFile(path, b.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
}

// ── findSessionFiles tests ────────────────────────────────────────────────

func TestFindSessionFiles_discovery(t *testing.T) {
	dir := t.TempDir()

	makeFile := func(name string, mtime time.Time) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, nil, 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}

	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)

	makeFile("Session_AAA", t3)     // matched
	makeFile("Session_BBB", t1)     // matched
	makeFile("data.bin", t2)        // matched
	makeFile("other.txt", t2)       // NOT matched
	makeFile("Session_", t2)        // matched (starts with Session_)
	makeFile("notsession.bin2", t3) // NOT matched (wrong suffix)
	// Nested file
	nested := filepath.Join(dir, "subdir")
	os.MkdirAll(nested, 0755)
	makeFile(filepath.Join("subdir", "Session_CCC"), t2) // matched (recursive)

	got := findSessionFiles(dir)

	// We should get: Session_BBB (t1), data.bin (t2), Session_CCC (t2), Session_AAA (t3), Session_ (t2)
	// Sort by mtime: t1 < t2 < t2 < t2 < t3
	// Within same mtime the relative order isn't guaranteed by the function,
	// but we can verify all expected files are present and sorted by mtime.
	if len(got) != 5 {
		t.Fatalf("expected 5 session files, got %d: %v", len(got), fileNames(got))
	}

	// First file should be oldest (t1)
	if base := filepath.Base(got[0].path); base != "Session_BBB" {
		t.Errorf("oldest file should be Session_BBB (t1), got %s", base)
	}
	// Last file should be newest (t3)
	if base := filepath.Base(got[len(got)-1].path); base != "Session_AAA" {
		t.Errorf("newest file should be Session_AAA (t3), got %s", base)
	}
}

func TestFindSessionFiles_empty(t *testing.T) {
	dir := t.TempDir()
	got := findSessionFiles(dir)
	if len(got) != 0 {
		t.Errorf("expected no files in empty dir, got %d", len(got))
	}
}

func TestFindSessionFiles_noMatches(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "readme.txt"), nil, 0644)
	os.WriteFile(filepath.Join(dir, "data.json"), nil, 0644)

	got := findSessionFiles(dir)
	if len(got) != 0 {
		t.Errorf("expected no matches, got %d", len(got))
	}
}

// ── collectDedupedTabs tests ──────────────────────────────────────────────

func TestCollectDedupedTabs_basic(t *testing.T) {
	r := &Result{Windows: []*Window{{Tabs: []*Tab{
		{Url: "https://a.com", Title: "Page A", History: []*HistoryItem{{Url: "https://a.com", Title: "Page A"}}},
	}}}}
	sorted := collectDedupedTabs([]*Result{r})
	if len(sorted) != 1 || sorted[0].Url != "https://a.com" {
		t.Fatalf("got %d tabs, first Url=%s", len(sorted), sorted[0].Url)
	}
}

func TestCollectDedupedTabs_filtersEmptyURL(t *testing.T) {
	r := &Result{Windows: []*Window{{Tabs: []*Tab{
		{Url: "", Title: "no url"},
		{Url: "https://keep.com", Title: "Keep", History: []*HistoryItem{{Url: "https://keep.com", Title: "Keep"}}},
	}}}}
	sorted := collectDedupedTabs([]*Result{r})
	if len(sorted) != 1 {
		t.Fatalf("expected 1 (empty filtered), got %d", len(sorted))
	}
}

func TestCollectDedupedTabs_filtersChromeExtension(t *testing.T) {
	r := &Result{Windows: []*Window{{Tabs: []*Tab{
		{Url: "chrome-extension://abc/options.html", Title: "Ext"},
		{Url: "https://keep.com", Title: "Keep", History: []*HistoryItem{{Url: "https://keep.com", Title: "Keep"}}},
	}}}}
	sorted := collectDedupedTabs([]*Result{r})
	if len(sorted) != 1 {
		t.Fatalf("expected 1 (extension filtered), got %d", len(sorted))
	}
}

func TestCollectDedupedTabs_filtersAboutBlank(t *testing.T) {
	r := &Result{Windows: []*Window{{Tabs: []*Tab{
		{Url: "about:blank", Title: ""},
		{Url: "https://keep.com", Title: "Keep", History: []*HistoryItem{{Url: "https://keep.com", Title: "Keep"}}},
	}}}}
	sorted := collectDedupedTabs([]*Result{r})
	if len(sorted) != 1 {
		t.Fatalf("expected 1 (about:blank filtered), got %d", len(sorted))
	}
}

func TestCollectDedupedTabs_dedupByURL(t *testing.T) {
	r1 := &Result{Windows: []*Window{{Tabs: []*Tab{
		{Url: "https://dup.com", Title: "Old", History: []*HistoryItem{{Url: "https://dup.com", Title: "Old"}}},
	}}}}
	r2 := &Result{Windows: []*Window{{Tabs: []*Tab{
		{Url: "https://dup.com", Title: "New", History: []*HistoryItem{{Url: "https://dup.com", Title: "New"}}},
	}}}}

	sorted := collectDedupedTabs([]*Result{r1, r2})
	if len(sorted) != 1 {
		t.Fatalf("expected 1 deduped tab, got %d", len(sorted))
	}
	if sorted[0].Title != "New" {
		t.Errorf("expected 'New' (last-wins), got '%s'", sorted[0].Title)
	}
}

func TestCollectDedupedTabs_sortedByURL(t *testing.T) {
	r := &Result{Windows: []*Window{{Tabs: []*Tab{
		{Url: "https://z.com", Title: "Z", History: []*HistoryItem{{Url: "https://z.com", Title: "Z"}}},
		{Url: "https://a.com", Title: "A", History: []*HistoryItem{{Url: "https://a.com", Title: "A"}}},
		{Url: "https://m.com", Title: "M", History: []*HistoryItem{{Url: "https://m.com", Title: "M"}}},
	}}}}
	sorted := collectDedupedTabs([]*Result{r})
	if len(sorted) != 3 {
		t.Fatalf("expected 3 tabs, got %d", len(sorted))
	}
	got := []string{sorted[0].Url, sorted[1].Url, sorted[2].Url}
	want := []string{"https://a.com", "https://m.com", "https://z.com"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sorted[%d].Url = %s, want %s", i, got[i], want[i])
		}
	}
}

func TestCollectDedupedTabs_nilHistoryNormalized(t *testing.T) {
	r := &Result{Windows: []*Window{{Tabs: []*Tab{
		{Url: "https://a.com", Title: "A", History: nil},
	}}}}
	sorted := collectDedupedTabs([]*Result{r})
	if len(sorted) != 1 {
		t.Fatalf("expected 1 tab, got %d", len(sorted))
	}
	if sorted[0].History == nil {
		t.Error("History should be non-nil empty slice, got nil")
	}
	if len(sorted[0].History) != 0 {
		t.Errorf("expected empty History, got %d items", len(sorted[0].History))
	}
}

func TestCollectDedupedTabs_multipleWindows(t *testing.T) {
	r := &Result{Windows: []*Window{
		{Tabs: []*Tab{{Url: "https://w1.com", Title: "W1", History: []*HistoryItem{{Url: "https://w1.com", Title: "W1"}}}}},
		{Tabs: []*Tab{{Url: "https://w2.com", Title: "W2", History: []*HistoryItem{{Url: "https://w2.com", Title: "W2"}}}}},
	}}
	sorted := collectDedupedTabs([]*Result{r})
	if len(sorted) != 2 {
		t.Fatalf("expected 2 tabs from 2 windows, got %d", len(sorted))
	}
}

func TestCollectDedupedTabs_emptyResults(t *testing.T) {
	sorted := collectDedupedTabs(nil)
	if len(sorted) != 0 {
		t.Errorf("expected empty from nil input, got %d", len(sorted))
	}

	sorted = collectDedupedTabs([]*Result{})
	if len(sorted) != 0 {
		t.Errorf("expected empty from empty slice, got %d", len(sorted))
	}
}

func TestCollectDedupedTabs_allFiltered(t *testing.T) {
	r := &Result{Windows: []*Window{{Tabs: []*Tab{
		{Url: "", Title: "x"},
		{Url: "chrome-extension://x/", Title: "x"},
		{Url: "about:blank", Title: ""},
	}}}}
	sorted := collectDedupedTabs([]*Result{r})
	if len(sorted) != 0 {
		t.Errorf("expected empty when all filtered, got %d", len(sorted))
	}
}

// ── End-to-end: runMergeDedup with generated SNSS files ───────────────────

func TestRunMergeDedup_basic(t *testing.T) {
	resetSessionState()
	dir := t.TempDir()
	out := filepath.Join(t.TempDir(), "out.jsonl")

	writeSnssFile(t, filepath.Join(dir, "Session_001"), []string{"https://b.com", "https://a.com"}, []string{"B", "A"})

	runMergeDedup(dir, out)

	// Read and verify
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 JSONL lines, got %d", len(lines))
	}

	var first, second SlimTab
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatal(err)
	}

	// Should be sorted by URL
	if first.Url != "https://a.com" || first.Title != "A" {
		t.Errorf("first: Url=%s Title=%s, want https://a.com / A", first.Url, first.Title)
	}
	if second.Url != "https://b.com" || second.Title != "B" {
		t.Errorf("second: Url=%s Title=%s, want https://b.com / B", second.Url, second.Title)
	}
	// Each should have 1 history entry
	if len(first.History) != 1 || len(second.History) != 1 {
		t.Errorf("expected 1 history entry per tab, got %d / %d", len(first.History), len(second.History))
	}
}

func TestRunMergeDedup_dedupAcrossFiles(t *testing.T) {
	resetSessionState()
	dir := t.TempDir()
	out := filepath.Join(t.TempDir(), "out.jsonl")

	// Older file — will be overwritten by newer
	oldP := filepath.Join(dir, "Session_Old")
	writeSnssFile(t, oldP, []string{"https://dup.com"}, []string{"Old Title"})
	older := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	os.Chtimes(oldP, older, older)

	// Newer file — same URL, should win
	newP := filepath.Join(dir, "Session_New")
	writeSnssFile(t, newP, []string{"https://dup.com"}, []string{"New Title"})
	newer := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	os.Chtimes(newP, newer, newer)

	runMergeDedup(dir, out)

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var tab SlimTab
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &tab); err != nil {
		t.Fatal(err)
	}
	if tab.Title != "New Title" {
		t.Errorf("expected 'New Title' (newer session wins), got '%s'", tab.Title)
	}
}

func TestRunMergeDedup_filteredExtension(t *testing.T) {
	resetSessionState()
	dir := t.TempDir()
	out := filepath.Join(t.TempDir(), "out.jsonl")

	writeSnssFile(t, filepath.Join(dir, "Session_001"),
		[]string{"chrome-extension://abc/options.html", "https://real.com"},
		[]string{"Ext", "Real"})

	runMergeDedup(dir, out)

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line (extension filtered), got %d: %s", len(lines), data)
	}
	if !strings.Contains(string(data), "https://real.com") {
		t.Errorf("should contain real.com, got: %s", data)
	}
}

func TestRunMergeDedup_stdout(t *testing.T) {
	resetSessionState()
	dir := t.TempDir()
	writeSnssFile(t, filepath.Join(dir, "Session_001"), []string{"https://a.com"}, []string{"A"})

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	done := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 4096)
		n, _ := r.Read(buf)
		done <- buf[:n]
	}()

	runMergeDedup(dir, "")

	w.Close()
	os.Stdout = old

	data := <-done
	var tab SlimTab
	if err := json.Unmarshal(bytes.TrimSpace(data), &tab); err != nil {
		t.Fatal(err)
	}
	if tab.Url != "https://a.com" {
		t.Errorf("expected https://a.com, got %s", tab.Url)
	}
}

// ── Helper ──

// resetSessionState clears the package-level SNSS parse state so each
// end-to-end test starts fresh. Without this, parse() accumulates entries
// across test cases because tabs/windows/groups are package-level maps.
func resetSessionState() {
	tabs = map[uint32]*tab{}
	windows = map[uint32]*window{}
	groups = map[string]*group{}
}

func fileNames(sfs []sessionFile) []string {
	names := make([]string, len(sfs))
	for i, sf := range sfs {
		names[i] = filepath.Base(sf.path)
	}
	return names
}
