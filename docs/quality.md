# Repository quality and final-artifact qualification

P0 gates individual changes with actual task/suite receipts. P1 checks this
repository and its final deliverables. Neither authenticates human approval,
proves arbitrary business assertions, or qualifies a paid model/real host.

## Run locally

Use the exact tool versions in `scripts/quality-policy.json`, plus Git, Bash,
a C compiler for Linux race, and native symlink capability. Python uses only
the standard library. Missing tools/capabilities are failures, not approved
skips. Add the installed Go toolchain to PATH if needed.

For Linux amd64, from the repository root:

```bash
work="$(mktemp -d)"
python3 -I -B scripts/quality.py bundles --output "$work/bundles" --version local-p1
python3 -I -B scripts/quality.py native \
  --bundle "$work/bundles/candidate" --baseline-bundle "$work/bundles/baseline" \
  --target linux_amd64 --output "$work/native"
python3 -I -B scripts/quality.py extended --output "$work/extended"
```

Use the actual target on another native platform; `native` rejects a different
host/toolchain. `extended` currently runs on Linux. All output directories must
be new. Keep `work` to inspect reports, raw stdout/stderr, commands, coverage,
private old/new projects and installed binaries. Nothing is installed into the
real user's PATH. This recipe makes no commit, push or release.

`bundles` builds all six current artifacts and an authentic historical baseline
from the pinned commit in the policy, using a private local clone. The baseline
is a tested commit, **not a claim that a historical published release was used**.
Native qualification never substitutes a rebuilt CLI for the provided final
artifact. Unit/component tests may compile their own independent fixtures.

Local dirty-source testing is supported, but the bundle must match the exact
current source inventory; rebuild after edits. Go's actual compile/embed inputs
are checked for all six targets and both modules: an ignored input must first
be tracked or unignored. `assume-unchanged`/`skip-worktree` index flags are
rejected rather than treating hidden edits as clean source. This does not
change those flags or the user's index.

A local PASS is not a clean
six-platform release qualification. CI additionally requires clean Git state,
current commit/run context and a complete native matrix.

## Mandatory checks

Every PR/main push/tag/manual/scheduled qualification runs:

- Root and nested example full tests, `-count=1 -json`; both modules' `go vet`.
- Linux root and example race. The separately built scaffold CLI also has
  `-race`; build-info tests verify this. A private race log catches diagnostics
  even when a negative-command fixture captures child stderr.
- Both Python discovery scopes must execute nonzero tests independently, with
  no failed/error/skipped/expected-failure/unexpected-success results. Counts
  for each scope are retained; one suite cannot conceal an empty other suite.
- Coverage versus the actual same-platform base revision in a private clone.
  Linux amd64 also has a fixed floor from observed baseline data. Fractions are
  compared exactly; repeated `-coverpkg` blocks are merged before counting.
- Owned shell syntax, embedded override parity and Git whitespace checks.
- Verified bundle → temporary prefix → version → initialization → all three
  smoke scripts against the same installed final binary (DELIVER-01).
- Real pinned old bundle → old project → corrupted-install rejection → current
  bundle installation → actual upstream/local conflict → rejected invalid
  acceptance → explicit fixture resolution → successful retry. User data,
  local resolution and pure upstream baseline are checked (DELIVER-02).

The policy lists required test identities and every allowed platform/reason
skip. Five Windows-specific identity tests may skip only on Linux/macOS; they
must execute on Windows. Unix permission/TMPDIR cases have explicit Windows
exclusions. Optional upstream regeneration may remain unexecuted without a
matching `MULIX_SUPERPOWERS_SOURCE`; it is reported as SKIP, not PASS. Missing
Bash, Node, Git or symlink support cannot be excused that way.

Scheduled/manual jobs additionally repeat selected actual cross-process,
interruption and partial-write recovery tests, and run bounded Go fuzz. The
fuzz target compares fenced extraction with the equivalent unfenced YAML,
rejects duplicate fences and exercises arbitrary malformed input. A bounded
passing run is not an exhaustive security or correctness proof. New or changed
minimized failing corpus files (up to 8 MiB each) are retained under
`fuzz-reproducers/` even on failure and uploaded with extended evidence.

Timeout cleanup uses a private per-invocation, nested command registry. Workers
register before exec; cancellation reaches the owned descendant branches even
when an inner runner starts another session. Standalone delivery commands use
the same primitive. This is cooperative framework process ownership, not a
sandbox for code deliberately escaping it. Windows/macOS behavior still needs
the corresponding native hosted runs.

## Build, inspect and install artifacts

```bash
MULIX_VERSION=v0.1.0 bash scripts/release.sh --output /new/path/mulix-v0.1.0
python3 -I -B scripts/delivery.py verify --bundle /new/path/mulix-v0.1.0
bash scripts/install.sh --bundle /new/path/mulix-v0.1.0 --prefix /chosen/bin
```

Run these repository scripts from a checkout; they require Python 3.10+.
Artifact mode needs neither Git nor Go. The bundle contains six one-file
archives, a version/source/tool-bound manifest and SHA256SUMS. Validation
checks the full inventory, archive and binary hashes, sizes, ordinary members
and actual executable architecture. The filesystem inventory must contain only
those eight regular files, with no unlisted directories or symlinks. Unix
archives allow one ordinary TAR member only: PAX/GNU extended headers are
rejected before parsing metadata, decompressed reads are bounded by the
manifest binary size plus fixed padding, and gzip is checked through EOF for
CRC/trailer integrity. No arbitrary paths are extracted.
Checksums do not authenticate the publisher:
obtain the bundle and its expected checksum through a trusted channel.

Default release output is `dist/mulix_<version>`, never a deletion of `dist/`.
Existing output is refused even when empty. `--dry-run` prints a plan only;
it builds/runs nothing and cannot be acceptance evidence. Compiler identity is
queried under the same controlled toolchain environment as compilation.

Direct release/source installation supports new **unignored** output/prefix
paths inside the checkout. Compilation staging stays outside source (a private
sibling directory is used if the configured temporary root is itself inside
source; that location must be writable). Installation rechecks source after
executing the staged version, omitting only this invocation's exact untracked
ordinary staging file. Tracked files, symlinks/directories and actual Go
compile/embed inputs cannot be exempted. No whole output/prefix directory is
excluded, and Git dirty status is not rewritten as clean.

Published outputs remain visible to subsequent source inventories. The
multi-step qualification commands above therefore keep their evidence and
bundles outside source or in ignored directories; otherwise their own final
outputs would change the candidate between steps.

The installer writes a unique same-filesystem staging file, executes its
actual version, rechecks inputs and atomically replaces the destination.
Validation/version/replacement failure preserves the old executable and
unrelated data. A killed installer can leave its private staging file; retry
uses a fresh name and does not sweep files it does not own. Remove an orphan
only after identifying it and verifying no owning process remains. This is
process-interruption recovery, not a tested power-loss or malicious-writer
transaction/sandbox guarantee.

Source installation remains available:

```bash
bash scripts/install.sh --source /local/mulix-checkout --prefix /chosen/bin
# An explicit local-source --version stamps that build; it is not tag authentication.
bash scripts/install.sh --source /local/mulix-checkout --version local-debug --prefix /chosen/bin
```

With neither `--source` nor `--bundle`, explicit invocation retains the remote
source-clone behavior (main, or the requested `--version` ref). No default
installation is invoked by the test driver. Python can be selected explicitly
with `PYTHON=/absolute/path/to/python3`.

## CI and release boundary

`.github/workflows/quality.yml` pins action dependencies to full commits,
uses a read-only token, leaves no checkout credential behind and has no path
filter. One job builds the bundles. Six jobs download that exact immutable
artifact ID and execute natively:

| Target | Runner |
|---|---|
| linux_amd64 | ubuntu-24.04 |
| linux_arm64 | ubuntu-24.04-arm |
| darwin_amd64 | macos-15-intel |
| darwin_arm64 | macos-15 |
| windows_amd64 | windows-2025 |
| windows_arm64 | windows-11-arm |

Windows runners must actually provide symlink capability; do not turn a
capability failure into a blanket test skip. Runner availability, native
behavior and tool downloads must be verified by the hosted run, not inferred
from this file or a Linux cross-build. Private runner usage may consume billed
minutes; this implementation does not launch a hosted job or change billing.

The fixed **P1 required** job runs even after a failed dependency. It requires
successful jobs plus a complete latest-attempt native report set for this
run/commit/candidate manifest hash. Old PASS reports cannot cover a latest
FAIL, omitted target or changed candidate. If bundles are rebuilt on rerun,
rerun all targets against those new bytes. The optional extended job is
mandatory for schedule/manual events and deliberately unscheduled on PR/push.

No workflow publishes a GitHub release or installs to a real project. For a
release, publish the exact qualified bundle, not another source rebuild, and
archive the manifest/matrix/native reports and raw evidence with it. Actions
artifacts here expire after seven days; they are not a long-term certificate.

## Required-check activation (separate authorization)

A YAML file does **not** enable branch protection. A repository owner must
first run and inspect the workflow, then make `P1 required` an enforced check
for the default branch/ruleset, with up-to-date source as appropriate. Review
changes to the workflow and policy: source-level gates are not cryptographic
attestation against someone authorized to rewrite those gates.

Read-only examples, when an authenticated `gh` with sufficient access exists:

```bash
gh api repos/z-fenix/mulix-coding/branches/main/protection/required_status_checks
gh api repos/z-fenix/mulix-coding/rules/branches/main
```

Permission/404 failures do not establish that protection is absent. At P1
implementation time, this machine has no `gh`; remote protection is UNKNOWN,
and the five other target native jobs have not run. Enabling protection,
pushing, publishing and actual user installation require separate permission.

Runner label sources (queried 2026-11-12):
- https://docs.github.com/en/actions/reference/runners/github-hosted-runners
- https://docs.github.com/en/actions/how-tos/write-workflows/choose-where-workflows-run/choose-the-runner-for-a-job
