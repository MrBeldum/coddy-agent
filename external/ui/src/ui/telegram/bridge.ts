/**
 * The Mini App side of Telegram's web events protocol
 * (core.telegram.org/api/web-events), written here instead of loading
 * telegram-web-app.js: the web UI needs a dozen of its events, and the script
 * would have to come from telegram.org on every load, for every visitor.
 *
 * Outgoing, an event goes the way the client offers: the mobile and desktop
 * apps inject `window.TelegramWebviewProxy.postEvent(type, json)`, the oldest
 * Windows client `window.external.notify(json)`, and a web client
 * (web.telegram.org) runs the app in an iframe and reads
 * `postMessage(json, "*")` from it. Incoming, the apps call
 * `window.Telegram.WebView.receiveEvent(type, data)` (older builds
 * `window.TelegramGameProxy.receiveEvent` or `TelegramGameProxy_receiveEvent`),
 * and a web client posts `{eventType, eventData}` to the frame. Nothing here
 * is installed until a launch is detected, so an ordinary browser runs none of
 * it.
 *
 * Every event is presentation (heights, insets, colours, the back button):
 * nothing that arrives here is trusted with anything else. `set_custom_style`,
 * which would put the parent's CSS into the page, is ignored.
 */

export type TelegramEventHandler = (data: unknown) => void;

export type TelegramBridge = {
  /** Sends an event to the client; a no-op where there is no client. */
  post(type: string, data?: unknown): void;
  /** Subscribes to an event from the client; returns the unsubscribe. */
  on(type: string, handler: TelegramEventHandler): () => void;
  /** Removes the receivers this bridge installed. */
  dispose(): void;
};

type Receiver = (type: string, data: unknown) => void;

type TelegramWindow = Window & {
  TelegramWebviewProxy?: { postEvent?: (type: string, data: string) => void } | undefined;
  Telegram?: { WebView?: { receiveEvent?: Receiver | undefined } | undefined } | undefined;
  TelegramGameProxy?: { receiveEvent?: Receiver | undefined } | undefined;
  TelegramGameProxy_receiveEvent?: Receiver | undefined;
};

function inIframe(win: Window): boolean {
  try {
    return win.parent != null && win.parent !== win;
  } catch {
    // A cross-origin parent that cannot even be compared is a parent.
    return true;
  }
}

/** createTelegramBridge installs the receivers on win and returns the bridge. */
export function createTelegramBridge(win: Window = window): TelegramBridge {
  const w = win as TelegramWindow;
  const handlers = new Map<string, Set<TelegramEventHandler>>();
  const framed = inIframe(win);

  const receive = (type: string, data: unknown) => {
    const set = handlers.get(type);
    if (!set) {
      return;
    }
    for (const handler of [...set]) {
      try {
        handler(data);
      } catch (err) {
        console.error("telegram: handler for", type, err);
      }
    }
  };

  const post = (type: string, data?: unknown) => {
    const eventData = data === undefined ? "" : data;
    try {
      if (typeof w.TelegramWebviewProxy?.postEvent === "function") {
        w.TelegramWebviewProxy.postEvent(type, JSON.stringify(eventData));
        return;
      }
      const external = (w as unknown as { external?: { notify?: (s: string) => void } }).external;
      if (external && typeof external.notify === "function") {
        external.notify(JSON.stringify({ eventType: type, eventData }));
        return;
      }
      if (framed) {
        win.parent.postMessage(JSON.stringify({ eventType: type, eventData }), "*");
      }
    } catch (err) {
      console.error("telegram: post", type, err);
    }
  };

  const onMessage = (ev: MessageEvent) => {
    if (ev.source !== win.parent) {
      return;
    }
    let msg: unknown = ev.data;
    if (typeof msg === "string") {
      try {
        msg = JSON.parse(msg);
      } catch {
        return;
      }
    }
    if (!msg || typeof msg !== "object") {
      return;
    }
    const { eventType, eventData } = msg as { eventType?: unknown; eventData?: unknown };
    if (typeof eventType !== "string" || eventType === "set_custom_style") {
      return;
    }
    if (eventType === "reload_iframe") {
      post("iframe_will_reload");
      win.location.reload();
      return;
    }
    receive(eventType, eventData);
  };

  const previous = {
    webView: w.Telegram?.WebView,
    gameProxy: w.TelegramGameProxy,
    gameProxyReceive: w.TelegramGameProxy_receiveEvent,
  };
  w.Telegram = { ...(w.Telegram ?? {}), WebView: { ...(w.Telegram?.WebView ?? {}), receiveEvent: receive } };
  w.TelegramGameProxy = { ...(w.TelegramGameProxy ?? {}), receiveEvent: receive };
  w.TelegramGameProxy_receiveEvent = receive;
  if (framed) {
    win.addEventListener("message", onMessage);
    post("iframe_ready", { reload_supported: true });
  }

  return {
    post,
    on(type, handler) {
      let set = handlers.get(type);
      if (!set) {
        set = new Set();
        handlers.set(type, set);
      }
      set.add(handler);
      return () => {
        set?.delete(handler);
      };
    },
    dispose() {
      handlers.clear();
      win.removeEventListener("message", onMessage);
      if (w.Telegram) {
        w.Telegram.WebView = previous.webView;
      }
      w.TelegramGameProxy = previous.gameProxy;
      w.TelegramGameProxy_receiveEvent = previous.gameProxyReceive;
    },
  };
}

/**
 * versionAtLeast compares Bot API versions part by part ("10.1" >= "7.7");
 * an unknown version is taken as the oldest, so a feature gated on it is
 * left out rather than sent to a client that may not know it.
 */
export function versionAtLeast(version: string, min: string): boolean {
  const a = version.split(".").map((p) => Number.parseInt(p, 10));
  const b = min.split(".").map((p) => Number.parseInt(p, 10));
  if (a.some((n) => !Number.isFinite(n))) {
    return false;
  }
  for (let i = 0; i < Math.max(a.length, b.length); i++) {
    const x = a[i] ?? 0;
    const y = b[i] ?? 0;
    if (x !== y) {
      return x > y;
    }
  }
  return true;
}
