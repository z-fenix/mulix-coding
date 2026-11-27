# Resuming From an Existing Ledger

Part of subagent-driven-development. This plan's workspace
already has a ledger at `<workspace>/progress.md`.

If the first line names the approved plan path, resume at the first task
without current closure. For native plans the identity is the Mulix-root-relative
`issues/` directory; a `- Task N: complete (receipt <id>)` line must name the
latest valid task receipt. Later tasks can advance the source without undoing
that closure. A superseded receipt or changed task definitions make previous
entries historical. Legacy plans still accept their original completion lines.

If the last line for a task is a fix round, resume at the next round. Leave
ledgers that name another plan untouched, and never adopt the old flat
`sdd/progress.md` for a native task directory.
