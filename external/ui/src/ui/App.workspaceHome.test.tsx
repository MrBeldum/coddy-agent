import React from "react";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { App } from "./App";
import { ConfirmProvider } from "./components/useConfirm";
import { initLocale } from "./i18n/i18n";

vi.mock("./nav/NavRail", () => ({
  NavRail: (props: { onNewChat: () => void }) => (
    <button type="button" data-testid="new-chat" onClick={props.onNewChat}>
      New chat
    </button>
  ),
}));

vi.mock("./chat/ChatScreen", () => ({
  ChatScreen: (props: {
    sessionId?: string;
    workspaceCtx?: { path?: string } | null;
  }) => (
    <output data-testid="workspace-state">
      {props.sessionId || "home"}:{props.workspaceCtx?.path || ""}
    </output>
  ),
}));

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

function heldStream() {
  return new Response(new ReadableStream<Uint8Array>({ start: () => {} }), {
    headers: { "Content-Type": "text/event-stream" },
  });
}

const ACTIVE_WORKSPACE = "/projects/active";
const DEFAULT_WORKSPACE = "/projects/default";

const fetchMock = vi.fn(
  async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input);
    if (path === "/coddy/events") return heldStream();
    if (path === "/v1/models") return json({ data: [] });
    if (path.startsWith("/coddy/sessions?")) return json({ sessions: [] });
    if (path.startsWith("/coddy/sessions/sess_active/messages")) {
      return json({
        session_id: "sess_active",
        messages: [{ role: "user", content: "hello" }],
      });
    }
    if (path === "/coddy/workspace/context") {
      const sid = new Headers(init?.headers).get("X-Coddy-Session-ID");
      const selected =
        sid === "sess_active" ? ACTIVE_WORKSPACE : DEFAULT_WORKSPACE;
      return json({
        path: selected,
        name: selected.split("/").at(-1),
        is_git_repo: false,
        is_worktree: false,
      });
    }
    if (path === "/coddy/config") return json({});
    if (path.startsWith("/coddy/slash-commands")) return json({ items: [] });
    return json({}, 404);
  },
);

beforeEach(() => {
  initLocale("en");
  localStorage.clear();
  history.replaceState(null, "", "/#/s/sess_active");
  fetchMock.mockClear();
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  history.replaceState(null, "", "/");
});

test("ordinary new chat keeps and previews the active chat workspace", async () => {
  render(
    <ConfirmProvider>
      <App />
    </ConfirmProvider>,
  );

  await waitFor(() =>
    expect(screen.getByTestId("workspace-state")).toHaveTextContent(
      `sess_active:${ACTIVE_WORKSPACE}`,
    ),
  );

  fireEvent.click(screen.getByTestId("new-chat"));

  await waitFor(() =>
    expect(screen.getByTestId("workspace-state")).toHaveTextContent(
      `home:${ACTIVE_WORKSPACE}`,
    ),
  );
  await new Promise((resolve) => setTimeout(resolve, 0));
  const homeWorkspaceProbes = fetchMock.mock.calls.filter(
    ([input, init]) =>
      String(input) === "/coddy/workspace/context" &&
      new Headers((init as RequestInit | undefined)?.headers).get(
        "X-Coddy-Session-ID",
      ) !== "sess_active",
  );
  expect(homeWorkspaceProbes).toEqual([]);
});
