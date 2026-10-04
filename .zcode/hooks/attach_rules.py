#!/usr/bin/env python3
"""Attach `.cursor/rules/*.mdc` to ZCode sessions deterministically.

ZCode has no glob-based rule attachment: its instruction file (`AGENTS.md`) is
resolved once per session from the working directory up to the project root, so
nested instruction files never load, and there is no equivalent of the `globs`
field in `.cursor/rules/*.mdc`. Asking the model to "read the relevant rule
file" is advisory and gets skipped.

This hook restores Cursor's behaviour by reading the frontmatter of the rule
files directly:

  SessionStart -> inject every rule with `alwaysApply: true`
  PreToolUse   -> inject rules whose `globs` match the file paths a tool is
                  about to touch, at most once per rule per session

The hook reads `.cursor/rules/` directly and creates no ZCode copy of a rule body.
It fails open: any unexpected input exits 0 with no output, so a broken rule file
can never block an edit.

Wired up in `.zcode/config.json` under `hooks.events.{SessionStart,PreToolUse}`.
ZCode delivers edited paths as JSON fields of the tool payload
(`tool_input.file_path`, `.path`, ...), so this variant reads those structured
fields. The Codex sibling accepts the same fields and also parses patch headers.
"""

from __future__ import annotations

from contextlib import contextmanager
import hashlib
import json
import os
import re
import sys
import tempfile
from pathlib import Path

if os.name == "nt":
    import msvcrt
else:
    import fcntl

# The repository root is two levels up from this script:
# .zcode/hooks/attach_rules.py -> repo root.
SCRIPT_ROOT = Path(__file__).resolve().parent
REPO_ROOT = SCRIPT_ROOT.parents[1]
RULES_DIR = Path(os.environ.get("ZCODE_RULES_DIR") or (REPO_ROOT / ".cursor" / "rules"))


def default_state_dir(repo_root: Path) -> Path:
    repo_key = hashlib.sha256(str(repo_root.resolve()).encode()).hexdigest()[:12]
    return Path(tempfile.gettempdir()) / f"zcode-attach-rules-{repo_key}"


STATE_DIR = Path(os.environ.get("ZCODE_RULES_STATE_DIR") or default_state_dir(REPO_ROOT))

# Field names inside a ZCode tool payload that carry a file path. The matcher is
# intentionally permissive: it also walks nested structures, so a path nested
# deeper than these keys is still found. The known names keep the walk focused
# and avoid treating arbitrary string values as paths.
PATH_FIELDS = frozenset(
    {
        "file_path",
        "path",
        "notebook_path",
        "source",
        "destination",
        "new_path",
        "old_path",
        "old_string_path",
        "filePath",
        "fileName",
        "directory",
    }
)


class Rule:
    def __init__(self, path: Path, description: str, globs: list[str], always: bool, body: str):
        self.path = path
        self.description = description
        self.globs = globs
        self.always = always
        self.body = body

    @property
    def rel(self) -> str:
        return self.path.relative_to(REPO_ROOT).as_posix()


def glob_to_regex(pattern: str) -> re.Pattern[str]:
    """Translate a Cursor glob into a regex.

    `fnmatch` is unusable here because its `*` also crosses `/`, which makes
    `external/httpserver/**/*.go` miss `external/httpserver/server.go`.
    """
    out: list[str] = []
    i, n = 0, len(pattern)
    while i < n:
        if pattern.startswith("**/", i):
            out.append("(?:.*/)?")
            i += 3
        elif pattern.startswith("**", i):
            out.append(".*")
            i += 2
        elif pattern[i] == "*":
            out.append("[^/]*")
            i += 1
        elif pattern[i] == "?":
            out.append("[^/]")
            i += 1
        else:
            out.append(re.escape(pattern[i]))
            i += 1
    return re.compile("^" + "".join(out) + "$")


def strip_yaml_comment(value: str) -> str:
    quote = ""
    escaped = False
    for index, char in enumerate(value):
        if quote:
            if char == "\\" and not escaped:
                escaped = True
                continue
            if char == quote and not escaped:
                quote = ""
            escaped = False
            continue
        if char in ("'", '"'):
            quote = char
            continue
        if char == "#" and (index == 0 or value[index - 1].isspace()):
            return value[:index].strip()
    return value.strip()


def unquote(value: str) -> str:
    value = value.strip()
    if len(value) >= 2 and value[0] == '"' and value[-1] == '"':
        try:
            return json.loads(value)
        except json.JSONDecodeError:
            return value[1:-1]
    if len(value) >= 2 and value[0] == "'" and value[-1] == "'":
        return value[1:-1].replace("''", "'")
    return value


def split_globs(value: str) -> list[str]:
    value = strip_yaml_comment(value)
    value = value.removeprefix("[").removesuffix("]")
    out: list[str] = []
    current: list[str] = []
    quote = ""
    escaped = False
    brace_depth = 0

    def flush() -> None:
        item = unquote("".join(current))
        if item:
            out.append(item)
        current.clear()

    for char in value:
        if quote:
            current.append(char)
            if char == "\\" and quote == '"' and not escaped:
                escaped = True
                continue
            if char == quote and not escaped:
                quote = ""
            escaped = False
            continue
        if char in ("'", '"'):
            quote = char
            current.append(char)
        elif char == "{":
            brace_depth += 1
            current.append(char)
        elif char == "}":
            brace_depth = max(0, brace_depth - 1)
            current.append(char)
        elif char == "," and brace_depth == 0:
            flush()
        else:
            current.append(char)
    flush()
    return out


def parse_rule(path: Path) -> Rule | None:
    """Read one `.mdc` file. Frontmatter is flat, so no YAML dependency."""
    text = path.read_text(encoding="utf-8")
    if not text.startswith("---"):
        return None
    end = text.find("\n---", 3)
    if end < 0:
        return None
    head = text[3:end]
    body = text[end + 4 :].strip()

    description, globs, always = "", [], False
    lines = head.splitlines()
    index = 0
    while index < len(lines):
        line = lines[index]
        key, sep, value = line.partition(":")
        if not sep:
            index += 1
            continue
        key, value = key.strip(), value.strip()
        if key == "description":
            description = unquote(strip_yaml_comment(value))
        elif key == "globs":
            cleaned_value = strip_yaml_comment(value)
            if cleaned_value:
                globs = split_globs(cleaned_value)
            else:
                index += 1
                while index < len(lines):
                    item = lines[index].strip()
                    if not item or item.startswith("#"):
                        index += 1
                        continue
                    if not item.startswith("-"):
                        index -= 1
                        break
                    pattern = unquote(strip_yaml_comment(item[1:].strip()))
                    if pattern:
                        globs.append(pattern)
                    index += 1
        elif key == "alwaysApply":
            always = unquote(strip_yaml_comment(value)).lower() == "true"
        index += 1
    return Rule(path, description, globs, always, body)


def load_rules() -> list[Rule]:
    rules = []
    for path in sorted(RULES_DIR.glob("*.mdc")):
        rule = parse_rule(path)
        if rule and rule.body:
            rules.append(rule)
    return rules


def state_file(session_id: str) -> Path:
    safe = re.sub(r"[^A-Za-z0-9._-]", "_", session_id or "unknown")[:120]
    return STATE_DIR / f"{safe}.json"


def load_sent(session_id: str) -> set[str]:
    try:
        return set(json.loads(state_file(session_id).read_text(encoding="utf-8")))
    except Exception:
        return set()


def save_sent(session_id: str, sent: set[str]) -> bool:
    try:
        STATE_DIR.mkdir(parents=True, exist_ok=True)
        destination = state_file(session_id)
        with tempfile.NamedTemporaryFile(
            mode="w",
            encoding="utf-8",
            dir=STATE_DIR,
            prefix=destination.name + ".",
            suffix=".tmp",
            delete=False,
        ) as handle:
            json.dump(sorted(sent), handle)
            temporary = Path(handle.name)
        os.replace(temporary, destination)
        return True
    except Exception:
        return False


@contextmanager
def session_lock(session_id: str):
    STATE_DIR.mkdir(parents=True, exist_ok=True)
    lock_path = state_file(session_id).with_suffix(".lock")
    with lock_path.open("a+b") as handle:
        if os.name == "nt":
            handle.seek(0, os.SEEK_END)
            if handle.tell() == 0:
                handle.write(b"\0")
                handle.flush()
            handle.seek(0)
            msvcrt.locking(handle.fileno(), msvcrt.LK_LOCK, 1)
        else:
            fcntl.flock(handle.fileno(), fcntl.LOCK_EX)
        try:
            yield
        finally:
            handle.seek(0)
            if os.name == "nt":
                msvcrt.locking(handle.fileno(), msvcrt.LK_UNLCK, 1)
            else:
                fcntl.flock(handle.fileno(), fcntl.LOCK_UN)


def claim_rule_ids(session_id: str, rule_ids: set[str]) -> set[str]:
    try:
        with session_lock(session_id):
            sent = load_sent(session_id)
            claimed = rule_ids - sent
            if not claimed:
                return set()
            if not save_sent(session_id, sent | claimed):
                return claimed
            return claimed
    except Exception:
        return set(rule_ids)


def update_session_state(session_id: str, source: str, always: set[str]) -> None:
    try:
        with session_lock(session_id):
            sent = load_sent(session_id) if source == "resume" else set()
            save_sent(session_id, sent | always)
    except Exception:
        pass


def collect_target_paths(tool_input: object) -> list[str]:
    """Collect repo-relative file paths from a ZCode tool payload.

    ZCode hands the hook a JSON object whose fields depend on the tool
    (`file_path` for Edit/Write, `path`/`notebook_path`/... for others). We walk
    the structure and keep the string values found under known path-carrying
    keys, so an absolute or nested path is still matched once normalised.
    """
    found: list[str] = []

    def walk(node: object) -> None:
        if isinstance(node, dict):
            for key, value in node.items():
                if key in PATH_FIELDS and isinstance(value, str) and value:
                    found.append(value)
                else:
                    walk(value)
        elif isinstance(node, list):
            for item in node:
                walk(item)

    walk(tool_input)

    root = REPO_ROOT.resolve()
    rel: list[str] = []
    for raw in found:
        raw = raw.strip()
        if not raw or "\x00" in raw:
            continue
        path = Path(raw)
        candidate = path if path.is_absolute() else root / path
        try:
            relative = candidate.resolve(strict=False).relative_to(root)
        except (OSError, RuntimeError, ValueError):
            continue
        normalized = relative.as_posix()
        if relative.parts and normalized not in rel:
            rel.append(normalized)
    return rel


def render(rules: list[Rule], preamble: str) -> str:
    chunks = [preamble]
    for rule in rules:
        chunks.append(f"<!-- from {rule.rel} -->\n\n{rule.body}")
    return "\n\n".join(chunks)


def emit(event: str, context: str) -> None:
    json.dump(
        {"hookSpecificOutput": {"hookEventName": event, "additionalContext": context}},
        sys.stdout,
    )


def main() -> int:
    try:
        payload = json.load(sys.stdin)
    except Exception:
        return 0

    event = payload.get("hook_event_name") or os.environ.get("ZCODE_HOOK_EVENT", "")
    session_id = payload.get("session_id", "")

    try:
        rules = load_rules()
    except Exception:
        return 0
    if not rules:
        return 0

    if event == "SessionStart":
        source = payload.get("source", "")
        always = [r for r in rules if r.always]
        update_session_state(session_id, source, {r.rel for r in always})
        if not always:
            return 0
        emit(
            event,
            render(
                always,
                "Project rules for this repository, always in force."
                " The ZCode project hook attached them from `.cursor/rules/`;"
                " no separate ZCode copy is maintained. More rules are attached"
                " automatically when you edit files they cover.",
            ),
        )
        return 0

    if event != "PreToolUse":
        return 0

    paths = collect_target_paths(payload.get("tool_input"))
    if not paths:
        return 0

    candidates: list[Rule] = []
    for rule in rules:
        if rule.always or not rule.globs:
            continue
        patterns = [glob_to_regex(g) for g in rule.globs]
        if any(p.match(path) for path in paths for p in patterns):
            candidates.append(rule)

    claimed = claim_rule_ids(session_id, {rule.rel for rule in candidates})
    if not claimed:
        return 0

    matched = [rule for rule in candidates if rule.rel in claimed]
    touched = ", ".join(sorted(set(paths))[:8])
    emit(
        event,
        render(
            matched,
            f"Project rules that cover the files you are editing ({touched})."
            " Apply them to this change before continuing.",
        ),
    )
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Exception:
        # Fail open: never block an edit because of this hook.
        sys.exit(0)
