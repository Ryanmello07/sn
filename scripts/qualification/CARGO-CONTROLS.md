# Cargo omission controls

Copied crates with the same package name/version and old file mtimes can reuse
the same root test executable in a shared target. Different source hashes alone
do not prove that Cargo compiled those bytes. Retain such attempts as unqualified
harness outcomes, including apparently passing controls.

`cargo_control.py RECIPE OUTPUT` performs one intended failing control. OUTPUT
must be a fresh directory beneath an existing private qualification root. All
Cargo invocations that share the target must respect its
`.urnetwork-cargo-control.lease`; the runner also refuses already active target
processes. It cleans only the named root package with `cargo clean --package`,
preserving dependencies, and requires Cargo's JSON artifact to say `fresh:false`
and identify the exact physical root crate. It copies the ELF into OUTPUT,
checks its SHA-256, executes that copy, and checks source/recipe/ELF again. The
retained test binary is part of the receipt and must not be edited or reused as
a future writer. No package-wide or target-wide clean is used.

The recipe schema is `urnetwork-cargo-control-v1` and contains:

- `baseline_crate`, `source_crate`, `target_dir`: exact absolute physical paths.
- `baseline_files`: the complete `census(baseline)` mapping of crate-relative
  names to `{sha256,bytes}`, authenticated against the frozen source intake.
- `mutations`: one to eight `{path,before,after}` entries for existing `src/*.rs`
  files. Each before/after value has the same `{sha256,bytes}` shape. Extra files,
  hidden dependency/configuration changes and mutable baseline bytes refuse.
- `package`, `test`, `expected_assertion`: exact package, full libtest root name,
  and a distinctive behavioral assertion. The baseline test must independently
  pass. A compiler failure, zero tests, timeout, or another panic cannot qualify.
- `cargo` and `rustc`: `{path,sha256}` for the actual resolved toolchain binaries,
  not rustup shims; `compile_seconds`, `test_seconds` in 1–900, `jobs` in 1–2.
- Optional `environment`: only `CARGO_HOME`, `RUSTUP_HOME`, `PATH`,
  `CARGO_PROFILE_DEV_DEBUG`, `CARGO_PROFILE_TEST_DEBUG`, `TMPDIR`. Compiler
  wrappers or ambient Rust flags refuse. `CARGO_INCREMENTAL=0` is enforced.
- Optional `minimum_free_bytes` (default 110 GiB): checked before compilation
  and before retaining the ELF. Qualification owners may require a higher floor.

Freeze and hash the complete recipe before execution. Bind its baseline census
to the existing Git/source receipt, not merely to a fresh mutable directory.
The runner receipt records the recipe/runner/source hashes, all commands/logs,
Cargo artifact, retained ELF and final classification. A failed guard emits an
`UNQUALIFIED` receipt where the owned output directory has been created. Normal
product tests and independent qualifications remain distinct scopes.

The small guard tests run without Cargo or dependency compilation:

```
python3 -B scripts/qualification/cargo_control_test.py
```
