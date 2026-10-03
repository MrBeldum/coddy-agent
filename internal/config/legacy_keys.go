package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Keys that left config.yaml move on load into the files that hold them now,
// so an old config keeps working after an update without anyone editing it.
// The move runs under the config file lock and only when it can take it: a
// load inside a config transaction (which holds the lock) leaves it for the
// next load. The key's block is cut out of the text; the rest of the file
// stays byte for byte, and the old file is kept beside it as
// <config>.bak-<time>.

// migrateLegacyKeys moves the old top-level mcp_servers key: its servers go
// into <home>/mcp.json with every value meaning what it meant in config.yaml
// (legacyMCPValue: an environment reference stays one, which mcp.json
// resolves when the server starts), and a name the file already declares is
// left alone.
func migrateLegacyKeys(paths Paths, data []byte) []byte {
	if strings.TrimSpace(paths.ConfigPath) == "" {
		return data
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return data
	}
	root := doc.Content[0]
	keyIdx := -1
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "mcp_servers" {
			keyIdx = i
			break
		}
	}
	if keyIdx < 0 {
		return data
	}
	if !configPathWriteMu.TryLock() {
		return data
	}
	defer configPathWriteMu.Unlock()

	log := slog.Default()
	var servers []MCPServerConfig
	if err := root.Content[keyIdx+1].Decode(&servers); err != nil {
		log.Warn("config: the old mcp_servers key does not read as a list of servers; left in place", "path", paths.ConfigPath, "error", err)
		return data
	}
	var moved, skipped []string
	if len(servers) > 0 {
		target := GlobalMCPJSONPath(paths.Home)
		entries, err := ReadMCPJSONFile(target)
		if err != nil {
			log.Warn("config: mcp_servers not moved, the target mcp.json does not read", "target", target, "error", err)
			return data
		}
		for _, srv := range servers {
			name := strings.TrimSpace(srv.Name)
			if name == "" {
				continue
			}
			if _, ok := entries[name]; ok {
				skipped = append(skipped, name)
				continue
			}
			entries[name] = legacyMCPServerToJSON(srv, paths.Home)
			moved = append(moved, name)
		}
		if len(moved) > 0 {
			if err := writeMCPJSONFileEntries(target, entries); err != nil {
				log.Warn("config: mcp_servers not moved, mcp.json could not be written", "target", target, "error", err)
				return data
			}
		}
	}
	next := removeYAMLKeyBlock(data, root.Content[keyIdx])
	if err := rewriteConfigKeepingBackup(paths.ConfigPath, data, next); err != nil {
		log.Warn("config: mcp_servers moved but config.yaml could not be rewritten; the key is ignored", "path", paths.ConfigPath, "error", err)
		return data
	}
	log.Info("config: moved mcp_servers out of config.yaml",
		"into", GlobalMCPJSONPath(paths.Home), "moved", moved, "already_there", skipped, "config", paths.ConfigPath)
	return next
}

// legacyMCPServerToJSON is the mcp.json entry of a declaration read from the
// raw text of config.yaml, every value carried over by legacyMCPValue.
func legacyMCPServerToJSON(s MCPServerConfig, home string) MCPJSONServer {
	value := func(v string) string { return legacyMCPValue(v, home) }
	s.Command, s.URL = value(s.Command), value(s.URL)
	s.Args = append([]string(nil), s.Args...)
	for i := range s.Args {
		s.Args[i] = value(s.Args[i])
	}
	s.Env = append([]EnvVarConfig(nil), s.Env...)
	for i := range s.Env {
		s.Env[i].Value = value(s.Env[i].Value)
	}
	s.Headers = append([]HTTPHeaderConfig(nil), s.Headers...)
	for i := range s.Headers {
		s.Headers[i].Value = value(s.Headers[i].Value)
	}
	return MCPJSONFromServer(s)
}

// legacyMCPValue rewrites one value as config.yaml wrote it into the
// mcp.json spelling that starts the server with the same value
// (ExpandMCPValue): config.yaml expanded $NAME and ${NAME} from the
// environment when it loaded, read "$$" as a literal "$" and put the home in
// for ${CODDY_HOME}. A reference stays a reference, so a secret kept in the
// environment is not written into the file; the home is written out.
func legacyMCPValue(raw, home string) string {
	s := strings.ReplaceAll(raw, "$$", escapedDollarSentinel)
	s = strings.ReplaceAll(s, "${CODDY_HOME}", yamlSafePath(home))
	s = strings.ReplaceAll(s, sessionCWDPlaceholder, sessionCWDSentinel)
	s = os.Expand(s, func(name string) string { return "${" + name + "}" })
	s = strings.ReplaceAll(s, sessionCWDSentinel, sessionCWDPlaceholder)
	// A literal "$" before a brace would read as a reference in mcp.json,
	// where "$${" is the escape for it.
	s = strings.ReplaceAll(s, escapedDollarSentinel+"{", "$${")
	return strings.ReplaceAll(s, escapedDollarSentinel, "$")
}

// rewriteConfigKeepingBackup writes next over the config, keeping the old
// bytes as <config>.bak-<time> with the config's own permissions.
func rewriteConfigKeepingBackup(path string, old, next []byte) error {
	perm := os.FileMode(0o600)
	if st, err := os.Stat(path); err == nil {
		perm = st.Mode().Perm()
	}
	backup := fmt.Sprintf("%s.bak-%s", path, time.Now().Format("20060102-150405.000000000"))
	if err := os.WriteFile(backup, old, perm); err != nil {
		return fmt.Errorf("backup %s: %w", backup, err)
	}
	return atomicWriteFile(path, next, perm)
}

// removeYAMLKeyBlock cuts one mapping key out of raw YAML text: the key's
// line, every line of its value (deeper indented, blank, or a sequence item
// at the key's own indentation), and the comment lines right above the key at
// its indentation (its description; not the schema modeline). Blank lines
// that end the block stay, so the next section keeps its separation.
func removeYAMLKeyBlock(raw []byte, key *yaml.Node) []byte {
	lines := strings.SplitAfter(string(raw), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	start := key.Line - 1
	col := key.Column - 1
	if start < 0 || start >= len(lines) {
		return raw
	}
	end := start + 1
	for end < len(lines) {
		line := strings.TrimRight(lines[end], "\r\n")
		if strings.TrimSpace(line) == "" {
			end++
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		rest := line[indent:]
		if indent > col || (indent == col && (rest == "-" || strings.HasPrefix(rest, "- "))) {
			end++
			continue
		}
		break
	}
	for end > start+1 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	for start > 0 {
		prev := strings.TrimRight(lines[start-1], "\r\n")
		indent := len(prev) - len(strings.TrimLeft(prev, " "))
		rest := prev[indent:]
		if indent != col || !strings.HasPrefix(rest, "#") || strings.Contains(rest, "yaml-language-server") {
			break
		}
		start--
	}
	// A blank line on both sides of the cut would leave two in a row: the
	// sections around it keep one between them, and the file does not end on
	// a blank line it did not end on before.
	blank := func(i int) bool { return strings.TrimSpace(lines[i]) == "" }
	if start > 0 && blank(start-1) && (end == len(lines) || blank(end)) {
		start--
	}
	return []byte(strings.Join(lines[:start], "") + strings.Join(lines[end:], ""))
}
