package config

import (
	"fmt"
	"log/slog"
	"os"
	"sort"
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

// legacyMove is one key that left config.yaml: where it sits in the file and
// how its value is carried into the file that holds it now. move reports
// what it moved and what the target had already, for the log line.
type legacyMove struct {
	path  string
	key   *yaml.Node
	value *yaml.Node
	move  func(value *yaml.Node) (moved, kept []string, err error)
}

// migrateLegacyKeys moves the keys that left config.yaml: the old top-level
// mcp_servers into <home>/mcp.json, every value meaning what it meant in
// config.yaml (legacyMCPValue: an environment reference stays one, which
// mcp.json resolves when the server starts), a name the file declares
// already left alone; and skills.sources into the sources of
// <home>/marketplaces.json, a source the file has already (in any case) and
// the system source not repeated. A move that cannot write its target leaves
// its key where it is; the others go ahead, in one rewrite of config.yaml
// with one backup of the old file.
func migrateLegacyKeys(paths Paths, data []byte) []byte {
	if strings.TrimSpace(paths.ConfigPath) == "" {
		return data
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return data
	}
	root := doc.Content[0]
	var moves []legacyMove
	if k, v := mappingEntry(root, "mcp_servers"); k != nil {
		moves = append(moves, legacyMove{path: "mcp_servers", key: k, value: v,
			move: func(v *yaml.Node) ([]string, []string, error) { return moveLegacyMCPServers(paths, v) }})
	}
	if _, skills := mappingEntry(root, "skills"); skills != nil && skills.Kind == yaml.MappingNode {
		if k, v := mappingEntry(skills, "sources"); k != nil {
			moves = append(moves, legacyMove{path: "skills.sources", key: k, value: v,
				move: func(v *yaml.Node) ([]string, []string, error) { return moveLegacySkillSources(paths, v) }})
		}
	}
	if len(moves) == 0 {
		return data
	}
	if !configPathWriteMu.TryLock() {
		return data
	}
	defer configPathWriteMu.Unlock()

	log := slog.Default()
	var done []legacyMove
	for _, m := range moves {
		moved, kept, err := m.move(m.value)
		if err != nil {
			log.Warn("config: a key that left config.yaml could not be moved; left in place and not used", "key", m.path, "path", paths.ConfigPath, "error", err)
			continue
		}
		log.Info("config: moved a key out of config.yaml", "key", m.path, "moved", moved, "already_there", kept, "config", paths.ConfigPath)
		done = append(done, m)
	}
	if len(done) == 0 {
		return data
	}
	// Cut from the bottom of the file up, so a cut never moves the lines of
	// a key still to be cut.
	sort.Slice(done, func(i, j int) bool { return done[i].key.Line > done[j].key.Line })
	next := data
	for _, m := range done {
		next = removeYAMLKeyBlock(next, m.key)
	}
	if err := rewriteConfigKeepingBackup(paths.ConfigPath, data, next); err != nil {
		log.Warn("config: keys moved but config.yaml could not be rewritten; they are ignored", "path", paths.ConfigPath, "error", err)
		return data
	}
	return next
}

// mappingEntry finds key in a mapping node, returning its key and value nodes.
func mappingEntry(m *yaml.Node, key string) (*yaml.Node, *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i], m.Content[i+1]
		}
	}
	return nil, nil
}

// moveLegacyMCPServers writes the servers of an old mcp_servers list into
// <home>/mcp.json, leaving a name the file declares already alone.
func moveLegacyMCPServers(paths Paths, value *yaml.Node) (moved, kept []string, err error) {
	var servers []MCPServerConfig
	if err := value.Decode(&servers); err != nil {
		return nil, nil, fmt.Errorf("mcp_servers does not read as a list of servers: %w", err)
	}
	if len(servers) == 0 {
		return nil, nil, nil
	}
	target := GlobalMCPJSONPath(paths.Home)
	entries, err := ReadMCPJSONFile(target)
	if err != nil {
		return nil, nil, err
	}
	for _, srv := range servers {
		name := strings.TrimSpace(srv.Name)
		if name == "" {
			continue
		}
		if _, ok := entries[name]; ok {
			kept = append(kept, name)
			continue
		}
		entries[name] = legacyMCPServerToJSON(srv, paths.Home)
		moved = append(moved, name)
	}
	if len(moved) > 0 {
		if err := writeMCPJSONFileEntries(target, entries); err != nil {
			return nil, nil, err
		}
	}
	return moved, kept, nil
}

// moveLegacySkillSources appends the sources of an old skills.sources list to
// the sources of <home>/marketplaces.json. A source the file has already, in
// any case, and the system source, which is in effect without any file, are
// not repeated.
func moveLegacySkillSources(paths Paths, value *yaml.Node) (moved, kept []string, err error) {
	var sources []string
	if err := value.Decode(&sources); err != nil {
		return nil, nil, fmt.Errorf("skills.sources does not read as a list of sources: %w", err)
	}
	target := GlobalMarketplacesPath(paths.Home)
	file, err := ReadMarketplacesFile(target)
	if err != nil {
		return nil, nil, err
	}
	known := map[string]bool{strings.ToLower(SystemSkillsSource): true}
	for _, s := range file.Sources {
		known[strings.ToLower(strings.TrimSpace(s))] = true
	}
	for _, s := range sources {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if known[strings.ToLower(s)] {
			kept = append(kept, s)
			continue
		}
		known[strings.ToLower(s)] = true
		file.Sources = append(file.Sources, s)
		moved = append(moved, s)
	}
	if len(moved) > 0 {
		if err := WriteMarketplacesFile(target, file); err != nil {
			return nil, nil, err
		}
	}
	return moved, kept, nil
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
