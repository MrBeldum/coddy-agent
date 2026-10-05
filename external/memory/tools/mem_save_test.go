//go:build memory

package memtools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func saveNote(t *testing.T, body string) (string, string) {
	t.Helper()
	st, _ := testMemoryStore(t)
	args, err := json.Marshal(map[string]string{
		"title":         "note",
		"body":          body,
		"scope":         "project",
		"relative_path": "notes/note.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := memorySaveTool(st).Execute(context.Background(), string(args), nil)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(st.ProjectRoot(), "notes", "note.md"))
	if err != nil {
		t.Fatalf("read saved note: %v", err)
	}
	return out, string(data)
}

func TestMemorySaveKeepsBodyWithinLimit(t *testing.T) {
	// 900 two-byte Cyrillic characters: 1800 bytes, but within the character limit.
	body := strings.Repeat("ж", maxNoteBodyRunes)
	out, got := saveNote(t, body)
	if strings.Contains(out, "truncated") {
		t.Fatalf("unexpected truncation warning: %q", out)
	}
	if got != body+"\n" {
		t.Fatalf("body changed: got %d runes, want %d", utf8.RuneCountInString(got), maxNoteBodyRunes+1)
	}
}

func TestMemorySaveTruncatesByRunesAndWarns(t *testing.T) {
	// The cut falls right where a byte-based slice would split a character.
	body := "a" + strings.Repeat("ж", maxNoteBodyRunes+50)
	out, got := saveNote(t, body)
	if !strings.Contains(out, "truncated") {
		t.Fatalf("expected truncation warning, got %q", out)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("saved note is not valid UTF-8")
	}
	if !strings.HasSuffix(got, "\n...\n") {
		t.Fatalf("missing truncation marker: %q", got[len(got)-10:])
	}
	kept := strings.TrimSuffix(got, "\n...\n")
	if n := utf8.RuneCountInString(kept); n != maxNoteBodyRunes {
		t.Fatalf("kept %d runes, want %d", n, maxNoteBodyRunes)
	}
}
