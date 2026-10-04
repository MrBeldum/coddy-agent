//go:build gateway || gateway.telegram

package gateway

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/EvilFreelancer/coddy-agent/external/gateway/telegram"
	"github.com/EvilFreelancer/coddy-agent/internal/config"
	"github.com/EvilFreelancer/coddy-agent/internal/logger"
)

// TelegramAvailable reports whether this binary carries the Telegram adapter.
const TelegramAvailable = true

// ServeTelegram builds the Telegram bot and runs it until ctx is cancelled.
func ServeTelegram(ctx context.Context, opts Options) error {
	if opts.Cfg == nil || opts.Mgr == nil || opts.Log == nil {
		return errors.New("gateway: Cfg, Mgr and Log are required")
	}
	tg := &opts.Cfg.Gateways.Telegram
	if !tg.Enabled {
		return errors.New("gateway: the Telegram bot is not enabled; set gateways.telegram.enable: true in config")
	}
	if tg.EffectiveToken() == "" {
		return errors.New("gateways.telegram.enable is true but no token was found; set gateways.telegram.token or the " +
			config.TelegramBotTokenEnvVar + " environment variable")
	}
	// Each adapter logs under its own component so logger.levels can raise one
	// bot to debug without the rest of the process following it. Adapter tags
	// are derived from the untagged logger, not stacked on the hub's own, so a
	// record carries exactly one component.
	storePath := filepath.Join(opts.Cfg.ResolvedSessionsRoot(), "gateway_sessions.json")
	bot := telegram.New(tg, opts.Mgr, opts.DefaultCWD,
		logger.Component(opts.Log, logger.ComponentGatewayTelegram), storePath, opts.Mirror)
	if opts.Prompts != nil {
		bot.SetPromptSurfaces(opts.Prompts)
	}
	if opts.Wakes != nil {
		bot.SetWakeSurfaces(opts.Wakes)
	}
	NewHub(logger.Component(opts.Log, logger.ComponentGateway), bot).Start(ctx)
	return nil
}
