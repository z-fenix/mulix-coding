# Project quality policy

Create `.mulix/quality.yaml` with the project's real commands before approving
its first test matrix. This is project-owned configuration: init/update installs
this reference, never overwrites the policy. Review policy changes like code.
Every configured suite is mandatory for final `mulix verify`, including suites
not referenced by a task. Declare unit, integration and final-artifact checks
that the project needs; an undeclared suite cannot be discovered automatically.

```yaml
schema: mulix.quality.v1
suites:
  - id: unit
    command: [go, test, -json, -count=1, ./...]
    cwd: .
    format: go-json
    timeout_seconds: 300
```

Commands are argv arrays with no implicit shell; cwd is a physical directory
inside the project. Timeout defaults to 300 seconds, maximum 7200. Go commands
must disable result caching (`-count=1`) and avoid repeated identities (`-count`
greater than 1 or repeated `-cpu` values). Nested Go modules need separate suites
with their own cwd; root `./...` does not include them.

Formats:

- `go-json`: exact case identity is `Package#Test` from `go test -json` events.
  A mapped subtest uses its full name, e.g. `example.org/app#TestSave/invalid`.
- `json`: a real test adapter emits one JSON object per test on stdout, e.g.
  `{"name":"cli.creates-item","status":"pass"}`. Status is pass/fail/skip;
  failures must exit nonzero. Send diagnostic prose to stderr. Adapt the real
  framework's result stream, not an unconditional echo or a summary string.

### Opt-in Karate RC1 suite

```yaml
schema: mulix.quality.v1
suites:
  - id: karate
    format: karate-jsonl
    timeout_seconds: 300
    karate:
      version: 2.1.5.RC1
      sha256: 9e5fc19c560fa6824aae07a52ec8ca11542ecf6e93bd9f823b039477525cf9e3
      environment: qa
      env: [APP_BASE_URL]
```

Supply the reviewed JAR as an absolute `MULIX_KARATE_JAR` path. The runner
uses Java 21+ from PATH or an absolute `MULIX_KARATE_JAVA` installation binary;
it checks the JDK's `release` metadata without launching Java to check a
receipt. `karate.environment` and `karate.env` are optional. List only the
project environment variable names the test needs; provide values separately.
Mulix owns argv, cwd and output paths for this format. It never downloads a JAR.

Use `.mulix/.runtime/<change>/tests/features/task-N/` for that task's `.feature`
entries, `features/final/` for final-only entries, and `helpers/` for reusable
`.feature`/`.js`, optionally including `helpers/karate-config.js`. Feature
includes can use `this:../../helpers/...` from a directly nested task entry.
Keep reports, logs, credentials and the JAR out of `tests/`.

For each case, put its root-relative source in `file` and
`<file>#<exact scenario name>` in `test` (use the expanded Outline name).
Declare final-only mandatory cases under `verify_cases` in the same
`mulix-tests` block: use `id`, `criterion`, `suite`, `file`, and `test`, without
`task`. Task commands use only the mapped task's entries; final verification
runs all task and final entries, not helpers as standalone cases. It also runs
all other configured mandatory suites. Read the produced JSONL evidence; a zero
exit on a dry run, empty scope, or skipped required scenario is not PASS.

All suites require executed, non-skipped tests; mapped tests must be present and
not skipped. Missing/malformed/duplicate results, inconsistent exit codes and
infrastructure failures fail closed. Unmapped skips are counted and reported;
review their reasons before delivery. stdout and stderr are separately bounded
to 16 MiB. Keep generated build/test output in ignored directories or context
scratch so it does not change source during the run. Git projects fingerprint
tracked plus nonignored untracked files; non-Git projects scan regular files.
Ignored application inputs and external services/dependencies are not fully
bound: promote important inputs to tracked files and qualify externally in CI.

Use `mulix test run --task N --stage red`, then implement, then `--stage green`.
A successful RED command exits zero because it observed the required test failure;
the receipt retains the subprocess's actual nonzero exit. Tests, policy, spec
and plan definitions stay stable between RED/GREEN; verified checkbox-status
edits are normalized. Non-code tasks declare a concrete reason and use `check`.
The CLI saves receipts to existing state.yaml and summaries to task reports.
Human prose or `state set verify_result pass` cannot substitute for execution.

Final review goes in `docs/changes/<change>/report.md`:

```mulix-review
reviewer: actual reviewer identity or review session reference
result: approved
blocking: []
```

Run `mulix verify` after review, then `mulix state transition verify-pass`.
All mandatory suites and mapped cases must pass against current inputs. Editing
source, tests, spec, policy, plan definitions or the final report invalidates
final evidence. If finishing changes the revision after verify-pass, rerun
`mulix verify` in the not-yet-archived archive phase before `archived`.
Failed/incomplete retries supersede old successes. A corrective `verify-fail`
or a new `build-complete` verification cycle invalidates old final verification;
historical task RED/GREEN provenance remains. Correct the underlying
failure; do not use `--force` to claim acceptance. Recovery requires an explicit
`--reason`, records guard overrides and creates no test qualification.

This is local evidence, not cryptographic attestation. Review identity is a
local claim; semantic assertion quality and reviewer independence need human
review. A writer controlling policy, adapters and all local files can forge
claims outside this workflow's assurance boundary.
