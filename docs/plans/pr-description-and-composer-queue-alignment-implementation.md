# PR Description and Composer Queue Alignment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Extend the paired PR-description rule with the operator's evidence conventions and remove the queued-message text's 3px vertical nudge.

**Architecture:** Keep PR guidance in the existing paired `workflow` rule. Keep the UI change CSS-only, update its source-contract test and the root design contract, and leave queue markup/state untouched.

**Tech Stack:** Markdown rule frontmatter, CSS, Vitest source-contract tests, embedded React SPA.

---

### Task 1: Remove the queue text nudge through RED-GREEN

**Files:**
- Modify: `external/ui/src/ui/chat/composerQueueCss.test.ts`
- Modify: `external/ui/src/styles.css`
- Modify: `DESIGN.md`

- [ ] Add a failing test:

```ts
test("queued message text uses its natural line box", () => {
  expect(body(".composer-queue-text")).not.toMatch(/padding-top\s*:/);
});
```

- [ ] Run:

```bash
cd external/ui
npm run test -- composerQueueCss.test.ts
```

Expected: FAIL because `.composer-queue-text` contains `padding-top: 3px`.

- [ ] Remove the centering comment and `padding-top: 3px` from `.composer-queue-text`. Keep every other declaration unchanged.

- [ ] Replace the `DESIGN.md` statement about 3px top padding with a contract that the queue text uses its natural line box without a vertical nudge.

- [ ] Re-run the focused test and expect PASS.

- [ ] Commit:

```bash
git add external/ui/src/ui/chat/composerQueueCss.test.ts external/ui/src/styles.css DESIGN.md
git commit -m "fix(ui): align queued message text naturally"
```

### Task 2: Port PR-description conventions into the paired workflow rule

**Files:**
- Modify: `.cursor/rules/workflow.mdc`
- Modify: `.claude/rules/workflow.md`

- [ ] Preserve the existing Summary, Screenshots, Verification, and Known limitations / CI template.

- [ ] Change PR-only evidence destination from a fork to the `screenshots` branch of `origin` (`coddy-project/coddy-agent`).

- [ ] Add paired bullets requiring isolated `HOME` and `CODDY_HOME`, neutral fixtures, inspection of every PNG for private data, and no private skill/infrastructure names in public PR prose.

- [ ] Keep new round directories and no-overwrite raw URL behavior.

- [ ] Run:

```bash
go test ./internal/rules -run TestRepositoryRuleMirrors -count=1
make test-agent-rules
git diff --check
```

- [ ] Commit:

```bash
git add .cursor/rules/workflow.mdc .claude/rules/workflow.md
git commit -m "docs(rules): preserve PR description conventions"
```

### Task 3: Verify, publish, and attach visual evidence

- [ ] Run:

```bash
make build TAGS="http ui"
make test
make lint
```

- [ ] Push `feat/port-claude-memory-rules` and open a PR with the existing project section order.

- [ ] Run a clean stand with isolated `HOME` and `CODDY_HOME`, capture queue before/after at 1280px and a 390px regression view in a temporary browser page through `localhost`, inspect every PNG, and push it to `origin/screenshots` under a new PR-specific round directory.

- [ ] Update the PR body with the screenshot table and final CI state.
