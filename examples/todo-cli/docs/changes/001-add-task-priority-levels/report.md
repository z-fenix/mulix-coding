# Verification Report: Add Task Priority Levels

**Change**: `001-add-task-priority-levels`

## Automated Checks

- `go build ./...`: pass
- `go vet ./...`: pass
- `gofmt -l .`: no files need formatting
- `go test ./... -v -count=1`: all 11 tests pass (4 in `src/models`, 7 in
  `tests/unit`)

## Manual Verification Against Acceptance Scenarios

Ran the built `todo` binary directly against every acceptance scenario
in spec.md:

- US1 scenario 1 (`todo add "buy milk" --priority high`): task created
  with priority `high`. Confirmed.
- US1 scenario 2 (`todo add "buy milk"` with no `--priority`): task
  created with priority `medium`. Confirmed.
- US2 scenario 1 (mixed low/high/medium tasks, `todo list`): output
  order high, medium, low. Confirmed.
- US2 scenario 2 (same-priority tasks at different times, `todo list`):
  older task listed first. Confirmed (covered by
  `TestSortByPriority_TiesBreakOldestFirst` and
  `TestList_OrdersHighMediumLowWithOldestFirstTies`).
- US3 scenario 1 (`todo set-priority <id> high` on an existing task):
  priority updated. Confirmed.
- US3 scenario 2 (`todo set-priority <id> high` on an unknown id): exits
  non-zero with "task not found", no task modified. Confirmed.
- Edge case (`--priority urgent`, an invalid value): exits non-zero with
  a clear error, no task written. Confirmed (see "Issue found and
  fixed" below — this initially passed silently).
- Edge case (pre-existing task JSON with no `priority` key at all):
  loads without error and lists as `medium`. Confirmed by
  `TestLoad_TaskWithoutPriorityFieldDefaultsToMedium`.

## Issue Found and Fixed During Verification

Manual smoke-testing the built binary (rather than only the unit tests,
which called `cli.Add`/`cli.SetPriority` directly with already-split
arguments) surfaced a real bug: `todo add "buy milk" --priority high`
silently ignored `--priority` and stored `medium`, and
`todo add "bad" --priority urgent` exited 0 instead of rejecting the
invalid value.

Root cause: `runAdd` used Go's `flag` package to parse `args`, but
`flag.Parse` stops consuming arguments at the first non-flag token. The
CLI's own usage puts the description before `--priority`
(`todo add <description> --priority <level>`), so `flag.Parse` treated
`--priority high` (or `--priority urgent`) as trailing positional
arguments and never populated the `--priority` value at all — every
call silently fell through to the empty-string default.

None of the existing unit tests caught this because they all called
`cli.Add(store, description, priority)` directly with `priority`
already split out, bypassing the argv-parsing layer entirely where the
actual bug was.

Fix: replaced the `flag.FlagSet`-based parsing in `runAdd` with a
hand-rolled `parseAddArgs` that scans for `--priority` anywhere in the
argument list rather than requiring flags before positionals. Added
`TestRun_AddWithTrailingPriorityFlagIsParsedCorrectly` in
`tests/unit/add_test.go`, which calls `cli.Run` with the actual argv
shape (`["add", "buy milk", "--priority", "high"]`) instead of calling
`cli.Add` directly, so this class of bug is now covered by a test that
exercises the same layer where it occurred.

Re-ran the full manual smoke test and the full automated suite after
the fix; both are clean (see above).

## Result

**verify_result: pass**
