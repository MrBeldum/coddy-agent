import contextlib
import importlib.util
import io
import json
import shutil
import subprocess
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
        "globs: # provider paths\n"
        "  - \"internal/llm/**/*.go\" # provider core\n"
        "  # provider commands\n"
        "\n"
        "  - cmd/coddy/providers.go # sign-in commands\n"
        "  - 'fixtures/foo\\' # terminal backslash\n"
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


def flow_rule(directory: Path) -> Path:
    path = directory / "flow.mdc"
    path.write_text(
        "---\n"
        "description: \"Flow rule\" # display text\n"
        "globs: [\"fixtures/foo,bar.go\", 'fixtures/it''s.go', \"\\u0069nternal/**/*.go\"] # scoped paths\n"
        "alwaysApply: true # required\n"
        "---\n\n"
        "Flow rule body.\n",
        encoding="utf-8",
    )
    return path


def multiline_flow_rule(directory: Path) -> Path:
    path = directory / "multiline-flow.mdc"
    path.write_text(
        "---\n"
        "description: Multiline flow\n"
        "globs: [\n"
        "  \"internal/llm/**/*.go\",\n"
        "  \"cmd/coddy/providers.go\"\n"
        "]\n"
        "alwaysApply: false\n"
        "---\n\n"
        "Multiline flow body.\n",
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


def run_concurrent_claims(source: Path, host_dir: str) -> list[list[str]]:
    with tempfile.TemporaryDirectory() as tmp:
        root = Path(tmp)
        script = root / host_dir / "hooks" / "attach_rules.py"
        script.parent.mkdir(parents=True)
        shutil.copy2(source, script)
        start = root / "start"
        code = (
            "import importlib.util,json,time\n"
            "from pathlib import Path\n"
            f"spec=importlib.util.spec_from_file_location('adapter', {str(script)!r})\n"
            "module=importlib.util.module_from_spec(spec)\n"
            "spec.loader.exec_module(module)\n"
            f"start=Path({str(start)!r})\n"
            "while not start.exists(): time.sleep(0.001)\n"
            "print(json.dumps(sorted(module.claim_rule_ids('shared-session', {'provider-proxy.mdc'}))))\n"
        )
        processes = [
            subprocess.Popen(
                [sys.executable, "-c", code],
                cwd=root,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
            )
            for _ in range(12)
        ]
        start.touch()
        results = []
        errors = []
        for process in processes:
            stdout, stderr = process.communicate(timeout=15)
            if process.returncode != 0:
                errors.append(stderr)
                continue
            results.append(json.loads(stdout))
        if errors:
            raise AssertionError("\n".join(errors))
        return results


class AdapterContractTest(unittest.TestCase):
    def test_python_adapters_parse_yaml_list_globs(self):
        codex = load_module("codex_rules", CODEX)
        zcode = load_module("zcode_rules", ZCODE)
        with tempfile.TemporaryDirectory() as tmp:
            path = list_rule(Path(tmp))
            expected = [
                "internal/llm/**/*.go",
                "cmd/coddy/providers.go",
                "fixtures/foo\\",
            ]
            self.assertEqual(expected, codex.parse_rule(path).globs)
            self.assertEqual(expected, zcode.parse_rule(path).globs)

    def test_python_adapters_parse_flow_yaml_scalars(self):
        codex = load_module("codex_flow", CODEX)
        zcode = load_module("zcode_flow", ZCODE)
        with tempfile.TemporaryDirectory() as tmp:
            path = flow_rule(Path(tmp))
            for module in (codex, zcode):
                rule = module.parse_rule(path)
                self.assertEqual("Flow rule", rule.description)
                self.assertEqual(
                    ["fixtures/foo,bar.go", "fixtures/it's.go", "internal/**/*.go"],
                    rule.globs,
                )
                self.assertTrue(rule.always)
                multiline = module.parse_rule(multiline_flow_rule(Path(tmp)))
                self.assertEqual(
                    ["internal/llm/**/*.go", "cmd/coddy/providers.go"],
                    multiline.globs,
                )
                self.assertFalse(multiline.always)

    def test_python_adapter_claims_are_interprocess_safe(self):
        for source, host_dir in ((CODEX, ".codex"), (ZCODE, ".zcode")):
            results = run_concurrent_claims(source, host_dir)
            claimed = [result for result in results if result]
            self.assertEqual([["provider-proxy.mdc"]], claimed)

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

    def test_failed_emit_does_not_claim_rule(self):
        def broken_emit(_event, _context):
            raise BrokenPipeError("host closed stdout")

        for source, name in ((CODEX, "codex_emit_failure"), (ZCODE, "zcode_emit_failure")):
            module = load_module(name, source)
            with tempfile.TemporaryDirectory(dir=ROOT) as tmp:
                root = Path(tmp)
                rules_dir = root / "rules"
                rules_dir.mkdir()
                list_rule(rules_dir)
                state_dir = root / "state"
                original_emit = module.emit
                module.emit = broken_emit
                with self.assertRaises(BrokenPipeError):
                    run_pretool(
                        module,
                        rules_dir,
                        state_dir,
                        {"file_path": "internal/llm/openai.go"},
                    )
                module.emit = original_emit
                self.assertIn(
                    "Every provider request",
                    run_pretool(
                        module,
                        rules_dir,
                        state_dir,
                        {"file_path": "internal/llm/openai.go"},
                    ),
                )

    def test_state_failures_degrade_to_duplicate_delivery(self):
        rule_ids = {"provider-proxy.mdc"}

        @contextlib.contextmanager
        def broken_lock(_session_id):
            raise OSError("state directory unavailable")
            yield

        for source, name in ((CODEX, "codex_state_failure"), (ZCODE, "zcode_state_failure")):
            save_failure = load_module(name + "_save", source)
            save_failure.save_sent = lambda _session_id, _sent: False
            self.assertEqual(rule_ids, save_failure.claim_rule_ids("session", rule_ids))

            lock_failure = load_module(name + "_lock", source)
            lock_failure.session_lock = broken_lock
            self.assertEqual(rule_ids, lock_failure.claim_rule_ids("session", rule_ids))
            lock_failure.update_session_state("session", "startup", {"always.mdc"})

    def test_adapters_do_not_claim_vendor_policy_ownership(self):
        for path in (CODEX, ZCODE):
            self.assertNotIn("single source of truth", path.read_text())

    def test_no_codex_manual_index(self):
        self.assertFalse((ROOT / ".codex/rules.md").exists())


if __name__ == "__main__":
    unittest.main()
