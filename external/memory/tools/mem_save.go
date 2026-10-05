//go:build memory

package memtools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	memstorage "github.com/EvilFreelancer/coddy-agent/external/memory/storage"
	"github.com/EvilFreelancer/coddy-agent/internal/llm"
	"github.com/EvilFreelancer/coddy-agent/internal/tooling"
)

// maxNoteBodyRunes caps a saved note body. It counts characters (runes), not
// bytes, so non-Latin text such as Cyrillic gets the same budget as ASCII and
// the cut never lands inside a multi-byte character.
const maxNoteBodyRunes = 900

// truncateNoteBody trims body to maxNoteBodyRunes characters and appends a
// "..." marker line. It reports whether anything was dropped.
func truncateNoteBody(body string) (string, bool) {
	if utf8.RuneCountInString(body) <= maxNoteBodyRunes {
		return body, false
	}
	r := []rune(body)
	return string(r[:maxNoteBodyRunes]) + "\n...", true
}

func memorySaveTool(store *memstorage.Store) *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name:        NameSave,
			Description: "Write or overwrite a distilled memory note. Prefer relative_path with folders for reusable organization.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"title": map[string]interface{}{"type": "string", "description": "Short title; used for default flat filename when relative_path is omitted"},
					"body":  map[string]interface{}{"type": "string", "description": fmt.Sprintf("Markdown or plain text body to store, at most %d characters; a longer body is truncated", maxNoteBodyRunes)},
					"scope": map[string]interface{}{"type": "string", "enum": []interface{}{"global", "project"}},
					"relative_path": map[string]interface{}{
						"type":        "string",
						"description": "Optional path under scope root with .md or .txt extension, e.g. design/auth-flow.md. When omitted, a slug from title is written at scope root.",
					},
				},
				"required": []interface{}{"title", "body", "scope"},
			},
		},
		Execute: func(ctx context.Context, argsJSON string, _ *tooling.Env) (string, error) {
			_ = ctx
			var args struct {
				Title        string `json:"title"`
				Body         string `json:"body"`
				Scope        string `json:"scope"`
				RelativePath string `json:"relative_path"`
			}
			if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
				return "", err
			}
			body, truncated := truncateNoteBody(strings.TrimSpace(args.Body))
			rel := strings.TrimSpace(args.RelativePath)
			p, err := store.WriteFlexible(args.Scope, args.Title, rel, body)
			if err != nil {
				return "", err
			}
			if truncated {
				return fmt.Sprintf("saved as %s (warning: body truncated to %d characters)", p, maxNoteBodyRunes), nil
			}
			return "saved as " + p, nil
		},
	}
}
