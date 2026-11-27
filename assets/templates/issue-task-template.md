## Task 1: [Component Name]

<!-- Native changes: save one file per task directly in the change's `issues/`
     directory, e.g. `01-short-name.md`. Use at least two filename digits;
     keep the positive number stable and match it in the heading and YAML.
     Number the plan 1..n with no gaps. Legacy `tasks.md` has its own template. -->

**Spec:** `.mulix/.runtime/<change>/spec.md` — [requirement and AC IDs]

**Goal, dependencies and constraints:** [what this task delivers, which
approved interfaces it uses, and the spec/constitution constraints it obeys]

**Files and interfaces:**
- Modify: `path/to/source` — [public contract and exact signature]
- Test: `path/to/source_test.go` — [scope, facilities and observable result]

Keep one `mulix-tests` fence for this task. Declare only this task in `tasks`
and only its cases in `cases`. Use unique TC IDs across all task documents;
together the tasks must cover every AC and each needs an executable case.
Use the exact emitted test identity and project-relative source file. The task
responsible for a final-only Karate case adds `verify_cases` with task `0` in
this same fence; see `.mulix/templates/quality-template.md`.

```mulix-tests
tasks:
  - number: 1
    kind: code
cases:
  - id: TC-001
    criterion: AC-001
    task: 1
    point: exact value preservation
    given: a literal input
    when: the public operation runs
    then: the returned value equals the literal input
    suite: unit
    test: fixture#TestValue
    file: value_test.go
```

**Step 1: Write the mapped test.** [test code and independent expectation]

**Step 2: Observe behavioral RED.** Run `mulix test run --task 1 --stage red`.
Expected: the mapped test executes and fails for the specified behavior,
not because of a build or environment failure.

**Step 3: Implement the minimum behavior.** [exact changes]

**Step 4: Observe GREEN.** Run `mulix test run --task 1 --stage green`.
Expected: the mapped tests and selected suites pass on the current inputs.

For a non-code task use `kind: verification` or `kind: documentation`, an
explicit `reason`, and an actual `--stage check` instead of invented RED/GREEN.
Record observed step results in the SDD `progress.md` ledger, not in this file.
Review and commit only when authorized; task completion does not imply merge.
