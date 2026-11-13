# P1 Continuous Qualification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make repository-required testing and final-artifact installation/upgrade qualification repeatable and fail closed in local runs and a six-target native CI matrix.

**Architecture:** Standard-library Python separates pure evidence/policy checks, bundle build/verification/atomic installation, native delivery scenarios, and command orchestration. Bash entry points remain. CI builds one candidate bundle and one pinned historical bundle, then each native job tests those exact bytes and uploads source/artifact-bound evidence for a fixed aggregate gate.

**Tech Stack:** Go 1.27.3; Python 3.14.8 stdlib; Node 26.13.0; Bash/Git; GitHub Actions pinned by full SHA.

**Spec:** `docs/superpowers/specs/2026-11-12-p1-continuous-qualification-design.md`

## Global Constraints

- User requested implementation and full tests, without further questions. Execute inline; one independent whole-branch review after implementation. Builder check returned `nothing important left`.
- Branch `feat/p1-continuous-qualification`, baseline `ccca3222f05e5732f7c5a602e0384ac35c444f2f`; never commit/push/publish/change remote settings in this request.
- Preserve P0 contracts/receipts and the 13 synchronized formal adapter resources, ignored existing bin/dist, and all previous evidence.
- Six release/native targets: linux_amd64, linux_arm64, darwin_amd64, darwin_arm64, windows_amd64, windows_arm64. Linux executes race, all platforms full/vet/Python/smoke/install/upgrade.
- Pinned actual historical upgrade source: `c50621e0191c51c38573e183c81ab9a193bceb7f`.
- New output directories must not overwrite anything. Installation is staged, version-checked and atomically replaced inside the requested prefix; tests use private prefixes only.
- No pip/npm dependencies; Python reads untrusted bundle data using `-I -B`. Do not execute code or build from downloaded artifact directories.
- Native/cross-compile/remote-protection results remain distinct. Local uncommitted candidate tests are not clean release certification.
- All commands/logs/RED→GREEN evidence stay under `.superpowers/sdd/2026-11-12-p1-continuous-qualification/`; do not delete this workspace because no commits are authorized.

## Review Focus

1. Reruns and partial matrices: the latest report for every target must bind the same current candidate; missing/failed/skipped dependencies cannot turn green. Task 1 adds aggregate behavioral cases.
2. Bundle path/member/type/size and checksum ambiguities: reject malformed input before any binary executes or installed bytes change. Task 2 adds corrupt/member/duplicate-JSON cases.
3. Atomic installation interruption and Windows replacement refusal: preserve old binary/data and permit a clean retry without deleting someone else's files. Task 3 adds actual interrupted-worker and injected replacement failure cases.
4. Tool/platform capability omissions and subprocess race diagnostics hidden by negative commands: fail even when a parent command returns zero. Tasks 1/4 add skip and captured-race-file cases.
5. Coverage profile repeated blocks and platform differences: merge duplicate ranges, compare exact fractions against an actual same-platform base, and reject unknown/uncovered inventories. Tasks 1/4 add repeated-block and differing-denominator cases.

## File Map

- `scripts/quality-policy.json`: required identities, approved skips, tool versions, targets, historical baseline and Linux coverage floors.
- `scripts/quality_policy.py`: pure strict JSON/Go event/coverage/aggregate/source-snapshot validation.
- `scripts/quality_run.py`: shared explicit-argv execution, timeout, durable logs and race-diagnostic validation (factored during Task 3).
- `scripts/quality_process.py`: private inherited process-tree registry; workers register before exec and timeout cancellation covers nested sessions, shared by delivery and Runner.
- `scripts/quality.py`: bundle/native/extended/Python/aggregate orchestration.
- `scripts/delivery.py`: build/verify/install CLI and bundle/atomic-install functions.
- `scripts/qualify_delivery.py`: real old-bundle→candidate installation and project upgrade scenarios, three smokes against installed candidate.
- `scripts/tests/test_quality.py`, `scripts/tests/test_delivery.py`: behavioral regressions, synthetic policy fixtures and real private filesystem/process tests.
- `internal/scaffold/{race_on,race_off,race_binary}_test.go`, modified `session_scripts_test.go`: race inheritance for separately built CLI.
- `internal/lockfile/qualification_test.go`: deterministic pre-cancelled lock non-execution/recovery.
- `internal/quality/fuzz_test.go`: bounded reproducible fuzz properties.
- `.github/workflows/quality.yml`: immutable artifact production, six native jobs, extended job, fixed `P1 required` aggregate.
- `README.md`, `docs/quality.md`: local commands, artifact/source installation, gate boundaries and remote-protection follow-up.

### Task 1: Mandatory evidence policy and race-aware CLI tests

**Interfaces:**
- Produces `GateError(ValueError)`, `require(condition, message)`, `strict_json(data: bytes) -> object`.
- Produces `source_snapshot(root: Path) -> dict` with `commit`, `dirty`, `fingerprint`, canonical source/index inventory; later tasks compare before/after.
- Produces `evaluate_go(data: str, packages: dict[str,bool], target: str, policy: dict, required: list[str] | None = None) -> dict` with pass/skip/fail counts and test statuses.
- Produces `coverage(path: Path) -> dict` (`covered`, `total`) and `check_coverage(candidate: dict, base: dict, floor: dict | None) -> None`.
- Produces `aggregate(reports: list[dict], commit: str, bundle_hash: str, policy: dict, needs: dict, event: str) -> dict`.

- [x] Write `test_required_go_missing_skipped_or_incomplete_is_blocked`, `test_go_duplicate_cached_or_failed_events_are_blocked`, `test_only_explicit_platform_skip_is_allowed`, `test_coverage_merges_duplicate_ranges_and_compares_fractions`, `test_aggregate_requires_latest_complete_bound_matrix` first. Assertions use hand-written event/report fixtures: unknown skip/nonzero/zero/incomplete/failed/missing binding must raise GateError; complete exact set returns PASS.
- [x] Run `python3 -I -B -m unittest discover -s scripts/tests -p test_quality.py -v`.
  Expected: assertion failures for absent new policy implementation, not test import/syntax errors. Record RED.
- [x] Implement pure policy module and JSON policy; use exact existing test identities, explicit target/reason skip allowlist, nonempty complete Go/package terminals and fresh execution. Merge repeated coverage blocks by file/range, require consistent statement weights and compare integer cross-products. Linux amd64 floor root `3344/4127`, example `86/119`; current root repeat showed `3344` or `3345` due `internal/lockfile/lock.go:42` cancellation branch.
- [x] Add build-tag race constants and `TestSessionScriptBinaryInheritsRaceInstrumentation`, reading `debug/buildinfo` of actual `sessionTestBin`; under `go test -race`, existing uninstrumented TestMain must fail this assertion. Run `go test -race -count=1 -run '^TestSessionScriptBinaryInheritsRaceInstrumentation$' ./internal/scaffold` and save RED.
- [x] Modify only TestMain's separate build argv to include `-race` when the test executable has the race tag. Add deterministic `TestLock_PreCancelledNeverRunsAndCanRetry` (cancel context first, callback must not execute, errors.Is for ErrTimeout/context.Canceled, fresh context can acquire same key).
- [x] Add `FuzzAcceptanceBlock`: encode arbitrary payload as valid YAML, choose a fence longer than any backtick run and compare with unfenced YAML decoding; a second same-tag fence is rejected; arbitrary raw input never panics. Seeds include Chinese/FR/SC/backtick/newline data. Follow-up fuzz exposed final-newline loss in Block: three hand-derived literal/folded scalar regressions went RED, `internal/quality/contract.go` now preserves the captured line terminator and regressions went GREEN. Keep both discovered corpus inputs in `internal/quality/testdata/fuzz/FuzzAcceptanceBlock/`; the differential oracle does not certify the third-party YAML encoder's roundtrip.
- [x] Run focused Python/Go tests, then root and example full tests; complete with `python3 -I -B -m unittest discover -s scripts/tests -v` plus root/example Go tests. Expected: all green. Record no commit (authorization boundary).

### Task 2: Immutable final release bundles

**Interfaces:**
- Consumes Task 1 source_snapshot, strict_json and GateError.
- Produces `build_bundle(source: Path, output: Path, version: str) -> dict`.
- Produces `verify_bundle(bundle: Path) -> dict`; validates all six archives without execution.
- Produces `binary_bytes(bundle: Path, manifest: dict, target: str) -> bytes` and `native_target() -> str`.
- CLI `python3 -I -B scripts/delivery.py build --source ROOT --output NEW --version VERSION [--dry-run]`; `verify --bundle DIR`.

- [x] Write release regressions in test_delivery first: existing output/sentinel survives; unknown options fail; dry-run has no filesystem effects; complete six-target bundle validates; truncated/checksum/member traversal/link/wrong target/duplicate JSON/version injection rejects. Use private source/outputs and real Go build for positive artifacts; synthetic modified archives are negative fixtures only.
- [x] Run focused tests and observe missing feature/current destructive release or argument behavior RED. Never run original release.sh in the actual root dist.
- [x] Implement build and verify: version limited to `[A-Za-z0-9][A-Za-z0-9._+-]{0,99}`; six actual `CGO_ENABLED=0 go build -trimpath` outputs with linker version stamp. Record actual Go tool/version/source snapshot. Bundle manifest `mulix.release.v1` and exact checksums. Archive contains only one regular executable, correct format/architecture, bounded size; stage and publish only into a new reserved output, manifest/checksums written last. On failure no prior content is replaced.
- [x] Replace release Bash body with Python 3 discovery and CLI forwarding, preserving `MULIX_VERSION`/git-describe default and supporting `--output`; default `dist/mulix_<version>`. Dry-run prints a non-qualified plan only.
- [x] Re-run delivery and all Python tests, root/example full tests; expected all green. Preserve real candidate bundle for later task use; do not treat mocked/corrupted fixtures as qualification.

### Task 3: Atomic source/artifact installation and actual historical upgrade

**Interfaces:**
- Consumes Task 2 verify_bundle/binary_bytes/native_target and Task 1 source binding.
- Produces `install_bundle(bundle: Path, prefix: Path, expected_version: str | None = None) -> Path`.
- Produces source installation using same atomic stage/version/replace primitive.
- CLI `delivery.py install --bundle DIR --prefix PREFIX [--version EXPECTED]`, mutually exclusive `--source ROOT`; omitted source retains existing remote-source behavior only when installer explicitly invoked.
- Produces `qualify_delivery.qualify(bundle: Path, baseline: Path, output: Path, target: str) -> dict` with DELIVER-01/02 and candidate/baseline source/version/hash bindings. `quality.py` invokes its CLI with explicit paths, not a replacement build.

- [x] Write installer regressions first: unknown argument nonzero; no Git/Go needed for bundle mode; bad hash/wrong version preserves existing binary and unrelated files; OS replacement error preserves original; interruption after real staging but before publication preserves old binary and clean retry succeeds. Test-owned worker may inject a wait at version syscall, then is killed; no production test hook. Keep known orphan staging fixture distinguishable and do not claim automatic cleanup of unowned files.
- [x] Run regressions, observe old install.sh unknown option returns success / artifact mode absent and new install API absent RED.
- [x] Implement same-filesystem unique temporary staging (Windows `.exe` suffix), ordinary-file checks, expected actual CLI `mulix <version>` verification, flush/fsync/atomic replace and only-own-temp cleanup. No destination deletion before replace; catch errors with nonzero CLI exit. Preserve source --version/--prefix/--source interface and document version override behavior.
- [x] Implement real DELIVER-02 with baseline `c50621e`: install old bundle, initialize private DSH project, preserve an independent user-data file, modify the old spec template at a genuinely changed upstream section, attempt damaged candidate install (must preserve old), install correct candidate, run update to observe conflict without changing local content, reject invalid acceptance, explicitly resolve via existing `update --accept REL`, retry update and verify data/baseline. Use real old/new binaries, not fabricated install metadata.
- [x] Run actual `smoke-e2e.sh`, `smoke-sessions.sh`, `smoke-update.sh` with explicit same final installed candidate; assert candidate bytes unchanged. Record stdout/stderr/exit codes and source/bundle snapshots.
- [x] Run Python and root/example full tests; expected all green. No actual user installation.

### Task 4: Native orchestrator, required CI gate, extended tests and docs

**Interfaces:**
- Consumes all Task 1-3 outputs.
- CLI `quality.py native --output NEW --bundle DIR --baseline-bundle DIR --target TARGET --base-ref FULL_SHA [--ci]`.
- CLI `quality.py extended --output NEW`; `quality.py aggregate --reports DIR --commit SHA --bundle-hash HASH --event EVENT` consumes dependency JSON from explicit environment (not shell interpolation).
- Native report `mulix.qualification.v1`: status, target, attempt, commit, bundle manifest hash, source before/after, commands with real exits, Go statuses/skips/coverage, delivery results, tool versions, local/CI qualification boundary.

- [x] Write orchestrator behavioral tests for hidden race log despite zero exit, required missing tool, source/bundle drift, failed command/timeout, wrong actual platform, incomplete report and CI dirty source rejection. Write aggregate rerun/missing-platform/cancelled/skipped-dependent cases before implementation.
- [x] Run new tests and observe RED; implement explicit argv subprocess execution with bounded timeout and durable complete logs. Mandatory tool/native symlink preflight. Actual full root/example `-count=1 -json -coverprofile`, vet, Python tests (unittest report requires nonzero tests, no skip/fail/error), and Linux root/example race. Race logging catches independent CLI diagnostics even if negative-command tests capture stderr.
- [x] Produce actual base coverage in a private local clone at full base SHA, same native Go toolchain; compare candidate root/example against base with precise fractions and platform-specific fixed floors. Record source before/after, reject drift; do not compare raw platform-specific source fingerprint against Linux builder fingerprint, only matching commit and own-run stability.
- [x] Implement extended tests: repeated fresh selected concurrency/failure/recovery tests with exact required identities, and bounded Go fuzz (e.g. 20s, parallel=2). Logs/failures persist and race diagnostics block. No optional external services or paid models are invented.
- [x] Create one read-only workflow using checkout v6 `d23441a48e516b6c34aea4fa41551a30e30af803`, setup-go v6 `924ae3a1cded613372ab5595356fb5720e22ba16`, setup-python v6 `ece7cb06caefa5fff74198d8649806c4678c61a1`, setup-node v6 `249970729cb0ef3589644e2896645e5dc5ba9c38`, upload-artifact v6 `b7c566a772e6b6bfb58ed0dc250532a479d7789f`, download-artifact v7 `37930b1c2abaa49bbe596cd826c3c89aef350131`. Bundle names include run/attempt; native downloads exact bundle artifact ID. Matrix uses six spec labels, passes expected target/base commit/current SHA. Extended required for schedule/manual only. Fixed `P1 required` always executes, validates needs plus latest complete report matrix and candidate manifest hash; no path filter or continue-on-error.
- [x] Add docs for one local qualification command, safe outputs, bundle/source install, optional extended run, evidence boundaries, approved skip inventory and protection activation/inspection requiring later authorization. Do not claim hosted CI or protection enabled.
- [x] Run all suites and actual local `quality.py native` using newly built current and real historical bundles, plus extended locally. Verify cross-built six artifacts structurally but execute only Linux amd64 locally.
  Expected: local/native Linux tests pass, real installation/upgrade and smokes pass; other five native targets/remote required setting remain explicitly NOT_RUN/UNKNOWN.

## Final Verification and Review

- Run one fresh independent whole-branch review of tracked AND untracked P1 changes, spec/plan and ledger rulings. A diff of HEAD..HEAD would miss everything because commits are not authorized.
- Reproduce verified Important/Critical findings RED, fix GREEN, run full suites again. No second review loop; minor notes are disclosed, not silently fixed.
- Fresh final run must include root/example full/vet/race, all Python suites, actual release bundle verification and Linux qualification/install/upgrade/three smokes, bounded extended, shell syntax/parity, source/index/head/branch before/after and `git diff --check`.
- Save results/report, preserve branch and all evidence. No commit/push/merge/publish/protection change; report precisely what has and has not run.

### Single review-fix pass

The independent review found six Important groups. Re-grade by actual effects:
clean hidden-input edits belong to source identity; gzip completion belongs to
archive validation; valid Unicode Go JSON, controlled compiler provenance and
ordinary basename invocation are also functional fixes, not cosmetic minors.

- [x] Source input identity: `scripts/tests/test_source_identity.py` uses actual
  Go embedding and private Git flags; reject ignored compile/embed inputs for
  all six targets/modules and hidden index flags. RED→GREEN.
- [x] TAR expansion and completion: checksum-consistent PAX metadata, missing
  gzip footer and bad CRC cases RED→GREEN; inspect one fixed TAR header before
  metadata processing and bound all decompressed reads through gzip EOF.
- [x] Bundle traversal: reject unlisted filesystem entries; qualifier copies
  verified files only. Actual outside-symlink fixture RED→GREEN.
- [x] Process lifetimes: `scripts/tests/test_process_tree.py` proves both nested
  Runner sessions and standalone delivery descendants stop on timeout; shared
  owned registry/worker primitive RED→GREEN. The Windows completed-process API
  contract has a portable simulated regression; native Windows is not claimed.
- [x] Python completeness: both discovery scopes independently nonempty; reject
  expected failures and retain each scope's counts. RED→GREEN.
- [x] Fuzz failure evidence: `scripts/tests/test_fuzz_evidence.py` obtains an
  actual minimized native Go failure, then checks the extended failure/finally
  boundary preserves its bytes but not unchanged seeds. RED→GREEN.
- [x] Unicode framing/logging: actual Go U+0085 JSON and non-UTF-8-locale log
  decoding regressions RED→GREEN; ASCII LF framing, explicit UTF-8 decoding.
- [x] Compiler identity: inherited invalid toolchain fixture RED→GREEN; query
  compiler metadata under the exact controlled build environment.
- [x] Wrapper invocation: `bash install.sh --help` / `bash release.sh --dry-run`
  from scripts/ RED→GREEN; basename and qualified paths both resolve correctly.
- Final full-suite/installed-candidate result is recorded in this plan's ignored
  ledger and `final-*/verification-summary.json`. Keep this source-bound plan
  unchanged during/after that run; a pre-existing green report is not final
  evidence for later source edits.

### Requested review completion — 2026-11-13

The user requested completion of the remaining review fixes and a fresh final
full qualification. The formerly deferred unignored-checkout destination issue
is now addressed, without another independent review/re-review dispatch.

- [x] Reproduce real release/source-install failures in a private committed Go
  fixture with new unignored output/prefix paths, fresh/existing prefixes and a
  configured in-checkout temporary root. Existing source-change rejection
  tests must remain green; no user checkout install is used.
- [x] Keep release/source compilation staging outside source, falling back to a
  private writable sibling if the configured temporary root is inside source.
- [x] Pass the unique staged installation path to validation; source inventory
  can omit only that exact untracked ordinary non-build-input file. Reject
  tracked/nonregular exclusions and actual Go embed inputs, including all:.
  Preserve real Git dirty state and detect unrelated source changes.
- [x] Observe RED→GREEN for 5 checkout-delivery tests and 6 source-identity tests
  (3 existing plus 3 new exclusion guards). This adds 8 tests overall, rather
  than claiming all 11 were newly added. Record original product failures and
  missing-new-interface RED separately.
- Update the ignored ledger with a new fresh final run after these source/doc
  changes; earlier final-a5yjgwwz remains historical evidence only. Qualification
  output stays ignored/external because published files remain visible source
  inputs for later steps; no broad directory exclusion weakens source binding.

The original review agent reported an extra fork contrary to its dispatch
constraint; that deviation remains recorded. Final fixes use direct behavioral
regressions and a newly built candidate/full suite, not a new reviewer verdict.
