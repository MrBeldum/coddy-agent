# PR description evidence and composer queue alignment

Status: approved design. The operator selected the minimal extension of existing rules and the existing queue component.

## Goal

Keep pull request descriptions in the established project format, make screenshot evidence safe and reproducible for all contributors, and remove the visual vertical nudge from queued-message text.

## Scope

### Pull request description rule

Update both native workflow representations:

- `.cursor/rules/workflow.mdc`;
- `.claude/rules/workflow.md`.

Keep the existing section order and wording contract:

1. `Summary` describes the final observable diff and compatibility notes;
2. `Screenshots` is present only for a changed user-visible surface and uses the existing surface-by-width table;
3. `Verification` lists exact commands and outcomes;
4. `Known limitations / CI` states unresolved limitations, skipped checks, failures, or pending jobs honestly.

Add the missing operator conventions from Claude memory:

- PR-only screenshot evidence goes to the `screenshots` branch of `origin` (`coddy-project/coddy-agent`), never to a personal fork remote;
- every recapture uses a new PR/round directory, and an existing raw URL is never overwritten;
- the screenshot stand runs with isolated `HOME` and `CODDY_HOME` and neutral fixtures;
- every PNG is opened and inspected before push for private skill names, internal hosts, tokens, usernames, and personal filesystem paths;
- public PR prose and screenshots never name private working skills or internal infrastructure;
- the PR body continues to describe the final diff and is updated after follow-up commits and CI results.

Do not change the site publication destination or direct-push policy in this change.

### Composer queue text

Update `external/ui/src/styles.css`:

- remove `padding-top: 3px` from `.composer-queue-text`;
- remove the comment claiming that the nudge centers the text on the 24px close control;
- keep all other queue geometry unchanged, including row padding, close-control size, line clamp, wrapping, and cross/wand right-edge alignment.

Update `DESIGN.md` so the queue contract says that text uses its natural line box with no vertical nudge. The first line no longer needs to be centered against the close control by padding.

### Regression contract

Update `external/ui/src/ui/chat/composerQueueCss.test.ts` before the CSS change:

- assert that `.composer-queue-text` contains no `padding-top` declaration;
- retain all existing glass, close-control, and right-edge alignment assertions.

The test must fail against the current `padding-top: 3px` rule, then pass after removal.

## Visual verification

Use the running UI from a clean stand with isolated `HOME` and `CODDY_HOME`.

- capture the queued-message surface before and after at 1280px;
- capture 390px as a regression check because the same queue row renders on the stacked shell;
- use a temporary browser-context page, set `PORT=5201` or another free port, and navigate through `http://localhost:$PORT`;
- inspect each PNG before pushing it to the `origin` screenshots branch;
- attach the before/after table to the pull request, not to `docs/assets/`, because this is PR evidence rather than documentation of a new feature.

## Verification

Run:

```bash
cd external/ui
npm run test -- composerQueueCss.test.ts
cd ../..
make test-agent-rules
go test ./internal/rules -run TestRepositoryRuleMirrors -count=1
make build TAGS="http ui"
make lint
```

Then run the live browser capture described above.

## Non-goals

- no queue markup or React state changes;
- no change to queue card height, close control, files count, mode button, or first-use choice;
- no new PR-description rule file;
- no root `AGENTS.md` change;
- no change to site publication policy;
- no generated docs or committed screenshots under `docs/assets/`.
