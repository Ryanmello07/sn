"""Deterministic guard controls; these do not launch or clean real Cargo jobs."""

import importlib.util
import json
from pathlib import Path
import tempfile
import unittest


spec = importlib.util.spec_from_file_location("cargo_control", Path(__file__).with_name("cargo_control.py"))
guard = importlib.util.module_from_spec(spec)
spec.loader.exec_module(guard)


class CargoControlTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name).resolve()
        self.baseline, self.source = self.root / "baseline", self.root / "control"
        for directory in (self.baseline, self.source):
            (directory / "src").mkdir(parents=True)
            (directory / "Cargo.toml").write_text('[package]\nname="example-probe"\nversion="0.1.0"\n')
            (directory / "Cargo.lock").write_text('version=4\n')
            (directory / "src/lib.rs").write_text("fn admitted() -> bool { true }\n")
        before = guard.census(self.baseline)
        (self.source / "src/lib.rs").write_text("fn admitted() -> bool { false }\n")
        after = guard.census(self.source)
        self.recipe = {"baseline_crate": str(self.baseline), "source_crate": str(self.source),
                       "baseline_files": before, "package": "example-probe", "mutations": [
                           {"path": "src/lib.rs", "before": before["src/lib.rs"],
                            "after": after["src/lib.rs"]}]}

    def artifact(self, fresh):
        path = self.root / "compiler.jsonl"
        path.write_text(json.dumps({"reason": "compiler-artifact", "fresh": fresh,
            "profile": {"test": True}, "target": {"name": "example_probe",
            "src_path": str(self.source / "src/lib.rs")}, "executable": "/retained/example"}) + "\n")
        return path

    def test_same_package_old_mtime_stale_artifact_is_not_a_control(self):
        guard.validate_sources(self.recipe)
        with self.assertRaisesRegex(guard.Refused, "stale control artifact"):
            guard.fresh_artifact(self.artifact(True), "example-probe", self.source)
        accepted = guard.fresh_artifact(self.artifact(False), "example-probe", self.source)
        self.assertIs(accepted["fresh"], False)

    def test_compiler_artifact_must_name_this_physical_crate(self):
        with self.assertRaisesRegex(guard.Refused, "another crate"):
            guard.fresh_artifact(self.artifact(False), "example-probe", self.baseline)

    def test_undeclared_source_and_dependency_changes_are_refused(self):
        guard.validate_sources(self.recipe)
        (self.source / "src/extra.rs").write_text("// unbound source\n")
        with self.assertRaisesRegex(guard.Refused, "undeclared"):
            guard.validate_sources(self.recipe)
        (self.source / "src/extra.rs").unlink()
        (self.source / "Cargo.lock").write_text('version=3\n')
        with self.assertRaisesRegex(guard.Refused, "undeclared"):
            guard.validate_sources(self.recipe)

    def test_mutating_frozen_baseline_does_not_relabel_the_control(self):
        (self.baseline / "src/lib.rs").write_text("// changed original\n")
        with self.assertRaisesRegex(guard.Refused, "baseline crate changed"):
            guard.validate_sources(self.recipe)

    def test_exact_assertion_and_count_required_even_with_exit101(self):
        stdout, stderr = self.root / "stdout", self.root / "stderr"
        selector = "historical::tests::exact_control"
        stdout.write_text(f"running 1 test\ntest {selector} ... FAILED\n"
                          "test result: FAILED. 0 passed; 1 failed; 0 ignored;\n")
        stderr.write_text("assertion failed: unproved write was accepted\n")
        guard.classify_test(101, stdout, stderr, selector, "unproved write was accepted")
        with self.assertRaisesRegex(guard.Refused, "exact intended assertion"):
            guard.classify_test(101, stdout, stderr, selector, "a different failure")
        with self.assertRaisesRegex(guard.Refused, "exit/count"):
            guard.classify_test(1, stdout, stderr, selector, "unproved write was accepted")

    def test_cargo_build_failure_and_filtered_zero_tests_are_not_causality(self):
        stdout, stderr = self.root / "stdout", self.root / "stderr"
        for output in ("error[E0425]: missing source item", "running 0 tests\n0 passed; 0 failed"):
            stdout.write_text(output)
            stderr.write_text("expected message from compiler context")
            with self.assertRaisesRegex(guard.Refused, "exactly the selected failed root"):
                guard.classify_test(101, stdout, stderr, "historical::tests::exact_control", "expected message")


if __name__ == "__main__":
    unittest.main()
