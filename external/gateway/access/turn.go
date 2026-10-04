//go:build gateway || gateway.telegram || gateway.pachca

package access

import (
	"strconv"
	"strings"

	"github.com/EvilFreelancer/coddy-agent/internal/config"
	"github.com/EvilFreelancer/coddy-agent/internal/session"
)

// AdminOnlyNote is what a bot says, and what the agent is told, when somebody
// who is not its admin asks for what only an admin may do.
const AdminOnlyNote = "only the bot's admins can do this"

// nonAdminDeniedTools are the tools a turn of a messenger user who is not the
// bot's admin may not call: they change the whole agent's configuration, its
// scheduled runs or the session's model, or start work that escapes the
// conversation (a subagent, a worktree).
var nonAdminDeniedTools = []string{
	"config_set", "config_revert", "config_commit", "config_rollback",
	"switch_model", "spawn_agent", "worktree_create",
	"coddy_scheduler_job_create", "coddy_scheduler_job_replace", "coddy_scheduler_job_patch",
	"coddy_scheduler_job_delete", "coddy_scheduler_job_pause", "coddy_scheduler_job_resume",
	"coddy_scheduler_job_run", "coddy_scheduler_job_cancel",
}

// NonAdminTurn is the restriction a bot puts on the turn of a user who is
// not its admin: the tools above are refused, and every call that needs
// approval asks the bot, which refuses it. An admin's turn runs unrestricted.
func NonAdminTurn() *session.TurnRestriction {
	return &session.TurnRestriction{
		DeniedTools: append([]string(nil), nonAdminDeniedTools...),
		AskAlways:   true,
		Note:        AdminOnlyNote,
	}
}

// KeyIsAdmin reports whether the conversation a session key names belongs to
// an admin: a direct chat or an individual group session by its person, a
// group whose isolation is admin by definition, a shared group never (it is
// everybody's).
func KeyIsAdmin(key string, p Policy) bool {
	parts := strings.Split(key, ":")
	switch {
	case len(parts) == 3 && parts[1] == "user":
		return idIsAdmin(parts[2], p)
	case len(parts) == 5 && parts[1] == "chat" && parts[3] == "user":
		return idIsAdmin(parts[4], p)
	case len(parts) == 4 && parts[1] == "chat" && parts[3] == "admin":
		return true
	}
	return false
}

func idIsAdmin(s string, p Policy) bool {
	id, err := strconv.ParseInt(s, 10, 64)
	return err == nil && p.IsAdmin(id)
}

var _ Policy = (*config.TelegramGatewayConfig)(nil)
