#!/usr/bin/env python3
"""Run one source-pinned Rust omission control without Cargo freshness guesses.

All owners sharing a target must use its lease. An additional process census
refuses pre-existing unleased Cargo/rustc jobs. Only this package is cleaned;
dependency artifacts remain warm. Tests execute an immutable copied ELF, never
the mutable target path. The recipe and every source byte are checked again
before results can be classified. No test or compiler failure is a causal pass.
"""

import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import shutil
import signal
import stat
import subprocess
import sys
import time
import tomllib


SCHEMA = "urnetwork-cargo-control-v1"
MAXIMUM_RECIPE_BYTES = 1024 * 1024
MAXIMUM_FILES = 256
MAXIMUM_FILE_BYTES = 8 * 1024 * 1024
MINIMUM_FREE_BYTES = 110 * 1024 ** 3


class Refused(Exception):
    """An unqualified harness outcome; it can never be an intended red."""


def require(condition, reason):
    if not condition:
        raise Refused(reason)


def digest(path):
    with Path(path).open("rb") as source:
        result = hashlib.file_digest(source, "sha256").hexdigest()
    return result


def read_recipe(path):
    raw = path.read_bytes()
    require(0 < len(raw) <= MAXIMUM_RECIPE_BYTES, "recipe byte bound")
    recipe = json.loads(raw)
    require(recipe.get("schema") == SCHEMA, "recipe schema differs")
    return recipe, hashlib.sha256(raw).hexdigest()


def census(root):
    """Bind the entire small crate, including untracked code/config additions."""
    result = {}
    require(root.is_absolute() and root.resolve() == root, "crate path aliases")
    for directory, children, names in os.walk(root, followlinks=False):
        children[:] = sorted(name for name in children if name not in (".git", "target"))
        for name in children:
            require(not (Path(directory) / name).is_symlink(), "crate directory alias")
        for name in sorted(names):
            if directory == str(root) and name == ".git":
                continue
            path = Path(directory) / name
            info = path.lstat()
            require(stat.S_ISREG(info.st_mode) and info.st_nlink == 1,
                    "crate file is not a unique regular file")
            require(info.st_size <= MAXIMUM_FILE_BYTES, "crate file byte bound")
            result[path.relative_to(root).as_posix()] = {
                "sha256": digest(path), "bytes": info.st_size,
            }
            require(len(result) <= MAXIMUM_FILES, "crate file count bound")
    require("Cargo.toml" in result and "Cargo.lock" in result, "crate manifest absent")
    return result


def validate_sources(recipe):
    baseline = Path(recipe["baseline_crate"])
    source = Path(recipe["source_crate"])
    require(baseline != source, "control must have its own physical crate")
    pinned = recipe["baseline_files"]
    require(census(baseline) == pinned, "frozen baseline crate changed")
    expected = dict(pinned)
    seen = set()
    for mutation in recipe["mutations"]:
        name = mutation["path"]
        require(name in pinned and name.startswith("src/") and name.endswith(".rs")
                and name not in seen, "control must name exact existing Rust sources")
        require(mutation["before"] == pinned[name]
                and mutation["after"] != pinned[name], "control mutation basis differs")
        seen.add(name)
        expected[name] = mutation["after"]
    require(0 < len(seen) <= 8, "control mutation count bound")
    require(census(source) == expected, "control has missing or undeclared source changes")
    package = tomllib.loads((source / "Cargo.toml").read_text())["package"]
    require(package["name"] == recipe["package"], "package identity differs")
    return source, expected


def active_target_processes(target):
    """Refuse existing target writers/readers; never stop another owner's job."""
    conflicts = []
    target_text = str(target)
    for item in Path("/proc").iterdir():
        if not item.name.isdigit() or int(item.name) == os.getpid():
            continue
        try:
            if item.stat().st_uid != os.getuid():
                continue
            args = (item / "cmdline").read_bytes().split(b"\0")
            if not args or not args[0]:
                continue
            command = os.fsdecode(args[0])
            values = [os.fsdecode(value) for value in args if value]
            relevant = Path(command).name in ("cargo", "rustc", "rustdoc")
            uses_target = any(target_text in value for value in values)
            if relevant and not uses_target:
                environment = (item / "environ").read_bytes().split(b"\0")
                uses_target = ("CARGO_TARGET_DIR=" + target_text).encode() in environment
                if not uses_target and Path(command).name == "cargo":
                    uses_target = (item / "cwd").resolve() / "target" == target
            if uses_target and (relevant or command.startswith(target_text + "/")):
                conflicts.append(int(item.name))
        except (FileNotFoundError, ProcessLookupError):
            continue
        except PermissionError as error:
            raise Refused("cannot inspect an owned target process") from error
    return conflicts


def run_process(args, cwd, environment, output, label, timeout):
    """Own and join the complete process group, including failed compilation."""
    stdout = output / (label + ".stdout")
    stderr = output / (label + ".stderr")
    with stdout.open("xb") as out, stderr.open("xb") as err:
        process = subprocess.Popen(args, cwd=cwd, env=environment, stdout=out,
                                   stderr=err, start_new_session=True)
        try:
            code = process.wait(timeout=timeout)
        except BaseException:
            try:
                os.killpg(process.pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                os.killpg(process.pid, signal.SIGKILL)
                process.wait()
            raise
    return {"argv": args, "exit": code, "stdout_sha256": digest(stdout),
            "stderr_sha256": digest(stderr)}


def fresh_artifact(path, package, crate):
    """Cargo's explicit root artifact is required, not elapsed time or mtimes."""
    selected = []
    for raw in path.read_bytes().splitlines():
        try:
            item = json.loads(raw)
        except (ValueError, UnicodeDecodeError):
            continue
        if item.get("reason") != "compiler-artifact":
            continue
        target = item.get("target", {})
        if target.get("name") == package.replace("-", "_") and item.get("profile", {}).get("test"):
            require(Path(target.get("src_path", "")).resolve() == crate / "src/lib.rs",
                    "compiler artifact points at another crate")
            require(item.get("fresh") is False, "Cargo reused a stale control artifact")
            require(item.get("executable"), "test artifact executable is absent")
            selected.append(item)
    require(len(selected) == 1, "exactly one newly compiled root test artifact is required")
    return selected[0]


def classify_test(code, stdout, stderr, selector, expected_assertion):
    """One selected assertion failure is distinct from setup, panic, or timeout."""
    text = stdout.read_text(errors="replace") + stderr.read_text(errors="replace")
    require("running 1 test" in text and f"test {selector} ... FAILED" in text,
            "control did not execute exactly the selected failed root")
    require(code == 101 and "test result: FAILED. 0 passed; 1 failed;" in text,
            "control exit/count differs from one intended behavioral failure")
    require(expected_assertion and expected_assertion in text,
            "control did not reach its exact intended assertion")


def run(recipe_path, output):
    recipe, recipe_sha = read_recipe(recipe_path)
    source, expected = validate_sources(recipe)
    target = Path(recipe["target_dir"])
    require(target.is_absolute() and target.resolve() == target and target.is_dir(),
            "target must be an existing exact owned directory")
    require(target.stat().st_uid == os.getuid(), "target has another owner")
    require(0 < recipe["compile_seconds"] <= 900 and 0 < recipe["test_seconds"] <= 900,
            "phase time budgets must be finite")
    require(1 <= recipe.get("jobs", 2) <= 2, "control compiler job bound")
    require(recipe.get("minimum_free_bytes", MINIMUM_FREE_BYTES) >= MINIMUM_FREE_BYTES,
            "control cannot lower the shared admission floor")
    for tool in ("cargo", "rustc"):
        tool_path = Path(recipe[tool]["path"])
        require(tool_path.is_absolute() and tool_path.resolve() == tool_path,
                tool + " must pin the actual toolchain executable, not a shim")
        require(digest(recipe[tool]["path"]) == recipe[tool]["sha256"],
                tool + " executable differs")
    require(shutil.disk_usage(target).free >= recipe.get("minimum_free_bytes", MINIMUM_FREE_BYTES),
            "target is below the admission floor")
    output.mkdir(mode=0o700, parents=False, exist_ok=False)
    receipt = {"schema": SCHEMA, "status": "UNQUALIFIED", "recipe_sha256": recipe_sha,
               "runner_sha256": digest(Path(__file__)), "source_files": expected,
               "started_unix": time.time(), "steps": []}
    lease = target / ".urnetwork-cargo-control.lease"
    descriptor = os.open(lease, os.O_RDWR | os.O_CREAT | os.O_NOFOLLOW, 0o600)
    try:
        fcntl.flock(descriptor, fcntl.LOCK_EX | fcntl.LOCK_NB)
        require(not active_target_processes(target), "target has an active Cargo/rustc/test owner")
        environment = os.environ.copy()
        environment.update(recipe.get("environment", {}))
        require(set(recipe.get("environment", {})) <= {
            "CARGO_HOME", "RUSTUP_HOME", "PATH", "CARGO_PROFILE_DEV_DEBUG",
            "CARGO_PROFILE_TEST_DEBUG", "TMPDIR",
        }, "unreviewed compiler environment field")
        environment.update({"RUSTC": recipe["rustc"]["path"], "CARGO_TARGET_DIR": str(target),
                            "CARGO_INCREMENTAL": "0", "CARGO_BUILD_JOBS": str(recipe.get("jobs", 2))})
        for key in ("RUSTFLAGS", "CARGO_ENCODED_RUSTFLAGS", "RUSTC_WRAPPER", "RUSTC_WORKSPACE_WRAPPER"):
            require(not environment.get(key), "ambient compiler override: " + key)
        cargo = recipe["cargo"]["path"]
        package_version = tomllib.loads((source / "Cargo.toml").read_text())["package"]["version"]
        clean = [cargo, "clean", "--locked", "--offline", "--package",
                 recipe["package"] + "@" + package_version,
                 "--target-dir", str(target)]
        receipt["steps"].append(run_process(clean, source, environment, output, "clean", 60))
        require(receipt["steps"][-1]["exit"] == 0, "package-only clean failed")
        compile_args = [cargo, "test", "--locked", "--offline", "--lib", "--no-run",
                        "--message-format=json", "--target-dir", str(target)]
        receipt["steps"].append(run_process(compile_args, source, environment, output,
                                            "compile", recipe["compile_seconds"]))
        require(receipt["steps"][-1]["exit"] == 0, "control did not compile")
        require(validate_sources(recipe)[1] == expected, "source changed during compilation")
        artifact = fresh_artifact(output / "compile.stdout", recipe["package"], source)
        executable = Path(artifact["executable"])
        require(executable.resolve().is_relative_to(target), "artifact escaped owned target")
        require(shutil.disk_usage(output).free - executable.stat().st_size >=
                recipe.get("minimum_free_bytes", MINIMUM_FREE_BYTES), "artifact retention crosses floor")
        retained = output / "control-test"
        shutil.copyfile(executable, retained)
        retained.chmod(0o500)
        executable_sha = digest(executable)
        require(digest(retained) == executable_sha, "artifact changed during retention")
        receipt["artifact"] = {"cargo": artifact, "path": str(retained), "sha256": executable_sha}
        test_args = [str(retained), "--exact", recipe["test"], "--nocapture", "--test-threads=1"]
        receipt["steps"].append(run_process(test_args, source, environment, output,
                                            "test", recipe["test_seconds"]))
        require(validate_sources(recipe)[1] == expected, "source changed during test")
        require(read_recipe(recipe_path)[1] == recipe_sha, "recipe changed during control")
        require(digest(retained) == executable_sha, "executed artifact changed")
        classify_test(receipt["steps"][-1]["exit"], output / "test.stdout", output / "test.stderr",
                      recipe["test"], recipe["expected_assertion"])
        receipt["status"] = "EXPECTED_BEHAVIORAL_FAILURE"
    except BaseException as error:
        receipt["error"] = str(error)
        raise
    finally:
        receipt["finished_unix"] = time.time()
        (output / "receipt.json").write_text(json.dumps(receipt, indent=2, sort_keys=True) + "\n")
        os.close(descriptor)


def main():
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("recipe", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    try:
        run(args.recipe.resolve(), args.output.absolute())
    except (Refused, OSError, ValueError, KeyError, subprocess.TimeoutExpired) as error:
        print("Cargo control UNQUALIFIED:", error, file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
