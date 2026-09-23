# T006 — Brief: add argument parsing and validation

Task: Implement add-argument parsing (`parseAddArgs` scanning for
`--priority` anywhere), validation, and default handling in
src/cli/add.go, making T005 pass.

Context for the implementer:

- Do not use `flag.FlagSet` — it stops parsing at the first non-flag
  token and would silently drop a trailing `--priority`.
- Validate before any store write: invalid value exits non-zero with a
  clear error and the store stays untouched.
- Default to `models.PriorityMedium` when the flag is absent.

Dispatched to: subagent-3.
