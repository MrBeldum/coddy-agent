# Plan: the web UI as a Telegram Mini App (issue #320, PR #373)

Status: design record written 2026-10-04 on `codex/telegram-mini-app` (PR #373)
after merging `main` (8b6f68a9). The cross-review verdict and the deviations
found while implementing go to sections 9 and 10.

## 1. What the issue asks

Issue #320: the web UI opened inside the Telegram mobile app as a Mini App

1. does not know it is in Telegram;
2. does not fit the Mini App window: the content overflows and stretches;
3. opens the mode and model menus at the bottom of the screen, where they
   cannot be used.

Acceptance: the environment is detected at load; on a phone the layout fits
the window with no horizontal scroll or overflow; the mode and model menus
open in a usable place; both themes work; ordinary mobile and desktop
browsers behave as before.

The operator asked for more than the issue: what else the change should do,
which configuration it needs, how `tgfake` can emulate a Mini App for testing,
and how to keep everything Telegram-specific in the Telegram package so that a
binary built without the gateway tag loses nothing and breaks nothing.

## 2. What PR #373 does

Three commits by hijera (the change, a merge of `main`, a fix that keeps the
keyboard inset on the docked composer), merged with `main` again here:

- `index.html` stores `location.search + location.hash` in
  `window.__coddyTelegramLaunchURL`, then loads
  `https://telegram.org/js/telegram-web-app.js?63` synchronously in `<head>`;
- `ui/telegramMiniApp.ts` (`initTelegramMiniApp`, called from `main.tsx`)
  decides Mini App mode from a non-empty `Telegram.WebApp.initData` or a
  non-empty `tgWebAppData` / `tgWebAppVersion` / `tgWebAppStartParam` in the
  launch URL, sets `data-telegram-mini-app="true"` on `<html>`, writes
  `--coddy-telegram-viewport-height`, `--coddy-telegram-stable-height` and four
  `--coddy-telegram-safe-*` variables, follows `viewportChanged`,
  `safeAreaChanged`, `contentSafeAreaChanged` and window/visual-viewport
  resizes, and calls `ready()`;
- one `@media (max-width: 1199px)` block in `styles.css`: `overflow-x: clip` on
  `html`, the shell takes the visible height, the top bar and the docked
  composer respect the safe area, the docked composer is lifted by
  `max(--coddy-keyboard-inset, 100dvh - stable height)`, and every
  `.mode-menu--sheet` (mode, model, reasoning, permission, environment, folder,
  branch) becomes a panel centred in the visible height;
- six vitest cases, a paragraph in `docs/surfaces/web-ui.md` and in
  `DESIGN.md`, and a 390px screenshot of the centred mode menu.

## 3. How far it fixes the issue

| Criterion | State |
| --- | --- |
| Detected at load | Yes, from launch markers or `initData`. |
| Fits the window | Partly: the composer and the shell follow the visible height; the slash and `@` sheet, the context sheet and the dialogs do not, and the transcript's last lines stay under the lifted composer of a half-open app. |
| Mode and model menus usable | Yes, centred in the visible height. |
| Both themes | Yes for Coddy's themes; Telegram's own colours are ignored. |
| Ordinary browsers unchanged | **No**: every browser now loads a script from `telegram.org` before the page can render. |

Found by reading the change against the Telegram documentation
(`core.telegram.org/bots/webapps`, `core.telegram.org/api/web-events`), the
SDK it loads, and the stacked shell's CSS:

- **P1. A third-party script for every visitor.** The script is parser-blocking
  in `<head>`, so where `telegram.org` is slow or blackholed (some corporate
  networks, some countries, an air-gapped LAN) the page stays blank until the
  connection times out - in an ordinary browser that never sees Telegram. Every
  page load reports the visitor to `telegram.org`; the SDK logs four
  `[Telegram.WebView] > postEvent` lines to the console in every browser, and
  inside any iframe it posts `iframe_ready` to the parent with target `*` and
  reloads itself when the parent says `reload_iframe`. The file behind `?63`
  can change under a released binary. Inside Telegram the WebView reaches
  `telegram.org` directly, not through the MTProto proxy the app may use, so
  where only the app gets through the Mini App does not start.
- **P2. `overflow-x: clip` on `html`.** The stacked shell's own comment
  forbids it: a non-visible overflow on `html` stops `body`'s `overflow-y`
  from passing to the viewport, `body` becomes a scroll container, and the
  sticky chat title stops sticking. Nothing needs the clip: since #340 nothing
  in the shell is wider than the page.
- **P3. The top inset.** The override of `--coddy-mobile-top-inset` sits on
  `.shell`, drops the environment banner's height that the `:root` value adds,
  and does not reach what is portalled into `<body>` (the folder browser) or
  the banner itself (`--coddy-mobile-bar-h`). Safe areas are combined with
  `max`, while Telegram's content safe area is measured inside the device's
  (fullscreen: status bar plus Telegram's buttons), so the inset is their sum.
- **P4. The window is never configured.** No `expand()`: a Mini App opened
  from a button may start at half height, the state the issue was filed from.
  Vertical swipes stay on, so pulling the transcript down at its top collapses
  or closes the app. No BackButton: Android's back key closes the whole Mini
  App even while a sheet or Settings is open. Telegram's header keeps
  Telegram's colour over Coddy's background.
- **P5. Centred panels instead of sheets, and only some of them.**
  `DESIGN.md` makes every menu and picker of the stacked shell a bottom sheet;
  the change turns the composer's menus into centred dialogs and leaves the
  slash and `@` sheet (`.slash-menu--sheet`, its bottom set inline), the
  context sheet and the confirmation dialog at the bottom or centre of the CSS
  viewport. The cause was a sheet pinned below the visible part of a half-open
  app; lifting it by the hidden part, the rule the composer follows, fixes the
  cause and keeps the design. The transcript's tail spacer
  (`--chat-composer-reserve`) does not know about that lift either.
- **P6. The launch parameters stay in the URL.** The router ignores them, but
  the Docs reader and the environment switch keep the raw fragment, and
  `coddy_env_routes` in `localStorage` can end up holding the signed
  `tgWebAppData`.
- **P7. Nothing on the bot side.** An operator wires the Mini App by hand in
  @BotFather, and a chat has no way to open its own session in the web UI,
  although the session is an ordinary one the web UI already shows live.
- **P8. Telegram Web cannot sign in.** web.telegram.org runs a Mini App in a
  cross-site iframe. The sign-in cookie is `SameSite=Strict`
  (`external/httpserver/auth_login.go`), so it never travels there: the form
  accepts the password and the next request is a 401. Nothing says why.
- **P9. Tests and evidence.** No happy-path feature, no browser check, and
  `tgfake` cannot open a Mini App (a `web_app` button is refused with
  `BUTTON_TYPE_INVALID`, `setChatMenuButton` answers 404, the token is not
  kept, so `initData` cannot be signed). The screenshot is an emulation in
  Russian, while the documentation is captured in English.
- **P10. Documentation.** Nothing tells an operator how to make the web UI a
  Mini App: HTTPS, sign-in, the menu button.

Side findings in the same area, fixed in this change: `docs/surfaces/gateway.md`
and the gateway rule name `external/gateway/start.go` / `start_stub.go`, which
are `serve_gateway.go` / `serve_stub.go`; `cmd/coddy/serve.go`
`gatewayFingerprint` lists the Telegram fields by hand, so a field added to the
config would not rebuild the bot on a reload; the `/resume` and subagent
permission harnesses answer the Bot API with a hand-written handler instead of
`tgfake`, against the gateway rule.

## 4. Decisions

### D1. No script from telegram.org: a bridge of our own

The Mini App protocol is small and documented (`core.telegram.org/api/web-events`):
a Mini App posts events through `window.TelegramWebviewProxy.postEvent(type,
json)` in the mobile and desktop apps, `window.external.notify(json)` in the
oldest Windows client, or `window.parent.postMessage(json, '*')` from the
iframe of a web client; the client answers by calling
`window.Telegram.WebView.receiveEvent(type, data)` (older builds:
`window.TelegramGameProxy.receiveEvent`, `TelegramGameProxy_receiveEvent`) or,
in an iframe, with a `message` from the parent. Coddy needs a dozen of those
events, so `ui/telegram/bridge.ts` implements exactly that: one `postEvent`,
one `on(type, handler)`, the receivers installed only once a launch is
detected, `iframe_ready` posted and `reload_iframe` honoured in an iframe as the
SDK does, and `set_custom_style` ignored (it injects the parent's CSS).
Ordinary browsers run none of it and load nothing new; `index.html` goes back
to what `main` has. Version gates follow the Bot API version the launch names
(`tgWebAppVersion`): the header and background colours from 6.1 (the header
by value from 6.9, by theme key before), the swipe behaviour from 7.7, the
bottom bar colour from 7.10; the safe areas exist from 8.0, and asking an
older client for them is harmless, as the SDK does unconditionally.

### D2. Launch parameters: read once, kept for the tab, removed from the URL

`ui/telegram/launch.ts` reads the parameters Telegram puts into the fragment
(`#tgWebAppData=...&tgWebAppVersion=...`, or after a fragment the button URL
already had, `#<route>?tgWebAppData=...`, the form the SDK's parser accepts)
and the query (`tgWebAppStartParam`, and the fallbacks of older clients). A
launch is a non-empty `tgWebAppVersion` or `tgWebAppData`; a start parameter
alone is not, since anyone can type one into an address. The module is the
first import of `main.tsx` and runs at import, before anything else can read
or rewrite the fragment. It keeps the parameters in `sessionStorage`
(`coddy.telegram.launch`), so a reload inside the Mini App - pull to refresh,
`reload_iframe`, the SPA's own reloads - is still a Mini App, and it puts the
URL back to the SPA's route alone with `history.replaceState` before the
router reads it (P6). `initData` stays in that one place for a server that may
validate it one day; the SPA never treats it as an identity.

### D3. The Mini App window

At start, in this order: the theme (D5) and the CSS variables from what the
launch says; `web_app_request_viewport`, `_safe_area`, `_content_safe_area`,
`_theme`; `web_app_expand` (a chat needs the height, and the half-open state is
where the issue came from); vertical swipes off (`web_app_setup_swipe_behavior`,
so pulling the transcript does not fold the app; the header still does); the
header, background and bottom bar colours from the theme's canvas tokens
(`--coddy-canvas-gradient-top` for the header, `--coddy-canvas-gradient-bottom`
for the other two), sent again whenever `data-theme` changes; `web_app_ready`
last, so Telegram drops its placeholder over a page that already has its
colours. Events: `viewport_changed` sets the visible height and, when
`is_state_stable`, the stable height; `safe_area_changed` and
`content_safe_area_changed` set the insets, top and bottom being the sum of the
two (P3); `theme_changed` goes to D5. Until the first `viewport_changed` the
heights come from `visualViewport` and `innerHeight`, as in the PR.

**BackButton.** Telegram's back button stands for Escape, the key that already
"undoes one step" everywhere (`nav/railEscape.ts`). It is shown while a
conversation is open, a screen of the rail is open, or a modal layer is in the
DOM (the scrims and dialogs the module knows by class; a layer it misses only
leaves Telegram's Close in place, which is what happens today). Pressing it
dispatches a cancelable Escape at the focused element; every layer that
answers the key answers the button, through the handlers it already has. A
one-shot listener on `document` in the bubble phase, added just before the
dispatch and so run after the rail's, sees whether anything claimed it; if
nothing did, it claims it itself (`preventDefault`), so the question card,
which listens on `window` and treats an unclaimed Escape as "skip", never sees
it, and the module leaves the conversation for the start screen through the
router's own `setSessionHashInLocation("")`. With the button hidden Telegram
shows Close, and Android's back key closes the app from the start screen with
nothing open.

### D4. Sheets stay bottom sheets; what a half-open app lifts

`DESIGN.md` makes every menu and picker of the stacked shell a bottom sheet. In
a Mini App the sheet keeps that shape and is lifted by the part of the WebView
Telegram does not show, `--coddy-telegram-hidden-bottom` = `100dvh - stable
height` (never below zero), combined with the keyboard inset the way the
composer already is, and capped to the visible height. Lifted the same way:
the docked composer (as in the PR), `.mode-menu--sheet`, `.slash-menu--sheet`
on the start screen (whose inline bottom is zero there; above the docked
composer it follows the card, which is lifted already),
`.context-breakdown-menu--sheet`, the confirmation dialog and the image viewer
(their backdrops end at the visible bottom, so what they centre is visible).
The transcript's tail spacer grows by the hidden part, so the last message
stays above the lifted composer. Drawers and docks (History, Settings,
Scheduler, Tasks, Docs, Swarm) keep their bottom: their head and close button
are at the top, in sight, and with `expand()` and swipes off a half-open app is
one the user pulled down on purpose.

The `overflow-x: clip` on `html` goes (P2). The top inset is set where the
stacked shell sets it, on `:root`, by overriding `--coddy-mobile-bar-h`, so
the banner term, the portalled dialogs and the banner itself follow (P3).

### D5. The theme follows Telegram until the user picks one

With no `coddy_ui_theme` cookie, a Mini App starts on `light` or `dark` by the
brightness of Telegram's `bg_color` (the SDK's rule) and follows
`theme_changed`. It applies the theme with `applyUiTheme`, after
`bootstrapUiThemeFromCookie` in `main.tsx` (before it, the bootstrap would
overwrite it), and writes nothing, so a pick in Settings → Appearance, which
writes the cookie, wins from then on, in Telegram and outside it. The picker
follows `data-theme` already. Telegram keeps its placeholder up until
`web_app_ready`, which is posted after the theme, so the switch is not seen.

### D6. Opening a chat's own conversation

A conversation in a chat is an ordinary session, already live in the web UI.
The bot's link to it carries the session in the query, `?session=<id>`,
because the fragment belongs to Telegram's launch parameters; the launch module
turns it into the SPA's session route (`#/s/<id>`) and drops it from the URL. A
start parameter that is a session id (`t.me/<bot>?startapp=sess_...`, for a bot
with a main Mini App in @BotFather) is read the same way. Session ids are
`sess_` and hex, inside what Telegram allows in a start parameter.

### D7. Configuration

```yaml
gateways:
  telegram:
    mini_app:
      url: https://coddy.example.com/   # the public https address of this coddy serve
      menu_button: true                  # the bot's menu button opens the web UI (default)
```

- `url` empty (the default) means the bot offers no Mini App and does not touch
  the menu button: an operator who set one in @BotFather keeps it.
- `url` must be absolute and `https`, or `http` on a loopback host (the offline
  stand; Telegram itself refuses `http`), with no fragment (the fragment is
  Telegram's) and no user info. It is checked only while the gateway is
  enabled, like the rest of the block, and the error names
  `gateways.telegram.mini_app.url`.
- `menu_button` is a `*bool`, true when absent.
- No key for the web UI's own address elsewhere: nothing else needs it, and
  the HTTP server cannot know the address a TLS proxy publishes it under.
- The block goes through the JSON mirror, the settings form, the schema, the
  example config and the `configure-coddy` skill, like every key.
- `gatewayFingerprint` moves from `cmd/coddy/serve.go` into the gateway
  package's untagged `serve.go` (`gateway.Fingerprint`) and is built from the
  whole Telegram block, so a key added later rebuilds the bot on a reload
  without anyone remembering to list it.

### D8. The bot offers the Mini App

`external/gateway/telegram/miniapp.go`, behind the gateway tag:

- **Menu button.** On every start, with `url` set and `menu_button` on, the
  bot's default menu button (`setChatMenuButton` without `chat_id`) opens
  `url`. Coddy remembers what it set, per bot, in the gateway store (a reserved
  entry next to `$last_model`). When the configuration stops asking for it,
  the button goes back to the default only if it still holds what Coddy set; a
  button the operator changed in @BotFather in the meantime is left alone.
- **`/app`.** Answers with one button that opens the chat's own session
  (`url?session=<id>`), or the start screen when the chat has none yet. In a
  private chat it is a `web_app` button; Telegram allows those only there, so a
  group gets an ordinary `url` button to the same address. Without `url` the
  command says which key turns it on. It passes the access checks every command
  passes, joins `setMyCommands` and `/help` only while `url` is set, and counts
  as addressed to the bot in a group.
- The library the gateway uses (`go-telegram-bot-api/v5`) has no Mini App
  types, so both calls go through `MakeRequest` with JSON of our own, the way
  `richclient.go` sends Rich Messages.

### D9. Sign-in stays as it is

Signing in with Telegram's `initData` (HMAC-SHA256 under the bot token) would
spare the password, but it is a new way into an agent that runs commands, and
it does not belong in this change:

- `httpserver` has no hook for it: the session store and the login policy are
  unexported, and the only mode is `password`;
- `default_access` is `all` by default, so "a user the bot talks to" is not an
  allowlist; a Telegram sign-in needs its own (admins by default);
- it would still not work in Telegram Web, where the cookie cannot travel
  (P8), without loosening the cookie's SameSite and the same-origin checks the
  CSRF defence rests on.

A Mini App on a phone or a desktop runs in a top-level WebView, where the
password sign-in and its cookie work as in a browser. Proposed as a follow-up:
`gateways.telegram.mini_app.sign_in` naming who may sign in (admins by
default), a method the gateway offers through `serve.Runtime` the way it offers
its permission prompts, `initData` checked against the token and an
`auth_date` window, a session in the same store.

What this change does about P8 is generic and small: when the server accepts
the password but the next `/coddy/auth/me` still has no session, the sign-in
screen says that the browser did not keep the sign-in in this frame and offers
to open Coddy in a tab of its own. Any third-party frame gets the same honest
answer; nothing in it is Telegram's.

### D10. Where the code lives, and a build without the gateway

- **SPA:** everything in `external/ui/src/ui/telegram/` - `launch.ts` (D2),
  `bridge.ts` (D1), `miniApp.ts` (D3, D5), `backButton.ts` (D3),
  `telegram.css` (D4, imported by `miniApp.ts`, so the bundle carries it after
  `styles.css`), and their tests. Outside it: two lines of `main.tsx` and the
  generic sign-in note of D9. `layoutGridCss.test.ts` reads every stylesheet
  under `src`, not only `styles.css`, so the grid still covers this one. The
  SPA's Mini App mode needs nothing from the server, so it works the same
  against a binary built without the gateway (an operator can point a
  @BotFather button at it by hand).
- **Go:** the config block in `internal/config` (untagged, like every gateway
  key), `gateway.Fingerprint` in the untagged `external/gateway/serve.go`,
  everything else in `external/gateway/telegram` behind
  `gateway || gateway.telegram`. No new HTTP route, nothing in `httpserver`.
- **Checked:** `go build` and `go vet` with no tags, `http`, `http ui`,
  `gateway.telegram`, and the full set; `make test`; the CI matrix.

### D11. tgfake emulates a Mini App

- **Bot API:** `web_app` on inline buttons (private chats only; `https`, or
  `http` on a loopback host, otherwise the error Telegram gives) and on reply
  keyboard buttons; `setChatMenuButton` / `getChatMenuButton` for the default
  and per chat; `/sim/state` lists the menu buttons.
- **The token:** kept from the path of the first Bot API call when `--token` is
  not given, so the stand needs no new flag to sign `initData`.
- **`POST /sim/webapp/launch`** `{chat_id, user_id, username, first_name, url,
  start_param, color_scheme}` returns the launch URL and `initData` the way
  Telegram builds them: `query_id`, `user`, `auth_date`, `hash` = hex
  HMAC-SHA256 of the data-check-string under HMAC-SHA256(key "WebAppData",
  token). `signature` (Ed25519 under Telegram's key) cannot be produced and is
  left out.
- **The chat page** opens a Mini App from a `web_app` button or the menu button
  in a phone frame beside the chat. The WebView is an iframe and the page is
  its Telegram, the way web.telegram.org is: it answers the requests of D3,
  shows Back when the app asks for it and sends `back_button_pressed`, paints
  its header with the colour the app sets, and has controls for the size,
  expanded or half open (the iframe keeps the full height and the frame clips
  it, as on a phone), light or dark (`theme_changed`), a notch
  (`safe_area_changed`) and an Android-style keyboard (the WebView shrinks).
  It logs every event both ways and keeps them in `window.__tgfakeMiniApp` for
  scripts.

## 5. Tests

- **Go, config:** `mini_app.url` validation table (https, loopback http,
  fragment, user info, relative, empty), defaults, JSON mirror round trip,
  `TestDocsConfigSchemaMatchesStructs` through the schema.
- **Go, gateway:** `features/gateway_telegram_mini_app.feature` (the bot sets
  its menu button to the web UI; `/app` opens the chat's session; a bot that no
  longer offers a Mini App takes back the button it set), run against `tgfake`;
  unit tests for the link, the group fallback, the ownership rule, the command
  list and `/help`; `gateway.Fingerprint` moves when any field of the Telegram
  block moves (reflection over the struct).
- **Go, tgfake:** `features/tgfake_mini_app.feature` (a `web_app` button and
  the menu button open a launch whose `initData` checks out against the
  token), unit tests for the refusals Telegram gives, the menu button round
  trip and the page.
- **SPA:** vitest for `launch` (fragment and query forms, persistence, URL
  clean-up, deep link), `bridge` (each transport, the receivers, the iframe
  messages, nothing installed outside a launch), `miniApp` (the start sequence
  and its order, version gates, events to CSS variables, the sum of the
  insets, colours on a theme change, theme without and with a cookie),
  `backButton` (visibility, a claimed and an unclaimed Escape, the question
  card untouched), the CSS contract of `telegram.css`, and the sign-in note;
  `features/web_ui_telegram_mini_app.feature` binds them as the happy path;
  `index.html` loads nothing from another origin.
- **Browser:** `npm run check:telegram` (`scripts/telegram-mini-app-check.mjs`):
  the real `coddy serve` (`TAGS="http ui gateway"`) and `tgfake` with the
  scripted model; the bot answers, `/app` gives a button, the page opens it in
  its phone frame, and in the half-open state the composer, the mode sheet and
  the last message are inside the visible part, Back closes the sheet, the
  sticky title sticks, nothing scrolls sideways. Local, like `check:transcript`.
- **Builds:** the tag sets of D10.

## 6. Documentation

- `docs/surfaces/web-ui.md`: the Mini App section rewritten (what the UI does
  inside Telegram, no third-party script, the half-open rules, Back, the
  theme, the deep link, Telegram Web and sign-in), with English screenshots
  captured through `tgfake`.
- `docs/surfaces/gateway.md`: a Mini App section (HTTPS behind a TLS proxy,
  sign-in on, `mini_app.url`, the menu button and @BotFather, `/app`, security
  notes), `/app` in the commands table, the Mini App panel in *Debugging
  against a fake Bot API*, the stale `start.go` names.
- `docs/reference/config.md` (generated), `config.example.yaml`, the
  `configure-coddy` skill, `DESIGN.md` (the Telegram Mini App rules),
  `docs/plans/telegram-mini-app.md` (this record).
- `.claude/rules/gateway.md`, `.claude/rules/ui-spa.md` and their `.cursor`
  mirrors, the map rows of `AGENTS.md` for the gateway, `tgfake` and the SPA.

## 7. Order of work

1. This record; cross-review round 1; corrections.
2. `tgfake`: Mini App support with its tests.
3. Config: the block, its mirrors and checks, `gateway.Fingerprint`.
4. Bot: menu button and `/app` with the feature.
5. SPA: `ui/telegram/` (launch, bridge, window, theme, back), `telegram.css`
   replacing the block in `styles.css`, `index.html` back to `main`, the
   sign-in note, tests and the feature.
6. The browser check, screenshots, documentation.
7. The side findings: stale names, the two harnesses on `tgfake`.
8. `make test`, `make lint`, `make docs-check`; cross-review round 2;
   corrections.

## 8. Out of scope

- Signing in with Telegram (D9), proposed as a follow-up.
- Fullscreen mode (`requestFullscreen`): the content safe area is handled, the
  request is not made.
- A per-chat menu button that always opens that chat's session: one call per
  session change and stale buttons to clean when the URL goes; `/app` does the
  job.
- Telegram's main and secondary bottom buttons, haptics, cloud storage.
