import contextlib
import importlib.util
import io
import json
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
CODEX = ROOT / ".codex/hooks/attach_rules.py"
ZCODE = ROOT / ".zcode/hooks/attach_rules.py"


def load_module(name: str, path: Path):
    spec = importlib.util.spec_from_file_location(name, path)
    if spec is None or spec.loader is None:
        raise AssertionError(f"cannot load {path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def list_rule(directory: Path) -> Path:
    path = directory / "provider-proxy.mdc"
    path.write_text(
        "---\n"
        "description: Provider proxy\n"
        "globs:\n"
        "  - \"internal/llm/**/*.go\" # provider core\n"
        "  # provider commands\n"
        "\n"
        "  - cmd/coddy/providers.go # sign-in commands\n"
        "alwaysApply: false\n"
        "---\n\n"
        "Every provider request follows its proxy.\n",
        encoding="utf-8",
    )
    return path


def always_rule(directory: Path) -> Path:
    path = directory / "always.mdc"
    path.write_text(
        "---\n"
        "description: Always-on rule\n"
        "alwaysApply: true\n"
        "---\n\n"
        "Always follow the repository workflow.\n",
        encoding="utf-8",
    )
    return path


def run_hook_optional(module, rules_dir: Path, state_dir: Path, payload: dict) -> str | None:
    module.RULES_DIR = rules_dir
    module.STATE_DIR = state_dir
    payload.setdefault("session_id", "provider-proxy-case")
    old_stdin = sys.stdin
    output = io.StringIO()
    try:
        sys.stdin = io.StringIO(json.dumps(payload))
        with contextlib.redirect_stdout(output):
            assert module.main() == 0
    finally:
        sys.stdin = old_stdin
    raw = output.getvalue()
    if not raw:
        return None
    rendered = json.loads(raw)
    return rendered["hookSpecificOutput"]["additionalContext"]


def run_hook(module, rules_dir: Path, state_dir: Path, payload: dict) -> str:
    context = run_hook_optional(module, rules_dir, state_dir, payload)
    if context is None:
        raise AssertionError("hook emitted no context")
    return context


def run_pretool(module, rules_dir: Path, state_dir: Path, tool_input: dict) -> str:
    return run_hook(
        module,
        rules_dir,
        state_dir,
        {"hook_event_name": "PreToolUse", "tool_input": tool_input},
    )


class AdapterContractTest(unittest.TestCase):
    def test_python_adapters_parse_yaml_list_globs(self):
        codex = load_module("codex_rules", CODEX)
        zcode = load_module("zcode_rules", ZCODE)
        with tempfile.TemporaryDirectory() as tmp:
            path = list_rule(Path(tmp))
            expected = ["internal/llm/**/*.go", "cmd/coddy/providers.go"]
            self.assertEqual(expected, codex.parse_rule(path).globs)
            self.assertEqual(expected, zcode.parse_rule(path).globs)

    def test_provider_proxy_rule_is_emitted_by_python_adapters(self):
        codex = load_module("codex_emit", CODEX)
        zcode = load_module("zcode_emit", ZCODE)
        with tempfile.TemporaryDirectory(dir=ROOT) as tmp:
            root = Path(tmp)
            rules_dir = root / "rules"
            rules_dir.mkdir()
            list_rule(rules_dir)
            for name, module in (("codex", codex), ("zcode", zcode)):
                context = run_pretool(
                    module,
                    rules_dir,
                    root / f"state-{name}",
                    {"file_path": "internal/llm/openai.go"},
                )
                self.assertIn("Every provider request follows its proxy.", context)

    def test_session_start_preamble_is_grammatical(self):
        codex = load_module("codex_preamble", CODEX)
        zcode = load_module("zcode_preamble", ZCODE)
        with tempfile.TemporaryDirectory(dir=ROOT) as tmp:
            root = Path(tmp)
            rules_dir = root / "rules"
            rules_dir.mkdir()
            always_rule(rules_dir)
            for name, module in (("Codex", codex), ("ZCode", zcode)):
                context = run_hook(
                    module,
                    rules_dir,
                    root / f"state-{name.lower()}",
                    {"hook_event_name": "SessionStart", "source": "startup"},
                )
                self.assertNotIn("They are The", context)
                self.assertIn(f"The {name} project hook attached them", context)

    def test_session_start_sources_control_scoped_deduplication(self):
        codex = load_module("codex_lifecycle", CODEX)
        zcode = load_module("zcode_lifecycle", ZCODE)
        with tempfile.TemporaryDirectory(dir=ROOT) as tmp:
            root = Path(tmp)
            rules_dir = root / "rules"
            rules_dir.mkdir()
            always_rule(rules_dir)
            list_rule(rules_dir)
            tool_input = {"file_path": "internal/llm/openai.go"}
            for name, module in (("codex", codex), ("zcode", zcode)):
                state_dir = root / f"state-{name}"
                self.assertIn("Every provider request", run_pretool(module, rules_dir, state_dir, tool_input))

                run_hook(
                    module,
                    rules_dir,
                    state_dir,
                    {"hook_event_name": "SessionStart", "source": "resume"},
                )
                self.assertIsNone(
                    run_hook_optional(
                        module,
                        rules_dir,
                        state_dir,
                        {"hook_event_name": "PreToolUse", "tool_input": tool_input},
                    )
                )

                for source in ("compact", "clear", "startup"):
                    run_hook(
                        module,
                        rules_dir,
                        state_dir,
                        {"hook_event_name": "SessionStart", "source": source},
                    )
                    self.assertIn(
                        "Every provider request",
                        run_pretool(module, rules_dir, state_dir, tool_input),
                    )

    def test_zcode_default_state_dir_is_repository_scoped(self):
        zcode = load_module("zcode_state_dir", ZCODE)
        first = zcode.default_state_dir(ROOT / "first")
        second = zcode.default_state_dir(ROOT / "second")
        self.assertNotEqual(first, second)
        self.assertTrue(first.name.startswith("zcode-attach-rules-"))
        self.assertTrue(second.name.startswith("zcode-attach-rules-"))

    def test_codex_extracts_structured_edit_paths(self):
        codex = load_module("codex_paths", CODEX)
        payload = {
            "file_path": "internal/llm/openai.go",
            "destination": "internal/llm/openai_new.go",
            "nested": {"path": "cmd/coddy/providers.go"},
        }
        self.assertEqual(
            [
                "internal/llm/openai.go",
                "internal/llm/openai_new.go",
                "cmd/coddy/providers.go",
            ],
            codex.patched_paths(payload),
        )

    def test_python_adapters_reject_paths_outside_repository(self):
        codex = load_module("codex_containment", CODEX)
        zcode = load_module("zcode_containment", ZCODE)
        inside = str(ROOT / "internal" / "llm" / "openai.go")
        escaped = str(ROOT / ".." / "outside.go")
        payload = {
            "file_path": inside,
            "destination": escaped,
            "source": "internal/llm/\x00bad.go",
        }
        expected = ["internal/llm/openai.go"]
        self.assertEqual(expected, codex.patched_paths(payload))
        self.assertEqual(expected, zcode.collect_target_paths(payload))

    def test_adapters_do_not_claim_vendor_policy_ownership(self):
        for path in (CODEX, ZCODE):
            self.assertNotIn("single source of truth", path.read_text())

    def test_no_codex_manual_index(self):
        self.assertFalse((ROOT / ".codex/rules.md").exists())


if __name__ == "__main__":
    unittest.main()
