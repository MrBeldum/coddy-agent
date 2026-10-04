package session

import (
	"os"
	"path/filepath"
	"strings"
)

// ResolveInstructionFile resolves one instructions.files entry to an absolute
// path. ${CODDY_HOME} and ${CWD} expand, a leading ~ expands to the user's home
// directory, an absolute entry is taken as it stands, and a relative one is
// anchored at the session workspace. An entry that cannot be resolved - a
// ${CODDY_HOME} reference without a home, a relative entry without a cwd -
// returns "" rather than a path assembled out of a literal placeholder.
func ResolveInstructionFile(entry, cwd, home string) string {
	path := strings.TrimSpace(entry)
	if path == "" {
		return ""
	}
	if strings.Contains(path, "${CODDY_HOME}") {
		if strings.TrimSpace(home) == "" {
			return ""
		}
		path = strings.ReplaceAll(path, "${CODDY_HOME}", home)
	}
	if strings.Contains(path, "${CWD}") {
		if strings.TrimSpace(cwd) == "" {
			return ""
		}
		path = strings.ReplaceAll(path, "${CWD}", cwd)
	}
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		path = filepath.Join(userHome, strings.TrimLeft(strings.TrimPrefix(path, "~"), `/\`))
	}
	if !filepath.IsAbs(path) {
		if strings.TrimSpace(cwd) == "" {
			return ""
		}
		path = filepath.Join(cwd, path)
	}
	return filepath.Clean(path)
}

// ResolveInstructionFiles resolves the entries of instructions.files, in their
// order, for rules.LoadStanding: the files the operator added below the
// AGENTS.md and DESIGN.md layers. An entry that cannot be resolved is left
// out.
func ResolveInstructionFiles(entries []string, cwd, home string) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if path := ResolveInstructionFile(entry, cwd, home); path != "" {
			out = append(out, path)
		}
	}
	return out
}
