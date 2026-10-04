package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

// TelegramBotTokenEnvVar is the environment variable consulted for the bot token
// when gateways.telegram.token is left empty (so the token can live in .env instead
// of config.yaml). Mirrors the provider api_key → NAME_API_KEY convention.
const TelegramBotTokenEnvVar = "TELEGRAM_BOT_TOKEN"

// TelegramAPIBaseEnv overrides the Bot API origin (default https://api.telegram.org)
// for the Telegram gateway and for the --dry-run probe alike: a self-hosted Bot
// API server, or the offline stand cmd/tgfake. The value is an origin such as
// "http://127.0.0.1:18790"; a trailing slash is tolerated.
const TelegramAPIBaseEnv = "CODDY_TELEGRAM_API_BASE"

// GatewayConfig is the root config block for all messenger gateways (built with -tags gateway or gateway.telegram).
type GatewayConfig struct {
	Telegram TelegramGatewayConfig `yaml:"telegram"`
}

// IsolationMode controls how sessions are scoped in a group chat.
type IsolationMode string

const (
	// IsolationIndividual gives each user their own session within the group.
	IsolationIndividual IsolationMode = "individual"
	// IsolationShared uses a single session for everyone in the group.
	IsolationShared IsolationMode = "shared"
	// IsolationAdmin only responds to admin users; all admins share one session.
	IsolationAdmin IsolationMode = "admin"
)

// AccessLevel controls who may interact with the bot in a chat.
type AccessLevel string

const (
	AccessAll    AccessLevel = "all"
	AccessAdmins AccessLevel = "admins"
	// AccessGroup:<name> — checked by prefix match at runtime.
)

// TelegramGatewayConfig configures the Telegram bot adapter.
type TelegramGatewayConfig struct {
	Enabled bool   `yaml:"enable"`
	Token   string `yaml:"token"`

	// Proxy is how the bot reaches the Bot API, read like providers[].proxy
	// (ParseProxySetting): empty or "inherit" follows the environment's
	// proxy (HTTPS_PROXY, HTTP_PROXY, NO_PROXY), "none" connects directly,
	// and an http, https, socks5 or socks5h URL goes through that proxy.
	// Example: "none", "socks5h://127.0.0.1:1080", "http://proxy.example.com:3128"
	Proxy string `yaml:"proxy"`

	// RichMessages enables Bot API 10.1 Rich Messages: the agent's native Markdown
	// (headings, tables, task lists, code, footnotes, LaTeX) is sent verbatim, tool
	// activity streams as a "Thinking…" placeholder, and executed tools are listed in
	// a collapsible block. Requires a Bot API server that supports 10.1; the gateway
	// falls back to legacy formatting if a rich send fails. Default false.
	RichMessages bool `yaml:"rich_messages"`

	// Admins is the list of Telegram user IDs with elevated permissions.
	Admins []int64 `yaml:"admins"`

	// DefaultAccess is the fallback access level for chats without a specific override.
	// Values: "all", "admins", "group:<name>".
	DefaultAccess AccessLevel `yaml:"default_access"`

	// DefaultIsolation is the fallback isolation mode for group chats.
	DefaultIsolation IsolationMode `yaml:"default_isolation"`

	// UserGroups defines named sets of user IDs for group-level access control.
	UserGroups []TelegramUserGroup `yaml:"user_groups"`

	// Chats holds per-chat overrides for isolation and access.
	Chats []TelegramChatConfig `yaml:"chats"`

	// MiniApp makes the web UI of this coddy serve the bot's Mini App.
	MiniApp TelegramMiniAppConfig `yaml:"mini_app"`
}

// TelegramMiniAppConfig makes the web UI the bot's Mini App: Telegram opens it
// from the bot's menu button and from the button /app answers with, on the
// conversation of that chat. The SPA adapts to Telegram by itself; these keys
// only tell the bot where the web UI is.
type TelegramMiniAppConfig struct {
	// URL is the public https address the web UI is served at, behind a TLS
	// proxy; plain http only on a loopback host, for the offline stand.
	// Empty: the bot offers no Mini App and leaves the menu button alone.
	URL string `yaml:"url"`
	// MenuButton makes the bot's menu button open URL. Nil means true.
	MenuButton *bool `yaml:"menu_button"`
}

// MenuButtonEnabled reports whether the bot's menu button should open the
// Mini App: a URL is set and menu_button is not false.
func (m TelegramMiniAppConfig) MenuButtonEnabled() bool {
	return strings.TrimSpace(m.URL) != "" && (m.MenuButton == nil || *m.MenuButton)
}

// validateMiniAppURL checks the address Telegram is to open as a Mini App:
// absolute https (Telegram opens nothing else), plain http only on a loopback
// host, no user info, and no fragment - Telegram puts its launch parameters
// there. Empty is fine: no Mini App.
func validateMiniAppURL(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("%q is not an absolute https address", raw)
	}
	if u.Scheme == "http" && !isLoopbackHostname(u.Hostname()) {
		return fmt.Errorf("%q: Telegram opens a Mini App over https only (plain http is accepted for a loopback host, for the offline stand)", raw)
	}
	if u.User != nil {
		return errors.New("the address must not carry a user name or password")
	}
	if u.Fragment != "" || strings.Contains(raw, "#") {
		return errors.New("the address must not have a fragment: Telegram puts its launch parameters there")
	}
	return nil
}

// isLoopbackHostname reports whether host is localhost or a loopback address.
func isLoopbackHostname(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// TelegramUserGroup is a named set of Telegram user IDs.
type TelegramUserGroup struct {
	Name    string  `yaml:"name"`
	UserIDs []int64 `yaml:"user_ids"`
}

// TelegramChatConfig is a per-chat override.
type TelegramChatConfig struct {
	ChatID    int64         `yaml:"chat_id"`
	Isolation IsolationMode `yaml:"isolation"`
	Access    AccessLevel   `yaml:"access"`
}

// Normalize trims whitespace in string fields.
func (t *TelegramGatewayConfig) Normalize() {
	t.Token = strings.TrimSpace(t.Token)
	t.Proxy = normalizeProxySetting(t.Proxy)
	t.DefaultAccess = AccessLevel(strings.TrimSpace(string(t.DefaultAccess)))
	t.DefaultIsolation = IsolationMode(strings.TrimSpace(string(t.DefaultIsolation)))
	t.MiniApp.URL = strings.TrimSpace(t.MiniApp.URL)
}

// ApplyDefaults fills zero values with safe defaults.
func (t *TelegramGatewayConfig) ApplyDefaults() {
	if t.DefaultAccess == "" {
		t.DefaultAccess = AccessAll
	}
	if t.DefaultIsolation == "" {
		t.DefaultIsolation = IsolationIndividual
	}
}

// EffectiveToken returns the configured token, or the TELEGRAM_BOT_TOKEN environment
// variable when token is left empty. Returns empty when neither is set.
func (t *TelegramGatewayConfig) EffectiveToken() string {
	if tok := strings.TrimSpace(t.Token); tok != "" {
		return tok
	}
	return strings.TrimSpace(os.Getenv(TelegramBotTokenEnvVar))
}

// Validate checks the Telegram config when enabled. The token is intentionally not
// required here: it may be supplied at runtime via the TELEGRAM_BOT_TOKEN environment
// variable (see EffectiveToken). The gateway logs a clear warning and skips the bot if
// no token can be resolved at startup. An error names the key it is about
// ("gateways.telegram.proxy: ..."), so coddy -t points at that line.
func (t *TelegramGatewayConfig) Validate() error {
	if !t.Enabled {
		return nil
	}
	if err := validateProxySetting(t.Proxy); err != nil {
		return fmt.Errorf("gateways.telegram.%w", err)
	}
	if err := validateMiniAppURL(strings.TrimSpace(t.MiniApp.URL)); err != nil {
		return fmt.Errorf("gateways.telegram.mini_app.url: %w", err)
	}
	return nil
}

// ChatConfig returns the per-chat override for chatID, or nil when no override exists.
func (t *TelegramGatewayConfig) ChatConfig(chatID int64) *TelegramChatConfig {
	for i := range t.Chats {
		if t.Chats[i].ChatID == chatID {
			return &t.Chats[i]
		}
	}
	return nil
}

// IsAdmin reports whether userID is in the admins list.
func (t *TelegramGatewayConfig) IsAdmin(userID int64) bool {
	for _, id := range t.Admins {
		if id == userID {
			return true
		}
	}
	return false
}

// UserGroupIDs returns the user IDs for the named group, or nil when not found.
func (t *TelegramGatewayConfig) UserGroupIDs(name string) []int64 {
	name = strings.TrimSpace(name)
	for _, g := range t.UserGroups {
		if g.Name == name {
			return g.UserIDs
		}
	}
	return nil
}
