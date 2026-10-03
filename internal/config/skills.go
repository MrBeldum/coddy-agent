package config

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
)

// SkillsAutoDiscoveryFlagName is the CLI flag (on `coddy acp` / `coddy serve`)
// that overrides skills.auto_discovery.
const SkillsAutoDiscoveryFlagName = "skills-auto-discovery"

// ApplySkillsAutoDiscoveryFlag overrides skills.auto_discovery only when the
// -skills-auto-discovery flag was explicitly provided on fs; otherwise the config
// value (which defaults to true) is left untouched. Shared by the acp and http
// command entrypoints so both behave identically.
func ApplySkillsAutoDiscoveryFlag(fs *flag.FlagSet, cfg *Config, val *bool) {
	if fs == nil || cfg == nil || val == nil {
		return
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == SkillsAutoDiscoveryFlagName {
			v := *val
			cfg.Skills.AutoDiscovery = &v
		}
	})
}

// SystemSkillsSource is the marketplace Coddy is born with: the catalogue that
// publishes the skills of the standard delivery plus the rest of the same
// collection. It is not a default that gets written into skills.sources - it
// sits beside that list, which is why no surface offers to remove it and why a
// config file never has to be edited to have it. Only an address: nothing is
// fetched from it until `coddy skills sync` asks.
const SystemSkillsSource = "EvilFreelancer/rpa-skills"

// Skills is the YAML skills section (key skills).
type Skills struct {
	Dirs []string `yaml:"dirs"`

	// Sources lists remote skill sources to install from (GitHub repos, git URLs,
	// or an http(s) URL to an agents-standard marketplace.json). Fetched on demand
	// via `coddy skills sync` (never automatically), materialized into ManagedDir.
	Sources []string `yaml:"sources"`

	// AutoDiscovery enables the model-driven load_skill tool: the agent may pull a
	// catalogued skill's full instructions into a turn on its own when the request
	// matches, instead of requiring an explicit /name invocation. Defaults to true.
	AutoDiscovery *bool `yaml:"auto_discovery"`
}

// ManagedDir returns the directory used for coddy-managed skills (enable/disable state,
// UI-installed skills). Always resolves to ${CODDY_HOME}/skills or ~/.coddy/skills.
func (c *Skills) ManagedDir(coddyHome string) string {
	if coddyHome != "" {
		return filepath.Join(coddyHome, "skills")
	}
	return expandSkillsHome("~/.coddy/skills")
}

// DefaultSkillDirs are the skill directories every workspace reads, whatever
// skills.dirs says, lowest priority first: the user's agents skills (shared
// with every agent that reads ~/.agents/skills), the project's agents skills,
// Coddy's own skills (where the standard delivery and installed skills land),
// the project's Coddy skills. A name found in several is taken from the last,
// so a project overrides the user and Coddy's folders override the agents
// ones. The placeholders stay in the entries: ${CWD} expands per session
// against its workspace (the folder a new chat picked included), ${HOME} and
// ${CODDY_HOME} when the loader reads them.
func DefaultSkillDirs() []string {
	return []string{
		"${HOME}/.agents/skills",
		"${CWD}/.agents/skills",
		"${CODDY_HOME}/skills",
		"${CWD}/.coddy/skills",
	}
}

// SearchDirs is every skill directory a workspace reads, lowest priority
// first: DefaultSkillDirs, then the extra directories of skills.dirs in their
// order. Every reader of skills goes through it, so the defaults cannot be
// configured away and an extra directory wins a name over all of them. A
// folder named twice (an old config that spelled the defaults out) is read
// once, at its last place (the skills loader drops the earlier one).
func (c Skills) SearchDirs() []string {
	return append(DefaultSkillDirs(), c.Dirs...)
}

// ApplyDefaults leaves Dirs as the file has it: skills.dirs only adds
// directories to DefaultSkillDirs (see SearchDirs), so an absent key means no
// extra ones. Sources stays exactly as the file has it too: the marketplace of
// the standard delivery is a system source (SystemSkillsSource), listed beside
// this key rather than inside it.
func (c *Skills) ApplyDefaults(coddyHome string, expandCODDYHome func(string) string) {
	if c.AutoDiscovery == nil {
		v := true
		c.AutoDiscovery = &v
	}
	if len(c.Dirs) == 0 {
		return
	}
	for i := range c.Dirs {
		c.Dirs[i] = expandCODDYHome(c.Dirs[i])
	}
}

// AutoDiscoveryEnabled reports whether the model-driven load_skill tool is offered.
func (c *Skills) AutoDiscoveryEnabled() bool {
	if c.AutoDiscovery == nil {
		return true
	}
	return *c.AutoDiscovery
}

// Validate accepts any layout produced by ApplyDefaults.
func (c *Skills) Validate() error {
	return nil
}

func expandSkillsHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}
